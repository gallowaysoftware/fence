package show

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The ghost band: each hard route's net worth, morning by morning, in 60 gym runs of its policy
// (sim/lab/scripts/ghost_bands.py), so a viewer can see a run ahead of or behind the book like a
// race against a record. The book is our own measurement, not game state: any run may be shown it.

//go:embed data/ghost_bands.json
var ghostBandsJSON []byte

// Book is a route's band. P10…P90 are percentiles of net worth on the morning of day i+1.
type Book struct {
	Route string `json:"route"`
	// Fallback is set when the run names no route and is shown the reference shop's book.
	Fallback     bool    `json:"fallback,omitempty"`
	Runs         int     `json:"runs"`
	Wins         int     `json:"wins"`
	MedianWinDay int     `json:"median_win_day"`
	P10          []int64 `json:"p10"`
	P25          []int64 `json:"-"`
	P50          []int64 `json:"p50"`
	P75          []int64 `json:"-"`
	P90          []int64 `json:"p90"`
}

// fallbackRoute is the book shown to a hard apprentice run without a route: the reference shop
// that every route changes one thing of (docs/research/ROUTES.md).
const fallbackRoute = "appraiser"

var (
	booksOnce sync.Once
	books     map[string]*Book
)

func loadBooks() map[string]*Book {
	booksOnce.Do(func() {
		var f struct {
			Routes map[string]struct {
				Runs, Wins    int
				MedianWinDay  int `json:"median_win_day"`
				P10, P25, P50 []int64
				P75, P90      []int64
			} `json:"routes"`
		}
		if err := json.Unmarshal(ghostBandsJSON, &f); err != nil {
			panic("show: data/ghost_bands.json: " + err.Error())
		}
		books = map[string]*Book{}
		for name, r := range f.Routes {
			books[name] = &Book{Route: name, Runs: r.Runs, Wins: r.Wins, MedianWinDay: r.MedianWinDay, P10: r.P10, P25: r.P25, P50: r.P50, P75: r.P75, P90: r.P90}
		}
	})
	return books
}

// BookFor is the book a run is raced against: its route's (every route is a hard one) unless the
// run is known to be on another difficulty; with no route, the reference shop's for a run known to
// be hard from the apprentice start; else none.
func BookFor(route, difficulty, start string) *Book {
	if difficulty != "" && difficulty != "hard" {
		return nil
	}
	bs := loadBooks()
	if route != "" {
		return bs[route]
	}
	if difficulty != "hard" || (start != "" && start != "apprentice") {
		return nil
	}
	if b := bs[fallbackRoute]; b != nil {
		c := *b
		c.Fallback = true
		return &c
	}
	return nil
}

// at is the band on the morning of day; past the book's last day it holds the last one (by
// then most lab runs have bought the shop and keep their last net worth).
func (b *Book) at(day int) (p10, p25, p50, p75, p90 int64) {
	i := min(day, len(b.P50)) - 1
	return b.P10[i], b.P25[i], b.P50[i], b.P75[i], b.P90[i]
}

// The run against its book.
const (
	PaceAhead  = "ahead"
	PaceOnPace = "on pace"
	PaceBehind = "behind"

	ZoneAbove  = "above"
	ZoneInside = "inside"
	ZoneBelow  = "below"
)

// Ghost is a morning's standing against the book: Status is ahead, on pace or behind; Zone where
// net worth sits against the 10–90 % band. Path is the run's mornings so far, so a panel that
// joins late (or a replay window) can draw the whole line.
type Ghost struct {
	Route    string     `json:"route"`
	Fallback bool       `json:"fallback,omitempty"`
	Status   string     `json:"status"`
	Zone     string     `json:"zone"`
	NetWorth int64      `json:"net_worth"`
	P10      int64      `json:"p10"`
	P50      int64      `json:"p50"`
	P90      int64      `json:"p90"`
	Path     [][2]int64 `json:"path"`
}

// pace reads net worth against the band. Ahead is past the 75th percentile and stays ahead until
// it falls back under the median; behind likewise under the 25th: a run near a quartile would
// otherwise flip every morning.
func pace(prev string, nw, p25, p50, p75 int64) string {
	switch {
	case nw > p75 || (prev == PaceAhead && nw >= p50):
		return PaceAhead
	case nw < p25 || (prev == PaceBehind && nw <= p50):
		return PaceBehind
	}
	return PaceOnPace
}

func zone(nw, p10, p90 int64) string {
	switch {
	case nw > p90:
		return ZoneAbove
	case nw < p10:
		return ZoneBelow
	}
	return ZoneInside
}

// ghostRun is the builder's race against the book: this morning's standing, yesterday's, and the
// last one told (a beat, or the first morning).
type ghostRun struct {
	now, prev, told standing
	path            [][2]int64
}

type standing struct{ status, zone string }

