package strategy

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// prodState is the shop's production as the snapshot shows it: the production and water
// blocks (docs/re/PRODUCTION.md §8, docs/re/WATER.md §6.1) when the mod sends them, and
// what the inventory implies otherwise. Field shapes are provisional, so every block is
// read loosely and anything unrecognised is kept as compact text.
type prodState struct {
	machines []prodMachine
	hydro    []hydroBay
	wine     []wineBottle
	furnaces []furnaceState
	scav     *scavenge
	water    []waterContainer
	taps     []string
	// other is production/water content this bridge does not recognise, as compact text.
	other string

	has          map[string]bool
	berryByUID   map[int64]string
	wineByUID    map[int64]wineBottle
	berries      map[string][]int64
	emptyBottles []int64
	yeast        map[string]int
	stock        map[string]int
	scavOpen     bool
}

type idRef struct {
	UID *int64 `json:"uid"`
	ID  string `json:"id"`
}

type prodMachine struct {
	UID           *int64   `json:"uid"`
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	Where         string   `json:"where"`
	Powered       *bool    `json:"powered"`
	PowerPerNight *int64   `json:"power_per_night"`
	BatteryCharge *int64   `json:"battery_charge"`
	State         string   `json:"state"`
	Progress      *int64   `json:"progress"`
	Target        *int64   `json:"target"`
	Speed         *int64   `json:"speed"`
	ETA           *int64   `json:"eta_nights"`
	Modules       []idRef  `json:"modules"`
	QualityBonus  *int64   `json:"quality_bonus"`
	PerfBonus     *int64   `json:"performance_bonus"`
	Inputs        []idRef  `json:"inputs"`
	Outputs       []idRef  `json:"outputs"`
	StatusText    string   `json:"status_text"`
	MaxPurityBP   *int64   `json:"max_purity_bp"`
	OutML         *int64   `json:"out_ml"`
	OutGrade      string   `json:"out_grade"`
	legacy        []string // id/name/contents of an unstructured entry, for kind detection
	empty         bool
}

type hydroBay struct {
	UID  *int64 `json:"uid"`
	Seed *struct {
		UID         *int64 `json:"uid"`
		PlantID     string `json:"plant_id"`
		Perennial   *bool  `json:"perennial"`
		Established *bool  `json:"established"`
	} `json:"seed"`
	Progress     *int64 `json:"progress"`
	Target       *int64 `json:"target"`
	ETA          *int64 `json:"eta_nights"`
	HarvestCount *int64 `json:"harvest_count"`
	WaterML      *int64 `json:"water_ml"`
	WaterUse     *int64 `json:"water_use"`
	Status       string `json:"status"`
}

type wineBottle struct {
	UID         *int64 `json:"uid"`
	Where       string `json:"where"`
	State       string `json:"state"`
	WaterML     *int64 `json:"water_ml"`
	Ingredients []struct {
		UID        *int64 `json:"uid"`
		ID         string `json:"id"`
		BerryGrade string `json:"berry_grade"`
	} `json:"ingredients"`
	NightsLeft  *int64 `json:"nights_left"`
	Tier        *int64 `json:"tier"`
	Grade       string `json:"grade"`
	Age         *int64 `json:"age"`
	Value       *int64 `json:"value"`
	Counterfeit *bool  `json:"counterfeit"`
}

type furnaceState struct {
	UID    *int64 `json:"uid"`
	Inputs *struct {
		Scrap   *int64 `json:"scrap"`
		Ore     *int64 `json:"ore"`
		Junk    *int64 `json:"junk"`
		Flux    *int64 `json:"flux"`
		AdvFlux *int64 `json:"adv_flux"`
	} `json:"inputs"`
	OutputCount   *int64 `json:"output_count"`
	Blast         *bool  `json:"blast"`
	JunkModule    *bool  `json:"junk_module"`
	BatteryCharge *int64 `json:"battery_charge"`
	QualityBonus  *int64 `json:"quality_bonus"`
}

type scavenge struct {
	AttemptsLeft *int64   `json:"attempts_left"`
	MinorPct     *float64 `json:"minor_pct"`
	MajorPct     *float64 `json:"major_pct"`
	WoundedFresh *bool    `json:"wounded_fresh"`
	ScannerPct   *int64   `json:"scanner_double_pct"`
}

