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
| 2 | Hydroponic Farmer | Vintner | `farmer` | 60/60 | — |
| 3 | Gunsmith | Armourer | `gunsmith`, `gunsmith_farm` | 60/60 | — |
| 4 | Water Merchant | Water Baron | `water_baron`, `fountain`, `water_night_shift`, `fountain_pharma` | 60/60 | — |
| 5 | Rancher (Rat Wrangler) | Rancher | `rancher`, `rancher_sprinter` | 60/60 | — |
| 6 | Fence | Fence | `outlaw`, `fence_fortress` | 60/60 | Security −30 on hard |
| 7 | Rock Bottom | Shark | `rock_bottom` | 57/60 | the underdog |
| 8 | Aquafarmer | new: Fishmonger | none | not measured | gym support; confirm it is selectable (built with the "pending" constructor but not flagged coming-soon or hidden) |
| 9 | Chemist | Cook | none | not measured | gym support; the cartel's weekly quota, stipend and protection (docs/personas/cook.md); the chemistry engine |
| — | Junker, Organ Trader, Street Food Vendor, Moonshiner, Pharmacist | — | — | — | coming soon in this build: not playable |

Season 1 is ready to stream for starts 1–7 today; 8 and 9 need their engines and gym starts first.

## Season 2, the Specialists: niche runs with jeopardy

Each is a card whose odds the lab puts between ~30 and ~70 %, so the run can be lost. The production engines run at
the best grade their equipment allows (docs/research/QUALITY.md, CHEMISTRY-ENGINE.md, to come).

| Card | Start | Archetype | Rule | Why it plays well |
|---|---|---|---|---|
| Legal Highs | Apprentice → Retired Chemist | Pharmacist (new) | chemistry, caffeine only; no contraband in stock or sales | a clean lab racing a rent clock; purity is the score |
| The Cook | Chemist | Cook | lab-pure dream dust under the cartel's quota | heat, inspections, liquid luck as insurance; the darkest run |
| Water to Fish | Aquafarmer or Water Merchant | Fishmonger | income from water and fish only; no Board | a closed loop from tap to tilapia |
| Green Water | Hydroponic Farmer | Botanist (new) | water purified by plants only (hydroreed's `purifying` trait, ~28 % of the contaminants a night); no purifier, filters or tablets | the slowest, strangest water chain in the game |
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

The archetype's persona file sets what the player cares about and how it talks: what it brags about, what it
despises, how it takes a loss. The strategist writes its thesis and plans in that voice, the narrator speaks it, and
the co-host is cast as its opposite (the Honest Broker's co-host is a shark; the Cook's is a pharmacist). New personas
needed: Fishmonger, Pharmacist, Botanist.

## Readiness gates, per card

1. Rules from the decompiled source, written up in docs/re (own words, no game text).
2. The engine in the gym at the best grade its equipment allows; the card's handicaps enforceable as locked keys.
3. The engine in the **mod** (the live game): an engine that exists only in the gym cannot be streamed.
4. Odds measured: the executor alone on 60 hard seeds under the card; then the model on drift in a league rehearsal.
5. The player sees only what a player could: no hidden per-item truth (HIDDEN-SYSTEMS.md gap 1 must be closed before
   any stream).
6. Persona file and the format card's copy.
