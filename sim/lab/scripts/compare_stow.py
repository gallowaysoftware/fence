import json, glob, os, statistics
lab={"water_baron":112,"vintner":98,"smelter":84,"fence":105,"cook":105,"rancher":83,"aquafarmer":196,"armourer":98}
print(f"{'route':14} {'win%':>5} {'median day':>10} {'lab':>5} {'dNW end':>9}")
by={}
for f in glob.glob('var/league/sweep-stow-drift/*/gym.summary.json'):
    s=json.load(open(f))
    route=f.split('__r-')[1].split('__')[0]
    by.setdefault(route,[]).append(s)
for r,runs in sorted(by.items()):
    wins=[]
    for s in runs:
        over=s.get('game_over')
        wins.append(1 if over=='buyout' else 0)
    days=[s['last_day'] for s in runs if s.get('game_over')=='buyout']
    nws=[s['net_worth_by_day'][-1] for s in runs if s.get('net_worth_by_day')]
    med=f"{statistics.median(days):.0f}" if days else "–"
    print(f"{r:14} {sum(wins)}/{len(runs):>2} {med:>10} {lab.get(r,'–'):>5} {statistics.median(nws) if nws else 0:>9.0f}")
