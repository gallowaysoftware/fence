# Chemistry, drugs and the lab (RE notes)

Build 26525552 / 0.46D. Same conventions as `WATER.md` / `PRODUCTION.md`: **VISIBLE** = rendered by
the UI, **HIDDEN** = internal only, **DERIVABLE** = computable from visible facts. Types are in the
global namespace of `Assembly-CSharp`. Numbers come from the code; the ones marked **(lab)** were also
measured headless on the game's own machines (`sim/lab/ChemExps.cs`, verdicts in
`docs/sim/VERIFIED.md` § Chemistry). Tables extracted from the install: `var/gamedata/chemistry.json`
(liquids, finisher recipes, grade modifiers, item compositions, supply bottles, unpack contents, module
effects, heating and cooling curves) and `var/gamedata/recipes.json`; regenerate with
`dotnet sim/extract/bin/Release/net10.0/Fence.Extract.dll chem`.

Water is the same liquid model (`WATER.md` §1); this file covers everything else that happens in a
container. Other recipes (printer chips, food, crafting) are in `RECIPES.md`.

## 0. The short version

- A container holds **36 liquids**, each an integer amount in µL (`Liquid.Liquids`). Drugs are made by
  turning one liquid into another in a container, removing everything that is not the active
  ingredient, drying it, and pressing it into units in the chem finisher. **Purity = active µL /
  total µL of the unit**, and the unit's grade (7 bands) sets its price (−95 % … +135 %).
- Every reaction runs **once per second on every container**, wherever it is (shelf, plate,
  evaporator), in real time while the store is open and for **360 simulated seconds every night**
  (`ChemicalHelper.RunNightlyChemicalSimulation`, called from `PlayerStore` before machines run).
  Temperature only moves on a heating plate; off the plate a container cools toward 26 °C.
- Inside the lab, three things remove a liquid from a container: a **reaction** (turns it into something else,
  often `debris`), a **filter** on a pour (removes large particles), and **evaporation** (evaporator overnight,
  or boiling above the liquid's boiling point). Water tools also remove them (the purifier, portable purifier,
  purifying plants, RAS filters and UV turn contaminants into water; `docs/research/SOURCES-SINKS.md` §1) and the
  finisher draws them. Everything with particle size 5 that is not evaporative
  (protein, fat, starch, acid, base, reducing agent, chemical contaminant, heavy metal…) can only leave by
  reaction — that is why purification is chemistry and not filtering.
- The money is in **lab-pure dream dust** (≈ 5.5 units per dream cap with 4 modules everywhere, 42 base,
  ≈ 72 at market with the contraband markup) and, legally, **caffeine pills** from coffee beans (≈ 8 units
  of 24 per bean, contraband 0). Phanta pills are
  high-volume but must stay **below 75 % purity** or they are toxic.
- The three traps that ruin a batch: (1) leaving extract on an **unmodded plate overnight** (fixed 99 °C,
  360 s → all dream extract destroyed, lab); (2) setting the plate target exactly at a window's floor (the
  container settles ≈ 0.2 °C *below* target, so a 40 °C target extracts **nothing**, lab); (3) adding
  lipobond to a container still above 55 °C (it boils away before it binds fat, lab).

```
 dream cap ──blender──► mush (+water, protein, mineral, microbe, physical)
   + alcohol ≥5× mush, 40–50 °C ──► extract 50 % / plant matter 50 %   (≥50 °C mush rots, ≥60 °C extract dies)
   + acid = protein ──► debris          (alcohol ≥40 % of the mix has already killed the microbes)
   + base = leftover acid ──► nothing   (neutralisation leaves no salt)
   pour through kotton (≥150) then water filter (≥22) ──► plant matter, debris, physical, mineral gone
   evaporator, 300 ml/night ──► dry (≤100 µL water+alcohol+lipobond)
   chem finisher (2 ml per unit, −12 % at 0 modules … 0 % at 4) ──► dream dust, graded by purity
```

## 1. The liquid model

Storage, purity of water and grades of water are in `WATER.md` §1. For chemistry:

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Amounts | integer µL per liquid on the container (`CURRENT_PART_<ID>` tags); also on chemical items (caps, pills) | VISIBLE with the composition spectrometer (`chem_scanner`, 400): every liquid in µL and % of total, total volume, toxicity (lethal ×doses / toxic), euphoria level, synaprest and birth-control flags; for containers also with the AquaScan (200, composition only). §4.1 | `ChemScannerUIManager`, `WaterScannerUIManager`, `ChemicalHelper.InitChemItem` |
| Volumes otherwise | every liquid container's tooltip shows a fill bar and "current / capacity" in whole ml (rounded): the graduated-or-Experienced-Chemist branch is always taken because `StartingPerk.IsExperiencedChemist` returns true unconditionally in this build. Below 1 ml the tooltip cannot resolve it (use a scanner) | VISIBLE | `WaterHelper.CreateWaterContainerTooltip`, `StartingPerk.IsExperiencedChemist` |
| Pouring | `WaterHelper.TransferLiquid`: moves `min(target free space, source total)`, every liquid **in proportion**. There is no "pour X ml": you measure by pouring into a dropper/cylinder (it fills to its capacity) and then pouring that | VISIBLE | `TransferLiquid`, `SplitProportionally` |
| Filtering | a pour **into a filter pitcher** with a filter seated: each liquid whose particle size ≥ the filter's size is removed down to `filterMinimumPurity` × **that liquid's own poured amount**, in chunks of `filterVolumeRemoved` µL each costing `filterDurabilityUsage` durability; a filter at 0 durability removes nothing (the pour still happens). Microbes always pass (except the UV filter, which removes only microbes) | VISIBLE (result) | `WaterHelper.HandleFilter` |
| Temperature | `CONTAINER_TEMP_TAG` (°C, double). New/empty containers 24; mixing is volume-weighted (liquid added by `AddLiquid`, e.g. blending, dissolving, pouring a bottle of spirit, arrives at 26 °C); off a plate it decays toward 26 °C each second by `exp(−6.25/(√µL + 11))` | VISIBLE only as the plate's readout: whole °C, **truncated**, only while the plate window is open with the container in its slot. No tooltip, scanner or other UI shows it | `ChemicalHelper.GetCooledTemp`, `HandleReceiveLiquidTemp`, `MachineHeatingPlate` tick |
| Dissolving | soluble chem items (every chem product, water and nutrient tablets, feces) can be targeted onto a container; item and container must both be owned. The whole composition is added; if it overflows, the container first loses the excess in proportion | VISIBLE | `ChemicalHelper.InitChemItem`, `DissolveInto` |
| Blending | blendable items in the blender: each liquid × (1 − loss), rounded, poured into the slotted container, item destroyed. Overflow is trimmed **proportionally from the whole container** (old contents included) | VISIBLE | `ChemicalHelper.BlendInto`, `WaterHelper.RemoveExcessLiquid` |
| "Wet" | > 100 µL of evaporative liquids (water, alcohol, lipobond) | VISIBLE (finisher status) | `WaterHelper.IsWet` |

### 1.1 The 36 liquids

Particle size decides filtering: **kotton fabric** (size 150, durability 1000, 65) stops ≥150; the
**water filter** / advanced filter / hydroreed filter (size 22) stop ≥22 — both remove the liquid
completely except mineral, organic waste and physical contaminant, which keep 0.16 % of their own poured
amount, and microplastic, 0.2 % (lab: 2.5 ml mineral → 0.004 ml). Lethality and euphoria are per µL (§5).
Every liquid except water is `canBeRemovedByPurifier`: the water purifier turns **any** non-water liquid,
the active ingredients included, into water (largest particles first), so a batch must never go in it
(`MachinePurifier.RemoveLargestFirst`).

