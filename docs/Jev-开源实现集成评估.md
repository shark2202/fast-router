---
type: Research Note
title: Jev / System One 开源实现集成评估
description: 盘点截至 2026-09-26 可用的开源 System One 实现，并评估它们与 fast-router 的集成路径、风险和验证顺序。
timestamp: 2026-09-26T00:00:00-04:00
tags: [jev, system-one, integration, research]
---

# Jev / System One 开源实现集成评估

> **结论先行**：确实已有多个开源实现。对 fast-router 来说，最适合先做可逆集成试验的是 **LLM2Jev/llm2jev 的本地 HTTP 服务**；若要同协议替换模型服务，可并测 **Laya** 与 **local-jev**；若希望沿用 GGUF + llama.cpp 且部署一个 System One 服务，可测 **jev-rs**。现有 Go/Zig 路径仍可保留为内嵌实现，但它当前的 yes/no 评分不等于最新 llm2jev 的单次候选标签评分，且尚缺真实模型端到端验收。

## 来源

本报告依据以下一手材料及本仓库截至 2026-09-26 的源代码：

* [llm2jev README](https://github.com/tic-top/llm2jev)：当前上游仓库；项目历史名 AnyJev，README 描述当前 API、后端和算法。
* [LLM2Jev 请求到模型说明](https://github.com/Yinsongxu/LLM2Jev/blob/main/docs/request-to-model_zh.md)：逐候选 yes/no 算法说明；它解释了本仓库早期研究笔记引用的方法，但不能代表当前 llm2jev 主线。
* [Laya](https://github.com/LeonaDavinci/laya-system-one)：模型、HTTP 服务与公开协议。
* [local-jev](https://github.com/amithgc/local-jev)：本地模型服务、支持平台及项目自述的测试边界。
* [jev-rs](https://github.com/yijunyu/jev-rs)：Rust 服务、llama-server/GGUF 接法及服务端能力。
* [openjev-sglang](https://github.com/ekzhang/openjev-sglang)：SGLang GPU 服务实现。
* [SemIf](https://github.com/TheoLeeCJ/SemIf-OpenJev)：开源模型评分项目及其自述定位。
* [AgentJev](https://github.com/malevrigns/agent-jev)：专用决策头、独立 HTTP 协议和服务运行说明。
* 本仓库：`router/scorer.go`、`router/zig_backend.go`、`zig/frwrapper.zig`、`SOP/build.md`、`WIKI/implementation-status.md`。

## 方法

1. 优先查看上游项目 README/文档，而非仅按项目名或二手清单筛选。
2. 按“推理实现/服务”“模型权重”“算法范式”分别归类，避免把同一 API 误当成同一模型或同一算法。
3. 以 fast-router 已有请求结构、Backend 接口、Go/Zig 动态库构建和运行时依赖评估集成成本。
4. 对准确率、速度等项目自报结果只作为候选线索；不把跨数据集、跨机器的数字当作可比结论。

### 2026-09-26 上游复核快照

本轮对公开主分支做了浅克隆和源码级检查，锁定以下复核快照，避免把浮动的 `main` 分支误写成固定依赖：

| 项目 | 复核 commit | 源码确认的关键事实 |
|---|---|---|
| llm2jev | `2b252d504972764211ef172c1155ac0fedc9c3de` | MIT；`POST /v1/systemone`；每个问题一次 prefill；SGLang/vLLM/HF/MLX backend；支持 `score_many` |
| Laya | `d120d4ba220711b93c171973118753460310e16b` | Apache-2.0；`laya-serve` 提供 `/v1/systemone`；可选 bearer auth；有 CPU/GPU/Docker serving 路径 |
| local-jev | `64a0b31ff343dca32142496cf9edca5a0174a18d` | README 明确标注 MIT；默认 Qwen LLM 与 `nli-deberta-large` 轻量选项；默认端口 8765 |
| jev-rs | `e0d906be222f8a38d102924a99d7e1c8019fc174` | `MIT OR Apache-2.0`；`jev serve`、llama-server、OpenAI-compatible 和 whole-request System One backend 都在同一工程内 |
| openjev-sglang | `5f633dccaf5a45c7e0a45d06653d019bb6bf1441` | 目标是 SGLang + Qwen3.6-35B-A3B + B200；API 与模型进程拆分，偏 GPU 部署 |
| AgentJev | `a965ca8ff06ccabc0c796dca5447b55cc2069cee` | Apache-2.0；`/api/evaluate` 自定义协议；checkpoint 是专用 decision head，不是普通 causal GGUF |

## 候选方案

| 方案 | 类型与当前能力 | 与 fast-router 的集成方式 | 适用理由 | 主要代价/风险 | 当前判断 |
|---|---|---|---|---|---|
| **llm2jev**（原 AnyJev） | Python/HTTP System One 实现；支持 SGLang、vLLM、Transformers、Apple Silicon MLX；最新 README 是“每个问题一次 prefill，读取所有候选标签 logits”，提供 `/v1/systemone` | 单独运行服务，在 Go gateway/scorer 前增加 HTTP System One client；也可借鉴其当前单次标签评分逻辑改造原生 Backend | 选择广、接口直接；对已有本地模型服务可减少重复造服务层；MLX 适合 Apple Silicon 快速试验 | Python 与模型引擎另行部署；vLLM 对 logprobs/served model 等有启动约束；prompt、标签 tokenizer、temperature 和模型模板必须一致 | **优先做 sidecar 对照实验**，不是把源码直接塞进 Go |
| **Laya** | 非自回归决策模型及可选 Jev-compatible HTTP server；支持 choice/score/noul、多语言 | 将 `POST /v1/systemone` 配置为外部决策服务，做 Go 侧薄 client | 协议和答案 shape 与 Jev 接近，专门决策模型，适合作为质量/延迟对照 | Python/PyTorch 部署和模型权重；模型上下文/语言/类别能力需用 fast-router 数据集实测；性能数字依赖硬件/批处理；发布许可须核对代码和各 checkpoint | **可集成，适合模型质量对照** |
| **local-jev** | 提供 `/v1/systemone` 本地服务；包含生成式 LLM、NLI 等 backend 和校准/模型卡机制 | 同样通过外部 HTTP client 接入 | 有较小 NLI checkpoint 可作为轻量基线，也可切换更大 LLM checkpoint；协议开箱即用 | 项目说明首次获取 `nli-deberta-large` 约 0.87 GB，默认选择准确率更高的 LLM 时约 9.32 GB；实际速度/内存看硬件；需分别评估代码 license 和各模型权重 license | **可集成，适合作为本地服务基线** |
| **jev-rs** | MIT/Apache-2.0 Rust 工具/服务；`jev serve` 暴露 `/v1/systemone`，通过 llama-server 使用 GGUF；支持本地/兼容 OpenAI 后端 | 可用已有 llama.cpp GGUF 独立启动服务，再接 Go HTTP client；或把其 Rust scorer 当作独立进程 | 与 fast-router 当前 GGUF + llama.cpp 路线相容，且提供服务、评测与校准命令 | 需要额外 Rust/llama-server 运行组件；请求协议兼容不代表 prompt/logits、概率和 confidence 等数学细节一致 | **可集成，若目标是 GGUF 服务化则值得并测** |
| **openjev-sglang** | Jev-compatible HTTP 服务；README 描述以 Qwen3.6-35B-A3B、SGLang 和 B200 为目标的 GPU 容器 | 部署到 GPU 主机，fast-router 通过 HTTP 调用 | 面向吞吐、prefix/radix cache 与多问题共享前缀，适合已有 GPU serving 基础设施 | 本机 CPU/轻量单机不是目标环境；GPU、显存、容器运维成本高 | **技术上可集成，当前 fast-router 本地默认发行形态不优先** |
| **SemIf** | 独立的开放模型 System One 风格评分实现，包含多个模型/backend 的评分研究与实验 | 若其稳定服务协议可用则外部适配；否则按其 prompt/readout 作为实验参考 | 有不同模型和 backend 的原始分数、校准等探索价值 | 需逐版本核对服务协议、依赖、模型许可和可复现性；项目明确不是 Jev 官方实现/权重复刻 | **研究候选，不作为第一条生产集成路线** |
| **AgentJev** | Qwen3-0.6B 骨干加专用候选决策头；提供本机服务，但 HTTP 是自定义 `/api/evaluate`，非 `/v1/systemone` | 可写协议转换 adapter 后当 sidecar；权重不能当普通 causal GGUF 模型直接加载 | 针对 agent/coding decision 训练，正好能作为 fast-router 路由任务的专用模型候选 | 需要 Python/PyTorch；上游说明 checkpoint 不是普通 causal LM，服务默认使用 CUDA；需自测设备兼容、映射概率字段和 checkpoint 许可 | **研究性可集成，优先做 adapter + 离线样本评估** |

另有 NanoJev 等专用决策模型属于“模型/权重候选”；是否可接入取决于推理后端能否加载其决策头并拿到相同问题契约所需的 logits/概率，再通过 fast-router 的任务集验收。不能只凭模型叫“Jev”就认为是可替换服务。

## fast-router 当前状态与兼容边界

### 已有的集成接缝

* `router/scorer.go` 定义 `Backend` / `ExtendedBackend`，目前 scorer 可在 ExtendedBackend 下采用逐候选 yes/no，其他 backend 走单次候选代码 logits。
* `router/zig_backend.go` 和 `zig/frwrapper.zig` 已实现对应的 yes/no、token ID 与 chat-template FFI 符号。
* `zig/libfrwrapper.dylib` 在本工作区可见，但构建产物不等于发行包可用；`SOP/build.md` 要求目标平台提供匹配的 libllama/libggml，共同验证 Zig wrapper、GGUF 和 ABI。
* `WIKI/implementation-status.md` 将真实模型和动态库路径标记为待实测。此次未运行模型级 benchmark，也未更改业务实现。

### 不要混淆的三种“兼容”

1. **Wire/API 兼容**：请求和响应可以按 `/v1/systemone` 交换。llm2jev、Laya、local-jev、jev-rs、openjev-sglang 都公开了这类服务能力；接入仍需写 Go HTTP client、管理 timeout/error、model/base URL/auth，并核对请求结构。
2. **算法兼容**：目前并不相同。老版 LLM2Jev 文档讲逐选项 yes/no；当前 `tic-top/llm2jev` README 转为一次 prefill 读取 A/B/C 等选项 label logits。fast-router 当前 yes/no 分支读取 yes token 的 raw logit，再对选项做 softmax；它既不是 yes/no 对内归一化，也不是当前 llm2jev 的候选标签路径。
3. **模型/运行时兼容**：GGUF 能加载、API 能通，都不能证明准确率、置信度或 latency 可替换。聊天模板、thinking 开关、标签是否单 token、logits 取位、温度校准及候选顺序都可能改变路由决策。

### 本仓库 yes/no 路径待核实点

以下结论直接来自当前源代码，属于需补测试/实测的问题，而不是对某模型效果的判断：

* `scoreChoiceYesNo` 构造 prompt 时没有使用 `Question.Instructions`，与 Jev 请求语义不完整。
* Zig `fr_score_yesno` 仅读取 `yes` token 的 logit；没有同时读取 `no` 并算 `P(yes | yes,no)`，故不能称为逐项二元归一化概率。
* 逐候选结果再经候选维度 softmax，产生的数值是相对分布；不能未经校准就视为可靠置信度。
* `fr_apply_template` 当前 `h` 被忽略，并把 `null` template 传入 llama.cpp；模型模板实际解析、JSON 转义和各目标平台行为必须单测及真机验证。
* 请求 map 的 iteration/order、错误回退、token 统计、超长输入以及 context 清理并发安全都应纳入集成验收。

以上代表“有集成路径”，并不代表现有 yes/no 实现已达到可生产替换标准。

## 建议的集成决策

### 首选：先做可逆的 HTTP sidecar 比较

让当前原生 scorer 保持为 `native`，增加配置可选 `systemone_http`；Go 侧只负责发统一请求、解析标准答案和映射错误，不把上游的模型依赖并入主进程。第一轮比较：

1. **llm2jev**：验证最新单次 prefill/标签 logits 路线。
2. **local-jev**：最快建立可用的服务基线，含轻量 NLI 与 LLM 两类 backend。
3. **Laya**：验证专用非自回归 decision model 路线。
4. **jev-rs**：若优先保留 GGUF/llama.cpp，再测它的 general scorer 和 HTTP wire server。
5. **AgentJev**：作为 coding-agent 专项模型，用 adapter 接自定义 HTTP 契约对照。

选择成功维度：路由任务集准确率/宏平均 F1、概率 Brier/ECE、P50/P95 延迟、常驻内存/磁盘、冷启动、并发安全、安装复杂度。不能只比较项目 README 的 JevBench 百分比。

主要失败方式：wire schema 细节不符导致 API 失败；提示词或 tokenizer 差异造成任务精度跌落；置信度未校准造成错误阈值/路由；外部服务超时导致 gateway 不可用；依赖/模型许可与打包策略不匹配。

可逆性：以配置开关选择 scorer，默认保持当前路径；将 HTTP scorer 限制为可选 sidecar，失败时明确返回/按策略回退；同一冻结样本集并行记录答案和耗时。先只 shadow 测量，不立即改变生产路由。

### 为什么不先把某个实现整体并入 fast-router

* 当前 Go/Zig 路径已具备模型内嵌决策的雏形；另引入 Python/Rust 全家桶会增加打包、升级和故障域。
* System One wire format 只是传输契约，不是统一的模型精度或置信度语义。
* 先以外部服务做基准对照，可以区分问题属于 prompt/算法、模型质量、底层 runtime 还是服务集成。
* 若 sidecar 明显胜出，再决定重写 scorer、保留 sidecar 或替换发行架构；这些选择均可回退。

## 局限

* 本次只核验了候选项目的公开 README/文档和本仓库当前接缝；没有安装依赖、审核完整代码、检查所有传递依赖许可或执行真实模型测试。
* 上游仓库快速变化，表格结论仅对应 2026-09-26 可见信息；发布前需锁定 commit/tag、依赖、模型版本及权重许可。
* 项目自报 benchmark 的机器、数据、prompt、候选数量和校准方法并不统一，不能据此排名。
* “可以集成”指存在合理可实施的本地进程/API/算法接缝，不等于已在 fast-router 验收通过。

## 结论

确实不止一种开源实现，而且有些已经提供兼容的 System One 服务。fast-router **不缺集成切口**：现有 native backend 可继续发展，HTTP sidecar 则是低耦合的比较方式。当前最有价值的下一步不是凭 README 选“最强 Jev”，而是在同一批 router 样本上比较 llm2jev、Laya、local-jev（必要时 jev-rs）与 native scorer 的质量、校准、延迟和运维成本，再据证据选择。

本轮复核后，**llm2jev 是最适合首先接入的协议基线**：它与 fast-router 的 System One 请求契约一致、MIT、依赖边界清楚，而且可以先使用 HTTP，不必立即改动 Go/Zig ABI。**jev-rs 是最适合第二条验证的 GGUF 基线**：它可以把任意 GGUF 经 llama-server 暴露为 System One 服务，同时还支持把 Laya/AgentJev 这类 whole-request backend 转成统一入口。Laya 适合质量/速度对照，local-jev 适合本地离线基线；AgentJev 则应单列为专用模型 adapter，不应伪装成标准 Jev backend。

特别更正：已有 [LLM2Jev 研究笔记](LLM2Jev-研究笔记.md) 对旧方法的分析仍可作历史背景，但其“LLM2Jev 当前方法”及“仓库缺少 yes/no/template 实现”的描述已不再代表截至本次核查的上游和工作树。应以本报告的时间点与当前源码为准。

## Citations

1. [llm2jev 当前上游 README](https://github.com/tic-top/llm2jev)
2. [旧版 LLM2Jev yes/no 方法说明](https://github.com/Yinsongxu/LLM2Jev/blob/main/docs/request-to-model_zh.md)
3. [Laya 上游](https://github.com/LeonaDavinci/laya-system-one)
4. [local-jev 上游](https://github.com/amithgc/local-jev)
5. [jev-rs 上游](https://github.com/yijunyu/jev-rs)
6. [openjev-sglang 上游](https://github.com/ekzhang/openjev-sglang)
7. [SemIf 上游](https://github.com/TheoLeeCJ/SemIf-OpenJev)
8. [AgentJev 上游](https://github.com/malevrigns/agent-jev)
