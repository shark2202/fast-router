---
type: Design Specification
title: fast-router 智能路由设计共识 v0.1 —— 基于 Jev 范式的本地 LLM 智能路由器
description: 经 grilling 质询达成的设计共识落盘。本地 LLM 网关,对外暴露固定模型名的 OpenAI/Anthropic 双端点兼容 API,用 Jev 式单 token 打分（Qwen3.5-9B）对每个任务轮（新 user message）做任务类型分类，匹配声明能力向量挡死 + 实测矩阵加权选最优 upstream+model，工具循环内继承路由。判据回流经独立统计复核校准标签(LLM 侦察兵不法官)。含设计三元组、DecisionRecord、政策继承块、已知未闭合项与诚实边界。v0.1 设计前置审查门产物,实例化非验证。
source_grilling: 本会话 grilling 决策树（Q1-Q10）
source_paradigm: docs/fast-browser-use-System1架构与可复用性研究.md
theory_version: "正文 v1.0（修订九，2026-09）"
timestamp: 2026-09-25T00:45:00+08:00
status: 设计共识 v0.1，待 Phase 1 试点校准
applicability: 单机单用户本地网关，给 pi-agent/codex/claude code 等 agent 框架做智能路由
negative_example: 不可据此断言路由分数是校准过的正确性概率；Jev 分类器用 Qwen 打分是范式固有未校准，靠判据回流校准
---

# fast-router 智能路由设计共识

## ——基于 Jev 范式的本地 LLM 智能路由器 v0.1

> **三行说明**
> ① 给人类（goal-keeper）：你在三个触点出场——不可逆实例化签字（upstream 凭据/定价承诺）、判据回流复核裁决、例外路由/HALT。其余交给网关与 Jev 分类器。
> ② 给实现：网关层复用 litellm schema 转换；Jev 核心抽取自 fast_browser_use（candidate_codes + score + KV 前缀缓存）；回填回路自研，LLM 只提议不裁决。
> ③ 状态：v0.1 设计前置审查门产物，实例化非验证——尚无一次运行证据，Phase 1 首刀（代码生成）走全流程即第一批读数。

---

## 出版说明

本设计是 [fast_browser_use System 1 架构研究](./fast-browser-use-System1架构与可复用性研究.md) 的落地——把 Jev 的"单 token 候选打分"范式从浏览器动作选择迁移到 LLM 路由决策。经一轮 grilling 质询（Q1-Q10）收敛决策树主干后落盘。

**诚实边界**：①实例化非验证，无一次完整运行证据；②Jev 分类器用 Qwen 打分是范式固有未校准（类比 fast_browser_use 自承“分数非校准正确性概率”），靠判据回流 + 独立统计复核校准；③本设计为单模型家族自检（grilling 由单一研究者产出），未派 fresh 无上下文审查员复核（I-DISC 部分满足）；④内存假设 Mac ≥16GB 未确认；⑤第一刀代码生成的判据回流机制未闭合（G-R1）。

**Q7 修订（2026-09-25）**：原“session 锁定”被推翻——一个 session 内任务可能切换（先代码后翻译），锁死模型会错配。修正为“任务轮判断”：新 user message 触发 Jev 重判，工具循环（message 末尾 tool_result）继承当前任务轮路由。连带：Q6 输入改为“当前新 user message 首 N token”（不看历史，不受对话膨胀影响）；延迟从“session 首调用 1 次”变为“每任务轮 1 次”（Q9 < 1s 预算不变，触发频率变）。

---

## 第一章 设计共识（决策树回顾）

| Q | 决策点 | 收敛结论 |
|---|---|---|
| Q1 | 目标函数 | 多维组合：声明能力**挡死** + 实测矩阵+成本**加权优化**（词典序两层） |
| Q2 | 效果度量 | (e) 任务自带可判据（硬）+ 代理指标（软），**不用 LLM-as-judge**，第一刀切**代码生成** |
| Q3 | 标签来源 | (d) 人工预填种子 + 数据回归回填 |
| Q3.1 | LLM 回填角色 | (v) **侦察兵不是法官**——LLM 提议须经独立统计复核才生效 |
| Q4 | 标签结构 | (d) 声明能力向量（挡死）+ 实测矩阵（优化）分层 |
| Q5 | 能力轴/任务类型 | (c)+(d) 混合演化+空层检测+规模纪律；**10 个种子任务类型** |
| Q6 | 输入信号 | (c) 可选 hint 短路 + **当前新 user message** 首 N token（不看历史）；**单机单用户** |
| Q7 | 触发时机 | (ii) **任务轮判断**：新 user message 触发 Jev 重判，工具循环（末尾 tool_result）继承；不锁 session |
| Q8 | Jev 候选 | (b) Jev 打**任务类型**（10 个 ≤256 可行），模型匹配在 Jev 外 |
| Q9 | 分类器+代码 | (d) 9B 起步验证范式→优化期降级小模型；**抽取核心重写**不整体 fork；<1s 延迟（每任务轮 1 次） |
| Q10 | 网关层 | (c) 复用 litellm schema 转换 + 自研路由控制流；**双端点**；model 名可选强 hint |

