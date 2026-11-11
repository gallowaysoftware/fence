# T2 distillation: training a local strategist on strong play

Goal (docs/PLAN.md, "Training path", T2): a local model that writes the day's directives
like the strongest player we have. We fine-tune it on that player's decisions and, once
`sim/rollout` lands, on rollout-search winners.

Everything here lives outside git:
- data: `var/distill/`
- weights: `~/models/hf/`, `~/models/distill/`

## Pieces
| What | Where |
|---|---|
| Dataset builder, and a step-mode player for evaluation | `cmd/fencedistill` (`build`, `play`; round 2: `delta`, `play -delta`) |
| Prompt used for both training and inference | `strategy.DistillPrompt`; round 2: `strategy.DistillDeltaPrompt` and `FinishDistillDelta` (`internal/strategy/distill.go`) |
| Training, eval, merge, GGUF export | `tools/distill/` (uv project, pinned; `train.py`, `evaluate.py`, `merge.py`, `export.sh`, `pilot.sh`; round 2: `gym-pilot.sh`) |

## The task format
The league's strategist makes two calls:
1. A prose plan with ten headings.
2. In the same conversation, the directives JSON.

Claude's step-mode play has no prose plans, only directives and a note. We could invent the
missing plans, but that would teach the model text nobody wrote. So the distilled model
does the job in **one call**:
- **system:** the strategist's `planSystem` (all the mechanics), plus `styleRule` when a
  persona is set.
- **user:** `BuildPlanPrompt`, unchanged, rendered through the strategist's own code from
  the gym's `snapshot.json`. The final "Write the plan" line is replaced by the directives
  ask, with the same key list and the service, upgrade and permit ids. With `-gym`
  (default), it adds the gym executor's extension keys:
  - `engine`, `growth`, `upgrade_reserve`, `mortgage`
  - `buy.stock_cap`, `buy.restock_below`
  - `sell.liquidate_min_pct`, `sell.sell_hot`
- **assistant:** `WHY: <one line>`, then the directives as one compact JSON object.

Yesterday's facts come from the gym's `yesterday` result. Yesterday's plan is
yesterday's WHY line. Lessons and persona are empty.

**The bridge does not have this mode yet.** It still runs the two-call conversation, and
`protocol.Directives` drops the extension keys. Claude's strong play leans on those keys
(engine, mortgage, stock caps). So the first evaluation runs outside the bridge:
`fencedistill play` drives a gym `-step-dir` game, exactly as Claude played it
(see "Evaluating in the league").

## Dataset (`go run ./cmd/fencedistill build`)
Inputs:
- Every step-mode game under `-games` (default `var/play,var/director/wins`): any
  directory holding `day-001/snapshot.json`. Three layouts are read:
  - **Claude, per-day notes:** `day-NNN/notes.md` (claude-1, claude-2).
  - **Claude, game notes:** one `notes.md` for the game, with lines such as
    `- d36-42: … WHY: …` or `- day 45: JUDGMENT …`, plus `plan.json` (claude-3, p7/p8).
  - **Director:** `day-NNN/why.txt`, with the director's parameters in `summary.json`
    (`var/director/wins`).
- Rollout rows from `-rollouts`, default `var/rollout/*.jsonl` (see below).

Paths matching `-exclude`, default `attempt`, are skipped. These are abandoned restarts.

Per day:
- **Decision or routine.**
  - **Per-day notes.** Claude's standing-order scripts head each note with the policy in
    force: a first line such as `standing orders: …` or `mortgage plan: …`, or a
    `Decision: …` line. The policy is repeated day after day, with that day's numbers after
    ` | ` (first line) or `; ` (Decision line).
    - The first day a policy (seen on 3 or more days, or naming standing orders) appears is
      the day it was decided: a **decision**. Its repeats are **routine**.
    - A note copied unchanged from an earlier day is routine. Its rationale becomes
      "standing orders unchanged.", because the copied note refers to the wrong day.
    - A one-off note is a decision.
  - **Game notes.** The days a line covers are decisions when the line has `WHY:` or
    `JUDGMENT` and does not restate standing orders. The rationale is the WHY clause.
    Open ranges (`day 85+`) and result-only lines are not decisions. Every other day is
    routine, with `plan.json`'s standing orders as its rationale.
  - **Director.** Every day is routine: the director is a parameterised policy. The
    rationale is `why.txt` (often empty, giving "standing orders unchanged.").
