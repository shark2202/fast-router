---
type: Design Document
title: llm2jev HTTP sidecar adapter
description: 为 fast-router 增加独立、可测试、可逆的 System One HTTP client，第一阶段不改变 native scorer 和生产启动路径。
timestamp: 2026-09-26T00:00:00-04:00
tags: [design, jev, llm2jev, http, sidecar]
---

# llm2jev HTTP Sidecar Adapter 设计

## 1. 目标

为 fast-router 增加一个独立的 HTTP client，使 Go 代码可以调用 llm2jev 或其他兼容 `POST /v1/systemone` 的 System One 服务。

第一阶段只交付：

* 请求构造；
* HTTP 传输；
* 可选 Bearer 认证；
* context 取消与超时传递；
* 响应解析；
* 明确的 HTTP、JSON 和传输错误；
* 单元测试。

第一阶段不交付：

* 修改 native Zig/libllama scorer；
* 修改 `Gateway` 默认路由；
* 新增配置文件字段；
* 自动启动或下载 Python sidecar；
* 自动重试；
* native scorer 与 remote scorer 的自动回退；
* benchmark 或质量结论。

## 2. 背景与约束

当前 fast-router 已有：

* `router.System1Request`；
* `router.System1Response`；
* 本地 `Scorer` 和 `Backend` 抽象；
* native Zig/libllama 推理路径。

System One HTTP 服务是“整请求”接口，不是当前 `Backend` 的 `ChoiceScore(prompt, candidateCodes)` 接口。因此不能通过解析 prompt 的方式把远程服务硬塞成 `Backend`。第一阶段应保留整请求语义，提供独立 client。

项目约束：

* 只修改与本任务直接相关的文件；
* 不破坏当前 native scorer；
* 不自动写入 API key；
* 不使用破坏性 Git 命令；
* 研究和设计结论必须落盘；
* 以测试证据判断完成，不以代码存在判断完成。

## 3. 成功维度、死法与可逆性

### 3.1 成功维度

| 维度 | 验收标准 |
|---|---|
| 协议 | 对 `/v1/systemone` 发出 POST JSON，请求字段与现有 `System1Request` 一致 |
| 认证 | 配置 token 时发送 Bearer header；未配置时不发送伪造认证 |
| 可靠性 | context 取消、超时、连接失败和非 2xx 都返回可识别错误 |
| 解析 | 合法 System One 响应可还原为 `System1Response`；非法 JSON 返回解析错误 |
| 兼容性 | 现有 router 测试和 schema 测试继续通过 |
| 可维护性 | client 不依赖具体 llm2jev 实现，可调用任意兼容服务 |

### 3.2 最可能的失败方式

1. URL 规范化错误，导致 `/v1/systemone` 重复或丢失。
2. sidecar 返回非 2xx，但错误体被吞掉，排查困难。
3. HTTP 请求没有绑定调用方 context，导致 Gateway 将来接入后请求泄漏。
4. 远端响应字段扩展或类型不一致，客户端错误信息不明确。
5. 把整请求接口错误地建模为低层 logits Backend，造成 prompt 语义丢失。

### 3.3 可逆性

* client 作为独立文件和独立接口存在；
* 第一阶段不被默认启动路径引用；
* 不修改 `Scorer`、`Gateway`、配置结构；
* 删除新增 client 和测试即可回退；
* 后续若协议不合适，可改成独立 remote scorer，而不会影响 native scorer。

## 4. 方案选择

### 方案 A：独立 `SystemOneHTTPClient`（采用）

```text
System1Request
      │
      ▼
SystemOneHTTPClient.Evaluate(ctx, req)
      │
      ▼
POST {baseURL}/v1/systemone
      │
      ▼
System1Response
```

优点：

* 保留 System One 的整请求语义；
* 不需要反解析 prompt；
* 可直接服务 llm2jev、Laya、local-jev、jev-rs 等兼容服务；
* 易于用 `httptest.Server` 测试；
* 与现有 native scorer 解耦。

缺点：

* 暂时不能直接替换现有 `Scorer`；
* 后续仍需增加 remote scorer 选择和配置层。

### 方案 B：实现 `Backend` 适配器

远端服务只接受结构化 request，而当前 `Backend` 只收到已经渲染的 prompt 和候选代码。适配器必须从 prompt 重新识别 state、instructions 和 criteria，存在信息损失和脆弱解析。

不采用。

### 方案 C：本阶段直接改 Gateway

需要同时引入配置字段、启动 client、远程 scorer 选择、健康检查和失败回退，变更面大，且无法区分协议 client 问题与运行时集成问题。

不采用，留给后续独立设计。

## 5. 接口设计

### 5.1 文件

```text
router/systemone_http.go
router/systemone_http_test.go
```

### 5.2 公共接口

```go
type SystemOneClient interface {
    Evaluate(ctx context.Context, req System1Request) (System1Response, error)
}
```

### 5.3 实现结构

```go
type HTTPSystemOneClient struct {
    Endpoint string
    Token    string
    Client   *http.Client
}
```

约束：

