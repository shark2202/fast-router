---
type: Architecture Note
title: System One 引擎抽象与可替换性
description: 说明 System One 的协议、引擎接口和模型 runtime 三层关系，并核对 fast-router 当前实现是否真正支持可替换引擎。
timestamp: 2026-09-26T00:00:00-04:00
tags: [system-one, architecture, engine, protocol, replaceability]
---

# System One 引擎抽象与可替换性

## 结论先行

**目标架构应该是：System One 是稳定的决策接口/能力契约，具体引擎和模型实现可替换。**

但当前 fast-router 还没有完全达到这个状态：

* **协议层已经独立**：`System1Request` / `System1Response` 对齐 `POST /v1/systemone` 的请求响应形状。
* **低层 Backend 已可替换**：`Scorer` 依赖 `Backend`，native Zig/libllama 是其中一种实现。
* **完整 System One 引擎还没有独立成接口**：Gateway 直接持有具体 `*Scorer`；HTTP sidecar、jev-rs、Laya、lev 不能直接作为当前 `Backend` 替换。
* **当前实现不是完整 System One**：`Scorer.Score` 当前只接受 `choice`，虽然类型中声明了 `noul`、`score`，但没有完成这些 answer 类型。

因此，准确说法是：

> 当前项目已经有“System One 协议 + 可替换低层推理 Backend”的雏形，但还没有“可插拔的完整 System One Engine”。

## 来源

### 项目源代码

* `router/scorer.go`
* `router/zig_backend.go`
* `router/zig_backend_windows.go`
* `router/gateway.go`
* `docs/Jev-开源实现集成评估.md`

### 社区实现

