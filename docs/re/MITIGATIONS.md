# Loss-condition mitigations (RE notes)

Build 26525552 / 0.46D. Companion to the "Endings" table in `DAY.md`: for every way a run
can end, every way to **prevent**, **delay** or **reset** it. Same conventions as `STATE.md`.
Paraphrase only; evidence cells name the type and method (global namespace, `Assembly-CSharp`)
that implements the rule. Numbers come from the decompile plus the extracted tables
(`var/gamedata/upgrades.json`, `services.json`, `difficulty.json`, `reputation.json`,
`loc_en.json`, never committed). Display names are the English loc strings.

Difficulty columns are always `cozy / normal / hard / challenge`.

## 0. Shared machinery

### 0.1 Wild Network upgrades (`NetworkUpgrade`)

| Fact | Detail | Evidence |
|---|---|---|
| Where | Phone number 8376 ("browse Wild's services") opens the Wild UI; also opened by Wild's reminder/bailout visitors | `PhoneDialogList.PhoneWild`, `StoreClientListStory` |
| Price paid | `GetCost()` = base `cost` field × `upgradePriceMultiplier` (1 / 1 / 1.5 / 2) × 0.85 if LL rep ≥ 100 (perk `LLPILLAR`) × 0.9 with start perk "Wild's Favorite" × 1.1 with "On Wild's Bad Side", then rounded. The scaling steps below are applied to the stored `cost` field, so the multipliers compound on top | `NetworkUpgrade.GetCost` |
| Currency | **Wild's Favor first, then cash.** Button is enabled when `cost <= cash + favor` | `WildUIManager.OnUnlockClicked`, `WildNetworkUI.OnUseClicked` |
| `state` | **1 = purchasable (not owned)**, **2 = owned**. For a consumable "armed" means `state == 2`; using it flips it back to 1. (So "state 1 = bought and ready" is the reverse of what the code does.) | `NetworkUpgrade.IsReady/IsUnlocked/Unlock` |
| Cooldown | `cooldownDuration > 0`: on purchase `cooldownCurrent = cooldownDuration`; every day start decrements it (floor 0). `IsReady()` needs `cooldownCurrent == 0` **and** `state == 1`. The UI shows "Cooldown (N Days)" | `NetworkUpgrade.OnDayStart`, `StoreStation.StartDay`, `WildUIManager.OnElementClicked` |
| `heldUntilUsed` | If set, `Unlock()` leaves `state == 2` (armed, waiting to be consumed) instead of resetting to 1. Only `LANDLORD_GRACE` | `NetworkUpgrade.Unlock` |
| `isRepeatable` | Marker for the UI ("already bought" message instead of a button while armed). Only the two graces | `WildUIManager.OnElementClicked` |
| Prerequisites | `prerequisite` list of upgrade ids, all must be `state == 2`; UI says "This upgrade requires X." Tab "Advanced" is hidden until the property is paid off | `NetworkUpgrade.IsHavePrerequisite`, `IsHiddenForRun` |
| Wild's Favor sources | Trade: `tradeFavorRate` × original item value per trade (0.10 / 0.05 / 0 / 0). Static LL-rep gains (requests, chem and water feedback): **+40 favor per rep point**. Intro: 300 / 200 / 100 / 0 once. Bailout: +250 once when wealth is trending down (cozy and normal only). Nightly service fee refund (see fixers). UI: "Wild's Favor: N" | `StoreReputation.OnItemTraded`, `StoreReputation.ModReputation`, `StoreClientListStory`, `StoreClientManager.HandleWildBailout` |

### 0.2 Nightly services (`StoreService`)

Toggled in the end-of-day tablet; fee is charged at the start of the night for every service with
`unlocked && isUsing`. Fixers, referrals, insurance and criminal network refund
`wildServiceFavorRefund` of the fee as Wild's Favor (0.75 / 0.5 / 0.25 / 0.15). Evidence:
`EndOfDayUIManager` toggle, `PlayerStore.EndNight` loop.

### 0.3 Security record (what the player can read)

`SecData` drives it; the dossier holds the security intel document from the start on every
scenario (`NewGameData`, `intel_sec`), and `RecordUIManager` shows: evidence "n/100" (only when
total suspected crime ≥ 100, otherwise N/A), projected sentence (≥100 fine, ≥300 incarceration,
≥1500 execution), operation phase (investigation < 90, planning 90-99, execute sentence at
100), case officer name by crime bracket, and the list of crimes with amounts. A night-log line
fires when evidence > 90 ("an active investigation ... is almost complete") and when total
trafficking crime > 1000 (sting warning). The rent reminder ("N Credits in rent is due ...") shows
at the start of day for rent days 0-2.

### 0.4 Evidence mechanics every security section relies on

