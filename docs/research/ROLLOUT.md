# Rollout search over the directives executor (T2 groundwork)

`sim/rollout` improves the gym's directives executor (`-human player`) by one step of policy
iteration (Bertsekas; `docs/research/SOLVER.md` §7, "Rollout / approximate DP"): at a decision
morning, try each candidate plan, let the base plan play the following days in simulation, and play
the plan whose futures score best. The same machinery labels states for T2 search distillation
(`docs/PLAN.md` "Training path"): every decision is a JSONL row with the visible snapshot, the
candidates and their rollout scores.

## Method

**Branching by fork.** The headless game is single-threaded and deterministic, so the cheapest way to
reach a decision state K times is not to replay K games from day 1 but to `fork(2)` the running one.
`fence-rollout` hosts the episode itself (it references `sim/gym` like `sim/lab` does) and, at a
decision morning, forks one child per (candidate, sample). Each child plays its branch, writes one
JSON result and `_exit`s; the parent reaps them and plays on. A branch costs only the days it plays
(H instead of d + H): at day 100 with H = 14, 8× cheaper than replaying.

Three runtime constraints make fork safe in .NET; all are set by the project, nothing in the gym:
- **W^X off** (`DOTNET_EnableWriteXorExecute=0`; `fence-rollout` re-execs itself with it). With W^X
  on, JIT code lives in `MAP_SHARED` memfd pages, so a child that JITs a method writes into its
  parent's code: any exception, NRE or GC in a child crashed both processes.
- **No background threads that matter**: workstation non-concurrent GC, tiered compilation off
  (`Fence.Rollout.csproj`). Only the forking thread exists in the child; finalizers never run there.
- The child leaves with `_exit`, never by returning.

Verified two ways on every executor build used here (3b64ed0, 716e8a3, 8a366a6):
- `fence-rollout check`: branch without re-seeding, base plan only. The branch's net worth matches the
  main line day for day (40/40, 60/60, 40/40 days).
- `play -follow base`: candidates built, snapshots taken, branches forked, but the base plan always
  played. This reproduces `fence-gym run -human player` byte for byte (`cash_by_day`, `net_worth_by_day`;
  seed 1 × 70 days on 3b64ed0, seed 2 × 80 days on 716e8a3).

