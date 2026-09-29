---
type: Research Note
title: Kev System One 决策模型深度研究
description: 研究 Kev 的 System One 模型、评估证据、CPU/Intel Mac 适配和 fast-router 集成边界。
source: jaredpalmer/kev GitHub、模型卡、服务与模型源码、fast-router SystemOne 接口
timestamp: 2026-09-29
---

# Kev System One 决策模型深度研究

## 结论先行

**Kev 是协议和任务形态上最贴近 fast-router System One 的开源决策引擎候选，但不是可直接替换现有 Jev 算法的已验证模型。** 它直接实现 `POST /v1/systemone`，用 Qwen backbone + LoRA + pointer head 对 `noul`、`choice`、`score` 问题输出结构化概率；fast-router 已有转发完整 System One 请求的 HTTP engine seam，因此接入 POC 的工程成本较低。

但对当前项目的关键限制是：公开准确率来自通用/政策/蕴含等评测，并非 fast-router 的 A–J task-code 数据；Intel x64 CPU 没有发布的延迟与质量结果；Kev-27B 明确需要 80 GB GPU，Kev 在 Apple M5 的数字不能外推到本项目 Intel i7-9750H。**建议先将 Kev-0.8B 或 4B 做本地 CPU/独立 GPU sidecar shadow 实验，先验证 wire compatibility，再用独立标注的 fast-router 数据评估质量和延迟；未过门前不替换生产引擎。**

## 来源与方法

### 来源

研究固定在 Kev `main` 快照 `fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c`（提交时间 2026-09-28；核对时与 `refs/heads/main` 一致）。主要材料：

