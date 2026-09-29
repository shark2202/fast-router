---
type: Research Note
title: Qwen3.8-27B-in-C 深度研究
description: 核验 Qwen3.8-27B 原生 C CPU 推理引擎的架构、性能、正确性、macOS 构建结果及 fast-router 集成价值。
source: shyringo/qwen3.8-27b-in-c GitHub 固定提交、源码与本地构建验证
timestamp: 2026-09-29
---

# Qwen3.8-27B-in-C 深度研究

## 结论先行

这个项目证明了**大模型 CPU 推理可以通过专用原生实现、低比特权重与细致内核优化在普通笔记本上运行**，但“能运行”不等于低延迟：其公开基准在 Intel i5-1340P / WSL2 上，IQ1_M 约 2.52 token/s、首 token 4.064 秒；Q4_K_M 约 1.88 token/s、首 token 4.359 秒。

对 fast-router 来说，它有两个不同角色，不能混为一谈：

1. **作为本地聊天生成上游**：实现 OpenAI Chat Completions，可作为本地模型服务候选；CPU 速度低且服务端能力有限，须实测并加访问控制。
2. **作为 Jev/System One 决策引擎**：不能直接接入。它是文本生成 API，没有 `/v1/systemone`、pointer decision head、路由类别评测或 choice probability contract。要转换为 Jev scorer 需要新增决策封装/评分逻辑并重新训练或设计 prompting，不是换个 URL。

本次还在当前 Intel i7-9750H macOS 环境执行了上游自带构建测试：`make portable` 全部通过；`make strict` 在 AVX2 内存探测代码处编译失败，报 `_SC_AVPHYS_PAGES` 未声明。故 README 的 macOS 使用指引与该固定源码快照的 strict/native 构建之间存在需要上游修复或本地适配的问题；本次没有改动外部仓库，也没有下载 6–16 GB 模型权重运行真实推理。

## 来源与方法

### 来源

研究固定在 `shyringo/qwen3.8-27b-in-c` `main` 快照 `a8809a93e6a8db063490f0e11f52eb8be2861f9b`（提交时间 2026-09-10；核对时与远端 `refs/heads/main` 一致）。主要来源：

