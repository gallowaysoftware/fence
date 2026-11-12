# Sources and sinks

Every resource the shop can make, buy, hold or lose, with every code path that creates it and every path that
consumes it. Build 26525552, read from the decompiled source; rows marked **(lab)** were measured on the game's own
machines (`sim/lab/FormulaExps.cs`, docs/sim/VERIFIED.md "Formula exploits"). Prices are hard-difficulty unless
stated: Dr. Jackson ×1.5, the retired farmer / rancher / aquafarmer / winemaker / chemist / gunsmith ×1.3, the
supplier ×2, the retired junker ×1, Wild/network upgrades ×1.5. Reputation gains ×0.75, losses ×1.25. The ranked
loops built from this graph, and what each engine should do with them, are in `FORMULA-EXPLOITS.md`.

Column **C/R**: C = consumed; R = reusable (a tool, catalyst, solvent, threshold or term that is read, not spent).

## 1. Liquids

All amounts are integer µL per liquid on the container. Generic paths, left out of the rows below:

- **Reaction tick**: `ChemicalHelper.HandleTemperatureReaction` every real second while the shop is open and 360 times
  each night (`RunNightlyChemicalSimulation`).
- **Pour**: `WaterHelper.TransferLiquid` moves min(free space, source) of every liquid in proportion. A seated filter
  in the target (`HandleFilter`) strips each liquid whose particle size ≥ the filter's; microbes pass every filter but
  the UV one. Filtered volume is destroyed.
- **Destroyers**: draining (`EmptyContainer`, `WineHelper.DrainBottle`); the chem finisher's draw (`RemoveAndReturn`);
  hydroponic drinking and overflow (`RemoveMl`).
- **Converters into water (volume kept)**: the purifier (`MachinePurifier.RemoveLargestFirst`, any non-water liquid,
  largest particle first; budget (min(V, 10 L) × 0.003 + 25 ml) × (dirt bonus + P/100) × 1.25 a night; cap 99.01 %,
  Pure at Q ≥ 100); the portable purifier (`WaterHelper.PurifyToBaseWater`, to 98 %, ⌈V/100 ml⌉ charges per use);
  the purifying plant trait (`PlantWaterQuality.Purify`: max(1 ml, 28.3 %) of each liquid except water and mineral a
  night at rank IV, down to per-liquid floors of chemical 4, heavy metal 6, microbe 2 bp of the water; **no volume
  budget**); RAS filters (`AquacultureHelper.RunFilter`, everything a filter catches); UV (`WaterHelper.OnUVUsed`,
  microbes → water, the filter is not used up).
- **Evaporator** (`MachineEvaporator.Evaporator`, each night and each turbo boost, battery draw 6): removes up to
  round(300 ml × (1 + P/100)) of the evaporative liquids (water, alcohol, lipobond) in proportion; each removed liquid
  × Q % (Q clamped 0–100) goes to the distillate container. The solvent-recovery module (Q +50, E −25) zeroes every
  liquid but alcohol. Q 0 recovers nothing. (lab: Q 50 → 50 % of the alcohol; solvent + 2 fineness, Q 100 → 100 % at
  252 ml a night; 4 fineness, Q 100 → water and alcohol both back, the water clean.)
- **Boiling** (`HandleBoilingReaction`): above the boiling point, −max(0.1 ml, 2 %) a second. Water 100 °C, alcohol
  78.37, lipobond 55.
- **Value**: water sells at 40 per litre of total liquid × the grade modifier (`WaterFeatureHelper.GetWaterPrice`).

### 1.1 Water and its grades

Grades are basis points of water over the total: Pure 9990+, HQ 9900+, Base 9800+, Ghost 9600+, Rust 9000+, Gutterflow
3500+ (`AddWater`; bought HQ/Base/Ghost roll near the bottom of their band).

