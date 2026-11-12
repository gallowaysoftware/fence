# Start here (state at 2026-11-12, before a reboot)

Read this first in a new session, then docs/CAMPAIGN.md and docs/research/STRATEGY-SPACE.md.

## Where things stand

- **Goal 1 is met: the Gauntlet.** Every playable start wins on hard on its native path, measured on the
  fair-play build (docs/CAMPAIGN.md, docs/research/ROUTES.md): `appraiser`, `farmer`, `armourer`,
  `water_baron`, `rancher`/`rancher_sprinter`, `fence` 60/60; `rock_bottom` 57/60; `aquafarmer` 60/60
  (by the mortgage, ~day 196); `cook` 57/60 (meets the cartel quota until the exit, then the Board).
- **Fair play is in.** The executor reads only what a player could see or measure (`sim/gym/Known.cs`;
  the guard test `sim/gym.tests/FairPlayTests.cs` fails on new hidden reads). docs/research/FAIR-PLAY.md.
- **Goal 2, in progress: faster and funnier wins.** Engines built from the decompiled source, all off
  by default: chemistry with a standing alcohol bath and solvent recovery (`Lab.cs`), one power system
  (`Power.cs`), plant gene lines incl. caffeinated fillerweed (`Breeding.cs`), the frame forge (`Arms.cs`),
  fish with eels and sturgeon (`Fish.cs`), staged clean water (`Cellar.cs`), the stolen-goods trade
  (`Stolen.cs`). They work in the lab but mostly win *later* in the gym: the bound is capital, kit and
  purchase order, which the build planner (below) attacks.
- **The live game has not been played by the autopilot yet.** That is the next owner step.

## Unmerged branches (git worktrees beside this checkout)

Each was asked to commit its work with a "state at reboot" note. For each: read the note, `git merge main`
into it (local main, never origin: origin/main is a squashed history), run the tests, then merge.

| Branch | What | Doc |
|---|---|---|
| `build-order` | `build: {...}`, off by default (byte-identical off). Rescues engine stacks (farmer + weed line 37/60 → 58–60/60; sprinter + weed 43 → 56/60) but no engine yet beats its route. Blocker #1: the **stock grid** (engine kit leaves 0–8 free cells from ~day 50, the counter's buying falls 8 900 → ~4 500); then sellers' part waits and the Board routes' short horizon. Unfinished: `var/sim/exp/bo7` (storage bay first — the next measurement), `bog` (all engines on the Gauntlet routes, 16/30 seeds), gym tests incl. `BuildPlannerWaitsForGeorgioThenBuysWholeKit`, `make test-mod`, PROGRESS entry, the `RESULT_STORAGE` placeholder in the doc | docs/research/BUILD-ORDER.md ("WIP / state at reboot") |
| `cheap-wins` | Levers measured on 60 seeds, all routes still 60/60: `permit: bribe` −11 days, Georgio hold −8.5, Board blacklist −7.6, wanted reports −4.7, rent skip −1.5, no insurance under Alert −6.4; combined (wanted + block + rent skip) `appraiser` median 98 → 84, `outlaw` 105 → 91. Defaults on: wanted, blacklist, rent skip, Georgio hold, Alert insurance off; routes still `permit: buy`. Unfinished: the final set `arms-cheap5.txt` → `var/sim/exp/cheap-final` after the Gauntlet merge, full gym tests with defaults on, ROUTES.md (Alert row, medians, bribe recommendation), PROGRESS entry. The live mod doesn't report posters or run the fridge yet | docs/research/HIDDEN-SYSTEMS.md (WIP note) |
| `fish-power` | WIP 45c5f87, **not merge-ready**: `make test-mod` fails 2/500 until compat/hooks.json lists the new members (MachineFeedDispenser.GetPelletKcal, AquacultureHelper.GetPelletShare, GameItem.FindAdjacentType, GameItem.unitCount, GameInventory.TryInventorySlot, SlotMarker.numTransfer/TryAcceptOnce, GridShape/GridShapeBuilder); nothing measured yet. Built: eel power hub (lab-verified adjacency; first eel arrives ~day 64, evening placement moved nothing — debug), pellets ≤ 24 %, young rat cuts, the alarm overvolt fix. Q-200 furnace judged out of reach (learning modules too rare). Next: hooks, hub debug, run `arms-fishpower-base.txt` and `arms-fishpower.txt` | docs/re/AQUACULTURE.md (state-at-reboot note at the end) |

Tests: `make test-go`, `make test-mod`, `dotnet test sim/gym.tests -c Release`. Build tip: `dotnet restore
--source ~/.nuget/packages`, then `--no-restore --disable-build-servers`.

## The first live test (watch-only, not broadcast)

docs/STREAM.md §8 (M1) and docs/STREAM-OPS.md §4 and §12. Saves were backed up before the reboot
(`make backup-saves` again anyway). Then:

```sh
make mod-install                      # the installed mod must match this checkout (pre-flight checks it)
docker start fence-show-kokoro        # CPU TTS on :8881
make gate                             # drift gate on :7380 (keep running)
systemctl --user stop fence-bridge.service
make bridge ARGS="-mode observe -planner llm -directives delta -route appraiser -persona magnate \
  -provider openai -base-url http://127.0.0.1:7380/plan/v1 -model $DRIFT_MODEL -no-schema -thinking off"
make show-live ARGS="-persona magnate -route appraiser -fill-after 40s -tts-url http://127.0.0.1:8881"
make stream-preflight                 # GO expected, with OBS and panel-waiting warnings
```

In the game: Mods → fence → `autopilot` on, `autopilot_pace_ms` 800, `act_enabled` off,
`autopilot_directives` = the `appraiser` route as one line (`jq -c 'del(._route)'
sim/lab/scripts/directives/routes/appraiser.json`). New game: hard, Apprentice, perks vending +
short sleeper. At the first morning press **Resume autopilot**. Panel: `http://127.0.0.1:7350/`.
F10 pauses. Afterwards restart `fence-bridge.service` and read the newest `var/log/*.jsonl`.

## Working rules (the owner's)

- PUBLIC repo: never commit game binaries, decompiled code, extracted game data or text, transcripts,
  raw recordings, screenshots, hostnames/IPs or keys. Commit explicit paths only. Publish only as a
  squashed `public-main` (see git history of this doc); never push local main's history.
- Derive play from the source's exact formulas like an optimising player; never call something
  worthless without the condition under which that holds (docs/research/CLAIMS-AUDIT.md).
- On stream the 5090 runs only the game and the encoder; drift (27B) is player and narrator, through the gate.
- CPU: the owner set a cap of 32 gym processes; the idle runner (`sim/search/idle_runner.py`) is paused
  until the owner restarts it. Never `pgrep -f` for the game: use `pgrep -x 'Probably Stolen'`.
- The decompiled source (not in the repo) is kept at `~/.cache/fence-decomp/26525552`; link it as `var/decomp`.
