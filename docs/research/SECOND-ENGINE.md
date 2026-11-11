# A second money engine on hard (2026-11-11)

**Question.** Every winning hard route gets 55–71 % of its money from the Appraisal Board, and without the Board a
shop grows its net worth by 700–900 a week from day 70 against a rent that needs ~1 000–1 500 a week more
(ROUTES.md, "Without the Appraisal Board"). Can a second engine close that gap?

**Verdict: no, not on hard.** Demand is not the limit: free caffeine pills or coffee beans put in stock sell almost to
the last unit, and a dozen lab-pure pills a day would win every run. Supply is. Every engine that turns a cheap input into a good the counter sells runs into the same
four walls: inputs come a few at a time from visitors, the hydroponic bays that could make them need clean water and
power the shop does not have, the chemistry lab's modules almost never reach the counter, and the 1 500 the Retired
Chemist costs on hard is only affordable after Georgio's payback around day 70, which leaves too few weeks before the
rent wins. Built and measured: a cash-crop lever for the bays (`crops`) and a caffeine lab (`lab: "caffeine"`); both
lose net worth without the Board (0 wins in 96 runs) and cost ~38 days with it. Numbers below.

Method: hard, `-start apprentice -perks vending+short_sleeper -human player`; the `appraiser` route with `appraisal`
removed ("Board-free") or as shipped; paired on fresh seeds 8500001–016; this branch's gym. Δ are paired means
± 1.96 SE. The decompiled code was read for every rule; the lab yields come from `fence-lab production.crops`
(new, `sim/lab/ProductionExps.cs`) and the chemistry experiments already in `sim/lab/ChemExps.cs`.

## Paper estimates

### Crops in a hydroponic bay (`fence-lab production.crops`)

24 nights, 4 bays per crop, replanted from the harvest. Value a night counts what is sold, not the seed kept to
replant (coffee replants one of its own beans; nutrifruit is infertile and needs a bought seed, 40, each harvest).

