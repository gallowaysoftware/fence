# The show layer

What a viewer of the stream sees and hears besides the game window: a panel with the AI's
plan, its reasoning and the shop's money clock, and a narrator speaking the big moments in
the player's voice. It is built and tested against gym games so it is ready when the live
game is. The live game itself is another agent's work (`docs/STREAM.md`); this layer only
reads what the bridge already writes.

```
 bridge decision log (JSON lines) ──┐            ┌──> panel (OBS browser source, SSE)
 bridge /v1/summary (plan prose)  ──┼─> builder ─> hub ─┤
 bridge.log, gym summary (replay) ──┘    ^              └──> narrator ─> LLM line ─> TTS WAV ─> hub (say)
 other producers ── POST /v1/ingest ─────┘
```

- `internal/show`: the feed schema, the builder (decision log → events), the hub (state,
  SSE fan-out, recorder, scoreboard), replay and live producers, the narrator and TTS.
- `cmd/fenceshow`: serves the panel on `127.0.0.1:7350` and runs one producer.
- `scripts/show-demo.sh` / `make show-demo`: replays a finished hard league game into
  `var/show/demo/`.

## Rule: player-visible only

Every event is built from the decision log's `data` and `context` objects. The decoder
types have no `hidden` field, so a hidden value (true item values, aug flags, presence) can
never reach the feed; `TestBuilderDay` plants a marker in every `hidden` object and fails
if it appears. Item names come from the game at run time and are never committed.

## Feed schema (v1)

One JSON object per line on disk (`-record`), one SSE message on the wire. Every event:

| Field | |
|---|---|
| `v` | schema version, 1 |
| `seq`, `at` | hub sequence and publish time (unix ms) |
| `ts` | when it happened in the source (decision-log time) |
| `run`, `day` | run id (`<player>:<run_id>`), game day |
| `kind` | below |
| `level` | 0 ticker, 1 notable (panel highlight), 2 moment (the narrator may speak) |
| `text` | a complete caption; every consumer can fall back to it |
| `clock` | when the event changes money: `cash`, `net_worth`, `rent_due`, `rent_days`, `arrears`, `grace`, `owned`, `buyout_price`, `mortgage_weekly`/`mortgage_weeks_left`, `offer_down`/`offer_weekly`, `weekday`. While a mortgage runs its weekly payment falls on the rent day in place of the rent (`rent_days` counts to it) |
| `beat` | a stream beat the narrator is told about (below), absent on the rest |

