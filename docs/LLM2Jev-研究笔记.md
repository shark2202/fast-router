---
type: Research Note
title: LLM2Jev 深入研究 —— 另一种 Jev 范式实现对比
description: 深入研究 github.com/Yinsongxu/LLM2Jev 的 Jev 范式实现方式，对比 fast_router 的单 token 多候选打分。LLM2Jev 用每候选独立 yes/no 评估 + 前缀复用，与 fast_router 的单 token softmax 根本不同。发现了 fast_router 8B P1=6.7% 低分的可能根因（单 token + Qwen3 thinking 不兼容 + 位置偏好），以及可借鉴的改进方向。
source: https://github.com/Yinsongxu/LLM2Jev/blob/main/README_zh.md
read_at: 2026-09-25T19:00:00+08:00
method: curl 抓取 README_zh.md + request-to-model_zh.md + shared-prefix-cache_zh.md（markdown 源）
applicability: 适用于 fast_router C3 Jev scorer 的改进方向评估
---

# LLM2Jev 深入研究

## ——另一种 Jev 范式实现对比

> **三行说明**
> ① LLM2Jev 是独立开源项目，把本地 LLM 转 Jev 风格决策模型，兼容 `POST /v1/systemone` API。
> ② 核心方法与 fast_router **根本不同**：每候选独立 yes/no 评估（不是单 token 多候选 softmax），前缀复用优化。
> ③ 发现 fast_router 8B P1=6.7% 低分的可能根因：单 token 打分 + Qwen3 thinking 不兼容 + 位置偏好。LLM2Jev 的方法可能解决这些问题。

---

## 一、来源

