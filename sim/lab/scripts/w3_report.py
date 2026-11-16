import json, glob, os, statistics, re
def fountain_day(seed):
    p=f'var/league/w3-trade-floor/local-qwen__r-water_baron__s{seed}__water-baron/bridge.log'
    if not os.path.exists(p): return None
    m=re.findall(r'fork day=(\d+) kind=fountain choice=buy', open(p,errors='ignore').read())
    return int(m[0]) if m else None
rows=[]
for seed in range(2610003,2610011):
    f=f'var/league/w3-trade-floor/local-qwen__r-water_baron__s{seed}__water-baron/gym.summary.json'
    if not os.path.exists(f): rows.append((seed,None,None,None)); continue
    s=json.load(open(f)); p=s['player']
    nw=s['player_days'][-1]['net_worth_end'] if s.get('player_days') else None
    rows.append((seed, fountain_day(str(seed)), nw, p['fountain'].get('income_total')))
print("seed | fountain day | NW end | fountain income")
for r in rows: print(r)
done=[r for r in rows if r[2] is not None]
if len(done)==8:
    fds=[r[1] for r in done if r[1]]
    print("fountain median:", statistics.median(fds) if fds else None)
    print("NW median:", statistics.median([r[2] for r in done]))
