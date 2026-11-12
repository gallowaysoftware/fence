# The campaign: how the live runs are structured

A live run is a **scenario card**: the conditions the run is dealt, and the archetype the player plays as. The model
chooses its route inside those conditions (`fencechoose`, docs/STREAM.md "Model agency") and speaks as the archetype
all run: the day-0 thesis, the plans, the narrator's lines and the co-host's foil.

## The scenario card

| Field | What it fixes | Notes |
|---|---|---|
| `start` | the game's start (below) | the save is created on that start |
| `perks` | the starting perks | ≤ 3 points, ≤ 4 slots (7 with Colorful Character); negatives refund points (docs/re/PERKS.md §1) |
| `difficulty` | hard | challenge for an encore |
| `seed` | announced before the run, or rolled on stream | the gym is deterministic per seed, so the lab's odds for *that* seed can be shown |
| `format` | handicaps, locked keys, deadline (internal/format) | a handicap the executor cannot break: the model's plans are overruled on locked keys |
| `archetype` | persona: voice, values, boasts and scorn, the engines it favours | docs/personas; flavours every line; biases the route menu, does not dictate it |
| `allowed` / `banned` | engines and goods the run may not touch | e.g. no contraband; enforced as format handicaps |
| `odds` | the executor's measured wins out of 60 under the card | shown on the format card; a card goes live only once measured |

A card is a format plus a start, perks and an archetype: `sim/lab/scripts/directives/formats/*.json` gains
`start`, `perks` and `persona`, and `fencechoose -format` reads them (to build).

## Season 1, the Gauntlet: beat every start on hard

Nine playable starts in the installed build (`StartingScenario` list in the decompile). Season 1 plays each once, on
hard, as the archetype the start suggests, with the route the model picks. Order: the reference first, then
ascending difficulty, the cartel last.