| Source | Type.Method | Amount / cost | Grade |
|---|---|---|---|
| Rust tap | `WaterHelper.InitLiquidContainerItem` (faucet) | free, fills the container | 90.00–91.50 % (daily roll), needs `RUSTWATER_TAP` |
| Gutter tap | same | free | 80–90 %, needs `GUTTERFLOW_TAP` (+ `WATER_TRADER`) |
| Bought bottles | `WaterPremadeHelper.*` via `StoreClientListWater`, events, story | per seller | any |
| Rations | `FoodItemDirectory.SetupWaterRationPour` → `AddBaseWater` | 750 ml for 30, 375 for 15; **into any owned container**, not only wine bottles | Base |
| Moisture farm | `MachineMoistureFarm.Fill` | 850 ml × (1 + P/100) ÷ (1 + 0.25 × other farms running), power 6 | grade of Q + 50: Ghost; Base ≥ 50; HQ ≥ 100; Pure ≥ 150 |
| Evaporator distillate | `MachineEvaporator` | Q % of the water evaporated | **100 % water only if water is the only evaporative liquid in**; otherwise mixed with alcohol/lipobond (lab) |
| Spirits | `WaterHelper.PourSpirit` (Nudka), `WineHelper.PourWineIntoContainer` | Nudka 412.5 ml; a wine pour 750 − alcohol − extract − contaminant ml | Base |
| Mini alcohol bottle | `PreBuildChemHelper.CreateMiniAlcoholBottle` | 75 ml | clean |
| Produce | `ChemicalCompositionHelper` | nutrifruit 78.6 ml / 100 g; dream cap 4; nightmare cap 8; coffee bean 3.2 | — |
| Conversions | purifier, portable purifier, reed, RAS filter, UV, gutter-tilapia remediation (`RunRemediation`) | volume kept | upward |

| Sink | Type.Method | Rate |
|---|---|---|
| Hydroponic drinking | `MachineHydroponic.HandleWaterConsumption` | 20–287 ml a night per plant, in proportion (purity unchanged) |
| Fish waste | `AquacultureHelper.AddWaste` | water → organic waste, 0.5–2 ml per 100 g a night |
| Polluting trait | `PlantWaterQuality.Pollute` | 0.1 % × r^0.75 of the water → organic 50 / microbe 35 / physical 15 % |
| Wine fermentation | `WineHelper.StartFermentation` → `DrainBottle` | the bottle's water |
| Boiling, evaporator, sale, vending fountain, drain | — | — |

Upgrades between grades: the purifier (to HQ; Pure at Q ≥ 100), the portable purifier (to 98 %), a pour filter (organic,
physical, mineral, microplastic, size-200 liquids to 0.16 % of their own amount per pass), chlorine (chemical
contaminant down to 0.7 % of the volume, then microbes), UV (microbes), the reed (28 %/night of everything but
mineral, to its floors), the distillate, the moisture farm. **Pure with no modules (lab):** rust tap → two filter
passes → reed to its floors (≈ 12 nights) → UV = 9990 bp. One filter pass leaves 0.6 bp of filterables and ends at
9989 (HQ). Downgrades: fish waste, pollution, boiling (removes water only), any non-water addition, mixing.

### 1.2 Every other liquid