- **项目**：[LLM2Jev](https://github.com/Yinsongxu/LLM2Jev)（独立开源，与 Jev/TypeSafe 无关联）
- **抓取文档**：README_zh.md / docs/request-to-model_zh.md / docs/shared-prefix-cache_zh.md
- **抓取方式**：curl raw.githubusercontent.com（markdown 源，可达）
- **抓取时点**：2026-09-25T19:00

## 二、核心发现：两种根本不同的 Jev 实现

### fast_router（fast_browser_use 范式）：单 token 多候选

```
prompt = policy + "CANDIDATES:\nA: code_gen\nB: review\n..." + "STATE:\n..." + "Which code?"
→ 一次 forward
→ 取 logits[A_token, B_token, ...] → softmax → 概率分布
```

- **一次 forward**，快
- 候选必须映射成**单 token**（A/B/C...，tokenizer 限制）
- 有**位置偏好**（候选 token 在 softmax 中的位置可能影响分数）
- hardcoded chat template（`<|im_start|>` 格式）

### LLM2Jev：每候选独立 yes/no 评估

```
对每个候选 criteria（如 "物流配送"）:
  prompt = state + instructions + "这个请求属于'物流配送'吗？yes/no"
  → 独立 forward（prefill）
  → 取 yes token 的 logit → 该候选的分数

所有候选分数 → softmax → 概率分布
```

- **N 次 forward**（N = 候选数），慢
- 候选可以是**任意文本**（"物流配送"/"扣款和账单"，不需单 token）
- **无位置偏好**（每候选独立评估，顺序不影响）
- 用后端的 `apply_chat_template`（正确适配各模型）
- **前缀复用**：state + instructions 共享 KV cache（分阶段提交，SGLang Radix Cache）

### 对比表

| 维度 | fast_router | LLM2Jev |
|---|---|---|
| 评估方式 | 单 token 多候选 softmax | 每候选独立 yes/no |
| forward 次数 | 1 次 | N 次（N=候选数） |
| 候选限制 | 必须单 token（A/B/C） | 任意文本 |
| 位置偏好 | 有（候选 token 位置） | 无（独立评估） |
| chat template | hardcoded（Qwen2.5 格式） | 后端 apply_chat_template（自动适配） |
| 前缀复用 | KV 前缀缓存（policy+table） | 分阶段提交（state+instructions 共享） |
| 后端 | zig+libllama（CPU） | SGLang（GPU）/ Transformers / MLX |
| Jev API | System1Request/Response（兼容） | POST /v1/systemone（兼容） |
| 多模态 | ❌ | ✅（图文） |

## 三、对 fast_router 8B P1=6.7% 的根因分析

LLM2Jev 的方法揭示了 fast_router 8B 低分的**三个可能根因**：

### 1. 单 token 打分 + Qwen3 thinking 不兼容

fast_router 的 prompt 以 `<|im_start|>assistant\n` 结尾，直接取 logits 选 A/B/C。但 **Qwen3 有 thinking 通道**——assistant 后应该先 thinking（`<|im_start|>assistant\n`）再 answer。hardcoded 格式可能在 thinking 位置取 logits，而不是 answer 位置。

LLM2Jev 用 `apply_chat_template`（正确关闭 thinking）+ yes/no 评估（在 answer 位置取 yes/no logit）——不受 thinking 干扰。

### 2. 位置偏好

fast_router 的单 token softmax——候选 A 的 token id 和候选 J 的 token id 在 logits 向量中的位置不同，模型可能对某些位置有偏好（训练偏差）。

LLM2Jev 每候选独立评估——无位置偏好。

### 3. chat template 不匹配

fast_router hardcoded Qwen2.5 的 `<|im_start|>` 格式。Qwen3 的 chat template 有 tools 条件分支 + thinking 逻辑——hardcoded 可能不匹配。

LLM2Jev 用后端的 `apply_chat_template`——自动适配。

## 四、可借鉴的改进方向

### 优先级 1：改用每候选独立 yes/no 评估（替代单 token 多候选）

**改动**：
- scorer.go 的 ChoiceScore：不再一次 forward + softmax over 候选 token
- 改为：对每个候选，构造 "state + instructions + '属于{候选}吗？yes/no'" prompt → forward → 取 yes logit
- N 次 forward，但前缀复用（state + instructions 共享）

**收益**：
- 解决位置偏好
- 解决 Qwen3 thinking 不兼容（yes/no 在 answer 位置）
- 候选可以是任意文本（不需单 token 映射）

**代价**：
- N 次 forward（10 候选 = 10 次）——但前缀复用 + KV cache 可缓解
- 延迟增加（但 GPU/MLX 可接受）

### 优先级 2：用 llama_chat_apply_template（替代 hardcoded）

**改动**：
- frwrapper.zig：加 llama_chat_apply_template（C API），自动用模型的 chat template
- 不再 hardcoded `<|im_start|>` 格式

**收益**：
- 正确适配 Qwen2.5/Qwen3/Qwen3.5 各系列
- 解决 thinking 通道问题

### 优先级 3：前缀复用优化

**改动**：
- 每候选独立评估时，state + instructions 是共享前缀
- frwrapper.zig：加前缀缓存（类似 LLM2Jev 的分阶段提交）
- 先 forward 一个候选建立前缀 KV cache，后续候选复用

**收益**：
- N 次 forward 变为 ~1 次全前缀 + N 次短后缀
- 大幅减少延迟

## 五、局限

1. **LLM2Jev 文档是概念级**：README + docs 描述方法，但没看到完整的 logits 提取代码（可能要 clone 源码深入）
2. **SGLang 后端**：LLM2Jev 主要用 SGLang（GPU），fast_router 用 llama.cpp（CPU/GPU）——后端不同，前缀复用机制要适配
3. **yes/no 评估的 prompt 设计**：LLM2Jev 的 yes/no prompt 具体格式要 clone 源码确认（文档只说"候选变成 yes/no 判断"）
4. **未实测**：LLM2Jev 的方法在 fast_router 上的效果未验证——需实现 + 对比 benchmark

## 六、结论

LLM2Jev 揭示了 fast_router 的单 token 打分方法的**三个潜在缺陷**（位置偏好 + thinking 不兼容 + hardcoded template），这些可能是 8B P1=6.7% 的根因（而非简单的"模型系列差异"）。

**最务实的改进**：改为每候选独立 yes/no 评估 + llama_chat_apply_template。这能同时解决三个缺陷，且与 LLM2Jev 的验证方法一致（LLM2Jev 已用此方法在 Qwen3-1.7B 上 work）。

**下一步**：
1. clone LLM2Jev 源码，确认 yes/no prompt 的精确格式 + logits 提取方式
2. 在 fast_router 实现 yes/no 评估（frwrapper.zig 改 fr_score 为 per-candidate）
3. 用 Qwen3-8B（已下载）重跑 benchmark，对比 P1 是否提升
4. 如果 P1 提升，确认根因是方法（单 token → yes/no），不是模型系列