---

## 第二章 设计三元组

### A. 闭包分解图

| 闭包# | 责任（做完整即可判定） | 引用 AC | 依赖闭包 | 接口最小化声明 | 可逆性(R/I) |
|---|---|---|---|---|---|
| C1 网关接入 | 双端点接收 + litellm schema 转内部统一表示 | AC2 | 无 | OpenAI/Anthropic 请求 schema → 内部 MessageSet | R（纯转换，可重放） |
| C2 任务轮判定 | 判定请求末尾是 tool_result（工具循环，继承路由）还是新 user message（新任务轮，触发 Jev） | AC3 | C1 | (is_new_turn, inherited_route?) | R |
| C3 Jev 路由决策 | 首 N token + policy + 任务类型表 → 单 token softmax 选任务类型 | AC4 | C2 | task_type ∈ {A..J} | R（可重路由） |
| C4 挡死匹配 | 任务类型→能力需求向量；∩ 模型声明能力向量 → 排除不合格 | AC5 | C3 | candidate_models[] | R |
| C5 加权选择 | 剩余候选 × 实测矩阵 × 成本 → 选最优 upstream+model | AC6 | C4 | (upstream, model) | R |
| C6 转发+循环继承 | 任务轮路由选定 upstream+model；工具循环内继承；转发请求；SSE 透传+格式转换 | AC7 | C5 | SSE stream（客户端格式） | R（任务轮可重判） |
| C7 判据回流 | 采集路由结果 + 判据信号（编译/测试通过；代理指标） | AC8 | C6 | (task_turn, model, verdict) | I（判据是历史事实，不可改） |
| C8 标签回填 | LLM 侦察兵提议 → 独立统计复核 → 实测矩阵更新 | AC9 | C7 | matrix_update(candidate/active) | R（candidate→active 双门） |
| C9 任务类型演化 | 空层检测 → 任务类型增删（经统计复核） | AC10 | C8 | task_type_set_delta | R（带版本回滚） |

**闭包切分判据**：每个闭包"做完整即可判定"——C3 选完任务类型即可判定（不依赖 C4-C9），C4 挡死完即可判定合格候选集，etc。接口最小化：闭包间只传标量/小结构，不传上下文。

**任务轮路由控制流**（Q7 修正后）：
```
请求进入 (C1 网关: OpenAI/Anthropic 双端点 → 内部统一表示)
  │
  ▼
C2 任务轮判定: message 末尾?
  ├─ tool_result (工具循环) → 继承当前任务轮路由 → C6 转发
  └─ 新 user message (新任务轮) ↓
      ├─ model 名带 upstream 前缀? → 强 hint 直路由 → C6
      ├─ X-Task-Hint? → 短路跳过 Jev → C6
      └─ Jev 单 token 打分 (C3):
            prefill [policy+任务类型表 KV缓存] + [新 message 首 N token]
            → softmax over {A..J} → 选任务类型
      ▼
      C4 挡死: 任务类型→能力需求 ∩ 模型声明能力 → 排除不合格
      ▼
      C5 加权: 剩余候选 × 实测矩阵 × 成本 → 选最优 upstream+model
      ▼
      C6 转发+循环继承 → 转发 upstream → SSE 透传+格式转换
      ▼
      (该任务轮的工具循环用此模型; 下一任务轮独立重判)

后台回路: C7 判据回流 → C8 LLM 侦察兵+独立统计复核 → 实测矩阵更新 → C9 空层检测→任务类型演化
```

### B. 界面合同

