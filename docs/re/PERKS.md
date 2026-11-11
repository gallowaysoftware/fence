# Perks (RE notes)

Build 26525552. Two perk systems change the rules a run is played under:

- **Starting perks**: 48, chosen once on the new-game screen, stored in
  `PlayerStore.startingPerks : List<StartingPerk>`, checked through static
  `StartingPerk.Is<Name>() : System.Boolean` helpers (`StartingPerk.IsPerkActive(System.String)`).
- **Reputation perks**: 41 on the five standard factions plus 4 on the Cartel (Chemist start
  only), unlocked by rep thresholds, checked with
  `StoreReputation.IsPerkUnlocked(System.String) : System.Boolean`.

Display names are from `var/gamedata/loc_en.json` (`PerkTable` / `RepPerkTable`); effect text
below is ours, from code. Where the in-game description and the code disagree, the code wins and
the row says so. Machine-readable twin: `internal/strategy/perks.json` (section 6).

## 1. How starting perks are chosen

| Rule | Value | Evidence |
|---|---|---|
| Screen | after start and difficulty: start → difficulty → perks | `NewGameDifficultyUI` → `NewGamePerksUI.Open()` |
| Budget | sum of `cost` ≤ **3**. Positive perks cost 1–2, negative ones refund 1–4 points, neutrals cost 0–1. No lower bound: an all-negative loadout is legal | `NewGamePerksUI.UsedPoints`, `CanStart`, `MAX_PERK_POINT` |
| Slots | at most **4** perks, +3 with Colorful Character (7) | `NewGamePerksUI.MaxPerkCount` (`maxSlot`) |
| Exclusions | 10 incompatible selectable pairs (both directions checked): ascetic–minimalist, banned_commissary–welfare_program, convict–model_citizen, lucky–unlucky, maimed–{proficient_scavenger, careful_scavenger, degenerative_osteoarthritis}, prime_real_estate–side_street, public_enemy–well_connected, wild_favorite–wild_bad_side | `NewGamePerksUI.LockedBy`, `starting_perks.json` |
| First run | the very first run of a save profile clears and blocks perks | `NewGamePerksUI` (`totalRunCount == 0`) |
| Start types | every released start can take any perk; no start forces or forbids one | `StartingScenario`, `NewGameData` |
| Order | kit first (`NewGameData.HandleInitialItem`), then perks (`StartingPerkContent.HandleNewGamePerk`), so perk cash/rent/rep **stack on** the start's own offsets | `EmporiumEntry` new-game path |
| Rep offsets | perk offsets use `ModReputationRaw` (unscaled); start offsets (Fence, Gunsmith, Chemist) use `ModReputation`, so they are scaled by the difficulty gain/loss multipliers | `StartingPerkContent.HandleNewGamePerk`, `NewGameData.HandleInitialItem` |
| Mid-run changes | addictions are removed after 20 clean days; Blackmail is removed when refused or the blackmailer is captured; the hypertech smuggler's **mind reprogrammer** can add a non-negative `mindEditable` perk or remove any `mindEditable` perk (charge = |cost|, cooldown 5 days per point) | `HealthData` nightly tick, `StoreClientListSpec.TriggerBlackmailerCapture`, `MindReprogrammerHelper.CanApply/Apply` |
| Legal loadouts | 185,463 with ≤ 4 perks; 9,097,004 counting Colorful Character's 7-perk loadouts | enumeration over `starting_perks.json` |

A few ids have `Is*()` helpers but no factory in `StartingPerkList.InitStartingPerk`; they
cannot be selected in this build and are ignored here.

**Visibility.** Active starting perks are **VISIBLE** in the calendar's Status panel
(`AdvCalendarUIManager`, name + description with numbers). Hidden: the luck value behind
Lucky/Unlucky and the gambler's coin, the reactor's decay rate, the Secret Admirer odds.
Reputation perks are **VISIBLE**: `ReputationUIManager.UpdateToolTipContent` shows every perk of
a faction, negative ones included, with `threshold/200`, unlocked state and description.

### Difficulty interactions

`var/gamedata/difficulty.json` (cozy / normal / hard / challenge):

- Rep gain ×1.5/1/0.75/0.5 and loss ×0.5/1/1.25/2 decide how fast thresholds are reached or
  fallen through. Perk offsets are raw, so on challenge Well Connected (+15) is worth ~30 rep of
  trading, and a Convict start is ~15 rep of losses that would otherwise have cost 7.5.
- Fence start Security −30 is scaled: cozy −15, challenge −60 (Most Wanted on day 1).
- Aug Menace makes the aug game over apply on cozy/normal, which otherwise lack it
  (`augGameOverAtFullPresence`).
