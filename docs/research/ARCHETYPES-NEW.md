# Production archetypes the gym could not play (2026-11-11)

ARCHETYPES.md lists engines from the code; ROUTES.md lists what wins on hard. This note covers the production
archetypes in between: the ones the gym could not play, or played unfaithfully, until now. Each was checked
against the game code the gym runs (the decompile, never committed), given a lever where it was worth one, and
measured on hard.

**Method.** Hard, 1000-day horizon, the gym of this branch. Every arm is a route file with one lever changed
(the `appraiser` route, the cash buyout around day 90, unless the table names `smelter`, `gunsmith` or the rancher
start), paired with that route on 60 fresh seeds 4700001–060; the confirmation block is 4710001–060.
"Appraisal off" drops `appraisal: join` from the same file and plays 30 seeds to day 200, to see each engine as
the main income. Δ day is the mean paired difference in
win day over the seeds both arms won, Δ NW the paired net-worth difference on day 50 / day 84, all ± 1.96 SE.
Arms and helper scripts are throwaway (`var/`); the directives are the route file plus the lever named.

## Results

On the appraiser route every arm still wins 60/60: the Appraisal Board carries hard, so these engines decide how
fast, not whether. Medians are win days; the appraiser's own median is 90 on 4700001–060 and 97 on 4710001–060.

| Archetype (lever) | Start | Wins | Median | Δ day | Δ NW d50 / d84 | Confirmation (4710001–060) | Verdict |
|---|---|---|---|---|---|---|---|
| **Rancher: rats** (`engine: wine+furnace+rats`), against the appraiser | rancher | 60/60 | 90 | −3.6 ± 4.3 | +468 / +1 060 | 60/60, median 83, **−8.4 ± 4.7**, +977 / +1 701 | **new route `rancher`**: as fast as the appraiser or faster |
| rats against cutting the kit rats up on day 1 | rancher | 60/60 | 90 | −4.3 ± 4.7 | +538 / +1 014 | **−9.3 ± 4.5**, +749 / +2 098 | the engine itself pays: +681 to +937 of meat over feed a run |
| Pharma phone (`pharma: "phone"`) | apprentice | 60/60 | 90 | +2.3 ± 5.0 | −470 / +65 | 60/60, median 90, −2.2 ± 3.2, −164 / +316 | neutral; **new route `pharma`** for variety |
| Pharma + shareholder | apprentice | 60/60 | 97 | **+6.7 ± 3.4** | −1 200 / −1 997 | — | costs a week: 3 750 does not pay back in 13 weeks |
| Beer phone (`beer: "phone"`) | apprentice | 60/60 | 97 | +4.7 ± 6.2 | −320 / −549 | — | neutral to slightly slower |
| Beer + shareholder (gifts now stowed) | apprentice | 60/60 | 97 | **+5.7 ± 3.8** | −1 034 / −1 312 | — | costs most of a week |
| Beer + shareholder, alcohol held for Upper Levels (`hold_for`) | apprentice | 60/60 | 97 | +7.8 ± 6.0 | −1 059 / −1 932 | — | holding does not help |
| Chem phone, no contraband trade | apprentice | 60/60 | 97 | +5.4 ± 5.4 | −386 / −931 | — | nothing to trade (now gated, below) |
| Miner shareholder (`miner_shareholder`), on the `smelter` route | apprentice | 60/60 | 90 | +2.3 ± 3.3 | −501 / −443 | — | neutral, as before |
| 3D printer (`printer: "run"`), on the `gunsmith` route | gunsmith | 60/60 | 111 | **+15.8 ± 7.0** | +589 / −1 896 | — | costs two weeks: chips are bought in the saving window |

**Appraisal off** (30 seeds 4700001–030, 200 days; the shop mostly loses hard without the Board, so the measure is
days and net worth): no engine changes survival. Pharma phone ±0 (Δ NW d100 −4 ± 581), beer + shareholder
−517 ± 1 110, pharma + shareholder +8.3 ± 9.0 days and +1 220 ± 1 141 by day 200. On the rancher start, rats against
the day-1 cull +697 ± 955 at day 100, and caging or cutting the loose rats is worth +2 128 ± 913 (the loose-rat
penalty, below). As the main engine the rats roughly break even on meat against feed (2 483 against 2 744 a run).

## Fidelity fixes found on the way

