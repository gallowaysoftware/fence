# Strategy space after the formula sweep (2026-11-12)

Inputs: FORMULA-EXPLOITS.md (18 lab experiments), SOURCES-SINKS.md, CLAIMS-AUDIT.md, PRODUCTION-VERDICTS.md,
HIDDEN-SYSTEMS.md, and the engines merged today. The top claims below were re-checked in the decompiled source by
the coordinator, not only read from summaries (frame-built guns finish New with base stats × print quality,
`GunFrameHelper.Finish`; the permit donation is 25 + 10 × ⌊day/7⌋ a week from day 45, `StoreClientList.GetBribeAmount`;
alcohol is not a reactant in R1/R7).

**Verdict.** The game is far deeper than the routes that win it today. Every route so far wins by trading plus the
Appraisal Board, with production bolted on at the guide's recipe. The source gives at least five engines that,
built from the formulas, each plausibly earn a Board-sized income on their own, and a dozen cheap rules every route
should play. Board-free (3 wins in 2 940 before today) is the format to re-test once they land.

## 1. Engines, re-ranked

| Engine | What the formulas allow | Was | Bound now | Owner |
|---|---|---|---|---|
| Chemistry | standing alcohol vessel (caps one at a time, alcohol never consumed) + solvent recovery returning all of it + wine as top-up; 6–12 caps a night through one evaporator | alcohol "the wall", 26 pills a week | mush/beans and vessel time | chem-loop (running) |
| Gene lines | edits clone forward with heat cleared; caffeine/mush production traits move between species; bountiful/fast/potent/drought ranks stack across generations | one hardy rank on bloomberries, off | manipulator time and odds | gene-lines (running) |
| Arms | printed frames finish New (+30 % stats) with stats × print quality → a heavy pistol clears every Mid order bare (~300), with mods High orders 685–816; mod kits for service orders; build to threshold + 1 | bought Fair guns, ~0 orders filled | frame chips + metal/CNC modules | **unowned (round 2)** |
| Water | Pure with no modules (filter × 2 → reed → UV), fountain Pure slot +186 a night, HQ wine water by blending | "Pure unreachable on hard" | litres per day | water-cellar (running) |
| Wine | HQ water, 3 good berries + yeast = Master Vintage 175, aging +3 × grade a night uncapped, never refuse a buyer | "aging hurts" | water, bays | water-cellar (running) |
| Fish | eel tank as a power hub (78–234 units a night to neighbours), fillerweed → omnivores (690 kcal a bay-night), pellets to 24 %, harvest at ≥ 900 meat | feed- and power-starved | feed, tanks | **unowned (round 3)** |
| Power | eel hub; recharger + POWER_HIJACK; nightmare caps → poison barter → a 100-charge cell a night | cells bought at ~1.65 | — | fish round 3 / chem-loop |
| Furnace | learning modules trained in an unpowered alarm reach Q 200 without a neural core | ore-bound | ore | **unowned** |

## 2. Rules every route should play (cheap, measured or source-exact)

Grace-defer the last 1–2 rents before the buyout (≈ +1 000, up to a week earlier) · hold cash on day 43's morning
for Georgio (+1.5 per credit held) · one insured night in four (43 % cheaper) · the appraisal blacklist (hand,
ribwich) · report wanted posters (≈ 2.1 k a run) · no insurance under Alert · the fridge (+20 % on drinks, rations,
premium water) · the window counts multi-type goods in every type · 2 tablets per jug · never buy the business
permit, donate weekly and shoot every aug (≈ +2 400 by day 98; A/B pending). Owner: cheap-wins (running).

## 3. What it means for the campaign

- **Native routes become real.** Gunsmith: the frame forge. Chemist/Apprentice: the solvent lab. Farmer: gene lines +
  reed water. Water Merchant: Pure water and the fountain. Aquafarmer: eels as a power plant. Each is a distinct,
  explainable engine for the narrator.
- **New niche cards.** Permitless (never buy the permit, live off aug bounties: jeopardy built in); The Forge (only
  guns you built); The Breeder (one gene line from a single seed); Eel Power (no bought cell all run).
- **Board-free** gets re-measured with the engines stacked; if it becomes winnable at 30–70 %, it is the boss card.
- Challenge's "the cash buyout is out of reach" is re-tested with the engines.

## 4. Gym bugs found by the sweep

`PlayerCounter.BribeReserve` holds owed + 95 but the weekly add passes 95 from day 59 (a refusal crime follows) ·
`Modules.cs` overvolts modules in the alarm (a 10 % ruin roll every night) · `Water.cs:315` dissolves too many
tablets. Owners: cheap-wins (reserve), water-cellar (tablets), the next fish/modules round (alarm).

## 5. Next round (under the 16-gym cap)

1. Arms round 2: the frame forge and mod kits (2 gyms).
2. Fish/power/modules round 3: eel hub, fillerweed feed, pellets, harvest timing, rats at age 2–4, learning modules
   (2 gyms), after one of the running agents frees its share.
3. When chem-loop, water-cellar, gene-lines and cheap-wins land: stack everything on each start's native route and
   re-measure the Gauntlet and Board-free.