* `Endpoint` 可接受 base URL 或完整 `/v1/systemone` URL；
* 构造函数统一规范化到最终 endpoint；
* `Client == nil` 时使用明确的默认 client；
* 不使用全局可变 client；
* Token 为空时不发送 `Authorization`；
* 请求 body 使用 `json.Marshal`，禁止手工拼接 JSON；
* response body 使用有限读取，避免异常服务返回无限大错误体。

建议构造函数：

```go
func NewHTTPSystemOneClient(endpoint string, token string, client *http.Client) (*HTTPSystemOneClient, error)
```

构造函数负责：

* 拒绝空 URL；
* 要求 URL 包含 scheme 和 host；
* 允许 `http` 和 `https`；
* 将 base URL 规范化为 `/v1/systemone`；
* 不允许把 query 或 fragment 静默拼接到 endpoint。

### 5.4 错误模型

定义可供调用方判断的远程错误：

```go
type SystemOneHTTPError struct {
    StatusCode int
    Status     string
    Body       string
}
```

行为：

* 传输层失败：包装底层 error，保留 `errors.Is/As` 能力；
* context 取消或 deadline：保留 `context.Canceled` / `context.DeadlineExceeded`；
* 非 2xx：返回 `*SystemOneHTTPError`；
* 2xx 但 JSON 无法解析：返回包含 endpoint 的解析错误；
* 不自动把远端错误转换成成功响应；
* 错误 body 截断到固定上限，例如 4 KiB。

### 5.5 请求与响应

第一版直接复用现有 `System1Request` 与 `System1Response`，避免引入重复协议类型。

当前 fast-router 路由链只使用 `choice` 问题，因此第一阶段保证现有 choice response 可以解析。对远端返回的未知 answer 字段，JSON decoder 应允许忽略未知字段，以兼容协议扩展。

后续若需要完整支持 `noul`、`score` 或结构化 state，再单独扩展协议类型，不在本阶段扩大范围。

## 6. 请求流程

```text
调用方
  │
  ├─ 构造 System1Request
  │
  ├─ Evaluate(ctx, req)
  │     │
  │     ├─ json.Marshal(req)
  │     ├─ http.NewRequestWithContext(ctx, POST, endpoint, body)
  │     ├─ Content-Type: application/json
  │     ├─ 可选 Authorization: Bearer <token>
  │     ├─ Client.Do(request)
  │     ├─ 检查 2xx
  │     └─ json.Decoder(response.Body).Decode(&response)
  │
  └─ 返回 System1Response 或明确错误
```

不做：

* 重试；
* 熔断；
* sidecar 自动发现；
* 健康检查；
* 后台 goroutine；
* response streaming。

这些能力需要接入 Gateway 后根据实际故障数据设计。

## 7. 测试设计

使用 `httptest.NewServer`，不依赖真实 Python、GPU、模型或网络。

### 必须覆盖

1. base URL 自动补齐 `/v1/systemone`；
2. 已包含 `/v1/systemone` 时不重复追加；
3. 请求方法为 POST；
4. `Content-Type` 为 `application/json`；
5. JSON body 等于预期的 `System1Request`；
6. token 存在时发送 Bearer token；
7. token 为空时不发送 Authorization；
8. 合法 200 response 正确解析；
9. 非 2xx 返回 `SystemOneHTTPError`；
10. 非法 JSON 返回解析错误；
11. server delay 能被 context deadline 取消；
12. 连接错误返回底层传输错误；
13. 无效 endpoint 在构造阶段失败。

### 不在本阶段测试

* llm2jev 模型准确率；
* prompt/template 一致性；
* GPU/CPU 性能；
* 多实现结果等价性；
* sidecar 自动拉起；
* Gateway 回退策略。

## 8. 后续接入边界

本 client 完成后，后续设计可选择：

1. 新增 `RemoteScorer`，把完整 `System1Request` 转发到 sidecar；
2. 将 `Gateway.scorer` 从具体 `*Scorer` 抽象成 scorer interface；
3. 增加配置：

```json
{
  "scorer": {
    "kind": "native",
    "endpoint": "http://127.0.0.1:8080/v1/systemone",
    "token": ""
  }
}
```

4. 增加 shadow mode，同时调用 native 和 remote，记录质量、延迟与分歧；
5. 在冻结样本集上比较 native、llm2jev、Laya、local-jev 和 jev-rs。

这些都不属于本设计的实现范围。

## 9. 完成判定

本设计对应的实现只有在以下条件同时满足时才算完成：

* `router/systemone_http.go` 存在并实现接口；
* `router/systemone_http_test.go` 覆盖本设计列出的必测项；
* `go test ./router/... ./router/schema/...` 通过；
* 不修改 native scorer 的默认行为；
* 没有新增未声明的运行时依赖；
* 文档、测试和错误模型与实现一致。

## 10. 来源

* [llm2jev README](https://github.com/tic-top/llm2jev)
* [fast-router 当前 System One 类型定义](../../../router/scorer.go)
* [fast-router 当前 Gateway 调用路径](../../../router/gateway.go)
* [Jev 开源实现集成评估](../../Jev-开源实现集成评估.md)
