#!/usr/bin/env bash
# Replays a finished league game through the show feed, panel and narrator, recording a
# sample under var/show/demo/ (FENCE_SHOW_OUT): the feed, each narration WAV, the narration joined into one
# file, a transcript, and panel screenshots (when a headless Chromium is found).
#
#   scripts/show-demo.sh [MATCH_DIR] [DAYS] [SPEED]
#
# Narrator LLM: FENCE_SHOW_LLM_URL / FENCE_SHOW_LLM_MODEL (make show-demo passes drift's
# from local.mk); unset, the captions are the lines. TTS: Kokoro at FENCE_SHOW_TTS_URL
# (default 127.0.0.1:8880), else piper, else captions only. Open the printed URL while it
# runs to watch and hear it.
set -euo pipefail
cd "$(dirname "$0")/.."

match=${1:-var/league/t5-hard-drift/drift-delta__apprentice__s970001}
days=${2:-38-46}
speed=${3:-1}
out=${FENCE_SHOW_OUT:-var/show/demo}
listen=${FENCE_SHOW_LISTEN:-127.0.0.1:7350}

[[ -d $match ]] || { echo "no match directory $match" >&2; exit 1; }
# The scoreboard: the same player's other finished league games.
player=$(basename "$match")
seed="$(dirname "$(dirname "$match")")/*/${player%%__*}__*"
rm -rf "$out"
mkdir -p "$out/audio" "$out/shots"
go build -o "$out/fenceshow" ./cmd/fenceshow

nice "$out/fenceshow" -replay "$match" -days "$days" -speed "$speed" -listen "$listen" \
	-record "$out/feed.jsonl" -audio-dir "$out/audio" -board "$out/board.json" \
	-board-seed "${FENCE_SHOW_BOARD_SEED:-$seed}" -exit-after 25s 2>"$out/fenceshow.log" &
pid=$!
trap 'kill $pid 2>/dev/null || true' EXIT
echo "panel: http://$listen/?demo=1   (column: /column, lower third: /lower)"

shell=$(ls -d "$HOME"/.cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell 2>/dev/null | tail -1 || true)
shot() {
	[[ -n $shell ]] || return 0
	timeout 60 nice "$shell" --no-sandbox --hide-scrollbars --window-size="$2" --virtual-time-budget=2500 \
		--screenshot="$out/shots/$1.png" "http://$listen/$3" >/dev/null 2>&1 || true
}
n=0
while kill -0 "$pid" 2>/dev/null; do
	sleep "${FENCE_SHOW_SHOT_EVERY:-45}"
	kill -0 "$pid" 2>/dev/null || break
	n=$((n + 1))
	shot "full-$n" 1920,1080 "?demo=1&snap=1"
	[[ $n == 2 ]] && { shot column 560,1080 "column?snap=1"; shot lower 1360,315 "lower?snap=1"; }
done
wait "$pid" || true

python3 - "$out" <<'EOF'
import json, sys
out = sys.argv[1]
checks, ms = {}, []
with open(f"{out}/transcript.txt", "w") as t:
    for l in open(f"{out}/feed.jsonl"):
        e = json.loads(l)
        if e["kind"] == "say":
            s = e["say"]
            c = s.get("check", "")
            checks[c] = checks.get(c, 0) + 1
            if s.get("ms"):
                ms.append(s["ms"])
            tag = ", ".join(x for x in [s["source"], s.get("voice", ""), c, f"{s['ms']} ms" if s.get("ms") else "", s.get("mood", "")] if x)
            t.write(f"day {e['day']:>3}  [{tag}]  {s['line']}\n")
            for f in s.get("faults") or []:
                t.write(f"          first try: {f}\n")
    n = sum(v for k, v in checks.items() if k)
    if n:
        ms.sort()
        t.write(f"\nfact check: {checks.get('pass', 0)}/{n} first try, {checks.get('retry', 0)} on the retry, {checks.get('fallback', 0)} fell back to the caption; "
                f"LLM per line median {ms[len(ms) // 2]} ms, max {ms[-1]} ms\n")
EOF
if command -v ffmpeg >/dev/null && ls "$out"/audio/*.wav >/dev/null 2>&1; then
	list=$out/audio/list.txt
	: >"$list"
	for f in "$out"/audio/line-*.wav; do echo "file '$(basename "$f")'" >>"$list"; done
	ffmpeg -loglevel error -y -f concat -safe 0 -i "$list" -af "apad=pad_dur=0.6" "$out/narration.wav" || true
	rm -f "$list"
fi
echo "demo in $out:"
ls -1 "$out" "$out/shots"
cat "$out/transcript.txt"
