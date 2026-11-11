package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/fakemod"
	"github.com/gallowaysoftware/fence/internal/memory"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/server"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func readIndex(t *testing.T, path string) memory.RunIndex {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var x memory.RunIndex
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x
}

func golden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s differs (run with -update after checking):\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestGolden(t *testing.T) {
	for _, name := range []string{"mixed", "empty"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join("testdata", name)
			sessions, err := decisionlog.ReadDir(filepath.Join(dir, "log"))
			if err != nil {
				t.Fatal(err)
			}
			index := readIndex(t, filepath.Join(dir, "index.json"))
			var st bytes.Buffer
			if err := status(&st, sessions, build(sessions, index), 10*time.Minute); err != nil {
				t.Fatal(err)
			}
			golden(t, filepath.Join(dir, "status.txt"), st.Bytes())

			out := t.TempDir()
			var first []byte
			for i := range 2 {
				var msg bytes.Buffer
				if err := export(&msg, out, build(sessions, index)); err != nil {
					t.Fatal(err)
				}
				all := concatDir(t, out)
				if i == 0 {
					first = all
				} else if !bytes.Equal(first, all) {
					t.Fatal("export is not deterministic")
				}
			}
			golden(t, filepath.Join(dir, "export.golden"), first)
		})
	}
}

// concatDir joins every file in dir, sorted, each under a "== name" header.
func concatDir(t *testing.T, dir string) []byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("== " + e.Name() + "\n")
		b.Write(raw)
	}
	return b.Bytes()
}

// Hidden values are in the export only as labels; every other field is what the player saw.
func TestHiddenOnlyInLabels(t *testing.T) {
	sessions, err := decisionlog.ReadDir("testdata/mixed/log")
	if err != nil {
		t.Fatal(err)
	}
	trs := build(sessions, readIndex(t, "testdata/mixed/index.json"))
	labels := 0
	for _, tr := range trs {
		for _, d := range tr.Days {
			labels += len(d.Labels)
			for _, l := range d.Labels {
				if !strings.Contains(string(l.Hidden), "true_value") && !strings.Contains(string(l.Hidden), "identifier") {
					t.Errorf("label without the hidden values: %s", l.Hidden)
				}
			}
			d.Labels = nil
			b, _ := json.Marshal(d)
			for _, needle := range []string{"true_value", "real_features", "days_to_inspection", "dr_jackson", "stealth_wanted", `"hidden"`} {
				if strings.Contains(string(b), needle) {
					t.Errorf("%s day %d: %q outside labels", tr.Name, d.GameDay, needle)
				}
			}
		}
	}
	// Two day_snapshots, customer_observed, decision d-0002.
	if labels != 4 {
		t.Fatalf("%d labels, want 4", labels)
	}
}

func TestLinks(t *testing.T) {
	sessions, err := decisionlog.ReadDir("testdata/mixed/log")
	if err != nil {
		t.Fatal(err)
	}
	trs := build(sessions, readIndex(t, "testdata/mixed/index.json"))
	if trs[0].Name != "run-0001" || len(trs[0].Days) != 2 {
		t.Fatalf("first trajectory %s with %d days", trs[0].Name, len(trs[0].Days))
	}
	d := trs[0].Days[0]
	type row struct {
		what      string
		clientSeq *int64
		want      int64
	}
	rows := []row{
		{"covered customer", d.Customers[1].ClientSeq, 42},
		{"decision", d.Decisions[0].ClientSeq, 42},
		{"outcome via its decision", d.Outcomes[0].ClientSeq, 42},
		{"human action", d.HumanActions[0].ClientSeq, 42},
	}
	for _, r := range rows {
		if r.clientSeq == nil || *r.clientSeq != r.want {
			t.Errorf("%s: client_seq %v, want %d", r.what, r.clientSeq, r.want)
		}
	}
	if m := d.HumanActions[0].Matched; m == nil || *m {
		t.Errorf("add_markup_10 against advised sell_all: matched %v, want false", m)
	}
	if m := d.HumanActions[1].Matched; m != nil {
		t.Errorf("staging is not a choice: matched %v, want null", *m)
	}
}

