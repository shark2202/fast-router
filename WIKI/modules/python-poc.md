---
type: Historical Module
title: Python POC 模块
description: 早期 Jev scorer、任务轮判定和 matcher/selector 验证实现。
tags: [python, poc, historical]
timestamp: 2026-09-25T00:00:00+08:00
---

# Python POC 模块

## 作用

`src/fast_router/` 是 Go 重写前的验证实现，证明了任务轮判定、能力挡死、加权选模和单 token scorer 的基本形状。它不是当前 server 的运行时依赖。

## 模块

* `jev_scorer.py`：候选 code、prompt、模型后端和 benchmark 相关逻辑。
* `turn_detector.py`：OpenAI/Anthropic 任务轮判定。
* `matcher.py`：任务能力需求与模型能力向量匹配。
* `selector.py`：成本和 measured pass rate 的选择逻辑。
* `registry.py`、`task_types.py`：seed registry 和任务类型。

## 历史证据

现有 POC 文档记录：任务轮判定通过；0.5B 与 1.5B CPU 实测显示分类准确率和延迟都不足以直接满足目标；模型规模和推理后端是关键变量。这些结果支持继续验证，但不能外推为 Go/llama.cpp 真实链路结果。

## 与 Go 的关系

Go 版保留相同的 C2/C4/C5 概念，并把 C3 的 Backend 抽象和 Jev Choice schema 明确化。Python 代码可作为行为对照和历史溯源，但新增功能应优先落在 Go 主线并补 Go 测试。

## 运行边界

Python 测试通常需要把 `src` 加入 `PYTHONPATH`。模型 benchmark 还依赖模型权重和具体推理库；不要因为纯逻辑测试通过就认为模型资源可用。
