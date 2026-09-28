#!/usr/bin/env python3
"""Convert runtime train_log.jsonl into SFT retraining data (design ②).

Teacher labels = slow-tier records (9B in background). Mixed with replay of
the original synthetic set to counter catastrophic forgetting. High/low
confidence filtering and disagreement mining optional via flags.
"""
import json, random, sys, argparse

random.seed(42)

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--train_log", default="data/train_log.jsonl")
    ap.add_argument("--replay", default="/tmp/fr-distill/sft_router.jsonl",
                    help="original synthetic SFT set for anti-forgetting replay")
    ap.add_argument("--out", default="/tmp/fr-distill/sft_retrain.jsonl")
    ap.add_argument("--min_conf", type=float, default=0.5,
                    help="teacher samples below this confidence are skipped (noisy labels)")
    ap.add_argument("--replay_ratio", type=float, default=1.0,
                    help="replay conversations per teacher conversation")
    a = ap.parse_args()

    desc = json.load(open("/tmp/task_desc.json"))
    codes = sorted(desc.keys())

    teachers = []
    seen_states = set()
    try:
        for line in open(a.train_log):
            r = json.loads(line)
            if r.get("tier") != "slow":
                continue
            if r.get("confidence", 0) < a.min_conf:
                continue
            st = r.get("state", "").strip()
            if len(st) < 6 or st in seen_states:
                continue
            seen_states.add(st)
            teachers.append((st, r["task_code"]))
    except FileNotFoundError:
        print(f"no train log at {a.train_log}", file=sys.stderr)
        sys.exit(1)

    convs = []
    for st, code in teachers:
        if code not in desc:
            continue
        convs.append({"conversations": [
            {"role": "user", "content": f"{st}\nQuestion: Is this about \"{code}\" ({desc[code]})? Answer Yes or No."},
            {"role": "assistant", "content": "Yes"}]})
        for nc in random.sample([c for c in codes if c != code], 3):
            convs.append({"conversations": [
                {"role": "user", "content": f"{st}\nQuestion: Is this about \"{nc}\" ({desc[nc]})? Answer Yes or No."},
                {"role": "assistant", "content": "No"}]})

    replay = []
    try:
        for line in open(a.replay):
            replay.append(json.loads(line))
    except FileNotFoundError:
        pass
    n_replay = min(len(replay), int(len(convs) * a.replay_ratio))
    replay = random.sample(replay, n_replay) if n_replay < len(replay) else replay

    out = teachers_conv + replay if (teachers_conv := convs) else replay
    random.shuffle(out)
    with open(a.out, "w") as f:
        for c in out:
            f.write(json.dumps(c, ensure_ascii=False) + "\n")
    print(f"{len(teachers)} teacher states -> {len(convs)} convs + {len(replay)} replay = {len(out)} -> {a.out}")

if __name__ == "__main__":
    main()