- Wild's Favorite / Bad Side multiply `upgradePriceMultiplier` (1/1/1.5/2) and
  `insuranceCostMultiplier` (1/1/2/3); rent perks compound with `rentIncreaseMultiplier`
  (1.025/1.05/1.1/1.1).
- Sentinel's scalper markup grows `scalperMarkupPerDay` per day (0.5/1/2/3).

## 2. Starting perks

Cost is points (negative = refund). Fit/avoid name archetypes from `docs/research/ARCHETYPES.md`.
**Impact**: *major* = changes which actions are good or available; *moderate* = shifts a number
the planner already weighs; *minor* = cosmetic or small windfall.

### 2.1 Money, rent and traffic

| Perk (id) | Cost | Effect (code) | Strategy | Fit / avoid |
|---|---|---|---|---|
| Savings (`saving`) | +1 | +150 cash (`HandleNewGamePerk`) | one purchase sooner; biggest on Rock Bottom, Gunsmith, Aquafarmer | all |
| Bad Lease (`bad_lease`) | −1 | rent +40, compounds weekly (≈ +500 over 10 weeks, normal) | cheap point for engines that clear rent easily | vintner, magnate / shark |
| Prime Real Estate (`prime_real_estate`) | 0 | +1 normal customer/day (`StoreClientManager.GetNormalCustomerCount`, clamp 0–6); rent +75 | positive once a marginal customer clears ~12–20 a day; also speeds trade rep | honest-broker, magnate, vintner / shark |
| Side Street (`side_street`) | −1 | −1 normal customer/day (same site) | ~15–20 % of base traffic | vintner, rancher, smelter, cook / honest-broker, shark, magnate |
| Short Sleeper (`short_sleeper`) | +1 | an after-hours outing no longer costs one normal customer the next day (`GetNormalCustomerCount`, `PlayerStore.HandleAfterhourDebuff`) | makes nightly scavenging free in traffic | smelter, shark |
| Wild's Favorite (`wild_favorite`) | +1 | upgrades ×0.9, insurance ×0.5 (`NetworkUpgrade` cost, `StoreService`) | −10 % on every engine; stacks with LL Pillar ×0.85 | magnate, vintner, water-baron, rancher |
| On Wild's Bad Side (`wild_bad_side`) | −1 | upgrades ×1.1, insurance ×1.5 | a growth tax; cheap for runs that buy few upgrades | shark, honest-broker / magnate |
| Welfare Program (`welfare_program`) | +1 | commissary from day 1; on day 1 and each day%7==1 a genuine stamp worth 160/160/120/80/40 while adjusted wealth (cash + (stock estimate − cash)/3) ≤ 600/1200/1800/2400/3000 (`StartingPerkContent.HandleWelfareProgram`) | ≈ 160 a week early, fading with wealth | shark, honest-broker, rancher |

### 2.2 Prices and bargaining

| Perk | Cost | Effect | Strategy | Fit / avoid |
|---|---|---|---|---|
| Awful Negotiator (`awful_negotiator`) | −3 | bargain button never shown (`NegociationUIManager.RefreshBargainControl`); a fixed 5 % penalty feature on every counter item: buy 5 % dearer, sell 5 % cheaper (`GameItem.OnAddedToClientArea`, `ItemFeatureList.AwfulNegotiatorBuyPenalty/SellPenalty`) | removes the haggling action family; win on sourcing, markup and production | vintner, rancher, smelter, water-baron / shark, honest-broker, fence |
| Alcohol / Narcotic / Nicotine Addiction (`alcohol_addiction`, `narcotic_addiction`, `nicotine_addiction`) | −1 each | withdrawal from day 1 (counter starts at 3): while ≥ 3 days since a dose, 8 % per addiction on every counter trade (`HealthData.GetTotalAddictionPenalty`, `AddictionBuy/SellPenalty`). 20 clean days cure it and remove the perk; a dose resets the counter negative. Kit: nudka / pink injector / counterfeit cigarette | cold turkey costs ≈ 17 days of −8 % and is then free; feeding it delays the cure. Two addictions = −16 % | vintner, rancher, smelter, cook (narcotic) / honest-broker, shark |
| Sycophant (`sycophant`) | +1 | UL +15 (raw); UL gains ×1.25 (`StoreReputation.ModReputation`); UL budgets +10 % (`StoreClient.ApplyBudgetModifier`) | one trade from UL Tips; with Well Connected UL starts at 30 | vintner, water-baron, honest-broker, magnate / shark |
| Diplomatic (`diplomatic`) | +2 | trade rep ×1.33 (`StoreReputation.OnItemTraded`) | every positive rep perk a third sooner | armourer, honest-broker, vintner, rancher, magnate |

### 2.3 Crime, inspections and luck

