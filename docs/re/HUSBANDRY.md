# Husbandry: rats (RE notes)

Build 26525552 / 0.46D. Same conventions as `RECIPES.md`: **VISIBLE** = the UI renders it (evidence =
the UI code), **HIDDEN** = internal only, **DERIVABLE** = computable from visible facts. Types are in
the global namespace of `Assembly-CSharp`. Everything is read from code, not play-tested. "(inferred)"
marks a conclusion drawn from the absence of code, "(computed)" marks arithmetic done here. Values are
`unitValue` before features, markup and events (`PRODUCTION.md` §0).

Scope: rats, cages, the feed dispenser, the cage page of the auto-butcher, serums, marking pen, rat
trap, vacuum robot, gene scanner, the rancher upgrade and client. Fish and the RAS are in
`AQUACULTURE.md` and `WATER.md`; this file covers the dispenser and auto-butcher code they share.
There is one species, `AnimalSpecies.Rat` (`ANIMAL_RAT`); `AnimalSpecies.All` holds nothing else.

## 0. The short version

- A rat is born at 5 g, gains weight on a fixed age schedule scaled by its growth gene, **peaks at
  age 10**, and then loses about 17 g a night. A rat with base genes peaks at **407 g**. Cut value is a
  step function of weight; the big step is **401 g** (one `raw_meat` plus a rounded-up `small_morsel`,
  174). Rats with growth below about 0.997 never reach it.
- Rats eat only from the dispenser's "other food" and meat buckets, never pellets or fish feed. A
  rat burns its hunger gene (base 65 kcal) a night whatever its size, so a grower costs about
  950 kcal from birth to age 10, or 25 credits of the cheapest food (computed).
- Every mature rat in the cage breeds. Births outrun what the cage can hold by about 3×, so **space
  is the constraint, not fertility**. Growth rate (space per credit) and immunity (disease) are the
  genes that pay; fertility and litter size do not.
- Base immunity is 30, and below 50 a rat falls ill on about 13 % of nights. A sick rat cuts at
  ×0.75 weight, and 30 % recover each night.
- Upgrading the cage (an ingot per row, 3 to 8 rows) is the best purchase: about +50 credits a night per
  ingot (computed).
- Best rule (computed, §12): cut each rat on the first night its cut value reaches the best value it
  can reach by age 10; fill the cage to about 85 %; cull surplus newborns (15 each) first; breed
  from the highest-growth, high-immunity rats. About **450 credits of meat a night from a full
  80-cell cage**, against about 220 for a fixed 400 g rule with sick culls. Whether that much meat
  sells is a demand question (§12.5).

## 1. Items and where they come from

| Item | Value | Size (cells) | Role | Evidence |
|---|---|---|---|---|
| `animal_cage` | 250 (+40 per row upgrade) | 4×3 | 10×3 grid, a dispenser slot and an auto-butcher slot | `AnimalCage.CreateAnimalCage` |
| `livestock_feed_dispenser` | 300 | 3×3 | kcal store, 40 000 max | `MachineFeedDispenser.CreateFeedDispenser` |
| `auto_butcher` | 400 | 3×3 | 10×10 output tray; no power | `MachineAutoButcher.CreateAutoButcher` |
| `marking_pen` | 50 | | 5 colours + eraser | `AnimalMarkHelper.CreateMarkingPen` |
| `vacuum_robot` | 250 (+33 with incinerator) | 3×2 | 8×4 feces store | `HusbandryDirectory.VacuumRobot` |
| `serum_red`, `serum_green` | 100 each (10 uses, 10 per use) | | injected 20 µL per use | `HusbandryDirectory.SerumRed/SerumGreen` |
| `gene_scanner` | 500 | | toggle; shows alleles | `AmenitiesItemDirectory.GeneScanner` |
| `mouse_trap` | 50 | 3×1 | catches wild rats | `RatTrap.CreateMouseTrap` |
| `surgery_tool` | 200 | | neuter or abort | `MedsItemDirectory.SurgeryToolset`, `HusbandryHelper.InitAnimalSurgeryTool` |
| `rat` | 0, reset to 0 every night | 2–8 by weight (§3) | | `HusbandryDirectory.Rat`, `HusbandryHelper.OnAnimalItemDayEnd` |
| `feces_small` / `feces` (medium) / `feces_large` | 0 | 1 / 1 / 2 | 10/20/40 mL organic waste, 1/2/4 debris, 4/8/16 microbes | `HusbandryDirectory.Feces*` |

Live rats are worth 0, so they cannot be sold for money. Sell meat.

**`RETIRED_RANCHER` network upgrade** (750 at base difficulty, `NetworkUpgradeList.CreateRetiredRancherUnlock`;
other referral upgrades scale by difficulty, `RECIPES.md` §1.1). It drops a cage with a dispenser in
its slot, 6 rats (male and female at ages 5, 3 and 1, **random genes**), a `kitchen_cleaver`,
`labeler_blue`, `marking_pen`, 6 small nutrifruit (144 kcal each) and 3 `processed_meat` (800 kcal,
canned) into the back room, and queues the rancher for the next day (`NetworkUpgradeList`, RETIRED_RANCHER action).

**Rancher start** (`NewGameData`, `StartType.RANCHER`): the same rats with **base genes**
(`GeneticHelper.SetToDefault`), the cage and dispenser shabby, cleaver, labeler, pen, a green serum
with 2 uses, and 6 small nutrifruit. The upgrade counts as owned. The Aquafarmer start is a different
kit (RAS, tank, 2 industrial feed, knife, pen; `StartType.AQUAFARMER`).

**Retired rancher client** (`StoreClientListSpec.CreateRetiredRancher`). Visits on every day with
day % 7 = 5 (5, 12, 19 …) for the Rancher start or once the upgrade is owned
(`StoreClientManager`), and once right after purchase. A seller (`ClientIntent.SELL`) with
`premiumBuy15` = `retiredSupplierMarkup`: +0 / +15 / +30 / +45 % on cozy / normal / hard / challenge
(`ItemFeatureList.PremiumBuy15`, `Difficulty`). Every visit offers:

