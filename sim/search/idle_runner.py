#!/usr/bin/env python3
"""Idle-CPU runner: continuous hard policy search plus a standing scoreboard (sim/search/README.md).

  idle_runner.py start     detach (setsid) and run in the background; pidfile var/idle/runner.pid
  idle_runner.py stop      SIGTERM the pid in the pidfile, wait for it to exit
  idle_runner.py status    phase, incumbent, last scoreboard lines, promotions
  idle_runner.py run       foreground (what `start` runs)

Every gym process and build runs under SCHED_IDLE at nice 19, at most WORKERS at a time, and only while
MemAvailable stays above MEM_FLOOR_GB. While "Probably Stolen.exe" runs nothing new is launched.

Scoreboard: whenever main's HEAD changes, the executor plays seeds SCORE_SEEDS on hard (the HARD policy: start,
perks and that commit's directives file) and on normal (defaults) and appends a line to var/scoreboard/log.jsonl.
Commits whose gym inputs (BUILD_PATHS) and hard policy are unchanged reuse the previous results: the gym is
deterministic per seed. A line is checked for regressions only against the last line with the same hard policy.

Search: rounds of local search over the build commit's sim/search/space-hard.json around the incumbent
(var/search/hard/incumbent.json, seeded from HARD; a change to HARD restarts it there). Round r uses fresh seeds
SEARCH0 + 1000 r + (1..24 screening, 501..560 confirmation). A candidate is promoted only on the 60
confirmation seeds, paired against the incumbent.
"""
import hashlib
import json
import math
import os
import random
import signal
import subprocess
import sys
import threading
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import search  # noqa: E402  (summarize, game_running)

REPO = search.REPO
VAR = os.path.join(REPO, "var")
IDLE = os.path.join(VAR, "idle")
PIDFILE = os.path.join(IDLE, "runner.pid")
LOGFILE = os.path.join(IDLE, "runner.log")
STATUS = os.path.join(IDLE, "status.json")
BUILDS = os.path.join(IDLE, "builds.json")
STUDY = os.path.join(VAR, "search", "hard")
BOARD = os.path.join(VAR, "scoreboard")
WT_ROOT = os.path.expanduser("~/.cache/fence-idle")

WORKERS = int(os.environ.get("FENCE_IDLE_WORKERS", "28"))
MEM_FLOOR_GB = 8.0
GB_PER_GYM = 1.0
LAUNCH_RAMP_SEC = 20
HORIZON = 1000
EPISODE_TIMEOUT = 3600
# Reserved seed ranges: the scoreboard's 60 fixed seeds and one 1000-seed block per search round.
SCORE_SEEDS = list(range(990001, 990061))
SEARCH0 = 3_000_000
SCREEN = [(1, 12), (13, 24)]
CONFIRM = (501, 560)
N_CANDIDATES = 14
FINALISTS = 3
BUILD_PATHS = ["sim/gym", "sim/headless", "mod/src", "compat"]
# The hard incumbent: start, perks and a directives file read from the commit being served. The search's start
# is space-hard.json's `args`; keep the two equal.
HARD = {"start": "apprentice", "perks": "vending+short_sleeper", "directives": "sim/lab/scripts/directives/hard_income_cash_sprint.json"}
SPACE = "sim/search/space-hard.json"
NORMAL_ARGS = ["-human", "player"]

stop = threading.Event()


def log(msg):
    print(time.strftime("%Y-%m-%d %H:%M:%S ") + msg, flush=True)


def write_json(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = f"{path}.{os.getpid()}.tmp"
    with open(tmp, "w") as f:
        json.dump(obj, f, indent=1, sort_keys=True)
        f.write("\n")
    os.replace(tmp, path)


def read_json(path, default=None):
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, ValueError):
        return default


def append_jsonl(path, obj):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "a") as f:
        f.write(json.dumps(obj, separators=(",", ":"), sort_keys=True) + "\n")


def set_status(**kw):
    st = read_json(STATUS, {})
    st.update(kw, updated=time.strftime("%Y-%m-%d %H:%M:%S"), pid=os.getpid())
    write_json(STATUS, st)


def git(*args, cwd=REPO):
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True).stdout.strip()


def idle_cmd(cmd):
    return ["chrt", "-i", "0", "nice", "-n", "19", *cmd]


def mem_available_gb():
    with open("/proc/meminfo") as f:
        for line in f:
            if line.startswith("MemAvailable:"):
                return int(line.split()[1]) / 1048576
    return 0.0


def sleep(sec):
    stop.wait(sec)


