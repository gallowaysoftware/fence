#!/usr/bin/env python3
"""The show's ghost band: each hard route's day-by-day net-worth distribution in the gym (docs/SHOW.md).

  ghost_bands.py run   [-seed0 8100000] [-seeds 60] [-parallel 8] [-dir var/ghost] [-only ROUTE,...]
  ghost_bands.py build [-dir var/ghost] [-out internal/show/data/ghost_bands.json] [-partial 1]

`run` plays every route in sim/lab/scripts/directives/routes/*.json (its `_route.args`, 1000-day horizon)
on seeds seed0+1..seed0+seeds, one gym process a seed at nice 10, and keeps each summary in
DIR/<route>/<seed>.json; a summary already there is not run again. While the game is running it launches
nothing and renices its own runs to 19. `build` writes the bands.

Net worth is the game's estimated total value (cash plus stock at the estimate), the quantity the
bridge's snapshots and the show's clock carry, on each day's morning: day 1's from `player_days`, day d's
from `net_worth_by_day[d-2]` (the gym records it after the night, equal to the next morning snapshot's).
Index i of a band is the morning of day i+1. A won run keeps its last morning before the buyout; any
other ending counts 0 (arms.py's nw_at). The curve runs until no more than a tenth of the runs still play.
"""
import json
import os
import shlex
import subprocess
import sys
import time

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))
GYM = os.environ.get("FENCE_GYM_DLL") or os.path.join(REPO, "sim", "gym", "bin", "Release", "net10.0", "fence-gym.dll")
ROUTES = os.path.join(REPO, "sim", "lab", "scripts", "directives", "routes")
GAME = "Probably Stolen.exe"
PCTS = (10, 25, 50, 75, 90)


def flags(argv, defaults):
    out = dict(defaults)
    i = 0
    while i < len(argv):
        k = argv[i].lstrip("-")
        if k not in out:
            sys.exit(f"unknown flag {argv[i]}")
        out[k] = type(defaults[k])(argv[i + 1])
        i += 2
    return out


def routes(only=""):
    names = sorted(f[:-5] for f in os.listdir(ROUTES) if f.endswith(".json"))
    if only:
        names = [n for n in names if n in only.split(",")]
    return names


def route_meta(name):
    with open(os.path.join(ROUTES, name + ".json")) as f:
        return json.load(f)["_route"]


def game_running():
    """By image name, as internal/show/proc.go: a shell that merely mentions the game is not it."""
    for pid in os.listdir("/proc"):
        if not pid.isdigit():
            continue
        try:
            with open(f"/proc/{pid}/cmdline", "rb") as f:
                arg0 = f.read().split(b"\0")[0].decode(errors="replace")
            with open(f"/proc/{pid}/comm") as f:
                comm = f.read().strip()
        except OSError:
            continue
        if arg0.replace("\\", "/").rsplit("/", 1)[-1].lower() == GAME.lower() or comm == GAME[:15]:
            return True
    return False


def run(argv):
    o = flags(argv, {"seed0": 8100000, "seeds": 60, "parallel": 8, "dir": "var/ghost", "only": ""})
    jobs = []
    for name in routes(o["only"]):
        args = shlex.split(route_meta(name)["args"])
        os.makedirs(os.path.join(o["dir"], name), exist_ok=True)
        for seed in range(o["seed0"] + 1, o["seed0"] + o["seeds"] + 1):
            out = os.path.join(o["dir"], name, f"{seed}.json")
            if not os.path.exists(out):
                jobs.append((name, seed, out, args))
    print(f"{len(jobs)} runs to play", flush=True)
    live, done, reniced = [], 0, False
    while jobs or live:
        for p, name, seed, out, log in list(live):
            if p.poll() is not None:
                live.remove((p, name, seed, out, log))
                log.close()
                done += 1
                if p.returncode != 0 or not os.path.exists(out + ".tmp"):
                    print(f"FAIL {name} {seed} rc={p.returncode} (see {out}.log)", flush=True)
                else:
                    os.replace(out + ".tmp", out)
                    os.remove(out + ".log")
                if done % 20 == 0:
                    print(f"{done} done, {len(jobs)} queued", flush=True)
        playing = game_running()
        if playing and not reniced:
            for p, *_ in live:
                subprocess.run(["renice", "-n", "19", "-p", str(p.pid)], capture_output=True)
            print("game running: launching nothing, own runs at nice 19", flush=True)
        reniced = playing
        while jobs and not playing and len(live) < o["parallel"]:
            name, seed, out, args = jobs.pop(0)
            log = open(out + ".log", "w")
            cmd = ["nice", "-n", "10", "dotnet", GYM, "run", "-seed", str(seed), "-days", "1000", "-no-trajectory",
                   "-summary", out + ".tmp", *args, "-directives", os.path.join(ROUTES, name + ".json")]
            live.append((subprocess.Popen(cmd, cwd=REPO, stdout=log, stderr=subprocess.STDOUT), name, seed, out, log))
        time.sleep(2 if not playing else 30)


