---
type: Architecture Design
title: fast-router 架构设计 v0.2 —— 业务/数据/技术/功能四视图（recon fact 校验后）
description: v0.1 草案经 litellm 1.102.1 源码 fact 校验后整合。复用 litellm Deployment/ModelInfo schema(能力向量用 ModelInfo.extra 扩展)+ 双端点(/v1/chat/completions + anthropic_endpoints /v1/messages)+ anthropic_interface 转换函数;路由策略不复用 litellm(其 QualityRouter 是 per-model 数据驱动评分),自研 Jev 任务类型分类 + 声明能力挡死 + 实测矩阵加权三层结构。OpenAI/Anthropic 官方规范已抓（openai 3.19.2 / anthropic 1.8.0 SDK schema，pydantic types 即官方规范代码化）。
source_consensus: docs/fast-router-智能路由设计共识-v0.1.md
source_recon: litellm 1.102.1 源码（pip download 解压，/tmp/litellm-probe，2026-09-25 抓取）
recon_sdk: openai 3.19.2 / anthropic 1.8.0 SDK 源码（pip download 解压，/tmp/sdk-probe，2026-09-25 抓取——pydantic/TypedDict types 即官方规范代码化）
status: v0.2，litellm + OpenAI/Anthropic SDK fact 均已校验；POC 待 Phase 1
---

# fast-router 架构设计 v0.2

## ——业务/数据/技术/功能四视图（recon fact 校验后）

> **三行说明**
> ① v0.1 草案的 [待 fact 校验] 已用 litellm 1.102.1 源码核实，本版为 fact 校验后整合。
> ② 关键架构决策：复用 litellm schema/端点/转换；路由策略不复用（litellm 是 per-model 数据评分，fast-router 是任务类型分类+声明挡死+实测加权三层）。
> ③ 诚实分级：litellm 实现 fact（“最佳实践”层）+ OpenAI/Anthropic SDK schema fact（“行业规范”层）均已抓，源码路径锚定。

---

## recon fact 摘要（litellm 1.102.1 源码，2026-09-25 抓取）

### model registry schema（types/router.py）
- `Deployment` = `{model_name, litellm_params: LiteLLM_Params, model_info: ModelInfo}`（核心部署单元，line 562）
- `LiteLLM_Params(GenericLiteLLMParams)` = `{model}` + 连接参数（CredentialLiteLLMParams: api_key/api_base/api_version）+ 定价（CustomPricingLiteLLMParams，line 441/308）
- `ModelInfo(MirroredPricingParams)` = `{id, base_model, tier, team_id, team_public_model_name, blocked, ptu_count, cost_per_ptu_per_hour, allow_fail_open, enable_tag_filtering, internal_router_model, updated_at/by, created_at/by}` + **`model_config = ConfigDict(extra="allow")`**（line 163）——**可扩展任意字段不改 schema**
- `MirroredPricingParams`（types/utils.py:3519）= `{input_cost_per_token, output_cost_per_token, input/output_cost_per_character, cache_read/creation_input_token_cost, tiered_pricing}`

### 端点（proxy/）
- `/v1/chat/completions`（OpenAI 格式，proxy/proxy_server.py:10947）
- `/v1/messages`（Anthropic 格式，**proxy/anthropic_endpoints/endpoints.py:94**——litellm 确实支持双端点，v0.1 草案判断修正）
- `passthrough/main.py` 支持端点原样透传
- `route_priority.py` 把 /v1/messages 列入路由优先级

### 转换层
- `anthropic_interface/messages/`：Anthropic 兼容转换模块
- litellm 内部统一表示 → 各 provider 格式（OpenAI/Anthropic/...）的转换已实现

### 路由策略（types/router.py:893 + router_strategy/）
- `RoutingStrategy` enum: `LEAST_BUSY / LATENCY_BASED / COST_BASED / USAGE_BASED_ROUTING / USAGE_BASED_ROUTING_V2 / PROVIDER_BUDGET_LIMITING` + `simple-shuffle / lar1`
- 新式智能路由（CustomListener hook）：
  - `QualityRouter`（router_strategy/quality_router/）——基于历史质量评分路由
  - `ComplexityRouter`——按请求复杂度路由
  - `AutoRouter` / `AdaptiveRouter`——自适应

