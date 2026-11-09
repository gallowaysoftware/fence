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
→ `200 {"session_id": "…", "protocol": 1, "mode": "observe" | "advise" | "act"}`

`mode` is the bridge's choice. `observe`: events only. `advise`: decisions are requested
and the answer is shown to the player, never executed. `act`: the answer is executed.

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
| `day_snapshot` | `phase: morning` after `waking_up`, `phase: close` after `shutter_closed` | `store`, `reputation` (with `next_perk`), `inventory` (barter item shape + `where` ∈ stock/showcase/counter), `security`, `upgrades`, `services`, `market_events`. Example: `protocol/fixtures/event.day_snapshot.json` |
| `customer_observed` | once per customer when the counter first becomes ready, **covered or not** | `client_seq`, `covered`, `reason`, `context` (barter context v1, as much as applies), `options`. Example: `event.customer_observed.json` |
| `human_action` | every move the player makes at the counter or in the night phase | `client_seq`, `action` (a barter option id, or `stage`/`unstage`/`expose_feature`/`showcase`/…, see `docs/re/DAY.md`), `item_uid`, the `decision_id` and `advised` option in force at the time, if any. Example: `event.human_action.json` |

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
appraisal formulas) is not gated on owning in-game guides. Truth flags (`realCondition`,
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
| `client.faction` | as above, or `tourist`/`church`/`middle` (no rep bar). Shown in-game only as bubble colour; we name it |
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
strategist's plan for the day and the day it was written; absent before the first plan. The mod fetches it on `client_arrived`,
`waking_up`, after each outcome, and at most every 5 s. `404` for an unknown session.

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