| Stock | Value | Odds |
|---|---|---|
| cage, dispenser, auto-butcher, marking pen, kitchen cleaver (65), `labeler_blue` (50), vacuum robot, red serum, green serum, gene scanner, `wire_cutter` (40) | as §1 | always, 1 each |
| rat ♂ and rat ♀, age 5, random genes | 120 each | always |
| rat ♂ age 5, growth gene set to base × U(1.25, 1.5) on both alleles | 500 | 25 % |
| rat ♀ age 5, same | 500 | 25 % |
| rat ♂ age 5, litter gene base × U(1.25, 1.5) on both alleles (3.75–4.5) | 500 | 25 % |
| rat ♀ age 5, same | 500 | 25 % |
| 4 small nutrifruit | 8 each | always |

Elite rats are homozygous (`GeneticHelper.SetTrait`), so every offspring inherits one elite allele.
A growth-1.25 male bred to a base female gives pups near growth 1.125, which peak at about 445 g
(computed).

**Other sources.** Dr Jackson stocks a gene scanner when none is owned and RETIRED_RANCHER or
PLANT_DNA is unlocked, always an auto-butcher once RETIRED_RANCHER or AQUACULTURE is unlocked, and a
mouse trap 5 % of the time (`StoreClientList.PlaceInventorInventory`). Scavenging finds a mouse trap
1 % of the time (`ScavHelper`). The PLANT_DNA upgrade grants a gene scanner (`NetworkUpgradeList`).

## 2. One night for a rat

`PlayerStore.EndNight` walks a **snapshot** of owned items three times (Early, main, Late;
`PRODUCTION.md` §0). Rats act in the main pass (`onCycleEndSlotItemFunc` →
`HusbandryHelper.OnAnimalItemDayEnd`), in this order:

1. value ← 0; dead rats stop here.
2. age +1, then old age (§7).
3. hunger −= hunger gene; move around or escape (§8); eat (§4); starvation (§7).
4. hygiene disease roll; spontaneous disease roll; health regen; disease damage and recovery roll;
   death at 0 health (§7).
5. overcrowding attack (§7); fertility update; growth and droppings (§3); mating (§6); pregnancy
   tick and birth (§6); serum metabolism (§5.3).

Newborns are added during the main pass, so they are not in that night's snapshot and do not tick
until the next night (inferred from `FindAllItem()` being called before the loop). The cage's wear
tick is in the same pass. The auto-butcher and the vacuum robot run in the **Late** pass, after growth.

## 3. Stages, weight and growth

`AnimalSpecies.Rat`: stage start ages 0, 1, 3, 5, 11, 14; anchor weights 5, 50, 200, 350, 450,
400 g. The stage name shown in the tooltip: infant (< 1), juvenile (1–2), adolescent (3–4), adult
(5–10), elder (≥ 11) (`GetAnimalGrowthStageDisplay`). VISIBLE.

**Growth** (`GetAnimalCurrentWeightChange`, `GrowAnimal`). Each night, after the age increment, the
rat gains the slope of the stage it is now in, (next anchor − this anchor) / (next age − this age),
× its growth stat, rounded to the nearest integer (ties to even). The slopes are +75 g (ages 1–4),
+16.67 g (ages 5–10) and **−16.67 g (ages 11–13)**. At 14 and later growth stops, unless longevity
> 1, in which case the −16.67 slope continues until age ≥ 14 × longevity. Hunger modifies the gain:

- hunger < 400 (of 500) at growth time: gain × (1 − (400 − hunger) / 800), so at most −50 %;
- starving: −½ × |slope| (always a loss);
- floor: weight never falls below 25 % of the current stage's anchor.

The age-0 slope (+45) is never used, because age is incremented before growth. HIDDEN (the
formula); weight and age are VISIBLE every night, so the curve is DERIVABLE.

Weight by age at full feed (computed with `HusbandryHelper` rules; growth stat g):

| g | 1 | 2 | 3 | 4 | 5 | 6 | 7 | 8 | 9 | **10** | 11 | 12 | 13 | first ≥ 401 |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 0.8 | 65 | 125 | 185 | 245 | 258 | 271 | 284 | 297 | 310 | 323 | 310 | 297 | 284 | never |
| 0.9 | 73 | 141 | 209 | 277 | 292 | 307 | 322 | 337 | 352 | 367 | 352 | 337 | 322 | never |
| 0.95 | 76 | 147 | 218 | 289 | 305 | 321 | 337 | 353 | 369 | 385 | 369 | 353 | 337 | never |
| 1.0 | 80 | 155 | 230 | 305 | 322 | 339 | 356 | 373 | 390 | **407** | 390 | 373 | 356 | 10 |
| 1.1 | 87 | 169 | 251 | 333 | 351 | 369 | 387 | 405 | 423 | 441 | 423 | 405 | 387 | 8 |
| 1.2 | 95 | 185 | 275 | 365 | 385 | 405 | 425 | 445 | 465 | 485 | 465 | 445 | 425 | 6 |
| 1.3 | 103 | 201 | 299 | 397 | 419 | 441 | 463 | 485 | 507 | 529 | 507 | 485 | 463 | 5 |
| 1.5 | 117 | 229 | 341 | 453 | 478 | 503 | 528 | 553 | 578 | 603 | 578 | 553 | 528 | 4 |

g = 0.99 peaks at 397 g, so the 401 g line falls at g ≈ 0.997 (computed). Feed hardly moves the curve: a
rat held at 375 hunger grows ×0.969 and a base rat then peaks at 393 g, under the line (§4).

**Body size** follows the sprite chosen by weight (`AnimalSpecies.spriteByWeight`,
`TryChangeSprite`; shapes from `var/gamedata/item_shapes.json`): < 180 g `s` 2×1 (2 cells); 180–399
`m` 3×1; 400–599 `l` 3×1; 600–999 `xl` 3×2 (**6 cells**); ≥ 1000 `xxl` 4×2. If the bigger shape
does not fit anywhere in the grid, the sprite is not changed (the weight still is). VISIBLE.

`HusbandryHelper.SimulateWeightAtAge` gives purchased and trapped rats the no-rounding version of the
same curve for their age.

