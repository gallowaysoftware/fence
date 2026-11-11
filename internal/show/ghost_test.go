package show

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every route file has a book, and every book is a band: as long as its median, p10 ≤ p25 ≤ p50 ≤
// p75 ≤ p90 each morning.
func TestBooksCoverEveryRoute(t *testing.T) {
	files, _ := filepath.Glob("../../sim/lab/scripts/directives/routes/*.json")
	if len(files) == 0 {
		t.Fatal("no route files")
	}
	bs := loadBooks()
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".json")
		b := bs[name]
		if b == nil {
			t.Errorf("%s: no book in data/ghost_bands.json (sim/lab/scripts/ghost_bands.py)", name)
			continue
		}
		if b.Runs < 60 || b.Wins > b.Runs || b.MedianWinDay <= 0 {
			t.Errorf("%s: runs %d wins %d median win day %d", name, b.Runs, b.Wins, b.MedianWinDay)
		}
		n := len(b.P50)
		if n < 30 || len(b.P10) != n || len(b.P25) != n || len(b.P75) != n || len(b.P90) != n {
			t.Errorf("%s: band lengths %d %d %d %d %d", name, len(b.P10), len(b.P25), n, len(b.P75), len(b.P90))
			continue
		}
		for i := range n {
			if !(b.P10[i] <= b.P25[i] && b.P25[i] <= b.P50[i] && b.P50[i] <= b.P75[i] && b.P75[i] <= b.P90[i]) {
				t.Errorf("%s day %d: not a band", name, i+1)
				break
			}
		}
	}
	if len(bs) != len(files) {
		t.Errorf("%d books for %d route files", len(bs), len(files))
	}
}

func TestBookFor(t *testing.T) {
	for _, c := range []struct {
		route, difficulty, start, want string
		fallback                       bool
	}{
		{"smelter", "hard", "apprentice", "smelter", false},
		{"smelter", "", "", "smelter", false},
		{"smelter", "challenge", "apprentice", "", false},
		{"no_such_route", "hard", "apprentice", "", false},
		{"", "hard", "apprentice", "appraiser", true},
		{"", "hard", "", "appraiser", true},
		{"", "hard", "rock_bottom", "", false},
		{"", "normal", "apprentice", "", false},
		{"", "", "apprentice", "", false},
	} {
		b := BookFor(c.route, c.difficulty, c.start)
		got := ""
		if b != nil {
			got = b.Route
		}
		if got != c.want || (b != nil && b.Fallback != c.fallback) {
			t.Errorf("BookFor(%q, %q, %q) = %q (fallback %v), want %q (%v)", c.route, c.difficulty, c.start, got, b != nil && b.Fallback, c.want, c.fallback)
		}
	}
	if loadBooks()[fallbackRoute].Fallback {
		t.Error("the fallback copy leaked into the shared book")
	}
}

func TestPaceHysteresis(t *testing.T) {
	const p25, p50, p75 = 100, 200, 300
	for _, c := range []struct {
		prev string
		nw   int64
		want string
	}{
		{"", 301, PaceAhead},
		{"", 250, PaceOnPace},
		{"", 99, PaceBehind},
		{PaceAhead, 200, PaceAhead},
		{PaceAhead, 199, PaceOnPace},
		{PaceAhead, 50, PaceBehind},
		{PaceBehind, 200, PaceBehind},
		{PaceBehind, 201, PaceOnPace},
		{PaceOnPace, 299, PaceOnPace},
	} {
		if got := pace(c.prev, c.nw, p25, p50, p75); got != c.want {
			t.Errorf("pace(%q, %d) = %q, want %q", c.prev, c.nw, got, c.want)
		}
	}
}

// The morning publishes the standing against the book; a change of pace or a crossing of the band's
// edge is a beat with its numbers in the caption, and the fact check knows them.
func TestGhostMorning(t *testing.T) {
	bk := BookFor("appraiser", "hard", "apprentice")
	b := NewBuilder(Player{Route: "appraiser"})
	_, _, p50, _, p90 := bk.at(10)
	_, _, p50at9, _, _ := bk.at(9)
	_, _, _, _, p90at11 := bk.at(11)
	_, _, p50at12, _, _ := bk.at(12)
	_, _, p50at13, _, _ := bk.at(13)
	lines := []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r1", "difficulty": "hard", "start_type": "apprentice"}),
		snap("morning", 9, p50at9),
		snap("morning", 10, p90*2),
		snap("morning", 10, p90*2),
		snap("morning", 11, p90at11*2),
		snap("morning", 12, p50at12),
		snap("morning", 13, p50at13),
	}
	evs := feedAll(t, b, lines...)
	var ghosts []Event
	for _, e := range evs {
		if e.Kind == KindGhost {
			ghosts = append(ghosts, e)
		}
	}
	if len(ghosts) != 5 {
		t.Fatalf("ghost events: %d, want one a morning", len(ghosts))
	}
	var beats []int
	for _, g := range ghosts {
		if g.Beat == BeatPace {
			beats = append(beats, g.Day)
		}
	}
	// A crossing is told once it has held a second morning.
	if fmt.Sprint(beats) != "[11 13]" {
		t.Errorf("beats on days %v, want [11 13]", beats)
	}
	if g := ghosts[0]; g.Level != LevelTicker || g.Ghost.Status != PaceOnPace || g.Ghost.Zone != ZoneInside {
		t.Errorf("first morning: %+v", g)
	}
	if g := ghosts[1]; g.Level != LevelTicker || g.Ghost.Zone != ZoneAbove {
		t.Errorf("the first morning above: %+v", g)
	}
	above := ghosts[2]
	if above.Beat != BeatPace || above.Ghost.Zone != ZoneAbove || above.Ghost.Status != PaceAhead || !strings.Contains(above.Text, "above the appraiser book's band") {
		t.Errorf("above the band: %q %+v", above.Text, above.Ghost)
	}
	if n := len(above.Ghost.Path); n != 3 || above.Ghost.Path[1] != [2]int64{10, p90 * 2} || ghosts[1].Ghost.P50 != p50 {
		t.Errorf("path %v", above.Ghost.Path)
	}
	back := ghosts[4]
	if back.Beat != BeatPace || back.Ghost.Zone != ZoneInside || !strings.Contains(back.Text, "back inside") {
		t.Errorf("back inside: %q %+v", back.Text, back.Ghost)
	}
	f := FactsOf(above)
	if faults := CheckLine("Ahead of the book and miles clear.", f); len(faults) != 0 {
		t.Errorf("a true claim faulted: %v", faults)
	}
	if faults := CheckLine("I'm behind the book today.", f); len(faults) != 1 {
		t.Errorf("a false claim passed: %v", faults)
	}
	if faults := CheckLine("Net worth "+num(p90*2)+", the lab never saw it.", f); len(faults) != 0 {
		t.Errorf("the caption's number faulted: %v", faults)
	}
	if !strings.Contains(f.Prompt(), "ahead of it") {
		t.Errorf("facts prompt: %s", f.Prompt())
	}

	h := NewHub(nil, "")
	defer h.Close()
	for _, e := range evs {
		h.Publish(e)
	}
	s := h.State()
	if s.Ghost == nil || s.Ghost.Ghost.Zone != ZoneInside || s.Player == nil || s.Player.Book == nil || len(s.Player.Book.P90) == 0 {
		t.Errorf("hub state: ghost %+v player %+v", s.Ghost, s.Player)
	}
	for _, e := range s.Ticker {
		if e.Kind == KindGhost {
			t.Error("a ghost event in the thinking ticker")
		}
	}
}