- **Outcome.** `forward` is the change in net worth over the next 7 days (`-horizon`), as a
  fraction of the day's net worth (at least 500).
  - Dropped: days within 14 days (`-collapse`) of a game over.
  - Dropped: days whose next week lost more than 25 % of net worth (`-max-drop`).
  - Weight: `clamp(1 + forward, 0.5, 2)` × 2 for decision days, × 0.5 for routine days
    whose policy (directives minus `max_spend` and `cash_floor`) repeats yesterday's, and
    × `-director-weight` (default 1) for director days.
  - Games still running (no `final.json`) keep only the days whose horizon is complete.
- **Dedupe.** Exact (prompt, answer) duplicates are dropped.
- **Split.** Blocks of 14 days per game are hashed, and 1 block in 8 goes to val. This
  keeps neighbouring, near-identical days on the same side. `-val-games` holds out whole
  games instead.

Output:
- `var/distill/train.jsonl` and `val.jsonl`: one row per day, as
  `{"messages":[system,user,assistant],"weight":w,"meta":{source,game,day,difficulty,kind,repeat,net_worth,forward,ending,split}}`.
- `var/distill/stats.json`.

### Size (2026-11-10, 18 Claude games + 48 director wins)
| Source | Games | Endings | Decision days | Rows kept |
|---|---|---|---|---|
| claude-1 (normal 90001, hard 90002) | 2 | alive at 200, rent | 44 | 255 |
| claude-2 p1/p3/p4/p5 (normal) | 8 | 1 buyout (mortgage), rent ×3, repossession ×2, arrest, execution | 57 | 1,163 |
| claude-2 p2 (hard), p6 (water) | 4 | rent ×4 | 13 | 402 |
| claude-3 p7/p8 (hard, game notes) | 4 | rent ×3, arrest | 44 | 429 |
| director wins (24 at d9c4d40, 24 older) | 48 | buyout (mortgage) | 0 (all routine) | 6,869 |

- **Train:** 8,034 rows (107 decision, 1,827 Claude routine, 6,100 director); total weight
  7,352.
- **Val:** 1,084 rows (18 decision, 297 Claude routine, 769 director).
- **Dropped:** 224 before a collapse, 226 after a net-worth fall, 495 exact duplicates
  (mostly director games on the same seed under different director versions).
- **Length** (bytes/3.5): 5.8k–8.9k tokens per row (p50 7.6k). The answer is 21–274 tokens
  (p50 99). The Qwen3 tokenizer measured a 947-row subset at 5.4k–8.6k, with answers of
  59–505 tokens.

A buyout ending (`buyout`, `buyout_mortgage`) is a win. Its last days are not dropped as a
collapse, and a net-worth fall in it is not filtered (a mortgage's down payment is the
winning move). Claude's days in a won game are weighted ×1.5 (`-win-weight`). Director
games are all wins by selection, so their days weigh as routine.

Sources are in-process gym games only: Claude's step-dir games, the director, and rollout
rows later. The league's plan-mode logs are not used. They record no plans or directives,
and their games are confounded: the bridge answered the counter instead of the gym player.

The data is mostly policy. 75 % of rows are director days, and only 125 kept days of Claude's
carry fresh reasoning. That is enough to teach the format and a winning standing policy (the
director wins on normal). It is not enough to teach judgment. That has to come from rollout
rows, and from more decision-dense play.

### Rollout rows
One JSON object per line. The builder accepts:
- `snapshot` (required): the step-mode `snapshot.json` object, or the bare `day_snapshot`
  data.
- `best` or `directives` (required): the winning candidate.
- `day`, `game`, `rationale`: optional.
- `best_score` and `base_score`: optional. When both are present, a row whose best
  candidate does not beat the base is dropped. The weight is
  `1 + margin/|base|` (at most 2).

