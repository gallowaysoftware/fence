# Water and liquid chemistry (RE notes)

Build 26525552 / 0.46D. Same conventions as `STATE.md` / `DAY.md` / `PRODUCTION.md`:
**VISIBLE** = rendered by the UI (evidence = the UI code), **HIDDEN** = internal only,
**DERIVABLE** = computable from visible facts. Types are in the global namespace of
`Assembly-CSharp`. Read from code, not play-tested; "(inferred)" marks conclusions drawn from
the absence of code. Proposed hooks are in `compat/hooks.proposed.json` under `record.water`
and `snap.water`. Machine chains (what a hydroponic bay or wine bottle produces) are in
`PRODUCTION.md`; this file covers the water itself and how each consumer reacts to it.

## 0. The short version

- There is **no pH, salinity or TDS number**. Water is a **mixture of 36 liquids**, each held as
  an integer amount in microlitres (µL) on the container. "Purity" is
  `water_µL × 10000 / total_µL` (basis points, 9800 = 98.00 %). Everything else is a contaminant,
  including minerals and the additives you dissolve on purpose.
- Purity maps to **six grades**; price, customer acceptance, wine score and crime weights all
  key on the grade band, not the exact number. Plants and fish key on **individual
  contaminants**, not on purity.
- Each container carries two purities: the **true** one (the liquid parts) and the **presumed
  label** (`ItemFeature.customIntValue1` on the `CATEGORY_WATER_PURITY` feature). Price and
  customer checks use the label until it is exposed. Most treatments change the true purity
  but leave the label stale, so you must re-test to get paid for clean water (§4).
- Readings are **exact** (strip and AquaScan print the true purity to 0.01 %; no measurement
  noise exists in code). Hidden things are hidden because you have not tested, not because a
  test is fuzzy.

```
sources ──► container (36 liquid parts + temp + label)
  faucet (tap upgrades, daily-rolled purity)        │
  bought bottles / scav & company sellers           │ treat: filter (pour), UV, chlorine tablet,
  moisture farm (night)                             │ portable purifier, purifier (night),
  rations (wine bottle only)                        │ evaporator (night), heat, acid/base,
  evaporator distillate                             │ hydroreed "purifying" (night), fish
                                                    │ remediation (night)
                                                    ▼
   ┌───────────────┬──────────────┬──────────────┬───────────────┬────────────────┐
   sell at counter  vending        wine bottle    hydroponic bay   RAS fish tank
   (grade+label;    fountain       (grade term    (per-contaminant (volume + organic
   crime trackers)  (night, label  +1..−3 in      health per       waste %; filters,
                    by slot)       wine score)    progress point;  remediation)
                                                  minerals = food)
```

## 1. The water model

### 1.1 Storage

| Field | Where | Vis. | Evidence |
|---|---|---|---|
| Liquid amounts | one int tag per liquid, `Liquid.currentPart` = `"CURRENT_PART_<ID>"` (µL); read all with `WaterHelper.GetLiquidParts(GameItem) : System.Int32[]` in `Liquid.Liquids` order | HIDDEN (AquaScan shows all, §3) | `WaterHelper`, `Liquid.InitLiquid()` |
| Capacity | tag `LIQUID_CONTAINER_CAPACITY` (µL; ml × 1000, ceiling 2 147 483 ml) | VISIBLE | `InitLiquidContainerItem` |
| Volume | `WaterHelper.GetTotalVolume(GameItem) : System.Int32` (µL) | exact ml VISIBLE only on graduated containers or with the Experienced Chemist perk; otherwise a band (Empty / Almost Empty / Less Than Half / Half / More Than Half / Almost Full / Full) | `CreateWaterContainerTooltip`, `GetEstimatedVolume` |
| True purity | `WaterHelper.GetWaterPurity(GameItem) : System.Int32` (bp, 0 when empty) | HIDDEN until tested | — |
| Label (presumed purity) | `WaterFeatureHelper.GetWaterFeature(GameItem) : ItemFeature` → `customIntValue1`; `fakeCondition` (label grade), `realCondition` (true grade), `IsExposed()`, `IsFeatureMatch()` (true when both grades are the same band) | label grade VISIBLE in tooltip; true grade VISIBLE after a test ("Tested: …" line, `CreateWaterFeatureTooltip`) | `WaterFeatureHelper` |
| Temperature | tag `CONTAINER_TEMP_TAG` (double, °C; new/empty = 24, ambient = 26) | VISIBLE only on the heating plate panel | `ChemicalHelper.InitContainerTemp`, `MachineHeatingPlate` |
| Last scan | tag `LAST_SCAN_RESULT_TAG` (string "Last scan: 98.12 % Basewater") | VISIBLE in tooltip; cleared whenever the liquid changes | `WaterHelper.SetLastScanResult` / `ResetLastScanResult` |
| Strip-tested flag | tag `STRIP_TESTED_TAG` | HIDDEN and transient: every one-second tick of a non-empty container runs the reaction pass, whose boiling step reports a change unconditionally, so `OnLiquidChanged` clears the flag within a second. It only carries over on a pour into an empty container. Nothing a player does depends on it | `ChemicalHelper.HandleTemperatureReaction`, `HandleBoilingReaction` |
| Item type | `WATER` when non-empty, `HOUSEHOLD_GOOD` when empty (customers buy them as different goods) | VISIBLE | `WaterHelper.UpdateContainerGoodType` |
| Colour | weighted mix of liquid colours (`Liquid.color` × `colorMultiplier`: organic ×7, chemical ×6, heavy metal ×4, microbe ×4, wine extract ×150) | VISIBLE sprite tint (a weak dirtiness cue) | `WaterHelper.GetLiquidContainerColor` |

### 1.2 The liquids that matter for water

`Liquid` fields per entry: `particleSize` (filter cut-off), `filterMinimumPurity`,
`filterDurabilityUsage`, `filterVolumeRemoved`, `baseMaxPurityFromMachine` (floor for
remediation and plant purification), `basePurifierRemoveAmount`, `canBeRemovedByPurifier`,
`boilingPoint`/`isEvaporative`, `lethality`, `euphoria`, `metabolismRatePerDay`.

| id | particle | filter (size-22) behaviour | floor bp | evaporates | Notes |
|---|---|---|---|---|---|
| `water` | 10 | — | — | 100 °C | the only "good" part |
| `organic_waste` | 40 | caught; keeps 0.16 %, 1 dur per 1 ml | 10 | no | fish waste; heat ≥ 80 °C turns it into physical contaminant (33 %) |
| `microbe` | 50 | **never** caught by ordinary filters (explicit exception); only the UV filter | 9 | no | killed by chlorine, UV, heat ≥ 70 °C, acid/base, alcohol ≥ 40 % |
| `physical_contaminant` | 100 | caught; keeps 0.16 %, 2 dur per 1.5 ml | 7 | no | |
| `mineral` | 30 | caught; keeps 0.16 %, 1 dur per 2 ml | 7 | no | **plant food** (§2.3); counted as a contaminant for purity |
| `microplastic` | 25 | caught; keeps 0.2 %, 2 dur per 1.5 ml | 18 | no | not in any premade water |
| `heavy_metal` | 12 | **not** caught (12 < 22) | 25 | no | purifier's last group: after minerals, before chemical |
| `chemical_contaminant` | 2 | **not** caught | 16 | no | chlorine removes it down to 0.7 % of volume; the **very last** thing the purifier removes |
| `chlorine` | 5 | not caught | — | no | from purification tablets; consumed within one tick |
| `acid`, `base` | 5 | not caught | — | no | neutralise each other; kill microbes; mineralise organic waste |
| `alcohol` | 10 | not caught | — | 78.37 °C | wine pours 20 %, Nudka 45 % |
| `debris`, `*_mush`, `plant_matter` | 200 | caught by anything incl. kotton fabric | — | no | chemistry by-products |
| others | | | | | narcotic/chem chain liquids (dream cap, caffeine, zyanide, fanta…) — out of scope here |

