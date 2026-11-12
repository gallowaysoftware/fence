# Output quality: the best grade the shop's equipment allows

The owner's ask (2026-11-12): the game gives a vague guide, the hidden systems go much deeper, and the executor
should make the best grade its equipment allows in water, hydroponics, wine and the furnace, using only what a
player can see or measure. This file is the audit (the best output per equipment level), the gap between that and
what the executor did, the fixes, the information boundary, the clean-water supply the shop can build, and the
measurements on hard. Mechanics are from the decompile and the lab (docs/re/WATER.md, PRODUCTION.md, WINE.md,
PLANTS.md, MODULES.md; docs/sim/VERIFIED.md). Code: `sim/gym/Hydro.cs` (the `quality` lever), `sim/gym/Quality.cs`
(the summary's grade counters), `sim/lab/scripts/quality.py` (the report).

## Bottom line

- **Grades went up; buyout days did not move.** Hard, 60 fresh seeds 9700001–060, every route paired with main as
  of 96e44d4. All 14 arms won 60/60. No route's median win day moved significantly (Δ −3.7 … +3.3 days, every CI
  across 0).
- **Wine.** Minerals in band went from 3–8 % of bay-nights to 60–70 %. Bad berries fell 4–9 points and best berries
  rose 6–8 points. The mean bottle score rose 0.3–0.6, and the share of Master Vintage sold went from 17–20 % to
  25–36 %. Wine income did not change (Δ −89 … +348, all within noise), because the cellar is bound by water: the
  bays run dry for weeks (§2), so there are no more berries to use.
- **Water.** The stale-label fix holds on `water_baron`:
  - production +1 020 ± 366 a run;
  - water sales +1 173 ± 683;
  - litres priced as Rustwater against true Rustwater: 24 % → 12 %, against 11 % → 10 % true.
  - On `fountain` the counter gains +1 853 ± 621, but the fountain loses 1 260 ± 507. It now fills its slots by the
    label, not by the hidden truth. Net +65 ± 374.
- **The staged clean-water build (§8, 2026-11-12)** does not reach the targets either: `vintner` +14.2 ± 4.2 days
  (60 seeds), `fountain` +4.2 ± 10.4 (fountain sales +2 456 a run), 0/16 without the Board. Bottles (~1.2 offered
  a day), Georgio's day-43 loan and the counter's cash are the walls, not water.
- **Measured against the verdict audit's targets:**
  - cellar −17.0 ± 2.2 days;
  - fountain −12.4 ± 1.6 days.
  - **This branch does not reach either.** Both targets need clean water at volume early in the run. That is the
    staged build of §5, and the executor does not have it yet: the reeds as built here start curing around day 81
    and cost days (§5).

## 1. The best output per equipment level

### Water (grades Pure ≥ 99.90 / HQ ≥ 99.00 / Base ≥ 98.00 / Ghost ≥ 96.00 / Rust ≥ 90.00 / Gutter)

| Equipment | Best grade | Per day | Plant-safe? (every contaminant ≤ 10 bp) | Notes |
|---|---|---|---|---|
| Bought bottles, rations | as sold (Base mostly; HQ rare) | seller-bound | no (Base 4 %/point on a bloomberry, HQ ~0.9 %) | labels can lie: ghost as base, base as HQ, ghost as pure |
| Faucet, `GUTTERFLOW_TAP` / `RUSTWATER_TAP` | Gutter 80–90 % / Rust 90–91.5 % | unlimited, free | no | label is the truth on a fill |
| + filter pitcher + 1 tablet a litre | Base ~98.3–98.7 % from rust, HQ ~99.1 % from base | labour-bound; a filter ~10.8 L of rust water | no: the tablet stops at 70 bp chemical and the filter misses heavy metal (~4 %/point) | label stale (the raw grade) until tested |
| Portable purifier (water merchant start) | exactly Base, label updated | 30 L for life | no | never recharges |
| Water purifier, bare (Jackson once `WATER_TRADER`) | HQ 99.01 % (cap) | one container a night: 3 L Base → HQ in a night, 10 L of filtered water in ~1–2 | no: leaves ~55 bp chemical, ~28 heavy metal (2.75 %/point) | label stale; its tooltip shows last night's removal and the cap |
| Purifier + 4 fineness (quality 100) | **Pure** 99.90 % | 3 L HQ → Pure a night | yes (~10 bp chemical left) | fineness 16 % a Jackson visit each (150 on hard): in a hard run the shop sees ~1–2 by the buyout |
| Moisture farm (Jackson, 525 on hard) | Ghost bare (850 ml/night); Base with 2 fineness (714); HQ with 4 (578); Pure with 6 and a bay kit (~442) | ≤ 0.85 L | Pure only | farms share output (÷ 1 + 0.25 per extra farm) |
| Evaporator (`RETIRED_CHEMIST`, 1 500 on hard) | **Pure** at any quality > 0 (100 % water, no minerals) | 69 / 126 / 204 ml a night with 1 / 2 / 4 fineness; 0 bare | yes (add tablets: no minerals at all) | label stale (an empty receiver reads Gutterflow) |
| **Hydroreed bay** (a bay, a 20-credit seed, 8 power a night) | contaminants ≤ 4–6 bp, minerals kept | a vessel per ~11–13 nights from raw tap water, any volume: 6 L jug ≈ 0.5 L/night, 20 L tank ≈ 1.6 L/night | **yes**, with the tap's own minerals (gutter 170–375 bp, rust ~150: in a bloomberry's or coffee plant's band) | label stays the raw grade (minerals count against purity: ~97–98.5 %). Through the filter afterwards: HQ/Pure edge |

