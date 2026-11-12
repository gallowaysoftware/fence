# Fair play: the executor reads only what a player could

The rule for the stream: **the player sees only what a player could.** The executor may know the game's rules (a
knowledgeable player's knowledge) and anything a player can see or measure: the UI, tooltips, dialogue, the
magnifier's inspection view, a strip or AquaScan reading, the spectrometer, the gene scanner, the plant analyser,
and its own records. It never reads hidden per-item truth.

This audit covers the executor: the shared files compiled into the mod (`mod/src/Fence.Autopilot/SharedExecutor.props`),
the gym-only files that make decisions (`Episode*.cs`), and the counter rules the gym shares with the mod
(`mod/src/Fence.Core/Barter/BarterRules.cs`). Visibility comes from the `docs/re` VISIBLE / HIDDEN / DERIVABLE tags,
checked against the decompiled game (build 26525552). It closes HIDDEN-SYSTEMS.md gap #1 and the `water_label` read in
QUALITY.md §4.

## 1. What read the truth (main at e07d205)

| Where | What it read | Can a player see it? | Fix |
|---|---|---|---|
| `PlayerRole.cs:73` `HidesFake` (the display case's and the vending machine's filter, `PlayerRole.cs:213`, `Vending.cs:105`) | `IsFeatureMatch`, `IsRealFeatureCheaper`: the true condition of every unexposed feature | No: `realCondition` is HIDDEN until exposed (STATE.md, Items) | `Known.MayHideFake` |
| `PlayerRole.cs:90` `KeepWhy` "water_label" | `WaterHelper.GetWaterPurity`: true water purity against the label | No: the label's band is shown, the purity needs a strip or the AquaScan (WATER.md, QUALITY.md §4) | `Known.MayExpose`: a bought or poured-into container's label is unverified until a faucet fill or a test; a wanted container gets a strip before staging |
| `PlayerRole.cs:93` `KeepWhy` "exposable" | `StoreClient.CanClientExposeAnyFeatureDeep`: whether this buyer would catch a fake | No (STATE.md: "do not send") | `Known.MayExpose(buyer, item)` |
| `PlayerCounter.cs:59` `BribePicks` | the same, for the permit official | No | `Known.MayExpose` |
| `PlayerCounter.cs:92` `Exposable` (`PullExposable`) | the same, on the back counter | No | `Known.MayExpose` |
| `Labels.cs:129` `ForBuyer` (`labeler: per_buyer`) | the same, after relabelling a fake genuine | No | `Known.MayExpose` on the fake's recorded read |
| `DemandTrace.cs:55` (diagnostic) | `IsFeatureMatch` in the trace line | No | lists the unexposed condition features instead |
| `BarterRules.cs:61` `CoverageGap` (mod core, used by the gym) | `IsWanted` includes stealth-wanted clients, so they were dismissed | No: a stealth-wanted client has no poster (STATE.md) | `IsWanted && !IsStealthWanted` |
| `Breadth.cs:67` (`breadth.queue`, off by default) | the queue's buyers and their interests | No: the queue's count is shown, not who is in it (STATE.md) | the `queue` term is removed |
| `Lab.cs:156`, `:174` reagent bottles | `CURRENT_PART_*`: a bottle's composition, without the spectrometer | No: composition needs the spectrometer or AquaScan (CHEMISTRY.md) | without the spectrometer a supply is known by its bottle (`PreBuildChemHelper`: red acid, blue base, orange reducing agent, small pink lipobond, mini bottle alcohol, violet precursor) |
| `Lab.cs:229` cartel quota | `ChemicalProductHelper.GetPurity`: each dust unit's true purity | Only with the spectrometer | the spectrometer's purity, else the label's grade (the truth at pressing) |
| `Lab.cs:580` blend check | the vessel's mush content | No; its volume is shown | the vessel's volume grew |
| `Hydro.cs:187`, `:243` reed nights (moved to `Cellar.cs` by the water-cellar merge: `Batch.From`) | the label's exact basis points (after a faucet fill, the day's hidden tap roll) | The band only | the band's floor (gutterflow at the tap's 80 %) |
| `Cellar.cs` reed cure check (water-cellar merge) | contaminants, gated on the AquaScan | Yes | `Known.AquaScan` |
| `Breeding.cs` (breeding merge), seven trait-rank reads | `PlantGeneticsHelper.GetTraitRank`, not gated on owning the gene scanner its header cites | Only with the gene scanner | `Known.GeneScanner`, 0 without one |
| `Arms.cs` `IsNew` (frame-forge merge) | `realCondition` of the equipment condition | The claim: equipment condition never lies (`ItemFeatureList.EquipementConditionFeature`) | the claimed condition |
| `Dna.cs:33` `Hardy` (`wine_dna`, off by default) | a seed's trait ranks | Only with the gene scanner (PLANTS.md §5) | `Known.GeneScanner`, 0 without one |
| `Production.cs:586` bay status (log) | plant health | Only with the plant analyser | `Known.PlantAnalyser`, −1 without one |
| `Water.cs:95`, `Hydro.cs:189`, `:283`, `:360` | AquaScan purity and contaminants | Yes, already gated on owning the AquaScan | moved behind `Known.AquaScan` so the gate lives in one place |

