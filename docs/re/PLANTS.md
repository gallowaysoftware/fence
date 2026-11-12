# Plants, plant genetics and breeding (RE notes)

Build 26525552 / 0.46D. Same conventions as `WATER.md` / `PRODUCTION.md`: **VISIBLE** = rendered by the UI,
**HIDDEN** = internal only, **DERIVABLE** = computable from visible facts. Read from the decompiled game (the
types below are in the global namespace of `Assembly-CSharp`); numbers marked *(measured)* come from
`docs/sim/VERIFIED.md`. The bay itself (panels, power, night logic) is `PRODUCTION.md` §1, the water chemistry is
`WATER.md`, bay modules are `MODULES.md` §3, wine is `WINE.md`. This file covers what a plant is, how it grows,
how its health becomes output, and how the DNA manipulator edits it.

## 0. The short version

- A seed is an item carrying a **plant id** and a set of **trait ranks** (one integer tag per trait). Every stat
  (growth speed, harvest quantity, water use, contaminant sensitivity, seed yield, calories, durability) is the
  plant's base number times the product over its traits of `1 + percent × rank^0.75 / 100`.
- A plant loses health **per progress point**, and a bay applies as many points a night as its speed (8 bare),
  even on the night it finishes. So the penalty per harvest is `nights × speed`, not the work number: 24 points
  for a 3-night first bloomberry cycle, 16 for a 2-night refruit.
- Health only matters at the harvest roll. Each plant maps it differently: bloomberry rolls berry grades,
  nutrifruit and kotton pick sizes, kotton, coffee and dream cap also lose count, fillerweed and hydroreed ignore
  it.
- Bloomberry water is a **chemical-contaminant problem**: tap water made "base" or "HQ" with a filter and
  tablets keeps ≈ 70 bp of chemical (the tablet floor) and ≈ 100 bp of heavy metal (filters do not catch it), which
  is ≈ 4 % health a point, as bad as bought Basewater. Pure water, a `purifying` bloomberry, or water cleaned in a
  hydroreed bay take it to 0.
- Breeding is **cloning plus edits**: own-seed outputs copy the parent's traits exactly (no crossing, no mutation
  anywhere in the plant code). The only way to change a genome is the DNA manipulator: extract a trait from one
  seed onto a disk, apply the disk to another seed, one rank per operation, success falling 5 % per point of
  complexity and heat.
- Refilling a written disk from a seed that has the trait is **deterministic** (no random pick) and adds as many
  uses as the donor's innate rank. The random pick only matters for the first disk of a trait.

## 1. Plant definitions (`PlantList.Register`, `PlantDefinition`)

Base stats, then the effective stats with the innate traits applied. Work is the progress to harvest (bay speed 8
a night unmodded, 12 at performance 100). Nutrients are basis points of mineral per unit of water in the bay's
container (band = level ± range); "mineral use" is µL of mineral consumed per progress point. Contaminant effect
multiplies every contaminant loss. Values are `unitValue`.

| Seed (value) | Work base → effective | Nights at 8 | Yield base → effective | Water ml/night | Nutrients (band) | Mineral use | Contam. effect × sensitivity | Innate traits |
|---|---|---|---|---|---|---|---|---|
| `bloomberry_seed` (40) | 18 → 22 first, refruit 10 → 12 | 3, then every 2 | 8 → 6 | 100 | 200 ± 75 (125–275) | 50 | 1.25 | perennial, edible |
| `nutrifruit_seed` (40; 8 freed, 4 freed-but-infertile) | 23 → 16 | 2 | 5 → 7 | 100 | 150 ± 100 (50–250) | 50 | 1.0 × 0.70 = 0.70 | complex genome II, fast grower II, nutritious, bountiful II, hardy, infertile X |
| `kotton_seed` (20) | 32 → 43 | 6 | 9 → 13 | 176 → 265 | 350 ± 150 (200–500) | 75 | 1.26 × 1.50 = 1.90 | bountiful III, thirsty II, frail II, slow grower |
| `dream_cap_seed` (24, contraband 1) | 28 | 4 | 8 | 100 | 250 ± 75 (175–325) | 30 | 1.01 | dream-cap-mush production, weak, fertile |
| `coffee_bean` (28) | 21 | 3 | 8 | 100 | 250 ± 75 (175–325) | 30 | 1.01 | caffeine production, coffee-mush production, potent |
| `fillerweed_seed` (9) | 11 → 8 | **1** | 4 → 5 | 287 → 450 | 100 ± 100 (0–200) | 75 | 0 (immune) | simple genome II, edible, unappetizing, hardy III, bountiful II, fast grower II, thirsty III, nutrient-poor II |
| `hydroreed_seed` (20) | 18 | 3 | 0 | 20 | 100 ± 100 (0–200) | 25 | 0 (immune) | purifying IV, dense, hardy |

