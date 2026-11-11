"""Directive agreement on var/distill/val.jsonl: how closely a model's directives match the
strong player's on held-out days.

Backends:
  --endpoint URL --model NAME   an OpenAI-compatible server (llama-swap, the bridge's endpoints)
  --hf BASE [--adapter DIR]     local transformers generation (greedy)
  --gold                        score the targets against themselves (parser sanity: 1.0)

  uv run evaluate.py --endpoint http://127.0.0.1:9000/v1 --model qwen3-8b-fence-r1 --out ~/models/distill/qwen3-8b-r1/eval.json
"""

import argparse
import json
import re
import sys
import time
import urllib.request
from collections import defaultdict
from pathlib import Path

from common import prompt_text, read_rows, write_json

ENUM_KEYS = {"bargain", "permit", "augs", "engine", "mortgage", "scavenge"}
LIST_KEYS = {"services", "upgrades_buy", "growth", "buy.prefer_types", "buy.avoid_types", "sell.hold_uids", "window.show_uids"}
SUB_KEYS = {"buy", "sell", "window"}
# Kinds a sample fills at random; the other kinds (decision, rollout, change, switch) come
# first, up to two thirds of the sample.
ROUTINE_KINDS = {"routine", "empty", "keep", "repeat"}


def parse_answer(text):
    """(why, directives or None). The JSON object is the first balanced {...} after WHY."""
    text = re.sub(r"<think>.*?</think>", "", text, flags=re.S).strip()
    why = ""
    m = re.match(r"\s*WHY:\s*(.*)", text)
    if m:
        why = m.group(1).split("\n", 1)[0]
    start = text.find("{")
    while start >= 0:
        depth = 0
        for i in range(start, len(text)):
            if text[i] == "{":
                depth += 1
            elif text[i] == "}":
                depth -= 1
                if depth == 0:
                    try:
                        obj = json.loads(text[start : i + 1])
                        if isinstance(obj, dict):
                            return why, obj
                    except json.JSONDecodeError:
                        pass
                    break
        start = text.find("{", start + 1)
    return why, None


def flatten(d):
    out = {}
    for k, v in d.items():
        if k in ("day", "why"):
            continue
        if k in SUB_KEYS and isinstance(v, dict):
            for kk, vv in v.items():
                out[f"{k}.{kk}"] = vv
        else:
            out[k] = v
    return out


def norm_id(s):
    s = str(s).upper()
    return s[: -len("_SERVICE")] if s.endswith("_SERVICE") else s


def agree(key, gold, pred):
    """1 for agreement, 0 for none, fractions for lists and near numbers."""
    if pred is None:
        return 0.0
    if key in LIST_KEYS or isinstance(gold, list):
        g = {norm_id(x) for x in gold or []}
        p = {norm_id(x) for x in pred} if isinstance(pred, list) else set()
        return 1.0 if not g and not p else len(g & p) / len(g | p)
    if isinstance(gold, bool) or key in ENUM_KEYS:
        return float(str(gold).lower() == str(pred).lower())
    if isinstance(gold, (int, float)) and not isinstance(gold, bool):
        try:
            p = float(pred)
        except (TypeError, ValueError):
            return 0.0
        tol = max(0.15 * abs(gold), 25)
        return float(abs(p - gold) <= tol)
    return float(gold == pred)


def score(gold, pred, delta=False):
    """Per-key agreement over the gold keys, plus keys the model set that gold left out.
    A delta (round 2) is scored over both answers' keys: a change gold did not make is a
    miss, and {} against {} is full agreement."""
    g = flatten(gold or {})
    p = flatten(pred) if pred else {}
    keys = {k: agree(k, v, p.get(k)) for k, v in g.items()}
    extra = sorted(set(p) - set(g))
    if delta:
        if pred is None:
            keys = keys or {"{}": 0.0}
        else:
            keys.update({k: 0.0 for k in extra})
            if not keys:
                keys = {"{}": 1.0}
    return keys, extra


