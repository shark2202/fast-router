---
type: Developer Guide
title: fast-router 开发协作指南
description: 本项目的 agent 协作、知识记录、Git 和变更边界。
tags: [development, agents, git, process]
timestamp: 2026-09-25T00:00:00+08:00
---

# 开发协作指南

## 权威约束

1. `AGENTS.md` 是本仓库的 agent 协作规则。
2. `OKF-SPEC.md` 约束知识文档的 frontmatter、index、log、链接和引用。
3. `SOP/build.md` 是构建/打包/分发流程的操作基线。
4. 代码、测试和实际命令输出优先于过时的进展文字。

## 决策模板

任何重要变更都必须回答：

* **维度**：从哪些维度判断成功？
* **死法**：判断错误时最可能如何失败？
* **可逆性**：能否以有限成本回退并通过反馈收敛？

对真实推理、跨协议、分发包等高风险改动，先做最小可逆实验，再扩展范围。

## 调研落盘

调研文档至少记录：来源、方法、发现、局限、结论。不要只在聊天中给结论。新研究应进入 `docs/` 或 `WIKI/`，并使用 OKF frontmatter。

## Git 规则

* 只提交当前任务相关文件。
* 禁止 `git push`；由用户手动推送。
* 未经明确要求，不使用 `--force`、`--no-verify`、`reset --hard`。
* 不提交 API key、运行配置、模型权重、缓存、构建产物。

## 变更边界

* Go 路由逻辑变更：同步更新对应测试和 `WIKI/modules/go-router.md` 或状态页。
* Zig ABI 变更：同时验证 `frwrapper.zig`、Go FFI 声明和目标平台库。
* 构建流程变更：同步更新 `SOP/build.md` 和 `WIKI/build-and-test.md`。
* 设计状态变更：同步更新 `docs/` 进展文档和 `WIKI/implementation-status.md`。

## 常用检查

```bash
git status --short
gofmt -l cmd router
go test ./...
go build ./cmd/fast-router
git diff --check
```
