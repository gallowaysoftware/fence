# Stream: an LLM plays a whole run, live

The north star: a live stream in which a language model plays a whole run of Probably Stolen
unattended, in the real game, with its reasoning on screen. This doc is the concept, the
feasibility study of making the gym's executor play the live game (the "autopilot"), and the
milestones to the first stream. The show itself (layout, TTS, pacing of commentary) is
`docs/SHOW.md`.

## 1. The concept

**On screen:** the game window, a reasoning panel beside it, optional TTS narration.

- **Roles.** The LLM is the *strategist*: each morning it reads the shop's visible state and
  writes the day's plan (prose for the panel, directives for the executor: `docs/PROTOCOL.md`
  "Directives"); it may also take counter decisions and comments on what happens. The
  *executor* (the gym's `-human player`, `sim/gym`) is the hands: it stages, sells, buys,
  tends machines and runs the night by the plan. The *autopilot* is the mod-side adapter that
  lets the executor play the live game at a watchable pace.
- **Rotation.** Several LLMs take turns across runs (one model per run, recorded in `hello` and the
  run metadata), so the stream doubles as a league: same game, same executor, different
  strategists. Mirror runs (same seed or save, different model) are cheap because everything
  but the strategist is fixed.
- **Information rule: visible + hidden, flagged.** Only player-visible information appears on
  screen, the same split as `decide.context` / `decide.hidden`. Every feed entry carries the
  visible facts it rests on; an entry whose rule read a hidden value is marked
  (`hidden_used`), its reason is shown without the hidden value, and the panel shows the
  marker, so viewers can see where the hands knew more than a player could. The goal is zero
  marked entries (the fairness pass, M3).
- **Why it is watchable.** The interesting decisions are the day's: what to stock and hold, when
  to take the mortgage, which upgrade, whether to bribe or buy the permit, when to shoot. The
  plan is on screen in the morning and the day shows whether it worked; the counter shows every
  haggle with its odds. Normal is won by the executor in ~58 of 60 runs (buyout around day
  180–190); hard in 8 of 60 with the income set (1 of 60 on another seed set): a hard run is
  a real contest.

**Length.** The gym averages ~9 customers and ~11 counter decisions a day. At about one action a
second plus the game's own walk-ins, dialogue and blackscreens, a day takes 3–4 minutes, so a
normal run to buyout is ~10–12 hours: a run spans several streams (the game saves every night;
the autopilot resumes from a save).

### Model agency: the decisions viewers argue about

A delta strategist over the scripted executor reads as a script with a commentator (Qwen won every league game but
2–5 days slower, never better). So the model also gets the run-shaping decisions, each one on screen with its reason;
the executor keeps the routine.

- **Day 0: the route.** Before day 1 the model sees the route menu (`docs/research/ROUTES.md`: each route's start,
  perks, persona, brief and lab record, from `internal/agency/lab.json`) and either picks a route or composes its own:
  a menu route as the base plus levers from a documented list (`agency.LeverText`). A composition is validated before
  it plays: contract keys against `protocol/schema/directives.schema.json`, gym-only levers against their bounds, and
  the combinations the executor cannot play (rats off the rancher start, water off the water merchant's, a fountain
  without water, the Board switched off) are refused; one repair round, then the menu's default. When the match fixes
  the start, only routes that play it are offered. The model writes a one-paragraph thesis: it goes to the feed as a
  `plan` event (level 2) and into every later plan and fork prompt. The stream's previous runs are named in the prompt
  so the choice rotates. League: `"choose_route": "any" | "start"` on a plan player (`cmd/fenceleague/agency.go`; the
  match keeps its scheduled route slot as its id, `match.json` records `chosen_route`, the answer is `choice.json`, a
  composition `route.json`); bridge: `-choice FILE`.
- **Forks** (`-forks`, `internal/strategy/forks.go`): when the morning shows one arising, the model is asked before the
  plan, with the visible facts and the options, and answers `{"choice", "why"}`. The choice is a directive patch held
  over every later day (until the event ends, for a hold); an unusable answer leaves the route's own rule.
  | Fork | Arises | Options |
  |---|---|---|
  | `mortgage` | the loan shark's offer is on the table and the cash covers the down payment; again a week after "later" | later / sign / decline (final); the facts include the lab's cost of signing (a mortgage wins only at its last payment, ~day 170 when signed near day 55). "sign" also moves the route's `buyout_by_day` to day 1, which would otherwise answer "later" for it |
  | `sprint` | first morning from day 54, not owned, not paying, the route saves for a cash buyout | sprint as the route says / all in (cut insurance and upgrades from today, cash by day 112) / keep the insurance / give up the race and sign |
  | `fountain` | the route runs water, Wild offers the fountain and the cash covers it + rent + 500 (up to 3 asks, a week apart) | not now / buy Wild's / the landlord's |
  | `engine` | once, from day 7, cash ≥ 1 500 + rent, and the route lacks the furnace (or the rats on the rancher start) | stay / light the furnace / start the rats |
  | `style` | day 28 and day 63 on a route without a trading style | the generalist / the six styles of TRADING-STYLES.md, each with its lab Δ |
  | `hold` | a market event lifts medical, food, water or materials by 30 %+ and the shop holds 3+ of them (once per event, a week apart) | sell into it / hold those items to the event's end / buy more of the type. **Never arises yet:** the snapshot's inventory items carry no type (gym and mod), so neither this fork nor the style fork's stock mix can see them |

  Each fork is a `candidates` feed event (metric `choice`: the panel shows the options and marks the chosen one, no
  score), level 2 for the mortgage, the sprint and the fountain or when the model leaves the route's rule; the bridge
  log has a `fork` line per decision. The directive keys the forks need beyond the contract's (`buyout_save`,
  `buyout_save_from`, `fountain`, `bargain_buy`, `speculate`, `specialty`) are now in the contract (PROTOCOL.md).
- **Plan cadence** (`-plan-every N`): the morning plan is written every N days and held, with the forks' patches, in
  between (the mid-day re-plans are off). N = 7 keeps a game to ~40 model calls instead of ~350.
- **Lean day-0 prompt** (`"agency_lean": true` on a league player): the menu as one line a route (its levers over the
  appraiser's and its record) and the levers by name, ~1.6k tokens instead of ~6.8k; drift's MLX server refused the full
  prompt for memory (HTTP 507).
- **Inventor** (`cmd/fenceinvent`, offline): the model proposes routes the menu lacks (compositions plus a thesis and
  a novelty claim); the gym screens each on 30 hard seeds against the `appraiser`, and only a 57/60 confirmation on 60
  fresh seeds puts one on the menu (ROUTES.md, "Invented routes").

**Measured (2026-11-11, hard, Qwen-27B local, `-forks -plan-every 7`, against the scripted `appraiser` on the same
seeds; `league/a1`–`a4-agency-qwen.json`, seeds 8510001–032).** Every game won on both sides (32/32 each). Paired win
day **+13.3 ± 11.1** for the model (median day 100.5 against 90). Splitting it with a third arm, the scripted executor on
the route the model chose: the route choice itself costs nothing (+0.2 ± 8.5), the in-run layer (forks + weekly plans)
costs +13.1 ± 13.4, and almost all of that is the mortgage. Until a3 the fork's "sign" was silently overruled by the
route's cash rule; a3 made it real and the model signed in 3 of 8 runs, each won by the mortgage on day 167–174 (+70 to
+91 days). a4 added the lab's cost of signing to the fork's facts and the model declined in all 8 runs: paired
**+0.0 ± 13.0**, within noise of the script.

| League | Menu | Mortgage "sign" | Paired Δ win day | Route alone | In-run |
|---|---|---|---|---|---|
| a1 | 22 routes | overruled (bug) | +6.1 ± 13.1 | −2.6 ± 13.9 | +8.8 ± 8.9 |
| a2 | 26 (4 invented) | overruled (bug) | +18.4 ± 26.2 | +5.2 ± 5.0 | +13.1 ± 26.4 |
| a3 | 34 (12 invented) | real | +28.9 ± 29.7 | −3.5 ± 7.8 | +32.4 ± 34.5 |
| a4 | 34, the cost of signing shown | real | +0.0 ± 13.0 | +1.8 ± 31.2 | −1.8 ± 29.6 |

