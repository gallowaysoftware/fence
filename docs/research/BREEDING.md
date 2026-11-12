# Gene lines: what the DNA manipulator is worth (2026-11-12)

**Question.** The owner: "I bet you also missed plant gene editing didn't you." PLANTS.md §3–7 documented the
manipulator, but the executor used one sliver of it (`wine_dna`: one hardy rank on bloomberry seeds, measured null).
Read as an optimising player: what is consumed, what is reusable, what compounds, which thresholds make something
free — and which gene lines does that make worth breeding?

**Verdict.** **The mechanics, yes; on the hard routes, not yet.** The lab confirms every rule the lines rest on,
and one the docs missed: a production trait put on fillerweed makes a 1-night, contaminant-immune, tap-water crop
whose produce is pure caffeine, pressed into pills the same morning with no alcohol — the caffeine lab's binding
link. On the hard Board route the gym's caffeine-weed arm still costs time (+11.8 ± 9.9 days to the buyout, 60/60
both): the line's capex (PLANT_DNA, cells, crop bays, the tap) competes with the buyout, the lab's blender and
finisher are missing on most mornings, and the median line reaches caffeine II–III by the end.

Rules: docs/re/PLANTS.md (corrected here where the lab disagreed). Lab: `sim/lab/BreedExps.cs`
(`fence-lab breed.odds | breed.extract | breed.caffeineweed | breed.coffee | breed.bloom`). Executor:
`sim/gym/Breeding.cs` (with `Dna.cs`), directive `breeding: {...}` (protocol/schema/directives.schema.json,
docs/PROTOCOL.md). Arms: `sim/lab/scripts/directives/breeding/`.

## 1. What the source makes free (verified on the game's machines)

| Claim | Lab |
|---|---|
| Odds = clamp(95 − 5·max(0, complexity + heat) + ⌊Q/2⌋, 5, 100), work 20 + 4·max(0, ·) at 10 a night (2 nights at ≤ 0) | tooltip exact at heat 0/4/8/14 on coffee and 0/10 on fillerweed; measured 94/83/49/26 % and 95/89 % (n = 100 each, all within the 99 % CI); nights 2/4/6/8 and 2/3 |
| A failed apply spends no disk use; heat += 2 + \|trait complexity\| either way | uses spent = successes exactly; heat 0 → 6 after a complexity-4 apply |
| Fillerweed's simple genome II (complexity −8) banks 8 points: its first ops run at 95 % | 95 % at heat 0 and 85 % (plan) at heat 10 |
| A refill adds the donor's **current** rank of a trait innate to its species, else 1, with no random pick | coffee bred to caffeine III → +3, to potent IV → +4; fillerweed carrying caffeine III → +1; fillerweed hardy III → +3 (n = 40 each). A bred donor is harder to extract from (its own complexity): 45 % at caffeine III |
| Random extraction is uniform over the seed's unlocked traits; without the stabilizer the seed is destroyed | coffee: caffeine 36 / mush 31.5 / potent 32.5 % (n = 378); 0/400 beans survived |
| Stabilizer: donor kept, one rank lighter, heat + 2 + \|c\| | 40/40 kept, rank 3 → 2 on success, heat 5 |
| Clones (own-seed outputs) copy every rank with heat 0 | fillerweed children carry caffeine/potent ranks, heat 0, every harvest |
| **Production traits act on any crop's produce** (they are marked for seed produce, not barred from non-seed crops); potent needs an active liquid first | caffeine disk accepted on a plain fillerweed; potent refused on a plain fillerweed, accepted after caffeine |
| A caffeine fillerweed's produce holds **only caffeine**: 2 ml × r^0.75 × (1 + 0.25·p^0.75) an item, nothing else, not contraband, still food | 5 items a harvest every night on raw Rustwater; µL per item equal to the formula at (1,0), (2,0), (3,1), (4,2), (6,2) — e.g. 8 035 µL at caffeine IV + potent II |
| That produce blends to a **dry, pure caffeine residue**: pressed the same morning, no alcohol, acid, lipobond, filter or evaporator | blender 0 → finisher 0: 88 % near pharma; finisher 1: 94 % pharma; 4 + 4 modules: 100 % lab pure (24 each). Pills = ml × (1 − blender loss) / 5 |
| The blender takes 8 fillerweeds a press (its 4×4 grid) | 8 loaded of 20 offered; press again for the rest |
| Coffee: bountiful II → 11 beans, IV → 13; fast grower II → a 2-night crop; caffeine III +3.2 ml, coffee mush III +16 ml mush, potent III ×1.26 every active | as stated (3 bays × 9–12 nights each) |
| Bloomberry on Basewater + tablets: wine points a berry (nights 10–21) plain 1.16, hardy I 1.03, **purifying I 1.58**, purifying I + bountiful I 1.62 × 7 berries, bountiful I 1.04 × 7 | 6 bays × 21 nights each |
| Bloomberry on filtered Rustwater + tablets (chemical 219 bp, no chlorine): plain −0.52, hardy I −0.49, **purifying I +0.66** (still converging), purifying + bountiful +0.97 × 7, bountiful −0.54 | same |

