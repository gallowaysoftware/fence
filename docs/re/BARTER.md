# Barter flow (RE notes)

Build: Steam `26525552`, `Application.version` 0.46D, `Assembly-CSharp.dll` decompiled with
ilspycmd. All types are in the global namespace of `Assembly-CSharp` unless qualified.
Negotiation is **not** a supported modding surface: `ModHook` has no barter/trade events
(only customer-generation, shutter, sleep, tooltip, … events). Everything below is Harmony
territory and can move on any patch — see "Hook stability" at the end.

## Correcting the protocol's mental model

There is **no free-form counter-offer and no "their price"**. The price of every item is
computed by the game (`GameItem.GetNegociatedValue() : System.Int64`) from base value ×
feature modifiers. The player's levers are:

1. **Which items** go on the back counter (the "weighted area"/scale). Only items there are
   traded.
2. **One bargain per side**, as a fixed percentage step from the Bargain panel.
3. **Appraisal** (exposing item features with the right tool), which changes the price.
4. **Accept** (Buy N / Sell N / Accept wholesale) or **dismiss** the customer.

`protocol/fixtures/decide.request.json` (`counter` with an int `price`) does not match the
game; the option set for `kind: "barter"` should be the discrete actions in the table under
"Buttons → methods".

## Lifecycle of an ordinary customer

| Step | Method (evidence) | Notes |
|---|---|---|
| Queue built at shutter open | `PlayerStore.OnShutterOpened() : System.Void` → `StoreClientManager.GenerateClient() : System.Void` | Fills `StoreClientManager.clientStack : List<StoreClient>`. Normal customers come from `HandleNormalClient()` → `PickClient() : StoreClient` → `StoreClientList.GetRandomClientPool(System.String).Pick(System.Int32)` (faction-weighted by `ComputeClientType() : System.String`). Mod events `ModHook.OnGenerateCustomer{VeryEarly,Early,Normal,Late,VeryLate}` (`Action<StoreClientManager>`) fire inside. |
| Customer called | `StoreBell.RingBell()` → `PlayerStore.TryCallNextClient() : System.Void` → `PlayerStore.GetNextClient() : System.Void` | Pops `clientStack[0]`, sets `PlayerStore.currentClientInstance : StoreClientInstance` (`StoreClientInstance.CreateClientInstance(StoreClient)`), starts walk-in tween `StoreClientMono.StartApprochingStore()`. Auto-next after a dismiss if `EmporiumOption.Instance.autoCallNext` (`PlayerStore.DismissCurrentClient`). |
| Arrives | `StoreClientMono.OnArrived() : System.Void` → `StoreUIManager.OnNextClientArrived() : System.Void` | Sets `PlayerStore.isClientArrived = true`; runs `StoreClient.OnArrival`; after `WaitAndDo(0.25f)` starts `StoreClient.mainDialogue` (`StoreClient.StartMainDialogue()`). |
| Offer created (SELL) | end action of the client's `mainDialogue`, e.g. `StoreClientList.CreateScavGeneral()` → `AddScavGeneralStock()` | Items are spawned onto the **front counter** via `PlayerStore.TryAddDirectSellingItemToTable(GameItem, System.Boolean isOwend, System.Boolean isStolen, System.Boolean ignorePerk, System.Int32 forcedHeat) : System.Boolean` (stolen heat assigned here by `StolenHelper.InitStolenItem`). Front counter can overflow: GameLog shows `Destroyed unplaced items: … (front counter full)`. |
| Offer created (BUY) | none — the client has wants, not items | `StoreClient.clientBuyingTagList`/`clientBuyingIdList`, `clientBlackTagList`, budget (`GetBudget()`), restrictions (`noAcceptingContraband`, `isRequireVariety`). The player puts **own** items on the back counter. |
| Item placed on back counter | `GameGridInventory.onSlotAddItemEndFunc` on `EmporiumEntry.backInvinvElement` → `PlayerStore.AddedItemToWeightedArea(GameItem) : System.Void` → `PlayerStore.OnWeightedAreaChanged() : System.Void` | Sets `PlayerStore.currentNegociatedItem`; `GameItem.OnAddedToWeightedArea()` adds client-specific features (stolen discount, client price modifier, perfume, …). For BUY: `PlacedItemForSelling(GameItem)` — client may **expose a fake feature** (`StoreClient.OnItemPlaced(GameItem)`; `NegociationHelper.OnFeatureExposed(StoreClient)` = −1 rep with that faction unless aug). For SELL: `PlacedItemForBuying(GameItem)`. Tablet auto-opens if `EmporiumOption.AutoOpenNegotiationUI` (`PlayerStore.HandleAutoNegoUIDisplay(GameItem)`). |
| Haggle | `BargainUIManager` (see table) | State lives in private fields of the `BargainUIManager` singleton, keyed to the current client (`trackedClient`, reset in private `Refresh()` when the client changes). Effects are stored as `ItemFeature`s on items (`bargainDiscount`, `bargainMarkup`, `bargainBuyingMarkup`, `bargainBuyingDiscount`, `blackmailDiscount`, `blackmailMarkup`, `threatenDiscount`, `threatenMarkup`), plus client budget changes and trade-rep multiplier overrides `StoreClient.sellTradeRepMultiplierOverride`/`buyTradeRepMultiplierOverride : System.Nullable<System.Double>`. |
| Deal | `NegociationUIManager.OnBuyAllClick()` / `OnSellAllClick()` / `OnWholesaleClick()` | See "Resolution". |
| Leaves | `PlayerStore.DismissCurrentClient(System.Boolean callNextWhenPossible, System.Boolean right, System.Boolean interruptDialog) : System.Void` | **Every** exit goes through this: player dismiss, bell, default "all done" dialogue end action (`StoreClient.AddBasicDialog()`), bargain walk-out, shutter close. Destroys un-owned counter items (`RemoveNotOwnedItemFromTable()`), nulls `currentClientInstance`. |

