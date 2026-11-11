# Strategy experiments (gym, 2026-11-09)

Controlled A/B runs of the real game headless (`sim/gym`, game 0.46D, normal difficulty) to find
which strategies win across days. Every arm plays the same seeds (paired comparison, common random
numbers); horizon **200 days or victory (buyout)**; 60 seeds per arm for experiments 1–9, 40 per
arm for the start-type sweep. Means are ± 95 % CI (1.96·sd/√n); "Δ" columns are paired
differences against the named reference arm, also ± 95 % CI.

> **Caveat on stolen goods (added 2026-11-09, owner's question, checked in code):** "buy hot,
> sell at heat 0" scored well partly because the gym's players kept a clean trafficking record,
> so inspections (none below ~50) almost never came, and runs ended by day ~100. At an
> inspection each stolen item outside a smuggler bay is found with chance = its heat %; found
> items are confiscated and cost Security reputation per item (−80: thorough inspections, −100:
> arrest). The gym can't stage items into bays yet. Treat the finding as conditional: fine while
> the record is low and hot stock stays off the counter and window; cap hot stock to hiding
> capacity as the record rises.

## Headline

1. **No policy the gym can express reaches buyout or day 200.** Across 75 experiment arms ×
   60 seeds and 88 start arms × 40 seeds (8 080 runs), buyout share is **0 %** and the longest
   run ends on day 105 (G + wholesale-only + permit, rent). The trading-only shop roughly breaks even, and rent compounds ×1.05 a
   week, so every run hits one of two walls:
   - **Rent wall (day 45–100)**: weekly gross margin (≈ 200–400) falls behind rent; Landlord
     Grace is used once and cannot be re-bought in the gym; the next unpayable rent ends the run.
   - **Permit wall (day 79–80, deterministic)**: refusing the business permit commits
     `OPERATING_WITHOUT_LICENSE` every visit from day 38 (`StoreClientList.CreateBusinessPermit`,
     opt2); crime reaches ~700, evidence 100 on day 79, sentencing → `arrest`. Of 2 310 arrests
     in the study, 2 252 (97.5 %) have `OPERATING_WITHOUT_LICENSE` as the dominant crime; the
     other 58 are trafficking/fencing arrests in the contraband-selling arm and the fence start
     (crime breakdown in each run summary's `end.crimes`). Buying the permit
     costs 1 500, which no gym policy can afford without starving the shop (exp. 5).
   So the end game is decided by the things the gym **cannot do yet**: upgrades bought with Wild
   favor/cash (Landlord Grace, referrals, showcase, renovation), production engines, supplier
   orders. Judge the findings below as "what keeps a trader alive and growing until the engines
   come online", not as a route to buyout.
2. **What clearly matters** (largest paired effects, all CIs exclude 0): services on (both
   insurance and janitorial: +22 days, +800 net worth at day 60), never pay full estimate
   (wholesale is the only profitable way to buy: removing it costs −26 days), EV-positive
   bargaining (+5 days, +800 nw60; aggressive bargaining loses −8 to −19 days), stolen goods
   held until clean (+6 days, +665 nw60). These stack: the combined policy **G** reaches
   nw60 ≈ 2 350 against 807 for services-only and 7 for the shipped default.
3. **The owner's strategies**: scavenging every night held up (never: −5 days, −340 nw30);
   letting heat die before selling held up (hold beats both selling hot and avoiding stolen
   goods). Never bargaining is safe but leaves ~800 nw60 against EV-positive bargaining, and is
   far better than bargaining aggressively. Speculation is neutral in the only form the gym can
   test (no forecasts); production, quality sorting and keep-vs-sell need machine ticks.

## Method

- Gym: `sim/gym` in worktree `fence-strategy-lab` (snapshot of `main` at 3ff3b96 plus the knobs
  below). In-process heuristic, `-human knobs`, `-difficulty normal`, `-start none` unless noted.
  ~40 000 game-days/min aggregate on 30 cores; a 60-seed × 200-day arm takes ~3 s.
- Reproduce one arm:
  `dotnet sim/gym/bin/Release/net10.0/fence-gym.dll smoke -seeds 60 -days 200 -human knobs -knobs "services=both,bargain=ev" -no-trajectory -dir var/sim/exp/NAME`
  (`-seed0 N` offsets the seed range; summaries carry `knobs` and an `end` block with rep, permit,
  crime breakdown, evidence, rent, net worth, stock count).
- Metrics: days survived; ending distribution (`rent`, `arrest`, `buyout`, …); net worth
  (`PlayerStore.GetTotalEstimatedValue`: cash + estimated stock) on day 30 and day 60 (0 if dead);
  peak net worth. Buyout share and buyout day would be headline metrics; they are 0 / undefined
  everywhere, so survival days and nw60 carry the comparisons.
- Outliers: two seeds (s1 in `svc_weeks=14`, s36 in `window=empty`) briefly own an item valued
  ~100 000 that never converts to cash (a valuation artifact), which blows up their nw30/peak
  CIs; their days and nw60 are unaffected. Treat nw30/peak of those two arms as unusable.

### Knobs (`sim/gym/Knobs.cs`, `-knobs "k=v,…"`)

Unset knobs keep the documented default; `-human knobs` with no knobs is byte-identical to
`-human default` (checked: same trajectory, seed 3, 60 days).

| Knob | Values | Where |
|---|---|---|
| `services` | `tablet` (default: unchanged, i.e. off), `none`, `ins`, `jan`, `both` | night |
| `svc_weeks` | cash buffer: service on while cash ≥ rent due + N × daily cost (default 7) | night |
| `scav`, `scav_risk` | `always`/`never`; stop scavenging when minor+major wound % exceeds N | night |
| `window`, `window_n`, `window_hot` | `types` (default: best item per type), `concentrate` (all of the top-value type), `top` (N most valuable), `empty`; cap; include stolen/contraband | morning |
| `sell_hot`, `sell_contraband` | 0 keeps stolen (heat > 0) / contraband off the counter | staging |
| `max_stage` | items staged per buyer (default 6) | staging |
| `spec` | 1 holds items a market event currently depresses | staging |
| `bargain`, `bargain_pct` | `never`, `discount` (default: budget-raising discount only), `ev` (discount, add-markup and demand-discount when the label odds make them EV-positive), `aggr` (always markup/demand at `bargain_pct`) | counter |
| `buy_margin`, `ws_margin` | buy only when estimate ≥ price × m (regular / wholesale) | counter |
| `max_heat`, `buy_contraband` | refuse lots with any item at heat ≥ N; allow serious contraband | counter |
| `reserve`, `floor` | `rent2` (default: keep rent when ≤ 2 days left), `rent` (always), `none`; extra fixed floor | counter |
| `save_permit` | from day N, reserve 1 500 (+ rent) until the permit is owned | counter |
| `stock_cap` | stop buying while stock value exceeds N | counter |
| `nice_faction`, `nice_pct` | always make the "nice" bargain (offer discount when selling, offer markup when buying) to this faction (`lower_levels`, `upper_levels`, `revolution`, `black_market`, `all`) | counter |
| `permit`, `dlg` | `buy` the permit when cash ≥ 1 500; `smart` declines Georgio's loan and Eleanor's 50-credit gift, never reports the cartel | dialogue |

Reference policies: **B** = `services=both`; **G** = `services=both,bargain=ev,max_heat=101,sell_hot=0,window=concentrate`.

### Fidelity caveats that apply everywhere

From `sim/gym/README.md`: items are **1×1** (no inventory or showcase pressure; a concentrated
window holds up to 35 items of one type), **no per-second machine ticks** (no production), **no
upgrade buying** (Wild favor accumulates unspent; Landlord Grace, referrals, showcase,
renovation, fixer, evidence removal all unavailable), no supplier phone orders, story/inspection/
barter/appraisal/info-dealer customers are dismissed, permit-official bribe donations impossible
(the official is non-dismissable and uncovered). The counter policy reads label odds, not
memory. Starting machines are stock to the gym: production starts **sell their engines**.

## 1. Services: none vs insurance vs janitorial vs both

Setup: reference = shipped default (`services=tablet`, which leaves both off: identical to `none`).
Services kept on while cash ≥ rent due + 7 days of the service.

| Arm | n | Days | Endings | nw30 | nw60 | Peak | Δdays vs none | Δnw60 vs none |
|---|---|---|---|---|---|---|---|---|
| none / default | 60 | 46±2 | rent 60 | 481±75 | 7±8 | 1604±47 | — | — |
| insurance | 60 | 58±3 | rent 59, arrest 1 | 1099±138 | 293±136 | 1820±115 | +12±3 | +286±134 |
| janitorial | 60 | 53±2 | rent 59, arrest 1 | 816±112 | 166±135 | 1702±106 | +7±3 | +159±135 |
| both | 60 | 67±3 | rent 50, arrest 10 | 1459±132 | 807±176 | 1958±115 | **+22±3** | **+800±177** |
| G with none | 60 | 55±3 | rent 60 | 1069±117 | 224±94 | 1873±63 | — | — |
| G with ins | 60 | 69±3 | rent 36, arrest 24 | 1655±131 | 985±211 | 2159±128 | +14±4 | +761±232 |
| G with jan | 60 | 67±3 | rent 43, arrest 17 | 1764±180 | 1148±273 | 2365±181 | +12±4 | +924±255 |
| G with both | 60 | 76±2 | rent 18, arrest 42 | 2392±132 | 2355±275 | 3144±246 | **+21±3** | **+2131±285** |

Cash buffer (`svc_weeks` 0/2/14 vs 7, on B): no effect (Δdays −1±2, −1±2, −0±3; Δnw60 within ±230).

Verdict: **both services, always.** The effect is the largest in the study and larger on a
stronger policy (+2 131 nw60 on G): uninsured nights burgle stock (30 burglaries in 76 days in
a diagnostic run where low cash kept switching insurance off) and garbage cuts attractiveness by
up to half (`garbage × 5 %` of base) and with it the queue. Insurance alone beats janitorial alone
on the base policy; on G they are equal; together they compound. The buffer rule does not matter.
Fidelity: burglary and garbage are exact ports; with real footprints stock is smaller, so
insurance (priced by net worth) is cheaper and burglary losses smaller in absolute terms.
**High confidence.**

## 2. Bargaining: never vs EV-positive vs aggressive

Setup on B. `never` = sell/buy at list; `discount` = shipped rule (budget-raising "offer
discount" only when the budget binds); `ev` = also add-markup / demand-discount when the label's
accept/refuse/walk-out odds make the expected cash positive (ignores the rep cost of "bad"
bargains); `aggr` = always add-markup/demand-discount at 25 % (or 10 %).

| Arm | n | Days | Endings | nw60 | Peak | Δdays vs never | Δnw60 vs never | LL rep at end |
|---|---|---|---|---|---|---|---|---|
| never | 60 | 66±3 | rent 48, arrest 12 | 783±190 | 1903±105 | — | — | +15.7 |
| discount (shipped) | 60 | 67±3 | rent 50, arrest 10 | 807±176 | 1958±115 | +1±4 | +24±248 | +15.3 |
| ev | 60 | 71±3 | rent 31, arrest 29 | 1581±250 | 2515±197 | **+5±4** | **+798±301** | −24.1 |
| aggr 10 % | 60 | 58±3 | rent 59, arrest 1 | 277±98 | 1889±97 | −8±4 | −506±216 | −48.7 |
| aggr 25 % | 60 | 47±2 | rent 60 | 54±45 | 1883±86 | **−19±4** | −729±202 | −95.4 |
| G-never | 60 | 72±2 | rent 30, arrest 30 | 1499±249 | 2376±179 | — | — | +23.5 |
| G-discount | 60 | 75±2 | rent 27, arrest 33 | 1975±319 | 2679±267 | +2±3 | +476±405 | +26.1 |
| G (ev) | 60 | 76±2 | rent 18, arrest 42 | 2355±275 | 3144±246 | +4±3 | **+855±357** | −30.0 |
| G-aggr 10 % | 60 | 65±3 | rent 50, arrest 10 | 857±195 | 2158±105 | −7±4 | −642±320 | −57.2 |

Verdict: **bargain only when the stated odds make it EV-positive; never bargain on reflex.**
EV-positive adds ~+800 nw60 on either base. Aggressive bargaining is the worst policy tested
after "no services": walk-outs (−3 rep each) and bad-bargain rep multipliers drive Lower Levels
toward −100 (−95 at 25 %), shrinking traffic. The owner's "never" is close to the shipped
discount-only rule and costs ~800 nw60 against EV-positive play. The EV rule already costs
~40 LL rep by day 75; a rep-aware EV (charge the rep penalty) is the obvious next refinement,
and matters for runs that would go past day 100 (LL −100 is the `mob` ending).
Fidelity: odds formulas are exact ports; walk-out rep loss is real. **High confidence** on
direction; medium on size.

## 3. Stolen goods: avoid vs hold until clean vs sell hot

Setup on B. `avoid` refuses any lot with a stolen item (heat ≥ 1); `default50` = shipped (refuse
heat ≥ 50, sell whatever heat); `hot` = buy any heat, sell hot; `hold` = buy any heat, never
stage an item while stolen (heat falls 18/night → clean in ≤ 6 nights; no smuggler bay in the
gym, so held goods sit in the main stock); contraband variants allow serious contraband.

| Arm | n | Days | Endings | nw60 | Peak | Δdays vs avoid | Δnw60 vs avoid |
|---|---|---|---|---|---|---|---|
| avoid | 60 | 66±3 | rent 52, arrest 8 | 743±181 | 1952±118 | — | — |
| default (heat < 50, sell hot) | 60 | 67±3 | rent 50, arrest 10 | 807±176 | 1958±115 | +2±2 | +64±160 |
| hold heat < 50 | 60 | 68±3 | rent 48, arrest 12 | 844±190 | 2038±140 | +2±2 | +101±164 |
| buy any, sell hot | 60 | 69±2 | rent 35, arrest 25 | 1302±264 | 2218±153 | +3±3 | +559±275 |
| **buy any, hold until clean** | 60 | 72±2 | rent 35, arrest 25 | 1407±234 | 2293±141 | **+6±3** | **+665±268** |
| contraband, sell | 60 | 58±3 | rent 25, **arrest 35** | 801±249 | 2269±191 | **−8±4** | +59±338 |
| contraband, hold | 60 | 64±3 | rent 49, arrest 11 | 804±255 | 1956±137 | −2±4 | +61±282 |
| G-avoid | 60 | 69±3 | rent 33, arrest 27 | 1374±305 | 2420±197 | — | — |
| G-sell hot | 60 | 71±3 | rent 18, arrest 42 | 1989±285 | 2958±217 | +2±4 | +615±386 |
| G (hold) | 60 | 76±2 | rent 18, arrest 42 | 2355±275 | 3144±246 | **+6±3** | **+980±356** |

Verdict: **buy hot goods (they are cheap and raise rep ×1.5–3), and hold them until heat 0
before selling** — the owner's practice held up. The value is in the cheap hot lots (raising
the heat cap from 50 to "any" is most of the gain); holding adds survival over selling hot. Selling
serious contraband is net negative: −8 days and 35/60 arrests (9 directly for trafficking; in the
rest the trafficking crime stacks on the permit crime and brings sentencing forward). Arrests in
the other arms are the permit wall, not fencing.
Fidelity: heat decay, fencing crime and inspection cadence are exact ports; with no smuggler
bay in stock, held items would be found at inspections (found chance = heat %) — rare here
because trafficking crime stays < 50 when nothing hot is sold. 1×1 items understate the space
cost of holding. **Medium-high confidence.**

## 4. Window: concentrate vs spread vs empty vs hot

Setup on B. `types` = shipped (best item of each of 11 types), `concentrate` = all items of the
type with the largest summed value (the showcase client follows that type; tier 2 at ≥ 145,
tier 3 at ≥ 350 from day 14 — SPEC §4.4), `conc8` = the same capped at 8 items, `top` = the N most
valuable items regardless of type, `empty`; `_hot` includes stolen/contraband.

| Arm | n | Days | Endings | nw60 | Peak | Δdays vs types | Δnw60 vs types |
|---|---|---|---|---|---|---|---|
| types (shipped) | 60 | 67±3 | rent 50, arrest 10 | 807±176 | 1958±115 | — | — |
| concentrate | 60 | 70±3 | rent 38, arrest 22 | 1113±232 | 2152±149 | +2±4 | +307±297 |
| concentrate ≤ 8 | 60 | 67±2 | rent 48, arrest 12 | 865±221 | 1982±108 | +0±4 | +58±276 |
| top value | 60 | 67±3 | rent 51, arrest 9 | 777±183 | 1927±84 | −1±4 | −30±249 |
| empty | 60 | 62±2 | rent 60 | 300±90 | (artifact) | **−6±3** | **−507±196** |
| types incl. hot | 60 | 66±3 | rent 50, arrest 10 | 717±162 | 1972±111 | −1±2 | −90±162 |
| concentrate incl. hot | 60 | 69±3 | rent 42, arrest 18 | 1028±237 | 2090±145 | +1±4 | +222±303 |
| G-types | 60 | 75±2 | rent 18, arrest 42 | 1950±259 | 2759±188 | — | — |
| G (concentrate) | 60 | 76±2 | rent 18, arrest 42 | 2355±275 | 3144±246 | +1±3 | +405±362 |
| G-empty | 60 | 73±2 | rent 36, arrest 24 | 1715±228 | 2462±168 | −2±3 | −235±352 |

Verdict: **a stocked window matters (empty costs −6 days, −500 nw60); concentrating on one type
is a likely but unconfirmed gain** (+300–400 nw60, CIs touch 0). The gain disappears when
capped at 8 items, so it may be an artifact of 1×1 footprints letting a case hold 35 items of one
type and push that type over the tier-3 threshold. Showing hot goods gains nothing (security
clients roll heat on the showcase; no measurable cost either at this horizon).
Fidelity: **strongly affected** by 1×1 footprints. Re-run after the footprint fix.
**Low-medium confidence** on concentrate; high on "not empty".

## 5. Cash floor, rent reserve, the permit

Setup on B (and G). `rent2` = shipped (reserve rent when ≤ 2 days to rent), `rent` = always keep
the rent due, `none`, fixed floors on top of `rent2`; `permit=buy` buys the permit when cash ≥ 1 500
at a visit; `save_permit=D` also reserves 1 500 from day D; `stock_cap` stops buying above a
stock value.

| Arm | n | Days | Endings | nw60 | Δdays vs rent2 | Δnw60 vs rent2 | Permit bought |
|---|---|---|---|---|---|---|---|
| rent2 (shipped) | 60 | 67±3 | rent 50, arrest 10 | 807±176 | — | — | 0 |
| rent always | 60 | 69±2 | rent 46, arrest 14 | 580±150 | +2±3 | −226±197 | 0 |
| no reserve | 60 | 66±2 | rent 55, arrest 5 | 964±208 | −2±3 | +157±258 | 0 |
| floor 150 | 60 | 66±3 | rent 53, arrest 7 | 497±138 | −2±3 | −310±220 | 0 |
| floor 300 | 60 | 65±2 | rent 58, arrest 2 | 339±111 | −3±3 | −468±199 | 0 |
| floor 600 | 60 | 31±1 | rent 60 | 0±0 | **−36±3** | −807±176 | 0 |
| permit when affordable | 60 | = rent2 (never affordable) | | | 0 | 0 | 0 |
| save for permit from day 30 | 60 | 63±2 | rent 60 | 214±70 | −4±1 | −592±160 | 2 |
| save from day 45 | 60 | 67±3 | rent 57, arrest 3 | 384±90 | −1±1 | −423±125 | 2 |
| G + permit when affordable | 60 | 77±2 | rent 32, arrest 28 | 2155±260 | +1±1 vs G | −200±149 vs G | 14 |
| G + save from day 45 | 60 | 75±2 | rent 34, arrest 26 | 1324±168 | −0±1 vs G | −1030±201 vs G | 7 |
| G + wholesale-only + save 45 | 60 | 79±3 | rent 43, arrest 17 | 977±193 | +4±3 vs G | −1377±328 vs G | 38 |
| G + stock cap 1 600 | 60 | 72±2 | rent 32, arrest 28 | 1561±196 | −4±2 vs G | −793±293 vs G | 0 |

Verdict: **cash idle in a floor is cash not compounding — keep only the rent reserve near rent
day.** Every fixed floor and stock cap hurts; a 600 floor halves survival. Buying the permit
converts the day-80 arrest into a rent death a few days later (mean +1 to +4 days) at a large
net-worth cost: worth it only once the shop can carry 1 500 without starving — which the gym's
trading-only shop never can. In the real game the permit is unavoidable for a long run; the
strategist should plan the permit as a **day 38–75 cash target funded by something other than
trading margin** (upgrades/production), and treat the permit official's weekly visit (from day 38,
`d % 7 == 3`) and the day-79 arrest as a hard deadline.
Fidelity: the bribe-donation path (giving items instead of 1 500 cash) is not playable in the
gym and may be the cheap route. **High confidence** on floors; the permit conclusion is
**medium** (bribe path untested).

## 6. Faction strategy: push one faction vs balanced

Setup on B. `nice_faction=X` always makes the "nice" bargain with faction X (offer 10 % discount
when selling, offer 10 % markup when buying — +0.65 trade-rep multiplier), aiming for the first
perk (LL 20 Referral: +1 customer/day; UL 20 Tips: +15 % on UL sales).

| Arm | n | Days | nw60 | Δdays vs none | Δnw60 vs none | Target rep at end |
|---|---|---|---|---|---|---|
| none | 60 | 67±3 | 807±176 | — | — | LL 15.3, UL 3.2 |
| Lower Levels | 60 | 52±2 | 54±34 | **−15±4** | **−753±186** | LL 15.8 |
| Upper Levels | 60 | 66±3 | 861±233 | −2±3 | +54±253 | UL 4.7 |
| Upper Levels 25 % | 60 | 65±2 | 691±227 | −2±3 | −116±242 | UL 4.9 |
| Revolution | 60 | 67±2 | 701±161 | −0±1 | −106±130 | Rev 8.3 |
| Black Market | 60 | 65±3 | 737±199 | −3±2 | −70±196 | BM 0.1 |
| all | 60 | 50±2 | 18±19 | **−17±3** | −789±176 | LL 13.9 |

Verdict: **do not buy reputation with price concessions.** Nice bargains cost 10 % of every
trade with the faction and did not even raise the faction's rep at the end (LL 15.8 vs 15.3 —
the shop dies before the gain compounds), so no perk was reached earlier. LL is the bulk of
traffic, so pushing it costs the most. Reputation in the gym comes from trade volume (and hot
buys, ×1.5–3), so the cheap way to the first perks is volume and hot goods, not discounts.
Fidelity: rep formulas are exact; most perks need ≥ 20 and the gym rarely gets there before
dying, so the payoff side (referral traffic, tips) is barely exercised. **Medium confidence**;
re-test once runs live past day 100.