Notes on the effective numbers:

- Work is `round(base / growth multiplier)` in single-precision with round-half-to-even, so the bloomberry's
  18 / 0.8 lands exactly on 22.5 and rounds to **22** (the refruit, 10 / 0.8 = 12.5, to 12). The measured harvest
  nights (3, then every 2) are the same either way; earlier docs said 23.
- Yield is `round-half-away(base × quantity multiplier)`, then `× (1 + ⌊Q/2⌋ / 100)` for the bay's module quality
  Q (same rounding). Bloomberry 8 × 0.75 = 6 *(measured)*.
- The perennial's −50 % seed yield is moot (bloomberries return no seed). Nutrifruit's infertile X drives its seed
  yield multiplier to the floor (0.0001): its three seed outputs almost never materialise until the nutrifruit
  strain is freed (the Eleanor story choice; seeds created after that carry no infertile rank).
- Fillerweed and hydroreed have contaminant effect 0, and fillerweed's nutrient floor is 0 bp, so a fillerweed
  never loses health from water unless minerals exceed 200 bp.
- A seed's base stats are also stamped as tags at creation, but every getter reads the plant definition first; the
  tags only matter for seeds of an unregistered plant.

### Outputs and how health maps to them

| Plant | Harvest | Health effect |
|---|---|---|
| Bloomberry | `count` wine berries, each with its own condition roll | ≤ 25: 55 % moldy / 45 % under-over; 26–55: 25 / 45 / 30 % normal; 56–85: 25 % under-over / 45 % normal / 30 % best; > 85: 40 % normal / 60 % best. Count unaffected |
| Nutrifruit | 3 own seeds (× seed copies, see §3), then `count + farm_module_nutrifruit modules` fruit, sized | ≤ 15 all small; ≤ 30 70 % small; ≤ 50 30 % small; ≤ 70 all normal; ≤ 80 20 % large; ≤ 90 70 % large; ≤ 95 all large; 96–99 half huge (all huge if module quality Q/2 > 50); 100 all huge. Count never reduced. Small 8 / normal 12 / large 18 / huge 32 |
| Kotton | sized fibre (or kotton fabric with the loom module, same count), plus own seeds | Count × 0.5 / 0.65 / 0.8 / 0.9 at health ≤ 15 / 30 / 50 / 70 (rounded half to even), sizes as nutrifruit but no huge (> 90 all large). Small 36 / normal 46 / large 60. Seeds: 1 at ≤ 50, 1 or 2 (even odds) at ≤ 80, 2 above |
| Dream cap | 1 own seed, then `count + 2 × neutralizer modules` caps, sized like kotton (count reduced) | Neutralizer: edible meal caps, contraband stripped; nightmare module alone: all output becomes nightmare caps. Small 17 / large 31 (comestible 13 / 27) |
| Coffee | `count` coffee beans, which are its own seeds | Count × 0.5 / 0.6 / 0.7 / 0.8 at health ≤ 15 / 30 / 50 / 70 (at least 1) |
| Fillerweed | `count − 1` fillerweed (11) + 1 small (7); 1 own seed + 50 % a second (none with the production module, which adds 4 to count) | none |
| Hydroreed | one hydroreed filter per filter module; 1 own seed + 50 % a second | none |

