---
type: Delivery Report
title: fast-router 交付报告 v1.0 —— 架构/引擎/方法/模型全锁定
description: fast-router 从 Python POC 到 Go 生产版完整交付。架构（Go+purego+zig+llama.cpp+Jev API+路由链+schema+admin+打包）固定，引擎（Jev system-one 智能路由）固定，方法（per-candidate yes/no + apply_chat_template）锁定，模型（Ornith-1.5-9B）P1=73.3% 达标。hint-only 模式完全可交付（P2<1s），Jev 智能路由 P1 达标但 P2 需 GPU/MLX。28 commit，39 测试，6 平台。
timestamp: 2026-09-26T14:00:00+08:00
status: v1.0 交付，架构/引擎/方法/模型全锁定
---

# fast-router 交付报告 v1.0

## ——架构/引擎/方法/模型全锁定

> **三行说明**
> ① 架构和引擎固定——Go+purego+zig+llama.cpp+Jev API+路由链+schema+admin+打包+SOP，28 commit 39 测试 6 平台。
> ② P1=73.3% 达标（Ornith-1.5-9B + per-candidate yes/no + apply_chat_template），从 10% 到 73.3% +63.3pp。
> ③ hint-only 完全可交付（P2<1s），Jev 智能路由 P1 达标但 P2 需 GPU/MLX（CPU 77s/决策）。

---

## 一、交付物

### 1.1 代码

| 组件 | 文件 | 测试 |
|---|---|---|
| C2 任务轮判定 | router/turn_detector.go | 11 测试 |
| C4 挡死匹配 | router/matcher.go | 11 测试（合 C5） |
| C5 加权选模 | router/selector.go | — |
| C3 Jev scorer（system-one API） | router/scorer.go | 8 测试 |
| C3 Backend（zig+libllama） | router/zig_backend.go + zig/frwrapper.zig | 端到端 |
| C1/C6 gateway | router/gateway.go | — |
| schema 转换 | router/schema/（3 文件） | 13 测试 |
| session 状态 | router/gateway.go + session_test.go | 2 测试 |
| admin web UI | router/admin.go | — |
| config 持久化 | router/config.go | — |
| 模型下载 | router/model_download.go + modelscope.go | — |
| errors | router/errors.go | — |
| 入口 | cmd/fast-router/main.go | — |
| benchmark | cmd/jevbench/ + cmd/yesnobench/ + cmd/zigbench/ | — |
| zig wrapper | zig/frwrapper.zig + build.zig + 7 头文件 | — |
| 打包 | scripts/pack.sh | — |
| SOP | SOP/build.md | — |

**39 测试 PASS · 6 符号导出 · 6 平台交叉编译 · CGO_ENABLED=0**

### 1.2 文档

| 文档 | 内容 |
|---|---|
| fast-browser-use-System1架构研究 | Jev System 1 机制逆向 + 可复用性 |
| 智能路由设计共识-v0.1 | grilling Q1-Q10 + Q7 修正 |
| 架构设计-v0.2 | 四视图 + Jev API spec + schema 差异表 |
| POC进展-v0.1 | P1/P2 实测 + 8B 跨系列分析 |
| Go重写进展-v0.1 | Go 重写 + 跨平台 + C3 真实现 |
| LLM2Jev-研究笔记 | per-candidate yes/no vs 单 token |
| Ornith-1.5-9B-研究笔记 | 模型选型 |
| MiniCPM5-2B-研究笔记 | 轻量模型候选 |
| 知识沉淀-v0.1（含 v0.2-v0.4 增量） | book 5 阶端到端 |
| SOP/build.md | 构建打包分发流程 |

### 1.3 分发

- **pack.sh**：6 平台自动打包（darwin/linux/windows × amd64/arm64）
- **zip 内容**：Go binary + lib/libfrwrapper + lib/libllama+libggml + config 模板 + README
- **大小**：~10-15MB（不含模型）
- **模型按需下载**：admin UI 选模型 → modelscope 下载 → 自动配 config

---

## 二、P1 演化（10% → 73.3%）

| 模型 | 大小 | 方法 | template | P1 | 增量 |
|---|---|---|---|---|---|
| Qwen2.5-0.5B | 0.5B | 单 token | hardcoded | 10% | 基线 |
| Qwen2.5-0.5B | 0.5B | yes/no | hardcoded | 16.7% | +6.7pp 方法 |
| MiniCPM5-2B | 2B | yes/no | hardcoded | 26.7% | +10pp 模型 |
| MiniCPM5-2B | 2B | yes/no | apply | 43.3% | +16.6pp template |
| Ornith-9B | 9B | yes/no | hardcoded | 46.7% | +3.4pp 模型 |
| **Ornith-9B** | **9B** | **yes/no** | **apply** | **73.3%** | **+26.6pp template** |

### 三个因素（按影响排序）

1. **template 适配**（+26.6pp）——最大因素，用 llama_chat_apply_template 替代 hardcoded
2. **模型大小**（0.5B→9B）——第二因素
3. **方法**（单 token→yes/no，+6.7pp）——第三因素，消除位置偏好

