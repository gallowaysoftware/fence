# Day-level state and decisions (RE notes)

Build 26525552 / 0.46D. Same conventions as `STATE.md`: **VISIBLE** = rendered by the UI
(evidence = the UI code that renders it), **HIDDEN** = internal only, **DERIVABLE** = computable
from visible facts. Types are in the global namespace of `Assembly-CSharp`; `List<T>` means
`System.Collections.Generic.List<T>`. Proposed hooks are in `compat/hooks.proposed.json`
under the features `record.snapshot`, `record.human`, `record.night`, `record.customer`,
`record.run`.

Two facts shape everything below:

- **`RunStats` is a free funnel.** It is a set of `public static void On…` methods the game
  calls at every economically meaningful moment (rent, services, upgrades, mortgage,
  scavenging, inspections, day end, run end). Postfixing them gives exact amounts without
  reimplementing any rule.
- **Every non-counter choice is a dialogue choice.** Rent, buyout, mortgage, inspection
  accept/bribe, story and wanted decisions, supplier orders by phone: all are
  `Dialogue.AddChoice(...)` entries resolved by `DialogUIManager.SelectChoice(System.Int32)`
  (store) or `DialoguePhoneUIManager.SelectChoice(System.Int32)` (phone). One prefix on each
  records them all. GameLog's `Could not find GameItem identifier '$' / 'yes' / ''` warnings are
  these choice tokens being built.

`EmporiumAnalytics.Track*` methods are called even though analytics is disabled
(`OVERWRITE_DISABLE_ANALYTICS`; each method returns early on `!isAnalyticsInitialized`), so a
postfix on them still fires with the arguments. They are a second set of stable funnels.

## 1. Day snapshot

### When

| Phase | Hook | Why |
|---|---|---|
| `morning` | `ModHook.OnWakingUpEarly` (event) | **Not** `OnWakingUpLate`: `StartOfDayUIManager.InitPanel()` (between Early and Late) renders then **clears** `PlayerStore.nightLogs`. Read the night log at Early; everything else is final by then (`BeginDay()` already ran). |
| `close` | `ModHook.OnShutterClosedLate` | End of the counter day, before the night. |
| `sleep` | `ModHook.OnGoingSleepEarly` | Fired inside `PlayerStore.EndNight()` after the night's outings and purchases, **before** services are charged and overnight effects (heat decay, burglary, vandalism, chemistry) run. Captures the final service selection and the stock the night acts on. |
| `run_start` / `load` | postfix `PlayerStore.InitialSave()` / `ModHook.OnGameLoadedLate` | Day 1 has no wake-up. `runID` is assigned in `InitialSave`. |

### Fields

All reads are from `PlayerStore.Instance` (`ps`) unless noted.

**store**

| Field | Read | Vis. | Note |
|---|---|---|---|
| `cash` | `ps.playerCash : System.Int32` | VISIBLE | |
| `day` | `StoreStation.Instance.dayCounter : System.Int32` | VISIBLE | |
| `weekday` | `StoreCalendar.GetDayOfWeek() : System.String` (instance) or `day % 7` | VISIBLE | rent day is `day % 7 == 0` |
| `wild_favor` | `ps.wildFavor : System.Int32` | VISIBLE | second currency, spent before cash on upgrades (`WildNetworkUI` "favor" stat) |
| `net_worth` | `ps.GetTotalEstimatedValue() : System.Int32` | VISIBLE | calendar → Status "total wealth" (`AdvCalendarUIManager.OnStatusButtonClicked`) |
| `attractiveness` | `ps.GetCurrentStoreAttractiveness() : System.Int32`, `ps.GetTrashAttPenalty() : System.Int32`, `ps.projectorAttractivenessBonus : System.Int32` | VISIBLE | calendar → Nightly report meter (`UpdateAttractivenessSubsection`) |
| `queue_length` | `ps.GetClientInQueueCount() : System.Int32` | count VISIBLE? (unverified) | 0 in the morning; the queue is built at shutter open |
| `retail_markup` | `ps.retailMarkup : System.Int32` | VISIBLE | **not a decision**: set once from `Difficulty.Rules.retailMarkup` (`StoreService.ApplyDifficultyStart`); belongs in `run_start` |
| `commissary_tokens` | `ps.commissaryData.redeemedCredit : System.Int32` | VISIBLE when `ps.isStampIntroduced` | calendar Status |
| `business_permit` | `ps.hasBusinessPermit : System.Boolean` | VISIBLE | |
| `newspaper_unlocked` | `ps.isNewspaperUnlocked : System.Boolean` | VISIBLE | gates `market_events.news` |

**rent / property** (replaces `rent_due` / `rent_days_left`)

