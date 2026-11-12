# Stolen goods: the Fence start's trade (RE notes)

Build 26525552 (0.46D). Question answered here: where stolen goods come from, what they cost, how they cool,
who may buy them and at what price, what the police do about them, and what the Black Market gives back, so
the Fence start's native trade (buy stolen cheap, let it cool, sell it clean) can be played well.

Tags: **MEASURED** = checked headless (`sim/lab` `stolen.*` and earlier `barter.*`/`state.*`/`conceal.*`
experiments, verdicts in `docs/sim/VERIFIED.md`; or gym runs, named); **CODE** = read from the decompile only.
Related: `CONCEALMENT.md` (where hot goods hide, who finds them), `MITIGATIONS.md` §3 (crimes, evidence,
inspections), `PERKS.md` (faction perks), `BARTER.md` (bargains, blackmail), `WINDOW.md` (the display case).

## 1. The Fence start

| | Normal | Hard | Evidence |
|---|---|---|---|
| Security | −30 | **−37.5** (−30 × the 1.25 loss multiplier): Watchlist (−20, daily evidence ×1.2) from day 1, 2.5 short of Must Be The Wind (−40: the alarm button stops reporting) | MEASURED `stolen.start` |
| Black Market | +30 | **+22.5** (+30 × 0.75): 2.5 short of the BM client bonus at 25 (§3) | MEASURED |
| Cash, rent | 450 (150 less than the apprentice), rent 250 (+50) | same | MEASURED |
| Kit | a shabby **Smuggler's Bay** (5×5 inside), 2 dream dust, a heavy handmade pistol (Fair) and 2 boxes of its ammo, the med/eng/sec keycards, 2 shabby black lamps | same | CODE `NewGameData`, MEASURED |
| Exchange | the BM Underground Exchange is open from day 1; the janitor's exchange introduction comes on day 1 | same | CODE |

The kit is contraband: keycards (mid), dream dust and the pistol/ammo. None of it is a stolen item. Nothing in
the start changes how often thieves come, the discount, the heat or the police: the Fence start differs from
the others only by the bay, the contraband kit, the exchange and the two rep offsets.

## 2. What a stolen item is

| Rule | Value | Evidence |
|---|---|---|
| Created | only on a seller's counter, by a client's offer flagged stolen (`TryAddDirectSellingItemToTable(…, isStolen)`), once `isStolenItemIntroduced` (day 13 with the intro, day 1 skipping it) | CODE |
| Initial heat by base value | ≤ 17: not stolen at all; < 50 → 25; < 85 → 50; < 140 → 75; else 100 (some offers force 100: loot crates) | MEASURED |
| Cooling | −18 every night for every stolen item, contraband or not (the contraband slow-down is dead code); clean (tag removed) at 0. Heat 100 is clean after 6 nights, 75 after 5, 50 after 3, 25 after 2 | MEASURED |
| Nothing else cools it | no bay, stash, machine or service lowers heat; display never lowers it; any security find sets it to **100** | CODE, MEASURED |
| Visible | heat in the tooltip ("stolen, N"); the counter discount as a feature ("five finger I–IV") | CODE `StolenHelper` |

### The five-finger discount (buying)

While a seller's stolen item is on the counter it carries a buying feature, by heat: ≤ 25 **−10 %**, ≤ 50
**−15 %**, ≤ 75 **−20 %**, above **−25 %**. The shown estimate and the asking price both include it; the
moment it is bought the feature goes and the item's estimate is its clean one again (MEASURED
`stolen.discount`: a 200 surgery tool shows 180/170/160/150 at heat 25/50/75/100 and reads 200 once bought).
So **a stolen item's estimate on the seller's counter understates its shelf value by 11–33 %**: a rule that
buys at "x % of the estimate" demands a much deeper cut on stolen goods than on clean ones.

Buying is never a crime. It raises the seller's faction rep ×1.5/2/2.5/3 by heat (MEASURED).

### Selling