Harvest goes into the 9×6 output grid at night (overflow to a hopper, else the ship inventory). Produce that is not
a seed runs every trait's harvest hook (taste, calories, edible, durability, active ingredients); own-seed outputs
instead inherit the parent's traits (§3) and run only the hooks marked for seed produce (potent/weak and the
production traits — which is why a coffee bean carries caffeine).

## 2. Growth, health and the per-point penalty (`HydroponicHelper`)

Per night, if the bay has a seed, power and at least one night's water (else it is paused, no penalty), it advances
its **processing speed** in progress points (8, raised by performance at ratio 2: P 25 → 9, 50 → 10, 100 → 12,
200 → 16). Then, **once per point of speed**:

1. **Minerals**: mineral bp of the water (`part × 10000 / water`, so bp of water, not of volume) below the band →
   −1 health, above → −0.5; then `mineral use` µL of mineral is consumed.
2. **Contaminants**: for each of heavy metal, chemical, microbe, organic waste, acid and base: x = round(bp ×
   sensitivity multiplier); step loss 0 (x ≤ 10), 0.15 (≤ 15), 0.2 (≤ 20), 0.3 (≤ 25), 0.4 (≤ 30), 0.6 (≤ 40),
   0.8 (≤ 60), 1.6 (≤ 100), 3.2 (≤ 200), 6.4 (> 200) health; each step × contaminant effect, truncated to
   hundredths of a point. Physical contaminant, microplastic, mineral and chlorine never cost health.

Then once per night: the plant drinks its water use (removed in proportion, so ratios do not improve), the
smuggler module strips contraband from the growing seed, the plant's own grow hook runs, then every trait's grow
hook (`purifying`, `polluting`). If progress reached the work, the harvest happens that same night. A perennial
then starts its next cycle immediately (health back to starting health, work = refruit); anything else is
consumed.

Health starts at 100 (no trait changes starting health) and is not clamped below 0. Because the penalty runs per
point of speed, a faster bay does **not** reduce the penalty per point; it changes the points per harvest
(nights × speed): bloomberry 24 / 16 at speed 8, 24 / 12 at speed 12 (P 100, nightly), 20 / 10 at speed 10 with
`fast_grower` I (nightly).

Visibility: the seed tooltip shows effective work, yield and water use (VISIBLE), and with the gene scanner on also
the base values. The plant analyser (owned tool, 200; works on any seed) shows the nutrient band, mineral use,
water use, a contaminant-sensitivity word for effect × sensitivity, and the growing plant's health ("growth score",
VISIBLE). The per-point losses are HIDDEN but DERIVABLE from an AquaScan reading of the bay's container and the
tables above.

### Bloomberry health by water (computed from the rules above; × 1.25 effect, tablets keeping minerals in band)

Contaminant bp are of water. Loss is per point; first / refruit health = 100 − 24 × loss / 100 − 16 × loss.

| Water in the bay | Remaining contaminants (bp) | Loss / point | First / refruit | Refruit band → wine points per berry |
|---|---|---|---|---|
| Tap Rustwater (≈ 91 %) through the filter + 1 tablet/L (≈ 98.3 %, "Basewater"; what the executor makes) | chemical ≈ 71, heavy ≈ 99 | 4.0 % | 4 / 36 | 26–55 → 0.05 |
| same + `hardy` I | 50 / 69 | 3.0 % | 28 / 52 | 26–55 → 0.05 |
| same + `hardy` II | 35 / 49 | 1.75 % | 58 / 72 | 56–85 → 1.05 |
| that water after the purifier (99.01 % cap; heavy metal goes before chemical) | chemical ≈ 66, heavy ≈ 34 | 2.75 % | 34 / 56 | 56–85 (barely) → 1.05 |
| purifier HQ + `hardy` I | 46 / 24 | 1.37 % | 67 / 78 | 56–85 → 1.05 |
| Bought Basewater ≈ 98 % (measured) | 61 / 41 org / 31 / 20 mic | 4.0 % | 16 / 44 *(measured)* | 26–55 → 0.05 |
| Bought HQ ≈ 99 % (measured) | 30 / 20 / 15 / 10 | 0.94 % | 56 / 70 *(measured)* | 56–85 → 1.05 |
| Pure (≥ 99.90 %: every contaminant ≤ 10 bp) | — | 0 | 100 / 100 | > 85 → 1.6 |
| any of the above, `purifying` I bloomberry, converged | chemical 16, heavy 25 (floors) | 0.62 % | 85 / 90 | > 85 → 1.6 (first harvest 56–85) |
| `purifying` I + `hardy` I | 11 / 18 | 0.43 % | 89 / 93 | > 85 → 1.6 |
| `purifying` II | 8 / 12 | 0.18 % | 95 / 97 | > 85 → 1.6 |
| water cleaned first in a hydroreed bay (§4) | ≤ 4 / ≤ 6 | 0 | 100 / 100 | > 85 → 1.6 |