| Liquid | Sources | Sinks | C/R |
|---|---|---|---|
| `heavy_metal` | premade water mixes (10–15 % of the impurity); supply-bottle impurity (2–8 % of each bottle, split over 4 contaminants, `PreBuildChemHelper.SetImpurity`) | purifier (last), portable purifier, remediation, reed (floor 6 bp) | contaminant |
| `organic_waste` | water mixes; feces 10/20/40 ml; nutrifruit; fish waste; pollution | R20 heat (≥ 80 °C, 33 % → physical); R19 acid/base (1 per 40 → 10 % mineral); water filter; RAS filter | C |
| `microplastic` | **nothing in the code adds it** | filters | dead node |
| `microbe` | water mixes; feces; caps 2 / 5 ml; nutrifruit; supply impurity; pollution | R16 heat; R17 alcohol ≥ 40 % of the total (alcohol not consumed); R18 acid/base (1 per 100 killed); chlorine; UV | C |
| `chemical_contaminant` | water mixes; supply impurity; counterfeit wine pour 8 ‰; filler in bought products | chlorine (to 0.7 % of volume; 15 ml of chlorine spent per tick whatever is left, lab); purifier; remediation; reed (floor 4 bp) | contaminant |
| `physical_contaminant` | water mixes; caps; beans; nutrifruit; R20; pollution; **the finisher's impurity** (12/6/3/1/0 % of the unit at 0–4 modules) | water filter; remediation | contaminant |
| `mineral` | water mixes; nutrient tablet 15 ml (5); caps; nutrifruit; R19 | hydroponic feeding (`HydroponicHelper.HandleNutriment`, destroyed); water filter; purifier; **spared by the reed** | C (plant food) |
| `dream_cap_mush` | blended/dissolved dream cap (22 ml a cap); production trait | **R1** (≥ 40 °C; max(2 ml, 10 %) a second; 0.5 × min(1, alcohol / mush still present / 5) → extract, rest → plant matter); R3 rot ≥ 50 °C; filters | C |
| `dream_cap_extract` | R1; dissolved bought dust | R4 (≥ 60 °C, destroyed); R9 nomore; finisher (2 ml a dust) | C |
| `plant_matter` | R1–R3, R6–R8; meal caps | filters | waste |
| `alcohol` | mini bottle 175 ml (2 per common pack, 50); Nudka 337.5 ml (85); **wine pour 150 ml from any finished bottle, Undrinkable included**; distillate at Q % | evaporator (back at Q %); boiling > 78 °C; purifier/reed → water; finisher allows ≤ 100 µL of evaporatives. **Never consumed by R1, R2, R7, R17**; blocks R9 above 10 % | **R (solvent)** |
| `caffeine` | coffee bean ≈ 8 ml; R7 (≥ 50 °C, 0.75 × min(1, alcohol / this second's 15 % chunk / 5)); R8 water (≥ 80 °C, 12.5 %); bought pills | finisher (5 ml a pill) | product |
| `protein` | nutrifruit, caps, beans | R10 (net acid or base 1:1 → debris) | C |
| `fat` | coffee bean | R11 (1 lipobond per 8 fat → debris) | C |
| `starch`, `sugar` | nutrifruit; filler in bought products | finisher only | inert |
| `coffee_mush` | coffee bean 45 ml | R7, R8; filters | C |
| `contraceptive`, `zyanide` | bought pills only | finisher | buy-only |
| `nightmare_cap_mush` | nightmare cap 15 ml; dream-cap trait with the nightmare module | R2; R6; filters | C |
| `nightmare_extract` | R2; bought dust | R5 (≥ 50 °C, 5 % per °C); finisher | C |
| `chlorine` | water tablet 15 ml (6) | R21: ≤ 15 ml spent per tick whether or not anything reacts, the rest decays max(1 ml, 20 %) | C in about one tick |
| `synaprest` | **nothing** (the injector uses another mechanism) | — | dead node |
| `fanta` | R14 (+0.1 × methylol reacted); bought pills | R15 (≥ 130 °C); finisher | product |
| `methylol`, `stabilizing_agent` | precursor bottle (restricted pack 100) | R14; R12 | C |
| `reducing_agent` | restricted pack 250 ml | R9 (1:1); R14 (0.75 ×) | C |
| `acid`, `base` | common pack 250 ml each | R10, R12, R13, R18, R19; hydroponic penalty | C |
| `debris` | R9–R12; feces | filters; purifier | waste |
| `lipobond_agent` | common pack 150 ml | R11 (1 per 8 fat); boiling > 55 °C; evaporator (lost with the solvent module). **Catalyst in R9, not consumed** | partly R |
| `livestock_steroid`, `livestock_immunity` | serums (100, 10 × 20 µL injections) | −5 µL a day in the animal | C |
| `wine_extract` | wine pour only (tier × 4 ‰ of 750 ml) | nothing uses it | dead end |
| `nomore` | R9 (45–55 °C, alcohol ≤ 10 %) | finisher (4 ml) | product |

### 1.3 Liquid loops

| Loop | Steps | Per cycle | Needs | Lab |
|---|---|---|---|---|
| **Alcohol as a standing solvent** | keep ≈ 5 × one cap's mush of alcohol in the vessel; blend caps into it one at a time; heat 20 s each | 6 or 12 caps at the full 50 % on 120 ml; 1 evaporator night instead of 4 | blender, plate | yes |
| **Alcohol recovery** | evaporator with solvent recovery (+ 2 fineness for Q 100) | 50 % / 100 % of the alcohol evaporated | modules 275 + 2 × 100 | yes |
| Wine → alcohol | pour any finished wine (an Undrinkable one is worth ~0) into a vessel | 150 ml alcohol + ~590 ml Base water | a bottle | read |
| Tap → Pure without modules | rust tap → filter twice → reed to its floors → UV | Pure, any volume (no reed budget) | reed bay, UV filter (150) | yes |
| Tap → purifier → wine-bottle water | the purifier slot takes any owned container, a wine bottle included; ≤ 1.7 L of raw rust reaches HQ in one night | the wine water term (+1) for free tap water | purifier | read |
| Distillate as clean water | evaporator Q > 0 on water alone | Q % of 300 ml a night | Q modules | yes (mixed input) |
| Organic waste → mineral | feces or fish waste + acid (R19) | ≈ 4 ml mineral per large feces | acid | read; negligible |
| RAS water | fish make organic waste; RAS filters return it as water | volume kept; 1 filter durability per ml | filters | read |

## 2. Items, power and meters

### 2.1 Metals and fabrication

| Resource | Sources | Sinks | C/R |
|---|---|---|---|
| junk (3) | dump search (~55 % of searches); debris pulls; screwdriver on a ruined module | furnace junk branch (6 → 4 scrap, 8 power, only on a night with no smelt); reactor repair; souvenir buyer | C |
| scrap_metal (10) | junker 3–4 at 8.5; scav material seller; story seller; Rev quartermaster; dump; gun disassembly (2–4); junk module | furnace (2 a smelt, 3 blast); printer recipes; machine upgrades (`StartingPerkContent.ConsumeUpgradeMaterial`); silencer | C |
| common_ore (15) | miner 3–4 at ~12; mining courier 6 (12 after 350 spent; −20 %, −35 % with shareholder; 5-day cooldown); **miner dividend 6 a week** (`ShareholderHelper`); Rev quartermaster | furnace only; purity step = 2 × ore / inputs, integer (`IngotPurityHelper.SetPurityFromInput`) | C |
| flux_agent (4) | supplier 4 a visit at 8; retired junker; scav sellers; dump; crate; Jackson gift | furnace, only when the module flux level < 1 | C (conditional) |
| advanced_flux_agent (300) | Jackson special stock 450; crate 12 %; Rev raider | furnace flux level ≥ 1 every smelt | **R** |
| metal_ingot (50 × purity: −25/−15/0/+25/+50/+200 %) | **furnace only** (2 → 1 at 8 power; blast 3 → 2 at 16); a rare dust-barter outcome | printer (knife 2, machete 3, gun part 1, frame 1); upgrades (flat 50 whatever the purity); MATERIAL buyers | C |
| common_electronic (33) | junker 28; scav sellers; thief; Rev raider; dump; crate; debris crowbar; ruined module | printer recipes; `ModuleHelper.RestoreModule` (corrupt → P/E/Q); extractor refill; upgrades (fridge, RAS, converter, scanner +15 flat) | C |
| printer_plastic (12) | retired gunsmith 4 a visit at 15.6; junker; supplier; scav; disassembly | printer recipes | C |
| gun_part (25) | printer chip: 2/3/3/4/4/5 by ingot purity (`MachinePrinter.GUN_PART_COUNT_BY_PURITY`); thief; disassembly; refund of a service job sent away (`GunHelper.ReturnPlayerParts`) | gun assembly; repair (spent on failure too) | C |
| printer chips (100–480), printer modules (metal 300, CNC 400, batch 250) | retired gunsmith ×1.3 | the printer | **R** |
| blank_keycard (15) | Jackson, 2 a visit once a desequencer is owned | desequencer | C |
| keycards (ser 40 … cmd 125) | desequencer (33 progress, 10 power a night); crate; command-card barter | locks (destroyed); exchange promos (3 sec / 4 med / 4 eng → crate); card buyers 250–1000 | C |
| loot crates | exchange promos; thief; story | arrive **locked**: a matching card opens them | C |
| tools (screwdriver, cutter, welder, crowbar, toolbox, scanner) | scav, junker, Jackson, crates, dump | **no durability** | R |
| wire, nuts_metal | junker, scav, dump, debris | **no recipe uses them**: sale only | — |
| unreachable | rare ore/electronics, ammo boxes, broken-machine scrapper, `sup` crypto module, box dispenser, deal maker, aug chip, organs, meat culture, ship systems | — | — |

Weapons are §3.

### 2.2 Biology

| Resource | Sources | Sinks | C/R |
|---|---|---|---|
| rat | rancher pair 156 each; elite 650; `RETIRED_RANCHER` (1125) kit; births (litter ~3, 3 nights); rat trap (50 % a night with cheese) | death; butchering → cuts. Cannot be sold | C |
| rat cuts: raw meat 159, small raw 65, morsel 30, small morsel 15 | `HusbandryHelper.CreateButcherCuts` (400/200/100/50 g; any leftover above 0 rounds up to one more small morsel) | FOOD/LUXURY buyers; rat feed (prion roll); fish feed (**no prion check**) | C |
| feces | rats | vacuum robot; blendable chem waste. No fertiliser use | waste |
| feed items (processed meat 800 kcal/25, fat meat 1250/40, milk, noodles…) | commissary (stamps); thieves; loot; kits | feed dispenser buckets OTHER / MEAT / RAT / PELLET / INDUSTRIAL | C |
| industrial_fish_feed (5000 kcal, 100) | aquafarmer 2 a week at 130 | omnivores; growth −25 % × that night's share | C |
| gutterflow_pellet (4000 kcal, 40) | aquafarmer 2 a week at 52; splittable in halves | omnivores; **lifetime pellet share strictly above 25 % → trash-grade fillets** (gutter tilapia exempt); **no growth penalty** (lab) | C |
| fish (tilapia, gutter, graft) | roe 4 a week from the aquafarmer; breeding 15 % a night per fed female | fillet; death | C |
| eel, electric eel (130), sturgeon (650) | aquafarmer big-fish slots after `AQUACULTURE_II`; **never breed** | fillet; old age | C |
| fillets: large 143 (≥ 900 meat), normal 57 (400), small 26, tiny 13; eel 136, small eel 68; fish scrap | `AquacultureItems.BreakDownMeat` (greedy) | FOOD/LUXURY buyers; MEAT bucket for carnivores | C |
| caviar 240, sturgeon roe 120, tilapia roe 30–60 | sturgeon surgery / kill; auto-butcher | buyers; hatching | C |
| seeds and produce | retired farmer ×1.3; self-seeding; `PlantList` harvests (8 power a night) | buyers; chemistry; feed; filters (hydroreed); **nothing spoils** | C (bloomberry perennial) |
| nightmare caps | dream-cap bay with `farm_module_nightmare_cap` (8 a harvest, ~4 nights, seed back) | blender; **poison barter** | C |
| wine | winemaker (bottles 15.6, yeasts, berries); bioreactor yeast (R) | buyers; ages +3 value a night with no cap; the wine pour (alcohol) | C |

### 2.3 Chemical solids and consumables

| Resource | Sources | Sinks | C/R |
|---|---|---|---|
| common_chemical (50) | retired chemist 65; scav; thief; addict barter; Rev raider; crates; cartel | unpacks to 2 minis, lipobond, acid, base; buyers 75–200 | C |
| restricted_chemical (100) | retired chemist 50 % at 130; Rev raider; crates | unpacks to precursor + reducing agent; sells 170 | C |
| chem products (dust, Phanta, caffeine, nomore…) | finisher (graded by purity); Ashish; LL chemists; **chemist dividend 8 dust a week**; **pharma dividend 5 injectors a week**; crates | buyers; cartel quota; dust barters; poison barter | C |
| water_filter (25, 400 durability) | retired chemist 32.5; **filter barter (3 filters of any durability → 1 new)** | wear; **not destroyed at 0** | durable, recyclable |
| water tablets, UV filter (R), portable purifier | chemist, Jackson | water | C / R |
| turbo boosters (single 35, advanced 300) | crates / Jackson special stock 450 | one extra night cycle; **no per-day cap**; the advanced one recharges +5 a night to 15 | C / R |
| modules | Jackson, special stock, crates, exchange, chemist, cartel | machines; chem modules stick on insert; overvolted 10 %/working night → ruined; **module barter takes any 2 MODULE items (ruined/corrupt included) → 1 standard** | R / wear |

### 2.4 Power

| Producer | Type.Method | Per night | Conditions |
|---|---|---|---|
| Power service | `StoreService.UpdateCost`; `PlayerStore.UpdatePowerState` | none: it only sets `isPowerOn` | fee 2 × (⌊day/10⌋ + 5); `isPowerOn` is read only by the recharger, the power block and the darkness panel (no other effect) |
| Recharger | `MachineRecharger.Recharger` | +3 per cell (+6 Commercial Power), **4 cells** → 12 / 24 | needs `isPowerOn` |
| Power block | `FurnitureItemDirectory.CreatePowerBlock` (Early pass) | +2 (+4 CP) to each edge-adjacent loose cell or battery machine | needs `isPowerOn` |
| Breeder cell | `MiscItemDirectory.EnergyCreditBreeder` | +4 to itself (cap 100) | none |
| **Electric eel** | `AquacultureHelper.GeneratePower` (Early pass) | 1 / 2 / 3 / 3 by stage, summed over the tank, **delivered in full to the RAS cell and to every edge-adjacent cell or battery machine** | ignores power and blackouts |
| Makeshift reactor | `ReactorHelper.ChargeAdjacent` | up to 4 to each adjacent item | reactor perk only |
| Current converter | `CurrentConverterHelper` | unlimited; billed ⌈nominal draw × rate⌉ even on idle nights | 450 |
| **Poison barter** | `ExchangeList.PoisonBarter` | a **new** cell charged with the two POISON items' value (capped 100); 2 nightmare caps → 100 (lab) | exchange open; once a night |
| Cells | Jackson (full 165, empty 15); buyers 80–1000 | sale value 10 + charge | — |

Draw per night: wine rack 24, projector 16, DNA 12, desequencer 10, hydroponic 8, furnace 8 (blast 16), printer 8,
evaporator 6, moisture farm 6, RAS 5 + 2 w + 3 h − 2 × electronics, purifier 4, alarm 4 (Early pass), fridge 2+,
vending 2+. Modules: base − base × E/100, truncated; too little charge skips the whole night.

### 2.5 Money-like meters

| Meter | Sources | Sinks / decay | Notes |
|---|---|---|---|
| Cash | sales (min(price, remaining budget)); vending (value +10 %); Georgio's loan (repays 4×, amount read from cash at his line on day 43); appraisal fees; captures; aug kills (100 + days since augs, cap 250); requests; story | rent (×1.1 a week, truncated; Grace defers it into `temporaryRent`, **which the buyout never collects**, lab); buyout 24 000 + 50 a day after day 50; nightly services charged **with no cash check** (power, janitor, insurance (estimated value incl. cash / 400 + 3) × 2, referrals, fixers…); permit 2000; fines; bribes | — |
| Reputation (LL, UL, SEC, BM, REV; Cartel) | trade: value × multiplier / 400 on the pre-bargain value; clean inspection +3; capture +5; aug kill SEC +8, others +2; transmitters +1 a night | contraband found, fines, wrongful reports or kills | ±200, **no decay**. Each static LL point also gives +40 Wild favor. SEC 80 = +50 % to all budgets; BM −60 = uninsurable **and no burglary at all** |
| Crime (per type) | contraband value (BM ÷2, REV ÷3, Cartel 0); FENCING = round(price × heat %) | **−20 a day per type** while evidence < 80; fixers per type | inspections only while trafficking + fencing ≥ 50 |
| Evidence | +2 … +33 a day by total crime (0 below 100) | never decays; reset by the fine or Evidence Removal | — |
| Stolen heat | 25–100 | −18 a night | detection chance = heat % |
| Aug presence | +15 per escape | −10 per kill | game over at 100; the all-clear needs the business permit |
| Burglary counter n | +1 per uninsured night (rolls 33 + 5n %) | **one insured night resets it** (lab) | rolled only while insurance is unlocked |
| Permits | business permit 2000; or weekly goods 25 + 10 × ⌊day/7⌋ from day 45 (lab); refusal crime 50 + 20 × ⌊day/7⌋ | — | gun permits I–III 450/900/1350 at SEC 20/50/80 |

### 2.6 Item loops

| Loop | Per cycle | Needs | Status |
|---|---|---|---|
| Eel tank as power hub | tank output × (1 + adjacent cells/machines): 2 adult eels → 6 × 13 = 78 units a night | `AQUACULTURE` + `_II` (2400), electric eels | read |
| Nightmare caps → poison barter → cell | 2 caps → a 100-charge cell (110 sale, 165 at Jackson) once a night | dream-cap bay + nightmare module, exchange | lab |
| Learning modules trained in an unpowered alarm | +1 per positive stat a night to 2×; kept when moved | a learning module (≈ 8.8 % of random modules) | lab |
| Keycards → engineering crate | 4 eng cards → locked crate, EV ≈ 1230 list | desequencer | read; contents hidden |
| Meat → eel fillets | ≈ +32 a night per eel | eels, feed | read |
| Filter barter | 3 spent filters → 1 new | exchange | read |
| Module barter | 2 ruined/corrupt → 1 standard (EV ≈ 77) | exchange | read |
| Junk → scrap → ingots | 6 junk → 4 scrap for 8 power | junk module | read |
| Fillerweed → omnivore fish | 690 kcal a night per bay | bay, fish | read |

## 3. Weapons

Order formulas, builds and the measured payouts are in `FORMULA-EXPLOITS.md` §2.

| Resource | Sources (hard prices) | Sinks | C/R |
|---|---|---|---|
| printed_gun (40, .22) | printer pistol chip: 2 plastic → 1, work 50; double chip 3 → 2 | orders (destroyed); weapon buyers; disassembly → 2 plastic | C |
| handmade / heavy pistol frame | printer CNC chips (handmade 455, heavy 585; need the metal 390 and CNC 520 modules): 1 ingot (+1 scrap heavy), work 60/70, draw 14; frame quality = Q/4 + ingot bonus | `GunFrameHelper.Finish` with a gun part → a **New** gun, base stats × (100 + quality) % | C |
| handmade (120, .22), heavy (160, 10 mm), glock (200), revolver (240, no mods), SMG (280), shotgun (310) | frames (handmade, heavy); gun sellers at −20 %, **Poor or Fair** (LL: handmade; merc: heavy 75 % / revolver; UL: police handgun, SMG); starts | orders; weapon buyers (contractor refuses MAKESHIFT); disassembly: handmade 2 parts + plastic + 2 scrap, heavy 2 + plastic + 4 scrap, glock 4, revolver 5, SMG 6, shotgun 7 parts (mods kept) | C |
| gun_part (25) | printer chip (300): 1 ingot → 2/3/3/4/4/5 by purity; disassembly; loot; `ReturnPlayerParts` (a declined service gun returns one per repair attempt) | frame finish; repair (spent on failure too) | C |
| mods: rail (rds 60, makeshift rds 25), barrel (silencer 55 / 90, makeshift 15, compensator 50, SMG barrel), magazine (.22: 15/25/35; 10 mm: 30/40/50; SMG 55/70/85), underbarrel (laser 50/100, toy 18, bipod, grip, launcher), stock (SMG/shotgun only), 3 special (springs 40 / improved 20, trigger 50, match barrel 75, pistol switch 40, ammo display 70, gel grip 70) | printer chips at the retired gunsmith: improved springs (2 scrap → 4), makeshift rds and toy laser (plastic + electronic → 4), rds / laser / silencer (× 2), .22 mags, 10 mm mags (metal module); the gunsmith's finished mods (+30 %); **gun sellers at −20 %** | on a gun: value added to the gun's price (no condition or markup); order rows; service orders pay them at value | C (sold with the gun) |
| ammo, ammo boxes (6 rounds) | gun sellers' rolls; starts. No recipe makes rounds | weapon buyers; a short or mixed box is exposed and costs its wrong rounds' value | C |
| gun permits I–III (450 / 900 / 1350; SEC 20 / 50 / 80) | network upgrades | shown in the case: gun-order clients (Basic 62 %, Mid ~53 %, High ~48 % of days); covered items become RESTRICTED (half markup) | R |
| repair | gun part + `GunHelper.TryRepairGun`: chance = repairability × attempts since the last success | +1 condition step (+13 / +12 % of unit value), never to New | part C |
