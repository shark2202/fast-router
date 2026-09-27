---
type: Known Gaps
title: fast-router 已知缺口与风险
description: v1.0 交付后仍未闭合的影响生产化、验证和维护的缺口。
tags: [risks, gaps, decisions, verification, v1.0]
timestamp: 2026-09-26T18:00:00+08:00
---

# 已知缺口与风险

> 基线日期：2026-09-26（v1.0 交付后）。相对 v0 知识包基线，已经闭合的项会显式标注；新出现的项来自交付报告的诚实边界部分（[fast-router-交付报告-v1.0.md §四](../../docs/fast-router-交付报告-v1.0.md)）。

## 已闭合（不再视为缺口）

* ~~C3 真实链路未跑通~~ — v1.0 已用真实 Ornith-1.5-9B + per-candidate yes/no + apply_chat_template 跑出 P1=73.3%；详见交付报告。
* ~~工具循环路由继承未完成~~ — `Gateway.sessions` + `session_test.go` 覆盖：`ContinuationTurn` 复用上游，`NewTaskTurn` 重新路由。
* ~~`scripts/pack.sh` 缺验收清单~~ — `SOP/build.md` 已提供跨平台验收清单。

## P0：Jev 智能路由 P2 不可用（硬件约束）

9B CPU 上 P2=77s/决策，需 GPU（CUDA）或 Apple Silicon MLX 才实用。当前 `zig/build.zig` 留有 GPU 构建钩子但未测试。

* 路径：要么补齐 Metal / CUDA 后端并真实验收，要么在产品层把 Jev 智能路由定位为"GPU/MLX 可用才推荐"，hint-only 仍是默认。
* 优化路径：prefix-reuse（state+instructions 共享 KV cache）可减 forward 次数，预计 9B CPU 15s 左右；**未实现**。

## P0：C7-C9 判据回流未实现

当前 registry 是静态的，`Measured` 字段写入但没有 verdict ledger、没有统计复核、没有 candidate→active 的学习闭环。这意味着路由质量不会随真实流量自动校准。

* 路径：先做最小可逆方案（落盘 verdict + 异步校准），不要直接把 measured 锁死到上游协议。

## P0：I-DISC 未满足

所有交付产物由单一研究者产出，未经过 fresh review。要让决策进入可信通道，必须由独立 agent 复核代码 + 文档（建议 `requesting-code-review` skill）。

## P1：Windows on-device 未实测

`zig_backend_windows.go` 走 `syscall.NewLazyDLL` 编译过，但没有在 Windows 上跑过 yes/no 基准与端到端转发。

## P1：真实 agent 客户端联调未做

hint-only 与 schema 转换的所有链路都用 curl 验过，没有用 codex / claude code / pi-agent 等真实 agent 客户端跑完整任务。SDK 风格与 SSE 流式行为可能与 curl 路径有差异。

## P1：网关端到端覆盖不足

单元测试覆盖 schema 转换 + session + engine seam，但仍缺：

* Jev 智能路由 + 真实 upstream 的非流式与流式全链路（当前仅做 hint-only + 真实 upstream 的全链路）。
* upstream 错误、超时、取消和客户端断开在 Jev 评分耗时的链路下的传播。
* 多轮工具循环跨协议 + session 缓存命中 + Jev 评分回退的组合行为。

## P1：安全边界

Admin API 可读写 API key 和 registry，当前代码未显示认证/授权。默认设计面向 localhost；如果部署到非本机网络，必须先加访问控制、日志脱敏和配置文件权限验证。

## P1：配置和数据一致性

配置保存是 JSON 文件覆盖写；没有版本迁移、原子写、schema 校验或并发冲突处理。registry 的 `Measured` 字段存在，但没有 verdict ledger、统计复核和 candidate→active 的持久化流程（C7-C9 缺口见上）。

## P2：模型下载路径

模型下载代码说明会调用 modelscope Python SDK，但当前 Wiki 不把它当作构建时必需依赖。需要确认 SDK 不可用时的 fallback、断点续传、文件完整性校验、磁盘空间和下载后的 config 更新行为。

## P2：外部 System One Engine 未真实接入

`HTTPSystemOneClient` 已实现并测试，但 jev-rs / Laya / llm2jev / local-jev 等真实 sidecar 尚未接入并跑通端到端请求。相关评估见 [Jev / System One 开源实现集成评估](../../docs/Jev-开源实现集成评估.md) 和 [System One 引擎抽象与可替换性](../../docs/System-One-引擎抽象与可替换性.md)。

## P2：设计文档漂移

旧的 Python POC 文档仍描述"Go 尚未开始"或"模型不可达"的历史状态；Go 重写进展是当前主线。更新进展时应写绝对日期和证据，不要覆盖历史记录。

## 维护策略

每次闭合缺口时同时更新：

1. 代码和测试。
2. 对应 `docs/` 进展/设计文档。
3. 本页风险条目和 `WIKI/implementation-status.md`。
4. `SOP/build.md`（如果构建/分发流程改变）。