The furnace and wine water read **true** purity; selling, the fountain and customers read the **label** until a
strip or AquaScan Match (WATER.md §4). Plants read individual contaminants, not the grade.

### Hydroponics (bloomberry; PLANTS.md §2)

| Water in the bay | Health a point | Refruit health | Bad berries | Wine points a berry |
|---|---|---|---|---|
| Ghost / rust | ≥ 7.75 % | < 0 | 100 % | ≈ −0.5 |
| Base (bought, or tap + filter + tablet) | ~4 % | 36–44 | ~70–75 % | ≈ 0.05–1 |
| HQ bought / purifier HQ | 0.94 % / 2.75 % | 70 / 56 | 34 % / ~50 % | ≈ 1.05 |
| any, minerals out of band (no tablets) | +1 %/point below, +0.5 % above | | | |
| HQ + tablets in band | 0.94 % | ~85 | ~22 % | |
| **Pure or reed-cured + minerals in band** | 0 | 100 | **0 %** (62 % best) | ≈ 1.6 |

Bay quality modules add berries (6 → 7 at Q ≥ 18, 8 at 50); performance 100 makes a harvest every night
(MODULES.md §3). A bloomberry bay locks its module grid for good once planted.

### Wine (WINE.md)

Score = Σ berry points + 1 for two or more berries + 1 for a yeast + water term (Pure/HQ +1, Base 0, Ghost −1,
Rust −2, Gutter −3, from the true purity of everything poured in); tiers ≥ 6 Master (175) / 5 Reserve (150) / 3–4
Production (125) / 2 Homebrew (100) / 1 Bucket (85) / ≤ 0 undrinkable. The best grade the equipment allows is
set by the berries (the bays' water) and the bottle's water (HQ for +1); aging adds +3 a step, a rack doubles the
steps.

### Furnace (PRODUCTION.md §3)

Level = Low + ⌊2 × ore / inputs⌋ + flux level. Very High (75) is the ceiling with ore and a flux (blast: 3 ore → 2
Very High). Perfect (150) needs quality ≥ 200 unblasted or ≥ 300 blasted: neural cores (the uncapped one is the
Hypertech smuggler's, 1 500 + a 2 500 network upgrade, contraband) — not reachable in a hard run. The smelt takes
ore before scrap, so ore already goes in together; the one loss is a lone ore beside two scrap on a blast night (0
levels for it), worth holding back — at the gym's ore rates a few credits a run, not implemented. Ingot labels are
the truth from the smelt (no label gap).

## 2. What the executor did, and the gap

Measured with the summary's new `production.quality` (seeds 9400001–030, hard, routes as shipped; FIN rows from
the final run):

Before (main) on the final seeds:

| Route | Bad berries | Best | Minerals low / in / high | Bay-nights ≥ 4 %/point | Bottle water HQ / Base | Mean score | Master sold | Water priced vs true (Rust) |
|---|---|---|---|---|---|---|---|---|
| `appraiser` | 49 % | 20 % | 97 / 3 / 1 | 32 % | 15 / 83 % | 3.16 | 17 % | — |
| `farmer` | 54 % | 18 % | 86 / 8 / 6 | 44 % | 16 / 82 % | 3.30 | 20 % | — |
| `vintner` | 49 % | 19 % | 97 / 3 / 0 | 23 % | 11 / 86 % | 3.27 | 20 % | — |
| `rancher` | 49 % | 20 % | 97 / 3 / 0 | 25 % | 14 / 84 % | 3.18 | 17 % | — |
| `water_baron` | | | | | | | | 24 % priced, 11 % true |
| `fountain` | | | | | | | | 35 % priced, 16 % true |

Ingots: 39–49 % Very High, 39–49 % Fair (scrap with flux), no Perfect. That is already the ceiling for the inputs.

The gaps, in order of size:

1. **Minerals.** The bays dosed one nutrient tablet per litre *added*, but bought one tablet at a time (the
   farmer offers three), so 88–99 % of bay-nights ran below the band (−1 % health a point, −16 a refruit).
2. **Stale labels on sold water.** Treated and purified water whose test found no strip lost its pending test
   and sold at the raw water's label: on `water_baron` ~29 % of the litres sold were priced as Rustwater against
   ~11 % true, ~2 k of value a run.
3. **Bay water.** 17–51 % of bay-nights cost ≥ 4 % health a point (ghost or base water topped up into the
   bays); no route makes plant-safe water.
4. **Bottle water.** 84–92 % of bottles started on Basewater (rations); HQ when bought by chance.
5. **Hidden reads.** The engines chose water by its true purity (`WaterHelper.GetWaterPurity`) and planned wine
   scores with the hidden water term (`WineHelper.GetWaterQualityScore`): information a player does not have.

## 3. The fixes (`quality` directive, `sim/gym/Hydro.cs`)

| Lever | Default | What it does |
|---|---|---|
| `quality.bays` | true | The bay takes the best label grade owned (ghost only when nothing better), never tops a better grade up with a worse one, and keeps five nights of the bays' drinking back from the bottles. Nutrient tablets bring the water to the plant's level (never past the band's top when one fewer stays in it), from a ledger: the tablets dosed, the water poured and the plant's use rate × bay speed a night; with an AquaScan its reading. Every tablet offered is bought while short of six a bay. |
| `quality.bottles` | true | A bottle is filled from tested HQ or Pure water before rations (+1); the wine score is planned with the poured water's known grade. |
| `quality.reeds` | — | Superseded by `clean_water` (§8, `sim/gym/Cellar.cs`). `Production.PlantSafeWater()` still lists the cured vessels for any engine. |
| (always) | — | Treated or purified water waits up to a week, unsold, for its strip (more strips are bought while tests wait); the purifier's done-check is its tooltip; every water choice reads the label or a test. |

Not built: the purifier's quality upgrade to Pure (four fineness from Jackson's random stock arrive too late to
pay; a `jackson_equipment` route could add it), the evaporator (1 500 on hard for ~0.2 L a night), the furnace's
leftover-ore hold, the wine-bottle purifier trick (a filled bottle in the purifier lifts it to HQ overnight:
needs a purifier the wine routes rarely own).

## 4. What the executor may read

| Read | Visible to a player? | Used for |
|---|---|---|
| A container's label grade (`customIntValue1` banded) | yes (tooltip) | every water choice |
| A test's reading (strip or AquaScan; exact) and Match | yes, at a strip's cost (AquaScan free) | relabelling; known grade from then |
| The rules: tap and premade mixes, tablet/filter/purifier/reed effects, plant bands and use rates | yes (knowledgeable player) | reed nights, mineral ledger, purifier done-check |
| Purifier tooltip: last night's removal, the cap | yes | purifier done-check |
| Exact ml of a container | band only unless graduated or the Experienced Chemist perk; the executor poured it | volumes (a boundary kept: the executor knows what it poured) |
| Per-liquid composition (minerals, chemical) | AquaScan only | the mineral ledger is replaced by the reading when one is owned; the reed's done-check likewise |
| True purity, the wine score, plant health | no (tests, dev tooltip, plant analyser) | only the summary's `production.quality` counters (measurement, never a decision) |

Still reading truth (documented, not in these engines): `PlayerRole`'s counter check that holds over-labelled
water off the shelf (`water_label`) compares the label with the true grade; a player would have to test every
bought bottle.

## 5. Clean water at scale

Litres a day of plant-safe water per path, its cost on hard and how early it can start (bloomberry and coffee
bays drink 100 ml a night; kotton 265):

| Path | Plant-safe L/day | Bays fed (100 ml) | Capital on hard | Running | Earliest |
|---|---|---|---|---|---|
| Hydroreed + 6 L jug, gutter tap | ~0.45 | 4 | `WATER_TRADER` 450 + `GUTTERFLOW_TAP` 375 + a bay (~390) + seed 20 | 8 power a night | tap + a reed seed (Retired Farmer) + ~13 nights; first cure ~day 45–50 in the gym |
| Hydroreed + 20 L tank | ~1.5 | 15 | + tank 525 (Jackson, 50 % a visit with `WATER_TRADER`) | same | same |
| Purifier + 4 fineness, fed filtered tap water | ~3 | 30 | purifier 375 + 4 × 150 fineness | filter, tablets, 4–6 power | when four fineness have turned up (rarely before the buyout) |
| Evaporator + 4 fineness | ~0.2 (no minerals) | 2 | 1 500 + 600 | 6+ power | `RETIRED_CHEMIST` |
| Moisture farm + 6 fineness + bay kit | ~0.44 | 4 | 525 + 900 + 150 | 6+ power | rarely |

The cheapest path to enough clean water for N bays is the tap plus ⌈N / 4⌉ hydroreed bays with jugs (or one with
a 20 L tank): cured gutter water also carries minerals inside the bloomberry and coffee bands, so the bays need no
tablets. It is the only path a hard run can reach early, and it is open to the chemistry engine's coffee bays
through `Production.PlantSafeWater()` (the cured vessels, largest first; tested when cured, so the label is the
truth; raw tap water keeps the tap's minerals, filtered water needs tablets) with `clean_water` set (§8).

**What the gym measured for the reeds** (hard, paired against the route as shipped, 30 seeds 9400001–030 and
9600001–030; `quality.reeds`):

| Arm | Win day Δ | Bay-nights at 0 % health a point | Cures a run (first, median) | Bottles a run |
|---|---|---|---|---|
| `farmer` + 1 reed (raw tap, before the permit gate) | +47.1 ± 15.1 | 25 % (from 0) | 3.7 | 45 (56) |
| `vintner` + 1 reed (same) | +29.2 ± 13.8 | 4 % | 1.4 | 45 (60) |
| `vintner` + 2 reeds (filtered tap, after the permit, 20 L tank) | median 104 against 97 | 4 % | 1.4 (day 81; 5 of 30 never) | 51 (61) |
| `vintner` + 2 reeds + `bays: 5` + `wine_score: 6` | median 118 against 97 | 2 % | | 48 |
| `farmer` + 2 reeds + `bays: 5` + `wine_score: 6` | +112.7 ± 13.8 | 7 % | | 34 (56) |

The reed water works where it arrives (a quarter of the farmer's bay-nights at zero contaminant cost), but the
executor reaches it too late and too slowly: the first cure is around day 81, the taps, bay, tank and filters are
bought while the cellar's own bottles and bays are short, and the bottles fall by a fifth. The cause of the
farmer arm's collapse was not pinned down. `reeds` stays off; the staged build the verdict audit describes
(taps right after the permit, one reed from day ~20, the bays fed from it before the cellar grows) is the next
executor, not this one.

## 6. Measurements (hard, paired, fresh seeds 9500001–060)

`sim/lab/scripts/quality.py` on `arms.py` runs. Base = main (96e44d4) with the measurement-only counters; new = this
branch with the defaults (`quality.bays` and `bottles` on, `reeds` 0). Hard, routes as shipped, seeds 9700001–060.

| Route | Wins base / new (Wilson) | Median win day | Δ win day (paired) | McNemar | Δ production a run | Grades, base → new |
|---|---|---|---|---|---|---|
| `appraiser` | 60 / 60 (94–100 %) | 97 → 97 | −0.5 ± 3.8 | p = 1 | +335 ± 464 | bad 49 → 40 %, best 20 → 27 %, score 3.16 → 3.58, Master 17 → 28 % |
| `farmer` | 60 / 60 | 97 → 90 | +0.0 ± 4.6 | p = 1 | −237 ± 461 | bad 54 → 46 %, best 18 → 26 %, score 3.30 → 3.87, Master 20 → 36 % |
| `vintner` | 60 / 60 | 97 → 97 | −2.1 ± 6.0 | p = 1 | +111 ± 523 | bad 49 → 43 %, best 19 → 25 %, score 3.27 → 3.66, Master 20 → 29 % |
| `rancher` | 60 / 60 | 83 → 90 | +3.1 ± 3.3 | p = 1 | −170 ± 395 | bad 49 → 45 %, best 20 → 24 %, score 3.18 → 3.45, Master 17 → 25 % |
| `water_baron` | 60 / 60 | 104 → 104 | −3.7 ± 6.5 | p = 1 | **+1 020 ± 366** | Rust-priced 24 → 12 % (true 11 → 10 %) |
| `fountain` | 60 / 60 | 111 → 111 | +3.3 ± 6.3 | p = 1 | +65 ± 374 | counter water +1 853 ± 621; fountain −1 260 ± 507 |
| `sprint` (`hard_income_cash_sprint`) | 60 / 60 | 97 → 97 | −0.5 ± 3.8 | p = 1 | +335 ± 464 | as `appraiser` (same seeds, same play) |

Δ is the mean over seeds both arms won, ± 1.96 SE. Production = wine + water + fountain + ingots − engine inputs
(`income.py`). Wins cannot move on these routes: every arm wins every seed.

**Levers measured on 30 seeds and left off:**
- `wine_score: 6` on `vintner`: +1.9 ± 13.6 days, 47 bottles against 61. Bottles wait for berries that do not
  come.
- `reeds` (§5).

## 7. What is left

1. **The staged clean-water build** of PRODUCTION-VERDICTS.md:
   - taps right after the permit;
   - one hydroreed bay on a 20 L tank of filtered tap water from day ~20;
   - the bays and bottles fed from it before the cellar grows;
   - then the fountain at attractiveness 100, with commons to the fountain before vending, and an HQ jug in the
     case for the stockpiler.

   This is what the −17 / −12 day targets need. The pieces exist here: the reed tank, `PlantSafeWater()`, HQ
   testing, the permit gate. What is missing is the schedule and the cash plan. The farmer arm's collapse with
   reeds is not diagnosed.
2. **Supply for the cellar.** Bays dry for weeks is the cellar's real limit. Tap water through a filter and a
   tablet (Base) keeps the plants alive, and the executor runs that pipeline only when `tap` is set.
3. **The fountain by label.** Testing every bought container before it reaches a slot would win back the 1 260 a run
   it lost without reading the truth.
4. **The wine-bottle purifier trick, the furnace's leftover ore, `wine_dna_trait: purifying`** (built, not
   measured).
5. **PlayerRole's `water_label` check** still reads the hidden truth (§4).
6. **The mod.** The engines are shared executor files (`mod/src/Fence.Autopilot/SharedExecutor.props` now
   includes `Quality.cs` and `Hydro.cs`), so the live autopilot runs them. `make mod` and `make test-mod` are green:
   - nine new game members in `compat/hooks.json`;
   - `IngotPurityHelper` in `RuntimeResolver`.

   Not tried in the live game: the faucet fill of a vessel inside a reed's slot (a player drags it out to the sink),
   and the pitcher fill at the faucet. The owner's test is docs/STREAM-OPS.md section 12.4. Live, the fills target
   the shop's own faucet (`Chore.Faucet`); a made faucet would be a stray item in the scene.

## 8. The staged clean-water build (2026-11-12)

The build of §7 item 1, as `clean_water` (`sim/gym/Cellar.cs`, off by default; it replaces `quality.reeds`).
Board routes: it raises the cellar's grades and does not shorten the buyout; the targets (cellar −17.0 ± 2.2
days, fountain −12.4 ± 1.6) are not reached, and the reasons are below.

### What the build does

| Step | When | What a player does |
|---|---|---|
| Taps | from day 44, with `reserve` (1 000) over the rent, and the 2 000 permit fee kept until it is bought | WATER_TRADER, GUTTERFLOW_TAP (and RUSTWATER_TAP with `tap: "rust"`) |
| Reed bay | the next bay once the taps are on and `reed_after` (1) bloomberry bay grows | plants a hydroreed seed (the Retired Farmer sells two every visit) and keeps that bay a reed for good: the reed is harvested for its seeds every few nights and replanted, its vessel stays |
| Vessel | at once | the biggest empty jug or tank (a 20 L tank from the Retired Water Merchant, who comes every week with WATER_TRADER; an empty bought bottle until then) |
| Fill | when the vessel is under 200 ml | at the faucet through the pitcher's filter (new filters as one runs out), never topped up raw; raw only without a filter |
| Cure | the rule's nights, counted only on nights the bay had power | worst contaminant = 30 % of the tap's impurity, −29 % a night (lab `verdict.reedcure`): filtered or raw rust 10 nights, gutter 12; then a strip puts the true grade on the label |
| Decant | once cured | into loose jugs and tanks (`store_l`, 24 L); a water route pours into the bottles and jugs it sells (`cure: "hq"`: at the strip that reads High Quality, ~night 7) |
| Use | every morning | the bays drink the stores first (tablets make up the minerals the filter took), the bottles take tested HQ, the fountain takes tested containers |

Also: the kit (jugs, tanks, pitcher, filters, strips, reed seeds) is bought only from the engine's own suppliers
(Retired Farmer, Water Merchant, Winemaker, Chemist, the water seller, Dr. Jackson): a general seller's lot that
holds a wanted input is bought for the inputs alone (`PlayerRole.StageFromSeller`), which cost the counter its
stock. The kit is stowed off the back counter (left there it goes with the next buyer's sale; that is how the
pitchers kept disappearing). `tap: "none"` buys no tap and cures bought water for the bays alone. `power: true`
buys a recharger at Dr. Jackson's and POWER_HIJACK and rotates the cells (the power service now goes off under
the hijack; with a recharger it used to stay on, ~40 a night). A water route shows tested HQ water worth ≥ 350 in
the case for the stockpiler (`showcase`).