| Liquid | Particle | Removed by | Boils at °C | Lethality | Euphoria | Where it comes from |
|---|---|---|---|---|---|---|
| `water` | 10 | — | 100 | | | everything wet |
| `heavy_metal` | 12 | — (purifier) | | | | 2–8 % impurity of every supply bottle |
| `organic_waste` | 40 | water filter | | | | feces, food, rust water |
| `microplastic` | 25 | water filter | | | | nothing in the code adds it |
| `microbe` | 50 | never filtered (UV only) | | | | caps, beans, water |
| `chemical_contaminant` | 2 | — (chlorine, purifier) | | | | supply-bottle impurity |
| `physical_contaminant` | 100 | water filter (not kotton) | | | | caps, beans; **the finisher's impurity** |
| `mineral` | 30 | water filter (not kotton) | | | | caps, water |
| `dream_cap_mush` | 200 | both | | | 0.04 | blended dream cap |
| `dream_cap_extract` | 5 | — | | | 1.0 | extraction |
| `plant_matter` | 200 | both | | | | extraction by-product, degraded mush |
| `alcohol` | 10 | — | 78.37 | | | mini alcohol bottle (70 %); nudka poured on a container (750 ml: 45 % alcohol + base water, `WaterHelper.PourNudka`); wine poured (20 %) |
| `caffeine` | 5 | — | | | | coffee beans |
| `protein` | 5 | — | | | | caps, beans, fruit |
| `fat` | 5 | — | | | | coffee beans |
| `starch` | 5 | — | | | | nutrifruit (17.3 % of its weight); the street "cut" |
| `sugar` | 5 | — | | | | fruit |
| `coffee_mush` | 200 | both | | | | blended coffee bean |
| `contraceptive` | 5 | — | | | | birth-control pills only |
| `zyanide` | 5 | — | | 1.66 | | cyanide pills only |
| `nightmare_extract` | 5 | — | | 4.0 | 0.25 | extraction |
| `nightmare_cap_mush` | 200 | both | | 0.6 | 0.1 | blended nightmare cap |
| `chlorine` | 5 | — | | | | water tablet (15 ml) |
| `synaprest` | 5 | — | | | | (no recipe) |
| `fanta` (Phanta) | 5 | — | | 2.0 | 10.0 | methylol reduction |
| `methylol` (N-methylol base) | 5 | — | | | | restricted supplies |
| `reducing_agent` | 5 | — | | | | restricted supplies |
| `acid` | 5 | — | | 0.1 | | common supplies |
| `base` | 5 | — | | 0.1 | | common supplies |
| `stabilizing_agent` | 5 | — | | | | comes with methylol |
| `debris` | 200 | both | | | | every precipitation/sedimentation |
| `lipobond_agent` | 5 | — | **55** | | | common supplies |
| `livestock_steroid`, `livestock_immunity` | 5 | — | | | | red / green serum (100 each), injected 20 µL into an animal |
| `wine_extract` | 5 | — | | | | pouring a wine bottle into a container (4 ‰ × quality tier 0–5 of 750 ml, so 0–15 ml; a counterfeit gives 8 ‰ plus 8 ‰ chemical contaminant; `WineHelper.PourWineIntoContainer`); no use in chemistry itself, but the same pour adds 150 ml of alcohol (20 % of 750 ml), an alcohol source the evaporator's recovery module can distil |
| `nomore` | 5 | — | | | | dream extract reduction |

No reaction produces zyanide, contraceptive, synaprest or wine extract: cyanide and birth-control pills
can only be bought and re-dissolved; synaprest exists in no item.

## 2. Reactions (`ChemicalHelper.HandleTemperatureReaction`, every second)

The handlers run in this order each second (the sum expression is evaluated left to right): microbe
heat, microbe alcohol, organic-waste heat, dream-cap extraction, nightmare-cap extraction, coffee
extraction (solvent, then water), nomore, dream extract degradation, dream mush degradation, nightmare
extract/mush degradation, organic mineralisation, acid/base microbe kill, stabiliser sedimentation,
protein precipitation, lipobond sedimentation, acid/base neutralisation, Phanta heat, methylol
reduction, chlorine, boiling. "Min X" means at least X µL move per second (so small amounts finish in
one tick).

