# Arms: permits, gun orders, guns, mods, ammo and the printer (RE notes)

Build 26525552 (0.46D). Read from the decompiled source, then checked in the lab (`sim/lab/ArmsExps.cs`,
`fence-lab arms.*`; verdicts in docs/sim/VERIFIED.md, "Arms") and the gym. Numbers are the game's; the words are
ours. "Lab" marks a claim measured headless; everything else is source-only. Related: RECIPES.md §1 (printer chips),
CONCEALMENT.md (inspections), CLIENTS.md (buyers), MODULES.md (printer modules).

## 0. The short version

- **Gun-order clients come only from a permit on display.** Each weapon permit item in the display case rolls its
  own order client every day (Permit I 50 %, II 40 %, III 35 %), with the chance climbing by half the base for every
  day it missed. A permit in stock brings nobody: the executor's old window took the Gunsmith start's permit out of
  the case on day 1, so the shop saw no gun order in 70 days (gym). The in-game guide never says the permit must be
  displayed.
- **A permit covers its own contraband level and nothing else** (lab). Permit II does not make a level-1 gun legal;
  Permit III does not cover levels 1–2. You need all three. Permits arrive at a shutter opening once the Wild upgrade
  is owned and Security is at the gate (20 / 50 / 80; lab), and are lost below 0 / 30 / 60.
- **Orders pay base plus every bonus row the gun clears, strictly above the threshold** (lab), cash only, no
  reputation (lab: 0.0 over 20 deliveries). The gun and everything attached to it is destroyed.
- **A "best" order pays 1.5 times the gun's market value including its mods** (lab), the one place a mod sells above
  its estimate without haggling.
- **Mods add their own value to the gun and their percentages and flat points to its stats** (lab). Value never
  depends on stats: stats matter only to orders. A .22 extended mag (35) takes a printed gun's firepower from 19 to
  46 (lab), past every Basic and Mid firepower threshold.
- **A printed gun is legal at birth under Permit I** (lab: the Gunsmith start's old printer, 3 nights, 7 power a
  night, comes out New, estimate 50).
- **Two headless gaps** the gym now covers (VERIFIED.md): directories were built without their field initializers,
  so any mod with a per-gun allow list (every magazine, barrel, grip, stock) threw when slotted; and every item reads
  as destroyed to `GameItemElement.IsDestroyed`, so a service order's gun was never found.

## 1. The Gunsmith start (`NewGameData.HandleInitialItem`, GUNSMITH branch)

