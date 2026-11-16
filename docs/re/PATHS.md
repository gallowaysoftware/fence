# Production paths: the small plan space (design note)

The game has too many total states to plan over, but the *decision space* is tiny: roughly
**eleven production paths**, each with a 3–6 item equipment set, and a handful of *couplings*
where one path's output is another's input. A run's strategy is a subset of paths plus a
payback-ordered build within each. That is small enough for a player — or a planner — to
evaluate explicitly.

## The paths (net/night, hard, from docs)

| Path | Equipment (cost) | Net/night | Source |
|---|---|---|---|
| rats (husbandry) | cages, breeders, feed | 450 (peak rule, 80 cells) | HUSBANDRY §7 |
| wine (1 berry/bottle) | hydroponic 300 + bottles + yeast | 270 (bottle-limited) | PRODUCTION §7 |
| reed-cure water | reed bay 490 (hydroponic + seed) | 240 (needs raw supply + HQ buyer) | WATER, measured |
| fish (aquaculture) | AQUACULTURE 400 + tanks + roe | 125–390 (feed-capped) | AQUACULTURE §5.1 |
| furnace (blast + ore) | furnace 200 + module 150 | 85 | PRODUCTION §7 |
| water (purifier) | purifier ~350 + uv 270 | 60 | WATER, measured |
| arms (heavy frames) | Permit II + forge printer | 134 per gun, printer-bound | ARMS §5 |
| chem (dream caps) | chemist kit 1500 (retired chemist) | ~72/unit at market | CHEMISTRY §0 |
| berries as food | hydroponic 300 + seed 40 | 34 | PRODUCTION §7 |
| counter-trading | none | margin-bound (15 %) | BARTER |
| printing | old printer + plastic | 25/night | ARMS |

## The couplings (why combinations beat sums)

- **Water feeds four paths**: wine quality, chem purity, fish volume, bay water. The purifier
  (6n payback) raises the ceiling of every water-consuming path at once — which is why it is the
  archetype-independent first buy.
- **Ingots feed arms**: scrap/ore from scavenge is free; the furnace is arms' prerequisite, not a
  peer. Blast module (150) doubles the furnace's 18 to 85 — the module pays in 2 nights.
- **Berries fork**: wine (270) dominates food (34) whenever a bottle and yeast exist. The
  binding constraint is bottles and yeast, not berries — so the *bottle supply* is the real
  lever, not the bay count.
- **The fountain is a platform**: 27n alone, commons 2–4n after it, and it monetises every
  grade of water the purifier produces — the only path that turns *surplus* into cash.

## The planning algorithm (what the player does)

1. **Enumerate reachable paths** for the start (suppliers: Water Merchant → water paths day 1;
   chemist start → chem day 1; others gated by suppliers worth 250–1500).
2. **Payback-rank each path's equipment**, cheapest-payback first: reed bay 2.0 → commons 1.7 →
   blast module 2 → uv 4.5 → purifier 5.8 → taps 6.2 → farm 15.4 → fountain 26.9.
3. **Global buy order** = sort by (payback, dependency depth). Amass anything under the current
   target's remaining stretch; enter the savings phase only for the first item whose payback
   exceeds it (fountain 1800/27n is the usual first).
4. **Re-evaluate on windfalls**: a supplier visit (farmer, chemist, fishmonger) resets step 3 —
   the path graph is static, the *prices* aren't.

## What this changes in the code (small, deliberate)

- `BuildOrder` already implements steps 2–3 for *directive-listed* kits (`Paper` tables,
  `payback_margin`, `PushesToMortgage`). What is missing is **path-level composition**: the
  strategist picks a subset of paths (day-0), but nothing *derives* the kit list from the path —
  the routes hard-code it. A `paths.json` (path → equipment, per-night, prerequisites) would let
  the strategist compose plans by naming paths, and let the executor compute payback ordering
  itself instead of reading hand-tuned `income:` values.
- The save floor (`TradeFloor`) generalises: it should key on the *pending item with the shortest
  payback across all committed paths* — currently only the purifier and fountain are modeled.

Not changing anything else on this basis yet — the water-baron run is the reference
implementation of one path; the league report will show whether the planner arms scale on their
own before we build the path graph into composition.

## Proposed `paths.json` (design)

```json
{
  "water": {
    "equipment": [
      {"id": "uv_filter", "cost": 270, "from": "jackson"},
      {"id": "water_purifier", "cost": 350, "from": "jackson", "wants": "PurifierTarget"},
      {"id": "vending_fountain", "cost": 1800, "from": "wild", "save_for": true},
      {"id": "common_electronic", "cost": 60, "from": "anywhere", "repeat_to": 17}
    ],
    "per_night": {"uv_filter": 60, "water_purifier": 60, "vending_fountain": 67, "common_electronic": 35},
    "requires": []
  },
  "wine": {
    "equipment": [
      {"id": "hydroponic", "cost": 390, "from": "farmer"},
      {"id": "bloomberry_seed", "cost": 40},
      {"id": "retired_winemaker", "cost": 300, "from": "wild", "enabler": true}
    ],
    "per_night": {"hydroponic": 270},
    "requires": []
  },
  "reed_cure": {
    "equipment": [
      {"id": "hydroponic", "cost": 390},
      {"id": "hydroreed_seed", "cost": 20},
      {"id": "water_tank", "cost": 455, "from": "jackson"}
    ],
    "per_night": {"default": 240},
    "requires": ["water_source"]
  }
}
```

Composition then reads: `route.paths = ["wine", "reed_cure", "water"]` → the executor walks each
path's equipment in payback order (per_night/cost), respects `requires`, and sets TradeFloor from
the first `save_for` item. The strategist's day-0 job shrinks to naming paths + tuning numbers
(`purifiers: 3`, `reeds_max: 4`), and mid-run pivots become "add path X" decisions the planner
can make from the same table. `BuildOrder.Paper` stays as the price source; `paths.json` replaces
only the hand-tuned `income:`/`ramp:` values.