| Rule | Value | Evidence |
|---|---|---|
| Crime sources | Contraband sold (`CONTRABAND_ITEM_TAG`, value = negotiated price, typed by item class), stolen goods sold or delivered (value × stolen-value %), bribery, fraud/endangerment trackers (dirty or mislabeled water, bad chems, bad ammo), manslaughter (shooting a non-aug), selling to the Business Permit official's refusal, operating without a permit | `SecData.CheckSoldContraband/CheckDeliveredStolen/CommitCrime`, `PlayerStore.CheckCrime` |
| Faction discount at the sale | Customer faction scales the crime written: Black Market ÷2, Revolution ÷3, Lower Level ÷2 only with perk `LLTIGHTLIPS` (LL rep ≥ 80), **Cartel 0**, others ×1. Clients flagged `isNoContributeToEvidence` add nothing. Start perks: Intimidating ×0.67, Blabbermouth ×1.33 | `SecData.ApplyCrimeModifier`, `CommitCrime` |
| Natural decay | Each morning, **every crime type loses 20, but only while evidence < 80.** At 80+ crimes stop decaying | `SecData.OnNewDay` |
| Evidence gain per morning (by total suspected crime) | <100: 0; 100-299: +2; 300-849: +6; 850-1199: +9; 1200-1499: +12; 1500-2499: +20; ≥2500: +33. Then: perk `SEC_TRUSTED` ÷2; `SEC_PRIORITY` ×1.5 (else `SEC_WATCHLIST` ×1.2); perk `UL_PETITION` ×1.5 | `SecData.OnNewDay` |
| Evidence never falls on its own | Only `EVIDENCE_REMOVAL`, paying a fine (`ResetCrime`) or a debug path lower it. When total crime drops under 100 the gain stops, but the number stays put and resumes later | `SecData` (no other writer of `evidenceLevel`) |
| Days from 0 to 100 at a steady total | 50 / 17 / 12 / 9 / 5 / 4 (same brackets, no perks) | derived |
| At 100 | `SendOfficer()` queues the sentencing officer the same morning (if none pending and total crime > 0) | `SecData.OnNewDay/SendOfficer` |

---

## 1. Rent (`rent`)

Trigger: rent day is **every 7th calendar day (`day % 7 == 0`)**, not a countdown. Declining
to pay with no grace armed ends the run.

| Mitigation | id / how | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| **Rent Grace** | `LANDLORD_GRACE` | Armed grace turns "don't pay" into a deferral: this week's rent is added to `temporaryRent` and owed with next week's (landlord also dismisses you). Rent still grows by the difficulty multiplier (1.025 / 1.05 / 1.1 / 1.1 per week, applied whether paid or not) | Base 100, **doubles after every purchase** (100, 200, 400, 800 ...), then the difficulty multiplier. At normal: 100 / 200 / 400. Hard ×1.5 (150 / 300 / 600) | One use. Armed = `state 2`. Using it sets `state 1`. 15-day cooldown starts **at purchase**, so buying it and burning it within 15 days locks the re-buy until day 15. **The buyout never collects the arrears:** `BuyOutStore` charges only the price, so rent deferred on the last one or two rent days before a buyout is never paid (lab) | None | `NetworkUpgradeList.UnlockActions`, `PlayerStore.OnRentNotPaid`, `StoreClientList.CreateLandlordClient` |
| Free starting grace | difficulty rule `landlordGraceOwnedAtStart` | Cozy and normal begin with grace already armed and **cost not doubled** (first re-buy is 100). Hard and challenge begin without it | free | as above | none | `StoreService.ApplyDifficultyStart` |
| Visible tell | landlord greeting | With **no** grace armed the greeting threatens eviction if the rent is not there; with grace armed it is a plain question about the rent | free | none | none | `StoreClientList.CreateLandlordClient` |
| Pay | landlord choice 1 | Pays `rentValue + temporaryRent`, clears the backlog. Choice disabled when cash is short | cash | none | cash ≥ rent + backlog | `PlayerStore.CanPayRent/OnRentPaid` |
| Cash for rent day | Rent-day **Merchant** visitor | A buyer (budget about 200-300 + 3 × day) comes on every rent day | none | 3-day client cooldown | none | `StoreClientManager.CheckRentDay`, `StoreClientList.CreateMerchant` |
| Scav loan payback | Georgio story loan | If you lent him cash (25% or 50% of your credits at the time), he repays **4×** either 25+ days later, or on a rent day when cash < rent and more than 5 days have passed | the loan itself | once | take the loan offer | `StoreClientUniqueList` loan and payback clients, `StoreClientManager.CheckRentDay` |
| Wild bailout | free favor | +250 Wild's Favor once (cozy/normal) when wealth has trended down over 8 days; favor pays for upgrades, so it can fund a grace | free | once | Wild phone state "regular" | `StoreClientManager.HandleWildBailout` |
| Buyout | landlord choice 3 (**win**) | Pays the property price in cash and ends rent for good (`buyout`, may continue play) | 12 000 / 12 000 / 24 000 / 36 000 base + after day 50: 25 / 50 / 50 / 100 per day, capped at +3 000 / 5 000 / 10 000 / 20 000 | n/a | cash ≥ price | `PlayerStore.GetStoreBuyPrice/BuyOutStore` |
| Mortgage | Loan shark offer | Stops rent; replaces it with a weekly payment (section 2). Offered once cash ≥ 2 × down payment | down payment max(500, 20% / 15% / 12.5% / 20% of price rounded to 500) | n/a | loan shark seen, cash ≥ 2 × down | `StoreClientManager.CheckRentDay`, `PlayerStore.TakeMortgage` |

Notes
- Missing rent with grace does **not** lower the debt: `temporaryRent` accumulates (one missed week doubles the next payment), so the realistic window is "miss once, pay double next week".
- A second miss before you re-arm grace is fatal. Re-buying costs double each time, so arm it again the same week.
- Keep at least `rentValue + temporaryRent` in cash on every `day % 7 == 0` morning.

## 2. Repossession (`repo`)

Only exists while paying a mortgage. Weekly payment at the loan shark's collection visit
(same `day % 7 == 0` schedule).

