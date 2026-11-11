#!/usr/bin/env python3
"""Paired strategy arms on the gym (docs/research/EXPERIMENTS.md).

  arms.py run ARMS_FILE -dir DIR [-seeds 30] [-days 200] [-parallel N] [-only NAME,...] [-- extra gym args]
  arms.py report DIR [-ref NAME] [ARM ...]

ARMS_FILE: one arm per line, `name | gym smoke args` (blank lines and # comments skipped), e.g.
  base       | -human knobs -difficulty normal
  ev         | -human knobs -difficulty normal -knobs bargain=ev
Every arm plays the same seeds 1..N (common random numbers), so differences are paired per seed.
Means are ± 1.96·sd/√n; Δ columns are paired differences against -ref (default: the first arm).
"""
import collections
import json
import math
import os
import shlex
import subprocess
import sys

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
# FENCE_GYM_DLL runs another build (e.g. a dev build while sim/gym/bin serves running games).
GYM = os.environ.get("FENCE_GYM_DLL") or os.path.join(REPO, "sim", "gym", "bin", "Release", "net10.0", "fence-gym.dll")


def ci(xs):
    n = len(xs)
    if n == 0:
        return float("nan"), float("nan")
    m = sum(xs) / n
    if n < 2:
        return m, float("nan")
    sd = math.sqrt(sum((x - m) ** 2 for x in xs) / (n - 1))
    return m, 1.96 * sd / math.sqrt(n)


def ending(r):
    if isinstance(r.get("crash"), str):
        return "CRASH"
    return r.get("game_over") or "alive"


def won(r):
    return ending(r).startswith("buyout")


def nw_at(r, day):
    """Net worth after `day` played days; a buyout keeps its last value, any other ending counts 0."""
    nw = r.get("net_worth_by_day") or []
    if len(nw) >= day:
        return nw[day - 1]
    return nw[-1] if nw and won(r) else 0


def value(r, metric):
    if metric == "days":
        return r.get("days_played", 0)
    if metric == "peak":
        return max(r.get("net_worth_by_day") or [0])
    if metric == "end":
        return (r.get("end") or {}).get("net_worth", 0)
    if metric.startswith("nw"):
        return nw_at(r, int(metric[2:]))
    raise KeyError(metric)


def load(d):
    with open(os.path.join(d, "smoke.json")) as f:
        return {r["seed"]: r for r in json.load(f)}


def parse_arms(path):
    arms = []
    with open(path) as f:
        for line in f:
            line = line.split("#", 1)[0].strip()
            if not line:
                continue
            name, _, args = line.partition("|")
            arms.append((name.strip(), shlex.split(args)))
    return arms


def run(argv):
    extra = []
    if "--" in argv:
        i = argv.index("--")
        argv, extra = argv[:i], argv[i + 1:]
    opts = {"-seeds": "30", "-days": "200", "-parallel": str(max(1, os.cpu_count() - 2)), "-dir": None, "-only": ""}
    pos = []
    it = iter(argv)
    for a in it:
        if a in opts:
            opts[a] = next(it)
        else:
            pos.append(a)
    if len(pos) != 1 or not opts["-dir"]:
        sys.exit(__doc__)
    only = set(filter(None, opts["-only"].split(",")))
    for name, args in parse_arms(pos[0]):
        if only and name not in only:
            continue
        d = os.path.join(opts["-dir"], name)
        os.makedirs(d, exist_ok=True)
        cmd = ["dotnet", GYM, "smoke", "-seeds", opts["-seeds"], "-days", opts["-days"], "-parallel", opts["-parallel"],
               "-no-trajectory", "-dir", d] + args + extra
        with open(os.path.join(d, "smoke.out"), "w") as out:
            subprocess.run(cmd, stdout=out, stderr=subprocess.STDOUT, cwd=REPO)
        with open(os.path.join(d, "args.txt"), "w") as f:
            f.write(shlex.join(args + extra) + "\n")
        print(line_for(name, load(d)), flush=True)


def line_for(name, rs):
    rs = list(rs.values())
    f = lambda t: f"{t[0]:.0f}±{t[1]:.0f}"
    ends = collections.Counter(ending(r) for r in rs)
    wins = sorted(r.get("last_day", 0) for r in rs if won(r))
    bo = f" buyout {len(wins)}/{len(rs)} (day {wins[len(wins) // 2]})" if wins else " buyout 0"
    why = collections.Counter(max(((r.get("end") or {}).get("crimes") or {"?": [0, 0]}).items(), key=lambda kv: kv[1][0])[0]
                              for r in rs if ending(r) == "arrest")
    if why:
        bo += " | arrest crime " + " ".join(f"{k} {v}" for k, v in why.most_common(3))
    alive = [r for r in rs if ending(r) == "alive"]
    if alive:
        ratios = sorted((r.get("end") or {}).get("net_worth", 0) / max(1, (r.get("end") or {}).get("buyout_price") or 17000) for r in alive)
        bo += f" | alive at cap: net worth / buyout median {ratios[len(ratios) // 2]:.2f}"
    days = sorted(value(r, "days") for r in rs)
    med = days[len(days) // 2] if days else 0
    return (f"{name:26s} n={len(rs):3d} days {f(ci([value(r, 'days') for r in rs]))} (median {med}) | "
            + " ".join(f"{k} {v}" for k, v in sorted(ends.items())) + f" |{bo} | "
            + " ".join(f"{m} {f(ci([value(r, m) for r in rs]))}" for m in ("nw30", "nw60", "nw100", "nw200", "end")))


def report(argv):
    ref = None
    if "-ref" in argv:
        i = argv.index("-ref")
        ref = argv[i + 1]
        argv = argv[:i] + argv[i + 2:]
    if not argv:
        sys.exit(__doc__)
    root = argv[0]
    names = argv[1:] or sorted(n for n in os.listdir(root) if os.path.exists(os.path.join(root, n, "smoke.json")))
    data = {n: load(os.path.join(root, n)) for n in names}
    ref = ref or names[0]
    base = data[ref]
    for n in names:
        print(line_for(n, data[n]))
    print(f"paired vs {ref}:")
    for n in names:
        if n == ref:
            continue
        seeds = sorted(set(data[n]) & set(base))
        parts = []
        for m in ("days", "nw60", "nw100", "nw200"):
            mu, h = ci([value(data[n][s], m) - value(base[s], m) for s in seeds])
            parts.append(f"Δ{m} {mu:+.0f}±{h:.0f}")
        print(f"  {n:26s} (n={len(seeds)}) " + "  ".join(parts))


if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in ("run", "report"):
        sys.exit(__doc__)
    (run if sys.argv[1] == "run" else report)(sys.argv[2:])
