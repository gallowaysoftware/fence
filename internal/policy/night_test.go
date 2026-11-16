package policy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// nightReq builds a night request from the context JSON, offering the listed options.
func nightReq(t *testing.T, ctx string, options ...string) *protocol.DecideRequest {
	t.Helper()
	req := &protocol.DecideRequest{SessionID: "s", DecisionID: "d", Kind: "night", GameDay: 30, DeadlineMS: 20000, Context: json.RawMessage(ctx)}
	for _, id := range options {
		req.Options = append(req.Options, protocol.Option{ID: id, Label: id})
	}
	return req
}

func TestNightParse(t *testing.T) {
	tests := []struct {
		name    string
		ctx     string
		wantErr bool
	}{
		{"baseline parses", `{"cash":2400,"wound":0}`, false},
		{"not an object", `42`, true},
		{"no cash", `{"day":30,"wound":1}`, true},
		{"cash not a number", `{"cash":"a lot"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseNight(json.RawMessage(tt.ctx))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNightHeuristic(t *testing.T) {
	tests := []struct {
		name    string
		ctx     string
		options []string
		want    string
		inWhy   string
	}{
		{"wants non-empty buys the trip",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":5,"wound":0,` +
				`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"},{"id":"dumping_ground","name":"Dumping Ground"}],` +
				`"jackson_wants":{"furnace":2}}`,
			[]string{optGoJacks, optScav, optStayIn}, optGoJacks, "will buy"},
		{"wants list both in the rationale",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":5,"wound":0,` +
				`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"}],` +
				`"jackson_wants":{"furnace":2,"water_purifier":1}}`,
			[]string{optGoJacks, optStayIn}, optGoJacks, "furnace x2, water_purifier x1"},
		{"no wants, rent far, cash to spare: still Jackson",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":5,"wound":0,` +
				`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"},{"id":"dumping_ground","name":"Dumping Ground"}],` +
				`"jackson_wants":{}}`,
			[]string{optGoJacks, optScav, optStayIn}, optGoJacks, "well clear"},
		{"no wants, rent far, thin cash: the dump",
			`{"day":30,"cash":700,"rent_due":500,"rent_days_left":5,"wound":0,` +
				`"locations":[{"id":"dumping_ground","name":"Dumping Ground"}],"jackson_wants":{}}`,
			[]string{optScav, optStayIn}, optScav, "pays for itself free"},
		{"no wants, wound 0, rent far: the dump",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":5,"wound":0,` +
				`"locations":[{"id":"dumping_ground","name":"Dumping Ground"}],"jackson_wants":{}}`,
			[]string{optScav, optStayIn}, optScav, ""},
		{"wound 4, no wants: stay in",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":5,"wound":4,` +
				`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"},{"id":"dumping_ground","name":"Dumping Ground"}],` +
				`"jackson_wants":{}}`,
			[]string{optScav, optStayIn}, optStayIn, ""},
		{"wound 1, no wants, rent far: the dump",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":5,"wound":1,` +
				`"locations":[{"id":"dumping_ground","name":"Dumping Ground"}],"jackson_wants":{}}`,
			[]string{optScav, optStayIn}, optScav, ""},
		{"rent due tomorrow, no wants: stay in",
			`{"day":30,"cash":2400,"rent_due":500,"rent_days_left":1,"wound":0,` +
				`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"},{"id":"dumping_ground","name":"Dumping Ground"}],` +
				`"jackson_wants":{}}`,
			[]string{optScav, optStayIn}, optStayIn, "rent 500 is due in 1"},
		{"unparseable context defers", `{"cash": "a lot"}`, []string{optGoJacks, optStayIn}, protocol.OptionDefer, ""},
		{"no option fits",
			`{"cash":2400,"wound":0,"locations":[]}`, []string{optCallWild}, protocol.OptionDefer, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := nightReq(t, tt.ctx, tt.options...)
			d := Heuristic{}.Decide(context.Background(), req)
			if d.Choice.Option != tt.want {
				t.Fatalf("choice %q, want %q (why: %s)", d.Choice.Option, tt.want, d.Rationale)
			}
			if tt.inWhy != "" && !strings.Contains(d.Rationale, tt.inWhy) {
				t.Fatalf("rationale %q, want containing %q", d.Rationale, tt.inWhy)
			}
			if err := CheckChoice(req, d.Choice); err != nil {
				t.Fatalf("the heuristic's own answer fails the check: %v", err)
			}
		})
	}
}