// ghostMorning reads the morning's net worth against the run's book: a ticker event the panel
// draws, or a beat when the run has changed pace or crossed the band's edge since the last one told
// and held it a second morning (the rehearsal's run sat on the 90th percentile and crossed it seven
// times in thirty days). Day 1 is the same for every run, and a bought shop has nothing left to race.
func (b *Builder) ghostMorning(day int) []Event {
	bk := b.Player.Book
	g := &b.ghost
	if bk == nil || len(bk.P50) == 0 || day < 2 || b.clock.Owned || b.clock.NetWorth <= 0 {
		return nil
	}
	if n := len(g.path); n > 0 && g.path[n-1][0] >= int64(day) {
		return nil
	}
	nw := b.clock.NetWorth
	p10, p25, p50, p75, p90 := bk.at(day)
	g.path = append(g.path, [2]int64{int64(day), nw})
	g.prev, g.now = g.now, standing{pace(g.now.status, nw, p25, p50, p75), zone(nw, p10, p90)}
	gh := &Ghost{Route: bk.Route, Fallback: bk.Fallback, Status: g.now.status, Zone: g.now.zone, NetWorth: nw, P10: p10, P50: p50, P90: p90,
		Path: append([][2]int64(nil), g.path...)}
	ev := Event{Kind: KindGhost, Level: LevelTicker, Day: day, Clock: b.Clock(), Ghost: gh}
	book := fmt.Sprintf("the %s book", strings.ReplaceAll(bk.Route, "_", " "))
	was := g.told
	switch {
	case was.status == "":
		g.told = g.now
		fallthrough
	case g.now == was || g.now != g.prev:
		ev.Text = fmt.Sprintf("Day %d against %s: %s, net worth %s, median %s.", day, book, g.now.status, num(nw), num(p50))
		return []Event{ev}
	case g.now.zone == ZoneAbove && was.zone != ZoneAbove:
		ev.Text = fmt.Sprintf("Day %d: net worth %s, above %s's band: richer than 9 in 10 of its lab runs by this morning (90th percentile %s, median %s).", day, num(nw), book, num(p90), num(p50))
	case g.now.zone == ZoneBelow && was.zone != ZoneBelow:
		ev.Text = fmt.Sprintf("Day %d: net worth %s, below %s's band: poorer than 9 in 10 of its lab runs by this morning (10th percentile %s, median %s).", day, num(nw), book, num(p10), num(p50))
	case g.now.zone != was.zone:
		ev.Text = fmt.Sprintf("Day %d: net worth %s, back inside %s's band, %s (median %s).", day, num(nw), book, g.now.status, num(p50))
	default:
		ev.Text = fmt.Sprintf("Day %d: net worth %s, now %s (median %s; was %s).", day, num(nw), against(g.now.status, book), num(p50), was.status)
	}
	g.told = g.now
	ev.Level, ev.Beat = LevelMoment, BeatPace
	return []Event{ev}
}

func against(status, book string) string {
	switch status {
	case PaceAhead:
		return "ahead of " + book
	case PaceBehind:
		return "behind " + book
	}
	return "on pace with " + book
}

// PrevRun is a second ghost: an earlier run's mornings (on the same seed, for a fair race), drawn
// beside the run's own line.
type PrevRun struct {
	Label string     `json:"label"`
	Path  [][2]int64 `json:"path"`
}

// LoadPrevRun reads an earlier run's mornings from a decision log, a directory holding one (a league
// match or a rehearsal: its newest log/*.jsonl), or a gym summary (gym.summary.json).
func LoadPrevRun(path string) (*PrevRun, error) {
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		log := NewestLog(filepath.Join(path, "log"))
		if log == "" {
			return nil, fmt.Errorf("%s: no decision log in log/", path)
		}
		path = log
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p [][2]int64
	label := "previous run"
	var sum struct {
		Seed     int64   `json:"seed"`
		NetWorth []int64 `json:"net_worth_by_day"`
		Days     []struct {
			NetWorthStart int64 `json:"net_worth_start"`
		} `json:"player_days"`
	}
	if strings.HasSuffix(path, ".json") && json.Unmarshal(b, &sum) == nil && len(sum.Days) > 0 {
		// The gym records net worth after each night: day d's entry is day d+1's morning.
		for i, nw := range sum.NetWorth {
			p = append(p, [2]int64{int64(i + 2), nw})
		}
		if sum.Seed != 0 {
			label = fmt.Sprintf("previous run, seed %d", sum.Seed)
		}
	} else {
		p = morningsOf(b)
	}
	if len(p) == 0 {
		return nil, fmt.Errorf("%s: no mornings", path)
	}
	return &PrevRun{Label: label, Path: p}, nil
}

// morningsOf reads the first run's morning net worth, day by day, from a decision log.
func morningsOf(log []byte) [][2]int64 {
	var out [][2]int64
	runs := 0
	for line := range bytes.SplitSeq(log, []byte("\n")) {
		var l struct {
			Event *struct {
				Kind string `json:"kind"`
				Data struct {
					Phase string `json:"phase"`
					Store struct {
						Day      int64  `json:"day"`
						NetWorth *int64 `json:"net_worth"`
					} `json:"store"`
				} `json:"data"`
			} `json:"event"`
		}
		if json.Unmarshal(line, &l) != nil || l.Event == nil {
			continue
		}
		e := l.Event
		if e.Kind == "run_start" {
			if runs++; runs > 1 {
				break
			}
		}
		if e.Kind == "day_snapshot" && e.Data.Phase == "morning" && e.Data.Store.NetWorth != nil {
			if n := len(out); n == 0 || out[n-1][0] < e.Data.Store.Day {
				out = append(out, [2]int64{e.Data.Store.Day, *e.Data.Store.NetWorth})
			}
		}
	}
	return out
}
