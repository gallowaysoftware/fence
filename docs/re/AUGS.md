# Augs: rules from the decompile, and the `aug_check` design

Build 26525552 / 0.46D. Same conventions as `DAY.md`: types are in the global namespace of
`Assembly-CSharp`; members are cited by signature. Everything in §1–§4 is read from code and
nothing is play-tested unless it says so. Numbers marked (inferred) are conclusions drawn from
code paths, not quoted rules.

Corrections to earlier notes:

- `AugHelper.OnAugLeftAlive()` only raises presence and writes a night-log line. The
  reputation losses (−15 Sec, −15 LL, −10 Rev, −10 BM) belong to the **wrongful kill**
  (`AugHelper.CleanupKill()`), not to letting an aug go.
- On cozy/normal, reaching 100 presence does **nothing**. `augWarning` is not a presence
  warning: it is a night-log heads-up that an aug is due the next day (§4.2).
- Presence never changes the spawn rate or the tiers. It drives two things only: the game-over
  check, and the all-clear that ends the aug threat (§1.6).

## 1. Generation

### 1.1 Introduction

| Rule | Value | Evidence |
|---|---|---|
| Intro day | 12 (1 with the Aug Menace perk) | `PlayerStore.GetAugIntroducedDay() : System.Int32` (const `AUG_INTRODUCED_DAY`) |
| Trigger | The mentor visits on day 12 if augs are not yet introduced. His dialogue unlocks the shotgun and hands over a mentor note plus the Augmented Identification Guide | `StoreClientManager` story block → `StoreClientListStory.CreateMentorShotgun()` → `StoreClientDialogList.MentorShotgunDialog()` |
| Unlock | First unlock sets presence to **15**, `isAugIntroduced`, `isShootingUnlocked`. Guarded by "not already unlocked", so it is a one-time reset. Re-arming after a confiscation (`ReturnIfLost`) does not reset presence | `Shotgun.Unlock() : System.Void` (static) |
| New game | presence also initialised to 15 | `PlayerStore` reset (`DEFAULT_AUG_PRESENCE`) |
| Aug Menace | augs from day 1: shotgun, guide and note in the starting inventory; first aug 2–4 days later; kill reward ×1.25; no scripted intro augs; never ends; game over at 100 on **every** difficulty | `StartingPerkContent` (Aug Menace unlock), `StartingPerk.IsAugMenace() : System.Boolean` |

### 1.2 Schedule

`StoreClientManager.HandleAugClient(System.Boolean isEarlyCall)` runs twice in
`GenerateClient()`: once before the day's wanted and normal customers (it skips 50 % of the
time) and once after them. A day spawns at most one aug (`isAugSpawnHandledToday`).

| Day | What spawns |
|---|---|
| intro + 1, intro + 6 (13, 18) | one **undisguised** aug each (medical buyer), scripted. Not with Aug Menace |
| first non-scripted call | sets `nextAugSpawnDay = intro + 6 + U{6..8}` (days 24–26) |
| day ≥ `nextAugSpawnDay` | one aug (§1.3); then `nextAugSpawnDay = today + U{6..8}` |

So there is one aug every 6–8 days, 7 on average: about 13 by day 100 on the default track. The
cadence is fixed by the RNG and the day counter alone. Neither presence nor kills move it.
Only augs that reach the counter count: an aug still queued when the shop closes changes
nothing (inferred: every presence change hangs off the counter exit, §3.2).

### 1.3 Disguised or not

`StoreClientManager.AddAugClient()`:

1. With 80 % chance it tries to **disguise**:
   a. it converts a customer already in today's queue who comes from an event, is buying
      MATERIAL or MEDICAL, can be dismissed, and is not Security or Revolution
      (`FindEventCustomerAugCandidate()`); or else
   b. if the display case shows MATERIAL or MEDICAL goods, it creates the buyer that display
      attracts (`ShowcaseHelper.CreateDisplayCaseAugBuyer() : StoreClient`, `WINDOW.md`).
   The chosen customer goes through `AugHelper.MakeDisguisedAug(StoreClient, System.Int32)`.
2. Otherwise, or when neither source exists, it spawns an **undisguised** aug: a medical buyer
   or a materials buyer, 50/50 (`StoreClientList.CreateAugMed(System.Int32)`,
   `CreateAugMaterial(System.Int32)`).

The window is therefore the player's lever: with no MATERIAL/MEDICAL on display and no matching
event customer queued, every aug comes undisguised.

### 1.4 What augs want

