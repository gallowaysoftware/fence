#!/usr/bin/env python3
"""Paired report of a hybrid league (docs/research/HYBRID.md).

  hybrid_report.py var/league/<name> [baseline=executor]

Per player: wins, median and mean days, endings, net worth on day 100, LLM calls and wall time per
game; paired against the baseline on the seeds both finished: Δ days and Δ net worth on day
100 (mean ± 1.96 stderr; a run over by then counts its last day) and wins gained / lost. Then, per
hybrid player, what its decisions played: the share of decisions that left the plan in force, by
candidate source and trigger, how often a proposal outscored `keep` in the rollouts (at all, and by
the switch gate), and the rollout gain of the played candidate over `keep`.
"""
import collections
import glob
import json
import math
import os
import re
import statistics
import sys


def load(d):
    res = {}
    for f in glob.glob(os.path.join(d, "*", "result.json")):
        r = json.load(open(f))
        res.setdefault(r["player"], {})[r["seed"]] = r
    return res


def mean_se(xs):
    if not xs:
        return float("nan"), float("nan")
    m = statistics.mean(xs)
    return m, (statistics.stdev(xs) / math.sqrt(len(xs)) if len(xs) > 1 else float("nan"))


def nw_on(r, day):
    nw = r.get("net_worth_by_day") or [0]
    return nw[min(day, len(nw)) - 1]


def signed(d, player, seed):
    """The day the run signed the mortgage, from its decision log or trajectory; None if it never did."""
    for f in glob.glob(os.path.join(d, f"{player}__*__s{seed}__*", "log", "*.jsonl")) + glob.glob(os.path.join(d, f"{player}__*__s{seed}__*", "trajectory.jsonl")):
        for line in open(f):
            if '"mortgage_taken"' in line:
                m = re.search(r'"game_day":(\d+)', line)
                return int(m.group(1)) if m else -1
    return None


def won(r):
    return r["ending"].startswith("buyout")


def decisions(d, player):
    rows = []
    for f in glob.glob(os.path.join(d, f"{player}__*", "decisions.jsonl")):
        if not os.path.exists(os.path.join(os.path.dirname(f), "result.json")):
            continue
        for line in open(f):
            rows.append(json.loads(line))
    return rows


