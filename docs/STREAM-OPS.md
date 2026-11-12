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
| **The game box** | Linux, RTX 5090 (32 607 MiB), Proton | on stream: the game and OBS's encoder on the GPU; the mod, the bridge (`:7341`), fenceshow (`:7350`), the drift gate (`:7380`) and CPU Kokoro (`:8881`) | llama-swap on `:9000` serves the 27B the leagues use, **off stream only** |
| **drift** | MacBook (M5 Max, 128 GB), two MLX servers behind its llama-swap | the strategist `$DRIFT_MODEL` (`DRIFT_MODEL` in `local.mk`; Qwen3.8-27B, `qwen38-27b-mlx`, on MTPLX, since 2026-11-11; Qwen3.8 Flash, `qwen38fn-mlx`, before) and the narrator's fallback `$DRIFT_NARRATOR_MODEL` (Qwen3.5-9B, `mlx-community/Qwen3.5-9B-MLX-4bit`, on mlx_lm.server, since 2026-11-11), both at `$DRIFT_URL` | the stream's player and its narrator, both on the 27B through the gate (owner, 2026-11-12); may be offline. The 27B serves one request at a time and queues a second caller behind the first, hence the gate; the 9B is a separate process the narrator falls back to directly ("The narrator's model") |
| **The API** | Anthropic | optionally the narrator (`claude-haiku-5-5`) or the strategist | only when `ANTHROPIC_API_KEY` is set in the environment |

**The rule on stream (owner, 2026-11-11).** The agent stands in for a human streamer: one persona
that plays and narrates, so drift is both the *strategist* and the *narrator*, the same 27B through
the drift gate below (the narrator on the gate's `/narrate/v1` since 2026-11-12; "The narrator's
model"). The 5090 runs only the game and OBS's encoder: no LLM on it during a stream.

**Roles.** The *strategist* writes each morning's plan (the bridge, `-planner llm -directives
delta`); the *hands* are the executor inside the mod (no model); the *narrator* is fenceshow's
(one short line per moment); *TTS* is Kokoro on the CPU (piper as fallback). The API (Haiku) is an
optional narrator upgrade: faster and never queued behind a plan, at a per-line cost, and a
different voice from the player. Two strategists are proven on
the stream's policy: in the t7 hard league both `qwen27-delta` (the 27B on the 5090) and
`drift-delta` (drift) bought the shop on day 175 in 3 of 3 games, the same as the executor alone
(`var/league/t7-hard-*/REPORT.md`). Plan latency p50: drift on Flash 3.8 s, the 27B 7.3 s; a live day takes
3–6 minutes, so either keeps up. drift on the 27B: see "The drift gate", measured.

**GPU budget on the 5090.** On stream it holds the game and OBS's encoder (NVENC) only; the
game's own VRAM is still unmeasured (read `nvidia-smi` while it runs on the first stream). Off
stream, the leagues' 27B takes 29 427 of 32 607 MiB (measured 2026-11-10).

| TTS | VRAM | Speed | When |
|---|---|---|---|
| **CPU Kokoro**, the default (`fence-show-kokoro`, `:8881`) | none | 6 s a short line idle, 12 s under load (pre-flight, 2026-11-10); 1.5 s median, 15 s worst in the rehearsal | every stream |
| GPU Kokoro, an owner option (`vibe start tts_kokoro`, `:8880`) | 2–3 GB beside the game | well under a second | when `nvidia-smi` shows the game leaves 3 GB free; fenceshow `-tts-url http://127.0.0.1:8880` |

The live narrator drops a line that is ready later than 25 s after its moment, so on the CPU under
load it says a little less.

**The narrator's LLM.** The strategist's own model, drift's 27B (`$DRIFT_MODEL`), through the gate's
`/narrate/v1` with `-llm-timeout 15s` (owner, 2026-11-12): the player's voice is the player's model, and
the gate keeps its lines behind the plans, so a plan never waits for more than a line in flight. The
fact check means it never says a wrong number; a line that fails twice, one the gate drops while a
plan holds drift, or a model that does not answer within `-llm-timeout`, speaks the caption.
Fallbacks, in order: drift's 9B (`$DRIFT_NARRATOR_MODEL`) called directly at `$DRIFT_URL` with
`-llm-timeout 8s`, around the gate, since it is its own process; the API (Haiku), `-llm-provider
anthropic`, or `auto` with `ANTHROPIC_API_KEY` set. Never point the narrator at llama-swap on the
5090. fenceshow's `-yield-to-game` applies only to a narrator served on this machine: a loopback
URL that is not the gate, or the gate in front of a local model (fenceshow asks the gate's `/gate`
every 30 s); drift, directly or through the gate, keeps speaking while the game runs.

**When drift is offline.** The pre-flight's strategist row is NO-GO (the gate answers 502 for an
unreachable upstream). Options, in order: wait for drift; the API as strategist and narrator
(`-provider anthropic` on the bridge, `ANTHROPIC_API_KEY` for fenceshow); the 27B on the 5090 as
strategist (`make bridge … -base-url http://127.0.0.1:9000/v1`), which breaks the stream rule and
costs the GPU Kokoro option, with the narrator on the API or captions. Mid-stream: section 6, "Model
endpoint down".

**Leagues on stream night.** Stop them (`max_gyms_while_gaming` only throttles gyms; a league's
local endpoint yields to the game and unloads its model, which then reloads on the next plan).
The hybrid planner's rollouts are CPU-heavy: with CPU Kokoro, keep rollouts to a few threads or
run the hybrid on another run.

**Fallbacks** (each automatic; the panel says so):

