# Visual inspection mini-games (RE notes)

Same decompile as `BARTER.md` (Assembly-CSharp, global namespace). Question answered here:
which inspection mechanics can an LLM play **fairly** — from what a player can see, never
from the truth flag (`PROTOCOL.md` "Information boundary").

## Shared machinery

| Piece | Evidence | Notes |
|---|---|---|
| The tool | `ToolDirectory.CreateMagnifier()` (tags `MAGNIFYING_GLASS`, `CAN_USE_ON_NON_OWNED_TAG`): targets any item with `INSPECTABLE_TAG`, calls `InspectionUIManager.OnInspectItem(GameItem)` | The player must own the magnifier. Not to be confused with the counter **magnifying screen** (`StoreMagnifyingGlass.OnClick` → `StoreUIManager.EnterMagnifyingMode()` / `ToggleMagnifyZoom()`), which zooms the camera on the customer (aug tells, below). |
| Inspection panel | `InspectionUIManager.OnInspectItem(GameItem)` dispatches on tags (`CIGARETTE_TAG`, `FOOD_STAMP_TAG`, `INS_INJECTOR_TAG`, `PLASMA_CELL_TAG`, `RIBWICH_TAG`, `HAND_TAG`, `BERRY_TAG`, `APPRAISAL_ITEM`) to a pair of views (`*UISmall` / `*UILarge`, each `OnInit(GameItem)`), plus a magnifying-glass overlay (`magnifyingGlass`) | No `ModHook` event. Hook = Harmony postfix on `OnInspectItem`, or poll `InspectionUIManager.Instance.panel.activeSelf` + `currentInspectingItem`. |
| Claim vs truth | `ItemFeature.SetConditions(ItemCondition fake, ItemCondition real)`; `GetCurrentItemCondition()` returns `fakeCondition` until `IsExposed()`, then `realCondition` | `fakeCondition` = what the customer **claims** (visible); `realCondition` = truth (HIDDEN). Categories `CATEGORY_GENUINE_CIGARETTE` / `_STAMP` / `_INJECTOR` (`ItemFeatureList.Cigarette*`, `Stamp*`, `InjectorFeature(fake, real)`). |
| Cue vs truth members | Cues are int/string **tags** on the `GameItem`, which the views turn into active sprite children or TMP text (`GameItem.GetTagReadonly(string)`). Truth is `realCondition` plus the `*_ISSUE*` tags that the confront check reads | Cue tags are the data the renderer consumes. For sprite cues, **index 0 is the genuine variant by construction** (every `SetGenuine` writes 0) — so a raw sprite index is the truth in disguise (see "Fairness"). |
| Confront panel | `HandleCigarette/HandleStamp/HandleInjector` → `Open*ConfrontPanel(GameItem)` if `CanOpenConfrontPanel` (item not owned, no `ALREADY_CONFRONTED_TAG`, not `NegociationUIManager.IsTradeLocked()`) | Layout depends on the **claim** only (genuine claim → five issue toggles; counterfeit claim → one "it's genuine" toggle; injector claimed non-genuine → no panel). Does not leak truth. |
| Verdict | `InspectionUIManager.OnConfront()`: claim genuine → `On*Confront()` with the issue toggle (if several are on, the **last** checked in code order wins); claim counterfeit → `On*Talk()` | Correct iff the item carries the chosen issue tag (`InsCigaretteHelper.IsConfrontCorrect(string, GameItem)`, `InspectableHelper.IsConfrontCorrect`). |
| Success | `InspectableHelper.OnConfrontSuccess(GameItem)`: sets `ALREADY_CONFRONTED_TAG`, adds `ItemFeatureList.ConfrontSucessDiscount()` (−15 %, Market stage, temporary buying), analytics `success`; the feature is exposed → `ItemFeature.SetExposed(true)` runs the real condition's action (`ItemConditionActionList`) which rewrites name + `unitValue` | |
| Failure | `InspectionUIManager.OnConfrontFailed()`: **−2 rep** with the client's faction (falls back to `FACTION_LOWER_LEVEL`), fail sound, analytics `fail` | No `ALREADY_CONFRONTED_TAG` on failure → the panel reopens; every wrong guess costs another −2. |
| Outcome hook | postfix `EmporiumAnalytics.TrackConfront(string itemId, string confrontType, string outcome)` (already in `DAY.md`) | `confrontType` ∈ `confront`/`talk`. |
| Reference guides | Guide items open static panels: `PaperUIManager.OpenCigaretteGuide/OpenStampGuide/OpenInjectorGuide/OpenAugIdGuide`, `cigaretteColorPanel` (item `cigarette_color`), `MultiPageUIManager.OpenUI(int)` for `hand_guide` (6), `fuel_guide` (0), `ribwich_guide` (4) | Guide content is prefab art/text, **not in the decompile**. The rules below come from code; which of them the guides actually print must be checked in game before giving them to the model. Appraisal guides are handed out on promotion (`AppraisalRankHelper`, `AppraisalItemType.guideItemId`). |

