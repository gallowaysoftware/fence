# In-game manuals and papers

Index of every readable document in Probably Stolen. The game text is copyrighted and is NOT
in this repo. Personal-use copies assembled from the installed game's English localization
(`var/gamedata/loc_en.json`) live in `var/knowledge/manuals/<id>.md` (gitignored). The `id` below is
the item id (`MiscItemDirectory`) and the file stem. Code evidence is in `var/decomp`
(`MiscItemDirectory`, `PaperUIManager`, `MultiPageUIManager`, `IntelHelper`, `PlayerStore`,
`StoreShutterButton`, `NewGameData`, `NetworkUpgradeList`).

**Who gets them (owner's correction, 2026-11-09):** the free inspection guides and the
Mentor's Notes come only with the **apprentice start**; the arrival days below (e.g. stamp guide
day 11, injector guide day 25) apply to that start. Other starts don't receive them. The model is
still given the rules on every start (expert-player standard, `docs/PROTOCOL.md`), which
matters most on starts where a human would have to learn them by trial.

Limits of the local copies: they hold every localized string of a manual, in table order, not in page
order. Numbers that the prefab prints as sprite art (cigarette reference pack, stamp seal/logo/chip
variants, injector reference bodies, hand material price table, ribwich date-run diagrams) are not text
and are not in the copies. Confirm those by looking in game.

External transcription: wikily.gg has one page per manual under
`https://wikily.gg/probably-stolen/manuals-and-guides/<slug>` (index at that path). It looks faithful
where checked (injector guide figures match the localization exactly). No other fan wiki, Steam guide
or Reddit source found transcribes the manuals; the wikis already in `var/knowledge/text` (IGN, Gamer
Blurb, game wikis) only paraphrase the mechanics.

## Inspection guides (the rules for INSPECT.md)

| id / title | Obtained | What it teaches | vs our docs | Web |
|---|---|---|---|---|
| `injector_guide` Immunivax Assessment Guide (page heading: Injector User Handbook) | Day 25 delivery (`StoreShutterButton`); in dossier at start unless Sentinel start; Sentinel start gets it in the starting inventory (`NewGameData`) | Five checks: indicator state (genuine / expired / empty), label (missing label is counterfeit; spelling, missing company or product name, big font mismatch), seven-digit serial whose first three digits are the batch, valid range 1250000-4559999, recalled batches 250, 251, 301, 355, 385, 386, 421 are unusable, visible cracks are unusable, main body colours must match | Confirms INSPECT.md serial, recall list, label and body rules. Range edge: code draws prefix 125-455 plus 1000-9999, so the true lowest is 1251000, the guide rounds to 1250000. Guide says nothing about an expiry date, matching "no expiry date" in INSPECT.md. Guide does not say missing label can be told apart from other label faults beyond "counterfeit". Doc's "empty serial" and 6/8-digit serials are implied by "must be seven digits" | wikily `immunivax-assessment-guide` (checked, matches) |
| `stamp_guide` Food Stamp Identification Handbook (heading: Republic Food Stamp Authenticity Handbook), plus `logo_checker` | Day 11 delivery with the stamp mentor note, unless the commissary is banned; in dossier at normal starts | Stamps carry a chip, a missing or damaged chip makes the stamp unusable even if genuine; lettering faults (spelling, alignment, colour, font, missing); logo must be present and correct; printed value must be one of the listed denominations; a security seal exists exactly when the value is 160 or more | Confirms rule (b) seal vs amount and the denomination rule (a). DISCREPANCY 1: guide lists 240 as a legal denomination, but `CreateRealStamp/CreateFakeStamp` only draw 40 to 200 (no 240 found in the random generators). Treat 240 as legal but never seen. DISCREPANCY 2: guide frames a bad chip as "unusable even if genuine"; code treats any non-reference chip as a counterfeit tell (`STAMP_CHIP_ISSUE`). The player-facing verdict would differ for a damaged genuine stamp, which the generators never produce. Fake amount is genuine+10, which the guide's denomination list catches | wikily `food-stamp-identification-handbook` |
| `cigarette_guide` Hemingway Identification Guide (heading: Hemingway Cigarette Authenticity Handbook) and `cigarette_color` Hemingway Color Checker | Mentor story step (`StoreClientListStory`); both in dossier at normal starts | Counterfeits are worth far less; a missing or broken security seal means counterfeit; lettering faults (spelling, misalignment, uneven print, colour, font, missing); logo faults; compare the box colour against the colour card | Confirms the five issue categories. Guide never mentions a "box" tell separately from colour, consistent with INSPECT.md saying the box toggle is wrong on random fakes. Guide says "broken" seal, INSPECT.md only models missing (-1) | wikily `hemingway-identification-guide` |
| `aug_id_guide` Augmented Identification Guide | Starting item for the Shotgun-style perk (`StartingPerkContent`), a Security note at one story step (`StoreClientDialogList`) | Visual tells (odd pupils, uncanny expression, visible chip) and non-visual tells (stillness, twitching, outfit not matching faction, and similar) | Matches INSPECT.md "aug tells". Short list, no numbers | wikily `augmented-identification-guide` |

## Appraisal guides

