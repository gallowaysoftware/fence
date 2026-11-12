# Aquaculture: the RAS tank, fish and fillets (RE notes)

Build 26525552 / 0.46D. Conventions as in `RECIPES.md`: **VISIBLE** = the UI renders it (evidence = the UI
code), **HIDDEN** = internal only, **DERIVABLE** = computable from visible facts and the rules. "(computed)"
marks arithmetic done here; "(lab)" marks a verdict from `sim/lab/FishExps.cs` (docs/sim/VERIFIED.md). Types are
in the global namespace of `Assembly-CSharp`. Water chemistry of the tank is in `WATER.md` §2.4; the feed
dispenser and auto-butcher (shared with rats) are in `HUSBANDRY.md`. It supersedes the old `RECIPES.md` §7.1–7.3.

## 0. The short version

- **A fish's meat is 0.9 × every kcal it has swallowed**, the food still in its stomach included
  (`AquacultureHelper.GetMeatKcal`: floor((kcal eaten − egg grams × egg kcal/g) × conversion)). Weight, age and
  size do not enter the fillet count. A juvenile tilapia that fills its 700-kcal stomach to 85 % on its first
  night shows ~500–550 meat the next morning: **a fish fillet plus a tiny one (72 credits) from a three-day-old,
  2-cell fish**, for ~500 kcal of feed worth ~10 (lab: 54 → 504 meat after one night).
- So the limits are **how many fish hatch** (15 % a night per fed adult female, a clutch of 6), **feed**
  (industrial feed only from the retired aquafarmer, 2 bags = 10 000 kcal a week), **power** (5 a night per
  tank; three dark nights kill everything) and **room** (a hatch with no room is destroyed).
- Live fish are worth 0 at the counter (`AquacultureItems.Fish`: unitValue 0): only fillets, caviar and grafts sell.
- The best outputs per fish: tilapia ≈ 72 at three days old (computed); eels 816 at day 18 on ~3 400 kcal of
  meat; sturgeon caviar 1 200 a strip (5 jars) roughly every two weeks per female from day 18.

## 1. The tank (`ras`, `MachineRAS`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Item | `ras` 300 (+ the value of every upgrade material), tank grid 8×4 | VISIBLE | `MachineRAS.CreateRAS` |
| Panels | battery, water slot (`ras_water_tank` only, cannot be removed), 4 filter slots (any `WATER_FILTER`), dispenser slot + blend button, tank grid, auto-butcher slot + "butcher now" | VISIBLE | `CreateRAS` |
| Tank accepts | owned fish, roe, fry and any owned item with calories; never containers or machines | VISIBLE (drop refused) | tank `mayInventoryAddItemFunc` |
| Water tank | `ras_water_tank`, 12 L, value 0, starts **empty** | VISIBLE | `MachineRAS.CreateWaterTank` |
| Power | 5 a night; +2 per width upgrade, +3 per height upgrade, −2 per efficiency upgrade, floor 1. Drawn only when the tank holds water **and** live stock | DERIVABLE (draw shown on the machine) | `UpdatePowerUsage`, `IsWorthRunning`, `MachineHelper.TryDrawCyclePower` |
| Upgrades | `scrap_metal` → width +1 (max 4), `metal_ingot` → height +1 (max 4), `common_electronic` → efficiency (max 12). Full size 12×8 = 96 cells. Efficiency upgrades bought ahead still count against later size upgrades (the floor applies to the sum) | VISIBLE | `UpgradeWidth/Height/PowerEfficiency`, `ApplyTankSize` |
| Night order | early pass: electric eels charge the battery and adjacent cells/machines; main pass: draw power, then `TickTank` | HIDDEN (order) | `onCycleEndEarlySlotItemFunc`, `onCycleEndSlotItemFunc` |

### 1.1 One night in the tank (`AquacultureHelper.TickTank`)

1. **Unpowered**: every live fish, roe and fry loses 34 % of its max health (lab: 100 → 66, dead on the third dark night).
2. **Age**: unfertilised roe is fertilised if a *fed adult male* of its species is in the tank (and does not age
   that night); fertilised roe ages and hatches at 1 day into a fry group; other stock ages only if its hunger is
   ≥ 37.5 % of max, by its growth speed (fractional progress carries over).
