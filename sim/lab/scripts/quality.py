#!/usr/bin/env python3
"""Output grades and income per arm, paired against a reference arm (docs/research/QUALITY.md).

  quality.py DIR [-ref NAME] [ARM ...]

DIR is an `arms.py run` directory (one subdirectory per arm, same seeds). Per arm:
  wins (Wilson 95 %), median win day, paired win-day delta over seeds both arms won (mean ± 1.96 SE),
  McNemar exact two-sided p on wins (discordant seeds), production income per run (income.py), and the
  grades from the summary's `player.production.quality` and `wine_sold`:
    bad%     moldy + under/over-ripe share of harvested berries; best% the +2 share
    score    mean wine score of the bottles started; tiers sold Undr/Buck/Home/Prod/Res/Mast
    bay      seeded bay-nights by the health a progress point costs (0, <1, <2, <4, 4+ %), minerals low/in/high
    bottle   bottles started by true water grade
    ingots   ingots by purity VL/L/F/H/VH/P
    water    litres sold by the grade it was priced at, and by true grade
    cellar   per run: bottles in, sold and their mean price; bay-nights dry / unpowered, empty-bottle and idle-berry
             days; reed cures, cured litres and the first cure day (median)
"""
import json
import math
import os
import sys

sys.path.insert(0, os.path.dirname(__file__))
from income import contributions  # noqa: E402


def wilson(wins, runs):
    if runs == 0:
        return 0, 0
    z = 1.96
    p = wins / runs
    den = 1 + z * z / runs
    mid = (p + z * z / (2 * runs)) / den
    half = z * math.sqrt(p * (1 - p) / runs + z * z / (4 * runs * runs)) / den
    return round(100 * max(0, mid - half)), round(100 * min(1, mid + half))


def mcnemar(b, c):
    n = b + c
    if n == 0:
        return 1.0
    k = min(b, c)
    tail = sum(math.comb(n, i) for i in range(k + 1)) / 2 ** n
    return min(1.0, 2 * tail)


def mean_ci(xs):
    n = len(xs)
    if n == 0:
        return float("nan"), float("nan")
    m = sum(xs) / n
    if n < 2:
        return m, float("nan")
    sd = math.sqrt(sum((x - m) ** 2 for x in xs) / (n - 1))
    return m, 1.96 * sd / math.sqrt(n)


def load(d):
    runs = {}
    for f in sorted(os.listdir(d)):
        if f.endswith(".summary.json"):
            with open(os.path.join(d, f)) as fh:
                s = json.load(fh)
            runs[s["seed"]] = s
    return runs


def won(s):
    return (s.get("game_over") or "").startswith("buyout")


def q(s):
    return ((s.get("player") or {}).get("production") or {})


def add(acc, xs):
    for i, x in enumerate(xs or []):
        if i < len(acc):
            acc[i] += x
        else:
            acc.append(x)
    return acc


def share(xs):
    t = sum(xs) or 1
    return "/".join(f"{100 * x / t:.0f}" for x in xs)


