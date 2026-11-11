#!/usr/bin/env python3
"""Run the director (or the executor alone) over many seeds in parallel, then report.

  uv run batch.py run -gym DLL -out DIR -seeds 90101-90130 [-difficulty normal] [-start farmer]
                      [-params FILE.json] [-human-only] [-parallel 6] [-nice 10]
  uv run batch.py report DIR [DIR ...]
  uv run batch.py save-wins DIR DEST   (each won game's day folders + summary, for distillation)
"""
import argparse
import concurrent.futures as cf
import json
import os
import shutil
import statistics
import subprocess
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
WIN = ('buyout_mortgage', 'buyout')


def seeds(spec):
    out = []
    for part in spec.split(','):
        if '-' in part:
            lo, hi = part.split('-')
            out += list(range(int(lo), int(hi) + 1))
        else:
            out.append(int(part))
    return out


def run(a):
    os.makedirs(a.out, exist_ok=True)
    # Freeze the policy for the batch: later edits to sim/director must not leak into running games.
    code = os.path.join(a.out, 'code')
    os.makedirs(code, exist_ok=True)
    for f in ('play.py', 'policy.py'):
        if not os.path.exists(os.path.join(code, f)):
            shutil.copy(os.path.join(HERE, f), code)

    def one(seed):
        d = os.path.join(a.out, f's{seed}')
        if os.path.exists(os.path.join(d, 'summary.json')):
            return seed
        cmd = ['nice', '-n', str(a.nice), sys.executable, os.path.join(code, 'play.py'), '-gym', a.gym, '-seed', str(seed),
               '-dir', d, '-difficulty', a.difficulty, '-start', a.start, '-days', str(a.days)]
        if a.params:
            cmd += ['-params', a.params]
        if a.human_only:
            cmd.append('-human-only')
        subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=open(os.path.join(a.out, f's{seed}.err'), 'w'))
        return seed

    with cf.ThreadPoolExecutor(a.parallel) as ex:
        for s in ex.map(one, seeds(a.seeds)):
            print('done', s, flush=True)
    report([a.out])


def report(dirs):
    for d in dirs:
        if not os.path.isdir(d):
            continue
        rows = []
        for name in sorted(os.listdir(d)):
            f = os.path.join(d, name, 'summary.json')
            if os.path.exists(f):
                rows.append(json.load(open(f)))
        if not rows:
            continue
        wins = [r for r in rows if r['game_over'] in WIN]
        ends = {}
        for r in rows:
            ends[r['game_over'] or 'cap'] = ends.get(r['game_over'] or 'cap', 0) + 1
        wd = [r['last_day'] for r in wins]
        md = [r['mortgage_day'] for r in rows if r.get('mortgage_day')]
        print(f"{d}: {len(wins)}/{len(rows)} wins"
              f" | median win day {statistics.median(wd) if wd else '-'}"
              f" | median days {statistics.median(r['last_day'] or 0 for r in rows)}"
              f" | mortgages {len(md)} (median day {statistics.median(md) if md else '-'})"
              f" | endings {json.dumps(dict(sorted(ends.items())))}")


def save_wins(src, dest):
    tag = os.path.basename(os.path.normpath(src))
    n = 0
    for name in sorted(os.listdir(src)):
        f = os.path.join(src, name, 'summary.json')
        if not os.path.exists(f) or json.load(open(f))['game_over'] not in WIN:
            continue
        out = os.path.join(dest, f'{tag}-{name}')
        shutil.copytree(os.path.join(src, name), out, dirs_exist_ok=True,
                        ignore=shutil.ignore_patterns('run.jsonl', 'gym.log', '*.tmp'))
        n += 1
    print(f'{n} wins from {src} -> {dest}')


def main():
    if len(sys.argv) > 1 and sys.argv[1] == 'save-wins':
        save_wins(sys.argv[2], sys.argv[3])
        return
    if len(sys.argv) > 1 and sys.argv[1] == 'report':
        report(sys.argv[2:])
        return
    ap = argparse.ArgumentParser()
    ap.add_argument('cmd')
    ap.add_argument('-gym', required=True)
    ap.add_argument('-out', required=True)
    ap.add_argument('-seeds', default='90101-90130')
    ap.add_argument('-difficulty', default='normal')
    ap.add_argument('-start', default='farmer')
    ap.add_argument('-days', type=int, default=1000)
    ap.add_argument('-params', default=None)
    ap.add_argument('-human-only', action='store_true')
    ap.add_argument('-parallel', type=int, default=6)
    ap.add_argument('-nice', type=int, default=10)
    a = ap.parse_args()
    a.params = os.path.abspath(a.params) if a.params else None
    a.out = os.path.abspath(a.out)
    run(a)


if __name__ == '__main__':
    main()
