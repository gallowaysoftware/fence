# Player archetypes

Probably Stolen has one winning condition (buy the shop out, or pay off the mortgage) and many
ways to fund it. This note lays out the starts and the income engines the game offers, then
turns them into archetypes: a start + an engine path + a temperament, each a persona the
strategist can be given (`docs/personas/<id>.md`, `strategy.Options.Persona`, bridge
`-persona`, a gym `sim_run` event's `persona`). Variety is the point: the mechanics favour one
line, but every archetype below is a workable run with its own risks and its own story.

Numbers are build 26525552, normal difficulty, read from code (`docs/re/*`, `docs/sim/SPEC.md`,
the decompile, `var/gamedata/*.json`); nothing here is play-tested unless it says so. "Net"
means value at estimate before the retail markup, events and haggling.

## Common frame

- Every run: cash 600, rent 200 due every 7 days (first on day 7), ×1.05 per landlord visit,
  buyout 12,000 (rises 50/day after day 50, cap +5,000); mortgage offered at cash ≥ 2× the
  2,000 down payment, then 12 weekly payments of 930.
- Wild's network upgrades (`upgrades.json`) cost credits, paid first from **Wild favor**:
  5 % of every trade's value, a refund of 50 % of Wild service fees, +40 per Lower Levels rep
  point gained, 200 on introduction (`StoreReputation.OnItemTraded`, `WildUIManager.OnUnlockClicked`).
  So a shop trading 1,000/day earns ~50 favor/day toward upgrades on top of profit.
- Production machines tick **once per night** (`PlayerStore.EndNight`); a night without
  sleep is lost. Chemistry's heating plate, the fridge and the auto-butcher also tick **per
  real second** while the shop is open (`onOneSecondItemFunc`).
- **No start locks you in.** Every start's engine can be bought later from Wild's network or a
  visiting seller (table B). A start only seeds the first days: its kit, its cash, its rent, its
  faction offsets. So an archetype below is a **path** (a target engine set, a purchase order
  with triggers, a temperament) playable from any start, with a note on which start makes it
  cheapest.
- Starting perks (3 points, at most 4 perks, `NewGamePerksUI`) can stand in for kit: `vending`
  (2) installs the vending machine, `working_pipes` (2) the Gutterflow and Rustwater taps,
  `sawyer_crew` (1) the Underground Exchange, `handy` (1) a toolbox with a welder,
  `wild_favorite` (1) −10 % on every upgrade, `model_citizen` (1) +30 Security.

## A. The starts

Cash is after the kit is paid for (base 600). Rent is the weekly rent before the first ×1.05;
an extra +40/+50 compounds with every landlord visit (~+600 over ten weeks).
"Replaces" is the upgrade the kit stands in for, i.e. what another start pays to catch up.

| Start (menu) | Cash | Rent | Kit (`NewGameData.HandleInitialItem`) | Replaces | Pushes you toward |
|---|---|---|---|---|---|
| **Apprentice** (`APPRENTICE`, "recommended for new players") | 550 | 200 | Storage bay, 2 processed meat, juice, 2 toothpaste, 2 toilet paper; **the only start that gets the Mentor's Handbook and notes and the free inspection guides** (stamp guide day 11, injector guide day 25, cigarette guide; `docs/knowledge/MANUALS.md`) | storage bay (300) | Trading and the **inspection trades**: injectors, food stamps, cigarettes and the appraisal service are natural for a human here, since the rules arrive in the mail. Every other start learns tells by trial. (Our model gets the rules on every start, by the expert-player standard.) |
| **Water Merchant** (`WATER_MERCHANT`) | 400 | 200 | Water manual, full test-strip and purification-tablet boxes, jugs and bottles (ghost and rust water), pitcher + filter, shabby portable purifier (75 %), shabby bottle printer, `WATER_TRADER` unlocked | Water Trading (300) + kit | Filtering and relabelling, then purifier and taps |
| **Fence** (`FENCE`) | 450 | 250 | Shabby smuggler bay, 2 dream dust, heavy handmade gun + ammo, med/eng/sec keycards, 2 black lamps; Black Market +30, Security −30; the Underground Exchange from day 1 (others: day 25) | smuggler bay (250, inventor) | Buying stolen, cooling it in the bay, the exchange; black lamps invite Black Market targets |
| **Hydroponic Farmer** (`FARMER`) | 450 | 240 | Farming manual, an old hydroponic (3 glitched + 1 ruined stuck modules) and a decrepit one (2 eco + 2 ruined stuck), seeds (nutrifruit, fillerweed), nutrient tablets, rations, `RETIRED_FARMER` unlocked | Retired Farmer (350) + a second bay (300) | Crops; one Winemaker upgrade (200) away from the wine engine |
| **Gunsmith** (`GUNSMITH`) | 150 | 250 | Old 3D printer, firearm sign, 4 plastic, gun storage bay, 11 boxes of ammo, 4 guns, mods, **Class I permit in the window**, Security +25, `RETIRED_GUNSMITH` + `GUN_PERMIT_I` unlocked | Retired Gunsmith (650) + Permit I (300) | Gun orders, Security rep for Permits II/III |
| **Chemist** (`DRUG_DEALER`) | 300 | 200 | Shabby blender, heating plate, evaporator, chem finisher, glassware, coffee bean, chem scanner, mini smuggler bay, chemistry manual, Security −15, `RETIRED_CHEMIST` unlocked; **the cartel**: weekly dust quota, stipend, protection from burglary | Retired Chemist (1000); the cartel exists **only** on this start | Dream dust to quota; the cartel's purity curve |
| **Rat Wrangler** (`RANCHER`) | 600 | 200 | Shabby cage + dispenser, 3 rat pairs (ages 5/3/1), 6 nutrifruit, green serum, cleaver, labeler, pen; `RETIRED_RANCHER` unlocked | Ranching (750) | Breeding and butchering; the only engine start that costs no cash |
| **Aquafarmer** (`AQUAFARMER`, pending text) | 150 | 240 | 2 shabby RAS tanks with water tanks and power, an adult tilapia pair, fertilized roe, 2 fish feed, 2 ghost-water jugs; `AQUACULTURE` unlocked (skipping its Water Trading prerequisite) | Water Trading (300) + Aquaculture (400) + a RAS (300) | Fish; very little cash for the first rents |
| **Rock Bottom** (`ROCK_BOTTOM`, "extreme challenge") | 0 | 200 | Nothing (not even the cassettes) | — | Scavenging and flipping; the Landlord Grace favor as a lifeline |
| Junker (`SCAV`, "work in progress", menu-disabled; the gym's `-start scav` runs it) | 400 | 200 | Scav token, metallurgy manual, shabby metal scanner, flashlight, satchel, bandages, shabby furnace with power, `RUINED_MACHINE_UNLOCK` | Ruined-machine scrapping (300) + furnace (200) | Furnace and scrapping |
| Moonshiner, Organ Trader, Pharmacist, Street Food Vendor | — | — | Menu entries marked work in progress; no kit code in this build | — | not playable |

## B. Income engines and how to acquire them from any start

Costs are credits on normal (×1.5 hard, ×2 challenge; ×0.85 at Lower Levels 100; ×0.9
`wild_favorite`), paid from Wild favor first. "Visitor" is a weekly seller the upgrade brings.

| Engine | Acquire from a non-native start (upgrade id, cost, prerequisite) | Running cost | Net, from code | Risk |
|---|---|---|---|---|
| **Counter trading** | always on; `SHOWCASE_I` 200 → `SHOWCASE_II` 500 (+1 guaranteed window buyer each); `BUY_REFERAL` 200 (+1 shopper, +1 window buyer); `SELL_REFERAL` 200 (+1 seller); `RENOVATION1–3` 500/750/1000 (+150 attractiveness each: 400 → tier-1 clients +10 % budget, 550 → +1 customer +25 %, 700 → tier-2 +50 %) | referrals 10/night | each window buyer is a guaranteed sale at tier 1–3 budgets; budgets +25 % per Endorsement tier (2500/5000/10000) | haggling rep |
| **Inspection trades** (injectors, stamps, cigarettes) | always on; the guides are free only on Apprentice | — | fakes are worth far less than genuine: pay a fake's price, sell the genuine at full | buying a fake at a genuine price |
| **Appraisal service** | appraisal board from day 5, join 85; certificate in the window brings appraisal clients (33–55 %) | promotions 106/128/149 | per item: base fee 20/40/60/100 by rank + 5–25 % of your estimate | refusals → walk-outs, lost progress |
| **Water** | `WATER_TRADER` 300 (kit; water-trader visitor; Dr. Jackson stocks purifier, moisture farm, bottle printer); `GUTTERFLOW_TAP` 250 → `RUSTWATER_TAP` 350 (or `working_pipes`); `VENDING_FOUNTAIN` 1200 | power, filters 25, tablets 6 | a filter ≈ 10.8 L rust → base = +324; purifier base → HQ 3 L = +60/night; Pure for the stockpiler (budget 500 + day); fountain up to 3 base + 2 HQ + 1 pure bottles/night | over-labelling = fraud/endangerment crime + rep |
| **Wine** | `WINEMAKER` 200 (3 yeast, 3 bottles, 2 berries, water, magnifier, manual; winemaker visitor sells 4 bottles, yeast, berries); berries need a bay: `RETIRED_FARMER` 350 (kit + farmer visitor) | bottle 12, yeast 10, power 8/night | one healthy bay gives 6 berries a harvest (every 2 nights after the first 3: ~21 a week) → wine **~270/night at 1 berry a bottle, ~205 at 2**, bottle-limited (measured, `docs/sim/VERIFIED.md`); aging +3 to +5.25/bottle/night, uncapped, ×2 in a rack (350); no market event touches wine | none; counterfeit (red) yeast makes contraband |
| **Other crops** | `RETIRED_FARMER` 350; `PLANT_DNA` 1250 (needs farmer) for traits | power 8, water | kotton (loom module → fabric) ~100–140/night, coffee ~65, fillerweed ~50 (1-night cycle), nutrifruit 20–60, dream cap ~45 (contraband 2) | dream/nightmare cap contraband |
| **Furnace / metal** | furnace 200 from the inventor (`inventorIntro`) or the retired junker (`RUINED_MACHINE_UNLOCK` 300, hidden in the menu); blast module 150, junk module 120 (inventor); ore: `MINER` 250 → `MINER_SHAREHOLDER` 900 (6 ore/Friday, −35 % wholesale) | flux 4, power 8/smelt (16 with the blast module) | 2 scrap + flux ~18/night; blast on ore **~85/night** at 16 power a smelt (measured); ingots ride MATERIAL events (+30 construction, +50 meteor) | none |
| **Gunsmithing** | `RETIRED_GUNSMITH` 650 (shabby printer + power; gunsmith visitor); `GUN_PERMIT_I` 300 → `II` 600 → `III` 900, delivered at Security 20/50/80, revoked below 0 | printer power 8, plastic 12, ingots | permit in the window → gun buyers 50/40/35 %, rewards 63–100 / 190–200 / 350–360 (+46–80 per bonus met), cash on acceptance; printer: 2 plastic → printed gun (40) in ~2 nights, 1 ingot → 3 gun parts (75) | guns above your permit are contraband |
| **Chemistry** | `RETIRED_CHEMIST` 1000 (lab kit, chemist visitor, shady farmer selling dream caps, street dealer budget 200 + 4·day); `CHEMIST` 250 (dust at −20 %) → `CHEMIST_SHAREHOLDER` 1440 (8 dust/Saturday, −35 %); the **cartel only on the Chemist start** | caps 23, alcohol, power | dream dust uv 18 at 60 % purity, **+135 % at ≥ 99 %**; caffeine pills (legal, 10); cartel stipend 150 (+4·rep at Cartel 25) | dust is contraband 2; quota misses cost cartel rep, zero dust is game over; heating plate needs real-time ticks |
| **Livestock: rats** | `RETIRED_RANCHER` 750 (kit + rancher visitor); auto butcher 400 | nutrifruit ~21/rat | a rat at day 8 (≥ 400 g) = raw meat 159; ~135/night per breeding female, ~150–250/night per cage gross (cage space, footprints unverified) | disease (~13 %/night), escapes below cage health 20 |
| **Livestock: fish** | `AQUACULTURE` 400 (needs `WATER_TRADER` 300; 1 RAS, roe, feed; aquafarmer visitor) → `AQUACULTURE_II` 1200 (eels) | feed 100 per 5000 kcal, power 5, filters | tilapia ~60/night per tank; eels ~30–35/night each (6 per tank); sturgeon caviar ~85/night | unpowered or unfiltered tanks kill fish |
| **Vending** | `VENDING_MACHINE` 1200 or the `vending` perk | power 2+ | ~70/night of varied cheap food at attractiveness 15; near-certain sales at ≥ 75 | mislabelled or stolen stock = crime |
| **Suppliers / shareholders** | `BREWER` 200/1450, `MINER` 250/900, `CHEMIST` 250/1440, `PHARMA` 300/2500 | — | −20 % wholesale (−35 % as shareholder); each dividend pays its share back in 10 weeks at value | chemist dust is contraband |
| **Fencing** | smuggler bay 250 (inventor, day ≥ 12, weekday 5; at 66 % of list) or a `smuggler_bay_mod` from the exchange; `CRIMINEL_NETWORK` 350 (+45/night: a 500–750 budget criminal buyer and +2 BM); `INSPECTION_INFORMANT` 300; `WILD_FIXER` 350/450/550 (25/45/70 a night); `EVIDENCE_REMOVAL` 500 (14-day cooldown); `SEC_GRACE` 250 | fees | stolen goods bought cheap, cooled 18 heat/night, sold clean; buying stolen earns ×1.5–3 rep; Black Market 40 adds weekly thieves with stock ×1.25–2.5 | hot sale = fencing crime; an inspection finding a hot item outside the bay resets it to heat 100 |
| **Bounties and reports** | always on (wanted list unless `known_snitch`) | — | captures 100–1500 + 5 Security; reports 50 + 25/40/60 per contraband item | report penalty −8 → −20 with that faction |
| **Hypertech smuggler** | `HYPERTECH_SMUGGLER` 2500 + 500 a visit | — | resale margin negative unless a buyer pays a premium; a 15000 turbo booster or 10000 bay is a late-game toy | contraband 4 cores |
| **Marketplace requests** | `MARKETPLACE_REQUESTS` 250 | — | rewards 1.5–2× item value + rep; event requests 350–700 | — |
| **Information dealer** | visits after day 13, 50 a tip | — | a known event 1–4 days ahead (FOOD +50, WATER +50, MEDICAL +50 …) to buy into | — |

## C. Windfalls

The game's dice throw machines and modules at you. Sellers (`var/gamedata/clients.json`
stock samples), the dumping ground (`loot_tables.json`) and the exchange carry items that are
worth an upgrade or a whole engine. Every persona says: **stay on path unless a windfall
clearly beats it**; a windfall that starts an engine worth more than the current path's next
purchase is a reason to pivot.

| Windfall | Where it turns up | What it enables | Worth to |
|---|---|---|---|
| Furnace (200), blast module (150), junk module (120) | inventor, retired junker | the metal engine without the Junker start | Smelter (core), Armourer (ingots → parts), Magnate |
| Smuggler bay (250) / mod bay (400) | inventor; exchange (bay + 8 Oxymore) | uninspectable storage | Fence, Cook (core); anyone carrying stolen stock |
| Hydroponic bay (300), farm modules (nutrifruit, kotton loom, dream cap) | retired farmer | a second bay, new crops | Vintner, Rancher (feed), Magnate |
| Water purifier (250), moisture farm (350), evaporator (300) | retired water merchant, Dr. Jackson | HQ/Pure water | Water Baron, Vintner (Pure water = +1 wine score), fish |
| RAS (300), auto butcher (400), feed dispenser (300) | retired aquafarmer / rancher | fish or rats on autopilot | Rancher, Water Baron's fish branch |
| 3D printer (350), CNC / metal printer modules | retired gunsmith | printed guns, gun parts from ingots | Armourer, Smelter |
| Turbo booster (35, single use) / adv (300, recharges in 3 nights) | dumping ground (basic); Lower Levels rare merchant (LL 40) | one extra machine night on demand: a bay harvest step, a smelt | every production path; worth ~one night of the best machine |
| System modules: quality / performance / efficiency (70), eco / fineness / overclock (100) | dumping ground, scavenger and thief sellers, inventor | +quality (purifier to Pure at ≥ 100, wine rack aging), +speed | Water Baron, Vintner, Smelter; resale 70–100 otherwise |
| Neural cores (capped 250, uncapped 1500, contraband) | dumping ground, exchange (1 %), hypertech smuggler | high-value contraband | Fence, Shark (flip it); others report or avoid |
| Keycards (sec 100, cmd 125, contraband 2–3) | Fence kit, exchange crates, desequencer | exchange barters, sales to criminal buyers | Fence |
| Storage expansions (machine bay ext 1200, large bay 850) | inventor storage stock | room for more machines and stock | Magnate, any production path |
| Wine rack (350) | Dr. Jackson once the winemaker is unlocked | 2 aging steps a night | Vintner |
| A seller dumping goods far below estimate | any day | a cheap stock lot | everyone, within cash floor |

## D. Archetypes

Each is a persona file (`docs/personas/<id>.md`, ≤ 800 bytes) the strategist renders as YOUR
STYLE. Day targets assume normal difficulty and a start that does not already own the engine.

### 1. Honest Broker — `honest-broker` (cheapest from Apprentice)
A trained appraiser who never cheats. Signature loops: concentrated window at tier 2–3, inspection
trades (genuine injectors, stamps and cigarettes bought at fair prices, fakes priced as fakes),
the appraisal service for fees and certificates in the window. Order: Showcase I by day 10,
Buyer Referral by day 14, Showcase II and Renovation I by day 30, Renovation II at cash 3000.
Factions: Lower Levels 20 → Security 60 → Upper Levels 20. Risk: none; no contraband, threats,
blackmail or hot sales. Pivot: at cash 2500 after day 30, add the cellar. Opportunism: buy any
underpriced lot or module to resell; never a stolen bargain that would still be hot at rent.
Win: buyout by ~day 70 with **no crime ever recorded**.

### 2. Water Baron — `water-baron` (Water Merchant; else Water Trading 300)
Owns the tap. Loops: filter + chlorine turns free rust water into Basewater (+324 a filter), the
purifier lifts a container to HQ each night, Pure feeds the stockpiler and the fountain. Order:
taps by day 10, a second purifier by day 20, Vending Fountain at cash 2500, Aquaculture once two
purifiers run (the fish branch, which is where the Aquafarmer start leads). Factions: Lower Levels
20 → 60 (water-event rumours); Upper Levels for HQ and Pure. Risk: low, but the label is its word:
test and relabel everything. Opportunism: purifiers, moisture farms, quality modules; a quality
module that lifts the purifier to Pure is a pivot inside the path. Win: buyout by ~day 65 with
**no fraud or endangerment crime**.

### 3. Vintner — `vintner` (Hydroponic Farmer; else Retired Farmer 350) — **mechanics favour this one**
Runs a cellar. Loops: bloomberries on HQ/Pure water with nutrient tablets (6 berries a harvest, ~21 a
week per bay), one good berry per bottle with yeast while bottles last (~270 a night; two a bottle
gives ~205), bottles aged (uncapped) and sold to Upper Levels and the discerning shopper.
Order: Winemaker by day 3, all bottles and yeast each visit, Water Trading by day 14, wine rack at
cash 1500, a second bay at 3000. Factions: Upper Levels 20 → 100. Risk: low. Never sells a berry
as food while a bottle is free. Opportunism: bays, bottles, turbo boosters, quality modules.
Win: buyout with **the finest cellar**.

### 4. Smelter — `smelter` (Junker in the gym; else the inventor's furnace 200)
Turns trash into metal. Loops: dumping ground every night, furnace every night, ore from the
Mining Coop, ingots held for MATERIAL rises. Order: Mining Coop by day 10, blast module by day 14,
Miner shareholder once ore is weekly, the gunsmith printer at cash 2000 (ingots → gun parts).
Factions: Lower Levels 20 → 40 (rare merchant sells turbo boosters). Risk: wounds, not crime.
Opportunism: furnaces, modules, ore lots; a printer windfall pivots toward the Armourer.
Win: buyout by ~day 75 on metal.

### 5. Fence — `fence` (Fence; else the inventor's smuggler bay)
The Black Market's shop. Loops: buy stolen cheap (extra rep), cool it in the bay, sell it clean;
criminal-network buyers with 500–750 budgets; the Underground Exchange. Order: Inspection Early
Warning by day 7, Criminal Networking by day 14, Fixers as the record grows, Clemency held.
Pivot: at Black Market 40 weekly thieves arrive with richer stock. Factions: Black Market 40 →
60, Security above −60. Risk: high heat, moderate contraband. Opportunism: bays, keycards,
neural cores. Win: buyout in cooled goods, **record under 300 crime**.

### 6. Cook — `cook` (Chemist; else Retired Chemist 1000, without the cartel)
Runs a lab. Loops: dream caps → extract → dust, purity is the price (+135 % at ≥ 99 %); on the
Chemist start, the weekly cartel quota (10 dust at 40 %, +5 and +5 % a week, capped 70 %) pays a
stipend and protection. Order: Lower Levels Chemist by day 7, Fixers by day 14, Evidence Removal
before evidence hits 80, chem modules first. Factions: Cartel 25 → 50 → 100 (exit), Black Market
40. Risk: highest: contraband 2 product, a quota miss can end the run. Win: **cartel 100 and the
buyout**.

### 7. Armourer — `armourer` (Gunsmith; else Retired Gunsmith 650 + Permit I 300)
Arms the station legally. Loops: the permit in the window draws gun buyers who pay cash on
acceptance (63–360 + bonuses), mods and ammo, printed guns and parts, weapon events (mob justice
+33 %). Order: Permit II at Security 50 by day 30, Permit III at 80, furnace at cash 1500.
Factions: Security first (officer trades, wanted captures +5). Risk: legal contraband only.
Opportunism: chips, mods, frames, ingots. Win: **Permit III and the buyout, Security 80**.

### 8. Rancher — `rancher` (Rat Wrangler, free kit; else Ranching 750 or Aquaculture 400)
Feeds the Lower Levels. Loops: rats bred on nutrifruit, butchered near 400 g (159 a carcass),
later tilapia; food held for FOOD rises. Order: Retired Farmer by day 14 for feed, a second cage
at 1500, auto butcher at 2500. Factions: Lower Levels 20 → 40 → 100 (Pillar: upgrades −15 %).
Risk: disease, not crime. Win: **Lower Levels 100** and the buyout by day 90.

### 9. Shark — `shark` (Rock Bottom)
Starts with nothing and bites. Loops: scavenge nightly, flip everything, hard bargains with
sellers in need, markups on hurried buyers, threats when the odds favour it. Order: Landlord
Grace the first affordable day, Seller Referral by day 10, Showcase I, then whatever its best
find points to. Pivot: the first machine it finds becomes its engine (it adopts that path's
order). Factions: hard on Upper Levels and tourists, fair to Black Market (losses double) and
Lower Levels. Risk: high on rep, moderate on crime. Win: **survive the first month**, then the
buyout by any means.

### 10. Magnate — `magnate` (any start; Apprentice or Farmer cheapest)
The generalist: trading pays the rent, and every engine that pays back within two weeks gets
added (cellar, water, furnace, vending). Order: Showcase I by day 10, Winemaker + Farmer by day
14, Water Trading by day 21, furnace by day 28, Vending Machine at 3000, Renovations after.
Pivot rule: add an engine only when cash after it covers two rents; drop one that misses its
payback. Win: buyout by day 60 **with four engines running**.

### Why the Vintner, and why the others still play

The code's numbers favour wine: a single healthy bay (~21 berries a week) feeds ~270 a night of
wine at one berry a bottle (~205 at two) for ~550 of upgrades (Winemaker + Farmer), wine is immune to every market event, ages without a cap and
needs no crime. Only bottles and yeast limit it, and the winemaker sells those weekly. The
Magnate is the Vintner plus everything else and is the strongest run once cash allows.

The others stay worth playing because each wins something wine cannot: the Honest Broker and
Armourer build Security to the point where inspections stop (Security 100: no scheduled
inspections) and budgets rise (+50 % at 80); the Water Baron sells the same litre three ways and
speculates on the commonest event family; the Fence and Cook carry the run's drama (heat,
inspections, the cartel) and the richest buyers; the Rancher's meat (159 a carcass) rivals wine
per night if disease and cage space hold; the Smelter and Shark show what the dumping ground
alone can do, and the Shark is the only one that must adapt to whatever the dice bring.

