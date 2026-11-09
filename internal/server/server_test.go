package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const fixtures = "../../protocol/fixtures"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// withSession rewrites a fixture's session_id; fixtures carry a placeholder.
func withSession(t *testing.T, raw []byte, session string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	m["session_id"] = session
	b, _ := json.Marshal(m)
	return string(b)
}

func patch(t *testing.T, raw string, key string, v any) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if v == nil {
		delete(m, key)
	} else {
		m[key] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

type funcPolicy func(ctx context.Context, req *protocol.DecideRequest) policy.Decision

func (funcPolicy) Name() string { return "test" }
func (f funcPolicy) Decide(ctx context.Context, req *protocol.DecideRequest) policy.Decision {
	return f(ctx, req)
}

type harness struct {
	t           *testing.T
	srv         *httptest.Server
	server      *Server
	log         *decisionlog.Logger
	logDir      string
	lastSession string
}

func newHarness(t *testing.T, mode protocol.Mode, p policy.Policy) *harness {
	t.Helper()
	return newHarnessConfig(t, Config{Mode: mode, Policy: p})
}

// newHarnessConfig serves cfg with a fresh decision log in place of cfg.Log.
func newHarnessConfig(t *testing.T, cfg Config) *harness {
	t.Helper()
	dir := t.TempDir()
	l, err := decisionlog.New(dir, decisionlog.DefaultQueue, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Log = l
	s := New(cfg)
	h := &harness{t: t, srv: httptest.NewServer(s.Handler()), server: s, log: l, logDir: dir}
	t.Cleanup(func() { h.srv.Close(); s.Close(); l.Close() })
	return h
}

func (h *harness) post(path, body string) (int, string) {
	h.t.Helper()
	resp, err := http.Post(h.srv.URL+path, "application/json", strings.NewReader(body))
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (h *harness) get(path string) (int, string) {
	h.t.Helper()
	resp, err := http.Get(h.srv.URL + path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (h *harness) hello() string {
	h.t.Helper()
	code, body := h.post("/v1/hello", string(fixture(h.t, "hello.request.json")))
	if code != http.StatusOK {
		h.t.Fatalf("hello: %d %s", code, body)
	}
	var resp protocol.HelloResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		h.t.Fatal(err)
	}
	h.lastSession = resp.SessionID
	return resp.SessionID
}

// records closes the logger and returns the session's records.
func (h *harness) records(session string) []map[string]any {
	h.t.Helper()
	h.log.Close()
	f, err := os.Open(h.log.Path(session))
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			h.t.Fatalf("bad log line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

func TestHealth(t *testing.T) {
	h := newHarness(t, protocol.ModeObserve, nil)
	resp, err := http.Get(h.srv.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || strings.TrimSpace(string(b)) != `{"ok":true,"protocol":1}` {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
}

func TestHello(t *testing.T) {
	good := string(fixture(t, "hello.request.json"))
	tests := []struct {
		name     string
		mode     protocol.Mode
		body     string
		wantCode int
		wantBody string
	}{
		{"observe", protocol.ModeObserve, good, 200, `"mode":"observe"`},
		{"act", protocol.ModeAct, good, 200, `"mode":"act"`},
		{"protocol mismatch", protocol.ModeAct, patch(t, good, "protocol", 2), 409, `"protocol":1`},
		{"malformed", protocol.ModeAct, `{"protocol":`, 400, "error"},
		{"empty", protocol.ModeAct, ``, 400, "empty body"},
		{"trailing garbage", protocol.ModeAct, good + `{}`, 400, "trailing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.mode, nil)
			code, body := h.post("/v1/hello", tt.body)
			if code != tt.wantCode || !strings.Contains(body, tt.wantBody) {
				t.Fatalf("got %d %s, want %d containing %s", code, body, tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestHelloSessionsAreDistinct(t *testing.T) {
	h := newHarness(t, protocol.ModeObserve, nil)
	if a, b := h.hello(), h.hello(); a == b || a == "" {
		t.Fatalf("session ids %q %q", a, b)
	}
}

func TestEventSeq(t *testing.T) {
	h := newHarness(t, protocol.ModeObserve, nil)
	sid := h.hello()
	ev := withSession(t, fixture(t, "event.request.json"), sid)
	for _, seq := range []int{1, 2, 5, 4, 6} {
		if code, body := h.post("/v1/event", patch(t, ev, "seq", seq)); code != http.StatusNoContent {
			t.Fatalf("seq %d: %d %s", seq, code, body)
		}
	}
	type seen struct {
		missed     float64
		outOfOrder bool
	}
	var got []seen
	for _, r := range h.records(sid) {
		if r["type"] == "event" {
			m, _ := r["missed"].(float64)
			o, _ := r["out_of_order"].(bool)
			got = append(got, seen{m, o})
		}
	}
	want := []seen{{0, false}, {0, false}, {2, false}, {0, true}, {0, false}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestObserveSeq(t *testing.T) {
	tests := []struct {
		name       string
		seqs       []int64
		wantMissed int64
		wantOOO    bool
	}{
		{"starts at 0", []int64{0}, 0, false},
		{"starts at 1", []int64{1}, 0, false},
		{"starts late", []int64{17}, 16, false},
		{"consecutive", []int64{3, 4}, 0, false},
		{"gap", []int64{3, 7}, 3, false},
		{"duplicate", []int64{3, 3}, 0, true},
		{"backwards", []int64{3, 2}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &session{}
			var missed int64
			var ooo bool
			for _, q := range tt.seqs {
				missed, ooo = s.observeSeq(q)
			}
			if missed != tt.wantMissed || ooo != tt.wantOOO {
				t.Fatalf("missed=%d ooo=%v, want %d %v", missed, ooo, tt.wantMissed, tt.wantOOO)
			}
		})
	}
}

func TestEventAndOutcomeErrors(t *testing.T) {
	h := newHarness(t, protocol.ModeObserve, nil)
	sid := h.hello()
	ev := withSession(t, fixture(t, "event.request.json"), sid)
	oc := withSession(t, fixture(t, "outcome.request.json"), sid)
	tests := []struct {
		name, path, body string
		want             int
	}{
		{"event ok", "/v1/event", ev, 204},
		{"event unknown session", "/v1/event", patch(t, ev, "session_id", "s-gone"), 404},
		{"event malformed", "/v1/event", `{`, 400},
		{"event missing kind", "/v1/event", patch(t, ev, "kind", nil), 400},
		{"outcome ok", "/v1/outcome", oc, 204},
		{"outcome unknown session", "/v1/outcome", patch(t, oc, "session_id", "s-gone"), 404},
		{"outcome missing decision", "/v1/outcome", patch(t, oc, "decision_id", nil), 400},
		{"wrong method", "/v1/health", ``, 405},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if code, body := h.post(tt.path, tt.body); code != tt.want {
				t.Fatalf("got %d %s, want %d", code, body, tt.want)
			}
		})
	}
}

func decodeDecision(t *testing.T, body string) protocol.DecideResponse {
	t.Helper()
	var r protocol.DecideResponse
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
	return r
}

func TestDecide(t *testing.T) {
	slowIgnoresCtx := funcPolicy(func(context.Context, *protocol.DecideRequest) policy.Decision {
		time.Sleep(2 * time.Second)
		return policy.Decision{Choice: protocol.Choice{Option: "sell_all"}, Source: protocol.SourceLLM}
	})
	panics := funcPolicy(func(context.Context, *protocol.DecideRequest) policy.Decision { panic("boom") })
	invents := funcPolicy(func(context.Context, *protocol.DecideRequest) policy.Decision {
		return policy.Decision{Choice: protocol.Choice{Option: "burn_the_shop"}, Source: protocol.SourceLLM}
	})
	req := string(fixture(t, "decide.request.json"))
	tests := []struct {
		name       string
		mode       protocol.Mode
		policy     policy.Policy
		body       func(sid string) string
		wantCode   int
		wantOption string
		wantSource protocol.Source
		maxLatency time.Duration
	}{
		{"heuristic act", protocol.ModeAct, policy.Heuristic{}, func(s string) string { return withSession(t, []byte(req), s) }, 200, "sell_all", protocol.SourceHeuristic, 0},
		{"advise also decides", protocol.ModeAdvise, policy.Heuristic{}, func(s string) string { return withSession(t, []byte(req), s) }, 200, "sell_all", protocol.SourceHeuristic, 0},
		{"observe defers", protocol.ModeObserve, policy.Heuristic{}, func(s string) string { return withSession(t, []byte(req), s) }, 200, "defer", protocol.SourceFallback, 0},
		{"unknown kind defers", protocol.ModeAct, policy.Heuristic{}, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "kind", "loan_shark")
		}, 200, "defer", protocol.SourceHeuristic, 0},
		{"policy ignoring deadline", protocol.ModeAct, slowIgnoresCtx, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "deadline_ms", 300)
		}, 200, "defer", protocol.SourceFallback, 300 * time.Millisecond},
		{"tiny deadline", protocol.ModeAct, slowIgnoresCtx, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "deadline_ms", 1)
		}, 200, "defer", protocol.SourceFallback, 100 * time.Millisecond},
		{"policy panics", protocol.ModeAct, panics, func(s string) string { return withSession(t, []byte(req), s) }, 200, "defer", protocol.SourceFallback, 0},
		{"policy invents an option", protocol.ModeAct, invents, func(s string) string { return withSession(t, []byte(req), s) }, 200, "defer", protocol.SourceFallback, 0},
		{"unknown session", protocol.ModeAct, policy.Heuristic{}, func(string) string { return req }, 404, "", "", 0},
		{"malformed", protocol.ModeAct, policy.Heuristic{}, func(string) string { return `{"session_id":` }, 400, "", "", 0},
		{"no options", protocol.ModeAct, policy.Heuristic{}, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "options", []any{})
		}, 400, "", "", 0},
		{"no deadline", protocol.ModeAct, policy.Heuristic{}, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "deadline_ms", nil)
		}, 400, "", "", 0},
		{"huge body", protocol.ModeAct, policy.Heuristic{}, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "context", map[string]any{"blob": strings.Repeat("x", DefaultMaxBodyBytes)})
		}, 413, "", "", 0},
		{"unknown fields tolerated", protocol.ModeAct, policy.Heuristic{}, func(s string) string {
			return patch(t, withSession(t, []byte(req), s), "priority", "high")
		}, 200, "sell_all", protocol.SourceHeuristic, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.mode, tt.policy)
			sid := h.hello()
			start := time.Now()
			code, body := h.post("/v1/decide", tt.body(sid))
			elapsed := time.Since(start)
			if code != tt.wantCode {
				t.Fatalf("code %d %s, want %d", code, body, tt.wantCode)
			}
			if tt.maxLatency > 0 && elapsed > tt.maxLatency {
				t.Fatalf("answered after %v, deadline %v", elapsed, tt.maxLatency)
			}
			if code != 200 {
				return
			}
			r := decodeDecision(t, body)
			if r.DecisionID != "d-0001" || r.Choice.Option != tt.wantOption || r.Source != tt.wantSource {
				t.Fatalf("got %+v, want %s/%s", r, tt.wantOption, tt.wantSource)
			}
		})
	}
}