### **关键差异化发现**
litellm 的智能路由（QualityRouter/ComplexityRouter）是**数据驱动的 per-model 评分**——根据历史质量/复杂度评分在 model group 内选模型。**它不做"任务类型分类"**，没有"声明能力向量挡死"，没有"Jev 单 token 本地打分"。fast-router 的三层结构（任务类型分类 + 声明能力挡死 + 实测矩阵加权）是 litellm 没有的——fast-router 复用 litellm 的 schema/端点/转换层，但路由策略层自研，不与 litellm 的 RoutingStrategy 重叠。

---

## 第一章 业务架构

### 1.1 业务参与者

| 参与者 | 角色 | 关键属性 |
|---|---|---|
| agent 客户端 | 路由服务消费者 | pi-agent / codex / claude code；发 OpenAI(/v1/chat/completions) 或 Anthropic(/v1/messages) 格式 |
| fast-router | 智能路由网关 | 本地单进程；Jev 任务类型打分 + 声明挡死 + 实测加权 + 转发 |
| upstream LLM 提供商 | 真实模型后端 | OpenAI / Anthropic / litellm 支持的其他 provider |
| goal-keeper（人类） | 不可委托内核 | upstream 凭据/定价签字、判据回流复核裁决、例外 HALT |

### 1.2 业务能力

| 能力 | 描述 | 对应组件 | 复用 litellm? |
|---|---|---|---|
| 智能路由 | 任务轮判断 + Jev 任务类型打分 + 声明挡死 + 实测加权 | turn-detector/jev-scorer/matcher/selector | 否（自研，litellm 路由策略不适用） |
| 模型注册 | upstream/model 元数据 + 能力向量 | registry-store | **schema 复用 litellm Deployment/ModelInfo** |
| 请求转发与格式转换 | 双端点接入 + schema 转换 + SSE 透传 + 工具循环继承 | gateway/forwarder | **复用 litellm anthropic_interface + proxy 端点** |
| 判据回流与校准 | 判据采集 + LLM 侦察兵 + 统计复核 | verdict-collector/label-refiner | 否（自研） |
| 成本记账 | per-task-turn 成本/延迟/判据 | registry-store | 复用 litellm spend logging 模式 |

### 1.3 业务用例

| UC# | 用例 | 主参与者 | 产出 |
|---|---|---|---|
| UC1 | 路由一个请求 | agent 客户端 | 选定 upstream+model，转发响应 |
| UC2 | 注册 upstream/model | goal-keeper | Deployment 条目（含能力向量） |
| UC3 | 上报判据 | agent/router 推断 | verdict_ledger 记录 |
| UC4 | 校准路由表 | goal-keeper | 实测矩阵 candidate→active |
| UC5 | 任务类型演化 | router 空层检测 | 任务类型增删（经统计复核） |

---

## 第二章 数据架构

### 2.1 实体清单（复用 litellm schema + 扩展）

| 实体 | litellm 对应 | 关键字段 | fast-router 扩展 |
|---|---|---|---|
| deployment | `Deployment`（types/router.py:562） | model_name, litellm_params, model_info | 无（直接复用） |
| model_info | `ModelInfo`（types/router.py:163） | id, base_model, tier, blocked, ... | **+ capability_vector**（用 `extra="allow"` 扩展，不改 schema） |
| pricing | `MirroredPricingParams`（types/utils.py:3519） | input/output_cost_per_token, cache costs | 无（直接复用） |
| task_type | （litellm 无——自研） | code, label, capability_requirement_vector, version, source | 自研实体 |
| measured_matrix | （litellm QualityRouter 有 quality_scores 但非此结构——自研） | task_type_id, model_id, pass_rate, sample_size, status | 自研实体 |
| task_turn_record | litellm spend log 类似 | turn_id, message_hash, task_type, chosen_model, scores, latency | 自研（参考 litellm spend logging） |
| verdict_ledger | （自研） | turn_id, verdict, signal_source | 自研 |
| label_update_ledger | （自研） | id, llm_proposal, statistical_review, applied | 自研 |

### 2.2 capability_vector schema（扩展到 ModelInfo.extra）

```yaml
# 挂在 ModelInfo 的 extra 字段（litellm extra="allow" 允许）
capability_vector:
  code: {score: 0.9, source: 声明, confidence: 0.5, recheck_condition: "版本变更"}
  reasoning: {score: 0.7, source: 声明, ...}
  long_context: {score: 1.0, ...}
  vision: {score: 0.0, ...}
  # ... 按 Q5 任务类型表的能力需求轴
```

**优势**：复用 litellm model registry 的 CRUD/管理 UI/导入导出，能力向量只是 ModelInfo 的扩展字段，不破坏 litellm 兼容性。