| | Undisguised | Disguised |
|---|---|---|
| Intent | BUY | the host's intent (always a buyer by construction) |
| Wants | medical: MEDICAL. Materials: MATERIAL + ENERGY_CELL | the host's tags (MATERIAL or MEDICAL) |
| Budget | uniform 150–250 | the host's |
| Name | a generic citizen name chosen by a faction roll: "Upper Level Citizen", "Thief", "Lower Level Citizen" (1/3 each) | the host's name |
| `clientFaction` | **left at `"NONE"`**: the roll only picks the name and the sprite family. The speech bubble is therefore plain grey | the host's faction (bubble colour normal) |
| Dialogue | tiered aug script (§2.2) | the host's normal dialogue |
| Sales | `SoldItem` marks items sold to an aug as non-returnable; feature exposures and ammo-box confrontations cost no rep (`!isAug` guards in `StoreClient.ClientExposeFeature`, `NegociationHelper.OnFeatureExposed`, `AmmoBoxHelper.HandleAmmoBoxCheck`) | same |

### 1.5 Tiers

`AugHelper.RollAugTier() : System.Int32`, with `t = clamp01((day − intro − 7) / 100)` and a
roll from 0 to 99:

| Tier | Share at t = 0 (days 12–19) | at t = 0.5 (day 69) | at t = 1 (day 119+) |
|---|---|---|---|
| 1 (subtle) | 5 % | 40 % | 75 % |
| 2 | 15 % | 20 % | 25 % |
| 3 | 40 % | 20 % | 0 % |
| 4 (blatant) | 40 % | 20 % | 0 % |

Augs start blatant and get subtler over about 100 days. Tier drives the undisguised script
(§2.2), the sprite odds (§2.1), and the tier-4 reaction to the gun (§2.3). A disguised aug gets
a tier too, but only the gun reaction uses it.

**Hold window.** Through intro + 7 (days 12–19, `AugHelper.IsInTierHoldWindow()`), an
undisguised tier-1/2 aug that rolled neither the aug sprite nor a mismatch gets the aug sprite
anyway. Both scripted intro augs therefore always show at least one visual or verbal tell.

### 1.6 Presence

`PlayerStore.augPresence : System.Int32` (HIDDEN). Every write:

| Event | Change | cozy / normal / hard / challenge | Evidence |
|---|---|---|---|
| Aug introduced / new game | set to 15 | | `Shotgun.Unlock()`, `PlayerStore` reset |
| Aug leaves alive | + `augLeftAlivePresenceRise` | +10 / +10 / +15 / +16 | `AugHelper.OnAugLeftAlive()` |
| Good kill | − `augKillPresenceDrop`, floored at 0 | −25 / −25 / −10 / −8 | `AugHelper.OnGoodKill()` |

There is no daily drift and no other writer. Presence is read in exactly two places:

- **Game over** (`EndOfDayUIManager`, after `PlayerStore.BeginDay()` and before the morning
  save): if presence ≥ 100 and (`DifficultyRules.augGameOverAtFullPresence` or Aug Menace),
  `ExecuteGameOver("aug")` runs ("Augmented": augs break in at night and implant you). This
  is a morning check: the visit that crosses 100 kills the run at the next sleep.
- **All-clear** (`PlayerStore.IsAugAllClearDue(System.Int32) : System.Boolean`). Never with
  Aug Menace. Otherwise it needs all of: day ≥ intro + 50 (62), presence **≤ 0**, a business
  permit, and that permit held at least 25 days. When due, a Security officer visits instead of
  an aug, `isAugEnded` is set, augs stop for good, and the shotgun is taken unless the player
  holds gun permit I (`StoreClientListSecurity.CreateAugAllClearOfficer()`).

On hard you survive at most 5 augs left alive with no kills (15 + 6×15 = 105 → dead). On
challenge it is also 5 (15 + 6×16 = 111). Each kill on hard buys back two-thirds of an escape.
On normal one kill (15 − 25 → 0) already qualifies for the all-clear.

## 2. Tells

Rule for the mod: describe what the screen shows. Never read `isAug`, `augTier`,
`augIdleTell` or `identifier` (suffix `_disguised_aug`, ids `aug_med` / `aug_material`)
into the prompt. Every tell below has a perception-legal member that carries exactly the
rendered state.

### 2.1 Visual