## 4. Hunger and feeding

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Hunger | max 500, starts at 500; a newborn starts at 125 (25 %) | VISIBLE (bar + numbers) | `AnimalSpecies.baseStats`, `GiveBirth` |
| Burn | the hunger-rate gene (base 65) per night, independent of weight | VISIBLE (stat line) | `HandleHunger` |
| When it eats | only when hunger < 375 (75 % of max) | HIDDEN | `TryFeed` |
| From the dispenser | the cage's slotted dispenser first, then any dispenser lying in the same grid. Draws ceil(target − hunger) kcal, capped by what is stored; target = max × the dispenser's slider (default 85 % = 425) | target VISIBLE (slider, tooltip) | `TryFeed`, `GetReachableDispensers`, `MachineFeedDispenser.GetTargetRatio` |
| Loose food | if still under 375 after the dispenser, eats **one whole** calorie item lying in its own inventory (not canned, not pellets or fish feed), up to max hunger | HIDDEN | `TryFeed` |
| What a rat can eat | dispenser buckets "other food", "meat" and "rat meat" only. Pellet and industrial-feed kcal are invisible to rats | HIDDEN | `GetAnimalKcal`, `DrawKcal(dispenser, amount)` (allowPellet false) |

The slider runs 0–100 in steps of 5. A rat whose hunger gene is below 50 sits at exactly 375 after its burn
at target 85 % (425 − < 50 > 375), so it skips feeding on alternate nights and grows at about
×0.97 on those nights. At target ≥ 80 % a 65-burn rat is always ≥ 400 at growth time. **Set 100 %**:
the extra buffer costs about 75 kcal once per rat, and every rat stays above 400 (computed).

**Feed per grower** (computed, base hunger 65, target 85): first meal 365 (125 → 60 after the burn, refilled to 425),
then 65 a night for ages 2–10 = 585; ≈ 950 kcal from birth to age 10. Growth rate does not change food,
so fast growers make cheaper meat per gram.

### 4.1 Feed dispenser (`MachineFeedDispenser`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Storage | 6×4 grid, accepts any owned calorie item, live or dead animals, fish (canned food included) | VISIBLE | `CreateFeedDispenser` |
| Blend | its own button, the cage's blend button, or auto-blend (toggle, default **off**; when on, blends whenever an item is inserted). Kills and butchers animals, fillets fish, then turns every calorie item into kcal 1:1 | VISIBLE | `Blend`, `BlendContentsUnguarded`, `TryBlendItem` |
| Capacity | 40 000 kcal; an item that would overflow stays in the grid | VISIBLE (stored / 40 000) | `TryBlendItem` |
| Buckets | `STORED_ANIMAL_RAT` (meat tagged rat; a prion-tagged piece flags the bucket), `STORED_MEAT` (other meat), `STORED_OTHER`, `STORED_GUTTERFLOW_PELLET`, `STORED_INDUSTRIAL_FISH_FEED` | VISIBLE (one tooltip line each) | `TryBlendItem`, `CreateTooltip` |
| Draw order | proportional to bucket sizes across the buckets the eater may use (largest-remainder rounding). Rats: other + meat + rat meat | HIDDEN | `SplitProportionally` |
| Cannibal draw | if a draw takes any kcal from the rat bucket, the eater rolls prion (§7.2); certain if the bucket is prion-flagged. The flag clears when the bucket empties | HIDDEN | `TryFeed`, `CheckCannibalism`, `DrawKcal` |
| Empty | button with a confirmation, wipes every bucket | VISIBLE | `EmptyStorage` |
| Power | none | | |

**Never put rat meat or live rats in a rat dispenser.** Any proportional draw then touches the rat
bucket, and at base immunity 70 % of draws give prion.

### 4.2 Cheapest rat food (computed from `unitValue` and kcal)

| Food | kcal | Value | kcal / credit | Bucket |
|---|---|---|---|---|
| `beis_icecream` | 1500 | 40 | 37.5 | other |
| `processed_milk` | 600 | 16 | 37.5 | other |
| `processed_juice` | 450 | 14 | 32.1 | other |
| `processed_meat` (canned; the dispenser takes canned food) | 800 | 25 | 32.0 | meat |
| `fat_meat` | 1250 | 40 | 31.2 | meat |
| `meat_scrap` | 350 | 12 | 29.2 | meat |
| `fish_scrap` | any | kcal × 4 / 100 | 25.0 | meat |
| `cup_noodle` | 500 | 20 | 25.0 | other |
| `nutrifruit` (huge / normal / small) | 768 / 240 / 144 | 32 / 12 / 8 | 24 / 20 / 18 | other |
| `fillerweed` | 259 | 11 | 23.5 | other |
| `processed_cheese` | 450 | 20 | 22.5 | other |
| `galaxy_blend` | 450 | 25 | 18 | other |
| `li_eat_snackbar`, `cat_bar` | 250, 210 | 17, 15 | 14.7, 14 | other |
| `meal_cap` | 195 | 19 | 10.3 | other |
| `gutterflow_pellet`, `industrial_fish_feed` | 4000, 5000 | 40, 100 | 100, 50 | **rats cannot eat these** |

Sources: `FoodItemDirectory`, `HydroponicDirectory`, `PreBuiltItemHelper.CreateNutrifruit*`,
`AquacultureHelper.GetFishScrapValue`. At 37.5 kcal per credit a grower costs about 25 credits of
food (950 / 37.5, computed). At purchase markups this rises to perhaps 30–40, still about 15–20 % of a
174 cut.

## 5. Genes and serums

### 5.1 The seven genes (`AnimalSpecies.Rat.geneticTraitIDs`, `GeneticHelper`)

Each gene has two alleles. **Expressed** = their mean. **Current stat** = (expressed + flat bonus)
× (1 + bonus %), and only serums and prion add bonuses. New random rats (wild, bought, kit from the upgrade) get each
allele = base × mean of two U(0.5, 1.5) draws: a triangle on 0.5–1.5 × base. The expressed value
then has an SD of about 14 % of base (computed). Rancher-start rats are exactly base.

