# Production chains (RE notes)

Build 26525552 / 0.46D. Same conventions as `STATE.md` / `DAY.md`: **VISIBLE** = rendered by the
UI (evidence = the UI code), **HIDDEN** = internal only, **DERIVABLE** = computable from visible
facts. Types are in the global namespace of `Assembly-CSharp`. Everything is read from code, not
play-tested; "(inferred)" marks conclusions from the absence of code. Proposed hooks are in
`compat/hooks.proposed.json` under `record.production` and `snap.production`.

Water is an input to wine, hydroponics and several machines below. Its chemistry (purity,
minerals, contaminants, testing, treatment) is in [`WATER.md`](WATER.md); this file only says
where water enters a chain and which water term moves the output.

## 0. The two facts that shape everything

1. **All production ticks once per night, never in real time.** `PlayerStore.EndNight()`, after
   `ModHook.FireOnGoingSleepEarly()` and nightly services, walks every owned item
   (`PlayerStore.FindAllItem(System.Boolean)`: stock, counters, showcase, trash, docs, hidden stash,
   afterhour bag, vending, recursively including items inside machines and bottles) three times:
   `GameItem.onCycleEndEarlySlotItemFunc`, then `onCycleEndSlotItemFunc` (alongside
   `StolenHelper.UpdateStolenItem`), then `onCycleEndLateSlotItemFunc`, with
   `HopperHelper.RunWindow(1|2|3)` between passes and `CurrentConverterHelper.isNightCycle == true`
   throughout. `ModHook.FireOnGoingSleepLate()` follows. Every machine below implements its work in
   `onCycleEndSlotItemFunc`. One night = one cycle; nothing advances while the store is open or
   while a machine window is open. A night that is skipped (no sleep) does not happen: there is no
   catch-up.
2. **Value is staged.** `GameItem.GetCurrentValue(...)` / `GetNegociatedValue()` compute
   `round(((unitValue + powerValue) * (1 + innate%/100) + containerPrice) * (1 + market%/100) + misc) * final`.
   Production quality features (wine grade, ingot purity) are **innate** %; market events and the
   retail markup are **market** % (additive with each other). Aging adds to `unitValue`, so it is
   multiplied by the quality %.

Power: a machine draws `MachineryHelper.GetMachinePowerUsage(GameItem)` (tag
`machinery_drawn_power`) per cycle from the item in its battery slot
(`MachineHelper.TryDrawCyclePower(GameItem, GameSlotInventory)`). The usual battery is an
`energy_credit`, whose sale value is about 1 credit per charge unit
(`PowerHelper.GetPowerSourceItemValue`), so **power costs ~1 credit per unit**. Modules in the
module bay set `TOTAL_PERCENTAGE_{PERFORMANCE,POWER,QUALITY}_BONUS_INT`
(`ModuleHelper.ComputeModuleEffect`); `ModuleHelper.ApplyBasicModuleEffect` turns performance into
`CURRENT_PROCESSING_SPEED_TAG` (divided by `MACHINE_PERFORMANCE_RATIO_INT` when set) and efficiency
into lower power use. `MachineryHelper.GetCurrentQualityBonus(GameItem)` is the raw quality %.

## 1. Garden: hydroponic bay (`hydroponic`, `MachineHydroponic`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | `hydroponic`, unitValue 300, `MACHINE` | VISIBLE | `MachineHydroponic.Hydroponic()`; `FurnitureItemDirectory` |
| Panels (storage order) | 0 battery slot, 1 module bay (types `MODULE_TYPE_UNIVERSAL`, `MODULE_TYPE_HYDROPONIC`), 2 water slot (any owned `LIQUID_CONTAINER_TAG`), 3 seed slot (owned `SEED`, not a container), 4 output grid 9x6 (player cannot insert), 5 note | VISIBLE | `CreateMachineInventoryWindow`; `GetWaterSlot/GetInputSlot/GetOutputSlot(GameItem)` |
| Power | 8 per night | DERIVABLE | `InitMachinery(…, 8)` |
| Speed | 8 progress per night (`BASE_PROCESSING_SPEED`), performance modules at ratio 2 | VISIBLE (progress in tooltip) | `InitProgressTypeMachine(…, 8)`; `MACHINE_PERFORMANCE_RATIO_INT = 2` |
| Night logic | No seed: idle (finishes a dangling cycle). Seed but no power or not enough water: **paused, no progress, no penalty**. `STATE_READY` + seed: draw power, `HydroponicHelper.StartGrowing`, grow one cycle. `STATE_WORKING`: draw power, grow one cycle | tooltip status VISIBLE (`GetGrowStatus`: no water / no power / awaiting seed / starting / growing) | `Hydroponic()` cycle lambda |
| Planting | Drop a seed into slot 3 any time; growth starts on the next night. The seed cannot be removed while `STATE_WORKING` (slot remove filter). A hatchet removes (destroys) the plant: `HydroponicHelper.RemovePlantFromHydroponic(GameItem)` | VISIBLE | `SetupInputSlot`; `InitHatchetPlantRemoval` |
| Harvest | Automatic at night into the output grid; the player drags produce out. No ripeness window, no rot on the vine (inferred: nothing decays output) | VISIBLE | `HydroponicHelper.ContinueGrowing` → `FinishGrowing` → `PlantDefinition.onHarvest` |
| Water | `GetWaterUsePerCycle(seed)` ml per night (plant `waterUseRate` × genetics `WATER_USE`), removed from the water-slot container | VISIBLE (seed tooltip) | `HandleWaterConsumption` |
| Nutrients / contaminants | Each progress point checks the water slot's `mineral` level against `nutrimentLevel ± nutrimentRange` (below: health −1 %, above: −0.5 %) and consumes `mineralUseRate`; each other contaminant (heavy metal, chemical, microbe, organic, acid, base) costs health on a step table (0 up to 6.4 % per point) × `contaminantEffect` × genetics sensitivity. Chemistry: see `WATER.md` | HIDDEN in the tooltip; VISIBLE as "growth score" with the plant analyser (WATER.md) | `HandleNutriment`, `HandleContaminant`, `ApplyContaminantEffect` |
| Health | `PLANT_HEALTH_INT` (×100 scale), reset to `startingHealth` (100 × genetics) at each cycle start | HIDDEN | `StartGrowing`, `StartNextFruitingCycle` |
| Yield | `GetHarvestCount(seed)` = `round(base × (1 + quality/100))`, base = plant `harvestQuantity` × genetics `HARVEST_QUANTITY`, quality = `ModuleHelper.GetAjustedQuality` (quality bonus / `MACHINE_QUALITY_RATIO_INT` 2) | base VISIBLE (seed tooltip "harvest quantity"); module bonus DERIVABLE | `GetHarvestCount`, `GetBaseHarvestQuantity` |
| Growth time | `GetRequiredProgress(seed)` = `requiredProgress / GROWTH_SPEED multiplier`; perennials, once established, use `refruitProgress` (or 50 %) | VISIBLE ("required work") | `GetRequiredProgress`, `GetRefruitBaseProgress` |
| Upgrade path | Hydroponic modules (performance → speed, quality → yield); plant genetics via `PlantGeneticsHelper` / DNA manipulator (see §6) | | |

