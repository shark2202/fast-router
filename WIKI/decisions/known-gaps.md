---
type: Known Gaps
title: fast-router 已知缺口与风险
description: 当前工程中影响生产化、验证和维护的明确缺口。
tags: [risks, gaps, decisions, verification]
timestamp: 2026-09-25T00:00:00+08:00
---

# 已知缺口与风险

## P0：真实链路未闭合

* C3 的 Go 逻辑有 mock 测试，但真实 Zig + llama.cpp + GGUF 尚未形成可重复的验收记录。
* P1 分类准确率和 P2 延迟必须在明确模型、量化版本、CPU/GPU、平台和样例集下重新记录。
* `scripts/pack.sh` 的六平台目标需要逐平台验证，而不是只检查脚本语法。

## P0：任务轮继承未完成

设计要求工具循环继承当前任务轮路由；当前 gateway 识别了 turn 类型，但没有 session route store，非新任务轮仍可能重新执行 route 链。错误结果是工具循环期间切换模型或重复 scorer 成本。

最小可逆路径：先增加内存 session/turn 测试，再决定是否引入持久化；不要直接把 session 锁死到整个 conversation。

## P1：网关端到端覆盖不足

单元测试覆盖 schema 转换，但还缺：

* 真实 OpenAI-compatible upstream 的非流式和流式验收。
* 真实 Anthropic-compatible upstream 的非流式和流式验收。
* tool use/tool result 多轮跨协议验收。
* upstream 错误、超时、取消和客户端断开传播。

## P1：安全边界

Admin API 可读写 API key 和 registry，当前代码未显示认证/授权。默认设计面向 localhost；如果部署到非本机网络，必须先加访问控制、日志脱敏和配置文件权限验证。

## P1：配置和数据一致性

配置保存是 JSON 文件覆盖写；没有版本迁移、原子写、schema 校验或并发冲突处理。registry 的 `Measured` 字段存在，但没有 verdict ledger、统计复核和 candidate→active 的持久化流程。

## P2：模型下载路径

模型下载代码说明会调用 modelscope Python SDK，但当前 Wiki 不把它当作构建时必需依赖。需要确认 SDK 不可用时的 fallback、断点续传、文件完整性校验、磁盘空间和下载后的 config 更新行为。

## P2：设计文档漂移

旧的 Python POC 文档仍描述“Go 尚未开始”或“模型不可达”的历史状态；Go 重写进展是当前主线。更新进展时应写绝对日期和证据，不要覆盖历史记录。

## 维护策略

每次闭合缺口时同时更新：

1. 代码和测试。
2. 对应 `docs/` 进展/设计文档。
3. 本页风险条目和 `WIKI/implementation-status.md`。
4. `SOP/build.md`（如果构建/分发流程改变）。