### Why QUALITY's reeds cured only around day 81, and what changed

- The taps waited for the cellar's own bays to be planted and the reed bay for **all** the bloomberry bays: the
  reed was planted around day 62. Now the reed bay is the next bay after the taps.
- The reed's vessel was filled raw (no pitcher yet: the pitcher was bought, sat on the back counter and went with
  the next sale), so the batch needed 13–14 nights. Now filtered, 10 nights; HQ by night 6–7.
- A vessel the bays had drained to a few ml stayed in the slot uncured for a week (the refill waited for it to be
  empty); a vessel the build did not put there (the bay's old water) was never replaced; when the reed was
  harvested, its bay went back to bloomberries and the next reed started elsewhere with nothing.
- A faucet fill is 10 L: the 20 L tank was half full.
- The audit's "20 L of filtered tap water → HQ in 4 nights" was measured on premade Rustwater. Faucet water carries
  more chemical (the slowest contaminant to go): `verdict.reedcure` on the faucet's own mix (purity / worst
  contaminant / minerals, bp):

| 20 L | Night 0 | 4 | 6 | 10 | 14 | HQ | Plant-safe | Filter durability |
|---|---|---|---|---|---|---|---|---|
| raw gutter 85 % | 8500 / 529 / 264 | 9438 / 126 | 9602 / 63 | 9711 / 23 | 9759 / 5 / 229 | never (minerals) | night 12 | 0 |
| filtered gutter | 8950 / 529 / 79 | 9670 / 129 | 9796 / 65 | 9880 / 24 | 9914 / 5 / 70 | night 11 | night 12 | 923 |
| raw rust 90.75 % | 9075 / 305 / 152 | 9653 / 76 | 9754 / 38 | 9831 / 10 | 9847 / 5 / 139 | never (minerals) | night 10 | 0 |
| filtered rust | 9434 / 305 / 23 | 9834 / 77 | 9904 / 39 | 9956 / 10 | 9967 / 5 / 20 | **night 6** | **night 10** | 707 |

  A 20 L filtered batch takes ~1.8 filters (rust) or ~2.3 (gutter). Raw tap water keeps its minerals inside the
  bloomberry's band but never reaches HQ (minerals count against purity), so it serves the bays, not the bottles.
  Pure (99.90 %) is not reached in 14 nights: the reed's floors stop it at ~99.67 %. The evaporator's distillate is
  Pure at any quality > 0 (`MachineEvaporator`: each evaporated liquid lands in the distillate × quality %, water
  without contaminants; the label is not updated), but 69–204 ml a night with 1–4 fineness for the Retired
  Chemist's 1 500 on hard, and the chemistry engine wants the same machine with the solvent-recovery module for
  alcohol: ~0.2 L of Pure against the reed's 2–3 L of HQ a day, and the reed costs a bay.