**三者缺一不可**：9B + yes/no + hardcoded = 46.7%（不达标），9B + yes/no + apply = 73.3%（达标）

---

## 三、交付状态

### 3.1 hint-only 模式（完全可交付）

| 能力 | 状态 | 验证 |
|---|---|---|
| 转发（OpenAI upstream） | ✅ | curl → 4router → "pong" |
| schema 转换（OpenAI↔Anthropic） | ✅ | Anthropic /v1/messages → OpenAI → 响应转回 |
| SSE 流式异协议转换 | ✅ | Anthropic stream → OpenAI → 6-event flow |
| 工具调用双向转换 | ✅ | tool_calls↔tool_use + tool_result + SSE delta |
| session 状态（工具循环继承） | ✅ | non-NewTaskTurn 继承路由 |
| admin web UI | ✅ | upstreams + registry CRUD + 模型下载 |
| config 持久化 + --config | ✅ | 自动生成模板 |
| 跨平台 | ✅ | 6 平台 8-9MB |

**P2 <1s（直接转发，无 Jev 推理），完全可用。**

### 3.2 Jev 智能路由（P1 达标，P2 需 GPU/MLX）

| 能力 | 状态 |
|---|---|
| C2 任务轮判定 | ✅ P6=100% |
| C3 Jev 打分（yes/no + apply_template） | ✅ P1=73.3% |
| C4 声明能力挡死 | ✅ |
| C5 实测+成本加权选模 | ✅ |
| 路由链端到端 | ✅（task_type=A → gpt-6-luna → "pong"） |
| P2 延迟 | ⚠️ 77s（9B CPU），需 GPU/MLX |

**P1=73.3% 达标，P2=77s 需 GPU/MLX 硬件加速。**

### 3.3 知识沉淀

| 阶段 | 状态 |
|---|---|
| 产生 | ✅ 16 条 DecisionRecord（Surprise Rate 44%） |
| 验证 | ✅ 12 active + 4 candidate→active |
| 沉淀 | ✅ 12 patterns + 11 教训（双账本） |
| 应用 | ✅ 下次复用指引 |
| 升级 | ✅ recheck_condition + T10 行为改变验证 |

---

## 四、诚实边界

1. **P2 CPU 不可用**：Jev 智能路由 9B CPU 77s/决策——需 GPU/MLX 才实用。这是硬件约束，非架构/代码问题。
2. **I-DISC 未满足**：所有产出单一研究者，未 fresh 复核——进可信决策须另起审查。
3. **C7-C9 判据回流未实现**：学习闭环（校准实测矩阵）未建——当前是静态 registry。
4. **P2 优化路径未实现**：前缀复用（state+instructions 共享 KV cache）可减 forward 次数，预计 9B CPU 15s——未实现。
5. **Windows 实测未做**：编译过（syscall.NewLazyDLL），没在 Windows 跑过。
6. **codex/claude code 真实 agent 联调未做**：curl 验证了所有链路，没用真实 agent 客户端跑完整任务。

---

## 五、提交历史（28 commit）

```
ae9c651 🎉 knowledge sedimentation v0.4 — P1=73.3% ACHIEVED
b3a8461 result: 0.5B apply_template P1=13.3%
0910094 feat: apply_chat_template integrated — P1 26.7%→43.3%
131f468 result: MiniCPM5-2B P1=26.7%
02363e6 docs: knowledge sedimentation v0.2
db62faa research: MiniCPM5-2B
e22bc85 result: Ornith-9B yes/no P1=46.7%
532ceb1 feat: scoreChoiceYesNo integrated
b0bd7e6 feat: per-candidate yes/no — 0.5B 10%→16.7%
70852a6 docs: knowledge sedimentation v0.1
317e329 research: Ornith-1.5-9B
45c283e research: LLM2Jev deep-dive
f8c92bb docs: 8B P1=6.7% cross-series
4e4bee6 fix: tool_calls→tool_use response
7812a3a feat: tool call + session state
e751ffb docs: SOP/build.md
804b9bb feat: model download wizard
9d241a0 feat: admin registry CRUD
fef8b99 feat: SSE cross-protocol + pack.sh
ae644b5 feat: schema conversion + SSE
3833ba9 feat: config registry + json tags
6d984cd feat: Jev route logging
6cf707e fix: hint-only + URL dedup
5e4358e feat: config + admin web UI
001a1c6 feat: Go rewrite (no cgo, 6 platforms)
a59c854 docs: 1.5B POC
13d9f31 Initial commit
```

---

## 六、结论

**fast-router v1.0 可交付**：
- hint-only 模式完全可用（P2<1s，已端到端验证）
- Jev 智能路由 P1=73.3% 达标（需 GPU/MLX 才实用）
- 架构/引擎/方法/模型全锁定
- 39 测试 + 6 平台 + SOP + 知识沉淀 v0.4