Rows become `kind: rollout` examples in the same format.

## Training recipe
`tools/distill/train.py` uses plain PyTorch, transformers and peft. It does not use TRL or
unsloth, because their APIs churn; the loop is short.
- **Loss** is on the answer tokens only: `logits_to_keep`, so the 150k-vocab head is never
  materialised for the 7k-token prompt.
- **Weights and compute:** each epoch keeps row i with probability
  `p_i = min(1, R·w_i/Σw)` and scales its loss by `(w_i/p_i)·(Σp/Σw)`. That gives the
  weighted gradient in expectation, while R (`--epoch-rows`, default the row count) bounds
  an epoch's compute. Heavy rows (decision days) are always kept; light ones (director and
  repeated routine days) are subsampled.
- **LoRA:** r 32, alpha 64, dropout 0.05, all linear layers (vision and MTP excluded).
- **Optimiser:** AdamW, lr 1e-4, cosine with 5 % warmup, effective batch 16 sequences
  (batch 1 × accumulation 16).
- **Epochs:** 2. Sequences up to 12,288 tokens (longer ones are dropped and counted).
- **Chat template:** rendered with `enable_thinking=False`, exactly as `evaluate.py` and
  the bridge (`Thinking: false`) prompt it.

Pins (`pyproject.toml`, Python 3.12, all released two weeks or more before use):
- torch 2.14.1 (cu130 index, x86_64 and aarch64 wheels)
- transformers 5.20.2, peft 0.21.2, accelerate 1.15.0, bitsandbytes 0.50.2 (x86_64 and
  aarch64 wheels), gguf 0.19.0
- huggingface_hub 2.2.0, via `uvx`, for downloads

### Recipe A: pilot on the 5090 (QLoRA, 8B)
- **Base: Qwen3-8B** (dense, `Qwen3ForCausalLM`), downloaded to `~/models/hf/Qwen3-8B`.
  Qwen3.8 has no small dense model: only 27B, the 180B Flash-Next MoE, and 2.4T. Qwen3-8B
  is the smallest current model with first-class transformers, peft and llama.cpp support.
- **Run:** `tools/distill/pilot.sh qwen3-8b-r1 --epoch-rows 2000 --epochs 1`.
  The full set is about 51M tokens per epoch, 5–10 h on the 5090.
  - It refuses to start while the game, a league (`fenceleague run`), a model loaded in
    llama-swap, or more than 2 GB of VRAM use is present. It never unloads anything.
  - It then runs `train.py --qlora`, `evaluate.py` on 200 val rows (every decision row plus a
    seeded routine sample; `EVAL_SAMPLE`), and `export.sh merged … Q8_0`.
- **Output:** `~/models/distill/qwen3-8b-r1/`, holding the adapter, `train_log.json`,
  `eval.json` (+ `.rows.jsonl`) and `qwen3-8b-r1-Q8_0.gguf`.
- **Estimates:**
  - VRAM: about 14–20 GB peak (4-bit weights about 6 GB, plus LoRA and optimiser, plus
    activations for about 8k tokens with gradient checkpointing).
  - Training: 1 epoch × 2,000 sampled rows × about 7.5k tokens ≈ 15M tokens at 1.5–3k
    tokens/s ≈ **1.4–2.8 h**.
  - Val generation (200 rows, about 150 tokens each, 4-bit HF generate): 15–25 min.
  - Merge, convert and quantize: about 10 min.
- **Not on the GPU while** the league runs on localmodel. A league model loaded beside
  training will run one or both out of memory.

### 5090 run log
- 2026-11-09 22:30 AST: **pilot not run.** llama-swap still held `qwen3.8-27b` (29.9 GB)
  after the league stopped, and nothing here unloads it.