3. **Hunger, feeding, starvation** for every live fish and fry (roe never eats).
4. **Waste**: Σ weight/100 g × waste rate ml of water turn into organic waste (tilapia 0.5, graft 1, sturgeon 1, eels 2).
5. **Remediation** (gutter tilapia, powered or not): 0.5 ml per 100 g each of heavy metal, chemical and physical
   contaminant back to water, down to the base purifier floor.
6. **Filters** (powered only): each filter takes organic waste down to 0.16 % of itself, 1 durability per unit
   removed (`WaterHelper.HandleFilter`, `Liquid` organic_waste).
7. **Water checks** (§3), then anything at 0 health dies (a group is destroyed).
8. **Breeding** (§2.3), then **wandering**: each fish whose stage sprite is larger than its current one grows
   into it **only if the tank has room**; otherwise 40 % move to a random free spot.

## 2. Fish

### 2.1 Species (`FishSpecies`)

| Species | Stage start days (roe, fry, juv, adol, adult, elder) | Weight g (juv → elder) | Water need ml (roe … elder) | Old age | Upkeep kcal/100 g | Conversion | Max hunger | Max health | Diet | Other |
|---|---|---|---|---|---|---|---|---|---|---|
| `tilapia` | 0,1,3,5,8,13 | 50, 200, 450, 600 | 100,100,100,350,750,1000 | 27 | 2 | 0.90 | 700 | 100 | omnivore | breeds; meat speeds growth +25 % |
| `gutter_tilapia` | 0,1,3,6,10,16 | same | same | 33 | 2 | 0.85 | 700 | 100 | omnivore | breeds; ignores waste; pellet-proof; cleans water |
| `graft_tilapia` | 0,1,3,5,8,13 | same | same | 27 | 2 | 0.70 | 700 | 100 | omnivore | breeds; waste sensitivity 2; skin grafts; medical roe |
| `eel` | –,–,0,5,10,18 | 150, 600, 1400, 2400 | –,–,200,600,1200,2000 | 36 | 3 | 0.90 | 800 | 120 | carnivore | never breeds |
| `electric_eel` | as eel | same | same | 36 | 5.25 | 0.75 | 1 200 | 120 | carnivore | makes 1/2/3/3 power a night (juv…elder), none while starving |
| `sturgeon` (female only) | –,–,0,10,18,34 | 500, 1500, 3500, 5000 | –,–,500,1500,3000,4000 | 80 | 2 | 0.90 | 800 | 150 | carnivore | grows eggs (caviar) |

All VISIBLE in the fish tooltip except the raw table (DERIVABLE from it): stage, age, weight, meat kcal, diet,
traits, health, hunger, pellet share, growth speed, water need, days to the next stage, nights to old age, power
output, eggs (`AquacultureHelper.CreateFishTooltip`). Elder upkeep is halved. Growth kcal = 1.2 per gram gained.
Old age: 50 % a night from the old-age day; age only counts fed nights.

Sprite sizes (cells): roe 1, fry 4 (2×2, one group of 6), tilapia juvenile 2, adolescent 4, adult/elder 6; eel 6 / 8 / 10;
sturgeon 14. A fish keeps its old sprite until the tank has room for the new one (`TryGrowIntoRoom`).