* [仓库 README](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/README.md)：模型表、API、预期效果、训练与服务性能。
* [模型实现](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/kev/model.py)：编码、隔离、pointer head、打分接口。
* [API 实现](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/kev/api.py)：协议数据结构、输入渲染、输出概率。
* [HTTP 服务](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/kev/serve.py)：端点、批处理、前缀缓存、鉴权和并发模型。
* [Kev-27B 模型卡](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/docs/model-cards/kev-27b.md)、[研究流程](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/docs/autoresearch.md) 与 [计划记录](https://github.com/jaredpalmer/kev/blob/fdfdfd2c7a98225715a9cc8f4d9e1ef29ebbe79c/PLAN.md)。
* fast-router 本地 `router/systemone_engine.go`、`router/systemone_http.go`、`router/scorer.go` 与既有 CPU / Jev 评估记录。

### 方法

1. 固定并核对上游 `main` SHA，避免将可变分支内容误作稳定事实。
2. 分别检查 README 公开结果、模型/服务代码与模型卡，区分“模型指标”“API 兼容”“设备能力”和“本机验证”。
3. 对照 fast-router 的请求/响应类型及 `HTTPSystemOneClient`，评估最短接入路径。
4. 依照项目决策原则，分别列出成功维度、主要死法、回滚方式。

本次为静态源码与文档审查；未下载 Kev 权重、启动服务或在 fast-router 数据集上运行评分。

## 发现

### 1. 它是决策模型，不是让生成文本再解析的普通聊天模型

Kev 的核心路径是 Qwen backbone（不带生成词表头）+ rank-16 LoRA + 小型 pointer head。请求中的 state 被编码一次；每个问题通过 `<q> ... <opt> ... <decide>` 结构表达，pointer head 以 `<decide>` 隐状态与每个选项结束位置的隐状态计算选项 logits，再 softmax 成概率。不会生成自然语言答案。

API 可在一次请求中混合：

* `choice`：返回 argmax 选项、全量 probabilities 和 confidence；
* `noul`：返回 yes 概率；
* `score`：返回有序等级上的期望分数、概率和 legend。

问题彼此隔离；Qwen3.5/3.8 的 Gated DeltaNet 路径将各问题作为独立 row，并复用 state prefix。Kev 公开实现也带 option permutation 和 separate-question 检查端点。这个架构比“用通用 LLM 生成 A–J 再解析”更贴近 Jev/System One 的输入输出任务。

### 2. 与 fast-router 的协议集成路径近乎现成

fast-router 的 `HTTPSystemOneClient` 会将完整 `System1Request` POST 到兼容的 `/v1/systemone`，并把响应解码为 `System1Response`。Kev 接收同名字段 `state/model/questions`，返回 `model/answers/usage`；Choice answer 包含 `type/choice/probabilities/confidence`，与 fast-router 当前结构相符。

因此初次 POC 不必把 Kev 模型嵌进 Go/Zig 进程：可以把 Kev 当 HTTP sidecar，利用现有 engine seam 切换后端。仍须用真实序列化请求做端到端 contract test（字段可选性、空 criteria、最大候选数、上下文超限、错误码、超时/取消、鉴权）；协议相似不能代替测试。

任务适配不是零成本：fast-router 的路由 task codes 是 A–J，并依赖本项目自己的任务定义与判据；Kev 的公开分类任务和已有训练语料并不等于这些路由任务。可将候选 code 映射成 `choice.criteria` 的 key，将各类判据放入 description/instructions，但 prompt、类别先验、标签定义和候选数量都需要按 fast-router 数据校准。现有本地路由代码也包含候选映射与 yes/no scoring 等专用逻辑，切换到 Kev pointer head 会改变算法，必须重新测 P1，不能只测 API 返回 200。

### 3. 已发布的效果不错，但不是本项目效果的证据

截至固定快照，README 给出的新来源 dev/test accuracy 为：Kev-0.8B `0.648/0.697`、Kev-4B `0.817/0.838`、Kev-9B `0.822/0.852`、Kev-27B `0.848/0.896`；训练来源数据的测试准确率约 `0.838–0.870`。Kev-27B 新来源测试 Brier 为 `0.164`。README 明确说明 Jev 只跑了 dev，Jev 与 Kev 的训练数据不可控，因此不是受控架构比较。

模型卡同时给出风险边界：在 5% 错误预算下，Kev 可自动化比例约 `0.45–0.57`，低于 Jev 的 `0.70`；Kev-9B 的高置信错误率约 `4.0%`，接近 Jev 的 `3.7%`，但 calibrated confidence 并不等于准确率。知识型问题上 Kev 明显落后。以上指标不回答 fast-router 的 A–J 分类质量。

训练值得关注的工程经验是：从发布的 Kev checkpoint 用 `--init_from` 微调，而不是从基础 Qwen 重训；训练数据不足时改进可能落在噪声内；微调增益主要是任务内增益，应保留独立 held-out 集。README 报告了 1,050 条生成样本的支持任务和 5,219 条金融投诉样本的案例，但这些数字不能直接推导本项目所需样本量或准确率。

### 4. CPU 可运行不等于 Intel CPU 上够快

Kev 服务在 CUDA/ROCm、Apple Silicon MLX 与通用 PyTorch 路径间选 backend；默认设备探测最终可落到 CPU。然而 README 的公开性能表是 GPU 数据，Apple Mac 表则是 **M5、32 GB、MLX**：Kev-0.8B 五问新文本 149 ms、Kev-4B 721 ms；重复文本使用缓存后分别 28 ms、136 ms。没有 Intel x64 CPU benchmark，也没有 Kev-9B/27B Intel Mac 结果。

当前项目机器为 Intel Core i7-9750H。不能把 M5/MLX 性能推算成它的 CPU 延迟。Kev 模型运行在 CPU 上是技术可行路径，不是符合 fast-router 延迟目标的证据；长状态还会增加成本。README 的训练状态长度最高 384 tokens，而 serving 接受 65,536 tokens，这种服务上限并不意味着长状态准确率已充分训练验证。

此外，Kev 用 Python、PyTorch、Transformers/PEFT 等依赖，CPU eager attention 使用自定义 mask 路径；它不是可直接编译进 fast-router 的小型无依赖库。作为隔离的 GPU/CPU sidecar 更自然。Kev-27B 需要约 55 GB bf16 权重、含 serving buffers 约 66 GB，README 明确其目标是 80 GB GPU，没有 Mac 路径；它不是本机候选。

### 5. 研究纪律和服务安全值得借鉴，但也要审查

上游将 train/dev/test、锁定评测、数据清单、训练轮次注册、成本预算和 test 单次读取写入研究流程，并记录例外与模型卡；这种“先注册实验再看结果”的证据纪律适用于 fast-router 的模型比较。

服务默认只 bind loopback，`KEV_API_KEY` 未设置时 `/v1/*` 不鉴权；部署到远端时必须设置 key 并置于受控网络/反向代理后。Kev sidecar 支持批请求与 state-prefix LRU cache，但缓存会占模型内存；其 `latency_ms` 是模型批次时间，不含端到端网络延迟/排队等待，fast-router 必须另外记录实际调用耗时。

## 评估与决策记录

| 维度 | 预期成功标准 | 最可能死法 | 可逆性 / 收敛办法 |
|---|---|---|---|
| 接口接入 | fast-router HTTP engine 通过真实 Kev contract test；请求/响应及错误路径一致 | 字段形状、模型别名或超限语义不匹配 | sidecar 可独立关闭，回切 native engine |
| 决策质量 | 在去重、独立标注且冻结的路由集上，P1/macro-F1 与 baseline 对比，并报告置信区间/混淆矩阵 | 把通用 benchmark 或 `ok_rate` 当路由真值；A–J 数据分布不同 | shadow 只记录不执行，失败时不影响路由 |
| 延迟 | 在 i7-9750H 上测冷/热、短/长 state 的 P50/P95 和全链路延迟 | 用 M5/GPU 表格外推 Intel CPU；排队延迟漏报 | 超预算时恢复原引擎或仅在 shadow 使用 |
| 置信度使用 | 在本项目 held-out labels 上做 reliability / selective risk 曲线 | 将 probability/confidence 当成已保证的准确率 | 默认只输出观测值，不用于自动放行；逐步校准 |
| 部署安全 | 远端服务强制 API key 和网络访问控制 | Kev 默认服务无 key，被暴露成开放决策端点 | loopback 部署、密钥、代理与快速下线 |

## 对 fast-router 的建议

### POC 顺序

1. **协议验证**：锁定 Kev 快照/模型权重 revision，启动 sidecar；使用现有 HTTP engine 发真实请求，核对所有 response/error 行为。
2. **shadow 质量验证**：把同一请求同时送 native baseline 和 Kev，不让 Kev 的结果影响实际选模；用人工独立真值，不把上游 `ok_rate` 当任务类别标签。
3. **硬件分层**：分别测当前 Intel CPU（至少 Kev-0.8B）和可用 GPU 环境；记录进程 RSS、模型加载时间、state 长度、候选数、P50/P95、超时率与 P1。
4. **小规模微调**：若 zero-shot 的错误有稳定类别结构，再用本项目标注的 train/dev split 从发布 checkpoint warm-start 微调；冻结 test 不参加选参。
5. **门控部署**：只在质量与延迟都满足现有门槛、且 shadow 回放一致后，进行小比例可回滚 A/B。

### 候选优先级

* **Kev-0.8B**：适合先回答“紧凑 decision model 在本机 CPU 是否可用”。当前无 Intel 性能证据，且公开新来源测试准确率相对低；定位为实验候选而非默认生产模型。
* **Kev-4B**：公开质量高于 0.8B，适合 GPU sidecar / Apple Silicon 对照；本机 CPU 延迟与内存均未知。
* **Kev-9B**：不能因与现有 9B 路由基线参数量接近就认为效果等价；只有专门训练/测试后才有比较意义。
* **Kev-27B**：质量指标高，但 80 GB GPU 部署要求与当前 CPU 方向不匹配；仅适用于独立 GPU 研究。

## 局限

1. 这是对固定上游快照的静态审查；模型权重、运行时结果、Intel CPU 表现和 fast-router P1 均未在本次运行验证。
2. 上游公开分数由其数据、切分、训练流程和评测定义决定；Jev 对照并非受控试验。
3. Kev 仓库资料是项目作者的自报结果，需复验才能作为 fast-router 决策证据。
4. 本机 System One schema 表示当前 Choice 使用 `map[string]string` criteria；Kev 接受字符串/结构化 JSON content。JSON wire 结构看似相容，Go 类型边界仍需按真实请求验证。
5. 任务标签须独立于运行成功率和被路由模型输出，避免选择偏差与反馈回路。

## 结论

**Kev 值得集成验证，首先作为可替换 HTTP System One sidecar，而不是直接作为 CPU 生产替代品。** 它的关键优势是“正确的问题形式 + 现成协议 + 可微调 + 概率输出”，而不是已经证明在当前 Intel Mac 上低延迟或在 fast-router task-code 上准确。

按“先定义看什么、再定义怎么死、最后决定能否试”：成功看独立路由标签上的 P1、校准与端到端 P95；最容易失败于数据域偏移和 Intel CPU 延迟；通过 shadow 与 sidecar 开关可以低成本回退。因此目前判定为 **可做小型 POC / candidate，不可替换已锁定生产基线**。
