# Production verdicts, re-examined (2026-11-12)

**Question.** Our research says production nets little on hard (SECOND-ENGINE.md, ROUTES.md, PROGRESS.md,
ARCHETYPES-NEW.md). The price spreads say otherwise. Were the negative verdicts wrong, and why?

**Answer: most were wrong, for the same few reasons.** None of the verdicts got the game's numbers wrong. What
they got wrong is the measurement:

- They measured **our executor's engine**, and that engine ran on the slow or expensive path:
  - it bought power as cells;
  - it purified raw tap water;
  - it put two berries in a bottle;
  - it never fitted a chem module;
  - it left bays dry.
- Then they **read the result as the ceiling** of the good.

The demand side is far larger than the old "supply wall" framing allowed. On hard, Board-free, free goods from day 30
win every run:
- 24 pharma caffeine pills a day: 16/16 by day 90.
- 3 Master Vintage bottles a day: 16/16 by day 69.

The water fountain sells 431–617 a night with no customer.

Costed engines (the whole kit paid on the day the cash allows) change two results:
- **With the Board** they shorten the cash buyout:
  - A Master Vintage cellar: −19.2 ± 5.5 days (97 → 76).
  - The fountain: −14.0 ± 4.5 days (97 → 83).
  - A pharma caffeine lab: −10.1 ± 4.5 days.
  - SECOND-ENGINE.md had +38 days.
- **Board-free** they lift wins from 0/16 to 6–8/16 when the kit is cheap enough.

On 60 fresh seeds the cellar is −17.0 ± 2.2 days and the fountain −12.4 ± 1.6.

One old wall is real: **capital before day ~70**. Until Georgio's payback the shop never has the kit's price spare. So a
Board-free win needs an engine built in stages, early.

The other walls were measurement artifacts:
- inputs;
- water;
- power;
- modules, as far as the gym tested them.

Two walls the old research did not name:
- alcohol packs for the lab;
- the window slots that bring the bulk buyers.

Method:
- Code reads are cited by `Class.Method`.
- The lab measurements are new experiments in `sim/lab/VerdictExps.cs`.
- Gym runs are on hard, `-start apprentice -perks vending+short_sleeper -human player`. Seeds are 8500001–016, the
  same 16 SECOND-ENGINE.md used, so its arms are paired with ours. Board-free = the `appraiser` route without
  `appraisal`, 200 days.
- Wins are buyouts, with a Wilson 95 % CI. Δ are paired means ± 1.96 SE. McNemar compares wins.
- The arm `c92x12` reproduces SECOND-ENGINE.md's "12 pharma pills" row exactly: 6/16 and ΔNW100 +15 412 ± 1 687. So
  this build and theirs agree.

## The verdicts, ranked by value at stake

Rows are ranked by the measured gain on the Board route. The first column is the verdict's section number below.