### 2.3 数据流

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

### 2.4 存储选型

- 起步：sqlite + 文件（单机，轻量）。
- model registry：复用 litellm 的 model registry 存储模式（litellm 支持 config 文件 + DB 两种，fast-router 起步用 config 文件 + sqlite）。

---

## 第三章 技术架构

### 3.1 组件分解（复用/自研分工明确）

| 组件 | 职责 | 来源 | 改造 |
|---|---|---|---|
| gateway | 双端点接入 | **复用 litellm proxy 端点**（/v1/chat/completions + /v1/messages via anthropic_endpoints） | 用 litellm proxy 作网关骨架，hook 进 fast-router 路由 |
| schema-convert | OpenAI↔Anthropic↔内部表示 | **复用 litellm anthropic_interface/messages** | 直接用，不自研转换 |
| sse-proxy | SSE 透传 | litellm proxy 已实现 | 直接用 |
| turn-detector | 任务轮判定（tool_result vs user message） | 自研 | message 末尾结构判定 |
| jev-scorer | Jev 单 token 任务类型打分 | 抽取自 fast_browser_use model.py | 取 candidate_codes + score + KV 前缀缓存 |
| matcher | 声明能力挡死 | 自研 | 任务类型→能力需求 ∩ capability_vector |
| selector | 实测矩阵+成本加权选模 | 自研 | 加权综合分 + tie-break |
| forwarder | upstream 转发 + 工具循环继承 | litellm router 转发 | session 内工具循环继承（litellm 默认 per-request，fast-router 加任务轮继承层） |
| verdict-collector | 判据采集 | 自研 | [待 POC3，G-R1] |
| label-refiner | LLM 侦察兵 + 统计复核 | 自研 | I-DISC 防线 |
| registry-store | 模型/任务类型/矩阵 | litellm model registry + sqlite 扩展 | capability_vector 挂 ModelInfo.extra |

### 3.2 技术栈

| 层 | 选型 | fact 依据 |
|---|---|---|
| 语言 | Python | litellm / mlx_lm / transformers 生态 |
| 网关骨架 | **litellm proxy**（复用端点+转换+SSE） | litellm 1.102.1 已实现双端点+转换 |
| Jev 推理 | MLX / PyTorch（复用 fast_browser_use 后端） | fast_browser_use 已验证 |
| 路由策略 | 自研（不复用 litellm RoutingStrategy） | litellm 是 per-model 评分，fast-router 是任务类型分类 |
| 存储 | litellm model registry + sqlite | litellm 已有 model registry 存储 |

### 3.3 部署

- 单机单进程：litellm proxy（网关）+ fast-router 路由 hook + 9B 常驻（~5-6GB）[待确认 Mac 内存]。
- 对外：localhost，litellm proxy 双端点。
- 客户端配 base_url 指向 localhost，model 名任意。

### 3.4 架构修正（vs v0.1 草案）

| v0.1 草案假设 | v0.2 fact 校验后修正 |
|---|---|
| "litellm 不直接提供 Anthropic /v1/messages 接收端点，fast-router 自建" | **修正**：litellm 有 `anthropic_endpoints/endpoints.py:94 /v1/messages`，直接复用，不自建 |
| "网关层复用 litellm schema 转换子模块" | 确认：复用 `anthropic_interface/messages`，且复用 litellm proxy 端点骨架（不止转换函数） |
| "路由策略未定是否复用 litellm" | **明确不复用**：litellm RoutingStrategy 是 per-model 评分；fast-router 三层结构自研 |
| "model registry schema 待 fact 校验" | 确认：复用 Deployment/ModelInfo，capability_vector 用 extra="allow" 扩展 |

---

## 第四章 功能清单