### Plants (`PlantList.Register()`)

| Seed id (value) | Produce | Work (first / refruit) | Nights at speed 8 | Base yield | Notes |
|---|---|---|---|---|---|
| `bloomberry_seed` (40) | `wine_berry` ×8 | 18 / 10 | 3, then **every 2** | 8 | Perennial: the seed is never consumed; nutrients 200±75, contaminant ×1.25, water 100 ml/night |
| `nutrifruit_seed` (40; 8 or 4 once freed) | nutrifruit (S/M/L/huge by health), +3 seeds | 23 | 3 | 5 | Size split by health and quality (`PlantHarvest.SplitByHealth`) |
| `kotton_seed` (20) | kotton fiber (46 each, sizes) or `kotton_fabric` with the loom module; 1–2 seeds | 32 | 4 | 9 | Water 176 |
| `dream_cap_seed` (24, contraband 1) | dream cap (23, contraband 2) / meal cap with module / nightmare cap with module; +1 seed | 28 | 4 | 8 | Contraband status stripped by the dream-cap or smuggler module |
| `coffee_bean` (28) | coffee beans, ×0.5–0.8 if health ≤ 70 | 21 | 3 | 8 | |
| `fillerweed_seed` (9) | fillerweed (11) ×(n−1) + 1 small, +1–2 seeds | 11 | 2 | 4 | Water 287; +4 with production module (no seeds then) |
| `hydroreed_seed` (20) | hydroreed filters (per filter module), +1–2 seeds | 18 | 3 | 0 | Purifier plant |

### Berries: the quality grades

Each harvested berry gets `BERRY_CONDITION_INT` from `PlantList.RollBloomberryCondition(health)`
(berries bought or spawned elsewhere use `WineHelper.InitBerry`: uniform over −2, −1, 1, 2).

| Condition | Look (inspection view, `InsBerryUI.OnInit`) | Wine score contribution |
|---|---|---|
| −2 | **moldy** | −1 |
| −1 | **under-ripe** (two variants) or **over-ripe** (`BERRY_VISUAL_VARIANT_INT` 0/1 vs 2; purely cosmetic) | 0 |
| 1 | normal | +1 |
| 2 | "small" sprite (the best grade) | +2 |

| Plant health at harvest | −2 | −1 | 1 | 2 |
|---|---|---|---|---|
| ≤ 25 | 55 % | 45 % | | |
| 26–55 | 25 % | 45 % | 30 % | |
| 56–85 | | 25 % | 45 % | 30 % |
| > 85 | | | 40 % | 60 % |

Visibility: the grade is VISIBLE as the berry's look in the inspection view (`InspectableHelper`);
the number is dev-mode only (`WineHelper.CreateTooltip`). Health is HIDDEN, so the grade mix is
the player's only read on how healthy the plant was. Under-ripe and over-ripe are the **same**
grade. A berry sells as FOOD: unitValue 14, no condition feature, so all grades sell for the same
price.

## 2. Wine: fermentation in the bottle (`wine_bottle`, `WineHelper`)

The bottle itself is the fermenter; there is no separate machine.

| Step | Rule | Vis. | Evidence |
|---|---|---|---|
| Bottle | `wine_bottle`, unitValue 12 (15 after reset), 750 ml liquid container, ingredient grid 6x2 accepting owned `WINE_INGREDIENT_TAG` (berries) or `WINE_YEAST_TAG` | VISIBLE | `WineHelper.InitWineBottle(GameItem)` |
| Fill | ≥ 749 ml water (pour a container on it, or drop a `water_ration` 750 ml / `water_ration_small` 375 ml) | VISIBLE (tooltip "water x / 750 ml") | `TryStartFermentation`, `PourRationIntoBottle` |
| Start | Window button `mech_wine_start_fermenting` → `WineHelper.TryStartFermentation(GameItem)` (refuses with a dialog without ≥ 1 berry, unless illegal yeast, or without water) → `StartFermentation(GameItem)` | VISIBLE | `InitWineBottle` button callback |
| Score | `WINE_SCORE_INT` = Σ berry contribution (table above) + 1 if ≥ 2 berries + 1 if any yeast + water-quality term (+1, +1, 0, −1, −2, −3 by purity band; see `WATER.md`) | **HIDDEN** until finished (dev tooltip only) | `StartFermentation`, `GetWaterQualityScore` |
| Consumed | All berries and yeasts in the grid are destroyed (reusable yeast is returned to stock); the water is drained | VISIBLE | `StartFermentation` |
| Duration | `FERMENTATION_TIME_REMAINING_INT` = 4, minus yeast power each night: no yeast 4 nights, `wine_yeast` (10 cr, power 2) **2 nights**, `wine_superyeast` (20, power 4) **1 night**, `wine_yeast_infinite` bioreactor (250, power 2, reusable) 2 nights, `wine_yeast_red` (8, power 2, contraband 1) 2 nights. Only the last yeast's power counts; extra yeasts are wasted | VISIBLE ("n nights remaining" = ceil(remaining / power)) | `onCycleEndSlotItemFunc` case 1; `CreateTooltip` |
| Finish | unitValue 100; quality feature `CATEGORY_WINE_QUALITY`; tier > 0 becomes drinkable `SUBSTANCE` + `ALCOHOL`, tier ≥ 4 also `LUXURY_ITEM` | VISIBLE (label sprite `wine_label<tier>`, feature text) | `FinishFermentation(GameItem)`, `ItemFeatureList.CreateWineQualityFeature(System.Int32)` |
| Undrinkable | Tier 0 can be emptied back to a bottle (`EmptyWineBottle`) | VISIBLE | `CanEmptyWineBottle` |
| Illegal yeast | Always "gloomberry" counterfeit: unitValue 44, contraband level 2, `ENDANGERMENT_ITEM_TAG`, fixed score 3, never ages | VISIBLE (red label, contraband tooltip) | `FinishCounterfeitWine` |

