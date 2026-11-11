package show

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func beats(evs []Event) map[string][]Event {
	out := map[string][]Event{}
	for _, e := range evs {
		if e.Beat != "" {
			out[e.Beat] = append(out[e.Beat], e)
		}
	}
	return out
}

func boardMemberAt(seq int) map[string]any {
	return ev("customer_observed", 8, map[string]any{"client_seq": seq, "context": map[string]any{"client": map[string]any{"name": boardMember, "intent": "dialogue"}}})
}

func choice(day, seq int, text string) map[string]any {
	return ev("human_action", day, map[string]any{"client_seq": seq, "action": "dialogue_choice", "text": text})
}

// A gym game's beats, in the shapes the t7 hard league logged them.
func TestBuilderBeatsGym(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	evs := feedAll(t, b,
		ev("run_start", 1, map[string]any{"run_id": "r", "difficulty": "hard", "aug_detection": "perfect"}),
		snapshot("morning", 8, 900, 3000, nil),
		boardMemberAt(48),
		choice(8, 48, "[Join the board (170 Credits)]"),
		ev("appraisal", 9, map[string]any{"client_seq": 55, "type": "hand", "estimate": 120, "read_value": 115, "fee": 38, "grade": 1, "slip_true_value": 115}),
		ev("appraisal", 9, map[string]any{"client_seq": 56, "type": "fuel", "estimate": 90, "refused": "too_high"}),
		ev("human_action", 16, map[string]any{"client_seq": 110, "action": "dialogue_choice", "text": "[Accept (-212 Credits)]",
			"history": []string{"Appraiser Board Member: Good to see you again.", "Appraiser Board Member: Congratulations, today we promote you to Appraiser."}}),
		choice(16, 110, "[Decline for now]"),
		boardMemberAt(140),
		choice(23, 140, "[Accept (-255 Credits)]"),
		ev("customer_observed", 13, map[string]any{"client_seq": 90, "context": map[string]any{"client": map[string]any{"name": "Thief", "intent": "buy"}}}),
		ev("human_action", 13, map[string]any{"client_seq": 90, "action": "unholster"}),
		ev("aug", 13, map[string]any{"event": "good_kill", "name": "Thief", "cash_delta": 101}),
		snapshot("morning", 63, 7590, 14000, nil),
		ev("money", 63, map[string]any{"kind": "mortgage_taken", "price": 24650, "down_payment": 3000, "weekly": 1666, "weeks": 2}),
		ev("money", 70, map[string]any{"kind": "mortgage_payment", "amount": 1666, "credits_left": 12285}),
		ev("money", 77, map[string]any{"kind": "mortgage_payment", "amount": 1666, "credits_left": 13000}),
		ev("money", 77, map[string]any{"kind": "mortgage_paid_off"}),
		ev("run_end", 77, map[string]any{"ending": "buyout_mortgage", "day": 77, "cash": 13000}),
	)
	all, _ := json.Marshal(evs)
	if bytes.Contains(all, []byte(secret)) || bytes.Contains(all, []byte("115")) {
		t.Fatalf("a hidden or slip value reached the feed: %s", all)
	}
	bt := beats(evs)
	if j := bt[BeatAppraisalJoin]; len(j) != 1 || j[0].Amount != -170 || j[0].Level != LevelMoment {
		t.Errorf("join = %+v", j)
	}
	p := bt[BeatPromotion]
	if len(p) != 2 || !strings.Contains(p[0].Text, "to Appraiser, for 212") || !strings.Contains(p[1].Text, "rank 2, for 255") {
		t.Errorf("promotions = %+v", p)
	}
	var appraised []string
	for _, e := range find(evs, KindThought) {
		if strings.HasPrefix(e.Text, "Appraisal") {
			appraised = append(appraised, e.Text)
		}
	}
	if strings.Join(appraised, "|") != "Appraisal (hand): estimate 120, accepted, fee 38.|Appraisal (fuel): estimate 90, refused as too high." {
		t.Errorf("appraisals = %q", appraised)
	}
	var gun Event
	for _, e := range find(evs, KindThought) {
		if strings.HasPrefix(e.Text, "Gun out") {
			gun = e
		}
	}
	if gun.Text != "Gun out: the Thief is an aug." || len(gun.HiddenUsed) != 1 || gun.HiddenUsed[0] != "aug_truth" {
		t.Errorf("the gym's perfect detection must be marked: %+v", gun)
	}
	if a := bt[BeatAug]; len(a) != 1 || a[0].Amount != 101 {
		t.Errorf("aug = %+v", a)
	}
	s := bt[BeatMortgageSigned]
	if len(s) != 1 || s[0].Amount != -3000 || s[0].Clock.MortgageWeekly != 1666 || s[0].Text != "Signed the mortgage: 3,000 down, then 1,666 a week for 2 weeks, and the shop is ours." {
		t.Errorf("signed = %+v", s)
	}
	pay := bt[BeatMortgagePayment]
	if len(pay) != 2 || pay[0].Text != "Mortgage payment made: 1,666. 1 week to go." || pay[0].Clock.Cash != 12285 ||
		pay[1].Text != "The last mortgage payment: 1,666. Nothing more owed on the shop." {
		t.Errorf("payments = %+v", pay)
	}
	if e := bt[BeatBuyout]; len(e) != 1 || e[0].Kind != KindRunEnd || !e[0].End.Won {
		t.Errorf("buyout = %+v", e)
	}
	if m := find(evs, KindMoney); m[len(m)-1].Level != LevelNotable || !m[len(m)-1].Clock.Owned {
		t.Errorf("paid off = %+v (a notable: the run end is the spoken beat)", m[len(m)-1])
	}
}

