package agency

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/gallowaysoftware/fence/internal/route"
	"github.com/gallowaysoftware/fence/protocol/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Composition is a route the strategist builds itself: a menu route with levers changed.
type Composition struct {
	Name  string `json:"name"`
	Base  string `json:"base"`
	Start string `json:"start,omitempty"`
	Perks string `json:"perks,omitempty"`
	// Levers are directive keys over the base's; buy and sell merge into the base's objects,
	// and null removes a key from the base.
	Levers map[string]any `json:"levers"`
}

// lever is one key a composition may set that the directives contract does not define: the
// gym executor's own levers (sim/gym/README.md), with the doc the strategist is shown.
type lever struct {
	doc   string
	check func(v any) error
}

func intIn(lo, hi int) func(any) error {
	return func(v any) error {
		f, ok := v.(float64)
		if !ok || f != float64(int(f)) || int(f) < lo || int(f) > hi {
			return fmt.Errorf("want an integer %d–%d", lo, hi)
		}
		return nil
	}
}

func oneOf(vals ...any) func(any) error {
	return func(v any) error {
		if slices.Contains(vals, v) {
			return nil
		}
		return fmt.Errorf("want one of %v", vals)
	}
}

func pair(lo, hi int) func(any) error {
	return func(v any) error {
		a, ok := v.([]any)
		if !ok || len(a) != 2 || intIn(lo, hi)(a[0]) != nil || intIn(lo, hi)(a[1]) != nil || a[0].(float64) > a[1].(float64) {
			return fmt.Errorf("want [first, last] days %d–%d", lo, hi)
		}
		return nil
	}
}

func subset(allowed ...string) func(any) error {
	return func(v any) error {
		a, ok := v.([]any)
		if !ok {
			return fmt.Errorf("want a list of %v", allowed)
		}
		for _, x := range a {
			if s, _ := x.(string); !slices.Contains(allowed, s) {
				return fmt.Errorf("want a list of %v", allowed)
			}
		}
		return nil
	}
}

var isBool = oneOf(true, false)

// gymLevers are keyed by path: "buy.no_singles" is no_singles inside buy.
var gymLevers = map[string]lever{
	"bays":                   {"wine bays, 0–6 (the routes run 2, the vintner 4)", intIn(0, 6)},
	"furnaces":               {"blast furnaces, 1–2 (the smelter runs 2)", intIn(1, 2)},
	"furnace_eco":            {"eco modules beside the furnace's blast module", isBool},
	"ore_phone":              {"order ore by phone for the furnace", isBool},
	"labeler":                {`"per_buyer": read fakes under the magnifier and sell a few as genuine to buyers who cannot tell`, oneOf("per_buyer")},
	"alarm":                  {`"perfect": buy a burglar alarm and tune it until no thief gets through`, oneOf("perfect")},
	"transmitters":           {`["sec"]: broadcast to Security until the police treat the shop as VIP (needs alarm)`, subset("sec")},
	"pharma":                 {`"phone": Dale's courier brings pink injectors at a fifth off`, oneOf("phone")},
	"beer":                   {`"phone": Red Imp beer by phone (neutral to slightly slower in the lab)`, oneOf("phone")},
	"jackson_every":          {"days between Dr. Jackson's visits, 7–28 (routes: 14)", intIn(7, 28)},
	"short_rent_buy_pct":     {"while cash is under the rent floor, keep buying lots at this % of the estimate to sell before rent day (rock_bottom: 85)", intIn(0, 100)},
	"short_rent_days":        {"from how many days before rent short_rent_buy_pct stops, 1–7", intIn(1, 7)},
	"georgio":                {`Georgio's day-43 loan: "lend_25", "lend_50" or "no"`, oneOf("lend_25", "lend_50", "no")},
	"georgio_peak_days":      {"[first, last]: stop buying and sell down over those days to lend Georgio more", pair(30, 60)},
	"purify_raw":             {"tap water straight into an idle purifier (water engine)", isBool},
	"purifiers":              {"water purifiers, 1–8 (water engine)", intIn(1, 8)},
	"printer":                {`"run": the gunsmith's 3D printer (cost two weeks in the lab)`, oneOf("run")},
	"restock_missed":         {"restock the types buyers asked for and the shop lacked", isBool},
	"buy.no_singles":         {"wholesale lots only (engine inputs excepted)", isBool},
	"sell.liquidate_min_pct": {"lowest % of the estimate a sell-down for rent accepts, 40–100 (routes: 78)", intIn(40, 100)},
	"sell.hot_max_heat":      {"stage stolen goods at this heat or less for any buyer but Security, 0–100", intIn(0, 100)},
}