def wait_until_free():
    """Blocks while the owner is playing; returns False on stop."""
    announced = False
    while not stop.is_set() and search.game_running():
        if not announced:
            log("Probably Stolen.exe is running: paused")
            set_status(paused=True)
            announced = True
        sleep(15)
    if announced:
        log("game gone: resuming")
        set_status(paused=False)
    return not stop.is_set()


# ---------------------------------------------------------------- builds


def build_key(commit):
    return hashlib.sha1(" ".join(git("rev-parse", *[f"{commit}:{p}" for p in BUILD_PATHS]).split()).encode()).hexdigest()[:16]


def ensure_build(commit):
    """The gym dll for `commit`, built from a clean detached worktree into var/sim/gyms/search-<commit>.
    Builds with the same gym inputs are shared. Returns {dll, key, commit, dir} or None on stop."""
    key = build_key(commit)
    builds = read_json(BUILDS, {})
    b = builds.get(key)
    if b and (b.get("failed") or os.path.exists(b["dll"])):
        return b
    if not wait_until_free():
        return None
    short = commit[:12]
    out = os.path.join(VAR, "sim", "gyms", f"search-{short}")
    wt = os.path.join(WT_ROOT, f"wt-{short}")
    log(f"build {short} (key {key}) -> {out}")
    set_status(phase=f"building {short}")
    os.makedirs(WT_ROOT, exist_ok=True)
    if os.path.exists(wt):
        subprocess.run(["git", "worktree", "remove", "--force", wt], cwd=REPO, capture_output=True)
    git("worktree", "add", "--detach", wt, commit)
    try:
        tmp = out + ".tmp"
        subprocess.run(["rm", "-rf", tmp], check=True)
        p = subprocess.run(idle_cmd(["dotnet", "build", os.path.join(wt, "sim", "gym"), "-c", "Release", "-o", tmp, "-nologo", "-v", "q"]),
                           cwd=wt, capture_output=True, text=True)
        if p.returncode != 0:
            log(f"build {short} failed:\n{p.stdout[-2000:]}{p.stderr[-1000:]}")
            builds[key] = {"failed": True, "commit": commit, "key": key}
            write_json(BUILDS, builds)
            return builds[key]
        if os.path.exists(out):
            subprocess.run(["rm", "-rf", out], check=True)
        os.replace(tmp, out)
    finally:
        subprocess.run(["git", "worktree", "remove", "--force", wt], cwd=REPO, capture_output=True)
    b = {"dll": os.path.join(out, "fence-gym.dll"), "key": key, "commit": commit, "dir": out, "built": time.strftime("%Y-%m-%d %H:%M:%S")}
    # One short episode first: it creates the headless game copies under var/gamedata before 28 processes race to.
    warm = run_episode(b, ["-human", "player"], "normal", 1, days=2)
    if warm is None:
        return None
    if warm["ending"] == "CRASH":
        log(f"build {short}: warm-up crashed: {warm.get('crash')}")
        builds[key] = {"failed": True, "commit": commit, "key": key}
        write_json(BUILDS, builds)
        return builds[key]
    builds[key] = b
    write_json(BUILDS, builds)
    log(f"build {short} ready")
    return b


def at_commit(commit, rel):
    """A committed file's text, or None. Space and policy files live outside BUILD_PATHS, so they are read
    from the commit being served rather than from the build's."""
    p = subprocess.run(["git", "show", f"{commit}:{rel}"], cwd=REPO, capture_output=True, text=True)
    return p.stdout if p.returncode == 0 else None


# ---------------------------------------------------------------- episodes


game_on = threading.Event()


def watch_game():
    while not stop.is_set():
        on = search.game_running()
        if on != game_on.is_set():
            log("Probably Stolen.exe is running: no new episodes" if on else "game gone: resuming")
            set_status(paused=on)
            game_on.set() if on else game_on.clear()
        stop.wait(10)


class Pool:
    def __init__(self, workers):
        self.workers = workers
        self.cv = threading.Condition()
        self.active = 0
        self.children = {}
        self.launches = []

    def acquire(self):
        with self.cv:
            while True:
                if stop.is_set():
                    return False
                now = time.time()
                self.launches = [t for t in self.launches if now - t < LAUNCH_RAMP_SEC]
                # a gym grows to ~1 GB over its first seconds, so recent launches count against the floor
                if (self.active < self.workers and not game_on.is_set()
                        and mem_available_gb() - GB_PER_GYM * len(self.launches) >= MEM_FLOOR_GB):
                    self.active += 1
                    self.launches.append(now)
                    return True
                self.cv.wait(5)

    def release(self):
        with self.cv:
            self.active -= 1
            self.cv.notify_all()

    def kill_all(self):
        for pid in list(self.children):
            try:
                os.killpg(pid, signal.SIGTERM)
            except OSError:
                pass


