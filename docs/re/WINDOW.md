# The window (display case): rules from the decompile

Build 26525552 / 0.46D. Same conventions as `DAY.md`. Types are in the global namespace of
`Assembly-CSharp`. "Showcase" = `EmporiumEntry.showcaseElement : GameGridInventory`.

Everything below is read from code; nothing here is play-tested. Items marked (inferred) are
conclusions from absence of code, not a quoted rule.

## Rules

### Capacity and placement

| Rule | Value / formula | Evidence |
|---|---|---|
| Base grid | 7 x 5 cells | `EmporiumEntry` ctor builds `GameGridInventory(7, 5)`; consts `SHOWCASE_X/Y` |
| Upgrade I (`SHOWCASE_I`) | 8 x 6, costs 200, `+1` display buyer slot | `EmporiumEntry.UpgradeDisplayCaseI`; `NetworkUpgradeList.CreateShowcaseI`, `SHOWCASE_CUSTOMER_INCREASE`; `PlayerStore.GetShowcaseTypeVisitorCount` |
| Upgrade II (`SHOWCASE_II`) | 9 x 7, costs 500, needs I, `+1` buyer slot | `EmporiumEntry.UpgradeDisplayCaseII`; `NetworkUpgradeList.CreateShowcaseII` |
| Which items fit | Any item the player owns (not customer-held items); no type filter. Footprint is the usual grid footprint | `EmporiumEntry.CanInsertIntoPlayerInventories` -> `GeneralHelper.IsItemOwned`, wired as `showcaseElement.mayInventoryAddItemFunc` |
| When it can change | Any time, open or closed. The only hook is `PlayerStore.AddedItemToDisplayCase` / `RemovedItemToDisplayCase`, which just refresh the clientele UI. Nothing locks the grid when the shutter opens | `EmporiumEntry` slot callbacks; `PlayerStore.AddedItemToDisplayCase` |
| But it only matters at open | The forecast is consumed once, at shutter open. Edits after that affect only the next day. The game shows a one-time tip saying so (`mech_display_case_timing_tip`) if you add an item while OPEN | `PlayerStore.OnShutterOpened` -> `StoreClientManager.GenerateClient`; `PlayerStore.ShowDisplayCaseTimingTipOnce` |
| Persistence | Contents are saved and loaded with the store; they carry over between days | `PlayerStore` `showcaseInvJSON` (save/load) |

### Display-type buyers ("merchandise customers"): the main mechanic