## 7. Scavenging every night vs never

| Arm | n | Days | nw30 | nw60 | Δdays vs always | Δnw30 | Δnw60 |
|---|---|---|---|---|---|---|---|
| every night (shipped) | 60 | 67±3 | 1459±132 | 807±176 | — | — | — |
| never | 60 | 62±3 | 1122±138 | 541±183 | −5±5 | **−338±208** | −265±269 |
| stop at wound risk > 8 % | 60 | 67±3 | 1420±102 | 882±195 | −1±3 | −39±160 | +75±232 |
| G-never vs G | 60 | 73±3 | 2099±165 | 1961±293 | −3±3 | −293±216 | −394±401 |

Wound risk never reached 12 % per attempt in practice (`scav_risk=12` ≡ always). No run ended by
wounds. Verdict: **scavenge every night** (owner's practice): free stock worth ~+300 net worth by
day 30; a risk cutoff changes nothing measurable. Same direction on every start (below), large
on rock bottom and water merchant. Fidelity: loot tables are the real `level1` tables; tool-gated
extras and expeditions are not modelled. **Medium-high confidence.**

## 8. Wholesale and buy thresholds

Sellers in the gym ask exactly the estimate, so the regular-buy margin is a switch: any
`buy_margin` > 1.0 means "buy only through wholesale (15–35 % under estimate) or after a
successful demand-discount" (1.05, 1.1, 1.2 and 99 give identical runs).

| Arm | n | Days | Endings | nw60 | Δdays vs B | Δnw60 vs B |
|---|---|---|---|---|---|---|
| B (buy at estimate, wholesale when est ≥ price) | 60 | 67±3 | rent 50, arrest 10 | 807±176 | — | — |
| wholesale only above 1.2× | 60 | 60±3 | rent 58, arrest 2 | 285±94 | −8±4 | −522±201 |
| wholesale only above 1.4× | 60 | 46±2 | rent 60 | 7±9 | −21±3 | −800±176 |
| never wholesale | 60 | 42±1 | rent 60 | 0±0 | **−26±3** | −807±176 |
| **no regular buys (wholesale only)** | 60 | 76±2 | rent 17, arrest 43 | 1429±214 | **+8±3** | **+622±241** |
| G + no regular buys | 60 | 79±0 | rent 1, **arrest 59** | 1980±201 | +3±2 vs G | −375±339 vs G |

Verdict: **wholesale is the profit engine; buying at the estimate is net negative.** Accept every
wholesale offer whose estimate covers the price (the 15 % minimum discount is always enough);
stop paying full estimate for single lots unless a demand-discount lands. On G the regular-buy cut
moves the whole cohort onto the day-79 permit wall (59/60 arrests), i.e. it survives rent but
not the permit. Fidelity: 1×1 items make stock free to hold; real inventory pressure makes
low-margin buys worse still, so the direction should survive. **High confidence.**

## 9. Other owner strategies and odds and ends

| Arm | n | Days | nw60 | Δdays vs B | Δnw60 vs B | Note |
|---|---|---|---|---|---|---|
| speculation: hold event-depressed stock | 60 | 69±2 | 932±232 | +1±3 | +125±270 | neutral (G: −1±3, +79±351) |
| dialogue "smart" (decline loan/gift) | 60 | 62±2 | 554±174 | **−5±1** | −253±135 | Georgio's loan (+137 now, −548 later) keeps runs alive over a rent day |
| stage ≤ 3 items per buyer | 60 | 61±2 | 593±168 | −6±4 | −214±256 | fewer sales |
| stage ≤ 12 items per buyer | 60 | 68±2 | 695±168 | +1±3 | −112±234 | budget binds anyway |

Speculation as the owner plays it (buy ahead of a *forecast* shortage) needs foresight the gym
policy does not have: events are unknown ahead except through LL 60 rumors, information
dealers (dismissed by the gym) and faction operations; the market calendar the owner reads is
not exposed to the gym's policies. The testable half (hold depressed stock until the event
ends) is neutral. Georgio's loan is usurious (pay back 4×) yet worth taking when it bridges a rent
payment: a rent miss ends the run, the interest doesn't. **Low confidence** on speculation
(untested in its real form).

