// Package agency gives the strategist the run-shaping choices (docs/STREAM.md, "Model agency"):
// the day-0 route, picked from the menu of tested hard routes (docs/research/ROUTES.md) or
// composed from their levers, and the inventor's new routes for the gym to screen.
package agency

import (
	"cmp"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/route"
)

//go:embed lab.json
var labJSON []byte

// Lab is a route's measured record on hard, as the route docs report it.
type Lab struct {
	Wins         string `json:"wins"`
	MedianWinDay int    `json:"median_win_day"`
	Seeds        string `json:"seeds"`
	// Income is the money's split, Board / counter / production / Georgio in %.
	Income string `json:"income,omitempty"`
	Note   string `json:"note,omitempty"`
}

func (l Lab) text() string {
	if l.Wins == "" {
		return "not measured"
	}
	s := fmt.Sprintf("won %s on hard (seeds %s), median win day %d", l.Wins, l.Seeds, l.MedianWinDay)
	if l.Income != "" {
		s += ", money Board/counter/production/Georgio " + l.Income + " %"
	}
	if l.Note != "" {
		s += "; " + l.Note
	}
	return s
}

// LabStats are lab.json's routes.
func LabStats() (map[string]Lab, error) {
	var f struct {
		Routes map[string]Lab `json:"routes"`
	}
	if err := json.Unmarshal(labJSON, &f); err != nil {
		return nil, fmt.Errorf("agency/lab.json: %w", err)
	}
	return f.Routes, nil
}

// Entry is one route on the menu.
type Entry struct {
	route.Route
	Lab Lab
	// Odds are its odds of beating the run's deadline (Menu.Deadline); nil when unmeasured or none.
	Odds *Odds
}

// Menu is every hard route, in name order.
type Menu struct {
	Entries []Entry
	// Deadline is the run's deadline day, 0 for none; Seeds the lab seeds of the entries' Odds.
	Deadline int
	Seeds    string
	// excluded is the route Without dropped; ref the appraiser, kept for the lean diffs
	// should it be the one dropped.
	excluded string
	ref      *Entry
}

// LoadMenu reads the routes in dir (route.DefaultDir when empty; its challenge/ subdirectory
// is a different difficulty and stays out).
func LoadMenu(dir string) (*Menu, error) {
	if dir == "" {
		dir = route.DefaultDir
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no routes in %s", dir)
	}
	lab, err := LabStats()
	if err != nil {
		return nil, err
	}
	m := &Menu{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		r, err := route.Parse(p, b)
		if err != nil {
			return nil, err
		}
		m.Entries = append(m.Entries, Entry{Route: r, Lab: lab[r.Name]})
	}
	slices.SortFunc(m.Entries, func(a, b Entry) int { return strings.Compare(a.Name, b.Name) })
	return m, nil
}

// Get is the entry named name.
func (m *Menu) Get(name string) (Entry, bool) {
	for _, e := range m.Entries {
		if e.Name == name {
			return e, true
		}
	}
	return Entry{}, false
}

// WithDeadline is the menu with each route's odds of beating f's deadline, as the lab measured
// them under f's handicaps; m itself when f has no deadline.
func (m *Menu) WithDeadline(f format.Format) *Menu {
	if f.DeadlineDay == 0 {
		return m
	}
	out := *m
	out.Entries, out.Deadline, out.Seeds = slices.Clone(m.Entries), f.DeadlineDay, ""
	fo, ok := DeadlineOdds(f.Name)
	if !ok || fo.DeadlineDay != f.DeadlineDay {
		return &out
	}
	out.Seeds = fo.Seeds
	for i, e := range out.Entries {
		if o, ok := fo.Routes[e.Name]; ok && o.Runs > 0 {
			out.Entries[i].Odds = &o
		}
	}
	return &out
}

// For is the menu a run may pick from: every route, or only those that play start when the
// match fixes it ("" for none).
func (m *Menu) For(start string) *Menu {
	if start == "" {
		return m
	}
	out := *m
	out.Entries = nil
	for _, e := range m.Entries {
		if e.Start == start {
			out.Entries = append(out.Entries, e)
		}
	}
	return &out
}