| Tell (guide wording, paraphrased) | What the player sees | Member to read (perception-legal) | Rates on augs | False positives on humans |
|---|---|---|---|---|
| **Unnatural stillness** | the customer does not bob at all while at the counter | `StoreClientMono.idleSuppressed : System.Boolean` (private), valid when `idleDecisionClient` is the current blueprint | undisguised 25 %; disguised 45 % | **0** in code: humans always get the normal bob. The only other freezes are a debug toggle (`IdleAnimator.SetGloballyEnabled`) and a scripted shot-survivor |
| **Twitching** | a small fast side-to-side shake instead of the slow vertical bob (amplitude 0.035, 4 Hz, horizontal axis) | `StoreClientMono.idleShakeHorizontal : System.Boolean` (private) | undisguised 3.75 %; disguised 35 % | **0** in code |
| **Odd pupils / uncanny expression / visible chip** | an aug portrait: one of 18 dedicated sprites (`augLL*`, `augBM*`, `augUL*`, male and female, 3 each) | `StoreClient.spriteName : System.String` → pool membership in `SpriteDict`, described in words (e.g. "a chip at the temple, glassy pupils") once the sprites have been looked at | undisguised: tier 1 40 %, tier 2 35 %, tier 3 30 %, tier 4 25 % (tier 1–2 forced on during the hold window); disguised 35 % | **0**: only `AugHelper` draws from these pools |
| **Outfit not matching their faction** | a portrait from another faction or trade (scav, farmer, chef, medic, engineer, miner, merc, scientist, or another core faction) under a name or bubble that says otherwise | `spriteName` family vs `displayName` / bubble colour | undisguised: 20 % of tier ≥ 2 (with an aug sprite of the *other* faction if both roll); disguised 45 % | **unmeasured**. The disguised mismatch only excludes the host's own core-faction pool, so a "mismatch" can look ordinary for an occupation host. Weak tell until measured |
| **Name vs bubble** (not in the guide) | an undisguised aug named "Upper Level Citizen" or "Thief" talks in a **plain** bubble; real ones talk in the upper-level or black-market bubble | `displayName` + the bubble the dialogue UI picks (`StoreClient.bubbleColor` = `"DEFAULT"` → faction colour, `DialogUIManager`) | 2/3 of undisguised augs | ≈ 0 (inferred: the genuine clients that use these names set the matching faction: `StoreClientFactionSetup.InitUpperLevelCitizen`, the black-market thief seller). The third, "Lower Level Citizen", is indistinguishable: lower levels also use the plain bubble |

How deterministic: idle and sprite are decided once per visit, so they hold for the whole visit.
A disguised aug has both fixed at creation by one roll (`MakeDisguisedAug`). For an undisguised
aug the idle is rolled when it arrives (`StoreClientMono.PlayIdleForCurrentClient()`) and the
sprite at creation. The roll uses `UnityEngine.Random`, which is not seeded per run, so the mod
must read the result rather than predict it.

Joint roll of `MakeDisguisedAug` (one 0–99 draw):

| Draw | Idle | Portrait |
|---|---|---|
| 0–34 | still | mismatched |
| 35–54 | normal | aug |
| 55–74 | twitch | host's own |
| 75–84 | still | aug |
| 85–94 | twitch | mismatched |
| 95–99 | twitch | aug |

**Every disguised aug shows stillness, twitching or an aug portrait** (100 %). The idle alone
catches 80 %.

### 2.2 Dialogue (undisguised only)

`AugHelper.CreateRandomMedAugDialogue(System.Int32, System.Int32)` /
`CreateRandomMaterialAugDialogue(...)` → `BuildTieredAugDialogue`. Lines come from an aug-only
pool (keys `aug_dialog_*`). Paraphrased by tier; `showBudget` is a 50 % roll:

| Tier | Opening | Tell |
|---|---|---|
| 1 | a slightly stiff request ("I require…", a formal greeting) and a bland reason; a plain budget line if shown | formality only |
| 2 | 75 %: a request with a slip, such as calling itself "this unit" then correcting, "requesting… whatever you have", or a reason about fabrication, replication, "the network", "a subject" or "a volunteer" preparing for augmentation. 25 % (needs `showBudget`): a clean request followed by a budget phrased as an allocation | always one |
| 3 | 75 %: two slips in one exchange. 25 %: one slip plus an allocation-style budget | always one or two |
| 4 | three blatant lines: insists it is a normal human or hobbyist, names augmentation implants or the network, states an "allocated" budget, asks to "process this efficiently" | blatant |

The guide's "robotic vocabulary" and "overly formal phrasing" refer to these lines. They fit
the mentor note: augs buy raw materials (to make chips) and medical supplies (to implant hosts).
False positives: the strings are aug-only, so exact matches never fire on humans. A
paraphrase-level judgment ("sounds formal") will fire on some humans (unmeasured). A disguised
aug speaks its host's lines: **no verbal tell**.

### 2.3 Behaviour under pressure