## 10. Start types

`NewGameData.StartType` swept with 11 policies × 40 seeds × 200 days.

Playability in the gym:

| Start | Status |
|---|---|
| none, apprentice, scav, water merchant, farmer, gunsmith, fence | run; production engines (hydroponics, gun printer, water purification) are inert or sold as stock |
| moonshiner, organ trader, pharmacist, street food vendor | **byte-identical to `none`** in 0.46D headless: no start-specific setup branch runs |
| drug dealer | ends day 7–8 `cartel_retired`: the cartel's weekly check-in demands `dream_dust` the gym cannot cook (chemistry needs per-second ticks); reporting the cartel (first choice) also ends it |
| rancher, aquafarmer | crash at setup (`AnimalMarkHelper.CreateMarkingPen` null; the known `marking_pen` factory failure) |
| rock bottom | 150 cash; every policy dies of rent on day 13–17 |

Mean days survived (and nw60) per start × policy (40 seeds each, all buyout 0 %):

| Start | default | B | B+ev | B+hold | B+conc | B+wholesale-only | B, no scav | B, no wholesale | G | G+wholesale-only | G+permit |
|---|---|---|---|---|---|---|---|---|---|---|---|
| none | 45 (8) | 68 (844) | 69 (1386) | 72 (1484) | 70 (1117) | 75 (1370) | 62 (547) | 41 (0) | 75 (2374) | **79** (1961) | 76 (2188) |
| apprentice | 50 (16) | 69 (831) | 69 (1397) | 74 (1590) | 70 (1406) | 78 (1740) | 68 (1018) | 45 (4) | 74 (2266) | **78** (2243) | 77 (2003) |
| scav | 48 (0) | 69 (781) | 72 (1433) | 72 (1492) | 68 (1035) | 76 (1582) | 65 (512) | 45 (0) | 75 (**2523**) | **79** (2149) | 79 (2274) |
| water merchant | 46 (0) | 65 (560) | 70 (1224) | 72 (1231) | 66 (852) | 74 (1061) | 61 (332) | 44 (0) | 74 (2305) | **78** (1872) | 75 (2135) |
| gunsmith | 49 (17) | 63 (731) | 67 (1179) | 70 (1330) | 68 (1211) | 75 (1536) | 62 (473) | 47 (0) | 73 (2316) | **79** (2138) | 74 (2174) |
| farmer | 46 (14) | 59 (227) | 66 (885) | 64 (901) | 59 (496) | 73 (1125) | 57 (309) | 40 (0) | 68 (1499) | **79** (1729) | 69 (1462) |
| fence | 46 (16) | 51 (170) | 54 (615) | 55 (438) | 52 (260) | 63 (800) | 52 (180) | 40 (0) | 59 (1203) | **65** (1548) | 61 (1039) |
| rock bottom | 15 | 16 | 17 | 16 | 16 | 15 | 13 | 14 | 17 | 17 | 17 |