// The live autopilot names the board's dialogues by client id, and a resume after a handover is the
// AI taking the shop back.
func TestBuilderBeatsLive(t *testing.T) {
	b := NewBuilder(Player{ID: "live"})
	ap := func(action, why, detail string) map[string]any {
		return ev("autopilot", 16, map[string]any{"day": 16, "phase": "customer", "action": action, "why": why, "detail": detail, "client": boardMember})
	}
	evs := feedAll(t, b,
		ap("dialogue_choice", "Join the board (170 Credits)", "appraisalBoardIntro [2]"),
		ap("dialogue_choice", "Accept (-212 Credits)", "appraisalBoardPromotion [0]"),
		ap("dialogue_choice", "Decline for now", "appraisalBoardPromotion [1]"),
		ap("dialogue_choice", "Pay Rent (220)", "landlord [0]"),
		ap("resume", "resumed by the player (overlay)", ""),
		ap("raise_gun", "perfectly still, twitching: an aug's tells", ""),
		ev("autopilot_fault", 16, map[string]any{"state": "faulted", "reason": "move did not take: Lamp → case"}),
		ap("resume", "resumed by the player (F10)", ""),
		ap("resume", "resumed by the player (F10)", ""),
	)
	bt := beats(evs)
	if len(bt[BeatAppraisalJoin]) != 1 || len(bt[BeatPromotion]) != 1 || !strings.Contains(bt[BeatPromotion][0].Text, "rank 1, for 212") {
		t.Errorf("board beats = %+v", bt)
	}
	if h := bt[BeatHandover]; len(h) != 1 || h[0].Text != "The autopilot stopped (move did not take: Lamp → case). Over to the human." {
		t.Errorf("handover = %+v", h)
	}
	if tb := bt[BeatTakeBack]; len(tb) != 1 || tb[0].Level != LevelMoment {
		t.Errorf("take back = %+v (only the first resume after a stop)", tb)
	}
	if th := find(evs, KindThought); len(th) != 3 || th[2].Level != LevelNotable {
		t.Errorf("the other choices stay thoughts; the gun raised is notable: %+v", th)
	}
}

func TestMood(t *testing.T) {
	loss, gain := int64(-900), int64(900)
	cases := []struct {
		name string
		c    *Clock
		net  *int64
		want string
		cue  string
	}{
		{"no clock", nil, nil, "", ""},
		{"an estimate gone stale", &Clock{Cash: -840, MortgageWeekly: 1666}, nil, "", ""},
		{"owned", &Clock{Owned: true, Cash: 10}, nil, "triumphant", "Gloat"},
		{"rent not covered", &Clock{Cash: 200, RentDue: 300, RentDays: 2}, nil, "broke", "Sweat"},
		{"arrears", &Clock{Cash: 5000, RentDue: 300, Arrears: 10}, nil, "broke", "Sweat"},
		{"barely", &Clock{Cash: 500, RentDue: 300}, &loss, "tight", "still stings"},
		{"mortgage not covered", &Clock{Cash: 1500, RentDue: 400, MortgageWeekly: 1666}, nil, "broke", "Sweat"},
		{"near the buyout", &Clock{Cash: 2000, RentDue: 400, NetWorth: 16000, BuyoutPrice: 24000}, &gain, "flush", "swagger"},
		{"steady", &Clock{Cash: 1400, RentDue: 400, NetWorth: 5000, BuyoutPrice: 24000}, &gain, "steady", "good one"},
		// Day 31 of the full replay: cash six rents deep but a fifth of the way to the buyout.
		{"rich in rent, far from the buyout", &Clock{Cash: 1700, RentDue: 266, NetWorth: 5000, BuyoutPrice: 24000}, nil, "steady", "Even-keeled"},
		{"rich in rent, a third of the way", &Clock{Cash: 2500, RentDue: 292, NetWorth: 8000, BuyoutPrice: 24000}, nil, "flush", "swagger"},
	}
	for _, c := range cases {
		m := MoodOf(c.c, c.net)
		if m.Name != c.want || !strings.Contains(m.Cue, c.cue) {
			t.Errorf("%s: %+v, want %s with %q", c.name, m, c.want, c.cue)
		}
		if strings.ContainsAny(m.Cue, "0123456789") {
			t.Errorf("%s: a mood must carry no numbers: %q", c.name, m.Cue)
		}
	}
}