Without tablets every row also loses 1 %/point to missing minerals (filters and the purifier strip minerals; the
`purifying` trait and hydroreed spare them). Wine points per berry: moldy −1, under/over 0, normal +1, best +2,
weighted by the band's roll odds.

## 3. Traits (`PlantTraitList`, `PlantTrait`, `PlantGeneticsHelper`)

Every trait is unlimited in rank. A rank r scales a per-rank-one percent by `r^0.75` (r 1/2/3/4/5 → ×1, 1.68, 2.28,
2.83, 3.34), floored at −99.99 %. Stat multipliers of all traits multiply, floored at 0.0001. Traits in the same
**exclusive group** cannot coexist (applying one is refused while the other is present).

| Trait | Group | Complexity | Effect at rank I | Notes |
|---|---|---|---|---|
| edible | edible | 2 | non-food produce becomes FOOD with 100 calories | acts on produce |
| nutritious / nutrient-poor | nutrition | 3 / −2 | calories ±25 % | acts on produce |
| fast grower / slow grower | growth | 4 / −3 | growth speed ±25 % (work ÷ multiplier) | |
| bountiful / sparse | yield | 4 / −3 | harvest quantity ±20 % | |
| delicious / unappetizing | taste | 3 / −2 | edible produce price ±15 % (a value modifier on a taste feature) | acts on produce |
| drought resistant / thirsty | water | 2 / −1 | water use ∓25 % | |
| hardy / frail | resilience | 3 / −2 | contaminant sensitivity ∓30 % (hardy IV ×0.15; **hardy V hits the floor: immune**) | |
| fertile / infertile | seeds | 2 / −1 | seed yield ±25 % | only on crops that return seeds |
| simple / complex genome | genome | 0 | complexity −4 / +4 per rank | **locked**: cannot be extracted or applied |
| perennial | — | 5 | survives harvest and refruits; harvest −25 %, growth −20 %, seed yield −50 % | incompatible with infertile |
| purifying / polluting | water quality | 3 / −2 | each growth night removes `min(95 %, 10 % × r^0.75)` of every purifier-removable contaminant except mineral (at least 1 ml each), down to a floor of each liquid's machine floor ÷ r (chemical 16, heavy metal 25, microplastic 18, organic 10, microbe 9, physical 7 bp at rank I); removed volume returns as water / turns 0.1 % × r^0.75 of the water into organic 50 %, microbe 35 %, physical 15 % | rank IV (hydroreed): 28 %/night, floors 4/6/4/2/2/1 bp |
| dense | fibre | 3 | harvested durable items +25 % durability | only crops with durable produce (hydroreed filters) |
| potent / weak | potency | 3 / −2 | active ingredients ±25 % | only seeds with an active liquid (coffee, dream cap, or any seed given a production trait first); acts on seed produce too |
| caffeine / coffee-mush / dream-cap-mush production | — | 3 / 2 / 4 | infuses 2 / 10 / 2 ml × r^0.75 of that liquid into each produce item (dream-cap mush adds contraband 2 unless the neutralizer module is in; the nightmare module converts it) | acts on seed produce too, **and on any crop's produce**: a fillerweed given caffeine harvests items whose only liquid is caffeine (lab, `breed.caffeineweed`; docs/research/BREEDING.md) |

