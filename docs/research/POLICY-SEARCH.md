# Policy search (gym, 2026-11-09/10)

Which *fixed* directive set should the gym's executor (`-human player`) play? This is a black-box
search over the executor's directive levers on real-rules episodes. The horizon is **win (buyout)
or fail, capped at 1000 days**, and the objective is **win rate first, then the earlier win, then
longer survival and net worth at day 100**. The search runs on training seeds, and the final
numbers come from held-out seeds, paired against the executor's defaults.

The target moved three times during the night as the executor improved: cbca73d knobs → 716e8a3
player → 8a366a6 mortgage route (normal) → **d9c4d40, hard**. On d9c4d40 normal is 28/30 wins on
defaults, so it is close to saturated; hard is 0/30.

## Bottom line (2026-11-10 02:45 AST, build d9c4d40)

**Hard: no directive set wins.** There were 0 wins in 2 526 training episodes over 179 policies and
0/100 on fresh held-out seeds (Wilson 95 % upper bound 3.7 %). The best set found buys survival and
fewer arrests, not wins. Measured on seeds 700001–700100, `-start apprentice`, paired against the
default:

| Policy | Wins | Median days [95 % CI] | Mean days, paired Δ | Endings | Permit owned | nw100 |
|---|---|---|---|---|---|---|
| player default | 0/100 | 125 [124, 129] | 129.2 | rent 64, arrest 36 | 33 | 3 550 |
| **best** (below) | 0/100 | **138 [132, 139]** | 137.9, **+8.7±5.7** | rent 78, arrest 22 | 66 | 3 603 (+53±369) |
| best without `conceal` and `inputs_max_spend` | 0/100 | 137 [132, 141] | 138.1, +8.9±5.7 | rent 70, arrest 29, execution 1 | 58 | 3 559 |

Objective Δ vs default: +0.008±0.006 for both. The training estimate was about +20 days on seeds
1–48; the held-out gain is roughly half that, the usual winner's curse.

Best hard directives (`-human player -start apprentice -directives FILE`):

```json
{"bays":3,"buy":{"single_max_pct":76},"conceal":{"policy":"always"},"inputs_max_spend":478,
 "mortgage_max_spend":898,"mortgage_window":[60,200],"sell":{"liquidate_min_pct":46},
 "spend_favor":false,"upgrade_reserve":2053}
```

What these levers do on hard:
- They keep cash rather than spend it: a 2 053 upgrade reserve, no favor spending, 76 % single-lot
  buying, and selling down to 46 % of estimate when short.
- That cash buys the permit about twice as often (66 vs 33/100), which cuts arrests from 36 to 22.
- The runs still die of rent about two weeks later.
- `conceal: always` moves arrests to rent deaths without changing days.
- Net worth at day 100 does not change, so there is no income gain. That fits the Claude players'
  finding that income binds on hard; the levers exposed today can't raise it.

**Normal**: there are no claims from this search. On 8a366a6 nothing beat the default (above), and
on d9c4d40 the defaults already win 28/30. Per the coordinator, the best normal executor is now
branch `furnace` 3912d75 (54/60). None of these results were run on it.

Next for hard: the income levers that don't exist yet (wine aging/holds, an engine that scales past
3 bays, supplier orders). Rerun `space-hard.json` once they are directives.

## Tool

- `sim/search/search.py` (usage in its docstring) and `sim/search/board.py` (a win table paired
  against the default). The spaces are JSON files in `sim/search/`: `space-hard.json` (current),
  `space-mortgage.json` (8a366a6, normal), `space-player.json` (716e8a3, broad) and `space.json`
  (`-human knobs`). A lever goes to the directives file (`dir`, a dotted path), to a gym flag
  (`flag`), or into `-knobs`. Knob names are checked against `sim/gym/Knobs.cs`.
- To rerun on a newer executor, rebuild the gym, add the new levers to a space file and start a new
  study (`var/search/<name>/`). A study stamps the gym and game binaries and refuses others. It is
  resumable: kill it and rerun with the same `-study`.