1. **Supplier gifts were thrown away.** A free first delivery (the Red Imp case, Dale's six injectors, Ashish's
   eight dust) and every shareholder dividend arrive already ours on the front counter, and the game clears that
   counter at night (`EmporiumEntry.ClearFrontInv`, called from `PlayerStore`'s night). A player picks them up; the
   gym only picked up the furnace's ore. So the 2026-11-10 test that found the beer phone and its shareholder
   "neutral to slightly negative" lost the free case and every weekly dividend case. The executor now stows them
   (gym and live autopilot alike), only while a supplier lever is on, so no existing route changes.
2. **The Rat Wrangler's rats ran loose in the stock grid.** The kit puts six rats in the stock grid, not in the
   cage. The night tick works on any grid: loose rats breed, eat the shop's food and drop feces into the stock
   grid until they die there. Every earlier rancher screen played that way. On hard, the appraiser policy on the
   rancher start wins 13.5 ± 5.7 days later than on the apprentice start (7.6 ± 4.6 on the confirmation seeds);
   cutting the six rats up on day 1 alone recovers it (+0.7 ± 4.4 and +0.9 ± 4.5 against the apprentice). Unchanged unless `engine` names `rats`.

## Per archetype

### Red Imp beer (BREWER, BREWER_SHAREHOLDER)

From code: the introduction (200; 300 on hard) puts the rep on the phone; the first call brings one free case,
later calls a case of eight beers (value 145) at 20 % off every 4 days, two cases once 400 has been spent. The
shareholder upgrade (1 450; 2 175 on hard) adds another 15 % off and one free case a week
(`ShareholderHelper`). A case sells whole or bottle by bottle for the same money (145 whole against 8 × 18 + the
empty case), so "selling full cases" changes nothing; the owner's 169 a case is the retail markup plus an Upper
Levels tip.

Measured: the beer phone buys ~25 cases a run at ~121 each and sells ~19–24 at ~137; the margin is thin
because the courier's 20 % (35 % as a shareholder) comes off the same estimate the counter sells at. The
shareholder's case a week (~11 gifts a run) does not repay 2 175 + 300 in a 13-week run. The 2026-11-10 verdict
stands on a faithful gym: neutral to slightly negative on hard. The owner's "good money" is a longer game (the shareholder pays back in
~15 weeks of dividends) and Upper Levels tips the gym does not chase.

### Pharma (PHARMA, PHARMA_SHAREHOLDER)

From code: Dale's introduction (300; 450 on hard) brings six pink injectors free; then 5 injectors (value 50) a
call at 20 % off, 10 once 500 is spent, every 7 days. The shareholder (2 500; 3 750 on hard) adds 15 % off and 5
injectors a week. Pink injectors are legal medical narcotics; addicts, medical buyers and the window's
substance buyers take them.

Measured: ~81 injectors bought a run at ~40, ~69 sold at ~51 (+11 each); with the shareholder ~42 gift
injectors and ~110 sold for 5 634. The phone alone is neutral on hard (screen +2.3 ± 5.0, confirmation
−2.2 ± 3.2, both 60/60), so it ships as the `pharma` route; the 3 750 shareholder costs a week. The lever orders only when the cash
covers the lot over the rent and the grid has room.

### Chemist's dust (CHEMIST, CHEMIST_SHAREHOLDER)

From code: Ashish (250; 375 on hard) is a Black Market courier who sells street dream dust (contraband 2, value 18
at 60–80 % purity, ~31 at market with the contraband markup) at 20 % off and buys chemical supplies; 8 dust a call,
16 once 400 is spent, every 4 days; 8 dust free on the first call; the shareholder (1 440) adds 8 dust a week.
With the route's defaults (no contraband bought or sold) the executor phoned, the courier came, and nothing
traded: `chem: "phone"` cost 5.4 ± 5.4 days for nothing. Paid orders now wait for `buy.contraband` (selling also
needs `sell.sell_contraband`); at ~6 a unit of margin and DRUG_TRAFFICKING evidence per sale it was not measured
further.
The real chemistry money is the lab (below), not the courier.

### Miner shareholder (MINER_SHAREHOLDER)

From code: 900 (1 350 on hard) for 6 ore a week and 15 % more off Caulson's deliveries; the furnace already
stowed this dividend, so the earlier test was faithful. New lever `miner_shareholder: true` (with `ore_phone`).

Measured on the `smelter` route (ore phone, two furnaces): bought around day 22 in 59 of 60 runs, +2.3 ± 3.3 days,
neutral. Six ore a week is ~90 of value; it does not repay 1 350 before the cash buyout.

### Rats (Rat Wrangler start, RETIRED_RANCHER)

