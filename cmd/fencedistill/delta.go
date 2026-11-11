package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/route"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

// The round-2 dataset (docs/research/DISTILL.md, "Round 2"): the delta planner's one-call
// task (strategy.DistillDeltaPrompt), with targets that an outcome backs:
//   - hybrid decisions (decisions.jsonl): the candidate the rollouts chose, with its reason;
//   - plan-mode delta games that won (bridge.log): the teacher's linted delta for the day;
//   - days whose answer is {} (the executor's own play), heavily down-weighted.
type deltaOptions struct {
	hybrid, league []string
	repo           string
	out            string
	holdout        []int64
	collapse       int
	switchW        float64 // a hybrid decision that changed the plan in force
	changeW        float64 // a teacher day whose delta differs from yesterday's
	repeatW        float64 // a non-empty delta repeated from yesterday
	emptyW         float64 // {} repeated: the executor's own play
	blindW         float64 // multiplier for blind-ablation proposals (no written reason)
	rationaleMax   int
}

func deltaBuild(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fencedistill delta", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o deltaOptions
	var hybrid, league, holdout string
	fs.StringVar(&hybrid, "hybrid", "", "comma-separated league dirs (globs) whose matches hold decisions.jsonl")
	fs.StringVar(&league, "league", "", "comma-separated league dirs (globs) whose plan-mode delta matches hold bridge.log")
	fs.StringVar(&o.repo, "repo", ".", "the checkout match.json's executor directives paths are relative to")
	fs.StringVar(&o.out, "out", "var/distill/r2", "output directory for train.jsonl, val.jsonl, stats.json")
	fs.StringVar(&holdout, "holdout", "", "comma-separated seeds whose rows go to val only")
	fs.IntVar(&o.collapse, "collapse", 14, "drop hybrid decisions this close to a lost game's end")
	fs.Float64Var(&o.switchW, "switch-weight", 3, "weight of a hybrid decision that changed the plan in force")
	fs.Float64Var(&o.changeW, "change-weight", 2, "weight of a teacher day whose delta differs from yesterday's")
	fs.Float64Var(&o.repeatW, "repeat-weight", 0.5, "weight of a non-empty delta repeated from yesterday")
	fs.Float64Var(&o.emptyW, "empty-weight", 0.1, "weight of a {} answer repeated from yesterday (the executor's play)")
	fs.Float64Var(&o.blindW, "blind-weight", 0.5, "multiplier for rows chosen among the blind ablation's lever candidates")
	fs.IntVar(&o.rationaleMax, "rationale-max", 300, "longest reason, in bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var err error
	if o.hybrid, err = globDirs(hybrid); err != nil {
		return err
	}
	if o.league, err = globDirs(league); err != nil {
		return err
	}
	for _, s := range splitList(holdout) {
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("-holdout %q: %w", s, err)
		}
		o.holdout = append(o.holdout, n)
	}
	st, err := buildDelta(o)
	if err != nil {
		return err
	}
	return st.print(stdout)
}