| F# | 功能 | 优先级 | 组件 | 复用/自研 | 状态 |
|---|---|---|---|---|---|
| F1 | /v1/chat/completions 端点 | P0 | gateway | 复用 litellm proxy | ✅ fact 确认 |
| F2 | /v1/messages 端点 | P0 | gateway | 复用 litellm anthropic_endpoints | ✅ fact 确认 |
| F3 | 请求 schema 转换 | P0 | schema-convert | 复用 anthropic_interface | ✅ fact 确认 |
| F4 | SSE 流式透传 + chunk 转换 | P0 | sse-proxy | 复用 litellm proxy | ✅ fact 确认（OpenAI chunk.delta ↔ Anthropic 6 种 raw event） |
| F5 | 工具调用格式转换 | P0 | schema-convert | 复用 anthropic_interface | ✅ fact 确认（OpenAI tool_calls{function} ↔ Anthropic ToolUseBlock/ToolResultBlock） |
| F6 | 任务轮判定 | P0 | turn-detector | 自研 | [待 POC6] |
| F7 | Jev 单 token 任务类型打分 | P0 | jev-scorer | 自研（抽取 fast_browser_use） | [待 POC1/POC2] |
| F8 | KV 前缀缓存 | P0 | jev-scorer | 抽取自 fast_browser_use | |
| F9 | 任务类型→能力需求查表 | P0 | matcher | 自研 | 种子表已定 |
| F10 | 声明能力挡死 | P0 | matcher | 自研 | |
| F11 | 实测矩阵+成本加权选模 | P0 | selector | 自研 | [G-R3 冷启动降级] |
| F12 | upstream 转发+工具循环继承 | P0 | forwarder | litellm 转发 + 自研继承层 | |
| F13 | 模型注册管理 | P1 | registry-store | 复用 litellm model registry | ✅ fact 确认 |
| F14 | capability_vector 配置 | P1 | registry-store | 扩展 ModelInfo.extra | ✅ fact 确认可行 |
| F15 | 路由决策日志 | P1 | registry-store | 参考 litellm spend logging | |
| F16 | 判据采集 | P1 | verdict-collector | 自研 | [待 POC3，G-R1] |
| F17 | LLM 侦察兵提议 | P2 | label-refiner | 自研 | |
| F18 | 独立统计复核门 | P2 | label-refiner | 自研 | I-DISC 防线 |
| F19 | 实测矩阵 candidate→active | P2 | registry-store | 自研 | 固化双门 |
| F20 | 任务类型空层检测+演化 | P2 | turn-detector+registry | 自研 | |
| F21 | 成本记账 | P2 | registry-store | 参考 litellm spend | |
| F22 | model 名强 hint 直路由 | P1 | gateway | 自研 hook | |
| F23 | X-Task-Hint 短路 | P1 | gateway | 自研 hook | |

**复用率**：F1-F5/F13-F15/F21 共 8 项复用 litellm；F6-F12/F16-F20/F22-F23 共 14 项自研。网关层基本复用，路由层全自研。

---

## 第五章 fast-router vs litellm 差异化（关键）

| 维度 | litellm | fast-router |
|---|---|---|
| 路由依据 | per-model 历史评分（QualityRouter/ComplexityRouter）或规则（cost/latency/usage） | **任务类型分类（Jev 单 token）+ 声明能力挡死 + 实测矩阵加权** |
| 任务类型概念 | 无 | 有（10 类种子，可演化） |
| 声明能力向量 | 无 | 有（挡死不匹配模型） |
| 路由决策位置 | 规则/云端数据驱动 | **本地 Jev 单 token 打分** |
| 校准机制 | QualityRouter 用历史质量分 | LLM 侦察兵 + 独立统计复核（I-DISC 防线） |

**结论**：fast-router 不是 litellm 的替代，是 litellm 网关之上的**路由策略替换**——复用 litellm 的网关/schema/端点/SSE/模型注册（成熟稳定），只把路由策略层从 litellm 的 per-model 评分替换为任务类型分类+声明挡死+实测加权的三层结构。差异化在路由策略，不在网关。

---

## 第六章 POC 计划（对应设计共识 P1-P6）

| POC# | 验证 | 组件 | 通过判据 | 依赖 |
|---|---|---|---|---|
| POC6 | 任务轮判定准确率 | turn-detector | >95%（结构判定） | 无，可先做 |
| POC1 | Jev 任务类型分类准确率 | jev-scorer | >70%（Qwen3.5-9B，10 类，首 1024 token） | 抽取 fast_browser_use 核心 |
| POC2 | 路由决策延迟 | jev-scorer + KV 缓存 | <1s（每任务轮） | POC1 |
| POC3 | 判据信号回流 | verdict-collector | codex/claude code 暴露编译/测试结果，或 router 代理够用 | G-R1 |
| POC4 | 冷启动降级 | selector | 挡死后选最便宜不显著降质 | POC1 |

**POC 序**：POC6（结构判定，零依赖）→ POC1（范式验证）→ POC2（延迟）→ POC4（冷启动）→ POC3（判据回流，最大未知）。

---

## 第七章 诚实边界