| Crop | Water | Harvest every | Per harvest | Value a night (per bay) |
|---|---|---|---|---|
| coffee bean | pure + a nutrient tablet a litre | 3 nights | 8 beans | 65 |
| coffee bean | base + tablets / base | 3 | 5.7 / 5 | 44 / 37 |
| coffee bean | ghost, rust or gutter (any grade) | 3 | 4 | 28 |
| kotton | pure + tablets | 6 | 12 fiber (46–60) + 1.4 seeds | 92 |
| kotton | base or worse | 6 | 6 fiber | 36 |
| nutrifruit | pure + tablets | 2 | 6 (sized by health) | 96 (76 after the seed) |
| fillerweed | pure + tablets | 1 | 5 + 1.5 seeds | 51 (750 kcal) |
| dream cap (contraband 2) | pure + tablets | 4 | 7 + 1.3 seeds | 40 |
| bloomberry (the cellar's) | pure + tablets | 2 | 6 berries | 38.5 |

Costs per bay: 300 (a farmer's visit) and 8 power a night, ~13 at the 1.65 a unit cells cost on hard. The crops
the shop could water (base or tap water) earn 28–44 a night before power, 15–31 after: 100–220 a week per bay.
Closing a 1 000–1 500 gap takes 5–10 bays, their cells, and a litre of clean water a bay every week, in a shop
that holds almost no clean water at all (below).

### Rats on a feed bay

A bay of fillerweed makes ~750 kcal a night, nutrifruit ~900, against the ~450 kcal a day sellers offer, so a feed
bay would lift the food wall in ARCHETYPES-NEW.md. It does not make money: measured over 120 runs the rats return
about what their feed cost at the counter (2 483 of meat against 2 744 of feed), so a bay's fillerweed fed to rats
is worth about what it sells for raw (~50 a night), less the colony's deaths. Not built.

### Chemistry: caffeine (legal) and dream dust (contraband)

From docs/re/CHEMISTRY.md §4–7 and the code:

| | Caffeine pill | Dream dust |
|---|---|---|
| Unlock | RETIRED_CHEMIST (1 500 on hard): blender, plate, evaporator, finisher, glassware, one bean, one supply pack | same |
| Input | coffee bean (28): retired farmer 2 a visit, shady farmer ~1.5 twice a week, or a coffee bay | dream cap (23, contraband 2): shady farmer ~3 twice a week, or a bay (seeds come back) |
| Per input, 4 modules on blender and finisher | 8.3 pills × 24 (lab pure) = 200 | 5.5 dust × 72 (market) = 400 |
| Per input, no modules | 5.4 pills × 14 (near pharma at best) ≈ 75; with only kotton to filter, street grade (10) | ~3.6 dust, near pharma |
| Consumables | a supply pack (50) has 350 ml alcohol: ~10 beans at full yield | same |
| Heat | none (a bad unit to a non-BM buyer is FRAUD) | DRUG_TRAFFICKING every sale; the cartel and Lun |

The modules are the catch. The Retired Chemist lays his four machines on the front counter first, and the counter
is full before his `chem_module` (always one, plus 50/50/25 %) is reached: in his six visits logged on seed 8500001
(days 54–79) no module was offered. Dr. Jackson sells them only with RETIRED_CHEMIST (50 % + 25 %), at +50 % on hard. So the
realistic lab runs unmodded: ~75 a bean. Dream dust doubles that but is contraband 2 in stock and in every sale,
and the gym's routes that carry contraband already end in arrests; it was not built.

### Demand: what the counter would sell (`supply_probe`)

A gym-only probe (`sim/gym/SupplyProbe.cs`, not a player move) puts free units in stock every morning from day 30;
what sells is the most an engine making that good could earn. Board-free, 16 seeds:

| Free each day | Sold | Per unit | Sales a week | Wins (Board-free) | Δ last day | Δ NW day 100 |
|---|---|---|---|---|---|---|
| 12 lab-pure caffeine pills (99.5 %) | 97.8 % | 22.1 | 1 815 | **16/16**, median day 105 | — | — |
| 12 pharma-grade pills (92 %) | 97.7 % | 17.1 | 1 397 | 6/16 (10 alive on day 200) | +0 ± 20 | +15 412 ± 1 687 |
| 4 coffee beans | 92.0 % | 26.0 | 667 | 4/16 (mortgage) | +25 ± 17 | +4 661 ± 2 139 |
| 4 near-pharma pills (85 %), what an unmodded lab makes | 97.9 % | 12.9 | 354 | 0/16 | −4 ± 21 | +1 193 ± 1 087 |

(Reference: the Board-free route, 0/16, median last day 196. A run that buys out early has no net worth on day 100,
so that column is blank for the 16/16 arm.) The counter sells 92–98 % of everything put in front of it, and the
line where Board-free runs start to win sits between ~700 and ~1 800 a week of free income, the gap ROUTES.md
measured.

The counter sells 95–99 % of everything put in front of it. An engine making a dozen lab-pure pills a day would win
the run; the question is only whether one can be built.

## What was built

- **`crops: {"coffee_bean": 2, ...}`** (`sim/gym/Crops.cs`, shared with the live autopilot): that many more bays
  beyond the cellar's `bays`, each growing its crop, replanted from its own harvest, the rest sold as stock. With
  `crops_tap` (default true) it buys WATER_TRADER and GUTTERFLOW_TAP and fills a crop bay at the faucet when it runs
  dry: coffee keeps half its harvest on any water grade.
- **`lab: "caffeine"`** (`sim/gym/Lab.cs`, shared): buys RETIRED_CHEMIST (after the permit unless
  `lab_after_permit: false`; `lab_reserve`, default 500, kept over rent and floor), then runs a batch a step each
  morning while the day's real seconds run the reactions: blend the beans, acid equal to the protein and lipobond an
  eighth of the fat on the shelf, alcohol up to ¾ of the mush; base for any acid left, onto the plate (unmodded it
  holds ~99 °C: the coffee extracts while the alcohol lasts, then the alcohol boils off); pour through the filter
  pitcher and back, into the powered evaporator; once dry, the finisher presses pills until the residue runs out.
  Modules go to the finisher first, then the blender. It buys beans (12 held), packs, a second pitcher, filters, a
  cell for the evaporator and any missing machine; the kit is stowed off the counters (a buyer's "sell all" took it).
- Both off by default; with them off the gym is unchanged (below).

## Measured

Board-free, 16 seeds 8500001–016, 200 days; reference the route without the Board (0/16, median last day 196,
3 alive on day 200). No lever arm was alive on day 200 (its Δ net worth on day 200 is the reference's −922 ± 1 002).

| Arm | Wins | Median last day | Endings | Δ last day | Δ NW day 100 | Engine output a run |
|---|---|---|---|---|---|---|
| `crops: {coffee_bean: 2}` | 0/16 | 168 | rent 16 | −7.5 ± 17.8 | −1 258 ± 969 | 131 harvested, 2 382 sold, 12 L of tap water |
| `crops: {coffee_bean: 4}` | 0/16 | 154 | rent 14, arrest 1, execution 1 | −25.2 ± 17.9 | −1 612 ± 907 | 122 harvested, 2 088 sold |
| `lab: caffeine` (after the permit) | 0/16 | 147 | rent 16 | −28.5 ± 17.2 | −2 375 ± 738 | 85 pills, 912 sold |
| `lab`, `lab_after_permit: false`, `lab_reserve: 1500` | 0/16 | 147 | rent 14, arrest 2 | −29.9 ± 17.4 | −2 150 ± 745 | 22 beans → 120 pills, 1 193 sold |
| `lab` + `crops: {coffee_bean: 2}` | 0/16 | 140 | rent 13, arrest 3 | −34.0 ± 21.5 | −2 429 ± 1 161 | 1 bean, 0 pills; 62 harvested |
| the same, `lab_after_permit: false` | 0/16 | 133 | rent 11, arrest 5 | −31.9 ± 22.9 | −2 251 ± 1 078 | 9 pills; 69 harvested |

With the Board (the `appraiser` route as shipped plus the lever, 16 seeds, 300 days; the route wins 16/16, median
day 98):

| Arm | Wins | Median win day | Δ win day |
|---|---|---|---|
| `lab: caffeine` | 16/16 (4 by mortgage) | 119 | **+38.9 ± 25.7** (198 pills, 1 947 sold a run) |
| `crops: {coffee_bean: 2}` | 16/16 (4 by mortgage) | 119 | **+37.6 ± 24.4** (105 harvested, 1 941 sold a run) |

The lab works mechanically: it buys the Retired Chemist, stows the kit, opens supply packs, blends, measures acid
and lipobond through the kit's cylinders and droppers, heats, filters, dries and presses pills that sell. Without
the Board it unlocks on day 69–71 in 13 of 16 runs (the 1 500 is not there before Georgio's payback) and then makes
~11 pills a week: a mean of 22 beans a run reach it (at most 45), its pills sell at ~10 (street grade, filtered through kotton until a
water filter turns up, no module ever fitted in 16 runs), and the kit's one pitcher often has no second vessel to
work against (`vessels` stalls). Off by default, the gym is byte-identical to main (below).

## Why it does not close the gap

- **Inputs arrive by the handful.** Beans come two a week from the retired farmer and ~3 from the shady farmer
  (who comes only with RETIRED_CHEMIST); the lab got 22 a run on average. The probe's winning arm is 12 pills a day, about
  10 beans a week at four modules or 15 at none.
- **The bays cannot make them.** A coffee bay on the water the shop can get (tap water, base water) makes 28–44 a
  night and spends ~13 of it on power; the shop holds almost no clean water (none on most mornings of the Board-free
  run on seed 8500001, the wine bays included), and on hard the cells come from Dr. Jackson at 165. Measured crop bays harvested
  ~130 items in a run where two bays on tap water would make ~370, sitting unpowered or dry, and lost money after
  the two tap upgrades and the bays.
- **The lab stays unmodded.** Modules decide the price (12 % impurity and 35 % blender loss at none: ~75 a bean
  against ~200 at four), and the Retired Chemist's modules come last on a counter his machines have already filled;
  Jackson's are +50 % and only some visits. An unmodded lab's 4 pills a day are worth ~350 a week even when free.
- **It starts too late and costs too much.** 1 500 for the unlock (plus any machine a buyer took off the counter)
  is spent on day ~69, exactly when a Board-free shop has its one cash peak; the permit and its bribes compete for
  the same money (on the first seeds tried, the lab bought before the permit ended in OPERATING_WITHOUT_LICENSE
  arrests, hence `lab_after_permit`).
- **With the Board it is a detour.** The Board already pays the rent; anything that spends 1 500–2 500 before the
  cash buyout delays the buyout by about five weeks.

**Not built.** Dream dust doubles the lab's value per input but is contraband 2 in stock and in every sale, needs
the same modules, and the dealer comes twice a week; the rats on a feed bay convert feed to meat at about par (paper
estimate above). Nothing else in the code earns ~1 000 a week by day 70 without the Board: the furnace, wine,
water, the fountain, suppliers, the printer and rats were measured before (ROUTES.md, ARCHETYPES-NEW.md).

**What would change the answer.** A cheaper or earlier chemist (the chemist start, whose cartel the gym does not
play), a module source, or a power and water supply that lets 4–6 coffee bays run at their 44–65 a night.

## Defaults unchanged

With every new lever off, 8 runs (hard `appraiser`, `smelter`, `rancher` from its start, normal `appraiser`; seeds
8500101–102, 300 days) give byte-identical summaries to local main b6bbd24, apart from the wall-clock fields.

## Reproducing

```
fence-gym smoke -seeds 16 -seed0 8500000 -days 200 -parallel 8 -human player -difficulty hard \
  -start apprentice -perks vending+short_sleeper -directives <the appraiser route without "appraisal", plus the lever>
fence-lab production.crops
```
