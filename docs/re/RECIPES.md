# Recipes and item transformations (RE notes)

Build 26525552 / 0.46D. Same conventions as `STATE.md` / `PRODUCTION.md` / `WATER.md`:
**VISIBLE** = rendered by the UI (evidence = the UI code), **HIDDEN** = internal only,
**DERIVABLE** = computable from visible facts. Types are in the global namespace of
`Assembly-CSharp`. Everything is read from code, not play-tested. "(inferred)" marks a conclusion
drawn from the absence of code, and "(computed)" marks arithmetic done here from code numbers.

This file covers every **non-chemistry** way one item becomes another: machine recipes, item-on-item
combinations, unpacking, dismantling and the animal and fish chains. Other docs already cover
some of these, so this file only links to them:

| Topic | Where |
|---|---|
| Hydroponics, wine (fermentation, aging, rack), furnace and ingot purity, market events | [`PRODUCTION.md`](PRODUCTION.md) §1–3, §5 |
| Water treatment (filters, tablets, UV, portable purifier, purifier, evaporator, moisture farm, hydroreed) and the RAS water chemistry | [`WATER.md`](WATER.md) |
| Blender, heating plate, evaporator recipes, chem finisher, drugs | [`CHEMISTRY.md`](CHEMISTRY.md) |
| Labeler (relabelling a condition is a sales trick, not a recipe) | [`PRODUCTION.md`](PRODUCTION.md) §4 |
| Cigarette, stamp, injector, ribwich, plasma-fuel and hand guides | These are **appraisal and inspection references, not crafts**. See [`INSPECT.md`](INSPECT.md) and [`BARTER.md`](BARTER.md). A ribwich is an appraisal collectible valued by `RibwichHelper.GetCorrectValue(GameItem)` (value table: `local/appraisal_guides.json`, not committed) |
| Vending machine and fountain | A night **sales channel**, not a recipe: [`DAY.md`](DAY.md), [`PERKS.md`](PERKS.md), [`WATER.md`](WATER.md) §2.2 |
| Makeshift reactor (nuclear waste byproduct) | [`MITIGATIONS.md`](MITIGATIONS.md) |

Values are `unitValue` from the item factories (`*Directory.cs`) before features, markup and
events. "Margin" = output value − input value − power, with power valued at ~1 credit per unit
(`PRODUCTION.md` §0). Condition features change the price as **innate %** (`GameItem.GetCurrentValue(...)`).
Equipment conditions: New +25, Excellent +12, Good 0, Fair −12, Poor −25 (`ItemConditionList`).

## 0. The short version

- The game has **no general crafting bench in this build**. `CraftingRecipe`, `CraftingRecipeManager`,
  `VendingRecipeManager` and `CraftingHelper.InitCraftingTable/InitVendingTable` are used only by
  unreleased content (§9). Every real recipe is one of these:
  1. **Chip-driven progress machines**: the 3D printer (§1) and the desequencer (§2). The recipe
     lives as tags on the chip item, so it is data.
  2. **Button machines**: the bottle printer (§3).
  3. **Item-on-item targeting** (`GameItem.onThisTargetItemFunc` / `onItemTargetThisFunc`): combine,
     dismantle, unpack and upgrade (§4–§6). These recipes are hard-coded in lambdas, not data.
  4. **Livestock**: RAS fish, rats, feed dispenser, auto-butcher (§7).
  5. **The fridge**, which adds a value feature instead of making a new item (§8).
- The best non-chemistry margins are **3D-printed melee weapons** from cheap metal (§1.4). The
  combat machete chip turns 3 Fair ingots (150) into 3 machetes worth ~456 for 60 power, over 5
  nights. Ingot purity barely matters for melee prints.
- Most **material-on-machine upgrades add the material's full value to the machine**
  (`machine.unitValue += upgradeItem.unitValue`). The credits are stored in the machine, not
  lost, and the Tinkerer perk refunds the material 20 % of the time (§6).

## 1. 3D printer (`printer`, `MachinePrinter`)

### 1.1 Machine

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | `printer`, unitValue 350, `STANDARD_MACHINE_TAG` (turbo-boostable) | VISIBLE | `MachinePrinter.Printer()` |
| Panels | battery, module bay (`MODULE_TYPE_UNIVERSAL`, `MODULE_TYPE_PRINTER`), input grid 9×6 (owned, no containers, no turbo boosters), chip slot (owned `PRINTER_CHIP_TAG`), output grid 9×5, note | VISIBLE | `CreateMachineInventoryWindow(...)` |
| Timing | **Per night** (main window). In `STATE_READY` with a chip whose inputs are present and whose machine tags match, it consumes the inputs, locks the chip, draws power and adds one night of progress at once. In `STATE_WORKING` it draws power and adds progress. When finished, it outputs and unlocks the chip | progress VISIBLE (default tooltip) | `Printer()` cycle lambda, `MachineProgressHelper.ContinueProgressTypeMachine(GameItem)` |
| Speed / power | 24 progress per night (`InitProgressTypeMachine(…, 24)`), 8 power per **working** night. Nights = ceil(work / speed) | DERIVABLE | `InitMachinery(machine, 8)` |
| Chip | **Never consumed**: one chip prints forever | VISIBLE | `FinishPrinting(...)` unlocks it |
| Quality | `MACHINE_QUALITY_RATIO_INT` 4: adjusted quality = module quality % / 4 (`ModuleHelper.GetAjustedQuality(GameItem)`) | HIDDEN (module tooltip shows %) | |
| Input matching quirk | Inputs are counted per **item object**, not per stack unit, and each matched item is destroyed whole (`MachineProgressHelper.HasRequiredAdditionalItems`, `GetItemsToConsume`). The printer materials do not stack (inferred: no `SetStack` in their factories), so this does not bite in practice | | |
| Obtained | `RETIRED_GUNSMITH` network upgrade (650). The `retired_gunsmith` client sells a prebuilt printer, a screwdriver, plastic and chips (§1.3) | | `StoreClientListSpec.CreateRetiredGunsmith()` |

