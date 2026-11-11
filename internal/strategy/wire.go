package strategy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// snapshot is the day_snapshot event's data, normalised from either shape the mod may
// send: the provisional fixture (store.rent_due, security.trafficking_crimes, flat
// market_events) or the one proposed in docs/re/DAY.md §1 (rent{}, property{},
// security.crimes, market_events{active, calendar}, clientele{}, structured services
// and upgrades). Unknown fields are ignored; pointers keep an unread value distinct from
// zero.
type snapshot struct {
	Phase          string
	Day            *int64
	Cash           *int64
	Weekday        string
	NetWorth       *int64
	WildFavor      *int64
	Attractiveness *int64
	RetailMarkup   *int64
	Queue          *int64
	Rent           *rentInfo
	Property       *propertyInfo
	Reputation     map[string]repEntry
	Inventory      []item
	Clientele      *clientele
	Security       *security
	UpgradesOwned  []string
	UpgradesOffer  []upgradeOffer
	// ownedIDs are the owned upgrades' ids, which UpgradesOwned (display names) hides.
	ownedIDs []string
	Services []service
	Active   []marketEvent
	Calendar []calendarDay
	NightLog []string
	Requests []string
	// requestAsks is each request's wanted item, count and credit reward, parallel to
	// Requests.
	requestAsks []requestAsk
	// storeExtra holds scalar store fields this bridge does not know yet, shown verbatim
	// so a field the mod adds reaches the planner without a release.
	storeExtra map[string]string
	// other is every unknown top-level key, as compact text.
	other map[string]string
	// productionRaw and waterRaw are read by production(), which caches its result in prod.
	productionRaw, waterRaw json.RawMessage
	prod                    *prodState
	// scavOpen: the afterhours list offers the dumping ground tonight.
	scavOpen bool
	// aug is the aug presence the strategist rebuilt from the run's history; nil unknown.
	aug *AugTrack
	// wineIdle is the days since a new finished wine appeared; nil unknown or none yet.
	wineIdle *int
	// weekGain is net worth now less a week ago; nil unknown.
	weekGain *int64
	// StartingPerks and RepPerks are the perks block (perk ids); RepPerks nil when absent.
	StartingPerks, RepPerks []string
	// Permit: the business permit is held; nil when the mod does not say.
	Permit *bool
}

type rentInfo struct {
	Rent, Arrears, DaysLeft *int64
	LandlordGrace           *bool
}

// due is what the landlord asks for: rent plus arrears.
func (r *rentInfo) due() (int64, bool) {
	if r == nil || r.Rent == nil {
		return 0, false
	}
	d := *r.Rent
	if r.Arrears != nil {
		d += *r.Arrears
	}
	return d, true
}

type propertyInfo struct {
	Owned                     *bool
	MortgageWeekly, WeeksLeft *int64
	BuyoutPrice               *int64
	BuyoutAffordable          *bool
	OfferDown, OfferWeekly    *int64
	OfferWeeks                *int64
}

type perkRef struct {
	ID        string `json:"id"`
	Threshold *int64 `json:"threshold"`
}

type repEntry struct {
	Value            *float64        `json:"value"`
	Status           string          `json:"status"`
	NextPerk         *perkRef        `json:"next_perk"`
	NextNegativePerk *perkRef        `json:"next_negative_perk"`
	UnlockedRaw      json.RawMessage `json:"unlocked_perks"`
}

// unlocked accepts ["Referral"] or [{"id":"Referral"}].
func (r repEntry) unlocked() []string {
	return names(r.UnlockedRaw)
}

type clientele struct {
	NormalCustomers *int64   `json:"normal_customers"`
	DisplayBuyers   []string `json:"display_buyers"`
	Predictions     []struct {
		Client string `json:"client"`
		Chance *int64 `json:"chance"`
		Item   string `json:"item"`
	} `json:"display_predictions"`
	Discerning *struct {
		Chance   *int64 `json:"chance"`
		Cooldown *int64 `json:"cooldown"`
	} `json:"discerning_shopper"`
}

type crime struct {
	ID     string `json:"id"`
	Amount *int64 `json:"amount"`
}