| Tell | What happens | Member | Augs | Humans |
|---|---|---|---|---|
| **Gun reaction** | raising the shotgun makes the customer react. Tier-4 augs (disguised or not) give robotic "I am feeling… fear" lines; everyone else, including tier 1–3 augs, gives the generic panicked pool | `AugHelper.GetClientReactionUnholster() : Dialogue` (static; a client's own `unholsterDialogue` overrides it) | tier 4 only | 0 |
| **Markup / demand-discount refusal** | an aug always refuses a markup or a demanded discount, and never walks out. The tablet's odds do not know this and show the normal accept % | `BargainUIManager.OfferMarkup(System.Int32)`, `DemandBuyingDiscount(System.Int32)` → `RefuseAugHaggle(System.Int32)` (CONFIRMED, `docs/sim/VERIFIED.md`) | 100 % | refuse-and-stay = (100 − 1.2·pct + b) − (100 − 2·pct + b′), about **8 %** at 10 %, 20 % at 25 % (before rep bonuses). An acceptance or a walk-out proves human |
| **Threaten** | aug: always accepts with a robotic line ("Acknowledged… terms accepted"), +20 % markup or −20 % discount. Human: an angry denial, −7 rep, or 50 % −10 rep and they leave | `BargainUIManager.ThreatenAug()`, `HandleFalseThreaten()` | 100 % (exact) | exact |
| **No rep movement** | exposing a fake feature on an aug's item costs no rep (normally −1, −4 or −5) | `!isAug` guards (§1.4) | 100 % | 0, but only observable when the situation arises |

**The 100 % tooltip.** `GetBargainSuccessChance(BargainType.Threaten, int)` returns 100 for
every customer. It does not check `isAug`. The button's info text says a wrong accusation
always fails. The mod's label must not repeat the 100 % as if it were the odds for this customer.

**Threaten forfeits the gun.** `Shotgun.CanPickup()` returns false once `ThreatenOffered`. A
threatened aug can no longer be shot: it will leave alive, and presence goes up. Threaten is an
exact oracle that costs you the kill.

**Raising the gun forfeits the customer.** `StoreUIManager.EnterShootingMode()` interrupts
and drops the dialogue. On `ExitShootingMode()` without a shot, the conversation cannot
resume (outside three scripted characters with `holsterResumesDialogue`), so the customer is
dismissed. Raising the gun to test someone means a lost sale, and for an aug it means "left
alive" (inferred from `DialogUIManager.ResumeConversation()` returning false when nothing was
paused).

### 2.4 Instruments

| Instrument | How obtained | What it shows | Reliability |
|---|---|---|---|
| **Aug Scanner** (`aug_scanner`, base value 200) | the Inventor's visit lists it with 50 % chance while augs are active and you don't own one (`StoreClientList.PlaceInventorInventory`) | its tooltip: "Ready to scan" / **AUG DETECTED** (red) / "No Aug detected" (green), plus an alarm or a sonar ping. Uses 25 energy from the inserted battery; one scan per customer (`StoreClient.isAugScanned`). Does not affect the shotgun | exact. Description: "100 % accuracy". `AugScanner.CreateTooltip(RichTextBuilder, GameItem)`; activation in `MiscItemDirectory` |
| **Augmented Identification Guide** (`aug_id_guide`) | from the mentor on day 12; in the starting inventory with Aug Menace | visual: unusual pupils, uncanny expression, visible chip. Non-visual: unnatural stillness, twitching, outfit not matching faction, robotic vocabulary, overly formal phrasing (local text: `var/knowledge/manuals/aug_id_guide.md`, never commit) | a checklist; maps 1:1 to §2.1–2.2 |
| **Mentor's note** | with the shotgun | augs want materials and medical supplies; formal or robotic speech; shoot only when certain; don't call Security, augs flee first | |
| Magnifying screen | counter (`StoreUIManager.EnterMagnifyingMode()`) | zooms the portrait so pupils and chip can be seen | perception aid |

### 2.5 How many augs are identifiable

Assuming perfect perception (the PROTOCOL.md standard):

- **Disguised**: 100 % show stillness, twitching or an aug portrait.
- **Undisguised, tier 2–4**: 100 % show a verbal tell.
- **Undisguised, tier 1, after the hold window**: clean on every visual cue when the name is
  "Lower Level Citizen" (1/3), the idle is normal (71.25 %) and the sprite matches (60 %). That
  is **14 %** of tier-1 undisguised augs (inferred). Their only tells are the stiff aug-only
  lines, the shopping pattern (medical, or materials plus energy cells, with a budget of
  150–250) and the haggle refusal. The share grows as tier 1 dominates late in the run (75 %
  from day 119).

## 3. Options and consequences

### 3.1 Shooting

Availability: `Shotgun.CanPickup() : System.Boolean`. It needs a customer at the counter who
is arrestable and dismissable (or `forceShootable`), not Security, not `nonShootable`, not
being arrested, no shootout running, and not already threatened. It also needs
`PlayerStore.isShootingUnlocked`. The gun is a hold-click (0.4 s), then a separate shot.