### 1.2 Printer modules (`ModuleDirectory`)

| Module (value) | Performance / efficiency / quality % | Effect | Base printer after it |
|---|---|---|---|
| `printer_module_metal` (300) | 0 / −50 / 0 | Sets machine tag `Metal Printing Module` (re-synced every night by `MachinePrinter.SyncModuleTags`) | speed 24, power 12 |
| `printer_module_cnc` (400) | −25 / −25 / 0 | Sets `CNC Module`; frame and gun-part chips need **both** tags | metal + CNC: speed 18, power 14 |
| `printer_module_melee_batch` (250) | −50 / −50 / 0 | +1 weapon per melee print **per module** (`CountMeleeBatchModules`) | metal + batch: speed 12, power 16 |

Speed = 24 × (1 + perf%/100). Power = 8 × (1 − eff%/100), with a minimum of 1
(`ModuleHelper.ApplyBasicModuleEffect(GameInventory, GameItem, GameItem)`).

### 1.3 Chip recipes

The chip tags `REQUIRED_ITEM_ID_n` / `REQUIRED_ITEM_COUNT_n`, `PROGRESS_ITEM_TARGET_TAG`,
`PROGRESS_ITEM_TARGET_ITEM_ID_TAG` and `PROGRESS_ITEM_TARGET_ITEM_COUNT_TAG` are set by
`MachineProgressHelper.SetSingle/SetDualAdditionalRequiredItem` and
`InitProgressSourceItem(GameItem, int, string, bool, int, string)`. They are VISIBLE in the chip
tooltip (`AddAdditionalItemRequirementsTooltip`). Inputs: `printer_plastic` 12, `scrap_metal` 10,
`common_electronic` 33, `metal_ingot` 50 (Fair; 37.5–150 by purity, `PRODUCTION.md` §3). Nights and
power assume the minimum module set the chip needs.

| Chip (value) | Inputs | Output ×n (value each) | Work | Needs | Nights / power | Margin per run (computed) |
|---|---|---|---|---|---|---|
| `printer_chip_small_bottle` (100) | 1 plastic | `small_bottled_water` ×6 (4) | 10 | — | 1 / 8 | +4 |
| `printer_chip_internal_springs_improved` (150) | 2 scrap | `internal_springs_improved` ×4 (20) | 20 | — | 1 / 8 | **+52** |
| `printer_chip_makeshift_rds` (200) | 1 plastic + 1 electronic | `rds_makeshift` ×4 (25) | 20 | — | 1 / 8 | +47 |
| `printer_chip_makeshift_toy_laser` (200) | 1 plastic + 1 electronic | `laser_toy` ×4 (18) | 20 | — | 1 / 8 | +19 |
| `printer_chip_rds` (300) | 3 scrap + 1 electronic | `rds` ×2 (60) | 24 | — | 1 / 8 | +49 |
| `printer_chip_laser_sight` (300) | 2 scrap + 1 electronic | `laser` ×2 (50) | 24 | — | 1 / 8 | +39 |
| `printer_chip_silencer` (300) | 5 scrap | `silencer` ×2 (55) | 24 | — | 1 / 8 | **+52** |
| `printer_chip_22_mag` (250) | 1 plastic | `mag_22` ×2 (15) | 24 | — | 1 / 8 | +10 |
| `printer_chip_22_mag_high` (300) | 2 plastic | `mag_22_high` ×2 (25) | 36 | — | 2 / 16 | +10 |
| `printer_chip_22_mag_ext` (350) | 3 plastic | `mag_22_ext` ×2 (35) | 48 | — | 2 / 16 | +18 |
| `printer_chip_10_mag` (300) | 3 scrap | `mag_10mm` ×2 (30) | 36 | metal | 2 / 24 | +6 |
| `printer_chip_10_mag_high` (350) | 2 scrap | `mag_10mm_high` ×1 (40) | 48 | metal | 2 / 24 | −4 |
| `printer_chip_10_mag_ext` (400) | 3 scrap | `mag_10mm_ext` ×1 (50) | 60 | metal | 3 / 36 | −16 |
| `printer_chip_pistol` (250) | 2 plastic | `printed_gun` ×1 (40; New +25 → 50; contraband 1) | 50 | — | 3 / 24 | +2 |
| `printer_chip_pistol_double` (400) | 3 plastic | `printed_gun` ×2 (50) | 60 | — | 3 / 24 | +40 |
| `printer_chip_box_cutter` (150) | 2 scrap + 1 plastic | `box_cutter` ×3 (28) | 34 | — | 2 / 16 | +21 |
| `printer_chip_kitchen_knife` (200) | 3 scrap + 1 plastic | `kitchen_knife` ×4 (45) | 64 | — | 3 / 24 | **+82** |
| `printer_chip_hatchet` (250) | 4 scrap + 1 plastic | `hatchet` ×4 (55) | 75 | — | 4 / 32 | **+97** |
| `printer_chip_kitchen_cleaver` (300) | 5 scrap + 2 plastic | `kitchen_cleaver` ×4 (65) | 82 | — | 4 / 32 | **+107** |
| `printer_chip_combat_knife` (380) | 2 ingots | `combat_knife` ×5 (75) | 98 | metal | 5 / 60 | **+148** (Fair ingots) |
| `printer_chip_combat_machete` (480) | 3 ingots | `combat_machete` ×3 (185) | 120 | metal | 5 / 60 | **+246** (Fair ingots) |
| `printer_chip_gun_part` (300) | 1 ingot | `gun_part` ×2–5 by purity (25) | 30 | metal + CNC | 2 / 28 | −3 (Fair), +4.5 (Low) |
| `printer_chip_handmade_pistol_frame` (350) | 1 ingot | `handmade_pistol_frame` ×1 (70, contraband 1) | 60 | metal + CNC | 4 / 56 | see §1.5 |
| `printer_chip_heavy_handmade_pistol_frame` (450) | 1 ingot + 1 scrap | `heavy_handmade_pistol_frame` ×1 (100, contraband 2) | 70 | metal + CNC | 4 / 56 | see §1.5 |