### 1.3 Grades

| Index | Grade (`WaterFeatureHelper.*_DISPLAY`) | Purity band | Condition mod (`ItemConditionList.CreateXWater`) | Effective value per litre | Wine term |
|---|---|---|---|---|---|
| 0 | Pure Water | ≥ 99.90 % | +100 % | 80 | +1 |
| 1 | High Quality Water | 99.00–99.89 % | +50 % | 60 | +1 |
| 2 | Basewater | 98.00–98.99 % | 0 | 40 | 0 |
| 3 | Ghostwater | 96.00–97.99 % | −33 % | 26.8 | −1 |
| 4 | Rustwater | 90.00–95.99 % | −75 % | 10 | −2 |
| 5 | Gutterflow | < 90 % (and an empty container: purity 0) | −90 % | 4 | −3 |

Value: `unitValue = round(40 × litres)` (`WaterFeatureHelper.GetWaterPrice(System.Int32)`;
the `WATER_PRICES` array 200/100/40/27/20/10 is **unused**), times the condition modifier of the
**label** while unexposed, of the true grade once exposed (`ItemFeature.GetFakeValueModifier` /
`GetActualValueModifier`); the container's `LIQUID_CONTAINER_BASE_PRICE_TAG` is added **after**
the modifier, unmodified (`GameItem.ComposeStagedValue`); then market events and retail markup.
A full 750 ml `bottled_water` is worth **65 / 50 / 35 / 25 / 12 / 8** by grade (pure 30 × 2 + 5,
not (30 + 5) × 2 = 70; gutter 3 + 5 = 8, not 3.5) (measured, docs/sim/VERIFIED.md). "Hazardous" label = label < 98 % (`IsLabelledHazardousWater`).

### 1.4 Sources

| Source | Purity | Composition of the impurity | Vis. | Evidence |
|---|---|---|---|---|
| Faucet with `RUSTWATER_TAP` (network upgrade 350, needs `GUTTERFLOW_TAP`) | today's roll 90.00–91.50 % (Rustwater), fills the container to the brim, free | `waterContaminantDistribution` (organic 20, microbe 15, physical 10, chemical 30, mineral 15, heavy 10 %) | daily purity HIDDEN (`StoreStation.todayRustwaterPurity`), DERIVABLE by testing one fill | `WaterHelper.InitLiquidContainerItem` faucet branch; `StoreStation.StartDay` |
| Faucet with `GUTTERFLOW_TAP` only (250, needs `WATER_TRADER`) | 80.00–89.99 % (Gutterflow) | same distribution | same | same |
| Starting perk "working pipes" | unlocks both taps | | | `StartingPerkContent` |
| Premade fills `WaterHelper.FillWith*` / `Add*` (`AddWater(GameItem, int grade, …)`; contaminant mixes below) | grade 0 uniform 99.90–99.99; grades 1–3 **rolled near the lower edge** (exponential, mean ≈ 15 % of the band above the floor: base ≈ 98.15 %; measured, docs/sim/VERIFIED.md); grade 4 uniform 90.00–95.99; grade 5 uniform 35–89.99 % | grade 2 `baseWater…`, grade 4 `rustWater…`, grade 5 `gutterflow…`, else `waterContaminantDistribution` | | `AddWater`, `RollNearLowerEdge`, `WaterPremadeHelper` |
| Water sellers (`StoreClientListWater.GetRandomWaterSupplier`: honest 20 %, company 12 %, dishonest scav 7 %, scav otherwise) | via `WaterPremadeHelper.GetRandomWater` odds 30/20/10/10/10 → accurate base, accurate ghost, accurate rust (low end), **ghost labelled base**, **base labelled HQ**; dishonest scav adds ghost-as-base (75 %, 50 %) and **ghost-as-pure** (25 %) | | label VISIBLE, truth HIDDEN | `StoreClientListWater` |
| Moisture farm (night) | 850 ml × (1 + perf %) / (1 + 0.25 × (other running farms)); grade from `quality bonus + 50`: <0 gutter, <50 rust, <100 **ghost (default)**, <150 base, <200 HQ, else pure | grade's premade distribution | tooltip shows ml and grade VISIBLE | `MachineMoistureFarm.GetOutputVolume/GetOutputQuality/Fill` |
| Evaporator (night) | removes up to 300 ml × (1 + perf %) of evaporative liquids (water, alcohol, lipobond) from the input; distillate gets `quality bonus %` (clamped 0–100) of it, so **distillate is 100 % water at any quality above 0** — quality sets only the volume (with the solvent-recovery module only alcohol is kept). At quality 0 the water is simply lost. The distillate container keeps its old label (an empty one reads Gutterflow) until tested; the added amount is capped per liquid, not against the container's total, so a container already holding other liquid can be overfilled | | | `MachineEvaporator`, `WaterHelper.AddLiquid` |
| Water rations (`water_ration` 750 ml / `_small` 375 ml) | poured only into a wine bottle: base grade (≈ 98.15 % on average, near the lower edge) | base distribution | | `WineHelper.PourRationIntoBottle` |
| Water Merchant start | ghostwater 2 L, rust (low end) 6 L, strips, tablets, pitcher + filter, shabby portable purifier at 75 charge, shabby bottle printer | | | `NewGameData` (`StartType.WATER_MERCHANT`) |

Contaminant mixes (share of the impurity; `WaterPremadeHelper`). Grades 0, 1 and 3 and both taps use
the default mix; a separate ghostwater mix exists in code but no fill uses it. The moisture farm makes
its grade's premade fill, so its mix follows the grade.

| Mix | organic | microbe | physical | chemical | mineral | heavy metal |
|---|---|---|---|---|---|---|
| default (pure, HQ, ghost, both taps) | 20 % | 15 % | 10 % | 30 % | 15 % | 10 % |
| base (grade 2, rations) | 20 % | 10 % | 10 % | 30 % | 15 % | 15 % |
| rust (grade 4) | 22 % | 15 % | 10 % | 30 % | 15 % | 8 % |
| gutter (grade 5) | 29.9 % | 15 % | 10 % | 30 % | 15 % | 0.1 % |