| Field | Read | Vis. | Note |
|---|---|---|---|
| `rent` | `ps.rentValue : System.Int32` | VISIBLE | grows ×`Difficulty.Rules.rentIncreaseMultiplier` after **every** landlord visit, paid or not (1.025 cozy, 1.05 normal, 1.1 hard/challenge), the product truncated to an int each time: normal 200 → 209 → 219 → 229 → 240 → 251 (+4.5–4.8 %/week), cozy a flat +5/week (205, 210, 215…), hard 220 → 242 → 266 → 292 → 321 (measured, docs/sim/VERIFIED.md; "paid or not" not exercised) |
| `rent_arrears` | `ps.temporaryRent : System.Int32` | VISIBLE | missed rent carried over; due amount is `rent + rent_arrears` (`CanPayRent()`) |
| `rent_days_left` | `ps.dayUntilRent : System.Int32` | VISIBLE | what the calendar prints (`OnCalendarButtonClicked`). The morning reminder uses `ps.GetRentDayCounter()` = `(7 − day % 7) % 7` instead; record both, they can disagree after a load |
| `landlord_grace` | `NetworkUpgrade.IsLandlordGraceUnlocked() : System.Boolean` (static) | VISIBLE (landlord's first line differs) | without it, not paying = game over `rent` |
| `property` | `ps.IsPropertyPaid`, `ps.isPayingMortgage : System.Boolean`, `ps.mortgageWeeklyPayment`, `ps.mortgageWeekLeft : System.Int32` | VISIBLE | calendar landlord / loan-shark notice |
| `buyout_price` | `ps.GetStoreBuyPrice(System.Boolean) : System.Int32` (pass `false`) | VISIBLE (calendar "property value") | `propertyBasePrice` 12,000 cozy/normal, 24,000 hard, 36,000 challenge; +25/+50/+50/+100 per day after day 50, capped +3,000/+5,000/+10,000/+20,000. So "the 12,000 goal" is only the cozy/normal base and grows after day 50 |
| `buyout_affordable` | `ps.CanBuyOutStore() : System.Boolean` | DERIVABLE | the landlord only offers the buyout choice when true |
| `mortgage_offer` | `ps.GetMortgageDownPayment()`, `ps.GetMortgageWeeklyPaymentPreview() : System.Int32`, `PlayerStore.GetMortgageWeeks() : System.Int32` (static), `ps.CanBeOfferedMortgage() : System.Boolean` | VISIBLE when offered (`LoanPanelUIManager.OpenUI`, loan-shark dialogue) | offered when cash ≥ 2× down payment |

There are **no player loans**: `PlayerStore.GetLoan()` / `PaidLoan()` are empty and
`dayUntilLoan`/`loanValue` are vestigial. The only debts are rent arrears and the mortgage. The
player can *lend* to Georgio (story; `RunStats.OnGeorgioLoan(System.Int32)`,
`OnGeorgioPayback(System.Int32)`).

**reputation** — as `STATE.md`, per faction in `ps.storeReputations : List<StoreReputation>`
(`factionId`, `GetReputationExact() : System.Double`, `StoreReputation.GetReputationStatus(System.Int32)`).
`next_perk` = the lowest `StoreReputationPerk` in `StoreReputation.perks` with
`perkThreshold > rep` (fields `perkId : System.String`, `perkThreshold : System.Int32`,
`isUnlocked : System.Boolean`). Add `next_negative_perk` = the highest negative threshold
below the current value: avoiding −100 perks (Lynch, Hitman, ImmediateArrest, Hit, Retired)
is what keeps a run alive (each queues a game-over morning visitor in
`PlayerStore.OnLateShutterOpen()`). VISIBLE (`ReputationUIManager`).

**inventory** — one row per owned item, from
`EmporiumEntry.Instance.GetAllOwnedItems(System.Boolean isBurglary, System.Boolean includeAfterhour) : List<GameItem>`
(pass `false, true`). It walks every container recursively, so items inside bags/machines are
included. `where` = the root container found by walking `GameItem.parentInventory : GameInventory`
up (`GraphUtils.FindParentType<GameItem>` for nested items) to one of these `EmporiumEntry`
members:

| `where` | Member | Note |
|---|---|---|
| `stock` | `invElement : GameGridInventory` | |
| `showcase` | `showcaseElement : GameGridInventory` | display case; drives forecast customers, and an arriving Security client who sees stolen/contraband in it triggers an inspection |
| `back_counter` | `backInvinvElement`, `backInvinvElementCounter : GameGridInventory` | staged for the current customer |
| `front_counter` | `frontInvinvElement : GameGridInventory` | |
| `hidden` | `hiddenElement : GameGridInventory` | stash behind the poster; only searched while the stash is open (`StorePoster.Instance.isPulled`; `PullUp` opens it — measured, docs/re/CONCEALMENT.md) |
| `trash` | `trashInvElement : GameSlotInventory` | searched by inspectors unless lazy |
| `docs` | `docInvElement : GameSlotInventory` | |
| `vending_machine` / `vending_fountain` | `vendingMachineElement` / `vendingFountainElement : GameSlotInventory` | sell overnight (`RunStats.OnVendingSale`, `OnFountainSale`) |
| `afterhour` | `afterhourWindow` | carried bag while outside |
| `inside:<uid>` | — | nested in another owned item |

Per item: the `STATE.md` item shape (`uid` = `GameItem.uniqueId : System.Int32`, `id`, `name`,
`estimated_value`, `bought_at` from tag `BOUGHT_PRICE_TAG`, visible `features`, stolen heat
from `STOLEN_VALUE_INT`, contraband level / serious). All VISIBLE in the item tooltip; truths
(`realCondition`, `AppraisalHelper.GetCorrectBaseValue`) go to `hidden.items`.

**There is no showcase pricing.** The display case has no price field; items in it sell at the
normal counter price when a forecast buyer arrives. Drop `showcase_price` from the plan.

**clientele forecast** (VISIBLE: `ClienteleUIManager.RefreshUI`, the store's clientele display)

| Field | Read |
|---|---|
| `normal_customers` | `ps.storeClientManager.GetNormalCustomerCount() : System.Int32` |
| `display_buyers` | `ShowcaseHelper.GetDisplayCaseType() : List<System.String>` (item types that will attract a buyer) |
| `display_predictions` | `ShowcaseHelper.GetDisplayCasePrediction() : List<System.ValueTuple<System.String,System.Int32,System.String>>` = (clientId, chance %, itemId); name via `StoreClientListDict.GetClientDisplayName(System.String)` |
| `discerning_shopper` | `ShowcaseHelper.GetDiscerningShopperChance()`, `GetDiscerningShopperCooldownOpenings() : System.Int32` |

**security** (VISIBLE only through the record document: `RecordUIManager.Init()` renders it
when the player activates the `intel_sec` item. Gate on `ps.isSecurityRecordIntroduced` **and**
owning `intel_sec`; unverified how the player first gets it.) Read `ps.secData : SecData`:

| Field | Read | Note |
|---|---|---|
| `crimes` | `crimeTypes : List<System.String>`, `crimeAmount : List<System.Int32>` | VISIBLE: `GetSuspectedCrimeDisplay()` prints each crime **with its amount**, so `STATE.md`'s "raw crimeAmount HIDDEN" is wrong for `crimeAmount`. `crimeAmountTotal` (never decays) is HIDDEN. Amounts decay 20/day while evidence < 80 (`SecData.OnNewDay`) |
| `evidence` | `evidenceLevel : System.Int32`; display `GetEvidenceLevelDisplay() : System.String` | VISIBLE as `N/100` (or "not applicable" below 100 total crime). The fixture's `"Low"` is wrong. At 100 a sentencing officer is sent |
| `projected_sentence` | `GetProjectedSentence() : System.String` | VISIBLE; fine < 300, incarceration < 1500, execution ≥ 1500 total |
| `case_officer` | `GetCaseOfficer(System.Int32) : System.String` with `GetTotalCrimeValue()` | VISIBLE |
| `recommended_action` | `GetAction() : System.String` | VISIBLE: reliable inspector (bribery ≥ 500), extra inspections (trafficking ≥ 1000), sting (contraband ≥ 1000) |
| `operation` | `GetOperation() : System.String` | VISIBLE: investigation / planning (≥ 90) / execute (100) |
| `note` | `note : System.String` | VISIBLE |
| `trafficking_value` | `GetTotalTraffickingCrimes() : System.Int32` | DERIVABLE (sum of visible amounts). It is a credit value, not a count: rename the fixture's `trafficking_crimes` |

Inspection timing (`StoreClientManager.dayUntilInspection`) stays HIDDEN; with the early-warning
service it surfaces as night-log lines at 3/2/1 days (`PlayerStore.HandleInspectionEarlyWarning`),
which the snapshot already carries in `night_log`.

**upgrades** — `ps.networkUpgrade : Dictionary<System.String,NetworkUpgrade>` (VISIBLE in the
Wild Network window, `WildNetworkUI`). Per entry: `id`, `GetLocalizedTitle() : System.String`,
`GetCost() : System.Int32` (difficulty and perk multipliers applied), `state : System.Int32`
(0 not bought, 1 bought-and-ready for consumables such as `LANDLORD_GRACE`/`SEC_GRACE`,
2 unlocked), `IsUnlocked()`, `IsHavePrerequisite() : System.Boolean`, `cooldownCurrent`, `tab`.
Skip ids where `NetworkUpgrade.IsHiddenForRun(System.String)` or `hidden`. Record
`owned` (state 2 / ready) and, compactly, `available` (prerequisites met, not owned, with cost).

**services** — `ps.storeService : List<StoreService>`; per entry `id`, `displayName`,
`cost : System.Int32`, `unlocked`, `isUsing`, `isMandatory : System.Boolean`,
`currentCooldown : System.Int32`. VISIBLE in the sleep tablet (`EndOfDayUIManager.InitUI`; rows
only for `unlocked`, and `POWER_SERVICE` is hidden once power is hijacked). Ids:
`POWER_SERVICE`, `INSURANCE_SERVICE`, `JANITORIAL_SERVICE`, `SEC_INFORMANT` (inspection early
warning), `WILD_FIXER`, `BUY_REFERAL`, `SELL_REFERAL`, `CRIMINEL_NETWORK`, `CARTEL_PROTECTION`,
`CURRENT_CONVERTER_SERVICE`, `DEBRIS_REMOVAL_SERVICE`, `BUTCHER_HUNTING_SERVICE`,
`HYPERTECH_SMUGGLER_SERVICE`. Costs are recomputed nightly (`StoreService.UpdateCost()`).

**market_events** — `StoreStation.Instance.storeEventManager : StoreEventManager`.

| Field | Read | Vis. |
|---|---|---|
| `active` | `GetActiveEvents() : List<StoreEvent>` minus `isHiddenFromNewsPaper` | VISIBLE in the newspaper (`NewsUIManager.PopulateUI`) |
| `calendar` | `GetActiveEventOnDay(System.Int32, System.Boolean excludeCosmetic, System.Boolean excludeUnknown) : List<StoreEvent>` for the 14 calendar days (week start .. +13), defaults `true, true` | VISIBLE (`AdvCalendarUIManager.OpenUI`); includes **known future** events (`futurEvents` with `IsKnown`) and the innate `rent_day` / `mortgage_payment` / `cartel_visit` markers |
| per event | `identifier`, `GetDisplayName() : System.String`, `eventType : StoreEvent+EventType` (NORMALE/THREAT/COSMETIC/INNATE), `startDay`, `totalDuration`, `isDurationVisible`, `negociationDatas : List<NegociationData>` (`itemType`, `value` %) | VISIBLE |
| unknown future events | `futurEvents` with `!IsKnown`, `isHidden` | HIDDEN |

The `Unsubscribed` starting perk hides all non-innate events (`StartingPerk.IsUnsubscribed()`).

**afterhours** — `AfterhourLocation.GetDiscovered() : List<AfterhourLocation>` (`Id`,
`GetTitle()`, `IsAvailable()`, `IsClosedByEvent()`), and per location
`AfterhourEvents.GetActive(System.String) : List<AfterhourEventDefinition>` (`id`, `GetTitle()`,
`GetEffectTexts() : List<System.String>`) with `AfterhourEvents.GetDaysLeft(System.String) : System.Int32`.
VISIBLE on the transit map (`CustomUIAfterhourEventsPanel`; unverified exact rendering).

**requests** — `ps.storeRequests : List<StoreRequest>`: `id`, `title`, `requesterName`,
`factionId`, `requiredItem`, `requiredItemCount`, `creditReward`, `reputationReward`,
`duration`. VISIBLE on the request board (`StoreRequestElement`).

**night_log** — `ps.nightLogs : List<System.String>` (rich text) read at `OnWakingUpEarly`.
VISIBLE. Carries theft, vandalism, insurance, referrals, investigation-speed and inspection
warnings, aug warnings.

**Hidden block** (`hidden`, never prompted): `days_to_inspection`
(`ps.storeClientManager.dayUntilInspection`), `inspection_countdown_active`
(`IsInspectionCountdownActive()`), `aug_presence` (`ps.augPresence : System.Int32`, game over at
100 on some difficulties), `luck` (`ps.healthData.luck`), `crime_total`
(`secData.crimeAmountTotal`), unknown future events, item truths.

### Proposed `day_snapshot.data`

```
phase, store{cash, day, weekday, wild_favor, net_worth, attractiveness{total, trash_penalty,
  projector_bonus}, queue_length, commissary_tokens, business_permit},
rent{rent, arrears, days_left, days_left_morning, landlord_grace},
property{owned, mortgage{weekly, weeks_left}|null, buyout_price, buyout_affordable,
  mortgage_offer{down_payment, weekly, weeks}|null},
reputation{<faction>{value, status, next_perk, next_negative_perk, unlocked_perks[]}},
inventory[{uid, id, name, where, estimated_value, bought_at, features[], stolen_heat,
  contraband_level, serious_contraband}],
clientele{normal_customers, display_buyers[], display_predictions[{client, chance, item}],
  discerning_shopper{chance, cooldown}},
security{visible, crimes[{id, amount}], evidence, evidence_level, projected_sentence,
  case_officer, recommended_action, operation, note, trafficking_value} ,
upgrades{owned[{id, name}], available[{id, name, cost}]},
services[{id, name, cost, unlocked, using, mandatory, cooldown}],
market_events{active[...], calendar[{day, events[...]}]},
afterhours[{location, available, events[{id, name, effects[], days_left}]}],
requests[...], night_log[]
```

`retail_markup`, difficulty, start and perks move to `run_start` (section 5).

## 2. Night phase and the rest of the day

Sequence: shutter closed (`StoreShutterButton` → `PlayerStore.EndDay()`) → optional outing
(`MapUIManager.OpenUI()` → transit map → `MapUIManager.Travel(AfterhourLocation)` → `Return()`)
→ sleep (`StoreBedroll.OnClick()` or `MapUIManager.Sleep()` → `EndOfDayUIManager.OpenUI()`;
service checkboxes; `EndOfDayUIManager.OnEndDayButtonClick()`) → `PlayerStore.EndNight()`
(services charged, overnight effects) → `BeginDay()` → morning report.

| Decision | When / where | Observe (prefer public) | Data |
|---|---|---|---|
| **Nightly services** | sleep tablet | Read the final `isUsing` per service at `ModHook.OnGoingSleepEarly`; charges via postfix `RunStats.OnServicePaid(System.String, System.Int32)`. Toggles are private (`EndOfDayUIManager.OnToggled`) or UI-only (`ServiceElement.OnToggled()`); diff rather than hook | `night_action: services_set`, `{selected:[id], costs}` |
| **Sleep** | sleep tablet | prefix `EndOfDayUIManager.OnEndDayButtonClick()` or postfix `EmporiumAnalytics.TrackOnSleep()` | `sleep` |
| **Wild Network upgrade** | phone → Wild (`PhoneDialogList.PhoneWild` → `WildNetworkUI.Open(System.Boolean)`), also some story clients | postfix `RunStats.OnWildUpgradeBought(System.Int32 cashSpent)` then postfix `EmporiumAnalytics.TrackWildUpgradePurchased(System.String upgradeId)` (called in that order by both `WildNetworkUI` and legacy `WildUIManager.OnUnlockClicked()`). `ModHook.OnNetworkUpgradeUnlocked` also fires for non-purchase unlocks | `upgrade_buy {id, cost, cash_spent, favor_spent}` (favor = cost − cash) |
| **Supplier orders** | phone call to a supplier (Red Imp brewer, mining, pharma, chemist, rev merchant) | prefix `DialoguePhoneUIManager.SelectChoice(System.Int32)` (choice text = `choices[i].Item2.plainText`; labels are `""`), postfix `PlayerStore.QueueFuturClient(System.String storeClientId, System.Int32 day)` (courier queued) and `StorePhoneClient.UseService()`. Call start: `PhoneUIManager.StartPhoneDialog(System.Int64)` | `phone_order {supplier, choice_text, courier_id, in_days}`. Payment happens when the courier arrives as an ordinary SELL client (existing barter hooks) |
| **Rent / buyout / mortgage** | landlord (`identifier "landlord"`, `ClientIntent.RENT`, rent day) and loan shark (`"loan_shark2"` offer, `"loan_shark_collect"`) at the counter | prefix `DialogUIManager.SelectChoice(System.Int32)` (labels `"pay rent choice"`, `"Don't pay the rent"`, `"buyout choice"`; loan shark `"yes choice"` / `"no choice"` / `"soon choice"`), plus postfixes `RunStats.OnRentPaid(System.Int32, System.Int32)`, `OnRentMissed()`, `OnStoreBoughtWithCash(System.Int32)`, `OnMortgageTaken(System.Int32, System.Int32, System.Int32, System.Int32)`, `OnMortgagePayment(System.Int32, System.Int32)`, `OnMortgagePaidOff()` | `dialogue_choice` + amounts |
| **Afterhour outing** | transit map | `ModHook.OnAfterhourArriveLate(string)` / `OnAfterhourLeaveLate(string)` (already hooked), or prefix `MapUIManager.Travel(AfterhourLocation)` | `go_out {location}`, `return` |
| Dumping ground | `dumping_ground` | postfix `RunStats.OnScavengeAttempt()`, `OnScavengeInjury(System.Boolean major)`, `OnScavengedItems(List<GameItem>)`; action `ScavHelper.ScavengeDumpingGrounds()`. Visible odds: `ScavHelper.GetScavTimeLeft() : System.Int32`, `GetMinorWoundChance()`, `GetMajorWoundChance() : System.Single` | `scavenge {attempts_left, minor_pct, major_pct}` → items, injury |
| Commissary | `commissary` | postfix `CommissaryData.BuyItem(System.String)` (diff `redeemedCredit` to confirm), `CommissaryUIManager.OnRedeemAllButtonClick()` | `commissary_buy {id, price}`, `commissary_redeem` |
| Dr. Jackson | `dr_jackson` | he is an ordinary seller at the counters (`StoreClientList.PlaceInventorInventory()`); existing barter hooks (`PlayerStore.OnItemBought`) capture purchases | as barter |
| **Request board** | any time | prefix `StoreRequest.FillRequest()` (instance; called from `StoreRequestElement`) | `fill_request {id, items, reward}` |
| **Showcase placement** | any time | postfix `PlayerStore.AddedItemToDisplayCase(GameItem)` / `RemovedItemToDisplayCase(GameItem)` | `showcase` / `unshowcase {item_uid}` |
| **Hide stash** | poster | postfix `StorePoster.PullDown(System.Boolean)` / `PullUp(System.Boolean)` | `poster {up|down}` (up = stash open, searched) — matters for inspections |
| Exchange board | night resolution | prefix `ExchangeManager.HandleExchange()` (reads `itemsBuffer`) | low priority |
| Vending stock | any time | inventory diff of `vending_*` between snapshots; sales via `RunStats.OnVendingSale(System.Int32)`, `OnFountainSale(System.Int32)` | |

Not decisions or out of scope: retail markup (fixed per difficulty), loans (none), item repair
/ crafting / machines / labeler / chemistry. These are production loops with many entry points
(`MachineHelper`, `CraftingHelper`, `LabelerUIManager`); the snapshot inventory diff between
`close` and `sleep` / next `morning` records their net effect. Mislabeling (labeler, vending)
commits FRAUD/ENDANGERMENT crimes and shows up in `security.crimes`.

Overnight outcomes worth recording as `night` events (all postfix, all public static):
`RunStats.OnBurglary(System.Int32)`, `RunStats.OnDayEnded(System.Int32 day, System.Int32 netWorth, System.Int32 credits)`.

## 3. Human actions at the counter

`human_action.action` values. Barter ids reuse the `decide` option ids from `PROTOCOL.md`.

| `action` | Hook | Data / notes |
|---|---|---|
| `stage` / `unstage` | postfix `PlayerStore.AddedItemToWeightedArea(GameItem)` / `RemovedItemToWeightedArea(GameItem)` | `item_uid`, `owned`. These fire for **every** add/remove on `backInvinvElement` (`onSlotAddItemEndFunc` / `onSlotRemoveItemEndFunc`), including the game's own moves (`AddDirectToWeightedTable`, dismiss clean-up, `MoveDealItemToInventory`, Dr. Jackson's counter shuffle) and the mod's `act.stage`. Suppress while inside those (prefix/postfix depth counter on `PlayerStore.DismissCurrentClient`, `RemoveNotOwnedItemFromTable`, `BuyItem`, `SellItem`, `AcceptBarter`, `EmporiumEntry.ClearBackInv`) and while the mod acts. The player-only drag path (`ItemMouseDragHandler.EndDrag`, private) is the fallback if this leaks |
| `front_add` / `front_remove` | postfix `PlayerStore.AddedItemToClientArea(GameItem)` / `RemovedItemToClientArea(GameItem)` | only when the item is owned and not game-spawned; low value |
| `showcase` / `unshowcase` | see §2 | |
| `open_tablet` / `close_tablet` | postfix `StoreClientMono.OnClick()` (player clicked the customer) and `NegociationUIManager.CloseUI()`. Do **not** use `NegociationUIManager.OpenUI()` alone: `PlayerStore.HandleAutoNegoUIDisplay(GameItem)` opens it automatically when `EmporiumOption.AutoOpenNegotiationUI` | |
| `expose_feature` | prefix + postfix `ItemFeatureUI.OnFeatureUIClick()` | prefix: `ItemFeatureUI.itemFeature : ItemFeature` (`identifier`, `category`, `parentItemUniqueId`), tool = `RTSController.previousSelectedObjects[0]` → `NegociationElement.GetNegociationToolID() : System.String` (or a `NotebookElement` clue). Postfix: `result` = `exposed` if `itemFeature.IsExposed()` flipped, `exposing_dialog` if `GetExposeDialog()` started a dialogue, else `bad_match` (`OnExposeFeatureBadMatch()`). Also `NotebookElement.ElementClicked()` for public-hidden features. Do not hook `ItemFeature.ExposeFeature(System.Boolean, System.Boolean)` for this: customers, scanners and helpers call it too (≈30 call sites) |
| `inspect_item` / `confront` | prefix `InspectionUIManager.OnInspectItem(GameItem)` (from the inspection kit, `ToolDirectory`), postfix `EmporiumAnalytics.TrackConfront(System.String itemId, System.String confrontType, System.String outcome)` | cigarettes, stamps, injectors: player claims "confront"/"talk"; outcome success/fail |
| `buy_all`, `accept_wholesale`, `sell_all`, `confirm_over_budget` | existing `barter.buy_all`, `barter.wholesale`, `barter.sell_all`, `barter.confirm_over_budget` (prefix) | |
| `accept_offer` | prefix `NegociationUIManager.OnOfferAcceptClick()` | barter / info dealer / gunsmith; `mode` from `StoreClient.clientIntent` |
| `offer_discount_N`, `add_markup_N`, `offer_markup_N`, `demand_discount_N`, `blackmail`, `threaten` | existing bargain hooks (already observed) | |
| `dismiss` | existing `barter.dismiss` | |
| `ring_bell` | prefix `StoreBell.RingBell()` | calls the next customer; also dismisses the current one |
| `dialogue_choice` | prefix `DialogUIManager.SelectChoice(System.Int32)` | `index`, `label` = `choices[i].Item1.name` (internal, e.g. `"pay rent choice"`, often `""`), `text` = `choices[i].Item2.plainText` (what the player read), `enabled` = `Dialogue.IsChoiceEnabledAt(System.Int32)`, plus all choice texts offered. Read via `DialogUIManager.Instance.GetCurrentDialog() : Dialogue`, field `choices : List<System.ValueTuple<GameItem,Dialogue,System.Action>>` |
| `phone_choice` | prefix `DialoguePhoneUIManager.SelectChoice(System.Int32)` | same shape |
| `report_customer` | prefix `PlayerStore.OnCustomerReported()` (security button `StoreSecButton.OnClick()`, also phone) | outcome: `EmporiumAnalytics.TrackCustomerReported(StoreClient)`; wrongful → `RunStats.OnWrongfulReport()` |
| `unholster` / `holster` | prefix `StoreUIManager.EnterShootingMode()` / `ExitShootingMode()` (holster only while `isShot` is false: lowering the gun without a shot dismisses the customer) | |
| `shoot` | prefix `StoreUIManager.Shoot()`: every shot, with the tells on screen. What it hit comes after Security's verdict as an `aug` event: `good_kill` (prefix + postfix `AugHelper.OnGoodKill()`), `wrongful_kill` (prefix + postfix `AugHelper.CleanupKill()`, reading `AugHelper.GetShotClient()` first), `wanted_capture` (postfix `PlayerStore.OnWantedClientShot(StoreClient)`); an aug that leaves any other way is `left_alive` (`AugHelper.OnAugLeftAlive()`) | `RunStats.OnAugShot()` fires only inside `OnGoodKill`, so it misses wrongful kills; `CleanupKill` is the only hook that sees them (`docs/re/AUGS.md` §3.1) |
| `appraisal_estimate` | prefix `AppraisalServiceHelper.OnEstimate(GameItem, System.Int32 value)` | the player's number; outcome `EmporiumAnalytics.TrackAppraisalSent(GameItem, System.Int32, System.Int32)` (third arg is the true value → `hidden`) |
| `open_shutter` / `close_shutter` | existing shutter events | |
| night ids | §2: `services_set`, `sleep`, `upgrade_buy`, `phone_order`, `go_out`, `return`, `scavenge`, `commissary_buy`, `commissary_redeem`, `fill_request`, `poster` | |

## 4. Customers outside the advice path

All customers pass `RunStats.OnCustomerArrived(StoreClient)` when called (`PlayerStore.GetNextClient()`)
and `StoreUIManager.OnNextClientArrived()` on arrival (existing `barter.session_start`). Emit `customer_observed` there for every customer with
`covered:false` and a `reason`, then record whatever the player does via `dialogue_choice`,
`accept_offer`, `dismiss`, `report_customer`, `shoot`. Visible fields common to all:
`displayName`, intent, faction (bubble colour), `dismissable`, the dialogue lines
(`DialogUIManager.UpdateDialogHistory` / `ps.dialogHistory : Queue<System.String>`) and offered
choice texts.

| Kind | Identify | Visible extras | Player's choice |
|---|---|---|---|
| Story / unique / cartel / morning | `StoreClientListStory`, `StoreClientUniqueList`, `StoreClientListCartel`, `StoreClientListMorning`; often `dismissable=false` | choice texts | `dialogue_choice`; trades through barter hooks |
| Wanted | `StoreClient.isWanted` (DERIVABLE by matching `wantedPosterId` to the posters, `WantedUIManager`) | poster; `isStealthWanted` HIDDEN | `report_customer`, `shoot`, trade, or let go (`EmporiumAnalytics.TrackWantedNotReported()` fires at night, `PlayerStore.HandleWantedPersonVistedCheck`) |
| Security (patrol) | `StoreClient.isSecurity`, `StoreClientListSecurity` | dialogue | none; an arriving officer that sees stolen/contraband on the counter or in the display case sets heat 100 and starts an inspection |
| **Inspection** | `identifier "securityInspector"`, `ClientIntent.INSPECTION`, `dismissable=false` (`StoreClientList.CreateInspectionClient(System.Int32 personality, System.Boolean fromShowcase)`) | **personality is VISIBLE by name**: "Security Inspector" (0), "Shady Security Inspector" (1, bribable), "Lazy Security Inspector" (2, skips the trash). The bribe choice appears only for 1 and only if `CanPay(ContrabandHelper.GetBribeAmount())`, with the amount in its text | see below |
| Barter | `ClientIntent.BARTER`, `StoreClient.barterOffer : BarterOffer` | their items on the front counter, condition text `BarterOffer.GetItemPlacedResponse()` | `accept_offer` (`PlayerStore.AcceptBarter()`), stage/unstage, dismiss |
| Appraisal service | `ClientIntent.APPRAISAL_SERVICE` | the item; too-high/too-low reactions | `appraisal_estimate` (repeatable; `clientImpatience` +49 per refusal, leaves at 100, HIDDEN) |
| Info dealer | `ClientIntent.INFORMATION_DEALER` | `serviceCost`, pitch | `accept_offer` (pays) or dismiss |
| Gunsmith | `ClientIntent.GUNSMITH`, `gunOrderData : GunOrderData` | requirements, `GetTotalRewardDisplay() : System.String` | stage a gun, `accept_offer` |
| Landlord / loan shark | §2 | | `dialogue_choice` |
| Procurement | `ClientIntent.PROCUREMENT_OFFER/COLLECT` | — | **dead content**: `StoreClientListProcurement` factories are referenced nowhere |

### Inspection flow (high stakes)

1. Arrival (cadence HIDDEN; early-warning night logs if the service runs). The main dialogue ends
   in `StoreClientDialogList.SecurityDialogInspectionLastLine(System.Int32, System.Boolean)`;
   its start action calls `EmporiumAnalytics.TrackInspectorArrived(System.Int32 personality, System.Boolean fromShowcase)`
   — postfix it to emit `customer_observed {kind: inspection}` with the visible name.
2. **Decision**: `dialogue_choice` — "accept" (always) or "bribe N" (personality 1). Bribe →
   `EmporiumAnalytics.TrackInspectionBribe(System.Int32)` / `RunStats.OnInspectorBribe(System.Int32)`,
   pays, commits BRIBERY, dismisses — no search. The player can still move items until the
   choice is made (unverified that dragging is unlocked during this dialogue).
3. Search: `ContrabandHelper.StartInspection(System.Int32 personality, System.Boolean fromShowcase)`
   (public static) searches stock, both counters, showcase, trash (unless lazy) and the poster
   stash only while it is open (poster pulled up, `isPulled`); stolen items are found with
   `StolenHelper.GetFoundChance(GameItem)`; luck can void the search (HIDDEN roll).
4. Outcome: postfix `EmporiumAnalytics.TrackOnInspected(System.String outcomeType, System.Int32 lostCount, System.Int32 lostValue, System.Boolean usedGrace)`
   — `outcomeType` ∈ `CONTRABAND_LEVEL_NONE/LOW/MID/HIGH/CRITICAL`, `STOLEN_TAG`; then
   `RunStats.OnInspection(System.Boolean passed, System.Int32 seizedValue)`. HIGH = fine
   (arrest if unaffordable and no `SEC_GRACE`), CRITICAL = arrest unless `SEC_GRACE`
   (consumed). Clean inspection: +3 Security rep. Prefix `StartInspection` to snapshot what was
   exposed (owned items by `where`, poster state) for the training label.

## 5. Run level

| Item | Read / hook | Vis. |
|---|---|---|
| Run start | postfix `PlayerStore.InitialSave()` (after `StartNewGame`, starting items and perks; assigns `runID : System.String`, `runNumber : System.Int32`). `ModHook.OnNewGameStarted(string)` fires earlier with the scenario id but before `runID` exists | — |
| Start / scenario | `ps.startType : NewGameData+StartType` (WATER_MERCHANT, APPRENTICE, SCAV, FENCE, MOONSHINER, DRUG_DEALER, ORGAN_TRADER, PHARMACIST, STREET_FOOD_VENDOR, FARMER, GUNSMITH, ROCK_BOTTOM, RANCHER, MODDED, AQUAFARMER), `ps.startScenarioId : System.String`; `StartingScenario.GetCurrent() : StartingScenario` | VISIBLE |
| Starting perks | `ps.startingPerks : List<StartingPerk>` (`id`, `GetLocalizedDisplayName()`) | VISIBLE (calendar Status) |
| Difficulty | `ps.difficultyId : System.String` (`cozy`/`normal`/`hard`/`challenge` or a mod's), `ps.isHardMode`; rules `Difficulty.Rules : DifficultyRules` (static) — record `retailMarkup`, `rentIncreaseMultiplier`, `propertyBasePrice`, `vendorPermitFee`, `mortgage*`, `augGameOverAtFullPresence` | VISIBLE (difficulty screen effect lines, `Difficulty.GetEffectLines(Difficulty)`) |
| Flags | `ps.skipIntro`, `ps.endlessMode : System.Boolean` | VISIBLE |
| Run end | prefix `PlayerStore.ExecuteGameOver(System.String ending)` (every ending goes through it; it calls `EmporiumAnalytics.TrackGameOver` and `RunStats.OnRunEnded(System.Int32, System.Int32, System.Int32)` first) | |
| Continue after buyout | postfix `GameOverUIManager.ContinuePlayClick()` (only offered for `buyout*`) | |

Endings (`ending` argument) and their triggers:

| `ending` | Cause | Where |
|---|---|---|
| `rent` (also the default) | refused rent without `LANDLORD_GRACE` | landlord choice |
| `buyout` / `buyout_mortgage` | **win**: bought the store with cash / paid the last mortgage week; `showContinuePlay` | landlord / loan-shark collect |
| `repo` | refused a mortgage payment with too little stock to repossess (otherwise `PlayerStore.Repo()` seizes items and the run continues) | loan-shark collect |
| `arrest` | inspection CRITICAL without grace, unpaid HIGH fine without grace, sentencing officer | `StoreClientDialogList`, `StoreClientListSecurity` |
| `execution` | sentencing at ≥ 1500 crime value | `StoreClientListSecurity` |
| `mob`, `sec`, `hitman`, `cartel_retired` | morning visitor queued by a −100 perk (LL Lynch, SEC ImmediateArrest, UL Hitman, Cartel Retired) | `StoreClientListMorning` |
| `bombed` | REV −100 (Hit) | `PlayerStore.SetupRevBombing` → `StoreUIManager.TriggerBombing(System.String)` |
| `lou_bombed`, `lou_bombed_debt` | Lou storyline | `LouToyBoxHelper` |
| `sentinel` | sentinel addiction pool ≥ 20 | `HealthData` |
| `aug` | `augPresence ≥ 100` with `augGameOverAtFullPresence` or Aug Menace | `EndOfDayUIManager.OnEndDayButtonClick` |
| `reactor` | reactor explosion overnight | same |
| `cartel_retired` | also from cartel story choices | `StoreClientListCartel` |

## Riskiest / least certain

- **Human vs game moves** on the counters: the weighted-area callbacks do not say who moved the
  item. The suppression list in §3 is from reading the call graph, not from play.
- **Security record visibility** depends on owning the `intel_sec` document; how and when the
  player gets it is unverified. Until then, treat `security` as `visible:false` and log it in
  `hidden`.
- `DialogUIManager.SelectChoice` reads `currentDialog` (private field) through the public
  `GetCurrentDialog()`; choice labels are often empty, so the training signal is the choice
  **text**, which is localized.
- Dialogue choices built by `AddChoiceTargetable` (item dropped on the choice) and the response
  window (`EmporiumEntry.FindChoiceAction(GameItem)`) bypass `SelectChoice`; only one call site
  each was found, both in raid / unused content, so this is believed safe but unverified.
- Afterhour event rendering on the map and the clientele display's queue counts were read from
  code only.
- Postfixing `EmporiumAnalytics.Track*` depends on the game keeping those calls while analytics is
  disabled; `RunStats.On*` is the safer twin where both exist.