func TestMoodCarriesPace(t *testing.T) {
	c := &Clock{Cash: 2000, NetWorth: 5000, RentDue: 300, RentDays: 3, BuyoutPrice: 28000}
	m := withPace(MoodOf(c, nil), PaceBehind)
	if !strings.Contains(m.Cue, "behind the book") || m.Name != "steady" {
		t.Errorf("mood %+v", m)
	}
	if strings.ContainsAny(m.Cue, "0123456789") {
		t.Errorf("a number in the mood: %q", m.Cue)
	}
	owned := withPace(MoodOf(&Clock{Owned: true}, nil), PaceAhead)
	if strings.Contains(owned.Cue, "book") {
		t.Errorf("a bought shop still races: %q", owned.Cue)
	}
}

// The ghost bands file stays small enough to embed and to read in a diff.
func TestGhostBandsSize(t *testing.T) {
	st, err := os.Stat("data/ghost_bands.json")
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() > 128<<10 {
		t.Errorf("data/ghost_bands.json is %d bytes", st.Size())
	}
}

func TestLoadPrevRun(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "log", "s-1.jsonl")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	for _, l := range []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "a"}),
		snap("run_start", 1, 829), snap("morning", 2, 900), snap("close", 2, 950), snap("morning", 3, 1000),
		ev("run_start", 1, map[string]any{"run_id": "b"}), snap("morning", 2, 5),
	} {
		b, _ := json.Marshal(l)
		log.Write(append(b, '\n'))
	}
	if err := os.WriteFile(logPath, []byte(log.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := LoadPrevRun(dir)
	if err != nil || fmt.Sprint(p.Path) != "[[2 900] [3 1000]]" {
		t.Errorf("from a log dir: %v %v", p, err)
	}
	sum := filepath.Join(dir, "gym.summary.json")
	_ = os.WriteFile(sum, []byte(`{"seed":7,"net_worth_by_day":[900,1000],"player_days":[{"net_worth_start":829}]}`), 0o644)
	p, err = LoadPrevRun(sum)
	if err != nil || fmt.Sprint(p.Path) != "[[2 900] [3 1000]]" || p.Label != "previous run, seed 7" {
		t.Errorf("from a gym summary: %v %v", p, err)
	}
}

// The narrator keeps the latest standing in its mood, and a pace beat gets its cue.
func TestNarratorHearsThePace(t *testing.T) {
	n := &Narrator{}
	n.defaults()
	events := make(chan Event, 2)
	clock := &Clock{Cash: 2000, NetWorth: 5000, RentDue: 300, RentDays: 3, BuyoutPrice: 28000}
	beat := Event{Seq: 2, Kind: KindGhost, Level: LevelMoment, Beat: BeatPace, Day: 30, Clock: clock,
		Text:  "Day 30: net worth 5,000, now behind the sprinter book (median 6,000; was on pace).",
		Ghost: &Ghost{Route: "sprinter", Status: PaceBehind, Zone: ZoneInside, NetWorth: 5000, P10: 4000, P50: 6000, P90: 8000}}
	events <- beat
	close(events)
	n.intake(t.Context(), events)
	sys := n.system(n.mood(Event{Kind: KindTrade, Clock: clock}))
	if !strings.Contains(sys, "behind the book") {
		t.Errorf("system prompt without the pace:\n%s", sys)
	}
	p := n.prompt(beat, FactsOf(beat), nil, "")
	if !strings.Contains(p, "the race against the book") || !strings.Contains(p, "behind it; net worth 5,000, the book's median 6,000") {
		t.Errorf("prompt:\n%s", p)
	}
	if Priority(beat) != Priority(Event{Kind: KindThought, Beat: BeatMilestone}) {
		t.Errorf("a pace beat ranks %d", Priority(beat))
	}
}