def generate_endpoint(url, model, messages, max_tokens, timeout):
    body = json.dumps({
        "model": model, "messages": messages, "max_tokens": max_tokens, "temperature": 0,
        "chat_template_kwargs": {"enable_thinking": False},
    }).encode()
    req = urllib.request.Request(url.rstrip("/") + "/chat/completions", body, {"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.load(r)["choices"][0]["message"]["content"] or ""


class HFGen:
    def __init__(self, base, adapter, qlora):
        import torch
        from transformers import AutoModelForCausalLM, AutoTokenizer, BitsAndBytesConfig

        self.torch = torch
        self.tok = AutoTokenizer.from_pretrained(adapter or base)
        kw = dict(dtype=torch.bfloat16, device_map={"": 0} if torch.cuda.is_available() else "cpu")
        if qlora:
            kw["quantization_config"] = BitsAndBytesConfig(load_in_4bit=True, bnb_4bit_quant_type="nf4",
                                                           bnb_4bit_compute_dtype=torch.bfloat16)
        self.model = AutoModelForCausalLM.from_pretrained(base, **kw)
        if adapter:
            from peft import PeftModel

            self.model = PeftModel.from_pretrained(self.model, adapter)
        self.model.eval()

    def __call__(self, messages, max_tokens):
        ids = self.tok(prompt_text(self.tok, messages), return_tensors="pt", add_special_tokens=False).to(self.model.device)
        with self.torch.no_grad():
            out = self.model.generate(**ids, max_new_tokens=max_tokens, do_sample=False)
        return self.tok.decode(out[0, ids["input_ids"].shape[1]:], skip_special_tokens=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--data", default="../../var/distill/val.jsonl")
    ap.add_argument("--endpoint")
    ap.add_argument("--model", default="")
    ap.add_argument("--hf")
    ap.add_argument("--adapter")
    ap.add_argument("--qlora", action="store_true")
    ap.add_argument("--gold", action="store_true")
    ap.add_argument("--persist", action="store_true", help="baseline: predict the previous day's directives (from train+val)")
    ap.add_argument("--empty", action="store_true", help="baseline: answer {} (the delta planner's executor)")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--sample", type=int, default=0, help="score the decision-like rows (at most 2/3 of N) plus a seeded random fill of routine rows, N in all")
    ap.add_argument("--max-tokens", type=int, default=700)
    ap.add_argument("--timeout", type=float, default=300)
    ap.add_argument("--out", required=True, help="eval JSON (the per-row answers go next to it as .rows.jsonl)")
    a = ap.parse_args()

    rows = read_rows(a.data)
    prev = {}
    for f in ("train.jsonl", "val.jsonl"):
        p = Path(a.data).with_name(f)
        if p.exists():
            for r in read_rows(p):
                prev[(r["meta"]["game"], r["meta"]["day"])] = r["messages"][-1]["content"]
    if a.sample and a.sample < len(rows):
        import random

        dec = [r for r in rows if r["meta"]["kind"] not in ROUTINE_KINDS]
        rest = [r for r in rows if r["meta"]["kind"] in ROUTINE_KINDS]
        rng = random.Random(1)
        if len(dec) > a.sample * 2 // 3:
            dec = rng.sample(dec, a.sample * 2 // 3)
        rows = dec + rng.sample(rest, min(len(rest), a.sample - len(dec)))
    if a.limit:
        rows = rows[: a.limit]
    if a.gold or a.persist or a.empty:
        gen = lambda msgs, n: None  # noqa: E731
    elif a.endpoint:
        gen = lambda msgs, n: generate_endpoint(a.endpoint, a.model, msgs, n, a.timeout)  # noqa: E731
    elif a.hf:
        gen = HFGen(a.hf, a.adapter, a.qlora)
    else:
        sys.exit("one of --endpoint, --hf, --gold or --persist")

    per_key = defaultdict(list)
    changed = []
    by_kind = defaultdict(list)
    by_source = defaultdict(list)
    parsed, has_why, extras, out_rows = 0, 0, defaultdict(int), []
    t0 = time.time()
    for i, r in enumerate(rows):
        target = r["messages"][-1]["content"]
        _, gold = parse_answer(target)
        if a.gold:
            text = target
        elif a.persist:
            text = prev.get((r["meta"]["game"], r["meta"]["day"] - 1), "")
        elif a.empty:
            text = "{}"
        else:
            text = gen(r["messages"][:-1], a.max_tokens)
        why, pred = parse_answer(text)
        delta = r["meta"].get("format") == "delta"
        if delta and pred is not None:
            why = "{}" if not flatten(pred) else str(pred.get("why") or "")
        parsed += pred is not None
        has_why += bool(why)
        keys, extra = score(gold, pred, delta)
        before = prev.get((r["meta"]["game"], r["meta"]["day"] - 1))
        if before is not None:
            old = flatten(parse_answer(before)[1] or {})
            g = flatten(gold)
            changed += [v for k, v in keys.items() if k in g and agree(k, g[k], old.get(k)) < 1]
        for k, v in keys.items():
            per_key[k].append(v)
        for k in extra:
            extras[k] += 1
        mean = sum(keys.values()) / max(len(keys), 1)
        by_kind[r["meta"]["kind"]].append(mean)
        by_source[r["meta"]["source"]].append(mean)
        out_rows.append({"game": r["meta"]["game"], "day": r["meta"]["day"], "kind": r["meta"]["kind"],
                         "agreement": round(mean, 3), "keys": keys, "answer": text})
        print(f"{i + 1}/{len(rows)} {r['meta']['game']} d{r['meta']['day']} {r['meta']['kind']}: {mean:.2f}", flush=True)

    allv = [m for v in by_kind.values() for m in v]
    res = {
        "model": a.model or a.adapter or a.hf or ("persist" if a.persist else "empty" if a.empty else "gold"),
        "rows": len(rows),
        "json_ok": round(parsed / max(len(rows), 1), 3),
        "why_ok": round(has_why / max(len(rows), 1), 3),
        "agreement": round(sum(allv) / max(len(allv), 1), 3),
        # Agreement on the keys where yesterday's directives would not have agreed with today's:
        # copying yesterday scores ~0 here, so this is the number that shows judgment.
        "changed_agreement": round(sum(changed) / max(len(changed), 1), 3),
        "changed_keys": len(changed),
        "by_kind": {k: round(sum(v) / len(v), 3) for k, v in sorted(by_kind.items())},
        "by_source": {k: round(sum(v) / len(v), 3) for k, v in sorted(by_source.items())},
        "per_key": {k: {"agree": round(sum(v) / len(v), 3), "n": len(v)} for k, v in sorted(per_key.items())},
        "extra_keys": dict(sorted(extras.items())),
        "seconds": round(time.time() - t0),
    }
    write_json(a.out, res)
    with open(Path(a.out).with_suffix(".rows.jsonl"), "w") as f:
        for o in out_rows:
            f.write(json.dumps(o) + "\n")
    print(json.dumps({k: res[k] for k in ("json_ok", "why_ok", "agreement", "changed_agreement", "changed_keys", "by_kind", "by_source")}))


if __name__ == "__main__":
    main()
