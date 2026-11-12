# Who buys what (RE notes)

Build 0.46D, same conventions as `DAY.md` and `WINDOW.md` (global namespace of `Assembly-CSharp`).
Read from code; the gym measurements at the end are labelled as such. Questions answered: how a
client's wants are set, how a buyer takes what is staged, how the window changes who comes, and which
categories each client type buys.

## How a client's wants are set

| Rule | Evidence |
|---|---|
| Wants are **fixed per client type**, not rolled. Each `StoreClientList*.Create…()` factory adds constant entries to `clientBuyingTagList` (types/tags), sometimes `clientBuyingIdList` (exact item ids), `clientBlackTagList` (refused types) and a `willAcceptItem` predicate. Only the budget is random (a range plus a day term) | `StoreClientList.CreateSpacerClientFood`, `CreateMercBuyer`, …; `StoreClientListTier*.cs` |
| The tablet shows as "interests" only the `clientBuyingTagList` entries that are in `TypeHelper.ALL_TYPES_LIST` (`ALL_TYPE_EXCEPT_CONTRABAND` shown as all types), and as restrictions the `clientBlackTagList` entries in that list. `clientBuyingIdList` and `willAcceptItem` are never shown, so some buyers show no interests at all (the SUBSTANCE window buyers, the water buyers) | `NegociationUIManager.BuildInfoBlock` |
| A buyer accepts an item when it is not refused and (all types, or its id is listed, or it carries a listed tag, or `willAcceptItem` says yes) | `StoreClient.IsClientBuyingThisItem` |
| Refusals: `NOT_FOR_RESALE`, a black-listed id or type, contraband for a no-contraband buyer, an id already bought by a variety buyer, a narcotic under the buyer's purity grade, hazardous waste or a hazardous water label, a luxury item for a water buyer, and a container any of whose contents the buyer would not take | `StoreClient.IsClientRefusingItem` |
| Variety buyers (`SetVariety`): take at most one of each item id, and every owned item placed on the counter while they are there gets a visible +15 value modifier (`varietyBuyerBonus`, Market stage) | `StoreClient.SetVariety`, `IsItemInVarietyList`; `ItemFeatureList.VarietyBuyerBonus` |
| `UL_TOP_REVIEW` (UL perk) adds 25 % to a budget | `StoreClient.ApplyBudgetModifier` |

## How a buyer takes what is staged

| Rule | Evidence |
|---|---|
| "Sell all" offers every owned item on the back counter the buyer accepts. If their sum fits the budget, each is sold in turn (from the last placed backwards); otherwise the game asks for confirmation | `NegociationUIManager.OnSellAllClick`, `GetSellableBackInvItems`, `IsClientHaveEnoughToBuy` |
| Per item, before the sale the buyer may re-inspect it and expose a fake (no sale, rep loss); then it must be wanted, not exposable, within the remaining budget (unless confirmed over budget), and new to a variety buyer | `NegociationUIManager.SellMultiple`; `PlayerStore.TryReinspectBeforeSale`, `CanSellThisItem` |
| Payment is the negotiated price capped at the remaining budget; the budget drops by what was paid. Confirmed over budget, every item still changes hands, so items after the budget runs out are given away for 0 | `PlayerStore.SellItem` |
| So the buyer does not choose: the player chooses what to stage, the budget caps the total, and the over-budget tail is lost | — |

## How the window changes who comes

`WINDOW.md` has the full rules. In short: once a day at shutter open (`StoreClientManager.GenerateClient`),
the top 1–4 major types in the case by displayed value each bring one display buyer of that type
(`ShowcaseHelper.GetDisplayCaseType`, `CreateClientByStockType`), in addition to the normal customers. The
type's real value in the case sets the tier (145 → tier 2, 350 from day 14 → tier 3; attractiveness 400
also lifts tier 1 to 2). Signs and permits in the case add predicted named visitors that use up normal
customer slots. The case does not change the normal customer count or which normal clients are drawn.

## Normal customers

`HandleNormalClient` first rolls the case's predictions, then fills the rest of `GetNormalCustomerCount()`
(0–6: +1 each at attractiveness 200, 250, 500 unless vandalized; ±1 for some perks; −1 after an outing
unless short_sleeper) with `PickClient`. `PickClient` rolls the faction pools in order (black market, upper
level, revolution; each its own % chance from rep, `GetBlackMarketClientChance` 4–17, `GetUpperLevelClientChance`
5–15, `GetRevolutionClientChance` 0–9) and otherwise draws from the lower-level general pool, weighted
by the attractiveness tier (`GetAttractivenessTier`: 0 below 400, 1 from 400, 2 from 650;
`RandomClientPool.Pick`). Normal clients start on day 14 with the intro, day 2 with it skipped.

General pool (weights at tier 0; a few fall at higher tiers):
| Client | Intent | Wants (visible) | Budget |
|---|---|---|---|
| merchant (100) | buy | everything but contraband | 200–300 + 3/day |
| spacerFood (100) | buy | FOOD | 35–50 + 2/day |
| spacerFoodVariety (100) | buy, variety | FOOD | 100–120 + 2/day |
| spacerAmenityVariety (100) | buy, variety | HOUSEHOLD_GOOD | 50–85 + 2/day |
| mercenaryBuyer (100) | buy | WEAPON, ARMOR, AMMUNITION (refuses LUXURY_ITEM) | 300 + 2/day |
| foodBankBuyer (50) | buy | FOOD | 150–200 + 2/day |
| miner (100) | sell and buy | SUBSTANCE | ~65–75 |
| lowerLevelChemist (150) | sell and buy | CHEMICAL_SUPPLIES | 75–125 |
| junker (100; 80, 60) | sell and buy | SUBSTANCE | 50 + 1/day |
| sellers: scavGeneral 200, scavCrate 150, waterSeller 100, scavBlood 100 (66, 33), scavInspectable 100, scavStamp 70, shadyPharma 50; barter: addict and food barter 75 each (30, 1) | sell | — | — |

