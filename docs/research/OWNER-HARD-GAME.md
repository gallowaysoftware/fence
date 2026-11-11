# The owner's hard game vs the gym's best hard policy

Why did the owner's hard run (Rock Bottom, perks proficient_scavenger + alert + short_sleeper +
bad_lease) reach a net worth of 30.5k on day 100 when the gym's best hard policy reaches ~8k? The
owner asked three things: am I investing more in production, is the vending machine better than
it looks, and do I just buy low and sell high better?

**Short answer.** Vending explains none of it. Buying cheaper explains none of it either: the owner
pays *more* of the estimate than the gym does. The edge has four parts, in this order:
appraisal fees (a channel the gym never opens), better wine, a higher resale markup (from selling
to Upper Levels tippers and into shortages), and more trade volume. Rent and upkeep take back
about a fifth of the gain. Most of the 3.5× net-worth gap is equipment and stock counted at
estimate. The owner held *less cash* than the gym until day ~98.

## Data and method

- **Owner.** Eleven saves of the run (days 103–113, live slot + `~/.local/share/fence/save-backups`).
  Their `runStats` hold game-wide counters: counter sales, stock spent, vending, Wild and
  Dr. Jackson spend, rent, scavenging, plus net worth and cash for every day from day 1. The bridge
  log `var/log/s-c2c09368fb3aa1ed.jsonl` covers days 110–112 item by item (62 sales, 73 purchases,
  every offer with its estimate). Its sales total 4,062, which matches the saves' counter delta
  exactly. Days 1–102 have only daily cash and net worth, so the weekly breakdown covers days
  103–112. The split by source is from the three logged days and is noisy: five wine bottles, one
  at 485.
- **Other cash income** = the change in cash minus every flow the game counts. On the logged days it
  is appraisal fees (627 and 459 from two Scavenger appraisal visits) and one aug bounty (~199).
  A day-109 appraisal visit shows in the game log too.
- **Gym.** Executor `f75fcb4` (`var/sim/gyms/search-f75fcb46a361`), `-difficulty hard -start apprentice
  -human player -perks vending+short_sleeper -directives sim/lab/scripts/directives/hard_income_mortgage.json`.
  The weekly flows are the 60-seed fidelity set (`var/sim/exp/fid/h60`, seeds 1100301–360, days
  99–112). The splits by source, the buy/sell ratios and the trade mechanics come from four new
  traced runs (`var/sim/exp/owner`, seeds 1500001–004, days 100–112). The four match the 60 on
  trade sales and vending; their wine sales run ~20 % lower.

## 1. Income per week, late game

| per week | owner, days 103–112 | gym, days 99–112 (60 seeds) | Δ |
|---|---|---|---|
| counter sales | 8,589 | 3,633 (trade 2,418, wine 1,191, water 24) | **+4,956** |
| vending | 748 | 713 | +35 |
| other cash: appraisal fees, bounties | ~2,230 | ~−120 | **+2,350** |
| stock and production inputs bought | −5,178 | −2,718 | −2,460 |
| rent | −1,800 | −486 | −1,314 |
| services (janitorial, power, converters) | ~−445 | −355 | −90 |
| **operating cash** | **4,144** | **667** | **+3,477** |
| invested (Wild 707, Dr. Jackson 884) | −1,591 | −11 | |
| mortgage | 0 | −667 | |
| change in cash | +2,551 | −13 | |

Customers are about the same: ~10.3 a day for the owner, 10.1 for the gym. Over the whole game
the owner's counters read: sales 59.1k, stock 44.3k, vending 9.1k, Wild 16.6k cash, Dr. Jackson
9.7k (12 machines), rent 11.0k, permit 2.0k, and Georgio net +1.9k (the gym makes the same loan).
Cash income outside these counters totals ~24k, ~1.5k a week averaged over the run. Appraisal
fees are most of it on every day that can be checked.

## 2. Counter sales by item source (owner days 110–112, gym days 100–112, per week)