| Gene | Base | What it does | Shown |
|---|---|---|---|
| `ANIMAL_GROWTH_RATE_TAG` | 1.0 | multiplies every growth slope, including the post-peak loss | VISIBLE stat line |
| `ANIMAL_HUNGER_RATE_TAG` | 65 | kcal burned per night (lower is better) | VISIBLE |
| `ANIMAL_FERTILITY_TAG` | 50 | % mating chance before penalties (§6) | VISIBLE when mature (fertility line) |
| `ANIMAL_LITTER_SIZE_TAG` | 3 | mean litter (§6) | VISIBLE |
| `ANIMAL_IMMUNITY_TAG` | 30 | disease onset below 50, recovery %, regen, prion resistance (§7) | VISIBLE; plus a "strong" flag at ≥ 50 and an "immunocompromised" flag below 25 |
| `ANIMAL_MAX_HEALTH_TAG` | 100 | health pool; disease removes 25 % of it a night | VISIBLE (stat line and health bar) |
| `ANIMAL_LONGEVITY_TAG` | 1.0 | old age at age ≥ 14 × L; elder fertility penalty at ≥ 11 × L; L > 1 extends the post-peak weight loss | VISIBLE |

Not genes (fixed per rat): gestation 3 nights, litter variance ±1, at most 3 matings a night, max hunger 500
(`AnimalSpecies.baseStats`).

### 5.2 Inheritance and mutation (`ApplyInheritedGenetics`, `MutateAllele`)

- At conception the mother stores the father's two alleles per gene (`StorePartnerGenes`).
- Each pup takes one random allele from the mother and one from the stored father pair, per gene.
- Each inherited allele mutates with 15 % chance. Mutation size: uniform ±8 % × (1 − R), ×3 on 5 %
  of mutations, where R = 1 − 1 / (1 + 2 × |v − base| / base) (R = 0 at base, 0.5 at ±25 %, 0.67 at
  ±50 %). A pull of 25 % × R of the range is added toward base. Floor 0.01.
- So selection holds: a pair of growth-1.3 parents breeds about 1.3 pups. Mutation is slow and pulls
  back gently. HIDDEN (allele values are VISIBLE with the gene scanner, §11).

### 5.3 Serums (`HusbandryDirectory.SerumRed/SerumGreen`, `Liquid` `livestock_steroid` / `livestock_immunity`, `IngesterHelper`)

| Serum | Effect while active | Dose | Duration |
|---|---|---|---|
| red (`livestock_steroid`) | growth +20 %, immunity −40 %, hunger rate +25 % | 20 µL per use, 10 uses, 10 credits a use | metabolised 5 µL a night: **4 nights** |
| green (`livestock_immunity`) | immunity +75 %, hunger rate +25 % | same | same |

The bonus is applied once, when the concentration rises from 0. A second dose while active only
adds 4 more nights (`IngesterHelper.Inject`). Effects are bonus %, added to prion's −20 %. The item text
gives no numbers (HIDDEN magnitudes), but the stat lines show current vs expressed while active and
the ingester line shows µL and −5 a day (`IngesterHelper.CreateIngesterTooltip`). DERIVABLE.

- Green lifts base immunity 30 → 52.5, which stops spontaneous disease (§7.1) and caps regen at 30 a
  night. Covering a grower's 10 nights takes 3 doses (30 credits) against an expected disease loss of
  about 20–25 a grower (computed, §7.1), so it is about break-even for growers. Use it on breeders and on sick
  rats worth saving.
- Red on a rat with g in 0.85–1.0, dosed at age 1 (the 75 g/night stage), lifts its peak over 401 g
  (computed: g 0.9 with one dose reaches 419; g 0.85 needs two doses, 405). That is +49 a rat for 10–20
  credits, but immunity 30 → 18 means 18 % onset a night and negative regen. Unverified (§14).

## 6. Breeding

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Mature | age ≥ 3 | VISIBLE ("mature" flag) | `IsMature` |
| Can mate | alive, not starving, not pregnant, not neutered, mature, and under 3 matings **this night** (the count resets each night) | HIDDEN (count) | `CanMate`, `OnAnimalItemDayEnd` |
| Attempts | on its own tick each rat tries the opposite-sex mates in random order, up to 3; each try also counts against the partner | HIDDEN | `CheckMate` |
| Chance per try | min(current fertility of the two) % | DERIVABLE (both fertilities VISIBLE) | `TryMate` |
| Current fertility | fertility stat − penalties, each a share of the expressed fertility: hunger < 200: up to −40 % (linear); elder (age ≥ 11 × longevity) −15 %; diseased −75 %; overcrowding −20 % per level | VISIBLE (current vs base when they differ) | `UpdateFertility` |
| Gestation | the pregnancy counter ticks +1 a night, including the night of conception if she conceived on her own tick; birth at 3. So **birth 2–3 nights after conception** | VISIBLE (progress bar, days/3) | `HandlePregnancyProgress` |
| Litter | floor(litter stat) + 1 with chance = fraction, then + uniform {−1, 0, +1}, at least 1. Base: 2–4, mean 3 | HIDDEN | `GiveBirth` |
| Pups | age 0, 5 g, hunger 125, added to the cage; if the cage is full: hopper overflow, back room, stock, showcase, front, else destroyed | | `GiveBirth`, `GraphUtils.EmporiumTryAdd` |
| Next litter | the mother can conceive again the next night | | |
| Miscarriage | 75 % a night while pregnant and starving; the surgery tool on a pregnant rat also aborts | VISIBLE (flag) | `Miscarry`, `OperateOnAnimal` |
| Neutering | surgery tool on a non-pregnant rat: permanent, cannot mate | VISIBLE (flag) | `OperateOnAnimal` |

**Rate** (computed): a free female with one fertile male (50/50) gets about two tries a night, so she conceives
on about 75 % of nights. Cycle ≈ 1.3 + 2.5 ≈ 3.8 nights, which is **≈ 0.8 pups a night per mature female**.
One male can serve many females, because he gets up to 3 tries on his own tick and females try him too.

## 7. Health, disease and death

### 7.1 Disease (`CheckImmunityDisease`, `CheckHygiene`, `HandleRegen`, `HandleDisease`)

