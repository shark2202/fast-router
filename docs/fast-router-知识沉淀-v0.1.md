---
type: Knowledge Sedimentation
title: fast-router 知识沉淀 v0.1 —— 5 阶端到端系统化落盘
description: 按 book/基于ai-native的知识沉淀流程规范.md 的 5 阶端到端流程（产生→验证→沉淀→应用→升级），系统化沉淀 fast-router 项目从 Python POC 到 Go 生产版的全部经验。架构和引擎已固定（Go+purego+zig+llama.cpp+Jev API+路由链+schema+admin+打包），P1 依赖具体模型能力（不阻塞架构/引擎）。含 patterns 层（跨项目可复用经验）+ 回放层（DecisionRecord 校准）+ 注册表（成本四字段）。
source: fast-router 全项目（13d9f31..317e329，17 commit）
theory: book/基于ai-native的知识沉淀流程规范.md
timestamp: 2026-09-25T20:00:00+08:00
status: 知识沉淀 v0.1，架构/引擎固定，P1 待模型验证
---

# fast-router 知识沉淀 v0.1

## ——5 阶端到端系统化落盘

> **三行说明**
> ① 架构和引擎已固定——Go+purego+zig+llama.cpp+Jev API+路由链+schema+admin+打包+SOP，17 commit 39 测试 6 平台验证。
> ② P1 依赖具体模型能力（Ornith-1.5-9B + per-candidate yes/no 待验证），不阻塞架构/引擎/流程。
> ③ 按 book 5 阶端到端系统化沉淀：产生（实测数据）→ 验证（固化判定）→ 沉淀（双账本）→ 应用（复用指引）→ 升级（recheck）。

---

## 一、产生（运行产出 + DecisionRecord 校准）

### 1.1 项目路径回放

```
Python POC（验证范式）
  → fast_browser_use 研究（Jev System 1 机制逆向）
  → grilling Q1-Q10 设计共识（10 决策 + Q7 修正）
  → 架构设计 v0.2（四视图 + Jev API spec + schema 差异表）
  → POC：0.5B P1=10% / 1.5B P1=47%（Qwen2.5 同系列趋势）
Go 重写（生产化）
  → C2/C4/C5 纯 Go 移植（30 测试）
  → C3 scorer（Jev system-one API + Choice）
  → zig wrapper（@cImport llama.h，消解 struct ABI）
  → C3 Backend（purego Unix + syscall Windows，不用 cgo）
  → gateway（双端点 + 路由链 + 转发）
  → schema 转换（OpenAI↔Anthropic 请求/响应/SSE/工具调用）
  → session 状态（工具循环继承）
  → admin web UI（upstreams + registry + 模型下载）
  → config 持久化 + --config
  → pack.sh + SOP/build.md
  → 6 平台交叉编译（8-9MB，CGO_ENABLED=0）
  → 端到端验证（hint-only + Jev 智能路由 + 工具调用 + SSE）
研究（方法改进）
  → LLM2Jev 研究（per-candidate yes/no vs 单 token）
  → Ornith-1.5-9B 研究（理想模型候选）
  → 8B 实测 P1=6.7%（Qwen3 跨系列 + thinking 不兼容）
  → fr_score_yesno + fr_apply_template 已加到 zig（待 Go 集成）
```

### 1.2 DecisionRecord 校准（预测 × 结果 × Surprise）

| 预测 | 事前 | 实测 | 结果 | Surprise（意外新知） |
|---|---|---|---|---|
| P6 任务轮判定 >95% | 0.85 | 100%（11/11） | 命中 | 无——结构判定确定性高 |
| P1 Jev 分类 >70%（0.5B） | 0.7 | 10% | 翻车 | 0.5B 远不够（+模型太小是主因） |
| P1 Jev 分类 >70%（1.5B） | 0.7 | 47% | 半命中 | +33pp 趋势确认模型大小有效 |
| P1 Jev 分类 >70%（8B Qwen3） | 0.8 | 6.7% | 翻车 | **跨系列不可外推** + **thinking 通道不兼容** + **hardcoded template 不适配** |
| P2 延迟 <1s（0.5B） | 0.6 | 1.7-2.3s | 翻车 | CPU 不可行，需 MLX/GPU |
| Q7 session 锁定 | — | 修正 | 翻车 | session 内任务会变（先代码后翻译），改为任务轮判断 |
| 单 token 打分无位置偏好 | 0.9 | 8B 6.7% | 翻车 | **LLM2Jev 揭示：单 token 有位置偏好 + thinking 不兼容** |
| hardcoded chat template 通用 | 0.8 | Qwen3 6.7% | 翻车 | **不同模型系列 template 不同（thinking/tools 条件）** |
| hint-only 可交付 | 0.9 | pong ✅ | 命中 | 无——直路由 + schema 转换 work |
| 工具调用转换 | 0.7 | tool_use ✅ | 命中 | 无——双向转换 work |
| 跨平台交叉编译 | 0.85 | 6 平台 ✅ | 命中 | purego Unix-only（Windows 用 syscall.NewLazyDLL） |
| zig @cImport llama.h | 0.7 | 编译 ✅ | 命中 | ggml.h 依赖链（要 vendored 7 个头文件） |