### 2.2 Hunger, feeding and growth

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Drain a night | max(5, upkeep + growth kcal of tonight's gain); a fry group pays only growth (its 6 juveniles' 300 g over 2 nights: 180 kcal a night) | VISIBLE (dev line only) / DERIVABLE | `HandleHunger`, `GetStockDrain`, `GetNightlyDrain` |
| When it eats | only below 75 % of max hunger | DERIVABLE (the "hungry" status shows it) | `TryFeed`, `AddStatusLine` |
| Feeding order | 1. slotted dispenser up to its target (default 85 %); 2. carcasses in the tank (carnivores/omnivores) to 85 %; 3. loose gutterflow pellets, then loose industrial feed bags, to 85 %; 4. any other loose food item, **eaten whole** (capped at 100 %), one item a night; 5. predation | HIDDEN (order) | `TryFeed` |
| Predation | a hungry meat-eater (below 75 %) with nothing else swallows the lightest live tank mate ≤ 20 % of its weight (roe count as 60 kcal, others their meat kcal). A 450 g adult eats juveniles up to 90 g | VISIBLE (dev line) / DERIVABLE | `TryPredation`, `CanPreyOn` |
| Kcal eaten | every kcal absorbed raises `FISH_KCAL_EATEN` — the stomach counts the moment it is filled | HIDDEN tag, but the tooltip's **meat** line shows 0.9 × it | `AddHunger`, `GetMeatKcal` |
| Growth speed | 1 − 0.25 × last night's industrial share + 0.25 × last night's meat share (meat boost: tilapia kinds only). Pure industrial feed: 0.75 (lab, from the night after the meal) | VISIBLE (growth speed line) | `GetGrowthSpeed`, `UpdateRecentDiet` |
| Diet | carnivores: meat only (no pellets, industrial feed or other food); omnivores anything | VISIBLE | `CanEat`, `TryFeed` flags |
| Starvation | first night at 0 hunger: starving; second: dies (a group is destroyed) | VISIBLE (status) | `HandleStarvation` |
| Weight | follows the kcal it has *grown* (absorbed on nights it aged), interpolated on the fully-fed curve | VISIBLE (weight) | `GetWeightForBodyKcal` |
| Pellet taint | more than 25 % of lifetime kcal from gutterflow pellets (gutter tilapia exempt): every fillet and egg jar is **trash grade** (−75 %, not food) | VISIBLE (pellet line, then the trash-grade feature) | `IsPelletTainted`, `GutterflowPelletHelper.ApplyTrashGrade` |

### 2.3 Breeding (tilapia kinds only)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Spawn | each **fed adult female** (hunger ≥ 50 %) lays one unfertilised roe at 15 % a night (lab: 12.5 %, 99 % CI 6.6–22.3, n = 120) | VISIBLE ("mature" status; roe appears) | `TryBreed`, `IsFedAdult` |
| Fertilise | the next night, if any fed adult male of the species is in the tank | VISIBLE (roe status) | `AgeOne` |
| Hatch | fertilised roe hatches at age 1 into a fry group of 6 (hunger 300) (lab: 1 night) | VISIBLE (tooltip: nights to hatch) | `HatchRoe` |
| Juveniles | the fry group ages 2 fed nights, then becomes 6 juveniles (age 3), each with 1/6 of the group's hunger and kcal; **they eat the same night**. Roe → juveniles took 4 nights in the lab on industrial feed | VISIBLE (fry tooltip) | `GrowFry`, `TickTank` loop order |
| No room | a hatch or juvenile that does not fit is **destroyed** | HIDDEN | `GraphUtils.TryAcceptAllOrDestroy` |
| Fry outside a tank | destroyed at night; a fish outside a tank dies at night | VISIBLE (status) | `InitFryItem`, `InitFishItem` |
| Plain roe | is FOOD (60 kcal) and sells: tilapia 30, gutter 15, graft 60 (medical, heals 3) | VISIBLE | `AquacultureItems.Roe` |

Expected output (computed): 0.15 × 6 = 0.9 juveniles a night per fed female; one male serves any number.

### 2.4 Yields

Meat splits greedily (`AquacultureItems.BreakDownMeat`): tilapia and sturgeon into `large_fish_fillet` 900 kcal (143,
luxury), `fish_fillet` 400 (57), `small_fish_fillet` 200 (26), `tiny_fish_fillet` 100 (13); eels into `eel_fillet` 500
(136, luxury), `small_eel_fillet` 250 (68), `tiny_fish_fillet` 100 (13). A remainder ≥ 25 kcal is `fish_scrap`
(value kcal × 4/100, min 1; not a FOOD item). Value per meat kcal: large 0.159, fillet 0.143, small/tiny 0.13, eel 0.272.
Fillets carry the species as `MEAT_TYPE_TAG`. Graft tilapia add floor(weight/200) `tilapia_skin_graft` (24, medical,
never trash grade). A sturgeon with eggs adds floor(egg g/100) caviar (240) jars on the blade.

Killing: a blade (`BLADE_ITEM_TAG`: the knives, cleaver, machete, hatchet…) kills on the first use and fillets the
carcass on the second (`HusbandryHelper` blade lambda → `AquacultureHelper.OnBladeUsed`). The carcass keeps the meat
kcal fixed at death, but **tank mates eat carcasses like a dispenser** (§2.2 step 2) and it still fouls the water.
The dispenser's blend and the auto-butcher also fillet (`MachineFeedDispenser.BlendContentsUnguarded`,
`MachineAutoButcher`).

