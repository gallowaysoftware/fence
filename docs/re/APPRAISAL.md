# Appraisal board (RE notes)

Same decompile as `BARTER.md` (build 0.46D). Question answered here: how the appraisal side job
works end to end, what a player can know when estimating, and what it pays. The per-item value
rules (hand, ribwich, plasma cell) and the inspection view are in `INSPECT.md`, "Appraisal
service"; this file covers the board, the clients, the fee and the rank ladder. Numbers are
the vanilla tables; difficulty multipliers come from `Difficulty` (cozy / normal / hard / challenge).

## Joining

| Step | Method | Notes |
|---|---|---|
| Offer | `StoreClientManager.HandleAlwayClient` adds `StoreClientListStory.CreateStoryAppraisalBoard()` on day 8 (day 5 with skip-intro, `HandleSkipClient`) | A board member (`appraisalBoardIntro`, dialogue client, not shootable). |
| Choices | Last line's choices, in order: decline, come back later, join | "Join" is enabled while cash ≥ the fee. "Later" queues `appraisalBoardIntro2` (`CreateStoryAppraisalBoard2`, same choices) 7 days out, as often as asked. Decline is final. |
| Fee | `PlayerStore.GetAppraisalJoinCost()` = 85 × `appraisalJoinCostMultiplier` | cozy 85, normal 85, **hard 170**, challenge 255 |
| On join | the join line's start action | Pays the fee; puts on the counter, owned: a mentor note, `hand_guide`, a `magnifier` and the junior certificate `cert1`; sets `isAppraisalIntroduced` (and `appraisalIntroducedDay`). |
| First client | day 9, `StoreClientListAppraisal.CreateAppraisal1()` if already joined | A scripted scavenger with two random hands. |

The counter is wiped every night (`PlayerStore` nightly handler → `EmporiumEntry.ClearFrontInv()`,
which destroys everything on it, owned or not), so the kit has to be put away the day it is handed
over. Only the certificate comes back by itself: `StoreShutterButton` → `AppraisalRankHelper.ReissueLostCertificate()`
puts a new one of the current rank in stock at opening when none is owned.

## The certificate

`AppraisalRankHelper.InitCertificate`: a `DOCUMENT` worth 0 with `IMPORTANT_TAG`; activating it opens
`AppraisalItemsWindow` (below). Its tooltip (`CreateCertificateTooltip`) prints the rank's base fee,
the fee per grade (or "refused"), today's client chance, the item types the rank is qualified for,
and the promotion progress in percent.

**It must be in the display case.** `ShowcaseHelper.GetDisplayCasePrediction()` adds the client
`scavAppraisalRandom` only when `AppraisalRankHelper.GetDisplayedCertificate()` finds a certificate
among `showcaseElement.childItems`. In stock it does nothing. Appraisal clients come from nowhere else
after day 9.

## Appraisal clients

- **Roll.** `StoreClientManager.HandleNormalClient()` walks the display-case predictions in order and
  rolls each one's chance (`RNG.Roll`) until the day's normal-customer count is reached; every hit
  takes one of those slots. So an appraisal client replaces an ordinary customer, it does not add one.
  The prediction list puts the certificate after gun buyers, scav tokens, Black Lamp invitations and
  the supply signs, and before the firearm signs.
- **Rate.** `AppraisalRankHelper.GetAttractionChance()` = min(100, base × (2 + days missed) / 2), base by
  rank 33 / 42 / 48 / 55 %. `OnCustomerRolled` resets the missed-day counter
  (`StoreClientManager.appraisalDaysMissed`) on a day the client came and adds one on a day the
  certificate was shown but no client rolled. The chance depends only on rank and the streak: not on
  reputation, attractiveness or difficulty (attractiveness only sets how many normal slots there are).
  At Master the chance is 55, 82, 100 % after 0, 1, 2 missed days: a client on most days.
