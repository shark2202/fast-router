---
type: Module Guide
title: Go Router 模块
description: fast-router 当前主实现的 Go 包、接口和测试边界。
tags: [go, router, modules]
timestamp: 2026-09-25T00:00:00+08:00
---

# Go Router 模块

## 包职责

`router` 包把网关、路由决策、配置、管理 API、模型下载和协议转换放在同一 Go module 中。当前没有独立的持久化层或服务层包；配置和 registry 以内存结构 + JSON 文件为主。

## 核心接口

```go
type Backend interface {
    ChoiceScore(prompt string, candidateCodes []string) (logits []float64, usage Usage, err error)
}
```

`Scorer` 依赖 Backend，不依赖具体推理引擎。这样可以用 mock 验证 choice 排序/softmax，也可以替换为 Zig/libllama 或云 Jev 实现。

## 核心数据

* `TaskType`：任务代码、名称、描述和 capability requirement。
* `ModelEntry`：model id、upstream、上下文窗口、成本、能力向量和 measured 数据。
* `Config`：listen、模型文件/动态库路径、upstreams、registry。
* `Upstream`：名称、base URL、API key、协议。
* `Answer`：choice、概率分布和 confidence。

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

* `scorer_test.go`：候选选择、255 上限、空 criteria、softmax、端到端 matcher 链。
* `turn_detector_test.go`：两个协议的用户轮、工具结果、ambiguous 和未知协议。
* `matcher_selector_test.go`：能力挡死、blocked reason、冷启动和 measured 选择。
* `schema/schema_test.go`：system、response、SSE 和同协议 passthrough。

## 注意事项

`Gateway.route` 当前没有真正的 session route store；tool loop 和 ambiguous 只被识别，尚未稳定继承之前路由。`Admin` 是本地原型管理面，代码中未见认证/授权层，不应直接暴露公网。