Melee margins use the expected condition mix of §1.4 (×0.821 of list value). The margins leave
out the chip's purchase price. Chips are kept, so the chip is capital: the machete chip (480) pays
back in two runs.

**Where chips come from** (`StoreClientListSpec.CreateRetiredGunsmith()`): small bottle,
makeshift RDS, toy laser, pistol, springs, box cutter and kitchen knife are **always** offered.
RDS, laser sight, silencer, hatchet, cleaver and double pistol are offered on rolls (40 % each
from day 20, 15 % before; double pistol 25 %). Metal module 33 %, melee batch 25 %, CNC 25 %, combat
knife 20 %, machete 15 %, frame 20 %, heavy frame 15 % and gun part 25 % get **+15/+10 points per
gun permit tier** (II and III). The .22 and 10 mm mag chips are on flat 15–33 % rolls. A shabby
pistol chip and 2 plastic come with the gunsmith network upgrade (`NetworkUpgradeList`).

### 1.4 What sets output quality

| Output kind | Rule | Vis. | Evidence |
|---|---|---|---|
| Melee (`PRINTER_MELEE_CHIP_TAG`) | Each weapon rolls score = uniform 20–30 + adjusted quality + **average ingot bonus** of the consumed ingots (Very low −30, Low −20, Fair 0, High +20, Very high +30, Perfect +100; 0 when no ingots are used). Score < 25 **Poor**, < 60 **Fair**, < 90 **Good**, else **Excellent** | condition VISIBLE on the item; the roll is HIDDEN | `ApplyPrintedMeleeCondition(GameItem, GameItem)`, `CaptureIngotQuality(...)` |
| Gun-part count | Ingot purity: Very low 2, Low/Fair 3, High/Very high 4, Perfect 5 | VISIBLE (count) | `GUN_PART_COUNT_BY_PURITY` |
| Frames (`GUN_FRAME_TAG`) | Stores print quality = adjusted quality + ingot bonus in `GUN_FRAME_PRINT_QUALITY_INT`. The finished gun's firepower, style, reliability, recoil and ergonomics are scaled by (100 + quality)% | HIDDEN | `HandlePrintedItemQuality(...)`, `GunHelper.MultiplyBaseStat(GameItem, int)` |
| `ADV_GUN_TAG` outputs (printed gun) | stats ×(100 + adjusted quality)% | HIDDEN | same |
| Everything else | none | | |

Melee with scrap or plastic chips and no quality module: P(Poor) = 5/11, P(Fair) = 6/11, so expected
value = list × 0.821 (computed). **Ingot purity barely pays for melee.** With Fair ingots the
machete run nets +246, with Very high (+30, 75 each) +209, with Low (−20, all Poor) +229 and with
Very low +244 (computed). Good needs score ≥ 60, i.e. a +30…+40 bonus. Excellent (+12 %) needs
Perfect ingots (150 each), which cost far more than the 12 % they add. Quality modules
(÷4) are the cheap way to move the roll.

Melee batch modules: the machete with metal + one batch module makes 4 per run, takes 10 nights
and draws 16 per night: +297 per run but +30 per night, against +49 per night without the module
(computed). **The batch module lowers throughput per night**; it only saves input material.

### 1.5 Gun frames → guns (item-on-item)

`gun_part` targeted onto an owned `GUN_FRAME_TAG` frame → the frame's gun
(`GunFrameHelper.Finish(GameItem, GameItem)`, via `MaterialDirectory.GunPart()`). The part and the
frame are consumed. A printed frame (it carries a print quality) always yields condition **New**
(+25 %).

| Chain (computed, Fair ingot, part valued at 25) | Output value | Inputs + power | Margin |
|---|---|---|---|
| Ingot → `handmade_pistol_frame` + `gun_part` → `handmade_pistol` (120, contraband 1) | 150 | 50 + 25 + 56 | +19 |
| Ingot + scrap → `heavy_handmade_pistol_frame` + `gun_part` → `heavy_handmade_pistol` (160) | 200 | 60 + 25 + 56 | **+59** |

### 1.6 Disassembly with a screwdriver (`GunHelper.DisassembleWeapon(GameItem)`)

`screwdriver` (25, kept) on an owned `ADV_GUN_TAG` gun: it unloads the gun (`TryUnloadAll`),
destroys it and gives parts. Use it only when a gun's condition or features push its value below
the parts; at Poor (−25 %) every gun is still worth more whole (computed).

| Gun (value) | Parts (value) |
|---|---|
| `printed_gun` (40) | 2 plastic (24) |
| `handmade_pistol` (120) | 2 gun parts + 1 plastic + 2 scrap (82) |
| `heavy_handmade_pistol` (160) | 2 gun parts + 1 plastic + 4 scrap (102) |
| `glock_receiver` (200) | 4 gun parts (100) |
| `revolver` (240) | 5 gun parts (125) |
| `smg` (280) | 6 gun parts (150) |
| `shotgun` (310) | 7 gun parts (175) |

**Gun repair** with a gun part (`GunHelper.TryRepairGun(GameItem, GameItem)`, part consumed):
the success chance is the gun's base repairability × the number of attempts since the last success.
Base repairability: printed and handmade 100, revolver 60, heavy 80, glock 65, SMG 40, shotgun 35
(`GunHelper.InitGun(...)`). Success raises the condition one step: Poor → Fair → Good →
Excellent, never to New. A part (25) buys +12–13 % of the gun's value, so repair pays on every
gun above ~200 (computed).

## 2. Desequencer (`desequencer`, `MachineDesequencer`)

The row in `PRODUCTION.md` §4 has the summary; these are the recipe numbers. 350 credits,
**contraband 2**, 10 power per working night, speed 33. A crypto module (kept, contraband 1–4)
plus **any owned access card** in the input slot (consumed at the finish; use `blank_keycard`, 15)
gives one keycard (`FinishDesequencer(GameItem, GameSlotInventory, GameSlotInventory)`).

