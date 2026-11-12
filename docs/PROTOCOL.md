# Mod ↔ bridge protocol (v1)

HTTP/JSON. The bridge listens on `127.0.0.1:7341` (configurable on both sides). The mod is
always the client — the game's `ModScanner` refuses mods that run background threads, so
the mod cannot host a server. Schemas live in `protocol/schema/`, examples in
`protocol/fixtures/`; both the Go and C# test suites round-trip every fixture.

`protocol` is an integer bumped on any breaking change. The bridge answers `409` to a
`hello` whose `protocol` it does not speak; the mod then stays in vanilla mode.

## `GET /v1/health`
`200 {"ok": true, "protocol": 1}`

## `POST /v1/hello`
Sent once per game launch, on the first `ModHook.OnGameLoadedLate`. The mod cannot hash
`Assembly-CSharp.dll` (that needs the `files` capability), so `assembly_sha256` is filled
from the embedded `compat/versions/` snapshot matching `assembly_mvid`, and is empty on a
build `make compat` has not recorded yet.

```json
{
  "protocol": 1,
  "mod_version": "0.1.0",
  "game": {"version": "0.46D", "unity": "2021.3.45f2", "assembly_sha256": "…", "assembly_mvid": "911508a6-…"},
  "compat": {"ok": false, "missing": ["StoreClientManager::HandleSkipClient()"], "disabled_features": ["act.barter"]}
}
```
→ `200 {"session_id": "…", "protocol": 1, "mode": "observe" | "advise" | "act", "appraisal_guides": {…}}`

`mode` is the bridge's choice. `observe`: events only. `advise`: decisions are requested
and the answer is shown to the player, never executed. `act`: the answer is executed.

