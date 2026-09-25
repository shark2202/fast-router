---
type: Research Note
title: fast_browser_use 如何复现 Jev 的 System 1 决策能力 —— 架构与可复用性研究
description: 逆向分析 /Users/mac/codes/fast-browser-use/fast_browser_use 的模型加载与单 token 打分机制，评估其架构范式与代码实现的可复用性，给出解耦改造路径。本结论为自产实证（读码推演级），非第三方复现；校准度未实测。
source_repo: /Users/mac/codes/fast-browser-use/fast_browser_use
read_at: 2026-09-25T00:20:00+08:00
method: 源码逐文件精读 + 机制抽象 + 耦合点枚举
applicability: 适用于"用现成对话模型冒充有界决策分类器"的范式判断；不适用于"RLCD 专门训练的校准分类器"场景
negative_example: 不可据此断言 fast_browser_use 的分数是校准过的正确性概率；其 model.py 自述分数为"候选归一化相对偏好，非校准的正确性概率"
---

# fast_browser_use 如何复现 Jev 的 System 1 决策能力

## ——架构与可复用性研究

> **三行说明**
> ① 研究对象：fast_browser_use 不重训模型，用现成 Qwen3.5-9B + 三个工程技巧把它变成"单 token 分类器"，复现 Jev 的 System 1 决策**机制与速度**。
> ② 核心结论：**范式可复用性高，代码不可直接复用**——被 Qwen 结构校验、MLX 缓存、浏览器动作空间三处耦合卡住。
> ③ 诚实边界：复现的是机制不是校准度；本研究为单模型家族自检（读码推演级），非第三方复现，换模型/换场景需实测"指令模型冒充分类器"假设是否成立。

---

## 一、研究对象与背景

**Jev**（TypeSafe AI，2026-09 中旬发布）提出 "System 1 vs System 2" 决策范式：

- **System 1（快思考）**：对有界、类型化的决策空间直接打分，跳过自回归解码，号称比传统 LLM 快 20–200×。
- **System 2（慢思考）**：GPT/Claude 式自由文本自回归推演。

Jev 通过 RLCD（Reinforcement Learning for Calibrated Decisions）专门训练，分数是校准过的；目前仅以云 API 提供，不公开权重。

**fast_browser_use** 是 Jev 的**本地开源逆向实现**，以浏览器自动化为落地场景：
- 模型：Qwen3.5-9B（MLX 4-bit / Apple Silicon）或 Qwen3.5-35B-A3B（PyTorch / CUDA）。
- 目标：100% 本地、离线、零云端成本。
- 关键区别：**不重训，靠现成对话模型 + in-context 指令冒充分类器**。

源仓库内相关文件（读取时点见 frontmatter）：
- `fast_browser_use/model.py`（核心：候选编码、单 token 打分、KV 前缀缓存、policy/plan/text 三个 System 2 入口）
- `fast_browser_use/text_backend.py`（MLX 模型加载，Qwen 结构硬校验）
- `fast_browser_use/torch_backend.py`（PyTorch 后端，覆盖 score/_stream）
- `fast_browser_use/agent.py`（agent 循环：predict/act/tick + StalePage 防抖）
- `fast_browser_use/actions.py`（浏览器动作空间索引）
- `docs/design.md`（设计说明：full-goal 执行、决策模式、settle 机制）

---

## 二、它如何实现 System 1 能力

### 1. 模型加载（backend 抽象 + Qwen 结构硬校验）

```
resolve_backend()       → Darwin+arm64 = mlx；否则 = torch（model.py L40）
model_location()        → huggingface_hub.snapshot_download，只下 *.json/*.jinja/*.safetensors
                          （显式丢弃 vision 权重，只保留 language_model）
load_text_model()       → MLX：校验 config 是 dense_9b 或 moe_35b（text_backend.py L11）
load_torch_model()      → PyTorch：校验同样结构参数，Qwen3_5ForCausalLM.from_pretrained
                          且校验无 missing_keys/mismatched_keys/error_msgs（torch_backend.py L46）
get_model()             → 单例 + _init_lock 懒加载（model.py L370）
```

两个后端实现同一接口（`score / generate_text / plan / think_score`），靠继承复用：`LocalModel`（MLX）被 `TorchModel` 继承，共享 `template/plan/generate_text/think_score`，各自覆盖 `__init__/score/_stream`。

### 2. 核心技巧①：候选 → 单 token 映射（System 1 的灵魂）

`candidate_codes(tokenizer, count)`（model.py L155）把每个候选动作映射成**恰好编码为 1 个 token 的标签**：