| Score | Tier | Grade (`ItemConditionList`) | innate % | Value at finish |
|---|---|---|---|---|
| ≥ 6 | 5 | Master Vintage | +75 | 175 |
| 5 | 4 | Reserve (LUXURY) | +50 | 150 |
| 3–4 | 3 | Production | +25 | 125 |
| 2 | 2 | Homebrew | 0 | 100 |
| 1 | 1 | Bucket brew | −15 | 85 |
| ≤ 0 | 0 | Undrinkable | −100 | 0 |

Examples with neutral water (term 0) and one yeast: one moldy berry 0 → undrinkable; one
under/over-ripe berry 1 → bucket brew 85; one normal berry 2 → homebrew 100; one best berry 3 →
production 125; two normal 4 → production; best + normal 5 → reserve 150; two best 6 → master
175. A moldy berry in a pair costs a full grade; an under/over-ripe one costs nothing but gives
nothing beyond the ≥ 2 bonus. Pure water (+1) lifts every row one step.

### Aging

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Where | **Anywhere owned.** A finished, non-counterfeit bottle ages in its own night callback wherever it sits (stock, showcase, counters) | VISIBLE (tooltip "aged n days") | `InitWineBottle` cycle case 2 → `WineHelper.OnAgeWine` → `AgableHelper.OnAge` |
| Rate | `AGE_INT` +1 and `unitValue` +3 per night (`AGE_VALUE_MOD_PER_DAY_INT`), so +3 × grade multiplier credits per night (Master Vintage +5.25) | DERIVABLE | `AgableHelper.OnAge` |
| Cap | **None.** No upper bound on age or value in code | (inferred) | `AgableHelper` |
| Wine rack (`wine_rack`, "Age Well", unitValue 350) | 6x4 bottle grid (finished non-counterfeit wine only), battery, module bay. Each night with power (24, cut by performance and efficiency modules) it applies `1 + max(0, quality%)/125` extra aging steps to every bottle | VISIBLE | `MachineWineRack.AgeWell()`, `GetAgingDays(GameItem)` |
| Rack + own tick | Bottles inside the rack are found by `FindAllItem`, so they also take their own +1 (inferred): **2 steps/night** without quality modules, 3 at 125 % quality | (inferred) | `PlayerStore.FindAllItem` recursion |
| Reset | Drinking or pouring out resets age and grade (`ResetToEmptyBottle`) | | |

Sources: bottles from the retired winemaker (4 per visit, also yeasts, 8 berries, water), the
`WINEMAKER` Wild upgrade (cost 200: 3 yeast, 3 bottles, 2 berries, water, magnifier, book), Dr.
Jackson (wine rack 33 % when winemaker unlocked), supplier superyeast (stolen); seeds from the
retired farmer and Dr. Jackson. `StoreClientListSpec.CreateRetiredWinemaker`,
`NetworkUpgradeList` (`WINEMAKER_COST`), `StoreClientList` inventor stock.

## 3. Furnace (`furnace`, `MachineFurnace`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | `furnace`, unitValue 200 | VISIBLE | `MachineFurnace.Furnace()` |
| Panels | battery, module bay (`MODULE_TYPE_UNIVERSAL`, `MODULE_TYPE_FURNACE`), input grid 9x6 (`scrap_metal`, `common_ore`, `flux_agent`, `advanced_flux_agent`; `junk` only with the junk module), output grid 9x6 | VISIBLE | `CreateMachineInventoryWindow` |
| Per night | **One smelt** if the output can take an ingot, ≥ 2 metal inputs (3 with the blast module) and power: consumes the last 2 (3) scrap/ore, makes 1 `metal_ingot` (2 with blast) | VISIBLE (result next morning) | `Furnace()` cycle lambda |
| Power | 8 per smelt (only drawn when it smelts) | DERIVABLE | `InitMachinery(…, 8)` |
| Purity | Level = LOW + `2 × ore / inputs` (integer) + flux level, clamped to VERY_LOW…PERFECT. Flux level = quality% / 100 (−1 with blast); a `flux_agent` raises it to at least 1 and is consumed; an `advanced_flux_agent` raises it to at least 1 and is **never consumed** | label HIDDEN until tested (flashlight shows only LOW/VERY_LOW, analyser shows all); price DERIVABLE: the true modifier is applied before testing | `IngotPurityHelper.SetPurityFromInput`, `GetPurityConst`, `SetPurity` |
| Junk module | If no smelt happened that night: 6 `junk` → 4 `scrap_metal` | | `HaveJunkModule` branch |
| Modules | `furnace_module_blast` (150): 3 in → 2 out, −1 flux level; `furnace_module_junk` (120) | | `ModuleDirectory` |

| Purity | Level | innate % | Ingot value (base 50) |
|---|---|---|---|
| Perfect | 10 | +200 | 150 |
| Very high | 9 | +50 | 75 |
| High | 8 | +25 | 62.5 |
| Fair | 7 | 0 | 50 |
| Low | 6 | −15 | 42.5 |
| Very low | 5 | −25 | 37.5 |

| Recipe (per night) | Purity | Out | In cost | Margin before power |
|---|---|---|---|---|
| 2 scrap | Low | 42.5 | 20 | 22.5 |
| 2 scrap + flux | Fair | 50 | 24 | 26 |
| 1 scrap + 1 ore + flux | High | 62.5 | 29 | 33.5 |
| 2 ore + flux | Very high | 75 | 34 | 41 |
| blast: 3 scrap + flux | Fair ×2 | 100 | 34 | 66 |
| blast: 3 ore + flux | Very high ×2 | 150 | 49 | 101 |

