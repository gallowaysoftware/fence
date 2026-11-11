GAME_DIR ?= $(HOME)/.local/share/Steam/steamapps/common/Probably Stolen
export GAME_DIR

# drift: the roaming MacBook serving Qwen3.8-Flash-Next through MTPLX. It may be offline;
# the bridge then defers every decision.
# Machine-specific endpoints live in local.mk (untracked); see local.mk.example.
-include local.mk
DRIFT_URL ?= http://localhost:9002/v1
DRIFT_MODEL ?= qwen38fn-mlx
DRIFT_FLAGS = -policy llm -base-url $(DRIFT_URL) -model $(DRIFT_MODEL) -no-schema -thinking off -temperature 0.2

.PHONY: show-demo knowledge knowledge-status data-status data-export service-install service-status service-logs service-uninstall bridge-drift-act test test-go test-mod mod mod-install compat compat-proposed bridge bridge-drift fakemod eval eval-drift backup-saves

test: test-go test-mod

test-go:
	go vet ./...
	@test -z "$$(gofmt -s -l .)" || { gofmt -s -l .; exit 1; }
	go test -race ./...

test-mod:
	dotnet test mod/Fence.sln -c Release

mod:
	dotnet build mod/Fence.sln -c Release

mod-install:
	mod/install.sh

# Rebuild after a new snapshot: the mod embeds compat/versions/ to recognise verified builds.
compat:
	dotnet run --project mod/tools/Fence.Compat -c Release

compat-proposed:
	dotnet run --project mod/tools/Fence.Compat -c Release -- --check-only --hooks compat/hooks.proposed.json

bridge:
	go run ./cmd/bridge $(ARGS)

bridge-drift:
	go run ./cmd/bridge -mode advise -provider openai $(DRIFT_FLAGS) $(ARGS)

bridge-drift-act:
	go run ./cmd/bridge -mode act -provider openai $(DRIFT_FLAGS) $(ARGS)

eval:
	go run ./cmd/fenceeval $(ARGS)

# Live: calls drift, so it is never part of `make test`.
eval-drift:
	go run ./cmd/fenceeval $(DRIFT_FLAGS) $(ARGS)

fakemod:
	go run ./cmd/fakemod -bridge http://127.0.0.1:7341 -fixtures protocol/fixtures

backup-saves:
	scripts/backup-saves.sh

data-status:
	go run ./cmd/fencedata status $(ARGS)

data-export:
	go run ./cmd/fencedata export -out var/export $(ARGS)

SERVICE = fence-bridge
SERVICE_DIR = $(HOME)/.config/systemd/user
SERVICE_ENV = $(HOME)/.config/fence/bridge.env

# bin/fence-bridge-svc, not bin/bridge: rebuilding the manual bridge must not swap the
# binary under the running service.
service-install:
	go build -o bin/fence-bridge-svc ./cmd/bridge
	mkdir -p $(SERVICE_DIR) $(dir $(SERVICE_ENV))
	sed 's|@REPO@|$(CURDIR)|g' deploy/$(SERVICE).service > $(SERVICE_DIR)/$(SERVICE).service
	test -e $(SERVICE_ENV) || cp deploy/fence-bridge.env.example $(SERVICE_ENV)
	systemctl --user daemon-reload
	systemctl --user enable --now $(SERVICE)
	systemctl --user restart $(SERVICE)

service-status:
	systemctl --user status $(SERVICE) --no-pager

service-logs:
	journalctl --user -u $(SERVICE) -f

service-uninstall:
	-systemctl --user disable --now $(SERVICE)
	rm -f $(SERVICE_DIR)/$(SERVICE).service
	systemctl --user daemon-reload

# Strategy-knowledge pipeline: community sources -> claims (GLM on the heavy cell) ->
# merged, code-checked clusters -> docs/knowledge/. Resumable: every stage caches under
# var/knowledge/. FENCE_DECOMP must point at the decompiled Assembly-CSharp.
FENCE_DECOMP ?= $(CURDIR)/var/decomp
export FENCE_DECOMP

knowledge:
	go run ./cmd/fenceknow all $(ARGS)

knowledge-status:
	go run ./cmd/fenceknow status $(ARGS)

# ---- sim/gym: the real game headless, playing the mod's side of the protocol ----
# A sim bridge for HTTP runs: its own port, log dir and memory DB, so sim data never mixes into
# the live bridge's (:7341, var/log, var/memory.db).
GYM = dotnet sim/gym/bin/Release/net10.0/fence-gym.dll
GYM_BRIDGE_ADDR ?= 127.0.0.1:7399
GYM_BRIDGE_FLAGS ?= -mode act -policy heuristic