**Surprise Rate**：12 预测中 5 翻车 = 42%。主要 Surprise：跨系列不可外推 + thinking 不兼容 + 单 token 位置偏好。

### 1.3 成本递减四字段（注册表）

| 字段 | 值 |
|---|---|
| 耗时 | ~1 天（从 Python POC 到 Go 生产版 + 研究 + 沉淀） |
| 缓存复用构件数 | fast_browser_use（candidate_codes/score）+ litellm（schema 转换概念）+ Jev API spec + llama.cpp C API + purego |
| 本次新固化数 | 10 patterns（见 3.1）+ 5 教训（见 3.2） |
| Surprise Rate | 42%（5/12 翻车） |

---

## 二、验证（固化判定）

### 2.1 已验证 active（可复用，双门过）

| # | 经验 | 验证证据 | 状态 |
|---|---|---|---|
| P-001 | Go + CGO_ENABLED=0 跨平台推理引擎 | 6 平台交叉编译 + 39 测试 + 端到端 | active |
| P-002 | purego（Unix）+ syscall.NewLazyDLL（Windows）不用 cgo | 编译验证 + 端到端 dlopen | active |
| P-003 | zig @cImport llama.h 消解 struct ABI | frwrapper 编译 + 5 符号导出 + 端到端推理 | active |
| P-004 | Jev system-one API（Choice schema）智能路由 | docs.typesafe.ai spec + 端到端（task_type=A → pong） | active |
| P-005 | 路由链 C2→C3→C4→C5 + gateway | 39 测试 + 端到端（Jev→挡死→选模→转发） | active |
| P-006 | OpenAI↔Anthropic schema 双向转换（请求/响应/SSE/工具调用） | 13 schema 测试 + 端到端（Anthropic→OpenAI→pong） | active |
| P-007 | session 状态 + 任务轮继承 | 2 session 测试 + C2 11 测试 | active |
| P-008 | config JSON 持久化 + admin web UI + 热重载 | 端到端（POST /api/config 持久化 + SetUpstreams/SetRegistry） | active |
| P-009 | 模型按需下载（不打包 zip）+ modelscope SDK | admin UI 下载向导 + 5 模型选项 | active |
| P-010 | pack.sh 6 平台打包 + SOP/build.md | darwin/amd64 zip 9.5MB 验证 + SOP 445 行 | active |

### 2.2 candidate（未完全验证，待复现/审查）

| # | 经验 | 状态 | 解除条件 |
|---|---|---|---|
| C-001 | per-candidate yes/no 优于单 token（LLM2Jev 方法） | candidate | Ornith + yes/no P1 对比 benchmark |
| C-002 | llama_chat_apply_template 替代 hardcoded | candidate | fr_apply_template 集成 Go 侧 + 对比 |
| C-003 | Ornith-1.5-9B 是理想 Jev 模型 | candidate | Ornith P1 benchmark |
| C-004 | hint-only 模式可交付（不依赖 Jev） | candidate→active | 已端到端验证（pong），但未 fresh 审查 |
| C-005 | 8B Qwen3 P1=6.7% 是跨系列 + thinking 问题（非模型大小） | candidate | per-candidate yes/no + apply_template 重跑 8B |

### 2.3 I-DISC 状态

- **未满足**：所有产出由单一研究者（我）完成，未派 fresh 无共享上下文审查员复核
- **影响**：active 条目均为自产实证（现场演示级），非第三方复现
- **解除**：派 fresh 审查员复核设计/代码/文档解读

---

## 三、沉淀（双账本）

### 3.1 蒸馏层（patterns 层——跨项目可复用经验协议）

| ID | pattern | 内容 | 适用边界 | 负例（何时不适用） |
|---|---|---|---|---|
| P-001 | Go 跨平台推理引擎 | CGO_ENABLED=0 + purego/syscall + zig wrapper + llama.cpp C API，6 平台单源码交叉编译 | 本地推理网关/工具 | 需要原生 GPU 加速（CUDA/Metal）时需额外后端 |
| P-002 | zig wrapper 消解 FFI struct ABI | @cImport C 头文件，zig 编译器处理 struct layout，Go 侧只绑窄 ABI（标量+指针） | C 库有复杂 struct（如 llama_batch 含二维指针） | C 库 API 简单（纯标量）时直接 purego 即可 |
| P-003 | Jev system-one 智能路由 | system-one API（Choice schema）+ 任务类型分类 → 挡死 → 加权选模 → 转发 | LLM 网关需要按任务智能选模型 | 单一模型场景（不需路由） |
| P-004 | 多协议 schema 双向转换 | OpenAI↔Anthropic 请求/响应/SSE/工具调用全链转换（system 位置 + tool_calls↔tool_use + chunk↔event 状态机） | 兼容多 LLM 协议的网关 | 只需兼容单一协议 |
| P-005 | config-driven registry + admin UI | JSON config 持久化 + web UI CRUD + 热重载（SetUpstreams/SetRegistry mu-guarded） | 需要运行时配置的本地服务 | 固定配置（不需运行时改） |
| P-006 | 模型按需下载（不打包） | zip 只含 binary+libs+config 模板，用户首启通过 admin UI 下模型（modelscope SDK） | 模型大的分发场景 | 模型小（<100MB）可直接打包 |
| P-007 | session 状态 + 任务轮继承 | C2 检测任务轮（tool_result vs new user）→ non-NewTaskTurn 继承上次路由（不重复推理） | agent 工具循环场景 | 无工具调用的单轮请求 |
| P-008 | llama.cpp nightly 预编译（不编译） | 35 个平台包含 libllama/libggml 共享库，下载解压即用 | 用 llama.cpp 推理 | 需要定制编译（如 Metal/GPU 加速） |
| P-009 | Jev 打分两种方法 | 单 token 多候选（快，一次 forward，有位置偏好）vs per-candidate yes/no（慢，N 次 forward，无偏好，正确 template） | Jev system-one 实现 | 非 Jev 场景 |
| P-010 | 分发包结构 | Go binary + lib/libfrwrapper + lib/libllama+libggml + config 模板 + README，~10-15MB | 跨平台本地工具分发 | 纯 Web 服务（不需本地 binary） |

