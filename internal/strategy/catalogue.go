package strategy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

//go:embed catalogue.json
var catalogueJSON []byte

// CatalogueEntry is one item's use: what it does for the shop's own production and what
// that is worth, from docs/re/PRODUCTION.md §9 and docs/re/WATER.md §8.
type CatalogueEntry struct {
	Key       string   `json:"key"`
	Match     []string `json:"match"`
	Chain     string   `json:"chain"`
	SoldAs    string   `json:"sold_as"`
	BaseValue *int64   `json:"base_value"`
	Use       string   `json:"use"`
	Consumed  string   `json:"consumed"`
	Effect    string   `json:"effect"`
	// UseValue is credits per item, or per Per ("night", "week", "cycle"); nil when the
	// docs give no number.
	UseValue    *int64           `json:"use_value"`
	Per         string           `json:"per"`
	UseBasis    string           `json:"use_basis"`
	GradeValues map[string]int64 `json:"grade_values"`
	// When lists the shop states that open the use: any alternative, each a "+"-joined
	// conjunction of condition names (prodState.cond).
	When   []string `json:"when"`
	Source string   `json:"source"`

	re []*regexp.Regexp
}

type Catalogue struct {
	Items []CatalogueEntry `json:"items"`
}

// useHorizonNights converts per-night use values into a sum comparable with a one-off
// sale price: equipment is judged on what it earns over two weeks.
const useHorizonNights = 14

var catalogue = mustCatalogue(catalogueJSON)

func mustCatalogue(raw []byte) *Catalogue {
	c, err := LoadCatalogue(raw)
	if err != nil {
		panic(err)
	}
	return c
}

// LoadCatalogue parses a catalogue file (catalogue.json's format).
func LoadCatalogue(raw []byte) (*Catalogue, error) {
	var c Catalogue
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("catalogue: %w", err)
	}
	for i := range c.Items {
		e := &c.Items[i]
		if e.Key == "" || len(e.Match) == 0 || len(e.When) == 0 || e.Source == "" {
			return nil, fmt.Errorf("catalogue entry %d (%q): key, match, when and source are required", i, e.Key)
		}
		for _, p := range e.Match {
			re, err := regexp.Compile(p)
			if err != nil {
				return nil, fmt.Errorf("catalogue %s: %w", e.Key, err)
			}
			e.re = append(e.re, re)
		}
		for _, w := range e.When {
			for _, c := range strings.Split(w, "+") {
				if !knownCondition(c) {
					return nil, fmt.Errorf("catalogue %s: unknown condition %q", e.Key, c)
				}
			}
		}
	}
	return &c, nil
}

// Lookup returns the first entry whose pattern matches the item's id or name
// (lower-cased).
func (c *Catalogue) Lookup(id, name string) (*CatalogueEntry, bool) {
	id, name = strings.ToLower(strings.TrimSpace(id)), strings.ToLower(strings.TrimSpace(name))
	for i := range c.Items {
		e := &c.Items[i]
		for _, re := range e.re {
			if (id != "" && re.MatchString(id)) || (name != "" && re.MatchString(name)) {
				return e, true
			}
		}
	}
	return nil, false
}

// open reports whether one of e's alternatives holds in ps, and otherwise names what is
// missing.
func (e *CatalogueEntry) open(ps *prodState) (bool, string) {
	var missing []string
	for _, alt := range e.When {
		ok := true
		for _, c := range strings.Split(alt, "+") {
			if !ps.cond(c) {
				ok = false
				missing = append(missing, conditionText[c])
				break
			}
		}
		if ok {
			return true, ""
		}
	}
	return false, strings.Join(dedupe(missing), " or ")
}

