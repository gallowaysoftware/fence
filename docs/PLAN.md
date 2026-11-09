# Plan

Goal: an LLM runs the shop in Probably Stolen — haggles, judges what is stolen, manages
the six factions — and gets better at it across runs. Judgment is the LLM's; execution
(moving, dragging items) stays with the game or the human.

## Direction (revised 2026-11-09)
The game is won across days, not at the counter: what to stock and hold, which factions to
push past a perk threshold or keep off −100, rent and cash flow, how much heat and criminal
record to carry into the next inspection, which upgrades and services to buy. A single
customer is a small, mostly obvious decision, and coercion is rarely used. So:

- **No per-customer solver.** `docs/research/SOLVER.md` stays as reference; its S1–S4 track
  is dropped. A whole run cannot be searched, and the per-turn part is not where the game is.
- **The unit of strategy is the day.** A strategist writes a plan each morning from the
  shop's full visible state; customer advice serves that plan, and the night's reflection
  scores the day against it.
- **The simulator is the game itself.** `sim/extract` runs the real Assembly-CSharp headless
  (~340k idle days/min); `sim/gym` drives it through the same protocol as the live mod, so
  every bridge policy plays the real rules. Validated against recorded real play.
- **Every session is data.** Kyle's own play is the best demonstration data we will get.
  Whenever the game runs, everything a player could observe and every decision (human or
  model) is recorded with its outcome, whether or not the model is advising.

## Phases
| Phase | Deliverable | Exit criterion |
|---|---|---|
| **P0 Wiring** ✅ 2026-11-09 | Mod on the official loader, events reach the bridge, `make compat` | `hello` + a day of events |
| **P1 Advisor + overlay** (built, awaiting `docs/CHECKLIST.md`) | Per-customer advice in a turn-based overlay | Context matches the screen for a full day |
| **P2a Actor** (built, off by default) | Guarded execution of counter moves | A day of standard trades, human only staging |
| **D Always recording** | Bridge as a user service that is up whenever the game is; mod buffers while it is down and the overlay shouts when it is not recording; observation of the whole day: every customer (covered or not), the human's moves, daily state snapshots (inventory, showcase, cash, rent, reputations and perks, security record, upgrades, services, market), the night phase and after-hours choices; `fencedata` export of per-run trajectories with hidden values as labels; `make data-status` | Every session since D landed appears in the export with no gaps |
| **S Strategist** | Morning plan from the day snapshot + memory (goals, cash floor, faction targets, stock to move or hold, heat limits); plan shown in the overlay and fed to every customer prompt; nightly reflection scores the day against the plan | Plans are specific and visibly change advice; reflection cites them |
| **N Night phase** | Advice for suppliers, upgrades, services and after-hours locations (advise first) | A week of night advice Kyle agrees with or overrides for stated reasons |
| **L Learning from play** | From the recorded runs: what Kyle does that the model doesn't (imitation), what worked (outcome-labelled), lessons and price/odds priors; later a distilled local model | Measurably closer to Kyle's choices and results on held-out days |

Models: drift's Qwen3.8-Flash-Next (`make bridge-drift`); the bridge also speaks Anthropic
and any OpenAI-compatible endpoint.

## P0 field notes (build 26525552, two in-game days)
- All 17 event hooks fire, `seq` 1–28 with no gaps; the mod disables cleanly on quit.
- `customer_generated` fires once per shutter-open (the day's queue), not per customer.
  Per-customer arrival needs the P1 hook `StoreUIManager.OnNextClientArrived`.
- The day counter advances during sleep: `afterhour_event_*` after `going_sleep` already
  carries the next day.
- New local mods start disabled; enable in the Mods menu, then restart.

## P1 notes
- Model: Qwen3.8-Flash-Next on drift (`make bridge-drift`): thinking off, temperature 0.2,
  no `response_format` (drift lacks llguidance). Scenario eval 14/15 acceptable, p95 2.1 s
  warm; the mod's deadline is 10 s. `make eval-drift` re-runs it.
- Option labels state the three roll outcomes separately (`accept 82%, refuse 8%, walk out
  10%`): the model misread cumulative bands.
- Memory is on by default (`var/memory.db`): price ledger, bargain odds observed per
  faction/rep band, client history, nightly reflection into per-run lessons.

## Overlay and P2a notes
- The overlay (`CustomUIManager`, a supported surface) treats a customer as a turn and a
  day as a meta-turn: header, this customer (advice, every option with the bridge's
  `option_notes`, result), today (customers so far, `/v1/summary` totals and lessons).
- Acting needs bridge mode `act` AND the mod setting `act_enabled` AND not paused/faulted,
  re-checked before every game call; only `BarterExecutor` calls game actions (Cecil
  tests). The overlay's Pause/Resume is the kill switch. The human still stages items.

## Learning
1. **Structured memory** — prices and customer history are facts; a table beats vector
   search. Injected into the prompt per decision.
2. **Reflection** — after each in-game day, the model reviews that day's decisions and
   `outcome`s and rewrites a bounded lessons file that every prompt includes.
3. **Distillation** — not RL: the game is real-time, single-instance, a few hundred
   decisions per day. Filter by outcome, fine-tune, compare.

## Settled by reverse engineering (`docs/re/`)
- Decision point: no single game method; the mod polls the game's own bargain-enabled
  conditions after `StoreUIManager.OnNextClientArrived`. Outcomes: `PlayerStore.OnItemBought`,
  `StoreClient.OnItemBought`, `PlayerStore.DismissCurrentClient`.
- No pausing; stale decisions are caught by `state_signature`.
- First coverage: ordinary buying/selling customers. Barter, appraisal, info dealers,
  gunsmith, procurement, inspections and story clients defer.
- Appraisal (exposing features with a tool) is advise-only; calling the expose method
  directly would skip the tool and cheat.