- The pipeline was run end to end on the CPU with Qwen3-0.6B instead:
  - `train.py --limit 4`: 2 steps, val loss 1.21 → 0.77; the adapter saved.
  - `evaluate.py`, through both the HF+adapter path and a llama-server endpoint (identical
    answers).
  - `export.sh merged` (Q8_0) and `export.sh lora`, at llama.cpp b10797.

  An untuned model answers this prompt with a prose plan, not directives. `planSystem`
  still describes the plan format, so a base-model baseline through `DistillPrompt` is a
  weak one. Compare untuned models through the bridge's own two-call path.

- 2026-11-10 02:47–05:21 AST: **pilot r1** (`qwen3-8b-r1`).
  - **GPU:** the 27B was unloaded through llama-swap's `/unload` (the league's own route).
    The run used `ALLOW_LEAGUE=1`, because the only running league (t1-glm) used heavy.
  - **Training:** QLoRA on Qwen3-8B, `--epoch-rows 1000 --epochs 1`: 63 steps, about
    105 s per step (about 1.3k tokens/s), 1 h 56 min, peak 19.2 GB.
    - Val loss (96 rows): 2.126 → 0.684 (step 10) → 0.519 (30) → 0.455 (50) → **0.449** (63).
      Final train loss was about 0.3.
    - `--epoch-rows 2000` would have taken about 3.6 h.
  - **Played:** furn gym `3912d75`, `-start none`, served at 127.0.0.1:18800 by a standalone
    llama-server (`--parallel 6`), against `-human player` on the same seeds:

    | Seed | `-human player` | `qwen3-8b-r1` |
    |---|---|---|
    | normal 90023 | buyout (mortgage) d154, NW 10,771 | execution d206, NW 13,536 (peak 16,123) |
    | normal 90024 | buyout (mortgage) d175, NW 9,079 | execution d144, NW 4,174 |
    | normal 90025 | buyout (mortgage) d147, NW 10,418 | execution d200, NW 11,473 |
    | hard 90026 | arrest d133, NW 4,757 | arrest d111, NW 1,374 |
    | hard 90027 | arrest d80, NW 4,528 | arrest d134, NW 1,727 (peak 7,404) |
    | hard 90028 | arrest d140, NW 4,061 | rent d126, NW 1,966 |

  - **Why the model lost the normal games.** It copies the director's standing values
    nearly every day. On the three normal games, `"mortgage":"later"` was on 532 of 538 days and `"permit":"bribe"` on 539
    of 540. So it never takes the mortgage that `-human player` (mortgage unset, so auto)
    uses to win. Meanwhile the bribe route keeps building crime (1,390 by day 140 on 90024)
    until execution.
    - In the director's data, `later` is followed by a `take` on the offer day. The model
      learned the 99 % default, not the switch.
    - Decision days are about 2 % of rows and are subsampled at `--epoch-rows 1000`, so
      the rare switches are the signal that got diluted.
  - **Endpoint eval** (the same 200 sampled val rows, Q8 GGUF): JSON 0.97, agreement **0.633**
    (decision 0.38, routine 0.658), `changed_agreement` **0.163**. Copying yesterday scores
    0.886 overall (decision 0.735) and 0.107 on changed keys. The model is slightly better on
    the keys that changed, and much worse overall.
  - **Format failure:** on 90023 the model looped on `sell.hold_uids` (sequential uids) until
    it hit `max_tokens`. 94 days fell back to the base directives.
  - **Next run:**
    - Train on decision and policy-switch days with `-director-weight` well below 1, or
      drop director rows after the first day of each policy.
    - Add the mortgage/permit switch days from director logs as decision rows.
    - Cap list lengths in targets.
    - Serve with a repetition penalty, or raise `max_tokens`.

### Recipe B: heavy (bf16 LoRA, 27B, on a DGX Spark)
- **Base: Qwen3.8-27B (HF)**, the same model the league runs as `localmodel/qwen3.8-27b`.
  Its architecture is `qwen3_5`, a hybrid of Gated DeltaNet linear attention and full
  attention every 4th layer (64 layers, multimodal wrapper). That needs transformers ≥ 5.8
  (5.20.2 is pinned). `train.py` falls back to `AutoModelForImageTextToText` when the causal
  class does not map.
