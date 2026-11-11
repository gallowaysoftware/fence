#!/usr/bin/env bash
# Round-2 gym pilot: a distilled delta planner (fencedistill play -delta) against the executor
# alone, both over the same base policy, on fresh hard seeds. Without ENDPOINT it serves GGUF
# with its own llama-server and stops it on exit; it refuses to start one while the game runs.
#
#   tools/distill/gym-pilot.sh RUN GGUF [SEED...]
#   e.g. gym-pilot.sh qwen3-8b-r2 ~/models/distill/qwen3-8b-r2/qwen3-8b-r2-Q8_0.gguf
#   ENDPOINT=http://127.0.0.1:9000/v1 MODEL=name gym-pilot.sh RUN - [SEED...]   (an existing server)
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
run=${1:?run name}
gguf=${2:?GGUF path, or - with ENDPOINT}
shift 2
seeds=("$@")
((${#seeds[@]})) || seeds=(3100001 3100002 3100003 3100004 3100005 3100006)
base=${BASE_DIRECTIVES:-$repo/sim/lab/scripts/directives/hard_income_cash_appraisal.json}
gym=${GYM:-$repo/sim/gym/bin/Release/net10.0/fence-gym.dll}
out=${OUT:-$repo/var/sim/distill/$run}
port=${PORT:-18801}
par=${PARALLEL:-${#seeds[@]}}
model=${MODEL:-$run}

mkdir -p "$out"
[[ -f $gym ]] || nice dotnet build "$repo/sim/gym" -c Release
(cd "$repo" && go build -o "$out/fencedistill" ./cmd/fencedistill)

if [[ -z ${ENDPOINT:-} ]]; then
	if pgrep -f 'Probably Stolen' >/dev/null; then
		echo "Probably Stolen is running: not using the GPU" >&2
		exit 1
	fi
	# A prompt is ~10k tokens and an answer at most 400.
	llama-server --model "$gguf" --alias "$model" --host 127.0.0.1 --port "$port" --jinja --flash-attn on \
		-ngl 99 --parallel "$par" --ctx-size $((12288 * par)) >"$out/llama-server.log" 2>&1 &
	srv=$!
	trap 'kill $srv 2>/dev/null; wait $srv 2>/dev/null' EXIT
	for _ in $(seq 120); do
		curl -sf "http://127.0.0.1:$port/health" >/dev/null && break
		kill -0 $srv 2>/dev/null || { echo "llama-server exited; see $out/llama-server.log" >&2; exit 1; }
		sleep 2
	done
	ENDPOINT=http://127.0.0.1:$port/v1
fi

common=(run -days 1000 -difficulty hard -start apprentice -human player -perks vending+short_sleeper -directives "$base" -no-trajectory)
for s in "${seeds[@]}"; do
	[[ -f $out/executor-$s.json ]] || nice dotnet "$gym" "${common[@]}" -seed "$s" -summary "$out/executor-$s.json" >"$out/executor-$s.log" 2>&1
done
pids=()
for s in "${seeds[@]}"; do
	[[ -f $out/student-$s.json ]] && continue
	rm -rf "$out/student-$s"
	nice dotnet "$gym" "${common[@]}" -seed "$s" -step-dir "$out/student-$s" -step-timeout 900 \
		-summary "$out/student-$s.json" >"$out/student-$s.gym.log" 2>&1 &
	pids+=($!)
	"$out/fencedistill" play -delta -base "$base" -step-dir "$out/student-$s" -endpoint "$ENDPOINT" -model "$model" \
		-max-tokens 400 >"$out/student-$s.play.log" 2>&1 &
	pids+=($!)
done
((${#pids[@]})) && wait "${pids[@]}"

python3 - "$out" "${seeds[@]}" <<'EOF'
import json, sys
out, seeds = sys.argv[1], sys.argv[2:]
def res(p):
    try:
        j = json.load(open(p))
        return j.get("game_over") or "alive", j.get("days_played")
    except OSError:
        return "missing", None
print(f"{'seed':>8}  {'executor':<22}{'student':<22}")
for s in seeds:
    e, st = res(f"{out}/executor-{s}.json"), res(f"{out}/student-{s}.json")
    print(f"{s:>8}  {e[0] + ' d' + str(e[1]):<22}{st[0] + ' d' + str(st[1]):<22}")
EOF