A trait that acts on produce has no effect (and the manipulator refuses it) on a plant whose harvest is its own seed
(coffee), unless it is marked for seed produce. The gene scanner prints "no effect" next to such traits.

**Inheritance.** When a harvest outputs the plant's own seed, each output is a clone: it copies every trait rank
of the parent and starts with zero heat. The number of copies per seed output is the seed-yield multiplier's whole
part plus a roll on its fraction (fertile I: 1 + 25 %; infertile X: ~0). Nothing else changes a genome: no
crossing, no pollination, no mutation in the plant code (bought and spawned seeds carry only the innate ranks).

**Bred donors** (lab, `breed.extract`). The uses a refill adds are the donor's *current* rank of a trait innate to its
species, so a line's own clones become rich donors (a coffee bean bred to caffeine III refills +3, potent IV +4); a
trait the species lacks gives 1 however high (fillerweed with caffeine III: +1). The op runs at the donor's own
complexity: 45–65 % on those bred beans. Complexity, unlike heat, is permanent on clones; negative traits lower it
for good, which is how a clone line banks odds (BREEDING.md §1).

## 4. Water-quality plants

- **Hydroreed** (purifying IV, immune to contaminants, 20 ml a night, 3-night cycle, returns 1–2 seeds, 20 a
  seed) is a water-treatment machine in disguise: in its bay's container it removes 28 % of every non-mineral
  contaminant a night down to ≤ 6 bp each. Filtered-and-tableted tap water (chemical ≈ 71, heavy ≈ 99 bp) falls
  below 10 bp of each in about six nights, keeping its minerals; the container ends at the floors (heavy metal 6 + chemical 4 + microbe 2 bp): 9988 bp, HQ. **Pure needs the
  filterables gone and the microbes too:** two filter passes first, then UV after the reed: 9990 bp (lab,
  `FormulaExps.ReedPure`; one filter pass leaves 0.6 bp and ends at 9989). The removal is a percentage of each
  contaminant with no volume budget and the bay's slot takes any owned liquid container, so a 20–50 L tank cleans
  in the same nights as a litre (`PlantWaterQuality.Purify`, `MachineHydroponic`). It runs only while a seed grows, so it must be replanted the morning after
  each harvest (it is not perennial).
- A **purifying bloomberry** cleans its own bay's water the same way at rank I's floors (§2 table): 10 % a night,
  so a fresh 6 L of tap-treated water converges in about two weeks, and top-ups of a few hundred ml barely move it.

## 5. The DNA manipulator (`MachineDnaManipulator`)

Bought: Wild's `PLANT_DNA` (1 250 × the upgrade multiplier, needs `RETIRED_FARMER`) drops a manipulator, four blank
disks and a gene scanner on the back counter. After that Dr. Jackson stocks a manipulator on 50 % of visits, one
blank disk always plus one each on 50 %, 50 %, Jackson-1 and Jackson-2 (1–5), each DNA module on 25 % of visits,
and a gene scanner while none is owned (`PlaceInventorInventory`). Values: manipulator 1 000, disk 140, gene
scanner 500, stabilizer / positive filter / negative filter 500, precision 1 000 (before markup).

Machine: battery, module bay (universal and DNA modules), trait-disk slot (input), seed slot, blank-disk slot
(output), note, 2×2 eject grid. 12 power a night, 10 progress a night, performance and quality at ratio 2. Each
night in the ready state it plans one operation from what is loaded; if not blocked and the battery pays, it starts
and works the first night. Seeds already growing, seeds of no plant, and disks incompatible with the loaded seed
are refused by the slots.

