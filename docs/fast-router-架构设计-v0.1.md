---
type: Architecture Design
title: fast-router 架构设计 v0.1（草案）—— 业务/数据/技术/功能四视图
description: 基于已落盘设计共识，用行业规范+最佳实践+POC 逐步缩小可能性空间。本 v0.1 草案先基于设计共识起草四视图骨架+功能清单，标注 [待 fact 校验]（recon-gateway/recon-apispec 子代理调研 litellm 与 OpenAI/Anthropic API 规范中）与 [待 POC 验证]（对应设计共识 P1-P6）。子代理返回后整合为 v0.2。
source_consensus: docs/fast-router-智能路由设计共识-v0.1.md
recon_in_flight: [recon-gateway（litellm 架构与数据模型）, recon-apispec（OpenAI/Anthropic API 规范与格式互转）]
timestamp: 2026-09-25T01:00:00+08:00
status: v0.1 草案，待 fact 校验与 POC，实例化非验证
---

# fast-router 架构设计 v0.1（草案）

## ——业务架构 / 数据架构 / 技术架构 / 功能清单

> **三行说明**
> ① 本草案基于已落盘设计共识（Q1-Q10 + Q7 任务轮修正），先起草四视图骨架。
> ② 标注 `[待 fact 校验]` = 等 recon 子代理抓 litellm/OpenAI/Anthropic 官方规范后校验细化；`[待 POC 验证]` = 对应设计共识 P1-P6，须 Phase 1 实测。
> ③ 本版是骨架非定稿；recon 返回后整合为 v0.2，POC 后整合为 v1.0。

---

## 第一章 业务架构

### 1.1 业务参与者

| 参与者 | 角色 | 关键属性 |
|---|---|---|
| agent 客户端 | 路由服务消费者 | pi-agent / codex / claude code；发 OpenAI 或 Anthropic 格式请求 |
| fast-router | 智能路由网关 | 本地单进程；Jev 任务类型打分 + 挡死 + 加权选模 + 转发 |
| upstream LLM 提供商 | 真实模型后端 | OpenAI / Anthropic / [待 fact 校验：其他 litellm 支持的 provider] |
| goal-keeper（人类） | 不可委托内核 | upstream 凭据/定价签字、判据回流复核裁决、例外 HALT |

### 1.2 业务能力

| 能力 | 描述 | 对应闭包 |
|---|---|---|
| 智能路由 | 任务轮判断 + Jev 任务类型打分 + 挡死 + 加权选模 | C2/C3/C4/C5 |
| 模型注册与能力标签 | upstream/model 元数据 + 声明能力向量 + 实测矩阵 | C9/存储 |
| 请求转发与格式转换 | 双端点接入 + schema 转换 + SSE 透传 + 工具循环继承 | C1/C6 |
| 判据回流与校准 | 判据采集 + LLM 侦察兵提议 + 独立统计复核 | C7/C8 |
| 成本与质量记账 | per-task-turn 成本/延迟/判据四字段 | C7/存储 |

### 1.3 业务用例

| UC# | 用例 | 主参与者 | 前置 | 产出 |
|---|---|---|---|---|
| UC1 | 路由一个请求 | agent 客户端 | upstream 已注册 | 选定 upstream+model，转发响应 |
| UC2 | 注册 upstream/model | goal-keeper | 凭据就位 | 模型注册表 + 声明能力向量 |
| UC3 | 上报判据 | agent 客户端/router 推断 | 任务轮完成 | 判据账本记录 |
| UC4 | 校准路由表 | goal-keeper | 判据积累 | 实测矩阵 candidate→active |
| UC5 | 任务类型演化 | router（空层检测） | 空层触发 | 任务类型增删（经统计复核） |

### 1.4 业务价值流

```
agent 发请求 → fast-router 智能选模 → upstream 执行 → 响应回传
                  ↓（后台）
            判据回流 → 校准实测矩阵 → 下次路由更准
```

**业务价值**：使用端无感（固定 model 名 + 不切 API），按任务自动选最优模型，成本/质量优于单一模型绑定。

---

## 第二章 数据架构

### 2.1 实体清单

