# Claude as a player (2026-11-09, overnight)

A Claude agent played two full gym games through `-step-dir` (one directives file per day;
`sim/gym/README.md` "Playing as an agent"), executor `-human player` at `be7cd17`. Per-day
snapshots, directives, results and reasoning notes are kept locally in `var/play/claude-1/`.

How it played: days 1–7 by hand, then standing orders applied by a small script that
recomputed the rent floor, sell-down target, spend limit, wine holds and stock caps each day
and stopped for Wild offers, permit visits, augs, evidence, rent danger and low reputation;
the orders were revised 20 times. Routine days are policy output, not fresh reasoning.

| Game | Claude | Executor alone (no directives) |
|---|---|---|
| normal, seed 90001 | **alive at day 200**, net worth 2503, peak 6190 (day 104) | rent game over day 175, peak 4620 |
| hard, seed 90002 | rent game over day 91, peak 1734 | rent game over day 56, peak 1082 |

No buyout: on normal the buyout price rose from 12,000 to over 17,000.

## What worked (normal)
- Spending Wild favor as soon as it covers an upgrade (unspent favor is wasted): SHOWCASE_I,
  BUY_REFERAL, SELL_REFERAL, SHOWCASE_II, RENOVATION1–3 → attractiveness 700; later the supplier
  contacts.
- Power service off while no powered machine is owned.
- Aging finished wine (26 bottles worth 4,763 by day 90) — the source of the peak.
- Releasing the cellar when cash ran dry (lowest grades first): sales 0–2 → 12–25 a day.
- Turning the wine engine off late (day 169) to free ~1,100 in inputs into a food +50 % event.

## Mistakes and near-deaths
- Holding too much wine starved the shop twice: cash ~100 → spend limit 0 → nothing to sell.
  Aging (~1.5–2 %/day) doesn't pay for a dead counter.
- Executor stock caps count held wine and engine inputs; `sell.min_pct` silently disabled the
  executor's 70 % sell-down floor; an unset `permit` let the executor buy the permit (1,500).
- The wine engine over-bought inputs (2 berries per empty bottle + 8, two bioreactors) while
  water was the real limit.
- Late game: rent 513 → 753/week plus ~50/night of services outran income.
- Hard: retail markup is 0, so selling only converts stock at estimate; the executor's
  services rule switched janitorial off when cash couldn't cover rent + service, trash halved
  attractiveness and sales went to zero (days 39–61); Landlord Grace ran out on day 77 and rent
  jumped 514 → 1,079 → 1,700; bribe refusals kept logging unlicensed operation (evidence 83).

## Lessons for the strategist
1. Spend favor the day it covers an upgrade: Landlord Grace, WINEMAKER, SHOWCASE_I, BUY_REFERAL,
   SELL_REFERAL, SHOWCASE_II, RENOVATION1–3, supplier contacts.
2. Liquidity before aging: hold wine only while cash covers rent, a week of services and ~1,000
   for buying; otherwise sell the lowest grades first.
3. Set `permit` explicitly every day.
4. Don't set `sell.min_pct` unless managing sell-down yourself.
5. Size stock caps as held wine + engine inputs + trading target.
6. If no new wines start for ~10 days, `engine: none`.
7. On hard, trading volume isn't profit: grace, scavenging and janitorial first.
8. Power service off until a powered machine exists.

## Lessons for the executor
Force a service on below rent coverage; use `prefer_types` (announced shortages); hold wine by
grade/value and exclude held/reserved items from stock caps; cap engine input buying and handle
water for wine; allow the loan shark to bridge a rent day; marketplace requests and the wine
rack; per-customer detail in `result.json`; name the crime kind in `risks.record`.

## Round 2 — player 1, buyout hunter (executor `716e8a3`, 1000-day cap)
| Game | Ending | Peak net worth |
|---|---|---|
| normal 90011, Water Merchant + the expert's perks (minus Aug Menace) | arrest, day 133 | 4801 (day 105) |
| normal 90012, no start | execution, day 173 | 4013 (day 81) |
Buyout price: 12,000 to day 50, +50/day, capped at 17,000 from day 150; net worth grew ~+100/day
at best, so a cash buyout never came near. The executor does take the buyout when offered, but
nothing lets a plan target it. Both runs were killed by the player's own orders: `upgrades_buy`
paid shareholders/supplier contacts in **cash** once favor ran out (net worth 4392 → 687), and an
explicit `permit: bribe` kept unlicensed-operation crime building after the shelves were bare.
The water business — the expert's route — isn't possible yet: treated water only fills empty
vessels the executor never acquires, and is always reserved for wine. Takeaways: an **early
mortgage** (before ~day 60, price ~12k) looks like the realistic win for a trading shop; buy the
permit once affordable; never spend cash on non-essential upgrades. Data: `var/play/claude-2/`.