func TestHiddenReachesLogNotPolicy(t *testing.T) {
	var sawHidden []string
	var mu sync.Mutex
	p := funcPolicy(func(_ context.Context, req *protocol.DecideRequest) policy.Decision {
		mu.Lock()
		sawHidden = append(sawHidden, string(req.Hidden))
		mu.Unlock()
		return policy.Decision{Choice: protocol.Choice{Option: "dismiss"}, Source: protocol.SourceLLM}
	})
	h := newHarness(t, protocol.ModeAct, p)
	sid := h.hello()
	if code, body := h.post("/v1/decide", withSession(t, fixture(t, "decide.request.json"), sid)); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if len(sawHidden) != 1 || sawHidden[0] != "" {
		t.Fatalf("policy saw hidden: %q", sawHidden)
	}
	var want map[string]any
	json.Unmarshal(fixture(t, "decide.request.json"), &want)
	for _, r := range h.records(sid) {
		if r["type"] != "decision" {
			continue
		}
		req := r["request"].(map[string]any)
		if fmt.Sprint(req["hidden"]) != fmt.Sprint(want["hidden"]) {
			t.Fatalf("decision record hidden = %v, want %v", req["hidden"], want["hidden"])
		}
		if req["state_signature"] != want["state_signature"] {
			t.Fatalf("decision record state_signature = %v", req["state_signature"])
		}
		return
	}
	t.Fatal("no decision record")
}

