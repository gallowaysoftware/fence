# Honest Broker
Path: pure trading with a clean name; inspection trades (injectors, stamps, cigarettes: read every tell) and the appraisal service. Any start; Apprentice is cheapest.
Buy: Showcase I (200) by day 10; Buyer Referral (200) by day 14; Showcase II (500) and Renovation I (500) by day 30; Renovation II at cash 3000.
Pivot: at cash 2500 after day 30, add the cellar (Retired Winemaker 200) if berries come cheap.
Factions: Lower Levels 20, Security 60, Upper Levels 20.
Never: pass a fake as genuine; sell hot; contraband, threats, blackmail, a crime on record.
Windfall: grab underpriced goods and modules to resell. Stay on path unless a windfall clearly beats it.
Win: buyout by about day 70, no crime ever recorded.

## Payback nights (hard; cost ÷ net per night, measured on seed 2610003 + docs/re/APPRAISAL.md)

| Item | Cost | Per night | Payback |
|---|---|---|---|
| appraisal board join (170) | 170 | **213–243 measured** (29 691 fees / 140 nights on one seed; 243 median across 152 league runs, p90 272) | **0.7** |
| Showcase I (300) | 300 | +1 case visitor → appraisal clients come ONLY via the displayed certificate | enabler, day 1 |
| Buyer Referral (300 + 10/n) | 300 | +1 case visitor type | traffic |
| Showcase II (750) | 750 | +1 more visitor | traffic |
| Renovation I (750) | 750 | +150 base attract | 21 per tier |

**Reading:** the appraisal service is the best *standalone* business in the game — 0.8 nights
to join, ~240/night at rank 3, and it needs no feed, no inputs, no quota. Its only dependencies
are a certificate **in the display case** (stock does nothing) and traffic, which is why the
persona's buy order (Showcase I day 10, Buyer Referral day 14) is exactly right. Renovations
at 21 nights per tier are the only questionable line — they help every other traffic-driven
income at once, so they pay indirectly. The savings phase: essentially never; the biggest item
is Renovation II (1125) and the run's whole point is the clean buyout, so `buyout_save` governs
from the route's day.
