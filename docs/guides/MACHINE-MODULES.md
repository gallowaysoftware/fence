# Machine modules: what to put where

A player's guide to the module bays in *Probably Stolen* (build 0.46D). Every number comes from the
game's code, and the important ones were checked by running the game's own code headless. Prices are
the item's base value; Dr. Jackson adds 15 % on normal and **50 % on hard**. The technical version, with
evidence, is [`docs/re/MODULES.md`](../re/MODULES.md).

## Best setup at a glance

| Machine | Budget (early, hard) | Best you can realistically build | Why |
|---|---|---|---|
| **Security alarm** | Overclock + Performance (stop 100 %) | the same, plus a Transmitter on Security | Stops every burglary, so you can **cancel insurance for good** |
| **Hydroponic bay (bloomberry)** | 3 Quality, or 1 Fineness (7 berries a harvest) | 3 Overclock + 1 Fineness: **7 berries every night** (bare: 6 every 2 nights) | Speed only helps at +100 % performance, then you harvest nightly |
| **Projector** | nothing (bare already +10 % budgets) | Capped Neural Core, or 5 Fineness on an expanded bay: attractiveness 400 | Better customers (tier-1 pool) |
| **Wine rack** | 4 Efficiency (24 → 16 power) | Capped Core + 3 Fineness: bottles age 3× a night | A bare rack loses money on anything below Reserve wine |
| **Furnace** | Blast module | Blast + 2 Eco (16 → 10 power a smelt) | Quality modules are not worth it here |
| **Water purifier** | nothing (makes HQ) | 4 Fineness: makes **Pure** (99.90 %) | Pure sells for a third more than HQ |
| **Moisture farm** | nothing | 4 Overclock (2.4× the water), or Capped Core + 2 Fineness (HQ, 1.8×) | Volume beats grade unless you own a core |
| **3D printer** | 1 Performance (most chips a night faster) | see the chip table | Breakpoints, not percentages |

**The two rules that matter most**

1. **Modules only count at breakpoints.** Machines work in whole nights and round their output, so
   +16 % on its own often changes nothing. Check the breakpoint tables below before you buy.
2. **Put the modules in a hydroponic bay before you plant.** The bay locks the moment growth starts,
   and a bloomberry re-locks every harvest, so its bay never opens again unless you chop the plant
   with a hatchet (the seed is lost).

## How modules work (one screen)

- Every module has three numbers: **Performance** (speed or volume), **Efficiency** (power) and
  **Quality** (output grade or count). The machine simply adds up everything in its bay. There is no
  cap and no diminishing return on the sum, but each machine reads the sum differently (below).
- Negative numbers are real: an Overclock (+36 performance, −44 efficiency) raises the power draw by
  44 %; a Fineness (−8, −15, +25) slows the machine a little and costs a little power.
- The standard bay fits **four 2×2 modules**. Long (1×4) machine modules rotate: with careful
  placement a standard bay takes 1 long + 4 squares, 2 long + 2 squares, or 4 long. The game's
  automatic placement is worse (1 long + 2 squares), so place them by hand.
- A **bay expansion kit** (150; two per machine) opens the bay to **six** squares after the first kit;
  the second kit only adds room for long modules. Fit the kit *before* filling the bay.
- **Nodes** (the small 1×2 "random node", 50) boost every module they touch by a fifth of that module's
  numbers, good and bad alike. Two Overclocks with four nodes around them reach +100 % performance.