| Module (value, contraband) | Work → nights | Output (value) | Margin with a blank card (computed) |
|---|---|---|---|
| `crypto_module_ser` (200, 1) | 75 → 3 | `ser_keycard` (40) | −5 |
| `crypto_module_sup` (200, 1) | 75 → 3 | `sup_keycard` (40) | −5 |
| `crypto_module_med` (250, 2) | 100 → 4 | `med_keycard` (75) | +20 |
| `crypto_module_eng` (250, 2) | 100 → 4 | `eng_keycard` (75) | +20 |
| `crypto_module_sec` (300, 3) | 125 → 4 | `sec_keycard` (100) | +45 |
| `crypto_module_cmd` (350, 4) | 150 → 5 | `cmd_keycard` (125) | +60 |

The cards' value lies in access and requests, not in the sale price.

## 3. Bottle printer (`bottle_printer`, `MachineBottlePrinter`)

| Fact | Value | Evidence |
|---|---|---|
| Item | 100, battery slot, output grid 4×4 (+1 column per unlocked size) | `BottlePrinter()` |
| Recipes (button, **real time**, power only) | `small_bottled_water` (4) for 4 power; `bottled_water` (5) for 6 power after 3 upgrades; `large_bottled_water` (9) for 11 power after 9 upgrades | `SMALL/NORMAL/LARGE_PRINT_COST`, `IsNormalUnlocked`, `IsLargeUnlocked` |
| Upgrade | `common_electronic` (33) targeted onto it: +1 count (max 9), and **the printer's value rises by 33** | `UpgradePrinter(GameItem, GameItem)` |
| Margin | 0 / −1 / −2 per empty bottle. A bottle's value comes from filling it with water (`WATER.md`). The printer's small-bottle chip makes empties at ~0.7 power each, cheaper | (computed) |
| Obtained | Starting kit (Water Merchant start, shabby); Dr. Jackson with `WATER_TRADER` (`PRODUCTION.md` §4) | |

`scrap_metal` (10) targeted onto a `small_bottled_water` (4) → `silencer_makeshift` (15). Both
inputs are consumed: margin +1 (`MaterialDirectory.OnScrapMetalTargetItem(GameItem, GameItem)`).

## 4. Item-on-item recipes that make a new item

| Source → target | Output | Consumed | Value (in → out) | Evidence |
|---|---|---|---|---|
| `scrap_metal` → `small_bottled_water` | `silencer_makeshift` | both | 14 → 15 | `MaterialDirectory.OnScrapMetalTargetItem` |
| `gun_part` → printed frame | handmade / heavy pistol (New) | both | §1.5 | `GunFrameHelper.Finish` |
| `bandage_item` / `hemostatic_bandage_item` / `topical_bandage_item` / `tilapia_skin_graft` → `glass_shard` | `glass_shard_shiv` (8, melee) | **only the shard** (one unit); the wrap is not destroyed (inferred: no destroy call on the source) | 2 → 8 per shard, **+6** | `MiscItemDirectory.GlassShard()` (receiver-side `onItemTargetThisFunc`), `IsShivWrap(GameItem)` |
| `common_electronic` (33) → unstuck `system_module_corrupt` (45) | random module: `system_module_performance` 33 %, `system_module_efficiancy` 22.1 %, `system_module_quality` 44.9 % (all 70) | both | 78 → 70 (−8; the module is the point) | `ModuleHelper.RestoreModule(GameItem, GameItem)`, `CanElectronicPartTarget` |
| `common_electronic` → `module_extractor` | max uses −1, refill +4 | electronic | | `ModuleHelper.RepairExtractor` |
| `screwdriver` → `system_module_ruined` (30) | `common_electronic` + `printer_plastic` + `junk` (48) | the module | 30 → 48, **+18** | `ToolDirectory.CreateScrewdriver()` |
| `screwdriver` → ADV gun | parts | the gun | §1.6 | `GunHelper.DisassembleWeapon` |
| water ration → `wine_bottle` | fills the bottle | the ration | `PRODUCTION.md` §2 | `WineHelper.PourRationIntoBottle` |
| `surgery_tool` → live sturgeon | caviar / roe jars | fish health | §7.3 | `AquacultureHelper.StripEggs(GameItem)` |
| blade → fish / rat | carcass, then cuts | the animal | §7.2, §7.5 | `HusbandryHelper.InitBladeItem` |

## 5. Unpacking, dismantling and clearing

| Item | Action | Output | Value | Evidence |
|---|---|---|---|---|
| `common_chemical` (50) | Activate (owned). It asks first unless `RETIRED_CHEMIST` is unlocked | 2 `mini_bottle` alcohol mixes, a lipobond-agent bottle, an acid bottle, a base bottle | liquids: see `CHEMISTRY.md` | `MaterialDirectory.CommonChemicalSupplies()`, `PreBuildChemHelper` |
| `restricted_chemical` (100, contraband 2) | Activate | 2 precursor bottles, a reducing-agent bottle | `CHEMISTRY.md` | `RestrictedChemicalSupplies()` |
| `scratcher_stack` (100) | Activate | 5 `scratcher` (20), carrying the stack's spawn luck | 100 → 100 | `MiscItemDirectory.ScratcherTicketStack()` |
| `red_beer` (18) | Drink | `empty_beer_bottle` (5, junk) | | `FoodItemDirectory.RedBeer()` |
| Broken machine (`security_alarm`, `furnace`, `moisture_farm` from the scrapper) | `screwdriver`: 3 `nuts_metal` (14) and the machine loses 33 % of base value. `wire_cutter`: 3 `wire` (16), −33 %. `welder`: 4 `scrap_metal` and the machine is **destroyed**. Tools are kept | | Broken furnace: 42 − 66, 48 − 66, then 40 for the remaining ~68: **dismantling loses value**; the scrap is the only reason | `MachineBrokenHelper.OnScrewdriverUsed/OnWireCutterUsed/OnWelderUsed`; no repair path found (inferred) |
| Hoarder debris, tool pile (`debris_pile2`) | `crowbar` / `wire_cutter` / `welder` targeted once each: **the tool is destroyed** and gives 3 `common_electronic` / 3 `wire` / 3 `scrap_metal`. After all three, the pile clears into 5 dump rolls + 5 non-junk dump rolls | | crowbar 75 → 99 (+24), cutter 40 → 48 (+8), welder 80 → 30 (−50) | `DebrisHelper.ClearWithTool`, `TOOL_REWARDS` |
| Hoarder rest pile (`debris_pile1`) | Each night with no after-hours outing: +1 scrap + 1 dump roll to the back counter; clears after 10 nights (+10 rolls) | | | `DebrisHelper.OnNight()` |
| Hoarder service pile (`debris_pile3`) | While the debris-removal service is active: 5 dump rolls per night; clears after 10 nights | | fee via `StoreService.UpdateCost()` | same |

