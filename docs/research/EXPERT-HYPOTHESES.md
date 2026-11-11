# Expert hypotheses for the gym

Habits that recur across the strong players in `docs/knowledge/EXPERTS.md`, each turned into
something the gym (`sim/gym`, `make gym-arms`) can decide by outcome. A habit is a hypothesis,
not a rule: experts play with skills the gym's policies lack (reading aug tells, running
machines, appraising), so a habit can be right for them and wrong for our policy, and the gym
says which. Source data: `var/expert/decisions.jsonl` (decision tuples), `var/expert/runs.json`,
`var/expert/creators/*.json`, `var/expert/habits.json` (all gitignored, rebuilt by
`cmd/fenceexpert`).

Status: **knobs** = expressible with today's knobs/flags (`sim/gym/Knobs.cs`, `-upgrades`,
`-perks`, `-warm`, `-start`); **knob needed** = a small new knob in the human role or
`GymHeuristic`; **gym gap** = needs a gym capability listed in `docs/research/EXPERIMENTS.md`
("What the gym needs to test the rest").

## Hypotheses

| # | Habit | Who does it | How to test it in the gym | Expected effect | Status |
|---|---|---|---|---|---|
| H1 | **Commit to one production engine in the first week and reinvest everything into it**; cash stays near zero until the engine compounds, then rises steeply. Water is the usual engine (purifier, filters, a second purifier, batteries). | Harvey (credits ≈ 0 until ~day 13, 4,339 spent at Dr. Jackson on 8 of 20 nights, buyout day 21), Fruit Newt (water, day-50 deed), HeyPashi (second purifier before shop space), TheNeomare (Water Trader on day 2), Darlom (two smelters), CthuLuck (printer every night), PerunGamingAU (furnace the day after the first rent); 7 of 9 | Arms by engine × week of first machine: `-start water_merchant` with the water moves once the gym has production moves; until then a `-warm` scenario at day 7 with `WATER_TRADER` and a purifier, compared with the same warm start trading only | Large: the only path we have seen reach buyout before day 50. Trading-only shops end by day ~33 on challenge (demo 1), ~40 on hard and 72–125 on normal (EXPERIMENTS v2) | gym gap (production moves) |
| H2 | **Fill each buyer to their visible budget**: stage several items (bundles of the same good in different sizes) until the total sits just under "Customer Budget". | Harvey (446 of a 476 budget; 116 of 195), Aliensrock (sizes sales to the buyer's budget), HeyPashi (fill the big container first, then the small bottle) | `max_stage` = 6 (default) vs 10 vs 14, and a new `stage=budget` rule that adds items while total ≤ budget × 0.95 | More revenue per covered customer; effect bounded by stock variety | knobs (`max_stage`); knob needed for the budget rule |
| H3 | **Insurance only when it pays**: skip it during the new-shop burglary grace (first ~3 nights) and on nights when the stock at risk × theft chance is below the fee; **janitorial every second night**. | HeyPashi (explicit EV: 30–50 % theft chance × cheap stock < fee), Aliensrock (insurance off after day 3, janitorial every other night), CthuLuck (grace period; lost a gun when it lapsed), Fruit Newt (Red Ink: janitor only, no insurance), PerunGamingAU (both about every second night, insurance kept on while contraband is in stock), TheNeomare and Sileevio (janitor every second night); 7 of 9 | New `services=ev` knob: insurance on iff night > 3 and Σ(stock value) × theft p > fee; janitorial on alternate nights. Compare with `services=both` (current default, +21 days over none) | Small saving (Harvey's whole-run insurance was 146, janitorial 37); risk of a large loss on the night it is wrong. The gym already says both-every-night ≫ none; this tests the middle | knob needed |
| H4 | **Buy only wholesale or well under estimate on hard/challenge** (Fruit Newt's stated rule on Red Ink), because a normal sale returns only base value there. | Fruit Newt (Red Ink), Aliensrock (hard: profit only from buying low), HeyPashi (takes 15–25 %-off lots) | Existing: `buy_margin=1.1,ws_margin=1.0` (wholesale-only) and `buy_margin=1.25,ws_margin=1.1`; difficulty hard and challenge | Normal: +6 days (EXPERIMENTS v2); hard: −5 days; challenge: no effect (demo below). The gym's trader is income-bound, so the habit only pays with an engine (H1) | knobs — tested, see demo |
| H5 | **Sell into shortages and crises, hold through surpluses**: no water or food sales on surplus days; dump water during a water crisis. | Aliensrock (sells mainly into shortages and buys intel on coming ones), HeyPashi (no water sales on surplus days), Fruit Newt (treats a water crisis as the moment to sell), Harvey (bought from Dr. Jackson during a +40 % water-supply crisis, sold jugs at 498), CthuLuck, PerunGamingAU, Sileevio; 7 of 9. Against: TheNeomare and HeyPashi refuse food bought during a shortage | `spec=1` (hold depressed items) vs off, plus a buy-side mirror: a new `spec_buy=1` that buys a good while an event depresses it | v1 found speculation neutral without forecasts; with events visible a day ahead expect a few hundred nw60 | knobs (`spec`); knob needed for buy side |
| H6 | **Hold stolen goods until heat is low, sell only under ~10 % heat, and never to Security buyers**; buy them cheap (thief discount stacks with wholesale). | HeyPashi (sells 7 %, holds 33 %), Aliensrock (waits a day for heat; never sells hot to security), Fruit Newt (hides hot stock off the window) | `max_heat=101,sell_hot=0` exists (hold until 0); a numeric `sell_hot=10` threshold would test the experts' version | Already +4 days / +470 nw60 for hold-until-clean (v2); expect a small extra from earlier turnover | knobs; threshold knob needed |
| H7 | **Liquidate junk and duplicates on rent eve** instead of dying holding stock; time contraband sales to the weekly premium buyer (Odin) just before rent. | Aliensrock (sells junk to reach rent on day 7), CthuLuck (sells medical to Odin right before rent), Fruit Newt (sells everything on the table and strips machine parts to raise cash for a big purchase) | New `liquidate=1`: when rent is due tomorrow and cash < rent, sell lowest-margin stock to every buyer at the budget-raising discount. EXPERIMENTS v2 saw G die holding 5,000–6,400 of stock | Converts rent deaths into survival; large on hard where the rent wall is day 32–39 | knob needed |
| H8 | **Scavenge the dumping ground every night early, never without a bandage or Giorgio's 3-night protection; drop it once an engine out-earns it.** | 8 of 9: Aliensrock, CthuLuck, Darlom, Fruit Newt, PerunGamingAU, Sileevio, TheNeomare, HeyPashi (stops ~day 20 when water makes ~10× a haul). Harvey goes only 4 of 20 nights (30 scavenges, 704 of loot, no injuries) and spends 8 nights at Dr. Jackson | `scav=always` with `scav_risk` low vs `scav=never` is already in v2 (neutral on normal, −3±2 days on hard); add a `scav_until=N` cut-off and an event-gated `scav=events` arm (Harvey's pattern) | v2 says nightly scavenging is noise for a trader; the experts' version is tied to Rock Bottom starts with no cash, so test on `-start rock_bottom` (dies by day 17 in v1 with every policy) where it should matter most | knobs; small knobs needed |
| H9 | **Showcase (display case) early, referrals only situationally**; skip signs until the stock matches them. | HeyPashi (display first, then seller referral, toggled), Aliensrock (weighs referrals to get trade flowing), CthuLuck (no signs too early) | `-upgrades LANDLORD_GRACE+SHOWCASE_I+SELL_REFERAL` vs baseline | Demo below: +2±5 days, n.s. on normal | knobs — demo below |
| H10 | **Spend early on capacity** (backpack, storage container, fridge, fridge efficiency) before shop cosmetics. | Fruit Newt (Giorgio's backpack day 5), HeyPashi (large container), Aliensrock (fridge + two efficiency upgrades), CthuLuck, Darlom, PerunGamingAU (satchel, then a discounted backpack); 6 of 9 | Needs stock-capacity accounting in the gym's buy rule (the footprint model exists; buys are not limited by it in the policy) | Enables H1 and wholesale; small alone | gym gap |
| H11 | **Report thieves, impostors and narcotics owners for the bounty**; shoot confirmed augs only. | HeyPashi (turns in thieves for the bounty, regrets not reporting narcotics for 100–200), Fruit Newt (+450 from reporting on day 1), PerunGamingAU and Aliensrock (aug tells; shoot only when sure) | `dlg` choices for report/ignore once the gym exposes the report and shoot decisions as decide kinds | Free cash early; the aug part is skill-gated (see H12) | gym gap |
| H12 | **Expert perk loadouts**: refund points with negatives that do not touch the plan (Blabbermouth, Gambling Addiction) to buy rep and engine perks (Working Pipes, Sycophant, Model Citizen), plus Aug Menace for bounty cash. | Harvey (exactly that 7-perk set), HeyPashi (negatives as the challenge), TheNeomare (achievement guide with a perk order) | `-perks` arms on the expert's start | Demo below: −15 days for Harvey's set in the gym, −10 without Aug Menace. The loadout pays only with the skills it assumes (aug reading, water engine) | knobs — demo below |
| H13 | **Use appraisal as an income stream** once the appraisal upgrades are in (HeyPashi estimates ~2,000 a week with water). | HeyPashi, Aliensrock (junior appraiser), Harvey (appraisal on day 6) | Appraisal tablet mode in the gym | Unknown; large per HeyPashi | gym gap |
| H14 | **Cash buyout as soon as cash ≥ deed price on a rent day**, not the mortgage; on normal that came on day 21 with 22,461 cash. | Harvey (cash, day 21), Fruit Newt (deed before day 50) | A gym rule `buyout=asap` once trajectories ever reach the price (none do yet) | Decides only when H1 works | blocked by H1 |
| H15 | **All-in versus a rent buffer.** Strong players split: convert every spare credit into stock and machines and liquidate if rent bites, or keep the rent (and more) back days ahead. | All-in: Harvey (credits ≈ 0 to day 13), Aliensrock (1 credit the day before rent), PerunGamingAU (broke after buying the furnace). Buffer: CthuLuck, Fruit Newt, Sileevio, TheNeomare | Already tested in v1 (`reserve=none|rent2|rent`, `floor=150|300|600`): no reserve ≈ rent2 (−2±3 days, +157 nw60), every fixed floor hurts (600 halves survival). Retest on v2 with an engine once H1 is possible | v1 sides with the all-in players for a trader; expect the gap to widen with an engine (idle cash does not compound) | knobs — prior evidence |
| H16 | **Stock breadth for variety buyers**: keep one of each item type and mix types in one sale (variety buyers pay about +15 % an item but take one of each). | Aliensrock, HeyPashi (stuck with bulk food when only variety buyers came), PerunGamingAU | A `breadth=1` staging rule (one of each type before duplicates) and a buy rule that prefers types not in stock | Small revenue gain; protects against bulk-stock gluts | knob needed |
| H17 | **Race Security to 100 (VIP)**: an alarm transmitter broadcasting to Security (+1 a night), report impostors and false theft claims, sell clean goods to Security buyers, buy requisition stock at a markup. At 100 there are no inspections and no burglary, so insurance and concealment stop costing anything and contraband can be held. | PerunGamingAU (Security 102 by day 56 on hard, E3); code agrees on the VIP effect (`docs/re/PERKS.md`) | Arm `sec_race=1`: install the transmitter on Security as soon as it can be bought, report every reportable visitor, never sell stolen goods to Security; drop insurance at 100. Measure the day Security reaches 100 and days survived against the default | Removes inspection seizures and insurance late in the run; cost is the transmitter's power and the bounties given up elsewhere | knob needed |
| H18 | **Take the landlord's vending machine** (+150 a week, compounding) and stock it with surplus water. | PerunGamingAU (day 43, E3), against the executor's fixed decline (`sim/gym/PlayerRole.cs`). His machine took 34–72 a night (about 240–500 a week, before the cost of the water stocked) and he called it disappointing | `vending=accept` with a water or snack stocking rule, against `decline`, on hard with a water engine | Unknown: the revenue beats +150 early, but the rent compounds at 10 % a week on hard | knob needed |
| H19 | **Renovation II early**: 500+ attractiveness gives the third extra customer a day and a bigger budget (Perun: +25 % credits, +3 customers), so buy it before more machines. | PerunGamingAU (regretted buying it late, day 56) | `growth` directive order with `RENOVATION1,RENOVATION2` first and a lower cash floor for them, against the default order | Positive if traffic limits sales, as it does once an engine produces more than the counter sells | knobs (`growth`) |
| H20 | **Water engine on Rock Bottom hard**: Water Trader paid in cash around day 21, then purifier, moisture farm (at most two), the gutterflow tap and a large tank; sell HQ water (≥ 99 %) at a discount so buyers take several bottles. | PerunGamingAU (E2), alongside H1's water players | `-start rock_bottom -difficulty hard` with `WATER_TRADER` and `GUTTERFLOW_TAP` forced, against the default engine choice; plus a `discount_multi` knob for multi-bottle buyers (see H2) | The only engine shown keeping pace with hard rent to day 59 from $0 | knobs (upgrades); knob needed (discount) |
| H21 | **Time big Jackson purchases to the day after rent** (10 % off, Sunday in `docs/sim/SPEC.md`), and claimed: a keycard takes 25 % off one large purchase. | PerunGamingAU (E1@41:59, E2@51:23) | `jackson_day=1`: defer Jackson buys over 300 to the discount day. Check the keycard claim in the decompile before any arm | Small but free | knob needed; keycard claim unverified |

## Demo runs (2026-11-09, this pass)

Small paired tests, 24 seeds per arm, 200 days, `-human knobs`, gym binary as built at 20:28
from the working tree (HEAD 3b5413b plus uncommitted `sim/` edits in progress) and copied before
use, `nice -n 19`, 8 processes. Output in `var/expert/gym/`. They are demos of the method, not findings: n = 24 cannot see effects under
~5 days.

### Demo 1: buy thresholds on Red Ink (H4)

`-difficulty challenge -start none`, default role.

| Arm | Days | Endings | nw30 | Δdays vs base |
|---|---|---|---|---|
| base | 33±2 | rent 24 | 50±30 | — |
| wholesale-only (`buy_margin=1.1,ws_margin=1.0`) | 34±2 | rent 24 | 42±15 | +1±3 |
| deep discount (`buy_margin=1.25,ws_margin=1.1`) | 34±2 | rent 24 | 42±15 | +1±3 |

No effect: on Red Ink every trading-only run dies of rent on day 27–41 whatever it buys. The
experts who say "only wholesale" on Red Ink also run an engine from week one; the habit cannot
be judged apart from H1.

### Demo 2: upgrade order (H9) on normal

G policy (`bargain=ev,max_heat=101,sell_hot=0`), `-start none`.

| Arm (`-upgrades`) | Days | Endings | nw60 | Δdays / Δnw60 vs baseline |
|---|---|---|---|---|
| baseline (grace, showcase I) | 73±4 | arrest 15, rent 9 | 1777±390 | — |
| grace, renovation I, showcase I | 75±4 | arrest 11, rent 13 | 1464±467 | +2±6 / −313±630 |
| grace, showcase I, seller referral | 75±3 | arrest 19, rent 5 | 1763±298 | +2±5 / −14±478 |
| grace, showcase I, endorsement I | 73±4 | arrest 15, rent 9 | 1777±390 | identical: 2,500 never affordable |

Nothing separable from noise at n = 24; the permit wall (arrest, day ~80) still decides most runs.

### Demo 3: Harvey's perk loadout (H12) on his start

`-start water_merchant -difficulty normal`, G policy.

| Arm (`-perks`) | Days | Endings | nw60 | Δdays / Δnw60 vs no perks |
|---|---|---|---|---|
| none | 75±3 | arrest 19, rent 5 | 1739±305 | — |
| Harvey's 7 (working_pipes, aug_menace, colorful_character, model_citizen, sycophant, gambling_addiction, blabbermouth) | 60±1 | **aug 21**, rent 3 | 756±310 | **−15±3** / **−983±449** |
| same without aug_menace | 65±3 | arrest 19, rent 5 | 1118±302 | **−10±3** / −622±393 |

The buyout player's loadout is clearly worse for the gym's policy: Aug Menace ends 21 of 24 runs on
day 55–60 (the policy never shoots augs), and the rest of the set still costs 10 days (not isolated; the
candidates are Blabbermouth's ×1.33 crime and Gambling Addiction's forced buys, while Working
Pipes' free water goes unused by a policy with no water moves). This is the
point of the method: the habit is right for a player who reads aug tells and runs water, and wrong
for our current policy. Retest when the policy can shoot augs and run the purifier.

Reproduce one arm (any of the three): `dotnet sim/gym/bin/Release/net10.0/fence-gym.dll smoke
-seeds 24 -days 200 -parallel 8 -difficulty normal -start water_merchant -human knobs -knobs
bargain=ev,max_heat=101,sell_hot=0 -perks <ids> -no-trajectory -dir var/expert/gym/harvey/<arm>`,
then `python3 sim/lab/scripts/arms.py report var/expert/gym/harvey -ref wm_noperks`.

## Suggested next arms (not run)

Ordered by expected value of the answer: H7 (liquidate on rent eve, needs one knob, hits the
gym's main death), H3 (services EV rule), H2 (budget filling; `max_stage` arms first), H5
(`spec=1` on normal and hard), H8 on `-start rock_bottom`, H16. H1, H10, H11 and H13 wait for gym
capabilities; H1 is the one that matters most: 7 of 9 strong players and the only verified
fast buyout run on one compounding engine, and no trading-only policy in the ~12,000 runs of
EXPERIMENTS.md v1 and v2 has reached buyout.