// worth is the use value comparable with a one-off sale, for an item of berry grade g
// (ignored unless the entry prices by grade); ok false when the docs give no number.
func (e *CatalogueEntry) worth(g string) (int64, bool) {
	if len(e.GradeValues) > 0 {
		if v, ok := e.GradeValues[g]; ok {
			return v, true
		}
		return e.GradeValues["normal"], true
	}
	if e.UseValue == nil {
		return 0, false
	}
	v := *e.UseValue
	switch e.Per {
	case "night":
		v *= useHorizonNights
	case "week":
		v *= useHorizonNights / 7
	}
	return v, true
}

func (e *CatalogueEntry) valueText(g string) string {
	if len(e.GradeValues) > 0 {
		if v, ok := e.GradeValues[g]; ok && g != "" {
			return fmt.Sprintf("≈ %d", v)
		}
		lo, hi := int64(1<<62), int64(0)
		for k, v := range e.GradeValues {
			if k == "moldy" {
				continue
			}
			lo, hi = min(lo, v), max(hi, v)
		}
		return fmt.Sprintf("≈ %d-%d by grade", lo, hi)
	}
	if e.UseValue == nil {
		return "not priced"
	}
	t := fmt.Sprintf("≈ %d", *e.UseValue)
	if e.Per != "" {
		t += " a " + e.Per
	}
	return t
}

// ItemUse is the catalogue's answer for one item in the shop's current state.
type ItemUse struct {
	Key    string
	Chain  string
	Use    string
	Effect string
	Source string
	// UseValue is per item, or per Per; nil when the docs give no number.
	UseValue *int64
	Per      string
	// Open is whether the shop can put the item to this use now; Missing names what it
	// lacks when not.
	Open    bool
	Missing string
}

// UseNote is a one-line use-versus-sale judgement for an item at the counter, read
// against the shop's morning snapshot (nil or unreadable: judged without the shop's
// state, so every use counts as open). sale is what the deal would pay, 0 if unknown.
// "" when the catalogue knows no use for the item.
func UseNote(snapshot json.RawMessage, id, name string, sale int64) string {
	e, ok := catalogue.Lookup(id, name)
	if !ok {
		return ""
	}
	var ps *prodState
	if s, err := parseSnapshot(snapshot); err == nil && len(snapshot) > 0 {
		ps = s.production()
	}
	grade := berryGradeText(name)
	t := fmt.Sprintf("use value %s (%s: %s)", e.valueText(grade), e.Chain, e.Effect)
	if ps != nil {
		if open, missing := e.open(ps); !open {
			t = fmt.Sprintf("no use in this shop now (needs %s); otherwise %s", nonEmpty(missing, "a use the shop lacks"), t)
		}
	}
	if sale > 0 {
		t += fmt.Sprintf(", sale ≈ %d", sale)
	}
	return t
}

// UseNote is UseNote against the latest morning snapshot of session's run.
func (s *Strategist) UseNote(session, id, name string, sale int64) string {
	s.mu.Lock()
	var raw json.RawMessage
	if r := s.sessions[session]; r != nil {
		best := -1
		for n, d := range r.days {
			if d.morningRaw != nil && n > best {
				best, raw = n, d.morningRaw
			}
		}
	}
	s.mu.Unlock()
	return UseNote(raw, id, name, sale)
}

// LookupUse is the catalogue entry for an item without reference to the shop's state.
func LookupUse(id, name string) (ItemUse, bool) {
	e, ok := catalogue.Lookup(id, name)
	if !ok {
		return ItemUse{}, false
	}
	return ItemUse{Key: e.Key, Chain: e.Chain, Use: e.Use, Effect: e.Effect, Source: e.Source, UseValue: e.UseValue, Per: e.Per, Open: true}, true
}

// useGroup is the inventory's items of one catalogue entry (and berry grade), with the
// catalogue's verdict on them.
type useGroup struct {
	e     *CatalogueEntry
	grade string
	name  string
	uids  []int64
	sale  int64
	saleN int

	verdict string // keep, unpriced, idle, sell
	use     int64
	missing string
}