```python
for label in pool:  # A, B, C ... Z, AA, AB ...
    tokens = tokenizer.encode(label, add_special_tokens=False)
    if len(tokens) == 1 and tokens[0] not in ids:   # 单 token + 唯一 + 不共享首 token
        labels.append(label); ids.append(tokens[0])
```

于是"在 N 个候选里选一个"被压缩成"生成 1 个 token"。`score()` 取最后位置的 logits，**只在候选 token id 子集上 softmax**，得到每个候选的相对概率——**完全跳过自回归解码**。

这一步是整个 System 1 范式的核心：把分类问题伪装成"生成 1 个 token"的生成问题，复用 LM 的 logits 机制，但不付出自回归的成本。

### 3. 核心技巧②：KV-cache 前缀复用（MLX 专属优化）

`LocalModel.score`（model.py L233）把 prompt 里**不变前缀**（policy + goal，即 `\nPAGE:\n` 之前部分）的 KV cache 按用途存起来：

```python
prefix = tokenize(prompt.split("\nPAGE:\n")[0])[:-2]   # policy+goal，跨决策不变
if not hit:
    cached_state = make_prompt_cache(model); _prefill(prefix, cached_state)
cache = copy.deepcopy(cached_state)                      # 每次决策 deepcopy 一份
_prefill(remaining[:-1], cache)                          # 只 prefill 变化部分（页面+候选表）
logits = model(array([remaining[-1:]]), cache=cache)     # 只对最后 1 token 取 logits
probs = softmax(logits[label_ids[:count]])               # 候选子集 softmax
```

这让每次动作决策的算力几乎只花在"最后 1 个 token"上。

**注意不对称性**：PyTorch 后端（torch_backend.py L101）**没有**做这个优化——每次 `use_cache=False, logits_to_keep=1` 全量重算，只靠 `logits_to_keep=1` 省掉非末尾位置的投影。两个后端性能特性不一致，是复用时的一个隐患。

### 4. 核心技巧③：受限的 System 2（按需、短、强制 JSON）

System 2 不是被取代，而是被**严格约束**成只在必要时、以最小代价调用：

| 方法 | 用途 | 约束手段 | 位置 |
|---|---|---|---|
| `make_plan` | 生成步骤清单 | 强制 `{"steps":["` 前缀，≤512 token，流式解析，1–12 步 | model.py L300 |
| `generate_text` | 填表字段值 | 强制 `{"text":` 前缀，≤128 token，解析到合法 JSON 即停 | model.py L280 |
| `think_score` | 原生思考后决策 | Qwen thinking 通道 ≤2048 token，结束后在固定 `Action code: ` slot 单 token 打分 | model.py L290 |

`think_score` 是 System2+System1 的混合：让模型先"想"（自回归），但最后的决策仍是单 token 打分。

### 5. Agent 循环（agent.py）

```
tick = predict + act
predict: observe页面 → (可选 make_plan) → choose(score)
act:     执行动作 → observe
决策模式: joint（DONE 与动作同批打分，默认） / binary（先二值判完成度，再选动作）
防抖:    连续 3 个动作无页面变化 → blocked（agent.py 末段）
StalePage: 页面变了重新决策，不 double-click（predict 后若 fingerprint 不匹配则拒绝）
```

循环设计上有几个稳健性细节值得注意：
- 决策**消费一次即清空**（`state["decision"] = None`），retry 不会 double-click。
- 执行**先于** observe 记录进 history，避免 stale post-action observation 抹掉已执行动作。
- `MAX_STEPS * 2` 的模型调用预算 + `MAX_STEPS` 动作预算双层兜底。

---

## 三、可复用性评估

### 范式可复用性：高 ✅

这套组合技巧本身通用，不限浏览器：

1. 把决策空间枚举成有限候选集
2. 每个候选映射成单 token 标签
3. prefill 一次 prompt，对最后 token 取 logits 在候选子集 softmax
4. 复用不变前缀的 KV cache

可套用场景：分类、路由、工具选择、菜单/选项选择、配置选择、任何"从有限候选项里选一个"的决策。`candidate_codes()` 函数本身已相当通用。

### 代码可复用性：低 ⚠️（6 处强耦合）

