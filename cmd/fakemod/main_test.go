package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/fakemod"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/server"
)

const fixtures = "../../protocol/fixtures"

// startBridge serves the real handler on a random loopback port.
func startBridge(t *testing.T, mode protocol.Mode, p policy.Policy) string {
	t.Helper()
	url, _ := startBridgeLog(t, mode, p, t.TempDir())
	return url
}

// startBridgeLog is startBridge logging to dir; the returned func shuts it down.
func startBridgeLog(t *testing.T, mode protocol.Mode, p policy.Policy, dir string) (string, func()) {
	t.Helper()
	l, err := decisionlog.New(dir, decisionlog.DefaultQueue, nil)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: server.New(server.Config{Mode: mode, Policy: p, Log: l}).Handler()}
	go srv.Serve(ln)
	stop := func() {
		srv.Shutdown(context.Background())
		l.Close()
	}
	t.Cleanup(stop)
	return "http://" + ln.Addr().String(), stop
}

// What fakemod sends is recorded in full: every event logged, no seq gaps.
func TestRecordedWithoutGaps(t *testing.T) {
	dir := t.TempDir()
	base, stop := startBridgeLog(t, protocol.ModeObserve, policy.Heuristic{}, dir)
	if _, err := fakemod.Run(context.Background(), fakemod.Config{BaseURL: base, FixturesDir: fixtures}); err != nil {
		t.Fatal(err)
	}
	stop()
	sessions, err := decisionlog.ReadDir(dir)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions %d, %v", len(sessions), err)
	}
	events := 0
	for _, r := range sessions[0].Records {
		if r.Type == "event" {
			events++
		}
	}
	if g := decisionlog.Gaps(sessions[0], time.Minute); !g.Clean() || events != 4 {
		t.Fatalf("events %d, gaps %+v", events, g)
	}
}

// fakeOpenAI stands in for llama-swap: it answers every completion with text.
func fakeOpenAI(t *testing.T, text string) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestEndToEnd(t *testing.T) {
	tests := []struct {
		name       string
		mode       protocol.Mode
		policy     func(t *testing.T) policy.Policy
		wantOption string
		wantSource protocol.Source
		policyName string
	}{
		{"observe", protocol.ModeObserve, func(*testing.T) policy.Policy { return policy.Heuristic{} }, "defer", protocol.SourceFallback, "heuristic"},
		{"heuristic act", protocol.ModeAct, func(*testing.T) policy.Policy { return policy.Heuristic{} }, "sell_all", protocol.SourceHeuristic, "heuristic"},
		{"llm via openai-compatible", protocol.ModeAdvise, func(t *testing.T) policy.Policy {
			url := fakeOpenAI(t, `<think>budget covers it</think>{"rationale":"good odds","option":"add_markup_10","params":{}}`)
			return &policy.LLM{Client: &llm.OpenAI{BaseURL: url, Model: "qwen"}, Bounds: true}
		}, "add_markup_10", protocol.SourceLLM, "llm"},
		{"llm garbage falls back", protocol.ModeAct, func(t *testing.T) policy.Policy {
			return &policy.LLM{Client: &llm.OpenAI{BaseURL: fakeOpenAI(t, "I dunno"), Model: "qwen"}}
		}, "defer", protocol.SourceFallback, "llm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := startBridge(t, tt.mode, tt.policy(t))
			var out bytes.Buffer
			res, err := fakemod.Run(context.Background(), fakemod.Config{BaseURL: base, FixturesDir: fixtures, Out: &out})
			if err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			if res.Mode != tt.mode || res.Decision.Choice.Option != tt.wantOption || res.Decision.Source != tt.wantSource {
				t.Fatalf("got mode=%s %s/%s, want %s %s/%s\n%s", res.Mode, res.Decision.Choice.Option, res.Decision.Source,
					tt.mode, tt.wantOption, tt.wantSource, out.String())
			}
			if res.Summary.SessionID != res.SessionID || res.Summary.Mode != tt.mode || res.Summary.Policy != tt.policyName {
				t.Errorf("summary %+v", res.Summary)
			}
			if got := strings.Join(res.Events, ","); got != "day_snapshot,shutter_opened,customer_observed,human_action" {
				t.Errorf("events sent %s", got)
			}
			for _, step := range []string{"health:", "hello:", "event:", "decide:", "outcome:", "summary:"} {
				if !strings.Contains(out.String(), step) {
					t.Errorf("output missing %s:\n%s", step, out.String())
				}
			}
		})
	}
}

// A bridge restart between hello and decide forgets the session; the mod re-hellos.
func TestRehelloAfterRestart(t *testing.T) {
	handler := server.New(server.Config{Mode: protocol.ModeAct}).Handler()
	restarted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/decide" && !restarted {
			restarted = true
			handler = server.New(server.Config{Mode: protocol.ModeAct}).Handler()
		}
		handler.ServeHTTP(w, r)
	}))
	defer srv.Close()
	res, err := fakemod.Run(context.Background(), fakemod.Config{BaseURL: srv.URL, FixturesDir: fixtures})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Rehello || res.Decision.Choice.Option != "sell_all" {
		t.Fatalf("got %+v", res)
	}
}

func TestRunCLI(t *testing.T) {
	base := startBridge(t, protocol.ModeAct, policy.Heuristic{})
	var out, errOut bytes.Buffer
	if err := run(context.Background(), []string{"-bridge", base, "-fixtures", fixtures}, &out, &errOut); err != nil {
		t.Fatalf("%v\n%s%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "option=sell_all") {
		t.Fatalf("output:\n%s", out.String())
	}
}

func TestRunCLIBridgeDown(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	var out bytes.Buffer
	err := run(context.Background(), []string{"-bridge", "http://" + addr, "-fixtures", fixtures, "-timeout", "2s"}, &out, &out)
	var opErr *net.OpError
	if err == nil || !errors.As(err, &opErr) {
		t.Fatalf("want a connection error, got %v", err)
	}
}