// contractLevers are the directives-contract keys a composition may set; their values are
// checked against protocol/schema/directives.schema.json, plus the bounds below.
var contractLevers = map[string]lever{
	"engine":                {`production: "wine", "water", "furnace", "rats" joined by "+", or "none" (rats need the rancher start, water the water_merchant start)`, nil},
	"bargain":               {`haggling with buyers: "never", "ev_positive", "discount" ("aggressive" loses every run to the Lower Levels mob)`, oneOf("never", "ev_positive", "discount")},
	"bargain_buy":           {`haggling with sellers: "never", "ev_positive", "discount"`, oneOf("never", "ev_positive", "discount")},
	"permit":                {`"buy", or null to leave it to the executor (buys once evidence reaches 30)`, nil},
	"buyout_by_day":         {"buy the shop for cash by this day, 84–200 (routes: 140); the loan shark hears \"later\" while it projects", intIn(84, 200)},
	"buyout_save":           {`spending cut while saving for the buyout: any of "insurance", "upgrades", "inputs", "vending" (routes: ["insurance"])`, nil},
	"buyout_save_from":      {"the day buyout_save starts, 1–140 (routes: 56)", intIn(1, 140)},
	"fountain":              {`Wild's water fountain: "buy" (1 800) or "landlord" (+150 weekly rent); needs the water engine`, nil},
	"speculate":             {`{"types": [...], "max_days": N}: hold these goods from ordinary buyers until a market event, an Upper Levels buyer with tips, or N days`, typed},
	"specialty":             {`{"types": [...], "lot_share": 0–1}: buy only these goods`, typed},
	"mortgage_margin":       {"the executor signs the loan shark when the projected cover reaches this share of the payments (routes: 0.8)", nil},
	"buy.wholesale_max_pct": {"accept a wholesale lot up to this % of its estimate (routes: 88)", intIn(50, 100)},
	"buy.single_max_pct":    {"accept a single lot up to this % of its estimate (routes: 75)", intIn(0, 100)},
	"buy.hot_stock_cap":     {"stolen items held at most (0: buy nothing stolen)", intIn(0, 100)},
	"buy.prefer_types":      {"item types to favour when buying", typed},
	"buy.avoid_types":       {"item types never bought", typed},
	"sell.min_pct":          {"lowest % of the estimate an ordinary sale accepts (routes: 57)", intIn(30, 100)},
}

// ItemTypes are the item categories speculate, specialty and the buy lists take.
var ItemTypes = []string{"MEDICAL", "FOOD", "WATER", "MATERIAL", "HOUSEHOLD_GOOD", "LUXURY_ITEM", "ART", "WEAPON", "TOOL", "SUBSTANCE"}

// typed checks a {"types": [...]} lever's types, or a bare list of types.
func typed(v any) error {
	list := v
	if obj, ok := v.(map[string]any); ok {
		list = obj["types"]
		if list == nil {
			return errors.New("needs types")
		}
	}
	if err := subset(ItemTypes...)(list); err != nil {
		return err
	}
	if len(list.([]any)) == 0 {
		return errors.New("needs at least one type")
	}
	return nil
}