def won(r):
    return (r.get("game_over") or "").startswith("buyout")


def mornings(r):
    return [r["player_days"][0]["net_worth_start"]] + (r.get("net_worth_by_day") or [])


def nw_at(r, day):
    nw = r["_mornings"]
    if len(nw) >= day:
        return nw[day - 1]
    return nw[-1] if nw and won(r) else 0


def pct(xs, p):
    """Linear-interpolated percentile of sorted xs."""
    k = (len(xs) - 1) * p / 100
    lo = int(k)
    hi = min(lo + 1, len(xs) - 1)
    return round(xs[lo] + (xs[hi] - xs[lo]) * (k - lo))


def band(runs):
    for r in runs:
        r["_mornings"] = mornings(r)
    playing = lambda d: sum(1 for r in runs if len(r["_mornings"]) >= d)
    last = 1
    while playing(last + 1) > len(runs) / 10:
        last += 1
    out = {f"p{p}": [] for p in PCTS}
    for d in range(1, last + 1):
        xs = sorted(nw_at(r, d) for r in runs)
        for p in PCTS:
            out[f"p{p}"].append(pct(xs, p))
    win_days = sorted(r["last_day"] for r in runs if won(r))
    return {
        "runs": len(runs),
        "wins": len(win_days),
        "median_win_day": pct(win_days, 50) if win_days else 0,
        "days": last,
        **out,
    }


def build(argv):
    o = flags(argv, {"dir": "var/ghost", "out": "internal/show/data/ghost_bands.json", "seed0": 8100000, "seeds": 60, "partial": 0})
    seeds = range(o["seed0"] + 1, o["seed0"] + o["seeds"] + 1)
    bands = {}
    for name in routes():
        runs = []
        for seed in seeds:
            path = os.path.join(o["dir"], name, f"{seed}.json")
            if not os.path.exists(path):
                if o["partial"]:
                    break
                sys.exit(f"missing {path}: run first")
            with open(path) as f:
                r = json.load(f)
            if isinstance(r.get("crash"), str):
                sys.exit(f"{path} crashed: {r['crash'][:200]}")
            runs.append(r)
        else:
            bands[name] = band(runs)
    head = {
        "difficulty": "hard",
        "seeds": f"{seeds[0]}-{seeds[-1]}",
        "horizon": 1000,
        "net_worth": "cash plus stock at the estimate on the morning of day i+1 (index i); a won run keeps its last morning before the buyout, any other ending counts 0",
        "script": "sim/lab/scripts/ghost_bands.py",
    }
    lines = ["{", f'"source": {json.dumps(head)},', '"routes": {']
    for i, name in enumerate(bands):
        sep = "," if i < len(bands) - 1 else ""
        lines.append(f"{json.dumps(name)}: {json.dumps(bands[name], separators=(',', ':'))}{sep}")
    lines += ["}", "}"]
    os.makedirs(os.path.dirname(o["out"]) or ".", exist_ok=True)
    with open(o["out"], "w") as f:
        f.write("\n".join(lines) + "\n")
    for name, b in bands.items():
        d = min(50, b["days"])
        print(f"{name:14} wins {b['wins']}/{b['runs']} median win day {b['median_win_day']:3} curve {b['days']:3} days; "
              f"day {d}: {b['p10'][d-1]} / {b['p50'][d-1]} / {b['p90'][d-1]}")


if __name__ == "__main__":
    if len(sys.argv) < 2 or sys.argv[1] not in ("run", "build"):
        sys.exit(__doc__)
    {"run": run, "build": build}[sys.argv[1]](sys.argv[2:])