| Rule | Value | Evidence |
|---|---|---|
| Price | no sell-side penalty: a stolen item sells at the same price as a clean one | CODE (`ApplyStolenDiscount` is called only for sellers' items) |
| Crime | `FENCING` = sale price × heat / 100, ÷2 to a Black Market buyer, ÷3 Revolution, ÷2 Lower Levels with Tight Lips (LL 80), 0 Cartel; nothing at heat 0 | MEASURED (`barter.crime`) |
| Vending | the vending machine accepts stolen goods and sells them overnight at the retail markup; FENCING at full rate (no buyer faction) | CODE `VendingMachineHelper.HandleNight` |
| Security buyers | an officer who sees a stolen item on the counter or in the case rolls its heat %, sets it to 100 and searches the shop | MEASURED (`CONCEALMENT.md`) |

FENCING counts as trafficking. Crimes fall **20 a night per type** while evidence < 80
(`SecData.OnNewDay`); the scheduled-inspection countdown runs only while trafficking ≥ 50 (MEASURED). A shop
that sells nothing warm writes no FENCING at all, so stolen stock on its own never brings a scheduled inspector.

## 3. Who brings stolen goods

All sellers below are on the counter as ordinary SELL clients; the stolen flag shows on each item.

| Source | Faction | When | Lot | Evidence |
|---|---|---|---|---|
| Thief (`thief`) | BM | BM pool weight 50; every `day % 7 == 1` with Safe Hands (BM 40); day 13 with the intro | ~13 independent rolls ×(1 / 1.25 / 1.5 / 1.75 / 2.5 at BM < 40 / 40 / 60 / 80 / 100): electronics, medical, luxury, water filters, a genuine injector, **and contraband** (keycards, drugs); with Safe Hands a stolen loot crate (evidence, medical, engineering…) | CODE |
| Petty thief (`pettyThief`) | BM | BM pool 75/50/25; 20 % a day with Safe Hands | small household/food lots, wholesale −10 %, often a contraband item | CODE |
| Food thief (`foodThief`) | BM | BM pool 75/50/25 | food, wholesale −15…−20 % | CODE |
| Shady pharmacist (`shadyPharma`) | Upper Levels | the Lower Levels pool, weight 50 | stolen medical (injectors, meds), wholesale +15 | CODE; the outlaw's biggest stolen source (gym, §6) |
| Thief suppliers (`thiefGeneric*Seller`) | BM | a supply sign in the display case (75 %) picks thief over scavenger with odds `BM : max(0, LL + 25)` | the sign's type, stolen, BM-rep-scaled | CODE `ShowcaseHelper.Get*SupplierId` |
| Clinic nurse (`nurseMinor`) | — | minor client | 2 pink injectors, 4 birth-control pills, common medical, stolen once stolen goods are introduced | CODE |
| Revolution raider | REV | REV 80 (Raid Loot), `day % 7 == 5` | 25 stolen items | CODE |
| Georgio (`georgio3`), wanted 7, wanted 2's return, the evidence seller | story | fixed days | small stolen lots | CODE |

How often the BM pool fires: base chance 10, −3 below BM 0, −3 below 25, +4 at 25, +3 more at 50 (so 4 / 7 /
14 / 17 per draw at BM < 0 / < 25 / ≥ 25 / ≥ 50). On hard the Fence start sits at 22.5: **2.5 rep short of
doubling its BM customers**. Gym measurement of the rate by rep: §6.

## 4. Black Market reputation and what it buys

| BM | Perk | Effect for the trade | Evidence |
|---|---|---|---|
| always | Honor Amongst Thieves | every BM loss ×2 (on hard ×2 × 1.25) | CODE, `PERKS.md` |
| 20 | Small Time Broker | a label; the real effects are the continuous ones: BM client chance (§3), thief stock ×1.25/1.5/1.75/2.5 at 40/60/80/100 | CODE |
| 25 | — | BM client chance 7 → 14 | CODE |
| 40 | Safe Hands (`BM_RELIABLE`) | a thief every `day % 7 == 1`, a petty thief on 20 % of days, loot crates in the thief's lot | CODE |
| 60 | Underground Network | BM operations shown on the calendar 4 days ahead: gang war (medical +50 % for 3 days, needs BM power ≥ 85), illegal weapon shipment (weapons and ammo −35 % for 2 days) | CODE `StoreOperationManager`, `OperationListBM` |
| −40 / −60 | Blacklisted / Uninsurable | BM sellers stop offering contraband, exchange closed, janitor ×2 / no insurance | CODE |

How BM rep moves: trades with BM clients (value × multiplier / 400, × 0.75 on hard; buying stolen ×1.5–3),
contraband sales to BM buyers (× contraband level). A bargain walk-out is −3 × 2 (Honor) × 1.25 = **−7.5** on
hard; a failed blackmail −5 × 2.5 = −12.5, and a successful one turns the trade's rep multiplier to −6. At
~0.5 rep per 100 credits of heat-100 goods bought, BM 22.5 → 40 is ~3 500 credits of stolen purchases.

**Reporting a thief** (not wanted) pays 50 + 4 % of the stolen goods' price (58 for a 200-credit lot), gives
Security **nothing** (it adds the highest contraband level on the counter: 0 for plain stolen goods), and
costs the thief's faction its escalating report penalty: −8, −10, −12 … (floor −20) before multipliers, i.e.
**−20, −25, −30 BM** on hard for the first three reports (MEASURED `stolen.report`). A fence never reports.

There is no buyback mechanic: no client or service buys back what a fence sold (no code path).

## 5. The police

| Check | What it does to stolen goods | Evidence |
|---|---|---|
| Scheduled inspector | countdown only while trafficking (incl. FENCING) ≥ 50; finds each exposed stolen item at heat % (then heat 100), seizes the found set, −(1 + tier) Security each (tier 1–4 by base value); bays and the closed poster stash are never searched | MEASURED (`CONCEALMENT.md`) |
| Crackdown (Security operation, 2 days; queued when Security power ≥ 20 and BM power ≥ 40, shown only at Security ≥ 60) | **one normal inspector each day it is active**, unannounced: the event's client action is wired (`InitNormalEvent` → `crackdownaddClientAction` → `CreateInspectionClient`) | MEASURED `stolen.crackdown` (30/30 crackdown days queued an inspector, 0/30 control days) |
| Security visitors | top-level counter/showcase check at heat % | MEASURED |
| Showcase | a stolen item in the case summons an inspector at the opening (heat % roll) | MEASURED |
| Evidence | daily evidence once total crime ≥ 100 (2/6/9/12/20/33 by crime band), ×1.2 on the Watchlist, ×1.5 Most Wanted | CODE `SecData.OnNewDay` |

The crackdown corrects `CONCEALMENT.md` §4 ("whether it is ever wired was not checked"): it is wired, and for
a Fence start (Security < 60) it is an inspector with no warning even when trafficking is 0. Bays are the only
defence.

## 6. Playing it (gym measurements)

Hard, the Fence start, vending + short sleeper, seeds 2910001–060, 1000-day horizon; `player.stolen` in each run
summary (sim/gym/Episode.Stolen.cs follows every item bought while stolen until it is sold, vended or lost).

**How often thieves come, by BM rep** (every BM seller the executor met, binned by that morning's BM rep; 22 900
run-days of `outlaw` and the native screens): below 25, ~1.0 BM seller a week; 25–40, 1.4; **at 40 and above, 3.8**
(Safe Hands' weekly thief and 20 % petty thief). The 25 step is small in practice; the 40 step nearly triples supply.

**What `outlaw` does with stolen goods** (the same executor plays `fence_fortress`, which only adds selling at any
heat):

| | `outlaw` | the native `fence` route (`stolen` engine) |
|---|---|---|
| stolen bought a run (items / paid) | 32 / 859 | 47 / 2 458 |
| from thieves (paid) | 17 | 1 466 |
| sold (items / for / cost) | 28 / 1 310 / 738 | 41 / 2 827 / 2 043 |
| margin (sold + vended − cost) | 572 | 785 |
| share of the run's income | 1.3 % | 1.9 % |
| lost (seized, burgled, wiped) at cost | 70 | 361 |
| stolen offers declined (est) | 10 217 | 10 473 |
| BM rep at the end; runs reaching 40 | 27; 0/60 | 44; 36/60 (median day 77) |
| Security at the end | 12 | 7 |

Why `outlaw` buys so little: (1) it judges a seller's stolen goods by the estimate the counter shows, which
already carries the five-finger discount, so its 75 % limit asks for ~56 % of the clean value at heat 100: thieves'
lots are "asking" too much; (2) a thief's lot often holds a keycard or drug, and the executor sent the whole seller
away ("contraband") instead of buying the rest; (3) it demands discounts from BM sellers, whose walk-outs cost BM rep
twice over. It never sells warm on purpose; the few warm sales it makes (3 a run) are goods left on the back counter
when stock is full, sold to the next buyer (a gym executor gap, not a rule).

What it still leaves: most of the declined value is the Butcher (`wanted2_return`, a wanted-poster criminal: ~80
stolen cuts of meat a run, ~8 300 clean for ~6 500, wholesale −20 %) and `wanted7`; the executor treats wanted
sellers as bounties. Whether a fence should buy the Butcher's meat instead of reporting him (450 bounty) was not
measured.

**The trade is thin.** Even tripled, the stolen margin is ~2 % of a winning run's money, the Board ~63 %. The money a
stolen lot ties up for the nights it cools competes with the buyout savings, so buying more does not win sooner:
`fence` against `outlaw` −1.4 ± 8.3 days (paired, n.s.). Blackmailing thieves (×0.8 on their goods) costs the BM rep that
brings them: in a 20-seed screen BM ended at −9 instead of 46 and the stolen trade halved. Selling warm goods under the
crime allowance (trafficking + the sale's FENCING ≤ 45; crimes fall 20 a type a night) added 30 warm sales and twice
the FENCING, and no speed. See `docs/research/ROUTES.md`, "The Fence".