(CIs: ±2–4 days, ±200–450 nw60; full lines in the run directory.)

What differs by start:
- The **ranking of knobs is the same on every playable start**: services ≫ wholesale ≫
  EV-bargaining ≈ hold-hot > concentrate > scavenging; G+wholesale-only maximises survival
  everywhere (it runs into the day-79 permit wall), G maximises net worth.
- **Fence** is the hardest playable start: it begins with Security −33 / Black Market +30 and hot
  stock; arrests come earlier (16/40 already on B) and every policy loses ~10–17 days against
  `none`. Wholesale-only helps most here (+12 days on B), since it stops buying more heat at
  full price. A fence start needs a heat plan (smuggler bay, selling to BM buyers who halve
  fencing crime) the gym does not have.
- **Farmer** and **water merchant** pay extra rent (+40/+50) and less starting cash for engines
  the gym cannot run, so they trail `none` on B (−9 and −3 days); with the full policy they catch
  up. Their real value is production — not measurable until per-second ticks land.
- **Gunsmith** starts with valuable stock (peak nw higher, nw30 ≈ 1 780 on B) but the same walls.
- **Scav** gains most from scavenging (it is its identity) and reaches the highest nw60 under G.
- **Rock bottom** needs an opening the gym can't play (sell everything day 1, scavenge, skip
  services); no knob moves it.

