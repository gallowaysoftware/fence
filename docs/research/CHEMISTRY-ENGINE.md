# The chemistry engine (2026-11-12)

**Question.** The owner: "for things like chemistry we understand all the details, so the player should produce
pharma grade chemicals if possible with their equipment." Until now the executor's only chemistry was the
caffeine-only `lab` of SECOND-ENGINE.md, which made street-grade pills. Can an executor that knows the rules
(docs/re/CHEMISTRY.md) make the best grade its kit allows, and does that change the hard routes?

**Verdict.** **The grade, yes; the route, no.** 91–97 % of the units the gym pressed came out at the
highest grade its kit allowed at the time (pharma once a finisher module is fitted, which the engine buys in almost
every run). But on hard the engine still costs the incumbent time, and Board-free it does not win: with the Board the
caffeine lab is 60/60 like the `sprinter`, a median 28 days later (+56.6 ± 14.2 paired); Board-free 0/60 like the
reference and 32 days sooner dead. Dream dust is fatal on the Board route (2/30: security, arrest and execution
endings), and the Cook (the Chemist start, which owns the lab on day 1) cannot keep the cartel's weekly count past
week 2–4. What holds the engine back is the supply around it (beans, alcohol, bays, power, capital), not the
chemistry; the paper model below sizes each wall. The caffeine lab is a legal variety-route candidate, `caffeine_lab`
(ROUTES.md; not on the menu until its ghost band and deadline odds are run).

## What was built

`lab: {...}` in the directives (`sim/gym/Lab.cs`, compiled into the mod; schema `protocol/schema/directives.schema.json`,
docs/PROTOCOL.md). Off by default: with it off the gym is byte-identical to main (sprinter seeds 2910001–004 and a
Board-free rancher pair, summaries equal but for wall-clock fields). `lab: "caffeine"` keeps its old meaning and old
rules.

```json
"lab": {"products": ["caffeine_pill", "dream_dust", "fanta_pill"], "modules": "auto", "payback_weeks": 4}
```

| Key | Default | What it does |
|---|---|---|
| `products` | — | what to make, in priority order (one batch at a time, one work vessel) |
| `modules` | `auto` | chem modules: `auto` buys the next one when its gain pays back within `payback_weeks` (4); `none`; `max` |
| `jackson_need` | 7 | nights between trips to Dr. Jackson while a module is wanted (0: never) |
| `ratio` | 5 (3 under the cartel) | alcohol : dream mush |
| `inputs` | 12 | beans or caps a batch at most |
| `wait_filter_days` | 7 | mornings a batch waits for a water filter (or a fresh one, when the pour came out cloudy) |
| `phanta_target` | 0.66 | the purity a too-pure Phanta residue is cut to (toxic from 75 %) |
| `sell`, `on_shelf` | true, 6 | our own dust and Phanta go to buyers; six of each stay out of the smuggler's bays for them |
| `quota_extra`, `quota_margin` | 10, 12 | the Chemist start: dust held beyond the cartel's count; the cut's floor above the required purity |
| `reserve`, `after_permit` | 500, true | as the old `lab_reserve`, `lab_after_permit` |

### The chains (each step is one morning; the day's seconds run the reactions)

- **Caffeine pills** (legal). Blend the beans; acid equal to the protein and lipobond an eighth of the fat, both cold
  (lipobond boils from 55 °C before it binds); alcohol 1× the mush. Next morning base for any acid left, onto the
  plate (any plate: caffeine extracts from 50 °C and never degrades). Next morning pour through kotton (mush, plant
  matter, debris) and then the water filter (physical contaminant, mineral), so the water filter's 400 durability
  is spent only on what kotton passes. A plate with a module dries the batch above 100 °C overnight (one module:
  fixed 160 °C; two: set 104); an unmodded plate stops at 99 °C, so the evaporator does it. Press when the
  finisher's status says ready.
- **Dream dust** (contraband 2). Blend the caps; acid = protein; alcohol `ratio`× the mush; extraction held at 44 °C
  (the plate's ± buttons step 5 °C from 99: 44 is the nearest setting in the 40–49 window, and it settles ~0.2 °C
  under). A plate with fewer than two modules cannot hold a window: the player watches the readout (whole
  degrees) and switches it between low and off, 42/46 °C, for the morning's 150 s, then off, so nothing sits on a
  99 °C plate overnight (that destroys the extract). Next morning base, kotton then water filter, evaporator (the
  extract dies from 60 °C, so never the plate). Press.