Flow: `StoreUIManager.Shoot()` → wound and fall animations → `StartShootoutSequence()` →
Security arrives → `AugHelper.GetShootResult() : System.Int32` (0 = aug, 1 = innocent,
2 = wanted with `shotCountsAsCapture`) → `StoreClientDialogList.SecShootoutDialog(System.Int32)`.

| Result | What Security says | Effects |
|---|---|---|
| **Good kill** (aug) | confirms it was an aug, "good kill" | `AugHelper.OnGoodKill()`: cash `clamp(100 + days since intro, 100, 250)` (report-reward perks apply; Aug Menace ×1.25), paid directly and documented by a Security slip on the counter; rep +2 Rev, +2 BM, +2 LL, +8 Sec; presence − drop (floor 0); `RunStats.OnAugShot()` |
| **Wanted capture** | recognises the wanted face, warns you, logs a capture | bounty (`PlayerStore.OnWantedClientShot(StoreClient)`); no crime |
| **Wrongful kill** (any human, including wanted ones that don't count as captures) | Security says you killed a real person, opens a manslaughter investigation, lets you keep the gun | `AugHelper.CleanupKill()`: crime **MANSLAUGHTER** of 250 (day < 25), 500 (day ≥ 25), 750 (day ≥ 50), ×0.67 Intimidating / ×1.33 Blabbermouth (`SecData.CommitCrime`); rep −10 Rev, −10 BM, −15 LL, −15 Sec (through `StoreReputation.ModReputation(string,int)`, so faction multipliers apply: Black Market losses are doubled per `VERIFIED.md`); the victim's wanted poster closes; the client's `onShotDead` hook runs (story consequences for named characters); the sale is lost. **No game over**, no fine, no cash change |

What a wrongful kill costs: the crime record feeds the investigation. A total of 100–299 adds
+2 evidence a day, 300–849 adds +6, 850–1199 adds +9, and so on. The record decays by 20 a day
while evidence is below 80 (`SecData.OnNewDay()`). One late-game manslaughter (750) on top of
ordinary fencing puts the shop in the "+9/day, investigation fast" band for weeks. The −15
Security rep can push Security negative. A gun-permit revocation then confiscates the shotgun
(`Shotgun.WouldPermitRevocationTakeGun()`), and with no shotgun every later aug leaves alive.

### 3.2 "Left alive"

`PlayerStore.DismissCurrentClient(System.Boolean, System.Boolean, System.Boolean)` calls
`AugHelper.OnAugLeftAlive()` whenever the departing customer is an aug and
`StoreUIManager.isShot` is false. Every counter exit goes through it, so **every exit
except being shot counts**:

- served normally (sale done, customer leaves)
- dismissed (`TryPlayerDismissCurrentClient`, ringing the bell for the next customer)
- walked out after a haggle (augs never do) or after a dialogue end action
- **reported**: `PlayerStore.OnCustomerReported()` on an aug dismisses it first, then Security
  arrives and says the aug is gone again and that they hacked Security's comms. There is no
  report reward and no wrongful-report penalty (that path returns early). The aug still counts
  as left alive
- **threatened**: the aug can't be shot afterwards, so its exit counts
- **gun raised, then lowered**: the customer is dismissed (§2.3)

A shot aug is dismissed while `isShot` is still true (`OnShootoutLeave` →
`OnArrestDialoguePlayed` → dismiss → `StartLeaveSec` clears `isShot`), so it does not count.

### 3.3 Summary

| Option | Aug | Human |
|---|---|---|
| shoot | +100…250 cash, small rep gains, presence − drop | manslaughter crime 250/500/750, rep −15 LL/−15 Sec/−10 Rev/−10 BM, sale lost |
| scan (if owned and charged) | exact answer, the gun stays available | exact answer |
| markup / demand-discount probe | always refuses, stays | accepts (proves human), refuses (≈ 8–20 %), or walks (−3 rep) |
| threaten | +20 % deal; **cannot shoot**; left alive (+rise) | −7, or 50 %: −10 and they leave |
| serve / dismiss | left alive (+rise) | normal |
| report | left alive (+rise) | wrongful report (existing report rules) |

## 4. The hidden meter, from the player's side

### 4.1 Presence is fully reconstructible

Every presence change leaves a visible trace:

- **Left alive** → the next morning's night log carries one line per escaped aug: "one of your
  contacts says an Aug visited your store yesterday" (`NightLog.AugLeftAlive()`, added
  unconditionally, never de-duplicated). It appears on every difficulty, including for augs
  the player never noticed.
- **Good kill** → Security's verdict at the counter, plus the Security slip.
- **Start** → 15 on the day the mentor hands over the shotgun (or day 1 with Aug Menace).
- Difficulty and perk are known, so the rise and drop values are known.

