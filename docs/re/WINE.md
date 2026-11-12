# Wine value: grade, buyers, upgrades (RE notes)

Build 26525552 / 0.46D. Read from code; numbers marked *measured* come from the gym (the game's own
code run headless, `sim/gym`). The chain itself (bottle, fermentation, tiers, aging) is in
[`PRODUCTION.md`](PRODUCTION.md) §1–2 and the water side in [`WATER.md`](WATER.md) §2.3–2.5; this file
answers one question: **what makes a bottle worth more, and who pays the most for it.**

## 0. The short version

1. A bottle's price is its grade times its age. Grade comes from the score fixed at the start of
   fermentation (`WineHelper.StartFermentation`): the sum of the berries' points **with no limit on how
   many berries go in**, +1 for two or more berries, +1 for any yeast, and the water term (HQ or Pure +1,
   Base 0, Ghost −1). Score 6 = Master Vintage (value 175), 5 = Reserve (150), 3–4 = Production (125).
   Each night after it finishes, a bottle gains +3 unit value, multiplied by its grade (`AgableHelper.OnAge`).
2. Berry points come from the plant's **health at harvest** (`PlantList.RollBloomberryCondition`), and health
   comes from the bay's water: contaminants cost health every growth point (`HydroponicHelper.HandleContaminant`),
   minerals outside 125–275 bp cost 1 % a point (`HandleNutriment`). The gym's bays run on ~98.4–99.2 % water,
   and its berries come out 19 % moldy, 41 % under/over-ripe, 32 % normal and **9 % best** (*measured*, 8 hard
   runs, 5 312 berries). With Pure water and nutrient tablets 62 % are best (`WATER.md` §2.3).
3. The owner's wine upgrades work through those two numbers: the DNA manipulator (with its precision module)
   through bay health (more best berries), the racks through age. None of them changes the
   price a buyer pays for a given bottle.
4. Who buys changes the price only through the **Upper Levels Tips** feature (+15 %, any Upper Levels buyer
   once UL reputation reaches 20) and through the **budget** (`GameItem.OnAddedToWeightedArea`). An Upper Levels
   Hedonist pays a Worn-out Spacer's price plus Tips, and has 350–435 + day to spend against
   145–180 + day/4, so it takes two or three bottles where the spacer takes one. On hard there is no retail
   markup (`DifficultyRules.retailMarkup` unset).

## 1. Grade