- **Phanta pills** (contraband 3). One precursor bottle, acid at a quarter of its stabiliser. Next morning base for
  the leftover acid, kotton, reducing agent 0.75× the methylol, 124 °C (two modules; one module by the same
  thermostat at 121/124 °C; an unmodded plate never reaches 120). Next morning press, cut with acid first when the
  residue would come out at 74.5 % or more (toxic from 75 %). Needs the spectrometer: a precursor bottle carries
  0.4–17 ml of contaminants, so its pills land anywhere from junk to toxic.
- **Not built: nomore** (legal relief injector). Its only input is dream extract plus reducing agent from the
  restricted (contraband) pack; 40 ml of extract makes 6 injectors worth 702 at lab pure against 20 lab-pure dusts
  worth 840 at base and 1 428 at market. The legality buys nothing while the caps it is made from are contraband
  2 in stock. **Not built: nightmare dust** (a poison, lethal from 6 % purity, only poison buyers).

### The grade each chain reaches, by kit

Purity = residue × (1 − the finisher's impurity); the residue is what the chain leaves (lab, `sim/lab/ChemExps.cs`,
and the gym's spectrometer readings of every residue before pressing, below).

| Finisher modules | 0 | 1 | 2 | 3 | 4 |
|---|---|---|---|---|---|
| Caffeine, kotton + water filter (residue 99.0–99.3 %) | 87 % near pharma | 93 % pharma | 96 % pharma | 98 % pharma | 99.1 % **lab pure** |
| Caffeine, kotton only (residue ~83 %, physical contaminant left) | 73 % street | 78 % street | 80.5 % near pharma | 82 % near pharma | 83 % near pharma |
| Dream dust, kotton + water filter (99.2 %) | 87 % near pharma | 93 % pharma | 96 % pharma | 98 % pharma | 99.2 % **lab pure** |
| Phanta (residue ~62 %, bottle-dependent) | 55 % cut | 58 % cut | 60 % street | 61 % street | 62 % street |

Blender modules change the yield, not the grade (35/20/10/5/0 % loss); plate modules change what can be held
(dream extraction and Phanta need two, or a player at the readout) and whether caffeine dries without the
evaporator (one). Three finisher modules cannot reach lab pure: that needs a 100 % residue. Lab pure needs four.

**Which module next, with numbers** (`modules: auto`, hard). Chem modules come from the retired chemist's weekly
counter (one, then 50/50/25 %, last on a list his machines already fill) at +45 % and from Dr. Jackson at +50 %:
~375 each. Per bean at 8.3 pills (4 blender modules) and hard's sale at the estimate: unmodded 75, first finisher
module 100 (+25: near pharma → pharma), blender 1/2 +23/+15, finisher 2–4 together +16 a module (pharma → lab
pure, nothing for the first two of them), last blender modules +7.5 each. At 4 beans a week the first finisher
module pays its 375 in ~4 weeks and is always bought; the engine weighs the rest against its observed bean rate
and reports `module_plan` (next machine, gain a week, payback weeks). After the first finisher module, one plate
module is bought outright when the lab makes caffeine: it retires the evaporator and its cells.

## What the executor may know

| Quantity | How a player sees it | The engine |
|---|---|---|
| A container's composition | the composition spectrometer (400, comes on the Retired Chemist's counter; no power, not consumed) | reads it only while one is owned; otherwise doses from the recipe's nominal amounts per input and the reactions a player knows run to completion (acid 1:1 protein, lipobond 1:8 fat, acid 1:1 base) |
| Temperature | the plate's readout, whole degrees, container seated | the thermostat reads exactly that; off the plate nothing |
| Volumes | every container's tooltip (whole ml) | glassware pours: a dose is short by less than the smallest glass owned, never over (except lipobond and alcohol, which boil off) |
| Dry or not | the finisher's status line (ready / wet / not enough) | `MachineChemFinisher.CanOutput`, `WaterHelper.IsWet` |
| A unit's grade | its label (true for a unit we pressed) | read for the histogram; the cartel cut reads the residue on the spectrometer |
| The cartel's terms | the handler says this week's and next week's count and purity | `cartelRequiredCount`, `cartelRequiredPurity` |

Never read for a decision: a bottle's hidden contents without the spectrometer, temperature off the plate, the
purity tag of a unit.

## Measured

Hard, paired seeds 2910001–060 (dust, farmer: –030), 1000-day horizon, `-human player -start apprentice -perks
vending+short_sleeper` unless named, this branch's gym (frozen copy of 2c06719; Cook rerun on 3b0578d). Each arm is
`routes/sprinter.json` (the incumbent; `appraisal: "off"` for Board-free) plus the lever. CIs: Wilson 95 % on wins,
paired mean ± 1.96 SE on days; McNemar on the discordant wins.

