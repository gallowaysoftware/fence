# Trading styles on hard

Every hard route shares one counter policy: buy wholesale lots at up to 88 % of the estimate, sell to whoever
asks, haggle only when the stated odds favour it. This note asks whether the shop can trade in other ways and
still win, and whether those ways look different on a stream. Production, start and perks stay fixed (the
`appraiser` route: apprentice, vending + short sleeper, wine + furnace, the Appraisal Board, cash buyout); only
how the shop buys and sells changes.

**Result.** Six styles win hard 60/60 on the screening seeds and 60/60 on a fresh confirmation block, and each
leaves a fingerprint the reference does not: a medicine shop, a household-goods shop, a hoarder who sells into
shortages, a wholesale flipper, a dealer who never haggles, and a shop that refuses stolen goods. They are new
routes (below). The specialists and the hoarder are 1½–3 weeks slower than the generalist; the other three are
within noise of it. Aggressive haggling loses every run to a lynch mob. Without the Appraisal Board every style
loses (0/60), and none closes the Board-free gap.

## Mechanics that set the limits (from the game code)

- **Hard sells at the estimate.** The retail markup is 0 on hard (15–20 on easier rules), and single items are
  offered at exactly their estimate, so a single item bought to resell earns nothing unless something lifts its
  price later. The margin is the wholesale lot's discount (the reference pays ~81 % of the estimate) and the
  stolen-goods discount.
- **What lifts a price**, each an additive percentage on the item: a market event (`NegociationData` on the
  event: on one sampled run, weapons +25 on 32 of 91 days, materials +30 on 9, medical, food and water +50 on
  6–7 each, household −40 on 22); Upper Levels tips (+15, `UL_TIPS`, from Upper Levels 20: the reference ends
  at ~19, so tips almost never apply before the buyout); neuroactive perfume (+15 on a sprayed buyer, −15 on a
  sprayed seller's goods and so on the wholesale price; five uses a bottle; Eleanor sells one on day 27 and later
  leaves her self-refilling bottle; the contraband pheromone version is ±25); a selling markup the buyer accepts
  (`add_markup_*`, at the odds the bargain panel states).
- **Haggling costs reputation.** A trade moves faction rep by its value/400, scaled by the bargain; Lower Levels
  below −100 send the mob (`LLLYNCH`).
- **The calendar shows known events four days ahead** (`GetActiveEventOnDay` excludes unknown ones); the
  executor already buys types an event will raise.

## Levers added (`sim/gym/Style.cs`, default off)

| Lever | What it does |
|---|---|
| `specialty: {types, lot_share, buy_at_estimate}` | Buys only goods of these types: a wholesale lot only when they are `lot_share` of its estimate (default 0.5), a single lot only when every staged item is one; from a mixed seller only they are staged; the display case concentrates on them. |
| `speculate: {types, ul, events, max_days, buy_at_estimate}` | Goods of these types are kept from ordinary buyers until an Upper Levels buyer with tips unlocked comes, a market event lifts their price, or `max_days` pass (default 21); a shop short of cash sells them anyway. |
| `perfume: {buyers, sellers, min_sale, min_lot, buy_at_estimate}` | Keeps neuroactive perfume as a tool and sprays it (the bottle's own slot action through `Chore.Act`) on a buyer whose staged goods are worth 200+ and fit the budget after +15 %, and on a seller whose lot, 15 % cheaper, is within the buying limits. Gym only: the live autopilot does not spray. |
| `buy.no_singles` | Wholesale lots only (engine inputs excepted). |
| `sell.hot_max_heat: N` | Stages stolen goods at heat N or less for any buyer but Security. |

Off, the gym's summaries are byte-identical to the base build (main a957b56) on hard seeds 2620901–906
(`appraiser`, `smelter`, `rock_bottom`, `outlaw`) and normal 2620951–952 (`appraiser`, `lean`). The demand trace
(`FENCE_DEMAND_TRACE`) now records each sold item (price, estimate, cost and days held, tips), each bought item's
heat, every counter choice and the day's stock and events; it is read-only (a traced run's summary is identical
to an untraced one).

## Results

Hard, apprentice, `-perks vending+short_sleeper`, 1000-day horizon; the arms are the `appraiser` route plus the
change. Wins are buyouts (Wilson 95 % CI). Paired Δ is the mean difference in win day against the reference on
seeds both won, ± 1.96 SE. NW is the median net worth on days 50 and 98.