pool = Pool(WORKERS)
cache_lock = threading.Lock()


def run_episode(b, args, diff, seed, days=HORIZON):
    """One gym episode; None when stopped before it finished."""
    if not pool.acquire():
        return None
    tmpdir = os.path.join(IDLE, "tmp")
    os.makedirs(tmpdir, exist_ok=True)
    summ = os.path.join(tmpdir, f"{threading.get_ident()}-{seed}-{diff}.json")
    cmd = idle_cmd(["dotnet", b["dll"], "run", "-seed", str(seed), "-days", str(days), "-difficulty", diff,
                    "-no-trajectory", "-summary", summ, "-out", summ + "l", *args])
    t0 = time.time()
    try:
        p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=REPO, start_new_session=True)
        pool.children[p.pid] = True
        try:
            out, _ = p.communicate(timeout=EPISODE_TIMEOUT)
        except subprocess.TimeoutExpired:
            os.killpg(p.pid, signal.SIGKILL)
            out, _ = p.communicate()
        pool.children.pop(p.pid, None)
        if stop.is_set() and p.returncode != 0:
            return None
        j = read_json(summ, {})
        for f in (summ, summ + "l"):
            try:
                os.remove(f)
            except OSError:
                pass
        if p.returncode != 0 or not j:
            j = {"crash": f"exit {p.returncode}: {out.decode(errors='replace')[-300:]}"}
        e = search.summarize(seed, diff, days, j)
        e["sec"] = round(time.time() - t0, 1)
        return e
    finally:
        pool.release()


def survival(e):
    """Days survived, a win or a run alive at the horizon counting as the full horizon."""
    return HORIZON if e["win"] or e["ending"] == "alive" else e["days"]


def score(e):
    """Survival days plus, for a win, the days it saved before the horizon: an earlier win scores higher."""
    return survival(e) + (HORIZON - e["days"] if e["win"] else 0)


def run_many(b, jobs, cache_path):
    """jobs: [(key, args, diff, seed)] -> {(key, diff, seed): episode}; cached in cache_path (per build)."""
    cache = {}
    if os.path.exists(cache_path):
        with open(cache_path) as f:
            for line in f:
                try:
                    e = json.loads(line)
                except ValueError:
                    continue
                if e.get("build") == b["key"]:
                    cache[(e["key"], e["diff"], e["seed"])] = e
    todo = [j for j in jobs if (j[0], j[2], j[3]) not in cache]
    results = dict(cache)

    def one(key, args, diff, seed):
        e = run_episode(b, args, diff, seed)
        if e is None:
            return
        e.update(key=key, build=b["key"])
        with cache_lock:
            results[(key, diff, seed)] = e
            append_jsonl(cache_path, e)

    threads = []
    for j in todo:
        if stop.is_set():
            break
        t = threading.Thread(target=one, args=j, daemon=True)
        t.start()
        threads.append(t)
        while sum(t.is_alive() for t in threads) >= pool.workers and not stop.is_set():
            time.sleep(0.5)
    for t in threads:
        while t.is_alive():
            t.join(1)
    if stop.is_set():
        return None
    return {(k, d, s): results[(k, d, s)] for k, _, d, s in jobs if (k, d, s) in results}


# ---------------------------------------------------------------- stats


def mean_se(xs):
    n = len(xs)
    if n == 0:
        return float("nan"), float("nan")
    m = sum(xs) / n
    if n < 2:
        return m, float("nan")
    return m, math.sqrt(sum((x - m) ** 2 for x in xs) / (n - 1) / n)


