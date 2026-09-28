---
type: Research Note
title: "MiniMind 深度研究：微型 LLM 从零训练链路对 fast-router L4 的意义"
description: jingyaogong/minimind 调研——CPU/消费级 GPU 可训的 26M-64M 模型全链路（含白盒蒸馏），修订自进化评估 L4"模型自训练"的可行性判定。
source: GitHub API + README@master（2026-09-28 抓取）
timestamp: 2026-09-28
---

# MiniMind 深度研究

## 结论先行（三行）

> ① minimind 是 62.7k★ 的"从零训练微型 LLM"完整开源链路（Apache-2.0，Qwen3 对齐，26M-64M），**把 fast-router 自进化 L4（模型自训练/蒸馏）从"本机不可行"翻转为"低成本可行实验"**。
> ② 最短路径：Ornith-9B（73.3% P1）做教师标注 → minimind 白盒蒸馏/SFT 出 64M 任务分类器 → GGUF（Qwen3 对齐直通 llama.cpp）→ 换入 `model.fast_path`——**推理链路零改动**（仍是进程内 GGUF）。
> ③ 三条成本档位：本机 CPU 慢训（离线一次性，官方支持但"速度差异非常大"）/ 租 3090（~1.3¥/h，分钟级）/ 复用其原生蒸馏代码。死法：64M 分类上限未知（可能低于 laya 322M 的 56.7%，但蒸馏自本域分布更贴合）；训练需 Python 基建（仅离线，不破分发不变量）。

---

## 一、来源

| 来源 | 内容 |
|---|---|
| GitHub API（2026-09-28） | 62,774★ / 8,161 fork / Apache-2.0 / pushed 2026-09-22（极活跃）/ Python |
| README@master | 模型线、训练链路、硬件说明、数据格式、评测方法 |
| 关联 | huggingface.co/collections/jingyaogong/minimind + modelscope 镜像（本网络可达） |

## 二、方法

围绕自进化评估 L4 的四个判定问题定向核验：① 无 GPU 可训性 ② 蒸馏管线 ③ 数据格式（教师标签如何流入）④ 导出回 llama.cpp 的路径。

## 三、发现

| # | 事实 | 对 fast-router 的意义 |
|---|---|---|
| F1 | 模型线：minimind-3 64M（2026-04，结构/Tokenizer 对齐 Qwen3）、MoE 198M-A64M、历史最小 26M | fast tier 候选体量：64M ≈ 0.5B 的 1/8，CPU 单前向预计几十 ms |
| F2 | 全链路从零实现：Pretrain/SFT/LoRA/DPO/PPO/GRPO/CISPO/**Agentic RL**/**白盒蒸馏** | 蒸馏代码现成；Agentic RL 是远期（路由奖励强化） |
| F3 | 设备：`cuda` 不可用时可 `CPU`/`MPS` 运行，官方明示"训练速度与兼容性会有非常大的差异"；基线=单 3090 跑 SFT 1 epoch 2h；租卡 ~1.3¥/h | 本机 CPU 路径存在但慢；**租卡几分钟即可完成我们的小数据训练** |
| F4 | SFT 数据格式 `{"conversations":[{role,content}...]}` jsonl；主线数据含大量"模型蒸馏合成数据"（qwen3-4b 合成 10w 条 tool-call） | 教师标注→数据集是纯格式工作，无门槛 |
| F5 | Qwen3 生态对齐 + 官方兼容 llama.cpp/vllm/ollama 推理 | convert_hf_to_gguf 直通 → **换入 fast_path 后推理侧零改动** |
| F6 | 其选择题评测法："比较各候选的条件概率 p(x\|y) 取最大" | 与 fast-router 的 Choice 打分**方法论同构**——分类任务天然适配 |
| F7 | 中文主线（数据偏中文） | 与任务集语言分布匹配 |

## 四、对 L4 的修订（自进化评估文档联动）

原判定（2026-09-28 上午）："L4 方向成立，本机不可行，留作离线管线"。修订为：