| Kind | Payload | From |
|---|---|---|
| `run_start` | `player`: `id`, `model`, `persona`, `persona_name`, `persona_tagline`, `route`, `route_brief`, `difficulty`, `start`, `perks`, `seed`, `planner` | `run_start` event + match.json |
| `plan` | `plan`: `day`, `source` (`llm` prose, `directives` only, `executor` when the model changed nothing), `headline` (≤ 4 lines from GOALS/CASH/RISK), `sections`, `directives` (in words), `changes` vs the previous plan, `change`, `why` (the model's reasons, "bargain: …", then the bridge's corrections, "overruled: …"), `goals` (the standing plan: the last prose goal, mortgage savings, rent cover, buyout progress, the base directives' engine and permit) | `/v1/summary` (live); bridge.log (replay) |
| `candidates` | `candidates`: `day`, `metric`, `options[]`: `label`, `source`, `directives`, `score`, `stderr`, `rollouts`, `chosen`, `note` | the hybrid planner, a `show` line in its decision log (or `/v1/ingest`) |
| `thought` | `hidden_used` (names only) when an autopilot rule read a hidden value; the panel marks it ◆ | a customer and the executor's reason (gym), a model's own rationale (advise/act), services, outings, concealment, every live autopilot action with its reason |
| `autopilot` | `autopilot`: `state` (`paused`, `running`, `faulted`, `handover`), `reason`, `needs_human` | the live autopilot paused or resumed by the player (notable), or stopped and waiting for a human (moment) |
| `trade` | `trade`: `side`, `items` (grouped names), `count`, `cash`, `estimate`, `cost`, `profit`/`has_profit` (sold: vs what was paid; bought: estimate − price), `customer`, `faction`, `wholesale`, `fakes` (items whose shown name says counterfeit) | barter decisions + `trade` outcomes |
| `purchase` | `amount` | upgrades (with the Wild favor share), the permit |
| `money` | `amount` | rent paid/missed, Georgio's loan and payback, mortgage, bribes |
| `heat` | `heat`: `visible`, `evidence`, `level` ("14/100"), `operation`, `officer`, `sentence`, `crimes`, `hot_items` (stolen items on hand) | the security record in each snapshot |
| `aug` | `amount` | shot an aug (bounty), a wrongful kill, a wanted capture |
| `day` | `summary`: `cash_delta`, `net_worth_delta`, `sold`/`sold_items`, `bought`/`bought_items`, `customers`, `rep` (faction → change) | `day_ended` + the close snapshot |
| `run_end` | `end`: `ending`, `day`, `cash`, `net_worth`, `won` (a buyout) | `run_end` |
| `say` | `say`: `line`, `audio` (URL path or absent), `voice`, `for` (the moment's seq), `source` (`llm`/`caption`), `check` (`pass`, `retry`, `fallback`), `faults` (the first try's), `ms` (LLM time for the line), `mood` (`MoodOf` when it spoke) | the narrator |

Levels are the loudness contract. Moments (level 2) are: the first plan and plans that change
the run (permit, mortgage, engine, upgrades, cash floor, augs, selling down), trades of 500+ or a
twentieth of net worth (whichever is more), sales of 300+ profit or a fortieth of net worth, any
loss of 200+, a counterfeit sold, upgrades and the permit, Georgio, missed rent, heat
operations/officers/sentences and evidence crossing a tens mark, aug shootings, the weekly
day summary or a net-worth day of ±600 or a twenty-fifth of net worth, the run's start and end,
and every beat. The bars rise with net worth so a rich shop's routine sale is not narrated daily.

**Beats** (`beat`): `appraisal_join` and `appraisal_promotion` (the board's dialogue: the gym
names the speaker in the choice's history and the new title, "Master Appraiser"; the live
autopilot names the dialogue by client id), `mortgage_signed` (down payment, weekly, weeks),
`mortgage_payment` (each one, with the weeks left; the first, every fourth and the last two
are moments, the rest notable), `buyout` (the won run's end,
or a cash buyout), `aug_confronted` (a shot), `handover` (the autopilot stops for its human:
"Over to the human: …") and `autopilot_back` (the first resume after it). The gym's appraisals
are ticker lines with the estimate and fee; the gun coming out is a notable line, marked ◆
(`aug_truth`) when the run's `aug_detection` is `perfect`, which no player has.

**Small beats** (`internal/show/smallbeats.go`) are the everyday moments between the big ones, so
a 3–6 minute live day is not one line and minutes of silence: `counter` (a haggled deal or a
notable trade, at most two a day and three customers apart; the caption adds the haggle:
"…for 127 (estimate 150), after a haggle (demand discount 10%): accepted."), `night` (what the
outing brought home, from the sleep snapshot's new items against the close snapshot's: "Night at
the dumping ground, 5 scavenges: back with Quality Module and Scrap Metal (estimate 74)." or
"Night at Dr. Jackson's: back with nothing."), `morning_plan` (the day's first notable plan,
raised to a moment; later changes that day stay notable), `milestone` (net worth crossing 25,
50 and 75 % of the buyout price, once each a run) and `appraisal_streak` (the third accepted
appraisal in a row and every fifth; a refusal starts over). Their numbers are in the caption, so
the fact check covers them, and the narrator may tell them twice as late as `-max-age` and
`-max-lag` allow the rest: a night out is still news 30 s on, a rent payment is not. The dress
rehearsal's 98 days have 409 moments instead of 81: 127 counter, 92 morning plan (10 plans were
moments before), 85 night, 32 streak and 3 milestones.