`appraisal_guides` (optional): the appraisal guides' value tables when the bridge has them locally
(`local/appraisal_guides.json` or `FENCE_APPRAISAL_GUIDES`; `docs/re/APPRAISAL.md`, "The value
table"). The mod has no file access; the autopilot joins the appraisal board only with them.

## `POST /v1/event`
Fire-and-forget. The mod never waits on the response and drops events if the bridge is
down.

```json
{"session_id": "…", "seq": 17, "kind": "shutter_opened", "game_day": 69, "data": {}}
```
→ `204`. `seq` is per-session and monotonic so the bridge can detect drops.

Kinds come from `compat/hooks.json` (`event.<kind>` features) plus `client_arrived`
(`data`: visible `name`, `faction`, `intent`), sent once per customer. `going_sleep` carries
the day that just ended; the day counter advances during sleep.

### Recording events (phase D)
Events may carry a top-level `hidden` object with the same rule as `decide.hidden`: logged
for evaluation and training, never reaching a prompt, memory recall or reflection.

| kind | when | `data` |
|---|---|---|
| `day_snapshot` | `phase: morning` after `waking_up`, `phase: close` after `shutter_closed` | `store` (with `difficulty` and the mod's `aug_detection`), `reputation` (with `next_perk`), `perks` (`{starting: [ids], reputation: [ids]}`: the run's starting perks and every reputation perk unlocked now), `inventory` (barter item shape + `where` ∈ stock/showcase/counter; a container — smuggler bay, bag, machine — adds `capacity: {cells, free}`, its grid cells and those no item covers), `security`, `upgrades`, `services`, `market_events`. Example: `protocol/fixtures/event.day_snapshot.json` |
| `customer_observed` | once per customer when the counter first becomes ready, **covered or not** | `client_seq`, `covered`, `reason`, `context` (barter context v1, as much as applies), `options`. Example: `event.customer_observed.json` |
| `human_action` | every move the player makes at the counter or in the night phase | `client_seq`, `action` (a barter option id, or `stage`/`unstage`/`expose_feature`/`showcase`/…, see `docs/re/DAY.md`; the gun: `unholster`, `holster`, `shoot` with the customer's `name` and its `aug` / `aug_tells` at the shot), `item_uid`, the `decision_id` and `advised` option in force at the time (the aug_check decision for gun moves), if any; `actor: "autopilot"` (with its `rule`) when the autopilot made the move, absent for the player's own. Example: `event.human_action.json` |
| `autopilot` | every action of the live autopilot (docs/STREAM.md), and the player pausing (`pause`) or resuming (`resume`) it | `day`, `phase` (morning/open/customer/night), `action` (`plan_day`, `close_morning_report`, `plan_window`, `showcase`, `unshowcase`, `open_shutter`, `ring_bell`, `dialogue_choice`, `plan_stage`, `stage`, `plan_stage_seller`, `stage_theirs`, `counter` with the option in `detail`, `dismiss`, `plan_unstage`, `unstage`, `close_shutter`, `open_tablet`, `services`, `sleep`), `target` (`uid`, `name`) for item moves, `why` (the executor's reason), `rule` (its rule key), `detail`, `plan_day`, `client_seq`/`client` at the counter, `hidden_used` (names of hidden values the rule read, never the values; `[]` when none). Example: `event.autopilot.json` |
| `autopilot_fault` | the autopilot stops and needs the human | `state` (`faulted`: an error, an action that did not take, a stall; `handover`: a customer or screen it has no handler for), `reason`, `message` (the overlay's banner), `action` (the last one taken), `client`, `client_kind`. Resume is manual. Example: `event.autopilot_fault.json` |
| `aug` | a shot resolves, or an aug leaves alive | `event` ∈ `good_kill` (cash and rep deltas), `wrongful_kill` (`crime: {id: MANSLAUGHTER, amount}`, rep delta), `wanted_capture`, `left_alive` (`cause` ∈ served/dismissed/reported/threatened/holstered); `client_seq`, `name`. `hidden`: `is_aug`, `presence_before`/`after`. Sent whether or not an aug_check was asked; the strategist counts kills from it |

Recording does not depend on the mode or the policy: in `observe` mode, and with the model
down, these still flow. The mod keeps a bounded in-memory buffer while the bridge is
unreachable and flushes it in order when it returns.

## `POST /v1/decide`
Example: `protocol/fixtures/decide.request.json`. The game computes every price itself, so
barter options are the discrete moves the player has (sell/buy all staged items, one
bargain step per side at 10/25/50/75%, blackmail, dismiss — see `docs/re/BARTER.md`), with
the odds the game's own tooltips show written into the label. `params` (an int with
bounds) exists for future kinds; barter does not use it.

**Information boundary (decided 2026-11-09).** `context` carries only what a player can
see on screen. `hidden` (optional) carries internal values — true item features before
exposure, appraisal value, stealth-wanted/aug status, days to the next inspection. The
bridge logs `hidden` with the decision for evaluation and training labels, and **never**
puts it in a prompt (`TestHiddenNeverInPrompt`); the mod keeps it out of `context`. The VISIBLE/HIDDEN split per field
is in `docs/re/STATE.md`.

**Perception standard (decided 2026-11-09):** the model plays like an expert player —
perfect perception of everything on screen and full knowledge of the game's rules.
Visual tells are described as what they depict ("seal: missing", "logo: off-centre"), never
as raw sprite/variant indices; rule knowledge (serial prefixes, recalls, stamp amounts,
appraisal formulas) is not gated on owning in-game guides. **Aug detection reads the visible
tells** (`tells`, the default since 2026-11-10, M3; the stream's "visible + hidden, flagged" rule):
`client.aug` is absent and `client.aug_tells` lists the zero-false-positive tells on screen, in
words (perfectly still, twitching, aug portrait, aug-only lines, a generic "Upper Level
Citizen"/"Thief" in a plain bubble). The aug-only lines are recognised by their displayed text
against the game's own aug script, read from its string table at run time (`AugLines`; only the
keys are in the repo), and only where the customer's lines are at hand (aug_check). In the gym this
detector flags every aug and no human (docs/re/AUGS.md §6). The mod setting `aug_detection: perfect`
(and the gym's `-aug-detection perfect`) still states each customer's status directly
(`client.aug`, a bool) for analysis; it was the assumption from 2026-11-09 to M3 and never runs on
stream. The truth stays in
`hidden.client.is_aug` either way (plus `aug_tier`, `aug_idle_tell`, `disguised` for an aug). Truth flags (`realCondition`,
`*_ISSUE*` tags, true values) stay hidden.

`state_signature` is an opaque string the mod derives from the state it sent (client,
staged items, bargain flags). The mod recomputes it before executing and defers on a
mismatch, because the game keeps running while the model thinks and the human may act
first (`docs/re/TIME.md`). The bridge only logs it.

→ `200` — example: `protocol/fixtures/decide.response.json`.

- `defer` is always a legal `choice.option` and means "the human decides". The bridge
  returns it on LLM failure, timeout or an invalid answer after one repair attempt.
- The bridge must return within `deadline_ms`; the mod also enforces it and defers on
  its own if the bridge does not.
- `source`: `llm`, `heuristic` (the scripted baseline policy), or `fallback`.
- Unknown `kind` → the bridge returns `defer`, never an error. New client types arriving
  in game updates must degrade to the human, not break.

### Barter context (v1)
Canonical example: `protocol/fixtures/decide.request.json`. Every field is player-visible
(`docs/re/STATE.md`); a value the mod cannot read is omitted, never guessed.

| Field | Meaning |
|---|---|
| `store` | `cash`, `day`, `retail_markup`, `rent_due`, `rent_days_left` |
| `reputation` | faction → `{value, status}`, factions with a rep bar only (`lower_levels`, `upper_levels`, `security`, `black_market`, `revolution`, `cartel`) |
| `client.intent` | what the **client** wants: `buy` (shop sells), `sell` (shop buys), `sell_and_buy` |
| `client.faction` | as above, or `tourist`/`church`/`middle` (no rep bar). Shown in-game only as bubble colour; we name it. A customer with no faction (every undisguised aug) talks in the plain bubble the Lower Levels use, so it is sent as the faction its bubble implies (`lower_levels`), never `none` |
| `client.aug` / `client.aug_tells` | see Perception standard: the status (perfect detection) or the visible tells (tells mode) |
| `client.budget` | remaining budget when the client is buying; `null` otherwise |
| `client.interests` / `restrictions` | the tablet's Interests / Restrictions tags |
| `client.wholesale` | `{price, discount_pct}` when a seller offers wholesale, else `null` |
| `counter.theirs` | the client's items on the counters (shop may buy) |
| `counter.yours_staged` | the shop's items on the back counter (shop may sell) |
| item | `uid`, `id`, `name`, `price` (the deal price the tablet shows), `estimated_value`, `bought_at` (own items), `features` (`text`, `group` ∈ condition/market/bargain — claimed until exposed), `stolen_heat` (`null` if not stolen), `contraband_level` (0 = none), `serious_contraband` |
| `bargain` | which one-shot moves are used: `sell_side_used`, `buy_side_used`, `blackmail_used`, `threaten_used` |
| `market_events` | active market modifiers: `item_type`, `value_pct` |

`hidden` (never prompted): `client.identifier`, `client.is_aug`, `client.stealth_wanted`,
`items[].true_value`, `items[].real_features`, `days_to_inspection`.

### Stage decisions (`kind: "stage"`)
For a buying customer with nothing they want staged, the real choice is which of the shop's
items to offer. Example: `protocol/fixtures/decide.stage.request.json` / `.response.json`.
Context = barter v1 without `bargain`, plus `budget_left`, `candidates` (barter item shape +
`where`, `hidden_from_inspection`, `matches`, `attracted`, `over_budget`; ≤ 12, ranked by fit)
and `candidates_omitted`. Options: `offer_<uid>`, `nothing_fits`, `dismiss`. One item per
decision; once the human stages it the signature changes and the mod asks again (stage while
candidates and budget remain, then barter). Advise-only: the mod never stages items itself.
The mod excludes, and the server refuses (`policy.CheckChoice`), stolen or contraband goods
for security and contraband for customers who refuse it. Signature: `s<barter sig>:g<budget>`.

Deadlines: 20 s in advise mode (the human plays in parallel), 10 s in act mode.

### Inspect decisions (`kind: "inspect"`)
Asked when the player inspects an item and the game offers a verdict (confront panel for
cigarettes, food stamps, injectors; appraisal-service estimates for hands, Ribwiches,
plasma cells). Example: `protocol/fixtures/decide.inspect.request.json` / `.response.json`;
rules and cues in `docs/re/INSPECT.md`, vocabulary in `internal/policy/testdata/inspect_cues.json`.
Context: item kind, the customer's claim, printed text verbatim, and every visual cue in
words (never sprite indices); `hidden` holds the real condition, issue tags and true value.
Options mirror the game: `confront_<issue>` per panel toggle plus `genuine` for a claimed-
genuine item; `genuine` / `counterfeit` for a claimed fake; `estimate` with an int `value`
for appraisals. Labels state the consequences (wrong confront −2 rep with that faction).
The bridge adds a deterministic rule check; `CheckChoice` refuses confronting an item whose
cues are all fine. Advise-only. Verdicts are recorded as `inspect_verdict` / `confront` /
`appraisal_estimate` human actions linked to the decision; outcomes `confront` and `appraisal`.

### Aug checks (`kind: "aug_check"`)
Asked when augs are active (introduced, no all-clear yet), a customer is at the counter, the gun
can be raised (`Shotgun.CanPickup`'s conditions, or it is raised already) and the customer is an
aug (`perfect`) or suspected (`tells`: a zero-false-positive tell, a scanner verdict of aug, a
refused probe, or a buyer of medical supplies or materials). The trigger never reads the truth in
`tells` mode. Example: `protocol/fixtures/decide.aug_check.request.json` / `.response.json`; rules
in `docs/re/AUGS.md`.

Context: `detection`, `client` (`name`, `faction`, `intent`, `wants`, `budget`; `aug` in perfect
mode; in tells mode `aug_tells`, `portrait` and `idle` in words and the last dialogue `lines`;
`haggle` when a markup or demanded discount was refused and they stayed; `scan` once the scanner
showed its verdict), `gun_raised`, `threaten_used`, `run` (`day`, `days_since_intro`, `difficulty`,
`presence_rise`, `kill_presence_drop`, `game_over_at_100`, `aug_menace`, `kill_reward`,
`manslaughter_crime`). `hidden`: `client` (with the aug labels), `shoot_result_if_shot`
(`good_kill`/`wrongful_kill`/`wanted_capture`), `aug_presence`.

Options, as the UI allows, each label stating the consequences: `shoot` (aug: about +100–250
cash, rep +2 LL/BM/Rev and +8 Sec, presence − drop; human: MANSLAUGHTER 250/500/750 by day, rep
−15 LL/−15 Sec/−10 Rev/−10 BM, no game over; the sale is lost), `scan` (scanner owned, customer
not scanned), `probe_markup_10` / `probe_discount_10` (the open bargain side, with the tablet's
odds for a human), `threaten` (exact, but forfeits the shot: an aug escapes), `serve`, `dismiss`
(an aug leaves alive: presence + rise). No `report`: against an aug it never beats `serve`.

Advise-only in every mode: the mod never raises, fires or lowers the gun, scans, probes or
threatens on this kind (`ActSafetyTests` checks the shotgun, kill, report and scan members are
never called). `policy.CheckChoice` refuses `shoot` for a customer stated or scanned human, and
for one with no tell, no scanner verdict and no refused probe. Signature:
`a<client seq>:<A|H in perfect mode, else the tells>:s<scan>:h<refusals>:g<gun raised>:o<option ids>`.
Results: the `aug` event above, and the same object as an outcome (`kind: "aug"`, `result` = the
event) to the customer's latest aug_check decision when there is one.

### Barter option ids
The mod offers only options the game's UI would enable right now (`docs/re/BARTER.md`,
"Buttons → methods"). `N` ∈ {10, 25, 50, 75}.

| Client wants to… | Ids |
|---|---|
| buy (shop sells) | `sell_all`, `offer_discount_N`, `add_markup_N`, `blackmail`, `threaten`, `dismiss` |
| sell (shop buys) | `buy_all`, `accept_wholesale`, `offer_markup_N`, `demand_discount_N`, `blackmail`, `threaten`, `dismiss` |

`sell_all` over the client's budget includes confirming the capped sale; the label states
the capped amount. A `sell_and_buy` client is offered only the side the tablet shows.

Labels (built in `mod/src/Fence.Core/Barter/BarterRules.cs`, odds from the game's own
`GetBargainSuccessChance`/`GetBargainLeaveChance`): `Sell 2 items for 680`,
`Buy 1 item for 120`, `Accept wholesale: all 5 items for 900 (20%)`,
`Offer 10% discount (always accepted; raises their budget 20%)`,
`Add 25% markup (accept 52%, refuse 20%, walk out 28% (-3 rep))`,
`Offer 10% markup (always accepted)`, `Demand 25% discount (…same roll format…)`,
`Blackmail for 20% (accept N% if they are a criminal, else -5 rep; if innocent: -7 rep, 50% they walk out -10 rep)`,
`Threaten for 20% (… if they are an aug; …)`.

Item features render as the tablet shows them, modifier included (`Worn (-20%)`); type
tags (`interests`, `market_events[].item_type`) are the game's lower-cased ids.
`state_signature` is `c<client seq>:i<uid><o|t><b|f>@<price>,…:b<4 bargain flags>:m<b|s|n>`
(owned/theirs, back/front counter, tablet mode); the bridge treats it as opaque.

`option_notes` (optional): option id → a short display note (≤ 120 chars) such as the
bridge's expected value for that option. For the overlay only; never executed.

## `GET /v1/summary?session_id=…`
The day (meta-turn) view for the overlay. Example: `protocol/fixtures/summary.response.json`.
`today` comes from memory's day stats for the current run (zeros when memory is off),
`lessons` is the run's current lessons text (≤ 2 KB, `""` before the first reflection) and
`lessons_day` the day that produced it. `plan` / `plan_day` (optional, ≤ 2 KB) are the
strategist's plan for the day and the day it was written; absent before the first plan. `bet`
(optional) is that plan's move on the run's bet ("Bets" below). The mod fetches it on `client_arrived`,
`waking_up`, after each outcome, and at most every 5 s. `404` for an unknown session.

## Directives (structured plan)
The strategist's plan as machine-readable directives, so an executor (the gym's player today,
the mod's autopilot later) can follow it. Served in `/v1/summary` as `directives` alongside the
prose `plan`, and absent when there is no plan. Every field is optional; an executor uses its
own default for anything missing, and a present `0` is an instruction (no floor, no hot stock).
Schema: `protocol/schema/directives.schema.json`; example: `protocol/fixtures/directives.json`.

The bridge writes them with the plan: after the prose plan, the same conversation asks the
model for the directives as one JSON object (the prompt is then served from cache, so the
prose plan is unchanged by it). What the model gives is validated against the state (unknown
uids, services and upgrades dropped, names mapped to ids, percentages clamped to 0–100 for
buying); keys it leaves out or gets wrong are derived in code from the prose plan and the state.
Then the lints hold them to the plan's rules: the floor covers rent plus the services' nights
from two days before rent day and never exceeds cash; `max_spend` and `upgrades_buy` stay within
cash above the floor (Wild favor first; landlord grace may use the rent reserve); short of rent,
`sell_down_to_cash` reaches it and only kept or cooling items are held; from trafficking 40
`hot_stock_cap` is at most the hot items in smuggler bays plus their free cells; a type both
preferred and avoided is neither; no stolen,
contraband or kept item is shown; the permit is always named while not held: `buy` once cash
covers the fee, the rent due and 500, else `bribe` (a `refuse` becomes one of them);
augs are shot where presence ends the run; the open dumping ground is scavenged unless wounded.
From Claude agents' gym games (`docs/research/CLAUDE-PLAY.md`): `upgrades_buy` starts with the
upgrades of the Wild favor order not yet bought (the executor buys each when favor or spare cash
covers it); power is off while nothing draws store power, and insurance and janitorial are on;
`sell.min_pct` is dropped while selling down; finished wine is held while cash covers rent, a
week of services and 1 000 (Homebrew and better), otherwise only Reserve and Master Vintage;
`engine` is `none` once no new finished wine has appeared for 5 days; `bargain` is `never`
while Lower Levels is below -10. Upgrades the snapshot lists
with `state` 1 are Wild's, not the shop's: only `state` 2 is owned.

With the bridge's `-directives delta` the directives are instead only the changes to the
executor's own defaults that the model gives a one-line reason for (`{}` plays the executor
alone); the lints revert the deviations that have cost runs: a cash floor or spending cap on
supplier day (day % 7 = 5), an upgrade that neither favor pays nor leaves rent + 1 200, a bribe
when the permit is affordable, a refusal, aggressive bargaining, `engine: none` before the wine
engine stalls, insurance or janitorial off, power on with nothing drawing it. When the gym
plays a base policy (its `-directives FILE`; a league's `executor.directives`), the bridge's
`-base-directives FILE` shows the model that base; the fields it sends override the base's,
except an engine that names only part of the base's (a restated `wine` would stop its furnace).

| Field | Meaning |
|---|---|
| `day` | the day the directives were written for |
| `cash_floor` | never spend below this |
| `buy` | `wholesale_max_pct` / `single_max_pct` of estimate to accept, `max_spend` today, `prefer_types` / `avoid_types`, `hot_stock_cap` (stolen items held) |
| `sell` | `min_pct` of estimate, `sell_down_to_cash` (sell stock until cash reaches this, e.g. before rent), `hold_uids` |
| `bargain` | `never` / `ev_positive` / `aggressive` |
| `window` | `show_uids`, `avoid_stolen` |
| `services` | service ids to have on tonight |
| `upgrades_buy` | upgrade ids to buy tonight, in order |
| `permit` | `buy` / `refuse` / `bribe` |
| `production` | `[{machine_uid, action, items, note}]` |
| `augs` | `shoot` / `serve` |
| `scavenge` | go to the dumping ground tonight |
| `engine` | gym executor: the production engines to run, `wine`, `water`, `furnace`, `rats`, `fish` joined by `+`, or `none` (`fish`: `sim/gym/Fish.cs`, docs/re/AQUACULTURE.md) |
| `mortgage` | gym executor: sign the loan shark's offer by its rule (`auto`), at the next offer (`take`), not today (`later`: he returns next week), never again (`decline` is final), or only to bridge a rent (`accept_if_rent`). The delta lint turns a `decline` into `later` unless the cash buyout projects within 4 weeks |
| `buyout_by_day` | gym executor: answer the loan shark `later` while the cash buyout projects to be affordable by this day |
| `mortgage_margin`, `mortgage_float` | gym executor: the `auto` rule's cover share of all payments and the cash it keeps beyond 1.5 payments |
| `mortgage_window` | gym executor: `[from, to]`, the days on which it may sign |
| `mortgage_save_from_day` | gym executor: from this day, stop buying and save toward the offer |
| `mortgage_max_spend` | gym executor: daily buying cap while paying the mortgage |
| `appraisal` | gym executor: `join` the appraisal board and serve its clients, or `off` |
| `buyout_save`, `buyout_save_from` | gym executor: the spending cut while saving for the cash buyout (`insurance`, `upgrades`, `inputs`, `vending`) and the day it starts |
| `fountain` | gym executor: buy Wild's water fountain (`buy`) or take the landlord's (`landlord`, +150 weekly rent) |
| `bargain_buy` | gym executor: haggling with sellers (`never`, `ev_positive`, `discount`, `aggressive`); `bargain` is with buyers |
| `speculate` | gym executor: `{types, max_days}`, hold those goods from ordinary buyers until a market event or an Upper Levels buyer with tips, or `max_days` |
| `specialty` | gym executor: `{types, lot_share}`, buy only those goods |
| `lab` | gym executor: the chemistry lab (`docs/research/CHEMISTRY-ENGINE.md`), off when absent: `{products: ["caffeine_pill", "dream_dust", "fanta_pill"], modules: auto\|none\|max, payback_weeks, jackson_need, ratio, …}`; `"caffeine"` is the older form |

The last five are route levers a strategist's fork may change mid-run (docs/STREAM.md, "Model agency"); the gym
reads them from the morning's directives like any route file's.

## Bets
With the bridge's `-bets` (delta directives only), the delta answer may also carry the run's
**bet**: one claim the morning plan stakes, with a deadline, that the show sets up and calls
back to when it is settled (`docs/SHOW.md`, "Bets and callbacks"). Bets are optional and never
directives: they need no `why`, and the executor never sees them. At most one is open.

```json
{"bet": {"claim": "Cash past 2,500 by day 21, rent and all", "kind": "cash", "target": 2500, "by": 21}}
```

| `kind` | Fields | Won when (player-visible state only) | Lost when |
|---|---|---|---|
| `cash`, `net_worth` | `target` | a snapshot on or before day `by` shows it | day `by` closes short |
| `sell` | `uids` or `item` (a name in stock), optional `target` (least the sale brings) | the items are sold by day `by` (for `target` or more) | day `by` closes unsold, or the sale brought less |
| `hold` | `uids` or `item` | day `by` closes with the items still held | one is sold or gone first |
| `upgrade` | `item` (an upgrade on offer; names map to ids) | the snapshot shows it owned by day `by` | day `by` closes without it |
| `event` | `item` (a market item type the snapshot names; any other becomes `judged`) | an active market event raises that type by day `by` | day `by` closes without one |
| `payment` | | a rent or mortgage payment by day `by` | a missed rent, or no payment by day `by` |
| `judged` | | the model's `bet_verdict` says so | the model's `bet_verdict` says so |

"By day N" includes day N's trading and its night (the night's upgrades too): the bet is
over at day N's `sleep` snapshot ("day `by` closes" above). The bridge rejects (and logs `bet rejected`)
a bet with no claim (claims are cut to 140 bytes), an unknown kind, a deadline today or more than 42 days out (a bet settled the night it is staked has no time
to build), one already true when staked (filler), an item not in stock, an upgrade owned or not on
offer, or the bet settled in the last 3 days staked again (Qwen bet on the same upgrade four
nights running, lost each time). A stake whose deadline passed before its plan landed is ignored. `"bet": "drop"` abandons the open bet; a new bet replaces it (the old one counts as
abandoned); the same bet again keeps it. A `judged` bet comes due the morning after its
deadline: the next plan's delta ask (the same morning, or the next weekly plan under
`-plan-every`) tells the model to judge it from the state with
`"bet_verdict": {"won": true|false, "why": "<the fact>"}`; a plan that does not expires it, and
the bridge logs that as a `drop`. Every check above runs on snapshots, not plans, so weekly
plans settle the other kinds on the same day daily ones do. Each plan's delta ask shows the open bet (with the cash or net worth against its target)
and what settled since the last plan.

`internal/bet` checks bets for both sides: the bridge and the show feed the same tracker the same
decision-log events (every `day_snapshot`, sold `trade` outcomes, `money` rent and mortgage
events), so they settle a bet on the same day. The bridge logs each move as `msg=bet day=N
json={"stake":…|"drop":true|"verdict":…}` and each settlement as `msg="bet settled"`;
`/v1/summary` serves the latest plan's move as `bet` (`protocol.BetMove`).

## `POST /v1/outcome`
Sent when the game resolves something a decision started. Fire-and-forget; a decision may
get several (a bargain, then a sale). Example: `protocol/fixtures/outcome.request.json`.
Barter `result.kind`:

| kind | fields |
|---|---|
| `bargain` | `option`, `accepted` (absent when the roll result could not be read — never guessed), `walked_out`, `rep_delta` (faction → delta) |
| `trade` | `side` (`sold`/`bought`), `items` (uids), `cash_delta`, `rep_delta` |
| `left` | `cause` (`dismissed`, `business_done`, `walked_out`, `shutter`) |

Every barter `result` carries `actor`: `mod` (the mod executed it in act mode) or `human`.
Mod actions attach to the decision that was executed; human actions to the client's latest
decision; a client the mod never asked about
(not covered, or the bridge was down) produces none. `trade.cash_delta` is the shop's
signed cash change.

→ `204`. This is what memory and later training learn from.

## Errors and edge rules
- Error bodies: `{"error": "…", "protocol": 1}`. `400` malformed, `404` unknown session
  (the bridge restarted: re-`hello`, then retry once), `405`, `409` protocol mismatch,
  `413` body over 1 MiB.
- `seq` may start at 0 or 1; gaps and reordering are logged, never rejected.
- A `decide` in `observe` mode is answered with `defer`.
- Schemas are strict (`additionalProperties: false`); the Go runtime ignores unknown
  fields. Add a field to the schema before sending it.