## Round 2 — player 3, archetypes (executor `716e8a3`, 1000-day cap, normal)
| Game | Ending | Peak net worth | Executor alone, same seed |
|---|---|---|---|
| vintner, seed 90015, farmer start | rent, day 252 | 6657 (day 132); 139 wines worth ~13,400 | rent, day 98 |
| fence, seed 90016, fence start | rent, day 203 | 3195 (day 70) | rent, day 182 (peak 6246, wine engine) |
Both paths peaked, then compounding rent (×1.05/week, 1,100–1,300 a week by day 230) outgrew
income; neither reached the mortgage threshold. The wine engine ran at ~4 bottles a week, not the
docs' ~270/night: wine bottles take 12 cells and the executor reserves every berry and water
vessel, so the stock grid fills and the winemaker's deliveries fail the free-cell check (he was
dismissed on ~8 of 11 visits, days 33–103); water ran dry twice. The fence loop (buy hot, cool,
sell clean) works but is thin; the fence kit sat frozen because the executor never stages
contraband or uses the smuggler bay; bribing instead of buying the permit cost 3,686 in goods vs a
1,500 fee. Favor piled up unspent (1,900–2,300). Lessons: buy the permit; persona upgrades only
from favor or surplus; plan the mortgage threshold (~4–5k cash) by ~day 120 once weekly earnings
stop beating the rent curve; give every archetype a funding engine.

## Round 2 — player 4, the early-mortgage route (executor `716e8a3`, normal)
| Game | Ending | Mortgage | Peak net worth |
|---|---|---|---|
| seed 90017 | rent, day 196 | never offered | 5804 (day 70) |
| seed 90018 | repo, day 105 (4 of 12 payments) | day 70: down 2000, 12 × 1022 | 5903 (day 70) |
How the mortgage works (game code): the down payment is 15 % of the price rounded to 500; the
offer needs cash ≥ 2 × the down payment (4,000 until ~day 114, then 5,000); 12 weekly payments
at 50 % interest (~1,022 at day 70, ~1,348 at the price cap) — 3–4× the rent of the time; the
loan shark's intro comes once when cash first reaches one down payment, then the offer is checked
only on rent mornings; repossession is all-or-nothing (stock at estimate must cover one full
payment, and it doesn't count as a payment). Georgio's loan pays back 4× (+1,600 / +2,726 on day
69) and was what made the offer possible. **Verdict: a trading-only shop plateaus at 4–6k net
worth by ~day 33 and can't carry the payments; the mortgage needs an income engine of roughly
≥ 1,000/week over services first.** Other findings: shooting augs disguised as Lower Levels
customers costs ~−5 LL each; `bargain: discount` still demands discounts from sellers (walk-outs
−3); favor piled up (7,146) while Landlord Grace was never re-armed after a miss.

## Round 2 — player 5, vintner + mortgage (executor `8a366a6`, normal) — FIRST CLAUDE WIN
| Game | Ending | Mortgage | Payments |
|---|---|---|---|
| seed 90019, farmer start | **buyout_mortgage, day 154 (win)** | day 70: down 2000, 12 × 1022 | 12/12 |
| seed 90020, same plan + fixes | repo, day 140 (cash 1049 vs payment 1055) | day 77: down 2000, 12 × 1055 | 8/12 |
Under the mortgage the shop netted about one payment a week (net worth drifted ~−100/week), so
the **cushion** decided it: Georgio's loan (25 % of cash on day 43, repaid 4× around day 70:
2,916 in game 1 vs 1,544 in game 2) put game 1 at ~4,700 cash when taking the mortgage. Wine was
steady income (~62/day finished in game 1, 110 bottles) but sold too fast to serve as the repo
cushion. Game 2 was lost to executor rules: a cash floor of one payment that bought nothing (no
water, bottles or yeast on supplier days; stock ran to zero), and a permit bought for 1,500 four
days before a 1,055 payment (its affordability check ignores the mortgage). Lessons: take the
mortgage only with cash after the down payment ≥ ~1.5 payments + ~1,000 buying float (count
Georgio's pending 4×); never buy the permit during the mortgage unless cash ≥ fee + 2 payments;
WATER_TRADER right after WINEMAKER (water binds); spend favor in strict order.

## Round 2 — player 2, hard survivor (executor `716e8a3`, hard, 1000-day cap)
| Game | Start | Ending | Peak net worth |
|---|---|---|---|
| seed 90013 | none | rent, day 140 | 3045 (day 70) |
| seed 90014 | gunsmith | rent, day 126 | 3930 (day 70) |
Previous Claude hard game: day 91; the executor alone: median ~119. On hard, trading income levels
off at ~600–700 a week by day ~60 (no retail markup, 5–9 customers a day) while rent compounds
×1.1 a week (~200 → ~1,000 by day 105); every one of ~25 executor/policy variants tested died
between days 112 and 140. The mortgage needs 6,000 cash on a rent morning; no run held more than ~3,000.
Levers that helped: preparing for **Georgio's day-43 loan** (stop buying from day 40 so cash
peaks; +1,500–1,700 net worth on day 69 — lending 50 % instead of the executor's fixed 25 % would
double it), and using Landlord Grace as late as possible with favor (15-day re-buy cooldown: two
deferrals, then full arrears of ~3,300). Lower Levels rep below −20 costs a daily customer
(Poor Reviews); demanding discounts when buying counts as a bad bargain. Rejected on hard, by
paired medians: buyer referral (56 days), seller referral (105), cash renovations before ~day 70
(98), buying the permit (98), wine from any start day (94–112).