### Who owns the haggle state

| State | Owner |
|---|---|
| Current customer | `PlayerStore.currentClientInstance : StoreClientInstance` → `GetClientBlueprint() : StoreClient` |
| Item under negotiation | `PlayerStore.currentNegociatedItem : GameItem`; tablet's own `NegociationUIManager.currentNegociatedItem : GameItem` |
| Items in play | `EmporiumEntry.Instance.frontInvinvElement` / `backInvinvElement : GameGridInventory`; ownership `GeneralHelper.IsItemOwned(GameItem)` |
| Bargain flags (offered/applied/percent per side) | private fields of `BargainUIManager`; readable via public properties `DiscountOffered`, `MarkupOffered`, `BlackmailOffered`, `BuyingMarkupOffered`, `BuyingDiscountOffered`, `ThreatenOffered : System.Boolean`, `…Percent : System.Int32` |
| Tablet mode (Buying/Selling/Barter/InfoDealer/GunOrder) | private `NegociationUIManager.mode`, from `ResolveMode(StoreClient)` keyed on `StoreClient.clientIntent : StoreClient.ClientIntent` |
| Business done | `StoreClient.isBusinessComplete : System.Boolean` (`CompleteBusiness()`); "nothing left" check `StoreClientInstance.IsClientBusinessDone(StoreClientInstance) : System.Boolean` |

## Buttons → methods

All `NegociationUIManager` and `BargainUIManager` members are on the `Instance` singleton
(`public static … Instance { get; }`).

