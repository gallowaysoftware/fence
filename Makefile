GAME_DIR ?= $(HOME)/.local/share/Steam/steamapps/common/Probably Stolen
export GAME_DIR

# drift: the roaming MacBook serving Qwen3.8-Flash-Next through MTPLX. It may be offline;
# the bridge then defers every decision.
# Machine-specific endpoints live in local.mk (untracked); see local.mk.example.
-include local.mk
DRIFT_URL ?= http://localhost:9002/v1
DRIFT_MODEL ?= qwen38fn-mlx
DRIFT_FLAGS = -policy llm -base-url $(DRIFT_URL) -model $(DRIFT_MODEL) -no-schema -thinking off -temperature 0.2

.PHONY: knowledge knowledge-status data-status data-export service-install service-status service-logs service-uninstall bridge-drift-act test test-go test-mod mod mod-install compat compat-proposed bridge bridge-drift fakemod eval eval-drift backup-saves

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

.PHONY: gym-build gym-test gym-smoke gym-bench gym-run gym-bridge

gym-build:
	dotnet build sim/gym -c Release

gym-test:
	dotnet test sim/gym.tests -c Release

gym-smoke: gym-build
	$(GYM) smoke -seeds 20 -days 30 $(ARGS)

gym-run: gym-build
	$(GYM) run $(ARGS)

gym-bridge:
	go run ./cmd/bridge -listen $(GYM_BRIDGE_ADDR) -log-dir var/sim/log -memory-db var/sim/memory.db $(GYM_BRIDGE_FLAGS) $(ARGS)

# In-process (built-in heuristic), then HTTP against a throwaway heuristic bridge.
gym-bench: gym-build
	$(GYM) bench -seed 7 -days 300 -human prudent
	mkdir -p var/sim/bench && go build -o var/sim/bench/bridge ./cmd/bridge
	var/sim/bench/bridge -listen $(GYM_BRIDGE_ADDR) -log-dir var/sim/bench/log -memory-db var/sim/bench/memory.db $(GYM_BRIDGE_FLAGS) >var/sim/bench/bridge.out 2>&1 & \
	  pid=$$!; sleep 1; $(GYM) bench -seed 7 -days 300 -human prudent -bridge http://$(GYM_BRIDGE_ADDR); status=$$?; kill $$pid; exit $$status