## Producers

**Replay** (`-replay MATCH_DIR`): reads a `cmd/fenceleague` match directory: the decision
log (`log/*.jsonl`, required), `bridge.log` for each day's directives and corrections,
`gym.summary.json` for the executor's per-customer reasons, `result.json` for the board. It
paces events (`-speed`; a hard day runs about 25 s at speed 1) and can play a window
(`-days 40-44`; the earlier days still set the clock, names and plan). Replays are
**narration-paced** by default: after each plan/money/heat/aug/purchase moment the replay
waits (≤ 60 s) until the narrator has spoken it, and the narrator's age and lag limits rise
to 90 s, since the replay owns its clock.
`-convert` writes the whole feed to stdout instead. `-recorded-pace` (with `-replay` or `-feed`)
holds each event as long as the source did (decision-log `ts`, a feed's `at`; gaps capped at 3
minutes), divided by `-speed`: a rehearsal's decision log replays at stream pace, which is how
the narrator's density is measured offline (with `-narration-paced=false`, so the narrator's
live limits apply).

Since b2f1da0 the bridge logs the prose plan (`plan=` on "day plan written") and the
model's reason for each change (`why=` on the directives line), so replays show the
model's plan and its reasons. Older league logs have the directives only (badge "model
directives"). A day on which the model changed nothing shows the standing plan's goals
(badge "standing plan"): "Saving for the mortgage: 4,120 / 6,000", "Rent 353 in 6 days:
covered, 228 to spare", "Buyout: net worth 3,726 / 24,000", the base directives' engine
and permit (read from the bridge's `-base-directives`). A feed recorded live (`-record`)
has the prose and replays with `-feed FILE`.

**Live hook** (`-live-log DIR -bridge URL`, or `-live-match MATCH_DIR` for a running league
match): no bridge change is needed. fenceshow tails the newest decision log in the bridge's
`-log-dir` (a new file is a new session; a new `run_id` in a session is a new run) and polls `GET /v1/summary?session_id=…`
every 2 s, publishing a `plan` event whenever the prose or directives change. On attach it
reads the existing log silently and publishes only the latest plan, heat and day, so the
panel is current without replaying the backlog. Checked against a running t6 hard league
match: real prose plans came through with GOALS/CASH headlines.

**Ingest** (`POST /v1/ingest`, one event or JSON lines; `kind` and `text` required): for
producers that know more than the logs. A slate from the hybrid "LLM proposes, gym decides"
planner looks like this (fence-rollout writes the same event into its own decision log; see
"Hybrid games"):

```json
{"kind":"candidates","day":41,"level":2,"text":"Three plans simulated over 24 rollouts; the cautious one keeps 9 more days.",
 "candidates":{"day":41,"metric":"days survived","options":[
   {"label":"cautious: floor 600, no upgrades","source":"llm","score":212.5,"stderr":3.1,"rollouts":24,"chosen":true},
   {"label":"buy Seller Referral now","source":"llm","score":203.4,"stderr":3.4,"rollouts":24}]}}
```

**Hybrid games** (`docs/research/HYBRID.md`). A hybrid match has no bridge: fence-rollout plays
the game and, with `-decision-log` (always in a league match: `log/hybrid.jsonl`), writes the main
line's decision log in the bridge's format, so `-replay` reads it like any league match. Each
decision adds a line `{"type":"show","show":{…}}` holding a finished `candidates` event, which the
builder passes through in order: the panel's "Alternatives considered" box shows every candidate
rolled (the plan in force first, then the base plan, then the proposals), its mean score as buyout
progress in % over the rollout horizon ± its standard error, and the rollout count, the chosen one
marked ▸; each option's `note` holds the proposer's reason and the futures' mean net worth. The
caption says what was weighed and why it was asked ("Day 43 (georgio): considered keep the plan /
lend half / no loan; 4 simulated futures say lend half (…)"). A decision is a moment (the narrator
may speak it) when it changes the plan or answers the mortgage offer, Georgio or a payment due;
a weekly review that keeps the plan is notable only. The narrator's fact check knows the slate's
numbers. A hybrid replay has no bridge plan: each slate is followed by a `plan` event (notable, the
slate is the moment) for its chosen candidate, badge "simulated choice", with the delta in words and
the proposer's reason as its why; when base is chosen it is the standing plan.

Demo (`var/show/hybrid-demo/`: feed, transcript, piper narration, panel screenshots): days 40–44
of `h2-hybrid-hard/hybrid-qwen27__apprentice__s2300001` (Qwen-27B proposing, the old hard policy
`hard_income_mortgage.json`), captions only:

```sh
FENCE_SHOW_OUT=var/show/hybrid-demo FENCE_SHOW_LLM_URL= \
  scripts/show-demo.sh var/league/h2-hybrid-hard/hybrid-qwen27__apprentice__s2300001__p-short_sleeper+vending 40-44 2
```

It shows three slates: day 40 (a Wild upgrade became affordable) goes back to the base plan; day
41 (Georgio) weighs lending 50 % against 25 % and plays "georgio 25 stable"; day 43 plays "georgio
early repay" at 25.2 ± 0.9 % against 22.0 ± 0.6 for the plan in force and 23.5 ± 0.5 for the base,
four futures each. Drop `FENCE_SHOW_LLM_URL=` to have the narrator speak them in persona. The
caption names the plans weighed and the chosen reason only while it fits 200 characters (the
narrator's cut; otherwise "considered 5 plans", no reason); the plan box
shows it whole (up to four lines), and proposers are asked for at most 20 words.

## The panel

`fenceshow` serves one page in three layouts, sized for a 1920×1080 canvas with the game
captured at 1360×765 in the top-left corner:

| URL | Size | Shows |
|---|---|---|
| `/` (or `/full`) | 1920×1080 | the column and the lower third around a transparent hole for the game |
| `/column` | 560×1080 | name card, day + cash + rent pips + net worth/buyout bar + sparkline, heat, today's plan, alternatives (when posted), thinking ticker, runs board (moments only with `?moments=1`: they live in the lower third) |
| `/lower` | 1360×315 | the narrator caption and the last three moments |
| `/about` | 560×1080 | the "about this AI" card alone: who plays, the fairness rule (player-visible information only; the ◆ count this run), how the plan, the hands and the narrator split the work, and the rig (`-setup "line;line"`) |

Query flags: `?audio=0|1` (default on for `/` and `/column`, off for `/lower`, so only one
source plays the voice), `?about=SECONDS` (the about card takes the ticker's place for 22 s every
480 s by default and at each run start; 0 turns it off), `?demo=1` paints the game hole, `?snap=1` draws once from `/state`
without holding a stream open (headless screenshots). The page is hand-written JS and CSS
embedded in the binary: no CDN, no fonts fetched, works offline. Animations are a fade-in for
new lines, a number tween for cash, a soft pulse when the plan changes and a breathing live
dot. State is server-side, so an OBS reload or a late browser source draws the current run at
once.

### OBS setup

1. Run fenceshow (below) on the streaming machine.
2. Scene, canvas 1920×1080. Add the game capture (Game Capture / PipeWire window capture of
   Probably Stolen); Transform → Edit: position 0,0, size 1360×765.
3. Add a **Browser** source: URL `http://127.0.0.1:7350/`, width 1920, height 1080, put it
   above the game capture. Tick **Control audio via OBS** (the narrator's audio then appears
   as this source in the mixer); untick "Shutdown source when not visible".
   Alternatively use two browser sources, `/column` (560×1080 at 1360,0) and `/lower`
   (1360×315 at 0,765), and enable audio on one only.
4. Optional: Advanced Audio Properties → add a little delay to the game audio if the voice
   should not talk over it, or a compressor on the browser source.
5. Refresh the browser source (or not: the page reconnects by itself) when fenceshow restarts.

## The narrator

Separate from the decider. It watches the hub for moments (level 2), turns one into a line
in the player's persona and voice, and publishes a `say` event with the WAV the panel plays.

- **Rate.** One pending moment at a time: a louder or newer one replaces it (run end >
  run start > aug/heat/money/purchase > plan/candidates > trade > day). At least `-min-gap`
  (8 s) of silence after each line's audio. A moment older than `-max-age` (15 s) is skipped,
  and in live mode a line ready later than `-max-lag` (25 s) after its moment is dropped: the
  voice never runs behind the game; it just says less. A handover outranks everything but the
  run's start and end. After `-fill-after` (60 s) of silence a notable event may be spoken
  too, so a slow live day is not dead air; filler never displaces a waiting moment.
- **Character.** Every run has a persona: the match's (`-persona` in its bridge args), or
  `-persona`, or `-default-persona` (magnate). The name card shows its name and its path in
  a few words ("as Shark · Nothing but nerve"). The narrator speaks as it: a fixed Kokoro
  voice and a speaking style per persona (`internal/show/persona.go`: shark am_fenrir, a
  cocky street hustler; magnate bm_george, an unflappable old-money operator; …), with the
  persona file as background for motive. `-voice` overrides the voice. A run on a route
  (`docs/research/ROUTES.md`, the match's `-route` or fenceshow's `-route`) also puts the route
  on the name card ("smelter route · Trash into metal") and gives the narrator the route's
  brief, so the same persona sounds different on a metal shop and a wine cellar.
- **Mood.** The narrator's character follows the money (`MoodOf`, from the latest clock and the
  last day's net-worth change): *triumphant* once the shop is owned, *broke* when the next rent or
  mortgage payment is not covered (sweat, gallows humour), *tight* under twice it, *flush* at 60 %
  of the buyout price or six payments in hand (swagger), else *steady*; a big up or down day adds
  a word. The mood goes in the system prompt without numbers, so it cannot break the fact check.
- **Beats.** A beat's moment carries a cue saying what kind of moment it is ("the appraisal board
  just promoted you", "say plainly that it is over to the human, and why"); a small beat's cue
  asks for the telling ("a customer at the counter: who they were, how the haggle went", "the
  morning: what you mean to do today, in one sentence"). A milestone ranks with money moments,
  a night or a streak with trades.
- **Line.** The LLM gets the moment's caption, the plan's headline/changes/reasons/goals,
  the moment's **facts** spelled out (`FactsOf`: cash; the rent, whether the cash covers it
  and by how much, "You are NOT short on rent"; the day's net and cash change in words;
  the sale or buy amount and profit), the last four lines, the phrases to rest and one fresh
  image (below). One or two
  sentences, ≤ 28 words, at most one number. The rule: invent colour, motive and character
  freely; the numbers are sacred.
- **Fact check.** `CheckLine` reads every number in the line (digits or words: "five
  hundred eighty one", "a hundred and thirty") and its claims, and checks them against the
  facts: a number must be one the moment gave (rounded within a tenth); "short" on the rent
  when the cash covers it, or "covered" when it does not; "down" on an up day; "sold for N"
  against the sale; "I sold" on a buy; the rent due "tomorrow" when it is six days out. A
  failing line is asked for once more with its faults named; a second failure speaks the
  caption. `say.check` records which, so a demo or a stream log gives the pass rate.
- **Variety.** Each persona keeps a phrasebook for the run (`internal/show/variety.go`): a line
  that reuses a three-word phrase of its last 20 lines (two words of colour, within a clause) or a
  four-word one of its last 40, leans on
  an image of its last 10 in any wording (the Shark's "the till", "blood", "pocket", "whisper",
  …, and each image of its bank), or opens with the same two words as one of its last 6 is a
  fault like a wrong number, asked for once more; a retry that still repeats speaks the caption.
  Words of the moment's own caption and facts are news, not habits ("the dumping ground", an
  item's name). The prompt lists what to rest (the recent images most used first, the recent
  three-word phrases, the last openings) and offers one image from the persona's bank for its
  mood (three to six each for flush or triumphant, tight or broke, and steady), in rotation and
  never one just used.
  With `-record` on a live source, a restarted fenceshow reads the run's spoken lines back, so a
  restart no longer clears the memory.
  Measured with drift's qwen38fn-mlx on t5-hard-drift/drift-delta__apprentice__s970001:
  `make show-demo` (days 38–46) 7/8 lines passed first time, the 8th on the retry
  ("Georgio's seventy-three" for a 730 loan), none fell back; days 2–37 without TTS 20/20.
  LLM time per line: median 4.7 s and 7.1 s, worst 15 s. The check reads numbers, not
  fractions or deeds: "half of it a favor" (it was all of it) and "the deed is signed" on a
  rented shop pass.
- **Model.** `-llm-provider auto` (the default) uses the Anthropic API (`-anthropic-model`,
  default claude-haiku-5-5) when `ANTHROPIC_API_KEY` is set, else the local model at
  `-llm-url` (OpenAI-compatible), else captions. `openai`, `anthropic` or `none` force one.
  The key comes from the environment only.
- **Degrades.** LLM down, slow (8 s live, 30 s replay) or yielding to the game (only a model
  served on this machine yields: an OpenAI-compatible URL on loopback) → the caption is the line. Kokoro down → piper (`~/.local/bin/piper`, en_US-lessac-medium) →
  no audio, caption only. Nothing ever blocks the feed.
- **Sharing the GPU.** `-yield-to-game` (on) checks every 30 s for `Probably Stolen.exe` and
  stops calling a narrator model served on this machine while it runs; the API and a remote node
  keep narrating in character (the stream's plan: `docs/STREAM-OPS.md`). Point `-llm-url` at drift (`DRIFT_URL` in `local.mk`;
  `make show-demo` does) or at an API; a narrator line is ~400 prompt tokens and ~40 out, one
  every 15–30 s. Do not point it at llama-swap on 127.0.0.1:9000 by a model name that is not
  already loaded: llama-swap would load a second 27B next to the one the leagues use.

### Measured: density and variety (2026-11-11)

The dress rehearsal's decision log (docs/STREAM-OPS.md section 10: seed 2950001, the Shark on the
sprinter route, a game day ≈ 2 minutes) replayed through `-replay … -recorded-pace
-narration-paced=false -fill-after 40s`, so every event lands when it did live and the narrator works
under its live limits: Qwen 3.8 27B (thinking off, one request at a time) and CPU Kokoro `am_fenrir`.
Measured from the recorded feed: lines a minute; voice on air (WAV length over wall time); silence
between one line's audio and the next; the fact check (first try / retry / caption fallback); the
most repeated three-word phrases.

| Run | Code | Load | Lines/min | On air | Silence median / p90 / max | Fact check | Most repeated 3-grams |
|---|---|---|---|---|---|---|---|
| Live rehearsal, days 23–98 | main | live | 0.81 | 6.9 % | 67 / 121 / 149 s | 89 / 21 / 8, 4 timeouts | "in the till" 14, "rent is a" 8, "in my pocket" 7 |
| Replay, days 30–41 | main | light | 0.79 | 6.2 % | 61 / 129 / 140 s | 15 / 3 / 0 | none twice; openings "credits for" 3 |
| Replay, days 30–41 | phrasebook + small beats | light | **1.62** | **13.4 %** | **11** / 100 / 123 s | 29 / 6 / 2 | none twice |
| Replay, days 30–41 | main | heavy | 0.65 | 6.6 % | 93 / 136 / 138 s | 13 / 2 / 0 | "in my pocket" 3, "upper levels pay" 2 |
| Replay, days 30–41 | + clause grams, late small beats | heavy | **1.53** | **14.5 %** | **20** / 86 / **94** s | 31 / 4 / 0 | eight phrases twice ("i let him", "in the bag", "worn out spacer"), none three times |

"Light" is an idle machine (a line made in ~2 s); "heavy" is 60 gym processes on the CPU, when a line
took 5–15 s to make (CPU Kokoro, mostly) and moments waiting for it passed the 15 s `-max-age`. Two intermediate
runs under heavy load, before the last two fixes, spoke 1.05 and 0.87 lines a minute: one sent 4 of
24 lines to the caption over grammar ("i let him", "a row the board" across a full stop), and in
both most small beats aged out behind a 13 s line. Phrases are now read within a clause with
pronouns as stop words, and small beats may be twice as late (`-max-age`, `-max-lag`) as money and
the gun. The target, 1.5–2 lines a minute at 2 minutes a day without a phrase coming back a third
time, is met in both conditions. Lines by moment in the last run: counter 14, night 8, morning
plan 6, appraisal streak 2, and 5 from the old moments.

Transcript: the first 15 lines from day 30 of each light replay.

Before (main), days 30–39:

| Day | Moment | Line |
|---|---|---|
| 30 | purchase | Wild takes a cut of the showcase. Cash covers rent easy. |
| 31 | trade | Upper Levels pay for pleasure. Four hundred thirty-nine credits just for pouring wine. |
| 31 | aug_confronted | 119 credits for the bounty. That aug is gone. |
| 32 | plan | Rent is three days out. I got two thousand four hundred seventy-nine clear. |
| 32 | trade | Upper Levels bleed credits for cheap vices. 354 for smoke and wine. |
| 33 | trade | 355 credits for water and a care package. Upper Level servants pay premiums for basic survival. |
| 34 | day | Rent is due tomorrow. I got 3971 in the drawer. |
| 35 | trade | Spacer coughed up 175 for the wine. Easy blood. |
| 35 | day | 789 fresh in the till. The landlord can wait, I am not breaking a sweat yet. |
| 36 | trade | Another spacer drained. 163 in the pocket. |
| 37 | trade | 163 more. This spacer had no idea he was feeding a shark. |
| 38 | money | 2,000 credits for a permit? I am a business now, baby. |
| 38 | day | 1,500 gone in a day. Still got enough to choke the landlord in four days. |
| 39 | trade | Another spacer bled dry. 138 more in the jar. |
| 39 | aug_confronted | Shot that aug right at the counter. He thought he was an inventor, but he was just a mark. |

After (phrasebook + small beats), days 30–35:

| Day | Moment | Line |
|---|---|---|
| 30 | morning_plan | Morning. I’m flipping that hydroponic rig for credits, then hitting the dump for scrap. |
| 30 | purchase | Seven fifty for the glass case. I’m circling slow, patient. |
| 31 | trade | Four hundred seventy-five for three bottles. The upper levels are easy marks when they get thirsty. |
| 31 | aug_confronted | Eleven niner bounty for a fake inventor. The gun’s cold, but the credits are hot. |
| 32 | morning_plan | Day 32 plan: Cash end of day >= 2479 (rent floor) |
| 32 | counter | Upper levels drunk on cheap wine, paid three fifty. Easy meat, no fight. |
| 32 | counter | Georgio blinked, sweat on his brow. I took a ten percent cut. |
| 32 | night | Three scavenges turned up nothing but rust. Back with water tablets, worth about thirty-seven. |
| 33 | counter | Upper Level servant, stiff collar, wanted Eleanor's care package. Haggled him down to three fifty-five. |
| 33 | counter | Farmer tried to stiff me. I laughed in his face and snapped the crate shut. |
| 33 | appraisal_streak | Forty straight appraisals. The Board pays my rent while I sleep. |
| 34 | morning_plan | Day 34 plan: Pay rent 292 on day 35: cash ≥ 292 before closing |
| 34 | counter | Old man blinked, I didn't. Six crates, sixty credits, pocketed. |
| 34 | night | Dr. Jackson’s gave me nothing but a headache. |
| 35 | counter | Spacer stumbled in, smelling like burnt ozone. I held the wine, he paid 175, and I let him walk free. |

Two of the after lines got a number past the check: "Eleven niner" (119) and "Three scavenges" (5);
"niner" and a night's small counts are checked since (`ff64e14`, `3aee01a`). The two caption lines
are retries that still repeated: "then hitting the" (grammar, not a repeat since `ff64e14`) and "the
board pays" (a real one, from the streak line before).

### TTS setup

Kokoro is the same service llm-economic-domination's video pipeline uses
(`tools/produce/render.py`, `cmd/voice`): Kokoro-FastAPI, OpenAI-compatible
`/v1/audio/speech`, image `ghcr.io/remsky/kokoro-fastapi-gpu`, model Kokoro-82M and its
voices (already on this machine with the image; the demo downloaded nothing).

- On the GPU (fast: well under a second a line, ~2–3 GB VRAM): `vibe start tts_kokoro`, or
  `docker compose -f ~/.config/vibe/compose/kokoro-tts/docker-compose.yaml up -d`; it listens
  on 127.0.0.1:8880, fenceshow's default. Fine on stream night if no local LLM fills the 5090;
  not while the leagues' 27B holds 27 GB of it.
- On the CPU (no VRAM; 6–20 s a line depending on load), on another port so it never
  collides with vibe's:
  `docker run -d --name fence-show-kokoro -p 127.0.0.1:8881:8880 --cpus 8 -e USE_GPU=false
  -v kokoro-tts_kokoro-models:/data -e HF_HOME=/data/hf ghcr.io/remsky/kokoro-fastapi-gpu:latest`
  and `-tts-url http://127.0.0.1:8881`. This is what the demo used.
- Audio goes out as one WAV per line under `-audio-dir` (default `var/show/audio`, gitignored
  with `var/`), served at `/audio/` and played by the panel; OBS captures it from the browser
  source. That needs no virtual audio device and keeps voice and captions in one place.

## Running it

```sh
make show-demo                                    # replay days 38–46 of a finished hard game into var/show/demo
make show-demo ARGS="var/league/<l>/<match> 40-44 1"
go run ./cmd/fenceshow -replay var/league/<l>/<match> -days 40-60 -speed 1.5
go run ./cmd/fenceshow -feed var/show/feed.jsonl          # replay a recorded live feed
go run ./cmd/fenceshow -live-match var/league/<l>/<match> -record var/show/feed.jsonl
go run ./cmd/fenceshow -live-log var/log -bridge http://127.0.0.1:7341 \
   -player drift -model qwen38fn-mlx -persona shark -llm-url "$DRIFT_URL" -llm-model qwen38fn-mlx
```

then open `http://127.0.0.1:7350/?demo=1`. The scoreboard lives in `var/show/board.json`
across runs; `-board-seed 'var/league/<l>/*'` adds finished matches to it.

## Gaps

- Fakes caught by a buyer are not in the logs (the game blocks the sale; no outcome says
  why), so only counterfeits named on the counter show.
- The live real game without the autopilot has no executor reasons; the ticker then shows
  customers, outcomes and any model rationale (act/advise), which is thinner. With the autopilot
  (docs/STREAM.md) every action carries the executor's reason as a `thought`.
- League games played before b2f1da0 replay with directives, not prose or reasons.
- The live hook reads plans from `/v1/summary`, which has no reasons; live plans show the
  bridge's prose and directives, and reasons only on replay.
