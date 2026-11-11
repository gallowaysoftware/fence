# Expert players

Who plays Probably Stolen well on YouTube, what they do, and how we know. Gathered 2026-11-09
(1.0 released 2026-10-28) by `cmd/fenceexpert` (`internal/expert`). Everything below is in our
own words; transcripts, frames and video stay under `var/expert/` and are never committed.

Nobody's play is ground truth (docs/PLAN.md). A strong player's habit is a hypothesis with a
better prior than a random one; the gym decides (`docs/research/EXPERT-HYPOTHESES.md`).

## How skill was judged

Evidence, strongest first: a verified ending (buyout or mortgage paid, with the day and the
difficulty on screen); a long run on Thin Margins (hard) or Red Ink (challenge), especially from
Rock Bottom; many runs and explained reasoning that matches the code; audience size counts for
nothing. Pre-1.0 (playtest/demo) runs are kept but ranked lower: rules, prices and difficulties
changed at 1.0.

## Ranked creators

`var/expert/sources.json` holds 154 candidate videos from 16 creators (metadata only). The
mined set (`pick`) is 11 narrated 1.0 videos from 8 creators; Harvey's two unnarrated parts were
read off the screen instead. PerunGamingAU's E2 and E3 were added on 2026-11-10 (13 videos).

| # | Creator | Strongest evidence | Mined video(s) |
|---|---|---|---|
| 1 | Harvey | **Bought the store with cash on day 21 on Mark Up (normal)**, Water Merchant start, 5 unedited parts, "no mod no cheat"; the end screen shows sales 33,160, both rents paid, buyout 12,000. Unnarrated, so it was read off the screen (below), not transcribed. | [Ep1 (days 1–6)](https://www.youtube.com/watch?v=cSWZ2gd_5II), [Ep5 (days 18–21)](https://www.youtube.com/watch?v=c9XlMUHiQA8) |
| 2 | HeyPashi | Rock Bottom + Red Ink (challenge) + 18 perk points left unspent ("the hardest run possible"); 4 parts so far, chaptered by day, explains expected values out loud. | [Ep1](https://www.youtube.com/watch?v=JdSJchdVhY8), [Ep3](https://www.youtube.com/watch?v=0ES59zA5MxA) |
| 3 | Fruit Newt | 1.0 day-50 deed run (Water Merchant, lowest difficulty) that bought the store; kept playing into Hypertech; now Rock Bottom + Red Ink. ~40 videos since the playtest, including a 100-day challenge that paid off its debt. | [Deed by day 50](https://www.youtube.com/watch?v=1mpYoIo4xYA), [Owning the store](https://www.youtube.com/watch?v=voEFddS-Ewc), [Rock Bottom Red Ink](https://www.youtube.com/watch?v=hHrZS2HP15U) |
| 4 | TheNeomare | Max-difficulty specialist since the playtest (SCD + Hard + Rock Bottom; Rock Bottom + Hard + Model Citizen); 1.0 guide to the Rock Bottom hard-mode achievement with perks and upgrade order. | [Hardest achievement guide](https://www.youtube.com/watch?v=9YPMuTFeAMU) |
| 5 | Sileevio | 1.0 Rock Bottom on Challenge (Red Ink), 13 daily parts and alive; playtest runs that reported 15k in a week. | [Ep1](https://www.youtube.com/watch?v=Y84o-N1ajbA) |
| 6 | CthuLuck | Longest runs found: playtest Rock Bottom to day 130 and its ending; 1.0 Gunsmith (Weapons Trader) on Thin Margins past day 100. | [Weapons Trader days 1–20](https://www.youtube.com/watch?v=gV0zMrqkmnE) |
| 7 | Darlom | 1.0 live Rock Bottom on Red Ink + Sentinel (SCD); small audience, outcome unverified. | [Red Ink + SCD](https://www.youtube.com/watch?v=nVTNvAz1qI4) |
| 8 | Aliensrock | 1.0 fresh run on hard (Thin Margins), systematic sim player with a large audience; 9 parts on the game. | [Hard mode](https://www.youtube.com/watch?v=Yi-ZXeIEO_M) |
| 9 | PerunGamingAU | 1.0 $0 start (Rock Bottom) on hard, careful reasoning on camera; 3 parts, alive on day 59 with Security 100 and heading for the buyout (not yet shown). | [E1](https://www.youtube.com/watch?v=Qdw0HPmI_q4), [E2](https://www.youtube.com/watch?v=479dXhXsUHs), [E3](https://www.youtube.com/watch?v=SdANY8y3C7Y) |
| 10 | HumbleSeal | Playtest "hardest challenge" to day 100; perk tier lists (pre-1.0 rules). | not mined |
| 11 | Joker's Gambit | 1.0 real-time speedrun (first attempt) and a full good-ending run, both unnarrated: route evidence only (the speedrun transcript is music). | not mined |
| 12 | Recreational Suffering | Engine guides (water, wine, chemistry, fish, rats, guns) with the largest reach; mechanics rather than runs, already covered by the knowledge pipeline. | not mined |
| — | PCG-Spacegoon, TeraByte, Probably Augmented Petty Thief, Gamer Bear, GENFisher | Red Ink / Rock Bottom attempts, a water-merchant buyout clip, an honest-merchant achievement run; skill unverified. | not mined |

Web searches found no Reddit, Steam-discussion or leaderboard ranking of buyout days (the game is
12 days old); Harvey's is the only verified fast buyout we found.

## Strategies as paths

Each path is start → engines → upgrade order → risk policy, summarised in our words from the
extracted decisions (`var/expert/decisions.jsonl`, 1,162 tuples) and, for Harvey, from the
screen.

### Harvey — day-21 cash buyout, Mark Up, Water Merchant (read off the screen)

- **Start and perks.** Water Merchant on Mark Up (normal) with seven perks via Colorful
  Character: Working Pipes, Model Citizen, Sycophant and Aug Menace, paid for with Gambling
  Addiction and Blabbermouth. So: free water taps, Security and Upper Levels rep from day 1, the
  aug bounty, and negatives meant to barely touch a clean water shop (the gym disagrees:
  `docs/research/EXPERT-HYPOTHESES.md` H12).
- **Engine.** Water from day 1 as the main line: bottles, jugs and pitchers of filtered
  water sold in bundles. Cash is spent to almost nothing every night (617 → 28 overnight on day 1;
  33 on day 3); Dr. Jackson gets 4,339 over 8 of 20 nights (4 machines and modules), including
  an AquaScan bought during a water-supply crisis. On night 5 he calls Wild and spends about 1,000
  of 1,138, leaving 126. By day 6 the shop also runs a hydroponic and a recirculating aquaculture
  system.
- **Selling.** Bundles sized to the shown customer budget (446 against 476); the best flip was a
  jug of water bought for 15 and sold for 498 to a water stockpiler; best day 18 with 5,583 sold.
- **Cash curve.** Credits stay near zero until about day 13, then climb steeply: sales 33,160 over
  21 days against 8,565 of stock. Cash 22,461 (wealth 30,034) on day 21, deed bought for 12,000 in
  cash. The closest call was the first rent (day 7, 438 to spare).
- **Overheads.** Rent 409 (two weeks), insurance 146 and janitorial 37 for the whole run, no power bill, no business permit, no bribes. Nights: 8 in the store, 4 at the
  dumping ground (30 scavenges, 704 of loot, no injuries), 8 at Dr. Jackson.
- **What it teaches.** Speed comes from compounding one engine, not from trading margin: the
  whole-run overheads were under 700 against 33,160 of sales.

### HeyPashi — Rock Bottom, Red Ink, every negative perk

- **Start.** Rock Bottom on Red Ink with the negatives taken for points and the points left
  unspent: Ascetic, Sentinel (an injector every six days), Awful Negotiator (no bargaining),
  Public Enemy, Blackmail, Blabbermouth.
- **Engines.** Scavenging while broke (~50 a haul), the appraisal service from day 5 (first big
  customer ~270), then water: a second purifier, a large container, a second battery and a
  charger, all before any shop upgrade. He puts water plus maxed appraisal at about 4,000 a week
  and drops scavenging around day 20. Later a water vending machine (2,400, ~10-week payback,
  immune to water surpluses).
- **Upgrade order** (roughly, as narrated). Second purifier → storage container → battery + charger → all market signs →
  vending machine → attractiveness (1,000) → more customers (1,500); display upgrade and seller
  referral around day 11, the referral switched on and off.
- **Risk.** Crime kept under 50 (the inspection threshold) and checked before any contraband
  sale; contraband one item at a time behind a legal display weapon; stolen goods never on the
  counter, sold under ~10 % heat; every reputation above −20 (vandalism); counterfeit checks on
  every seller item (serial ranges, packing numbers).
- **Cash.** Rent and blackmail paid first; saves for breakpoint upgrades and refuses
  purchases that do not reach one; insurance only when the stock at risk justifies it, janitor
  every second day.

### Fruit Newt — water to the deed by day 50, then Red Ink

- **Start.** Water Merchant on Price Gouge (cozy) for the deed-by-day-50 run; Rock Bottom on Red
  Ink for the challenge run.
- **Engines.** Water purification from day 1 (ghost → base → high quality, relabelled), materials
  to Dr. Jackson, water and food crises sold at spike prices. On Red Ink: food sales and a
  furnace (efficiency over quality), chemistry planned once solar power made machines cheap.
- **Upgrade order.** Insurance and janitor day 1 → water parts → Giorgio's backpack (day 5) → a
  battery-free machine → renovation (day 6) → Giorgio's scavenging protection; on Red Ink,
  magnifier + food sign + fridge day 1, furnace day 3. After the deed: endorsement, then
  Hypertech.
- **Risk.** Hides stolen stock from the window and from Security; reports customers for cash
  early (450 on day 1 of the water run); pays fixers to move hot goods.
- **Cash.** Wholesale or nothing on Red Ink; keeps the rent threshold in view each morning;
  declines the loan shark.

### Aliensrock — hard, lean, contraband to Odin

- **Start.** Fresh 1.0 run on Thin Margins, no perks stated.
- **Engines.** Nightly scavenging, discounted wholesale lots flipped the next day, a fridge for
  chilled goods (two efficiency upgrades), appraiser board from day 5, a contraband pipeline to
  Odin through a smuggler's bay, intel bought to stockpile ahead of shortages, aug bounties, and
  late a chemistry upgrade (3,319).
- **Risk.** Buys stolen goods cheap and openly, waits a day for heat, never sells them to
  Security; contraband bought in small steps and sold only to Odin; shoots only confirmed augs.
- **Cash.** Deliberately lean: down to 1 credit the day before the first rent, saved by selling
  junk. Insurance off during the burglar grace, janitor every other night.
- **Regrets.** Too broke to take several good wholesale lots; Wild's Network unused all week.

### PerunGamingAU — $0 start on hard (series)

One 1.0 run of Rock Bottom on Thin Margins, three parts so far: E1 (2026-11-04, days 1–~21),
E2 (2026-11-07, days ~21–35; the save was lost, so he replayed to the same point with the same
decisions) and E3 (2026-11-10, days 38–43, then a cut to days 55–59). No build number is stated.
Outcome so far: alive on day 59 with 5–8k cash, Security 102, rent 553 a week, and a stated
sprint to the ~25,000 buyout next episode. That figure fits the code (24,000 + 50 a day after
day 50). Timestamps are `E#@m:ss`.

- **Start.** Short Sleeper, Proficient Scavenger, Diplomat, Colorful Character and Aug Menace,
  paid with Blackmail and the Commissary ban (E1@2:33–5:35).
- **Engines, in order.** Scavenging and aug bounties (day 1); appraisal certificate on day 4
  (E1@22:19) and the appraiser promotion for 200 (E1@52:05); furnace + charger at Dr. Jackson's
  day-after-rent 10 % (E1@41:59, E1@49:22); Water Trader kit **paid in cash** about day 21, then
  aqua scan (250), UV lamp, labeler, purifier and moisture farm, ending near 0 from 903
  (E2@3:36–9:39); wine for 200 (E2@27:07); extended storage bay 722 (E2@31:18); large tank 315
  and the pipe connection (gutterflow tap) 275 (E2@45:04, E2@48:17), which feeds tap water through
  filters, UV, tablets and the purifier to HQ water; a Jackson bundle (machine bay, a transmitter
  module, power block) for 750 (E2@51:23); second machine bay 850 (E3@1:03); small
  business permit 2,000 plus an 85 tip on day 43 (E3@27:16); aquaculture with the gunsmith kit
  and weapon permit I (975, E3@31:20); Renovation II (~1,000) on day 56 (E3@38:34); a commercial
  power supply that doubles battery charging (E3@43:41).
- **The Security race.** An alarm with a transmitter module (6 power a day) that broadcasts to
  Security pays +1 Security a night (E2@0:30). With reporting impostors and false theft claims
  (E2@36:55, E3@37:32) and buying Security's requisition stock at a markup (E3@52:55), he got
  Security 102 by day 56. At 100 he says there are no inspections and no burglary, so contraband
  can sit anywhere (E3@42:09, E3@52:55). He then pointed the transmitter at the Black Market and
  Upper Levels (E3@53:26).
- **Risk.** Stolen goods only after the heat decays. Contraband that can't be hidden is dumped
  to a buyer at 10–25 % off rather than risk an inspection (E1@45:05, E2@33:19, E3@23:12; Odin
  took most of a weapons haul, leaving 2,965 cash). Machine bays used as hiding space (E3@7:10).
  Refuses to mislabel water for sale (E2@26:34, E2@34:51), but put HQ water in the window
  labelled "pure" to pull noble buyers, without selling it under that label (E3@45:41).
- **Cash.** Spends everything early (E1) but saves for the 2,000 permit and skips Jackson
  (E3@0:32, E3@12:16). Weekly rent on hard compounds to ~25,000 by week 52, so the rule is
  "outscale the rent or buy out" (E2@5:07). On day 56 he stops adding industries and saves for
  the buyout (E3@38:03). Lent Georgio 1,531 on day 43 (E3@17:01). Insurance every 2nd–3rd night,
  1 in 3–4 once the alarm and Security rep are up (E2@17:18, E2@44:33), none at Security 100.
  Neither the loan shark nor the mortgage comes up in the three parts.
- **Selling.** Discounts of 25–35 % so buyers can afford several bottles, because a discount
  raises the buyer's budget (E2@33:19, E2@41:58). Matches the code: an offered discount raises
  the budget by 2 × pct % (`docs/re/BARTER.md`).
- **Regrets.** Renovation II too late: at 550 attractiveness customers have +25 % credits and
  there are +3 customers a day (E3@42:09). The landlord's vending machine (+150 rent a week,
  compounding) sold only 34–72 a night (E3@24:44, E3@35:00).

**Against our docs and the code** (each one a hypothesis, not a correction):

| Claim | Where | Against | Status |
|---|---|---|---|
| Accepting the landlord's vending machine (+150 a week) beats paying 1,800 for one this early | E3@15:28 | The gym executor declines it as pure cost (`sim/gym/PlayerRole.cs`, which stocks no vending machine). His machine took 34–72 a night (about 240–500 a week) against +150 a week, compounding; the cost of the water he stocked it with is not given, and he called it disappointing | contradicts the gym default; unresolved |
| A disposable keycard takes 25 % off a large Jackson purchase | E2@39:57, E2@51:23 | Not in the code docs; `docs/re/PERKS.md` finds no code Jackson discount for LL Pillar either | unverified |
| Pipe connection costs 275 on hard | E2@47:47 | `docs/re/WATER.md`: `GUTTERFLOW_TAP` 250, and he says upgrades cost +50 % on hard (E1@6:35) | price unverified |
| Appraisal board on day 4 | E1@22:19 | STRATEGY.md says day 8 | timing only |
| Two moisture farms give 160 % of one | E2@19:52 | `docs/re/WATER.md` ÷ (1 + 0.25 per extra farm) gives the same | agrees with code |
| Security 100: no inspections, no burglary; 60: rarer inspections, slower investigations | E3@12:49, E3@52:55 | `docs/re/PERKS.md` (`SEC_TRUSTED`, `SEC_VIP`) | agrees with code |
| Renovation II, 550 attractiveness: +3 customers a day | E3@42:09 | `docs/re/WINDOW.md`: +1 customer at each of 200, 250 and 500 | agrees with code |

**Not covered by STRATEGY.md before this series:** Security VIP as a mid-game target and the
transmitter that builds it; the gutterflow tap feeding the water engine; aquaculture (graft
tilapia skin and eggs give medical supplies, E3@39:36; cheap pellets kept under ~20–25 % of
feed, E3@50:18); the landlord vending deal; the noble-buyer window lure; battery double-charging
once about 8 cells are charging (E3@1:34).

### CthuLuck — Gunsmith on hard

- **Start.** Gunsmith on Thin Margins.
- **Engines.** The 3D printer every night, custom gun orders (the day-19 realisation: custom
  orders pay far more than counter sales, so mods and good guns are saved for them), nightly
  scavenging, wholesale weapon lots at 15 % off, contraband and medical sold to Odin right
  before rent.
- **Risk.** Skips hot goods when there is nowhere to hide them; keeps contraband apart so an
  inspection would cost one item; declined evidence removal at 229.
- **Cash.** Tight rent tracking; nothing discretionary within 2–3 days of rent; bought at −25 %
  or better. Learned the burglar grace lasts three nights by losing a gun on night 4.

### Darlom — Rock Bottom, Red Ink, Sentinel

- **Start.** Rock Bottom on Red Ink with Sentinel (weekly Immunivax) and a makeshift reactor.
- **Engines.** Smelting (two smelters, quality modules), appraisal (named as the route to the
  36,000 deed), nightly scavenging, batteries bought empty and sold charged, injectors traded for
  Immunivax to a three-week buffer, and a printer.
- **Risk.** Lets heat die; never sells stolen goods to Security; pays inspection bribes; checks
  every injector batch against the recall range.
- **Cash.** Spends on production capacity first, on the view that only scaling outruns rent;
  sells at the floor price when rent is due.

### TheNeomare — Rock Bottom on hard, honest water and appraisal

- **Start.** Rock Bottom on Thin Margins with Working Pipes, Model Citizen, Vigilant (no
  overnight theft), Careful and Proficient Scavenger, appraisal licence and Colorful Character,
  paid with Ascetic.
- **Engines.** Scavenging covers the first rent; the Odin bounty (450) pays for the Water Trader
  permit on day 2; appraiser board on day 4.
- **Order.** Water Trader (day 2) → filter (day 3) → appraiser board (day 4) → test kit.
- **Risk.** None taken: reports Odin for the bounty and never oversells water quality.
- **Cash.** Buys water only at 15 %+ off; janitor every second day, no power service; holds
  cash before rent.

### Sileevio — Rock Bottom, Red Ink, survival first

- **Start.** Rock Bottom on Red Ink with the no-night-robbery perk and Unlucky.
- **Engines.** Scavenging and counter sales only in the first fortnight; event speculation on
  medical goods from day 8.
- **Cash.** Rent above everything; buys only at a stated discount (~25 %); janitorial before
  power once there is a cushion; contraband refused until the cushion exists.

## Habits that recur

From the cross-creator pass (`var/expert/habits.json`, 38 habits) and checked against the
per-creator digests. Counts are of the 9 creators above (Harvey included where the screen shows
it).

| Habit | Who (of 9) | Who does the opposite |
|---|---|---|
| Scavenge the dumping ground almost every night early, with a bandage or Giorgio's protection | 8 (all but Harvey, who went 4 of 20 nights) | HeyPashi stops once water out-earns it (~day 20) |
| Buy stock only at a real discount (wholesale, 15–25 %+ off); refuse full-price sellers | 7 | exceptions for exclusive items or a rent emergency |
| One production engine early and reinvest into it (water, smelting, printer) | 7 | Sileevio and Aliensrock stay traders in the mined episodes |
| Janitor on a cadence (about every second night), insurance only after the 3-night burglar grace and when stock justifies it | 7 | CthuLuck learned the grace by losing a gun |
| Track rent days ahead; no big buys in the 2–3 days before it | 7 | Aliensrock and PerunGamingAU go to zero and liquidate (Perun saves for the permit by day 38) |
| Stockpile ahead of a shortage and sell into the spike; hold through surpluses | 6 | TheNeomare and HeyPashi refuse food bought during a shortage |
| Appraisal service as a main income once affordable (day 4–5) | 5 | Fruit Newt defers it |
| Capacity on sight (backpack, satchel, container, fridge) | 6 | |
| Let stolen goods cool before selling; never sell them to Security | 5 | nobody |
| Sell to the specialised buyer (Odin for contraband, tool buyer for materials, Dr. Jackson) rather than discount to whoever is there | 6 | |
| Verify counterfeits, batch numbers and condition before buying or pricing | 5 | |

**Where strong players disagree** (the most useful arms): all-in reinvestment versus a rent
buffer (Harvey, Aliensrock, PerunGamingAU vs CthuLuck, Fruit Newt, Sileevio, TheNeomare);
contraband through Odin (Aliensrock, CthuLuck, PerunGamingAU) versus reporting him
(TheNeomare) or staying out of contraband (Darlom);
nightly scavenging versus engine time (most vs Harvey, HeyPashi late).


## Screen-reading pilot

Can a local vision model recover a trajectory from an unnarrated video? Harvey's Ep1 (34 min)
at 480p, 20 frames every 100 s, read by `localmodel/gemma-4-31b-mm` on the 5090 (game not
running; model unloaded afterwards), checked by eye against the frames
(`var/expert/frames/groundtruth-cSWZ2gd_5II.json`).

| Field | Visible in | Read correctly | Wrong | Missed |
|---|---|---|---|---|
| Day number | 15 | 14 | 0 | 1 |
| Days until rent | 14 | 13 | 0 | 1 |
| Cash, first prompt | 14 | 8 | 4 | 2 |
| Cash, revised prompt (9 hardest frames re-read) | 14 (est.) | 11 | 3 | 1 |
| Prices and totals in trade panels | 12 values | 12 | 0 | 0 |
| Item estimates in hover tooltips | 6 | 2 | 0 | 4 |
| Customer budget (field added in the revision; only frame 1900 re-read) | 1 | 1 | 0 | 0 |

- Cash errors are systematic: two-digit amounts with the ₵ glyph ("28₵", "33₵") come back as 280
  and 330. 720p fixed one of the three. The model also reported a phone number (8376) as cash
  on the frame where the phone covers the cash display. A crop of the cash display plus a
  continuity check (cash moves only by visible sales, buys and bills) should remove both.
- Screen type was right on 16 of 20 (shop, appraisal and Dr. Jackson's stall get called
  "counter").
- Not attempted: reputation bars, inventory contents, upgrade purchases. The end-of-run summary
  screens (sales, where the money went, nights, perks, highlights) are the richest single source
  and sit in the last minutes of a winning video.
- Cost: 9–10 s a frame warm (12.3 s including the load), ~400 prompt + ~145 completion tokens.
  One frame per 30 s of video is ~20 min of 5090 time per video-hour; one per 10 s is about real
  time. 480p video is ~90 MB per hour.

**Verdict: near-trajectory extraction is feasible** for day, rent countdown, cash (after the crop
and continuity fixes) and every trade panel (items, prices, totals, budgets), i.e. a
per-customer sales log and a daily cash curve. Not feasible as is for reputation, stock or
upgrade choices; those need targeted frames (Wild's Network window, the end screens) or a
stronger reader.

## Rebuilding

`var/expert/sources.json` (the ranked candidates, `pick` marks the mined videos), then:

```
go run ./cmd/fenceexpert fetch       # transcripts via the knowledge fetcher (uvx yt-dlp)
go run ./cmd/fenceexpert extract     # GLM on heavy, one request in flight → var/expert/extract/
go run ./cmd/fenceexpert merge       # → decisions.jsonl, runs.json
go run ./cmd/fenceexpert summarize   # → creators/<creator>.json
go run ./cmd/fenceexpert habits      # → habits.json
go run ./cmd/fenceexpert frames -video ID [-every 100 | -at 300,600] [-n 20] [-height 480]
```

`frames` refuses to start, and stops, while `Probably Stolen.exe` runs, and unloads the vision
model when it finishes. Every stage caches per video or per chunk; an empty extraction is not
cached, so a rerun retries it.