| Arm | Wins (95 % CI) | Median win / last day | Endings | Paired Δ last day | Chemistry a run |
|---|---|---|---|---|---|
| `sprinter` (reference) | 60/60 (94–100 %) | 97 | buyout 60 | — | — |
| **+ `lab` caffeine** (`caffeine_lab`) | 60/60 (94–100 %) | 125 | buyout 39, mortgage 21 | **+56.6 ± 14.2** | 53 beans, 18 batches, 286 pills (282 sold, 4 185) |
| + caffeine, `crops: {coffee_bean: 3}` | 60/60 (94–100 %) | 209 | buyout 17, mortgage 43 | +92.5 ± 11.2 | 1.5 crop bays, 118 beans harvested (62 sold raw), 47 to the lab, 222 pills (3 011) |
| + dust and caffeine (30 seeds) | **2/30 (2–21 %)** | 206 / 88 | sec 17, arrest 8, execution 3, mortgage 2 | McNemar 28 : 0, p < 0.001 | 18 caps → 45 dust (29 sold, 1 258); 64 pills |
| Board-free reference | 0/60 (0–6 %) | — / 181 | rent 38, repo 22 | — | — |
| Board-free + caffeine | 0/60 (0–6 %) | — / 139 | rent 59, arrest 1 | −31.7 ± 8.8 | lab bought in 52/60 (first batch day 74), 21 beans, 104 pills (1 582) |
| Board-free + caffeine + crops | 0/60 (0–6 %) | — / 132 | rent 36, arrest 22, execution 2 | −45.8 ± 8.9 | lab bought in 11/60, 1.8 bays, 10 pills |
| farmer, Board-free reference (30) | 0/30 (0–11 %) | — / 178 | rent 24, repo 6 | — | — |
| farmer, Board-free + caffeine + crops (30) | 0/30 (0–11 %) | — / 129 | rent 18, arrest 12 | −45.3 ± 10.6 | lab bought in 9/30, 2 bays, 7 pills |
| Cook (`-start chemist`, Board, dust then caffeine) | 0/60 (0–6 %) | — / 35 (quartiles 28–36, longest 61) | cartel 46, sec 13, arrest 1 | — | 4.7 cartel visits, 33 caps → 100 dust (the count first), 25 pills; evaporator out of power on 534 run-mornings, no caps on 370 |

**Grades** (every unit's label against the best its kit allowed at the moment it was pressed: the finisher's
modules and whether the water filter was used; the batch log in `lab.products.<id>.log`):