- **Items.** `CreateAppraisalRandom()`: a lower-level scavenger (`APPRAISAL_SERVICE` intent) whose second
  line puts 2–4 items (`Random.Range(2, 5)`) of **one** type on the counter, the type drawn uniformly
  from `AppraisalItemRegistry.GetPool(rank)`: hand from Junior, ribwich from Appraiser, plasma cell from
  Senior, minus blocked types. Items carry `APPRAISAL_ITEM` and a pre-filled customer figure
  (`APPRAISAL_DEFAULT_VALUE`, the type's base value), which says nothing about the item.
- **Blocking.** The certificate's window (`AppraisalItemsWindow`) toggles types into
  `PlayerStore.appraisalBlacklist`. Below Master only types older than the newest unlock can be
  blocked; at Master any; at least one stays allowed (`AppraisalItemRegistry.CanBlock`, `IsLastAllowed`).

## Estimating

The player inspects each item with the magnifier (`InspectionUIManager.OnInspectItem`): the item's
view (`AppHandUI`, `RibwichUI`, `FuelUI`) and, while the current client is an appraisal client, the
appraisal panel (`UpdateAppraisalPanel`): a number field, ± buttons, and submit
(`OnAppraisalServiceEstimated` → `AppraisalServiceHelper.OnEstimate(item, estimate)`). The true value
(`AppraisalHelper.GetCorrectBaseValue` → the type's `getCorrectValue`) is a fixed function of the cues
the view draws (`INSPECT.md` says which cues; the numbers are in "The value table" below), so a player with the type's guide can compute
it. Two caveats: a double-meat ribwich hides its top bun and pickles (`RibwichUI.OnInit`), and they are
always plain on such a ribwich (`RibwichHelper.InitCorrectDoubleMeat`); and two ribwich counterfeit
tells are sprite variants of the logos, which need the guide's reference picture.

`OnEstimate`, in order:
1. Impatience already ≥ 100: the client gives up (dialogue, dismissed).
2. Refusal (`IsAppraisalAcceptable`): below 50 % or above 150 % of the true value, or a grade the rank
   does not pay (`AppraisalRankHelper.IsGradeAccepted`). The client says "too low" or "too high",
   impatience +49, and `OnEstimateRefused` takes progress for grades whose progress entry is negative.
   Reaching 100 impatience (the third refusal from a calm client) is a walk-out: `OnCustomerWalkout`
   (−5 progress at Appraiser and Senior) and the client leaves with the items.
3. Accepted: `GiveAppraisalFee`, `OnEstimateAccepted` (progress), the item is destroyed, and when the
   counter has no more of the client's items the client completes and leaves.

**Grades** (`GetEstimatePrecision`, percent of the true value, integer division as `RNG.CalculatePercentage`):

| grade | below | above |
|---|---|---|
| 1 perfect | ≥ 95 % | ≤ 105 % |
| 2 excellent | > 90 % | < 110 % |
| 3 good | > 85 % | < 115 % |
| 4 mediocre | > 75 % | < 125 % |
| 5 awful | ≤ 75 % | ≥ 125 % |

`AchievementTracker.CheckMasterAppraiser` counts perfect grades (an achievement, no effect on play).

## The fee

`AppraisalServiceHelper.GiveAppraisalFee`: **base fee + ⌊estimate × percent / 100⌋**, paid in cash at
once, per item, and a slip (`PreBuiltItemHelper.CreatePaperWithText`, worth 0) on the counter giving
the grade, the accuracy in percent, the estimate, **the true value** and the fee breakdown. The
percent applies to the **estimate**, not the true value, so within a grade a higher estimate pays
more: aiming at the top of the perfect band (105 %) earns 5 % more than an exact estimate.

| rank (`AppraisalRankHelper.Ranks`) | base fee | perfect | excellent | good | mediocre | awful | client chance |
|---|---|---|---|---|---|---|---|
| 0 Junior (`cert1`) | 20 | 15 % | 12 % | 10 % | 8 % | 5 % | 33 % |
| 1 Appraiser (`cert2`) | 40 | 18 % | 15 % | 12 % | 10 % | refused | 42 % |
| 2 Senior (`cert3`) | 60 | 21 % | 18 % | 15 % | refused | refused | 48 % |
| 3 Master (`cert4`) | 100 | 25 % | 21 % | refused | refused | refused | 55 % |

So a Master's perfect estimate of a 300 hand pays 100 + 315 × 25 % = 178; a visit brings 2–4 items.

## Rank and promotion

Progress (`PlayerStore.appraisalProgress`) per accepted estimate by grade, before the difficulty
multiplier (`appraisalProgressGainMultiplier`: cozy 1.25, normal 1, hard 1, challenge 0.75; losses ×
`appraisalProgressLossMultiplier`: 0.5 / 1 / 1.5 / 2):

| rank | perfect | excellent | good | mediocre | awful | walk-out | to promote | promotion cost (× join fee) |
|---|---|---|---|---|---|---|---|---|
| Junior | 10 | 7 | 5 | 4 | 3 | 0 | 100 | 1.25 (hard 212) |
| Appraiser | 10 | 7 | 5 | 4 | −3 | −5 | 150 | 1.5 (hard 255) |
| Senior | 10 | 7 | 5 | −3 | −4 | −5 | 250 | 1.75 (hard 298) |
| Master | — | — | — | — | — | — | — | — |

A refused estimate only applies the negative entries (`OnEstimateRefused` takes min(0, entry)).
Progress is capped at the threshold and stops while a promotion is pending. With perfect estimates
the ladder is 10 + 15 + 25 = 50 items, about 17 visits.

On reaching the threshold, `appraisalPromotionDay` = tomorrow; `HandleAlwayClient` →
`HandlePromotionClient` then queues the board member's promotion offer
(`StoreClientListAppraisal.CreatePromotionOffer`, `appraisalBoardPromotion`) each morning from that day.
Choices, in order: accept (enabled while cash ≥ the cost: `GetPromotionCost` = round(join fee ×
the new rank's cost share), Unity's round-half-even), later (`DeclinePromotionForNow`: back in 7 days),
never (`DeclinePromotionForGood`: progress stops for good). `Promote()` raises the rank, resets
progress, swaps every owned certificate for the new one where the old one was (so a displayed
certificate stays displayed), and puts the new type's guide on the counter (`ribwich_guide` at
Appraiser, `fuel_guide` at Senior).

## Penalties, summed up

- Bad grade, accepted: a lower fee percent and less progress (no loss below Appraiser).
- Refused: no fee, +49 impatience, progress loss for the negative grades (× 1.5 on hard).
- Third refusal: walk-out, −5 progress (× 1.5 on hard) at Appraiser and Senior; the items leave.
- No reputation, crime or cash penalty anywhere in the flow.
- Opportunity cost: each appraisal client takes one of the day's normal customer slots, and the
  certificate takes room in the display case.

## In the gym

The headless game runs all of the above unchanged (board, join, nightly counter wipe, certificate
roll, items, grading, fee, promotions). Two things are the player's and are done by the executor
(`appraisal: join`, `sim/gym/Appraisal.cs`, `Episode.Appraisal.cs`):

- **Put the kit away.** Before this, a run that joined lost the magnifier and guide overnight with the
  rest of the counter; the executor now puts the kit in the dossier (`Dossier()` takes documents and
  the magnifier; stock if it is full) and keeps it out of sales. Slips go in the bin.
- **Inspect and estimate.** `InspectionUIManager` is a stubbed presentation manager headless, so the
  executor reads the cues the item's view draws (`Appraisal.Perceive`: material, interface marking,
  wear, add-ons; ribwich meat, bun and pickles unless hidden, wrap, logos against the reference,
  date and facility stamps when present; plasma colour, seal and casing marks), applies the guide's
  table (`Appraisal.Value`, our own code), and submits through `AppraisalServiceHelper.OnEstimate`,
  as the panel's submit button does. It needs the magnifier and that type's guide. The true value is
  logged as hidden, never read for the estimate.

## The value table

The guides' value tables are game content and are **not committed**. They live in
`local/appraisal_guides.json` (gitignored via `/local/`), or wherever `FENCE_APPRAISAL_GUIDES` points;
a git worktree, whose `local/` is empty, falls back to the main checkout's. Readers:

- the gym (`sim/gym/Boot.cs` loads it, `Appraisal.Value` applies it); a policy with `appraisal: join`
  throws without it;
- the bridge (`policy.AppraisalGuidesPath`), which values appraisals in the inspect rule check and
  states the tables in the inspect prompt; without it the value is unknown and the advice defers;
- the live mod, which has no file access: the bridge hands it the tables in the hello response
  (`appraisal_guides`, `docs/PROTOCOL.md`), and without them the autopilot does not join the board.

Format, per item type (`hand`, `ribwich`, `plasma`), over the cues as `Appraisal.Perceive` names them:

```json
{"base": B, "min": M, "invalid": V,
 "invalid_when": [{"cue": matcher, ...}, ...],
 "add": [{"when": {"cue": matcher, ...}, "add": N, "group": "name", "why": "label"}, ...]}
```

The value is `invalid` if any `invalid_when` condition matches; otherwise `base` plus the `add` of every
rule whose `when` matches (of rules sharing a `group`, only the first that matches), floored at `min`.
A condition matches when every cue in it does. A matcher is a value (equal), an array (any of),
`{"range": [lo, hi]}` (an integer cue within, inclusive) or `{"not": matcher}`; a cue the item does not
show (a hidden bun, a missing stamp's date) is absent and matches only under `not`. `why` is an optional
label for the bridge's rule check and prompt; the gym ignores it. The Go tests use a fictional table
in the same format, `internal/policy/testdata/appraisal_guides.fixture.json`.