The dump rolls use `dumpingGroundTG` (`PRODUCTION.md` §6). The piles come only from the Hoarder
start (`StartingPerkContent.HandleNewGamePerkBeforeItems()`, see `PERKS.md`).

## 6. Material upgrades (target a machine or tool with a material)

None of these makes an item. They turn a material into machine capacity, and most **add the
material's value to the machine's `unitValue`**, so selling the machine recovers it.
`StartingPerkContent.ConsumeUpgradeMaterial(GameItem)` keeps the material 20 % of the time with
the Tinkerer perk.

| Material → target | Effect per use (max) | Value kept? | Evidence |
|---|---|---|---|
| `common_electronic` → `bottle_printer` | +1 upgrade (9): normal at 3, large at 9 | yes | `MachineBottlePrinter.UpgradePrinter` |
| `common_electronic` → `fridge` | power −2 per night (7; floor 2) | yes | `MachineFridge.UpgradePowerEfficiency` |
| `scrap_metal` → `fridge` | +1 row to both zones, power +2 (7) | yes | `MachineFridge.UpgradeStorage` |
| `scrap_metal` / `metal_ingot` → `ras` | width (scrap, 4) / height (ingot, 4) +1 cell, power +2 / +3 | yes | `MachineRAS.UpgradeWidth/UpgradeHeight` |
| `common_electronic` → `ras` | power −2 (12) | yes | `MachineRAS.UpgradePowerEfficiency` |
| `scrap_metal` / `metal_ingot` → `water_tank` | +2 L (scrap) or +4–14 L by ingot purity (6 L default), cap 50 L | yes | `WaterTankHelper.Upgrade`, `INGOT_ML_BY_LEVEL` |
| `metal_ingot` → `animal_cage` | +1 height (8), +20 max health | (+40 value per the cage code) | `AnimalCage.UpgradeSize` |
| `scrap_metal` → `animal_cage` | repair 50 % of max health; scrap destroyed | no | `AnimalCage.Repair` |
| `scrap_metal` → `toolbox` | +1 unlocked slot (3 → 9) | yes | `ToolboxHelper` receiver lambda |
| `metal_ingot` → vending machine core | +capacity (10 upgrades, +1 power each) | no | `VendingMachineHelper.UpgradeCapacity` |
| `common_electronic` → vending machine/fountain core | +5 attractiveness (17 upgrades, cap 100) | no | `UpgradeAttractiveness` |
| `common_electronic` → `metal_scanner` | double-loot chance (`PRODUCTION.md` §6) | | `ScavHelper.UpgradeScavScanner` |
| `common_electronic` → `vacuum_robot` | incinerator (once) | yes | `HusbandryHelper.UpgradeVacuumRobot` |
| `common_electronic` → current converter | rate −0.1 (5) | yes | `CurrentConverterHelper.UpgradeEfficiency` |
| `common_electronic` / `scrap_metal` / `junk` → makeshift reactor | upgrade / repair (`MITIGATIONS.md`) | no | `ReactorHelper` |
| `hypertech_electronic` (2500) → hypertech battery | output +10 | yes | `HypertechBatteryHelper.UpgradeOutput` |

## 7. Livestock: fish and rats

Water quality, waste and filters in the RAS are in `WATER.md` §2.4. This section covers the
production side. Feed prices per kcal (computed): `industrial_fish_feed` 5000 kcal at 100 = 0.02
per kcal; `gutterflow_pellet` 4000 kcal at 40 = 0.01; rat `raw_meat` 1200 kcal at 159 = 0.13;
`fish_scrap` 0.04 (its value is kcal × 4 / 100). Fillets and cuts are **fixed-price items**; there
is no per-gram pricing.

### 7.1 RAS tank (`ras`, `MachineRAS`, `AquacultureHelper.TickTank(...)`)

| Fact | Value | Evidence |
|---|---|---|
| Item | `ras` 300 (+ upgrade values), tank 8×4 → 12×8, water slot `ras_water_tank` 12 L (value 0) | `MachineRAS` |
| Power | 5 per night (+2 per width, +3 per height upgrade, −2 per efficiency upgrade, min 1). Drawn only with water and live stock | `UpdatePowerUsage`, `IsWorthRunning` |
| Window | main night pass; electric eels make power in the Early pass (1/2/3/3 per night at Juvenile/Adolescent/Adult/Elder, none while starving) | `AquacultureHelper.GeneratePower` |
| Breeding | Tilapia kinds only. A fed adult female (hunger ≥ 50 %) spawns 1 roe at 15 % per night. Roe beside a fed adult male is fertilised, hatches next night into a fry group of 6, and 2 growth-days later becomes 6 juveniles. Eels and sturgeon do not breed | `TryBreed`, `AgeOne`, `HatchRoe`, `GrowFry` |
| Growth | +1 age day per night if hunger ≥ 37.5 %. Speed = 1 − 0.25 × industrial-feed share + 0.25 × meat share (tilapia only). Pure industrial feed is ×0.75 | `GetGrowthSpeed` |
| Hunger | max(5, upkeep + grown grams × 1.2 kcal) per night; upkeep = weight/100 × rate (halved for Elders) | `HandleHunger` |
| Feeding order | dispenser (to its target %, default 85), carcasses, loose pellets/feed, any edible item, then **predation** of live stock ≤ 20 % of the eater's weight | `TryFeed`, `TryPredation` |
| Death | 2 nights at hunger 0; 50 % per night past old age; any fish outside a tank at night | `HandleStarvation` |