type security struct {
	Visible           *bool
	Crimes            []crime
	Evidence          string
	Trafficking       *int64
	Suspected         string
	ProjectedSentence string
	CaseOfficer       string
	Action            string
	Operation         string
	Note              string
	extra             map[string]string
}

type service struct {
	ID, Name string
	Cost     *int64
	// Using is nil when the mod sends a bare id list, which means in use.
	Using *bool
	// Unlocked is nil when the mod does not say.
	Unlocked *bool
}

func (s service) inUse() bool { return s.Using == nil || *s.Using }

type marketEvent struct {
	Name     string
	Mods     []string
	StartDay *int64
	Duration *int64
	Innate   bool
	Threat   bool
	ID       string
	// types and pcts are the event's modifiers: lower-cased item type, value change in %.
	types []string
	pcts  []int64
}

type calendarDay struct {
	Day    int64
	Events []marketEvent
}

type item struct {
	UID  *int64 `json:"uid"`
	ID   string `json:"id"`
	Name string `json:"name"`
	// Type is the item type market events and the window's buyer list name; the mod
	// may send it as type or item_type.
	Type           string   `json:"type"`
	ItemType       string   `json:"item_type"`
	TypeList       []string `json:"types"`
	Where          string   `json:"where"`
	EstimatedValue *int64   `json:"estimated_value"`
	BoughtAt       *int64   `json:"bought_at"`
	Features       []struct {
		Text string `json:"text"`
	} `json:"features"`
	StolenHeat        *int64 `json:"stolen_heat"`
	ContrabandLevel   *int64 `json:"contraband_level"`
	SeriousContraband *bool  `json:"serious_contraband"`
	// Capacity is the grid inside a container (smuggler bay, bag, machine), in cells.
	Capacity *struct {
		Cells *int64 `json:"cells"`
		Free  *int64 `json:"free"`
	} `json:"capacity"`
}