**蓝图（低成本可行实验）**：
1. 数据：verdicts.jsonl（C7 已在积累）+ 30 样本种子集 + 可选合成扩充（9B 教师对历史 state 打标）
2. 训练：SFT/蒸馏 minimind-3（64M）为任务分类器——租 3090 分钟级，或本机 CPU 离线慢训
3. 导出：GGUF → 配置 `model.fast_path` 指向它
4. 效果预期：fast tier 从 0.5B（3.9s 首评，P1~23%）→ 64M（预计 <300ms 首评，P1 待测——下界 0.5B 的 23.3%，上界逼近教师 73.3%，且**蒸馏自本域分布，可比 laya 通用 checkpoint（56.7%）更贴合**）
5. 闭环：C7 verdicts 持续积累 → 周期性重训 → fast tier 持续进化（真·自进化回路）

**死法**：①64M 容量上限未知（laya 322M 也才 56.7%）②训练数据量需求（30 样本远不够，需万级——依赖 verdict 积累或合成）③Python 训练基建引入（仅离线工具链，推理不变量保持）④师生分布偏差（9B 的 73.3% 是上限不是保证）。

**可逆性**：完美——fast tier 本就是可插拔路径，训练产物只是一个 .gguf 文件。

## 五、局限

1. 本轮纯文档研究，未克隆/未实测训练与推理速度。
2. CPU 训练耗时未量化（官方仅定性"差异非常大"）；64M×万级样本×短序列的 CPU 实际耗时需实验。
3. 64M 分类器在我们 30 样本集上的 P1 完全未测——这是下一步 spike 的核心问题。
4. 白盒蒸馏（logits 级）需要教师可导出 logits——Ornith-9B 经 llama.cpp 可取 logits ✓（fr_score 已在做），但与 PyTorch 训练侧的对接细节未研究。

## 六、结论

**minimind 把"自训练一个 fast-router 专属小模型"的成本打到了"租卡几分钟 + 一份数据集"的量级**，且产出物无缝落回现有进程内推理链路。L4 从研究远景升格为可执行实验，前置依赖是数据积累（C7 verdicts）与一次 30 样本 P1 验证。建议排序：verdict 数据积累（自动进行中）→ 64M 蒸馏 spike（半天）→ 视 P1 决定是否替换 fast tier。

## 附录：64M 地板 Spike 实测（2026-09-28）

**管线验证（全部打通，端到端 ~1 小时）**：

```
full_sft_768.pth (137MB, modelscope)
  → convert_model.py 路径: 权重载入 Qwen3ForCausalLM(strict=True, 0 缺失)
  → save_pretrained fp16 (127.8MB safetensors, 真 Qwen3 架构)
  → llama.cpp convert_hf_to_gguf（补丁: get_vocab_base_pre 回退 qwen2）
  → /tmp/minimind-3.gguf (128MB f16)
  → yesnobench（进程内 zig backend）
```

**实测数据**：

| 指标 | minimind-3 64M | 对照（Ornith-9B） | 对照（Qwen0.5B） |
|---|---|---|---|
| P1（zero-shot） | **10% (3/30) = 随机** | 73.3% | 23.3% |
| P2/样本（10 前向） | **1.09s** | 38.6s | ~4s |
| 单前向 | ~110ms | ~3.9s | ~400ms |

**过程修复（已入库）**：minimind 中文向词表里 "yes" 非单 token——scorer 与 yesnobench 增加肯定词回退链（yes→Yes→是，prompt 同步用词，探针实测 Yes=3376 / 是=357 均单 token）。

**结论**：
1. zero-shot 地板 = 随机 → **蒸馏训练是硬前提**（L4 蓝图的"训练后预期 ≥23.3%"中，"训练后"三字是全部重量所在）。
2. 速度地板优秀：64M 单前向 110ms，蒸馏后做 fast tier 首评预计 <1.1s（0.5B 的 1/3.5），batch KV 后更低。
3. 下一步（L4 蒸馏训练 spike）：合成训练集（10 类 × 模板扩充 + 可选 9B 教师标注）→ CPU SFT（63.9M × 万级短样本，预计小时级）→ 复测 P1。训练代码全部现成（minimind repo train_model.py）。