| Rule | Value / formula | Evidence |
|---|---|---|
| Rolled when | Once per day inside `GenerateClient`, at shutter open (`HandleShowcaseClient`). Not at day rollover, not per customer | `PlayerStore.OnShutterOpened`; `StoreClientManager.GenerateClient` order |
| Skipped when | Store attractiveness `< 100`, or (intro mode and day 1) | `StoreClientManager.HandleShowcaseClient` |
| Number of buyers | `GetShowcaseTypeVisitorCount()` = `1` + `1` if the Buy Referral service is active + `1` per owned Showcase upgrade (I, II) `- 1` per vandalism-affected tier, and `- 1` more if the store is vandalized; floor 1. Max 4 while clean | `PlayerStore.GetShowcaseTypeVisitorCount`; `StoreService.GetBuyReferalService` |
| Which types | Rank item types in the case by the sum of `GetNegociatedValue()` (the displayed/claimed price, so fake-until-exposed condition counts) of each item that carries the type; take the top N types with value `> 0`. Only `MAJOR_TYPES_LIST` types count: FOOD, MEDICAL, MATERIAL, HOUSEHOLD_GOOD, SUBSTANCE, WEAPON, WATER, ARMOR, TOOL, MODULE, AMMUNITION. `WATER_RATION` folds into WATER, `ENERGY_CELL` folds into MATERIAL. A multi-typed item adds its value to every type it has. Ties break by first-seen order in the case (stable sort) | `ShowcaseHelper.GetDisplayCaseType`; `PlayerStore.GetTypesByValue`; `TypeHelper.MAJOR_TYPES_LIST` |
| Guaranteed? | Yes. One buyer per selected type, no dice roll. Not capped by the normal-customer count. (Creation can return null only for an unmapped type, which cannot happen for the 11 types) | `ShowcaseHelper.GetDisplayCaseCustomer`, `HandleDisplayCase` |
| Which client | `CreateClientByStockType(type)` picks a tier from the **total value of that type in the case**, `GetTypeTotalValue` = sum of `GetCurrentValue(no retail markup, with children, with events)` (real value after market events, ENERGY_CELL counted into MATERIAL). Tier 3 needs total `>= 350` and day `>= 14` (HOUSEHOLD_GOOD `>= 300`); tier 2 needs `>= 145`, or attractiveness `>= 400`; else tier 1 (random pick among the tier-1 ids). A multi-typed item counts its full value toward **each** of its types' tiers too (`CountsAsDisplayType`): 35 caffeine pills (MEDICAL + HOUSEHOLD, 350) bring the hospital admin and the upper maid on the same opening (lab, `FormulaExps.WindowMultiType`). Water also gets tier 2 from high-quality display water and tier 3 from pure water, read from the **label** (the claimed grade, weighted by value) at any total value (`GetMostCommonWaterTypeInDisplay`; read). The forecast is read only when the shutter opens, so the case need hold the value only at that moment. MODULE and TOOL are always tier 2 or better | `ShowcaseHelper.CreateClientByStockType`, `GetTypeTotalValue`, `GetMostCommonWaterTypeInDisplay` |
| Tier tables | WATER: thirstySpacer/scavWaterShopper -> waterSpace2 -> waterStockpiler. FOOD: spacerFoodI/hungrySpacer/spacerChef -> expeditionSupplier -> privateChef. MATERIAL/MODULE/ENERGY_CELL/TOOL: lowerRepairman/scrap_byer/maintenanceWorker -> lowerInventor -> stationEngineer. MEDICAL: spacerMedical/sickChildCaretaker/lowLevelDoctor -> clinicNurse -> hospitalAdmin. HOUSEHOLD: spacerHouseholdItem/youngSpacerHousehold/familyMother -> citizenHousehold -> upperMaid. WEAPON/ARMOR/AMMUNITION: scavenger -> selfDefenseEnthusiast -> privateSecurityContractor. SUBSTANCE: desperateAddict -> wornOutSpacer -> upperLevelHedonist | `ShowcaseHelper` id arrays and `CreateClientByStockType` |
| Price they pay | **No display price exists.** The buyer is an ordinary client with a budget; the sale is the normal counter negotiation (`GetNegociatedValue`, bargain, etc.). The case only decides *who walks in*. Nothing ties the buyer to the displayed item: `attractedType` / `attractedItemId` are read only to print "attracted by X" in the tablet header (inferred from a repo-wide grep: only `NegociationUIManager` reads them). The buyer wants stock of the type, which can come from anywhere in the store | `StoreClientManager.HandleShowcaseClient`; `NegociationUIManager` (subtitle at ~L542); `DAY.md` "no showcase pricing" |
| Effect on the general queue | None. Display buyers are added in addition to the random normal customers. Case contents do not change attractiveness | `StoreClientManager.GenerateClient` (separate `HandleShowcaseClient` and `HandleNormalClient`); `PlayerStore.GetCurrentStoreAttractiveness` has no showcase term |

### Special-item predictions (`GetDisplayCasePrediction`): item-triggered named visitors

These are separate from the type buyers above. They are rolled in `HandleNormalClient`, per entry,
with a real dice roll, and they **consume the normal-customer count** (stop when `num >= GetNormalCustomerCount()`),
in list order. Entries are `(clientId, chance %, itemId)`.