| UI control | Method | Guard inside the method | Guard only in the UI (mod must replicate) |
|---|---|---|---|
| Buy N items (client SELL) | `NegociationUIManager.OnBuyAllClick() : System.Void` | client present, `!IsTradeLocked()`, `!isBusinessComplete`, `!isRegularSellDisable`, total ≤ `playerCash` | — (buys **all** un-owned items on the back counter; if `isMultiBuyDisabled`, only the private `selectedItem` or the first) |
| Accept wholesale | `NegociationUIManager.OnWholesaleClick() : System.Void` | `isOfferWholesale`, not locked | `playerCash >= PlayerStore.GetWholesalePrice()` (**not** checked in the method — cash can go negative) |
| Sell N items (client BUY) | `NegociationUIManager.OnSellAllClick() : System.Void` | not locked, `!isBusinessComplete`; over budget → `OpenConfirmPanel()` | — |
| Confirm sell over budget | `NegociationUIManager.OnConfirmPanelYes() : System.Void` | not locked | panel open; sale is capped at remaining budget (`PlayerStore.SellItem`) |
| Accept offer (BARTER / info dealer / gunsmith) | `NegociationUIManager.OnOfferAcceptClick() : System.Void` | per intent | — |
| Bargain: Offer Discount −10/25/50/75 % (you sell) | `BargainUIManager.OfferDiscount(System.Int32 percent) : System.Void` | `CanOfferDiscount()` | **all bargain buttons**: `StoreClient.isBargainAvailable && PlayerStore.isBargainIntroduced && !StartingPerk.IsAwfulNegotiator() && !MapUIManager.Instance.isOutside` (`NegociationUIManager.RefreshBargainControl`), and not while `DialogUIManager.Instance.GetCurrentDialog() != null`, `StoreClientMono.Instance.IsMoving()`, `StoreUIManager.Instance.isAboutToStartMainDialog`, `NegociationUIManager.IsTradeLocked()` (`BargainUIManager.RefreshUI`). Percent must be one of 10/25/50/75 — the methods accept any int. |
| Bargain: Add Markup +% (you sell) | `BargainUIManager.OfferMarkup(System.Int32)` | `CanOfferMarkup()` | as above |
| Bargain: Offer Markup +% (you buy) | `BargainUIManager.OfferBuyingMarkup(System.Int32)` | `CanOfferBuyingMarkup()` | as above |
| Bargain: **Demand Discount** −% (you buy) | `BargainUIManager.DemandBuyingDiscount(System.Int32)` | `CanDemandBuyingDiscount()` | as above |
| Bargain: **Blackmail** 20 % | `BargainUIManager.Blackmail() : System.Void` | `CanBlackmail()` (not security) | as above |
| Bargain: Threaten (aug) 20 % | `BargainUIManager.ThreatenAug() : System.Void` | `CanThreatenAug()` | group only shown when `PlayerStore.isAugIntroduced` |
| Refuse / dismiss | `PlayerStore.TryPlayerDismissCurrentClient(System.Boolean callNextWhenPossible) : System.Void` | `tryInterceptDismiss` hook | `PlayerStore.lockNextBellFlag <= 0 && PlayerStore.IsClientDimissable()` (`StoreDismissButton.OnClick`) |
| Appraise (expose feature) | `ItemFeatureUI.OnFeatureUIClick()` → `ItemFeatureUI.ExposeFeature()` → `ItemFeature.ExposeFeature(System.Boolean, System.Boolean)` | needs the selected tool (`NegociationElement.GetNegociationToolID()`) to equal `ItemFeature.GetToolNeeded()`, or a matching notebook clue | Selection is UI state (`RTSController.previousSelectedObjects`). **No clean API**; calling `ItemFeature.ExposeFeature` directly skips tool ownership — that is cheating. |
| Open tablet | `NegociationUIManager.OpenUI()`; click on client `StoreClientMono.OnClick()` toggles | — | — |
| Report (wanted) / shoot | `PlayerStore.OnCustomerReported()`, shotgun | out of scope for P1/P2 | — |

## Bargain mechanics (verified in `BargainUIManager`)

`rep` = `StoreReputation.GetStoreReputation(client.clientFaction).GetReputation()`.
`event` = +15 if `client.eventSourceId` non-empty. `dm` = `PlayerStore.dealMakerBargainBonus`.

