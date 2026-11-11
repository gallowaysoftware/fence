# Concealment: where an item can sit, and who finds it there (RE notes)

Same decompile as `BARTER.md` (Assembly-CSharp 0.46D). Question answered here: for every place
an owned item can be, who or what can take it away (inspectors by personality, security visitors,
burglars, the repo man, the night cleanup), how likely that is, and how to hide stock well.

Tags: **MEASURED** = checked headless in `sim/lab` (`conceal.*` experiments; verdicts in
`docs/sim/VERIFIED.md` "Concealment"); **CODE** = read from the decompile only.
Related: `MITIGATIONS.md` §3B (inspection outcomes, Clemency, bribe), `STATE.md` (heat),
`WINDOW.md` (showcase inspector), `PERKS.md` (Security thresholds, luck).

## 1. Who looks

| Finder | When | Looks at | Odds of a find | Evidence |
|---|---|---|---|---|
| **Scheduled inspector** | countdown while trafficking crimes (incl. FENCING) ≥ 50; every 3–5 days at < 150 down to 1–2 at ≥ 750 (uniform); none at Security 100 (`SEC_VIP`) | stock, front counter, back counter, showcase, **trash can unless lazy**, **poster stash only while the poster is up**; recursively into every container **except smuggler bays** | contraband: certain; stolen: heat %; then a luck roll (luck %) voids the whole find | MEASURED; `ContrabandHelper.StartInspection(int, bool)`, `RecursiveInspect` |
| Personality | rolled per scheduled inspector: normal / shady / lazy, ⅓ each; always normal once bribery crime ≥ 500 or at Security ≤ −80 (`SEC_THOROUGH_INSPECTION`). The name says which ("Shady/Lazy Security Inspector") | shady = normal search, but offers a bribe `clamp(100 + 2·day − 2·Sec, 40, 400)` that cancels it; lazy = skips the trash can | — | MEASURED (shares, bribe formula, lazy) |
| **Showcase inspector** | queued at the opening, no warning, when the showcase holds contraband of **any** level (always) or a stolen item (heat % roll, then heat 100) | full search (same as normal), no +3 on a clean result | as above | MEASURED (`WINDOW.md` rows in VERIFIED.md) |
| **Security visitor arrival check** | every security client on arrival (patrol officer, requisition officer, Officer Lun, off-duty officers, aug all-clear officer, permit-revoking officer, inspectors themselves; not the sentencing officer) | **top-level** items on the front counter, back counter and showcase only: contraband of any level always; stolen with chance heat % (then heat 100) | a hit turns the visit into a **full search of the whole shop**, trash included (a lazy inspector still skips it) | MEASURED (patrol officer; bay on the counter is invisible) |
| Security visitor present, item dropped | owned contraband put on either counter or the showcase while a security client is at the counter (stolen: heat % roll) | starts a full search (no +3) | — | CODE; the handler fires when called directly, headless placement does not reach it (VERIFIED.md "can't test") |
| **Fake inspector** (Leroy, `wanted9`, a wanted-poster criminal) | unannounced (never in the early warning) | full search, but outcome capped at seizure: no Security loss, no arrest, no slip. He is dismissable and reportable (bounty 300) | as above | MEASURED (unwarned arrivals are all `wanted9`); outcome CODE |
| **Burglary** (night) | only once the insurance service is unlocked; uninsured: 33 % + 5 % per uninsured night (+33 `LLTARGET`, −16 `SEC_PROTECTION`, none at `SEC_VIP`, with cartel protection, or the Alert perk); an alarm may stop it | one random item from stock, both counters, showcase, trash can, dossier, vending, outside pockets, recursively; **not** the poster stash, nothing inside a bay, no bays, no machines/documents/locked/important/zero-value items | 38 % on the first uninsured night | MEASURED (pool + odds); `PlayerStore.HandleInsurance`, `GetRandomBurglaryEligibleItem` |
| **Repossession** (mortgage only) | weekly when you skip a payment | same pool as burglary, best fit worth one payment | — | CODE (`PlayerStore.IsEligibleForRepo`, `GetAllRepoEligibleItems`) |
| **Night cleanup** | every night | front counter: **everything** destroyed (owned or not); trash can: destroyed if the janitorial service ran, except hazardous waste and a **modified** bay (and its contents) | certain | MEASURED |
| BM Exchange | nightly, needs the janitor service and the Exchange | consumes barter inputs from the trash can | — | CODE (`ExchangeManager.HandleExchange`) |