| Fact | Value | Vis. |
|---|---|---|
| Spontaneous onset | only if immunity < 50: (int)(lerp(25 %, 5 %, immunity / 50)) a night. Immunity 0 → 25 %, 20 → 17 %, **30 → ~13 %**, 40 → 9 %, ≥ 50 → 0 | HIDDEN (DERIVABLE from the immunity line) |
| Hygiene onset | if feces + dead bodies cover ≥ 25 % of the cage: below 35 %, a 15 % roll arms a 15 % chance; at ≥ 35 %, a 35 % roll arms 35 %; then + 20 % per overcrowding level, added even when the arming roll failed. Feces made earlier in the same night count | HIDDEN |
| Effect | −25 % of max health a night; fertility −75 %; **butchered at 75 % of weight** | VISIBLE (flag, health bar) |
| Recovery | after each night's damage, chance = immunity % | HIDDEN |
| Regen | every night: (immunity − 25) / 25 × 0.3 × max health, capped at +0.3 × max. Immunity 30 → +6; 25 → 0; **below 25 negative** (20 → −6) | DERIVABLE |
| Contagion | none: no rat infects another (inferred) | |

At immunity 30 and 100 health a sick night nets −19, so death comes on the 6th straight sick night: 17 %
of episodes (0.7⁵, computed). About 30 % of a base-immunity colony is sick at any moment
(13 / (13 + 30), computed). A grower that is sick on its peak night cuts as 305 g (110) instead of 407
(174). The expected loss is about 20–25 a grower, deaths included (computed). Most episodes end within 3 nights.

### 7.2 Prion (`CheckCannibalism`, `OnGetPrionDisease`)

- Cause: only **eating rat**. A dispenser draw that touches the rat bucket, or eating a loose rat cut
  lying in the cage. Chance (100 − immunity) %, certain if the meat is prion-tagged. Already-prion rats
  do not roll.
- Effect: permanent −20 % growth, immunity and longevity (bonus %). Its cuts carry `ANIMAL_PRION_TAG`.
  No price effect found: the tag is read only by the dispenser and the auto-butcher (inferred from grep).
  No cure; not inherited (a bonus, not an allele).
- VISIBLE (flag).

### 7.3 Overcrowding (`GetAnimalOvercrowdingLevel`, `HandleOvercrowding`)

