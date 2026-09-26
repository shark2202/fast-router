---
type: Research Note
title: MiniCPM5-2B 深度研究 —— 面向 fast-router C3 Jev scorer 的适配评估
description: 基于 Hugging Face 模型卡、配置文件、GGUF 模型仓库和官方关联论文，对 MiniCPM5-2B 的模型事实、训练与评测声明、部署路径、工具调用、单 token 打分适配性和 fast-router 集成风险进行核验。
source: https://huggingface.co/openbmb/MiniCPM5-2B/blob/main/README-cn.md
read_at: 2026-09-26T00:00:00-04:00
method: 官方模型卡与 config.json 逐段核对 + GGUF 仓库核对 + arXiv 编号反查 + 与 fast-router 当前 Go/Zig 实现对照
applicability: 用于判断 MiniCPM5-2B 是否适合作为 fast-router C3 Jev 任务类型 scorer 的本地模型，以及规划最小验证实验
negative_example: 不可据此断言 MiniCPM5-2B 已在 fast-router 的单 token 分类任务上达标；模型卡通用 benchmark、模型服务示例和真实路由分类准确率不是同一证据
---

# MiniCPM5-2B 深度研究

## ——面向 fast-router C3 Jev scorer 的适配评估

> **研究日期**：2026-09-26（America/New_York）
>
> **一句话结论**：MiniCPM5-2B 是一个比当前 0.5B/1.5B POC 更值得验证的 C3 候选：参数规模足够、面向 coding agent/tool-use、本身提供 GGUF/llama.cpp 路径、使用标准 `LlamaForCausalLM`。但它不是“下载即接入”：fast-router 当前 wrapper 默认 CPU、上下文 4096、硬编码 Qwen 风格 prompt，且尚未验证 MiniCPM5 的 chat template、thinking 边界和 A-J 单 token 条件。

## 1. 来源与研究方法

### 1.1 主要来源