| Species (`FishSpecies`) | Stage start days (Roe…Elder) | Weights g (Juv…Elder) | Old age | Upkeep kcal/100 g | Conversion | Diet |
|---|---|---|---|---|---|---|
| Tilapia | 0,1,3,5,8,13 | 50, 200, 450, 600 | 27 | 2 | 0.9 | omnivore |
| Gutter tilapia | 0,1,3,6,10,16 | same | 33 | 2 | 0.85 | omnivore; ignores waste, pellet-taint immune, cleans water |
| Graft tilapia | as tilapia | same | 27 | 2 | 0.7 | omnivore; skin grafts, medical roe |
| Eel | 0,0,0,5,10,18 | 150, 600, 1400, 2400 | 36 | 3 | 0.9 | carnivore |
| Electric eel | as eel | same | 36 | 5.25 | 0.75 | carnivore; makes power |
| Sturgeon (female) | 0,0,0,10,18,34 | 500, 1500, 3500, 5000 | 80 | 2 | 0.9 | carnivore; eggs |

Carnivores cannot use pellets, industrial feed or "other" food, only meat.

### 7.2 Fish → fillets (`AquacultureHelper.CreateFilletPieces(GameItem)`)

Trigger: a blade (first use kills, second fillets), the auto-butcher, or the feed dispenser.
Meat kcal = floor((kcal eaten − egg grams × egg kcal/g) × conversion) (`GetMeatKcal`). A carcass
freezes it at death. Meat splits greedily (`AquacultureItems.BreakDownMeat`):

| Piece | kcal | Value |
|---|---|---|
| `large_fish_fillet` (luxury) | 900 | 143 |
| `fish_fillet` | 400 | 57 |
| `small_fish_fillet` | 200 | 26 |
| `tiny_fish_fillet` | 100 | 13 |
| `eel_fillet` (luxury) | 500 | 136 |
| `small_eel_fillet` (luxury) | 250 | 68 |
| remainder ≥ 25 kcal | `fish_scrap` | kcal × 4/100 |

Tilapia and sturgeon use the fish pieces; both eels use the eel pieces. Byproducts: graft tilapia
give floor(weight / 200) `tilapia_skin_graft` (24, medical, heals 2), and a gravid sturgeon gives
floor(egg g / 100) caviar. **Pellet taint**: if > 25 % of lifetime kcal came from pellets
(gutter tilapia exempt), every fillet and egg jar becomes trash grade (−75, not food)
(`IsPelletTainted`, `GutterflowPelletHelper.ApplyTrashGrade`).

| Fully fed yield (`GetExpectedKcalEatenAtAge`) | Pieces | Value | Feed (computed, approx.) |
|---|---|---|---|
| Tilapia day 8 (511 kcal) | fillet + tiny | 70 | ~600 kcal industrial ≈ 12 |
| Tilapia day 13 (716) | fillet + small + tiny | 96 | ~900 kcal ≈ 18 |
| Graft tilapia day 13 (557) | fillet + tiny + scrap, + 3 grafts | 70 + 72 | |
| Eel day 18 (3024) | 6 eel fillets | **816** | ~3800 kcal of **meat** |
| Sturgeon day 34 (6619) | 7 large + small + tiny | 1040, plus eggs | carnivore |

Bred tilapia cost only feed (≈ +60–80 each, computed, ignoring power), and a breeding pair compounds
at 6 fry per hatch. Eels are bought (juveniles 100) and must eat meat. At rat-meat prices (0.13
per kcal) the feed costs about as much as the fillets sell for (computed). They pay when fed fish scrap,
offal or tilapia (0.04 per kcal or self-bred).

### 7.3 Sturgeon eggs (`AquacultureHelper.StripEggs(GameItem)`)

From Adult, with health ≥ 135/150, eggs grow +40 g per aged day up to 15 % of stage weight (525 g
Adult, 750 g Elder). They cost 2.5 kcal/g, which is taken out of the meat. `surgery_tool` (200, kept)
strips floor(egg g / 100) jars of 100 g. A fish's jars 1–10 are `caviar` (240), jars 11–16
alternate caviar and `sturgeon_roe` (120), and later jars are all roe (`GetSurgeryYield`). Each
strip costs 30–75 health. A full Adult load is 5 jars, **1200 credits** of caviar per strip
(computed). This is the most valuable livestock output.

### 7.4 Feed dispenser and feeds

- `livestock_feed_dispenser` (300, no power, 40 000 kcal). Its button or auto-blend kills and
  butchers or fillets any animals and fish inside, then turns every calorie item into kcal 1:1 in
  buckets: rat meat (tracks prion), other meat, other food, pellet, industrial.
  `MachineFeedDispenser.BlendContentsUnguarded`, `TryBlendItem`. Draws are proportional across the
  buckets an animal may eat.
- `industrial_fish_feed` (5000 kcal, 100) and `gutterflow_pellet` (4000 kcal, 40) are **never
  crafted**, only bought or granted. They split in half (≥ 1000 kcal) and merge up to their max
  (`IndustrialFishFeedHelper`, `GutterflowPelletHelper.Merge`).
- Roe as food: `tilapia_roe` 30, `gutter_tilapia_roe` 15, `graft_tilapia_roe` 60 (medical, heals 3);
  60 kcal each (`AquacultureItems.Roe`).

### 7.5 Rats (`HusbandryHelper`, `AnimalSpecies.Rat`, `animal_cage`)