| # | Reaction | Inputs | Condition | Per second | Output | Evidence |
|---|---|---|---|---|---|---|
| R1 | **Dream cap extraction** | `dream_cap_mush` + `alcohol` | T ≥ 40 °C | processes max(2 ml, 10 %) of the mush; fraction to extract = 0.5 × min(1, alcohol / **mush still present** / 5) | extract, rest → plant matter. Alcohol is not consumed | `HandleDreamCapMushReaction` |
| R2 | Nightmare cap extraction | `nightmare_cap_mush` + alcohol | T ≥ 40 | same as R1 | nightmare extract / plant matter | `HandleNightmareCapMushReaction` |
| R3 | Dream mush rot | mush | T ≥ 50 | 1 % per °C above 50 (10 % at 60, 19.9 % at 69.9), **drops to 10 % at 70**, then 10→90 % over 70–100, 90→100 % over 100–120 | plant matter | `HandleDreamCapMushDegradation` |
| R4 | **Dream extract destruction** | extract | T ≥ 60 | 1 % per °C above 60, 10→90 % over 70–100, 90→100 % over 100–120 | destroyed (vanishes) | `HandleDreamCapExtractDegradation` |
| R5 | Nightmare extract destruction | nightmare extract | T ≥ 50 | **5 % per °C** above 50 (the divisor is 2, not 10: 45 % at 59 °C, 50 % at 60, 99.5 % at 69.9), drops to 10 % at 70, then as R4 | destroyed | `HandleNightmareCapExtractDegradation` |
| R6 | Nightmare mush rot | nightmare mush | T ≥ 50 | as R3 | plant matter | `HandleNightmareCapMushDegradation` |
| R7 | **Coffee extraction, solvent** | `coffee_mush` + alcohol | T ≥ 50 | processes max(1 µL, 15 %) of the mush; fraction to caffeine = 0.75 × min(1, alcohol / **this second's chunk** / 5) | caffeine / plant matter | `HandleCoffeeBeanExtractionSolvent` |
| R8 | Coffee extraction, water | coffee mush + water | T ≥ 80 | 5 % of the mush; fraction 0.125 × min(1, water/chunk/5) | caffeine / plant matter | `HandleCoffeeBeanExtractionWater` |
| R9 | **Nomore** | dream extract + reducing agent + lipobond (all > 0) | 45 ≤ T ≤ 55 **and** alcohol ≤ 10 % of total | min(extract, reducing) × (5 % below 50 °C, 10 % at 50+) × min(1, 4 × lipobond / extract), at least min(that, 0.1 ml) | 60 % nomore, 40 % debris; extract and reducing consumed 1:1; **lipobond not consumed** | `HandleNomoreReaction` |
| R10 | **Protein precipitation** | protein + **net** acid or base (acid − base or base − acid) | any T | 20 % of min(net excess, protein) (min 1 µL) | debris; the net excess is consumed 1:1 | `HandleProteinPrecipitation` |
| R11 | **Lipobond sedimentation** | `fat` + lipobond | any T | 20 % of min(fat, 8 × lipobond) (min 1 ml) | debris (fat + lipobond); 1 lipobond per 8 fat | `HandleLipobondAgentSedimentation` |
| R12 | Stabiliser sedimentation | `stabilizing_agent` + acid and/or base | any T | 20 % of min(acid + base, stabiliser) (min 1 ml) → debris; acid and base each lose max(0.1 ml, 5 %) | debris | `HandleStabilizingAgentSedimentation` |
| R13 | Acid/base neutralisation | acid + base | any T | 10 % of the smaller (min 1 ml) of each | nothing (both vanish; no water, no salt) | `HandleAcidBaseNeutralization` |
| R14 | **Methylol reduction (Phanta)** | methylol in **excess of the stabiliser** + reducing agent | 110 ≤ T < 120: 2.5 %; 120 ≤ T < 130: 15 %; both min 1 ml, of min(methylol − stabiliser, reducing) | methylol consumed, reducing −0.75×, **Phanta +0.1×** | nothing happens while stabiliser ≥ methylol | `HandleMethylolHeatReaction` |
| R15 | Phanta heat damage | fanta | T ≥ 130 | −2 % | destroyed | `HandleFantaHeat` |
| R16 | Microbe heat | microbe | ≥ 70: −20 %; ≥ 100: all | | killed | `HandleMicrobeHeat` |
| R17 | Microbe alcohol | microbe | alcohol ≥ 40 % of total | kills up to the alcohol volume (all, in practice) | killed | `HandleMicrobeAlcohol` |
| R18 | Acid/base microbe kill | microbe + acid/base | any T | kills up to 20 × (acid + base); consumes 1 acid/base per 100 killed | killed | `HandleAcidBaseMicrobeKill` |
| R19 | Organic mineralisation | organic waste + acid/base | any T | up to 40 × (acid + base) organic → 10 % mineral; 1 acid/base per 40 | mineral | `HandleOrganicWasteMineralization` |
| R20 | Organic heat | organic waste | ≥ 80: 15 %; ≥ 100: all | 33 % of it → physical contaminant | | `HandleOrganicWasteHeat` |
| R21 | Chlorine | chlorine (≤ 15 ml used per s) + water present | any T | potency 5.1 µL per µL: first chemical contaminant above **0.7 % of total volume**, then microbes; leftover chlorine decays max(1 ml, 20 %) | | `HandleChlorine` |
| R22 | Boiling | every evaporative liquid | T > its boiling point | −max(0.1 ml, 2 %) | evaporated (lost, not collected) | `HandleBoilingReaction` |

Measured (lab, `chem.extraction*`, `chem.proteinfat`):

| Test | Result |
|---|---|
| 4 caps, alcohol 6× mush, plate target held 180 s at 35 / **40** / 45 / 49 / 50 / 55 / 60 / 65 / 70 °C | extract 0 / **0** / 50 / 50 / 50 / 45 / 41 / 0 / 0 % of the mush. A 40 °C target settles at 39.9 °C and never starts R1; at 55 °C a tenth of the mush rots first; at 65 °C the extract dies as fast as it forms |
| Starting ratio alcohol : mush 0.5 / 1 / 2 / 3 / 4 / 5 / 6 / 8 at 45 °C | extract 15.8 / 25.3 / 37.4 / 44.5 / 48.4 / 50 / 50 / 50 % of the mush; extract per ml of alcohol 0.32 / 0.25 / 0.19 / 0.15 / 0.12 / 0.10 / 0.08 / 0.06 — the ratio is re-read against the mush still unreacted, so low ratios recover a lot |
| 10 ml protein + 5 / 10 / 15 ml acid | protein left 5 / 0 / 0 ml after 60 s (≈ 90 % gone by 10 s); acid left 0 / 0 / 5 — strictly 1:1 |
| 10 ml protein + 10 acid + 10 base together | protein untouched: equal acid and base give no net excess |
| 8 ml fat + 1 ml lipobond (1:8) / 2 ml (1:4) | fat 0 / 0, lipobond left 0 / 1 ml: exactly 1 per 8 |
| 20 ml acid + 12 ml base | 8 ml acid left after 20 s, base 0; no product |

**The extraction ratio, reconsidered.** The manual's "3–6× alcohol" maximises extract per *cap*. Per
ml of solvent (which the evaporator must remove at 300 ml a night, §3) a low ratio is 2.5× better:
ratio 1 yields 25 % of the mush but 0.25 ml extract per ml alcohol, ratio 5 yields 50 % but only 0.10.
Neither is the right question: **alcohol is not a reactant**, and the ratio is read against the mush still in
the vessel. Keep ≈ 5 × one cap's mush (110–120 ml) of alcohol in the vessel and blend caps into it one at a time,
20 s at 45 °C each: every cap converts at the full 50 %, and the same 120 ml serves 6 or 12 caps (lab,
`FormulaExps.DreamBatch`: 66 ml extract from 6 caps either way; 195 ml of solvent to evaporate instead of 1024, so
**1 evaporator night instead of 4**; 33 lab-pure units from the batched chain, `DreamBatchPure`). All 6 caps at once
in 120 ml give 23.8 %. CthuLuck's batch method (mush in 25 ml portions into 100 ml alcohol) is this rule. The binding
input becomes caps and plate time, not alcohol or evaporator nights.

## 3. Equipment

The Retired Chemist network upgrade (1000; 1 500 on hard) brings his weekly visit, which offers all four chem machines on his counter (`StoreClientListSpec.CreateRetiredChemist`); the chemist start (§7.3) has them from day 1. The
blender, heating plate and chem finisher take only `chem_module` (250) — **up to 4 each**, owned only, and a
seated chem module is marked stuck (`ModuleHelper.SetModuleStuck`) — and need no power; the evaporator
takes ordinary universal/evaporator modules and a battery. Every machine slot that takes a container
accepts only an owned liquid container. Values from
`var/gamedata/chemistry.json` → `module_effects` (code + extraction):

| Chem modules | 0 | 1 | 2 | 3 | 4 | Evidence |
|---|---|---|---|---|---|---|
| Blender loss (all liquids of each item) | 35 % | 20 % | 10 % | 5 % | 0 % | `MachineBlender.GetLoss` |
| Finisher impurity (share of each unit replaced by physical contaminant) | 12 % | 6 % | 3 % | 1 % | 0 % | `MachineChemFinisher.GetLoss` |
| Heating plate target | fixed **99 °C** | fixed **160 °C** | settable 26–160, ±5 per press | settable | settable | `MachineHeatingPlate` tick |
| Plate heating speed | ×1 | ×1 | ×1 | ×4 | ×16 | `HandleHeating` |

| Machine (id, value) | Panels | Behaviour | Real time vs night | Evidence |
|---|---|---|---|---|
| Blender (`blender`, 200) | modules 4×4, input grid 4×4 (any blendable item), container slot (owned liquid container) | Button (does nothing without a container or if it is full): each blendable item in the input poured into the slotted container (§1 Blending), checked for "full" before each item. Blendable (`BLENDABLE_TAG`): dream/nightmare/meal caps, coffee beans, nutrifruit, water and nutrient tablets, feces, and every chem product (dusts, pills, nomore injector). Tooltip shows the loss % | instant, button | `MachineBlender.Blender`, `CreateTooltip` |
| Heating plate (`heating_plate`, 250) | modules, container slot (owned), temperature readout, off/low/high, target − / + (shown only at ≥ 2 modules) | Each second, if not off and the container is not empty: T += (target − T) × f/350 + 1, capped at target (a hotter container is left alone); f = 10 low / 30 high, ×4 at 3 modules, ×16 at 4. The plate's own cooling bug: it marks the *machine*, not the container, as heating, so the container also cools every second → it settles a little below target (≈ 0.2 °C for 250 ml–3 L at 2 modules; lower for small volumes once water boils off). Target: 99 on a new plate; 0/1 modules force 99/160 every tick; at ≥ 2 the stored target is kept and each press moves it 5, clamped to 26…160, so the reachable values depend on where it starts (from 99: …44, 49, 54…124, 129…; after hitting 160: multiples of 5; after hitting 26: 31, 36, 41, 46…). The readout under the target shows the container's T as a truncated whole °C, so a container settled at 44.8 reads 44. Tooltip: max temperature (99 / 160) and slow / fast / very fast. No power | **per second, and 360 s every night** — a container left on it overnight sits at target for 6 minutes | `MachineHeatingPlate.HeatingPlate`, `HeatLiquidOnStove`, `CreateTooltip`; `ChemicalHelper.HandleAmbientTempDrop` |
| Evaporator (`evaporator`, 300) | battery, modules 4×4 (universal, evaporator), container, distillate slot, note | At cycle end (each night, and on a turbo boost: `TURBO_BOOST_ELIGIBLE_TAG`), if a battery is seated, the container holds any evaporative liquid and the battery pays the draw (base 6, modules change it): removes up to round(300 ml × (1 + P/100)) of the evaporative liquids (water, alcohol, **lipobond**) split in proportion to their amounts; nothing else leaves. Distillate: each removed liquid × Q % (Q clamped 0–100) is added to the distillate container if one is seated (else lost); with `evaporator_module_solvent_recovery` (275, Q +50, E −25) only the alcohol share is kept, water and lipobond are discarded. Q from universal modules recovers alcohol too, mixed with clean water: solvent + 2 fineness = Q 100 returns **all** the alcohol evaporated at 252 ml a night (lab, `FormulaExps.Distillate`). The distillate add caps each liquid at the container's capacity but not the total. No temperature effect | night (after the 360 s chemistry) and turbo | `MachineEvaporator.Evaporator`, `CanEvaporateContainer`; `MODULES.md` |
| Chem finisher (`chem_finisher`, 300) | modules, container slot (owned), output 6×4, Configure + Output buttons, target and status lines | Configure lists all 7 recipes (no unlocks), each with its name, required volume (ml, 2 decimals) and a "no wet" note. Status line, every second: no target / ready / wet ingredient (> 100 µL water + alcohol + lipobond) / not enough reagent (total < recipe volume). **No predicted purity or grade.** Output (if ready): draws the recipe volume from the container in proportion, removes loss % of the unit's volume (truncated) and adds the same amount of physical contaminant, places it in the output grid, then grades it. One press = one unit. The finisher never checks that the active ingredient is present: any dry residue can be pressed into any recipe. If the output grid has no room the unit is not placed anywhere (the return value is ignored) and its volume is gone (read, not lab-tested) | instant, button | `MachineChemFinisher.ChemFinisher`, `CanOutput`, `IntroduceImpurity`; `ChemFinisherUIManager.OpenUI`; `ChemFinisherRecipe.CanOutput` |
| Composition spectrometer (`chem_scanner`, 400) | — | Target any liquid container, chem item or chem product, owned or not; not consumed, no power, no ownership check on the scanner itself. Panel refreshes every second while open (§4.1) | instant | `MedsItemDirectory.ChemScanner`, `ChemScannerUIManager` |
| AquaScan (`aquascan`, 200) | — | Must be owned; targets liquid containers only (owned or not). Same per-liquid composition list (µL and %) as the spectrometer, plus water purity (tested vs label) and it stamps "last scan" water purity on the container's tooltip. No toxicity/euphoria lines, cannot read pills or dusts | instant | `AmenitiesItemDirectory.AquaScan`, `WaterScannerUIManager.RefreshUI`, `WaterHelper.SetLastScanResult` |
| Water test strip (3, single use) | — | Water share of a container only (`GetWaterPurity` = water µL / total) | instant | `WaterTestHelper.HandleWaterCheckTestingStrip` |
| Fridge (`fridge`, 100) | battery, chill zone 5×2, freeze zone 3×2 (+1 row per storage upgrade) | While the battery has charge, every second moves each non-empty liquid container in the chill zone 0.5 °C toward 3 °C, in the freeze zone 1 °C toward −18 °C, and (unlike the plate) flags the container so ambient drift is skipped. Every temperature-gated reaction stops; the room-temperature ones (R10–R13, R17–R19, R21) do not | per second and at night | `MachineFridge.Fridge`, `CoolZoneContainers` |
| Chemist storage bay (`storage_bay_chem`, 400) | 17×10 grid | Storage only: chem supplies, liquid containers, NARCOTIC / MEDICAL / water-purification items, chem products, both scanners, chem modules. **Not** a concealment bay: inspections search it (§7.5) | — | `ShipSystemDirectory.ChemistStorageBay` |
| Filter pitcher (`water_pitcher`, 3 L) + filter | — | Pour into it to filter (§1) | instant | `WaterHelper.HandleFilter` |
| Water purifier, moisture farm, UV, tablets | — | Water tools, `WATER.md` §1.5. The purifier destroys every chemical (§1.1) | | |

Heating curves measured on the game's own plate (water; seconds to reach; `chemistry.json` →
`heating_curves`):