| Trigger item in case | Visitor / chance | Evidence |
|---|---|---|
| `permit_gun_1/2/3` | `ll_gun_buyer` 50, `merc_gun_buyer` 40, `ul_gun_buyer` 35 base. Chance = `min(100, base * (2 + daysMissed) / 2)` times `(1 + GunOrderData.GetMarketModifierPercent()/100)`, clamped 0..100. `daysMissed` grows by 1 each day the buyer is predicted but does not show, resets to 0 on a show | `ShowcaseHelper.GetGunBuyerChance`, `OnGunBuyersRolled`, `GetChanceSuffix` (shows `*`) |
| `scav_token` (count n) | `scavGeneral` at `20 * n`% (one entry; not clamped here) | `GetDisplayCasePrediction` |
| `black_lamp` (per target) | invitation target, chance `clamp(lamps * baseChance(target) + BMRep/2, 0, 95)`; one entry per distinct target | `BlackLampHelper.GetActiveInvitations`, `GetChanceForTarget` |
| `sign_water` | a water supplier, 75%. Fixed by day-of-week once the water trader is unlocked (day%7: 1,5 waterCompany; 2,6 waterSellerScav; 3,0 honestBadWaterSeller; 4 waterHishonestSellerScav); else `honestBadWaterSeller` | `GetRandomWaterSupplierId` |
| `sign_food` / `sign_material` / `sign_medical` / `sign_household` | a supplier, 75%. Faction pick is weighted `max(0, LLRep + 25)` scav vs `max(0, BMRep)` thief; chosen once per prediction call (`PickOnce`), cleared after the roll | `GetFoodSupplierId` etc.; `ClearPendingSupplierPicks` |
| `sign_ruined_machine` | `machine_scrapper`, 75% | `GetDisplayCasePrediction` |
| appraisal certificate | `scavAppraisalRandom`, `AppraisalRankHelper.GetAttractionChance()` = `min(100, rankChance * (2 + daysMissed)/2)`; the same days-missed ramp | `AppraisalRankHelper.GetDisplayedCertificate`, `GetAttractionChance`, `OnCustomerRolled` |
| `sign_firearm` | `ll_gun_seller` (weight 100) or `merc_gun_seller` (90), 40% | `GetFirearmSellerId`, const `FIREARM_SIGN_CHANCE` |
| `sign_firearm_heavy` | `ul_gun_seller`, 25% | `GetDisplayCasePrediction` |
| `sign_expedition` | gates the expedition visitor (needs expedition unlocked) | `StoreClientManager.HandleExpeditionClient` |
| Aug disguise | MATERIAL or MEDICAL in the displayed types lets the aug buyer arrive in disguise (uses the type pick) | `ShowcaseHelper.CreateDisplayCaseAugBuyer`; `StoreClientManager.HandleAugClient` |

Sign items are "merchandise" slot users: they occupy grid cells and do not count toward type value
for a type unless they carry the type.

### Discerning shopper

| Rule | Value / formula | Evidence |
|---|---|---|
| Prerequisite | Reputation perk `UL_DISCERNING_TASTE` (UL rep 0 tier) | `ShowcaseHelper.GetDiscerningShopperChance`; `StoreReputationPerk.ULDiscerningTaste` |
| Needs | At least 1 item with type `LUXURY_ITEM` in the case; day `>= 14` unless intro skipped; cooldown expired | `GetDiscerningShopperChance` |
| Chance | `1 + max(0, ULRep) / 5` percent (integer division); ULRep 100 gives 21% | `StoreReputationPerk.GetDiscerningTasteChance`; base and divisor consts |
| Rolled | At shutter open in `HandlePerkClient`, after the normal-customer step; one roll; not tied to value or count of luxury items | `StoreClientManager.HandlePerkClient` |
| Cooldown | 5, started on arrival. The tracker decrements once per opening (before the roll), so after a visit on day D the next possible roll is the opening of day D+5 (4 openings skipped). The UI shows `left - 1` as "openings" remaining | `StoreClientList.CreateDiscerningShopper`; `StoreClientManager.UpdateTracker`, `IsClientOnCooldown`; `ShowcaseHelper.GetDiscerningShopperCooldownOpenings` |
| Buys / pays | A `FACTION_UPPER_LEVEL` buyer wanting tag `LUXURY_ITEM`, budget `750 + rand(0..249) + dayCounter`; normal counter negotiation. A cap on spend, not a fixed price | `StoreClientList.CreateDiscerningShopper`, `CreateUpperLowerVisitorLuxury` |

### Risks

| Rule | Value / formula | Evidence |
|---|---|---|
| Contraband in case | Any item tagged `CONTRABAND_ITEM_TAG` (any level) **guarantees** a `fromShowcase` Security Inspector (personality 0, the plain one, not bribable) at the very start of the day's generation, before everything else | `StoreClientManager.HandleShowCaseSecurity` (called after `FireOnGenerateCustomerVeryEarly`); `ShowcaseHelper.IsShowCaseContainSeriousContraband`; `StoreClientList.CreateInspectionClient(0, true)` |
| Stolen item in case | Per stolen item, roll `STOLEN_VALUE_INT` percent (its heat/found chance); the first success triggers the same inspector, and sets that item's heat to 100. Rolled each opening | `ShowcaseHelper.IsShowCaseContainSeriousStolen`; `StolenHelper.GetFoundChance` |
| Inspection outcome | `ContrabandHelper.StartInspection(personality, true)` searches stock, both counters, **the showcase**, the trash (unless lazy), and the poster-hidden stash if the poster is pulled; concealed items are collected separately. Fines, seizure and Security-rep loss follow the worst item (levels LOW warning, MID seizure, HIGH fine, CRITICAL; stolen = seizure with rep loss). The usual clean-inspection `+3` Sec rep is **not** granted when `fromShowcase` | `ContrabandHelper.StartInspection` |
| Any other Security client | A patrol officer arriving while contraband or stolen is in the case becomes undismissable and starts an inspection (stolen sets heat 100). The same applies if the item is added to the case while the officer is present | `StoreClient` Security `OnArrival` and `OnItemAddedToShowcase` handlers; `ContrabandHelper.SecurityCheckDisplayCase` |
| Heat | Display never lowers heat. A found stolen item is raised to 100. No code in the showcase path *adds* heat to unfound items | `StolenHelper.SetHeat` callers above |
| Theft or damage | No customer-theft or breakage logic touches the case. (Vandalism cuts the buyer count by 1, and the second tier loses its slot while vandalized; see above.) Not exhaustively checked for wanted/thief client scripts | `PlayerStore.GetShowcaseTypeVisitorCount`, vandalism at `PlayerStore.cs` ~L2145 |