```
P_0 = 15 on intro day
each escape line  : P += rise
each good kill    : P = max(0, P − drop)      (apply in time order)
```

This should equal `hidden.aug_presence` exactly. Any drift points to a bug in the tracker or
an unknown path, and evaluation should flag it.

### 4.2 Other visible signals

- **Aug due tomorrow** (cozy/normal only, `augWarning`): `PlayerStore.HandleAugWarning()`
  writes "Security says an Aug is likely to visit today" when tomorrow ≥ `nextAugSpawnDay` and
  tomorrow is not a scripted intro day. It is not a presence indicator. It says nothing about
  disguised hosts and only warns on scheduled-spawn days.
- **Cadence**: the next aug comes 6–8 days after the last scheduled one (§1.2). Once one
  aug has been seen or reported by the night log, the next window is three days wide, even on
  hard where there is no warning. Before day 24 the visits are fixed: days 13 and 18.
- **All-clear officer**: the end of the threat. Seeing it means presence was 0.
- Run summary: "Augs Shot". The newspaper never mentions augs.

## 5. Design

### 5.1 `aug_check` decision kind (advise-only)

**When.** The mod asks only when augs are introduced, not ended, and `Shotgun.CanPickup()` or
the threaten button is available. Triggers, all computed from visible state only:

1. the customer has finished arriving and shows at least one visual tell (§2.1), or wants
   MEDICAL / MATERIAL+ENERGY_CELL;
2. a new dialogue line, a haggle result or a scanner result appears for that customer (the
   signature changes);
3. **the human raises the gun** (`EnterShootingMode` prefix). This is the last chance to
   advise, and the context includes the gun reaction once it plays.

A trigger must never depend on `isAug`. Otherwise asking itself would leak the answer.

**Context** (words, never ids):

```
client: name, bubble ("plain" | "upper-level" | "black-market" | …),
        portrait (e.g. "lower-level clothes; glassy pupils, chip at the temple" /
                  "miner's work gear" — sprite family + aug-pool features, in words),
        idle ("bobs normally" | "perfectly still" | "small fast side-to-side twitch"),
        wants (tablet interests), budget, lines_so_far [verbatim displayed text],
        gun_reaction (if raised), haggle (e.g. "refused 10% markup without leaving
        (tablet showed 80% accept)"), scan ("not owned" | "no charge" | "ready" |
        "AUG DETECTED" | "No Aug detected"), threaten_used
history: day, days_since_intro, difficulty {rise, drop, game_over_at_100}, aug_menace,
         augs_left_alive (night-log count), good_kills, wrongful_kills,
         inferred_presence (§4.1), escapes_until_game_over, aug_due ("warned today" |
         "in window d+6..d+8" | "not due"), all_clear_eligible_from
```

`scan` reads `isAugScanned` and, only once it is true, the tooltip verdict (which is `isAug`).
That is legal because the screen shows it.

**Options** (only the ones the UI allows right now):

| id | label states |
|---|---|
| `shoot` | "Shoot: if an aug, +N cash, presence −drop; if human, manslaughter (crime C), rep −15 LL/−15 Sec/−10 Rev/−10 BM" |
| `scan` | "Scan (25 energy): exact" |
| `probe_markup_10` / `probe_discount_10` | "Add 10% markup: an aug always refuses without leaving; humans accept ~X%, refuse ~Y%, walk ~Z%" (only when that bargain side is unused) |
| `threaten` | "Threaten: exact (+20% if aug), but you can no longer shoot this customer, so an aug escapes (+rise presence); human: −7 rep, or −10 and they leave" (never "100%") |
| `serve` | "Serve normally: an aug escapes (+rise presence)" |
| `dismiss` | same consequence as serve |
| `defer` | always |

`report` is left out. Against an aug it never beats `serve`, and against a human it is the
wanted-report decision, which belongs to barter.

**Advise-only.** The advice never fires, raises or lowers the gun, never scans, and never
threatens or probes on this kind. (Since M3 the autopilot, a separate driver, raises the gun and
fires on its own verdict: §6 and `docs/STREAM.md` section 5.) Raising the gun alone dismisses the customer (§2.3), so an
"act" mode here would be unsafe even for the information options. The bridge's
`policy.CheckChoice` refuses `shoot` when no visual, verbal, haggle or scan tell is present,
and applies §5.2.

**hidden** (logged, never prompted): `is_aug`, `aug_tier`, `aug_idle_tell`, `disguised`,
`shoot_result_if_shot` (the value `GetShootResult` would return), `aug_presence`,
`next_aug_spawn_day`.

**Signature**: `c<seq>:l<lines shown>:h<haggle flags>:s<scan state>:t<threaten>:g<gun raised>`.