func globDirs(list string) ([]string, error) {
	var out []string
	for _, p := range splitList(list) {
		m, err := filepath.Glob(p)
		if err != nil {
			return nil, err
		}
		out = append(out, m...)
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

// DeltaStats is the round-2 report written next to the data.
type DeltaStats struct {
	Games     []DeltaGame        `json:"games"`
	Kept      map[string]int     `json:"kept"` // by split/source/kind
	Dropped   map[string]int     `json:"dropped"`
	Weight    map[string]float64 `json:"weight"` // by split/source/kind
	Keys      map[string]int     `json:"changed_keys"`
	PromptTok map[string]int     `json:"prompt_tokens"`
	TargetTok map[string]int     `json:"target_tokens"`
}

type DeltaGame struct {
	Match  string `json:"match"`
	Source string `json:"source"`
	Seed   int64  `json:"seed"`
	Base   string `json:"base"`
	Ending string `json:"ending"`
	Days   int    `json:"days"`
	// Pace is the weight multiplier from the executor's result on the same seed (teacher
	// games): its win day over this game's, clamped to [0.5, 2].
	Pace float64 `json:"pace,omitempty"`
	Kept int     `json:"kept"`
}

// matchMeta is what a league match records about itself: match.json and result.json.
type matchMeta struct {
	Seed     int64 `json:"seed"`
	Executor struct {
		Directives string `json:"directives"`
	} `json:"executor"`
	Player struct {
		ID   string `json:"id"`
		Mode string `json:"mode"`
	} `json:"player"`
	Ending string `json:"-"`
	Days   int    `json:"-"`
	Base   string `json:"-"` // the base directives text, as the bridge shows it
	base   map[string]any
}

func readMatch(dir, repo string) (*matchMeta, error) {
	var m matchMeta
	if err := readJSON(filepath.Join(dir, "match.json"), &m); err != nil {
		return nil, err
	}
	var res struct {
		Ending     string `json:"ending"`
		DaysPlayed int    `json:"days_played"`
	}
	if err := readJSON(filepath.Join(dir, "result.json"), &res); err != nil {
		return nil, err
	}
	m.Ending, m.Days = res.Ending, res.DaysPlayed
	if p := m.Executor.Directives; p != "" {
		if !filepath.IsAbs(p) {
			p = filepath.Join(repo, p)
		}
		var err error
		if m.Base, m.base, err = readBase(p); err != nil {
			return nil, err
		}
	}
	return &m, nil
}

// readBase is a base directives file as the bridge shows it (-base-directives) and as a
// normalized delta.
func readBase(path string) (string, map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	text, err := route.StripMeta(b)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	m, err := normDelta(json.RawMessage(text), nil)
	if err != nil {
		return "", nil, fmt.Errorf("%s: %w", path, err)
	}
	return text, m, nil
}

func won(ending string) bool { return strings.HasPrefix(ending, "buyout") }

func buildDelta(o deltaOptions) (*DeltaStats, error) {
	st := &DeltaStats{Kept: map[string]int{}, Dropped: map[string]int{}, Weight: map[string]float64{}, Keys: map[string]int{}}
	var all []Example
	for _, l := range o.hybrid {
		games, _ := filepath.Glob(filepath.Join(l, "*", "decisions.jsonl"))
		for _, f := range games {
			ex, err := hybridExamples(filepath.Dir(f), o, st)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", filepath.Dir(f), err)
			}
			all = append(all, ex...)
		}
	}
	for _, l := range o.league {
		games, _ := filepath.Glob(filepath.Join(l, "*", "bridge.log"))
		for _, f := range games {
			ex, err := teacherExamples(filepath.Dir(f), o, st)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", filepath.Dir(f), err)
			}
			all = append(all, ex...)
		}
	}

	seen := map[[32]byte]bool{}
	var train, val []Example
	var pTok, tTok []int
	for _, e := range all {
		h := sha256.Sum256([]byte(e.Messages[1].Content + "\x00" + e.Messages[2].Content))
		if seen[h] {
			st.Dropped["duplicate"]++
			continue
		}
		seen[h] = true
		if e.Meta.Split == "val" {
			val = append(val, e)
		} else {
			train = append(train, e)
		}
		k := e.Meta.Split + "/" + e.Meta.Source + "/" + e.Meta.Kind
		st.Kept[k]++
		st.Weight[k] += e.Weight
		pTok = append(pTok, approxTokens(e.Messages[0].Content+e.Messages[1].Content))
		tTok = append(tTok, approxTokens(e.Messages[2].Content))
	}
	st.PromptTok, st.TargetTok = spread(pTok), spread(tTok)
	if err := os.MkdirAll(o.out, 0o755); err != nil {
		return nil, err
	}
	if err := writeJSONL(filepath.Join(o.out, "train.jsonl"), train); err != nil {
		return nil, err
	}
	if err := writeJSONL(filepath.Join(o.out, "val.jsonl"), val); err != nil {
		return nil, err
	}
	for k, v := range st.Weight {
		st.Weight[k] = round3(v)
	}
	b, _ := json.MarshalIndent(st, "", "  ")
	return st, os.WriteFile(filepath.Join(o.out, "stats.json"), append(b, '\n'), 0o644)
}

