#!/usr/bin/env python3
"""The arms routes against a reference (docs/research/ROUTES.md, "The Armourer").

  armsreport.py DIR REF ARM [ARM ...]

DIR holds `arms.py run` outputs (DIR/<arm>/s<seed>.summary.json). Per arm: buyouts (Wilson 95 %), median win day,
McNemar against REF on the paired seeds (exact two-sided binomial on the discordant pairs), the paired win-day
difference on seeds both won (mean ± 1.96 SE), income by source over the run (gun orders, arms counter sales, the
Board, counter and vending), the permit tier reached and the day, and Security at the end.
"""
import glob
import json
import math
import os
import statistics as st
import sys


def wilson(k, n, z=1.96):
    if n == 0:
        return 0, 0
    p = k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return max(0, c - h), min(1, c + h)


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    k = min(b, c)
    p = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return min(1.0, 2 * p)


def load(root, arm):
    out = {}
    for p in glob.glob(os.path.join(root, arm, "s*.summary.json")):
        s = json.load(open(p))
        seed = os.path.basename(p).split(".")[0]
        days = s.get("player_days") or []
        end = (s.get("game_over") or "alive")
        flow = {}
        for d in days:
            for k, v in (d.get("cash_flow") or {}).items():
                flow[k] = flow.get(k, 0) + v
        pl = s.get("player") or {}
        ar = pl.get("arms") or {}
        last = days[-1] if days else {}
        sec = (((last.get("reputation") or {}).get("security")) if isinstance(last.get("reputation"), dict) else None)
        out[seed] = {
            "won": str(end).startswith("buyout"), "day": len(days), "flow": flow,
            "board": (pl.get("appraisal") or {}).get("fees_total", 0) or 0, "arms": ar,
        }
    return out


def main():
    if len(sys.argv) < 4:
        sys.exit(__doc__)
    root, ref, arms = sys.argv[1], sys.argv[2], sys.argv[3:]
    base = load(root, ref)
    for a in [ref] + arms:
        r = load(root, a)
        n = len(r)
        k = sum(x["won"] for x in r.values())
        lo, hi = wilson(k, n)
        wd = sorted(x["day"] for x in r.values() if x["won"])
        line = f"{a:22s} {k}/{n} ({100 * lo:.0f}–{100 * hi:.0f} %) median win day {wd[len(wd) // 2] if wd else '-'}"
        if a != ref:
            seeds = sorted(set(r) & set(base))
            b = sum(1 for s in seeds if r[s]["won"] and not base[s]["won"])
            c = sum(1 for s in seeds if base[s]["won"] and not r[s]["won"])
            both = [r[s]["day"] - base[s]["day"] for s in seeds if r[s]["won"] and base[s]["won"]]
            if len(both) > 1:
                m, sd = st.mean(both), st.stdev(both)
                line += f" | McNemar {b}:{c} p={mcnemar(b, c):.2f} | Δ win day {m:+.1f} ± {1.96 * sd / math.sqrt(len(both)):.1f} (n={len(both)})"
        print(line)
        f = lambda key: st.mean(x["flow"].get(key, 0) for x in r.values())
        arm = [x["arms"] for x in r.values() if x["arms"]]
        inc = f"  per run: board {st.mean(x['board'] for x in r.values()):.0f}, counter {f('trade_sales'):.0f}, vending {f('vending_sales'):.0f}, wine {f('wine_sales'):.0f}"
        if arm:
            inc += (f", gun orders {st.mean(x.get('order_income', 0) for x in arm):.0f} ({st.mean(x.get('orders', 0) for x in arm):.0f} orders,"
                    f" given {st.mean(x.get('given_value', 0) for x in arm):.0f}), arms counter sales {st.mean(x.get('arms_sales', 0) for x in arm):.0f}")
            tiers = [x.get("tier", 0) for x in arm]
            p2 = sorted(x["permit2_day"] for x in arm if x.get("permit2_day"))
            p3 = sorted(x["permit3_day"] for x in arm if x.get("permit3_day"))
            inc += (f"\n  permit tier at the end: I {tiers.count(1)}, II {tiers.count(2)}, III {tiers.count(3)}, none {tiers.count(0)};"
                    f" Permit II in {len(p2)} (median day {p2[len(p2) // 2] if p2 else '-'}), III in {len(p3)} (median day {p3[len(p3) // 2] if p3 else '-'});"
                    f" Security at the end median {st.median(x.get('security', 0) for x in arm):.0f}")
            bt = [sum(x["by_tier"][i] for x in arm) / len(arm) for i in range(3)]
            it = [sum(x["income_by_tier"][i] for x in arm) / len(arm) for i in range(3)]
            inc += f"\n  orders by tier per run {bt[0]:.1f}/{bt[1]:.1f}/{bt[2]:.1f}, income {it[0]:.0f}/{it[1]:.0f}/{it[2]:.0f}"
        print(inc)


if __name__ == "__main__":
    main()
