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
  4. **Livestock**: RAS fish, rats, feed dispenser, auto-butcher (§7, [`AQUACULTURE.md`](AQUACULTURE.md), [`HUSBANDRY.md`](HUSBANDRY.md)).
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
| Item | `printer`, unitValue 350, not contraband, `STANDARD_MACHINE_TAG` (turbo booster and bay kit eligible) | VISIBLE | `MachinePrinter.Printer()` |
| Panels | battery, module bay (the standard 26-cell bay, `MODULE_TYPE_UNIVERSAL` + `MODULE_TYPE_PRINTER`, owned only), input grid 9×6 (owned, no containers, no turbo boosters), chip slot (owned `PRINTER_CHIP_TAG`), output grid 9×5 (owned, no containers), note slot | VISIBLE | `CreateMachineInventoryWindow(...)`, `RefuseTurboBoosters` |
| Night cycle | Nothing happens unless the battery can pay the full draw (`MachineHelper.CanPower`). In `STATE_READY`, if a chip is in the slot, its machine tags are present and the input grid holds its inputs: it records the ingot bonus, destroys the inputs, locks the chip and the module bay, draws power and adds one night of progress at once. In `STATE_WORKING` it draws power and adds progress. On the night progress reaches the target it outputs, unlocks chip and bay and returns to ready | progress VISIBLE (default tooltip) | `Printer()` cycle lambda, `MachineProgressHelper.ContinueProgressTypeMachine(GameItem)` |
| Restart | The chip **stays in the slot** after a print. If the input grid still holds another run's inputs, the next night starts the next run with no player action. The input grid holds 54 cells, so one load can cover many runs (inferred from the cycle; see §1.9 lab list) | DERIVABLE | same |
| Speed / power | 24 progress per night, 8 power per **working** night. Nights = ceil(work / speed). Overshoot is lost | DERIVABLE | `InitProgressTypeMachine(…, 24)`, `InitMachinery(machine, 8)`, `FinishProgressTypeMachine` |
| Speed 0 | Performance −100 or lower gives speed ≤ 0: the run starts, draws power every night and never finishes (computed: metal + two batch modules with nothing to offset them) | HIDDEN | `ModuleHelper.ApplyBasicModuleEffect` |
| Chip | **Never consumed**; locked only while a run is in progress | VISIBLE | `FinishPrinting(...)` unlocks it |
| Quality | Adjusted quality q = trunc(module quality % / 4) (`MACHINE_QUALITY_RATIO_INT` 4, re-set every night) | HIDDEN (module tooltip shows %) | `ModuleHelper.GetAjustedQuality(GameItem)` |
| Input matching | Counts **item objects** by id, not stack units, and destroys each matched item whole; it takes the matching items nearest the **end** of the grid's item list (the last ones added, inferred) | | `MachineProgressHelper.HasRequiredAdditionalItems`, `GetItemsToConsume` |
| Stolen inputs | The input grid checks only ownership, so bought stolen inputs are accepted; outputs are fresh items with no stolen flag (inferred: `DirectoryMaster.Item` makes new items) | HIDDEN | `ContainerHelper.AllowOnlyOwnedItemsExcludeContainers`, `FinishPrinting` |
| Overflow | Outputs that do not fit the output grid go to the back room, then stock, showcase, front counter; destroyed only if all are full | | `GraphUtils.EmporiumTryAdd` |

**How a player gets one** (prices before the hard ×1.5 network multiplier, `MITIGATIONS.md`):

| Source | What comes with it | Evidence |
|---|---|---|
| Gunsmith start (−450 cash, +50 rent) | `CreateOldGunPrinter`: shabby printer, 100-charge cell, shabby pistol chip, and two **stuck** square modules in the bay: an eco module (−16 / +32) and a glitched module (+15 / −15). Net P −1, E +17: speed 24, draw 7 (computed). Also 4 plastic, the firearm sign, mods and ammo. `RETIRED_GUNSMITH` is set unlocked | `NewGameData` (GUNSMITH branch), `PreBuiltItemHelper.CreateOldGunPrinter()` |
| `RETIRED_GUNSMITH` network upgrade (650; 975 on hard) | Shabby printer, 100-charge cell, shabby pistol chip, 2 plastic, a screwdriver, a gun part and a handful of gun mods to the back room; queues the retired gunsmith for the next day | `NetworkUpgradeList` (RETIRED_GUNSMITH action), `CreateRetiredGunsmithUnlock()` |
| Retired gunsmith client | Visits on every day with day % 7 = 4 (Gunsmith start or the upgrade). Every visit offers a printer (350), a screwdriver and 4 plastic; chips and modules below | `StoreClientManager` (day % 7 = 4 branch), `StoreClientListSpec.CreateRetiredGunsmith()` |

The retired gunsmith's goods carry `premiumBuy15` = `retiredSupplierMarkup`: +0 / +15 / +30 / +45 % on
cozy / normal / hard / challenge (`ItemFeatureList.PremiumBuy15`, `Difficulty`). A machete chip costs 624 on hard.

### 1.2 Printer modules (`ModuleDirectory`)

All three are 1×4 (`long_module1/2/3`) printer-type modules. A standard bay takes one long + four squares,
two longs + two squares, three longs + one square, or four longs, placed by hand; a bay kit adds room
(`MODULES.md` §1.1).

| Module (value) | P / E / Q % | Effect | Sold (retired gunsmith, per visit) |
|---|---|---|---|
| `printer_module_metal` (300) | 0 / −50 / 0 | Machine tag `Metal Printing Module` (re-synced every night by `SyncModuleTags`): needed by the 10 mm mag, combat knife, machete, frame and gun-part chips | 33 % + 15 per gun permit tier |
| `printer_module_cnc` (400) | −25 / −25 / 0 | Machine tag `CNC Module`; frame and gun-part chips need **both** tags | 25 % + 15 per tier |
| `printer_module_melee_batch` (250) | −50 / −50 / 0 | +1 weapon per melee print **per module**; the extra weapons roll condition like the rest | 25 % + 15 per tier |