**No hindsight.** The gym is deterministic per seed, so a branch on the real stream would see the real
future (the customers, the inspections, the repossession roll). Each branch instead re-seeds the game's
generators (`Headless.Seed`: `UnityEngine.Random` and the game's `RNG`) with `hash(seed, day, sample)`
right after the morning snapshot. Branches sample futures *like* the real one; the main line keeps its
own stream. Sample m uses the same seed for every candidate (common random numbers), so candidate
differences are paired.

**Score** (toward a win: buyout, or game over, capped at 1000 days):
- a buyout (cash or `buyout_mortgage`): 20000 − days taken;
- any other ending (rent, arrest, repo): −10000 + 100 × days survived in the branch;
- otherwise buyout progress at the horizon:
  10000 × (net worth + mortgage equity − rent shortfall) / buyout price.
  - Mortgage equity is the principal share of the down payment and the weekly payments made, so taking
    the mortgage is not scored as a loss of cash.
  - Rent shortfall is the rent, or the mortgage payment, due within 3 days and not covered by cash.

A candidate's score is the mean over its samples. The best is played for `-every` days; ties go to `base`.

**Two candidate sets** (`sim/rollout/Candidates.cs`). Each candidate is a directives delta over the
base plan, held for `-every` days, then the base plan to the horizon.
- *General* (default): `reserve_rent`, `sell_down`, `sell_high`, `buy_tight` / `buy_loose`, `no_buy`,
  `bargain_aggressive`, `sell_hot`, the other engine, `services_none` / `services_ins_jan`, the other
  permit choice, `growth_<ID>` (one buyable upgrade first in the `growth` list, up to 3), `no_growth`,
  `upgrade_reserve`, `liquidate_deep`, `save_mortgage`, and `mortgage_take` / `mortgage_decline` once
  cash covers a down payment. That makes 14–19 candidates.
- *Mortgage focus* (`-focus mortgage`): decisions only in the mortgage window, meaning cash ≥ one down
  payment, or paying the mortgage.
  - Before the mortgage: `save_mortgage` (cash floor and upgrade reserve at 2 down payments, the offer
    threshold `PlayerStore.CanBeOfferedMortgage`) and `hold_cash_for_buyout` (with decline). From cash
    ≥ 1.5 down payments, also `mortgage_take`, `mortgage_decline`, `mortgage_margin_1.5` and
    `save_then_take`.
  - During the 12 payments: `cushion_2w` / `cushion_3w` (floor and reserve at 2 or 3 payments),
    `sell_down_2w` and `liquidate_deep` (sell down to 2 payments, at 50 % of estimate when liquidating).
  - Always: `no_buy`, `buy_tight`, `buy_loose`, `no_growth`.

## Results vs `-human player`

Paired seeds 1–20, `-start none`, same binary for both arms (`python3 sim/lab/scripts/arms.py report`).
The executor moved under this work three times. Each row is the build the comparison ran on.

| executor | difficulty | rollout configuration | `player` | rollout player | paired Δ (95 % CI) |
|---|---|---|---|---|---|
| 3b64ed0 | hard, 200-day cap | general, every 7, H 14, 2 samples | median 55 days; rent 20/20 | median 111 days (n = 10, then stopped by the rebase); rent 9, arrest 1 | days **+52 ± 10**, nw60 +1685 ± 519 |
| 3b64ed0 | normal, 200-day cap | same, seed 1 only (pilot) | rent, day 133 | alive at day 200 | — |
| 716e8a3 | hard, 1000-day cap | general, every 10, H 14, 2 samples | median 118; rent 17, arrest 3 | median 111 (n = 11); rent 9, arrest 2 | days **−7 ± 3**, nw100 −227 ± 178 |
| 8a366a6 | normal, 1000-day cap | mortgage focus, every 14, H 42, 1 sample | **6/20 buyouts** (`buyout_mortgage`, median day 154); repo 6, rent 8; median 160 days | **2/20 buyouts**; repo 10, rent 8; median 153 days | days −5 ± 16, nw100 −958 ± 725 |

Baselines for 716e8a3 and 8a366a6 are `fence-gym smoke -human player -days 1000` on the same seeds and
build (`var/rollout/eval-*/player-*`). On 716e8a3 normal the baseline median was 160 days, not the
200 the README reports: that was a working-tree build (`var/sim/gyms/g2`) and 30 seeds.

8a366a6 per seed (`player` / rollout):
- Rollout won where `player` lost: seed 7 (repo 153 → buyout 153).
- Rollout lost a `player` win: seeds 8, 13, 14, 18 (to repo) and 19 (to rent).
- Rollout survived longer without a win: seeds 1, 2, 12, 17, 20 (e.g. rent 174 → 251).
- Shorter or equal: the rest.

Why the 8a366a6 search loses wins:
- **One sample.** Once the mortgage is taken, repossession is mostly the luck of the payment weeks. In
  seed 13 on day 71, `buy_loose`, `no_buy` and `cushion_3w` each drew a repo future and `base` did not;
  with a single sample the argmax follows the draw.
- **Equity in the score.** Mortgage equity counts toward progress at the horizon. Taking the mortgage
  therefore looks good within 42 days even when the run is repossessed a few weeks later: in seed 8
  the search took it on day 77 (+1057 over base) and lost on day 146.
- **Next.** For this decision: 4–8 samples per candidate, with branches rolled to the end of the
  mortgage (84 days) rather than a fixed horizon. That is about 8× the cost, too much for ≤ 4
  processes tonight.

### 6 samples, rolled to the end of the mortgage (2bcc56a, normal)

Build 2bcc56a (main after 3fa2074; before 85ef288 and 3912d75). Paired normal seeds 1–20,
`-days 1000`. Flags: `-focus mortgage -every 7 -samples 6 -to-payoff -screen`.
- `-to-payoff`: each branch runs to the end of the mortgage. While paying, that is the weeks left × 7 + 7
  days. Before signing, it is a full 12-week term plus the hold plus 7 days (98 days). Branches stop
  early at a buyout, repo or other ending.
- `-screen`: the base plan is rolled first. When all 6 of its samples buy out, the other candidates are
  not branched (206 of 293 decisions).
- Mortgage candidates use the executor's levers. Before signing: `mortgage_take`, `mortgage_later`
  (a `mortgage_window` past this hold), `mortgage_float` 500 / 2000, `mortgage_margin` 1.5 and
  `save_mortgage`. During payments: `cushion_3w`, `repo_reserve_payments` 2.5 / 0.5,
  `mortgage_max_spend` 250 and an earlier, bigger sell-down. Always: `no_buy`, `buy_tight`, `buy_loose`
  and `no_growth`.

| | `player` | rollout player |
|---|---|---|
| buyouts | **19/20** (repo 1) | **18/20** (arrest 1, execution 1) |
| converted (player lost → search won) | | seed 7: repo day 133 → buyout day 154 |
| lost (player won → search lost) | | seed 8: buyout 189 → arrest 138; seed 9: buyout 161 → execution 134 |
| faster buyouts | | seeds 10, 12, 14, 19 (e.g. 210 → 154) |
| paired Δ | | days −7 ± 8, nw100 −287 ± 895 |

Net −1. That is within seed noise: neutral gym changes have swung 13–22 of 30.
- **Seed 8 lost to the permit.** The search chose `save_mortgage` and `buy_tight`. Both leave less
  stock, and the executor pays the permit official's bribe in goods. From day 82 the bribe failed and
  OPERATING_WITHOUT_LICENSE reached 1390 crime. The focus set has no permit lever to trade this off.
- **Seed 9 was a game-ending event the samples saw coming.** From day 88 most `base` samples end in
  `execution`. The search picked candidates that avoided it in their samples (`no_buy`, `buy_tight`),
  but the main line still hit it on day 134.

Cost: 349 971 branch days for 293 decisions (1 194 per decision; 6 690 when not screened). Two
drivers ran with 9 branches each, ~3.2 h wall, ~1 800 days/min combined at load ~30–40. Rows:
`var/rollout-head/rows-payoff-normal-all.jsonl` (293 rows, 15 MB; 206 are screened, base only).

## Dataset

434 rows in `var/rollout/dataset/` (11 MB), one file per executor build. The base plan differs between
builds, so keep them apart:

| file | rows | rows from | candidates |
|---|---|---|---|
| `rows-3b64ed0-general-hard.jsonl` | 158 | rollout play, hard, 10 games | general |
| `rows-716e8a3-general-hard.jsonl` | 125 | rollout play, hard, 11 games | general |
| `rows-8a366a6-mortgage-normal.jsonl` | 143 | rollout play, normal, 20 games | mortgage focus, 1 sample |
| `rows-8a366a6-mortgage-normal-e7m2-partial.jsonl` | 8 | one game cut short (every 7, 2 samples) | mortgage focus |

This is short of the few thousand rows asked for. The executor was rebased twice under this work, the
process budget fell from 8 to 4, and every row is a decision of an evaluation game. At the rates below,
3000 rows take 4–25 hours. `rollout.py dataset` (base-plan main lines on fresh seeds, `-seed0 1000`)
is the command to grow it once the executor settles.

One JSON object per line:

| field | |
|---|---|
| `seed`, `difficulty`, `start`, `day` | the game and the decision morning |
| `policy` | `rollout`: the main line played the best candidates (rows from `play`). `player`: the main line played the base plan (`-follow base`, rows from `dataset`) |
| `search` | `focus`, `every`, `horizon` (days actually rolled, shorter near the cap), `samples`, `score` (the rule in words) |
| `snapshot` | exactly what `-step-dir` writes to `snapshot.json` that morning, minus `respond_with` and `timeout_seconds`: the live mod's `day_snapshot`, plus `yesterday`, `risks` and `base_directives` |
| `candidates` | `[{name, directives, score, rollouts: [{sample_seed, score, game_over, days, nw_end, nw_delta, cash_end, rent_due, rent_days_left, buyout_price, mortgage, mortgage_equity, crime, evidence}]}]` |
| `best`, `best_directives` | the argmax (ties to `base`) |
| `margin_over_base` | best score − base score; 0 means the base plan was (one of) the best |

`directives` are deltas over the base plan, in the `docs/PROTOCOL.md` "Directives" vocabulary (plus
the gym's extensions), so a training target is "write these directives given this snapshot". A trainer
can weight or filter rows by `margin_over_base` and the per-sample scores (how decisive the search was).
Rows from different executor builds have different base plans: filter by file (build) before training.

## Cost and throughput

| run | processes | load (32 cores) | branch + main-line days | wall | days/min | decisions (rows)/hour |
|---|---|---|---|---|---|---|
| 3b64ed0 hard, general, H 14 × 2 | 7 branches + parent | ~35 | 54 716 + 1 065 | 649 s | 5 160 | 850 |
| 716e8a3 hard, general, H 14 × 2 | 7 + 1 | ~45 | 47 810 + 1 176 | 730 s | 4 030 | 620 |
| 8a366a6 normal, mortgage, H 42 × 1 | 3 + 1 | ~50 | 44 087 + 3 368 | 4 263 s | 670 | 120 |

- **Per process:** 220–740 days/min, depending on the machine's load.
- **Per decision:** K × M × H branch days. Measured: 355 per decision (general, 3b64ed0 hard) and 308 per decision (mortgage focus).
- **Without fork:** a decision on day d would cost K × M × (d + H), 5–10× more in the mortgage window.
- **Hardware:** CPU only. Branches are nice 10 under `rollout.py`; each is a copy-on-write fork of the
  running game.

## Running it

```
dotnet build sim/rollout -c Release
R=sim/rollout/bin/Release/net10.0/fence-rollout.dll
dotnet $R check -seed 1 -days 40                 # fork fidelity
dotnet $R play -seed 3 -summary var/rollout/s3.json -rows var/rollout/rows.jsonl -focus mortgage -horizon 42 -every 14 -samples 1 -par 3
python3 sim/rollout/rollout.py play -dir var/rollout/eval/rollout-mortgage-normal -seeds 20 -difficulty normal \
    -rows var/rollout/rows-mortgage-normal.jsonl -- -focus mortgage -horizon 42 -every 14 -samples 1 -par 3
python3 sim/rollout/rollout.py dataset -rows var/rollout/rows-player-normal.jsonl -seeds 20 -seed0 1000 -- -par 3
python3 sim/lab/scripts/arms.py report var/rollout/eval player-normal rollout-mortgage-normal
```

`play` flags:
- `-days 1000` (the cap), `-every 7`, `-horizon 14`, `-samples 2`, `-par 7`, `-max-upgrades 3`, `-margin 0`
- `-follow best|base`, `-focus mortgage`, `-exclude name,prefix_`
- `-directives FILE` / `-knobs` (the base plan, as in the gym)

Branches write to `/dev/shm/fence-rollout/<pid>`. At most `-par` branches run at once, under nice 10
from `rollout.py`. While `Probably Stolen.exe` runs, the parent drops itself and its future branches to
nice 19 and at most 4 branches.

## Gym hooks

One line: `<InternalsVisibleTo Include="fence-rollout" />` in `sim/gym/Fence.Gym.csproj`, as for
`fence-lab`. Everything else uses what the gym already exposes: hosting the episode, the planner
callback, re-seeding, and the morning snapshot (`StepMode.Snapshot`, via reflection).

## What we learnt, and next steps

- **Short horizons stop helping once the executor is good.** Against the 3b64ed0 executor, 14-day
  rollouts doubled hard survival. Against 716e8a3 they cost a week. That executor already plays the
  local levers well, and hard dies on a rent that compounds ×1.1 a week, which no 14-day branch sees.
  Most decisions picked a lever with a small margin over base. With 2 samples and 16 candidates, that is
  the winner's curse (`-margin` exists for it).
- **The decisive choices are long-range and noisy.** The mortgage (take, when, what cushion) decides
  the win, and its outcome is luck-heavy. Rollouts need many samples to the end of the payments, not
  one sample to a fixed horizon. The cheaper alternative is a learned value at the horizon (TD on these
  rows, labelled with how the run ended).
- **Candidates change one lever.** Combining the top two levers is a cheap second pass.
- **Expert iteration.** Train the strategist on these rows, make it the base plan, roll again.