Inputs: `scrap_metal` 10, `common_ore` 15, `junk` 3, `flux_agent` 4, `advanced_flux_agent` 300,
`metal_ingot` 50 (`MaterialDirectory`). Ingots are `MATERIAL`, so MATERIAL market events move
them (§5).

## 4. Other machines and systems

Survey level (one pass over each `Machine*.cs`; numbers from code, less cross-checked than §1–3).
"Night" = `onCycleEndSlotItemFunc` unless noted. Water machines are listed for their place in
the chain; their chemistry is in `WATER.md`.

| Machine (id, unitValue) | Power | Inputs → outputs, timing | What sets output | Entry points | Source |
|---|---|---|---|---|---|
| Water purifier (`water_purifier`, 250) | 4, only if it removed something | Purifies one liquid container in place, once per night | Max purity by quality bonus (99.01 % at < 15, 100 % at ≥ 200); budget grows with dirtiness. See `WATER.md` | `MachinePurifier.WaterPurifier()`, `PurifyContainer(GameItem, GameItem, System.Boolean)` | Dr. Jackson with `WATER_TRADER` (300); retired water merchant |
| Evaporator (`evaporator`, 300) | 6 | Removes up to 300 ml (× performance) of evaporable liquid per night; distillate = evaporated × quality % (solvent-recovery module: alcohol only) | quality % = recovery | `MachineEvaporator.Evaporator()` | `RETIRED_CHEMIST` bundle (1000) |
| Moisture farm (`moisture_farm`, 350) | 6 | Fills a container with 850 ml × (1 + perf %) per night, ÷ (1 + 0.25 × (farms running − 1)) | Water grade from quality + 50 (Ghostwater by default) | `MachineMoistureFarm.MoistureFarm()`, `GetOutputVolume`, `GetOutputQuality(GameItem)` | Dr. Jackson with `WATER_TRADER`; Scrapper (broken) |
| Bottle printer (`bottle_printer`, 100) | 4 / 6 / 11 **per button press** (real time) | Prints an empty `small_bottled_water` (450 ml, 4) / `bottled_water` (750 ml, 5; needs 3 upgrades) / `large_bottled_water` (2 L, 9; 9 upgrades) | Upgrades by targeting with electronic material, max 9 | `MachineBottlePrinter.BottlePrinter()`, `UpgradePrinter(GameItem, GameItem)` | starting kit; Dr. Jackson with `WATER_TRADER` |
| 3D printer (`printer`, 350) | 8 per working night | Chip (kept) + materials → goods; progress machine, speed 24/night vs chip target 10–120 (e.g. 1 plastic → 6 small bottles in 1 night; 2 plastic → printed gun in 3; 1 ingot → 2–5 gun parts by purity) | Ingot purity (−30…+100 quality), quality ratio 4 | `MachinePrinter.Printer()`, `FinishPrinting(...)` | `RETIRED_GUNSMITH` (650) |
| Desequencer (`desequencer`, 350, **contraband 2**) | 10 | crypto chip (kept) + blank keycard (15, consumed) → keycard; speed 33/night vs 75–150 (3–5 nights); outputs contraband 2–3 (sec 100, cmd 125) | — | `MachineDesequencer.Desequencer()` | Dr. Jackson |
| Deal maker (`deal_maker`, 400) | 25 per activation | Button while a customer is present, once per customer: `PlayerStore.dealMakerBargainBonus` = 10 + quality/10 % bargain chance. Not nightly | quality | `MachineDealMaker.TryActivate(GameItem, GameSlotInventory)` | no source found in code |
| Blender (`blender`, 200) | none | Button: blendable items → liquid in a container | loss 35/20/10/5/0 % by chem modules | `MachineBlender.Blender()` | `RETIRED_CHEMIST` |
| Chem finisher (`chem_finisher`, 300) | none | Button: container liquid → product (dream dust, caffeine pill…) | impurity 12/6/3/1/0 % by modules | `MachineChemFinisher.ChemFinisher()` | `RETIRED_CHEMIST` |
| Heating plate (`heating_plate`, 250) | none | **Real time** (`onOneSecondItemFunc`), and 360 simulated seconds per night via `ChemicalHelper.RunNightlyChemicalSimulation()` | chem modules set target temperature | `MachineHeatingPlate.HeatingPlate()` | `RETIRED_CHEMIST` |
| Wine rack (`wine_rack`, 350) | 24 | §2 | | | |
| Fridge (`fridge`, 100) | 2+ | Chill/freeze zones add +20/+25 % features to drinks/ice cream; features expire unless kept cold each night | | `MachineFridge` | Dr. Jackson |
| Auto butcher (`auto_butcher`, 400), RAS (`ras`, 300), feed dispenser (300) | 0 / 5 / — | Aquaculture and husbandry (Late window for the butcher) | | `MachineAutoButcher.Run(GameItem, System.Boolean)` | `AQUACULTURE` (400), `RETIRED_RANCHER` (750) |
| Plant DNA manipulator (`plant_dna_manipulator`, 1000) | 12, speed 10 | Moves traits between seeds and DNA disks (growth speed, yield, water use…) | | `MachineDnaManipulator` | `PLANT_DNA` (1250) |
| Projector, alarm, recharger, cassette player | 16 / 4 / — / — | Attractiveness (`WINDOW.md`), burglary stop, recharge batteries +3/night, music | | | |
| (unreleased content) | | Some machines exist in code but are not obtainable in this build; deliberately not documented here | | | — |

Turbo boosters (`turbo_booster_adv`, 300, reusable, recharges in 3 nights; hypertech 15000) give
any `STANDARD_MACHINE_TAG` machine (hydroponic, furnace, wine rack, purifier, printer…) or
`TURBO_BOOST_ELIGIBLE_TAG` one an **extra cycle on demand**:
`MachineTurboBoosterAdv.BoostMachine(GameItem)` runs its Early and main night callbacks
immediately (not Late, no hoppers). On a bloomberry bay that is one more harvest step; on a furnace
one more smelt. A boosted bottle does not age (bottles are not machines).

