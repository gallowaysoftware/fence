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
		"why": "what Mara wants, within the budget", "rule": "player:stage_for_buyer", "client": "Mara", "client_seq": 3, "hidden_used": []string{"item_exposable"}})
	stage["event"].(map[string]any)["hidden"] = map[string]any{"true_value": secret}
	evs := feedAll(t, b,
		ev("autopilot", 12, map[string]any{"day": 12, "phase": "morning", "action": "plan_day", "why": "rent 350 in 4 days"}),
		stage,
		ev("autopilot", 12, map[string]any{"day": 12, "phase": "customer", "action": "counter", "detail": "add_markup_10", "why": "add_markup_10 for about 132 (estimate 120)", "client": "Mara", "client_seq": 3}),
		ev("autopilot", 12, map[string]any{"day": 12, "phase": "customer", "action": "pause", "why": "you made a move (stage): the autopilot stopped so it cannot fight you"}),
		ev("autopilot_fault", 12, map[string]any{"day": 12, "phase": "customer", "state": "handover", "reason": "Ivo: appraisal customer, not dismissable",
			"message": "AUTOPILOT PAUSED — your turn: Ivo: appraisal customer, not dismissable. Play it by hand, then press Resume."}),
	)
	all, _ := json.Marshal(evs)
	if bytes.Contains(all, []byte(secret)) {
		t.Fatalf("a hidden value reached the feed: %s", all)
	}
	th := find(evs, KindThought)
	// The staging folds into the counter's line, which carries its hidden reads.
	if len(th) != 1 || th[0].Text != "Add markup 10% Canned beans (Mara): add_markup_10 for about 132 (estimate 120)." || !slices.Equal(th[0].HiddenUsed, []string{"item_exposable"}) {
		t.Fatalf("thoughts = %+v", th)
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

func TestBuilderCashFromMoneyEvents(t *testing.T) {
	sold := func(uid, cost int) map[string]any {
		return ev("money", 4, map[string]any{"kind": "item_sold", "item_uid": uid, "cost": cost})
	}
	outcome := map[string]any{"type": "outcome", "decision_id": "d1", "outcome": map[string]any{"game_day": 4,
		"result": map[string]any{"kind": "trade", "side": "sold", "items": []int{7}, "cash_delta": 25}}}
	tests := []struct {
		name  string
		lines []map[string]any
		want  int64
	}{
		{"autopilot sales", []map[string]any{sold(7, 25), sold(8, 12), ev("money", 4, map[string]any{"kind": "item_bought", "item_uid": 9, "cost": 10})}, 527},
		{"vending", []map[string]any{ev("money", 4, map[string]any{"kind": "vending_sale", "value": 18})}, 518},
		{"money then outcome", []map[string]any{sold(7, 25), outcome}, 525},
		{"outcome then money", []map[string]any{outcome, sold(7, 25)}, 525},
		{"bought then sold", []map[string]any{ev("money", 4, map[string]any{"kind": "item_bought", "item_uid": 7, "cost": 12}), sold(7, 18)}, 506},
		{"rent paid", []map[string]any{ev("money", 4, map[string]any{"kind": "rent_paid", "amount": 200, "credits_left": 300})}, 300},
		{"rent paid no credits left", []map[string]any{ev("money", 4, map[string]any{"kind": "rent_paid", "amount": 200})}, 300},
		{"service paid", []map[string]any{ev("money", 4, map[string]any{"kind": "service_paid", "service": "JANITORIAL_SERVICE", "cost": 14})}, 486},
		{"wild upgrade", []map[string]any{ev("money", 4, map[string]any{"kind": "wild_upgrade", "cash_spent": 150})}, 350},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder(Player{ID: "p"})
			feedAll(t, b, append([]map[string]any{snapshot("open", 4, 500, 2000, nil)}, tc.lines...)...)
			if got := b.Clock().Cash; got != tc.want {
				t.Fatalf("cash %d, want %d", got, tc.want)
			}
		})
	}
}