### Measured: screens (hard, 16 seeds 9400001–016, paired against the route on main)

| Arm | Wins | Median win day | Δ win day | What changed in the cellar |
|---|---|---|---|---|
| `vintner` (main) | 16/16 | 90 | — | bottles in 62, sold 50 at 127; bay-nights dry 137; 0 % of bay-nights at zero contaminant cost |
| + `clean_water` from day 20 (first build) | 12/12 | 111 | +23.3 ± 8.5 | Georgio's day-43 loan fell from 1 655 to 913: every credit spent before day 43 costs three |
| + `clean_water` (rust, day 44) | 15/16 | 104 | +12.1 ± 10.5 | 30 % of bay-nights at zero cost, best berries 21 → 24 %, bottles 54 → 66, Master 27 → 32 % of sales; first cure day 59 |
| + `clean_water` gutter | 16/16 | 104 | +11.4 ± 10.0 | first cure day 61 |
| + `clean_water` + power kit | 13/16 | 104 | +9.7 ± 12.7 | unpowered bay-nights 87 → 102: 4 cells in a recharger make 12 a night against five bays' 40 |
| + `clean_water` tapless | 16/16 | 104 | +12.7 ± 8.9 | bought water bought in lots that held other goods |
| + `wine_score: 6` alone | 16/16 | 104 | +7.4 ± 8.8 | bottles wait for berries (as QUALITY §6 found) |
| `appraiser` + `clean_water` | 16/16 | 111 | +12.2 ± 9.7 | Master 40 % of sales, bottles 74 |
| `appraiser` + tapless | 15/16 | 104 | +6.1 ± 6.0 | |
| `farmer` + `clean_water` | 14/16 | 104 | +15.5 ± 10.5 | Master 45 % of sales; two runs not bought out by day 200 |
| `farmer` + tapless | 15/16 | 104 | +15.4 ± 9.0 | |
| `water_baron` + `clean_water` | 15/16 | 111 | −1.9 ± 10.8 | production +132 ± 868 a run |
| `fountain` (main) | 14/16 | 111 | — | fountain 4 208 a run: a spent vessel the full stock grid refuses stays in its slot and the slot sells nothing again (fixed below) |
| `fountain`, the spent-vessel fix and commons held for it | 14/16 | 118 | +5.2 ± 23.3 | fountain +897 by day 90, vending −1 809, counter water −2 486 |
| `fountain` + `clean_water` | 15/16 | 118 | +12.4 ± 25.8 | |