Labeler (`labeler`, 50): `LabelerHelper.OnTargetItem(GameItem)` → `LabelerUIManager` sets a
feature's `fakeCondition` for any condition category except ingot purity, equipment condition and
**wine quality** (so wine grades cannot be faked). Over-labeling raises the price until a customer
exposes it and is a FRAUD crime (`DAY.md`). It is a sales tool, not production.

## 5. Market events, shortages and speculation

### How an event moves a price

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Effect | Each active `StoreEvent` carries `negociationDatas : List<NegociationData>` (`itemType`, `value` %). An item whose types include `itemType` gets `value` added to its **market** % (same stage as the retail markup); several events and several matching types on one item add up | VISIBLE (news, calendar, item "Market" feature `mech_market_price_change`) | `GameItem.GetCurrentValue`, `ItemFeature.CreateFeatureFromNegociationData`, `GameItem.AddFeatureRelatedToEvent` |
| Buy and sell alike | The same % applies to items customers bring in, so buying during an event costs the event price too | | |
| Lifetime | Starts at `StoreStation.StartDay()` → `StoreEventManager.OnDayStart()`; `duration` drops by 1 per night (`OnDayEnd`, called from `EndOfDayUIManager` after `EndNight`); removed at 0 | VISIBLE (calendar span) | |
| Random events | From day 6 (intro), each morning: if < 3 non-cosmetic events are active, roll 50 % (0 active) / 35 % (1) / 25 % (2) and **activate immediately** a weighted pick not on cooldown. **No advance warning** | HIDDEN until it starts | `HandleNewEventNormale`, `PickRandomEventByType` |
| Cooldown | `duration + cooldown` (default 3) from start, extended to cover queued copies | HIDDEN | `ApplyCooldown`, `ApplyCooldownAfterEnd` |
| Opposing events | A new event that moves a type opposite to an active one replaces it (priority rules) | | `GetImcompatibleEventPreCheck` |
| Advance notice | Only queued events are known in advance: information-dealer tips (1–4 days ahead, with a paper slip counting down), the LL Tips perk rumor (`PickRumorEvent`, 4 % per opening, 2–3 days ahead), plus scripted/innate ones. Faction operations are queued 4 days ahead but `IsKnown = false` | VISIBLE (calendar, slip) when known | `QueueFuturEvent`, `StoreClientListInformationDialog`, `PlayerStore.OnLateShutterOpen` |
| Calendar | 14 cells starting at this week's Monday (7–13 days ahead), known non-cosmetic events only | VISIBLE | `AdvCalendarUIManager.OpenUI`, `StoreEventManager.GetActiveEventOnDay` |

### The events (normal pool, `StoreEventManager.normalEventBlueprints`, weight)

| Event (weight) | Days | Price effects |
|---|---|---|
| `badNutrifruitHarvest` (15) | 3 | FOOD **+50**; LL soup-kitchen request; a food buyer each day (budget 75–99) |
| `seedShortage` (15; retired once nutrifruit is freed) | 2 | FOOD +33, SEED +50; farmer seed buyer |
| `nutrifruitHarvest` (tip only) | 2–4 | FOOD −20 |
| `medicalSuppliesShortage` (15) | 2 | MEDICAL +50; three med requests; pharmacist buyer |
| `water_treatment_breakdown` (15) | 3 | WATER/WATER_RATION +50, BREVAGE +45, WATER_PURIFICATION_SUPPLY +40 |
| `scheduled_treatment_maintenance` (15) | 2 | WATER +35, BREVAGE +30, purification +25 |
| `miner_return_ice_asteroid` (15) | 2 | WATER −25, BREVAGE −20, purification −15 |
| `construction_project` (15) | 3 | MATERIAL +30; foreman buyer |
| `meteorShower` (story) | 3 | MATERIAL +50 |
| `miningExpeditionReturn` (15) | 1 | MATERIAL −25; miner seller |
| `storeroom_excavated` (15) | 1 | HOUSEHOLD_GOOD −20 |
| `lower_level_mob_justice` (8) | 1 | WEAPON/AMMUNITION/ARMOR +33 |
| `oxymoreWave` (6) | 2 | MEDICAL +25 |
| `powerShortage` (7) / `powerPlantMaintenance` (10) / `stationWideBlackout` (4) | 2 / 1 / 2 | ENERGY_CELL +66 / +33 / +50 (+WEAPON/AMMO +25); blackout cuts power unless power-hijack |
| `cruiseShipDocked` (10) | 2 | clients only |

Faction operations (`OperationList*`): LL riot FOOD +30 (1 day), LL worker strike FOOD/WATER +40
(3), UL food drive FOOD −50 (3), UL medical aid MEDICAL −25 / FOOD −10, BM gang war MEDICAL +50,
Rev mobilization WEAPON/AMMO/MEDICAL/ARMOR +40, Sec raid on BM NARCOTIC +50, and others.

**No event touches SUBSTANCE, ALCOHOL or LUXURY_ITEM** (except NARCOTIC): wine prices are
event-proof. FOOD (berries) and MATERIAL (ingots, scrap) are the event-sensitive outputs here.

### Shortage clients

`StoreClientManager.HandleMarketClient()` is meant to add a shortage buyer
(`StoreClientListShortage.Create{Food,Medical,Substance,Material,WeaponArmor}ShortageClient`,
budgets 125–175 / 150–200 / 100–130 / 190–240 / 250–300) when a major type's total event % is
≥ 30, and a wholesale seller (`StoreClientListSurplus.*`, wholesale −15 %, −20 % at ≤ −75) when
≤ −30. **In this build it never fires**: `PlayerStore.GetMarketModifierType(List<System.String>)`
keys its sums by `NegociationData.identifier`, which is a fresh GUID, not the type name, so the
filter never matches and the list is always empty (inferred from code; a play-test with an active
FOOD event would confirm). The per-event clients in `StoreEventActionDict` (food buyer, foreman,
pharmacist…) do arrive, one per active day.

### Speculation math