**Leak to fix in barter v1.** `client.faction` maps `clientFaction` through
`Fence.Barter.Factions.Name`. That gives `"none"` for every undisguised aug, while the player
only sees a plain bubble, the same one lower-level citizens get. Measure how often non-aug
customers carry `NONE` in recorded `customer_observed`. If it is rare, send the bubble colour
instead of the faction (or map NONE to the plain-bubble token) in both barter and `aug_check`.

### 5.2 Strategist: inferred presence and the RISK rule

The tracker lives in the strategist's day state and is rebuilt from the event log (night-log
escape lines, Security kill verdicts, intro day, difficulty). Every morning it publishes
`inferred_presence`, `escapes_left = ceil((100 − P) / rise) − 1` (how many more escapes the
run survives) and the next aug window. It is checked against `hidden.aug_presence` offline only.

**RISK rule.** Let `p` = the policy's belief that this customer is an aug. Shoot when `p` meets
the bar for the current band:

| Band (game-over rules on: hard, challenge, Aug Menace) | escapes_left | shoot when |
|---|---|---|
| SAFE | ≥ 3 | `p ≥ 0.95`: a zero-false-positive tell (stillness, twitch, aug portrait, Upper/Thief name in a plain bubble, aug-only lines, scanner) |
| CAUTION | 2 | `p ≥ 0.80`: a strong tell, or two weak ones plus a haggle refusal |
| DANGER | 1 | `p ≥ 0.50` |
| CRITICAL | 0 (the next escape ends the run) | `p ≥ 0.20`. Scan or probe first if the option exists. A wrongful kill is recoverable; the game over is not |

Without game-over rules (cozy/normal, no Aug Menace), escapes cost only the kill reward and
delay the all-clear, so the bar is always `p ≥ 0.95`.

Standing rules for the strategist's plan:

- Buy the scanner at the Inventor as soon as it is offered on hard or challenge, and keep a
  battery charged. It turns every uncertain case into an exact one without forfeiting the shot.
- Never threaten a suspected aug when `escapes_left ≤ 1`. The 20 % is not worth an escape.
- On hard, two clean kills reach presence 0 and (from day 62, permit held 25 days) end the
  threat. Plan to keep MATERIAL/MEDICAL on display during aug windows if the policy reads idle
  and portrait tells better than dialogue: disguised augs are 100 % visually marked.
  Undisguised tier-1 augs are not.
- Expect about one aug a week; prepare on window days, and relax on the other days because
  no aug can spawn.

### 5.3 Recording

| Event | Hook | Fields |
|---|---|---|
| `human_action: unholster` / `holster` | `StoreUIManager.EnterShootingMode()` / `ExitShootingMode()` prefix (holster = exit while `isShot` is false) | `client_seq`, `decision_id`/`advised` in force |
| `human_action: shoot` | `StoreUIManager.Shoot()` prefix | `client_seq`, tells present at the time |
| `human_action: threaten` | existing `barter.bargain_threaten` | |
| `human_action: report` | existing `human.report` | |
| `human_action: scan` | `PowerHelper.DrawPowerSource(GameItem, System.Int32)` postfix when the current client's `isAugScanned` flipped (the activation itself is a compiler-generated lambda) | verdict as shown |
| outcome `aug: good_kill` | `AugHelper.OnGoodKill()` postfix | cash delta, rep delta, `hidden.presence_before/after` |
| outcome `aug: wrongful_kill` | `AugHelper.CleanupKill()` prefix (read `GetShotClient()` before it clears) | crime added, rep delta, `hidden.is_aug=false` |
| outcome `aug: wanted_capture` | existing `PlayerStore.OnWantedClientShot(StoreClient)` | |
| outcome `aug: left_alive` | `AugHelper.OnAugLeftAlive()` postfix, with the exit cause taken from the scope of the enclosing `DismissCurrentClient` (served / dismissed / reported / threatened / holstered) | `hidden.presence_before/after` |
| every customer | existing `customer_observed` with `hidden.is_aug`, plus `aug_tier`, `aug_idle_tell`, `disguised` | |
| snapshot | existing `hidden.aug_presence`; add `hidden.next_aug_spawn_day`, `hidden.aug_ended`, `hidden.all_clear_due` | |

`docs/re/DAY.md`'s `shoot` row covers only wanted captures and good kills (`RunStats.OnAugShot`
fires only in `OnGoodKill`). Wrongful kills reach neither hook, so `CleanupKill` is required.

Evaluation: precision and recall of the advised `shoot` against `hidden.is_aug` (human
baseline from the same logs), wrongful kills per 100 days, escapes per difficulty band, and
`inferred_presence − hidden.aug_presence`, which should always be 0.

Proposed hooks: `compat/hooks.proposed.json`, ids `aug.*`, features `advise.aug` /
`record.aug`.