| Claim (community) | Verdict | Code |
|---|---|---|
| Steps 10/25/50/75 % | **Confirmed** for discount/markup groups | `PRICE_PERCENTS = {10,25,50,75}`; Blackmail and Threaten are a single 20 % button (`BuildGroup`) |
| One bargain per side per customer | **Confirmed**: one of {Offer Discount, Add Markup} when you sell, one of {Offer Markup, Demand Discount} when you buy | `CanOfferDiscount`/`CanOfferMarkup` false once `discountOffered || markupOffered`; same for buying side. Reset only on client change (`Refresh`). A **failed** roll still consumes the slot. |
| Discount offers can't follow coercion | **New** | `CanOfferDiscount`, `CanOfferBuyingMarkup` false after `blackmailOffered || threatenOffered`; Add Markup / Demand Discount are still allowed after coercion |
| One blackmail, one threaten | **Confirmed** | `CanBlackmail` false if `blackmailOffered`; `CanThreatenAug` false if `threatenOffered`; both false for `isSecurity` |
| Add Markup / Demand Discount accept = 100 − 2×pct | **Confirmed, with modifiers**: `100 − 2·pct + rep/6 (×2 if negative) + event + dm`; 0 if `alwaysRejectBadBargain`; aug clients always refuse (no walk-out) | `OfferMarkup`, `DemandBuyingDiscount`, `GetFactionRepBonus` |
| Refuse-but-stay = 100 − 1.2×pct, else walk out (−3 rep) | **Confirmed**: stay if `roll < 100 − 1.2·pct + rep/6' + event` (no `dm`) or `staysAfterFailedBargain`; walk-out = `ModReputation(faction, −3)`, `clientIntent = DIALOGUE`, dismissed at dialogue end | same |
| Offer Discount / Offer Markup always accepted | **Confirmed** (`GetBargainSuccessChance` = 100) | Offer Discount also **raises budget** by `2·pct %` of current budget |
| Add Markup cuts budget | **New**: on success budget −10/15/20/25 % for 10/25/50/75 | `GetMarkupBudgetCutPercent` |
| Blackmail 60 + rep/4, criminals only | **Confirmed**: `60 + rep/4 (×2 if negative) + event + dm`, 0 if `alwaysRejectBadBargain`. Success: −20 % on their items (selling) / +20 % on yours (buying). Fail: −5 rep. **Innocent** (`!StoreClient.IsWanted()`): −7 rep, or 50 %: −10 rep and walks out | `Blackmail`, `HandleInnocentBlackmail`. "Criminal" = `isWanted` **or** currently selling serious contraband or a stolen item (`StoreClient.IsWanted()`), so it is decidable from counter contents. |
| Threaten from day 12 | **Partly**: gated on `PlayerStore.isAugIntroduced`; `PlayerStore.GetAugIntroducedDay()` = 12 (1 with Aug Menace perk); the flag itself is set in `Shotgun.Unlock()` — exact trigger unverified | `UpdateGroupVisibility`. Success only on augs (`isAug`); on non-augs same penalty as innocent blackmail |
| Rep effect of haggling | **New**: trade rep multiplier per side becomes negative after a successful "bad" bargain (−0.5/−1.5/−3/−5 for ≤10/≤25/≤50/>50 %) or blackmail (−6), 0 after threaten; "nice" bargains add +0.65/+1.2/+2/+2.9 | `ComputeDirectionalMultiplier`, `GetBadBargainPenalty`, `GetNiceBargainBonus`; applied in `StoreReputation.OnItemTraded` |
| Chances visible? | **Yes** — hover tooltip prints accept/refuse/walk-out % (`GetBargainTooltip(BargainType, System.Int32)`), and the panel shows the rep multiplier line | `BargainButton.OnPointerEnter` |

## Resolution