// types are the item's lower-cased types; the mod may send type, item_type or types.
func (it item) types() []string {
	var out []string
	for _, t := range append([]string{it.Type, it.ItemType}, it.TypeList...) {
		if t = strings.ToLower(t); t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func (it item) itemType() string { return strings.Join(it.types(), "/") }

// displayable is clean stock: stolen or contraband goods in the window bring an
// inspection (docs/re/WINDOW.md, Risks).
func (it item) displayable() bool { return !it.stolen() && !it.contraband() }

func (it item) stolen() bool     { return it.StolenHeat != nil }
func (it item) contraband() bool { return it.ContrabandLevel != nil && *it.ContrabandLevel > 0 }

// seenBySecurity: a patrolling officer who sees stolen or contraband goods here sets heat
// to 100 and starts an inspection (docs/re/DAY.md, Security patrol).
func (it item) seenBySecurity() bool {
	return slices.Contains([]string{"showcase", "counter", "back_counter", "front_counter"}, it.Where)
}

type wire struct {
	Phase string                     `json:"phase"`
	Store map[string]json.RawMessage `json:"store"`
	Rent  *struct {
		Rent          *int64 `json:"rent"`
		Arrears       *int64 `json:"arrears"`
		DaysLeft      *int64 `json:"days_left"`
		LandlordGrace *bool  `json:"landlord_grace"`
	} `json:"rent"`
	Property *struct {
		Owned    *bool `json:"owned"`
		Mortgage *struct {
			Weekly    *int64 `json:"weekly"`
			WeeksLeft *int64 `json:"weeks_left"`
		} `json:"mortgage"`
		BuyoutPrice      *int64 `json:"buyout_price"`
		BuyoutAffordable *bool  `json:"buyout_affordable"`
		MortgageOffer    *struct {
			DownPayment *int64 `json:"down_payment"`
			Weekly      *int64 `json:"weekly"`
			Weeks       *int64 `json:"weeks"`
		} `json:"mortgage_offer"`
	} `json:"property"`
	Reputation   map[string]repEntry        `json:"reputation"`
	Inventory    []item                     `json:"inventory"`
	Clientele    *clientele                 `json:"clientele"`
	Security     map[string]json.RawMessage `json:"security"`
	Upgrades     json.RawMessage            `json:"upgrades"`
	Services     json.RawMessage            `json:"services"`
	MarketEvents json.RawMessage            `json:"market_events"`
	NightLog     []string                   `json:"night_log"`
	Requests     []map[string]any           `json:"requests"`
	Perks        *struct {
		Starting   []string `json:"starting"`
		Reputation []string `json:"reputation"`
	} `json:"perks"`
}

// knownTop are the data keys parseSnapshot reads; any other key (production, machines,
// afterhours…) is rendered generically until its shape is settled.
var knownTop = []string{"phase", "store", "rent", "property", "reputation", "inventory", "clientele", "security", "upgrades", "services", "market_events", "night_log", "requests", "production", "water", "perks"}

var knownStore = []string{"cash", "day", "weekday", "wild_favor", "net_worth", "attractiveness", "retail_markup", "queue_length", "rent_due", "rent_days_left", "commissary_tokens", "business_permit", "newspaper_unlocked"}

func parseSnapshot(raw json.RawMessage) (*snapshot, error) {
	var w wire
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil, err
	}
	s := &snapshot{Phase: w.Phase, Reputation: w.Reputation, Inventory: w.Inventory, Clientele: w.Clientele, NightLog: w.NightLog}
	if p := w.Perks; p != nil {
		s.StartingPerks = p.Starting
		s.RepPerks = p.Reputation
		if s.RepPerks == nil {
			s.RepPerks = []string{}
		}
	}

	st := w.Store
	s.Day, s.Cash = intField(st, "day"), intField(st, "cash")
	s.Weekday = strField(st, "weekday")
	s.NetWorth, s.WildFavor = intField(st, "net_worth"), intField(st, "wild_favor")
	s.RetailMarkup, s.Queue = intField(st, "retail_markup"), intField(st, "queue_length")
	s.Permit = boolField(st, "business_permit")
	if a := intField(st, "attractiveness"); a != nil {
		s.Attractiveness = a
	} else if obj := objField(st, "attractiveness"); obj != nil {
		s.Attractiveness = intField(obj, "total")
	}
	s.storeExtra = scalarExtras(st, knownStore)
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) == nil {
		s.productionRaw, s.waterRaw = top["production"], top["water"]
		s.scavOpen = dumpingGroundOpen(top["afterhours"])
		for k, v := range top {
			if slices.Contains(knownTop, k) {
				continue
			}
			if t := compactText(v); t != "" {
				if s.other == nil {
					s.other = map[string]string{}
				}
				s.other[k] = t
			}
		}
	}

	switch {
	case w.Rent != nil:
		s.Rent = &rentInfo{Rent: w.Rent.Rent, Arrears: w.Rent.Arrears, DaysLeft: w.Rent.DaysLeft, LandlordGrace: w.Rent.LandlordGrace}
	case intField(st, "rent_due") != nil:
		s.Rent = &rentInfo{Rent: intField(st, "rent_due"), DaysLeft: intField(st, "rent_days_left")}
	}
	if p := w.Property; p != nil {
		s.Property = &propertyInfo{Owned: p.Owned, BuyoutPrice: p.BuyoutPrice, BuyoutAffordable: p.BuyoutAffordable}
		if m := p.Mortgage; m != nil {
			s.Property.MortgageWeekly, s.Property.WeeksLeft = m.Weekly, m.WeeksLeft
		}
		if o := p.MortgageOffer; o != nil {
			s.Property.OfferDown, s.Property.OfferWeekly, s.Property.OfferWeeks = o.DownPayment, o.Weekly, o.Weeks
		}
	}
	s.Security = parseSecurity(w.Security)
	s.UpgradesOwned, s.UpgradesOffer, s.ownedIDs = parseUpgrades(w.Upgrades)
	s.Services = parseServices(w.Services)
	s.Active, s.Calendar = parseMarket(w.MarketEvents)
	for _, r := range w.Requests {
		if t := requestText(r); t != "" {
			s.Requests = append(s.Requests, t)
			s.requestAsks = append(s.requestAsks, requestAsk{item: firstStr(r, "required_item", "item"), count: numPtr(firstOf(r, "required_item_count", "count")), reward: numPtr(r["credit_reward"])})
		}
	}
	return s, nil
}

// day is the snapshot's own day, else fallback (the event's game_day).
func (s *snapshot) day(fallback int) int {
	if s.Day != nil && *s.Day > 0 {
		return int(*s.Day)
	}
	return fallback
}