Faction pools: black market — thief, pettyThief, foodThief (sellers) and shadyMerchant (buys CONTRABAND,
ACCESS_CARD; 250–300 + 3/day); upper level — upperLowerVisitor (SUBSTANCE, 250–300 + 2/day),
upperLowerVisitorLuxury (LUXURY_ITEM, same budget), cluelessTouristBuyer (FOOD, WATER, WATER_RATION,
SUBSTANCE; 180–250), brokeUpperLevel (seller); revolution — revolutionaryBuyer (WEAPON, MUNITION,
AMMUNITION, MEDICAL; 150–200 + 2/day).

Display buyers (`StoreClientListTier*.cs`; tier 1 → 2 → 3):
| Case type | Tier 1 (budget ~80–150) | Tier 2 (145–195 + day/4) | Tier 3 (400–1000 + day) |
|---|---|---|---|
| FOOD | spacerFoodI, hungrySpacer, spacerChef (variety) | expeditionSupplier | privateChef (400–500) |
| MEDICAL | spacerMedical, sickChildCaretaker, lowLevelDoctor (variety) | clinicNurse | hospitalAdmin (750–1000) |
| MATERIAL/TOOL/ENERGY_CELL/MODULE | lowerRepairman, scrap_byer, maintenanceWorker (variety); wants MATERIAL, TOOL, ENERGY_CELL | lowerInventor (+MODULE, NODE; 145–295) | stationEngineer (750–1000) |
| HOUSEHOLD_GOOD | spacerHouseholdItem, youngSpacerHousehold, familyMother (variety) | citizenHousehold | upperMaid (500–650) |
| SUBSTANCE | desperateAddict | wornOutSpacer | upperLevelHedonist (350–435) |
| WEAPON/ARMOR/AMMUNITION | scavenger | selfDefenseEnthusiast | privateSecurityContractor (500–700) |
| WATER | thirstySpacer, scavWaterShopper | waterSpace2 | waterStockpiler (500–600) |
The SUBSTANCE and WATER display buyers show no interests: they buy through `willAcceptItem` (SUBSTANCE, or a
narcotic at or above a purity grade) and water rules.

## What the gym measures against this (hard, best policy, 60 seeds 1400001–060)

The executor (`-start apprentice -perks vending+short_sleeper`, `hard_income_mortgage.json`) sees about 5
buyers a day on days 31–100 and 6 on days 100–200, offering 1.4k and 2.6k of budget; it sells 29 % and 19 %
of it. 41–45 % of buyers leave with nothing staged. See `docs/PROGRESS.md` "Stock breadth" for the
breakdown by want and by why nothing was staged.

## The wanted list (reporting)

Measured with `fence-lab barter.wanted` (hard, docs/sim/VERIFIED.md) and the gym (`appraiser` route, hard,
seeds 2910001–009); the rest from code (`PlayerStore.OnCustomerReported`, `GrantWantedCaptureReward`,
`ApplyReportFactionChange`, `StoreClient.CanReport`).

| Client id | Bounty | Faction | Report changes that faction | Gym visits a run (all dismissed today) |
|---|---|---|---|---|
| `wanted1` (+ `_return`) | 300 | Lower Levels | −6 (the report did not go through headless: a storyline intercept, `tryInterceptReport`) | 1 + 3 |
| `wanted2` (+ `_return`) | 450 | Lower Levels | **+6** (improves) | 1 + 8 |
| `wanted3` | 100 | Revolution | −6 (the report did not go through headless; not measured) | 1 |
| `wanted4` | 300 | Revolution | −6 | 1 |
| `wanted5` | 1 500 | Black Market | none; **but** reporting her starts the Lou chain, which ends in a demand of 1 000 on hard and a game over if underpaid (`LouBombHelper`, `LouOperativeHelper`; read, not measured) | 1 |
| `wanted6` (+ `_supply`) | 400 | Lower Levels | −6 | 1 + 9 |
| `wanted7` | 125 | Lower Levels | −6 | 8 |
| `wanted8` (+ `_weekly`) | 125 | Lower Levels | −6 | 1 + 7 |
| `wanted9` | 300 | Lower Levels | none (penalty 0) | 1 |

- A correct report pays the bounty when the alarm's officer leaves, +5 Security, closes the poster, and applies
  the faction change. Difficulty scales the reputation: measured on hard, `wanted8_weekly` gave +125 cash,
  **+3.75** Security (gain × 0.75) and **−7.5** Lower Levels (loss × 1.25); `wanted9` +300 / +3.75 / 0.
- Shooting a wanted client (`OnWantedClientShot`) pays the same bounty and faction change as reporting it.
- Not reportable: after a bargain coercion, or a client marked not arrestable / not dismissable.
- The gym's executor dismisses every wanted visit (`BarterRules.CoverageGap`): 26–59 visits a run (mean 46 over
  9 runs). Bounties on first visits sum to ~3.6k (~2.1k without `wanted5`); reporting also ends the return visits.
