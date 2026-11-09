package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

type fakeRecorder struct {
	mu        sync.Mutex
	calls     []string
	decisions []*protocol.DecideRequest
	events    []protocol.Event
	panics    bool
	// block, if set, stalls every call until it is closed.
	block chan struct{}
}

func (f *fakeRecorder) add(s string) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
	if f.panics {
		panic("recorder bug")
	}
}

func (f *fakeRecorder) Hello(session string, _ protocol.HelloRequest) { f.add("hello") }
func (f *fakeRecorder) Event(session string, ev protocol.Event) {
	f.mu.Lock()
	f.events = append(f.events, ev)
	f.mu.Unlock()
	f.add("event:" + ev.Kind)
}
func (f *fakeRecorder) Outcome(session string, oc protocol.Outcome) {
	f.add("outcome:" + oc.DecisionID)
}
func (f *fakeRecorder) Decision(session string, req *protocol.DecideRequest, resp protocol.DecideResponse) {
	f.mu.Lock()
	f.decisions = append(f.decisions, req)
	f.mu.Unlock()
	f.add("decision:" + resp.Choice.Option)
}

func (f *fakeRecorder) joined() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}

func dismissPolicy() policy.Policy {
	return funcPolicy(func(context.Context, *protocol.DecideRequest) policy.Decision {
		return policy.Decision{Choice: protocol.Choice{Option: "dismiss"}, Source: protocol.SourceLLM}
	})
}

// playFixtures sends every recording fixture plus a decision and an outcome.
func playFixtures(t *testing.T, h *harness) string {
	t.Helper()
	sid := h.hello()
	for _, s := range []struct{ path, fixture string }{
		{"/v1/event", "event.day_snapshot.json"},
		{"/v1/event", "event.request.json"},
		{"/v1/event", "event.customer_observed.json"},
		{"/v1/decide", "decide.request.json"},
		{"/v1/event", "event.human_action.json"},
		{"/v1/outcome", "outcome.request.json"},
	} {
		if code, body := h.post(s.path, withSession(t, fixture(t, s.fixture), sid)); code/100 != 2 {
			t.Fatalf("%s %s: %d %s", s.path, s.fixture, code, body)
		}
	}
	return sid
}

const wantCalls = "hello,event:day_snapshot,event:shutter_opened,event:customer_observed,decision:dismiss,event:human_action,outcome:d-0001"

func TestRecordersSeeEverythingButHidden(t *testing.T) {
	ok, panicking := &fakeRecorder{}, &fakeRecorder{panics: true}
	h := newHarnessConfig(t, Config{Mode: protocol.ModeAct, Policy: dismissPolicy(), Recorders: []Recorder{panicking, nil, ok}})
	playFixtures(t, h)
	h.server.Close()
	for name, rec := range map[string]*fakeRecorder{"ok": ok, "panicking": panicking} {
		if got := rec.joined(); got != wantCalls {
			t.Errorf("%s: calls %s, want %s", name, got, wantCalls)
		}
		if len(rec.decisions) != 1 || len(rec.decisions[0].Hidden) != 0 || len(rec.decisions[0].Context) == 0 {
			t.Errorf("%s: decision must carry context and no hidden: %+v", name, rec.decisions)
		}
		for _, ev := range rec.events {
			if len(ev.Hidden) != 0 || len(ev.Data) == 0 {
				t.Errorf("%s: event %s must carry data and no hidden: %+v", name, ev.Kind, ev)
			}
		}
	}
	// The decision log still has the hidden values the recorders never saw.
	hidden := 0
	for _, r := range h.records(h.lastSession) {
		if r["type"] != "event" {
			continue
		}
		if _, ok := r["event"].(map[string]any)["hidden"]; ok {
			hidden++
		}
	}
	if hidden != 2 {
		t.Fatalf("decision log kept hidden on %d events, want 2 (day_snapshot, customer_observed)", hidden)
	}
}

func TestSlowRecorderIsolated(t *testing.T) {
	block := make(chan struct{})
	slow, fast := &fakeRecorder{block: block}, &fakeRecorder{}
	h := newHarnessConfig(t, Config{Mode: protocol.ModeAct, Policy: dismissPolicy(), Recorders: []Recorder{slow, fast}, RecorderQueue: 2})
	start := time.Now()
	playFixtures(t, h)
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("requests waited on the slow recorder: %v", el)
	}
	deadline := time.Now().Add(2 * time.Second)
	for fast.joined() != wantCalls {
		if time.Now().After(deadline) {
			t.Fatalf("fast recorder got %s, want %s", fast.joined(), wantCalls)
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(block)
	h.server.Close()
	// Queue 2 plus the call in progress; the rest were dropped for the slow one only.
	if n := len(strings.Split(slow.joined(), ",")); n > 3 {
		t.Errorf("slow recorder got %d records, queue should have bounded it", n)
	}
	if h.server.RecordsDropped() == 0 {
		t.Error("drops not counted")
	}
}

func TestNoRecorders(t *testing.T) {
	srv := httptest.NewServer(New(Config{Mode: protocol.ModeAct}).Handler())
	defer srv.Close()
	h := &harness{t: t, srv: srv}
	sid := h.hello()
	if code, _ := h.post("/v1/decide", withSession(t, fixture(t, "decide.request.json"), sid)); code != http.StatusOK {
		t.Fatalf("decide %d", code)
	}
}
