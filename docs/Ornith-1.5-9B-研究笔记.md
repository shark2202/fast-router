---
type: Research Note
title: Ornith-1.5-9B 深入研究 —— fast_router 的理想 Jev 模型候选
description: 深入研究 ornith-ai/Ornith-1.5-9B-GGUF。基于 Qwen3.5+Gemma4，9B dense，MIT 许可，GGUF Q4_K_M 5.78GB，多模态，agent 专用训练（SWE-bench 70.6% vs Qwen3.5-9B 53.2%）。是 fast_router Jev 打分的理想模型候选——9B（设计共识目标）+ Qwen3.5 同系列（趋势可比）+ agent 训练（分类可能更准）。
source: https://huggingface.co/ornith-ai/Ornith-1.5-9B-GGUF（modelscope 镜像抓取）
read_at: 2026-09-25T19:30:00+08:00
method: modelscope API 抓取文件列表 + README（curl raw）
applicability: fast_router C3 Jev scorer 的模型选型评估
---

# Ornith-1.5-9B 深入研究

## ——fast_router 的理想 Jev 模型候选

> **三行说明**
> ① Ornith-1.5-9B 基于 Qwen3.5+Gemma4，9B dense，agent 专用训练（自我改进 + RL），benchmark 全面碾压 Qwen3.5-9B。
> ② GGUF Q4_K_M 5.78GB，MIT 许可，modelscope 可下，有多模态 mmproj。
> ③ 是 fast_router Jev 打分的理想候选——9B（设计共识目标）+ Qwen3.5 同系列（和 0.5B/1.5B 趋势可比）+ agent 训练（任务分类可能更准）。

---

## 一、来源

- **HuggingFace**: https://huggingface.co/ornith-ai/Ornith-1.5-9B-GGUF
- **ModelScope**: ornith-ai/Ornith-1.5-9B-GGUF（镜像，curl 可达）
- **抓取方式**: modelscope API（文件列表 + README raw）
- **抓取时点**: 2026-09-25T19:30

## 二、核心发现

### 模型信息

| 项 | 值 |
|---|---|
| 基座 | Qwen3.5 + Gemma4 + 额外 continued pretraining + mid-training + post-training |
| 参数 | 9B dense |
| 训练方式 | 端到端自我改进循环（任务生成 + 脚手架构建 + rollout + RL） |
| 许可 | MIT |
| 多模态 | ✅ 有 mmproj-Ornith-1.5-9B-BF16.gguf（921MB，vision 投影） |
| GGUF 量化 | Q4_K_M (5.78GB) / Q5_K_M (6.64GB) / Q6_K (7.56GB) / Q8_0 (9.79GB) / BF16 (18.4GB) |

### Benchmark（vs Qwen3.5-9B + 其他）

| Benchmark | Ornith-1.5-9B | Qwen3.5-9B | 提升 |
|---|---|---|---|
| SWE-bench Verified | **70.6** | 53.2 | +17.4 |
| Terminal-Bench 2.1 (Claude Code) | **47** | 18.9 | +28.1 |
| Terminal-Bench 2.1 (Terminus-2) | **46.2** | 21.3 | +24.9 |
| SWE-bench Pro | **47.5** | 31.3 | +16.2 |
| SWE-bench Multilingual | **54.4** | 39.7 | +14.7 |
| MCP-Atlas (Agentic) | **54.2** | 46.8 | +7.4 |
| GPQA Diamond | **86.4** | 81.7 | +4.7 |
| HLE (with tools) | **30.5** | 24.5 | +6.0 |

**Ornith-1.5-9B 全面碾压 Qwen3.5-9B**——编码（SWE-bench +17.4）、agent（MCP-Atlas +7.4）、推理（GPQA +4.7）。

### 自我改进训练

Ornith-1.5 扩展了 Ornith-1.0 的自我改进循环：从脚手架 + rollout 优化扩展到联合优化**任务生成 + 脚手架构建 + 解决方案 rollout**。不依赖人工策展任务，持续生成新训练任务 + 发现有效策略 + RL 改进策略。

## 三、对 fast_router 的意义

### 为什么是理想 Jev 模型候选

1. **9B 参数**——设计共识 Q9(d) "9B 起步验证范式"的目标大小
2. **基于 Qwen3.5**——和 fast_browser_use 的 Qwen3.5-9B 同基座，和 0.5B/1.5B（Qwen2.5）同大系列（Qwen 2.5/3/3.5）
3. **agent 专用训练**——编码/推理/agentic 强，可能对 Jev 任务类型分类更准（agent 训练的模型理解任务类型更好）
4. **GGUF Q4_K_M 5.78GB**——可下载（modelscope），llama.cpp 可加载
5. **多模态 mmproj**——如果 fast_router 要支持 vision 任务类型（G multimodal_understanding）
6. **MIT 许可**——可商用

### 对比之前测的模型

| 模型 | 系列 | 大小 | P1 | 问题 |
|---|---|---|---|---|
| Qwen2.5-0.5B | Qwen2.5 | 0.5B | 10% | 太小 |
| Qwen2.5-1.5B | Qwen2.5 | 1.5B | 47% | 不够，但趋势好 |
| Qwen3-8B | Qwen3 | 8B | 6.7% | 不同系列 + thinking 通道不兼容 |
| **Ornith-1.5-9B** | **Qwen3.5+** | **9B** | **待测** | **同系列 + agent 训练 + 9B 目标** |

### 预期

基于趋势（Qwen2.5: 0.5B 10% → 1.5B 47%，+33pp/3x 参数）：
- 9B 预期 >70%（47% + 3x 参数 × 趋势斜率）
- Ornith agent 训练可能额外提升（agent 模型理解任务类型更好）
- 但要配合 LLM2Jev 的 per-candidate yes/no 方法（解决 thinking + 位置偏好）

## 四、局限

1. **README 是模型卡**：benchmark + 训练方法，没有 Jev 打分适配的具体说明
2. **5.78GB 下载**：modelscope 网速波动（279kB/s-5MB/s），可能 30min-3h
3. **CPU 推理慢**：9B CPU forward 可能 >10s/决策（8B 已 9.5s），需 GPU/MLX
4. **多模态 mmproj**：llama.cpp 要额外加载 mmproj（--mmproj 参数），frwrapper.zig 不支持
5. **未实测**：Ornith 在 fast_router 上的 P1 未验证

## 五、结论

**Ornith-1.5-9B 是 fast_router Jev 打分的理想模型候选**——9B + Qwen3.5 同系列 + agent 训练 + MIT + GGUF + 多模态。

**建议**：
1. 下载 Ornith-1.5-9B-Q4_K_M.gguf（5.78GB，modelscope）
2. 配合 LLM2Jev 的 per-candidate yes/no 方法（解决 thinking + 位置偏好）
3. 用 llama_chat_apply_template（替代 hardcoded，适配 Ornith/Qwen3.5 的 chat template）
4. 跑 benchmark 验证 P1 是否达标（>70%）

**如果 Ornith-1.5-9B + per-candidate yes/no + apply_chat_template 达到 P1>70%**，fast_router 的 Jev 智能路由就达标了——可以完全交付（不只是 hint-only）。
