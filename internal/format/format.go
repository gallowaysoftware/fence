// Package format reads the stream's formats (docs/SHOW.md, "Formats"): a named setup of a
// difficulty, an optional route, handicaps the executor enforces and an optional buyout deadline,
// with a card for the panel, a pitch for the narrator and the lab's odds.
package format

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/route"
)

// DefaultDir is where format names resolve, relative to the repository root.
const DefaultDir = "sim/lab/scripts/directives/formats"

// DefaultRoute is the route a format without one is measured and played on.
const DefaultRoute = "sprinter"

var nameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Format is one formats/<name>.json.
type Format struct {
	Name       string `json:"-"`
	Title      string `json:"title"`
	Difficulty string `json:"difficulty"`
	// Route is a route name; empty lets the run pick its own (the lab plays DefaultRoute).
	Route     string   `json:"route,omitempty"`
	Handicaps []string `json:"handicaps,omitempty"`
	// DeadlineDay, when set, counts only a buyout on or before that day as a win.
	DeadlineDay int `json:"deadline_day,omitempty"`
	// Card is the panel's line under the title; Pitch the narrator's one-line premise.
	Card  string `json:"card"`
	Pitch string `json:"pitch"`
	Lab   Lab    `json:"lab"`
}

// Lab is the executor's measured record on the format, without a strategist.
type Lab struct {
	Route        string         `json:"route,omitempty"`
	Seeds        string         `json:"seeds,omitempty"`
	Runs         int            `json:"runs"`
	Wins         int            `json:"wins"`
	MedianEndDay int            `json:"median_end_day,omitempty"`
	MedianWinDay int            `json:"median_win_day,omitempty"`
	Endings      map[string]int `json:"endings,omitempty"`
	// Note says where the numbers come from when they are not one measured block.
	Note string `json:"note,omitempty"`
}

// Handicap is a rule the run declares up front: a directives overlay and the keys the executor
// keeps at the overlay's value whatever the day's plan says (the gym's `locked` lever).
type Handicap struct {
	ID    string
	Label string
	// Rule is how the strategist is told it.
	Rule    string
	Overlay map[string]any
	Locked  []string
}

// Handicaps are the ones a format may declare.
var Handicaps = []Handicap{
	{ID: "no_board", Label: "No Appraisal Board", Rule: "the shop never joins the Appraisal Board (appraisal: off)",
		Overlay: map[string]any{"appraisal": "off"}, Locked: []string{"appraisal"}},
	{ID: "no_mortgage", Label: "No mortgage", Rule: "the loan shark's offer is declined for good: the only win is the cash buyout (mortgage: decline)",
		Overlay: map[string]any{"mortgage": "decline"}, Locked: []string{"mortgage", "loan"}},
	{ID: "no_haggle", Label: "No haggling", Rule: "no discounts demanded, no markups or discounts at the counter: every price as it stands (bargain: never)",
		Overlay: map[string]any{"bargain": "never", "bargain_buy": "never"}, Locked: []string{"bargain", "bargain_buy"}},
	{ID: "clean_shop", Label: "Nothing stolen", Rule: "any lot holding a stolen item is refused (buy.hot_stock_cap: 0)",
		Overlay: map[string]any{"buy": map[string]any{"hot_stock_cap": 0}}, Locked: []string{"buy.hot_stock_cap"}},
	{ID: "no_insurance", Label: "No insurance", Rule: "never insured: janitorial is the only service bought (services: [JANITORIAL_SERVICE])",
		Overlay: map[string]any{"services": []any{"JANITORIAL_SERVICE"}}, Locked: []string{"services"}},
}

// HandicapByID finds a handicap.
func HandicapByID(id string) (Handicap, bool) {
	i := slices.IndexFunc(Handicaps, func(h Handicap) bool { return h.ID == id })
	if i < 0 {
		return Handicap{}, false
	}
	return Handicaps[i], true
}

// Load reads a format by name from dir (DefaultDir when empty).
func Load(dir, name string) (Format, error) {
	if !nameRE.MatchString(name) {
		return Format{}, fmt.Errorf("format %q: not a name (lowercase letters, digits, - and _)", name)
	}
	if dir == "" {
		dir = DefaultDir
	}
	b, err := os.ReadFile(filepath.Join(dir, name+".json"))
	if err != nil {
		return Format{}, fmt.Errorf("format %q: %w", name, err)
	}
	return Parse(name, b)
}

