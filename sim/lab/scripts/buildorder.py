#!/usr/bin/env python3
"""Paired arms for the build planner (docs/research/BUILD-ORDER.md), at a fixed number of gym processes.

  buildorder.py run ARMS_FILE -dir DIR [-seeds 16] [-seed0 N] [-days 1000] [-procs 2] [-only A,B] [-- extra gym args]
  buildorder.py report DIR -ref NAME [ARM ...]

ARMS_FILE lines are `name | gym run args` (as arms.py). Unlike arms.py this starts each seed as its own `fence-gym run`
(no smoke parent), at most -procs at once across all arms, and skips seeds already summarised (resumable). It writes
DIR/<arm>/smoke.json in arms.py's format, so arms.py and quality.py read it too.

report: wins (Wilson 95 %), median win day, paired Δ win day over seeds both won and Δ last day over all seeds
(mean ± 1.96 SE), exact McNemar on wins, and the planner's telemetry (`player.build`): kits approved / completed,
spend, and the days kits waited.
"""
import collections
import json
import math
import os
import shlex
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(__file__))
from arms import parse_arms  # noqa: E402
from quality import mcnemar, wilson  # noqa: E402
from income import contributions  # noqa: E402

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
GYM = os.environ.get("FENCE_GYM_DLL") or os.path.join(REPO, "sim", "gym", "bin", "Release", "net10.0", "fence-gym.dll")


def run(argv):
    extra = []
    if "--" in argv:
        i = argv.index("--")
        argv, extra = argv[:i], argv[i + 1:]
    o = {"-seeds": "16", "-seed0": "0", "-days": "1000", "-procs": "2", "-dir": None, "-only": ""}
    pos = []
    it = iter(argv)
    for a in it:
        if a in o:
            o[a] = next(it)
        else:
            pos.append(a)
    if len(pos) != 1 or not o["-dir"]:
        sys.exit(__doc__)
    only = set(filter(None, o["-only"].split(",")))
    arms = [(n, a) for n, a in parse_arms(pos[0]) if not only or n in only]
    seeds = [int(o["-seed0"]) + k for k in range(1, int(o["-seeds"]) + 1)]
    queue = []
    for name, args in arms:
        d = os.path.join(o["-dir"], name)
        os.makedirs(d, exist_ok=True)
        with open(os.path.join(d, "args.txt"), "w") as f:
            f.write(shlex.join(args + extra) + "\n")
        for s in seeds:
            summ = os.path.join(d, f"s{s}.summary.json")
            if not os.path.exists(summ):
                queue.append((name, d, s, args))
    # Seed-major order: every arm advances together, so a stopped batch still pairs.
    queue.sort(key=lambda x: (x[2], [n for n, _ in arms].index(x[0])))
    running = []
    procs = int(o["-procs"])
    while queue or running:
        if os.system("pgrep -x 'Probably Stolen' >/dev/null") == 0:
            for p, *_ in running:
                p.kill()
            sys.exit("the game is running: stopped")
        while queue and len(running) < procs:
            name, d, s, args = queue.pop(0)
            cmd = ["dotnet", GYM, "run", "-seed", str(s), "-days", o["-days"], "-no-trajectory",
                   "-out", os.path.join(d, f"s{s}.jsonl"), "-summary", os.path.join(d, f"s{s}.summary.json")] + args + extra
            log = open(os.path.join(d, f"s{s}.out"), "w")
            running.append((subprocess.Popen(cmd, stdout=log, stderr=subprocess.STDOUT, cwd=REPO), log, name, d, s))
        time.sleep(2)
        for r in list(running):
            if r[0].poll() is not None:
                r[1].close()
                running.remove(r)
                print(f"{r[2]} s{r[4]} exit {r[0].returncode}", flush=True)
    for name, _ in arms:
        collect(os.path.join(o["-dir"], name))


def collect(d):
    rs = []
    for f in sorted(os.listdir(d)):
        if f.endswith(".summary.json"):
            with open(os.path.join(d, f)) as fh:
                rs.append(json.load(fh))
    with open(os.path.join(d, "smoke.json"), "w") as fh:
        json.dump(rs, fh)
    return rs


def load(d):
    return {r["seed"]: r for r in collect(d)}


def won(r):
    return (r.get("game_over") or "").startswith("buyout")


def ci(xs):
    if not xs:
        return float("nan"), float("nan")
    m = sum(xs) / len(xs)
    if len(xs) < 2:
        return m, float("nan")
    sd = math.sqrt(sum((x - m) ** 2 for x in xs) / (len(xs) - 1))
    return m, 1.96 * sd / math.sqrt(len(xs))


