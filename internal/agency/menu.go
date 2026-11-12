// Package agency gives the strategist the run-shaping choices (docs/STREAM.md, "Model agency"):
// the day-0 route, picked from the menu of tested hard routes (docs/research/ROUTES.md) or
// composed from their levers, and the inventor's new routes for the gym to screen.
package agency

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

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
}

// Menu is every hard route, in name order.
type Menu struct {
	Entries []Entry
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

// For is the menu a run may pick from: every route, or only those that play start when the
// match fixes it ("" for none).
func (m *Menu) For(start string) *Menu {
	if start == "" {
		return m
	}
	out := &Menu{}
	for _, e := range m.Entries {
		if e.Start == start {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
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
		fmt.Fprintf(&b, "- %s (start %s, perks %s, persona %s): %s Lab: %s.\n", e.Name, e.Start, e.Perks, e.Persona, e.Brief, e.Lab.text())
	}
	return b.String()
}

// LeanText is the menu in one short line a route: what it changes over the appraiser and its record.
func (m *Menu) LeanText() string {
	ref, _ := m.Get(DefaultRoute)
	var b strings.Builder
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "- %s (%s): %s; won %s, median day %d\n", e.Name, e.Start, diffText(ref, e), e.Lab.Wins, e.Lab.MedianWinDay)
	}
	return b.String()
}