| Operation | Loaded | Result on success | On failure |
|---|---|---|---|
| **Apply** | a written disk in the input slot | the seed gains the disk's rank of the trait (always 1 for extracted disks); one use spent, the disk destroyed at 0 | nothing applied, no use spent; seed keeps the heat |
| **Extract, random** | a blank disk in the output slot | a random extractable (not locked) trait of the seed, filtered by the modules below, is written with uses = the seed's current rank of it if the trait is innate to that species, else 1 | disk stays blank |
| **Extract, refill** | a written disk in the output slot, and a seed that has that trait | the disk gains uses (same rule) — **no random pick** | disk unchanged |

Without the stabilizer, an extraction **destroys the seed** whether or not it succeeds. With it, the seed survives,
loses one rank of the extracted trait on success, and gains heat.

**Odds and time.** Effective complexity = complexity + heat, where complexity sums over all traits: (rank − innate
rank) × the trait's complexity when added (so adding a negative trait *lowers* it), |complexity| × ranks removed
when below innate, plus the genome offsets (simple genome −4 a rank, complex +4). Heat grows by 2 + |complexity
of the trait| after every apply (success or not) and every stabilized extraction; it clears only on a cloned child
seed. Then:

- progress needed = 20 + 4 × max(0, effective complexity); nights = ⌈progress / speed⌉
- success = clamp(95 − 5 × max(0, effective complexity) + ⌊Q/2⌋, 5, 100), Q = the machine's module quality

| Module | P / E / Q | Effect | Machine with it |
|---|---|---|---|
| Stabilizer | −50 / −25 / 0 | extraction keeps the seed (−1 rank of the trait) | speed 7, power 15 |
| Positive filter | 0 / −25 / 0 | random extraction picks only non-negative traits | power 15 |
| Negative filter | 0 / −25 / 0 | random extraction picks only negative traits (both filters: blocked) | power 15 |
| Precision | −75 / −75 / 0 | tuned by using it on a written disk; random extraction then takes only that trait and is blocked when the seed lacks it | speed 6, power 21 |

Everything here is VISIBLE: the manipulator tooltip shows the planned operation, success chance and work; the disk
tooltip shows trait, rank, uses, complexity, exclusivity and the crops it does nothing for; the gene scanner (toggle
on) adds every seed's traits, ranks, innate/locked marks, complexity and heat to its tooltip.

### Donors for the traits that matter (random extraction, no filter)

| Donor (value) | Extractable traits | P(trait) | Uses on the disk | Success | Notes |
|---|---|---|---|---|---|
| hydroreed (20) | purifying, dense, hardy | 1/3 each | purifying 4, hardy 1 | 95 % | best first-disk source for hardy or purifying |
| fillerweed (9) | edible, unappetizing, hardy, bountiful, fast grower, thirsty, nutrient-poor | 1/7 each (1/4 for each of the four positives with the positive filter) | hardy 3, bountiful 2, fast grower 2 | 95 % (genome −8) | best refill source for hardy (3 uses for 9) |
| nutrifruit (40) | fast grower, nutritious, bountiful, hardy, infertile | 1/5 | fast 2, bountiful 2, hardy 1 | 55 % (complex genome +8) | poor donor |

## 6. Best yield and quality: what to breed and how

**What a bloomberry can carry.** Bloomberries return no seeds, so every edit is made on a bought seed with its
heat never clearing. The first operation on a fresh seed is always 95 %; the second is 95 − 5 × (c + 2 + c) for a
first trait of complexity c: 55 % after hardy or purifying, 45 % after bountiful or fast grower; the third is ≤ 25 %.
So a practical bloomberry has **one trait, sometimes two**.

