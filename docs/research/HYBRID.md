# Hybrid planner: the LLM proposes, the gym decides

Delta directives (`internal/strategy/delta.go`) let the LLM change the executor's plan, but each
change is accepted or reverted by fixed lints, so the best the LLM can do is match the executor (it
does, within noise: `docs/PROGRESS.md`, 02:50). The hybrid planner removes the lints from the loop:
at a decision point the LLM proposes alternatives, and forked rollouts of the real game rules
(`sim/rollout`, `docs/research/ROLLOUT.md`) choose among them. Every decision is logged with the
rollout scores, which is the training data for distillation round 2: train only on choices the
rollouts showed were better.

## Loop

1. **Decision points** (`sim/rollout/Hybrid.cs`, read off the main line each morning):
   - `review`: at least one decision every 7 days;
   - `mortgage_offer`: a rent morning (`dayUntilRent` 0, before the loan shark's visit) with cash
     ≥ 2 down payments, not yet paying; `mortgage_window`: cash first past 1.5 down payments;
     `payment_due`: paying, ≤ 3 days to a payment, cash + repossessable stock under 1.5 payments;
   - `georgio`: day 41–43 before his day-43 loan; `permit`: day 36 (the official from day 38) and
     the morning cash first covers fee + rent + 500;
   - `investigation`: evidence up 15 since the last decision, or total crime into a higher band
     (100 / 300 / 850 / 1 200);
   - `windfall`: cash up ≥ max(800, 50 %) since yesterday morning;
   - `shortage`: a market rise ≥ 30 % within 3 days (once per item type a week);
   - `upgrade`: a Wild upgrade affordable for the first time.

   An event decides at most once per 2 days, except the mortgage offer, Georgio and a payment due,
   which always do. One decision is one LLM call, so the count is roughly days / 7 plus events.
2. **Candidates**: `keep` (the delta in force), `base` (the run's base policy, when keep differs) and
   up to 3 proposals. Each proposal is a complete delta over the base with a one-line reason.
   - `-hybrid llm`: the model's (`internal/strategy/propose.go`). It sees the morning state step
     mode shows a player, the run's risks (mortgage numbers, Georgio, permit, record), the base
     policy, the delta in force, the triggers and the executor's rules and keys. Proposals are parsed
     like delta directives (`parseDirectives`) plus the executor's extensions (`georgio`,
     `georgio_peak_days`, `repo_reserve_payments`, …), with no lints: a bad proposal loses its
     rollouts. Only `lintBaseEngine` stays (a restated `wine` would switch off the base's furnace).
   - `-hybrid blind` (the ablation): 3 candidates drawn from the rollout search's one-lever set
     (`Candidates.Build` + `Candidates.Mortgage`, ~15–20 levers), seeded by (seed, day).
3. **Rollouts**: fork the main line once per (candidate, sample). A branch plays the candidate for
   7 days, then the base policy, to the horizon: 28 days, or 42 inside the mortgage window (cash ≥
   one down payment, or paying). A **commitment** decision rolls to the end of the mortgage term
   instead (weeks × 7 + 14 days, 126 on hard; `-no-commit-payoff` turns it off): before signing, with
   cash past one down payment and the offer not declined, when any candidate (the plan in force
   included) sets `mortgage`, `mortgage_window`, `mortgage_margin`, `mortgage_float`, `mortgage_cover`,
   `mortgage_save_from_day`, `buyout`, `buyout_by_day` or `buyout_check_from`. A 42-day window ends
   before the repossession a signing buys (`docs/PROGRESS.md`, hybrid on the hard stress test), so
   the rollouts saw the equity, not the default. Branches still end early at a buyout or a loss.
   Each sample re-seeds the game's generators (no hindsight); sample m has the same seed for every
   candidate (common random numbers).
4. **Score** per branch, mean over samples: the rollout search's score (a buyout 20 000 − days;
   another ending −10 000 + 100 × days survived; else 10 000 × (net worth + mortgage equity − rent
   shortfall) / buyout price) plus a risk term at the horizon: −40 × (evidence − 40) above evidence
   40 (a quarter of that under 300 crime, no arrest), −3 000 while paying with cash + repossessable
   stock under one payment, and, for a branch cut mid-term, the repossession its trend projects: when
   cash + repossessable stock, falling at the net-worth trend since signing (7–28 days), run out
   before the last payment, the branch scores as a loss on that day.
5. **Choice**: the best mean among `base` and the candidates whose paired gain over `base` is at least
   `-switch-se` (1.0) standard errors (the max of noisy means otherwise wins too often). The plan in
   force is gated like any proposal, so a held delta must keep beating the base policy, and going back
   to base needs no gate. The chosen delta is held until the next decision.
6. **Log**: one JSONL row per decision (`-rows`, `decisions.jsonl` in a league match): the snapshot,
   triggers, delta in force, every candidate with source, reason, directives, per-sample rollouts,
   score and paired z against `keep` and `base`, the horizon and whether the decision was a commitment,
   the argmax, the played candidate, and the proposer's raw answer, latency, repair and dropped
   proposals.
7. **Show**: with `-decision-log FILE` the main line also writes its decision log in the bridge's
   format (`log/hybrid.jsonl` in a league match), and each decision adds a `show` line carrying the
   feed's `candidates` event (`sim/rollout/ShowLog.cs`): every candidate rolled, its mean score as
   buyout progress in % (score / 100) ± its standard error, the proposer's reason and the futures'
   mean net worth, and a caption naming the verdict ("considered A / B / C; 4 simulated futures say
   B"). It is a moment (level 2) when the plan changes or the trigger is the mortgage offer,
   Georgio or a payment due, else notable. Each option carries its delta and the proposer's whole
   reason, and the replay shows the chosen one as today's plan. `fenceshow -replay` plays the match
   like a league game (`docs/SHOW.md`, "Hybrid games").

## Running it

```
dotnet build sim/rollout -c Release
fenceleague propose -dir DIR/propose -base-url http://127.0.0.1:9091/v1 -model qwen3.8-27b &
dotnet sim/rollout/bin/Release/net10.0/fence-rollout.dll play -seed 1300001 -difficulty hard -start apprentice \
    -perks vending+short_sleeper -directives sim/lab/scripts/directives/hard_income_mortgage.json \
    -hybrid llm -propose-dir DIR/propose -samples 4 -par 4 -rows DIR/rows.jsonl -summary DIR/summary.json \
    -decision-log DIR/log/hybrid.jsonl
```

In a league, a player with `"mode": "hybrid"` runs fence-rollout instead of fence-gym (the gym
worktree from `fenceleague gym` must have `sim/rollout` built); with an `endpoint` the league
answers its proposals through the proxy (calls counted in `llm.jsonl`), without one it is the blind
ablation. Proposals run with thinking off unless the endpoint sets `"thinking": "on"`: a proposal is
capped at 1 500 completion tokens, which GLM spent thinking. The request carries a JSON schema
(`response_format`, where the server takes it); an answer that still does not parse has its brackets
and strings closed locally, then gets one repair call that shows the model its answer and the error
(42 of the 47 failed answers in h2 parse after the local pass). `rollout_flags` pass `-samples`, `-par`, `-horizon` and the rest. A hybrid match does not
hold an endpoint slot: the proxy's slots cap its requests.

Flags: `-hybrid llm|blind`, `-propose-dir`, `-decision-log`, `-propose-timeout 600`, `-blind 3`, `-review 7`,
`-cooldown 2`, `-horizon 28`, `-window-horizon 42`, `-switch-se 1.0`, `-no-commit-payoff`, `-samples`, `-par`, `-perks`.