- **Random shapes and effects.** Jackson's modules come in odd shapes. A three-cell C-shaped
  Performance/Efficiency/Quality module has only ¾ of the stats; a five-cell Overclock, Eco or Fineness
  has **+25 %** (Overclock +45/−55, Fineness +31 quality). From day 21 they may carry effects:
  *Premium* (×1.2), *Inferior* (×0.8), *Top/Side mount* (+20 % against the bay's top/left edge),
  *Receiver* (+10 % per touching node), *Learning* (starts at 0, +1 a working night up to double),
  *Degrading* (double, then −1 a working night), *Overvolted* (double, but **10 % a night to burn out**
  into a ruined module) and *Negative feedback*. Avoid Overvolted and Degrading on machines that run
  every night.

### Module list

| Module | Value | Perf / Eff / Quality | Notes |
|---|---|---|---|
| Performance | 70 | +16 / 0 / 0 | the cheap speed module |
| Efficiency | 70 | 0 / +16 / 0 | |
| Quality | 70 | 0 / 0 / +8 | weak; Fineness is three times stronger |
| Overclock | 100 | +36 / −44 / 0 | the way to big speed |
| Eco | 100 | −16 / +32 / 0 | power saver that slows the machine |
| Fineness | 100 | −8 / −15 / +25 | the quality module to buy |
| Capped Neural Core | 250 | +100 / −100 / +50 | rare (loot crates, 1 % barters); **contraband 3** |
| Uncapped Neural Core | 1500 | +200 / −100 / +100 | Hypertech smuggler only; **contraband 4** |
| Corrupt module | 45 | −8 / −5 / 0 | + a common electronic → a fresh Performance, Efficiency or Quality module |
| Ruined module | 30 | — | screwdriver → electronic + plastic + junk |

## Machine by machine

### Hydroponic bay

Power 8 a night while growing. Performance raises speed at **half** strength: speed 8 becomes 9 at
+25 %, 10 at +50 %, 11 at +75 %, **12 at +100 %**. Quality adds half its value as a percentage to the
harvest count, rounded.

**Bloomberry (the wine plant).** It needs 23 work for the first crop and 12 for each crop after.
Below +100 % performance it fruits every 2 nights whatever you do; at +100 % it fruits **every
night** (first crop on night 2). Berries per crop: 6 bare, **7 at quality 18**, **8 at 50**, **9 at 84**.

| Loadout | Berries a night | Power a night | Cost (hard) | Payback at 14/berry (food) | as wine (~80/berry) |
|---|---|---|---|---|---|
| bare | 3 | 8 | — | — | — |
| 3 Quality | 3.5 | 8 | 315 | 45 nights | 8 nights |
| 1 Fineness | 3.5 | 9 | 150 | 25 | 4 |
| 2 Overclock + 2 Performance | 6 | 15 | 510 | 15 | 2 |
| 3 Overclock | 6 | 18 | 450 | 14 | 2 |
| **3 Overclock + 1 Fineness** | **7** | 19 | 600 | 13 | 2 |
| kit + 3 Overclock + Performance + 2 Fineness | 8 | 20 | 1 080 | 19 | 3 |
| Capped Core alone | 8 | 16 | rare | | |
| Capped Core + Overclock + 2 Fineness | 9 | 21 | rare | | |
| 4 Efficiency (minimum power) | 3 | 3 | 420 | 84 (power saved) | |

Things that do **nothing** on a bloomberry bay: one or two Performance modules (speed 8 → 8 or 9), two
Quality modules (6.48 rounds to 6). Faster crops also spend fewer growth points per harvest, so the
plant is slightly healthier per crop, and water use per berry halves (water is drawn per night).

Other plants (nights to harvest at +0 / +50 / +75 / +100 % performance): nutrifruit 3/3/3/2,
kotton 4/4/3/3, dream cap 4/3/3/3, coffee 3/3/2/2, fillerweed 2/2/1/1, hydroreed 3/2/2/2 (2 already
at +25 %). The plant modules (nutrifruit, dream cap, loom, fillerweed, hydroreed filter, smuggler) are
long modules for their own plant only; see the list at the end.

### Wine rack

Holds 6 bottles. Each night with power it ages every bottle one extra step (two with +125 quality);
the bottle's own +1 a night happens anywhere you keep it. One step adds 3 × the grade multiplier:
2.55 bucket brew, 3 homebrew, 3.75 production, 4.5 reserve, 5.25 master vintage. Power 24 a night;
here **both** performance and efficiency lower the draw (at half strength).

| Loadout | Aging per night | Power | 6 master wines, net a night | 6 production wines |
|---|---|---|---|---|
| no rack | 1 | 0 | free | free |
| bare rack | 2 | 24 | +7.5 | **−1.5** |
| 4 Efficiency | 2 | 16 | +15.5 | +6.5 |
| 4 Performance | 2 | 17 | +14.5 | +5.5 |
| Capped Core + 3 Fineness | 3 | 32 | +31 | +13 |
| kit + 5 Fineness | 3 | 37 | +26 | +8 |

Verdict: a rack only pays with Reserve or Master wine, or with efficiency modules. The gym found that
holding wine back to age costs more than it earns on hard; sell first, age the surplus.

### Furnace

Power 8 per smelt (or junk conversion), nothing on idle nights. Quality turns into flux: +100 replaces a
flux, +200 adds one purity step on a normal furnace. Not worth it.

| Loadout | Per smelt | Power per smelt |
|---|---|---|
| bare | 1 ingot from 2 metal | 8 |
| Blast (150) | 2 ingots from 3 metal, one purity step lower | 16 |
| Blast + 2 Eco | 2 ingots | 10 (saves 6 a smelt: 300 pays back in 50 smelts) |
| Junk module (120) | turns 6 junk into 4 scrap on nights it cannot smelt | |

Blast + Junk are both long modules: they fit together with two squares if you rotate by hand.

### Security alarm

Burglary stop chance = 50 % + performance. **+50 % performance stops every burglary**:
Overclock + Performance (5 power a night) or two Overclocks. The alarm only matters on nights you are
not insured, so a perfect alarm lets you drop insurance completely. On hard, insurance costs twice
(stock value ÷ 400 + 3) a night: about 26 a night with 4 000 of stock, 46 with 8 000. The alarm
(150) + Overclock + Performance cost about 480 on hard and pay back in about three weeks at 4 000 of
stock, under two at 8 000.

**Transmitter** (100, long): +1 reputation a night (0.75 on hard) with the faction it is set to (click it to
cycle Security → Black Market → Upper Level → Revolution). One per faction; with an expansion kit the
alarm holds four transmitters (one per faction) plus Overclock + Performance. Only the first alarm counts.

### Projector

Store attractiveness = 250 (+150 per renovation) − trash + projector. The projector adds
100 + quality ÷ 2 + performance ÷ 4. What attractiveness buys:

| Attractiveness | Effect |
|---|---|
| 200, 250, 500 | +1 normal customer each |
| 300 | customer budgets +10 % |
| 400 | better client pool (tier 1); display buyers promoted |
| 450 / 600 / 750 / 900 | budgets +25 / 50 / 75 / 100 % |
| 650 | client pool tier 2 |

A bare projector takes you from 250 to 350 (+10 % budgets). To reach 400 you need a boost of 150:
a Capped Core alone (32 power), five Fineness on an expanded bay, or four of the five-cell Fineness.
Four ordinary Fineness reach 392, just short. With one renovation, a bare projector gives 500
(+1 customer, +25 % budgets).

### Water purifier

Power 4, only on nights it actually removes something. Quality sets the purest it can reach:
99.01 % (High Quality) bare; **99.90 % (Pure) at quality 100** — four Fineness, or a Capped Core with
two Fineness. Performance speeds the cleaning. Pure is worth 80 a litre against 60 for HQ, and pure
water brings the water stockpiler.

### Moisture farm

850 ml of Ghostwater a night for 6 power. Performance multiplies the volume; quality lifts the grade
(Basewater at 50, HQ at 100, Pure at 150). Each extra farm running cuts every farm's volume a little.

| Loadout | Water a night | Value (at full price) | Power | Payback (hard) |
|---|---|---|---|---|
| bare | 850 ml ghost | 23 | 6 | |
| 2 Fineness | 714 ml base | 29 | 7 | 62 nights |
| 4 Fineness | 578 ml HQ | 35 | 9 | 67 |
| 2 Overclock + 2 Performance | 1.7 L ghost | 47 | 11 | 27 |
| 4 Overclock | 2.1 L ghost | 56 | 16 | 26 |
| Capped Core + 2 Fineness | 1.6 L HQ | 94 | 13 | rare core |

### Evaporator

300 ml a night (× performance) of water or alcohol leaves the container; quality is the share recovered
as pure distillate (none bare). Four Fineness (or two Solvent Recovery modules for alcohol) recover
100 %. Its bay is a plain 4×4 square (four modules, no kit).

### 3D printer

Speed 24 a night; performance adds at full strength. Nights per print = work ÷ speed, rounded up.
Total performance needed (count the −25 of the CNC module and the −50 of the batch module):

| Chip (work) | Bare nights | Faster with |
|---|---|---|
| pistol (50) | 3 | 2 nights at +5 (any Performance) |
| double pistol, 10 mm ext, frame (60) | 3 | 2 at +25 |
| kitchen knife (64) | 3 | 2 at +34 (an Overclock) |
| hatchet (75) / cleaver (82) | 4 | 3 at +5 / +17; 2 at +59 / +71 |
| combat knife (98) | 5 | 4 at +5, 3 at +38 |
| machete (120) | 5 | 4 at +25, 3 at +67 |
| box cutter (34), .22 high (36), .22 ext (48) | 2 | 1 at +42 / +50 / +100 |

Quality adds a quarter of itself to each melee weapon's condition roll: quality 20 (three Quality
modules) removes every *Poor* result (a Fair weapon sells about 17 % above a Poor one). Metal + CNC take two long
slots and leave room for two squares if placed by hand.