What follows from the formulas:

1. **Disk uses are not the scarce thing.** A refill from a donor whose species has the trait innately gives the
   donor's whole rank, and a bred line's own surplus clones carry the line's ranks. The scarce things are
   manipulator nights (≥ 2 an operation) and the success odds, which fall 5 points per point of the genome's
   permanent complexity.
2. **Complexity can be banked.** Negative traits lower the genome's complexity for good (heat clears on every clone,
   complexity does not). On fillerweed, nutrient-poor (−2) and unappetizing (−2) act only on the produce's calories
   and price — worthless once the produce is blended — so a bank op at 95 % buys back 2/3 of a caffeine rank's
   complexity. The executor applies one when the next edit's odds would be under `min_success`.
3. **The caffeine trait moves caffeine off the alcohol-bound chain.** A coffee bean is 8 ml free caffeine and 44.8 ml
   mush that needs 1× alcohol, two mornings, acid, lipobond, two filters and drying (CHEMISTRY.md §6.2;
   CHEMISTRY-ENGINE.md: alcohol was the binding link). A caffeine fillerweed carries none of that: blend, press.

## 2. Gene lines and their paper value

Values a bay-night at hard's estimate (pills: near pharma 14, pharma 18, lab pure 24), before power (≈ 13 a bay-night).

**Caffeine weed** (`caffeine_weed`: fillerweed, a 1-night crop immune to contaminants, on raw tap water with no
tablets; 5 items a night; caffeine ranks first, potent once caffeine is in, then by gain per complexity point):

| Genome (caffeine, potent) | Complexity | Caffeine ml / bay-night | Pills, blender 0 | Value, pharma (finisher 1) | Value, lab pure (4 + 4) | With the fillerweed module (9 items, no seeds), lab pure |
|---|---|---|---|---|---|---|
| (1, 0) | −5 | 10.0 | 1.3 | 23 | 48 | 86 |
| (2, 0) | −2 | 16.8 | 2.2 | 39 | 81 | 145 |
| (3, 0) | +1 | 22.8 | 3.0 | 53 | 109 | 197 |
| (3, 1) | +4 | 28.5 | 3.7 | 67 | 137 | 246 |
| (5, 1) | +10 | 41.8 | 5.4 | 98 | 201 | 361 |
| (6, 2) | +16 | 54.5 | 7.1 | 127 | 261 | 470 |
| (7, 3) | +22 | 67.6 | 8.8 | 158 | 324 | 584 |

Against: the same bay of plain fillerweed sells raw at ~51 a night (SECOND-ENGINE.md), so the weed beats the raw crop
from (3, 1) at pharma, from (2, 0) at lab pure. A coffee bay on pure water makes ~2.3 net beans a night, ≈ 97 ml of
caffeine after extraction (≈ 19 pills at blender 4) — but each bean needs ~45 ml of alcohol and a lab batch slot; the
weed needs neither, and runs on raw tap water.

Cost of the line: PLANT_DNA (1 250 × the upgrade multiplier, needs RETIRED_FARMER), the lab's blender and finisher
(RETIRED_CHEMIST), the crop bays and the tap. Operations to (6, 2) with banking: ~8 applies, ~6 bank ops, ~6
refills/extractions (caffeine disks refill +1 from a plain coffee bean) ≈ 20 ops ≈ 45 manipulator-nights with one
manipulator.