* [llm2jev](https://github.com/tic-top/llm2jev)：明确把自己定位为 documented System One wire format 的独立实现，并支持多个 inference backend。
* [jev-rs](https://github.com/yijunyu/jev-rs)：将 `/v1/systemone` 作为 wire contract，同时挂接 llama-server、OpenAI-compatible、Laya 和 AgentJev backend。
* [system-one](https://github.com/mpuig/system-one)：明确区分当前实现、API compatibility subset 和计划能力。
* [verdict](https://github.com/khimaros/verdict)：以相同 HTTP wire shape 把 llama-server 暴露成 Jev-compatible endpoint。

## 方法

1. 读取 fast-router 的 request/response 类型、Gateway 调用路径、Scorer 和 Backend 接口。
2. 对比多个社区实现如何处理 `/v1/systemone`、模型 backend 和 runtime。
3. 将“协议兼容”“引擎接口兼容”“模型/runtime 兼容”拆成三个独立判断。
4. 对当前代码中的实现能力和目标设计分别标记，避免把类型声明当成完整功能。

## 三层模型

```text
┌─────────────────────────────────────────────────────────┐
│  Layer 1: System One Protocol / Capability Contract     │
│  POST /v1/systemone                                     │
│  Request: state + typed questions                       │
│  Response: typed answers + probabilities + usage        │
└───────────────────────┬─────────────────────────────────┘
                        │ stable boundary
┌───────────────────────▼─────────────────────────────────┐
│  Layer 2: SystemOneEngine                               │
│  Evaluate(ctx, request) -> response                     │
│  Native / Remote / Shadow / Ensemble                    │
└───────────────────────┬─────────────────────────────────┘
                        │ implementation seam
┌───────────────────────▼─────────────────────────────────┐
│  Layer 3: Model + Inference Runtime                    │
│  llama.cpp / MLX / PyTorch / Core ML / Rust server      │
│  GGUF / encoder checkpoint / decision head              │
└─────────────────────────────────────────────────────────┘
```

### Layer 1：协议/能力契约

这一层应该固定的是：

* endpoint 和 HTTP method；
* request 的 `state`、`model`、`questions`；
* typed question 的类型和 criteria 结构；
* response 中 question id 与 answer 的对应关系；
* 概率分布、confidence、usage 等字段的基本语义；
* 错误状态和基础限制。

这一层不应该固定：

* 是单 token、yes/no、候选 label 还是 encoder head；
* 是一次 prefill 还是多次 forward；
* 是否使用 KV cache、prefix cache 或 batch；
* 使用什么模型权重、量化格式和硬件；
* 引擎是进程内库、HTTP 服务还是远程服务。

多个社区项目能互换的原因，就是它们共享的是这一层的 wire contract，而不是同一套推理实现。llm2jev 和 jev-rs 都明确把 `/v1/systemone` 作为可替换的协议边界。citeturn0search0turn0search1

### Layer 2：引擎接口

这是 fast-router 当前缺失的主要抽象。

建议形成一个高层接口：

```go
type SystemOneEngine interface {
    Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}
```

可选的能力描述：

```go
type EngineCapabilities struct {
    Types             []string // choice, noul, score
    MaxChoices        int
    SupportsStructuredState bool
    SupportsMultimodal      bool
    SupportsCalibration     bool
}
```

Gateway、路由链和业务策略只依赖 `SystemOneEngine`，不依赖：

* `llama_model`；
* tokenizer；
* candidate token；
* chat template；
* Python/Rust sidecar；
* 某个具体模型名。

### Layer 3：模型与 runtime

这一层负责真正的判断实现：

* single-token label logits；
* per-candidate yes/no；
* all-options label logits；
* Laya/AgentJev/NanoJev decision head；
* NLI classifier；
* calibration、temperature、confidence gate；
* llama.cpp、MLX、PyTorch、Core ML、Rust server；
* GGUF 或 encoder/decision-head checkpoint。

只要 Layer 2 的输出满足 Layer 1 的语义，Layer 3 可以替换。

## fast-router 当前真实边界

### 已经可替换的部分

当前 `router/scorer.go` 中：

```go
type Backend interface {
    ChoiceScore(prompt string, candidateCodes []string) ([]float64, Usage, error)
}
```

这允许替换：

* mock backend；
* native Zig/libllama backend；
* 未来另一个“接收渲染后 prompt、返回 candidate logits”的 backend。

`ExtendedBackend` 进一步扩展 yes/no、token ID 和 chat template。

这属于 **Layer 3 内部的算法/runtime 接缝**，不是完整 System One 引擎接缝。

### 目前不能直接替换的部分

一个 jev-rs、Laya 或 llm2jev HTTP 服务不能直接实现当前 `Backend`，因为：

* `Backend` 收到的是已经构造好的字符串 prompt；
* 远程 System One 服务需要完整的 `state + questions`；
* prompt 中的 `instructions`、criteria 和 question type 可能已经丢失；
* remote service 返回的是完整 answer，而不是 candidate logits；
* remote service 可以有自己的模板、校准和概率语义。

因此，直接写一个“从 prompt 反解析成 System One request”的 Backend adapter 是错误方向。

### 当前 Gateway 仍然耦合具体实现

现有 Gateway 持有具体的 `*Scorer`，启动路径创建 native Zig backend，再创建 scorer。当前没有：

```go
type SystemOneEngine interface {
    Evaluate(context.Context, System1Request) (System1Response, error)
}
```

所以现在可以替换的是 `Scorer` 下面的 Backend，不能无侵入地替换整个 System One engine。

## “接口/spec 固定”应该如何理解

### 固定：协议语义

对于 fast-router，应把以下内容视为稳定契约：

* `POST /v1/systemone`；
* request/response JSON shape；
* choice/noul/score 的 answer 语义；
* probability 的归一化范围；
* question id 的稳定映射；
* 错误可识别性；
* 最大候选数和输入大小等明确限制。

### 不固定：实现细节

以下内容必须允许迭代：

* candidate label；
* prompt renderer；
* chat template；
* thinking 开关；
* forward 次数；
* prefix/KV cache；
* temperature 和 calibration；
* 模型权重；
* CPU/GPU/Metal/Core ML；
* 进程内/sidecar/远程部署。

### 不能假设所有实现的概率等价

同一个 JSON response shape 不代表概率语义完全一致。例如：

* 一个实现对所有候选 label 做 softmax；
* 另一个实现对每个候选做 yes/no，再跨候选归一化；
* Laya/AgentJev 可能使用 decision head；
* 不同实现的 confidence 可能是 top probability、margin 或校准后的值。

因此接口可替换，不代表结果可互换。必须用 contract test + frozen evaluation set 同时验证：

1. wire compatibility；
2. answer semantic compatibility；
3. accuracy/F1；
4. Brier/ECE；
5. latency/memory；
6. failure behavior。

## 推荐目标架构

### 目标代码结构

```text
Gateway
  └── SystemOneEngine
        ├── NativeSystemOneEngine
        │     └── Scorer
        │           └── Backend
        │                 └── Zig/libllama
        │
        ├── RemoteSystemOneEngine
        │     └── SystemOneHTTPClient
        │           └── jev-rs / laya.cpp / llm2jev / lev
        │
        └── ShadowSystemOneEngine
              ├── primary
              └── observer
```

### 迁移顺序

1. 保留当前 `Scorer` 和 native Backend，不改变默认行为。
2. 把当前 `Scorer.Score` 包装成 `NativeSystemOneEngine.Evaluate`。
3. 将 HTTP client 设计为通用 `SystemOneHTTPClient`，不要绑定 llm2jev 名称。
4. 用 `RemoteSystemOneEngine` 调用 jev-rs。
5. Gateway 依赖 `SystemOneEngine` 接口，而不是具体 `*Scorer`。
6. 增加 shadow mode，比较 native 与 remote 的 answer、概率、延迟。
7. 只有 frozen evaluation 证明收益后，才切换默认 engine。

## 最终判断

**是的，System One 在架构上应该是独立引擎，接口/spec 是稳定边界，实现可以替换、更新和优化。**

但当前 fast-router 还处于过渡形态：

* 协议类型已经存在；
* native scorer 已经存在；
* low-level Backend 已经可替换；
* high-level System One Engine 尚未抽象；
* 当前只完整支持 choice 路由，不是完整 typed System One。

所以当前正确的改造方向不是继续给某个社区项目写专用逻辑，而是把引擎边界提升一层：

```text
SystemOneRequest/Response
          ↓
SystemOneEngine interface
          ↓
Native / jev-rs / laya.cpp / lev / llm2jev
```

这会让社区实现真正成为可替换 engine，而不是只能被当作外部 benchmark。

## 局限

* 本文依据当前工作树和社区项目公开源码/README；没有宣称所有项目的完整 API 长期稳定。
* 当前 fast-router 的 `System1Request.State` 仍是 `string`，比部分社区实现支持的结构化 state 更窄。
* 当前 `Scorer` 的 `noul`、`score` 类型声明不等于已实现。
* 概率校准和 confidence 语义必须在集成测试中重新核对，不能只看 JSON 字段名。