| Perk | Cost | Effect | Strategy | Fit / avoid |
|---|---|---|---|---|
| Intimidating (`intimidating`) | +2 | every committed crime ×0.67 (`SecData.CommitCrime`) | crime budget +50 %: later inspections, smaller sentences | fence, cook, shark, water-baron |
| Blabbermouth (`blabbermouth`) | −2 | every crime ×1.33; BM and REV gains ×0.5 (`CommitCrime`, `ModReputation`) | free points for honest runs; poison for fence/cook | honest-broker, vintner, rancher, armourer / fence, cook, shark |
| Lucky (`lucky`) | +1 | luck +15 (`healthData.luck`): 15 % chance an inspection that found anything overlooks it all (`ContrabandHelper`, `HealthData.RollLuck`); 15 % to dodge each scavenging wound (`ScavHelper`); better lottery/crate draws | a hidden free pass on bad inspections | fence, cook, shark, smelter |
| Unlucky (`unlucky`) | −1 | luck −15: no saves; 15 % of searches empty (`HealthData.RollBadLuck`) | cheap for shop runs | vintner, rancher, honest-broker, armourer / fence, cook, smelter, shark |
| Gambling Addiction (`gambling_addiction`) | −1 | lottery from day 1 (`LotteryHelper.IsLotteryUnlocked`, else day > 14); a scratcher and the **gambler's coin** (+5 luck while carried, `LotteryHelper.GetCarriedLuckBonus`); forced purchases when a seller offers tickets (25) / scratchers (20) / stacks (100): first one if affordable, then 50/40/30/20 % with cash ≥ 2/5/10/25× price (`HandleGamblingAddictionItemAdded`) | a refund point that *adds* luck; forced buys are small | fence, cook, shark |
| Convict (`convict`) | −1 | Security −30 (raw); reports, captures and aug kills pay ×0.75 in cash and in the Security rep they give (`HandleReportRewardConvict`, `PlayerStore` report/capture paths, `AugHelper`) | starts on SEC Watchlist (evidence ×1.2); see 2.6 | fence, cook, shark / armourer, honest-broker |
| Model Citizen (`model_citizen`) | +1 | Security +30 (raw); those rewards ×1.5 | Security Surplus from day 1, fast road to 60/80/100 | armourer, honest-broker, magnate / fence, cook |
| Known Snitch (`known_snitch`) | −2 | no wanted-poster customers and no wanted list (`IsClientBlockedByKnownSnitch`, `GetRandomValidWantedClient`); REV and BM −10 | removes bounty income (100–1500 + Sec +5 a capture) | vintner, rancher, smelter, water-baron / armourer, honest-broker, fence |
| Blackmail (`blackmail`) | −2 | blackmailer on day 1 and each day%7==1 asks 40 + day (`HandleBlackmailClient`, `GetBlackmailCost`); refusing or capturing him (stealth-wanted, 150) costs −30 with LL, REV, Security and UL, then the perk is removed (`TriggerBlackmailerCapture`) | refuse or capture on day 1: −30 rep (LL Poor Reviews and SEC Watchlist) instead of ≈ 1300 in fees | shark, fence, smelter / armourer, honest-broker |
| Vigilant (`alert`) | +2 | no burglary roll at all (`PlayerStore.HandleInsurance`) | insurance is pure waste: drop it | magnate, fence, vintner, honest-broker |

### 2.4 Rep offsets

| Perk | Cost | Effect | Strategy |
|---|---|---|---|
| Well Connected (`well_connected`) | +2 | +15 every faction | every faction one or two trades from its 20 perk; best stacked with a faction offset |
| Public Enemy (`public_enemy`) | −2 | −15 every faction | nothing triggers at −15, but every −20 perk is one slip away; BM < 0 lowers BM client odds |
| Revolution Sympathizer (`revolution_sympathizer`) | +1 | REV +25; an extra REV request on day%7==1 when REV Request is live (`StoreRequest.StartDay`) | REV Request from day 1 (needs Marketplace Requests, 250); REV client weight 6 |
| Convict / Model Citizen / Sycophant / Known Snitch | | see 2.2–2.3 | |

### 2.5 Kit, capacity, scavenging and obligations