### Other machines

- **Desequencer** (10 power, speed 33): one Performance makes the 75-work cards take 2 nights instead
  of 3, the 100-work cards 3 instead of 4 and the 150-work card 4 instead of 5; an Overclock does the
  125-work card in 3 instead of 4.
- **DNA manipulator** (12 power, speed 10, half-strength performance): +20 % performance for each +1
  speed; quality adds half itself to the success chance. The Stabilizer keeps the seed but slows it.
- **Blender, chem finisher, heating plate**: chem modules only, up to four each (see `CHEMISTRY.md`).
- **No module bay**: fridge, RAS, vending machine, bottle printer, recharger, current converter. They take
  material upgrades instead (common electronics cut power or raise attractiveness, ingots and scrap
  add space).

## Power

| Source | What it does |
|---|---|
| Energy cell (100 charge) | about 1 credit per unit of power |
| Breeder cell | refills +4 a night before machines draw: runs a purifier or a bare alarm forever |
| Recharger (150) | +3 a night to each cell inside (+6 with Commercial Power), needs store power |
| Current converter (300) | pays power from cash at 1.5 credits a unit (1.0 after five common electronics; Power Hijack takes 15–50 % off); stops in a blackout |
| Hypertech cell (2 500) | free and endless, **but only for machines drawing 12 or less**: a nightly bloomberry bay (15–21) needs Efficiency modules first |
| Turbo booster | runs a machine's night once more on demand: on a nightly bloomberry bay that is a whole extra crop |

