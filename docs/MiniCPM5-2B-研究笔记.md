---
type: Research Note
title: MiniCPM5-2B 深入研究 —— 端侧 2B Jev 模型候选
description: 深入研究 openbmb/MiniCPM5-2B。2B dense Llama 架构，端侧设计，SOTA 2B（媲美 4B），代码/数学/工具调用/Agent 强。GGUF Q4_K_M 1.56GB（下载快 + CPU 快），Apache 2.0。是 fast_router 的轻量 Jev 模型候选——2B 比 9B 小 4x，CPU 推理快 4x，声称媲美 4B 级。
source: https://huggingface.co/openbmb/MiniCPM5-2B (modelscope 镜像抓取)
read_at: 2026-09-26T03:00:00+08:00
method: modelscope API 抓取 README + config.json + 文件列表
---

# MiniCPM5-2B 深入研究

## ——端侧 2B Jev 模型候选

> **三行说明**
> ① MiniCPM5-2B 是 2B dense Llama 架构模型，面向端侧/本地部署，SOTA 2B（媲美 4B），代码/数学/工具调用/Agent 强。
> ② GGUF Q4_K_M = 1.56GB（下载快 + CPU 快），Apache 2.0，有 chat_template.jinja。
> ③ 是 fast_router 的轻量 Jev 模型候选——2B 比 Ornith-1.5-9B 小 4x（1.56G vs 5.4G），CPU 快 4x，声称媲美 4B 级。

---

## 一、来源

- **HuggingFace**: https://huggingface.co/openbmb/MiniCPM5-2B
- **ModelScope**: OpenBMB/MiniCPM5-2B + OpenBMB/MiniCPM5-2B-GGUF
- **抓取方式**: modelscope API（README + config.json + 文件列表）
- **抓取时点**: 2026-09-26T03:00

## 二、模型信息

| 项 | 值 |
|---|---|
| 架构 | LlamaForCausalLM（llama.cpp 原生支持） |
| 参数 | 2B dense |
| hidden_size | 2048 |
| num_hidden_layers | 42 |
| num_attention_heads | 16 |
| vocab_size | 130,560 |
| max_position_embeddings | 131,072 (128K 上下文) |
| 许可 | Apache 2.0 |
| chat_template | 有（chat_template.jinja，9KB） |
| 训练数据 | Ultra-FineWeb + UltraData-Math/Code/SFT-Agent/RL |
| 定位 | 端侧/本地部署/资源受限场景 |
| GGUF Q4_K_M | 1.56GB |
| GGUF Q8_0 | 2.68GB |
| GGUF F16 | 5.04GB |

## 三、能力（README 声称）

- **同尺寸 SOTA**：2B 级开源模型 SOTA
- **媲美 4B**：整体表现可与 4B 级模型竞争
- **强项**：代码推理、数学推理、指令遵循、长文本、工具调用、代码智能体、搜索智能体、通用智能体
- **端侧设计**：面向本地部署和资源受限场景

## 四、对 fast_router 的意义

### 对比已有模型

| 模型 | 系列 | 大小 | GGUF Q4 | P1（已测） | 下载 |
|---|---|---|---|---|---|
| Qwen2.5-0.5B | Qwen2.5 | 0.5B | 0.5GB | 10%/16.7% | 快 |
| Qwen2.5-1.5B | Qwen2.5 | 1.5B | 3GB | 47% | 慢 |
| Qwen3-8B | Qwen3 | 8B | 5GB | 6.7% | 慢 |
| Ornith-1.5-9B | Qwen3.5+ | 9B | 5.4GB | 46.7% | 极慢 |
| **MiniCPM5-2B** | **Llama** | **2B** | **1.56GB** | **待测** | **快** |

### 优势

1. **小 + 快**：1.56GB 下载快（~5min），2B CPU 推理快（预计 ~1s/forward vs 9B ~5s）
2. **Llama 架构**：llama.cpp 原生支持，fr_apply_template 自动适配（Llama chat template 标准）
3. **端侧设计**：和 fast_router "本地推理"目标一致
4. **Agent/工具调用强**：可能对 task_type 分类更准（Agent 训练的模型理解任务边界更好）
5. **Apache 2.0**：可商用
6. **声称媲美 4B**：如果 P1 > Qwen2.5-1.5B（47%），说明 2B 级模型可达高 P1

### 预期

- 2B CPU yes/no（10x forward）预计 ~10-15s/sample（vs 9B 120s）
- 30 samples 预计 ~5-8min（vs 9B ~50min）
- P1 预期：如果媲美 4B，可能 >47%（Qwen2.5-1.5B 单 token）；但 yes/no + Llama template 可能更高

## 五、局限

1. **只有 safetensors + GGUF（无 MLX 4bit）**：但 GGUF 够用（llama.cpp）
2. **Llama 架构 ≠ Qwen**：不同系列，趋势不可外推（教训 L-003）——但 Llama 比 Qwen 更标准，fr_apply_template 更可能正确适配
3. **hardcoded Qwen2.5 template**：yesnobench 用 hardcoded `<|im_start|>`，Llama 模型的 chat template 不同——需用 fr_apply_template（已加到 zig 但 yesnobench 未集成）
4. **2B 可能不够**：声称媲美 4B，但 fast_router 测的 Qwen2.5-1.5B（47%）也是 ~2B 级——如果 MiniCPM5-2B ~47%，说明 2B 级天花板
5. **未实测**：MiniCPM5-2B 在 fast_router 上的 P1 未验证

## 六、结论

**MiniCPM5-2B 是 fast_router 的轻量 Jev 模型候选**——1.56GB + 2B + Llama + 端侧 + Agent 强。

**与 Ornith-1.5-9B 的定位差异**：
- Ornith 9B：高准确率但慢（5.4GB + 120s/sample），需 GPU/MLX
- MiniCPM5-2B：轻量快但可能准确率低（1.56GB + ~12s/sample），CPU 可用

**理想组合**：
- CPU 场景：MiniCPM5-2B（快 + 轻）
- GPU/MLX 场景：Ornith-1.5-9B（准但需算力）

**下一步**：下载 MiniCPM5-2B-Q4_K_M（1.56GB，后台进行中）+ 跑 yesnobench（预计 ~6min）+ 对比 P1。

**如果 MiniCPM5-2B P1 >47%**（超过 Qwen2.5-1.5B），说明 2B 级模型 + Llama 架构 + Agent 训练 = 更优的 Jev 模型（比 Qwen 系列更适合）。