## 6. The fair detector (M3)

The stream's rule is "visible + hidden, flagged": the executor may act only on what a player sees.
Every tell a player can use, from the code (§1–§4) and what the detector does with it:

| Kind | Tell | Detector | Notes |
|---|---|---|---|
| Behaviour (idle) | perfectly still at the counter | **sure** | live: `StoreClientMono`'s idle flags; gym: the same roll on its own stream |
| Behaviour (idle) | small fast side-to-side twitch | **sure** | as above |
| Appearance | an aug portrait (odd pupils, uncanny expression, a chip) | **sure** | live: the sprite's pool; gym: hooks on `AugHelper` see the aug pool chosen |
| Appearance | "Upper Level Citizen" or "Thief" in a plain bubble | **sure** | the name claims a faction whose real members use another bubble |
| Appearance | an outfit that does not match the bubble | weak, not used | unmeasured on humans (the gym has no sprites) |
| Dialogue | the aug script (stiff requests, "this unit", "allocated" budgets, blatant tier 4) | **sure** | by displayed text against the game's own aug lines (`AugLines`, from the string table at run time); disguised augs speak their host's lines |
| Dialogue | robotic "fear" lines when the gun is raised (tier 4) | not used | raising the gun forfeits the customer (§2.3) |
| Dialogue | the threaten reply, reluctant-accept lines after a haggle | not used | threaten forfeits the shot; the executor shoots before the counter |
| Behaviour | refuses a markup or demanded discount and stays | not used | ~8–20 % of humans do too (§2.3) |
| Behaviour | no rep change on an exposed fake | not used | rarely arises |
| Shopping | medical, or materials + energy cells, budget 150–250 | weak, not used | 3 865 human visits matched it in the 60-seed run below |
| Tool | the aug scanner (Inventor, 50 % while augs are active; 25 energy a scan) | **sure** when shown | the executor does not buy it (no need, below) |
| Tool | the Augmented Identification Guide, the magnifier | perception aids | the guide's list is this table's sure tells |
| Upgrades / services | none touches augs | – | no Wild's upgrade or service in the code reads `isAug` |
| News / presence | the night log's escape line, Security's kill verdict, "an aug is likely today" (cozy/normal), the all-clear officer, the 6–8 day cadence | timing only | presence is reconstructible (§4.1); the newspaper never mentions augs |

The executor shoots on any sure tell or the scanner's AUG DETECTED, and serves otherwise
(`sim/gym/AugRule.cs`, shared with the live autopilot). The gym logs the hidden truth only to score
it (`aug_eval` in each run's summary; `-aug-detection perfect` is the pre-M3 cheat for analysis,
`-aug-lines off` drops the line recognition).

Hard, the best policy (`hard_income_mortgage_appraisal.json`, apprentice, vending + short sleeper),
60 fresh seeds 2500001–060, `-days 1000` (`docs/STREAM.md` section 7):

| Arm (hard, best policy, 60 fresh seeds 2500001–060) | Wins | Median day | Endings | Augs shot / escaped | Detector precision / recall |
|---|---|---|---|---|---|
| perfect detection (`-aug-detection perfect`, the pre-M3 cheat) | 60/60 | 175 | buyout 60 | 481 / 0 | – (reads the truth) |
| **fair detector** (`tells`, the default) | **60/60** | **175** | buyout 60 | 481 / 0 | **1.000 / 1.000** (481 augs, 23 707 human visits) |
| fair detector without line recall (`-aug-lines off`) | 60/60 | 175 (+0.3 ± 0.6 paired) | buyout 60 | 449 / 44 (≤ 4 a run) | 1.000 / 0.911 (misses only undisguised augs) |
| serve all (`augs: serve`) | 0/60 | 47 | aug 60 | 0 / 360 | – |

Precision is 1.000 for every sure tell (none fired on a human) and recall is 1.000 with line
recall: §2.5's 14 % of tier-1 undisguised augs with no visual tell all speak the script. Without
line recall the misses are undisguised augs only (44 of 493), and the run still wins: the shooting
keeps presence low. The scanner, refusing medical/material buyers or a security service are not
needed.

## Open questions (need a look in game)

1. What the 18 aug portraits look like, and the words for each (pupils, chip position,
   expression). Same for the trade/faction sprite families used by the mismatch tell.
2. The base rate of "outfit not matching faction" among humans, and of `clientFaction == NONE`
   among non-aug customers (from recordings).
3. Whether the stillness is noticeable at a glance at normal zoom (the bob is
   `IdleAnimator.amplitude`, field default 0.25 before any prefab override), and whether the twitch is.
4. Whether disguised event hosts can be non-arrestable, which would make them unshootable.
