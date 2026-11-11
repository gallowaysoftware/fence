#!/usr/bin/env python3
"""Policy search on the headless gym (docs/research/POLICY-SEARCH.md).

Finds the best fixed policy -- directive levers of the `-human player` executor, or knob settings of
`-human knobs`, plus gym flags such as -perks -- by playing real-rules episodes in parallel gym processes.

  search.py run         -study DIR [-space FILE] [-tracks normal,hard] [-until HH:MM] [-workers 8]
  search.py extend      -study DIR [-tracks ...] [-top 3] [-seeds 48]   best candidates onto more training seeds
  search.py holdout     -study DIR [-tracks ...] [-diffs normal,hard] [-seed0 100000] [-top 2] [-seeds 100] [-refs a,b] [-extra NAME=KEY ...]
  search.py sensitivity -study DIR -track T [-seeds 16] [-config best|KEY]
  search.py report      -study DIR [-top 10]                            leaderboards + search distributions
  search.py export      -study DIR [-top 2]                             arms.py lines (+ directives files)

The space file (default sim/search/space-player.json; space.json is the -human knobs space) names the
human role, reference policies and every lever: `dir` levers go into a directives file (-directives),
`flag` levers are gym flags, the rest are -knobs entries (checked against sim/gym/Knobs.cs at startup).
Types: cat (JSON values, null = unset), int/float (lo/hi, log scale, optional on/off gate), subset (an
ordered pool, Bernoulli per item; budget/max_items repair; optional gate). `when` makes a lever
conditional on another's value. An optional "search" object overrides pop/rungs/extend.

Objective per episode (horizon H = 1000 days unless the space says otherwise; the run stops at a win,
a game over or day H):
  win (ending buyout*)   3 + (H - day)/H                 in (3, 4]: earlier wins score more
  alive at H             1 + min(1, net worth / buyout price at H)     in [1, 2]
  game over at day d     0.9 d/H + 0.1 min(1, net worth on day 100 / buyout price on day 100)   in [0, 1)
Buyout price: docs/re/DAY.md (normal 12 000 + 50/day after day 50, cap +5 000; hard 24 000, cap +10 000).
A track scores a seed by the mean over its difficulties ("combined" = normal + hard).

Search, per track: cross-entropy over a factored distribution (diagonal Gaussian per numeric lever in
[0,1]-normalised space, categorical per discrete lever, Bernoulli per subset item and per optional gate).
Each generation samples `pop` policies (+ the track's 2 best so far; generation 0 also the space's refs),
races them by successive halving on a fixed training seed prefix (`rungs`, half survive each rung), and
moves the distribution 30 % toward the upper half of each rung from the second on. Every 3 generations
the 3 best are extended to `extend` seeds. Tracks run concurrently on one pool and share an episode cache
keyed on (policy, difficulty, seed). Seeds: training 1.., held-out 100001.., sensitivity 200001...

A study dir (var/search/<name>) holds evals.jsonl, candidates.jsonl, state-<track>.json, dirs/ (the
directives files), log.txt; rerunning resumes. meta.json stamps the gym and game binaries and the space;
a study refuses other binaries (start a new one).

Load: at most -workers gym processes (default 8) at nice 10; while "Probably Stolen.exe" runs, at most 4
at nice 19 (running ones are reniced).
"""
import collections
import hashlib
import json
import math
import os
import random
import re
import shlex
import signal
import subprocess
import sys
import threading
import time

REPO = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
GYM = os.path.join(REPO, "sim", "gym", "bin", "Release", "net10.0", "fence-gym.dll")
KNOBS_CS = os.path.join(REPO, "sim", "gym", "Knobs.cs")
GAME_DIR = os.environ.get("GAME_DIR", os.path.expanduser("~/.local/share/Steam/steamapps/common/Probably Stolen"))
DEFAULT_SPACE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "space-player.json")

ALPHA = 0.3
P_FLOOR = 0.03
SIGMA_FLOOR = 0.04
TRAIN0, HOLD0, SENS0 = 0, 100000, 200000
DIFFS = {"normal": ["normal"], "hard": ["hard"], "combined": ["normal", "hard"]}
# docs/re/DAY.md: base, per day after day 50, cap
BUYOUT = {"cozy": (12000, 25, 3000), "normal": (12000, 50, 5000), "hard": (24000, 50, 10000), "challenge": (36000, 100, 20000)}
NW_STEP = 10


def buyout_price(diff, day):
    b, per, cap = BUYOUT[diff]
    return b + min(max(0, day - 50) * per, cap)


def log(study, msg):
    line = time.strftime("%H:%M:%S ") + msg
    print(line, flush=True)
    with open(os.path.join(study, "log.txt"), "a") as f:
        f.write(line + "\n")


# ---------------------------------------------------------------- space