| Plate | 40 °C | 50 °C | 60 °C | 99 °C | 120 °C | Settles at |
|---|---|---|---|---|---|---|
| 0 modules, low | 5 s | 9–10 s | 14–15 s | never | — | 98.1–98.7 (target 99) |
| 0 modules, high | 3 s | 4 s | 6–7 s | never | — | same |
| 1–2 modules, low (target 160) | 4 s | 6 s | 8–9 s | 21–24 s | 31–40 s | 128–155 (water boils off) |
| 3 modules, high | 1 s | 1 s | 1 s | 2 s | 3 s | 155+ |
| 4 modules | 1 s | 1 s | 1 s | 1–2 s | 1–2 s | 157–159 |

Cooling off the plate from 99 °C (`cooling_curves`): 250 ml → 61 °C after 60 s, 43 °C after 120 s, 27 °C
after 300 s; 1 L → 76 / 61 / 37 °C; 3 L → 85 / 73 / 51 °C. Big volumes hold a window for minutes.

**Practical temperature control without 2 modules:** the 0- and 1-module plates cannot hold a
window. Heat on low and lift the container out as soon as the readout passes the target (≈ 7 s to
45 °C on an unmodded plate, lab), then let it react on the shelf while it cools: 250 ml stays above 40 °C
for about 2 minutes. R1 at max(2 ml, 10 %) per second finishes a normal batch in under a minute (lab:
6 caps fully extracted this way). Never leave it on the plate when the day ends.

## 4. Purity, grades and value

`ChemicalProductHelper.GradeProduct`: purity = active µL / total µL of the unit (stored as a double,
`CHEMICAL_PRODUCT_PURITY_TAG`). The grade is a `CATEGORY_CHEMICAL_PURITY` feature with an innate % on
value (`ItemConditionList.Create*`; the chemistry manual prints the same table):

| Grade (rank) | Purity | Value | Dream dust (18) | Phanta pill (12) | Caffeine pill (10) | Nomore injector (50) |
|---|---|---|---|---|---|---|
| Lab pure (6) | ≥ 99 % | +135 % | **42** | 28 (lethal) | 24 | 117 |
| Pharma grade (5) | 90–99 | +85 % | 33 | 22 (toxic) | 18 | 92 |
| Near pharma (4) | 80–90 | +40 % | 25 | 17 (toxic) | 14 | 70 |
| Street standard (3) | 60–80 | 0 | 18 | **12** (60–74 % only) | 10 | 50 |
| Cut product (2) | 45–60 | −45 % | 10 | 7 | 6 | 28 |
| Junk grade (1) | 30–45 | −75 % | 4 | 3 | 3 | 13 |
| Starch grade (0) | < 30 | −95 % | 1 | 1 | 1 | 3 |