Fully-fed curve, fish kept to age *a* and fed only its drain (computed from `GetExpectedKcalEatenAtAge`). A real fish
also carries its stomach on top: a three-day-old tilapia that filled up overnight shows ~500–550 meat (lab: 504 →
fish fillet + tiny = 70; gym: 546 → 72), as much as the curve gives at day 8:

| Tilapia age | 4 | 5 | 7 | 8 | 13 | 27 |
|---|---|---|---|---|---|---|
| meat kcal (curve only) | 137 | 221 | 413 | 511 | 716 | 792 |
| value | 14 | 26 | 57 | 70 | 96 | 100 |

| Eel age (bought at 0) | 5 | 10 | 14 | 18 | 36 |
|---|---|---|---|---|---|
| meat kcal | 542 | 1 552 | 2 277 | 3 024 | 3 607 |
| value | 138 | 410 | 613 | **816** | 965 |

| Sturgeon age (bought at 10, 500) | 18 | 26 | 34 | 80 |
|---|---|---|---|---|
| meat kcal | 3 807 | 5 181 | 6 619 | 8 689 |
| fillet value | 598 | 801 | 1 040 | 1 361 |

Electric eels: same fillets as eels at conversion 0.75 (763 at day 18).

### 2.5 Sturgeon eggs (`AquacultureHelper.StripEggs`)

From Adult, while health ≥ 90 % (135/150), eggs grow 40 g a fed night up to 15 % of the stage weight (525 g Adult, 750 g
Elder), each gram costing 2.5 kcal taken out of the meat. `surgery_tool` (kept; sold by a supplier,
`StoreClientListSupplier`) strips floor(egg g/100) jars of 100 g: the fish's jars 1–10 are caviar (240), 11–16 alternate
caviar and `sturgeon_roe` (120), later jars roe. Health cost: 20 % of max on a full load, up to +30 % when early
(`GetSurgeryHealthCost`). A full Adult strip: 5 caviar = 1 200 (computed); regrowth needs health back at 90 %
(+10 % a fed, undamaged, powered night). Eggs and jar counts are VISIBLE (tooltip); the jar sequence is HIDDEN
(dev line only) but DERIVABLE from the rule.

## 3. Water, waste and power for fish

| Check | Effect | Vis. |
|---|---|---|
| Ratio = tank ml / Σ water need of the stock | < 0.5: everything dies; < 1: damage max health × (1 − ratio) | VISIBLE (tank tooltip ml/need, "low water") |
| Waste % = organic waste / total volume | ≥ 8 % (4 % graft): dies; > 3 % (1.5 %): damage up to 50 % of max health; gutter tilapia ignore it | VISIBLE (tooltip, colour bands, "polluted"/"toxic") |
| Regeneration | no damage, powered, and hunger ≥ 50 % (or roe): +10 % of max health | DERIVABLE |

Volume never falls: waste is converted water and filters turn it back. Only the volume and the organic share matter;
the grade of the water poured in does not (`WATER.md` §2.4). 6 L (one `water_jug`) carries a pair and their young
(need 1.6–2.7 L in the gym). Waste per night is small (a 600 g adult: 3 ml) but builds: in the lab a fed pair without a
filter died of waste within 120 nights; one `water_filter` in a slot keeps it near 0.