### 3.2 蒸馏层（教训——避免重交学费）

| ID | 教训 | 机理 | 对策 |
|---|---|---|---|
| L-001 | 单 token 打分有位置偏好 | 候选 token id 在 softmax 中的位置不同，模型有训练偏差 | 用 per-candidate yes/no（独立评估，无位置偏好） |
| L-002 | hardcoded chat template 不适配不同模型系列 | Qwen2.5/Qwen3/Qwen3.5 template 不同（thinking 通道/tools 条件） | 用 llama_chat_apply_template（自动适配） |
| L-003 | 跨模型系列不可外推 P1 趋势 | Qwen2.5（0.5B→1.5B +37pp）≠ Qwen3（8B 6.7%），不同训练/行为 | 同系列内验证趋势，换系列重新 benchmark |
| L-004 | Qwen3 thinking 通道干扰 logits | hardcoded `assistant\n` 后取 logits，但 Qwen3 先 thinking 再 answer——logits 在 thinking 位置 | per-candidate yes/no 在 answer 位置取 yes/no logit + apply_chat_template 正确关闭 thinking |
| L-005 | /tmp 被系统清理导致 libllama 丢失 | /tmp 临时目录重启清理，libllama.dylib 丢失导致 dlopen 失败 | 用持久路径（~/.local/share/） |

### 3.3 回放层（DecisionRecord 事后校准——保留分支结构）

见 1.2 DecisionRecord 校准表（含预测×结果×Surprise，支持反事实评估"如果当时走了 per-candidate yes/no 而非单 token"）。

**关键反事实**：如果 POC 阶段就用 per-candidate yes/no + apply_chat_template（而非单 token + hardcoded），8B P1 可能不是 6.7%——可能 >47%（因为解决了 thinking + 位置偏好）。这是 LLM2Jev 研究揭示的"如果当时走了没走的路"。

---

## 四、应用（命中短路——下次同类项目复用指引）

### 4.1 下次做"本地 LLM 网关/路由"项目时

**直接复用（active patterns）**：
1. P-001 Go 跨平台引擎（CGO_ENABLED=0 + purego/syscall + zig + llama.cpp）
2. P-004 schema 双向转换（OpenAI↔Anthropic 全链）
3. P-005 config + admin UI + 热重载
4. P-006 模型按需下载
5. P-007 session + 任务轮继承
6. P-008 llama.cpp nightly 预编译
7. P-010 分发包结构

**注意教训（避免重交学费）**：
1. L-001 用 per-candidate yes/no（不要单 token）
2. L-002 用 apply_chat_template（不要 hardcoded）
3. L-003 同系列内验证趋势
4. L-004 注意 thinking 通道
5. L-005 用持久路径

**需重新验证（candidate）**：
- C-001 per-candidate yes/no 效果（Ornith benchmark 待跑）
- C-002 apply_chat_template 效果
- C-003 Ornith-1.5-9B P1

### 4.2 缓存命中短路

下次同类目标的组织形成时：
- C2/C4/C5 纯 Go 路由逻辑 → 直接复用（30 测试对照）
- schema 转换 → 直接复用（13 测试）
- gateway + admin + config → 直接复用
- zig wrapper + llama.cpp C API → 直接复用（5 符号）
- pack.sh + SOP → 直接复用

---

## 五、升级（recheck + 失效淘汰）

### 5.1 recheck_condition

| pattern | 复验条件 | 有效期 |
|---|---|---|
| P-001 Go+purego+zig | Go/purego/zig 大版本变更 | 长期（ABI 稳定） |
| P-003 Jev system-one API | Jev API spec 变更（docs.typesafe.ai） | 中期（Jev 在迭代） |
| P-004 schema 转换 | OpenAI/Anthropic API 变更 | 中期（API 在迭代） |
| P-008 llama.cpp nightly | llama.cpp C API 大改 | 中期（C API 相对稳定） |
| P-009 打分两种方法 | 新 Jev 实现出现（如 LLM2Jev 更新） | 短期（方法在演化） |

### 5.2 失效淘汰条件