* [中文 README](https://github.com/shyringo/qwen3.8-27b-in-c/blob/a8809a93e6a8db063490f0e11f52eb8be2861f9b/README.zh-CN.md)：架构、使用方式、性能与精度边界。
* [架构说明](https://github.com/shyringo/qwen3.8-27b-in-c/blob/a8809a93e6a8db063490f0e11f52eb8be2861f9b/docs/ARCHITECTURE.md)、[优化说明](https://github.com/shyringo/qwen3.8-27b-in-c/blob/a8809a93e6a8db063490f0e11f52eb8be2861f9b/docs/OPTIMIZATIONS.md)、[正确性说明](https://github.com/shyringo/qwen3.8-27b-in-c/blob/a8809a93e6a8db063490f0e11f52eb8be2861f9b/docs/CORRECTNESS.md)、[API 限制](https://github.com/shyringo/qwen3.8-27b-in-c/blob/a8809a93e6a8db063490f0e11f52eb8be2861f9b/docs/API.md)。
* 固定快照源码：`GNUmakefile`、`src/qwen38/qwen38_model.c`、`src/qwen38/qwen38_quant.c`、`src/cli/qwen38_http.c`、`include/qwen38/qwen38_model.h`。
* fast-router 本地 `router/systemone_engine.go`、`router/systemone_http.go`、`router/scorer.go` 和 `docs/CPU-LLM推理方案评估-2026-09-29.md`。

### 方法

1. 核对 GitHub main 的 SHA 与本地 shallow clone 的 HEAD。
2. 将 README 性能数据拆成模型量化、硬件、上下文、TTFT、TPOT、RSS 和计时边界，并检查正确性文档对“无损”的定义。
3. 静态检查 C ABI、HTTP endpoint、模型加载和 CPU 优化实现，分别判断它是 System One engine 还是普通生成 backend。
4. 在本项目现有 Intel x86_64 Mac（Core i7-9750H）上运行上游提供的 `make portable` 与 `make strict`，不下载模型权重。

## 发现

### 1. 它是专用 C 推理 runtime，不是“把 llama.cpp 包起来”

上游实现 GGUF 读取、Qwen3.8 模型计算图、tokenizer、采样、聊天与 HTTP server；项目目标是无 GPU/CUDA/Python/PyTorch/外部推理框架，在 CPU 上直接运行固定来源的 Unsloth Dynamic V3 GGUF。

模型为 27B、64 层混合架构（48 层 Gated DeltaNet、16 层全注意力），词表 248,320。README 支持 IQ1_M、Q4_K_M 等量化权重；代码里实现对应量化解码、AVX2/VNNI 路径和 DeltaNet 状态更新。不是一般意义上可任意加载模型的统一 runtime；对 fast-router 而言，需考虑模型架构锁定、GGUF contract 和上游专有推理实现。

### 2. 公开性能是“笔记本能跑”，不是交互式低延迟

README 记录的参考机为 Windows 11 + WSL2 Ubuntu 22.04.5、Intel i5-1340P、32 GB host RAM、WSL 24 GiB 上限、GCC 11.4、4,096 context、12 OpenMP threads。权重、tokenizer、运行时布局、状态和线程池在测量前已常驻；不使用 swap。

| 权重 / 负载 | TTFT | TPOT | 速度 | 峰值 RSS |
|---|---:|---:|---:|---:|
| IQ1_M，直接生成 16 token | 4.064 s | 0.397 s/token | 2.52 token/s | 6.13 GiB |
| IQ1_M，4-token batch | — | 0.253 s/token | 3.95 token/s | — |
| Q4_K_M，直接生成 32 token | 4.359 s | 0.531 s/token | 1.88 token/s | 14.49 GiB |

这些是多次运行中的最佳墙钟时间，不是 P50/P95。TTFT 从已常驻的引擎收到请求时开始，不包括启动/加载/映射；生成 TPOT 才计完整模型 decode。4-token batch 是模型内部的批处理路径，不等价于多个并发 HTTP 请求。对同步路由决策而言，四秒首 token 和每个 token 约 0.4–0.5 秒，远非亚秒决策模型。

用户当前 Intel Mac 为 i7-9750H，与基准不是同款 CPU，也不是相同操作系统、内存带宽和编译器。不能由 i5-1340P/WSL2 数据推导本机速度，须本机加载模型并按 fast-router workload 重新测。

### 3. “精度无损”限定为同一 GGUF 下的 runtime 数值一致

项目把优化路径与同一份量化 GGUF 的 native baseline 对照：完整 logits 逐 bit 一致。这是对执行实现的正确性证据，不是 IQ1_M/Q4_K_M 与 BF16/FP8 权重精度相同的声明。

其独立 llama.cpp oracle 比对也显示差异：Q4_K_M 的全 logits RMSE 约 0.0747、cosine 约 0.99927；IQ1_M RMSE 约 0.436、cosine 约 0.981，top-5 集合相同但后两位次序变化。故适当说法是“自有优化不在相同 packed quantized weights 上再引入额外误差”，不可简化成“模型准确率无损”。

实现的优化具有工程价值：分层 prompt batching 共享权重遍历；低比特 SIMD 核；内存条件允许时重排 IQ1 布局；DeltaNet 状态遍历融合；跨轮次 tail 融合；受控线程数；本机 mmap；MTP 事务回滚。不过 MTP 在参考负载上的校验开销高于收益，默认不开启。优化收益依赖具体 CPU、量化模型与上下文。

### 4. 它没有 System One API 或分类概率输出

服务端提供本机绑定的 OpenAI-compatible `/v1/chat/completions` 和生成/工具调用，不提供 `/v1/systemone`。C header 暴露加载 GGUF、forward token、prefill 等原始模型 logits API，但没有“问题 + 多候选 → calibrated probabilities”的 decision primitive。

相比之下，fast-router `HTTPSystemOneClient` 只接受 System One response；直接把 Qwen 的 Chat Completions endpoint 填进去会因为请求和响应 schema 不同而失败。可能的改造路径是：

* 快速但行为较脆：包装器把 A–J prompt 发给生成模型，强制输出单个 code 再解析；没有概率分布，错误格式/拒答需处置，且新增自回归延迟。
* 结构化但工作量较大：用 C API 建 decision/scoring adapter（例如固定 prompt 下读取各候选 next-token logits，或多选项打分），再定义概率归一化、校准、隔离、超时、上下文规则与评测。这个方案仍不等同 Kev 的 pointer head，也没有可用的 fast-router 准确率证据。

如果目的是用它作为“本地目标 LLM”处理用户的生成请求，则 OpenAI chat API 是有用接口；如果目的是“替代 Jev 做路由分类”，它不是 drop-in。

### 5. macOS 构建验证结果有明确边界

在当前 Intel macOS 环境、上游快照源码目录执行：

* `make portable`：通过。禁用架构专用 SIMD 与 OpenMP，构建主程序、工具及全部单元测试；GGUF、quant、sampler、NFC、HTTP/JSON、tool parser 等测试通过。
* `make strict`：失败。Clang 在 `src/qwen38/qwen38_model.c:1339` 报 `use of undeclared identifier '_SC_AVPHYS_PAGES'`。该代码位于 AVX2 内存探测分支，用 `sysconf(_SC_AVPHYS_PAGES)` 读取可用页数；当前 Darwin SDK 未定义此 Linux 常量。

这并不证明 macOS 完全不能运行：portable 构建已在本机通过；但 native AVX2/OpenMP strict 路径尚未在当前环境通过。README 的 quick start 需要 native 性能路径时，应该先修复/条件编译 macOS 可用内存探测，再重新跑 strict 和带真实权重的推理。我们未修改外部项目或本地项目代码。

### 6. 对 CPU 路由模型的间接价值：性能工程参考多于可复用引擎

Qwen C repo 的量化 SIMD、内存映射、batch prefill、CPU 线程调度和 runtime correctness test 值得学习；但它优化的是 27B 自回归生成。fast-router 的路由是 System One 多类别决策，已有候选 scorer/批量 KV 复用且有专用小模型探索。把生成模型做得能以 2.5 token/s 运行，并不会自然变成快的分类器。

若 fast-router 需要一个**本地聊天上游**，它可以作为小流量/离线实验；若目标是让 Jev 第一次同步评分更快，应优先 Kev 这类 decision-native 模型或专用小 scorer 的公平评测，而不是直接集成该 27B chat server。

## 成功维度、死法与可逆性

| 试验目的 | 成功看什么 | 最可能怎么失败 | 回退 |
|---|---|---|---|
| 本地聊天上游 | 在本机目标提示与上下文下 P50/P95、首 token、RSS、输出质量达到用途门槛 | 低比特模型能加载但慢、首 token 长、请求串行，或 OOM | 保留现有远端 upstream；本地仅测试 profile |
| System One 适配 | JSON contract、类别概率/校准、独立路由集 P1、全链路 P95 | Chat 文本包装不稳定；把 next token score 当 Jev 概率；数据域不匹配 | 新适配器以 feature flag/shadow 方式部署 |
| 性能工程复用 | 同一 scorer/模型的热点得到可复现收益，数值与基线符合约束 | 直接移植模型专用 kernel 或错误复用实验结果 | 单独 benchmark 分支；不动生产 runtime |
| 原生 macOS 运行 | Intel strict 构建、真实量化权重、端到端会话可重复 | AVX2 路径编译失败、实际权重/内存/散热不达预期 | portable 仅作功能验证；失败则不进入生产 |

## 建议

1. **不要直接把当前仓库作为 Jev engine 接入生产。** 接口和算法都不对齐。
2. 若评估“27B CPU 低成本生成”，只做独立本机 benchmark；记录 cold-start、resident TTFT、短/长 prompt、连续多请求的 P50/P95、RSS、温度/频率降速。不能用 README 的最佳单次数据替代。
3. 若研究 C runtime，先将本次 strict 构建错误作为 upstream compatibility issue/本地 patch POC 单独处理；补 macOS 和 Linux 双平台 `make strict`、`make portable`，再下载哈希固定权重运行。
4. fast-router CPU 路由主线优先保持决策任务窄：对比已有 64M scorer、Kev-0.8B/4B（如果能满足运行环境）与当前 9B baseline；统一冻结标签、候选、模板、线程、冷/热状态及 P50/P95。
5. 在没有任务准确率和路由延迟实测之前，该项目在 fast-router 中应标为 **CPU runtime 研究参考 / chat-upstream candidate**，不是 System One 引擎候选。

## 局限

1. 上游 benchmark 是项目自报最佳值，本研究未在其参考机上独立复现。
2. 本机只做了构建/单元测试；未拉取多 GB GGUF，未验证加载、性能、实际对话质量或 RSS。
3. 当前源码只在 i7-9750H macOS portable 路径通过；strict 失败的具体兼容问题不等同完整 macOS 支持结论。
4. 不同 IQ1_M/Q4_K_M 的模型质量、CPU ISA、OpenMP、上下文和散热均可能改变结果。
5. C API 暴露原始 logits 不代表已经具备 System One 评分语义；任何 adapter 都须单独建立任务评估和 calibration。

## 结论

**这是一个有价值的 CPU 推理工程实现，但不是当前 fast-router 的 Jev/System One 可集成方案。** 它提供了“27B 可在低内存 CPU 上执行”的证据，也清楚展示了代价：多秒 TTFT、约 2 token/s、模型专用实现和服务串行边界。对 fast-router，短期应借鉴其 CPU kernel/correctness 方法；若测试本地聊天，则把它作为独立 OpenAI-compatible upstream；若替代 Jev，则需另做 scoring adapter、训练/校准和独立路由评估。

本次本机 portable 测试通过、strict/native 测试失败，是已记录的具体 macOS 构建差异；尚未验证带权重推理，更不能宣称项目已在当前 Mac 上完成可用性验收。
