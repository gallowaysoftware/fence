package agency

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/route"
)

func testMenu(t *testing.T) *Menu {
	t.Helper()
	m, err := LoadMenu(filepath.Join("..", "..", route.DefaultDir))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// TestMenuHasLabStats: every shipped route carries its lab record, so a promoted route
// cannot reach the menu without its numbers.
func TestMenuHasLabStats(t *testing.T) {
	m := testMenu(t)
	if len(m.Entries) < 20 {
		t.Fatalf("%d routes", len(m.Entries))
	}
	for _, e := range m.Entries {
		if e.Lab.Wins == "" || e.Lab.MedianWinDay == 0 {
			t.Errorf("%s: no lab stats in agency/lab.json", e.Name)
		}
	}
	if !strings.Contains(m.Text(), "smelter (start apprentice") {
		t.Errorf("menu text: %s", m.Text())
	}
	if f := m.For("farmer"); len(f.Entries) != 1 || f.Entries[0].Name != "farmer" {
		t.Errorf("farmer start: %+v", f.Entries)
	}
}

func lev(s string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		panic(err)
	}
	return m
}

func TestCompositionValidate(t *testing.T) {
	m := testMenu(t)
	for _, tc := range []struct {
		name string
		c    Composition
		want string // "" for valid
	}{
		{"speculating smelter", Composition{Name: "hoard_smelter", Base: "smelter", Levers: lev(`{"speculate":{"types":["MATERIAL"],"max_days":7},"buy":{"no_singles":true}}`)}, ""},
		{"drop the permit", Composition{Name: "loose", Base: "appraiser", Levers: lev(`{"permit":null}`)}, ""},
		{"rats off the rancher", Composition{Name: "ratty", Base: "appraiser", Levers: lev(`{"engine":"wine+rats"}`)}, "rancher start"},
		{"bad engine", Composition{Name: "x1", Base: "appraiser", Levers: lev(`{"engine":"beer"}`)}, "directives.schema.json"},
		{"unknown lever", Composition{Name: "x2", Base: "appraiser", Levers: lev(`{"rocket":true}`)}, "not a lever"},
		{"mob", Composition{Name: "x3", Base: "appraiser", Levers: lev(`{"bargain":"aggressive"}`)}, "bargain"},
		{"menu name", Composition{Name: "smelter", Base: "appraiser", Levers: lev(`{"bays":3}`)}, "already a menu route"},
		{"no levers", Composition{Name: "x4", Base: "appraiser"}, "none changed"},
		{"bad type", Composition{Name: "x5", Base: "appraiser", Levers: lev(`{"specialty":{"types":["GOLD"]}}`)}, "want a list"},
		{"fountain without water", Composition{Name: "x6", Base: "appraiser", Levers: lev(`{"fountain":"buy"}`)}, "water engine"},
		{"start", Composition{Name: "x7", Base: "appraiser", Start: "astronaut", Levers: lev(`{"bays":3}`)}, "start"},
	} {
		errs := tc.c.Validate(m)
		got := strings.Join(errs, "; ")
		if tc.want == "" && got != "" || tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCompositionFile(t *testing.T) {
	m := testMenu(t)
	c := Composition{Name: "hoard_smelter", Base: "smelter", Levers: lev(`{"speculate":{"types":["MATERIAL"]},"sell":{"min_pct":60},"permit":null}`)}
	b, err := c.File(m, "", "A brief.")
	if err != nil {
		t.Fatal(err)
	}
	r, err := route.Parse("hoard_smelter.json", b)
	if err != nil {
		t.Fatal(err)
	}
	var d map[string]any
	json.Unmarshal([]byte(r.Base), &d)
	sell := d["sell"].(map[string]any)
	if r.Start != "apprentice" || r.Persona != "smelter" || d["engine"] != "furnace" || d["permit"] != nil || sell["min_pct"] != 60.0 || sell["liquidate_min_pct"] != 78.0 || d["speculate"] == nil {
		t.Errorf("%+v %v", r, d)
	}
}

type fakeLLM struct{ answers []string }

func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	a := f.answers[0]
	f.answers = f.answers[1:]
	return llm.Response{Text: a, Model: "fake"}, nil
}

const thesis = "A metal shop that hoards its ingots and materials until the market spikes, because the furnace is the only engine that pays in cash before the buyout and the vintner's cellar is slow money; I will watch the material events on the calendar and sell into every rise."

func TestChoose(t *testing.T) {
	m := testMenu(t)
	ans := func(v map[string]any) string { b, _ := json.Marshal(v); return string(b) }
	ch := Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "smelter", "thesis": thesis})}}, m, "", nil, false)
	if ch.Route != "smelter" || ch.Composed || ch.Fallback != "" || ch.Start != "apprentice" || ch.Thesis != thesis {
		t.Errorf("menu pick: %+v", ch)
	}
	// A bad composition is repaired once.
	bad := ans(map[string]any{"compose": map[string]any{"name": "ratty", "base": "appraiser", "levers": map[string]any{"engine": "rats"}}, "thesis": thesis})
	good := ans(map[string]any{"compose": map[string]any{"name": "hoard_smelter", "base": "smelter", "levers": map[string]any{"speculate": map[string]any{"types": []string{"MATERIAL"}}}}, "thesis": thesis})
	ch = Choose(context.Background(), &fakeLLM{answers: []string{bad, good}}, m, "", nil, false)
	if !ch.Composed || ch.Route != "hoard_smelter" || ch.Fallback != "" {
		t.Errorf("repaired composition: %+v", ch)
	}
	if b, err := ch.RouteFile(m); err != nil || !strings.Contains(string(b), `"composed_from":"smelter"`) {
		t.Errorf("route file %s %v", b, err)
	}
	// Two bad answers fall back to the default; a fixed start keeps the menu to its routes.
	ch = Choose(context.Background(), &fakeLLM{answers: []string{bad, bad}}, m, "", nil, false)
	if ch.Route != DefaultRoute || ch.Fallback == "" {
		t.Errorf("fallback: %+v", ch)
	}
	ch = Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "smelter", "thesis": thesis}), "{}"}}, m, "farmer", nil, false)
	if ch.Route != "farmer" || ch.Fallback == "" || len(ch.Menu) != 1 {
		t.Errorf("fixed start: %+v", ch)
	}
}

func TestLeanPromptIsSmall(t *testing.T) {
	m := testMenu(t)
	full, lean := ChoosePrompt(m, "", nil, false), ChoosePrompt(m, "", nil, true)
	if len(lean)*2 > len(full) || !strings.Contains(lean, "- smelter (apprentice): ") {
		t.Errorf("lean %d bytes, full %d:\n%s", len(lean), len(full), lean)
	}
}

func TestLiftMeta(t *testing.T) {
	c := Composition{Name: "x", Base: "smelter", Levers: lev(`{"perks":"vending+short_sleeper","persona":"shark","start":"apprentice","bays":3}`)}
	liftMeta(&c)
	if c.Start != "apprentice" || c.Perks != "vending+short_sleeper" || len(c.Levers) != 1 || len(c.Validate(testMenu(t))) != 0 {
		t.Errorf("%+v %v", c, c.Validate(testMenu(t)))
	}
}
