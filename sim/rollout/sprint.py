#!/usr/bin/env python3
"""Deadline-sprint games on fence-rollout (docs/research/SEARCH-LABELS.md).

  sprint.py -dir DIR -seed0 N -seeds 60 [-games 4] [-par 3] [-route sprinter] [-directives FILE] [-deadline 91] [-- fence-rollout args]

Runs -games games at once, each forking up to -par branches, under nice 10; a seed whose summary exists is
skipped. Each game writes DIR/s<seed>.summary.json and, with `-rows` in the extra args, DIR/rows/<seed>.jsonl.
-directives replaces the route file (a format's materialized directives, `fenceformat write`); the route still
names the start and perks. A new game waits while the load average is above FENCE_MAX_LOAD (default 45) and stops while the game runs.
"""
import concurrent.futures as cf
import json
import os
import subprocess
import sys
import time

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
BIN = os.path.join(REPO, "sim", "rollout", "bin", "Release", "net10.0", "fence-rollout.dll")


def game_running():
    return subprocess.run(["pgrep", "-x", "Probably Stolen"], capture_output=True).returncode == 0


def play(seed, o, extra):
    summary = os.path.join(o["-dir"], f"s{seed}.summary.json")
    if os.path.exists(summary):
        return f"{seed} done"
    # Two drivers on one directory split its seeds instead of playing one twice.
    try:
        os.close(os.open(summary + ".lock", os.O_CREAT | os.O_EXCL))
    except FileExistsError:
        return f"{seed} taken by another driver"
    while os.getloadavg()[0] > float(os.environ.get("FENCE_MAX_LOAD", "45")):
        time.sleep(20)
    if game_running():
        os.remove(summary + ".lock")
        return f"{seed} skipped: the game is running"
    route = json.load(open(os.path.join(REPO, "sim/lab/scripts/directives/routes", o["-route"] + ".json")))["_route"]
    args = ["nice", "-n", "10", "dotnet", BIN, "play", "-seed", str(seed), "-difficulty", "hard", "-start", route["start"],
            "-perks", route["perks"], "-directives", o["-directives"] or os.path.join(REPO, "sim/lab/scripts/directives/routes", o["-route"] + ".json"),
            "-days", o["-deadline"], "-deadline", o["-deadline"], "-par", o["-par"], "-summary", summary + ".tmp"]
    extra = [os.path.join(o["-dir"], "rows", f"{seed}.jsonl") if a == "ROWS" else a for a in extra]
    p = subprocess.run(args + extra, cwd=REPO, capture_output=True, text=True)
    if p.returncode != 0 or not os.path.exists(summary + ".tmp"):
        os.remove(summary + ".lock")
        return f"{seed} FAILED exit {p.returncode}: {(p.stderr + p.stdout)[-400:]}"
    os.replace(summary + ".tmp", summary)
    os.remove(summary + ".lock")
    return (p.stdout.strip().splitlines() or ["(no output)"])[-1]


def main():
    argv, extra = sys.argv[1:], []
    if "--" in argv:
        i = argv.index("--")
        argv, extra = argv[:i], argv[i + 1:]
    o = {"-dir": None, "-seed0": None, "-seeds": "60", "-games": "4", "-par": "3", "-route": "sprinter", "-directives": None, "-deadline": "91"}
    it = iter(argv)
    for a in it:
        if a not in o:
            sys.exit(__doc__)
        o[a] = next(it)
    if not o["-dir"] or o["-seed0"] is None:
        sys.exit(__doc__)
    os.makedirs(os.path.join(o["-dir"], "rows"), exist_ok=True)
    seeds = range(int(o["-seed0"]) + 1, int(o["-seed0"]) + int(o["-seeds"]) + 1)
    t0 = time.time()
    with cf.ThreadPoolExecutor(int(o["-games"])) as ex:
        for line in ex.map(lambda s: play(s, o, extra), seeds):
            print(line, f"[{time.time() - t0:.0f} s]", flush=True)


if __name__ == "__main__":
    main()