| Perk | Cost | Effect | Strategy | Fit / avoid |
|---|---|---|---|---|
| Vending Machine (`vending`) | +2 | the 1200 vending upgrade installed day 1 (`VendingMachineHelper.UnlockVendingMachine`) | fastest-paying perk (~70/night); honest stock only | magnate, rancher, vintner, honest-broker, water-baron |
| Working Pipes (`working_pipes`) | +2 | Gutterflow + Rustwater taps unlocked (600 of upgrades) | starts the water engine; free water for crops/wine/fish | water-baron, vintner, rancher, magnate |
| Handy (`handy`) | +1 | shabby toolbox with screwdriver, wire cutter, crowbar, welder (`AddHandyToolbox`) | tools expose hidden features in barter, open tool-gated dump extras, clear Hoarder's tool pile | honest-broker, smelter, shark, fence |
| Sawyer's Crew (`sawyer_crew`) | +1 | Underground Exchange day 1 (else day 25 except Fence start); janitor free (vandalized day/5 + 30) (`StoreService`, `HandleSawyerCrewClient`) | early exchange barters | fence, shark, smelter |
| Tinkerer (`tinkerer`) | +1 | item upgrades keep their material 20 % of the time (`ConsumeUpgradeMaterial`) | small | armourer, smelter |
| Tapehead (`tapehead`) | +1 | 7 standard cassettes (shabby; base 150, ART + LUXURY_ITEM) instead of 4 degraded ones (base 35) | cosmetic music, but also luxury stock for the window/discerning shopper; sellability unverified | vintner, honest-broker, shark |
| Secret Admirer (`secret_admirer`) | +1 | gifts at shutter open: 3 % a day, +6 %/day after 3 dry days, certain by 12; mostly cheap goods; once an Immunivax (Sentinel), once a black injector (addicted), bandages when wounded (`HandleSecretAdmirerOnOpenShutter`) | ~one small item a week | all (minor) |
| Colorful Character (`colorful_character`) | +1 | +3 perk slots | only enables wider loadouts | — |
| Illiterate (`illiterate`) | −1 | guides cannot be read (`HealthData.CanRead`) | minor for our agent (rules are injected); check the bridge never gates knowledge on reading | all |
| Minimalist (`minimalist`) | −2 | stock grid 20×10 instead of 24×10 (`EmporiumEntry.SetSmallInv`) | −17 % space | honest-broker, shark, armourer / magnate, rancher |
| Ascetic (`ascetic`) | −4 | stock grid 16×10 (−33 %) | the biggest refund; forces fast turnover, small machines | honest-broker, shark / magnate, vintner, rancher, smelter, water-baron |
| Hoarder (`hoarder`) | 0 | three immovable debris piles on the grid's bottom row: one clears after 10 rest nights, one with crowbar/wire cutter/welder (each yields loot), one by a 50-credit service over 10 nights (`DebrisHelper.PlaceHoarderPiles`) | temporary capacity loss with salvage | honest-broker, shark, smelter / magnate, rancher |
| Proficient Scavenger (`proficient_scavenger`) | +1 | 7 search attempts instead of 5 (`ScavHelper.GetMaxScavAttempts`) | +40 % scavenging | smelter, shark |
| Careful Scavenger (`careful_scavenger`) | 0 | 3 attempts, wound chance 0 (`GetMinor/MajorWoundChance`) | no lost outings; with Proficient: 5 safe attempts | smelter, shark |
| Degenerative Osteoarthritis (`degenerative_osteoarthritis`) | −1 | attempts 5/4/3 in weeks 1–3, then none once below 3 (day 22); 2 bandages | scavenging is a 3-week asset (to day 35 with Proficient) | shop/production runs / smelter, shark |
| Maimed (`maimed`) | −2 | dumping ground closed, zero attempts (`DumpingGroundLocation`, `ScavHelper`), dump-dependent story beats skipped | free points if you never scavenge | vintner, rancher, honest-broker, armourer, water-baron / smelter, shark |
| Agoraphobia (`agoraphobia`) | −1 | after any outing, 2 nights at home (`IsAgoraphobiaResting`) | outings every third night | non-scavengers / smelter, shark |
| Sentinel Collapse Disorder (`sentinel`) | −3 | weekly Immunivax required: warnings from 6 days, damage 1/2/3 a night from 7/10/14 days late, damage 20 = game over `sentinel` (`HealthData` nightly tick). Kit: one genuine Immunivax, injector guide, magnifier; a scalper sells Immunivax on day%7==6 at a markup growing per day (`HandleScalperClient`, `GetScalperMarkup`) | a hard weekly purchase that also forces telling genuine from fake injectors; keep one injector's price as a cash floor | honest-broker, cook / shark |
| Aug Menace (`aug_menace`) | +1 | augs from day 1 (shotgun, aug guide; first aug in 2–4 days; `GetAugIntroducedDay` = 1), no all-clear ever (`IsAugAllClearDue`), presence ≥ 100 ends the run on every difficulty (`EndOfDayUIManager`); kill cash ×1.25 (`AugHelper`) | a permanent identify-and-shoot task (docs/re/AUGS.md); profitable only with good tell reading | armourer, shark / honest-broker, vintner |
| Makeshift Reactor (`makeshift_reactor`) | +1 | off-grid: no power bill or blackouts, power-hijack hidden. Reactor integrity 100, decays ~18/night, powers the store above 25, repair scrap +60 / junk +30; at 0 it explodes (game over `reactor`) (`ReactorHelper.OnNight`, `ConsumeExplosion`) | saves day/10 + 5 a night but adds a must-not-miss maintenance loop | smelter, magnate / honest-broker |