- P-009 单 token 方法：如果 per-candidate yes/no 在 Ornith 上 P1 >70%（单 token <70%），单 token 方法降级为"快速但低准确率"备选（不淘汰，有延迟优势）
- L-001..L-004 教训：如果 per-candidate yes/no + apply_template 在 Ornith 上 P1 >70%，这些教训从"待验证"升级为"已验证 active"

### 5.3 T10 行为可验证改变（沉淀完成判据）

**"下次行为变了没"**：
- 下次做本地 LLM 网关 → 用 Go+purego+zig（不用 Python/cgo）✅ 行为改变
- 下次做 Jev 打分 → 用 per-candidate yes/no（不用单 token）✅ 行为改变（待 Ornith 验证确认）
- 下次做 schema 转换 → 用双向全链（请求/响应/SSE/工具调用）✅ 行为改变
- 下次分发 → 模型不打包（按需下载）✅ 行为改变

---

## 六、诚实边界

1. **架构/引擎固定**：Go+purego+zig+llama.cpp+Jev API+路由链+schema+admin+打包，17 commit 39 测试 6 平台验证——固定。
2. **P1 依赖模型能力**：Ornith-1.5-9B + per-candidate yes/no 待验证，不阻塞架构/引擎/流程。
3. **I-DISC 未满足**：单一研究者产出，未 fresh 复核——进可信决策须另起审查。
4. **知识沉淀是 v0.1**：文档落盘 + patterns/回放/注册表系统化，但无自动复用机制（应用层是人工指引，非运行时命中短路）。
5. **C7-C9 判据回流未实现**：学习闭环（校准实测矩阵）未建——当前是静态 registry，不是动态校准。
6. **Surprise Rate 42%**：5/12 翻车，主要在 P1（模型/方法假设错误）——架构/引擎假设全命中，打分方法假设翻车。

---

## 附录 溯源

- 知识沉淀流程规范：`book/基于ai-native的知识沉淀流程规范.md`
- 项目全量：fast-router 仓库（13d9f31..317e329，17 commit）
- 文档索引：8 个设计/研究/进展文档 + SOP/build.md + 本文件
- 代码：router/（20 Go 文件）+ zig/（frwrapper.zig + build.zig + 7 头文件）+ cmd/（3 入口）+ scripts/pack.sh
- 测试：39 测试（router 31 + schema 8，不含 session/tool 8 = 39 含）
- 实测：0.5B/1.5B/8B P1/P2 + 端到端（pong/tool_use/SSE/schema/session）

---

## v0.2 增量更新（2026-09-26）

### 新增 DecisionRecord 校准

| 预测 | 事前 | 实测 | 结果 | Surprise |
|---|---|---|---|---|
| P1 yes/no 0.5B > 单 token | 0.8 | 16.7% vs 10% (+6.7pp) | 命中 | 位置偏好确实存在且 yes/no 消除 |
| P1 Ornith-9B yes/no >70% | 0.8 | 46.7% | 翻车 | 9B 也不达 70%——prompt 设计 + template 适配是瓶颈，不只模型大小 |
| MiniCPM5-2B P1 | — | 待测 | — | 2B Llama 端侧候选，1.56GB 快下快跑 |

### 新增 patterns

| ID | pattern | 内容 |
|---|---|---|
| P-011 | 双模型策略 | CPU 场景用 2B（MiniCPM5-2B 1.56GB ~12s/sample），GPU/MLX 用 9B（Ornith 5.4GB） |
| P-012 | per-candidate yes/no 引擎三件套 | fr_score_yesno + fr_apply_template + fr_get_token_id，不依赖单 token 映射，自动适配任意模型 chat template |

### 新增教训

| ID | 教训 | 机理 | 对策 |
|---|---|---|---|
| L-006 | 9B 也不一定 >70% | Ornith-9B（媲美 4B）yes/no P1=46.7%——模型大小不是唯一因素，prompt 设计 + template 适配是瓶颈 | 优化 prompt + 用 apply_chat_template |
| L-007 | yes/no +6.7pp 但不解决根本 | 0.5B 10%→16.7% 确认位置偏好，但 P1 仍低——prompt 设计和 template 比方法选择更重要 | 研究 LLM2Jev 精确 prompt + 集成 apply_chat_template |

### 新增/更新 candidate

| ID | 经验 | 状态 | 解除条件 |
|---|---|---|---|
| C-006 | MiniCPM5-2B P1（2B Llama 端侧） | candidate | 下载完成 + yesnobench |
| C-007 | apply_chat_template 集成到 yesnobench | candidate | 改 yesnobench 用 fr_apply_template 替代 hardcoded |
| C-005 更新 | 8B Qwen3 6.7% 是跨系列 + thinking 问题 | candidate→部分验证 | Ornith yes/no 46.7%（miss 分散）确认位置偏好消除，但 P1 仍低——template + prompt 是下一步 |

### 更新注册表

| 字段 | v0.1 | v0.2 |
|---|---|---|
| 耗时 | ~1 天 | ~1.5 天（加 Ornith 9B + MiniCPM5-2B 研究 + yes/no 集成） |
| 缓存复用构件数 | 5 | 7（加 fr_score_yesno + fr_apply_template） |
| 新固化数 | 10 patterns + 5 教训 | 12 patterns + 7 教训 |
| Surprise Rate | 42%（5/12） | 43%（6/14）——Ornith 9B P1=46.7% 翻车 |

