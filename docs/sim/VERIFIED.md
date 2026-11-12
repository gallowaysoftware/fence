# Mechanics verified against the real game, headless

Build 26525552 / 0.46D. Every row below was measured by running the installed game's own
`Assembly-CSharp` headless (`sim/headless` + `sim/gym`) under controlled conditions with the
experiment harness `sim/lab` (`fence-lab`). Nothing here is read from the decompile; the decompile
was used only to choose what to measure. Raw results: `var/lab/results.jsonl` (one JSON line per
claim, not committed).

Verdicts: **CONFIRMED** (the measured value matches the doc; for rates, the doc's value lies in the
99 % Wilson interval), **CONTRADICTED** (measured value given), **REFINED** (the doc's rule holds but
it is incomplete or its number is off in a way that matters), **CAN'T TEST HEADLESS** (why given).
`n` is the number of trials (or nights, or openings).

```
make gym-build && dotnet build sim/lab -c Release
dotnet sim/lab/bin/Release/net10.0/fence-lab.dll list
dotnet sim/lab/bin/Release/net10.0/fence-lab.dll barter.rolls -n 20000
dotnet sim/lab/bin/Release/net10.0/fence-lab.dll all -n 20000     # 32 experiments, ~15 s on 16 cores
```

**Re-run on 0c8d9fd** (the gym fidelity pass: real footprints, English text, per-second play clock, both
fixes below merged): all 32 experiments, 456 claims; no verdict changed (368 confirmed, 10 contradicted,
78 info). Real footprints remove one case and add two findings: three berries and a yeast no longer
fit one bottle (the `[2,2,2]` tier-5 case is gone; tier 5 is still reached by two best berries on
pure water), and c4 (9 cells) does not fit the 2×2 poster stash. The lab's own stock grows when an
item does not fit (`W.Own`), since it measures mechanisms, not space.

## Headless fidelity fixes the lab needed (they also affect the gym)

