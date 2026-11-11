# Progress log

The scoreboard for "something that can play the game well". Newest entries first. Numbers
come from the headless gym (real game rules, see `sim/gym/README.md`) unless stated.

## VOD kit — 2026-11-11: chapters, title and clips from the recorded feed

**Result.** `cmd/fencevod` (`internal/vod`) turns a run's feed (or decision log plus the feed's narrator
lines) and the OBS recordings' start times into one kit per recording: YouTube chapters, a spoiler-free
title, a description with the result below a spoiler line and tinyBuild's copyright notice, tags, 30–60 s
highlight windows and an ffmpeg `cut.sh`. Local only: no upload, no model call. Moments are ranked by the
narrator's `show.Priority`, so bets, forks, the ghost band's pace and other kinds slot in once ranked there;
a run's format card is read from the feed's JSON. On the dress rehearsal (seed 2950001, made-up OBS start
02:36:40): 3 h 12 min, 24 chapters (weeks in pairs, the board's join and three promotions, four plan pivots,
the permit, Georgio, a counterfeit, "How it ends"), 12 clips of 30–45 s, eleven ending on the Shark's line;
split into two recordings, days 1–50 and 50–98 with their own chapters. Cutting from a synthetic test-pattern
recording gave every clip at its planned length. Runbook, OBS recording advice and the rights check
(tinyBuild's influencer policy allows monetised videos with a copyright notice; the soundtrack's Content ID
status is unknown): docs/STREAM-OPS.md section 7.

## Trading styles — 2026-11-11: six ways to trade that win hard 60/60

