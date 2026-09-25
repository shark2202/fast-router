---
type: Implementation Status
title: fast-router 实现状态矩阵
description: 区分当前代码、测试覆盖、真实依赖验证和规划能力。
tags: [status, verification, gaps]
timestamp: 2026-09-25T00:00:00+08:00
---

# 实现状态

## 状态词义

* **代码存在**：仓库有实现，但不代表真实依赖或生产场景通过。
* **测试通过**：已有自动化测试覆盖并在本地命令中通过。
* **待实测**：需要真实模型、动态库、upstream 或多平台环境。
* **设计/规划**：文档提出，但代码没有闭环。

## 当前矩阵

| 能力 | 代码位置 | 当前状态 | 证据/边界 |
|---|---|---|---|
| C2 任务轮判定 | `router/turn_detector.go` | 测试通过 | OpenAI/Anthropic 结构测试覆盖；ambiguous 仍是显式状态 |
| C3 Choice scorer | `router/scorer.go` | 纯 Go 逻辑测试通过 | Backend 可插拔；测试使用 mock，不等于真实模型准确率 |
| C3 Zig/libllama Backend | `router/zig_backend.go`, `zig/frwrapper.zig` | 代码存在，待实测 | 依赖动态库、llama.cpp、GGUF、ABI 和目标平台 |
| C4 能力挡死 | `router/matcher.go` | 测试通过 | 基于 seed capability vector；数据仍是 POC/声明值 |
| C5 加权选模 | `router/selector.go` | 测试通过 | measured 为空时冷启动按成本；真实质量回流未闭合 |
| OpenAI 入口 | `router/gateway.go` | 代码存在 | 需要真实 upstream 端到端验证 |
| Anthropic 入口 | `router/gateway.go` | 代码存在 | 需要真实 Anthropic 兼容 upstream 验证 |
| OpenAI/Anthropic schema | `router/schema/` | 单元测试通过 | 复杂工具和多事件流仍应做端到端验收 |
| Admin UI/config | `router/admin.go`, `config.go` | 代码存在 | 保存和热加载路径有实现；认证和并发安全边界需部署评审 |
| GGUF 模型下载 | `router/model_download.go`, `modelscope.go` | 代码存在，待实测 | 依赖 Python modelscope 或直接下载 fallback 的实际环境 |
| CLI server | `cmd/fast-router/main.go` | 可编译目标 | 是否能加载本机动态库取决于库路径和模型配置 |
| benchmark | `cmd/jevbench`, `cmd/zigbench` | 入口存在，待真实模型 | 真实 P1/P2 结果不能从 mock 测试推断 |
| 六平台交叉编译 | `scripts/pack.sh` | 脚本存在，待完整验证 | Zig、llama.cpp nightly 和各平台库下载可能失败 |
| 工具循环路由继承 | Gateway 设计目标 | 未闭环 | 当前 route 注释说明暂无 session state，可能重新路由 |
| 判据回流/校准 | 设计文档能力 | 未实现 | 无 verdict ledger 和独立统计复核闭环 |

## 自动化测试规模

截至 2026-09-25，`go test -json ./...` 报告 **37 个 Go 测试通过**：

* C2 任务轮判定：11 个。
* C3 scorer：8 个。
* C4/C5 与 route chain：11 个。
* schema：7 个。

Python POC 另有任务轮和 matcher/selector 测试。Go 是当前主线，Python 是历史/对照实现。

## 关键判断

“C2/C3/C4/C5 完成”在项目进展文档中表示 Go 逻辑和接口完成；它不表示 C3 的真实推理 Backend、C1/C6 的真实网络链路或分发包已经验收。