| Item | Value |
|---|---|
| Cash | −450 against the default start |
| Rent | +50 a week (the starting rent and the current one) |
| Security | +25 through `StoreReputation.ModSecReputation` (scaled by the difficulty's gain multiplier; ~18 on hard, gym) |
| Upgrades | `RETIRED_GUNSMITH` and `GUN_PERMIT_I` set owned directly: their unlock actions do not run (no extra printer, no next-day gunsmith visit) |
| Permit | `permit_gun_1` placed **in the display case** and granted |
| Printer | the old gun printer (`PreBuiltItemHelper.CreateOldGunPrinter`): shabby, a 100-charge cell, a shabby pistol chip in its slot, two stuck modules (eco and "glitched": net performance −1, efficiency +17) |
| Guns | handmade pistol ×2 (one Poor), printed gun ×2 (one Poor, one New) |
| Mods | silencer, red dot, .22 mag ×2, .22 high mag, toy laser, makeshift silencer, laser sight, improved springs |
| Other | 4 printer plastic, a shabby gun storage bay, 6 non-lethal + 5 lethal .22 rounds, a screwdriver, a gun part, the firearm sign |

With skip-intro the start also schedules a Basic gun-order client on days 1 and 4 and a gun seller on day 2
(`StoreClientManager.HandleSkipClient`). The retired gunsmith visits every day with day % 7 = 4 (days 4, 11, 18…),
because the start type alone qualifies (`StoreClientManager.HandleContentUnlockClient`).

The old printer: speed 24 a night (the −1 % truncates away), **7 power** a working night (lab), pistol chip in place
(lab). Shabby changes only its resale value (−75 %).

## 2. Weapon permits

| Permit | Wild upgrade (cost; hard ×1.5) | Prerequisite | Granted at shutter when Security ≥ | Revoked when Security < | Covers |
|---|---|---|---|---|---|
| I | `GUN_PERMIT_I` 300 (**450**) | — | 20 | 0 | level-1 FIREARM and AMMUNITION |
| II | `GUN_PERMIT_II` 600 (**900**) | I | 50 | 30 | level-2 only |
| III | `GUN_PERMIT_III` 900 (**1 350**) | II | 80 | 60 | level-3 only |

Costs measured in the lab on hard (Retired Gunsmith 975). `NetworkUpgradeList`, `StoreShutterButton.HandlePermit`,
`StoreClientManager.HandleImportantScriptedClient`, `PermitHelper`.

- **Grant** (`StoreShutterButton.HandlePermit`, every opening): with the upgrade owned and Security at the gate, the
  permit item is put **on the front counter** (owned) and its coverage applied. Lab: III owned at Security 79 is not
  granted, at 80 it is, and the item is on the front counter. The counter is cleared at night: put the item away
  (and on display, §3) or it is gone.
- **Coverage is exact-level** (`PermitHelper.OnNewItemCreated`, `GrantedFromPermit`, `IsCoveredType`): an item of
  type FIREARM or AMMUNITION (frames count as FIREARM) at exactly the permit's contraband level loses its contraband
  status and becomes `RESTRICTED_n`. Every item the game makes through `DirectoryMaster.Item` is checked, so printer
  output, finished frames and goods a seller brings are legal at birth when a permit covers them (lab: printed gun,
  frame, .22 ammo under I; heavy pistol and glock under II; SMG under III; with I revoked and II held a new printed gun
  is contraband 1).
- **Restricted, not contraband**: no crime on sale, invisible to inspections, and a buyer pays a smaller markup than
  for contraband: +17 / +35 / +50 % (half of +35 / +70 / +100, `ItemFeatureList.RestrictedLow/Mid/HighMarkUp`, added
  when the item goes on the sale table).
- **Revocation** (`StoreClientListSecurity.CreateRevokePermitOfficer`): a Security officer comes on any day a held
  permit's floor is broken, one per failing permit. He reverts only FIREARM: restricted guns turn contraband again
  (lab), the permit item is destroyed, Permit I's loss takes the counter shotgun. AMMUNITION stays covered: the
  ammunition entry is never removed (lab: new lethal .22 is still legal after Permit I is revoked). The permit
  returns on its own once Security is back at the gate (it is still owned at Wild's).
- **Holding a weapon above your permit**: it is ordinary contraband (CONCEALMENT.md): an inspection warns at level 1,
  seizes at 2, fines and seizes at 3; selling it commits WEAPON_TRAFFICKING (guns) or CONTRABAND_TRAFFICKING (ammo).
  The firearm signs and the gun storage bay hide nothing.

## 3. Gun orders (`StoreClientListGun`, `GunOrderData`, `ShowcaseHelper`)

### 3.1 Who comes

| Permit on display | Client | Tier | Faction | Base daily chance |
|---|---|---|---|---|
| `permit_gun_1` | `ll_gun_buyer` | Basic | Lower Levels | 50 % |
| `permit_gun_2` | `merc_gun_buyer` | Mid | Lower Levels | 40 % |
| `permit_gun_3` | `ul_gun_buyer` | High | Upper Levels | 35 % |

`ShowcaseHelper.GetGunBuyerChance`: min(100, base × (2 + days missed) / 2) × (1 + WEAPON + FIREARM market % / 100). A
missed day (no client that day, or crowded out) raises the next day's chance: 50 → 75 → 100 for Permit I, so the
steady rate is 8/13 ≈ 62 % of days. Gym: 59–85 Permit I clients in ~100–120 days. The three are rolled first among
the display case's clients and share the day's normal-customer slots: **each order client replaces an ordinary
customer** (gym: displaying the permit and sending every order client away cost the `gunsmith` route ~7 days to the
buyout on 16 seeds). The tablet shows the chance and a "*" when it is boosted.

### 3.2 What they ask

`GetRandomBasic/Mid/HighOrder`: when the shop owns no gun from the tier's list (Basic: printed or handmade pistol;
Mid: heavy pistol, glock, revolver; High: SMG, shotgun) half the orders are **service** orders; otherwise one of 9
(Basic) or 8 (Mid, High) kinds uniformly, service included.

| Tier | Base reward | Kinds (mandatory rows; bonus rows pay per threshold cleared) |
|---|---|---|
| Basic | 63–110 | accurate (FP > 17; ACC > 20/24/30 and REL > 24 at 46 each), suppressed (same + any silencer 22, a real one 53 more), firepower (FP > 11; 14/17/20/22 at 34–40), big (FP > 11; size), stylish (STY > 4; 7–23 at 34–63), general (FP > 15, REL > 23, ACC > 23; nine rows 17–40), handling (ERG > 44, RCL > 54), best .22 |
| Mid | 190–210 | suppressed (REL > 32), stylish (STY > 25), big (FP > 25), firepower (FP > 26; 28/32/35/38 at 40–55), general (FP > 25, REL > 29, ACC > 33), handling (10 mm, ERG > 39, RCL > 29), best 10 mm |
| High | 350–370 | firepower (FP > 59), style (STY > 52), big (size > 16), reliability (REL > 59), handling (10 mm, ERG > 54, RCL > 24), general (FP > 57, REL > 57, ACC > 44), best 10 mm |

Every row is **strictly greater than** the number shown (lab). A real silencer counts for both suppressor rows.

- **Best order** (`CreateBestOrder`): the only requirement is the caliber (.22 for Basic, 10 mm above). It pays the
  gun's market value with its mods, events and the forced retail markup, plus half (lab: a printed gun with an
  extended mag and a red dot, estimate 145, paid 217). Condition is irrelevant despite the dialogue.