| Mitigation | id / how | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| Pay the installment | loan shark "pay" choice | Decrements weeks left; paying the last week ends the run as a **win** (`buyout_mortgage`) | `ceil((loan + loan × rate% / 52 × weeks) / weeks)`, rate 25 / 50 / 75 / 100 %, weeks 8 / 12 / 16 / 16 (integer division; e.g. normal, day ≤ 50: loan 10 000 after a 2 000 down payment ≈ 930 per week) | none | cash ≥ payment | `StoreClientList.CreateLoanSharkCollect`, `PlayerStore.PayWeeklyMortgage/CalculateWeeklyPayment` |
| **Stock as a buffer: skip and be repossessed** | loan shark "no" choice | If your repo-eligible items are worth at least one payment, the shark takes a best-fit set of items worth about one payment, leaves a Repo Slip, resets the countdown to 7 days, and **the run continues**. The payment is not credited (weeks left unchanged) | the seized goods | repeatable while stock lasts | sum of eligible item values ≥ weekly payment | `PlayerStore.IsEnoughForRepo/Repo` |
| What is *not* seizable | item placement | Excluded: documents, containers, machines, locked or behind-lock items, anything inside a Smuggler's Bay, items tagged important, zero-value items, Lou's toy box contents. Putting stock outside these classes keeps the soft-fail available; hiding it in a bay removes it | none | none | none | `PlayerStore.IsEligibleForRepo`, `GameItem.IsRootedInSmugglerBay` |
| Georgio payback | see section 1 | 4× loan return when cash < payment (same check runs for mortgage days) | the loan | once | loan taken earlier | `StoreClientManager.CheckRentDay` |
| Buy out before it bites | landlord path only | Not possible once on a mortgage (`CanBuyOutStore` needs the property unowned and no mortgage routing) | n/a | n/a | n/a | `PlayerStore.IsPayingRent` |

If eligible stock is below one payment the "no" choice is a hard game over. Mortgage payments
are far larger than rent (about 4-5 weeks of rent at normal), so the mortgage trades a
slow rent bleed for a short high-stakes schedule that ends in a win.

## 3. Arrest and execution

Two independent paths: the **investigation** (evidence reaching the sentencing officer) and
**inspections** (contraband found at the counter).

### 3A. Investigation: sentencing officer (`arrest`, `execution`)

On arrival the officer re-reads the record. If evidence < 95 **or** total crime ≤ 0 it is
"evidence gone" (no sentence, Security rep -20, state reset). Otherwise total crime decides:
< 300 fine (= the total, rep -10, **resets all crime and evidence**; if cash < fine it is `arrest`),
300-1499 `arrest`, ≥ 1500 `execution`. There is no clemency branch here.

| Mitigation | id | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| **Evidence Removal** (display "Evidence Removal") | `EVIDENCE_REMOVAL` | Sets evidence to 0 and the case state to "no officer". **Does not touch the crime amounts**, so evidence regrows by the bracket rate. If the officer is already queued he arrives, sees evidence < 95 and leaves ("evidence gone"), costing Security rep -20 | Base 500, **+100 per purchase** (500, 600, 700 ...), then difficulty × (normal 500 / 600 / 700; hard 750 / 900) | Instant effect. Auto-returns to purchasable; **14-day cooldown**; no held state | None | `NetworkUpgradeList.UnlockActions`, `NetworkUpgrade.Unlock`, `StoreClientListSecurity.CreateSecurityOfficer` |
| Wild's Fixers | `WILD_FIXER` + nightly service `WILD_FIXER` ("Wild's Fixers") | Each night, **per crime type**, subtract a percentage then a flat amount: tier 1: 15% + 50; tier 2: 20% + 150; tier 3: 25% + 400 (floor 0). Works at any evidence level (natural decay stops at 80, this does not). Applied at night, before the next morning's evidence tick | Upgrade 350 / 450 / 550 (tiers 1/2/3, each requires the previous) then difficulty. Service fee per night 25 / 45 / 70 (tier-dependent), refunded as favor 75% / 50% / 25% / 15% | Permanent; toggle the service each night. No cooldown | tier n needs tier n-1; service needs tier 1 | `NetworkUpgradeList` fixer constants, `SecData.OnFixerUsed`, `PlayerStore.HandleOtherServices`, `StoreService.UpdateCost` |
| Wild's Fixers II / III | `WILD_FIXER2`, `WILD_FIXER3` ("Wild's Fixers II/III") | As above, stronger. A type with ≤ 533 at tier 3 is wiped in one night; 1 000 becomes 350 | see above | see above | see above | same |
| **Keep the total under 300 and pay the fine** | gameplay | A sentence at < 300 costs the total (< 300 credits), Security rep -10, and wipes crime and evidence. Fixers or faction choice at the sale keep totals in this band | cash only | n/a | cash ≥ fine | `StoreClientListSecurity.GraftFineDialogue` |
| Sell to discounted factions | choice of customer | Black Market ÷2, Revolution ÷3, Cartel 0, LL ÷2 at LL rep ≥ 80, `isNoContributeToEvidence` customers 0 | none | n/a | rep thresholds | `SecData.ApplyCrimeModifier` |
| Security rep perks | rep | `SEC_TRUSTED` (≥ 60): evidence gain ÷2. Conversely `SEC_WATCHLIST` (≤ -20) ×1.2 and `SEC_PRIORITY` (≤ -60) ×1.5 | n/a | n/a | rep | `SecData.OnNewDay` |
| Weapon permits | `GUN_PERMIT_I/II/III` ("Weapon Permit I-III") | Covered firearms stop being contraband (no crime on sale, not found at inspection). Delivered once Security rep ≥ 20 / 50 / 80; **revoked** if rep falls under 0 / 30 / 60 | 300 / 600 / 900, then difficulty | permanent | previous permit | `PermitHelper.OnPermitGranted`, `StoreClientListSecurity.CreateRevokePermitOfficer` |
| Business permit | permit official visitor (days 24+, weekly after 38) | Buying removes the weekly "operating without a license" crime (each refusal after the first visit adds 50 + 20 × ⌊day/7⌋, uncapped: 170 on day 42, 330 on day 98) | 500 / 1500 / 2000 / 3000 (vendor permit fee) | one-time | cash | `StoreClientList.CreateBusinessPermit`, `Difficulty` |
| Permit official donations | goods with the "donation" feature | Handing over goods worth the accumulated bribe amount (25 + 10 × ⌊day/7⌋ added each visit from day 45, uncapped: 85 on day 45, 155 on day 94, 285 on day 185; 960 in total over days 45–94 against a 2000 permit on hard; lab, docs/sim/VERIFIED.md "Formula exploits"; min 10) lets that week pass without the refusal crime | goods | per week | official visit | `StoreClientList.CreateBusinessPermit` |
| Evidence locker event | day-7 scripted event (not on skip-intro) | A thief sells a stolen evidence crate. **It does not reset the investigation.** It is a trade: return to Security (+10 Security rep, +200 credits base) or sell to the Black Market (+5 BM rep) | the crate price | once | none | `StoreEventManager.QueueEvent`, `StoreRequestList.SecEvidence/BMEvidence`, `StoreClientUniqueList.UniqueEvidenceSeller` |