- **Memory:** bf16 weights are 55.6 GB. With LoRA (r 32 all-linear, about 0.3B parameters),
  its fp32 AdamW state, gradient-checkpointed activations at 8–10k tokens, and only the
  answer's logits, the peak is about **70–80 GB**, against about 118 GB usable on one GB10.
- **Stack on GB10:**
  - aarch64, CUDA 13.0, driver 580, sm_121.
  - torch 2.14.1+cu130 has manylinux aarch64 wheels, as does bitsandbytes 0.50.2 (not
    needed for bf16). transformers and peft are pure Python.
  - **To verify on the box before the window:**
    - `torch.cuda.get_arch_list()` covers sm_121 (or sm_120 PTX JIT works).
    - The `qwen3_5` linear-attention layers have a fast path. Without
      `flash-linear-attention`/`causal-conv1d`, transformers uses the slow torch path. fla
      is Triton (aarch64 wheels exist); causal-conv1d compiles (`MAX_JOBS=3`: 10 jobs
      OOM-killed a Spark).
- **Procedure:** GLM must be fully stopped (it fills both boxes), and the training box must stay
  under ~100 GB of unified memory (an overspend wedges it until a power cycle). The step-by-step
  stop/train/restore runbook has private host details and lives in `var/distill/HEAVY-RUNBOOK.md`
  (local only).
- **Time** (*est.*): at `--epoch-rows 2000`, one epoch is about 15M tokens × 6 × 27.8B ≈
  2.5×10¹⁸ FLOP. At about 40–60 TFLOP/s effective bf16 on one GB10, that is **12–17 h per
  epoch**. It is longer if the linear-attention fast path is missing. Both Sparks with DDP
  over the CX-7 pair would roughly halve it; this needs the NCCL environment block from
  the private fleet docs, and has not been tried for training. Plan an
  overnight window. Run Recipe A first: it shakes out the pipeline on hardware that costs
  nothing to stop.

### Heavy run log
- 2026-11-09 ~22:25 AST (UTC−4): **not run; GLM not stopped.** The knowledge pipeline had
  finished with GLM (commit 9990181), and the t2-plan league had stopped at 21:47. Even
  so, the owner's condition was "stop and report if anything is risky or undocumented", and
  several things were:
  - Training on the Sparks is undocumented in the fleet repo. It has no Python, uv or
    PyTorch setup, and the hosts must not be `apt upgrade`d.
  - Overspending unified memory wedges a Spark until someone power-cycles it by hand.
  - There is no HF Qwen3.8-27B checkpoint on the boxes, and a copy is about 56 GB.
  - The `qwen3_5` linear-attention training kernels on aarch64/sm_121 are unverified.
  - The pipeline itself had not yet run end to end on the 5090.

  Recipe B is the plan for a scheduled window, once Recipe A has run. Its smoke step (step 4)
  is the first thing to run on the Sparks.

## Export
`tools/distill/export.sh` checks out llama.cpp at `235f4e8`, the build of the local
llama-server, into `~/models/distill/llama.cpp`. It has two modes:
- `merged`: CPU merge (`merge.py`), then `convert_hf_to_gguf.py --outtype bf16`, then
  `llama-quantize` (Q8_0 by default).
- `lora`: `convert_lora_to_gguf.py`, for `llama-server --lora` on the base GGUF.

A llama-swap entry, modelled on the existing 27B entry:
- merged: `llama-server --model ~/models/distill/qwen3-8b-r1/qwen3-8b-r1-Q8_0.gguf --jinja --ctx-size 16384 --flash-attn on`
- LoRA: the existing 27B command plus `--lora …-lora.gguf`

Serve with thinking off; the targets have no reasoning trace.

## Evaluation
**Offline.** `evaluate.py` runs on val:
- `--endpoint` for anything llama-swap serves, including the untuned 27B as a baseline.
- `--hf/--adapter` for the local model.
- `--persist` and `--gold` for sanity checks.

It reports:
- `json_ok` and `why_ok`.
- `agreement`: the per-key match against the target directives, averaged over the target's
  keys.
  Enums and booleans must match exactly; numbers must be within 15 % or 25 credits; lists
  score by Jaccard, with service ids normalised.