// useGroups judges every stock item the catalogue knows, in inventory order. Items in a
// machine or the pitcher are in use and left out.
func (s *snapshot) useGroups(ps *prodState) []*useGroup {
	groups := map[string]*useGroup{}
	var out []*useGroup
	for _, it := range s.Inventory {
		if it.stolen() || ps.isFinishedWine(it) || ps.isBusyBottle(it) || s.inMachine(it) {
			continue
		}
		e, ok := catalogue.Lookup(it.ID, it.Name)
		if !ok {
			continue
		}
		g := ""
		if e.Key == "wine_berry" {
			g = ps.berryGrade(it)
		}
		k := e.Key + "/" + g + "/" + itemName(it)
		ug := groups[k]
		if ug == nil {
			ug = &useGroup{e: e, grade: g, name: itemName(it)}
			groups[k] = ug
			out = append(out, ug)
		}
		if it.UID != nil {
			ug.uids = append(ug.uids, *it.UID)
		}
		if it.EstimatedValue != nil {
			ug.sale += *it.EstimatedValue
			ug.saleN++
		}
	}
	for _, ug := range out {
		sale, saleKnown := ug.saleEach()
		open, missing := ug.e.open(ps)
		use, priced := ug.e.worth(ug.grade)
		ug.use, ug.missing = use, missing
		switch {
		case !open && ug.e.When[0] == "never":
			ug.verdict = "sell"
		case !open:
			ug.verdict = "idle"
		case !priced:
			ug.verdict = "unpriced"
		case !saleKnown || use > sale:
			ug.verdict = "keep"
		default:
			ug.verdict = "sell"
		}
	}
	return out
}

// keepUIDs maps every item the catalogue says to keep to its group.
func (s *snapshot) keepUIDs(ps *prodState) map[int64]*useGroup {
	m := map[int64]*useGroup{}
	for _, ug := range s.useGroups(ps) {
		if ug.verdict == "keep" {
			for _, u := range ug.uids {
				m[u] = ug
			}
		}
	}
	for _, u := range ps.cleanWaterUIDs() {
		uid := u
		v := int64(0)
		m[u] = &useGroup{e: cleanWaterEntry, name: s.nameOf(&uid, "water"), uids: []int64{u}, verdict: "keep", use: v}
	}
	return m
}

// renderUseValues writes the USE VALUE lines: items worth more used than sold right
// now, items whose use is not open in this shop, and the other way round.
func (s *snapshot) renderUseValues(b *strings.Builder, ps *prodState) {
	var keep, unpriced, idle, sell, berries []string
	var berryEntry *CatalogueEntry
	for _, ug := range s.useGroups(ps) {
		label := ug.label()
		sale, saleKnown := ug.saleEach()
		switch ug.verdict {
		case "idle":
			idle = append(idle, fmt.Sprintf("%s (needs %s)", label, ug.missing))
		case "unpriced":
			unpriced = append(unpriced, fmt.Sprintf("%s: %s — %s [%s]", label, ug.e.Use, ug.e.Effect, ug.e.Source))
		case "keep":
			if ug.e.Key == "wine_berry" {
				berryEntry = ug.e
				berries = append(berries, fmt.Sprintf("%s use %s", label, ug.e.valueText(ug.grade)))
				continue
			}
			t := fmt.Sprintf("%s: use %s", label, ug.e.valueText(ug.grade))
			if ug.e.Per != "" {
				t += fmt.Sprintf(" (%d over %d nights)", ug.use, useHorizonNights)
			}
			if saleKnown {
				t += fmt.Sprintf(" vs sale est. %d", sale)
			}
			t += fmt.Sprintf(" — %s: %s", ug.e.Chain, ug.e.Effect)
			if ug.e.UseBasis != "" {
				t += " (" + ug.e.UseBasis + ")"
			}
			keep = append(keep, t+" ["+ug.e.Source+"]")
		case "sell":
			if open, _ := ug.e.open(ps); !open {
				sell = append(sell, fmt.Sprintf("%s: %s", label, ug.e.Effect))
			} else {
				sell = append(sell, fmt.Sprintf("%s: use %s, below sale est. %d", label, ug.e.valueText(ug.grade), sale))
			}
		}
	}
	if e := berryEntry; e != nil {
		keep = append(keep, fmt.Sprintf("%s (sale about %d each) — %s: %s [%s]", strings.Join(berries, "; "), berryFoodValue, e.Chain, e.Effect, e.Source))
	}
	keep = append(keep, ps.cleanWaterKeeps(s)...)
	if len(keep)+len(unpriced)+len(idle)+len(sell) == 0 {
		return
	}
	b.WriteString("USE VALUE (item-use catalogue; an item is worth the better of its sale and its use here):\n")
	for _, t := range keep {
		b.WriteString("- KEEP: " + t + "\n")
	}
	for _, t := range unpriced {
		b.WriteString("- USE (not priced, judge it): " + t + "\n")
	}
	if len(idle) > 0 {
		b.WriteString("- No use in this shop now, safe to sell unless you will get what it needs: " + strings.Join(idle, "; ") + "\n")
	}
	for _, t := range sell {
		b.WriteString("- Sale beats use: " + t + "\n")
	}
}

