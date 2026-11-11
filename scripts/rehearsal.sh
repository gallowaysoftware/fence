#!/usr/bin/env bash
# Dress rehearsal (docs/STREAM-OPS.md, "Dress rehearsal"): the whole live stack — the bridge in its
# live configuration, fenceshow's live hook with the narrator and TTS — driven by a gym game slowed
# to a stream's pace by cmd/pacer. Stops everything it started on exit.
#
#   STRATEGIST_URL=… STRATEGIST_MODEL=… NARRATOR_URL=… NARRATOR_MODEL=… scripts/rehearsal.sh [SEED]
#
# Env: ROUTE (sprinter), PERSONA (shark), DAY (open hours per game day, 40s), TTS_URL (CPU Kokoro),
# GYM_DLL (a built fence-gym.dll), OUT (var/rehearsal/<date>), PLAYER (the board's player id).
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD
seed=${1:-2950001}
: "${STRATEGIST_URL:?an OpenAI-compatible base URL ending in /v1}" "${STRATEGIST_MODEL:?}"
route=${ROUTE:-sprinter} persona=${PERSONA:-shark} day=${DAY:-40s}
out=${OUT:-var/rehearsal/$(date +%F-%H%M)}
gym=$(realpath -m "${GYM_DLL:-sim/gym/bin/Release/net10.0/fence-gym.dll}")
[[ -f $gym ]] || { echo "no gym at $gym: make gym-build, or GYM_DLL=…" >&2; exit 1; }
mkdir -p "$out/bin" "$out/log" "$out/show" "$out/gym"
for c in bridge fenceshow pacer; do go build -o "$out/bin/$c" "./cmd/$c"; done

pids=()
trap 'kill "${pids[@]}" 2>/dev/null || true; wait 2>/dev/null || true' EXIT
nice -n 5 "$out/bin/bridge" -listen 127.0.0.1:7361 -mode observe -policy heuristic -planner llm -directives delta \
	-route "$route" -persona "$persona" -provider openai -base-url "$STRATEGIST_URL" -model "$STRATEGIST_MODEL" \
	-thinking off -planner-wait 100s -log-dir "$out/log" -memory-db "$out/memory.db" -memory-carry-lessons=false \
	${EXEMPLARS:+-exemplars "$EXEMPLARS"} >"$out/bridge.out" 2>&1 &
pids+=($!)
narrator=(-llm-provider none)
[[ -n ${NARRATOR_URL:-} ]] && narrator=(-llm-provider openai -llm-url "$NARRATOR_URL" -llm-model "${NARRATOR_MODEL:?}")
nice -n 5 "$out/bin/fenceshow" -live-log "$out/log" -bridge http://127.0.0.1:7361 -listen 127.0.0.1:7360 \
	-player "${PLAYER:-rehearsal}" -model "$STRATEGIST_MODEL" -persona "$persona" -route "$route" \
	-record "$out/show/feed.jsonl" -board "$out/show/board.json" -audio-dir "$out/show/audio" \
	"${narrator[@]}" -tts kokoro -tts-url "${TTS_URL:-http://127.0.0.1:8881}" \
	-setup "Strategist: $STRATEGIST_MODEL, delta plans over the $route route;Hands: the gym's executor, paced like the live game" \
	>"$out/fenceshow.out" 2>&1 &
pids+=($!)
"$out/bin/pacer" -listen 127.0.0.1:7362 -bridge http://127.0.0.1:7361 -day "$day" >"$out/pacer.out" 2>&1 &
pids+=($!)
sleep 2
echo "panel: http://127.0.0.1:7360/   out: $out"
(cd "$(dirname "$gym")" && exec nice -n 10 dotnet "$(basename "$gym")" run -seed "$seed" -days 1000 -difficulty hard \
	-start apprentice -human player -perks vending+short_sleeper -deadline-ms 110000 \
	-directives "$root/sim/lab/scripts/directives/routes/$route.json" -bridge http://127.0.0.1:7362 \
	-out "$root/$out/gym/trajectory.jsonl" -summary "$root/$out/gym/gym.summary.json") >"$out/gym/gym.out" 2>&1
echo "game over: $(tail -1 "$out/gym/gym.out")"
sleep "${LINGER:-60}" # let the narrator land the ending
