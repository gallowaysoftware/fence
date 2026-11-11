package strategy

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func mustSnap(t *testing.T, raw string) *snapshot {
	t.Helper()
	s, err := parseSnapshot(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCoolDaysContraband(t *testing.T) {
	tests := []struct {
		name  string
		heat  int64
		level *int64
		want  int64
	}{
		{"plain 100", 100, nil, 6},
		{"plain 18", 18, nil, 1},
		{"plain 19", 19, nil, 2},
		{"critical contraband cools as fast", 100, ptr(int64(4)), 6},
		{"low contraband cools as fast", 50, ptr(int64(1)), 3},
		{"clean", 0, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			it := item{StolenHeat: &tt.heat, ContrabandLevel: tt.level}
			if got := it.coolDays(); got != tt.want {
				t.Fatalf("coolDays = %d, want %d", got, tt.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }

func TestHotExposure(t *testing.T) {
	s := mustSnap(t, `{"store":{"cash":500},"security":{"trafficking_value":40},"inventory":[
		{"uid":1,"id":"smuggler_bay","name":"Smuggler Bay"},
		{"uid":2,"name":"Ring","estimated_value":200,"stolen_heat":50,"where":"inside:1"},
		{"uid":3,"name":"Watch","estimated_value":100,"stolen_heat":50},
		{"uid":4,"name":"Pin","estimated_value":40,"stolen_heat":25,"where":"hidden"},
		{"uid":5,"name":"Lamp","estimated_value":300,"stolen_heat":100,"where":"counter"},
		{"uid":6,"name":"Cold","estimated_value":90,"stolen_heat":0}]}`)
	n, credits, rep := s.hotExposure()
	// Watch: 50% of 100 and of 4 rep; Lamp: 100% of 300 and of 5 rep.
	if n != 2 || credits != 350 || rep != 7 {
		t.Fatalf("exposure n=%d credits=%d rep=%d, want 2, 350, 7", n, credits, rep)
	}
	var b strings.Builder
	s.renderHotExposure(&b)
	for _, w := range []string{"2 item(s) outside a smuggler bay (1 inside one)", "about 350 credits and 7 Security rep", "under 50 (now 40)"} {
		if !strings.Contains(b.String(), w) {
			t.Errorf("missing %q in %q", w, b.String())
		}
	}
}

func TestLossConditions(t *testing.T) {
	tests := []struct {
		name      string
		snap      string
		day       int
		first     string
		imminent  []string
		absent    []string
		mitigated []string
	}{
		{name: "rent short tomorrow, no grace",
			snap:     `{"store":{"cash":100},"rent":{"rent":600,"days_left":1,"landlord_grace":false},"reputation":{"lower_levels":{"value":30},"black_market":{"value":-95}},"upgrades":{"owned":[],"available":[{"id":"LANDLORD_GRACE","name":"Landlord Grace","cost":100}]}}`,
			first:    "rent: 600 due in 1 day(s), cash 100 is SHORT 500",
			imminent: []string{"rent"}, absent: []string{"Black Market"},
			mitigated: []string{"BUY Landlord Grace tonight (100: favor 0 + cash 100)"}},
		{name: "evidence near sentencing with crime over 1500",
			snap:     `{"store":{"cash":2000},"security":{"evidence":"92/100","projected_sentence":"Execution","crimes":[{"id":"FENCING","amount":1600}]},"upgrades":{"available":[{"id":"EVIDENCE_REMOVAL","name":"Evidence Removal","cost":500}]}}`,
			first:    "arrest by sentencing: evidence 92/100",
			imminent: []string{"sentencing", "execution"}, mitigated: []string{"BUY Evidence Removal tonight (500"}},
		{name: "critical contraband outside a bay with inspections on",
			snap:  `{"store":{"cash":200},"security":{"trafficking_value":300},"inventory":[{"uid":9,"name":"C4","contraband_level":4}]}`,
			first: "arrest: CRITICAL contraband C4 #9", imminent: []string{"arrest"}},
		{name: "faction close to -100",
			snap:  `{"store":{"cash":200},"reputation":{"revolution":{"value":-97},"upper_levels":{"value":-99},"security":{"value":-40}}}`,
			first: "Revolution at -100 = the shop bombed: now -97, 3 away", imminent: []string{"revolution"},
			absent: []string{"Upper Levels"}},
		{name: "aug presence only on hard",
			snap: `{"store":{"cash":200,"difficulty":"hard"}}`, day: 20,
			first: "aug presence 15/100 (0 escape(s) +15 each, 0 good kill(s) -10 each; history incomplete, at least this): 5 more escape(s) survivable, SAFE"},
		{name: "reactor owned", snap: `{"store":{"cash":200},"inventory":[{"uid":3,"id":"reactor_core","name":"Reactor"}]}`,
			first: "reactor: Reactor #3 can explode overnight"},
		{name: "grace owned ranks rent last", snap: `{"store":{"cash":10},"rent":{"rent":600,"days_left":1,"landlord_grace":true},"reputation":{"lower_levels":{"value":0}}}`,
			first: "Lower Levels at -100", absent: []string{"aug", "reactor", "sentinel"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := mustSnap(t, tt.snap)
			cs := s.lossConditions(tt.day)
			if len(cs) == 0 || !strings.HasPrefix(cs[0].text, tt.first) {
				t.Fatalf("first condition %+v, want prefix %q", cs, tt.first)
			}
			imm := map[string]bool{}
			for _, c := range cs {
				if c.imminent {
					imm[c.key] = true
				}
			}
			for _, k := range tt.imminent {
				if !imm[k] {
					t.Errorf("%s not imminent: %+v", k, cs)
				}
			}
			var b strings.Builder
			s.renderLossConditions(&b, tt.day)
			for _, a := range tt.absent {
				if strings.Contains(b.String(), a) {
					t.Errorf("rendered %q:\n%s", a, b.String())
				}
			}
			for _, m := range tt.mitigated {
				if !strings.Contains(b.String(), m) {
					t.Errorf("missing mitigation %q:\n%s", m, b.String())
				}
			}
		})
	}
}

func TestLintLossAndUpgrades(t *testing.T) {
	persona := "Path: fence.\nBuy: Inspection Early Warning (300) by day 7; Criminal Networking (350) by day 14; wine rack at cash 1500."
	snap := func(extra string) *snapshot {
		return mustSnap(t, `{"store":{"cash":800,"wild_favor":100},"rent":{"rent":200,"days_left":5,"landlord_grace":false},
			"reputation":{"security":{"value":-20}},`+extra+`}`)
	}
	offer := `"upgrades":{"available":[{"id":"CRIMINEL_NETWORK","name":"Criminal Networking","cost":350},{"id":"INSPECTION_INFORMANT","name":"Inspection Early Warning","cost":300},{"id":"WINE_RACK","name":"Wine Rack","cost":350}]}`
	tests := []struct {
		name, extra, plan string
		want              string
	}{
		{"ignores the next affordable buy", offer, "GOALS\n- sell stock", "names Inspection Early Warning and the shop can afford it tonight (favor 100 + cash 200)"},
		{"buys it", offer, "GOALS\n- BUY tonight: Inspection Early Warning (300: favor 100 + cash 200, style)", ""},
		{"cash-gated step is skipped", `"upgrades":{"available":[{"id":"WINE_RACK","name":"Wine Rack","cost":350}]}`, "GOALS\n- sell", ""},
		{"unaffordable", `"upgrades":{"available":[{"id":"INSPECTION_INFORMANT","name":"Inspection Early Warning","cost":3000}]}`, "GOALS\n- sell", ""},
		{"mitigation beats the path", `"security":{"evidence":"85/100"},"upgrades":{"available":[{"id":"INSPECTION_INFORMANT","name":"Inspection Early Warning","cost":300},{"id":"EVIDENCE_REMOVAL","name":"Evidence Removal","cost":500}]}`,
			"GOALS\n- BUY tonight: Inspection Early Warning\nRISK\n- evidence 85: no crime", "Evidence Removal pushes it back"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lintUpgrades(tt.plan, snap(tt.extra), persona, 30)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("err %v, want %q", err, tt.want)
			}
		})
	}

	loss := []struct {
		name, plan string
		want       string
	}{
		{"ignores imminent rent", "GOALS\n- sell the watch", "rent: 200 due in 1 day(s), cash 100 is SHORT 100"},
		{"addresses rent", "RISK\n- rent 200 tomorrow: sell the watch", ""},
	}
	s := mustSnap(t, `{"store":{"cash":100},"rent":{"rent":200,"days_left":1,"landlord_grace":false}}`)
	for _, tt := range loss {
		t.Run(tt.name, func(t *testing.T) {
			errs := lintLoss(tt.plan, s, 30)
			if tt.want == "" && len(errs) > 0 || tt.want != "" && (len(errs) == 0 || !strings.Contains(errs[0].Error(), tt.want)) {
				t.Fatalf("errs %v, want %q", errs, tt.want)
			}
		})
	}
}

func TestAugTrack(t *testing.T) {
	esc := "one of your contacts says an Aug visited your store yesterday"
	tests := []struct {
		name       string
		difficulty string
		mornings   map[int][]string
		kills      []int
		day        int
		want       AugTrack
		cond       string
		imminent   bool
	}{
		{name: "hard: escapes and a kill", difficulty: "hard",
			mornings: map[int][]string{13: {}, 14: {esc}, 19: {esc, "rent paid", esc}}, kills: []int{20}, day: 21,
			want: AugTrack{Escapes: 3, GoodKills: 1, Presence: 50},
			cond: "aug presence 50/100 (3 escape(s) +15 each, 1 good kill(s) -10 each): 3 more escape(s) survivable, SAFE"},
		{name: "challenge: one escape from the end", difficulty: "challenge",
			mornings: map[int][]string{13: {esc, esc}, 14: {esc, esc}, 15: {esc}}, day: 15,
			want: AugTrack{Escapes: 5, Presence: 95},
			cond: "aug presence 95/100 (5 escape(s) +16 each, 0 good kill(s) -8 each): 0 more escape(s) survivable, CRITICAL", imminent: true},
		{name: "kills floor at 0", difficulty: "normal",
			mornings: map[int][]string{13: {}}, kills: []int{13, 14}, day: 15,
			want: AugTrack{GoodKills: 2, Presence: 0}},
		{name: "a gap in history is flagged", difficulty: "hard",
			mornings: map[int][]string{20: {esc}}, day: 20,
			want: AugTrack{Escapes: 1, Presence: 30, Partial: true}, cond: "history incomplete"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var h augHistory
			for d, l := range tt.mornings {
				h.noteMorning(d, l)
			}
			for _, d := range tt.kills {
				h.noteKill(d)
			}
			s := mustSnap(t, `{"store":{"cash":100,"difficulty":"`+tt.difficulty+`"}}`)
			s.aug = h.trackFor(s, tt.day)
			if s.aug == nil || *s.aug != tt.want {
				t.Fatalf("track %+v, want %+v", s.aug, tt.want)
			}
			c, ok := s.augCondition(tt.day)
			if tt.cond == "" {
				var b strings.Builder
				s.renderAugs(&b, tt.day)
				if ok || !strings.HasPrefix(b.String(), "Augs: presence") {
					t.Fatalf("normal difficulty: condition %v, render %q", ok, b.String())
				}
				return
			}
			if !ok || !strings.Contains(c.text, tt.cond) || c.imminent != tt.imminent {
				t.Fatalf("condition %+v, want %q imminent %v", c, tt.cond, tt.imminent)
			}
		})
	}
	if !isGoodKill([]byte(`{"event":"good_kill","client_seq":3}`)) || isGoodKill([]byte(`{"event":"wrongful_kill"}`)) {
		t.Fatal("isGoodKill")
	}
}

func TestStrategistAugPresence(t *testing.T) {
	f := &fakeLLM{}
	s := New(Options{LLM: f})
	defer s.Close()
	morning := func(day int, log string) {
		s.Event("s1", event("day_snapshot", day, `{"phase":"morning","store":{"cash":500,"day":`+strconv.Itoa(day)+`,"difficulty":"hard"},"night_log":[`+log+`]}`))
	}
	morning(13, ``)
	morning(14, `"one of your contacts says an Aug visited your store yesterday"`)
	// The outcome repeats the event when a decision was asked; only the event counts.
	s.Outcome("s1", protocol.Outcome{DecisionID: "k", GameDay: 14, Result: json.RawMessage(`{"kind":"aug","result":"good_kill"}`)})
	s.Event("s1", event("aug", 14, `{"event":"good_kill","client_seq":3}`))
	morning(15, ``)
	s.wg.Wait()
	reqs := f.requests()
	last := reqs[len(reqs)-1].Messages[0].Content
	if !strings.Contains(last, "aug presence 20/100 (1 escape(s) +15 each, 1 good kill(s) -10 each)") {
		t.Fatalf("morning prompt lacks the rebuilt presence:\n%s", last)
	}
}
