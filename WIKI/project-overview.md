---
type: Project Overview
title: fast-router 项目概览
description: 本地 LLM 智能路由器的事实状态、工作范围和知识来源。
tags: [overview, architecture, status, ai-native]
timestamp: 2026-09-25T00:00:00+08:00
---

# 项目概览

## 目的

fast-router 是一个本地运行的 LLM 智能路由器原型：对 OpenAI 和 Anthropic 风格请求提供统一入口，在任务轮级别识别任务类型，再按模型声明能力、成本和实测结果选择 upstream/model，最后转发请求和响应。

当前仓库的主实现是 Go；Zig 负责把 llama.cpp 的 C API 收窄成可由 Go `purego` 调用的 FFI；Python 目录保留早期 Jev 打分与任务轮判定 POC。

## 事实来源与调研方法

### 来源

1. `AGENTS.md`：协作、Git、OKF 和决策约束。
2. `OKF-SPEC.md`：Markdown + YAML frontmatter 知识包规范。
3. `SOP/build.md`：构建、打包、分发和用户侧运行流程。
4. `cmd/`、`router/`、`zig/`、`scripts/`：当前实现。
5. `*_test.go`、`tests/*.py`：行为契约和已验证范围。
6. `docs/`、`book/`、`data/`：设计背景、研究记录和样例数据。

### 方法

按“目录清点 → 入口阅读 → 核心调用链 → 测试/脚本核验 → 设计文档对照”的顺序检查。实现状态按四类记录：已实现、测试覆盖、声明但未实测、历史/规划。

## 成功维度

* **结构**：新人能从 `WIKI/index.md` 找到入口、构建命令和模块职责。
* **行为**：C2/C3/C4/C5 的 Go 纯逻辑测试能通过；网关能编译并启动到配置决定的模式。
* **边界**：Mock、POC、设计目标和真实推理/真实 upstream 验证被明确区分。
* **可维护性**：新增模块或流程可以在对应 Wiki 概念文档中增量更新，而不需要重写总文档。

## 最可能的失败方式

1. 把 `SOP/build.md` 的目标流程误认为所有步骤已在当前机器验证。
2. 把 `Scorer` 的 Mock Backend 测试误认为真实 GGUF 推理已验证。
3. 把当前网关的协议转换误认为已经覆盖所有 OpenAI/Anthropic 边界情况。
4. 把任务轮工具循环继承误认为已有 session 状态锁定；当前代码仍有重新路由的 POC 注释。
5. 把 seed registry 中的模型名和价格当成实时供应商事实。

## 可逆性

Wiki 只新增 Markdown，不改变运行时行为；每篇文档有独立 frontmatter 和链接，可单独修订或删除。代码事实变化时，优先更新对应模块页和 `implementation-status.md`，再更新总入口。

## 当前一句话状态

Go 路由核心和协议转换已有测试覆盖，Zig wrapper 与 Go FFI 代码已存在，但真实 llama.cpp/模型链路、跨平台打包、网关端到端和任务轮持久继承仍需单独验证。

## 局限

本 Wiki 是仓库内事实的逆向整理，不替代 API 正式规范、供应商价格页或真实部署验收。没有运行成功的步骤会明确标记为“待验证”，不会用设计文档中的将来时替代证据。

## 结论

项目当前最适合被理解为“可编译的 Go 路由原型 + 已定义的 Zig/llama.cpp 接缝 + 历史 Python POC”，而不是已经完成生产部署的智能路由服务。