func (ug *useGroup) label() string {
	t := ug.name
	if ug.grade != "" {
		t += " (" + berryGradeName[ug.grade] + ")"
	}
	if len(ug.uids) > 1 {
		t += fmt.Sprintf(" x%d", len(ug.uids))
	}
	for i, u := range ug.uids {
		if i == 4 {
			t += " …"
			break
		}
		t += fmt.Sprintf(" #%d", u)
	}
	return t
}

func (ug *useGroup) saleEach() (int64, bool) {
	if ug.saleN == 0 {
		return 0, false
	}
	return ug.sale / int64(ug.saleN), true
}

func dedupe(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x != "" && !slices.Contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}

// inMachine: the item sits inside a machine or the filter pitcher (a planted seed, a
// battery, a bay's water, a fitted filter), so it is in use, not stock to keep or sell.
func (s *snapshot) inMachine(it item) bool {
	uid, ok := strings.CutPrefix(it.Where, "inside:")
	if !ok {
		return false
	}
	for _, c := range s.Inventory {
		if c.UID != nil && fmt.Sprint(*c.UID) == uid {
			return machineKind(c.ID) != ""
		}
	}
	return false
}

// requestUseNote flags a request that asks for items the catalogue says to keep, with
// what the request pays each against their use.
func (s *snapshot) requestUseNote(i int) string {
	if i >= len(s.requestAsks) {
		return ""
	}
	a := s.requestAsks[i]
	if a.item == "" {
		return ""
	}
	e, ok := catalogue.Lookup(a.item, "")
	if !ok {
		return ""
	}
	for _, ug := range s.useGroups(s.production()) {
		if ug.e != e || ug.verdict != "keep" {
			continue
		}
		t := fmt.Sprintf(" [asks for KEEP items: use %s each", e.valueText(ug.grade))
		if a.reward != nil && a.count != nil && *a.count > 0 {
			t += fmt.Sprintf(" vs %d each from this request", *a.reward / *a.count)
		}
		return t + "]"
	}
	return ""
}

// nameOf is the inventory name of uid, else def.
func (s *snapshot) nameOf(uid *int64, def string) string {
	if uid != nil {
		for _, it := range s.Inventory {
			if it.UID != nil && *it.UID == *uid {
				return itemName(it)
			}
		}
	}
	return def
}

// cleanWaterEntry stands for tested High Quality or Pure water kept for the bay and wine;
// it is judged in production.go, not read from the catalogue file.
var cleanWaterEntry = &CatalogueEntry{Key: "clean_water", Chain: "hydroponics and wine", Effect: "High Quality or Pure water for the bay and the bottles"}
