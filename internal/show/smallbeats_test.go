package show

import (
	"strings"
	"testing"
	"time"
)

func withBeat(evs []Event, beat string) []Event {
	var out []Event
	for _, e := range evs {
		if e.Beat == beat {
			out = append(out, e)
		}
	}
	return out
}

func snap(phase string, day int, nw int64, inv ...map[string]any) map[string]any {
	return ev("day_snapshot", day, map[string]any{
		"phase": phase, "store": map[string]any{"cash": 500, "day": day, "net_worth": nw},
		"property": map[string]any{"owned": false, "buyout_price": 10000}, "inventory": inv,
	})
}

func counterLines(id string, uid int, who string, cash int) []map[string]any {
	side, delta := "sold", cash
	if cash < 0 {
		side, delta = "bought", cash
	}
	return []map[string]any{
		{"type": "decision", "decision_id": id, "request": map[string]any{"kind": "barter", "game_day": 3,
			"context": map[string]any{"client": map[string]any{"name": who, "intent": "buy"},
				"counter": map[string]any{"yours_staged": []map[string]any{{"uid": uid, "name": "Lamp", "price": 60, "estimated_value": 60}}}}}},
		{"type": "outcome", "decision_id": id, "outcome": map[string]any{"game_day": 3, "result": map[string]any{"kind": "trade", "side": side, "items": []int{uid}, "cash_delta": delta}}},
	}
}

func TestSmallBeats(t *testing.T) {
	b := NewBuilder(Player{})
	customer := func(seq int, who string) map[string]any {
		return ev("customer_observed", 3, map[string]any{"client_seq": seq, "context": map[string]any{"client": map[string]any{"name": who, "intent": "buy"}}})
	}
	lines := []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r1"}),
		snap("morning", 3, 2600, map[string]any{"uid": 1, "name": "Lamp"}),
		customer(1, "Ann"),
	}
	// A haggled deal is a story even when small; a notable trade too, but a third that day is not.
	ann := counterLines("c1", 1, "Ann", -60)
	lines = append(lines, ann[0], map[string]any{"type": "outcome", "decision_id": "c1", "outcome": map[string]any{"game_day": 3,
		"result": map[string]any{"kind": "bargain", "option": "demand_discount_10", "accepted": true}}}, ann[1])
	lines = append(lines, customer(2, "Bea"), customer(3, "Cy"), customer(4, "Di"))
	lines = append(lines, counterLines("c2", 2, "Di", 200)...)
	lines = append(lines, customer(5, "Ed"), customer(6, "Flo"), customer(7, "Gus"))
	lines = append(lines, counterLines("c3", 3, "Gus", 200)...)
	// Appraisals: three accepted in a row is a streak; a refusal starts over.
	for range 3 {
		lines = append(lines, ev("appraisal", 3, map[string]any{"type": "hand", "estimate": 100, "fee": 20}))
	}
	lines = append(lines, ev("appraisal", 3, map[string]any{"type": "hand", "estimate": 100, "refused": "too_low"}))
	lines = append(lines, ev("appraisal", 3, map[string]any{"type": "hand", "estimate": 100, "fee": 20}))
	// The night: what the sleep snapshot has that the close one did not.
	lines = append(lines,
		snap("close", 3, 2700, map[string]any{"uid": 1, "name": "Lamp"}),
		ev("human_action", 3, map[string]any{"action": "go_out", "location": "dumping_ground"}),
		ev("human_action", 3, map[string]any{"action": "scavenge"}),
		ev("human_action", 3, map[string]any{"action": "scavenge"}),
		ev("human_action", 3, map[string]any{"action": "return", "location": "dumping_ground"}),
		snap("sleep", 3, 2700, map[string]any{"uid": 1, "name": "Lamp"}, map[string]any{"uid": 9, "name": "Scrap", "estimated_value": 12},
			map[string]any{"uid": 10, "name": "Scrap", "estimated_value": 12}, map[string]any{"uid": 11, "name": "Bolt", "where": "inside:9"}),
		snap("morning", 4, 5100),
		snap("close", 4, 5100),
		ev("human_action", 4, map[string]any{"action": "go_out", "location": "dr_jackson"}),
		snap("sleep", 4, 5100),
	)
	evs := feedAll(t, b, lines...)

	c := withBeat(evs, BeatCounter)
	if len(c) != counterBeatsPerDay || c[0].Level != LevelMoment || !strings.Contains(c[0].Text, "after a haggle (demand discount 10%): accepted") || c[1].Trade.Customer != "Di" {
		t.Errorf("counter beats = %+v", c)
	}
	if s := withBeat(evs, BeatAppraisalStreak); len(s) != 1 || s[0].Text != "Appraisal streak: 3 in a row accepted, 60 in fees over the streak." {
		t.Errorf("streak beats = %+v", s)
	}
	n := withBeat(evs, BeatNight)
	if len(n) != 2 || n[0].Text != "Night at the dumping ground, 2 scavenges: back with 2× Scrap (estimate 24)." || n[1].Text != "Night at Dr. Jackson's: back with nothing." {
		t.Errorf("night beats = %+v", n)
	}
	// 2,600 of 10,000 crosses a quarter; 5,100 crosses half; neither is told twice.
	m := withBeat(evs, BeatMilestone)
	if len(m) != 2 || m[0].Text != "Net worth 2,600: a quarter of the way to the 10,000 buyout (25%)." || !strings.HasPrefix(m[1].Text, "Net worth 5,100: halfway") {
		t.Errorf("milestones = %+v", m)
	}
	for _, e := range slices2(c, n, m) {
		f := FactsOf(e)
		for _, w := range []string{"24", "10,000", "2,600"} {
			if strings.Contains(e.Text, w) && len(CheckLine("That is "+w+" right there.", f)) > 0 {
				t.Errorf("%q: the caption's %s fails the fact check", e.Text, w)
			}
		}
	}
}

func slices2(ss ...[]Event) []Event {
	var out []Event
	for _, s := range ss {
		out = append(out, s...)
	}
	return out
}

func TestSmallBeatPromptAndPriority(t *testing.T) {
	n := &Narrator{}
	ev := Event{Day: 3, Kind: KindThought, Beat: BeatNight, Text: "Night at the dumping ground: back with nothing."}
	got := n.prompt(ev, FactsOf(ev), nil, "")
	if !strings.Contains(got, "This moment: the night out") || strings.Contains(got, "big moment") {
		t.Errorf("prompt = %q", got)
	}
	plan := Priority(Event{Kind: KindPlan})
	if Priority(Event{Kind: KindThought, Beat: BeatMilestone}) <= plan || Priority(ev) >= plan || Priority(ev) <= Priority(Event{Kind: KindDay}) {
		t.Error("small beats rank wrong against the plan and the day")
	}
}

func TestSmallBeatsWaitLonger(t *testing.T) {
	old := time.Now().Add(-20 * time.Second).UnixMilli()
	n := &Narrator{}
	said := narrate(t, n, Event{Seq: 1, At: old, Level: LevelMoment, Kind: KindThought, Beat: BeatNight, Text: "Night at Dr. Jackson's: back with nothing."})
	if len(said) != 1 {
		t.Errorf("a 20 s old night was not told: %+v", said)
	}
	if said := narrate(t, &Narrator{}, Event{Seq: 1, At: old, Level: LevelMoment, Kind: KindMoney, Text: "Paid the rent: 300."}); len(said) != 0 {
		t.Errorf("a 20 s old money moment was told: %+v", said)
	}
}