**Confirmation block, 60 fresh seeds 7110001–060** (build v3 of this branch; the final build reproduces these
runs exactly):

| Style (route) | Wins | Median win day | Paired Δ win day | NW d50 / d98 | Losses |
|---|---|---|---|---|---|
| reference (`appraiser`) | 60/60 (94–100 %) | 98 | — | 6 632 / 28 585 | — |
| medical specialist (`apothecary`) | 60/60 (94–100 %) | 105 | +11.6 ± 6.0 | 5 841 / 27 212 | — |
| household specialist (`hardware_store`) | 60/60 (94–100 %) | 105 | +13.8 ± 6.5 | 5 906 / 26 807 | — |
| speculator (`speculator`) | 60/60 (94–100 %) | 112 | +23.2 ± 9.5 | 5 539 / 26 205 | — |
| wholesale flipper (`bulk_buyer`) | 60/60 (94–100 %) | 98 | +3.5 ± 5.4 | 6 376 / 28 958 | — |
| never haggles (`fair_dealer`) | 60/60 (94–100 %) | 98 | +1.9 ± 4.8 | 6 122 / 28 716 | — |
| no stolen goods (`clean_shop`) | 60/60 (94–100 %) | 98 | +3.1 ± 5.3 | 6 700 / 28 252 | — |
| perfume (no route) | 60/60 (94–100 %) | 98 | +0.7 ± 4.2 | 6 603 / 28 910 | — |

**Fingerprints** (same block, days 1–98, pooled; wine, water and engine inputs left out of the trade figures).
Sales and bought are a week; paid is the share of the estimate paid; wholesale the share of purchases in
multi-item wholesale lots; hot the share bought stolen; stock the owned items held at day end (kit included);
markup the resale price over the purchase price; event the share of sales carrying a market-event rise; UL the
Upper Levels share of sales; uplift the price paid over the staged value (perfume shows here); bargains the share
of buyers sold a markup / offered a discount, and discounts demanded of sellers a week; the mix is resale by
category (%).

| Style | Sales | Items/day | Bought | Paid % | Wholesale % | Hot % | Stock | Markup | Event % | UL % | Uplift | Bargains | Resale mix |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| reference | 1 270 | 6.4 | 769 | 81 | 92 | 6.2 | 38 | 1.32 | 12 | 10 | 98.7 | 0 / 5 / 5.4 | medical 24, substance 17, food 17 |
| `apothecary` | 940 | 4.7 | 360 | 84 | 79 | 15.5 | 34 | 1.26 | 12 | 12 | 100.4 | 0 / 2 / 6.4 | **medical 67**, substance 15, household 8 |
| `hardware_store` | 790 | 4.7 | 195 | 96 | 39 | 3.7 | 32 | 1.18 | 11 | 12 | 98.7 | 0 / 2 / 6.7 | **household 81**, medical 10, luxury 6 |
| `speculator` | 889 | 4.4 | 606 | 80 | 93 | 7.2 | **46** | 1.31 | **22** | 12 | 97.6 | 0 / 13 / 4.4 | medical 26, substance 19, food 16 |
| `bulk_buyer` | 1 361 | 6.7 | **969** | 81 | **97** | 8.2 | 39 | 1.31 | 12 | 14 | 98.8 | 0 / 6 / 6.5 | medical 43, substance 17, food 11 |
| `fair_dealer` | **1 554** | **7.2** | **1 166** | 81 | 92 | 8.4 | 41 | 1.22 | 12 | 15 | 98.6 | **0 / 0 / 0** | medical 30, substance 17, food 15 |
| `clean_shop` | 1 238 | 6.3 | 729 | 80 | 93 | **0** | 37 | 1.29 | 12 | 9 | 98.7 | 0 / 5 / 4.8 | medical 22, food 17, substance 15 |
| perfume | 1 281 | 6.4 | 786 | 81 | 92 | 6.7 | 39 | 1.34 | 13 | 11 | 100.8 | 0 / 5 / 5.4 | medical 27, substance 17, food 15 |

What each style does differently:
- **Specialists** trade a third less, and their resale is two-thirds to four-fifths one category. The medicine
  shop takes medical lots that carry stolen goods (15 % of its buying is hot, held until cool); the
  household shop buys mostly single items (wholesale 39 %) at nearly the estimate, so its margin is thinnest.