Where the days go, `vintner` against `vintner` + `clean_water` (paired cash flows, days 44–80): the taps and kit
+759, inputs +485, the counter's purchases −1 450 and sales −1 696 (less cash to buy with), wine sales −34. The
cured water arrives from day ~59; by day 80 it has not yet turned into wine income.

**The fridge does not reach the water engine's goods.** `MachineFridge` chills (+20 % value) only `BREVAGE`,
`WATER_RATION` and `bottled_water_premium`, and refuses wine bottles. A container the water engine fills turns
`WATER` (`WaterHelper.UpdateContainerGoodType`), and the rations go into the wine, so only bought premium bottles
could be chilled for resale: not measured.

### Measured: the routes, fresh seeds 9800001–, paired against main

`sim/lab/scripts/arms-cellar.txt`; base arms on a main build (e07d205), the rest on this branch. The box was shared
by seven agents and then capped at two or three gym processes, so the planned 60 seeds a cell ran where it could:
60 for `vintner` and the main arms with the Board, 30 for the fountain arms, 16 for the rest and for every
Board-free cell (hard, 200 days, `quality.py`: Wilson 95 %, McNemar exact, Δ = paired mean over seeds both won,
± 1.96 SE). `vintner` and `farmer` run `clean_water: {"tap": "gutter"}`, the water routes `clean_water: {}`.

