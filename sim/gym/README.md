# sim/gym — the real game as an environment

`fence-gym` runs the installed game's own `Assembly-CSharp` headless (`sim/headless`, shared with
`sim/extract`) and plays the live mod's side of `docs/PROTOCOL.md` against it. Every bridge policy
(heuristic, LLM, strategist, memory) plays the real rules unchanged; the gym only stands in for
the human and the Unity engine. Personal research use; nothing under `var/` is ever committed.

```
make gym-build                      # also builds var/gamedata/.headless-gym-<key> (one per headless build) and loot_tables.json
make gym-run ARGS="-seed 3 -days 30 -v"
make gym-bridge                     # a sim bridge on 127.0.0.1:7399 (var/sim/log, var/sim/memory.db)
make gym-run ARGS="-seed 3 -bridge http://127.0.0.1:7399"
make gym-smoke                      # 20 seeds x 30 days per human role (default, prudent), in parallel
make gym-bench                      # in-process, then HTTP against a throwaway heuristic bridge
make gym-test
make gym-build MOD_SRC=/path/to/mod/src   # build against another checkout of mod/src (e.g. a pinned
                                          # snapshot while mod/ is mid-edit)
make lab                            # sim/lab: every mechanics experiment (docs/sim/VERIFIED.md)
make gym-arms ARMS=file DIR=var/sim/exp/x   # paired strategy arms (docs/research/EXPERIMENTS.md)
```