## Round 2 — player 6, water engine (executor `8a366a6`, normal, Water Merchant start)
| Game | Perks | Ending | Peak net worth |
|---|---|---|---|
| seed 90021 | expert's set minus Aug Menace, + Savings | rent, day 84 | 2,119 (day 82) |
| seed 90022 | Working Pipes, Savings, Sycophant, Unlucky | rent, day 154 | 4,876 (day 86) |
The engine netted ~150–500 a week (best week ~740 at estimate), far from the ~1,000/week a
mortgage needs; the mortgage was never offered. The ceiling was the executor, not the route:
water buyers arrive as sell-and-buy swaps (empties in, filled water out) the executor can't
answer, so they were dismissed; owning a purifier skipped the tablets, so output degraded to
Rustwater; test strips ran out with no restock, so treated water kept its dirty label; batches
under-filled containers. A patched probe reached ~620/week before consumables ran out. Strategy
notes: show only clean water valued 145–350 (350+ High-Quality from day 14 brings the Water
Stockpiler); bargaining walk-outs cost Lower Levels −3 each — stop below about −20; Landlord
Grace can't be re-bought right after use, so two missed rents in a row are fatal.

## Round 3 — player 7, hard wine route (executor `d9c4d40`, hard, farmer start)
| Game | Ending | Peak net worth | Mortgage |
|---|---|---|---|
| seed 91001 | arrest, day 170 (operating without a license) | 7,235 (day 133) | never offered |
| seed 91002 | rent, day 105 | 4,264 (day 50) | never offered |
The hard mortgage needs 6,000 cash on a rent morning before ~day 91 (7,000 after), then 16
payments of ~1,615–1,900; gross income was ~800/week at day 40 and ~1,450 at day 130, and hard
trading nets roughly zero (days 76–90: sales 1,773 vs purchases 1,701). Findings: **Georgio repays
on a rent day once cash is below the rent** — spending down to 146 on day 48 brought the 4×
payback (2,828) on day 49 instead of ~69; **aged wine gains ~4–5 a night** and holding it ~15
days lived 8–38 days longer on 3/3 rehearsal seeds (the executor sold it after a day); the
farmer start's bays were sold before WINEMAKER was bought (executor bug); all 32 arrests in the
player arms were operating without a license under the bribe route. The executor has no furnace
engine. Lessons: plan backwards from the mortgage gate; collect Georgio early; hold wine; never
miss rent by a few credits (Grace's 15-day cooldown makes two misses fatal); buy the permit once
evidence starts rising.

## Round 3 — player 8, hard mortgage rush (executor `d9c4d40`, hard, apprentice start)
| Game | Ending | Peak net worth | Executor alone, same seed |
|---|---|---|---|
| seed 91003 | rent, day 140 | 5,968 (day 75) | rent, day 112 |
| seed 91004 | rent, day 133 | 4,280 (day 51) | rent, day 119 |
The mortgage was never offered: no run in any of ~10 static 16-seed arms reached 6,000 cash on
a rent morning (best 5,647), and the one scripted run that took an offer (day 77) made one
payment and was repossessed on day 91 — hoarding for the gate kills the shop's income. Verdict:
on hard the mortgage needs income near one payment a week (~1,700) first; the shop nets
~700–900. Useful: **apprentice is the best hard start** (median 139 days vs none 123, farmer 125,
gunsmith 118, fence 62); pulling Georgio's payback to day 49 by being under the rent on a rent
morning worked in 15/16 scripted runs; the exact mortgage arithmetic is now in
`docs/re/MITIGATIONS.md`. Executor gaps: engine inputs bought with `engine: none`; an unset
`permit` buys the 2,000 permit in the middle of a mortgage hoard; windfalls go to cash renovations.
