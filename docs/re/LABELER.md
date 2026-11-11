# Labels, fakes and who exposes them (RE notes)

Same decompile as `BARTER.md` (build 0.46D). Questions answered here: how a fake feature on an item the
player owns can be identified and dealt with, what the labeler does, and which buyers expose what, so a
policy can label for the buyer in front of it. The fakes themselves (injectors, cigarettes, stamps) and
their visible cues are in `INSPECT.md`.

## A feature's claim and its truth

An item feature with conditions (`ItemFeature.useCondition`) carries a claimed condition
(`fakeCondition`, what is shown and priced) and a real one (`realCondition`). The price follows the
claim until the feature is exposed (`GetCurrentItemCondition`). For injectors, cigarettes and stamps the
condition's activate action rewrites the item's name and `unitValue` (Immunivax 160 → expired 80,
counterfeit 40, unusable 8). The claim matches the truth when the two identifiers are equal
(`IsFeatureMatch`). A matching claim can never be exposed.

## Exposing a fake on an owned item

- **The confront panel does not work on owned goods.** `InspectionUIManager.CanOpenConfrontPanel` requires
  the item not to be owned, so the magnifier's confront (`OnConfront`) is a buying-time tool only.
- **The negotiation-tool route is dead.** `ItemFeatureUI.OnFeatureUIClick` exposes a feature when the
  selected tool's id equals `ItemFeature.GetToolNeeded()`, but no feature ever gets a tool (`SetTool` has no
  caller).
- **Scanners** expose water and chemical purity (`WaterScannerUIManager`, `ChemScannerUIManager`), and
  using up a food stamp exposes a fake one.
- **The magnifier still shows the cues.** `InspectionUIManager.OnInspectItem` renders an owned injector or
  cigarette pack like any other (the view, not the confront panel). For injectors the cues fix the truth
  exactly (`InsInjectorHelper`: indicator expired → expired; indicator unusable, a broken body or a
  recalled serial prefix → unusable; an off-colour body, a wrong company or model, a malformed or
  out-of-range serial → counterfeit; each fake has exactly one fault). For cigarettes any non-reference
  seal, lettering, logo or box colour means counterfeit (`InsCigaretteHelper.SetConterfeit`).
- So for an owned fake, the player's tool is **the labeler**: label the claim to the truth the magnifier
  showed, and the item becomes an honest one (match: unexposable, priced at its true value).

## The labeler

`labeler` (tool, worth 50, `IMPORTANT_TAG`; `ToolDirectory.CreateLabeler`). Targets an owned item with at
least one editable feature and opens `LabelerUIManager`. Each feature row is a dropdown of the category's
conditions (`ItemConditionList.conditionLookup`). Picking one (`LabelerElement.OnDropdownValueChanged`)
sets `fakeCondition` to it and calls `SetExposed(false)`, which runs the new claim's activate action (the
price follows at once). Editable categories (`LabelerHelper.IsEditableLabelFeature`): open box, rusted,
dented, expired, water purity, cigarette, stamp and injector genuineness, free, replica, chemical purity.
Ingot purity, equipment condition and wine quality are refused (`CanEditLabel`). Food freshness and stuck
modules are not in the lookup, so they cannot be labelled either. A "repeat" button reapplies the last labels.

Where to get one: skip-intro starts have it in stock (`PlayerStore.HandleSkipIntro`). With the intro it is
handed over on the counter on day 4 (`StoreShutterButton`, with a mentor note), and like everything on the
counter it is destroyed that night (`EmporiumEntry.ClearFrontInv`) unless put away. The inventor's shop
stocks one when the player owns none (`StoreClientList.PlaceInventorInventory`).

## Who exposes what

Exposure at the counter is deterministic: no roll. When an item is placed for a buyer
(`StoreClient.OnItemPlaced`), and again before each sale (`PlayerStore.TryReinspectBeforeSale`), the buyer
exposes every feature that `CanClientExposeThisFeature` allows. A condition feature needs a claim better
than the truth (`IsRealFeatureCheaper`), not exposed, and the buyer's capacity function. That function is
fixed by client type (`StoreClient` faction setup, `ClientCanExposeFunc`):

| capacity (`StoreClient.CompleteClientCreation`) | clients | cigarettes | injectors (genuine claim) | water | chemicals |
|---|---|---|---|---|---|
| `ScavClientExposeCapacity` | any client tagged `SCAV` | yes | unusable*, counterfeit, expired | flashlight grades | if the client checks chemicals |
| `LLClientExposeCapacity` | Lower Levels and Black Market | yes | unusable*, counterfeit, expired | test strip | if the client checks chemicals |
| `SecClientExposeCapacity` | Security | yes | unusable*, counterfeit, expired | test strip | if the client checks chemicals |
| `TouristClientExposeCapacity` | tourists | no | unusable*, expired | flashlight grades | no |
| `ULClientExposeCapacity` | Upper Levels | no | unusable*, expired | flashlight grades | if the client checks chemicals |
| `BasicClientExposeCapacity` | everyone else (Revolution, story and generic clients) | yes | no | flashlight grades | no |
| `TradeBoardExposeCapacity` | the trade board official | every mismatched label in the lookup | yes | yes | yes |

A few client definitions set their own capacity (`StoreClientListTier`, `StoreClientListWanted`, the
`Rev`/`Odin`/`ALL` functions). "Flashlight grades" are the two worst water grades (rust, gutterflow).

\* not when a recalled serial is its only fault (`InsInjectorHelper.IsOnlyIssueRecalled`).

No counter buyer except the trade board official checks open-box, rust, dent, expiry or replica labels. A
second set of checks, by client tags (`NegociationHelper.IsClientNegociating`: `CHECK_RUST`, `CHECK_DENTED`,
`CHECK_EXPIRED`, `CHECK_OPEN_BOX`, …, −1 rep), has no caller in this build.

**Cost of an exposure.** `StoreClient.ClientExposeFeature`: −4 reputation with the buyer's faction (none
from an aug; −5 on hard, as the gym measures it); the feature is exposed (the price
falls to the truth) and the buyer says so. There is **no crime** for a counter exposure.

**Crimes from mislabelling** come only from:
- the vending machine: each mislabelled item sold adds its price to `vendingMislabelFraudValue` (or the
  endangerment value for dangerous ones), committed as FRAUD (or ENDANGERMENT) at night, with rep loss
  (`VendingMachineHelper.TrackMislabelledSale`, `SettleVendingMislabels`); marketplace requests likewise
  (`TrackMislabelledRequestItem`);
- water sold under a better grade (`SecData.SellMislabeledWater`, FRAUD every 2 500 ml) and bad chemicals
  (`SecData.CheckSoldChem`), whether or not the buyer exposes them.

So at the counter, a mislabelled injector, cigarette or condition label sold to a buyer who cannot expose
it carries no penalty in this build. A buyer who can costs −5 rep on hard and the sale at the claimed price.

## In the gym

`sim/gym/Labels.cs` (both off by default):
- `fakes: "honest"`: each morning, with a magnifier and a labeler owned, every owned injector or cigarette
  pack claimed genuine is read off its cues, and one with a fault is labelled to its truth.
- `labeler: "per_buyer"` adds to that: while a buyer whose type cannot expose a genuine claim is at the
  counter, the identified fakes are labelled genuine for him and back to the truth after he leaves. Each
  morning open-box, rust, dent, expiry and replica claims go to the best condition. Relabelled goods are
  kept out of the vending machine.
The labeler window is stubbed headless, so the executor applies `OnDropdownValueChanged`'s change itself.
The day-4 labeler and the magnifier are put away in the dossier.