Level from (cells of all animals, dead included) / (cage width × height): > 55 % → 1, > 70 % → 2,
> 90 % → 3. VISIBLE (each rat's tooltip shows the level).

| Level | Fertility | Attack (per rat per night) | Hygiene onset |
|---|---|---|---|
| 1 | −20 % of expressed | none | +20 % |
| 2 | −40 % | 5 %: kills the lightest live rat lighter than itself (victim gets a scratch-mark flag) | +40 % |
| 3 | −60 % | 15 % | +60 % |

The victim's carcass stays and can still be cut (a 5 g newborn → 1 small morsel, 15).

### 7.4 Death causes

| Cause | Rule | Evidence |
|---|---|---|
| Old age | from age ≥ 14 × longevity: 75 % a night (checked right after the age increment) | `CheckOldAge` |
| Starvation | hunger 0 sets "starving"; still 0 the next night → dead. Starving rats lose weight and miscarry | `HandleStarvation` |
| Disease | health ≤ 0 | `HandleHealth` |
| Overcrowding attack | §7.3 | `HandleOvercrowding` |
| Dispenser | a live rat in the dispenser grid is killed and cut on blend | `BlendContentsUnguarded` |
| Blade | first stroke kills | `InitBladeItem` |

Dead rats stay in the cage. They count toward hygiene and overcrowding, and are worth their weight when
cut.

## 8. Cage (`AnimalCage`)

| Fact | Value | Vis. | Evidence |
|---|---|---|---|
| Grid | 10 wide × 3 rows; an ingot adds a row, up to 8 (80 cells) | VISIBLE (tooltip: rows left) | `UpgradeSize`, `CanUpgradeSize` |
| Accepts | owned items, not containers; machines only if `ALLOWED_IN_CAGE_TAG` (dispenser, vacuum robot) | | `mayInventoryAddItemFunc` |
| Wear | each night: (cells of live rats) / 4, integer. The ×2 overcrowding branch never fires: nothing sets `ANIMAL_OVERCROWDING_TAG` (inferred, no writer) | VISIBLE (durability bar) | `onCycleEndSlotItemFunc` |
| Health | 100; +20 max and +20 current per row upgrade (200 at 8 rows) | VISIBLE | `UpgradeSize` |
| Repair | `scrap_metal` (10) on the cage: +50 % of max | VISIBLE (hint) | `Repair`, `MaterialDirectory.CanScrapMetalTargetItem` |
| Upgrade | `metal_ingot` (50, any purity) on the cage: +1 row, value +40 | VISIBLE | `MaterialDirectory.OnIngotTarget` |
| Escape | below 20 health: each rat each night has round(lerp(25 %, 5 %, health / 20)) chance to land in stock, back room or showcase (random). Otherwise it moves to a random free spot in the cage | HIDDEN | `GetEscapeChance`, `MoveAround` |
| Packing | rats are 2×1 or 3×1, so a 10-wide row holds three adults (90 %) or a mix of adults and pups (100 %) | DERIVABLE | |

A full 8-row cage at ~68 rat cells wears 17 a night: one scrap every ~6 nights, about 1.7 credits a night
(computed).

**Rats outside a cage** (in stock, back room or showcase) wander. Below 300 hunger, if their inventory holds no
food, they jump to another store inventory that does, and they eat one non-canned food item a night
there, **including stock for sale**. Otherwise 40 % move to a random spot and 30 % jump to another store
inventory (`MoveAround`). A rat in a mouse trap stays put.

## 9. Butchering and cuts

**Blade**: any melee item with `InitBladeItem` (`kitchen_cleaver`, `kitchen_knife`, `skinning_knife`,
`box_cutter`, `hatchet`, `combat_machete`, `glass_shard_shiv`, and one more). Owned blade on an owned
rat: the first use kills, the second cuts. **The cuts drop into the rat's own inventory**, so a rat cut
inside its cage leaves meat the others will eat (prion, §7.2) (`HusbandryHelper.OnButcher`).
`skinning_knife` shows a running butcher count (`BUTCHER_TALLY_TAG`).

**Cuts** (`HusbandryHelper.CreateButcherCuts`). Weight w (× 0.75 if diseased, rounded) is split
greedily: `raw_meat` per 400 g, then `small_raw_meat` per 200, `morsel` per 100, `small_morsel` per
50, and **any remainder > 0 adds one more small morsel**. Each cut gets `MEAT_TYPE_TAG` = rat, plus
the prion tag if the rat had it. HIDDEN (DERIVABLE by cutting).

| Cut | Value | kcal | Cells | Value / g | Value / cell |
|---|---|---|---|---|---|
| `raw_meat` (also `LUXURY_ITEM`) | 159 | 1200 | 3×2 | 0.40 | 26.5 |
| `small_raw_meat` | 65 | 550 | 2×2 | 0.33 | 16 |
| `morsel` | 30 | 275 | 2×1 | 0.30 | 15 |
| `small_morsel` | 15 | 140 | 1×1 | 0.30 | 15 |

Value by weight (computed): 5–50 g → 15; 51–100 → 30; 101–150 → 45; 151–200 → 60; 201–300 → 80;
301–350 → 110; 351–399 → **125**; 400 → 159; **401–450 → 174**; 451–500 → 189; 501–550 → 204;
551–600 → 224. Diseased at peak 407 → 305 → 110.

No value modifier touches meat: no freshness on cuts (`FoodItemHelper.InitFreshFood` is used only by
unused content), no prion or species price effect, and the fridge does not take meat (`RECIPES.md` §8).
The meat's sell price is its `unitValue` through the normal staging (`PRODUCTION.md` §0). FOOD-type
market events apply (`PRODUCTION.md` §5). `raw_meat` is `LUXURY_ITEM`, so luxury buyers want it and
mercenaries refuse it (`CLIENTS.md`).

`MachineMeatCulture` (a lab that copies raw meat) is unused content (`UnusedDirectory`).

## 10. Auto-butcher (`MachineAutoButcher`)

400, no power, 10×10 output tray. It runs on its host: put it in the cage's butcher slot (cage page)
or the RAS's (tank page). With no host it works on the loose items in whatever inventory it sits in,
using both pages. It runs in the **Late** pass every night (`onCycleEndLateSlotItemFunc`), and also on its
"butcher now" button or the cage's.

**Cage page settings** (`CagePage`, `InitSettings`; all VISIBLE in the machine window):

| Setting | Range | Default |
|---|---|---|
| Harvest | off / carcasses / weight / age | **off** (the machine does nothing out of the box) |
| Weight range | 100–1000 g, step 50; the top at 1000 means no cap | 400 – no cap |
| Age range | 1–30 days; the top at 30 means no cap | 10 – no cap |
| Keep females / males | 0–10 each | 2 / 1 |
| Count marked/named toward kept | on/off | on |
| Spare pregnant | on/off | on |
| Cull sick | on/off | on |
| Cull prion | on/off | on |
| Drop-to-butcher (machine-wide) | on/off | on |
| Kill and store only (machine-wide) | on/off | off |

Tank page: weight 100–5000 step 100 (default 600), age 1–100 (default 20), spare egg-layers on,
fillet carcasses on, cuts to dispenser off, collect roe off. See `RECIPES.md` §7.

**One run on a cage** (`RunCage`, `PickKeepers`, `CountProtectedOutside`, `IsProtected`):

1. Do nothing if harvest is off.
2. Cut list = dead rats that are not protected. "Protected" = marked with the pen or custom-named.
3. Cull sick / prion (if on): move unprotected live rats with that flag to the cut list, **at any weight or age**.
4. Weight or age mode only: candidates = live rats inside the range. Keepers among the candidates are:
   every protected candidate, every pregnant one (if spare pregnant), then the **youngest**
   non-neutered females and males until the keep counts are met. The counts are first reduced by
   protected rats *outside* the candidate set (if counting is on). The other candidates join the cut
   list.
5. Cut each rat into the tray. If not every piece fits, nothing is cut, the rat is skipped ("tray
   full"), and the cut list continues. Kill-and-store-only puts whole carcasses in the tray instead.

Consequences (computed from the rules):

- Keepers are re-picked every night **from the current candidates only**. In age mode the youngest
  candidates are kept, so tonight's keepers are cut tomorrow, one night past peak (174 → 125 for a base
  rat). In weight mode a base rat leaves the ≥ 400 range the night after its peak, so keepers
  accumulate as un-cuttable rats until old age.
- Mark the breeders with the pen and set keep to 0/0 (or leave 2/1 with counting on and mark 2 ♀ + 1 ♂).
  A run never cuts or culls a marked rat, dead or alive (dropping one on the machine by hand still cuts it).
- A 407 g rat makes 7 tray cells (raw meat + small morsel), so the tray holds about 14 rats. Empty it
  daily.
- "Butcher content" cuts unprotected carcasses lying in the tray. Drop-to-butcher processes a rat
  dropped on the machine.
- The machine cannot cull surplus newborns: the smallest weight is 100 and the smallest age 1. Carcasses
  from overcrowding attacks are cut in any non-off mode.
- Tooltip: rats cut last run (`CreateTooltip`). VISIBLE.

## 11. Other tools

**Marking pen** (`AnimalMarkHelper`): owned pen on an owned rat or fish sets a colour (red, purple,
green, blue, orange); the same colour again or eraser mode clears it. The mark is drawn on the item
(`GameItemElement`). Its only mechanical effect is auto-butcher protection (`IsProtected`). A custom
name (`CUSTOM_NAME_TAG`, `GeneralHelper.HasCustomName`) protects the same way.

**Vacuum robot** (`HusbandryDirectory.VacuumRobot`): sits in the cage grid (it is allowed in), **taking
6 cells**. In the Late pass, or on activation, it moves feces from the grid it sits in into its 8×4 store
until full. `common_electronic` (33) on it installs a toggleable incinerator (on by default after the
upgrade) that destroys the stored feces after each cleanup (`UpgradeVacuumRobot`, `ModuleHelper`). It
needs no power. It cleans after the main pass, so it does nothing for that night's hygiene roll. For an
executor that bins feces each morning it is only a 6-cell cost (≈ 30 credits a night at §12 rates,
computed).

**Droppings** (`Poop`): one dropping each night that a rat's gain is ≥ 30 g (base rats: ages 1–4 only; g ≥ 1.8
for ages 5–10). Small below 600 g, medium 600–999, large ≥ 1000.