.PHONY: gym-build gym-test gym-smoke gym-bench gym-run gym-bridge lab gym-arms

# MOD_SRC=dir builds against another checkout of mod/src (e.g. a pinned snapshot while mod/ is mid-edit).
GYM_BUILD_FLAGS = $(if $(MOD_SRC),-p:ModSrc=$(MOD_SRC))

gym-build:
	dotnet build sim/gym -c Release $(GYM_BUILD_FLAGS)

gym-test:
	dotnet test sim/gym.tests -c Release $(GYM_BUILD_FLAGS)

# The baseline human role, then the prudent one (sim/gym/README.md, "Human-role defaults").
gym-smoke: gym-build
	$(GYM) smoke -seeds 20 -days 30 -human default -dir var/sim/smoke-default $(ARGS)
	$(GYM) smoke -seeds 20 -days 30 -human prudent -dir var/sim/smoke-prudent $(ARGS)

gym-run: gym-build
	$(GYM) run $(ARGS)

# sim/lab: every mechanics experiment, one child process each; results in var/lab/results.jsonl.
lab: gym-build
	dotnet build sim/lab -c Release $(GYM_BUILD_FLAGS)
	dotnet sim/lab/bin/Release/net10.0/fence-lab.dll all -n 20000 $(ARGS)

# Paired strategy arms: ARMS is a file of `name | gym smoke args` lines (sim/lab/scripts/arms.py).
gym-arms: gym-build
	python3 sim/lab/scripts/arms.py run $(ARMS) -dir $(DIR) $(ARGS)
	python3 sim/lab/scripts/arms.py report $(DIR)

gym-bridge:
	go run ./cmd/bridge -listen $(GYM_BRIDGE_ADDR) -log-dir var/sim/log -memory-db var/sim/memory.db $(GYM_BRIDGE_FLAGS) $(ARGS)

# In-process (built-in heuristic), then HTTP against a throwaway heuristic bridge.
gym-bench: gym-build
	$(GYM) bench -seed 7 -days 300 -human prudent
	mkdir -p var/sim/bench && go build -o var/sim/bench/bridge ./cmd/bridge
	var/sim/bench/bridge -listen $(GYM_BRIDGE_ADDR) -log-dir var/sim/bench/log -memory-db var/sim/bench/memory.db $(GYM_BRIDGE_FLAGS) >var/sim/bench/bridge.out 2>&1 & \
	  pid=$$!; sleep 1; $(GYM) bench -seed 7 -days 300 -human prudent -bridge http://$(GYM_BRIDGE_ADDR); status=$$?; kill $$pid; exit $$status

# ---- T0 league: the scoreboard (cmd/fenceleague). Machine config in var/league/local.json
# (untracked; see league/local.example.json); results under var/league/<name>/.
LEAGUE_SPEC ?= league/t0-first.json
LEAGUE = var/league/bin/fenceleague

.PHONY: league league-status league-report league-probe league-gym league-bin

league-bin:
	go build -o $(LEAGUE) ./cmd/fenceleague

league: league-bin
	$(LEAGUE) run -spec $(LEAGUE_SPEC) $(ARGS)

league-status: league-bin
	$(LEAGUE) status -spec $(LEAGUE_SPEC) $(ARGS)

league-report: league-bin
	$(LEAGUE) report -spec $(LEAGUE_SPEC) $(ARGS)

league-probe: league-bin
	$(LEAGUE) probe $(ARGS)

# A clean, built gym worktree of HEAD (ARGS="-overlay ../fence-strategy-lab" adds that
# worktree's uncommitted sim/gym changes); put the printed path in local.json's gym_dir.
league-gym: league-bin
	$(LEAGUE) gym $(ARGS)

# The show layer (docs/SHOW.md): replay a finished league game through the panel and the
# narrator into var/show/demo. The narrator calls drift lightly; SHOW_LLM_URL= for captions.
SHOW_LLM_URL ?= $(DRIFT_URL)
SHOW_LLM_MODEL ?= $(DRIFT_MODEL)
show-demo:
	FENCE_SHOW_LLM_URL=$(SHOW_LLM_URL) FENCE_SHOW_LLM_MODEL=$(SHOW_LLM_MODEL) scripts/show-demo.sh $(ARGS)

# The stream's go/no-go table (docs/STREAM-OPS.md, "Pre-flight"): every automatable check, exit 1 on
# a NO-GO. ARGS="-backup" also backs the saves up; endpoints come from the environment, the bridge's
# ~/.config/fence/bridge.env, or flags (-strategist-url, -narrator-url, -tts-url, -show, -persona).
.PHONY: stream-preflight
stream-preflight:
	go run ./cmd/preflight $(ARGS)
