#!/usr/bin/env python3
"""Fair play's cost (docs/research/FAIR-PLAY.md): the same routes under two builds, paired by seed.

  fairplay_report.py REF_DIR ARM_DIR [ROUTE ...]

Each DIR holds `arms.py run` outputs (DIR/<route>/s<seed>.summary.json). Per route: wins with Wilson 95 % CIs,
median win day, McNemar (exact two-sided) on the paired seeds, the paired win-day difference on seeds both won
(mean ± 1.96 SE), and buyer exposures a run (player.fair_play.buyer_exposures, when the build counts them).
"""
import glob
import json
import math
import os
import statistics as st
import sys

sys.path.insert(0, os.path.dirname(__file__))
from armsreport import mcnemar, wilson  # noqa: E402


def load(root, route):
    out = {}
    for p in glob.glob(os.path.join(root, route, "s*.summary.json")):
        with open(p) as f:
            d = json.load(f)
        out[d["seed"]] = d
    return out


def won(d):
    return (d.get("game_over") or "").startswith("buyout") and not d.get("crash")


def exposures(runs):
    xs = [(d.get("player") or {}).get("fair_play", {}).get("buyer_exposures") for d in runs.values()]
    xs = [x for x in xs if x is not None]
    return f"{st.mean(xs):.1f}" if xs else "–"


def main(argv):
    ref_dir, arm_dir, routes = argv[0], argv[1], argv[2:]
    routes = routes or sorted(os.listdir(arm_dir))
    print("| route | wins ref (95 % CI) | wins arm (95 % CI) | median win day ref → arm | McNemar b/c, p | paired Δ win day | exposures a run ref → arm |")
    print("|---|---|---|---|---|---|---|")
    for r in routes:
        a, b = load(ref_dir, r), load(arm_dir, r)
        seeds = sorted(set(a) & set(b))
        if not seeds:
            continue
        wa = sum(won(a[s]) for s in seeds)
        wb = sum(won(b[s]) for s in seeds)
        n = len(seeds)
        la, ha = wilson(wa, n)
        lb, hb = wilson(wb, n)
        only_ref = sum(won(a[s]) and not won(b[s]) for s in seeds)
        only_arm = sum(won(b[s]) and not won(a[s]) for s in seeds)
        p = mcnemar(only_ref, only_arm)
        da = [a[s]["days_played"] for s in seeds if won(a[s])]
        db = [b[s]["days_played"] for s in seeds if won(b[s])]
        both = [b[s]["days_played"] - a[s]["days_played"] for s in seeds if won(a[s]) and won(b[s])]
        if len(both) > 1:
            m = st.mean(both)
            se = st.stdev(both) / math.sqrt(len(both))
            delta = f"{m:+.1f} ± {1.96 * se:.1f} (n {len(both)})"
        else:
            delta = "–"
        med = f"{st.median(da) if da else '–'} → {st.median(db) if db else '–'}"
        print(f"| `{r}` | {wa}/{n} ({100*la:.0f}–{100*ha:.0f} %) | {wb}/{n} ({100*lb:.0f}–{100*hb:.0f} %) | {med} | {only_ref}/{only_arm}, {p:.2f} | {delta} | {exposures(a)} → {exposures(b)} |")


if __name__ == "__main__":
    main(sys.argv[1:])
