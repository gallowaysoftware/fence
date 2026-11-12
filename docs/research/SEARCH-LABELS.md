# Search labels for the day-91 sprint (2026-11-11)

**Question.** Before any training: can rollout search (`sim/rollout`, docs/research/ROLLOUT.md) produce decision
labels that beat the scripted executor? Labels that do not beat it are not worth imitating. The target is the
`sprint_91` format: hard, the `sprinter` route, a win only if the shop is bought by day 91. The executor alone
won 31/60 on seeds 8300001–060 (this branch's gym reproduces 31/60 exactly).

**Verdict: no.** Search finds no decision that beats the executor on this format, and neither does anything
learnt from its labels. On 120 fresh evaluation seeds the executor wins 67; the online rollout policy (search at
every fork, follow the best) wins 66 (+0 −1 against the executor, McNemar p = 1.00); a rule policy fitted to the
labels wins 54 to 68 depending on how strictly it is filtered, never significantly better and once significantly
worse (54: p = 0.024). Only 19 of 674 labelled fork mornings (2.8 %) carry a switch that clears z ≥ 2 against the
executor's default, fewer than sign-flipped noise gives (28 on average). The format's real lever is upstream of
every mid-run fork: the route. The `smelter` route, executor alone, wins 86/120 against the sprinter's 67/120 on
the same seeds (+38 −19, p = 0.016). Training a model on these labels would teach it the executor.

## Setup

- Hard, `-start apprentice -perks vending+short_sleeper`, the `sprinter` route file as the base plan; main line and
  branches run to day 91 (`-days 91`), so every label is the format's own outcome, not a proxy at a horizon.
- Seeds: training (labels, the static screens, the route screen, the rule fit) 8700001–100; evaluation
  8800001–060 and the confirmation block 8810001–060, used only to score arms (the rules were fitted on training
  rows alone); 8820001–300 for a larger test of the rule policies. Executor baselines are
  `fenceformat lab` runs of the same gym build on the same seeds.
- Build: main 3df3861 + this branch; the gym and the executor are untouched (`sim/gym`, `mod/` unchanged), so
  the executor arm is main's executor. `fence-rollout check` still replays the main line day for day, and
  `-deadline 91 -follow base` reproduces the executor's run (seed 8300001: net worth 26 756 on day 91 in both).

## Why the sprint is hard to move: the buyout is a threshold on rent mornings

The landlord sells only on a rent morning (days 7, 14, …), when cash covers the price (hard: 24 000 + 50 a day
after day 50, so 25 700 on day 84, 26 050 on day 91, 26 400 on day 98). The executor's 60 runs on 8300001–060
end on day 77 (6), 84 (12), 91 (13), 98 (11) or are unbought on day 100 (18). The day-98 group misses by a week: on day 91 its net
worth is 25 849–28 649, above the price in 9 of 11 runs, but its cash is 23 100–25 400. Of the 18 runs still
unbought on day 100, 16 are within 2 200 of the price in net worth on day 91 (4 are above it), two are 3 900 and
7 700 short.

## Static screens first (training seeds 8700001–060)

One-line directive changes for the whole run, executor alone. Wins out of 60, and seeds won / lost against the
route on the same seeds:

| Arm | Wins | vs route |
|---|---|---|
| `sprinter` (route) | 35/60 | |
| `buyout_selldown_days: 7` | 35/60 | +0 −0 (net-worth path identical on 53) |
| … + `liquidate_min_pct: 60` | 35/60 | +0 −0 (identical on 53) |
| `buyout_save: [insurance, upgrades]` from 56 | 35/60 | +0 −0 (identical on 56) |
| `buyout_save` all four cuts from 56 | 31/60 | +5 −9 |
| insurance + upgrades cut from 42 | 30/60 | +6 −11 |

The obvious deadline levers do nothing: the executor's buyout sell-down only starts once cash + 70 % of stock
covers *today's* price (`ExecutorMorning.Plan`), so starting it earlier changes nothing for the runs that are
short, and by day 56 the upgrades are already bought. Saving harder or earlier loses runs on average. So a gain,
if any, has to come from state-dependent switches: exactly what search labels are for.

## Forks (`sim/rollout/Sprint.cs`)

A fork is a fixed morning with a few candidate directive changes. Each candidate is merged over the plan in
force (`keep`, the default) and held to the deadline. Rent falls on multiples of 7, so the sell-down forks sit one
rent ahead of the buyouts they aim at (84, 91) and on the eve of 91.

| Day | Fork | Options (delta over the plan in force) | Source |
|---|---|---|---|
| 28 | style | `clean_shop` (`buy.hot_stock_cap: 0`); pilot only: `fair_dealer` | agency style fork, the styles as fast as the generalist |
| 42 | save | `save_now` (insurance cut from today), `all_in` (insurance + upgrades from today), `save_all` (insurance, upgrades, vending, inputs); pilot only: `insured` (never cut) | agency sprint fork (sprint / all in / insured) |
| 56, 70 | save | `save_all` (from the fork on); pilot only: `insured` | same |
| 77, 84, 88 | selldown | `force_selldown` (`sell.sell_down_to_cash`: the day-91 price + 100), `hold` (`buyout: hold_cash_for_buyout`); pilot only: the same with `liquidate_min_pct: 60`, `no_buy` | the executor's buyout levers, forced past its gate |
| 38 (pilot only) | georgio | `georgio_peak_days: [40, 43]`, `georgio: lend_25` | Georgio's loan |

Not used from the agency's forks: the mortgage (16 payments end after day 91), the engine fork (the sprinter
already runs wine + furnace) and the fountain (no water engine). The 20-game pilot (8 samples) dropped the options
that lost (paired samples won / lost against keep): `insured` +10/−71, `georgio_25` +3/−43, `no_buy` +10/−27,
`fair_dealer` +16/−28, `georgio_peak` +17/−20; `force_selldown_liq60` played exactly as `force_selldown`.