"Per gun permit tier": +1 step each for `GUN_PERMIT_II` (600) and `GUN_PERMIT_III` (900), so up to +30
(`CreateRetiredGunsmith`, local `Chance`). Universal modules (`MODULES.md` §2.1) work too.

Speed = 24 + trunc(24 × P / 100). Draw = 8 − trunc(8 × E / 100), minimum 1
(`ModuleHelper.ApplyBasicModuleEffect`). With the printer modules alone (computed):

| Bay | P / E | Speed | Draw |
|---|---|---|---|
| bare | 0 / 0 | 24 | 8 |
| metal | 0 / −50 | 24 | 12 |
| metal + CNC | −25 / −75 | 18 | 14 |
| metal + batch | −50 / −100 | 12 | 16 |
| metal + batch + overclock + performance | +2 / −144 | 24 | 19 |
| metal + CNC + batch | −75 / −125 | 6 | 18 |
| metal + 2 batch | −100 / −150 | **0: never finishes** | 20 |

### 1.3 Chip recipes

The chip tags `REQUIRED_ITEM_ID_n` / `REQUIRED_ITEM_COUNT_n`, `PROGRESS_ITEM_TARGET_TAG`,
`PROGRESS_ITEM_TARGET_ITEM_ID_TAG`, `PROGRESS_ITEM_TARGET_ITEM_COUNT_TAG` and
`PROGRESS_ITEM_REQUIRED_MACHINE_TAG[_n]` are set by `MachineProgressHelper.SetSingle/SetDualAdditionalRequiredItem`,
`InitProgressSourceItem(GameItem, int, string, bool, int, string)` and `AddRequiredMachineTag`; the
melee chips are built by `ModuleDirectory.CreateMeleeChip` and the frame and gun-part chips by
`CreateCncChip`. Work, inputs and required modules are VISIBLE in the chip tooltip
(`CreateSourceItemDefaultTooltip`). Matches `progress_recipes` in `var/gamedata/recipes.json`.
List input values: `printer_plastic` 12, `scrap_metal` 10, `common_electronic` 33, `metal_ingot` 50
(Fair). Nights and power assume the minimum module set the chip needs. "Margin" here = output at list
value − inputs at list value − power (chip price left out; §1.8 uses prices a player pays).

| Chip (value) | Inputs | Output ×n (value each) | Work | Needs | Nights / power | Margin (list) | Offered by the retired gunsmith |
|---|---|---|---|---|---|---|---|
| `printer_chip_small_bottle` (100) | 1 plastic | `small_bottled_water` ×6 (4, empty) | 10 | — | 1 / 8 | +4 | every visit |
| `printer_chip_internal_springs_improved` (150) | 2 scrap | `internal_springs_improved` ×4 (20) | 20 | — | 1 / 8 | +52 | every visit |
| `printer_chip_makeshift_rds` (200) | 1 plastic + 1 electronic | `rds_makeshift` ×4 (25) | 20 | — | 1 / 8 | +47 | every visit |
| `printer_chip_makeshift_toy_laser` (200) | 1 plastic + 1 electronic | `laser_toy` ×4 (18) | 20 | — | 1 / 8 | +19 | every visit |
| `printer_chip_rds` (300) | 3 scrap + 1 electronic | `rds` ×2 (60) | 24 | — | 1 / 8 | +49 | 40 % from day 20, else 15 % |
| `printer_chip_laser_sight` (300) | 2 scrap + 1 electronic | `laser` ×2 (50) | 24 | — | 1 / 8 | +39 | 40 % / 15 % |
| `printer_chip_silencer` (300) | 5 scrap | `silencer` ×2 (55) | 24 | — | 1 / 8 | +52 | 40 % / 15 % |
| `printer_chip_22_mag` (250) | 1 plastic | `mag_22` ×2 (15) | 24 | — | 1 / 8 | +10 | 33 % |
| `printer_chip_22_mag_high` (300) | 2 plastic | `mag_22_high` ×2 (25) | 36 | — | 2 / 16 | +10 | 20 % |
| `printer_chip_22_mag_ext` (350) | 3 plastic | `mag_22_ext` ×2 (35) | 48 | — | 2 / 16 | +18 | 25 % |
| `printer_chip_10_mag` (300) | 3 scrap | `mag_10mm` ×2 (30) | 36 | metal | 2 / 24 | +6 | 25 % |
| `printer_chip_10_mag_high` (350) | 2 scrap | `mag_10mm_high` ×1 (40) | 48 | metal | 2 / 24 | −4 | 15 % |
| `printer_chip_10_mag_ext` (400) | 3 scrap | `mag_10mm_ext` ×1 (50) | 60 | metal | 3 / 36 | −16 | 20 % |
| `printer_chip_pistol` (250) | 2 plastic | `printed_gun` ×1 (40, New +25 % → 50; contraband 1) | 50 | — | 3 / 24 | +2 | every visit |
| `printer_chip_pistol_double` (400) | 3 plastic | `printed_gun` ×2 (50) | 60 | — | 3 / 24 | +40 | 25 % from day 20 only |
| `printer_chip_box_cutter` (150) | 2 scrap + 1 plastic | `box_cutter` ×3 (28) | 34 | — | 2 / 16 | +21 | every visit |
| `printer_chip_kitchen_knife` (200) | 3 scrap + 1 plastic | `kitchen_knife` ×4 (45) | 64 | — | 3 / 24 | +82 | every visit |
| `printer_chip_hatchet` (250) | 4 scrap + 1 plastic | `hatchet` ×4 (55) | 75 | — | 4 / 32 | +97 | 40 % / 15 % |
| `printer_chip_kitchen_cleaver` (300) | 5 scrap + 2 plastic | `kitchen_cleaver` ×4 (65) | 82 | — | 4 / 32 | +107 | 40 % / 15 % |
| `printer_chip_combat_knife` (380) | 2 ingots | `combat_knife` ×5 (75) | 98 | metal | 5 / 60 | +148 (Fair) | 20 % + 10 per tier |
| `printer_chip_combat_machete` (480) | 3 ingots | `combat_machete` ×3 (185) | 120 | metal | 5 / 60 | +246 (Fair) | 15 % + 10 per tier |
| `printer_chip_gun_part` (300) | 1 ingot | `gun_part` ×2–5 by purity (25) | 30 | metal + CNC | 2 / 28 | −3 (Fair), +4.5 (Low), +9.5 (High) | 25 % + 10 per tier |
| `printer_chip_handmade_pistol_frame` (350) | 1 ingot | `handmade_pistol_frame` ×1 (70, New → 87.5; contraband 1) | 60 | metal + CNC | 4 / 56 | §1.5 | 20 % + 10 per tier |
| `printer_chip_heavy_handmade_pistol_frame` (450) | 1 ingot + 1 scrap | `heavy_handmade_pistol_frame` ×1 (100, New → 125; contraband 2) | 70 | metal + CNC | 4 / 56 | §1.5 | 15 % + 10 per tier |

