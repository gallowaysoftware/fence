#!/usr/bin/env python3
"""Batches of fence-rollout games (docs/research/ROLLOUT.md).

  rollout.py play    -dir DIR [-seeds 20] [-seed0 0] [-difficulty normal] [-days 1000] [-rows FILE] [-only 7,12] [-- fence-rollout args]
  rollout.py dataset -rows FILE [-seeds 20] [-seed0 1000] [-difficulty normal] [-days 1000] [-- fence-rollout args]

One game at a time (each forks up to -par branches, default 7, so the batch stays within 8 processes),
under nice 10. `play` writes DIR/s<seed>.summary.json and DIR/smoke.json in the gym's smoke shape, so
`sim/lab/scripts/arms.py report` pairs it against a `fence-gym smoke` arm on the same seeds.
`dataset` plays the base plan (`-follow base`) and only labels its decisions.
"""
import json
import os
import subprocess
import sys
import time

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
BIN = os.environ.get("FENCE_ROLLOUT_BIN") or os.path.join(REPO, "sim", "rollout", "bin", "Release", "net10.0", "fence-rollout.dll")


def parse(argv):
    extra = []
    if "--" in argv:
        i = argv.index("--")
        argv, extra = argv[:i], argv[i + 1:]
    opts = {"-dir": None, "-seeds": "20", "-seed0": None, "-difficulty": "normal", "-days": "1000", "-rows": None, "-only": None}
    it = iter(argv)
    for a in it:
        if a not in opts:
            sys.exit(__doc__)
        opts[a] = next(it)
    return opts, extra


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in ("play", "dataset"):
        sys.exit(__doc__)
    cmd = sys.argv[1]
    opts, extra = parse(sys.argv[2:])
    seed0 = int(opts["-seed0"] or (0 if cmd == "play" else 1000))
    d = opts["-dir"] or (os.path.splitext(opts["-rows"])[0] + ".games" if opts["-rows"] else None)
    if not d:
        sys.exit(__doc__)
    os.makedirs(d, exist_ok=True)
    results = []
    t0 = time.time()
    seeds = [int(x) for x in opts["-only"].split(",")] if opts["-only"] else range(seed0 + 1, seed0 + int(opts["-seeds"]) + 1)
    for seed in seeds:
        summary = os.path.join(d, f"s{seed}.summary.json")
        args = ["nice", "-n", "10", "dotnet", BIN, "play", "-seed", str(seed), "-difficulty", opts["-difficulty"],
                "-days", opts["-days"], "-summary", summary]
        if opts["-rows"]:
            args += ["-rows", opts["-rows"]]
        if cmd == "dataset":
            args += ["-follow", "base"]
        p = subprocess.run(args + extra, cwd=REPO, capture_output=True, text=True)
        print((p.stdout.strip().splitlines() or ["(no output)"])[-1], f"[{time.time() - t0:.0f} s]", flush=True)
        if p.returncode != 0 or not os.path.exists(summary):
            results.append({"seed": seed, "crash": f"exit {p.returncode}: {(p.stderr + p.stdout)[-600:]}"})
        else:
            with open(summary) as f:
                results.append(json.load(f))
        with open(os.path.join(d, "smoke.json"), "w") as f:
            json.dump(results, f, indent=1)


if __name__ == "__main__":
    main()
