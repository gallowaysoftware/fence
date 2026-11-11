#!/usr/bin/env python3
"""Win table for a study: every policy with >= MIN seeds on a track, paired against a reference policy.

  board.py STUDY [-track normal] [-min 16] [-ref "-human player"] [-top 12]

Per policy, on the training seeds it shares with the reference: wins (buyout*), median win day,
endings, mean objective and the paired difference against the reference (± 95 % CI).
"""
import collections
import json
import math
import os
import sys


def main(argv):
    study, o = argv[0], {"-track": "normal", "-min": "16", "-ref": "-human player", "-top": "12"}
    it = iter(argv[1:])
    for a in it:
        o[a] = next(it)
    by = collections.defaultdict(dict)
    with open(os.path.join(study, "evals.jsonl")) as f:
        for line in f:
            e = json.loads(line)
            if e["diff"] == o["-track"]:
                by[e["key"]][e["seed"]] = e
    ref = by.get(o["-ref"], {})
    rows = []
    for key, es in by.items():
        if len(es) < int(o["-min"]):
            continue
        seeds = sorted(s for s in es if s in ref and s < 100000)
        if not seeds:
            continue
        wins = sorted(es[s]["last_day"] for s in seeds if es[s]["win"])
        rwins = sum(ref[s]["win"] for s in seeds)
        d = [es[s]["obj"] - ref[s]["obj"] for s in seeds]
        m = sum(d) / len(d)
        h = 1.96 * math.sqrt(sum((x - m) ** 2 for x in d) / (len(d) - 1) / len(d)) if len(d) > 1 else float("nan")
        ends = collections.Counter(es[s]["ending"] for s in seeds).most_common(4)
        rows.append((len(wins) / len(seeds), -(wins[len(wins) // 2] if wins else 9999), key, len(seeds), len(wins), rwins,
                     wins[len(wins) // 2] if wins else None, ends, sum(es[s]["obj"] for s in seeds) / len(seeds), m, h))
    rows.sort(reverse=True)
    print(f"{'wins':>7} {'ref':>4} {'n':>3} {'win day':>7} {'obj':>6} {'Δobj vs ref':>14}  endings | policy")
    for r in rows[: int(o["-top"])]:
        _, _, key, n, w, rw, wd, ends, ob, m, h = r
        print(f"{w:3d}/{n:<3d} {rw:4d} {n:3d} {str(wd or '-'):>7} {ob:6.3f} {m:+7.3f}±{h:5.3f}  "
              + ", ".join(f"{e} {c}" for e, c in ends) + " | " + key.replace("-human player ", "").replace("-directives-json ", ""))


if __name__ == "__main__":
    main(sys.argv[1:])
