#!/usr/bin/env bash
# The 5090 pilot: QLoRA a small base on var/distill, evaluate on val, export a GGUF.
# Refuses to start unless the GPU is idle: no league running, nothing loaded in llama-swap,
# the game not running. Nothing here unloads or stops anything.
#
#   tools/distill/pilot.sh [RUN_NAME [train.py flags...]]
#   e.g. pilot.sh qwen3-8b-r1 --epoch-rows 2000 --epochs 1
#   DATA=var/distill/r2 pilot.sh qwen3-8b-r2 ...   (another dataset directory)
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
run=${1:-qwen3-8b-r1}
base=${BASE:-$HOME/models/hf/Qwen3-8B}
out=${OUT:-$HOME/models/distill/$run}
swap=${SWAP:-http://127.0.0.1:9000}
data=$(cd "$repo" && realpath "${DATA:-var/distill}")

busy=()
pgrep -x 'Probably Stolen' >/dev/null && busy+=("Probably Stolen is running")
# ALLOW_LEAGUE=1: a league is running on other endpoints only (e.g. heavy), never localmodel.
[[ ${ALLOW_LEAGUE:-} == 1 ]] || { pgrep -x fenceleague >/dev/null && busy+=("a league is running (fenceleague)"); }
running=$(curl -s -m 5 "$swap/running" || true)
[[ $running == *'"model"'* ]] && busy+=("llama-swap has a model loaded: $running")
used=$(nvidia-smi --query-gpu=memory.used --format=csv,noheader,nounits | head -1)
((used > 2000)) && busy+=("GPU memory in use: ${used} MiB")
if ((${#busy[@]})); then
	printf 'GPU not free:\n' >&2
	printf '  - %s\n' "${busy[@]}" >&2
	exit 1
fi

mkdir -p "$out"
cd "$here"
uv run python train.py --data "$data" --base "$base" --out "$out" --qlora "${@:2}" 2>&1 | tee "$out/train.log"
uv run python evaluate.py --data "$data/val.jsonl" --hf "$base" --adapter "$out/adapter" --qlora \
	--sample "${EVAL_SAMPLE:-200}" --out "$out/eval.json" 2>&1 | tee "$out/eval.log"
"$here/export.sh" merged "$base" "$out/adapter" "$out" "$run" Q8_0
