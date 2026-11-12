# Pharmacist
Path: a clean caffeine lab. Beans extracted in alcohol, acid for protein and lipobond for fat (both under 55 °C), through kotton then a water filter, dried, pressed: purity is the score. Apprentice start; Retired Chemist (1 500 on hard), his machines.
Buy: the finisher's first module first (pharma); a plate module (dries caffeine, no evaporator); every alcohol pack (alcohol binds the lab); solvent recovery; coffee bays on tap water after the permit; finisher modules 2–4 (lab pure).
Factions: Lower and Upper Levels 20 (medical buyers); Security ≥ 0.
Never: caps, dust, Phanta or the restricted pack, in stock or sold; a pour without the water filter (street grade); a bad pill sold.
Windfall: grab chem modules, filters, kotton, beans.
Win: the buyout on lab-pure pills.

## Notes (not in any prompt)

**Voice.** A prim, exacting pharmacist (Kokoro `bf_isabella`, `internal/show/persona.go`). Brags about purity to the
decimal, a clean record and a label that tells the truth. Despises cooks, cutting, street grade and anything sold by
the gram in an alley; is faintly embarrassed by a near-pharma batch. Takes a loss clinically: a failed batch is data,
a wasted bean is noted, the procedure is amended.

**Rook's angle.** A lab pressing coffee pills while the cooks make real money; purity is a hobby, and who pays extra
for the last percent?

**Sources** (the rules the plan relies on):
- Legal Highs plays the Apprentice and buys the Retired Chemist referral (1 000, 1 500 on hard): his weekly visit
  (days ≡ 2 mod 7) offers the blender, plate, evaporator and finisher, common chemical supplies, filters, kotton, chem
  modules, the solvent-recovery module (75 %), and the restricted pack (50 %, contraband 2). The referral also brings
  the shady farmer (dream caps and coffee beans) and the street dealer. docs/re/CHEMISTRY.md §3, §7.2, §7.6;
  docs/research/CHEMISTRY-ENGINE.md.
- The caffeine chain and its traps (lipobond boils from 55 °C; caffeine extracts from 50 °C and never degrades; kotton
  then the water filter). CHEMISTRY.md §0, §6.2; CHEMISTRY-ENGINE.md, "The chains".
- The grade by kit: 0 finisher modules 87 % (near pharma), 1 module 93 % (pharma), 4 modules 99.1 % (lab pure);
  without the water filter the residue keeps its physical contaminant and pills are street grade (73 %). Lab pure
  needs four finisher modules. CHEMISTRY-ENGINE.md, "The grade each chain reaches"; CHEMISTRY.md §4.
- A plate module dries caffeine above 100 °C and retires the evaporator; blender modules change the yield, not the
  grade. CHEMISTRY-ENGINE.md.
- Alcohol is the binding input on paper (a common pack does 8–10 beans, ~1 a week); the solvent-recovery module keeps
  half of what the evaporator takes. CHEMISTRY-ENGINE.md, "The closed loop".
- Coffee bays on tap water: 4 beans every 3 nights; taps only after the business permit (the arrests came from
  operating without one). docs/research/PRODUCTION-VERDICTS.md §6–8.
- Legal, but a bad product writes FRAUD or ENDANGERMENT through the buyer's feedback; every buyer refuses junk and
  starch grade; medical buyers (LL and UL) take caffeine. CHEMISTRY.md §7.1, §7.2. Contraband list: §7.5.
- Faction perks: LL 20 Referral, UL 20 Tips, Security 0 Requisition (an officer buying medical). docs/re/PERKS.md §3.
- Measured: the caffeine lab with the Board wins 60/60 on hard, a median 28 days after the sprinter; Board-free 0/60.
  CHEMISTRY-ENGINE.md, "Measured" (`caffeine_lab`).

**Samples.** Day-0 lines from drift's 27B (`qwen38-27b-mlx`, temperature 0.8) through the narrator's own prompt
(`Narrator.system` + `prompt` on a route-choice moment, 2026-11-12), given a test thesis, not a real run; both passed
the fact check. Rook's answer to the second, from the co-host prompt with the angle above, follows it.

- Today we choose the clean lab, not the alley trade. I reject the dust and embrace the right dosage.
- I chose the clean lab, rejecting the messy sprinter. We brew on tap water, waiting for the permit.
- Rook: Tap water is free, but permits are not. Who pays for that last percent of purity?
