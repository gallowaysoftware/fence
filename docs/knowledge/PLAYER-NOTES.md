# Player notes (Kyle)

First-hand strategy from the owner's own runs. Highest-trust source for *strategy*; mechanics
still get checked against the code. Add to it, don't rewrite it.

## 2026-11-09 — run on hard, around day 108
- **Production, not just trading.** Grows berries, ferments them into wine and ages it; a
  furnace melts scrap into something sellable. The shop's income is partly what it makes.
- **Quality sorting.** Keeps only good berries for wine; moldy, under- and over-ripe ones
  are sold as food instead of wasted.
- **Dumping ground every night.** Scavenging is a nightly routine, not an occasional trip.
- **Speculating on shortages.** Bought food in bulk *before* a food shortage event and sells
  it during the shortage for profit (the market calendar shows events in advance).
- **Let heat die before selling.** Checked each piece of meat's heat before selling; pieces
  in the smuggler's bay that were still hot stayed there another day or two. Verified in
  code: heat falls 18/day anywhere (2/4/6/8 less for low/mid/high/critical contraband,
  `StolenHelper.UpdateStolenItem`); at 0 the item is no longer stolen, so the sale is clean —
  no fencing crime (`SecData.CheckSoldContraband` only fires on stolen/contraband). Smuggler
  bays (`smuggler_bay`, `_mod`, `_mini`) block inspection searches
  (`ContrabandHelper.IsBlockSearchContainer`); a stolen item found elsewhere goes back to heat 100.
- **Sell value vs use value.** Sold water filters as household goods, then realised they
  could have been kept to filter water. Many items are inputs to the shop's own production
  (water, wine, fish, hydroponics…), so an item's worth is the better of its sale price and
  what it adds when used. The plan has to decide what to keep before the counter sells it.
- **Water: one source, three uses.** During a water surplus, sells water for good money, but
  also needs clean water for hydroponics and wine. Cleans water with filter pitchers and a
  water-cleaning plant feeding a huge container (lots of Basewater). Adds mineral tablets to
  hydroponics to hit the berries' mineral range; berries came out mostly garbage lately
  ("was doing better before, maybe the water"). Code says (WATER.md §2.3): Basewater (~98 %)
  drives bloomberry health to ~0 → worst condition rolls; HQ + tablets ≈ 78, Pure + tablets
  = 100. Filters/purifier strip minerals, so tablets go in after; target 200 ± 75 bp
  mineral. Purifier machine reaches HQ unupgraded (Pure needs quality ≥ 100); pitcher and
  portable purifier stop at Basewater; the purifying plant spares minerals. Suggested split:
  Basewater → selling, purifier output → berries and wine. Plant analyser "growth score" =
  the health that decides quality. (Not yet verified in play.)