| # | Start | Archetype | Route menu today | Lab on hard | Blocker |
|---|---|---|---|---|---|
| 1 | Apprentice | Honest Broker | 20+ routes | 60/60 | — |
| 2 | Hydroponic Farmer | Vintner (Botanist for Green Water) | `farmer` | 60/60 | — |
| 3 | Gunsmith | Armourer | `gunsmith`, `gunsmith_farm` | 60/60 | — |
| 4 | Water Merchant | Water Baron | `water_baron`, `fountain`, `water_night_shift`, `fountain_pharma` | 60/60 | — |
| 5 | Rancher (Rat Wrangler) | Rancher | `rancher`, `rancher_sprinter` | 60/60 | — |
| 6 | Fence | Fence | `outlaw`, `fence_fortress` | 60/60 | Security −30 on hard |
| 7 | Rock Bottom | Shark | `rock_bottom` | 57/60 | the underdog |
| 8 | Aquafarmer | Fishmonger | none on the menu; candidates `aquafarmer_sprinter` (`engine: fish`), `aqua_fish_weed` | 53/60; 60/60 with fillerweed bays (mostly at the mortgage's end) | ghost band and deadline odds; selectable (docs/re/AQUACULTURE.md §4) |
| 9 | Chemist | Cook | none | 0/60 (the cartel ends 46 runs, median last day 35) | the evaporator's power and the cartel's growing count (docs/research/CHEMISTRY-ENGINE.md, "The Cook, why it fails") |
| — | Junker, Organ Trader, Street Food Vendor, Moonshiner, Pharmacist | — | — | — | coming soon in this build: not playable |

**The lab column overstates readiness.** Those wins are mostly the Appraisal Board winning from any start, not
the start's own trade: the `gunsmith` route is the Board plus wine and a furnace, and never buys a weapon permit,
fills a gun order, mods a gun or runs the printer. A Gauntlet run plays the start's **native path** at the best its
kit allows (the Gunsmith's arms trade, the Farmer's bays, the Water Merchant's water, the Rancher's rats, the
Fence's stolen goods, the Chemist's lab, the Aquafarmer's tanks), so each start needs a native route measured
on hard before it airs; the Board is allowed alongside unless the card bans it. Native engines in progress:
chemistry, water/hydroponics/wine quality, fish and rats, the arms trade.

## Season 2, the Specialists: niche runs with jeopardy

Each is a card whose odds the lab puts between ~30 and ~70 %, so the run can be lost. The production engines run at
the best grade their equipment allows (docs/research/QUALITY.md, CHEMISTRY-ENGINE.md, to come).

| Card | Start | Archetype | Rule | Why it plays well |
|---|---|---|---|---|
| Legal Highs | Apprentice → Retired Chemist | Pharmacist | chemistry, caffeine only; no contraband in stock or sales | a clean lab racing a rent clock; purity is the score |
| The Cook | Chemist | Cook | lab-pure dream dust under the cartel's quota | heat, inspections, liquid luck as insurance; the darkest run |
| Water to Fish | Aquafarmer or Water Merchant | Fishmonger | income from water and fish only; no Board | a closed loop from tap to tilapia |
| Green Water | Hydroponic Farmer | Botanist | water purified by plants only (hydroreed's `purifying` trait, ~28 % of the contaminants a night); no purifier, filters or tablets | the slowest, strangest water chain in the game |
| Sommelier | Hydroponic Farmer | Vintner | wine only; sell only the top tier, aged on the rack | patience against the rent; a cellar worth watching grow |
| Foundry | Apprentice | Smelter | ingots only; held for material rises | the blast furnace and the market's swings |
| Closed Loop | any | Magnate | no bought inputs: everything grown, filtered, bred | the whole hidden economy in one run |
| Bounty Hunter | Fence | Fence | report the wanted posters (bounties ~2.1 k a run); Lou's chain left open | docs/research/HIDDEN-SYSTEMS.md gap 2; a storyline that can end the run |
| Clean Hands | Apprentice | Honest Broker | no contraband, no fakes, no threats | the Board's purest form, sprinted |
| Colorful Character | Rock Bottom | Shark | seven perks, the negatives included (addictions: cold turkey or feed it) | chaos with a measured price |
| Board-free | any | the model's pick | the Appraisal Board off | the boss fight: 3 wins in 2 940 today; the production work is aimed at it |

Deadline formats (`sprint_91`, `purist_sprint`, `challenge_clock`) stack on any card, and the last-played route is
off the day-0 menu, so a card replayed is a different run.

## The archetype is the voice

The archetype's persona file sets what the player cares about and how it plays: what it brags about, what it
despises, how it takes a loss. The strategist's plans follow it (the style section of the plan prompt), the narrator
speaks in it (a fixed Kokoro voice and a speaking style, `internal/show/persona.go`), and the co-host Rook is cast as
its opposite: each persona carries the angle Rook takes on it (the Honest Broker's is a shark's, the Cook's a
pharmacist's). The day-0 thesis itself is written persona-blind (`fencechoose` has no persona); the persona comes in
when the narrator speaks it and Rook answers. A persona file's `## Notes` (its sources, voice, Rook's angle and sample
lines) are for people: no prompt carries them, and the part before them stays under 800 bytes.

## The archetypes

Every persona, its native start and route, its cards and how it sounds. A route's persona is the route file's
`_route.persona`, used when a run names none.

| Persona | Native start | Native route | Cards | Voice | Rook's angle |
|---|---|---|---|---|---|
| Honest Broker | Apprentice | `lean`, `fair_dealer`, `pharma` | Season 1 #1, Clean Hands | `af_bella` | honesty that leaves money on the counter |
| Vintner | Hydroponic Farmer | `farmer`, `vintner` | Season 1 #2, Sommelier | `bf_emma` | wine that ages while the rent does not |
| Botanist | Hydroponic Farmer | none yet (`quality.reeds` is built, the staged build is not) | Green Water | `af_heart` | two weeks for water a machine cleans overnight |
| Armourer | Gunsmith | `armourer`, `gunsmith` | Season 1 #3 | `bm_lewis` | a gun shop waiting on a permit |
| Water Baron | Water Merchant | `water_baron`, `fountain`, `water_night_shift` | Season 1 #4 | `af_sarah` | tap water at a markup, one jug from a fraud case |
| Fishmonger | Aquafarmer (or Water Merchant) | `aquafarmer_sprinter` (candidate) | Season 1 #8, Water to Fish | `bm_fable` | a fish stall in a pawn shop, one dark night from empty tanks |
| Rancher | Rancher | `rancher`, `rancher_sprinter` | Season 1 #5 | `am_adam` | a pawn shop that smells like a barn |
| Fence | Fence | `outlaw`, `fence_fortress` | Season 1 #6, Bounty Hunter | `am_michael` | one inspection from a cell |
| Shark | Rock Bottom | `rock_bottom`, `sprinter` | Season 1 #7, Colorful Character | `am_fenrir` | a hustler with no engine |
| Cook | Chemist | none (0/60) | Season 1 #9, The Cook | `af_nicole` | the cartel owns the lab, the way out keeps receding |
| Pharmacist | Apprentice → Retired Chemist | `caffeine_lab` (candidate: 60/60 with the Board, 0/60 without) | Legal Highs | `bf_isabella` | coffee pills while the cooks make the money |
| Smelter | Apprentice (the Junker start is not playable) | `smelter`, `smelter_sprinter`, `ore_monopolist` | Foundry | `am_puck` | a scrapyard calling itself a foundry |
| Magnate | any | `appraiser` (the default persona) | Closed Loop | `bm_george` | four engines half fed |

Rook speaks in `bf_alice` beside a male narrator and `bm_daniel` beside a female one (`CohostVoice`), so no persona
uses either.

## Readiness gates, per card

1. Rules from the decompiled source, written up in docs/re (own words, no game text).
2. The engine in the gym at the best grade its equipment allows; the card's handicaps enforceable as locked keys.
3. The engine in the **mod** (the live game): an engine that exists only in the gym cannot be streamed.
4. Odds measured: the executor alone on 60 hard seeds under the card; then the model on drift in a league rehearsal.
5. The player sees only what a player could: no hidden per-item truth (HIDDEN-SYSTEMS.md gap 1 must be closed before
   any stream).
6. Persona file and the format card's copy.