| Path | Cash | Inventory | Reputation / heat |
|---|---|---|---|
| Player buys: `PlayerStore.BuyItem(GameItem)` | `playerCash -= (int)GetNegociatedValue()` | `OnItemBought` → `GeneralHelper.SetItemOwned`, `MoveDealItemToInventory` if `EmporiumOption.MoveBoughtItemsToInventory` | `StoreReputation.OnItemTraded(faction, cost, cost, client, isPlayerSold:false, item)`: rep += value × tradeMult (× stolen 1.5–3, × contraband level/2+1, × 0.66 LL / 1.2 UL) / 400 |
| Wholesale: `OnWholesaleClick` | `playerCash -= GetWholesalePrice()` | all un-owned items (front+back) | per item `OnItemBought(item, discounted)` |
| Player sells: `PlayerStore.SellItem(GameItem)` | `playerCash += min(value, budget)`; budget reduced | item un-owned, `SoldItem.TrackItem` | `PerformOnSoldCheck` → `SecData.CheckSoldContraband` (FENCING / *_TRAFFICKING crime), dirty water, endangerment; `OnItemTraded(…, isPlayerSold:true, …)` (+1 multiplier when budget-capped) |
| Sale blocked by client inspection | `PlayerStore.TryReinspectBeforeSale(GameItem) : System.Boolean` — client exposes a fake feature first, no sale | — | −1 rep (`OnFeatureExposed`) |
| Barter accept: `PlayerStore.AcceptBarter()` | — | swaps front/back ownership | `BarterOffer.OnBarterAccepted()` → `PlayerStore.OnItemsSold(List<GameItem>)` |

After a deal, `StoreClient.OnDealAccepted(Dialogue)` plays "after deal"/"all done" dialogue;
the default all-done dialogue's end action dismisses the client.

## Walking out

No patience meter for ordinary customers (only `clientImpatience`, used solely by the
appraisal service: +49 per refused estimate, leaves at ≥100 — `AppraisalServiceHelper.OnEstimate(GameItem, System.Int32)`).
A customer leaves only when:

| Cause | Where |
|---|---|
| Player dismisses / rings bell | `StoreDismissButton.OnClick`, `StoreBell.RingBell` |
| Business done → all-done dialogue | `StoreClient.OnDealAccepted` + `AddBasicDialog` end action; BUY done when `useClientBudget && GetBudget() <= 0`, SELL done when no un-owned items remain (`StoreClientInstance.IsClientBusinessDone`) |
| Failed Add Markup / Demand Discount roll beyond the "stay" band | `BargainUIManager.OfferMarkup` / `DemandBuyingDiscount` |
| Innocent blackmail / false threaten (50 %) | `HandleInnocentBlackmail`, `HandleFalseThreaten` |
| Shutter closed | `StoreShutterButton` → `DismissCurrentClient` |

Budget only limits what a BUY customer pays (`PlayerStore.SellItem` caps at budget;
`HasBudgetLeftToBuy(GameItem)`); it is shown on the tablet.

## Decision and outcome points

There is no single "player must choose now" method — the game is an open sandbox around the
counter. Recommended:

| Point | Member | Why |
|---|---|---|
| **Session start** | postfix `StoreUIManager.OnNextClientArrived() : System.Void` | Once per customer, after arrival; client blueprint is final. Public. |
| **Decision ready** (P1/P2) | Poll from the mod's own `MonoBehaviour.Update` (no hook): `currentClientInstance != null && isClientArrived && DialogUIManager.Instance.GetCurrentDialog() == null && !StoreClientMono.Instance.IsMoving() && !StoreUIManager.Instance.isAboutToStartMainDialog && !NegociationUIManager.IsTradeLocked() && client.isMainDialogueStarted`, plus intent-specific content (SELL: un-owned items on counters; BUY: client present). Fire once per (client, counter-signature). | This is exactly the predicate the game uses to enable the bargain buttons; reads only public state, so it survives refactors of private UI code. |
| Re-decide on counter change | postfix `PlayerStore.AddedItemToWeightedArea(GameItem)` / `PlayerStore.OnWeightedAreaChanged()` | Price changes when items move or features apply. Optional — polling a signature also works. |
| **Outcome: bought** | postfix `PlayerStore.OnItemBought(GameItem, System.Int32 cost)` | Single funnel for regular and wholesale buys; exact price. |
| **Outcome: sold** | postfix `StoreClient.OnItemBought(GameItem, System.Int32 amount)` (called from `PlayerStore.SellItem` with the budget-capped amount; names are client-perspective) | Exact cash received. |
| **Outcome: bargain result** | postfix on the six `BargainUIManager` methods, diffing `…Offered` properties, `StoreClient.clientIntent == DIALOGUE` (walk-out) and item features | Records accept/refuse/walk-out for memory. |
| **Outcome: customer left** | prefix `PlayerStore.DismissCurrentClient(System.Boolean, System.Boolean, System.Boolean)` | Every exit path; prefix still sees `currentClientInstance`. |