class Space:
    def __init__(self, path):
        with open(path) as f:
            j = json.load(f)
        self.path = os.path.abspath(path)
        self.spec = j["knobs"]
        self.human = j.get("human", "knobs")
        # gym args every episode gets, e.g. ["-start", "apprentice"]
        self.extra = j.get("args", [])
        self.refs = {k: (a if "-human" in a else ["-human", self.human] + self.extra + a) for k, a in j.get("refs", {}).items()}
        self.horizon = j.get("horizon", 1000)
        s = j.get("search", {})
        self.pop, self.rungs, self.extend_n = s.get("pop", 16), s.get("rungs", [4, 8, 16, 32]), s.get("extend", 48)
        # [lo, hi]: local search -- each sample redraws lo..hi levers of the track's best policy from the
        # distribution instead of drawing every lever; for spaces whose default is already strong.
        self.mutate = s.get("mutate")
        # the local search's incumbent until a policy has finished the last rung (default: the executor's rules)
        self.init = j.get("init", {})
        knob_names = [k for k, v in self.spec.items() if "flag" not in v and "dir" not in v]
        if knob_names:
            known = set(re.findall(r'"(\w+)"', re.search(r"Known\s*=.*?\{(.*?)\};", open(KNOBS_CS).read(), re.S).group(1)))
            bad = [k for k in knob_names if k not in known]
            if bad:
                sys.exit(f"space knobs not in Knobs.cs: {bad} (known: {sorted(known)})")
        self.names = list(self.spec)

    # A policy is {lever: value}; None = unset (the executor's own rule).
    def active(self, pol, name):
        w = self.spec[name].get("when")
        return not w or all(pol.get(k) in v for k, v in w.items())

    def value(self, n, v):
        """The emitted value, or None when the lever is unset/default."""
        s = self.spec[n]
        if v is None:
            return None
        t = s["type"]
        if t == "subset":
            v = list(v)
            if "flag" in s:
                v = "+".join(v) if v else s.get("empty", "none")
        elif t == "float":
            v = round(float(v), s.get("digits", 1))
        elif t == "int":
            v = int(v)
        if "default" in s and json.dumps(v) == json.dumps(s["default"]):
            return None
        return v

    def args(self, pol):
        """Gym args: flags, -knobs, and a ("-directives-json", JSON) pair the runner turns into a file."""
        flags, knobs, dirs = [], [], {}
        for n in sorted(self.names):
            if not self.active(pol, n):
                continue
            v = self.value(n, pol.get(n))
            if v is None:
                continue
            s = self.spec[n]
            if "flag" in s:
                if v != "":
                    flags += [s["flag"], str(v)]
            elif "dir" in s:
                node = dirs
                *path, leaf = s["dir"].split(".")
                for p in path:
                    node = node.setdefault(p, {})
                node[leaf] = v
            else:
                knobs.append(f"{n}={v:g}" if isinstance(v, float) else f"{n}={v}")
        out = ["-human", self.human] + self.extra + flags
        if knobs:
            out += ["-knobs", ",".join(knobs)]
        if dirs:
            out += ["-directives-json", json.dumps(dirs, sort_keys=True, separators=(",", ":"))]
        return out

    def key(self, pol):
        return " ".join(self.args(pol))

    # ---- distribution
    def init_dist(self):
        d = {}
        for n, s in self.spec.items():
            t = s["type"]
            if t == "cat":
                pr = s.get("prior", [1] * len(s["values"]))
                d[n] = {"p": [x / sum(pr) for x in pr]}
            elif t in ("int", "float"):
                d[n] = {"mu": self.norm(n, s["default"]) if "default" in s else 0.5, "sd": 0.3}
            elif t == "subset":
                d[n] = {"p": list(s.get("p", [s.get("p0", 0.3)] * len(s["pool"])))}
            if s.get("optional"):
                d[n]["on"] = s.get("p_on", 0.5)
        return d

    def norm(self, n, v):
        s = self.spec[n]
        if s.get("log"):
            return (math.log(v) - math.log(s["lo"])) / (math.log(s["hi"]) - math.log(s["lo"]))
        return (v - s["lo"]) / (s["hi"] - s["lo"])

    def denorm(self, n, u):
        s = self.spec[n]
        u = min(1.0, max(0.0, u))
        x = math.exp(math.log(s["lo"]) + u * (math.log(s["hi"]) - math.log(s["lo"]))) if s.get("log") else s["lo"] + u * (s["hi"] - s["lo"])
        return int(round(x)) if s["type"] == "int" else round(x, s.get("digits", 1))

    def sample(self, d, rng):
        pol = {}
        for n, s in self.spec.items():
            t, dn = s["type"], d[n]
            if s.get("optional") and rng.random() >= dn["on"]:
                pol[n] = None
            elif t == "cat":
                pol[n] = rng.choices(s["values"], dn["p"])[0]
            elif t in ("int", "float"):
                pol[n] = self.denorm(n, rng.gauss(dn["mu"], dn["sd"]))
            elif t == "subset":
                pol[n] = self.repair(n, [i for i, p in enumerate(dn["p"]) if rng.random() < p], rng)
        return pol

    def repair(self, n, idx, rng):
        s = self.spec[n]
        if "budget" not in s:
            return [s["pool"][i] for i in idx]
        rng.shuffle(idx)
        out, pts = [], 0
        for i in idx:
            if pts + s["cost"][i] <= s["budget"] and len(out) < s.get("max_items", 99):
                out.append(i)
                pts += s["cost"][i]
        return [s["pool"][i] for i in sorted(out)]

    def update(self, d, elites):
        for n, s in self.spec.items():
            t, dn = s["type"], d[n]
            es = [e for e in elites if self.active(e, n)]
            if not es:
                continue
            if s.get("optional"):
                on = sum(1 for e in es if e.get(n) is not None) / len(es)
                dn["on"] = min(1 - P_FLOOR, max(P_FLOOR, (1 - ALPHA) * dn["on"] + ALPHA * on))
                es = [e for e in es if e.get(n) is not None]
                if not es:
                    continue
            if t == "cat":
                f = [sum(1 for e in es if json.dumps(e.get(n)) == json.dumps(v)) / len(es) for v in s["values"]]
                dn["p"] = _floor([(1 - ALPHA) * p + ALPHA * x for p, x in zip(dn["p"], f)])
            elif t in ("int", "float"):
                xs = [self.norm(n, e[n]) for e in es]
                m = sum(xs) / len(xs)
                sd = math.sqrt(sum((x - m) ** 2 for x in xs) / len(xs))
                dn["mu"] = (1 - ALPHA) * dn["mu"] + ALPHA * m
                dn["sd"] = max(SIGMA_FLOOR, (1 - ALPHA) * dn["sd"] + ALPHA * sd)
            elif t == "subset":
                for i, item in enumerate(s["pool"]):
                    f = sum(1 for e in es if item in (e.get(n) or [])) / len(es)
                    dn["p"][i] = min(1 - P_FLOOR, max(P_FLOOR, (1 - ALPHA) * dn["p"][i] + ALPHA * f))

    def neighbours(self, pol):
        """One-at-a-time alternatives for sensitivity: (label, policy)."""
        out = []
        for n, s in self.spec.items():
            if not self.active(pol, n):
                continue
            cur, t = pol.get(n), s["type"]
            if cur is not None and s.get("optional"):
                out.append((f"{n} unset", {**pol, n: None}))
            if t == "cat":
                for v in s["values"]:
                    if json.dumps(v) != json.dumps(cur):
                        out.append((f"{n}={json.dumps(v)}", {**pol, n: v}))
            elif t in ("int", "float"):
                if cur is None:
                    out += [(f"{n}={self.denorm(n, u)}", {**pol, n: self.denorm(n, u)}) for u in (0.0, 0.5, 1.0)]
                else:
                    u = self.norm(n, cur)
                    for du in (-0.25, 0.25):
                        if 0 <= u + du <= 1:
                            out.append((f"{n}={self.denorm(n, u + du)}", {**pol, n: self.denorm(n, u + du)}))
            elif t == "subset":
                if cur is None:
                    continue
                for item in s["pool"]:
                    if item in cur:
                        out.append((f"{n} -{item}", {**pol, n: [x for x in cur if x != item]}))
                    else:
                        nxt = [x for x in s["pool"] if x in cur or x == item]
                        if "budget" in s and (sum(s["cost"][s["pool"].index(x)] for x in nxt) > s["budget"] or len(nxt) > s.get("max_items", 99)):
                            continue
                        out.append((f"{n} +{item}", {**pol, n: nxt}))
        return out


