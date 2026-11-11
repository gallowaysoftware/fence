package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecValidation(t *testing.T) {
	tests := []struct {
		name, json, wantErr string
	}{
		{"ok", `{"name":"t","seeds":[1],"players":[{"id":"h","mode":"heuristic"}]}`, ""},
		{"bad name", `{"name":"T 1","seeds":[1],"players":[{"id":"h","mode":"heuristic"}]}`, "name"},
		{"no seeds", `{"name":"t","players":[{"id":"h","mode":"heuristic"}]}`, "needs seeds"},
		{"bad mode", `{"name":"t","seeds":[1],"players":[{"id":"h","mode":"oracle"}]}`, "mode"},
		{"llm without endpoint", `{"name":"t","seeds":[1],"players":[{"id":"p","mode":"plan"}]}`, "needs an endpoint"},
		{"duplicate player", `{"name":"t","seeds":[1],"players":[{"id":"h","mode":"heuristic"},{"id":"h","mode":"local"}]}`, "duplicate"},
		{"unknown field", `{"name":"t","seeds":[1],"players":[],"nope":1}`, "unknown field"},
		{"bad baseline", `{"name":"t","seeds":[1],"players":[{"id":"h","mode":"heuristic"}],"baseline":"x"}`, "baseline"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "s.json")
			os.WriteFile(p, []byte(tt.json), 0o644)
			s, err := LoadSpec(p)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
				if s.Difficulty != "hard" || s.Days != 200 || s.Starts[0] != "none" || s.Baseline != "h" {
					t.Fatalf("defaults %+v", s)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestMatchesOrderAndIDs(t *testing.T) {
	s := &Spec{Name: "t", Seeds: []int{2, 1}, Starts: []string{"none"}, Personas: []string{"shark"},
		Players: []Player{{ID: "a", Mode: ModeHeuristic}, {ID: "b", Mode: ModeLocal}}}
	var ids []string
	for _, m := range s.Matches() {
		ids = append(ids, m.ID())
	}
	want := "a__none__s2__shark b__none__s2__shark a__none__s1__shark b__none__s1__shark"
	if got := strings.Join(ids, " "); got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestIsGameImage(t *testing.T) {
	tests := []struct {
		argv0 string
		want  bool
	}{
		{`Z:\home\x\.local\share\Steam\steamapps\common\Probably Stolen\Probably Stolen.exe`, true},
		{`C:\games\probably stolen.EXE`, true},
		{"/usr/bin/zsh", false},
		{"pgrep", false},
		{"Probably Stolen.exe.bak", false},
	}
	for _, tt := range tests {
		if got := isGameImage(tt.argv0); got != tt.want {
			t.Errorf("isGameImage(%q) = %v", tt.argv0, got)
		}
	}
}

// A shell whose command line names the game must not count as the game.
func TestGameRunningMatchesImageNotCommandLine(t *testing.T) {
	proc := t.TempDir()
	mk := func(pid, cmdline, comm string) {
		d := filepath.Join(proc, pid)
		os.MkdirAll(d, 0o755)
		os.WriteFile(filepath.Join(d, "cmdline"), []byte(cmdline), 0o644)
		os.WriteFile(filepath.Join(d, "comm"), []byte(comm+"\n"), 0o644)
	}
	mk("100", "zsh\x00-c\x00pgrep -f 'Probably Stolen.exe'\x00", "zsh")
	mk("101", "pgrep\x00-f\x00Probably Stolen.exe\x00", "pgrep")
	if gameRunning(proc) {
		t.Fatal("matched a command line")
	}
	mk("200", `Z:\games\Probably Stolen\Probably Stolen.exe`+"\x00", "Probably Stolen")
	if !gameRunning(proc) {
		t.Fatal("missed the game by argv[0]")
	}
	os.RemoveAll(filepath.Join(proc, "200"))
	mk("300", "wine64-preloader\x00", "Probably Stolen")
	if !gameRunning(proc) {
		t.Fatal("missed the game by comm")
	}
}

func TestBootstrapDeterministic(t *testing.T) {
	x := []float64{45, 60, 79, 80, 52, 101}
	a, b := boot(x, median, seedFor("l", "p", "days")), boot(x, median, seedFor("l", "p", "days"))
	if a != b {
		t.Fatalf("same seed, different CI: %+v %+v", a, b)
	}
	if a.Est != 69.5 || a.Lo > a.Est || a.Hi < a.Est || a.Lo < 45 || a.Hi > 101 {
		t.Fatalf("CI %+v", a)
	}
	if c := boot([]float64{7}, mean, 1); c.Est != 7 || c.Lo != 7 || c.Hi != 7 {
		t.Fatalf("single sample %+v", c)
	}
}

func TestBuildResultAndReport(t *testing.T) {
	dir := t.TempDir()
	spec := &Spec{Name: "t", Seeds: []int{1}, Starts: []string{"none"}, Difficulty: "hard", Days: 200, Baseline: "h",
		Players: []Player{{ID: "h", Mode: ModeHeuristic}, {ID: "p", Mode: ModePlan, Endpoint: "e"}}}
	ms := spec.Matches()
	write := func(m Match, summary, traj, llm, bridgeLog, decisions string) {
		md := filepath.Join(dir, m.ID())
		os.MkdirAll(filepath.Join(md, "log"), 0o755)
		os.WriteFile(filepath.Join(md, "gym.summary.json"), []byte(summary), 0o644)
		os.WriteFile(filepath.Join(md, "trajectory.jsonl"), []byte(traj), 0o644)
		os.WriteFile(filepath.Join(md, "llm.jsonl"), []byte(llm), 0o644)
		os.WriteFile(filepath.Join(md, "bridge.log"), []byte(bridgeLog), 0o644)
		os.WriteFile(filepath.Join(md, "log", "s-1.jsonl"), []byte(decisions), 0o644)
		r, err := BuildResult(m, spec, md)
		if err != nil {
			t.Fatal(err)
		}
		if err := writeJSON(filepath.Join(md, resultFile), r); err != nil {
			t.Fatal(err)
		}
	}
	write(ms[0], `{"last_day":3,"days_played":3,"cash":0,"game_over":"rent","crash":null,"cash_by_day":[100,50,0],"net_worth_by_day":[300,200,100],"stats":{"customers":9,"decisions":6}}`,
		`{"game_day":1,"labels":[{"source":"day_snapshot","hidden":{"aug_presence":15,"security":{"evidence":2}}}]}
{"game_day":3,"labels":[{"source":"day_snapshot","hidden":{"aug_presence":30}}]}
`, "", "", `{"type":"decision","response":{"source":"heuristic"}}
{"type":"decision","response":{"source":"fallback"}}
`)
	write(ms[1], `{"last_day":5,"days_played":5,"cash":10,"game_over":"aug","cash_by_day":[1,2,3,4,10],"net_worth_by_day":[1,2,3,4,500],"stats":{"customers":9,"decisions":6}}`,
		"", `{"status":200,"latency_ms":1000,"prompt_tokens":10,"completion_tokens":5}
{"status":500,"latency_ms":3000,"error":"x"}
`, `level=INFO msg="day plan written" day=1
level=WARN msg="day plan failed; keeping the previous plan" day=2
`, "")

	rs, err := LoadResults(spec, dir)
	if err != nil || len(rs) != 2 {
		t.Fatalf("results %d, %v", len(rs), err)
	}
	h, p := rs[0], rs[1]
	if h.Ending != "rent" || h.MaxAug != 30 || len(h.AugByDay) != 3 || h.AugByDay[1] != 15 || h.Fallbacks != 1 || h.Evidence[0] != 2 {
		t.Fatalf("heuristic result %+v", h)
	}
	if p.Ending != "aug" || p.LLMCalls != 2 || p.LLMErrors != 1 || p.PromptTokens != 10 || p.LatencyP50MS != 3000 || p.Plans != 1 || p.PlanFailures != 1 || p.PeakNW != 500 {
		t.Fatalf("plan result %+v", p)
	}
	if h.NWAt(3) != 100 || h.NWAt(30) != 0 {
		t.Fatalf("nw at %d %d", h.NWAt(3), h.NWAt(30))
	}
	sum := Summarize(spec, rs)
	if len(sum.Paired) != 1 || sum.Paired[0].Days.Est != 2 {
		t.Fatalf("paired %+v", sum.Paired)
	}
	md := sum.Markdown()
	if md != Summarize(spec, rs).Markdown() {
		t.Fatal("report is not deterministic")
	}
	for _, want := range []string{"| rent |", "| aug |", "| h | heuristic · "} {
		if !strings.Contains(md, want) && !strings.Contains(md, strings.Trim(want, "| ")) {
			t.Errorf("report lacks %q:\n%s", want, md)
		}
	}
}

func TestPerkSets(t *testing.T) {
	fixed := &Spec{Name: "t", Seeds: []int{1}, Perks: []string{"none", "b+a"}, Players: []Player{{ID: "h", Mode: ModeHeuristic}}}
	if err := fixed.normalize(); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, m := range fixed.Matches() {
		ids = append(ids, m.ID())
	}
	if got := strings.Join(ids, " "); got != "h__none__s1 h__none__s1__p-a+b" {
		t.Fatalf("fixed sets: %s", got)
	}

	cat := filepath.Join(t.TempDir(), "perks.json")
	os.WriteFile(cat, []byte(`{"selection":{"points":3,"base_slots":4},"perks":[
		{"id":"a","kind":"starting","cost":2,"incompatible":["b"]},
		{"id":"b","kind":"starting","cost":2,"incompatible":["a"]},
		{"id":"c","kind":"starting","cost":-3},
		{"id":"d","kind":"starting","cost":1,"impact":"minor"},
		{"id":"e","kind":"starting","cost":1},
		{"id":"rep","kind":"reputation","cost":0}]}`), 0o644)
	sample := func(include bool) *Spec {
		return &Spec{Name: "t", Seeds: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, Starts: []string{"none", "scav"},
			PerkSample: &PerkSample{Catalogue: cat, NoneShare: 0.3, CuratedShare: 0.2, Curated: []string{"c+a", "d+e"},
				RuleChanging: []string{"c"}, HeldOut: HeldOut{Perks: []string{"b"}, Combos: []string{"d+e"}}, IncludeHeldOut: include},
			Players: []Player{{ID: "h", Mode: ModeHeuristic}, {ID: "l", Mode: ModeLocal}}}
	}
	sampled := sample(false)
	if err := sampled.normalize(); err != nil {
		t.Fatal(err)
	}
	ms := sampled.Matches()
	sets := map[string]int{}
	for i := 0; i < len(ms); i += 2 {
		if ms[i].Perks != ms[i+1].Perks {
			t.Fatalf("players on seed %d drew different perks: %s %s", ms[i].Seed, ms[i].Perks, ms[i+1].Perks)
		}
		if ms[i].HeldOut || strings.Contains(ms[i].Perks, "b") || ms[i].Perks == "d+e" || strings.Contains(ms[i].Perks, "rep") {
			t.Fatalf("training league drew %q (held out %v)", ms[i].Perks, ms[i].HeldOut)
		}
		if ms[i].Perks != NoPerks {
			if _, err := sampled.PerkSample.legalSet(strings.Split(ms[i].Perks, "+")); err != nil {
				t.Fatalf("illegal loadout %q: %v", ms[i].Perks, err)
			}
		}
		sets[ms[i].Perks]++
	}
	if sets[NoPerks] == 0 || sets["a+c"] == 0 || len(sets) < 4 {
		t.Fatalf("draws %v", sets)
	}
	if again := sampled.Matches(); again[0].Perks != ms[0].Perks || again[len(again)-1].Perks != ms[len(ms)-1].Perks {
		t.Fatal("sampling is not deterministic")
	}
	eval := sample(true)
	if err := eval.normalize(); err != nil {
		t.Fatal(err)
	}
	held := 0
	for _, m := range eval.Matches() {
		if m.HeldOut {
			held++
		}
	}
	if held == 0 {
		t.Fatal("an evaluation league drew no held-out sets")
	}
	illegal := sample(false)
	illegal.PerkSample.Curated = []string{"a+b"}
	if err := illegal.normalize(); err == nil || !strings.Contains(err.Error(), "incompatible") {
		t.Fatalf("incompatible curated combo: %v", err)
	}
	bad := &Spec{Name: "t", Seeds: []int{1}, Perks: []string{"a"}, PerkSample: &PerkSample{Catalogue: "x"}, Players: []Player{{ID: "h", Mode: ModeHeuristic}}}
	if err := bad.normalize(); err == nil {
		t.Fatal("perks and perk_sample together must fail")
	}
}

func TestBaseDirectivesReachGymAndPlanBridge(t *testing.T) {
	spec := &Spec{Name: "x", Seeds: []int{1}, Days: 10, PlanWait: "1s", DeadlineMS: 1000,
		Executor: Executor{Human: "player", Directives: "/d/base.json"}}
	l := &League{Spec: spec, Local: &Local{}, GymDLL: "gym.dll"}
	for _, tc := range []struct {
		mode       string
		bridgeBase bool
	}{{ModePlan, true}, {ModeHeuristic, false}} {
		m := Match{Player: Player{ID: "p", Mode: tc.mode}, Seed: 1, Start: "none", Perks: NoPerks, Difficulty: "hard"}
		if g := strings.Join(l.gymArgs(m, "", "/m"), " "); !strings.Contains(g, "-directives /d/base.json") {
			t.Errorf("%s gym args %q lack the base", tc.mode, g)
		}
		b := strings.Join(l.bridgeArgs(m, nil, "", "127.0.0.1:1", "/m"), " ")
		if got := strings.Contains(b, "-base-directives /d/base.json"); got != tc.bridgeBase {
			t.Errorf("%s bridge args %q: base shown %v, want %v", tc.mode, b, got, tc.bridgeBase)
		}
	}
}

// TestRouteRunSpec: a route entry is one run spec: the gym gets the route's start, perks and
// directives, a plan bridge the route, its base policy and the persona.
func TestRouteRunSpec(t *testing.T) {
	routes := filepath.Join("..", "..", "sim", "lab", "scripts", "directives", "routes")
	smelter, rock := filepath.Join(routes, "smelter.json"), filepath.Join(routes, "rock_bottom.json")
	dir := t.TempDir()
	write := func(s string) string {
		p := filepath.Join(dir, "s.json")
		os.WriteFile(p, []byte(s), 0o644)
		return p
	}
	spec, err := LoadSpec(write(`{"name":"r","seeds":[7],"days":10,"executor":{"human":"player"},
		"players":[{"id":"q","endpoint":"e","mode":"plan"}],
		"routes":[{"route":"` + smelter + `"},{"route":"` + rock + `","persona":"vintner"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	ms := spec.Matches()
	if len(ms) != 2 {
		t.Fatalf("%d matches", len(ms))
	}
	if m := ms[0]; m.ID() != "q__r-smelter__s7__smelter" || m.Start != "apprentice" || m.Perks != "short_sleeper+vending" {
		t.Fatalf("smelter match %s %+v", m.ID(), m)
	}
	if m := ms[1]; m.ID() != "q__r-rock_bottom__s7__vintner" || m.Start != "rock_bottom" {
		t.Fatalf("rock bottom match %s %+v", m.ID(), m)
	}
	l := &League{Spec: spec, Local: &Local{}, GymDLL: "gym.dll", Persona: true, Perks: true}
	m := ms[1]
	g := strings.Join(l.gymArgs(m, "", "/m"), " ")
	for _, want := range []string{"-start rock_bottom", "-perks short_sleeper+vending", "-directives " + m.RoutePath} {
		if !strings.Contains(g, want) {
			t.Errorf("gym args %q lack %q", g, want)
		}
	}
	b := strings.Join(l.bridgeArgs(m, nil, "", "127.0.0.1:1", "/m"), " ")
	for _, want := range []string{"-base-directives " + m.RoutePath, "-route " + m.RoutePath, "-persona vintner"} {
		if !strings.Contains(b, want) {
			t.Errorf("bridge args %q lack %q", b, want)
		}
	}

	for _, extra := range []string{`"starts":["fence"]`, `"perks":["vending"]`, `"personas":["shark"]`} {
		_, err := LoadSpec(write(`{"name":"r","seeds":[7],"players":[{"id":"h","mode":"heuristic"}],` + extra + `,"routes":[{"route":"` + smelter + `"}]}`))
		if err == nil || !strings.Contains(err.Error(), "exclusive") {
			t.Errorf("%s with routes: err %v", extra, err)
		}
	}
	if _, err := LoadSpec(write(`{"name":"r","seeds":[7],"players":[{"id":"h","mode":"heuristic"}],"routes":[{"route":"nope"}]}`)); err == nil {
		t.Error("an unknown route must fail")
	}
}
