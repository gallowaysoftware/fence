package strategy

import (
	"strings"
	"testing"
)

// TestPlanPromptWaterInput: a policy whose engine contains water tells the plan that raw
// vessels are input, not merchandise; a wine-furnace policy says nothing of it.
func TestPlanPromptWaterInput(t *testing.T) {
	sc := loadScenario(t, "rent-tomorrow-low-cash")
	tests := []struct {
		name, base string
		want       bool
	}{
		{"water furnace", `{"engine":"furnace+water"}`, true},
		{"wine furnace", `{"engine":"wine+furnace"}`, false},
		{"no engine", `{"cash_floor":300}`, false},
		{"unparseable", "not json", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := sc.Input()
			in.BaseDirectives = tt.base
			prompt, err := BuildPlanPrompt(in)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(prompt, "WATER INPUT"); got != tt.want {
				t.Errorf("prompt has the water-input fact = %v, want %v", got, tt.want)
			}
			if tt.want && !strings.Contains(prompt, "Never stage them for sale") {
				t.Error("fact is cut short")
			}
		})
	}
}

// TestPlanPromptRouteWaterFact: on a water route the fact sits with the route, before the
// style; off water it is absent.
func TestPlanPromptRouteWaterFact(t *testing.T) {
	sc := loadScenario(t, "rent-tomorrow-low-cash")
	in := sc.Input()
	in.Route = "water_baron: the water route"
	in.Persona = "The Baron\nNever: zz_never"
	in.BaseDirectives = `{"engine":"furnace+water"}`
	prompt, err := BuildPlanPrompt(in)
	if err != nil {
		t.Fatal(err)
	}
	route, fact, style := strings.Index(prompt, "THIS RUN'S ROUTE"), strings.Index(prompt, "WATER INPUT"), strings.Index(prompt, "YOUR STYLE")
	if route < 0 || fact < 0 || style < 0 || route > fact || fact > style {
		t.Fatalf("route %d, fact %d, style %d: the fact sits between the route and the style", route, fact, style)
	}
}
