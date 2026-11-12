#!/usr/bin/env python3
"""Deadline-sprint report (docs/research/SEARCH-LABELS.md).

  sprint_report.py compare -deadline 91 BASE_DIR ARM_DIR [ARM_DIR...]   paired wins, McNemar, win day
  sprint_report.py labels ROWS_DIR [ROWS_DIR...]                        label statistics per fork

A summary is fence-gym's <seed>.json or fence-rollout's s<seed>.summary.json. A win is a buyout on or before
the deadline. The paired win day counts a loss as the deadline + 7 (the next rent morning), so it is defined on
every seed; Δ is arm - base, mean ± 1.96 SE.
"""
import glob
import json
import math
import os
import re
import sys
from collections import Counter, defaultdict


def wilson(k, n, z=1.96):
    if n == 0:
        return 0, 0
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return c - h, c + h


def mcnemar_exact(b, c):
    """Two-sided exact binomial p for b vs c discordant pairs."""
    n = b + c
    if n == 0:
        return 1.0
    k = min(b, c)
    p = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return min(1.0, 2 * p)


def load(d, deadline):
    out = {}
    for f in glob.glob(os.path.join(d, "*.json")):
        m = re.match(r"s?(\d+)(\.summary)?\.json$", os.path.basename(f))
        if not m:
            continue
        j = json.load(open(f))
        over = j.get("game_over") or ""
        day = j.get("last_day")
        won = over.startswith("buyout") and day <= deadline
        out[int(m.group(1))] = (won, day if won else deadline + 7, j)
    return out


def compare(argv):
    deadline = 91
    if argv[0] == "-deadline":
        deadline, argv = int(argv[1]), argv[2:]
    base = load(argv[0], deadline)
    rows = []
    for d in argv:
        arm = load(d, deadline)
        seeds = sorted(set(base) & set(arm))
        n = len(seeds)
        k = sum(arm[s][0] for s in seeds)
        lo, hi = wilson(k, n)
        b = sum(1 for s in seeds if arm[s][0] and not base[s][0])
        c = sum(1 for s in seeds if base[s][0] and not arm[s][0])
        dd = [arm[s][1] - base[s][1] for s in seeds]
        mean = sum(dd) / n if n else 0
        se = math.sqrt(sum((x - mean) ** 2 for x in dd) / (n - 1) / n) if n > 1 else 0
        wd = sorted(arm[s][1] for s in seeds if arm[s][0])
        med = wd[len(wd) // 2] if wd else None
        rows.append((d, k, n, lo, hi, med, b, c, mcnemar_exact(b, c), mean, se))
    print(f"{'arm':60s} wins   95% CI   med  +won -lost  McNemar p  Δ day (loss = {deadline + 7})")
    for d, k, n, lo, hi, med, b, c, p, mean, se in rows:
        print(f"{d[-60:]:60s} {k:2d}/{n} {lo * 100:3.0f}-{hi * 100:3.0f}%  {med}  {b:4d} {c:4d}   {p:7.3f}   {mean:+.1f} ± {1.96 * se:.1f}")


def labels(argv):
    rows = []
    for d in argv:
        for f in sorted(glob.glob(os.path.join(d, "*.jsonl"))):
            rows += [json.loads(l) for l in open(f) if l.strip()]
    by = defaultdict(list)
    for r in rows:
        by[(r["day"], r["fork"])].append(r)
    print(f"{len(rows)} rows from {len({r['seed'] for r in rows})} games")
    print(f"{'day':>4} {'fork':9s} rows screened  p_win(keep)  switch  labels; best ungated (z<gate)")
    for (day, fork), rs in sorted(by.items()):
        scr = sum(r["search"].get("screened") for r in rs)
        pk = sum(r["candidates"][0]["p_win"] for r in rs) / len(rs)
        lab = Counter(r["label"] for r in rs if r["label"] != "default")
        arg = Counter(r["argmax"] for r in rs if r["argmax"] != "keep" and r["label"] == "default")
        print(f"{day:4d} {fork:9s} {len(rs):4d} {scr:8d}  {pk:10.2f}  {sum(lab.values()):6d}  {dict(lab)}; {dict(arg)}")
    gains = Counter()
    for r in rows:
        for c in r["candidates"][1:]:
            gains[c["name"], "gained"] += c["wins_gained"]
            gains[c["name"], "lost"] += c["wins_lost"]
    print("samples won / lost against keep (all rows, paired):")
    for name in sorted({k[0] for k in gains}):
        print(f"  {name:22s} +{gains[name, 'gained']:4d} -{gains[name, 'lost']:4d}")


if __name__ == "__main__":
    if len(sys.argv) < 3 or sys.argv[1] not in ("compare", "labels"):
        sys.exit(__doc__)
    {"compare": compare, "labels": labels}[sys.argv[1]](sys.argv[2:])
