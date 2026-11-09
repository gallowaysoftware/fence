# sim/gym — the real game as an environment

`fence-gym` runs the installed game's own `Assembly-CSharp` headless (`sim/headless`, shared with
`sim/extract`) and plays the live mod's side of `docs/PROTOCOL.md` against it. Every bridge policy
(heuristic, LLM, strategist, memory) plays the real rules unchanged; the gym only stands in for
the human and the Unity engine. Personal research use; nothing under `var/` is ever committed.

```
make gym-build                      # also builds var/gamedata/.headless-gym and loot_tables.json
make gym-run ARGS="-seed 3 -days 30 -v"
make gym-bridge                     # a sim bridge on 127.0.0.1:7399 (var/sim/log, var/sim/memory.db)
make gym-run ARGS="-seed 3 -bridge http://127.0.0.1:7399"
make gym-smoke                      # 20 seeds x 30 days, child processes in parallel
make gym-bench                      # in-process, then HTTP against a throwaway heuristic bridge
make gym-test
```

`run` flags: `-seed`, `-days`, `-difficulty cozy|normal|hard|challenge`, `-start none|fence|scav|…`
(`NewGameData.StartType`), `-skip-intro`, `-human default|prudent`, `-bridge URL` (else the
in-process heuristic), `-out FILE` (default `var/sim/runs/<run>.jsonl`), `-no-trajectory`,
`-wire FILE` (every request as sent), `-parity` (check option gates against the game), `-repeat N`
(N episodes in one process, each in a fresh `AssemblyLoadContext`), `-v`.

The bridge must run in `act` mode: in `observe` it answers every decide with `defer`, and the gym
then falls back to the human role for every customer.

## How it plays

- **Episode** (`Episode.cs`): new game from `NewGameData` (difficulty, start, seed), then per day
  morning → window → open → each customer → close → night → sleep, each a `Step()`. Ends at the
  requested day count or at `PlayerStore.ExecuteGameOver` (`rent`, `arrest`, `buyout`, …).
- **Same reads, same moves as the mod**: `GameBarter`, `GameDay` and `BarterExecutor` are compiled
  from `mod/src/Fence.Mod` into the gym; contexts, option ids, labels, signatures, outcomes and
  snapshots come from `Fence.Core`. Outcomes carry `actor` (`mod` for executed choices, `human`
  for the gym's defaults).
- **Determinism**: `UnityEngine.Random` is a seeded, state-restorable copy of .NET's seeded
  generator; coroutines (`StartCoroutine`, `WaitForSeconds`, `MetaHelper.WaitAndDo`) and
  `UnityEngine.Time` run on a virtual clock that advances only when the gym pumps. Same seed,
  policy and options give a byte-identical trajectory, in a fresh process or a fresh load context.
- **Recording**: events flow to the bridge like the mod's (the bridge records sim sessions in its
  own log dir); the gym also writes its own trajectory in the `fencedata export` shape, with
  synthetic timestamps.

## Marking sim data

`hello.mod_version` is `<Fence.Core version>+sim` (SemVer build metadata), which the bridge logs
and `fencedata` copies into every day's `sessions[].mod_version`. The run parameters go out as the
first event, `sim_run` (`backend`, `seed`, `difficulty`, `start`, `skip_intro`, `human_role`,
`days`). Proposed protocol change: an optional `hello.sim` object with those fields (schema,
`internal/protocol.HelloRequest`, `fencedata.SessionInfo`), so consumers can filter without
parsing versions; until then use the suffix.

## Human-role defaults (`HumanRole.cs`)

Each default taken is counted in the run summary and recorded as a `human_action` with
`sim_default: <key>`.