| # | 耦合点 | 位置 | 问题 | 改造方向 |
|---|---|---|---|---|
| 1 | **模型硬绑定** | `text_backend.py` L11 / `torch_backend.py` L46 | 硬编码校验 Qwen3.5-9B / 35B-A3B 的 `hidden_size/num_layers/intermediate_size` 等结构参数 | 改成配置驱动 + 能力探测，只保留"语言模型权重完整"校验 |
| 2 | **候选 token 依赖 tokenizer** | `candidate_codes` model.py L155 | 依赖 tokenizer 能把 A/B/C 编成单 token（多数 BPE 成立，但不保证）；换 tokenizer 可能失败 | 加 tokenizer 适配层 + fallback 策略（多 token 标签降级） |
| 3 | **thinking 模板绑定 Qwen** | `think_score` model.py L290 | 依赖 Qwen chat template 以 `imdi` 开头 + 单 token `</think>` 边界 | 抽象成"模型原生思考协议"接口，换模型换实现或降级为纯 score |
| 4 | **MLX 缓存是 MLX 专有** | `LocalModel.score` model.py L233 | `make_prompt_cache`/`wired_limit`/cache `deepcopy` 是 mlx-lm API；Torch 后端没复用 → 两后端性能不对称 | 定义 `PrefixCache` 抽象，两后端各自实现，统一 score 接口 |
| 5 | **继承关系别扭** | `TorchModel(LocalModel)` torch_backend.py | Torch 继承 MLX 的 `LocalModel` 却覆盖几乎所有方法；语义上"Torch is-a MLX LocalModel"不成立 | 重构为 `BaseScoreModel` + `MlxModel`/`TorchModel` 平行实现 |
| 6 | **动作空间/policy 浏览器专用** | `actions.py` / model.py 顶部 POLICY | CLICK/TYPE_TEXT/SELECT + DONE/BLOCKED 与浏览器场景深度绑定 | 抽象成"域候选生成器"+"域 policy"可替换模块 |

### 复用到新场景的最小改造路径

```
1. 抽出 candidate_codes → 通用候选编码层（已基本通用，加 fallback 即可）
2. 抽出 score() 的"单 token 打分"核心 → BaseScoreModel.score（与后端无关）
3. LocalModel 拆成 BaseScoreModel + MlxModel/TorchModel，各自实现 KV 缓存策略
4. 模型加载从 Qwen 硬校验 → 配置驱动（只保留"语言模型权重完整"校验）
5. 替换 action_space/policy 为目标域的候选生成器
6. think_score 的"原生思考"做成可选协议（非 Qwen 模型降级为纯 score 或外接 System2）
```

---

## 四、诚实边界

1. **复现的是机制与速度，不是校准度**。fast_browser_use 用现成对话模型靠 in-context 指令冒充分类器；model.py 的 `decision_from_scores` 自述 `"score_note": "Candidate-normalized model scores; not calibrated correctness probabilities."`。Jev 原版是 RLCD 专门训练、分数校准过的。换模型/换场景时，"指令模型冒充分类器"假设是否成立需实测，不能想当然。

2. **本研究为单模型家族自检**。结论来自我（研究者）读码推演，未派 fresh 无上下文子代理独立复核（I-DISC 未满足）；属"现场演示级"证据，非第三方复现。若要进入可信赖决策，须另起无共享上下文审查。

3. **读取时点**：源码状态见 frontmatter `read_at`；fast_browser_use 仍在迭代，本结论的代码行号与机制描述会随上游变更失效。

4. **未实测性能**：本研究未跑 benchmark，"20–200×"是 Jev 官方宣称，fast_browser_use 的实际加速比未在本研究中验证。

5. **适用边界**：本评估适用于"用现成对话模型冒充有界决策分类器"的范式判断；不适用于"RLCD 专门训练的校准分类器"场景（那需要训练侧分析，本研究未覆盖训练侧）。

---

## 五、结论与后续动作建议

**结论**：fast_browser_use 用"单 token 候选打分 + KV 前缀缓存 + 受限 System 2"三件套，在开源 Qwen 上复现了 Jev 的 System 1 决策**机制**，速度收益真实，但其**范式通用、代码不通用**——直接 fork 到新场景会被 Qwen 结构校验、MLX 缓存、浏览器动作空间三处耦合卡住，需按第三节表格做 6 处解耦改造。

**后续动作（按优先级）**：

1. 若要复用到 fast-router 或其他域：先抽出 `BaseScoreModel` + 通用候选编码层两个核心抽象，其余按域替换。
2. 若要进入可信决策：派 fresh 无上下文审查员复核本结论（消解 I-DISC 缺口）。
3. 若要验证速度收益：跑一次 benchmark，对照自回归 baseline，得到实测加速比。
4. 若要验证校准度：跑一组已知正确答案的决策任务，看 softmax 分数与正确性的相关性——这是判断"指令模型冒充分类器"是否成立的关键实验。
