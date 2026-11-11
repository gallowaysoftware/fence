# fence mod (C#)

The in-game half of fence: a mod for the game's official loader (`Mods/fence/`) that forwards
`ModHook` events to the bridge on `127.0.0.1:7341`, asks it for barter advice (P1) and, only when
the bridge is in `act` mode **and** the `act_enabled` setting is on, plays the advised move (P2). It
declares one capability, `network`, and follows the official modding guide
(github.com/dragonpan2/probably-stolen-modding): netstandard2.1, game DLLs referenced and
never shipped, `ModHelper.StartCoroutine` instead of threads, `ModHelper.GetConfig` for
settings, `ModLog` for logging.

| Project | What |
|---|---|
| `src/Fence.Core` | netstandard2.0, no Unity: protocol DTOs, `BridgeSession` (hello/event/decide state, bounded event queue, circuit breaker, deadlines), `ChoiceValidator`, the hook manifest and `CompatChecker`; `Barter/` builds the v1 barter context, gates the options like the game's UI, tracks advice per customer and shapes outcomes, all against the `IBarterGame` interface. Embeds `compat/hooks.json` and `compat/versions/*.json`. |
| `src/Fence.Mod` | netstandard2.1, the `IMod`. `HookBindings` (one method per event), `RuntimeResolver` (reflection on the loaded game for `hello.compat`), `BridgeClient` (UnityWebRequest coroutines), `GameBarter` (`IBarterGame` over the live game), `BarterAdvisor` (4 Hz poll, decide, act, outcomes), `BarterExecutor` (the only class that presses game buttons, behind an `ActGate`), `FenceOverlay` (the `fence.overlay` CustomUI window), `FencePatches` (observation-only Harmony patches, id `mod.fence`, each applied on its own). |
| `src/Fence.Autopilot` | netstandard2.1, not shipped yet: the gym executor's decision files (`sim/gym`, listed in the project) compiled under the mod's rules, so the live autopilot can share them (`docs/STREAM.md`). A scanner test fails on any capability they would need. |
| `tools/Fence.Compat` | net10 console, Mono.Cecil: checks a hook manifest against the installed game and writes the build snapshot. |
| `tests/Fence.Tests` | xUnit: fixtures, breaker/deadline/session, compat checker, manifest, and the game's own `ModScanner` run against the built DLLs. |

The game must be installed: the build compiles against `GAME_DIR` (default
`~/.local/share/Steam/steamapps/common/Probably Stolen`; override with `GAME_DIR=…` or
`-p:GameDir=…`).

## Commands (run from the repo root)

```sh
dotnet build mod/Fence.sln -c Release
dotnet test  mod/Fence.sln -c Release
mod/install.sh                                   # build + copy manifest.xml and the DLLs into $GAME_DIR/Mods/fence
dotnet run --project mod/tools/Fence.Compat -c Release                      # check compat/hooks.json, write the snapshot
dotnet run --project mod/tools/Fence.Compat -c Release -- --check-only --hooks compat/hooks.proposed.json
```

`Fence.Compat` writes `compat/versions/<buildid>.json` and `compat/surface/<buildid>.txt`
and exits 1 when a `required` hook is missing. After it records a new build, rebuild the mod:
the snapshot is embedded so the running mod can recognise the build by Assembly-CSharp's
MVID and report its sha256 in `hello` (it cannot hash the DLL itself without `files`).

## First run in the game

1. `mod/install.sh`, then start the game. In **Mods**, activate *fence* and accept the
   "Access the internet" prompt. Restart (enabling takes effect on the next launch).
2. For development, turn on **Developer mode** in fence's detail pane, so a rebuild loads
   without a new approval. Without it, every rebuild changes the hashes and asks again.
3. Settings: gear button on fence in the Mods menu (`enabled`, `bridge_url`, `act_enabled` and `autopilot` — both off
   by default — `autopilot_pace_ms`, `autopilot_hold_ms`, `autopilot_directives`). They live in
   `…/LocalLow/Questing Goose Studio/Probably Stolen/ModConfigs/fence.json`.
4. Logs: lines prefixed `[fence]:` in `Player.log` and the F8 console.

## The overlay (P1/P2)

