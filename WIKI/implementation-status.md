---
type: Implementation Status
title: fast-router 实现状态矩阵
description: 区分当前代码、测试覆盖、真实依赖验证和规划能力，并标记 v1.0 交付后仍存在的边界。
tags: [status, verification, gaps, v1.0]
timestamp: 2026-09-26T18:00:00+08:00
---

# 实现状态

> **基线日期**：2026-09-26。fast-router v1.0 已交付（[交付报告](../docs/fast-router-交付报告-v1.0.md)）：
> hint-only 模式 P2<1s 端到端可用；Jev 智能路由 P1=73.3%（Ornith-1.5-9B + per-candidate yes/no + apply_chat_template）。
> 本页状态矩阵反映 v1.0 后的真实代码与测试边界。

## 状态词义

* **代码存在**：仓库有实现，但不代表真实依赖或生产场景通过。
* **测试通过**：已有自动化测试覆盖并在 `go test ./...` 中通过。
* **端到端验证**：已在真实 upstream / 模型 / 平台上跑通验收用例。
* **设计/规划**：文档提出，但代码没有闭环。

## 当前矩阵

| 能力 | 代码位置 | 当前状态 | 证据/边界 |
|---|---|---|---|
| C2 任务轮判定 | `router/turn_detector.go` | 测试通过 | OpenAI/Anthropic 各协议测试覆盖；ambiguous 仍是显式状态 |
| C3 Choice scorer | `router/scorer.go` | 纯 Go 逻辑测试通过 | 单 token 与 per-candidate yes/no 双路径；自动适配 ExtendedBackend |
| C3 per-candidate yes/no | `router/scorer.go`（`scoreChoiceYesNo`）+ `router/zig_backend.go` | 单元测试通过 + 真实模型基准 | P1 从 16.7%（0.5B）→73.3%（9B + apply_template）；真实模型由 `cmd/yesnobench` 验证 |
| C3 `llama_chat_apply_template` | `router/zig_backend.go`（`ApplyChatTemplate`）+ `zig/frwrapper.zig` | 单元测试通过 + 真实模型基准 | 单点最大正向因素（+26.6pp） |
| C3 Zig/libllama Backend | `router/zig_backend.go`（Unix）/ `zig_backend_windows.go`（Windows）+ `zig/frwrapper.zig` | 单元测试通过 + 真实模型基准 | `purego`（Unix）+ `syscall.NewLazyDLL`（Windows），CGO_ENABLED=0 |
| System One Engine seam | `router/systemone_engine.go`（native）+ `router/systemone_http.go`（HTTP） | 单元测试通过 | gateway 已通过 `NewGateway(engine, …)` 注入；HTTP 适配可指向外部 `/v1/systemone` sidecar |
| C4 能力挡死 | `router/matcher.go` | 测试通过 | 基于 seed capability vector；数据仍是 POC/声明值 |
| C5 加权选模 | `router/selector.go` | 测试通过 | measured 为空时冷启动按成本；真实质量回流未闭合 |
| 路由链 session 继承 | `router/gateway.go`（`sessions` + `cachedRoute`）+ `router/session_test.go` | 测试通过 | `TurnKind != NewTaskTurn` 复用上次 upstream/model/protocol；`NewTaskTurn` 重新路由 |
| OpenAI 入口 | `router/gateway.go` | 端到端验证（hint-only） | curl → fast-router → "pong" 真实 upstream |
| Anthropic 入口 | `router/gateway.go` | 端到端验证（hint-only + 流式） | Anthropic `/v1/messages` → OpenAI upstream → 响应转回；SSE 6-event flow |
| OpenAI/Anthropic schema | `router/schema/`（3 文件） | 单元测试通过 + 端到端验证 | system、message、tool_calls↔tool_use、SSE 流式均通过 |
| 工具循环路由继承 | `router/gateway.go`（session 缓存） + `router/session_test.go` | 测试通过 | 非 `NewTaskTurn` 继承路由；tool_calls↔tool_use 双向 |
| Admin UI/config | `router/admin.go`, `config.go` | 代码存在 | 保存和热加载路径有实现；认证和并发安全边界需部署评审 |
| GGUF 模型下载 | `router/model_download.go`, `modelscope.go` | 代码存在 | admin UI 选模型 → modelscope SDK 下载；fallback 路径未独立验收 |
| CLI server | `cmd/fast-router/main.go` | 可编译目标 | 是否能加载本机动态库取决于库路径和模型配置 |
| benchmark（单 token） | `cmd/jevbench/main.go` | 入口存在，真实模型基准 | 当前基线已被 yes/no + apply_template 取代，保留作对照 |
| benchmark（yes/no） | `cmd/yesnobench/main.go` | 入口存在，真实模型基准 | P1=73.3% 主验收路径 |
| benchmark（FFI 调试） | `cmd/zigbench/main.go` | 入口存在 | 单样本 Zig/FFI 调试 |
| 六平台交叉编译 | `scripts/pack.sh` + `SOP/build.md` | 脚本存在，6 平台 zip 生成 | zig + llama.cpp nightly + 各平台库下载可能失败；按 `SOP/build.md` 逐项验收 |
| 判据回流/校准（C7-C9） | 设计文档能力 | 未实现 | 无 verdict ledger 和独立统计复核闭环 |

## 自动化测试规模

截至 2026-09-26，`go test -v ./...` 报告 **60 个 Go 测试通过**（`fast-router/router` + `fast-router/router/schema`）：

* C2 任务轮判定：11 个。
* C3 scorer / yes-no：8 个。
* C4/C5 matcher-selector + 路由链：11 个。
* schema（请求/响应/SSE/工具调用）：13 个。
* session 路由继承：2 个。
* System One engine（native + HTTP client）：9 个。
* Gateway 与 engine 装配：1 个。
* 其他支撑测试若干。

Python POC 另有任务轮和 matcher/selector 测试。Go 是当前主线，Python 是历史/对照实现。
交付报告中的"39 测试"为 v1.0 锁定当天的快照；当前已扩到 60 个。

## 关键判断

* “C2/C3/C4/C5 完成”在项目进展文档中表示 Go 逻辑和接口完成；**对 C3 而言**已用真实 Ornith-1.5-9B GGUF 在 9B 量化模型上跑出 P1=73.3%，但 P2=77s（CPU）仍需 GPU/MLX 才实用。
* "端到端验证"目前仅对 hint-only 模式 + schema 转换 + session 路由成立；Jev 智能路由的完整链路（upstream 透传 + Jev 决策 + 真实回写）仍是基准级验证，不是长链路生产验证。
* System One Engine seam 已实现 native + HTTP 两种 adapter，但外部 sidecar（jev-rs / Laya / llm2jev 等）尚未完成真实接入验收。