The melee rows (box cutter to machete) carry `PRINTER_MELEE_CHIP_TAG` and use the expected condition
mix of §1.4 (×0.821 of list value). The gunsmith's rolls are independent per visit; he also sells
finished mods on rolls (`silencer2`, `laser2`, mags, springs at 50 %). The shabby pistol chip from the
start and the network upgrade is the only free chip. No other seller, loot table or crate carries
printer chips or printer modules (inferred: no other reference in the decompile or
`var/gamedata/loot_tables.json`).

### 1.4 What sets output quality

| Output kind | Rule | Vis. | Evidence |
|---|---|---|---|
| Melee (`PRINTER_MELEE_CHIP_TAG`) | Each weapon, batch extras included, rolls score = uniform integer 20–30 (inclusive) + q + **average ingot bonus** of the consumed ingots (Very low −30, Low −20, Fair 0, High +20, Very high +30, Perfect +100; integer average; 0 when no ingots). Score < 25 **Poor** (−25 %), < 60 **Fair** (−12 %), < 90 **Good** (0), else **Excellent** (+12 %). The condition is an innate % on the sale value | condition VISIBLE on the item; the roll is HIDDEN | `ApplyPrintedMeleeCondition(GameItem, GameItem)`, `CaptureIngotQuality(...)`, `RNG.GetRandomInt` (max inclusive), `ItemConditionList.CreateEquipementCondition*` |
| Gun-part count | Purity of the consumed ingot: Very low 2, Low 3, Fair 3, High 4, Very high 4, Perfect 5 (3 if none) | VISIBLE (count) | `GUN_PART_COUNT_BY_PURITY` |
| Frames (`GUN_FRAME_TAG`) | Stores print quality = q + ingot bonus in `GUN_FRAME_PRINT_QUALITY_INT`. The finished gun's firepower, style, reliability, recoil, ergonomics and accuracy are scaled by (100 + quality) % | HIDDEN | `HandlePrintedItemQuality(...)`, `GunHelper.MultiplyBaseStat(GameItem, int)` |
| Printed gun (`ADV_GUN_TAG`) | Stats × (100 + q) % (no ingot term) | HIDDEN | same |
| Everything else | none: mods, mags and bottles come out plain | | |

Gun stats do not enter the sale value; they matter to gun-order bonus requirements
(`GunOrderData`, inferred from the only other readers of the stat tags).

Expected value multiplier for melee by s = q + ingot bonus (computed from the 11 equally likely rolls):

| s | ≤ −10 | −5 | 0 | 1…4 | 5…29 | 30 | 35 | 40…59 | 60 | 70+ |
|---|---|---|---|---|---|---|---|---|---|---|
| P(Poor) | 11/11 | 10/11 | 5/11 | (5 − s)/11 | 0 | 0 | 0 | 0 | 0 | 0 |
| best result | Poor | Fair | Fair | Fair | Fair | Good 1/11 | Good 6/11 | Good | Exc 1/11 | Excellent |
| × list value | 0.750 | 0.762 | 0.821 | — | 0.880 | 0.891 | 0.945 | 1.000 | 1.011 | 1.120 |

- **Fair ingots and no quality module**: 5/11 Poor, 6/11 Fair (×0.821). **Low or Very low ingots**: every
  weapon Poor (×0.750). High (+20) and Very high (+30, 1/11 Good) are ×0.880 / ×0.891. Only Perfect
  ingots reach Excellent.
