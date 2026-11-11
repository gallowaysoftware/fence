#!/usr/bin/env python3
"""Play one gym game with the director through step mode.

  uv run play.py -gym DLL -seed N -dir DIR [-difficulty normal] [-start farmer] [-days 1000]
                 [-params FILE.json] [-human-only]

Writes DIR/day-NNN/{snapshot,directives,result}.json (the gym), DIR/day-NNN/why.txt (the director's
reasons), DIR/final.json and DIR/summary.json. -human-only runs the executor alone (no step mode).
"""
import argparse
import json
import os
import subprocess
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import policy  # noqa: E402


def gym_cmd(a, dir_):
    cmd = ['dotnet', a.gym, 'run', '-human', 'player', '-seed', str(a.seed), '-days', str(a.days),
           '-difficulty', a.difficulty, '-start', a.start, '-no-trajectory',
           '-out', os.path.join(dir_, 'run.jsonl'), '-summary', os.path.join(dir_, 'gym-summary.json')]
    if not a.human_only:
        cmd += ['-step-dir', dir_, '-step-timeout', '120']
    return cmd


def gym_root(dll):
    # DLL = ROOT/sim/gym/bin/Release/net10.0/fence-gym.dll; the gym reads var/gamedata from ROOT.
    return os.path.abspath(os.path.join(os.path.dirname(dll), *(['..'] * 5)))


def write_atomic(path, text):
    tmp = path + '.tmp'
    with open(tmp, 'w') as f:
        f.write(text)
    os.replace(tmp, path)


def load(path):
    for _ in range(50):
        try:
            with open(path) as f:
                return json.load(f)
        except (json.JSONDecodeError, FileNotFoundError):
            time.sleep(0.02)
    raise RuntimeError('unreadable ' + path)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('-gym', required=True)
    ap.add_argument('-seed', type=int, required=True)
    ap.add_argument('-dir', required=True)
    ap.add_argument('-difficulty', default='normal')
    ap.add_argument('-start', default='farmer')
    ap.add_argument('-days', type=int, default=1000)
    ap.add_argument('-params', default=None)
    ap.add_argument('-human-only', action='store_true')
    a = ap.parse_args()
    os.makedirs(a.dir, exist_ok=True)
    overrides = json.load(open(a.params)) if a.params else {}
    p = policy.Params.load(overrides)
    mem = policy.Memory()
    log = open(os.path.join(a.dir, 'gym.log'), 'w')
    proc = subprocess.Popen(gym_cmd(a, a.dir), stdout=log, stderr=subprocess.STDOUT, cwd=gym_root(a.gym))
    day = 1
    while not a.human_only:
        dd = os.path.join(a.dir, f'day-{day:03d}')
        snap_path = os.path.join(dd, 'snapshot.json')
        while not os.path.exists(snap_path):
            if proc.poll() is not None or os.path.exists(os.path.join(a.dir, 'final.json')):
                break
            time.sleep(0.01)
        if not os.path.exists(snap_path):
            break
        snap = load(snap_path)
        day = snap['day']
        mem.observe(snap)
        d, why = policy.decide(snap, mem, p)
        write_atomic(os.path.join(dd, 'why.txt'), '; '.join(why) + '\n')
        write_atomic(os.path.join(dd, 'directives.json'), policy.dumps(d))
        day += 1
    proc.wait()
    gs = {}
    try:
        gs = json.load(open(os.path.join(a.dir, 'gym-summary.json')))
    except (OSError, json.JSONDecodeError):
        pass
    out = {'seed': a.seed, 'difficulty': a.difficulty, 'start': a.start, 'human_only': a.human_only,
           'game_over': gs.get('game_over'), 'last_day': gs.get('last_day'), 'cash': gs.get('cash'),
           'mortgage_day': mem.mortgage_day, 'georgio_lent': mem.georgio_lent, 'exit': proc.returncode,
           'params': p.to_json()}
    write_atomic(os.path.join(a.dir, 'summary.json'), json.dumps(out, indent=1))
    print(json.dumps({k: v for k, v in out.items() if k != 'params'}))


if __name__ == '__main__':
    main()
