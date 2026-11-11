# Readable state (RE notes)

Build 26525552 / 0.46D. **VISIBLE** = the player can see it in the UI (evidence = the UI
code that renders it). **HIDDEN** = internal only. **DERIVABLE** = not shown directly but
computable from visible facts. Singletons: `PlayerStore.Instance`, `StoreStation.Instance`,
`EmporiumEntry.Instance`, `NegociationUIManager.Instance`, `BargainUIManager.Instance`,
`DialogUIManager.Instance` (all `public static … Instance { get; }`).

## Player / store

| Value | Read from | Vis. | Evidence |
|---|---|---|---|
| Cash | `PlayerStore.playerCash : System.Int32` (or `GetCash() : System.Int32`) | VISIBLE | tablet footer "your funds" (`NegociationUIManager.BuildSelling`) |
| Day | `StoreStation.dayCounter : System.Int32` (`StoreStation.GetDayCounter()` static) | VISIBLE | calendar / day UI |
| Store state | `PlayerStore.storeState : PlayerStore.StoreState` (MORNING/OPEN/CLOSED…) | VISIBLE | shutter |
| Queue length | `PlayerStore.GetClientInQueueCount() : System.Int32`; queue `storeClientManager.clientStack : List<StoreClient>` | count VISIBLE (unverified UI), **identities HIDDEN** | clientStack is `[ES3NonSerializable]`; next customer not shown |
| Rent due | `PlayerStore.GetRentValue() : System.Int32`, `GetRentDayCounter() : System.Int32` | VISIBLE (assumed: rent UI) | — |
| Retail markup | `PlayerStore.retailMarkup : System.Int32` (default 15) | VISIBLE (setting) | `GameItem.GetCurrentValue(useRetailMarkup)` |
| Store attractiveness | `PlayerStore.GetCurrentStoreAttractiveness() : System.Int32` | unverified | drives customer tier and budget bonus (`StoreClient.ApplyBudgetModifier`) |

## Reputation (6 factions)

| Value | Read from | Vis. |
|---|---|---|
| Per-faction rep (−200…200) | `PlayerStore.storeReputations : List<StoreReputation>`; `StoreReputation.factionId : System.String`, `GetReputationExact() : System.Double` | VISIBLE — `ReputationElement` prints `F2 + "/200"` and status |
| Status label | `StoreReputation.GetReputationStatus(System.Int32) : System.String` (static; steps every 10, e.g. 10 Known … 100+ Prestigious; −10 Inconsistent … −150 Blacklisted) | VISIBLE |
| Perks & thresholds | `StoreReputation.perks : List<StoreReputationPerk>`; `perkId`, `perkThreshold : System.Int32`, `isUnlocked`; `StoreReputation.IsPerkUnlocked(System.String)` | VISIBLE (`ReputationUIManager` shows `threshold/200`) |

Faction ids: `FACTION_LOWER_LEVEL`, `FACTION_UPPER_LEVEL`, `FACTION_SECURITY`,
`FACTION_BLACK_MARKET`, `FACTION_REVOLUTION`, `FACTION_CARTEL` (only exists for the
`DRUG_DEALER` start — `StoreReputation.InitReputation()`). Customers can also be
`FACTION_TOURIST`, `FACTION_CHURCH`, `FACTION_MIDDLE`, which have no rep bar.

Perk thresholds (from `StoreReputationPerk` factories): LL 20 Referral, 40 RareGood, 60 Tips,
80 TightLips, 100 Pillar; −20 PoorReviews, −40 Distrusted, −60 Target, −80 Avoided, −100 Lynch.
UL 0 DiscerningTaste, 20 Tips, 40 Charity, 60 Gossip, 80 HouseholdName, 100 TopReview; −40
TouristTrap, −60 Petition, −100 Hitman. SEC 0 Requisition, 20 Surplus, 40 Protection, 60
Trusted, 80 ReliableVendor, 100 VIP; −20 Watchlist, −40 AlarmIgnored, −60 Priority, −80
ThoroughInspection, −100 ContrabandTolerance/ImmediateArrest. BM 0 Honor, 20 Broker, 40
Reliable, 60 Network; −40 Distrusted, −60 Uninsurable. REV 20 Request, 40 Quartermaster, 60
Friend, 80 RaidLoot; −40 Blacklisted, −100 Hit. CARTEL 25 IncreasedStipend, 50
ReliableChemist, 100 AuthorizedExit; −100 Retired.