### T10 行为改变验证（v0.2）

- 下次做 Jev 打分 → 用 per-candidate yes/no（不用单 token）✅ 行为改变
- 下次选模型 → 优先 2B 端侧（MiniCPM5-2B），CPU 友好 ✅ 行为改变
- 下次做 prompt → 用 apply_chat_template（不 hardcoded）✅ 行为改变（待 yesnobench 集成确认）
- 下次评估 P1 → 不只看模型大小，prompt 设计 + template 适配同样关键 ✅ 认知改变

---

## v0.3 增量更新（2026-09-26 MiniCPM5-2B 结果）

### 新增 DecisionRecord

| 预测 | 事前 | 实测 | 结果 | Surprise |
|---|---|---|---|---|
| MiniCPM5-2B yes/no P1 | 0.6（媲美 4B） | 26.7% | 翻车 | 2B Llama 介于 0.5B(16.7%) 和 1.5B(47%) 之间——模型大小有效但收益递减 |

### 完整 P1 趋势（所有模型 × 方法）

| 模型 | 系列 | 大小 | 方法 | P1 | P2 |
|---|---|---|---|---|---|
| Qwen2.5-0.5B | Qwen2.5 | 0.5B | 单 token | 10% | 1.7s |
| Qwen2.5-0.5B | Qwen2.5 | 0.5B | yes/no | 16.7% | 4.7s |
| Qwen2.5-1.5B | Qwen2.5 | 1.5B | 单 token | 47% | 18s |
| Qwen3-8B | Qwen3 | 8B | 单 token | 6.7% | 9.5s |
| Ornith-1.5-9B | Qwen3.5+ | 9B | yes/no | 46.7% | 120s |
| MiniCPM5-2B | Llama | 2B | yes/no | 26.7% | 12.3s |

### 更新教训

| ID | 教训 | 机理 | 对策 |
|---|---|---|---|
| L-008 | 模型大小收益递减 | 0.5B→2B→9B: 16.7%→26.7%→46.7%（+10pp/+20pp），都 <70%——**prompt 设计 + template 是共同瓶颈，不只模型大小** | 集成 apply_chat_template + 研究 LLM2Jev 精确 prompt |

### 更新 candidate

| ID | 状态 | 结果 |
|---|---|---|
| C-006 MiniCPM5-2B | candidate→已测 | P1=26.7%（2B Llama 介于 0.5B 和 1.5B 之间，CPU 友好 12.3s） |

### 更新注册表

| 字段 | v0.2 | v0.3 |
|---|---|---|
| 新固化数 | 12 patterns + 7 教训 | 12 patterns + 8 教训（+L-008 收益递减） |
| Surprise Rate | 43% | 44%（7/16，MiniCPM5-2B 26.7% 翻车） |

### 结论：P1 瓶颈诊断

**所有模型 × 方法都 <70%**——共同瓶颈是：
1. hardcoded Qwen2.5 chat template（不适配 Llama/Qwen3/Qwen3.5）
2. 简单 "is this about X?" prompt（不利用模型推理能力）
3. fr_apply_template 已实现但未集成到 yesnobench

**下一步优先级**：
1. 集成 apply_chat_template 到 yesnobench（替代 hardcoded）
2. 研究 LLM2Jev 精确 prompt 格式（clone 源码）
3. 用 apply_template + 优化 prompt 重跑所有模型

---

## v0.4 增量更新（2026-09-26 P1=73.3% 达标！）

### 🎉 P1 > 70% 目标达成

**Ornith-1.5-9B + per-candidate yes/no + apply_chat_template: P1 = 73.3% (22/30)**

### 完整 P1 演化

| 模型 | 大小 | 方法 | template | P1 | 增量 |
|---|---|---|---|---|---|
| Qwen2.5-0.5B | 0.5B | 单 token | hardcoded | 10% | 基线 |
| Qwen2.5-0.5B | 0.5B | yes/no | hardcoded | 16.7% | +6.7pp 方法 |
| MiniCPM5-2B | 2B | yes/no | hardcoded | 26.7% | +10pp 模型 |
| MiniCPM5-2B | 2B | yes/no | apply | 43.3% | +16.6pp template |
| Ornith-9B | 9B | yes/no | hardcoded | 46.7% | +3.4pp 模型 |
| **Ornith-9B** | **9B** | **yes/no** | **apply** | **73.3%** | **+26.6pp template** |

### 三个因素（按影响排序）

1. **template 适配**（+26.6pp for 9B / +16.6pp for 2B）——最大因素
2. **模型大小**（0.5B→9B：无 template +30pp / 有 template +56.6pp）
3. **方法**（单 token→yes/no：+6.7pp，消除位置偏好）

### candidate→active 升级

| ID | 原状态 | 新状态 | 原因 |
|---|---|---|---|
| C-001 per-candidate yes/no 优于单 token | candidate | **active** | 73.3% > 46.7%（hardcoded） |
| C-002 apply_chat_template | candidate | **active** | +26.6pp 验证 |
| C-003 Ornith-1.5-9B | candidate | **active** | P1=73.3% > 70% |
| C-005 8B 低分是 template 问题 | candidate | **active** | template 是最大因素验证 |

