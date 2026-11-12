#!/usr/bin/env bash
# Every panel element live at once, for the layout's headless screenshots (docs/SHOW.md, "The
# panel"): a replay with a format, the ghost band, fencetwitch's dry run (the format's prediction and
# the chat votes), and made-up agency and bet events posted to /v1/ingest: a day-0 route choice with a
# long thesis, two bets with long claims settled by the feed's own clock, and a four-option fork whose
# vote Twitch then opens. No LLM, no TTS: the narrator speaks the captions. Writes var/panel/demo/
# (PANEL_DEMO_OUT): the feed, the logs and shots/ (/column, /lower and the full page with each shot's
# /state beside it).
#
#   scripts/panel-demo.sh RUN_DIR [DAYS] [SPEED]
#
# RUN_DIR is the dress rehearsal's (docs/STREAM-OPS.md section 10; log/ and bridge.log), DAYS 54-59
# by default: the made-up bets and fork are keyed to those days (the first bet is won on day 57, the
# second lost on day 59). PANEL_DEMO_PREV: an earlier run for
# the second ghost.
set -euo pipefail
cd "$(dirname "$0")/.."

match=${1:?usage: scripts/panel-demo.sh RUN_DIR [DAYS] [SPEED]}
days=${2:-54-59}
speed=${3:-1}
out=${PANEL_DEMO_OUT:-var/panel/demo}
listen=${PANEL_DEMO_LISTEN:-127.0.0.1:7352}

rm -rf "$out"
mkdir -p "$out/shots"
go build -o "$out/fenceshow" ./cmd/fenceshow
go build -o "$out/fencetwitch" ./cmd/fencetwitch

nice "$out/fenceshow" -replay "$match" -route sprinter -persona shark -format "${PANEL_DEMO_FORMAT:-sprint_91}" \
	${PANEL_DEMO_PREV:+-prev-run "$PANEL_DEMO_PREV"} -model "${PANEL_DEMO_MODEL:-qwen3.8-27b}" \
	-days "$days" -speed "$speed" -listen "$listen" -record "$out/feed.jsonl" -board "" \
	-llm-provider none -tts none -audio-dir "$out/audio" -format-record "$out/record.jsonl" \
	-exit-after 30s 2>"$out/fenceshow.log" &
show=$!
sleep 1
python3 -I - "http://$listen" >"$out/inject.log" 2>&1 <<'PY' &
import json, sys, urllib.request

show = sys.argv[1]
THESIS = ("We are skipping the safe appraiser because a steady drip of appraisal fees is a slow death "
          "against a rent that climbs every week. We need a cash spike that scales with time, not just "
          "consistency: the sprinter route keeps the Board for the floor and saves every spare credit "
          "for a cash buyout before day 91, and it never signs the loan shark's paper, because a mortgage "
          "only wins at its last payment, long after the deadline has passed.")
BETS = [  # stake day, kind, target, by, claim
    (54, "cash", 7000, 58, "Cash past 7,000 by day 58, with the rent paid and the hot stock still in the stash, not at the counter"),
    (58, "net_worth", 12000, 58, "Net worth past 12,000 by day 58 on the household sales alone, without touching the stash"),
]
FORK_DAY = 55
FORK = [
    ("sprint as the route says: no insurance from day 56, cash buyout by day 112", False),
    ("all in: cut insurance and upgrades from today, cash by day 112", True),
    ("keep the insurance and sprint without it otherwise", False),
    ("give up the race and sign the loan shark's mortgage", False),
]
FORK_WHY = ("Day 91 is the deadline and the buyout is still 15,000 away; insurance is 150 a week we "
            "cannot spare, and every upgrade from here pays back after the race is already decided.")


def post(ev):
    req = urllib.request.Request(show + "/v1/ingest", data=json.dumps(ev).encode(), headers={"Content-Type": "application/json"})
    urllib.request.urlopen(req, timeout=5).read()
    print(ev["kind"], ev.get("beat", ""), ev["text"][:80], flush=True)