## Labelling

- **Rollouts to the deadline.** At a fork, `fence-rollout` forks the running game once per (candidate, sample).
  Each branch re-seeds the game's generators with `hash(seed, day, sample)` (no hindsight; ROLLOUT.md), sample m
  uses the same seed for every candidate (common random numbers), and plays the candidate to day 91.
- **Score.** A buyout by day 91 scores 10 000 + 25 × days early; unbought on day 91, 4 000 × min(1.2, net worth /
  price); any other ending 0. So P(win) dominates, then days early, then closeness.
- **Samples.** 12 per candidate (8 in the pilot), 2–4 candidates a fork, `-screen` (when `keep` wins every sample,
  the others are not rolled: they can only win earlier). A switch label needs the paired z against `keep` ≥ 2.0;
  anything within that is labelled `default`. With 12 samples the paired standard error of a P(win) difference
  is ~0.15 (see "Why" below), so the gate passes only a gain of ~0.3 or more at one fork morning.
- **Rows.** `var/search/{pilot,labels}/rows/<seed>.jsonl` (28 MB, not committed): ROLLOUT.md's row format plus
  `fork`, `features` (the morning's visible numbers: cash, stock, net worth, the gap to the day-91 price, the
  executor's own cash-buyout projection carried to day 91 as `proj_gap`, 2- and 4-week gains, rent, evidence,
  insurance), per candidate `p_win`, `p_win_se`, `samples`, `score_se`, `wins_gained` / `wins_lost` (paired
  samples against `keep`), `z_vs_keep`, and `label` (`default` or the switch). The main line follows the executor
  (`-follow base`), so each label is "one switch from the executor's state".

### Label statistics (training seeds 8700001–100, 674 rows)

