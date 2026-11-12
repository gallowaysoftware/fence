# Machine modules (RE notes)

Build 26525552 / 0.46D. Conventions as in `PRODUCTION.md`. Read from the decompile, then measured
headless where marked **[lab]** (see "Verification" at the end). The owner-facing version is
[`docs/guides/MACHINE-MODULES.md`](../guides/MACHINE-MODULES.md); the machine-readable table is
[`modules.json`](modules.json). Water, chemistry and printer recipes stay in `WATER.md`,
`CHEMISTRY.md` and `RECIPES.md`; this file covers what the module bay does to each machine.

## 1. How a module bay works

| Rule | Value | Evidence |
|---|---|---|
| Stats | Every module carries three integers: performance P, efficiency E, quality Q (%). The machine sums them over everything in its bay (modules and nodes) into `TOTAL_PERCENTAGE_{PERFORMANCE,POWER,QUALITY}_BONUS_INT`. Sums are linear: **no cap, no diminishing returns** in the sum itself | `ModuleHelper.ComputeModuleEffect(List<GameItem>, GameItem)` |
| Speed | progress machines: `speed = base + trunc(base·P / ratio) / 100`, truncated; ratio = `MACHINE_PERFORMANCE_RATIO_INT` (2 on hydroponic, DNA manipulator, wine rack; 4 on projector; else 1) | `ModuleHelper.ApplyBasicModuleEffect` |
| Power | `draw = base − trunc(base·E / 100)` (wine rack: E and P both divided by 2 and P also *lowers* the draw), never below 1. Negative E raises the draw | same; `MachineWineRack.AgeWell` tags |
| Quality | Read per machine, raw or divided by `MACHINE_QUALITY_RATIO_INT` (`ModuleHelper.GetAjustedQuality`) — §3 says which | per machine |
| When applied | On every insert/remove and again after every working night (`MachineHelper.OnMachineActioned`), which also fires per-use module effects (§2.3) | `MachineHelper.SetupModuleBay`, `OnMachineActioned` |
| **Integer breakpoints** | Progress only counts in whole nights and the overshoot is lost (`MachineProgressHelper.FinishProgressTypeMachine` resets to 0), and harvest counts round. So a module that does not cross a breakpoint does **nothing** (e.g. one performance module on a hydroponic bay: speed 8 → 8) | **[lab]** |
| Locked bay | The hydroponic bay locks its module grid from planting to harvest; a **perennial (bloomberry) re-locks in the same night**, so its bay is locked for good. Only a hatchet (destroys the seed) unlocks it. Printer and DNA manipulator lock while working | `HydroponicHelper.StartGrowCycle/StartNextFruitingCycle`, `RemovePlantFromHydroponic`; **[lab]**: insert refused on a working bloomberry bay |
| Owned only | A bay accepts only owned items of its allowed module types (`MODULE_TYPE_UNIVERSAL` plus the machine's own type) | `ContainerHelper.AllowOnlyTaggedItemsOr` |

### 1.1 Bay geometry

Standard bay: 7×5 with the four corners and a centre plus blocked (26 cells,
`MachineHelper.DEFAULT_MODULE_BAY_CONFIG`). `module_bay_expansion_kit` (150; target any
`STANDARD_MACHINE_TAG` machine; two uses per machine) opens it to 31 then 35 cells and adds 100 to the
machine's value (`MachineHelper.ExpandModule`, `ModuleDirectory.ModuleBayExpansionKit`). Items rotate
(R/drag keys, `ItemMouseDragHandler`). Plain modules are 2×2, most machine-specific modules 1×4
(sprite footprints, `var/gamedata/item_shapes.json`).

| Bay | 2×2 only | 1×4 + 2×2 (best hand packing, computed) | Auto-placement (game's `TryAcceptAll`, **[lab]**) |
|---|---|---|---|
| standard | **4** | 1+4, 2+2, 3+1, 4+0 | 4 squares; 3 long; 1 long + 2 squares |
| kit 1 | **6** | 1+5, 2+4, 3+3, 4+2 | 6 squares; 6 long; 1 long + 4 squares |
| kit 2 | 6 | 2+6, 3+5, 4+4 | 6 squares; 8 long; 1 long + 6 squares |

Adding kits to a full bay and then dropping more squares in fitted only +1 square (auto-placement does
not repack): fit kits first, or re-place by hand. Evaporator, blender, finisher and plate have their
own 4×4 bays (4 squares) and take no kit.

## 2. The modules

### 2.1 Universal modules (2×2, any standard machine)

| id | value | P / E / Q | Where sold |
|---|---|---|---|
| `system_module_performance` | 70 | +16 / 0 / 0 | Jackson 33 %/visit (random shape), module intro (once, first-time discount), Rev raider 22.5 % (stolen), barter offer 6 %, exchange, dump, engineering crate 50 % |
| `system_module_efficiancy` | 70 | 0 / +16 / 0 | same pattern |
| `system_module_quality` | 70 | 0 / 0 / +8 | same pattern |
| `system_module_overclock` | 100 | +36 / −44 / 0 | Jackson 16 %, Rev raider 22.5 %, barter 3 %, exchange, engineering crate 33 % |
| `system_module_eco` | 100 | −16 / +32 / 0 | Jackson 16 %, barter 3 %, exchange, dump 0.5 %, crate 33 % |
| `system_module_fineness` | 100 | −8 / −15 / +25 | Jackson 16 %, barter 3 %, exchange, dump 0.5 %, crate 33 % |
| `system_capped_neural_core` (contraband 3) | 250 | +100 / −100 / +50 | engineering crate 4 %, barter 1 %, exchange 1 % |
| `system_uncapped_neural_core` (contraband 4) | 1500 | +200 / −100 / +100 | Hypertech smuggler (network 2500), one per visit |
| `system_module_corrupt` | 45 | −8 / −5 / 0 | junk in old machines; + `common_electronic` → a fresh P (33 %) / E (22 %) / Q (45 %) module (`ModuleHelper.RestoreModule`) |
| `system_module_ruined` | 30 | 0 | screwdriver → electronic + plastic + junk; overvolted modules end here |
| `node_medium` (1×2, via `random_node`) | 50 | Support: +⅕ of every **adjacent** module's base P/E/Q (signs included); a plain `node_medium` has no effect, only the `random_node` spawn carries Support | Jackson 33 %/visit, special stock 5, engineering crate 50 %×2 |
| `system_module_streamlining`, `blank_module`, `node_small` | — | — | defined, no source found in code (inferred unobtainable) |

Other sources: the **dump** (`dumpingGroundTG`: 1 % of searches a random P/E/Q, 0.5 % a P/eco/fineness);
the **Underground Exchange** (`ExchangeList.ModuleBarter`, day 25, day 1 with Sawyer's Crew): two
modules in, one standard module out (P/E/Q weight 100, overclock/eco/fineness 33, never one you gave),
1 % a capped core, once a night; the **engineering loot crate** (`PreBuiltItemHelper.FillEngineering`;
exchange for 4 engineering keycards, some sellers); Jackson's **Monday special stock** (network 1250)
has 5 of each random module and 5 nodes, 2 kits, 2 advanced turbo boosters.

Module catalog: `ModuleDirectory.*Module()`. Prices at Dr. Jackson are value × (1 + `inventorMarkup`):
+0 / +15 / +50 / +100 % (cozy…challenge, `Difficulty`), −10 % on his Sunday event, −15 % with LL Pillar.

**Random shapes** (Jackson, Rev raider, crates and dump spawn `random_*`, `PreBuiltItemHelper.Set*ModuleShape`):
P/E/Q: L or T (4 cells, value ×0.92), **C (3 cells, stats ×0.75, value ×0.85)** or square, ¼ each.
Overclock/eco/fineness: ⅔ get a 5-cell shape with **stats ×1.25** (overclock +45/−55, eco −20/+40,
fineness −10/−18/+31, value ×1.15), ⅓ square. Truncated to integers.

### 2.2 Machine-specific modules (1×4)

| id | value | P / E / Q | Machine: effect | Sold |
|---|---|---|---|---|
| `furnace_module_blast` | 150 | 0 / −100 / 0 | furnace: 3 inputs → 2 ingots, flux level −1, draw 16 | Jackson 20 %, crate 15 % |
| `furnace_module_junk` | 120 | 0 | furnace: idle night 6 junk → 4 scrap | Jackson 20 % |
| `alarm_module_transmitter` | 100 | 0 / −50 / 0 | alarm: **+1 rep a night** (× difficulty gain: 0.75 hard) with its faction; click cycles Sec → BM → UL → Rev; one flag per faction; only the first alarm's bay counts; the desequencer bay accepts it but ignores it (`MachineAlarm.Operate`) | Jackson 20 % |
| `farm_module_nutrifruit` | 150 | −10 / −10 / 0 | +1 nutrifruit per harvest | Jackson 25 % (Retired Farmer) |
| `farm_module_dream_cap` | 150 | +15 / +15 / 0 | dream caps: +2 per harvest, edible, contraband stripped | same |
| `farm_module_nightmare_cap` (contraband 3, 1×2) | 150 | −33 / −50 / 0 | dream caps → nightmare caps (unless the neutralizer is also in) | same |
| `farm_module_kotton_loom` | 200 | −50 / −100 / 0 | kotton → kotton fabric | same |
| `farm_module_fillerweed_production` | 115 | 0 / −25 / 0 | +4 fillerweed, no seeds back | same |
| `farm_module_hydroreed_filter` | 105 | 0 / −15 / 0 | +1 hydroreed filter per harvest **per module** | same |
| `farm_module_smuggler` (1×2) | 100 | 0 | strips contraband from the growing seed only | same |
| `evaporator_module_solvent_recovery` | 275 | 0 / −25 / +50 | evaporator: distillate keeps only alcohol | Retired Chemist bundle |
| `printer_module_metal` / `_cnc` / `_melee_batch` | 300 / 400 / 250 | 0/−50/0, −25/−25/0, −50/−50/0 | printer recipe gates (`RECIPES.md` §1.2) | Retired Gunsmith |
| `dna_manipulator_module_stabilizer` | 500 | −50 / −25 / 0 | extraction keeps the seed (removes the rank instead) | Jackson 25 % (Plant DNA) |
| `..._positive_filter` / `_negative_filter` | 500 | 0 / −25 / 0 | extract only beneficial / only harmful traits | same |
| `..._precision` | 1000 | −75 / −75 / 0 | extract one chosen trait | same |
| `chem_module` (2×2) | 250 | — | blender/finisher/plate, count 0–4 (`CHEMISTRY.md`) | Jackson with Retired Chemist |

### 2.3 Random effects (`ModuleEffectHelper`)

Jackson's and other random modules roll 0–2 effects, **only from day 21** (always with skip-intro starts);
nodes always get Support. Effects act on the module's own base stats:

| Effect | Rule |
|---|---|
| Premium / Inferior | stats ×1.2 / ×0.8, value ±40 |
| Top mount / Side mount | +20 % while touching the bay's top / left edge |
| Receiver | +10 % per adjacent node |
| Negative feedback | stats ×1.25, then adds ¼ of the adjacent modules' negative stats to itself |
| Overvolted | stats ×2; **each working night 10 % to turn into a ruined module** |
| Degrading | stats ×2; each working night −1 on every positive stat (negatives stay) |
| Learning algorithm | positive stats start at 0 and gain +1 per working night up to 2× base |

"Working night" = each `OnMachineActioned` call: a night the machine did its job, or a turbo boost. **The security
alarm calls it every night in its Early pass whether or not it has power**, so an unpowered alarm is a free trainer
for learning modules (lab: a learning fineness went 0 → 50 in 50 nights, `FormulaExps.LearningAlarm`) and a
nightly 10 % ruin roll for overvolted ones. The learned stats are stored on the module and kept when it moves. About
8.8 % of random modules roll the learning effect (`ModuleEffectHelper.InitRandomEffect`; read).

## 3. Per machine

`base` = unmodded draw per working night. Breakpoints in **bold** were measured **[lab]**.

| Machine (value) | Bay types | Base power / speed | P does | E does | Q does | Breakpoints that matter |
|---|---|---|---|---|---|---|
| Hydroponic bay (300) | universal, hydroponic | 8 / 8, P ratio 2 | speed: 9 at P≥25, 10 at 50, 11 at 75, **12 at 100**, 16 at 200 | draw | harvest count `round½↑(base·(1 + ⌊Q/2⌋/100))` (`HydroponicHelper.GetHarvestCount`); also nutrifruit size split | bloomberry (work 22/12, PLANTS.md): **every night at P≥100** (first harvest night 2), else every 2. Berries 6 → **7 at Q≥18**, **8 at Q≥50**, **9 at Q≥84**, 10 at Q≥118 |
| Wine rack (350) | universal | 24 | lowers draw (÷2) | lowers draw (÷2) | extra aging steps `1 + Q/125` per night | **Q≥125: 3 steps a night** (own tick + 2), else 2 |
| Furnace (200) | universal, furnace | 8 per smelt | — | draw | flux level `Q/100` (−1 with blast) | Q≥100 replaces flux; Q≥200 +1 purity (no blast) |
| Water purifier (250) | universal | 4 (only nights it removes) | budget × (dirt bonus + P/100) | draw | purity cap 99.01 → 99.16 (Q≥15), 99.36 (25), 99.65 (50), 99.80 (75), **99.90 = Pure (Q≥100)**, 100 (200) (`GetMaxPurityCap`) | Q≥100 for Pure |
| Moisture farm (350) | universal | 6 | volume 850 ml × (1 + P/100) | draw | grade of Q+50: ghost; **base at Q≥50, HQ at Q≥100**, pure at 150 | ÷ (1 + 0.25 × other farms running) |
| Evaporator (300) | universal, evaporator (4×4) | 6 | volume 300 ml × (1 + P/100) | draw | distillate % = Q (clamped 0–100) | Q≥100 recovers all |
| 3D printer (350) | universal, printer | 8 / 24 | speed `24 + trunc(24P/100)` | draw | melee condition roll +Q/4; printed gun and frame stats ×(100+Q/4) % | per-chip nights (guide §Printer) |
| Desequencer (350) | universal, alarm | 10 / 33 | speed | draw | — | work 75: 3→2 nights at P≥16; 100: 4→3 at P≥4; 125: 4→3 at P≥28; 150: 5→4 at P≥16, →3 at P≥52 |
| DNA manipulator (1000) | universal, DNA | 12 / 10, P ratio 2 | speed (11 at P≥20) | draw | success +Q/2 (`PlantGeneticsHelper.GetSuccessChance`) | op work 20 + 4×complexity |
| Mirage projector (400) | universal | 16 | attractiveness +P/4 | draw | attractiveness +Q/2 | boost `100 + Q/2 + P/4` (`GetAttractivenessBoost`); **capped core → 150 → 400 total** |
| Security alarm (150) | universal, alarm | 4 (early pass) | burglary stop % = 50 + P | draw | — | **P≥50: every burglary stopped** (`RNG.Roll(stop + 1)`) |
| Deal maker (400) | universal | 25 per press | lowers draw | lowers draw | bargain +Q/10 % | no source in code |
| Blender / finisher / plate | chem only | none | — | — | count of chem modules | `CHEMISTRY.md` |

Machines with **no module bay** (upgraded with materials instead, `ModuleHelper.CanElectronicPartTarget`,
`MaterialDirectory.CanIngotTarget`): fridge (electronic −2 draw, scrap +storage), RAS (electronic −2 draw,
scrap/ingot size), vending machine (electronic +5 attractiveness to 100, ingot capacity), bottle printer
(electronic, sizes), current converter (electronic −0.1 rate), hypertech cell (hypertech electronic +10
output), metal scanner (electronic +5 % double loot), recharger, cassette player, auto butcher.

### 3.1 Projector and attractiveness

`PlayerStore.GetCurrentStoreAttractiveness` = 250 (+150 per renovation) − trash penalty + projector
boost (reset every night, set by a powered projector). Thresholds: ≥ 200 / 250 / 500 +1 normal
customer each; ≥ 400 client pool tier 1 and display buyers promoted, ≥ 650 tier 2
(`StoreClientManager.GetAttractivenessTier`); client **budgets** +10 % at ≥ 300, +25 % at 450,
+50 % at 600, +75 % at 750, +100 % at 900 (`StoreClient.ApplyBudgetModifier`). An unmodded projector
(350) already gives +10 % budgets. Reaching 400 needs boost 150: capped core alone, or 5 fineness on a
kit-1 bay (≈152), or four 5-cell fineness (≈152). Reaching 450 from base 250 needs boost 200 (uncapped
core, or capped core + 5 fineness).

### 3.2 Power sources (battery slot)

| Source (value) | Behaviour | Evidence |
|---|---|---|
| `energy_credit` (10 + charge) | 100 charge; ~1 credit per unit when sold | `MiscItemDirectory.EnergyCredit`, `PowerHelper.GetPowerSourceItemValue` |
| `energy_credit_ext` (30 + charge) | 250 charge | |
| `energy_credit_breeder` (400 + charge) | 100 charge, **+4 every night in the early pass**, before machines draw: runs a ≤4-draw machine (purifier, bare alarm) forever | `EnergyCreditBreeder` |
| `energy_credit_dis` (contraband 1) | 1000 charge, never recharges, sells at 33 % of charge | `DisposableBattery` |
| `recharger` (150) | +3 a night (6 with Commercial Power, 350) to every cell inside, only with store power (service or Power Hijack) | `MachineRecharger.Recharger` |
| `current_converter` (300) | draws cash: 1.5 credits per unit (−0.1 per common electronic, 5 max → 1.0; Power Hijack −15/25/50 %); at night it is billed as the converter service; blackouts stop it | `CurrentConverterHelper` |
| `energy_credit_hyp` (2500, hypertech smuggler) | free and endless, but **only machines drawing ≤ 12** (+10 per hypertech electronic) run on it | `HypertechBatteryHelper.CanSupply` |

### 3.3 Turbo boosters

`turbo_booster` (35, single use), `turbo_booster_adv` (300; 5 a night from its own cell to 15, so one
boost per 3 nights) and the hypertech one run a machine's early + main night callback on demand:
one more smelt, one more growth step (a whole bloomberry harvest on a P≥100 bay), one more aging step
of the rack. Printers refuse boosters as inputs (`MachinePrinter.RefuseTurboBoosters`).

## 4. Loadouts (computed with the formulas above; bold = measured)

Hydroponic, bloomberry (berries/night, draw/night): bare **3, 8**; 3 quality **3.5, 8**; 1 fineness
**3.5, 9** (first harvest a night later); 2 overclock + 2 performance 6, 15; 2 five-cell overclock +
1 performance 6, 16; **3 overclock 6, 18**; **3 overclock + fineness 7, 19**; kit + 3 overclock +
performance + 2 fineness 8, 20; **capped core 8, 16**; **capped core + overclock + 2 fineness 9, 21**;
2 overclock + 4 support nodes 6, 17 (**speed 12, draw 17**); 4 efficiency 3, 3; **4 eco 2, 1**.

Wine rack (draw): bare **24**; 4 performance **17**; 4 efficiency **16**; 4 eco 15; kit + 6 efficiency
12; **capped core + 3 fineness 32 for 3 steps a night**; kit + 5 fineness 37 for 3 steps.

Furnace with blast (draw per smelt): **16**; + 2 eco **10**; + 4 eco on a kit bay 6.

Moisture farm (ml, grade, draw): bare **850 ghost 6**; **2 fineness 714 base 7**; **4 fineness 578 HQ 9**;
4 overclock 2074 ghost 16; **capped core + 2 fineness 1564 HQ 13**.

Projector (boost, draw): bare **100, 16**; **4 fineness 142, 25**; **4 overclock 136, 44**;
**capped core 150, 32**; **capped core + 3 fineness 181, 39**.

Alarm (stop %, draw): bare 50, 4; overclock + performance 100, 5; **2 overclock 122, 7**;
transmitter + overclock + performance 100, 7.

## 5. What the gym executor does today, and levers worth adding

Today (`sim/gym`): the furnace engine installs `furnace_module_junk` and `furnace_module_blast` when
it owns them (`Furnace.cs`, junk first unless `blast_first`); nothing else ever gets a module. Bays,
racks, purifiers and the projector run bare; no alarm, no transmitter. Insurance is toggled by
`PlayerRole.Services` (`ev` mode: on when stock × 0.35 > the fee).

| Lever (proposed directive) | What it does | Expected effect | Priority |
|---|---|---|---|
| `alarm: perfect` | Buy the alarm (66 %/visit until owned), fit overclock + performance (or 2 overclocks, or P ≥ 50 any way), keep it powered, then drop insurance for good | Burglary stopped every night; saves the insurance fee, ×2 on hard: 2 × (stock estimate / 400 + 3) a night (26/night at 4k stock) for ~5 power | **1** (overhead, not production — PROGRESS.md says overheads and trade bind on hard) |
| `transmitters: [sec, …]` | Transmitter(s) in the alarm bay, one per faction (kit 1 fits 4 long + 2 squares) | +0.75 rep a night per faction on hard; the Security race (H17) | 2 |
| `jackson_module_intro: buy` | Buy the once-only P/E/Q offer at the first-time discount (−35 % hard, −75 % normal) | three modules for ~140 on hard | 2 (cheap) |
| `bay_modules: nightly` | Before planting a bloomberry, fit P ≥ 100 (2 overclock + 2 performance, 2 five-cell overclock + performance, 3 overclock, 2 overclock + 4 nodes); add Q ≥ 18 when room | 3 → 6 (to 9) berries a night per bay | 3 (only if berries bind the wine engine; PROGRESS.md: on hard production does not bind) |
| `projector_modules: tier1` | Capped core (or 5 fineness on a kit bay) in the projector | attractiveness 350 → 400: tier-1 client pool; with renovations reaches the 450/600 budget steps | 3 (untested; measure) |
| `rack_modules: efficiency` | 4 efficiency/eco in the rack, or no rack below Reserve wine | bare rack loses money on Production wine (6 × 3.75 = 22.5 < 24 draw) | 4 |
| `furnace_eco` | blast + 2 eco | 16 → 10 per smelt | 5 |

Built as `sim/gym/Modules.cs` (all but `rack_modules`). Measured on the hard appraisal incumbent (docs/PROGRESS.md,
"Machine modules and the wound rest"): `alarm: perfect` adds ~7k net worth by day 150 (the insurance it skips), a
Security transmitter beside it reaches Security 100 by day ~115 for about a third of that saving, and the module
introduction, bay modules, projector modules and furnace eco do not pay.

## Verification

**[lab]** rows were measured on the installed game's own code headless, with a scratch harness built on
the gym (a snapshot of `sim/lab`'s binaries; nothing added to `sim/`), seeds 2000001–2000005, 2026-11-10:
twelve hydroponic loadouts over 12 nights (harvest nights, counts, draw), bay fits with 0–2 kits,
the furnace blast + 2 eco draw (10 per smelt, two Very high ingots from 3 ore), projector boosts and a
live night (attractiveness 400 with a capped core), moisture-farm volume and grade, purifier caps,
printer/desequencer/DNA speeds, alarm stop rate, and the rack (bare: age 0 → 8 in 4 nights, 96 power;
capped core + 3 fineness: 0 → 12, 128 power; stock: 0 → 4). Every prediction above matched.
