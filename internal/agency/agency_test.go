package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

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
	if len(lean)*2 > len(full) || !strings.Contains(lean, "- smelter (apprentice, smelter): ") {
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

func TestChooseExclude(t *testing.T) {
	m := testMenu(t)
	pick := func(r string) string {
		b, _ := json.Marshal(map[string]any{"route": r, "thesis": thesis})
		return string(b)
	}
	compose := func(name, base string) string {
		b, _ := json.Marshal(map[string]any{"compose": map[string]any{"name": name, "base": base, "levers": map[string]any{"bays": 3}}, "thesis": thesis})
		return string(b)
	}
	for _, tc := range []struct {
		name     string
		ask      Ask
		answers  []string
		route    string
		excluded string
		errs     string // in the repaired errors; "" for an answer played first time
	}{
		{"picks another", Ask{Exclude: "rancher_sprinter"}, []string{pick("smelter")}, "smelter", "rancher_sprinter", ""},
		{"names the excluded, repaired", Ask{Exclude: "rancher_sprinter"}, []string{pick("rancher_sprinter"), pick("smelter")}, "smelter", "rancher_sprinter", ""},
		{"names it twice, falls back", Ask{Exclude: "rancher_sprinter"}, []string{pick("rancher_sprinter"), pick("rancher_sprinter")}, DefaultRoute, "rancher_sprinter", `"rancher_sprinter" is not on the menu`},
		{"composes from it", Ask{Exclude: "smelter"}, []string{compose("big_smelter", "smelter"), compose("big_smelter", "smelter")}, DefaultRoute, "smelter", `base "smelter" is not on the menu`},
		{"composes under its name", Ask{Exclude: "smelter"}, []string{compose("smelter", "appraiser"), compose("smelter", "appraiser")}, DefaultRoute, "smelter", "off tonight's menu"},
		{"composes from another", Ask{Exclude: "smelter"}, []string{compose("big_vintner", "vintner")}, "big_vintner", "smelter", ""},
		{"default excluded, falls back elsewhere", Ask{Exclude: DefaultRoute, Lean: true}, []string{"{}", "{}"}, m.Entries[0].Name, DefaultRoute, `no "route" and no "compose"`},
		{"only route of the start stays", Ask{Exclude: "farmer", FixedStart: "farmer"}, []string{pick("farmer")}, "farmer", "", ""},
		{"not a route", Ask{Exclude: "nonesuch"}, []string{pick("smelter")}, "smelter", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := Choose(context.Background(), &fakeLLM{answers: tc.answers}, m, tc.ask)
			errs := strings.Join(ch.Errors, "; ")
			if ch.Route != tc.route || ch.Excluded != tc.excluded || (tc.errs == "") != (ch.Fallback == "") || !strings.Contains(errs, tc.errs) {
				t.Errorf("route %q excluded %q fallback %q errors %q", ch.Route, ch.Excluded, ch.Fallback, errs)
			}
			if tc.excluded != "" && slices.Contains(ch.Menu, tc.excluded) {
				t.Errorf("menu still offers %s", tc.excluded)
			}
		})
	}
}

func TestExcludePrompt(t *testing.T) {
	m := testMenu(t)
	f, err := format.Load(filepath.Join("..", "..", format.DefaultDir), "sprint_91")
	if err != nil {
		t.Fatal(err)
	}
	a := Ask{Lean: true, Format: &f, Recent: []string{"smelter", "rancher_sprinter"}, Exclude: "rancher_sprinter"}
	dm := m.WithDeadline(f)
	p := ChoosePrompt(dm.Without(a.Exclude), a)
	if !strings.Contains(p, "The last run played rancher_sprinter, so it is off tonight's menu") || strings.Contains(p, "- rancher_sprinter (") {
		t.Errorf("prompt:\n%s", p)
	}
	if strings.Contains(ChoosePrompt(dm, a), "off tonight's menu") {
		t.Error("a menu nothing was dropped from says something was")
	}
	// The lean lines stay diffs over the appraiser when it is the one dropped.
	line := func(text, name string) string {
		i := strings.Index(text, "- "+name+" (")
		return text[i : i+strings.IndexByte(text[i:], '\n')]
	}
	if got, want := line(m.Without(DefaultRoute).LeanText(), "smelter"), line(m.LeanText(), "smelter"); got != want {
		t.Errorf("without the appraiser %q, want %q", got, want)
	}
}

