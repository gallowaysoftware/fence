#!/usr/bin/env python3
"""Fit inspectable sprint rules to search labels (docs/research/SEARCH-LABELS.md).

  sprint_fit.py -out RULES.json [-min-z 2.0] [-min-rows 5] [-min-games 3] [-depth 2] [-validated] ROWS_DIR [ROWS_DIR...]

For every (fork day, option) the fitter tries each rule "apply the option when f1 OP t1 [and f2 OP t2]" on the
fork morning's features and keeps the one with the largest summed paired gain in P(win) over the rows it fires
on, when that sum is -min-z standard errors clear of zero (per-row variance from the discordant samples). A
screened row (keep won every sample, so nothing else was rolled) counts as gain 0. The fit is repeated on each
half of the seeds (odd / even) and scored on the other half, so a rule's out-of-sample gain is shown next to it.
Options the pilot dropped (Sprint.cs `Dropped`, and the georgio fork) are never fitted: the rule policy cannot play
them. With -validated a rule is kept only when the fit on each half gains out of sample on the other.
The output keeps the best rule per fork day (options were rolled one at a time against keep, never two
together): a fence-rollout `-rules` file with one plain sentence per rule.
"""
import glob
import json
import math
import os
import sys
from collections import defaultdict

# Mirrors Sprint.Dropped plus the pilot's georgio fork.
NOT_PLAYABLE = {"fair_dealer", "insured", "no_buy", "force_selldown_liq60", "georgio_peak", "georgio_25"}

FEATURES = ["cash", "stock", "net_worth", "gap", "proj_gap", "gain_week4", "gain_week2", "cash_gap", "stock_share",
            "rent", "evidence", "georgio_lent", "insured"]


def load(dirs):
    rows = []
    for d in dirs:
        for f in sorted(glob.glob(os.path.join(d, "*.jsonl"))):
            rows += [json.loads(l) for l in open(f) if l.strip()]
    return rows


def examples(rows):
    """(day, option) -> [(seed, features, gain, var)]"""
    out = defaultdict(list)
    options = defaultdict(set)
    for r in rows:
        for c in r["candidates"][1:]:
            options[r["day"]].add(c["name"])
    for r in rows:
        keep = r["candidates"][0]
        by = {c["name"]: c for c in r["candidates"][1:]}
        for name in options[r["day"]]:
            c = by.get(name)
            if c is None:
                if not r["search"].get("screened"):
                    continue
                gain, var = 0.0, 0.0
            else:
                m = max(1, c["samples"])
                gain = c["p_win"] - keep["p_win"]
                # Paired difference of m Bernoulli pairs: each discordant sample is ±1/m.
                disc = c["wins_gained"] + c["wins_lost"]
                var = max(disc / m - gain * gain, 0) / m if m > 1 else 0.25
            out[r["day"], name].append((r["seed"], r["features"], gain, var))
    return out


def conds_fire(conds, f):
    return all((f[k] >= t) if op == ">=" else (f[k] < t) for k, op, t in conds)


def thresholds(xs, k=12):
    v = sorted(set(xs))
    if len(v) <= k:
        return v
    return sorted({v[int(i * (len(v) - 1) / k)] for i in range(1, k)})


def candidates(ex, depth):
    singles = []
    for k in FEATURES:
        for t in thresholds([f[k] for _, f, _, _ in ex]):
            singles += [[(k, ">=", t)], [(k, "<", t)]]
    yield []
    yield from singles
    if depth >= 2:
        for i, a in enumerate(singles):
            for b in singles[i + 1:]:
                if a[0][0] != b[0][0] or a[0][1] != b[0][1]:
                    yield a + b


def score(conds, ex):
    g = v = 0.0
    n, games = 0, set()
    for seed, f, gain, var in ex:
        if conds_fire(conds, f):
            g, v, n = g + gain, v + var, n + 1
            games.add(seed)
    z = g / math.sqrt(v) if v > 0 else (math.inf if g > 0 else 0)
    return g, z, n, len(games)


def best_rule(ex, o):
    best = None
    for conds in candidates(ex, o["depth"]):
        g, z, n, ng = score(conds, ex)
        if n < o["min_rows"] or ng < o["min_games"] or z < o["min_z"] or g <= 0:
            continue
        # Fewer conditions win ties; the gain is in wins (P(win) summed over fork mornings).
        key = (round(g, 3), -len(conds))
        if best is None or key > best[0]:
            best = (key, conds, g, z, n)
    return best


def sentence(day, option, conds):
    when = " and ".join(f"{k} {op} {t:g}" for k, op, t in conds) or "always"
    return f"day {day}: {option} when {when}"


def main():
    argv = sys.argv[1:]
    o = {"out": None, "min_z": 2.0, "min_rows": 5, "min_games": 3, "depth": 2, "validated": False}
    dirs = []
    it = iter(argv)
    for a in it:
        if a == "-validated":
            o["validated"] = True
        elif a.startswith("-"):
            key = a[1:].replace("-", "_")
            o[key] = next(it) if key == "out" else float(next(it))
        else:
            dirs.append(a)
    o["depth"] = int(o["depth"])
    rows = load(dirs)
    ex = examples(rows)
    fork_of = {r["day"]: r["fork"] for r in rows}
    rules, report = [], []
    for (day, option), e in sorted(ex.items()):
        if option in NOT_PLAYABLE:
            continue
        b = best_rule(e, o)
        halves = []
        for par in (0, 1):
            fit = [x for x in e if x[0] % 2 == par]
            test = [x for x in e if x[0] % 2 != par]
            hb = best_rule(fit, {**o, "min_games": max(2, o["min_games"] // 2), "min_rows": max(3, o["min_rows"] // 2)})
            halves.append(score(hb[1], test)[:3] if hb else None)
        line = f"{day:3d} {option:22s} rows {len(e):4d}  all-rows gain {sum(x[2] for x in e):+6.2f}"
        validated = all(h is not None and h[0] > 0 for h in halves)
        if b and (validated or not o["validated"]):
            _, conds, g, z, n = b
            line += f"  RULE {sentence(day, option, conds)}: gain {g:+.2f} wins over {n} rows, z {z:.1f}"
            rules.append({"fork": fork_of[day], "day": day, "option": option, "when": [list(c) for c in conds],
                          "gain": round(g, 3), "z": round(z, 2) if math.isfinite(z) else 99, "rows": n,
                          "validated": validated, "oos": [None if h is None else round(h[0], 3) for h in halves],
                          "text": sentence(day, option, conds)})
        line += "  split-half out-of-sample: " + ", ".join("none" if h is None else f"{h[0]:+.2f} ({h[2]} rows)" for h in halves)
        report.append(line)
    print("\n".join(report))
    # One rule per fork day: options were each rolled alone against keep, never two together.
    top = {}
    for r in rules:
        if r["day"] not in top or r["gain"] > top[r["day"]]["gain"]:
            top[r["day"]] = r
    rules = [top[d] for d in sorted(top)]
    if o["out"]:
        json.dump(rules, open(o["out"], "w"), indent=1)
        print(f"{len(rules)} rules -> {o['out']}")


if __name__ == "__main__":
    main()