| Fact | Value | Evidence |
|---|---|---|
| Cage | `animal_cage` 250, no power, 10×3. Wear per night = live size / 4 (×2 overcrowded); escape chance 5–25 % below 20 health | `AnimalCage` |
| Growth | stage days 0,1,3,5,11,14 → 5, 50, 200, 350, 450, 400 g; per night Δweight/Δdays × growth gene, reduced up to 50 % when hunger < 400; hunger 65/night | `GetAnimalCurrentWeightChange`, `GrowAnimal` |
| Breeding | mature at day 3; mating chance = the lower fertility (base 50 %); litter 3 ± 1; gestation 3 nights | `CheckMate`, `GiveBirth` |
| Cannibalism | eating rat meat: prion at (100 − immunity) % (certain if the meat is prion-tagged); prion −20 % growth, immunity, longevity | `CheckCannibalism` |
| Feces | `feces_small/medium/large` (0 value; organic waste, debris, microbes) when a night's growth ≥ 30 g | `Poop` |
| Value | a live rat's `unitValue` is reset to 0 every night: **sell meat, not rats** (the rancher sells age-5 rats at 120) | `OnAnimalItemDayEnd` |
| Butchering | weight (×0.75 if diseased) split greedily: `raw_meat` per 400 g (159), `small_raw_meat` 200 g (65), `morsel` 100 g (30), `small_morsel` 50 g (15), any remainder +1 small morsel. A 450 g adult → raw meat + small morsel = **174** | `HusbandryHelper.CreateButcherCuts(GameItem)` |

### 7.6 Auto-butcher (`auto_butcher`, `MachineAutoButcher.Run(GameItem, bool)`)

400 credits, **no power**, 10×10 tray. Runs in the **Late** night pass, or by button, by dropping
a creature on it, or by "butcher content". It sits in a cage or RAS slot. Modes: off (the
default, so it does nothing out of the box), carcasses, weight, age. Defaults: cage 400–1000 g or
10–30 days; tank 600–5000 g or 20–100 days. It always keeps the 2 youngest females and the
youngest male as breeders. Cage options: spare pregnant, cull sick, cull prion. Tank options: spare egg-layers,
fillet carcasses, optional cuts-to-dispenser and roe collection. A creature is skipped if the
tray cannot hold all its pieces.

### 7.7 How livestock is obtained

`AQUACULTURE` (400, needs `WATER_TRADER`): RAS + tank, fertilised tilapia roe, 2 industrial feed,
knife, marking pen. It also queues `retired_aquafarmer`, who sells the RAS, dispenser, auto-butcher,
cleaver, filter, pellets, feed, and roe (tilapia ×2, gutter, graft). `AQUACULTURE_II` (1200): an eel
pair, and the aquafarmer adds 3 random big fish. `RETIRED_RANCHER` (750): cage + dispenser, 6
rats, cleaver, labeler, marking pen; the `retired_rancher` client sells cages, dispensers, the
auto-butcher, the vacuum robot (250), serums and rats. The Rancher and Aquafarmer starts begin
with the same kit (`NewGameData`).

## 8. Fridge (`fridge`, `MachineFridge`)

The fridge adds an innate % feature to items; it makes no new item.

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | 100, `TURBO_BOOST_ELIGIBLE_TAG`, chill zone 5×2 and freeze zone 3×2 (+1 row each per scrap upgrade) | VISIBLE | `Fridge()`, `ApplyStorageSize` |
| Accepts | owned liquid containers, BREVAGE and WATER_RATION items, `bottled_water_premium`, `beis_icecream`, `nudka`; never machines, storage or `wine_bottle` | VISIBLE | `MayZoneAcceptItem(GameItem)` |
| Power | 2 per night (+2 per storage upgrade, −2 per efficiency upgrade, floor 2), drawn only if not empty | DERIVABLE | `UpdateFridgePowerUsage` |
| Night effect | Chillables in **either** zone get `chilled` (+20 %). `nudka` in the chill zone gets chilled (+20 %), in the freeze zone `iceCold` (+25 %). `beis_icecream` in the freeze zone gets `frozen` (+25 %) | VISIBLE (feature line) | `ChillItem`, `IceColdItem`, `FreezeItem` |
| Expiry | Markers are cleared before the night pass and checked after it (`PlayerStore.EndNight`). An item that was not re-cooled that night loses its feature. **Take it out and it loses the bonus next morning** (inferred from `HandleColdFeatureExpiry`) | | `ClearColdMarkers`, `HandleColdFeatureExpiry` |
| Real time | Liquid containers move toward 3 °C (chill, 0.5 °/s) or −18 °C (freeze, 1 °/s) while powered | | `CoolZoneContainers` |
| Obtained | Dr. Jackson (inventor): always offered until owned, then 25 % | | `StoreClientList.CreateInventor()` |

Per-night value (computed, 2 power): `bottled_water_premium` 325 → +65, `nudka` 85 → +21, ice cream
40 → +10, `water_ration` 30 → +6, drinks 14–18 → +3. A fridge pays only for premium water and spirits,
or when it is full.

## 9. Unreleased or unobtainable (one line each)

- Generic crafting tables: `UnusedDirectory.CreateCraftingTable()` and `CreateTestVendingMachine()`
  (the only users of `CraftingRecipe` / `VendingRecipeManager`) are not registered as items.
- `UnusedDirectory.AutomaticMeatProcessor()`, `AutomaticMeatProcessorExample()`,
  `CreateRecycler()`, and `MachineMeatCulture.AutomaticMeatCultureLab()` have no caller.
- `box_dispenser` (`MachineBoxDispenser`) is registered but has no seller and dispenses
  `ammo_box`, which exists only in `UnusedDirectory`. Ammo box pack and unpack
  (`AmmoBoxHelper.Package/Unpackage`) are likewise unreleased.
- Salvage (`SalvageHelper`; only a wall in `UnusedDirectory`), cup-noodle cooking
  (`FoodItemHelper.CanCookCupNoodle`, no caller), lootboxes, pill bottles and pouches in
  `UnusedDirectory`, and ship hull upgrades (`Base`).