## What the strategist should believe

Ranked by confidence × size. "Gym" = trading-only play without upgrades or production.

1. **Keep insurance and janitorial on every night** (very high confidence). Largest effect
   measured: +21–22 days, +800 (base) to +2 100 (good policy) net worth at day 60. Either alone
   gives about half; together they compound.
2. **Wholesale is where the margin is; buying a single lot at its estimate loses money**
   (high). Never refuse a wholesale offer whose estimate covers the price (−26 days without
   wholesale); stop buying regular lots at full estimate (+8 days). Demand a discount instead.
3. **The permit is a hard deadline**: refusing it every week from day 38 accumulates
   `OPERATING_WITHOUT_LICENSE` crime → evidence 100 → arrest on day 79–80, every time
   (very high: 97.5 % of the 2 310 arrests in the study; the rest are contraband sales and the
   fence start). Plan 1 500 cash (or the bribe-donation route)
   before then, funded by something other than trading margin — the gym shows trading alone
   can't fund it without dying of rent instead.
4. **Trading alone cannot outrun rent** (high, gym-bounded). Weekly margin ≈ rent; rent compounds
   ×1.05/week; Landlord Grace is single-use. Every run without upgrades/production dies by day
   ~105 with 0 % buyout. The end game is upgrades (re-buy Landlord Grace, referrals, showcase,
   renovation) and production engines; Wild favor accrues from trading and should be spent.