| Bloomberry setup (speed 8) | Berries / harvest | Refruit health | Wine points per berry | Points per bay-week (3.5 harvests) |
|---|---|---|---|---|
| plain, tap-treated or bought base water (the executor today) | 6 | 36–44 | 0.05 | ≈ 1 |
| `hardy` I, same water | 6 | 52 | 0.05 | ≈ 1 |
| plain, purifier-HQ water | 6 | 56 | 1.05 | 22 |
| `purifying` I, any ≥ base water (after ~2 weeks) | 6 | 90 | 1.6 | 34 |
| plain, Pure + tablets (or hydroreed-cleaned water) | 6 | 100 | 1.6 | 34 |
| `bountiful` I, Pure + tablets | **7** | 100 | 1.6 | 39 |
| `purifying` I + `bountiful` I (2nd op 55 %) | 7 | 90 | 1.6 | 39 |
| `bountiful` I + bay quality ≥ 16 (two quality modules) or bay P ≥ 50 + `fast_grower` I | 8, or 7 nightly | 100 | 1.6 | 45–49 |

Rules that follow:

1. **Water first.** On the water the executor makes today a bloomberry rolls in the 26–55 band (70 % moldy or
   under/over-ripe) and `hardy` I does not change the band. The fix is water with chemical and heavy metal ≤ 10 bp:
   Pure, a hydroreed bay, or a `purifying` bloomberry. Hardy only pays from rank II on tap water (55 % second op)
   or at rank I on purifier-HQ water.
2. **`purifying` beats `hardy`** as the one trait for a bloomberry on tap water: same complexity (3), but rank I
   brings the bay's own water to the floors (0.62 %/point, refruit 90) instead of 52. Its disk comes from a
   hydroreed with 4 uses (vs 1 for hardy), so one lucky extraction breeds four seeds.
3. **With clean water, `bountiful` I is the trait**: +1 berry a harvest (+17 %), all at the clean-water grade. Rank II
   (8 berries) needs a second op at 45 %; two quality modules on the bay give the same 8.
4. **Refill, do not re-extract.** Once a disk of the trait exists, keep it alive: before its last use, load it as the
   output disk with a donor that has the trait (fillerweed for hardy: +3 uses, 95 %, a 9-credit seed; hydroreed for
   purifying: +4 uses, 95 %). The random pick, the precision module and new blank disks are only for the first disk.
5. **Order on one seed**: the trait you need most first (95 %); after a failure the seed is still plantable.

**Coffee and kotton breed for real**, because their outputs are clones with cleared heat: edit one seed, grow it,
and every descendant carries the edit and starts cold for the next one.

- Coffee: `bountiful` I 8 → 10 beans (+25 %), II 11, III 12, IV 13; `fast_grower` II (work 21 → 15) makes it a 2-night
  crop (+50 % throughput). Ops on a cold child: first edit 95 %, the second trait 75 % (complexity 4), the third
  ~55 %. Coffee keeps a full harvest only above health 70 (pure or near-pure water with 7–13 tablets per 6 L).
- Kotton: innate frail II makes it the most water-sensitive plant (×1.9), and its 6-night cycle is 48 penalty
  points. It needs Pure or hydroreed-cleaned water **and** 8–20 nutrient tablets per 6 L (band 200–500 bp; one
  tablet a litre is 150 bp, below the band: −1 a point, health ≈ 52, which is exactly the measured 12 normal fibre,
  ≈ 92 a night, in `docs/research/SECOND-ENGINE.md`). In band, health 100 gives 13 large fibre (60) a harvest,
  ≈ 130 a night, against ≈ 36 a night on tap water (half the count, all small). Breeding: `hardy` on top of
  frail is refused (same group); remove a frail rank first (stabilizer + precision tuned to frail), or simply give it
  clean water. Removing `slow_grower` (stabilizer) brings work 43 → 32 (4 nights, +50 %).

**Measured** (lab, `breed.bloom`, 6 bays × 21 nights each; wine points a berry on nights 10–21). On
`FillWithBaseWater` + 6 tablets the plain plant already reaches 1.16 (better than the computed Basewater row above:
that water is cleaner than the bought bottles the table used), hardy I 1.03, **purifying I 1.58**, purifying I +
bountiful I 1.62 at 7 berries, bountiful I 1.04 at 7. On filtered Rustwater + a nutrient tablet a litre without
water tablets (chemical 219 bp): plain −0.52, hardy I −0.49, bountiful I −0.54, **purifying I +0.66** and purifying
+ bountiful +0.97, still rising as the bay converges. Rules 1–3 hold; hardy I is worth nothing on either water.

