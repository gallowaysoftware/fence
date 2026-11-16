# Water Baron
Path: the tap. Filtered tap water (filter + chlorine) to Basewater, the purifier to High Quality, an HQ jug worth 350+ in the case for the stockpiler; later fish tanks (fish need volume, not grade). Cheapest from Water Merchant; else Water Trading (300).
Buy: taps (250, 350) without touching the permit's money; a second purifier by day 20; Vending Fountain (1200) at cash 2500; Aquaculture (400) once two purifiers run.
Factions: Lower Levels 20 then 60 (market events foretold); Upper Levels for HQ and Pure.
Never: raise a label; sell treated water untested (it sells at its old label); sell a filter, purifier or tablet as goods.
Windfall: grab purifiers, moisture farms, filters, fineness modules.
Win: buyout by about day 65, never a fraud or endangerment crime.

## Notes (not in any prompt)

**Corrections (2026-11-12).** The bare purifier stops at 99.01 % (HQ); Pure needs four fineness upgrades or the
evaporator, so the stockpiler is pulled by an HQ jug worth 350 or more in the case, from day 14. Fish read only a
tank's volume and waste, never its grade. Treated water keeps its old label until a strip or the AquaScan tests it, and
sells at that label. Taps bought with the permit's cash got runs arrested for operating without a license. Lower
Levels 60 foretells any market event, not water events. Sources: docs/re/WATER.md §1.5, §2.1, §2.4, §4;
docs/research/QUALITY.md §1, §2; docs/research/PRODUCTION-VERDICTS.md §1, §8; docs/re/PERKS.md §3.

## Payback nights (hard; cost ÷ net per night, measured + docs/re/WATER.md)

| Engine | Cost | Per night | Payback |
|---|---|---|---|
| common electronic → fountain attractiveness | 60 | +35 | **1.7** |
| hydroponic reed bay (390 + seed) | 490 | 240 (20 L raw→HQ) | **2.0** |
| uv_filter (Jackson 270) | 270 | 60 (HQ per pitcher) | 4.5 |
| machine purifier (Jackson ~350) | 350 | 60 (base→HQ per 3 L) | 5.8 |
| moisture farm (525) | 525 | 34 (850 ml; feeds purifier/fountain) | 15.4 |
| vending fountain (1800) | 1800 | 67 at attract 15 | 26.9 |
| vending machine (~900) | 900 | 16 alone; feeds the fountain | 56.2 |

**Acquisition order for the archetype:** purifier/uv → reed bays (if a raw supply and an HQ buyer
exist) → moisture farm → fountain → commons spam (1.7 nights each — the compounding engine) →
vending machine last. The save floor keys on this order: it holds cash hardest for the fastest
payback item still missing (`Production.PurifierWanted` before `Fountain.PendingCost`), which is
why the graduated floor lands the fountain ~13 days earlier than the flat one.

The tank (455) is excluded on purpose: it is a buffer, not production — it pays nothing by itself
and only enables night cycles. "Worth amassing before the savings phase" = every engine whose
payback is shorter than the current saving target's remaining stretch.

## Upgrade payback (hard, incl. the ×1.5 upgrade price)

| Upgrade | Cost | Effect | Payback |
|---|---|---|---|
| common electronic → fountain attractiveness | 60 | +5 attract ≈ +35/night (67 at 15 → 391 at 100) | **1.7–4** |
| GUTTERFLOW_TAP | 375 | unlimited pours, 80–90 % purity; feeds the purifier's 60/night | **6.2** |
| RUSTWATER_TAP | 525 | same, 90–91.5 % rust — half a purifier's throughput | 17.5 |
| RETIRED_FARMER | 525 | hydroponic kit + weekly seeds: enables 2.0n reed bays | enabler |
| VENDING_FOUNTAIN | 1800 | 67/night at attract 15; commons compound it to 391 at 100 | 26.9 → 5 once commons flow |
| RENOVATION I–III | 750/1125/1500 | +150 base attractiveness each | 21.4 (one tier) |
| WINEMAKER | 300 | measured wine uplift 11/night (grapes-bound) | 27 |
| VENDING_MACHINE | 1800 | 16/night measured pre-fountain; only pays after the fountain exists | 112 |
| SHOWCASE I/II, BUY/SELL_REFERAL | 300/750, 300 + 10/n | +1 case visitor each; counter uplift unmeasured | traffic, not income |
| LANDLORD_GRACE, SEC_GRACE, EVIDENCE_REMOVAL | 150/375/750 | one-time risk removal | insurance |
| JACKSON I/II, WATER_TRADER, MINER, PHARMA, MARKETPLACE | 375–975 | supplier depth: cheaper inputs, more kit | input-cost cuts |

**Reading:** the fountain is a *platform*, not an income line — 27 nights alone, but every
commons after it pays back in 2–4 nights, and RENOVATION tiers stack the same base attract. The
executor's save order (purifier → farm → fountain) is right; the *commons spam* after the
fountain is the compounding phase the persona's "Aquaculture once two purifiers run" sits beside.
Services are the leak to watch: referrals run 10/night each (73/week pair) for unmeasured traffic.
