#!/usr/bin/env python3
"""One line a day for a played game directory: trace.py DIR [FROM [TO]]."""
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import policy  # noqa: E402


def line(dd):
    snap = json.load(open(os.path.join(dd, 'snapshot.json')))
    s, r = snap['snapshot'], snap['risks']
    st = s['store']
    top = policy.top_inventory(s)
    stock = sum(policy.est(i) for i in top)
    w = policy.finished_wines(top)
    due, left, mort = policy.owed(snap)
    m = r.get('mortgage') or {}
    res = {}
    if os.path.exists(os.path.join(dd, 'result.json')):
        res = json.load(open(os.path.join(dd, 'result.json')))
    why = open(os.path.join(dd, 'why.txt')).read().strip() if os.path.exists(os.path.join(dd, 'why.txt')) else ''
    pw = policy.wine_prod(snap.get('yesterday'))
    cf = {k: v for k, v in (res.get('cash_flow') or {}).items() if v}
    ms = f" M{m['weekly']}x{m['weeks_left']}" if mort else (' OFF' if m.get('offerable_now') else '')
    return (f"d{snap['day']} c{st['cash']} nw{st['net_worth']} stk{stock} fav{st.get('wild_favor')} att{st['attractiveness']['total']} "
            f"{'pay' if mort else 'rent'}{due}/{left} pm{int(st['business_permit'])} cr{r['record']['crime']} ev{r['record']['evidence']} "
            f"LL{s['reputation']['lower_levels']['value']:.0f} wine{len(w)}/{sum(policy.est(i) for i in w)} st{pw.get('started_total')}{ms}"
            f" | sold{res.get('sold_items')}/{res.get('sold_value')} bought{res.get('bought_items')}/{res.get('spent')} ->c{res.get('cash_end')}"
            f" {json.dumps(cf) if cf else ''} {res.get('game_over') or ''} || {why}")


def main():
    d = sys.argv[1]
    lo = int(sys.argv[2]) if len(sys.argv) > 2 else 0
    hi = int(sys.argv[3]) if len(sys.argv) > 3 else 9999
    for name in sorted(os.listdir(d)):
        if name.startswith('day-') and lo <= int(name[4:]) <= hi:
            print(line(os.path.join(d, name)))


if __name__ == '__main__':
    main()
