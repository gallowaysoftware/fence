# Stream ops: the go-live runbook

How to put a run of Probably Stolen on a live stream with an LLM in charge: which machine runs
what, the pre-flight, the OBS scene, rotating models and personas between runs, the controls
during a stream, recovery, and the data export afterwards. The concept and the autopilot are
`docs/STREAM.md`; the panel, the feed and the narrator are `docs/SHOW.md`.

Everything here can be rehearsed without the game: `make stream-preflight`, a replay through
fenceshow (`scripts/show-demo.sh`), and `go test ./cmd/fakemod -run TestLiveShowEndToEnd` (fakemod
playing the mod against a real bridge and strategist, fenceshow's live hook, the hybrid slate
and the narrator, read back from the panel's endpoints).

## 1. Machines and models

Three places can run a model. None is named here by address; endpoints live in `local.mk`,
`~/.config/fence/bridge.env` and the shell environment, never in the repo.

| Node | Hardware | Serves | Notes |
|---|---|---|---|
| **The game box** | Linux, RTX 5090 (32 607 MiB), Proton | the game, the mod, the bridge (`:7341`), fenceshow (`:7350`), Kokoro, OBS | llama-swap on `:9000` serves the 27B the leagues use |
| **drift** | MacBook, MTPLX | `qwen38fn-mlx` (Qwen3.8 Flash) | may be offline; one request at a time: a second caller gets HTTP 503 while it generates |
| **The API** | Anthropic | the narrator (`claude-haiku-5-5`), optionally the strategist | only when `ANTHROPIC_API_KEY` is set in the environment |

**Roles.** The *strategist* writes each morning's plan (the bridge, `-planner llm -directives
delta`); the *hands* are the executor inside the mod (no model); the *narrator* is fenceshow's
(one short line per moment); *TTS* is Kokoro (piper as fallback). Two strategists are proven on
the stream's policy: in the t7 hard league both `qwen27-delta` (the 27B on the 5090) and
`drift-delta` (drift) bought the shop on day 175 in 3 of 3 games, the same as the executor alone
(`var/league/t7-hard-*/REPORT.md`). Plan latency p50: drift 3.8 s, the 27B 7.3 s; a live day takes
3–6 minutes, so either keeps up.

**GPU budget on the 5090.** Measured 2026-11-10 with the leagues' 27B loaded: 29 427 of 32 607
MiB used, 3 180 MiB free. GPU Kokoro needs 2–3 GB (`docs/SHOW.md`, "TTS setup"), and the game
itself needs VRAM too (unmeasured: read `nvidia-smi` while it runs on the first stream). So:

| Strategist this run | TTS | Narrator LLM | Why |
|---|---|---|---|
| the 27B on the 5090 | **CPU Kokoro** (`fence-show-kokoro`, `:8881`) | the API, else drift | the 27B, the game and GPU Kokoro do not fit together |
| drift or the API | **GPU Kokoro** (`vibe start tts_kokoro`, `:8880`) | the API, else drift | the 5090 holds only the game and Kokoro |

CPU Kokoro took 6 s for a short line idle and 12 s under load (pre-flight, 2026-11-10); the live
narrator drops a line that is ready later than 25 s after its moment, so on the CPU it says
less. GPU Kokoro is well under a second.

**The narrator's LLM.** Prefer the API: it never competes with the game or the strategist. drift
works but refuses a second request while it plans (HTTP 503; the narrator then speaks the
caption, and fact-checking means it never says a wrong number either way). Never point the
narrator at llama-swap by a model name that is not loaded: it would load a second model next to
the strategist. fenceshow's `-yield-to-game` now applies only to a narrator served on this machine
(an OpenAI-compatible URL on loopback); the API and drift keep speaking in character while the
game runs.

**Leagues on stream night.** Stop them (`max_gyms_while_gaming` only throttles gyms; a league's
local endpoint yields to the game and unloads its model, which then reloads on the next plan).
The hybrid planner's rollouts are CPU-heavy: with CPU Kokoro, keep rollouts to a few threads or
run the hybrid on another run.

**Fallbacks** (each automatic; the panel says so):

| Lost | What the stream does |
|---|---|
| the strategist | no new plan reaches `/v1/summary`; the autopilot plays on the directives it has: the base policy (`autopilot_directives`) and the last plan the bridge still serves. The plan box keeps the standing plan |
| the narrator LLM | the caption is the line (`say.source: caption`) |
| Kokoro | piper (`~/.local/bin/piper`), then captions only |
| the bridge | the mod defers every decision and buffers its events; the autopilot plays on the base directives; fenceshow follows the new session when the bridge is back |
| fenceshow | OBS's browser source goes blank; restart it with the same flags (section 6) |

## 2. Pre-flight

Run it on the game box, game closed, about 30 minutes before going live:

```sh
make stream-preflight ARGS="-strategist-url $DRIFT_URL -strategist-model qwen38fn-mlx -persona shark"
```

Endpoints come from flags, the environment (`FENCE_BASE_URL`, `FENCE_MODEL`, `FENCE_SHOW_LLM_URL`,
`FENCE_SHOW_LLM_MODEL`, `FENCE_SHOW_TTS_URL`, `ANTHROPIC_API_KEY`) or the bridge service's
`bridge.env`. It prints one row per check and a verdict, and exits 1 on any NO-GO. No row prints an
address, so the table can be shown on stream.

| Check | GO when | On NO-GO / WARN |
|---|---|---|
| game build | the Steam build id has a snapshot in `compat/versions/` | the game patched: section 7, "Game patch" |
| compat hooks | `Fence.Compat --check-only` resolves every required hook (what `make compat` checks, without writing) | a hook is gone: section 7 |
| mod installed | `Mods/fence/Fence.Mod.dll` matches this checkout's Release build | `make mod-install` |
| save backup | a backup newer than the newest save exists | `make backup-saves`, or `ARGS=-backup` |
| bridge | `/v1/health` answers | start it (`systemctl --user start fence-bridge` or `make bridge ARGS=…`) |
| strategist | the endpoint lists the model (never a completion: that could load a model) | section 7, "Model endpoint down" |
| narrator LLM | one short test line within 8 s | WARN: captions only |
| TTS test line | Kokoro speaks "Pre-flight check. The narrator is on the air." in the persona's voice, saved to `var/show/preflight.wav`; the persona's file exists | WARN when slower than 3 s a line (CPU) |
| fenceshow panel | `/state`, `/`, `/column`, `/lower`, `/about` answer; WARN without `-setup` lines | start fenceshow (section 4) |
| panel soundcheck | with `-soundcheck`: fenceshow's own narrator speaks a test moment with audio | SKIP by default: the moment shows on the panel, so only before going live |
| OBS scene | a scene collection has a game capture and a browser source on the panel with "Control audio via OBS" | WARN; check by hand on the streaming machine |
| GPU memory | at least 3 000 MiB free | WARN: CPU Kokoro |
| game process | – | WARN when the game runs and the narrator is local: it would speak captions only |
| disk | 4 000 MiB free under `var/` | NO-GO |

Then by hand, in this order:

1. `ARGS="-soundcheck"` once fenceshow and OBS are up: the browser source's meter must move in
   the OBS mixer, and the line must be heard on the stream's monitor output.
2. Start the game; Mods → fence: `autopilot` on, `autopilot_pace_ms` 800–1200, `act_enabled` off;
   the overlay's key bind reads F10. Load the run's save at a morning.
3. OBS: a 30-second local recording of the full scene; play it back (game picture inside its
   hole, the panel's column and lower third, the narrator audible over the game).
4. Panel: the name card shows this run's model and persona; the about card shows the setup lines.

## 3. OBS scene

Canvas 1920×1080, 60 fps. `docs/SHOW.md` ("OBS setup") has the panel's geometry.

**Scene "Live"** (bottom to top):

1. *Game*: PipeWire window capture of Probably Stolen (Proton). Transform: position 0,0, size
   1360×765. Capture the window, not the screen, so notifications never reach the stream.
2. *Panel*: Browser source `http://127.0.0.1:7350/`, 1920×1080, custom CSS empty, **Control audio
   via OBS** ticked, "Shutdown source when not visible" unticked, "Refresh browser when scene
   becomes active" unticked. Or two sources: `/column` (560×1080 at 1360,0) and `/lower` (1360×315
   at 0,765), audio on `/column` only (`/lower` is silent by default).