## Cigarettes (`packet_red_cigarette`)

| | |
|---|---|
| Shown | `CigaretteUI.OnInit(GameItem)`: box base sprite + one sprite from each of `seals`, `letterings`, `logos`, `boxes` (colour overlay). **No text.** |
| Cue tags | `CIGARETTE_SEAL` (0 / 1 / −1 = missing), `CIGARETTE_LETTERING` (0–7 / −1), `CIGARETTE_LOGO` (0–4 / −1), `CIGARETTE_BOX` (0, or 1–4 colour variant) |
| Truth | `realCondition` `genuineCigarette`/`counterfeitCigarette`; issue tags `CIGARETTE_{SEAL,LOGO,LETTERING,COLOR,BOX}_ISSUE` |
| Rule | `InsCigaretteHelper.SetGenuine`: every cue 0. `SetConterfeit`: rerolls until ≥1 deviation (seal 10 %, logo 20 %, lettering 30 %, colour 20 % per pass). So: genuine ⇔ every element matches the reference pack and colour card. `CIGARETTE_BOX_ISSUE` is only set by the scripted `SetConterfeitGeorgio2` — the "box" toggle is always wrong on random fakes. |
| Mix | `PreBuiltItemHelper.RandomCigarette()` after the intro: ~20 % fake-claimed-fake, ~40 % fake-claimed-genuine, ~30 % genuine, ~10 % genuine-claimed-fake |
| Verdict → effect | Claim genuine: confront with lettering/colour/seal/logo/box. Right → price drops to counterfeit (`CounterfeitCigaretteAction`: `unitValue` 100 → 50, loses `LUXURY_ITEM`) + −15 %. Wrong → −2 rep, retry allowed. Claim counterfeit: "it's genuine" (`CreateTalkDialogue`): if truly genuine → exposed to genuine (`unitValue` 100), **+2 rep** with that client, −15 % discount; if not → the customer denies, no penalty. Skip → buy at the claimed price; buyers can expose a fake cigarette later (`ClientCanExposeFunc.ClientCanExposeCigarette`, −1 rep per `BARTER.md`). |
| Tier | **2** — every cue is a sprite variant; only "missing seal/logo/lettering" (−1) is coarse enough to transcribe (1b). |

## Food stamps (`food_stamp`)

