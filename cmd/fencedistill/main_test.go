package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGame lays out a step-mode game: one day per net worth (the morning's), with notes
// and directives, and final.json ending it.
func writeGame(t *testing.T, dir string, nw []int64, notes []string, dirs []string, ending string) {
	t.Helper()
	raw, err := os.ReadFile("../../protocol/fixtures/event.day_snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	put := func(path string, v any) {
		b, _ := json.Marshal(v)
		if s, ok := v.(string); ok {
			b = []byte(s)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i := range len(nw) - 1 {
		n := i + 1
		d := filepath.Join(dir, fmt.Sprintf("day-%03d", n))
		put(filepath.Join(d, "snapshot.json"), map[string]any{"day": n, "difficulty": "normal", "snapshot": ev.Data})
		put(filepath.Join(d, "directives.json"), dirs[i])
		put(filepath.Join(d, "notes.md"), notes[i])
		put(filepath.Join(d, "result.json"), map[string]any{"day": n, "net_worth_start": nw[i], "net_worth_end": nw[i+1]})
	}
	put(filepath.Join(dir, "final.json"), map[string]any{"day": len(nw), "game_over": ending, "net_worth": nw[len(nw)-1]})
}

func TestBuild(t *testing.T) {
	tmp := t.TempDir()
	game := filepath.Join(tmp, "g1")
	orders := `{"day":1,"bargain":"ev_positive","engine":"wine","buy":{"max_spend":100,"stock_cap":900}}`
	writeGame(t, game,
		[]int64{1000, 1100, 1200, 1300, 1300, 600, 600},
		[]string{"Day 1: open cheap, rent first.", "standing orders: trade", "standing orders: trade", "Day 1: open cheap, rent first.", "standing orders: trade", "standing orders: trade"},
		[]string{orders, orders, orders, orders, orders, orders},
		"rent")
	o := options{games: []string{tmp}, out: filepath.Join(tmp, "out"), horizon: 2, collapse: 2, maxDrop: 0.25,
		decisionMult: 2, repeatMult: 0.5, block: 100, valEvery: 1 << 30, gym: false, rationaleMax: 500}
	st, err := build(o)
	if err != nil {
		t.Fatal(err)
	}
	// Days 4 and 5 fall to 600 within two days; day 6 is within two days of the game over.
	want := map[string]int{"net_worth_fall": 2, "before_collapse": 1}
	for k, v := range want {
		if st.Dropped[k] != v {
			t.Errorf("dropped[%s] = %d, want %d (all: %v)", k, st.Dropped[k], v, st.Dropped)
		}
	}
	rows := readRows(t, filepath.Join(o.out, "train.jsonl"))
	if len(rows) != 3 {
		t.Fatalf("kept %d rows, want 3", len(rows))
	}
	// Day 2 sets the standing orders (a decision); day 3 repeats them.
	if rows[0].Meta.Kind != "decision" || rows[1].Meta.Kind != "decision" || rows[2].Meta.Kind != "routine" || !rows[2].Meta.Repeat {
		t.Errorf("kinds = %+v, %+v, %+v", rows[0].Meta, rows[1].Meta, rows[2].Meta)
	}
	if rows[0].Weight <= rows[2].Weight {
		t.Errorf("decision weight %v not above repeated routine %v", rows[0].Weight, rows[2].Weight)
	}
	ans := rows[0].Messages[2].Content
	if !strings.HasPrefix(ans, "WHY: Day 1: open cheap") || strings.Contains(ans, "engine") ||
		strings.Contains(ans, "stock_cap") || strings.Contains(ans, `"day"`) {
		t.Errorf("contract-only answer = %q", ans)
	}
}

func readRows(t *testing.T, path string) []Example {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []Example
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var e Example
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatal(err)
		}
		out = append(out, e)
	}
	return out
}

func TestJudgedDays(t *testing.T) {
	notes := `# game
- d1: standing orders = defaults. WHY: nothing to decide yet.
- d36-38: buy.max_spend 0. WHY: Georgio takes 50% of cash.
- d43 result: lent 756.
- day 45: JUDGMENT dump cash below rent
- day 1-37: standing orders (base executor). WHY: nothing to decide
- day 85+: standing orders to the end. WHY: done`
	got := judgedDays(notes)
	want := map[int]string{
		36: "Georgio takes 50% of cash.", 37: "Georgio takes 50% of cash.", 38: "Georgio takes 50% of cash.",
		45: "JUDGMENT dump cash below rent",
	}
	if len(got) != len(want) {
		t.Fatalf("judged days = %v, want %v", got, want)
	}
	for d, w := range want {
		if got[d] != w {
			t.Errorf("day %d = %q, want %q", d, got[d], w)
		}
	}
}

func TestPolicyKey(t *testing.T) {
	for _, tc := range []struct{ notes, want string }{
		{"standing orders: trade | max_spend 98", "standing orders: trade"},
		{"Situation: day 10, cash 610.\nDecision: standing orders v1 (wholesale<=95%, ev); floor 304.", "standing orders v1 (wholesale<=95%, ev)"},
		{"Day 37: permit official tomorrow.", "Day 37: permit official tomorrow."},
	} {
		if got := policyKey(tc.notes); got != tc.want {
			t.Errorf("policyKey(%q) = %q, want %q", tc.notes, got, tc.want)
		}
	}
}
