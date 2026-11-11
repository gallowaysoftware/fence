package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/protocol/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	entries, err := schema.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".schema.json") {
			continue
		}
		b, err := schema.FS.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
		if err != nil {
			t.Fatal(err)
		}
		// Under its $id, so the summary's $ref finds the directives schema.
		if err := c.AddResource(schema.BaseURL+e.Name(), doc); err != nil {
			t.Fatal(err)
		}
	}
	sch, err := c.Compile(schema.BaseURL + name)
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

func validate(t *testing.T, sch *jsonschema.Schema, body string) {
	t.Helper()
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(inst); err != nil {
		t.Fatalf("response does not match the schema: %v\n%s", err, body)
	}
}

type funcSummarizer func(ctx context.Context, session string) (protocol.SummaryResponse, error)

func (f funcSummarizer) Summary(ctx context.Context, session string) (protocol.SummaryResponse, error) {
	return f(ctx, session)
}

func TestSummary(t *testing.T) {
	day := protocol.SummaryResponse{
		Day:        69,
		Today:      protocol.DayStats{Decisions: 7, Sales: 3, Purchases: 2, CashDelta: 415, Walkouts: 1, RepDelta: map[string]float64{"lower_levels": 0.8}},
		Lessons:    "- Keep 600 for rent.",
		LessonsDay: 68,
		// The server owns these; a summarizer cannot override them.
		SessionID: "s-other", Mode: protocol.ModeAct, Policy: "x", Model: "y",
	}
	zero := protocol.SummaryResponse{Today: protocol.DayStats{RepDelta: map[string]float64{}}}
	fixed := func(v protocol.SummaryResponse, err error) Summarizer {
		return funcSummarizer(func(context.Context, string) (protocol.SummaryResponse, error) { return v, err })
	}
	tests := []struct {
		name       string
		summarizer Summarizer
		query      func(sid string) string
		wantCode   int
		want       protocol.SummaryResponse // session, mode, policy and model are checked separately
		maxLatency time.Duration
	}{
		{"memory off", nil, url.QueryEscape, 200, zero, 0},
		{"memory on", fixed(day, nil), url.QueryEscape, 200, day, 0},
		{"nil rep delta is an object", fixed(protocol.SummaryResponse{Day: 3}, nil), url.QueryEscape, 200,
			protocol.SummaryResponse{Day: 3, Today: protocol.DayStats{RepDelta: map[string]float64{}}}, 0},
		{"partial on error", fixed(protocol.SummaryResponse{Day: 3}, errors.New("db locked")), url.QueryEscape, 200,
			protocol.SummaryResponse{Day: 3, Today: protocol.DayStats{RepDelta: map[string]float64{}}}, 0},
		{"slow summarizer honouring ctx", funcSummarizer(func(ctx context.Context, _ string) (protocol.SummaryResponse, error) {
			<-ctx.Done()
			return protocol.SummaryResponse{}, ctx.Err()
		}), url.QueryEscape, 200, zero, time.Second},
		{"slow summarizer ignoring ctx", funcSummarizer(func(context.Context, string) (protocol.SummaryResponse, error) {
			time.Sleep(2 * time.Second)
			return day, nil
		}), url.QueryEscape, 200, zero, time.Second},
		{"panicking summarizer", funcSummarizer(func(context.Context, string) (protocol.SummaryResponse, error) { panic("boom") }),
			url.QueryEscape, 200, zero, 0},
		{"missing session_id", nil, func(string) string { return "" }, 400, zero, 0},
		{"unknown session", fixed(day, nil), func(string) string { return "s-nope" }, 404, zero, 0},
	}
	sch := compileSchema(t, "summary.response.schema.json")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarnessConfig(t, Config{Mode: protocol.ModeAdvise, Policy: policy.Heuristic{}, Model: "qwen", Summarizer: tt.summarizer})
			sid := h.hello()
			path := "/v1/summary"
			if q := tt.query(sid); q != "" {
				path += "?session_id=" + q
			}
			start := time.Now()
			code, body := h.get(path)
			if tt.maxLatency > 0 && time.Since(start) > tt.maxLatency {
				t.Fatalf("answered after %v", time.Since(start))
			}
			if code != tt.wantCode {
				t.Fatalf("code %d %s, want %d", code, body, tt.wantCode)
			}
			if code != http.StatusOK {
				return
			}
			validate(t, sch, body)
			var got protocol.SummaryResponse
			dec := json.NewDecoder(strings.NewReader(body))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.SessionID != sid || got.Mode != protocol.ModeAdvise || got.Policy != "heuristic" || got.Model != "qwen" {
				t.Fatalf("header fields %+v", got)
			}
			want := tt.want
			want.SessionID, want.Mode, want.Policy, want.Model = sid, protocol.ModeAdvise, "heuristic", "qwen"
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			if string(a) != string(b) {
				t.Fatalf("got  %s\nwant %s", a, b)
			}
		})
	}
}