// LeverText documents every lever a composition may set, for the strategist.
func LeverText() string {
	var b strings.Builder
	keys := slices.Sorted(func(yield func(string) bool) {
		for k := range contractLevers {
			if !yield(k) {
				return
			}
		}
		for k := range gymLevers {
			if !yield(k) {
				return
			}
		}
	})
	for _, k := range keys {
		l, ok := contractLevers[k]
		if !ok {
			l = gymLevers[k]
		}
		fmt.Fprintf(&b, "- %s: %s\n", k, l.doc)
	}
	fmt.Fprintf(&b, "Item types: %s.\n", strings.Join(ItemTypes, ", "))
	return b.String()
}

var (
	schemaOnce sync.Once
	dirSchema  *jsonschema.Schema
	schemaErr  error
)

func directivesSchema() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		const name = "directives.schema.json"
		b, err := schema.FS.ReadFile(name)
		if err != nil {
			schemaErr = err
			return
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			schemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schema.BaseURL+name, doc); err != nil {
			schemaErr = err
			return
		}
		dirSchema, schemaErr = c.Compile(schema.BaseURL + name)
	})
	return dirSchema, schemaErr
}

// Validate checks c against the menu it was composed from and returns every problem.
func (c *Composition) Validate(m *Menu) []string {
	var errs []string
	bad := func(format string, a ...any) { errs = append(errs, fmt.Sprintf(format, a...)) }
	if !nameOK(c.Name) {
		bad("name %q: want lowercase letters, digits and _ (up to 32)", c.Name)
	} else if _, clash := m.Get(c.Name); clash {
		bad("name %q is already a menu route; pick the route instead or name yours differently", c.Name)
	} else if c.Name == m.excluded {
		bad("name %q is the route off tonight's menu; name yours differently", c.Name)
	}
	base, ok := m.Get(c.Base)
	if !ok {
		bad("base %q is not on the menu", c.Base)
		return errs
	}
	start, perks := c.start(base), c.perks(base)
	if !slices.Contains(m.Starts(), start) {
		bad("start %q: want one of %v", start, m.Starts())
	}
	if !slices.Contains(m.PerkSets(), sortedSet(perks)) {
		bad("perks %q: want one of %v", perks, m.PerkSets())
	}
	if len(c.Levers) == 0 {
		bad("levers: none changed; that is the %s route itself", c.Base)
	}
	contract := map[string]any{}
	for _, path := range sortedPaths(c.Levers) {
		v := path.value
		if l, ok := gymLevers[path.key]; ok {
			if v != nil {
				if err := l.check(v); err != nil {
					bad("%s: %v", path.key, err)
				}
			}
			continue
		}
		l, ok := contractLevers[path.key]
		if !ok {
			bad("%s: not a lever (see the list)", path.key)
			continue
		}
		if v == nil {
			continue
		}
		if l.check != nil {
			if err := l.check(v); err != nil {
				bad("%s: %v", path.key, err)
				continue
			}
		}
		if parent, child, nested := strings.Cut(path.key, "."); nested {
			obj, _ := contract[parent].(map[string]any)
			if obj == nil {
				obj = map[string]any{}
				contract[parent] = obj
			}
			obj[child] = v
		} else {
			contract[path.key] = v
		}
	}
	if s, err := directivesSchema(); err != nil {
		bad("directives schema: %v", err)
	} else if err := s.Validate(contract); err != nil {
		bad("against directives.schema.json: %s", oneLine(err.Error()))
	}
	d, err := c.merged(base)
	if err != nil {
		bad("%v", err)
		return errs
	}
	errs = append(errs, crossCheck(d, start)...)
	return errs
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// crossCheck rejects the combinations no route was tested with and the executor cannot play.
func crossCheck(d map[string]any, start string) []string {
	var errs []string
	engine, _ := d["engine"].(string)
	parts := strings.Split(engine, "+")
	if slices.Contains(parts, "rats") && start != "rancher" {
		errs = append(errs, "engine rats needs the rancher start (its cage)")
	}
	if slices.Contains(parts, "water") && start != "water_merchant" {
		errs = append(errs, "engine water needs the water_merchant start")
	}
	if d["fountain"] != nil && !slices.Contains(parts, "water") {
		errs = append(errs, "fountain needs the water engine")
	}
	if d["printer"] != nil && start != "gunsmith" {
		errs = append(errs, "printer needs the gunsmith start")
	}
	if d["transmitters"] != nil && d["alarm"] == nil {
		errs = append(errs, "transmitters need alarm")
	}
	if d["appraisal"] != "join" {
		errs = append(errs, "the Appraisal Board must stay joined (no route wins hard without it)")
	}
	return errs
}

func nameOK(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

func (c *Composition) start(base Entry) string {
	if c.Start != "" {
		return c.Start
	}
	return base.Start
}

func (c *Composition) perks(base Entry) string {
	if c.Perks != "" {
		return c.Perks
	}
	return base.Perks
}

type leverPath struct {
	key   string
	value any
}

// sortedPaths flattens buy and sell one level ("buy.no_singles"), the only nested levers.
func sortedPaths(levers map[string]any) []leverPath {
	var out []leverPath
	for k, v := range levers {
		if obj, ok := v.(map[string]any); ok && (k == "buy" || k == "sell") {
			for ck, cv := range obj {
				out = append(out, leverPath{k + "." + ck, cv})
			}
			continue
		}
		out = append(out, leverPath{k, v})
	}
	slices.SortFunc(out, func(a, b leverPath) int { return strings.Compare(a.key, b.key) })
	return out
}

// merged is the base's directives with the levers applied.
func (c *Composition) merged(base Entry) (map[string]any, error) {
	var d map[string]any
	if err := json.Unmarshal([]byte(base.Base), &d); err != nil {
		return nil, fmt.Errorf("base %s: %w", base.Name, err)
	}
	for _, p := range sortedPaths(c.Levers) {
		parent, child, nested := strings.Cut(p.key, ".")
		target := d
		key := p.key
		if nested {
			obj, _ := d[parent].(map[string]any)
			if obj == nil {
				obj = map[string]any{}
				d[parent] = obj
			}
			target, key = obj, child
		}
		if p.value == nil {
			delete(target, key)
		} else {
			target[key] = p.value
		}
	}
	return d, nil
}

// File is the composed route as a routes/<name>.json file: the merged directives under a
// "_route" key whose brief says what was composed.
func (c *Composition) File(m *Menu, persona, brief string) ([]byte, error) {
	if errs := c.Validate(m); len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	base, _ := m.Get(c.Base)
	d, err := c.merged(base)
	if err != nil {
		return nil, err
	}
	if persona == "" {
		persona = base.Persona
	}
	if len(brief) > route.MaxBriefBytes {
		brief = strings.TrimSpace(brief[:route.MaxBriefBytes-3]) + "..."
	}
	start, perks := c.start(base), c.perks(base)
	d["_route"] = map[string]any{"start": start, "perks": perks, "persona": persona, "brief": brief,
		"args": "-difficulty hard -start " + start + " -perks " + perks + " -human player", "composed_from": c.Base}
	return json.Marshal(d)
}

// Summary names the levers changed, for a brief or a feed line.
func (c *Composition) Summary() string {
	var parts []string
	for _, p := range sortedPaths(c.Levers) {
		v, _ := json.Marshal(p.value)
		parts = append(parts, p.key+" "+string(v))
	}
	s := c.Base + " with " + strings.Join(parts, ", ")
	if c.Start != "" {
		s += ", start " + c.Start
	}
	if c.Perks != "" {
		s += ", perks " + c.Perks
	}
	return s
}

// LeverNames lists the levers by name only.
func LeverNames() string {
	var keys []string
	for k := range contractLevers {
		keys = append(keys, k)
	}
	for k := range gymLevers {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return strings.Join(keys, ", ") + ". Item types: " + strings.Join(ItemTypes, ", ") + "."
}
