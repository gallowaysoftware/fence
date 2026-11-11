package show

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// secret marks every hidden value in the fixtures; it must never reach an event.
const secret = "SECRET-TRUE-VALUE"

func line(t *testing.T, v map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ev(kind string, day int, data map[string]any) map[string]any {
	return map[string]any{"type": "event", "ts": "2026-01-01T00:00:00Z", "event": map[string]any{
		"kind": kind, "game_day": day, "data": data, "hidden": map[string]any{"note": secret, "true_value": 9999},
	}}
}

func snapshot(phase string, day int, cash, nw int64, security map[string]any) map[string]any {
	return ev("day_snapshot", day, map[string]any{
		"phase": phase,
		"store": map[string]any{"cash": cash, "day": day, "weekday": 3, "net_worth": nw},
		"rent":  map[string]any{"due": 300, "days_left": 2},
		"property": map[string]any{"owned": false, "buyout_price": 24000, "mortgage": nil,
			"mortgage_offer": map[string]any{"down_payment": 3000, "weekly": 700}},
		"reputation": map[string]any{"lower_levels": map[string]any{"value": map[string]float64{"morning": 10, "close": 13}[phase]}},
		"inventory": []map[string]any{
			{"uid": 7, "name": "Widget", "estimated_value": 100, "bought_at": 40},
			{"uid": 8, "name": "Hot Gadget", "estimated_value": 60, "stolen_heat": 30},
		},
		"security": security,
		"upgrades": map[string]any{"owned": []map[string]any{}, "available": []map[string]any{{"id": "SHELF_X", "name": "Better Shelf"}}},
	})
}

func feedAll(t *testing.T, b *Builder, lines ...map[string]any) []Event {
	t.Helper()
	var out []Event
	for _, l := range lines {
		evs, err := b.Feed(line(t, l))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, evs...)
	}
	return out
}

func find(evs []Event, k Kind) []Event {
	var out []Event
	for _, e := range evs {
		if e.Kind == k {
			out = append(out, e)
		}
	}
	return out
}