func TestDecideNotes(t *testing.T) {
	notes := func(n map[string]string) policy.Policy {
		return funcPolicy(func(context.Context, *protocol.DecideRequest) policy.Decision {
			return policy.Decision{Choice: protocol.Choice{Option: "dismiss"}, Source: protocol.SourceLLM, Notes: n}
		})
	}
	slow := funcPolicy(func(ctx context.Context, _ *protocol.DecideRequest) policy.Decision {
		<-ctx.Done()
		time.Sleep(100 * time.Millisecond)
		return policy.Decision{Choice: protocol.Choice{Option: "dismiss"}, Source: protocol.SourceLLM}
	})
	heuristic := policy.OptionNotes(func() *protocol.DecideRequest {
		var r protocol.DecideRequest
		json.Unmarshal(fixture(t, "decide.request.json"), &r)
		return &r
	}())
	if len(heuristic) == 0 {
		t.Fatal("fixture yields no notes")
	}
	tests := []struct {
		name   string
		mode   protocol.Mode
		policy policy.Policy
		want   map[string]string
	}{
		{"heuristic", protocol.ModeAct, policy.Heuristic{}, heuristic},
		{"unoffered ids dropped", protocol.ModeAct, notes(map[string]string{"dismiss": "free", "defer": "x", "burn_the_shop": "y", "sell_all": " "}),
			map[string]string{"dismiss": "free"}},
		{"long note clipped", protocol.ModeAct, notes(map[string]string{"sell_all": strings.Repeat("ab ", 60)}),
			map[string]string{"sell_all": policy.ClipNote(strings.Repeat("ab ", 60))}},
		{"none", protocol.ModeAct, notes(nil), nil},
		{"deadline fallback keeps notes", protocol.ModeAct, slow, heuristic},
		{"observe has none", protocol.ModeObserve, policy.Heuristic{}, nil},
	}
	sch := compileSchema(t, "decide.response.schema.json")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.mode, tt.policy)
			body := patch(t, withSession(t, fixture(t, "decide.request.json"), h.hello()), "deadline_ms", 400)
			code, resp := h.post("/v1/decide", body)
			if code != 200 {
				t.Fatalf("%d %s", code, resp)
			}
			validate(t, sch, resp)
			got := decodeDecision(t, resp).OptionNotes
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(tt.want)
			if string(a) != string(b) {
				t.Fatalf("notes %s\nwant  %s", a, b)
			}
		})
	}
}

type funcPlanner func(session string) (string, int, bool)

func (f funcPlanner) CurrentPlan(session string) (string, int, bool) { return f(session) }

func TestSummaryPlan(t *testing.T) {
	long := strings.Repeat("Hold the cassettes until the market event ends.\n", 60)
	tests := []struct {
		name     string
		planner  Planner
		wantPlan string
		wantDay  int
	}{
		{"no planner", nil, "", 0},
		{"plan", funcPlanner(func(string) (string, int, bool) { return " Keep 600 for rent.\n", 69, true }), "Keep 600 for rent.", 69},
		{"none yet", funcPlanner(func(string) (string, int, bool) { return "stale", 68, false }), "", 0},
		{"empty", funcPlanner(func(string) (string, int, bool) { return "  ", 69, true }), "", 0},
		{"clipped at a line", funcPlanner(func(string) (string, int, bool) { return long, 70, true }),
			strings.TrimRight(long[:strings.LastIndexByte(long[:protocol.MaxPlanBytes], '\n')], "\n"), 70},
		{"panicking", funcPlanner(func(string) (string, int, bool) { panic("boom") }), "", 0},
		{"slow", funcPlanner(func(string) (string, int, bool) { time.Sleep(2 * time.Second); return "late", 1, true }), "", 0},
	}
	sch := compileSchema(t, "summary.response.schema.json")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarnessConfig(t, Config{Mode: protocol.ModeObserve, Planner: tt.planner})
			sid := h.hello()
			start := time.Now()
			code, body := h.get("/v1/summary?session_id=" + url.QueryEscape(sid))
			if el := time.Since(start); el > time.Second {
				t.Fatalf("answered after %v", el)
			}
			if code != http.StatusOK {
				t.Fatalf("%d %s", code, body)
			}
			validate(t, sch, body)
			var got protocol.SummaryResponse
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if got.Plan != tt.wantPlan || got.PlanDay != tt.wantDay {
				t.Fatalf("plan %q day %d, want %q day %d", got.Plan, got.PlanDay, tt.wantPlan, tt.wantDay)
			}
			if len(got.Plan) > protocol.MaxPlanBytes {
				t.Fatalf("plan is %d bytes", len(got.Plan))
			}
			if tt.wantPlan == "" && strings.Contains(body, `"plan`) {
				t.Fatalf("absent plan must be omitted: %s", body)
			}
		})
	}
}

