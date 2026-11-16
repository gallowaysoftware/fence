#!/usr/bin/env python3
"""The cheap hidden-system levers against their route (docs/research/HIDDEN-SYSTEMS.md).

  cheapreport.py DIR REF ARM [ARM ...]

DIR holds `arms.py run` outputs (DIR/<arm>/s<seed>.summary.json). Per arm: buyouts (Wilson 95 %), median win day,
McNemar against REF on the paired seeds (exact two-sided binomial on the discordant pairs), the paired win-day
difference on seeds both won (mean ± 1.96 SE), cash buyouts (McNemar on them when they differ), and per run: wanted reports and bounties, Board fees, rents skipped,
insurance paid, liquid luck drunk, and Security / Lower Levels rep at the end.
"""
import glob
import json
import math
import os
import statistics as st
import sys

sys.path.insert(0, os.path.dirname(__file__))
from armsreport import mcnemar, wilson  # noqa: E402


def load(root, arm):
    out = {}
    for p in glob.glob(os.path.join(root, arm, "s*.summary.json")):
        s = json.load(open(p))
        days = s.get("player_days") or []
        flow = {}
        for d in days:
            for k, v in (d.get("cash_flow") or {}).items():
                flow[k] = flow.get(k, 0) + v
        pl = s.get("player") or {}
        rep = (s.get("end") or {}).get("rep") or {}
        dflt = s.get("defaults") or {}
        out[os.path.basename(p).split(".")[0]] = {
            "won": str(s.get("game_over") or "alive").startswith("buyout"),
            "end": s.get("game_over") or "alive",
            "day": s.get("last_day") or len(days),
            "flow": flow,
            "board": (pl.get("appraisal") or {}).get("fees_total", 0) or 0,
            "wanted": pl.get("wanted") or {},
            "luck": (pl.get("liquid_luck") or {}).get("drunk", 0),
            "skips": dflt.get("player:rent_skip_before_buyout", 0) if isinstance(dflt, dict) else 0,
            "sec": rep.get("FACTION_SECURITY", 0), "ll": rep.get("FACTION_LOWER_LEVEL", 0),
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
        losses = {}
        for x in r.values():
            if not x["won"]:
                losses[x["end"]] = losses.get(x["end"], 0) + 1
        cash = sum(x["end"] == "buyout" for x in r.values())
        line = f"{a:28s} {k}/{n} ({100 * lo:.0f}–{100 * hi:.0f} %), {cash} for cash, median win day {wd[len(wd) // 2] if wd else '-'}"
        if losses:
            line += " | lost: " + ", ".join(f"{e} {c}" for e, c in sorted(losses.items()))
        if a != ref:
            seeds = sorted(set(r) & set(base))
            b = sum(1 for s in seeds if r[s]["won"] and not base[s]["won"])
            c = sum(1 for s in seeds if base[s]["won"] and not r[s]["won"])
            both = [r[s]["day"] - base[s]["day"] for s in seeds if r[s]["won"] and base[s]["won"]]
            line += f" | McNemar {b}:{c} p={mcnemar(b, c):.2f}"
            bc = sum(1 for s in seeds if r[s]["end"] == "buyout" and base[s]["end"] != "buyout")
            cc = sum(1 for s in seeds if base[s]["end"] == "buyout" and r[s]["end"] != "buyout")
            if bc or cc:
                line += f" (cash {bc}:{cc} p={mcnemar(bc, cc):.2f})"
            if len(both) > 1:
                m, sd = st.mean(both), st.stdev(both)
                same = sum(1 for d in both if d == 0)
                line += f" | Δ win day {m:+.1f} ± {1.96 * sd / math.sqrt(len(both)):.1f} (n={len(both)}, {same} unchanged)"
            both = [r[s]["day"] - base[s]["day"] for s in seeds if r[s]["end"] == "buyout" and base[s]["end"] == "buyout"]
            if (bc or cc) and len(both) > 1:
                line += f" | Δ cash day {st.mean(both):+.1f} ± {1.96 * st.stdev(both) / math.sqrt(len(both)):.1f} (n={len(both)})"
        print(line)
        mean = lambda f: st.mean(f(x) for x in r.values()) if r else 0
        print(f"  per run: wanted reported {mean(lambda x: x['wanted'].get('reported', 0)):.1f}, bounties {mean(lambda x: x['wanted'].get('bounties', 0)):.0f};"
              f" board {mean(lambda x: x['board']):.0f}; rents {mean(lambda x: x['flow'].get('rent', 0)):.0f} (skips {mean(lambda x: x['skips']):.2f});"
              f" services {mean(lambda x: x['flow'].get('services', 0)):.0f}; burglary goods {mean(lambda x: x['flow'].get('burglary_goods', 0)):.0f};"
              f" luck drunk {mean(lambda x: x['luck']):.2f}; end Security {mean(lambda x: x['sec']):.0f}, Lower Levels {mean(lambda x: x['ll']):.0f}")


if __name__ == "__main__":
    main()