| source | owner sales | owner cost basis | owner margin | gym sales | gym cost basis | gym margin |
|---|---|---|---|---|---|---|
| produced: wine | 2,763 | 131 (bottles) | ~95 % before inputs | 1,191 | inside inputs | — |
| produced: hydroponic berries | 359 | 0 | — | 240 | inside inputs | — |
| produced: furnace ingots | 350 | 0 | — | 552 | inside inputs | — |
| supplier: Red Imp beer (cases and cans) | 1,176 | ~665 | 43 % | ~0 | | |
| bought: resale (customers, wholesale lots, suppliers) | 3,726 | 2,340 | **37 % (1.59× cost)** | 1,496 | 1,251 | **16 % (1.20×)** |
| scavenged, unknown | 506 | 0 | 100 % | 68 | 0 | 100 % |
| asset sale (furnace + module, 280+180 paid) | 597 | 1,073 | loss | 0 | | |
| **total** | 9,478 (3-day rate; 10-day rate 8,589) | | | 3,633 | | |

Production inputs: the owner bought ~0.9k a week of bottles, berries, yeast and water supplies
over the logged days. The gym spends 1.13k a week on inputs (berries, bottles, furnace charge
and ore). Net of inputs, production earns ~2.6k a week for the owner and ~0.9k for the gym. The
owner gets more for each bottle: ~237 on average from Upper Levels Hedonists (sample of five),
against ~100–130 at finish in the gym. The owner has the DNA manipulator with its precision module,
five hydroponic bays and two AgeWell racks, and sells old bottles (one made around day 75 went
for 485).

## 3. Buy low, sell high?

| | owner (days 110–112) | gym (4 seeds, days 100–112; shares from 60 seeds) |
|---|---|---|
| price paid ÷ estimate (value-weighted) | **89 %** (scavenger lots 85 %, Red Imp 66 %, suppliers 100 %) | **86 %** (wholesale lots 82 %, suppliers 97 %, single items 100 %) |
| price received ÷ estimate | 100 % | 98.5 % |
| seller visits bought from | **15/17 (88 %)** | 58 % |
| buyer visits sold to | **10/13 (77 %)** | 49 % |
| sold value carrying Tips (+15 %) | **64 %** | 20 % |
| sold value carrying a market-event bonus | 27 % | 16 % |
| realized resale markup over purchase price | **1.59×** | 1.20× |
| stock on hand | ~200 items + 3 large storage bays | 89–144 items, grid 162–219/240 cells |

So no: the owner does not buy lower. Both sell at the estimate. The owner's markup is higher
because of when an item is sold and who buys it. Medical goods bought at ~85 % were sold to the
Upper Level Hospital Supplier during the medical shortage (+50 %) with Tips (+15 %). Wine and beer
went to Upper Levels Hedonists. The owner buys from almost every seller, holds a wide stock in
storage bays, and waits for the matching buyer or event. Attractiveness is 700 against the gym's
~550, from all three renovations and both showcases, which brings the richer client tier.

## 4. Vending

| | owner | gym (60 seeds) |
|---|---|---|
| income to day 112 | 9,106 | median 9,088 |
| days 103–112 | 1,069 (107/day; 0 on 3 of 10 nights) | ~102/day |
| attractiveness / capacity | 100 / 8 rows | 100 / 9–12 rows |
| stocked with | odds and ends: snacks, tampons, yeast, toilet paper, a cigarette pack, 4 raw meat; 0–5 items most nights | executor fills it from stock |

The machine sells at night (`VendingMachineHelper.HandleNight`) whatever eligible items it holds, at
current value with the retail markup plus a vending bonus, provided its power cell has charge. Both
machines are at the attractiveness cap, so income is limited by what is stocked. The owner's
machine sat empty three nights out of ten. The note in `f6b54b7` that the gym had no vending
machine predates the `vending` perk. With it, vending is a wash. It is not the owner's edge.

## 5. Investment

The owner kept reinvesting: cash stayed under the gym's median until day ~98 (day 91: 390 against
~4,080), while net worth ran ahead in equipment and stock. On day 113, net worth 35.6k = cash 8.7k
+ ~27.9k of items at estimate. About 15k of the items is equipment: current converters 4.2k,
large storage bays 2.6k, water tanks 1.6k, hydroponics 1.5k, machine bay 1.2k, DNA manipulator and
module 2.0k, racks 0.7k. Wild cost 16.6k in cash (21 upgrades, plus favor) against the gym's ~2.9k.
Dr. Jackson cost 9.7k.

Days are dated where a save or the log shows them. The rest are estimated from item-id order
mapped onto the gym's id-per-day curve (±10 days):

