package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxyCountsAndLimits(t *testing.T) {
	var inFlight, peak atomic.Int32
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"usage":{"prompt_tokens":7,"completion_tokens":3}}`))
	}))
	defer up.Close()
	ep := &Endpoint{BaseURL: up.URL + "/v1", Model: "m"}
	p := newTestProxy(t, NewGameGate(t.TempDir()), map[string]*Endpoint{"e": ep}, map[string]int{"e": 1})
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dir := t.TempDir()
	if err := p.Register("m1", "e", ep, filepath.Join(dir, "llm.jsonl")); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Post(p.BaseURL("m1")+"/chat/completions", "application/json", strings.NewReader(`{}`))
			if err != nil || resp.StatusCode != 200 {
				t.Errorf("post: %v %v", err, resp)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()
	p.Unregister("m1")
	if peak.Load() != 1 {
		t.Fatalf("peak concurrency %d, want 1", peak.Load())
	}
	var r Result
	if err := r.readCalls(filepath.Join(dir, "llm.jsonl")); err != nil {
		t.Fatal(err)
	}
	if r.LLMCalls != 3 || r.PromptTokens != 21 || r.CompletionTokens != 9 || r.LLMErrors != 0 {
		t.Fatalf("counted %+v", r)
	}
	if resp, _ := http.Get(p.BaseURL("nope") + "/models"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown match: %d", resp.StatusCode)
	}
}

func TestProxyHoldsYieldingEndpointWhileGaming(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer up.Close()
	proc := t.TempDir()
	game := filepath.Join(proc, "42")
	os.MkdirAll(game, 0o755)
	os.WriteFile(filepath.Join(game, "cmdline"), []byte(`C:\x\Probably Stolen.exe`+"\x00"), 0o644)
	gate := NewGameGate(proc)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go gate.Watch(ctx, 20*time.Millisecond)

	ep := &Endpoint{BaseURL: up.URL + "/v1", Model: "m", YieldsToGame: true}
	p := newTestProxy(t, gate, map[string]*Endpoint{"e": ep}, map[string]int{"e": 1})
	p.Start()
	defer p.Close()
	p.Register("m1", "e", ep, filepath.Join(t.TempDir(), "llm.jsonl"))
	time.Sleep(50 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		resp, err := http.Post(p.BaseURL("m1")+"/chat/completions", "application/json", strings.NewReader(`{}`))
		if err == nil {
			resp.Body.Close()
		}
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("request went through while the game runs")
	case <-time.After(200 * time.Millisecond):
	}
	os.RemoveAll(game)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("request still held after the game closed")
	}
}

func TestYieldingMatchesAbortForTheGame(t *testing.T) {
	proc := t.TempDir()
	l := &League{Log: slog.New(slog.DiscardHandler), Local: &Local{}, active: map[string]*activeMatch{}, gate: NewGameGate(proc)}
	l.gate.OnChange(l.onGame)
	yctx, ycancel := context.WithCancel(context.Background())
	nctx, ncancel := context.WithCancel(context.Background())
	if !l.track("drift-match", true, ycancel) || !l.track("heavy-match", false, ncancel) {
		t.Fatal("tracking refused while nobody plays")
	}
	l.gate.set(true)
	if yctx.Err() == nil {
		t.Fatal("yielding match still running while the owner plays")
	}
	if nctx.Err() != nil {
		t.Fatal("non-yielding match aborted")
	}
	if !l.untrack("drift-match") || l.untrack("heavy-match") {
		t.Fatal("untrack must report the yield, and only that")
	}
	if l.track("drift-again", true, func() {}) {
		t.Fatal("a yielding match started while the owner plays")
	}
	l.gate.set(false)
	if !l.track("drift-again", true, func() {}) {
		t.Fatal("a yielding match stayed blocked after the game closed")
	}
}

func TestProxyRetries507Once(t *testing.T) {
	defer func(d time.Duration) { retryAfter = d }(retryAfter)
	retryAfter = time.Millisecond
	for _, tt := range []struct {
		name     string
		fails    int32
		want     int
		wantCall int32
	}{{"recovers", 1, 200, 2}, {"gives up after one retry", 5, 507, 2}} {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) <= tt.fails {
					w.WriteHeader(http.StatusInsufficientStorage)
					return
				}
				w.Write([]byte(`{}`))
			}))
			defer up.Close()
			ep := &Endpoint{BaseURL: up.URL + "/v1", Model: "m"}
			p := newTestProxy(t, NewGameGate(t.TempDir()), map[string]*Endpoint{"e": ep}, map[string]int{"e": 1})
			p.Start()
			defer p.Close()
			log := filepath.Join(t.TempDir(), "llm.jsonl")
			p.Register("m1", "e", ep, log)
			resp, err := http.Post(p.BaseURL("m1")+"/chat/completions", "application/json", strings.NewReader(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			p.Unregister("m1")
			var r Result
			r.readCalls(log)
			if resp.StatusCode != tt.want || calls.Load() != tt.wantCall || r.LLMRetries != 1 {
				t.Fatalf("status %d after %d upstream calls, %d retries", resp.StatusCode, calls.Load(), r.LLMRetries)
			}
		})
	}
}

func newTestProxy(t *testing.T, gate *GameGate, eps map[string]*Endpoint, conc map[string]int) *Proxy {
	t.Helper()
	slotRoot = t.TempDir()
	p, err := NewProxy(gate, eps, conc)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSlotsShareACapAcrossHolders(t *testing.T) {
	dir := t.TempDir()
	a, _ := NewSlots(dir, "e", 2)
	b, _ := NewSlots(dir, "e", 2) // another process's view of the same endpoint
	r1, err := a.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r2, _ := b.Acquire(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := a.Acquire(ctx); err == nil {
		t.Fatal("a third request got a slot of a cap of 2")
	}
	r1()
	r3, err := b.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r2()
	r3()
}