type waterContainer struct {
	UID         *int64          `json:"uid"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Parent      json.RawMessage `json:"parent"`
	CapacityUL  *int64          `json:"capacity_ul"`
	VolumeUL    *int64          `json:"volume_ul"`
	VolumeBand  string          `json:"volume_band"`
	LabelBP     *int64          `json:"label_bp"`
	LabelGrade  json.RawMessage `json:"label_grade"`
	TrueGrade   json.RawMessage `json:"true_grade"`
	Exposed     *bool           `json:"exposed"`
	Discovered  *bool           `json:"discovered"`
	LastScan    string          `json:"last_scan"`
	Stale       *bool           `json:"stale"`
	Filter      *waterFilter    `json:"filter"`
	Value       *int64          `json:"value"`
	MaxPurityBP *int64          `json:"max_purity_bp"`
}

type waterFilter struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Durability *int64 `json:"durability"`
	Max        *int64 `json:"max_durability"`
	State      string `json:"state"`
}

// hiddenKeys never reach a prompt, even from a block rendered generically: the mod is to
// send them under hidden, and this is the second line of defence.
var hiddenKeys = []string{"hidden", "health", "score", "wine_score", "purity_bp", "parts", "true_purity", "true_grade", "match",
	"today_tap_bp", "dirty_water_tracker", "mislabeled_water_tracker", "mislabelled_water_tracker",
	"fountain_mislabel_rep_loss_ll", "fountain_mislabel_rep_loss_ul", "vending_mislabel_count", "real_condition", "true_value"}

// parseProduction reads the production and water blocks; what it does not recognise is
// returned as leftover for compact rendering.
func parseProduction(prodRaw, waterRaw json.RawMessage) *prodState {
	ps := &prodState{}
	left := map[string]any{}
	var top map[string]json.RawMessage
	if len(prodRaw) > 0 && json.Unmarshal(prodRaw, &top) != nil {
		var v any
		if json.Unmarshal(prodRaw, &v) == nil {
			left["production"] = v
		}
	}
	for k, raw := range top {
		var ok bool
		switch k {
		case "machines":
			ok = ps.parseMachines(raw, left)
		case "hydroponics":
			ok = json.Unmarshal(raw, &ps.hydro) == nil
		case "wine":
			ok = json.Unmarshal(raw, &ps.wine) == nil
		case "furnaces":
			ok = json.Unmarshal(raw, &ps.furnaces) == nil
		case "berries":
			var bs []struct {
				UID   *int64 `json:"uid"`
				Grade string `json:"grade"`
			}
			if ok = json.Unmarshal(raw, &bs) == nil; ok {
				for _, b := range bs {
					if b.UID != nil {
						if ps.berryByUID == nil {
							ps.berryByUID = map[int64]string{}
						}
						ps.berryByUID[*b.UID] = normBerry(b.Grade)
					}
				}
			}
		case "scavenge":
			ok = json.Unmarshal(raw, &ps.scav) == nil
		}
		if !ok {
			var v any
			if json.Unmarshal(raw, &v) == nil {
				left[k] = v
			}
		}
	}
	ps.parseWater(waterRaw, left)
	if t := textOf(left); t != "" {
		ps.other = clip(t, 600)
	}
	return ps
}

// structuredMachine is a machine entry in PRODUCTION.md §8's shape; older provisional
// entries ({id, name, contents, …}) are kept as text.
func structuredMachine(m map[string]any) bool {
	for _, k := range []string{"kind", "state", "progress", "eta_nights", "powered", "battery_charge", "power_per_night"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

func (ps *prodState) parseMachines(raw json.RawMessage, left map[string]any) bool {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return false
	}
	var rest []any
	for _, e := range list {
		var m map[string]any
		if json.Unmarshal(e, &m) != nil {
			continue
		}
		var pm prodMachine
		if structuredMachine(m) && json.Unmarshal(e, &pm) == nil {
			ps.machines = append(ps.machines, pm)
			continue
		}
		rest = append(rest, m)
		legacy := prodMachine{legacy: []string{firstStr(m, "id"), firstStr(m, "name"), firstStr(m, "kind")}}
		if c, ok := m["contents"].(string); ok && strings.EqualFold(c, "empty") {
			legacy.empty = true
		}
		if f, ok := m["filter"].(string); ok {
			legacy.StatusText = "filter " + f
		}
		ps.machines = append(ps.machines, legacy)
	}
	if len(rest) > 0 {
		left["machines"] = rest
	}
	return true
}

func (ps *prodState) parseWater(raw json.RawMessage, left map[string]any) {
	if len(raw) == 0 {
		return
	}
	if json.Unmarshal(raw, &ps.water) == nil {
		return
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return
	}
	for k, v := range obj {
		switch k {
		case "containers", "items":
			if json.Unmarshal(v, &ps.water) == nil {
				continue
			}
		case "taps":
			var taps map[string]bool
			if json.Unmarshal(v, &taps) == nil {
				for _, t := range sortedKeys(taps) {
					if taps[t] {
						ps.taps = append(ps.taps, t)
					}
				}
				continue
			}
		}
		var x any
		if !slices.Contains(hiddenKeys, k) && json.Unmarshal(v, &x) == nil {
			left["water."+k] = x
		}
	}
}

// machineKind names the machine an id, kind or name refers to, or "".
func machineKind(s string) string {
	s = strings.ToLower(s)
	switch {
	case s == "":
		return ""
	case strings.Contains(s, "hydroponic"):
		return "hydroponic"
	case strings.Contains(s, "furnace") && !strings.Contains(s, "module"):
		return "furnace"
	case strings.Contains(s, "purifier") && !strings.Contains(s, "portable"):
		return "water_purifier"
	case strings.Contains(s, "wine_rack") || s == "wine rack":
		return "wine_rack"
	case strings.Contains(s, "moisture"):
		return "moisture_farm"
	case strings.Contains(s, "evaporator"):
		return "evaporator"
	case s == "printer" || strings.Contains(s, "3d printer"):
		return "printer"
	case s == "ras" || strings.Contains(s, "fish tank"):
		return "ras"
	case s == "water_pitcher":
		return "pitcher"
	}
	return ""
}

func (pm prodMachine) kind() string {
	for _, s := range append([]string{pm.Kind, pm.ID, pm.Name}, pm.legacy...) {
		if k := machineKind(s); k != "" {
			return k
		}
	}
	return ""
}

func (pm prodMachine) hasModule(id string) bool {
	return slices.ContainsFunc(pm.Modules, func(r idRef) bool { return strings.EqualFold(r.ID, id) })
}

// production derives the shop's production state once per snapshot.
func (s *snapshot) production() *prodState {
	if s.prod != nil {
		return s.prod
	}
	ps := parseProduction(s.productionRaw, s.waterRaw)
	ps.has = map[string]bool{}
	ps.berries = map[string][]int64{}
	ps.yeast = map[string]int{}
	ps.stock = map[string]int{}
	ps.wineByUID = map[int64]wineBottle{}
	for _, w := range ps.wine {
		if w.UID != nil {
			ps.wineByUID[*w.UID] = w
		}
	}
	for _, m := range ps.machines {
		k := m.kind()
		if k == "" {
			continue
		}
		ps.has[k] = true
		ps.has["machines"] = true
		if k == "hydroponic" && (m.empty || strings.Contains(strings.ToLower(m.State+m.StatusText), "awaiting seed")) {
			ps.has["hydroponic_empty"] = true
		}
		if k == "furnace" && m.hasModule("furnace_module_junk") {
			ps.has["junk_module"] = true
		}
	}
	if len(ps.hydro) > 0 {
		ps.has["hydroponic"], ps.has["machines"] = true, true
		for _, h := range ps.hydro {
			if h.Seed == nil || h.Seed.PlantID == "" {
				ps.has["hydroponic_empty"] = true
			}
		}
	}
	for _, f := range ps.furnaces {
		ps.has["furnace"], ps.has["machines"] = true, true
		if f.JunkModule != nil && *f.JunkModule {
			ps.has["junk_module"] = true
		}
	}
	machineUIDs := map[string]bool{}
	for _, it := range s.Inventory {
		if k := machineKind(it.ID); k != "" && k != "pitcher" {
			ps.has[k], ps.has["machines"] = true, true
			if it.UID != nil {
				machineUIDs[fmt.Sprint(*it.UID)] = true
			}
			// A bay the production block does not describe may be empty: its seed's use stays open.
			if k == "hydroponic" && len(ps.hydro) == 0 && !slices.ContainsFunc(ps.machines, func(m prodMachine) bool { return m.kind() == "hydroponic" }) {
				ps.has["hydroponic_empty"] = true
			}
		} else if k == "pitcher" {
			ps.has["pitcher"] = true
		}
	}
	for _, it := range s.Inventory {
		inMachine := machineUIDs[strings.TrimPrefix(it.Where, "inside:")]
		e, ok := catalogue.Lookup(it.ID, it.Name)
		if !ok || it.stolen() {
			continue
		}
		switch e.Key {
		case "wine_berry":
			if !inMachine {
				g := ps.berryGrade(it)
				ps.berries[g] = append(ps.berries[g], uidOf(it))
			}
		case "wine_bottle":
			if !ps.isFinishedWine(it) && !ps.isBusyBottle(it) {
				ps.emptyBottles = append(ps.emptyBottles, uidOf(it))
			}
		case "wine_yeast", "wine_superyeast", "wine_yeast_infinite":
			ps.yeast[e.Key]++
		case "scrap_metal", "common_ore", "junk", "flux_agent", "advanced_flux_agent":
			if !inMachine {
				ps.stock[e.Key]++
			}
		}
		if ps.isFinishedWine(it) {
			ps.has["finished_wine"] = true
		}
	}
	for _, w := range ps.wine {
		if w.State == "finished" {
			ps.has["finished_wine"] = true
		}
	}
	for g, us := range ps.berries {
		if g != "moldy" && len(us) > 0 {
			ps.has["wine_berries"] = true
		}
	}
	for _, h := range ps.hydro {
		if h.Seed != nil && strings.Contains(h.Seed.PlantID, "bloomberry") && h.ETA != nil && *h.ETA <= 1 {
			ps.has["wine_berries"] = true
		}
	}
	ps.has["wine_bottles"] = len(ps.emptyBottles) > 0
	if ps.has["furnace"] {
		loaded := 0
		for _, f := range s.furnaceLoads(ps) {
			loaded += f.scrap + f.ore
		}
		ps.has["furnace_fed"] = loaded+ps.stock["scrap_metal"]+ps.stock["common_ore"] >= 2
	}
	ps.has["scavenge"] = ps.scav != nil || s.scavOpen
	ps.waterConditions(s)
	s.prod = ps
	return ps
}

func uidOf(it item) int64 {
	if it.UID != nil {
		return *it.UID
	}
	return -1
}

var conditionText = map[string]string{
	"always":           "",
	"never":            "a use in this shop",
	"hydroponic":       "a hydroponic bay",
	"hydroponic_empty": "an empty hydroponic bay",
	"wine_berries":     "berries that are not moldy",
	"wine_bottles":     "empty wine bottles",
	"finished_wine":    "finished wine",
	"furnace":          "a furnace",
	"furnace_fed":      "a furnace with scrap or ore",
	"junk_module":      "the furnace junk module",
	"printer":          "a 3D printer",
	"machines":         "a machine",
	"ras":              "a fish tank",
	"scavenge":         "scavenging",
	"water_to_treat":   "dirty water or a tap to clean",
	"water_stale":      "treated or mislabelled water to test",
}

func knownCondition(c string) bool { _, ok := conditionText[c]; return ok }

func (ps *prodState) cond(c string) bool {
	switch c {
	case "always":
		return true
	case "never":
		return false
	}
	return ps.has[c]
}

// berry grades, best first; contribution to the wine score (PRODUCTION.md §1).
var (
	berryContribution = map[string]int{"best": 2, "normal": 1, "under": 0, "over": 0, "moldy": -1}
	berryGradeName    = map[string]string{"best": "best", "normal": "normal", "under": "under-ripe", "over": "over-ripe", "moldy": "moldy", "": "grade unknown"}
)

func normBerry(s string) string {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "mold") || strings.Contains(s, "mould"):
		return "moldy"
	case strings.Contains(s, "under"):
		return "under"
	case strings.Contains(s, "over"):
		return "over"
	case strings.Contains(s, "best") || strings.Contains(s, "small") || strings.Contains(s, "perfect") || strings.Contains(s, "prime"):
		return "best"
	case strings.Contains(s, "ripe") || strings.Contains(s, "normal") || strings.Contains(s, "good"):
		return "normal"
	}
	return ""
}

func berryGradeText(s string) string { return normBerry(s) }

func (ps *prodState) berryGrade(it item) string {
	if it.UID != nil {
		if g, ok := ps.berryByUID[*it.UID]; ok {
			return g
		}
	}
	return normBerry(featureText(it))
}

var wineGradeWords = []string{"undrinkable", "bucket", "homebrew", "production", "reserve", "master"}

func (ps *prodState) isFinishedWine(it item) bool {
	if it.UID != nil {
		if w, ok := ps.wineByUID[*it.UID]; ok {
			return w.State == "finished"
		}
	}
	if e, ok := catalogue.Lookup(it.ID, it.Name); !ok || e.Key != "wine_bottle" {
		return false
	}
	if slices.ContainsFunc(it.types(), func(t string) bool { return t == "substance" || t == "alcohol" }) {
		return true
	}
	f := strings.ToLower(featureText(it))
	return slices.ContainsFunc(wineGradeWords, func(w string) bool { return strings.Contains(f, w) })
}

func (ps *prodState) isBusyBottle(it item) bool {
	if it.UID == nil {
		return false
	}
	w, ok := ps.wineByUID[*it.UID]
	return ok && w.State == "fermenting"
}

// ---- wine ----

// wineTier is the grade a score gives (PRODUCTION.md §2): tier, name, value at finish.
func wineTier(score int) (int, string, int) {
	switch {
	case score >= 6:
		return 5, "Master Vintage", 175
	case score == 5:
		return 4, "Reserve", 150
	case score >= 3:
		return 3, "Production", 125
	case score == 2:
		return 2, "Homebrew", 100
	case score == 1:
		return 1, "Bucket brew", 85
	}
	return 0, "Undrinkable", 0
}

// agingPerNight is +3 base value a night times the grade's innate % (PRODUCTION.md §2,
// Aging), in hundredths.
var agingPerNight = map[int]int{5: 525, 4: 450, 3: 375, 2: 300, 1: 255}

const berryFoodValue = 14

// recipe is one bottle's fill: berry grades by index (0 best, 1 normal, 2 under/over).
type recipe struct {
	berries [3]int
	yeast   bool
}

func (r recipe) score(water int) int {
	n := r.berries[0] + r.berries[1] + r.berries[2]
	sc := 2*r.berries[0] + r.berries[1] + water
	if n >= 2 {
		sc++
	}
	if r.yeast {
		sc++
	}
	return sc
}

func (r recipe) text() string {
	var parts []string
	for i, name := range []string{"best", "normal", "under/over-ripe"} {
		for range r.berries[i] {
			parts = append(parts, name)
		}
	}
	t := strings.Join(parts, "+")
	if r.yeast {
		t += " + yeast"
	} else {
		t += ", no yeast"
	}
	return t
}

var recipes = func() []recipe {
	var out []recipe
	for _, y := range []bool{true, false} {
		for a := range 3 {
			out = append(out, recipe{yeast: y})
			out[len(out)-1].berries[a]++
			for c := a; c < 3; c++ {
				r := recipe{yeast: y}
				r.berries[a]++
				r.berries[c]++
				out = append(out, r)
			}
		}
	}
	return out
}()

type wineKey struct{ b, n, m, y, k int }

type winePlan struct {
	value int
	fills []recipe
}

// bestFill chooses fills for up to k bottles from b best, n normal and m under/over-ripe
// berries and y yeasts, maximising wine value at finish plus 14 for every berry left to
// sell as food. Two berries per bottle at most: whether a third fits the bottle's grid
// is unverified (PRODUCTION.md, Riskiest).
func bestFill(b, n, m, y, k, water int) winePlan {
	k = min(k, 8)
	surplus := berryFoodValue * (max(b-2*k, 0) + max(n-2*k, 0) + max(m-2*k, 0))
	b, n, m, y = min(b, 2*k), min(n, 2*k), min(m, 2*k), min(y, k)
	memo := map[wineKey]winePlan{}
	var solve func(key wineKey) winePlan
	solve = func(key wineKey) winePlan {
		if p, ok := memo[key]; ok {
			return p
		}
		best := winePlan{value: berryFoodValue * (key.b + key.n + key.m)}
		if key.k > 0 {
			for _, r := range recipes {
				if r.berries[0] > key.b || r.berries[1] > key.n || r.berries[2] > key.m || (r.yeast && key.y == 0) {
					continue
				}
				next := wineKey{key.b - r.berries[0], key.n - r.berries[1], key.m - r.berries[2], key.y, key.k - 1}
				if r.yeast {
					next.y--
				}
				_, _, v := wineTier(r.score(water))
				sub := solve(next)
				if v+sub.value > best.value {
					best = winePlan{value: v + sub.value, fills: append([]recipe{r}, sub.fills...)}
				}
			}
		}
		memo[key] = best
		return best
	}
	p := solve(wineKey{b, n, m, y, k})
	p.value += surplus
	return p
}

func (p winePlan) describe(water int, nights string) string {
	counts := map[string]int{}
	var order []string
	for _, r := range p.fills {
		_, name, v := wineTier(r.score(water))
		t := fmt.Sprintf("%s -> %s %d", r.text(), name, v)
		if counts[t] == 0 {
			order = append(order, t)
		}
		counts[t]++
	}
	var parts []string
	for _, t := range order {
		parts = append(parts, fmt.Sprintf("%d x (%s)", counts[t], t))
	}
	return strings.Join(parts, ", ") + nights
}

func (ps *prodState) renderWine(b *strings.Builder, s *snapshot, day int) {
	nb, nn := len(ps.berries["best"]), len(ps.berries["normal"])+len(ps.berries[""])
	nm := len(ps.berries["under"]) + len(ps.berries["over"])
	moldy := ps.berries["moldy"]
	bottles := len(ps.emptyBottles)
	yeast := ps.yeast["wine_yeast"] + ps.yeast["wine_superyeast"]
	if ps.yeast["wine_yeast_infinite"] > 0 {
		yeast = max(bottles, 1)
	}
	var fermenting, finished []string
	for _, w := range ps.wine {
		switch w.State {
		case "fermenting":
			t := "bottle" + uidText(w.UID)
			if w.NightsLeft != nil {
				t += fmt.Sprintf(" ready in %d night(s) (day %d)", *w.NightsLeft, day+int(*w.NightsLeft))
			}
			fermenting = append(fermenting, t)
		case "finished":
			finished = append(finished, wineText(w))
		}
	}
	for _, it := range s.Inventory {
		if ps.isFinishedWine(it) {
			if _, ok := ps.wineByUID[uidOf(it)]; !ok {
				t := itemName(it) + uidText(it.UID)
				if f := featureText(it); f != "" {
					t += " " + f
				}
				if it.EstimatedValue != nil {
					t += fmt.Sprintf(", est. %d", *it.EstimatedValue)
				}
				finished = append(finished, t)
			}
		}
	}
	berriesSeen := nb+nn+nm+len(moldy) > 0
	if !berriesSeen && bottles == 0 && len(fermenting) == 0 && len(finished) == 0 && yeast == 0 {
		return
	}
	var stock []string
	stock = append(stock, fmt.Sprintf("%d empty bottle(s)%s", bottles, uidList(ps.emptyBottles)))
	ytext := fmt.Sprintf("%d yeast", ps.yeast["wine_yeast"])
	if n := ps.yeast["wine_superyeast"]; n > 0 {
		ytext += fmt.Sprintf(", %d superyeast", n)
	}
	if ps.yeast["wine_yeast_infinite"] > 0 {
		ytext += ", a reusable yeast bioreactor"
	}
	stock = append(stock, ytext)
	if berriesSeen {
		var bs []string
		for _, g := range []string{"best", "normal", "", "under", "over", "moldy"} {
			if us := ps.berries[g]; len(us) > 0 {
				bs = append(bs, fmt.Sprintf("%d %s%s", len(us), berryGradeName[g], uidList(us)))
			}
		}
		stock = append(stock, "berries: "+strings.Join(bs, ", "))
	}
	fmt.Fprintf(b, "- Wine: %s.\n", strings.Join(stock, "; "))
	if berriesSeen && nb+nn+nm > 0 {
		if bottles > 0 {
			now := bestFill(nb, nn, nm, yeast, bottles, 0)
			nights := " (ferments 4 nights without yeast, 2 with)"
			fmt.Fprintf(b, "  Best fill of the %d bottle(s) tonight: %s; wine %d + berries left as food.\n", bottles, now.describe(0, nights), now.value)
			if yeast < bottles {
				with := bestFill(nb, nn, nm, bottles, bottles, 0)
				fmt.Fprintf(b, "  With one wine_yeast (10) per bottle instead: %s, worth %d (+%d) and done in 2 nights.\n", with.describe(0, ""), with.value, with.value-now.value)
			}
		}
		more := bestFill(nb, nn, nm, yeast+1, bottles+1, 0).value - bestFill(nb, nn, nm, yeast, bottles, 0).value
		if more > 22 {
			fmt.Fprintf(b, "  Bottles and yeast are the limit: each extra bottle + yeast (12 + 10) adds about %d from these berries.\n", more)
		}
		if nm > 0 {
			fmt.Fprintf(b, "  SUGGESTION: under- and over-ripe berries%s score 0 (normal +1) but a lone one with yeast still makes Bucket brew 85 and one beside a better berry adds the 2-berry +1; selling them as food gets 14. Sell them as food only when no bottle and yeast is free for them.\n",
				uidList(append(append([]int64{}, ps.berries["under"]...), ps.berries["over"]...)))
		}
	}
	if bottles < nb+nn+nm || yeast < bottles {
		src := "bottles and yeast come from the retired winemaker visitor (4 bottles, yeast) and the Winemaker upgrade (200: 3 yeast, 3 bottles, 2 berries)"
		for _, u := range s.UpgradesOffer {
			if strings.Contains(strings.ToLower(u.name()), "winemaker") {
				src += "; the Winemaker upgrade is on offer now"
			}
		}
		b.WriteString("  Where to get more: " + src + ".\n")
	}
	if len(moldy) > 0 {
		fmt.Fprintf(b, "  Moldy berries%s: food only (-1 in a bottle costs a grade).\n", uidList(moldy))
	}
	rations := 0
	for _, it := range s.Inventory {
		if e, ok := catalogue.Lookup(it.ID, it.Name); ok && e.Key == "water_ration" && !strings.Contains(it.ID, "small") {
			rations++
		}
	}
	if rations > 0 && bottles > 0 {
		fmt.Fprintf(b, "  Water rations on hand: %d (750 ml of Basewater each: drop one on a bottle to fill it).\n", rations)
	}
	b.WriteString("  Water in a bottle: Basewater or a ration 0, tested Pure or High Quality +1, Ghostwater -1, Rustwater -2, Gutterflow -3; 749 ml or more.\n")
	if len(fermenting) > 0 {
		fmt.Fprintf(b, "  Fermenting: %s.\n", strings.Join(fermenting, "; "))
	}
	if len(finished) > 0 {
		fmt.Fprintf(b, "  Finished wine (ages anywhere, no cap; no market event moves alcohol): %s.\n", strings.Join(finished, "; "))
	}
}

func wineText(w wineBottle) string {
	t := "bottle" + uidText(w.UID)
	if w.Counterfeit != nil && *w.Counterfeit {
		return t + " counterfeit gloomberry (contraband 2, never ages)"
	}
	if w.Grade != "" {
		t += " " + w.Grade
	}
	if w.Age != nil {
		t += fmt.Sprintf(", aged %d night(s)", *w.Age)
	}
	if w.Value != nil {
		t += fmt.Sprintf(", value %d", *w.Value)
	}
	if w.Tier != nil {
		if g := agingPerNight[int(*w.Tier)]; g > 0 {
			t += fmt.Sprintf(", +%s a night", fmtNum(float64(g)/100))
		}
	}
	return t
}

func uidText(u *int64) string {
	if u == nil {
		return ""
	}
	return fmt.Sprintf(" #%d", *u)
}

func uidList(us []int64) string {
	var b strings.Builder
	for i, u := range us {
		if i == 6 {
			b.WriteString(" …")
			break
		}
		if u >= 0 {
			fmt.Fprintf(&b, " #%d", u)
		}
	}
	return b.String()
}

// ---- furnace ----

type furnaceLoad struct {
	uid                                 *int64
	scrap, ore, junk, flux, advFlux     int
	blast, junkModule, power, powerSeen bool
	battery                             int64
	quality                             int64
}

// furnaceLoads describes every furnace: from the furnaces block, else the machine entry
// and the inventory items inside it.
func (s *snapshot) furnaceLoads(ps *prodState) []furnaceLoad {
	var out []furnaceLoad
	for _, f := range ps.furnaces {
		l := furnaceLoad{uid: f.UID}
		if in := f.Inputs; in != nil {
			l.scrap, l.ore, l.junk, l.flux, l.advFlux = n64(in.Scrap), n64(in.Ore), n64(in.Junk), n64(in.Flux), n64(in.AdvFlux)
		}
		l.blast = f.Blast != nil && *f.Blast
		l.junkModule = f.JunkModule != nil && *f.JunkModule
		if f.BatteryCharge != nil {
			l.battery, l.powerSeen = *f.BatteryCharge, true
		}
		if f.QualityBonus != nil {
			l.quality = *f.QualityBonus
		}
		for _, m := range ps.machines {
			if m.kind() == "furnace" && m.UID != nil && f.UID != nil && *m.UID == *f.UID {
				l.fromMachine(m)
			}
		}
		out = append(out, l)
	}
	if len(out) > 0 {
		return out
	}
	seen := map[string]bool{}
	add := func(uid *int64, m *prodMachine) {
		l := furnaceLoad{uid: uid}
		if m != nil {
			l.fromMachine(*m)
		}
		if uid != nil {
			key := fmt.Sprint(*uid)
			if seen[key] {
				return
			}
			seen[key] = true
			for _, it := range s.Inventory {
				if it.Where != "inside:"+key {
					continue
				}
				if e, ok := catalogue.Lookup(it.ID, it.Name); ok {
					switch e.Key {
					case "scrap_metal":
						l.scrap++
					case "common_ore":
						l.ore++
					case "junk":
						l.junk++
					case "flux_agent":
						l.flux++
					case "advanced_flux_agent":
						l.advFlux++
					case "furnace_module_blast":
						l.blast = true
					case "furnace_module_junk":
						l.junkModule = true
					}
				}
			}
		}
		out = append(out, l)
	}
	for i := range ps.machines {
		if ps.machines[i].kind() == "furnace" && ps.machines[i].legacy == nil {
			add(ps.machines[i].UID, &ps.machines[i])
		}
	}
	for _, it := range s.Inventory {
		if machineKind(it.ID) == "furnace" {
			add(it.UID, nil)
		}
	}
	return out
}

func (l *furnaceLoad) fromMachine(m prodMachine) {
	if m.BatteryCharge != nil {
		l.battery, l.powerSeen = *m.BatteryCharge, true
	}
	if m.Powered != nil {
		l.powerSeen, l.power = true, *m.Powered
	}
	if m.QualityBonus != nil {
		l.quality = *m.QualityBonus
	}
	l.blast = l.blast || m.hasModule("furnace_module_blast")
	l.junkModule = l.junkModule || m.hasModule("furnace_module_junk")
}

func n64(p *int64) int {
	if p == nil {
		return 0
	}
	return int(*p)
}

// ingotPurity is IngotPurityHelper's level for one smelt (PRODUCTION.md §3): LOW (6) +
// 2*ore/inputs + the flux level, clamped to VERY_LOW (5)..PERFECT (10).
func ingotPurity(ore, inputs int, flux bool, blast bool, quality int64) int {
	fl := int(quality / 100)
	if blast {
		fl--
	}
	if flux {
		fl = max(fl, 1)
	}
	return min(max(6+2*ore/inputs+fl, 5), 10)
}

var ingotName = map[int]string{10: "Perfect", 9: "Very high", 8: "High", 7: "Fair", 6: "Low", 5: "Very low"}

// ingotValue is the 50 base times the purity's innate %, in tenths of a credit.
var ingotValue = map[int]int{10: 1500, 9: 750, 8: 625, 7: 500, 6: 425, 5: 375}

func tenths(v int) string { return fmtNum(float64(v) / 10) }

func (s *snapshot) renderFurnaces(b *strings.Builder, ps *prodState, day int) {
	scrapS, oreS, fluxS, junkS := ps.stock["scrap_metal"], ps.stock["common_ore"], ps.stock["flux_agent"], ps.stock["junk"]
	for _, l := range s.furnaceLoads(ps) {
		per := 2
		if l.blast {
			per = 3
		}
		out := per - 1
		fmt.Fprintf(b, "- Furnace%s", uidText(l.uid))
		var mods []string
		if l.blast {
			mods = append(mods, "blast module: 3 in -> 2 ingots")
		}
		if l.junkModule {
			mods = append(mods, "junk module")
		}
		if len(mods) > 0 {
			fmt.Fprintf(b, " (%s)", strings.Join(mods, ", "))
		}
		fmt.Fprintf(b, ": loaded %d scrap, %d ore, %d flux", l.scrap, l.ore, l.flux)
		if l.advFlux > 0 {
			b.WriteString(", an advanced flux (never consumed)")
		}
		fmt.Fprintf(b, "; in stock %d scrap, %d ore, %d flux", scrapS, oreS, fluxS)
		if junkS > 0 {
			fmt.Fprintf(b, ", %d junk", junkS)
		}
		b.WriteString(".")
		need := int64(8)
		if l.blast {
			need = 16
		}
		if l.powerSeen && (l.battery < need && !l.power) {
			fmt.Fprintf(b, " NO POWER for a smelt (%d a smelt): put a charged energy credit in its battery or nothing melts tonight.", need)
		} else if l.powerSeen && l.battery > 0 {
			fmt.Fprintf(b, " Battery %d (%d a smelt).", l.battery, need)
		}
		metal := l.scrap + l.ore + scrapS + oreS
		fmt.Fprintf(b, " One smelt a night from %d loaded metal: ", per)
		if l.scrap+l.ore >= per {
			b.WriteString("tonight's smelt is loaded")
		} else {
			fmt.Fprintf(b, "load %d more tonight or it idles", per-(l.scrap+l.ore))
		}
		fmt.Fprintf(b, "; the metal on hand lasts %d night(s).\n", metal/per)
		flux := l.flux+fluxS > 0 || l.advFlux > 0
		var opts []string
		for _, ore := range []int{0, 1, 2} {
			if l.blast && ore == 2 {
				ore = 3
			}
			if ore > l.ore+oreS || per-ore > l.scrap+scrapS && ore < per {
				continue
			}
			name := map[int]string{0: fmt.Sprintf("%d scrap", per), 1: "scrap+ore", 2: "2 ore", 3: "3 ore"}[ore]
			if l.blast && ore == 1 {
				name = "2 scrap+ore"
			}
			p := ingotPurity(ore, per, false, l.blast, l.quality)
			t := fmt.Sprintf("%s -> %s %s", name, ingotName[p], tenths(ingotValue[p]*out))
			pf := ingotPurity(ore, per, true, l.blast, l.quality)
			if pf != p {
				t += fmt.Sprintf(" (%s %s with flux)", ingotName[pf], tenths(ingotValue[pf]*out))
			}
			opts = append(opts, t)
		}
		if len(opts) > 0 {
			fmt.Fprintf(b, "  Ingots (MATERIAL; material events move them): %s.", strings.Join(opts, "; "))
			if !flux {
				b.WriteString(" No flux on hand: a flux_agent (4, consumed) lifts each ingot one purity level (+7.5 to +12.5).")
			}
			b.WriteString(" Scrap sells for 10, ore 15: both are worth more melted.\n")
		}
		if start, pct, ok := s.nextRise("material", day); ok && metal >= per {
			nights := int(start) - day
			n := min(nights, metal/per) * out
			fmt.Fprintf(b, "  Material +%d%% from day %d: smelting every night from tonight gives %d ingot(s) by the morning of day %d to sell into it.\n", pct, start, n, start)
		}
		if junkS > 0 && !l.junkModule {
			b.WriteString("  Junk has no furnace use without the junk module.\n")
		}
	}
}

// ---- water ----

// waterGrades by index (WATER.md §1.3): name, value per litre in tenths.
var waterGrades = []struct {
	name  string
	perL  int
	minBP int64
}{{"Pure", 800, 9990}, {"High Quality", 600, 9900}, {"Basewater", 400, 9800}, {"Ghostwater", 268, 9600}, {"Rustwater", 100, 9000}, {"Gutterflow", 40, 0}}

func gradeFromBP(bp int64) int {
	for i, g := range waterGrades {
		if bp >= g.minBP {
			return i
		}
	}
	return 5
}

// gradeIndex reads a grade given as an index, a name ("Basewater", "High Quality Water",
// "pure"…) or a "Last scan: 98.12 % Basewater" line; -1 when unreadable.
func gradeIndex(raw json.RawMessage) int {
	if len(raw) == 0 {
		return -1
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n >= 0 && n <= 5 {
		return int(n)
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return -1
	}
	return gradeName(s)
}

func gradeName(s string) int {
	s = strings.ToLower(s)
	switch {
	case strings.Contains(s, "high") || strings.Contains(s, "hq"):
		return 1
	case strings.Contains(s, "pure"):
		return 0
	case strings.Contains(s, "base"):
		return 2
	case strings.Contains(s, "ghost"):
		return 3
	case strings.Contains(s, "rust"):
		return 4
	case strings.Contains(s, "gutter"):
		return 5
	}
	return -1
}

func (w waterContainer) label() int {
	if g := gradeIndex(w.LabelGrade); g >= 0 {
		return g
	}
	if w.LabelBP != nil {
		return gradeFromBP(*w.LabelBP)
	}
	return -1
}

// truth is the tested grade: only once a strip or scan has shown it to the player.
func (w waterContainer) truth() int {
	if w.LastScan != "" {
		if g := gradeName(w.LastScan); g >= 0 {
			return g
		}
	}
	tested := (w.Exposed != nil && *w.Exposed) || (w.Discovered != nil && *w.Discovered)
	if tested {
		return gradeIndex(w.TrueGrade)
	}
	return -1
}

func (w waterContainer) parentText() string {
	if len(w.Parent) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(w.Parent, &s) == nil {
		return s
	}
	var m map[string]any
	if json.Unmarshal(w.Parent, &m) == nil {
		t := firstStr(m, "name", "id", "machine")
		if r := firstStr(m, "slot", "role"); r != "" {
			t += " " + r
		}
		return t
	}
	return ""
}

// treating is the machine that changes this container's water tonight, if any.
func (w waterContainer) treating() string {
	p := strings.ToLower(w.parentText())
	for _, k := range []string{"purifier", "evaporator", "moisture", "heating"} {
		if strings.Contains(p, k) {
			return k
		}
	}
	return ""
}

func (w waterContainer) ml() (int64, bool) {
	if w.VolumeUL == nil {
		return 0, false
	}
	return *w.VolumeUL / 1000, true
}

func (w waterContainer) worth(grade int) (int64, bool) {
	ml, ok := w.ml()
	if !ok || grade < 0 {
		return 0, false
	}
	return ml * int64(waterGrades[grade].perL) / 10000, true
}

func (w waterContainer) filterWorn() bool {
	f := w.Filter
	if f == nil {
		return false
	}
	if strings.Contains(strings.ToLower(f.State), "worn") {
		return true
	}
	return f.Durability != nil && *f.Durability <= 40
}

func (ps *prodState) waterConditions(s *snapshot) {
	for _, w := range ps.water {
		g := max(w.label(), w.truth())
		if ml, ok := w.ml(); g >= 3 && (!ok || ml > 0) {
			ps.has["water_to_treat"] = true
		}
		if w.filterWorn() {
			ps.has["water_to_treat"] = true
		}
		if l, t := w.label(), w.truth(); (t >= 0 && l >= 0 && t != l) || w.treating() != "" || (w.Stale != nil && *w.Stale) {
			ps.has["water_stale"] = true
		}
		if strings.Contains(strings.ToLower(w.parentText()), "ras") {
			ps.has["ras"] = true
		}
	}
	if len(ps.taps) > 0 {
		ps.has["water_to_treat"] = true
	}
	for _, m := range ps.machines {
		if strings.Contains(strings.ToLower(m.StatusText), "worn") {
			ps.has["water_to_treat"] = true
		}
	}
	for _, it := range s.Inventory {
		if !slices.Contains(it.types(), "water") {
			continue
		}
		if g := gradeName(itemName(it) + " " + featureText(it)); g >= 3 || strings.Contains(strings.ToLower(itemName(it)), "dirty") {
			ps.has["water_to_treat"] = true
		}
	}
}

func (ps *prodState) renderWater(b *strings.Builder, s *snapshot) {
	if len(ps.water) == 0 && len(ps.taps) == 0 {
		return
	}
	b.WriteString("- Water (sold at its LABEL until tested; filters, the purifier, heat and tablets change the water but not the label; pouring into a non-empty container keeps that container's label):\n")
	byUID := map[int64]item{}
	for _, it := range s.Inventory {
		if it.UID != nil {
			byUID[*it.UID] = it
		}
	}
	for _, w := range ps.water {
		name := nonEmpty(w.Name, w.ID)
		if w.UID != nil {
			if it, ok := byUID[*w.UID]; ok {
				name = itemName(it)
			}
		}
		t := "  " + nonEmpty(name, "container") + uidText(w.UID)
		if p := w.parentText(); p != "" {
			t += " in " + p
		}
		if ml, ok := w.ml(); ok {
			t += fmt.Sprintf(", %s L", fmtNum(float64(ml)/1000))
		} else if w.VolumeBand != "" {
			t += ", " + w.VolumeBand
		}
		l, tr := w.label(), w.truth()
		if l >= 0 {
			t += ", label " + waterGrades[l].name
			if v, ok := w.worth(l); ok {
				t += fmt.Sprintf(" (sells as ~%d)", v)
			}
		}
		if w.LastScan != "" {
			t += "; " + plainLog(w.LastScan)
		} else if tr >= 0 {
			t += "; tested " + waterGrades[tr].name
		}
		switch {
		case l >= 0 && tr >= 0 && tr > l:
			t += fmt.Sprintf(" -> OVER-LABELLED: checking customers expose it (-1 rep, price falls to %s)", waterGrades[tr].name)
			if tr == 3 || (tr <= 2 && l < tr) {
				t += " and each sale adds hidden crime (dirty or mislabelled water)"
			}
			t += "; relabel to the truth (strip or AquaScan Match) before selling or showing it"
		case l >= 0 && tr >= 0 && tr < l:
			t += fmt.Sprintf(" -> UNDER-LABELLED: sells as %s", waterGrades[l].name)
			if v, ok := w.worth(tr); ok {
				t += fmt.Sprintf(" though it is %s (~%d)", waterGrades[tr].name, v)
			}
			t += "; relabel it (strip or AquaScan Match) before selling"
		case w.Stale != nil && *w.Stale:
			t += " -> label STALE: test before selling"
		case tr < 0 && l >= 0 && w.treating() == "" && w.Filter == nil:
			t += ", untested"
		}
		if k := w.treating(); k != "" {
			t += fmt.Sprintf("; the %s changes it tonight and leaves the label stale: test it before selling", k)
			if w.MaxPurityBP != nil {
				t += fmt.Sprintf(" (purifier cap %s %%, %s)", fmtNum(float64(*w.MaxPurityBP)/100), waterGrades[gradeFromBP(*w.MaxPurityBP)].name)
			}
		}
		if f := w.Filter; f != nil {
			ft := nonEmpty(f.Name, nonEmpty(f.ID, "a filter"))
			if f.Durability != nil {
				ft += fmt.Sprintf(" durability %d", *f.Durability)
				if f.Max != nil {
					ft += fmt.Sprintf("/%d", *f.Max)
				}
			}
			if w.filterWorn() {
				ft += " (WORN: about 37 durability per litre of rust water; " + s.spareFilterText() + ")"
			}
			t += "; filter " + ft + "; pours through it leave the label stale"
		}
		b.WriteString(t + ".\n")
	}
	if ps.has["hydroponic"] || ps.has["wine_bottles"] || ps.has["finished_wine"] {
		b.WriteString("  Route water by grade: Basewater (pitcher, portable purifier, tablets) to selling or vending, after testing and matching its label; High Quality or Pure (the purifier machine; Pure needs its quality 100+) to the hydroponic bay and wine bottles (+1 wine score). Bottles and bays judge the true grade, buyers the label.\n")
	}
	if len(ps.taps) > 0 {
		fmt.Fprintf(b, "  Taps: %s (free water: Rustwater 90-91.5%%, Gutterflow 80-90%%, today's purity unknown until tested; filter + 1 chlorine tablet a litre makes Basewater).\n", strings.Join(ps.taps, ", "))
	}
}

// ---- hydroponics, machines, scavenging ----

func (ps *prodState) renderHydro(b *strings.Builder, day int) {
	for _, h := range ps.hydro {
		t := "- Hydroponic bay" + uidText(h.UID) + ": "
		if h.Seed == nil || h.Seed.PlantID == "" {
			t += "EMPTY (plant a seed; growth starts the next night)"
		} else {
			t += strings.ReplaceAll(h.Seed.PlantID, "_", " ")
			if h.Seed.Perennial != nil && *h.Seed.Perennial {
				t += " (perennial: never replant"
				if h.Seed.Established != nil && *h.Seed.Established {
					t += ", established: 12 work to refruit"
				}
				t += ")"
			}
			if h.Progress != nil && h.Target != nil {
				t += fmt.Sprintf(", progress %d/%d", *h.Progress, *h.Target)
			}
			if h.ETA != nil {
				t += fmt.Sprintf(", harvest in %d night(s) (morning of day %d)", *h.ETA, day+int(*h.ETA))
			}
			if h.HarvestCount != nil {
				t += fmt.Sprintf(" of %d", *h.HarvestCount)
			}
		}
		if h.WaterML != nil {
			t += fmt.Sprintf("; water %d ml", *h.WaterML)
			if h.WaterUse != nil && *h.WaterUse > 0 {
				n := *h.WaterML / *h.WaterUse
				t += fmt.Sprintf(", %d ml a night (enough for %d night(s))", *h.WaterUse, n)
				if h.ETA != nil && n < *h.ETA {
					t += "; REFILL before then or it pauses"
				}
			}
		}
		if h.Status != "" {
			t += "; " + h.Status
		}
		b.WriteString(t + ".\n")
		if h.Seed != nil {
			ps.renderBayWater(b, h.Seed.PlantID)
		}
	}
}

// contaminantSensitive plants lose health to every contaminant in their water;
// fillerweed and hydroreed do not (WATER.md §2.3, contaminant ×0).
func contaminantSensitive(plant string) bool {
	return !strings.Contains(plant, "fillerweed") && !strings.Contains(plant, "hydroreed")
}

// renderBayWater judges the water feeding a bay: on Basewater a bloomberry plant's health
// is about 16-44 at harvest, three quarters bad berries (measured, docs/sim/VERIFIED.md).
func (ps *prodState) renderBayWater(b *strings.Builder, plant string) {
	for _, w := range ps.water {
		if !strings.Contains(strings.ToLower(w.parentText()), "hydroponic") {
			continue
		}
		g, how := w.truth(), "tested"
		if g < 0 {
			g, how = w.label(), "label"
		}
		name := nonEmpty(w.Name, w.ID) + uidText(w.UID)
		switch {
		case !contaminantSensitive(plant):
		case g < 0:
			fmt.Fprintf(b, "  Bay water %s: grade unknown; test it: anything below High Quality ruins the plant's health.\n", name)
		case g >= 2:
			fmt.Fprintf(b, "  WATER PROBLEM: the bay drinks %s (%s %s). On Basewater a %s plant's health is about 16 at the first harvest and 44 at later ones (three quarters of the berries bad); on Ghostwater or worse it goes below 0 and every berry is bad. Feed it High Quality or Pure water (the purifier's output) and sell the Basewater instead.\n",
				name, how, waterGrades[g].name, strings.ReplaceAll(plant, "_", " "))
		case how == "label":
			fmt.Fprintf(b, "  Bay water %s is labelled %s but untested: test it, labels go stale after treatment.\n", name, waterGrades[g].name)
		}
	}
	if strings.Contains(plant, "bloomberry") {
		b.WriteString("  Bloomberry wants minerals at 200 ± 75 bp of the water (nutrient tablets, 15 ml each, added AFTER filtering or purifying, which strip minerals): Pure + about 6 tablets per 6 L keeps health at 100 (no bad berries, 62% best, measured); High Quality about 56-70 (a third bad, about a fifth with tablets); Basewater about 16-44 (three quarters bad). Health resets to 100 in the harvest night after the berries roll, so the analyser's growth score the morning after a harvest is the new cycle.\n")
	}
}

func (ps *prodState) renderMachines(b *strings.Builder, day int) {
	for _, m := range ps.machines {
		if m.legacy != nil {
			continue
		}
		if k := m.kind(); k == "furnace" || (k == "hydroponic" && len(ps.hydro) > 0) {
			continue
		}
		t := "- " + nonEmpty(m.Name, nonEmpty(m.ID, nonEmpty(m.Kind, "machine"))) + uidText(m.UID) + ":"
		var parts []string
		if m.State != "" {
			parts = append(parts, m.State)
		}
		if m.Progress != nil && m.Target != nil {
			parts = append(parts, fmt.Sprintf("progress %d/%d", *m.Progress, *m.Target))
		}
		if m.ETA != nil {
			parts = append(parts, fmt.Sprintf("done in %d night(s) (day %d)", *m.ETA, day+int(*m.ETA)))
		}
		if m.Powered != nil && !*m.Powered {
			parts = append(parts, "NO POWER: idle tonight")
		}
		if m.BatteryCharge != nil {
			t := fmt.Sprintf("battery %d", *m.BatteryCharge)
			if m.PowerPerNight != nil {
				t += fmt.Sprintf(" (%d a night)", *m.PowerPerNight)
			}
			parts = append(parts, t)
		}
		if m.MaxPurityBP != nil {
			parts = append(parts, fmt.Sprintf("cleans one container a night up to %s %% (%s)", fmtNum(float64(*m.MaxPurityBP)/100), waterGrades[gradeFromBP(*m.MaxPurityBP)].name))
		}
		if m.OutML != nil {
			t := fmt.Sprintf("makes %d ml a night", *m.OutML)
			if m.OutGrade != "" {
				t += " of " + m.OutGrade
			}
			parts = append(parts, t)
		}
		if len(m.Inputs) > 0 {
			var ids []string
			for _, r := range m.Inputs {
				ids = append(ids, r.ID)
			}
			parts = append(parts, "holds "+strings.Join(ids, ", "))
		}
		if m.StatusText != "" {
			parts = append(parts, plainLog(m.StatusText))
		}
		b.WriteString(t + " " + strings.Join(parts, ", ") + ".\n")
	}
}

func (ps *prodState) renderScavenge(b *strings.Builder) {
	sc := ps.scav
	if sc == nil {
		return
	}
	var parts []string
	if sc.AttemptsLeft != nil {
		parts = append(parts, fmt.Sprintf("%d attempts", *sc.AttemptsLeft))
	}
	if sc.MinorPct != nil && sc.MajorPct != nil {
		parts = append(parts, fmt.Sprintf("wound odds %s%% minor / %s%% major per search (a fresh wound ends the night)", fmtNum(*sc.MinorPct), fmtNum(*sc.MajorPct)))
	}
	if sc.WoundedFresh != nil && *sc.WoundedFresh {
		parts = append(parts, "freshly wounded")
	}
	if sc.ScannerPct != nil {
		parts = append(parts, fmt.Sprintf("double loot %d%%", *sc.ScannerPct))
	}
	if len(parts) > 0 {
		fmt.Fprintf(b, "- Scavenging (one outing a night, after closing): %s.\n", strings.Join(parts, ", "))
	}
}

// renderProduction writes the PRODUCTION state block: per machine, plant and bottle what
// it holds and what it can make, with the code's numbers.
func (s *snapshot) renderProduction(b *strings.Builder, day int) {
	ps := s.production()
	var body strings.Builder
	ps.renderHydro(&body, day)
	ps.renderWine(&body, s, day)
	if ps.has["furnace"] {
		s.renderFurnaces(&body, ps, day)
	}
	ps.renderMachines(&body, day)
	ps.renderWater(&body, s)
	ps.renderScavenge(&body)
	if body.Len() == 0 && ps.other == "" {
		return
	}
	b.WriteString("PRODUCTION (machines, plants and bottles advance only while you sleep, one step a night; a night without sleep is lost, nothing catches up):\n")
	b.WriteString(body.String())
	if ps.other != "" {
		fmt.Fprintf(b, "production: %s\n", ps.other)
	}
}

// cleanWaterUIDs are the containers cleanWaterKeeps says to keep.
func (ps *prodState) cleanWaterUIDs() []int64 {
	if !ps.has["hydroponic"] && !ps.has["wine_bottles"] {
		return nil
	}
	var out []int64
	for _, w := range ps.water {
		if g := w.truth(); g >= 0 && g <= 1 && w.UID != nil && !strings.Contains(strings.ToLower(w.parentText()), "hydroponic") {
			if ml, ok := w.ml(); !ok || ml > 0 {
				out = append(out, *w.UID)
			}
		}
	}
	return out
}

// cleanWaterKeeps are KEEP lines for tested High Quality or Pure water while a bay or
// empty bottles need it: on Basewater a bloomberry plant's berries rot, and a bottle
// loses its +1 (WATER.md §2.3, §2.5).
func (ps *prodState) cleanWaterKeeps(s *snapshot) []string {
	bay, bottles := ps.has["hydroponic"], ps.has["wine_bottles"]
	if !bay && !bottles {
		return nil
	}
	var out []string
	for _, w := range ps.water {
		g := w.truth()
		if g < 0 || g > 1 || strings.Contains(strings.ToLower(w.parentText()), "hydroponic") {
			continue
		}
		if ml, ok := w.ml(); ok && ml == 0 {
			continue
		}
		var uses []string
		if bay {
			uses = append(uses, "the hydroponic bay (bloomberries on Pure + tablets 0% bad, on High Quality about a third bad, on Basewater three quarters bad)")
		}
		if bottles {
			uses = append(uses, "wine bottles (+1 score, one grade, 749 ml each)")
		}
		t := fmt.Sprintf("%s%s (%s by test or scan): feed it to %s", s.nameOf(w.UID, nonEmpty(w.Name, w.ID)), uidText(w.UID), waterGrades[g].name, strings.Join(uses, " and "))
		if v, ok := w.worth(g); ok {
			t += fmt.Sprintf(", before selling it for ~%d", v)
		}
		out = append(out, t+" [WATER.md §2.3, §2.5]")
	}
	return out
}

// spareFilterText names a stock filter to fit in place of a worn one.
func (s *snapshot) spareFilterText() string {
	for _, it := range s.Inventory {
		if s.inMachine(it) || it.Where == "showcase" {
			continue
		}
		if e, ok := catalogue.Lookup(it.ID, it.Name); ok && (e.Key == "water_filter" || e.Key == "water_filter_adv" || e.Key == "hydroreed_filter") {
			return "swap in " + itemName(it) + uidText(it.UID) + " from stock tonight"
		}
	}
	return "swap in a fresh filter"
}

// nextRise is the start day and percentage of the next calendar event that raises type t.
func (s *snapshot) nextRise(t string, day int) (int64, int64, bool) {
	for _, d := range s.Calendar {
		if d.Day <= int64(day) {
			continue
		}
		for _, e := range d.Events {
			for i, et := range e.types {
				if et == t && e.pcts[i] > 0 {
					start := d.Day
					if e.StartDay != nil {
						start = *e.StartDay
					}
					if start > int64(day) {
						return start, e.pcts[i], true
					}
				}
			}
		}
	}
	return 0, 0, false
}

// badBayWater names each contaminant-sensitive bay drinking water known to be Basewater
// or worse.
func (ps *prodState) badBayWater() []string {
	var out []string
	for _, h := range ps.hydro {
		if h.Seed == nil || !contaminantSensitive(h.Seed.PlantID) {
			continue
		}
		for _, w := range ps.water {
			if !strings.Contains(strings.ToLower(w.parentText()), "hydroponic") {
				continue
			}
			g := w.truth()
			if g < 0 {
				g = w.label()
			}
			if g >= 2 {
				out = append(out, fmt.Sprintf("the hydroponic bay%s drinks %s", uidText(h.UID), waterGrades[g].name))
			}
		}
	}
	return out
}

func (ps *prodState) wornFilters() bool {
	for _, w := range ps.water {
		if w.filterWorn() {
			return true
		}
	}
	return false
}