// List reads every format in dir, by name.
func List(dir string) ([]Format, error) {
	if dir == "" {
		dir = DefaultDir
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Format
	for _, p := range paths {
		f, err := Load(dir, strings.TrimSuffix(filepath.Base(p), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, nil
}

// Parse reads a format file's bytes.
func Parse(name string, b []byte) (Format, error) {
	f := Format{Name: name}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Format{}, fmt.Errorf("format %s: %w", name, err)
	}
	var errs []error
	if f.Title == "" || f.Card == "" || f.Pitch == "" {
		errs = append(errs, errors.New("needs title, card and pitch"))
	}
	if f.Difficulty != "hard" && f.Difficulty != "challenge" {
		errs = append(errs, fmt.Errorf("difficulty %q: want hard or challenge", f.Difficulty))
	}
	if f.Route != "" && !nameRE.MatchString(f.Route) {
		errs = append(errs, fmt.Errorf("route %q is not a name", f.Route))
	}
	for _, h := range f.Handicaps {
		if _, ok := HandicapByID(h); !ok {
			errs = append(errs, fmt.Errorf("unknown handicap %q", h))
		}
	}
	if f.DeadlineDay < 0 || f.Lab.Wins > f.Lab.Runs || f.Lab.Runs < 0 {
		errs = append(errs, errors.New("negative deadline or lab wins above runs"))
	}
	// The pitch rides in every narrator prompt, where a number would be one the fact check rejects.
	if strings.ContainsAny(f.Pitch, "0123456789") {
		errs = append(errs, errors.New("the pitch holds a digit; the narrator would quote it"))
	}
	if len(f.Card) > 160 || len(f.Pitch) > 200 {
		errs = append(errs, errors.New("card over 160 bytes or pitch over 200"))
	}
	if err := errors.Join(errs...); err != nil {
		return Format{}, fmt.Errorf("format %s: %w", name, err)
	}
	return f, nil
}

// RouteFile is the route file the format plays: its own route, else routeRef (a name or a path),
// else DefaultRoute; a challenge format's names resolve in the routes' challenge directory.
func (f Format) RouteFile(routeRef string) (string, error) {
	if f.Route != "" && routeRef != "" && strings.TrimSuffix(filepath.Base(routeRef), ".json") != f.Route {
		return "", fmt.Errorf("format %s plays the %s route, not %s", f.Name, f.Route, routeRef)
	}
	ref := f.Route
	if ref == "" {
		ref = routeRef
	}
	if ref == "" {
		ref = DefaultRoute
	}
	dir := route.DefaultDir
	if f.Difficulty == "challenge" {
		dir = filepath.Join(dir, "challenge")
	}
	return route.Resolve(dir, ref)
}

// Apply is a route file's bytes with the format's handicaps laid over its directives, their keys
// locked and the format named in "_format"; "_route" is kept, so it still reads as that route.
func (f Format) Apply(routeFile []byte) ([]byte, error) {
	var d map[string]any
	if err := json.Unmarshal(routeFile, &d); err != nil {
		return nil, err
	}
	var locked []any
	if l, ok := d["locked"].([]any); ok {
		locked = l
	}
	for _, id := range f.Handicaps {
		h, _ := HandicapByID(id)
		merge(d, h.Overlay)
		for _, k := range h.Locked {
			if !slices.Contains(locked, any(k)) {
				locked = append(locked, k)
			}
		}
	}
	if len(locked) > 0 {
		d["locked"] = locked
	}
	d["_format"] = f.Name
	return json.MarshalIndent(d, "", "  ")
}

// Materialize writes the format's directives for routeRef (see RouteFile) to dir/<route>.json,
// the file a gym's -directives, a bridge's -route and the mod's autopilot_directives take.
func (f Format) Materialize(routeRef, dir string) (string, error) {
	src, err := f.RouteFile(routeRef)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf("format %s: %w", f.Name, err)
	}
	out, err := f.Apply(b)
	if err != nil {
		return "", fmt.Errorf("format %s: %s: %w", f.Name, src, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, filepath.Base(src))
	return path, os.WriteFile(path, append(out, '\n'), 0o644)
}

func merge(dst, src map[string]any) {
	for k, v := range src {
		if sv, ok := v.(map[string]any); ok {
			if dv, ok := dst[k].(map[string]any); ok {
				merge(dv, sv)
				continue
			}
			cp := map[string]any{}
			merge(cp, sv)
			dst[k] = cp
			continue
		}
		dst[k] = v
	}
}

// HandicapLabels are the format's handicaps as the card shows them, with the deadline last.
func (f Format) HandicapLabels() []string {
	var out []string
	for _, id := range f.Handicaps {
		h, _ := HandicapByID(id)
		out = append(out, h.Label)
	}
	if f.DeadlineDay > 0 {
		out = append(out, fmt.Sprintf("Bought by day %d", f.DeadlineDay))
	}
	return out
}

// StrategistText is the format as the strategist is told it, after the route.
func (f Format) StrategistText() string {
	s := f.RulesText()
	if f.Lab.Runs > 0 {
		s += fmt.Sprintf("\nThe executor alone wins this %s.", f.Lab.Odds())
	}
	return s
}

// RulesText is the format's premise, handicaps and deadline, without the lab's record.
func (f Format) RulesText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "THIS RUN'S FORMAT: %s (%s). %s\n", f.Title, f.Difficulty, f.Pitch)
	if len(f.Handicaps) > 0 {
		b.WriteString("Handicaps, declared up front and enforced by the executor (a change to these keys is ignored):\n")
		for _, id := range f.Handicaps {
			h, _ := HandicapByID(id)
			fmt.Fprintf(&b, "- %s: %s\n", h.Label, h.Rule)
		}
	}
	if f.DeadlineDay > 0 {
		fmt.Fprintf(&b, "Deadline: the shop must be bought by day %d; a later buyout counts as a loss.\n", f.DeadlineDay)
	}
	return strings.TrimSpace(b.String())
}

// Won is whether a run that ended on day with ending counts as a win of the format.
func (f Format) Won(ending string, day int) bool {
	return strings.HasPrefix(ending, "buyout") && (f.DeadlineDay == 0 || day <= f.DeadlineDay)
}
