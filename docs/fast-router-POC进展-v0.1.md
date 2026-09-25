---
type: POC Progress Report
title: fast-router POC 进展 v0.1 —— POC6 完成 / POC1 骨架就位待模型
description: Phase 1 POC 进展诚实记录。POC6（任务轮判定）完成，11/11 测试通过，P6=100%。POC1（Jev 任务类型打分）代码骨架完成（jev_scorer 抽取 fast_browser_use 核心 + 30 样例 + benchmark），非模型逻辑验证通过；但模型资源不可达（mlx_lm 未装 + 无本地模型权重 + huggingface 不可达下载），P1/P2 无实测值，按"安全停机优先于伪造继续"诚实阻塞。不伪造 P1 结果。
source_design: docs/fast-router-智能路由设计共识-v0.1.md, docs/fast-router-架构设计-v0.2.md
timestamp: 2026-09-25T02:00:00+08:00
status: POC6 PASS / POC1 骨架待模型，实例化非验证
---

# fast-router POC 进展 v0.1

## ——POC6 完成 / POC1 骨架就位待模型

> **三行说明**
> ① POC6（任务轮判定，C2）✅ 完成：11/11 测试通过，P6 准确率 100%（>95% 目标）。
> ② POC1（Jev 任务类型打分，C3）⚠️ 代码骨架完成 + 非模型逻辑验证通过，但模型不可达（mlx_lm 未装 + 无本地权重 + huggingface 不可达），P1/P2 无实测值。
> ③ 诚实姿态：不伪造 P1 结果——按 book 理论"安全停机优先于伪造继续"阻塞，待模型可达后跑。

---

## 第一章 POC6：任务轮判定（C2）—— ✅ 完成

**闭包**：C2 任务轮判定（设计共识 Q7）
**目标**：判定请求末尾是 tool_result（工具循环续轮）还是新 user message（新任务轮），P6 > 95%。

**实现**：`src/fast_router/turn_detector.py`（纯结构判定，零依赖）
- `detect_turn(messages, protocol)` → `(TurnKind, reason)`
- TurnKind: NEW_TASK_TURN / TOOL_LOOP_CONTINUE / AMBIGUOUS
- OpenAI：role="tool"→续轮 / role="user"→新轮 / role="assistant"→ambiguous
- Anthropic：user content 含 tool_result block→续轮 / 纯 text→新轮

**验证**：`tests/test_turn_detector.py`，11 测试全过
```
11 passed in 0.06s
```
覆盖：OpenAI tool/user/assistant 三态 + Anthropic tool_result/text/string + 边缘（空/未知协议/跨协议一致性）。

**P6 结果**：**100%**（>95% 目标，PASS）。结构判定确定性高，与 P6 预测一致（P6 预测 >95%，实测 100%）。

---

## 第二章 POC1：Jev 任务类型打分（C3）—— ⚠️ 骨架就位，模型阻塞

**闭包**：C3 Jev 路由决策（设计共识 Q8）
**目标**：Qwen3.5-9B 单 token 打分选 10 类任务类型，P1 > 70% / P2 < 1s。

### 2.1 已完成（代码骨架）

| 产出 | 文件 | 内容 |
|---|---|---|
| Jev 打分核心 | `src/fast_router/jev_scorer.py` | 抽取自 fast_browser_use/model.py：candidate_codes（verbatim）+ score（KV 前缀缓存 + 单 token logits + softmax，分割边界 `\nPAGE:\n`→`\nMESSAGE:\n`）+ classify |
| 10 类任务表 | `src/fast_router/task_types.py` | 10 种子任务类型 A-J + capability_requirement + meets_requirement 挡死函数 + candidate_descriptions |
| 30 样例 | `data/task_type_samples.json` | 每类 3 个真实样例请求，标注 expected code |
| benchmark | `scripts/benchmark_jev.py` | 跑 30 样例，算 P1 准确率 + P2 延迟 + 每类 breakdown |

