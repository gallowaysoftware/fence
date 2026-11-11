package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func morningSnapshot(t *testing.T, day int) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../../protocol/fixtures/event.day_snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	ev.Data["phase"] = "morning"
	if st, ok := ev.Data["store"].(map[string]any); ok {
		st["day"] = day
	}
	b, _ := json.Marshal(ev.Data)
	return b
}

func writeFile(t *testing.T, path string, v any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	b, ok := v.([]byte)
	if !ok {
		if s, isStr := v.(string); isStr {
			b = []byte(s)
		} else {
			b, _ = json.Marshal(v)
		}
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildDelta(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "base.json"), `{"engine":"wine+furnace","buy":{"single_max_pct":75},"buyout_by_day":140}`)
	executor := map[string]any{"human": "player", "directives": "base.json"}

	// A plan-mode league: the teacher won on day 3, the executor alone on day 6.
	league := filepath.Join(root, "t8")
	writeFile(t, filepath.Join(league, "spec.json"), map[string]any{"executor": executor})
	teacher := filepath.Join(league, "qwen__apprentice__s7__p-x")
	writeFile(t, filepath.Join(teacher, "match.json"), map[string]any{"seed": 7, "executor": executor, "player": map[string]any{"id": "qwen", "mode": "plan"}})
	writeFile(t, filepath.Join(teacher, "result.json"), map[string]any{"ending": "buyout", "days_played": 3})
	writeFile(t, filepath.Join(league, "player__apprentice__s7__p-x", "result.json"), map[string]any{"ending": "buyout_mortgage", "days_played": 6})
	var events, logLines strings.Builder
	for d := 1; d <= 3; d++ {
		ev := map[string]any{"type": "event", "event": map[string]any{"kind": "day_snapshot", "game_day": d, "data": morningSnapshot(t, d)}}
		b, _ := json.Marshal(ev)
		events.Write(append(b, '\n'))
	}
	line := func(day int, js, why string) {
		w := "null"
		if why != "" {
			w = strconv.Quote(why)
		}
		fmt.Fprintf(&logLines, "time=x level=INFO msg=directives day=%d json=%s why=%s\n", day, strconv.Quote(js), w)
	}
	line(1, `{"day":1,"bargain":"never","engine":"wine+furnace"}`, `{"bargain":"Lower Levels rep is low","engine":"restated"}`)
	line(1, `{"day":1,"scavenge":false}`, `{"scavenge":"windfall re-plan"}`)
	line(2, `{"day":2,"bargain":"never","buy":{"single_max_pct":75}}`, `{"*":"same as yesterday"}`)
	line(3, `{"day":3,"buy":{},"scavenge":true}`, `{}`)
	writeFile(t, filepath.Join(teacher, "log", "s-1.jsonl"), events.String())
	writeFile(t, filepath.Join(teacher, "bridge.log"), logLines.String())

	// A hybrid league: a repossessed game, so its mortgage move is dropped.
	hl := filepath.Join(root, "h3")
	hybrid := filepath.Join(hl, "hybrid__apprentice__s9__p-x")
	writeFile(t, filepath.Join(hybrid, "match.json"), map[string]any{"seed": 9, "executor": executor, "player": map[string]any{"id": "hybrid", "mode": "hybrid"}})
	writeFile(t, filepath.Join(hybrid, "result.json"), map[string]any{"ending": "repo", "days_played": 100})
	var rows strings.Builder
	decision := func(day int, inForce, played string, cands ...map[string]any) {
		snap := map[string]any{"day": day, "snapshot": morningSnapshot(t, day)}
		b, _ := json.Marshal(map[string]any{"day": day, "snapshot": snap, "in_force": json.RawMessage(inForce), "played": played, "candidates": cands})
		rows.Write(append(b, '\n'))
	}
	cand := func(name, source, why, d string) map[string]any {
		return map[string]any{"name": name, "source": source, "why": why, "directives": json.RawMessage(d)}
	}
	decision(10, `{}`, "llm_never", cand("keep", "keep", "", `{}`), cand("llm_never", "llm", "rep is sliding", `{"bargain":"never"}`))
	decision(17, `{"bargain":"never"}`, "keep", cand("keep", "keep", "", `{"bargain":"never"}`))
	decision(24, `{"bargain":"never"}`, "llm_take", cand("keep", "keep", "", `{"bargain":"never"}`), cand("llm_take", "llm", "sign now", `{"bargain":"never","mortgage":"take"}`))
	decision(95, `{"bargain":"never"}`, "keep", cand("keep", "keep", "", `{"bargain":"never"}`))
	writeFile(t, filepath.Join(hybrid, "decisions.jsonl"), rows.String())

	out := filepath.Join(root, "out")
	st, err := buildDelta(deltaOptions{hybrid: []string{hl}, league: []string{league}, repo: root, out: out, holdout: []int64{9},
		collapse: 14, switchW: 3, changeW: 2, repeatW: 0.5, emptyW: 0.1, blindW: 0.5, rationaleMax: 300})
	if err != nil {
		t.Fatal(err)
	}
	if st.Dropped["hybrid_mortgage_in_repo_game"] != 1 || st.Dropped["hybrid_before_collapse"] != 1 {
		t.Errorf("dropped = %v", st.Dropped)
	}

	read := func(name string) map[string]Example {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]Example{}
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var e Example
			if err := json.Unmarshal([]byte(l), &e); err != nil {
				t.Fatal(err)
			}
			m[fmt.Sprintf("%s/%d", e.Meta.Source, e.Meta.Day)] = e
		}
		return m
	}
	train, val := read("train.jsonl"), read("val.jsonl")
	for _, tc := range []struct {
		key, kind, answer string
		weight            float64
		inVal             bool
	}{
		// The morning's line wins over the re-plan; the restated base engine goes.
		{"teacher/1", "change", `{"why":{"bargain":"Lower Levels rep is low"},"bargain":"never"}`, 4, false},
		// The restated base buy goes; "*" covers bargain.
		{"teacher/2", "repeat", `{"why":{"bargain":"same as yesterday"},"bargain":"never"}`, 1, false},
		// scavenge has no reason, so it goes; the empty buy goes.
		{"teacher/3", "change", `{}`, 4, false},
		{"hybrid/10", "switch", `{"why":"rep is sliding","bargain":"never"}`, 3, true},
		{"hybrid/17", "keep", `{"why":"rep is sliding","bargain":"never"}`, 0.5, true},
	} {
		set := train
		if tc.inVal {
			set = val
		}
		e, ok := set[tc.key]
		if !ok {
			t.Errorf("%s: missing", tc.key)
			continue
		}
		if e.Meta.Kind != tc.kind || e.Messages[2].Content != tc.answer || e.Weight != tc.weight {
			t.Errorf("%s: kind %s weight %v answer %s; want %s %v %s", tc.key, e.Meta.Kind, e.Weight, e.Messages[2].Content, tc.kind, tc.weight, tc.answer)
		}
		if !strings.Contains(e.Messages[1].Content, `"buyout_by_day":140`) {
			t.Errorf("%s: prompt lacks the base policy", tc.key)
		}
	}
	if y := train["teacher/2"].Messages[1].Content; !strings.Contains(y, `changes: {"bargain":"never"}`) {
		t.Error("teacher/2: prompt lacks yesterday's changes")
	}
	if len(train)+len(val) != 5 {
		t.Errorf("rows = %d train + %d val, want 5", len(train), len(val))
	}
}

func TestSlogValue(t *testing.T) {
	line := `time=x level=INFO msg=directives day=12 json="{\"day\":12,\"permit\":\"buy\"}" why=null`
	for _, tc := range []struct{ key, want string }{
		{"day", "12"},
		{"json", `{"day":12,"permit":"buy"}`},
		{"why", "null"},
		{"missing", ""},
	} {
		if got := slogValue(line, tc.key); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.key, got, tc.want)
		}
	}
}

func TestSameValue(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{`1000`, `1100`, true},
		{`1000`, `1200`, false},
		{`0.8`, `1.0`, false},
		{`["A","B"]`, `["B","A"]`, true},
		{`{"min_pct":57}`, `{"min_pct":57,"hold_uids":[]}`, true},
		{`{"min_pct":57}`, `{"min_pct":80}`, false},
		{`"later"`, `"take"`, false},
	} {
		var a, b any
		_ = json.Unmarshal([]byte(tc.a), &a)
		_ = json.Unmarshal([]byte(tc.b), &b)
		if got := sameValue(a, b); got != tc.want {
			t.Errorf("sameValue(%s, %s) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
