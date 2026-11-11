package strategy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestFinishDelta(t *testing.T) {
	tests := []struct {
		name  string
		patch func(m map[string]any)
		raw   string
		base  string
		want  string
		fix   string
	}{
		{"defaults", nil, `{}`, "", `{"day":41}`, ""},
		{"no reason, no change", nil, `{"bargain":"never"}`, "", `{"day":41}`, "no reason given"},
		{"one reason for all", nil, `{"bargain":"never","why":"Lower Levels near -20"}`, "", `{"day":41,"bargain":"never"}`, ""},
		{"a reasoned change", nil, `{"bargain":"never","why":{"bargain":"Lower Levels near -20"}}`, "", `{"day":41,"bargain":"never"}`, ""},
		{"supplier day keeps no cap", func(m map[string]any) { store(m)["day"] = 40 },
			`{"cash_floor":900,"buy":{"max_spend":100},"why":{"cash_floor":"x","buy":"y"}}`, "", `{"day":40,"buy":{}}`, "supplier day"},
		{"bribe reverted when affordable", func(m map[string]any) { store(m)["cash"] = 3400 },
			`{"permit":"bribe","why":{"permit":"save cash"}}`, "", `{"day":41}`, "bribe reverted"},
		{"cash upgrade without margin dropped", nil,
			`{"upgrades_buy":["AUG_SCANNER"],"why":{"upgrades_buy":"augs"}}`, "", `{"day":41}`, "dropped AUG_SCANNER"},
		{"mortgage levers pass", nil,
			`{"mortgage":"take","mortgage_margin":0.6,"mortgage_window":[100,300],"why":"payments are covered"}`, "",
			`{"day":41,"mortgage":"take","mortgage_margin":0.6,"mortgage_window":[100,300]}`, ""},
		{"a bad mortgage window is dropped", nil, `{"mortgage_window":[300,100],"why":"x"}`, "", `{"day":41}`, "mortgage_window: dropped"},
		{"decline short of the buyout becomes later", nil, `{"mortgage":"decline","why":"x"}`, "", `{"day":41,"mortgage":"later"}`, "decline is final"},
		{"appraisal can be turned off", nil, `{"appraisal":"off","why":"x"}`, `{"appraisal":"join"}`, `{"day":41,"appraisal":"off"}`, ""},
		{"engine keeps the base's furnace", nil, `{"engine":"wine","why":"keep wine"}`, `{"engine":"wine+furnace"}`, `{"day":41}`, "engine: wine reverted"},
		{"engine may add to the base", nil, `{"engine":"wine+water+furnace","why":"water"}`, `{"engine":"wine+furnace"}`, `{"day":41,"engine":"wine+water+furnace"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := finishDelta(json.RawMessage(tt.raw), nil, shop(t, tt.patch), int(*shop(t, tt.patch).Day), tt.base)
			got, _ := json.Marshal(r.Directives)
			eq(t, string(got), tt.want)
			if fixes := strings.Join(r.Fixes, "\n"); tt.fix != "" && !strings.Contains(fixes, tt.fix) {
				t.Errorf("fixes lack %q:\n%s", tt.fix, fixes)
			}
		})
	}
}

func TestBaseNote(t *testing.T) {
	if baseNote("") != "" {
		t.Error("no base should add nothing")
	}
	if n := baseNote(`{"engine":"wine+furnace"}`); !strings.Contains(n, `{"engine":"wine+furnace"}`) {
		t.Errorf("base note %q lacks the base", n)
	}
}

func TestFinishDeltaKeepsWhy(t *testing.T) {
	r := finishDelta(json.RawMessage(`{"bargain":"never","why":{"bargain":"Lower Levels near -20","permit":" "}}`), nil, shop(t, nil), 41, "")
	if len(r.Why) != 1 || r.Why["bargain"] != "Lower Levels near -20" {
		t.Errorf("why = %v", r.Why)
	}
	r = finishDelta(json.RawMessage(`{"bargain":"never","why":"one reason"}`), nil, shop(t, nil), 41, "")
	if r.Why["*"] != "one reason" {
		t.Errorf("why = %v", r.Why)
	}
}

func TestLintDecline(t *testing.T) {
	near := func(cash, nw, price int) func(m map[string]any) {
		return func(m map[string]any) {
			store(m)["cash"], store(m)["net_worth"] = cash, nw
			m["property"] = map[string]any{"buyout_price": price}
		}
	}
	gain := int64(3000)
	tests := []struct {
		name  string
		patch func(m map[string]any)
		gain  *int64
		want  string
	}{
		{"short of the price: later", near(6000, 9000, 26000), nil, "later"},
		{"weekly gain closes the gap: decline stands", near(14000, 18000, 26000), &gain, "decline"},
		{"cash already covers: decline stands", near(28000, 29000, 26000), nil, "decline"},
		{"no price known: later", nil, nil, "later"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := shop(t, tt.patch)
			s.weekGain = tt.gain
			d := protocol.Directives{Mortgage: "decline"}
			fixes := lintDecline(&d, s)
			if d.Mortgage != tt.want {
				t.Errorf("mortgage %q, want %q (fixes %v)", d.Mortgage, tt.want, fixes)
			}
			if (tt.want == "later") != (len(fixes) == 1) {
				t.Errorf("fixes %v", fixes)
			}
		})
	}
}
