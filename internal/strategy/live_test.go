package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
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
// by substring, FENCE_LIVE_PROMPT=1 prints the prompts too. FENCE_LIVE_PERSONAS plans each
// scenario once per persona id from docs/personas ("none" for no persona), e.g.
// FENCE_LIVE_PERSONAS=none,vintner,fence.
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
	if ids := os.Getenv("FENCE_LIVE_PERSONAS"); ids != "" {
		var crossed []Scenario
		for _, id := range strings.Split(ids, ",") {
			text := ""
			if id = strings.TrimSpace(id); id != "none" {
				if text, err = LoadPersona(filepath.Join("..", "..", DefaultPersonaDir), id); err != nil {
					t.Fatal(err)
				}
			}
			for _, sc := range scs {
				sc.Name += " @" + id
				sc.Persona = text
				crossed = append(crossed, sc)
			}
		}
		scs = crossed
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

// TestLiveWindfall drafts a plan, then a mid-day windfall delta for an engine item a seller
// offers, on a real model. FENCE_LIVE=1 as for TestLivePlans.
func TestLiveWindfall(t *testing.T) {
	if os.Getenv("FENCE_LIVE") != "1" {
		t.Skip("set FENCE_LIVE=1 to call the model")
	}
	off, temp := false, 0.2
	c := &llm.OpenAI{
		BaseURL:       envOr("FENCE_LIVE_URL", "http://localhost:9002/v1"),
		Model:         envOr("FENCE_LIVE_MODEL", "qwen38fn-mlx"),
		OpenAIOptions: llm.OpenAIOptions{NoSchema: true, Thinking: &off, Temperature: &temp},
	}
	cases := []struct {
		scenario, persona string
		item              counterItem
	}{
		{"furnace-scrap-no-flux", "smelter", counterItem{UID: ptr(int64(990)), ID: "furnace_module_blast", Name: "Blast Furnace Module", Price: ptr(int64(150)), EstimatedValue: ptr(int64(150))}},
		{"hot-stock-near-inspection-threshold", "fence", counterItem{UID: ptr(int64(991)), ID: "smuggler_bay", Name: "Smuggler Bay", Price: ptr(int64(250)), EstimatedValue: ptr(int64(250))}},
	}
	scs, err := LoadScenarios("testdata/days")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	for _, tc := range cases {
		var sc *Scenario
		for i := range scs {
			if scs[i].Name == tc.scenario {
				sc = &scs[i]
			}
		}
		if sc == nil {
			t.Fatalf("no scenario %s", tc.scenario)
		}
		if sc.Persona, err = LoadPersona(filepath.Join("..", "..", DefaultPersonaDir), tc.persona); err != nil {
			t.Fatal(err)
		}
		plan, _, err := DraftPlan(ctx, c, sc.Input(), 0)
		if err != nil {
			t.Fatalf("%s plan: %v", tc.scenario, err)
		}
		cash := ptr(int64(0))
		if s, err := parseSnapshot(sc.Event.Data); err == nil && s.Cash != nil {
			cash = s.Cash
		}
		w := windfall{item: tc.item, how: "offered", cash: cash}
		resp, err := c.Complete(ctx, llm.Request{System: windfallSystem, MaxTokens: windfallMaxTokens,
			Messages: []llm.Message{{Role: llm.RoleUser, Content: BuildWindfallPrompt(plan, sc.Persona, sc.Event.Data, w)}}})
		if err != nil {
			t.Fatalf("%s windfall: %v", tc.scenario, err)
		}
		t.Logf("\n=== %s @%s, windfall: %s\n--- plan\n%s\n--- delta\n%s", tc.scenario, tc.persona, w.text(sc.Event.Data), plan, TidyWindfall(resp.Text))
	}
}

// usageTap records each completion's usage block, for the cost of directives.
type usageTap struct {
	mu   sync.Mutex
	base http.RoundTripper
	got  []json.RawMessage
}

func (u *usageTap) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := u.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var v struct {
		Usage   json.RawMessage `json:"usage"`
		Timings json.RawMessage `json:"timings"`
	}
	json.Unmarshal(body, &v)
	u.mu.Lock()
	u.got = append(u.got, json.RawMessage(fmt.Sprintf(`{"usage":%s,"timings":%s}`, orNull(v.Usage), orNull(v.Timings))))
	u.mu.Unlock()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

func orNull(r json.RawMessage) string {
	if len(r) == 0 {
		return "null"
	}
	return string(r)
}

func (u *usageTap) take() []json.RawMessage {
	u.mu.Lock()
	defer u.mu.Unlock()
	g := u.got
	u.got = nil
	return g
}

// TestLiveDirectives drafts a plan and its directives for a few scenarios on a real model
// and logs the directives, what validation fixed, and each call's latency and usage.
// FENCE_LIVE=1 as for TestLivePlans; FENCE_LIVE_ONLY and FENCE_LIVE_PERSONAS apply.
func TestLiveDirectives(t *testing.T) {
	if os.Getenv("FENCE_LIVE") != "1" {
		t.Skip("set FENCE_LIVE=1 to call the model")
	}
	tap := &usageTap{base: http.DefaultTransport}
	off, temp := false, 0.2
	c := &llm.OpenAI{
		BaseURL:       envOr("FENCE_LIVE_URL", "http://localhost:9002/v1"),
		Model:         envOr("FENCE_LIVE_MODEL", "qwen38fn-mlx"),
		HTTP:          &http.Client{Transport: tap},
		OpenAIOptions: llm.OpenAIOptions{NoSchema: true, Thinking: &off, Temperature: &temp},
	}
	scs, err := LoadScenarios("testdata/days")
	if err != nil {
		t.Fatal(err)
	}
	only := envOr("FENCE_LIVE_ONLY", "rent-tomorrow-low-cash,hot-stock-near-inspection-threshold,calendar-threat-and-boom,wine-berries-two-bottles-no-yeast")
	persona := ""
	if id := os.Getenv("FENCE_LIVE_PERSONAS"); id != "" && id != "none" {
		if persona, err = LoadPersona(filepath.Join("..", "..", DefaultPersonaDir), id); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	for _, sc := range scs {
		if !slices.ContainsFunc(strings.Split(only, ","), func(o string) bool { return strings.Contains(sc.Name, o) }) {
			continue
		}
		sc.Persona = persona
		tap.take()
		r := RunScenarios(ctx, c, []Scenario{sc}, 0)[0]
		if r.Err != nil {
			t.Errorf("%s: %v", sc.Name, r.Err)
			continue
		}
		calls := tap.take()
		js, _ := json.MarshalIndent(r.Directives.Directives, "", " ")
		t.Logf("\n=== %s\nplan %s, directives %s; model keys %v; err %v\ncalls (plan..., directives):\n%s\n--- directives\n%s\n--- fixes\n%s",
			sc.Name, r.Latency.Round(100*time.Millisecond), r.DirectivesLatency.Round(100*time.Millisecond), r.Directives.FromModel, r.Directives.Err,
			joinRaw(calls), js, strings.Join(r.Directives.Fixes, "\n"))
	}
}

func joinRaw(rs []json.RawMessage) string {
	var b strings.Builder
	for _, r := range rs {
		b.Write(r)
		b.WriteByte('\n')
	}
	return b.String()
}