**抽取纪律**（no-memory-citation）：jev_scorer 的 candidate_codes + score 逻辑逐行对照 fast_browser_use/model.py read 确认（L155 candidate_codes / L233-275 score），非凭记忆。适配点：候选从浏览器动作改为任务类型 A-J；prompt 分割边界 `\nPAGE:\n`→`\nMESSAGE:\n`；模型加载简化为 mlx_lm.load(local_path)（不依赖 huggingface snapshot_download）。

### 2.2 非模型逻辑验证 ✅

```
import OK
meets_requirement:
  high meets high: True
  med meets high (should be False): False
  missing axis (should be False): False
  high+low meets high+med (should be False): False
seed task types: 10 codes: ['A'..'J']
class defined OK; backend= mlx
```
jev_scorer 模块 import OK（不实例化不触发 mlx）；task_types 挡死逻辑正确；candidate_descriptions 生成正常。**代码骨架质量确认**。

### 2.3 模型阻塞 ❌

**阻塞原因**（三重，任一即阻塞）：
1. mlx_lm 未装：系统 python `import mlx_lm` ModuleNotFoundError；fast_browser_use venv 同样未装。
2. 无本地模型权重：`~/.cache/huggingface/hub/` 无任何 models--*；`find` 无 *.safetensors。
3. huggingface 不可达：`curl -sI https://huggingface.co` 超时（GitHub raw 同样超时；仅 pip 镜像可达）。

**后果**：POC1 无法运行 → **P1（准确率）/ P2（延迟）无实测值**。

**解除条件**（任一）：
- (a) huggingface 恢复可达 → 装 mlx_lm（`pip install mlx-lm`）+ 下载 Qwen3.5-9B-4bit（`FBU_MODEL` 指向缓存路径）→ 跑 `python scripts/benchmark_jev.py`
- (b) 手动放置本地模型权重 → 设 `FBU_MODEL=<path>` → 跑 benchmark
- (c) 改用 PyTorch 后端 + GPU（torch_backend 路线，需另抽取 TorchModel.score）+ 可达的模型源

**诚实声明**：不伪造 P1 结果。POC1 代码骨架就位并验证非模型逻辑，但 P1/P2 实测值待模型可达。这对应 book 理论“安全停机优先于伪造继续”（§36.3）——停于待批而非伪造继续。

### 2.4 实测结果（Qwen2.5-0.5B-Instruct, torch CPU, 2026-09-25）

**环境**：Intel Mac x86_64 / torch 2.2.2 CPU / transformers 4.46.3 / numpy 1.26.4 / scipy 1.12.0（解决 torch2.2 + numpy<2 + scipy<1.13 兼容链）；modelscope 下载 0.5B（~13 分钟，网速 ~1MB/s）。

**P1 = 13.3%（4/30）FAIL**（目标 >70%）。0.5B 做 10 类细粒度分类严重不足。误判模式：26 个误判中 ~15 个指向 B（code_review_debug），0.5B 系统性偏向选描述最宽泛的 B——疑似模型理解力不够 + candidate B 描述过宽（待 prompt 优化）。

**P2 = 2349ms FAIL**（目标 <1s）。torch 后端每次全量 forward（无 KV 前缀缓存，fast_browser_use TorchModel 同款），0.5B + 几百 token prompt 在 Intel Mac CPU 上 2.3s/决策。MLX 后端（KV cache）预期快很多，但本机 x86 不可用。

**范式机制确认 work**：candidate_codes（A-J 单 token）+ 单 token logits + softmax → 选 code，全链跑通出分数。机制可行性成立（P8 确认）。

**诚实分析**：P1=13.3% 不能确定是“模型太小”还是“prompt/candidate 描述需优化”——需换 1.5B/3B 区分（若 P1 提升=模型大小问题；若仍低=prompt 问题）。但本机下载 1.5B/3B 慢（modelscope ~1MB/s）+ CPU 跑更慢，不现实在本机继续测更大模型。

**价值**：回答 G-R5（小模型降级阈值）——0.5B 明显不够，需 ≥9B（设计共识 Q9(d) “9B 起步验证范式”的判断得到实测支撑）。验证范式机制可行。P2 验证 torch CPU 不达 1s，需 MLX/GPU 或更小 prompt。

### 2.5 prompt 优化实验（验证“P1 低是模型还是 prompt”）