- **Quality ≥ 20 % (q ≥ 5) removes every Poor roll**: three `system_module_quality` (+8 each, q 6) or one
  `system_module_fineness` (+25, q 6). +7.2 % on every melee print. Good needs s ≥ 40 (Very high
  ingots + quality 40, or the capped neural core's +50).
- Per run (computed, Fair ingots): machete 456 → 488 with a fineness module (+32); kitchen knife 148 →
  158. Fair ingots + fineness = High ingots without it, so ore is not worth spending on purity here.

### 1.5 Gun frames → guns (item-on-item)

`gun_part` targeted onto an owned `GUN_FRAME_TAG` frame (both owned, frame in an inventory) → the
frame's gun (`GunFrameHelper.Finish(GameItem, GameItem)`, via `MaterialDirectory.GunPart()`). The part
and the frame are consumed. A printed frame (it carries a print quality) always yields condition **New**
(+25 %); an unprinted frame passes its own condition on (`GunFrameHelper.GetCondition`).

| Chain (computed, Fair ingot at 50, part at 25) | Output value | Inputs + power | Margin |
|---|---|---|---|
| Ingot → `handmade_pistol_frame` + `gun_part` → `handmade_pistol` (120 → 150 New, contraband 1) | 150 | 50 + 25 + 56 | +19 |
| Ingot + scrap → `heavy_handmade_pistol_frame` + `gun_part` → `heavy_handmade_pistol` (160 → 200 New, contraband 2) | 200 | 60 + 25 + 56 | **+59** |

Both guns are contraband; selling or holding them carries the risks in `CONCEALMENT.md`.

### 1.6 Disassembly with a screwdriver (`GunHelper.DisassembleWeapon(GameItem)`)

`screwdriver` (25, kept) on an owned `ADV_GUN_TAG` gun: it unloads the gun (`TryUnloadAll`),
destroys it and gives parts. Use it only when a gun's condition or features push its value below
the parts; at Poor (−25 %) every gun is still worth more whole (computed).

| Gun (value, contraband) | Parts (value) |
|---|---|
| `printed_gun` (40, 1) | 2 plastic (24) |
| `handmade_pistol` (120, 1) | 2 gun parts + 1 plastic + 2 scrap (82) |
| `heavy_handmade_pistol` (160, 2) | 2 gun parts + 1 plastic + 4 scrap (102) |
| `heavy_handmade_pistol_s` (225, 2) | 3 gun parts + 1 plastic + 4 scrap (127) |
| `glock_receiver` (200, 2) | 4 gun parts (100) |
| `revolver_short_tac` (230, 2) | 5 gun parts (125) |
| `revolver` (240, 2) | 5 gun parts (125) |
| `smg` (280, 3) | 6 gun parts (150) |
| `shotgun` (310, 3) | 7 gun parts (175) |

**Gun repair** with a gun part (`GunHelper.TryRepairGun(GameItem, GameItem)`, part consumed):
chance = base repairability + the carried chance, which grows by the base after each failure and resets
to 0 on success (so base × attempts since the last success). Base repairability (`GUN_REPAIRBILITY_TAG`,
`GunHelper.InitGun`): printed and handmade 100, heavy 80, short tac revolver 75, heavy-s 70, glock 65,
revolver 60, SMG 40, shotgun 35. Success raises the condition one step: Poor → Fair → Good →
Excellent, never to New; New and Excellent cannot be repaired (`CanRepair`). A part (25) buys +12–13 %
of the gun's value, so repair pays on every gun above ~200 (computed).

### 1.7 Who buys the output

| Output | Item types | Buyers | Evidence |
|---|---|---|---|
| Melee weapons | `WEAPON`, `MELEE_WEAPON` (knife and cleaver also `HOUSEHOLD_GOOD`, box cutter and hatchet `TOOL`) | merchant, mercenary buyer (300 + 2/day), revolutionary buyer, weapon display buyers, and gun-order clients who fall back to buying a melee weapon | `CLIENTS.md`, `StoreClientListGun.TryStayForMeleeWeapon` |
| Gun mods (springs, sights, silencer, mags) | `GUN_MOD` only | no general or display buyer lists `GUN_MOD`. They sell to the merchant (anything not contraband), the vending machine at 0.15× its usual sale chance, and inside gun orders: a service order pays the fitted parts' value, a "best" order pays 1.5× the gun's value with its fitted mods, bonus requirements (e.g. suppressed) pay extra | `VendingMachineHelper.GetSaleChance`, `GunOrderData.GetTotalReward/GetPartsValue` |
| Printed gun, frames, handmade pistols | contraband 1–2 | contraband buyers; risk per `CONCEALMENT.md` | |
| Gun parts | `MATERIAL` | material buyers (`PRODUCTION.md` §3), gun repair, frames | |

Gun mods are the printer's cheapest outputs to make but have the narrowest market; the gym values
output at its estimate, which overstates mods if they do not sell (inferred).

### 1.8 Max output

**Input prices a player pays** (hard; computed from the sellers' terms; `PRODUCTION.md` §3 for ore and
flux). Wholesale lots are bought whole at the discount (`PlayerStore.GetWholesalePrice`).

| Input (list) | Main sellers, per visit | Paid on hard |
|---|---|---|
| `scrap_metal` (10) | junker (general pool): 3 + 50 % a fourth, wholesale −15 %. Material seller via `sign_material` in the showcase (75 % a day; scav or thief by reputation): scav 1 + four 50 % rolls, −15…−25 %. Story scrap seller (day 5): 5 at −33 %. Occasional: scav haul 4 at −25 %, salvage pilot 2 at −33 %, material surplus event, Rev quartermaster, scav barter | **8.5** (6.7–8.5) |
| `printer_plastic` (12) | retired gunsmith: 4 every visit at +30 %. Junker: 2 + 50 % a third, −15 %. Scav material seller: three rolls at 25 + LL rep/2 % (min 10), −15…−25 %. Scav general: three 15 % rolls. Supplier: 4 at `premiumBuy` (+50 % × 2 on hard = +100 %) | **15.6** at the gunsmith; 10.2 junker; 24 supplier |
| `common_electronic` (33) | junker 1 + 15 %; scav material seller 80 %; scav general 80 %; thief material seller (black-market rep): 1 + four (50 + BM rep/2) % rolls, **stolen**, −15 %; salvage pilot 3 at −33 % | **28** (22–28) |
| `metal_ingot` (50 Fair) | no seller: the furnace. Fair = 2 scrap + 1 flux + 8 power; blast module: 3 scrap + 1 flux + 16 power → 2 Fair | **29** (23 with blast) |
| power | ~1 per unit (`PRODUCTION.md` §0) | 1 |

`StoreClientList.CreateJunker`, `StoreClientListSupplier.AddScavMaterialStock/AddThiefMaterialStock`,
`ShowcaseHelper.GetMaterialSupplierId`, `StoreClientListStory.CreateStoryScrapSeller`,
`StoreClientList.PlaceSupplierInventory`, `ItemFeatureList.PremiumBuy`. The material sign is sold by
Dr. Jackson while not owned (`PlaceInventorInventory`, `AddIfNotOwn`) and is the Scav start's sign.

**Ranking, bare minimum modules, paid prices** (computed; "/ scrap" counts an ingot as 2 scrap). Chip
price left out: chips are capital.

| Chip | Inputs (paid) | Nights / power | Output EV | Margin / run | / night | / power | / scrap |
|---|---|---|---|---|---|---|---|
| combat_machete | 3 ingot (87) | 5 / 60 | 456 | +309 | +61.7 | +5.14 | **+51.4** |
| silencer | 5 scrap (42) | 1 / 8 | 110 | +60 | +59.5 | +7.44 | +11.9 |
| rds | 3 scrap + 1 elec (54) | 1 / 8 | 120 | +58 | +58.5 | +7.31 | +19.5 |
| springs | 2 scrap (17) | 1 / 8 | 80 | +55 | +55.0 | +6.88 | +27.5 |
| makeshift_rds | 1 plastic + 1 elec (44) | 1 / 8 | 100 | +48 | +48.4 | +6.05 | — |
| laser_sight | 2 scrap + 1 elec (45) | 1 / 8 | 100 | +47 | +47.0 | +5.88 | +23.5 |
| combat_knife | 2 ingot (58) | 5 / 60 | 308 | +190 | +38.0 | +3.16 | +47.5 |
| kitchen_knife | 3 scrap + 1 plastic (41) | 3 / 24 | 148 | +83 | +27.6 | +3.44 | +27.6 |
| cleaver | 5 scrap + 2 plastic (74) | 4 / 32 | 213 | +108 | +26.9 | +3.37 | +21.5 |
| hatchet | 4 scrap + 1 plastic (50) | 4 / 32 | 181 | +99 | +24.8 | +3.09 | +24.8 |
| toy_laser | 1 plastic + 1 elec (44) | 1 / 8 | 72 | +20 | +20.4 | +2.55 | — |
| box_cutter | 2 scrap + 1 plastic (33) | 2 / 16 | 69 | +20 | +10.2 | +1.27 | +10.2 |
| pistol_double | 3 plastic (47) | 3 / 24 | 100 | +29 | +9.7 | +1.22 | — |
| gun_part | 1 ingot (29) | 2 / 28 | 75 | +18 | +9.0 | +0.64 | +9.0 |
| heavy_frame | 1 ingot + 1 scrap (38) | 4 / 56 | 125 | +32 | +7.9 | +0.56 | +10.5 |
| 22_mag | 1 plastic (16) | 1 / 8 | 30 | +6 | +6.4 | +0.80 | — |
| 10_mag | 3 scrap (26) | 2 / 24 | 60 | +10 | +5.2 | +0.44 | +3.5 |
| 22_mag_ext | 3 plastic (47) | 2 / 16 | 70 | +7 | +3.6 | +0.45 | — |
| 22_mag_high | 2 plastic (31) | 2 / 16 | 50 | +3 | +1.4 | +0.18 | — |
| frame | 1 ingot (29) | 4 / 56 | 88 | +2 | +0.6 | +0.04 | +1.2 |
| small_bottle | 1 plastic (16) | 1 / 8 | 24 | +0 | +0.4 | +0.05 | — |
| 10_mag_high | 2 scrap (17) | 2 / 24 | 40 | −1 | −0.5 | −0.04 | −0.5 |
| pistol | 2 plastic (31) | 3 / 24 | 50 | −5 | −1.7 | −0.22 | — |
| 10_mag_ext | 3 scrap (26) | 3 / 36 | 50 | −12 | −3.8 | −0.32 | −3.8 |

**Best module set per chip** (standard bay, universal modules P/E/Q/overclock/eco/fineness plus batch
modules; computed by search; module prices left out). The one-night chips cannot get faster, so
modules only cut power: four `system_module_efficiancy` take the draw from 8 to 3 (+5 a run). The rest:

| Chip | Inputs plentiful (max / night) | Inputs bind (max / run) |
|---|---|---|
| combat_machete | metal + fineness + 2 overclock + performance: 3 nights, draw 20, +341 a run, **+114 a night** | metal + batch + overclock + performance: 5 nights, draw 19, 4 machetes, **+425 a run (+71 a scrap)** |
| combat_knife | metal + 3 overclock + quality: 2 nights, draw 22, +215, +107 a night | metal + fineness + 3 performance: 3 nights, +233 (+58 a scrap) |
| cleaver | batch + 3 overclock + performance: 2 nights, 5 cleavers, +149, +75 a night | batch + fineness + 3 performance: 4 nights, +160 (+32 a scrap) |
| hatchet | batch + 3 overclock + performance: 2 nights, +132, +66 a night | batch + fineness + overclock + 2 performance: 3 nights, +144 (+36 a scrap) |
| kitchen_knife | batch + overclock + 3 performance: 2 nights, +114, +57 a night | batch + fineness + 3 performance: 3 nights, +118 (+39 a scrap) |
| heavy_frame | metal + CNC + 2 overclock: 2 nights, +46 | same |
| gun_part | metal + CNC + overclock + performance: 1 night, +29 | same |
| pistol_double | 3 performance + eco: 2 nights, +41 | — |

Batch module math: metal + one batch module alone makes the machete take 10 nights at draw 16 (+361 a
run, +36 a night); one overclock + one performance cancel the −50 performance, so the fourth machete
costs no extra night. Two batch modules need +100 performance to move at all and a bay kit for room.

**What to print.**
- **Printer time is the limit** (inputs plentiful): the machete with metal + fineness + two overclocks +
  performance (+114 a night), then the combat knife; without the metal module, the cleaver or hatchet
  with a batch module and overclocks (+66–75 a night); without modules, silencer, RDS and springs tie at
  +55–60 a night.
- **Scrap is the limit** (the gym's case, docs/research/ARCHETYPES-NEW.md): margin per scrap decides.
  Ingot melee through a furnace earns +51 (machete) to +71 (machete + batch) per scrap, the combat knife
  +48–58, springs +27.5, the kitchen knife +28–39. The silencer (+12) and RDS (+20) waste scrap.
  Without a furnace and metal module: springs for scrap, makeshift RDS for electronics (+48 per
  electronic, against +58 for the RDS that also burns 3 scrap worth +82 as springs).
- **Plastic** (4 a week from the gunsmith) goes to makeshift RDS with an electronic (+48), else to
  kitchen knives; the .22 mags, pistols and bottles return almost nothing.
- **Gun mods need a market** (§1.7): springs and sights realize their value through gun orders and
  the merchant; melee sells to every weapon buyer.

### 1.9 Facts to confirm in the lab

Expected results, from the code above: one-night chip (springs) completes in one night drawing 8;
the metal module raises the draw to 12; metal + CNC gives speed 18; metal + two batch modules never
finishes; a batch module adds one weapon; a bare kitchen-knife run over many runs is about 45 % Poor
and 55 % Fair, and 100 % Fair with a fineness module; Low ingots on the machete give all Poor; gun parts
per ingot 3 (Low/Fair), 4 (High), 5 (Perfect); a loaded input grid restarts the chip each night
without tending; stolen inputs are accepted and outputs are not stolen; the Gunsmith start's two stuck
modules give speed 24 and draw 7, and whether metal + CNC still fit around them.

## 2. Desequencer (`desequencer`, `MachineDesequencer`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | 350, **contraband 2**, `MACHINE`, `STANDARD_MACHINE_TAG` | VISIBLE | `MachineDesequencer.Desequencer()` |
| Panels | battery, module bay (standard 26-cell; universal and alarm modules; an alarm module does nothing here), chip slot (an owned crypto module: any non-machine item with `DESEQUENCER_TARGET_ID_TAG`), input slot (one owned `ACCESS_CARD`), output slot (one owned access card, not a blank) | VISIBLE | `CreateMachineInventoryWindow`, slot rules in `Desequencer()` |
| Recipe | crypto module (kept) + **any** owned access card in the input slot → the module's keycard. The input card is consumed at the finish; a blank (`blank_keycard`, 15) is the cheap input, but a spare keycard works too | VISIBLE (module tooltip, progress text on the card) | `FinishDesequencer(GameItem, GameSlotInventory, GameSlotInventory)` |
| Night cycle | Needs a battery that can pay the full draw. Ready + card + module + **empty output slot**: draws power, copies the module's target id and work onto the machine, locks the input card and adds one night of progress. Working: if the output slot is empty, draws power and adds progress; on reaching the target, puts the card in the output slot and returns to ready | progress VISIBLE | same |
| Output slot full | A card left in the output slot **stalls** the machine: no progress, no draw (it does not finish into a fuller slot) | DERIVABLE | same |
| Module captured at start | Target id and work are copied when the run starts and the module is never locked, so removing or swapping it mid-run changes nothing; one module can start runs on several desequencers in turn (inferred) | HIDDEN | same |
| Speed / power | 33 a night, 10 power per working night; speed = 33 + trunc(33 × P / 100), draw = 10 − trunc(10 × E / 100), min 1 | DERIVABLE | `InitProgressTypeMachine(machine, 33, …)`, `InitMachinery(machine, 10)`, `ApplyBasicModuleEffect` |
| One card per run | One input and one output slot: one card per run, then a morning visit to empty the output | VISIBLE | |

| Crypto module (value, contraband) | Work → nights bare / with modules (computed) | Output (value, contraband) | Margin, blank at 22.5, power 1 (computed) |
|---|---|---|---|
| `crypto_module_ser` (200, 1) | 75 → 3; 2 at P ≥ 16 | `ser_keycard` (40, 1) | −12.5; −2.5 in 2 nights |
| `crypto_module_sup` (200, 1) | 75 → 3; 2 at P ≥ 16 | `sup_keycard` (40, 1) | same; see "sources" |
| `crypto_module_med` (250, 2) | 100 → 4; 3 at P ≥ 4; 2 at P ≥ 52 | `med_keycard` (75, 1) | +12.5 (3.1 a night); +32.5 in 2 nights (16.3) |
| `crypto_module_eng` (250, 2) | 100 → 4; 3 at P ≥ 4; 2 at P ≥ 52 | `eng_keycard` (75, 1) | same as med |
| `crypto_module_sec` (300, 3) | 125 → 4; 3 at P ≥ 28 | `sec_keycard` (100, 2) | +37.5 (9.4); +47.5 in 3 nights (15.8) |
| `crypto_module_cmd` (350, 4) | 150 → 5; 4 at P ≥ 16; 3 at P ≥ 52 | `cmd_keycard` (125, 3) | +52.5 (10.5); **+72.5 in 3 nights with four performance modules (24.2)** |

Blank card at 22.5 = 15 × 1.5, Dr. Jackson's hard markup (`PRODUCTION.md` §4). Four performance
modules (P 64, speed 54) are the best set: they reach every breakpoint above (ser/sup, med/eng in 2
nights, sec and cmd in 3) without raising the draw. Sec in 2 nights would need P ≥ 91 (speed 63).

**What a keycard is worth beyond its price**

| Use | Rule | Evidence |
|---|---|---|
| Open a locked crate | A keycard opens a crate with the matching lock and is consumed; **the command card opens any lock** | `LockHelper.CanOpenLock/TryOpenLock`, `MiscItemDirectory` keycard targets |
| Exchange: command card | 1 `cmd_keycard` → 2 random cards from sec / eng / med (EV 167 against 125), once a night | `ExchangeList.CommandKeycardBarter` |
| Exchange: crates | 3 sec → a security loot crate; 4 med → a medical crate; 4 eng → an engineering crate (the module source in `MODULES.md`); one a night each, promotional | `ExchangeList` (`sec_crate_barter`, `med_crate_barter`, `eng_crate_barter`) |
| Sell | Keycards are `ACCESS_CARD` + contraband 1–3: the shady merchant buys them; holding them is a seizure risk (`CONCEALMENT.md`) | `CLIENTS.md` |

Via the exchange the command card is the best recipe: 3 nights with four performance modules, 22.5 +
30 in, EV 167 out (+114, 38 a night, computed), one a night at most, once the Underground Exchange is open (`MODULES.md` §2.1).

**Sources** (hard prices: Dr. Jackson +50 %).

| Item | Seller and odds per visit | Evidence |
|---|---|---|
| desequencer | Dr. Jackson: 66 % while none is owned (with one blank card and crypto rolls ser 10, med 7, eng 7, sec 5, cmd 2 %); 25 % once owned. Guaranteed when the player owns a locked crate whose keycard he lacks | `StoreClientList.PlaceInventorInventory`, `GuaranteeLockedCrateKeycardItems` |
| crypto modules | Jackson once a desequencer is owned: ser 20, med 15, eng 15, sec 10, cmd 5 %, plus 2 blank cards every visit. Jackson's special stock (`MODULES.md` §2.1): one of each but sup. The thief (black market, stolen): ser 10, med 7, eng 7, sec 5, cmd 2 % × the BM-rep stock multiplier. **`crypto_module_sup`** is offered only by the locked-crate guarantee | `PlaceInventorInventory`, `PlaceInventorSpecialStock`, `StoreClientList.CreateThief`, `GuaranteeLockedCrateKeycardItems` |
| blank cards | Jackson only (above), plus the locked-crate guarantee tops up to one blank per locked crate | same |

**Max output**: one desequencer with four performance modules makes a command card every 3 nights
(+72.5 sold, +114 through the exchange); with only a sec module, +47.5 per 3 nights. The machine is
contraband 2 and its outputs contraband 1–3, so it suits a shop that already hides contraband.

## 3. Bottle printer (`bottle_printer`, `MachineBottlePrinter`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | 100, `IMPORTANT_TAG`, no item type (not a `MACHINE`), battery slot, output grid 4×4 (+1 column when normal unlocks, +1 when large unlocks: up to 6×4); output accepts only owned liquid containers | VISIBLE | `BottlePrinter()`, `ApplyUpgradeState` |
| Recipes | Three **buttons**, real time, no night cycle, power only: `small_bottled_water` (450 ml, 4) for 4 power; `bottled_water` (750 ml, 5) for 6 power once 3 upgrades are in; `large_bottled_water` (2 L, 9) for 11 power once 9 are in. A press does nothing if the battery cannot pay or the grid has no room | VISIBLE (buttons, tooltip) | `SMALL/NORMAL/LARGE_PRINT_COST`, `TryPrint`, `IsNormalUnlocked` (≥ 3), `IsLargeUnlocked` (≥ 9) |
| Upgrade | `common_electronic` (owned) targeted onto it: +1 count, max 9; **the printer's value rises by the electronic's 33**; Tinkerer keeps the electronic 20 % of the time | VISIBLE (tooltip counts) | `UpgradePrinter(GameItem, GameItem)`, `MaterialDirectory.Electronic()`, `StartingPerkContent.ConsumeUpgradeMaterial` |
| Bottles | Come out empty and owned; their value is the container price (4 / 5 / 9) until filled (`WATER.md`) | | `FoodItemDirectory.SmallBottledWater/BottledWater/LargeBottledWater` |
| Obtained | Water Merchant start (shabby, with a 25-charge cell; `WATER_TRADER` set unlocked). Dr. Jackson offers one every visit while none is owned, but only with the `WATER_TRADER` network upgrade (300; 450 on hard) | | `NewGameData` (WATER_MERCHANT branch), `StoreClientList.PlaceInventorInventory` (`AddIfNotOwn`), `NetworkUpgradeList` (`WATER_TRADER`) |

**Max output** (computed, power at 1): as products the bottles lose 0 / 1 / 2 each (4−4, 5−6, 9−11).
As containers for water, power per litre is 8.9 (small), 8.0 (normal) and 5.5 (large). The 3D printer's
small-bottle chip makes 6 small bottles from 1 plastic + 8 power: 3.3 each with plastic at 12, 3.9 at the
gunsmith's 15.6, against 4 power on the bottle printer, so the chip saves at most 0.7 a bottle and
costs a printer night. Unlocking large costs 9 electronics (297, kept in the printer's value). Which
size to print is a water question (`WATER.md`).

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
| `surgery_tool` → live sturgeon | caviar / roe jars | fish health | AQUACULTURE.md §2.5 | `AquacultureHelper.StripEggs(GameItem)` |
| blade → fish / rat | carcass, then cuts | the animal | AQUACULTURE.md §2.4, HUSBANDRY.md | `HusbandryHelper.InitBladeItem` |

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

The full notes moved to their own files (2026-11-12): **fish, the RAS tank and its water** in
[`AQUACULTURE.md`](AQUACULTURE.md), **rats, cages, genes, the feed dispenser, the auto-butcher, the rat trap and the
retired rancher** in [`HUSBANDRY.md`](HUSBANDRY.md). The short version and the corrections to this section's old text:

- **Fish meat is 0.9 × every kcal swallowed, stomach included** (`AquacultureHelper.GetMeatKcal`): a three-day-old
  tilapia that fills up overnight is worth a fish fillet and a tiny one (~70), the same as a fully fed adult on the
  growth curve. Weight does not enter the fillet count. AQUACULTURE.md §2.2, §2.4.
- **Rats peak at age 10 and then lose ~17 g a night**; a base-gene rat peaks at 407 g, and the cut value steps from
  125 (351–399 g) to 174 at **401 g** (exactly 400 g is 159). HUSBANDRY.md §3, §12.2.
- Cage wear is never doubled (nothing sets the overcrowding tag); overcrowding attacks are 5 % a night above 70 %
  occupancy and 15 % above 90 %. HUSBANDRY.md.
- The auto-butcher's keepers are chosen each night from that night's candidates (0–10 of each sex, marked or named
  animals protected and counted); spare-pregnant, cull-sick and cull-prion default on. HUSBANDRY.md.
- The Rancher and Aquafarmer starts have **different** kits: the Aquafarmer gets two shabby RAS tanks, a tilapia pair,
  fertilised roe, two bags of industrial feed and two jugs of ghost water, for −450 cash and +40 rent
  (`NewGameData`). AQUACULTURE.md §4.
- Rats cannot draw pellet or industrial-fish-feed kcal from a dispenser; fish can draw rat meat. HUSBANDRY.md,
  AQUACULTURE.md §2.2.

## 8. Fridge (`fridge`, `MachineFridge`)

The fridge adds an innate % feature to items; it makes no new item.

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | 100, `MACHINE`, `TURBO_BOOST_ELIGIBLE_TAG`, battery slot, chill zone 5×2 and freeze zone 3×2 (+1 row to each per storage upgrade, up to 5×9 and 3×9) | VISIBLE | `Fridge()`, `ApplyStorageSize` |
| Accepts | owned items that are liquid containers (any), chillables (`BREVAGE` or `WATER_RATION` type, or `bottled_water_premium`), `beis_icecream` or `nudka`; never machines, `STORAGE` items or `wine_bottle` | VISIBLE | `MayZoneAcceptItem(GameItem)`, `IsChillable`, `IsFreezable`, `IsSpirit` |
| Power | Draw = 2 + 2 × storage upgrades − 2 × efficiency upgrades, floor 2. Drawn once a night, only when either zone holds something; if the battery cannot pay, nothing is cooled that night | DERIVABLE | `UpdateFridgePowerUsage`, `MachineHelper.TryDrawCyclePower` |
| Upgrades | `scrap_metal` → +1 row in both zones, +2 draw (max 7); `common_electronic` → −2 draw (max 7, floor 2). Each adds the material's value to the fridge | VISIBLE (tooltip) | `UpgradeStorage`, `UpgradePowerEfficiency` |
| Night effect | Chillables in **either** zone get `chilled` (+20 %). `nudka` in the chill zone gets `chilled` (+20 %); in the freeze zone `iceCold` (+25 %, replaces chilled). `beis_icecream` in the freeze zone gets `frozen` (+25 %); in the chill zone nothing. Chilling also reaches chillable items **inside** an accepted item (a `beerCase` is `BREVAGE`, so a full case chills its 8 `red_beer`) | VISIBLE (feature line; not exposable) | `Fridge()` cycle lambda, `ChillItem`, `IceColdItem`, `FreezeItem`, `PreBuiltItemHelper.FullBeer` |
| Not chilled | Filled water bottles (`small_bottled_water` etc. are `HOUSEHOLD_GOOD`), wine, food other than ice cream: they sit in the fridge and only their liquid temperature changes | | `IsChillable` |
| Expiry | Before the night's machine pass every "kept cold" marker is cleared; after it, any item with a cold feature but no fresh marker loses it. An item keeps its bonus through the next day only if it spent the night in a powered fridge | DERIVABLE | `PlayerStore` night (`ClearColdMarkers` … `HandleColdFeatureExpiry`) |
| Real time | While the battery holds ≥ 1, non-empty liquid containers move toward 3 °C (chill, 0.5 °/s) or −18 °C (freeze, 1 °/s). Temperature carries no value feature (inferred) | | `CoolZoneContainers` |
| Obtained | Dr. Jackson: every visit while none is owned, then 25 % | | `StoreClientList.PlaceInventorInventory` |

**Max output** (computed). The bonus is **once per item**, not per night: one powered night in the fridge
makes the item sell for +20 / +25 % the next day, and every further night only keeps it. The gain is
the bonus on what is sold, less 2 power a night.

| Item (value) | Zone | Gain when sold |
|---|---|---|
| `bottled_water_premium` (325) | either | +65 |
| `beerCase` with 8 `red_beer` (18 each) | either | +28.8 |
| `nudka` (85) | freeze | +21.3 (chill zone +17) |
| `liquid_luck` (100) | either | +20 |
| `beis_icecream` (40) | freeze | +10 |
| `water_ration` (30) / `water_ration_small` (15) | either | +6 / +3 |
| `energy_drink`, `red_beer` (18), `processed_milk` (16), `soda_red` (15), `processed_juice` (14) | either | +2.8–3.6 |

A fridge pays when the shop sells premium water, spirits, beer cases or rations steadily: 2 power a
night against +3 to +65 per item sold. Rotate stock through it the night before it goes on sale;
leaving items in longer earns nothing more.

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
| Chip, module and input sellers | `StoreClientListSpec.CreateRetiredGunsmith` (roll odds, local `Chance`), `StoreClientList.CreateJunker`, `PlaceInventorInventory`, `GuaranteeLockedCrateKeycardItems`, `StoreClientListSupplier.AddScavMaterialStock/AddThiefMaterialStock` | `AddDirectSellingItemToTable` calls inside dialogue end actions | **code only**: transcribe (§1.3, §1.8, §2) |
| Keycard exchanges | `ExchangeList.CommandKeycardBarter`, `SecCrateBarter`-style builders (`sec/med/eng_crate_barter`) | `ExchangeBarter` objects (conditions, counter) | yes, plain objects; `onAccept` is code |
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
