#!/usr/bin/env python3
"""Where a policy's money comes from (docs/research/ROUTES.md, "Where the money comes from").

  income.py DIR [DIR ...]

Each DIR holds gym run summaries (`fence-gym run -summary FILE`, `*.json` or `*.json.gz`). Per DIR, the mean per
run of each engine's net contribution, from the summary's `player_days[].cash_flow`, `player.appraisal.fees_total`
and `player.furnace.ingot_sales`:

  board     Appraisal Board fees
  counter   counter and vending sales less the trading stock bought (vending sells stock, so the two share a cost)
  wine, ingots, water, fountain   gross sales of each product line
  inputs    engine inputs bought (berries, bottles, yeast, ore, flux, cells, water supplies, machines)
  georgio   Georgio's payback less his loan

Shares are of the positive contributions: board, counter, production (wine + ingots + water + fountain - inputs)
and georgio.
"""
import gzip
import json
import os
import sys


def load(path):
    opener = gzip.open if path.endswith(".gz") else open
    with opener(path, "rt") as f:
        return json.load(f)


def contributions(s):
    flow = {}
    for d in s.get("player_days") or []:
        for k, v in (d.get("cash_flow") or {}).items():
            flow[k] = flow.get(k, 0) + v
    p = s.get("player") or {}
    ingots = (p.get("furnace") or {}).get("ingot_sales", 0) or 0
    c = {
        "board": (p.get("appraisal") or {}).get("fees_total", 0) or 0,
        "counter": flow.get("trade_sales", 0) - ingots + flow.get("vending_sales", 0) - flow.get("trade_purchases", 0),
        "wine": flow.get("wine_sales", 0),
        "ingots": ingots,
        "water": flow.get("water_sales", 0),
        "fountain": flow.get("fountain_sales", 0),
        "inputs": flow.get("input_costs", 0),
        "georgio": flow.get("georgio_payback", 0) - flow.get("georgio_loan", 0),
    }
    c["production"] = c["wine"] + c["ingots"] + c["water"] + c["fountain"] - c["inputs"]
    return c


def main(dirs):
    keys = ["board", "counter", "wine", "ingots", "water", "fountain", "inputs", "production", "georgio"]
    print("dir".ljust(28) + "".join(k.rjust(11) for k in keys) + "   shares board/counter/production/georgio")
    for d in dirs:
        files = [os.path.join(d, f) for f in sorted(os.listdir(d)) if f.endswith(".json") or f.endswith(".json.gz")]
        runs = []
        for f in files:
            s = load(f)
            if isinstance(s, dict) and "player_days" in s:
                runs.append(contributions(s))
        if not runs:
            continue
        mean = {k: sum(r[k] for r in runs) / len(runs) for k in keys}
        pos = sum(max(0, mean[k]) for k in ("board", "counter", "production", "georgio")) or 1
        shares = "/".join(f"{100 * max(0, mean[k]) / pos:.0f}" for k in ("board", "counter", "production", "georgio"))
        print(os.path.basename(os.path.normpath(d)).ljust(28) + "".join(f"{mean[k]:11.0f}" for k in keys) + f"   {shares} % (n={len(runs)})")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    main(sys.argv[1:])