// hybridDecision is one row of fence-rollout's decisions.jsonl (docs/research/HYBRID.md).
type hybridDecision struct {
	Day      int             `json:"day"`
	Snapshot stepSnapshot    `json:"snapshot"`
	InForce  json.RawMessage `json:"in_force"`
	Played   string          `json:"played"`
	Cands    []struct {
		Name       string          `json:"name"`
		Source     string          `json:"source"`
		Why        string          `json:"why"`
		Directives json.RawMessage `json:"directives"`
		Score      float64         `json:"score"`
		Z          float64         `json:"z_vs_first"`
	} `json:"candidates"`
}

func hybridExamples(dir string, o deltaOptions, st *DeltaStats) ([]Example, error) {
	m, err := readMatch(dir, o.repo)
	if errors.Is(err, os.ErrNotExist) {
		st.Dropped["hybrid_unfinished_game"]++
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g := DeltaGame{Match: matchName(dir), Source: "hybrid", Seed: m.Seed, Base: filepath.Base(m.Executor.Directives), Ending: m.Ending, Days: m.Days}
	defer func() { st.Games = append(st.Games, g) }()
	f, err := os.Open(filepath.Join(dir, "decisions.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	var out []Example
	// The reason the plan in force was adopted with, so a "keep" row can restate it.
	inForceWhy := ""
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var r hybridDecision
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, err
		}
		inForce, err := normDelta(r.InForce, m.base)
		if err == nil {
			inForce, err = playable(inForce, r, m)
		}
		if err != nil {
			st.Dropped["hybrid_bad_delta"]++
			continue
		}
		var played, blind bool
		var target map[string]any
		var why string
		for _, c := range r.Cands {
			if c.Name != r.Played {
				continue
			}
			played = true
			if target, err = normDelta(c.Directives, m.base); err != nil {
				break
			}
			if target, err = playable(target, r, m); err != nil {
				break
			}
			why, blind = strings.TrimSpace(c.Why), c.Source == "blind"
		}
		if !played || err != nil {
			st.Dropped["hybrid_bad_delta"]++
			continue
		}
		lost := m.Ending != "" && !won(m.Ending)
		if lost && r.Day > m.Days-o.collapse {
			st.Dropped["hybrid_before_collapse"]++
			continue
		}
		// The h2 finding: a rollout horizon ends before a repossession, so the rollouts
		// sign mortgages that lose. A repossessed game's mortgage moves are not evidence.
		if m.Ending == "repo" && touchesMortgage(target, inForce) {
			st.Dropped["hybrid_mortgage_in_repo_game"]++
			continue
		}
		changed := len(diffKeys(target, inForce)) > 0
		var kind string
		var w float64
		switch {
		case changed:
			kind, w = "switch", o.switchW
			if blind {
				w *= o.blindW
			}
			if why == "" {
				why = "a better line than the plan in force for the coming weeks."
			}
			inForceWhy = why
			for k := range diffKeys(target, inForce) {
				st.Keys[k]++
			}
		case len(target) == 0:
			kind, w = "empty", o.emptyW
		default:
			kind, w = "keep", o.repeatW
		}
		answer := deltaAnswerOne(target, nonEmpty(inForceWhy, "the plan in force still holds."), o.rationaleMax)
		yp := strategy.DayPlan{Day: r.Day - 1, Plan: changesLine(inForce)}
		in := strategy.PlanInput{Day: r.Day, Snapshot: r.Snapshot.Snapshot, Yesterday: &yp, BaseDirectives: m.Base}
		ex, ok := deltaExample(in, answer, w, "hybrid", kind, dir, m, o, st)
		if ok {
			g.Kept++
			out = append(out, ex)
		}
	}
	return out, sc.Err()
}

// teacherExamples reads a plan-mode delta match that won: each morning's day_snapshot from
// the bridge's decision log and the day's linted delta (and its reasons) from bridge.log.
func teacherExamples(dir string, o deltaOptions, st *DeltaStats) ([]Example, error) {
	m, err := readMatch(dir, o.repo)
	if errors.Is(err, os.ErrNotExist) {
		st.Dropped["teacher_unfinished_game"]++
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if m.Player.Mode != "plan" {
		return nil, nil
	}
	g := DeltaGame{Match: matchName(dir), Source: "teacher", Seed: m.Seed, Base: filepath.Base(m.Executor.Directives), Ending: m.Ending, Days: m.Days}
	defer func() { st.Games = append(st.Games, g) }()
	if !won(m.Ending) {
		st.Dropped["teacher_lost_game"]++
		return nil, nil
	}
	g.Pace = pace(dir, m, o.league)
	mornings, err := morningSnapshots(filepath.Join(dir, "log"))
	if err != nil {
		return nil, err
	}
	plans, err := bridgeDeltas(filepath.Join(dir, "bridge.log"))
	if err != nil {
		return nil, err
	}
	days := make([]int, 0, len(plans))
	for d := range plans {
		days = append(days, d)
	}
	slices.Sort(days)
	var out []Example
	var prev map[string]any
	prevDay := 0
	for _, d := range days {
		p := plans[d]
		target, err := normDelta(p.json, m.base)
		if err != nil {
			st.Dropped["teacher_bad_delta"]++
			continue
		}
		why := map[string]string{}
		for k := range target {
			w := nonEmpty(p.why[k], p.why["*"])
			if w == "" {
				st.Dropped["teacher_key_without_reason"]++
				delete(target, k)
				continue
			}
			why[k] = w
		}
		snap, ok := mornings[d]
		if !ok {
			st.Dropped["teacher_no_morning"]++
			prev, prevDay = target, d
			continue
		}
		var yp *strategy.DayPlan
		if prev != nil {
			yp = &strategy.DayPlan{Day: prevDay, Plan: changesLine(prev)}
		}
		var kind string
		var w float64
		switch {
		case prev == nil || len(diffKeys(target, prev)) > 0:
			kind, w = "change", o.changeW
			for k := range diffKeys(target, prev) {
				st.Keys[k]++
			}
			if len(target) == 0 && prev == nil {
				kind, w = "empty", o.emptyW
			}
		case len(target) == 0:
			kind, w = "empty", o.emptyW
		default:
			kind, w = "repeat", o.repeatW
		}
		w *= g.Pace
		prev, prevDay = target, d
		in := strategy.PlanInput{Day: d, Snapshot: snap, Yesterday: yp, BaseDirectives: m.Base}
		ex, ok := deltaExample(in, deltaAnswer(target, why, o.rationaleMax), w, "teacher", kind, dir, m, o, st)
		if ok {
			g.Kept++
			out = append(out, ex)
		}
	}
	return out, nil
}

func deltaExample(in strategy.PlanInput, answer string, w float64, source, kind, dir string, m *matchMeta, o deltaOptions, st *DeltaStats) (Example, bool) {
	system, user, err := strategy.DistillDeltaPrompt(in)
	if err != nil {
		st.Dropped[source+"_prompt_error"]++
		return Example{}, false
	}
	split := "train"
	if slices.Contains(o.holdout, m.Seed) {
		split = "val"
	}
	return Example{
		Messages: []Message{{"system", system}, {"user", user}, {"assistant", answer}},
		Weight:   round3(w),
		Meta: Meta{Source: source, Game: matchName(dir), Day: in.Day, Difficulty: "hard", Kind: kind,
			Repeat: kind == "keep" || kind == "repeat" || kind == "empty", Ending: m.Ending, Split: split,
			Format: "delta", Seed: m.Seed, Base: filepath.Base(m.Executor.Directives)},
	}, true
}

// pace compares a won game with the executor alone on its seed (a league's baseline match,
// "player__…", beside it or in another league on the same base): a faster win than the
// executor weighs more, a slower one less.
func pace(dir string, m *matchMeta, leagues []string) float64 {
	_, rest, ok := strings.Cut(filepath.Base(dir), "__")
	if !ok || m.Days == 0 {
		return 1
	}
	var ex struct {
		Ending     string `json:"ending"`
		DaysPlayed int    `json:"days_played"`
	}
	found := false
	for _, l := range append([]string{filepath.Dir(dir)}, leagues...) {
		var spec struct {
			Executor struct {
				Directives string `json:"directives"`
			} `json:"executor"`
		}
		if readJSON(filepath.Join(l, "spec.json"), &spec) != nil || spec.Executor.Directives != m.Executor.Directives {
			continue
		}
		if readJSON(filepath.Join(l, "player__"+rest, "result.json"), &ex) == nil {
			found = true
			break
		}
	}
	if !found {
		return 1
	}
	if !won(ex.Ending) {
		return 2
	}
	return round3(math.Min(math.Max(float64(ex.DaysPlayed)/float64(m.Days), 0.5), 2))
}

func matchName(dir string) string { return filepath.Base(filepath.Dir(dir)) + "/" + filepath.Base(dir) }

// normDelta keeps what the delta contract carries (protocol.Directives) and changes: unknown
// keys and sub-keys, "day", empty objects and values that restate base go.
func normDelta(raw json.RawMessage, base map[string]any) (map[string]any, error) {
	out := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return out, nil
	}
	var d protocol.Directives
	if err := json.Unmarshal(raw, &d); err != nil {
		return nil, err
	}
	d.Day = 0
	b, _ := json.Marshal(d)
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	for k, v := range out {
		mm, isMap := v.(map[string]any)
		if bm, ok := base[k].(map[string]any); ok && isMap {
			for kk, vv := range mm {
				if exactly(vv, bm[kk]) {
					delete(mm, kk)
				}
			}
		} else if base != nil && exactly(v, base[k]) {
			delete(out, k)
			continue
		}
		if isMap && len(mm) == 0 {
			delete(out, k)
		}
	}
	return out, nil
}

func exactly(a, b any) bool {
	return deltaKey(map[string]any{"": a}) == deltaKey(map[string]any{"": b})
}

// sameValue says whether two delta values play alike: numbers within 15 % (or 25 credits
// from 100 up), lists as sets, objects key by key.
func sameValue(a, b any) bool {
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		tol := 0.15 * math.Abs(y)
		if math.Abs(y) >= 100 {
			tol = math.Max(tol, 25)
		}
		return math.Abs(x-y) <= tol
	case []any:
		y, ok := b.([]any)
		if !ok {
			return len(x) == 0 && b == nil
		}
		set := func(l []any) []string {
			var o []string
			for _, v := range l {
				o = append(o, deltaKey(map[string]any{"": v}))
			}
			slices.Sort(o)
			return slices.Compact(o)
		}
		return slices.Equal(set(x), set(y))
	case map[string]any:
		y, _ := b.(map[string]any)
		for k, v := range x {
			if !sameValue(v, y[k]) {
				return false
			}
		}
		for k, v := range y {
			if _, ok := x[k]; !ok && !sameValue(nil, v) {
				return false
			}
		}
		return true
	case nil:
		switch y := b.(type) {
		case []any:
			return len(y) == 0
		case map[string]any:
			return len(y) == 0
		}
		return b == nil
	}
	return deltaKey(map[string]any{"": a}) == deltaKey(map[string]any{"": b})
}