func (s *snapshot) rep(id string) (float64, bool) {
	r, ok := s.Reputation[id]
	if !ok || r.Value == nil {
		return 0, false
	}
	return *r.Value, true
}

func (s *snapshot) earlyWarning() bool {
	return slices.ContainsFunc(s.Services, func(v service) bool {
		id := strings.ToLower(v.ID)
		return v.inUse() && (id == "sec_informant" || id == "early_warning")
	})
}

func parseSecurity(m map[string]json.RawMessage) *security {
	if m == nil {
		return nil
	}
	sec := &security{
		Visible:           boolField(m, "visible"),
		Trafficking:       intField(m, "trafficking_value"),
		Suspected:         strField(m, "suspected"),
		ProjectedSentence: strField(m, "projected_sentence"),
		CaseOfficer:       strField(m, "case_officer"),
		Action:            strField(m, "recommended_action"),
		Operation:         strField(m, "operation"),
		Note:              strField(m, "note"),
	}
	if sec.Trafficking == nil {
		sec.Trafficking = intField(m, "trafficking_crimes")
	}
	sec.Evidence = strField(m, "evidence")
	if sec.Evidence == "" {
		if n := intField(m, "evidence"); n != nil {
			sec.Evidence = fmt.Sprintf("%d/100", *n)
		} else if n := intField(m, "evidence_level"); n != nil {
			sec.Evidence = fmt.Sprintf("%d/100", *n)
		}
	}
	if raw, ok := m["crimes"]; ok {
		json.Unmarshal(raw, &sec.Crimes)
	}
	sec.extra = scalarExtras(m, []string{"visible", "trafficking_value", "trafficking_crimes", "suspected", "projected_sentence",
		"case_officer", "recommended_action", "operation", "note", "evidence", "evidence_level", "crimes"})
	return sec
}

type upgradeOffer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Cost *int64 `json:"cost"`
}

func (u upgradeOffer) name() string { return nonEmpty(u.Name, u.ID) }

func (u upgradeOffer) text() string {
	if u.Cost != nil {
		return fmt.Sprintf("%s (%d)", u.name(), *u.Cost)
	}
	return u.name()
}

// parseUpgrades accepts ["counter_lamp"] or {owned:[{id,name}], available:[{id,name,cost}]}.
// parseUpgrades reads the upgrades block. Snapshots recorded before the mod's fix
// (5bcd757) listed for-sale upgrades (state 1) under owned; those are skipped.
func parseUpgrades(raw json.RawMessage) (owned []string, offer []upgradeOffer, ids []string) {
	if len(raw) == 0 {
		return nil, nil, nil
	}
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		return names(raw), nil, nil
	}
	var obj struct {
		Owned     []json.RawMessage `json:"owned"`
		Available []upgradeOffer    `json:"available"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil, nil, nil
	}
	for _, a := range obj.Available {
		if a.name() != "" {
			offer = append(offer, a)
		}
	}
	for _, e := range obj.Owned {
		var o struct {
			ID    string `json:"id"`
			State *int64 `json:"state"`
		}
		if json.Unmarshal(e, &o) == nil && o.State != nil && *o.State == 1 {
			continue
		}
		if o.ID != "" {
			ids = append(ids, o.ID)
		}
		owned = append(owned, names(json.RawMessage("["+string(e)+"]"))...)
	}
	return owned, offer, ids
}

// parseServices accepts ["early_warning"] or [{id, name, cost, using, …}].
func parseServices(raw json.RawMessage) []service {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var out []service
	for _, e := range list {
		var id string
		if json.Unmarshal(e, &id) == nil {
			out = append(out, service{ID: id})
			continue
		}
		var o struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Cost     *int64 `json:"cost"`
			Using    *bool  `json:"using"`
			Unlocked *bool  `json:"unlocked"`
		}
		if json.Unmarshal(e, &o) == nil && (o.ID != "" || o.Name != "") {
			using := o.Using
			if using == nil {
				f := false
				using = &f
			}
			out = append(out, service{ID: o.ID, Name: o.Name, Cost: o.Cost, Using: using, Unlocked: o.Unlocked})
		}
	}
	return out
}

// parseMarket accepts a flat list of events or {active: [...], calendar: [{day, events}]}.
func parseMarket(raw json.RawMessage) (active []marketEvent, cal []calendarDay) {
	if len(raw) == 0 {
		return nil, nil
	}
	var list []map[string]any
	if json.Unmarshal(raw, &list) == nil {
		for _, e := range list {
			active = append(active, toEvent(e))
		}
		return active, nil
	}
	var obj struct {
		Active   []map[string]any `json:"active"`
		Calendar []struct {
			Day    *int64           `json:"day"`
			Events []map[string]any `json:"events"`
		} `json:"calendar"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return nil, nil
	}
	for _, e := range obj.Active {
		active = append(active, toEvent(e))
	}
	for _, d := range obj.Calendar {
		if d.Day == nil {
			continue
		}
		cd := calendarDay{Day: *d.Day}
		for _, e := range d.Events {
			cd.Events = append(cd.Events, toEvent(e))
		}
		cal = append(cal, cd)
	}
	return active, cal
}