| Route | Board | Seeds | Main: wins, median day | `clean_water`: wins, median day | Δ win day | McNemar | Production Δ a run |
|---|---|---|---|---|---|---|---|
| `vintner` | yes | 60 | 60/60 (94–100 %), 97 | 56/60 (84–97 %), 111 | +14.2 ± 4.2 | p = 0.12 | −942 ± 502 (inputs +1 426) |
| `farmer` | yes | 16 | 60/60, 90 | 10/16 (39–82 %), 118 | +24.5 ± 25.8 (n = 10) | p = 0.03 | −1 939 ± 1 844 |
| `water_baron` | yes | 16 | 60/60, 104 | 15/16 (72–99 %), 118 | +12.1 ± 10.2 | p = 1 | −1 467 ± 1 275 |
| `fountain`, fixes only | yes | 30 | 58/60 (89–99 %), 111 | 27/30 (74–97 %), 111 | +2.4 ± 6.2 | p = 0.62 | −592 ± 485 (fountain +1 547, counter water −1 507) |
| `fountain` | yes | 30 | 58/60, 111 | 26/30 (70–95 %), 111 | +4.2 ± 10.4 | p = 0.38 | +560 ± 998 (fountain +2 456 ± 667) |
| `water_night_shift` | yes | 16 | 11/16 (44–86 %), 111 | 10/16 (39–82 %), 104 | −11.7 ± 58.8 (n = 6) | p = 1 | −636 ± 1 341 (fountain +3 155) |
| `vintner` | no | 16 | 0/16 (0–19 %); 5 alive, 6 rent, 5 repo | 0/16; 16 rent | last day −24 ± 15 | — | NW d200 −1 559 ± 1 213 |
| `fountain` | no | 16 | 0/16; 13 rent, 3 arrests | 0/16; 6 rent, 10 arrests | last day −20 ± 15 | — | fountain 2 471 against 2 936 |