1. **litellm fact 已抓（最佳实践层）**：model registry schema / 端点 / 转换层 / 路由策略，源码路径锚定（/tmp/litellm-probe，litellm 1.102.1，2026-09-25）。
2. **OpenAI/Anthropic 官方规范已抓（行业规范层）**：openai 3.19.2 / anthropic 1.8.0 SDK 的 pydantic/TypedDict types 即官方规范代码化，SSE/工具调用/响应 schema 已锚定（见附录 schema 差异表）。litellm 实现与其一致。
3. **litellm 版本绑定**：fact 基于 1.102.1，litellm 迭代快，schema/端点可能变；capability_vector 用 extra="allow" 扩展降低耦合，但须 version pin。
4. **实例化非验证**：架构设计完成不等于可运行；POC1-6 须 Phase 1 实测。
5. **差异化未经市场验证**：fast-router 三层路由是否优于 litellm QualityRouter，未实测对比——这是 Phase 2 议题。
6. **I-DISC 部分满足**：recon fact 由单一研究者抓取（我），未派 fresh 审查员复核源码解读；进可信决策须另起审查。

---

## 附录 溯源

- 设计共识：`docs/fast-router-智能路由设计共识-v0.1.md`
- v0.1 草案：`docs/fast-router-架构设计-v0.1.md`
- litellm 源码（fact 来源）：litellm 1.102.1，pip download 解压于 /tmp/litellm-probe/litellm-src/
  - model registry: `litellm/types/router.py`（Deployment:562, ModelInfo:163, LiteLLM_Params:441, RoutingStrategy:893）
  - 定价: `litellm/types/utils.py:3519`（MirroredPricingParams）
  - 端点: `litellm/proxy/proxy_server.py:10947`（/v1/chat/completions）, `litellm/proxy/anthropic_endpoints/endpoints.py:94`（/v1/messages）
  - 转换: `litellm/anthropic_interface/messages/`
  - 路由策略: `litellm/router_strategy/`（quality_router/auto_router/complexity_router/adaptive_router）
- 范式研究：`docs/fast-browser-use-System1架构与可复用性研究.md`
- 理论根：`book/ai-native组织理论.md`（P7 行业先验四层制、no-memory-citation、relock-after-switch）
- 未达 fact（剩余缺口）：无——litellm 实现层 + OpenAI/Anthropic 官方 schema 层均已抓。仅 I-DISC 缺口（源码解读由单一研究者，未 fresh 复核）。

## 附录 B：OpenAI vs Anthropic schema 差异表（网关转换核心 fact）

Fact 来源：openai 3.19.2 SDK（`/tmp/sdk-probe/openai-3.19.2-*/openai/types/chat/`）+ anthropic 1.8.0 SDK（`/tmp/sdk-probe/anthropic-1.8.0-*/anthropic/types/`）。

| 维度 | OpenAI | Anthropic | 转换要点 |
|---|---|---|---|
| 响应结构 | choices[].message + finish_reason | content: List[ContentBlock] + stop_reason | 单 message ↔ content block 列表 |
| stop 原因 | finish_reason: stop/length/tool_calls/content_filter/function_call | stop_reason (StopReason enum) | tool_calls ↔ tool_use |
| SSE 格式 | 单一 chunk: choices[].delta（增量 content/tool_calls） | 6 种 raw event: message_start/content_block_start/content_block_delta/content_block_stop/message_delta/message_stop | chunk 流 ↔ event 状态机转换 |
| 工具调用(产出) | message.tool_calls[]: {id, function:{name, arguments}, type:"function"} | content block: ToolUseBlock{id, name, input, type:"tool_use"} | function.arguments(JSON str) ↔ input(Dict) |
| 工具结果(输入) | role:"tool" message + tool_call_id | content block: ToolResultBlock{tool_use_id, content, is_error, type:"tool_result"} | tool message ↔ tool_result block |
| system | messages 内 role:"system" | 顶层 system 字段（非 messages 内） | 拆/合 messages |

**网关转换实现要点**（复用 litellm anthropic_interface）：
- 请求方向：OpenAI client 发 messages[{role:system,...}] → 转 Anthropic 顶层 system + messages；反向同理。
- 响应方向：Anthropic content blocks → 转 OpenAI choices[].message（tool_use block → tool_calls）。
- SSE 方向：Anthropic event 状态机（message_start→content_block_start→content_block_delta*→content_block_stop→message_delta→message_stop）→ OpenAI chunk 流（每 delta 一个 chunk，最后 chunk 带 finish_reason）。
- 工具结果：OpenAI tool message → Anthropic user message 内 tool_result block。
