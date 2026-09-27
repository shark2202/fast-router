---
type: Architecture
title: fast-router 当前架构与数据流
description: C1-C6 路由链、System One Engine seam、Go/Zig/Python 边界和协议转换路径。
tags: [architecture, routing, gateway, ffi, v1.0]
timestamp: 2026-09-26T18:00:00+08:00
---

# 架构与数据流

> v1.0 基线：架构/引擎/方法/模型四层全部锁定。详细决策见 [交付报告 v1.0](../docs/fast-router-交付报告-v1.0.md)。

## 分层

```text
client
  │ OpenAI /v1/chat/completions 或 Anthropic /v1/messages
  ▼
Go Gateway (C1/C6)
  ├─ C2 DetectTurn  ──► session 路由缓存（继承上一轮路由）
  ├─ strong model hint / hint-only fallback
  ├─ C3 Scorer -> SystemOneEngine -> Backend
  │              ├─ NativeSystemOneEngine（in-process Scorer）
  │              └─ HTTPSystemOneClient（外部 /v1/systemone sidecar）
  │   Backend（ExtendedBackend）
  │     ├─ ChoiceScore（单 token）
  │     ├─ ScoreYesNo + ApplyChatTemplate（per-candidate yes/no，v1.0 主路径）
  │     ├─ GetTokenID（解析 yes token id）
  │     └─ Zig wrapper -> llama.cpp -> GGUF
  ├─ C4 Match(capability vector)
  ├─ C5 Select(measured + cost)
  ├─ schema.ConvertRequest / ConvertResponse / SSEConverter
  └─ upstream HTTP
```

## 路由链

1. `Gateway.ServeHTTP` 根据路径分派 `/v1/chat/completions`、`/v1/messages`、`/v1/models`。
2. `route` 先调用 `DetectTurn` 得到 `TurnKind`：
   * `NewTaskTurn` → 重新进入 C3-C5；
   * `ContinuationTurn`（OpenAI tool 结果 / Anthropic tool_result）→ 命中 session 缓存，复用上一轮的 upstream + model + protocol；
   * `AmbiguousTurn` → 当前仍按新任务处理。
3. 没有 engine / engine 不可用时走 hint-only：直接返回默认 upstream 和客户端 model。
4. 有 engine 时从最后一个 user message 提取 state，构造 `System1Request`：
   * 单 token 路径：`scoreChoice` 用候选编码 A-Z + AA-…，调用 `Backend.ChoiceScore` 取 logits 后 softmax；
   * yes/no 路径：`scoreChoiceYesNo` 用 chat template 把每个候选包成独立 prompt，调用 `ExtendedBackend.ScoreYesNo` 取 "yes" logit 后 softmax（v1.0 主路径）。
5. `Match` 用任务类型 capability requirement 过滤模型。
6. `Select` 在幸存模型中按 measured pass rate 与成本评分；冷启动无 measured 时按成本降级。
7. 选择 upstream 后重写请求中的 model 字段，必要时经 `schema.ConvertRequest` 转换协议，再转发响应。

## 任务轮语义

OpenAI 的 `tool` 结果和 Anthropic 的 `tool_result` block 被判为 `ContinuationTurn`；新的 user 文本被判为 `NewTaskTurn`；assistant 结尾或未知协议为 `AmbiguousTurn`。

`Gateway.sessions` 缓存 `(upstream, modelID, protocol)` 三元组：

* `NewTaskTurn` 命中缓存后覆盖并触发新路由；
* `ContinuationTurn` 直接复用，跳过 Jev scorer；
* `AmbiguousTurn` 走新任务路径，避免在不确定轮次继承错误路由。

`SessionInheritance` 与 `NewTaskTurnNotInherited` 两个测试覆盖该行为（见 `router/session_test.go`）。

## System One Engine seam

`SystemOneEngine` 是 v1.0 引入的高层决策边界，与 `Backend` 分离：

```go
type SystemOneEngine interface {
    Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}
```

* `NativeSystemOneEngine`：把现有 `Scorer` 包装成 engine 接口；`Backend` 仍是可插拔的（ExtendedBackend 自动走 yes/no）。
* `HTTPSystemOneClient`：把请求 POST 到外部 `/v1/systemone` 端点（路径自动归一化，避免重复 `/v1/systemone/v1/systemone`），可指向 jev-rs / Laya / llm2jev 等 sidecar。

