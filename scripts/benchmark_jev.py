#!/usr/bin/env python3
"""POC1 benchmark: Jev task-type classification accuracy (P1 > 70%).

Loads the local Jev scorer (FBU_MODEL env or --model arg), runs the 30 sample
requests in data/task_type_samples.json, and reports per-class + overall accuracy.

Requires: mlx-lm + a local Qwen3.5-style model directory (because huggingface
download is unreachable in this dev env — pass a local path).

Usage:
  FBU_MODEL=~/models/Qwen3.5-9B-4bit python scripts/benchmark_jev.py
  python scripts/benchmark_jev.py --model /path/to/model
"""

import argparse
import json
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT / "src"))

from fast_router.jev_scorer import JevTaskTypeScorer  # noqa: E402
from fast_router.task_types import BY_CODE  # noqa: E402

SAMPLES_PATH = ROOT / "data" / "task_type_samples.json"
P1_TARGET = 0.70


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--model", default=os.environ.get("FBU_MODEL"),
                    help="local dir OR model id to download (default: mlx-community/Qwen3.5-9B-4bit)")
    ap.add_argument("--source", default=os.environ.get("FBU_SOURCE", "modelscope"),
                    choices=["modelscope", "huggingface"],
                    help="download source when model is not a local dir (default: modelscope)")
    ap.add_argument("--backend", default=os.environ.get("FBU_BACKEND", "auto"),
                    choices=["auto", "mlx", "torch"],
                    help="inference backend (default: auto = mlx on arm64-mac, else torch)")
    ap.add_argument("--samples", default=str(SAMPLES_PATH))
    args = ap.parse_args()

    print(f"loading scorer: model={args.model or '(default)'}, source={args.source}, backend={args.backend}", file=sys.stderr)
    scorer = JevTaskTypeScorer(model_path=args.model, source=args.source, backend=args.backend)
    print(f"  loaded in {scorer.load_ms} ms; labels={scorer.labels}", file=sys.stderr)

    samples = json.loads(Path(args.samples).read_text())
    print(f"\nrunning {len(samples)} samples...\n")

    correct = 0
    total = 0
    mistakes = []
    per_class = {}
    latencies = []

    for s in samples:
        code, scores, tele = scorer.classify(s["message"])
        total += 1
        exp = s["expected"]
        per_class.setdefault(exp, {"correct": 0, "total": 0})
        per_class[exp]["total"] += 1
        latencies.append(tele["latency_ms"])
        if code == exp:
            correct += 1
            per_class[exp]["correct"] += 1
        else:
            mistakes.append((s, code, scores))

    accuracy = correct / total if total else 0.0
    avg_lat = sum(latencies) / len(latencies) if latencies else 0.0
    p2_pass = avg_lat < 1000.0

    print("=" * 60)
    print(f"P1 accuracy: {correct}/{total} = {accuracy:.1%}  (target > {P1_TARGET:.0%})  "
          f"{'PASS' if accuracy >= P1_TARGET else 'FAIL'}")
    print(f"P2 latency:  avg {avg_lat:.0f} ms  (target < 1000 ms)  "
          f"{'PASS' if p2_pass else 'FAIL'}  cache_hit on 2nd+: yes")
    print()
    print("per-class:")
    for code in sorted(per_class):
        tt = BY_CODE[code]
        pc = per_class[code]
        acc = pc["correct"] / pc["total"] if pc["total"] else 0
        print(f"  {code} {tt.name:28} {pc['correct']}/{pc['total']} = {acc:.0%}")
    print()
    if mistakes:
        print(f"misclassifications ({len(mistakes)}):")
        for s, got, scores in mistakes:
            exp_tt = BY_CODE[s["expected"]]
            got_tt = BY_CODE.get(got)
            got_name = got_tt.name if got_tt else got
            print(f"  expected={s['expected']}({exp_tt.name}) got={got}({got_name})  "
                  f"msg={s['message'][:50]!r}")
    print()
    print(f"score_note: candidate-normalized preferences, NOT calibrated probabilities.")
    return 0 if (accuracy >= P1_TARGET and p2_pass) else 1


if __name__ == "__main__":
    raise SystemExit(main())