How the numbers play out: `EVIDENCE_REMOVAL` alone buys about `100 / gain` days where gain is the
bracket rate (≥ 2 500: 4 days; 1 500-2 499: 5; 1 200-1 499: 9; 850-1 199: 12; 300-849: 17).
With a 14-day cooldown it only covers a full cycle when total crime is < 850; above that pair it
with fixers. Fire it when the night log says the investigation is nearly complete (evidence > 90) and
before the officer is at the counter. Once an officer is queued, the removal still defuses it
(evidence < 95 on arrival), at the price of Security rep -20.

### 3B. Inspections (`arrest` via fine or critical contraband)

Countdown exists only when the **sum of trafficking-type crimes ≥ 50** and you do not hold
`SEC_VIP`. Spacing by that sum: < 150: 3-5 days; 150-299: 3-4; 300-449: 3; 450-599: 2-3; 600-749: 2;
≥ 750: 1-2. `SEC_TRUSTED` adds +2/+4 days, `SEC_THOROUGH_INSPECTION` (≤ -80) subtracts 1.
Personality: random normal / shady / lazy, forced normal once bribery crime ≥ 500
("Reliable Inspector Only"). A **display case holding serious contraband or stolen goods summons an
extra inspector the same day** (`HandleShowCaseSecurity`).

Result is decided by the worst item found: low = warning (items kept), mid or stolen = seizure,
**high = fine** (sum of item values; if cash < fine and no clemency, `arrest`), **critical =
`arrest`** unless Clemency is armed. Security rep falls by 1 per item plus its level (capped at 25).
A clean inspection gives +3 Security rep.

| Mitigation | id | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| **Clemency** | `SEC_GRACE` | Armed: a **critical** find (or a **high** find you cannot pay) ends in a warning instead of arrest. Contraband is still confiscated and the rep penalty still applies; it is not consumed by any other result | Base 250, **+100 per purchase** (250, 350, 450 ...), then difficulty | One use: `state 2` armed, set to 1 when used. No cooldown field | None | `NetworkUpgradeList.UnlockActions`, `StoreClientDialogList.SecurityDialogInspectionGrace/Fine`, `ContrabandHelper.StartInspection` |
| Inspection Early Warning Service | `INSPECTION_INFORMANT` + nightly service `SEC_INFORMANT` ("Inspection Early Warning") | Night log warnings at 3, 2 and 1 days (text: "in 2 days", "tomorrow", "imminent"). **Does not cover surprise display-case inspections.** Not needed (and disabled) with `SEC_VIP` | Upgrade 300 then difficulty; fee 10 per night | service toggle; no cooldown | None | `PlayerStore.HandleInspectionEarlyWarning`, `StoreServiceSheet` |
| Bribe the shady inspector | choice in the inspection dialogue (only personality 1) | Pay `clamp(100 + 2 × day - 2 × SecRep, 40, 400)`; the inspection **does not happen**. Cost side: adds to the bribery tracker (decays 5 per day); each time it reaches 250, 250 bribery crime is written (evidence input), and bribery crime ≥ 500 stops shady/lazy inspectors from being rolled | that amount | any time a shady inspector comes | cash ≥ amount | `StoreClientDialogList.SecurityDialogInspectionLastLine`, `ContrabandHelper.GetBribeAmount`, `PlayerStore.CheckCrime`, `SecData.GetInspectorPersonality` |
| Smuggler's Bay | items `smuggler_bay`, `smuggler_bay_mod`, `smuggler_bay_mini` ("Smuggler's Bay", "(Modified)", "Mini") | Inspectors skip the contents entirely (counted as concealed; no seizure, no rep change). Also exempt from repossession. The modified bay comes from an Exchange barter that consumes a plain bay | item price (inventor stock line is chance-based; some starts begin with one) | permanent | own the item | `ContrabandHelper.IsBlockSearchContainer/IsSmugglerBay`, `ExchangeList.SmugglerBarter`, `StoreClientList` inventor stock |
| Hidden stash, locked items | item and slot tags | Items that are hidden, locked or behind a lock are not inspected; the poster-covered stash is searched **only if the wall poster is pulled**. A lazy inspector also skips the trash can | none | none | the furniture | `ContrabandHelper.InspectItem/StartInspection` |
| Luck | start perk "Lucky" (+15), a carried gambler's coin (+5), **liquid luck** (+75 while active) | On a bad find one luck roll (luck %) clears the **entire** found list, and the clean-inspection +3 Security is withheld. Liquid luck: each bottle drunk adds 3 days (stacking), the +75 lasts while any day is left and ticks down at sleep; measured 74.1 % of finds voided at luck 75 (n 4 000; docs/sim/VERIFIED.md) | liquid luck: Georgio gives one free and later sells three at −20 % wholesale (`StoreClientUniqueList.CreateGeorgioLuck*`) | bottle consumed | perk / item | `HealthData.GetLuck`, `RollLuck`; `LiquidLuckHelper.Drink`, `GetActiveLuckBonus`, `OnSleep`; `LotteryHelper.GetCarriedLuckBonus` |
| Weapon permits | `GUN_PERMIT_I-III` | Covered firearms are not contraband | see 3A | permanent | see 3A | `PermitHelper.OnPermitGranted` |
| VIP | rep `SEC_VIP` (≥ 100) | No inspections at all and no countdown | rep | permanent while rep ≥ 100 | Security rep 100 | `StoreClientManager.HandleInspectionClient` |
| Keep serious contraband off the display case | gameplay | Avoids the extra same-day inspector | none | n/a | none | `StoreClientManager.HandleShowCaseSecurity` |
| Fixers | `WILD_FIXER*` | Drop the trafficking total under 50 and the inspection countdown switches off | see 3A | nightly | see 3A | `StoreClientManager.IsInspectionCountdownActive` |
| Pay the high fine | cash | If cash ≥ fine the only damage is the fine, seized goods and Security rep | fine = sum of found item values | per inspection | cash ≥ fine | `StoreClientDialogList.SecurityDialogInspectionFine` |

