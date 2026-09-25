---
type: Documentation Index
title: fast-router 现有文档索引
description: docs、book、data 和 SOP 知识资产的分类导航。
tags: [docs, knowledge, references]
timestamp: 2026-09-25T00:00:00+08:00
---

# 现有文档索引

## 规范与流程

* [OKF-SPEC](../OKF-SPEC.md) - Markdown 知识包规范。
* [AGENTS](../AGENTS.md) - Agent 协作和决策规则。
* [Build SOP](../SOP/build.md) - 构建、打包、分发和故障排查。

## 当前设计与进展

* [Go 重写进展](../docs/fast-router-Go重写进展-v0.1.md) - Go C2/C3/C4/C5 状态、Jev API 对齐和剩余工作。
* [架构设计 v0.2](../docs/fast-router-架构设计-v0.2.md) - litellm/schema fact 校验后的目标架构。
* [智能路由设计共识](../docs/fast-router-智能路由设计共识-v0.1.md) - 任务轮、Jev、挡死、选模和回流设计。
* [Python POC 进展](../docs/fast-router-POC进展-v0.1.md) - 早期模型准确率、延迟与边界记录。
* [架构设计 v0.1](../docs/fast-router-架构设计-v0.1.md) - fact 校验前的草案，主要用于历史对照。

## 研究

* [System 1 可复用性研究](../docs/fast-browser-use-System1架构与可复用性研究.md) - 单 token 候选打分、KV cache、System 2 和解耦建议。

## 理论与组织

`book/` 是 ai-native 理论和组织流程知识库，不是 Go runtime 依赖。主要主题包括：

* `ai-native组织理论.md`、`ai-native组织理论-元组织.md`：理论和元组织。
* `基于ai-native的落地框架.md`：落地框架。
* `基于ai-native的开发组织的元组织.md`：开发组织。
* `基于ai-native的测试质检组织的元组织.md`：测试质检组织。
* `基于ai-native的需求分析组织的元组织.md`：需求分析组织。
* `基于ai-native的软件生产组织的元组织.md`：软件生产组织。
* `基于ai-native的工程基础设施部署规范.md`：基础设施部署。
* `基于ai-native的知识沉淀流程规范.md`：知识沉淀流程。

## 数据

* [任务类型样例](../data/task_type_samples.json) - Python/POC 样例数据，不是正式任务类型注册表。