| Day | Fork | Rows | Screened | P(win) of keep | Switch labels | Argmax but not significant |
|---|---|---|---|---|---|---|
| 28 | style | 100 | 6 | 0.47 | 2 (`clean_shop`) | 35 |
| 38 | georgio (pilot) | 20 | 4 | 0.57 | 0 | 7 |
| 42 | save | 100 | 13 | 0.47 | 8 (`save_now` 3, `save_all` 4, `all_in` 1) | 58 |
| 56 | save | 100 | 22 | 0.53 | 2 (`save_all`) | 32 |
| 70 | save | 100 | 32 | 0.52 | 5 (`save_all`) | 41 |
| 77 | selldown | 95 | 28 | 0.52 | 1 (`force_selldown`) | 36 |
| 84 | selldown | 89 | 30 | 0.50 | 1 (`hold`) | 37 |
| 88 | selldown | 70 | 16 | 0.38 | 0 | 29 |

**19 of 674 fork mornings (2.8 %) carry a switch label; the best candidate is significantly better than the
executor's default at 2.8 % of forks.** That is not above chance: flipping the sign of every paired sample
difference at random (200 draws, same rows) gives 28 switches on average (95 % range 18–37). Pooled over all rows
the paired samples won / lost against `keep` are: `save_all` +364/−298, `save_now` +149/−123, `all_in` +137/−131,
`force_selldown` +89/−100, `hold` +89/−102, `clean_shop` +202/−231. By day 70 most states are already decided:
in the pilot, `keep`'s P(win) was exactly 0 or 1 at 3 of 20 states on day 28, 14 of 20 on day 70 and 13 of 15 on
day 88.

## Step 2: do the labels beat the executor?

Evaluation seeds 8800001–060 and 8810001–060; the executor is `fenceformat lab` on the same seeds and gym. Wins
are buyouts by day 91 (Wilson 95 % CI); +won / −lost are seeds the arm won and the executor lost and vice versa;
McNemar is the exact two-sided test on those; Δ day is the paired mean of the win day, a loss counted as day 98,
± 1.96 SE.

| Arm | 8800001–060 | 8810001–060 | Pooled 120 (95 % CI) | +won −lost | McNemar p | Δ day |
|---|---|---|---|---|---|---|
| executor (`sprinter`) | 37 | 30 | 67 (47–64 %) | | | |
| **online rollout policy** (search at every fork, 12 samples, gate z ≥ 2) | 37 (+0 −0) | 29 (+0 −1) | 66 (46–64 %) | +0 −1 | 1.00 | +0.1 ± 0.3 |
| rule policy, all 7 fitted rules | 33 (+5 −9) | 21 (+3 −12) | 54 (36–54 %) | +8 −21 | **0.024 (worse)** | +1.0 ± 1.2 |
| rule policy, the 3 rules that validate on both seed halves | 35 (+3 −5) | 27 (+7 −10) | 62 (43–60 %) | +10 −15 | 0.42 | −0.3 ± 1.0 |
| rule policy, day 70 `save_all` when cash ≥ 14 409 only | 36 (+3 −4) | 32 (+7 −5) | 68 (48–65 %) | +10 −9 | 1.00 | −0.8 ± 0.7 |
| *side result:* the `smelter` route, executor alone | 47 (+18 −8) | 39 (+20 −11) | **86 (63–79 %)** | **+38 −19** | **0.016** | −3.6 ± 2.3 |

On 300 more fresh seeds (8820001–300), where a rule policy costs nothing: executor 149/300 (44–55 %); the day-70
rule 149/300 (+21 −21, p = 1.00, Δ day −0.3 ± 0.4); the three validated rules 147/300 (+31 −33, p = 0.90). The
day-70 rule fires in 103 of 120 evaluation games (cash ≥ 14 409 on day 70 is the norm), so it is in effect "cut
vending and inputs from day 70", and that is worth nothing.

1. **Online search** is the upper bound of what the search knows at play time. It switched at 16 of 774 fork
   mornings (2.1 %) and changed the outcome of one game in 120, a loss. The plan in force is the executor's in 104
   of 120 games.