Power is the real hazard: the Aquafarmer kit's two 50-charge cells last 10 nights at 5 a night. Two efficiency
upgrades bring a tank to 1 a night. Cells (`energy_credit`) and the `current_converter` (1.5 credits a unit from the
shop's cash) come from Dr. Jackson (`StoreClientList.CreateInventor`).

## 4. Getting fish and feed

| Source | What | Evidence |
|---|---|---|
| Aquafarmer start (`StartType.AQUAFARMER`) | −450 cash, rent +40; 2 shabby RAS each with an (empty) water tank and a 50 cell; adult tilapia pair (age 8); fertilised tilapia roe; 2 industrial feed; scrap; electronic; kitchen knife; marking pen; 2 jugs of ghost water (6 L each); `AQUACULTURE` unlocked | `NewGameData` AQUAFARMER branch |
| Is it selectable? | **Yes.** `StartingScenario.Vanilla` registers it with `CreatePending` (English fallback text) and neither `comingSoon` nor `hidden`; `GetVisible` lists it and `CanStart` passes (`IsAvailableInBuild` is always true) | `StartingScenario` |
| `AQUACULTURE` (400; needs `WATER_TRADER`) | a RAS + tank, fertilised roe, a 50 cell, a jug of rust water, 2 industrial feed, knife, marking pen; queues the retired aquafarmer | `NetworkUpgradeList` |
| Retired aquafarmer | every day with day % 7 = 2 once AQUACULTURE is owned: RAS, water filter, dispenser, auto-butcher, marking pen, cleaver (equipment 25 % each after AQUACULTURE II), **2 pellets, 2 industrial feed**, fertilised roe ×4 (tilapia ×2, gutter, graft); +3 random big fish with AQUACULTURE II (eel or electric eel juveniles at 100, sturgeon adolescents at 500) | `StoreClientManager`, `StoreClientListSpec.CreateRetiredAquafarmer` |
| `AQUACULTURE_II` (1 200) | an eel pair | `NetworkUpgradeList` |
| Feeds | `industrial_fish_feed` 5 000 kcal, value kcal/50 (0.02 a kcal), splits ≥ 1 000; `gutterflow_pellet` 4 000 kcal, 40 (0.01) — never crafted | `IndustrialFishFeedHelper`, `GutterflowPelletHelper` |

So the only bulk fish feed is **10 000 industrial + 8 000 pellet kcal a week**, and pellets are safe only for gutter
tilapia or under a quarter of a fish's diet. Other food works for omnivores through the dispenser (no growth penalty);
meat is +25 % growth for tilapia.

## 5. Max output (computed, then measured)

Per tilapia tank: the binding limit is hatches (0.9 juveniles a female-night), then feed and power, then room.
A juvenile harvested the morning after its first fill is ~72 credits for ~500 kcal (10 credits of industrial feed);
kept longer it adds ~0.9 × its nightly drain (~13 credits a night as an adolescent) while taking 2–4 cells. With two
females and a male per 32-cell tank: ~130 a night of fillets at full breeding (computed); the feed cap is
~10 000 kcal a week ≈ 20 juveniles ≈ 1 440 a week unless other food is bought.

| Choice | Best | Why |
|---|---|---|
| Species | tilapia (breeds, sells as fish fillets) | eels and sturgeon pay more per fish but must be bought and eat meat |
| Harvest | the morning a grower's meat passes 400 (a fish fillet), sooner when room or feed is short | the stomach counts: the first fill is the cheapest 500 meat a fish ever eats |
| Breeders | 2 females + 1 male per tank (more as the tank grows), heirs kept before old age | output scales with females; a male serves all |
| Feed | industrial feed in the dispenser (or a bag loose in the tank), other cheap food when short | supply, not price, binds |
| Power | electronics first (draw to 1), then size upgrades; a converter once the kit cells run down | three dark nights kill the stock |
| Water | 6 L a tank is enough; one filter | waste builds slowly |

Measured on hard: §7.

### 5.1 Paper best case against the gym (computed, 2026-11-12)

The Aquafarmer start's two tanks at their source-code ceiling, per night:

| Output | Paper | How | Gym (`aqua_fish_max`, median a day) |
|---|---|---|---|
| Tilapia fillets | ~320 | 2 females a 32-cell tank × 2 tanks × 0.9 juveniles + the aquafarmer's 4 roe a week (≈ 3.4 juveniles a night): ~7 a night at ~70, but feed caps it at ~4.5 a night (≈ 18 000 kcal a week: 10 000 industrial, ≤ 2 500 pellets under the 25 % taint line, ~5 000 from one fillerweed bay) | 102 (fillets sold) |
| Eel uplift | ~30 | fillets fed to 2 eels at 0.245 a kcal instead of sold at 0.14; capped by what two eels drain (~300 kcal a night) | ~25 (4 800 of eel fillets a run) |
| Caviar | ~86 | one sturgeon from day 18: 5 jars (1 200) every ~14 nights for the first 10 jars | ~0 (rarely bought: 500 + markup) |
| Costs | −45 | feed ~30, filters ~10, power ~3 (converter at draw 1) | — |
| **Net** | **~390** | | **~125** |

The gap is about 3×. In the runs it comes from: feed (75 hungry tank-mornings a run with the big tank; the aquafarmer is
the only bulk seller), breeding gaps when old breeders die before an heir comes of age (15 % a night is lumpy), eels lost
to waste on the 6 L the shop's water allows (24 a run died, mostly in the big tank), the sturgeon's cash gate, and every
capital purchase (AQUACULTURE_II 1 200, converters, eels) landing in the weeks the cash buyout needs the money.

## 6. Player-visible boundary (what the executor may read)

The executor (`sim/gym/Fish.cs`) reads only what the tooltips show or a player derives from the rules: species, sex,
stage, age, weight, **meat kcal**, hunger, health, growth speed, water need, the tank's water ml / need and waste %,
the battery charge, the dispenser's stored kcal, filter durability. It never reads `FISH_KCAL_EATEN`, the egg-jar
sequence or RNG state. It calls the game's own blade, blend and upgrade actions. One headless difference: the gym fillets with
`CreateFilletPieces` + `Destroy` (the body of `OnFillet`), because `OnFillet`'s destroyed-guard reads every headless
item as destroyed (no UI handler; `GameItemElement.IsDestroyed`).

## 7. Measured (gym, hard)

`engine: fish` on the Aquafarmer start, `rancher_sprinter`'s policy, 60 paired seeds (docs/research/ROUTES.md, "The fish
engine on the Aquafarmer start"): per run (median) 84 fish filleted from 27 clutches, **7 700 credits of fillets
sold (~80 a day)** for ~600 of feed, 2 fish lost, 4–5 unpowered mornings. Net worth +1 200 at day 30 and +2 500 at
day 100 against the same start without the engine; win rate unchanged (53/60 against 52/60, McNemar p = 1.0; cash
buyouts 23 against 18, p = 0.38). The binding limits seen in the runs: feed (20 hungry mornings a run: the
aquafarmer's 10 000 kcal a week is used up) and power before Dr. Jackson's converter.

**Round 2 (2026-11-12; `sim/lab/scripts/arms-fish2.txt`). `aqua_fish_max` and the Board-off arms ran on build 782ca8b plus
the permit gate (`var/sim/exp/fish5`), the others on 782ca8b (`fish4`).**
The engine at max output — eels and sturgeon (`fish_big`), the aquafarmer's roe (`fish_roe`), fillerweed bays
(`crops`, through the wine engine's bays), stock put in water right after purchase, a converter per tank, two live
filters in the big tank, a spare blade — 60 paired seeds 8710001–060, wins = any buyout (Wilson 95 %, McNemar exact
against `aqua_none`):

| Arm | Wins | Cash buyouts | Fish income a day (median) | nw100 (mean) |
|---|---|---|---|---|
| `aqua_none` | 52/60 (76–93 %) | 18/60 | — | 21 836 |
| `aqua_fish` (defaults) | 53/60 (78–94 %), p = 1.0 | 22/60 | 83 | 23 917 |
| `aqua_fish_max` (+ eels, sturgeon, roe) | **59/60 (91–100 %), p = 0.039** | 0/60 | 102 of tilapia + ~25 of eel | 18 400 |
| `aqua_fish_weed` (+ fillerweed bays) | **60/60 (94–100 %), p = 0.008** | 4/60 | 41 (+ ~16 of fillerweed sold) | 20 386 |
| `aqua_fish_all` (all of it) | **60/60 (94–100 %), p = 0.008** | 0/60 | 67 + ~24 of eel | 17 638 |
| Board off: `aqua_none` | 0/60 | — | — | 3 122 |
| Board off: `aqua_fish` | 0/60 | — | 48 | 3 667 |
| Board off: `aqua_fish_max` | 0/60 | — | 58 | 2 856 |

The bigger engines make more fish and win more often, but every one of their wins is the mortgage's end (day 195): the
capital they buy is the cash the sprint policy would have bought the shop with. Without the Board nothing wins: fish
income (~50–60 a day) is below the hard rent, and the extra spending ends runs earlier (day 125 against 146), 12 of 60
in an arrest for an unpaid permit even with fish capital held back until the permit is bought.