A premade fill (`AddWater` with its feature init: faucet, bought bottles, moisture farm) re-labels the
**whole** container to the true purity of the result, unexposed, even when it tops up a container that
already held water. Exception: when the container carries a filter (the pitcher), the fill is filtered
on the way in and the label is set to the purity the fill *would* have had unfiltered, i.e. under the
truth. Rations poured into a wine bottle skip the feature entirely.

Containers (`InitLiquidContainerItem(GameItem, capacityMl, basePrice, isFilterContainer, isGraduated, isDropper, isTransparent, sheet)`):
`mini_bottle` 250 ml (3), `small_bottled_water` 450 (4), `bottled_water` 750 (5),
`large_bottled_water` 2000 (9), `water_jug` 6000 (15), `water_pitcher` 3000 (25, the only
filter container, graduated), `water_tank` 20 000 ml growing to 50 000 with scrap metal
(+2000) or ingots (+4000…14 000 by purity level) (350, graduated), `ras_water_tank` 12 000 (0,
RAS-only), cylinders 25/50/100 ml and droppers 1/5 ml (graduated), med bottles 150–250 ml.
The bottle printer makes empty 450/750/2000 ml bottles for 4/6/11 power.

### 1.5 How water changes

All "remove" operations below delete the contaminant and, where noted, give its volume back as
water.