| Score input | Points | Code |
|---|---|---|
| Each berry: best (+2 grade) / normal (+1) / under- or over-ripe (−1) / moldy (−2) | 2 / 1 / 0 / −1 (a negative condition is raised by one) | `StartFermentation`: `if (c < 0) c++` |
| Two or more berries | +1 | same |
| Any yeast | +1 (only the last yeast's power sets the speed) | same |
| Water in the bottle | Pure / HQ +1, Base 0, Ghost −1, Rust −2, Gutter −3 (true purity of all the liquid in the bottle at the start, so mixed pours average by volume; the bottle's own water label is a fixed dummy and never read) | `GetWaterQualityScore` |

A score already on the bottle before the start counts too (the start adds to the stored score; a fresh or
emptied bottle holds 0). The finished grade is VISIBLE (the bottle's label art and its quality line) and,
unlike water, is never mislabelled: its label and truth are set together, so it is priced right from the
first morning. Only the score number itself is dev-only.

Tiers (`GetWineQualityTier`, `ItemFeatureList.CreateWineQualityFeature`): ≥ 6 Master Vintage +75 %, 5 Reserve
+50 % (also `LUXURY_ITEM`), 3–4 Production +25 %, 2 Homebrew 0, 1 Bucket Brew −15 %, ≤ 0 undrinkable. The bottle's
grid is 6 × 2 cells, so a bottle can take far more berries than the two the gym used to give it: four normal
berries + yeast reach 6 on Base water, three on HQ water.

What the points cost. A berry sells for 14 as food in any grade. The step 4 → 5 and 5 → 6 is worth +25 at
finish (+43 % more after aging, which is multiplied by the grade); 3 → 4 is worth nothing. So a normal berry
is worth 0 or 25 in a bottle depending on where it lands, and a second normal berry that only lifts a bottle
from 3 to 4 is wasted: sold as food it would bring 14.

## 2. Berry health: where the grade really comes from

`RollBloomberryCondition(health)` at harvest: ≤ 25 → 45 % moldy / 55 % under-over; ≤ 55 → 25 / 45 / 30 % normal;
≤ 85 → 25 % under-over / 45 % normal / 30 % best; > 85 → 40 % normal / 60 % best. Health resets to 100 when each
fruiting cycle starts (`StartNextFruitingCycle`) and loses, per point of the bay's nightly speed (so 24 points in
the 3-night first cycle and 16 in each 2-night refruit at speed 8, although the work is 22 / 12):

- **Contaminants** (`ApplyContaminantEffect`): each of heavy metal, chemical, microbe, organic waste, acid and
  base is read in basis points of the water, multiplied by the seed's `CONTAMINANT_SENSITIVITY` stat, and
  mapped to a stepped loss (0 % up to 10 bp, 0.15 % at 15, … 1.6 % at 100, 6.4 % over 200), times the plant's
  `contaminantEffect` (bloomberry 1.25). The loss depends on the individual contaminants, not the grade: bought
  Basewater and tap water made "base" with a filter and tablets both cost ~4 % a point (the tablet leaves ≈ 70 bp of
  chemical, the filter all the heavy metal), bought HQ ~0.9 %, purifier HQ ~2.75 %, Pure 0 (`PLANTS.md` §2).
- **Minerals** (`HandleNutriment`): below 125 bp −1 %, above 275 bp −0.5 % a point; a nutrient tablet adds
  15 ml of mineral.

So berry quality is a water problem. The three player tools that attack it:

| Tool | What it changes | Effect for bloomberries |
|---|---|---|
| Better water in the bay (HQ or Pure, plus tablets) | contaminant bp | bought HQ ≈ 70 health at a refruit (56–85 band: 30 % best), purifier HQ ≈ 56, Pure 100 (60 % best) |
| Water purifier (`MachinePurifier`, 250; Dr. Jackson, Retired Water Merchant) | lifts one container a night to the 99.01 % cap (HQ) | HQ water for the bays, and +1 on every bottle filled from it |
| DNA trait `hardy` (from fillerweed rank 3, nutrifruit or hydroreed rank 1) | `CONTAMINANT_SENSITIVITY` × (1 − 0.30 × rank^0.75), applied to each contaminant's bp before the step table | tap-treated water: 4 % → 3 % a point at rank I (refruit 36 → 52, same 26–55 band), 1.75 % at II; purifier HQ: 2.75 → 1.37 % at I (refruit 78) |
| DNA trait `purifying` (hydroreed, rank 4) | removes 10 % × rank^0.75 of every non-mineral contaminant from the bay's water each growth night, down to each liquid's floor ÷ rank (`PlantWaterQuality.Purify`) | rank I converges to chemical 16 / heavy metal 25 bp: 0.62 % a point, refruit ≈ 90 (> 85: 60 % best) on any water ≥ base, in about two weeks |

`bountiful` (+20 % × rank^0.75 harvest) and `fast_grower` (+25 % speed) add berries, not points; in the gym
berries are not the binding input (§5), and at speed 8 a `fast_grower` I bloomberry still needs two nights a
refruit (work 10); at bay performance ≥ 50 (speed 10) it fruits every night, which bare needs performance 100.

## 3. The DNA manipulator and its modules

Unlocked by Wild's `PLANT_DNA` (1 250, × 1.5 on hard = 1 875; needs `RETIRED_FARMER`). Once unlocked, Dr. Jackson
stocks the manipulator 50 % of visits (`PreBuiltItemHelper.CreateDnaManipulator`, value 1 000), one to five blank
disks (140 each) and each module 25 % of visits (`StoreClientList.PlaceInventorInventory`).

`MachineDnaManipulator`: 12 power, 10 progress a night. Each night it plans one operation (`PlanOperation`):

- **Extract** (a seed in the target slot, a blank disk in the output slot): picks a random
  unlocked trait of the seed (`PlantGeneticsHelper.PickRandomTrait`) and writes it to the disk with as many uses
  as the seed's current rank of it when the trait is innate to that species, else 1 (`ExtractionUses`: fillerweed
  `hardy` 3 uses, hydroreed `purifying` 4). **The seed is destroyed**, even on a failed roll, unless the
  stabilizer module is fitted.
- **Refill** (a written disk in the output slot and a seed that has its trait): no random pick; the disk gains uses
  by the same rule. So after the first disk of a trait, more uses cost one donor seed at 95 %.
- **Apply** (a written disk in the input slot): adds the disk's rank (always 1 for extracted disks) to the seed's
  trait rank, one use spent on success; a failed apply spends no use and leaves the seed plantable.
- Every operation takes `20 + 4 × complexity` progress and succeeds with `95 − 5 × complexity + quality/2` %
  (clamped 5–100), where complexity = the seed's trait ranks added beyond its innate ones × each trait's
  complexity (`hardy` 3, `bountiful` 4, `purifying` 3, …; negative traits subtract), |complexity| × innate ranks
  removed, ± 4 a rank of simple / complex genome, plus its **heat**, which grows by 2 + |trait complexity|
  after every operation, success or not, and never cools on a perennial (`AddHeat`; only a child seed starts
  cold, `InheritTraits`, and bloomberries return no seeds). A fresh bloomberry takes `hardy` I at 95 %; the
  second operation on the same seed is at ~55 %, the third ~15 %.
- A seed in a bay cannot go in (the slot refuses `GROWING_TAG`): the modified seed must be planted in an empty
  bay, or the old plant cut out with a hatchet (`RemovePlantFromHydroponic`).

Modules (`ModuleDirectory`, all `MODULE_TYPE_DNA_MANIPULATOR`):

| Module | Value | Effect |
|---|---|---|
| Precision (`dna_manipulator_module_precision`) | 1 000 | Tuned by using it on a written disk (`SetupPrecisionModule`); then extraction takes only that trait (`GetExtractionFilter`), and is blocked when the seed lacks it. Performance −75 %, efficiency −75 %: speed 10 → 6, power 12 → 21 a night (a 20-progress operation takes 4 nights instead of 2) |
| Stabilizer | 500 | The extracted seed survives, losing one rank of the trait (`RemoveTraitRank`) and gaining heat. Performance −50 %, efficiency −25 %: speed 7, power 15 |
| Positive / negative filter | 500 each | Extraction picks only good (or only bad) traits; both together block extraction. Efficiency −25 %: power 15 |

So the precision module only matters for the first disk of a trait: without it, a fillerweed seed (edible,
unappetizing, hardy, bountiful, fast grower, thirsty, nutrient-poor; simple genome is locked) gives `hardy` one
time in seven, a hydroreed (purifying, dense, hardy) one time in three, and each try destroys a seed and fills a
140 blank disk with whatever it drew. After that, refills are deterministic. Full trait and donor tables:
`PLANTS.md`. A gene scanner (`gene_scanner`, Jackson) shows a seed's traits,
ranks, complexity and heat (`CreateSeedGeneticsTooltip`): the whole process is player-visible.

## 4. Aging and racks

- Every finished bottle anywhere in the shop ages a step a night: `AGE_INT` +1, `unitValue` +3, so +3 × the
  grade multiplier in value (Production +3.75, Reserve +4.5, Master +5.25). No cap.
- The AgeWell rack (`MachineWineRack.AgeWell`, 350; Dr. Jackson once WINEMAKER is unlocked) has a 6 × 4 grid, which holds **6** bottles (a bottle is 1 × 4; docs/research/PRODUCTION-VERDICTS.md), and,
  with 24 power a night, ages each `1 + quality%/125` extra steps (`GetAgingDays`): two steps a night with its own
  tick (*measured*, `docs/sim/VERIFIED.md`). 24 power is ~24 credits of cells a night, so a rack pays from about
  five Production bottles (or four Masters) inside it.
- A bottle in a rack is a child of the rack: it is not on the shelf for buyers or for the display case.

## 5. Buyers

All wine buyers pay the counter price (`GetNegociatedValue`); nothing in a buyer's code reads the wine's grade.
The price differs only by these features, added when the item lands on the counter
(`GameItem.OnAddedToWeightedArea`):

| Feature | When | Effect |
|---|---|---|
| Retail markup | buyer accepts it and the difficulty has one (normal 15 %, hard none) | + markup % |
| Tips (`ItemFeatureList.Pourboire`) | Upper Levels buyer, `UL_TIPS` perk (UL reputation ≥ 20) | +15 % |
| Market events | any; wine is in no event pool | — |

| Buyer | Faction | Budget | Takes |
|---|---|---|---|
| Upper Levels Hedonist (`StoreClientListTierSubstance.CreateUpperLevelHedonist`) | Upper Levels | 350–435 + day | any `SUBSTANCE`; the display case's tier-3 substance buyer (case substance value ≥ 350, day ≥ 14) |
| Worn-out Spacer | Lower Levels | 145–180 + day/4 | any `SUBSTANCE`; tier 2 |
| Discerning Shopper (`CreateDiscerningShopper`) | Upper Levels | 750–1 000 + day | `LUXURY_ITEM` (Reserve and Master wine); comes at 1 + UL rep/5 % a day while a luxury item is in the case and `UL_DISCERNING_TASTE` is held, 5-day cooldown |
| UL visitor (luxury) | Upper Levels | 250–300 + 2 × day | `LUXURY_ITEM` |
| Merchants, tourists, miners, addicts | various | small | substance by tag |

So the Hedonist is the best ordinary buyer only because of Tips and room in the budget for several bottles.
In the gym 64 % of bottles already go to Upper Levels buyers, mostly Hedonists (*measured*, base policy).

## 6. Why holding wine to age lost (`wine_min_age: 10`, −14 days on the pre-appraisal policy)

Aging itself is real money: ten nights add ~38 to a Production bottle and ~52 to a Master Vintage. The hold lost for
three reasons, all measured on 16 hard seeds (1800001–016, appraisal policy, days 1–99):

1. **It wasted the display case.** Held wine stayed on the shelf, so `window_mode: concentrate` kept showing it, and a
   case worth ≥ 350 in `SUBSTANCE` brings an Upper Levels Hedonist every opening (`ShowcaseHelper.CreateClientByStockType`).
   The Hedonist then found the wine refused (`KeepWhy` "wine_aging"). With the hold: 755 Hedonist visits, 346 of them
   buying nothing, 170 spent per visit; without it: 244 visits, none empty, 469 per visit.
2. **It parked the binding input's output.** The cellar makes ~1.2 bottles a day, limited by 750 ml water rations
   (§7). Holding ten days keeps ~12 bottles (~1.5 k) off the market, and on the pre-appraisal hard policy cash was
   the binding limit (rent rising 10 % a week, the mortgage float and payment floor).
3. **It released only at `Liquidating`**, after the shop was already short, so held bottles went out at the
   liquidation floor (`sell.liquidate_min_pct`). Even with aging the average bottle sold for less (116 against 125).

Fixed here: held wine now stays out of the display case, and `wine_hold_cash` holds only while cash covers rent,
floor and a margin. Aging still does not pay in the gym (§7): racks lift the bottle to ~142 but fewer bottles are
sold.

## 7. What the gym does, and what the levers did

**What binds the gym's cellar** (instrumented pre-appraisal base policy, 8 hard runs 1800001–008): empty bottles sit idle (4–6 on an average
evening), unused berries pile up (16–23, and more than half of all berries are sold as food at 14), so the binding
input is **water for the bottles** (rations from sellers). Grade is bound by **berry quality**: 19 % moldy, 41 %
under/over-ripe, 32 % normal, 9 % best, i.e. ~0.5 points a berry, because the bays run on 98–99 % water. Bottles
average 125–130 at sale, mostly Production (125); 55–65 % go to Upper Levels buyers (the Hedonist). On the appraisal policy UL reputation is past the Tips
threshold (20) by day 100 in 14 of 16 runs, so most of those sales already carry the +15 %.