5. **Bargain only when the odds make it EV-positive** (high). +800 nw60. Reflex markups lose:
   aggressive 25 % markups/demands cost −19 days and drive Lower Levels to −95 (the `mob` ending
   is at −100). The owner's "never bargain" leaves ~800 nw60 on the table but is far safer than
   aggressive play. Charge the rep cost of "bad" bargains in long runs.
6. **Buy hot goods cheap, sell them only at heat 0** (medium-high). +6 days, +665–980 nw60.
   Holding beats selling hot; avoiding stolen goods entirely is the worst of the three. Do not
   sell serious contraband (−8 days, arrests).
7. **Scavenge every night** (medium-high). ~+300 net worth by day 30, no wound endings observed;
   a wound-risk cutoff adds nothing.
8. **Never leave the window empty** (high); **concentrating it on one high-value type probably
   helps** (+300–400 nw60, not significant, may be a 1×1-footprint artifact).
9. **Don't hold idle cash beyond the rent reserve** (high). Fixed floors and stock caps all hurt;
   a 600 floor halves survival. Keep rent covered from 2 days out; otherwise keep cash working.
10. **Don't buy reputation with discounts** (medium). Nice bargains to a faction cost −15 days for
    LL and did not advance its rep; rep comes from volume and hot buys.
11. **Take Georgio's loan when it bridges rent** (medium): usurious, but declining it cost −5 days.
12. **Start choice** (medium): `none`, apprentice, scav and gunsmith play alike for a trader;
    fence is ~10–15 days harder (heat/security start); farmer and water merchant only pay off
    with production; rock bottom needs a bespoke opening.

## What the gym needs to test the rest

- **Upgrade buying** (Wild Network purchase path, favor first then cash): Landlord Grace re-buys,
  referrals, showcase, renovation, fixer, evidence removal — the end game and the only route past
  the rent wall. Highest priority for buyout experiments.
