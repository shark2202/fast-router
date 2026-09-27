---
type: Module Guide
title: Go Router 模块
description: fast-router 当前主实现的 Go 包、接口和测试边界。
tags: [go, router, modules, v1.0]
timestamp: 2026-09-26T18:00:00+08:00
---

# Go Router 模块

## 包职责

`router` 包把网关、路由决策、配置、管理 API、模型下载、协议转换和 System One Engine seam 放在同一 Go module 中。当前没有独立的持久化层或服务层包；配置和 registry 以内存结构 + JSON 文件为主。

## 核心接口

### C3 Backend

```go
type Backend interface {
    ChoiceScore(prompt string, candidateCodes []string) (logits []float64, usage Usage, err error)
}

// ExtendedBackend = 单 token + per-candidate yes/no + chat template（v1.0 主路径）
type ExtendedBackend interface {
    Backend
    ScoreYesNo(prompts []string, yesTokenID int32) ([]float64, Usage, error)
    GetTokenID(word string) (int32, error)
    ApplyChatTemplate(messages string, addAssistant bool) (string, error)
}
```

`Scorer` 自动检测 ExtendedBackend；满足时走 `scoreChoiceYesNo`（per-candidate），否则回退到 `scoreChoice`（单 token）。

### System One Engine

```go
type SystemOneEngine interface {
    Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}
```

* `NativeSystemOneEngine`：把 `Scorer` 包装成 engine 接口（默认入口）。
* `HTTPSystemOneClient`：把请求 POST 到外部 `/v1/systemone` sidecar（路径归一化、错误码、context 取消、空 Bearer 均有测试）。

## 核心数据

* `TaskType`：任务代码、名称、描述和 capability requirement（10 个种子，A-J）。
* `ModelEntry`：model id、upstream、上下文窗口、成本、能力向量和 measured 数据。
* `Config`：listen、模型文件/动态库路径、upstreams、registry。
* `Upstream`：名称、base URL、API key、协议。
* `Answer`：choice、概率分布和 confidence。
* `cachedRoute`：`Gateway` 的 session 路由缓存（upstream/model/protocol）。

## HTTP 接口

| 方法 | 路径 | 职责 |
|---|---|---|
| POST | `/v1/chat/completions` | OpenAI 请求入口 |
| POST | `/v1/messages` | Anthropic 请求入口 |
| GET | `/v1/models` | 返回当前简化模型列表 |
| GET | `/admin` | 管理页面 |
| GET/POST | `/api/config` | 读取/保存配置 |
| GET/POST | `/api/registry` | 读取/保存模型 registry |
| GET | `/api/models` | 列出可下载模型 |
| POST | `/api/models/download` | 启动异步模型下载 |
| GET | `/api/models/download/status` | 查看下载状态 |
| POST | `/api/upstream/test` | 测试 upstream `/models` |

## 测试边界

* `turn_detector_test.go`：两个协议的用户轮、工具结果、ambiguous 和未知协议（11）。
* `scorer_test.go`：候选选择、255 上限、空 criteria、softmax、yes/no 与 ExtendedBackend 路径（8）。
* `matcher_selector_test.go`：能力挡死、blocked reason、冷启动和 measured 选择（11）。
* `session_test.go`：session 继承与 NewTaskTurn 不继承（2）。
* `systemone_engine_test.go`：NativeSystemOneEngine 透传到 Scorer + context 取消（2）。
* `systemone_http_test.go`：HTTP client 路径、错误码、JSON 解码、context 取消、空 Bearer、端点归一化（7）。
* `gateway_engine_test.go`：Gateway 通过 SystemOneEngine 装配并完成路由（1）。
* `schema/schema_test.go`：system、response、SSE、同协议 passthrough、tool_calls↔tool_use（13）。

合计 60 个 Go 测试通过（v1.0 锁定时为 39 个测试；后续 commits 补齐了 systemone_engine / systemone_http / session / gateway_engine 等）。

## 注意事项

* `Gateway.sessions` 是内存缓存，进程重启即失效；session 路由继承是设计目标与单元测试都已覆盖的实现，不应再被描述为"未闭环"。
* `Backend` 与 `SystemOneEngine` 是两个独立 seam：换引擎不等于换 backend，反之亦然。
* `Admin` 是本地原型管理面，代码中未见认证/授权层，不应直接暴露公网。