| Operation | Trigger / signature | Effect | Label after |
|---|---|---|---|
| Pour | `WaterHelper.TransferLiquid(GameItem source, GameItem target)` | proportional split of every part (`SplitProportionally`), limited by free capacity; temperatures mix by volume; target's filter (pitcher) applied per liquid | **into an empty target: copies the source label** (true purity if the source was exposed); **into a non-empty target: keeps the target's old label** (mixing silently creates mislabelled water) |
| Filter on pour | `WaterHelper.HandleFilter(string liquidId, int amount, GameItem source, GameItem filter)` | acts only when `FILTER_SIZE_TAG ≤ particleSize`; strips that liquid down to `filterMinimumPurity` of the poured amount, spending `filterDurabilityUsage` per `filterVolumeRemoved`; microbes pass; durability 0 = no filtering. UV filter in a filter slot strips **only** microbes | stale |
| Drain | `WaterHelper.EmptyContainer(GameItem)` (drag onto `drain`) | all parts 0; label reset, unexposed | reset |
| UV filter used on a container | `WaterHelper.OnUVUsed(GameItem)` (item `uv_filter`, any non-empty container, no ownership check, no durability) | all microbes become water | stale |
| Purification tablet | dissolves: `ChemicalHelper.DissolveInto(GameItem chem, GameItem container)` adds 15 ml chlorine (displacing liquid if full); next tick `HandleChlorine`: potency 5.1 × chlorine (≤ 15 ml per tick = 76.5 ml) first removes chemical contaminant **down to 0.7 % of total volume**, then microbes; chlorine is consumed (≥ 20 %/tick decay). Removed volume is **not** returned as water (the container shrinks) | stale |
| Nutrient tablet | same dissolve path, 15 ml mineral | stale |
| Portable purifier | `WaterHelper.PurifyToBaseWater(GameItem)` via the item's target action (owned purifier, owned non-empty container); refuses at ≥ 98 % with a message, which is itself a free player-visible "this is already ≥ 98 %" test; costs ⌈volume / 100 ml⌉ charges (300 max) | raises to exactly ≥ 98.00 % by shrinking everything except chemical and heavy metal first (minerals included), then those; removed volume returned as water. Lands on the bottom edge of Basewater, so one more contaminant (a nutrient tablet) drops it to Ghost | **updated** (`SetPresumedPurity`) |
| Water purifier machine (night) | `MachinePurifier.PurifyContainer(GameItem machine, GameItem container, System.Boolean ignoreBonus)` | budget µL = (min(volume, 10 L) × 0.003 + 25 ml) × (dirt bonus + perf %/100) × 1.25; dirt bonus by purity: 98 %→1, 94 %→2, 88.5 %→5, 80 %→10, 62.5 %→15, 37.5 %→20 (linear between). Order: every removable liquid except water, largest particle first (debris/mush 200, physical 100, microbe 50, organic 40, microplastic 25, then the small chemistry liquids), and last of all mineral → heavy metal → chemical (smallest particle last); within one particle size proportionally. Stops when the contaminants left equal `volume × (10000 − cap) / 10000`, cap = `10000 − round(99 × mult)` with mult 1.0 / 0.85 (q≥15) / 0.65 (25) / 0.35 (50) / 0.2 (75) / 0.1 (100) / 0 (200): **99.01 % (HQ) unupgraded, 99.90 % (Pure) needs quality ≥ 100** (raw quality: the purifier's quality ratio tag only changes its tooltip). Performance adds P/100 to the dirt bonus (fineness −8 each). Removed volume returned as water. **What is left matters for plants:** HQ made by the purifier is mostly chemical and heavy metal (base 98.15 % → 99.01 % leaves ≈ 55 bp chemical, 28 heavy metal, 16 mineral, and tablet-treated tap water keeps ≥ 70 bp chemical), worse for a bay than premade HQ; at the Pure cap only ≈ 10 bp of chemical remain, under every plant threshold | stale |
| Heat | heating plate `MachineHeatingPlate.HandleHeating` (max 99 °C, 160 °C upgraded; speed ×4/×16 with 3/4 upgrades); cools toward 26 °C each second | ≥ 70 °C kills 20 % of microbes/s, ≥ 100 °C all; ≥ 80 °C organic waste −15 %/s (100 °C: all) with 33 % becoming physical contaminant; **> 100 °C boils off 2 % of the water per second, lowering purity** | stale |
| Acid / base | `HandleAcidBaseNeutralization`, `HandleAcidBaseMicrobeKill` (1 acid/base kills 20× microbes), `HandleOrganicWasteMineralization` (1 acid/base turns 40× organic waste into 4× mineral) | | stale |
| Alcohol | `HandleMicrobeAlcohol`: alcohol ≥ 40 % of volume kills microbes 1:1 | | stale |
| Plant trait `purifying` (rank r, hydroponic water slot, nightly) | `PlantWaterQuality.Purify(GameItem, int)`: removes min(95 %, 10 % × r^0.75) of every purifier-removable contaminant **except minerals** (≥ 1 ml each), floor `baseMaxPurityFromMachine / r`; returned as water | stale |
| Plant trait `polluting` | `PlantWaterQuality.Pollute(GameItem, int)`: turns 0.1 % × r^0.75 of the water into organic 50 / microbe 35 / physical 15 % | stale |
| Fish | `AquacultureHelper.TickTank` (§2.4): waste turns water into organic waste; gutter tilapia remediate heavy metal, chemical, physical | stale |
| Spirits | `WaterHelper.PourSpirit(GameItem, int ml, int alcohol%)` (Nudka 750 ml at 45 %) | alcohol + base water | — |
| Time | **no decay of its own**: the only timed callback on a liquid container is the one-second reaction tick (and its 360 night repeats); with no chlorine, acid/base, alcohol ≥ 40 % or heat above 70 °C none of its reactions changes anything | | `ChemicalHelper.containerTempTick`, `RunNightlyChemicalSimulation` |
| Legacy, uncalled | `WaterHelper.OnWaterPurificationTabletAdded(…)`/`…2`, `OnHydroponicNutrientTabletAdded`, `AddWaterOld`, `WaterFeatureHelper.HandleWaterCheckFlashlight` | — | — |

## 2. Consumers and the effect of each parameter

### 2.1 Selling water at the counter

| Rule | Detail | Evidence |
|---|---|---|
| Who buys | Water-type customers accept by **label grade** (`WaterFeatureHelper.GetCurrentWaterDisplay`: true grade if exposed, else label). Tier buyers: `waterScavenger` base/ghost (budget 60–100), `thirstySpacer` base/HQ/ghost (90–110), `scavWaterShopper` base/HQ/pure (120–150), `waterSpace2` base/HQ (145–195 + day/4), `waterStockpiler` pure/HQ (500–600 + day); event and minor buyers mostly pure/HQ/base (+ rations), one minor wants HQ only, a panic buyer also takes ghost; story clients want base | `StoreClientListTier/Event/Minor/Story` |
| Showcase pull | showcased water picks the next water client: ≥ 350 value or mostly-pure label (day ≥ 14) → stockpiler; ≥ 145 or pure/HQ label → `waterSpace2` | `ShowcaseHelper.CreateClientByStockType` |
| Refusals | a filter left inside the pitcher ("take the filter out"); luxury items to non-UL/non-tourist water buyers (the advanced filter and portable purifier are `LUXURY_ITEM`); hazardous label (< 98 %) for clients with `refusesHazardousWaterLabel` | `StoreClient.IsRefusingForFilterInside`, `IsRefusingLuxuryItem`, `IsClientRefusingItem` |
| Customer checks during selling | Clients with the basic check (`AddBasicCheck`: all tier clients) expose an **over-labelled** container (true grade worth less than the label) on negotiation: the "strip" check fires for any over-label; the "flashlight" check also requires true purity < 96 % | `NegociationHelper.CheckWaterFeatureStrip/Flashlight`, `ClientNegociate` |
| On exposure | price drops to the true grade; −1 rep with that faction (not for Aug) | `NegociationHelper.OnFeatureExposed` |
| Under-label | Placing an **under-labelled** item: by faction, LL/BM/SEC/REV clients can expose any mismatch, scav/tourist/UL/basic only rust/gutter truths; when the truth is better than the label the dialogue gives **+3 rep** with the client's faction and reprices up | `StoreClient.CanClientExposeAnyFeature`, `ClientCanExposeFunc`, `WaterFeatureHelper.UpdateWaterFeatureDialog` |
| Crime on sale | `PlayerStore.PerformOnSoldCheck(GameItem, StoreClient)` → `SecData.CheckSoldDirtyWater(GameItem, StoreClient)` for every sold container that is unexposed and not matching (tourists exempt). Weighted ml (`SecData.GetMislabeledWaterWeight`): ghost labelled base ×1 / HQ ×2 / pure ×4 → `dirtyWaterTracker` (ENDANGERMENT); base labelled HQ ×1 / pure ×2, HQ labelled pure ×1.5, gutter labelled rust ×0.25 → `mislabeledWaterTracker` (FRAUD). **Rust or gutter labelled base or better weigh 0** (no tracker; only the customer check above) | `SecData` |
| Crime settlement | each shutter open (`PlayerStore.OnShutterOpened` → `CheckCrime`): trackers −5 ml; at ≥ 2500 ml commit `floor(ml / 500) × 20` crime and keep the remainder; dirty water also puts a ghostwater threat note on the table. Perks: Intimidating ×0.67, Blabbermouth ×1.33 | `PlayerStore.CheckCrime`, `SecData.CommitCrime` |

### 2.2 Vending fountain (night)

`VendingFountainHelper.HandleNight` runs in `onCycleEndLateSlotItemFunc`, i.e. **after** the
purifier and moisture farm have run that night. Three source slots labelled Base, HQ and Pure;
a slot may hold a container or a whole `water_purifier` / `moisture_farm` (its container slot is
read). Bottle count/size per slot scales with `VENDING_ATTRACTIVENESS` (pure only at ≥ 20).
Each sale (`SimulateWaterSale(int labelPurity, string containerId, GameSlotInventory, GameItem)`)
skips rust/gutter sources, labels the bottle **as the slot's grade**, prices it from that label
(+10 % vending bonus, retail markup), and runs `PerformOnSoldCheck` (crime trackers above) plus
`VendingMachineHelper.TrackMislabelledFountainSale`: at night settlement every mislabel costs ≥ 1
rep split LL/UL (pure-slot losses go to UL, HQ half/half, base to LL) and a night-log line.
So **ghostwater in the Base slot is endangerment**; base in the HQ slot is fraud.

### 2.3 Hydroponics (berries, kotton, coffee…)

Per night (`onCycleEndSlotItemFunc`), if powered and the water container holds ≥ the seed's
`GetWaterUsePerCycle` ml (else paused, no penalty), the bay advances `CURRENT_PROCESSING_SPEED_TAG`
progress points (base 8). **Per point** (`HydroponicHelper.ContinueGrowing`):

- **Minerals** (`HandleNutriment`): `mineral × 10000 / water` (bp of water) vs
  `nutrimentLevel ± nutrimentRange`: below → health −1 %; above → −0.5 %; then consumes
  `mineralUseRate` µL of mineral.
- **Contaminants** (`HandleContaminant` → `ApplyContaminantEffect`): for each of heavy metal,
  chemical, microbe, organic waste, acid, base: x = bp of water × genetics sensitivity
  (`hardy` −30 %, `frail` +30 % per rank-scale); penalty 0 (≤ 10), 0.15 % (≤ 15), 0.2 % (≤ 20),
  0.3 % (≤ 25), 0.4 % (≤ 30), 0.6 % (≤ 40), 0.8 % (≤ 60), 1.6 % (≤ 100), 3.2 % (≤ 200), 6.4 %
  (> 200), × the plant's `contaminantEffect`. Physical contaminant, microplastic and mineral are
  not penalised here.
- Then once per night: water use (`RemoveMl` proportional, so contaminant **ratios** do not
  improve as the plant drinks), grow-tick traits (`purifying`/`polluting`).

Plant water parameters (`PlantList`): bloomberry nutrients 200 ± 75 bp, contaminant ×1.25,
100 ml/night, 23 points then 12 to refruit (18 / 10 base, slowed 20 % by the innate `perennial` trait; measured, docs/sim/VERIFIED.md); nutrifruit 150 ± 100, ×1.0, 100 ml; kotton
350 ± 150, ×1.26, 176 ml; dream cap / coffee 250 ± 75, ×1.01; fillerweed 100 ± 100, ×0, 287 ml;
hydroreed 100 ± 100, ×0, 20 ml, innate `purifying` rank 4 (28 %/night).

Harvest quality comes from final health (`PlantHarvest.SplitByHealth`; bloomberry condition roll
`PlantList.RollBloomberryCondition`: ≤ 25 → 45 % −2 / 55 % −1; ≤ 55 → 25 % −2, 45 % −1,
30 % +1; ≤ 85 → 25 % −1, 45 % +1, 30 % +2; > 85 → 40 % +1, 60 % +2). Health is VISIBLE only
through the **plant analyser** (`PlantAnalyserUIManager`, "growth score" = `PLANT_HEALTH_INT`/100);
the bay tooltip does not print it.

Worked bloomberry cycle (contaminant ×1.25). The first cycle is ≈ 24 penalty points (3 nights at
speed 8), every refruit only **16** (2 nights); health resets to 100 in the harvest night, *after*
the berries roll (`FinishGrowing` → `StartNextFruitingCycle`), so the morning reading after a
harvest is always 100 % and never the health the berries got. Health is not clamped at 0. Measured
end to end on 6 bays × 216 berries per setup (measured, docs/sim/VERIFIED.md):

| Water in the slot | Contaminant loss / point | Mineral loss / point | Health at harvest (first / refruit) | Berries (measured) |
|---|---|---|---|---|
| Basewater ≈ 98.0 % (chem ≈ 61, organic ≈ 41, heavy ≈ 31, microbe ≈ 20, mineral ≈ 31 bp) | (1.6 + 0.8 + 0.6 + 0.2) × 1.25 = 4 % | 1 % (31 < 125) | ≈ 16 / ≈ 44 | **≈ 75 % bad** (moldy or under/over-ripe), not "worst roll" |
| HQ ≈ 99.0 % (chem 30, organic 20, microbe 15, heavy 10) | (0.4 + 0.2 + 0.15) × 1.25 ≈ 0.94 % | 1 % | ≈ 56 / ≈ 70 | **≈ 34 % bad**, mostly normal/best |
| HQ + ~6 nutrient tablets per 6 L (mineral ≈ 150 bp) | ≈ 0.94 % | 0 | ≈ 78 / ≈ 85 (computed) | ≈ 22 % bad |
| Pure alone (no minerals) | 0 | 1 % | ≈ 76 / ≈ 84 (computed) | ≈ 25–29 % under/over-ripe |
| Pure + ~6 nutrient tablets per 6 L (mineral ≈ 150 bp; or a 250 ml nutrient-solution mini bottle) | 0 | 0 | 100 | **0 % bad, 62 % best** |
| Ghostwater / Rustwater (the farmer start ships its bays with Ghostwater) | ≈ 7.75 % (ghost, computed) | 1 % | far below 0 | 100 % bad (≈ 55 % moldy) |

Minerals are the only contaminant plants want. The purifier strips them (last) and so do
filters (particle 30 ≥ 22); the `purifying` trait spares them. Add nutrient tablets *after*
filtering or purifying.

### 2.4 Fish (RAS tank)

`MachineRAS` water slot takes only `ras_water_tank` (12 L, cannot be removed); 4 filter slots
take any `WATER_FILTER`. Nightly `AquacultureHelper.TickTank(GameItem, GameGridInventory, GameItem, List<GameSlotInventory>, GameSlotInventory, System.Boolean)`:

1. Unpowered: every fish −34 % max health, and filters do not run.
2. Waste: Σ weight/100 g × `wasteMlPer100gPerNight` (tilapia & gutter tilapia 0.5, graft
   tilapia 1, sturgeon 1, eels 2) ml of water turned into organic waste.
3. Remediation: living fish with `remediationMlPer100gPerNight` (gutter tilapia 0.5) remove
   that many ml each of heavy metal, chemical, physical (down to their floors), returned as water.
4. Filters (powered): each runs `HandleFilter` on the whole tank: organic waste falls to 0.16 %
   of itself per filter, 1 durability per ml; microbes untouched (UV filter: microbes only).
5. Water checks (`ApplyWaterChecks`): ratio = tank ml / Σ stage water need (tilapia adult 750,
   elder 1000; eel adult 1200; sturgeon adult 3000 ml); waste % = organic / total × 100.
   Ratio < 0.5 or waste ≥ death % → health 0. Else damage = max × (1 − ratio) plus, above
   damage %, max × 0.5 × (waste − dmg)/(death − dmg). Damage % = 3 / sensitivity, death % =
   8 / sensitivity (graft tilapia sensitivity 2 → 1.5 / 4). Gutter tilapia ignore waste. No
   damage and powered and fed (hunger ≥ 50 %) → +10 % health.

**Only volume and organic waste matter to fish** — the starting grade of the tank water is
irrelevant except for its own organic share. Tooltip shows ml / need and waste % with colour
bands at 3 % and 8 % (VISIBLE, `MachineRAS.CreateTooltip`). Example: 1 kg of tilapia adds 5 ml
waste/night = 0.04 %/night of a full 12 L tank; damage starts at 360 ml (3 %), so an unfiltered
tank lasts ~72 fish-kg-nights; one 400-durability filter absorbs 400 ml of waste.

### 2.5 Wine

`WineHelper.StartFermentation(GameItem)` adds `GetWaterQualityScore(GameItem)` — the **true**
grade of the bottle's contents (not the label; the bottle's own water feature is a fixed fake
"base"): pure/HQ +1, base 0, ghost −1, rust −2, gutter −3. Tiers by score: ≥ 6 Master Vintage
(+75 %), 5 Reserve (+50 %), 3–4 Production (+25 %), 2 Homebrew (0), 1 Bucket Brew (−15 %),
≤ 0 Undrinkable (−100 %) on a 100 base, then aging +3 %/day (`PRODUCTION.md`). Needs ≥ 749 ml.
Rations give base (0). Anything else in the bottle (e.g. alcohol) lowers purity. The score is
HIDDEN (dev tooltip) but DERIVABLE: test the water (AquaScan works on an unfermented bottle; it
is a liquid container) before starting.