2. **Rules.** `sim/rollout/sprint_fit.py` searched every one- and two-condition threshold rule on the features for
   each (fork, option), keeping the rule with the largest summed paired gain in P(win) when that sum is z ≥ 2,
   at least 5 rows and 3 games, and one rule per fork day. It found 7 (`var/search/rules-all.json`):

   | Rule | In-sample gain (wins summed over rows), z | Out of sample, fit on one half of the seeds |
   |---|---|---|
   | day 28 `clean_shop` when stock ≥ 1 265 and 2-week gain < 1 262 | +2.2, 2.1 | +0.42, −0.71 |
   | day 42 `save_now` when stock share < 0.30 and evidence ≥ 6 | +3.7, 3.4 | +0.54, +1.25 |
   | day 56 `save_all` when stock < 1 397 and 2-week gain < 2 146 | +1.8, 2.1 | −0.25, −0.08 |
   | day 70 `save_all` when cash ≥ 14 409 | +6.0, 5.4 | +2.50, +2.83 |
   | day 77 `force_selldown` when stock < 587 and `proj_gap` ≥ 4 020 | +0.6, 2.6 | —, −1.83 |
   | day 84 `force_selldown` when `proj_gap` ≥ 1 060 and 2-week gain < 3 426 | +1.3, 2.1 | +0.04, — |
   | day 88 `hold` always | +0.7, 2.5 | +0.08, +0.38 |

   The full set loses 13 runs net on the evaluation seeds. The three that validate on both halves (42, 70, 88)
   are within noise of the executor, and on the training seeds themselves they tie it (54/100 against 54/100,
   +13 −13). The day-70 rule, the strongest pattern in the labels, is +1 net in 120.

### Sanity on the other deadline formats (8800001–060, executor `fenceformat lab`)

| Format | Executor (`sprinter`) | Online rollout policy | `smelter` route, executor alone |
|---|---|---|---|
| `purist_sprint` (the four vows, by day 91) | 20/60 (23–46 %) | 20/60 (+0 −0; 8 switches in 413 forks) | **36/60 (47–71 %), +22 −6, p = 0.004** |
| `challenge_clock` (challenge, the mortgage paid off by day 182) | 42/60 (57–80 %) | not run | 45/60 (63–84 %), +13 −10, p = 0.68 |

On `purist_sprint` the vows lock the style and the insurance (`locked`), so the online search had fewer live
options and still found nothing. `challenge_clock` is won by the mortgage's last payment on day 182, which none of
these cash-sprint forks touch; the mortgage-focus rollout search (ROLLOUT.md) and the hybrid planner v2
(PROGRESS.md) already found no gain over the executor on that decision, so it was not rerun. The route does not
matter there either.

## Why search does not beat the executor here

- **The levers move P(win) by a couple of points; one fork morning's rollouts cannot see that.** Pooled over every
  row, the best option, `save_all`, nets +66 wins in 2 620 paired samples: +0.025 P(win) per fork (`save_now`
  +0.027, `all_in` +0.006, `force_selldown` −0.005, `hold` −0.006, `clean_shop` −0.027). A candidate and `keep`
  on the same re-seeded future end identically in only 14 % of pairs, and their wins correlate at φ = 0.45: any
  directive change perturbs the game enough to decorrelate the future, so common random numbers halve the
  variance at best. With 12 samples the paired SE of a P(win) difference is ~0.15; seeing +0.025 at z = 2 needs
  ~1 800 samples per candidate per fork, 150× this run (1.1 M branch days). Even if real, +0.025 at each of three
  save forks is ~+4 wins in 60 at most, inside the executor's own CI.
- **Most states are already decided.** In the pilot, by day 70 the morning's P(win) is 0 or 1 in 14 of 20 states; the
  sprint is won or lost by how the shop traded in its first two months, which no late lever changes, and the
  early forks (28, 42) are where outcomes are most uncertain and the levers matter least.
- **The executor's sell-down is not the bottleneck.** Forcing the sell-down to the day-91 price (`force_selldown`)
  is +89/−100: selling stock at the liquidation floor in the last week converts too little to cash and loses
  some of the runs that would have bought anyway.
