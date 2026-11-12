# Production chains: full inventory (RE notes)

Build 26525552 / 0.46D. Same conventions as [`RECIPES.md`](RECIPES.md): **VISIBLE** = rendered by the
UI (evidence = the UI code or a localized description), **HIDDEN** = internal only, **DERIVABLE** =
computable from visible facts. Types are in the global namespace of `Assembly-CSharp`. Read from
code, not play-tested. "(inferred)" marks a conclusion drawn from the absence of code,
"(computed)" marks arithmetic done here from code numbers. Prices on **hard** use the hard
`DifficultyRules` (`Difficulty.Vanilla()`, third preset): `retailMarkup` 0, `inventorMarkup` 50 %
(Dr. Jackson's goods), `upgradePriceMultiplier` 1.5 (Wild network), `powerCostMultiplier` 2.

This file is the index of every way the game turns inputs (items, power, credits, time) into more
value, plus the support machines around them. Chains already documented elsewhere are only
listed; the chains that had no RE notes are written up in §1–§3.

## 0. The short version

- Every released night machine is accounted for below. The only released chains that had no RE
  notes were **power generation** (recharger, power block, breeder cell), the **brewer, chemist and
  pharma shareholder dividends**, and **gambling** (lottery and scratchers).
- **Power is not ~1 credit a unit on hard.** Dr. Jackson's 50 % markup applies to the charge
  as well as the cell, so bought power costs about 1.65 credits a unit, and a current converter
  1.5. Power from a recharger or a power block costs nothing beyond the nightly power fee
  (computed, §1.4).
- **The nightly power service runs only the chargers.** Machines draw from their own cells
  whether or not the service is paid. The fee (10 + 2 × ⌊day/10⌋ on hard) therefore pays for the
  recharger and power blocks and nothing else that makes money (§1.1).
- Ranked by margin per credit of capital on hard: a power block (16 a night, or 32 with
  Commercial Power, for 300), then a recharger (12 or 24 a night for 225). The dividends come
  well behind: chemist ~35, pharma ~36 and brewer ~21 a night, each for 2,000–3,750. The breeder
  cell is last. Gambling loses about 40 % of every credit staked (§3).

## 1. Inventory table

Status: **released** = registered and sold or given in normal play; **unreleased** = registered
or coded but no seller, caller or perk reaches it (see §4).

### 1.1 Production chains

| Class / helper | Item id | Makes | Power | Obtained | Status | Covered by |
|---|---|---|---|---|---|---|
| `MachineHydroponic`, `HydroponicHelper`, `PlantGeneticsHelper` | `hydroponic` | seeds + water → berries, caps, kotton, coffee | 8/night | retired farmer, Dr. Jackson | released | [`PRODUCTION.md`](PRODUCTION.md) §1, [`WATER.md`](WATER.md) §2.3 |
| `MachineDnaManipulator` | `plant_dna_manipulator` | moves seed traits | 12 | `PLANT_DNA` upgrade | released | [`WINE.md`](WINE.md) §3 |
| `WineHelper`, `AgableHelper` | `wine_bottle` | berries + water + yeast → wine; aging +3 value/night | — | retired winemaker | released | [`PRODUCTION.md`](PRODUCTION.md) §2, [`WINE.md`](WINE.md) |
| `MachineWineRack` | `wine_rack` | extra aging steps | 24 | Dr. Jackson | released | [`WINE.md`](WINE.md) §4 |
| `WineDirectory.YeastBioreactor` | `wine_yeast_infinite` | reusable yeast | — | retired winemaker / Jackson | released | [`PRODUCTION.md`](PRODUCTION.md) §2, §9 |
| `MachineFurnace`, `IngotPurityHelper` | `furnace` | scrap/ore → ingots | 8 (16 blast) | Jackson | released | [`PRODUCTION.md`](PRODUCTION.md) §3 |
| `MachinePrinter` | `printer` | chip + material → goods | 8 | `RETIRED_GUNSMITH` | released | [`RECIPES.md`](RECIPES.md) §1 |
| `MachineDesequencer` | `desequencer` | blank card → keycards | 10 | Jackson | released | [`RECIPES.md`](RECIPES.md) §2 |
| `MachineBottlePrinter` | `bottle_printer` | water → bottles | 4/6/11 per press | start kit, Jackson | released | [`RECIPES.md`](RECIPES.md) §3 |
| `MachineFridge` | `fridge` | adds chilled/frozen value features | 2+ | Jackson | released | [`RECIPES.md`](RECIPES.md) §8 |
| `MachineRAS`, `AquacultureHelper`, `IndustrialFishFeedHelper` | `ras` | fish growth, roe, fillets, eggs; electric eels also charge cells (`AquacultureHelper` tank power output) | 5 | `AQUACULTURE` | released | [`AQUACULTURE.md`](AQUACULTURE.md) |
| `HusbandryHelper`, `AnimalCage`, `RatTrap`, `MachineFeedDispenser`, `MachineAutoButcher` | `animal_cage`, `rat_trap`, `feed_dispenser`, `auto_butcher` | rats → meat | 0/—/… | `RETIRED_RANCHER` | released | [`HUSBANDRY.md`](HUSBANDRY.md) |
| `MachineBlender`, `MachineHeatingPlate`, `MachineEvaporator`, `MachineChemFinisher` | `blender`, `heating_plate`, `evaporator`, `chem_finisher` | drugs, pills | 0/0/6/0 | `RETIRED_CHEMIST` | released | [`CHEMISTRY.md`](CHEMISTRY.md) |
| `MachinePurifier`, `MachineMoistureFarm`, `IngesterHelper`, water filters | `water_purifier`, `moisture_farm`, … | cleaner water, condensed water | 4 / 6 | `WATER_TRADER`, Jackson | released | [`WATER.md`](WATER.md) |
| `VendingFountainHelper`, `VendingMachineHelper` | `vending_fountain`, `vending_machine` | night sales channel | — | upgrade / perk | released | [`DAY.md`](DAY.md), [`WATER.md`](WATER.md) §2.2, [`PERKS.md`](PERKS.md) |
| `MachineRecharger` | `recharger` | grid power → cell charge | uses store power | Jackson | released | this file §2.2 |
| `FurnitureItemDirectory.CreatePowerBlock` | `powerblock` | grid power → charge in adjacent cells | uses store power | Jackson | released | this file §2.3 |
| `MiscItemDirectory.EnergyCreditBreeder` | `energy_credit_breeder` | +4 charge to itself every night | none | Jackson (8 %), rare merchants, Revolution | released | this file §2.4; [`MODULES.md`](MODULES.md) §3.2 |
| `ReactorHelper` | `makeshift_reactor` | charge for every cell; leaves nuclear waste | — | starting perk | released | [`MITIGATIONS.md`](MITIGATIONS.md) §7 |
| `ShareholderHelper` | (Wild upgrades `*_SHAREHOLDER`) | credits → weekly goods + extra wholesale discount | — | Wild network | released | this file §3 (miner also [`PRODUCTION.md`](PRODUCTION.md) §3) |
| Georgio loan (`StoreClientUniqueList.CreateGeorgioLoan`) | — | credits → 4× credits, once | — | story, day 43 | released | [`MITIGATIONS.md`](MITIGATIONS.md) §1 and Mortgage section |
| `ScavHelper` | — | night outing → loot | — | door | released | [`PRODUCTION.md`](PRODUCTION.md) §6 |
| `DebrisHelper` | `debris_pile1/2/3` | piles → scrap and dump loot | — | Hoarder perk | released | [`RECIPES.md`](RECIPES.md) §5, [`PERKS.md`](PERKS.md) |
| `MachineBrokenHelper` | broken alarm/furnace/moisture farm | repair or strip for parts | — | scrapper | released | [`RECIPES.md`](RECIPES.md) §5 |
| Material upgrades (`StartingPerkContent.ConsumeUpgradeMaterial`) | — | material value stored in machine | — | — | released | [`RECIPES.md`](RECIPES.md) §6 |
| `LotteryHelper`, `ScratcherHelper` | `lottery_ticket`, `scratcher`, `scratcher_stack` | credits → prizes | — | lottery rep (`CreateLotteryRep`), Jackson (scratchers), Gambling Addiction perk | released | this file §4 |
| `AppraisalHelper` and friends | — | appraisal fees | — | Appraisal Board | released | [`APPRAISAL.md`](APPRAISAL.md) |

### 1.2 Support machines (no output of their own)

| Class | Item id | Role | Power | Obtained | Covered by |
|---|---|---|---|---|---|
| `MachineTurboBoosterAdv`, `ToolDirectory` | `turbo_booster`, `turbo_booster_adv`, `turbo_booster_hypertech` | one extra machine cycle on demand | 5/night to recharge (adv) | Jackson, Revolution, Lou gift, hypertech smuggler | [`PRODUCTION.md`](PRODUCTION.md) §4, [`MODULES.md`](MODULES.md) §3.3 |
| `CurrentConverterHelper` | `current_converter` | runs a machine on cash, 1.5 cr/unit | — | Jackson | [`MODULES.md`](MODULES.md) §3.2 |
| `HypertechBatteryHelper` | `energy_credit_hyp` | endless power for one machine drawing ≤ 12 | — | hypertech smuggler (after buyout) | [`MODULES.md`](MODULES.md) §3.2 |
| `MachineProjector` | `mirage_projector` | store attractiveness | 16 | Jackson from day 10 | [`WINDOW.md`](WINDOW.md), [`MODULES.md`](MODULES.md) §3.1 |
| `MachineAlarm` | `security_alarm` | burglary stop; transmitter module feeds Security rep | 4 | Jackson | [`MODULES.md`](MODULES.md) |
| `HopperHelper` | `intake_hopper`, `outlet_hopper` | moves items between machines between night passes | — | Jackson from day 21 | [`PRODUCTION.md`](PRODUCTION.md) §0 |
| `MachineStorageCoordinator` | `storage_coordinator` | storage automation | — | Jackson after day 14 (40 %) | none: no production effect (inferred) |
| `MachineCassettePlayer`, `CassetteHelper` | `cassette_player` | music only: no value, rep or traffic hook found | — | fixed store furniture (`EmporiumEntry`) | none needed (inferred) |
| `MindReprogrammerHelper` | `mind_reprogrammer` | edits starting perks | — | hypertech smuggler | [`PERKS.md`](PERKS.md) §1 |
| `LabelerHelper` | `labeler` | relabels conditions (sales trick) | — | Jackson | [`LABELER.md`](LABELER.md) |
| `MachineDealMaker` | `deal_maker` | bargain bonus on button press | 25/press | **no seller** | §5 |

## 2. Power generation

### 2.1 What the power service actually does

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Store power flag | `isPowerOn` = power service in use, or any Power Hijack tier unlocked. With the Makeshift Reactor perk it comes from the reactor instead | HIDDEN (flag); VISIBLE (dim store) | `PlayerStore.UpdatePowerState()` |
| What reads the flag | **only** the recharger, the power block and the darkness overlay. Machines take their nightly draw straight from the cell in their battery slot (`MachineHelper.TryDrawCyclePower`), which never checks the flag | VISIBLE (the service tooltip says cell machines keep running when it is unpaid) | grep of `isPowerOn`: `FurnitureItemDirectory.CreatePowerBlock`, `MachineRecharger.Recharger`, `PlayerStore.HandlePower/LoadPower` |
| Fee | (⌊day/10⌋ + 5) × `powerCostMultiplier` per night: 5 + ⌊day/10⌋ on cozy/normal, **10 + 2 × ⌊day/10⌋ on hard** (10 to day 9, 20 at day 50, 30 at day 100). The start bonus (+3 on hard) is added once in `ApplyDifficultyStart` and overwritten the first time the cost is recomputed (inferred) | VISIBLE (services panel) | `StoreService.UpdateCost()`, `StoreService.ApplyDifficultyStart()` |
| Free | 0 during a station blackout, with any Power Hijack tier (300, 450 on hard), or with the reactor perk | VISIBLE | same |
| Commercial Power | Wild upgrade 350 (525 on hard): doubles the recharger (3 → 6) and power block (2 → 4) rates. Hidden in reactor runs | VISIBLE (upgrade text gives +100 %) | `NetworkUpgrade.IsCommercialPowerUnlocked()`, `NetworkUpgrade.IsHiddenForRun` |
| Blackout | `PowerHelper.ChargeBattery` refuses to charge while a blackout affects the player, so chargers idle that night (electric eels pass `ignoreBlackout`) | HIDDEN | `PowerHelper.ChargeBattery(GameItem, int, bool)` |

So the fee buys charger output and light. With no charger it buys nothing that earns money.
Leaving the service off would save 10–30 a night on hard if dimmed lights carry no hidden
penalty (inferred, lab §6 item 3).

### 2.2 Recharger (`recharger`, unitValue 150)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Capacity | 4×2 grid; cells are 1×2, so **4 cells** | DERIVABLE (window) | `MachineRecharger.CreateMachineInventoryWindow`; shapes `battery*` 1×2, `recharger_base` 4×2 (`var/gamedata/item_shapes.json`) |
| Accepts | owned, non-disposable power sources. Current converters and hypertech cells refuse it. A breeder cell is accepted and gets both charges | VISIBLE (drop refused) | `itemInventory.mayInventoryAddItemFunc`; `CurrentConverterHelper.InitCurrentConverter`, `HypertechBatteryHelper.InitHypertechBattery` |
| Rate | **+3 per cell per night**, +6 with Commercial Power, capped at the cell's max; main night pass; only while `isPowerOn` | HIDDEN (description gives no number); DERIVABLE from cell readouts | `Recharger()` cycle lambda, `PowerHelper.ChargeBattery` |
| Output | 12 units a night (24 with Commercial Power) when all 4 cells have room | DERIVABLE | (computed) |
| Seller | Dr. Jackson, in the store and at night: 66 % a visit while none is owned, 25 % once one is | — | `StoreClientList.PlaceInventorInventory` |
| Price | 150, 225 on hard | VISIBLE | `inventorMarkup` |

### 2.3 Power block (`powerblock`, unitValue 200, 2×2)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Effect | Early night pass: every item edge-adjacent in the same grid gets **+2** (4 with Commercial Power). A loose cell (`power_source_item`) is charged directly, and a machine with a battery slot (`MACHINE_RECHARGABLE_TAG`) has its slot cell charged. The recharger has no battery slot, so it gets nothing | VISIBLE (description gives +2, stacking and the recharger exception) | `FurnitureItemDirectory.CreatePowerBlock`, `MachineHelper.RechargeMachineBattery`, `MachineHelper.SetupBatterySlot` |
| Adjacency | Edge contact only: the item's shape shifted one cell left, right, up or down must overlap. Diagonals don't count | HIDDEN | `GameItem.FindAdjacentType`, `GameGridInventory.IsAdjacent` |
| Neighbours | A 2×2 block has 8 edge cells. With 1×2 cells, up to **8 cells** fit around it (2 above, 2 below, and 2 on each side placed offset) (computed; lab §6 item 2) | DERIVABLE | shapes |
| Stacking | Several blocks touching one cell each add +2 | VISIBLE | description |
| Early pass | Charges before machines draw in the main pass, so a machine beside a block draws 2 (4) less from its own cell each night | HIDDEN | `PlayerStore.EndNight()` pass order (`PRODUCTION.md` §0) |
| Output | Up to 16 a night (32 with Commercial Power) for each block | DERIVABLE | (computed) |
| Seller | Dr. Jackson, **75 % every visit**, even when one is owned, so several can be bought | — | `PlaceInventorInventory` |
| Price | 200, 300 on hard | VISIBLE | |

### 2.4 Breeder cell (`energy_credit_breeder`, base 400 + charge, max 100)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Self-charge | +4 a night in the Early pass, capped at 100. Needs neither store power nor a blackout exemption | VISIBLE (description says it self-charges; the rate is not shown) | `MiscItemDirectory.EnergyCreditBreeder` |
| Extra charging | Takes recharger (+3) and power block (+2) charge on top | HIDDEN | `PowerHelper.ChargeBattery(GameItem, int, bool)` excludes only disposable, converter and hypertech cells |
| Sale value | 400 + charge (500 full) | DERIVABLE | `PowerHelper.GetPowerSourceItemValue` |
| Sellers | Dr. Jackson 8 % a visit, 2 in special stock (afterhour event `special_stock`); the lower-level rare merchant, the broke upper-level seller, barter and exchange offers, and the Revolution quartermaster (stolen) | — | `StoreClientList.PlaceInventorInventory/PlaceInventorSpecialStock/CreateLowerLevelRareMerchant/CreateBrokeUpperLevel`, `StoreClientListRev`, `BarterOfferList`, `ExchangeList` |

### 2.5 Turning charge into credits

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Cell value | `energy_credit`: 10 + charge (0–100); `energy_credit_ext`: 30 + charge (0–250). Charge enters the price before the innate and market multipliers | VISIBLE (price) | `PowerHelper.GetPowerSourceItemValue`, `GameItem.ValueModifier` |
| Buyers | `ENERGY_CELL` buyers: lower repairman, scrapper and maintenance worker (budgets 80–125), lower inventor (145–295 + day/4), station engineer (750–1000 + day), and the power-event buyers (lower levels 150–200, upper levels 325–400). Dr. Jackson does **not** buy cells | VISIBLE (wants panel) | `StoreClientListTier.CreateLowerRepairman/CreateScrapper/CreateMaintenanceWorker/CreateLowerInventor/CreateStationEngineer`, `StoreClientListEvent.CreatePower*Buyer` |
| Events | `powerShortage` +66 %, `stationWideBlackout` +50 %, `powerPlantMaintenance` +33 % to `ENERGY_CELL` (market %) | VISIBLE (calendar) | [`PRODUCTION.md`](PRODUCTION.md) §5 |
| Buying power on hard | Jackson sells a full cell (110 value) for 165, an empty one for 15 and a full ext cell for 420. Each unit of charge therefore costs **1.65** (computed). A current converter costs 1.5 a unit (1.0 with 5 upgrades). A disposable cell holds 1,000 units for 331 + contraband pricing, about 0.5 a unit before contraband, but it is contraband 1 and is sold only in special stock | VISIBLE | `ItemFeatureList.InventorMarkup` (market stage), `PreBuiltItemHelper.EnergyCredit/EnergyCreditExt`, `MiscItemDirectory.DisposableBattery` |
| Empty cells | Supply is thin: Jackson stocks one empty cell, one ext cell and one or two full cells a visit | — | `PlaceInventorInventory` |

### 2.6 Margins on hard (computed)

Each unit of charge is worth 1 credit if the cell is sold, and 1.65 if it replaces power that would
otherwise be bought from Jackson. The capital column includes the empty cells at 15 each.

| Setup | Capital (hard) | Units / night | Sold / night | Replaces Jackson power / night | Payback (sold) |
|---|---|---|---|---|---|
| Recharger, 4 cells | 285 | 12 | 12 | 19.8 | 24 nights |
| Recharger + Commercial Power | 285 + 525 | 24 | 24 | 39.6 | 34 nights |
| Power block, 8 cells | 420 | 16 | 16 | 26.4 | 26 nights |
| Power block, 8 cells + Commercial Power | 420 + 525 | 32 | 32 | 52.8 | 30 nights |
| Each further power block (CP owned) | 420 | 32 | 32 | 52.8 | 13 nights |
| Breeder cell | 750 (resells for 500) | 4 | 4 | 6.6 | 63 nights on the 250 lost to markup |
| Power fee (only a cost if the lights could otherwise go off) | — | — | −10 to −30 | — | Power Hijack I (450) pays back in 15–45 nights |

Notes: a 100-unit cell fills in 34 nights at 3 a night, so cells have to be sold or moved into
machines to keep a charger busy. Selling cells during a `powerShortage` adds 66 %. A cheaper use
than selling is to place the power blocks between machines, which feeds them in the Early pass
and replaces bought power at 1.65 a unit.

## 3. Shareholder dividends (`ShareholderHelper`)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Purchase | Wild upgrade, one-time; needs the matching introduction | VISIBLE | `NetworkUpgradeList.CreateShareholder` |
| Costs (normal / hard ×1.5) | Brewer intro 200 + shareholder 1,450 (2,475 total on hard); Chemist 250 + 1,440 (2,535); Pharma 300 + 2,500 (4,200); Miner 250 + 900 (1,725) | VISIBLE | `NetworkUpgradeList.*_SHAREHOLDER_COST`, `CreateBrewer/Chemist/Pharma/Miner`, `NetworkUpgrade.GetCost()` |
| Dividend day | Night pass when `dayCounter % 7` equals: brewer 4, miner 5, chemist 6, pharma 0. Goods appear owned on the counter with a night-log line | VISIBLE (night log) | `ShareholderHelper.HandleDividends()` |
| Goods | Brewer: 1 `beerCase` holding 8 `red_beer` (18 each, case 1) = 145. Miner: 6 `common_ore` (15) = 90. Chemist: 8 `dream_dust` (18, contraband 2). Pharma: 5 `pink_injector` (50, legal) = 250 | DERIVABLE | `ShareholderHelper.dividends`, `PreBuiltItemHelper.FullBeer`, `var/gamedata/items.json` |
| Dust price | 18 × 1.70 (contraband level 2 markup) ≈ 31 each, so ≈ 245 a week before any grade feature. Selling it is a contraband crime | DERIVABLE | [`CHEMISTRY.md`](CHEMISTRY.md) §7.1 price rule |
| Wholesale | The supplier's courier wholesale discount grows by a further 15 points (e.g. −20 → −35 %) | VISIBLE (wholesale offer) | `ShareholderHelper.WholesaleDiscount` |

Margin on hard, counting only the shareholder upgrade because the introduction is usually
already owned for the supply line (computed):

| Shareholder | Upgrade cost (hard) | Dividend / week | Per night | Payback |
|---|---|---|---|---|
| Chemist | 2,160 | ≈ 245 (dust at market, contraband) | ≈ 35 | ≈ 62 nights |
| Pharma | 3,750 | 250 | 35.7 | 105 nights |
| Brewer | 2,175 | 145 (more if chilled, `RECIPES.md` §8) | 20.7 | 105 nights |
| Miner | 1,350 | 90 raw ore (≈ 2 blast ingots) | 12.9 | 105 nights |

The extra 15 % wholesale discount adds 15 % of whatever is bought from that courier. Buying the
miner's 12-ore lot (180 base) every 5 days saves about 5 a night (computed from `PRODUCTION.md` §3). None of
these pays back in under about 60 nights, so they suit a run that has already passed the
buyout-price checkpoints. They are not an early-hard play.

## 4. Gambling (not a chain)

| Rule | Value | Vis. | Evidence |
|---|---|---|---|
| Lottery | Ticket 25. Six numbers from 1–49, weekly draw. Prizes by matches: 2 → 35, 3 → 300, 4 → 2,500, 5 → 100,000, 6 → 1,000,000. Unlocked after day 14 (day 1 with Gambling Addiction) | VISIBLE | `LotteryHelper` consts |
| Lottery EV | **14.27 per 25 staked (−43 %)** at luck 0 (computed, hypergeometric) | DERIVABLE | |
| Luck | |luck| % chance of one extra draw (+1 per full 100). With positive luck the draw that pays the most is kept. Effective luck is the best of player luck and each ticket's spawn luck | HIDDEN | `LotteryHelper.DrawWinningNumberWithLuck`, `GetExtraDrawCount` |
| Scratcher | 20 each (stack of 5 for 100). Three symbols, each matching the favoured one with probability 0.2 × (1 + luck/100). Pairs pay 10, or 30 for the nutrifruit symbol; triples pay 75, or 350 | VISIBLE (on scratch) | `ScratcherHelper.SetRandomSymbols/RollSymbol/UpdateValue` |
| Scratcher EV | 11.9 per 20 at luck 0, 14.7 at luck 15, 17.8 at luck 30 (computed by simulating the roll code, 400 k draws). Still a loss at Lucky + gambler's coin (20) | DERIVABLE | |

## 5. Unreleased or unreachable (one line each)

- `deal_maker` (`MachineDealMaker`, 400, 25 power a press, +10 + quality/10 % bargain chance) is
  registered and has a prebuilt helper (`PreBuiltItemHelper.CreateDealMaker`) that nothing
  calls. It appears in no seller, loot table (`var/gamedata/loot_tables.json`) or client data.
- `MachineMeatCulture.AutomaticMeatCultureLab`, `UnusedDirectory.AutomaticMeatProcessor*`,
  `CreateRecycler`, `CreateArmorConditioner`, `AmenitiesDispenser`, `WaterDispenser`,
  `CreateSpeaker`, the crafting table and the test vending machine have no caller.
  `box_dispenser` and `test_gizmo` are registered with no seller. See also `RECIPES.md` §9.
- Organs (`OrganHelper`, `organ_*`, `organ_transport_container`) exist only in
  `UnusedDirectory` with no seller or buyer (grep of `StoreClient*`, drop and barter tables).
- Expeditions (`Expedition`, `ExpeditionHelper`, hirelings): unlocked only by the
  `expedition_organizer` perk, which is absent from `var/gamedata/starting_perks.json`, and
  `Expedition.ExecuteExpedition` has no caller.
- Ship and space systems (`ShipHelper` machinery, `MiningHelper` asteroid mining, `PlasmaHelper`
  reactors, `Space*Trigger` events, `SalvageHelper`) belong to the legacy ship prototype that
  `MenuManager` scenarios use. The shop loop never reaches them (inferred). `plasma_fuel` is only
  an appraisal collectible ([`INSPECT.md`](INSPECT.md)). `hypertech_bay` is released but is
  storage, not production.

## 6. Lab experiments

| # | Experiment | Expected |
|---|---|---|
| 1 | Recharger with 4 empty `energy_credit`s, power service on, one night; repeat after Commercial Power | each cell 0 → 3, then +6 a night; a 5th cell does not fit |
| 2 | Power block with 1×2 cells on all four sides (offset on the sides); separately, one cell touched by two blocks | 8 cells at +2 each; the shared cell +4; a diagonal cell +0 |
| 3 | Turn the power service off for a night with a furnace (charged cell), hydroponic bay and recharger running | furnace smelts and bay grows as usual; recharger and power block add 0; customer count and attractiveness unchanged next day (confirms the fee buys only charging) |
| 4 | Power block beside a furnace whose cell holds exactly 6 | Early pass +2 → 8, so the smelt succeeds (8 drawn) and the cell ends at 0 |
| 5 | Breeder in a recharger during a blackout night | breeder +4 (Early, ignores blackout); recharger adds 0 |
| 6 | Sell a full `energy_credit` to a maintenance worker, normal day vs `powerShortage` | 110 vs ≈ 183 on hard (no retail markup) |
| 7 | Chemist shareholder: check the dividend dust's purity and the price a shady buyer offers | 8 dust on the counter on a day with `dayCounter % 7 == 6`; price ≈ 31 each before grade |
| 8 | Buy 50 lottery tickets at luck 0, then again at Lucky + coin (20) | about 14 back per ticket at luck 0; at luck 20 a 20 % chance of one extra draw, worth at most ~17 per ticket for a single ticket and less per ticket on a large lot (the extra draw maximises the lot's total) |

## Verification

Read against the decompile in `var/decomp/` and the extracted tables in `var/gamedata/`
(`item_shapes.json`, `items.json`, `loot_tables.json`, `starting_perks.json`, `loc_en.json` for
which numbers the UI shows). Nothing in this file has been measured in the gym yet.