func deltaKey(d map[string]any) string { b, _ := json.Marshal(d); return string(b) }

// diffKeys are the top-level keys whose values differ between two deltas.
func diffKeys(a, b map[string]any) map[string]bool {
	out := map[string]bool{}
	for k, v := range a {
		if !sameValue(v, b[k]) {
			out[k] = true
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out[k] = true
		}
	}
	return out
}

func touchesMortgage(target, inForce map[string]any) bool {
	for k := range diffKeys(target, inForce) {
		if strings.HasPrefix(k, "mortgage") {
			return true
		}
	}
	return false
}

// playable is what the delta planner's lints (strategy.FinishDistillDelta) would play of a
// hybrid delta: the hybrid plays its candidates unlinted, but the student's answers are
// linted, so a change the lints undo teaches nothing.
func playable(d map[string]any, r hybridDecision, m *matchMeta) (map[string]any, error) {
	if len(d) == 0 {
		return d, nil
	}
	b, err := marshalNoEscape(d)
	if err != nil {
		return nil, err
	}
	answer := `{"why":"x",` + strings.TrimPrefix(b, "{")
	dr := strategy.FinishDistillDelta(answer, r.Snapshot.Snapshot, r.Day, m.Base)
	if dr.Err != nil {
		return nil, dr.Err
	}
	out, _ := json.Marshal(dr.Directives)
	return normDelta(out, m.base)
}