## Coverage by client kind

| Client source | Intents (count of assignments) | Uses ordinary BUY/SELL flow? |
|---|---|---|
| `StoreClientList` (scavs, normal pool, merchant, landlord, inspection) | 22 BUY, 18 SELL, 8 SELLNBUY, 9 DIALOGUE, 1 INSPECTION, 1 RENT | yes for BUY/SELL/SELLNBUY |
| `StoreClientListTier*`, `Shortage`, `Surplus`, `Water`, `Minor`, `Church`, `Rev`, `Event` | BUY / SELL / SELLNBUY | yes (some with `isRequireVariety`, wholesale, `noAcceptingContraband`) |
| `StoreClientListSupplier` | 10 SELL, 1 SELLNBUY | yes; usually `isOfferWholesale` |
| `StoreClientListWanted` | 8 BUY, 6 SELL, 12 DIALOGUE, 1 INSPECTION, 1 BARTER | yes for BUY/SELL, plus extra Report/shoot choice the first integration should defer |
| `StoreClientListCartel` | 2 BUY, 1 SELL, 5 DIALOGUE | trades yes; DIALOGUE is story (defer) |
| `StoreClientListStory`, `StoreClientUniqueList`, `StoreClientListSpec` | many DIALOGUE + BUY/SELL | trades yes but often scripted (`acceptDealDialogue`, `tryInterceptDismiss`, `dismissable=false`) — defer when `!dismissable` |
| `StoreClientListBarter` | BARTER (`BarterOffer`) | **own flow**: item-for-item, `BarterOffer.IsBarterAcceptable()` → `OnOfferAcceptClick` → `PlayerStore.AcceptBarter()` |
| `StoreClientListAppraisal` | APPRAISAL_SERVICE | **own flow**: numeric estimate `AppraisalServiceHelper.OnEstimate(GameItem, System.Int32)`, impatience |
| `StoreClientListInformation` | INFORMATION_DEALER | own (pay `serviceCost`) |
| `StoreClientListGun` | 4 GUNSMITH (+BUY/SELL) | own (`GunOrderData`) |
| `StoreClientListProcurement` | PROCUREMENT_OFFER/COLLECT | own (`ProcurementData`) |
| Security inspection | INSPECTION | own (`InspectionUIManager`) |
| `StoreClientListSecurity`, `Morning` | DIALOGUE | no trade |

First integration (BUY, SELL, SELLNBUY with `dismissable == true` and no `isWanted`) covers
the bulk of daily traffic; everything else → `defer`.

## Hook stability

Prefer, in order: public fields/properties read by polling; public "button" methods
(`OnBuyAllClick`, `OnSellAllClick`, bargain methods, `TryPlayerDismissCurrentClient`);
postfixes on public funnel methods (`OnItemBought`, `DismissCurrentClient`,
`OnNextClientArrived`). Avoid private members (`NegociationUIManager.RefreshNow`, `mode`,
`selectedItem`, `PlayAcceptDealDialogueOnce`). Unknowns:
- Whether `GraphUtils.TryAcceptAll(GraphNodeStorage, GameItem, System.Int32)` into
  `backInvinvElement` fires `onSlotAddItemEndFunc` exactly like a drag (needed for P2 to
  stage items). Verify in-game in `advise` mode first.