func TestCheckNightChoice(t *testing.T) {
	both := `{"cash":2400,"rent_due":500,"rent_days_left":5,"wound":0,` +
		`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"},{"id":"dumping_ground","name":"Dumping Ground"}]}`
	tests := []struct {
		name    string
		ctx     string
		option  string
		wantErr string
	}{
		{"jackson with the location", both, optGoJacks, ""},
		{"scavenge with the location", both, optScav, ""},
		{"jackson without the location",
			`{"cash":2400,"wound":0,"locations":[{"id":"dumping_ground","name":"Dumping Ground"}]}`,
			optGoJacks, `no "dr_jackson"`},
		{"scavenge without the location",
			`{"cash":2400,"wound":0,"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"}]}`,
			optScav, `no "dumping_ground"`},
		{"call_wild is never location-gated", `{"cash":2400,"wound":0,"locations":[]}`, optCallWild, ""},
		{"stay_in is never location-gated", `{"cash":2400,"wound":0,"locations":[]}`, optStayIn, ""},
		{"defer", both, protocol.OptionDefer, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := nightReq(t, tt.ctx, tt.option)
			err := CheckChoice(req, protocol.Choice{Option: tt.option})
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("err %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestJudgeable(t *testing.T) {
	tests := []struct {
		kind string
		want bool
	}{
		{"barter", true}, {"stage", true}, {"inspect", true}, {"aug_check", true},
		{"night", true}, {"services", false}, {"window", false}, {"unknown_kind", false},
	}
	for _, tt := range tests {
		if got := Judgeable(tt.kind); got != tt.want {
			t.Errorf("Judgeable(%q) = %v, want %v", tt.kind, got, tt.want)
		}
	}
}

func TestNightRender(t *testing.T) {
	var b strings.Builder
	c, err := parseNight(json.RawMessage(`{` +
		`"day":30,"cash":2400,"rent_due":500,"rent_days_left":3,"wound":4,` +
		`"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"},{"id":"dumping_ground","name":"Dumping Ground"}],` +
		`"services":[{"id":"INSURANCE_SERVICE","cost":146,"using":true},{"id":"JANITORIAL_SERVICE","cost":37,"using":false}],` +
		`"jackson_wants":{"furnace":2,"water_purifier":1},"bag_items":4,"plan_notes":"stock the furnace"}`))
	if err != nil {
		t.Fatal(err)
	}
	renderNight(&b, c)
	got := b.String()
	for _, want := range []string{
		"Night (day 30)",
		"cash 2400, rent 500 due, 3 night(s) left, wound: fairly serious (4)",
		"Location: Dr. Jackson's",
		"Location: Dumping Ground",
		"Services on tonight: insurance_service (146/night)",
		"Dr. Jackson can stock: furnace x2, water_purifier x1",
		"Bag: 4 item(s)",
		"The plan: stock the furnace",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("render missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "janitorial_service") {
		t.Errorf("a service not in use was rendered:\n%s", got)
	}

	// The notes: one per option, from the facts.
	note := nightNotes(c)
	if got, want := note(optGoJacks, ""), "worth it when jackson_wants is non-empty or cash covers a kit"; !strings.Contains(got, want) {
		t.Errorf("go_jackson note = %q, want containing %q", got, want)
	}
	if got := note(optScav, ""); got != "free loot, but wound odds (night log shows the state) and the junk split" {
		t.Errorf("scavenge note = %q", got)
	}
	if got := note(optStayIn, ""); got != "no cost, no gain" {
		t.Errorf("stay_in note = %q", got)
	}
	if got := note(optCallWild, ""); got != "browse Wild's network for a phone order" {
		t.Errorf("call_wild note = %q", got)
	}

	// The options carry the notes through OptionNotes too.
	req := nightReq(t, `{"cash":2400,"wound":0,"locations":[{"id":"dr_jackson","name":"Dr. Jackson's"}],"jackson_wants":{"furnace":2}}`,
		optGoJacks, optStayIn)
	if notes := OptionNotes(req); notes[optGoJacks] == "" || notes[optStayIn] != "no cost, no gain" {
		t.Errorf("OptionNotes = %v", notes)
	}
}
