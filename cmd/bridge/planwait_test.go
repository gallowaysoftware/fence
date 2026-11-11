package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

type fakePlans struct {
	mu  sync.Mutex
	day int
}

func (f *fakePlans) set(day int) {
	f.mu.Lock()
	f.day = day
	f.mu.Unlock()
}

func (f *fakePlans) CurrentPlan(string) (string, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return "plan", f.day, f.day > 0
}

func TestGatePlansPassThrough(t *testing.T) {
	h := policy.Heuristic{}
	if p := gatePlans(h, nil, time.Second); p != policy.Policy(h) {
		t.Fatal("no planner must not gate")
	}
	if p := gatePlans(h, &fakePlans{}, 0); p != policy.Policy(h) {
		t.Fatal("no wait must not gate")
	}
}

func TestPlanGate(t *testing.T) {
	plans := &fakePlans{}
	g := gatePlans(policy.Heuristic{}, plans, 2*time.Second)
	if g.Name() != "heuristic" {
		t.Fatalf("name %q", g.Name())
	}
	decide := func(day int) time.Duration {
		start := time.Now()
		g.Decide(context.Background(), &protocol.DecideRequest{SessionID: "s", GameDay: day, Kind: "nope"})
		return time.Since(start)
	}

	morning := func(day int) {
		g.(*planGate).Event("s", protocol.Event{Kind: "day_snapshot", GameDay: day, Data: json.RawMessage(`{"phase":"morning"}`)})
	}
	if d := decide(2); d < morningGrace || d > morningGrace+300*time.Millisecond {
		t.Fatalf("day 2 without a morning took %v, want the grace only", d)
	}

	morning(3)
	go func() {
		time.Sleep(200 * time.Millisecond)
		plans.set(3)
	}()
	if d := decide(3); d < 150*time.Millisecond || d > time.Second {
		t.Fatalf("first decision of day 3 took %v, want held until the plan (~200ms)", d)
	}
	if d := decide(3); d > 100*time.Millisecond {
		t.Fatalf("second decision of day 3 took %v, want no hold", d)
	}

	// Day 4's plan never comes: one held decision, bounded by the wait, then none.
	morning(4)
	if d := decide(4); d < 2*time.Second {
		t.Fatalf("day 4 returned after %v, want the full wait", d)
	}
	if d := decide(4); d > 100*time.Millisecond {
		t.Fatalf("a failed plan held the day's next decision for %v", d)
	}

	morning(5)
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	start := time.Now()
	g.Decide(ctx, &protocol.DecideRequest{SessionID: "s", GameDay: 5, Kind: "nope"})
	if d := time.Since(start); d > time.Second {
		t.Fatalf("held %v past the decide deadline's margin", d)
	}
}

type fakeDirectives struct{ fakePlans }

func (f *fakeDirectives) CurrentDirectives(string) (json.RawMessage, int, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return json.RawMessage(`{}`), f.day, f.day > 0
}

func TestHoldSummaryForTodaysDirectives(t *testing.T) {
	plans := &fakeDirectives{}
	g := gatePlans(policy.Heuristic{}, plans, 2*time.Second).(*planGate)
	h := g.holdSummary(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	get := func() time.Duration {
		start := time.Now()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/summary?session_id=s", nil))
		return time.Since(start)
	}
	if d := get(); d > summaryGrace+100*time.Millisecond {
		t.Fatalf("no morning yet, held %v", d)
	}
	g.Event("s", protocol.Event{Kind: "day_snapshot", GameDay: 3, Data: json.RawMessage(`{"phase":"morning"}`)})
	go func() {
		time.Sleep(200 * time.Millisecond)
		plans.set(3)
	}()
	if d := get(); d < 150*time.Millisecond || d > time.Second {
		t.Fatalf("morning summary returned after %v, want held until the directives (~200ms)", d)
	}
	if d := get(); d > summaryGrace+100*time.Millisecond {
		t.Fatalf("second summary of the day held %v", d)
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/health", nil))
}

// A plan that failed leaves the model idle: the held decision gives up after idleGrace, not
// the whole wait.
func TestPlanGateGivesUpWhenTheModelIsIdle(t *testing.T) {
	g := gatePlans(policy.Heuristic{}, &fakePlans{}, time.Minute).(*planGate)
	g.modelIdle = func(time.Duration) bool { return true }
	g.Event("s", protocol.Event{Kind: "day_snapshot", GameDay: 4, Data: json.RawMessage(`{"phase":"morning"}`)})
	start := time.Now()
	g.Decide(context.Background(), &protocol.DecideRequest{SessionID: "s", GameDay: 4, Kind: "nope"})
	if d := time.Since(start); d < idleGrace || d > idleGrace+time.Second {
		t.Fatalf("held %v, want about idleGrace (%v)", d, idleGrace)
	}
}

func TestModelWatch(t *testing.T) {
	w := &modelWatch{Client: blockingClient{}}
	if !w.idleFor(0) {
		t.Fatal("a fresh watch is idle")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Complete(ctx, llm.Request{}); close(done) }()
	time.Sleep(50 * time.Millisecond)
	if w.idleFor(0) {
		t.Fatal("idle with a call in flight")
	}
	cancel()
	<-done
	if !w.idleFor(0) || w.idleFor(time.Hour) {
		t.Fatal("idle must count from the last call's end")
	}
}

type blockingClient struct{}

func (blockingClient) Complete(ctx context.Context, _ llm.Request) (llm.Response, error) {
	<-ctx.Done()
	return llm.Response{}, ctx.Err()
}
