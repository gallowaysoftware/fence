package strategy

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// TestLivePlans drafts a plan for every testdata/days scenario on a real model and prints
// them for review. It calls the network, so it runs only with FENCE_LIVE=1:
//
//	FENCE_LIVE=1 go test ./internal/strategy -run TestLivePlans -v
//
// FENCE_LIVE_URL and FENCE_LIVE_MODEL default to drift; FENCE_LIVE_ONLY picks scenarios
// by substring, FENCE_LIVE_PROMPT=1 prints the prompts too.
func TestLivePlans(t *testing.T) {
	if os.Getenv("FENCE_LIVE") != "1" {
		t.Skip("set FENCE_LIVE=1 to call the model")
	}
	off, temp := false, 0.2
	c := &llm.OpenAI{
		BaseURL:       envOr("FENCE_LIVE_URL", "http://localhost:9002/v1"),
		Model:         envOr("FENCE_LIVE_MODEL", "qwen38fn-mlx"),
		OpenAIOptions: llm.OpenAIOptions{NoSchema: true, Thinking: &off, Temperature: &temp},
	}
	scs, err := LoadScenarios("testdata/days")
	if err != nil {
		t.Fatal(err)
	}
	if only := os.Getenv("FENCE_LIVE_ONLY"); only != "" {
		var keep []Scenario
		for _, sc := range scs {
			if strings.Contains(sc.Name, only) {
				keep = append(keep, sc)
			}
		}
		scs = keep
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for i, r := range RunScenarios(ctx, c, scs, 0) {
		var b strings.Builder
		fmt.Fprintf(&b, "=== %s (%s, %d bytes)\nwant: %s\n", r.Scenario, r.Latency.Round(100*time.Millisecond), len(r.Plan), scs[i].Note)
		if os.Getenv("FENCE_LIVE_PROMPT") == "1" {
			fmt.Fprintf(&b, "--- prompt\n%s\n--- plan\n", r.Prompt)
		}
		if r.Err != nil {
			t.Errorf("%s: %v", r.Scenario, r.Err)
		}
		b.WriteString(r.Plan)
		t.Log("\n" + b.String())
		if r.Err == nil {
			sc, err := DraftScorecard(ctx, c, scs[i].Event.GameDay, r.Plan, liveFacts, 0)
			if err != nil {
				t.Errorf("%s scorecard: %v", r.Scenario, err)
			}
			t.Logf("--- scorecard against a quiet day\n%s", sc)
		}
	}
}

// liveFacts is a deliberately unremarkable day, so the scorecard has to check each goal.
const liveFacts = "cash +310 from trades; reputation unchanged; 3 sale(s) +540, 1 purchase(s) -230, 0 walk-out(s), 1 dismissed; 7 customer(s)."

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