Rep mechanics that matter for barter: trade rep = value × tradeMult / 400 (× 0.66 LL, × 1.2 UL,
× 1.5–3 when buying stolen, × contraband level) — `StoreReputation.OnItemTraded(System.String,
System.Int32, System.Int32, StoreClient, System.Boolean, GameItem)`; BM losses ×2
(`ModReputation(System.Double)`). The bargain panel shows the live multiplier and rep per 1000
credits (VISIBLE, `BargainUIManager.UpdateRepGainText`).

## Current customer — `PlayerStore.currentClientInstance.GetClientBlueprint() : StoreClient`

| Value | Member | Vis. | Evidence / note |
|---|---|---|---|
| Name | `displayName : System.String` | VISIBLE | tablet header |
| Id | `identifier : System.String` | HIDDEN (stable key for memory) | |
| Faction | `clientFaction : System.String` | VISIBLE indirectly | dialogue bubble color by faction (`DialogUIManager.SetClientBubbleColor`), rep line in bargain panel; not printed as text |
| Intent | `clientIntent : StoreClient.ClientIntent` | VISIBLE | tablet section title (`NegociationUIManager.SectionTitle`) |
| Wants (BUY) | `clientBuyingTagList`, `clientBlackTagList : List<System.String>` | VISIBLE | "Interests"/"Restrictions" rows (`BuildInfoBlock`); `clientBuyingIdList`/`clientBlackIdList` HIDDEN |
| Budget (BUY) | `useClientBudget : System.Boolean`, `GetBudget() : System.Int32` | VISIBLE | tablet footer "client budget" (`BuildBuying`) |
| No contraband / variety / security | `noAcceptingContraband`, `isRequireVariety`, `isSecurity : System.Boolean` | VISIBLE | tags in `BuildInfoBlock` |
| Attracted by | `attractedItemId`, `attractedType : System.String` | VISIBLE | header subtitle |
| Wholesale offer | `isOfferWholesale : System.Boolean`, `wholesaleDiscount : System.Int32`, `PlayerStore.GetWholesalePrice() : System.Int32` | VISIBLE | `BuildSelling` |
| Wanted | `isWanted : System.Boolean`, `wantedPosterId : System.String` | DERIVABLE (posters in `WantedUIManager`); `isStealthWanted` HIDDEN | |
| Criminal for blackmail | `StoreClient.IsWanted() : System.Boolean` (= isWanted ∨ selling serious contraband ∨ selling stolen) | DERIVABLE | stolen/contraband tags are in item tooltips |
| Aug | `isAug : System.Boolean` | HIDDEN (idle-animation tells; aug scanner tool) | `StoreClientMono.PlayIdleForCurrentClient` |
| Bargain chances | `BargainUIManager.GetBargainSuccessChance(BargainType, System.Int32)`, `GetBargainLeaveChance(BargainType, System.Int32) : System.Int32` | VISIBLE | button tooltip (`GetBargainTooltip`) — note this leaks `alwaysRejectBadBargain` (0 %) and `staysAfterFailedBargain` (0 % walk-out) to the player too |
| Mood / patience | — | n/a | no mood/patience for ordinary customers; `clientImpatience : System.Int32` is appraisal-only |
| Dismissable | `dismissable : System.Boolean` | VISIBLE | dismiss-button tooltip |
| Business done | `isBusinessComplete : System.Boolean` | VISIBLE (dialogue) | |
| Price modifiers | `buyPriceModifier`, `sellPriceModifier : System.Int32`, `tradeReputationMultiplier : System.Double` | VISIBLE via resulting item features / rep line | |

## Items

Counters: `EmporiumEntry.frontInvinvElement`, `backInvinvElement : GameGridInventory`
(`childItems : List<GameItem>`); helpers `GetAllNonOwnedItem()`, `GetBackInvItems()`,
`GetOwnedItemsBack() : List<GameItem>`. Ownership `GeneralHelper.IsItemOwned(GameItem) : System.Boolean`.
Own stock: `EmporiumEntry.invElement : GameGridInventory`, showcase `showcaseElement`;
`EmporiumEntry.GetAllOwnedItems(System.Boolean, System.Boolean) : List<GameItem>`.