// deltaAnswerOne is deltaAnswer with one reason for every key, which the delta contract
// takes as a string "why".
func deltaAnswerOne(d map[string]any, why string, limit int) string {
	if len(d) == 0 {
		return "{}"
	}
	w, _ := marshalNoEscape(clip(oneLine(why), limit))
	rest, _ := marshalNoEscape(d)
	return `{"why":` + w + "," + strings.TrimPrefix(rest, "{")
}

// deltaAnswer is the target: the delta object with "why" first, so the reasons are written
// before the changes they justify.
func deltaAnswer(d map[string]any, why map[string]string, limit int) string {
	if len(d) == 0 {
		return "{}"
	}
	w := map[string]string{}
	for k := range d {
		if r := oneLine(why[k]); r != "" {
			w[k] = clip(r, limit)
		}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(w)
	rest, _ := marshalNoEscape(d)
	return `{"why":` + strings.TrimSpace(b.String()) + "," + strings.TrimPrefix(rest, "{")
}

func marshalNoEscape(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	return strings.TrimSpace(b.String()), err
}

// changesLine is yesterday's delta as the prompt's "YESTERDAY'S PLAN".
func changesLine(d map[string]any) string {
	if len(d) == 0 {
		return "changes: {} (the executor's plan)"
	}
	s, _ := marshalNoEscape(d)
	return "changes: " + s
}

// morningSnapshots maps each game day to its morning day_snapshot event's data, the
// snapshot the strategist plans from.
func morningSnapshots(logDir string) (map[int]json.RawMessage, error) {
	files, _ := filepath.Glob(filepath.Join(logDir, "*.jsonl"))
	out := map[int]json.RawMessage{}
	for _, p := range files {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if !bytes.Contains(line, []byte(`"day_snapshot"`)) || !bytes.Contains(line, []byte(`"phase":"morning"`)) {
				continue
			}
			var r struct {
				Event struct {
					Kind    string          `json:"kind"`
					GameDay int             `json:"game_day"`
					Data    json.RawMessage `json:"data"`
				} `json:"event"`
			}
			if json.Unmarshal(line, &r) != nil || r.Event.Kind != "day_snapshot" {
				continue
			}
			if _, ok := out[r.Event.GameDay]; !ok {
				out[r.Event.GameDay] = r.Event.Data
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

type loggedDelta struct {
	json json.RawMessage
	why  map[string]string
}

// bridgeDeltas reads the strategist's `msg=directives day=N json=… why=…` lines (slog
// text) from bridge.log: the day's delta after the lints, and the model's reasons. A
// windfall re-plan logs a later line for the same day; the morning's (first) one is kept.
func bridgeDeltas(path string) (map[int]loggedDelta, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[int]loggedDelta{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, " msg=directives ") {
			continue
		}
		day, err := strconv.Atoi(slogValue(line, "day"))
		if err != nil {
			continue
		}
		if _, ok := out[day]; ok {
			continue
		}
		var why map[string]string
		if w := slogValue(line, "why"); w != "" && w != "null" {
			_ = json.Unmarshal([]byte(w), &why)
		}
		out[day] = loggedDelta{json: json.RawMessage(slogValue(line, "json")), why: why}
	}
	return out, sc.Err()
}

// slogValue is key's value in a slog text line, unquoted.
func slogValue(line, key string) string {
	i := strings.Index(line, " "+key+"=")
	if i < 0 {
		return ""
	}
	rest := line[i+len(key)+2:]
	if strings.HasPrefix(rest, `"`) {
		q, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return ""
		}
		v, _ := strconv.Unquote(q)
		return v
	}
	v, _, _ := strings.Cut(rest, " ")
	return v
}

func (st *DeltaStats) print(w io.Writer) error {
	fmt.Fprintln(w, "games:")
	for _, g := range st.Games {
		fmt.Fprintf(w, "  %-7s %-70s %-36s %-16s day %3d pace %4.2f kept %3d\n", g.Source, g.Match, g.Base, g.Ending, g.Days, g.Pace, g.Kept)
	}
	fmt.Fprintf(w, "kept: %v\n", sortedMap(st.Kept))
	fmt.Fprintf(w, "weight: %v\n", sortedMap(st.Weight))
	fmt.Fprintf(w, "dropped: %v\n", sortedMap(st.Dropped))
	fmt.Fprintf(w, "changed keys: %v\n", sortedMap(st.Keys))
	fmt.Fprintf(w, "prompt tokens (approx): %v\ntarget tokens (approx): %v\n", st.PromptTok, st.TargetTok)
	return nil
}
