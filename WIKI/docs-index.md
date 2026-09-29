---
type: Documentation Index
title: fast-router 现有文档索引
description: docs、book、data 和 SOP 知识资产的分类导航。
tags: [docs, knowledge, references]
timestamp: 2026-09-28T18:00:00+08:00
---

# 现有文档索引

## 规范与流程

* [OKF-SPEC](../OKF-SPEC.md) - Markdown 知识包规范。
* [AGENTS](../AGENTS.md) - Agent 协作和决策规则。
* [Build SOP](../SOP/build.md) - 构建、打包、分发和故障排查。

## 当前交付与进展

* [交付报告 v1.0](../docs/fast-router-交付报告-v1.0.md) - 架构/引擎/方法/模型四层锁定的 v1.0 交付，P1=73.3%，39 测试快照，6 平台。
* [Go 重写进展](../docs/fast-router-Go重写进展-v0.1.md) - Go C2/C3/C4/C5 状态、Jev API 对齐和剩余工作。
* [架构设计 v0.2](../docs/fast-router-架构设计-v0.2.md) - litellm/schema fact 校验后的目标架构。
* [智能路由设计共识](../docs/fast-router-智能路由设计共识-v0.1.md) - 任务轮、Jev、挡死、选模和回流设计。
* [知识沉淀 v0.1](../docs/fast-router-知识沉淀-v0.1.md) - AI-native 五阶段知识闭环（含 v0.2-v0.7 增量）。
* [Jev 路由决策效果评估](../docs/Jev路由决策效果评估-2026-09-28.md) - 区分运行成功、任务判定正确和选模因果收益。
* [CPU LLM 推理方案评估](../docs/CPU-LLM推理方案评估-2026-09-29.md) - CPU 推理优化、专用小模型与 fast-router 实测边界。
* [Kev System One 决策模型深度研究](../docs/Kev-System-One决策模型深度研究-2026-09-29.md) - Kev 的 decision-native 架构、System One 接口兼容性、质量证据与 Intel CPU 集成边界。
* [Qwen3.8-27B-in-C 深度研究](../docs/Qwen3.8-27B-in-C深度研究-2026-09-29.md) - 原生 C CPU 推理性能、正确性、macOS 构建实测及其与 System One 的接口差异。
* [Python POC 进展](../docs/fast-router-POC进展-v0.1.md) - 早期模型准确率、延迟与边界记录。
* [架构设计 v0.1](../docs/fast-router-架构设计-v0.1.md) - fact 校验前的草案，主要用于历史对照。

## 研究

* [Jev / System One 开源实现集成评估](../docs/Jev-开源实现集成评估.md) - 对比 llm2jev、Laya、local-jev、jev-rs 等的集成路径与限制。
* [System One 引擎抽象与可替换性](../docs/System-One-引擎抽象与可替换性.md) - 区分协议、引擎接口与模型 runtime，核对当前架构的真实替换边界。
* [LLM2Jev 研究笔记](../docs/LLM2Jev-研究笔记.md) - per-candidate yes/no vs 单 token 的方法学依据。
* [Ornith-1.5-9B 研究笔记](../docs/Ornith-1.5-9B-研究笔记.md) - v1.0 锁定模型（9B + apply_template，P1=73.3%）。
* [MiniCPM5-2B 深度研究](../docs/MiniCPM5-2B-深度研究.md) - 模型事实、部署、chat template、tool calling 与 fast-router C3 适配评估。
* [MiniCPM5-2B 初步研究](../docs/MiniCPM5-2B-研究笔记.md) - 较早的候选模型记录；详细事实核验以深度研究为准。
* [System 1 可复用性研究](../docs/fast-browser-use-System1架构与可复用性研究.md) - 单 token 候选打分、KV cache、System 2 和解耦建议。

## 审计

* [fast-router audit report 2026-09-26](../docs/audit-report-fast-router-2026-09-26.md) - 仓库结构与已知差距审计，作为 v1.0 之前的 I-DISC 复核素材。

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