| Gap | Effect on the gym before 0c8d9fd | Fix (now in `main`) |
|---|---|---|
| `Headless.PatchGame` replaces every fluent `With*/Set*/Add*` method on `Machinery*` types with `return this` | `MachineryPanelRow.Add` and `MachineryPanel.SetContent` never attach their panels: **every machine and wine bottle has no slots** (`MachineHelper.GetBodyStorage` returns null). No hydroponics, furnace, rack, purifier, bottle or battery slot works headless | Only drawing methods are stubbed on `Machinery*` types (`sim/headless/Headless.cs`, patch version `25-lab2`). Gym smoke (seeds 3 and 5 farmer, 30 days) still runs clean |
| `GameItemElement.IsDestroyed()` is `handler == null`, and the fake render handler compares equal to null | `ChemicalHelper.RunNightlyChemicalSimulation` skips every container: no chlorine, acid/base, heat or organic reactions at night | Handlers marked alive before each play-time pass and before sleep (`Episode.PassTime`/`Sleep`; the lab's `W.Sleep` too) |
| Nightly burglary (33 % + 5 %/uninsured day) | Items under observation vanish | The lab keeps insurance on (`W.NoBurglary`) |

## Barter (`docs/re/BARTER.md`)

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| Add Markup accept = `100 − 2p + rep/6′ + event + dm` (rep/6 truncated, ×2 if negative) | `OfferMarkup(p)` on a seated LL `merchant`, rep set per trial; p ∈ {10,25,50,75} × rep {0,60,−30} × event | 20 000 per case (14 cases) | e.g. 10 %: 80.1 % (80); 25 %: 49.8 % (50); 25 % rep 60: 60.5 % (60); rep −30: 40.2 % (40); event: 65.1 % (65); 50 %+: 0 % | CONFIRMED (all) |
| Refuse-but-stay band `100 − 1.2p + …`, otherwise walk-out | same | 20 000 per case | stay 7.8/20.1/39.2/10.0 % for 10/25/50/75 (8/20/40/10) | CONFIRMED |
| Walk-out costs −3 rep (LL, normal) and sets intent DIALOGUE | rep diff on each walk-out | ~3 000–18 000 per case | −3 every time | CONFIRMED |
| Demand Discount: same accept/stay/walk formulas | `DemandBuyingDiscount(p)` on a seller | 20 000 per case (7 cases) | 80.2/50.6/0/0 % accept; stay 8.0/20.1/40.6/10.1 % | CONFIRMED |
| Add Markup success cuts budget 10/15/20/25 % | budget before/after | ≥ 1 000 per case | 0.150 (25 %), 0.200 (50 %) | CONFIRMED |
| Offer Discount always accepted, budget +2p % | `OfferDiscount(p)` | 5 000 per p | 100 %; +20/50/100/150 % | CONFIRMED |
| Offer Markup (you buy) always accepted | `OfferBuyingMarkup(p)` | 5 000 per p | 100 % | CONFIRMED |
| Blackmail on a criminal: `60 + rep/4′ + event` | `isWanted` client | 20 000 per case | rep 0: 60.0 %; rep 40: 69.4 % (70); rep −20: 50.1 % (50); event: 75.2 % (75) | CONFIRMED |
| Q&A #1: failed blackmail on a criminal: −5 rep, client stays; BM ×2 "presumed" | rep diff, intent | ≥ 6 000 failures per case | LL −5, BM **−10** (the ×2 applies through the static overload); never walks | CONFIRMED (BM ×2 now verified) |
| Innocent blackmail / false threaten: −7, or 50 % −10 and walks | non-wanted / non-aug client | 20 000 each | walk 50.2 % / 49.9 %; −10 exactly when walking; BM −14 / −20 | CONFIRMED |
| Threaten an aug always succeeds; aug refuses markup without walking | `isAug` | 5 000 | 100 %; 0 % accept, 0 % walk | CONFIRMED |
| One-shot guards: no 2nd markup/discount per side; no discount after coercion; Add Markup still allowed; Threaten still allowed after Blackmail; none on security | `Can*()` after each action | 1 each | all as documented | CONFIRMED |
| `IsWanted()` true for a non-wanted seller with a stolen item on the counter | stolen ingot on front counter | 1 | false → true | CONFIRMED |
| Trade rep = value × tradeMult × faction factor / 400, pre-bargain value, BM losses ×2, nice bargain +0.65/1.2/2/2.9, bad −0.5/−1.5/−3/−5, blackmail −6 | `SellItem` of an ingot after none/markup 25/discount 25/blackmail to LL, UL, BM | 14–40 per case | e.g. LL none +0.0825, markup −0.125, discount +0.185, blackmail −0.495; UL ×1.2; BM loss ×2 | CONFIRMED (within rounding) |
| Buying stolen multiplies the rep gain 1.5/2/2.5/3 by heat | `BuyItem` at heat 25/50/75/100 | 1 each | exact | CONFIRMED |
| Q&A #2: Demand Discount lowers `GetWholesalePrice()` | wholesale seller, 3 ingots | 1 | 128 → 97 (−24 %) | CONFIRMED |
| Q&A #3: contraband to a `noAcceptingContraband` client is blocked | `IsClientBuyingThisItem`, `CanSellThisItem` | 1 | both false | CONFIRMED |
| Q&A #3: flag set 35 % for LOWER_LEVEL_CITIZEN clients | 400 real openings | 303 LL buyers | 38.0 % (99 % CI 31–45) | CONFIRMED (the factory alone never sets it; it happens at generation) |
| Q&A #4: buying stolen is no crime; selling stolen = FENCING value×heat/100; contraband = trafficking value; BM buyer ÷2 | `BuyItem` / `SellItem`, crime ledger | 1 each | 0; 160 (=200×0.8); 272; BM 92 and 148 | CONFIRMED |
| Threaten "from day 12" (trigger unverified) | gym play, intro on and off, seeds 2/8/21 | 4 runs | `isAugIntroduced` flips on day 12 in every run (Shotgun.Unlock from the mentor dialogue) | CONFIRMED |
| Wanted list (no docs/re file before; now `docs/re/CLIENTS.md` "The wanted list"): bounty and faction terms per poster client; a report pays the bounty, +5 Security, and the faction change, scaled by difficulty (hidden-systems audit, 2026-11-12) | `barter.wanted`: each `wanted*` factory's fields; seat, `OnCustomerReported`, run the alarm's `onSecAlarmLeftAction`, hard | 1 per case | bounties 100–1 500; `wanted8_weekly` +125 cash / **+3.75** Security / **−7.5** LL; `wanted9` +300 / +3.75 / 0; `wanted1`, `wanted3` not reported headless (storyline intercept) | CONFIRMED (hard: rep gains × 0.75, losses × 1.25) |

## Window (`docs/re/WINDOW.md`)

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| Display buyers N = 1 + referral + showcase I + II − vandalism terms, floor 1; one per top-N type, no dice | `HandleShowcaseClient` with 4 types in the case | 200 per config (6 configs) | 1 / 2 / 2 / 4 / vandalized 1 / 1, identical every opening | CONFIRMED |
| Types ranked by summed displayed value | MEDICAL 200 vs FOOD 2×12, vs FOOD 2×240 | 1 each | MEDICAL, then FOOD | CONFIRMED |
| Skipped below attractiveness 100; empty case none | projector bonus tuned | 1 each | 99 → 0, 100 → 1 | CONFIRMED |
| Tier by case value: < 145 tier 1 (random of 3), ≥ 145 tier 2, ≥ 350 and day ≥ 14 tier 3, attractiveness ≥ 400 promotes | FOOD values 12/143/151/383, day 13/20, att 400 | 300 per case | as documented (tier 1 picks ~⅓ each) | CONFIRMED |
| A non-major type folds in by its major type (box_tampon HOUSEHOLD + LUXURY) | case = box_tampon | 1 | HOUSEHOLD_GOOD buyer | CONFIRMED |
| Discerning shopper chance 1 + ULrep/5 % | `HandlePerkClient`, cooldown cleared | 4 000 per rep | rep 0: 1.2 % (1); 50: 11.6 % (11); 100: 21.0 % (21) | CONFIRMED |
| Discerning cooldown: next possible roll at D+5 | 3 000 simulated openings at 41 % | 200 visits | minimum gap 5 openings | CONFIRMED |
| UL_DISCERNING_TASTE is the rep-0 perk | `IsPerkUnlocked` at UL 0 | 1 | unlocked | CONFIRMED |
| Contraband of **any** level in the case guarantees a showcase inspector | levels 1–4 | 200 each | 100 % | CONFIRMED |
| Stolen item in the case: inspector at heat %, found item heat → 100 | heat 10/25/50/75 | 20 000 each | 10.2/26.1/50.4/75.2 %; heat 100 every time | CONFIRMED |
| The showcase inspector is generated first | full opening with c4 in the case | 1 | queue position 0, plain `securityInspector` | CONFIRMED |

## State, day, inspections (`docs/re/STATE.md`, `DAY.md`, `docs/sim/SPEC.md` §6)

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| Initial heat by base value (≤17 clean, <50 → 25, <85 → 50, <140 → 75, else 100) | `InitStolenItem` on 11 items | 11 | exact | CONFIRMED |
| Heat −18 per night, clean at 0 | real nights | 9 nights | 100, 82, 64, 46, 28, 10, 0 | CONFIRMED |
| Contraband slows decay by 2/4/6/8 | stolen eng_keycard / dream_dust / c4 / crypto_module_cmd (levels 1–4) | 9 nights each | **−18 every night for every level** (`UpdateStolenItem` tests tags `CONTRABAND_LEVEL_*`, but the level is the *value* of tag `CONTRABAND_LEVEL`, so the branch never fires) | CONTRADICTED |
| Smuggler bays block the search | `RecursiveInspect` with c4 inside each bay | 1 each | smuggler_bay, _mini, _mod: concealed, not found; loose: found | CONFIRMED (mini included, via ITEM_HIDDEN_TAG) |
| Poster stash searched only when pulled | full `StartInspection` | 1 each | not pulled: c4 stays; pulled: seized | CONFIRMED |
| Poster stash capacity (real footprints) | place c4 | 1 | the stash is 2×2; c4 is 9 cells and does not fit (the test above enlarges the stash) | INFO |
| Inspection cadence by trafficking: <50 none, 3–5, 3–4 (≥150), 3 (≥300), 2–3 (≥450), 2 (≥600), 1–2 (≥750), uniform | `HandleInspectionClient` once per simulated day | 600 intervals per band | ranges exact; uniform (e.g. 32.8 % at 3 for 3–5) | CONFIRMED |
| FENCING counts as "trafficking"; FRAUD does not | same | 600 / 4 000 | FENCING 200 → 3–4 days; FRAUD 800 → never | CONFIRMED (docs should say "trafficking incl. FENCING") |
| Rent ×1.025/1.05/1.1 per landlord visit | gym play 36 days, rent paid | 5 visits each | normal 200→209→219→229→240→251; cozy 205, 210, 215, 220, 225; hard 220, 242, 266, 292, 321 | REFINED: the product is **truncated** (`(int)(rent × mult)`), so normal is +4.5–4.8 %/week and cozy is a flat +5/week |
| Buyout = base + per-day after day 50, capped | `GetStoreBuyPrice` at days 1–200 (normal) | 7 | 12 000; 12 050 at 51; 14 500 at 100; 17 000 cap | CONFIRMED |
| Shortage/surplus clients never appear (GUID key) | `HandleMarketClient` with 4 events forced; 400 real openings | 4 + 400 | 0 shortage/surplus clients, also on 233 openings with a ≥30 % type event | CONFIRMED |
| Per-event clients do arrive | full opening with the event | 4 | foodBuyer, pharmacist, foreman, miner first in queue | CONFIRMED |

## Production (`docs/re/PRODUCTION.md`)

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| Wine score = Σ berry (−1/0/+1/+2) + 1 if ≥ 2 berries + 1 if yeast + water term; tiers ≥6/5/3–4/2/1/≤0 | 19 bottles (berry mixes, 4 yeasts, water grades 0–5, rations), real nights | 19 (18 since 0c8d9fd) | every score and tier as documented; no score cap (3 best berries = 8 → tier 5, measured with 1×1 footprints; with real ones 3 berries + yeast do not fit one bottle) | CONFIRMED |
| Value at finish 175/150/125/100/85/0 | same | 18 | exact | CONFIRMED |
| Fermentation: none 4 nights, wine_yeast 2, superyeast 1, bioreactor 2, red yeast 2 | same | 19 | exact | CONFIRMED |
| Red yeast → counterfeit: value 44, contraband 2, fixed score 3, never ages | same | 1 | as documented | CONFIRMED |
| Start refused without water, without a berry (legal yeast), or with 375 ml | `TryStartFermentation` | 3 | refused | CONFIRMED |
| Aging +1 AGE and +3 × (1 + innate %) value per night, anywhere owned | 9 finished bottles, 6 nights | 54 bottle-nights | +2.5/3/3.8/4.5/5.2 per night by tier | CONFIRMED |
| "Rack ages twice" (own tick + rack step), 24 power/night | bottle in a powered rack vs in stock | 6 nights | rack 0,2,4,…,12; stock 0,1,…,6; 144 power over 6 nights | CONFIRMED |
| Bloomberry: harvest after 3 nights, then every 2; perennial | 54 bays, 13 nights | 324 harvests | nights 3, 5, 7, 9, 11, 13; seed survives | CONFIRMED |
| Bloomberry yield 8 per harvest | same | 324 harvests | **6** every harvest: the innate `perennial` trait carries HARVEST_QUANTITY −25 % and GROWTH_SPEED −20 % (work 23 then 12, not 18/10) | CONTRADICTED |
| Health loss per progress point (−1 %/pt below the mineral band, contaminant step table × 1.25) and health reset at each fruiting cycle | morning health per bay | 54 bays × 13 nights | pure 100→92→84; HQ 85/71; base 72/44; ghost 30/−40; rust 22/−56 per night pair; reset to 100 on every harvest night; health is not clamped (goes negative) | CONFIRMED (reset happens in the harvest night, see below) |
| Berry roll table by health band | `RollBloomberryCondition` at 10/40/70/95 | 20 000 each | 55.3/44.7; 25.3/45.1/29.6; 25.6/45.0/29.5; 40.1/59.9 % | CONFIRMED |
| End to end: bad-berry share predicted by health at harvest | 9 water setups × 6 bays | 216 berries each | pure 29 % (25), pure+6 tablets 0 % (0), HQ 34 % (30), HQ+6 tablets 22 % (17), base 75 % (75), ghost 100 % (100), rust 100 % (100) | CONFIRMED |
| WATER.md worked table: base water → health 0, "worst roll"; HQ → ≈55, "mostly −1/−2" | same | 216 berries each | base: first harvest ≈16, refruits ≈44 → 75 % bad, not all; HQ: ≈56 then ≈70 → 34 % bad, mostly normal/best | CONTRADICTED (the table ignores that refruit cycles are 16 points, not 23–24) |
| Furnace: one smelt/night; purity LOW/FAIR/HIGH/VERY_HIGH by ore and flux; blast 3→2; advanced flux never consumed | 8 furnaces, one night | 8 | every recipe exact; values 42/50/62/75, blast 100/150 | CONFIRMED |
| Furnace power 8 per smelt | battery diff | 8 | 8 without modules, **16 with the blast module** | REFINED |
| Scavenging: minor (5 + attempts) %, else major (1 + 0.25 × attempts) %, flashlight halves both | `ScavengeDumpingGrounds` at attempts 0–4, luck 0 | 100 000 per attempt (and 200 000 at attempt 0) | minor 5.1/6.3/6.8/8.1/9.1 %; major 1.0/1.2/1.4/1.7/1.8 %; flashlight halves | CONFIRMED |
| 5 attempts per visit; first 10/20 searches immune (intro on) | counters, 40 risky searches | 40 | 5; 10/20 with intro (0/0 with skipIntro); no wound in searches 1–10 | CONFIRMED |
| Loot: one `dumpingGroundTG` roll (+extras); never stolen | 95 796 searches | 95 796 | 1.02 items, 12.9 cr mean; junk 54 %, then ~2 % each of ~20 items; 0 stolen | CONFIRMED (table now measured) |

## Water (`docs/re/WATER.md`)

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| Six grades at 99.90 / 99.00 / 98.00 / 96.00 / 90.00 % | `GetPurityArrayIndex` at band edges | 12 | exact | CONFIRMED |
| Premade fills: grade 0 uniform 99.90–99.99; 1–3 near the lower edge (mean ≈ 8 % of band); 4 uniform 90–95.99; 5 uniform 35–89.99 | `AddWater(grade)` | 20 000 per grade | all in band; grades 1–3 mean **15.4–15.7 %** of the band above the floor (base ≈ 98.15 %); others as documented | REFINED (mean offset ≈ 2× the doc's) |
| Value = (round(40 × L) + container) × condition | full 750 ml bottled_water per grade | 6 | 65 / 50 / 35 / 25 / 12 / 8 | CONTRADICTED: the condition modifier applies to the water value only; the container price is added unmodified (pure 30×2 + 5) |
| Pour into empty copies the source label; topping up keeps the target's label | TransferLiquid | 2 | ghost label copied; base label kept at true 95.1 % | CONFIRMED |
| Portable purifier: to exactly ≥ 98.00 %, label updated | 6 L rust | 1 | 9800 bp, label base | CONFIRMED |
| Tablet: chemical → 0.7 % floor, then microbes; label stale; removed volume not returned | 6 tablets in 6 L rust, one night | 1 | chem 311 → 74 bp of water (0.7 % of volume), microbes 0, label still rust | CONFIRMED (needs the night-chemistry fix) |
| Filter pitcher strips organic/physical/mineral, not chemical/heavy metal/microbes; ≈ 37 durability per L at 91 % | 6 L poured (3 L fit) | 3 | rust 92.8 → 96.0 % at 32.8 dur/L; ghost 15.9; base 8.4 dur/L | CONFIRMED |
| Filter + tablets: rust → ≈ 98.3 % base | then 1 tablet/L, one night | 3 | rust → 98.68 %, ghost → 98.92 %, base → 99.13 % (HQ) | CONFIRMED |
| Purifier cap 99.01 % unupgraded; one night lifts 3 L of base to HQ; rust needs ~5 nights; label stale | 3 purifiers, 8 nights | 24 machine-nights | 6 L rust 92.80 → 95.17 → 96.70 → 97.89 → 98.81 → 99.01 (night 5); 3 L base → 99.01 in 1 night; HQ above the cap untouched; labels unchanged | CONFIRMED |
| Wine water term +1/+1/0/−1/−2/−3 from the true grade | bottles with water grades 0–5 | 6 | exact | CONFIRMED |
| Hydroponics: minerals are food, other contaminants cost health per point | garden table above | 54 bays | as above | CONFIRMED |

## Chemistry (`docs/re/CHEMISTRY.md`)

`sim/lab/ChemExps.cs`, 11 experiments, 73 claims (`fence-lab all -only chem.`; raw lines in
`var/lab/chem-results.jsonl`). Machines are the game's own: the blender's and finisher's buttons are
pressed through their callbacks, the plate and each container run their once-a-second callbacks in the
game's order (machine, then the container), the evaporator runs in the real night, supply bottles come
from `PreBuildChemHelper`. Pours of a measured volume use `WaterHelper.RemoveAndReturn` (what a
dropper or cylinder does).

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| Finisher impurity 12 / 6 / 3 / 1 / 0 % at 0–4 chem modules | 20 ml pure extract → dream dust | 50 units | 88 / 94 / 97 / 99 / 100 % (near pharma, pharma, pharma, lab pure, lab pure) | CONFIRMED |
| Finisher refuses a container with > 100 µL of evaporatives | 101 µL water in pure extract | 1 | no unit | CONFIRMED |
| Grade bands and values (dream dust 18: 42 / 33 / 25 / 18 / 10 / 4 / 1); narcotic buyers refuse junk/starch, grade 2 also cut | units at 100…20 % | 13 | exact; euphoria 3 from 80 %, 2 from 50 %, 1 from 37.5 % | CONFIRMED |
| Phanta pill toxic from 75 % purity, lethal at 100 % | units at 100/80/75/74/60/40 % | 6 | toxic at 75 % and above, clean at 74 %; lethal only at 100 % | CONFIRMED |
| Dream extraction window 40–60 °C (manual) | plate held at 35…70 °C, 180 s | 9 | extract 0 % at 35 and **40** (a 40 °C target settles at 39.9), 50 % at 45–50, 45 % at 55, 41 % at 60, 0 % at 65–70 | REFINED: the window is 40 ≤ T < 50 for full yield, and the plate target must be ≥ 41 |
| Yield = 50 % × min(1, ratio/5), ratio against the mush still unreacted | alcohol : mush 0.5…8 at 45 °C | 8 | 15.8 / 25.3 / 37.4 / 44.5 / 48.4 / 50 / 50 / 50 % — well above r/5 × 50 % at low ratios | REFINED (re-reading the ratio each second recovers most of the yield) |
| Extract left on an unmodded plate overnight is destroyed | 42.6 ml extract, one night | 3 | unmodded plate (99 °C) 0 %; 2-module plate at 45 °C 100 %; shelf 100 % | CONFIRMED |
| Protein precipitates 1:1 with the net acid/base excess; equal acid + base do nothing | 10 ml protein + 5/10/15 ml acid; + 10 acid + 10 base | 4 | protein left 5/0/0 ml, acid left 0/0/5; with both, protein untouched | CONFIRMED |
| Lipobond binds 8 fat per ml; neutralisation 10 %/s, min 1 ml, no product | 8 ml fat + 1/2 ml lipobond; 20 acid + 12 base | 3 | fat 0, lipobond left 0/1 ml; 8 ml acid left, nothing formed | CONFIRMED |
| Kotton (size 150) passes physical and mineral; water filter (22) leaves 0.16 % | mixed pour | 2 | kotton: debris and plant matter gone, physical/mineral pass; filter: 2.5 ml → 0.004 ml | CONFIRMED |
| Evaporator 300 ml/night split in proportion; distillate only with quality; solvent-recovery keeps alcohol | 400 ml water + 400 ml alcohol, 3 nights | 6 | 300 ml per night (150 + 150); no distillate unmodded; with the module (quality 50) 75 ml alcohol per night, no water | CONFIRMED |
| Lab-pure dream dust chain (4 modules, ratio 5.3, 45 °C, acid = protein, base, kotton + filter, evaporator) | 6 caps, 4 mini alcohol | 33 units | residue 99.22 %, 33 × lab pure (5.5 per cap), value 1386, 4 evaporator nights | CONFIRMED (CHEMISTRY §6.1) |
| Manual chain, no modules, heat to 45 °C then lift | 6 caps, 3 minis | 81 units | no filter: 51 × junk (36.5 %); kotton only: 30 × street (62.5 %) | INFO |
| Caffeine chain (4 modules, alcohol at 60 °C) | 3 beans | 25 units | 99.1 % lab pure when lipobond goes in below 55 °C; **95.7 % pharma** when added at 60 °C (lipobond boils, 4.9 ml fat survives); water extraction at 90 °C: 8 units | REFINED (add lipobond cold) |
| Phanta chain | 1 precursor bottle | 4 variants | acid = ¼ stabiliser + 4-module plate at 128 °C: 12 ml Phanta (10 % of methylol), 39 pills at 61.6 % street; whole acid bottle: 31 % junk; 1-module plate (fixed 160 °C): Phanta destroyed (0.04 ml); chlorine tablets lowered purity | INFO (CHEMISTRY §6.3) |
| Nomore: 45–55 °C, alcohol ≤ 10 %, 60 % yield, lipobond not consumed | 40 ml extract + 40 ml reducing | 4 | 24 ml nomore at 52 °C in 60 s, at 47 °C in 120 s; lipobond unchanged; 25 % alcohol → nothing | CONFIRMED |

## Concealment (`docs/re/CONCEALMENT.md`)

Experiments `conceal.*` (`sim/lab/ConcealExps.cs`), 10 experiments, 156 claims (133 confirmed, 0
contradicted, 23 info), `-n 20000`. Mid contraband (dream dust, 1×1) is the witness: it is
destroyed when found, so "seized" is observable. Built from `HEAD` (`git archive`) in a scratch
copy, since `sim/gym` had uncommitted work.

| Claim | Method | n | Result | Verdict |
|---|---|---|---|---|
| A full inspection searches stock, both counters, showcase, trash can (not by a lazy inspector), the poster stash only while open; never a smuggler bay's contents | `StartInspection` with a real `CreateInspectionClient(0/1/2)` (and a lazy blueprint with argument 0), dream dust in 17 places (the dossier refuses it) | 28 (7 places × 4 inspectors) | as stated, every case; lazy is taken from the client's flag even when the argument is 0 | CONFIRMED |
| Bays protect wherever they sit: each bay in stock, a bay on the back counter, in the showcase, in the trash can, a mini bay in an **open** poster stash | same | 28 (7 placements × 4 inspectors) | never seized | CONFIRMED |
| A plain container (first-aid box) gives no protection; inside a modified bay it does | same | 12 | seized in stock / trash (unless lazy); never inside the bay | CONFIRMED |
| Security rep: clean +3; only concealed items 0; lazy inspector skipping a trash can full of contraband +3; low alone −2 and kept; low + mid −5, low kept, mid seized; stolen base 200 at heat 100 −5 | same | 1 each | as stated | CONFIRMED |
| Stolen non-contraband in stock is seized with chance = heat | 4 000 inspections per heat | 4 000 × 3 | 24.5 / 48.7 / 73.4 % (25 / 50 / 75) | CONFIRMED |
| Luck voids the whole find with chance = luck | two mid items (stock + trash), luck 15 and 20 | 4 000 each | 15.0 / 20.0 %; never one without the other | CONFIRMED |
| Liquid luck (+75 while active) voids the whole find; a voided find gives no clean +3 (hidden-systems audit, 2026-11-12) | one mid item in stock, base luck 0, `liquidLuckDaysLeft` 3 (`conceal.odds`) | 4 000 | 74.1 % (99 % CI 72.3–75.8) voided; Security change 0 on all 2 963 voided finds | CONFIRMED |
| Personality ⅓ each; always normal at bribery crime ≥ 500; bribe = clamp(100 + 2·day − 2·Sec, 40, 400) | `GetInspectorPersonality`, `GetBribeAmount` | 30 000; 5 000; 6 | 33.8 / 32.9 / 33.3 %; 100 % normal; formula exact | CONFIRMED |
| A shady inspector who sees contraband on the counter at arrival offers no bribe and searches | `OnArrival` then the dialogue chain, picking the last choice | 2 | clean counter: 2 choices, bribe taken, nothing searched; counter contraband: 1 choice, stock searched | CONFIRMED |
| Security visitor (patrol officer) arrival: contraband of level 1 or 2 on the front counter, back counter or showcase triggers, and the chain ends in a full search of the shop; in stock, trash or the open stash nothing happens | `OnArrival`, then the dialogue chain | 12 | as stated | CONFIRMED |
| Arrival check on stolen goods: chance = heat, then heat 100 | back counter, heat 25 and 75 | 4 000 each | 25.4 / 74.9 %; heat 100 on every hit | CONFIRMED |
| A bay holding contraband on the back counter or in the showcase: no arrival trigger, no showcase inspector | `OnArrival`, `HandleShowCaseSecurity` | 2 | none | CONFIRMED |
| Overnight: front counter wiped (owned items too); trash can emptied only with the janitor on, a modified bay and its contents survive it, a plain bay does not; stock, back counter, showcase, poster stash kept | real open, close and sleep, janitor on and off | 1 night each | as stated | CONFIRMED |
| Burglary/repo pool: stock, back counter, showcase, trash can yes; poster stash, bay contents, the bays themselves (IMPORTANT_TAG), a mini bay in the stash, the dossier's magnifier no | `GetAllRepoEligibleItems` | 1 each | as stated | CONFIRMED |
| Uninsured first night: one item stolen at 33 + 5 % | `HandleInsurance` | 4 000 | 38.2 % | CONFIRMED |
| Early warning: "imminent" the night before every scheduled inspector; "tomorrow" = 2 openings, "in 2 days" = 3 | 300 real days at FENCING 100 / 500 / 800, `SEC_INFORMANT` on, night logs read | 398 scheduled inspectors | 398 / 398; lags as stated (≥ 750: "in 2 days" never appears). The 10 unannounced inspectors were all the fake inspector `wanted9` | CONFIRMED |
| Capacities and footprints | catalog of every item | 56 contraband items | stash 2×2; mini bay 2×2 → 3×3 inside, bay 3×3 → 5×5, modified 3×3 → 8×8; trash can and dossier 10×10; no bay nests in a bay; the mini bay fits the stash; no contraband item fits the dossier | INFO |

## SPEC §14 unknowns

| Unknown | Resolution |
|---|---|
| 1. Dialogue-choice flows headless | Resolved for the gym: landlord rent choices run (rent schedule above), the inspector flow runs (`StartInspection`), the mentor's shotgun dialogue fires on day 12. |
| 2. Seller stock vs reputation | Measured (300 visits per point): `scavGeneral` 8.9 items / ≈350 cr and `scavHaul` 7.0 / 105 cr at LL rep −50, 0, 50, 100 — **no LL scaling for these two**. BM `foodThief`: 3.0 → 3.0 → 3.4 → 5.9 items (42 → 90 cr) at BM −50/0/50/100, always stolen. |
| 3. Night production | Testable now (after the Machinery fix): wine, rack, hydroponics, furnace, purifier all run on the game's own `EndNight`. |
| 5. Unread code: `StoreEventActionDict` event clients | They arrive (foodBuyer, pharmacist, foreman, miner first in the queue on the event's day). |
| 4, 6 | Not lab questions (human-policy realism; patch churn). |

## The berry question ("growth 100 % but mostly bad berries")

Two things combine:

1. **The farmer start ships both hydroponic bays with Ghostwater** (large bottle, 96.2–96.6 %):
   chemical ≈ 112 bp, organic ≈ 74, microbe ≈ 56, heavy metal ≈ 37, and only ≈ 56 bp of mineral.
   Each progress point costs ≈ 8.75 % health (3.2 + 1.6 + 0.8 + 0.6 % from the contaminant step
   table, × 1.25 for bloomberry, plus 1 % for too few minerals), so the plant is at 30 after night 1
   and far below zero by the first harvest. Every berry rolls in the
   ≤ 25 band: 55 % moldy, 45 % under/over-ripe, never normal or best. Measured on the shipped bays:
   42 berries, 25 moldy, 17 under/over-ripe, 0 good. Rust and base water behave the same way (100 % /
   75 % bad).
2. **Health resets to 100 in the same night as the harvest roll** (`StartNextFruitingCycle` right
   after `FinishGrowing`). The morning after every harvest the plant reads 100 % (and the progress bar
   is back at the start), even though the berries just harvested were rolled at the depleted health.
   It is never "100 % at harvest"; the reading the player sees is the fresh cycle.

What works: pure water with 6 nutrient tablets per 6 L (≈ 150 bp mineral) keeps health at 100 →
0 % bad, 62 % best. A 250 ml nutrient-solution mini bottle in pure water (≈ 320 bp, slightly over the
band) → 0 % bad. Pure water alone → ≈ 25–29 % under/over-ripe (no minerals: −1 %/point). Filtering or
purifying strips minerals too, so add tablets afterwards.

## Corrections needed in `docs/re` / `docs/sim` (not made here)

1. **STATE.md / SPEC §6.1** — heat decay is 18 per night for every item; the contraband slow-down
   (−2/−4/−6/−8) is dead code (tag test against the tag's value). Stolen contraband cools exactly as fast
   as anything else (100 → clean in 6 nights).
2. **PRODUCTION.md §1** — bloomberry yields **6** berries per harvest, work **23** then **12** (innate
   `perennial`: −25 % yield, −20 % growth speed); timings (3 nights, then every 2) are unchanged. The
   "~28 berries/week" in §9 is ≈ 21, and the §7 chain table's 8 berries per cycle should be 6.
3. **WATER.md §2.3 worked table** — refruit cycles have only 16 penalty points: base water gives ≈ 16
   at the first harvest and ≈ 44 at later ones (75 % bad, not "worst roll"); HQ gives ≈ 56 then ≈ 70
   (≈ 30 % bad, mostly normal/best, not "mostly −1/−2"). Health is not clamped at 0.
4. **WATER.md §1.3 value** — the condition modifier applies to the water value only; the container's
   base price is added after it (750 ml pure = 65, not 70; gutter = 8, not 3.5).
5. **WATER.md §1.4** — premade grades 1–3 average ≈ 15 % of their band above the floor (base ≈
   98.15 %), not ≈ 8 %.
6. **DAY.md rent** — the multiplier result is truncated: normal 200 → 209 → 219 → 229 → 240 → 251;
   cozy is effectively +5 per week.
7. **PRODUCTION.md §3** — the blast module doubles power per smelt (16).
8. **BARTER.md Q&A #1** — remove "presumably / unverified": BM ×2 applies to the static
   `ModReputation(string, int)` (failed blackmail −10 BM, innocent −14/−20 BM). Threaten: "day 12"
   is now verified (mentor's shotgun dialogue, intro on or off).
9. **BARTER.md Q&A #3** — the 35 % `noAcceptingContraband` roll happens at client generation
   (`CompleteClientCreation`), not in the factory; the factory output alone never has it.
10. **STATE.md / SPEC §6.3** — "trafficking" for inspection cadence includes FENCING.
11. **SPEC §14.2** — LL rep does not change `scavGeneral`/`scavHaul` stock; BM rep does raise
    `foodThief` stock (≈ 2× at 100).
12. **sim/gym/README.md "What the headless host fakes"** — add the two gaps above (machine panels
    empty; night chemistry skipped). Both are fixed in `main` (0c8d9fd).
13. **DAY.md §1 `hidden` row** — backwards: `StorePoster.PullUp` sets `isPulled = true` and shows the
    stash, which is then searched; `PullDown` covers it. Further concealment corrections are listed
    in `docs/re/CONCEALMENT.md` §7.

## Can't test headless (or not tested)

| Claim | Why |
|---|---|
| Real-time machine effects (fridge) | The gym runs every item's once-a-second callback on its play clock (0c8d9fd); the chemistry machines are now exercised (§ Chemistry), the fridge is not |
| Tooltip/UI visibility claims (what the player sees) | No UI |
| Security patrol reacting to contraband added to the case while present | Needs the arrival flow of a specific patrol client; not set up |
| Rent "grows whether paid or not" | Not paying ends the run without `LANDLORD_GRACE`; not exercised |
| Gunsmith, barter-offer and info-dealer flows | Tablet modes still stubbed in the gym (the Appraisal Board's `OnEstimate` now runs in the gym, docs/re/APPRAISAL.md; no lab experiment yet) |
| Owned contraband dropped on a counter or the showcase while a security visitor stands there starts a search | Headless placement does not reach the visitor's area handlers (`GameItem.OnAddedTo*Area`); calling the handlers directly does turn the visit non-dismissable (`conceal.patrol`, INFO) |