The term is read from the purity of everything liquid in the bottle at the moment of starting, so
pours mix by volume. A ration rolls Basewater near its floor (≈ 98.15 % on average, often 98.0x), so
half a small ration topped up with Pure water reaches HQ only about half the time; a full bottle of
tested HQ or Pure water is the reliable +1. Two ways to get there without moving water: the water
purifier takes an unfermented wine bottle in its container slot (it is a liquid container), and one
night lifts a ration-filled bottle (750 ml, budget ≈ 34 ml bare, ≈ 23 ml with four fineness) to the cap,
HQ bare or Pure with quality ≥ 100; the berries and yeast already in the grid are untouched. After
fermentation starts the water is drained and the term is fixed.

### 2.6 Other users

| Consumer | What it reads | Evidence |
|---|---|---|
| Chemistry (dream cap, coffee, Nomore, synthesis chain) | water as solvent; heat-driven extractions (`HandleCoffeeBeanExtractionWater` ≥ 80 °C); contaminant purity is scored by the chem scanner, not water grade | `ChemicalHelper`, `ChemScannerUIManager` |
| Livestock ingestion | `Liquid.IngestableLiquids` (steroid, immunity: −5 µL/day metabolism) | `IngesterHelper` |
| Fridge | chills liquid containers (cold feature) | `MachineFridge` |
| Ship water recycler / station fountain | space-mode systems, separate from shop water | `ShipHelper`, `StationMachinerieHelper` |