def median(xs):
    s = sorted(xs)
    return (s[(len(s) - 1) // 2] + s[len(s) // 2]) / 2 if s else float("nan")


def boot_se_median_diff(a, b, n=1000):
    """Bootstrap stderr of median(a) - median(b) over paired seeds."""
    rng = random.Random(7)
    idx = range(len(a))
    ds = []
    for _ in range(n):
        pick = [rng.choice(idx) for _ in idx]
        ds.append(median([a[i] for i in pick]) - median([b[i] for i in pick]))
    m = sum(ds) / n
    return math.sqrt(sum((d - m) ** 2 for d in ds) / (n - 1))


def endings(es):
    c = {}
    for e in es:
        c[e["ending"]] = c.get(e["ending"], 0) + 1
    return dict(sorted(c.items(), key=lambda kv: -kv[1]))


# ---------------------------------------------------------------- scoreboard


def scoreboard(commit, b):
    log_path = os.path.join(BOARD, "log.jsonl")
    lines = []
    if os.path.exists(log_path):
        with open(log_path) as f:
            lines = [json.loads(x) for x in f if x.strip()]
    if any(x["commit"] == commit[:12] for x in lines):
        return True
    policy = at_commit(commit, HARD["directives"]) or "{}\n"
    dsha = hashlib.sha1(policy.encode()).hexdigest()[:12]
    pdir = os.path.join(BOARD, "policies")
    os.makedirs(pdir, exist_ok=True)
    ppath = os.path.join(pdir, f"{dsha}.json")
    if not os.path.exists(ppath):
        with open(ppath, "w") as f:
            f.write(policy)
    spec = {"start": HARD["start"], "perks": HARD["perks"], "directives": HARD["directives"], "directives_sha": dsha}
    psha = hashlib.sha1(json.dumps(spec, sort_keys=True).encode()).hexdigest()[:12]
    prev = next((x for x in reversed(lines) if "hard" in x and x.get("hard_policy") == psha), None)
    prev_normal = next((x for x in reversed(lines) if "normal" in x), None)
    subject = git("log", "-1", "--format=%s", commit)
    entry = {"time": time.strftime("%Y-%m-%d %H:%M:%S"), "commit": commit[:12], "subject": subject, "build": b["key"],
             "hard_policy": psha, "hard_policy_spec": spec, "seeds": f"{SCORE_SEEDS[0]}-{SCORE_SEEDS[-1]}"}
    reuse = next((x for x in reversed(lines) if x.get("build") == b["key"] and x.get("hard_policy") == psha and "hard" in x), None)
    if reuse:
        entry.update(hard=reuse["hard"], normal=reuse["normal"], same_as=reuse["commit"])
        per_seed = read_json(os.path.join(BOARD, "runs", f"{reuse['commit']}.json"))
    else:
        set_status(phase=f"scoreboard {commit[:12]}")
        log(f"scoreboard {commit[:12]}: hard + normal on seeds {entry['seeds']}")
        hard_args = ["-human", "player", "-start", HARD["start"]] + (["-perks", spec["perks"]] if HARD["perks"] else []) + ["-directives", ppath]
        jobs = [(f"hard:{psha}", hard_args, "hard", s) for s in SCORE_SEEDS]
        jobs += [("normal", NORMAL_ARGS, "normal", s) for s in SCORE_SEEDS]
        res = run_many(b, jobs, os.path.join(BOARD, "evals.jsonl"))
        if res is None:
            return False
        per_seed = {}
        for track in ("hard", "normal"):
            es = [res[(f"hard:{psha}" if track == "hard" else track, track, s)] for s in SCORE_SEEDS]
            per_seed[track] = {str(e["seed"]): {"days": e["days"], "survival": survival(e), "win": e["win"], "ending": e["ending"]} for e in es}
            m, se = mean_se([survival(e) for e in es])
            entry[track] = {"wins": sum(e["win"] for e in es), "median_days": median([e["days"] for e in es]),
                            "mean_survival": round(m, 1), "se_survival": round(se, 1), "endings": endings(es),
                            "crashes": sum(e["ending"] == "CRASH" for e in es)}
    write_json(os.path.join(BOARD, "runs", f"{commit[:12]}.json"), per_seed)
    flags = []
    old = read_json(os.path.join(BOARD, "runs", f"{prev['commit']}.json")) if prev and per_seed else None
    if old:
        a = [score(per_seed["hard"][str(s)]) for s in SCORE_SEEDS]
        o = [score(old["hard"][str(s)]) for s in SCORE_SEEDS]
        drop = median(o) - median(a)
        se = boot_se_median_diff(a, o)
        entry["vs"] = {"commit": prev["commit"], "hard_median_drop": drop, "hard_median_se": round(se, 1), "hard_wins_drop": prev["hard"]["wins"] - entry["hard"]["wins"]}
        if drop > 2 * se and drop > 0:
            flags.append(f"hard median score {median(o):.0f} -> {median(a):.0f} (drop {drop:.0f} > 2 x {se:.1f})")
    elif per_seed:
        entry["vs"] = {"hard": "no earlier line with this hard policy"}
    if prev_normal and per_seed:
        entry.setdefault("vs", {})["normal_commit"] = prev_normal["commit"]
        entry["vs"]["normal_wins_drop"] = prev_normal["normal"]["wins"] - entry["normal"]["wins"]
        if entry["vs"]["normal_wins_drop"] >= 4:
            flags.append(f"normal wins {prev_normal['normal']['wins']} -> {entry['normal']['wins']}")
    entry["regression"] = flags
    append_jsonl(log_path, entry)
    log(f"scoreboard {commit[:12]}: hard {entry['hard']['wins']}/{len(SCORE_SEEDS)} median {entry['hard']['median_days']}, "
        f"normal {entry['normal']['wins']}/{len(SCORE_SEEDS)} median {entry['normal']['median_days']}" + (f"  REGRESSION: {'; '.join(flags)}" if flags else ""))
    return True


# ---------------------------------------------------------------- search


def get_path(d, path):
    for p in path.split("."):
        if not isinstance(d, dict) or p not in d:
            return None
        d = d[p]
    return d


def set_path(d, path, v):
    """Sets a dotted path; None removes it and any parent it leaves empty."""
    head, _, rest = path.partition(".")
    if not rest:
        if v is None:
            d.pop(head, None)
        else:
            d[head] = v
        return
    child = d.get(head) if isinstance(d.get(head), dict) else {}
    set_path(child, rest, v)
    if child:
        d[head] = child
    else:
        d.pop(head, None)


class Pol:
    """A policy: a directives dict plus gym-flag levers."""

    def __init__(self, dirs, flags=None):
        self.dirs, self.flags = json.loads(json.dumps(dirs)), dict(flags or {})

    def lever(self, name, spec):
        return get_path(self.dirs, spec["dir"]) if "dir" in spec else self.flags.get(name)

    def put(self, name, spec, v):
        if "dir" in spec:
            set_path(self.dirs, spec["dir"], v)
        elif v is None:
            self.flags.pop(name, None)
        else:
            self.flags[name] = v

    def key(self):
        return json.dumps({"dirs": self.dirs, "flags": self.flags}, sort_keys=True, separators=(",", ":"))

    def to_json(self):
        return {"directives": self.dirs, "flags": self.flags}


def norm(s, v):
    if s.get("log"):
        return (math.log(v) - math.log(s["lo"])) / (math.log(s["hi"]) - math.log(s["lo"]))
    return (v - s["lo"]) / (s["hi"] - s["lo"])


def denorm(s, u):
    u = min(1.0, max(0.0, u))
    x = math.exp(math.log(s["lo"]) + u * (math.log(s["hi"]) - math.log(s["lo"]))) if s.get("log") else s["lo"] + u * (s["hi"] - s["lo"])
    return int(round(x)) if s["type"] == "int" else round(x, s.get("digits", 1))


def redraw(s, cur, rng):
    """A new value for one lever, different from cur when possible."""
    t = s["type"]
    for _ in range(12):
        if s.get("optional") and cur is not None and rng.random() < 0.2:
            v = None
        elif t == "cat":
            v = rng.choices(s["values"], s.get("prior", [1] * len(s["values"])))[0]
        elif t in ("int", "float"):
            u = norm(s, cur) + rng.gauss(0, 0.2) if isinstance(cur, (int, float)) and not isinstance(cur, bool) else rng.random()
            v = denorm(s, u)
        elif t == "subset":
            have = list(cur or [])
            item = rng.choice(s["pool"])
            have = [x for x in have if x != item] if item in have else have + [item]
            v = [x for x in s["pool"] if x in have]
        else:
            return cur
        if json.dumps(v) != json.dumps(cur):
            return v
    return cur


def mutate(inc, spec, rng, lo=1, hi=3):
    """Redraw 1-3 levers of the incumbent, weighted by the space's `weight`."""
    pol = Pol(inc.dirs, inc.flags)
    names = [n for n, s in spec.items() if "dir" in s or "flag" in s]
    k = rng.randint(lo, hi)
    picked = set()
    for _ in range(50):
        if len(picked) >= k:
            break
        n = rng.choices(names, [spec[x].get("weight", 1) for x in names])[0]
        w = spec[n].get("when")
        if w and not all(json.dumps(pol.lever(c, spec[c]) if c in spec else None) in [json.dumps(x) for x in vals] for c, vals in w.items()):
            continue
        picked.add(n)
        pol.put(n, spec[n], redraw(spec[n], pol.lever(n, spec[n]), rng))
    return pol, sorted(picked)


def pol_args(space, pol, dirs_dir):
    js = json.dumps(pol.dirs, sort_keys=True, separators=(",", ":"))
    path = os.path.join(dirs_dir, hashlib.sha1(js.encode()).hexdigest()[:16] + ".json")
    if not os.path.exists(path):
        with open(path + ".tmp", "w") as f:
            f.write(js + "\n")
        os.replace(path + ".tmp", path)
    args = ["-human", space.get("human", "player"), *space.get("args", [])]
    if "-start" not in args:
        args += ["-start", HARD["start"]]
    for n in sorted(pol.flags):
        s = space["knobs"].get(n, {})
        v = pol.flags[n]
        # An incumbent's perks still apply when the space being served has no perks lever.
        flag = s.get("flag") or {"perks": "-perks"}.get(n)
        if v is None or not flag:
            continue
        v = "+".join(v) if isinstance(v, list) else str(v).lower() if isinstance(v, bool) else str(v)
        args += [flag, v or s.get("empty", "none")]
    return args + ["-directives", path]


def load_incumbent(commit):
    """The search incumbent; reseeded from HARD when HARD changes (promotions keep the base they grew from)."""
    base = json.dumps(HARD, sort_keys=True)
    inc = read_json(os.path.join(STUDY, "incumbent.json"))
    if inc and inc.get("base") == base:
        return inc
    if inc:
        append_jsonl(os.path.join(STUDY, "incumbents.jsonl"), {**inc, "retired": time.strftime("%Y-%m-%d %H:%M:%S")})
    dirs = json.loads(at_commit(commit, HARD["directives"]) or "{}")
    flags = {"perks": HARD["perks"]} if HARD["perks"] else {}
    inc = {"directives": dirs, "flags": flags, "base": base, "source": f"{HARD['directives']} at {commit[:12]}",
           "since": time.strftime("%Y-%m-%d %H:%M:%S"), "round": None}
    log(f"incumbent reseeded from {HARD}")
    write_json(os.path.join(STUDY, "incumbent.json"), inc)
    return inc


def search_round(b, commit):
    state = read_json(os.path.join(STUDY, "state.json"), {"round": 0})
    r = state["round"]
    text = at_commit(commit, SPACE)
    space = json.loads(text) if text else None
    if not space:
        log(f"no {SPACE} in {commit[:12]}: search idle")
        sleep(600)
        return
    spec = space["knobs"]
    inc_j = load_incumbent(commit)
    inc = Pol(inc_j["directives"], inc_j.get("flags"))
    base = SEARCH0 + 1000 * r
    rpath = os.path.join(STUDY, "rounds", f"round-{r:04d}.json")
    rd = read_json(rpath)
    if not rd or rd.get("build") != b["key"] or rd.get("incumbent") != inc.key():
        rng = random.Random(f"round:{r}:{b['key']}")
        cands, seen = [], {inc.key()}
        for _ in range(N_CANDIDATES * 10):
            if len(cands) >= N_CANDIDATES:
                break
            p, levers = mutate(inc, spec, rng)
            if p.key() not in seen:
                seen.add(p.key())
                cands.append({"policy": p.to_json(), "levers": levers})
        rd = {"round": r, "build": b["key"], "commit": commit[:12], "seed_block": base, "incumbent": inc.key(),
              "space_levers": sorted(spec), "candidates": cands, "started": time.strftime("%Y-%m-%d %H:%M:%S")}
        write_json(rpath, rd)
    dirs_dir = os.path.join(STUDY, "dirs")
    os.makedirs(dirs_dir, exist_ok=True)
    pols = {"incumbent": inc} | {f"c{i:02d}": Pol(c["policy"]["directives"], c["policy"]["flags"]) for i, c in enumerate(rd["candidates"])}
    args = {k: pol_args(space, p, dirs_dir) for k, p in pols.items()}
    cache = os.path.join(STUDY, "evals.jsonl")
    log(f"round {r} (build {commit[:12]}, seeds {base}+): {len(rd['candidates'])} candidates, levers "
        + ", ".join(sorted({x for c in rd["candidates"] for x in c["levers"]})))
    set_status(phase=f"search round {r}", round=r)

    def keyof(k):
        return pols[k].key()

    alive = list(pols)
    screened = []
    for lo, hi in SCREEN:
        seeds = [base + i for i in range(1, hi + 1)]
        res = run_many(b, [(keyof(k), args[k], "hard", s) for k in alive for s in seeds], cache)
        if res is None:
            return
        sc = {k: sum(score(res[(keyof(k), "hard", s)]) for s in seeds) / len(seeds) for k in alive}
        alive = sorted(alive, key=lambda k: -sc[k])
        screened.append({k: round(sc[k], 1) for k in alive})
        if (lo, hi) != SCREEN[-1]:
            cands = [k for k in alive if k != "incumbent"]
            alive = cands[: max(2, len(cands) // 2)] + ["incumbent"]
    finalists = [k for k in alive if k != "incumbent" and sc[k] > sc["incumbent"]][:FINALISTS]
    out = {"round": r, "commit": commit[:12], "build": b["key"], "seed_block": base, "screen": screened, "finalists": {}}
    if finalists:
        cseeds = [base + i for i in range(CONFIRM[0], CONFIRM[1] + 1)]
        res = run_many(b, [(keyof(k), args[k], "hard", s) for k in finalists + ["incumbent"] for s in cseeds], cache)
        if res is None:
            return
        ie = [res[(keyof("incumbent"), "hard", s)] for s in cseeds]
        out["incumbent"] = {"wins": sum(e["win"] for e in ie), "median_days": median([e["days"] for e in ie]),
                            "mean_survival": round(mean_se([survival(e) for e in ie])[0], 1), "endings": endings(ie)}
        best = None
        for k in finalists:
            ce = [res[(keyof(k), "hard", s)] for s in cseeds]
            dd, sd = mean_se([score(c) - score(i) for c, i in zip(ce, ie)])
            dw, sw = mean_se([c["win"] - i["win"] for c, i in zip(ce, ie)])
            wins_c, wins_i = sum(e["win"] for e in ce), out["incumbent"]["wins"]
            ok = (dw > 2 * sw) or (wins_c >= wins_i and dd > 0 and dd > 2 * sd)
            row = {"levers": rd["candidates"][int(k[1:])]["levers"], "policy": pols[k].to_json(), "wins": wins_c,
                   "median_days": median([e["days"] for e in ce]), "d_survival": round(dd, 1), "se": round(sd, 1),
                   "endings": endings(ce), "promote": ok}
            out["finalists"][k] = row
            if ok and (best is None or dd - 2 * sd > best[1]["d_survival"] - 2 * best[1]["se"]):
                best = (k, row)
        if best:
            k, row = best
            promo = {"time": time.strftime("%Y-%m-%d %H:%M:%S"), "round": r, "commit": commit[:12], "build": b["key"],
                     "seeds": f"{cseeds[0]}-{cseeds[-1]}", "levers": row["levers"], "policy": row["policy"],
                     "previous": inc.to_json(), "wins": row["wins"], "wins_incumbent": out["incumbent"]["wins"],
                     "median_days": row["median_days"], "median_days_incumbent": out["incumbent"]["median_days"],
                     "d_survival": row["d_survival"], "se": row["se"], "endings": row["endings"]}
            append_jsonl(os.path.join(STUDY, "promotions.jsonl"), promo)
            pdir = os.path.join(STUDY, "promoted")
            os.makedirs(pdir, exist_ok=True)
            with open(os.path.join(pdir, f"round-{r:04d}.json"), "w") as f:
                f.write(json.dumps(row["policy"]["directives"], sort_keys=True, separators=(",", ":")) + "\n")
            write_json(os.path.join(STUDY, "incumbent.json"), {**row["policy"], "base": inc_j.get("base"), "source": f"round {r}", "since": promo["time"],
                                                              "round": r, "commit": commit[:12], "evidence": promo})
            out["promoted"] = k
            log(f"round {r}: PROMOTED {k} ({', '.join(row['levers'])}): {row['wins']} wins, median {row['median_days']} vs "
                f"{out['incumbent']['median_days']}, paired +{row['d_survival']}±{row['se']} days")
    if "promoted" not in out:
        log(f"round {r}: no promotion (finalists {', '.join(f'{k} {v['d_survival']:+}±{v['se']}' for k, v in out['finalists'].items()) or 'none beat the incumbent on screening'})")
    out["finished"] = time.strftime("%Y-%m-%d %H:%M:%S")
    append_jsonl(os.path.join(STUDY, "rounds.jsonl"), out)
    state["round"] = r + 1
    write_json(os.path.join(STUDY, "state.json"), state)


# ---------------------------------------------------------------- main loop


def main_head():
    return git("rev-parse", "refs/heads/main")


def run():
    os.makedirs(IDLE, exist_ok=True)
    old = read_pid()
    if old and old != os.getpid() and alive(old):
        sys.exit(f"already running: pid {old}")
    with open(PIDFILE, "w") as f:
        f.write(f"{os.getpid()}\n")

    def on_term(*_):
        log("SIGTERM: stopping")
        stop.set()
        pool.kill_all()
        with pool.cv:
            pool.cv.notify_all()

    signal.signal(signal.SIGTERM, on_term)
    signal.signal(signal.SIGINT, on_term)
    threading.Thread(target=watch_game, daemon=True).start()
    log(f"start: pid {os.getpid()}, {WORKERS} workers, SCHED_IDLE nice 19")
    set_status(phase="starting", paused=False)
    try:
        current = None
        while not stop.is_set():
            head = main_head()
            b = ensure_build(head)
            if b is None:
                break
            if b.get("failed"):
                if current is None:
                    sleep(300)
                    continue
            else:
                if not scoreboard(head, b):
                    break
                current = b
            if not wait_until_free():
                break
            search_round(current, current["commit"] if b.get("failed") else head)
    finally:
        set_status(phase="stopped")
        try:
            if read_pid() == os.getpid():
                os.remove(PIDFILE)
        except OSError:
            pass
        log("stopped")


def read_pid():
    try:
        return int(open(PIDFILE).read().strip())
    except (OSError, ValueError):
        return None


def alive(pid):
    try:
        with open(f"/proc/{pid}/cmdline", "rb") as f:
            return b"idle_runner.py" in f.read()
    except OSError:
        return False


def cmd_start():
    pid = read_pid()
    if pid and alive(pid):
        sys.exit(f"already running: pid {pid}")
    os.makedirs(IDLE, exist_ok=True)
    with open(LOGFILE, "a") as logf:
        p = subprocess.Popen([sys.executable, os.path.abspath(__file__), "run"], stdout=logf, stderr=subprocess.STDOUT,
                             stdin=subprocess.DEVNULL, cwd=REPO, start_new_session=True)
    print(f"started pid {p.pid}; log {LOGFILE}")


def cmd_stop():
    pid = read_pid()
    if not pid or not alive(pid):
        sys.exit("not running")
    os.kill(pid, signal.SIGTERM)
    for _ in range(120):
        if not alive(pid):
            print(f"stopped pid {pid}")
            return
        time.sleep(1)
    sys.exit(f"pid {pid} still running after 120 s")


def cmd_status():
    pid = read_pid()
    print(f"runner: {'running, pid ' + str(pid) if pid and alive(pid) else 'not running'}")
    st = read_json(STATUS, {})
    print(f"status: {json.dumps(st, sort_keys=True)}")
    inc = read_json(os.path.join(STUDY, "incumbent.json"), {})
    print(f"incumbent ({inc.get('source')}, since {inc.get('since')}): flags {json.dumps(inc.get('flags'), sort_keys=True)} "
          f"directives {json.dumps(inc.get('directives'), sort_keys=True)}")
    for name, path, n in (("scoreboard", os.path.join(BOARD, "log.jsonl"), 3), ("rounds", os.path.join(STUDY, "rounds.jsonl"), 3),
                          ("promotions", os.path.join(STUDY, "promotions.jsonl"), 10)):
        if os.path.exists(path):
            with open(path) as f:
                lines = f.readlines()
            print(f"{name} ({len(lines)} total, last {min(n, len(lines))}):")
            for line in lines[-n:]:
                j = json.loads(line)
                if name == "scoreboard":
                    spec = j.get("hard_policy_spec") or {}
                    print(f"  {j['commit']} hard [{j['hard_policy']} {spec.get('perks', '')} {os.path.basename(spec.get('directives', ''))}] "
                          f"{j['hard']['wins']}/{sum(j['hard']['endings'].values())} median {j['hard']['median_days']} | "
                          f"normal {j['normal']['wins']}/{sum(j['normal']['endings'].values())} "
                          f"median {j['normal']['median_days']}" + (f" (same as {j['same_as']})" if j.get("same_as") else "")
                          + (f"  REGRESSION {j['regression']}" if j.get("regression") else ""))
                elif name == "rounds":
                    print(f"  round {j['round']} {j['commit']}: " + (f"promoted {j['promoted']}" if j.get("promoted") else "no promotion")
                          + "".join(f"; {k} {v['d_survival']:+}±{v['se']}" for k, v in j["finalists"].items()))
                else:
                    print(f"  round {j['round']} {j['time']}: {j['levers']} wins {j['wins']} vs {j['wins_incumbent']}, "
                          f"median {j['median_days']} vs {j['median_days_incumbent']}, +{j['d_survival']}±{j['se']}")


if __name__ == "__main__":
    cmds = {"start": cmd_start, "stop": cmd_stop, "status": cmd_status, "run": run}
    if len(sys.argv) != 2 or sys.argv[1] not in cmds:
        sys.exit(__doc__)
    cmds[sys.argv[1]]()