### 新教训

| ID | 教训 | 机理 | 对策 |
|---|---|---|---|
| L-009 | template 是最大因素 | 9B template +26.6pp > 模型 0.5B→9B +30pp（但 template 在 2B 上 +16.6pp > 模型 0.5B→2B +10pp）| 优先适配 template 再增大模型 |
| L-010 | template 是 model-specific | Qwen 系列 hardcoded 已对（-3.4pp），Llama 系列 apply 才对（+16.6pp）| Qwen 用 hardcoded，非 Qwen 用 apply |
| L-011 | P1 达标需三者合一 | 模型（9B）+ 方法（yes/no）+ template（apply）缺一不可：46.7%（缺 template）→ 73.3%（三者齐全）| 三个因素同时优化 |

### T10 行为改变（v0.4 最终）

- ✅ 用 yes/no（不用单 token）
- ✅ 用 apply_chat_template（不用 hardcoded）—— 非 Qwen 必须
- ✅ 用 Ornith-1.5-9B（9B + Qwen3.5 + agent 训练）
- ✅ P1 > 70% 达标——Jev 智能路由完全交付

## v0.5 增量更新（2026-09-27 技术栈评估与机制校准）

> 本次非实验局，是**评估局**：回答"zig + golang + c/cpp 是否最佳组合"，并逐条核实五点机制表述。完整分析见 `docs/fast-router-技术栈评估-zig-go-cpp.md`（本节只沉淀增量，不重复）。

### 结论（三行）

> ① "最佳"缺维度不可判定；在 fast-router 约束下（单机开发/6 平台分发/无 cgo/进程内推理）是**可辩护的局部最优 + 可逆性最好**，不存在单向门。
> ② 三层自由度不对等：C/C++ 层无选择（llama.cpp 即引擎），真自由度只在网关语言 + FFI 策略，而这两项的替代方案已被历史 DecisionRecord 证伪。
> ③ 本质不是"每层都最强"，而是"每层把复杂度转移给最能扛的那层"：struct ABI→Zig、编译→nightly 预编译、运行时绑定→purego/NT loader、业务→Go。

### 新增 DecisionRecord 校准（五点机制表述 × 仓库事实）

| 预测（用户表述） | 事前 | 核实 | 结果 | Surprise |
|---|---|---|---|---|
| Go 高效网络适合网关 | 0.85 | goroutine+netpoller 对 HTTP/SSE 真实契合；但当前瓶颈在推理（P2=77s）非网络 | 命中 | 买的是开发便利与正确性，不是当前性能 |
| Zig 方便嵌入 C/C++/Rust | 0.7 | 对 C 真（@cImport）；C++ 只能链接不能嵌头；Rust 无特殊能力（回到 extern "C"） | **翻车** | llama.cpp 可用恰因它暴露的是 C API（llama.h），这是运气不是必然 |
| purego 非 cgo 可行 | 0.9 | 运行时 dlopen ✅；但只安全绑标量+指针（正是 Zig 层存在理由）；Windows 侧实际用 syscall.NewLazyDLL | 命中 | purego.Dlopen 是 dlfcn/Unix-only——"一套 purego 全平台"是错觉 |
| 整体方便交叉编译 | 0.75 | Go 半边真一键；native 半边需目标平台 llama 库（SOP ⚠️），且 6 平台仅构建级验证，Windows 运行时未验 | **半翻车** | 复杂度没消失，转移到了版本耦合矩阵（zig 0.14.x × llama b11175 × 6 平台） |
| 编译工具方便 | 0.85 | go build + zig build + curl，不需 C 编译器不编译 llama.cpp | 命中 | 链内简单但链间有摩擦（版本锁定纪律是持续成本） |

**本局 Surprise Rate：5 中 2 翻车 1 半 = 40%**——主要 Surprise：Zig 能力边界被高估（C++/Rust 部分）、交叉编译两半论。

### 新增 patterns

| ID | pattern | 内容 | 适用边界 | 负例 |
|---|---|---|---|---|
| P-013 | 复杂度转移分层原则 | 多语言栈的合理性判据：每层把复杂度转移给最能扛的那层（struct ABI→系统语言胶水；编译→上游预编译；绑定→OS loader；业务→高生态语言），而不是每层都选"最强"语言 | 任何多语言分层 FFI 项目 | 单语言已够用时（为分层而分层） |
| P-014 | 栈选择自由度审计 | 选型前先审计哪层真有自由度：被生态锁死的层（如 llama.cpp 之于本地 GGUF 推理）不参与比较，只比较真自由层（网关语言、FFI 策略），可减少伪选项 | 任何"XX 语言组合是否最佳"问题 | 无黑盒依赖的纯自研项目 |
| P-015 | 交叉编译两半论 | 声称"方便交叉编译"时拆两半验证：Go 半边（CGO_ENABLED=0 一键）与 native 半边（需目标平台库 + 运行时验证）分开判定，构建级验证 ≠ 运行级验证 | 带 native 库的 Go/Rust 分发项目 | 纯 Go/纯静态项目（无 native 半边） |