### 2.6 Notable combinations

- **Gunsmith + Model Citizen + Well Connected** (3 pts): Security 25 + 30 + 15 = 70 on day 1:
  Trusted (evidence ½, rarer inspections), Permit II eligible (50), Surplus merchant. The
  Armourer's strongest opening.
- **Fence + Convict** (+1 pt): Security −60 on normal: Most Wanted (evidence ×1.5), Alarm Ignored
  (no reports). Add Public Enemy and Security reaches −75; one more slip to Thorough Inspection.
  A deliberate hard mode, not an efficient one.
- **Sycophant + Well Connected** (3 pts): UL 30 on day 1: Pourboire (+15 % on UL sales), UL
  client weight +4, Sycophant budgets +10 %. The Vintner's opening.
- **Lucky + Gambling Addiction** (0 pts net): 20 % inspection save. With **Intimidating** (2):
  the Fence/Cook risk package at exactly 3 points.
- **Proficient + Careful Scavenger** (1 pt): 5 attempts, never wounded. With Short Sleeper:
  nightly scavenging at no customer cost.
- **Vending + Working Pipes** (4 pts) is over budget alone; pays for itself with one refund
  (e.g. Maimed or Bad Lease) for a production run that never scavenges.
- **Ascetic** (−4) funds Vending + Working Pipes (+4) and leaves 3 points and a slot; the run
  pays for it in stock space.
- **Sentinel + Secret Admirer**: the admirer's one free Immunivax covers a week.
- **Alert** makes BM Uninsurable and LL Marked harmless.
- Redundancies: Sawyer's Crew exchange on the Fence start; Maimed with any scavenging perk is
  excluded; Short Sleeper is wasted with Agoraphobia or Maimed.

## 3. Reputation perks

Rep is a double in [−200, 200]; there is no daily drift. `StoreReputation.UpdateReputation`
re-evaluates every perk on every change: a positive perk is active while rep ≥ threshold, a
negative one while rep ≤ threshold. **Perks relock** when rep moves back. Threshold-0 perks
(UL Discerning Taste, SEC Requisition) are active at the start while rep stays ≥ 0; BM Honor is
`isAlwaysActive`. The −100 perks queue a killer visitor at the next shutter open
(`PlayerStore.OnLateShutterOpen`), so rep must be back above −100 before the morning. Upper
Levels and Black Market have **no** −100 perk: they can sink to −200 without ending the run.

The faction ⇄ trade rep rules (BM losses ×2, UL Petition doubling Security losses, trade rep
value/400 with LL ×0.66, UL ×1.2) are in docs/re/STATE.md and docs/sim/SPEC.md §7.

### Lower Levels (`FACTION_LOWER_LEVEL`)

| Thr. | Perk (id) | Effect (code) |
|---|---|---|
| 20 | Referral (`LLREFERAL`) | +1 normal customer/day (`GetNormalCustomerCount`; the roll is 100 %) |
| 40 | Rare Goods (`LLRAREGOOD`) | day%7 ∈ {1,5}: 50 % rare merchant, else 25 % gunsmith, else 25 % chemist (`HandlePerkClient`) |
| 60 | Word on the Street (`LLTIPS`) | 4 % a morning: a future market event revealed 2–3 days ahead; LL operations visible (`OnLateShutterOpen`, `StoreOperationManager.CanPlayerKnowFactionOperation`) |
| 80 | Tight Lips (`LLTIGHTLIPS`) | crime from contraband sold to LL customers ½ (`SecData.ApplyCrimeModifier`) |
| 100 | Pillar of the Lower Levels (`LLPILLAR`) | upgrades ×0.85; inventor stock −15 % (`NetworkUpgrade`, `StoreClientList.CreateInventor/InventorStorage`). The text also names Dr. Jackson; no code discounts him |
| −20 | Poor Reviews (`LLPOORREVIEWS`) | −1 normal customer/day |
| −40 | Mistrusted (`LLDISTRUSTED`) | LL customers stop offering contraband (`TryAddDirectSellingItemToTable`) |
| −60 | Marked (`LLTARGET`) | burglary +33; insurance ×2 (`HandleInsurance`, `StoreService`) |
| −80 | Shunned (`LLAVOIDED`) | suppliers do not answer, no inventor, Dr. Jackson closed (`PhoneUIManager`, `HandleInventorClient`, `DrJacksonLocation`) |
| −100 | Mob Justice (`LLLYNCH`) | mob at next opening: game over `mob` |

### Upper Levels (`FACTION_UPPER_LEVEL`)