`Gateway.NewGateway` 接受 `SystemOneEngine`；当前主入口使用 native adapter。HTTP client 通过 `NewHTTPSystemOneClient(endpoint, token, httpClient)` 构造，覆盖 2xx、错误码、JSON 解码、context 取消和空 Bearer 等场景。

## C3 scorer 接口

```go
type Backend interface {
    ChoiceScore(prompt string, candidateCodes []string) (logits []float64, usage Usage, err error)
}

type ExtendedBackend interface {
    Backend
    ScoreYesNo(prompts []string, yesTokenID int32) ([]float64, Usage, error)
    GetTokenID(word string) (int32, error)
    ApplyChatTemplate(messages string, addAssistant bool) (string, error)
}
```

* Go 层负责 API 结构、确定性候选排序、候选数量上限 255、softmax、最高概率选择。
* Backend 负责 tokenization、模型 forward、yes/no logit、apply_chat_template。
* `Scorer` 自动检测 ExtendedBackend：满足时走 yes/no 路径，否则回退到单 token。

## Zig 边界

Go 不直接绑定 llama.cpp 的复杂结构体。`frwrapper.zig` 通过 Zig `@cImport` 处理 `llama_model`、`llama_context`、batch 和参数结构，导出 6 个 C ABI：

| 符号 | 作用 |
|---|---|
| `fr_load` | 加载 GGUF，返回 handle |
| `fr_score` | 单 token 候选打分 |
| `fr_score_yesno` | 候选 yes/no 打分（per-candidate 方法） |
| `fr_get_token_id` | 解析单词对应的 token id（如 `"yes"`) |
| `fr_apply_template` | 调 `llama_chat_apply_template`，按模型 chat template 渲染 |
| `fr_free` | 释放 handle |

* Unix 路径：`purego` dlopen `libfrwrapper.{so,dylib}`。
* Windows 路径：`syscall.NewLazyDLL`（`zig_backend_windows.go`），同样 CGO_ENABLED=0。

`fr_score` 的逻辑是：tokenize prompt → 验证每个 candidate 是单 token → 清空 KV memory → decode → 读取末 token logits → 写入候选分数。
`fr_score_yesno` 的逻辑是：tokenize 单条候选 prompt → 取末 token 在 `yesTokenID` 上的 logit。

## 协议转换

`router/schema` 包含 `schema.go`、`request_response.go`、`sse.go`、`tool_calls.go`，支持：

* 请求：system 字段位置、message 内容和工具格式转换。
* 响应：OpenAI `choices[].message` 与 Anthropic content blocks、finish/stop reason 转换。
* SSE：OpenAI chunk 与 Anthropic 多事件状态机转换。
* 工具调用：`tool_calls ↔ tool_use` + `tool_result`，包括响应端 `tool_calls→tool_use` 的修复（见 `4e4bee6`）。

网关根据 upstream protocol 决定 `/v1/chat/completions` 或 `/v1/messages`，并设置 Bearer 或 `x-api-key` 认证头。

## 配置和热加载

`Config` 包含 listen、model 路径、upstreams 和 registry。启动时不存在配置文件会写出默认模板；admin API 可以保存配置、更新 upstream，并热加载 Gateway。registry 保存后也会更新 Gateway 的模型列表。

## 当前架构边界

* hint-only 模式 + 协议转换 + session 继承已在真实 upstream 上端到端验证（curl 验证）。
* Jev 智能路由在真实 Ornith-1.5-9B 上跑出 P1=73.3%，但 P2=77s（CPU）需 GPU/MLX 才实用。
* Zig backend 在 macOS/Linux 上真实运行通过；Windows 仅编译过，未在 Windows 上实测。
* 设计文档提到 C7-C9 判据回流和 prefix-reuse 优化；当前代码未实现 verdict ledger、measured matrix 持久化或 KV cache 共享。
* System One Engine 的 HTTP adapter 已可指向外部 sidecar，但 jev-rs / Laya / llm2jev 真实接入未做端到端验收。