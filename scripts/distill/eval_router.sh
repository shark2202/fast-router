#!/bin/bash
# trained pth -> Qwen3-HF -> GGUF -> yesnobench P1
set -e
cd /tmp/minimind
python3 - <<'PYEOF'
import torch, warnings
from transformers import Qwen3Config, Qwen3ForCausalLM, AutoTokenizer
from model.model_minimind import MiniMindConfig
warnings.filterwarnings('ignore')
lm_config = MiniMindConfig(hidden_size=768, num_hidden_layers=8, max_seq_len=8192, use_moe=False)
sd = torch.load("/tmp/minimind-out/router_sft_768.pth", map_location="cpu")
cfg = Qwen3Config(
    vocab_size=lm_config.vocab_size, hidden_size=lm_config.hidden_size,
    intermediate_size=lm_config.intermediate_size, num_hidden_layers=lm_config.num_hidden_layers,
    num_attention_heads=lm_config.num_attention_heads, num_key_value_heads=lm_config.num_key_value_heads,
    head_dim=lm_config.hidden_size // lm_config.num_attention_heads,
    max_position_embeddings=lm_config.max_position_embeddings,
    rms_norm_eps=lm_config.rms_norm_eps, rope_theta=lm_config.rope_theta,
    tie_word_embeddings=lm_config.tie_word_embeddings,
    use_sliding_window=False, sliding_window=None)
m = Qwen3ForCausalLM(cfg); m.load_state_dict(sd, strict=True); m = m.to(torch.float16)
m.save_pretrained("/tmp/router-sft-hf")
AutoTokenizer.from_pretrained("model/").save_pretrained("/tmp/router-sft-hf")
print("exported")
PYEOF
cd /tmp/llama.cpp && python3 convert_hf_to_gguf.py /tmp/router-sft-hf --outfile /tmp/router-sft.gguf --outtype f16 2>&1 | tail -1
cd /Users/mac/codes/fast-router && DYLD_LIBRARY_PATH=/tmp/llama-verify/llama-b11175 go run ./cmd/yesnobench /tmp/router-sft.gguf 2>&1 | grep -E "YES/NO|affirmation|score_note"
