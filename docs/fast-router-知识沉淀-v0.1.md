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