Investigation-path "report" risk: a wrongful report costs Security rep -8 and a 50-credit fine.
The wrongful-report counter drains 1 every 5 days; the "next report leads to arrest" text has no
game-over code attached (unverified, inferred from `wrongfulReportMax` having no other reader).

## 4. Faction reputation at -100

Reputation is a double clamped to ±200 with no passive decay. A perk is active when
`amount <= threshold` (negative perks), re-evaluated on every change, but the death visitors are
checked **once each morning when the shutter opens** (`OnLateShutterOpen`). So the rep must be above
-100 at the moment you open the shop. Loss multipliers: `reputationLossMultiplier` 0.5 / 1 / 1.25 / 2;
gain multipliers 1.5 / 1 / 0.75 / 0.5; Black Market losses ×2; Security losses ×2 when `UL_PETITION`
(UL ≤ -60) is active.

| Faction | -100 perk | Ending | Warning ladder (negative perks) |
|---|---|---|---|
| Lower Level | `LLLYNCH` "Mob Justice" | `mob` | -20 Poor Reviews, -40 Mistrusted (no contraband from LL sellers), -60 Marked (theft, insurance ×2), -80 Shunned (suppliers refuse) |
| Security | `SEC_IMMEDIATE_ARREST` "By Any Means" | `sec` | -20 Watchlist, -40 Must Be The Wind, -60 Most Wanted, -80 No Stone Unturned |
| Revolution | `REV_HIT` "Hit list" | `bombed` | -40 Blacklisted |
| Cartel (drug-dealer start only) | `CARTEL_RETIRED` "Forced Retirement" | `cartel_retired` | none |
| Upper Level | `UL_HITMAN` "Hired Hitman" (defined in `StoreReputationPerk`, but **not in the Upper Level perk list** `StoreReputation.CreatePerks`, so it can never be created: the `hitman` ending is unreachable in this build; inferred) | `hitman` | -40 Tourist Trap, -60 Petition to Security |
| Black Market | none at -100 | n/a | -40 Blacklisted (no contraband sales to you, exchange barred, janitor ×2), -60 Uninsurable |

### 4.1 Repair routes (positive deltas available in code)

| Faction | Routes (rep points before multipliers) | Evidence |
|---|---|---|
| Lower Level | Trade with LL customers: each trade adds `value / 400 × 0.66` (so about 1 point per 600 credits); marketplace/story requests: meteor 10, soup kitchen 8, medical shortage 8, construction 6, small ones 1-2; correctly fixed chems and water for LL buyers +3. Every static LL gain also grants **+40 Wild's Favor per point**. Avoid reporting LL sellers (each report costs -8, worsening by 2 to -20) and failed blackmail/threats (-5 to -10) | `StoreReputation.OnItemTraded`, `StoreRequestList`, `ChemicalProductHelper`, `WaterFeatureHelper`, `StoreReputation.ReportedClient`, `BargainUIManager` |
| Security | Clean inspection +3; report a wanted customer +5 (scaled by Convict / Model Citizen perks); report a contraband or stolen seller +item level (and cash); shoot a real aug +8; Security requests (evidence crate +10, retirement party +2); trade with Security customers | `ContrabandHelper.StartInspection`, `PlayerStore.GrantWantedCaptureReward/CreateStolenReportReward`, `AugHelper.OnGoodKill` |
| Revolution | Revolution requests (oxymore 5, scrap metal 5, security card 6, peat 4); shoot a real aug +2; scripted story steps (+12 to +15); trade with Revolution customers. Shooting innocents -10 | `StoreRequestList`, `AugHelper`, `SerpicoStoryline`, `StoreClientListWanted` |
| Black Market | Black Market requests (evidence crate 5, construction 3, medical 3); the Criminal Networking service grants a small BM rep each use (3-day service cooldown); aug kill +2; trade | `StoreRequestList`, `NetworkUpgrade` CRIMINEL_NETWORK, `AugHelper` |
| Upper Level | Requests (rare item 2, charity 5, pet meat 2); trade (×1.2, and ×1.25 with Sycophant); machine alarm tick +1 | `StoreRequestList`, `StoreReputation` |
| Cartel | Weekly deliveries: meeting the required count gives + (count); short delivery costs -4 per missing unit; purity band: ≥ required+10 gives (excess / 4), required-5 to +10 neutral or slightly negative, below that negative by the gap; **average purity ≤ required / 4 (≤ 10 when 40 is required) is an immediate `cartel_retired`**; requesting extra materials costs rep (-6 when "reliable", more otherwise) | `StoreClientListCartel` |

