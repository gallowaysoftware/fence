package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/format"
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
	ch := Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "smelter", "thesis": thesis})}}, m, Ask{})
	if ch.Route != "smelter" || ch.Composed || ch.Fallback != "" || ch.Start != "apprentice" || ch.Thesis != thesis {
		t.Errorf("menu pick: %+v", ch)
	}
	// A bad composition is repaired once.
	bad := ans(map[string]any{"compose": map[string]any{"name": "ratty", "base": "appraiser", "levers": map[string]any{"engine": "rats"}}, "thesis": thesis})
	good := ans(map[string]any{"compose": map[string]any{"name": "hoard_smelter", "base": "smelter", "levers": map[string]any{"speculate": map[string]any{"types": []string{"MATERIAL"}}}}, "thesis": thesis})
	ch = Choose(context.Background(), &fakeLLM{answers: []string{bad, good}}, m, Ask{})
	if !ch.Composed || ch.Route != "hoard_smelter" || ch.Fallback != "" {
		t.Errorf("repaired composition: %+v", ch)
	}
	if b, err := ch.RouteFile(m); err != nil || !strings.Contains(string(b), `"composed_from":"smelter"`) {
		t.Errorf("route file %s %v", b, err)
	}
	// Two bad answers fall back to the default; a fixed start keeps the menu to its routes.
	ch = Choose(context.Background(), &fakeLLM{answers: []string{bad, bad}}, m, Ask{})
	if ch.Route != DefaultRoute || ch.Fallback == "" {
		t.Errorf("fallback: %+v", ch)
	}
	ch = Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "smelter", "thesis": thesis}), "{}"}}, m, Ask{FixedStart: "farmer"})
	if ch.Route != "farmer" || ch.Fallback == "" || len(ch.Menu) != 1 {
		t.Errorf("fixed start: %+v", ch)
	}
}

func TestLeanPromptIsSmall(t *testing.T) {
	m := testMenu(t)
	full, lean := ChoosePrompt(m, Ask{}), ChoosePrompt(m, Ask{Lean: true})
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

// TestDeadlineOdds: every hard deadline format that leaves the route open has every menu route's
// odds, so the day-0 menu never shows a route without them.
func TestDeadlineOdds(t *testing.T) {
	m := testMenu(t)
	fs, err := format.List(filepath.Join("..", "..", format.DefaultDir))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range fs {
		if f.DeadlineDay == 0 || f.Difficulty != "hard" || f.Route != "" {
			continue
		}
		n++
		fo, ok := DeadlineOdds(f.Name)
		if !ok || fo.DeadlineDay != f.DeadlineDay || !slices.Equal(fo.Handicaps, f.Handicaps) {
			t.Errorf("%s: odds %+v", f.Name, fo)
			continue
		}
		for _, e := range m.WithDeadline(f).Entries {
			if o := e.Odds; o == nil || o.Runs < 60 || o.Wins > o.Runs || o.Lo > o.Pct() || o.Hi < o.Pct() {
				t.Errorf("%s %s: odds %+v", f.Name, e.Name, o)
			}
		}
	}
	if n < 2 {
		t.Errorf("%d deadline formats", n)
	}
}

func TestDeadlinePrompt(t *testing.T) {
	m := testMenu(t)
	f, err := format.Load(filepath.Join("..", "..", format.DefaultDir), "sprint_91")
	if err != nil {
		t.Fatal(err)
	}
	dm := m.WithDeadline(f)
	sm, _ := dm.Get("smelter")
	a := Ask{Format: &f}
	full := ChoosePrompt(dm, a)
	want := fmt.Sprintf("By day 91 under this format: bought in %d of %d runs", sm.Odds.Wins, sm.Odds.Runs)
	if !strings.Contains(full, "Deadline: the shop must be bought by day 91") || !strings.Contains(full, want) {
		t.Errorf("full prompt lacks the deadline or %q", want)
	}
	a.Lean = true
	lean := ChoosePrompt(dm, a)
	if len(lean)*2 > len(full) || len(lean) > len(ChoosePrompt(m, Ask{Lean: true}))*5/4 {
		t.Errorf("lean %d bytes, full %d", len(lean), len(full))
	}
	// The lean menu leads with the best odds.
	best := dm.byOdds()[0]
	menu := lean[strings.Index(lean, "THE MENU"):]
	if first := menu[strings.Index(menu, "\n- ")+3:]; !strings.HasPrefix(first, best.Name+" (") || !strings.Contains(first, fmt.Sprintf("; by day 91 %d %% (", best.Odds.Pct())) {
		t.Errorf("lean menu starts %q, want %s", first[:80], best.Name)
	}
	if m.WithDeadline(format.Format{}) != m {
		t.Error("no deadline, same menu")
	}
}