- `BargainUIManager` methods rely on `PlayerStore.RefreshCounterItem()` and start dialogues;
  calling them while the panel is closed looks safe (state is in `Refresh()`), unverified.

## Mechanics Q&A (follow-up)

| # | Question | Answer | Evidence |
|---|---|---|---|
| 1 | Failed blackmail on a real criminal (`IsWanted()`)? | Client **stays**. Only effects: faction rep -5 (via `StoreReputation.ModReputation(string, int)`; the per-faction multipliers in the instance `ModReputation(double)` such as Black Market x2 loss are presumably applied too, unverified for the static overload), a reject dialogue plus a "bad talk" client line. `clientIntent` is not changed and there is no dismiss. `blackmailOffered` stays true, so the option is spent for this client, and `CanBlackmail`/`CanThreatenAug` also stay closed (Threaten checks `threatenOffered` only; Blackmail checks both). Success chance is 60 + rep bonus + 15 (event customer) + Deal Maker bonus; rep bonus is rep/4, doubled when negative. | `BargainUIManager.Blackmail()` failure branch; `BargainUIManager.CanBlackmail()`; `GetFactionRepBonusBlackmail(string)`; `BargainUIManager.IsCriminal()` -> `StoreClient.IsWanted()` |
| 2 | Does `demand_discount_N` change `GetWholesalePrice()`? | Yes, indirectly. The demand adds a `bargainBuyingDiscount` feature (-N% value mod) to every **non-owned** item on both counters and to items added later. `GetWholesalePrice()` sums `GetNegociatedValue()` over `GetAllNonOwnedItem()` and then applies the blueprint's `wholesaleDiscount`, so the discounted per-item values flow into it. There is no separate wholesale path. (That `GetNegociatedValue` honours the feature is inferred from the feature's `ValueStage.Final` mod; I did not trace the GameItem value pipeline.) Augs never get it: `trackedClient.isAug` -> `RefuseAugHaggle`. Demand is only offered when `IsClientSelling()`. | `BargainUIManager.DemandBuyingDiscount(int)`, `ApplyNonOwnedItemFeature(Func<ItemFeature>, string)`, `ItemFeatureList.BargainBuyingDiscount(int)`; `PlayerStore.GetWholesalePrice()` returns -1 if `!isOfferWholesale` |
| 3 | Selling contraband to a `noAcceptingContraband` client? | **Blocked, no penalty path.** `IsClientRefusingItem` returns true for contraband when the flag is set, so `IsClientBuyingThisItem` is false and `CanSellThisItem` is false. The sell and confirm paths both call `CanSellItemFromTable`, so even the over-budget confirm panel never opens. Placing such an item on the counter just plays the "no contraband" dialogue and shows the NoContraband tag. The flag is set 35% of the time for `LOWER_LEVEL_CITIZEN` clients, and cleared when intent is not BUY/SELLNBUY or when `AcceptingContrabandOverride` is set. | `StoreClient.IsClientRefusingItem(GameItem, bool)`; `StoreClient.IsClientBuyingThisItem(GameItem, bool)`; `PlayerStore.CanSellThisItem(GameItem, bool)`; `PlayerStore.PlacedItemForSelling(GameItem)`; `PlayerStore.SellItemFromTable()` |
| 4 | Is BUYING stolen goods a crime/heat/security event by itself? | **No.** The buy funnel does no `SecData` work: `BuyItem` -> `OnItemBought` only does rep, price tracking, stats and ownership. Crime is committed on **sale**: `SellItem` -> `PerformOnSoldCheck` -> `SecData.CheckSoldContraband`, which calls `CommitCrime("FENCING", ...)` for `STOLEN_TAG` items (value scaled by `STOLEN_VALUE_INT`%) and the drug/weapon/card/contraband trafficking crimes for `CONTRABAND_ITEM_TAG` (skipped if the buyer has `isNoContributeToEvidence`). Owning stolen stock is a **latent** risk: a Security client arriving finds stolen items on the counter or display case and sets heat to 100 (`dismissable=false`), and `GetFoundChance` rolls apply elsewhere (`ShowcaseHelper`, security-arrival dialogues). Faction effect: buying is a rep **gain**. In `OnItemTraded` a stolen item multiplies the gain by 1.5/2/2.5/3 (stolen value <=25/50/75/else), serious contraband by level/2+1, then the usual faction modifiers (Lower x0.66, Upper x1.2, Diplomatic x1.33, Blabbermouth halves positive Black Market and Revolution gains, Sycophant x1.25 on Upper). The stolen/contraband multipliers are **not** faction-specific; only the generic per-faction tweaks differ. | `PlayerStore.BuyItem(GameItem)`; `PlayerStore.OnItemBought(GameItem, int)`; `PlayerStore.PerformOnSoldCheck(GameItem, StoreClient)`; `SecData.CheckSoldContraband(GameItem, StoreClient)`; `StoreReputation.OnItemTraded(string, int, int, StoreClient, bool, GameItem)`; `StolenHelper.GetStolenItemTradingRepBonus(GameItem)`; `StoreClient` security-arrival hook (stolen on counter/display -> `StolenHelper.SetHeat(.., 100)`) |
| 5 | Reputation status names, -200 to +200 | **Positive** (`amount > 0`): 1-9 Neutral, 10-19 Known, 20-29 Established, 30-39 Recognized, 40-49 Dependable, 50-59 Recommended, 60-69 Trusted, 70-79 Highly Rated, 80-89 Renowned, 90-99 Esteemed, 100-200 Prestigious. **Zero and slightly negative**: -9..0 Neutral. **Negative**: -10..-19 Inconsistent, -20..-29 Disfavored, -30..-39 Unreliable, -40..-49 Shady, -50..-59 Dubious, -60..-69 Questionable, -70..-79 Notorious, -80..-89 Disreputable, -90..-99 Avoided, -100..-149 Boycotted, -150..-200 Blacklisted. Names are loc keys `rep_*` (English text assumed to match the key; localized strings not checked). Rep is clamped to [-200, 200]; the function takes the truncated `int`. | `StoreReputation.GetReputationStatus(int)`; clamp in `StoreReputation.CheckReputation()`; `GetReputation()` truncates the double |
| 6 | Why is Threaten 100% on augs, and what happens on a non-aug? | There is no roll: `ThreatenAug()` sets `threatenApplied = true` whenever `trackedClient.isAug`, so `GetBargainSuccessChance` hard-codes 100 for Threaten to mirror that. The tooltip still takes the "roll" branch (`BargainUsesRoll` includes Threaten, `BargainCanLeave` does not), but the number is always 100. On success a -20% `threatenDiscount` (client selling) or +20% `threatenMarkup` (client buying) feature goes on un-owned/owned items. The success chance shown is **not** conditional on the client: it does not check `isAug`, so on a non-aug the tooltip still says 100 while the actual outcome is the failure path (UI misleads, unverified whether the button is hidden elsewhere). **Non-aug**: `HandleFalseThreaten()`: 50% roll -> rep -10, client intent set to DIALOGUE and dismissed after leave and bad-talk lines; otherwise rep -7 and the client stays. This is the same structure and numbers as `HandleInnocentBlackmail()`, so BARTER.md is confirmed. Threaten is unavailable against `isSecurity` clients; `threatenOffered` makes it one-shot. | `BargainUIManager.ThreatenAug()`; `BargainUIManager.HandleFalseThreaten()`; `BargainUIManager.HandleInnocentBlackmail()`; `BargainUIManager.GetBargainSuccessChance(BargainType, int)`; `BargainUIManager.CanThreatenAug()`; `BargainUsesRoll(BargainType)` |
