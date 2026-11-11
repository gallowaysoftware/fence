# Hard routes: a menu for the stream

Fifteen hard-difficulty policies that win, each with a different start, perk set or engine, so a stream
does not show the same run every night. Each one is a directives file in
`sim/lab/scripts/directives/routes/<name>.json`; its `_route` key (ignored by the gym and the autopilot)
gives the start and perks to pass on the command line:

```
fence-gym run -seed N -days 1000 -human player -difficulty hard -start <start> -perks <perks> \
  -directives sim/lab/scripts/directives/routes/<name>.json
```

Each route but `rancher` and `pharma` (2026-11-11, hard only) also has a challenge version, `routes/challenge/<name>.json` (below, "The challenge routes").

**What they share.** Every route is the appraisal incumbent (`hard_income_mortgage_appraisal.json`) with
one thing changed. The Appraisal Board's fees (~49k a run, the same in every route) pay for the shop. Since
2026-11-11 every route tells the loan shark "later", drops the insurance from day 56 and buys the shop for cash
(`buyout_by_day: 140`, `buyout_save: ["insurance"]`, `buyout_save_from: 56`; below, "Every route buys for
cash"), so a winning run ends around day 84–112 instead of 175, with the mortgage as the fallback when the
savings stall. The routes differ in what the shop does with the rest of its day: what it makes, what it sells,
which faction likes it, and how close it runs to the edge.

## The menu

Wins and the median win day are the cash policy on 60 fresh hard seeds 2910001–060, 1000-day horizon (main
a79bcd7 + the `buyout_save` lever, 2026-11-11), with the mortgage-era median (seeds 2202001–060) in parentheses.
The net-worth columns are that mortgage-era measurement (main 1722b2a + the module levers, 2026-11-10; ΔNW is
the paired net-worth difference against the appraiser at day 150, ± 1.96 SE); a cash run ends before day 150.
Wins are buyouts; the CI is Wilson 95 %.

| Route | Start / perks | What changes | Wins (95 % CI) | Median win day | NW d50 / d100 / d150 | ΔNW d150 | For the audience |
|---|---|---|---|---|---|---|---|
| `appraiser` | apprentice / vending + short sleeper | — (the reference) | 60/60 (94–100 %) | **98** (175) | 6 891 / 22 530 / 29 026 | — | The reference: a pawn shop that lives off the Appraisal Board, with a wine cellar, a furnace and a vending machine on the side. |
| `watchman` | apprentice / vending + short sleeper | `alarm: "perfect"`, `transmitters: ["sec"]` | 60/60 (94–100 %) | **98** (175) | 6 552 / 23 614 / 36 832 | **+7 211 ± 1 639** | Buys a burglar alarm, tunes it until no thief gets through, cancels the insurance and broadcasts to Security until the police treat the shop as VIP (Security 100 in 52/60 runs, median day 116). |
| `farmer` | farmer / vending + short sleeper | the Retired Farmer start | 60/60 (94–100 %) | **91** (175) | 6 586 / 24 527 / 31 274 | +2 490 ± 1 054 | Starts with hydroponic bays already in the back room: the most wine and the best net worth of the plain routes. |
| `vintner` | apprentice / vending + short sleeper | `engine: "wine"`, `bays: 4` | 60/60 (94–100 %) | **98** (175) | 7 026 / 23 482 / 30 257 | +792 ± 1 151 | All wine, no furnace: four bloomberry bays and a cellar, and nothing smelted. |
| `labeler` | apprentice / vending + short sleeper | `labeler: "per_buyer"` | 60/60 (94–100 %) | **91** (175) | 6 750 / 21 956 / 28 127 | −1 240 ± 1 136 | Reads the fakes under the magnifier, labels ~30 of them honestly, and sells ~5 a run as genuine to buyers who cannot tell. |
| `no_vending` | apprentice / short sleeper + proficient scavenger | no vending perk | 60/60 (94–100 %) | **91** (175) | 6 358 / 20 564 / 28 472 | −1 148 ± 1 144 | No vending machine: more counter trading (54k sold, against 44k) and the most dump nights, and the wounds to show for it (48 door-shut nights a run). |
| `gunsmith` | gunsmith / vending + short sleeper | the Gunsmith start | 60/60 (94–100 %) | **91** (175) | 6 330 / 20 756 / 27 412 | −1 640 ± 1 334 | The gunsmith's kit and Security's goodwill from day one (ends at 65). |
| `outlaw` | fence / vending + short sleeper | the Fence start (Security −30 on hard) | 60/60 (94–100 %) | **105** (175) | 6 340 / 20 913 / 27 504 | −1 898 ± 962 | Starts in Security's bad books and stays there (ends at 10), and wins anyway. |
| `smelter` | apprentice / vending + short sleeper | `engine: "furnace"`, `ore_phone`, `furnaces: 2`, `furnace_eco` | 60/60 (94–100 %) | **84** (175) | 6 018 / 20 526 / 27 399 | −1 991 ± 1 126 | A metal shop: no wine, two blast furnaces fed by phoned ore, ~330 ingots a run and the busiest counter (61k sold). |
| `lean` | apprentice / vending + short sleeper | `engine: "none"` | 60/60 (94–100 %) | **91** (175) | 5 546 / 19 910 / 23 283 | −6 128 ± 1 093 | No production at all: buy, sell, appraise. Proves the Board alone carries hard. |
| `water_baron` | water merchant / vending + short sleeper | `engine: "furnace+water"` | 60/60 (94–100 %) | **112** (182) | 5 496 / 16 413 / 17 860 | −10 938 ± 1 224 | Purifies and bottles water (~16k sold a run) beside the furnace; the poorest winner, a week later, one arrest. |
| `rock_bottom` | rock bottom / vending + short sleeper | the Rock Bottom start | 57/60 (86–98 %) | **126** (182) | 4 778 / 16 108 / 25 590 | −5 733 ± 2 009 | The underdog: starts broke, loses 4 in 60 to the first or second rent (days 14–21), and the survivors win a week late. Since 2026-11-11 it wins 120/120 (below, "Rock Bottom made robust"). |
| `sprinter` | apprentice / vending + short sleeper | `buyout_by_day: 140`, `buyout_save: ["insurance"]` from day 56 | 60/60 (94–100 %) | **98** (175) | 6 383 / 28 158 / — | — | Tells the loan shark "later", drops the insurance from day 56 (burglars take ~1 000 of goods a run, the fee would be ~5 000) and buys the shop outright for cash, eleven weeks before the mortgage would end (60/60 on 2910001–060, days 70–119). The appraiser's policy and kit with the shark's persona; the hard incumbent since 2026-11-11. |
| `rancher` | rancher / vending + short sleeper | `engine: "wine+furnace+rats"` | 120/120 (97–100 %) | **90 / 83** | — | — | The Rat Wrangler start: a cage of rats bred, fed from the dispenser and cut up at 400 g for the counter (~2 500 of meat a run), with the Board, wine and a furnace. Paired win day against `appraiser` −3.6 ± 4.3 / −8.4 ± 4.7 on seeds 4700001–060 / 4710001–060 (docs/research/ARCHETYPES-NEW.md). |
| `pharma` | apprentice / vending + short sleeper | `pharma: "phone"` | 120/120 (97–100 %) | **90 / 90** | — | — | Dale's courier brings five pink injectors a call at a fifth off, sold to addicts and medics (~80 a run): a supplier on screen. Paired win day against `appraiser` +2.3 ± 5.0 / −2.2 ± 3.2 on the `rancher` seeds. |

**On the current main** (55902b7: the outing fix and fair aug detection; this branch's final build, whose alarm
refits worn modules), 60 more fresh seeds 2203001–060: `appraiser` 60/60, day 175, NW 6 682 / 20 234 / 26 211;
`watchman` 60/60, day 175, ΔNW d150 +4 055 ± 1 668, Security 100 in 54/60 runs (median day 117); the alarm alone
(`alarm: "perfect"`, no transmitter) 60/60, day 175, ΔNW d150 **+6 996 ± 1 811**. The transmitter takes room in
the alarm's bay, so the Security race gives back about a third of the alarm's insurance saving (17 of 60 runs
end with the alarm short of performance 50, against 1 without the transmitter). The plain alarm is worth adding
to any route that does not race Security.

## Every route buys for cash (2026-11-11)

Each route got `buyout_by_day: 140`, `buyout_save: ["insurance"]` and `buyout_save_from: 56`. Paired against
the route's previous file (the mortgage at the first offer) on the same seeds, 1000-day horizon, main a79bcd7 +
the lever; screened on 2900001–060, confirmed on 2910001–060:

| Route | Wins before → after (2910001–) | Median win day before → after | Bought for cash | Paired Δ win day (2910001–) | 2900001–: wins, median after, Δ |
|---|---|---|---|---|---|
| `appraiser` | 60 → 60 | 175 → 98 | 60 | −78.4 ± 3.1 | 60 → 60, 98, −79.6 ± 2.6 |
| `watchman` | 60 → 60 | 175 → 98 | 60 | −77.8 ± 3.0 | 59 → 60, 105, −75.2 ± 2.5 |
| `farmer` | 60 → 60 | 175 → 91 | 60 | −87.0 ± 3.0 | 60 → 60, 98, −81.0 ± 3.3 |
| `vintner` | 60 → 60 | 175 → 98 | 59 | −78.2 ± 5.5 | 60 → 60, 94, −81.0 ± 3.1 |
| `labeler` | 60 → 60 | 175 → 91 | 60 | −82.0 ± 2.6 | 60 → 60, 98, −77.6 ± 4.8 |
| `no_vending` | 60 → 60 | 175 → 91 | 60 | −84.6 ± 2.6 | 60 → 60, 91, −82.7 ± 2.3 |
| `gunsmith` | 60 → 60 | 175 → 91 | 60 | −79.1 ± 3.0 | 60 → 60, 98, −76.2 ± 3.2 |
| `outlaw` | 60 → 60 | 175 → 105 | 58 | −66.4 ± 6.0 | 60 → 60, 105, −66.5 ± 5.9 |
| `smelter` | 60 → 60 | 175 → 84 | 60 | −90.1 ± 2.7 | 60 → 60, 91, −84.9 ± 2.9 |
| `lean` | 60 → 60 | 175 → 91 | 60 | −85.3 ± 3.4 | 60 → 60, 84, −83.8 ± 3.6 |
| `water_baron` | 60 → 60 | 175 → 112 | 53 | −57.0 ± 9.5 | 60 → 60, 105, −67.9 ± 6.2 |
| `rock_bottom` | 57 → 57 | 182 → 126 | 38 | −36.2 ± 10.9 | 58 → 58, 126, −49.0 ± 9.1 |
| `sprinter` (old file: `buyout_by_day: 140` only) | 60 → 60 | 112 → 98 | 60 | −30.9 ± 11.6 | 60 → 60, 98, −26.4 ± 9.6 |

Δ is the mean paired difference over the seeds both arms won, ± 1.96 SE. Every route wins as often as before;
rock_bottom's losses are the same early rents on the same seeds in both arms. The insurance cut is what makes
the cash rule pay on every route. `buyout_by_day: 140` alone (screened on 2900001–060) won 60/60 with a median
of 98–112 on ten routes but was *slower* on `water_baron` (median 217 against 175) and `rock_bottom` (210
against 182), and 126 or 154 did not fix them: their savings project short on day 84, so the hybrid signs the
mortgage on days 98–112 and wins on 210–224. Without the insurance fee the four-week gain carries the
projection. `outlaw` was best at 154 without the cut (median 126 against 175) and is at 105 with the cut at
140, so every route uses 140. Without insurance a run loses ~1 000 of goods to burglars between day 56 and the
buyout (33 nights with a loss) and saves ~5 000 of fees.

## Rock Bottom made robust (2026-11-11)

On main befe6c7 the `rock_bottom` route still loses, two ways:

| Seeds (hard, 1000 days) | Losses |
|---|---|
| 2202001–060 (the menu's) | 4: rent on day 14 (2202014, 018, 026) and 21 (2202033), unchanged from the menu |
| 2600001–060 (fresh) | 2: execution on days 125 and 129 (2600031, 058) |

**The executions** are a permit trap. With `"permit": "buy"` the executor buys only when the cash covers the
fee, the rent due and the day's floor, and on the rent-day visits the floor is the rent again. Rock Bottom's cash
lags, so in those two runs it refused the official on every visit (10 times, cash up to 8 700) while
OPERATING_WITHOUT_LICENSE climbed past 1 500. Leaving `permit` unset hands it to the executor's own rule (buy once
the evidence reaches 30 and the fee and the rent are covered): 60/60 on 2600001–060, ΔNW d150 +967 ± 1 241 against
`permit: buy`; the permit is bought on days 30–79 in 59 runs (one on day 100). `permit_evidence: 0` also wins 60/60
(+328 ± 1 107). The early rents are untouched by it (the old seeds lose the same four).

**The early rents.** In the four losses the shop has 150–270 of value in kit and none in goods: from day 8 the buyers
leave with "nothing sellable staged" for days, while the forward rent floor (the rent less a few days of expected
sales) blocks every lot because the cash is under it. The cash cannot reach the rent by being held; the floor only
keeps the shelf empty. Shortfalls on rent day: 43, 149, 115 and 101. Screens over 40 days on 2202001–060 that did not
save any of the four: `engine_reserve_max` 0 or 200, selling harder (`min_pct` 50, `liquidate_min_pct` 65), and no
cash upgrades before day 28 (`upgrades_cash_max` 60). The lever that does is new, `short_rent_buy_pct` (gym,
`sim/gym/README.md`): while the cash is under the floor and rent day is `short_rent_days` or more away, the executor
keeps buying lots at that share of their estimate (down to `short_rent_reserve`, 0) to sell before rent day.

| Arm (with `permit` unset) | 2202001–060 wins | ΔNW d150 | 2600001–060 wins | ΔNW d150 |
|---|---|---|---|---|
| no lever | 56 (rent ×4) | | 60 | |
| `short_rent_buy_pct: 85` (from 3 days out) | 58 (rent 2202018, 026) | +1 544 ± 1 659 | 59 (an arrest, 2600021, day 130) | −164 ± 1 218 |
| **`short_rent_buy_pct: 85`, `short_rent_days: 2`** | **60** | +314 ± 2 320 | **60** | −2 113 ± 2 033 |
| 90, from 3 days out (40-day screen) | 58 | | | |
| 100, from 3 days out (40-day screen) | 58 | | | |

The route now plays the last row: 120/120 against 116/120, median win day 182 on both sets, at a net-worth cost
that is not significant on either set (pooled about −900 ± 1 550). The lever is in `rock_bottom.json` only; off,
the gym's summaries match the previous build on 10 hard (2620901–910, the incumbent) and 10 normal (2620951–960)
seeds, so no other route changes. `water_baron`, the poorest winner in the menu, wins 60/60 on 2600001–060 on this
main (NW 5 778 / 17 394 / 19 187, median day 175), so it got no change.

## Who plays which route

Each route's `_route` key names the persona (`docs/personas`) that plays it when a run names none, and a brief
of what the shop does, which the strategist and the narrator are given. A persona fits a route by character, not
by its own buy list: on a route the route's base policy does the buying.

| Route | Default persona | Also fits | Why |
|---|---|---|---|
| `appraiser` | magnate | honest-broker | the generalist: the Board, a cellar, a furnace and vending all at once |
| `watchman` | armourer | honest-broker | Security first and by the book; the alarm and the VIP race are the cautious play |
| `farmer` | rancher | vintner, magnate | the only start with growing things in the back room (no route uses livestock) |
| `vintner` | vintner | magnate | wine and nothing else |
| `labeler` | fence | shark | reads every fake and quietly passes a few as genuine |
| `no_vending` | shark | | scavenges hardest and wears the wounds |
| `gunsmith` | armourer | | its native start: Security's goodwill from day one |
| `outlaw` | fence | shark | its native start: Security's bad books, no courting the police |
| `smelter` | smelter | | trash into metal |
| `lean` | honest-broker | magnate | pure trading and appraisal with a clean name |
| `water_baron` | water-baron | | the tap |
| `rock_bottom` | shark | | its native start: nothing but nerve |
| `rancher` | rancher | | its native start: the rats are the run's engine |
| `pharma` | honest-broker | magnate | legal medicine from a phone supplier, a clean name |

The cook has no route: the drug-dealer start ends on day 8 (`cartel_retired`).

**One run spec.** A league plays "model × persona × route" with `routes` in place of `starts`, `perks`,
`personas` and `executor.directives`:

```json
{"name": "r1-routes", "seeds": [2610001], "days": 1000, "difficulty": "hard", "executor": {"human": "player"},
 "players": [{"id": "qwen27-delta", "endpoint": "localmodel", "mode": "plan", "bridge_flags": ["-directives", "delta"]}],
 "routes": [{"route": "smelter"}, {"route": "rock_bottom", "persona": "shark"}]}
```

The gym gets the route's start, perks and directives; a plan bridge gets `-route` (the brief and the base policy),
`-base-directives` and `-persona`. Matches are named `<player>__r-<route>__s<seed>__<persona>`. The plan prompt adds
a route section before the persona's style ("THIS RUN'S ROUTE (already in force)") and a rule that the plan speaks
of the route's engine and keeps its base policy unless the day gives a reason; the persona's `Buy:` and `Pivot:`
lines are left out on a route, so the plan lint does not demand purchases the route was tested without. Delta
lints are unchanged. The show reads the route from the match: the name card says "smelter route · Trash into
metal", and the narrator gets the brief as background. The live hook takes `fenceshow -route NAME`.

## LLM mini-league (in progress)

`league/r1-routes-qwen.json` (Qwen-27B, seeds 2610001–002, concurrency 2) and `league/r1-routes-drift.json`
(drift, seed 2610003, its one slot shared with the other leagues) play smelter × smelter, vintner × vintner,
outlaw × fence and rock_bottom × shark, each with the executor alone (`player`) as the paired baseline. The
league's gym was built before the short-rent lever, so its rock_bottom matches play `permit` unset without it.
The baselines all win (12/12: smelter and vintner on day 175, rock_bottom on 182, outlaw 175–182). The LLM
matches were still running at this writing (qwen ~1.3 game days a minute per match, drift ~2.5).

A 3-day sanity run of the same specs (seed 2619999) shows the plans taking the route's character from the
first morning. Smelter, day 2, GOALS: "Acquire furnace and blast module for 450 cash, leaving 164 buffer for
day 7 rent"; PRODUCTION: "Furnace: None owned yet. Priority: Acquire today. Scavenging: DUMPING GROUND tonight.
Target: Scrap (x4) or Ore (x2) + Flux." Vintner, day 3, GOALS: "Start wine engine ... Display 1 luxury item for
Discerning Taste"; FACTIONS: "Upper Levels: push toward +20 perk; sell wine at markup". Narrated through
`fenceshow -replay` with Qwen-27B (no TTS), the smelter run opens "Day one on hard. I am the apprentice of ore,
ready to turn this trash into gold." and plans "Six hundred and fourteen is a small mountain. I will spend four
hundred fifty to wake the beast, leaving just enough to breathe." Before main's run_start perks fix the same
replay spoke as the default magnate ("a respectable cushion against the inevitable").

## Screens that did not make the menu

20 seeds 2201001–020 with the incumbent policy unchanged (`s1_*` on 5078aa9, `s2_*` on 1722b2a + levers):

| Start or perk set | Wins | Median win day | NW d150 | Note |
|---|---|---|---|---|
| scav | 20/20 | 175 | 30 366 | close to the appraiser; not distinct enough |
| rancher | 20/20 | 175 | 28 001 | its livestock is not used by any engine (and ran loose in the stock grid: 7–13 days slower; now the `rancher` route) |
| aquafarmer | 20/20 | 182 | 26 618 | its RAS tank is not used by any engine |
| moonshiner, pharmacist, street food vendor, organ trader | 20/20 | 175 | 28 550 | identical net worth day by day: the gym gives each the same bare start (600 cash, no kit), so not a route |
| drug dealer | 0/20 | — | — | `cartel_retired` on day 8 in every run: the cartel start ends the run before the policy can play it |
| apprentice, `vending + proficient_scavenger` / `alert + short_sleeper` / `+ bad_lease` | 20/20 each | 175 | 28 758 / 28 865 / 29 331 | within noise of the incumbent |
| `vintner` with 3 bays | 20/20 | 175 | 28 268 | 4 bays is the better wine-only route |
| `smelter` with one furnace (`ore_phone` only) | 20/20 | 175 | 24 957 | two furnaces + eco is better |
| water merchant, `wine+water` | 20/20 | 175 | 23 876 | less water (6k), more wine: a weaker water route |
| water merchant, incumbent unchanged | 20/20 | 175 | 25 787 | sells little water (3k): not a water route |

## The wound rest on weaker routes

`scavenge_max_wound: 3` on the two routes that scavenge hardest or start weakest, same 60 seeds:
`no_vending` 60/60 → 60/60, ΔNW d150 −193 ± 1 341 (paired with `no_vending`; door-shut nights 48 → 2, rest
nights 57); `rock_bottom` 56/60 → 55/60 (+3/−4), ΔNW d150 −1 982 ± 2 472. No route gains from it.

## Challenge (2026-11-11)

Challenge is hard with a harsher rules table (docs/re/MITIGATIONS.md §10): the shop costs 36,000 plus 100 a day
after day 50, the mortgage wants 20 % down at 100 % a year, insurance and the Appraisal Board's join fee are ×3,
Wild's upgrades ×2 and the permit 3,000. A route plays challenge with `-difficulty challenge` in place of
`-difficulty hard`; the challenge policy:

```
fence-gym run -seed N -days 1000 -human player -difficulty challenge -start apprentice -perks vending+short_sleeper \
  -directives sim/lab/scripts/directives/challenge_best.json
```

**The challenge policy** (`challenge_best.json`) is the hard incumbent with no `buyout_by_day`, `buyout_save:
["insurance", "upgrades", "vending"]` from day 1 and `georgio_peak_days: [40, 43]`. It wins **60/60 on fresh seeds
3210001–060 (95 % CI 94–100 %), median day 182**, against 58/60 and day 210 for the hard incumbent (paired Δ win
day −22.8 ± 2.6); 60/60 and 182 on 3200001–060 too. Every win is the mortgage, signed on day 70 in 42 of 60 runs:
the cash buyout is out of reach (the price rises 700 a week and the rent 10 % a week, faster than any run saves),
so the shop cuts every overhead until the loan shark's offer, lends Georgio as much as it can on day 43, and
signs on the first rent morning after his fourfold payback. Sixteen payments later the deed is the shop's.
Day 182 is the floor for this path: the offer needs 15,000 cash on a rent morning and no run has it before day 70
(docs/PROGRESS.md, "Challenge").

**The routes on challenge.** Left: each route's file unchanged, 60 seeds 3200001–060 (every win but one by mortgage).
Right: the same file with the three challenge changes, 30 seeds 3200001–030, wins against the unchanged file on
those seeds and the paired Δ win day over seeds both won (± 1.96 SE):

| Route | Wins, unchanged | Median win day | Losses | Wins with the challenge changes | Median win day | Paired Δ win day | Losses |
|---|---|---|---|---|---|---|---|
| `appraiser` | 59/60 | 210 | repo 1 | | | | |
| `watchman` | 60/60 | 210 | — | 30/30 → 30/30 | 189 | -17.3 ± 5.3 | — |
| `farmer` | 57/60 | 210 | repo 3 | 29/30 → 29/30 | 182 | -18.8 ± 4.2 | repo 1 |
| `vintner` | 60/60 | 206 | — | 30/30 → 30/30 | 182 | -22.6 ± 4.1 | — |
| `labeler` | 59/60 | 210 | repo 1 | 30/30 → 30/30 | 182 | -25.9 ± 3.6 | — |
| `no_vending` | 60/60 | 210 | — | 30/30 → 30/30 | 182 | -22.9 ± 2.5 | — |
| `gunsmith` | 60/60 | 203 | — | 30/30 → 29/30 | 189 | -16.7 ± 4.2 | repo 1 |
| `outlaw` | 42/60 | 228 | repo 14, rent 4 | 21/30 → 24/30 | 196 | -29.3 ± 8.6 | repo 6 |
| `smelter` | 59/60 | 210 | repo 1 | 30/30 → 30/30 | 182 | -19.1 ± 3.5 | — |
| `lean` | 59/60 | 210 | repo 1 | 29/30 → 29/30 | 182 | -21.2 ± 4.0 | repo 1 |
| `water_baron` | 23/60 | 203 | repo 37 | 10/30 → 20/30 | 182 | -20.0 ± 5.5 | repo 10 |
| `rock_bottom` | 22/60 | 224 | repo 24, rent 14 | 12/30 → 19/30 | 210 | -30.6 ± 11.6 | repo 9, rent 2 |
| `sprinter` | 59/60 | 210 | repo 1 | | | | |

`appraiser` and `sprinter` play the hard incumbent's directives, so their row is the incumbent's; with the changes
they are `challenge_best`. With the changes the other strong routes win 29–30 of 30 (gunsmith one fewer than
unchanged) and finish 16–26 days sooner on the paired mean. The weak three lose mostly to repossession late in the
mortgage (payments ~2,500 a week; when the cash held for the next payment stops all trade buying, the shelf empties
and a payment lands short): `water_baron` (37 of 60 repossessed unchanged, 10 of 30 with the changes), `outlaw`
(starts in Security's bad books, and challenge doubles every reputation loss) and `rock_bottom` (13 of 60 lose the
rent on day 7 or 14, then repossession). The changes help all three (24, 20 and 19 wins of 30), but each still
loses a fifth or more of its runs; the challenge files below fix that.

### The challenge routes (2026-11-11)

Every route now has a challenge file, `sim/lab/scripts/directives/routes/challenge/<name>.json`. Its `_route.args`
pass `-difficulty challenge` and its brief describes the challenge run:

```
fence-gym run -seed N -days 1000 -human player -difficulty challenge -start <start> -perks <perks> \
  -directives sim/lab/scripts/directives/routes/challenge/<name>.json
```

Each is the route with the three challenge changes (no `buyout_by_day`, `buyout_save: ["insurance", "upgrades",
"vending"]` from day 1, `georgio_peak_days: [40, 43]`) and two more. `rock_bottom` takes a third:

- **`services: ["JANITORIAL_SERVICE"]`**: no insurance, ever (and janitorial every night, the power service off).
  `buyout_save` cuts the insurance only until the signing; after it the ×3 fee comes back. Over the first eight
  weeks of the mortgage the weak three pay ~570–600 a week in services with the insurance and ~95–120 without it,
  while burglaries take ~145–175 of goods a week without it (screening seeds 3400001–030). Listing the power
  service as well (`["JANITORIAL_SERVICE", "POWER_SERVICE"]`) loses: 20, 21 and 8 wins of 30.
- **`upgrades_cash_max: 0`**: Wild's upgrades for favor only. Once the save ends at the signing, the cash ones
  (taps, renovations, showcases, referrals) take ~310–340 a week through the first eight weeks of the mortgage.
- **`short_rent_days: 3`** (`rock_bottom` only; the hard route has 2): on 100 seeds 3400001–100 played to day 15,
  13 runs lose one of the first two rents with 2 and 7 with 3 (switching `short_rent_buy_pct` off gives the
  same 7).

`appraiser` and `sprinter` are therefore `challenge_best.json` plus the two levers, with their own persona and
brief; their directives, start and perks are the same, so they play the same runs.

Wins and median win day on **60 fresh challenge seeds 3410001–060** (1000-day horizon; Wilson 95 % CI); every
win is the mortgage. Sign day is the median signing day of the winners. The last two columns play the route with
the three changes alone on the same seeds; the paired Δ win day is over seeds both won (± 1.96 SE).

| Route | Wins (95 % CI) | Median win day | Sign day | Losses | Three changes alone | Paired Δ win day |
|---|---|---|---|---|---|---|
| `appraiser` | 60/60 (94–100 %) | 182 | 70 | — | 60/60 (—) | +0.0 ± 0.0 |
| `watchman` | 60/60 (94–100 %) | 182 | 70 | — | 58/60 (repo 2) | -0.2 ± 2.7 |
| `farmer` | 60/60 (94–100 %) | 182 | 70 | — | 59/60 (repo 1) | -3.9 ± 3.4 |
| `vintner` | 60/60 (94–100 %) | 182 | 70 | — | 58/60 (repo 2) | -1.1 ± 1.7 |
| `labeler` | 60/60 (94–100 %) | 182 | 70 | — | 57/60 (repo 3) | -0.9 ± 1.1 |
| `no_vending` | 60/60 (94–100 %) | 182 | 70 | — | 59/60 (repo 1) | -1.2 ± 2.4 |
| `gunsmith` | 60/60 (94–100 %) | 182 | 70 | — | 59/60 (repo 1) | -0.7 ± 1.5 |
| `outlaw` | 59/60 (91–100 %) | 189 | 77 | repo 1 | 42/60 (repo 18) | -0.7 ± 4.0 |
| `smelter` | 60/60 (94–100 %) | 182 | 70 | — | 60/60 (—) | +0.0 ± 0.0 |
| `lean` | 60/60 (94–100 %) | 182 | 70 | — | 60/60 (—) | +0.0 ± 0.0 |
| `water_baron` | 60/60 (94–100 %) | 189 | 77 | — | 41/60 (repo 19) | +1.9 ± 2.4 |
| `rock_bottom` | 57/60 (86–98 %) | 203 | 91 | rent 3 | 34/60 (repo 17, rent 9) | -4.1 ± 6.9 |
| `sprinter` | 60/60 (94–100 %) | 182 | 70 | — | 60/60 (—) | +0.0 ± 0.0 |

**Screens** on 30 seeds 3400001–030, wins and median win day against the route with the three changes alone
(paired Δ win day). The weak three:

| Arm | `outlaw` | `water_baron` | `rock_bottom` |
|---|---|---|---|
| the three changes | 23/30, day 203 (repo 7) | 16/30, day 186 (repo 14) | 19/30, day 203 (repo 8, rent 3) |
| + `mortgage_floor_stock: 0.5` | 23/30, day 203 (repo 7), Δ +0.0 ± 0.0 | 16/30, day 182 (repo 14), Δ +0.0 ± 0.0 | 21/30, day 203 (repo 6, rent 3), Δ +0.0 ± 0.0 |
| + `mortgage_floor_stock: 1.0` | 25/30, day 203 (repo 5), Δ +0.0 ± 0.0 | 16/30, day 186 (repo 14), Δ +0.0 ± 0.0 | 20/30, day 203 (repo 7, rent 3), Δ +0.0 ± 0.0 |
| + `mortgage_floor_days: 2` | 25/30, day 203 (repo 5), Δ +0.0 ± 0.0 | 16/30, day 186 (repo 14), Δ +0.0 ± 0.0 | 20/30, day 203 (repo 7, rent 3), Δ +0.0 ± 0.0 |
| + `repo_reserve_payments: 2.5` | 23/30, day 203 (repo 7), Δ +0.0 ± 0.0 | 16/30, day 186 (repo 14), Δ +0.0 ± 0.0 | 19/30, day 203 (repo 8, rent 3), Δ +0.0 ± 0.0 |
| + `mortgage_float: 3000` | 23/30, day 203 (repo 7), Δ +0.0 ± 0.0 | 16/30, day 186 (repo 14), Δ +0.0 ± 0.0 | 19/30, day 203 (repo 8, rent 3), Δ +0.0 ± 0.0 |
| + `mortgage_selldown_days: 7`, margin 500 | 24/30, day 203 (repo 6), Δ +0.0 ± 0.0 | 13/30, day 182 (repo 17), Δ +0.0 ± 0.0 | 19/30, day 203 (repo 8, rent 3), Δ +0.0 ± 0.0 |
| + `mortgage_margin: 1.25` | 17/30, day 238 (repo 9, rent 4), Δ +37.5 ± 10.0 | 20/30, day 203 (repo 8, rent 2), Δ +15.2 ± 8.4 | 22/30, day 234 (rent 6, repo 2), Δ +32.5 ± 12.4 |
| + `mortgage_save_from_day: 63` | 17/30, day 203 (repo 7, rent 5, arrest 1), Δ +5.0 ± 5.6 |  |  |
| + `upgrades_cash_max: 0` | 23/30, day 196 (repo 7), Δ -5.5 ± 6.9 | 22/30, day 186 (repo 8), Δ -1.9 ± 5.3 | 24/30, day 192 (rent 3, repo 3), Δ -0.8 ± 8.4 |
| + no insurance | 29/30, day 203 (repo 1), Δ -0.3 ± 4.8 | 29/30, day 189 (repo 1), Δ +0.4 ± 0.9 | 24/30, day 203 (rent 3, repo 3), Δ +8.3 ± 11.7 |
| + no insurance, power on | 20/30, day 203 (repo 9, rent 1), Δ +4.4 ± 7.3 | 21/30, day 189 (repo 9), Δ +2.3 ± 8.2 | 8/30, day 210 (rent 11, repo 10, arrest 1), Δ +14.0 ± 17.0 |
| + no insurance, `mortgage_floor_stock: 1.0` | 28/30, day 203 (repo 2), Δ -1.3 ± 4.6 | 29/30, day 189 (repo 1), Δ +0.4 ± 0.9 | 24/30, day 203 (rent 3, repo 3), Δ +3.5 ± 4.7 |
| + no insurance, `mortgage_floor_days: 2` | 28/30, day 203 (repo 2), Δ -1.3 ± 4.6 | 27/30, day 189 (repo 3), Δ +0.4 ± 0.9 | 24/30, day 203 (rent 3, repo 3), Δ +3.5 ± 4.7 |
| + no insurance, `upgrades_cash_max: 0` | 30/30, day 189 (—), Δ -6.7 ± 3.6 | 29/30, day 189 (repo 1), Δ -0.9 ± 4.6 | 26/30, day 203 (rent 3, repo 1), Δ +0.0 ± 8.8 |
| + no insurance, `upgrades_cash_max: 0`, floor days 2, floor stock 1.0 | 30/30, day 189 (—), Δ -6.7 ± 3.6 | 29/30, day 189 (repo 1), Δ +0.0 ± 4.2 | 26/30, day 203 (rent 3, repo 1), Δ +0.0 ± 8.8 |
| + no insurance, `upgrades_cash_max: 0`, `short_rent_days: 3` |  |  | 26/30, day 200 (rent 2, repo 2), Δ -3.9 ± 6.0 |

The cash held for the payment was not the lever: `mortgage_floor_stock`, `mortgage_floor_days`,
`repo_reserve_payments`, `mortgage_float` and a longer sell-down leave every won run unchanged and save at most two
runs of thirty (`water_baron` none; the longer sell-down costs it three). The money that runs short late in the
mortgage leaves early in it, on the insurance and the upgrades the save held back until the signing. On the other
ten routes (nine policies: `sprinter` plays `appraiser`'s runs) the two levers won 270 of 270 screening runs against
265 for the three changes alone (paired Δ win day from −6.3 ± 4.1 for `farmer` to +1.9 ± 2.9 for `gunsmith`), and
540 of 540 on the fresh seeds against 530, so every challenge file takes them.

## Reproducing

Arms ran with `fence-gym smoke -seed0 2202000 -seeds 60 -days 1000 -parallel 10 -no-trajectory`; summaries
are in `var/sim/exp/lev2/routes/f_<route>/` (follow-ups) and `s1_*`, `s2_*` (screens). The pairing script
reads `net_worth_by_day`, `player_days[].cash_flow` and `end.rep` from each run summary.

The challenge-route arms ran one process a seed, `fence-gym run -seed N -days 1000 -no-trajectory -summary FILE`
with the route's `_route.args` and its directives, on the gym of main 2141a86 (`var/sim/gyms/search-2141a86d0f57`);
the early-rent screen added `-days 15`. The signing day is the first day whose cash falls by more than 6,000 (the
down payment).