| # | Old verdict (where) | Wrong? | The missed nuance | Value at stake (hard) |
|---|---|---|---|---|
| 4 | Wine: aging hurts (−14 days), racks and more bays are neutral (PROGRESS.md "Wine levers", WINE.md §6) | **Mostly wrong** | (a) The executor sells at mean age 0.2 nights, mostly Production (125), from 2 berries a bottle on Base-ish water. (b) On HQ water + tablets, 84 % of berries are normal or best (lab), and 3 such berries + yeast + HQ water = Master Vintage (175). (c) Aging is +3 × grade a night on any shelf, uncapped: 3 % a night on a 125 bottle. The −14 days came from the hold refusing the Hedonist (`KeepWhy` "wine_aging"), an executor artifact. (d) WINE.md's rack holds **6** bottles (1×4 shape in a 6×4 grid), not 24 | ~5 Master bottles a bay-week on HQ water (~900). Probe: 3 free Master bottles a day win 16/16 Board-free by day 69, all sold at ~170. Costed: **−19.2 ± 5.5 days** with the Board (97 → 76); Board-free 7/16 against 0/16 |
| 1 | Water does not close the gap; the fountain sells ~300–600 a week; the shop makes only 2–3 L of clean water a day (ROUTES.md "Water", PROGRESS.md ~1535–1556) | **Wrong** | (a) The fountain is deterministic: **431 a night** from Base+HQ at attractiveness 100, **617** with Pure (lab). (b) A **hydroreed bay** cleans its water slot by a percentage, so a 20 L tank of filtered tap water reaches HQ in 4 nights with no tablets and no purifier cap (lab). Filter + one tablet a litre + one purifier night also gives ≤ 10 L of HQ a night. (c) An HQ jug worth ≥ 350 in the case pulls the **water stockpiler** (500–599 + day) every day from day 14. (d) Power is free (#2) | ~2 500–3 000 a week net at attractiveness 100. Measured: −14.0 ± 4.5 days on the Board route; Board-free 4/16 (full kit) to 8/16 (landlord's fountain) against 0/16 |
| 3 | Caffeine lab: "supply, not demand, is the wall"; an unmodded lab's 4 pills a day are worth ~350 a week (SECOND-ENGINE.md, PROGRESS.md ~213) | **Wrong on the wall, right that our lab fails** | (a) Beans need not come from visitors. A coffee bay on free gutter/rust tap water keeps half its harvest (4 per 3 nights; 1 net a night after replanting). On HQ water it makes 6 (1.67 net a night, lab). (b) "4 pills a day" was the bean supply the executor got, not what an unmodded lab makes. (c) Demand at 24 a day is there (probe below). (d) Chem modules: Dr. Jackson with RETIRED_CHEMIST offers 0.75 a visit, 1.25/1.75 with JACKSON1/2; one finisher module lifts near pharma to pharma | Probe: 24 near-pharma pills a day win 14/16 Board-free; 24 pharma 16/16 by day 90. Costed: −10.1 ± 4.5 days with the Board; Board-free 0/16 on the full 5 700 kit (starts day ~74), 6/16 on the farmer start's 3 500 kit. The "too late, too costly" wall is right Board-free |
| 2 | Power is a real cost: cells at ~1.65 a unit, ~200 a week (PROGRESS.md ~1139; SECOND-ENGINE.md "13 of power a night per bay") | **Wrong** | A **recharger** (225 on hard) charges 8 cells +3 a night (+6 with COMMERCIAL_POWER, 525). **POWER_HIJACK** (450) zeroes the power service. A powerblock feeds adjacent machines. The gym never buys any of them | The Board-free reference spends 3.5 k cash on cells (6.0 k of charge) in 171 days: ~150–250 a week, in every route. Every engine's margin rises by 8–24 a machine-night |
| 5 | Rats on a feed bay convert feed at par; not built (SECOND-ENGINE.md, ARCHETYPES-NEW.md ~128) | **Wrong premise** | The "par" figure is bought feed against meat from colonies that starved out (113/120). Fillerweed is immune to contaminants (`contaminantEffect` 0). It grows on free tap water: 5 a night on a 1-night cycle (innate traits; PRODUCTION.md's table ignores them), ~750 kcal a night. Its raw value as food is only reachable through a capped FOOD slot | Paper: one feed bay keeps ~10 rats fed; ~1 rat/night at 159 + scrap is ~150–200 a night if raw meat sells (FOOD + LUXURY) |
| 6 | Coffee beans to the counter: 4 a day only 4/16 (SECOND-ENGINE.md probe) | **Incomplete** | 8 beans a day win 9/16 Board-free (probe below). Bays on free tap water make them, at no power cost (#2) | ~1 450 a week at 8 beans a day; needs ~8 tap bays or ~5 HQ bays |
| 7 | Crop bays: lose money (−1 258 NW at day 100), "sat unpowered or dry" (SECOND-ENGINE.md) | **Wrong reading** | The arm measured an executor that kept the bays dry and unpowered: 131 beans harvested where two tap bays make ~370. Power counted at 1.65/unit. The water source is the free tap | A working tap bay nets ~26 a night raw, ~70 as near-pharma caffeine |
| 8 | Purify tap water for wine/hydro: −63 days, 11 arrests (PROGRESS.md ~1532) | **Wrong cause** | The arrests came from buying the taps before the permit (OPERATING_WITHOUT_LICENSE), not from water. The treated water was raw-purified (slow path) | Order of purchase, not value |
| 9 | Furnace: ore-bound, ~80 a night fed, ~450 a week steady (sim/gym/README.md, PROGRESS.md ~1644) | **Right**, slightly understated | Blast furnace power is free with a recharger (~+50 a week). Perfect ingots (150, 2 per blast smelt) need quality ≥ 200–300, i.e. neural cores (1 500, Hypertech smuggler). The STATION engineer (tier-3 MATERIAL window, 750–999 + day) can absorb the ingots | +50–100 a week; ore stays the wall |
| 10 | Dream dust: not built (contraband 2) (SECOND-ENGINE.md) | **Right for now** | Twice caffeine's value per input, but every unit in stock or in the window is contraband 2: inspections, DRUG_TRAFFICKING on every sale | — (risk, not economics) |
| 11 | Printer: pays only over a longer game (ARCHETYPES-NEW.md) | **Right** | Power is free now (8 a night), which is small next to ~1 390 of chips | small |
| 12 | Water buyers are rare in the gym (PROGRESS.md ~1556) | Already retracted (PROGRESS.md ~1448) | The water shopper shows as "Spacer". The stockpiler is a window pull, not a random visitor (#1) | — |
| 13 | SEARCH-LABELS.md: the engine and fountain forks were dropped ("no water engine") | **Moot** | Dropped because no route ran a water engine, not because they lost | — |

Also checked and still right: kotton and nutrifruit are poor cash crops. Kotton takes 6 nights for 12–13 fiber at 46
and frail. Nutrifruit is infertile and needs a 40 seed per harvest. The docs' plant table is off for both (see
"Doc errors").

## Measured

### Demand ceilings (`supply_probe`: free units in stock each morning from day 30, Board-free, 16 seeds)

| Free each day | Wins (95 % CI) | Median win day | Δ last day | Δ NW day 100 | Sold | Per unit |
|---|---|---|---|---|---|---|
| none (reference) | 0/16 (0–19 %) | — | — | — | — | — |
| 12 pharma pills (92 %) | 6/16 (18–61 %) | 125 | +0.0 ± 19.9 | +15 412 ± 1 687 | 98 % | 17.1 |
| **24 pharma pills** | **16/16 (81–100 %)** | **90** | −80.6 ± 17.7 | +22 766 ± 914 | 94 % | 17.2 |
| 48 pharma pills | 16/16 (81–100 %) | 83 | −85.8 ± 17.4 | +23 900 ± 1 079 | 94 % | 17.2 |
| **24 near-pharma pills (85 %, an unmodded lab)** | **14/16 (64–97 %)** | 118 | −43.6 ± 24.2 | +17 790 ± 1 539 | 93 % | 13.0 |
| 48 near-pharma pills | 16/16 (81–100 %) | 104 | −61.8 ± 19.5 | +20 894 ± 1 489 | 93 % | 12.9 |
| 8 coffee beans | 9/16 (33–77 %) | 125 | −13.2 ± 26.2 | +14 348 ± 2 121 | 92 % | 25.9 |
| 3 Master Vintage bottles | **16/16 (81–100 %)** | **69** | −100.7 ± 17.5 | +22 661 ± 1 235 | 100 % | 168 |
| 5 Master Vintage bottles | 16/16 (81–100 %) | 69 | −102.0 ± 17.3 | +40 174 ± 1 905 | 100 % | 171 |
| 8 Master Vintage bottles | 16/16 (81–100 %) | 62 | −108.6 ± 17.4 | +26 702 ± 2 623 | 100 % | 171 |

McNemar against the reference (b wins it alone, c loses it alone): 24 pharma b = 16, c = 0; 24 near pharma b = 14,
c = 0; 8 beans b = 9, c = 0.

**Δ last day:** a buyout ends the run early, so a negative Δ last day is a faster finish.

**Demand ceiling for pills:** about 28–30 a day. At 48 a day the stock grid fills and ~30 a day are injected and sold.

**Who buys:**
- Caffeine pills are MEDICAL + HOUSEHOLD_GOOD and a chemical product.
- So both the window's MEDICAL tier 3 (hospital admin, 750–999 + day) and HOUSEHOLD tier 3 (upper maid, 500–649 + day)
  buy them, and so does every SUBSTANCE window buyer (`ChemicalProductHelper.WillAcceptNarcotic`).
- One buyer comes per window type per day (`ShowcaseHelper.GetDisplayCaseType`); that is the real cap.
- Wine is SUBSTANCE (+ LUXURY at Reserve and up). In the probe the upper maid bought most of it, so wine demand is
  not limited to the Hedonist.

### Engines with their costs (`supply_probe_capex`, `supply_probe_cost_per_day`, `supply_probe_cash_per_day`)

**How the cost model works.** The probe now charges like an engine:
- It starts on the first day from day 30 when cash covers capex + the rent due + 500.
- It pays the capex then, and pays the daily cost every morning after.
- `supply_probe_cash_per_day` credits a deterministic night machine's takings (the fountain) each morning.

The capex is the full kit bought at once at hard prices: upgrades ×1.5, Dr. Jackson +50 %, retired suppliers +30 %.

**What it does not model.**
- **Space and window slots.** Probe goods sit beside the counter trade, so they compete with it for stock space and
  window slots as a real engine's output would. But nothing is spent on the space itself.
- **Buying the kit in stages** starts an engine earlier than one lump sum would.
- **Taking cash out of stock.** This lowers the start day.

**The arms and their costs:**

| Arm | Capex | Daily cost |
|---|---|---|
| `c85x24`: an unmodded caffeine lab on 5 tap coffee bays | 5 300 | 40 |
| `c92x24`: the same plus one finisher module | 5 700 | 40 |
| `c85x12` | 4 000 | 25 |
| `f431`: the fountain at attractiveness 100, Base + HQ only | 4 750 | 75 |
| `f312`: the fountain at attractiveness 60 | 4 450 | 55 |
| `f431L`: the landlord's fountain | 2 600 | 120 |
| `c92x24L`: the farmer start's bays and RETIRED_FARMER already owned | 3 500 | 40 |

What the capex buys:
- **Caffeine arms:**
  - Lab and power: RETIRED_CHEMIST 1 500, RETIRED_FARMER 525, POWER_HIJACK 450, a recharger 225, COMMERCIAL_POWER 525.
  - Bays and water: 4 bays at 390, WATER_TRADER 450 + GUTTERFLOW 375.
  - Running cost: packs, filters and seeds.
- **`f431`:** fountain 1 800, the three water upgrades 1 350, a hydroreed bay 390, a 20 L tank 525, 17 common
  electronics, a recharger, POWER_HIJACK. The running cost is ~7.5 L a night at ~10 a litre.
- **`f431L`:** the landlord's fountain, so no fountain price. The daily cost includes ~45 of the extra rent.

Board-free, 16 seeds, 200 days:

| Arm | Wins (95 % CI) | Endings | Engine start day (median; never) | Δ last day | Δ NW day 100 | Engine sales a run |
|---|---|---|---|---|---|---|
| reference | 0/16 (0–19 %) | rent 8, repo 5, alive 3 | — | — | — | — |
| `c85x12` | 0/16 | alive 12, rent 2, repo 2 | 70; 2 never | +24.2 ± 18.6 | +699 ± 1 050 | 16 184 |
| `c85x24` | 0/16 | alive 11, rent 5 | 72; 5 never | +24.8 ± 18.3 | +666 ± 1 580 | 23 343 |
| `c92x24` | 0/16 | alive 10, rent 6 | 74; 6 never | +24.5 ± 18.4 | +1 065 ± 1 526 | 27 591 |
| `f312` | 2/16 (3–36 %) | alive 11, rent 3, mortgage 2 | 74; 3 never | +25.7 ± 17.5 | +977 ± 938 | 30 810 |
| `f431` | 4/16 (10–49 %) | alive 9, rent 3, mortgage 4 | 75; 3 never | +25.1 ± 17.3 | +2 010 ± 1 428 | 41 349 |
| **`f431L`** | **8/16 (28–72 %)** | mortgage 7, cash 1, alive 7, rent 1 | 70 (earliest 58); 1 never | +20.5 ± 16.1 | +3 058 ± 1 931 | 48 811 |
| `c92x24L` | 6/16 (18–61 %) | mortgage 6, alive 9, rent 1 | 70; 1 never | +25.1 ± 17.2 | +2 366 ± 1 518 | 45 433 |
| `w6x2c`: 2 Master bottles a day, capex 2 500, 45 a day | 7/16 (23–67 %) | mortgage 7, alive 8, rent 1 | 70 (earliest 58); 1 never | +24.8 ± 16.7 | +2 432 ± 1 661 | 38 627 |
| `w6x3c`: 3 Master bottles a day, capex 4 000, 70 a day | 7/16 (23–67 %) | mortgage 7, alive 7, rent 2 | 70; 2 never | +24.4 ± 16.9 | +3 664 ± 1 456 | 52 223 |

McNemar against the reference: `f431L` b = 8, c = 0 (χ² 6.1); `w6x2c`, `w6x3c` b = 7, c = 0 (χ² 5.1); `c92x24L`
b = 6, c = 0 (χ² 4.2).

**What the wine arms' cost covers.** Bays, hydroreed water, taps and free power. The daily cost is a bottle (12) and
a yeast (10) per bottle, plus filters. Every bottle sold, at ~170.

**Two things this shows.**
- **Every costed engine survives longer.** 9–12 of 16 are alive on day 200, against 3.
- **Board-free, the old verdict's fourth wall is right: capital before day 70.** Even a 2 600 kit is first affordable
  around Georgio's payback (day ~70), so the engine runs ~4 weeks less than the free probe. By then the buyout costs
  ~28 k and the rent ~1 000 a week.
  - The cheaper kits win 6–8 of 16 by the mortgage around day 195. That is the best Board-free result recorded: the old
    research had 1 win in 720 Board-free runs.
  - What would turn this into a route is an engine bought in stages from day ~20:
    - taps after the permit;
    - one hydroreed bay;
    - the landlord's fountain on day 43;
    - commons as they come.
  - The gym can test that only once an executor builds it.

With the Board (the `appraiser` route as shipped plus the engine, 16 seeds, 300 days):

| Arm | Wins | Median win day | Δ win day | Engine start day | Engine sales a run |
|---|---|---|---|---|---|
| `appraiser` | 16/16 | 97 | — | — | — |
| + `f431` (fountain, full kit at 4 750) | 16/16 | **83** | **−14.0 ± 4.5** | 49–64 | 12 095 |
| + `c92x24` (pharma lab on tap bays, 5 700) | 16/16 | **90** | **−10.1 ± 4.5** | 51–67 | 11 126 |
| + `w6x3c` (3 Master bottles a day, 4 000) | 16/16 | **76** | **−19.2 ± 5.5** | 38–70 (median 54) | 12 203 |

With the Board the engines pay for themselves before the buyout. This reverses SECOND-ENGINE.md's "+38 days with the
Board". That figure measured our failing lab and dry bays (85–198 pills, ~1 900 of sales a run), not the engine.

**Confirmed on 60 fresh seeds (8600001–060, same setup):**

| Arm | Wins (95 % CI) | Median win day | Δ win day | Δ NW day 100 |
|---|---|---|---|---|
| `appraiser` | 60/60 (94–100 %) | 97 | — | — |
| + `w6x3c` cellar | 60/60 (94–100 %) | **76** | **−17.0 ± 2.2** | +1 467 ± 674 |
| + `f431` fountain | 60/60 (94–100 %) | **83** | **−12.4 ± 1.6** | +1 189 ± 609 |

The caffeine lab was not re-run at 60 seeds; its −10.1 ± 4.5 days is a 16-seed screen.

### Lab (`fence-lab verdict.*`, new in `sim/lab/VerdictExps.cs`)

**`verdict.fountain`.** `VendingFountainHelper.HandleNight` on hard, with 20 L of each grade in its slot. Takings a night:

| Attractiveness | 15 | 20 | 40 | 60 | 75 | 85 | 100 |
|---|---|---|---|---|---|---|---|
| Base / HQ / Pure honestly | 72 | 111 | 215 | 356 | 422 | 503 | **617** |
| Base / HQ, Pure slot empty | 72 | 86 | 190 | 312 | 350 | 431 | **431** |
| Litres poured, Base / HQ / Pure | 0.75 / 0.45 / 0 | 1.0 / 0.45 / 0.25 | 2.75 / 0.7 / 0.25 | 2.75 / 2.45 / 0.45 | 3.5 / 2.45 / 0.75 | 4.75 / 2.75 / 0.75 | 4.75 / 2.75 / 2.0 |

- HQ water in all three slots sells for the same cash as honest grades, because the bottle takes its slot's label.
- But it tracks 375–3 000 ml a night of mislabel (fraud), which becomes crime at 2 500 ml.
- Each common electronic adds 5 attractiveness (`VendingMachineHelper.UpgradeAttractiveness`), at 33 base. 17 of them
  take the fountain from 15 to 100.

**`verdict.hqpipeline`.** Free rust tap water through a filter pitcher into a tank. Then a water tablet a litre and one
night. Then an unupgraded purifier:

| Tank | After filter + tablets + 1 night | After 1 purifier night | Filter use |
|---|---|---|---|
| 6 L | 94.57 % | **99.01 % (HQ)** | 33 dur/L |
| 10 L | 95.89 % | **99.01 % (HQ)** | 22 dur/L |
| 20 L | 92.41 % | 97.25 % (98.43 % after 4 nights: the budget stops scaling at 10 L) | 20 dur/L |
| 6 L, no tablets | 94.87 % | 96.51 %, HQ on night 4 | 42 dur/L |

The purifier's budget scales with volume up to 10 L (`MachinePurifier.PurifyContainer`). So one purifier makes
≤ 10 L of HQ a night from tablet-treated tap water. A 400-durability filter (25) lasts ~12–20 L.

**`verdict.hydroreed`.** A hydroreed seed in a bay purifies the bay's own water slot by a percentage each night
(`PlantWaterQuality.Purify`, innate `purifying` rank 4). No tablets, no purifier:

| Water in the slot | Night 0 | 2 | 4 | 6 | 8 |
|---|---|---|---|---|---|
| raw rust, 20 L | 92.80 % | 95.77 | 97.30 | 98.09 | 98.48 |
| filtered rust, 20 L | 96.95 % | **98.36 (Base)** | **99.08 (HQ)** | 99.44 | 99.60 |
| filtered rust, 6 L | 96.76 % | 98.33 | 99.14 | 99.55 | 99.74 |

Because the rate is a percentage, a 20 L tank cleans as fast as a 6 L one. Two hydroreed bays with two 20 L tanks
in rotation give ~10 L of HQ a night, enough for the fountain's 7.5 L. The bays use 20 ml a night and draw 8 power
(free with a recharger). On this curve, Pure (≥ 99.90 %) looks reachable in ~2 weeks; that is not measured past
night 8.

**`verdict.hqcrops`.** 24 nights, 4 bays each, on HQ water (`production.crops` covers the other grades):

| Crop | HQ | HQ + 6 nutrient tablets |
|---|---|---|
| coffee bean | 2.00 beans a night (6 per 3 nights; pure + tablets 8, base 5, tap 4) | 2.00 |
| bloomberry | 2.75 a night; conditions −2/−1/+1/+2: 2/68/106/88 (73 % normal or best) | 2.75; 0/42/127/95 (**84 %**) |
| kotton | 1.0 fiber a night | 1.0 |

## Each verdict in detail

### 1. Water (ROUTES.md "Water", PROGRESS.md ~1535–1556, ARCHETYPES)

**Old claim.**
- The fountain sells ~300–600 a week once bought.
- The shop makes 2–3 L of clean water a day; eight purifiers lock up the vessels.
- The best water arm survives +33 days but wins 0/60.

**What was missed.**
- **Supply.**
  - The executor's `purify_raw` puts raw tap water in a purifier. From 91 % the purifier needs ~5 nights a vessel
    (lab `water.purifier`).
  - Two cheaper paths exist:
    - **Filter + tablets:** one purifier night a ≤ 10 L tank.
    - **A hydroreed bay:** percentage cleaning, any volume, no tablets, no cap at 99.01 %.
  - Water tablets come 9 to a box from the scav medical seller and a few random sellers. They are too scarce to treat
    50 L a week, so the hydroreed bay is the volume path.
- **Channel.**
  - **The fountain's takings are fixed by attractiveness** (`HandleNight`): 431 a night at 100 without Pure.
  - **The stockpiler:** a showcased water type worth ≥ 350, or a Pure-majority case, sends the stockpiler (500–599 + day)
    as that window type's buyer every day from day 14 (`ShowcaseHelper.CreateClientByStockType`). He takes Pure or HQ.
  - The old water arms showed water only at the counter. The stockpiler needs the water type to win a window slot.
- **Attractiveness.**
  - "Common electronics come slowly" was a buying policy, not a supply limit. They are a 33 MATERIAL that any seller
    may carry.
  - The vending machine competes for them: the gym spends 17 on the vending machine first.
- **Power.** Free (§2).

**Corrected.**
- Fountain at 100, Base + HQ: 431 a night, ~3 000 a week gross.
- Running cost ~75 a night (filters ~2/L; the bays and the fountain draw free power).
- That is ≈ 2 500 a week net, more than the ~2 200 a week the Board pays.
- Measured with costs:
  - −14.0 ± 4.5 days on the Board route.
  - Board-free 4/16 with the full kit bought, 8/16 with the landlord's fountain. The kit is first affordable on
    day ~70, so the fountain runs at full attractiveness for under half the game.

**What an engine must do.**
1. Buy the taps after the permit.
2. Run 2 hydroreed bays on 20 L tanks of filtered tap water (Jackson's water stock with WATER_TRADER: tank 50 %, more
   with his investment).
3. Feed common electronics to the fountain before the vending machine.
4. Keep the Base and HQ slots full every evening.
5. Showcase one ≥ 350 HQ jug from day 14 when a window slot is free.
6. Retest with a strip before labelling HQ, or the label stays stale (`WaterFeatureHelper.UpdateWaterFeature`).
7. Never put a lower grade in a higher slot: it is fraud or endangerment.

### 2. Power (PROGRESS.md ~1139 "the cells are the real cost"; every engine's margin)

**Old claim.** Power costs ~1.65 a unit as cells. It is ~200 a week for the furnace, bays and vending machine, and
13 a night per bay.

**What was missed.**
- Machines draw from their own battery slot (`MachineHelper.TryDrawCyclePower`).
- Cells are rechargeable capital, not fuel:
  - `MachineRecharger.Recharger` charges every cell in its 8 slots +3 a night, or +6 with COMMERCIAL_POWER, while the
    store has power.
  - `PlayerStore.UpdatePowerState`: POWER_HIJACK (300 × 1.5) keeps power on with no service fee
    (`StoreService.UpdateCost`, `(day/10 + 5) × 2` a night on hard otherwise).
  - A powerblock (200) charges adjacent cells and machine batteries +2/+4 a night (`FurnitureItemDirectory.CreatePowerBlock`).
- The gym never buys any of them (`PlayerRole.DrawsStorePower` only notices one already owned).

**Corrected.**
- One recharger + POWER_HIJACK, 675, runs 24 units a night.
- With COMMERCIAL_POWER, 1 200 in all, it runs 48 units a night: enough for 4 bays and an evaporator, or a blast
  furnace + 2 bays + the fountain.
- The gym's Board-free reference spends 3.5 k on cells in 171 days, so the swap pays back in ~5–6 weeks. With the
  Board it is pure saving.

**What an engine must do.**
- Buy POWER_HIJACK and a recharger once there are ≥ 3 powered machines.
- Rotate charged cells into machine slots each morning. Better: put a powerblock beside each bay row.
- Stop buying energy credits from Jackson.

### 3. Caffeine lab (SECOND-ENGINE.md "four walls")

**Old claim.** The four walls:
- Inputs come by the handful.
- Bays need water and power the shop lacks.
- Modules never reach the counter.
- The 1 500 unlock comes too late.

**What was missed, wall by wall.**
1. **Inputs.**
   - Coffee replants from its own beans.
   - On free gutter/rust tap water a bay still makes 4 beans per 3 nights (`production.crops`; the health cut floors at
     ×0.5).
   - On HQ water it makes 6, on pure + tablets 8.
   - The old arm's bays sat dry or unpowered: 131 harvested where two would make ~370.
2. **Water and power.**
   - Tap water is enough for coffee.
   - Power is free (§2).
3. **Modules.**
   - True for the Retired Chemist. His machines are offered first, and `PlayerStore.TryAddDirectSellingItemToTable`
     skips offers once the front counter is full. That is the real game's code, not a gym artifact.
   - But Dr. Jackson with RETIRED_CHEMIST offers a chem module at 50 % + 25 % a visit, +50 % each with JACKSON1/2
     (`StoreClientList`, inventor stock).
   - No module was ever fitted in 16 runs. That points at the gym lab's buying, since ~3 Jackson visits per run should
     offer ~2.
   - One finisher module turns near pharma (14) into pharma (18): +29 %.
4. **Unlock and timing.**
   - 1 500 is real, and with the lump-sum cost model the engine starts on day ~70.
   - The farmer start (bays and RETIRED_FARMER owned) cuts ~2 000 off it.
5. **The wall the doc did not name: alcohol.**
   - The extraction needs ~0.75 ml of alcohol per ml of mush (R7 reads the ratio against each second's 15 % chunk):
     ~34 ml a bean.
   - A common chemical pack (350 ml of alcohol) does ~10 beans.
   - Packs come one a week from the Retired Chemist, plus 25 % + 25 % from the scav general seller and 8 % barter
     (`StoreClientListSpec`, `StoreClientList.CreateScavGeneral`, `BarterOfferList`).
   - 24 pills a day needs ~4.4 beans a day, ~3 packs a week.
   - The solvent-recovery module (275, offered 75 % per chemist visit) returns alcohol from the evaporator in
     proportion to its quality.

**Corrected.**
- Demand for 24 pills a day exists: 14/16 near pharma, 16/16 pharma.
- Costed, the engine takes 10 days off the Board route's win.
- Board-free it wins only when its kit is cheap: the farmer start's 3 500 kit wins 6/16; the full 5 700 kit 0/16,
  because that kit is first affordable on day ~74.
- The binding inputs are alcohol and the start date, not beans.

**What an engine must do.**
1. Coffee bays on tap water, ≥ 1 per bean a day wanted (HQ water from the hydroreed bay: 1.67).
2. Filter through kotton **and** a water filter. The old lab's pills sold at ~10 (street grade) because it filtered
   through kotton only.
3. Buy a finisher module from Jackson at the first offer.
4. Buy the solvent-recovery module plus quality modules for the evaporator.
5. Stock packs from every source; run ~0.75× alcohol to mush.
6. Free power.
7. Sell through the case: a MEDICAL or HOUSEHOLD window slot brings the tier-3 buyers once the pills there are worth
   ≥ 350.

### 4. Wine (PROGRESS.md "Wine levers", WINE.md §6, OWNER-HARD-GAME.md)

**Old claim.**
- `wine_min_age: 10` costs 14 days.
- Racks and more bays are neutral.
- Wine sells ~125.

**What was missed.**
- **Grade.**
  - The executor sold 110 bottles a run at a mean of 124, mostly Production. The gym's bays run on ~98.4–99.2 % water.
  - On HQ water + tablets, 84 % of berries are normal or best (lab).
  - The fermentation score adds each berry, +1 for two or more, +1 for yeast and +1 for HQ/Pure water
    (`WineHelper.StartFermentation`). So **three** normal-or-better berries on HQ water make Master Vintage (175).
    Two make at most Reserve.
- **Horizon.**
  - `AgableHelper.OnAge` adds +3 base value a night to every finished bottle, uncapped, scaled by grade:
    Production +3.75 a night (3 % of 125), Master +5.25.
  - The hold lost because the executor refused the Hedonist while bottles aged (`PlayerRole.KeepWhy`), not because
    aging is worthless. Hold some bottles, not all.
- **Rack.**
  - It holds 6 bottles, not 24: a 1×4 bottle in a 6×4 grid (`WINE_RACK_BOTTLE_OFFSET` has 6 entries).
  - At 24 power a night (free with §2), a rack of Master adds 31.5 a night. Small.

**Corrected.**
- A bay on HQ water: ~19 berries a week, ~16 good, ~5 Master bottles, ~900 a week.
- Demand probe: 3 free Master bottles a day win 16/16 Board-free by day 69, and 8 a day all sell (~7 a day reach
  the stock).
- Costed (3 a day for a 4 000 kit and 70 a day), the cellar takes **19.2 ± 5.5 days** off the Board route: 97 → 76,
  the largest gain measured here.

**What an engine must do.**
1. HQ water in bays and bottles (hydroreed).
2. Three good berries per bottle, plus yeast.
3. Moldy berries sold as food.
4. Hold a buffer of bottles to age, but never refuse a buyer while holding.
5. No rack unless the bottles are Master.

### 5. Rats and the feed bay (SECOND-ENGINE.md "Rats on a feed bay", ARCHETYPES-NEW.md ~128)

**Old claim.** The rats return about what their feed cost: 2 483 of meat against 2 744 of feed. A feed bay earns no
more than selling its crop.

**What was missed.**
- The 2 744 was feed bought from sellers for colonies that starved out (113/120 died by day ~27), so the ratio
  measures a dying colony.
- With innate traits, fillerweed grows in **1 night**, 5 a harvest, and is immune to contaminants (`PlantList`:
  fast_grower, contaminant ×0). So it runs on free tap water (451 ml a night) and free power.
- Its "raw value ~50 a night" assumes the FOOD window slot has room for 35 fillerweed a week at 11. It is a filler.

**Corrected (paper; the livestock-chains agent should measure).**
- One feed bay is ~750 kcal a night, ~10 rats' upkeep at ~65 kcal.
- Butchering at 400 g after ~8 days gives roughly a rat a night: raw meat 159 (FOOD + LUXURY) + scrap.
- That is ~150–200 a night if the meat sells (private chef, tier-3 FOOD, 400–499 + day).

**What an engine must do.**
- Fillerweed bays on tap water feeding the dispenser.
- Cage rows from ingots.
- Butcher at 400 g.

### 6–7. Coffee beans and crop bays (SECOND-ENGINE.md)

The bean probe was run at 4 a day; at 8 a day it wins 9/16.

The crop arms lost because the bays were dry or unpowered and power was bought. A tap bay's bean is free water and
free power.

Raw beans are the low-capital version of §3:
- No 1 500 unlock and no alcohol.
- ~26 a bean, against ~70 when pressed into near-pharma pills.

### 8. Tap water for the bays (PROGRESS.md ~1532)

**Old claim.** Forcing the taps costs 63 days.

The cost was 11 arrests for OPERATING_WITHOUT_LICENSE: the taps were bought before the permit, the same trap
SECOND-ENGINE.md hit with the lab. The treated water was also raw-purified, the slow path.

**What an engine must do.** Taps after the permit, filtered, through a hydroreed bay.

### 9. Furnace (sim/gym/README.md 2026-11-10, PROGRESS.md ~1644)

**Right.** Ore by phone is 12 every 5 days, so ~0.8 blast smelts a night. A blast smelt on 3 ore + flux makes 2 Very
High ingots (150) for ~40 of inputs.

**What it understated.**
- The 16 charge a smelt (free, §2).
- The station engineer window buyer (750–999 + day) for selling ingots by the dozen.
- Perfect ingots (150 each) need module quality ≥ 200 (`IngotPurityHelper`), i.e. neural cores. Not realistic on hard.

### 10–11. Dream dust, printer

Right as judged. Dream dust is a risk question: every unit is contraband 2 in stock, the window and every sale. The
printer is still chip-bound.

## Gym fidelity found on the way

- **Front counter.** The headless counter skipping offers when full is the game's own
  (`PlayerStore.TryAddDirectSellingItemToTable`), not a gym artifact. A real player clears the counter between visits
  just as the executor should.
- **Phone orders.** sim/gym/README.md's "Still lost: phone orders to suppliers" is stale; `ore_phone` works.
- **Not modelled by the executor:** rechargers, powerblocks, POWER_HIJACK, COMMERCIAL_POWER, hydroreed bays, the
  water stockpiler pull, 3-berry bottles.
- **Engine start dates.** The engine-cost probe charges the kit as one lump sum. A staged build starts engines
  earlier, so the engine rows are a floor.

## Doc errors found (fix in docs/re)

- **WINE.md:**
  - §4 says the rack holds "6 × 4 bottles". It holds 6.
  - §0 implies aging is "multiplied by its grade". Right, but the per-night base is a flat +3 with no innate-trait
    term.
- **PRODUCTION.md §1 plant table** ignores innate traits for every crop but bloomberry (`PlantList.Register`):

  | Crop | Code | Docs |
  |---|---|---|
  | nutrifruit | 7 per 2 nights, ~0 seeds back | 5 per 3 nights, 3 seeds back |
  | kotton | 13 per 6 nights, 250 ml | 9 per 4 nights, 176 ml |
  | fillerweed | 5 per night, 451 ml | 4 per 2 nights, 287 ml |

- **PRODUCTION.md** "power ~1 credit per unit": buying is ~1.65 on hard; charging is free beyond the service fee.

## Instructions for the engine builders

**chem-engine (../fence-chem-engine).**
1. Beans from your own coffee bays on tap water.
2. Free power: POWER_HIJACK + a recharger (+ COMMERCIAL_POWER).
3. Both filters, kotton then water filter. Street-grade pills mean the water filter was skipped.
4. Buy Jackson's chem modules, finisher first.
5. Alcohol is your wall. Count packs, run ~0.75× alcohol to mush, get the solvent-recovery module.
6. Put the pills in a window slot (MEDICAL or HOUSEHOLD) worth ≥ 350 so tier-3 buyers come.
7. Target 24 pharma pills a day. That rate wins 16/16 Board-free by day 90 if supplied.
8. Buy the lab after the permit.

**quality-max (../fence-quality-max).**
1. **Wine is the largest measured gain:** −19.2 ± 5.5 days on the Board route for 3 Master bottles a day.
   - The `appraiser` route already owns two bays and a cellar. What it lacks:
     - HQ water in the bays and in every bottle;
     - **three** normal-or-better berries a bottle, with yeast;
     - ~2 more bays.
   - Sell at any age. Never let a held bottle refuse a buyer.
2. Water for everything should come from hydroreed bays on 20 L tanks of filtered tap water: HQ in 4 nights, no
   tablets, no 99.01 % cap.
3. The fountain is the water sink: 431–617 a night at attractiveness 100, −14.0 ± 4.5 days on the Board route.
   Common electronics go to it first.
4. Showcase one ≥ 350 HQ jug for the stockpiler.
5. Furnace: free power and the station engineer; don't chase Perfect.

**livestock-chains (../fence-livestock-chains).**
1. The feed wall is solvable: fillerweed is contaminant-immune and 1-night with its innate traits.
2. Re-measure rats with a feed bay before trusting "rats break even".
3. Printer: free power changes little.

**hidden-audit (../fence-hidden-audit).** Power is the biggest non-production lever found:
- The gym buys ~3.5 k of cells in a Board-free run where 675–1 200 of upgrades would make power free.
- Check the vending machine's power too (2 + capacity upgrades a night).

## Reproducing

```
make gym-build && dotnet build sim/lab -c Release
dotnet sim/lab/bin/Release/net10.0/fence-lab.dll verdict.fountain     # also verdict.hqpipeline, verdict.hydroreed, verdict.hqcrops
fence-gym run -seed 85000NN -days 200 -human player -difficulty hard -start apprentice -perks vending+short_sleeper \
  -directives <appraiser.json without "appraisal", plus e.g.
  {"supply_probe": {"caffeine_pill": {"per_day": 24, "purity": 92}}, "supply_probe_capex": 5700, "supply_probe_cost_per_day": 40}>
```

`supply_probe` is gym-only and off by default. It also takes `wine_bottle` with `wine_score` (a finished bottle of
that fermentation score) and the three cost keys above. With them absent the gym is unchanged.