def med(xs):
    xs = sorted(xs)
    return xs[len(xs) // 2] if xs else None


def build_line(rs):
    approved, done, spent, waits = collections.Counter(), collections.Counter(), collections.Counter(), collections.Counter()
    for r in rs:
        b = (r.get("player") or {}).get("build") or {}
        for k, v in (b.get("kits") or {}).items():
            if "approved_day" in v:
                approved[k] += 1
            if v.get("complete_day") is not None:
                done[k] += 1
            spent[k] += v.get("spent", 0)
        for k, v in (b.get("waiting") or {}).items():
            waits[k] += v
    n = max(1, len(rs))
    kits = " ".join(f"{k} {approved[k]}/{done[k]} ({spent[k] / n:.0f})" for k in sorted(set(approved) | set(spent)))
    w = " ".join(f"{k} {v / n:.0f}" for k, v in waits.most_common(4))
    return f"kits approved/complete (spend a run): {kits or '-'} | waiting mornings a run: {w or '-'}"


def engines(r):
    """Per run: money by source (income.py) and each engine's own output."""
    c = contributions(r)
    p = r.get("player") or {}
    lab = next((d.get("lab") for d in reversed(r.get("player_days") or []) if d.get("lab")), None) or {}
    prod = p.get("production") or {}
    crops = prod.get("crops") or {}
    cellar = ((prod.get("quality") or {}).get("cellar")) or {}
    arms = p.get("arms") or {}
    first_lab = next((d["day"] for d in r.get("player_days") or [] if (d.get("lab") or {}).get("unlocked")), None)
    return {
        "georgio": c["georgio"], "counter": c["counter"], "production": c["production"], "wine": c["wine"],
        "pills": lab.get("pill_sales", 0) or 0, "crops": crops.get("sales", 0) or 0,
        "machines_missing": sum(v for k, v in (lab.get("stalls") or {}).items() if k.startswith("machines")),
        "unpowered": cellar.get("bay_unpowered_nights", 0) or 0, "lab_day": first_lab or 0,
        "forge": (arms.get("forge") or {}).get("income", 0) or 0,
        "arrests": 1 if r.get("game_over") == "arrest" else 0,
    }


def report(argv):
    if "-ref" not in argv:
        sys.exit(__doc__)
    i = argv.index("-ref")
    ref = argv[i + 1]
    argv = argv[:i] + argv[i + 2:]
    root = argv[0]
    names = argv[1:] or sorted(n for n in os.listdir(root) if os.path.isdir(os.path.join(root, n)))
    data = {n: load(os.path.join(root, n)) for n in names}
    base = data[ref]
    for n in names:
        rs = data[n]
        seeds = sorted(set(rs) & set(base))
        w = sum(won(rs[s]) for s in seeds)
        lo, hi = wilson(w, len(seeds))
        ends = collections.Counter(rs[s].get("game_over") or "alive" for s in seeds)
        line = f"{n:28s} n={len(seeds):3d} wins {w}/{len(seeds)} ({lo}-{hi} %) median win {med([rs[s]['last_day'] for s in seeds if won(rs[s])])}"
        line += " | " + " ".join(f"{k} {v}" for k, v in sorted(ends.items()))
        if n != ref:
            both = [s for s in seeds if won(rs[s]) and won(base[s])]
            m, h = ci([rs[s]["last_day"] - base[s]["last_day"] for s in both])
            ml, hl = ci([rs[s]["last_day"] - base[s]["last_day"] for s in seeds])
            b = sum(won(rs[s]) and not won(base[s]) for s in seeds)
            c = sum(won(base[s]) and not won(rs[s]) for s in seeds)
            line += f" | Δwin day {m:+.1f} ± {h:.1f} (n={len(both)}) Δlast day {ml:+.1f} ± {hl:.1f} | McNemar {b}:{c} p={mcnemar(b, c):.2g}"
        print(line)
        es = [engines(rs[s]) for s in seeds]
        keys = ["georgio", "counter", "production", "wine", "pills", "crops", "forge", "machines_missing", "unpowered"]
        line2 = " ".join(f"{k} {sum(e[k] for e in es) / max(1, len(es)):.0f}" for k in keys)
        labs = sorted(e["lab_day"] for e in es if e["lab_day"])
        if labs:
            line2 += f" lab_unlocked {len(labs)}/{len(es)} (median day {labs[len(labs) // 2]})"
        print("    per run: " + line2)
        if n != ref:
            eb = {s: engines(base[s]) for s in seeds}
            print("    paired Δ a run: " + " ".join(f"{k} {ci([engines(rs[s])[k] - eb[s][k] for s in seeds])[0]:+.0f}±{ci([engines(rs[s])[k] - eb[s][k] for s in seeds])[1]:.0f}" for k in ("georgio", "counter", "production", "pills", "crops")))
        if any((rs[s].get("player") or {}).get("build") for s in seeds):
            print("    " + build_line([rs[s] for s in seeds]))


if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in ("run", "report"):
        sys.exit(__doc__)
    (run if sys.argv[1] == "run" else report)(sys.argv[2:])