## E. Which archetypes the gym can play headless today

Since 2026-11-11 the gym also plays the phone suppliers and their shareholders, the rats and the 3D printer, and the
table below is partly stale; docs/research/ARCHETYPES-NEW.md has what each is worth on hard and what is still not
modelled (the chemistry lab, aquaculture).

`sim/gym/README.md` lists what the headless host fakes. Every path needs **upgrade purchases**
(the Wild Network window is stubbed; no decide kind exists for buying) — so today every
archetype plays only from its native start's kit, and only its counter side.

| Archetype | Also needs in the gym |
|---|---|
| Honest Broker | appraisal tablet mode; inspection mini-game reads; showcase upgrades bought |
| Shark | coercion moves (threaten, blackmail) are offered; otherwise closest to playable now |
| Fence | smuggler-bay moves (stage stolen stock into the bay), inspection confront panel, exchange moves; 1×1 footprints make the bay unlimited |
| Water Baron | water moves (pour, filter, test, relabel, fill purifier), vending fountain stocking |
| Vintner, Magnate | production moves (plant, water the bay, fill and start bottles, harvest), machine power; nights tick already |
| Smelter | furnace loading moves; dumping-ground loot is in (`loot_tables.json`) |
| Armourer | gunsmith order tablet mode, printer chip/material moves |
| Rancher | cage/tank moves, butchering; the auto butcher ticks **per second** (not run headless); `marking_pen` fails to build on the rancher start |
| Cook | **per-second ticks** for the heating plate and reactions, blender/finisher button moves, cartel dialogue |