| | |
|---|---|
| Shown | `StampUI.OnInit(GameItem)`: `amount` TMP text (`ins_stamp_credits` with `STAMP_AMOUNT`), sprites from `seals`, `letterings`, `logos`, `chips`. The item **name** also carries the printed amount (`ins_stamp_name`). |
| Cue tags | `STAMP_AMOUNT` (int, rendered as text), `STAMP_SEAL` (0 / −1), `STAMP_LETTERING` (0–8 / −1), `STAMP_LOGO` (0–5 / −1), `STAMP_CHIP` (0–3 / −1) |
| Truth | `CATEGORY_GENUINE_STAMP` `realCondition`; issue tags `STAMP_{SEAL,LOGO,CHIP,LETTERING,AMOUNT}_ISSUE` |
| Rules | (a) **Denomination**: `CreateRealStamp`/`CreateFakeStamp` draw from {40, 80, 120, 160, 200}; the amount issue adds 10 (`InsStampHelper.SetCounterfeit`) → printed value not a multiple of 40 ⇒ fake. (b) **Seal vs amount**: `SetGenuine` puts a seal iff amount ≥ 160; the seal issue inverts that for the printed amount ⇒ seal present ≠ (amount ≥ 160) ⇒ fake. (c) Logo/lettering/chip: any non-reference variant ⇒ fake. |
| Mix | `CreateRandomStamp()`: 75 % genuine, 25 % fake claimed genuine. Of the fakes, ≈ 20 % carry the amount issue and ≈ 35 % carry amount or seal (from the per-pass rolls 20/20/20/12/10) — the rest need the sprites. |
| Verdict → effect | Confront lettering/logo/seal/value/chip. Right → exposed (`CounterfeitStampAction`: `unitValue` 8, can't be redeemed) + −15 %. Wrong → −2 rep. Price is 0.75 × printed amount (the fake +10 inflates it). Skip → a fake reveals itself only when redeemed (`MiscItemDirectory.FoodStamp` activate: lowkey exposure, nothing credited). |
| Tier | **1** for (a) (text + arithmetic), **1b** for (b) (seal presence is coarse), **2** for logo/lettering/chip variants. |

## Injectors (`large_purple_injector`, Immunivax)

| | |
|---|---|
| Shown | `InjectorUI.OnInit(GameItem)`: **text** `serialTMP`, `companyTMP`, `modelTMP`; sprites `display[INJECTOR_INDICATOR]`, `body` or `broken[INJECTOR_BODY−1]`, `color[INJECTOR_COLOR]` overlay |
| Cue tags | `INJECTOR_SERIAL`, `INJECTOR_COMPANY`, `INJECTOR_MODEL` (strings, rendered verbatim); `INJECTOR_INDICATOR` (0 ok / 1 expired / 3 unusable), `INJECTOR_BODY` (0 / 1–3 broken / −1 hidden for colour fakes), `INJECTOR_COLOR` (−1 / 0–1) |
| Truth | `CATEGORY_GENUINE_INJECTOR` `realCondition` ∈ genuine / expired / counterfeit / unusable; issue tags `ISSUE_{INDICATOR,LABEL,SERIAL,BODY,COLOR}` |
| Rules (`InsInjectorHelper`) | Company must be exactly `Carmicheal Pharmaceutical`, model exactly `Immunivax` (fakes come from fixed misspelling lists, including empty). Serial = 7 digits: a 3-digit prefix 125–455 + 4 digits 1000–9999 (`GenerateCorrectSerial`). Bad serials: 6 or 8 digits, prefix 100–124 or 456–999, or empty (`GenerateInCorrectSerial`, empty ≈ 41 % of the time) ⇒ counterfeit. Prefix in `RECALLED_PREFIX` {250, 251, 301, 355, 385, 386, 421} ⇒ unusable (recalled). Indicator 1 ⇒ expired, 3 ⇒ unusable; broken body ⇒ unusable; off-colour body ⇒ counterfeit. **There is no expiry date** — "expired" is only the indicator sprite. |
| Mix | `CreateRandomInjector()`: all claimed genuine; ≈ 20 % expired, 20 % unusable, 18 % counterfeit, 42 % genuine. Text alone catches ≈ 75 % of counterfeits (label 50 %, serial 25 %) and ≈ 33 % of unusables (recalled serial); 0 % of expireds. |
| Verdict → effect | Confront indicator/label/serial/body/colour (only when claimed genuine). Right → exposed: `unitValue` 160 → 80 expired / 40 counterfeit / 8 unusable (`ItemConditionActionList`) + −15 %. Wrong → −2 rep. Skip → buyers catch it later except a recalled-only unusable (`ClientCanExposeUnusableInjector` + `IsOnlyIssueRecalled`); using one yourself works partly or not at all (`HealthData.ConsumeImmunivax`). |
| Tier | **1** for label + serial (exact strings + numeric rule), **2** for indicator/body/colour (coarse states → 1b candidates once the sprites are reviewed). |

## Appraisal service (hand, ribwich, plasma cell)

Customer intent `APPRAISAL_SERVICE`; items tagged `APPRAISAL_ITEM`; types from
`AppraisalItemRegistry` (hand rank 0, ribwich rank 1, plasma rank 2). Inspecting with the
magnifier also opens `appraisalPanel`: an int field pre-filled with
`APPRAISAL_DEFAULT_VALUE` (the customer's figure, visible), ± buttons
(`ChangeAppraisalServicePrice(int)`), submit (`OnAppraisalServiceEstimated()` →
`AppraisalServiceHelper.OnEstimate(GameItem, int)`). Truth = `AppraisalHelper.GetCorrectBaseValue(GameItem)` (HIDDEN).

**The true value is a deterministic function of the visible cues** — no RNG, no hidden
state. With a faithful cue transcript and the guide's value table this is exact arithmetic,
not estimation; only the iterate-and-bid strategy is tier 3.

Grading (`GetEstimatePrecision`, private): within 95–105 % of true → perfect; below:
≤ 75 % awful, ≤ 85 % mediocre, ≤ 90 % good, else excellent; above: ≥ 125 % awful,
≥ 115 % mediocre, ≥ 110 % good, else excellent. Refused when < 50 % or > 150 %
(`IsAppraisalAcceptable`), or when the rank pays 0 % for that grade
(`AppraisalRankHelper.IsGradeAccepted`; rank 1 refuses awful, rank 2 mediocre and worse,
rank 3 anything worse than excellent). A refusal says "too high"/"too low" (visible),
costs rank progress for bad grades, and adds 49 hidden impatience (third refusal = walk-out).
Accepted: fee = rank base fee + grade % **of the estimate** (so a high-side estimate within
the band pays more), and a slip naming the grade, the estimate **and the true value** —
a visible post-decision label.

| Item | Shown (`*UI.OnInit`) | Value rule (`*Helper.GetCorrectValue`) | Tier |
|---|---|---|---|
| Ribwich | Text: `barcodeTMP` = "month/ year", `facilityTMP` = "#N" (both under GameObjects that are hidden when the barcode/facility stamp is missing — whether the text hides with them is unverified). Sprites: meat (normal/double), top bun (normal/deluxe), wrap (sealed/open), pickles (normal/extra), Davre logo, Ribwich logo | Base 150. Opened wrap → 40. Counterfeit → 40 (`IsCounterfeit`: month outside 1–12, year outside 425–429 ∪ 432–438, facility outside 1–35, Davre logo ≠ reference, Ribwich logo present on a 425–429 pack or wrong on a 432–438 pack, missing barcode or facility stamp, deluxe bun without extra pickles or vice versa). Else +800 double meat, +300 deluxe, +80 first run (425–429), +50 Jan 425 or Dec 438, +50 facility {1,2,5,8,21,34}, −50 facility {7,15,18,26,31}; floor 40. The counterfeit generator's Ribwich-logo branches are dead (they test the period of an already-invalid date), so ≈ 70 % of fake ribwiches are caught by the date/facility text and ≈ 20 % only by the Davre logo sprite | **1** (text + table) + 1b (meat/bun/wrap/pickles/presence) + 2 (Davre logo) |
| Prosthetic hand | Sprites only: material (steel/carbon/gold), interface marking as full label or symbol (none when unspecified), addons (armored, bladed), wear/damage | 100 + material (gold +200, carbon +50) + interface (neural +120, EMG+ +45, unspecified −60) + condition (wear −35, damage −85) + armored +50 + bladed +80 + tactical set +100 (carbon, EMG+/neural, no wear or damage, both addons) + deluxe set +200 (gold, neural, no wear or damage); floor 50 | 2, all categorical → 1b if the sprites are unambiguous |
| Plasma cell | Sprites only: colour, seal brand (LL / Zhonghua / Republic / Ignition Lab), casing wear/heavy wear, crack, uncovered screw, reinforced. `FuelUI.bottomSerial` is cleared and never written | 300 − wear 45/heavy 90 − crack 150 − uncovered 30 + reinforced 60; colour poor −150 / high quality +150; brand bonuses (LL +60 on standard/poor colour; Zhonghua +120 if pristine standard; Republic +120 if uncracked standard; Ignition Lab +750 if pristine high quality); floor 90 | 2 (poor vs standard colour is likely a shade difference) |

## Other inspection mechanics

| Mechanic | What / rule | Tier |
|---|---|---|
| Berries (`BERRY_TAG`, wine ingredient) | `InsBerryUI`: normal, small, under/over-ripe variants, mould, from `BERRY_CONDITION_INT`; feeds `WINE_SCORE_INT` at fermentation (`WineHelper`). No verdict UI | 2, out of scope (crafting) |
| Disguised augs (magnifying screen) | `AugHelper.MakeDisguisedAug(StoreClient, int)` sets `augIdleTell` (Freeze / Seizure / Normal, read by `StoreClientMono` idle animation) and maybe a mismatched or aug sprite. Truth `StoreClient.isAug` (HIDDEN, `STATE.md`) | 2 and temporal (animation); the aug scanner is the tier-1 instrument |
| Water / chem scanners | `WaterScannerUIManager`, `ChemScannerUIManager` print tested vs labelled values as text; Confront/Match | 1 (instrument readout), see `WATER.md` |

## Fairness: what the model may be shown

- **Text cues** (injector serial/company/model, stamp amount, ribwich date/facility) are
  read from the rendered TMP (`InjectorUI.serialTMP.text`, `StampUI.amount.text`,
  `RibwichUI.barcodeTMP/facilityTMP` when their GameObject is active) — exactly what is
  on screen. Prefer the TMP over the tag so "what the player saw" is literal.
- **Sprite cues must not be sent as indices.** Genuine is always index 0 and every fake
  deviates, so `CIGARETTE_LOGO = 3` *is* the verdict. Only coarse categories a player
  can't miss (tier 1b) may go out as words: present/absent, material, wrap open,
  double meat, broken body, indicator state. Which variants count as coarse is a judgment
  call to make once per cue **after looking at the sprites**; until then treat them as tier 2.
- **Rules** go into the prompt only as player knowledge: gate each rule block on owning
  the corresponding guide item (`EmporiumEntry.Instance.GetItemCountById("injector_guide")`
  etc.), and only after confirming that guide prints it.
- `hidden` (logged, never prompted): `realCondition.identifier`, the `*_ISSUE*` tags, raw
  sprite indices, `GetCorrectBaseValue`.

### Tier-2 capture under the `network`-only capability

`ModScanner` puts `UnityEngine.ScreenCapture` in its `FileTypes` list
(`ModScanner.Pass.CheckTypeUse` → capability `files`), so `ScreenCapture.*` is out.
`Texture2D.ReadPixels`, `RenderTexture`, `Camera.Render` and
`ImageConversion.EncodeToPNG` are not on any list (the game itself uses `ReadPixels` in
`PaperSpriteEffects` and `RunSummaryUI`). In-memory path: coroutine →
`WaitForEndOfFrame` → `ReadPixels` of the inspection panel's screen rect
(`RectTransform.GetWorldCorners`) → `EncodeToPNG` → POST to the bridge with
`UnityWebRequest`. Read the back buffer rather than rendering a camera into a
`RenderTexture`, because a screen-space-overlay canvas does not reach a camera target.
Open question: whether the Large view is masked by the lens, so detail needs the lens
moved over each region the way a player would.

## Proposal: `kind: "inspect"`

Fired once per (client, item) when the inspection panel opens on a non-owned inspectable,
or on the appraisal panel.

```jsonc
{
  "kind": "inspect",
  "context": {
    "mode": "authenticity",               // or "appraise"
    "item": {"uid": "…", "type": "injector", "name": "Immunivax"},
    "claim": "genuine",                   // from GetCurrentItemCondition() / panel layout
    "client": {"faction": "lower_levels", "rep": 12},
    "text": {"serial": "3127764", "company": "Carmicheal Pharmaceutical", "model": "Imunivax"},
    "marks": {"body": "intact"},          // tier-1b words only; omitted when not approved
    "image": "cap_…png",                  // tier-2 only, when captured
    "references": {"injector_guide": "Company 'Carmicheal Pharmaceutical'; model 'Immunivax'; …"},
    "penalty_wrong_confront": "-2 reputation with this client's faction, retry allowed",
    "appraisal": null                     // mode=appraise: default_value, last_feedback ("too_high"/"too_low"), refusals, rank
  },
  "options": [
    "confront:label", "confront:serial", "confront:indicator", "confront:body", "confront:color",
    "tell_genuine",                       // only when claim = counterfeit (cigarettes, stamps)
    "accept",                             // no confront; continue to barter
    "closer_look",                        // ask the mod for a capture → re-decide with an image
    "defer"
  ],
  "hidden": {"real": "counterfeitInjector", "issues": ["ISSUE_LABEL"]}
}
```

- The model gives a verdict (genuine / counterfeit / expired / unusable) plus the matching
  issue; the bridge maps it to `confront:<issue>` (cigarette: lettering/color/seal/logo/box;
  stamp: lettering/logo/seal/amount/chip; injector: indicator/label/serial/body/color).
- The mod executes by setting the one toggle and calling `InspectionUIManager.OnConfront()`
  (public). For appraisal it writes `apprisalServiceInputField.text` and calls
  `OnAppraisalServiceEstimated()`; the reply is `{"estimate": int}`, re-asked after each
  refusal with the visible "too high/too low".
- `closer_look` is the tier-2 escalation; a model on text alone should pick it, or
  `accept`/`defer`, rather than guess, since each wrong guess costs reputation.
- Label after resolution: the postfix on `TrackConfront`, the exposed condition, and for
  appraisal the slip's true value — all visible to the player after the fact.
