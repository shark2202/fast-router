---
type: Research Note
title: MiniCPM5-2B 深入研究 —— 端侧 2B Jev 模型候选
description: MiniCPM5-2B 的初步候选评估。2B dense Llama 架构，端侧设计，提供 GGUF 与 llama.cpp 路径；适合作为 fast-router C3 scorer 候选，但准确率、延迟、chat template 和单 token 兼容性仍待实测。详细核验以 MiniCPM5-2B 深度研究为准。
source: https://huggingface.co/openbmb/MiniCPM5-2B (modelscope 镜像抓取)
read_at: 2026-09-26T03:00:00+08:00
method: modelscope API 抓取 README + config.json + 文件列表
superseded_by: ./MiniCPM5-2B-深度研究.md
---

# MiniCPM5-2B 深入研究

## ——端侧 2B Jev 模型候选

> **三行说明**
> ① MiniCPM5-2B 是 2B dense Llama 架构模型，面向端侧/本地部署，模型卡重点展示代码、数学、工具调用和 Agent 能力。
> ② GGUF Q4_K_M 页面大小约 1.56GB，Apache 2.0，并提供 chat template；“下载快”和“CPU 快”仍需本机实测。
> ③ 它是 fast-router 的轻量 Jev 候选，但不是已验证的生产 scorer；准确率、延迟和 template 适配以[深度研究](./MiniCPM5-2B-深度研究.md)为准。

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

1. **小 + 潜在低资源门槛**：Q4_K_M 页面大小约 1.56GB，可能降低下载和内存门槛；实际下载耗时、峰值内存和 CPU 推理速度未验证
2. **Llama 架构**：与 llama.cpp 路径方向匹配，但仍必须使用 MiniCPM5 自己的 chat template；当前主线 wrapper 尚未实现 `fr_apply_template`
3. **端侧设计**：和 fast_router "本地推理"目标一致
4. **Agent/工具调用强**：可能对 task_type 分类更准（Agent 训练的模型理解任务边界更好）
5. **Apache 2.0**：可商用
6. **声称媲美 4B**：如果 P1 > Qwen2.5-1.5B（47%），说明 2B 级模型可达高 P1

### 待验证假设

- 2B Q4 在 CPU 上可能比 8B/9B 更易运行，但不能据参数量直接推导每次 forward 或 30 样例总耗时
- P1 可能超过 Qwen2.5-1.5B 的 47%，但“媲美 4B”通用 benchmark 不能替代 fast-router 任务分类 benchmark
- yes/no + 原生 Llama template 可能比当前单 token + 硬编码 prompt 更稳，但需要对照实验

## 五、局限

1. **只有 safetensors + GGUF（无 MLX 4bit）**：但 GGUF 够用（llama.cpp）
2. **Llama 架构 ≠ Qwen**：不同系列，趋势不可外推（教训 L-003）——但 Llama 比 Qwen 更标准，fr_apply_template 更可能正确适配
3. **hardcoded Qwen2.5 template**：当前 scorer 使用硬编码 `<|im_start|>`，Llama/MiniCPM5 的 chat template 可能不同——需增加并验证模型原生 template
4. **2B 可能不够**：声称媲美 4B，但 fast_router 测的 Qwen2.5-1.5B（47%）也是 ~2B 级——如果 MiniCPM5-2B ~47%，说明 2B 级天花板
5. **未实测**：MiniCPM5-2B 在 fast_router 上的 P1 未验证

## 六、结论

**MiniCPM5-2B 是 fast_router 的轻量 Jev 模型候选**——1.56GB + 2B + Llama + 端侧 + Agent 强。

**与 Ornith-1.5-9B 的定位差异**：
- Ornith 9B：权重和推理资源门槛更高，具体延迟需按后端实测
- MiniCPM5-2B：权重体积更小，可能更适合低资源探索，但 CPU 延迟和准确率仍未测

**理想组合**：
- CPU 场景：MiniCPM5-2B（快 + 轻）
- GPU/MLX 场景：Ornith-1.5-9B（准但需算力）

**下一步**：下载 MiniCPM5-2B-Q4_K_M 后，先完成 tokenizer/template smoke test，再跑 yesnobench 或等价 C3 benchmark；不预设耗时和 P1 结果。

**如果 MiniCPM5-2B P1 >47%**（超过 Qwen2.5-1.5B），只能说明在当前 30 样例集和指定 scorer 方法下取得更好结果；还不能据此断言“Llama 架构 + Agent 训练”普遍优于 Qwen，需要重复实验和方法对照。