3. *Mic*: muted by default; push-to-talk for the human at a handover.

**Scene "Break"**: the panel `/about` (560×1080 or scaled up) beside a still of the shop and the
runs board (`/column`). Use it for a pause, a crash or a model swap.

**Audio routing.**

| Source | Into the stream | Notes |
|---|---|---|
| Panel browser source | yes | the narrator. Compressor (ratio 4:1, threshold −18 dB); this is the only source that plays the voice |
| Game | yes, −8 to −12 dB under the voice | Application Audio Capture (PipeWire) of the game, or the desktop audio with nothing else playing. A compressor with **sidechain = the panel source** ducks the game while the narrator speaks |
| Mic | push-to-talk | the human at a handover |
| Desktop audio | off | it would double the narrator (the browser plays it once, through OBS) |

The about card rotates into the column for 22 s at each run start and every 8 minutes
(`?about=SECONDS` on the panel URL; `?about=0` turns the rotation off). `/about` shows the card
alone for its own source.

## 4. Starting a run, and rotation

One run is one model and one persona from a save to an ending; a hard run on the `sprinter` route
(the cash sprint policy, `hard_income_cash_sprint.json`) buys the shop around day 98, at 3–6 minutes a
day (`docs/STREAM.md` estimates 3–4, its M2 test 30–45 minutes a week at pace 800): 5 to 10 hours, so it
spans several streams. The game saves every
night and the autopilot resumes from a save.