def _floor(ps):
    ps = [max(P_FLOOR, p) for p in ps]
    t = sum(ps)
    return [p / t for p in ps]


# ---------------------------------------------------------------- episodes


def summarize(seed, diff, horizon, j):
    if isinstance(j.get("crash"), str) or "days_played" not in j:
        return {"seed": seed, "diff": diff, "days": 0, "ending": "CRASH", "win": 0, "obj": 0.0, "nw": [], "crash": str(j.get("crash") or j.get("error"))[:300]}
    ending = j.get("game_over") or "alive"
    win = 1 if ending.startswith("buyout") else 0
    nw = j.get("net_worth_by_day") or []
    days = min(horizon, j.get("days_played", 0))
    last = j.get("last_day") or days
    peak = max(nw) if nw else 0
    end_nw = nw[-1] if nw else 0
    if win:
        obj = 3 + (horizon - last) / horizon
    elif ending == "alive":
        obj = 1 + min(1.0, end_nw / buyout_price(diff, horizon))
    else:
        nw100 = nw[99] if len(nw) >= 100 else 0
        obj = 0.9 * days / horizon + 0.1 * min(1.0, nw100 / buyout_price(diff, 100))
    return {"seed": seed, "diff": diff, "days": days, "last_day": last, "ending": ending, "win": win, "obj": obj, "peak_nw": peak,
            "end_nw": end_nw, "nw": nw[NW_STEP - 1::NW_STEP], "permit": (j.get("end") or {}).get("permit"),
            "production": (j.get("player") or {}).get("production")}