| day | asset | cost | plausible payback |
|---|---|---|---|
| 5 (board's offer) | joined the appraisal board (join day not recorded; Master rank by day 103) | 85 × difficulty multiplier (170 on hard) | **~2.2k/week at Master**: base fee 100 + 21–25 % of each estimate, 55 % daily visit chance rising after a missed day (`AppraisalRankHelper`); repaid in days |
| ~20 | furnace (+ junk module) | 280 + 180 | ingots ~0.35k/week; sold back on day 112 for 256 |
| 45 | business permit | 2,000 | the gym buys it too |
| ~60–66 | three hydroponic bays, DNA manipulator (Wild PLANT_DNA), smuggler module | 345–390 each; PLANT_DNA 1,250 base | berries plus wine inputs: the wine line |
| ~73 | fourth bay, first AgeWell rack | 375, 438 | aged wine |
| ~90 | DNA precision module, hydroreed filter | 1,250, 131 | higher wine score; part of the ~2.6k/week production net |
| ~100 | second AgeWell rack | 438 | |
| 110 | fifth bay (Retired Farmer) | 390 | |
| 110, 112 | two more current converters, water tank, mirage projector, 2 modules | 450, 375, 228, 500, 160 | power for the machines; converter service costs 46–58/day |
| unknown | Wild: BREWER + shareholder, VENDING_MACHINE, renovations 1–3, showcases I–II, POWER_HIJACK 1–3, taps, Retired Farmer, Winemaker, Water Trader, informant, marketplace requests | 10.95k base | beer ~0.5k/week net (payback ~4–5 weeks); vending ~0.75k/week; renovations and showcases through attractiveness and tips (not separable) |

Dated large cash drops: days 55–56 (−2.9k, net worth up, so machines), day 70 (−2.9k, net worth
−2.5k, so Wild), days 74–75 (−3.0k, machines), day 91 (−3.9k), day 102 (−2.5k, Wild).

## Where the edge comes from

The gains over the gym add up to ~6.2k a week before the owner's higher rent and upkeep
(−1.4k). Measured operating cash is +3.5k a week, not +4.8k, because the split comes from three
days that sold at a 9.5k-a-week rate (8.6k over ten days), and because equipment bought over the
counter (a hydroponic bay, current converters) counts as stock:

| component | +/week | share |
|---|---|---|
| appraisal fees (and bounties) | +2.35k | **38 %** |
| production, net of inputs (mostly wine) | +1.7k | **28 %** |
| resale margin (1.59× vs 1.20× on the owner's volume) | +0.77k | 12 % |
| Red Imp beer line | +0.5k | 8 % |
| scavenged and other goods sold | +0.44k | 7 % |
| resale volume (2.5× the gym's resale at the gym's margin) | +0.37k | 6 % |
| vending | +0.04k | 1 % |

The appraisal share rests on the 10-day cash residual plus two logged visits. The production,
beer and resale split rests on three days. Read these shares to ±10 points.

## Levers for the executor

1. **Join the appraisal board and serve appraisal clients.** The gym declines it every run
   ("[Don't join]" is the first enabled choice, taken as the default). It has no handling for
   `APPRAISAL_SERVICE` clients, and none came in 60 + 4 runs. The lever: join (170 on hard),
   keep the certificate in the showcase (`ShowcaseHelper` only rolls the appraisal client while a
   certificate is displayed), and estimate well: within 5 % of the item's true base value earns
   the best fee grade and the most promotion progress (`AppraisalServiceHelper`), so the executor
   needs a good estimate from player-visible information. This is the largest and cheapest lever. Check first that
   the headless game runs the appraisal inspection flow.
2. **Hold stock and match it to buyers instead of flipping it.** Buy from more sellers (88 % vs 58 %),
   hold medical, household, wine and beer until an Upper Levels buyer (Tips) or a shortage event
   comes, and stop dumping at the estimate. The binding limit is the full 240-cell grid. Storage
   bays are the way out, but the gym cannot stage items from inside a bay yet (2fde96a), so that
   comes first.
3. **Wine quality, not wine volume.** Earlier tests found more bays neutral and holding wine to age
   (`wine_min_age: 10`) harmful. The owner's edge is the price per bottle: the DNA manipulator with
   the precision module, AgeWell racks, and Hedonist buyers. Test PLANT_DNA + precision with
   Hedonist-targeted selling.
4. **Red Imp beer: sell full cases.** A case bought at 95 sells for 169 whole to a Hedonist,
   against 8 × 18 + 22 = 166 split. The beer phone and shareholder tested neutral, so check what
   the gym sells and to whom before rejecting the line.
5. **Not levers:** cheaper buying (the owner pays more of the estimate) and vending (equal; the
   owner even leaves the machine empty).