- **Aggregated labels overfit.** A rule search over 13 features × thresholds × options finds z > 2 patterns in
  noise; four of seven failed their own split-half check, and the full set is significantly worse than the
  executor on fresh seeds.

## What it implies

- **Training on these labels is pointless.** 97 % of them say "do what the executor does", and the 3 % that do not
  are indistinguishable from noise; neither online search nor any rule fitted to them beats the executor.
  A model distilled from them can at best reproduce the executor (DISTILL.md round 1 learned the default for the
  same reason).
- **The gain is upstream of the mid-run forks: the route.** The format leaves the route free (`sprint_91.json`
  names none; the lab plays `sprinter`), and the executor alone on `smelter` wins 86/120 against 67/120 on the
  evaluation seeds (p = 0.016) and 48/60 against 35/60 on the training seeds (+20 −7). `farmer`, `lean` and
  `rancher` are within noise of `sprinter` (37, 36, 36 of 60 on 8700001–060). This is a day-0 choice with no state
  to condition on, so it is a constant, not a label.
- **Routing each finding** (the stream's player is drift, Qwen3.8 Flash via MLX; no fine-tune on stream nights):
  - `smelter` for deadline sprints: **executor / format default** (`sprint_91` and `purist_sprint` measured on
    `smelter`; or the agency's day-0 route choice offers it with these odds) and **prompt advice** for drift's
    route pick ("for a day-91 deadline the smelter is the fastest route: 86 of 120 against 67").
  - The day-70 `save_all` rule, the strongest pattern in the labels: **not shipped**. On 300 fresh seeds it ties
    the executor exactly (149 = 149).
  - Mid-run sprint forks: **neither** prompt advice nor training. If drift is asked these forks on stream, the
    honest advice is "keep the route's plan"; its answer cannot be scored better than the executor's.
  - If search is to produce training signal at all, it needs a decision with a large effect: the route (done
    here by brute force) or levers that change income, not timing.

## Cost

| Run | Games | Branches | Branch days | Wall (sum per game) |
|---|---|---|---|---|
| pilot labels (8 samples) | 20 | 3 888 | 115 208 | 1.6 h |
| labels (12 samples) | 80 | 14 616 | 422 603 | 5.1 h |
| online 8800001–060 | 60 | 10 140 | 297 994 | 3.6 h |
| online 8810001–060 | 60 | 11 076 | 310 720 | 3.7 h |

4–6 games at once with 3 branches each (12–16 processes, nice 10), ~2 h wall per block on a box shared with
other gyms (load 20–40). A rule policy costs nothing: no branches.

## Reproducing

```
dotnet build sim/rollout -c Release
python3 sim/rollout/sprint.py -dir var/search/labels -seed0 8700020 -seeds 80 -games 2 -par 3 -- -samples 12 -follow base -screen -rows ROWS
python3 sim/rollout/sprint.py -dir var/search/online-8800 -seed0 8800000 -seeds 60 -games 2 -par 3 -- -samples 12 -gate 2 -screen -rows ROWS
python3 sim/rollout/sprint_report.py labels var/search/pilot/rows var/search/labels/rows
python3 sim/rollout/sprint_fit.py [-validated] -out var/search/rules.json var/search/pilot/rows var/search/labels/rows
python3 sim/rollout/sprint.py -dir var/search/rules-8800 -seed0 8800000 -seeds 60 -games 8 -par 1 -- -rules var/search/rules.json
go run ./cmd/fenceformat lab [-route smelter] -seed0 8800000 -seeds 60 -days 92 -out var/search/exec/sprinter-8800000 sprint_91
python3 sim/rollout/sprint_report.py compare var/search/exec/sprinter-8800000 var/search/online-8800 var/search/rules-8800
```

`fence-rollout play -deadline 91 [-gate 2.0] [-rules FILE]` turns on the sprint forks; without `-deadline` the
rollout search is unchanged. The pilot ran with the georgio fork and the dropped options (the build before
`Sprint.Dropped`); its rows are in the label set.