| 实体 | 职责 | 关键字段（骨架，[待 fact 校验：litellm model registry schema]） |
|---|---|---|
| upstream_provider | 上游提供商 | id, name, base_url, api_key_ref, protocol(openai/anthropic) |
| model | 模型注册 | id, provider_id, model_name, context_window, cost_per_1k(in/out) |
| capability_vector | 声明能力向量 [待 fact 校验：litellm 是否有类似字段] | model_id, axis(code/reasoning/long_context/vision/...), score, source(声明/实测), confidence, recheck_condition |
| task_type | 任务类型 | code, label, capability_requirement_vector, version, source(种子/涌现) |
| measured_matrix | 实测矩阵（task_type × model） | task_type_id, model_id, pass_rate, sample_size, last_updated, status(candidate/active) |
| task_turn_record | 任务轮路由决策日志 | turn_id, message_hash, task_type, chosen_model, scores, latency_ms, timestamp |
| verdict_ledger | 判据账本 | turn_id, verdict(binary/proxy), signal_source, timestamp |
| label_update_ledger | 标签更新台账 | id, llm_proposal, statistical_review(pass/fail), applied(candidate/active), timestamp |

### 2.2 数据流

```
请求 → task_turn_record（路由决策留档）
        ↓
判据采集 → verdict_ledger
        ↓
LLM 侦察兵提议 → label_update_ledger（candidate）
        ↓ 独立统计复核
measured_matrix 更新（candidate→active）
        ↓
capability_vector 校准（声明→实测升格）
        ↓ 空层检测
task_type 演化
```

### 2.3 存储选型 [待 fact 校验]

- 起步：sqlite + 文件（单机，轻量）。
- model registry schema：[待 fact 校验：litellm 的 model registry 怎么存，可否复用其字段命名]。
- Jev 模型权重：本地文件（huggingface cache，复用 fast_browser_use 的 model_location 逻辑）。

---

## 第三章 技术架构

### 3.1 组件分解

| 组件 | 职责 | 来源 | 改造 |
|---|---|---|---|
| gateway | 双端点接入 + schema 转换 + SSE 透传 | 复用 litellm schema 转换子模块 [待 fact 校验：litellm 哪些函数可取] | 不整体引入，取 message/tool/stream 转换函数 |
| turn-detector | 任务轮判定（tool_result vs user message） | 自研 | message 末尾结构判定 |
| jev-scorer | Jev 单 token 任务类型打分 | 抽取自 fast_browser_use model.py | 取 candidate_codes + score + KV 前缀缓存；解浏览器动作空间耦合 |
| matcher | 声明能力挡死 | 自研 | 任务类型→能力需求 ∩ 模型声明能力 |
| selector | 实测矩阵+成本加权选模 | 自研 | 加权综合分 + tie-break |
| forwarder | upstream 转发 + 格式回转 | litellm 转发逻辑 [待 fact 校验] | session 内工具循环继承 |
| verdict-collector | 判据采集 | 自研 | [待 POC：G-R1 判据信号回流机制] |
| label-refiner | LLM 侦察兵 + 统计复核 | 自研 | LLM 提议 + 独立统计复核门 |
| registry-store | 模型/任务类型/矩阵存储 | sqlite + 文件 | schema 见第二章 |

### 3.2 技术栈

| 层 | 选型 | 理由 |
|---|---|---|
| 语言 | Python | litellm/transformers/mlx_lm 生态 |
| Jev 推理 | MLX（Apple Silicon）/ PyTorch（CUDA） | 复用 fast_browser_use 后端 |
| 网关 schema | litellm 子模块 [待 fact 校验] | 不重复造 4 向格式转换轮子 |
| 存储 | sqlite + 文件 | 单机轻量 |
| 进程 | 单进程 + 线程锁（复用 fast_browser_use self.lock） | 9B 常驻 |

### 3.3 部署

- 单机单进程，9B 4-bit 常驻（~5-6GB）[待确认：Mac 内存]。
- 对外：localhost 双端点（/v1/chat/completions + /v1/messages）。
- 客户端配 base_url 指向 localhost，model 名任意。

### 3.4 POC 验证点 [对应设计共识 P1-P6]