Dream-dust and Phanta rows measured (lab, `chem.finishergrades`); the others are base × modifier. The
grade a buyer sees is the **label** (the feature's fake condition) until exposed. A pressed unit is graded
once, when it leaves the finisher, with label = truth; nothing re-grades it later except the scanner's
Match. Bought products come "street standard" (active 60–80 % of the recipe volume, rolled; the rest split
at random between physical contaminant, chemical contaminant and starch —
`ChemFinisherRecipe.SetCompositionStreetStandard`), except the shop-bought nomore injector, rolled at
**45–60 % (always cut product)** (`MedsItemDirectory.SetNomoreInjectorPurity`), and the cyanide pill, whose
zyanide is reset to 1 ml after grading (so its stored grade describes a 0.6 ml roll, not its contents).

| Unit | Draw per unit | Active needed for lab pure | Finisher cap at 0 / 1 / 2 / 3 / 4 modules (pure input) |
|---|---|---|---|
| Dream dust | 2 ml | 1.98 ml | 88 / 94 / 97 / 99 / 100 % (lab) → near pharma / pharma / pharma / lab pure (only from a 100 % residue) / lab pure |
| Nightmare dust | 4 ml | 3.96 ml | same caps |
| Caffeine pill | 5 ml | 4.95 ml | same |
| Phanta pill | 0.5 ml | — (stay < 75 %) | same |
| Nomore injector (`pink_alt_injector`) | 4 ml | 3.96 ml | same |
| Cyanide pill | 0.6 ml | — | same |
| Birth-control pill | 0.1 ml | — | same |

**Lab pure in practice needs 4 finisher modules and a ≥ 99 % dry residue** (the unit's purity is residue ×
0.99 with 3 modules, so 3 only works on a perfectly clean residue; with 4 it is the residue as is). With 0 modules nothing beats near pharma — CthuLuck's 87 % first batch is exactly a
≈ 99 % residue × 0.88.

**Do not cut for value.** Per ml of active ingredient a dream-dust unit is worth 21.2 at lab pure,
18.3 at pharma, 15.6 at near pharma, 15.0 at street and 11 at cut: purity always pays. Cutting (adding
starch, e.g. dried nutrifruit) only stretches a count — useful for a cartel quota that names a grade,
not a price (§7). The exception is Phanta: 60 % street pills earn 40 per ml of Phanta, 74 % pills 32, so
cut Phanta to just above 60 %.

### 4.1 Player-visible vs hidden

What an executor limited to the player's view can read, and with what:

| Quantity | Where it is visible | Needs | Resolution / limits | Evidence |
|---|---|---|---|---|
| Container volume | item tooltip, every container | nothing | whole ml (rounded) + fill bar; capacity | `WaterHelper.CreateWaterContainerTooltip` |
| Container composition (every liquid) | spectrometer or AquaScan panel | either scanner (400 / 200), no power, not consumed; AquaScan must be owned | per liquid: exact µL below 1 ml, 0.01 ml (truncated) from 1 ml, 0.01 L from 1 L; % of total to 2 decimals; total volume. Live (refreshes each second while open) | `ChemScannerUIManager.RefreshUI`, `WaterScannerUIManager.FormatAmount` |
| Toxic / lethal (×doses), euphoria level, contraceptive, anti-addiction flags | spectrometer only (containers and items) | `chem_scanner` | thresholds §5 | `ChemScannerUIManager.RefreshUI` |
| Water share | test strip, AquaScan, container tooltip after an AquaScan ("last scan") | strip (3, single use) or AquaScan | water µL / total, 2 decimals | `WaterHelper.GetWaterPurity`, `SetLastScanResult` |
| Temperature | plate readout only | container seated on a plate, plate window open | whole °C, truncated | `MachineHeatingPlate` tick |
| Plate target | plate target line | plate | whole °C | same |
| Wet / enough volume | finisher status line | recipe selected, container seated | binary | `MachineChemFinisher` tick |
| Machine module effects | tooltips: blender loss %, finisher contaminant %, plate max temp + speed | nothing | exact % | `*.CreateTooltip` |
| Product grade (label) | product tooltip "label" line; green/red once checked | nothing | grade band only; equals truth for a freshly pressed unit | `LabelerHelper.CreateLabeledItemTooltip` |
| Product true purity | spectrometer: "tested" % (2 decimals) + true grade, beside the label grade | `chem_scanner` | exact | `ChemScannerUIManager.RefreshChemMode` |
| Label vs truth fix | spectrometer Match button (relabels to truth); labeler (any grade) | scanner / labeler | — | `MatchChemLabel`, `LABELER.md` |
| Predicted unit purity before pressing | **nowhere**; DERIVABLE as residue active % × (1 − finisher loss %) from a scan of the container | scanner | — | §4 |
| Reaction progress, rates, thresholds | **HIDDEN** (DERIVABLE only by re-scanning over time) | — | — | §2 |
| Temperature off the plate, in the evaporator, at night | **HIDDEN** (DERIVABLE from the cooling law) | — | — | `GetCooledTemp` |
| Evaporator removal / distillate amounts | only as volume change (tooltip, scan) | — | — | `MachineEvaporator` |

## 5. Effects on the customer: euphoria and toxicity

Per unit (`ChemicalFeedbackHelper`): euphoria = Σ µL × euphoria; narcotic if ≥ 750; levels 1 weak (750),
2 mild (1000), 3 strong (1600), 4 extreme (2200). Toxicity = Σ µL × lethality: toxic ≥ 750, lethal ≥ 1000
(the scanner shows "×N" doses). All VISIBLE with the spectrometer.

| Product | Euphoria by purity | Toxic / lethal |
|---|---|---|
| Dream dust (2 ml × 1.0) | level 1 at 37.5 %, 2 at 50 %, 3 at 80 %; level 4 is unreachable | never |
| Phanta pill (0.5 ml × 10) | level 4 from 44 % | **toxic from 75 %, lethal at 100 %** (lab: 74 % clean, 75 % toxic) |
| Nightmare dust (4 ml) | 0.25/µL: level 1 from 75 % | lethal from 6.25 % — it is a poison, sold as one |
| Caffeine, nomore, birth control | none | none. MEDICAL-intent products (caffeine, nomore): feedback level +1 below 45 %, +2 below 30 %, + label grades above truth; a reaction then costs −3 rep extra below 45 %. An honest-label unit at ≥ 45 % has level 0: it never rolls a reaction, so no complaint and no return visit (`ChemicalFeedbackHelper.GetItemFeedbackLevel`) |
| Acid/base residue | — | 0.1/µL: 7.5 ml of acid in a unit would be toxic; irrelevant at drug scale |

What a bad reaction costs (refunds, rep, crime) is in §7.2.

## 6. Procedures (as a player would run them)

Quantities assume the usual containers: a 3 L filter pitcher as the work vessel, mini alcohol bottles
(250 ml: 175 ml alcohol + 75 ml water), supply bottles from the packs (§7.6). "Hold" means a plate with ≥ 2
modules set to the target; without it use the lift-off method (§3). Targets move ±5 from wherever they
are (§3): from a fresh 99 the in-window values are 44 or 49 (extraction), 54 (nomore), 124 (Phanta); 45 needs a
trip to the 160 clamp first. The readout truncates, so a held container reads one degree under an
integer target.

### 6.1 Lab-pure dream dust (lab: 6 caps → 33 units at 99.2 %, value 1386)

1. Blender with 4 chem modules (0 loss; 35 % loss unmodded): 6 dream caps into the pitcher → 132 ml mush,
   24 ml water, 12 ml protein, 3 ml mineral, 12 ml microbe, 12 ml physical.
2. Pour 4 mini alcohol bottles (700 ml alcohol, ratio 5.3). Alcohol is now ≥ 40 % of the mix, so every
   microbe dies on the next tick (R17).
3. Plate, target **45 °C** (40–49 works; never set 40, never reach 50): 120 s → 66 ml extract + 66 ml
   plant matter. Take it off.
4. Protein: measure acid equal to the protein (12 ml; the acid bottle is ≈ 95 % acid, so ≈ 12.6 ml of the
   bottle's mix) by dropper/cylinder; 60 s → 12 ml debris. If acid is left over, the same volume of base;
   30 s. (Dilute acid in alcohol, not water, for the last drops — CthuLuck's chat tip; water is just more
   to evaporate.)
5. Pour through kotton fabric (plant matter, debris), then through the water filter (physical, mineral).
   Left: 66 ml extract, 0.47 ml chemical contaminant + 0.03 ml heavy metal (from the acid bottle), 324 ml
   water, 700 ml alcohol.
6. Evaporator (powered): 1024 ml of solvent at 300 ml/night = **4 nights** (a turbo booster adds one).
   Collect the alcohol back with the solvent-recovery module (50 % of the alcohol evaporated with
   quality 50, lab).
7. Chem finisher, 4 modules (with 3 this 99.2 % residue gives 98.2 %, pharma), Dream Dust, press Output until it
   stops: **33 × lab pure**, 5.5 per cap, 42 each.

Unmodded, as the manual teaches it (lab, 6 caps, 3 minis, heated to 45 °C and lifted, no protein step):
no filter → 51 units at 36.5 % (junk, 4 each, 204 total); kotton filter only → 30 units at 62.5 %
(street, 540 total). The manual's "optional" filter is worth 2.6× the batch.

### 6.2 Caffeine pills (legal; lab: 3 beans → 25 units at 99.1 %, value 600)

1. Blender (4 modules): 3 coffee beans → 134 ml coffee mush, 24 ml caffeine already free, 24 ml fat,
   24 ml protein, 24 ml physical, 10 ml water.
2. One mini alcohol bottle (175 ml alcohol). Plate ≥ 50 °C (60 used): 120 s → caffeine 24 → 125 ml (R7
   reads the ratio against each second's 15 % chunk, so alcohol **0.75 × the mush** already gives the full
   75 %; 0.5 × gives 68.6 %, 0.25 × 50.3 %, lab `FormulaExps.CoffeeRatio`). Caffeine never degrades; heat freely.
3. **Cool below 55 °C**, then lipobond 1 : 8 of the fat (3 ml) and acid = protein (24 ml); 90 s; base for any
   leftover acid. (Lipobond added at 60 °C boils off first: 4.9 ml of fat survives and the batch drops to
   95.7 % pharma, lab.)
4. Water filter. Evaporator: 1 night.
5. Finisher, 4 modules, Caffeine Pill: 25 × lab pure (24 each). ≈ 200 of value per 28-credit bean.

With water instead of alcohol (400 ml at 90 °C, R8) only 12.5 % of the mush converts: 8 pills from the
same 3 beans.

### 6.3 Phanta pills (contraband 3; lab: 1 precursor bottle → 39 units at 61.6 %)

1. Pour one precursor bottle (≈ 120 ml methylol + 120 ml stabiliser + 2–8 % impurities) into the pitcher.
2. Strip the stabiliser (R14 does nothing while stabiliser ≥ methylol): acid ≈ **¼ of the stabiliser**
   (30 ml) is enough — R12 consumes only 5 % of the acid per second while removing 20 % of the
   stabiliser; 60 s → stabiliser 0, acid 1.7 ml. A whole acid bottle also works but leaves 86 ml of acid
   to neutralise and 4× the contaminants (the batch fell to 31 %, junk).
3. Base equal to the leftover acid; 60 s. Water filter (debris).
4. Reducing agent = 0.75 × the methylol (≈ 90 ml).
5. Plate **120–129 °C**: a 4-module plate at 128 °C reaches it in seconds and finishes in ≈ 40 s (12 ml
   Phanta from 120 ml methylol: the yield is 10 % by volume). A 2-module plate on high sags to 107–120 °C
   as the volume shrinks (half the rate); 115 °C targets settle at 109–112 °C and stall. **A 1-module plate
   is fixed at 160 °C**: it passes the window in seconds, then destroys the Phanta (0.04 ml left after
   240 s).
6. Usually already dry (no water in these bottles). Finisher: 0.5 ml per pill. Residue ≈ 62 % (the
   reducing bottle's contaminants dominate) → street standard 12 each; aim for 60–74 % (§4, §5). Chlorine
   tablets did not help (the reducing bottle adds its contaminants afterwards; tablets add their own).

One restricted pack (2 precursor bottles + 1 reducing agent ≈ 235 ml, enough for both) ≈ 80 street pills.

### 6.4 Nomore (Phanta-free "relief" injector, `pink_alt_injector`, legal medical)

Dream extract + reducing agent 1:1 + lipobond ≥ ¼ of the extract (not consumed; less only slows it) +
any water, **alcohol ≤ 10 % of the total** (so evaporate the extraction alcohol first — lab: 25 %
alcohol → nothing). Hold **50–55 °C** (52 °C: 40 ml extract → 24 ml nomore in 60 s; 47 °C takes 120 s).
Lipobond boils above 55. Filter the 40 % debris, evaporate water and lipobond, finisher 4 ml per
injector. 40 ml extract → 6 injectors (117 each at lab pure) versus 20 lab-pure dream dusts (840): worse
value, but legal (contraband 0).

### 6.5 Nightmare dust (poison, contraband 3)

As 6.1 with nightmare caps (15 ml mush per cap, 5 ml microbe) but **hold 40–49 °C exactly** (R5 destroys
5 % per °C above 50) and 4 ml per unit. 36 base. Every unit is lethal (§5): only poison buyers.

### 6.6 Other recipes in this model

- **Starch "cut"**: blend nutrifruit (43 ml starch, 196 ml water, 5 ml protein per fruit), filter,
  evaporate (or boil on a ≥ 1-module plate above 100 °C) → starch residue to dilute a product.
- **Water treatment** (tablets, filters, purifier, organic waste with acid/base or heat): `WATER.md`.
- **Animal serums** (`serum_red` steroid, `serum_green` immunity; 100 each, `HusbandryDirectory`): bought, not made; each injects 20 µL into a rat, metabolised 5 µL a day. While in the animal the steroid gives growth +20 %, immunity −40 %, hunger +25 %; the immunity serum immunity +75 %, hunger +25 % (`Liquid.OnEnterSystem`).

## 7. Drugs

The narcotics are **dream dust** (contraband 2) and **Phanta pills** (`fanta_pill`, contraband 3). The
poisons are **nightmare dust** (3) and **cyanide pills** (`zyanide_pill`, 2; no recipe). Legal chem
products: caffeine pills, birth-control pills (no recipe), the **nomore** injector (`pink_alt_injector`,
narcotic *relief*), and Synaprest (`black_injector`, 350, bought only). Economy facts below are from code
(`StoreClientListCartel`, `StoreClientManager`, `SecData`, `ContrabandHelper`, `MaterialDirectory`,
`StoreClientList*`), read but not lab-measured except where marked.

### 7.1 Products at a glance

| Product | Base | Contraband | Market price, street → lab pure (contraband markup included) | Made from (per unit) | Crime when sold |
|---|---|---|---|---|---|
| Dream dust | 18 | 2 (+70 % market) | 31 → **72** | 2 ml dream extract | DRUG_TRAFFICKING |
| Phanta pill | 12 | 3 (+100 %) | **24** at 60–74 % (toxic above) | 0.5 ml Phanta | DRUG_TRAFFICKING |
| Nightmare dust | 36 | 3 (+100 %) | 72 → 169 | 4 ml nightmare extract | CONTRABAND_TRAFFICKING |
| Cyanide pill | 33 | 2 | 56 street | — (buy) | CONTRABAND_TRAFFICKING |
| Caffeine pill | 10 | legal | 10 → 24 | 5 ml caffeine | none (legal), but bad product → FRAUD |
| Nomore injector | 50 | legal | 50 → 117 | 4 ml nomore | same |

Price = base × (1 + grade %) × (1 + market %), market % = contraband markup (+35 / +70 / +100 / +150 % for
levels 1–4, `ContrabandHelper.ApplyContrabandFeature`) + events + the buyer's discount
(`GameItem.ComposeStagedValue`; `RECIPES.md`/`BARTER.md` for retail markup and haggling).

Per input, at lab pure with 4 modules everywhere: **a dream cap ≈ 5.5 dusts ≈ 400 at market**; a coffee
bean ≈ 8 caffeine pills ≈ 200; a restricted pack (100) ≈ 80 Phanta pills ≈ 1900 at market if buyers exist —
Phanta's value is volume, and every unit is DRUG_TRAFFICKING evidence.

### 7.2 Buyers

The grade gate is `ChemicalProductHelper.WillAcceptNarcotic(item, grade)` from `StoreClient.IsClientRefusingItem`:
every buyer refuses **junk and starch** grade (for any chem product, caffeine and nomore included —
`StoreClient.IsClientRefusingItem` applies it to every `CHEMICAL_PRODUCT_TAG` item; the merchant and shady
merchant's grade 0 changes nothing); grade-2 buyers also refuse **cut product**; grade 3 (refuse street) is
coded but nothing uses it. It reads the **label** (`GetCurrentItemCondition`), so a relabelled unit passes until exposed.
Clients with `CanExposeChem` (the dealer, merchants, nurses, doctors, admins, Odin, trade boards; never
basic, tourist or Rev clients) expose a label/truth mismatch: label better than truth → **−4 rep** with the
buyer's faction and the true price; truth better than label → +3 rep and the higher price.

| Buyer | Faction | Budget | Grade | Exposes | Comes |
|---|---|---|---|---|---|
| Street dealer (`street_dealer`) | Black Market (crime ÷2) | 200 + 4 × day | 1 | yes | chemist start day 3; from day 8 on days ≡ 2, 5 (mod 7) with the chemist start or Retired Chemist |
| Desperate addict | Lower Levels (scav) | 100–125 | 1 | no | window SUBSTANCE tier 1 (`WINDOW.md`); the Oxymore-wave event |
| Worn-out spacer | Lower Levels | 145–180 + day/4 | **2** | no | window SUBSTANCE tier 2 |
| Upper-level hedonist | Upper Levels | 350–435 + day | **2** | no | window SUBSTANCE tier 3 |
| Miner minor / UL visitor minor | LL / UL | 100–125 / 500 | 1 | no | minor pool |
| Addict barter (`addictGeneralBarter`) | LL | barter (wire, blood bags, shivs, 8 % 2 × common chemical) | blocks junk/starch | no | chemist start day 2; general pool |
| Miner, junkers, UL-LL visitor, clueless tourist, ice miner, substance-shortage client | various | — | 1 | no | general/event pools (buy SUBSTANCE by tag) |
| Officer Lun (`officerLunShot` / `BackOnDuty`) | Security | 100 / 94 | 1 | no | Lun storyline +1 / +5; buys narcotics with **no evidence** — but see §7.5 |
| Medical buyers (spacer medical, sick-child caretaker, low-level doctor, clinic nurse, hospital admin 750–1000, pharmacist) | LL / UL | 80–1000 | 1 | doctor 50 %, nurse 75 %, admin yes | take caffeine, nomore, and dust by its MEDICAL tag |
| Cartel wounded soldier | Cartel (×3 trade rep, crime 0) | 150 | 1 | no | chemist start, days 9, 18, 27 (MEDICAL) |
| Poison: Odin (`wanted4Normal`, Rev, 350 → 1500), shady merchant (BM, contraband, −17…−75 % of the markup) | | | | yes | `BARTER.md`; the `poisonBuyer` factory exists but nothing spawns it |

Lower Levels and scav clients refuse contraband 35 % / 25 − LL rep/4 % of the time; Security always;
Upper Levels and tourists never (`StoreClient.CompleteClientCreation`).

**Underground Exchange** (`ExchangeManager`, `ExchangeList`; unlock and access in `PERKS.md`/`MITIGATIONS.md`).
Items left in the trash can are matched overnight; each barter once a night, higher priority first, and
each acceptance gives +75 BM progress. Purity is the **true** `CHEMICAL_PRODUCT_PURITY_TAG`, labels ignored:

| Barter | Takes | Pays |
|---|---|---|
| `dream_dust_barter` (always, priority 2) | 2 dream dust | nothing if the two purities (truncated) sum < 60; else one random roll (water filter, meat, fair ingot, electronics, knives, medical crate, birth-control pills, stamp, blood bags…), doubled or unlocked (energy credit 100, breeder credit, scav token) when the sum ≥ 120 |
| `improved_dream_dust_barter` (promotional: active only while among the 3 of 7 promos drawn by `ResetPromotional`) | 4 dream dust | weighted table by **mean** purity band (< 30, 30–45, 45–60, 60–80, 80–90, 90–99, ≥ 99): from 2 bandages at the bottom to Synaprest + genuine injector (40), 2 blue blood bags (30), 3 genuine injectors (20) or 2 Synaprest (10) at ≥ 99 |
| `poison_barter` (always, priority 1) | 2 POISON items | energy credit = Σ (purity / 2) for chem products (other poisons: their value), capped at 100 |

**After the sale (personal-use buyers only, not tourists; `SoldItem.HandleSoldItem`).** Each unit rolls
8 % × feedback level. For a narcotic the level is mostly its euphoria (2 / 3 / 4 → 2 / 4 / 6, so 16 / 32 /
48 %). Good product (euphoria ≥ 2, label true, not toxic) brings the buyer back **once**, next day, to buy
the same item with budget max(50 + day/5, 3 × the price paid). Bad product brings a hand note (rep hit at
once) or a complaint client asking for a refund or double compensation: −2 rep base, −5 more if toxic,
−15 more if lethal (a dead friend), −2 for no high, −1 for weak, −1/−2 for a label one/two grades off; refusing
the refund costs that rep (cap −20). Legal products sold to non-BM, non-Rev buyers also feed
`badChemTrackerFraud` (level 1–3: + price) or `…Endangerment` (≥ 4: + 2 × price), converted to one FRAUD /
ENDANGERMENT crime point per 300 and never decaying (`SecData.CheckSoldChem`, `PlayerStore.CheckCrime`).
Contraband products never write that crime (`ChemicalFeedbackHelper.CanReport`) — their crime is the
trafficking itself.

### 7.3 The cartel (chemist start, `NewGameData.StartType.DRUG_DEALER`)

Start: shabby blender, heating plate, evaporator (40), chem finisher, chem scanner and mini smuggler's bay;
1 and 5 ml droppers, 25/50/100 ml cylinders, pitcher, kotton fabric, coffee bean, 2 water, the chemistry
manual; Retired Chemist owned; `CARTEL_PROTECTION` service; Security rep −15; cash −300. Cartel rep starts at 0.

| Day | Visit | What happens |
|---|---|---|
| 1 | cartel intro | 3 dream caps + 1 common chemical, free. Insurance locked out |
| 2 | officer asks whether you will report | reporting → **−20 cartel rep**; staying quiet costs nothing |
| 4, 11, 18… (≡ 4 mod 7) | handler check-in | buy materials (3 caps + 1 common chemical) for −12 rep (−6 with Reliable Chemist) |
| 8, 15, 22… (≡ 1 mod 7) | **weekly quota** (calendar marker) | below |

**The weekly quota** (`CreateCartelHandlerWeekly`, `BuildPurityChain`). Terms start at **10 dream dust at
40 % purity**; each week +5 units (no cap) and +5 % purity (cap 70). On delivery the handler takes **every
dream dust on the counters** (surplus included) and judges the count and the **mean true purity**
(`CHEMICAL_PRODUCT_PURITY_TAG`, labels do not help):

| Count | Effect |
|---|---|
| none | **game over** |
| short | rep −4 per missing unit |
| met or more | rep **+ the whole count**; reward step |

| Mean purity (req = required) | Effect |
|---|---|
| ≤ req/4 | **game over** |
| ≤ req − 5 | rep −(req − mean) |
| ≤ req + 10 | rep +(mean − req), truncated; reward step |
| > req + 10 | rep +(mean − req)/4 only; reward step |

So rep peaks at **exactly req + 10** (week 1: 50 % gives +10; 51 % gives +2). Pay every week, met or not:
150 cash (+4 × cartel rep with Increased Stipend) and 3 dream caps + 1 common chemical; a `chem_module`
too when the count is met **and** the mean purity is above req − 5 (`BuildNextWeekChain`, reward ≥ 2). Between
req − 5 and req the rep change is negative but the module still comes. Rep moves through `ModCartelReputation` (difficulty multipliers,
`PERKS.md`). Cartel perks (`PERKS.md` §3): 25 Increased Stipend, 50 Reliable Chemist, 100 Authorized Exit,
−100 Forced Retirement (game over). At ≤ −20, nightly vandalism at |rep|/5 %.

**This is where cutting pays.** Quota units are judged on the mean, not per unit, and a 50 % unit costs
1 ml of extract against 1.98 ml for a lab-pure one. Week 1 (10 at 40 %) needs 10 ml of extract — one cap —
cut with starch (or simply kotton-only filtering, ≈ 62 %, which still gives +5). Overdelivery adds +1 rep per
extra unit. Purchased street dust (60–80 %) counts too.

**Leaving.** Authorized Exit (rep ≥ 100) only starts it: each weekly visit while authorized raises the
threshold by 25 (100 → 125 → 150 → 175 → 200); the exit completes at a weekly visit with rep ≥ 200 (the
clamp) — at least five visits after first reaching 100. Then visits stop, protection ends, insurance
returns; the dealer, shady farmer and retired chemist keep coming. ("Cartel 100 = free to leave" in
`docs/personas/cook.md` is wrong.)

**Protection** (`CARTEL_PROTECTION`, free, mandatory): no burglary at all (it replaces insurance). It does
**not** touch inspections, evidence or crime; sales to Cartel-faction clients carry no crime
(`MITIGATIONS.md`).

### 7.4 Addiction

Players only. Narcotics (and nomore) are usable only with the `narcotic_addiction` perk
(`HealthData.CanUseNarcotic`); consuming product never creates an addiction. A dose covers days by its
narcotic value (dust 1, Phanta 3, pink injector 1) plus a purity bonus (< 45 % −2, < 60 −1, < 80 0, < 90 +1,
< 99 +2, ≥ 99 +3); below 30 % a dose does nothing. Nomore covers on a separate relief clock with the same
purity bonus (`HealthData.ConsumeNarcotic`, `ConsumeNarcoticRelief`). Withdrawal (≥ 3 days since either) adds 8 % to the
addiction trade penalty. The 20-clean-day cure counts only days since a *dose* (relief does not stop it);
Synaprest removes one random addiction. Customers have no addiction state, except Officer Lun's counter
(+1 per narcotic sold at his first visit, +2 at the second, −10 per Synaprest) — above 0 it unlocks his
surplus visit (+9) and the Lun inspection (+13).

### 7.5 Law

| Contraband level | Items |
|---|---|
| 1 | dream cap seed (24) |
| 2 | dream cap (23), restricted chemical supplies (100), dream dust, cyanide pill |
| 3 | nightmare cap (26), nightmare dust, Phanta pill |
| legal | common chemical supplies (50), every chem machine, chem module, scanner, chem storage bay (400), the unpacked bottles (including the precursor), caffeine, nomore, birth control, Synaprest |

- Inspections, seizure, fines, evidence and the smuggler's bay: `INSPECT.md`, `DAY.md`, `MITIGATIONS.md`.
  Level ≥ 2 is "serious": caps, dust or restricted supplies in the window summon the showcase inspector
  (`WINDOW.md`).
- Work in progress is legal: a container of extract, the precursor bottles and every machine are clean;
  only caps, restricted packs and the finished units are contraband. Press units only when a buyer is
  coming, and keep finished units in the smuggler's bay.
- **The finisher's output grid is not a hiding place.** A Security inspection walks every item in the shop
  inventory, front counter, back room and showcase, and recurses into each item's children, machine windows
  included (`ContrabandHelper.StartInspection`, `RecursiveInspect`; `GameItem.SetContentWindow` makes the
  window a child). Only smuggler's bays (all three) and items flagged hidden, locked or behind a lock stop the
  descent (`IsBlockSearchContainer`, `InspectItem`); the trash can is searched unless the inspector is lazy;
  the hidden area only when the poster is pulled. So dust in the finisher grid, a chemist storage bay or any
  other machine is found and seized like dust on a shelf (CthuLuck's habit works only until an inspection).
  A dissolved product is just liquid in a container and carries no contraband tag.
- The Lun inspection (storyline +13, if his addiction counter > 0 and he was not arrested) destroys every
  NARCOTIC-tagged item (dream dust, Phanta, the `pink_injector`; not nightmare dust or nomore) lying **directly**
  in the shop inventory, front counter, back room or showcase. It does not recurse: anything inside a machine,
  bay, box or other container survives, and the trash and hidden area are not checked
  (`StoreClientUniqueList`, Lun's end action).
- Events: `sec_raid_black_market` NARCOTIC +50 % for 2 days; MEDICAL events also move dust and Phanta
  (`medicalSuppliesShortage` +50, `oxymoreWave` +25 and desperate addicts, `bm_gang_war` +50,
  `ul_medical_aid` −25, Rev raid +40, `raid_security` +75); additive with the contraband markup
  (`PRODUCTION.md` §5).

### 7.6 Inputs

| Input | Price | Contents / notes | Where |
|---|---|---|---|
| Common chemical supplies | 50, legal | unpacks to 2 mini alcohol (175 ml alcohol + 75 ml water, clean), lipobond 150 ml, acid 250 ml, base 250 ml (each 2–8 % impurity: chemical, physical, microbe, heavy metal) | retired chemist (weekly), cartel (weekly), Rev raids |
| Restricted chemical supplies | 100, contraband 2 | 2 precursor bottles (methylol + stabiliser, ≈ 120 ml each after impurity) + reducing agent ≈ 235 ml | retired chemist 50 %, Rev raids |
| Dream cap | 23, contraband 2 | 22 ml mush, 4 water, 2 protein, 2 microbe, 2 physical, 0.5 mineral (factory genetics); small (17) and large (31) caps are built from a 20.5 / 75 g base instead of 32.5 g | cartel weekly (3), shady farmer (days ≡ 3, 6), own hydroponic bay (seed 24: 8 caps / 4 nights, `PRODUCTION.md` §1) |
| Coffee bean | 28, legal | 44.8 ml mush, 8 caffeine, 8 fat, 8 protein, 8 physical, 3.2 water; also a seed (8 beans / 3 nights) | shady farmer 75 %, start kit |
| Bred produce | — | DNA traits change the active liquids of harvested caps and beans: a *Production* trait adds its liquid per rank (dream mush, coffee mush, caffeine; dream mush turns into nightmare mush with the nightmare module, and its produce becomes contraband), *Potent* scales every active liquid +25 % per rank, *Weak* down. Scan bred produce rather than trust the table | DNA manipulator (`PlantTraitList.Production`, `Infuse`, `ScaleActives`) |
| Nudka | 85, legal | poured onto a container with ≥ 750 ml free: 337.5 ml alcohol + 412.5 ml base-grade water (with base water's contaminants, `WATER.md`); the bottle is used up. ≈ 2.9× the water of a mini bottle per ml of alcohol | `WaterHelper.PourNudka` |
| Nightmare cap | 26, contraband 3 | 15 ml mush, 8 water, 5 microbe, 2 protein, 2 physical | shady farmer 75 % |
| Chem module | 250 | the only chem upgrade; 4 per machine | the retired chemist (one, then 50/50/25 % more, last on his list: his four machines and the rest come first and can fill his counter, `StoreClientListSpec.CreateRetiredChemist`); Dr. Jackson with the Retired Chemist (50 % and 25 % a visit, +50 % with each of his Jackson I/II upgrades; his visit and his after-hours shop, `StoreClientList.PlaceInventorInventory`), +50 % on hard; the cartel's weekly reward when count and purity pass |
| Solvent-recovery module | 275 | evaporator keeps alcohol (§3) | retired chemist 75 % |
| Kotton fabric | 65 | filter size 150, durability 1000 (2 per ml removed) | retired chemist, shady farmer; kotton plants |
| Water filter | 25 | size 22, durability 400 | retired chemist |
| Retired Chemist (network) | 1000 | the four machines, scanner, glassware, a weekly supplier visit (days ≡ 2, prices +0/15/30/45 % by difficulty) | network upgrade (owned on the chemist start) |
| Lower Levels Chemist (Ashish, phone) | 250 | a Black Market courier that **sells** street dust (−20 %, −35 % with the 1440 shareholder upgrade) and buys chemical supplies; not an input supplier | network upgrade |

## 8. Sources

- Code: `ChemicalHelper`, `ChemicalProductHelper`, `ChemicalFeedbackHelper`, `ChemicalCompositionHelper`,
  `ChemFinisherRecipe`, `PreBuildChemHelper`, `Liquid`, `MachineBlender`, `MachineHeatingPlate`,
  `MachineEvaporator`, `MachineChemFinisher`, `ChemFinisherUIManager`, `MedsItemDirectory`,
  `ChemScannerUIManager`, `WaterScannerUIManager`, `AmenitiesItemDirectory` (AquaScan, strip),
  `MachineFridge`, `MachinePurifier`, `MaterialDirectory`, `WaterHelper`, `WineHelper`, `LabelerHelper`,
  `ItemConditionList`, `ExchangeList`, `ContrabandHelper`, `StoreClientListCartel`,
  `StoreClientListTierSubstance`, `StoreClientUniqueList` (Lun), `ShipSystemDirectory`, `HealthData`.
- The in-game Chemistry 101 manual (`var/knowledge/manuals/chemistry_guide.md`, local): grades, the
  dream-dust and caffeine recipes, the Phanta hints. Where it differs from the code: its 40–60 °C
  extraction window is really 40–49 °C (mush rots from 50, extract dies from 60); its "80 °C minimum" for
  caffeine applies only to water extraction (alcohol works from 50 °C); filtering is not optional.
- CthuLuck, ["GIVEAWAY DAY 3/5 — Probably Stolen 1.0: The CHEMIST Reloaded!"](https://www.youtube.com/watch?v=h5WuPMeo6nY)
  (Chemist start, Thin Margins). Cross-checks: his first unmodded batch read 87 % (code: a ≈ 99 % residue
  × 0.88); "finisher −12" and "Pharma once a module is in the finisher" (code: 12 % / 6 %); "the base
  burner stops at 99, starch needs a module to dry" (code: fixed 99 °C target, water boils above 100);
  "no alcohol back without the evaporator module" (code: distillate = quality %, 0 without quality); acid
  1:1 with protein (code: strictly 1:1 on the net excess; chat's 1 : 0.777 is the acid bottle's ≈ 95 %
  purity plus dilution); lipobond 1:8 (code: exactly 8). Discrepancies: his finisher outputs 40 % and 51 %
  from 58 % inputs are the 12 % physical contaminant plus the starch-cut residue, not a variable penalty;
  "keep dream cap above 50 % or it evaporates" has no code basis.
- Lab: `sim/lab/ChemExps.cs`; results `var/lab/chem-results.jsonl` (local).