**API 端点契约**（双端点，litellm 标准）：
```
POST /v1/chat/completions   (OpenAI 格式, 给 codex/pi-agent)
POST /v1/messages           (Anthropic 格式, 给 claude code)
```
- 接受任意 model 名（最无感）。
- model 名带 upstream 前缀（如 `anthropic/claude-sonnet`）= 强 hint，直路由跳过 Jev。
- 可选 header：`X-Task-Hint`（任务类型短路，跳过 Jev）。
- 流式 SSE 透传：选定 upstream 后透传其 SSE，按客户端格式转换 chunk（OpenAI delta ↔ Anthropic content_block_delta）。

**任务轮契约**（Q7 修正：不锁 session，按任务轮判断）：
- 任务轮判定：请求 message 末尾是 tool_result → 工具循环内，继承当前任务轮路由（不重判）；末尾是新 user message → 新任务轮，触发 Jev 重判。
- 路由作用域：路由选定 upstream+model 作用于“当前任务轮”（含其工具循环），下一任务轮独立重判。
- 边缘兜底：新 user message 过短/判不出任务类型（如“继续”）→ 继承上一任务轮路由。
- 复路由：工具循环内不做（保上下文一致）；任务轮间天然复判（每轮独立）。

**判据回流契约**（G-R1 未闭合，挂账）：
- 代码生成判据：编译/测试通过（二值）。信号来源待定——agent 客户端上报 hook vs router 推断（观察后续请求是否报错）。
- 代理指标：重试率/编辑率。客户端侧信号，router 不可直接观测——需客户端上报或用 router 可观测代理（同 session 立即重试率、报错率）。

### C. 实施分层

| 层 | 来源 | 改造 |
|---|---|---|
| 网关层（C1/C6） | 复用 litellm schema 转换子模块 | 不整体引入 litellm，只取 message/tool/stream 格式转换函数 |
| Jev 核心（C3） | 抽取自 fast_browser_use model.py | 取 candidate_codes + LocalModel.score + KV 前缀缓存逻辑；解"浏览器动作空间"耦合（换任务类型表）；其余 5 处耦合优化期解耦 |
| 控制流（C2/C4/C5/C7-C9） | 自研 | 任务轮判定/挡死/加权/回流/演化 |

---

## 第三章 DecisionRecord

**决策：用 Jev 单 token 任务类型打分做本地 LLM 智能路由**

**维度（成功怎么判）**：
- 范式可行性：Jev 10 类任务分类在路由场景能 work（每任务轮准确率可观测）
- 校准度：路由分数经判据回流校准后与真实质量正相关
- 延迟：每任务轮路由决策 < 1s（session 内 N 条用户消息 = N 次）
- 内存：9B 常驻与 agent 客户端不冲突

**死法（概率×代价，词典序）**：
| # | 死法 | 机理 | 对策 |
|---|---|---|---|
| D1 | 候选通胀超 256 | 任务类型演化失控超 Jev 单 token 上限 | Q5 规模纪律：初始 10，上限 ~30，新增须经统计复核 |
| D2 | 任务类型虚假完备 | 维度表漏一整类任务，路由系统性错配 | Q5 空层检测 + 自下而上涌现修正（book 0.2/40.4） |
| D3 | 回填回声室（I-DISC 违反） | LLM 既提议标签又评判质量，误差相关放大 | Q3.1 (v)：LLM 只提议，独立统计复核才生效 |
| D4 | 判据信号回流失败 | codex/claude code 不暴露编译/测试结果，C7 拿不到 ground truth | G-R1 挂账；冷启动用 router 可观测代理（重试率/报错率）兜底 |
| D5 | 任务轮判定错误 | 误判 tool_result 续轮为新任务轮（工具循环中断）或误判新消息为续轮（不重判 → 错配） | message 结构判定可测；边缘兜底继承上一路由 |
| D6 | **协调者失效**（router 自身状态腐化） | router 缓存/session 表/实测矩阵状态腐化，路由决策基于脏数据 | 状态外置（进程可死状态不可死，book 过程管控协议）；DecisionRecord 留档；会话自举协议恢复 | 
| D7 | 声明能力标签过期 | 模型版本迭代，声明级标签失真 | Q3 (d)：标签带 recheck_condition，版本变更触发重测 |
| D8 | 9B 内存与 agent 冲突 | pi-agent 跑本地大模型时内存竞争 | 假设 Mac ≥16GB；优化期降级小模型 |

