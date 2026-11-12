# Fishmonger
Path: tanks to fillets. The Aquafarmer's two tanks: tilapia bred (two females, a male a tank), fed industrial feed, filleted the morning its meat passes 400; then eels fed on fillets, a sturgeon for caviar.
Buy: water in both tanks day 1 (any grade: volume and waste count); electronics before tank size; a filter a tank; Dr. Jackson's converter before the kit cells die (~day 10); every feed bag the aquafarmer brings; Aquaculture II (1 800 on hard) with two rents banked.
Factions: Lower Levels 20 (Referral); Upper Levels 20 (Tips) for eel and caviar.
Never: a third dark night; pellets past a quarter of a fish's food (trash grade); a hatch with no room; a fish out of water overnight.
Windfall: grab tanks, dispensers, filters, cells, a surgery tool.
Win: the buyout on fillets.

## Notes (not in any prompt)

**Voice.** A market-stall fishmonger with a barker's patter (Kokoro `bm_fable`, `internal/show/persona.go`). Brags about
the morning's fillets, a full tank and the first jar of caviar. Despises waste: a dark pump, pellet-fed "trash fish", a
hatch thrown away for want of room. Takes a loss like weather at sea: a shrug, a joke about the tide, then back to the
knife. Soft on the breeders, never on a fish on the board.

**Rook's angle.** A pawnbroker running a fish stall: the smell, the power bill, and one dark night that empties the
tanks; caviar is a dream that costs a sturgeon and a fortnight.

**Sources** (the rules the plan relies on):
- The start: two shabby tanks with a 50-charge cell each, an adult tilapia pair, fertilised roe, two bags of industrial
  feed, a knife, two 6 L jugs of ghost water; −450 cash, rent +40; selectable in this build. docs/re/AQUACULTURE.md §4.
- The tank's water tank starts empty; only the volume against the stock's need and the organic-waste share matter, not
  the grade; one water filter keeps the waste near 0. AQUACULTURE.md §1, §3; WATER.md §2.4.
- Power 5 a night a tank, −2 per electronics upgrade (floor 1); an unpowered night costs 34 % of max health, the third
  kills; the kit cells last ~10 nights; cells and the converter come from Dr. Jackson. AQUACULTURE.md §1, §1.1, §3.
- Meat is 0.9 × every kcal eaten, stomach included: harvest when the meat line passes 400 (a fish fillet). Live fish
  are worth 0 at the counter. AQUACULTURE.md §0, §2.4, §5.
- Breeding: a fed adult female lays at 15 % a night, a fed male fertilises, a clutch is 6; a hatch with no room is
  destroyed; fish and fry out of a tank die at night. AQUACULTURE.md §2.3.
- Pellets over 25 % of a fish's lifetime kcal make its fillets trash grade (gutter tilapia exempt). AQUACULTURE.md §2.2.
- Feed: the retired aquafarmer (days ≡ 2 mod 7) brings 2 industrial feed and 2 pellets a visit, the only bulk feed;
  omnivores eat other food through the dispenser. AQUACULTURE.md §4.
- Eels (816 at day 18) and sturgeon (5 caviar jars, 1 200, a strip from day 18, with a surgery tool) need
  AQUACULTURE_II (1 200 × 1.5 on hard) and the aquafarmer's big fish; eels eat meat only. AQUACULTURE.md §2.1, §2.4,
  §2.5, §4; hard multiplier: docs/re/MITIGATIONS.md §0.1.
- Faction perks: LL 20 Referral, UL 20 Tips (+15 % on UL sales). docs/re/PERKS.md §3. Eel fillets and large fillets
  are luxury (AQUACULTURE.md §2.4).
- What it earns: ~80 a day of fillets with the Board (53/60 on hard); with the Board off nothing wins (0/60: fish
  income is under the hard rent), so "Water to Fish" needs a lever before it airs. docs/research/ROUTES.md, "The fish
  engine on the Aquafarmer start"; AQUACULTURE.md §7.

**Samples.** Day-0 lines from drift's 27B (`qwen38-27b-mlx`, temperature 0.8) through the narrator's own prompt
(`Narrator.system` + `prompt` on a route-choice moment, 2026-11-12), given a test thesis, not a real run; both passed
the fact check. Rook's answer to the second, from the co-host prompt with the angle above, follows it.

- Tanks paid for, so we bleed tilapia at dawn. Skipped the plain sprinter; I keep my eye on those cells dying by day ten.
- We run fillets to caviar, not rats. Dawn knife ready.
- Rook: Rats? You passed on rats?