## 7. `sim/gym/Dna.cs` against the above

**Superseded by `breeding` (sim/gym/Breeding.cs, docs/research/BREEDING.md)**, which implements gaps 1–5 below as gene
lines; `wine_dna` is kept as it was, except that the DNA kit is now moved off the back counter, where it sat from
the night PLANT_DNA was bought and the next buyers took the manipulator (in the gym it was sold within days in 4 of
6 hard runs) — part of why `wine_dna` measured null.

What it does: buys PLANT_DNA, the manipulator and blank disks; random-extracts from a fillerweed / nutrifruit /
hydroreed (most hardy ranks first, i.e. fillerweed) onto a blank disk until a disk reads Hardy; applies it to
fresh bloomberry seeds (`wine_dna_ranks` per seed, default 1); plants bred seeds in an empty or extra bay or
hatchets a plain plant. Every read (disk label, trait ranks via the gene scanner) is player-visible.

Gaps, ranked by expected value (estimates from §2 and §6, per bred bay):

1. **Wrong trait for the water.** Hardy I on the executor's ≈ 98.3 % tap water leaves refruit health at 52, in the
   same 26–55 band as an unbred plant: ≈ 0 wine points gained per berry. This matches the measured null result in
   `WINE.md` §7. Proposed: a `wine_dna_trait` lever (`purifying` | `hardy` | `bountiful`), default `purifying`
   (refruit 36 → 90 on tap water: ≈ +1.55 points a berry, ≈ +33 points per bay-week, i.e. most bottles one or two
   tiers up), `bountiful` when the bay's water is Pure. Change `Trait` from a constant to the lever, add
   `"purifying"` donors (hydroreed only), and rank the donor by P(trait) × uses (hydroreed first for purifying).
2. **Refill instead of burning blank disks.** Today the last use destroys the Hardy disk and the next one is again a
   1-in-7 random extraction that destroys a donor and, 6 times in 7, writes a useless trait over a 140-credit disk
   (expected ≈ 6 disks ≈ 840 credits, or the 1 000 precision module, per new Hardy disk). Proposed: in `TendDna`,
   when the trait disk shows 1 use left (`DNA_DISK_USES_INT`, on its tooltip) and a donor with the trait is owned,
   put the written disk in the output slot and the donor in the seed slot (a refill: +3 uses from fillerweed for
   hardy, +4 from hydroreed for purifying), before applying.
3. **Second trait.** After the first op, a second op at 55 % (purifying first, then bountiful): +1 berry a harvest
   (+17 %) on success; a failed op leaves the seed usable. Proposed: `wine_dna_traits: ["purifying", "bountiful"]`
   replacing `wine_dna_ranks`. `wine_dna_ranks: 2` of hardy is worth nothing on tap water (refruit 52 → 72 only at
   rank II, at 55 %), and on HQ water rank I already reaches the 56–85 band.
4. **Use the by-products.** 2/7 of fillerweed extractions write bountiful or fast grower with 2 uses; today they
   become plain stock. Proposed: keep them and apply them as the second trait.
5. **First-disk donor.** For hardy, hydroreed gives the trait 1 time in 3 (vs 1 in 7 for fillerweed), at 95 %,
   so it burns ~2 disks instead of ~6 for the first Hardy disk; then refill from fillerweed. Proposed: buy one
   hydroreed seed as the first donor.

Outside `Dna.cs` but larger than all of the above: a **hydroreed bay that pre-cleans the bloomberry bays' water**
(§4) gives every bay the Pure-water row with no DNA spend at all (300 for a bay, 20 for a seed, 8 power a night),
and kotton/coffee crop bays (`sim/gym/Crops.cs`) drink raw tap water today, which halves coffee and kotton counts
and makes all kotton small (≈ 36 vs ≈ 130 a night for kotton). Kotton also needs ~2.3 nutrient tablets a litre, not
the one a litre `TendBay` adds.