**可逆性**：本体可逆（任务轮路由可重判、标签 candidate→active 双门、配置 flag-with-ttl）；工具循环内模型切换会丢上下文一致性——故工具循环内强制继承路由，只任务轮间重判。

**事前 Prediction（可证伪）**：
- P1：Jev 10 类任务分类首调用准确率 > 70%（Qwen3.5-9B，首 1024 token）
- P2：路由决策延迟 < 1s（9B MLX 4-bit，1024 token 请求，policy+表 KV 缓存命中）
- P3：第一刀代码生成的判据信号回流能 work（codex/claude code 暴露编译/测试结果，或 router 可观测代理够用）
- P4：冷启动实测矩阵为空时，挡死后选最便宜不会显著降低质量（靠声明能力挡死够用）
- P5：回填回路 LLM 侦察兵提议经独立统计复核，首期复核通过率 < 50%（LLM 提议多有偏置）
- P6：任务轮判定（tool_result 续轮 vs 新 user message）准确率 > 95%（结构判定，应近确定）

---运行后回填（Phase 1 后）---
| 预测 | 结果 | Surprise |

**事故与降级记录**：Phase 1 运行后留痕。

### Q7 修订 DecisionRecord（变更门留档，2026-09-25）

**变更**：路由作用域从"session 锁定"改为"任务轮判断"。

**三问**：
- 维度：上下文一致性（工具循环需连贯） vs 任务切换适配（session 内任务可能变）。原 session 锁定只保前者，牺牲后者。
- 死法：原方案死法 = session 内任务切换后仍用首任务模型 → 错配（如代码任务后接翻译仍用代码模型）。代价 = 路由质量下降，session 越长越严重。
- 可逆性：本体可逆（路由作用域是配置，可回退）；已扩散引用 = 无（v0.1 未实现，纯设计变更）。

**预测**：
- P6（新增）：任务轮判定（tool_result 续轮 vs 新 user message）准确率 > 95%——结构判定，应近确定。
- P1 修正：Jev 分类准确率现在按"每任务轮"统计（原"首调用"），样本量增大，校准更快。
- 延迟代价：session 内 N 条用户消息 = N 次 Jev 打分（原 1 次）。每次 < 1s，分摊在各请求上，agent 用户可接受。

**可逆性**：若任务轮判定在实测中 P6 < 95%（误判 tool_result 为新消息导致工具循环中断），回退方案 = 退回 session 锁定 + 显式 X-Session-Id 手动控制。

---

## 第四章 政策继承块（子组织自动继承）

**基础纪律**（抄开发局 §6.1）：三问决策前置；一切产出落盘；只认产物不认对话；先想清楚再动手；禁止不可逆破坏性操作；存在性断言标读取时点。

```yaml
policies:
  # 路由决策
  - id: single-token-candidate-cap        # Jev 候选上限
    required: [任务类型集合 ≤ 256，初始 10，上限 ~30]
  - id: task-turn-routing                 # 任务轮路由（Q7 修正：不锁 session）
    required: [新 user message 触发 Jev 重判；工具循环（message 末尾 tool_result）继承当前任务轮路由；不按 session 锁定]
  - id: task-type-evolution-gate          # 任务类型演化门
    required: [新增任务类型须经统计复核；空层检测触发补充探索]
  # 标签与校准
  - id: declare-then-calibrate            # 声明挡死 + 实测优化
    required: [声明能力向量挡死不合格；实测矩阵+成本加权选最优；声明级标签降权]
  - id: no-llm-as-judge                   # 禁 LLM 当质量法官
    forbidden: [LLM 直接产出路由结果的质量分]
  - id: llm-scout-not-judge              # LLM 侦察兵不法官
    required: [回填链路 LLM 只提议标签更新；生效须经不依赖 LLM 的独立统计复核]
  - id: label-recheck                     # 标签有效期
    required: [能力标签带 recheck_condition；版本变更/到期触发重测]
  # 判据与隐私
  - id: ground-truth-from-verdict         # ground truth 来源
    required: [质量信号只来自客观判据（编译/测试/解析）+ 数值代理指标；不经 LLM 评判]
  - id: local-only-privacy               # 本地隐私
    required: [单机单用户；prompt 不出本机；多租户扩展须重评跨用户隔离]
  # 根锚与防篡
  - id: orchestrator-failure-drill       # 协调者失效预演
    required: [DecisionRecord 死法栏含 router 自身失效项；状态外置可恢复]
  - id: existence-claims-timestamped
    required: [存在性断言（如"系统已有 X 模型标签"）标注读取时点]
```