- **Service order** (`CreateServiceOrder`, `CreateServiceGun`): the client brings a gun (Basic: handmade 70 / printed
  30; Mid: heavy 60 / glock 40; High: SMG 60 / shotgun 40; Poor 40 %, Fair 40 %, Good 20 %) and asks for 1 / 2 / 2 of
  its own stats raised by 8 / 10 / 12, with a second step and 2 / 2 / 3 side stats as bonuses (15–40 and 12–32). Pay:
  fee 30 / 60 / 100 (+0–10), the bonuses, and **every part of yours on the gun at its value**; a gun part used for a
  repair is paid only when the repair succeeds. Sent away, your parts come back.
- One visit only: no deadline, no return. Accept takes the gun the negotiation names or the **last** gun on the back
  counter (`GunOrderData.ResolveOrderGun`, ownership not checked).

### 3.3 What they pay

`GunOrderData.ApplyRewardModifiers`: each amount × (100 + WEAPON + FIREARM market % + retail markup) / 100; the retail
markup is 0 on hard (cozy 20, normal 15). Cash only (`OnOrderAccepted`); no reputation (lab), no contraband check on a
legal gun (`SecData.CheckSoldContraband` still runs: a contraband gun delivered is trafficking). The gun and its mods
are destroyed. Sent away, the client stays as a melee buyer (60 / 120 / 250). After a delivery it may buy ammo of the
gun's caliber (50 + 5 per bonus % chance, a third of the gun's value), a melee weapon (40 + 5n %) and, Mid and up,
munitions (30 + 5n %), accepting contraband.

What the guide leaves out: the display requirement, the climbing chance, the strict ">", the service-order bias
when you own no gun of the tier, that a best order pays for the mods at 1.5×, that the gun's mods go with it, and
that repairs that fail are never paid.

