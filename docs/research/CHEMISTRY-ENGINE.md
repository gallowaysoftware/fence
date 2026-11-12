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

So the best case is ~24 pharma pills a day, ~3 100 a week, from ~3 weeks after the lab and bays are bought. **The
binding link on paper is alcohol**: one pack a week runs ~10 beans, ~55–80 pills a week (~8–11 a day). Without the
solvent-recovery module (275; it keeps half the evaporated alcohol but slows the evaporator by a quarter) or more
packs, the lab cannot reach 24 a day whatever the bays grow. (Drying caffeine on a modded plate boils the alcohol
off instead of recovering it: the engine trades alcohol for evaporator nights there.)

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

## Live

The engine is in the shared executor (`SharedExecutor.props` compiles `Lab.cs` into `Fence.Mod`), and its moves and
pours go through `Chore` like every engine. It cannot run live yet, and it does nothing there: `LiveHost.MachineButton`
returns null (machine windows' buttons are not pressed live), and the lab checks the blender's start button before
any batch step (`no_buttons` stall; it still stows its kit and fits loose modules, plain moves). Porting needs:

- `LiveHands` presses for the blender's start, the finisher's configure (the recipe pick in `ChemFinisherUIManager`)
  and output, and the plate's off/low/high radio and ± buttons, each re-read like the other machine actions;
- the morning thermostat: a per-tick call of `Lab.Second()` from the driver for ~150 s and `Lab.EndMorning()` after
  (or require two plate modules live, which hold a window without it);
- the cartel: `Lab.StageQuota()` moves dust onto the back counter while the handler's line is on screen (a move
  during a dialogue, class d in docs/STREAM.md);
- hooks: every game member the engine touches is in `compat/hooks.json` under `autopilot.executor`, and
  `RuntimeResolver` knows the new types (`make test-mod` passes).

## Reproducing

```
fence-gym smoke -seeds 60 -seed0 2910000 -days 1000 -parallel 9 -no-trajectory -human player -difficulty hard \
  -start apprentice -perks vending+short_sleeper -directives <routes/sprinter.json + "lab": {...}>
```