## 3. Measuring

| Tool | Use | Reveals | Precision | Consumed | Evidence |
|---|---|---|---|---|---|
| `water_test_strip` (box of 10) | drag onto any non-empty container, even a customer's (`CAN_USE_ON_NON_OWNED_TAG`) | true purity % (2 dp) + grade, and the label, in a panel; **owned item: exposes and relabels to truth**; customer item: discovers the feature and offers **Confront** (when the label is better than truth) | exact | yes (becomes a spent strip worth 0; can reopen the reading) | `AmenitiesItemDirectory.WaterTestStrip`, `WaterTestHelper.HandleWaterCheckTestingStrip`, `WaterStripUIManager` |
| `aquascan` | owned scanner onto any container | true purity + grade + **every liquid with µL/ml/L and %**, refreshed every second while open; caches a "Last scan" tooltip line; **Match** (owned, mismatched) relabels to truth for free; **Confront** on customer items | exact | no | `AmenitiesItemDirectory.AquaScan`, `WaterScannerUIManager` |
| Graduated containers / Experienced Chemist perk | tooltip | exact ml | exact | — | `CreateWaterContainerTooltip` |
| Heating plate | panel | container temperature | integer °C | — | `MachineHeatingPlate` |
| Purifier tooltip | | last night's removed ml, max purity cap | | — | `MachinePurifier.CreateTooltip` |
| Moisture farm tooltip | | tonight's ml and grade | | — | `MachineMoistureFarm.CreateTooltip` |
| RAS tooltip | | tank ml / need, waste % | 1 dp | — | `MachineRAS.CreateTooltip` |
| Plant analyser | onto a growing seed | plant health ("growth score") | 0.01 % | no | `PlantAnalyserUIManager` |
| Labeler | relabel any grade (dropdown) — sets the label, not the water | — | — | no | `LabelerElement.OnDropdownValueChanged`, `WaterFeatureHelper.OnWaterLabelUpdated(ItemFeature, int)` |

## 4. Visible vs hidden, and the stale-label trap

| Quantity | Status |
|---|---|
| Label grade (public display "Basewater (98 %–99 % Purity)") | VISIBLE |
| True purity / grade | HIDDEN until a strip/AquaScan; then VISIBLE as "Tested" |
| Per-liquid composition | HIDDEN except AquaScan |
| Volume | band VISIBLE; exact on graduated containers |
| Today's tap purity | HIDDEN (DERIVABLE by testing) |
| Dirty/mislabel trackers, fountain rep debt | HIDDEN; outcomes VISIBLE (crime ledger, threat note, night log) |
| Wine water term, plant health per point, fish thresholds | HIDDEN mechanics; inputs DERIVABLE from tests and tooltips |

**Stale label:** `UpdateWaterFeature(GameItem, false)` — called by tablets, UV, purifier,
evaporator, plants, fish, heat — recomputes the true grade but keeps the old label and drops
exposure. An emptied container's label is purity 0 = **Gutterflow**, so anything first filled by
`AddLiquid` (evaporator distillate) or cleaned by the purifier sells at the old/low label until
re-tested. Exceptions that relabel automatically: faucet/premade fills (`AddWater` → `InitWaterFeature`,
label = truth, also on a top-up, see §1.4), the portable purifier (`SetPresumedPurity`), a strip/AquaScan
Match. Mixing into a non-empty container keeps the receiver's label (§1.5) — the easy way to commit fraud by
accident.

**Match** is offered only on an owned, non-empty, unexposed container whose label *band* differs from the
true band; a label of the right band keeps its old number and needs nothing. A Match exposes the feature
and writes the true purity into the label, so the price follows the true grade; the next stale-label
update (any treatment, a purifier night) drops the exposure and the price falls back to that written
number until the container is tested again.

**What a player can know without a test** (the boundary for an executor that plays fair):