- `per_key` and `extra_keys`.
- **`changed_agreement`**: agreement on only the keys where yesterday's directives would
  have been wrong today. On the pilot's 200-row val sample, copying yesterday scores 0.886
  overall (0.735 on decision days) but only **0.107** here (215 such keys). That is the number that shows judgment; overall
  agreement mostly measures the standing policy.

**In play.** This is what counts (owner: buyout or fail, capped at 1000 days). It uses the
same harness Claude played in.
1. Serve the GGUF in llama-swap (localmodel), only when the league is not using it.
2. Start the gym:
   `dotnet sim/gym/bin/Release/net10.0/fence-gym.dll run -human player -seed S -days 1000 -difficulty normal -step-dir var/sim/distill/<run>-S -step-timeout 900 -out … -summary …`
3. Drive it: `go run ./cmd/fencedistill play -step-dir var/sim/distill/<run>-S -model <swap-name>`.
   - Each day it writes `directives.json` (extension keys kept), `notes.md` (the WHY) and
     `answer.txt`.
   - An unusable answer gets one retry, then the base directives.
4. Compare on the league's seeds (90001–90030) that no training game used: Claude played
   90001, 90002, 90011, 90012 and 90015–90020. Compare against:
   - the executor alone (`player`)
   - `local-qwen-27b-plan` (the untuned 27B through the bridge)
   - Claude's games

   Metrics: ending mix, buyout rate and day, days survived, peak and final net worth. The
   step games' `final.json` and gym summaries carry them.
5. To run it inside `fenceleague`, the bridge needs a planner mode that:
   - asks `strategy.DistillPrompt` in one call;
   - reads the WHY as the day's plan;
   - passes the gym extension keys through to the gym.

   That is a separate change to `cmd/bridge` and `internal/protocol`.

Expert iteration (PLAN.md T2): the distilled model's own `play` games become new step-mode
games. Rollout search branches their decision days, the winners go back in through
`-rollouts`, and the model is retrained.

## Round 2: the delta planner (2026-11-11)

Round 1 trained the full-directives strategist and lost 6/6: it copied the director's
`"mortgage": "later"` and never signed. Since then the hard incumbent
(`hard_income_cash_appraisal.json`: appraisal board, cash buyout by day 140, mortgage fallback)
wins 60/60 on its own, and the LLM delta planners keep those wins (`docs/PROGRESS.md`). So round 2
trains the delta planner's job: name today's changes over the base, or answer `{}`. Copying `{}` is
the executor, which wins. The failure to avoid is a harmful change.