**Result.** The counter policy every route shared is now one style of six. On top of the `appraiser` route,
`apothecary` (medical specialist), `hardware_store` (household specialist), `speculator` (holds medical, food,
water and materials for shortages), `bulk_buyer` (wholesale lots only, a medical supplier's sign), `fair_dealer`
(never haggles) and `clean_shop` (refuses stolen goods) each win **60/60 on 60 screening seeds (7100001–060) and
60/60 on 60 fresh seeds (7110001–060)**, median win day 98–112; the specialists lose 12–14 days and the speculator
23 days (paired) against the generalist, the other three are within noise. Their fingerprints differ: resale two
thirds medical or four fifths household for the specialists, twice the reference's sales into market events for the
speculator, all buying in wholesale lots and a quarter more of it for the flipper, half as much buying again and a
fifth more sales for the fair dealer, nothing hot for the clean shop (docs/research/TRADING-STYLES.md). Aggressive
haggling loses every run to the Lower Levels mob (day ~58). Without the Appraisal Board every style loses (0/60);
none closes the Board-free gap (the best adds ~200 of net worth by day 98).

**Levers** (`sim/gym/Style.cs`, default off; summaries byte-identical to a957b56 on eight hard and normal seeds):
`specialty`, `speculate`, `perfume` (gym only: sprays neuroactive perfume on big trades; works, too few uses to
matter), `buy.no_singles`, `sell.hot_max_heat`. After merging main 8a1713a the summaries match main's on six hard and
normal seeds (`appraiser`, `smelter`, `rancher`, `fountain`, `pharma`) and four style runs reproduce their measured
summaries exactly. The demand trace now records each sale's price, cost, days held and tips, and every counter
choice.

## Morning summary (2026-11-11, end of run 2: 2026-11-10 16:30 → 2026-11-11 10:30 AST)

**Hard is solved by the executor, and so is challenge.** At the start of run 2 the best hard policy won 8/60.
Now `hard_income_cash_sprint.json` wins **60/60 at median day 98** (fresh seeds; the idle runner's
independent scoreboard: 60/60, median 90), and every one of the thirteen routes wins 57–60/60 by day 84–126.
Challenge: `challenge_best.json` and the thirteen `routes/challenge/` files win 57–60/60 at day 182–203 (the
loan shark's first offer on day 70 + 16 payments is the floor). What moved it, in order: the **Appraisal Board**
(6/60 → 59/60; ~2.2k a week of fees, the owner's own biggest edge), the **cash buyout** (`buyout_by_day: 140`,
found by drift declining the mortgage in an LLM game: day 175 → 105), **no insurance while saving** (day 105 →
98), a **fair aug detector** (visible tells only; precision and recall 1.0, wins unchanged), and fidelity
fixes (Dr. Jackson's purchases, outing door gates, the return prompt).

**The LLM layer keeps the wins but does not beat a tuned executor.** Delta planners on the cash base: Qwen 6/6
(median 105), GLM 6/6 (116), drift 4/4 so far (164) against the executor's 98; their early trading tweaks cost
net worth, and their one consistent edge is the mortgage call (on 2800001 all three bought for cash on days
105–126 where the executor's rule signed and won on day 231). The hybrid "LLM proposes, gym decides" planner
(v2: commitments judged to the end of the term, gate against base) no longer signs into repossession but does
not beat the executor, and Qwen's proposals equal blind levers. The LLM's value so far is discovery (the cash
buyout) and, for the stream, legible reasons. Distillation r2's outcome-backed dataset is built; training waits
for a free 5090 (`docs/research/DISTILL.md`).

**The stream path is built end to end; nothing has run in the real game yet.**
- Live autopilot M1–M3 in the mod (`docs/STREAM.md`): counter and staging, mornings, nights, phone, machines,
  the Appraisal Board live, and the gun on visible tells; default policy = the day-98 cash sprint. Owner tests:
  §8 (one day), §9 (a hard week), §12 (augs).
- Show layer (`docs/SHOW.md`): OBS panel, fact-checked narrator (every spoken number checked), personas per route,
  beats, small beats, phrasebook against repetition (1.5 lines a minute, no three-word phrase three times).
- Ops (`docs/STREAM-OPS.md`): `make stream-preflight`, runbook, and a 3 h 10 min dress rehearsal of the live stack
  on the gym (one hard game to a day-98 cash buyout, no crashes, 16 bugs fixed).

**For the owner:** (1) run the M1/M2/M3 live tests; (2) set an `ANTHROPIC_API_KEY` if you want the Haiku narrator
(recommended for stream night); (3) decide when the 5090 can train distillation r2 (~2.3 h); (4) the Appraisal
guide's value table sits in gym code — keep, or move to a local file. Still running at handover: the challenge
LLM league (`league/t9-challenge-llm.json`; read with `fenceleague report`), two drift games of t8, the
standalone Qwen server on :9091 (shutdown steps in the 1d3a173 entry), and the idle runner.

## New production archetypes — 2026-11-11: rats make a route, the suppliers and the printer do not pay on hard

**Result.** Two new hard routes, both 120/120 on fresh seeds 4700001–060 and 4710001–060: `rancher` (the Rat
Wrangler start with a rat engine; paired win day against `appraiser` −3.6 ± 4.3 and −8.4 ± 4.7, medians 90 and 83)
and `pharma` (the appraiser plus Dale's injector phone; +2.3 ± 5.0 and −2.2 ± 3.2). Everything else on this axis
wins 60/60 too, because the Board carries hard, but slows the cash buyout: beer + shareholder +5.7 ± 3.8 days,
pharma + shareholder +6.7 ± 3.4, the 3D printer on the `gunsmith` route +15.8 ± 7.0 (chips bought in the saving
window); beer alone, the chem phone and the miner shareholder are neutral. Details: docs/research/ARCHETYPES-NEW.md.

**Fidelity.** (1) Supplier free deliveries and shareholder dividends land on the front counter as ours and the night
destroys that counter; the gym kept only the furnace's ore, so the 2026-11-10 beer-shareholder test lost every
dividend case. They are stowed now (gym and live autopilot). The faithful verdict is the same. (2) The rancher
kit's rats ran loose in the stock grid (breeding, eating stock food, fouling it): the appraiser policy on that start
was 7.6–13.5 days slower than on the apprentice start, and cutting them up on day 1 removes the gap.

**Levers** (all off by default; summaries byte-identical to a957b56 on 12 runs of 150 days: appraiser, smelter, appraiser on
the rancher start, normal defaults): `beer` / `pharma` / `chem: "phone"` with `<key>_shareholder`, and
`miner_shareholder` (`sim/gym/Suppliers.cs`, formerly Beer.cs); `engine` part `rats` (`sim/gym/Rats.cs`; schema,
protocol and delta prompt updated); `printer: "run"` (`sim/gym/Printer.cs`). Rats are food-bound: sellers offer
~450 kcal of cheap food a day, so one breeding pair is all it feeds and most colonies die out by day ~27; a feed
bay would lift that. Not modelled: the chemistry lab and aquaculture (fish feed has no seller).

## Without the Board — 2026-11-11: the Board is 55–71 % of every route's money; nothing wins hard without it; a fountain route

**Result.** On 30 fresh hard seeds 5100001–030 the Appraisal Board pays 55–71 % of every route's money, the counter
(with the vending machine) 22–31 % and Georgio 7–17 %; production nets ~0 in cash before the buyout (inputs and
machines eat the sales; the smelter's furnace is best at +2 800 a run). With `appraisal` removed, the twelve route
policies (`sprinter` plays `appraiser`'s) win **1 of 720** runs (60 fresh seeds 5100001–060 each; median last day 133–196; rent and repossession): a Board-free
shop nets 700–900 a week before rent from day 70, half what the hard mortgage's ~1 700 a week needs. No Board-free
route reached 50/60: overhead cuts, Georgio's peak and early payback, the permit, the alarm and every mortgage-timing
lever give 0/30 on `appraiser` and `smelter` (screens 5200001–030), and the best water engine (new
`purify_raw` + `purifiers: 4` + `jackson_equipment`) survives +32.7 ± 8.0 days on 60 fresh seeds 5300001–060 but
wins 0/60. Tables and why: docs/research/ROUTES.md, "Without the Appraisal Board".

**New engine: the water fountain** (`fountain: "buy" | "landlord"`, `sim/gym/Fountain.cs`, shared with the live
autopilot): the game's night water vending, which sells a fixed set of bottles from its Base/HQ/Pure slots every
night (VendingFountainHelper.HandleNight). On `water_baron` it sells ~5 800 a run and leaves the win day unchanged,
so it ships as a fourteenth route, **`fountain`**: 60/60 on 5200001–060 and 60/60 on fresh 5300001–060, median day
112 (paired Δ win day +0.9 ± 5.2 and −6.9 ± 9.8 against `water_baron`). Without the Board it adds at most +9 days.

Also: every gym run on main since 41c81d0 crashed at start (Boot.Init named Newtonsoft before the game's assemblies
resolved); fixed on this branch. All new levers are off by default: summaries byte-identical on 10 hard and 10
normal seeds. `sim/lab/scripts/income.py` prints the income split from any set of summaries.

## Challenge routes — 2026-11-11: thirteen challenge files; no insurance and no cash upgrades save the weak three

**Result.** `sim/lab/scripts/directives/routes/challenge/<name>.json` is a challenge version of every route
(docs/research/ROUTES.md, "The challenge routes"). On **60 fresh challenge seeds 3410001–060** eleven of the
thirteen win 60/60 (Wilson 95 % CI 94–100 %; median win day 182, `water_baron` 189), `outlaw` 59/60 (91–100 %, day
189) and `rock_bottom` 57/60 (86–98 %, day 203); every win is the mortgage. The three that lost a fifth or more of
their runs with the three challenge changes alone now win: `outlaw` **59/60 (repo 1)**, `water_baron` **60/60** and
`rock_bottom` **57/60 (rent 3)**; with the changes alone the same seeds give 42/60 (repo 18), 41/60 (repo 19) and
34/60 (repo 17, rent 9).

**The recipe** is each route with the three challenge changes (`challenge_best`'s: no `buyout_by_day`, `buyout_save`
of insurance, upgrades and vending from day 1, `georgio_peak_days: [40, 43]`) and two existing levers for the whole
run: `services: ["JANITORIAL_SERVICE"]` (never insured) and `upgrades_cash_max: 0` (Wild's upgrades for favor only).
`rock_bottom` also takes `short_rent_days: 3`.

**Why.** `buyout_save` holds the insurance and Wild's upgrades back only until the mortgage is signed. In the first
eight weeks of the mortgage the weak three then pay ~570–600 a week in services (insurance is ×3 on challenge) and
~310–340 in Wild's cash upgrades (renovations, showcases, referrals, taps), against payments of ~2,500 a week.
Without insurance the services cost ~95–120 a week and burglaries take ~145–175 of goods. The payment floor was not
the constraint: every floor lever leaves the won runs unchanged. Screens, 30 seeds 3400001–030, wins (paired Δ win
day against the three changes alone):

| Arm | `outlaw` | `water_baron` | `rock_bottom` |
|---|---|---|---|
| the three changes | 23 (repo 7) | 16 (repo 14) | 19 (repo 8, rent 3) |
| + `mortgage_floor_stock` 0.5 / 1.0, `mortgage_floor_days: 2`, `repo_reserve_payments: 2.5`, `mortgage_float: 3000`, 7-day sell-down | 23–25 | 13–16 | 19–21 |
| + `mortgage_margin: 1.25` (signs later) | 17 (+37.5 days) | 20 (+15.2) | 22 (+32.5) |
| + `upgrades_cash_max: 0` | 23 | 22 | 24 |
| + no insurance | 29 | 29 | 24 |
| + no insurance, power service on | 20 | 21 | 8 |
| **+ no insurance + `upgrades_cash_max: 0`** | **30** (−6.7 ± 3.6) | **29** (−0.9 ± 4.6) | **26** (0.0 ± 8.8) |

On the other ten routes (nine policies: `sprinter` plays `appraiser`'s runs) the two levers won 270 of 270 screening
runs against 265 for the three changes alone (paired Δ win day from −6.3 ± 4.1 for `farmer` to +1.9 ± 2.9 for
`gunsmith`), and 540 of 540 on the fresh seeds against 530, so every challenge file takes them. `rock_bottom` also
loses early rents on challenge: on 100 seeds 3400001–100 played to day 15, 13 runs lose the rent on day 7 or 14 with
the hard route's `short_rent_days: 2` and 7 with 3; selling harder (`liquidate_min_pct` 65, `min_pct` 50),
`engine_reserve_max: 0` and `short_rent_buy_pct: 100` save at most one (12–13). Its remaining losses on the fresh
seeds are those rents (days 7, 7 and 14): the shop opens with no cash and has not sold enough by then.

Builds: the gym of main 2141a86 (`var/sim/gyms/search-2141a86d0f57`; main c9e7581 leaves `sim/gym`, `sim/headless`,
`mod/src` and `compat` unchanged); no gym change. `TestRouteFiles` (`internal/route`) now also checks
`routes/challenge/*.json`.

## Challenge — 2026-11-11: the mortgage on day 70 wins 60/60 at day 182

**Result.** Challenge is winnable with the existing levers. The new
`sim/lab/scripts/directives/challenge_best.json` wins **60/60 on fresh challenge seeds 3210001–060 (Wilson 95 % CI
94–100 %), median win day 182** against 58/60 and 210 for the hard incumbent (`hard_income_cash_sprint.json`) on
the same seeds; paired Δ win day −22.8 ± 2.6. The screening block 3200001–060 gave 60/60, 182 against 59/60, 210
(−20.6 ± 2.2). Every win is the mortgage (`buyout_mortgage`): 42 of 60 sign on day 70, 13 on day 77.

**What changes from hard** (docs/re/MITIGATIONS.md §10): only the rules table. The shop costs 36,000 (+100 a day
after day 50, so ~40,000 by day 90), the mortgage takes 20 % down at 100 % a year, insurance and Appraisal Board
fees are ×3, Wild's upgrades ×2, the permit 3,000, rank progress and reputation gains are slower and losses
harsher. The appraisal fees themselves do not scale: the Board still pays ~50k a run.

**Baseline.** The hard incumbent wins 59/60 on challenge (3200001–060), all by mortgage, median day 210: its
`buyout_by_day: 140` answers the loan shark "later" until day 84 and the cash buyout never comes, so it signs on
days 84–98 and finishes 16 weeks later. Ten of the thirteen hard routes win 57–60/60 the same way (median
203–210); `outlaw` falls to 42/60, `water_baron` to 23/60 and `rock_bottom` to 22/60, mostly to repossession
(docs/research/ROUTES.md, "Challenge").

**The policy** = the hard incumbent with three changes: no `buyout_by_day` (the cash buyout is out of reach: the
price rises 700 a week while the rent grows 10 % a week, so savings never close the gap; `buyout_by_day: 200` won
29/30 with one cash buyout but was +29 days slower), `buyout_save: ["insurance", "upgrades", "vending"]` from day 1
(no ×3 insurance, no ×2 upgrades, no vending upgrades until the mortgage is signed) and `georgio_peak_days: [40, 43]`
(sell down so Georgio borrows more and repays four times as much on day 69). Median cash on the morning of day 70
is 16,617 against 11,743.

Screens, paired with the incumbent on 3200001–030 (sa1_peak and its floor variants on 060):

| Arm | Wins | Median win day | Paired Δ win day |
|---|---|---|---|
| mortgage at the first offer (no `buyout_by_day`) | 29/30 | 196 | −11.3 ± 4.5 |
| + no insurance from day 1 / + Georgio peak | 29/30 / 29/30 | 196 / 196 | −10.6 ± 4.9 / −10.6 ± 5.3 |
| + `upgrades_cash_max: 0` | 30/30 | 189 | −17.4 ± 3.7 |
| + save insurance, upgrades, vending from day 1 | 28/30 (repo 2) | 182 | −21.3 ± 4.3 |
| the same from day 14 / 28 / 42 | 29/30 each | 182 / 189 / 189 | −19.8 / −18.0 / −16.2 |
| **+ save all from day 1 + Georgio peak (`challenge_best`)** | **60/60** | **182** | **−20.6 ± 2.2** |
| + `mortgage_floor_days: 2` / `mortgage_floor_stock: 0.5` | 60/60 / 56/56 | 182 / 182 | −20.6 / −20.7 (no change) |
| + `mortgage_save_from_day` 56 / 63 / 66 | 56 / 56 / 58 of 60 (repos) | 182 | −18.7 / −20.9 / −21.2 |
| + `georgio_collect: early` | 27/30 (repo 3) | 189 | −18.3 ± 4.6 |
| `mortgage_float: 3000`, `repo_reserve_payments: 2.5`, `mortgage_cover: 4` on save-all | 28/30 each | 182 | −21.3 / −21.3 / −17.9 |
| `permit: bribe` | 8/30 (execution 19, arrest 3) | 182 | — |

**The binding constraint is the mortgage offer, and day 182 is its floor.** The loan shark offers on a rent
morning only when the cash is at least twice the down payment (15,000 on day 70, 16,000 from day 84) and he has
already introduced himself, which happens on the first morning the cash covers one down payment. The payments
then run a fixed 16 weeks with no early payoff, so the win day is the signing day + 112. Before Georgio's payback
on day 69 no run of `challenge_best` reaches 15,000 (at most 12,584; median 9,160 on the morning of day 69), so
day 70 is the first rent morning that can sign and 182 the earliest win. A run signs on day 70 exactly when its
cash covered one down payment (7,500) on some morning from day 56 and 15,000 on the morning of day 70: 38 of 38 such
runs on 3200001–060 and 42 of 42 on 3210001–060. Of the rest (22 and 18), 14 and 10 were introduced but 5–3,400
short of 15,000 on day 70, 3 and 3 had the cash but no introduction yet, 5 and 5 had neither; they sign a week or
more later. What kills runs is a repossession late in
the mortgage (payments ~2,500 a week): the cash held for the next payment stops all trade buying, the shelf
empties ("nothing sellable staged") and a payment lands short. It hit 1–2 of 30 in the save-all arms and none in
`challenge_best` (0 of 120).

Builds: the gym of main 2141a86 (`var/sim/gyms/search-2141a86d0f57`; main e25cec0 leaves `sim/gym`, `sim/headless`,
`mod/src` and `compat` unchanged); no gym change.

## Hybrid planner v2 — 2026-11-11: commitments judged to the end of the term; no more signings into repossession, no gain either

**Fixes** (branch `hybrid-v2`, `docs/research/HYBRID.md`). (1) A decision whose candidates, the plan in force
included, set a mortgage or buyout key (`mortgage`, `mortgage_window`, `mortgage_margin`, `mortgage_float`,
`mortgage_save_from_day`, `buyout`, `buyout_by_day`, …) before signing, with cash past one down payment, rolls to
the end of the term (16 weeks + 14 days = 126 on hard) instead of 42 days; a branch still paying at its horizon
scores as the repossession its net-worth trend projects when cash + repossessable stock run out before the last
payment. (2) The switch gate defends `base`: every candidate, the held delta included, must beat base by 1 SE
(paired); going back to base needs none. (3) Proposals run with thinking off unless the endpoint asks, carry a
`response_format` schema, and a malformed answer gets a local bracket repair, then one repair call. (4) The show:
a hybrid replay's "Today's plan" is the chosen candidate (badge "simulated choice", the reason as its why, up to
four lines); the caption drops the plan names, then the reason, before it passes the narrator's 200-character cut.

**Setup.** `league/h3-hybrid-stress.json` (old policy `hard_income_mortgage.json`, seeds 2300001–010) and
`league/h3-hybrid-cash.json` (incumbent `hard_income_cash_appraisal.json`, seeds 2310001–006), apprentice,
`vending+short_sleeper`, 4 samples, 4 rollout workers a game, Qwen-27B local (thinking off, 2 slots of its own).

| stress (10 seeds) | wins | median days | endings | signed (repossessed) | median NW d100 | Δ days vs executor (paired, 95 %) | LLM calls / game | wall / game |
|---|---|---|---|---|---|---|---|---|
| executor | 0/10 | 181 | rent 7, repo 3 | 3 (3) | 6 751 | | | 1 min |
| hybrid-blind | 0/10 | 192 | rent 10 | 0 | 6 028 | +11.9 ± 18.4 | 0 | 22 min |
| hybrid-qwen27 | 0/10 | 195 | rent 10 | 0 | 7 386 | +11.2 ± 23.6 | 47 | 43 min |

| cash incumbent (6 seeds) | wins | median win day | mean | endings | Δ days vs executor (paired, 95 %) | LLM calls / game | wall / game |
|---|---|---|---|---|---|---|---|
| executor | 6/6 | 104 | 106 | buyout 6 | | | 0.5 min |
| hybrid-qwen27 | 6/6 | 104 | 117 | buyout 5, buyout_mortgage 1 | +10.5 ± 39.7 | 35 | 41 min |

**The horizon fix works; the hybrid still does not win.** Neither hybrid signed the mortgage in 20 stress games
(h2: blind 4/5 and GLM 2/2 signed, all repossessed); 113 decisions were commitments rolled 126 days. Both now end
on the rent, a few days later than the executor on average (not significant; seeds 2300001–005 alone: blind
−7.0 ± 15.6, Qwen −5.6 ± 20.5; 2300006–010: +30.8 ± 24.4, +28.0 ± 39.3, where the executor was repossessed on two).
On the cash incumbent Qwen matches the executor: same median day 104, per seed −42, +14, −21, −14, +98, +28. It
bought out on day 69 and 90 where the executor took 90 and 132 (an all-out sell-down plus `mortgage: decline`
on an offer morning, z 1.7), but on 2310005 the cash projection dried up after a deferral it chose on day 91 and
it won by the mortgage on day 209 (executor: cash on day 111). Timing "later" against "take" moved nothing.

**LLM against blind** (stress, paired): −0.7 ± 15.9 days, 5 better, 4 worse. In h2 Qwen led blind by 26 days
because blind signed into repossession; with that gone the proposals are worth what blind levers are. Each arm
played a delta over base in 31–32 % of decisions. Qwen's calls: 0 errors in 683 (h2: 19 of 254), no repair
needed (the schema holds), 28 s a call.

**The gate is still loose with ties.** 88 of 361 played deltas cleared it at z = 1.0 exactly, and 86 of those
differ from base in one sample of four (the other three tie): one lucky future and three identical ones give
z = 1. Next: require a strict z > 1 or at least two improving samples, then rerun; and more samples on commitment
decisions, where most branches end in a buyout a few days apart.

## Distillation round 2 — 2026-11-11: an outcome-backed delta-planner dataset; not trained (the 5090 was busy)

**Goal.** A small local model (Qwen3-8B QLoRA) that plays the delta-planner role on hard well enough
for a stream. Round 1 (full directives) lost 6/6 by copying the director's `mortgage: later`. Now
the executor alone wins on the cash-appraisal base, so the student only has to name the changes
that help, or answer `{}`.

**Task.** One call, `strategy.DistillDeltaPrompt`: the morning plan prompt, then the bridge's delta
ask over the base policy. The answer is the delta JSON with `"why"` first and no prose plan. It is
played through the bridge's delta lints (`strategy.FinishDistillDelta`, `fencedistill play -delta`).

**Dataset** (`fencedistill delta`, `var/distill/r2`): 2,792 train rows and 711 val rows; val is
held-out seeds 2100006, 2300004, 2310004 and 2800002.
- **Hybrid decisions**, 29 finished games (h1–h3). The target is the candidate the rollouts played.
  - 209 train rows change the plan in force (weight 3; blind levers 1.5).
  - Both the target and the plan in force are linted first: the lints undid 131 of 340 switches.
  - Dropped: the last 14 days of lost games, and mortgage moves in repossessed games.
- **Teacher days**, 18 winning plan-mode delta games (t7, t8: Qwen-27B, drift, GLM), from
  bridge.log's linted deltas.
  - 1,150 train rows change something (weight 2); 56 repeat (0.5).
  - Values restating the base are stripped.
  - Each game is weighted by its pace against the executor on the same seed: drift's day-91 cash
    buyout ×1.93; on 2800001, where the executor took the mortgage and won on day 230, ×1.84–2.
- **`{}` days** (the executor's play): 1,267 train rows at weight 0.1, 4 % of the total weight.

Saying `{}` everywhere scores 0.677 agreement on val (change days 0.371, switch 0.435). Stats,
caveats and commands: `docs/research/DISTILL.md`, "Round 2".

**Not trained.** At 03:30–06:00 AST the standalone Qwen-27B on 9091 (27.3 GB) served
`t8-hard-llm`, the persona-routes league and `h3-hybrid-cash`. The ready-to-run commands are:
1. `DATA=var/distill/r2 tools/distill/pilot.sh qwen3-8b-r2 --epoch-rows 1200 --epochs 1`: about 2.3 h.
2. `tools/distill/gym-pilot.sh qwen3-8b-r2 <gguf>`: seeds 3100001–006, against the executor.

The pilot harness ran end to end against a stub endpoint.

## Faster wins — 2026-11-11: drop the insurance from day 56 and every hard route is bought for cash by day 84–126

**Result.** The new hard incumbent `sim/lab/scripts/directives/hard_income_cash_sprint.json` (= the cash
incumbent + `buyout_save: ["insurance"]`, `buyout_save_from: 56`) wins **60/60 on fresh hard seeds 2910001–060
(Wilson 95 % CI 94–100 %), median win day 98 against 112** for the previous incumbent on the same seeds, all 60
for cash (days 70–119); paired Δ win day −30.9 ± 11.6. The screening block 2900001–060 gave the same: 60/60,
median 98 against 112, −26.4 ± 9.6. With the cash rule and the insurance cut, all twelve routes win as often as
before and finish 36–90 days sooner on the paired mean (ROUTES.md, "Every route buys for cash"):

| Route | Wins before → after | Median win day before → after | Paired Δ win day |
|---|---|---|---|
| `appraiser` | 60 → 60 | 175 → 98 | −78.4 ± 3.1 |
| `watchman` | 60 → 60 | 175 → 98 | −77.8 ± 3.0 |
| `farmer` | 60 → 60 | 175 → 91 | −87.0 ± 3.0 |
| `vintner` | 60 → 60 | 175 → 98 | −78.2 ± 5.5 |
| `labeler` | 60 → 60 | 175 → 91 | −82.0 ± 2.6 |
| `no_vending` | 60 → 60 | 175 → 91 | −84.6 ± 2.6 |
| `gunsmith` | 60 → 60 | 175 → 91 | −79.1 ± 3.0 |
| `outlaw` | 60 → 60 | 175 → 105 | −66.4 ± 6.0 |
| `smelter` | 60 → 60 | 175 → 84 | −90.1 ± 2.7 |
| `lean` | 60 → 60 | 175 → 91 | −85.3 ± 3.4 |
| `water_baron` | 60 → 60 | 175 → 112 | −57.0 ± 9.5 |
| `rock_bottom` | 57 → 57 | 182 → 126 | −36.2 ± 10.9 |

(Seeds 2910001–060, paired against each route's previous file, 1000-day horizon, main a79bcd7 + the lever; Δ
over seeds both arms won, ± 1.96 SE. rock_bottom's 3 losses are rent on day 14 on the same seeds in both arms.)

**The cash rule alone was not enough.** `buyout_by_day: 140` on each route's file, screened on 2900001–060: ten
routes 60/60 with medians 98–126 (paired −22 to −68 days), but `water_baron` (217 against 175, +25.8 ± 11.6)
and `rock_bottom` (210 against 182) got slower, and X = 126 or 154 did not fix either (water_baron 210 / 224,
rock_bottom 203 / 210). Their savings project short on day 84, so the hybrid signs the mortgage on days 98–112
instead of 63 and wins 16 weeks later. `outlaw` was 126 at 140 and 154, `gunsmith` and `lean` the same at all three.

**The speed levers** (gym, `sim/gym/README.md`: `buyout_save` with `buyout_save_from`, and
`buyout_selldown_days`; off by default), each paired against the cash incumbent on 2900001–060 (60/60, median 112):

| Lever | Wins | Median | Paired Δ win day |
|---|---|---|---|
| **no insurance from day 84** | 60/60 | 98 | **−18.6 ± 8.9** |
| no insurance from day 56 | 60/60 (all for cash) | 98 | **−26.4 ± 9.6** |
| no insurance from day 70 / 77 / 91 | 60/60 | 98 / 98 / 105 | −23.0 ± 10.5 / −23.1 ± 9.5 / −16.7 ± 8.7 |
| `alarm: "perfect"` (Jackson's alarm skips the insurance on most nights) | 60/60 | 105 | −12.0 ± 12.6 |
| Georgio: `georgio_peak_days: [40, 43]` (lend more) | 60/60 | 112 | −5.8 ± 12.6 |
| Georgio: `georgio_collect: early` (payback on the first rent day) | 60/60 | 112 | +7.5 ± 10.4 |
| no cash upgrades from day 84 | 60/60 | 112 | +0.0 ± 0.0 |
| no vending upgrades from day 84 | 60/60 | 112 | +0.1 ± 5.4 |
| no engine inputs from day 84 | 60/60 | 112 | +8.8 ± 11.4 |
| upgrades + inputs + vending cut from day 77 | 60/60 | 112 | +21.1 ± 14.7 |
| sell down for the buyout from 7 days out (`buyout_selldown_days: 7`) | 60/60 | 112 | +0.0 ± 0.0 |
| no insurance from 77 + perfect alarm / + Georgio peak | 60/60 | 98 / 98 | −21.1 ± 10.7 / −21.9 ± 9.8 |

The insurance is the shop's largest overhead (~8 000 of services a run after day 56 under the old policy, nearly all of it insurance);
without it burglars take ~1 000 of goods a run over 33 nights, and the higher four-week gain also keeps the
hybrid on the cash path (60 of 60 buy for cash, against 51). Cutting the engine's inputs backfires: the four-week
net-worth gain drops, the projection falls short more often and more runs sign the mortgage (48 buy for cash,
against 51). The upgrade cut and the earlier
sell-down move no win day on any of the 60 seeds. The alarm and Georgio's loan do not add to the insurance cut.

**Changes.** All thirteen routes carry `buyout_by_day: 140`, `buyout_save: ["insurance"]` and
`buyout_save_from: 56`; `appraiser`, `outlaw`, `rock_bottom` and `sprinter` have new briefs (cash instead of the
mortgage); `sprinter` is the new incumbent with the shark persona. `idle_runner.py` `HARD` names
`hard_income_cash_sprint.json`, and `space-hard.json` gains `buyout_save` and `buyout_save_from`. The runner
reads the directives at main's HEAD, so it is restarted (`idle_runner.py stop` / `start`) after this branch is
merged, not before. With the levers off the build matches main: identical summaries (all but the timing fields)
on 60 hard seeds (2900001–060, the cash incumbent), 10 normal seeds (2920001–010, `-human player`) and 10
smelter-route seeds (2920101–110, the phoned-ore path).

## LLM delta planners on the cash incumbent — 2026-11-11: all win, but no faster than the executor; challenge running

**Setup.** As the appraisal round below, on the cash-buyout incumbent (`hard_income_cash_appraisal.json`,
`buyout_by_day: 140`), gym and bridge `a79bcd7` (delta prompt: `decline` is final, `later` defers, the decline
lint), fresh seeds 2800001–006, leagues `league/t8-hard-{llm,drift,glm}.json`. Drift had finished 4 of 6 games
when this was written (2800005 at day 114, 2800006 pending).

| arm | games | wins | median win day | mean Δ days vs executor (paired, 95 %) | plan coverage |
|---|---|---|---|---|---|
| executor | 6 | 6/6 | 98 | | |
| Qwen-27B | 6 | 6/6 | 105 | +4.7 ± 61.7 | 0.77–1.0 |
| GLM | 6 | 6/6 | 115.5 | +7.0 ± 51.2 | 0.91–1.0 |
| drift | 4 | 4/4 | 164.5 | +29.8 ± 113.9 | 0.90–0.93 |

| seed | executor | Qwen | drift | GLM |
|---|---|---|---|---|
| 2800001 | 231 (mortgage) | **105** | **112** | **126** |
| 2800002 | 84 | 105 | 231 (mortgage) | 119 |
| 2800003 | 126 | 238 (mortgage) | 217 (mortgage) | 217 (mortgage) |
| 2800004 | 98 | 105 | 98 | 112 |
| 2800005 | 98 | **84** | … | 98 |
| 2800006 | 84 | 112 | … | 91 |

**Reading the outliers.** Every game is won; what moves is whether the cash buyout lands before day 140 or the
run falls back to the mortgage (~day 217–238). Slow LLM games trade worse early, before any mortgage decision:
on 2800003 net worth on day 100 is 21.1–24.4k against the executor's 26.5k, on 2800002 day 60 is 8.0–10.3k
against 11.9k, so the buyout misses its window and the shark's mortgage ends the run on day 217–238. The one
big gain is the reverse: on 2800001 the executor's projection gave up the cash route and it signed (win day
231), while all three planners kept answering `later` and bought out on days 105–126. The planners use the new
levers: 81 days of `mortgage: later`, 18 of a `buyout_by_day` change, 7 `take`; one `decline` was turned into `later` by the
lint and one kept (the buyout projected); no repossession. Kept changes are mostly services, buy/sell, engine restatements and
upgrades; lint drops 716 (Qwen), 398 (GLM), 1 489 (drift, 4 games).

So on this base an LLM layer keeps the 6/6 but does not beat day ~98; its value shows only where the executor's
own projection is wrong (2800001).

**Challenge (running).** `league/t9-challenge-llm.json`: `-difficulty challenge`, apprentice,
`vending+short_sleeper`, `challenge_best.json`, gym `c9e7581`, fresh seeds 3300001–006; Qwen-27B delta at 3 games
plus the executor control. The executor wins 6/6 (day 182 ×5, 189), `bridge-observe` matches it on every seed.
The Qwen games were at days 79–86 at 09:30 and need ~2 h per wave of 3. Read them with
`fenceleague report -spec league/t9-challenge-llm.json` (`var/league/t9-challenge-llm/REPORT.md`): a Qwen game
that ends `buyout` or `buyout_mortgage` is a win; compare its win day per seed with the executor's (paired), and
check its plan coverage (`day plan written` lines per day in its `bridge.log`; below ~0.85 the base played many
days alone).

## LLM delta planners on hard — 2026-11-11: they keep the appraisal incumbent's wins; one cash buyout, one loss to a final decline

**Setup.** `fenceleague` plan mode: the gym's executor plays, the bridge's planner (`-directives delta`,
exemplars on) writes the day's changes over a base policy it is now shown (`executor.directives`, bridge
`-base-directives`, `1263e79`); the delta contract gained the mortgage levers and gym engines (`de32a4a`) and
`appraisal` (`aef4876`). League `league/t7-hard-*.json`: apprentice, `vending+short_sleeper`,
`hard_income_mortgage_appraisal.json`, 1000-day cap, gym `aef4876`, fresh seeds 2100001–012. Planners:
Qwen-27B on the 5090 (thinking off, ≤3 games at once), drift (Qwen-Flash MLX, 1), GLM `glm53flash` on heavy (2).
A bridge with no LLM (`bridge-observe`) reproduces the executor on every seed of every league here.

| arm | games | wins | endings | median win day | Δ days vs executor (paired, 95 %) | plan coverage |
|---|---|---|---|---|---|---|
| executor | 12 | 11 | arrest 1 | 175 | | |
| Qwen-27B | 6 (2100001–006) | **6/6** | – | 175 | +1.2 ± 2.3 | 0.69–0.98 |
| drift | 6 (2100001–006) | **5/6** | rent 1 | 175 | +2.3 ± 46.1 | 0.96–1.0 |
| GLM | 2 (2100001–002) | 2/2 | – | 175 | 0 | 0.70–0.77 |

Per seed (executor / Qwen / drift): all win on day 175 except **2100004, drift wins for cash on day 91**
(executor 175), **2100005, drift loses to rent on day 272** and Qwen wins on day 182 (executor 175).

**What the planners changed.** Qwen changed something on 571 of 1 051 days and kept a change after the lints
on 421 (628 keys); drift 709 / 552 of 1 058 (909 keys). Kept: services, buy and sell, engine (nearly always restating the base's `wine+furnace`),
bargain (drift overrides the base's `discount` on 185 days), upgrades, mortgage (Qwen 10, drift 47).
Lint drops: Qwen 1 093, drift 2 749. Every `window.show_uids` is dropped (the executor's window stands:
384 Qwen, 325 drift), 120 Qwen engines naming only part of the base's `wine+furnace` were reverted (the
restated `wine` would have stopped the furnace), "no reason given" removes most of drift's keys, and
`sell.min_pct` is dropped while selling down. No lint blocked a mortgage or permit move. `appraisal: off`
appeared on one day each in two drift games, never in Qwen's or GLM's; the base's `join` held.

**The two outliers are both the mortgage.** On 2100004 drift answered the loan shark `decline` on days 56
and 84 ("1,639/week is too heavy", "keep liquidity") and the landlord's cash buyout came on day 91 (net worth
day 89: 29.5k vs the executor's 18.5k after its down payment) — this became the cash-buyout incumbent
(`hard_income_cash_appraisal.json`, `buyout_by_day`). On 2100005 drift declined on day 56, asked to `take` on
day 84 (a declined shark never returns) and lost to rent on day 272. Fix (`a79bcd7`): the delta prompt says
`decline` is final and offers `later`; a decline reverts to `later` unless cash + 70 % of the rest of net
worth + four weeks of the last week's gain covers the buyout price.

**Costs.** Qwen at 3 games on one 8-slot llama-server (no MTP) plays ~35 game days an hour per game and
misses its 100 s plan wait on 2–31 % of days (those days play the base). GLM on an idle heavy answers in
~14 s median; under load 56–100 s, past the gym's 120 s limit (stopped once for that). Drift at 1 game:
~100 game days an hour; it returned HTTP 507 (memory) on full prompts during the first normal games.

**Earlier rounds on superseded bases** (stopped when the base changed; partial):
| base | seeds | executor | LLM arms |
|---|---|---|---|
| normal, `-human player` defaults | 950001–024 | 24/24, median win day 185.5 | drift 2/2 (950002 day 245 vs 154, full coverage; 950001 only 33 % planned); Qwen's 8 games aborted at ~day 95 when the game gate fired |
| hard, `hard_search_best.json` | 970001–024 | 0/24, median 212.5 days | drift 970001 rent day 216 |
| hard, `hard_income_mortgage.json` | 1200001–024 | 3/24 (rent 12, repo 9), median 216 | drift 0/2 (rent 209 vs 216, repo 188 vs 174); Qwen stopped at ~day 90 |

## Hybrid planner on the hard stress test — 2026-11-11: the rollouts sign mortgages that repossess past their horizon

**Setup.** "LLM proposes, gym decides" (`docs/research/HYBRID.md`) against the executor alone, league
`league/h2-hybrid-hard.json`. Wins on the appraisal incumbent are at the ceiling and the route screen found no
start under 80 %, so the stress test is the old hard policy (`hard_income_mortgage.json`, apprentice,
`vending+short_sleeper`; 1/60 wins since the door gates). Fresh seeds 2300001–005 (all four arms finished
them), 4 samples, 4 rollout workers, 28-day horizon (42 in the mortgage window), switch gate 1.0 SE.
Proposers: Qwen-27B (local, thinking off) and GLM `glm53flash` (thinking off).

| arm | wins | median days | mean days | endings | median NW d100 | Δ days vs executor (paired, 95 %) | Δ NW d100 | LLM calls / game | wall / game |
|---|---|---|---|---|---|---|---|---|---|
| executor | 0/5 | 216 | 210.4 | rent 4, repo 1 | 7 425 | | | | 1 min |
| hybrid-blind | 0/5 | 181 | 183.8 | repo 4, rent 1 | 8 555 | −26.6 ± 14.6 | +762 ± 1 896 | 0 | 44 min |
| hybrid-qwen27 | 0/5 | 207 | 210.0 | rent 4, execution 1 | 5 098 | −0.4 ± 12.5 | −1 441 ± 2 370 | 50.8 | 100 min |
| hybrid-glm | 0/5 | 124 | 144.4 | repo 2, rent 2, arrest 1 | 4 983 | −66.0 ± 29.8 | −2 915 ± 2 212 | 36.8 | 68 min |

No arm wins. Qwen against the blind ablation, paired: **+26.2 ± 12.9 days** (all five seeds better); GLM
−39.4 ± 41.8.

**Why the hybrid loses days: the mortgage.** Blind signed the mortgage in 4 of 5 games (days 119–140) and was
repossessed in all four, 48–69 days later; GLM signed in 2 (day 70) and was repossessed 34 and 48 days later.
The executor's own rule answers the offer "soon" (it signed once). Each signing came from a candidate with
`mortgage: take` that scored best over the 42-day window horizon, which ends before the repossession: the
branches die after it, so the rollouts see equity, not the default. On GLM seed 2300001 the gate made it worse:
on the offer morning `base` outscored the held `mortgage: take` by 1 273 points (z 0.4), but the gate keeps
the plan in force unless the challenger clears 1 SE, so it kept the risky delta it had adopted on day 59.
Qwen never signed (it played `mortgage: take` in three games, but no signing followed), and its runs end
like the executor's, on the rent.

**What the proposals did.** A proposal outscored `keep` in the rollouts in 55 % (Qwen), 62 % (GLM) and
56 % (blind) of decisions, and cleared the switch gate and was played in 27 %, 30 % and 25 %. Most switches
come on a shortage morning (Qwen 64 of 90). GLM failed 26 of 184 calls (malformed JSON, e.g. an unclosed string), Qwen
19 of 254; a failed call rolls only keep and base. Calls take 77–82 s. A hybrid game costs 44–100 min against
1 min for the executor.

**Next, not done here.** Judge a mortgage decision to the end of the term (`-to-payoff`, already in
`Search.cs`) or extend the risk term to the payments left; and let the gate defend `base`, not the plan in
force, so a held delta has to keep beating the executor. Re-run on the same seeds before more LLM seeds.

**Shipped for the stream.** Every hybrid decision is a `candidates` show event in the match's decision log
(`log/hybrid.jsonl`), which `fenceshow -replay` plays like a league match; demo and format in
`docs/SHOW.md`, "Hybrid games" (`var/show/hybrid-demo/`: days 40–44 of the Qwen game on seed 2300001).

## Cash buyout — 2026-11-10: on the Board's money hard is won for cash on day 105, ten weeks before the mortgage

**Result.** With the appraisal income the cash buyout now beats the mortgage. The new hard incumbent
`sim/lab/scripts/directives/hard_income_cash_appraisal.json` (= `hard_income_mortgage_appraisal.json` +
`buyout_by_day: 140`) wins **60/60 fresh hard seeds 2700001–060 (Wilson 95 % CI 94–100 %), median win day 105
against 175**; paired against the old incumbent the run ends a median 70 days sooner (mean −55 ± 11). 53 runs buy
the shop for cash (days 70–140); 7 sign the mortgage late when their savings stall and win on days 210–245.
`-start apprentice -perks vending+short_sleeper`, 1000-day horizon, main fc2734b + the lever:

| Arm | Wins (95 % CI) | Median win day | Endings | Paired Δ win day, both won | Paired Δ end day (loss = 1000), mean / median |
|---|---|---|---|---|---|
| incumbent (mortgage at the first offer) | 60/60 (94–100 %) | 175 | buyout_mortgage 60 | — | — |
| cash: `mortgage: decline` | 55/60 (82–96 %) | 105 | buyout 55, rent 5 | −68.7 ± 4.3 (55) | +5.8 ± 63.2 / −70 |
| drift's changes: decline + insurance and janitorial + `bargain: never` | 57/60 (86–98 %) | 105 | buyout 57, rent 3 | −69.6 ± 4.8 (57) | −24.9 ± 50.0 / −70 |
| hybrid `buyout_by_day: 126` | 60/60 (94–100 %) | 105 | buyout 47, buyout_mortgage 13 | −46.5 ± 13.0 (60) | same / −70 |
| **hybrid `buyout_by_day: 140`** | **60/60 (94–100 %)** | **105** | buyout 53, buyout_mortgage 7 | −55.3 ± 10.7 (60) | same / −70 |
| `watchman` route (vs itself) | 60/60 (94–100 %) | 175 | buyout_mortgage 60 | — | — |
| watchman + decline | 54/60 (80–95 %) | 105 | buyout 54, rent 6 | −66.6 ± 4.8 (54) | +22.3 ± 68.2 / −70 |
| watchman + `buyout_by_day: 140` | 60/60 (94–100 %) | 112 | buyout 48, buyout_mortgage 12 | −48.1 ± 12.0 (60) | same / −63 |

(± is 1.96 SE of the paired difference.) Every cash run buys the business permit (60/60): `mortgage: decline` keeps
the executor's normal spending and only sells down toward the price within three days of a rent visit, unlike
`hold_cash_for_buyout`, which starved the permit in ba64e4c. Net worth on day 100: 28.2k against 20.9k (the
mortgage's down payment and first payments are not spent).

**Why the cash route works now.** Appraisal (~2k a week from day 8), plus Georgio's fourfold payback around day 69,
lifts cash from ~6k on day 56 to ~16–20k on day 70 and ~21k on day 84; the price is 24 000 + 50 a day after day
50 (26 000 on day 90). Declining the loan shark once is final (he does not come back: one `loan_shark2` visit
in each of the 60 decline runs, against 2–12 under the hybrid), so the 5 cash losses are runs whose cash peaks at 25–29k while the price climbs to its 34 000 cap and the
rent compounds 10 % a week; they die of rent on days 259–294. They are not separable early: on day 84 two of
them have the cash of a median day-105 winner.

**The hybrid** (`buyout_by_day: X`, gym, shared executor, off by default). On each offer it answers "later" (he
comes back next rent day) while cash + 70 % of the sellable stock + the last four weeks' net-worth gain carried
forward covers the price on day X; otherwise `mortgage` decides as usual (`auto`). The projection starts on
`buyout_check_from` (default 84). Before that the four-week gain predates the Georgio payback and the Master
appraisal rank and would sign almost every run at the first offer (replayed on the cash arm's trajectories,
X = 98–140: 49–55 of the 55 winners signed on days 56–63). X came from the same replay: from X = 126 up the rule signs all 5 losers
by day 140, and X = 140 converts the fewest cash winners into mortgages (1 in the replay). In the run itself
"later" is a different dialogue path from "decline", so trajectories diverge: all 5 cash-arm losers win (4 for
cash, 1 by mortgage) and 6 cash-arm winners sign instead; X = 126 signs 13. The late signers sign on days 98–133
(~1 750–1 900 a payment) and make all 16 payments; none is repossessed. With the levers off the build is
byte-identical to main on 10 hard (2700001–010, the old incumbent) and 10 normal (2700201–210, `-human player`)
seeds. The live autopilot compiles the lever (it reads `DifficultyRules`' property price fields, now tagged
`autopilot.executor` in `compat/hooks.json`) but has not run it.

**What drift did** (league t7-hard-drift, `-directives delta`, seeds 2100001–006). On 2100004 its one decisive
change was `{"mortgage": "decline"}` on day 56, a rent morning on which the loan shark made his first offer (cash
6 143 ≥ 2 × the 3 000 down payment). The other deltas were one-day changes (holds, window, services, a
`bargain: never` on 7 days, a few upgrade requests). The shark never came back, the executor's
`buyout: asap` took the landlord's offer on the day-91 rent visit (90 days played; cash 22 396 on day 77, price
26 050), 84 days before the executor's mortgage win on the same seed (day 174). On 2100005 it declined on
day 56 as well, then asked `"take"` on day 84 (and `auto` or `accept_if_rent` on 3 later days, `decline` on 12); with no shark left to
accept, cash plateaued at ~21k from day 111 while the price rose to 34 000 and the rent to ~2 100 a week by day
181 and ~3 400 by day 216, and the run ended on rent on day 272.
On 2100002 its `decline` came on day 54, not a rent day, so it had no effect. The executor's own decline arm on
those seeds buys out on day 105 (2100004) and day 70 (2100005).

**For the LLM layer.** Yes: drift found a route the executor's default missed. Declining at the first offer is
worth ~70 days in 55 of 60 runs; the executor signs at its first offer by rule, `mortgage: decline` was
last measured in ba64e4c (before appraisal: 0/60), and the idle runner's search, whose space has had
`mortgage: decline` all along, scored every win as 1000 days and so could not see a faster one. But drift did
not know two things the hybrid does: a decline is final (the delta prompt lists `decline` beside `take` as if it could be revisited, and on 2100005 the model asked to take the
offer four weeks later), and "later" (`soon choice`) keeps the option. With `buyout_by_day: 140` in the base
directives the delta planner needs to change nothing for the cash route.

**Idle runner.** `HARD` now names `hard_income_cash_appraisal.json`. Its score counts a win as survival 1000 plus
the days it left before day 1000 (day 105 → 1 895, day 175 → 1 825), so an earlier win breaks a tie in screening
and promotion (still only for candidates that win at least as often), and the scoreboard's regression check
compares that score instead of median days (a faster win used to read as a drop). `space-hard.json` gains
`buyout_by_day` [null, 126, 140, 154]. New route `sprinter` (`docs/research/ROUTES.md`).

## Persona × route runs, and Rock Bottom made robust — 2026-11-11

**Result.** A league can now play "model × persona × route" as one run spec (`routes` in the spec; the route gives
the start, perks and directives, the persona the strategist's style; docs/research/ROUTES.md "Who plays which
route"). Plans and narration take the route's character (smelter: "wake the beast"; vintner: Upper Levels and
the cellar). `rock_bottom` wins 120/120 on hard (2202001–060 and 2600001–060) against 116/120: `permit` is left
to the executor's rule (2 executions by an unbought permit, fixed) and a new gym lever, `short_rent_buy_pct: 85`
from 2 days out, keeps buying cheap lots while the cash is under a rent floor it cannot reach (4 early-rent
losses, fixed); ΔNW d150 +314 ± 2 320 and −2 113 ± 2 033 (not significant). `water_baron` wins 60/60 on
2600001–060 already. The LLM mini-league (Qwen-27B and drift
on four combos) was still running; its baselines win 12/12.

## Hard routes for the stream — 2026-11-10: twelve ways to win hard, all on the Appraisal Board's money

**Result.** With the appraisal income nearly every start and engine wins hard, so the stream can vary the run.
`docs/research/ROUTES.md` lists twelve routes, each a directives file in `sim/lab/scripts/directives/routes/`
(its `_route` key gives the start and perks). On 60 fresh seeds 2202001–060 (main 1722b2a + the module levers),
paired with the incumbent on the same seeds:

| Route | Start / change | Wins | Median win day | ΔNW d150 |
|---|---|---|---|---|
| `appraiser` | apprentice, the incumbent | 60/60 | 175 | — |
| `watchman` | + perfect alarm, Security transmitter | 60/60 | 175 | +7 211 ± 1 639 |
| `farmer` | farmer start | 59/60 | 175 | +2 490 ± 1 054 |
| `vintner` | wine only, 4 bays | 60/60 | 175 | +792 ± 1 151 |
| `labeler` | `labeler: "per_buyer"` | 60/60 | 175 | −1 240 ± 1 136 |
| `no_vending` | short sleeper + proficient scavenger | 60/60 | 175 | −1 148 ± 1 144 |
| `gunsmith` | gunsmith start | 60/60 | 175 | −1 640 ± 1 334 |
| `outlaw` | fence start (Security −30) | 60/60 | 175 | −1 898 ± 962 |
| `smelter` | furnace only, ore by phone, 2 furnaces + eco | 60/60 | 175 | −1 991 ± 1 126 |
| `lean` | no production engine | 60/60 | 175 | −6 128 ± 1 093 |
| `water_baron` | water merchant, `furnace+water` | 59/60 | 182 | −10 938 ± 1 224 |
| `rock_bottom` | rock bottom start | 56/60 | 182 | −5 733 ± 2 009 |

The Board's fees (~49k a run) are the same in every route; what differs is what the shop makes and sells
(wine 22k, ingots ~330 for the smelter, water 16k for the water baron, no vending income without the perk) and
which faction it courts. The drug-dealer start ends on day 8 (`cartel_retired`) in every run, and the
moonshiner, pharmacist, street-food and organ-trader starts play identically (the gym gives them the same bare
start), so none of them is a route.

## Machine modules and the wound rest — 2026-11-10: a perfect alarm pays for itself fifteen times over; nothing else moves the incumbent

**Result.** On the hard incumbent (`hard_income_mortgage_appraisal.json`, `-start apprentice -perks
vending+short_sleeper`), paired on 60 fresh seeds 2200001–060, 1000-day horizon, built on 5078aa9 + the levers
(`sim/gym/Modules.cs`, docs/re/MODULES.md §5). Every arm wins on the mortgage's own day (median 175), so net worth
and costs decide:

| Arm | Wins | NW d50 / d100 / d150 (median) | Paired ΔNW d100 | Paired ΔNW d150 | Services a run (median) | Module spend | Insurance skipped (cash) |
|---|---|---|---|---|---|---|---|
| incumbent | 60/60 | 6 745 / 23 172 / 29 824 | | | 17 433 | | |
| `alarm: "perfect"` | 59/60 (1 arrest) | 6 932 / 24 538 / 38 746 | +2 061 ± 1 465 | **+7 791 ± 2 324** | 2 754 (−14 180 ± 614) | 1 098 | 16 697 |
| `alarm` + `transmitters: ["sec"]` | 60/60 | 6 577 / 23 897 / 38 158 | +1 051 ± 1 477 | **+6 821 ± 1 826** | 3 945 (−12 325 ± 1 090) | 1 054 | 14 549 |
| `alarm` + `transmitters` + `jackson_module_intro` | 60/60 | 6 329 / 23 502 / 37 373 | +940 ± 1 461 | +6 759 ± 1 862 | 4 265 | 1 222 | 13 891 |
| `jackson_module_intro: "buy"` | 60/60 | 6 292 / 23 466 / 29 844 | −44 ± 1 104 | −376 ± 1 051 | 17 406 | 587 | |
| `projector_modules: "tier1"` | 59/60 (1 arrest) | 6 679 / 20 720 / 26 436 | −2 106 ± 1 445 | **−3 826 ± 1 374** | 16 182 | 1 486 | |
| `furnace_eco: true` | 60/60 | 6 886 / 23 015 / 29 797 | −326 ± 581 | −345 ± 783 | 17 015 | 698 | |
| `bays: 3` | 60/60 | 6 829 / 22 750 / 29 934 | −244 ± 724 | −10 ± 864 | 17 401 | | |
| `bays: 3` + `bay_modules: "nightly"` | 60/60 | 6 560 / 21 500 / 27 490 | −1 378 ± 1 136 | −2 754 ± 1 342 | 16 680 | 924 | |

(± is 1.96 SE of the paired difference.) The insurance is the incumbent's largest overhead: ~17k a run at ×2 on
hard, against an alarm (bought on Dr. Jackson's day-27 visit, median) and two to four performance modules for ~1.1k
plus ~5 power a night. The alarm is perfect from day ~44 and skips the insurance on ~115 nights a run. The gain
shows after the mortgage is signed (d150), not before the buyout day, which no lever moves. The one arrest
(seed 2200010) is an ENDANGERMENT charge built from day 86 on sold goods, on a trajectory the alarm had already
reshuffled; the incumbent's run has none.

With a transmitter on Security the alarm bay seats only the transmitter, an overclock and a performance module, so
6 runs in 60 (11 with the intro's modules competing for the squares) end below performance 50 and pay insurance
again; the saving is ~2k smaller. In exchange Security reaches 100 (VIP: no inspections, no burglary) in 48/60 runs,
median day 124 (54/60, day 111 with the intro), against 0/60 without; end Security rep 132 against 48.

**On the current main** (55902b7, after the outing fix and fair aug detection), fresh seeds 2203001–060, with the
final build (the alarm now refits best first when worn or burnt-out modules drop it below 50; paired against the
build before that change it is neutral, ΔNW d150 +18 ± 2 174): incumbent 60/60, NW 6 682 / 20 234 / 26 211;
`alarm: "perfect"` 60/60, ΔNW d100 +1 717 ± 1 300, d150 **+6 996 ± 1 811**, services 2 610 against 15 937;
alarm + Security transmitter 60/60, ΔNW d150 +4 055 ± 1 668, Security 100 in 54/60 (median day 117). The alarm is
the first lever since the appraisal that moves the hard incumbent's money; it does not move the win day, which
the mortgage schedule fixes. Not made the default here: it changes the stream's policy, and the live hands have
not fitted a module yet.

Not worth it: the module introduction (three modules for ~140 that the levers do not need), the projector (boost
~145, attractiveness ~395: just under the 400 tier, for ~1.5k and ~26 power a night), eco on the blast furnace
(16 → 10 a smelt, but the furnace runs ~85 smelts a run: ~300 of cells), and bay modules (a third bay held up to
14 days waiting for performance 100: three fits a run, −2.8k).

**Wound rest.** `scavenge_max_wound: k` skips the dump while `woundState > k`. Same seeds:

| Arm | Wins | NW d50 / d100 / d150 | Paired ΔNW d100 | Paired ΔNW d150 | Rest nights / door-shut nights (median) |
|---|---|---|---|---|---|
| incumbent (no rest) | 60/60 | 6 745 / 23 172 / 29 824 | | | 0 / 27 |
| k = 0 | 60/60 | 6 412 / 22 035 / 28 496 | −1 247 ± 1 060 | −1 600 ± 955 | 69 / 1 |
| k = 1 | 60/60 | 6 480 / 21 596 / 28 964 | −1 243 ± 1 157 | −1 515 ± 1 083 | 68 / 1 |
| k = 3 | 60/60 | 6 718 / 22 284 / 29 824 | −15 ± 1 153 | −462 ± 1 199 | 38.5 / 2 |

On the old incumbent without the appraisal income (`hard_income_mortgage.json`, build
5078aa9), where the dump still pays: no rest 3/60 wins (median 210 days), k = 0 1/60 (−3.3 ± 11.6 days), k = 3
0/60 (+0.2 ± 10.8 days). Resting trades a door-shut night for a rest night almost one for one and gives up the
dump nights in between. It does not help on weaker routes either
(docs/research/ROUTES.md: `no_vending`, 48 door-shut nights a run, and `rock_bottom`, both with k = 3, no gain). **Default:
unset** (no rest).

**Shared executor.** `Modules.cs` joins the shared files; `MachineAlarm` and `MachineProjector` members are in
`compat/hooks.json` and `RuntimeResolver`. With every lever off the rebased build's summaries match main (1722b2a) on
10 hard (2200901–910, the incumbent) and 10 normal (2200951–960, `-human player`) seeds, and again
on 55902b7 (M3).
The engines' item moves go through `Chore.Accept` and the transmitter's faction click through `Chore.Act`, like the
other shared engines; the levers have not been run live.

## Autopilot M3, the gun — 2026-11-10: fair aug detection costs nothing; the live autopilot shoots

**Fair detection (gym default).** The executor's aug rule (`sim/gym/AugRule.cs`, shared with the live
autopilot) now reads only visible tells: stillness, twitching, an aug portrait, the aug script's lines
(recognised by their displayed text against the game's own lines, read at run time), a generic name in a
plain bubble, or the scanner (`docs/re/AUGS.md` §6 lists every tell). `-aug-detection perfect` keeps the
old cheat for analysis and replays main byte for byte. Hard, best policy, 60 fresh seeds 2500001–060:

| Arm | Wins | Median day | Endings | Shot / escaped | Precision / recall |
|---|---|---|---|---|---|
| perfect detection | 60/60 | 175 | buyout 60 | 481 / 0 | – |
| **fair detector** | **60/60** | **175** | buyout 60 | 481 / 0 | **1.000 / 1.000** |
| fair, no line recall (`-aug-lines off`) | 60/60 | 175 | buyout 60 | 449 / 44 | 1.000 / 0.911 |
| serve all | 0/60 | 47 | aug 60 | 0 / 360 | – |

Fair and perfect play identically (net worth by day equal in all 60 runs). The best fair policy is the
incumbent, so the idle runner's HARD incumbent is unchanged.

**Live (built, not yet run in the game).** On a sure tell the autopilot raises the gun (`Shotgun.OnClick`),
fires once the crosshair is up (`StoreUIManager.Shoot`), reads Security's verdict through and hands over
on a manslaughter; `autopilot_gun: pause` hands every suspected aug to the human instead ("possible aug").
The shooting flow is class (b) (`docs/STREAM.md` sections 3 and 5); the owner's test is section 12.

## Autopilot M2 — 2026-11-10: the live executor plays the night; the gym answers the return prompt

**Live (built, not yet run in the game).** The autopilot plays the appraisal policy's day and night
(`docs/STREAM.md` section 5, "M2 as built"): the shared morning plan, the engines' chores and the
bedtime vending restock through the gated hands, supplier calls and Wild's Network through the
phone, the door, map, dump, loot and Dr. Jackson's shop, the return prompt, the appraisal board
(join, kit, clients through the inspection view and its panel), the permit donations and the
exposable pull. The owner's test is `docs/STREAM.md` section 9 (a hard week); the remaining
handovers are section 10, the hidden values still read section 11.

**Refactors** (`docs/STREAM.md` section 4, 1–3): the morning plan (`ExecutorMorning`), the
counter-adjacent decisions (`PlayerCounter`), appraisal's per-item decision, the phone orders
(`IExecutorHost.PhoneOrder`) and every engine move and machine action (`Chore`) are shared. Parity
against main `565f267`: 20 hard + 20 normal seeds with the appraisal policy and 10 hard with
`hard_income_mortgage.json`, all byte-identical.

**Gym fidelity fix: the return prompt.** After a dump outing whose loot did not fit, the gym never
answered `MapUIManager.Return`'s "leave items on the ground?" prompt, so the outing stayed open from
then on (from day 6–19, median 12 on hard) and counter purchases landed on the ground. The loot now
comes home most valuable first, cheap stock is left to make room when the loot is worth twice it,
and the prompt is answered. Appraisal policy, paired, 1000-day horizon:

| Arm | Wins before → after | Median win day | Paired Δ win day | NW d100 paired Δ |
|---|---|---|---|---|
| hard, 60 fresh seeds 2400001–060 | 59/60 → 60/60 | 174 → 174 | −0.1 ± 0.8 | −761 ± 1 291 |
| normal, 60 seeds 2400201–260 (same flags) | 60/60 → 60/60 | 62 → 62 | −0.2 ± 13.6 | −674 ± 3 348 |

The prompt is answered on 40 % (hard) / 46 % (normal) of nights. The new baseline: hard 60/60,
median win day 174, net worth on day 100 a median 20 480.

## Wine levers — 2026-11-10: the gym's wine is limited by berry health and water, and no lever pays before the buyout

**Result.** None of the wine levers improves the appraisal incumbent
(`hard_income_mortgage_appraisal.json`). On current main (with Dr. Jackson's shop purchases and his +50 % hard
markup), **60 fresh hard seeds 1800301–360**, every arm wins 60/60 (Wilson 95 % CI 94–100 %):

| Arm | median win day | Δ win day | nw at day 100 | Δ nw100 | wine/week | Δ wine/week | price/bottle | bottles/run |
|---|---|---|---|---|---|---|---|---|
| incumbent | 174 | — | 21 500 | — | 796 | — | 124 | 159 |
| `wine_dna` (Hardy bloomberries; bred plants in 45/60 runs) | 181 | +3.0 ± 1.0 | 19 990 | −1 510 ± 919 | 793 | −3 ± 54 | 129 | 155 |
| `wine_racks: 2` + `wine_min_age: 15` + `wine_hold_cash: 1000` | 174 | +2.6 ± 1.0 | 20 722 | −778 ± 1 016 | 866 | **+70 ± 51** | 141 | 155 |

On the tree before that change (5078aa9, 60 fresh hard seeds 1800101–160, all 60/60): `wine_dna` −2 109 ± 1 074
at day 100, +2.5 ± 1.0 win days, −19 ± 52 a week (133 a bottle); racks −730 ± 980, +1.4 ± 1.0, +23 ± 42 (142);
`wine_score: 5` −1 278 ± 1 359, −0.1 ± 0.8, −154 ± 81 (125). (± is 1.96 × SE of the paired difference; wine/week is
wine sales over days played.) Racks raise wine income a little, but net worth at day 100 is lower (held bottles and
the racks themselves) and the buyout comes no sooner. The incumbent stays as is; no directive file or search-space entry
is added. With the levers off the build is byte-identical to main on 60/60 fresh hard seeds (1800301–360) and 10/10
normal seeds (1800401–410), and was on 60/60 hard and 60/60 normal (1800201–260, 58/60 buyouts, median day 188)
before the rebase.

**Mechanics** (`docs/re/WINE.md`, from the game code). A bottle's score is the sum of its berries' points with no
cap on the number of berries, +1 for two or more, +1 for yeast, ±the water's grade; 6 = Master Vintage (175).
Berry points come from plant health at harvest, i.e. from the bay water's contaminants. The gym's bays run on
98–99 % water, so its berries are 21 % moldy, 40 % under/over-ripe, 30 % normal and 8 % best (8 879 berries,
16 runs): ~1.3 points a bottle, Production grade. Buyers pay the same price for a bottle whoever they are; the
Upper Levels Hedonist matters only through Tips (+15 % once UL rep ≥ 20) and a budget for two or three bottles.
The cellar's volume is bound by water for the bottles (750 ml rations), not by bottles or berries.

**Why the owner's upgrades do not carry over.** A free `hardy` trait on every seed (test-only build) shows the
ceiling: best berries 8 % → 18 % (rank I) / 27 % (rank II), bottles 125 → 142 / 154, +170 ± 64 / +248 ± 72 a week
and +2.2 k / +2.7 k net worth at day 100 (16 seeds). The real DNA route costs ~3–3.5 k (PLANT_DNA 1 875 on hard, the
manipulator 1 000 + 50 % hard markup from Dr. Jackson, offered on half his visits, disks 140 + markup), finds Hardy by a blind extraction one time in
3–7, and delivers the first bred plant around day 77 (39–128), leaving ~13 weeks before the day-174 buyout. Racks
add ~+16 a bottle by aging, but the rack runs sell 3–8 % fewer bottles. Berry targets have no spare points to
spend: bottles wait or start below target, and 17 % fewer are sold. (Both arms also harvest 20–28 % fewer berries
than the incumbent; not traced.) The owner's game ran on with 30 k net worth at day 100; these investments repay over a longer
game than the gym's buyout.

**Why aging lost before** (`wine_min_age: 10`, −14 days on the pre-appraisal policy): the held wine stayed in the
display case, so it kept pulling Hedonists who then found it refused (755 Hedonist visits, 346 buying nothing,
against 244 and 0 without the hold); it parked ~12 bottles while cash bound; and it released only when
liquidating, at the liquidation floor. Held wine now stays out of the case, and `wine_hold_cash` holds only
while cash covers rent, floor and a margin.

Selection: 16 hard seeds 1800001–016 (`docs/re/WINE.md` §7). Arms: `sim/lab/scripts/arms-wine.txt`, directives in
`sim/lab/scripts/directives/wine/`.

## Labeler levers — 2026-11-10: honest or per-buyer labels win the same and book a little less

**Result.** Neither lever moves the hard incumbent (`hard_income_mortgage_appraisal.json`, apprentice,
`-perks vending+short_sleeper`), paired on 60 fresh seeds 2200001–060, 1000-day horizon:

| Arm | Wins | Median win day | NW d50 / d100 / d150 (median) | Paired ΔNW d100 (95 % CI) |
|---|---|---|---|---|
| incumbent | 60/60 | 175 | 6 745 / 23 172 / 29 824 | |
| `fakes: "honest"` | 60/60 | 175 | 6 205 / 21 059 / 28 140 | −1 658 ± 1 107 |
| `labeler: "per_buyer"` | 60/60 | 175 | 6 542 / 21 956 / 28 740 | −1 333 ± 1 116 |

Every run in all three arms ends in the mortgage buyout on the same day (±0.7). A run labels ~30 owned fakes to
their truth (injectors and cigarette packs read off the magnifier's view), which cuts their booked value
by ~2.8k: most of the net-worth gap is that book value, not cash. `per_buyer` sells ~5 of them a run as
genuine to buyers who cannot expose the claim, which wins back part of the gap. Its other half, relabelling
open-box, rust, dent, expiry and replica claims upward, never fired (0 items in 60 runs): no owned item
carried an editable claim of those kinds below its best. The levers are kept as options, not defaults.

**Shared executor.** `Labels.cs` joins the shared files (`SharedExecutor.props`), recording its moves through
`IExecutorHost`; its game members are in `compat/hooks.json` and `RuntimeResolver`. With both levers off the
build's summaries match main (`5078aa9`) on 10 hard (2200901–910, the incumbent) and 10 normal
(2200951–960, `-human player`) seeds. Mechanics and who exposes what: `docs/re/LABELER.md`.

## Dr. Jackson's purchases — 2026-11-10: his shop now sells; on the appraisal incumbent nothing he sells moves the result

**Change.** `DrJacksonLocation.Arrive` ends in `MapUIManager.ActivateClientOutsideStore`: Dr. Jackson is the
counter's client for the visit. The gym laid out his stock without that step, so `OnBuyAllClick` returned and
every plan that visited his shop (`jackson`, `jackson_every`, the default water sign) paid the outing and bought
nothing (only `window_signs` and `storage_bays` had the fix). He is now the client on every visit. With him as
the client his goods also carry the difficulty's inventor markup (`ItemFeatureList.InventorMarkup`: normal
+15 %, hard +50 %; Sundays −10 %), and what is bought is carried home from the ground, as
`DrJacksonLocation.Leave` does. The summary's `player.jackson` lists purchases from him, at the shop and on his
visits to the store.

**Re-baseline** (60 fresh seeds each, 1000-day horizon, paired before → after; the hard policy is the appraisal incumbent `hard_income_mortgage_appraisal.json`, `-start apprentice -perks vending+short_sleeper`):
| arm | wins | median win day | median nw day 100 | Δ end net worth | endings after (before) | bought from Jackson a run after (shop / store visits) |
|---|---|---|---|---|---|---|
| hard, on b135f2f, seeds 1900001–060 | 60 → 59 (lost 1) | 174 → 174 | 23 966 → 23 576 | −964 ± 1 361 | arrest 1 (none) | 158 / 5 411 |
| hard, on 5078aa9 (door gates), seeds 1900301–360 | 60 → 60 | 174 → 174 | 22 165 → 21 729 | −2 ± 1 317 | none | 165 / 5 553 |
| normal defaults (`-human player`), on b135f2f, seeds 1900001–060 | 59 → 58 (lost 1) | 181 → 188 (both won: +6.6 ± 10.8) | 8 347 → 8 072 | −2 ± 799 | repo 1, rent 1 (rent 1) | 140 / 1 335 |

The fix is neutral on both: the default plan buys one water sign at the shop (150 on hard) and leaves it in stock.
What the engines need from Dr. Jackson they already bought on his visits to the store, which always worked.

**Jackson as the equipment shop** (hard appraisal incumbent `hard_income_mortgage_appraisal.json`, paired on
60 fresh seeds 1900101–160; all 60/60 wins except where noted, median win day 174 in every arm):
| arm | wins | Δ net worth day 100 | Δ end net worth | shop visits a run | bought at the shop / on his store visits |
|---|---|---|---|---|---|
| base (`jackson_every: 14`, buys the water sign) | 60 | — | — | 11.4 | 162 / 5 559 |
| `jackson_every: 7` | 60 | −1 260 ± 926 | −885 ± 1 055 | 22.4 | 162 / 5 495 |
| `jackson_sunday: true` (−10 %) | 60 | −544 ± 1 136 | −272 ± 1 181 | 10.9 | 140 / 5 378 |
| `jackson: []` (no water sign) | 60 | −426 ± 840 | +134 ± 1 231 | 11.4 | 0 / 5 332 |
| `window_signs: ["sign_water"]` (sign shown in the case) | 60 | −1 697 ± 1 069 | **−5 927 ± 1 025** | 11.4 | 160 / 4 998 |
| `jackson_equipment: true` | 59 (1 arrest) | −540 ± 885 | +236 ± 1 427 | 11.3 | 2 431 / 4 783 |
| `jackson_equipment` + `jackson_need: 3` | 60 | −228 ± 1 226 | −103 ± 1 565 | 17.3 | 3 418 / 4 444 |
(± is 1.96 SE of the paired difference.) No lever helps; only showing the water sign hurts, because a sign in the
case takes one of the two or three normal-customer slots (as `window_signs` did in "Stock breadth"). The win day is
the same in every arm: with the appraisal income every run signs the mortgage on schedule and pays it off on the
same day, so the remaining effects show only in net worth.

What he sells and what the gym buys from him (`StoreClientList.PlaceInventorInventory`): machines (furnace,
hydroponic bay, purifier, moisture farm, RAS tank, fridge, desequencer, DNA manipulator once PLANT_DNA is
unlocked, 50 %), bays and kits, power (cells charged and empty, an extended cell, the powerblock, the current
converter, the recharger), machine modules (furnace junk/blast 20 % each, performance/efficiency/quality 33 %,
overclock/eco/fineness 16 %, DNA modules incl. precision 25 % each), tools and guides, and the supplier signs.
Owned machines come back only one visit in four (`RNG.Roll(25)`); the rest rolls fresh each visit; Mondays bring
the special stock at +150 % once JACKSON_SPECIAL_STOCK is unlocked. He also comes to the store with the same
stock (minus backpacks). On the base policy the gym spends ~5.7k a run with him, almost all on his store visits:
energy cells 4.9k (~30 cells at 165, 110 + 50 % markup, from day ~19), furnace junk module 180 (day ~40),
blast module 220 (day ~68), bioreactor 222 (in 36 of 60 runs, day ~61), the water sign 162; the furnace itself
is bought on his first visit, day 6, at the new-customer discount (not in these totals). `jackson_equipment` buys the same set earlier at the shop (blast
module day ~48 instead of 68, bioreactor 48 instead of 61, hydroponic bays in 9 runs) and 1.3k more of cells,
for no gain. So the executor already buys its equipment at about the right time; buying it earlier does not pay
back on this policy. The owner's 9.7k there went on resale medical goods, the DNA manipulator and the purifier;
with a +50 % markup on hard, resale of his stock at the estimate loses a third on every item, so the resale
lever was not built. The cells are the real cost: power for the furnace, bays and vending machine costs ~200
a week bought as cells at 1.65 a unit; a current converter draws store credit at 1.5 a unit (−0.1 per upgrade,
less with the power-hijack tiers) plus the power service at night, which may be cheaper and is not modelled.
The DNA lever (branch `wine`) can buy his manipulator through `jackson` or `jackson_equipment` now that the
purchase path buys.

**mortgage_floor_stock on the appraisal incumbent.** `mortgage_floor_stock: 1.0`
(`hard_income_mortgage_appraisal_mfs.json`) on 60 fresh seeds 1900201–260: 60/60 against 60/60, and all 60 runs
byte-identical in net worth by day: with the appraisal income the cash never comes near the one-payment floor,
so the lever never binds. The idle runner's incumbent is unchanged.

**Gym fidelity, not fixed here.** `MapUIManager.Return` opens a "leave items on the ground?" confirmation when
the after-hours ground holds anything, and the gym never answers it, so the outing stays open overnight and
into the next day (`MapUIManager.isOutside`, under which goods bought at the counter go to the ground). This
happens whenever dump loot does not fit the stock grid: 130 of 150 nights on one hard seed (1899998, old
incumbent, before the door gates). The Jackson path now empties the ground before leaving when the goods fit; the dump path still leaves loot there.

## Outings pass the store door — 2026-11-10: a gym fidelity fix; the appraisal incumbent barely notices

**Change.** The gym's night outing called `MapUIManager.Travel` directly, skipping the gates of
`StoreDoor.OnClick`, the only way a player leaves the shop: not while seriously wounded
(`woundState > 5`), not during a `crackdown`, not while the agoraphobia perk rests, not before the
mentor introduces after-hours, and only once a night. `Episode.Outing` now applies them
(`Episode.DoorShut`; a refused night is the default `night:door_shut`). Baselines move; the old ones
let the executor scavenge on nights no player can.

**Effect** (best policies, 60 fresh seeds each, 1000-day horizon, paired before/after):

| Arm | Wins before → after | Median days | Endings after (before) | Nights refused |
|---|---|---|---|---|
| hard, `-start apprentice -perks vending+short_sleeper -directives hard_income_mortgage.json`, seeds 1700001–060 | **7/60 → 1/60** (lost 7, gained 1) | 205.5 → 209 | repo 28 (31), rent 30 (21), arrest 1 (1) | 2 097, ~35 a run |
| hard, appraisal incumbent (`hard_income_mortgage_appraisal.json`, same start and perks), seeds 1700001–060 | **60/60 → 59/60** (lost 1) | 174 → 174 | arrest 1 (none) | 1 678, ~28 a run |
| normal, `-human player`, seeds 1800001–060 | 58/60 → 57/60 (lost 3, gained 2) | 188 → 188 | repo 2, rent 1 (rent 1, execution 1) | 1 766, ~29 a run |

Days played, after − before: hard +5.6 ± 4.1, appraisal −1.8 ± 1.5 (54 of 60 runs end on the same day),
normal +2.0 ± 5.1 (SE). The appraisal board landed on main while this was measured; with its income the
lost dump nights hardly matter, so the stream's hard policy is unaffected. Nearly every refused night is a
serious wound from the dump (sampled seeds: 22–40 a run, plus night 1 before after-hours is
introduced). The executor keeps scavenging until a wound stops it and then loses those nights: for
the old hard incumbent the scavenging income it was counting on is gone, and the shortfall shows up as missed rent
(21 → 30). Follow-ups for the executor, not done here: stop scavenging before a serious wound
(`MaxWoundPct` is unset by default), treat wounds, or plan the rent without the dump while wounded.
The previous hard figures (8/60, 19/180, the stream's subset arms in `docs/STREAM.md` section 7) were
measured with the gap and need re-measuring.

Also fixed: `jackson_every: 0` (documented "never") divided by zero in `PlayerRole.Night`.

## Appraisal board — 2026-11-10: hard goes from 6/60 to 59/60 wins

**Result.** Joining the appraisal board and serving its clients (`appraisal: join`, new incumbent
`sim/lab/scripts/directives/hard_income_mortgage_appraisal.json` = `hard_income_mortgage.json` + the lever)
wins **59/60 fresh hard seeds 1600001–060 (Wilson 95 % CI 91–100 %) against 6/60 (5–20 %)** for the
incumbent, paired +53 wins (53 arm-only, 0 base-only). Every win is the mortgage, on day 174 (four on 181);
the one loss is an arrest on day 103. The incumbent's endings: rent 26, repossession 28, buyout 6 (days
181–223). Net worth on day 100: median 23.8k against 8.3k (paired +15.1k ± 1.0k); day 150: 30.3k against
8.0k. Cash on day 100: 19.8k against 4.8k, so the mortgage is signed at its first offer (first payment on
day ~70 instead of ~119).
- **Appraisal income**: from joining (day 8), a median **2 030 a week**; days 99–112 **2 245 a week** (the
  owner's late game: ~2 230). All 60 runs reach Master, on day 41 (median), after promotions costing 212,
  255 and 298. 6 334 visits (~3 items each), 19 023 estimates, all graded perfect; 0 refusals, 0 walk-outs,
  0 misreads.
- **Normal** (60 fresh seeds 1600201–260, `-human player` defaults vs + `{"appraisal":"join"}`): 60/60 wins
  against 59/60, median win day **146 against 188**; 9 runs buy the shop outright for cash on day 62.
  Appraisal 1 950 a week from joining.
- **Off is byte-identical**: with the lever off, the build's summaries match main (`4121504`) on 10 hard
  (1600001–010, the incumbent) and 10 normal (1600201–210) seeds. The rebased build replays the 60 hard
  appraisal runs byte for byte.

**Mechanics** (`docs/re/APPRAISAL.md`, from the game code). The board member comes on day 8: join (85 ×
difficulty multiplier, 170 on hard), later (back in 7 days) or decline. Joining gives a magnifier, the hand
guide and a junior certificate. Appraisal clients come only while a certificate is in the display case
(`ShowcaseHelper.GetDisplayCasePrediction`), at 33/42/48/55 % a day by rank, ×(2 + days missed)/2. The
chance depends on rank only, not reputation, and each client takes one of the day's normal-customer
slots. A client brings 2–4 items of one type: hands from Junior, ribwiches from Appraiser, plasma cells
from Senior. The fee per item is the base fee (20/40/60/100) + a percent of the **estimate** (Master: 25 %
perfect, 21 % excellent, other grades refused). Perfect = 95–105 % of the true value
(`AppraisalServiceHelper.GetEstimatePrecision`). The true value is a fixed function of the cues the
inspection view draws (`INSPECT.md`), so with the guide it can be computed exactly. Promotion takes
100/150/250 progress (+10 per perfect estimate, so 50 items in all); the offer comes the next morning.
Bad estimates: refusal at < 50 % / > 150 % or a grade the rank does not pay, +49 impatience, progress loss
for negative grades (× 1.5 on hard), and a walk-out on the third refusal. There is no rep or cash penalty.

**Fidelity.** The headless game already ran the whole flow, and no rule changed. The executor gained
the player's half. The counter is wiped every night (`EmporiumEntry.ClearFrontInv`), so a run that joined
used to lose the magnifier and guide on day 8. The kit now goes into the dossier, which takes documents
and the magnifier, and is never sold. `InspectionUIManager` is stubbed, so the executor reads the cues the item's
view draws (`Appraisal.Perceive`), applies the guide's table (`Appraisal.Value`, our own code) and submits
through `AppraisalServiceHelper.OnEstimate`, the panel's submit. No expose method is involved: appraisal
items have no hidden features. The true value is logged as hidden only. It aims at 105 % (the fee is a
percent of the estimate; `appraisal_aim_pct`). Caveats: 153 of the 19 023 items (0.8 %) were ribwiches
whose logo was a sprite variant against the guide's reference, read as an expert would; and the gym's
calibrated time per customer does not charge the extra seconds a player spends inspecting.

**Next.** The space gains `appraisal` and `appraisal_aim_pct` (`space-hard.json`); the idle runner's `HARD`
tuple still names the old incumbent. With ~20k cash by day 100, the hard cash buyout and the
owner's other levers (wine quality, held stock) are worth re-testing on top of appraisal.

## Stock breadth — 2026-11-10: supply, not breadth, is what the hard counter lacks

**Result.** No breadth lever raises hard income enough to win more. On hard every single item is bought and sold at its
estimate, so stocking the types the buyers ask for earns nothing unless it comes in a discounted
wholesale lot, and the gym buys a third of the owner's discounted supply. The one lever that wins more is
under the mortgage: `mortgage_floor_stock: 1.0` (repossessable stock counts toward the payment held in
cash) won **20/120** (95 % CI 11–24 %) against **13/120** (6–18 %) paired on 60 + 60 seeds (1400001–060 selection, 1400101–160
fresh: 9/60, Wilson 95 % CI 8–26 %, against 5/60, 4–18 %), with fewer repossessions; days and income
before signing are unchanged. It is a candidate, not yet a new incumbent (11 seeds gained, 4 lost,
p ≈ 0.12): `hard_income_mortgage_mfs.json`. With all new levers off the gym is byte-identical to main
(10 hard + 10 normal seeds); normal defaults on 60 fresh seeds (1400401–460): 56/60 buyouts, median
day 188. The demand trace, the client rules (`docs/re/CLIENTS.md`) and a Jackson purchase bug come
with it.

**Where the empty buyers go** (hard best policy, 60 seeds 1400001–060, `FENCE_DEMAND_TRACE`; days 31–200).
The executor sees 5.6 buyers a day offering 2.1k of budget (1.4k on days 31–100, 2.6k on days 100–200)
and sells 22 % of it (29 % / 19 %), 10.6 items a day. 43 % of buyers leave with nothing staged
(0.9k of budget a day). Why nothing was staged (share of those buyers, budget a day):
| status | share | budget/day | what |
|---|---|---|---|
| holds none of the type | 39 % | 419 | SUBSTANCE (upper-level visitor, miner, junker), WEAPON/ARMOR/AMMUNITION (mercenary), LUXURY_ITEM, CONTRABAND/ACCESS_CARD, CHEMICAL_SUPPLIES |
| all held for an engine | 32 % | 268 | FOOD: wine berries; HOUSEHOLD: empty wine bottles and yeast; MATERIAL (also the inventor's): flux, junk, ore for the furnace |
| all exposable by the buyer | 10 % (+3 % also stolen) | 104 | MEDICAL: unexposed fake Immunivax injectors (expired, unusable, counterfeit) bought blind in wholesale lots |
| only inside machines | 5 % | 47 | cells in the hydroponic bays, furnace and vending machine |
| stolen (held until cool) / over the budget / water labels | 6 % | ~40 | |
By want, the biggest losses a day are SUBSTANCE (121), weapons (119), MEDICAL (110), FOOD (96) and
LUXURY_ITEM (80). Sellers offer about the same types the shop already buys (household 6, material 4.6,
medical 3.5, food 3.5 units a day); SUBSTANCE (1.2 a day), weapons (0.3) and luxury goods (0.3) are rare.
How wants are set and how a buyer takes staged goods is now written up from the code in
`docs/re/CLIENTS.md`: wants are fixed per client type (only the budget is rolled), the tablet shows only
the tag list, and the buyer pays each staged item up to the budget left.

**Why buying breadth cannot pay on hard.** Every seller asks exactly the estimate for single items
(5 698 of 5 700 refusals in a sample were at 100 %), the bargain panel offers the executor almost no
demands on sellers, and hard sells at the estimate. So a single item bought to fill a gap earns nothing;
the only margin is a wholesale lot's 15–25 % (or an engine). The gym buys about 200 of trade stock a day
against the owner's ~670 (7.4k over days 103–113 at 83 % of estimate): what the counter lacks is
discounted supply, and demand is not the limit.

**Sources** (from the code): wholesale sellers in the queue (scav, crate, thief and generic suppliers);
supplier signs in the case (`sign_food`, `sign_medical`, `sign_household`, `sign_material`: each brings
that trade's generic seller, 15–25 % off, three openings in four, but takes a normal-customer slot;
Dr. Jackson sells all of them); phone suppliers (Red Imp beer; Pharma: 5 pink injectors a week, NARCOTIC +
SUBSTANCE, 50 each, 20 % off; Chem: dream dust; Mining: ore); production (berries in the hydroponic bays,
already the wine engine's input). No milk or herb source was looked for beyond these.

**Gym fidelity bug found.** Dr. Jackson's purchases buy nothing: the gym lays out his stock but never
makes him the counter's client (`MapUIManager.ActivateClientOutsideStore`), and `OnBuyAllClick` returns
without one. Every plan that visits him (`jackson`, `jackson_every`, the default sign_water) has been
paying the outing and buying nothing. Fixed only under `window_signs`, to keep baselines byte-identical;
making it the default needs its own re-baseline.

**Levers** (hard, apprentice, `-perks vending+short_sleeper`, on `hard_income_mortgage.json`; paired on
60 seeds 1400001–060; sales a week is the executor's counter sales; sell % is of the buyers' budget):
| arm | wins (95 % CI) | Δ days | sales/wk d31–100 (Δ) | sell % d31–100 / d100–200 | items/day d31–100 | signed / repo / rent | sales/wk under mortgage |
|---|---|---|---|---|---|---|---|
| base (`hard_income_mortgage`) | 8 (7–24 %) | — | 2 931 | 28.7 / 19.2 | 9.5 | 44 / 34 / 16 | 2 210 |
| `breadth: {}` (top 6, k 2, ≤ 95 %) | 4 | +2.5±9.8 | −24±128 | 27.9 / 18.6 | 9.4 | 32 / 28 / 27 | 2 115 |
| `breadth: {queue: true}` | 3 | +1.9±9.8 | +10±122 | 28.7 / 18.8 | 9.5 | 37 / 33 / 22 | 2 085 |
| `breadth: {queue: true, max_pct: 100}` | 7 | −6.3±10.2 | +151±108 | 29.7 / 19.3 | 9.6 | 39 / 30 / 21 | 2 350 |
| `window_signs`: all four supplier signs | 0 | −38.0±9.0 | −534±143 | 33.7 / 19.2 | 8.4 | 1 / 1 / 57 | — |
| `window_signs: ["sign_medical"]` | 7 | +7.2±10.7 | +154±113 | 30.2 / 20.2 | 9.7 | 31 / 23 / 25 | 2 674 |
| `inspect: "injectors"` | 1 | −3.4±10.4 | −141±137 | 29.9 / 18.4 | 9.3 | 30 / 28 / 29 | 1 993 |
| `stage_variety: true` | 5 | −4.0±12.7 | +0±109 | 28.4 / 19.1 | 9.6 | 41 / 36 / 18 | 2 149 |
| medical sign + variety + breadth (queue, 100 %) = combo | 6 | +6.7±9.0 | **+236±130** | 30.9 / 20.5 | 9.8 | 28 / 22 / 30 | 2 648 |
| combo + `bargain: ev_positive` | 6 | +2.0±9.1 | +75±155 | 30.3 / 20.5 | 9.2 | 26 / 20 / 32 | 2 753 |
| combo + `mortgage_floor_stock: 0.5` | 6 | +11.0±10.1 | +236±130 | 30.9 / 20.5 | 9.8 | 28 / 21 / 30 | 2 887 |
| `mortgage_floor_stock: 0.5` | 10 | +1.9±3.8 | −2±4 | 28.7 / 19.3 | 9.5 | 44 / 34 / 16 | 2 314 |
| `mortgage_floor_stock: 1.0` | 11 (11–30 %) | +2.4±4.0 | −2±4 | 28.7 / 19.3 | 9.5 | 44 / 33 / 16 | 2 334 |
| `hold_for`: 3 best MEDICAL, SUBSTANCE, LUXURY_ITEM kept for Upper Levels | 5 | +6.1±10.7 | −161±158 | 24.7 / 16.0 | 8.7 | 33 / 28 / 25 | 2 085 |
| `storage_bays` (+ Jackson's bay) | 2 | +0.2±9.2 | +10±123 | 28.5 / 18.3 | 9.6 | 24 / 22 / 33 | 2 046 |
| `storage_bays` + `hold_for` | 1 | +2.1±8.7 | −151±127 | 24.8 / 15.6 | 8.7 | 22 / 21 / 35 | 2 018 |
Breadth bought at the estimate (`max_pct: 100`) sells more (+151 a week) but earns nothing on it; at
95 % it hardly buys at all (no seller takes less without a discount demand). Every supplier sign takes
one of the two or three normal-customer slots a hard shop gets (attractiveness 250–550), so four signs
turn all random customers into suppliers: buyer budget falls 40 %. One medical sign is the useful dose:
more sales (+154 a week; +464 a week under the mortgage) but it and the stock it brings take the cash
the mortgage signing needs, so fewer runs sign (31 against 44) and more end on rent. Confronting
injectors costs wins (1): the case's medical value falls once fakes are exposed, and so does the tier
of its display buyer. Variety staging changes nothing measurable.
Holding for the tipping buyer (d515338) works as intended and still loses: the Upper Levels share of
counter sales rises from 33 % to 47 %, but the held goods are sales the other buyers would have made
(sell-through 24.7 % against 28.7 %), and an Upper Levels buyer comes about 0.7 times a day. Storage bays
stage faithfully (the goods move from the bay's own grid to the counter), but on hard the grid was not
the limit (162–219 of 240 cells), and the bays arms also carry the Jackson fix: his bay and the water
sign are now really bought, and that cash is what the mortgage signing needed (24 signings against 44).

**The cash floor under the mortgage** (from ba64e4c: losing runs run out of cash for buying). 
`mortgage_floor_stock: f` lets f × the repossessable stock count toward the payment held in cash.
Losing runs stop buying as their cash nears one payment. Counting the stock lifts purchases under the
mortgage 10 % and sales 5 %, and wins: paired on 60 + 60 seeds (1400001–060, 1400101–160) **20/120
against 13/120** (`mortgage_floor_stock: 1.0`; 11 seeds won only with it, 4 only without, sign test
p ≈ 0.12), with no extra repossessions (63 against 69). `0.5` gives 18/120. Days and income before
the mortgage are unchanged by construction.

## Hard mortgage timing — 2026-11-10: a sign-by deadline cuts repossessions, not losses

**Result.** No timing rule beats signing whenever `auto` allows. A sign-by deadline
(`mortgage_window: [1, 105]` on `hard_income_mortgage.json`) cuts repossessions from 64 to 29 on **120
fresh seeds 980001–980120**, but those runs die of rent instead, and it gives up the 3 late-signing wins:
**8/120 wins (Wilson 95 % CI 3–13 %) against 11/120 (5–16 %)**, paired −3 ± 1.7 wins (SE), +6.6 ± 1.3 days
for runs neither arm won. The incumbent stays `hard_income_mortgage.json`. Its fresh-seed record is now
**19/180 (10.6 %, CI 7–16 %)**: 8/60 on 970001–060, 4/60 on 980001–060, 7/60 on 980061–120. Wins
come on days 181–244. The idle runner now uses this policy (below), and its first scoreboard line on
990001–990060 reads 11/60.

**Mechanics** (from the game code: `PlayerStore`, `StoreClientList.CreateLoanSharkCollect`,
`StoreClientManager.CheckRentDay`; hard rules: price 24 000 + 50/day after day 50, cap +10 000, down
12.5 % rounded to 500, 75 % a year over 16 weeks):
- The payment is fixed at signing: ⌈(loan + loan × 75/100/52 × 16) / 16⌉, with loan = price − down.
  Each week of delay adds ~24 to every payment (~380 over the loan); waiting also pays rent rising 10 % a
  week.
- Collection comes on the rent-day morning. Georgio's payback is queued first when he is owed and cash is
  under the payment, then a merchant buyer. "Pay" needs cash ≥ payment. Otherwise, if repo-eligible
  goods are worth at least a payment at their current value (no retail markup), the shark takes a
  greedy best fit: items in descending value while the sum stays ≤ the payment, plus the item that lands
  closest if that sum is under 80 % of the payment. They are destroyed, a Repo Slip is left, the countdown
  resets to 7 days and **the week is not credited**. With less than a payment of eligible goods the run
  ends (`repo`).
- Not seizable: documents, containers, machines, locked or behind-lock items, anything in a smuggler's
  bay, important items, zero-value items.
- So a repossession is never a soft payment. It burns a payment's worth of stock and still leaves all
  16 payments to make.

**What kills the repossessed runs** (240 runs of the incumbent: 960001–060 for selection, 970001–060,
980001–120). On the first 120 of them, pre-signing numbers don't separate early winners from early
losers: income, sales, net-worth gain and the payment/earnings ratio are all equal within noise. Only
cash after the down payment differs, by about 330. The sign day does separate them:
- signed ≤ day 105: 26 wins, 55 repossessed of 82;
- signed after day 105: 3 wins, 77 repossessed of 80.
Late signers are the runs that took longest to meet `auto`'s float, so they start out paying more.
After signing, winners net ~1 500–1 800 a week, about the ~1 750 payment, and keep ~2 500 cash at
week 15. Losers net ~1 300, so cash falls under one payment by week 8. Then the cash floor (one payment)
stops trade buying, sales fall from ~4 300 to ~2 000 a week, vending empties, and repossession follows
(median 9 payments made, quartiles 7–12).

**Levers** (hard, 60 selection seeds 960001–060, vs the incumbent's 10 wins / 34 repo; the executor
gains `mortgage_cover` and `mortgage_floor_days`, both off by default; normal with
defaults is byte-identical on 60/60 fresh seeds 980001–060, 58/60 buyouts, and hard with the levers off
on 60/60):

| Arm | Wins | Repo | Verdict |
|---|---|---|---|
| `mortgage_window` [1, 98] / [1, 105] / [1, 112] | 8 / 10 / 10 | 6 / 11 / 18 | [1,105] kept wins on selection; fresh: −3 wins |
| [1, 105] + `mortgage_float` 500 / 0 (sign at a thinner cushion) | 8 / 7 | 17 / 20 | hurts |
| [1, 98] + `mortgage: take` | 7 | 14 | hurts |
| `mortgage_cover` 4.0 / 4.4 (cash after down + repo stock ≥ k payments) | 9 / 8 | 22 / 18 | delays signing: fewer wins |
| [1, 105] + cover 4.0 / 3.6 | 6 / 9 | 7 / 10 | hurts / neutral |
| `mortgage_floor_days` 2 / 4 (hold the payment in cash only near it) | 9 / 10 | 35 / 33 | neutral |
| [1, 105] + floor_days 2 | 7 | 14 | hurts |

**Cash buyout vs mortgage** (same base policy, 60 fresh seeds 980001–060, paired):

| Route | Wins (95 % CI) | Endings | Median days |
|---|---|---|---|
| mortgage (`auto`, margin 0.8) | 4 (3–16 %), win days 188–244 | repo 35, rent 20, arrest 1 | 209 |
| cash, buyout `asap` (`mortgage: decline`) | 0 (0–6 %) | rent 59, arrest 1 | 223 |
| cash, `buyout: hold_cash_for_buyout` | 0 (0–6 %) | arrest 57, execution 2, rent 1 | 110 |

The cash route never comes close. Net worth peaks at a median 46 % of the price on the day (best run 60 %;
median peak cash 9.7k, peak net worth 13.2k against a price of 26–34k). `hold_cash_for_buyout` does
save, to a median peak cash of 8.2k, but its floor (all cash up to the price) also blocks the permit fee,
so 57/60 runs are arrested for operating without a license around day 110. The owner's game had 30.5k net
worth at day 100; the gym's best has ~8–9k. Until gym income is ~3× higher, the cash sprint has no
traction, and the hybrid (mortgage if offered by day X, else save cash) reduces to the deadline arm
above, because the cash fallback never wins. Arms: `sim/lab/scripts/arms-mtime-*.txt`.

**Next.** Win rate is bound by mortgage-period income (~1 300–1 550 net against ~1 750 payments), not by
timing. The levers worth searching are the ones that keep trading alive under the payment floor, and
the player's trade volume (the fidelity check below finds that gap in the player, not the gym).

## Idle runner — hard incumbent switched (2026-11-10)

`sim/search/idle_runner.py` now takes the hard incumbent as a `HARD` tuple (start, perks,
directives): `-start apprentice -perks vending+short_sleeper` with `hard_income_mortgage.json`.
Scoreboard lines record `hard_policy` (hash) and `hard_policy_spec`, and are checked for hard regressions
only against lines with the same policy. The search incumbent carries gym flags (perks) and is reseeded
when `HARD` changes. `space-hard.json` adds perks, vending, beer, `jackson_sunday`, `bargain_buy:
ev_positive`, sign-by mortgage windows, `mortgage_cover` and `mortgage_floor_days`.

## Fidelity check — 2026-11-10: trade volume and water are player gaps, not gym gaps

Both suspected gaps were checked against the game code and the owner's saves and bridge
recordings. The gym is faithful on both, so no rule changed. The summary now carries
`end.client_visits` (the game's `permanentClientTracker`, as a save holds it), and with the
instrumented build the baselines are byte-identical to `f6b54b7` (10/10 seeds).

**Trade volume** (owner hard days 110–112 vs the gym's best hard policy, 60 fresh seeds
1100301–1100360, days 100–113):
| | owner | gym |
|---|---|---|
| customers a day (all kinds) | ~11 | 10.1 |
| buyer budget offered a day | ~2.3k | ~2.35k (one traced run) |
| share of budget sold | ~59 % | ~24 % |
| buyers who bought anything, a day | ~3.3 | 3.3 |
| items a day / counter sales a week | ~27 / ~9.5–12.3k | 12.0 / 3.7k |
Arrivals come from the game's own `GenerateClient` (once at opening; every queued client is
served), and the budgets match. The gap is what the shelves hold: half the gym's buyers find
nothing they buy ("nothing sellable staged"), while the owner fills baskets of medical goods,
beer, berries and milk. Attractiveness (owner 700, gym ~550) adds +25 % budget and the richer
client tier, which is a player choice (renovations; H19 was neutral). Bigger baskets alone do
nothing: an executor that stages up to 17 items into 1.5× the budget (not committed) sold
10.4 items a day against 10.2 (30 hard seeds 1100401–430, +5.8±6.5 days, 0 vs 2 wins).
Storage/machine bays: items inside a bay still can't be staged at the gym's counter, but the
executor never buys a storage bay, so the limit changes nothing today. It only matters once a
policy buys the owner's bays for wider stock.

**Water and shortages.** The GUID-keyed shortage sums are the real game's behaviour, not a
headless artefact. Events are built in code, the GUID is set in `NegociationData`'s
constructor, and both saves store GUIDs. Neither save's tracker has a shortage or surplus
client. The owner's hard days 110–111 ran `medicalSuppliesShortage` (+50) with no Worried
Resident, and no water shortage client exists in the code anyway. The earlier "water buyers
are rare" count was wrong: the water shopper (`waterSpace2`) shows as "Spacer". By id:
| visits per run | owner normal, day 70 | gym normal defaults (60 seeds, ~185 days) | gym `engine: water` (20 seeds, 120 days) |
|---|---|---|---|
| `waterSpace2` (budget ~145–195 + day/4) | 49 | 75 | 68 |
| `generic_water_buyer` | 6 | 9.6 | 6.4 |
| `waterStockpiler` | 0 | 1.7 | 0.9 |
The owner's water sales on days 68–70 were one or two bottles a day to that shopper (148 +
57). Their biggest sale was a spring-water bottle for 531 to an Upper Level Visitor on day 49.
In the gym the shopper comes, but the water engine has almost nothing to sell: in a traced run it treated 0 ml
by day 36, and the five or six filled vessels it holds are kept for the bottles and bays. It
also sells only water whose label matches its true grade, and the shopper takes only
Basewater/High Quality, never Pure. This is an executor gap: water output and what it offers.
The gym's rules are not the cause.

**Baselines on fresh seeds** (unchanged rules, recorded for later comparison): hard best
policy 9/60 wins on 1100301–1100360 (median 202 days; repo 31, rent 15, arrest 5); normal
defaults 58/60 buyouts on 1100201–1100260 (median day 184.5).

## Hard income — 2026-11-10: first hard wins (8/60 on fresh seeds)

**Result.** Two perks plus a vending machine the executor now stocks lift hard income enough
for the mortgage: `-perks vending+short_sleeper` with `hard_income_mortgage.json` wins **8/60
on fresh seeds 970001–970060** (Wilson 95 % CI 7–24 %; buyouts days 189–217; 10/60 on the
selection seeds 960001–960060). The same set without the mortgage (`hard_income_survive.json`)
survives a median 223 days, **+17.3±3.5 days and +280±32 a week of income** paired against the
previous best (`hard_search_best.json`) on the fresh seeds. Income below is before rent, days
31–100, paired mean±stderr over the same 60 seeds; days are mean days played. Executor
`3b57eba` (vending, beer, `jackson_sunday`) plus `landlord_vending`; every new lever is off unless
a perk or directive turns it on, and normal defaults on `3b57eba` are byte-identical to `050baeb` on
60/60 seeds (59/60 buyouts). Arms: `sim/lab/scripts/arms-income-*.txt`.

**What the owner's hard game did differently** (saves and bridge log, days 103–113; Rock Bottom,
perks proficient_scavenger + alert + short_sleeper + bad_lease). Net worth 4.8k on day 30,
13.2k on day 60, 30.5k on day 100 (gym: ~3.4k / 4.6k / 7.3k), after 16.6k on 21 Wild upgrades
and 9.7k at Dr. Jackson. Over days 103–113 it sold 12.3k at the counter (gym ~3.7k a week),
27 items a day in 7–17-item baskets (gym 12), bought 7.4k of stock at ~83 % of estimate, and
made 1.07k from a **vending machine** (9.1k over the game; the gym had none). Other differences:
short_sleeper (outings cost no customer), five hydroponic bays, the DNA manipulator, two wine
racks, ten current converters with three POWER_HIJACK tiers, three large storage bays, all
three renovations and both showcases, BREWER + shareholder, the furnace bought and later sold.

**Levers** (hard, apprentice, 60 seeds 960001–960060 unless marked):

| Lever | vs | Δ days | Δ income/week | Verdict |
|---|---|---|---|---|
| `-perks short_sleeper` | old best | +10.3±4.1 | +72±30 | helps |
| `-perks proficient_scavenger` | old best | −0.5±5.2 | −44±38 | neutral |
| sleeper + scavenger | old best | +14.3±4.0 | +111±34 | helps |
| owner's perks (+ alert, bad_lease) | old best | median 209 = | — | neutral (bad_lease rent eats it) |
| `-perks vending` (new: executor stocks it) | old best | +4.3±4.1 | +86±37 | small |
| **vending + short_sleeper** | old best | **+11.8±3.8** | **+240±32** | helps (they compound) |
| vending bought from Wild (`vending: buy`, 1 200 cash) | old best | −14.0±4.3 | −151±17 | hurts (too early) |
| + `bargain_buy: ev_positive` (= `hard_income_survive`) | vending+sleeper | +6.3±2.8 | −21±34 | helps days |
| fresh seeds 970001–060: survive set | old best | **+17.3±3.5** | **+280±32** | confirmed |
| mortgage `auto` (margin 1.0) | survive set | −9.7±2.3 | — | 7/60 wins |
| mortgage `take` | survive set | −39.4±3.7 | — | 8/60 wins, repo 45 |
| **mortgage margin 0.8** (0.6, 0.4 identical) | survive set | −20.1±3.3 | — | **10/60 wins**; fresh seeds 8/60 |
| margin 1.25 / window [100,…] / float 2 000 / repo reserve 2.5 / max spend 300 | margin 1.0 | | | 1 / 6 / 7 / 7 / 1 wins |
| looser buying (88/95, 95/100, executor defaults) | vending+sleeper | +0.7…+0.9 | −54…−57 | neutral: sellers ask at estimate, the floor and full grid bind, not the %s |
| `min_free_cells: 12` | survive set | −4.8±2.3 | −42±32 | neutral |
| `permit` unset (buy with a 500 margin) vs `buy` | vending+sleeper | −3.3±2.7 | −66±29 | neutral/slightly worse |
| `bays: 3` / `wine_min_age: 10` | vending+sleeper | −0.6 / −14.2±4.7 | −44 / −183 | neutral / hurts |
| `jackson_every: 7` / `ore_phone` + 2 furnaces | vending+sleeper | −1.3 / −0.2 | −77 / −73 | neutral |
| Red Imp beer phone / + shareholder (1 450) | survive set | −8.0±3.4 / −1.7±3.2 | −55 / −155 | neutral to slightly negative |
| landlord's vending machine (H18), +150 rent/week | survive set (sleeper[+scav] perks) | −41.6±3.5 | −89±37 | hurts: the compounding rent |
| renovations first (H19), no cash cap | survive set | −4.7±2.2 | −84±20 | neutral/slightly worse |

**The owner's tactics as hypotheses** (each on the survive set):
1. Scavenge nightly, smelt, sell: scavenging is essential (`scavenge: false` −101.6±5.7 days,
   −535±45 a week); the furnace is not (`engine: wine` −1.1±2.0, +18±37). **Helps / neutral.**
2. Household goods via the sign, vending: `jackson` sign_household + `prefer_types` household
   −2.3±1.2 days (neutral); the vending machine is **not** a bad call: switching it off costs
   −7.3±2.8 days and −163±35 a week. **Neutral / vending helps.**
3. Materials via the window: `prefer_types: MATERIAL` + sign_material changed nothing (+0.0±0.2;
   the concentrated window already picks its type). **Neutral.**
4. Jackson on his Sunday 10 % discount (`jackson_sunday`, new): −5.7±3.8, −54±37. **Neutral.**
   The refillable perfume and the keycard claim (H21) were not built: in code keycards unlock
   Jackson's locked stock, they give no discount.
5. Wine from small/normal berries, 2 per bottle, aged in racks: the executor already ferments
   two good berries a bottle when it has them; holding wine to age (`wine_min_age: 10`) hurts
   (−14.2±4.7). **Already done / aging hurts.**
6. Surplus/shortage speculation: the executor already restocks types an event raises (≥ 20 %).
   **Already done.**
7. Buy most things wholesale: it already buys wholesale first; looser limits neutral (above).
8. Purify tap water for wine and hydroponics: on hard the taps are never bought under the cash
   cap, so `tap: true` did nothing; forcing the taps with `upgrades_buy` costs −63 days (11 arrests
   for operating without a license) and the treated water left wine at 106 a bottle.
   Treated water in `engine: wine+water` gives 120 a bottle vs 122 without. **Neutral to
   negative in the gym.**

**Scale vs a second engine** (same base, roughly the same outlay of ~500–700): two more bays on
wine alone (`engine: wine`, `bays: 4`) −6.8±2.8 days; the furnace twice with no wine −10.2±2.5;
wine + furnace (two engines) is the base; wine alone −1.1±2.0. Neither scaling the wine nor
adding the furnace beats wine alone by a measurable amount: on hard the binding constraint is
trade volume and the rent clock, not production. The income that moved the needle came from a
new sales channel (vending) and free traffic (short_sleeper), not from more of an engine.

**Water** (owner's first game, normal: four purifiers, sales 37k by day 69):
| Arm | Result |
|---|---|
| hard, `engine: water` + tap | 142 median days (−84.5±3.1), water sales 663/week, trade sales halve (water crowds the grid) |
| hard, wine + water / wine + furnace + water | −5.3±2.3 / −8.8±2.2 days, income ±0 |
| hard, water + Jackson's purifier (2 owned by day 80 in a sampled run) | 146 median days |
| normal, defaults | 59/60 buyouts (day 182) |
| normal, `engine: water` + tap / + purifier | **3/60 / 2/60** buyouts |
| normal, wine + water | 41/60 |
| normal, beer phone + shareholder | 58/60 (day 210 vs 182) |
**Fidelity flag:** the owner says water "printed money" on normal; the gym's water engine sells
~470 a week there and loses. Water buyers are rare in the gym (in 20 normal runs: 43 Water
Stockpiler visits, ~220 each; the Retired Water Merchant and Water Trader come as sellers), and
shortage clients never fire in this build (`docs/re/PRODUCTION.md` §5, GUID-keyed sums). Worth
checking against a live log: water buyers per day and the price paid in a shortage. Aquaculture
is not modelled by the executor (no engine), so water-as-infrastructure for fish is untested.

**Deferred:** H17 (Security VIP race: transmitter, impostor reports), the refillable perfume,
storage/machine bays (items inside a bay are invisible to the gym's counter staging), current
converters, DNA manipulator, Rock Bottom start (runs in the gym, not compared).

**Routes for the strategist** (each tested, hard, fresh or selection seeds): vending + sleeper
survival (223 days, 0 wins); vending + sleeper + mortgage at margin 0.8 (8–10/60 wins, repo
34); the same with `mortgage: take` (8/60, more repo, earlier deaths); wine-only vs wine +
furnace (equal); water routes (lose on hard and normal in the gym). Next: a mortgage-time
policy that cuts the 34 repossessions (most repossessed takers signed on day 100 or later, when
payments pass ~1 760; most winners signed on days 75–125), and the water-demand fidelity check.

## Idle runner — 2026-11-10

LLM games leave most cores idle, so `sim/search/idle_runner.py` (`sim/search/README.md`) now
runs gym work under SCHED_IDLE (≤ 28 processes, paused while the game runs):
- **Standing scoreboard**: each new `main` HEAD plays hard (`hard_search_best.json`, apprentice)
  and normal (defaults) on the reserved seeds **990001–990060**, one line per commit in
  `var/scoreboard/log.jsonl`, with a regression flag (hard median −2 SE, normal −4 wins).
- **Continuous hard search**: local-search rounds around the incumbent (from
  `hard_search_best.json`, median 212 days, 0 wins), fresh seeds per round from 3 000 001; a
  candidate is promoted only on 60 fresh seeds (paired Δ survival > 2 SE, wins not lower).
  Promotions land in `var/search/hard/promotions.jsonl` for review and are not committed.

## Overnight run — 2026-11-09 20:17 → ~06:15 AST (unattended)

Lanes:
- **Executor** (`sim/gym`): make the gym's player competent — cash management, buying
  discipline, production engines, risk, and following directives.
- **Directives** (`internal/strategy`): the LLM strategist emits machine-readable directives
  (`docs/PROTOCOL.md`, `protocol/fixtures/directives.json`) that the executor follows.
- **Policy search** (`sim/search`): find the best fixed policy on CPU.
- **League** (`cmd/fenceleague`): LLM-planned games on heavy, drift and the 5090.
- **Claude as a player** (owner's suggestion): a Claude agent plays gym games day by day through a
  file-based step mode (snapshot in, directives out) — an upper bound for directive-level play
  and distillation data for local models.

## Morning summary (2026-11-10, end of the overnight run)

**Where it stands.** On normal, the gym's executor (`-human player`, executor `b9e104b`) now
**wins 117 of 120 games on fresh seeds (95 % CI 93–99 %)** by the mortgage route — up from 0
wins and a ~85-day median at the start of the night. Hard is still **0 wins** (0/60, CI 0–6 %);
the best hard policy (`sim/lab/scripts/directives/hard_search_best.json`, found by policy
search: wine + furnace, buy the permit, conceal, tighter buying) survives a median of **212 days**
on 60 fresh seeds, +18.6±6.8 vs furnace + concentrated window (from 32–39 at the start).

**What an LLM adds so far: nothing measurable yet — but now it can be measured.**
- The harness was confounded until 22:30: in plan mode the bridge, not the executor, played the
  counter. Fixed; a bridge with no LLM now reproduces the executor day for day (10/10).
- Qwen-27B *full* directives hurt (normal seed: −5,651 net worth by day 90); *delta* directives
  (the LLM changes only what it has a reason to, each change lint-checked) track the executor
  within noise and are now the bridge default. GLM delta reproduced the executor exactly.
- A first distilled model (QLoRA Qwen3-8B on 8k rows) lost to the executor on 6/6 seeds: it
  copied the scripted director's "mortgage: later" and never took the mortgage. Next round:
  down-weight scripted days, train on decision/switch days (`docs/research/DISTILL.md`).

**What moved the numbers (in order of impact):** lend Georgio 50 % (4× payback ~day 69) as the
cushion for the mortgage; take the mortgage on a rent morning with repossessable stock as a
buffer; a wine engine (hydroponic bay); nightly scavenging (once the gym actually did it —
a headless outing bug had silently skipped it after night 1); keeping fakes away from buyers
who can expose them; and removing two later changes that cost ~20/60 wins (a zero-estimate hold
and signs-first display), found by a forward bisect on fresh seeds.

**Hard: why no win.** The hard mortgage needs 6,000 cash on a rent morning and ~1,700/week of
payments; trading nets ~0 on hard and the best engine (two furnaces fed by phone-ordered ore)
adds ~450/week. Forcing the mortgage repossesses 23/30. Every lane (executor, policy search,
rollout search, four Claude players on hard) agrees: income must roughly quadruple. Your own hard game
(alive at day 113 with ~7.4k cash) did better than any of them — worth a session comparing it.

**Method lessons.** Single seeds are chaotic: 30-seed comparisons can't resolve differences
under ~6 wins; confirm on 60+ fresh seeds. Nobody's play (yours, YouTube's, Claude's) is
treated as ground truth — the gym decides by outcome.

**Not done / next:** distillation r2; a permit lever for the rollout search; hard income (water
engine demand, supplier orders, Dr Jackson's shop); drift needs a restart (HTTP 507 all night);
heavy training needs a scheduled window (no documented procedure on the Sparks — not attempted).

### 04:02 — scoreboard on 180 fresh seeds (executor `b9e104b`)
| Difficulty | Policy | Seeds (fresh) | Wins | 95 % CI | Median day |
|---|---|---|---|---|---|
| normal | `-human player` defaults | 900001–900120 | **117/120** buyout_mortgage | 93–99 % | win day 189 |
| hard | apprentice + `furnace_phone_2f.json` | 900001–900060 | 0/60 (rent 59, execution 1) | 0–6 % | ends day 203 |

### 03:43 — hard: furnace scaled to 209 days; forcing the mortgage loses
Ore is the furnace's limit; ordering it by phone (Wild's MINER upgrade) and running two furnaces
doubles furnace profit to ~450/week steady and lifts hard survival to a **median of 209 days**
(`b9e104b`, 30 fresh seeds, +21±5 vs 188). Some of those runs hold 5–8k cash around days 70–110,
near the hard mortgage trigger, so I tested taking it: `mortgage: take` on the same 30 fresh seeds
→ **repo 23, arrest 3, execution 3, rent 1, 0 wins, −52±14 days**. Payments (~1,700/week) still
need about four times what the furnace adds. Still 0 hard wins anywhere tonight. The rollout
search (6 samples to the end of the mortgage) tied the executor on normal (18/20 vs 19/20,
`1497718`): its candidate set can't see the permit.

### 03:15 — NORMAL: 116/120 on fresh seeds (regression found and fixed)
Forward bisect from `3912d75` (60 fresh seeds; the gym is deterministic per seed, so every
difference is a seed won or lost): two of main's changes caused the drop — holding items
estimated at ≤ 0 (sales −13 %, purchases −22 %) and putting signs first in the display case.
Both removed (`93c2a94`). **Scoreboard, executor `93c2a94`, `-human player`, 1000-day cap:**
| Difficulty | Seeds | Result |
|---|---|---|
| normal | 700001–700060 (fresh) | **58/60 buyouts**, median win day 188 |
| normal | 800001–800060 (fresh, confirmation) | **58/60**, median win day 188 |
| hard (apprentice) | 1–30 | 0/30; player median 146 days; **furnace + concentrated window 188 days** (rent 28, arrest 2) |

### 02:50 — first measured LLM directive play: full plans hurt, delta plans track the executor
With the plan-mode confound fixed, a bridge with no LLM reproduces the executor day for day
(10/10 games), so any gap is the LLM's directives. Qwen-27B on the 5090, paired seeds 93001+,
100-day screen (Δ = arm − executor alone):
| Arm | Normal 93001 | Hard 93001 | Hard 93002 |
|---|---|---|---|
| full directives | alive; NW d60 −1,815, d90 **−5,651** | rent day 76 (d60 −2,547) | d60 +519 (stopped d73) |
| delta directives | stopped day 3 | alive at 100; d60 +575, d100 −415 | ~even at day 44 (stopped) |
| GLM delta (heavy, still running) | – | d60 −31 | d61 −63 |
Full directives starved the shop (spend caps of 26–140 a day, cash floors near cash, sell floors
of 100 %) — now blocked by `protectBuying()`, but full still lost. Delta (the LLM changes only
what it has a reason to, every change lint-checked) stays within noise of the executor and is
now the bridge default (`2b24014`); a delta parser bug had been silently discarding every change.
No evidence yet that an LLM beats the executor: the sample is 1–3 games per arm because three
fixes forced restarts. The 5090 now runs the QLoRA distillation pilot (Qwen3-8B on 8,034 rows
from Claude's and the director's games) until ~06:00; GLM keeps playing hard on heavy.

### 01:45 check-in — best normal build: 54/60 on fresh seeds; a real regression on main
Measured on 60 fresh normal seeds (700001–700060, never used for tuning), fixed outings,
`-human player`, 1000-day cap:
| Build | Buyouts | Median win day | Other endings |
|---|---|---|---|
| **`3912d75` (furnace branch)** | **54/60 (90 %)** | 182 | execution 3, rent 2, repo 1 |
| `85ef288` (main) | 34/60 (57 %) | 196 | repo 12, rent 14 |
The executor's later changes on main cost ~20 wins in 60. A 30-seed toggle-off bisect couldn't
find the cause — single seeds are chaotic and neutral changes swung 13–22/30 — so a forward
bisect from `3912d75` on these 60 fresh seeds is under way, keeping only changes that don't
hurt. Lesson for every comparison here: 30 seeds can't resolve differences under ~6 wins;
use 60 fresh seeds and confirm on a second set.

### ~00:55 — hard survival up to a 181-day median; a normal regression to bisect
Re-baselined on the fixed outings (`85ef288`, 30 paired seeds, 1000-day cap, hard from the
apprentice start): **furnace + concentrated window 181 days** (player 145, +36), furnace 174,
concentrated window + buy the permit 174, buy the permit 160 — the gains come from cutting
arrests (11 → 1–2); those runs then die to rent. Still 0/30 hard wins. Nightly scavenging is
worth its lost customer on normal (every night 21/30 buyouts, every 2nd 14/30, off 5/30). But
normal fell from 29/30 (furnace branch) to **21/30** with the executor's latest changes
(fake-exposure guard, exact-debt bribes, Jackson sell-and-buy, furnace inputs held under any
engine) — being bisected now; the scoreboard's normal figure is the last good one until then.

### ~00:20 — gym fidelity bug: nightly scavenging never happened after night 1
Building the furnace engine exposed that headless map outings threw inside the travel callback,
so from night 2 the executor never reached the Dumping Ground — and never paid its cost (one lost
customer the next day). Fixed on branch `furnace` (`3912d75`), being merged by the executor's
owner. It moves every baseline: normal player 29/30 but buyout day **182** (was 154), hard median
118 with arrests up (the permit gets bought less often). Results measured on `d9c4d40` (including
the running LLM A/B) stay valid as paired comparisons but must be re-baselined. The furnace
itself (`engine: furnace`) nets only ~159/week on hard — input-starved — but gives the longest
hard survival yet, median **167 days** (0/30 wins), mostly by freeing cash for the permit.

### ~23:40 — hard: income is the wall; search and scripts don't beat the executor
Two more Claude players on hard (wine route: arrest d170 / rent d105; mortgage rush: rent
d140 / d133, both beating the executor on their seeds) and the director's hard arms all say
the same thing: the hard mortgage needs 6,000 cash on a rent morning and ~1,700/week of
payments, while the hard shop nets ~700–900/week — **income has to roughly double**. Found
on the way: Georgio repays 4× on the first rent morning that finds you short of the rent
(day 49 instead of 69, 15/16 scripted runs); aged wine gains 4–5 a night; apprentice is the
best hard start (median 139 days); the exact mortgage arithmetic is in `docs/re/MITIGATIONS.md`.
The executor lacks the owner's hard engines — a furnace (now being built by its own agent)
and wine aging (executor queue). The director (Claude's winning script) now only ties the
executor on normal (`6676f7c`); the fork-based rollout search (`3fa2074`, fork fidelity
verified at HEAD) lost wins with one sample per candidate (2/20 vs 6/20) — it needs 4–8
samples rolled to the end of the mortgage, which waits for spare CPU.

### 22:45 check-in — NORMAL IS SOLVED BY THE EXECUTOR (28/30, replicated on held-out seeds)
Executor `d9c4d40`, `-human player` defaults, normal, `-start none`, buyout or game over capped
at 1000 days; re-run from a clean worktree build:
| Seeds | Wins | Win day (median) | Other endings |
|---|---|---|---|
| 1–30 (tuning seeds) | **28/30** | 154 | repo 1, execution 1 |
| 500001–500030 (held out, never tuned on) | **27/30** | 154 | repo 1, rent 1, execution 1 |
| 90101–90130 (director agent's seeds) | 24/30 | 154 | repo 4, execution 2 |
| **all three sets** | **79/90 (88 %)** | 154 | |
| hard, 1–30 | 0/30 | – | rent 21, arrest 9 (median day 127) |
The line: wine engine (hydroponic bay), insurance off for 3 nights then janitorial every
second night, lend Georgio 50 % on day 43 (repaid 4× around day 69), take the mortgage on a
rent morning (~day 70), keep repossessable stock as a buffer, 12 payments → buyout day 154.
The same commit fixes the plan-mode confound (a bridge that defers now leaves the counter to
the executor), so LLM directive play can finally be measured: the T1 A/B (player alone vs
Qwen-27B full vs delta directives, 10 hard + 10 normal seeds, 1000-day cap) is running on the
5090. **Hard is now the frontier**: the executor, the policy search, the director and two new
Claude players (wine route; mortgage rush) are all aimed at a first hard win.

### ~22:50 — every plan prompt said all upgrades were owned; training pipeline ready
The snapshot builder listed every for-sale Wild upgrade as owned and none on offer, so each
LLM plan (and each training row) saw "all 49 upgrades owned"; fixed (`5bcd757`, with Landlord
Grace's re-buy cooldown now visible). Committed: the knowledge pipeline (`9990181`: 86
sources, ~2,900 claims, a code-verified PRIMER; GLM was right on 8/8 of its "confirmed"
verdicts but missed most code checks) and the T2 distillation pipeline (`9f8bbc6`: 1,418 rows
from Claude's games, of which only 51 days carry fresh reasoning — format and a standing
policy, not judgment yet). Heavy training was **not** started: no documented procedure or
training environment exists on the Sparks, and an overspend wedges a box, so per the owner's
"stop if risky" rule it's planned for a scheduled window (runbook kept locally). The 5090
pilot waits until the plan-mode A/B has had the GPU. Claude player 6 (water engine): the
engine netted 150–500/week because the executor can't serve water buyers (sell-and-buy
swaps), degrades output once a purifier is owned, and never restocks test strips — routed.

### ~22:15 — LLM plan-mode results so far were confounded; hard hits a rent wall
The T1 strategist A/B found that in plan mode the bridge, not the gym's tuned player, answered
counter decisions (act mode: the bridge's weaker Go heuristic; observe mode: every customer
dismissed). So "Qwen-planned games die earlier" (22:05 entry) measured the counter swap, not
the plans. League `t3-plan` was stopped after its 40 baseline matches (player: normal 1/10
buyouts, median 220 days; hard 0/30, median 114); the gym fix (bridge supplies only
directives, the gym's player keeps the counter) is first in the executor queue, then the
base-vs-delta A/B re-runs on the 5090. Claude player 2 on hard: rent on days 140 and 126 —
trading income plateaus at ~650/week while rent compounds ×1.1/week; ~25 policy variants
all die by day 112–140. The biggest lever found: lending Georgio 50 % (4× payback) instead
of the fixed 25 % — now an executor directive in progress, to be A/B'd on normal's mortgage route.

### ~22:40 — first Claude win; the winning recipe is being scaled
A Claude player won normal seed 90019 by **buyout_mortgage on day 154** (vintner + mortgage +
Georgio's 4× loan; `docs/research/CLAUDE-PLAY.md` player 5); its second game lost to repo by 6
credits, to two executor rules (floor starving the shop under the mortgage; permit bought
before a payment) — both routed for fixing. Player 4 established the mortgage mechanics
(15 % down, offer at cash ≥ 2× down on rent mornings, 12 payments at 50 % interest) and that a
trading-only shop can't carry it. Concealment and chemistry/drugs are mapped and lab-verified
(`docs/re/CONCEALMENT.md`, `docs/re/CHEMISTRY.md`). Next: the winning standing-orders script
becomes a deterministic director run on 30+ seeds for a real win rate (`sim/director`).

### ~22:05 — the league runs on the winning player; LLM plans need a floor
League `t3-plan` (gym 8a366a6, all three nodes, plan + directives) runs unattended; REPORT.md
in `var/league/t3-plan/` rewrites after each match. Baseline there: player normal 1/10 wins
(day 154), hard median 116 days. First finished LLM pair (seed 90001 normal, 200-day screen):
**player alone rent day 174 vs Qwen-27B-planned rent day 104** — free-form LLM directives do
worse than the tuned executor. Next: delta directives (the LLM changes only what it has a
reason to, lints revert risky deviations) and exemplars from strong play, so the LLM's floor
is the executor's level. Claude players' round 2: archetypes (vintner to day 252, fence 203)
and the buyout hunter (game over 133/173) — lessons routed; a vintner + mortgage player and a
mortgage-route player are now playing.

### 21:45 check-in — FIRST WINS (mortgage route)
Horizon now buyout or game over, capped at 1000 days. Executor `8a366a6`, 30 paired seeds, normal:
| Arm | Wins | Median win day | Endings |
|---|---|---|---|
| v2 default role | 0/30 | – | arrest 13, rent 17 |
| **player** (mortgage when cash + ½ stock + 80 % of 12 weeks' earnings cover the payments) | **5/30** | **154** | buyout_mortgage 5, repo 16, rent 9 |
| player, decline the mortgage | 0/30 | – | rent 30 (median day 286) |
| player, water engine / Water Merchant + Working Pipes | 0/30 | – | rent, arrest |
Hard: player median 118 days, 0 wins. The cash buyout (17k by day 150) is out of reach; the
mortgage (12 weekly payments of ~1,100–1,200 replacing rent, taken ~day 70 with ~5k cash) is
the win, and most takers lose to repossession, so cushion and timing are the levers — policy
search and rollout search are now aimed there, and the league uses this player.

### Measured node capacity (real fence prompts, 1/2/4/8 in flight)
| Node | Plan, single | Plans/min | Counter decisions/min |
|---|---|---|---|
| 5090 Qwen-27B | 7.6 s | ~8–9 | ~26–32 (2–8 in flight) |
| drift Qwen-Flash | 33 s | ~1.3 (out-of-memory beyond 1) | ~26 at 4, fails at 8 |
| heavy GLM (shared with the knowledge pipeline) | 81 s | ~1–3 | ~2–5 |
The 5090 is the quick-iteration node (T1 A/B tests); drift and heavy run long league games.

### ~21:35 — normal is survivable; three Claude players
Executor checkpoint `716e8a3` (30 paired seeds × 200 days): **normal median 200 days, 23/30
alive at day 200** (nw100 5792, nw200 4077); hard 90 days, 104 with wholesale ≤ 95 %. Still
no buyout. Owner tripled the Claude players: three agents now play two games each on
`716e8a3` — a buyout hunter (Water Merchant start with the expert's perks, and a plain
start), a hard survivor, and two archetypes (vintner, fence).

### ~21:25 — an expert proof that fast buyouts exist
Expert farming (`docs/knowledge/EXPERTS.md`, `docs/research/EXPERT-HYPOTHESES.md`): the only
verified fast buyout is a **cash buyout on day 21 on normal from a Water Merchant start**
(Working Pipes, Model Citizen, Sycophant, Aug Menace, Colorful Character; sales 33,160) — a
water business, not trading. The executor is getting a water engine so the gym can test it.
Habits shared by strong players: one engine early with everything reinvested, buy only at
15–25 %+ off, janitor every second night, nightly scavenging, stockpile before shortages.

### ~21:10 — first 200-day survival (Claude as a player)
| Game | Claude (directives) | Executor alone |
|---|---|---|
| normal 90001 | **alive at day 200**, peak net worth 6190 | rent, day 175 |
| hard 90002 | rent, day 91 | rent, day 56 |
`docs/research/CLAUDE-PLAY.md`. No buyout (price rose past 17k on normal). Its lessons are
going into the strategist (T1 round 1, with an A/B in the league) and its executor complaints
into the gym player (janitorial death spiral on hard, stock caps counting held wine, permit
default, engine over-buying, unused `prefer_types`, no loan bridging).

### 20:45 check-in — the executor is competent enough to matter
30 paired seeds, 200-day horizon, `-start none` (`be7cd17`, `-human player`):
| Difficulty | Before (v2 default) | Now (`player`) | Buyouts | Endings now |
|---|---|---|---|---|
| normal | 76 days, nw60 1003 | **146 days**, nw60 2178, nw100 2254 | 0/30 | rent 25, arrest 5 |
| hard | 34 days | **62 days** | 0/30 | rent 30 (no more aug) |
What moved it: shooting augs, bribing the permit official with donated goods from day 45,
declining the landlord's vending machines (+150/week rent, compounding), buying only below
estimate, and a wine engine. What still kills: rent from ~day 100 (normal) / ~55 (hard) —
margin plus ~3 wines a week falls behind compounding rent. The executor now reads
directives, and a step mode lets an agent play one day per file: **a Claude agent is now
playing a normal and a hard game** that way. League (full-LLM, 60 games of baselines done):
heuristic 48 days median, lab knobs 54; being rebuilt on the new player.

### 20:50 — first league (hard, 6 held-out seeds, 200-day horizon)
| Player | Games | Median survival | Ending | Net worth d30 |
|---|---|---|---|---|
| heuristic | 6 | 34 d | all rent | 174 |
| lab-executor knobs (in-process) | 6 | 46 d | all **aug** (presence ~105 by day 45) | 1233 |
| drift Qwen-Flash, plan mode | 2 | 34, 41 d | rent | — |
Plan-mode LLM games are byte-identical to the heuristic: the gym's player doesn't read plans
yet, so LLM plans can't change outcomes until the executor follows directives. Redirected
the league to full-LLM games until then. Throughput is one request at a time per node:
drift ~2 game-days/min, 5090 ~4; heavy is busy with the knowledge pipeline.

### Baseline at start (gym v2, `docs/research/EXPERIMENTS.md`)
| Difficulty | Best arm | Median survival | Buyout rate | What kills runs |
|---|---|---|---|---|
| normal | G + wholesale-only | ~85 days | 0 % | permit arrest day 80; rent days 77–126 |
| hard | any | 32–39 days | 0 % | rent; augs (≤ 11/40 runs) |

Live play (owner, hard, day 108–113): alive, cash ~7.4k, mature shop — the gym's
player is far weaker than a human; the rules are not the bottleneck.