---

## 第五章 已知未闭合项（挂账）

| # | 缺口 | 为什么未决 | 解除条件 |
|---|---|---|---|
| G-R1 | 判据信号回流机制 | codex/claude code 怎么暴露编译/测试结果给 router 未定 | Phase 1 首刀实测，选 hook 上报或 router 推断 |
| G-R2 | 代理指标观测 | 重试率/编辑率在客户端侧，router 看不到 | 客户端上报 hook 或用 router 可观测代理兜底 |
| G-R3 | 实测矩阵冷启动降级 | 初始空，加权优化退化策略未定 | 定"挡死后选最便宜/默认"降级规则 |
| G-R4 | 成本数据来源 | 各 upstream per-token 价格来源未定 | 手动配置 vs API 抓取 |
| G-R5 | 小模型降级阈值 | 何时从 9B 降到小模型未定 | P1 准确率达标 + 延迟/内存压力触发 |
| G-R6 | 多租户扩展 | 当前单机单用户 | 若需团队用，重评跨用户隔离/计费 |
| G-R7 | I-DISC 完整满足 | Jev 分类器用 Qwen 打分是范式固有未校准；回填回路有统计复核但分类器本身无 | 判据回流长期校准 + 异模型复核分类器 |

---

## 第六章 诚实边界

1. **实例化非验证**：v0.1 设计前置审查门产物，无一次完整运行证据，Phase 1 首刀即第一批读数。
2. **Jev 分数未校准**：分类器用 Qwen in-context 指令冒充分类器，分数是候选归一化相对偏好，非校准正确性概率（类比 fast_browser_use 自承）。靠判据回流 + 独立统计复核校准。
3. **I-DISC 部分满足**：回填回路有独立统计复核（LLM 侦察兵不法官）；但 Jev 分类器本身用 Qwen 打分，判别器是 LLM——范式固有未校准，只能靠判据回流长期校准 + 异模型复核（G-R7）。
4. **单模型家族自检**：本设计由 grilling 单一研究者产出，未派 fresh 无上下文审查员复核。进可信决策须另起审查。
5. **内存假设**：Mac ≥16GB 未确认；若 <16GB，9B 起步阶段要改（直接上小模型+规则）。
6. **第一刀判据回流未闭合**：G-R1 是 Phase 1 首刀的最大未知——若 codex/claude code 无法暴露编译/测试结果，(e) 判据度量退化为代理指标兜底，校准质量下降。
7. **不做硬编码计数**：所有数字（准确率阈值/延迟/内存）以 Phase 1 注册表为准，预测值 P1-P6 是事前锚点不是承诺。

---

## 第七章 实施路线

- **Phase 0（本文件）**：设计共识落盘（grilling 决策树 → 设计三元组 + DecisionRecord + 政策）。设计前置审查门产物。
- **Phase 1**：第一刀代码生成走全流程 → 首组校准数据（P1-P6 预测对照）→ 路由表/门禁强度重估 → v0.2。
  - 试点闭包序：C1 网关 → C3 Jev 核心（抽取 fast_browser_use）→ C4/C5 挡死+加权 → C2 任务轮判定 → C6 转发+循环继承 → C7 判据回流 → C8 回填 → C9 演化。
  - 每闭包独立可判定（AC），过门进下一闭包（节奏解耦防 churn）。
- **Phase 2**：据校准降级小模型（G-R5）；扩任务类型（C9 演化）；多租户评估（G-R6）。

---

## 附录 溯源

- 范式来源：`docs/fast-browser-use-System1架构与可复用性研究.md`（fast_browser_use 的 Jev System 1 机制逆向 + 可复用性评估）
- 上游代码：`/Users/mac/codes/fast-browser-use/fast_browser_use/model.py`（candidate_codes + LocalModel.score + KV 前缀缓存，抽取目标）
- 网关参考：litellm（schema 转换子模块复用目标）
- 理论根：`book/ai-native组织理论.md`（方法论总纲三问、I-DISC、Calibration、判据工程、过程管控协议）
- 域规范：`book/基于ai-native的开发组织的元组织.md`（开发局设计三元组 + 设计前置审查门 + DecisionRecord 模板）
- 设计过程：本会话 grilling（Q1-Q10 决策树）
