package policy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// The appraisal guides' value tables are game content and are not committed: they live in
// local/appraisal_guides.json (docs/re/APPRAISAL.md, "The value table"), the same file the gym
// reads (sim/gym/Appraisal.cs Value). Without it no appraisal value is computed.
const (
	AppraisalGuidesEnv  = "FENCE_APPRAISAL_GUIDES"
	appraisalGuidesFile = "local/appraisal_guides.json"
)

// guideTypes are the table keys (the gym's item types), with the mod's kind and the prompt's name.
var guideTypes = []struct{ key, kind, name string }{
	{"hand", kindHand, "Prosthetic hand"},
	{"ribwich", kindRibwich, "Ribwich"},
	{"plasma", kindPlasma, "Plasma cell"},
}

type guideTable struct {
	Base        int64       `json:"base"`
	Min         int64       `json:"min"`
	Invalid     int64       `json:"invalid"`
	InvalidWhen []guideCond `json:"invalid_when"`
	Add         []guideRule `json:"add"`
}

type guideRule struct {
	When  guideCond `json:"when"`
	Add   int64     `json:"add"`
	Group string    `json:"group"`
	// Why labels the rule in the rule check and the prompt; the gym ignores it.
	Why string `json:"why"`
}

// guideCond is a rule's `when`: every cue must match. Kept in file order for its description.
type guideCond []guideTerm

type guideTerm struct {
	Cue   string
	Match any // decoded JSON: string, float64, bool, []any, map[string]any
}

func (c *guideCond) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return errors.New("a rule condition must be an object")
	}
	*c = nil
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		var m any
		if err := dec.Decode(&m); err != nil {
			return err
		}
		*c = append(*c, guideTerm{Cue: t.(string), Match: m})
	}
	return nil
}

type appraisalTables map[string]*guideTable

func parseAppraisalGuides(raw []byte) (appraisalTables, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, err
	}
	t := appraisalTables{}
	for _, g := range guideTypes {
		b, ok := top[g.key]
		if !ok {
			continue
		}
		var tab guideTable
		if err := json.Unmarshal(b, &tab); err != nil {
			return nil, fmt.Errorf("%s: %w", g.key, err)
		}
		t[g.key] = &tab
	}
	if len(t) == 0 {
		return nil, errors.New("no hand, ribwich or plasma table")
	}
	return t, nil
}

// AppraisalGuidesPath is FENCE_APPRAISAL_GUIDES, else local/appraisal_guides.json in the
// repository (a git worktree's falls back to the main checkout's, as the gym's does).
func AppraisalGuidesPath(getenv func(string) string) string {
	if p := getenv(AppraisalGuidesEnv); p != "" {
		return p
	}
	return filepath.Join(mainCheckout(repoRoot()), filepath.FromSlash(appraisalGuidesFile))
}