| Lost | What the stream does |
|---|---|
| the strategist | no new plan reaches `/v1/summary`; the autopilot plays on the directives it has: the base policy (`autopilot_directives`) and the last plan the bridge still serves. The plan box keeps the standing plan |
| the narrator LLM | the caption is the line (`say.source: caption`); also when the gate drops a line that queued behind a plan too long |
| the drift gate | the bridge keeps its last plan and fenceshow speaks captions until it is back; restart `make gate` (it holds no state) |
| Kokoro | piper (`~/.local/bin/piper`), then captions only |
| the bridge | the mod defers every decision and buffers its events; the autopilot plays on the base directives; fenceshow follows the new session when the bridge is back |
| fenceshow | OBS's browser source goes blank; restart it with the same flags (section 6) |

### The drift gate

drift's 27B serves one request at a time, so whatever calls it goes through `cmd/fencegate`
(`internal/gate`): a loopback, OpenAI-compatible proxy that sends one request at a time to drift, the
strategist's first. The role is the path's first segment, so no client changes; only its base URL
does. The narrator speaks through `/narrate/v1` on the same 27B; its fallback, the 9B, is a process of
its own and is called directly, around the gate ("The narrator's model").

```sh
make gate                                           # fencegate on 127.0.0.1:7380, upstream $DRIFT_URL from local.mk
make bridge ARGS="-mode observe -planner llm -directives delta -route sprinter -persona shark \
  -provider openai -base-url http://127.0.0.1:7380/plan/v1 -model $DRIFT_MODEL -no-schema -thinking off"
go run ./cmd/fenceshow -live-log var/log -bridge http://127.0.0.1:7341 -player drift -model $DRIFT_MODEL \
  -persona shark -route sprinter -fill-after 40s -tts-url http://127.0.0.1:8881 \
  -llm-provider openai -llm-url http://127.0.0.1:7380/narrate/v1 -llm-model $DRIFT_MODEL -llm-timeout 15s
# fallback, the narrator on drift's 9B around the gate: -llm-url $DRIFT_URL -llm-model $DRIFT_NARRATOR_MODEL -llm-timeout 8s
# optional: -cohost, a second voice on the gate's /filler/v1 (docs/SHOW.md, "The co-host")
```