### Other levers

| Rule | Value / formula | Evidence |
|---|---|---|
| Store attractiveness | `base (250) - trashPenalty + projectorBonus`. Trash penalty = `base * garbage * 5%` when `garbageLevel > 2`. Projector bonus = `100 + quality/performance-derived`, set when the machine cycles | `PlayerStore.GetCurrentStoreAttractiveness`, `GetTrashAttPenalty`; `MachineProjector.GetAttractivenessBoost` |
| Attractiveness thresholds | `>= 100` enables display buyers; `>= 200`, `>= 250`, `>= 500` each add +1 normal customer (not if vandalized); `>= 400` promotes tier 1 display buyers to tier 2 | `StoreClientManager.HandleShowcaseClient`, `GetNormalCustomerCount`, `CreateClientByStockType` |
| Normal customer count | Clamped 0..6. This is the pool the named special-item visitors draw from | `StoreClientManager.GetNormalCustomerCount`, `HandleNormalClient` |
| Market events | Type-value ranking uses the displayed price (`GetNegociatedValue`); tier value uses `GetCurrentValue(includeEvents: true)`. So an event boosting a type raises tier-thresholds crossing, but nothing else links events to display | `PlayerStore.GetTypesByValue`; `ShowcaseHelper.GetTypeTotalValue` |
| Forecast is visible | The clientele panel shows the buyer types, the special predictions with chances, and the discerning shopper chance/cooldown, so the plan can be checked before opening | `ClienteleUIManager.RefreshUI` |

## What a good window strategy must weigh

1. **Fill slots by value per type, not by item count.** Only the top `N` types by displayed value
   attract buyers (N = 1 to 4). Concentrate value in the types you can supply and want to sell;
   a second, small type is wasted when N = 1 (no upgrades, no referral service).
2. **Tier is set by real value in the case.** Crossing 145 and then 350 (300 for household,
   day >= 14) turns a scavenger into a richer, bigger-budget buyer. Check what each item contributes
   after market events; one high-value item of the right type can cross the line.
3. **Display buyers are free and guaranteed, but they are not bound to the displayed item.**
   The case buys you a visit; the sale still comes from counter stock and the usual negotiation.
   Keep matching stock in the back, not just in the window.
4. **Never put contraband in the window.** A guaranteed inspection fires every opening, and a
   `fromShowcase` inspection also searches the rest of the store and gives no clean-inspection rep.
   Stolen items are a per-opening roll at their heat percentage; a high-heat stolen item is a
   near-certain inspection. Keep both in stock or hidden, not on display.
5. **Special-item signs and gun permits are the high-yield slots.** Chances of 75% (supplier signs)
   and the rising days-missed gun/appraisal ramps cost one grid slot each, but they draw from the
   normal-customer count, so they compete with random customers rather than adding to them.
6. **Luxury item for the discerning shopper** is worth one cell only if the UL perk is unlocked
   and day >= 14: 1% plus UL rep / 5, a 5-opening cooldown, and a budget around 750 or more.
   Low UL rep makes this a poor use of a slot.
7. **Do the setup before the shutter opens.** The forecast is frozen at open; late changes only
   help tomorrow. Plan the window the evening before, read the clientele panel, then open.
8. **Raise the ceiling.** Buyer count (referral service, upgrades I/II) and attractiveness
   (clean the trash, run the projector, avoid vandalism) are independent multipliers on the
   window's yield. Vandalism removes buyer slots until it is cleared.