- **Objective per episode** (H = 1000; the run stops at a win, a game over or day H):
  - a win (`buyout*`) on day d scores **3 + (H − d)/H**;
  - alive at day H scores **1 + min(1, net worth / buyout price)**;
  - a game over on day d scores **0.9·d/H + 0.1·min(1, nw100 / buyout price on day 100)**, where
    nw100 = 0 if the run died before day 100. The 8a366a6 study used peak net worth here instead.

  Buyout prices are from `docs/re/DAY.md` (normal 12 000 → 17 000; hard 24 000 → 34 000). Any win
  outranks any survival, which outranks any loss. A policy's score is its mean over seeds, so the
  win rate dominates.
- **Search**: a cross-entropy method over a factored distribution: a Gaussian per numeric lever, a
  categorical per discrete lever, and Bernoulli draws for subset items and on/off gates (off means
  the executor's own rule). The 716e8a3 run sampled every lever from it. Policies that changed about
  10 levers at once all lost badly to the tuned default, so the later spaces use a **local search**
  (`"mutate": [lo, hi]`): each candidate redraws 1–3 levers of the incumbent, weighted toward the
  levers of interest. Each generation races 10–12 candidates, the 2 best so far and (in generation
  0) the reference policies by successive halving on training seeds 1–8 → 16 → 32. The
  distribution moves 30 % of the way toward the upper half of each later rung. Every 3 generations
  the 3 best are extended to 48 seeds.
- **Episodes**: one gym process per (policy, difficulty, seed), `-start none`, cached. At most 4
  processes at nice 10 tonight (the machine is oversubscribed), and 4 at nice 19 while
  `Probably Stolen.exe` runs.
- **Seeds**: training 1–48; held-out 700001–700100 for the final hard numbers (the coordinator asked for fresh seeds at 700000+); `-seed0` sets the base.
- **Gym builds**: each lives in its own clone or worktree with copied `var/gamedata` and
  `var/tools`. The current one is `../fence-policy-search-d9` (branch `search-d9c4d40`). The
  earlier ones are in `../fence-policy-search` (branches `search-716e8a3`, `search-8a366a6`).

## Space for hard (`space-hard.json`, d9c4d40)

Every episode is `-start apprentice` (coordinator, 23:05: the best hard start, median 139 days vs 123
for `none`). The local search starts from the player default. Per the Claude players the binding
constraint on hard is **income**, so income levers get the most weight (w3–4: engine, services,
buying limits, sell floor). Mortgage and loan levers get w1–2. Study:
`var/search/hard-apprentice-d9c4d40`. A 17-policy generation 0 on `-start none` (23:03) gave no
wins: the default was 0/32, median about 120 days, rent 22 / arrest 10. Weighted levers are marked
(w) below.

**Missing lever**: wine aging or holding (sell aged bottles later at a higher value). The Claude
players point to it, but the d9c4d40 executor has no directive for it.

| Area | Levers (unset = the executor's rule; README "Playing as an agent") |
|---|---|
| Engine | `engine` (w) default / wine / none / wine+water / water, `bays`, `tap`, `engine_reserve_max`, `inputs_floor`, `inputs_max_spend` |
| Permit | `permit` (w) default / buy / bribe |
| Loans | `georgio` (w) default lend_50 / lend_25 / no, `georgio_peak_days` |
| Mortgage | `mortgage` (w) auto / take / decline / accept_if_rent, `mortgage_margin` (w), `mortgage_float` (w), `mortgage_window` (w), `repo_reserve_payments` (w), `mortgage_max_spend`, `mortgage_selldown_days`, `mortgage_selldown_margin`, `buyout` |
| Cash, buying | `cash_floor` (w), `buy.max_spend` (w), `buy.stock_cap`, `buy.wholesale_max_pct` (w), `buy.single_max_pct` (w), `buy.restock_below`, `restock_missed`, `min_free_cells` |
| Selling | `sell.min_pct` (w), `sell.liquidate_min_pct`, `sell.sell_down_to_cash` |
| Counter | `bargain`, `bargain_buy`, `ll_rep_guard` |
| Services | `services`, `services_mode` (default ev / both) |
| Wild's Network | `growth`, `spend_favor`, `upgrades_cash_max`, `upgrade_reserve` |
| Risk | `conceal` (always / on_warning / never), `augs`, `scavenge` |

## Results

### Normal on 8a366a6: the mortgage route (21:47–22:45, superseded by d9c4d40)

Three generations of local search ran over `space-mortgage.json`, 579 episodes on training seeds
1–32. The table pairs each policy against the default on those seeds.

| Policy | Wins | Median win day | Endings | Δobj vs default |
|---|---|---|---|---|
| **default** | **8/32** | 154 | rent 16, buyout_mortgage 8, repo 8 | — |
| `sell.liquidate_min_pct: 40` | 8/32 | 154 | same | +0.003±0.005 |
| `mortgage: accept_if_rent` | 8/32 | 154 | identical to the default | 0 |
| `mortgage: take` (+ liquidate 40) | 8/32 | 154 | repo 15, rent 9 | −0.03±0.02 |
| `buy.stock_cap: 21626` | 4/16 | 161 | repo 5, rent 7 | 0 |
| `buy.max_spend: 952` | 3/16 | 154 | | −0.23±0.45 |
| `mortgage_margin: 1.25` | 4/32 | 168 | rent 26, repo 2 | −0.42±0.41 |
| `services: [INSURANCE]` | 1/16 | 196 | | −0.63±0.97 |
| 10 random multi-lever policies | 0 | – | rent / arrest | −0.8 to −1.0 |

On that build nothing beat the default:
- Every lever the executor exposed then left the win count unchanged or lowered it.
- A higher `mortgage_margin` swaps repo deaths for rent deaths and halves the wins.
- Signing immediately (`take`) turns rent deaths into repo deaths with the same 8 wins.

The levers that mattered (take-day window, a cushion at signing, a repossession reserve, spend caps
while paying, an earlier sell-down) were missing then. d9c4d40 adds them, with defaults that already
win 28/30 on normal.

### Hard on d9c4d40, `-start apprentice` (running since 23:06)

**01:00 checkpoint**: 8 generations, 1 519 episodes, 112 policies. **0 wins in all 1 519 episodes.**
Hill-climbing has moved survival up, from about 136 mean days for the early best (`inputs_max_spend:
416`) to about 150 for the incumbent, on training seeds 1–32/48:

```json
{"bays":3,"buy":{"single_max_pct":76},"conceal":{"policy":"always"},"inputs_max_spend":478,
 "mortgage_max_spend":898,"mortgage_window":[60,200],"sell":{"liquidate_min_pct":46},
 "spend_favor":false,"upgrade_reserve":2053}
```

- On 48 seeds it scores 0.151±0.006 (about 150 days; rent 40, arrest 8).
- The same policy without `conceal` and `inputs_max_spend` scores 0.145 (143 days).
- The default stopped at 16 seeds: 0.128, about 128 days, rent 12 / arrest 3 / execution 1. Its paired
  number will come from the held-out run.

What the climb picked up, in order:
- `sell.liquidate_min_pct` 46 (sell harder when short of rent);
- a large `upgrade_reserve` (2 053) and `spend_favor: false` (keep the cash);
- `bays: 3`;
- a mortgage spend cap with a window of days 60–200;
- tighter single-lot buying (76 %);
- `conceal: always`, which cuts arrests.

**02:16**: the search was stopped after 14 generations (2 526 episodes, 179 policies, 0 wins); it had been flat since generation 5. The held-out results are in "Bottom line" above (`var/search/hard-apprentice-d9c4d40/holdout.md`).

### Side result: the `-human knobs` family (cbca73d, 20:28–21:20, superseded)

This ran 6 CEM generations (8 900 episodes) with the old objective (200 days, no hold-out):

- **Normal**: best `-perks intimidating+secret_admirer+hoarder -knobs buy_margin=1.04,max_heat=33,ws_margin=1.01`,
  about 95 mean days against 79 for v2's best arm `G_wsonly`. All top 5 took Intimidating and
  wholesale-only buying.
- **Hard**: about 46 days against 36. All top 5 took Saving + Proficient Scavenger and `reserve=none`.
- 0 buyouts. The player executor beats this whole family by a wide margin.

## Missing levers and notes

- `growth`/`upgrades_buy` order, `production` orders, `show_uids`/`hold_uids` and `prefer_types` are
  per-day plans, so they are not searched.
- Perks are searched only in the knob and player spaces, not in the mortgage or hard spaces.