**Hardy as an upper bound.** A test-only build that gave every bloomberry seed `hardy` for free (not a player
action, not committed): best berries 6 % → 18 % (rank I) → 27 % (rank II), price 125 → 142 → 154, wine income
+170 ± 64 / +248 ± 72 a week, net worth at day 100 +2.2 k / +2.7 k (16 seeds). That is the most the DNA route can
add, and only from day 1.

**The levers** (`sim/gym/Wine.cs`, `sim/gym/Dna.cs`; selection on 16 hard seeds 1800001–016 against the appraisal
incumbent, paired; all 16 won in every arm, so the columns are net worth at day 100, win day and wine):

| Arm | nw100 Δ | win day Δ | wine/week Δ | price | notes |
|---|---|---|---|---|---|
| `wine_score: 5` (no wait) | −0.3 k ± 2.5 | −0.4 ± 1.5 | −3 ± 123 | 125 | berry points are scarce: ~1.3 a bottle |
| `wine_score: 6` (wait 2) | −1.0 k ± 2.8 | +0.9 ± 1.7 | −99 ± 148 | 129 | bottles wait for berries that do not come |
| `wine_min_age: 10` + `wine_hold_cash: 1000` | −2.7 k ± 3.3 | +1.8 ± 2.2 | −42 ± 148 | 121 | 8 seeds |
| + `wine_racks: 2`, min age 15 | −1.8 k ± 1.3 | +2.2 ± 1.6 | −29 ± 78 | 139 | aged 9 days on average; 9 % fewer bottles |
| `hold_for` SUBSTANCE for Upper Levels | −0.8 k ± 2.3 | 0 ± 1.3 | −46 ± 134 | 128 | UL share 56 → 86 %, fewer bottles sold |
| `wine_purifier` | −0.6 k ± 2.4 | −0.9 ± 1.7 | +32 ± 146 | 119 | 8 seeds, pre-appraisal Jackson path; in the runs checked a purifier was first offered after day ~160 |
| `wine_dna` | −2.7 k ± 1.2 | +3.7 ± 1.8 | −97 ± 111 | 132 | PLANT_DNA day 34–58; first Hardy seed day 39–128 (median ~77); bred bays in 13/16 |
| `wine_dna` + `wine_dna_ranks: 2` | −2.8 k ± 2.3 | +3.5 ± 2.6 | −108 ± 176 | 129 | 8 seeds |
| `wine_dna` + `wine_dna_precision` | −3.1 k ± 1.3 | +4.4 ± 1.7 | −126 ± 106 | 130 | the module only helps after the first Hardy disk |
| `wine_dna` + `jackson_every: 7` | −4.9 k ± 2.3 | +5.2 ± 1.5 | −170 ± 134 | 134 | |

Why DNA loses although Hardy pays: it costs ~3–3.5 k (PLANT_DNA 1 875 on hard, the manipulator 1 000 and disks 140
each, plus Dr. Jackson's +50 % hard markup),
the manipulator comes from Dr. Jackson half the time (visits every 14 days), a blind extraction finds Hardy one
time in 3–7 and destroys the donor, and a replanted perennial restarts its 3-night first cycle. Bred plants fruit
from day ~80 at best, so at most ~13 weeks × ≤ 170 before the win on day ~174. The owner's game reached day 100
with 30 k net worth and kept playing; the payback exists over a longer game, not inside the gym's 25-week buyout.

Fresh-seed confirmation on current main (60 hard seeds 1800301–360, all arms 60/60 wins, median win day 174 →
181 with DNA): `wine_dna` −1.5 k ± 0.9 k net worth at day 100, +3.0 ± 1.0 win days, −3 ± 54 wine a week (129 a
bottle); racks −0.8 k ± 1.0 k, +2.6 ± 1.0 days, +70 ± 51 a week (141 a bottle). Details, and the pre-rebase check
of `wine_score: 5`, in PROGRESS.md, "Wine levers".