Insurance policy for this section: read the faction bars (`F2/200`) every morning; the
death perk is two bars away from the ladder's last warning at -80 for LL and Security. A day with
a lot of negative-rep events (reports, failed threats, innocent shots -15/-10/-10/-15 for
Sec/Rev/BM/LL) needs a repair action *before* sleeping.

## 5. Aug presence (`aug`)

Game over fires at **end of day** (on sleeping) when `augPresence >= 100` **and** difficulty
`augGameOverAtFullPresence` (hard, challenge) **or** the start perk "Aug Menace" (every
difficulty). Presence starts at 15. Augs first appear on day 12 (day 1 with Menace), with two
scripted visits, then one every 6-8 days.

| Mitigation | id / how | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| **Kill a real aug** | shoot at the counter | Presence **-25 / -25 / -10 / -8** (floor 0); pays 100 + days since intro (cap 250) in credits (×1.25 Menace); +2 rep with Rev, BM, LL; +8 Security | a shotgun (given at day 12, or with a weapon permit I) | per aug | the shotgun unlocked | `AugHelper.OnGoodKill`, `Shotgun.Unlock` |
| Aug Scanner | item `aug_scanner` ("Aug Scanner") | 100% accurate "AUG DETECTED / No Aug detected" readout at **25 energy per scan** | item price (offered by a seller chance when augs are active) | battery | owned battery | `AugScanner`, `StoreClientList` |
| Don't let one walk | gameplay | Each aug dismissed alive adds **+10 / +10 / +15 / +16** presence; the night log reports it | n/a | n/a | n/a | `PlayerStore.DismissCurrentClient`, `AugHelper.OnAugLeftAlive` |
| Don't shoot innocents | gameplay | A non-aug shot adds manslaughter crime 250 (before day 25) / 500 / 750 (day 50+) and rep Rev -10, BM -10, LL -15, Sec -15 | n/a | n/a | n/a | `AugHelper.CleanupKill` |
| Early warning | difficulty `augWarning` (cozy, normal) | A night-log line on the evening before a spawn day (except the first scripted days) | free | none | none | `PlayerStore.HandleAugWarning` |
| **All clear** (ends augs for the run) | `IsAugAllClearDue` | When day ≥ intro + 50, presence == 0, you own a business permit, and ≥ 25 days have passed since you bought it, a Security officer announces the all-clear and spawns stop. Not available with Menace | permit price (section 3A) | permanent | permit, presence 0 | `PlayerStore.IsAugAllClearDue`, `StoreClientManager.HandleAugClient` |

Pacing: from 15 you need about 6 un-killed augs on hard (85 / 15) before presence hits 100,
roughly six weeks; every kill buys back 8-25 points.

## 6. Sentinel collapse (`sentinel`)

Only with the start perk "Sentinel Collapse Disorder" (not editable by the mind reprogrammer). The
pool is permanent damage; **it is never reduced**. The run ends the night the pool reaches 20.

Nightly counter `daysSinceSentinelInjection`: ≥ 6 "mild" warning (no damage); ≥ 7: pool +1;
≥ 10: +2; ≥ 14: +3. From a fresh injection with no further dose the pool hits 20 on night 16 after it.
A night-log warning that one more late injection will be fatal appears when `pool + next damage >= 20`.

| Mitigation | id / how | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| **Genuine Immunivax Injector** | item `large_purple_injector` ("Immunivax Injector"), use from inventory | Resets the counter to 0 | item price | consumed | own a genuine one | `HealthData.ConsumeImmunivax` |
| Expired injector | same item, expired | Resets the counter to **3** (so it is due again 3 days sooner) | cheap | consumed | | same |
| Counterfeit injector | same item, counterfeit | Resets to a random 3-5 | cheap | consumed | | same |
| Unusable injector | same item, unusable | **No reset** | | | | same |
| Scalper | Black Market visitor every 7th day (`day % 7 == 6`) | Always offers one **genuine** injector, with a markup of `scalperMarkupBase + scalperMarkupPerDay × days since first visit` (15 + 1/day at normal) | rises daily | weekly | Sentinel perk | `StoreClientManager.HandleScalperClient`, `StoreClientListSpec.CreateScalper` |
| Starting injector and gift | scenario | One genuine injector at start; a one-time free genuine injector from the "Secret Admirer" perk if you hold no usable one | free | once | perk | `NewGameData`, `StartingPerkContent` |
| Random supply | pharmacist, scav sellers, supplier stock | Random injectors: 42% genuine, 20% expired, 20% unusable, 18% counterfeit; inspect with the magnifier (see `INSPECT.md`) | item price | | injector introduced | `PreBuiltItemHelper.CreateRandomInjector` |

Operating rule: inject on or before the "mild" warning (day 6); genuine every 6 days is the
steady state; keep one spare genuine in stock (supply is weekly via the scalper). Do not use
expired stock as the only plan, since it resets to 3.