**Bloom** (`bloom`: bloomberry seeds edited fresh, purifying then bountiful at 55 %): on Basewater +0.42 wine
points a berry (1.16 → 1.58), +17 % berries with bountiful; on filtered Rustwater the difference between a cellar
that molds (−0.5) and one that ferments (+0.7 → +1.0 as the bay converges). One purifying disk (hydroreed: 4 uses
a random hit, 1/3) breeds four seeds.

**Coffee** (`coffee`: fast grower II + bountiful II, then more): 11 beans every 2 nights against 8 every 3 — 5 net
beans a night against 2.3 (×2.1), on pure water; ×2.25 on tap water (count halved either way). Worth it where beans
bind (raw sale, or a lab that is no longer alcohol-bound).

**Not built, worth knowing:** hardy V makes any crop immune to contaminants (5 ranks, +15 complexity: coffee and
kotton on tap water at full count); drought resistant (water −25 % × r^0.75) needs thirsty removed first on fillerweed
(stabilizer + precision, three ops); delicious on food crops is +15 % × r^0.75 of a cheap item; dense on hydroreed
filters +25 % durability a rank. A coffee line bred up in caffeine is a rich caffeine donor (rank r = r uses a
refill).

## 3. The executor

`breeding: {lines: [...], manipulators, reserve, press, press_min_ml, hold}` — off when absent, needs the wine
engine (its bays and cells); clone lines take their bays from `crops` (e.g. `crops: {"fillerweed_seed": 2}`), and
`press` needs `lab`. A line is a preset name or `{crop, edits, bank, min_success, give_up}`.

Each morning, as a player at the manipulator, before the bays replant: the night's clones come out of the bays'
output; the finished op is read off the seed and disk (gene scanner, disk label); an op that did not start (no
charge) stays loaded; then for each idle manipulator, in line order: the cold clone of the line's best genome (or, for
bloomberries, the seed being edited, then a fresh one) and the queue's next trait; a bank trait first when the
tooltip's odds are under `min_success` and a bank disk is at hand or can be extracted; a refill before a disk's last
use while the trait is still wanted (from the donor giving the most expected uses, never one of the seeds the line
keeps); a random extraction from the likeliest donor when no disk of the trait exists. It buys PLANT_DNA when the
cash clears `reserve`, donor seeds of the species most likely to give a missing trait, blank disks and a second
manipulator at Dr. Jackson's, and keeps the line's best (bays + 2) seeds. The DNA kit is moved off the back counter
(it was being sold to the next buyer — which also hit `wine_dna`; see §5). The press runs before the lab each
morning: any produce whose only liquid is caffeine goes through the blender (8 a press) into an empty vessel and the
finisher, the lab's waiting vessel set aside and put back.

Player-visible only: trait ranks, heat and complexity (gene scanner), disk trait and uses (label), odds and work
(the manipulator's tooltip), innate marks (scanner), produce composition (spectrometer; a player also knows it from
the trait).

## 4. For the chemistry engine (Lab.cs): the interface

- **What arrives**: owned loose items, `CHEM_ITEM_TAG` + `BLENDABLE_TAG`, not `CHEMICAL_PRODUCT_TAG`, whose only
  non-zero liquid is `caffeine` (today: `fillerweed` / small fillerweed from a caffeine line; identifier is not the
  test). `Production.IsDryCaffeine(item)` is the predicate (make it public if Lab.cs wants it).
- **What to do with it**: blend into any clean vessel and press `caffeine_pill` (5 ml a unit) — the residue is dry
  and 100 % caffeine, so the unit's purity is exactly the finisher's cap (88 / 94 / 97 / 99 / 100 % at 0–4 modules).
  No alcohol, reagents, filters, plate or evaporator. Blender loss applies (35 % at 0 modules).
- **Who does it now**: `Production.PressWeed` (Breeding.cs), before `Lab.Morning`, only when the lab's blender and
  finisher exist and the finisher's slot is empty; a lab vessel in the blender is set aside and put back. If Lab.cs
  takes this over (one queue, its module plan counting weed caffeine as input), set `breeding.press: false`.
- **Holding**: `breeding.hold` (120) caffeine items are reserved for the press; beyond that they sell raw (11 each).
- **Module plan**: weed caffeine changes the blender's value — it is the only loss on this chain — so per ml of weed
  caffeine the 1st blender module is worth +23 % pills and the finisher's grade steps as before.

## 5. Measured

