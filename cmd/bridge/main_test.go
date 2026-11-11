package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gallowaysoftware/fence/internal/fakemod"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestConfigErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		env     map[string]string
		wantErr string
	}{
		{"bad mode", []string{"-mode", "autopilot"}, nil, "unknown mode"},
		{"bad mode from env", nil, map[string]string{"FENCE_MODE": "x"}, "unknown mode"},
		{"bad policy", []string{"-policy", "oracle"}, nil, "unknown policy"},
		{"bad provider", []string{"-policy", "llm", "-provider", "bard"}, nil, "unknown provider"},
		{"openai without url", []string{"-policy", "llm", "-model", "q"}, nil, "needs -base-url"},
		{"anthropic without key", []string{"-policy", "llm", "-provider", "anthropic"}, nil, "ANTHROPIC_API_KEY"},
		{"stray args", []string{"extra"}, nil, "unexpected arguments"},
		{"bad thinking", []string{"-policy", "llm", "-base-url", "http://x/v1", "-model", "q", "-thinking", "maybe"}, nil, "-thinking"},
		{"bad temperature", nil, map[string]string{"FENCE_POLICY": "llm", "FENCE_BASE_URL": "http://x/v1", "FENCE_MODEL": "q", "FENCE_TEMPERATURE": "hot"}, "-temperature"},
		{"bad flag", []string{"-nope"}, nil, "not defined"},
		{"unknown persona", []string{"-persona", "zz-no-such-persona"}, nil, "zz-no-such-persona"},
		{"bad persona id from env", nil, map[string]string{"FENCE_PERSONA": "Not An Id"}, "not an id"},
		{"bad planner", []string{"-planner", "oracle"}, nil, "unknown planner"},
		{"llm planner without url", []string{"-planner", "llm", "-model", "q"}, nil, "needs -base-url"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stderr bytes.Buffer
			err := run(context.Background(), append(tt.args, "-listen", "127.0.0.1:0", "-log-dir", t.TempDir()), envMap(tt.env), &stderr, nil)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildPolicy(t *testing.T) {
	tests := []struct {
		name string
		args []string
		env  map[string]string
		want string
		// wantModel is what /v1/summary reports.
		wantModel string
	}{
		{"default heuristic", nil, nil, "heuristic", ""},
		{"heuristic ignores -model", []string{"-model", "q"}, nil, "heuristic", ""},
		{"openai", []string{"-policy", "llm", "-base-url", "http://x/v1", "-model", "q"}, nil, "llm", "q"},
		{"anthropic", []string{"-policy", "llm", "-provider", "anthropic"}, map[string]string{"ANTHROPIC_API_KEY": "k"}, "llm", llm.AnthropicDefaultModel},
		{"from env", nil, map[string]string{"FENCE_POLICY": "llm", "FENCE_PROVIDER": "anthropic", "FENCE_MODEL": "m", "ANTHROPIC_API_KEY": "k"}, "llm", "m"},
		{"heuristic counter, llm planner", []string{"-planner", "llm", "-base-url", "http://x/v1", "-model", "q"}, nil, "heuristic", ""},
		{"llm planner from env", nil, map[string]string{"FENCE_PLANNER": "llm", "FENCE_BASE_URL": "http://x/v1", "FENCE_MODEL": "q"}, "heuristic", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseFlags(tt.args, envMap(tt.env), &bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			client, bounds, err := buildClient(c, envMap(tt.env))
			if err != nil {
				t.Fatal(err)
			}
			if p := buildPolicy(c, client, bounds, nil, nil); p.Name() != tt.want {
				t.Fatalf("policy %s, want %s", p.Name(), tt.want)
			}
			if m := modelName(c, client); m != tt.wantModel {
				t.Fatalf("model %q, want %q", m, tt.wantModel)
			}
		})
	}
}

func TestDefaultsBindLoopback(t *testing.T) {
	c, err := parseFlags(nil, envMap(nil), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if c.listen != "127.0.0.1:7341" || c.mode != "observe" || c.logDir != "var/log" ||
		!c.memory || c.memoryDB != "var/memory.db" || !c.carryLessons || !c.reflect || c.planner != "" || c.plannerWait != 0 {
		t.Fatalf("defaults %+v", c)
	}
}

func TestServeAndShutdown(t *testing.T) {
	tests := []struct {
		name       string
		memory     func(dir string) []string
		wantMemory bool
	}{
		{"memory on", func(dir string) []string { return []string{"-memory-db", filepath.Join(dir, "m", "memory.db")} }, true},
		{"memory off", func(dir string) []string {
			return []string{"-memory=false", "-memory-db", filepath.Join(dir, "m", "memory.db")}
		}, false},
		// A database that cannot be opened costs memory, not the bridge.
		{"memory unavailable", func(dir string) []string { return []string{"-memory-db", dir} }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			logDir := filepath.Join(dir, "log")
			ctx, cancel := context.WithCancel(context.Background())
			ready := make(chan string, 1)
			done := make(chan error, 1)
			var stderr bytes.Buffer
			args := append([]string{"-listen", "127.0.0.1:0", "-mode", "act", "-log-dir", logDir}, tt.memory(dir)...)
			go func() {
				done <- run(ctx, args, envMap(nil), &stderr, ready)
			}()
			var addr string
			select {
			case addr = <-ready:
			case err := <-done:
				t.Fatalf("exited early: %v\n%s", err, stderr.String())
			case <-time.After(5 * time.Second):
				t.Fatal("bridge did not start")
			}
			res, err := fakemod.Run(context.Background(), fakemod.Config{BaseURL: "http://" + addr, FixturesDir: "../../protocol/fixtures"})
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("shutdown: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("did not shut down")
			}
			if _, err := http.Get("http://" + addr + "/v1/health"); err == nil {
				t.Fatal("still serving after shutdown")
			}
			b, err := os.ReadFile(filepath.Join(logDir, res.SessionID+".jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if sum := res.Summary; sum.SessionID != res.SessionID || sum.Policy != "heuristic" || sum.Model != "" || (sum.Day == 69) != tt.wantMemory {
				t.Errorf("summary %+v (memory %v)", sum, tt.wantMemory)
			}
			for _, typ := range []string{`"type":"hello"`, `"type":"event"`, `"type":"decision"`, `"type":"outcome"`} {
				if !bytes.Contains(b, []byte(typ)) {
					t.Errorf("log missing %s after shutdown flush", typ)
				}
			}
			_, statErr := os.Stat(filepath.Join(dir, "m", "memory.db"))
			if gotMemory := statErr == nil; gotMemory != tt.wantMemory {
				t.Fatalf("memory db exists %v, want %v\n%s", gotMemory, tt.wantMemory, stderr.String())
			}
			if tt.wantMemory {
				db, err := sql.Open("sqlite", filepath.Join(dir, "m", "memory.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var n int
				if err := db.QueryRow(`SELECT count(*) FROM decisions WHERE session_id = ?`, res.SessionID).Scan(&n); err != nil || n != 1 {
					t.Fatalf("memory decisions %d, %v", n, err)
				}
			}
		})
	}
}

// The mod cannot read files, so hello hands it the appraisal guides when the bridge has them.
func TestHelloCarriesAppraisalGuides(t *testing.T) {
	const fixture = "../../internal/policy/testdata/appraisal_guides.fixture.json"
	for _, tt := range []struct {
		name, path, logLine string
		want                bool
	}{
		{"configured", fixture, "appraisal guides loaded", true},
		{"absent", filepath.Join(t.TempDir(), "none.json"), "no appraisal guides", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ready, done := make(chan string, 1), make(chan error, 1)
			var stderr bytes.Buffer
			env := envMap(map[string]string{"FENCE_APPRAISAL_GUIDES": tt.path})
			go func() {
				done <- run(ctx, []string{"-listen", "127.0.0.1:0", "-memory=false", "-log-dir", t.TempDir()}, env, &stderr, ready)
			}()
			var addr string
			select {
			case addr = <-ready:
			case err := <-done:
				t.Fatalf("exited early: %v\n%s", err, stderr.String())
			case <-time.After(5 * time.Second):
				t.Fatal("bridge did not start")
			}
			hello, err := os.ReadFile("../../protocol/fixtures/hello.request.json")
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.Post("http://"+addr+"/v1/hello", "application/json", bytes.NewReader(hello))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			var got protocol.HelloResponse
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if has := got.AppraisalGuides != nil; has != tt.want {
				t.Fatalf("hello has appraisal_guides %v, want %v", has, tt.want)
			}
			if tt.want {
				want, _ := os.ReadFile(fixture)
				var a, b any
				json.Unmarshal(got.AppraisalGuides, &a)
				json.Unmarshal(want, &b)
				if !reflect.DeepEqual(a, b) {
					t.Fatal("hello's appraisal_guides differ from the file")
				}
			}
			if !strings.Contains(stderr.String(), tt.logLine) {
				t.Fatalf("startup log lacks %q:\n%s", tt.logLine, stderr.String())
			}
		})
	}
}