// The narrator's character follows the money: the same persona swaggers when flush and sweats
// when broke, and a beat tells it what kind of moment it is.
func TestNarratorMoodAndBeat(t *testing.T) {
	f := &fakeLLM{reply: "Another one off the deed."}
	n := &Narrator{LLM: f, PersonaD: "../../docs/personas", Persona: "shark"}
	at := func() int64 { return timeNowMS() }
	rich := Event{Seq: 1, Kind: KindDay, Level: LevelNotable, At: at(), Summary: &DaySummary{NetWorthDelta: 1200},
		Clock: &Clock{Cash: 9000, NetWorth: 18000, BuyoutPrice: 24000, MortgageWeekly: 1666, RentDays: 5}}
	pay := Event{Seq: 2, Kind: KindMoney, Level: LevelMoment, Beat: BeatMortgagePayment, At: at(), Day: 77, Amount: -1666,
		Text: "Mortgage payment made: 1,666. 9 weeks to go.", Clock: &Clock{Cash: 7334, NetWorth: 18000, BuyoutPrice: 24000, MortgageWeekly: 1666, MortgageWeeksLeft: 9, RentDays: 7}}
	said := narrate(t, n, rich, pay)
	if len(said) != 1 || said[0].Say.Mood != "flush" {
		t.Fatalf("said %+v", said)
	}
	if sys := f.lastSystem(); !strings.Contains(sys, "street hustler") || !strings.Contains(sys, "swagger") || !strings.Contains(sys, "Yesterday was a good one") {
		t.Errorf("system = %q", sys)
	}
	if p := f.asked[0]; !strings.Contains(p, "a mortgage payment just went out") || !strings.Contains(p, "No rent while the mortgage runs. Next mortgage payment: 1,666 due in 7 days") {
		t.Errorf("prompt = %q", p)
	}

	f = &fakeLLM{reply: "Over to you, boss."}
	n = &Narrator{LLM: f, PersonaD: "../../docs/personas", Persona: "shark"}
	broke := Event{Seq: 3, Kind: KindAutopilot, Level: LevelMoment, Beat: BeatHandover, At: at(), Day: 20,
		Autopilot: &AutopilotState{State: "handover", Reason: "Ivo: appraisal customer", NeedsHuman: true},
		Text:      "Over to the human: Ivo: appraisal customer.", Clock: &Clock{Cash: 100, RentDue: 300, RentDays: 1}}
	said = narrate(t, n, broke)
	if len(said) != 1 || said[0].Say.Mood != "broke" || !strings.Contains(f.lastSystem(), "Sweat") || !strings.Contains(f.asked[0], "over to the human") {
		t.Errorf("handover: %+v / %q / %q", said, f.lastSystem(), f.asked)
	}
	if Priority(broke) <= Priority(pay) {
		t.Errorf("a handover must outrank a payment: %d vs %d", Priority(broke), Priority(pay))
	}
}

// Of sixteen weekly payments the first, every fourth and the last two are spoken.
func TestMortgagePaymentLevels(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	lines := []map[string]any{snapshot("morning", 63, 7000, 14000, nil), ev("money", 63, map[string]any{"kind": "mortgage_taken", "down_payment": 3000, "weekly": 1666, "weeks": 16})}
	for i := 0; i < 16; i++ {
		lines = append(lines, ev("money", 70+7*i, map[string]any{"kind": "mortgage_payment", "amount": 1666}))
	}
	var left []int
	for _, e := range feedAll(t, b, lines...) {
		if e.Beat == BeatMortgagePayment && e.Level == LevelMoment {
			left = append(left, e.Clock.MortgageWeeksLeft)
		}
	}
	if fmt.Sprint(left) != "[15 12 8 4 1 0]" {
		t.Errorf("spoken payments at weeks left %v", left)
	}
}
