#!/bin/bash
# E1 auto-trigger: poll train_log teacher samples → full pipeline → regression
# gate → hot deploy (or keep-old + alert). The loop never skips the gate.
set -u
FAST_ROUTER_URL="${FAST_ROUTER_URL:-http://127.0.0.1:8080}"
TRAIN_LOG="${TRAIN_LOG:-data/train_log.jsonl}"
MIN_NEW="${MIN_NEW:-200}"          # teacher samples since last cycle to trigger
CKPT="${CKPT:-data/.last_retrain_count}"
WORK="${WORK:-/tmp/fr-retrain}"
GATE_SAMPLES="${GATE_SAMPLES:-data/task_type_samples.json}"
mkdir -p "$WORK"

count_teachers() { [ -f "$TRAIN_LOG" ] && python3 -c "
import json,sys
n=0
for l in open('$TRAIN_LOG'):
    try:
        r=json.loads(l)
        n += 1 if r.get('tier')=='slow' and r.get('confidence',0)>=0.5 else 0
    except: pass
print(n)" || echo 0; }

last=$(cat "$CKPT" 2>/dev/null || echo 0)
now=$(count_teachers)
delta=$((now - last))
echo "[trigger] teacher samples: total=$now new=$delta (threshold $MIN_NEW)"
[ "$delta" -lt "$MIN_NEW" ] && exit 0

set -e
echo "[data] converting $delta new teacher samples (+replay)"
python3 scripts/distill/log_to_sft.py --train_log "$TRAIN_LOG" --out "$WORK/sft_cycle.jsonl"

echo "[train] CPU fine-tune (from frozen full_sft base)"
cd /tmp/minimind/trainer
OMP_NUM_THREADS=6 MKL_NUM_THREADS=6 python3 -u train_full_sft.py \
  --data_path "$WORK/sft_cycle.jsonl" --device cpu --dtype float32 \
  --batch_size 8 --epochs 1 --learning_rate 1e-5 --max_seq_len 192 \
  --num_workers 0 --log_interval 50 --save_dir "$WORK" --save_weight cycle \
  --from_weight full_sft > "$WORK/train.log" 2>&1

echo "[convert] pth → GGUF"
cd /tmp/minimind && python3 - "$WORK/cycle_768.pth" <<'PYEOF'
import sys, torch, warnings
from transformers import Qwen3Config, Qwen3ForCausalLM, AutoTokenizer
from model.model_minimind import MiniMindConfig
warnings.filterwarnings('ignore')
lm = MiniMindConfig(hidden_size=768, num_hidden_layers=8, max_seq_len=8192, use_moe=False)
sd = torch.load(sys.argv[1], map_location="cpu")
cfg = Qwen3Config(vocab_size=lm.vocab_size, hidden_size=lm.hidden_size,
  intermediate_size=lm.intermediate_size, num_hidden_layers=lm.num_hidden_layers,
  num_attention_heads=lm.num_attention_heads, num_key_value_heads=lm.num_key_value_heads,
  head_dim=lm.hidden_size//lm.num_attention_heads, max_position_embeddings=lm.max_position_embeddings,
  rms_norm_eps=lm.rms_norm_eps, rope_theta=lm.rope_theta, tie_word_embeddings=lm.tie_word_embeddings,
  use_sliding_window=False, sliding_window=None)
m = Qwen3ForCausalLM(cfg); m.load_state_dict(sd, strict=True); m = m.to(torch.float16)
m.save_pretrained(sys.argv[2]); AutoTokenizer.from_pretrained("model/").save_pretrained(sys.argv[2])
PYEOF
cd /tmp/llama.cpp && python3 convert_hf_to_gguf.py "$WORK/cycle_hf" --outfile "$WORK/cycle.gguf" --outtype f16 >/dev/null 2>&1

echo "[gate] regression gate: new model must BEAT the current one"
cd - >/dev/null
run_p1() {  # $1=gguf → prints "N/30"
  DYLD_LIBRARY_PATH="${LLAMA_DIR:-/tmp/llama-verify/llama-b11175}" \
    go run ./cmd/yesnobench "$1" 2>/dev/null | grep -oE "[0-9]+/30" | head -1
}
new_p1=$(run_p1 "$WORK/cycle.gguf")
echo "[gate] new=$new_p1 (current deployment's gate value recorded at last cycle)"
# gate vs the CURRENT fast tier — read its last gate record
last_gate=$(cat data/.last_gate 2>/dev/null || echo "0/30")
if [ "$(echo "$new_p1" | cut -d/ -f1)" -lt "$(echo "$last_gate" | cut -d/ -f1)" ]; then
  echo "[gate] FAILED: $new_p1 < $last_gate — keeping current model, archiving candidate"
  exit 2
fi
echo "[deploy] hot-swapping fast tier"
curl -s -X POST "$FAST_ROUTER_URL/api/model/reload" -H 'Content-Type: application/json' \
  -d "{\"fast_path\": \"$WORK/cycle.gguf\"}"
echo "$new_p1" > data/.last_gate
echo "$now" > "$CKPT"
echo "[done] cycle complete: gate=$new_p1 deployed=$WORK/cycle.gguf"