func TestBuilderDay(t *testing.T) {
	b := NewBuilder(Player{ID: "p1", Model: "m1"})
	b.Plans = func(day int) *PlanInput {
		floor := int64(250)
		return &PlanInput{Day: day, FromModel: []string{"cash_floor", "bargain"}, Directives: &protocol.Directives{CashFloor: &floor, Bargain: "never"},
			Fixes: []string{"sell.min_pct 65 dropped: the executor's floors stand"}}
	}
	evs := feedAll(t, b,
		ev("run_start", 1, map[string]any{"run_id": "r1", "difficulty": "hard", "start_type": "apprentice", "perks": []string{"short_sleeper"}}),
		snapshot("morning", 5, 500, 2000, map[string]any{"visible": true, "evidence": 8, "evidence_level": "8/100", "operation": "Investigation", "case_officer": "Officer Q"}),
		ev("customer_observed", 5, map[string]any{"client_seq": 11, "context": map[string]any{"client": map[string]any{"name": "Buyer Bo", "faction": "lower_levels", "intent": "buy"}}}),
		map[string]any{"type": "decision", "decision_id": "d1", "request": map[string]any{"kind": "barter", "game_day": 5,
			"context": map[string]any{"client": map[string]any{"name": "Buyer Bo", "faction": "lower_levels", "intent": "buy"},
				"counter": map[string]any{"theirs": []any{}, "yours_staged": []map[string]any{{"uid": 7, "name": "Widget", "price": 420, "estimated_value": 100, "bought_at": 40}}}},
			"hidden": map[string]any{"items": []map[string]any{{"uid": 7, "true_value": secret}}}},
			"response": map[string]any{"rationale": "bridge is in observe mode", "source": "fallback"}},
		map[string]any{"type": "outcome", "decision_id": "d1", "outcome": map[string]any{"game_day": 5, "result": map[string]any{"kind": "trade", "side": "sold", "items": []int{7}, "cash_delta": 420}}},
		map[string]any{"type": "decision", "decision_id": "d2", "request": map[string]any{"kind": "barter", "game_day": 5,
			"context": map[string]any{"client": map[string]any{"name": "Seller Sy", "faction": "upper_levels", "intent": "sell", "wholesale": map[string]any{"price": 90}},
				"counter": map[string]any{"theirs": []map[string]any{{"uid": 20, "name": "Gizmo", "price": 50, "estimated_value": 80}, {"uid": 21, "name": "Gizmo (Counterfeit)", "price": 50, "estimated_value": 70}}}}}},
		map[string]any{"type": "outcome", "decision_id": "d2", "outcome": map[string]any{"game_day": 5, "result": map[string]any{"kind": "trade", "side": "bought", "items": []int{20, 21}, "cash_delta": -90}}},
		ev("money", 5, map[string]any{"kind": "georgio_loan", "amount": 700}),
		ev("human_action", 5, map[string]any{"action": "upgrade_buy", "upgrade": "SHELF_X", "cost": 200, "cash_spent": 150}),
		ev("aug", 5, map[string]any{"event": "good_kill", "name": "Lower Level Citizen", "cash_delta": 100}),
		snapshot("close", 5, 1480, 2600, map[string]any{"visible": true, "evidence": 21, "evidence_level": "21/100", "operation": "Investigation", "case_officer": "Officer Q"}),
		ev("money", 5, map[string]any{"kind": "day_ended", "day": 5, "net_worth": 2600, "credits": 1480}),
		ev("run_end", 9, map[string]any{"ending": "rent", "day": 9, "cash": 12}),
	)

	all, _ := json.Marshal(evs)
	if bytes.Contains(all, []byte(secret)) || bytes.Contains(all, []byte("9999")) {
		t.Fatalf("a hidden value reached the feed: %s", all)
	}
	if rs := find(evs, KindRunStart); len(rs) != 1 || rs[0].Player.Difficulty != "hard" || rs[0].Run != "p1:r1" {
		t.Fatalf("run start = %+v", rs)
	}
	if ps := find(evs, KindPlan); len(ps) != 1 || ps[0].Level != LevelMoment || len(ps[0].Plan.Why) != 1 {
		t.Fatalf("plan = %+v", ps)
	}
	trades := find(evs, KindTrade)
	if len(trades) != 2 {
		t.Fatalf("trades = %+v", trades)
	}
	if s := trades[0].Trade; s.Side != "sold" || !s.HasProfit || s.Profit != 380 || trades[0].Level != LevelMoment {
		t.Errorf("sale = %+v level %d", s, trades[0].Level)
	}
	if bt := trades[1].Trade; !bt.Wholesale || bt.Estimate != 150 || bt.Fakes != 1 || bt.Customer != "Seller Sy" {
		t.Errorf("purchase = %+v", bt)
	}
	if th := find(evs, KindThought); len(th) != 2 || !strings.Contains(th[1].Text, "Buyer Bo") {
		t.Errorf("thoughts = %+v (the fallback's rationale is boilerplate)", th)
	} else if m := th[0]; m.Day != 5 || m.Clock == nil || m.Clock.Cash != 500 || m.Level != LevelTicker {
		// The panel turns to the new day at the morning, not when the plan comes a minute later.
		t.Errorf("morning = %+v", m)
	}
	if m := find(evs, KindMoney); len(m) != 1 || m[0].Level != LevelMoment || m[0].Amount != -700 {
		t.Errorf("money = %+v", m)
	}
	if p := find(evs, KindPurchase); len(p) != 1 || !strings.Contains(p[0].Text, "Better Shelf") || p[0].Amount != -150 {
		t.Errorf("purchase = %+v", p)
	}
	heat := find(evs, KindHeat)
	if len(heat) != 2 || heat[1].Level != LevelMoment || heat[1].Heat.HotItems != 1 {
		t.Errorf("heat = %+v", heat)
	}
	day := find(evs, KindDay)
	if len(day) != 1 || day[0].Summary.Rep["lower_levels"] != 3 || day[0].Summary.SoldItems != 1 || day[0].Clock.OfferDown != 3000 {
		t.Fatalf("day = %+v", day)
	}
	if end := find(evs, KindRunEnd); len(end) != 1 || end[0].End.Won || end[0].End.Ending != "rent" {
		t.Errorf("end = %+v", end)
	}
}