def main():
    d = sys.argv[1]
    base = sys.argv[2] if len(sys.argv) > 2 else "executor"
    res = load(d)
    print("| player | games | wins | median days | mean days | repo | rent | arrest/exec | median NW d100 | LLM calls/game | wall min/game | Δ days vs %s (paired) | Δ NW d100 | wins +/− |" % base)
    print("|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
    for p in sorted(res, key=lambda p: (p != base, p)):
        common = set(res[p]) & set(res.get(base, res[p]))
        rs = [res[p][s] for s in sorted(common)]
        days = [r["days_played"] for r in rs]
        ends = collections.Counter(r["ending"] for r in rs)
        calls = statistics.mean(r.get("llm_calls", 0) for r in rs) if rs else 0
        wall = statistics.mean(r.get("wall_seconds", 0) for r in rs) / 60 if rs else 0
        delta, dnw, plus, minus = "", "", 0, 0
        if p != base and base in res:
            m, se = mean_se([res[p][s]["days_played"] - res[base][s]["days_played"] for s in sorted(common)])
            delta = f"{m:+.1f} ± {1.96 * se:.1f}"
            m, se = mean_se([nw_on(res[p][s], 100) - nw_on(res[base][s], 100) for s in sorted(common)])
            dnw = f"{m:+.0f} ± {1.96 * se:.0f}"
            plus = sum(won(res[p][s]) and not won(res[base][s]) for s in common)
            minus = sum(won(res[base][s]) and not won(res[p][s]) for s in common)
        nw100 = statistics.median(nw_on(r, 100) for r in rs) if rs else 0
        print(f"| {p} | {len(rs)} | {sum(map(won, rs))} | {statistics.median(days) if days else 0:.0f} | {statistics.mean(days) if days else 0:.1f} | "
              f"{ends.get('repo', 0)} | {ends.get('rent', 0)} | {ends.get('arrest', 0) + ends.get('execution', 0)} | {nw100:.0f} | {calls:.1f} | {wall:.0f} | {delta} | {dnw} | {f'+{plus}/−{minus}' if p != base else ''} |")

    print("\n| player | signed | signing days | after signing: won / repo / other |")
    print("|---|---|---|---|")
    for p in sorted(res, key=lambda p: (p != base, p)):
        days = {s: signed(d, p, s) for s in sorted(res[p])}
        sig = {s: x for s, x in days.items() if x is not None}
        outs = collections.Counter("won" if won(res[p][s]) else res[p][s]["ending"] if res[p][s]["ending"] == "repo" else "other" for s in sig)
        print(f"| {p} | {len(sig)}/{len(days)} | {', '.join(str(x) for x in sig.values())} | {outs.get('won', 0)} / {outs.get('repo', 0)} / {outs.get('other', 0)} |")

    for p in sorted(res):
        rows = decisions(d, p)
        if not rows:
            continue
        print(f"\n{p}: {len(rows)} decisions")
        src = collections.Counter(next((c.get("source") for c in r["candidates"] if c["name"] == r["played"]), "?") for r in rows)
        print("  played by source:", dict(src))
        offered = collections.Counter(c.get("source") for r in rows for c in r["candidates"])
        print("  candidates by source:", dict(offered))
        argmax = collections.Counter(next((c.get("source") for c in r["candidates"] if c["name"] == r.get("argmax")), "?") for r in rows)
        print("  argmax by source (before the switch gate):", dict(argmax))
        beat, cleared, offered_n = 0, 0, 0
        for r in rows:
            keep = next((c for c in r["candidates"] if c["name"] == "keep"), None)
            props = [c for c in r["candidates"] if c.get("source") in ("llm", "blind")]
            if not keep or not props:
                continue
            offered_n += 1
            beat += max(c["score"] for c in props) > keep["score"]
            cleared += any(c["name"] == r["played"] for c in props)
        if offered_n:
            print(f"  decisions with a proposal rolled: {offered_n}; a proposal outscored keep in {beat} ({100 * beat / offered_n:.0f} %), "
                  f"cleared the switch gate and was played in {cleared} ({100 * cleared / offered_n:.0f} %)")
        gains = []
        for r in rows:
            sc = {c["name"]: c["score"] for c in r["candidates"]}
            if r["played"] != "keep":
                gains.append(sc[r["played"]] - sc["keep"])
        if gains:
            m, se = mean_se(gains)
            print(f"  switches: {len(gains)}, rollout gain over keep {m:.0f} ± {se:.0f}")
        trig = collections.Counter()
        trig_switch = collections.Counter()
        for r in rows:
            for t in r.get("triggers") or []:
                trig[t["id"]] += 1
                trig_switch[t["id"]] += r["played"] != "keep"
        print("  triggers (switched):", {k: f"{v} ({trig_switch[k]})" for k, v in sorted(trig.items())})
        keys = collections.Counter()
        for r in rows:
            c = next((c for c in r["candidates"] if c["name"] == r["played"]), None)
            if c and c.get("source") not in ("keep",):
                for k in (c.get("directives") or {}):
                    keys[k] += 1
        print("  keys in played switches:", dict(keys.most_common(12)))
        commit = [r for r in rows if (r.get("search") or {}).get("commitment")]
        if commit:
            print(f"  commitment decisions (rolled to the end of the term): {len(commit)}, mean horizon {statistics.mean(r['search']['horizon'] for r in commit):.0f} days")
        held = sum(1 for r in rows if next((c.get("source") for c in r["candidates"] if c["name"] == r["played"]), "") not in ("base",)
                   and next((c.get("directives") for c in r["candidates"] if c["name"] == r["played"]), {}) != {})
        print(f"  decisions playing a delta over base (gated against base): {held} ({100 * held / len(rows):.0f} %)")
        reps = collections.Counter((r.get("proposer") or {}).get("repair") for r in rows if r.get("proposer"))
        if reps:
            print("  proposer repairs:", {k or "none": v for k, v in reps.items()})
        fails = sum(1 for r in rows if (r.get("proposer") or {}).get("error"))
        if any(r.get("proposer") for r in rows):
            secs = [r["proposer"].get("seconds", 0) for r in rows if r.get("proposer")]
            print(f"  proposer: errors {fails}, mean {statistics.mean(secs):.1f} s per call, rejected {sum(len(r.get('rejected') or []) for r in rows)}")


if __name__ == "__main__":
    main()