// Without is the menu without the route named name, m itself when it is not on the menu or is
// the only route left.
func (m *Menu) Without(name string) *Menu {
	i := slices.IndexFunc(m.Entries, func(e Entry) bool { return e.Name == name })
	if i < 0 || len(m.Entries) == 1 {
		return m
	}
	out := *m
	out.Entries = slices.Delete(slices.Clone(m.Entries), i, i+1)
	out.excluded = name
	if ref, ok := m.reference(); ok {
		out.ref = &ref
	}
	return &out
}

// Excluded is the route Without dropped, "" for none.
func (m *Menu) Excluded() string { return m.excluded }

// reference is the appraiser, the route the lean menu describes the others against.
func (m *Menu) reference() (Entry, bool) {
	if m.ref != nil {
		return *m.ref, true
	}
	return m.Get(DefaultRoute)
}

// Starts and PerkSets are what the menu's routes play: a composed route stays within them.
func (m *Menu) Starts() []string { return m.distinct(func(e Entry) string { return e.Start }) }
func (m *Menu) PerkSets() []string {
	return m.distinct(func(e Entry) string { return sortedSet(e.Perks) })
}

func (m *Menu) distinct(f func(Entry) string) []string {
	var out []string
	for _, e := range m.Entries {
		if v := f(e); !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

func sortedSet(perks string) string {
	ids := strings.Split(perks, "+")
	slices.Sort(ids)
	return strings.Join(ids, "+")
}

// Text is the menu as the strategist reads it: one line per route.
func (m *Menu) Text() string {
	var b strings.Builder
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "- %s (start %s, perks %s, persona %s): %s Lab: %s.", e.Name, e.Start, e.Perks, e.Persona, e.Brief, e.Lab.text())
		if m.Deadline > 0 {
			b.WriteString(" " + m.deadlineText(e))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (m *Menu) deadlineText(e Entry) string {
	o := e.Odds
	if o == nil {
		return fmt.Sprintf("By day %d under this format: not measured.", m.Deadline)
	}
	s := fmt.Sprintf("By day %d under this format: bought in %d of %d runs, %d %% (95 %% range %d–%d %%)", m.Deadline, o.Wins, o.Runs, o.Pct(), o.Lo, o.Hi)
	if o.MedianWinDay > 0 {
		s += fmt.Sprintf(", median win day %d", o.MedianWinDay)
	}
	return s + "."
}

// byOdds is the entries, best odds of beating the deadline first; unmeasured routes last.
func (m *Menu) byOdds() []Entry {
	es := slices.Clone(m.Entries)
	slices.SortStableFunc(es, func(a, b Entry) int {
		return cmp.Compare(oddsKey(b), oddsKey(a))
	})
	return es
}

func oddsKey(e Entry) float64 {
	if e.Odds == nil {
		return -1
	}
	return float64(e.Odds.Wins) / float64(e.Odds.Runs)
}

// LeanText is the menu in one short line a route: what it changes over the appraiser and its record;
// with a deadline, best odds of beating it first.
func (m *Menu) LeanText() string {
	ref, _ := m.reference()
	es := m.Entries
	if m.Deadline > 0 {
		es = m.byOdds()
	}
	var b strings.Builder
	for _, e := range es {
		fmt.Fprintf(&b, "- %s (%s, %s): %s; won %s, median day %d", e.Name, e.Start, e.Persona, diffText(ref, e), e.Lab.Wins, e.Lab.MedianWinDay)
		if o := e.Odds; o != nil {
			fmt.Fprintf(&b, "; by day %d %d %% (%d–%d)", m.Deadline, o.Pct(), o.Lo, o.Hi)
		} else if m.Deadline > 0 {
			fmt.Fprintf(&b, "; by day %d not measured", m.Deadline)
		}
		b.WriteByte('\n')
	}
	return b.String()
}