day, clock, chose, forked = 0, {}, False, False
staked, open_bets, plan_days = set(), {}, set()
try:
    for raw in urllib.request.urlopen(show + "/events", timeout=600):
        line = raw.decode().strip()
        if not line.startswith("data:"):
            continue
        ev = json.loads(line[5:])
        if "kind" not in ev:  # the opening state message
            continue
        day = max(day, ev.get("day", 0))
        clock = ev.get("clock") or clock
        if not chose and ev["kind"] in ("plan", "day", "trade", "thought"):
            chose = True
            post({"kind": "plan", "level": 2, "beat": "route_choice", "day": day,
                  "text": "Day 0: the strategist picks the sprinter route. " + THESIS,
                  "plan": {"day": 0, "source": "llm", "change": True, "changes": ["run the sprinter route"],
                           "headline": ["Day 0: the strategist picks the sprinter route"],
                           "sections": [{"title": "Thesis", "lines": [THESIS]}]}})
        if ev["kind"] == "plan" and ev.get("beat") != "route_choice" and day not in plan_days:
            plan_days.add(day)
            for d, kind, target, by, claim in BETS:
                if d == day and claim not in staked:
                    staked.add(claim)
                    for old in list(open_bets.values()):  # one open bet at a time, as internal/bet keeps it
                        del open_bets[old["claim"]]
                        post({"kind": "bet", "level": 2, "beat": "bet_abandoned", "day": day,
                              "bet": dict(old, outcome="abandoned", fact=f"replaced by a new bet on day {day}"),
                              "text": f'Bet abandoned: "{old["claim"]}", staked on day {old["staked"]}. Replaced by a new bet on day {day}.'})
                    open_bets[claim] = {"claim": claim, "kind": kind, "target": target, "by": by, "staked": d, "outcome": "staked"}
                    post({"kind": "bet", "level": 2, "beat": "bet_made", "day": day, "bet": open_bets[claim],
                          "text": f'Bet staked on day {d}: "{claim}".'})
            if day == FORK_DAY and not forked:
                forked = True
                opts = [{"label": l, "source": "llm", "score": 0, "chosen": c, **({"why": FORK_WHY} if c else {})} for l, c in FORK]
                post({"kind": "candidates", "level": 2, "beat": "fork", "day": day,
                      "text": f"Day {day}, the cash sprint: how hard to save for the buyout: the strategist chooses {FORK[1][0]}. {FORK_WHY}",
                      "candidates": {"day": day, "metric": "choice", "options": opts}})
        for claim, b in list(open_bets.items()):
            have = clock.get("cash" if b["kind"] == "cash" else "net_worth", 0)
            what = "cash" if b["kind"] == "cash" else "net worth"
            if have >= b["target"]:
                b = dict(b, outcome="won", fact=f"{what} {have:,} on day {day}")
            elif day > b["by"]:
                b = dict(b, outcome="lost", fact=f"{what} {have:,} on day {day}, short of {b['target']:,}")
            else:
                continue
            del open_bets[claim]
            post({"kind": "bet", "level": 2, "beat": "bet_" + b["outcome"], "day": day, "bet": b,
                  "text": f'Bet {b["outcome"]}: "{claim}", staked on day {b["staked"]}. {b["fact"][0].upper() + b["fact"][1:]}.'})
except OSError as e:  # the replay ending closes the stream
    print("stream closed:", e)
PY
inject=$!
"$out/fencetwitch" -show "http://$listen" -dryrun-log "$out/dryrun.jsonl" -tally "$out/tally.json" \
	-chat-script "${PANEL_DEMO_CHAT:-internal/twitch/testdata/demo-chat.txt}" \
	-lock-after 90s -poll-min 15s -poll-max 2m 2>"$out/fencetwitch.log" &
twitch=$!
trap 'kill $show $inject $twitch 2>/dev/null || true' EXIT

shell=$(ls -d "$HOME"/.cache/ms-playwright/chromium_headless_shell-*/chrome-headless-shell-linux64/chrome-headless-shell 2>/dev/null | tail -1 || true)
[[ -n $shell ]] || echo "no headless Chromium (npx playwright install chromium-headless-shell): no screenshots" >&2
shot() {
	[[ -n $shell ]] || return 0
	timeout 60 nice "$shell" --no-sandbox --hide-scrollbars --window-size="$2" --virtual-time-budget=2500 \
		--screenshot="$out/shots/$1.png" "http://$listen/$3" >/dev/null 2>&1 || true
}
n=0
while kill -0 "$show" 2>/dev/null; do
	sleep "${PANEL_DEMO_SHOT_EVERY:-15}"
	kill -0 "$show" 2>/dev/null || break
	n=$((n + 1))
	t=$(printf %03d $n)
	curl -s "http://$listen/state" >"$out/shots/$t.state.json" || true
	shot "$t-column" 560,1080 "column?snap=1"
	shot "$t-lower" 1360,315 "lower?snap=1"
	(( n % 4 == 0 )) && shot "$t-full" 1920,1080 "?demo=1&snap=1"
done
kill "$twitch" "$inject" 2>/dev/null || true
cat "$out/inject.log"
ls "$out/shots" | wc -l