## 7. Reactor meltdown (`reactor`)

Only with the start perk "Makeshift Reactor". At every night start each reactor in the store loses
`max(0, 18 - upgrades)` integrity; if it reaches **0** it explodes at the day start. It powers
the store while integrity > 25. The night log warns "too damaged to power" (≤ 25) and "will explode
during the next night" when `integrity - decay <= 0`. Tooltip shows integrity, decay per night,
upgrades and charge.

| Mitigation | id / how | Effect | Cost & scaling | Consumable / cooldown | Prerequisite | Evidence |
|---|---|---|---|---|---|---|
| **Scrap Metal** | item `scrap_metal`, drop on the reactor | +60 integrity (capped at max integrity) | one scrap | consumed | integrity < max | `ReactorHelper.RepairWithScrap` |
| Junk | item `junk` | +30 integrity but **max integrity -5** each time | one junk | consumed | integrity < max | `ReactorHelper.RepairWithJunk` |
| **Electronic Parts (Common)** | item `common_electronic` | +1 upgrade (max 18): decay falls 1 per upgrade; **at 18 upgrades decay is 0 and the reactor cannot explode**. Charge per night = min(4, 1 + upgrades / 5). Tinkerer perk: 20% chance the part is kept | one part each | consumed | upgrades < 18 | `ReactorHelper.Upgrade/GetDecay/WillExplodeNextNight` |
| Replacement reactor | inventor stock | If you own none, the inventor offers a stock reactor at 150 credits (fresh integrity 100) | 150 | when you hold none | | `ReactorHelper.TryOfferStockReactor` |
| Timing | gameplay | At 100 integrity and decay 18, integrity lasts 5 nights (100 → 10) and the sixth night explodes; repair when `integrity <= 2 × decay` | | | | derived |

Radioactive waste appears every 160 integrity lost (`item_nuclear_waste`), not part of the
loss condition. Explosion is checked before the aug ending in `EndOfDayUIManager`.

---

## 8. Insurance policy: what a careful player keeps ready

Risk level is judged by total trafficking crime (visible in the security record), cash buffer and faction bars.

### Stage by stage

| Stage | Always | Risk: low (honest trade) | Risk: medium (some contraband/stolen, ≥ 300 crime) | Risk: high (≥ 850, or rep ≤ -60 anywhere) |
|---|---|---|---|---|
| **Days 1-12** | Cash ≥ rent + backlog on every day divisible by 7. Know whether grace is armed (greeting tell). On hard/challenge buy `LANDLORD_GRACE` first (150 / 200 + favor). Bank Wild's Favor (it pays for upgrades) | Nothing else | `SEC_GRACE` (250), sell contraband to BM / Rev / Cartel clients | `INSPECTION_INFORMANT` (300), hidden stash or Smuggler's Bay, `WILD_FIXER` 350 |
| **Days 12-40** | Re-arm `LANDLORD_GRACE` the week after using it (price doubles). Watch augs: shotgun, scanner, never let one leave | Keep an aug-kill rhythm | `WILD_FIXER` + service nightly, `EVIDENCE_REMOVAL` off cooldown | `WILD_FIXER2`, `EVIDENCE_REMOVAL` ready, cash ≥ 1 fine, keep Security rep > 0 for permits |
| **Day 24+** | Business permit (removes the weekly crime; required for the aug all-clear) | Buy it | Buy it | Buy it; also gates Advanced tab content after buyout |
| **Day 40+, rising rent** | Consider buyout when cash covers the price (ends rent risk, wins). Mortgage only with a stock reserve ≥ one weekly payment of repo-eligible, unhidden items | Buyout path | Buyout path | Fixers at tier 3 (`WILD_FIXER3`), `SEC_GRACE` re-armed after use |
| **Anytime** | Read the four bars: any faction ≤ -80 needs a repair action today. Sentinel: inject on day 6. Reactor: repair at `integrity <= 2 × decay`, keep upgrading | | | |

### Priority list by price/benefit (normal difficulty)

1. `LANDLORD_GRACE` 100 (free at start on cozy/normal): prevents the most common early ending. Never end a rent day without it armed or cash.
2. `SEC_GRACE` 250: one-off protection from a single critical find; arm before the first critical contraband is held.
3. `WILD_FIXER` chain 350 / 450 / 550 + 25-70 per night (half refunded in favor): the only mitigation that scales down the crime numbers every night; makes the sentence band a cheap fine instead of an arrest.
4. `EVIDENCE_REMOVAL` 500 (+100 each): the emergency reset when the night log says the investigation is nearly complete; keep it off cooldown if running ≥ 850 crime.
5. `INSPECTION_INFORMANT` 300 + 10 per night: lets you stash goods before a routine inspection (not the display-case one).
6. Smuggler's Bay / hidden stash: free after purchase, defeats inspections but also removes goods from repossession.
7. Business permit (500-3000): converts a recurring crime source into zero and starts the aug all-clear clock.

### Not mitigations

`HYPERTECH_SMUGGLER` ("Hypertech Smuggler", 2 500, Advanced tab) unlocks a nightly supplier
service (500 per use, 5-day cooldown) for high-tech items. It touches no loss condition. The evidence
locker crate does not reset the investigation. `CRIMINEL_NETWORK` only brings contraband buyers and
a little Black Market rep. The `hitman` ending cannot be reached (section 4).

## 9. Unverified and open

- Exact moment the sentencing officer arrives relative to the player's first chance to use the phone
  (he is queued in the morning with a 0-day delay; whether you can reach Wild's UI before he is at
  the counter was not exercised).
