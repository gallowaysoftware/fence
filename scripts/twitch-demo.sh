#!/usr/bin/env bash
# Replays a finished run through fenceshow with fencetwitch's dry run beside it: synthetic bets
# become predictions, a scripted chat votes, and the panel's audience box is screenshotted. Nothing
# reaches Twitch. Writes var/twitch/demo/ (TWITCH_DEMO_OUT): the feed, the dry-run log, the last
# tally, panel screenshots and a summary.
#
#   scripts/twitch-demo.sh RUN_DIR [DAYS] [SPEED]
#
# RUN_DIR is a league match or a rehearsal directory holding log/ and bridge.log (the dress
# rehearsal's bridge.out, renamed or linked).
set -euo pipefail
cd "$(dirname "$0")/.."

match=${1:?usage: scripts/twitch-demo.sh RUN_DIR [DAYS] [SPEED]}
days=${2:-30-41}
speed=${3:-0.5}
out=${TWITCH_DEMO_OUT:-var/twitch/demo}
listen=${TWITCH_DEMO_LISTEN:-127.0.0.1:7351}

rm -rf "$out"
mkdir -p "$out/shots"
go build -o "$out/fenceshow" ./cmd/fenceshow
go build -o "$out/fencetwitch" ./cmd/fencetwitch

nice "$out/fenceshow" -replay "$match" -route "${TWITCH_DEMO_ROUTE:-sprinter}" -persona "${TWITCH_DEMO_PERSONA:-shark}" \
	${TWITCH_DEMO_FORMAT:+-format "$TWITCH_DEMO_FORMAT"} -days "$days" -speed "$speed" -listen "$listen" \
	-record "$out/feed.jsonl" -board "" -narrate=false -exit-after 20s 2>"$out/fenceshow.log" &
show=$!
bets=()
[[ -n ${TWITCH_DEMO_BETS-internal/twitch/testdata/demo-bets.txt} ]] && bets=(-synthetic-bets "${TWITCH_DEMO_BETS-internal/twitch/testdata/demo-bets.txt}")
"$out/fencetwitch" -show "http://$listen" -dryrun-log "$out/dryrun.jsonl" -tally "$out/tally.json" \
	-chat-script "${TWITCH_DEMO_CHAT:-internal/twitch/testdata/demo-chat.txt}" \
	"${bets[@]}" \
	-lock-after 30s -poll-min 15s -poll-max 2m ${TWITCH_DEMO_ARGS:-} 2>"$out/fencetwitch.log" &
twitch=$!
trap 'kill $show $twitch 2>/dev/null || true' EXIT

shell=$(ls -d "$HOME"/.cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell 2>/dev/null | tail -1 || true)
shot() {
	[[ -n $shell ]] || return 0
	timeout 60 nice "$shell" --no-sandbox --hide-scrollbars --window-size="$2" --virtual-time-budget=2500 \
		--screenshot="$out/shots/$1.png" "http://$listen/$3" >/dev/null 2>&1 || true
}
n=0
while kill -0 "$show" 2>/dev/null; do
	sleep "${TWITCH_DEMO_SHOT_EVERY:-20}"
	kill -0 "$show" 2>/dev/null || break
	n=$((n + 1))
	shot "column-$n" 560,1080 "column?snap=1"
	(( n % 5 == 0 )) && shot "full-$n" 1920,1080 "?demo=1&snap=1"
done
kill "$twitch" 2>/dev/null || true
wait "$twitch" 2>/dev/null || true

python3 - "$out" <<'PY'
import json, sys
out = sys.argv[1]
calls, notes = [], []
for l in open(f"{out}/dryrun.jsonl"):
    e = json.loads(l)
    if "/helix/" in e["call"]:
        b = e.get("body", {})
        what = b.get("title") or b.get("status", "")
        calls.append(f'{e["t"][11:19]}  {e["call"]:<26} {e.get("status", ""):<10} {what} {e.get("err", "")}'.rstrip())
    elif e["call"] in ("bet_staked", "bet_settled", "poll_open", "poll_close", "tally_sent", "skip", "cancel_stale"):
        notes.append(f'{e["t"][11:19]}  {e["call"]:<12} {json.dumps(e.get("body"), ensure_ascii=False)}')
with open(f"{out}/summary.txt", "w") as f:
    f.write("Would-be Helix calls:\n" + "\n".join(calls) + "\n\nShow decisions:\n" + "\n".join(notes) + "\n")
print(open(f"{out}/summary.txt").read())
PY
ls "$out/shots"