class Runner:
    """Bounded pool of gym processes with an episode cache keyed on (policy key, difficulty, seed)."""

    def __init__(self, study, space, workers):
        self.study, self.space, self.max_workers = study, space, workers
        self.cache, self.inflight = {}, {}
        self.lock = threading.Condition()
        self.active = 0
        self.children = set()
        self.game = False
        self.n_run, self.t_run = 0, 0.0
        path = os.path.join(study, "evals.jsonl")
        if os.path.exists(path):
            with open(path) as f:
                for line in f:
                    try:
                        e = json.loads(line)
                    except ValueError:
                        continue
                    self.cache[(e["key"], e["diff"], e["seed"])] = e
        self.out = open(path, "a")
        self.tmp = os.path.join(study, "tmp")
        self.dirs = os.path.join(study, "dirs")
        os.makedirs(self.tmp, exist_ok=True)
        os.makedirs(self.dirs, exist_ok=True)
        threading.Thread(target=self._watch, daemon=True).start()

    def limits(self):
        return (min(4, self.max_workers), 19) if self.game else (self.max_workers, 10)

    def _watch(self):
        while True:
            g = game_running()
            if g != self.game:
                self.game = g
                log(self.study, "game running: 4 workers, nice 19" if g else f"game gone: {self.max_workers} workers, nice 10")
                if g:
                    for pid in list(self.children):
                        try:
                            os.setpriority(os.PRIO_PGRP, pid, 19)
                        except OSError:
                            pass
                with self.lock:
                    self.lock.notify_all()
            time.sleep(10)

    def materialize(self, args):
        """Replace a ("-directives-json", JSON) pair by "-directives FILE"."""
        out, it = [], iter(args)
        for a in it:
            if a == "-directives-json":
                js = next(it)
                path = os.path.join(self.dirs, hashlib.sha1(js.encode()).hexdigest()[:16] + ".json")
                if not os.path.exists(path):
                    tmp = f"{path}.{threading.get_ident()}.tmp"
                    with open(tmp, "w") as f:
                        f.write(js + "\n")
                    os.replace(tmp, path)
                out += ["-directives", path]
            else:
                out.append(a)
        return out

    def get(self, key, diff, seed):
        return self.cache.get((key, diff, seed))

    def evaluate(self, jobs):
        """jobs: [(key, args, diff, seed)]; runs the missing ones, returns when all are cached."""
        todo, seen, waits = [], set(), []
        with self.lock:
            for key, args, diff, seed in jobs:
                k = (key, diff, seed)
                if k in self.cache or k in seen:
                    continue
                seen.add(k)
                if k in self.inflight:
                    waits.append(self.inflight[k])
                else:
                    self.inflight[k] = threading.Event()
                    todo.append((key, args, diff, seed))
        threads = []
        for j in todo:
            with self.lock:
                while self.active >= self.limits()[0]:
                    self.lock.wait()
                self.active += 1
            t = threading.Thread(target=self._one, args=j)
            t.start()
            threads.append(t)
        for t in threads:
            t.join()
        for w in waits:
            w.wait()

    def _one(self, key, args, diff, seed):
        try:
            summ = os.path.join(self.tmp, f"{threading.get_ident()}-{seed}-{diff}.json")
            cmd = ["nice", "-n", str(self.limits()[1]), "dotnet", GYM, "run", "-seed", str(seed), "-days", str(self.space.horizon),
                   "-difficulty", diff, "-no-trajectory", "-summary", summ, "-out", summ + "l"] + self.materialize(args)
            t0 = time.time()
            p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=REPO, start_new_session=True)
            self.children.add(p.pid)
            out, _ = p.communicate()
            self.children.discard(p.pid)
            try:
                with open(summ) as f:
                    j = json.load(f)
                os.remove(summ)
            except (OSError, ValueError):
                j = {}
            if p.returncode != 0 or not j:
                j = {"crash": f"exit {p.returncode}: {out.decode(errors='replace')[-300:]}"}
            e = summarize(seed, diff, self.space.horizon, j)
            e["key"] = key
            e["sec"] = round(time.time() - t0, 1)
            with self.lock:
                self.cache[(key, diff, seed)] = e
                self.out.write(json.dumps(e, separators=(",", ":")) + "\n")
                self.out.flush()
                self.n_run += 1
                self.t_run += time.time() - t0
        finally:
            with self.lock:
                self.active -= 1
                ev = self.inflight.pop((key, diff, seed), None)
                self.lock.notify_all()
            if ev:
                ev.set()


def game_running():
    for pid in os.listdir("/proc"):
        if not pid.isdigit():
            continue
        try:
            with open(f"/proc/{pid}/cmdline", "rb") as f:
                argv0 = f.read().split(b"\0", 1)[0]
        except OSError:
            continue
        if argv0.replace(b"\\", b"/").rsplit(b"/", 1)[-1] == b"Probably Stolen.exe":
            return True
    return False