// rent_paid and service_paid used to leave the builder's cash untouched, so the
// panel drifted from the true value until the day_ended credits corrected it.
func TestMoneyEventsTrackCashUntilDayEnded(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	mid := feedAll(t, b,
		snapshot("morning", 4, 650, 2000, nil),
		ev("money", 4, map[string]any{"kind": "rent_paid", "amount": 200, "credits_left": 450}),
		ev("money", 4, map[string]any{"kind": "service_paid", "service": "INSURANCE_SERVICE", "cost": 14}),
	)
	if got := b.Clock().Cash; got != 436 {
		t.Fatalf("cash after rent and service %d, want 436", got)
	}
	var rentFound bool
	for _, e := range find(mid, KindMoney) {
		if strings.Contains(e.Text, "rent") && e.Amount == -200 {
			rentFound = true
		}
	}
	if !rentFound {
		t.Fatalf("no rent money event: %+v", mid)
	}
	dayEvs := feedAll(t, b, ev("money", 4, map[string]any{"kind": "day_ended", "day": 4, "net_worth": 2100, "credits": 436}))
	if got := b.Clock().Cash; got != 436 {
		t.Fatalf("cash after day_ended %d, want 436", got)
	}
	day := find(dayEvs, KindDay)
	if len(day) != 1 || day[0].Summary.CashDelta != -214 {
		t.Fatalf("day summary: %+v", day)
	}
}

func TestCounterLine(t *testing.T) {
	budget := int64(62)
	tests := []struct {
		name, detail, why string
		staged            []string
		dealt             bool
		want              string
	}{
		{"pass", "dismiss", "asking 392 (wholesale 330) against 392: over the plan's limits or the floor 245", []string{"Peat", "Soda"}, false,
			"Passed on Peat, Soda from Junker: asking 392 (330 for the lot), more than it will spend while keeping 245 for rent."},
		{"inputs", "buy_all", "engine inputs for 30 (estimate 30)", []string{"Scrap Metal", "Scrap Metal"}, false,
			"Bought 2× Scrap Metal from Junker for 30 (estimate 30): engine inputs are worth paying up for."},
		{"lot", "accept_wholesale", "wholesale 110 for an estimate of 129", nil, false,
			"Bought Junker's whole lot for 110 against an estimate of 129 (85%, room to resell)."},
		{"sold", "sell_all", "sell_all for about 39 (estimate 39)", []string{"Peat"}, false,
			"Sold Peat to Junker for about 39, the full estimate (after substance; budget 62)."},
		{"discount", "demand_discount_10", "demand_discount_10 (EV 26 vs 21)", nil, false,
			"Pushed Junker for 10% off: expected gain 26, walk-out risk included, beats 21 at their price."},
		{"after a deal", "dismiss", "nothing sellable staged", nil, true, ""},
		{"empty-handed", "dismiss", "nothing sellable staged", nil, false, "Junker left empty-handed: nothing in stock fits (after substance; budget 62)."},
		{"contraband", "dismiss", "contraband", []string{"Pills"}, false, "Turned Junker away: their goods are contraband."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder(Player{})
			b.clients[9] = client{name: "Junker", intent: "sell_and_buy", budget: &budget, interests: []string{"substance"}}
			b.staged[9] = tc.staged
			b.dealt[9] = tc.dealt
			got, ok := b.counterLine(9, "Junker", tc.detail, tc.why)
			if got != tc.want || ok != (tc.want != "") {
				t.Errorf("got %q (%v), want %q", got, ok, tc.want)
			}
		})
	}
}