| Arm | Units at the kit's max | Below it, and why (the residue's first impurity on the spectrometer) |
|---|---|---|
| caffeine (Board) | 15 692 / 17 154 (91.5 %) | 1 159 debris (a water filter spent mid-pour, pressed after the week's wait), 274 fat |
| caffeine + crops | 12 259 / 13 293 (92.2 %) | 859 debris, 77 fat |
| caffeine, Board-free | 6 036 / 6 246 (96.6 %) | 159 fat, 34 protein, 17 debris |
| dust + caffeine | 3 183 / 3 277 (97.1 %) | 41 fat, 40 protein, 8 acid |
| Cook | 5 065 / 7 508 (67 %) | 1 914 acid: the deliberate cut that stretches extract to the cartel's count (judged on mean purity); 316 protein, 149 plant matter |

**The Cook, why it fails.** The Chemist start skips the 1 500 unlock and gets free caps and a module a week from the
cartel, but its weekly count rises by 5 without limit (10, 15, 20, … at 40 → 70 % purity) and the run ends at the
first visit with no dust, or at −100 cartel rep (−4 a missing unit). Dust needs the evaporator (the extract dies from
60 °C, so the plate cannot dry it), the start's evaporator holds 40 charge (6 nights), and cells cost cash the start
does not have (−300 at the start): the lab stops drying in week 3–4 and the count is missed. The engine's own levers
here: the count first (nothing else takes the vessel), alcohol at 3× (less to evaporate), the cut with acid up to the
required purity + 12 %, exactly the count staged on the counter, the materials at the check-in when short, never
reporting the cartel. What would carry it: free power (POWER_HIJACK + a recharger) for the evaporator, and the
shady farmer's caps every visit; the count is then ~10 caps × 5–9 units a week against a quota that reaches 50 by
week 9, so the exit (rep 100, then 200 after four more visits) is a race the gym has not yet won.

The finisher's first module is the grade: pills were 76 % pharma, 14 % near pharma (before the module came) in the
caffeine arm, mean finisher modules 1.0, blender 0.9, plate 0.9 at the end. The `module_plan` pegs the next module's
payback at 5–6 weeks on the observed bean rate (~4 a week), so the engine rarely buys the 2nd–4th finisher modules
that lab pure needs; `modules: "max"` buys them regardless.

## The closed loop: paper model and the gap

**Best case on paper** (hard prices; a pill sells at its estimate). The verdict audit (PRODUCTION-VERDICTS.md) shows the
counter takes ~24 pharma pills a day (a free probe at that rate wins 16/16 Board-free by day 90) and tops out ~28–30.

| Link | Rate | Source |
|---|---|---|
| Coffee bay on free tap water | 4 beans / 3 nights, 1 net a night after replanting (HQ water: 6, 1.67 net) | SECOND-ENGINE.md, PRODUCTION-VERDICTS.md |
| Beans → pills | 5.4 (blender 0) … 8.3 (blender 4) a bean | CHEMISTRY.md §6.2 |
| Pill price | near pharma 14, pharma 18.5, lab pure 23.5 | §4 |
| 24 pharma pills a day | 3–4.5 beans a day: 3–5 tap bays, or 2–3 on HQ water | |
| Alcohol | 0.75–1× the mush, 34–45 ml a bean: a 350 ml common pack does 8–10 beans; packs come ~1 a week (retired chemist) plus the scav seller's 25 % + 25 % | PRODUCTION-VERDICTS.md |
| Lab | one vessel, ≤ 12 beans a batch, a batch every 3 mornings (plate-dried) or 4–5 (evaporator): ≤ 4 beans a day | this engine |
| Power | bays 8 a night, evaporator 6: free with POWER_HIJACK (450) + a recharger (225) | PRODUCTION-VERDICTS.md |
| Capex | Retired Chemist 1 500, Retired Farmer 525, 4 bays ~1 560, taps 825, power 675, a module 375: ~5 500 | |

So the best case is ~24 pharma pills a day, ~3 100 a week, from ~3 weeks after the lab and bays are bought.
(*Corrected, see "The supply loop" below:* this paragraph first named alcohol the binding link, sizing it as if
extraction consumed it. It does not: alcohol is a solvent in R1 and R7, lost only when a batch dries. One bath of
~250 ml takes a whole batch in increments, the solvent-recovery module returns half of it at drying, and it does not
slow the evaporator (its −25 is efficiency, i.e. power 6 → 7). Alcohol is not a wall; beans, capital and the stock
grid are.)

**What the gym achieved and why** (caffeine with the Board, the best arm): 53 beans and 286 pills a run, ~26 pills a
week from the lab's day ~51 to the buyout, against a demand of ~170 a week.
1. **Beans.** Without crops the lab sees ~5 beans a week from visitors (the retired farmer's 2, the shady farmer's
   ~1.5 twice a week). With `crops` the executor reached 1.5 crop bays of the 3 asked for and drew 11 L of tap water
   a run (a bay drinks ~1 L a night), so the bays were mostly dry: 118 beans in a run where 1.5 wet tap bays make
   ~150 net. Bays come one per retired farmer visit and cost cash the Board route is saving for the buyout.
2. **Alcohol and the vessel.** Even with beans in stock the lab used 47 of 118: packs (~1 a week) cap a batch, and
   one work vessel runs one batch at a time; 62 beans were sold raw.
3. **Capital before day ~70.** Board-free, the 1 500 unlock waits for cash: bought in 52/60 runs (first batch day
   74) alone, and in only 9–11 of 30–60 when the crop bays and taps compete for the same cash, while the permit's
   money goes short (arrests 12–22 of 30–60). The audit's costed probe (`c92x24L`, the farmer's kit, 6/16) assumed
   a lab that already makes 24 pills a day; this executor never gets there.
4. **Selling.** Pills go to stock, not to a window slot worth ≥ 350, so the tier-3 medical buyers the audit names
   are not pulled.

**What would close it**, in the order the numbers say: free power (POWER_HIJACK + recharger) and a bay watering
chore that keeps 3–5 coffee bays wet on tap water; alcohol (count packs, buy every pack offered, the solvent-recovery
module, 0.75× alcohol); a second work vessel so batches overlap; the lab bought in stages from day ~20 (the permit
first) rather than as one lump at day ~70; the pills in a MEDICAL window slot. Each is an executor chore around the
lab, not chemistry. The water work is ../fence-quality-max's: what this engine needs from it is a litre of tap (or
HQ) water a night per coffee bay, delivered by the bay chore.

