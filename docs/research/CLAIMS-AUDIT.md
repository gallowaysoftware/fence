# Claims audit: negative and absolute claims checked against the source (2026-11-12)

Prompted by the owner catching that docs/re/ARMS.md's "modding never adds resale value" read the formula right and
drew the wrong lesson. Method: every "never / no value / not worth / doesn't pay / can't / unreachable / breaks even"
claim in docs/re and docs/research (75 hits), each strategy-shaping one re-read against the decompiled source.

## Wrong or misleading

| Claim | Where | What the source says | Consequence | Fix owner |
|---|---|---|---|---|
| "A gun's value is its own plus each mod's, so modding never adds resale value" | re/ARMS.md §mods | A mod's value carries into the gun; customers ask for mods: service orders (half of Mid/High clients: fee 30/60/100 + bonus rows + your parts at value, `GunHelper.CreateServiceOrder`) and min-stat orders (bonus row per stat strictly above its threshold). | The armourer engine treated mods as worthless and filled almost no service orders: a mod-supply problem, not a value one. | formula sweep (weapons pass) |
| "Repair never pays at resale; it pays through gun orders" vs "repair pays on every gun above ~200" | research/HIDDEN-SYSTEMS.md vs re/RECIPES.md §1.6 | Both half right. A step is worth ~12.5 % of the gun's value; it costs 25 × expected parts, and expected parts = 1 for 100-repairability guns (printed, handmade) and ~1.85 for SMG (40). Repair pays at resale above ~200 of value for printed/handmade, ~370 for an SMG, and never on New/Excellent. | Repair as a resale step on valuable guns was dropped. | formula sweep (weapons pass) |
| "Aging still does not pay in the gym" | re/WINE.md §6 | PRODUCTION-VERDICTS.md: the loss came from the hold refusing buyers (`KeepWhy`), an executor artifact; aging adds +3 × grade a night, uncapped. | Stale doc steers away from a measured gain. | water-cellar |
| "the lab cannot reach 24 a day whatever the bays grow" (alcohol bound) | research/CHEMISTRY-ENGINE.md | Alcohol is a solvent: extraction keeps it (R1/R7 ratio against mush still present); it can be made (wine pour 150 ml a bottle) and recovered (evaporator distillate with the solvent-recovery module). | Wrong ceiling; the real bound is mush supply and vessel time. | chem-loop |
| `wine_extract` row: "no use in chemistry" | re/CHEMISTRY.md §1 | True of the extract itself, but the same pour adds 150 ml alcohol: wine is an alcohol source for the lab. | Reader misses the alcohol source (the alcohol row does list it). | chem-loop |
| "the cash buyout is out of reach" on challenge | research/ROUTES.md (challenge routes) | Measured with the executor's trading income only; no production engine at full grade was in that run. | Conditional, not a law: re-test once the engines land. | re-evaluation |

## Missed, not wrong

| Mechanic | Source | Value | Executor today |
|---|---|---|---|
| Fridge: chilled +20 % innate on beverages, water rations and premium bottled water (and their contents); ice-cold +25 %; nudka accepted; wine bottles refused | `MachineFridge.MayZoneAcceptItem`, `IsChillable`, `CreateChilledFeature` | +20 % on every chilled drink or ration sold, for the fridge's power | never uses the fridge |
| Plant gene lines: edits clone forward with heat cleared; production traits (caffeine 2 ml × r^0.75 per produce item) move between species | `PlantTraitList.Production`, `PlantGeneticsHelper` | possibly removes the bean wall | one trait on bloomberries, off by default (gene-lines agent) |
| Alcohol from wine and spirits; evaporator distillate (alcohol only with solvent recovery; water otherwise) | `WineHelper.PourWineIntoContainer`, `WaterHelper.PourSpirit`, `MachineEvaporator` | closes the alcohol loop; a possible Pure-water source | not used (chem-loop, water-cellar) |

## Checked and right

Live rats are worth 0 (no buyer path; sell meat). The fridge does not take meat. Ore is not worth spending on
purity for melee prints (fineness module does the same). Unreachable endings and operations (`hitman`, crackdowns)
are unreachable: their factories are never called. The labeler's negotiation-tool route is dead (no feature gets a
tool). Pharma shareholder's 3 750 does not pay back in 13 weeks (measured).