| Thr. | Perk | Effect |
|---|---|---|
| 0 | Discerning Taste (`UL_DISCERNING_TASTE`) | luxury buyer, 1 + max(0,UL)/5 % a morning while a luxury item is in the showcase, 5-day cooldown, from day 14 (no gate on skip-intro starts) (`ShowcaseHelper.GetDiscerningShopperChance`) — docs/re/WINDOW.md |
| 20 | Pourboire (`UL_TIPS`) | +15 % on every sale to a UL customer (`GameItem.OnAddedToClientArea`) |
| 40 | Upper Level Charity (`UL_CHARITY`) | donated goods when the commissary FOOD, ENERGY_CELL or WEAPON modifier ≥ 66 (`IsCharityComing`) |
| 60 | Gossipmonger (`UL_GOSSIP`) | UL operations visible |
| 80 | Household Name (`UL_HOUSEHOLD_NAME`) | 20 % a day: one extra UL customer |
| 100 | Rave Reviews (`UL_TOP_REVIEW`) | **every** budgeted customer +25 % budget (`StoreClient.ApplyBudgetModifier`; no faction check, despite the text) |
| −40 | Tourist Trap (`UL_TOURISTTRAP`) | UL client weight 8 → 5 (`GetUpperLevelClientChance`) |
| −60 | Petition to Security (`UL_PETITION`) | Security losses ×2 (`ModReputation`), daily evidence ×1.5 (`SecData`) |

### Security (`FACTION_SECURITY`)