| Signal | Tells you | Source |
|---|---|---|
| Label grade and its band text | the presumed grade (true only after a fill, a Match or the portable purifier) | item tooltip |
| Exact ml | graduated containers, wine bottles (an unfermented bottle always prints ml / 750), or the Experienced Chemist perk; otherwise a band | container tooltips |
| "Last scan" line | the purity at the last AquaScan, until the liquid changes | tooltip |
| Purifier tooltip | last night's removed ml (0 = nothing left to do at this cap) and the current max-purity cap | `MachinePurifier.CreateTooltip` |
| Moisture farm tooltip | tonight's ml and grade | `MachineMoistureFarm.CreateTooltip` |
| Portable purifier refusal | the container is already ≥ 98.00 % | `OnPortableWaterPurifierUsed` |
| Colour tint | a weak dirtiness cue | sprite |
| Rules | every formula in this file (fixed treatment results, the purifier's cap, distillate = 100 % water, faucet ranges 90.00–91.50 / 80.00–89.99) | — |

Strip and AquaScan readings are exact; the composition (per-liquid ml, which plants care about) is
visible only on the AquaScan.

## 5. Timing

| When | What | Evidence |
|---|---|---|
| Real time, 1 s ticks while the shop runs (`EmporiumEntry` `onOneSecondItemFunc`) | container cooling (26 °C), all temperature/chemical reactions (chlorine, acid/base, heat kills, boiling), heating plate | `ChemicalHelper.containerTempTick`, `TIME.md` |
| Night, `PlayerStore.EndNight` | 360 extra one-second ticks of every non-empty container (`ChemicalHelper.RunNightlyChemicalSimulation`) → `onCycleEndSlotItemFunc` (purifier, moisture farm, evaporator, hydroponics, RAS `TickTank`, advanced filter +50 durability) → `onCycleEndLateSlotItemFunc` (vending fountain sales) | `PlayerStore.EndNight` |
| Morning, `StoreStation.StartDay` | tap purities re-rolled | `StoreStation` |
| Shutter open | `CheckCrime`: water trackers −5 ml then settle | `PlayerStore.OnShutterOpened` |
| Night settlement (vending) | fountain mislabel rep loss + night-log line | `VendingMachineHelper.SettleVendingMislabels` |

Nothing about water decays with time by itself; overnight changes are only machine work and
chemistry already in the container.

## 6. Recording and the strategist

### 6.1 Snapshot `water[]` (per liquid container, owned; `snap.water`)

Iterate `PlayerStore.FindAllItem(System.Boolean)` for items tagged `LIQUID_CONTAINER_TAG`
(skip `WINE_CONTAINER_TAG` once fermenting — the tag is removed).

| Field | Read | Vis. |
|---|---|---|
| `uid`, `id`, `parent` (machine id + slot role: purifier / farm / evaporator in-out / hydroponic / RAS / fountain base-hq-pure / heating plate / none) | `GetUniqueID()`, `identifier`, `GeneralHelper.GetParentItem` | VISIBLE |
| `capacity_ul`, `volume_ul` | `WaterHelper.GetTotalCapacity`, `GetTotalVolume` | band/exact (see §1.1) |
| `purity_bp` | `WaterHelper.GetWaterPurity` | HIDDEN |
| `parts{id:µL}` (non-zero only) | `WaterHelper.GetLiquidParts` + `Liquid.Liquids[i].identifier` | HIDDEN |
| `label_bp`, `label_grade`, `true_grade`, `exposed`, `match`, `discovered` | `GetWaterFeature`: `customIntValue1`, `fakeCondition.identifier`, `realCondition.identifier`, `IsExposed()`, `IsFeatureMatch()`, `IsFeatureDiscovered()` | label VISIBLE, rest HIDDEN unless tested |
| `temp_c` | tag `CONTAINER_TEMP_TAG` | HIDDEN except heating plate |
| `last_scan` | tag `LAST_SCAN_RESULT_TAG` | VISIBLE |
| `filter` {id, durability, size} | `WaterHelper.GetFilterItem`, `DurabilityHelper.GetRealDurability`, tag `FILTER_SIZE_TAG` | VISIBLE |
| `value` | `GetCurrentValue(...)` as STATE.md | VISIBLE |

Machine extras: purifier `max_purity_bp` (`MachinePurifier.GetMaxPurityCap(MachineryHelper.GetCurrentQualityBonus(m))`),
`removed_last_night_ul` (tag `TOTAL_CONTAMINANT_REMOVED_TAG`);
moisture farm `out_ml`, `out_grade`; RAS `water_ml`, `need_ml`, `waste_pct`
(`AquacultureHelper.GetWaterMl/GetTotalWaterNeedMl/GetWastePercent`); hydroponic water slot
`mineral_bp` (`WaterHelper.GetContaminantPercentage(GameItem, "mineral")`) next to the seed's
`nutriment_level/range` and `GetContaminantEffect`; fountain `attractiveness` (tag
`VENDING_ATTRACTIVENESS`).

Global: `taps` {gutter, rust} (`NetworkUpgrade.IsUnlocked(System.String)`), HIDDEN
`today_tap_bp` {`StoreStation.todayGutterflowPurity`, `todayRustwaterPurity`}, HIDDEN trackers
`PlayerStore.secData.dirtyWaterTracker`, `mislabeledWaterTracker`,
`PlayerStore.fountainMislabelRepLossLL/UL`, `vendingMislabelCount`.

### 6.2 `human_action` values (`record.water`)

Prefer the public static funnels; most are also called by machines at night, so tag
`context = night` when `CurrentConverterHelper.isNightCycle` is true (existing
`prod.night_cycle_flag`) and drop those from human actions.

| action | Hook (prefix + postfix, diff the container snapshot) | Notes |
|---|---|---|
| `water_pour` {src, dst, ul, filtered_ul} | `WaterHelper.TransferLiquid(GameItem, GameItem)` | also fired by moisture farm `Fill`, fountain, wine pours (ignore at night / when src is a fresh jug with no parent) |
| `water_tap` {dst, ul, purity_bp, tap} | `WaterHelper.AddWater(GameItem, int, int, bool, int, int, bool)` when `useRange && low == high == today tap purity` | faucet branch has no separate method |
| `water_drain` {src, ul} | `WaterHelper.EmptyContainer(GameItem)` | also wine `DrainBottle` (separate) |
| `water_test_strip` {target, owned, purity_bp, label_bp} | `WaterTestHelper.HandleWaterCheckTestingStrip(GameItem, GameItem)` | |
| `water_scan` {target, owned} | `WaterScannerUIManager.Open(GameItem, GameItem)` | instance; also `WaterHelper.SetLastScanResult(GameItem)` |
| `water_relabel_match` {target, from, to} | `WaterFeatureHelper.OnWaterLabelUpdated(GameItem)` | strip and scanner Match both funnel here |
| `water_relabel_manual` {target, grade} | `LabelerElement.OnDropdownValueChanged()` | `OnWaterLabelUpdated(ItemFeature, int)` is also used by the fountain |
| `water_confront` {target, outcome} | `WaterScannerUIManager.ConfrontWater(GameItem, Button)` and `WaterStripUIManager.OnConfront()`; outcome from existing `EmporiumAnalytics.TrackConfront` | |
| `water_uv` {target} | `WaterHelper.OnUVUsed(GameItem)` | |
| `water_dissolve` {item, target} (tablet / nutrient / other soluble) | `ChemicalHelper.DissolveInto(GameItem, GameItem)` | |
| `water_portable_purify` {target, charges} | `WaterHelper.PurifyToBaseWater(GameItem)` | only caller is the portable purifier |
| `water_spirit_pour` | `WaterHelper.PourSpirit(GameItem, int, int)` | |
| `water_tank_upgrade` {tank, material, new_ml} | `WaterTankHelper.Upgrade(GameItem, GameItem)` | |
| `filter_install` / `filter_remove` | existing `prod.inv_accept_slot` / `prod.inv_expel_slot` on the pitcher filter slot and RAS filter slots | |
| machine slot moves (purifier/farm/evaporator/hydro/RAS/fountain) | existing `prod.inv_*` hooks | |

Night outcomes (`night` events): `water_purified` {container, removed_ul, purity_after}
(`MachinePurifier.PurifyContainer` postfix result), `water_farmed` {container, ul, grade}
(`MachineMoistureFarm.Fill(System.String, GameItem, System.Int32)`), `fountain_sale`
{slot_grade, bottle, value, true_grade} (`VendingFountainHelper.SimulateWaterSale`), `ras_tick`
(`AquacultureHelper.TickTank` pre/post waste %), `water_crime` (`SecData.SellDirtyWater(int)`,
`SellMislabeledWater(int)` at sale time; `PlayerStore.CheckCrime()` pre/post trackers).
Counter outcomes: `water_exposed_by_customer` (`NegociationHelper.OnFeatureExposed(StoreClient)` when
the feature category is `CATEGORY_WATER_PURITY`).

## 7. Profitable and dangerous uses, by the numbers

Values are tooltip values before markup and market events (§1.3).

| Play | Inputs | Output | Net |
|---|---|---|---|
| **Keep the filter** | rust tap water (free) through a pitcher with a `water_filter` (25) + chlorine tablets (6 each, 1 per L at 91 %) | organic/physical/mineral stripped, then chemical to 0.7 % and microbes: ≈ 98.3 % **Basewater** | ~37 durability per litre → one filter ≈ **10.8 L**; rust 10/L → base 40/L = **+324 per filter** vs 25 for selling it |
| Portable purifier | rust/gutter water; 1 charge per 100 ml | ≥ 98.00 % base, label updated | 300 charges = 30 L → **+900** (rust→base); value 325 falls toward 25 as it drains |
| Purifier to HQ | pre-treated 98.3 % water, 3 L pitcher | one night: budget (9 + 25 ml) × 1.25 = 42.5 ml ≥ the ~21 ml needed → 99.01 % HQ | base 120 → HQ 180 per pitcher per night; re-test to price it |
| Purifier from raw rust | 6 L jug at 91 % | ≈ 196, 101, 78, 59… ml/night (dirt bonus shrinks) → HQ after ~5 nights | 60 → 360 — pre-treating first is ~5× faster |
| Pure water | purifier quality bonus ≥ 100, or evaporator with quality bonus (distillate 100 %), or moisture farm quality ≥ 150 | Pure | 80/L; required for top berries, pulls the stockpiler (500 + day budget) |
| Moisture farm | power only | 850 ml ghost/night unupgraded (26.8/L ≈ 23/night) | each extra farm cuts all farms' output (÷ 1 + 0.25 per extra) |
| Wine | swap a base ration for HQ/pure water | +1 score: e.g. Production → Reserve (+25 % on 100 base) or Undrinkable → Bucket Brew (−100 % → −15 %) | 0.75 L of HQ (45) buys +25…+85 value per bottle |
| Bloomberries | pure (or HQ) + nutrient tablets to 125–275 mineral bp | pure + ~6 tablets per 6 L: 0 % bad, 62 % best; HQ + tablets ≈ 22 % bad; base water ≈ 75 % bad (measured, docs/sim/VERIFIED.md) | the difference between moldy and +2 berries for wine |
| Fish | filters in RAS slots | waste held near 0 | a 400-durability filter = 400 ml waste ≈ 80 kg-nights of tilapia |

Dangers:

| Risk | Numbers |
|---|---|
| Over-labelled water to a checking customer | exposure on negotiation (tier clients always check), −1 rep, price falls to truth |
| Ghost sold as base/HQ/pure | 1/2/4 × ml into the endangerment tracker; 2.5 L tracked → 100 crime + threat note; decays only 5 ml/day |
| Base→HQ/pure, HQ→pure | fraud tracker, same 500 ml = 20 crime rate |
| Fountain slot above the water's grade | crime as above **plus** ≥ 1 rep/night (LL/UL) and a night-log line; rust/gutter sources are simply not sold |
| Topping up a labelled container with worse water | label stays — silent mislabel (§1.5) |
| Purified/distilled water left un-retested | sells at the stale (often Gutterflow) label: −90 % |
| Boiling (> 100 °C, upgraded plate) | evaporates water 2 %/s → purity falls |
| Base water in a hydroponic bay | bloomberry health ≈ 16 at the first harvest, ≈ 44 at refruits → ≈ 75 % bad berries; Ghostwater/Rustwater → 100 % bad (measured, docs/sim/VERIFIED.md) |
| RAS unpowered | −34 % health/night and filters off |
| Selling the filters, purifier, UV, AquaScan | gives up the use value above (catalogue below) |

## 8. Item catalogue (sell value vs use value)

`sold as` = game item types (what customer type lists match) and base value; container values
exclude the water inside. Durability items price linearly toward their zero-durability value
(`DurabilityHelper.InitItemWithDurabilityPricingMode2`).

| Item | sold as | used for | consumed? | effect |
|---|---|---|---|---|
| `water_test_strip` | HOUSEHOLD_GOOD, WATER_PURIFICATION_SUPPLY; 3 (0 once spent) | test any container (incl. customers') | yes, 1 use | exact purity + grade; exposes/relabels owned water; enables Confront on a seller's over-label |
| `water_test_container` | HOUSEHOLD_GOOD, WPS; 1 (+ strips) | holds 10 strips | no | — |
| `aquascan` | HOUSEHOLD_GOOD, WPS; 200 | full composition, free relabel (Match), Confront | no | the only per-liquid reading; makes every relabel free |
| `uv_filter` | WPS; 150 | use on a container, or in an RAS filter slot (not the pitcher) | no (no durability) | all microbes → water / RAS: strips microbes nightly |
| `water_tablet` | HOUSEHOLD_GOOD, WPS; 6 | dissolve into water | yes | 15 ml chlorine: chemical → 0.7 % floor, then microbes (76.5 ml potency) |
| `water_tablet_container` | HOUSEHOLD_GOOD, WPS; 1 | holds 9 tablets | no | — |
| `nutrient_tablet` | FARMING_SUPPLIES; 5 | dissolve into hydroponic water | yes | +15 ml mineral (plant food; lowers purity) |
| `nutrient_tablet_container` | FARMING_SUPPLIES; 1 | holds 9 | no | — |
| `water_filter` | HOUSEHOLD_GOOD, FILTER, WPS; 25 → 1 | pitcher filter slot or RAS filter slot | durability 400 | size 22: strips organic, physical, mineral, microplastic, mush/debris (≈ 10.8 L of rust water; 400 ml fish waste) |
| `water_filter_adv` | HOUSEHOLD_GOOD, FILTER, WPS, **LUXURY_ITEM**; 350 | same | durability 400, **+50/night** while owned | same; ≈ 50 ml waste or ~1.35 L rust water per night forever |
| `hydroreed_filter` | HOUSEHOLD_GOOD, FILTER, WPS; 20 | same | durability 200 | same; grown from hydroreed with its module |
| `kotton_fabric` | MATERIAL, FILTER; 65 | filter slot | durability 1000 | size 150: catches only 200-size particles (debris, mushes, plant matter) — useless on ordinary water |
| `portable_water_purifier` | HOUSEHOLD_GOOD, WPS, **LUXURY_ITEM**; 325 → 25 | use on owned water < 98 % | 1 charge / 100 ml, 300 charges | to ≥ 98 % base, label updated; **no recharge**: the only code that changes its charge is the use itself |
| `water_pitcher` | HOUSEHOLD_GOOD (empty) / WATER (filled); 25 | 3 L graduated filter container | no | applies its filter to every pour/fill into it |
| `bottled_water`, `small_bottled_water`, `mini_bottle`, `large_bottled_water`, `water_jug` | HOUSEHOLD_GOOD empty / WATER filled; 5/4/3/9/15 | 750/450/250/2000/6000 ml | no | container value is added to the water value |
| `water_tank` | HOUSEHOLD_GOOD; 350 (+ upgrade materials) | 20–50 L graduated | no | bulk storage; purifier budget caps at 10 L, so big tanks purify slowly |
| `ras_water_tank` | —; 0 | the RAS tank (12 L), fixed | no | fish volume |
| `water_ration` / `water_ration_small` | WATER_RATION; 30 / 15 | sold to most water buyers, or poured into a wine bottle | yes | 750 / 375 ml base water (wine term 0) |
| `water_purifier` | MACHINE; 250 | nightly purification of one container | power | to 99.01 % (HQ); pure at quality ≥ 100 |
| `moisture_farm` | MACHINE; 350 | nightly water | power | 850 ml ghost (grade rises with quality) |
| `evaporator` | MACHINE; 300 | nightly distillation | power | 300 ml/night; distillate 100 % water × quality % |
| `heating_plate` | MACHINE; 250 | heat a container | power | microbe/organic kill ≥ 70/80 °C; boils water off > 100 °C |
| `bottle_printer` | 100 | print empty bottles | 4/6/11 power | containers |
| `vending_fountain` | — | sells water overnight by slot grade | power 2+ | revenue; mislabel = crime + rep |
| `labeler` | — | set any label | no | fraud tool; label only |