| Value | Member | Vis. | Evidence |
|---|---|---|---|
| Identity | `GameItem.identifier : System.String`, `GetDisplayName(System.Boolean) : System.String` | VISIBLE (name) | tablet row |
| Deal price | `GameItem.GetNegociatedValue() : System.Int64` | VISIBLE | tablet row / total (`TabletUI.Credits(item.GetNegociatedValue())`) |
| Estimated value | `GameItem.GetCurrentValue(System.Boolean, System.Boolean, System.Boolean, System.Boolean, System.Int64) : System.Int64` and `GetBaseValue() : System.Int64` | VISIBLE | tooltip `mech_tooltip_est_value` (`GameItemElement.GetTooltipBasic`) |
| Bought at | tag `BOUGHT_PRICE_TAG` (`GetTagReadonly(System.String).GetInt()`) | VISIBLE | tooltip |
| Condition/features as claimed | `GameItem.itemFeatures : List<ItemFeature>`; `ItemFeature.GetPublicDisplay() : System.String`, `GetValueStage()`, `GetCurrentValueMod() : System.Int32` | VISIBLE if `IsInitiallyShown() && !isDisabled && (!GetPublicHidden() || IsExposed())` | `NegociationUIManager.AddFeatureSection`, rendered in the expanded row under Condition / Market / Bargain |
| True condition | `ItemFeature.realCondition : ItemCondition`, `GetActualValueModifier()`, `IsFeatureMatch()` | **HIDDEN until exposed** (`IsExposed()`), then `GetExposedString()` is shown | the price uses the **fake** modifier until exposure (`GetCurrentValueMod`) — so `GetNegociatedValue` does not leak truth |
| Tool needed to expose | `ItemFeature.GetToolNeeded() : System.String` | HIDDEN as data; learned by the player through play | `ItemFeatureUI.OnFeatureUIClick` |
| Will a buyer catch a fake | `StoreClient.CanClientExposeAnyFeatureDeep(GameItem) : System.Boolean` | HIDDEN | depends on `realCondition`; do not send |
| Stolen + heat | `StolenHelper.IsStolenItem(GameItem) : System.Boolean`; heat `STOLEN_VALUE_INT` tag (0–100, decays 18/night for every stolen item, contraband included — the contraband slow-down in `UpdateStolenItem` never fires; measured, docs/sim/VERIFIED.md) | VISIBLE | tooltip `mech_stolen_tooltip` with the number (`StolenHelper.CreateStolenItemTooltip`); also a stolen discount feature when buying (`ApplyStolenDiscount`) |
| Contraband | `ContrabandHelper.IsContraband(GameItem)`, `IsSeriousContraband(GameItem) : System.Boolean`, `GetContrabandLevelInInt(GameItem) : System.Int32` | VISIBLE | tooltip `ContrabandHelper.CreateContrabandItemTooltip` |
| Appraisal true value | `AppraisalHelper.GetCorrectBaseValue(GameItem) : System.Int32` | **HIDDEN** ("TRUE VALUE" line is dev-tooltip only) | `AppraisalHelper.CreateAppraisalTooltip` |
| Market events | `StoreStation.storeEventManager.GetActiveEvents()` → `StoreEvent.negociationDatas : List<NegociationData>` (`itemType`, `value`) | VISIBLE (news / Market section) | `GameItem.GetCurrentValue` |

## Heat / security

| Value | Member | Vis. |
|---|---|---|
| Crimes, evidence, projected sentence | `PlayerStore.secData : SecData`; `GetTotalTraffickingCrimes()`, `GetEvidenceLevelDisplay()`, `GetSuspectedCrimeDisplay()`, `GetProjectedSentence() : System.String` | VISIBLE in the record UI (`RecordUIManager.OpenUI`) — raw `crimeAmount` lists HIDDEN |
| Days to next inspection | `StoreClientManager.dayUntilInspection : System.Int32`, `IsInspectionCountdownActive()` | HIDDEN; announced 3/2/1 days ahead only with the early-warning service (`PlayerStore.HandleInspectionEarlyWarning`) |
| Inspection cadence | `ComputeInspectionCooldownRange` (private): 3–5 days below 150 trafficking, down to 1–2 at ≥750; none below 50 or with SEC_VIP. "Trafficking" includes FENCING; FRAUD does not count (measured, docs/sim/VERIFIED.md) | HIDDEN (rule) |
| Fine on found contraband | `PlayerStore.GenerateContrabandFineFromItem(List<GameItem>) : System.Int32` | VISIBLE at inspection |

## Recommendation

Send the LLM only VISIBLE + DERIVABLE values. The game is already designed so that the
displayed price (`GetNegociatedValue`) is computed from public (fake-until-exposed)
features, so a player-visible context loses nothing needed for a fair decision. Never send
`realCondition`, `GetCorrectBaseValue`, `isAug`, `isStealthWanted`, `dayUntilInspection`,
`CanClientExposeAnyFeatureDeep`, or the queue contents.