func TestClipText(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"abc", 5, "abc"},
		{"ab\ncd\nef", 6, "ab\ncd"},
		{"abcdef", 4, "abcd"},
		{"aé", 2, "a"},
	}
	for _, tt := range tests {
		if got := clipText(tt.in, tt.n); got != tt.want {
			t.Errorf("clipText(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}

func TestSlowSummaryKeepsPlan(t *testing.T) {
	slow := funcSummarizer(func(ctx context.Context, _ string) (protocol.SummaryResponse, error) {
		<-ctx.Done()
		return protocol.SummaryResponse{}, ctx.Err()
	})
	plan := funcPlanner(func(string) (string, int, bool) { return "Sell the radios.", 12, true })
	h := newHarnessConfig(t, Config{Mode: protocol.ModeObserve, Summarizer: slow, Planner: plan})
	code, body := h.get("/v1/summary?session_id=" + url.QueryEscape(h.hello()))
	var got protocol.SummaryResponse
	json.Unmarshal([]byte(body), &got)
	if code != 200 || got.Plan != "Sell the radios." || got.PlanDay != 12 {
		t.Fatalf("%d %s", code, body)
	}
}

type directivesPlanner struct {
	funcPlanner
	raw string
	day int
	ok  bool
}

func (p directivesPlanner) CurrentDirectives(string) (json.RawMessage, int, bool) {
	return json.RawMessage(p.raw), p.day, p.ok
}

func TestSummaryDirectives(t *testing.T) {
	plan := funcPlanner(func(string) (string, int, bool) { return "Keep 600 for rent.", 69, true })
	noPlan := funcPlanner(func(string) (string, int, bool) { return "", 0, false })
	tests := []struct {
		name    string
		planner Planner
		want    string // the directives object as served; "" for absent
	}{
		{"planner without directives", plan, ""},
		{"directives", directivesPlanner{plan, `{"day":69,"cash_floor":0,"bargain":"ev_positive","buy":{"hot_stock_cap":0}}`, 69, true},
			`{"day":69,"cash_floor":0,"buy":{"hot_stock_cap":0},"bargain":"ev_positive"}`},
		{"day from the planner", directivesPlanner{plan, `{"augs":"shoot"}`, 69, true}, `{"day":69,"augs":"shoot"}`},
		{"none yet", directivesPlanner{plan, `{"augs":"shoot"}`, 69, false}, ""},
		{"unreadable", directivesPlanner{plan, `[1]`, 69, true}, ""},
		{"no plan, no directives", directivesPlanner{noPlan, `{"augs":"shoot"}`, 69, true}, ""},
	}
	sch := compileSchema(t, "summary.response.schema.json")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarnessConfig(t, Config{Mode: protocol.ModeObserve, Planner: tt.planner})
			code, body := h.get("/v1/summary?session_id=" + url.QueryEscape(h.hello()))
			if code != http.StatusOK {
				t.Fatalf("%d %s", code, body)
			}
			validate(t, sch, body)
			var got struct {
				Directives json.RawMessage `json:"directives"`
			}
			if err := json.Unmarshal([]byte(body), &got); err != nil {
				t.Fatal(err)
			}
			if string(got.Directives) != tt.want {
				t.Fatalf("directives %s, want %s", got.Directives, tt.want)
			}
		})
	}
}