1. [MiniCPM5-2B 中文模型卡](https://huggingface.co/openbmb/MiniCPM5-2B/blob/main/README-cn.md)
2. [MiniCPM5-2B `config.json`](https://huggingface.co/openbmb/MiniCPM5-2B/blob/main/config.json)
3. [MiniCPM5-2B-GGUF 模型仓库](https://huggingface.co/openbmb/MiniCPM5-2B-GGUF)
4. [arXiv:2506.07900](https://arxiv.org/abs/2506.07900)
5. [arXiv:2602.09003](https://arxiv.org/abs/2602.09003)
6. 本仓库 `router/scorer.go`、`router/zig_backend.go`、`zig/frwrapper.zig`、`docs/LLM2Jev-研究笔记.md` 和 `WIKI/`。

### 1.2 方法

* 以模型卡为产品事实入口，逐段抽取模型定位、架构、训练、评测、推理命令、工具调用和许可信息。
* 用 `config.json` 复核模型卡中的架构数字，不把自然语言“2B”当作精确参数事实。
* 反查模型卡列出的 arXiv 编号，判断它们是否真的是 MiniCPM5 技术报告。
* 查看 GGUF 仓库的量化元数据，识别模型卡、仓库元数据和自动页面之间的差异。
* 把模型要求逐项映射到 fast-router 当前 C3 Backend 的实际边界，而不是只做通用模型介绍。

## 2. 模型身份与硬事实

### 2.1 官方定位

MiniCPM5-2B 是 MiniCPM5 系列的 2B 级稠密 Transformer，定位是端侧、本地部署、资源受限场景，重点场景包括本地助手、coding agent、工具调用流程和紧凑推理。模型卡称其在所选 2B 对比集合中达到 SOTA，并可与部分 4B 级模型竞争；这是模型作者对特定比较集合的结论，不是全市场或独立复核结论。

### 2.2 `config.json` 的架构事实

| 字段 | 值 | 对 fast-router 的意义 |
|---|---:|---|
| `architectures` | `LlamaForCausalLM` | Go/Zig 可以优先走通用 llama.cpp 路径，不需要模型代码 fork |
| `model_type` | `llama` | 与当前通用 llama.cpp wrapper 的方向一致 |
| 精确参数量 | 2,516,756,480 | “2B”是四舍五入宣传名；精确值来自模型卡 |
| 非嵌入参数 | 1,981,982,720 | 评估实际计算规模时应区分 embedding 参数 |
| `hidden_size` | 2048 | 标准 dense decoder 规模 |
| `intermediate_size` | 6144 | MLP 宽度 |
| `num_hidden_layers` | 42 | 层数较高，CPU 单次 forward 仍需实测 |
| Q/KV heads | 16 / 2 | GQA，KV cache 相对节省 |
| `head_dim` | 128 | 16 × 128 = 2048 |
| `max_position_embeddings` | 131072 | 模型声明 128K 级原生上下文 |
| `rope_theta` | 5000000 | 长上下文 RoPE 配置 |
| `vocab_size` | 130560 | 候选 label 是否单 token 必须使用该 tokenizer/GGUF 实测 |
| `eos_token_id` | `[1, 130073]` | 存在多个 EOS id；生成/停止逻辑需依赖运行时处理 |
| `torch_dtype` | `bfloat16` | BF16 正式权重适合 GPU；本地 CPU 通常应使用 GGUF |
| `use_cache` | `true` | 与 prefix/KV cache 类优化方向一致 |

模型卡同时写明标准 `LlamaForCausalLM`，主模型卡列出总参数 2,516,756,480、42 层、16 Q heads/2 KV heads、131,072 上下文；配置文件进一步给出上表的细节。

### 2.3 参数量元数据存在冲突

GGUF 仓库页面底部的 Hugging Face 自动元数据显示“3B params”，并列出 Q4_K_M 约 1.56 GB、Q8_0 约 2.68 GB、F16 约 5.04 GB；主模型卡和 `config.json` 则给出精确 2.516B 参数。因此：

* 采购、内存预算和 Wiki 应以权重文件实际大小、GGUF metadata 和本地运行峰值为准。
* 不应把 GGUF 页面自动标签“3B”当作架构事实。
* “2B/3B”对比 benchmark 选组时要记录口径，避免把不同页面标签混在一起。

## 3. 训练与能力：哪些结论可信到什么程度

### 3.1 模型卡宣称的训练流程

模型卡描述三阶段训练：base training、mid-training 和后训练。后训练包括：

1. 400B tokens 的 deep-thinking SFT。
2. 面向数学、代码、Agent、写作等方向训练专用 RL teacher。
3. 使用 On-Policy Distillation（OPD）把 teacher 能力合并回发布模型。
4. OPD 使用 16 个 RL 专家，其中包含 5 个 agentic 专家；对 response 每个位置计算学生与 teacher 的全词表 reverse KL 作为优势估计。

这些信息足以支持“该模型的 post-training 明确重视 reasoning/agent/tool-use”的工程判断，但不能单独证明它适合 fast-router 的**有界任务分类**。Agent 生成能力、工具调用能力和单 token 选择校准度是三个不同问题。

### 3.2 评测结果的正确读法

模型卡给出的平均分为 53.9，并列出代码、数学、指令遵循、长文本、工具调用、代码智能体、搜索智能体和通用智能体等项目。表格说明：带 `†` 的分数来自 Artificial Analysis，其余为内部复现。

对 fast-router 有价值的信号包括：

* 工具调用和 agent benchmark 被纳入评测，而不是只有通用问答。
* coding/数学/长文本方向与 fast-router 的任务类型轴有重叠。
* 模型规模较小但能力密度高，符合本地 scorer 的内存目标。

不能直接推出的结论包括：

* benchmark 高不等于 Jev 任务类型分类准确率高。
* tool calling 成功不等于输出固定 `A`-`J` label 的概率分布校准。
* 2B 级通用能力强不等于一次 CPU forward 小于 1 秒。
* 模型卡中的内部复现不是独立第三方复核。

## 4. 推理与部署事实

### 4.1 官方支持矩阵

模型卡列出 Transformers、vLLM、SGLang、llama.cpp、Ollama、LM Studio、MLX、LiteRT-LM 等路径；并提供 BF16/FP16、GGUF、MLX/4bit 等模型变体。对当前 fast-router 最相关的是：

* **GGUF + llama.cpp**：与现有 Zig + llama.cpp 设计直接对齐。
* **MLX/4bit**：适合 Apple Silicon，但当前 Go 主线没有 MLX backend。
* **vLLM/SGLang**：适合作为远程或本机 OpenAI-compatible scorer 服务，不符合当前“Go 进程内 purego + Zig”目标，但可作为对照基线。

### 4.2 llama.cpp 运行要求

官方示例使用：

```bash
llama-server -m MiniCPM5-2B-F16.gguf \
  -a MiniCPM5-2B --port 8080 -ngl 99 -c 8192 --jinja
```

模型卡特别要求将 `min_p` 设为 `0.0`，因为 llama.cpp 默认 `min_p=0.05` 可能过滤掉打破重复循环所需的 token。这个建议主要针对**生成**，不应机械套用到 C3 的 logits 评分；C3 应直接读取候选 logits，并记录是否使用任何 sampling filter。

两个对 fast-router 的直接影响：

1. `--jinja` 说明必须使用模型自身 chat template；不能假定所有 Llama 架构都能用 Qwen2.5 的硬编码 prompt。
2. 官方示例只把上下文设为 8192，尽管模型配置声明 131072；实际运行上下文取决于显存/内存、KV cache 和 runtime 配置。

### 4.3 `chat_template.jinja` 的关键行为

直接查看仓库中的 `chat_template.jinja` 后，可以把 thinking 风险具体化，而不是只写“可能有 thinking”：

* 生成提示固定从 `<|im_start|>assistant\n` 开始。
* 当 `enable_thinking=false` 时，模板明确追加 `<think>\n\n</think>\n\n`，然后才进入答案槽。
* 当 `enable_thinking=true` 时，模板追加 `<think>\n`，模型会先进入思考内容。
* 当没有显式传入 `enable_thinking` 时，模板只保证 assistant 起始，不自动替 fast-router 选择答案槽。
* assistant 历史消息会解析已有 `<think>...</think>`，并将 reasoning 与最终 content 分开重组。

因此，C3 若要取 `A`-`J` 的答案 logits，最小可靠路径是显式使用 no-think 模式对应的模板输出，或实现“先结束 thinking、再在 answer slot 取 logits”的状态机。不能只拼接 `<|im_start|>assistant\n` 就假定位置正确。

该模板还把工具调用编码为 XML `<function name="..."><param ...>...</param></function>`，把工具返回编码为 `<tool_response>...</tool_response>`；这解释了模型卡为什么推荐专门的 tool parser。

### 4.4 工具调用格式

模型卡说明 MiniCPM5-2B 原生输出 XML 风格工具调用，推荐使用 SGLang 的 `minicpm5` parser 转为 OpenAI `tool_calls`。这意味着：

* 使用 SGLang/OpenAI server 时，工具调用解析由 parser 负责。
* 直接使用裸 llama.cpp 或自建 Gateway 时，不能默认收到标准 OpenAI `tool_calls` JSON；需要使用模型 chat template 和 XML parser，或验证 llama.cpp 当前 parser 行为。
* fast-router 的 C3 分类本身不需要工具调用，但 Gateway 转发 agent 请求时，这会影响 tool-use 兼容性。

## 5. 与 fast-router 当前实现的逐项对照

### 5.1 有利匹配

| MiniCPM5-2B 特征 | fast-router 现状 | 判断 |
|---|---|---|
| 标准 `LlamaForCausalLM` | Zig 通过 `llama.h` 调用 | 架构方向匹配 |
| GGUF 官方变体 | SOP/`model_download.go` 面向 GGUF | 可接入现有模型路径 |
| 本地/端侧定位 | fast-router 目标是本地网关 | 产品场景匹配 |
| coding/agent/tool-use 后训练 | C3 要识别 code、tool、reasoning 等任务轴 | 值得做候选实验 |
| GQA 16Q/2KV | llama.cpp 可管理 KV cache | 有利于内存，但速度需实测 |
| Apache-2.0 | 当前项目本地部署/分发计划 | 许可方向相对友好，仍需审查依赖与数据许可 |

### 5.2 直接冲突

#### 冲突 A：fast-router 仍硬编码 Qwen 风格 prompt

当前 `router/scorer.go` 的 `buildChoicePrompt` 拼接：

```text
<|im_start|>user
...
<|im_end|>
<|im_start|>assistant
```

这是一种 Qwen 风格假设，不是“标准 LlamaForCausalLM 的统一协议”。MiniCPM5 官方 llama.cpp 示例要求 `--jinja`，Transformers 示例使用 `apply_chat_template`；因此 MiniCPM5 接入前必须先获取并执行它自己的 template。

#### 冲突 B：当前 C3 直接在 assistant 位置取候选 logits

`frwrapper.zig` 在 prompt decode 后读取最后位置 logits，再从候选 token 中取分数。MiniCPM5 的官方 Transformers 示例显式开启 `enable_thinking=True`；如果 chat template 在 assistant 起始处插入 thinking 段，直接在错误位置取 `A`-`J` logits 可能产生系统性偏差。

正确做法不是猜测“是否有 thinking”，而是做两个明确实验：

1. `enable_thinking=false` 或等价 no-think template：测候选 label 的最后位置。
2. `enable_thinking=true`：确定需要先生成/跳过多少 thinking token，再在哪个 answer slot 评分。

#### 冲突 C：当前 wrapper 把上下文固定为 4096

`frwrapper.zig` 使用 `cp.n_ctx = 4096`。这不会让模型失效，但会把 MiniCPM5 声称的 131072 上下文能力截断到 fast-router 自己的 4096；对“只取新 user message 首 N token”的设计可能足够，但必须将它写成 fast-router 的策略约束，不能宣称使用了模型的完整长上下文。

#### 冲突 D：当前 wrapper 默认 CPU

`frwrapper.zig` 设置 `mp.n_gpu_layers = 0`。2B Q4 模型的可加载性看起来比 8B/9B 友好，但 P2 `<1s` 仍不能由参数量推断。应测：prefill、单候选 logits、10 候选打分、重复调用、冷/热 KV cache。

#### 冲突 E：单 token 条件尚未核验

C3 假定 `A`、`B`、……、`J` 在目标 tokenizer 中是单 token。MiniCPM5 的 vocab size 是 130560，但 vocab 大小不等于这些字符串一定单 token。必须对**实际 MiniCPM5 tokenizer/GGUF vocab**运行 probe；如果某些候选不是单 token，当前 `fr_score` 会返回 `-6`。

### 5.3 与 LLM2Jev 研究的联动

本仓库已有 `docs/LLM2Jev-研究笔记.md`，指出单 token 多候选方案可能受位置偏好、thinking 边界和硬编码 chat template 影响。MiniCPM5 的模型卡进一步强化了其中两个风险：它显式使用 chat template，且推荐 tool parser/生成模板，而不是裸 prompt。

因此 MiniCPM5 不应只作为“换一个更大模型”实验；它应作为**prompt/template 与 scorer 方法同时验证**的实验。至少要比较：

* 当前单 token 多候选 softmax。
* 使用模型原生 template 的单 token logits。
* 每候选独立 yes/no 评估 + 前缀缓存。

## 6. 论文与引用链核验

模型卡顶部列出了 `arxiv: 2506.07900` 和 `arxiv: 2602.09003`，但反查结果是：

* `2506.07900` 的标题是 **MiniCPM4: Ultra-Efficient LLMs on End Devices**，不是 MiniCPM5 技术报告。
* `2602.09003` 的标题是 **Data Science and Technology Towards AGI Part I: Tiered Data Management**，它解释 UltraData 分级数据管理背景，也不是 MiniCPM5 专属技术报告。
* 模型卡底部引用 BibTeX 仍然写的是 `minicpm4` 和 `MiniCPM4`。

这不一定否定 MiniCPM5 的模型事实，但说明当前模型卡的论文溯源不完整或复用了 MiniCPM4 模板。对研究记录应把“模型卡声明”和“专属论文证据”分开；在没有 MiniCPM5 专属技术报告前，训练流程、评测表和部署说明主要仍是模型发布方的 model-card claims。

## 7. 适配结论

### 7.1 是否值得作为 fast-router C3 候选

**值得，优先级高于当前 0.5B/1.5B POC 模型。**理由：

1. 2.516B 参数和 Q4 GGUF 体积适合本地探索。
2. Agent/tool-use/coding 后训练与任务类型分类场景有相关性。
3. 官方提供 GGUF 和 llama.cpp 使用路径，能复用 fast-router 现有 Zig 方向。
4. 标准 Llama 架构降低了模型代码适配风险。
5. 还提供 MLX、GPTQ、LiteRT 等变体，后续可作后端对照。

但当前只能给出“候选优先级”结论，不能给出“P1 达标”结论。

### 7.2 推荐角色

首选把 MiniCPM5-2B 用作：

* **C3 离线任务类型 scorer 候选**：输入当前 task-turn state，输出 A-J 分布。
* **prompt/template 兼容性试验模型**：检验原生 chat template 与 no-think 模式。
* **2B 级本地 fallback scorer**：当 8B/9B scorer 过慢或内存不足时作为低成本策略。

不建议第一步把它当作：

* 已经校准的概率模型。
* 生产级 tool-calling upstream 的默认后端。
* 不改代码即可替代当前 Qwen prompt 的 drop-in 模型。

## 8. 最小可逆验证计划

### 实验 0：获取与版本固定

记录：模型仓库 commit/revision、GGUF 文件名、量化级别、llama.cpp 版本、CPU/GPU、操作系统和运行命令。首选 `Q4_K_M`；对照 `Q8_0` 或 F16 只在资源允许时进行。

### 实验 1：裸 llama.cpp smoke test

```bash
llama-server \
  -m MiniCPM5-2B-Q4_K_M.gguf \
  -a MiniCPM5-2B \
  --port 8080 \
  -c 8192 \
  --jinja
```

用普通 chat、长文本和 tool-use 请求分别验证。生成测试按模型卡建议使用 `temperature=1.0, top_p=0.95, min_p=0.0`；不要把生成采样结果当作 scorer logits 结果。

### 实验 2：template/think probe

记录以下输入最终 token 序列和最后位置：

* `enable_thinking=false`。
* `enable_thinking=true`。
* 普通对话。
* 候选表 + `Which code?`。

验收标准：能明确回答“候选 A-J 的 logits 是在回答槽还是 thinking 槽提取”。

### 实验 3：tokenizer probe

对 `A`-`J`、带空格的 ` A`-` J`、候选自然语言 label 分别记录 token 数。验收标准：每个 fast-router 候选 code 都是恰好一个 token；否则切换候选池或改用 per-candidate yes/no。

### 实验 4：C3 对照 benchmark

在同一 30 样例集上比较：

| 方法 | 需验证的量 |
|---|---|
| 当前 hardcoded prompt + 单 token | P1、P2、熵、候选位置偏差 |
| MiniCPM5 原生 template + 单 token | template 修复收益 |
| no-think template + 单 token | thinking 影响 |
| 每候选 yes/no + prefix cache | 准确率收益与延迟代价 |

最少记录：准确率、每样本延迟、p50/p95、重复运行一致性、置信度与错误率关系、候选顺序置换敏感性。

### 实验 5：接入前后端测试

只有在实验 1-4 有证据后，才修改 `router/scorer.go`、`zig/frwrapper.zig` 和 `router/config.go`。接入应保持可逆：通过配置选择 MiniCPM5 模板/模型，不覆盖现有模型路径。

## 9. 风险、死法与可逆性

| 决策维度 | 最可能的死法 | 可逆措施 |
|---|---|---|
| 模型能力 | 通用 benchmark 强，但 C3 分类弱 | 先跑 30 样例，不把 benchmark 当验收 |
| Prompt | 取到 thinking 槽 logits，P1 假低 | 原生 template + no-think/think 对照 |
| Tokenizer | A-J 不是单 token，wrapper 返回 -6 | tokenizer probe；候选池可配置 |
| 性能 | 2B CPU prefill 仍超过 P2 预算 | 测 p50/p95；保留 hint-only 和远程 scorer fallback |
| Tool calling | XML 输出不能直接喂 OpenAI client | 使用官方 parser/backend，或在 Gateway 增加 XML 转换 |
| 长上下文 | 文档写 128K，wrapper 实际 4096 | 把 `n_ctx` 配置化并记录实际部署上限 |
| 溯源 | 论文编号和模型名不一致 | 以 revision 固定模型卡，单独标注未找到专属报告 |
| 分发 | GGUF 量化/llama.cpp 版本不匹配 | 先单平台 smoke test，再纳入 `scripts/pack.sh` |

## 10. 局限

1. 本研究没有在当前机器上下载权重并实际跑 MiniCPM5；网络环境无法稳定访问 Hugging Face 文件下载，故没有伪造 P1/P2 数字。
2. 没有本地安装 `transformers`/`tokenizers`，A-J 单 token 性质尚未从 tokenizer 实测确认。
3. 模型卡评测包含内部复现与 Artificial Analysis 数据，未逐项独立重跑。
4. 论文编号反查确认了引用链不等于 MiniCPM5 专属技术报告，但没有断言模型训练过程本身不真实；这里只区分证据等级。
5. GGUF 页面自动元数据“3B”与主卡精确参数冲突，尚未下载 GGUF metadata 做最终文件级裁决。

## 11. 本机实验前置检查（2026-09-26）

按仓库 [Build SOP](../SOP/build.md) 检查本机运行环境，并先执行不依赖模型权重的验证：

| 项目 | 结果 |
|---|---|
| Go | `go1.26.2 darwin/amd64` |
| Zig | `0.14.0` |
| `go test ./router/... ./router/schema/...` | 通过：router 与 schema 两个包 |
| `CGO_ENABLED=0 go build -o /tmp/fast-router-sop-check ./cmd/fast-router` | 通过 |
| SOP 默认 llama.cpp 库目录 `/tmp/llama-bins/llama-b11175` | 不存在 |
| `llama-server` / `llama-cli` / `llama-bench` | PATH 中未发现 |
| 本仓库内 GGUF 权重 | 未发现 |
| Python `transformers` / `tokenizers` / `huggingface_hub` | 未安装 |

因此本轮只完成 Go 本地构建验证；MiniCPM5 tokenizer/template probe、真实 scorer、P1/P2 benchmark 仍未运行。仓库已有 `yesnobench` 工作区修改，本轮未触碰。下一步须先按 SOP 获取并核验 llama.cpp 动态库，再获取固定 revision 的 MiniCPM5 GGUF 权重，并记录实际文件 SHA、量化、llama.cpp 版本、平台和 benchmark 命令。

## 12. 结论

MiniCPM5-2B 的工程价值不在于“又一个 2B 聊天模型”，而在于它同时满足四个 fast-router 研究条件：本地化、GGUF/llama.cpp 可用、agent/tool-use 导向、标准 Llama 架构。它非常适合成为 C3 scorer 的下一轮候选。

然而，真正的关键实验不是直接把模型名换成 `MiniCPM5-2B`，而是验证模型原生 chat template、thinking 开关、候选 tokenization 和单 token logits 位置。若继续使用当前硬编码 Qwen prompt，MiniCPM5 的架构优势可能被 prompt 协议错误抵消；若只看普通生成或 benchmark，也无法判断它是否适合作为 Jev 风格分类器。

**建议决策**：批准 MiniCPM5-2B 进入“候选模型 + template/scorer 对照实验”，暂不批准进入生产默认 scorer；先完成实验 0-4，保留现有 hint-only、mock Backend 和其他模型路径作为回退。

## 13. 参考资料

* [MiniCPM5-2B 中文模型卡](https://huggingface.co/openbmb/MiniCPM5-2B/blob/main/README-cn.md)
* [MiniCPM5-2B config.json](https://huggingface.co/openbmb/MiniCPM5-2B/blob/main/config.json)
* [MiniCPM5-2B chat_template.jinja](https://huggingface.co/openbmb/MiniCPM5-2B/blob/main/chat_template.jinja)
* [MiniCPM5-2B-GGUF](https://huggingface.co/openbmb/MiniCPM5-2B-GGUF)
* [MiniCPM4: Ultra-Efficient LLMs on End Devices, arXiv:2506.07900](https://arxiv.org/abs/2506.07900)
* [Data Science and Technology Towards AGI Part I: Tiered Data Management, arXiv:2602.09003](https://arxiv.org/abs/2602.09003)
* [本仓库 LLM2Jev 研究笔记](./LLM2Jev-研究笔记.md)
* [本仓库 Go 重写进展](./fast-router-Go重写进展-v0.1.md)