## The supply loop (2026-11-12, branch chem-loop)

**Question.** The engine reaches the best grade its kit allows but makes ~26 pills a week against a paper best case of
~24 a day. Close the supply walls (power, alcohol, beans, throughput, capital order, selling, the Cook) and measure.

**Verdict.** Every wall the paper named is now closed or measured, and **the loop still loses**. On the Board route it
costs the buyout; Board-free and on the farmer start it dies sooner than the reference. What binds is capital and the
stock grid, before any supply wall. Alcohol was never a wall: it is a solvent (below). Free power is real, and it
costs a Board run ~20 days. The coffee bays feed the lab ~1 bean a day at best.

### What was built (all off by default; with `lab` and `power` absent the gym is byte-identical to main: `sprinter`
2910001–004 and `fish/aqua_fish_all_noboard` 2910001–003, summaries equal but for the new `crops.cured_swaps` field)

- **Free power, `sim/gym/Power.cs`, `power: {...}`.** POWER_HIJACK (450) once the kit pays back against bought cells
  (`payback_weeks` 4, `unit_price` 1.5 a unit), Dr. Jackson's recharger (225; +3 a cell a night while the store has
  power, `MachineRecharger`), COMMERCIAL_POWER (525, ×2) once the draw passes the rechargers' 24 a night, and a second
  recharger after that. The services drop POWER_SERVICE once the hijack is in. **The interface is the loose charged
  cell.** Each morning, before the engines run, spent loose cells go into the rechargers. Charged cells (≥ 30) come out
  until `loose` of them are loose (default: one per machine holding a battery plus one, at least four). Any engine that swaps a loose
  cell into its machine and leaves the spent one loose (all of them do) runs on recharged cells unchanged.
  `Power.Need(engine, perNight)` reports a draw for the payback test, and `Power.TopUp(machine, min)` swaps for an
  engine with no cell logic of its own. While `lab` is on and its four machines are not all in, power waits: a kit
  with nothing to power is capital spent early.