// toEvent reads an event loosely: the field names of DAY.md's per-event row are not
// settled, so the common spellings are all accepted.
func toEvent(m map[string]any) marketEvent {
	ev := marketEvent{Name: firstStr(m, "name", "display_name", "identifier", "id"), ID: firstStr(m, "identifier", "id")}
	switch strings.ToUpper(firstStr(m, "type", "event_type")) {
	case "INNATE":
		ev.Innate = true
	case "THREAT":
		ev.Threat = true
	}
	ev.StartDay, ev.Duration = numPtr(m["start_day"]), numPtr(m["duration"])
	if ev.Duration == nil {
		ev.Duration = numPtr(m["total_duration"])
	}
	if mod := modText(m); mod != "" {
		ev.Mods = append(ev.Mods, mod)
		ev.types = append(ev.types, strings.ToLower(firstStr(m, "item_type", "itemType")))
		ev.pcts = append(ev.pcts, modPct(m))
	}
	for _, k := range []string{"negotiation", "negociation", "negotiation_data", "negociation_datas", "modifiers"} {
		if xs, ok := m[k].([]any); ok {
			for _, x := range xs {
				if mm, ok := x.(map[string]any); ok {
					if mod := modText(mm); mod != "" {
						ev.Mods = append(ev.Mods, mod)
						ev.types = append(ev.types, strings.ToLower(firstStr(mm, "item_type", "itemType")))
						ev.pcts = append(ev.pcts, modPct(mm))
					}
				}
			}
		}
	}
	return ev
}

func modText(m map[string]any) string {
	ty := firstStr(m, "item_type", "itemType")
	v := numPtr(m["value_pct"])
	if v == nil {
		v = numPtr(m["value"])
	}
	if ty == "" || v == nil {
		return ""
	}
	return fmt.Sprintf("%s %+d%%", ty, *v)
}

func modPct(m map[string]any) int64 {
	v := numPtr(m["value_pct"])
	if v == nil {
		v = numPtr(m["value"])
	}
	if v == nil {
		return 0
	}
	return *v
}

func (e marketEvent) text() string {
	var parts []string
	if e.Name != "" {
		parts = append(parts, e.Name)
	}
	if len(e.Mods) > 0 {
		parts = append(parts, strings.Join(e.Mods, ", "))
	}
	t := strings.Join(parts, ": ")
	if e.Threat {
		t = "THREAT " + t
	}
	if e.Duration != nil && *e.Duration > 0 {
		t += fmt.Sprintf(" (%d days)", *e.Duration)
	}
	return t
}

type requestAsk struct {
	item          string
	count, reward *int64
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			return v
		}
	}
	return nil
}