On `vintner` (60 seeds) the cellar's grades did not improve either: bad berries 41 → 53 %, best 27 → 20 %, Master
33 → 31 % of sales, bottles 57 → 56 a run. A watered bay draws 8 power a night, and the cells ran out (89 unpowered
bay-nights a run against 14). The reeds cured 2.3 batches a run, the first on day 63. No arm beats its route, so
no native route was added and `clean_water` stays off.

The Pure path (`pure`) never ran in the 30 fountain seeds: it waits for the UV filter, which only Dr. Jackson
stocks (one visit in five once WATER_TRADER is unlocked, `StoreClientList.PlaceInventorInventory`), and the
measured build asked for it only from sellers at the counter. His shop now buys it too (`Wine.cs`
`JacksonWineWants`); not re-measured. The fountain's extra takings with `clean_water` are HQ and Base water from
the reeds.

Board-free, the build makes things worse: the fountain shop is arrested in 10 of 16 runs against 3 (9 of them for
operating without a licence: the build's spending leaves less cash when the permit official calls; not traced run
by run), and the cellar falls to the rent three weeks sooner.

### Paper target against the gym, and why

| | Paper (PRODUCTION-VERDICTS) | Gym, this build |
|---|---|---|
| Cellar | −17.0 ± 2.2 days (3 Master bottles a day, 4 000 kit paid when affordable) | +14.2 ± 4.2 days (`vintner`) |
| Fountain | −12.4 ± 1.6 days (431 a night at attractiveness 100, 4 750 kit) | +4.2 ± 10.4 days; +2 456 fountain sales a run |
| Board-free | 7/16 (cellar), 8/16 (landlord's fountain) | 0/16 both |

1. **Bottles, not water, cap the cellar.** Sellers offer ~1.27 wine bottles a day (demand trace, 6 `vintner` runs:
   106–133 offered in 77–105 days, about half bought; the Retired Winemaker's four a week are most of them). The
   probe gave 3 a day free. At ~1.2 a day the most the cellar can gain is ~+45 a bottle (Master against the mix
   it sells) ≈ 55 a day, about what the kit costs over the ~40 days before the buyout.
2. **Georgio's day-43 loan.** He borrows half the cash and repays four times it, so any credit spent before day 43
   costs three. The build started on day 20 lost 23.3 ± 8.5 days; from day 44 the cured water arrives around day
   60, a month before the buyout.
3. **The counter's cash.** The shop is cash-bound until the buyout; the taps and kit (~2 500) leave the counter
   ~1 450 short of stock to buy over days 44–80 and it sells ~1 700 less.
4. **Power.** Dry bays draw no power; watered ones do, and cells come at ~0.45 a day. The recharger and hijack
   (+1 000) charge 3 a cell a night: four cells make 12 against five bays' 40.
5. **The fountain at low attractiveness pays less than the counter for the same water**: filling its slots took
   1 507 of counter water sales for 1 547 more at the fountain. 17 commons for attractiveness 100 compete with the
   vending machine (`fountain_commons`, off: holding them for the fountain cost vending 1 809 by day 90 for 897).
6. **Faucet water is slower than premade water in the reed** (chemical 30 % of its impurity): HQ by night 6, not 4.

### What is left

- The cellar's ceiling is the bottle supply: a measured route would buy every bottle offered and put three good
  berries in each without waiting (`wine_berries: 3`, on with `clean_water`); not measured on its own.
- Power before water: the bays' cells bind as soon as they drink.
- The fountain: fill it only from attractiveness ~60, where its litre beats the counter's; measure `pure` now that
  Dr. Jackson's shop buys the UV filter.
- FORMULA-EXPLOITS.md items not built: the portable purifier on 2 L / 6 L vessels then decanted, the purifier on
  ≤ 1.7 L of raw rust (HQ in a night), wine bottles in the purifier's slot, 400 ml Pure + 349 ml Base as HQ wine
  water. Built: two tablets a jug (Water.cs), the Pure path (above).
- Live-only chores the mod has not tried: the faucet fill of a vessel sitting in a reed's slot (a player drags it
  to the sink and back), the two-pass pitcher fill, the UV filter used on a tank, scrap metal used on a tank.