(or `FENCE_BASE_URL` / `FENCE_SHOW_LLM_URL` / `FENCE_SHOW_LLM_MODEL` in the environment or `bridge.env`;
`make show-live ARGS="-persona shark -route sprinter -fill-after 40s -tts-url http://127.0.0.1:8881"` runs
the same fenceshow with the narrator from the Makefile's `NARRATOR_*`.)

- **Order.** One request upstream at a time: `/plan/v1` (the strategist's plans, directives,
  windfall re-plans, lessons and any fork calls: everything the bridge asks) first, `/narrate/v1`
  next, `/filler/v1` last, first come first served within a role. A request never preempts the one
  in flight. Header `X-Fence-Role` overrides the path; `/v1/…` with no role is the strategist's.
- **Drops.** A queued request that waits longer than its role's limit gets HTTP 503 from the gate
  (`X-Fence-Gate: dropped`, error type `fencegate_dropped`): narrator 5 s (`-max-wait-narrate`),
  filler 3 s, the strategist never (`-max-wait-plan 0`); `X-Fence-Max-Wait` overrides per request.
  The narrator treats it like any failure and speaks the caption, so a line never waits out a plan.
  A caller that gives up (the narrator's `-llm-timeout`) leaves the queue, and the gate cancels its
  upstream call if it was in flight.
- **Callers outside the gate.** The gate sees only its own calls. MTPLX queues anything else that
  reaches the 27B (another agent's league, a test) in the same line, first come first served (no 503
  in the 2026-11-11 logs), so the gate's wait times miss it. On stream nothing else may call drift's 27B.
- **Retries.** An upstream 503 (someone outside the gate holds drift) is retried with backoff (250 ms
  doubling to 4 s) for up to 20 s (`-retry-for`), holding the slot.
- **Metrics.** `curl 127.0.0.1:7380/gate` (also `/metrics`): per role the requests, served, dropped,
  cancelled, errors, retries, queued, and p50/p95/max of the queue wait and the upstream time; a
  summary line in the log every 5 minutes (`-log-every`). No address appears in it.
- **Listings** (`GET …/models`) pass straight through without queueing, so the pre-flight can read
  them while a plan is written; drift unreachable gives 502.
- **The narrator's timeout.** fenceshow's `-llm-timeout` (default 8 s) covers the queue and the line.
  On the 27B through the gate use 15 s; on the 9B fallback the default is ample (a line in ~1 s).

**Measured (2026-11-11, drift on Qwen3.8-27B, `qwen38-27b-mlx`).** The dress rehearsal's decision
log (section 10: seed 2950001, the Shark on the sprinter route, a game day ≈ 2 minutes), days 30–41
replayed through fenceshow at the recorded pace (`-replay … -recorded-pace -narration-paced=false
-fill-after 40s`, so the narrator works under its live limits), the narrator on drift through the gate
and CPU Kokoro (`am_fenrir`; the box at load 25–66 from other work). Alongside, a strategist load like a
live run: when the replay reached each morning, that morning's plan and delta directives written with
the bridge's own builder (`strategy.DraftPlanConversation` and `DraftDirectives`, persona and route as
the rehearsal, no memory or exemplars) from the log's morning snapshot, through `/plan/v1`. One
request to drift at a time through the gate; no upstream 503 was seen. **But drift was not otherwise
idle:** MTPLX's flight log shows another agent's calls to the same 27B throughout three of the four
runs (strategist-sized prompts of 7–13k tokens, ~800 out, and 10–13k-token one-word calls): it served
170, 106 and 141 requests in the "8 s", "strategist alone" and "15 s" runs, of which the gate sent 45, 28
and 55; the "narrator alone" run had 3 outside calls. Read the plan times and the timeouts below as
measured under that extra load; a clean re-run is in "The narrator's model".

| Run | Narrator `-llm-timeout` | Lines/min | Lines in character / caption | Caption why: gate drop / timeout / check | Moments not spoken | Plan call p50 / p95 | Morning (plan + directives) p50 / p95 |
|---|---|---|---|---|---|---|---|
| Strategist alone | – | – | – | – | – | 30.9 / 40.7 s | 40.2 / 56.8 s |
| Narrator alone | 15 s | 1.43 | 27 / 6 (82 %) | 0 / 1 / 5 | 1 | – | – |
| Both, through the gate | 8 s (default) | 1.16 | 1 / 26 (4 %) | 12 / 14 / 0 | 2 | 43.0 / 68.0 s | 78.8 / 90.1 s |
| Both, through the gate | **15 s** | **1.34** | **19 / 12 (61 %)** | 8 / 5 / 0 | 2 | 31.8 / 52.6 s | 45.0 / 57.4 s |

- **Plans keep their pace.** With the narrator sharing at 15 s, a plan waited for a narrator line at
  most 11.4 s (p95 9.4 s); the morning took 45 s at p50 against 40 s alone, well inside a live day
  (3–6 minutes). The strategist was never dropped.
- **The 8 s default is too short for the 27B.** A line takes 2.4 s at p50 and 3.1 s at p95 when it
  runs, but some calls hang past 15 s with nothing ahead of them in the gate (5 of 35 shared, 1 of 44
  with the narrator alone). Found since: every one of those (63 slow small calls in the flight log)
  began while an outside call was generating, and MTPLX queued it behind; none was drift stalling.
  Abandoned calls do not hold drift: of 23 the narrator gave up on, 21 were still queued and were
  dropped without generating a token when their turn came, and 2 stopped after 13 and 19 tokens.
  MTPLX logged no memory-guard trims or prefill sheds since the 27B came up (those in
  `~/mtplx.err.log` are Flash-Next's, at 95 GB live), so no MTPLX flag would help: keep other
  callers off the 27B on stream.
  Through the gate use `-llm-timeout 15s`.
- **Where the captions come from.** Most of the remaining captions are moments that fall inside the
  45 s morning, when the plan holds drift (the morning plan beat itself, the pace line): the gate
  drops them after 5 s and the caption is spoken. Every line the model wrote passed the fact check;
  its faults were all repeats (`docs/SHOW.md`, "Variety").
- **Density.** 1.34 lines a minute shared, 1.43 alone: under the 1.5–2 target of `docs/SHOW.md`
  (met there with the 27B on the idle 5090 and an idle CPU); the silence median was 23 s, p90 87 s.
- **Upgrades if that is not enough:** the API (Haiku) as narrator, which never waits for a plan; or a
  second, small narrator model on drift: built, "The narrator's model".

Lines from the shared run (15 s), days 30–41 (`llm` in character; `caption` the gate or a timeout):

| Day | Moment | Source | Line |
|---|---|---|---|
| 30 | morning plan | caption (drop) | Day 30 plan: Sell Old Hydroponic System (est 515). Target: +515 cash. |
| 30 | purchase | llm | Showcase two is locked in. Circling slow, patient, letting the shop do the work. |
| 31 | trade | llm | 475 credits. The hedonist paid up front. |
| 33 | counter | llm | Servant from the Upper Levels, all stiff collars and sweat. Paid 355 for water and a care package. |
| 34 | night | llm | Came home with empty pockets and a heavy head. Just the long hustle, one mark at a time. |
| 35 | appraisal streak | llm | Forty-five straight. The Board loves a steady hand. |
| 37 | morning plan | llm | Rent is five days out and I’m sitting pretty. Let’s flush the cheap junk and keep the landlord smiling. |
| 39 | counter | caption (timeout) | Sold BloomBerry Wine to Worn-Out Spacer for 150 (+138 on cost). |
| 40 | counter | llm | Jackson came in sweating, needed power now. I held his gaze until he blinked, then took the 330 and let him leave lighter. |
| 41 | night | llm | Ten digs, one trap. Back with 50 credits. |

### The narrator's model

Since 2026-11-11 drift runs a second, small model for the narrator beside the 27B:
**Qwen3.5-9B, MLX 4-bit** (`mlx-community/Qwen3.5-9B-MLX-4bit`, Apache-2.0, 6.0 GB on disk, 5.6 GB
resident), served by `mlx_lm.server`. Since 2026-11-12 it is the narrator's **first fallback**, not its
primary: fenceshow calls it directly at `$DRIFT_URL` with `-llm-model $DRIFT_NARRATOR_MODEL -llm-timeout
8s` when the 27B's lines fail.

**Decision (owner, 2026-11-12): the narrator is the 27B through the gate.** The measurements below
gave no reason to pay the 9B's voice for density: on a quiet drift the 27B shared through the gate is in
character as often (81 % against the 9B's 82 %, 1.34 against 1.44 lines a minute), its voice is clearly
better (terse and punchy where the 9B is wordy, mixes metaphors and now and then inverts the sense), and
its lines hold the facts better (the fact check passed 25 of 26 against the 9B's 27 of 33, and the 9B
slips lines past the check it cannot read). The cost is the morning: the gate drops the plan and pace
beats while a plan holds drift, and the caption is spoken. The 9B stays loaded as the fallback.

**Why the fallback goes around the gate, not a second upstream in it.** The gate exists to put one model's single slot
in order. The narrator's model has its own process, so queueing its lines behind the plans would bring
back the morning drops this model is there to remove, and a plan never needs to wait for a line. The
gate's metrics lose the narrator, but fenceshow logs each line's `llm_ms` and `say.check`. The
pre-flight's "drift gate" row is GO with the narrator through the gate on the strategist's model, says
"the narrator on its fallback model, beside the gate" for the 9B called directly, warns if the 9B is
pointed at the gate's `/narrate/v1`, and warns if the narrator calls the strategist's 27B around the
gate, where its lines would race the plans for the one slot.

**Why this model.** Three candidates within the 10 GB budget, each narrating days 30–41 of the dress
rehearsal (narration-paced, no TTS) directly against drift, beside the 27B on the same prompts:

| Model (MLX 4-bit) | Licence | Size | Line p50 / p95 | Fact check: first try / retry / caption | Verdict |
|---|---|---|---|---|---|
| **Qwen3.5-9B** | Apache-2.0 | 6.0 GB | 0.8 / 1.3 s | 11 / 5 / 2 | in the persona's voice, wordier than the 27B; chosen as the small model |
| Ministral-3-8B-Instruct-2512 | Apache-2.0 | 5.6 GB | 0.9 / 1.4 s | 14 / 2 / 2 | the most colourful, but swears on air, trails off mid-sentence, and mlx-lm loads its tokenizer with a known bad regex |
| Qwen3.5-4B | Apache-2.0 | 3.1 GB | 0.5 / 1.1 s | 14 / 2 / 2 | flat, and some lines mean nothing ("The feed's already on the table") |
| Qwen3.8-27B (the strategist's model, for reference) | – | – | 2.8 / 3.7 s | 12 / 4 / 1 | terse and punchy: the best voice |

Gemma 4 was left out for its licence (Gemma terms, not Apache); Qwen3.8 has no small dense model.

**Serving (on drift; nothing of it is in this repo).**

- `~/narrator-venv`: its own venv (Homebrew Python 3.14, `mlx-lm==0.31.3`, `mlx==0.32.2`), so an
  MTPLX upgrade cannot break it. The weights are in `~/.cache/huggingface/hub`.
- launchd agent `local.narrator` (`~/Library/LaunchAgents/local.narrator.plist`): `mlx_lm.server
  --model mlx-community/Qwen3.5-9B-MLX-4bit --host 127.0.0.1 --port 8001 --max-tokens 160
  --chat-template-args '{"enable_thinking":false}' --prompt-cache-size 4`, `HF_HUB_OFFLINE=1`,
  `RunAtLoad` and `KeepAlive`; logs `~/narrator.log`, `~/narrator.err.log` (not in drift's log
  rotation; ~100 KB a stream).
- vibe cell def `~/.config/vibe/backends/narrator-qwen35-9b-mlx.yaml`: a `cloud_peer` to
  `http://127.0.0.1:8001` with `cell: drift`, rendered by `vibe router render` into llama-swap's
  `peers:`. A cloud_peer is left out of the fleet front by design, so no fleet change.
- The model id must be the `--model` string: mlx_lm.server loads whatever model a request names, and
  llama-swap forwards only the listed id.
- The 27B's llama-swap config before the change: `~/mtplx-dl-logs/llama-swap-config.bak-narrator-20261111`.
- Restart the narrator alone: `launchctl kickstart -k gui/$(id -u)/local.narrator` (the 27B and
  llama-swap are untouched).

**Removing it** (on drift): check `~/mtplx.log` shows no generation for two minutes (a llama-swap
restart drops calls in flight), then

```sh
launchctl bootout gui/$(id -u)/local.narrator
rm ~/Library/LaunchAgents/local.narrator.plist ~/.config/vibe/backends/narrator-qwen35-9b-mlx.yaml
~/.local/bin/vibe router render && launchctl kickstart -k gui/$(id -u)/local.llama-swap
rm -rf ~/narrator-venv ~/narrator-logs ~/.cache/huggingface/hub/models--mlx-community--Qwen3.5-9B-MLX-4bit
```

The stream's narrator is unaffected (it is the 27B through the gate); the API is then its only fallback.

**Measured (2026-11-11, night).** The numbers behind the decision. The same rehearsal window as "The drift gate" (days 30–41, seed
2950001, recorded pace, `-fill-after 40s`, CPU Kokoro `am_fenrir`, the same strategist load through
the gate), with drift otherwise idle: MTPLX served exactly the gate's calls in all three runs. The
shared row is re-run here because the earlier one had another agent's load on the 27B.

| Run | Narrator | Lines/min | In character / caption | Caption why: gate drop / timeout / check | Line p50 / p95 | Plan call p50 / p95 | Morning p50 / p95 |
|---|---|---|---|---|---|---|---|
| Strategist alone | – | – | – | – | – | 18.5 / 23.5 s | 20.7 / 25.3 s |
| **27B shared, through the gate** (`-llm-timeout 15s`; the stream's narrator since 2026-11-12) | 27B | **1.34** | **25 / 6 (81 %)** | 5 / 0 / 1 | **3.2 / 5.2 s** | 22.5 / 25.4 s | 24.5 / 29.3 s |
| Own model, around the gate (`-llm-timeout 8s`; the fallback) | Qwen3.5-9B | 1.44 | 27 / 6 (82 %) | 0 / 0 / 6 | 1.1 / 2.3 s | 20.3 / 25.7 s | 22.0 / 30.6 s |
| (earlier, with outside load) 27B shared | 27B | 1.34 | 19 / 12 (61 %) | 8 / 5 / 0 | – | 31.8 / 52.6 s | 45.0 / 57.4 s |

- **On a quiet drift the small model buys little density.** Clean, the 27B shared through the gate
  is in character 81 % of the time against the 9B's 82 %, at 1.34 against 1.44 lines a minute
  (silence median 17 s vs 15 s, p90 92 vs 85 s): both just under the 1.5–2 target. The earlier 61 %
  was mostly the outside load. What changes is where the captions come from: the 27B's are the
  morning plan and pace beats dropped while a plan holds it; the 9B's are lines that failed the
  fact check twice (its lines drew 12 faults: 8 repeats, 4 numbers), and it speaks in the
  mornings.
- **Fact check.** The 9B: 27 of 33 lines passed (21 first time, 6 on the retry). The 27B: 25 of 26
  (20, 5). The 9B also gets past the check with lines the check cannot read: "One hundred seventy-five
  credits down the drain" for a sale at 175, two numbers in one line.
- **Voice.** Clearly worse than the 27B: longer, mixed metaphors, now and then the sense inverted.
  Both sets of lines are below; the owner should listen before choosing.
- **Does it slow the player?** A little, only while a line is generating. Plans p50 20.3 s against
  18.5 s alone (the 27B shared: 22.5 s, where a plan also waits up to 3 s for a line in flight). A
  microbenchmark on the 27B (600 tokens, temperature 0) ran at 42 tok/s with the narrator idle and
  ~21 tok/s with narrator calls back to back: the two models split the GPU while both generate. An
  idle narrator costs nothing (31 tok/s with it stopped, 35 idle again; the baseline itself sagged
  over the benchmark, no thermal warning recorded). At live rates (a ~1 s line every ~40 s) that is a
  second or two on a plan.
- **Memory.** The 27B's MTPLX keeps 38 GB (its prompt cache intact); the narrator 5.6 GB; ~77 GB
  still free or reclaimable.

Lines, days 30–41 (`llm` in character; `caption` a fallback):

| Day | Moment | Own model (Qwen3.5-9B) | 27B shared (clean) |
|---|---|---|---|
| 30 | morning plan | I'll sell the hydroponic system to stock up on weapons and ammo, then scavenge tonight while I wait for rent. | Card sharp counting the deck. I'm flipping that hydroponic rig and hitting the dumps for scrap. |
| 30 | purchase | Seven-fifty on the Showcase, Wild's decisively ahead, and I'm just circling slow to let the rent bite later. | I dropped 750 on the glass. Cash is still heavy in the till, so I'm circling slow, patient, letting the room sweat. |
| 31 | pace | Ahead of the ghost, net worth 5,441, and this bank roll lets me loosen the noose on that rent coming due. | caption (drop): Day 31: net worth 5,441, back inside the sprinter book's band, ahead (median 4,906). |
| 32 | counter | Upper Levels money burns a hole in pockets, but I cashed that 354 check with a smile before they knew I'd already cleared the rent. | That Upper Levels suit sweated, but the street taught me to read it. Two packs, one bottle, 354 credits in hand. |
| 33 | counter | I let the Upper Level servant choke on his own pride while I pocketed the water tablets and the care package. | Upper Level servant sweated, but I read the fear. Took the water and care package, 355 credits. |
| 34 | morning plan | There's plenty in the till to cover the rent, so I'll clear those injectors and start brewing that wine while I keep the buyers hunting. | Clear the injectors off the counters, start the wine batch, and let the rent pay itself. |
| 35 | counter | The spacer's face screamed pay me fast so I took his credits and left his pride on the street. | Spacer looked dead on his feet. One sip, he coughed, paid 175, and dragged himself out before the door slammed. |
| 36 | counter | One hundred seventy-five credits down the drain and a spacer's dignity gone, leaving me with a fat margin for the rent. | Tired spacer stumbled in, desperate for a hit. I let him buy the wine for 175. |
| 37 | morning plan | Four thousand eight hundred twenty credits in my pocket and the rent is just a rounding error five days from now. I'll keep the shop stocked while I quietly bleed a few tourists for their pride. | Rent is five days out, and my pocket is heavy. Time for a feeding frenzy before the landlord eats me alive. |
| 41 | night | Mouse Traps are just good for scaring mice, not people. I'll sell this to the next desperate liar for a quick win. | Night air smells like rust and opportunity. Dragged home a Mouse Trap, worth about 50. |

## 2. Pre-flight

Run it on the game box, game closed, about 30 minutes before going live:

```sh
make gate &        # the drift gate on :7380, in front of $DRIFT_URL
make stream-preflight ARGS="-strategist-url http://127.0.0.1:7380/plan/v1 -strategist-model $DRIFT_MODEL -persona shark"
```

with fenceshow's narrator on the 27B through the gate (`FENCE_SHOW_LLM_URL=http://127.0.0.1:7380/narrate/v1`,
`FENCE_SHOW_LLM_MODEL=$DRIFT_MODEL`); the pre-flight checks it with `-narrator-url` and
`-narrator-model` as well.

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
| drift gate | shown when a role goes through fencegate: GO when both do on the one model and its upstream is another machine (with plan p50 and narrator drops so far), or when the strategist does and the narrator is on its 9B fallback directly; WARN when only the narrator does, when the narrator calls the strategist's model around the gate (racing the plans for its one slot), when the 9B goes through the gate, when the gate fronts a model on this machine, or when both roles call one model URL without the gate | `make gate`; point the bridge at `/plan/v1` and the narrator at `/narrate/v1` |
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
   hole, the panel's column and lower third, the narrator audible over the game). On air, record
   locally the whole time as well (section 7, "Record locally, upload the master").
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

For each run, first the day-0 choice (docs/STREAM.md, "Model agency"), before the mod, the bridge and
fenceshow start: the strategist picks or composes the run's route, with the stream's previous choices in
the prompt and the last one off the menu.

```sh
# drift's 27B through the gate (start the gate first), lean prompt; add -format F for a format run,
# -start S to fix the start, -dry-run to see a pick without recording it; PERSONA=id is the card's
# archetype, which writes the thesis and plays the run whatever route is picked
make choose PERSONA=botanist ARGS="-format sprint_91"
```

It prints the route and the save to start (difficulty, start, perks), the thesis, the mod's
`autopilot_directives` line (a format's handicaps applied), and the flags for the bridge (`-choice … -route …
-persona …`, plus `-format`) and fenceshow (the same without `-choice`): use them in place of the route and
persona in the commands below. A composed route is a file in the choice's directory, which `-route` takes as
it does a name. The choice is recorded in `var/stream/choices/<UTC stamp>/` (never committed); the newest
directory there is the route the next choice excludes, so **delete the directory of a choice that was not
played** (a rehearsal, a restart before day 1). A fallback (the model's answer was unusable twice) says so on
a `FALLBACK` line; delete it and ask again, or play it. No answer at all is an error and records nothing.

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

- **A format run** (docs/SHOW.md, "Formats": a run with a real chance of losing). Pick one from
  `go run ./cmd/fenceformat list` (the lab's odds and the attempts so far), then give all three
  pieces the same format:

  ```sh
  # the mod: Options → Mods → autopilot_directives, the format's directives in one line (handicaps locked);
  # `make choose ARGS="-format sprint_91"` prints this line for the route it chose
  go run ./cmd/fenceformat directives sprint_91            # add -route NAME for a format that names none
  # the bridge: the plans are told the format, and a change to a locked key is overruled
  make bridge ARGS="-mode observe -planner llm -directives delta -format sprint_91 -route sprinter -persona shark \
    -provider openai -base-url $STRATEGIST_URL -model $STRATEGIST_MODEL"
  # the panel and narrator: the format card, the lab's odds, the stakes; the run's end goes in the record
  go run ./cmd/fenceshow -live-log var/log -bridge http://127.0.0.1:7341 \
    -player qwen27 -model qwen3.8-27b -persona shark -route sprinter -format sprint_91 -fill-after 40s \
    -record var/show/stream-$(date +%F).jsonl -tts-url http://127.0.0.1:8881
  ```

  Start the save on the format's difficulty (a challenge format needs a challenge save). Put the
  default text back in `autopilot_directives` after the run. A run that ends while fenceshow is down is
  not recorded: add it with `go run ./cmd/fenceformat record -run <player>:<run_id> -ending rent -day
  141 sprint_91`. The record is `var/formats/record.jsonl`; keep it across streams like the board.
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
- *Narrator LLM*: nothing to do; captions are spoken. If the 27B's lines keep failing while plans
  still arrive, restart fenceshow on the 9B fallback (section 1, "The drift gate"); the 9B alone down:
  on drift, `launchctl kickstart -k gui/$(id -u)/local.narrator` (the 27B is untouched). Switch
  fenceshow to the API at the next break if it lasts.
- *Kokoro*: `docker start fence-show-kokoro` (CPU) or `vibe start tts_kokoro` (GPU); piper speaks
  meanwhile if its voice is installed.
- *drift's HTTP 503* ("background task bypassed while foreground generation…"): drift is busy with
  another request. Behind the gate this happens only when a caller outside the gate uses drift; the
  gate retries for 20 s (`-retry-for`) and logs "upstream busy; retrying". Find the other caller.
  Never give drift both roles without the gate.

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
8. Make the VOD kit (below) and upload the local recording to YouTube by hand.

### Record locally, upload the master

The run streams live on Twitch; the VODs go to YouTube. Record in OBS **at the same time** as
streaming (Settings → Output → Recording; "Start Recording" with "Start Streaming", or tick
"Automatically record when streaming" under Settings → Advanced) and upload that file. Do not
depend on Twitch: its past broadcasts expire (days to weeks depending on the account), its
"Export to YouTube" re-encodes the already-compressed stream, and a muted segment in the VOD
stays muted in the export. The local master is full quality, survives a dropped stream, and is
what `fencevod` times its chapters against.

- Format: MKV (a crash never loses the file; OBS's "Automatically remux to MP4" makes the upload
  copy), the canvas at 1920×1080 60 fps, a quality-based encoder (NVENC CQ ~20 or x264 CRF 18),
  the stream's audio tracks. Keep OBS's default file name (`%CCYY-%MM-%DD %hh-%mm-%ss`): `fencevod`
  reads the start time from it.
- One recording per stream session; a crash or an OBS restart starts a new file, which is a new
  recording to `fencevod` (pass both; each gets its own chapters).
- Keep the masters under `var/stream/<date>/` with the feed (never committed: raw recordings
  stay off the repo), and keep them until the YouTube upload has finished processing.
- The narration WAVs are already in the recording through the panel's browser source; nothing
  to mix afterwards.

### The VOD kit (`cmd/fencevod`)

From the stream's feed and the recordings' start times, one kit per recording: YouTube chapters,
a title, a description (with the chapters, and the result below a spoiler line), tags, a list of
30–60 s highlight windows for Shorts and Twitch clips, and an ffmpeg script that cuts them. Local
files only: it uploads nothing and calls no model.

```sh
# one session, OBS's default file name gives the start time; ffprobe gives the length
go run ./cmd/fencevod -feed var/show/stream-2026-11-20.jsonl \
  -recording "var/stream/2026-11-20/2026-11-20 19-58-12.mkv" -out var/vod/2026-11-20
# a run over three sessions: one -recording-start per recording (local time, or RFC 3339;
# "/2h45m" appended gives the length when the file is not at hand)
go run ./cmd/fencevod -feed var/show/stream-2026-11-20.jsonl -feed var/show/stream-2026-11-21.jsonl \
  -recording-start "2026-11-20 19:58:12" -recording-start "2026-11-21 20:01:40/3h5m" \
  -recording-start "2026-11-22 19:59:03" -out var/vod/run-shark
sh var/vod/2026-11-20/cut.sh            # the clips, into var/vod/2026-11-20/clips/ (needs ffmpeg)
```

Out, per recording (`part-N/` when there are several): `title.txt`, `description.txt`,
`chapters.txt`, `tags.txt`, `highlights.tsv` (start, end, length, day, caption), `cut.sh` and
`kit.json` (all of it, for scripts).

- **Timing.** Each event is placed at the time the panel showed it (a feed's publish time `at`;
  a decision log's `ts`) and offset from its recording's start. Recording times without a zone are
  local (`-tz` to change). A recording without a length ends where the next starts; the last one
  shortly after the feed's last event. A recording that holds no event is reported: its start
  time or `-tz` is wrong.
- **Chapters** (YouTube's rules: the first at 0:00, at least three, each at least 10 s; about one
  per 8 minutes, 10–30). The start of each game week, or of each two weeks when weeks would fill
  more than half the list; then the loudest moments by the narrator's own ranking
  (`show.Priority`) with a named beat ahead of a plain event of the same rank: the run's start,
  joining the appraisal board and each promotion, the mortgage and its payments, the gun, a
  handover, buyout milestones, the permit and upgrades, big trades and a counterfeit sold, plan
  pivots (the plan changes that make a plan a moment, `show.PlanPivot`), Georgio, missed rent, and
  any moment of a kind added later (bets, forks, the ghost band's pace, …) at whatever rank
  `Priority` gives it. The everyday small beats (counter, night, morning plan, appraisal streak),
  day closes and heat ticks are never chapters. Each repeat of a kind and beat ranks lower, so
  eight aug fights do not crowd out the rest, and the same pivot taken up again is one chapter.
  No two chapters closer than a third of the average spacing; a loud moment on a week's start
  names that chapter ("Weeks 5–6: days 29–42 · A quarter of the way to the deed"). The ending is
  always "How it ends", win or lose.
- **Title, description, tags.** The title says who plays, as whom, the format, difficulty and route,
  and the part, never the result. The description: the premise in the show's voice, the days, the
  format, the route's brief, the setup and seed, the fair-play rule, the chapters, tinyBuild's
  copyright notice (below), then a spoiler line and the result (or where a part leaves the run).
  Templates only: no LLM writes them, so they never invent a number.
- **Highlights** (`-clips`, default 12): moments ranked by `Priority`, a fact-checked line of the
  model's own (not a caption read out) worth a rank and a half, a morning plan a rank less; repeats
  of a kind and clips crowding the same stretch rank lower; none overlap. A window opens 12 s
  before the moment and closes 3 s after its line has been spoken (2.6 words a second), held to
  30–60 s inside the recording. The caption is the line. Lines are matched to their moment by the
  feed's `say.for`, so a decision log's moments can take a feed's lines (`-replay MATCH -feed FEED`:
  the moments are the current builder's, with today's beats).
- **Clips.** `cut.sh [RECORDING]` re-encodes each window (x264 CRF 20, AAC) so cuts are exact, not
  on keyframes. They are 16:9; crop to 9:16 for Shorts in the editor.

Tested on the dress rehearsal (section 10) with a made-up OBS start one minute before the run's
start, 02:36:40 AST: 3 h 12 min, days 1–98, 24 chapters (the weeks in pairs, the run start, the
appraisal board's join and three promotions, four plan pivots, the permit and five other purchases, the
first buyout milestone, an aug fight, two big sales, a counterfeit, Georgio's payback, "How it
ends") and 12 clips of 30–45 s, eleven of them ending on the Shark's own line. Split at 04:15 into
two recordings: days 1–50 and 50–98, 12 and 11 chapters, part 1 ending "to be continued" with its
cash and net worth, part 2 opening on its week. (The rehearsal directory is not a league match:
`-replay` read a directory holding its `log/`, its `bridge.out` as `bridge.log` and a hand-written
`match.json`; `-feed` gave the narrator's lines.) Cut from a synthetic 3 h 13 min test-pattern file named like an OBS recording: start and length
read from the file, 12 clips of exactly the planned lengths.

### Rights and music (checked 2026-11-11)

- **The publisher's policy.** Probably Stolen is developed by Questing Goose Studio and published by
  tinyBuild (Steam store page, https://store.steampowered.com/app/4348910/). tinyBuild's influencer
  policy, https://www.tinybuild.com/influencer-policy (linked from its help centre), authorises
  anyone to use images and sounds from any of tinyBuild's games in Let's Plays, streams and similar
  videos on YouTube, Twitch, Facebook and TikTok, "even if your videos are monetized or contain
  advertisements", provided the video shows original content and not only the game's, and carries
  the notice "Copyright © tinyBuild, LLC. All Rights Reserved. www.tinybuild.com" in or close to the
  video. It forbids uploading the soundtrack alone or re-uploading tinyBuild's own videos, names no
  excluded game, has no date or version, and "may be subject to change". `fencevod` puts the notice
  in every description. Formal documentation: PR@tinybuild.com. No Questing Goose-specific
  streaming policy was found, and nothing on the Steam page addresses streaming.
- **Music.** The game's soundtrack is sold as a Steam DLC ("Probably Stolen Soundtrack"); the store
  page says nothing of its licence, its composer, or whether it is registered with Content ID, and
  no other source said either. tinyBuild's policy permits the game's sounds in videos, but a
  distributor's Content ID claim can still land on a VOD; it would be a claim to dispute with the
  policy, not a strike. To keep the risk small: stream with the game's music lowered or off (the
  narrator carries the audio anyway), never add outside music, and on a claim dispute it citing the
  policy URL above. Twitch mutes VOD segments on detected copyrighted music, another reason to
  upload the local master.
- Re-check both pages before the first upload; the policy is undated.

## 8. Rehearsal without the game

- `make stream-preflight` on the real rig (section 2).
- `go test ./cmd/fakemod -run TestLiveShowEndToEnd`: the live path end to end.
- A dress rehearsal (section 10): `scripts/rehearsal.sh` runs the bridge in its live configuration,
  fenceshow's live hook with the narrator and TTS, and one gym game slowed to a stream's pace by
  `cmd/pacer`, until the game ends.
- A full replay at stream speed with narration (CPU Kokoro shown; stop it afterwards). It narrates
  on the 9B, like `make show-demo`: a replay needs no gate and so must not take the 27B from a
  strategist that may be using it; for the stream's own voice, with nothing else on the 27B, set
  `FENCE_SHOW_LLM_MODEL=$DRIFT_MODEL`:

  ```sh
  docker start fence-show-kokoro
  FENCE_SHOW_OUT=var/show/demo-full FENCE_SHOW_TTS_URL=http://127.0.0.1:8881 \
  FENCE_SHOW_LLM_URL=$DRIFT_URL FENCE_SHOW_LLM_MODEL=$DRIFT_NARRATOR_MODEL FENCE_SHOW_SHOT_EVERY=60 \
    scripts/show-demo.sh var/league/t7-hard-drift/drift-delta__apprentice__s2100001__p-short_sleeper+vending 1-175 1
  docker stop fence-show-kokoro
  ```

## 9. Watchability: the full replay (2026-11-10)

One complete winning hard game, `t7-hard-drift/drift-delta__apprentice__s2100001` (qwen38fn-mlx,
Flash-Next, planning delta directives over the appraisal policy; buyout by mortgage on day 175), replayed
through fenceshow at speed 1, narration-paced, narrator qwen38fn-mlx (Flash-Next) on drift, CPU Kokoro
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

- Leagues stopped, and nothing else calling drift's 27B but the gate. drift stays in the fleet's
  agent pool (owner, 2026-11-12); instead the owner does not run omp, the pool's client, while
  streaming, so nothing reaches drift's 27B around the gate. Drift plays and narrates through the
  gate (section 1, "The drift gate"): the narrator is the 27B on `/narrate/v1` with `-llm-timeout 15s`
  (the owner's pick, 2026-11-12: in character as often as the 9B, 81 % against 82 %, in a clearly better
  voice, losing lines only while a morning plan holds drift; "The narrator's model"), the 9B directly
  as its first fallback, the API (Haiku) after that. Pre-flight GO on the narrator and drift gate rows
  (it now asks without thinking).
- The co-host (docs/SHOW.md, "The co-host") is off unless the owner wants it: `-cohost` on the same
  fenceshow adds Rook, a second voice that needles the player's decisions on the gate's `/filler/v1`;
  measured on the rehearsal with the strategist load, plans and the narrator's density did not move.
- CPU Kokoro by default (rehearsed: no backlog, a line in median 1.5 s, p90 10 s, max 15 s under
  load, nothing dropped for TTS); GPU Kokoro, 2–3 GB beside the game, is the owner's option (section 1).
- fenceshow: `-route sprinter` (or `-base-directives` matching the bridge's) so the plan box has the
  base policy; `-fill-after 40s`; the defaults for `-min-gap` (8 s), `-max-age` (15 s), `-max-lag` (25 s).
- Autopilot pace 800–1200 as in section 5; a slower pace only lengthens the silences.
- A fenceshow restart is safe mid-run (same command, same `-record`); expect the sparkline to restart.

## 11. Twitch viewer participation (built, not connected)

`cmd/fencetwitch` (docs/SHOW.md, "Twitch") runs beside fenceshow: predictions on the bets and the
format verdict, chat votes for the next morning's plan. Today it runs only as a **dry run**: it logs
the Helix calls it would make and plays chat from a script. The live Helix and EventSub clients are
stubs (`internal/twitch/helix.go`): they were not built because nothing here could test them without
credentials (the Twitch CLI's `twitch mock-api` is not installed on the game box). Nothing in the repo
holds a credential or contacts Twitch.

**Rehearse it** (no account needed):

```sh
# beside a running fenceshow (replay or live): a silent chat, the tally for the bridge in var/twitch/
go run ./cmd/fencetwitch -show http://127.0.0.1:7350 -chat-script internal/twitch/testdata/demo-chat.txt
# the bridge: the tally reaches the morning plan, which answers it in a "Chat:" line under RISK
make bridge ARGS="… -chat-tally var/twitch/chat-tally.json"
# a replay with synthetic bets, a scripted crowd and column screenshots, into var/twitch/demo/
scripts/twitch-demo.sh RUN_DIR 30-41 0.5
```

**What the owner sets up to go live** (once the live client is built):

1. **Channel.** Predictions need Channel Points, which need Affiliate or Partner status ("limited to
   partners and affiliates", https://dev.twitch.tv/docs/api/predictions).
2. **App.** Register an application in the Twitch developer console (https://dev.twitch.tv/console/apps):
   any name, category "Chat Bot" or "Broadcasting Suite", a redirect URL on loopback
   (`http://localhost:3000`), client type Confidential. Note its **Client ID**; keep the client secret
   with the token, never in the repo.
3. **Token.** A **user** access token of the broadcaster's own account (predictions are the
   broadcaster's; the EventSub WebSocket transport takes user tokens) with the scopes:
   - `channel:manage:predictions`: create, lock, resolve, cancel and read predictions (Helix
     `POST/PATCH/GET /helix/predictions`);
   - `user:read:chat`: read chat through the EventSub subscription `channel.chat.message` v1 (the
     replacement for IRC's `chat:read`), condition `broadcaster_user_id` and `user_id` both the
     broadcaster's id.
   Get it with the authorization-code flow (e.g. `twitch token -u -s 'channel:manage:predictions
   user:read:chat'` from the Twitch CLI), and keep the refresh token: user tokens expire after a few
   hours.
4. **Storage, outside the repo.** `~/.config/fence/twitch-token` (JSON with `access_token` and
   `refresh_token`, mode 600) next to `bridge.env`; the client secret in the same directory. fencetwitch
   refuses a token file inside its working tree.
5. **Environment** (`~/.config/fence/twitch.env`, loaded by the service or the shell, never committed):

   | Variable | |
   |---|---|
   | `TWITCH_CLIENT_ID` | the app's Client ID |
   | `TWITCH_BROADCASTER_ID` | the channel's numeric user id (Helix `GET /helix/users?login=…`) |
   | `TWITCH_TOKEN_FILE` | the token file above |
   | `FENCE_CHAT_TALLY` | where fencetwitch writes the vote (`-tally`, default `var/twitch/chat-tally.json`) and the bridge reads it (`-chat-tally`, off when unset) |
   | `FENCE_SHOW_URL` | fenceshow's base URL for fencetwitch (default `http://127.0.0.1:7350`) |

6. **Before the first stream:** build the live client against `twitch mock-api` (Twitch CLI; it serves
   `/helix/predictions` with fake ids and needs no account), then a private test on the real channel with
   a 30 s prediction, and run the dry-run demo once to see the panel's box in OBS.

**On stream night** (after this lands): start fencetwitch after fenceshow, with `-verdict=false` on a
format run that will not end tonight (the verdict would hold Twitch's one prediction slot all night and
then be refunded); `-lock-after 2m` suits a live day of 3–6 minutes. Stopping fencetwitch (Ctrl-C) cancels
an open prediction with refunds and sends an open vote. A restarted fencetwitch cancels any prediction it
finds still open on the channel (`Active`), since the bet's history went with the old process.
