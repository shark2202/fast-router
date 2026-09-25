---
type: Architecture
title: fast-router 当前架构与数据流
description: C1-C6 路由链、Go/Zig/Python 边界和协议转换路径。
tags: [architecture, routing, gateway, ffi]
timestamp: 2026-09-25T00:00:00+08:00
---

# 架构与数据流

## 分层

```text
client
  │ OpenAI /v1/chat/completions 或 Anthropic /v1/messages
  ▼
Go Gateway (C1/C6)
  ├─ C2 DetectTurn
  ├─ strong model hint / hint-only fallback
  ├─ C3 Scorer -> Backend
  │              └─ Zig wrapper -> llama.cpp -> GGUF
  ├─ C4 Match(capability vector)
  ├─ C5 Select(measured + cost)
  ├─ schema.ConvertRequest / ConvertResponse / SSEConverter
  └─ upstream HTTP
```

## 路由链

1. `Gateway.ServeHTTP` 根据路径分派 `/v1/chat/completions`、`/v1/messages`、`/v1/models`。
2. `route` 先调用 `DetectTurn`，再处理带 `/` 或 `:` 的强 model hint。
3. 没有 scorer 时使用 hint-only：直接返回默认 upstream 和客户端 model。
4. 有 scorer 时从最后一个 user message 提取 state。
5. `Scorer.Score` 将 10 个种子任务类型转为 choice criteria，排序后编码为 `A`-`J`，调用 Backend 返回 logits，再做 softmax。
6. `Match` 使用任务类型 capability requirement 过滤模型。
7. `Select` 在幸存模型中按 measured pass rate 与成本评分；冷启动无 measured 时按成本降级。
8. 选择 upstream 后重写请求中的 model 字段，必要时转换协议，再转发响应。

## 任务轮语义

OpenAI 的 `tool` 结果和 Anthropic 的 `tool_result` block 被判为工具循环；新的 user 文本被判为新任务；assistant 结尾或未知协议为 ambiguous。当前 gateway 在非 `NewTaskTurn` 情况下仍会继续进入后续逻辑，代码明确写有“尚无 session state，先重新路由”的 POC 注释，因此“继承既有路由”是设计目标而非已完成行为。

## C3 scorer 接口

`Backend` 只有一个窄接口：

```go
type Backend interface {
    ChoiceScore(prompt string, candidateCodes []string) (logits []float64, usage Usage, err error)
}
```

Go 层负责 API 结构、确定性候选排序、候选数量上限 255、softmax 和最高概率选择；Backend 负责 tokenization、模型 forward 和候选 logits。

## Zig 边界

Go 不直接绑定 llama.cpp 的复杂结构体。`frwrapper.zig` 通过 Zig `@cImport` 处理 `llama_model`、`llama_context`、batch 和参数结构，导出标量/指针为主的 C ABI。`fr_score` 的逻辑是：tokenize prompt → 验证每个 candidate 是单 token → 清空 KV memory → decode → 读取末 token logits → 写入候选分数。

## 协议转换

`router/schema` 支持：

* 请求：system 字段位置、message 内容和工具格式转换。
* 响应：OpenAI `choices[].message` 与 Anthropic content blocks、finish/stop reason 转换。
* SSE：OpenAI chunk 与 Anthropic 多事件状态机转换。
* 同协议：原样透传。

网关根据 upstream protocol 决定 `/v1/chat/completions` 或 `/v1/messages`，并设置 Bearer 或 `x-api-key` 认证头。

## 配置和热加载

`Config` 包含 listen、model 路径、upstreams 和 registry。启动时不存在配置文件会写出默认模板；admin API 可以保存配置、更新 upstream，并热加载 Gateway。registry 保存后也会更新 Gateway 的模型列表。

## 当前架构边界

* Gateway 的 HTTP 骨架存在，但完整 upstream、SSE、跨协议端到端仍需真实服务验证。
* Zig backend 代码存在，但本地动态库、llama.cpp 共享库、GGUF 模型和 ABI 组合必须在目标平台实际验证。
* 设计文档提到任务轮继承和数据库/统计回流；当前代码未实现 session route store、verdict ledger 或持久化 measured matrix。