func TestRunCLI(t *testing.T) {
	out := t.TempDir()
	tests := []struct {
		name    string
		args    []string
		wantErr string
		wantOut string
	}{
		{"no command", nil, "usage", ""},
		{"unknown command", []string{"frobnicate"}, "unknown command", ""},
		{"stray argument", []string{"status", "-log-dir", "testdata/mixed/log", "extra"}, "unexpected", ""},
		{"status", []string{"status", "-log-dir", "testdata/mixed/log", "-memory-db", filepath.Join(out, "none.db")},
			"", "Total: 3 runs, 3 sessions"},
		{"export", []string{"export", "-log-dir", "testdata/mixed/log", "-memory-db", filepath.Join(out, "none.db"), "-out", out},
			"", "session-s-aaaa.jsonl: 2 days"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(context.Background(), tt.args, &stdout, &stderr)
			switch {
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("err %v, want %q", err, tt.wantErr)
			case tt.wantErr == "" && err != nil:
				t.Fatalf("%v\n%s", err, stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.wantOut) {
				t.Fatalf("output %q, want containing %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

// A fakemod session through the real bridge and memory comes out as one indexed run with
// the fixtures' hidden values as labels.
func TestBridgeToExport(t *testing.T) {
	dir := t.TempDir()
	logDir, dbPath := filepath.Join(dir, "log"), filepath.Join(dir, "memory.db")
	dlog, err := decisionlog.New(logDir, decisionlog.DefaultQueue, nil)
	if err != nil {
		t.Fatal(err)
	}
	mem, err := memory.Open(dbPath, memory.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv := server.New(server.Config{Mode: protocol.ModeObserve, Log: dlog, Recorders: []server.Recorder{mem}, Summarizer: mem})
	hs := httptest.NewServer(srv.Handler())
	if _, err := fakemod.Run(context.Background(), fakemod.Config{BaseURL: hs.URL, FixturesDir: "../../protocol/fixtures"}); err != nil {
		t.Fatal(err)
	}
	hs.Close()
	srv.Close()
	mem.Close()
	dlog.Close()

	var stdout, stderr bytes.Buffer
	out := filepath.Join(dir, "export")
	if err := run(context.Background(), []string{"export", "-log-dir", logDir, "-memory-db", dbPath, "-out", out}, &stdout, &stderr); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	b, err := os.ReadFile(filepath.Join(out, "run-0001.jsonl"))
	if err != nil {
		t.Fatalf("%v; export said %s", err, stdout.String())
	}
	var d Day
	if err := json.Unmarshal(bytes.TrimSpace(b), &d); err != nil {
		t.Fatalf("want one day: %v", err)
	}
	got := []int{d.GameDay, len(d.Snapshots), len(d.Customers), len(d.Decisions), len(d.HumanActions), len(d.Outcomes), len(d.Events), len(d.Labels)}
	// Events: the shutter, and the autopilot's action and stop (kept apart from the human's moves).
	want := []int{69, 1, 1, 1, 1, 1, 3, 3}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("day, snapshots, customers, decisions, actions, outcomes, events, labels = %v, want %v", got, want)
	}
	var kinds []string
	for _, e := range d.Events {
		kinds = append(kinds, e.Kind)
	}
	if strings.Join(kinds, ",") != "shutter_opened,autopilot,autopilot_fault" {
		t.Errorf("event kinds %v", kinds)
	}
	if m := d.HumanActions[0].Matched; m != nil {
		t.Errorf("staging matched %v", *m)
	}
	if c := d.Outcomes[0].ClientSeq; c == nil || *c != 41 {
		t.Errorf("outcome client_seq %v, want 41", c)
	}
}