For each run:

```sh
# the bridge: the run's strategist, route and persona (the route's directives are the base policy,
# the cash sprint; the persona shapes the morning plans)
make bridge ARGS="-mode observe -planner llm -directives delta -route sprinter -persona shark \
  -provider openai -base-url $STRATEGIST_URL -model $STRATEGIST_MODEL"
# the panel and narrator: the same model, route and persona on the name card, the rig on the about card
go run ./cmd/fenceshow -live-log var/log -bridge http://127.0.0.1:7341 \
  -player qwen27 -model qwen3.8-27b -persona shark -route sprinter -fill-after 40s \
  -record var/show/stream-$(date +%F).jsonl -tts-url http://127.0.0.1:8881 \
  -setup "Strategist: Qwen 27B on the stream PC, delta plans over the sprinter route;Hands: rule-based executor in the mod;Narrator: Claude Haiku, fact-checked;Voice: Kokoro"
```

- **Name each player.** `-player` is the scoreboard's player column and half of a run's id
  (`<player>:<run_id>`); give each model its own (`qwen27`, `drift`, `haiku`) so the board keeps
  them apart.
- **Rotation.** Same game, same executor, different strategist: rotate the model and the persona
  per run, not mid-run. Mirror runs are the cleanest contest: back up one hard save at a morning
  between day 3 and 7 (`make backup-saves`), play it with model A; restore the same save for
  model B. A suggested order: the 27B as Shark, drift as Magnate, the API as Honest Broker.
- **The scoreboard carries over.** fenceshow keeps it in `var/show/board.json` and writes it at
  each run's end; a run in progress shows as "playing" and is redrawn from its next `run_start`.
  `-board-seed 'var/league/t7-hard-*/*'` adds the league's finished games for context. Do not
  delete `board.json` between streams; to start a season clean, move it aside.
- **Swapping a model mid-run** (an endpoint died): restart the bridge with the new model and
  restart fenceshow with the new `-model`; the run keeps its board row, and the about card should
  say so (`-setup`).

## 5. During the stream

**Pause and resume.** The overlay's **Pause autopilot** button or **F10** stops the autopilot and
the counter within one action; **Resume autopilot** continues. Any move of your own while it runs
pauses it ("you made a move… the autopilot stopped so it cannot fight you"). The panel shows
"Autopilot paused" (no narration) and the narrator stays quiet about it.

**Handover etiquette.** When the autopilot cannot go on (`docs/STREAM.md` section 10: a customer it
cannot serve or dismiss, a dialogue with no rule, a window it did not open, a fault), the overlay
turns red, the panel's lower third says **"Over to the human: <reason>"** and the narrator says
so in character. Then:

1. Unmute, say who you are and what you are about to do ("the autopilot cannot appraise this one;
   I'll play this customer, then hand it back").
2. Play **only** what the banner names: that customer, that dialogue, that window. Everything you
   do is logged as the human's (`actor: human`) and kept apart from the AI's in the export; doing
   more muddies both the run and the data.
3. Do not undo the autopilot's choices you disagree with. That is the show.
4. Press **Resume autopilot**, mute. The panel says "Back on autopilot: the AI has the shop again"
   and the narrator picks it up.
5. Note the handover (day, who, why) in the stream notes for the run's write-up.

**The gun.** Since M3 the autopilot raises the gun and shoots on a visible-tell verdict
(`docs/STREAM.md`, "M3 as built"), but it has not yet run in the game: pass the owner's aug test
(`docs/STREAM.md` section 12) off-stream first. Until then, a hard stream serves augs, whose
presence ends a run around day 46. A shot that grows the crime record hands over to the human. The
panel shows the gun raised as a notable line and the shot as the `aug_confronted` beat.

**Pacing.** `autopilot_pace_ms` 800 is the M2 test's pace (a week in 30–45 minutes); 1200, the
default, gives a slower, more legible counter. The narrator speaks at most every 8 s plus its line, says nothing older than
15 s, and drops a line ready later than 25 s after its moment.

**What the narrator plays up** (`beat` in the feed): joining the appraisal board and each
promotion, the mortgage signed, its payments (the first, every fourth, the last two), the buyout, an aug
confronted, a handover and the hand-back. Its mood follows the money: swagger when flush or near
the buyout, tense when the next payment is barely covered, sweating when it is not. Every number
it says is checked against the game first.

## 6. Recovery

| Failure | Do this | Viewers see |
|---|---|---|
| **Game crash** | Restart the game, load the newest save (the morning of the day in progress: the game saves at sleep), press Resume. The bridge keeps running; fenceshow follows the new session's log by itself | a new session; the panel redraws; the day in progress replays from its morning (say so) |
| **Bridge crash** | The service restarts it in 2 s (`systemctl --user status fence-bridge`); a manual bridge: rerun the same command. The mod re-hellos on the first 404 | decisions defer for a few seconds; nothing else |
| **fenceshow crash** | Rerun the same command (same `-record` file: it appends). On attach it reads the log silently and publishes the latest plan, heat and day; the browser source reconnects by itself | a blank panel for a few seconds |
| **OBS crash** | Restart OBS; the browser source draws the current state from `/state` at once | the stream drops; restart it |
| **Game patch mid-week** | See below | – |
| **Model endpoint down** | See below | – |
| **Run ends** (buyout, eviction, arrest) | Let the narrator land it; switch to "Break"; the board row is written. Start the next run (section 4) | the ending and the board |

**Game patch mid-week.** Prevention first: in Steam, the game's Properties → Updates → "Only
update this game when I launch it", and during a stream week launch it with Steam in offline mode.
If it patched anyway, the pre-flight's "game build" row is NO-GO:

1. `make compat` (Fence.Compat against the installed game). Exit 0: every required hook resolves;
   it writes `compat/versions/<build>.json` and `compat/surface/<build>.txt`. Then `make test`,
   `make mod-install` (the mod embeds `compat/versions/` to recognise verified builds), commit
   the snapshot, rerun the pre-flight. Play a day with the autopilot off-stream before going live.
2. Exit 1: a required hook is missing. The mod still loads and disables only the features that
   need it (`hello.compat`); if any `autopilot.*` feature is among them, there is no autopilot
   stream this week. Fix the hook (`docs/COMPAT.md`), or stream a **replay** instead: a finished
   league game through fenceshow (`scripts/show-demo.sh MATCH 1-175 1`, section 8), labelled as a
   replay on the about card.
3. Never stream a build whose snapshot was not written by `make compat` on this machine.

**Model endpoint down.**

- *Strategist*: no new plan; the autopilot plays on the directives it has (the base policy and
  the last plan the bridge still serves). For a short outage, carry on. For a long one, restart the bridge on
  another strategist (section 4, "Swapping a model mid-run") at the next morning, and say so.
- *Narrator LLM*: nothing to do; captions are spoken. Switch fenceshow to the API at the next
  break if it lasts.
- *Kokoro*: `docker start fence-show-kokoro` (CPU) or `vibe start tts_kokoro` (GPU); piper speaks
  meanwhile if its voice is installed.
- *drift's HTTP 503* ("background task bypassed while foreground generation…"): drift is busy with
  another request. If drift is the strategist, do not give it the narrator too.

## 7. After the stream

1. Quit to the menu after a sleep (the save is the night's) and `make backup-saves`.
2. Stop fenceshow with Ctrl-C (the recording is complete line by line; the board is written at a
   run's end only).
3. `make data-status` (runs, sessions, days, gaps) and `make data-export` (one trajectory file per
   run under `var/export/`, human and autopilot play kept apart by `actor`).
4. Keep, per stream, under `var/stream/<date>/` (gitignored with `var/`): the feed
   (`var/show/stream-<date>.jsonl`), the narration WAVs (`var/show/audio/`), the bridge's decision
   logs of the sessions (`var/log/s-*.jsonl`), `var/show/board.json`, the OBS recording, and the
   stream notes (handovers).
5. The transcript and fact-check rate of the narrator, from the feed:

   ```sh
   jq -r 'select(.kind=="say") | "day \(.day) [\(.say.source) \(.say.check // "") \(.say.mood // "")] \(.say.line)"' var/show/stream-$(date +%F).jsonl
   jq -s '[.[] | select(.kind=="say") | .say.check] | group_by(.) | map({(.[0] // "caption"): length}) | add' var/show/stream-$(date +%F).jsonl
   ```

6. Hidden reads: `jq -c 'select(.hidden_used) | {day, text, hidden_used}'` on the feed lists every
   ◆ entry (the about card's count); the goal is none (`docs/STREAM.md` section 11).
7. Write the run's results (ending, day, handovers, faults) under a dated entry in
   `docs/PROGRESS.md`.

## 8. Rehearsal without the game

- `make stream-preflight` on the real rig (section 2).
- `go test ./cmd/fakemod -run TestLiveShowEndToEnd`: the live path end to end.
- A dress rehearsal (section 10): `scripts/rehearsal.sh` runs the bridge in its live configuration,
  fenceshow's live hook with the narrator and TTS, and one gym game slowed to a stream's pace by
  `cmd/pacer`, until the game ends.
- A full replay at stream speed with narration (CPU Kokoro shown; stop it afterwards):

  ```sh
  docker start fence-show-kokoro
  FENCE_SHOW_OUT=var/show/demo-full FENCE_SHOW_TTS_URL=http://127.0.0.1:8881 \
  FENCE_SHOW_LLM_URL=$DRIFT_URL FENCE_SHOW_LLM_MODEL=qwen38fn-mlx FENCE_SHOW_SHOT_EVERY=60 \
    scripts/show-demo.sh var/league/t7-hard-drift/drift-delta__apprentice__s2100001__p-short_sleeper+vending 1-175 1
  docker stop fence-show-kokoro
  ```

## 9. Watchability: the full replay (2026-11-10)

One complete winning hard game, `t7-hard-drift/drift-delta__apprentice__s2100001` (qwen38fn-mlx
planning delta directives over the appraisal policy; buyout by mortgage on day 175), replayed
through fenceshow at speed 1, narration-paced, narrator qwen38fn-mlx on drift, CPU Kokoro
(`bm_george`, the Magnate). Under `var/show/demo-full/`: `feed.jsonl`, `transcript.txt`,
`narration.wav`, 124 panel screenshots (`shots/full-N.png`, one a minute; `column.png`,
`lower.png`, `about.png`), and `after/` (days 60–75 again with the fixes below).

| Measure | Full game | Days 60–75 after the fixes |
|---|---|---|
| Wall time | 125 min for 175 days, 43 s a day | – |
| Lines | 151, 0.86 a day | 19 |
| Voice on air | 15 % of the time | 19 % |
| Silence between lines | median 32 s, p90 75 s, max 157 s; 29 gaps over 60 s | median 31 s, p90 51 s, max 84 s |
| Fact check | 143 first try, 6 on the retry, 2 captions | 19 first try |
| LLM per line | median 4.9 s, p90 12 s, max 22 s | median 5.2 s |
| Moods | flush 125, steady 20, tight 2, broke 2, triumphant 1 | flush 12, steady 5, broke 1 |
| Beats spoken | every one: join, 3 promotions, 8 augs, the mortgage, 16 payments, the buyout | – |

**What works.** The arc is legible from the panel alone: the appraisal board (joined day 8,
Appraiser day 16, Senior day 23, Master day 37), the permit, Georgio, the mortgage on day 63, the
countdown of payments and the buyout. The beats got lines worth hearing ("The board promotes me
today. A senior title for a senior portfolio."; "The nurse had a chip where her conscience should
be. I removed it."), and the finale lands ("Twenty-four thousand in the drawer and no rent to pay.").
The fact check held: no wrong number reached the air; its six retries were all invented figures.

**Pacing and dead air.** The voice is on air 15 % of the time and a line comes about every
45 s at replay speed. Live, a day takes 3–6 minutes and has about one moment, so the same rules
would leave several minutes between lines. Fixed: after `-fill-after` (60 s) of silence a notable
event is spoken too. The replay's own pace (43 s a day, plan moments held 7 s) is right for a VOD
or a "previously on" segment; the long run of mortgage weeks (days 70–175: payments, sales,
day closes) is the slow middle of the run and the place to cut or speed up (`-speed 2`).

**Repetition.** The worst problem. The mood rule made the shop "flush" from day 31 (cash six small
hard-game rents deep while a fifth of the way to the buyout), and 125 of 151 lines boasted: "a
rounding error" 14 times, "in the vault" 14, "thousand in the …" 23, "I pay the mortgage with a
yawn" three times; 16 mortgage payments sounded alike. Fixed: flush needs 30 % of the buyout
price; the mood colours a line without being its subject; six of the sixteen payments are spoken
(the first, every fourth, the last two); trade and day moments scale with net worth (late-game
moments fell from 57 day closes and 36 trades to 39 and 16 before the replay). Still open: the
Magnate's vocabulary ("ledger" 37 times, "portfolio" 23) needs a per-persona list of words to
rest, or a larger narrator model.

**Fact-check gaps found.** "A quiet Tuesday" (a weekday the facts never give: now rejected);
"two thousand dollar account" (credits: now in the prompt); "I pay the final installment" with
two weeks left (a deed, not a number: unchecked); "leaving five coins for the rent" (numbers
under 13 pass unchecked); a negative cash estimate between snapshots after a 7,004 payback to
Georgio made one line say "seven thousand gone" in a broke mood on a 13,338-cash day (now no
fact and no mood). Plan moments whose caption is a raw directive ("Clear Vending Crime: Remove
mislabeled items (Target: Evidence < 6)") are the two caption fallbacks: dull when spoken.

**The panel.** The screenshots showed the ticker's lines overlapping once the column filled,
plan lines with the commas of stripped item ids ("Water items (,,)"), "planner: plan" and "1 weeks
left": all fixed. The game hole is the demo stripe in a replay; live, the game fills it. The ◆
count on the about card counts only the gun in a gym replay (perfect aug detection); the gym's
other hidden reads are not marked in league logs, so a replay understates them.

**Recommendations before the first live stream.** Narrator on the API (faster, no 503s, a larger
vocabulary than a flash model); GPU Kokoro whenever the strategist is not on the 5090; keep
`-fill-after` at 60 s and watch the first hour for chatter; a short "previously on" replay of the
last stream's beats (`-days` window at `-speed 2`) to open each stream of a multi-stream run.

## 10. Dress rehearsal (2026-11-11)

The whole live stack, run continuously for 3 h 10 min against a gym game instead of the real one:
the bridge in its live configuration (`-mode observe -planner llm -directives delta -route sprinter
-persona shark`, memory and reflection on, plus `-planner-wait 100s` because the gym, unlike the mod,
reads the plan once each morning), the strategist Qwen 27B on the 5090 (thinking off), fenceshow
`-live-log` against the bridge's log dir and `/v1/summary` with the narrator on the same 27B
(one request at a time) and CPU Kokoro (`fence-show-kokoro`, `am_fenrir`). One hard game, seed
2950001, apprentice, `vending+short_sleeper`, the `sprinter` route as it stood before 6afbda0 (no
`buyout_save`), paced by `cmd/pacer`: 3 s before each customer, 0.1 s before each counter action, open
hours held to at least 40 s. With the morning plan wait, a game day took 1 min 57 s on average.
`scripts/rehearsal.sh` repeats it. Kept under `var/rehearsal/run1/` (not committed): the feed, 157
narration WAVs, the bridge and fenceshow logs, `monitor.tsv` (a sample a minute), 64 panel
screenshots (`shots/`, one every 3 minutes, and `full-final`, `column`, `lower`, `about` at the end),
and the pre-flights before and after (`var/rehearsal/preflight-*.txt`).

**Timeline.**

| Time | What happened |
|---|---|
| 02:33 | Cold pre-flight: NO-GO as expected (no bridge or panel yet, saves not backed up). Two rows wrong: "narrator LLM answered nothing" and "Probably Stolen.exe is running" (it was not) — both fixed below |
| 02:35 | False start: the pacer timed the day from waking, and the 50 s morning plan ate the whole budget. Paced from the shutter instead; restarted clean |
| 02:38 | Run start. Narrator: "Day one, just me and the dust. Time to bleed these marks dry." |
| 02:40 | Panel shows yesterday for the whole morning plan wait (fixed) |
| 02:51–04:32 | fenceshow restarted 7 times mid-run to deploy fixes (the runbook's recovery drill): panel back within 1 s each time; the first restarts greeted "Day one" on day 9 and restarted the feed's numbering (fixed) |
| 03:20 | `-fill-after` 60 s → 40 s from day 23 |
| 03:21–05:45 | Plans slow to 100–120 s: the 27B's 8 slots were full with two leagues. 7 days lost the directives step, 5 lost the plan (the panel kept the standing plan; the day played on the last plan) |
| 05:48 | Cash buyout on day 98, net worth 30,181. Narrator: "Deed is signed, ink is dry. No more rent, no more begging, just me and the counter." Board row written |

**Health.** No crash, stall or leak in 3 h 10 min. Bridge RSS 25–34 MB, pacer 14 MB, fenceshow 20–27 MB
(5 MB after each restart), file descriptors flat, the gym 0.13 → 0.58 GB (its own state). Kokoro
(CPU) 1.8–2.5 GB, no failed line. Growth for 98 days: decision log 19 MB, the bridge's memory DB
31 MB, feed 0.9 MB, audio 36 MB, logs < 0.5 MB: a 10-hour run fits in well under 1 GB. The panel's
day-close cash matched the game's on every day close (103 day events, restart replays included); its day
matched the game's through the morning plan wait only after the second morning fix (`4844a28`, checked on a short second run, seed 2950002:
no disagreement in 3 minutes over two day changes); with only the first, the panel trailed the game
by a day in 106 of 178 samples.

**The voice.**

| Measure | Rehearsal (live hook, 98 days) | Replay of 2026-11-10 (section 9) |
|---|---|---|
| Lines | 156, 0.82 a minute | 151, 1.2 a minute |
| Voice on air | 6.7 % | 15 % |
| Silence between lines | median 68 s, p90 121 s, max 149 s | median 32 s, p90 75 s, max 157 s |
| With `-fill-after 40s` (days 23–98) | median 42 s, 0.97 lines a minute, 8.4 % on air | – |
| Moment → line | median 5.2 s, p90 16 s, max 25 s | (narration-paced) |
| Narrator LLM | median 2.5 s, p90 4.9 s; 5 timeouts (8 s), 7 moments too old, 3 lines too late | 4.9 s median |
| Fact check | 119 first try, 24 on the retry, 8 caption fallbacks, 5 captions on a timeout | 143 / 6 / 2 |
| Beats | run start, appraisal join and three promotions, 7 augs, the cash buyout: all spoken | |

The voice never lagged the game by more than 25 s, and no wrong number reached the air after
the fixes (one did before: "Twenty-five five credits" for 255). It is sparse: a live day has one
or two moments, so most of the air is the game and the panel.

**Issues found and fixed** (each with a test):

| Issue | Fix |
|---|---|
| fenceshow's narrator and the pre-flight thought the game was running whenever any command line mentioned "Probably Stolen.exe" (a shell, a grep): the narrator would speak captions only | match the image name, as the league gate does (`958a382`) |
| Pre-flight's narrator test asked a thinking model without `enable_thinking: false`: "answered nothing" | ask as fenceshow does (`504b82b`) |
| Live panel's standing plan had no base policy (engine, permit, mortgage float): fenceshow never read `-base-directives` or the route | `-base-directives`, else the `-route`'s directives (`00ec0b6`) |
| Panel showed yesterday's day and cash for the whole morning plan wait (~50–110 s a day) | the morning publishes a ticker line with the new day's clock (`af1e775`, `4844a28`) |
| A restarted fenceshow re-greeted the run ("Day one…" on day 9) | the run start is replayed silently past day 1 (`5ba2b5e`) |
| A restart renumbered the recording from 1, and its WAVs overwrote earlier ones | continue the recording's sequence (`ee3f77b`) |
| A restarted narrator spoke stale filler at once | it waits `-fill-after` first (`a50577f`) |
| "Twenty-five five", "twenty-four-two", "one-seven-five" (a flash of Qwen's number words) passed or confused the check | numbers run together are a fault (`72e4692`); the narrator writes digits, which Kokoro reads as words (`26cf4fd`) |
| Right lines sent to captions: number words joined across a full stop; a purchase partly paid in Wild's favor; "nine thousand five seventy-eight" | `fbaf6aa`, `7d565ad`, `fc26048` |
| Plan captions spoken as "cash equals twenty-eight hundred" (`>=`) | comparison signs become words (`e696ae9`) |
| Repetition: "rent is just a whisper" three times in five days | a line sharing a four-word phrase with the last 20 is asked for again once (`dffd29e`); it helped little (see below) |
| On the cash route the plan read "Saving for the mortgage: 26,888 / 7,143", and after the buyout "mortgage offered" under "the shop is yours" | a cash route saves for the buyout; an owned shop has no offer (`526ab1c`) |

**Remaining risks.**

- *The strategist's latency on a shared model.* With two leagues on the 27B its plans took a median
  55 s and up to 120 s; 12 of 98 mornings lost the plan or its directives. Live the mod does not wait
  for the plan (no `plan_wait_s` yet), so a slow plan lands mid-day. Stop the leagues (section 1);
  the pre-flight does not measure plan latency.
- *Repetition.* The Shark leans on a few images ("in the till" 17 times, "rent is just a …",
  "in my pocket", "blood in the water"); the repeat guard accepts the second try and does not see
  three-word phrases. A larger narrator (the API) or a per-persona list of words to rest.
  Since addressed: a per-persona phrasebook for the run, an avoid list and image
  banks in the prompt (docs/SHOW.md "Variety"; measured in "Measured: density and variety").
- *Sparse narration.* 6.7 % voice on air; median silence 42 s at `-fill-after 40s`, with gaps to
  2.5 minutes. Live days run 3–6 minutes, so expect long quiet stretches; the game audio carries them.
  Since addressed: small beats (the counter, the night, the morning plan, buyout
  milestones, appraisal streaks; docs/SHOW.md).
- *A fenceshow restart* loses the net-worth sparkline's history and the moments list (the panel
  redraws from the latest day); the narrator's repeat memory starts over (now
  it is read back from `-record`).
- *Caption fallbacks* (8 + 5) are the raw caption: correct but flat ("Sold 2× Sealed Water Ration
  (Small), BloomBerry … for 217").
- *Not rehearsed:* the real mod and autopilot (pace, handovers, faults), GPU Kokoro, the API
  narrator, OBS, the current sprint policy's `buyout_save` (the gym used predates it), and a bridge
  restart (the gym does not re-hello).

**Recommended stream-night settings.**

- Leagues stopped; strategist the 27B (thinking off) or drift; narrator the API (Haiku), else the
  27B with one request at a time. Pre-flight GO on the narrator row (it now asks without thinking).
- GPU Kokoro when the strategist is off the 5090, else CPU Kokoro (rehearsed: no backlog, a line in median 1.5 s,
  p90 10 s, max 15 s under load, nothing dropped for TTS).
- fenceshow: `-route sprinter` (or `-base-directives` matching the bridge's) so the plan box has the
  base policy; `-fill-after 40s`; the defaults for `-min-gap` (8 s), `-max-age` (15 s), `-max-lag` (25 s).
- Autopilot pace 800–1200 as in section 5; a slower pace only lengthens the silences.
- A fenceshow restart is safe mid-run (same command, same `-record`); expect the sparkline to restart.