| Thr. | Perk | Effect |
|---|---|---|
| 0 | Requisition Order (`SEC_REQUISITION`) | from day 8, day%7==4: officer buys food/medical, trades raise Security |
| 20 | Security Surplus (`SEC_SURPLUS`) | day%7==3: surplus merchant |
| 40 | Security Protection (`SEC_PROTECTION`) | burglary −16 |
| 60 | Trusted (`SEC_TRUSTED`) | daily evidence ½; inspection countdown +2/+4 days; Security operations visible (`SecData`, `ComputeInspectionCooldownRange`) |
| 80 | Reliable Vendor (`SEC_RELIABLE_VENDOR`) | **every** budgeted customer +50 % budget (no faction check, despite the text); the surplus merchant adds stun gun, tazer, an injector |
| 100 | VIP (`SEC_VIP`) | no scheduled inspections (`IsInspectionCountdownActive`, `HandleInspectionClient`), no burglary |
| −20 | Watchlist (`SEC_WATCHLIST`) | daily evidence ×1.2 |
| −40 | Must Be The Wind (`SEC_ALARM_IGNORED`) | the security button no longer reports customers (`StoreSecButton.OnClick`) |
| −60 | Most Wanted (`SEC_PRIORITY`) | daily evidence ×1.5 (replaces Watchlist's ×1.2) |
| −80 | No Stone Unturned (`SEC_THOROUGH_INSPECTION`) | inspection countdown −1/−1; only regular inspectors (no lazy/shady personalities) |
| −100 | By Any Means (`SEC_IMMEDIATE_ARREST`) | arrest at next opening: game over `sec` |

### Black Market (`FACTION_BLACK_MARKET`)

| Thr. | Perk | Effect |
|---|---|---|
| 0 | Honor Amongst Thieves (`BM_HONOR`) | always active; BM losses ×2 (`ModReputation`; the text says ×1.5 and the unused `repLossMultiplier` field is 1.5) |
| 20 | Small Time Broker (`BM_SMALL_TIME_BROKER`) | no code checks the id: it labels the continuous BM-rep scaling (BM client weight 10, −3 below 0, −3 below 25, +4 at 25, more at 50; thief stock ×1.25/1.5/1.75/2.5 at 40/60/80/100) |
| 40 | Safe Hands (`BM_RELIABLE`) | thief every day%7==1; petty thief 20 % a day; thieves can carry loot crates |
| 60 | Underground Network (`BM_NETWORK`) | BM operations visible |
| −40 | Blacklisted (`BM_DISTRUSTED`) | BM customers stop offering contraband; exchange closed; janitor ×2 |
| −60 | Uninsurable (`BM_UNINSURABLE`) | insurance unavailable (`StoreService`) |

### Revolution (`FACTION_REVOLUTION`)

| Thr. | Perk | Effect |
|---|---|---|
| 20 | Revolution Marketplace Request (`REV_REQUEST`) | REV requests, 80 % on day%7==5 (+1 on day%7==1 with Revolution Sympathizer); needs Marketplace Requests |
| 40 | Revolution Quartermaster (`REV_QUARTERMASTER`) | day%7==0: discounted materials, better with rep |
| 60 | Friend of the Revolution (`REV_FRIEND`) | REV operations visible |
| 80 | Raid Loot (`REV_RAID_LOOT`) | day%7==5: raider selling loot |
| −40 | Blacklisted (`REV_BLACKLISTED`) | REV client weight 0 (`GetRevolutionClientChance`) |
| −100 | Hit list (`REV_HIT`) | bombing at next opening: game over `bombed` |

### Cartel (`FACTION_CARTEL`, Chemist start only)

| Thr. | Perk | Effect |
|---|---|---|
| 25 | Increased Stipend | weekly stipend 150 + 4 × Cartel rep (`StoreClientListCartel`) |
| 50 | Reliable Chemist | extra cartel materials cost 6 rep instead of 12 |
| 100 | Authorized Exit | the handler processes your exit; each exit dialogue raises this threshold by 25 |
| −100 | Forced Retirement | hitman at next opening: game over `cartel_retired` |

### What rep perks mean for strategy

- **Traffic**: LL Referral/Poor Reviews (±1 a day), UL Household Name (+0.2), UL Tourist Trap,
  REV Blacklisted.
- **Price**: UL Tips (+15 % to UL), UL Rave Reviews (+25 % all budgets), SEC Reliable Vendor
  (+50 % all budgets). Security 80 and UL 100 together lift every budget by 75 %.
- **Crime/inspections**: SEC Trusted/VIP vs Watchlist/Most Wanted/No Stone Unturned, UL
  Petition, LL Tight Lips. SEC VIP switches the inspection subsystem off.
- **Supply**: LL Rare Goods, BM Safe Hands, REV Quartermaster/Raid Loot, SEC Surplus; lost via
  LL Mistrusted/Shunned, BM Blacklisted.
- **Loss conditions**: LL/SEC/REV/Cartel at −100.
- **Information**: the four "operations visible" perks and LL rumors.

## 4. Archetype synergies

| Archetype | Recommended perks (≤ 3 pts) | Rep targets that pay |
|---|---|---|
| Honest Broker | Diplomatic, Prime Real Estate, Handy; refunds Blabbermouth, Unlucky, Maimed | LL 20, SEC 60→100, UL 20 |
| Water Baron | Working Pipes, Sycophant; refund Maimed | LL 60 (rumors), UL 20/100 |
| Vintner | Sycophant + Well Connected, or Working Pipes + Sycophant with a refund (Side Street, Blabbermouth) | UL 20 → 100, SEC 80 |
| Smelter | Proficient + Careful Scavenger, Short Sleeper, Handy; Makeshift Reactor if scrap flows | LL 40 (turbo boosters), REV 40 |
| Fence | Intimidating, Lucky, Gambling Addiction; Sawyer's Crew off the Fence start | BM 40, LL 80 (Tight Lips), keep SEC > −60 |
| Cook | Intimidating, Lucky; refund Narcotic Addiction | Cartel 25/50/100, LL 80 |
| Armourer | Model Citizen + Well Connected, or Model Citizen + Diplomatic; Aug Menace for bounty cash | SEC 20→100 |
| Rancher | Vending, Wild's Favorite; refund Maimed | LL 20 → 100 (Pillar) |
| Shark | Savings, Proficient Scavenger, Welfare Program; refunds Blackmail (refuse it), Wild's Bad Side | LL 20; UL and BM have no death floor |
| Magnate | Vending, Wild's Favorite; refund Bad Lease | SEC 80 + UL 100 (budgets) |

## 5. State space: what matters, clusters, sampling

### 5.1 Materiality

**Rule-changing** (the policy must act differently; train on each explicitly): Awful
Negotiator (no bargaining), Sentinel (weekly purchase, death), Aug Menace (aug task from day 1,
death on any difficulty), Makeshift Reactor (maintenance, death), Blackmail (pay/refuse), Known
Snitch and SEC Alarm Ignored (no bounties), Maimed / Agoraphobia (no or rare scavenging),
Banned Commissary (no stamps), Vending and Working Pipes (an engine on day 1), the three
addictions (cold-turkey vs feed decision), Ascetic (capacity), SEC VIP (no inspections), BM
Blacklisted and LL Shunned (supply cut), the −100 floors.

**Number-shifting** (same actions, different values; the policy should read them as features):
cash/rent (Savings, Bad Lease, Prime Real Estate, Wild ×2, Welfare), traffic (Prime Real
Estate, Side Street, Short Sleeper, LL Referral/Poor Reviews), counter prices (addictions,
UL Tips, budgets), crime multipliers (Intimidating, Blabbermouth, Tight Lips, evidence
multipliers), luck (Lucky, Unlucky, the coin), start rep (all offsets), scavenging counts.

**Minor/cosmetic**: Colorful Character (meta only), Tapehead, Secret Admirer, Tinkerer,
Illiterate (for our agent), Hoarder (temporary), the "operations visible" perks, UL Charity.

### 5.2 Clusters that behave alike

| Cluster | Members | One feature that summarises it |
|---|---|---|
| cash_rent | saving, bad_lease, prime_real_estate (rent part), wild_favorite, wild_bad_side, LLPILLAR, CARTEL_INCREASED_STIPEND | starting cash, weekly rent, upgrade-price multiplier |
| traffic | prime_real_estate, side_street, short_sleeper, LLREFERAL, LLPOORREVIEWS, UL_HOUSEHOLD_NAME, UL_TOURISTTRAP | expected normal customers/day |
| price | awful_negotiator, addictions, UL_TIPS, UL_TOP_REVIEW, SEC_RELIABLE_VENDOR, sycophant | counter price multiplier and budget multiplier |
| crime / inspection | intimidating, blabbermouth, LLTIGHTLIPS, SEC_TRUSTED/WATCHLIST/PRIORITY/THOROUGH/VIP, UL_PETITION | crime multiplier, evidence multiplier, inspection cadence |
| luck | lucky, unlucky, gambling_addiction | luck (−15…+20) |
| start_rep | well_connected, public_enemy, convict, model_citizen, sycophant, revolution_sympathizer, known_snitch | the day-1 rep vector |
| capacity | minimalist, ascetic, hoarder | stock grid width |
| scavenging | maimed, proficient, careful, osteoarthritis, agoraphobia | attempts/outing, wound chance, outing cadence |
| engine_grant | vending, working_pipes, handy, sawyer_crew, tinkerer | which engines/tools exist on day 1 |
| obligation | sentinel, blackmail, aug_menace, makeshift_reactor, addictions | a recurring must-do with a failure cost |
| content_removal | banned_commissary, known_snitch, maimed, illiterate, awful_negotiator, SEC_ALARM_IGNORED | a mechanic switched off |

Within a cluster, a policy that conditions on the summary feature generalises to unseen
members; across clusters the perks are close to independent (the only real couplings are
listed in 2.6).

### 5.3 Sampling scheme for training and league runs

Do not enumerate: ≈ 185 k legal ≤ 4-perk loadouts × 9 starts × 4 difficulties. Instead:

1. **Mix per batch**: 30 % no perks (the default and every first run), 50 % sampled
   loadouts, 20 % curated combos.
2. **Sampled loadouts**: draw 1–4 perks by rejection sampling under the real rules (budget ≤ 3,
   slots, incompatibilities). Weight rule-changing perks ×3, number-shifting ×1, minor ×0.3,
   so that every rule-changing perk appears in ≥ 8 % of perk runs. Colorful Character at
   ≤ 5 % (wide loadouts are rare and add no new rules).
3. **Curated combos** (one per row of 2.6, plus): Sentinel + Aug Menace (two death clocks);
   Awful Negotiator + an addiction (stacked price penalties, no haggling to offset); Ascetic +
   Vending + Working Pipes; Fence + Convict; Gunsmith + Model Citizen + Well Connected;
   Maimed + Makeshift Reactor (scrap with no dump: must buy scrap or die).
4. **Stratify** by start × difficulty so each rule-changing perk is seen on ≥ 3 starts and on
   both a gain-friendly (cozy/normal) and a loss-heavy (hard/challenge) difficulty.
5. **Reputation perks are state, not choices**: the policy meets high thresholds (SEC 80/100,
   UL 100, LL 100) late and rarely. In the simulator only, add a rep curriculum: start a share
   of runs (~15 %) with a randomised rep vector placed just below or above a threshold, so the
   policy sees VIP, Reliable Vendor, Rave Reviews, Most Wanted and the −100 floors early.
   Never do this in real-game league runs.
6. **Hold out** for evaluation: two curated combos and two rule-changing perks on one start
   each, never seen in training, to measure generalisation.
7. Always render active perks into the prompt (section 6); a policy that cannot see its perks
   cannot learn to condition on them.

## 6. Machine-readable catalogue

`internal/strategy/perks.json` (generated from the tables, ids and numbers cross-checked):

- `selection`: points 3, base slots 4, rules, mid-run changes.
- `reputation_rules`: range, relock rule, always-active and day-1 perks, game-over floors.
- `perks[]`: `id`, `kind` (`starting` | `reputation`), `name`, `polarity`, `cost` /
  `incompatible` / `extra_slots` / `mind_editable` (starting) or `faction` / `threshold`
  (reputation), `impact` (major | moderate | minor), `cluster`, `effect`, `numbers`,
  `strategy`, `fit` / `avoid` (archetype ids), `sites`, `visible`.

Loading: embed it like `catalogue.json` (`//go:embed perks.json` in `internal/strategy`),
unmarshal into a struct with those fields, index by `id`. Active starting perks come from the
bridge's run-start record (`ps.startingPerks` ids, docs/re/DAY.md); active reputation perks from
each faction's `unlocked_perks[]`. Render `name: effect` for each active perk, and for each
faction the next positive and next negative threshold with its `name` and `effect`.
