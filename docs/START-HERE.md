# Start here (state at 2026-11-16, after the perk survey, the overlay story and the chemist show)

Read this first in a new session, then `docs/PROGRESS.md` (durable record), `docs/CAMPAIGN.md`,
and `docs/re/PATHS.md` + `docs/re/CHEMISTRY.md` for the game mechanics.

## Where things stand

- **Local `main`** carries this session's three threads on top of `fbe5c67` (below). Suites:
  **Go** green, **mod 546/546**, **gym 102/102**, **compat** 935/935 hooks resolve.
- **The installed mod is older than `main`**: `Fence.Mod.dll f9875cb3…` (the chem cam build)
  lacks the overlay's day story. `make mod-install` (game stopped) before the next live game.
  Saves backed up to `~/.local/share/fence/save-backups/20261116-121840` (26 saves).
- **The game is stopped.** The last run ended day 15 (the user stopped it; no game-over — the
  day-14 handler was played by hand because the mod then lacked the yes-click, which is now in).
- **Bridge is up** on :7341 (`-mode advise -planner llm -policy llm -route cook -persona cook
  -directives delta -forks -provider openai -base-url http://127.0.0.1:7380/plan/v1 -model
  qwen38-27b-mlx -no-schema -thinking off`, log `/tmp/bridge_live.log`); the MLX server on
  :7380 is healthy. If the port is taken, check `pgrep -fa bridge` and kill by pid before
  restarting with the same flags. **Restart it before the next game**: it loaded `cook.json`
  before the card's perks changed.
- A stale `fenceshow` from an earlier water-baron session (`-live-log var/log/s-a8271e28…`) holds
  the panel port :7350; stop it before `make show-live`.