### 新增教训

| ID | 教训 | 机理 | 对策 |
|---|---|---|---|
| L-012 | "三种语言"的直觉成本模型是错的 | 日常体验是"一种语言 + 257 行适配器 + 一个下载的黑盒"；真实成本是三条工具链的**链间**版本耦合，不是链内语言数 | 评估多语言栈时数"工具链耦合点"而非"语言数" |
| L-013 | "方便嵌入"表述易高估 | @cImport 只解析 C 头；C++ 头不能嵌只能链接；Rust 要它主动导出 extern "C"。**嵌入的前提是对方暴露 C ABI** | 评估胶水语言前先确认目标库的 ABI 面 |
| L-014 | 构建级 ≠ 运行级验证 | 6 平台 zip 打包成功不等于 6 平台能跑（Windows 真机至今未验）；发布矩阵的死法在运行时不在编译时 | SOP 的"发布边界"条款必须执行：native 库加载+推理通过才算平台验证 |

### 新增/更新 candidate

| ID | 经验 | 状态 | 解除条件 |
|---|---|---|---|
| C-008 | llama-server 子进程可替代 zig 层（3→2 工具链） | candidate | 核验 llama-server HTTP API 是否暴露单 forward 多候选 logits / yes/no 打分 / apply_chat_template 等价能力 |
| C-009 | 技术栈"可辩护局部最优"判定本身 | candidate | 约束变化时（放弃 6 平台 / 转 GPU / 放弃进程内）重跑评估；Windows 真机验证后升级 |

### 更新注册表

| 字段 | v0.4 | v0.5 |
|---|---|---|
| 耗时 | ~2 天 | ~2.5 天（加技术栈评估局 + 机制校准） |
| 缓存复用构件数 | 8 | 8（无新增外部构件，新增自产评估 1 份） |
| 新固化数 | 12 patterns + 11 教训 | **15 patterns + 14 教训**（+P-013/014/015 +L-012/013/014） |
| Surprise Rate | —（无实验局） | 评估局 40%（2 翻车 + 1 半 / 5） |

### T10 行为改变（v0.5）

- ✅ 下次描述这套栈 → 用"复杂度转移"表述，不再说"每层都最强"（表述协议改变）
- ✅ 下次遇到"X 语言组合是否最佳" → 先做自由度审计（P-014），只比较真自由层，不再全层泛比
- ✅ 下次声称/听到"方便交叉编译" → 拆两半论（P-015）验证，构建级和运行级分开说
- ✅ 下次评估多语言栈成本 → 数工具链耦合点（L-012），不数语言数

---

### v0.5 溯源

- 评估全文：`docs/fast-router-技术栈评估-zig-go-cpp.md`（来源/方法/发现/局限/结论五要素）
- 历史决策依据：`docs/fast-router-Go重写进展-v0.1.md` 第一章 + `SOP/build.md` 关键设计决策
- 实体证据：`zig/frwrapper.zig`（257 行）、`router/zig_backend.go`、`router/zig_backend_windows.go`、`router/scorer.go`（Backend 接口）
- 理论根：`book/ai-native组织理论.md` + AGENTS.md 维度/死法/可逆性三问

## v0.6 增量更新（2026-09-27 R5 落地 + KV 复用 + 两个 v1.0 bug）

> 本局含一个架构解（R5 异步首评）+ 一个性能优化（批量 KV 复用）+ **两个 v1.0 存量 bug**（sessionKey、模板污染），全部由 TDD/基准探针驱动发现。

### 结论（三行）

> ① R5 落地：NewTaskTurn 不再同步阻塞 77s，hint 即走 + 后台回填 + 续轮继承，有效 P2<1s（P1 路径零改动）。
> ② 批量 KV 复用实测 2.1-3.9x（随 state 长度增长，argmax 100% 一致），9B 推算 77s→~20s。
> ③ 两个 v1.0 bug：sessionKey 增长前缀 hash 在真实工具循环从不命中；parseMessages 指针读穿引号致 prompt 带 JSON 垃圾 + state 翻倍——**全部历史 P1 数字在污染 prompt 上测得**。

### 新增 DecisionRecord 校准

| 预测 | 事前 | 实测 | 结果 | Surprise |
|---|---|---|---|---|
| KV 复用可得 6-8x | 0.75 | 首测 1.10x | **翻车** | 加速被 prompt 形状封顶（共享前缀仅 24-48%）——深挖后发现 prompt 本身有 bug |
| 修复模板 bug 后加速恢复 | 0.8 | 2.1x→3.9x（随 state 长度） | 命中 | 干净 prompt 共享分数升至 86%+ |
| session 继承已在 v1.0 验证 | 0.7 | key 从不命中（测试只验了 DetectTurn+手工塞缓存） | **翻车** | session_test 注释自己承认未验 key 连续性 |
| 模板路径 OK（P1 已达标） | 0.85 | 每个 prompt 含 `user","content":"` 垃圾前缀 + state 翻倍 | **翻车** | P1=73.3% 是 9B 在污染 prompt 上的鲁棒性下限；0.5B 修复后 16.7%→23.3% |
| zigBackend 无需互斥 | 0.6 | R5 异步并发下同 ctx 并发访问（竞态） | 翻车 | 同步路径靠 g.mu 串行是巧合而非设计 |