- Rent-day client ordering (Georgio payback before landlord) is inferred from `AddClient` order.
- Smuggler's Bay supply chances are read from a chance table and not sampled.
- Everything above is read from code and loc tables, not observed in play, except where `VERIFIED.md` says otherwise.

## Mortgage and Georgio's loan (exact arithmetic, verified against the gym's `risks.mortgage`)
| Fact | Detail |
|---|---|
| Price | Buyout price: 12,000 normal / 24,000 hard, +50 a day after day 50, capped at +5,000 / +10,000 |
| Down payment | price × 12.5 % rounded to the nearest 500 (banker's rounding: on hard day 90 gives 3,000, day 91 gives 3,500), at least 500 |
| Offer | Checked on rent mornings: cash ≥ 2 × the down payment, **and** the loan shark's intro already happened on an earlier day (it fires at the daily scripted-client check once cash ≥ one down payment). An intro on the rent morning itself gives no offer that week |
| Payments | 16 weekly payments: loan = price − down; interest = loan × 75 % / 52 × 16 (integer math); payment = ⌈(loan + interest) / 16⌉. Hard: 1,615 before day 50, 1,692 on day 70, 1,762 on day 98, ~2,000 by day 160, 2,307 at the cap |
| Repossession | A missed payment seizes stock worth up to one payment and resets the week, but doesn't count as a payment; with less repossessable stock than a payment the run ends |
| Georgio | Offered once on day 43; lend 25 % or 50 % of cash on hand; repays 4× on the first rent morning more than 5 days after the loan on which cash is below the rent (or the mortgage payment), before the landlord collects; otherwise automatically on day 69 |
| No bridges | No loan shark for rent; hard has no Wild trade favor and no Wild bailout |

## 10. Challenge against hard

Challenge is hard plus a harsher rules table and nothing else: no code path tests for the `challenge` id, and
everything gated on hard mode (`Difficulty.IsHardOrAbove`, `NewGameData.hardMode`) applies to both. So the
starting rent, the ×1.1 weekly rent growth, the missing free Landlord Grace, the missing Wild bailout, warnings
and reminders, aug game over at full presence, the 16-week mortgage term, no retail markup and ×2 power cost
are all the same as hard. What differs (hard → challenge; `DifficultyRules`, `Difficulty.CreateVanilla`):

| Rule | Hard | Challenge | Where it bites |
|---|---|---|---|
| Buyout price | 24,000, +50 a day after day 50, cap +10,000 | **36,000, +100 a day after day 50, cap +20,000** (40,000 on day 90, 45,000 on day 140, 56,000 from day 250) | `PlayerStore.GetStoreBuyPrice` |
| Mortgage | down 12.5 % of the price (to the nearest 500), 75 % a year | **down 20 %, 100 % a year**: day 84 offer is 8,000 down (so 16,000 cash on a rent morning to be offered) and 16 payments of 2,566; 9,000 down and 2,942 a week at day 140 | `GetMortgageDownPayment`, `CalculateWeeklyPayment` (interest = loan × rate / 100 / 52 × 16) |
| Insurance fee | (estimated value / 400 + 3) × 2, where the estimated value is **cash plus every owned top-level item** in the shop's inventories (`PlayerStore.GetTotalEstimatedValue`), fixed when the shutter closes; a 20 000 buyout fund alone makes it 106 a night; +4 on the starting fee | × **3**; +5 on the starting fee | `StoreService.UpdateCost`, `ApplyDifficultyStart` |
| Appraisal Board join fee | 85 × 2 = 170 | 85 × 3 = **255**; promotions 1.25 / 1.5 / 1.75 × that (319 / 382 / 446, rounded half to even) | `PlayerStore.GetAppraisalJoinCost` |
| Appraisal rank progress | gains × 1, losses × 1.5 | gains × **0.75**, losses × **2**: a third more good appraisals per promotion. The fees themselves do not scale | `AppraisalRankHelper.AddProgress` |
| Business permit fee | 2,000 | **3,000** | `vendorPermitFee` |
| Wild's network upgrades | price × 1.5 | × **2** | `NetworkUpgrade.GetCost` |
| Supplier markups | supplier base percent × 2; retired supplier +30 %; Dr. Jackson (inventor) +50 % | × **3**; +**45** %; +**100** % | `ItemFeatureList.SupplierMarkup`, `PremiumBuy15`, inventor markup feature |
| Scalper | 25 % + 2 % a day since his first visit | 25 % + **3** % a day | `StoreClientListSpec.GetScalperMarkup` |
| The merchant client buys from the shop at a discount | 20 % | **25 %** | `MerchantDiscountBuy` |
| Dr. Jackson's introductory discount (the furnace and module intro offers) | 35 % off | **10 %** off | `ItemFeatureList.FirstTimeDiscount`, `StoreClientListStory.CreateInventorIntro` |
| Faction reputation | gains × 0.75, losses × 1.25 | gains × **0.5**, losses × **2** | `StoreReputation.ModReputation` |
| Wild's Favor | intro 100; nightly service refund 25 % | intro **0**; refund **15 %** (no trade favor on either) | `networkIntroFavor`, `wildServiceFavorRefund` |
| Aug presence | kill −10, escape +15 | kill −**8**, escape +**16** | `AugHelper` |
| Lou's demand | 1,000 (minimum half) | **1,500** (minimum 750) | `StoreClientData.GetLouDemandValue` |

The price is the change that matters. The hard policy's cash buyout (~26,000 by day 98) becomes ~40,000 on
challenge, so the same income that buys the shop for cash on hard can only reach the mortgage offer
(16,000 cash), and the mortgage is 16 payments of ~2,600 instead of ~1,760.