Routes chosen (32): rancher 8, smelter 7, apothecary 5, pharma 2, rancher_sprinter 2 (invented), fountain, watchman,
bulk_buyer, outlaw, appraiser (a fallback: levers held `perks` and `persona`; now lifted), and three compositions:
`ironclad` (watchman + two eco furnaces on phoned ore), `iron_sprinter` (smelter, one furnace, sales to 50 %, cash by day
92) and `pharma_smelter` (on the rancher). The rotation prompt ("the stream's previous runs played …") is what spreads
them: the first choice of every league was smelter, rancher or rancher_sprinter, the fastest medians. Forks asked: 128
(sprint 32: "sprint" 31, "keep the insurance" 1; style 54: generalist 26, speculator 11, fair dealer 9, apothecary 3,
clean shop 4, hardware 1; mortgage 40: decline 19, sign 13, later 8;
engine 2: stay), none unparsable. Latency on the 27B (2 slots shared with another league): day-0 choice 11–17 s median
(~4.8–6.8k prompt tokens), a fork 4–6 s (~830 tokens), a plan 40–70 s; ~40–65 calls and 15–30 minutes a game.
**drift** (Qwen3.8-27B on MLX, lean prompt, one request at a time, 3 seeds before the cost-of-signing fix): day-0 8.2 s
median (2.3k tokens), a fork 3.9 s median (830 tokens), a plan 31 s median; it chose rancher_sprinter, ore_monopolist
and pharma (two invented routes), and signed the mortgage in 2 of 3 (+98 and +70 days); the third won 28 days early.
After the fix (2 seeds, `league/d2-agency-drift.json`): rancher_sprinter and rancher, declined both offers citing the
lab line, won on day 90 both (−14 and −14 against the script); day-0 7.6 s, a fork 3.5 s, a plan 23 s median; 10–12 minutes a game.

## 2. Where we are

- **Executor** (gym): the best hard policy, `-start apprentice -perks vending+short_sleeper -directives
  sim/lab/scripts/directives/hard_income_mortgage_appraisal.json` (the appraisal board and the
  mortgage at its first offer), wins hard 60/60 and normal 60/60 on fresh seeds (2400001–060, 2400201–260; section 7). It
  drives the game's own `Assembly-CSharp` headless, through the same readers the mod uses
  (`GameBarter`, `GameDay`, `BarterExecutor` are compiled into the gym).
- **LLM strategist**: delta directives play within noise of the executor alone
  (`docs/PROGRESS.md` 02:50); the bridge serves them in `/v1/summary`.
