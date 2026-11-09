package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Scenario is a hand-written morning (testdata/days/*.json): a day_snapshot event,
// optionally the day before, and a note on what a good plan does.
type Scenario struct {
	Name      string         `json:"-"`
	Note      string         `json:"note"`
	Yesterday *DayPlan       `json:"yesterday,omitempty"`
	Event     protocol.Event `json:"event"`
}

// LoadScenarios reads every *.json in dir, sorted by name.
func LoadScenarios(dir string) ([]Scenario, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	out := make([]Scenario, 0, len(paths))
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var sc Scenario
		if err := json.Unmarshal(raw, &sc); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if sc.Event.Kind != "day_snapshot" {
			return nil, fmt.Errorf("%s: event kind %q, want day_snapshot", p, sc.Event.Kind)
		}
		sc.Name = filepath.Base(p[:len(p)-len(".json")])
		out = append(out, sc)
	}
	return out, nil
}

// Input is the plan input the strategist would build for sc. Only the event's data is
// used: its hidden block never reaches a prompt.
func (sc Scenario) Input() PlanInput {
	return PlanInput{Day: sc.Event.GameDay, Snapshot: sc.Event.Data, Yesterday: sc.Yesterday}
}

// PlanResult is one scenario run against a model.
type PlanResult struct {
	Scenario string
	Prompt   string
	Plan     string
	Model    string
	Latency  time.Duration
	Err      error
}

// RunScenarios drafts a plan for each scenario with c, one at a time.
func RunScenarios(ctx context.Context, c llm.Client, scs []Scenario, maxTokens int) []PlanResult {
	out := make([]PlanResult, 0, len(scs))
	for _, sc := range scs {
		in := sc.Input()
		r := PlanResult{Scenario: sc.Name}
		r.Prompt, r.Err = BuildPlanPrompt(in)
		if r.Err == nil {
			start := time.Now()
			r.Plan, r.Model, r.Err = DraftPlan(ctx, c, in, maxTokens)
			r.Latency = time.Since(start)
		}
		out = append(out, r)
	}
	return out
}
