// Command fencedistill turns strong play into supervised fine-tuning data for the
// strategist (docs/research/DISTILL.md): step-mode games (gym -step-dir: per-day snapshot,
// directives, notes, result) and rollout-search rows become chat examples whose prompt is
// the strategist's own (strategy.DistillPrompt) and whose answer is a WHY line plus the
// day's directives. Days are weighted by what the next week brought and dropped when they
// preceded a collapse. `fencedistill play` plays a gym -step-dir game with a model on the
// same prompt, for evaluation.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/strategy"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fencedistill:", err)
		os.Exit(1)
	}
}

type options struct {
	games        []string // step-mode game directories, or parents of them
	rollouts     []string // rollout-search JSONL files
	out          string
	horizon      int     // days ahead that judge a day
	collapse     int     // days before a game over that are dropped
	maxDrop      float64 // a next-horizon net worth fall beyond this fraction drops the day
	decisionMult float64
	repeatMult   float64
	winMult      float64
	directorMult float64
	valEvery     int // one block in valEvery goes to val
	block        int // days per split block
	valGames     []string
	exclude      []string
	gym          bool
	rationaleMax int
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 && args[0] == "play" {
		return play(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "delta" {
		return deltaBuild(args[1:], stdout, stderr)
	}
	if len(args) > 0 && args[0] == "build" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("fencedistill build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	var games, rollouts, valGames, exclude string
	fs.StringVar(&games, "games", "var/play,var/director/wins", "comma-separated step-mode game dirs or parents of them (a dir holding day-NNN/)")
	fs.StringVar(&rollouts, "rollouts", "", "comma-separated rollout-search JSONL files or globs (default var/rollout/*.jsonl when present)")
	fs.StringVar(&o.out, "out", "var/distill", "output directory for train.jsonl, val.jsonl, stats.json")
	fs.IntVar(&o.horizon, "horizon", 7, "days ahead whose net worth judges a day")
	fs.IntVar(&o.collapse, "collapse", 14, "drop days this close to a game over")
	fs.Float64Var(&o.maxDrop, "max-drop", 0.25, "drop a day when net worth fell more than this fraction over the horizon")
	fs.Float64Var(&o.decisionMult, "decision-weight", 2, "weight multiplier for decision days")
	fs.Float64Var(&o.repeatMult, "repeat-weight", 0.5, "weight multiplier for routine days repeating yesterday's directives")
	fs.Float64Var(&o.winMult, "win-weight", 1.5, "weight multiplier for every day of a game that ended in a buyout")
	fs.Float64Var(&o.directorMult, "director-weight", 1, "weight multiplier for director-policy days (routine policy rows; lower it to spend less compute on them)")
	fs.IntVar(&o.block, "block", 14, "days per train/val split block")
	fs.IntVar(&o.valEvery, "val-every", 8, "one split block in this many goes to val")
	fs.StringVar(&valGames, "val-games", "", "comma-separated substrings: games whose path matches are val only")
	fs.StringVar(&exclude, "exclude", "attempt", "comma-separated substrings: game paths to skip (abandoned restarts)")
	fs.BoolVar(&o.gym, "gym", true, "include the gym executor's extension keys in the prompt and the targets")
	fs.IntVar(&o.rationaleMax, "rationale-max", 500, "longest WHY line, in bytes")
	if err := fs.Parse(args); err != nil {
		return err
	}
	o.games = splitList(games)
	o.valGames = splitList(valGames)
	o.exclude = splitList(exclude)
	for _, p := range splitList(rollouts) {
		m, err := filepath.Glob(p)
		if err != nil {
			return err
		}
		o.rollouts = append(o.rollouts, m...)
	}
	if rollouts == "" {
		o.rollouts, _ = filepath.Glob("var/rollout/*.jsonl")
	}
	st, err := build(o)
	if err != nil {
		return err
	}
	return st.print(stdout)
}

// Example is one chat-format training row.
type Example struct {
	Messages []Message `json:"messages"`
	Weight   float64   `json:"weight"`
	Meta     Meta      `json:"meta"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Meta struct {
	Source     string  `json:"source"` // "play" or "rollout"
	Game       string  `json:"game"`
	Day        int     `json:"day"`
	Difficulty string  `json:"difficulty,omitempty"`
	Kind       string  `json:"kind"` // "decision", "routine" or "rollout"
	Repeat     bool    `json:"repeat,omitempty"`
	NetWorth   int64   `json:"net_worth"`
	Forward    float64 `json:"forward"` // net worth change over the horizon, as a fraction
	Ending     string  `json:"ending,omitempty"`
	Split      string  `json:"split"`
	// Round 2 (delta.go): "delta" rows answer strategy.DistillDeltaPrompt.
	Format string `json:"format,omitempty"`
	Seed   int64  `json:"seed,omitempty"`
	Base   string `json:"base,omitempty"`
}

// Stats is the report written next to the data.
type Stats struct {
	Games     []GameStats        `json:"games"`
	Rollouts  int                `json:"rollout_rows"`
	Kept      map[string]int     `json:"kept"`    // by split/kind
	Dropped   map[string]int     `json:"dropped"` // by reason
	Weight    map[string]float64 `json:"weight"`
	PromptTok map[string]int     `json:"prompt_tokens"` // approx (bytes/3.5): min, p50, p95, max
	TargetTok map[string]int     `json:"target_tokens"`
}

type GameStats struct {
	Path      string `json:"path"`
	Days      int    `json:"days"`
	Ending    string `json:"ending"`
	LastDay   int    `json:"last_day"`
	NetWorth  int64  `json:"net_worth"`
	Decisions int    `json:"decision_days"`
	Kept      int    `json:"kept"`
}

func build(o options) (*Stats, error) {
	st := &Stats{Kept: map[string]int{}, Dropped: map[string]int{}, Weight: map[string]float64{}}
	dirs, err := gameDirs(o.games)
	if err != nil {
		return nil, err
	}
	dirs = slices.DeleteFunc(dirs, func(d string) bool { return matchesAny(d, o.exclude) })
	var all []Example
	for _, d := range dirs {
		ex, gs, err := gameExamples(d, o, st)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", d, err)
		}
		st.Games = append(st.Games, gs)
		all = append(all, ex...)
	}
	for _, f := range o.rollouts {
		ex, err := rolloutExamples(f, o, st)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		all = append(all, ex...)
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
		st.Kept[e.Meta.Split+"/"+e.Meta.Kind]++
		st.Weight[e.Meta.Split] += e.Weight
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

// gameDirs expands each path to the step-mode game directories under it.
func gameDirs(paths []string) ([]string, error) {
	var out []string
	for _, p := range paths {
		err := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && fileExists(filepath.Join(path, "day-001", "snapshot.json")) {
				out = append(out, path)
				return filepath.SkipDir
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	slices.Sort(out)
	return slices.Compact(out), nil
}

type stepSnapshot struct {
	Day        int             `json:"day"`
	Seed       int64           `json:"seed"`
	Difficulty string          `json:"difficulty"`
	Snapshot   json.RawMessage `json:"snapshot"`
	Yesterday  *dayResult      `json:"yesterday"`
}

type dayResult struct {
	Day           int    `json:"day"`
	CashStart     int64  `json:"cash_start"`
	CashEnd       int64  `json:"cash_end"`
	NetWorthStart int64  `json:"net_worth_start"`
	NetWorthEnd   int64  `json:"net_worth_end"`
	SoldValue     int64  `json:"sold_value"`
	SoldItems     int64  `json:"sold_items"`
	Spent         int64  `json:"spent"`
	BoughtItems   int64  `json:"bought_items"`
	Crime         int64  `json:"crime"`
	Evidence      int64  `json:"evidence"`
	GameOver      string `json:"game_over"`
	Rent          *struct {
		Due      int64 `json:"due"`
		DaysLeft int64 `json:"days_left"`
	} `json:"rent"`
}

type finalResult struct {
	Day      int    `json:"day"`
	GameOver string `json:"game_over"`
	NetWorth int64  `json:"net_worth"`
}

type playDay struct {
	n          int
	snap       stepSnapshot
	directives map[string]any
	notes      string
	why        string // the director's why.txt
	hasWhy     bool
	result     *dayResult
}

func gameExamples(dir string, o options, st *Stats) ([]Example, GameStats, error) {
	gs := GameStats{Path: dir}
	days, err := readDays(dir)
	if err != nil {
		return nil, gs, err
	}
	gs.Days = len(days)
	var fin finalResult
	if err := readJSON(filepath.Join(dir, "final.json"), &fin); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, gs, err
	}
	// A game without final.json is still running: its last days cannot be judged yet.
	running := fin.Day == 0
	gs.Ending, gs.NetWorth = nonEmpty(fin.GameOver, "alive"), fin.NetWorth
	if running {
		gs.Ending = "running"
	}
	endDay := fin.Day
	if endDay == 0 && len(days) > 0 {
		endDay = days[len(days)-1].n
	}
	gs.LastDay = endDay

	// nwStart[d] is the net worth on the morning of day d; the day after the last result is
	// the final net worth.
	nwStart := map[int]int64{}
	for _, d := range days {
		if d.result != nil {
			nwStart[d.n] = d.result.NetWorthStart
			nwStart[d.n+1] = d.result.NetWorthEnd
		}
	}
	if fin.Day > 0 {
		nwStart[fin.Day] = fin.NetWorth
	}

	// A buyout is the win (game_over "buyout", "buyout_mortgage"), not a collapse.
	won := strings.HasPrefix(fin.GameOver, "buyout")
	lost := fin.GameOver != "" && !won
	notesSeen := map[string]bool{}
	heads := map[string]int{}
	for _, d := range days {
		heads[policyKey(d.notes)]++
	}
	source, judged, standing, err := gameSource(dir, days)
	if err != nil {
		return nil, gs, err
	}
	var prevTarget, prevWhy string
	var prevDay int
	var out []Example
	val := matchesAny(dir, o.valGames)
	for _, d := range days {
		drop := func(reason string) { st.Dropped[reason]++ }
		if d.directives == nil {
			drop("no_directives")
			continue
		}
		var kind, why string
		switch source {
		case "director":
			kind, why = "routine", clip(nonEmpty(d.why, "standing orders unchanged."), o.rationaleMax)
		case "play-gamenotes":
			if j, ok := judged[d.n]; ok {
				kind, why = "decision", clip(j, o.rationaleMax)
			} else {
				kind, why = "routine", clip(standing, o.rationaleMax)
			}
		default:
			kind, why = classify(d, heads, notesSeen, o.rationaleMax)
		}
		if kind == "decision" {
			gs.Decisions++
		}
		target := targetText(why, d.directives, o.gym)
		repeat := kind == "routine" && sameOrders(d.directives, prevTarget)
		system, user, err := strategy.DistillPrompt(planInput(d, prevWhy, prevDay), o.gym)
		ordersKey := ordersKey(d.directives)
		prevTarget, prevWhy, prevDay = ordersKey, why, d.n
		if err != nil {
			drop("prompt_error")
			continue
		}

		start, ok := nwStart[d.n]
		if !ok {
			drop("no_result")
			continue
		}
		if lost && d.n > endDay-o.collapse {
			drop("before_collapse")
			continue
		}
		ahead := d.n + o.horizon
		if ahead > endDay {
			ahead = endDay
		}
		later, ok := nwStart[ahead]
		if !ok || (running && d.n+o.horizon > endDay) {
			drop("unjudged")
			continue
		}
		fwd := float64(later-start) / math.Max(float64(start), 500)
		// In a won game a fall can be the winning move (a mortgage's down payment).
		if fwd < -o.maxDrop && !won {
			drop("net_worth_fall")
			continue
		}
		w := math.Min(math.Max(1+fwd, 0.5), 2)
		switch {
		case kind == "decision":
			w *= o.decisionMult
		case repeat:
			w *= o.repeatMult
		}
		// Director games are all wins by selection; their days weigh as routine.
		if source == "director" {
			w *= o.directorMult
		} else if won {
			w *= o.winMult
		}
		split := "train"
		if val || blockHash(dir, d.n/o.block)%uint64(o.valEvery) == 0 {
			split = "val"
		}
		gs.Kept++
		out = append(out, Example{
			Messages: []Message{{"system", system}, {"user", user}, {"assistant", target}},
			Weight:   round3(w),
			Meta: Meta{Source: strings.TrimSuffix(source, "-gamenotes"), Game: dir, Day: d.n, Difficulty: d.snap.Difficulty, Kind: kind, Repeat: repeat,
				NetWorth: start, Forward: round3(fwd), Ending: gs.Ending, Split: split},
		})
	}
	return out, gs, nil
}

func readDays(dir string) ([]playDay, error) {
	ents, err := filepath.Glob(filepath.Join(dir, "day-*"))
	if err != nil {
		return nil, err
	}
	slices.Sort(ents)
	var days []playDay
	for _, e := range ents {
		var d playDay
		if _, err := fmt.Sscanf(filepath.Base(e), "day-%d", &d.n); err != nil {
			continue
		}
		if err := readJSON(filepath.Join(e, "snapshot.json"), &d.snap); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		if err := readJSON(filepath.Join(e, "directives.json"), &d.directives); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		var r dayResult
		if err := readJSON(filepath.Join(e, "result.json"), &r); err == nil {
			d.result = &r
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if b, err := os.ReadFile(filepath.Join(e, "notes.md")); err == nil {
			d.notes = strings.TrimSpace(string(b))
		}
		if b, err := os.ReadFile(filepath.Join(e, "why.txt")); err == nil {
			d.why, d.hasWhy = oneLine(string(b)), true
		}
		days = append(days, d)
	}
	// day-1000 sorts before day-101 by name.
	slices.SortFunc(days, func(a, b playDay) int { return a.n - b.n })
	return days, nil
}

// classify says whether a day was fresh reasoning ("decision") or standing-order policy
// output ("routine"), and gives its WHY line. The standing-order scripts head each note
// with the policy in force ("standing orders: ...", "mortgage plan: ...", or a
// "Decision: ..." line), repeated day after day with that day's numbers after " | ". The
// first day a policy appears is the day it was decided; its repeats are routine, as is a
// note copied unchanged from an earlier day (its day references are stale). A one-off
// note is fresh reasoning.
func classify(d playDay, heads map[string]int, seen map[string]bool, limit int) (kind, why string) {
	n := oneLine(d.notes)
	switch {
	case n == "":
		return "routine", "standing orders unchanged."
	case seen[n]:
		return "routine", "standing orders unchanged."
	}
	seen[n] = true
	k := policyKey(d.notes)
	policy := heads[k] >= routineRepeats || strings.Contains(strings.ToLower(k), "standing orders")
	if policy && seen["\x00"+k] {
		return "routine", clip(n, limit)
	}
	seen["\x00"+k] = true
	return "decision", clip(n, limit)
}

const routineRepeats = 3

// policyKey is the policy a note states without the day's numbers: its first line up to
// " | ", or its "Decision:" line up to "; ".
func policyKey(notes string) string {
	for _, l := range strings.Split(notes, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(l), "Decision:"); ok {
			k, _, _ := strings.Cut(strings.TrimSpace(rest), "; ")
			return k
		}
	}
	k, _, _ := strings.Cut(firstLine(notes), " | ")
	return strings.TrimSpace(k)
}

func firstLine(s string) string {
	h, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(h)
}

// targetText is the assistant answer: the WHY line, then the directives as one compact
// JSON object. "day" is the executor's bookkeeping, not a decision.
func targetText(why string, dir map[string]any, gym bool) string {
	d := contractOnly(dir, gym)
	b, _ := json.Marshal(d)
	return "WHY: " + why + "\n" + string(b)
}

var contractKeys = []string{"cash_floor", "buy", "sell", "bargain", "window", "services", "upgrades_buy", "permit", "production", "augs", "scavenge"}

// contractOnly drops "day" and, without gym, every key and sub-key outside the protocol
// contract (internal/protocol.Directives).
func contractOnly(d map[string]any, gym bool) map[string]any {
	out := map[string]any{}
	for k, v := range d {
		if k == "day" {
			continue
		}
		if gym {
			out[k] = v
			continue
		}
		if !slices.Contains(contractKeys, k) {
			continue
		}
		if m, ok := v.(map[string]any); ok && (k == "buy" || k == "sell") {
			allowed := map[string][]string{
				"buy":  {"wholesale_max_pct", "single_max_pct", "max_spend", "prefer_types", "avoid_types", "hot_stock_cap"},
				"sell": {"min_pct", "sell_down_to_cash", "hold_uids"},
			}[k]
			mm := map[string]any{}
			for kk, vv := range m {
				if slices.Contains(allowed, kk) {
					mm[kk] = vv
				}
			}
			if len(mm) == 0 {
				continue
			}
			v = mm
		}
		out[k] = v
	}
	return out
}

// ordersKey is the directives without the numbers the standing-order script recomputes
// every day, so a repeat is a day whose policy did not change.
func ordersKey(d map[string]any) string {
	c := contractOnly(d, true)
	if b, ok := c["buy"].(map[string]any); ok {
		b = maps(b)
		delete(b, "max_spend")
		c["buy"] = b
	}
	delete(c, "cash_floor")
	j, _ := json.Marshal(c)
	return string(j)
}

func sameOrders(d map[string]any, prev string) bool { return prev != "" && ordersKey(d) == prev }

func maps(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func planInput(d playDay, prevWhy string, prevDay int) strategy.PlanInput {
	in := strategy.PlanInput{Day: d.n, Snapshot: d.snap.Snapshot}
	if y := d.snap.Yesterday; y != nil {
		yp := strategy.DayPlan{Day: y.Day, Facts: resultFacts(y)}
		if prevDay == y.Day && prevWhy != "" {
			yp.Plan = "WHY: " + prevWhy
		}
		in.Yesterday = &yp
	}
	return in
}

// resultFacts renders a step result as the strategist's facts line does a day.
func resultFacts(r *dayResult) string {
	parts := []string{
		fmt.Sprintf("cash %d -> %d (%+d)", r.CashStart, r.CashEnd, r.CashEnd-r.CashStart),
		fmt.Sprintf("net worth %d -> %d", r.NetWorthStart, r.NetWorthEnd),
	}
	if r.Rent != nil {
		parts = append(parts, fmt.Sprintf("rent %d due in %d day(s)", r.Rent.Due, r.Rent.DaysLeft))
	}
	parts = append(parts, fmt.Sprintf("%d sale(s) %+d, %d purchase(s) %+d", r.SoldItems, r.SoldValue, r.BoughtItems, -r.Spent))
	parts = append(parts, fmt.Sprintf("crime %d, evidence %d", r.Crime, r.Evidence))
	return strings.Join(parts, "; ") + "."
}

// rolloutRow is one rollout-search decision point: the morning snapshot (the step-mode
// snapshot.json, or the bare day_snapshot) and the best-scoring candidate directives.
type rolloutRow struct {
	Game      string          `json:"game"`
	Day       int             `json:"day"`
	Snapshot  json.RawMessage `json:"snapshot"`
	Best      map[string]any  `json:"best"`
	Directive map[string]any  `json:"directives"`
	Rationale string          `json:"rationale"`
	BestScore *float64        `json:"best_score"`
	BaseScore *float64        `json:"base_score"`
}

func rolloutExamples(path string, o options, st *Stats) ([]Example, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Example
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for line := 1; sc.Scan(); line++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var r rolloutRow
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		st.Rollouts++
		best := r.Best
		if best == nil {
			best = r.Directive
		}
		if best == nil || len(r.Snapshot) == 0 {
			st.Dropped["rollout_incomplete"]++
			continue
		}
		margin := 0.0
		if r.BestScore != nil && r.BaseScore != nil {
			margin = *r.BestScore - *r.BaseScore
			if margin <= 0 {
				st.Dropped["rollout_no_gain"]++
				continue
			}
		}
		var step stepSnapshot
		if json.Unmarshal(r.Snapshot, &step) != nil || len(step.Snapshot) == 0 {
			step = stepSnapshot{Day: r.Day, Snapshot: r.Snapshot}
		}
		day := nonZero(r.Day, step.Day)
		d := playDay{n: day, snap: step}
		system, user, err := strategy.DistillPrompt(planInput(d, "", 0), o.gym)
		if err != nil {
			st.Dropped["prompt_error"]++
			continue
		}
		why := clip(oneLine(nonEmpty(r.Rationale, "the rollout search's best line from this morning.")), o.rationaleMax)
		game := nonEmpty(r.Game, path)
		split := "train"
		if matchesAny(game, o.valGames) || blockHash(game, day/o.block)%uint64(o.valEvery) == 0 {
			split = "val"
		}
		w := 1.0
		if r.BestScore != nil && r.BaseScore != nil {
			w = math.Min(1+margin/math.Max(math.Abs(*r.BaseScore), 1), 2)
		}
		out = append(out, Example{
			Messages: []Message{{"system", system}, {"user", user}, {"assistant", targetText(why, best, o.gym)}},
			Weight:   round3(w),
			Meta:     Meta{Source: "rollout", Game: game, Day: day, Difficulty: step.Difficulty, Kind: "rollout", Split: split},
		})
	}
	return out, sc.Err()
}

func (st *Stats) print(w io.Writer) error {
	fmt.Fprintln(w, "games:")
	for _, g := range st.Games {
		fmt.Fprintf(w, "  %-45s days %3d  ending %-9s day %3d  nw %5d  decision days %2d  kept %3d\n",
			g.Path, g.Days, g.Ending, g.LastDay, g.NetWorth, g.Decisions, g.Kept)
	}
	fmt.Fprintf(w, "rollout rows: %d\n", st.Rollouts)
	fmt.Fprintf(w, "kept: %v\n", sortedMap(st.Kept))
	fmt.Fprintf(w, "dropped: %v\n", sortedMap(st.Dropped))
	fmt.Fprintf(w, "weight: train %.1f, val %.1f\n", st.Weight["train"], st.Weight["val"])
	fmt.Fprintf(w, "prompt tokens (approx): %v\ntarget tokens (approx): %v\n", st.PromptTok, st.TargetTok)
	return nil
}

func sortedMap[V any](m map[string]V) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", k, m[k]))
	}
	return strings.Join(parts, " ")
}

func writeJSONL(path string, rows []Example) error {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func blockHash(game string, block int) uint64 {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s#%d", filepath.Base(game), block)))
	return binary.BigEndian.Uint64(h[:8])
}

func approxTokens(s string) int { return int(float64(len(s))/3.5 + 0.5) }

func spread(v []int) map[string]int {
	if len(v) == 0 {
		return nil
	}
	s := slices.Clone(v)
	slices.Sort(s)
	q := func(p float64) int { return s[int(p*float64(len(s)-1))] }
	return map[string]int{"min": s[0], "p50": q(0.5), "p95": q(0.95), "max": s[len(s)-1]}
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := strings.LastIndex(s[:n], " ")
	if cut < n/2 {
		cut = n
	}
	return strings.TrimRight(s[:cut], " ,;") + " …"
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func matchesAny(s string, subs []string) bool {
	return slices.ContainsFunc(subs, func(sub string) bool { return strings.Contains(s, sub) })
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func nonZero(a, b int) int {
	if a != 0 {
		return a
	}
	return b
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }
