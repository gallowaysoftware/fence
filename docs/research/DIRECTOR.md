# The director: Claude's winning standing orders as a reusable policy (2026-11-09)

`sim/director/` turns the first Claude win (CLAUDE-PLAY.md round 2, player 5: vintner + mortgage,
`buyout_mortgage` on day 154, seed 90019) into a deterministic policy for the gym's step mode. It
reads each day's `snapshot.json`, writes `directives.json`, and makes no LLM calls. The rules come
from the winning `drive.py` script plus the moves that player made by hand.

```
cd sim/director
uv run play.py  -gym DLL -seed 90101 -dir OUT [-difficulty normal] [-start farmer] [-params P.json]
uv run batch.py run -gym DLL -out DIR -seeds 90101-90130 [-params P.json] [-human-only] [-parallel 3]
uv run batch.py report DIR...          # win rate, median win day, endings
uv run batch.py save-wins DIR DEST     # copy each won game's day folders (distillation data)
uv run trace.py GAME_DIR [FROM TO]     # one line a day: cash, stock, payment, wine, cash flow, reasons
```

`batch.py` copies `policy.py` into `DIR/code/` before it starts, so later edits don't change a
running batch. Each day's `why.txt` records the director's reasons. The gym runs with
`-human player -start farmer -days 1000`, so every game ends in a buyout or a game over.

## Bottom line

| build | arm | normal (seeds 90101–90130) | hard |
|---|---|---|---|
| ddedfb5 | `-human player`, farmer start | 2/30 | – |
| ddedfb5 | `-human player`, no start | 4/30 | – |
| ddedfb5 | **director round 1** | **11/30**, median win day 154 | – |
| d9c4d40 | `-human player`, farmer start | 25/30, median win day 154 | 0/30, median day 115.5 (arrest 15, rent 15) |
| d9c4d40 | `-human player`, no start | 24/30, median win day 154 | 0/30, median day 118.5 (arrest 13, rent 17) |
| d9c4d40 | director round 4 (full) | 24/30, median win day 154 | 0/30, median day 119 (arrest 4, rent 26) |
| d9c4d40 | director, thin (executor + permit rule) | – | 0/30, median day 119 (arrest 15, rent 14, execution 1) |
| d9c4d40 | director, thin + wine engine | – | 0/30, median day 119 (same as thin: d9c4d40 already runs wine on hard) |

On `ddedfb5` the director won about 3–5 times as often as the executor alone. On `d9c4d40` the
executor itself carries most of these rules (the Georgio peak, the mortgage float, the repo reserve,
evidence removal, 50 % Georgio): on the same seeds the director ties it on normal (24/30 vs 25/30
with the farmer start, 24/30 with no start). On hard every arm is 0/30. What the executor lacks
there (no furnace, no wine aging, only partial event speculation) is described under "Hard".

## Rules (`policy.py`, `mode: full`)

Every number is a field of `Params`; a `-params` JSON overrides any of them.

1. **Georgio, day 43** (`georgio_prep*`). On day 43 he borrows a share of the cash on hand
   (cash/4 on ddedfb5, cash/2 by default from d9c4d40) and repays 4x. He repays on the first
   rent or payment morning after day 48 when cash is short of the payment, or else on day ~69.
   On days 41–42 the director buys nothing and sells down toward cash + 70 % of trading stock,
   with a floor of 75 % of the estimate. On day 43 it holds all cash.
2. **Mortgage take rule** (`take_*`). The offer comes on a rent morning when cash ≥ 2 × the down
   payment. Take it only when the cash after the down payment is at least 1.5 payments + 1 000,
   with Georgio's pending 4x payback counted as cash. Otherwise answer `later`, never `decline`.
   `take_count_permit` also adds the permit fee while unlicensed (tried in round 3; it delayed
   mortgages and lost, so it is off in round 4).
3. **Build to the offer** (`build_*`, `accum_*`). From day 35, within 4 days of a rent day, if cash +
   70 % of trading stock reaches the target: floor at the target, spend only the excess, and sell
   down from 3 days out. From day 38, cap trading stock at 1 800 to let cash pile up.
4. **Favor in strict order** (`upgrades`, `cash_ok`). Favor waits for the next upgrade in the
   list instead of leaking to cheaper ones. Only WINEMAKER may be paid in cash. Two orders were
   tried: WATER_TRADER right after WINEMAKER (the lesson from the second game), and the winning
   game's own order (showcases, referrals and renovations first, water trader seventh), which did
   better. Shareholders (a 15 % supplier discount) soak up surplus favor. From evidence 50,
   EVIDENCE_REMOVAL jumps the queue.
5. **Permit** (`permit_*`). Set it every day. Buy only on the official's visit day, and only when
   cash − fee covers what is owed this week + 1 500 (under the mortgage: ≥ 2 payments). Otherwise
   `bribe`.
6. **Payment and rent days** (`floor_days`, `sell_days_*`, `pay_margin`, `mort_floor`). Floor at
   the amount owed from 2 days out. Sell down from 3 days out before rent and 4 before a payment,
   to the payment + 100. On other mortgage days set an explicit floor of 200: unset, the executor
   floors one whole payment and buys nothing (the trap that lost the second game).
7. **Supplier day** (`supplier_days`, day % 7 = 5). Bottles, yeast and seeds come late in the
   day, so trading stock is capped at its current value.
8. **Liquidity before aging** (`liquid_extra`, `max_hold`). Hold the 40 best wines only while cash
   ≥ amount owed + services + 600, and never while building, accumulating or preparing for Georgio.
9. **Grid release** (`berry_*`). When berries exceed what the bottles can use and no bottle can
   start, turn the engine off for the day so the surplus sells (kit, 4 waters and the berries
   needed are held).
10. **Wine off without seeds** (`stall_days`). If nothing has started for 10 days, nothing is
    fermenting and no seed is in hand or in a bay, turn the engine off except on supplier days.
    The first version lacked the seed check and switched the engine off while it was ramping up.
11. **LL rep** (`ll_floor`, `ll_bargain`). Below LL −10, bargain `discount` (round 1) or `never`
    (round 2 on).
12. **Augs** (`augs_normal`, `augs_hard`). Shoot (perfect detection on normal; a good kill pays
    100–250 and +2 LL).
13. **Engine** (`engine`, `hard_engine`). Wine on normal, none on hard.

`mode: thin` (for d9c4d40 and later) emits only the day, the permit rule (buy on a visit day once
cash − fee ≥ rent + 300) and any `thin_extra` executor fields, and leaves the rest to the executor.

## Results by round (normal, farmer start, seeds 90101–90130, 1000-day cap)

| round | build | change | wins | median win day | endings |
|---|---|---|---|---|---|
| executor alone, farmer | ddedfb5 | – | 2/30 | 164.5 | rent 19, repo 9 |
| executor alone, no start | ddedfb5 | – | 4/30 | 154 | rent 11, repo 14, arrest 1 |
| **1** | ddedfb5 | the plan above (initial: LL→`discount`, WATER_TRADER second, no mortgage floor) | **11/30** | 154 | repo 17, rent 1, arrest 1 |
| 2 | ddedfb5 | + `refuse` the permit while bribing fails, mortgage floor 200, LL→`never`, shareholders | 3/30 | 154 | repo 18, **arrest 9** |
| 3 (stopped at 24) | ddedfb5 | refuse off; permit fee counted in the take rule; permit-floor fix; EVIDENCE_REMOVAL rule | 4/24 | 161 | repo 15, rent 5 |
| 3w (stopped at 21) | ddedfb5 | round 3 + the winning game's upgrade order | 6/21 | 154 | repo 13, rent 2 |
| executor alone, farmer | d9c4d40 | – | 25/30 | 154 | repo 3, execution 2 |
| executor alone, no start | d9c4d40 | – | 24/30 | 154 | repo 4, execution 2 |
| 4 | d9c4d40 | round 3w without the permit fee in the take rule | 24/30 | 154 | repo 4, execution 2 |

On d9c4d40 the director ties the executor. All six of its losses ended without the permit.
OPERATING_WITHOUT_LICENSE crime led to the 2 executions; the other 4 were repos. A
round 5 that bought the permit earlier (cushion 300, 1.2 payments under the mortgage) was written
(`params r5`) but not run, because the CPU was oversubscribed.

Rounds 3 and 3w were stopped when main moved to d9c4d40. Their partial games are kept.

## Failure modes

- **Repo, by far the largest.** Under the mortgage the shop makes 100–700 a week against a payment
  of ~1 022. Wins made 650–1 400 a week; losses 100–700. Neither the cash left after the down
  payment (losses had up to 5 000) nor the 4 weeks of earnings before the mortgage predicted the
  outcome. Once the cash drops below a payment, the sell-down kicks in, stock runs out, sales fall,
  and the next payment is missed. A repossession takes goods worth a payment but does not count as
  one.
- **The permit inside the mortgage.** The executor buys the permit on `bribe` once bribing has
  failed and cash covers the fee + the rent. It checks the rent, not the mortgage payment, so 1 500
  goes days before a payment. In round 1, 9 of the 17 repos and 6 of the 11 wins bought it within
  two weeks of signing. It costs about 1.5 payments, but it does not on its own decide the game.
  Refusing instead (round 2) means OPERATING_WITHOUT_LICENSE crime and arrests (9/30). Counting the
  fee in the take rule (round 3) delays the mortgage and loses as many games.
- **A cash floor that blocks the buy.** A permit-day floor of fee + cushion makes the executor's
  check (cash ≥ fee + rent + floor) impossible, so the permit is never bought and crime builds to
  an arrest or execution (`crime ≥ 300`, evidence 100). The floor must be the cushion only.
- **Junk counted as trading stock.** With the engine off, held water and bottles count against
  `stock_cap`, and an accumulation cap then stops all buying (seed 90111: no sales for 2 weeks).
- **LL rep.** `discount` still demands discounts from sellers (walk-outs, −LL). LL fell to −20/−30
  in a third of the games, which cuts Lower Levels traffic.
- **Favor piles up** (2 000+ unspent by the last payment); nothing it can buy raises income much.

## Hard

Results on d9c4d40, no start, seeds 90101–90130: every arm is 0/30 and dies by day 115–119 (median).
The full director (wine off, permit set every day, LL guard, accumulation cap) cuts arrests from
13–15 to 4, but those games then end on rent at the same median day. The thin director's
buy-the-permit rule hardly ever fires: only 1 of 30 games had the 2 000 fee plus the rent in hand on
a visit day.

The rules (difficulty.json): rent ×1.1 a week, retail markup 0, permit 2 000, property 24 000
(+50/day after day 50). The mortgage is 12.5 % down (3 000) and 75 % interest over 16 weeks, about
1 640–1 700 a week if signed around day 56–70, and it is offered only at cash ≥ 6 000. In the gym
the shop never gets near that. The highest cash a run reaches is a median ~2 700 (max 3 700–4 400),
and most of it is Georgio's payback. A rent morning with cash below the rent brings his payback
early (seed 90101: 3 068 on day 56).

The owner's live hard game had ~7.4k cash at day 108–113, so this is not proof that hard can't be
won. It shows what **the executor lacks** (docs/knowledge/STRATEGY.md, PLAYER-NOTES.md):

- **No furnace.** Nothing in `sim/gym` loads or runs it (scrap → Low ingots, ore → High ingots,
  junk module 6 junk → 4 scrap, Dr Jackson buys materials). STRATEGY.md names it as hard's
  production income, and it is absent.
- **No wine aging on its own.** Finished wine is ordinary stock and sells at once. Only a plan's
  `hold_uids` (the full director's cellar rule) ages it. Wine production itself does run on hard
  at d9c4d40 (seed 90109: 61 bottles by day 100), but sold young it didn't change survival
  (thin and thin + wine are identical).
- **Speculation only in part.** The executor buys types whose price rises ≥ 20 % within 3 days up
  to their estimate, and holds water through a surplus. It doesn't buy other goods in bulk during a
  surplus to sell into the matching shortage, and it doesn't buy the Shady Customer's 50 cr event
  tip.
- **Nightly scavenging is present.** The Dumping Ground is visited every night it is open, with no
  wound cap by default, so this one isn't missing.

The director rules can't make up for missing production. The next step for hard is executor work
(furnace, cellar aging, event speculation), then a director that schedules them.

## Data

- Winning games (day folders with snapshot, directives, result and why.txt, plus summary.json):
  `var/director/wins/<build>-<round>-s<seed>/`, 48 games: ddedfb5 rounds 1/2/3/3w (11 + 3 + 4 + 6)
  and d9c4d40 round 4 (24). `summary.json` holds the parameters used.
- Batches: `$SCRATCH/director-exp/` (session scratch, not kept).
