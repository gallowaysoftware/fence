package strategy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func writePersona(t *testing.T, dir, id, text string) string {
	t.Helper()
	p := filepath.Join(dir, id+".md")
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadPersona(t *testing.T) {
	dir := t.TempDir()
	path := writePersona(t, dir, "miser", "# The Miser\nHoard cash; zz_style_miser.\n")
	writePersona(t, dir, "huge", strings.Repeat("x", MaxPersonaBytes+1))
	writePersona(t, dir, "blank", "#\n  \n")
	writePersona(t, dir, "noted", "# The Miser\nHoard cash.\n\n## Notes (not in any prompt)\n"+strings.Repeat("zz_notes ", MaxPersonaBytes))
	tests := []struct {
		name, dir, ref, want, wantErr string
	}{
		{"id", dir, "miser", "The Miser\nHoard cash; zz_style_miser.", ""},
		{"path", "", path, "The Miser\nHoard cash; zz_style_miser.", ""},
		{"missing", dir, "nobody", "", "no such file"},
		{"too big", dir, "huge", "", "the limit is 800"},
		{"empty", dir, "blank", "", "is empty"},
		{"notes are not the prompt", dir, "noted", "The Miser\nHoard cash.", ""},
		{"not an id", dir, "The Miser", "", "not an id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadPersona(tt.dir, tt.ref)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

// TestPersonaFiles: every shipped persona loads by id within the size limit.
func TestPersonaFiles(t *testing.T) {
	dir := filepath.Join("..", "..", DefaultPersonaDir)
	paths, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil || len(paths) < 8 {
		t.Fatalf("%d personas in %s (%v), want at least 8", len(paths), dir, err)
	}
	for _, p := range paths {
		id := strings.TrimSuffix(filepath.Base(p), ".md")
		if _, err := LoadPersona(dir, id); err != nil {
			t.Error(err)
		}
	}
}

func TestPlanPromptStyle(t *testing.T) {
	sc := loadScenario(t, "rent-tomorrow-low-cash")
	plain, err := BuildPlanPrompt(sc.Input())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(plain, "YOUR STYLE") {
		t.Fatalf("style section without a persona:\n%s", plain)
	}
	sc.Persona = "The Miser\nzz_style_miser"
	styled, err := BuildPlanPrompt(sc.Input())
	if err != nil {
		t.Fatal(err)
	}
	style := strings.Index(styled, "YOUR STYLE (the plan follows it unless survival is at stake):\nThe Miser\nzz_style_miser\n")
	state, write := strings.Index(styled, "TODAY: day 27"), strings.Index(styled, "Write the plan for day 27.")
	if style < 0 || !(state < style && style < write) {
		t.Fatalf("style not between the state and the request:\n%s", styled)
	}
	if strings.Replace(styled, "\nYOUR STYLE (the plan follows it unless survival is at stake):\nThe Miser\nzz_style_miser\n", "", 1) != plain {
		t.Fatal("the style changed more of the prompt than its own section")
	}

	for _, persona := range []string{"", "zz_style_miser"} {
		f := &fakeLLM{}
		in := sc.Input()
		in.Persona = persona
		if _, _, err := DraftPlan(context.Background(), f, in, 0); err != nil {
			t.Fatal(err)
		}
		sys := f.requests()[0].System
		if persona == "" && sys != planSystem {
			t.Errorf("system prompt changed without a persona")
		}
		if persona != "" && sys != planSystem+"\n"+styleRule {
			t.Errorf("system prompt with a persona lacks the style rule")
		}
	}
}

func loadScenario(t *testing.T, name string) Scenario {
	t.Helper()
	scs, err := LoadScenarios("testdata/days")
	if err != nil {
		t.Fatal(err)
	}
	for _, sc := range scs {
		if sc.Name == name {
			return sc
		}
	}
	t.Fatalf("no scenario %s", name)
	return Scenario{}
}

// TestStrategistPersona: the default persona reaches every plan; a sim_run event overrides it
// for its session (and the run a reload joins); an unknown, path-like or hidden-only persona
// leaves the default in place.
func TestStrategistPersona(t *testing.T) {
	dir := t.TempDir()
	writePersona(t, dir, "miser", "# The Miser\nzz_style_miser\n")
	writePersona(t, dir, "spender", "# The Spender\nzz_style_spender\n")
	outside := writePersona(t, t.TempDir(), "outside", "zz_style_outside\n")

	simRun := func(session, data, hidden string) protocol.Event {
		ev := protocol.Event{SessionID: session, Kind: "sim_run", Data: json.RawMessage(data)}
		if hidden != "" {
			ev.Hidden = json.RawMessage(hidden)
		}
		return ev
	}
	morning := func(session string, day int) protocol.Event {
		ev := snapshotEvent("morning", day, 1000)
		ev.SessionID = session
		return ev
	}
	tests := []struct {
		name, deflt string
		events      []protocol.Event
		want        string
		absent      []string
	}{
		{"no persona", "", nil, "", []string{"YOUR STYLE"}},
		{"default", "zz_style_default", nil, "zz_style_default", nil},
		{"sim_run override", "zz_style_default", []protocol.Event{simRun("s1", `{"seed":3,"persona":"miser"}`, "")}, "zz_style_miser", []string{"zz_style_default"}},
		{"override without a default", "", []protocol.Event{simRun("s1", `{"persona":"spender"}`, "")}, "zz_style_spender", nil},
		{"other session's override", "zz_style_default", []protocol.Event{simRun("s9", `{"persona":"miser"}`, "")}, "zz_style_default", []string{"zz_style_miser"}},
		{"unknown persona", "zz_style_default", []protocol.Event{simRun("s1", `{"persona":"nobody"}`, "")}, "zz_style_default", nil},
		{"path refused", "zz_style_default", []protocol.Event{simRun("s1", `{"persona":"`+outside+`"}`, "")}, "zz_style_default", []string{"zz_style_outside"}},
		{"hidden ignored", "zz_style_default", []protocol.Event{simRun("s1", `{"seed":3}`, `{"persona":"miser"}`)}, "zz_style_default", []string{"zz_style_miser"}},
		{"sim_run without persona", "zz_style_default", []protocol.Event{simRun("s1", `{"seed":3}`, "")}, "zz_style_default", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeLLM{}
			s := New(Options{LLM: f, Persona: tt.deflt, PersonaDir: dir})
			defer s.Close()
			for _, ev := range tt.events {
				s.Event(ev.SessionID, ev)
			}
			s.Event("s1", morning("s1", 10))
			s.wg.Wait()
			reqs := f.planRequests()
			if len(reqs) != 1 {
				t.Fatalf("%d model calls, want 1", len(reqs))
			}
			prompt := reqs[0].Messages[0].Content
			if tt.want != "" && !strings.Contains(prompt, "YOUR STYLE (the plan follows it unless survival is at stake):\n") {
				t.Errorf("no style section")
			}
			if tt.want != "" && !strings.Contains(prompt, tt.want) {
				t.Errorf("prompt lacks %q", tt.want)
			}
			for _, a := range tt.absent {
				if strings.Contains(prompt, a) || strings.Contains(reqs[0].System, a) {
					t.Errorf("request contains %q", a)
				}
			}
		})
	}

	// A reload joins the run and keeps the override its first session set.
	f := &fakeLLM{}
	s := New(Options{LLM: f, Persona: "zz_style_default", PersonaDir: dir})
	defer s.Close()
	s.Event("s1", simRun("s1", `{"persona":"miser"}`, ""))
	s.Event("s1", morning("s1", 10))
	s.wg.Wait()
	s.Event("s2", morning("s2", 11))
	s.wg.Wait()
	reqs := f.planRequests()
	if len(reqs) != 2 || !strings.Contains(reqs[1].Messages[0].Content, "zz_style_miser") {
		t.Fatalf("the reloaded session lost the run's persona")
	}
}

// TestPlanPromptRoute: a route adds its own section before the style and its rule to the
// system prompt, and drops the persona's buy order, which the route's base policy replaces.
func TestPlanPromptRoute(t *testing.T) {
	sc := loadScenario(t, "rent-tomorrow-low-cash")
	persona := "The Miser\nPath: zz_path\nBuy: Showcase I by day 10; Landlord Grace\nPivot: zz_pivot\nNever: zz_never"
	sc.Persona = persona
	styled, err := BuildPlanPrompt(sc.Input())
	if err != nil {
		t.Fatal(err)
	}
	in := sc.Input()
	in.Route = "smelter: zz_route_brief"
	routed, err := BuildPlanPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	route, style := strings.Index(routed, "THIS RUN'S ROUTE (already in force):\nsmelter: zz_route_brief\n"), strings.Index(routed, "YOUR STYLE")
	if route < 0 || style < route || strings.Contains(routed, "zz_pivot") || strings.Contains(routed, "Showcase I by day 10") || !strings.Contains(routed, "zz_never") {
		t.Fatalf("route section, or the style without its buy order:\n%s", routed)
	}
	if !strings.Contains(styled, "Showcase I by day 10") {
		t.Fatal("without a route the persona keeps its buy order")
	}
	f := &fakeLLM{}
	if _, _, err := DraftPlan(context.Background(), f, in, 0); err != nil {
		t.Fatal(err)
	}
	if sys := f.requests()[0].System; sys != planSystem+"\n"+routeRule+"\n"+styleRule {
		t.Errorf("system prompt on a route: %q", sys[len(planSystem):])
	}
}