def arm_line(name, runs, ref):
    seeds = sorted(runs)
    w = sum(won(runs[s]) for s in seeds)
    lo, hi = wilson(w, len(seeds))
    days = sorted(runs[s]["days_played"] for s in seeds if won(runs[s]))
    med = days[len(days) // 2] if days else 0
    out = [f"{name}: wins {w}/{len(seeds)} ({lo}-{hi} %), median win day {med}"]
    if ref is not None and ref is not runs:
        both = [s for s in seeds if s in ref and won(runs[s]) and won(ref[s])]
        dm, dh = mean_ci([runs[s]["days_played"] - ref[s]["days_played"] for s in both])
        b = sum(1 for s in seeds if s in ref and won(runs[s]) and not won(ref[s]))
        c = sum(1 for s in seeds if s in ref and not won(runs[s]) and won(ref[s]))
        out.append(f"  vs ref: win day {dm:+.1f} ± {dh:.1f} (n={len(both)}), wins +{b}/-{c} McNemar p={mcnemar(b, c):.2f}")
        for k in ("wine", "water", "fountain", "ingots", "inputs", "production"):
            dm, dh = mean_ci([contributions(runs[s])[k] - contributions(ref[s])[k] for s in seeds if s in ref])
            out[-1] += f", Δ{k} {dm:+.0f}±{dh:.0f}"
    inc = {k: sum(contributions(runs[s])[k] for s in seeds) / len(seeds) for k in ("wine", "water", "fountain", "ingots", "inputs", "production")}
    out.append("  income/run: " + ", ".join(f"{k} {v:.0f}" for k, v in inc.items()))
    harvest, started, tiers, bay, mins, bottle, ingots, wl, wt, wc, cost = ([] for _ in range(11))
    for s in seeds:
        p = q(runs[s])
        ws = p.get("wine_sold") or {}
        add(harvest, ws.get("harvest_by_condition"))
        add(started, ws.get("started_by_score"))
        add(tiers, ws.get("by_tier"))
        qq = p.get("quality") or {}
        add(bay, qq.get("bay_nights_by_cost"))
        cost.append(qq.get("bay_cost_per_point", 0))
        add(mins, qq.get("bay_minerals_low_in_high"))
        add(bottle, qq.get("bottle_water_by_grade"))
        add(ingots, qq.get("ingots_by_purity"))
        add(wl, qq.get("water_sold_ml_by_label"))
        add(wt, qq.get("water_sold_ml_by_true"))
        add(wc, qq.get("water_sold_credits_by_label"))
    if sum(harvest):
        h = sum(harvest)
        n_started = sum(started) or 1
        out.append(f"  berries {h / len(seeds):.0f}/run: bad {100 * (harvest[0] + harvest[1]) / h:.0f} %, best {100 * harvest[4] / h:.0f} %;"
                   f" bottles {sum(started) / len(seeds):.0f}/run, mean score {sum(i * x for i, x in enumerate(started)) / n_started:.2f},"
                   f" tiers sold U/B/H/P/R/M {share(tiers)} %")
    if sum(bay):
        out.append(f"  bay-nights by health a point 0/<1/<2/<4/4+ % {share(bay)} % (mean {sum(cost) / max(1, len(cost)):.2f} %), minerals low/in/high {share(mins)} %; bottle water {share(bottle)} %")
    cel = [(q(runs[s]).get("quality") or {}).get("cellar") for s in seeds]
    if any(cel):
        n = len(seeds)
        tot = {k: sum((c or {}).get(k, 0) for c in cel) / n for k in ("bottles_in", "bay_dry_nights", "bay_unpowered_nights", "empty_bottle_days", "berry_days")}
        eng = [((q(runs[s]).get("quality") or {}).get("engine") or {}) for s in seeds]
        firsts = sorted(e.get("reed_first_day", 0) for e in eng if e.get("reed_first_day"))
        sold = [q(runs[s]).get("wine_sold") or {} for s in seeds]
        nb = sum(x.get("bottles", 0) for x in sold)
        out.append(f"  cellar/run: bottles in {tot['bottles_in']:.0f}, sold {nb / n:.0f} at {sum(x.get('value', 0) for x in sold) / max(1, nb):.0f};"
                   f" bay-nights dry {tot['bay_dry_nights']:.0f}, unpowered {tot['bay_unpowered_nights']:.0f}; empty-bottle days {tot['empty_bottle_days']:.0f},"
                   f" berry days {tot['berry_days']:.0f}; reed cures {sum(e.get('reed_cured', 0) for e in eng) / n:.1f}"
                   f" ({sum(e.get('reed_cured_ml', 0) for e in eng) / 1000 / n:.0f} L), first cure day {firsts[len(firsts) // 2] if firsts else '-'} ({len(firsts)}/{n} runs)")
    if sum(ingots):
        out.append(f"  ingots {sum(ingots) / len(seeds):.0f}/run VL/L/F/H/VH/P {share(ingots)} %")
    if sum(wl):
        out.append(f"  water sold {sum(wl) / 1000 / len(seeds):.0f} L/run, priced as P/HQ/B/G/R/Gut {share(wl)} %, true {share(wt)} %,"
                   f" credits by priced grade {'/'.join(str(round(x / len(seeds))) for x in wc)}")
    return "\n".join(out)


def main(argv):
    if not argv:
        sys.exit(__doc__)
    d = argv[0]
    ref_name, names = None, []
    i = 1
    while i < len(argv):
        if argv[i] == "-ref":
            ref_name = argv[i + 1]
            i += 2
        else:
            names.append(argv[i])
            i += 1
    names = names or sorted(x for x in os.listdir(d) if os.path.isdir(os.path.join(d, x)))
    arms = {n: load(os.path.join(d, n)) for n in names}
    ref = arms.get(ref_name) if ref_name else None
    for n in names:
        print(arm_line(n, arms[n], ref))


if __name__ == "__main__":
    main(sys.argv[1:])
