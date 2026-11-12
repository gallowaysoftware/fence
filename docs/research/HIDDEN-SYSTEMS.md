# Hidden systems: what the game doesn't tell you, and what the executor leaves on the table

The owner's ask: the in-game guide is vague, the game has far more depth, and the player should get the best
result its equipment and situation allow. This is an audit of every system outside water, hydroponics, wine,
furnace quality and chemistry (the quality agent's), and outside fish/RAS, rats and the butcher, the 3D printer,
the desequencer, the bottle printer and the fridge (the livestock-chains agent's). Sources:

- the decompiled game (build 26525552 / 0.46D, local only, never committed), as the primary source;
- `docs/re`, `docs/sim/VERIFIED.md` and `docs/research`;
- the executor (`sim/gym`, compiled into the live mod);
- four new checks (below, "Verified in this audit").

"decomp-only" marks a fact that no `docs/re` file had before this audit.

**Reading the values.** On hard the reference route (`appraiser`) already wins 60/60, buying out at a median of
day 98. The Appraisal Board pays ~49k a run, 55–71 % of all money (ROUTES.md). So a gap's value is given as
days off the buyout, credits a week or a run, or stream interest. "Visible?" asks whether exploiting the gap
needs only what a player can see or measure (the executor may know rules, never per-item truth).

## Ranked gaps (top 10)

| # | Gap | System | Value | Cost | Visible? | Status |
|---|---|---|---|---|---|---|
| 1 | **The fake-exposure guard reads hidden truth.** `PlayerRole.HidesFake`, `KeepWhy` "exposable" and "water_label", `PlayerCounter.Exposable`, `BribePicks` and `Labels.ForBuyer` call `CanClientExposeAnyFeatureDeep` / `IsFeatureMatch` / true water purity. Replace with a "known truth" map built from the player's own reads (magnifier, scanner, strips) plus the per-faction exposure table | labeler, barter | Fidelity: it breaks the stream's "the model plays with a player's information" rule. The guard is load-bearing: 54 → 58/60 normal wins, and it keeps hard Lower Levels rep off the mob ending | M | becomes yes | code read, confirmed |
| 2 | **Report wanted posters.** Every wanted visit is dismissed: 26–59 visits a run (mean 46), never a report | clients | ~2.1k cash a run in bounties (excluding Lou), +3.75 Security each, ~46 slots a run freed for buyers. Costs ~−25 LL rep on hard. Reporting Lou (`wanted5`, 1 500) starts a chain that can end the run: handle it or skip her | S–M (+M for Lou) | yes (poster matching) | **lab + gym verified** |
| 3 | **Block hands and ribwiches on the Appraisal Board** (Senior/Master): the type is drawn uniformly from the unblocked pool | appraisal | Plasma pays 192 a job vs 160/161 (probe logs): ~+12 %, ~+290 a week from Master (day ~41), ~+2.3k on a day-98 run | S (a directive that sets `appraisalBlacklist`) | yes | measured fees; arm not run |
| 4 | **Informant tips create the event they foretell**; the executor dismisses the informant, and its `RisingTypes` buys into events already running (zero margin on hard, −⅓ when a +50 event ends) | market events | +40–110 a week (~53 % of tips are +50 events), plus avoided losses | S (future-only `RisingTypes`) + M (accept the informant, stock ahead) | yes | read, not measured |
| 5 | **Alert perk: the executor still buys insurance.** With Alert, `HandleInsurance` returns before the burglary roll, so insurance buys nothing | perks | ~+2k NW a run for any Alert loadout (insurance days 4–55, and the uninsured nights after) | S (one check in `PlayerRole.Services`) | yes | code read, confirmed. ROUTES.md's Alert screen was measured with insurance on |
| 6 | **Skip the last rent before the buyout.** `BuyOutStore` charges only the price: this week's rent and a Grace-deferred backlog are never collected | debt | One rent (~700 on hard); ~−2 days median | S (an `ExecutorMorning` rule) | yes | read, not measured |
| 7 | **The aug arc ends around day 62.** Two scripted kills take presence to 0 by day 19. Once the permit has been held 25 days, the all-clear stops augs for good | augs | ~+450–1 100 cash and +20–40 Security a run if kept alive (let one walk per two). **High stream interest:** the best set piece vanishes mid-run | S (an `AugRule` option) | yes (presence is reconstructible from the night log) | **gym verified** |
| 8 | **Liquid luck: +75 luck while active.** One luck roll voids an inspection's whole find. Georgio gives a bottle and sells three; the executor sells them as stock | inspection, mitigations | Near-immunity for 3–6 inspections; survival on contraband or arrest-prone policies. Small on the clean reference route | S | yes | **lab verified** |
| 9 | **Scavenging with kit.** The executor leaves through the no-backpack choice, so carried tools, the scanner and the flashlight never count; it stops at the first wound when a bandage would reopen the dump | scavenging | Tools ~+200 a week; flashlight halves injury nights; bandage-and-continue ~+60 a week. Scavenging is noise on hard overall | M (backpack) / S (bandage) | yes | read |
| 10 | **Marketplace requests and UL ≥ 20.** Requests pay 1.5–2× value plus rep; the reference run ends at UL ≈ 19, one point short of Tips (+15 % on UL sales) | reputation | +110–260 a week | M | yes | read |

Below the top ten, in rough order:

- **Alarm bay kit on `watchman`.** 17/60 runs fall short of P50 when the transmitter takes the room. ~+1–3k NW by day 150, cost S.
- **Exchange barters on janitor nights.** +100–300 a week when the promos line up, cost M/L.
- **Gun orders, repair only to cross a threshold.** Gunsmith start only, cost L: the tablet flow is stubbed headless.
- **Relabel free gifts.** 60–250 a run, cost S.
- **Sell then shoot an aug.** A hypothesis worth 60–80 an aug; lab test S.
- **Commissary and stamps** (low), **Serpico** (+12 rep, stream), **the Butcher gun gate** (parity), **vending during events** (small), **a tier-3 window packer** (uncertain).

**Refuted by this audit: lifting the upgrade cash cap for renovations.** A sub-audit expected attractiveness
700 by day ~55 to lift budgets and bring the buyout ~7 days earlier. In the gym it makes the buyout later (below).

## Verified in this audit

| Check | Method | Result |
|---|---|---|
| The aug arc ends mid-run (gap 7) | `appraiser` route, hard, seeds 2910001–009, one trajectory each (`is_aug` clients, `hidden.aug_presence`) | 9/9: presence 15 → 0 on day 19 and stays 0. The last aug comes on days 57–62 in 8 runs and 76 in one; the runs end (buyout) on days 70–119. Augs come every 6–8 days, so ~5–8 are lost per run. Code: the all-clear is tested before the spawn-day test, on any day once due (`StoreClientManager.HandleAugClient`) |
| Liquid luck voids finds (gap 8) | `fence-lab conceal.odds` (new case): one mid contraband item in stock, base luck 0, `liquidLuckDaysLeft` 3, n 4 000 | 74.1 % voided (99 % CI 72.3–75.8; expected 75), and Security changed by 0 on all 2 963 voided finds: no clean +3 |
| Wanted bounties and report effects (gap 2) | `fence-lab barter.wanted` (new): every `wanted*` factory's terms; seat, report, run the alarm's action, on hard. Gym `client_visits` over the 9 runs above | Bounties: 300, 450, 100, 300, 1 500, 400, 125, 125, 300. `wanted8_weekly`: +125 cash, **+3.75** Security, **−7.5** LL; `wanted9`: +300 / +3.75 / 0. `wanted2`'s report *improves* LL. `wanted1` and `wanted3` are not reportable headless (storyline intercept). Visits: 46 a run, all dismissed |
| Renovations bought early (refuted) | `appraiser` vs the same with `upgrades_cash_max_days: [60, 1000]` (no cash cap before day 60), hard, 30 paired seeds 2910001–030 | Both arms 30/30 buyouts. RENOVATION2 is bought on day 45 under the cap (not "rarely") and day 36 without it; RENOVATION3 on day 46 without the cap, never with it. The buyout is **later** by 5.8 ± 2.4 days (paired mean ± SE; 10 seeds identical): 1 500 spent on day ~46 earns back less than it delays the deed |

The lab additions are in `sim/lab/ConcealExps.cs` (`Odds`) and `sim/lab/BarterExps.cs` (`Wanted`). The
VERIFIED.md rows and the `docs/re` corrections are in this commit.

## Per system

Each section has the same parts:

- **Guide**: what the game tells a player.
- **Real**: the mechanics, with sources.
- **Unknown**: what is not known or not verified.
- **Executor**: what our code does today.
- **Best play**: what an informed player with the same kit would do.
- **Gap**: the difference, as in the ranked table.

### Appraisal and the Appraisal Board

- **Guide:** join, inspect the customer's item against the guide, enter an estimate, earn a fee (more when accurate), get promoted to new item types.
- **Real:**
  - **Fee.** A base plus a percent of *your estimate*, so the top of the perfect band (105 %) pays most (APPRAISAL.md "The fee").
  - **Refusals.** A refusal tells you too high or too low; a grade the rank refuses is refused in the direction of the error (decomp-only, `AppraisalServiceHelper.OnEstimate`).
  - **Impatience** is per client across all its items: the third refusal on any mix of them walks the client out (decomp-only).
  - **Visits.** The visit chance is 33/42/48/55 % by rank × (2 + days missed)/2. It needs a free normal-customer slot: with 0 slots no appraiser comes, but the missed-day count still rises (`HandleNormalClient`).
  - **Item type** is drawn uniformly from the unblocked pool. From Senior the older types can be blocked; at Master, any (`AppraisalItemRegistry.CanBlock`/`GetPool`).
  - **Fees by type at Master,** from the gym's probe logs: hand 160 (n 159), ribwich 161 (n 145), plasma 192 (n 198; true value 90–1 200).
- **Unknown:** no lab experiment covers APPRAISAL.md (the gym runs the real `OnEstimate`). The type draw uses Unity's random generator, not the game's seeded one.
- **Executor:**
  - Joins, accepts promotions and keeps the certificate first in the case (`Appraisal.Dialogue`, `PlayerRole.Window`).
  - Estimates at 105 % of the guide's value (`Appraisal.Estimate`): 19 023 estimates, all perfect, Master on day 41.
  - Caps retries per item, not per client, and never blocks a type.
- **Best play:** block hands and ribwiches once allowed. Keep a normal slot free (attractiveness ≥ 200; don't let signs fill every slot).
- **Gap:** #3. Also a per-client refusal cap (protects live play if a cue is misread; S).

### Barter and haggling

- **Guide:** one discount or markup per side, shown as accept/refuse/walk odds; blackmail for criminals, threaten for augs.
- **Real:**
  - **Formulas** as BARTER.md, all CONFIRMED. On hard every rep change is scaled: gains × 0.75, losses × 1.25 (`StoreReputation.ModReputation`; measured −7.5 for a base −6 in this audit).
  - **Coercion stacks.** After a blackmail, Demand Discount and Add Markup are still allowed and compound as separate final-stage features.
  - **Exposed fakes.** A buyer exposing a fake costs **−4 rep per exposed feature** (−5 on hard) through `StoreClient.ClientExposeFeature`. The −1 in BARTER.md is only the `CHECK_*` negotiation path (decomp; not measured here).
  - **What buyers can expose** is fixed per faction (`ClientCanExposeFunc`): water purity, cigarettes, unusable injectors, chemical purity. No counter buyer can expose a fake stamp; only the trade board can.
  - **The Deal Maker machine** adds +10 % (+ module quality) to bargain odds for one customer.
- **Unknown:** the threaten tooltip on non-augs (UI); that the stacked features multiply is inferred.
- **Executor:**
  - `PlayerPolicy.Sell`/`Buy` take the EV-best of discount, markup and demand, with an LL rep guard on hard.
  - It never blackmails or stacks.
  - Staging reads hidden truth (gap #1).
- **Best play:** EV haggling as now (TRADING-STYLES: aggressive loses to the mob). Blackmail only against a faction whose rep no longer matters.
- **Gap:** #1. Blackmail-plus-demand on a wanted wholesale seller is low value (S).

### Clients and their hidden traits

- **Guide:** customers show wants, refusals and a budget; the case attracts buyers; posters list criminals.
- **Real:**
  - **Budget bonuses** (`StoreClient.ApplyBudgetModifier`): UL 100 +25 %; Security 80 **+50 %**; Sycophant +10 % on UL; Endorsement +25 % per tier; attractiveness 300/450/600/750/900 → +10/25/50/75/100 %.
  - **A sell-and-buy client's budget grows by 50 % of what you pay them** (`OnItemSold`, decomp-only), so buy from a miner first and he can buy more of yours.
  - **The no-contraband flag** is set on LL buyers 35 %, scavs (25 − LL/4) %, Security always.
  - **Price features:** Tourists and variety buyers +15 %.
  - **The wanted list:** docs/re/CLIENTS.md "The wanted list" (new).
  - **Barter-offer clients** trade one cheap item for a fixed bundle, ~13 visits a run.
- **Unknown:** the value of the barter bundles; a stealth-wanted client is hidden from the player.
- **Executor:**
  - Dismisses every wanted client (`BarterRules.CoverageGap`). That includes stealth-wanted ones, so a hidden flag leaks into behaviour (part of #1).
  - Sells before buying to sell-and-buy clients.
  - Dismisses barter offers.
- **Best play:** report posters (pace against LL rep, skip Lou); buy before selling to sell-and-buy clients; accept bundles worth more than the item asked.
- **Gap:** #2. Buy-before-sell (tens a week, S); barter offers (~650 a run inferred, M).

### Inspection of items (cigarettes, food stamps, injectors)

- **Guide:** examine a seller's item against the guides and accuse it of a flaw; a wrong accusation costs rep.
- **Real:**
  - **Confronting** (INSPECT.md): correct → true value −15 %; wrong → −2 rep with that faction, repeatable.
  - **Injectors:** each fake has one fault, and the text alone catches ~75 %.
  - **Food stamps:** an amount not a multiple of 40, or a seal that doesn't match, means a fake.
  - **A genuine stamp redeems for its face value** in commissary credit, so one bought at 0.75× returns 1.33× in goods, and redeeming exposes a fake at no cost (decomp-only, `CommissaryData`).
  - **Talking a "counterfeit" claim to genuine** is a free oracle: +2 rep and 85 if it is genuine, nothing lost if not (decomp-only, `InsCigaretteHelper`).
- **Unknown:** what the guide prefabs print (sprite art); whether tier-2 sprite cues are player-visible.
- **Executor:**
  - `ConfrontInjectors` behind `inspect: "injectors"` (Episode.Player.cs). The measured arm was −3.4 ± 10.4 days, not significant.
  - Cigarettes and stamps are never confronted; stamps are never redeemed.
- **Best play:** inspect everything inspectable; keep exposed fakes out of the case; use the talk path.
- **Gap:** low value (0–100 a week), high stream legibility. S each.

### Heat, stolen goods, inspections, concealment, permit

- **Guide:** stolen goods carry heat that falls; contraband is risky; inspectors search; hiding spots exist.
- **Real** (CONCEALMENT.md, MITIGATIONS.md, STATE.md):
  - **Heat** falls 18 a night for everything.
  - **FENCING crime = sale price × heat %,** so selling at heat 0 writes nothing.
  - **Buying stolen** carries a −10/−15/−20/−25 % discount by heat band (`ApplyStolenDiscount`; missing from STATE.md).
  - **An inspection** finds stolen items with chance = heat; then one luck roll voids the whole find and withholds the +3 (verified).
  - **Luck** = perk + coin (+5) + liquid luck (+75) (MITIGATIONS.md, corrected).
  - **Any** contraband in the showcase brings an inspector, and a stolen item's showcase roll resets its heat to 100 (`ShowcaseHelper`; MITIGATIONS.md §3B says only "serious": a sub-audit reading, not corrected here).
- **Unknown:** the patrol reacting to contraband added while present (can't test headless); whether items can be moved after the inspector's personality shows.
- **Executor:**
  - `Conceal.Morning` stashes every flaggable item each morning. `on_warning` behaves exactly like `always`; `use_trash_if_lazy` and `rep_value` are not implemented (CONCEALMENT.md §6 is stale on this).
  - The shady-inspector bribe compares only the seized value.
  - Liquid luck is never drunk.
- **Best play:** keep trafficking under 50 (sell at heat 0). With early warning, conceal only on the "imminent" night and collect the +3 otherwise. Drink liquid luck when contraband can't be hidden.
- **Gap:** #8. A true `on_warning` (+3 Security per clean inspection; matters when chasing VIP; S).

### Labeler

- **Guide:** change an owned item's label.
- **Real** (LABELER.md):
  - The label sets the claim and reprices at once. A counter exposure costs rep but is not a crime; mislabel crimes come only from vending, water and chemicals.
  - **A claim worse than the truth is never exposed** (`IsRealFeatureCheaper`), so relabelling a genuine pack claimed counterfeit up to genuine is honest (+50).
  - **Free gifts keep a "free" feature that prices them at 0;** "free" is a labeler category only the trade board can expose.
  - **Five editable categories never occur on any item in this build** (rusted, dented, open box/sealed, expired, replica; decomp, inferred from absence).
- **Executor:**
  - `Labels.Morning`/`HonestLabel`/`ForBuyer`, off by default.
  - `HonestLabel` skips non-"genuine" claims.
  - `UpgradeCategories` targets exactly the five dead categories.
  - "Free" is never relabelled.
  - The `labeler` route is −1 240 ± 1 136 NW (ROUTES.md).
- **Best play:** build the truth map from one's own reads (it also closes #1); relabel free gifts and under-claimed genuine packs.
- **Gap:** #1, plus free gifts (S, 60–250 a run) and the dead-code cleanup (S).

### Augs

- **Guide:** some customers are augs with tells; shoot only when sure.
- **Real** (AUGS.md):
  - **Presence:** escape +15, kill −10 on hard; game over at 100.
  - **Kill reward:** clamp(100 + days since intro, 100, 250), +8 Security, +2 LL/BM/Revolution. Each +2 LL is also +80 Wild's Favor (`ModReputation(string,int)`).
  - **The all-clear** comes on the first day it is due (corrected in AUGS.md §1.6). `Shoot` removes only unowned items from the table (decomp).
- **Unknown:** whether an aug can be shot after a completed sale (keep the cash and the bounty).
- **Executor:** `AugRule.Pick` shoots on any sure tell before trading: precision/recall 1.000/1.000, 8 kills a run.
- **Best play:** decide when the arc ends: alternating walk and kill nets +5 presence a pair, keeping augs, cash and Security coming at a bounded risk.
- **Gap:** #7 (verified). Sell-then-shoot is a lab question (S).

### Item conditions, repair and upgrade kits

- **Guide:** a condition line moves the price; a gun part fixes a gun; kits enlarge machines.
- **Real** (decomp-only as a set):
  - **Conditions** are fixed % modifiers (`ItemConditionList`): equipment New +25 … Poor −25; shabby −75; second-hand −50; free −999.
  - **Shabby** marks every start-kit machine (resells at 25 %).
  - **Gun condition also scales all six stats** ±30 % (`GunHelper.UpdateCurrentGunStats`).
  - **Gun repair:** chance = repairability × attempts since the last success. The part is used either way: 1.0–1.85 parts per step.
  - **Repair never pays at resale; it pays through gun orders** (`GunOrderData`: Basic 86–95 + ~46 per bonus threshold met; Best market × 1.5; service orders 30/60/100). RECIPES.md §1.6's "pays above ~200" ignores failed attempts.
  - **Kits.** Module bay kit 150, +100 value, max 2 per machine. Storage kit 1 063, +2 tabs.
  - **Broken machines carry stuck modules and charged cells** (`MachineBrokenHelper`; RECIPES.md §5 says scrap only).
- **Unknown:** the gunsmith and gun-order flows are stubbed headless.
- **Executor:** no repair, no gun orders, no kits.
- **Best play:** repair only to cross an order threshold (a Poor handmade pistol to Fair passes the firepower minimum); never value shabby kit at list.
- **Gap:** gun orders (Gunsmith start, L); alarm bay kit (below).

### Machine modules (general model)

- **Guide:** modules add performance, efficiency or quality; some have special effects.
- **Real** (MODULES.md §1–2.3 accurate, plus decomp):
  - **Stuck modules** are locked in until extracted. A random stuck module is ~60 % performance.
  - **Two random effects multiply** on the base (premium + overvolted 2.4×; overvolted + degrading 4×, decaying, 10 %/night ruin).
  - **Mount bonuses** change a working stat, not the base; machine totals sum working stats.
  - **Streamlining** is registered but has no source.
- **Executor:**
  - `Modules.cs` buys from Jackson and ranks by base stats.
  - `TendAlarm` refits to P ≥ 50; `ClearDead` dumps ruined/corrupt modules.
  - No kits, extractor or mount positions.
- **Best play:**
  - Kit the alarm bay so a transmitter plus four plain performance modules reach P50.
  - Restore corrupt modules with an electronic, and screwdriver ruined ones.
  - With an extractor, buy the scrapper's broken alarm for its modules.
- **Gap:** alarm bay kit on `watchman` (17/60 runs short of P50; ~+1–3k NW by day 150; S). Corrupt restore (S, small).

### Perks

- **Guide:** up to 4 perks within 3 points; negative perks refund points.
- **Real** (PERKS.md matches the decomp on everything sampled):
  - **Alert returns before the burglary roll** but after the alarm's transmitter step.
  - **Uninsured burglary** escalates 33 + 5 × nights since insurance was last paid. Every route drops insurance from day 56, so burglary is near-nightly from ~day 70.
  - **Mind-reprogrammer charges** cost a hypertech electronic (2 500).
- **Executor:**
  - `PlayerRole.Services` ignores Alert (and Security VIP) when buying insurance (#5).
  - The day-0 perk menu is frozen to two sets (`internal/agency/menu.go` `PerkSets`).
- **Best play:** pay for extra perks with refunds the route never feels: Blabbermouth on no-crime routes, Maimed on routes that never scavenge.
  - Example: vending + short sleeper + alert + blabbermouth (cheap to arm-test: `-perks` only, once #5 is fixed).
- **Gap:** #5; a hard perk search (Alert, Well Connected, Wild's Favorite, refund perks; M; est. +1–3k NW).

### Scavenging (dumping ground, Georgio)

- **Guide:** a few searches a night for junk; each can injure you.
- **Real** (PRODUCTION.md §6 + decomp):
  - **Searches:** 5 base (±2 by perk).
  - **Osteoarthritis** drops the total to **0 for good** once it would fall below 3 (day 22 on base 5); Georgio's +5 comes after that check.
  - **Each search** rolls minor injury (5 + n %), then major (1 + 0.25n %), then loot.
  - **A flashlight carried anywhere halves the injury odds.** Luck cancels an injury.
  - **Any healing item clears the "fresh" flag,** so searching resumes the same night. An unstable wound doesn't heal that night.
  - **Tools carried** add a 10 % extra per search each (screwdriver nuts, wire cutter wire, welder scrap). The metal scanner doubles loot 15 % (+5 per upgrade).
  - **Wednesday's fresh dump** raises the table EV ~27 %.
  - **Georgio's assistance** is a single 50-credit offer (3 nights, no injuries, +5 searches), never a random event.
  - **Georgio brings dumped nuclear waste back.**
- **Executor:**
  - Goes most nights through the no-backpack choice (`LiveHands.ChooseNoBag`; the gym the same).
  - Never bandages; takes Georgio's offer only through the dialogue default.
  - **Logging bug:** after an injury the loop runs on to 10 tries, logging up to ~9 fake `night:scavenge` actions.
- **Best play:** a backpack with flashlight, scanner and tools; bandage and continue; prefer Wednesdays when rationing; always buy Georgio's three nights.
- **Gap:** #9; the logging bug (S, data quality).

### Night outings and after-hours locations

- **Guide:** one trip a night: the dump, the commissary or Dr Jackson.
- **Real** (decomp-only, mostly):
  - **The door is shut** at wound > 5, during a crackdown (unreachable, below), and while the Butcher hunts you unless a gun is in the chosen backpack (`ResponseList.CreateAfterhoursResponse`).
  - **Any outing costs one customer** next day unless Short Sleeper.
  - **Jackson:**
    - Closed Mondays unless JACKSON_SPECIAL_STOCK, which replaces the stock at +150 points.
    - Sunday −10 points, on top of hard's +50 %.
    - Sells the satchel and backpacks only at night.
    - Buys materials and ruined modules.
  - **The commissary** restocks *opposite* to market events for its type.
  - **Serpico's food quest** (day 40+) pays +12 rep to a faction.
- **Executor:**
  - Jackson every 7 nights; never the commissary; Serpico ignored.
  - The gym's `NightOuting.DoorShut` misses the Butcher gate, so the gym can make outings a player cannot.
- **Gap:** Butcher gate (S, parity), backpack purchase (prerequisite for #9), commissary (low), Serpico (S/M, stream).

### Black market

- **Guide:** black-market visitors sell stolen goods and buy contraband; rep unlocks perks.
- **Real** (decomp-only, mostly):
  - **Visit share:** a BM client is picked 4/7/14/17 % of the time by rep band, so **BM rep 25 doubles BM traffic**.
  - **The shady merchant pays only half the contraband markup** (+17/35/50/75 % instead of +35/70/100/150 %), in exchange for half-counted crimes.
  - **The Exchange** (Sawyer, janitor nights): 7 standing barters plus 3 rotating promos. Loot-crate contents scale with luck at creation.
  - **Liquid luck:** gap #8.
- **Executor:**
  - Never sells contraband (`sell_contraband` off) and refuses serious lots.
  - No Exchange; liquid luck is sold as stock.
- **Gap:** #8; Exchange (M/L); contraband to BM buyers only (needs an arm; the sell-all arm cost −8 days with 35/60 arrests).

### Market events and forecasting

- **Guide:** events push category prices for a few days; the newspaper lists them; an informant sells tips.
- **Real:**
  - **Events** add a % at the market stage, stacking, on buys and sells.
  - **Random events** start the same day without warning (50/35/25 % a morning by active count, cap 3).
  - **A paid tip *creates* the event it describes** (`QueueFuturEvent(..., IsKnown: true)`): it is always right, 50 credits, ~1.2 tips a week from day 14, and ~53 % are +50 events.
  - **Hard's newspaper has no end countdown** (`eventEndCountdown` false).
  - **Faction power modifiers are never read,** so crackdown, martial law, riots, gang war and Sec raids are **unreachable**. Only four operations fire: weapons +25 (~8 days), weapons −35 (~14), household −40 (~9–10), and one Sec budget op.
  - **Vending prices ignore events** (`GetVendingPrice(includeEvents: false)`).
- **Unknown:** that the power modifiers are dead is from a grep, not a run.
- **Executor:**
  - `ExecutorMorning.Plan` puts any type with a ≥ +20 event active today…+3 in `RisingTypes` and buys it up to the estimate, including events already running.
  - The informant is dismissed; vending stocking ignores events.
- **Best play:**
  - Buy every tip above the rent floor, stock before the start, sell during the event; never buy into a running event.
  - Trade the weapons cycle (buy at −35, sell at +25).
  - Vend the depressed type.
- **Gap:** #4; the operations cycle (+30–80 a week, S); vending during events (small, S).

### Reputation

- **Guide:** per-faction bars, perks at thresholds, some factions end the run at −100.
- **Real** (PERKS.md §3, STATE.md):
  - **Trade rep** = pre-bargain value × multiplier / 400.
  - **On hard** gains ×0.75 and losses ×1.25: ~444 credits of UL trade per UL point.
  - **Perks that pay:**
    - LL 20 referral (+1 customer a day);
    - UL 20 tips (+15 % on UL sales);
    - **Sec 80 +50 % budget for every budgeted client;**
    - UL 100 +25 %;
    - Sec 100 VIP.
  - **Every static LL gain also grants +40 Wild's Favor a point** (not difficulty-scaled).
  - **Marketplace requests** (upgrade 375 on hard) pay 1.5–2× value plus rep.
- **Unknown:** a likely game bug: the scripted requests test `day + unlockDay` where a difference was probably meant. The payoff of rep perks was not re-measured since runs got long.
- **Executor:**
  - Rep is passive: LL guard, UL steering after Tips.
  - No request filling.
  - `watchman` reaches Sec 100 at a median of day 116, after the buyout.
- **Gap:** #10; aiming at Sec 80 before the buyout (S–M).

### Debt: rent, Grace, buyout, mortgage, Georgio

- **Guide:** pay weekly rent or lose; buy the deed or take the mortgage.
- **Real** (MITIGATIONS.md §1–2 + decomp):
  - **Rent grows on both the paid and the unpaid path** (`OnRentPaid` and `OnRentNotPaid` both call `RentIncrease`; settles VERIFIED.md's can't-test row by code).
  - **The buyout charges only the price,** never this week's rent or a Grace backlog.
  - **The mortgage has no early payoff,** and a repossession is not a payment.
  - **The landlord's vending offers** add +150 to rent, compounding.
- **Executor:** `ExecutorMorning.Plan` floors, sell-downs, buyout projection and `buyout_save`; `PlayerRole.Dialogue` for the landlord, Georgio and the mortgage; Grace bought first.
- **Gap:** #6; landlord fountain vs Wild's for the `fountain` route (~+200, S).

### Wild network, services, upgrades, vending

- **Guide:** phone 8376 for favors, referrals and nightly services.
- **Real** (MITIGATIONS.md §0 + decomp):
  - **Renovations:** +150 attractiveness each (750/1 125/1 500 on hard).
  - **Attractiveness thresholds:** 200/250/500 add a customer; ≥ 400 promotes display buyers to tier 2; 300…900 lift budgets.
  - **The garbage penalty** is a % of base; janitorial every other night zeroes it.
  - **Shareholders:** −15 % wholesale plus a weekly dividend.
  - **Vending:** sale chance = attractiveness × type factor × 200/(200 + price); repeated ids halve.
- **Executor:**
  - `PlayerRole.Upgrades` (Grace, engines, then the growth list).
  - `upgrades_cash_max: 937` keeps RENOVATION3 unbought.
  - `Vending.cs` is already at the cap.
- **Gap:** none confirmed. Early RENOVATION3 is **refuted** (+5.8 ± 2.4 days later).

### Display window (clientele)

- **Guide:** what you show decides who visits tomorrow.
- **Real** (WINDOW.md, all CONFIRMED):
  - **Display buyers** = 1 + referral + showcases; one per top-N type by displayed value.
  - **Tier** by that type's real value in the case: ≥ 145 → 2, ≥ 350 (from day 14) → 3.
  - **Discerning shopper:** 1 + UL/5 % with a luxury item shown.
- **Executor:** `PlayerRole.Window`: `types` (best item per type, so a type rarely reaches 350) or `concentrate`.
- **Gap:** a threshold-aware packer (N types each ≥ 350). Uncertain: past window arms were not significant, but none targeted tier 3 (S).

### Other systems found in the decomp

| System | What it is | Matters? |
|---|---|---|
| Lou storyline (`LouBombHelper`, `LouOperativeHelper`) | Reporting `wanted5` starts a chain ending in a demand (1 000 on hard); underpaying ends the run | **Yes:** a game-over risk the moment wanted-reporting (#2) is added |
| Lottery, scratchers, gambler's coin (`ScratcherHelper`, `LotteryHelper`) | A rep visits weekly. A scratcher costs 20 (EV 11.9 at luck 0, 30.5 at luck 75); a card re-rolls if luck rose since it was stamped. 10 scratched cards give the coin (+5 luck for good). Lottery EV 14.3 on 25 | Coin = cheap permanent inspection luck; cards +EV only under liquid luck. Stream interest high; M (UI) |
| Commissary (`CommissaryData`) | Fixed stamp-credit catalogue; restocks swing against events | Low |
| Serpico (`SerpicoStoryline`) | Food quest, then a faction +12 or a 600 report | Rep, stream |
| Storage expansion (`StorageExpansionHelper`) | +2 tabs a kit, max 5 | Low unless storage binds |
| Gun orders (`GunOrderData`, `StoreClientListTierWeapon`) | See conditions and repair | Gunsmith start only |
| Faction operations (`OperationList*`) | Only four ever fire | See market events |
| Event clients (`StoreEventActionDict`) | Per-event buyers arrive; shortage/surplus clients never do (key bug, VERIFIED.md) | Water route (other agent) |
| StoreScale | An item on the back counter reveals its weight feature | Inspection aid, not yet studied |
| Dead code | The space-roguelike prototype (Raid*, Ship* except `ShipSystemDirectory`, TurnManager, Overworld*, Station*, Combat*), expeditions, the church storyline, unused client and event lists | No |
| No gameplay effect | Analytics `*Event` classes, achievements, cosmetic classes, tag and grid infrastructure | No |

The full list of decomp classes with no `docs/re` coverage (359 names after UI prefixes) was produced for this
audit. The ones that matter are in the table above. Owned elsewhere: fish, rats and husbandry, plants and
genetics, water, wine, chemistry.

## Corrections to docs/re

**Made in this commit, each confirmed by a check above:**

- **MITIGATIONS.md Luck row.** Luck has three sources (perk, coin, liquid luck), and a void withholds the +3.
- **CONCEALMENT.md §3.** No +3 when luck voided a find.
- **AUGS.md §1.6.** The all-clear comes on the first day it is due, not on a spawn day; the measured arc length.
- **CLIENTS.md.** A new section, "The wanted list".
- **VERIFIED.md.** Two new rows; the stale appraisal "can't test" row narrowed.

**Found by reading the decomp, not yet measured or applied.** Each needs a lab check first:

- **BARTER.md:**
  - a buyer exposing a fake is −4 per feature, not −1;
  - the no-contraband flag also applies to scavs and Security;
  - rep figures are before the difficulty factor.
- **CLIENTS.md:** the budget modifiers; `OnItemSold`'s 50 %.
- **APPRAISAL.md:**
  - impatience is per client;
  - with no normal slot, no appraiser comes and the missed-day count still rises.
- **MITIGATIONS.md §3B:**
  - any contraband in the case brings the inspector;
  - the showcase roll resets heat to 100;
  - the rep formula's stolen tier.
- **CONCEALMENT.md §6:** the executor description is stale.
- **LABELER.md:**
  - add a stamp column and the "free" relabel;
  - five categories never occur.
- **STATE.md:** the stolen buy-discount bands.
- **PRODUCTION.md:**
  - §6: the osteoarthritis cliff, bandages reopening the dump, Georgio's assistance as a single offer;
  - §3/§5: Jackson's Mondays and night stock; the unreachable operations; tips create events; markup 0 on hard.
- **DAY.md:** Jackson buys as well as sells; hard has no event countdown.
- **RECIPES.md:**
  - §1.6 gun repair math;
  - §5 broken machines carry modules.
- **MODULES.md:** streamlining, the stuck-module distribution, effect stacking.
- **PERKS.md:**
  - the reprogrammer's charge cost;
  - Alert still runs transmitters;
  - only REV_FRIEND reveals operations.
- **WINDOW.md:** renovations in the attractiveness formula.
