# sim/search

`search.py` (policy search, docs/research/POLICY-SEARCH.md) and `board.py` (a win table for a study).

## Idle runner

`idle_runner.py` keeps otherwise idle CPU busy with gym work while LLM games wait on inference. Every
gym process and build runs under `chrt -i 0 nice -n 19` (SCHED_IDLE: it gets only CPU nobody else wants),
at most 28 at a time (`FENCE_IDLE_WORKERS`), and only while `MemAvailable` is at least 8 GB. While
`Probably Stolen.exe` runs it launches nothing and lets running episodes finish.

```
python3 sim/search/idle_runner.py start    # detaches (setsid); pidfile var/idle/runner.pid, log var/idle/runner.log
python3 sim/search/idle_runner.py status   # phase, incumbent, last scoreboard lines, rounds, promotions
python3 sim/search/idle_runner.py stop     # SIGTERM to the pidfile's pid; kills its own gym processes
```

**Builds.** Each new `main` HEAD is built from a clean detached worktree (`~/.cache/fence-idle/`) into
`var/sim/gyms/search-<commit>/`, never over a build in use. Commits that leave `sim/gym`, `sim/headless`,
`mod/src` and `compat` unchanged share a build (`var/idle/builds.json`). A commit that fails to build is
skipped and the search keeps the last good build.

**Hard policy.** `HARD` in `idle_runner.py` is the hard incumbent as a (start, perks, directives) tuple:
`-start apprentice -perks vending+short_sleeper` with `sim/lab/scripts/directives/hard_income_cash_sprint.json`
(read from each commit). Changing it needs a runner restart.

**Scoreboard.** On every new HEAD: hard with the `HARD` policy and normal with the executor's defaults, seeds
**990001–990060** each. One line per commit in `var/scoreboard/log.jsonl` (wins, median days, endings; per-seed
results in `var/scoreboard/runs/`); `hard_policy` is a hash of the tuple and the directives' text, and
`hard_policy_spec` spells it out. A commit with the same build and hard policy as an earlier one reuses its
results (the gym is deterministic per seed). `regression` flags a hard median score drop (score below) of more than 2 bootstrap
standard errors (paired seeds) against the last line **with the same hard policy** (a line with a new policy
starts a new baseline), or a drop of 4+ normal wins against the previous line.

**Hard search.** Rounds of local search around the incumbent (`var/search/hard/incumbent.json`: directives
plus gym flags such as `perks`; seeded from `HARD`, and reseeded when `HARD` changes, the old one going to
`incumbents.jsonl`) over the build commit's `space-hard.json`, re-read every round, so levers added
to the space file and committed are searched from the next round. Each candidate redraws 1–3 levers
(weighted). Round *r* uses its own seed block 3 000 000 + 1000·*r*: 14 candidates and the incumbent on
seeds +1…+12, the better half on +1…+24, then up to 3 that beat the incumbent there are played on the
60 confirmation seeds +501…+560 together with the incumbent. The score is survival days, a win (or a run
alive at day 1000) counting as 1000, plus for a win the days it left before day 1000 (a day-105 win scores
1 895, a day-175 win 1 825), so an earlier win breaks a tie. A finalist is promoted when its paired Δ score
is above 2 standard errors and it wins at least as often, or when its paired win difference alone is above
2 standard errors.
Promotions go to `var/search/hard/promotions.jsonl` and `promoted/round-NNNN.json` (directives) and
replace the incumbent. Nothing is committed: review them and commit a policy by hand. Every round is
logged to `rounds.jsonl`; episodes are cached per build in `evals.jsonl`, so a stopped round resumes.

Reserved seeds: 990001–990060 (scoreboard) and 3 000 001 upward (search rounds).