Two found bugs on the way:

- **`HidesFake` was inverted.** It required `!IsRealFeatureCheaper()`, so it flagged claims *worse* than the truth
  (a genuine pack claimed counterfeit, a purified jug under its old label: never exposed) and let real fakes through
  (a counterfeit pack claimed genuine has `IsRealFeatureCheaper` true). The case and the vending machine therefore
  held the wrong items. `Known` uses the game's own test: a claim better than the truth.
- **A packed ammo box counts as "exposable" whenever the buyer would accept it** (`StoreClient.CanClientExposeAnyFeature`
  returns true for an accepted box: a game quirk). `Known` holds a packed box from every buyer: its contents are not on
  show. No measured route packs boxes.

Reads that stay, and why (the regression guard lists each):

| Where | Read | Why it is not a decision |
|---|---|---|
| `Quality.cs` | true water purity, contaminants, ingot purity | the summary's `production.quality` counters (QUALITY.md §4) |
| `Wine.cs:148` | `WINE_SCORE_INT` at fermentation | the `started_by_score` counter |
| `Hydro.cs:80` | `IsFeatureMatch` | read right after the executor's own strip (or with the AquaScan): the scanner shows Match |
| `Episode.cs` | `clientStack` | the gym's day loop serves the queue as the game does |
| `Episode.Player.cs` | `isAug`, `IsAug`, `augPresence` | the gym plays the aug's idle animation as the game does; `aug_eval` and `decide.hidden` |
| `Episode.Appraisal.cs` | `GetCorrectBaseValue` | the slip's true value, logged after the estimate |
| `StepMode.cs` | `augPresence` | reconstructible from the night log (AUGS.md §4.1) |
| `Hydro.cs`, `Cellar.cs`, `PlayerRole.cs` | contaminants, purity | `FENCE_BAY_DEBUG` / `FENCE_WATER_DEBUG` lines |
| `Arms.cs` (forge) | ingot purity | only our own furnace's ingots, whose purity follows from what was loaded |

