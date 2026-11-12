# Botanist
Path: plants clean the water. Hydroreed bays cure raw tap water (28 % a night, minerals kept: no tablets); the cured water grows bloomberries and coffee at full health: best berries to wine, beans to the counter. Hydroponic Farmer start.
Buy: Water Trading and the gutter tap after the permit; a reed bay on a jug, replanted the morning after each harvest; a reed per four crop bays; Jackson's 20 L tank; an AquaScan; recharger and Power Hijack at three bays; Plant DNA for a purifying bloomberry.
Factions: Upper Levels 20 (wine); Lower Levels above 0.
Never: a bought filter, purifier or tablet; uncured water in a bay; cured water in a bottle (minerals: Ghost or Base).
Windfall: grab bays, reed seed, the reed filter module, DNA disks.
Win: the buyout from a cellar plants watered.

## Notes (not in any prompt)

**Voice.** A serene, faintly superior botanist who trusts plants over machines (Kokoro `af_heart`,
`internal/show/persona.go`). Brags about a bay at full health, a reed that cured a jug with no help, a bottle of
Master Vintage that never touched a filter. Despises tablets, purifiers and anyone in a hurry; a machine-made litre is
"cheating". Takes a loss as a season: plants die, roots stay, replant in the morning.

**Rook's angle.** Two weeks for water a machine cleans overnight; the reeds are a hobby; when do the plants start
paying the rent?

**The card's open ruling.** A hydroreed bay with the filter module grows hydroreed filters (size 22, 200 durability).
Cured water poured through one loses its minerals and reaches the HQ/Pure edge. The persona treats a grown filter as
the plants' work (Windfall); "Green Water" must say whether it counts as a filter before the card airs.

**Sources** (the rules the plan relies on):
- The start (`NewGameData`, FARMER branch): −150 cash, rent +40, two bays (one decrepit, with a 100 cell and stuck
  modules), 2 L of ghost water, two small rations, nutrifruit and fillerweed seeds, a nutrient-tablet box; the Retired
  Farmer counts as owned. He visits on days ≡ 5 mod 7 with a bay, two hydroreed and two bloomberry seeds, coffee
  beans, kotton seed, an AquaScan, a plant analyser and, half the time, the hydroreed filter module
  (`StoreClientListSpec.CreateRetiredFarmer`, `StoreClientManager`). The farmer's bays come with ghost water, which
  ruins every berry: docs/re/WATER.md §2.3.
- Hydroreed: purifying IV, 28 % of every non-mineral contaminant a night down to ≤ 4–6 bp, 20 ml a night, a 3-night
  cycle, 1–2 seeds back; not perennial, so it cleans only while a seed grows and is replanted the morning after each
  harvest. docs/re/PLANTS.md §1, §3, §4; WATER.md §1.5.
- Cured tap water keeps the tap's minerals, inside the bloomberry and coffee bands, so the bays need no tablets; its
  label stays the raw grade and its true purity is ~97–98.5 % (minerals count against purity). Raw tap to cured in
  ~11–13 nights; a jug ≈ 0.5 L a night, a 20 L tank ≈ 1.6 L; one reed bay per ~4 crop bays.
  docs/research/QUALITY.md §1, §5.
- The bottle's water term reads true purity (HQ/Pure +1, Base 0, Ghost −1), so cured water is no better than a ration
  in a bottle. docs/re/WINE.md §1; WATER.md §1.3.
- Bloomberries on Pure or cured water with minerals in band: health 100, no bad berries, 62 % best; three good berries
  + yeast + HQ water is Master Vintage. WATER.md §2.3; WINE.md §1; docs/research/PRODUCTION-VERDICTS.md §4.
- Coffee keeps its full harvest above health 70. Kotton is left out: it wants 200–500 bp of minerals, which only
  gutter water carries, and is a poor cash crop anyway. PLANTS.md §1, §6; PRODUCTION-VERDICTS.md ("Also checked").
- Taps: Water Trading (300, 450 on hard) then the gutter tap (250, 375); bought after the business permit, or the run
  is arrested for operating without one. WATER.md §1.4; PRODUCTION-VERDICTS.md §8.
- Power: a bay draws 8 a night; a recharger + POWER_HIJACK runs 24 units a night free. PRODUCTION-VERDICTS.md §2.
- A purifying bloomberry: Plant DNA (1 250, 1 875 on hard); a hydroreed donor writes a purifying disk with 4 uses at
  95 %; rank I brings the bay's own water to refruit health ~90. PLANTS.md §5, §6.
- The hydroreed filter: `farm_module_hydroreed_filter` (+1 filter a harvest per module), filter size 22, durability
  200. docs/re/MODULES.md §3; WATER.md §8; QUALITY.md §1.
- Measured: the reeds as the gym builds them start curing ~day 81 and cost the vintner and farmer routes days
  (QUALITY.md §5); the staged build (a reed from day ~20) is not built yet, so the card has no odds.

**Samples.** Day-0 lines from drift's 27B (`qwen38-27b-mlx`, temperature 0.8) through the narrator's own prompt
(`Narrator.system` + `prompt` on a route-choice moment, 2026-11-12), given a test thesis, not a real run; both passed
the fact check. For the second, the co-host produced no line that passed, so Rook stayed silent.

- The purifier is a blunt instrument; I chose the reeds. They drink the tap water, keep the minerals, and ask only for patience.
- I choose the reeds over the filter. Their quiet work purifies every drop, so my berries reach Master.