From code (`HusbandryHelper`, `AnimalCage`, `MachineFeedDispenser`): rats mature at 3 days, a pregnancy takes 3
nights, a litter is 3 ± 1; a rat reaches 200 g at 3 days, 350 at 5, 400 at 8 and 450 at 11 (half speed below 400
hunger), and dies of age from 14 days (× a longevity gene). The cleaver kills, then cuts by weight: raw meat (159,
6 cells) per 400 g, small raw meat (65) per 200 g, morsels. A rat burns ~65 kcal a night and a newborn starts at a
quarter of its 500; the cage's dispenser blends any food but canned food and pellets into stored calories and
tops each rat up to 85 %. The cage is 10 × 3; each ingot adds a row (to 8) and 20 health, scrap mends it, and
it wears by a quarter of the rats' cells a night (escapes below 20 health). Over 55 % full, fertility falls a
fifth; over 70 % each rat kills a smaller one 5 % of nights; over 90 %, 15 %. Feces and corpses over a quarter of
the cage start disease. Immunity is below its threshold at birth, so ~13 % of rats a night fall ill.

New lever: `engine` containing `rats` (`sim/gym/Rats.cs`, compiled into the mod). The binding input is food, not
space: sellers offer ~450 kcal a day of cheap food (mostly nutrifruit; traced over 60 days on 4700001), enough for
about one breeding pair and its litters. A larger herd starves (the first versions kept three females and lost
most of the colony by day 20). The default keeps one pair, spares a successor near old age, grows the cage with
ingots, cuts growers at 400 g and, when the food would not last two nights, the heaviest growers from 300 g.

Measured in the route (4700001–060): ~31 born, ~13 died (starvation, disease, age), ~37 cut up, ~77 cuts sold
for ~2 480 against ~1 540 of feed bought. Most colonies die out within a month (113 of 120 runs, median day 27):
each litter starts a quarter fed, the dispenser is often empty, and newborns starve on their second night at zero;
a breeder lost to disease or age then ends the line. Sparing a reserve grower of each sex and cutting only already
hungry growers (tried on 12 seeds) did not keep them alive, since the food is the limit. Still, the rats add 4–9
days over cutting them up (−4.3 ± 4.7 and −9.3 ± 4.5), and the rancher start with them wins as fast as the
apprentice appraiser. Without the engine the rancher start is a
week or two slower (the loose-rat penalty above). A feed bay (fillerweed or nutrifruit seeds in a hydroponic bay)
would lift the food wall; it is not built: the rats return about what their feed is worth, so a feed bay earns
no more than selling its crop (docs/research/SECOND-ENGINE.md).

### The 3D printer (Gunsmith start, RETIRED_GUNSMITH)

From code (docs/re/RECIPES.md §1): chips are never used up; 24 progress a night, 8 power. The money is in one-night
chips: improved springs (2 scrap → 4 × 20), the silencer (5 scrap → 2 × 55, offered on rolls), the makeshift RDS
(1 plastic + 1 electronic → 4 × 25); gun parts from ingots are break-even. New lever `printer: "run"`
(`sim/gym/Printer.cs`).

Measured on the `gunsmith` route: ~19 prints a run (springs and makeshift RDS; scrap and electronics bind),
~1 790 of output from ~745 of inputs, but ~1 390 of chips bought, which delays the cash buyout by 15.8 ± 7.0 days.
The printer pays only over a longer game or with chips already owned; the lever stays off in every route.

### Not modelled

- **Chemistry lab** (RETIRED_CHEMIST, 1 500 on hard; the cartel only on the drug-dealer start, which ends on day 8
  in the gym because the cartel quota is not played). docs/re/CHEMISTRY.md §6: a dream cap (23) becomes ~5.5
  lab-pure dust (42, ~72 at market) and a coffee bean (28) ~8 legal caffeine pills (24) — but only with four chem
  modules on every machine (250 each from the retired chemist's rolls), a plate held in a 40–49 °C window, measured
  acid and base pours, two filters and 1–4 evaporator nights a batch. That is a multi-step lab procedure with
  per-second reactions, not a morning chore; it is the richest unmodelled engine and the largest to build. Since built as `lab: "caffeine"`
  and measured: docs/research/SECOND-ENGINE.md.
- **Aquaculture** (AQUACULTURE, Aquafarmer start). `AquacultureHelper` is ~2 500 lines (water quality, filters,
  power, species stages, roe). The fish eat industrial fish feed, which no visiting seller's stock samples contain (the kit's two
  bags only), so a fish engine would hit the same food wall as the rats.
- **High-quality water for the bays**: already a lever (`wine_purifier`, ROUTES `water_baron`); nothing new.