Hard, paired seeds 2910001–060, 1000-day horizon, `-start apprentice -perks vending+short_sleeper -human player`,
gym frozen at 06f4d2c (before the water-cellar merge; breeding off is byte-identical to main after it, 2 seeds of `caffeine_lab`). Wilson 95 % on wins, paired mean ± 1.96 SE, exact McNemar.

| Arm | Wins (95 % CI) | Median last day | Endings | Paired Δ last day |
|---|---|---|---|---|
| `caffeine_lab` + `crops: {fillerweed_seed: 2}` (reference) | 60/60 (94–100 %) | 210 | buyout 14, mortgage 46 | — |
| + `breeding: {lines: ["caffeine_weed"]}` | 60/60 (94–100 %) | 210 | buyout 2, mortgage 58 | **+11.8 ± 9.9** (McNemar 0:0) |
| `sprinter` (reference) | 60/60 (94–100 %) | 91 | buyout 60 | — |
| + `breeding: {lines: ["bloom"]}` | 60/60 (94–100 %) | 112 | buyout 58, mortgage 2 | **+18.2 ± 5.5** (McNemar 0:0) |
| Board-free `caffeine_lab` + crops (reference) | 0/60 (0–6 %) | — / 119 | rent 25, arrest 34, execution 1 | — |
| Board-free + caffeine weed | 0/60 (0–6 %) | — / 114 | rent 19, arrest 40, execution 1 | −2.0 ± 6.1 |
| Board-free `sprinter` (reference) | 0/60 (0–6 %) | — / 189 | rent 43, repo 15, arrest 2 | — |
| Board-free + bloom | 0/60 (0–6 %) | — / 168 | rent 58, repo 2 | −9.5 ± 8.7 (dies sooner) |

Board-free, PLANT_DNA is bought in only 5 of 60 caffeine-weed runs (0.7 ops a run, no press); the bloom line buys
it in 57 and dies ~10 days sooner, still blocked on purifying donors and blank disks (2 108 and 1 623 stall-days).

The bloom line (60 runs): manipulator in all 60, 6.7 ops a run, a bred bloomberry planted in only 13. Stall-days: no
donor for purifying 1 631 (only the hydroreed carries it, and sellers rarely bring one), no blank disk 747, no plain
seed to edit 386. PLANT_DNA's ~1 900 on hard, three weeks before the buyout, is the whole delay.

Caffeine weed, what the line did (60 runs): manipulator owned in 58; 11.9 ops a run; elite genome at the end caffeine 0/I/II/III/IV/V
in 12/11/20/11/5/1 runs; bred seed planted in 25; the press ran in 6 runs (4 pills a run on average). Stall-days:
the lab's blender or finisher missing 2 606, no charged cell for the manipulator 1 996 (op loaded, not started 1 883),
no cold elite clone loose 711, no blank disk 709, no donor 694. Crop bays 0.78 a run (of 2 asked), 83 harvests.

**Paper vs gym.** Paper: caffeine III + potent I is ~67 a bay-night at pharma (above raw fillerweed's 51) from ~10
ops. Gym: ~0 — the line exists, but the press, the second crop bay and the cells it assumes do not. Every wall is
capital and kit around the line, not the genetics.

Per-seed summaries were kept in the session scratchpad (`gene-lines/arms/`, with `stats.py` / `breedagg.py`).

## 6. What is left

- Coffee and farmer+bloom arms (directives in `sim/lab/scripts/directives/breeding/`).
- Bloom: a hydroreed bought up front (the only purifying donor) and blank disks before PLANT_DNA's kit runs out.
- Make the line affordable before measuring again: buy the lab's blender + finisher first when the weed line is on,
  a cell budget for the manipulator, and the crop bays/tap ahead of PLANT_DNA; or a route that has them (Cook,
  farmer start).
- `press` belongs in Lab.cs (one queue, module plan counting weed caffeine); `IsDryCaffeine` is the predicate.
- Live/mod: the manipulator's slots are item moves (Chore.Accept), no buttons; the press uses the blender's start
  and the finisher's output (LiveHost.MachineButton). Untested live. The members breeding reads are in
  compat/hooks.json and the runtime resolver (866/866 resolve on 26525552).
- Not built: hardy V coffee/kotton on tap water, drought-resistant fillerweed, rich caffeine donors (bred coffee),
  the fillerweed production module on weed bays, a second manipulator by default.