### Task
One call, `strategy.DistillDeltaPrompt`:
- **system:** `planSystem` (plus the route and style rules when set, as the bridge's plan call).
- **user:** `BuildPlanPrompt`, then the bridge's delta ask: the executor's rules, the keys, the base
  note (`-base-directives`) and the delta facts (service ids, upgrades, permit).
  - Yesterday's plan is yesterday's played changes, as one line: `changes: {…}`.
  - No facts line, scorecard, lessons or exemplars. A step game and the bridge's log cannot both
    reproduce them.
- **assistant:** the delta object, with `"why"` first. `{}` keeps the base.
  - A teacher day keeps its per-key reasons.
  - A hybrid day gives its one candidate reason as a string `"why"`, which the delta contract
    applies to every key.

There is no prose plan. The 27B's plan call writes about 700–850 tokens a day; here the median answer
is `{}` and the 95th percentile about 100 tokens. `strategy.FinishDistillDelta` plays an answer
through the bridge's delta lints (`finishDelta`).

### Dataset (`go run ./cmd/fencedistill delta`)
```
fencedistill delta -out var/distill/r2 \
    -hybrid var/league/h1-hybrid-hard,var/league/h2-hybrid-hard,<hybrid-v2 worktree>/var/league/h3-hybrid-cash,<hybrid-v2 worktree>/var/league/h3-hybrid-stress \
    -league 'var/league/t7-*,var/league/t8-*' -holdout 2100006,2800002,2310004,2300004,1300002
```
Only finished games are read. Every target is first normalised:
- reduced to the delta contract (`protocol.Directives`), without `day` or empty objects;
- stripped of values that restate the base exactly (Qwen's daily `engine: wine+furnace`).

**(a) Hybrid decisions** (`decisions.jsonl`). The target is the played candidate: the rollouts'
best once it clears the switch gate, else `keep`.
- Both the target and the delta in force are passed through the delta lints. The hybrid plays its
  candidates unlinted; the student's answers are linted. A change the lints undo is not a change.
- **Kinds and weights:**
  - `switch`, when the target differs from the delta in force: weight 3 (× 0.5 for a blind
    lever). Its reason is the proposer's; a blind lever, which has none, gets a fixed line.
  - `keep`: 0.5, with the reason the plan in force was adopted for.
  - `empty`, a `{}` that changes nothing: 0.1.
- **Dropped:**
  - decisions in the last 14 days of a lost game;
  - in a repossessed game, any decision that moves a mortgage key. The h2 finding: the rollout
    horizon ends before the repossession.

**(b) Teacher days**: plan-mode delta games that won. A day's delta comes from bridge.log's first
`msg=directives` line for that day: the delta after the lints, with the model's reasons. Its
prompt uses the morning `day_snapshot` from the decision log.
- A key without a reason (its own, or `"*"`) is dropped, as the bridge would drop it.
- **Kinds and weights:**
  - `change`, when the delta differs from yesterday's: weight 2. Numbers within 15 %, or within
    25 credits from 100 up, count as the same.
  - `repeat`: 0.5.
  - `empty`, `{}` after `{}`: 0.1.
- **Pace.** Every day of a game is weighted by its pace against the executor alone on the same
  seed and base: the executor's win day over this game's, clamped to [0.5, 2].
  - 2100004, drift's day-91 cash buyout: 1.93.
  - 2800001, where the executor took the mortgage and won on day 230: Qwen 2.0, drift 2.0, GLM 1.84.
  - 2800002, where the executor won on day 83: Qwen 0.80, GLM 0.70.

**(c)** The `empty` rows of both sources: the executor's own play.

**Split.** Every row of a held-out seed is val. 1300002 had no finished game, so 4 seeds hold out
in practice.

### Size (2026-11-11 03:40 AST)
| Source | Base | Games | Train rows (weight) | Val rows |
|---|---|---|---|---|
| hybrid, h3-cash (Qwen) | cash appraisal | 4 (buyout ×4) | 88 | 30 |
| hybrid, h1/h2/h3-stress (Qwen, GLM, blind) | `hard_income_mortgage` | 25 (rent, repo, arrest, execution: all lost) | 800 | 201 |
| teacher, t7 (Qwen ×6, drift ×5, GLM ×2) | mortgage appraisal | 13 (buyout_mortgage ×12, cash ×1) | 1,614 | 286 |
| teacher, t8 (Qwen ×2, drift ×1, GLM ×2) | cash appraisal | 5 (buyout ×5) | 290 | 194 |

| Kind | Train rows | Train weight | Val rows |
|---|---|---|---|
| teacher `change` | 1,150 | 2,684 | 278 |
| hybrid `switch` | 209 | 597 | 46 |
| `keep` / `repeat` | 166 | 88 | 29 |
| `empty` (c) | 1,267 | 142 | 358 |
| **all** | **2,792** | **3,511** | **711** |

- **Dropped:**
  - 86 hybrid decisions before a collapse;
  - 29 mortgage moves in repossessed games;
  - 81 duplicates;
  - 1 lost teacher game (drift 2100005);
  - 26 unfinished games: t7 seeds 007–012, which never finished, and the still-running t8 and
    h3 games. Rebuild once they finish.
  - Linting turned 131 of the 340 train switches into `keep` or `empty` rows.
- **Changed keys** (switch and change rows): services 622, buy 532, sell 433, upgrades_buy 286,
  bargain 252, mortgage 211, scavenge 139, permit 53, augs 52, engine 30,
  mortgage_save_from_day 27, buyout_by_day 21, and fewer than 15 each for the rest.
- **Length** (bytes/3.5): prompts 7.0k–10.2k tokens (p50 8.8k); answers p50 1, p95 100, max 253.
- **Caveats:**
  - The teacher's changes rarely moved an outcome. On t7, paired against the executor, Qwen's
    were +1.2 ± 2.3 days. Most `change` days churn services, buy, sell and bargain, and pace weighting
    is the only outcome signal in them.
  - 90 % of the hybrid rows come from the stress base, whose games all lost. There the rollouts'
    28–42-day judgement is the only evidence.

**Offline baselines** on val (`evaluate.py`). A delta is scored over both answers' keys: a change
gold did not make counts 0, and `{}` against `{}` is 1.
- `--empty` (the executor), all 711 rows: agreement 0.677. By kind: change 0.371, switch 0.435,
  empty 1.0, keep/repeat 0.
- `--empty`, `--sample 200`: agreement 0.585 (change 0.395, switch 0.5). The sample now caps the
  decision-like rows at two thirds (133 of the 324 change and switch rows) and fills the rest from
  `{}`/keep/repeat rows, so false changes on quiet days are measured too.
- `--gold`: 1.0.

`--persist` is not meaningful here: hybrid rows are a week apart and teacher days have gaps.

### Ready to run (the 5090 must be free)
1. `DATA=var/distill/r2 tools/distill/pilot.sh qwen3-8b-r2 --epoch-rows 1200 --epochs 1`
   - About 2.3 h at r1's 1.3k tokens/s (1,200 sampled rows × about 9k tokens).
   - Per epoch at this budget, it keeps about 68 % of `change` rows at pace 1, every LLM-proposed
     `switch`, half the blind ones and 3 % of `empty` rows.
   - It then evaluates the adapter on the 200-row val sample and exports `qwen3-8b-r2-Q8_0.gguf`.
   - It refuses to start while a league runs, llama-swap holds a model, or the GPU uses more
     than 2 GB.
2. Offline comparisons on the same 200 rows (`--sample 200`):
   - `evaluate.py --empty`.
   - r1: its GGUF under a standalone `llama-server`, with `--endpoint`. r1 never saw this prompt,
     so it is a format floor, not a peer.
   - The teacher: Qwen-27B on its 9091 server, `--endpoint http://127.0.0.1:9091/v1 --model
     qwen3.8-27b`, only when no league is using it.
3. `tools/distill/gym-pilot.sh qwen3-8b-r2 ~/models/distill/qwen3-8b-r2/qwen3-8b-r2-Q8_0.gguf`
   - It plays the 8B as the delta planner on hard seeds 3100001–006 over the cash-appraisal base,
     each seed beside the executor alone.
   - Since this was written, `hard_income_cash_sprint.json` is the hard incumbent (median win day
     98, against 112 for the cash-appraisal base). To pilot over it, set `BASE_DIRECTIVES`. Its
     `buyout_save` keys are not in the delta contract, so the base note shows them without a
     training row having seen them.
   - It serves the GGUF with its own `llama-server` (6 slots, 12k context each) and stops it on
     exit.
   - Results go to `var/sim/distill/qwen3-8b-r2/`: the gym summaries, and per day `answer.txt`,
     `notes.md` (reasons, the played changes, the lints' fixes) and `directives.json`.
   - The harness ran end to end against a stub endpoint on seed 3100099: both arms bought out on
     day 97.
4. Checked on the CPU: `train.py --limit 4` on 4 r2 rows with Qwen3-0.6B ran 4 steps, and the
   r2 rows' val loss fell from 3.54 to 1.57.

### 5090 run log (round 2)
- 2026-11-11 03:30 AST: **not trained.** The standalone Qwen-27B on 9091 held 27.3 GB of the
  5090, serving:
  - `t8-hard-llm` (localmodel slots 0–2);
  - the persona-routes `r1-routes-qwen` league;
  - `h3-hybrid-cash`.

  At 06:00 the persona-routes league still held localmodel slots 0–1 and the GPU still used
  29.4 GB, so the run stopped after the dataset. Nothing here stops a league's model.