- **Alcohol as a solvent: `lab.bath` (default on wherever the driver runs the morning's seconds).** R7 converts each
  second's 15 % chunk of coffee mush at 0.75 × min(1, alcohol / chunk / 5), and R1 reads alcohol against the dream
  mush still present. Neither consumes it (`ChemicalHelper.HandleCoffeeBeanExtractionSolvent`,
  `HandleDreamCapMushReaction`). So one bath, sized for the inputs at hand and capped at `bath_ml` (250 ml coffee,
  300 ml caps), takes the batch in increments. Each increment is sized to the full yield (0.8× / 5× its mush) and fed
  once the last has converted: 25 s at ≥ 50 °C (0.85^25 ≈ 2 % left) for coffee, 40 s at ≥ 40 °C for caps. Coffee is
  held at 50–54 °C, under lipobond's 55 °C boil and well under water extraction's 80 °C, which turns mush into plant
  matter at an eighth of the yield. The old flow held 99 °C and lost mush to it. Caps are held at 40–49 °C. Each
  increment gets its own acid (1:1 protein) and lipobond (1:8 fat); both are consumed, and lipobond binds at 20 % a
  second. A bath waits up to `bath_days` (4) for more inputs (not while the cartel's count is short), then filters
  and dries once.
- **Alcohol back: the solvent-recovery module.** It gives Q +50 and E −25 (power 6 → 7, not speed;
  `ModuleDirectory.EvaporatorSolventRecovery`). The evaporator returns Q % of each evaporated liquid to the bottle in
  its distillate slot, alcohol only with the module. The engine buys the module, seats an empty mini bottle as the
  distillate, and takes it out at 150 ml as a supply bottle. With the module fitted, `dry: auto` dries a bath in the
  evaporator; without it, on a modded plate. Fineness modules (Q +25, a 3 % barter offer) go in when seen: two of
  them reach Q 100, every ml back.
- **Packs:** alcohol for `alcohol_weeks` (2) of the observed input rate, every pack offered, at most `packs_max` (6).
- **Beans and bays:** inputs held from sale and bought up to `hold` (two batches; a raw bean sells for ~25, pressed
  ~100). Crop bays (`crops`, Crops.cs) take reed-cured water (`Production.PlantSafeWater()`, the water engine's
  interface) when there is some (`crops_cured`, default on). `caffeine_inputs: [ids]` lets the lab press bred produce
  that carries the caffeine Production trait; the gene-lines engine grows it, and `Lab.TakesInput(id)` tells it what
  the lab takes.
- **Throughput:** a batch is up to `inputs` (20) per vessel, sized to the vessel's room. A second vessel was not
  built: the lab never ran out of vessel time (no run stalled on a busy vessel), only out of inputs, vessels,
  filters and machines.
- **Vessels, filters, room:** three water pitchers (a burglar takes one owned item a night when the shop is
  uninsured, and the chemist offers a pitcher every other week); an empty mini bottle as a two-bean work vessel when
  nothing larger is left; four water filters and two kotton. A full stock grid used to leave a bought machine on the
  back counter, where the next buyer's "sell all" took it (a heating plate was bought and sold three weeks running).
  Now the cheapest plain stock makes way, or the display case holds the machine, and kit lots that will fit are
  bought into a full grid. The chemist's storage bay (520, `chem_bay`) takes the small kit when the grid is short.
  Pills press only while the output grid has room: a unit pressed into a full grid is lost.
- **Capital order:** the lab, the crop taps and the power kit wait for the permit (`after_permit`); power waits for
  the lab's machines.
- **Selling:** with `window_mode: concentrate` the case goes MEDICAL once our pills reach 350 (`lab.window`;
  ShowcaseHelper: the hospital administrator, 750 + day to spend, from day 14; caffeine pills carry HOUSEHOLD too, which
  counts toward the upper maid's 300 when the case brings a second type). On hard there is no retail markup: a pill sells at its value, ~14–16 by grade.
- **The Cook:** power is not held back on the Chemist start (its machines are in from day 1), and Jackson's
  disposable cells are bought while the evaporator is low. The cartel's dust uses the bath (caps at 5× the mush still
  present), so drying a batch takes one evaporator night, not three to four.

### Measured

Hard, paired seeds 2910001–060 (the Cook 30), 1000-day horizon, `-human player -perks vending+short_sleeper`. Board
arms are `routes/sprinter.json` plus the lever; Board-free sets `appraisal: "off"`. Directives are in
`sim/lab/scripts/directives/lab/` (`caffeine_lab`, `caffeine_lab_power`, `caffeine_loop`, `*_noboard`, `cook`,
`cook_loop`). Wilson 95 % on wins, paired mean ± 1.96 SE on the last day, exact McNemar on discordant wins. Gym builds:
pre-bath = 7c22c84 (the Board arms B1–B3); bath = 4471f26 (everything else; the Cook on 9b2904d).

| Arm | Build | Wins (95 % CI) | Median win / last day | Endings | Paired Δ last day | McNemar b:c | The lab a run |
|---|---|---|---|---|---|---|---|
| `sprinter` (reference) | — | 60/60 (94–100 %) | 90 | buyout 60 | — | — | — |
| B1 + lab (supply fixes, no power, no bays) | pre-bath | 60/60 (94–100 %) | 125 | buyout 34, mortgage 26 | +70.0 ± 14.6 | 0 : 0 | 415 pills, 71 beans, 4.9 packs |
| B1 + lab | bath | 60/60 (94–100 %) | 132 | buyout 38, mortgage 22 | +62.9 ± 12.0 | 0 : 0 | 362 pills, 64 beans, 3.4 packs, 30 increments |
| B2 + lab + `power` | pre-bath | 60/60 (94–100 %) | 205.5 | mortgage 40, buyout 20 | +90.7 ± 11.5 | 0 : 0 | 502 pills, 77 beans; power free from day 54.5 |
| B3 + lab + power + coffee bays (`caffeine_loop`) | pre-bath | 58/60 (89–99 %) | 209 | mortgage 56, buyout 2, repo 2 | +114.0 ± 7.8 | 0 : 2 | 560 pills, 85 beans, 128 harvested |
| B3 `caffeine_loop` | bath | 57/60 (86–98 %) | 209 | mortgage 56, rent 2, buyout 1, repo 1 | +114.2 ± 5.7 | 0 : 3 (p 0.25) | 646 pills, 104 beans, 140 harvested (1.45 bays, 32 sold raw), 4.7 packs, 0.7 L recovered |
| Board-free reference | — | 0/60 (0–6 %) | — / 188 | rent 43, repo 15, arrest 2 | — | — | — |
| F1 Board-free + lab | bath | 0/60 (0–6 %) | — / 146 | rent 57, arrest 3 | −35.6 ± 10.6 | 0 : 0 | lab in 42/60 (first batch day ~74), 107 pills |
| F2 Board-free `caffeine_loop_noboard` | bath | 0/60 (0–6 %) | — / 121.5 | rent 50, arrest 10 | −57.8 ± 10.1 | 0 : 0 | lab in 26/60, 58 pills; power free in 10 |
| farmer, Board-free reference | — | 2/60 (1–11 %) | 195 / 181 | rent 46, repo 12, mortgage 2 | — | — | — |
| farmer, Board-free loop | bath | 0/60 (0–6 %) | — / 118 | rent 45, arrest 15 | −57.0 ± 8.3 | 0 : 2 (p 0.5) | lab in 19/60, 22 pills |
| the Cook (`cook`, 30) | bath | 0/30 (0–11 %) | — / 39.5 | cartel 14, sec 12, arrest 3, execution 1 | — | — | 5.5 cartel visits, 54 caps → 200 dust, 90 staged |
| the Cook + power (`cook_loop`, 30) | bath | 0/30 (0–11 %) | — / 37 | cartel 20, sec 9, arrest 1 | −2.8 ± 3.9 | 0 : 0 | power free in 22/30 (median day 27), 51 caps → 180 dust |

**Legal Highs** (caffeine only, no contraband bought or sold, which are the defaults) is the B and F rows: the lab
never touches contraband. **Grades:** on the bath build (B1) 81 % of the lab's pills were pharma, 12 % near pharma and 1 % lab pure;
5 % were cut grade: batches poured through the water filter alone, with no kotton, leave a fifth of the residue as
debris. From 19acb0e a batch waits for kotton as it does for a water filter (not in these runs).

### Paper against gym, wall by wall

| Link | Paper (best case) | Gym, best arm | What the gap is |
|---|---|---|---|
| Beans | 3–4.5 a day (3–5 tap bays) | 4.4–5.3 a **week** used (B1, B3) | Visitors bring ~5 a week. The bays ran 1.45 per run (of 3 asked), at tap-water health (×0.5 harvest): 140 harvested a run, 32 of them sold raw before the lab reached them. Bays come one per farmer visit and cost cash the buyout is saving |
| Pills | ~24 a day | 27–30 a week | beans × ~5.7 pills a bean (pharma, blender 0–1 modules) |
| Alcohol | first written as the wall (1 pack ≈ 10 beans) | 0.2–0.3 packs a week; 3.4 packs a run against 4.9 before the bath | Not a wall: a solvent. A 250 ml bath takes a 20-bean batch; recovery returned 0.35–0.7 L a run. Packs are bought for their acid and lipobond as much as for alcohol |
| Power | free for 675 | free from day 54–66 (B2, B3; 10/60 Board-free) | Free, but the kit (hijack 450, recharger 225, COMMERCIAL_POWER 525, ~18 cells) costs a Board run ~20 days (B1 → B2) |
| Vessel and batches | ≤ 4 beans a day on one vessel | no run stalled on vessel time; ~30 increments a run | Inputs bind long before the vessel does; a second vessel would add nothing here |
| Capital | ~5 500 kit, staged | Board-free: the 1 500 unlock in 19–42 of 60 runs, first batch day ~74–80 | The permit, rent and bays come first; arrests rise when the taps and power compete with the permit (F2 10, farmer 15) |
| Stock grid | — | B3: machines missing on 59 mornings a run, the grid full on 7 | Every engine's reserved inputs (bays, berries, bottles, beans, packs) fill the ~240-cell grid; the lab now makes room, uses the case and the chem bay (bought in 32/60) |
| Selling | pharma at 18.5 | 13.4–15.7 a pill sold | Hard has no retail markup; the medical case brings the administrator once 350 of pills are shown |
| The Cook | quota race | dies at day ~37–40 | Caps: ~10 a week from the cartel, the shady farmer and the check-in against a count that rises by 5 a week. At ~3.7 dust a cap after the cut, the count outruns the caps by week 6–8. Power is not the wall (stalled on ~2 mornings a run either way) |

### Each reagent's role (round 3, verified in `ChemicalHelper`, measured on the bath)

| Term | Source | What the engine does | Measured |
|---|---|---|---|
| Alcohol | a ratio in R1 and R7, never consumed; boils above 78.37 °C | one bath per batch, extraction at 50–54 °C / 40–49 °C, recovered at drying | above |
| Microbes | R17 kills all while alcohol ≥ 40 % of the total; R16 heat (20 %/s ≥ 70 °C, all ≥ 100 °C); acid/base kill them too | nothing extra: beans carry none, and the caps' microbes die to the acid dosed for protein | 0.00 % microbe in every residue logged (B1, B3, the Cook: 1 531 batches) |
| Caffeine | no degradation reaction | dried on a modded plate at 160 °C when no recovery module is fitted (no organic waste in a coffee batch, so R20's ≥ 80 °C mineralisation never applies) | — |
| Acid / base | R10 precipitates protein with the **net** excess only; R13 neutralises both at 10 %/s | acid for each increment's protein alone; base the next morning for what is left | residue acid 0.01–0.09 %, protein 0.5–0.7 % (the dose falls short by under the smallest glass) |
| Lipobond | consumed 1 : 8 by fat (R11, 20 %/s); a catalyst only in nomore (R9) | with the spectrometer the dose is fat ÷ 8 + 0.1 ml (it was + 0.5 ml an increment); its bottle's impurities stay in the residue | residue fat 0.2–0.3 %, chemical contaminant 0.5–0.6 % |
| Kotton and water filter | durability: 2 a ml removed (kotton 1 000, filter 400); a spent filter passes the rest | the old filter now leaves a full grid (chem bay, room made, back counter) so the next one can seat; a batch waits up to 21 mornings for kotton | cut-grade pills **4.6 % → 0**, a pill sold **15.42 → 15.77** (B1, 60 seeds, the bath build against round 3: +62.9 ± 12.0 → +61.4 ± 12.2 days against `sprinter`) |
| Phanta (R14/R15) | only methylol in excess of the stabiliser reduces; ≥ 130 °C destroys | unchanged (held 120–129 °C, stabiliser stripped with ¼ acid); Phanta is contraband and in no route measured here | — |
| Day seconds | reactions tick through the open day and the 360 night seconds | the bath's increments run in the morning seconds; settling (acid, lipobond) runs through the day | — |

### What is left

- **The wall is beans and capital, not chemistry or alcohol.** A lab pays only when it presses ~3+ beans a day from
  early on. That needs 3–5 bays watered with cured water (health > 70: twice tap water's harvest) and powered from
  day ~20, bought before the Retired Chemist. The staged clean-water build (../fence-water-cellar) and bred,
  bountiful coffee (../fence-gene-lines, `caffeine_inputs`) are where the beans have to come from. Until then the
  lab is a ~+60–115 day cost on the Board route and −35 to −58 days Board-free.
- **The stock grid** is a cross-engine wall: chem machines (9 cells each), bays (12) and reserved inputs fill it.
  Storage bays (Breadth.cs `storage_bays`) for plain stock, and engine inputs in their own bays, would let the lab
  run. Here it borrows the display case.
- **The Cook** needs caps the source does not sell at that rate (no seed for sale; the cartel gives 3–6 a week).
  The count rises by 5 a week. Not winnable on these sources; the exit (rep 100, then +25 for each of four stages)
  was reached to stage 2 at most.
- **Unbuilt:** distilling wine for alcohol (a 150 ml pour; ~120 a bottle sold against 65 a pack: it does not pay
  while packs come), a powerblock beside the evaporator (needs grid placement), fineness and quality modules from
  barter (wired, too rare to measure), nightmare caps for the poison barter's cell.
- **Live port:** Power.cs is plain item moves and the existing upgrade and Jackson paths, so it runs live as it is.
  The bath feeds increments from `Lab.Second()`: blender presses and vessel moves once a second. The live driver
  already ticks `Lab.Second()` (LiveMachines.cs), so it needs those presses gated like the others. Not attempted:
  the loop does not win.

## Live

Built, not yet run in the game (the owner's test: docs/STREAM-OPS.md section 12.3). The engine's moves and pours go
through `Chore` like every engine. Its window buttons are the live host's (`LiveHost.MachineButton` → the driver's
gated press; `mod/src/Fence.Mod/LiveMachines.cs`):

- The blender's start and the finisher's output are found by their label in the window's public widget tree and
  pressed as a click selects them (`SelectableElement.Select`, never hidden or disabled), so the `no_buttons` stall
  is gone.
- The finisher's configure (`ChemFinisherUIManager.OnChemRecipeSelected`) and the plate's radio and ± buttons are
  the callbacks' own bodies (a tag set that the window re-reads every second). The engine already runs them through
  `Chore.Act`.
- The thermostat: the driver calls `Lab.Second()` once a second of wall time while it runs, and `Lab.EndMorning()`
  with the evening chores. `Lab.Thermostat` is on live, as in the gym.
- The cartel's `Lab.StageQuota()` moves dust while the handler's line is on screen (class d) and is untested.
- Hooks: `autopilot.machines` in `compat/hooks.json`. A missing member turns the presses off, and the lab stalls as
  before.

## Reproducing

```
fence-gym smoke -seeds 60 -seed0 2910000 -days 1000 -parallel 9 -no-trajectory -human player -difficulty hard \
  -start apprentice -perks vending+short_sleeper -directives <routes/sprinter.json + "lab": {...}>
```