func TestPlanChangeLevels(t *testing.T) {
	b := NewBuilder(Player{})
	b.names[1], b.names[2] = "Lamp", "Kettle"
	held := func(uids ...int64) *protocol.Directives {
		return &protocol.Directives{Sell: &protocol.SellDirectives{HoldUIDs: uids}}
	}
	cases := []struct {
		name   string
		in     PlanInput
		level  int
		change bool
	}{
		{"first plan", PlanInput{Day: 1, FromModel: []string{"sell"}, Directives: held(1)}, LevelMoment, false},
		// The day's first notable plan is told as the morning's; later ones stay notable.
		{"only the held items move", PlanInput{Day: 2, FromModel: []string{"sell"}, Directives: held(2)}, LevelMoment, true},
		{"the held items again that day", PlanInput{Day: 2, FromModel: []string{"sell"}, Directives: held(1)}, LevelNotable, true},
		{"the permit", PlanInput{Day: 3, FromModel: []string{"permit"}, Directives: &protocol.Directives{Permit: "buy"}}, LevelMoment, true},
		{"nothing from the model", PlanInput{Day: 4}, LevelTicker, true},
		{"prose", PlanInput{Day: 5, Text: "GOALS\n- Sell the wine.\nCASH\n- Floor 300."}, LevelMoment, false},
		{"prose again", PlanInput{Day: 5, Text: "GOALS\n- Sell the lamp.\nCASH\n- Floor 300."}, LevelNotable, false},
	}
	for _, c := range cases {
		got := b.Plan(c.in)
		if got.Level != c.level || got.Plan.Change != c.change {
			t.Errorf("%s: level %d change %v, want %d %v (%+v)", c.name, got.Level, got.Plan.Change, c.level, c.change, got.Plan)
		}
	}
	if got := b.Plan(PlanInput{Day: 6, Text: "GOALS\n- Sell the wine.\nCASH\n- Floor 300."}); got.Text != "Day 6 plan: Sell the wine." || got.Plan.Source != "llm" {
		t.Errorf("prose plan = %q %+v", got.Text, got.Plan)
	}
}

func TestBuilderNextRunInSession(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	evs := feedAll(t, b,
		ev("run_start", 1, map[string]any{"run_id": "r1", "difficulty": "hard"}),
		ev("run_start", 1, map[string]any{"run_id": "r1", "difficulty": "hard"}),
		ev("run_end", 30, map[string]any{"ending": "arrest", "day": 30}),
		ev("run_start", 1, map[string]any{"run_id": "r2", "difficulty": "normal"}),
		ev("run_end", 200, map[string]any{"ending": "buyout", "day": 200}),
	)
	starts, ends := find(evs, KindRunStart), find(evs, KindRunEnd)
	if len(starts) != 2 || starts[1].Run != "p:r2" || starts[1].Player.Difficulty != "normal" {
		t.Fatalf("starts = %+v", starts)
	}
	if len(ends) != 2 || !ends[1].End.Won || ends[1].Run != "p:r2" {
		t.Fatalf("ends = %+v", ends)
	}
}

// The live autopilot's log (docs/STREAM.md "Recording"): every action a thought with its reason and the
// hidden reads named, every stop a moment saying a human is needed. Hidden values never reach the feed.
func TestBuilderAutopilot(t *testing.T) {
	b := NewBuilder(Player{ID: "p1"})
	stage := ev("autopilot", 12, map[string]any{"day": 12, "phase": "customer", "action": "stage", "target": map[string]any{"uid": 7, "name": "Canned beans"},
		"why": "what Mara wants, within the budget", "rule": "player:stage_for_buyer", "client": "Mara", "hidden_used": []string{"item_exposable"}})
	stage["event"].(map[string]any)["hidden"] = map[string]any{"true_value": secret}
	evs := feedAll(t, b,
		ev("autopilot", 12, map[string]any{"day": 12, "phase": "morning", "action": "plan_day", "why": "rent 350 in 4 days"}),
		stage,
		ev("autopilot", 12, map[string]any{"day": 12, "phase": "customer", "action": "counter", "detail": "add_markup_10", "why": "add_markup_10 for about 132 (estimate 120)", "client": "Mara"}),
		ev("autopilot", 12, map[string]any{"day": 12, "phase": "customer", "action": "pause", "why": "you made a move (stage): the autopilot stopped so it cannot fight you"}),
		ev("autopilot_fault", 12, map[string]any{"day": 12, "phase": "customer", "state": "handover", "reason": "Ivo: appraisal customer, not dismissable",
			"message": "AUTOPILOT PAUSED — your turn: Ivo: appraisal customer, not dismissable. Play it by hand, then press Resume."}),
	)
	all, _ := json.Marshal(evs)
	if bytes.Contains(all, []byte(secret)) {
		t.Fatalf("a hidden value reached the feed: %s", all)
	}
	th := find(evs, KindThought)
	if len(th) != 2 || th[0].Text != "Stage Canned beans (Mara): what Mara wants, within the budget." || len(th[0].HiddenUsed) != 1 {
		t.Fatalf("thoughts = %+v", th)
	}
	if th[1].Text != "Add markup 10% (Mara): add_markup_10 for about 132 (estimate 120)." || th[1].HiddenUsed != nil {
		t.Errorf("counter thought = %+v", th[1])
	}
	ap := find(evs, KindAutopilot)
	if len(ap) != 2 || ap[0].Autopilot.State != "paused" || ap[0].Autopilot.NeedsHuman || ap[0].Level != LevelNotable {
		t.Fatalf("autopilot = %+v", ap)
	}
	if f := ap[1]; f.Level != LevelMoment || !f.Autopilot.NeedsHuman || f.Autopilot.State != "handover" || f.Beat != BeatHandover ||
		f.Text != "Over to the human: Ivo: appraisal customer, not dismissable." {
		t.Errorf("fault = %+v", f)
	}
}