| id / title | Obtained | What it teaches | vs our docs | Web |
|---|---|---|---|---|
| `ribwich_guide` Ribwich Appraisal Guide | Appraisal rank promotion (`AppraisalRankHelper`, `AppraisalItemType.guideItemId`) | Base, floor, counterfeit and opened value; the two production runs; first-run and first/last-month bonuses; good and bad facility lists; the deluxe (sesame bun and extra pickles together) and double-patty bonuses; Davre logo required; second run needs the Ribwich sticker, first run must not have it. Numbers: `local/appraisal_guides.json`, not committed | Matches the local value table number for number, including the facility lists. Guide clearly states the sticker rule; our note that the generator's Ribwich-logo branches are dead means real fakes rarely test it | wikily `ribwich-appraisal-guide` |
| `fuel_guide` Plasma Cartridge Appraisal Guide | Appraisal promotion | Base and floor; casing wear, crack, exposed screws and reinforcement; colour low, standard or high grade; four makers (WLD, RNR, ZHS, IGN) each with a bonus when ALL stated conditions match. Numbers: `local/appraisal_guides.json`, not committed | Matches the local value table | wikily `plasma-cartridge-appraisal-guide` |
| `hand_guide` Cybernetic Hand Appraisal Guide | Appraisal promotion | Base and floor; material, wear/damage, control interface, armored/blade add-ons are independent; tactical and deluxe set bonuses; two worked examples. Numbers: `local/appraisal_guides.json`, not committed | Matches the local value table where the text is present; material and interface values are in sprite tables, so confirm against the code values | wikily `cybernetic-hand-appraisal-guide` |

## Trade and production manuals

| id / title | Obtained | What it teaches | vs our docs | Web |
|---|---|---|---|---|
| `water_guide` Water Trading Manual | Water network upgrade, water start, mid-game purchase (`WaterHelper`, `NetworkUpgradeList`, `NewGameData`) | Water grades and safe/unsafe/toxic classes, how scanners test, purification, scams | Check against WATER.md (not diffed yet) | wikily `water-trading-manual` |
| `hydroponic_guide` Hydroponic Farming Manual | Hydroponics start or upgrade, shop (`StoreClientListSpec`) | Running the hydroponic system; water and nutrient inputs and analyser | Check against PRODUCTION.md and WATER.md (not diffed yet) | wikily `hydroponic-farming-manual` |
| `chemistry_guide` Chemistry Manual | Chemistry upgrade or start | Machines, two recipes, the caffeine and Phanta products, purity grades | Not covered by our docs | wikily `chemistry-manual` |
| `ingot_book` Metallurgy Manual | Furnace unlock (`IngotPurityHelper`), story step | Smelting ingots in the furnace, purity | Check against PRODUCTION.md | wikily `metallurgy-manual` |
| `wine_book` BloomBerry Wine Fermenting Guide | Wine upgrade (`NetworkUpgradeList`) | Water, yeast, berry inputs to wine score | Check against PRODUCTION.md and WATER.md | wikily `bloomberry-wine-fermenting-guide` |

## Papers, notes and templates

| id / title | Obtained | What it teaches | vs our docs | Web |
|---|---|---|---|---|
| `tutorial_book` Mentor's Handbook plus 18 Mentor notes (basic trading, food, wholesale, events, barter, upgrades, contraband, inspection, stamp, commissary, stolen items, display case, visual inspection, appraisal, after hours, labels, shotgun, sentinel) | Book on day 2; notes arrive by day (2, 4, 6, 9, 11, 13) or story/perk triggers (`IntelHelper`, `StoreShutterButton`, `StoreClientListStory`) | Early rules in the Mentor's voice: security inspection and reputation, contraband markup tiers (formatted with live markup values), stamps as welfare, visual inspection via magnifier, labeler use | Mentor note on contraband takes its numbers from `PlayerStore` markup fields, so the text is per-run. Confirms DAY.md/BARTER.md themes; not diffed line by line | wikily `mentors-notes` |
| `rev_codeword` Revolutionary codeword paper | Marketplace requests unlock with the REV_REQUEST perk | A codeword for the Revolution contact | Unmapped in our docs | none found |
| `intel_sec` Security Record, `sec_slip` Incident Slip | In dossier from start; story step | Case file on the shopkeeper: suspected crimes, investigation progress, expected sentence, assigned officer and note, recommended action | Matches the SecData fields we know; the data is runtime, not static text | none found |
| `wanted_paper` Security Wanted List | In dossier unless Known Snitch perk | Wanted names; contents are runtime | none | none found |
| `newspaper` Newspaper (also `NewsUI`) | Daily delivery, destroyed at day end | Template with title, subtitle, end-in-days lines; each active event supplies a news blurb (about 80 `event_*_news` strings) | Template and blurbs are static; event timing is in DAY.md events | none found |
| `lottery_info`, `scratcher` | Story and commissary | Six unique numbers from 1-49, prize tiers by matches, scratcher prize table (2 match 10, 3 match 75, nutrifruit 30 and 350) | Not covered | wikily `lower-levels-lottery-information`, `scratcher-ticket` |
| `mentor_contract` Lease Agreement, `deed` Deed | Start of game | Landlord and shopkeeper lease terms; deed of ownership | Not covered | wikily `lease-agreement`, `deed` |
| `exchange_directive` Underground Exchange Directive | Black-market progress | Lists the items exchanged through the trash can overnight, per night and promotional | Cross-check with BARTER.md black market notes | wikily `underground-exchange-directive` |
| `dossier` | Start | A container for documents, not text. Tutorial hint explains it | n/a | none |

## Findings to act on

1. The player-facing rules for injectors, stamps and cigarettes in INSPECT.md are all printed in the guides, so
   giving the model the rules regardless of guide ownership only gives it what the player eventually gets. The
   injector guide arrives late (day 25 unless starting with it), so it is the biggest advance over a day-1 player.
2. Stamp guide mentions 240 credits, code never generates it. Update the denomination check to allow it.
3. Guide treats a missing or damaged stamp chip as "unusable even if genuine", while code marks any chip
   deviation as counterfeit; verify in game what a bad chip does to value before relying on either.
4. Cigarette guide says a broken seal also means counterfeit; the code only models a missing seal.
5. Injector serial lower bound is 1251000 in code and 1250000 in the guide; harmless.
