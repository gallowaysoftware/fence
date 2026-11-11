package main

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	// planPoll is how often a held decision checks for its day's plan.
	planPoll = 50 * time.Millisecond
	// morningGrace covers recorders seeing the morning snapshot after the first decide.
	morningGrace = time.Second
)

// gatePlans returns p unchanged unless there is a planner and a wait.
func gatePlans(p policy.Policy, plans policy.Planner, wait time.Duration) policy.Policy {
	if plans == nil || wait <= 0 {
		return p
	}
	return &planGate{Policy: p, plans: plans, wait: wait, held: map[string]int{}, summaryHeld: map[string]int{}, morning: map[string]int{}}
}

// planGate holds the first decision of each game day until the strategist has written that
// day's plan, and the morning's first summary until its directives (holdSummary). Only days
// with a morning snapshot get a plan (not the run's first day), and only the first request
// waits, so a failed plan does not stall the day. It records events to see the mornings.
type planGate struct {
	policy.Policy
	plans policy.Planner
	wait  time.Duration

	// modelIdle, when set, reports the strategist's model idle for at least d: a plan still
	// missing then has failed, and waiting longer only stalls the game.
	modelIdle func(d time.Duration) bool

	mu          sync.Mutex
	held        map[string]int // session → the last day a decision was held for
	summaryHeld map[string]int // session → the last morning a summary was held for
	morning     map[string]int // session → the last day with a morning snapshot
}

func (g *planGate) Decide(ctx context.Context, req *protocol.DecideRequest) policy.Decision {
	if g.claim(req.SessionID, req.GameDay) {
		g.await(ctx, req.SessionID, req.GameDay)
	}
	return g.Policy.Decide(ctx, req)
}

func (g *planGate) claim(session string, day int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if d, ok := g.held[session]; ok && d >= day {
		return false
	}
	g.held[session] = day
	return true
}

func (g *planGate) sawMorning(session string, day int) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.morning[session] >= day
}

// await returns once session has a plan for day, the wait is over, no plan is coming, or
// ctx is close enough to its deadline that the policy still needs the rest.
func (g *planGate) await(ctx context.Context, session string, day int) {
	now := time.Now()
	until := now.Add(g.wait)
	if dl, ok := ctx.Deadline(); ok && dl.Add(-time.Second).Before(until) {
		until = dl.Add(-time.Second)
	}
	grace := now.Add(morningGrace)
	t := time.NewTicker(planPoll)
	defer t.Stop()
	for time.Now().Before(until) {
		if _, d, ok := g.plans.CurrentPlan(session); ok && d >= day {
			return
		}
		if !g.sawMorning(session, day) && time.Now().After(grace) || g.gaveUp(now) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (g *planGate) Event(session string, ev protocol.Event) {
	if ev.Kind != "day_snapshot" {
		return
	}
	var snap struct {
		Phase string `json:"phase"`
	}
	if json.Unmarshal(ev.Data, &snap) != nil || snap.Phase != "morning" {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if ev.GameDay > g.morning[session] {
		g.morning[session] = ev.GameDay
	}
}

func (g *planGate) Hello(string, protocol.HelloRequest)                               {}
func (g *planGate) Decision(string, *protocol.DecideRequest, protocol.DecideResponse) {}
func (g *planGate) Outcome(string, protocol.Outcome)                                  {}

// summaryGrace is how long a summary request waits for its morning snapshot to reach the
// recorders; the gym asks for the day's directives right after sending it.
const summaryGrace = 250 * time.Millisecond

// holdSummary holds the first /v1/summary of each morning until the strategist has that
// day's directives (or plan, from a strategist without directives), so an executor that
// reads them in the morning follows today's plan, not yesterday's. Bounded by the wait.
func (g *planGate) holdSummary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/summary" {
			if session := r.URL.Query().Get("session_id"); session != "" {
				g.awaitMorning(r.Context(), session)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (g *planGate) awaitMorning(ctx context.Context, session string) {
	day := 0
	for grace := time.Now().Add(summaryGrace); ; {
		if day = g.claimMorning(session); day > 0 || !time.Now().Before(grace) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
	}
	if day == 0 {
		return
	}
	t := time.NewTicker(planPoll)
	defer t.Stop()
	start := time.Now()
	for until := start.Add(g.wait); time.Now().Before(until); {
		if g.planned(session, day) || g.gaveUp(start) {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// claimMorning returns a morning day of session that no summary has waited for yet, or 0.
func (g *planGate) claimMorning(session string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	d := g.morning[session]
	if d == 0 || g.summaryHeld[session] >= d {
		return 0
	}
	g.summaryHeld[session] = d
	return d
}

func (g *planGate) planned(session string, day int) bool {
	if dp, ok := g.plans.(interface {
		CurrentDirectives(session string) (json.RawMessage, int, bool)
	}); ok {
		_, d, ok := dp.CurrentDirectives(session)
		return ok && d >= day
	}
	_, d, ok := g.plans.CurrentPlan(session)
	return ok && d >= day
}

// idleGrace is how long the model must sit idle, with the plan missing, before a held request
// stops waiting: longer than the gap between a plan and its directives follow-up.
const idleGrace = 2 * time.Second

func (g *planGate) gaveUp(since time.Time) bool {
	return g.modelIdle != nil && time.Since(since) > idleGrace && g.modelIdle(idleGrace)
}

// modelWatch is the strategist's client, counting its calls in flight.
type modelWatch struct {
	llm.Client
	inflight atomic.Int32
	last     atomic.Int64 // unix nanos of the last call's end
}

func (w *modelWatch) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	w.inflight.Add(1)
	defer func() {
		w.last.Store(time.Now().UnixNano())
		w.inflight.Add(-1)
	}()
	return w.Client.Complete(ctx, req)
}

func (w *modelWatch) idleFor(d time.Duration) bool {
	return w.inflight.Load() == 0 && time.Since(time.Unix(0, w.last.Load())) >= d
}