## 4. Guns, condition, repair and mods

| Gun | Value | Contraband | Base FP / STY / REL / RCL / ERG / ACC | Caliber | Repairability | Mod slots |
|---|---|---|---|---|---|---|
| printed_gun | 40 | 1 | 15 / 5 / 15 / 55 / 40 / 15 | .22 | 100 | 8 |
| handmade_pistol | 120 | 1 | 20 / 15 / 30 / 65 / 50 / 30 | .22 | 100 | 8 |
| heavy_handmade_pistol | 160 | 2 | 30 / 30 / 34 / 30 / 40 / 40 | 10 mm | 80 | 8 |
| glock_receiver | 200 | 2 | 30 / 40 / 42 / 35 / 50 / 45 | 10 mm | 65 | 8 |
| revolver | 240 | 2 | 36 / 50 / 50 / 38 / 55 / 52 | .45 | 60 | none |
| smg | 280 | 3 | 65 / 55 / 65 / 25 / 60 / 50 | 10 mm | 40 | 8 |
| shotgun | 310 | 3 | 85 / 75 / 75 / 30 / 45 / 50 | 12 ga | 35 | 8 |

Frames: handmade 70 (level 1), heavy 100 (level 2); a gun part on a frame gives the gun at New
(`GunFrameHelper.Finish`). Legal weapons: non-lethal ammo, stun gun, tazer, every melee weapon but the stun baton.

- **Stats** (`GunHelper.UpdateCurrentGunStats`): base × (100 + condition % + Σ mod %) / 100 + Σ mod flat. Condition on
  stats: New +30, Excellent +15, Good 0, Fair −15, Poor −30 (lab: a New printed gun has FP 19). On value: New +25,
  Excellent +12, Good 0, Fair −12, Poor −25.
- **Repair** (`GunHelper.TryRepairGun`, a gun part on the gun): chance = repairability + the chance carried from the
  last failure; a success raises one step (never to New, never from Excellent); the part is spent either way. A step
  is +15 % of base on every stat and +12–13 % of value: worth it to clear an order's last threshold.