// ReadAppraisalGuides returns the guides file checked to parse; nil, nil when it does not exist.
func ReadAppraisalGuides(path string) (json.RawMessage, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := parseAppraisalGuides(b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

// repoRoot walks up from the working directory, then the executable's, to the directory with
// go.mod and sim/ (Boot.FindRepo in the gym); else the working directory.
func repoRoot() string {
	wd, _ := os.Getwd()
	starts := []string{wd}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, d := range starts {
		for d != "" {
			if fileExists(filepath.Join(d, "go.mod")) && dirExists(filepath.Join(d, "sim")) {
				return d
			}
			up := filepath.Dir(d)
			if up == d {
				break
			}
			d = up
		}
	}
	return wd
}

// mainCheckout: a worktree's local/ is empty; its .git file reads "gitdir: <main>/.git/worktrees/<name>".
func mainCheckout(repo string) string {
	b, err := os.ReadFile(filepath.Join(repo, ".git"))
	if err != nil {
		return repo
	}
	line, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
	if at := strings.Index(line, "/.git/worktrees/"); ok && at > 0 {
		return line[:at]
	}
	return repo
}

func fileExists(p string) bool { st, err := os.Stat(p); return err == nil && !st.IsDir() }
func dirExists(p string) bool  { st, err := os.Stat(p); return err == nil && st.IsDir() }

var guides struct {
	sync.Mutex
	set    bool
	tables appraisalTables
	system string
}

// UseAppraisalGuides sets the value tables the rule check and the inspect prompt use (nil: none).
// Until it is called they are read from AppraisalGuidesPath on first use.
func UseAppraisalGuides(raw json.RawMessage) error {
	var t appraisalTables
	if raw != nil {
		var err error
		if t, err = parseAppraisalGuides(raw); err != nil {
			return err
		}
	}
	guides.Lock()
	defer guides.Unlock()
	guides.set, guides.tables, guides.system = true, t, buildInspectSystem(t)
	return nil
}

func currentGuides() (appraisalTables, string) {
	guides.Lock()
	defer guides.Unlock()
	if !guides.set {
		// A malformed file degrades like a missing one; the bridge logs which it was.
		raw, _ := ReadAppraisalGuides(AppraisalGuidesPath(os.Getenv))
		t, _ := parseAppraisalGuides(raw)
		guides.set, guides.tables, guides.system = true, t, buildInspectSystem(t)
	}
	return guides.tables, guides.system
}

// guideCues are an item's cues in the table's vocabulary. A cue in unknown could not be read;
// one in neither map is known to be absent (absent says why, for the rule check).
type guideCues struct {
	vals    map[string]any
	absent  map[string]string
	unknown map[string]bool
}

func newGuideCues() *guideCues {
	return &guideCues{vals: map[string]any{}, absent: map[string]string{}, unknown: map[string]bool{}}
}

// The mod's cue words (testdata/inspect_cues.json) in the table's vocabulary (the gym's
// Appraisal.Perceive). A word mapped to nil means the cue is absent.
var guideWords = map[string]map[string]struct {
	cue   string
	words map[string]any
}{
	kindHand: {
		"material":          {"material", map[string]any{"aluminium alloy": "steel", "carbon composite": "carbon", "gold plated": "gold"}},
		"interface_marking": {"interface", map[string]any{"EMG": "emg", "EMG+": "emg_plus", "Neural": "neural", "none": "none"}},
		"condition":         {"condition", map[string]any{"pristine": "intact", "worn": "wear", "damaged": "damage"}},
		"armor_plating":     {"armored", map[string]any{"fitted": true, "none": false}},
		"blade":             {"bladed", map[string]any{"fitted": true, "none": false}},
	},
	kindRibwich: {
		"meat":         {"meat", map[string]any{"single": "normal", "double": "double"}},
		"top_bun":      {"bun", map[string]any{"regular": "normal", "deluxe": "deluxe", "hidden under double meat": nil}},
		"pickles":      {"pickles", map[string]any{"regular": "normal", "extra": "extra", "hidden under double meat": nil}},
		"wrap":         {"wrap", map[string]any{"sealed": "sealed", "opened": "open"}},
		"davre_logo":   {"davre_logo", map[string]any{cueMatches: "reference", cueWrongLog: "off", cueMissing: "absent"}},
		"ribwich_logo": {"ribwich_logo", map[string]any{cuePresent: "reference", cueWrongLog: "off", "absent": "absent"}},
	},
	kindPlasma: {
		"colour":        {"color", map[string]any{"standard": "standard", "high quality": "high", "poor": "poor"}},
		"seal_brand":    {"seal", map[string]any{"LL": "ll", "Zhonghua": "zhonghua", "Republic": "republic", "Ignition Lab": "ignition_lab", "none": "none"}},
		"casing_wear":   {"wear", map[string]any{"none": "none", "worn": "wear", "heavily worn": "heavy"}},
		"crack":         {"crack", map[string]any{"none": false, "cracked": true}},
		"screw":         {"uncovered", map[string]any{"covered": false, "uncovered": true}},
		"reinforcement": {"reinforced", map[string]any{"no": false, "reinforced": true}},
	},
}

// readGuideCues translates the item's visible cues; it never sees hidden values.
func readGuideCues(c *inspectCtx, r *ruleCheck) *guideCues {
	g := newGuideCues()
	words := guideWords[c.Item.Kind]
	for _, vis := range slices.Sorted(maps.Keys(words)) {
		m := words[vis]
		w, ok := c.visual(r, vis)
		if !ok {
			g.unknown[m.cue] = true
			continue
		}
		v, known := m.words[w]
		switch {
		case !known:
			r.unknown(vis + " " + strconv.Quote(w))
			g.unknown[m.cue] = true
		case v == nil:
			g.absent[m.cue] = w
		default:
			g.vals[m.cue] = v
		}
	}
	if c.Item.Kind == kindRibwich {
		readRibwichStamps(c, r, g)
	}
	return g
}

// readRibwichStamps reads the printed date and facility; a missing stamp leaves them absent.
func readRibwichStamps(c *inspectCtx, r *ruleCheck, g *guideCues) {
	if s, ok := c.visual(r, "barcode_stamp"); !ok {
		g.unknown["month"], g.unknown["year"] = true, true
	} else if s == cueMissing {
		g.absent["month"], g.absent["year"] = "barcode stamp missing", "barcode stamp missing"
	} else if d, ok := c.text(r, "production_date"); !ok {
		g.unknown["month"], g.unknown["year"] = true, true
	} else if m, y, err := parseRibwichDate(d); err != nil {
		r.unknown("production date " + strconv.Quote(d))
		g.unknown["month"], g.unknown["year"] = true, true
	} else {
		g.vals["month"], g.vals["year"] = int64(m), int64(y)
	}
	if s, ok := c.visual(r, "facility_stamp"); !ok {
		g.unknown["facility"] = true
	} else if s == cueMissing {
		g.absent["facility"] = "facility stamp missing"
	} else if f, ok := c.text(r, "facility"); !ok {
		g.unknown["facility"] = true
	} else if n, err := leadingInt(f); err != nil {
		r.unknown("facility " + strconv.Quote(f))
		g.unknown["facility"] = true
	} else {
		g.vals["facility"] = int64(n)
	}
}

// matches is Appraisal.Matches; a condition on a cue that could not be read never matches.
func (g *guideCues) matches(w guideCond) bool {
	for _, t := range w {
		if g.unknown[t.Cue] {
			return false
		}
		v, ok := g.vals[t.Cue]
		if !guideMatch(v, ok, t.Match) {
			return false
		}
	}
	return true
}

// guideMatch is Appraisal.Match: a value (equal), an array (any of), {"range": [lo, hi]} or {"not": m}.
func guideMatch(cue any, present bool, m any) bool {
	switch m := m.(type) {
	case []any:
		return slices.ContainsFunc(m, func(x any) bool { return guideMatch(cue, present, x) })
	case map[string]any:
		if n, ok := m["not"]; ok {
			return !guideMatch(cue, present, n)
		}
		r, ok := m["range"].([]any)
		n, isInt := cue.(int64)
		if !ok || len(r) != 2 || !present || !isInt {
			return false
		}
		lo, _ := r[0].(float64)
		hi, _ := r[1].(float64)
		return float64(n) >= lo && float64(n) <= hi
	case float64:
		n, ok := cue.(int64)
		return present && ok && float64(n) == m
	}
	return present && cue == m
}

// valueAppraisal applies the item type's table to its cues (Appraisal.Value).
func valueAppraisal(c *inspectCtx, r *ruleCheck) *valuation {
	key := ""
	for _, g := range guideTypes {
		if g.kind == c.Item.Kind {
			key = g.key
		}
	}
	tables, _ := currentGuides()
	t := tables[key]
	if t == nil {
		r.unknown("the appraisal guides' value table is not loaded (" + appraisalGuidesFile + ")")
		return nil
	}
	g := readGuideCues(c, r)
	var bad []string
	for _, w := range t.InvalidWhen {
		if g.matches(w) {
			bad = append(bad, describeCond(w)+" (read: "+g.describeRead(w)+")")
		}
	}
	if len(bad) > 0 {
		for _, b := range bad {
			r.fail("", "invalid", "the guide makes it invalid: "+b)
		}
		return &valuation{Value: t.Invalid, Lines: []string{fmt.Sprintf("invalid (%s), worth %d", bad[0], t.Invalid)}}
	}
	if len(t.InvalidWhen) > 0 && r.certain() {
		r.Passed = append(r.Passed, "no rule of the guide makes it invalid")
	}
	v := &valuation{Value: t.Base, Lines: []string{fmt.Sprintf("base %d", t.Base)}}
	used := map[string]bool{}
	for _, rule := range t.Add {
		if rule.Group != "" && used[rule.Group] || !g.matches(rule.When) {
			continue
		}
		if rule.Group != "" {
			used[rule.Group] = true
		}
		v.add(rule.Add, rule.label())
	}
	return v.floor(t.Min)
}

func (r guideRule) label() string {
	if r.Why != "" {
		return r.Why
	}
	return describeCond(r.When)
}

// describeRead says what was read for the cues w names.
func (g *guideCues) describeRead(w guideCond) string {
	var parts []string
	for _, t := range w {
		switch v, ok := g.vals[t.Cue]; {
		case ok:
			parts = append(parts, fmt.Sprintf("%s %v", t.Cue, v))
		case g.absent[t.Cue] != "":
			parts = append(parts, fmt.Sprintf("no %s, %s", t.Cue, g.absent[t.Cue]))
		default:
			parts = append(parts, "no "+t.Cue)
		}
	}
	return strings.Join(parts, ", ")
}

func describeCond(w guideCond) string {
	var parts []string
	for _, t := range w {
		switch m := t.Match.(type) {
		case bool:
			if m {
				parts = append(parts, t.Cue)
			} else {
				parts = append(parts, "not "+t.Cue)
			}
		default:
			parts = append(parts, t.Cue+" "+describeMatch(m))
		}
	}
	return strings.Join(parts, ", ")
}

func describeMatch(m any) string {
	switch m := m.(type) {
	case []any:
		var xs []string
		for _, x := range m {
			xs = append(xs, describeMatch(x))
		}
		return strings.Join(xs, " or ")
	case map[string]any:
		if n, ok := m["not"]; ok {
			return "not " + describeMatch(n)
		}
		if r, ok := m["range"].([]any); ok && len(r) == 2 {
			return fmt.Sprintf("%v-%v", r[0], r[1])
		}
	}
	return fmt.Sprintf("%v", m)
}

// renderGuides states the loaded tables for the inspect prompt.
func renderGuides(t appraisalTables) string {
	if t == nil {
		return "- The guides' value tables are not loaded (" + appraisalGuidesFile + " is absent on this machine), so no true value can be computed: the rule check marks the value unknown. Defer.\n"
	}
	var b strings.Builder
	for _, g := range guideTypes {
		tab := t[g.key]
		if tab == nil {
			fmt.Fprintf(&b, "- %s: its value table is not loaded; defer.\n", g.name)
			continue
		}
		fmt.Fprintf(&b, "- %s: base %d.", g.name, tab.Base)
		if len(tab.InvalidWhen) > 0 {
			var xs []string
			for _, w := range tab.InvalidWhen {
				xs = append(xs, describeCond(w))
			}
			fmt.Fprintf(&b, " Worth %d if any of: %s. Otherwise:", tab.Invalid, strings.Join(xs, "; "))
		}
		done := map[string]bool{}
		var xs []string
		for _, rule := range tab.Add {
			if rule.Group == "" {
				xs = append(xs, fmt.Sprintf("%+d %s", rule.Add, rule.label()))
				continue
			}
			if done[rule.Group] {
				continue
			}
			done[rule.Group] = true
			var gs []string
			for _, o := range tab.Add {
				if o.Group == rule.Group {
					gs = append(gs, fmt.Sprintf("%+d %s", o.Add, o.label()))
				}
			}
			xs = append(xs, rule.Group+", only the first that applies: "+strings.Join(gs, ", else "))
		}
		fmt.Fprintf(&b, " %s; at least %d.\n", strings.Join(xs, "; "), tab.Min)
	}
	b.WriteString("- The tables name cues in the guide's words: " + guideVocabulary() + ".\n")
	return b.String()
}

// guideVocabulary maps the prompt's cue words to the tables' where they differ.
func guideVocabulary() string {
	var out []string
	for _, g := range guideTypes {
		words := guideWords[g.kind]
		for _, vis := range slices.Sorted(maps.Keys(words)) {
			m := words[vis]
			var xs []string
			for _, w := range slices.Sorted(maps.Keys(m.words)) {
				if v := m.words[w]; v != nil && fmt.Sprint(v) != w {
					xs = append(xs, fmt.Sprintf("%v = %s", v, w))
				}
			}
			if len(xs) > 0 || m.cue != vis {
				out = append(out, fmt.Sprintf("%s %s is %s (%s)", g.key, m.cue, strings.ReplaceAll(vis, "_", " "), strings.Join(xs, ", ")))
			}
		}
	}
	return strings.Join(out, "; ") + "; ribwich month, year and facility are the printed date and facility number, absent when the stamp is missing"
}