func TestBuilderResumedSave(t *testing.T) {
	b := NewBuilder(Player{ID: "drift", Model: "m", PersonaName: "Magnate"})
	load := ev("day_snapshot", 5, map[string]any{"phase": "load", "run_id": "r1",
		"store": map[string]any{"cash": 207, "day": 5, "difficulty": "hard"},
		"perks": map[string]any{"starting": []string{"vending", "short_sleeper"}}})
	evs := feedAll(t, b, ev("game_loaded", 5, map[string]any{}), load, load,
		ev("run_start", 5, map[string]any{"run_id": "r1", "difficulty": "hard"}))
	starts := find(evs, KindRunStart)
	if len(starts) != 1 {
		t.Fatalf("run starts = %d, want 1 (the loaded save's, not repeated)", len(starts))
	}
	p := starts[0].Player
	if p == nil || p.Difficulty != "hard" || !slices.Equal(p.Perks, []string{"vending", "short_sleeper"}) || starts[0].Day != 5 {
		t.Fatalf("player = %+v, day %d", p, starts[0].Day)
	}
}

func TestBuilderWholesaleLot(t *testing.T) {
	player := Player{ID: "t"}
	lines := []string{
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":1,"kind":"customer_observed","game_day":1,"data":{"client_seq":1,"covered":true,"context":{"store":{"cash":400,"day":1},"client":{"name":"Scavenger","faction":"none","intent":"sell","wholesale":{"price":135,"discount_pct":25}},"counter":{"theirs":[],"yours_staged":[]}}}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":2,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":1,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":3,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":2,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":4,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":3,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":5,"kind":"day_snapshot","game_day":1,"data":{"phase":"close","store":{"cash":280,"day":1,"net_worth":1000},"inventory":[],"rent":null,"property":null,"reputation":{}}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":6,"kind":"customer_observed","game_day":2,"data":{"client_seq":2,"covered":true,"context":{"store":{"cash":280,"day":2},"client":{"name":"Other","faction":"none","intent":"buy","budget":10},"counter":{"theirs":[],"yours_staged":[]}}}}}`,
	}

	cases := []struct {
		name  string
		lines []string
		want  int64
	}{
		// Offer observed, singles bought at per-item prices, no accept_wholesale decision:
		// the player paid the singles, so the panel keeps the per-item sums (400 − 150)
		// and reclaims nothing.
		{"no reclaim without an acceptance", lines, 280},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := NewBuilder(player)
			var evs []Event
			for _, l := range tc.lines {
				out, err := b.Feed([]byte(l))
				if err != nil {
					t.Fatalf("feed: %v", err)
				}
				evs = append(evs, out...)
			}
			if b.clock.Cash != tc.want {
				t.Errorf("cash = %d, want %d", b.clock.Cash, tc.want)
			}
			if m := find(evs, KindMoney); len(m) != 0 {
				t.Errorf("money events = %+v, want none", m)
			}
		})
	}
}

func TestMorningReport(t *testing.T) {
	b := NewBuilder(Player{ID: "t"})
	l := line(t, ev("morning_report", 69, map[string]any{
		"day":       69,
		"night_log": []string{"Your insurance covered 120 of stolen goods."},
		"rent":      map[string]any{"due": 600, "days_left": 1},
		"cash":      2450,
		"net_worth": 6120,
		"calendar": []map[string]any{
			{"day": 70, "events": []map[string]any{{"id": "rent_day", "name": "Rent Day", "type": "innate"}}},
		},
	}))
	evs, err := b.Feed(l)
	if err != nil {
		t.Fatal(err)
	}
	m := find(evs, KindMorning)
	if len(m) != 1 {
		t.Fatalf("want 1 morning event, got %d", len(m))
	}
	e := m[0]
	if e.Morning == nil {
		t.Fatal("morning payload missing")
	}
	if e.Morning.Day != 69 || e.Morning.Cash != 2450 || e.Morning.NetWorth != 6120 {
		t.Errorf("morning = %+v", e.Morning)
	}
	if e.Morning.Rent.Due != 600 || e.Morning.Rent.DaysLeft != 1 {
		t.Errorf("rent = %+v", e.Morning.Rent)
	}
	if len(e.Morning.NightLog) != 1 {
		t.Errorf("night_log = %v", e.Morning.NightLog)
	}
	if len(e.Morning.Calendar) != 1 || len(e.Morning.Calendar[0].Events) != 1 {
		t.Errorf("calendar = %+v", e.Morning.Calendar)
	}
	if e.Clock == nil || e.Clock.Cash != 2450 || e.Clock.RentDue != 600 {
		t.Errorf("clock = %+v", e.Clock)
	}
	if e.Beat != BeatReport {
		t.Errorf("beat = %q", e.Beat)
	}
	if !strings.Contains(e.Text, "2,450") || !strings.Contains(e.Text, "600") {
		t.Errorf("text = %q", e.Text)
	}
}