Checked and clean (the sub-audit read every line): Rats, Fish, Furnace, Arms (gun stats are in every gun's tooltip),
Printer, Crops, Modules, Water, Wine, Fountain, Conceal, Style, Suppliers, AugRule (the default `tells` mode),
NightOuting, WildRead, ExecutorMorning (only known calendar events), Appraisal (the inspection view's cues), and the
policies (`HumanRole`, `KnobHumanRole`, `GymHeuristic`: no `hidden` field). Borderline, left as is: several water
engines rank containers inside one label band by the label's exact basis points (Production, Wine, Water, Fountain).
That is the label's own value, not the truth, but only its band is drawn.

**Time is not information.** Real-time play is allowed: keeping a day open so per-second reactions finish (an
extraction, a heating window) is something a human streamer can do, and it reveals nothing hidden. Fair play limits
what the executor knows, not how long it takes.

## 2. The known-truth store (`sim/gym/Known.cs`)

Only five feature categories can lie in this build: `ItemFeature.SetConditions` is called with a fake different from
the real condition only for cigarettes, food stamps, injectors (`ItemFeatureList`), chemical purity
(`ChemicalProductHelper.SetPurityFeature`) and water purity (`WaterFeatureHelper`), plus the labeler. Every other
claim is the truth by the rules. For the five:

| Category | How the player knows | Without it |
|---|---|---|
| cigarettes, stamps, injectors | the magnifier's inspection view, read once per item when it first matters (owned magnifier; every measured route gets one from the Board): the cues against the guides' rules (INSPECT.md; stamps: the amount a multiple of 40, the seal iff 160 or more, logo/lettering/chip as the guide) | suspect: held from any buyer whose type checks that category |
| water | the label is the truth after the executor's own faucet fill, a strip or AquaScan test, an exposure, or on a container it never bought nor poured into (a pour leaves the receiver's old label, `WaterFeatureHelper.UpdateWaterFeature`); the AquaScan reads any | the seller's word or a stale label: a buyer who wants it gets a strip first (while strips last), else it is held |
| chemical purity | the shop's own pressing; the spectrometer | a bought product is suspect |
| labeler rewrites | the executor's own record (`Labels` writes it to `Known`) | — |

Who checks what is a rule (`ClientCanExposeFunc`, set per client type at creation): Lower Levels, Black Market and
Security test water with strips; scavs, tourists, Upper Levels and the rest only catch gutterflow or rust water by
flashlight. Everyone but tourists and Upper Levels catches a fake cigarette. Injectors: an expired one claimed
genuine by everyone but the plain buyer, an unusable one unless its only fault is a recalled serial, a counterfeit by
Lower Levels, Security and scavs. Chemical purity: buyers who say they carry a scanner (the doctor's and nurse's
"scanner" lines). The permit official (the trade board) catches any over-claim. `Known.MayExpose(buyer, item)`
holds an item when some truth the player still thinks possible would be caught by that buyer; with no buyer
(`MayHideFake`: the case and the vending machine), by any.

## 3. The guard

`sim/gym.tests/FairPlayTests.cs` scans every `sim/gym/*.cs` for the hidden accessors (`IsFeatureMatch`,
`realCondition`, `IsRealFeatureCheaper`, `CanClientExpose*`, `GetCorrectBaseValue`, `isAug`, `isStealthWanted`,
`dayUntilInspection`, `clientStack`, `GetWaterPurity`, `GetContaminantPercentage`, ingot and chemical `GetPurity`,
`GetHealthPercent`, `GetTraitRank`, `augPresence`, `WINE_SCORE_INT`, `CURRENT_PART_*`, the `*_ISSUE` cue-truth tags,
…). `Known.cs` (the measurement layer) and `Quality.cs` (summary counters) may use them; anywhere else a use fails
the test unless it is on the per-file list of section 1's "reads that stay", with its count and reason.

## 4. The cost

Paired arms on the gym, 60 seeds (2910001–060), `-days 1000`, each route's own start, perks and directives
(`sim/lab/scripts/arms-fairplay-{hard,normal}.txt`, reported by `sim/lab/scripts/fairplay_report.py`). Three builds
of the same code (main at e07d205): **truth** (main as it was), **truth, fixed** (main with `HidesFake` corrected and
the exposure counter: it still reads the truth) and **fair** (this branch before the later merges). `sprinter` plays
`appraiser`'s runs seed for seed in every build (identical results), so it is listed once.

Fair against truth (wins, Wilson 95 % CI; McNemar on the paired seeds; paired win-day Δ on seeds both won, ± 1.96 SE):

| Route | hard: wins truth → fair | hard: median day | hard: Δ win day | normal: wins truth → fair | normal: median day | normal: Δ win day |
|---|---|---|---|---|---|---|
| `appraiser` / `sprinter` | 60/60 → 60/60 (94–100 %) | 90 → 97 | +2.6 ± 4.0 | 60/60 → 60/60 | 62 → 62 | −0.6 ± 1.2 |
| `rancher_sprinter` | 60/60 → 60/60 | 76 → 76 | +4.2 ± 15.4 | 60/60 → 60/60 | 62 → 62 | +0.2 ± 1.6 |
| `outlaw` | 60/60 → 60/60 | 104 → 104 | −0.2 ± 9.0 | 60/60 → 60/60 | 69 → 69 | −0.3 ± 1.2 |
| `water_baron` | 60/60 → 60/60 | 104 → 104 | +2.8 ± 9.1 | 60/60 → 60/60 | 62 → 69 | +1.6 ± 1.3 |
| `armourer` | 60/60 → 60/60 | 104 → 104 | −1.3 ± 9.2 | 60/60 → 60/60 | 62 → 69 | +0.6 ± 1.4 |

No route loses a win on either difficulty (McNemar: no discordant pair anywhere, p = 1). Normal buyouts land on
rent days, a week apart, so a median moving 62 → 69 is a quarter to a half of the seeds slipping one week.

Decomposed (`appraiser` and `water_baron`):

| | hard `appraiser` | hard `water_baron` | normal `appraiser` | normal `water_baron` |
|---|---|---|---|---|
| the `HidesFake` fix (truth → truth, fixed) | +0.8 ± 3.0 | −0.8 ± 7.6 | −0.2 ± 1.4 | −0.1 ± 1.2 |
| the information (truth, fixed → fair) | +1.8 ± 4.1 | +3.6 ± 8.0 | −0.3 ± 1.4 | **+1.8 ± 1.4** |
| buyer exposures a run, truth, fixed → fair | 0.3 → 0.4 | 2.4 → 2.3 | 1.1 → 1.1 | 1.7 → 1.3 |

The one cost that clears the noise is `water_baron` on normal, about two days (one rent week on a third of the seeds):
fair play spends ~31 more strips a run (101 against 70) and leaves water waiting for its test ~31 more times (97
against 66), and sells ~6 % less water (6 740 against 7 153 credits). Buyers expose fair play's stock no more often than
the truth-reading guard's: the exposures left (water jugs bought and poured into, mostly tested by the merchant) come
equally with the guard that reads the truth, so they are not an information gap. A player-visible recovery was tried and failed: twice the strip
stock (`water_strips: 24`, on the merged code) moved `water_baron` by +0.0 ± 1.5 days on normal and −2.2 ± 9.2 on
hard, so the lever was not kept. Once the AquaScan is owned (the water route buys it when a seller has one), `Known` reads every container through it.

The guard was load-bearing on main's old normal setting (54 → 58 of 60, HIDDEN-SYSTEMS.md); on these routes every arm
wins 60/60 either way, so the guard's value shows only as rep and days, and fair play keeps it.

## 5. The mod

`Known.cs` is a shared executor file (in `SharedExecutor.props`); `make mod` and `make test-mod` are green, with seven
new game members in `compat/hooks.json` and `AmmoBoxHelper` in `RuntimeResolver`. Live, `Known.Inspect` reads the
same cue tags the inspection view draws, without opening it. **A live inspection chore** would make it visible on
stream: target the item with the owned magnifier (`InspectionUIManager.OnInspectItem`, the magnifier's
`onThisTargetItemFunc`), hold the view a beat, close it, then record the read: one chore per inspectable item when it
is first asked about (bought cigarettes, stamps and injectors), gated on the magnifier being in reach. The strip
before staging bought water already goes through `Chore.Act("test_strip")`.

## 6. What is left

- **The water cost on normal** (+1.8 ± 1.4 days on `water_baron`) is not recovered. Candidates a player could use:
  buy the AquaScan earlier (it is bought when stocked; its price and the seller's stock decide when), or stop pouring
  into bought containers (the stale label is what needs the extra strips).
- **The live inspection chore** (section 5) is not built: live, `Known.Inspect` reads the cue tags the inspection
  view draws without showing the view.
- **Within-band label ranks.** Production, Wine, Water and Fountain rank containers inside one label band by the
  label's exact basis points; the label is not the truth, but only its band is drawn.
- **The bridge side** (`internal/`, `decide.hidden`) was not audited here: the LLM's context is the mod's
  (CLAUDE.md's invariant); this audit is the executor.
- **Measured before four merges.** The arms ran on e07d205; the water-cellar, formula, frame-forge and breeding merges came after
  and were re-audited (the guard caught the forge's two reads and the breeding engine's seven) but not re-measured route by route.
- **Real-time play** is allowed (section 1): fair play is about information, not time.