- **The speculator** holds a fifth more stock and sells twice as much into market events (22 % of sales against
  12 %), but turns its stock slower; the week it gives up is the cost of the capital standing on the shelf.
- **The wholesale flipper** buys a quarter more, all of it in lots, and the medical sign pulls its mix toward
  medicine.
- **The fair dealer** never demands a discount, so no seller walks out: it buys half as much again as the
  reference and sells a fifth more, at a lower markup (1.22).
- **The clean shop** buys nothing stolen and loses nothing measurable for it.

**Screening block, 60 seeds 7100001–060** (build v2/v3), every arm tried:

| Arm | Change | Wins | Median win day | Paired Δ win day | NW d50 / d98 | Losses |
|---|---|---|---|---|---|---|
| `ref` | — | 60/60 | 98 | — | 6 182 / 28 524 | — |
| `spec_med` | `specialty` MEDICAL (lot share 0.5) | 60/60 | 105 | +10.7 ± 6.2 | 5 771 / 27 800 | — |
| `spec_med_wide` | `specialty` MEDICAL, lot share 0.3 | 60/60 | 98 | +3.0 ± 4.6 | 6 059 / 28 731 | — |
| `spec_house` | `specialty` HOUSEHOLD_GOOD | 60/60 | 105 | +14.0 ± 8.4 | 5 835 / 27 444 | — |
| `spec_mat` | `specialty` MATERIAL | 60/60 | 112 | +19.7 ± 8.8 | 5 730 / 26 354 | — |
| `specul` | `speculate` medical, food, household, materials, water; 21 days; bought at estimate | 60/60 | 115.5 | +39.9 ± 13.7 | 4 978 / 24 977 | — |
| `specul_nobuy` | the same, not bought at estimate | 60/60 | 119 | +24.4 ± 7.7 | 5 066 / 24 481 | — |
| `specul_fast` | medical, food, water, materials; 7 days | 60/60 | 108.5 | +15.5 ± 7.9 | 5 594 / 26 764 | — |
| `wholesale` | `buy.no_singles`, wholesale up to 95 % | 60/60 | 98 | −1.4 ± 4.0 | 6 468 / 28 680 | — |
| `wholesale_signs` | + medical sign in the case, `sell.min_pct` 50 | 60/60 | 98 | −0.3 ± 4.1 | 6 515 / 29 164 | — |
| `picker` | single items only (wholesale 0 %, singles 85 %) | 60/60 | 105 | +14.7 ± 7.5 | 5 802 / 27 072 | — |
| `haggler` | `bargain: ev_positive` (markups at good odds) | 60/60 | 91 | −4.4 ± 4.4 | 6 989 / 29 340 | — |
| `haggle_sell` | + `bargain_buy: aggressive` | 60/60 | 98 | −0.9 ± 3.9 | 6 572 / 29 222 | — |
| `fair` | `bargain` and `bargain_buy: never` | 60/60 | 98 | −0.6 ± 4.7 | 6 497 / 29 368 | — |
| `markup_shark` | `bargain: aggressive` (25 % markup on every buyer) | **0/60** | — | — | 5 547 / 7 350 | mob 60 |
| `shark` | `bargain` and `bargain_buy: aggressive` | **0/60** | — | — | 5 374 / 6 164 | mob 60 |
| `clean` | `buy.hot_stock_cap: 0` | 60/60 | 98 | +1.9 ± 4.6 | 6 534 / 29 305 | — |
| `hotfence` | `sell.sell_hot: true` | 60/60 | 94.5 | −2.8 ± 4.1 | 6 702 / 28 608 | — |
| `coolfence` | `sell.hot_max_heat: 10` | 60/60 | 98 | +2.0 ± 2.4 | 6 318 / 28 475 | — |
| `perfume` | `perfume: {}` | 60/60 | 98 | +1.2 ± 3.7 | 6 462 / 29 028 | — |

Fingerprints of the arms that did not become routes (days 1–98): `spec_mat` sells 645 a week, 75 % materials
and 23 % household, markup 0.84 (it resells below cost); `specul` holds 48
items and its event share is 22 %; `picker` buys 272 a week, a third of it food; `haggle_sell` sells a markup to
3 % of buyers; the sharks sell a markup to two buyers in three, their Lower Levels rep reaches −103 and the mob
comes on day ~58 in every run; `hotfence` buys 7.2 % hot against 6.4 %, a difference too small to show.

## Failures and why