A draggable **fence** window on the store screen (left side) shows: mode (advise / act / act paused),
bridge status and model; **this customer** (who, status — waiting for the counter, thinking…, advice,
stale, deferred to you, acting…, not covered — the advised or executed move with the model's reason,
every option with the model's note, and the result); **today** (each customer → outcome, the bridge's
day summary and current lessons). Buttons: *Ask again*, *Hide* (collapse to one line), and in act
mode *Pause acting* / *Resume acting* (the kill switch). Hovering a counter item still adds one
`fence suggests: …` / `fence will: …` / `fence did: …` line. Customers the mod does not cover
(wanted, security, story, barter/appraisal/info dealers) get no advice.

## Acting (P2)

Off unless the bridge says `act` **and** `act_enabled` is on; otherwise exactly the advise behaviour.
Then, for covered customers, fence executes the chosen `sell_all` (confirming a capped over-budget
sale), `buy_all`, `accept_wholesale`, a bargain step, blackmail, threaten or `dismiss`. It never
stages items (you put your items on the back counter), appraises, reports or shoots. Before acting
it re-reads the counter: a different `state_signature` (you clicked first), a closed UI gate or an
option no longer offered drops the action as stale. At most 4 actions per customer and 12 per
minute, with a 1.5 s settle after each. Any exception turns acting off for the session. Outcomes
carry `result.actor` (`mod` / `human`). `make backup-saves` first.

## Autopilot (docs/STREAM.md, M1)

Off unless the `autopilot` setting is on, and then paused until you press **Resume autopilot** in the
overlay (or the key bind, default F10, Options → Mods). Running, it plays the day by the gym executor's
rules (`sim/gym/PlayerRole.cs`, compiled into Fence.Mod): closes the morning report, arranges the display
case, opens the shutter, rings for each customer, clicks through dialogue and picks choices it has a rule
for, stages items, plays the counter (`PlayerPolicy`, or the bridge's model in `act` mode), unstages
leftovers, dismisses uncovered customers it may, closes, sets tonight's services and sleeps. No outings,
phone, machines or gun yet (M2/M3). One action per tick, only when the game is still, `autopilot_pace_ms`
apart (moves at half that); every action is re-read and logged as an `autopilot` event with its reason.

It stops when you press Pause or the key, when you move or click anything yourself, on any error or
action that did not take, after 60 s without progress, and on a customer it cannot handle (a story,
security, info dealer, appraisal or barter customer it cannot dismiss, a dialogue choice it has no rule
for): the overlay shows a red banner, an `autopilot_fault` event goes to the bridge, and you play on by
hand until you press Resume. In-game presses outside the counter live in `LiveHands` only
(`ActSafetyTests`). `make backup-saves` first.

## Recording (phase D)

Always on, whatever the bridge mode (`Recorder`, `GameDay`, `FencePatches.Record`; rules in `src/Fence.Core/Recording/`):
`day_snapshot` at run start, load, morning (`OnWakingUpEarly`, before the night log is cleared), shutter open, close and
sleep (`OnGoingSleepEarly`); one `customer_observed` per customer; `human_action` for every counter and night move (moves
made by the game's own item shuffles, by a loading save or by `BarterExecutor` are filtered out); `money` from the
`RunStats` funnels; `inspection`, `run_start`, `run_end`, `run_stat`. Values the player cannot see go to the event's
`hidden`. While the bridge is down, events wait in a 4096-event buffer (recording kinds may grow it to 16384; context
events are dropped first) and flush in order. The overlay's first line says `recording`, or `NOT RECORDING — why` when the
bridge has been unreachable for more than 10 s, refused us, or an event was lost. A missing game member turns off only its
`record.*` feature (`hello.compat.disabled_features`).

## Checking the scanner in game

`dotnet test` runs the game's own `ModScanner.Scan` against the installed layout, failing on
hard flags, undeclared capabilities and the "pending" findings the December 10, 2026 update
refuses. To see the same thing in the game: create an empty `dev.txt` next to
`Probably Stolen.exe`, restart, press F8 and run `mod-scan fence`. Expect
`declared network`, `detected network`, no hard flags, no `DEPRECATED` line.