Not finders: aug scanners (they scan customers), `HYPERTECH_SMUGGLER` (a supplier service), the
evidence investigation (it reads the crime ledger, never the shelves).

## 2. Hiding places

Footprints are grid cells (w×h). "Inspector" = scheduled, showcase and visitor-triggered searches
alike unless noted. All rows MEASURED unless tagged CODE.

| Place | Capacity | How you get it / cost | Inspector: normal / shady / lazy | Visitor arrival check | Burglary & repo | Overnight |
|---|---|---|---|---|---|---|
| Stock | 24×10 (wider with stock expansions) | always | searched / searched / searched | no | yes | kept |
| Back counter | 18×4 | always | searched ×3 | **yes** | yes | kept |
| Front counter | 18×4 | always | searched ×3 | **yes** | — (wiped nightly) | **destroyed** |
| Showcase | 7×5, 8×6 (`SHOWCASE_I`), 9×7 (`SHOWCASE_II`) | always | searched ×3; contraband/stolen here at the opening **summons** an inspector that day | **yes** | yes | kept |
| Trash can | 10×10, 17×10 (`TRASHCAN_UPGRADE`); owned items only | always | searched / searched / **skipped** (and a lazy inspector who skips it still pays +3: the trash is not "concealed") | no | yes | janitor on: **destroyed**; off: kept (garbage +2/night) |
| **Poster stash** | **2×2**, owned items only | always; click the wall poster | poster **down**: never searched (counts as concealed: no +3); poster **up**: searched ×3 | no | **no** | kept |
| Smuggler's Bay (Mini) `smuggler_bay_mini` | footprint 2×2, **3×3 inside** | Drug Dealer start (shabby) — CODE: no other source found | **never** (any placement: stock, counter, showcase, trash, poster stash open or closed) | no (invisible even on the counter/showcase) | no (bay and contents) | kept |
| Smuggler's Bay `smuggler_bay` | footprint 3×3, **5×5 inside**; value 250 | Fence start (shabby); the Inventor (day ≥ 12, every `day % 7 == 5`, plus story visits): offered 66 % if you own none, 25 % if you do; Lou's thank-you gift (one of three) — CODE | never | no | no | kept (in the trash can: destroyed with the janitor on) |
| Smuggler's Bay (Modified) `smuggler_bay_mod` | footprint 3×3, **8×8 inside**; value 400 | BM Exchange barter: a plain bay + 8 pink injectors left in the trash can → delivered to the trash can (one per night) — CODE | never | no | no | kept, **even in the trash can with the janitor on** |
| Plain container in any searched place (first-aid box, backpacks, crates) | its grid | — | searched (recursion) | top level only | yes | as its place |
| Plain container **inside a bay** | — | — | never | no | no | kept |
| Locked supply crate (contents shipped behind a lock) | its grid; you cannot add to it | bought locked | contents skipped while locked — CODE | no | no — CODE | kept |
| Dossier | 10×10 (17×10 `DOSSIER_UPGRADE`) | always | never searched, but it only takes documents, guides, signs, labelers, magnifier, scav tokens, lottery papers, the gambler's coin: **no contraband item fits** (measured over every item) | no | no for documents/tools | kept |
| Vending machine | its grid | landlord deal, +150 rent per week | never searched — CODE; refuses contraband, accepts stolen goods, which it sells overnight through the normal sold-item check | no | yes — CODE | may sell |
| Outside (afterhours pockets/backpack) | small grids | at night only | never — CODE | no | yes — CODE | items left on the ground at a location are lost |

Nesting rules (MEASURED): a bay never goes inside another bay; the mini bay (2×2) fits the poster
stash, the trash can, both counters and the showcase; the 3×3 bays fit everything except the
poster stash and the dossier. A bay's contents are protected wherever the bay sits.

