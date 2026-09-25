---
type: Progress Report
title: fast-router Go 重写进展 v0.1 —— C2/C3/C4/C5 完成 + Jev system-one API 对齐
description: 从 Python 切到 Go（purego + zig wrapper + libllama + 压缩包分发 + 跨平台）。C2/C4/C5 纯 Go 移植完成，C3 Jev system-one scorer 纯 Go 接口完成（遵循 docs.typesafe.ai/api 的 Choice schema，Backend 可插拔）。全套 30 测试 PASS，端到端 Scorer→C4→C5 通。Jev system-one API 官方 spec 已抓取核实。剩余：zig wrapper（C3 Backend 真实现）+ C1/C6 网关 + 模型下载。
source_poc: docs/fast-router-POC进展-v0.1.md
jev_api_spec: https://docs.typesafe.ai/api（2026-09-25 抓取，/api.md markdown 源）
timestamp: 2026-09-25T11:00:00+08:00
status: Go 重写 C2/C3/C4/C5 完成；C3 Backend 真实现（zig）+ C1/C6 待
---

# fast-router Go 重写进展 v0.1

## ——C2/C3/C4/C5 完成 + Jev system-one API 对齐

> **三行说明**
> ① 从 Python 切到 Go：purego（不用 cgo，交叉编译）+ zig wrapper（消解 llama_batch struct ABI 风险）+ libllama（llama.cpp 预编译 nightly）+ 压缩包分发。
> ② C2/C4/C5 纯 Go 移植完成；C3 Jev system-one scorer 纯 Go 接口完成（遵循 Jev API Choice schema，Backend 可插拔）。
> ③ 全套 30 测试 PASS，端到端 Scorer→C4→C5 通。剩余：zig wrapper（C3 Backend 真推理）+ C1/C6 网关 + 模型下载。

---

## 第一章 切换 Go 的决策（依据）

Python 依赖地狱实测（numpy2/torch2.2/scipy1.18/transformers5.17 花了 3 轮才兼容），分发给 codex/claude code 用户不友好。切 Go：

| 决策 | 选择 | 依据（fact） |
|---|---|---|
| 语言 | Go | 网关层生态强 + 交叉编译 + 单二进制 |
| 不用 cgo | purego（ebitengine/purego） | README 确认：不用 cgo + 交叉编译 + dlopen + Tier1 覆盖 mac/linux/windows amd64/arm64 |
| 推理引擎 | llama.cpp（GGUF，替代 MLX） | MLX 是 Swift only，Go 无绑定；llama.cpp 跨平台 + 4bit + KV cache + 暴露 logits |
| FFI 方式 | zig wrapper + purego（非 purego 直绑） | llama_batch struct（含二维指针）跨平台 ABI 风险真实；zig @cImport 由编译器处理 struct，Go 侧 purego 只绑窄 ABI（标量+指针），稳 |
| llama.cpp 来源 | nightly 预编译 binary 包（35 个，全平台） | b11175 release 确认含 libllama.dylib/.so + libggml（macos-x64 包 11MB 已验证），不用自己编译/交叉编译 |
| 分发 | 压缩包（Go bin + libfrwrapper + libllama/libggml） | 用户不要求单二进制，压缩包可 |

---

## 第二章 Jev system-one API spec（关键 fact，2026-09-25 抓取）

来源：https://docs.typesafe.ai/api.md（Mintlify markdown 源，curl 可达）

**Endpoint**: `POST https://api.typesafe.ai/v1/system1`
**Request**: `{state, model, questions: map<id, Question>}`
**三种 Question**：noul（yes/no→0-1）/ **choice**（多选一+概率分布，≤255 options）/ score（有序等级 2-10 levels）
**Response**: `{model, answers: map<id, Answer>, usage: {input_tokens, output_tokens}}`
**Choice Answer**: `{type:"choice", choice, probabilities: map<option,prob>, confidence}`

**关键确认**：Jev Choice 与 fast_browser_use 单 token 打分**同构**——criteria{option:rubric} ↔ candidate table，probabilities ↔ softmax，≤255 options ↔ candidate_codes 256 上限。fast_browser_use 的逆向是正确的。

**C3 对齐**：`router/scorer.go` 的 `System1Request/Response/Question/Answer` struct 对齐此 schema；Choice 是路由用的（选 task type）。

---

## 第三章 完成状态（Go 版）

| 闭包 | 文件 | 测试 | 状态 |
|---|---|---|---|
| C2 任务轮判定 | router/turn_detector.go | 11 | ✅ PASS |
| C4 挡死匹配 | router/matcher.go | 11（合 C5） | ✅ PASS |
| C5 加权选模 | router/selector.go | — | ✅ PASS |
| C3 Jev scorer（接口+Choice） | router/scorer.go | 8 | ✅ PASS |
| 端到端 Scorer→C4→C5 | scorer_test.go | TestRouteChainScorerToMatcher | ✅ |
| 数据 | router/task_types.go, registry.go | — | ✅ |

**全套 30 测试 PASS，go test 0.05s，零外部依赖（go.mod 无 require）。**

C3 设计要点：
- `Backend` 接口（`ChoiceScore(prompt, codes)->logits`）可插拔：mock（测试）/ zig wrapper+libllama（本地进程内）/ cloud Jev API（fallback），接口同 schema
- Choice 逻辑：criteria→candidate codes（A-J）→ backend logits → softmax → probabilities + choice + confidence

---

## 第四章 剩余（Go 重写）

| 闭包 | 依赖 | 复杂度 |
|---|---|---|
| **C3 Backend 真实现**（zig wrapper） | zig + @cImport llama.h + libllama | 高（system-one 推理核心） |
| C1/C6 网关 | 纯 Go（net/http + OpenAI/Anthropic schema 转换 + SSE） | 中 |
| 模型下载 | 纯 Go（modelscope HTTP） | 低 |

**C3 Backend 接口已定**（ChoiceScore），zig wrapper 实现它即可接入——C3 的 Go 侧不动，符合"Backend 可插拔"设计。

---

## 第五章 诚实边界

1. **C3 Backend 是 mock**：纯 Go 接口 + Choice 逻辑 + mock backend 测试通过，但**真推理（zig+libllama）未实现**——P1/P2 待 zig wrapper 接入后实测。
2. **Jev API spec 已抓**：docs.typesafe.ai/api.md curl 核实，非假设。
3. **llama.cpp 预编译已验证**：macos-x64 包含 libllama.dylib + libggml（已下载确认），其他平台待按需验证。
4. **zig wrapper 未写**：方案确认（消解 struct ABI + 隔离层 + 交叉编译），但 zig 工程未建。
5. **跨平台 Go 二进制交叉编译 ready**（CGO_ENABLED=0），但实测只在本机 darwin/amd64 跑过 go test。

---

## 附录 溯源

- POC（Python 验证范式）：`docs/fast-router-POC进展-v0.1.md`（P1: 0.5B 13%→1.5B 47%，9B 外推达标；P2 torch CPU 不可行需 MLX/GPU）
- Jev API spec：https://docs.typesafe.ai/api.md（2026-09-25 抓取）
- llama.cpp C API：https://github.com/ggml-org/llama.cpp/include/llama.h（logits/decode/tokenize/load_model 确认）
- llama.cpp nightly：release b11175，35 个预编译包含 libllama 共享库
- purego：github.com/ebitengine/purego（不用 cgo + 交叉编译 + Tier1 全平台）
- 理论根：`book/ai-native组织理论.md`（no-memory-citation、relock-after-switch）
