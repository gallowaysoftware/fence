# The build planner: one capital plan across engines (2026-11-12)

**Question.** Every engine built this week is worth a lot on paper and about nothing in the gym, and the measured
causes were never the engine's mechanics: they were capital, kit and order around it. The lab's blender or finisher
missing on ~2 600 run-mornings (BREEDING.md §5), the DNA manipulator without a charged cell on ~2 000, crop bays
bought at 0.78 of the 2 asked, the clean-water kit draining ~1 450 of the counter's stock money over days 44–80
(QUALITY.md §8), watered bays unpowered 89 nights a run, permits short. Each engine bought its own kit with its own
reserve, none knew the others or Georgio's loan. With one planner doing that job, does the paper value become real?

**Verdict.** **The planner works; the paper value still does not become real.** One capital plan rescues the runs
the engines were killing: with the caffeine-weed line, lab, fillerweed bays and power on the farmer route, 58/60 win
instead of 37/60 (McNemar 23:2, p < 0.001; 60/60 with the horizon at day 98), and 56/60 instead of 43/60 on
`sprinter` (15:2, p = 0.002). Kits come whole (power 60/60 complete, crop bays 46–48/60 against 0.78 of 2 bays
before), before Georgio's loan nothing is spent, and power comes first (unpowered bay-nights 32 → 13 and 48 → 21).
But no engine beats the route it is added to: the best stacks are still +70 to +100 days (most of those wins are the
mortgage's end), the clean-water cellar is +41 days with or without the planner, the forge +81 to +92. The one
exception is `water_baron`, where the planner declines the clean-water kit and the stack returns to the route
(below). What blocks is no longer capital or order but three things the planner can only route around: the
**stock grid** (the engines' machines and kit leave 0–8 free cells from day ~50, against ~90 on the route, and a lot
the grid cannot take is a lot the counter turns away: counter buying halves, 8 900 → 4 000–4 700 over days 1–90), the
**sellers** (a committed kit waits 25–75 mornings for the Retired Chemist, Farmer or Dr. Jackson to bring its part),
and the **short Board horizon** (the routes buy out around day 91–105; a kit with a 7–25-day ramp has ~40 days to pay
1 000–3 300 back).

## Design

`build: {...}` in the directives (`sim/gym/BuildOrder.cs`, the kits in `sim/gym/BuildKits.cs`; schema
`protocol/schema/directives.schema.json`, docs/PROTOCOL.md). Off when absent: with it off the gym is byte-identical to
main (checked on seed 9400003, `caffeine_lab`: equal cash and net worth every day).

Each morning, after the day's plan (`ExecutorMorning.Plan`):

1. **Holds.** The rent and the services until rent day; the permit fee until it is bought (or, for a donating shop,
   the official's next weekly amount: 25 + 10 × ⌊day/7⌋ on top of what is owed); the counter's stock money
   (`trade_days`, 4 days of what it spent on goods over the last week, at least `trade_floor` 300). Only cash above
   these is the planner's.
2. **Georgio.** He lends a share of the loan morning's cash and repays four times it (FORMULA-EXPLOITS.md §5), so
   from `georgio_hold_days` (7) before that morning nothing new is committed and no committed part is bought; before
   it, a kit's cost counts × `georgio_mult` (2.5). After the loan the queue opens that evening (Wild's upgrades are
   bought at night).
3. **Kits, not fragments.** An engine describes its next purchase as a kit: the parts still missing (upgrade at Wild,
   item from a counter seller, item at Dr. Jackson's), its price (the run's paid price once one is paid), its income a
   day once complete (paper defaults, `income` overrides), its ramp, the kits it needs first, its nightly power draw
   and the stock cells it takes. A kit and its incomplete prerequisites are one unit: committed whole when the budget
   covers all of it, or not at all.
4. **Order.** Units are ranked by income a credit over the days left after their ramp; a unit must pay back before the
   horizon (`horizon`, else the projected cash buyout: cash + 70 % of stock against the price at the last four weeks'
   net-worth gain, capped by `buyout_by_day`; Board-free `horizon_default` 200). `must` kits skip that test.
5. **Commitment.** A committed kit keeps its claim on the budget until complete; nothing new jumps it.
6. **Gates.** The engines keep their own buying code. For the ids the planner manages it answers their gates:
   `Production.Wants` (counter sellers, through `BuildOrder.Cap`), `PlayerRole.Upgrades` (Wild: a committed part
   goes first in the night's order, past the engine's reserve; favor alone may still buy it), `PlayerRole.JacksonWants`
   and the night's trip to Dr. Jackson (`WantsJackson`, every `jackson_every` 5 nights while a part waits on him).
   Ids the route's own core also buys (the cellar's bays, WINEMAKER's water trader and farmer) keep their old rules
   alongside.
7. **One stow path.** Kit parts and engine-held items left on the back or front counter go to stock, else the display
   case; a machine that fits neither pushes the cheapest unreserved stock out for room (`BuildOrder.Put`). Parts the
   planner bought are held from sale until their engine takes them.
8. **Storage** (`storage: true`): a storage bay is a kit, put before any kit whose parts the stock grid cannot take,
   and owned bays are used (sim/gym/Breadth.cs `Storage`).

### The engine interface

```csharp
internal sealed class BuildKit {
    string Name; List<BuildPart> Missing;      // id, count, price, BuyAt.Wild | Seller | Jackson
    double IncomePerDay; int RampDays;          // once complete, net of running inputs
    string[] Needs; int DrawPerNight; int Cells; string Blocked;
}
```

`BuildKits.All()` builds today's kits by reading what the shop owns (no moves). Kits today:

| Kit | Engine on | Parts | Paper income a day (default) | Needs |
|---|---|---|---|---|
| `cellar_bays` | wine, only when `kits` lists it | bays to `bays` | 20 a bay | — |
| `crop_bays` | `crops` | WATER_TRADER, GUTTERFLOW_TAP (with `crops_tap`), bays | 30 a bay | — |
| `water` | `clean_water` | RETIRED_FARMER (no wine), the taps, the reed bays, a 20 L tank a reed | 50 | — |
| `lab` | `lab` | RETIRED_CHEMIST, blender, finisher, plate, evaporator | 45 | — |
| `dna` | `wine_dna` or `breeding` | PLANT_DNA, manipulators, two blank disks, a hydroreed seed for a bloom line | caffeine weed 40 a weed bay, bloom 15, `wine_dna` 25 | `lab` (the press), `crop_bays` (the weed), `cellar_bays` |
| `forge` | `arms.forge` | the printer, metal and CNC modules, the part and handmade chips (the heavy chip stays the arms engine's, after Permit II) | 33 a printer | — |
| `power` | `power` (Power.cs) | POWER_HIJACK, a recharger | 1.5 a unit of the other kits' draw (24 at most) | — |
| `storage` | `storage` (default on) | a storage bay when the grid lacks the kits' cells + 36 | — | — |

Every kit that draws power needs `power` while that kit is incomplete; every kit with loose parts needs `storage`
while the grid is short. Cells and COMMERCIAL_POWER stay Power.cs's; a second recharger is Power.cs's rule once the
first is in. A kit's parts are not bought until the kits it needs are complete.

The planner reads only what a player sees: cash, the rent and permit lines, Wild's prices, what the shop owns and
where it lies, the ledger of its own sales and purchases (FairPlayTests guard: no hidden accessor in either file).

## Measured

Hard, 1000-day horizon, `-human player -perks vending+short_sleeper`. Wins are buyouts (cash or the mortgage's end),
Wilson 95 %; Δ is the paired difference, mean ± 1.96 SE (win day over seeds both arms won; last day over all seeds);
McNemar exact on the discordant wins. `sim/lab/scripts/buildorder.py` runs each seed as its own `fence-gym run` and
prints these columns and the planner's telemetry (`player.build`: kits approved / complete, spend, the mornings a
committed kit waited and on what). Builds are frozen copies under `var/sim/gyms/`.

### Finals (60 seeds 2910001–060, build bo5 = main 2a90fcc with fair play + this branch)

`sim/lab/scripts/arms-build-final.txt`. "+ planner" is the engine arm plus `build: {}` (every default);
"h98" sets `build.horizon: 98`, the Board routes' median win day.

| Route | Arm | Wins (95 % CI) | Median win day | Δ win day vs route | Planner vs same engines: McNemar, Δ last day | Counter goods bought, days 1–90 | Net worth day 90 |
|---|---|---|---|---|---|---|---|
| `farmer` | route | 60/60 (94–100 %) | 91 | — | — | 8 168 | 27 998 |
| | + caffeine weed line, lab, 2 fillerweed bays, power | 37/60 (49–73 %) | 217 | +126.2 ± 6.2 (McNemar 0:23, p < 0.001) | — | 3 496 | 13 859 |
| | + planner | **58/60 (89–99 %)** | 210 | +95.5 ± 13.4 | **23:2, p < 0.001**; −10.2 ± 17.0 | 4 274 | 21 424 |
| | + planner, h98 | **60/60 (94–100 %)** | 126 | +70.0 ± 13.6 | **23:0, p < 0.001**; −37.3 ± 15.3 | 4 543 | 22 791 |
| `sprinter` | route | 60/60 (94–100 %) | 98 | — | — | 8 890 | 27 628 |
| | + caffeine weed line, lab, 2 fillerweed bays, power | 43/60 (59–81 %) | 217 | +121.1 ± 6.8 (0:17, p < 0.001) | — | 4 035 | 13 701 |
| | + planner | 56/60 (84–97 %) | 217 | +97.4 ± 13.3 | **15:2, p = 0.002**; −19.0 ± 14.3 | 4 745 | 21 051 |
| `vintner` | route | 60/60 (94–100 %) | 98 | — | — | 8 864 | 28 187 |
| | + `clean_water` (gutter) + power | 60/60 (94–100 %) | 119 | +41.6 ± 11.6 | — | 6 316 | 25 027 |
| | + planner | 60/60 (94–100 %) | 119 | +41.3 ± 13.0 | 0:0; −0.3 ± 17.2 | 6 099 | 26 002 |
| `armourer` | route | 60/60 (94–100 %) | 105 | — | — | 8 120 | 26 108 |
| | + the frame forge | 60/60 (94–100 %) | 210 | +81.4 ± 13.4 | — | 4 696 | 20 790 |
| | + planner | 60/60 (94–100 %) | 210 | +91.5 ± 11.2 | 0:0; +10.0 ± 15.3 | 4 613 | 20 710 |

What the planner did (per run, finals):

| Arm | Kits approved / complete (of 60) and spend a run | Mornings a committed kit waited, by cause | Engine output a run (planner / engines alone) |
|---|---|---|---|
| farmer weed + planner | crop bays 60/48 (657), power 60/60 (387), lab 31/2 (509), storage 31/28, DNA 3/0 | lab: seller 50, cash 24; crop bays: seller 45; storage: seller 17 | pills 2 / 1 217; crops 2 662 / 1 942; unpowered bay-nights 13 / 32; lab unlocked in 30 / 60 runs |
| farmer weed + planner h98 | crop bays 60/60 (840), power 60/60 (387); lab and DNA declined (payback) | crop bays: seller 28 | crops 2 882; unpowered 2 |
| sprinter weed + planner | crop bays 60/46, power 60/60, lab 33/10, storage 33/31, DNA 7/0 | crop bays: seller 57; lab: seller 43, cash 18; DNA: seller 13 | pills 129 / 1 967; crops 2 323 / 1 459; unpowered 21 / 48 |
| vintner cw + planner | power 60/60 (386), water 60/58 (515) | water: seller 24 | wine 6 590 / 6 142; production −190 / −782 against `vintner` |
| armourer forge + planner | forge 60/47 (1 582), storage 55/55 | forge: seller 74 | forge orders' income 260 / 116 |

### Screens (16 seeds 9400001–016)

Screen 1 (build bo1 = main aef93ae + the planner's first cut; `arms-build.txt`), before power, the lab's supply
fixes and the gene lines were on main:

| Arm | Wins | Median win day | Δ win day vs route |
|---|---|---|---|
| `sprinter` + caffeine lab (beans from visitors) | 16/16 | 126 | +65.2 ± 25.3 |
| + planner | 16/16 | 112 | +45.9 ± 29.0 |
| `sprinter` + lab + 2 coffee bays | 16/16 | 210 | +95.8 ± 21.2 |
| + planner (`must: [lab, crop_bays]`) | 16/16 | 196 | +69.6 ± 22.6 |
| `vintner` + `clean_water` | 16/16 | 112 | +27.1 ± 22.6 |
| + planner | 16/16 | 112 | +22.8 ± 16.6 |

Screen 4 (build bo4 = main a4509e0: Power.cs, the lab's supply loop, Breeding.cs; `arms-build4.txt`) added the
`must` arms (the kit bought whole as soon as affordable, whatever its payback) and Board-free:

| Arm | Wins | Δ last day vs route | vs the same engines without the planner |
|---|---|---|---|
| `sprinter` + weed line + power, `must: [dna]` | 10/16 | +107.2 ± 16.6 | McNemar 0:6 against `sprinter` |
| `farmer` + weed line + power, `must: [dna]` | 9/16 | +121.2 ± 14.0 | 0:7 |
| `sprinter` + caffeine loop (coffee 3, power), `must: [lab, crop_bays]` | 15/16 | +114.2 ± 10.1 | — |
| `armourer` forge, `must: [forge]`, `insure_kit: 400` | 16/16 | +107.2 ± 7.4 | insurance kept on cost more than burglars took |
| Board-free `sprinter` | 0/16 (10 rent, 6 repo) | — | — |
| Board-free + weed line + power | 0/16 | −54.7 ± 17.5 | — |
| + planner | 0/16 | −38.7 ± 20.1 | +16 days |
| Board-free `farmer` | 0/16 | — | — |
| Board-free + weed line + power | 0/16 (6 arrests) | −64.9 ± 14.1 | — |
| + planner | 0/16 (3 arrests) | −52.3 ± 19.3 | +12.6 days |

Forcing the DNA kit whole (`must`) is the worst arm everywhere: the line's ~3 300 (PLANT_DNA 1 875 on hard, the
manipulator, disks) on top of the lab and the bays sinks the cash buyout and, in 6–7 of 16 runs, the shop.

### Paper against the gym, per engine

| Engine (kit) | Paper (income a day once complete, the planner's default) | Gym, realised | Δ win day on its route, engines alone → with the planner | What still blocks |
|---|---|---|---|---|
| Caffeine lab (RETIRED_CHEMIST + 4 machines, ~2 600) | 45 net (CHEMISTRY-ENGINE: 24 pharma pills a day at best) | pill sales 37.5–40.5 a day after the unlock with visitors' beans (screen 1); 8–12 a day in the weed stacks, where the lab's machines are missing on 61–80 mornings a run | +65 → +46 (screen 1, 16 seeds) | sellers (lab: seller 43–50 mornings), grid (the chemist's bundle lands on the front counter of a full grid) |
| Caffeine weed (PLANT_DNA + manipulator + disks, ~3 300, needs the lab and the bays) | 40 a weed bay (BREEDING §2: 67 a bay-night at caffeine III + potent I) | ~0: weed pills 0–129 a run; the planner commits the DNA kit in 1–7 of 60 runs and completes it in none; forced (`must`) it completes in 13–14 of 16 and loses 6–7 runs | +126 → +95 (farmer, 60 seeds); `must` worse than either | the whole line (lab + bays + DNA) is ~6 000 before a pill; the horizon |
| Crop bays (taps + bays) | 30 a bay | raw fillerweed sales 7–17 a bay-day (17.4 when the planner keeps the lab and DNA out) | part of the stacks above | sellers (crop bays: seller 28–57 mornings), grid |
| Clean water (taps, reed bay, tank) | 50 (QUALITY §8 paper: cellar −17 days) | production −190 ± 585 a run against `vintner` with the planner, −782 ± 543 without | +41.6 → +41.3 (vintner, 60 seeds) | bottles cap the cellar (QUALITY §8); counter buying −2 800 |
| Frame forge (printer, modules, chips, ~1 600) | 33 a printer (ARMS §6.1) | forge orders' income 116 (alone) / 260 (planner) a run, 0.4 / 0.9 orders | +81 → +92 (armourer, 60 seeds) | sellers (forge: seller 74 mornings), Permit II, few orders; counter buying −3 400 |
| Power (POWER_HIJACK + recharger, ~675) | 1.5 a unit of draw | bought whole in 60/60 runs; unpowered bay-nights 32 → 13, 48 → 21 | — (part of each stack) | — |

**The stock grid, measured.** Morning free cells (`build.free_cells`, a probe arm with `build: {kits: []}`, seeds
9400001–002): `sprinter` means 101 and 78, under 36 on 1 and 13 of 90 days; the weed stack 51 and 47, under 36 on 52
and 56 days, 0–8 from day ~60. Turning the liquidation threshold off (`min_free_cells: 0`, 16 seeds) changes nothing
(counter buying 3 990 → 3 697): the counter turns lots away because they do not fit, not because it is selling down.
Holding 30 days of counter purchases instead of 4 (`trade_days: 30`) moves the weed stack +109 → +84 days on 12
seeds (not significant). With the grid diagnosed, the storage bay became an urgent kit (bought first whenever the
grid runs under `min_free_cells`; bo7: **yes — storage gives the runs back.** On 16 seeds 9400001–016 (build bo7) the
weed stacks win 16/16 where they won 9/16 (`farmer_weed`, McNemar 0:7 against the route) and 13/16 (`weed_power`,
0:3) without it, always by buyout or mortgage; the storage kit completes in 8–13 of 16 runs and the counter keeps
buying (counter spend a run 14 711–18 885 against the stacks' 15 838–18 885, the route's 10 883). Median win day is
still ~+110 (210 vs 98): the engines pay back late, the grid no longer kills the runs. The 60-seed finals on the
storage build are still to run (see Next).)

## WIP / state at reboot (2026-11-12)

**Built** (branch `build-order`, merged with local main through the Gauntlet, a34b00b): `sim/gym/BuildOrder.cs`
(holds, Georgio freeze, whole-kit commitment, ranking, the buyout check, gates, one stow path, `insure_kit`, the
urgent storage bay), `sim/gym/BuildKits.cs` (kits: cellar bays, crop bays, clean water, lab, DNA/gene lines, forge,
power through Power.cs, storage), the hooks in Production.cs / PlayerRole.cs / ExecutorMorning.cs /
Episode.Player.cs / Breadth.cs (all inert with `build` absent; byte-identical to main checked on seed 9400003),
schema and PROTOCOL entries, the runner/report `sim/lab/scripts/buildorder.py`, directives in
`sim/lab/scripts/directives/build/` (generated by `make.py`), arms files `arms-build*.txt`, and the gym test
`BuildPlannerWaitsForGeorgioThenBuysWholeKit` (not yet run: the box was frozen for the live test). The fair-play guard
regex finds nothing in either planner file. `mod/src/Fence.Autopilot` compiles with them (C# 9).

**Measured** (results in `var/sim/exp/`, gitignored): screen 1 `bo1`, screen 4 `bo4`, the 60-seed finals `bof`, the
grid probe and `trade_days` screen `bogrid` (complete except a few `_t30` seeds) — all in the tables above.

**Complete overnight (2026-11-13).** `bog` finished (30 seeds 8100001–030, build bo6) and `bo7` ran (16 seeds
9400001–016, build bo7, report in `var/sim/exp/*.report.txt`). The planner rescues every stack it killed, and
`water_baron`'s stack now returns to its route:

| Route | Engines alone | + planner | h98 | McNemar (route → planner) |
|---|---|---|---|---|
| `appraiser` | g_appraiser_all 22/30, Δ +131.7 ± 9.3 | 30/30, Δ +124.8 ± 12.3 | 30/30, Δ +99.2 ± 17.6 | 0:8 p = .008 |
| `farmer` | g_farmer_all 13/30, Δ +127.1 ± 13.3 | 28/30, Δ +117.8 ± 12.4 | 30/30, Δ +110.6 ± 14.3 | 0:17 p < .001 → 0:2 p = .5 |
| `armourer` | g_armourer_all 30/30, Δ +116.9 ± 10.1 | 30/30, Δ +119.7 ± 9.2 | 30/30, Δ +101.7 ± 15.9 | 0:0 |
| `water_baron` | g_water_baron_all 30/30, Δ +95.2 ± 18.8 | 30/30, Δ +29.2 ± 13.9 | 30/30, Δ +10.3 ± 6.2 (median 105) | 0:0 |

The planner declines what does not pay back: on `water_baron` it approves no water kit (build arm's counter spend
back to 11 121 a run against the stack's 8 908; h98 indistinguishable from the route). The lab stays incomplete in
most runs (appraiser 2/30, farmer 1/30 complete): the seller wait (water:seller 88–133 mornings a run) is the next
binding constraint. With storage (bo7): the weed stacks win 16/16 where they won 9/16 (`farmer_weed`, McNemar
0:7) and 13/16 (`weed_power`, 0:3) without it, the storage kit completing in 8–13 of 16 runs; median win day stays
~+110 — the engines pay back late, but the grid no longer kills the runs.

**Finals on the storage build (60 seeds 2910001–060, bo7, `var/sim/exp/bof.report.txt`).** The 16-seed picture
holds: the build planner rescues the weed stacks at scale — `farmer_weed` 37/60 → `farmer_weed_build` 58/60
(McNemar 0:2 p = .5), `weed_power` 43/60 → 56/60 (0:4 p = .12) — and the h98 variant is the strongest build of
all: `farmer_weed_build_h98` wins 60/60, median win day 126 (the route's 91), skipping the lab entirely
(`lab_unlocked` 3/60) and completing crop bays + power in every run. The forge/vintner stacks pay back late
(median 210/119 with no win-day gain over their routes), but complete in ~all runs; the planner declines the dna
and lab kits when the seller wait is the constraint. Storage completes 55/60 (armourer) and 31/60 (farmer) —
the seller wait, not the cash, binds.

**Next.** The gym test `BuildPlannerWaitsForGeorgioThenBuysWholeKit`, `make test-go`, `make test-mod`, and a
PROGRESS.md entry.