`run` flags: `-seed`, `-days`, `-difficulty cozy|normal|hard|challenge`, `-start none|fence|scav|…`
(`NewGameData.StartType`), `-skip-intro`, `-human default|prudent|bare|knobs`, `-knobs k=v,…` (with
`-human knobs`; `sim/gym/Knobs.cs`), `-perks ID+ID…`, `-warm SPEC`, `-permit buy|refuse`,
`-upgrades baseline|growth|none|ID+ID…`, `-sec-per-customer S`, `-sec-per-day S`, `-bridge URL` (else the
in-process heuristic), `-out FILE` (default `var/sim/runs/<run>.jsonl`), `-no-trajectory`,
`-wire FILE` (every request as sent), `-parity` (check option gates against the game), `-aug-detection
tells|perfect` (default `tells`: the executor judges augs on visible tells and the summary's `aug_eval`
scores it against the truth; `perfect` reads the truth flag, for analysis only), `-aug-lines off` (tells
without recognising the aug script's lines), `-repeat N`
(N episodes in one process, each in a fresh `AssemblyLoadContext`), `-v`. `smoke` also takes `-seed0 N`
(seeds N+1…N+seeds), `-no-trajectory` and `-parallel P`, and passes the run flags through.

## Status

- **2026-11-10 03:40 AST — furnace on hard: ore by phone and two furnaces, +21 days; still far from the
  mortgage.** `ore_phone` buys Wild's MINER (375) and phones the mining courier whenever his line is free
  (12 ore every 5 days at 12 each; docs/re/PRODUCTION.md §3), `blast_first` comes with it, and each
  furnace now gets its own modules. Owned gifts left on the front counter (the courier's free 8 ore) are
  stowed when the furnace runs (they used to be lost). Hard, `-start apprentice`, 30 paired seeds,
  `-days 1000` (`sim/lab/scripts/arms-furnace2.txt`, `var/sim/exp/f2/`; builds `var/sim/gyms/f2*`); furnace
  net a week is the run median (from the first furnace), steady is days 100–170:

  | arm | build | median days | endings | nw100 | furnace net/week | steady net/week (ingots/week) |
  |---|---|---|---|---|---|---|
  | furnace + concentrate (`furnace_conc.json`) | f2v5 | 188 | rent 29, arrest 1 | 5460 | 180 | 228 (8.4) |
  | + `ore_phone` (`furnace_phone.json`) | f2v5 | 202 (+14 ± 7) | rent 30 | 6711 | 285 | 362 (11.4) |
  | + `furnaces: 2` (`furnace_phone_2f.json`) | f2v6 | 209 (+20 ± 6) | rent 29, repo 1 | 6609 | 318 | 453 (16.0) |
  | + `furnaces: 3` (`furnace_phone_3f.json`) | f2v6 | 202 (+18 ± 5) | rent 29, repo 1 | 6408 | 329 | 472 (17.7) |
  | `furnaces: 2`, `furnace_metal: 15` | f2v6 | 209 (+20 ± 6) | rent 30 | 6665 | 323 | 447 (16.3) |

  Confirmed on 30 fresh seeds (`-seed0 700000`, build f2final): `furnace_phone_2f` **209** median days
  against 188 (paired +21 ± 5), nw200 +2 408 ± 571, furnace net 316 a week against 166 (steady 449
  against 225); 0 buyouts, every run still ends on rent. Did not help (dropped): Dr. Jackson's after-hours
  shop every night a module is missing (+2 ± 7 days: it costs the dump outing), a second or third spare
  cell (+13 / +5, no gain over the phone alone), MINER_SHAREHOLDER (1 350 on hard; 6 free ore a week and
  −35 % wholesale: +9 alone, +20 with two furnaces, no better than two furnaces without it), a larger metal
  hold (`furnace_metal: 30`, +15). Why the ceiling: a blast furnace on 3 ore + flux makes two Very High
  ingots (~150) a night for ~49 of inputs and 16 charge (~22 at the 1.35 a unit cells cost): ~80 a night
  when fed, and the phone feeds 0.8 of a night; scrap and junk run the second furnace at Fair/Low purity.
  The defaults are unchanged (normal, 10 fresh seeds: byte-identical cash and net worth by day).
- **2026-11-10 03:15 AST — normal regression isolated: signs-first display and the zero-estimate hold.**
  Forward bisect from the furnace branch (3912d75) to 85ef288, one group at a time, on 60 fresh seeds
  (`smoke -seeds 60 -seed0 700000 -days 1000 -human player`, normal; `var/sim/exp/fb/`, builds in
  `var/sim/gyms/`). The gym is deterministic per seed, so an unchanged run is byte-identical and every
  loss below is a seed the previous step won:

  | step | buyouts | median buyout day |
  |---|---|---|
  | 3912d75 (furnace branch) | 54/60 | 181 |
  | + morning endings and the default-off levers (wine aging, `georgio_collect`, `spend_down_to`, …) | identical on the 17 seeds run | 181 |
  | + fake-exposure guard (`Keep`, `HidesFake`, `PullExposable`) | 58/60 | 181 |
  | + exact-debt two-round bribes, own wine and berries first | 59/60 | 188 |
  | + `Keep` holds an item whose estimate is ≤ 0 | 23/31 (prior step 31/31 on the same seeds) | – |
  | + `Keep` holds signs and mislabeled water | 31/31 (26 runs identical) | – |
  | sign/water step + permit bought at evidence ≥ 30 | 57/60 | 181 |
  | sign/water step + Dr. Jackson every 7 nights (`ShopAtJackson`) | 58/60 | 188 |
  | sign/water step + bays, seeds and rack held before WINEMAKER | 57/60 | 181 |
  | signs first in the display case, with Jackson / with the reservation / with the permit | 17/24, 20/27, 14/20 (prior 100 %) | – |
  | 85ef288 (everything) | 34/60 | 195 |

  Dropped: the zero-estimate hold (the step with both holds, 44/60, sells 13 % and buys 22 % less a day;
  likely items with no estimate never leave the grid) and signs-first (likely a sign, never sold, takes a
  case slot from stock). The rest is kept: 58/60 (median buyout day 188) on these seeds, and **58/60**
  (188) against 57/60 (181) for 3912d75 on a second fresh set (`-seed0 800000`). Hard, `-start apprentice`, 30 seeds:
  player 146 median days (rent 13, arrest 14, execution 3), furnace + concentrate **188** (rent 28,
  arrest 2), against 145 and 181 on 85ef288.
- **2026-11-10 — re-baseline on the fixed outings (build `var/sim/gyms/m1`, the tree of 85ef288).** 30
  paired seeds, `-days 1000`. Hard, `-start apprentice` (`var/sim/exp/m1-hard`, arms in
  `sim/lab/scripts/arms-hard.txt`), 0 buyouts in every arm:

  | arm | median days | endings | nw60 | nw100 |
  |---|---|---|---|---|
  | player | 145 | rent 18, arrest 11, execution 1 | 2918 | 3630 |
  | `window_mode: concentrate` (`window_conc.json`) | 146 | rent 20, arrest 10 | 2754 | 4039 |
  | `engine: furnace` (`eng_furnace.json`) | 174 | rent 26, arrest 2, execution 2 | 3259 | 4166 |
  | furnace + concentrate (`furnace_conc.json`) | **181** | rent 29, arrest 1 | 3332 | 4108 |
  | `permit: buy` (`permit_buy.json`) | 160 | rent 19, arrest 10, execution 1 | 3179 | 4664 |
  | `permit_evidence: 10` (`permit_ev10.json`) | 143 | rent 21, arrest 8, execution 1 | 2918 | 3518 |
  | concentrate + `permit: buy` (`conc_permit_buy.json`) | 174 | rent 28, arrest 2 | 3134 | 4248 |

  Concentrate alone no longer helps (the earlier +28 was on the broken outings). What helps is buying the
  permit: every arm that cuts arrests to 1–2 gains about 30 days, and those runs then end on rent.
  Normal, `-start none` (`var/sim/exp/m1-normal`), nightly scavenging now costs a customer the next day:

  | arm | buyouts | median buyout day | endings | nw100 |
  |---|---|---|---|---|
  | scavenge every night (default) | **21/30** | 189 | buyout 21, rent 4, repo 4, execution 1 | 6444 |
  | `scavenge_every: 2` (`scav2.json`) | 14/30 | 203 | buyout 14, rent 12, repo 4 | 6197 |
  | `scavenge: false` (`scav_off.json`) | 5/30 | 154 | buyout 5, rent 14, repo 11 | 5937 |

  Scavenging every night is still worth its customer. Normal player is 21/30 here against 29/30 on the
  furnace branch alone; the entry above isolates it. 30-seed comparisons like these are too noisy to rank
  changes under ~6 wins: one changed decision reshuffles the rest of a run, and a toggle-off bisect of
  behaviour-neutral changes swung 13–22/30.
- **2026-11-10 — furnace engine; outings fixed.** (1) Every map outing used to stop in the travel's
  blackscreen callback (headless `OnLeaveStore`/`OnArriveStore` threw and the callback swallowed it):
  the executor searched the dumping ground on night 1 only (and on Georgio-assistance nights), and
  paid no next-day customer for its outings. Fixed (see "What the headless host fakes"); this moves
  every baseline. (2) `engine: furnace` (`sim/gym/Furnace.cs`, docs/re/PRODUCTION.md §3). Paired, 30
  seeds, `-days 1000`, hard with `-start apprentice` (`sim/lab/scripts/arms-furnace.txt`):

  | arm | build | buyouts | median days | endings | nw60 | nw100 | furnace net/week (median) |
  |---|---|---|---|---|---|---|---|
  | hard, player (wine) | HEAD 7a5e729 | 0/30 | 136 | rent 23, arrest 6, execution 1 | 3203 | 3663 | – |
  | hard, player (wine) | fixed outings | 0/30 | 118 | arrest 25, rent 5 | 3496 | 3425 | – |
  | hard, player, `scavenge: false` | fixed | 0/30 | 118 | rent 20, arrest 10 | 2574 | 3284 | – |
  | hard, `engine: none` | fixed | 0/30 | 132 | rent 16, arrest 14 | 2737 | 1710 | – |
  | hard, **`engine: furnace`** | fixed | 0/30 | **167** | rent 26, arrest 4 | 3440 | 4096 | **159** |
  | hard, `engine: wine+furnace` | fixed | 0/30 | 152 | arrest 16, rent 14 | 3583 | 4768 | 94 |
  | normal, player (wine) | HEAD 7a5e729 | 28/30 (day 154) | 153 | repo 1, execution 1 | 4841 | 8077 | – |
  | normal, player (wine) | fixed | 29/30 (day 182) | 181 | repo 1 | 4979 | 8617 | – |
  | normal, `engine: wine+furnace` | fixed | 27/30 (day 196) | 195 | repo 2, rent 1 | 4816 | 8194 | 98 |

  Paired on the fixed build: furnace vs `engine: none` +39 ± 13 days, nw100 +2 386 ± 590; furnace vs
  player +41 ± 13 days. Hard still never wins: one furnace adds ~150 a week (ingot value made less
  inputs at estimate and charge at cost; median 138 ingots, 88 smelts, 27 junk conversions per run),
  an order below the ~1 700 a week the hard mortgage needs. It is input-bound (junk from the dump
  gives a conversion every 3–4 nights; the junk module, 20 % per Jackson visit, comes late in some
  seeds). Much of the furnace arm's survival is the permit: without the wine engine's spending it buys
  the permit in 25/30 runs (player 5/30) and its arrests fall from 25 to 4. On normal, wine+furnace is
  slightly worse than wine alone (27 vs 29 buyouts, 14 days later): not a default.
- **2026-11-10 01:30 AST — hard, Apprentice start: `window_mode: concentrate` alone is +28 median days; Lower
  Levels rep leak closed.** (Measured before the outings fix above; re-baselining.) 30 paired seeds, `-difficulty hard -start apprentice -days 1000`
  (`var/sim/exp/hard6`, `var/sim/exp/hard7`, build `var/sim/gyms/h6`):

  | arm | median days | endings | nw60 | nw100 |
  |---|---|---|---|---|
  | player | 139 | rent 23, arrest 7 | 2196 | 3003 |
  | `window_mode: concentrate` (`window_conc.json`) | **167** | rent 21, arrest 9 | 3029 | 4314 |
  | concentrate + cash showcases (`conc_growth3_noins.json`), `-perks alert` | 111 | rent 26, arrest 4 | 1911 | 1702 |
  | … without `alert` | 76 | rent 28, arrest 2 | 1109 | 528 |

  The showcase-first upgrade list that led on the `none` start (181 in `hard4`) loses on this build on both
  starts (start none: 111 against 137 for player); `hard8` is bisecting which change since `h4` did it.
  Rep: on hard, Lower Levels rep reached −100 (mob ending) by day ~110. The leak was buyers exposing
  fakes: the permit official inspects each donation and every buyer re-inspects before a sale
  (`TryReinspectBeforeSale`), −5 a find on hard, sale or not, and display buyers inspect the case. The
  player now keeps back what the current buyer can expose (`Keep`), pulls anything exposable off the
  counter before selling (`PullExposable`), and keeps undisclosed fakes out of the case (`HidesFake`).
  On seed 3, LL now stays between −3 and +12 through day 150; the remaining losses are discount-demand
  walk-outs (−3.7 each). The morning perk endings (mob, sec, hitman, cartel_retired) now end the run
  as in the game; before, they threw headless and the run continued.
  Still 0 wins. The mortgage needs `2 × down` (≈ 6 000–7 000) in cash on a rent morning, after meeting the
  loan shark. Peak cash on any day is ~4 000 and peak net worth ~7 000; net worth stops growing around
  day 90 because weekly profit only matches the rent (×1.1 a week).
- **2026-11-09 23:05 AST — hard: +56 median days with concentrate + cash showcases + Vigilant.**
  `var/sim/exp/hard4` (30 paired seeds, `-difficulty hard -days 1000`):

  | arm | median days | endings | nw60 | nw100 |
  |---|---|---|---|---|
  | player | 125 | rent 23, arrest 6, execution 1 | 2423 | 2950 |
  | concentrate + upgrades (`conc_growth.json`) | 150 | rent 22, arrest 7, execution 1 | 2966 | 3915 |
  | … + `-perks alert`, janitorial only (`conc_growth_noins.json`) | 174 | rent 26, arrest 4 | 3340 | 4898 |
  | … showcases first: SHOWCASE_I, WINEMAKER, SHOWCASE_II, RETIRED_FARMER, RENOVATION1, WATER_TRADER, reserve 0 (`conc_growth3_noins.json`, `-perks alert`) | **181** | rent 25, arrest 4, execution 1 | 3154 | 5024 |
  | … + `-perks saving` | 167 | rent 25, arrest 5 | 3349 | 4642 |

  Vigilant (`alert`, 2 points) removes the burglary roll, so the hard insurance (×2 cost) is dropped.
  Each showcase is another display buyer a day; with the case concentrated, those are tier-3 budgets.
  Still 0 wins: the hard mortgage needs ~6 600 cash on a rent morning and no run gets there.
  Next (`hard5`, running): Dr. Jackson's night shop (now played: his stock is laid out and bought from
  like any seller; default list `jackson: ["sign_water"]`, a visit every `jackson_every: 7` nights when
  cash > rent + 800; a sign in the case brings that trade's supplier three days in four), more signs,
  and saving toward the mortgage threshold (`mortgage_save_from_day`).
- **2026-11-09 23:00 AST — hard: two levers beat `player` by ≥ 10 median days (no win yet).** 30 paired
  seeds, `-difficulty hard -days 1000` (`var/sim/exp/hard2`, `var/sim/exp/hard3`; arms in
  `sim/lab/scripts/arms-hard.txt`, directives in `sim/lab/scripts/directives/`):

  | arm | median days | endings | nw60 | nw100 |
  |---|---|---|---|---|
  | player | 125 | rent 23, arrest 6, execution 1 | 2423 | 2950 |
  | `window_mode: concentrate` (`window_conc.json`) | 132 | rent 24, arrest 6 | 2731 | 3284 |
  | concentrate + cash upgrades WINEMAKER, SHOWCASE_I, RETIRED_FARMER, WATER_TRADER, SHOWCASE_II, RENOVATION1 (`conc_growth.json`) | **150** | rent 22, arrest 7, execution 1 | 2966 | 3915 |
  | concentrate, `-perks alert`, janitorial only (`conc_noins.json`) | **146** | rent 23, arrest 4, execution 2, repo 1 | 3432 | 4436 |

  `window_mode: concentrate` fills the case with the type of highest summed value, so the day's display buyer
  is that type's tier-3 client once it shows 350 (wine → Upper Levels Hedonist, budget 350+; medical/material
  → 750+ budgets); hard has no trade favor, so showcases and renovations are bought with cash. Levers that
  did not move hard (hard1): Working Pipes (−21 days: the treated water crowds the grid), wine held to age
  15/30, the farmer start (−9), cash taps. The hard mortgage needs ~6 600 cash on a rent morning (twice the
  down payment); no hard run gets there yet.
- **2026-11-09 22:50 AST — 28/30 buyouts on normal (current defaults).** `-days 1000`, 30 paired
  seeds (`var/sim/exp/p21-*`, build `g14` = the working tree; arms in `sim/lab/scripts/arms-player.txt`,
  directive files in `sim/lab/scripts/directives/`):

  | arm | buyouts | median buyout day | endings | nw60 | nw100 | nw200 |
  |---|---|---|---|---|---|---|
  | normal, `base` (v2 default role) | 0/30 | – | arrest 13, rent 17 (median day 76) | 1003 | 0 | 0 |
  | normal, **player** | **28/30** | **154** | buyout_mortgage 28, repo 1, execution 1 | 4841 | 8077 | 8415 |
  | normal, player, `georgio: lend_25` | 28/30 | 161 | repo 2 | 5043 | 7789 | 7894 |
  | normal, player, `-start farmer` | 27/30 | 154 | repo 2, execution 1 | 4676 | 8822 | 10308 |
  | hard, `base` | 0/30 | – | rent 29, aug 1 (median day 34) | 0 | 0 | 0 |
  | hard, player | 0/30 | – | rent 21, arrest 9 (median day 127) | 2360 | 3112 | 0 |

  Every normal win is `buyout_mortgage` (12 weekly payments of ~1 000 signed around day 70); no run
  reached the cash buyout (12 000 rising to 17 000). What kills hard: the rent (×1.1 a week, ~1 100 by
  day 120) and the permit official (arrests for OPERATING_WITHOUT_LICENSE when the bribe route fails and
  the 2 250 fee is out of reach); the hard mortgage (down 3 600, ~2 500 a week) is beyond the cellar.
- **2026-11-09 22:30 AST — careful with a checkpoint taken 22:20–22:28.** The water "swap" fix (a
  `sell_and_buy` buyer with our goods staged and no `sell_all` → `buy_all`) first applied to every
  swapping customer, so the executor bought whole junk lots at full price: normal fell to 5/30
  buyouts (`var/sim/exp/p20-normal`). It now applies only when what we staged is water and what they
  brought is empty liquid containers; seeds 1, 2, 4 win again. Re-measuring in `p21`.
- **2026-11-09 22:20 AST — policy-search levers.** New directives, defaults = current behaviour:
  `mortgage_window`, `mortgage_float`, `mortgage_margin`, `repo_reserve_payments`,
  `mortgage_max_spend`, `mortgage_selldown_days`, `mortgage_selldown_margin`; also `georgio_loan`
  (25 | 50 | decline), `georgio_peak_days`, `ll_rep_guard`, `inputs_floor`, `inputs_max_spend` (see
  "Playing as an agent"). All gym tests pass (12).
- **2026-11-09 22:15 AST — plan-mode fix.** With `-human player` and a bridge that defers (observe
  mode), counter and aug decisions now fall back to the executor's own `PlayerPolicy` / `AugPolicy`
  instead of the bare role's dismissal (sim/gym/Episode.cs `Decide`, Episode.Player.cs aug check), so a
  bridge in observe mode supplies only the directives and the tuned counter plays. `-bridge defer:` is
  an in-process stand-in for such a bridge; the gym test `PlayerUnderADeferringBridgePlaysItsOwnCounter`
  holds the two byte-for-byte equal in outcomes (net worth and cash by day, aug kills). Run leagues
  with the bridge in observe mode. (In act mode the bridge's own counter policy still decides.)
  (The snapshot's `upgrades` owned/available mix-up is fixed in 5bcd757: owned = state 2, available =
  state 1 ready, `cooling` with days.)
- **2026-11-09 22:10 AST — 26/30 buyouts on normal.** `-days 1000`, 30 paired seeds, `-start none`
  (`var/sim/exp/p18-normal`, build `g11`): **player 26/30 `buyout_mortgage`, median buyout day 175**,
  4 repo, 0 rent/arrest; nw100 8 284, nw200 8 476. `mortgage_margin: 1.25` 25/30 (median day 182).
  What did it: the retired farmer's lot had been refused whole as "contraband" (his table carries
  dream-cap seeds the executor never stages) — now contraband is judged on what each option buys, so
  bloomberry seeds, filters and tablets come in; the bay's water is topped up from any clean container
  (a full stock grid had blocked swapping its bottle) with nutrient tablets from their box; the tap
  (WATER_TRADER → taps) bought with favor alone; repossessable stock ≥ 1.5 payments kept under the
  mortgage. The cellar now starts ~7–9 bottles a week from day 50 (~1 000 a week at finish).
  Same build, other starts: `-start farmer` 26/30 (median day 154), `-start fence` 16/30 (median 189;
  14 repo). Hard: 0/30 on every arm (player median 118 days, rent 23 / arrest 7; wine is off on hard
  by default — being re-tested with the fixed engine).
- **2026-11-09 21:55 AST — 12/30 buyouts on normal.** `-days 1000`, 30 seeds (`var/sim/exp/p16-*`,
  build `g8`): player 7/30 (`buyout_mortgage`, median day 154; repo 9, rent 14), **player +
  `services_mode: ev` 12/30** (median day 154; repo 9, rent 9) — now the default; hard: player 0/30,
  median 118 days. Water-merchant start with `engine: water` 0/30 (median 139 days normal, 55 hard):
  the water engine works mechanically (tap → filter → tablets or purifier → AquaScan/strip Match →
  sold) but makes ~0.4 L a day and sells slowly; not a win path in the gym yet.
- **2026-11-09 21:40 AST — first buyouts; new horizon (buyout or game over, capped at 1000 days).**
  30 paired seeds, `-days 1000`, `-start none`, normal (`var/sim/exp/p15-normal`, build `var/sim/gyms/g7`):

  | arm | buyouts | median buyout day | endings | median days | nw100 | nw200 |
  |---|---|---|---|---|---|---|
  | `base` (v2 default role) | 0/30 | – | arrest 13, rent 17 | 76 | 0 | 0 |
  | **player** (mortgage when cash − down + ½ stock + 80 % of 12 weeks' earnings ≥ 1.0 × payments) | **5/30** | **154** | buyout_mortgage 5, repo 16, rent 9 | 153 | 4676 | 2472 |
  | player, `mortgage_margin: 1.25` | 5/30 | 161 | buyout_mortgage 5, repo 12, rent 13 | 202 | 5591 | 3591 |
  | player, `mortgage: decline` | 0/30 | – | rent 30 | 286 | 7271 | 6997 |
  | player, `tap: true` | 0/30 | – | rent 28, arrest 2 | 160 | 4107 | 145 |
  | player, `-start water_merchant -perks working_pipes` | 0/30 | – | rent 26, arrest 4 | 139 | 2737 | 0 |
  | hard, `base` | 0/30 | – | rent 29, aug 1 | 34 | 0 | 0 |
  | hard, player | 0/30 | – | rent 27, arrest 3 | 118 | 702 | 0 |

  The win so far is the **mortgage** (`buyout_mortgage`: 12 weekly payments of ~1 100–1 200 instead of
  the rent, taken around day 70 with ~5 000 cash). The cash buyout (17 000 by day 150) is never in
  reach: the shop plateaus at a net worth of 7 000–12 000 around day 200 and the compounding rent
  (×1.047 a week) ends every run that declines the mortgage by day ~290. Most mortgage takers end in
  `repo` (a missed payment with too little stock to repossess) after 3–11 payments: the shop nets
  ~700 a week, so the 12 payments need ~5 000 of cushion and the timing is a coin flip. The player
  now takes the landlord's cash buyout whenever offered and, within 3 days of a rent visit, sells down
  toward the price when cash + 70 % of stock covers it (never triggered yet). Water selling (`engine:
  water`, tap → filter pitcher → tablets → strip and Match, the portable purifier) and the tap for
  wine both lose so far: supplies run out and the treated water sells slowly.
- **2026-11-09 21:25 AST — progress: normal survives to day 200.** Same protocol (30 paired seeds ×
  200 days, `-start none`; `var/sim/exp/p10-*`, built from the working tree as `var/sim/gyms/g2`):

  | arm | median days | alive at 200 | buyouts | endings | nw60 | nw100 | nw200 |
  |---|---|---|---|---|---|---|---|
  | normal, `base` (v2 default role) | 76 | 0/30 | 0 | arrest 13, rent 17 | 1003 | 0 | 0 |
  | normal, **player** | **200** | **23/30** | 0 | rent 6, arrest 1 | 3284 | 5792 | 4077 |
  | hard, `base` | 34 | 0/30 | 0 | rent 29, aug 1 | 0 | 0 | 0 |
  | hard, player | 90 | 0/30 | 0 | rent 29, arrest 1 | 489 | 38 | 0 |
  | hard, player + `buy.wholesale_max_pct: 95` | **104** | 0/30 | 0 | rent 28, arrest 2 | 1210 | 186 | 0 |

  New since 20:45: the farmer's hydroponic bay (bloomberries on clean water + nutrient tablets, battery
  swaps; its kit is stowed so the next buyer can't take it off the back counter), Wild favor spent on
  renovations and showcases (favor buys nothing else; it piled up to 2 000+ unspent), selling down to
  75 % of the estimate (90 % lost a third of the sales), and staging within a buyer's budget. Seed 3 now
  makes ~200 bottles in 200 days. Still no buyout: income plateaus around +300 a week over the rent;
  the mortgage (12 × ~1 200) is offered from cash 5 000 and the executor now takes it only when the
  cash, half the stock and 80 % of twelve weeks of earnings cover 125 % of the payments (one early
  taker ran out after 8 of 12 payments: ending `repo`). `-human knobs` is unchanged by all this:
  the knob arms are byte-identical to the build before the executor (checked on seeds 1–3, normal and
  hard, the league's knob set), so the league's 85/44 vs 79/34 gap is a different build or knob set,
  not a semantic change here.
- **2026-11-09 20:45 AST — progress.** Paired arms, 30 seeds × 200 days, `-start none`
  (`sim/lab/scripts/arms-player.txt`; `var/sim/exp/p6-*`). Best arm = `-human player` (defaults):

  | | median days | buyouts | endings | nw60 | nw100 | nw200 |
  |---|---|---|---|---|---|---|
  | normal, `base` (v2 default role) | 76 | 0/30 | arrest 13, rent 17 | 1003 | 0 | 0 |
  | normal, **player** | **146** | 0/30 | rent 25, arrest 5 | 2178 | 2254 | 0 |
  | hard, `base` | 34 | 0/30 | rent 29, aug 1 | 0 | 0 | 0 |
  | hard, **player** (wine off: `engine: none`) | **62** | 0/30 | rent 30 | 274 | 44 | 0 |

  What moved it: shooting augs (hard: no more `aug` endings), donating goods to the permit official
  from day 45 (normal: the day-80 arrest wall is gone), declining the landlord's vending machines
  (each adds 150 a week to the rent, compounding), buying only below the estimate, and the wine engine
  (normal: +550 nw100; bottles, rations and berries bought from every seller; berries now bind). What
  still kills runs: rent, from day ~100 on normal and ~55 on hard (trading margin plus ~3 wines a week
  falls behind the compounding rent).
- **2026-11-09 20:20 AST — directives supported; step mode works.** `-human player` is the
  directives-reading executor: in-process it reads `-directives FILE` (over the knobs), with
  `-bridge URL` it reads `/v1/summary` → `directives` every morning, and with `-step-dir DIR` it
  plays one day per `directives.json` (below, "Playing as an agent"). Augs are shot on perfect
  detection (`augs: shoot`, the default), the permit official is bribed with donated goods from day
  45 unless the plan says buy, and stock is sold down toward rent. Numbers to follow.

## Playing as an agent (`-step-dir`)

```
dotnet sim/gym/bin/Release/net10.0/fence-gym.dll run -human player -seed 3 -days 200 -difficulty normal \
    -step-dir var/sim/step/s3 [-step-timeout 600] [-warm SPEC] [-directives base.json]
```

Each morning (after the day's morning snapshot) the gym writes `DIR/day-NNN/snapshot.json` and waits
for `DIR/day-NNN/directives.json`; it then plays the whole day with the executor following those
directives and writes `DIR/day-NNN/result.json`. At the end of the run it writes `DIR/final.json`.
Files the gym writes appear atomically (written to `*.tmp`, then renamed); write `directives.json`
the same way, or completely within a second or two (a half-written file is retried for ~3 s).

- `snapshot.json`: `day`, `seed`, `difficulty`, `respond_with` (the path to write), `snapshot` (the
  live mod's visible `day_snapshot`: store, rent, property, reputation, inventory with uids, clientele,
  security record, upgrades, services, market events, night log, perks), `yesterday` (the previous
  day's result), `risks` (rent due and cover, Landlord Grace, permit and bribe owed, the official's next
  visit, crime and evidence, aug kills/escapes and the reconstructed presence, buyout price),
  `base_directives` (what an empty `{}` plays). `risks.mortgage` gives the offer (down payment, weekly,
  the cash that triggers it, whether the loan shark has introduced himself) or, while paying, the
  payment, days to it, repossessable stock and `game_over_if_unpaid`; `risks.wild` the favor and every
  upgrade still to buy with its cost, readiness and cooldown (Landlord Grace has a 15-day cooldown
  after each purchase, so it can sit unready after a use; also `risks.rent.landlord_grace_cooldown_days`);
  `risks.georgio` an outstanding loan; `item_types` every owned item's types by uid. `result.json`
  also lists `wild_purchases` (upgrade, cost, cash and favor spent). The forward cash floor holds
  from five days before rent: the rent less 60 % of the last week's average daily sales per day left.
- `directives.json`: the contract in `docs/PROTOCOL.md` "Directives" (`protocol/fixtures/directives.json`);
  every field optional, `{}` plays the base. A base file's `locked` (a list of keys, dotted for nested
  ones: `["mortgage", "buy.hot_stock_cap"]`; default none) keeps those keys at the base's value whatever
  a day's plan sends: a stream format's handicaps (docs/SHOW.md, "Formats"; `Directives.Merge`, shared
  with the live autopilot). How the executor reads the protocol fields:
  `services` forces exactly those services on (the tablet still drops what tonight's cash cannot pay);
  unset keeps insurance and janitorial on unless tonight's bill is unaffordable and turns the power
  service off while nothing owned draws store power. `permit`: `bribe` never buys (goods donated to
  the official from day 45), `buy` buys when the fee leaves rent and floor covered, unset buys when the
  fee leaves rent + floor + 500 or the bribe debt passes 300. `sell.min_pct` is the floor for
  ordinary sales only; liquidating (cash below `sell_down_to_cash`, or the rent from 3 days out) takes
  the lower of it and `sell.liquidate_min_pct` (default 65). `buy.stock_cap` / `buy.restock_below`
  count trading stock only (engine inputs and `hold_uids` excluded). `buy.prefer_types` (item types or
  ids) are bought up to their estimate. Gym extensions: `sell.sell_hot` (stage stolen goods; default
  false), `engine` (`wine`, `water`, `furnace`, joined by `+` or given as a list, or `none`; default
  wine), `furnaces` (default 1; each furnace gets its own junk and blast module), `furnace_blast`
  (default true), `ore_phone` (default false: buy Wild's MINER introduction and phone the mining
  courier for ore whenever his line is free), `blast_first` (default true with `ore_phone`: the blast
  module without waiting for the junk module) and `furnace_metal` (scrap and ore held per furnace;
  default 9, 21 with `ore_phone`) for the furnace engine,
  `bays` (hydroponic bays to run, default 3), `tap` (true: buy WATER_TRADER → taps and treat tap water
  for the bottles), `growth` (upgrades bought with favor or surplus cash, default RENOVATION1,
  SHOWCASE_I, RENOVATION2, RENOVATION3, SHOWCASE_II), `upgrade_reserve` (cash kept after an engine
  upgrade), `mortgage` (`auto` | `take` | `decline` | `accept_if_rent`; `loan: accept_if_rent` is an
  alias), `mortgage_margin` (auto takes it when cash − down payment + half the stock + 80 % of twelve
  weeks of earnings ≥ margin × the payments; default 1.0), `buyout` (`asap`, the default: take the
  landlord's offer whenever cash covers it, selling down toward it within 3 days of a rent visit;
  `hold_cash_for_buyout`: stop spending below the price), `buyout_by_day` (answer the loan shark's
  offer "later" while cash + 70 % of the sellable stock + the last four weeks' net-worth gain carried
  forward covers the price on that day, so the cash buyout comes first; once it does not, `mortgage`
  decides; default none) with `buyout_check_from` (the projection starts that day, default 84: before it
  Georgio's payback and the appraisal ramp make the four-week gain undershoot; "later" until then),
  `buyout_selldown_days` (default 3: how many days before a rent visit the buyout sell-down starts, once cash +
  70 % of the sellable stock covers the price), `buyout_save` (a list of `upgrades` | `inputs` | `vending` |
  `insurance`: from `buyout_save_from` (default day 1) until the shop is bought or mortgaged, no cash upgrades
  but Landlord Grace, no engine inputs or phoned ore, no vending upgrades, no insurance; default none),
  `services_mode` (`ev`, the default: no
  insurance in the first three nights or while trading stock × 0.35 is under its fee, janitorial every
  second night; `both`: both every night), `spend_favor` (default true: favor left after growth buys
  referrals, marketplace requests, the second showcase and renovations; EVIDENCE_REMOVAL at evidence
  ≥ 50), `upgrades_cash_max` (most cash an upgrade may take beyond favor; non-essential upgrades in
  `upgrades_buy` pay cash only from a 1 500 surplus either way), `engine_reserve_max` (stop buying
  engine inputs while held inputs are worth more), `restock_missed` (default true: types three buyers
  asked for in a week and found nothing of are bought up to their estimate while fewer than two are
  in stock), `min_free_cells` (default 36: below it the executor sells down as when short of rent),
  `sell.release_uids` (sell these even if an engine would hold them), `bargain: "discount"`
  (budget-raising discounts only, and no demands on sellers), `bargain_buy` (`never` | `ev_positive` |
  `aggressive`: the seller side alone), `restock_lot_max` (default 300: a restock or preferred-type
  single lot above this still needs the usual discount), `georgio` (`lend_25` | `lend_50` | `no`;
  alias `georgio_loan: 25 | 50 | decline`; unset lends 50 %), `georgio_peak_days: [40, 43]` (stop
  buying and sell down over those days so the loan, a share of cash on hand, is larger),
  `ll_rep_guard` (default −12: no discount demands on Lower Levels sellers at or below this rep),
  `mortgage_float` (default 1 000: cash after the down payment must also cover 1.5 payments plus
  this), `mortgage_window: [from_day, to_day]` (sign only then; default any day; on hard, of 80 signings
  after day 105 on 240 seeds 3 won and 77 were repossessed, but a `[1, 105]` deadline also gives up those 3),
  `mortgage_cover` (sign only while cash after the down payment plus repossessable stock covers this many
  payments; default none), `mortgage_floor_days` (hold the next payment in cash only from this many days
  before it; default always), `mortgage_floor_stock` (the cash held for the next payment is less this share
  of the repossessable stock's value, so trade buying goes on; default none),
  `repo_reserve_payments` (default 1.5: repossessable stock kept, by restocking, while paying),
  `mortgage_max_spend` (daily cap on trading purchases while paying; default none),
  `mortgage_selldown_days` / `mortgage_selldown_margin` (default 5 / 150: sell down toward the payment
  plus the margin from that many days out), `inputs_floor` / `inputs_max_spend` (engine inputs are exempt
  from `cash_floor` and `buy.max_spend`; under a mortgage they are bought while cash + 70 % of the
  sellable stock covers the next payment),
  `buy.contraband` / `sell.sell_contraband` (default false; never staged for Security or for buyers
  who refuse contraband), `conceal` (docs/re/CONCEALMENT.md §6: `policy` always | on_warning | never,
  `min_level`, `max_exposed_heat`, `use_poster`; each morning flaggable goods go into the largest
  smuggler's bay with room, then the poster stash, with the poster kept down, and cooled goods come
  back out; with a bay, hot goods are capped by its free cells instead of `buy.hot_stock_cap`).
  The vending machine (`sim/gym/Vending.cs`; the `vending` perk or Wild's VENDING_MACHINE): when one is owned,
  spare stock (not held, not an engine input, not hot, no hidden fake) goes in at bedtime, best expected sale
  a night per cell first and one of each kind before copies; each morning its cash is collected and what did
  not sell returns to the stock grid for the counter. Common electronics go into attractiveness and, after a
  night the grid was full, the cheapest ingot into another row (`vending_upgrades: false` stops both).
  `vending: "buy"` buys the machine from Wild with cash from `vending_day` (default 1) when `vending_reserve`
  (default 500) is left over the rent and floor; `vending: "off"` leaves an owned machine empty.
  Phone suppliers (`sim/gym/Suppliers.cs`): `beer: "phone"` buys Wild's BREWER introduction and
  phones the Red Imp rep whenever his line is free (a free case first, then one case of eight beers at 20 % off
  per call, two once 400 is spent; 4-day cooldown); `pharma: "phone"` does the same with PHARMA's Dale (six free
  pink injectors, then 5 a call, 10 once 500 is spent; 7-day cooldown) and `chem: "phone"` with CHEMIST's Ashish
  (eight free dream dust, then 8 a call, 16 once 400 is spent; 4-day cooldown; contraband: only the free
  delivery without `buy.contraband`). `<key>_shareholder: true` adds the shareholder upgrade (another 15 % off and a weekly
  dividend: a case, 5 injectors or 8 dust); `miner_shareholder: true` adds MINER_SHAREHOLDER once the furnace's
  `ore_phone` has bought MINER (6 ore every Friday). Each upgrade waits until `<key>_reserve` (default 500) is
  left over the rent and floor. Free deliveries and dividends land on the front counter as already ours, and the
  night clears that counter (`EmporiumEntry.ClearFrontInv`): they are stowed after each customer (before
  2026-11-11 the gym lost every dividend case and free delivery). The summary's `player.suppliers` counts
  orders, stowed gifts and each supplier good bought and sold.
  Rats (`sim/gym/Rats.cs`, `engine` containing `rats`; the Rat Wrangler start or RETIRED_RANCHER's kit): each
  morning rats outside a cage go in, droppings go to the bin, dead rats and those at `rats_butcher_g` (default
  400) are cut up with the cleaver (raw meat per 400 g, small raw meat per 200 g, morsels), breeders are kept
  (`rats_breeders: [females, males]`, default [1, 1], each with a young successor spared near its old age), the heaviest growers are cut while the rats fill more than
  `rats_occupancy` of the cage (default 0.7; growers under 150 g only above 85 %) or, from `rats_food_cut_g`
  (default 300), while the food would not last two nights, owned food (`Rats.Feeds`: milk, juice, noodles, nutrifruit and other cheap calories, never rat meat) is
  blended into the dispenser up to `rats_feed_kcal` (default 6000; the shop's own food stock in a pinch), an owned
  ingot grows the cage a row (up to 8 rows of 10) and scrap mends it below 60 %. The meat is FOOD stock. The
  summary's `player.rats` (and each day's `rats`) counts births, deaths, cuts, meat sold, feed bought and the cage.
  Cash crops (`sim/gym/Crops.cs`, docs/research/SECOND-ENGINE.md): `crops: {"coffee_bean": 2, ...}` buys that many
  bays beyond the cellar's `bays` and grows each crop in them, replanted from its own harvest, the rest sold as stock;
  with `crops_tap` (default true) it buys WATER_TRADER and GUTTERFLOW_TAP and fills a dry crop bay at the faucet.
  `production.crops` counts bays, harvests, sales and tap water. The caffeine lab (`sim/gym/Lab.cs`, `lab:
  "caffeine"`): buys RETIRED_CHEMIST (after the permit unless `lab_after_permit: false`; `lab_reserve`, default 500,
  over rent and floor) and runs one batch a step each morning (blend with acid and lipobond, heat, filter and dry,
  press pills); it holds `lab_beans` (default 12) beans and buys packs, a second pitcher, filters, chem modules and
  missing machines. `production.lab` counts batches, pills, sales, modules and why it stalled. Gym only,
  `supply_probe: {"caffeine_pill": {"per_day": 12, "purity": 99.5}}` puts free units in stock each morning from
  `supply_probe_from` (default 30), and `player.supply_probe` counts what sold: a demand ceiling, not a player move.
  The 3D printer (`sim/gym/Printer.cs`, `printer: "run"`; the Gunsmith start's printer or RETIRED_GUNSMITH's,
  docs/re/RECIPES.md §1): each morning its output goes to stock, a charged cell into its battery and, when it is
  idle, the first chip of `printer_chips` that is owned, fits its modules and has its inputs owned goes in with
  those inputs (default order: silencer, improved springs, makeshift RDS, RDS, laser sight, machete, combat knife,
  kitchen knife, cleaver, hatchet, box cutter; printed pistols are contraband and left out). Chips are bought from
  the retired gunsmith until `printer_buy_chips` (default 2) of the order are owned, with 1 500 over the rent;
  plastic, scrap, electronics and a cell as the owned chips need them. `player.printer` counts loads, output value,
  inputs and chip spend. `jackson_sunday: true` makes
  the Jackson visits on Sundays (his 10 % discount day), every `jackson_every / 7` Sundays.
  `jackson_equipment: true` also buys at his shop what the engines want (`Production.Wants`: the furnace and
  its modules, hydroponic bays, charged cells, the purifier, the bioreactor), not only the `jackson` list;
  `jackson_need: N` (with it) adds a visit, at most once in N nights, whenever an engine wants one of the
  furnace, its modules, a bay, the purifier or the bioreactor. His goods carry the difficulty's inventor
  markup (`Difficulty.Rules.inventorMarkup`: normal +15 %, hard +50 %) at the shop and on his store visits.
  The summary's `player.jackson` lists what was bought from him, at the shop and on his visits.
  The default list's water sign is bought but stays in stock unless `window_signs` lists it.
  `landlord_vending: "machine"` accepts the landlord's vending machine (+150 weekly rent) instead of declining it.
  The water fountain (`sim/gym/Fountain.cs`, docs/re/WATER.md §2.2; off by default): `fountain: "buy"` buys Wild's
  VENDING_FOUNTAIN (1 800 on hard) from `fountain_day` (default 1) once `fountain_reserve` (default 500) is left over
  the rent and floor; `fountain: "landlord"` takes the landlord's offer instead (days 43, 57, 71, 85; +150 weekly rent,
  compounding). Each night it pours and sells, with no customer and no chance, a fixed set of bottles from its Base,
  HQ and Pure slots (more and larger as its attractiveness rises: 1.2 L a night at 15, up to 9.5 L at 100). At bedtime
  each empty slot gets the fullest owned water vessel whose true grade reaches it (better water goes up a slot first);
  each morning its cash is collected, near-empty vessels come back for the water engine, a charged cell goes in, and
  common electronics raise its attractiveness before the vending machine's. It needs a water engine to feed it. The
  summary's `player.fountain` and each day's `fountain` give income, attractiveness and vessels poured; sales are
  `cash_flow.fountain_sales`. With `engine` water: `purify_raw: true` fills the largest empty vessel at the tap and
  puts it straight into an idle purifier when no pre-treated water waits (Rustwater reaches HQ in about six nights,
  ~1 L a night a purifier, with no filter or tablet), and `purifiers: N` (default 2) sets how many purifiers to own.
  Wine levers (`sim/gym/Wine.cs`, `sim/gym/Dna.cs`, docs/re/WINE.md; all off by default): `wine_score: N` fills each
  bottle with berries until its score reaches N (best first, a normal berry for a one-point gap, an under/over-ripe one
  for the pair bonus; short of N a filled bottle waits `wine_wait_days`, default 2); `wine_hold_cash: C` holds
  `wine_min_age` wine only while cash covers rent + floor + C (held wine also stays out of the display case now);
  `wine_racks: N` buys up to N AgeWell racks at Dr. Jackson's for the held wine; `wine_purifier: true` buys a water
  purifier and lifts the cellar's water to High Quality (+1 a bottle, healthier bays); `wine_dna: true` buys Wild's
  PLANT_DNA (once cash covers it plus rent and `wine_dna_reserve`, default 1 500), then Jackson's DNA manipulator and
  blank disks, extracts `hardy` from fillerweed/nutrifruit/hydroreed seeds, applies it to bloomberry seeds
  (`wine_dna_ranks`, default 1) and plants them (an owned hatchet clears a plain plant, else one more bay);
  `wine_dna_precision: true` also tunes Jackson's precision module on the first Hardy disk. The summary's
  `production.wine_sold` gives every bottle sold by price, age, grade and buyer, plus berry grades at harvest and
  bottle scores at the start.
  Stock breadth (`sim/gym/Breadth.cs`, docs/re/CLIENTS.md; all off unless set): `breadth: {top, k, window,
  queue, max_pct, lot_max}` keeps `k` sellable units (default 2) of each of the `top` types (default 6) the
  buyers of the last `window` days (default 14) asked for, by visible interest tags weighted by budget;
  `queue: true` adds the visible wants of the buyers still waiting today. When a seller's lot is not cheap
  enough to buy whole, only its goods of short types are staged and bought at up to `max_pct` of their
  estimate (default 95) while the staged price is at most `lot_max` (default 600); wholesale of the whole
  lot is judged as before. `stage_variety: true` stages one of each item id for a variety buyer.
  `window_signs: ["sign_medical", …]` buys those supplier signs at Dr. Jackson's and shows them in the case
  first: each brings its trade's wholesale seller (15–25 % off) three openings in four, in place of a random
  normal customer. At Dr. Jackson's the gym makes him the counter's client for the visit, as
  `DrJacksonLocation.Arrive` does (`MapUIManager.ActivateClientOutsideStore`'s logic half); before 2026-11-10
  it did so only with `window_signs` or `storage_bays`, and every other plan that visited him bought nothing.
  `hold_for: {types, k, faction}` keeps the k most valuable goods of each listed type for that faction's buyers
  (default Upper Levels: +15 % tips once UL rep reaches 20, `UL_TIPS`) unless the shop is short of cash.
  Trading styles (`sim/gym/Style.cs`, docs/research/TRADING-STYLES.md; all off unless set): `specialty: {types,
  lot_share, buy_at_estimate}` buys only goods of those types (a wholesale lot only when they are `lot_share` of its
  estimate, default 0.5; from a mixed seller only they are staged) and fills the display case from them;
  `speculate: {types, ul, events, max_days, buy_at_estimate}` keeps goods of those types from ordinary buyers until
  an Upper Levels buyer with tips unlocked comes (`ul`, default true), a market event lifts their price (`events`,
  default true) or they have waited `max_days` (default 21), unless the shop is short of cash; `buy_at_estimate`
  (default false) buys either style's types like `buy.prefer_types`. `perfume: {buyers, sellers, min_sale, min_lot,
  buy_at_estimate}` keeps neuroactive perfume (and Eleanor's self-refilling bottle) as a tool and sprays it on a
  buyer whose staged goods are worth `min_sale` (default 200) and still fit the budget after +15 %, and on a seller
  whose lot is worth `min_lot` (default 300) and, 15 % cheaper, within the buying limits; it buys perfume at its
  estimate unless `buy_at_estimate: false` (gym only: the live autopilot does not spray). `buy.no_singles: true` buys
  only wholesale lots (and engine inputs). `sell.hot_max_heat: N` stages stolen goods at heat N or less for any
  buyer but Security.
  `storage_bays: true` keeps owned storage bays as fixtures, fills them each morning with stock (most valuable
  per cell first) and stages goods for buyers and the case straight from them; with `"storage_bay"` in
  `jackson`, Dr. Jackson's bays are bought until two are owned.
  `inspect: "injectors"` inspects a seller's Immunivax injectors with the magnifier (then held as a tool)
  and confronts one whose indicator, body, label or serial shows a fault, through the game's confront
  dialogue (exposed at its true value less 15 %). `FENCE_DEMAND_TRACE=DIR` writes one line per buyer,
  seller and day to `DIR/s<seed>.jsonl` (what each buyer wanted, why nothing was staged, what sellers
  offered and what was bought).
  `appraisal: "join"` (`sim/gym/Appraisal.cs`, docs/re/APPRAISAL.md) joins the appraisal board on its offer
  (else asks it back in a week) and accepts each promotion, both while the fee leaves the day's floor
  covered; the certificate goes first into the display case, the kit (magnifier, guides, certificate) is
  stowed and never sold, and each appraisal client's items are estimated at `appraisal_aim_pct` (default
  105, the top of the perfect band) of the value read off the item's cues with the guide's table. The
  summary's `player.appraisal` and each day's `appraisal` give rank, fees, grades and refusals.
  `fakes: "honest"` (`sim/gym/Labels.cs`, docs/re/LABELER.md) reads each owned Immunivax injector and
  cigarette pack claimed genuine off the magnifier's view every morning and labels one showing a fault to its
  truth with the labeler (the day-4 labeler is put away), so it sells to any buyer at its true value.
  `labeler: "per_buyer"` adds: identified fakes are labelled genuine while a buyer whose type cannot expose
  that claim is at the counter (and back after), and open-box, rust, dent, expiry and replica claims go to the
  best condition each morning; relabelled goods stay out of the vending machine. `player.labels` counts them.
  Machine modules (`sim/gym/Modules.cs`, docs/re/MODULES.md §5), all off by default, bought from Dr. Jackson
  at the counter as engine inputs and fitted by dropping them into the machine's module grid:
  `alarm: "perfect"` buys the security alarm and modules reaching performance 50 (overclock + performance,
  two overclocks, or four performance; refitted best first when worn or burnt-out modules drop it below 50),
  keeps a charged cell in it and skips insurance on nights it stops every burglary; `transmitters: ["sec", "bm", "ul", "rev"]` fits one transmitter per faction (implies the
  alarm); `jackson_module_intro: "buy"` buys the introduction's three modules; `bay_modules: "nightly"` holds
  a new hydroponic bay's planting up to `bay_modules_wait` days (14) for performance 100; `projector_modules:
  "tier1"` buys and powers a mirage projector with fineness and quality modules; `furnace_eco: true` fits two
  eco modules beside the blast module. The summary's `player.modules` gives the days, fits and spend.
  `scavenge_max_wound: k` skips the dump while the wound (`woundState`, which the night log tells: +2 a minor,
  +5 a major, −4 a night once stable) is above k; above 5 the store door is shut until it heals.
  `short_rent_buy_pct: P` (off by default): while the cash is under the day's rent floor and rent day is at
  least `short_rent_days` (3) away, buy lots at P % of their estimate or less, down to `short_rent_reserve`
  (0) cash, instead of holding cash that cannot reach the rent (the `rock_bottom` route, docs/research/ROUTES.md).
  `production` entries: `{"action":"start_wine","machine_uid":N}` starts that bottle first,
  `{"action":"empty","machine_uid":N}` pours a finished bottle out,
  `{"action":"print","machine_uid":N,"size":"small|normal|large","count":k}` presses the bottle
  printer (`auto_print: true` keeps four empty vessels printed while selling water); others are
  recorded only. The
  engine itself: wine on cozy/normal (WINEMAKER, then RETIRED_FARMER's bay with bloomberries, water,
  nutrient tablets and batteries; undrinkable wine is emptied and reused; moldy berries are never
  fermented; only the berries, rations and water the empty bottles and bays can use are held).
  The furnace (`sim/gym/Furnace.cs`, docs/re/PRODUCTION.md §3): Dr. Jackson's day-6 furnace (or a later
  one from him), then the junk module and the blast module, one at a time, while the cash covers the
  next rent; each morning ingots go to stock, junk-made scrap back in, a charged cell into the battery,
  a flux and all scrap and ore into the input, and six junk on nights with too little metal to smelt.
  Scrap, ore, flux, modules, one spare cell and two conversions' worth of junk are held; ingots are
  stock. The summary's `player.furnace` and each day's `furnace` give its totals and state
  (`net_per_week`: ingot value made less inputs consumed at their estimate and charge at the price
  paid for cells).
  Service ids may omit `_SERVICE`.
- No `directives.json` within `-step-timeout` seconds (default 600): the day is played on the base
  directives and `result.json` says so in `note`. A rejected file (bad enum) does the same.
- `result.json`: cash and net worth start/end/delta, sales and purchases, one line per customer
  (`customers`: name, faction, intent, the counter's choice and its rationale, sold/bought value,
  walk-outs), `cash_flow` (rent, mortgage, services, permit, bribe goods, Wild upgrades, burglary, …),
  `aug_events` (with each kill's cash and rep change), `smuggler_bay` (what is concealed, what did not
  fit), crimes by kind (also in `risks.record.crimes`, with `evidence_days_to_100`), rep deltas, rent,
  permit,
  bribe owed, crime, evidence, aug results, production, `directives_used`, `game_over`.
- Works with `-warm`; the first snapshot is the warm day.

## Strategy knobs, starting perks, warm starts

- **Knobs** (`Knobs.cs`, `KnobHumanRole.cs`, `GymHeuristic.cs`): each strategy choice of the human role
  and the in-process heuristic behind a named knob, for controlled experiments
  (`docs/research/EXPERIMENTS.md` lists them). `-human knobs` with no knobs plays byte-for-byte like
  `-human default` (a gym test holds it to that).
- **Starting perks** (`-perks alert+bad_lease`): validated as the new-game screen does
  (`NewGamePerksUI`): known ids, at most 3 points, at most 4 + Σ `maxSlot` perks, no incompatible pair;
  start type and difficulty do not restrict them. The screen blocks perks on a player's first run; gym
  runs are always first runs (saves are stubbed), so that gate is not applied. Recorded in `sim_run`
  and the summary (`perks`).
- **Warm start** (`-warm "day=108,cash=6180,rent=1347,rent_days=4,rep=lower_levels:88.3+upper_levels:36.5,attract=700,permit=1,favor=0,upgrades=SHOWCASE_I+RENOVATION1,perks=alert"`):
  a new game, then the given state through the game's own code: upgrades by `NetworkUpgrade.Unlock`
  (each upgrade's unlock action runs, e.g. +150 base attractiveness per renovation, so give `attract`
  after them or leave it out), reputation by `StoreReputation.SetReputation` (rep perks re-evaluated),
  and the day, cash, rent, permit and favor fields the game's own logic reads. Implies `-skip-intro`.
  Not carried over: stock, machines and their contents, the crime record, story progress. The state
  the run starts from is read back into `sim_run` and the summary (`start_state`). `-days` counts from
  the warm day.

The bridge must run in `act` mode for its policy to play the counter: in `observe` it answers every
decide with `defer`, and the gym falls back to the human role for every customer (for `-human player`
that is the executor's own counter policy, so observe mode = the bridge supplies only directives).

## How it plays

- **Episode** (`Episode.cs`): new game from `NewGameData` (difficulty, start, seed), then per day
  morning → window → open → each customer → close → night → sleep, each a `Step()`. Ends at the
  requested day count or at `PlayerStore.ExecuteGameOver` (`rent`, `arrest`, `buyout`, …).
- **Same reads, same moves as the mod**: `GameBarter`, `GameDay` and `BarterExecutor` are compiled
  from `mod/src/Fence.Mod` into the gym; contexts, option ids, labels, signatures, outcomes and
  snapshots come from `Fence.Core`. Outcomes carry `actor` (`mod` for executed choices, `human`
  for the gym's defaults).
- **Shared with the live mod**: the executor's decision files (the list in
  `mod/src/Fence.Autopilot/Fence.Autopilot.csproj`) also compile under the mod's rules for the live
  autopilot (`docs/STREAM.md`): C# 9, no `Episode`, file I/O, reflection or threads in them. What they
  need from the driver goes through `IExecutorHost` (`ExecutorHost.cs`; the gym's side is
  `Episode.Host.cs`), and every item move or machine action an engine makes goes through `Chore`
  (`Chore.Accept` for `GraphUtils.TryAcceptAll`, `Chore.Act` for a helper call): headless they are
  the game calls themselves, live the autopilot's gated hands. `make test-mod` fails when one of
  them needs a capability or moves an item any other way.
- **Determinism**: `UnityEngine.Random` is a seeded, state-restorable copy of .NET's seeded
  generator; coroutines (`StartCoroutine`, `WaitForSeconds`, `MetaHelper.WaitAndDo`) and
  `UnityEngine.Time` run on a virtual clock that advances only when the gym pumps. Same seed,
  policy and options give a byte-identical trajectory, in a fresh process or a fresh load context.
- **Recording**: events flow to the bridge like the mod's (the bridge records sim sessions in its
  own log dir); the gym also writes its own trajectory in the `fencedata export` shape, with
  synthetic timestamps.

## Marking sim data

`hello.mod_version` is `<Fence.Core version>+sim` (SemVer build metadata), which the bridge logs
and `fencedata` copies into every day's `sessions[].mod_version`. The run parameters go out as the
first event, `sim_run` (`backend`, `seed`, `difficulty`, `start`, `skip_intro`, `human_role`,
`days`). Proposed protocol change: an optional `hello.sim` object with those fields (schema,
`internal/protocol.HelloRequest`, `fencedata.SessionInfo`), so consumers can filter without
parsing versions; until then use the suffix.

## Human-role defaults (`HumanRole.cs`)

Each default taken is counted in the run summary and recorded as a `human_action` with
`sim_default: <key>`.

| Decision | Default (`-human default`) |
|---|---|
| Window, before opening | Most valuable owned item of each of the 11 display types; never stolen or contraband (`window`) |
| Buying customer | Stage owned items `IsClientBuyingThisItem` accepts, most valuable first, within budget, at most 6; never stolen/contraband to security (`stage_for_buyer`) |
| Selling customer | Move their items priced at or under the tablet's estimate to the back counter (`stage_from_seller`) |
| After each customer | Unsold staged items go back to stock (`unstage_leftovers`) |
| Bridge defers or fails | `dismiss` if offered (`counter_fallback`) |
| Customers outside coverage (story, security, info dealer, barter, …) | Their dialogue plays; then dismiss, or force-dismiss when not dismissable (`customer:*`) |
| Dialogue choices | Buyout if offered; else pay rent; else don't pay; loan shark: decline; shady inspector: bribe only when holding serious contraband; otherwise the first enabled choice (`dialogue:*`) |
| Business permit | `-permit buy` (default): buy when cash covers the fee and the rent due; otherwise "later" on the first visit, then refuse (which commits OPERATING_WITHOUT_LICENSE, 50–170 evidence a week). `-permit refuse` never buys (`dialogue:permit_*`) |
| Night | Visit the dumping ground and scavenge until out of time, bring the loot to stock; phone Wild when an upgrade list is set (`night:*`). Before 2026-11-09 23:30 only the first night's searches happened: the outing never arrived headless (below) |
| Wild's Network (Joe's visit, reminders, the night phone) | Buy, in order, each upgrade of the list that cash + favor covers while leaving the rent due (favor is spent first, as the game does). `baseline`: LANDLORD_GRACE (offered again after every missed rent; bought even when the rent is not covered), SHOWCASE_I. `growth`: grace, both referrals, both showcases, three renovations. `none` (`wild:*`) |
| Services (the end-of-day tablet, after the night out) | Insurance and janitorial, each on while cash covers its nightly cost on top of the rent due and the services already kept; then the tablet's rules (mandatory on, cooling-down off, the dearest dropped while unaffordable) (`night:service_*`) |

`-human prudent` holds a week of each service in reserve on top of the rent instead of one night.
`-human bare` never switches optional services on (the role every run used before the baseline; an
uninsured store is burgled nightly and garbage halves attractiveness, only janitorial clears it).

Upgrade lists, 20 seeds x 150 days, normal, `-human default`, the in-process heuristic: `none` lives
to a median day 69 (16 rent, 4 arrest), `LANDLORD_GRACE` 76, `baseline` 76 with more customers a
day (6.1 vs 5.6), `growth` 55 (the spend does not pay back before the rent wall).

## Play time

`EmporiumEntry.Update` runs every item's `onOneSecondItemFunc` once a second of play: fridge, heating
plate, chem finisher, auto-butcher, gene scanner, container temperature and its reactions. The gym
passes play time explicitly: `-sec-per-customer` (default 80) after each customer and `-sec-per-day`
(default 255) for the rest of the day, 63% before opening and 37% after closing; each second pumps
the virtual clock one second and then runs the callbacks. Calibrated from the two live sessions in
`var/log` (hard, days 106–109, 0.46D): the shop was open 268, 576, 724 and 586 s; 9 and ~7 covered
customers on the two days with decisions (724 + 586 s / 16 ≈ 82 s each); waking to opening 70 and
254 s; closing to sleeping 81, 41, 131 and 121 s (≈ 162 + 94 s). Few samples: recalibrate as live
logs accumulate. Item render handlers are marked alive before each pass and before sleep
(`GameItemElement.IsDestroyed` would otherwise call every item destroyed and the nightly chemistry
pass would skip them all).

## Game data read from the install

`sim/headless/GameAssets.cs` (AssetsTools.NET with UABEA's `classdata.tpk`, fetched once into
`var/tools/`) writes, keyed on the hash of the asset file each comes from:

- `var/gamedata/loot_tables.json`: TableMaster/TableGroupMaster's ScriptableObjects (`level1`), fed
  through the game's own `LootRegistry.LoadVanilla*` (gym and `sim/extract`).
- `var/gamedata/item_shapes.json`: each item sprite's grid footprint (`resources.assets` sprites, the
  texture's stored mip level 4 alpha), served to the patched `GameItem.BuildShapeFromSprite`.
- `var/gamedata/loc_en.json`: the English string tables (addressables bundle), served to the patched
  `LocHelper.Get`/`HasEntry` in the gym (`sim/extract` keeps keys, which are its ids). Smart-string
  formatter options are reduced to positional `{n}` substitution.

## What the headless host fakes (could change economic outcomes)

1. **Play time is a calibrated constant** (above), not a player's pace; nothing waits on the
   per-second machines (a player would run the clock while a still finishes).
2. **No `Update`/tween logic**: walk-in/out tweens are replaced by calling `StoreClientMono.OnArrived`'s
   logic directly; anything else driven by a presentation `Update` or a DOTween callback is absent.
3. **Presentation managers stubbed** except the logic the gym un-stubs (dialogue flow, bargain
   panel, tablet trade buttons, shutter, arrival, map outings, blackscreen sequences) or replicates
   (Wild's Network: `WildNetworkUI.Open` becomes a host hook; the gym runs its non-drawing half, the
   purchase of `OnUseClicked` and `OnWindowClosed`). Map outings need two shims: `EmporiumEntry.OnLeaveStore`
   (it only hides the store's windows) is stubbed, and the store's two scene roots that
   `OnArriveStore` toggles are given empty objects; otherwise both throw inside the travel's blackscreen
   callback, which swallows it, and the outing never arrives (no attempts reset, no next-day customer
   cost, no return to the store). Still lost: phone orders to suppliers,
   appraisal/gunsmith/info-dealer/barter tablet modes, inspection confront panel, the scanners.
4. **Typewriter and click timing**: a dialogue line counts as shown on the next pump; the game's own
   delays still order end actions. A line nothing advances (Dr. Jackson's return) is clicked through.
5. **RNG stream**: the game's draws come from the gym's seeded generator, not Unity's; client sprite
   variety (`ClientSpriteVariety.Pick`) never draws.
6. **Saves stubbed**: `InitialSave` does not run, so `runID` is `sim-s<seed>` and `runNumber`/the
   total run count stay 0 (content gated on run count behaves as a first run). No run starts from a
   save; `-warm` approximates one (day, cash, rent, rep, attractiveness, permit, upgrades, perks) but
   not its stock or machines.
7. **Player settings** fixed by the gym: auto-advance dialogue on, auto-call-next off, move bought
   items to inventory on.
8. **Mod sprites**: `SetSpriteAndShapeFromMod` still reads a stubbed sprite (no mods are loaded).
9. **The human's half** (above) is a simple default, not play. Not modelled: bribing the trade
   board official (from day 45 he buys items marked as donations until the week's bribe is paid,
   which spares that week's OPERATING_WITHOUT_LICENSE but grants no permit), production (no machine
   is loaded or started), the drug dealer's weekly cartel quota (the run ends day ~8,
   `cartel_retired`), landlord offers (the water vending machine, +150 weekly rent, is accepted as the
   first choice).

Checked against the owner's saves and bridge recordings and found faithful (2026-11-10): the day's
customers (`StoreClientManager.GenerateClient`, once at opening, every queued client served), buyer
budgets, and the dead shortage/surplus clients (`docs/re/PRODUCTION.md` §5). The summary's
`end.client_visits` is the game's own `permanentClientTracker` (visits by client id), the same
dictionary a save holds, for comparing who came. Display names are shared across ids: the
`waterSpace2` water shopper shows as "Spacer".

Moonshiner, organ trader, pharmacist, street food vendor and scav starts play exactly like `none`:
the game itself marks them `comingSoon`/hidden (`StartingScenario`) and has no setup for them.

## Next: night phase and production policies

- New decide kinds for the night (`night_services`, `upgrade_buy`, `outing`), sent by the gym first
  (`IHumanRole.Services`/`Upgrades` already take `NightView`/`UpgradeView`) and then by the mod, with
  option sets built in `Fence.Core` so both sides share them. `NetworkUpgrade.state`: 1 is offered
  (buyable once its prerequisites hold and it is off cooldown), 2 unlocked; a missed rent puts
  LANDLORD_GRACE back to 1.
- Staging, window and dialogue choices as decide kinds, so a policy (not the gym) owns them; the
  dialogue view (`DialogueView`) is already the context.
- Production starts: the machine/crafting actions as moves (item time now runs).