// TestBuilderWholesaleLotAccepted: a lot the bridge's decision took gets its gap
// reclaimed once (exactly the sum minus the lot price); a fresh offer for the same
// name resets the gate, so the second lot's singles pay at per-item prices with no
// credit.
func TestBuilderWholesaleLotAccepted(t *testing.T) {
	player := Player{ID: "t"}
	lines := []string{
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":1,"kind":"customer_observed","game_day":1,"data":{"client_seq":1,"covered":true,"context":{"store":{"cash":400,"day":1},"client":{"name":"Scavenger","faction":"none","intent":"sell","wholesale":{"price":135,"discount_pct":25}},"counter":{"theirs":[],"yours_staged":[]}}}}}`,
		`{"type":"decision","ts":"t","decision_id":"d1","request":{"kind":"barter","game_day":1,"context":{"client":{"name":"Scavenger","faction":"none","intent":"sell","wholesale":{"price":135}},"counter":{"theirs":[],"yours_staged":[]}}},"response":{"rationale":"the lot is a bargain","source":"llm","choice":{"option":"accept_wholesale"}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":2,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":1,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":3,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":2,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":4,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":3,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		// A fresh offer for the same name; its singles are bought at per-item prices with
		// no bridge acceptance, so nothing is reclaimed for them.
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":5,"kind":"customer_observed","game_day":1,"data":{"client_seq":2,"covered":true,"context":{"store":{"cash":300,"day":1},"client":{"name":"Scavenger","faction":"none","intent":"sell","wholesale":{"price":135}},"counter":{"theirs":[],"yours_staged":[]}}}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":6,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":4,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
		`{"type":"event","ts":"t","event":{"session_id":"x","seq":7,"kind":"money","game_day":1,"data":{"kind":"item_bought","item_uid":5,"item_id":"bottled_water","client":"Scavenger","cost":50}}}`,
	}
	b := NewBuilder(player)
	var evs []Event
	for _, l := range lines {
		out, err := b.Feed([]byte(l))
		if err != nil {
			t.Fatalf("feed: %v", err)
		}
		evs = append(evs, out...)
	}
	m := find(evs, KindMoney)
	if len(m) != 1 {
		t.Fatalf("money events = %+v, want exactly one reclaim", m)
	}
	if m[0].Amount != 15 {
		t.Errorf("reclaim = %d, want 15 (150 − 135)", m[0].Amount)
	}
}

// TestBuilderDecisionRationaleNoClient: a decide line with a rationale but no client in
// the request must not panic in the thought handler (regression: cedd1aa dereferenced a
// nil counter client before the clientName fallback).
func TestBuilderDecisionRationaleNoClient(t *testing.T) {
	b := NewBuilder(Player{ID: "t"})
	l := line(t, map[string]any{"type": "decision", "ts": "t", "decision_id": "d1",
		"request":  map[string]any{"kind": "counter", "game_day": 1, "context": map[string]any{"counter": map[string]any{"theirs": []any{}, "yours_staged": []any{}}}},
		"response": map[string]any{"rationale": "hold for the morning rush", "source": "llm"}})
	if _, err := b.Feed(l); err != nil {
		t.Fatal(err)
	}
}