**本局 Surprise Rate：5 中 4 翻车 = 80%**——全部转化为修复/加固。

### 新增 patterns

| ID | pattern | 内容 | 适用边界 | 负例 |
|---|---|---|---|---|
| P-016 | 探针式性能归因 | 加速不达预期时先插桩（每阶段耗时/每序列首分歧位置/字节级 LCP）再下结论——本轮三层探针最终挖出与性能无关的模板 bug | 任何性能优化局 | 微小改动（不值得插桩成本） |
| P-017 | FFI C 字符串边界纪律 | C ABI 的字符串字段必须 NUL 终止且语义完整；指向序列化缓冲内部的指针会读穿结构边界（role 从 `user` 读到缓冲区尾） | 任何手写 FFI 包装层 | 传长度+指针的显式协议 |

### 新增教训

| ID | 教训 | 机理 | 对策 |
|---|---|---|---|
| L-015 | C 字符串字段读穿 | llama_chat_message 的 role/content 是 C 字符串；JSON 内部指针后随 `","content":"` 而非 NUL | span 解析 + NUL 拷贝 + 反转义（本轮修复） |
| L-016 | KV 复用收益受 prompt 形状封顶 | 加速上限 = 1/(1-共享分数)；当前 prompt 尾部 rubric 占 14-58%，实测 2.1-3.9x 即为此形状的物理上限 | 要更大加速需重构 prompt（目录前置、尾问极短）——但改 prompt 必须全量重验 P1 |
| L-017 | 基准数字随 bug 修复集体失效 | 模板污染影响所有历史测量（0.5B 16.7%、2B 43.3%、9B 73.3% 均在污染 prompt 上测得） | 修复影响输入语义时，所有历史数字标注"污染期测量"并排复测 |

### 新增/更新 candidate

| ID | 经验 | 状态 | 解除条件 |
|---|---|---|---|
| C-010 | 9B 干净模板 P1 复测（预期 ≥73.3%） | candidate | 下载 Ornith-9B 跑 yesnobench |
| C-011 | prompt 重构（目录前置）可再提 KV 复用与 P1 | candidate | C-010 完成后作为独立实验（改 prompt=全量重验） |

### 更新注册表

| 字段 | v0.5 | v0.6 |
|---|---|---|
| 耗时 | ~2.5 天 | ~3 天（加 R5+KV 复用+双 bug 修复局） |
| 缓存复用构件数 | 8 | 9（加 llama_memory_seq_rm 回卷模式） |
| 新固化数 | 15 patterns + 14 教训 | **17 patterns + 17 教训**（+P-016/017 +L-015/016/017） |
| Surprise Rate | 评估局 40% | 实验局 **80%**（4/5）——最高的一局，全部转为修复 |

### T10 行为改变（v0.6）

- ✅ 下次写 FFI 包装 → 字符串字段一律 span+NUL 拷贝，不传缓冲区内部指针（P-017）
- ✅ 下次性能不达预期 → 先插桩归因，不先猜（P-016）；探针可能挖出与性能无关的 bug
- ✅ 下次引用历史基准 → 先确认测量时的输入语义是否与当前一致（L-017）
- ✅ 下次声称"线程安全" → 找出串行化是设计还是巧合（g.mu 案例是巧合）

### v0.6 溯源

- 提交：9334901（R5+sessionKey）、9adbc33（KV 复用+模板 bug+互斥）
- 实测：0.5B A/B bench（zigbench -yesno-bench, PAD=1/4/16）+ yesnobench 复测
- 引擎评估：`docs/fast-router-引擎评估-2026-09-27.md` §R5

### v0.6 附录：9B 干净模板复测（2026-09-27 深夜局）

Ornith-1.5-9B-Q4_K_M（5.78GB，modelscope）+ 修复后模板 + 批量 KV 复用，yesnobench 30 样本：

| 指标 | 污染期（历史） | 干净模板（本次） | 判定 |
|---|---|---|---|
| P1 | 73.3% (22/30) | **73.3% (22/30)** | 完全一致——9B 对模板噪声鲁棒（0.5B +6.6pp 是小模型敏感） |
| P2/样本 | 77s | **38.6s** | 2.0x——与 0.5B 短态加速比一致；长 state 按标度 3-4x |

新增 DecisionRecord：

| 预测 | 事前 | 实测 | 结果 | Surprise |
|---|---|---|---|---|
| 9B 干净模板 P1 ≥73.3% | 0.75 | 73.3%（恰好） | 命中 | 分数逐位复现——大模型把垃圾前缀当噪声吸收 |
| 批量 KV 复用后 9B P2 ~20s | 0.7 | 38.6s | 半命中 | yesnobench 样本 state 短（共享分数低），2.0x 恰为短态预期；~20s 需 agent 长上下文才成立 |

Candidate 变更：C-010（9B 干净模板复测）→ **active**（73.3% 确认）。C-011（prompt 重构）保持 candidate，但现在预期收益下调：干净模板下 P1 已达标且 KV 复用已拿走短态 2x。