func TestBuilderShowLine(t *testing.T) {
	b := NewBuilder(Player{ID: "hybrid-blind"})
	evs := feedAll(t, b,
		// The mod sends perks as objects (GameDay.Run).
		ev("run_start", 1, map[string]any{"run_id": "r1", "difficulty": "hard", "start_type": "apprentice",
			"perks": []map[string]any{{"id": "vending", "name": "Vending Machine"}}}),
		snapshot("morning", 41, 900, 2000, nil),
		map[string]any{"type": "show", "ts": "2026-01-01T00:00:01Z", "show": map[string]any{
			"kind": "candidates", "level": 2, "text": "Day 41: considered keep the plan / lend georgio 50.",
			"candidates": map[string]any{"day": 41, "options": []map[string]any{{"label": "keep the plan", "score": 12.5}, {"label": "lend georgio 50", "score": 14.1, "chosen": true,
				"why": "a fourfold payback before the day-56 offer is worth more than the stock it costs", "delta": map[string]any{"georgio": "lend_50", "mortgage": "later", "cash_floor": 400}}}},
		}},
		map[string]any{"type": "show", "show": map[string]any{"kind": "candidates"}},
	)
	if rs := find(evs, KindRunStart); len(rs) != 1 || len(rs[0].Player.Perks) != 1 || rs[0].Player.Perks[0] != "vending" {
		t.Fatalf("run_start with object perks: %+v", rs)
	}
	got := find(evs, KindCandidates)
	if len(got) != 1 {
		t.Fatalf("want 1 candidates event (a show line without text is dropped), got %d", len(got))
	}
	c := got[0]
	if c.Day != 41 || c.V != SchemaVersion || c.TS != "2026-01-01T00:00:01Z" || c.Candidates == nil || !c.Candidates.Options[1].Chosen {
		t.Fatalf("candidates event: %+v", c)
	}
	plans := find(evs, KindPlan)
	if len(plans) != 1 {
		t.Fatalf("want the chosen candidate as today's plan, got %d plans", len(plans))
	}
	p := plans[0]
	want := []string{"keep at least 400 cash", "put the mortgage off a week", "lend Georgio half the cash"}
	if p.Day != 41 || p.Level != LevelNotable || p.Plan.Source != "hybrid" || !slices.Equal(p.Plan.Directives, want) ||
		len(p.Plan.Why) != 1 || !strings.HasPrefix(p.Plan.Why[0], "a fourfold payback") || p.Text != "Day 41 plan: Lend georgio 50." {
		t.Fatalf("hybrid plan: %+v %+v", p, p.Plan)
	}
}

// The rehearsal's panel still lagged a day: the gym and the mod send waking_up before the
// morning snapshot, and it had already moved the builder's day.
func TestMorningAfterWakingUp(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	var got []Event
	for _, l := range []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r"}),
		snapshot("morning", 1, 100, 500, nil),
		ev("waking_up", 2, map[string]any{}),
		snapshot("morning", 2, 140, 520, nil),
	} {
		line, _ := json.Marshal(l)
		evs, err := b.Feed(line)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, evs...)
	}
	var mornings []int
	for _, e := range got {
		if e.Kind == KindThought && strings.Contains(e.Text, "begins") {
			mornings = append(mornings, e.Day)
			if e.Clock == nil {
				t.Errorf("morning without a clock: %+v", e)
			}
		}
	}
	if len(mornings) != 2 || mornings[1] != 2 {
		t.Errorf("mornings = %v, want [1 2]", mornings)
	}
}