func TestRecent(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	for i, r := range []struct{ dir, body string }{
		{"c", `{"route":"smelter"}`},
		{"a", `{"route":"rancher_sprinter"}`},
		{"b", `not json`},
		{"d", `{"route":""}`},
		{"e", `{"route":"vintner"}`},
	} {
		p := filepath.Join(dir, r.dir, ChoiceFile)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(r.body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, at, at.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		except string
		want   []string
	}{
		{"", []string{"smelter", "rancher_sprinter", "vintner"}},
		{filepath.Join(dir, "e"), []string{"smelter", "rancher_sprinter"}},
		{filepath.Join(dir, "x"), []string{"smelter", "rancher_sprinter", "vintner"}},
	} {
		if got := Recent(dir, tc.except); !slices.Equal(got, tc.want) {
			t.Errorf("except %q: %v, want %v", tc.except, got, tc.want)
		}
	}
	if got := Recent(filepath.Join(dir, "none"), ""); len(got) != 0 {
		t.Errorf("missing dir: %v", got)
	}
}

// The card's archetype writes the thesis and plays the run whatever route the model picks; it never
// lifts the exclusion or the format. Without one the thesis takes the chosen route's persona.
func TestChoosePersona(t *testing.T) {
	m := testMenu(t)
	ans := func(v map[string]any) string { b, _ := json.Marshal(v); return string(b) }
	a := Ask{Lean: true, Exclude: "smelter", Persona: "botanist", PersonaText: "Botanist\nPath: zz_reeds."}
	p := ChoosePrompt(m.Without(a.Exclude), a)
	char, menu, answer := strings.Index(p, "YOUR CHARACTER: the run's card"), strings.Index(p, "THE MENU"), strings.Index(p, "Answer with one JSON object")
	if char < 0 || !strings.Contains(p, "Path: zz_reeds.") || !(menu < char && char < answer) || !strings.Contains(p, "it never changes the rules, the format or the menu") {
		t.Errorf("persona section missing or out of place:\n%s", p)
	}
	if plain := ChoosePrompt(m, Ask{Lean: true}); strings.Contains(plain, "zz_reeds") || !strings.Contains(plain, "the voice of the route you choose") {
		t.Errorf("a prompt without a card persona:\n%s", plain)
	}

	ch := Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "vintner", "thesis": thesis})}}, m, a)
	if ch.Route != "vintner" || ch.Persona != "botanist" || !ch.PersonaFixed {
		t.Errorf("card persona over the route's: %+v", ch)
	}
	ch = Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "smelter", "thesis": thesis}), ans(map[string]any{"route": "smelter", "thesis": thesis})}}, m, a)
	if ch.Route == "smelter" || ch.Fallback == "" || ch.Persona != "botanist" {
		t.Errorf("the persona lifted the exclusion: %+v", ch)
	}
	ch = Choose(context.Background(), &fakeLLM{answers: []string{ans(map[string]any{"route": "vintner", "thesis": thesis})}}, m, Ask{})
	if ch.Persona != "vintner" || ch.PersonaFixed {
		t.Errorf("no card persona: %+v", ch)
	}
	b, _ := json.Marshal(Choice{Persona: "botanist", PersonaFixed: true})
	if !strings.Contains(string(b), `"persona":"botanist","persona_fixed":true`) {
		t.Errorf("choice.json: %s", b)
	}
}