**Mouse trap** (`RatTrap`): a 1-slot container (cannot go in a cage). Each main pass: if empty, 1 %
chance a wild rat appears; if it holds `processed_cheese` (20), 50 % a rat appears and eats the
cheese, otherwise 25 % the cheese is lost. Wild rat: random sex, age 3–7, random genes, weight for its
age (`PreBuiltItemHelper.CreateRat`). About 25 credits of cheese per rat (computed: 0.5 rats per
0.625 cheese a night). Useful for fresh genes (high-growth wild founders), not volume.

**Gene scanner** (`AmenitiesItemDirectory.GeneScanner`): toggle on while owned. Every animal tooltip
then shows both allele values per gene, coloured against base (±5 %) (`CreateItemTooltip.AddStatLine`).

**Surgery tool** (200): §6.

## 12. Max output (computed)

### 12.1 What limits a cage

- **Space.** A base grower (g = 1, cut at 407 g on age 10) occupies 2 cells for ages 0–2 and 3 cells
  for ages 3–10: 3 × 2 + 8 × 3 = **30 cell-nights**. A g = 1.2 grower cut at 405 g on age 6:
  2 + 2 + 3 × 5 = **19 cell-nights**.
- **Births are not limiting.** Growers breed from age 3. In an 80-cell cage about 10 mature females give
  about 8 pups a night, against about 2.3 needed. The surplus is culled at birth (15 each, no food) or
  dies to overcrowding attacks.
- **Food is minor:** ~25 credits per grower against 174 meat.

Per grower: 174 − 25 = 149 over 30 cell-nights = **5 credits per cell-night** (g = 1). At g = 1.2:
174 − 18 (6 nights of food) over 19 = **8.2**. Growth rate is the gene that pays.

### 12.2 Is 400 g the right cut?

No. The value steps sit at 51, 101, 151, 201, 301, 351, 400, **401**, 451 … The step from 351–399
(125) to 401 (174) is +49. Each later step is +15 per 50 g, and at 17 g a night that is ~3 nights of a
3-cell slot (~45 credits of space) for 15. Exactly 400 g is worth 15 less than 401. Rules:

1. Cut a rat on the **first night its value equals the best value it can reach by age 10**. Read the
   peak off its visible growth stat (§3 table; peak ≈ 5 + 400 × g at full feed). For g ≥ 0.997 that is
   the first night ≥ 401 g (age 10 for g = 1.0, age 8 for 1.1, 6 for 1.2, 5 for 1.3). For g in 0.88–0.997
   it is the first night ≥ 351 g (125). Never keep a grower past age 10: it loses 17 g a night.
2. Keep fast growers under 600 g (g ≥ 1.5 crosses 600 at age 10 and doubles to 6 cells).
3. A sick grower near its peak: cut it only if it is already at its best value, or if its health is
   at or below 25 % of max (one more sick night may kill it). Otherwise wait: 30 % recover each night.

### 12.3 Steady state per cage (computed)

Usable cells ≈ 85 % of the grid (packing, §8). Overcrowding level 2 (> 70 %) is acceptable: its
fertility penalty does not bind, and its 5 % attacks hit the lightest rats, mostly surplus newborns
whose carcasses still cut for 15.

| Cage | Usable cells | Growers / night (g ≈ 1) | Meat − food | + newborn culls | ≈ net / night |
|---|---|---|---|---|---|
| 3 rows (30) | 25, 6 held by 2 breeders → 19 | 0.63 | 94 | ~30 | **~120** |
| 8 rows (80) | 68 − 6 = 62 | 2.1 | 308 | ~75 | **~380** |

A Monte Carlo of the §2–§7 rules (300 nights, 30 seeds, random-gene founders, feed at 37.5 kcal/credit,
morning actions only; geometry approximated by a cell count) gives, for 80 cells:

| Policy | Net / night |
|---|---|
| Cut at ≥ 400 g or old age, cut sick rats ≥ 100 g, occupancy cap 70 % (≈ `sim/gym/Rats.cs`) | 219 ± 12 |
| same, without the sick cut | 254 ± 6 |
| peak rule (§12.2), cap 70 % | 374 ± 8 |
| peak rule, cap 85 %, surplus newborns culled first | 390 ± 11 |
| + breeders chosen by growth and immunity, lowest-growth juveniles culled first | **450 ± 6** |
| cut at age 10 (the auto-butcher's age mode), cap 85 % | 396 ± 8 |
| peak rule, base-gene (Rancher start) founders | 391 ± 8 |
| dispenser target 85 % vs 100 % | no measurable difference |
| green serum on the breeders | +0–5 |

By cage size (peak rule, cap 85 %, selection): 40 cells 155, 50 cells 242, 60 cells 289, 70 cells 372,
80 cells 450. The 30-cell runs are unstable (40–80, ± 10): a colony of 8 adults often loses one sex.
Each ingot (+10 cells, 50 credits) is worth about **+50–70 a night**. A second cage plus dispenser
(550, plus 250 of ingots) roughly doubles output.

### 12.4 Settings that achieve it

- Dispenser target 100 %, auto-blend off (blend after loading), cheapest foods first (§4.2), no rat
  meat ever.
- Upgrade the cage to 8 rows before anything else. Repair below ~60 % with scrap. No vacuum robot.
- Breeders: none are strictly needed, because growers breed. Keep one designated pair (marked if the
  auto-butcher is used) as insurance against losing a sex, chosen by growth stat, then immunity ≥ 50
  ("strong immunity" flag), then low hunger rate. Fertility, litter and longevity do not matter for
  meat. An elite growth male from the rancher (500) pays back within days if bred into the colony
  (pups ≈ g 1.12, computed). Litter-elite rats are worthless here.
- Cull order when over ~85 %: newborns first (15), lowest growth first; never the heaviest grower.
- Without an executor (auto-butcher only): age mode, min 10, no cap; keep 0/0; mark one breeding
  pair; spare pregnant **off** (a pregnant 10-day rat kept two more nights cuts at 125, and its litter
  is surplus); cull sick **off**; cull prion on; empty the tray daily. Surplus pups then spill into the
  shop when the cage is full, so a human still culls them.

### 12.5 Caveat: selling it

These are values of meat produced. At 80 cells that is about two `raw_meat` and several small cuts a
night. Food buyers' budgets (`CLIENTS.md`: 35–50, 100–120, 150–200, a private chef 400–500) and FOOD
events decide how much of it sells, and at what markup. A `raw_meat` at 159 is over most food
budgets. Measure sell-through in the gym before scaling past one cage.

## 13. Player-visible boundary

What an executor may read from the game as a player would (tooltip code: `HusbandryHelper.CreateItemTooltip`,
called from `GameItemElement`):

| Per rat | Vis. |
|---|---|
| Sex, stage name, species | VISIBLE |
| Age (days), weight (g, rounded) | VISIBLE |
| Health bar and current / max | VISIBLE |
| Hunger bar and current / max | VISIBLE |
| Fertility: base %, or current vs base when they differ (mature rats only) | VISIBLE |
| Growth rate, litter size, immunity, max health, longevity, hunger rate: current stat, plus the expressed value in brackets when a serum or prion changes it (2 decimals) | VISIBLE |
| Pregnancy progress (days / 3) | VISIBLE |
| Flags: dead, starving, pregnant, miscarried, neutered, diseased, prion, scratch mark, mature | VISIBLE |
| Strong immunity (≥ 50) / immunocompromised (< 25); overcrowding level of its inventory | VISIBLE |
| Serum concentration and −5 µL a day | VISIBLE |
| Both alleles per gene | VISIBLE **only while an owned gene scanner is on** |
| Mark colour, custom name | VISIBLE |
| Body size in cells | DERIVABLE (sprite) |
| Old-age night (14 × longevity), elder night (11 × longevity), peak weight and cut value | DERIVABLE |
| Disease onset %, regen, recovery %, prion chance | DERIVABLE from immunity and max health |
| Partner alleles of a pregnant female; the genetics block | HIDDEN (dev mode only, `CreateGeneticTooltip`) |
| Matings this night, max matings (3), litter variance, RNG | HIDDEN |

| Per machine | Vis. |
|---|---|
| Cage durability current / max, rows left, the slotted dispenser's stored kcal | VISIBLE (`AnimalCage.CreateTooltip`) |
| Dispenser stored / 40 000, kcal per bucket, target %, auto-blend state | VISIBLE (`MachineFeedDispenser.CreateTooltip`) |
| Auto-butcher settings (window) and rats cut last run | VISIBLE |
| Vacuum robot incinerator state / upgrade hint | VISIBLE (`CreateVacuumRobotTooltip`) |
| Food kcal (meat foods say so) | VISIBLE (`FoodItemHelper.CreateFoodItemTooltip`) |

Not visible: the 75 % feeding threshold, the bucket draw rule, the escape chance, the cut table (all
learnable by watching).

## 14. Open questions for the lab (`sim/lab`)

Each would run on the game's own `EndNight` like `ProductionExps`:

1. Newborn first night. Expect pups seen in the morning at age 0, 5 g, hunger 125: they skipped the night of
   their birth.
2. Growth curve. A base-gene rat (`CreateRat(sex, 0, defaultStats: true)`) in a cage, dispenser at 85 %.
   Expect 80, 155, 230, 305, 322, 339, 356, 373, 390, **407**, 390, 373, 356, 356 at ages 1–14.
3. Low hunger gene at 85 %. A rat with hunger rate 45 should be fed on alternate nights only, peaking
   below 400. At 100 % it should peak at 407.
4. Cut table: 399 / 400 / 401 g → 125 / 159 / 174; a diseased 407 g rat → 110.
5. Auto-butcher keepers. Age mode 10, keep 2/1, nothing marked, rats aged 10 entering nightly: the
   previous night's keepers should be cut at age 11. With 2 ♀ + 1 ♂ marked and counting on: every
   unmarked candidate is cut.
6. Disease rate. Immunity-30 colony, clean cage, < 55 % occupancy: about 13 % onset a night. Green
   serum (52.5): none for 4 nights, then back.
7. Serum. Duration exactly 4 nights; a second dose while active extends it without stacking the
   bonus.
8. Prion via the dispenser. Rat meat + other food blended: the first rat to draw gets prion with
   70 % chance at immunity 30.
9. Cage wear = live rat cells / 4 a night, never doubled.
10. Overflow. Pups born into a full cage appear in the back room (then stock / showcase).
11. Overcrowding attacks. At > 70 % about 5 % of rats kill a lighter rat each night. The carcass cuts to 15
    at 5 g.
12. Mating rate. One ♂ and one ♀ (both fertility 50, fed): conception on about 75 % of nights, birth 2–3
    nights later, litter 2–4.
13. Rat trap. Empty: about 1 % a night; with cheese: about 50 % a rat, and 25 % of the rest lose the cheese.
14. Red serum on a g ≈ 0.9 rat at age 1: should peak ≥ 401 g.
15. Rancher prices on normal: listed value × 1.15 (cage 288, elite rat 575).
16. (gym, not lab) Meat sell-through: how many `raw_meat` / small cuts a day the client stream absorbs,
    and at what price.

## Measured (gym, hard, 2026-11-12)

`rats_cut: "peak"` (`sim/gym/Rats.cs`: §12.2's rule, occupancy cap 85 %) on `rancher_sprinter`, 60 paired seeds
8710001–060, 200 days (`sim/lab/scripts/arms-rats.txt`): rat meat sold **3 190 a run against 1 540** (median; 46 a
day against 19), 42 rats cut against 21, feed 1 620 against 1 850. Wins 58/60 both (McNemar p = 1.0), cash buyouts
55 against 53 (p = 0.75), median win day **69 against 76** (paired −7.2 ± 12.7 days, within noise). The lever stays
off by default; `rancher_sprinter` is unchanged.

With a fillerweed feed bay (the production-verdicts audit's suggestion: `engine: "wine+rats"`, `crops: {"fillerweed_seed":
2}`, `bays: 0`, on top of `rats_cut: "peak"`; `sim/lab/scripts/arms-fish2.txt`): 51/60 wins against 58/60 (McNemar
p = 0.065), cash buyouts 19 against 55, rat meat sold 2 790 a run (17 a day) against 3 190. In the gym the bay comes with
the wine engine's purchases (WINEMAKER, bays, power, water), and that capital costs more than the feed saves: the rats
were not short of food under the peak rule (feed spend ~1 600 a run). A feed bay without the wine engine's bundle is not
modelled.