# ---------------------------------------------------------------- scoring


def seeds(base, n):
    return [base + i for i in range(1, n + 1)]


def per_seed(runner, key, track, seed_list):
    vals = []
    for s in seed_list:
        es = [runner.get(key, d, s) for d in DIFFS[track]]
        if any(e is None for e in es):
            return None
        vals.append(sum(e["obj"] for e in es) / len(es))
    return vals


def mean(xs):
    return sum(xs) / len(xs) if xs else float("nan")


def ci(xs):
    n = len(xs)
    if n == 0:
        return float("nan"), float("nan")
    m = sum(xs) / n
    if n < 2:
        return m, float("nan")
    return m, 1.96 * math.sqrt(sum((x - m) ** 2 for x in xs) / (n - 1)) / math.sqrt(n)


def evaluated_n(runner, key, track, cap=10000):
    n = 0
    while n < cap and all((key, d, TRAIN0 + n + 1) in runner.cache for d in DIFFS[track]):
        n += 1
    return n


class Registry:
    """Every policy the study has seen: key -> {pol, args, track, gen, origin}."""

    def __init__(self, study):
        self.path = os.path.join(study, "candidates.jsonl")
        self.items = {}
        if os.path.exists(self.path):
            with open(self.path) as f:
                for line in f:
                    c = json.loads(line)
                    self.items[c["key"]] = c
        self.lock = threading.Lock()

    def add(self, key, args, pol, track, gen, origin):
        with self.lock:
            if key not in self.items:
                c = {"key": key, "args": args, "pol": pol, "track": track, "gen": gen, "origin": origin}
                self.items[key] = c
                with open(self.path, "a") as f:
                    f.write(json.dumps(c) + "\n")
        return key


def leaderboard(runner, reg, track, min_n):
    rows = []
    for key in list(reg.items):
        n = evaluated_n(runner, key, track)
        if n < min_n:
            continue
        m, h = ci(per_seed(runner, key, track, seeds(TRAIN0, n)))
        rows.append({"key": key, "n": n, "mean": m, "ci": h, "gen": reg.items[key]["gen"], "origin": reg.items[key]["origin"]})
    # mean - half the CI: a 48-seed result beats an equally good-looking lucky 32-seed one
    rows.sort(key=lambda r: -(r["mean"] - 0.5 * (r["ci"] if r["ci"] == r["ci"] else 1)))
    return rows


def best(runner, reg, track, k, min_n):
    return [reg.items[r["key"]] for r in leaderboard(runner, reg, track, min_n)[:k]]


def jobs_for(cands, diffs, seed_list):
    return [(c["key"], c["args"], d, s) for c in cands for d in diffs for s in seed_list]


# ---------------------------------------------------------------- search


