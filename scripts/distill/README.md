# L4 蒸馏工具链（minimind 路线）

- `gen_data.py`：合成 fast-router 任务路由 SFT 数据。训练 prompt 与 scorer
  打分 prompt 字节级一致（`{state}\nQuestion: Is this about "{code}" ({desc})? Answer Yes or No.`），
  正负样本 1:3，30 测试样本 held-out。需要 /tmp/task_desc.json（从 task_types.go 抽取）。
- `eval_router.sh`：训练产物 pth → Qwen3-HF → GGUF → yesnobench P1 一键评测。

管线前置：minimind repo clone（/tmp/minimind）、llama.cpp clone + qwen2 vocab 回退补丁、
torch/transformers/sentencepiece/gguf pip 依赖。权重经 modelscope（HF 被网络封锁）。

实测记录见 docs/minimind-研究笔记.md。