A held item's sale value during an event, relative to normal, is
`(1 + (markup + E)/100) / (1 + markup/100)` where `markup` = `PlayerStore.retailMarkup` (15 by
default) and E = the summed event %. Food bought at the normal price and sold during a bad harvest
(+50): ×1.435 (+43.5 %); during LL strike + bad harvest together: ×1.78. Holding cost is zero for
non-perishables (berries do not rot in code; check food expiry for packed food). The catch:
random events give **no lead time**, so the trade is only plannable with a tip (1–4 days) or a
rumor (2–3 days); otherwise the best one can do is hold a food buffer and sell into an event when
it lands. Event-specific buyers have small budgets (food buyer 75–99), so most of the uplift is
realized through ordinary FOOD customers.

## 6. Dumping-ground scavenging (`ScavHelper`)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Access | One outing per night (`PlayerStore.visitLeftTonight` = 1 at close); blocked when seriously wounded (`woundState > 5`), during a `crackdown`, or with the Maimed perk | VISIBLE (door tooltip) | `StoreDoor.OnClick`, `GetTooltip` |
| Attempts | 5 per visit (+2 Proficient Scavenger, −2 Careful Scavenger, osteoarthritis −1/week, + `scav_attempts` event modifier; Georgio's assistance +5). Reset on arrival | VISIBLE (panel "attempts remaining") | `GetMaxScavAttempts`, `DumpingGroundLocation.Arrive` |
| Wounds | Per attempt, minor `5 + attempts so far` %, else major `1 + 0.25 × attempts` % (halved with a flashlight in the bag, × `injury_chance` modifier; 0 with Careful Scavenger or `injury_immunity`). Luck can cancel a wound. First 10 / 20 searches of a run are immune to minor / major | VISIBLE (panel shows both %) | `GetMinorWoundChance`, `GetMajorWoundChance`, `ScavengeDumpingGrounds` |
| Wound cost | Minor `woundState` +2, major +5; a fresh wound ends scavenging for the night; > 5 blocks outings; heals −4/night once stable (10 % worsen / 50 % stabilise per night otherwise) | VISIBLE (night log) | `HealthData.ReceiveMinorWound`, `HandleNightlyWound` |
| Bad luck | Negative luck: chance of an empty search | | `HealthData.RollBadLuck` |
| Loot | One roll of table group `dumpingGroundTG` (contents live in Unity data, not the decompile; read them with the F8 `loot-list dumpingGroundTG`), plus: 3 % the satchel then the kids' backpack (once each), 1 % degraded cassette, 1 % mouse trap, 10 % each `nuts_metal` / `wire` / `scrap_metal` when a screwdriver / wire cutter / welder is in the bag, and a double-loot roll at the metal scanner's chance (15 %, +5 per upgrade, max 100) + `loot_double_chance` | VISIBLE (items land in the afterhour bag) | `GetRandomScavengedItem`, `SpawnDumpingGroundLoot`, `InitScavScanner` |
| Events | Wednesday `fresh_dump` (junk −50 % weight, injuries +25 %); `georgio_assistance` (no injuries, +5 attempts) | VISIBLE (location card) | `VanillaAfterhourEvents` |
| Diminishing returns | Only through rising wound odds within a visit; no depletion across nights (inferred) | | |
| Stolen / heat | Finds are spawned plainly; nothing in the scavenging path marks them stolen or adds heat (inferred) | | `ScavHelper` has no `StolenHelper` call |

Expected value per night is the table's mean value × (≈ 5 attempts × (1 − wound odds)) plus tool
bonuses; with a welder, +0.5 scrap (5 cr) per night feeds the furnace.

## 7. Most profitable chains (code numbers, neutral water, default markup ignored)

| Chain | Capital | Per cycle | Gross out | Inputs + power | Net / night |
|---|---|---|---|---|---|
| Bloomberries sold as food | hydroponic 300 + seed 40 | 8 berries / 2 nights | 112 | 16 power + 200 ml water | ~48 |
| Bloomberries → wine, 2 berries/bottle (healthy plant: 60 % best) | + bottles, yeast | 4 bottles / 2 nights | ~4 × 150–175 ≈ 650 | 4 × (12 bottle + 10 yeast) + 3 L water + 16 | **~280** |
| Bloomberries → wine, 1 berry/bottle | same | 8 bottles / 2 nights | ~8 × 115 ≈ 920 (best 125, normal 100) | 8 × 22 + 6 L + 16 | **~360**, bottle-limited |
| Wine aging | rack 350 (optional) | per bottle | +5.25/night (Master), +10.5 in a rack | 24 power for the whole rack | +5 to +10 per bottle |
| Furnace, 2 scrap + flux | furnace 200 | 1 ingot / night | 50 | 24 + 8 | ~18 |
| Furnace, blast + 3 ore + flux | + module 150 | 2 ingots / night | 150 | 49 + 8 | **~93** |

So the owner's instinct is right, with two corrections from the code: (1) **wine dominates** —
a berry is worth 14 as food but 85–125 as a one-berry wine, so even under/over-ripe berries
(bucket brew 85 with yeast, more with pure water) beat selling them as food when a bottle is free;
only moldy berries should never go in (they cost a grade). The binding constraint is bottles and
yeast, not berries. (2) Wine is untouched by market events, so speculation belongs to FOOD and
MATERIAL stock, not to the cellar; aging is uncapped and free outside the rack, so unsold wine
simply appreciates.

## 8. Recording

### `snap.production` (at `close`, `sleep`, and `morning` snapshots)

```
production{
  machines[{uid, id, kind, where, powered, power_per_night, battery_charge,
            state, progress, target, speed, eta_nights,
            modules[{id}], quality_bonus, performance_bonus,
            inputs[{uid,id}], outputs[{uid,id}], status_text}],
  hydroponics[{uid, seed{uid, plant_id, perennial, established}, progress, target,
               eta_nights, harvest_count, water_ml, water_use, status, health(HIDDEN)}],
  wine[{uid, where, state: empty|filled|fermenting|finished, water_ml,
        ingredients[{uid, id, berry_grade}], nights_left, tier, grade, age, value,
        counterfeit, score(HIDDEN)}],
  furnaces[{uid, inputs{scrap, ore, junk, flux, adv_flux}, output_count, blast, junk_module}],
  berries[{uid, where, grade: moldy|under|over|normal|best}],
  scavenge{attempts_left, minor_pct, major_pct, wounded_fresh, scanner_double_pct}
}
```

| Field | Read from | Vis. |
|---|---|---|
| machine list | `PlayerStore.FindAllItem(true)` filtered by tag `machinery` / `STANDARD_MACHINE_TAG`, plus bottles with `WINE_CONTAINER_TAG` | VISIBLE |
| state / progress / target | `MachineProgressHelper.GetMachineState(GameItem)`, `GetProgress(GameItem) : (System.Int32, System.Int32)`; speed `CURRENT_PROCESSING_SPEED_TAG` | VISIBLE (tooltip bar) |
| ETA (progress machines) | `ceil((target − progress) / speed)` nights, assuming power + water | DERIVABLE |
| inventories | `MachineHelper.GetBodyStorage(GameItem, System.Int32)` per panel index; battery `GetBatterySlot`, modules `GetModules(GameItem)` | VISIBLE |
| power | `MachineryHelper.GetMachinePowerUsage(GameItem)`; can run `MachineHelper.CanPower(GameItem)` | VISIBLE |
| quality / performance | `MachineryHelper.GetCurrentQualityBonus`, `GetCurrentPerformanceBonus(GameItem) : System.Int32` | VISIBLE (module tooltip) |
| hydroponic seed | `MachineHydroponic.GetInputSlot(GameItem).childItem`; `HydroponicHelper.GetPlantId`, `GetRequiredProgress`, `GetHarvestCount`, `GetWaterUsePerCycle`, `IsPerennialEstablished`, `MachineHydroponic.GetGrowStatus(GameItem)` | VISIBLE |
| plant health | `HydroponicHelper.GetHealthPercent(GameItem)` | HIDDEN without the plant analyser; VISIBLE as its "growth score" when the player owns one (WATER.md) |
| bottle state | tags `WINE_CONTAINER_STATE` (0 empty/filling, 1 fermenting, 2 finished), `FERMENTATION_TIME_REMAINING_INT` / `WINE_YEAST_POWER_INT` (nights = ceil), `AGE_INT`; `WineHelper.GetWineQualityTier(GameItem)` once state 2 | VISIBLE |
| wine score | tag `WINE_SCORE_INT` | **HIDDEN** while fermenting |
| berry grade | tags `BERRY_CONDITION_INT` + `BERRY_VISUAL_VARIANT_INT` mapped as §1 | VISIBLE (look) |
| ingot purity | `IngotPurityHelper.GetPurity(GameItem)`; shown only if the feature `IsFeatureDiscovered()` | HIDDEN until tested |
| scavenging | `ScavHelper.GetScavTimeLeft()`, `GetMinorWoundChance()`, `GetMajorWoundChance()` | VISIBLE |

### `human_action` values (`record.production`)

The game has almost no named production funnel; most moves are drags. One generic funnel catches
them all: postfix `GameGridInventory.UncheckedAccept(GameItem)` / `GameSlotInventory.UncheckedAccept(GameItem)`
(add) and `GameGridInventory.Expel(GameItem)` / `GameSlotInventory.Expel(GameItem)` (remove); both
end by invoking the inventory's `onSlotAddItemEndFunc` / `onSlotRemoveItemEndFunc`. Keep events
whose `GameInventory.GetParentItem()` is a machine or a wine bottle, classify by the panel index
(`MachineHelper.GetBodyStorage`), and **suppress** them while `CurrentConverterHelper.isNightCycle`
is true (machine outputs), during save load (`ModHook.OnGameLoaded*`), and inside game-driven
moves already guarded in `DAY.md`.

| `action` | Detection | Data |
|---|---|---|
| `plant` | add to hydroponic panel 3 | `machine_uid`, `seed{uid, plant_id}` |
| `unplant` | remove from panel 3 (only allowed when ready) or prefix `HydroponicHelper.RemovePlantFromHydroponic(GameItem)` (hatchet) | |
| `water_machine` / `unwater_machine` | add/remove on hydroponic panel 2 (or any machine water slot) | container uid, ml |
| `harvest` | remove from hydroponic panel 4 (output) | items[{uid, id, berry_grade}] |
| `load_machine` / `unload_machine` | add/remove on any other machine input or output panel (furnace input/output, rack, …) | `machine_id`, `panel`, item |
| `battery_in` / `battery_out`, `module_in` / `module_out` | add/remove on panel 0 / module bay | |
| `bottle_fill` | prefix `WineHelper.PourRationIntoBottle(GameItem, GameItem)` for rations; other pours go through `WaterHelper.TransferLiquid` (see `WATER.md`), so diff the bottle's ml between snapshots | ml after |
| `bottle_add` / `bottle_remove` | add/remove on a bottle's ingredient grid (parent has `WINE_CONTAINER_TAG`) | berry grade or yeast id |
| `ferment_start` | prefix + postfix `WineHelper.StartFermentation(GameItem)` (prefix reads the berries and yeast before they are destroyed; postfix reads nights left, and `WINE_SCORE_INT` as `hidden`) | `bottle_uid`, berries[grade], yeast, water_ml |
| `ferment_refused` | postfix `WineHelper.TryStartFermentation(GameItem)` returning false | reason (no berry / no water) |
| `wine_empty` / `wine_pour` / `wine_drink` | prefix `WineHelper.ResetToEmptyBottle(GameItem)` (all three end there; `PourWineIntoContainer(GameItem, GameItem)` and the drink path distinguish) | grade, age lost |
| `discard` | add to `EmporiumEntry.trashInvElement` (existing inventory diff) | grade for berries |
| `sell` / `buy` | existing barter hooks (`RunStats.OnItemSold`, `OnItemBought`) | the sort decision for berries is the pair (harvest grade → sold vs bottled) |
| `scavenge` | existing `DAY.md` hooks (`RunStats.OnScavengeAttempt`, `OnScavengeInjury`, `OnScavengedItems`) | |

Night outcomes (`night` events, not human actions): postfix `HydroponicHelper.FinishGrowing(GameItem)`
(harvest made; read the output grid diff), `WineHelper.FinishFermentation(GameItem)` (grade
revealed), `AgableHelper.OnAge(GameItem)` (count per bottle, aggregate), the furnace via the
output-grid diff between `sleep` and `morning`.

For the strategist and the simulator, the morning snapshot plus these night outcomes is enough to
replay production: every rule above is a deterministic function of the snapshot except the berry
grade roll (seeded by `RNG`, depends on HIDDEN health), scavenging loot, and event rolls.

## Riskiest / least certain

- Shortage/surplus clients being dead (GUID key) is a code reading; confirm in play with an
  active FOOD event and the clientele forecast.
- Bottles aging twice per night inside the rack relies on `FindAllItem` recursing into the rack's
  grid (it does for other containers); verify by aging one bottle in and one out of a rack.
- Berry footprint in the 6x2 bottle grid is not in code (sprite shape); the ingredient count per
  bottle may be capped below 2–3 by space.
- The `dumpingGroundTG` table contents are Unity data; use `loot-list` in the F8 console.
- §4 rows outside the furnace, rack and hydroponic come from one survey pass and were spot-checked
  only (deal maker has no source; moisture-farm volume).

## 9. Item use catalogue (sale value vs use value)

"Sold as" = unitValue and main types (what a customer pays before features, markup and events).
"Consumed?" = destroyed by the use. Water-treatment items are listed for completeness; their
effect on water is in `WATER.md`.

| Item | Sold as | Used for | Consumed? | Effect |
|---|---|---|---|---|
| `bloomberry_seed` | 40, SEED | hydroponic seed | **No** (perennial; only a hatchet removes it) | 8 berries every 2 nights forever after a 3-night start: ~28 berries/week. Never sell a working seed |
| `nutrifruit_seed` | 40 (8 / 4 once freed), SEED | hydroponic seed | Yes, returns 3 seeds | 5+ nutrifruit (sizes by health) per 3 nights |
| `kotton_seed` / `dream_cap_seed` / `fillerweed_seed` / `hydroreed_seed` / `coffee_bean` | 20 / 24 / 9 / 20 / 28 | hydroponic seed | Yes, return 1–2 seeds (coffee: none) | §1 plant table |
| `wine_berry` (any grade) | 14, FOOD (same price for every grade) | wine ingredient | Yes | +2 best, +1 normal, 0 under/over-ripe, −1 moldy to the wine score; one berry in a bottle with yeast turns 14 into 85–125 |
| `wine_bottle` | 12, HOUSEHOLD_GOOD | fermenter; becomes the wine | Yes (it is the product; reset on drink/pour) | Without it no wine; the binding constraint of the wine chain |
| `wine_yeast` | 10, HOUSEHOLD_GOOD | fermentation | Yes | +1 score (one grade) and 2-night ferment instead of 4 |
| `wine_superyeast` | 20 | fermentation | Yes | +1 score, 1-night ferment |
| `wine_yeast_infinite` (bioreactor) | 250 | fermentation | **No** (returned to stock) | +1 score, 2-night ferment, every batch: pays for itself after ~25 bottles vs `wine_yeast` |
| `wine_yeast_red` | 8, contraband 1 | fermentation | Yes | Makes counterfeit gloomberry wine (44, contraband 2, endangerment, no aging) regardless of berries |
| `water_ration` / `water_ration_small` | WATER_RATION | fills a bottle | Yes | 750 / 375 ml at a fixed grade; water grade moves the wine score −3…+1 (`WATER.md`) |
| finished wine | 100 × grade (85–175), SUBSTANCE/ALCOHOL (+LUXURY at Reserve+) | drink (alcohol), pour (750 ml alcohol/water mix) | on drink/pour | +3 base value per night held; event-proof |
| `scrap_metal` | 10, MATERIAL/JUNK | furnace input | Yes | 2 → 1 ingot (Low, 42.5; Fair 50 with flux) |
| `common_ore` | 15, MATERIAL | furnace input | Yes | Each ore in the pair lifts purity one level (2 ore + flux = Very high, 75) |
| `junk` | 3, MATERIAL/JUNK | furnace input with the junk module | Yes | 6 → 4 scrap (18 → 40) on nights with no smelt |
| `flux_agent` | 4, MATERIAL | furnace | Yes, when it acts | +1 purity level (Low → Fair: +7.5 per ingot) |
| `advanced_flux_agent` | 300, MATERIAL | furnace | **No** | Same +1 level every night forever: worth it after ~75 smelts vs 4-credit flux |
| `metal_ingot` | 50 × purity (37.5–150), MATERIAL | 3D printer input | Yes | 1 ingot → 2–5 gun parts by purity |
| `furnace_module_blast` | 150, module | furnace module | No | 3 inputs → 2 ingots per night (doubles throughput) |
| `furnace_module_junk` | 120, module | furnace module | No | Enables junk → scrap |
| hydroponic / furnace / other modules | module value | machine module bay | No (locked while working) | Performance → speed; quality → yield (hydroponic), flux level (furnace), extra aging steps (rack, +1 per 125 %) |
| `energy_credit` (any charge) | ~1 per charge unit + base | machine battery | Charge drawn | Hydroponic 8, furnace 8, rack 24, purifier 4, moisture farm 6, printer 8 per night |
| `turbo_booster_adv` | 300 | extra machine cycle | No (3-night recharge) | One more night of work for one machine |
| `plant_analyser` | 200, TOOL | reads seed genetics | No | Information only |
| `water_filter` / `water_filter_adv` / `hydroreed_filter` | 25 / 350 / 20, HOUSEHOLD_GOOD + FILTER + WATER_PURIFICATION_SUPPLY | water treatment | adv 400 and hydroreed 200 durability | Cleans water for wine (+score), hydroponics (plant health → berry grade) and fish; see `WATER.md`. Selling one gives 20–350 once; keeping it raises the grade of every bottle and harvest |
| `metal_scanner` | tool | scavenging | No | +15 % double-loot chance per search (+5 per upgrade, max 100) |
| `flashlight` | tool | scavenging | No | Halves both wound chances; also reveals Low/Very-low ingots |
| `screwdriver` / `wire_cutter` / `welder` | tools | scavenging | No | +10 % per search for nuts / wire / scrap (welder feeds the furnace) |
| `labeler` | 50, tool | relabel conditions | No | Raises price until exposed; FRAUD crime; cannot touch wine or ingot grades |