What fits (MEASURED over every contraband item, 56 in all): the 2×2 poster stash takes 35 —
the 1×1/1×2/2×1 drugs, pills, ammo and crypto modules and the 2×2 keycards, grenades, perfume and
neural cores (both critical items fit). The mini bay (3×3) takes 41: add c4, the short revolvers,
the desequencer and the restricted chemical (3×2). The plain bay (5×5) takes 51: add the
handmade/heavy pistols and revolvers (4×3, 5×3) and the 2×5 canister. Only the modified bay (8×8)
takes all 56, including the 6×4 evidence/security boxes, the stun baton (6×1) and the
shotgun/SMG (7×3).

## 3. What a find costs

Per scheduled or triggered inspection (MEASURED unless CODE):

- **Found set.** Every flaggable item in a searched place: contraband always; a stolen
  non-contraband item with probability heat/100 (heat falls 18 per night, `STATE.md`). Then one
  luck roll (`HealthData.GetLuck()` = luck + coin + liquid luck, in %) clears the whole set.
- **Outcome by the worst item.** Low: warning, items kept. Mid or stolen: everything found (except
  low) is seized. High: seizure plus a **fine = Σ unit values of all found items**; cash short →
  arrest (game over) unless Clemency (`SEC_GRACE`) is armed. Critical: **arrest** unless Clemency
  (then seizure + warning). CODE for high/critical (`MITIGATIONS.md` §3B).
- **Security reputation.** −Σ (1 + contraband level + stolen tier), capped at 25, where the stolen
  tier is +1/+2/+3/+4 for base value < 50 / < 85 / < 140 / ≥ 140. Measured: low −2, mid −3, low +
  mid −5, a stolen 200-base item −5. A clean inspection gives **+3 only if nothing flaggable was
  concealed** anywhere (bays, closed stash) and it was not a showcase inspection or a search set
  off by dropping an item in front of security; a lazy
  inspector who skipped a trash can full of contraband still gives +3.

Expected loss of keeping item *i* in a searched place for one inspection:

```
EL_i = p_i · (1 − luck) · (V_i + k · rep_i) + P(outcome ≥ high) · (fine + arrest risk)
p_i  = 1 for contraband, heat_i/100 for stolen goods
rep_i = 1 + level_i + tier_i          k = what one Security point is worth to you
```

and the cost of concealing everything instead is `3k` per inspection (the forfeited clean bonus),
plus the bay's price once. Per day multiply by the inspection rate: ¼ per day at 3–5-day
spacing, ⅓ at 3, ½ at 2, ⅔ at 1–2.

Worked examples (luck 0):

| Item, place | Per inspection | Over its life |
|---|---|---|
| Stolen 200-base item, heat 75, in stock, 3–5-day spacing | 75 % to lose it + 5 Security | heat 75 → 57 → 39 → 21 → 3 → 0 over five nights at ¼ inspection per day: 1 − Π(1 − ¼·heat) ≈ 41 % that it is seized before it is clean |
| Same item behind a closed poster or in a bay | 0 | 3 Security per inspection forgone while it hides |
| Mid contraband (e.g. a keycard) in stock | certain seizure + 3 Security | — |
| High contraband (c4) in stock | seizure, fine ≥ 250, −4 Security; **arrest if cash < fine and no Clemency** | at 3–5-day spacing one or two inspections a week: never leave it unhidden while the countdown runs |
| Critical (uncapped neural core) anywhere searched | **arrest** unless Clemency | always hide (fits the 2×2 stash) |
| Anything on the counter or showcase when a security visitor walks in | contraband: certain full search; stolen: heat % | visitors are unannounced: never leave flaggables there |

## 4. How much warning there is