- **Aggressive haggling** (`shark`, `markup_shark`): 0/60. A 25 % markup on every buyer sells to two in three
  but each one costs Lower Levels rep; the shop reaches −103 and is lynched on day ~58, every seed. EV-gated
  markups (`haggler`) are taken so rarely (the stated odds seldom favour them) that the style is the reference
  with a different label.
- **Holding stock** (`specul*`) wins but slowly: on hard the rent clock rewards turnover, and the tips the
  owner's game lived on need Upper Levels 20, which the cash sprint reaches only at its end. A one-week hold on
  the four +50 categories is the best of three.
- **Narrow categories** (`spec_mat`, `spec_house`): the fewer sellers carry the type, the slower the shop.
  Materials are also the furnace's product, so the shop's ingots compete with what it buys.
- **Single items only** (`picker`): every single item is offered at its estimate on hard, so this shop earns on
  events alone; it wins but two weeks late.
- **Perfume**: works (the uplift moves from 98.7 to 100.8 % of the staged value) but five sprays a bottle are
  too few to show; no route.
- **Selling hot** (`hotfence`, `coolfence`): the executor already cools stolen goods in the smuggler's bay, so
  selling them warm changes little; no fence route beyond the existing `outlaw`.

## Without the Appraisal Board (`appraisal` unset; seeds 7100001–060)

| Arm | Wins | NW d50 / d98 | Losses |
|---|---|---|---|
| `ref` | 1/60 (day 210, mortgage) | 3 917 / 6 756 | rent 45, repo 14 |
| `spec_med` / `spec_med_wide` | 0 / 0 | 3 248 / 4 798; 3 389 / 4 954 | rent 50–51, repo 5, arrest 4–5 |
| `spec_house` / `spec_mat` | 0 / 0 | 3 253 / 4 768; 2 850 / 3 872 | rent 45–52, arrest 5–15 |
| `specul` / `specul_nobuy` / `specul_fast` | 0 / 0 / 0 | 2 604–3 270 / 3 780–5 538 | rent 43–53, arrest 5–16 |
| `wholesale` / `wholesale_signs` | 0 / 0 | 3 711–3 850 / 6 238–6 590 | rent 43–46, repo 14–17 |
| `haggler` / `fair` | 0 / 0 | 3 604–3 727 / 6 636–6 906 | rent 35–47, repo 13–24 |
| `clean` / `hotfence` / `coolfence` | 0 / 0 / 0 | 3 672–3 766 / 6 458–6 750 | rent 46–53, repo 7–13 |
| `perfume` | 0 | 3 885 / 6 956 | rent 42, repo 18 |
| `shark` | 0 | 2 761 / 2 758 | mob 60 |

The fingerprints hold without the Board (the specialists' mixes, the speculator's event share and the fair
dealer's volume are the same within a point or two), but no trading style closes the gap: the best (`perfume`,
`fair`) end day 98 at most ~200 of net worth above the reference, against a Board-free shortfall of ~1 000–1 500 a
week. The specialists and the speculator lose sooner and add arrests (the permit goes unbought while the cash
lags). Trading style is flavour on top of an income engine, not a substitute for one.

## The routes

Each is `appraiser.json` with the style's levers, in `sim/lab/scripts/directives/routes/<name>.json`:

| Route | Levers | Persona |
|---|---|---|
| `apothecary` | `specialty: {types: [MEDICAL], lot_share: 0.3}` | honest-broker |
| `hardware_store` | `specialty: {types: [HOUSEHOLD_GOOD]}` | magnate |
| `speculator` | `speculate: {types: [MEDICAL, FOOD, WATER, MATERIAL], max_days: 7}` | shark |
| `bulk_buyer` | `buy.no_singles`, `buy.wholesale_max_pct: 95`, `restock_missed: false`, `window_signs: [sign_medical]`, `sell.min_pct: 50` | magnate |
| `fair_dealer` | `bargain: never`, `bargain_buy: never` | honest-broker |
| `clean_shop` | `buy.hot_stock_cap: 0` | armourer |

## Reproducing

`fence-gym run -seed N -days 1000 -difficulty hard -start apprentice -perks vending+short_sleeper -human player
-directives <file> -no-trajectory -summary FILE`, with `FENCE_DEMAND_TRACE=DIR` for the fingerprints, six
processes at a time. The arms' summaries and traces are in the session scratchpad (`runs/s1`, `runs/c1`), not
in the repository.