- **Screen recording is live** for broadcast review: `/tmp/fence_shots/watch.sh` captures the
  game window (1920×1108 at global 4015,198 on HDMI-A-1) every 20s while the game runs, newest
  150 frames in `/tmp/fence_shots/frames/`. `/tmp/fence_shots/see.sh` OCRs the latest frame
  (the session's vision model is not routed — OCR is the eyes). `/tmp/fence_shots/eval.sh`
  appends a state digest; `report.sh` a 5-min tally. **If the game window moves or resizes,
  re-query the geometry via KWin scripting** (the query that produced `{"x":4015,"y":198,...}`
  landed in `journalctl --user`; see the script in this doc's history) — the crop is hardcoded.

## What this session added

- **Perks per persona** (`docs/re/PERKS.md` §4.1): every route card but `no_vending` took `vending +
  short_sleeper`. The executor replayed each persona's route with 3–4 loadouts from the fit
  column (30 seeds), the leaders confirmed on 60 fresh seeds. Five cards changed: cook
  `short_sleeper + intimidating` (−10±6 days to the buyout), smelter `short_sleeper +
  proficient_scavenger + lucky` (−4±3), gunsmith `model_citizen + well_connected` (−6±4),
  aquafarmer `+ unlucky + wild_favorite` (nw200 +5.5k), vintner `+ bad_lease + wild_favorite`
  (nw60 +1.1k). The rest keep the old pair. Their ghost bands were rebuilt on gym `fbe5c67`
  (the other routes' bands are from older builds). Arms: `sim/lab/scripts/arms-perks-*.txt`.
- **Overlay "Today" as a story** (`Fence.Core/Barter/DayStory.cs`): one sentence per customer
  ("Junkie refused a 25% markup and walked out."), browsers folded, the autopilot's shop beats
  between them (opened, presses on the lab's machines, calls, Wild buys, the outing, services),
  and the summary in prose. Fed by `BarterAdvisor.NoteAutopilot` from the driver's `Record`.
- **The show on a chemist run** (`internal/show/lab.go`, docs/SHOW.md): a replay of the day 1–15
  cook log showed no lab at all, the cartel's count as a ticker line, "Bought 8× item" and the
  board join told twice. Now: a `lab` beat per run of presses, a `cartel_quota` moment from the
  handler's own lines (priority with milestones), a notable check-in naming the next count,
  "8 items", one board beat. Replayed through the gate's narrator on :7352.

## What the chemist campaign added (most recent first, all on main)

- **Chem cam** (`1d61fb2`): lab chores now play on camera. `LiveCam.Focus(machine)` jumps the
  camera to the machine item's world rect (`GameItemElement.handler.rectTransform.position`)
  and opens its `contentWindow` (`PixelWindow.Show()`); `ChoreHands.Accept`/`Act` focus on
  machine moves and presses; `RunChore` restores the counter camera in `finally`. The evening
  lab chore opens the chem_scanner's analyser first so the vessel panel reads during the batch.
  Motivation: the frames proved the whole press chore was off-screen.
- **Cartel self-click** (`69a3ca2`): the weekly handler's rule stages the quota and clicks
  (`player:cartel_yes`) — before, it fell to the no-rule default and handed over, the exact
  "asks something the executor has no rule for" the user hit on day 8 and day 14. Checkin/sec
  dialogues already returned picks.
- **Overlay in spectator language** (`69a3ca2`): "Autopilot ▶ ring bell" → "letting the next
  customer in"; "not covered: intent DIALOGUE" → "here to talk, not to trade"; the header
  dropped the model name. Tests updated (`ActTests`).
- **Satchel fix** (`10508be`): `GetAllOwnedItems` never visits the afterhours pocket slots, so
  the equipped satchel was invisible to the pack budget — the carry home took two pockets and
  expelled the rest at the dump. `PlanLoot` reads the equipped bag from
  `afterhourPocketSlotInvBackpack` (mod-side; the gym's own walk is untouched — a first attempt
  in the shared `NightOuting.OwnedBackpacks` broke `BagKeepTests` semantics and was reverted in
  `929c300`/`c8ca14c`). `LiveGame.Ground()` also appends the bag's grid contents.
- **Dialogue retry** (`90bbca4`): choices that enable late (the Corporate Waste Manager's
  barrels land via a `SetEndAction` after the dialogue opens) are retried twice at 2s before
  handing over; `noEnableRetries` resets on success.
- **Cartel mechanics, decomp-verified** (`aa51ad5`, `docs/PROGRESS.md`): handler visits day
  %7==1, check-in day %7==4; yes takes ALL counter dream_dust; zero dust → gunshot →
  `cartel_retired`; purity scam when below `cartelRequiredPurity` (default 40, +5/week);
  `QuotaMorning` presses blanks to `QuotaPurity + 10`. Waste left on counters respawns as
  owned at shutter-open (`StoreShutterButton.cs:186`); dumping waste triggers one Georgio visit.
- **League records** committed under `league/`: sweep-stow 152/152 and 304/304, w1/w2 water,
  w3 trade-floor (rejected: 40.5/57 vs fountain 40/52), w5 chemist probe. `var/league/local.json`
  points at `gym-c8ca14c`.
- Earlier in the campaign (still true): graduated `day.Floor` save floor wins, TradeFloor
  retired; drift > qwen on the same build; the chemical analyser in three layers (gym
  `Lab.Status`, mod snapshot `r.Data["lab"]`, `internal/strategy` render); `paths.json`
  compositions; `ProtectsLabTool` + `lab_input` hold.

## Runbook (the cook route, current default)

```sh
pgrep -fa bridge                      # if :7341 is held, kill those pids
go run ./cmd/bridge -mode advise -planner llm -policy llm -route cook -persona cook \
  -directives delta -forks -provider openai -base-url http://127.0.0.1:7380/plan/v1 \
  -model qwen38-27b-mlx -no-schema -thinking off > /tmp/bridge_live.log 2>&1 &
make mod-install                      # NEVER while the game runs
make backup-saves
```

In the game: new game, **hard, chemist (DRUG_DEALER), perks short_sleeper + intimidating** (the
cook card's loadout since the perk survey; `docs/re/PERKS.md` §4.1). The
bridge serves the cook route automatically (`routes/cook.json` carries `lab: {products:
[dream_dust], sell: false, cartel: {}}`) — the in-game `autopilot_directives` paste is
belt-and-suspenders. Autopilot on; F10 pauses; the overlay has Pause/Ask-again/Hide.

Expected beats now in place: day-1 beans/mushrooms held (lab_input), scanner bought by the
build order and protected, day-8 handler staged and self-clicked (watch for
`player:cartel_yes` in the log), presses on camera with the analyser open, satchel packed on
outings (only true overflow left), waste dialogues retried then clicked.

## Known rough edges

- The camera crop for frames is hardcoded to the window rect (see above).
- `LiveCam` assumes the camera rig doesn't re-assert position every frame; if jumps don't
  take, drive the rig's follow target instead (untested — worst case is status quo).
- The post-game-over BarterAdvisor NRE (harmless, once) is still unguarded.
- `league/sweep-stow.json` churns runner state (`executor.human`); `git checkout` it, don't
  commit the churn.
- Gym league builds live in `~/.cache/fence-league/gym-<sha>`; rebuild with
  `var/league/bin/fenceleague gym -ref HEAD` after pulling engine changes.

## Working rules (the owner's — unchanged)

- PUBLIC repo: never commit game binaries, decompiled code, extracted game data or text,
  transcripts, raw recordings, screenshots, hostnames/IPs or keys. Commit explicit paths only.
  Never push local main (origin is a squashed `public-main`; never push without being asked).
- Derive play from the source's exact formulas like an optimising player; never call something
  worthless without the condition under which that holds (`docs/research/CLAIMS-AUDIT.md`).
- CPU: at most 32 gym processes; 16 while the user games. Never `pgrep -f` for the game
  (`pgrep -f "Probably Stolen.exe"` is the one safe pattern; the process shows as
  `S:\steamapps\...\Probably Stolen.exe` under Proton).
- Never install the mod while the game runs. `make backup-saves` before a live session.
- Decompiled source (not in the repo): `~/.cache/fence-decomp/26525552`, link as `var/decomp`.
- The user plays live on this machine (5090); the model serves their game — don't hog it.
- Tests: `cd mod && dotnet test -c Release` (522); `dotnet test sim/gym.tests -c Release
  --no-restore` (102, ~2.5 min); `make compat` (936); `go test ./...`.

## First steps for the new owner

1. Read `docs/PROGRESS.md` (the durable record) and `docs/re/CHEMISTRY.md`.
2. Verify state: `sha256sum` the installed DLL against HEAD's build; `lsof -i :7341`; check
   the frame watcher.
3. The obvious next session: launch the game and watch a full week with the chem cam live —
   day-8 handler self-click, presses on camera, analyser open, satchel packed. Frames land in
   `/tmp/fence_shots/frames/`; review with `/tmp/fence_shots/see.sh`.
4. Open threads: the stream's `make show-live` on a live cook game (the panel and narrator were
   only replayed); a perk survey under the LLM (the gym measured the executor; personas only
   differ there — appraiser and sprinter share directives, so the gym cannot tell them apart);
   the challenge-difficulty cards (`routes/challenge/`) still take the old pair, untested.