def track_loop(study, space, runner, reg, track, until, stop):
    sp = os.path.join(study, f"state-{track}.json")
    st = json.load(open(sp)) if os.path.exists(sp) else {"gen": 0, "dist": space.init_dist()}
    train = seeds(TRAIN0, space.rungs[-1])
    while not stop.is_set() and time.time() < until:
        gen = st["gen"]
        rng = random.Random(f"{track}:{gen}")
        cands = {}
        if gen == 0:
            if space.init:
                k0 = space.key(space.init)
                cands[k0] = {"key": k0, "args": space.args(space.init), "pol": dict(space.init), "origin": "init"}
            for name, a in space.refs.items():
                cands.setdefault(" ".join(a), {"key": " ".join(a), "args": a, "pol": None, "origin": "ref " + name})
        for c in best(runner, reg, track, 2, space.rungs[-1]):
            cands.setdefault(c["key"], c)
        inc = best(runner, reg, track, 1, space.rungs[-1])
        inc = dict((inc[0]["pol"] or {}) if inc else space.init)
        while len(cands) < space.pop + ((len(space.refs) + bool(space.init)) if gen == 0 else 0) + 2:
            pol = space.sample(st["dist"], rng)
            if space.mutate:
                fresh, pol = pol, dict(inc)
                picked = set()
                k = rng.randint(*space.mutate)
                while len(picked) < k:
                    picked.add(rng.choices(space.names, [space.spec[n].get("weight", 1) for n in space.names])[0])
                for n in picked:
                    for _ in range(8):
                        if json.dumps(fresh[n]) != json.dumps(inc.get(n)):
                            break
                        fresh = space.sample(st["dist"], rng)
                    pol[n] = fresh[n]
            k = space.key(pol)
            cands.setdefault(k, {"key": k, "args": space.args(pol), "pol": pol, "origin": "sample"})
        for c in cands.values():
            reg.add(c["key"], c["args"], c["pol"], track, gen, c["origin"])
        alive = list(cands.values())
        t0 = time.time()
        elites = []
        for r, n in enumerate(space.rungs):
            runner.evaluate(jobs_for(alive, DIFFS[track], train[:n]))
            alive = sorted(alive, key=lambda c: -mean(per_seed(runner, c["key"], track, train[:n])))
            if r >= 1:
                elites += [c["pol"] for c in alive[: max(1, len(alive) // 2)] if c["pol"] is not None]
            if r < len(space.rungs) - 1:
                alive = alive[: max(2, len(alive) // 2)]
        space.update(st["dist"], elites)
        top = alive[0]
        m, h = ci(per_seed(runner, top["key"], track, train))
        log(study, f"[{track}] gen {gen}: {len(cands)} policies in {time.time() - t0:.0f}s; best {m:.3f}±{h:.3f} ({top['origin']}) {top['key']}")
        st["gen"] = gen + 1
        with open(sp + ".tmp", "w") as f:
            json.dump(st, f, indent=1)
        os.replace(sp + ".tmp", sp)
        if gen % 3 == 2:
            runner.evaluate(jobs_for(best(runner, reg, track, 3, space.rungs[-1]), DIFFS[track], seeds(TRAIN0, space.extend_n)))
        with open(os.path.join(study, f"leaderboard-{track}.json"), "w") as f:
            json.dump(leaderboard(runner, reg, track, space.rungs[-1])[:25], f, indent=1)


def check_meta(study, space):
    def h(p):
        try:
            with open(p, "rb") as f:
                return hashlib.sha256(f.read()).hexdigest()[:16]
        except OSError:
            return None
    meta = {"gym": h(GYM), "game": h(os.path.join(GAME_DIR, "Probably Stolen_Data", "Managed", "Assembly-CSharp.dll")), "space": h(space.path),
            "space_path": space.path, "horizon": space.horizon}
    mp = os.path.join(study, "meta.json")
    if os.path.exists(mp):
        old = json.load(open(mp))
        diff = [k for k in ("gym", "game", "horizon") if old.get(k) != meta[k]]
        if diff:
            sys.exit(f"{study}: made with other {', '.join(diff)}; start a new -study")
        return old
    meta["created"] = time.strftime("%Y-%m-%d %H:%M %Z")
    json.dump(meta, open(mp, "w"), indent=1)
    return meta


# ---------------------------------------------------------------- commands


def opts(argv, defaults):
    o = dict({"study": None, "space": "", "workers": "8"}, **defaults)
    it = iter(argv)
    for a in it:
        if not (a.startswith("-") and a[1:] in o):
            sys.exit(f"unknown argument {a}\n" + __doc__)
        v = next(it)
        o[a[1:]] = o[a[1:]] + [v] if isinstance(o[a[1:]], list) else v
    if not o["study"]:
        sys.exit(__doc__)
    os.makedirs(o["study"], exist_ok=True)
    mp = os.path.join(o["study"], "meta.json")
    path = o["space"] or (json.load(open(mp))["space_path"] if os.path.exists(mp) else DEFAULT_SPACE)
    space = Space(path)
    check_meta(o["study"], space)
    return o, space, Runner(o["study"], space, int(o["workers"])), Registry(o["study"])


def cmd_run(argv):
    o, space, runner, reg = opts(argv, {"tracks": "normal,hard", "until": ""})
    until = time.time() + 1e9
    if o["until"]:
        hh, mm = map(int, o["until"].split(":"))
        lt = time.localtime()
        t = time.mktime((lt.tm_year, lt.tm_mon, lt.tm_mday, hh, mm, 0, 0, 0, -1))
        until = t if t > time.time() else t + 86400
    stop = threading.Event()
    signal.signal(signal.SIGTERM, lambda *a: stop.set())
    log(o["study"], f"run: tracks {o['tracks']} until {time.strftime('%H:%M', time.localtime(until))}, {len(runner.cache)} cached episodes, space {space.path}")
    ths = [threading.Thread(target=track_loop, args=(o["study"], space, runner, reg, t, until, stop)) for t in o["tracks"].split(",")]
    for t in ths:
        t.start()
    last = time.time()
    while any(t.is_alive() for t in ths):
        time.sleep(15)
        if time.time() - last > 900:
            last = time.time()
            log(o["study"], f"episodes run {runner.n_run}, mean {runner.t_run / max(1, runner.n_run):.1f}s/episode, cache {len(runner.cache)}")
    log(o["study"], "run: done")


def cmd_extend(argv):
    o, space, runner, reg = opts(argv, {"tracks": "normal,hard", "top": "3", "seeds": "48"})
    for t in o["tracks"].split(","):
        runner.evaluate(jobs_for(best(runner, reg, t, int(o["top"]), space.rungs[-1]), DIFFS[t], seeds(TRAIN0, int(o["seeds"]))))
        for r in leaderboard(runner, reg, t, space.rungs[-1])[: int(o["top"])]:
            print(f"[{t}] {r['mean']:.3f}±{r['ci']:.3f} n={r['n']}  {r['key']}")


def wilson(k, n):
    if n == 0:
        return float("nan"), float("nan")
    z, p = 1.96, k / n
    d = 1 + z * z / n
    c = (p + z * z / (2 * n)) / d
    h = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n)) / d
    return max(0, c - h), min(1, c + h)


def median_ci(xs, rng):
    med = lambda v: (sorted(v)[(len(v) - 1) // 2] + sorted(v)[len(v) // 2]) / 2
    bs = sorted(med([rng.choice(xs) for _ in xs]) for _ in range(1000))
    return med(xs), bs[25], bs[974]


CURVE = (30, 60, 100, 200, 365, 500, 1000)


def nw_at(e, day, horizon):
    """Net worth on `day` (every NW_STEP days recorded): a win holds its last value, any other ending counts 0."""
    i = day // NW_STEP - 1
    if i < len(e["nw"]):
        return e["nw"][i]
    return e["nw"][-1] if e["nw"] and (e["win"] or (e["ending"] == "alive" and day <= horizon)) else 0


def describe(runner, key, diff, hs, horizon, ref=None):
    es = [runner.get(key, diff, s) for s in hs]
    rng = random.Random(7)
    md, lo, hi = median_ci([e["days"] for e in es], rng)
    nw_ = sum(e["win"] for e in es)
    wl, wh = wilson(nw_, len(es))
    wins = sorted(e["last_day"] for e in es if e["win"])
    row = {"obj": ci([e["obj"] for e in es]), "median_days": (md, lo, hi), "wins": (nw_, len(es), wl, wh),
           "win_day": wins[len(wins) // 2] if wins else None, "ends": collections.Counter(e["ending"] for e in es).most_common(),
           "nw": {d: mean([nw_at(e, d, horizon) for e in es]) for d in CURVE if d <= horizon},
           "permit": sum(1 for e in es if e.get("permit"))}
    if ref:
        row["d_obj"] = ci([e["obj"] - runner.get(ref, diff, s)["obj"] for e, s in zip(es, hs)])
    return row


def cmd_holdout(argv):
    o, space, runner, reg = opts(argv, {"tracks": "normal,hard", "top": "2", "seeds": "100", "refs": "", "extra": [], "min_n": "", "diffs": "normal,hard", "seed0": str(HOLD0)})
    hs = seeds(int(o["seed0"]), int(o["seeds"]))
    diffs = o["diffs"].split(",")
    min_n = int(o["min_n"] or space.extend_n)
    cands = collections.OrderedDict()
    for name in (o["refs"].split(",") if o["refs"] else list(space.refs)):
        a = space.refs[name]
        cands["ref " + name] = {"key": " ".join(a), "args": a}
    for spec in o["extra"]:
        name, _, key = spec.partition("=")
        cands[name] = reg.items[key]
    for t in o["tracks"].split(","):
        for i, c in enumerate(best(runner, reg, t, int(o["top"]), min_n)):
            if c["key"] not in [x["key"] for x in cands.values()]:
                cands[f"{t} #{i + 1}"] = c
    names = list(cands)
    runner.evaluate(jobs_for(cands.values(), diffs, hs))
    ref = cands[names[0]]["key"]
    L = [f"Held-out seeds {hs[0]}–{hs[-1]} (n={len(hs)}), horizon {space.horizon} days. Δobj is paired against {names[0]}.", ""]
    for name in names:
        L.append(f"- **{name}**: `{cands[name]['key']}`")
    for diff in diffs:
        L += ["", f"#### {diff}", "",
              "| Policy | Objective | Median days [95 % CI] | Wins [Wilson 95 %] | Median win day | Endings | Permit | Mean net worth d" + "/".join(map(str, [d for d in CURVE if d <= space.horizon])) + " | Δobj |",
              "|---|---|---|---|---|---|---|---|---|"]
        for name in names:
            r = describe(runner, cands[name]["key"], diff, hs, space.horizon, ref)
            md, lo, hi = r["median_days"]
            k, n, wl, wh = r["wins"]
            L.append(f"| {name} | {r['obj'][0]:.3f}±{r['obj'][1]:.3f} | {md:.0f} [{lo:.0f}, {hi:.0f}] | {k}/{n} [{100 * wl:.1f}–{100 * wh:.1f} %] | "
                     f"{r['win_day'] or '–'} | " + ", ".join(f"{e} {c}" for e, c in r["ends"]) + f" | {r['permit']} | "
                     + " / ".join(f"{v:.0f}" for v in r["nw"].values()) + f" | {r['d_obj'][0]:+.3f}±{r['d_obj'][1]:.3f} |")
    if len(diffs) < 2:
        names = []
    else:
        L += ["", "#### combined (mean of a seed's normal and hard objective)", "", "| Policy | Objective | Δobj |", "|---|---|---|"]
        base_c = per_seed(runner, ref, "combined", hs)
    for name in names:
        v = per_seed(runner, cands[name]["key"], "combined", hs)
        m, h = ci(v)
        dm, dh = ci([a - b for a, b in zip(v, base_c)])
        L.append(f"| {name} | {m:.3f}±{h:.3f} | {dm:+.3f}±{dh:.3f} |")
    text = "\n".join(L) + "\n"
    with open(os.path.join(o["study"], "holdout.md"), "w") as f:
        f.write(text)
    print(text)


def cmd_sensitivity(argv):
    o, space, runner, reg = opts(argv, {"track": "normal", "seeds": "16", "config": "best"})
    t = o["track"]
    c = best(runner, reg, t, 1, space.extend_n)[0] if o["config"] == "best" else reg.items[o["config"]]
    pol = c["pol"]
    if pol is None:
        sys.exit("sensitivity needs a sampled policy, not a ref")
    ss = seeds(SENS0, int(o["seeds"]))
    alts = [(lab, {"key": space.key(p), "args": space.args(p)}) for lab, p in space.neighbours(pol)]
    runner.evaluate(jobs_for([c] + [a for _, a in alts], DIFFS[t], ss))
    base = per_seed(runner, c["key"], t, ss)
    days = lambda k: [mean([runner.get(k, d, s)["days"] for d in DIFFS[t]]) for s in ss]
    wins = lambda k: sum(runner.get(k, d, s)["win"] for d in DIFFS[t] for s in ss)
    bd = days(c["key"])
    rows = []
    for lab, a in alts:
        v = per_seed(runner, a["key"], t, ss)
        dm, dh = ci([x - y for x, y in zip(v, base)])
        dd = ci([x - y for x, y in zip(days(a["key"]), bd)])
        rows.append((dm, dh, dd, wins(a["key"]), lab))
    rows.sort()
    L = [f"Sensitivity of `{c['key']}` on {t}, seeds {ss[0]}–{ss[-1]} (n={len(ss)}): objective {mean(base):.3f}, "
         f"wins {wins(c['key'])}/{len(ss) * len(DIFFS[t])}, mean days {mean(bd):.0f}.", "",
         "| Change | Δobjective | Δdays | wins |", "|---|---|---|---|"]
    L += [f"| {lab} | {dm:+.3f}±{dh:.3f} | {dd[0]:+.0f}±{dd[1]:.0f} | {w} |" for dm, dh, dd, w, lab in rows]
    text = "\n".join(L) + "\n"
    with open(os.path.join(o["study"], f"sensitivity-{t}.md"), "w") as f:
        f.write(text)
    print(text)


def cmd_report(argv):
    o, space, runner, reg = opts(argv, {"tracks": "normal,hard", "top": "10"})
    print(f"{len(runner.cache)} episodes cached, {len(reg.items)} policies, mean {mean([e.get('sec', 0) for e in runner.cache.values()]):.1f}s/episode")
    for t in o["tracks"].split(","):
        sp = os.path.join(o["study"], f"state-{t}.json")
        if not os.path.exists(sp):
            continue
        st = json.load(open(sp))
        print(f"\n[{t}] generation {st['gen']}")
        for r in leaderboard(runner, reg, t, space.rungs[-1])[: int(o["top"])]:
            es = [runner.get(r["key"], d, s) for d in DIFFS[t] for s in seeds(TRAIN0, r["n"])]
            ends = collections.Counter(e["ending"] for e in es).most_common(3)
            print(f"  {r['mean']:.3f}±{r['ci']:.3f} n={r['n']:3d} gen {r['gen']:3d} {r['origin']:12s} "
                  f"days {mean([e['days'] for e in es]):.0f} {ends}  {r['key']}")
        print("  distribution:")
        for n, s in space.spec.items():
            dn = st["dist"][n]
            gate = f" on {dn['on']:.2f}" if "on" in dn else ""
            if s["type"] == "cat":
                print(f"    {n:20s}{gate} " + " ".join(f"{json.dumps(v)}:{p:.2f}" for v, p in zip(s["values"], dn["p"])))
            elif s["type"] == "subset":
                print(f"    {n:20s}{gate} " + " ".join(f"{v}:{p:.2f}" for v, p in zip(s["pool"], dn["p"]) if p > 0.15))
            else:
                print(f"    {n:20s}{gate} mu {space.denorm(n, dn['mu'])} sd {dn['sd']:.2f}")


def cmd_export(argv):
    o, space, runner, reg = opts(argv, {"tracks": "normal,hard", "top": "2"})
    for t in o["tracks"].split(","):
        for i, c in enumerate(best(runner, reg, t, int(o["top"]), space.extend_n)):
            print(f"search_{t}_{i + 1} | " + shlex.join(runner.materialize(c["args"])))


if __name__ == "__main__":
    cmds = {"run": cmd_run, "extend": cmd_extend, "holdout": cmd_holdout, "sensitivity": cmd_sensitivity, "report": cmd_report, "export": cmd_export}
    if len(sys.argv) < 2 or sys.argv[1] not in cmds:
        sys.exit(__doc__)
    cmds[sys.argv[1]](sys.argv[2:])