- **Live mod (P2a)**: executes counter moves of covered customers only (`BarterExecutor`,
  behind `ActGate`, with the overlay's Pause/Resume as kill switch). The human stages items and
  does everything else.
- **Shared executor**: the executor's decision files compile under the mod's rules and pass
  the game's `ModScanner` with no capability (`mod/src/Fence.Autopilot`, section 4), and ship
  inside `Fence.Mod.dll` (`mod/src/Fence.Autopilot/SharedExecutor.props`).
- **Live autopilot (M1, M2 and M3's gun built, none yet run in the game; the owner's tests are
  sections 8, 9 and 12)**: `AutopilotDriver`, `LiveHands`, `LiveGame`, `LiveHost` in the mod, the planner in
  `Fence.Core/Autopilot`. M2 plays the whole day and night of the appraisal policy by the
  executor's rules (section 5, "M2 as built"), logs every action with its reason, and stops and
  shouts on anything else (section 10). M3 adds the gun on a visible-tell verdict (section 5, "M3 as
  built"); the gym's executor uses the same detector by default.

## 3. Feasibility: every executor action in the live game

Classes:

- **(a)** the same game method is safe to call in the live scene (it is what the click handler
  calls, with the handler's gate replicated);
- **(b)** needs the UI flow: drive the game's own handlers in the order a click would, so
  windows, animations and state stay consistent;
- **(c)** a headless-only shortcut in the gym that would cheat or desync live (live uses the
  path in the "live" column instead);
- **(d)** unknown until tried in the live game.

Surface: **S** = officially supported mod surface (`ModHelper`, `ModHook`, `ModConfig`,
`CustomUIManager`, `ModReflect`); **U** = unsupported game internals (compat risk on every
patch; each member goes into `compat/hooks.json` under an `autopilot.*` feature so a missing
one disables only that feature).

Evidence is the game's own handlers (local decompile, never committed) and `sim/gym`.

### Day flow

| # | Action | Gym (`sim/gym`) | Live path | Class | Surface |
|---|---|---|---|---|---|
| 1 | Morning report panel | not shown headless | `StartOfDayUIManager.OnStartDayButtonClicked` | b | U |
| 2 | Arrange the window | `GraphUtils.TryAcceptAll` stock ↔ case (`Episode.ArrangeWindow`) | `ModHelper.MoveItemToDisplayCase` / `MoveItemToInventory` (the same call); gate `GameItem.MayRemove()` as quick-transfer does | a | S |
| 3 | Open the shutter | `StoreShutterButton.OpenShutter` | `StoreShutterButton.OnClick` (gates: `lockShutterButtonFlag`, arrest/shootout, `CanStartDay`) | b | U |
| 4 | Call the next customer | `TryCallNextClient` + `Arrive()` replicating `StoreClientMono.OnArrived` without the walk-in | `StoreBell.OnClick` (rings `TryCallNextClient`); the walk-in tween and `OnArrived` run themselves | c → a | U |
| 5 | Dialogue lines | pumped clock; stalled lines clicked through | the player's auto-advance setting, else `DialogUIManager.OnAdvancePressed` | c → b | U |
| 6 | Dialogue choice (rent, buyout, landlord's offers, permit, Georgio, loan shark/mortgage, inspector bribe, story) | `DialogUIManager.SelectChoice(i)` | same; the choice button calls it. The mortgage is dialogue only | a | U |
| 7 | Stage for a buyer | stock/case → back counter (`Episode.StageBuyer`) | `ModHelper.MoveItemToBackCounter` + `MayRemove()`; one item per paced step | a | S |
| 8 | Pull exposable items off the counter | back → stock (`PullExposable`) | `ModHelper.MoveItemToInventory`; **reads hidden truth** (`CanClientExposeAnyFeatureDeep`) | a (flagged) | S |
| 9 | Stage a seller's items | front → back counter (`StageSeller`) | `ModHelper.MoveItemToBackCounter` | a | S |
| 10 | Counter move (sell/buy all, wholesale, bargain, blackmail, threaten, dismiss) | `BarterExecutor` (shared with the mod) | `BarterExecutor` behind `ActGate` (P2a) | a | U (negotiation explicitly unsupported) |
| 11 | Unstage leftovers, stow gifts | back/front → stock | `ModHelper.MoveItemToInventory` | a | S |
| 12 | Permit official's donation | stage + `OnSellAllClick` + `OnConfirmPanelYes` (`TryBribe`) | same as #7 + #10 | a | S + U |
| 13 | Dismiss a customer | `TryPlayerDismissCurrentClient(false)` | same (`StoreDismissButton.OnClick` calls it) | a | U |
| 14 | Force-dismiss a stuck customer | `DismissCurrentClient(false, true, true)` | never live: stop and shout | c | – |
| 15 | Uncovered customers (story, security, info dealer, barter, appraisal) | dialogue played, then dismiss/force | dismiss when `IsClientDimissable`; otherwise stop and shout until each kind has a handler | d | U |
| 16 | Aug check | `AugCheck` + the shared `AugRule` on the **visible tells** (default since M3; `-aug-detection perfect` keeps the truth flag for analysis) | the same: `AugCheck` in tells mode, `AugRule` (section 5, "M3 as built") | a | – |
| 17 | Shoot | replicates `StoreUIManager.Shoot` and Security's verdict dialogue | `Shotgun.OnClick` (the gun's own click: `EnterShootingMode`, blackscreen, camera, the customer's reaction), then Arm's left click `StoreUIManager.Shoot` behind `CanShoot`; the wound, Security's walk-in and verdict are the game's; its lines are advanced like any dialogue | b | U |
| 18 | Close the shutter | `StoreShutterButton.EndAndClose` | `StoreShutterButton.OnClick` → `CloseShutter` (confirm panel when the queue is not empty) | b | U |

### Night and back office

| # | Action | Gym | Live path | Class | Surface |
|---|---|---|---|---|---|
| 19 | Go out (dump, Dr. Jackson) | `MapUIManager.Travel(loc)` + scene-root shims | `StoreDoor.OnClick` → `OpenGoOutsideConfirm` → `OnGoOutsideConfirm` → location button (`Travel`). **The door gates the gym skips**: seriously wounded, `crackdown`, agoraphobia rest, afterhours not introduced | c → b | U |
| 20 | Scavenge | `MapUIManager.ScavengeButtonClick` | same (the button's handler) | a | U |
| 21 | Bring the loot home | afterhour bag → stock while outside | move after return, or `ModHelper.MoveItemToAfterhours`/`MoveItemToInventory` where the windows allow (quick-transfer routes to the bag outside) | d | S |
| 22 | Shop at Dr. Jackson's | his stock placed headless; stage + `OnBuyAllClick` | location's `Arrive` lays out the stock; stage + `OnBuyAllClick` | a | S + U |
| 23 | Return | `MapUIManager.Return` | same; `CheckItemLeftOnGround` may open a confirm panel | b | U |
| 24 | Wild's Network: open | `WildNetwork.Open` (non-drawing half of `WildNetworkUI.Open`) | Joe's visit opens it; at night dial 8376 on the phone (`PhoneUIManager`); `ModHelper.OpenWildNetwork` as fallback (skips the call) | c → b | U / S |
| 25 | Wild's Network: buy | replicates `WildNetworkUI.OnUseClicked` (favor first, then cash, `RunStats`, `Unlock`) | select the entry and invoke `OnUseClicked` (private: `ModReflect` on a literal `typeof`) | b | S (`ModReflect`) |
| 26 | Wild's Network: close | private `OnWindowClosed` by reflection | `ModHelper.CloseWildNetwork` | c → a | S |
| 27 | Nightly services | sets `StoreService.isUsing`, replays the tablet's rules | `StoreBedroll.OnClick` opens the tablet; toggle as `OnToggled` does (`isUsing` + `RefreshUI`); the tablet applies its own rules | b | U |
| 28 | Sleep | invokes `OnEndDayButtonClick`'s closure by reflection | `EndOfDayUIManager.OnEndDayButtonClick` | c → b | U |
| 29 | Phone a supplier (mining courier, Red Imp) | sets `phoneState`, `UseService`, `QueueFuturClient` directly (`Furnace.Phone`, `Beer.Morning`) | `PhoneUIManager.OpenUI` → `AutoCall(number)` → `DialoguePhoneUIManager.SelectChoice` | c → b | U |
| 30 | Wine: fill, add berries/yeast, start, empty | `WineHelper.PourRationIntoBottle`, `WaterHelper.TransferLiquid`, moves, `TryStartFermentation`, `EmptyWineBottle` | the bottle's own funcs: item-target (`canItemTargetThisFunc` gate) for pours, the window's start button callback is exactly `TryStartFermentation`, activate func for emptying | a (d for pour amounts) | U |
| 31 | Hydroponic bays: seed, water, tablets, battery, harvest | moves, `TransferLiquid`, `ChemicalHelper.DissolveInto` | same methods via the item-target handlers; machine windows closed | a / d | U |
| 32 | Furnace: modules, battery, load, collect | moves into the machine's body storages | same moves (`GraphUtils` on machine inventories) | a / d | U |
| 33 | Vending: collect, restock, power, upgrade | `VendingMachineHelper.TransferCashToPlayer`, moves, `UpgradeAttractiveness`/`UpgradeCapacity` | same (the collect button and the core's item-target funcs call them) | a | U |
| 34 | Smuggler's bays | moves into the bay | same | a | U |
| 35 | Poster stash | moves into `hiddenElement` with the poster down; sets `isPulled` | `StorePoster.PullUp`, move, `PullDown` | c → b | U |
| 36 | Water engine: filter, tablets, test strip, relabel, bottle printer | helpers; printer pressed via a reflection walk (now behind `IExecutorHost.MachineButton`) | item-target handlers; the strip's Match; the printer window's button | b / d | U |
| 37 | Time, coroutines, morning endings, handler liveness | virtual clock, `PassTime`, `MorningEnding`, `MarkHandlersAlive` | the game does it | c (not needed) | – |

**Counts** (37 rows, each counted once by its live path, a mixed row by the harder class):
**(a) 16** (#2, 4, 6–13, 16, 20, 22, 26, 33, 34; #8 flagged for its hidden read), **(b) 13**
(#1, 3, 5, 17, 18, 19, 23–25, 27–29, 35), **(c) 2** with no live counterpart (#14 force-dismiss,
#37 headless plumbing), **(d) 6** (#15, 21, 30–32, 36). Eleven rows are gym shortcuts that
the live path must not copy (#4, 5, 14, 17, 19, 24, 26, 28, 29, 35, 37); #16 is now the same
code in both. On the supported
surface: the item moves of #2, 7–9 and 11 (`ModHelper.MoveItemTo*` is exactly the gym's
`GraphUtils.TryAcceptAll` on the same grids), #26, and #25 through `ModReflect`; everything else
is game internals.

**Blockers, in the order a stream meets them:**

1. **Uncovered customers (#15).** Story, Security, info dealers, appraisal and barter
   customers are dismissed or force-dismissed headless. Live, a non-dismissable one stops the
   autopilot until each kind has a handler; until then the stream needs a human on call.
2. **Shooting (#16, #17).** Built in M3 (section 5, "M3 as built"): the executor detects augs
   from visible tells only and the live hands drive the gun through its own handlers; not yet run
   in the game (the owner's test is section 12). Before M3: perfect aug detection was a cheat and
   the shooting scene had never been driven by the mod. A stream can start
   with `augs: serve` on normal: presence still rises, but full presence ends a run only on
   hard and challenge (`DifficultyRules.augGameOverAtFullPresence`). Measured: serving costs 8
   of 60 normal wins; on hard it ends every run by about day 46 (section 7).
3. **The night's UI flows (#19, #23, #24, #27, #28, #29).** Door, map, phone, tablet, bedroll:
   each is a few clicks through public handlers, but none has been driven live. Scavenging is
   essential (hard: `scavenge: false` costs ~100 days), so outings are in the first stream.
4. **Hidden reads in the executor's rules** (fairness, not feasibility): `Keep`/`PullExposable`/`HidesFake` (an item's true features), water purity before a test.
   These are marked in the feed until replaced by visible proxies.
5. **Closed machine windows (#30–#32).** Moving items into a machine whose window is closed is
   what the gym does; that the window shows them when next opened is unverified.

**Gym fidelity finding (fixed, M2): the return prompt.** After a dump outing whose loot did not
fit the stock grid, `MapUIManager.Return` opened its "leave items on the ground?" prompt
(`CheckItemLeftOnGround`), which the gym never answered: the outing stayed open (`map.Current`
set) overnight and from then on, and counter purchases landed on the ground (the afterhours bag),
to be carried in by a later night's outing. The gym now brings the loot home most valuable first,
leaves cheap stock on the ground to make room when the loot is worth twice it
(`NightOuting.RoomFor`, shared with the live hands), and answers the prompt as its confirm button
does (the rest stays on the ground); Dr. Jackson's purchases that fit neither stock nor case stay
on his back counter, which his `Leave` carries home. Effect in section 7.

**Gym fidelity finding (fixed).** `Episode.Outing` called `MapUIManager.Travel` directly and so
skipped `StoreDoor.OnClick`'s gates: no outing while seriously wounded (`woundState > 5`), during a
`crackdown` event, while the agoraphobia perk rests, before afterhours is introduced, or once
tonight's visit is used. The gym now applies them (`Episode.DoorShut`, default `night:door_shut`).
Nearly every refusal is a serious wound from scavenging (about 30 nights a normal run, 35–40 a hard
one); the executor keeps planning outings it cannot take. 60 fresh seeds each: hard income set 7/60 →
1/60 wins, hard with the appraisal board 60/60 → 59/60, normal 58/60 → 57/60 (`docs/PROGRESS.md`).

### Constraints that hold for every live action

- **Main thread only**, from a coroutine (`ModHelper.StartCoroutine`); no `Task`, `Thread` or
  timers; waits with `WaitForSecondsRealtime`. Never `Time.timeScale` (`docs/re/TIME.md`).
- **Quiescent points only**: no line playing, client arrived and not walking, not trade-locked,
  no blackscreen sequence, not `MapUIManager.IsInTransit`, no confirm panel open, not loading
  (`ModHook.OnGameLoaded*`), not in a menu, intro or tutorial, not after game over. After an
  action, wait for quiescence again before the next.
- **Gates before calls**: `MayRemove()` for moves, the item-target `can…` funcs for pours and
  upgrades, the button's own conditions for shutter/door/bell, `ActPlanner` re-validation and
  `state_signature` for the counter (as P2a).
- **Every action is re-read afterwards** (the item is where it was sent, the choice took, the
  client left within a timeout); a miss is a fault, never a retry loop.

## 4. Autopilot architecture

```
bridge (Go)                          mod (Fence.Mod, in game)
-----------                          ------------------------
strategist → /v1/summary ─directives─▶ AutopilotDriver (coroutine, 4 Hz)
  plan + directives                    │  phase from game state → one action per step
                                       │  pacing, kill switch, watchdog
LLM counter (act mode, optional) ◀────▶│  PlayerRole / PlayerPolicy / engines  ← shared with the gym
                                       │  LiveHost : IExecutorHost
decision log ◀── autopilot events ─────┤  LiveHands: the only class pressing non-counter buttons
  └▶ fenceshow → panel, TTS            │  BarterExecutor (counter, existing)
     (docs/SHOW.md)                    └▶ overlay: Pause/Resume, status, fault banner
```

**Shared decision logic.** The executor's decision files stay where they are in `sim/gym` (other
work edits them daily; a move would conflict with all of it) and are compiled into both: the
gym as today, and `mod/src/Fence.Autopilot/Fence.Autopilot.csproj` under the mod's rules
(netstandard2.1, C# 9, the game's assemblies). That project lists the shared set: `ExecutorHost`,
`Directives`, `Knobs`, `HumanRole`, `KnobHumanRole`, `PlayerRole`, `Production`, `Water`,
`Furnace`, `Vending`, `Conceal`, `Beer`, `Breadth`, `Appraisal`, `Labels`, `Modules`, and since M2
`ExecutorMorning`, `PlayerCounter`, `NightOuting`, `WildRead`. Two gym-only pieces they call have
mod stand-ins in `mod/src/Fence.Autopilot/`: the file-writing `DemandTrace` (silent) and
`ModSide.CurrentDay`. A test runs the game's `ModScanner` over it and
fails on any capability (file, reflection, thread). Anything a shared file needs from its driver
goes through `IExecutorHost` (the day, item ids, recording a move, reporting an error, pressing a
machine button); file I/O and reflection live in gym-only files (`Directives.File.cs`,
`Episode.Host.cs`). The gym stays byte-identical across every such change (the parity gate,
section 6). Since M1 the same `<Compile>` list (`SharedExecutor.props`) is in `Fence.Mod.csproj`,
so the mod ships the executor inside `Fence.Mod.dll` (one assembly; no new manifest entry); every
game member it touches is in `compat/hooks.json` under `autopilot.executor`, the driver's own under
`autopilot.hands`, and a missing one makes the autopilot unavailable rather than wrong.

The refactors that moved the executor behind the host (each with the parity gate, section 6):

1. **Done (M2).** The morning plan (`Episode.PlayerMorning`: floors, cash target, buyout and
   mortgage saving, Georgio, price-rise types, the bribe reserve) is `ExecutorMorning.Plan(host,
   days)`, and the engines' chores are `ExecutorMorning.Chores(host)`. The ledger it reads is a
   list of days (`sold_value`, `net_worth_start`, `net_worth_end`): the gym's `PlayerLedger`, live
   the game's own `RunStats.netWorthByDay` (the calendar's wealth chart) plus the session's sales.
2. **Done (M2).** The counter-adjacent decisions are `PlayerCounter` (`BribeReserve`, `BribeDue`,
   `BribePicks`, `Exposable`, `JacksonStage`) and `Appraisal` (`Next`, `Refused`, the kit and the
   slips); each driver makes the moves.
3. **Done for the phone (M2).** `Furnace.Phone` and `Beer.Morning` decide the order and call
   `IExecutorHost.PhoneOrder`: headless the call's effects as its dialogue applies them, live a
   queued call through the phone. Every item move and machine action of a shared engine goes
   through `Fence.Gym.Chore`: headless the game call itself, live the gated hands (section 5).
   Still a gym shortcut: `Conceal`'s poster stash (`isPulled`), which live is off (M3).
4. `Episode`'s customer loop is the gym's *driver* and stays gym-only; the live driver is a
   second driver that calls the same role methods (`Window`, `StageForBuyer`,
   `StageFromSeller`, `Dialogue`, `Night`, `Services`, `Upgrades`, `PlayerPolicy.Pick`).

**The live driver** (`AutopilotDriver`). A coroutine at 4 Hz, like `BarterAdvisor`. Each tick it
reads the phase from game state (`storeState`, the morning/end-of-day panels, dialogue with
choices, client present and ready, `MapUIManager.IsOutside`, Wild's window, the phone) and, if
quiescent and not paused, takes **one** action: the next item of the current batch (a staging
list, the window's moves) or the next decision. Engines' morning chores run in the morning
before the shutter opens, in one frame (they are back-office moves into machines), and are
listed in the feed line by line.

**Pacing.** `pace_ms` (default 1 200) between actions, scaled per kind (moves ×0.5, decisions
×1, dialogue choices by the length of the line); `pace_hold_ms` holds before a big decision
(the morning plan, a mortgage, a shot) so the panel or TTS can land it, bounded. Configurable in
the mod's settings and live from the bridge (`/v1/summary` → `autopilot.pace_ms`), so the show
can speed up quiet stretches.

**Kill switch.** The overlay's Pause/Resume (`ActSwitch`) stops the driver and the counter
together, checked before every action. Also a game-option key bind
(`ModHelper.AddGameOptionKeyBind`, supported). **Any human input pauses it**: the recorder
already tells the executor's moves from the player's (`Recorder.Filter.EnterExecutor`), so a
human move while the autopilot runs pauses it and says so.

**Faults: stop and shout.** Any exception from a game call; an action that did not take (the
re-read in section 3); a phase with no progress for 60 s (watchdog); a dialogue with choices
from a client the executor has no rule for; a non-dismissable uncovered customer; a compat
feature missing at `hello`. The driver pauses with the reason, the overlay shows a red banner,
and an `autopilot_fault` event goes to the bridge so the panel shows "paused, needs a human".
Resume is manual. Not a fault: the bridge being down (the day plays on the base directives and
the panel says the strategist is offline), a stale counter decision (re-asked, as P2a).

**Recording.** Every action is an `autopilot` event: `{day, phase, action, target (uid, name),
detail, why, rule, plan_day, hidden_used}`, sent through the existing event queue (buffered
while the bridge is down) into the bridge's decision log, where `cmd/fenceshow`'s live hook
already reads (docs/SHOW.md; it maps them to `thought` events, closing SHOW.md's gap "the live
real game has no gym executor reasons"). Hidden values a rule read go in the event's `hidden`
object, which the show's decoder cannot see; `hidden_used` names them without values, so the
panel can mark the entry. The rationale comes from the executor's existing
rule keys and details (`Defaults.Take`, the ledger's `why`). The usual `human_action` stream
continues with `actor: autopilot`, so `fencedata` exports keep human and autopilot play apart.
The show already rehearses on gym games (`fenceshow -replay`, from `gym.summary.json`'s
per-customer reasons); the autopilot's events carry the same reasons live.

**Strategist and counter.** Mornings: the driver waits at the morning panel, bounded by
`plan_wait_s` (default 90), until `/v1/summary` has today's directives (the bridge's
`planGate` already holds a day for its plan), merges them over the base directives as the gym
does (`Directives.Merge`), and puts the plan on the panel. Counter: by default the bridge runs
in `observe` and the executor's own `PlayerPolicy.Pick` plays (the gym's
`PlayerUnderADeferringBridgePlaysItsOwnCounter` holds that path equal to the executor alone);
`act` mode lets the LLM take counter decisions through the existing P2a path. Commentary and
TTS are the bridge's (docs/SHOW.md): the mod never waits on narration except the bounded
holds above.

## 5. Milestones to stream 1

| Milestone | Scope | Exit test the owner runs |
|---|---|---|
| **M0 Shared executor** (done) | Executor files compile under the mod's rules; scanner-clean; gym unchanged | `make test` green; parity: 60 normal + 60 hard seeds byte-identical before/after |
| **M1 One unattended day** | Refactors 1–2 above; `AutopilotDriver`, `LiveHands`, `LiveHost`; morning panel, window, shutter, bell, staging, counter (executor via observe mode), unstage, dismiss, dialogue choices, close, services, sleep. No outings (`scavenge: false` for the day), no phone, `augs: serve` | Load a normal save at a morning. Bridge in observe with a base directives file. Settings: `autopilot` on, `pace_ms` 800. Press Resume, hands off. Pass: the next morning arrives with no input; the feed has a line with a reason for every customer and move; no `human_action` with actor `human`; overlay Pause mid-customer stops within one action and Resume continues; one deliberate hand move during staging pauses it with a shout |
| **M2 One unattended week** (built) | Night flows (door, map, scavenge, Jackson, return and its prompt), phone (Wild, suppliers), engines' morning chores (wine, bays, vending, furnace) and the bedtime restock, permit donations, the exposable pull, Joe's visit, rent day, the appraisal board (join, kit, clients), the shared morning plan, resume from a save | Section 9: seven consecutive days on a hard apprentice save (day 3–7) with the appraisal policy. Pass: 7 mornings, zero inputs, no faults (handovers only of section 10's kinds), every phase in the feed, rent paid; quit to menu mid-week and reload: it resumes |
| **M3 A whole hard run** (the gun built, section 5 "M3 as built"; the rest open) | Shooting with visible-tell detection, inspections and the poster stash, the mortgage, uncovered-customer handlers; the fairness pass (no `hidden_used` entries, or each one justified and measured in the gym) | New hard game, apprentice, `vending+short_sleeper`, `hard_income_mortgage.json`. Pass: plays to an ending screen (buyout or loss) without the keyboard; every fault auto-recovered or none |

**Minimum for a first watchable stream: M2 on normal.** Counter, staging, the window, dialogue
choices, services, sleep, night outings with scavenging, Wild's Network and the wine engine's
morning chores; `augs: serve`; no supplier phone, poster stash or water engine. In the gym this
subset wins 50 of 60 normal runs against 58 for the full executor (section 7): still a competent
shop with a legible day (a plan, a full counter, a night at the dump, a purchase). Shooting is
worth those 8 runs on normal and is mandatory on hard, so it leads M3. M1's subset alone (no outings, no
production) wins none of 60 normal runs: it survives but never reaches the mortgage, so a
stream on M1 needs the night and the morning chores played by hand on camera.

**M1 as built** (live side; tested with fakes and `cmd/fakemod`, not yet in the game):

- The planner (`Fence.Core/Autopilot/Planner.cs`) decides the next step from a `LiveView` read
  each tick (`LiveGame`); the driver turns it into an action with the executor's own rules
  (`PlayerRole.Window`, `StageForBuyer`, `StageFromSeller`, `Dialogue`, `Services`,
  `CounterFallback`, `PlayerPolicy.Pick`) and checks it took (`Expectation`: the item is where
  it was sent, the shutter moved, the line changed, the customer came or left, the next morning
  came; a miss is a fault). Item moves go through `ModHelper.MoveItemTo*` with `MayRemove()`.
- Counter moves go through `BarterExecutor` behind an autopilot `ActGate`, attributed to
  `actor: autopilot`. In `act` mode with `act_enabled` the bridge's model plays the counter and
  the autopilot plays its own pick only after a defer.
- (M1 planned the rent floors only and had no donation or exposable pull; M2 does both, below.)
- Base directives come from the mod setting `autopilot_directives`; the strategist's directives for
  the day (`/v1/summary` `directives`) merge over them when present. Not yet: the morning's
  `plan_wait_s` wait for the strategist's plan, and the pace set from `/v1/summary`.
- Settings: `autopilot` (off), `autopilot_pace_ms` (1200), `autopilot_hold_ms` (4000: the morning
  report and the landlord's, loan shark's, Georgio's and the permit official's questions). Key
  bind: Options → Mods → "Pause / resume autopilot", default F10 (session only).
- Stops (an `autopilot_fault` event, a red overlay banner, resume by hand): an exception, an
  action that did not take, 60 s without progress, an uncovered customer it cannot dismiss, a
  dialogue choice with no rule (the role's `dialogue:first_enabled`), a phone call, being outside,
  the run ending, services the cash cannot pay. A move or click of the player's own pauses it
  (`Recorder.ClaimHuman`), as do the overlay's button (which also pauses act mode) and the key.

**M2 as built** (live side; tested with fakes and the planner's unit tests, not yet in the game).
The driver plays the appraisal policy's whole day and night. Every row is pressed by `LiveHands`
behind the step's `ActGate`, re-read on the following ticks (or at once, for moves), and a miss is a
fault, never a retry (`ActSafetyTests` pins who may press what):

| Live action | Press | Class | Re-read |
|---|---|---|---|
| Morning plan | `ExecutorMorning.Plan` on the run stats' net worth and the session's sales | – | – |
| Engines' morning chores: furnace, hydroponic bays, wine (fill, berries, yeast, start, empty), vending (collect, power, upgrades), smuggler's bays | `Chore.Accept` → `hands.Accept` (`GraphUtils.TryAcceptAll`, `MayRemove`); `Chore.Act` → `hands.Chore` (the helper the machine's own handler calls) | a (moves), d (pours, closed windows) | the item is under its destination; an action cannot be re-read generically (the engine reads its own effect) |
| Supplier order by phone (`ore_phone`, `beer`) | `PhoneUIManager.OpenUI` (the phone's click), `AutoCall` (the phone book's quick dial, only when `WillAnswerCall`), `DialoguePhoneUIManager.SelectChoice` / `OnAdvancePressed`, `PhoneUIManager.CloseUI` | b | phone open, call answered, line changed, phone closed |
| Wild's Network by phone at night, or Joe's visit | the call's "browse" choice opens the window; `WildNetworkUI.OnSlotClicked` + `OnUseClicked` (private: `ModReflect`), `ModHelper.CloseWildNetwork` | b | the upgrade is unlocked; the window closed |
| Door, its confirm, the bag | `StoreDoor.OnClick` (its own gates; `NightOuting.DoorShut` first, a shut door skips the outing), `MapUIManager.OnGoOutsideConfirm`, the afterhours response's first choice (no backpack, checked by its label) | b | the choices, then the map, open |
| Travel | `MapUIManager.Travel` (the map's travel button's call) | b | outside at that location |
| Search the dump | `MapUIManager.ScavengeButtonClick` while searches are left and the wound odds are under the plan's limit (at most 10) | a | searches left drop or the bag changes |
| Loot home | `ModHelper.MoveItemToInventory`, most valuable first; room made with `MoveItemToAfterhours` (`NightOuting.RoomFor`) | a (S), d (outside) | in stock, or left on the ground |
| Dr. Jackson's | his goods to the back counter (`PlayerCounter.JacksonStage`), `BarterExecutor` buy-all, then the ground and his back counter into stock, case or back counter | a | cash changed; each move |
| Return, the prompt | `ModHelper.ReturnFromAfterhour(true)`; `GenericConfirmationUIManager.ConfirmButtonPressed` | b (S) | home |
| Vending restock at bedtime | `Vending.Evening` through `Chore` | a | as the chores |
| Appraisal board: join, promotions | `Appraisal.Dialogue` through the dialogue choice (M1's press) | a | the line changed |
| The kit, the certificate | the kit into the dossier between customers (`hands.Accept`); the certificate into the case by the window | a, d (the dossier's closed window) | under the dossier |
| Appraisal client | the magnifier's target call `InspectionUIManager.OnInspectItem`; cues read off the item the view draws (`Appraisal.Perceive`); `ChangeAppraisalServicePrice(delta)` (the panel's price buttons); `OnAppraisalServiceEstimated` (its submit); a refusal's reply read as the gym does; slips into the bin | b | the view opens, the field reads the estimate, the view closes |
| Permit official's donation | `PlayerCounter.BribePicks` to the back counter, the exposable pulled, `BarterExecutor` sell-all (+ its over-budget confirm) | a, d (moves while his line is on screen) | the amount owed changed |
| Exposable pull for a buyer | `PlayerCounter.Exposable` back to stock | a (flagged) | each move |

Over every plan, live forces `conceal.use_poster: false` and the labeler off
(`AutopilotDriver.LiveOnlyDirectives`): the poster stash is M3, and the labeler's gym path rewrites
conditions without its window. `autopilot_directives` defaults to the appraisal policy
(`hard_income_mortgage_appraisal.json`) with `augs: shoot` (M2 forced `augs: serve`).

**M3 as built: the gun** (live side tested with the planner's unit tests and the act-safety checks,
not yet run in the game; the owner's test is section 12).

*Detection, gym and live alike.* `AugCheck` in tells mode builds what a player sees of the customer
(`docs/re/AUGS.md` §2, §6), and the shared executor file `sim/gym/AugRule.cs` decides: shoot on a
zero-false-positive tell (perfectly still, twitching, an aug portrait, the aug script's lines, "Upper
Level Citizen" or "Thief" in a plain bubble) or the scanner's AUG DETECTED; serve otherwise. Nothing
it reads is hidden; every verdict carries the tells it rests on. The aug script's lines are
recognised by their displayed text, matched against the game's own lines read from its string table
at run time (`AugLines`); before M3 the mod derived that tell from `isAug`, which leaked the truth.
The gym is `-aug-detection tells` by default and logs the hidden truth only to score the detector
(the summary's `aug_eval`); `-aug-detection perfect` is the pre-M3 cheat, kept for analysis and
replaying main byte for byte (4 hard seeds checked). Headless, the gym draws two things the renderer
would show: the idle (`StoreClientMono.PlayIdleForCurrentClient`'s roll, on the gym's own random
stream so the game's is untouched) and whether the portrait came from the aug pool (new prefix hooks
on `AugHelper`, since `SpriteDict` is stubbed).

*Shooting live: class (b), built.* Every press is a handler the player's own click runs, and
everything between the presses is the game's own sequence:

| Live action | Press | Gate | Re-read |
|---|---|---|---|
| Verdict | none: `ReadAugVerdict` each tick on the customer at the counter; the step waits for a line to finish typing | the gun can be raised (`AugCheck.GunAvailable`: `Shotgun.CanPickup`'s inputs) | – |
| Raise the gun (`raise_gun`, after the hold `autopilot_hold_ms`) | `Shotgun.OnClick` (raises: `EnterShootingMode`, blackscreen, the shooting camera, the customer's reaction line) | `CanPickup` and the gun down, so the click can never lower it (lowering without a shot dismisses the customer) | shooting mode within 8 s, same customer |
| Fire (`shoot`) | `StoreUIManager.Shoot` (Arm's left click) | shooting mode, the crosshair up (`shootingUI`), `CanShoot`, the customer still the one judged | the shootout starts or shooting mode ends within 15 s |
| Security's verdict | `DialogUIManager.OnAdvancePressed` on each line while the shootout the autopilot started runs (the wound, body, walk-in and the verdict's end action are the game's) | the line waits for an advance | the line changed |
| The result (`shot_result`) | none | the shootout over and the customer gone | the crime record unchanged: logged with the cash change (a good kill's reward); a grown crime record (manslaughter) hands over |

Why not the fallback alone: the gym shows the detector flags every aug and no human (section 7), so
the risk is the scene, not the verdict, and the scene is two clicks through the game's own handlers
with the game playing everything between them. What could still go wrong is caught, never retried:
the gun not coming up, a shot that cannot fire, a gun raised by someone else (handover), a verdict
line that never advances (the watchdog's 60 s). The fallback is built too: the setting
`autopilot_gun: pause` stops on the verdict with **"AUTOPILOT PAUSED — your turn: possible aug (the
tells)"** and lets the human decide; a missing `autopilot.gun` member forces it. A plan's `augs: serve`
logs `aug_serve` and serves. Settings: `autopilot_gun` (`shoot`), `aug_detection` now defaults to
`tells` (the advice overlay; the autopilot uses tells whatever it says). The autopilot now needs
`advise.aug`: without the tells it could not see an aug.

## 6. Gates and measurements

**Parity gate (every change to a shared file).** Same seeds, same flags, before and after:
`smoke -seeds 60 -seed0 700000 -days 1000 -human player`, normal; and hard with
`-start apprentice -perks vending+short_sleeper -directives
sim/lab/scripts/directives/hard_income_mortgage.json`. Pass: every run's trajectory
(`s<seed>.jsonl`) is byte-identical. Other work relies on the gym being deterministic. M0
passed: 60/60 normal and 60/60 hard identical, and 20 + 20 again after rebasing onto ba64e4c. The bottle printer (moved behind
`IExecutorHost.MachineButton`) is not reached by these runs, nor by 8 water-merchant runs with
`engine: water, auto_print: true` (identical too; none owned a printer).

**Stream-1 subset arm.** The executor restricted to what the live hands can do at M1/M2,
paired against the full executor on the same seeds, so the stream's expected results are known
before it starts (results in section 7).

**M2 parity** (refactors 1–3, `867850c` against main `565f267`): 20 hard (2400001–020) and 20 normal
(2400201–220) seeds with the appraisal policy, and 10 hard with `hard_income_mortgage.json`
(2400401–410): every trajectory byte-identical. The outing fix then changes the gym (below); the
live-side commits after it replay the fix's trajectories byte for byte (8 hard + 8 normal).

## 7. Measurements

**M3: fair aug detection** (`-start apprentice -perks vending+short_sleeper -directives
sim/lab/scripts/directives/hard_income_mortgage_appraisal.json`, `-days 1000`):

| Arm (hard, best policy, 60 fresh seeds 2500001–060) | Wins | Median day | Endings | Augs shot / escaped | Detector precision / recall |
|---|---|---|---|---|---|
| perfect detection (`-aug-detection perfect`, the pre-M3 cheat) | 60/60 | 175 | buyout 60 | 481 / 0 | – (reads the truth) |
| **fair detector** (`tells`, the default) | **60/60** | **175** | buyout 60 | 481 / 0 | **1.000 / 1.000** (481 augs, 23 707 human visits) |
| fair detector without line recall (`-aug-lines off`) | 60/60 | 175 (+0.3 ± 0.6 paired) | buyout 60 | 449 / 44 (≤ 4 a run) | 1.000 / 0.911 (misses only undisguised augs) |
| serve all (`augs: serve`) | 0/60 | 47 | aug 60 | 0 / 360 | – |

The fair detector plays exactly like the cheat: net worth by day is identical in all 60 runs (every
aug shows a sure tell before the executor would act). It never flagged a human. By tell, over the
481 augs (several show more than one): the aug script's lines 272, a generic name in a plain bubble
178, an aug portrait 173, stillness 156, twitching 74; each fired on 0 humans. The shopping pattern
alone (medical, or materials) matched all 481 augs and 4 196 human visits, which is why it never
shoots. Without line recall the executor misses 9 % of augs (all undisguised, every tier) and still
wins every run: the kills keep presence far from 100. So the best fair policy is the incumbent
unchanged; no customer refusals, scanner or security service are needed. `perfect` replays main
`55b3c98` byte for byte (4 seeds checked).

**The return-prompt fix** (`df8a8f2`, section 3), appraisal policy (`-start apprentice -perks
vending+short_sleeper`), 1000-day horizon, paired on the same seeds:

| Arm | Wins before → after | Median win day | Paired Δ win day | Net worth d100, median before → after (paired Δ) |
|---|---|---|---|---|
| hard, 60 fresh seeds 2400001–060 | 59/60 → **60/60** (one arrest became a win) | 174 → 174 | −0.1 ± 0.8 | 21 436 → 20 480 (−761 ± 1 291) |
| normal, 60 seeds 2400201–260 | 60/60 → 60/60 | 62 → 62 | −0.2 ± 13.6 | 17 930 → 17 872 (−674 ± 3 348) |

The prompt opened in every run, first between day 6 and 19 (median day 12 hard, 9 normal), and
afterwards on 40 % (hard) and 46 % (normal) of nights: before the fix the outing stayed open from
that night on. Now 4 241 hard and 2 323 normal prompts are answered (items left on the ground) and
1 884 / 1 000 loot items made room by leaving cheaper stock. Net worth at day 150 on hard: −1 674 ±
1 303 (the loot the old gym carried in on later nights is now left). ± is 1.96 × SE of the paired
difference.


Gym, `-human player`, seeds 700001–700060, `-days 1000`; paired against the full executor on
the same seeds (mean ± standard error of days played). Directive files in
`sim/lab/scripts/directives/`.

| Arm | Wins | Median days | Days vs full | Endings |
|---|---|---|---|---|
| normal, full executor | 58/60 | 188 | – | rent 1, repo 1 |
| normal, stream-1 subset (`stream1.json`: `augs: serve`, no poster) | 50/60 | 226.5 | +34.2 ± 9.2 | rent 8, arrest 1, repo 1 |
| normal, M1 subset (`stream_m1.json`: also no outings, no engine) | 0/60 | 261.5 | +19.2 ± 11.8 | rent 39, repo 21 |
| hard, income set (`hard_income_mortgage.json`, `vending+short_sleeper`) | 1/60 | 212.5 | – | repo 32, rent 26, arrest 1 |
| hard, income set without the poster (`stream1_hard.json`) | 7/60 | 209 | +6.2 ± 4.5 | repo 30, rent 23 |
| hard, income set, `augs: serve` (`stream1_hard_serve.json`, 30 seeds) | 0/30 | 46 | −154.5 ± 5.4 | aug 30 |

The hard income set won 8/60 on seeds 970001–970060 (`docs/PROGRESS.md`); these seeds are
harder for it, and the poster's absence is within noise of it. Served augs end every hard run
by about day 46 (presence reaches 100).

`stream_m1.json` sets `jackson_every: 99999`, which still allows the day-6 visit: these runs
predate the fix that makes `0` mean "never" (`PlayerRole.Night` took `v.Day % every` before
checking `every > 0`).

## 8. M1 live test (the owner's script)

Do not run this on a save you cannot lose. Abort at any time with **Pause autopilot** in the
overlay or **F10**; to throw the day away, quit without sleeping (the game saves at sleep) or
restore the backup from step 1 (`~/.local/share/fence/save-backups/<time>/`).

1. `make backup-saves`.
2. `make test`, then `make mod-install`. Start the game; enable fence if asked (developer mode
   on, or approve the new build).
3. Mods → fence → gear: `autopilot` on, `autopilot_pace_ms` 800, `act_enabled` off; leave
   `autopilot_directives` at its default. Options → Mods: the key bind reads F10.
4. In a terminal: `make bridge ARGS="-mode observe"` (its startup log says whether the appraisal
   guides loaded: the bridge needs `local/appraisal_guides.json` to hand the autopilot the
   appraisal board's value tables). Optional, for the feed:
   `go run ./cmd/fenceshow -live-log var/log -bridge http://127.0.0.1:7341` and open the panel.
5. Load a **normal** save at a morning (morning report on screen, shutter closed, no customer).
   The overlay's first lines read "Autopilot paused: press Resume to start" and the button
   reads **Resume autopilot**.
6. Press **Resume autopilot**. Hands off mouse and keyboard.

What should happen, at about 1–2 s per step: the morning report stays ~4 s and closes; items move
into and out of the display case one by one (0.4 s each); the shutter opens; the bell rings; each
customer walks in, lines advance, choices are picked after a read; a buyer's items go to the back
counter one by one, then the counter move (sell all, a markup, a discount...); a seller's items
are staged, then bought or dismissed; leftovers go back to stock; the next customer. With the
queue empty the shutter closes, the end-of-day tablet opens, services are set, End Day. The
overlay's line shows each action ("Autopilot ▶ stage …: what … wants, within the budget").

**Pass:** the next morning arrives with no input (the autopilot then carries on into that day;
press Pause there). During the day, also check:

- **Pause mid-customer**: press **Pause autopilot** while a customer is at the counter. At most
  one more action happens; the line reads "Autopilot paused: paused by the player (overlay)".
  **Resume autopilot** continues. Repeat once with F10.
- **Hand move**: while it stages items for a buyer, drag one item yourself. It pauses with "you
  made a move (stage): the autopilot stopped so it cannot fight you". Put the item back where you
  want it and press Resume.
- **Handover** (if one comes): a story, security, info dealer, appraisal or barter customer that
  cannot be dismissed, or a dialogue choice it has no rule for, turns the line red: "AUTOPILOT
  PAUSED — your turn: …". Play that customer by hand, then Resume. Note who it was.

Afterwards, in the newest `var/log/*.jsonl` (the bridge's decision log):

```sh
L=$(ls -t var/log/*.jsonl | head -1)
# every action and its reason
jq -r 'select(.event.kind=="autopilot") | .event.data | "\(.day) \(.action) \(.target.name // "") — \(.why)"' "$L"
# stops; expect only the ones you caused or the handovers above
jq -c 'select(.event.kind=="autopilot_fault") | .event.data' "$L"
# moves not made by the autopilot; expect only your deliberate hand move(s), plus open/close_tablet
# (the counter tablet, which the game also opens and closes itself; they do not pause the autopilot)
jq -c 'select(.event.kind=="human_action" and .event.data.actor != "autopilot") | .event.data.action' "$L"
# trades and haggles the autopilot made
jq -c 'select(.type=="outcome") | .outcome.result | {kind, actor}' "$L"
```

Fail and send the log (and `Player.log`'s `[fence]` lines) if: a step hangs for more than a minute
without a red banner, something moves that should not (a hot item into the case, an item to the
wrong grid), the autopilot acts while paused, or the banner names an error.

**The (d) items, by hand** (autopilot paused, same session, at night after closing):

- **Loot home (#21).** Go out to the dump, scavenge once, return. Where is the loot: the
  afterhours bag in the shop, or already in stock? Quick-transfer one item from the bag: does it
  land in stock (the call the live hands would use, `MoveItemToInventory`)? Note any confirm panel
  on return (`CheckItemLeftOnGround`).
- **Pour amounts (#30).** With an empty wine bottle and a water ration (and a small ration if you
  have one): pour each into the bottle and note the bottle's ml and what is left of the ration.
  The gym assumes a ration fills 750 ml and a small one 375 ml (`Production.cs`); a full bottle is
  ≥ 749 ml.
- **Closed machine windows (#30–32).** Open a machine you own (wine rack, hydroponic bay,
  furnace, vending machine) and note what it shows; close it; sleep; next morning reopen it. The
  overnight changes the game made while it was closed (aging, growth, sales) should show. This
  is a proxy: the autopilot's own moves into a closed window are first tried at M2.

Write the results under "M1 live results" in `docs/PROGRESS.md`.

## 9. M2 live test (the owner's script): one unattended week on hard

Run section 8 first if it has not been run: M2 adds the night to everything M1 does. Same safety:
not on a save you cannot lose; abort with **Pause autopilot** or **F10**; to throw a day away, quit
without sleeping or restore the backup.

1. `make backup-saves`, `make test`, `make mod-install`; start the game.
2. **The save.** A hard game, apprentice start, perks vending + short sleeper, at a morning (the
   morning report on screen, shutter closed) **between day 3 and day 7**, so the week meets the
   appraisal board (day 8), a rent day, the dump most nights and Dr. Jackson's shop (day 6 or 20).
   Since M3 the autopilot shoots augs on visible tells (section 12 tests that on its own); for this
   week set `autopilot_gun` to `pause` if you would rather take each aug by hand.
3. Mods → fence → gear: `autopilot` on, `autopilot_pace_ms` 800, `act_enabled` off,
   `autopilot_directives`: reset it to its default (the cash-sprint policy); if it still holds M1's
   text, paste the policy:
   `{"augs":"shoot","bargain":"discount","bargain_buy":"ev_positive","bays":2,"buy":{"single_max_pct":75,"wholesale_max_pct":88},"conceal":{"policy":"always"},"engine":"wine+furnace","jackson_every":14,"permit":"buy","sell":{"liquidate_min_pct":78,"min_pct":57},"upgrades_cash_max":937,"window_mode":"concentrate","mortgage_margin":0.8,"appraisal":"join","buyout_by_day":140,"buyout_save":["insurance"],"buyout_save_from":56}`.
4. `make bridge ARGS="-mode observe"` (optional: `fenceshow` as in section 8). The bridge must have
   `local/appraisal_guides.json` (its startup log says "appraisal guides loaded"), or the autopilot
   does not join the appraisal board.
5. Load the save, press **Resume autopilot**, hands off for seven in-game days (about 30–45 min at
   pace 800). On day 4 of the seven, at a morning, quit to the menu (Escape → main menu), reload
   the save the game made that night, and press Resume once: it must carry on from that morning.

What a day looks like in the overlay's line: plan day → close morning report → chores (machines
tended; a phone call first if the furnace orders ore) → the window → shutter → customers (staging,
the counter, appraisal clients once the certificate is in the case: magnifier, the panel's price,
submit) → shutter closes → plan night → Wild by phone (call, browse, buy, close) → the door, the
bag, the map, the dump (searches, loot home, return; "leave on ground" when the loot did not fit)
or Dr. Jackson's (stage, buy, carry home, return) → vending restock → tablet → services → sleep.

**Pass:** seven mornings with zero inputs from you (other than the reload); no red banner except
the handovers listed in section 10 (note each: who, which day); rent paid on its day; the reload
resumes. In the newest `var/log/*.jsonl`:

```sh
L=$(ls -t var/log/*.jsonl | head -1)
# every action and its reason (as in section 8)
jq -r 'select(.event.kind=="autopilot") | .event.data | "\(.day) \(.phase) \(.action) \(.target.name // "") — \(.why)"' "$L"
# each phase present: expect plan_day, chores, plan_window, counter, plan_night, open_door, travel,
# scavenge, plan_loot, return, wild_plan, services, sleep; after day 8 inspect/estimate
jq -r 'select(.event.kind=="autopilot") | .event.data.action' "$L" | sort | uniq -c
# stops: expect none, or only section 10's handovers
jq -c 'select(.event.kind=="autopilot_fault") | .event.data' "$L"
# the engines' moves into machines, and anything left on the ground
jq -c 'select(.event.kind=="autopilot") | .event.data | select(.action=="chore_move" or .action=="chore" or .action=="leave_on_ground" or .action=="loot_room")' "$L"
# the appraisal estimates (accepted or refused)
jq -c 'select(.event.kind=="autopilot") | .event.data | select(.action=="estimate") | {day, why}' "$L"
# moves not made by the autopilot: expect only open/close_tablet
jq -c 'select(.event.kind=="human_action" and .event.data.actor != "autopilot") | .event.data.action' "$L"
```

**Check by eye** (the (d) rows of section 5): after a morning's chores, open each machine (wine
bottles, hydroponic bays, the furnace, the vending machine) and the dossier: what the feed says
went in is there. After a dump night, the loot is in stock (not still in the bag). After an
appraisal client, the slip is in the bin. Fail and send the log and `Player.log`'s `[fence]` lines
if a step hangs a minute without a red banner, an item lands somewhere the feed does not say, the
autopilot acts while paused, or an estimate goes in different from the feed's.

## 10. Remaining handover cases (live)

The autopilot stops with a red banner (a handover: play it by hand, then Resume) on:

- **Customers it cannot serve or dismiss**: story characters and others whose dialogue choices
  the executor has no rule for (`dialogue:first_enabled`), and uncovered customers (security,
  info dealers, barter) that are not dismissable. Dismissable ones are dismissed, as in the gym.
- **The gun** (M3): with `autopilot_gun: pause` (or the gun's members missing), every aug verdict:
  "possible aug (the tells)". With `shoot`: a gun raised by someone else, a gun that cannot be
  raised or fired, and Security's verdict of a manslaughter. An aug with no sure tell is served,
  as in the gym (none were missed there, section 7).
- **Windows it did not open**: a phone or call it did not dial, a choices window by day (a
  crate's scan), a confirm panel, a map or door menu with no outing planned, being outside
  somewhere the night's plan did not go.
- **Calls with an unexpected menu**: a supplier's call whose choices are not the order the
  engine chose (the big order needs 350 / 400 spent, as in the gym).
- **Appraisal**: an item the magnifier cannot open, the inspection view closing before the
  estimate is in.
- **Faults** (red banner, not a handover): any exception from a game call or a chore, a move or
  press that did not take, 60 s without progress, 8 counter moves for one customer, services the
  cash cannot pay.

Not handled live, and off or absent in the appraisal policy: the poster stash (off), the
labeler (off), the water engine's strip, relabel and bottle printer (gym shortcuts; `engine:
water` is not live-safe), the machine-module levers (`sim/gym/Modules.cs`: module fits are moves as in row 32,
the transmitter's faction a `Chore.Act`; never run live), `inspect: injectors`, storage bays, the strategist's `plan_wait_s` wait.
The live ledger starts from the game's run stats: net worth by day is the calendar's, but the sales
behind the forward rent floor are only the days the autopilot played (fewer days: a slightly
higher floor).

## 11. Hidden values the live rules still read (fairness)

Each is marked in the feed (`hidden_used`, names only) where the step records it; the goal is none
(M3). Still read live:

| Rule | Hidden value | Where | Marked |
|---|---|---|---|
| Staging for a buyer (`PlayerRole.KeepWhy`) | whether the buyer could expose an item's feature (`CanClientExposeAnyFeatureDeep`); a water container's true purity against its label | `PlanStageBuyer` | yes (`item_exposable`, `water_purity`) |
| The exposable pull (`PlayerCounter.Exposable`) | the same exposure | `PullExposable`, `Donate` | yes (`item_exposable`) |
| The permit donation's pool (`PlayerCounter.BribePicks`) | the official's exposure | `PlanBribe` | yes (`item_exposable`) |
| The window (`PlayerRole.Window`: `HidesFake`) | an item's true condition | `PlanWindow` | yes (`item_true_condition`) |
| Vending restock (`Vending.Eligible`: `HidesFake`) | an item's true condition | chores | **no**: chore lines carry no `hidden_used` |
| The wine and water engines' water picks (`WaterHelper.GetWaterPurity`) | a container's purity before a test strip | chores | **no** |
| Making room for loot (`NightOuting.RoomFor` via `KeepWhy`) | a water container's true purity | `loot_room` | **no** (liquid containers are never left, so it changes nothing) |

No longer read live: aug detection (M3: the verdict reads only visible tells, section 5; the gym's
idle roll and portrait hook stand in for the renderer, not for a player's knowledge),
the appraisal value (the estimate comes from the cues the inspection view draws and the guides'
tables; the true value is never read live, and the gym logs it as hidden only).

## 12. M3 live test (the owner's script): an aug at the counter

Run after section 9 (or with it). Same safety as sections 8 and 9: not on a save you cannot lose;
abort with **Pause autopilot** or **F10**. The first two augs are scripted: one undisguised aug on
the day after the mentor brings the shotgun (day 13 on a normal track) and one five days later (day
18), and both always show a tell (`docs/re/AUGS.md` §1.2, §1.5).

1. `make backup-saves`, `make test`, `make mod-install`; start the game.
2. **The save.** A hard game (apprentice, vending + short sleeper) at the morning of **day 13**, or
   at any morning on which the night log reported an aug 6 days earlier (the next one comes 6–8 days
   after the last). Keep a copy: the test is run twice.
3. Mods → fence → gear: `autopilot` on, `autopilot_pace_ms` 800, `act_enabled` off,
   `autopilot_directives` at its default (it reads `"augs":"shoot"`; an old text with
   `"augs":"serve"` must be reset), `aug_detection` `tells`, **`autopilot_gun` `pause`**.
4. `make bridge ARGS="-mode observe"`; load the save; press **Resume autopilot**; hands off.
5. **Run 1, the fallback.** When the aug reaches the counter (and its lines, if any, have played) the
   overlay turns red: **"AUTOPILOT PAUSED — your turn: possible aug (…)"** with the tells it saw. Check
   by eye that the tells are on screen (a still or twitching customer, an odd portrait, stiff lines,
   "Upper Level Citizen"/"Thief" in a plain bubble). Shoot it yourself (hold-click the gun, click the
   customer), let Security finish, press Resume. Note the day and the tells.
6. **Run 2, the gun.** Reload the copy, set `autopilot_gun` to `shoot`, Resume, hands off. Expected,
   at pace: the verdict holds ~4 s (`raise_gun` with its tells in the overlay), the blackscreen and
   the shooting view come up with the customer's reaction, the shot, the body, Security walks in and
   its lines advance one by one, the slip lands on the counter, the next customer is called.
   **Pass:** no red banner; the reward is in cash; the day carries on. Let it play to the next morning.
7. If anything stops it (the gun did not come up, the shot did not fire, a line never advanced), note
   the banner and what the screen shows, finish by hand, and send the log.

In the newest `var/log/*.jsonl`:

```sh
L=$(ls -t var/log/*.jsonl | head -1)
# the verdict, the shot and Security's result, with their reasons (tells in words)
jq -r 'select(.event.kind=="autopilot") | .event.data | select(.action|test("aug|gun|shoot|shot")) | "\(.day) \(.action) \(.client // "") — \(.why)"' "$L"
# the game's own record of the shot: expect good_kill, never wrongful_kill
jq -c 'select(.event.kind=="aug") | .event.data | {event, name, cash_delta}' "$L"
# the gun moves, attributed to the autopilot in run 2
jq -c 'select(.event.kind=="human_action" and (.event.data.action|test("holster|shoot"))) | .event.data | {action, actor}' "$L"
# stops: run 1 one handover ("possible aug"), run 2 none
jq -c 'select(.event.kind=="autopilot_fault") | .event.data' "$L"
```

Fail and send the log and `Player.log`'s `[fence]` lines if: the autopilot raises the gun at a
customer showing none of the tells it names, the gun comes down without a shot (the customer leaves),
a shot is fired at a human, or a step hangs a minute without a red banner. Write the results under
"M3 live results" in `docs/PROGRESS.md`.

## Show layer

See `docs/SHOW.md`: the panel, the narrator and TTS, and the feed schema (`cmd/fenceshow`,
`internal/show`). What it consumes from the autopilot: the `autopilot` events in the decision
log (section 4, "Recording"), the morning plan from `/v1/summary`, and the pause and fault
states (`autopilot_fault`), which should become a feed `kind` of their own so the panel can say
"paused, needs a human".