| Decision | Default (`-human default`) |
|---|---|
| Window, before opening | Most valuable owned item of each of the 11 display types; never stolen or contraband (`window`) |
| Buying customer | Stage owned items `IsClientBuyingThisItem` accepts, most valuable first, within budget, at most 6; never stolen/contraband to security (`stage_for_buyer`) |
| Selling customer | Move their items priced at or under the tablet's estimate to the back counter (`stage_from_seller`) |
| After each customer | Unsold staged items go back to stock (`unstage_leftovers`) |
| Bridge defers or fails | `dismiss` if offered (`counter_fallback`) |
| Customers outside coverage (story, security, info dealer, barter, …) | Their dialogue plays; then dismiss, or force-dismiss when not dismissable (`customer:*`) |
| Dialogue choices | Buyout if offered; else pay rent; else don't pay; loan shark: decline; shady inspector: bribe only when holding serious contraband; otherwise the first enabled choice (`dialogue:*`) |
| Night | Visit the dumping ground and scavenge until out of time, bring the loot to stock; services unchanged (the tablet's own rules: mandatory on, cooling-down off, drop the dearest optional ones if unaffordable); no upgrades (`night:*`) |

`-human prudent` is the same plus insurance and janitorial service whenever cash covers a week of
each on top of the rent. The default bleeds stock to nightly burglaries and halves attractiveness
with garbage (only the janitorial service clears it, `PlayerStore.HandleJanitorial`); in the
benchmark seed prudent survives 60 days where the default goes broke on day 42.

## What the headless host fakes (could change economic outcomes)

1. **Item footprints are 1×1** (`GameItem.BuildShapeFromSprite`): stock, display case, counters and
   containers hold far more than in the game; a seller's stock never overflows the front counter.
2. **No per-second item callbacks** (`EmporiumEntry.Update` → `onOneSecondItemFunc`): fridge,
   heating plate, chem finisher, auto-butcher, gene scanner, chemical temperature never tick. In the
   game their yield depends on how long the player spends per day.
3. **No `Update`/tween logic**: walk-in/out tweens are replaced by calling `StoreClientMono.OnArrived`'s
   logic directly; anything else driven by a presentation `Update` or a DOTween callback is absent.
4. **Presentation managers stubbed** except the logic the gym un-stubs (dialogue flow, bargain
   panel, tablet trade buttons, shutter, arrival, map outings, blackscreen sequences). Lost with them:
   Wild Network window (no upgrade purchases; Joe's introduction is force-dismissed), phone orders to
   suppliers, appraisal/gunsmith/info-dealer/barter tablet modes, inspection confront panel.
5. **Typewriter and click timing**: a dialogue line counts as shown on the next pump; the game's own
   delays still order end actions. A line nothing advances (Dr. Jackson's return) is clicked through.
6. **RNG stream**: the game's draws come from the gym's seeded generator, not Unity's; client sprite
   variety (`ClientSpriteVariety.Pick`) never draws.
7. **Saves stubbed**: `InitialSave` does not run, so `runID` is `sim-s<seed>` and `runNumber`/the
   total run count stay 0 (content gated on run count behaves as a first run).
8. **Player settings** fixed by the gym: auto-advance dialogue on, auto-call-next off, move bought
   items to inventory on.
9. **Localization returns keys**: item, client and dialogue text are loc keys (prompts see
   `name_scavenger`, `dialog_…`), and `LocHelper.HasEntry` is always false.
10. **Loot tables** come from the installed game's `level1` scene (`var/gamedata/loot_tables.json`,
    read with AssetsTools.NET and UABEA's `classdata.tpk`, fetched once into `var/tools/`); without
    them every table roll spawned a blank item. `sim/extract` does not load them yet, so its
    seller-stock samples still contain those blanks.
11. **Item factories**: `marking_pen` still fails headless (rancher start).
12. **The human's half** (above) is a simple default, not play.

## Next: night phase and production policies

- New decide kinds for the night (`night_services`, `upgrade_buy`, `outing`), sent by the gym first
  (it already has `NightView`) and then by the mod, with option sets built in `Fence.Core` so both
  sides share them. The Wild Network window needs its purchase path (`WildNetworkUI` buy →
  `RunStats.OnWildUpgradeBought`) un-stubbed or replicated; upgrade state is `NetworkUpgrade.state`
  (0 not bought, 1 bought consumable ready, 2 unlocked).
- Staging, window and dialogue choices as decide kinds, so a policy (not the gym) owns them; the
  dialogue view (`DialogueView`) is already the context.
- Production starts need item time: drive `onOneSecondItemFunc` from the virtual clock at a chosen
  seconds-per-customer rate, and the machine/crafting actions as moves.
- Footprints from the real sprites (atlas sizes) to restore inventory pressure.