- **Mods** (`GunModDirectory`, `GunModHelper`): eight slots (stock, rail, magazine, underbarrel, barrel, three
  special); one mod a slot; both owned (or the client's service gun); some only fit listed guns (magazines by caliber,
  barrel/bipod/launcher SMG only, grip/stocks SMG and shotgun); improved and plain springs exclude each other (lab).
  **No mod is contraband.** A mod's list value is added to the gun's price wherever the gun is valued (sales to
  weapon buyers, best orders at 1.5 ×, the ammo follow-up budget), but without the gun's condition or restricted
  markup (lab: the sum). So a modded gun sells for the gun plus its mods: a weapon buyer, who never buys a loose mod,
  pays for one on a gun, and a best order pays mods at 1.5 ×. Stats never change a price by themselves; they pay
  through order rows (strictly above each threshold) and service orders (fee + rows + **every part of yours on the
  client's gun at value**, `GunOrderData.GetPartsValue`), so a stocked mod kit is a supply question, not a value one.
  The three special slots refuse a duplicate mod; the stock slot fits SMG and shotgun only; `pistol_switch` pistols
  only. A declined service gun returns a gun part for every repair attempt, failed ones included
  (`GunHelper.ReturnPlayerParts`; read). Notable: `mag_22_ext` (35) FP +60 % +18; `mag_22_high` (25) FP +30 % +9; `internal_springs` (40)
  REL +25 % +10 (the "improved" springs, 20, are worse: +12 % +5); `rds` (60) STY +20 % +5, ACC +25 % +10; `silencer`
  (55) suppression 2; `laser` (50) STY and ERG +20 % +6.

## 5. Ammo

`small_pistol_ammo` (.22, 20, level 1), `_p` (26, level 2), `_nl` (14, legal); `heavy_pistol_ammo` (10 mm, 30, level 1),
`_p` (36, level 2), `_nl` (24, legal). Rounds are never loaded into guns. Packed into an ammo box (6 rounds, valued at
6 × the commonest round) they sit behind a lock: inspections and the sale check skip them (`AmmoBoxHelper`). Buyers:
scavengers (80–120), gun-order clients after a delivery, the shortage clients.

## 6. The 3D printer for the arms trade

RECIPES.md §1.3 holds the chip table. For the Armourer: the pistol chip (2 plastic → 1 printed gun, 3 nights) and the
double pistol chip (3 plastic → 2) make the order feedstock, legal at birth under Permit I (lab). The .22 mag chips
(plastic only) make the mods that push a printed gun past Mid thresholds. The frame chips need the metal and CNC
modules (from the retired gunsmith, more likely with each permit tier above I: +15 points a module, +10 a chip). A
printed heavy frame is level 2: legal only under Permit II.

**The retired gunsmith** (`StoreClientListSpec.CreateRetiredGunsmith`) only sells, at the retired-supplier markup
(cozy 0, normal 15, **hard 30 %**, challenge 45): a new printer, a screwdriver, 4 plastic and the basic chips always;
better chips, modules and mods on rolls (RECIPES.md §1.3, corrected: he also stocks silencer2, laser2, compensator,
the 10 mm mags, springs, trigger group, match barrel, pistol switch, ammo display and gel grip on 18–50 % rolls).

### 6.1 The frame forge (lab, `fence-lab forge.printers|orders|engine`)

A frame chip prints a frame from an ingot (the heavy one also takes a scrap); a gun part on the frame
(`GunFrameHelper.Finish`, the part's own use on the frame) gives the gun **New** (+30 % stats), its base stats ×
(100 + the frame's print quality) % (`GunHelper.MultiplyBaseStat`). Print quality = the printer's adjusted quality +
the ingot's bonus (`MachinePrinter.HandlePrintedItemQuality`, `CaptureIngotQuality`): Very Low −30, Low −20, Fair 0,
High +20, Very High +30, **Perfect +100**. The furnace's ingot is Low + 2 × the ore share of its inputs + its quality
/ 100 (at least 1 with a flux) levels (`IngotPurityHelper.SetPurityFromInput`): all-ore with a flux is Very High;
Perfect needs all ore and a quality-200 furnace (FORMULA-EXPLOITS §12).

| Measured | Result |
|---|---|
| The Gunsmith start's old printer with the metal and the CNC module | **refused**: its two stuck modules leave room for one of them, either order. The forge needs a new printer (the retired gunsmith always stocks one, 455 on hard) |
| New printer + metal + CNC | 14 power a night (8 bare); a heavy or handmade frame in **4 nights**; a gun-part run in **2 nights**: 3 parts from a Fair (or Low) ingot, 4 from Very High, 5 from Perfect; frames are legal at birth under their permit |
| Heavy frame by ingot (Q): gun FP/STY/REL/RCL/ERG/ACC | VL (−30) 27/27/29/27/36/36; Low (−20) 31/31/35/31/41/41; Fair (0) 39/39/44/39/52/52; High (20) 46/46/52/46/62/62; Very High (30) 50/50/57/50/67/67; **Perfect (100) 78/78/88/78/104/104** |
| Handmade frame by ingot | Fair 26/19/39/84/65/39; Very High 33/24/50/109/84/50; Perfect 52/39/78/169/130/78 |
| Estimate of the finished gun | handmade 150, heavy 200 whatever the quality (stats never move a price) |

Orders paid, 10 deliveries each through the order client's Accept (mean, all mandatory rows met unless stated):

| Gun | Basic: accurate / firepower / big / suppressed / stylish / general / handling / best .22 |
|---|---|
| handmade Q 0 | 274 / 222 / 102 / 274 / 227 / 282 / 200 / 225 (mean 226) |
| handmade Q 30 | 276 / 222 / 101 / 275 / 325 / 280 / 278 / 225 (mean 248) |

| Gun | Mid: suppressed / stylish / big / firepower / general / handling / best 10 mm |
|---|---|
| heavy Q 0 | 276 / 250 / 193 / 385 / 393 / 275 / 300 (mean 296) |
| heavy Q 30 | 378 / 331 / 196 / 387 / 429 / 355 / 300 (mean 339) |
| heavy Q 100 | 377 / 385 / 196 / 385 / 429 / 401 / 300 (mean 353) |

| Gun | High: firepower / style / reliability / handling / general / best 10 mm |
|---|---|
| heavy Q 30, bare | none / none / none / 425 / none / 300 |
| **heavy Q 100, bare** | **565 / 646 / 706 / 684 / 749 / 300** (mean 608; only big, size > 16, is out of reach) |

**Value per printer-night** (inputs at list: an ingot 50 × (1 + its purity's value modifier: Low −15 %, Very High
+50 %, Perfect +200 %), scrap 10, a part a third of a Low ingot, power 1.35 a unit; a gun takes 4 frame nights + ⅔ of
a part run, 4.67 printer-nights and 65 power):

| Build | Order tier | Mean paid | Inputs | Net a gun | Per printer-night |
|---|---|---|---|---|---|
| heavy, Perfect ingot (Q 100) | High (Permit III) | 608 | 262 | 346 | **74** |
| heavy, Very High ingot (Q 30) | Mid (Permit II) | 339 | 187 | 152 | **33** |
| heavy, Fair ingot (Q 0) | Mid | 296 | 162 | 134 | 29 |
| handmade, Very High (Q 30) | Basic (Permit I) | 248 | 177 | 71 | 15 |
| handmade, Fair (Q 0) | Basic | 226 | 152 | 74 | 16 |
| printed pistol (old printer, 3 nights, 7 power, 2 plastic at 12) | Basic (gym mean, round 1) | ~128 | ~52 | ~76 | ~25 |

So the build queue per permit tier is: Permit I, handmade frames (a Fair ingot is as good as a Very High one there),
and the old printer keeps making printed pistols, which earn more per night on its cheaper power; Permit II, heavy
frames from the purest ingot first; Permit III with a Perfect-ingot furnace, heavy frames for High orders, more than
twice a Mid order's night. A Basic order never pays a heavy's Mid value (the engine prices a 10 mm gun at 300 against
Basic orders under Permit II). Printer-nights bind, not orders: Basic clients come ~0.6 a day and Mid ~0.5, against
one gun every 4.67 nights per forge printer.

**Played in the gym (hard, 2910001–060, 1000-day horizon, `sim/lab/scripts/arms-forge.txt`, `armsreport.py`):
the forge does not pay on the Armourer's ~day-105 cash route, and on most seeds it never makes a gun.**

| Arm (`arms/…json`) | Wins (Wilson 95 %) | Median win day | Paired Δ win day | Cash buyouts | ΔNW day 60 / 100 | Runs with a built gun | Kit spend | Orders offered / filled (Basic, Mid) |
|---|---|---|---|---|---|---|---|---|
| `armourer` (round 1, reference) | 60/60 (94–100 %) | 105 | — | 56 | — | — | — | 6.1 / 4.3, 0.1 / 0 |
| `armourer_forge` (handmade + heavy frames, old printer idle) | 60/60 (94–100 %) | 203 | **+72.6 ± 14.0** | 20 | −2 127 / −7 002 | 11 | 1 748 | 5.5 / 4.2, 0 / 0 |
| `armourer_forge_mid` (heavy only, Permit II at Security 45) | 60/60 (94–100 %) | 203 | **+76.0 ± 13.1** | 19 | −2 168 / −7 788 | 5 | 1 587 | 4.3 / 3.7, 0.6 / 0.1 |
| `armourer_forge_pistols` (forge + the old printer's pistols, first build) | 60/60 (94–100 %) | 133 | +54.2 ± 12.8 | 31 | −1 486 / −5 033 | 18 | 1 838 | 48.1 / 26.0, 0.1 / 0.1 |

McNemar is 0:0 everywhere (every arm wins every seed). Δ is paired over seeds both won, ± 1.96 SE. Without the Board
not measured this round (the owner reprioritised; round 1 had every arm at 0/60 there).

Why, in order of weight:

1. **The kit is spent before it can earn, and that tips the cash buyout into the mortgage.** ~1 600–1 800 goes on
   modules and chips from day ~25; the route's day-84 projection then falls short and 40 of 60 runs sign the
   mortgage (win ~day 200) where `armourer` buys for cash on 56. The kit's own cost is ~6 days; the mortgage cliff is
   the rest of the +73.
2. **The kit rarely stays whole.** The retired gunsmith rolls each piece weekly (CNC 25 %, handmade chip 20 %, heavy
   chip 15 %, +10–15 points per permit tier above I); 25–35 of 60 runs held the whole kit at once (median day 46),
   but in the traced seed (2910002) the business-permit official was handed **the working forge printer with both
   modules inside** (`permit_bribe` 1 528 on day 45): `PlayerCounter.BribePicks` gave away an item the engines
   reserve (fair-play's file, not edited here). After day 56 the route drops insurance and burglars take loose
   modules and chips, and an idle printer's bay is not locked.
3. **Even whole, it pays back too slowly.** Mid heavies earn ~33 a printer-night, so a ~2 340 Mid kit needs ~70
   printer-nights under Permit II, which arrives on day ~48–62 of a run that ends near day 105.

The condition, stated: the forge pays when Permit II (or III with Perfect ingots, 74 a printer-night) will be held
for more than ~70 (≈ 31) printer-nights after the kit is whole, the kit is protected from the permit official and
burglars, and the purchase does not push the cash buyout into the mortgage. None holds on the hard Gunsmith route
as played; `forge` stays opt-in, off in `routes/armourer.json`.

## 7. Who buys weapons, and market events

| Buyer | Wants | Budget | Comes |
|---|---|---|---|
| mercenaryBuyer | WEAPON, ARMOR, AMMUNITION | 300 + 2/day | Lower Levels pool |
| scavenger | same | 80–120 | display case, WEAPON tier 1 |
| selfDefenseEnthusiast | same | 145–195 + day/4 | display case, tier 2 (WEAPON case ≥ 145) |
| privateSecurityContractor | WEAPON, AMMUNITION, ARMOR; not MAKESHIFT (no printed guns) | 500–700 + day | display case, tier 3 (WEAPON case ≥ 350 from day 14) |
| revolutionaryBuyer | WEAPON, MUNITION, AMMUNITION, MEDICAL | 150–200 + 2/day | Revolution pool |
| weapon shortage client | WEAPON, ARMOR | 250–300 | a WEAPON or ARMOR rise ≥ +30 |

Weapon price events (`OperationListBM/Sec/Rev/LL`, `EventListNormal`): illegal shipment −35 (2 days), raid on the
black market +40, Revolution mobilization +25 / +40, riot +50 (1 day), mob justice +33 (1 day), blackout +25. A rise
lifts sales, gun-order rewards and the order client's daily chance alike. The firearm signs (`sign_firearm`, from
Permit I; `sign_firearm_heavy`, from Permit III) bring gun **sellers** to the case (40 % and 25 % a day), not buyers.

## 8. Security

There is no Security reward for legal gun trading. Security comes from clean inspections (+3), reports and the
Security clients' trades (CONCEALMENT.md, PERKS.md). On hard the Gunsmith start nears 50 around day 40 (Permit II
bought at Security 45 is held from ~day 41; bought from day 60, from ~day 62) and reaches 80 only with the alarm and a
Security transmitter (gym, docs/research/ROUTES.md, "The Armourer").