func requestText(m map[string]any) string {
	title := firstStr(m, "title", "id")
	item := firstStr(m, "required_item", "item")
	if title == "" && item == "" {
		return ""
	}
	var parts []string
	if title != "" {
		parts = append(parts, title)
	}
	if who := firstStr(m, "requester_name", "requester"); who != "" {
		parts = append(parts, "from "+who)
	}
	if f := firstStr(m, "faction", "faction_id"); f != "" {
		parts = append(parts, factionName(f))
	}
	if item != "" {
		n := numPtr(m["required_item_count"])
		if n == nil {
			n = numPtr(m["count"])
		}
		if n != nil {
			item = fmt.Sprintf("%d x %s", *n, item)
		}
		parts = append(parts, "wants "+item)
	}
	if c := numPtr(m["credit_reward"]); c != nil {
		parts = append(parts, fmt.Sprintf("pays %d", *c))
	}
	if r := numPtr(m["reputation_reward"]); r != nil {
		parts = append(parts, fmt.Sprintf("+%d rep", *r))
	}
	if d := numPtr(m["duration"]); d != nil {
		parts = append(parts, fmt.Sprintf("%d days", *d))
	}
	return strings.Join(parts, ", ")
}

// compactText renders arbitrary JSON as short readable text: objects as k=v, lists
// bracketed, nulls and empty values dropped.
func compactText(raw json.RawMessage) string {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return ""
	}
	return clip(textOf(v), 500)
}

// dumpingGroundOpen reads the afterhours list ([{location, name, available}]) for an
// open dumping ground.
func dumpingGroundOpen(raw json.RawMessage) bool {
	var locs []map[string]any
	if json.Unmarshal(raw, &locs) != nil {
		return false
	}
	for _, l := range locs {
		where := strings.ToLower(firstStr(l, "location", "name", "id"))
		if open, _ := l["available"].(bool); open && strings.Contains(strings.ReplaceAll(where, "_", " "), "dump") {
			return true
		}
	}
	return false
}

func textOf(v any) string {
	switch x := v.(type) {
	case map[string]any:
		var parts []string
		for _, k := range sortedKeys(x) {
			if slices.Contains(hiddenKeys, k) {
				continue
			}
			if t := textOf(x[k]); t != "" {
				parts = append(parts, k+"="+t)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case []any:
		var parts []string
		for _, e := range x {
			if t := textOf(e); t != "" {
				parts = append(parts, t)
			}
		}
		if len(parts) == 0 {
			return ""
		}
		return "[" + strings.Join(parts, "; ") + "]"
	case string:
		return plainLog(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// plainLog strips the rich-text tags the game's night log carries.
func plainLog(s string) string {
	return strings.Join(strings.Fields(tagRE.ReplaceAllString(s, "")), " ")
}

// names reads ["a", …] or [{"name"/"id": …}, …].
func names(raw json.RawMessage) []string {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var out []string
	for _, e := range list {
		var s string
		if json.Unmarshal(e, &s) == nil {
			out = append(out, s)
			continue
		}
		var o map[string]any
		if json.Unmarshal(e, &o) == nil {
			if n := firstStr(o, "name", "id"); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

func intField(m map[string]json.RawMessage, k string) *int64 {
	raw, ok := m[k]
	if !ok {
		return nil
	}
	var f float64
	if json.Unmarshal(raw, &f) != nil {
		return nil
	}
	v := int64(f)
	return &v
}

func strField(m map[string]json.RawMessage, k string) string {
	var s string
	if raw, ok := m[k]; ok {
		json.Unmarshal(raw, &s)
	}
	return s
}

func boolField(m map[string]json.RawMessage, k string) *bool {
	raw, ok := m[k]
	if !ok {
		return nil
	}
	var b bool
	if json.Unmarshal(raw, &b) != nil {
		return nil
	}
	return &b
}

func objField(m map[string]json.RawMessage, k string) map[string]json.RawMessage {
	var o map[string]json.RawMessage
	if raw, ok := m[k]; ok {
		json.Unmarshal(raw, &o)
	}
	return o
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func numPtr(v any) *int64 {
	switch x := v.(type) {
	case float64:
		n := int64(x)
		return &n
	case string:
		if n, err := strconv.ParseInt(x, 10, 64); err == nil {
			return &n
		}
	}
	return nil
}

func scalarExtras(m map[string]json.RawMessage, known []string) map[string]string {
	var out map[string]string
	for k, v := range m {
		if slices.Contains(known, k) {
			continue
		}
		var x any
		if json.Unmarshal(v, &x) != nil {
			continue
		}
		var s string
		switch t := x.(type) {
		case string:
			s = t
		case float64, bool:
			s = string(v)
		default:
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[k] = s
	}
	return out
}