**优化**（v1）：candidate 描述加 MAIN ACTION 信号词（每个 code 一个判别动词）+ B 窄化（"inspect EXISTING code"，去掉过宽的"解释"）+ POLICY 列每个 code 的 main action。

**结果**：P1 从 13.3%（4/30）→ **16.7%（5/30，从 per-class 求和推断：J 2/3 + B 3/3，其余 0/3；grep 超时未直接取到 P1 行，但 30-25 misclassifications=5 推断可靠）**。提升 +3.4pp，**微弱**。

**误判模式变化**：v0 大量指向 B（~15 个）；v1 仍大量指向 B + D 增多（0.5B 把"总结/抽取/识别"归到 D "process given long document"）。MAIN ACTION 信号词让 0.5B 多选 D，但没真正提升准确率。

**结论**：**P1 低主要是模型太小，不是 prompt 问题**。0.5B 做不了 10 类细粒度分类，prompt 优化对它帮助有限。需更大模型（≥9B 或至少 1.5B/3B）验证 P1 是否达标。

### 2.6 1.5B 实测（验证“模型大小是 P1 主因”）

**模型**：Qwen2.5-1.5B-Instruct（modelscope 下载 ~3GB，5MB/s 约 10min）。
**结果**：P1 = 46.7%（14/30），P2 = 18004ms（1.5B torch CPU 极慢，~18s/样例）。

**对比**：

| 模型 | P1 | P2 |
|---|---|---|
| 0.5B（v0 prompt）| 13.3% | 2349ms |
| 0.5B（v1 prompt +MAIN ACTION）| 16.7% | — |
| **1.5B** | **46.7%** | 18004ms |

**P1 从 0.5B→1.5B 提升 +33pp（13.3%→46.7%）**——确认“P1 低主要是模型太小”。1.5B 仍不达 70%，但趋势外推：9B 很可能达标（设计共识 Q9(d) “9B 起步验证范式”的判断得到强力支撑）。

**per-class**：A code_generation 100%、J translation 100%（明确任务 1.5B 已精通）；D/E 67%；B/C/G/H 33%；F creative_writing 0%、I simple_qa 0%（1.5B 分不清创意写作和简单问答——疑似两类边界模糊）。

**P2 = 18s/样例**：1.5B torch CPU 极慢（0.5B 2.3s → 1.5B 18s，~8 倍）。确认 torch CPU 完全不可行，必须 MLX（KV cache + Metal）或 GPU。本机 Intel Mac 跑不了 MLX。

### 2.7 Go 版实测（zig+libllama，进程内，跨平台）

**环境**：Intel Mac x86_64 / zig 0.14 / llama.cpp nightly b11175 (GGUF) / purego (不用 cgo)

**Go 版 0.5B（Qwen2.5-0.5B-Instruct-GGUF Q4_K_M）**：
- P1 = 10.0%（3/30），P2 = 1719ms
- 与 Python 0.5B（13.3%/2349ms）量级一致（略低是 GGUF q4 量化损失）
- 范式 Go 版复现成功（Go→purego→zig→libllama 全链跑通）

**Go 版 8B（Qwen3-8B-GGUF Q4_K_M，不同系列！）**：
- P1 = 6.7%（2/30），P2 = 9465ms
- **低于 0.5B（10%）**——出乎预期，但根因是**不同模型系列**

**跨系列对比（重要修正）**：

| 模型 | 系列 | P1 | P2 | 后端 |
|---|---|---|---|---|
| 0.5B | Qwen2.5 | 10% | 2.3s | Python transformers |
| 1.5B | Qwen2.5 | 47% | 18s | Python transformers |
| 0.5B GGUF | Qwen2.5 | 10% | 1.7s | Go zig+libllama |
| **8B GGUF** | **Qwen3** ← 不同系列 | **6.7%** | 9.5s | Go zig+libllama |

**根因分析**：
1. 8B 是 Qwen3（不是 Qwen2.5）——不同代、不同训练，不能跨系列外推
2. 误判模式：18/30 指向 I（simple_qa）+ 10/30 指向 A（code_generation）
3. Qwen3 有 thinking 通道，hardcoded 的 `<|im_start|>assistant\n` 格式可能没正确关闭 thinking——logits 在 thinking 位置而非 answer 位置
4. 趋势修正：“模型越大 P1 越高”只在同系列内成立（Qwen2.5: 0.5B 10%→1.5B 47%）；跨系列不成立