| Arrival | Warning |
|---|---|
| Scheduled inspector | With the **Inspection Early Warning** service (`INSPECTION_INFORMANT` 300, then `SEC_INFORMANT` 10/night): night-log "imminent" the night **before** every scheduled inspector (MEASURED, 398/398); "tomorrow" = **2** openings away; "in 2 days" = **3** openings away (the texts read one day early). At ≥ 750 trafficking the countdown can restart at 1, so "imminent" may be the only line. Without it: none; the countdown is hidden |
| Showcase inspector | none, but it is your own doing: it only comes when the showcase holds contraband or stolen goods at the opening |
| Security visitors | none (patrol officer is a one-off minor client; the requisition officer comes every `day % 7 == 4` with `SEC_REQUISITION`; story officers on fixed days) |
| Fake inspector | none (wanted poster tells you who he is) |
| Security operations (crackdown, raids, extra patrols, martial law) | queued 4 days ahead; the calendar shows them only with `SEC_TRUSTED` (Security ≥ 60). A crackdown blocks going out at night. The game's event-action table has an inspector keyed "crackdown"; whether it is ever wired to the operation was not checked. There are no THREAT-type events (their blueprint list is empty, `docs/sim/SPEC.md`) — CODE |
| Burglary | the night log after the fact; uninsured nights are announced as they accrue |

## 5. Strategies

### Rules that hold at every stage
1. **Counters and showcase are never storage.** They are the only places checked by every
   security visitor, unannounced, and one hit escalates into a full search of the whole shop.
   Stage a flaggable item there only while serving the buyer, and take it back the moment the
   customer leaves. Never put contraband (any level) in the showcase: it guarantees an
   inspector at the next opening. A *bay* on the counter or in the showcase is fine (invisible).