func TestConcurrentDecides(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	p := funcPolicy(func(ctx context.Context, req *protocol.DecideRequest) policy.Decision {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		defer func() { mu.Lock(); inFlight--; mu.Unlock() }()
		select {
		case <-time.After(100 * time.Millisecond):
		case <-ctx.Done():
		}
		return policy.Decision{Choice: protocol.Choice{Option: "dismiss"}, Rationale: req.DecisionID, Source: protocol.SourceLLM}
	})
	h := newHarness(t, protocol.ModeAct, p)
	sid := h.hello()
	base := withSession(t, fixture(t, "decide.request.json"), sid)
	const n = 40
	var wg sync.WaitGroup
	errs := make(chan error, n)
	start := time.Now()
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := fmt.Sprintf("d-%03d", i)
			resp, err := http.Post(h.srv.URL+"/v1/decide", "application/json", strings.NewReader(patch(t, base, "decision_id", id)))
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			var r protocol.DecideResponse
			if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
				errs <- err
				return
			}
			if r.DecisionID != id || r.Rationale != id || r.Choice.Option != "dismiss" {
				errs <- fmt.Errorf("crossed answers: sent %s, got %+v", id, r)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("decides were serialized: %v", elapsed)
	}
	if peak < 2 {
		t.Errorf("peak concurrency %d", peak)
	}
	decisions := 0
	for _, r := range h.records(sid) {
		if r["type"] == "decision" {
			decisions++
		}
	}
	if decisions != n {
		t.Errorf("logged %d decisions, want %d", decisions, n)
	}
}