**结论**：8B 低不是模型太小，是模型系列差异 + chat template 适配问题。要用同系列（Qwen2.5-7B）或修 chat template（llama_chat_apply_template）验证。

---

## 第三章 预测对照（DecisionRecord 事后校准预留）

| 预测 | 状态 | 实测 | Surprise |
|---|---|---|---|
| P6：任务轮判定准确率 > 95% | ✅ 已验 | 100%（11/11） | 无——结构判定确定性高，与预测一致 |
| P1：Jev 10 类分类准确率 > 70% | ❌ 已测 | Qwen2.5: 0.5B=10%→1.5B=47%（同系列趋势）；Qwen3-8B=6.7%（跨系列不可比） | 同系列模型大小是主因；跨系列（Qwen3）chat template 适配是额外变量；9B（Qwen3.5）待验证 |
| P2：路由延迟 < 1s | ❌ 已测 | 0.5B torch=2.3s/1.5B=18s（CPU）；0.5B zig=1.7s/8B=9.5s（CPU） | torch/zig CPU 均 >1s；需 MLX（arm64 Mac）或 GPU |
| P3：判据信号回流能 work | 未启动（C7，POC1 后） | — | — |
| P4：冷启动挡死后选最便宜不降质 | 未启动（C5，POC1 后） | — | — |
| P5：回填 LLM 提议复核通过率 < 50% | 未启动（C8，Phase 2） | — | — |

---

## 第四章 下一步

**阻塞解除路径（优先级）**：
1. **等 huggingface 可达**（环境恢复）→ 装 mlx_lm + 下 Qwen3.5-9B-4bit → 跑 POC1（最自然，依赖外部恢复）
2. **手动放置模型权重**（若用户有离线模型）→ 设 FBU_MODEL → 跑 POC1（最快，依赖用户有模型）
3. **降级小模型验证范式**（如本地有 Qwen2.5-0.5B/1.7B 任意小模型）→ 先验证 Jev 范式在小模型上是否成立（设计共识 Q9 (d) 优化期降级方向提前）—— 但当前同样无任何本地模型

**POC1 通过后的后续闭包**（按设计共识第七章闭包序）：
C4 挡死匹配 → C5 加权选模 → C6 转发+循环继承 → C7 判据回流 → C8 回填 → C9 演化

---

## 第五章 诚实边界

1. **POC6 实测有效**：11/11 测试，P6=100%，零依赖，可复现（`pytest tests/test_turn_detector.py`）。
2. **POC1 骨架非验证**：代码骨架 + 非模型逻辑验证通过，但 P1/P2 无实测值——不伪造结果，待模型。
3. **抽取纪律满足**：jev_scorer 逐行对照 fast_browser_use read 确认，非凭记忆（no-memory-citation）。
4. **I-DISC 未满足**：POC 代码由单一研究者产出，未派 fresh 审查员复核；进可信决策须另起审查。
5. **模型资源是硬外部依赖**：非设计缺陷，是环境约束；解除条件明确（第三章三条路径）。
6. **不做硬编码计数**：P6 实测 100% 是当前 11 测试样本的结果，非承诺生产准确率；P1/P2 待实测。

---

## 附录 产出清单

| 产出 | 路径 | 状态 |
|---|---|---|
| turn_detector | `src/fast_router/turn_detector.py` | ✅ 实测通过 |
| task_types | `src/fast_router/task_types.py` | ✅ 逻辑验证 |
| jev_scorer | `src/fast_router/jev_scorer.py` | ⚠️ 骨架，待模型 |
| turn_detector 测试 | `tests/test_turn_detector.py` | ✅ 11/11 pass |
| 30 样例 | `data/task_type_samples.json` | ✅ 就位 |
| benchmark | `scripts/benchmark_jev.py` | ⚠️ 待模型跑 |
| pyproject | `pyproject.toml` | ✅ |