## 10. Extraction hooks

The existing headless extractor (`sim/extract/`, `Extractor.Recipes`) already runs the game's item
factories under the Unity stubs. Every factory calls `SetSpriteAndShape` →
`RenderHandler.LoadFromAtlas`, plus `LocHelper` and `AudioHelper`, so **nothing here can be built
without that stub harness**. Under it, every item below builds except those in
`extract_report.json`. Members marked *private* need reflection.

| Recipe table | Member that builds or holds it | Returns | Headless? |
|---|---|---|---|
| Printer and desequencer recipes | each chip or crypto item from `DirectoryMaster.Item(id)` (`ModuleDirectory` factories, incl. `CreateMeleeChip`, `CreateCncChip`) | tags `PROGRESS_ITEM_TARGET_TAG`, `…_TARGET_ITEM_ID_TAG`, `…_TARGET_ITEM_COUNT_TAG`, `REQUIRED_ITEM_ID_n/COUNT_n`, `PROGRESS_ITEM_REQUIRED_MACHINE_TAG[_n]`, `PRINTER_MELEE_CHIP_TAG`, `PRINTER_FRAME_CHIP_TAG`, `PRINTER_GUN_PART_CHIP_TAG`, `DESEQUENCER_TARGET_ID_TAG` | yes, factory under stubs; **already emitted** as `progress_recipes` in `var/gamedata/recipes.json` (it does not yet record the melee/frame/gun-part chip-kind tags or `DESEQUENCER_TARGET_ID_TAG`) |
| Printer speed / power / quality ratio | `DirectoryMaster.Item("printer")` tags `BASE_PROCESSING_SPEED`, `machinery_drawn_power`, `MACHINE_QUALITY_RATIO_INT` | 24 / 8 / 4 | yes (already in `machines`) |
| Printer module effects | `printer_module_*` tags `BONUS_PERCENTAGE_PERFORMANCE_INT/EFFICIENCY_INT/QUALITY_INT` | module % | yes |
| Gun-part count by purity | `MachinePrinter.GUN_PART_COUNT_BY_PURITY` (*private static readonly Dictionary*) | purity id → parts | yes, plain data (reflection) |
| Ingot quality bonus | `MachinePrinter.INGOT_QUALITY_BONUS` (*private static readonly*) | purity id → bonus | yes, plain data |
| Melee condition thresholds | `MachinePrinter.MELEE_ROLL_MIN/MAX`, `MELEE_CONDITION_*_SCORE` (public const) | 20/30, 25/60/90 | yes, constants |
| Equipment-condition % | `ItemConditionList.CreateEquipementCondition*()` `modValue` | +25…−25 | yes; touches `LocHelper` only |
| Gun disassembly yields | `GunHelper.DisassembleWeapon(GameItem)` | hard-coded branches | **code only**: transcribe, or run it on a stub inventory |
| Gun repairability | gun factory tag `GUN_REPAIRBILITY_TAG` (`GunHelper.InitGun`) | per gun | yes |
| Frame → gun | frame tag `GUN_FRAME_GUN_ID_STRING` (`GunFrameHelper.InitFrame`) | gun id | yes |
| Bottle printer | `MachineBottlePrinter.SMALL/NORMAL/LARGE_PRINT_COST`, `NORMAL_UNLOCK_COST`, `LARGE_UNLOCK_COST` (const) | power, unlocks | yes, constants |
| Item-on-item recipes (§4, §5) | lambdas in `MaterialDirectory.OnScrapMetalTargetItem`, `MiscItemDirectory.GlassShard`, `ModuleHelper.RestoreModule`, `ToolDirectory.CreateScrewdriver`, `MaterialDirectory.CommonChemicalSupplies/RestrictedChemicalSupplies`, `ScratcherTicketStack`, `MachineBrokenHelper.On*Used` | none: no table | **code only**: a hand-maintained list; the extractor can verify ids and values |
| Debris tool rewards | `DebrisHelper.TOOL_REWARDS` (*private static readonly*) and consts `REST_NIGHTS`, `SERVICE_NIGHTS`, `CLEAR_PULLS*` | tool → material | yes, plain data |
| Upgrade caps | consts in `MachineFridge`, `MachineRAS`, `VendingMachineHelper`, `ToolboxHelper`, `ReactorHelper`, `CurrentConverterHelper`; `WaterTankHelper.INGOT_ML_BY_LEVEL` (*private*) | caps, steps | yes, constants and plain data |
| Fridge features | consts `CHILLED_VALUE_BONUS`, `FROZEN_VALUE_BONUS`, `ICE_COLD_VALUE_BONUS`; `IsChillable/IsFreezable/IsSpirit` | +20/+25/+25; id rules | constants yes; eligibility is code |
| Fish species | `FishSpecies.All` (`Tilapia`, `GutterTilapia`, `GraftTilapia`, `Eel`, `ElectricEel`, `Sturgeon` static fields) | stage days and weights, upkeep, conversion, diet, old age | yes, **plain data** (names reach `LocHelper` only inside methods) |
| Fillet breakdown | `AquacultureItems.TILAPIA_MEAT`, `EEL_MEAT`, `GetMeatKcal`/`GetMeatValue` | piece kcal → id/value | yes, plain data |
| Expected yield curve | `AquacultureHelper.GetExpectedKcalEatenAtAge`, `GetWeightAtAge`, `GetUpkeepKcal` | kcal at age | yes, pure math (`Mathf`) |
| Rat species and cuts | `AnimalSpecies.Rat` / `All`; `HusbandryHelper.CreateButcherCuts` (code) | stages, genes; cut sizes | species yes (plain data); cuts code only |
| Auto-butcher defaults | `MachineAutoButcher.CagePage`, `TankPage` (*private*) | slider ranges, toggles | yes, plain data |
| Generic crafting (unreleased) | `GameItem.recipeManager.availableRecipes` (`RecipeManager`) on a crafting-table item | `List<CraftingRecipe>` | the tables are never registered, so there is nothing to enumerate |