| POC# | 验证 | 组件 | 通过判据 |
|---|---|---|---|
| POC1 | Jev 任务类型分类准确率 | jev-scorer | >70%（Qwen3.5-9B，10 类，首 1024 token） |
| POC2 | 路由决策延迟 | jev-scorer + KV 缓存 | <1s（每任务轮） |
| POC3 | 判据信号回流 | verdict-collector | codex/claude code 能暴露编译/测试结果，或 router 代理够用 |
| POC6 | 任务轮判定准确率 | turn-detector | >95%（结构判定，应近确定） |

---

## 第四章 功能清单

| F# | 功能 | 优先级 | 对应组件 | 状态 |
|---|---|---|---|---|
| F1 | OpenAI /v1/chat/completions 端点接入 | P0 | gateway | [待 fact 校验：litellm proxy 端点模式] |
| F2 | Anthropic /v1/messages 端点接入 | P0 | gateway | [待 fact 校验] |
| F3 | 请求 schema 转换（OpenAI↔Anthropic↔内部统一表示） | P0 | gateway | [待 fact 校验：litellm 转换函数] |
| F4 | SSE 流式响应透传 + chunk 格式转换 | P0 | gateway | [待 fact 校验：SSE 格式] |
| F5 | 工具调用格式转换（tool_calls↔tool_use） | P0 | gateway | [待 fact 校验] |
| F6 | 任务轮判定（tool_result vs user message） | P0 | turn-detector | [待 POC6] |
| F7 | Jev 单 token 任务类型打分 | P0 | jev-scorer | [待 POC1/POC2] |
| F8 | KV 前缀缓存（policy+任务类型表） | P0 | jev-scorer | 抽取自 fast_browser_use |
| F9 | 任务类型→能力需求查表 | P0 | matcher | 种子表已定 |
| F10 | 声明能力向量挡死 | P0 | matcher | |
| F11 | 实测矩阵+成本加权选模 | P0 | selector | [G-R3：冷启动降级待定] |
| F12 | upstream 转发 + 工具循环继承 | P0 | forwarder | |
| F13 | 模型/upstream 注册管理 | P1 | registry-store | [待 fact 校验：litellm model registry] |
| F14 | 声明能力向量配置（人工预填） | P1 | registry-store | |
| F15 | 路由决策日志（task_turn_record） | P1 | registry-store | |
| F16 | 判据采集（编译/测试/代理指标） | P1 | verdict-collector | [待 POC3，G-R1] |
| F17 | LLM 侦察兵提议标签更新 | P2 | label-refiner | |
| F18 | 独立统计复核门 | P2 | label-refiner | I-DISC 防线 |
| F19 | 实测矩阵 candidate→active 升格 | P2 | registry-store | 固化双门 |
| F20 | 任务类型空层检测+演化 | P2 | turn-detector+registry | |
| F21 | 成本记账（per-task-turn 四字段） | P2 | registry-store | |
| F22 | model 名强 hint 直路由 | P1 | gateway | |
| F23 | X-Task-Hint 短路 | P1 | gateway | |

---

## 第五章 收敛路径（行业规范 → 最佳实践 → POC）

按 book 理论 P7 行业先验四层制 + L2/L3 候选 + L4 灰度：

1. **行业规范（硬约束/半强制）** [进行中]：recon 子代理抓 litellm 架构 + OpenAI/Anthropic API 规范 → 校验第二/三章 schema 与组件，排除不合规设计。
2. **最佳实践（参考性）** [待]：据 fact 收敛数据模型字段命名、网关端点模式、SSE 转换做法。
3. **POC（验证）** [Phase 1]：按 POC1/2/3/6 验证 Jev 范式可行性、延迟、判据回流、任务轮判定。
4. **收敛**：POC 过 → candidate→active，架构 v1.0。

---

## 附录 溯源

- 设计共识：`docs/fast-router-智能路由设计共识-v0.1.md`
- 范式研究：`docs/fast-browser-use-System1架构与可复用性研究.md`
- recon 调研（进行中）：recon-gateway（litellm）、recon-apispec（OpenAI/Anthropic API）
- 理论根：`book/ai-native组织理论.md`（P7 行业先验四层制、no-memory-citation、relock-after-switch）