- **Per-second machine ticks**: production starts (farmer, water merchant, moonshiner chain,
  gunsmith printer, drug dealer's dream dust for the cartel), the owner's wine/berries/furnace
  economy, and "keep vs sell" of production inputs.
- **Real item footprints**: needed before trusting the window-concentration and hold-stock
  results (inventory pressure changes the cost of holding).
- **Permit bribe donations and the smuggler bay**: covering the non-dismissable permit official,
  and stashing hot goods out of inspection reach.
- **Forecast information**: market calendar, LL 60 rumors, information dealers (dismissed today) —
  required to test speculation the way the owner plays it.
- **Supplier phone orders, barter, appraisal, gunsmith customers** (all dismissed today).
- **Start setup for moonshiner/organ trader/pharmacist/street food** (identical to `none`
  headless) and the `marking_pen` factory (rancher, aquafarmer crash).

---

# v2 (faithful gym), 2026-11-09

The v1 study re-run on the gym after its fidelity pass (0c8d9fd): real item footprints, a per-second
play clock (machines tick), Wild's Network purchases, services as a human-role choice, the permit
as a buy-or-refuse choice, real loot tables and English text. v1 above is left as it was measured.

## Method

- Gym: `main` at 0c8d9fd plus the strategy knobs merged from the strategy lab (`Knobs.cs`,
  `KnobHumanRole.cs`, `GymHeuristic.cs`; `-human knobs` with no knobs is byte-identical to
  `-human default`, held by a gym test). In-process heuristic, `-start none`.
- **Reference arm `base` is the new default role**, not v1's shipped default: insurance and
  janitorial on while one night's cost is covered on top of the rent, Wild's baseline upgrades
  (LANDLORD_GRACE, SHOWCASE_I), permit bought when cash covers fee + rent due, scavenging nightly.
  v1's "B" (services on) is now the baseline; v1's "G" policy is
  `bargain=ev,max_heat=101,sell_hot=0,window=concentrate` on top of it.
- 40 paired seeds per arm, 200 days or a run-ending event, normal and hard: 36 arms × 2
  difficulties (`sim/lab/scripts/arms-v2.txt`) and 14 perk arms × 2 (`arms-perks.txt`), 4 000 runs.
  Δ = paired difference against `base` (or `none`), ± 95 % CI. Run with
  `make gym-arms ARMS=sim/lab/scripts/arms-v2.txt DIR=var/sim/exp/v2-hard ARGS="-seeds 40 -days 200 -- -difficulty hard"`.
- Knob changes since v1: `svc_weeks` is now `svc_nights` (it always counted nights of cost; default
  1, the default role's rule); `permit=buy|refuse` maps onto the role's permit rule (match by the
  localized reply line) and `permit_buffer` is gone; `services=default` is the new role's rule,
  `services=tablet` leaves the tablet alone (v1's default, ≡ `-human bare`); `scav_risk` is a
  `NightPlan` field. Upgrade policy is the gym flag `-upgrades none|baseline|growth|ID+ID`.

## Headline

1. **Still no buyout, on either difficulty.** 0 of 4 000 runs. Longest normal run: 125 days
   (G + wholesale-only + save for the permit from day 45, rent); longest hard run: 49 days (aug).
2. **New walls.** Normal: the permit wall is unchanged (refusing it from day 38 → arrest on day 80;
   every arrest in the grid is `OPERATING_WITHOUT_LICENSE` except 6 in the contraband arm and 1
   fencing arrest), and runs that do buy the permit die of rent between day 77 and 126. Hard: a
   **rent wall at day 32–39** for every policy: rent reaches ~970 by day 42 while the shop sells
   ~4 items a day; the permit wall is never reached; up to 11 of 40 runs per arm end in `aug` (aug
   presence) around day 40–49.
3. **The default role can never afford the permit** (normal: 0 of 40 base runs; the rule waits for
   fee + rent due in cash). `-permit refuse` gives the same outcome as `-permit buy` on all 80 `base` seeds. Only
   wholesale-only G gets there often (15/40); those runs are never arrested and die of rent
   instead, between day 84 and 119.
4. **What held up from v1**: services (now built into the role: switching them off costs −21 days),
   wholesale (never wholesale −24 days), holding stolen goods until clean (+4 days, +470 nw60),
   no aggressive bargaining (−13 days), no serious contraband (−14 days), G as a whole, and
   wholesale-only as the survival maximiser (G + wholesale-only: +13 days).
5. **What changed**: the window-concentration gain is gone (it was the 1×1 artifact);
   EV bargaining's gain shrank to noise (+175±400 nw60); scavenging no longer measurably matters;
   on hard almost nothing a trader can choose moves survival more than ±4 days.

## Results, normal (Δ vs `base`: 72±3 days, rent 24 / arrest 16, nw60 961±264)

| Area | Arm | Δdays | Δnw30 | Δnw60 | v1 finding | v2 |
|---|---|---|---|---|---|---|
| Services | none (≡ tablet) | **−21±3** | −985±199 | **−948±265** | both: +22 days | holds |
| | insurance only | −9±4 | −520±177 | −763±262 | ins > jan | holds |
| | janitorial only | −13±4 | −728±236 | −845±264 | | |
| | both, 7-night buffer | −1±2 | −82±176 | −2±333 | buffer irrelevant | holds |
| Bargaining | never | −1±4 | −164±220 | −251±273 | ≈ discount | holds |
| | EV-positive | −2±5 | +216±243 | +175±400 | +5 days, +800 nw60 | **weakened to noise** |
| | aggressive 10 % | **−13±5** | −228±222 | **−715±289** | −8 days | holds, stronger |
| Stolen goods | avoid all | −0±3 | −98±144 | −222±218 | | |
| | buy any, sell hot | −1±3 | +322±247 | +162±382 | +3 days | weaker |
| | **buy any, hold until clean** | **+4±3** | +266±161 | **+468±282** | +6 days, +665 | holds |
| | buy serious contraband | **−14±5** | +240±230 | −224±400 | −8 days | holds, stronger (28/40 arrests) |
| Window | concentrate | +0±4 | −105±229 | −176±340 | +300–400 nw60 (CI touched 0) | **gone: 1×1 artifact** |
| | concentrate ≤ 8 | +1±3 | −22±223 | +46±358 | | |
| | top value | −1±4 | −81±233 | −306±319 | | |
| | empty | −4±4 | −274±219 | −485±310 | −6 days, −500 | same direction, weaker |
| Wholesale | never | **−24±3** | −1275±187 | −960±264 | −26 days | holds |
| | wholesale only (no buys at estimate) | **+6±3** | +103±190 | +402±323 | +8 days | holds |
| Upgrades | none | −4±4 | −90±199 | −102±329 | (not testable) | baseline ≥ none, n.s. |
| | LANDLORD_GRACE only | +1±4 | −90±199 | −85±330 | | ≡ baseline |
| | growth list | **−18±4** | −1064±196 | **−892±262** | | spend doesn't pay back |
| Permit | refuse | 0 | 0 | 0 | | ≡ buy: never affordable |
| | save from day 30 / 45 | −3±1 / +0±1 | −21 / 0 | −691±251 / −463±167 | −4 / −1 days | holds; still 0/40 permits |
| Scavenging | never | −1±4 | −228±225 | −119±355 | −5 days, −340 nw30 | **weakened to noise** |
| Combined | **G** | +4±4 | **+648±231** | **+722±345** | best nw | holds |
| | G, upgrades none | +1±4 | +628±258 | +786±399 | | ≈ G |
| | G, growth upgrades | −5±5 | −610±199 | −501±310 | | growth hurts |
| | G, permit refuse | +2±4 | +648±231 | +804±369 | | ≈ G |
| | G, save permit from 45 | +2±4 | +648±231 | +32±278 | | saving costs nw |
| | **G + wholesale only** | **+13±4** | +648±232 | +639±339 | best survival | holds (85±4 days) |
| | G + wholesale only + save 45 | +10±4 | +648±232 | −129±323 | | |
| | G, avoid stolen | −1±5 | +359±269 | +294±381 | | |
| | G, types window | +1±4 | +502±277 | +655±404 | | ≈ G: concentration adds nothing |

Permit ownership by the end: base 0/40, G 4/40, G + wholesale-only 15/40 (+ save 45: 9/40).
Every permit holder ended in `rent` (days 77–126); every non-holder that lived to day 80 was
arrested.

## Results, hard (Δ vs `base`: 36±1 days, rent 38 / aug 2)

Every arm dies by day ~40; nw60 is 0 everywhere, so only days and nw30 are informative.

| Arm | Δdays | Δnw30 |
|---|---|---|
| services none / ins / jan / both-7 | −4±2 / +0±1 / −3±2 / −0±1 | −113 / −104 / −110 / −23 (all ±130) |
| bargaining never / EV / aggr 10 | −1±1 / −0±2 / −2±2 | n.s. |
| stolen avoid / hot / hold / contraband | +0±1 / +1±1 / +2±2 / +1±2 | n.s. |
| window conc / ≤8 / top / empty | +0±2 / −0±2 / +2±2 / +3±2 | n.s. |
| **never wholesale** | **−10±2** | −153±130 |
| wholesale only | **−5±2** (reverses vs normal) | −131±129 |
| upgrades none / grace only / growth | **−4±2** / **+3±1** / −0±1 | n.s. |
| scavenging never | −3±2 | −112±124 |
| G / G no upgrades / G + wholesale only | +2±2 / +2±2 / −2±3 | +67±113 / **+328±173** / +3±164 |

On hard the trader is rent-bound from the first weeks. Wild's grace helps (no upgrades −4 days);
the showcase in the baseline list costs (grace only +3 days); wholesale is the other lever above
noise, and wholesale-only now hurts.

## Starting perks (`-perks`, docs/re/PERKS.md)

Default role, Δ vs no perks (normal: 72±3 days; hard: 36±1).

| Perks | Δdays normal | Δnw60 normal | Δdays hard |
|---|---|---|---|
| saving | +2±4 | −10±285 | +1±2 |
| prime_real_estate | **−9±4** | −418±388 | −2±2 |
| side_street | **−7±3** | **−539±292** | −0±2 |
| short_sleeper | 0 (identical) | 0 | 0 |
| wild_favorite | +2±3 | +133±325 | +1±2 |
| vending | +2±2 | +46±193 | +0±1 |
| working_pipes | 0 (identical) | 0 | 0 |
| proficient_scavenger | +1±4 | −5±373 | +1±2 |
| awful_negotiator | **−13±3** | **−876±258** | −3±2 |
| bad_lease | **−11±3** | **−751±255** | −2±2 |
| owner's loadout (proficient_scavenger, alert, short_sleeper, bad_lease) | −4±3 | −273±309 | +3±2 |
| honest broker (diplomatic, prime_real_estate, handy) | −6±4 | −364±310 | −2±2 |
| magnate (vending, wild_favorite, bad_lease) | −7±4 | −488±313 | −0±1 |

The perks that cost a trading shop are the rent and price ones (Bad Lease, Awful Negotiator, Prime
Real Estate's rent, Side Street's traffic); the engine and convenience perks (Vending, Working
Pipes, Short Sleeper) are worth nothing to a policy that never runs an engine or uses the
time, and two of them (Short Sleeper, Working Pipes) leave the run's outcome unchanged on every seed. So in the gym today perks mostly price the
cash/rent and traffic clusters; the rule-changing perks need production and night policies first.

## Stolen goods, with real footprints

The v1 caveat still applies and is now sharper: held stolen goods sit in the main stock (the human
role cannot stage them into a smuggler bay), so at an inspection each is found with chance = its
heat. Inspections stay rare because the trafficking record stays low when nothing hot is sold
(1 fencing arrest in 40 "sell hot" runs). Real footprints add that hiding space is small: the
poster stash is 2×2 cells (c4 is 9 and does not fit; docs/sim/VERIFIED.md). Treat "hold until
clean" as right while the record is low; cap hot stock to bay capacity as it rises.

## Warm start: the owner's day-108 hard save vs the live logs

`-warm` (sim/gym/README.md) starts a hard, rock-bottom run with the save's day 108, cash 6 180, rent
1 347 due in 4 days, reputation (LL 88.3, UL 36.5, SEC 4.4, BM −1.5, REV −6.7), permit owned, base
attractiveness 700, its 19 owned Wild upgrades (renovations I–III, showcases I–II, winemaker,
brewer, water trader, …) and its perks (proficient scavenger, alert, short sleeper, bad lease).
Not carried over: the save's stock, machines and production. 40 seeds.

| Day 108 (first day) | Live (owner + LLM advice, `var/log`) | Gym, default role | Gym, G | Gym, G + wholesale only |
|---|---|---|---|---|
| Covered customers | 12 | 11 | 9.4 | 9.4 |
| Items sold | 32 | 17±1 | 13±1 (2-day mean) | 12±1 (2-day mean) |
| Items bought | 15 | 27±2 | 32±2 (2-day mean) | 13±2 (2-day mean) |
| Cash at the end of the day | **7 431 (+1 251)** | **3 529±143 (−2 651)** | 3 783±172 | **6 591±133 (+411)** |
| LL / UL rep after 2 days | 89.0 / 39.9 (after day 1) | 95 / 36 | 83 / 34 | 82 / 34 |

- **Traffic matches** (11–12 covered customers on day 108), so attractiveness, rep and upgrades carry
  over as intended. Day 109 live had 6 covered customers against the gym's 11–12 (one sample).
- **Buying is the gap**: the default heuristic buys at the estimate and spends 2 650 of stock on day
  one; the owner (and the LLM advice) sold twice the items and bought half. Wholesale-only gets the
  gym within a few hundred credits of cash-positive; the owner's extra margin is the sales side (32
  items against 12–17; the save's own stock and food during a +50 % food event, which the gym's
  stock does not have).
- **UL vs LL drift**: the owner gained UL (+3.4 in a day, selling food to UL chefs); the gym gains
  LL (+5 in 2 days, mostly hot and wholesale buys from LL sellers) and nothing on UL.
- **From day 108 to day 200** the gym's policies all die of rent: default after 34±2 days, G 37±2,
  G + wholesale only 59±1, G without upgrades 24±2 (rent 1 347 and rising every week). Default
  and G die holding 5 000–6 400 of net worth in stock: the role never sells down stock to make rent.
  The owner's run is alive past day 109; what keeps it alive (production, curated stock, selling
  into events) is what the gym's policies lack.

## What changes for the strategist

- Unchanged: services on; accept wholesale; hold stolen goods until clean; never bargain on
  reflex; no serious contraband; plan the permit as a hard day-80 deadline.
- Drop: "concentrate the window on one type" (1×1 artifact).
- Downgrade: EV bargaining (direction only) and nightly scavenging (no measurable effect now).
- New: on hard, a trading-only shop dies of rent by day ~40 whatever it chooses; grace re-buys
  help, the showcase does not. Late-game, the money is lost on the buy side: buy below estimate (wholesale, demanded
  discounts) and liquidate stock before rent rather than die holding it.
- Perks: avoid Bad Lease and Awful Negotiator unless the plan uses their refunded point for
  something the shop will run; engine perks pay only with production.