2. **Nothing stays on the front counter overnight.** It is wiped, owned items included.
3. **Hide by consequence, not by value.** Critical, then high (arrest and fines), then mid and
   hot stolen goods by `p · (value + k · rep)`, then low contraband last (a warning only,
   though −2 Security each and it adds to a mixed find's rep loss).
4. **Close the poster.** An open poster stash is searched like stock. Every stash access is
   lift → place → lower.
5. **Hiding costs 3 Security per inspection** while anything flaggable is hidden. When you hold
   nothing worth more than 3 Security points of risk, carry nothing hidden and collect the +3.
6. **Shady inspector:** bribe when the bribe is below the expected loss of what is exposed, but
   remember each bribe feeds the bribery tracker (≥ 500 bribery crime → only normal inspectors).
   A shady inspector who sees contraband or stolen goods on a counter/showcase as he arrives
   **skips the bribe offer** (MEASURED): keep the counters clean so the option exists.
7. **Lazy inspector and the trash can:** a lazy inspector skips the trash and still pays +3.
   It is a fallback, not a plan: personality shows only on arrival (whether you can still move
   items then was not tested), two in three inspectors do search the trash, and the janitor
   empties the trash every night.
8. **Burglary:** the poster stash and bays are the only theft-proof places; with insurance
   running there is nothing to protect.

### By run stage

| Stage | Situation | Cheapest sufficient concealment |
|---|---|---|
| Days 1–12, trafficking < 50 | no scheduled inspections at all | Keep flaggables off the counters and showcase; stock is safe. Use the poster stash (closed) for the most valuable 2×2-or-smaller hot item, mostly against burglary |
| Fence start | a plain bay (5×5) and dream dust from day 1; heat plan required | Everything hot goes in the bay; the stash takes the overflow (≤ 2×2) |
| Drug Dealer start | a mini bay (3×3) | Mini bay for c4-sized items; it also fits inside the stash if stock space is short |
| Mid run, trafficking ≥ 50 (inspections every 2–5 days) | the countdown is on | Buy the Inspection Early Warning (300 + 10/night): stock between inspections, conceal on the "imminent" night. Buy a bay from the Inventor as soon as offered (visits on `day % 7 == 5`). Cap hot and contraband stock to concealment capacity: ~4 cells (stash) + 25 (bay) |
| Heavy contraband (high/critical), long guns | arrest risk | Modified bay (8×8) via the BM Exchange (bay + 8 pink injectors); Clemency armed as insurance against a slip |
| Late, Security ≥ 60 / 100 | Trusted: +2/+4 days between inspections, operations visible; VIP: no inspections, no burglary | At VIP concealment only matters for security visitors' counter check and showcase. Below 100, the +3 per clean inspection is how you climb: stop hiding things you could sell or let cool |

### Timing
- With early warning: on the "imminent" night conceal everything flaggable that fits, close the
  poster; after the inspector leaves, take items back out as needed. "Tomorrow" is the night to
  sell down or move what will not fit.
- Without it: while the countdown runs, keep everything high/critical concealed at all times
  and hold stolen stock until heat 0 only within concealment capacity (`EXPERIMENTS.md` caveat).
- Stolen goods cool 18 heat per night: a heat-25 item is clean after 2 nights, so hiding it for
  two nights is usually cheaper than selling it hot (fencing crime, and a visitor may spot it on
  the counter).

## 6. What the gym player needs

Today the executor cannot stage anything into a bay or the stash (`EXPERIMENTS.md`), so held hot
stock and contraband sit in stock. Moves it needs, all on real footprints:

| Move | Effect | When |
|---|---|---|
| `stash(uid, into)` | move an owned item into `bay:<uid>` (any smuggler bay), `poster`, `trash`, or a plain container | conceal pass |
| `unstash(uid)` | back to stock | to stage it for a buyer, or after the inspection |
| `poster(up\|down)` | open/close the stash; every stash access ends with `down` | around `stash`/`unstash` into `poster` |
| conceal pass | rank flaggable items (critical, high, then `p · (value + k · rep)`), bin-pack largest-first into bays, then ≤ 2×2 items (or the mini bay) into the stash; report what did not fit | the night of an "imminent" warning; every night under `always`; after any purchase of flaggable goods |
| counter sweep | before each opening and after each customer: no contraband or stolen item left on either counter or in the showcase (bays excepted) | always |
| bribe | pay the shady inspector when the bribe < expected loss of what is exposed | shady arrival |

Directive fields (one object, every field optional):

```json
"conceal": {
  "policy": "always | on_warning | never",
  "min_level": 2,
  "max_exposed_heat": 25,
  "use_poster": true,
  "use_trash_if_lazy": false,
  "bribe_max": 200,
  "rep_value": 10
}
```

- `policy`: `always` keeps flaggables concealed whenever the countdown runs; `on_warning` conceals
  on the "imminent" night (needs the early-warning service; falls back to `always` without it);
  `never` = today's behaviour.
- `min_level`: lowest contraband level worth hiding (1 = everything; 2 leaves low contraband out,
  since a find is only a warning).
- `max_exposed_heat`: stolen items above this heat are concealed; below it they may stay in stock.
- `use_poster`: allow the stash (and keep the poster down).
- `use_trash_if_lazy`: dump into the trash can when a lazy inspector arrives (only if moves are
  still possible then).
- `bribe_max`: pay a shady inspector up to this much (0 = never).
- `rep_value`: credits per Security point (`k` in §3), for the ranking and the bribe rule.

The snapshot should carry, per owned item, its place (`stock`, `back_counter`, `showcase`,
`poster`, `trash`, `bay:<uid>`, `container:<uid>`), footprint, contraband level and heat; per bay,
its free grid; the poster state; the night log's inspection-warning line; and on an inspector, his
personality and the bribe. The day result should list what was concealed, what was found, and what
did not fit.

## 7. Corrections to other docs (not made here)
1. `DAY.md` §1 (`hidden` row): "only searched when the poster is pulled down" is backwards. In
   code `StorePoster.PullUp` sets `isPulled = true` and shows the stash (searched);
   `PullDown` covers it (`isPulled = false`, not searched).
2. `DAY.md` security-patrol row and `MITIGATIONS.md` §3B: the arrival check takes contraband of
   any level, stolen goods only with chance = heat; it looks at top-level items only (a bay on the
   counter hides its contents); a hit leads to a full search of the whole shop.
3. `MITIGATIONS.md` §3B early-warning row: the "imminent" line comes the night before; "tomorrow"
   means two openings away, "in 2 days" three. The fake inspector (`wanted9`) is never announced.
4. `MITIGATIONS.md` §3B: the clean-inspection +3 is withheld whenever anything flaggable is
   concealed (bays, closed stash), and on showcase inspections and searches set off by dropping an
   item in front of security.