## Where modules come from

- **Dr. Jackson** (in the store on days 6, 8, 12, then every week; or visit him at night). Each visit:
  Performance, Efficiency, Quality and a node 33 % each; Overclock, Eco and Fineness 16 % each; Blast,
  Junk and Transmitter 20 % each; an expansion kit 20 % (two more chances from day 20 and day 40); the
  alarm 66 % until you own one; the projector from day 10 with a chance equal to the day number.
  Farm modules (25 % each) need the Retired Farmer upgrade, chem modules the Retired Chemist, DNA
  modules Plant DNA.
- **His module introduction**: the first time he visits after you own a machine he offers one
  Performance, one Efficiency and one Quality at his first-time discount (−35 % on hard, −75 % on
  normal). **Buy all three.**
- **Monday special stock** (network upgrade, 1 250): five of every module type and five nodes, two kits.
- **Revolution raider** (stolen goods): Performance, Quality, Efficiency, Overclock 22.5 % each.
- **The Underground Exchange** (day 25): two spare modules in, one different module out, once a night.
- **The dump**: about one search in a hundred finds a module.
- **Capped Neural Core**: engineering loot crates (4 %), a 1 % barter, the exchange (1 %). Contraband.

## Buying order for a hard run

1. **Day 6–12, Jackson's module introduction**: take the Performance, Efficiency and Quality (~46 each).
2. **Alarm + Overclock**, as soon as both appear: Overclock + the intro Performance make it burglary-proof;
   cancel insurance from then on. Biggest saving per credit in this guide.
3. **Bloomberry bays**: before planting each new bay, fit whatever reaches +100 % performance (3 Overclock,
   or 2 Overclock + 2 Performance); add a Fineness or the intro Quality modules when they fit. Only worth
   it if your wine is short of berries rather than of bottles, yeast or buyers.
4. **Transmitter on Security** in the alarm (+0.75 a night), if you are racing Security perks.
5. **Expansion kit** for the bay that is full and still short of a breakpoint (bloomberry quality,
   projector 400).
6. **Projector with a Capped Core** if one turns up; otherwise leave the projector bare.
7. Late, and only if you run them: Blast + 2 Eco in the furnace, 4 Efficiency in the rack, 4 Fineness
   in the purifier.

## Gotchas

- A bloomberry bay **cannot be changed** once planted (the hatchet destroys the seed).
- One or two Performance modules do nothing on a hydroponic bay; two Quality modules do nothing on a
  bloomberry; one Performance does nothing on the DNA manipulator.
- Four ordinary Fineness put the projector at 392: eight short of 400.
- Fineness slows a hydroponic bay to speed 7: the first bloomberry crop comes a night later.
- Overclocks multiply the power bill: a bay with 3 Overclock + Fineness draws 19 a night instead of 8.
  On a hypertech cell (12 max) it would not run at all.
- Overvolted modules burn out (10 % a working night); Degrading ones fade; Learning ones start at zero.
- Neural cores are contraband: hide them from inspections.
- The game's auto-placement wastes bay space; rotate long modules and place by hand.
- The plain wine rack loses money on Production wine; the bottles age by themselves anyway.

## Plant modules (long, hydroponic bay only)

| Module | Value | Effect |
|---|---|---|
| Nutrifruit | 150 | +1 nutrifruit a harvest (−10 / −10) |
| Dream cap neutralizer | 150 | +2 caps a harvest, edible, not contraband (+15 / +15) |
| Nightmare cap (1×2, contraband) | 150 | dream caps become nightmare caps (−33 / −50) |
| Kotton loom | 200 | kotton comes out as fabric (−50 / −100) |
| Fillerweed production | 115 | +4 fillerweed, no seeds back |
| Hydroreed filter | 105 | +1 hydroreed filter a harvest per module |
| Smuggler (1×2) | 100 | strips contraband from the seed while it grows |