func TestDecisionLogJoinsOutcome(t *testing.T) {
	h := newHarness(t, protocol.ModeAct, policy.Heuristic{})
	sid := h.hello()
	if code, body := h.post("/v1/decide", withSession(t, fixture(t, "decide.request.json"), sid)); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	oc := withSession(t, fixture(t, "outcome.request.json"), sid)
	h.post("/v1/outcome", oc)
	h.post("/v1/outcome", patch(t, oc, "decision_id", "d-never"))
	recs := h.records(sid)
	var types []string
	for _, r := range recs {
		types = append(types, r["type"].(string))
	}
	if got := strings.Join(types, ","); got != "hello,decision,outcome,outcome" {
		t.Fatalf("record types %s", got)
	}
	dec := recs[1]
	for _, k := range []string{"request", "trace", "response", "latency_ms", "decision_id", "ts", "session_id", "policy", "mode"} {
		if _, ok := dec[k]; !ok {
			t.Errorf("decision record missing %s: %v", k, dec)
		}
	}
	if recs[2]["decision_id"] != "d-0001" || recs[2]["known_decision"] != true {
		t.Errorf("outcome not joined: %v", recs[2])
	}
	if recs[3]["known_decision"] != false {
		t.Errorf("stray outcome not flagged: %v", recs[3])
	}
}

func TestBridgeRestartForgetsSessions(t *testing.T) {
	a := newHarness(t, protocol.ModeAct, policy.Heuristic{})
	sid := a.hello()
	b := newHarness(t, protocol.ModeAct, policy.Heuristic{})
	code, body := b.post("/v1/decide", withSession(t, fixture(t, "decide.request.json"), sid))
	if code != http.StatusNotFound || !strings.Contains(body, "hello") {
		t.Fatalf("got %d %s; the mod needs a 404 telling it to re-hello", code, body)
	}
	newSid := b.hello()
	if code, body := b.post("/v1/decide", withSession(t, fixture(t, "decide.request.json"), newSid)); code != 200 {
		t.Fatalf("after re-hello: %d %s", code, body)
	}
}

func TestLogFailureNeverFailsRequests(t *testing.T) {
	h := newHarness(t, protocol.ModeAct, policy.Heuristic{})
	// Replace the log dir with a file so every open fails.
	if err := os.RemoveAll(h.logDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.logDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	sid := h.hello()
	if code, body := h.post("/v1/decide", withSession(t, fixture(t, "decide.request.json"), sid)); code != 200 {
		t.Fatalf("%d %s", code, body)
	}
	if code, _ := h.post("/v1/outcome", withSession(t, fixture(t, "outcome.request.json"), sid)); code != 204 {
		t.Fatalf("outcome %d", code)
	}
	h.log.Close()
	if h.log.Dropped() == 0 {
		t.Fatal("failed writes were not counted")
	}
}

func TestMargin(t *testing.T) {
	tests := []struct {
		deadlineMS int64
		want       time.Duration
	}{
		{1, 100 * time.Millisecond},
		{500, 100 * time.Millisecond},
		{2000, 200 * time.Millisecond},
		{20000, time.Second},
		{600000, time.Second},
	}
	for _, tt := range tests {
		if got := margin(tt.deadlineMS); got != tt.want {
			t.Errorf("margin(%d) = %v, want %v", tt.deadlineMS, got, tt.want)
		}
	}
}
