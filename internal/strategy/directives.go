package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	// directivesMaxTokens bounds the directives answer; a full object is about 250 tokens.
	directivesMaxTokens = 500
	// hotCapTrafficking is the trafficking record from which hot stock is capped to bay
	// room: inspections are scheduled from inspectionThreshold, and the plan prepares as
	// the record nears it.
	hotCapTrafficking = inspectionThreshold - 10
	// The executor's buy limits when the plan gives none: every wholesale lot whose
	// estimate covers the price, and a discount on single lots (docs/research/EXPERIMENTS.md
	// §7: buying at the estimate is net negative).
	defaultWholesaleMaxPct = 100
	defaultSingleMaxPct    = 80
)

// directivesAsk follows the plan in the same conversation, so the server reuses the
// plan's prompt from its cache and the prose plan stays byte-identical to a plan without
// directives.
const directivesAsk = `Now write that plan as directives for the executor that plays the counter and the night: one JSON object and nothing else, with only these keys (leave out a key you have no view on):
{"cash_floor": credits (the CASH floor),
"buy": {"wholesale_max_pct": accept a wholesale lot up to this % of its estimate, "single_max_pct": the same for a single lot, "max_spend": buying budget today, "prefer_types": [item types to buy], "avoid_types": [item types not to buy], "hot_stock_cap": most stolen items to hold},
"sell": {"min_pct": least % of the estimate to sell at, "sell_down_to_cash": sell stock until cash reaches this (rent), "hold_uids": [uids not to sell: KEEP items, stolen items still cooling]},
"bargain": "never" or "ev_positive" or "aggressive",
"window": {"show_uids": [uids for the window], "avoid_stolen": true},
"services": [service ids to have on tonight],
"upgrades_buy": [upgrade ids to buy tonight, in order],
"permit": "buy" or "refuse" or "bribe",
"production": [{"machine_uid": uid, "action": short verb phrase, "items": [uids], "note": why}],
"augs": "shoot" or "serve",
"scavenge": true or false}
Numbers are plain integers; a uid is the number after "#".`

// directivesPrompt is the follow-up asking for the directives, with the ids the state
// names only by display name.
func directivesPrompt(s *snapshot) string {
	var b strings.Builder
	b.WriteString(directivesAsk)
	var svc []string
	for _, v := range s.Services {
		if v.ID == "" {
			continue
		}
		t := v.ID
		if v.Name != "" {
			t += " (" + v.Name
			if v.inUse() {
				t += ", on"
			} else {
				t += ", off"
			}
			t += ")"
		}
		svc = append(svc, t)
	}
	if len(svc) > 0 {
		fmt.Fprintf(&b, "\nService ids: %s.", strings.Join(svc, ", "))
	}
	var up []string
	for _, u := range s.UpgradesOffer {
		if u.ID != "" {
			up = append(up, fmt.Sprintf("%s (%s)", u.ID, u.text()))
		}
	}
	if len(up) > 0 {
		fmt.Fprintf(&b, "\nUpgrade ids on offer: %s.", strings.Join(up, ", "))
	}
	if f := s.favorUpgrades(); len(f) > 0 {
		fmt.Fprintf(&b, "\nNot yet bought, in the order Wild favor buys them (list them all in upgrades_buy; the executor buys each when Wild calls and favor or spare cash covers it): %s.", strings.Join(f, ", "))
	}
	if s.Permit != nil {
		if *s.Permit {
			b.WriteString("\nBusiness permit: held (leave permit out).")
		} else {
			fmt.Fprintf(&b, "\nBusiness permit: not held (%d); say bribe or buy every day.", s.permitFee())
		}
	}
	return b.String()
}

// DirectivesResult is the day's directives and how they were made.
type DirectivesResult struct {
	Directives protocol.Directives
	// FromModel lists the keys the model supplied and validation kept; the rest were
	// derived from the prose plan and the state.
	FromModel []string
	// Fixes are what validation and the lints dropped, clamped or added.
	Fixes []string
	// Why is the model's reason for each key it set ("*": one reason for all), as given.
	Why map[string]string
	// Err is why the model's directives were unusable, if they were.
	Err error
	// Bet is the plan's move on the run's bet (delta directives only); nil for none.
	Bet *protocol.BetMove
}

// DraftDirectives asks c, in the plan's conversation, for the plan as directives, and
// returns them validated, completed from the plan and the state, and linted. A failed call
// or unusable answer falls back to directives derived in code.
func DraftDirectives(ctx context.Context, c llm.Client, d *PlanDraft) DirectivesResult {
	if d.delta {
		return DraftDeltaDirectives(ctx, c, d)
	}
	msgs := append(slices.Clip(d.messages), llm.Message{Role: llm.RoleUser, Content: directivesPrompt(d.snap)})
	var raw json.RawMessage
	resp, err := c.Complete(ctx, llm.Request{System: d.system, Messages: msgs, MaxTokens: directivesMaxTokens})
	if err == nil {
		raw, err = llm.ExtractJSON(resp.Text)
	}
	return finishDirectives(raw, err, d.Plan, d.snap, d.persona, d.day)
}

// finishDirectives turns the model's raw directives (nil when it gave none) into the
// day's directives.
func finishDirectives(raw json.RawMessage, rawErr error, plan string, s *snapshot, persona string, day int) DirectivesResult {
	var r DirectivesResult
	var model protocol.Directives
	present := map[string]bool{}
	if rawErr != nil {
		r.Err = rawErr
	} else {
		model, present, r.Fixes, r.Err = parseDirectives(raw, s)
	}
	derived := deriveDirectives(plan, s, persona, day)
	r.Directives, r.FromModel = mergeDirectives(model, present, derived)
	r.Directives.Day = day
	r.Fixes = append(r.Fixes, lintDirectives(&r.Directives, s, day)...)
	r.Fixes = append(r.Fixes, protectBuying(&r.Directives, s)...)
	return r
}

// protectBuying leaves the executor's own buying, selling and window alone unless rent is
// short (a bridge with no directives plays identically to the executor alone): plans set
// floors near cash and spending caps of 26-140, which starved a hard shop of stock until it
// died of rent on day 41 (league t1-ab4, seed 93001; the executor alone lives past day 120).
// The executor keeps the rent itself from 3 days out.
func protectBuying(d *protocol.Directives, s *snapshot) []string {
	if s.Cash != nil && *s.Cash < s.rentHold() {
		return nil
	}
	var fixes []string
	if d.CashFloor != nil && *d.CashFloor > 0 {
		fixes = append(fixes, fmt.Sprintf("cash_floor %d dropped: rent is covered, the executor keeps it itself", *d.CashFloor))
	}
	d.CashFloor = nil
	if b := d.Buy; b != nil {
		if b.MaxSpend != nil {
			fixes = append(fixes, fmt.Sprintf("buy.max_spend %d dropped: rent is covered", *b.MaxSpend))
		}
		if b.WholesaleMaxPct != nil || b.SingleMaxPct != nil {
			fixes = append(fixes, "buy limits dropped: the executor's own prices stand")
		}
		b.MaxSpend, b.WholesaleMaxPct, b.SingleMaxPct = nil, nil, nil
		if t, ok := s.trafficking(); b.HotStockCap != nil && (!ok || t < hotCapTrafficking) {
			fixes = append(fixes, fmt.Sprintf("buy.hot_stock_cap %d dropped: no inspection is near (hold-until-clean, gym +4 days)", *b.HotStockCap))
			b.HotStockCap = nil
		}
	}
	if sl := d.Sell; sl != nil {
		if sl.MinPct != nil {
			// Plans set 100 (no sales below the estimate) and 25 (dumping) where the executor
			// sells from 90% and liquidates from 65% (lesson 4, CLAUDE-PLAY.md).
			fixes = append(fixes, fmt.Sprintf("sell.min_pct %d dropped: the executor's floors stand", *sl.MinPct))
			sl.MinPct = nil
		}
		if sl.SellDownToCash != nil {
			fixes = append(fixes, fmt.Sprintf("sell.sell_down_to_cash %d dropped: rent is covered", *sl.SellDownToCash))
			sl.SellDownToCash = nil
		}
		must := append(s.holdUIDs(), s.wineHolds()...)
		sl.HoldUIDs = slices.DeleteFunc(sl.HoldUIDs, func(u int64) bool { return !slices.Contains(must, u) })
	}
	if d.Window != nil && len(d.Window.ShowUIDs) > 0 {
		// The executor fills the window from its own rule; one or two hand-picked items
		// replaced it (no measured gain from concentrating the window, EXPERIMENTS.md v2).
		fixes = append(fixes, "window.show_uids dropped: the executor's window stands")
		d.Window.ShowUIDs = nil
	}
	if d.Engine == "wine" && s.hard() {
		fixes = append(fixes, "engine: wine dropped on hard, where the executor runs none")
		d.Engine = ""
	}
	return fixes
}

// directiveKeys are the contract's top-level keys, in order.
var directiveKeys = []string{"cash_floor", "buy", "sell", "bargain", "window", "services", "upgrades_buy", "permit", "production", "augs", "scavenge", "engine",
	"mortgage", "mortgage_margin", "mortgage_float", "mortgage_window", "mortgage_save_from_day", "mortgage_max_spend", "buyout_by_day", "appraisal"}

// parseDirectives reads the model's object loosely (numbers as strings, uids as "#123",
// ids by display name) and keeps what the contract and the state allow; fixes say what was
// dropped or clamped. present marks each key the model gave a usable value for.
func parseDirectives(raw json.RawMessage, s *snapshot) (d protocol.Directives, present map[string]bool, fixes []string, err error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return d, nil, nil, fmt.Errorf("directives are not a JSON object: %w", err)
	}
	present = map[string]bool{}
	note := func(format string, a ...any) { fixes = append(fixes, fmt.Sprintf(format, a...)) }
	uids := s.knownUIDs()
	keepUIDs := func(key string, v any) []int64 {
		var out []int64
		for _, u := range anyUIDs(v) {
			if uids[u] {
				out = append(out, u)
			} else {
				note("%s: dropped unknown uid %d", key, u)
			}
		}
		return out
	}
	for k := range m {
		if !slices.Contains(directiveKeys, k) && k != "day" {
			note("dropped unknown key %q", k)
		}
	}
	if n, ok := num(m["cash_floor"]); ok {
		d.CashFloor = clampNote(n, 0, math.MaxInt64, "cash_floor", note)
		present["cash_floor"] = true
	}
	if b, ok := m["buy"].(map[string]any); ok {
		var bd protocol.BuyDirectives
		if n, ok := num(b["wholesale_max_pct"]); ok {
			bd.WholesaleMaxPct = clampNote(n, 0, 100, "buy.wholesale_max_pct", note)
		}
		if n, ok := num(b["single_max_pct"]); ok {
			bd.SingleMaxPct = clampNote(n, 0, 100, "buy.single_max_pct", note)
		}
		if n, ok := num(b["max_spend"]); ok {
			bd.MaxSpend = clampNote(n, 0, math.MaxInt64, "buy.max_spend", note)
		}
		if n, ok := num(b["hot_stock_cap"]); ok {
			bd.HotStockCap = clampNote(n, 0, math.MaxInt64, "buy.hot_stock_cap", note)
		}
		bd.PreferTypes, bd.AvoidTypes = typeList(b["prefer_types"]), typeList(b["avoid_types"])
		if bd.WholesaleMaxPct != nil || bd.SingleMaxPct != nil || bd.MaxSpend != nil || bd.HotStockCap != nil || len(bd.PreferTypes)+len(bd.AvoidTypes) > 0 {
			d.Buy = &bd
			present["buy"] = true
		}
	}
	if v, ok := m["sell"].(map[string]any); ok {
		var sd protocol.SellDirectives
		if n, ok := num(v["min_pct"]); ok {
			sd.MinPct = clampNote(n, 0, 200, "sell.min_pct", note)
		}
		if n, ok := num(v["sell_down_to_cash"]); ok {
			sd.SellDownToCash = clampNote(n, 0, math.MaxInt64, "sell.sell_down_to_cash", note)
		}
		sd.HoldUIDs = keepUIDs("sell.hold_uids", v["hold_uids"])
		if sd.MinPct != nil || sd.SellDownToCash != nil || len(sd.HoldUIDs) > 0 {
			d.Sell = &sd
			present["sell"] = true
		}
	}
	if v, ok := enumOf(m["bargain"], protocol.BargainModes); ok {
		d.Bargain = v
		present["bargain"] = true
	} else if m["bargain"] != nil {
		note("bargain: dropped %v (want one of %s)", m["bargain"], strings.Join(protocol.BargainModes, ", "))
	}
	if v, ok := m["window"].(map[string]any); ok {
		var wd protocol.WindowDirectives
		wd.ShowUIDs = keepUIDs("window.show_uids", v["show_uids"])
		if a, ok := v["avoid_stolen"].(bool); ok {
			wd.AvoidStolen = &a
		}
		d.Window = &wd
		present["window"] = true
	}
	if v, ok := m["services"].([]any); ok {
		for _, x := range v {
			ref, _ := x.(string)
			if id, ok := s.matchService(ref); ok {
				if !slices.Contains(d.Services, id) {
					d.Services = append(d.Services, id)
				}
			} else {
				note("services: dropped unknown service %q", ref)
			}
		}
		present["services"] = true
	}
	if v, ok := m["upgrades_buy"].([]any); ok {
		for _, x := range v {
			ref, _ := x.(string)
			u, ok := s.matchUpgrade(ref)
			switch {
			case !ok:
				note("upgrades_buy: dropped %q, not on offer", ref)
			case !slices.Contains(d.UpgradesBuy, u.ID):
				d.UpgradesBuy = append(d.UpgradesBuy, u.ID)
			}
		}
		present["upgrades_buy"] = true
	}
	if v, ok := enumOf(m["permit"], protocol.PermitModes); ok {
		d.Permit = v
		present["permit"] = true
	} else if m["permit"] != nil {
		note("permit: dropped %v (want one of %s)", m["permit"], strings.Join(protocol.PermitModes, ", "))
	}
	if v, ok := m["production"].([]any); ok {
		for _, x := range v {
			p, ok := x.(map[string]any)
			if !ok {
				continue
			}
			mu, ok := anyUID(p["machine_uid"])
			action, _ := p["action"].(string)
			if !ok || !uids[mu] || strings.TrimSpace(action) == "" {
				note("production: dropped %v (needs a known machine_uid and an action)", p["machine_uid"])
				continue
			}
			why, _ := p["note"].(string)
			d.Production = append(d.Production, protocol.ProductionDirective{
				MachineUID: mu, Action: clip(strings.TrimSpace(action), 60),
				Items: keepUIDs("production.items", p["items"]), Note: clip(strings.TrimSpace(why), protocol.MaxOptionNote),
			})
		}
		present["production"] = true
	}
	if v, ok := enumOf(m["augs"], protocol.AugModes); ok {
		d.Augs = v
		present["augs"] = true
	} else if m["augs"] != nil {
		note("augs: dropped %v (want shoot or serve)", m["augs"])
	}
	if v, ok := m["scavenge"].(bool); ok {
		d.Scavenge = &v
		present["scavenge"] = true
	}
	if v, ok := engineOf(m["engine"]); ok {
		d.Engine = v
		present["engine"] = true
	} else if m["engine"] != nil {
		note("engine: dropped %v (want wine, water, furnace joined by +, or none)", m["engine"])
	}
	parseMortgage(m, &d, present, note)
	if v, ok := enumOf(m["appraisal"], protocol.AppraisalModes); ok {
		d.Appraisal = v
		present["appraisal"] = true
	} else if m["appraisal"] != nil {
		note("appraisal: dropped %v (want join or off)", m["appraisal"])
	}
	if len(present) == 0 {
		return d, present, fixes, fmt.Errorf("directives have none of the contract's keys")
	}
	return d, present, fixes, nil
}

// engineOf reads an engine: none, or wine, water and furnace joined by "+" (or listed).
func engineOf(v any) (string, bool) {
	var parts []string
	switch x := v.(type) {
	case string:
		parts = strings.Split(x, "+")
	case []any:
		for _, p := range x {
			s, _ := p.(string)
			parts = append(parts, s)
		}
	default:
		return "", false
	}
	var out []string
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "none" && len(parts) == 1 {
			return p, true
		}
		if !slices.Contains(protocol.EngineParts, p) {
			return "", false
		}
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return strings.Join(out, "+"), len(out) > 0
}

// parseMortgage reads the gym executor's mortgage levers.
func parseMortgage(m map[string]any, d *protocol.Directives, present map[string]bool, note func(string, ...any)) {
	if v, ok := enumOf(m["mortgage"], protocol.MortgageModes); ok {
		d.Mortgage = v
		present["mortgage"] = true
	} else if m["mortgage"] != nil {
		note("mortgage: dropped %v (want one of %s)", m["mortgage"], strings.Join(protocol.MortgageModes, ", "))
	}
	if x, ok := m["mortgage_margin"].(float64); ok && !math.IsNaN(x) && !math.IsInf(x, 0) {
		c := min(max(x, 0.3), 3.0)
		if c != x {
			note("mortgage_margin: %g clamped to %g", x, c)
		}
		d.MortgageMargin = &c
		present["mortgage_margin"] = true
	}
	ints := []struct {
		key string
		dst **int64
		max int64
	}{{"mortgage_float", &d.MortgageFloat, 20000}, {"mortgage_save_from_day", &d.MortgageSaveFromDay, 1000}, {"buyout_by_day", &d.BuyoutByDay, 1000}, {"mortgage_max_spend", &d.MortgageMaxSpend, math.MaxInt64}}
	for _, f := range ints {
		if n, ok := num(m[f.key]); ok {
			*f.dst = clampNote(n, 0, f.max, f.key, note)
			present[f.key] = true
		}
	}
	if w, ok := m["mortgage_window"].([]any); ok {
		from, ok1 := num(firstOr(w, 0))
		to, ok2 := num(firstOr(w, 1))
		if len(w) == 2 && ok1 && ok2 && 0 <= from && from <= to {
			d.MortgageWindow = []int64{from, to}
			present["mortgage_window"] = true
		} else {
			note("mortgage_window: dropped %v (want [from, to] days)", w)
		}
	}
}

func firstOr(xs []any, i int) any {
	if i < len(xs) {
		return xs[i]
	}
	return nil
}

// mergeDirectives takes each key the model gave, and the derived value for the rest; in
// buy, sell and window a field the model left out is derived too.
func mergeDirectives(model protocol.Directives, present map[string]bool, derived protocol.Directives) (protocol.Directives, []string) {
	out := derived
	var from []string
	take := func(key string, set func()) {
		if present[key] {
			set()
			from = append(from, key)
		}
	}
	take("cash_floor", func() { out.CashFloor = model.CashFloor })
	take("buy", func() { out.Buy = mergeBuy(model.Buy, derived.Buy) })
	take("sell", func() { out.Sell = mergeSell(model.Sell, derived.Sell) })
	take("bargain", func() { out.Bargain = model.Bargain })
	take("window", func() { out.Window = mergeWindow(model.Window, derived.Window) })
	take("services", func() { out.Services = model.Services })
	take("upgrades_buy", func() { out.UpgradesBuy = model.UpgradesBuy })
	take("permit", func() { out.Permit = model.Permit })
	take("production", func() { out.Production = model.Production })
	take("augs", func() { out.Augs = model.Augs })
	take("scavenge", func() { out.Scavenge = model.Scavenge })
	take("engine", func() { out.Engine = model.Engine })
	take("mortgage", func() { out.Mortgage = model.Mortgage })
	take("mortgage_margin", func() { out.MortgageMargin = model.MortgageMargin })
	take("mortgage_float", func() { out.MortgageFloat = model.MortgageFloat })
	take("mortgage_window", func() { out.MortgageWindow = model.MortgageWindow })
	take("mortgage_save_from_day", func() { out.MortgageSaveFromDay = model.MortgageSaveFromDay })
	take("mortgage_max_spend", func() { out.MortgageMaxSpend = model.MortgageMaxSpend })
	take("buyout_by_day", func() { out.BuyoutByDay = model.BuyoutByDay })
	take("appraisal", func() { out.Appraisal = model.Appraisal })
	return out, from
}

func mergeBuy(m, d *protocol.BuyDirectives) *protocol.BuyDirectives {
	if d == nil {
		return m
	}
	out := *m
	out.WholesaleMaxPct = orPtr(out.WholesaleMaxPct, d.WholesaleMaxPct)
	out.SingleMaxPct = orPtr(out.SingleMaxPct, d.SingleMaxPct)
	out.MaxSpend = orPtr(out.MaxSpend, d.MaxSpend)
	out.HotStockCap = orPtr(out.HotStockCap, d.HotStockCap)
	if out.PreferTypes == nil {
		out.PreferTypes = d.PreferTypes
	}
	if out.AvoidTypes == nil {
		out.AvoidTypes = d.AvoidTypes
	}
	return &out
}

func mergeSell(m, d *protocol.SellDirectives) *protocol.SellDirectives {
	if d == nil {
		return m
	}
	out := *m
	out.MinPct = orPtr(out.MinPct, d.MinPct)
	out.SellDownToCash = orPtr(out.SellDownToCash, d.SellDownToCash)
	return &out
}

func mergeWindow(m, d *protocol.WindowDirectives) *protocol.WindowDirectives {
	if d == nil {
		return m
	}
	out := *m
	out.AvoidStolen = orPtr(out.AvoidStolen, d.AvoidStolen)
	if out.ShowUIDs == nil {
		out.ShowUIDs = d.ShowUIDs
	}
	return &out
}

func orPtr[T any](a, b *T) *T {
	if a != nil {
		return a
	}
	return b
}

var floorRE = regexp.MustCompile(`floor\D{0,20}?(\d[\d,]*)`)

// deriveDirectives is the plan as directives without the model: the prose plan's CASH
// floor, WINDOW uids and BUY tonight lines, and the state's rules (docs/research/
// EXPERIMENTS.md, docs/re/MITIGATIONS.md) for the rest.
func deriveDirectives(plan string, s *snapshot, persona string, day int) protocol.Directives {
	d := protocol.Directives{Day: day, Bargain: "ev_positive"}
	reserve := s.rentHold()
	floor := reserve
	if lines := section(plan, "CASH"); len(lines) > 0 {
		if m := floorRE.FindStringSubmatch(strings.ToLower(lines[0])); m != nil {
			if n, err := strconv.ParseInt(strings.ReplaceAll(m[1], ",", ""), 10, 64); err == nil {
				floor = max(n, reserve)
			}
		}
	}
	if s.Cash != nil {
		floor = min(floor, max(*s.Cash, 0))
	}
	d.CashFloor = &floor

	buy := protocol.BuyDirectives{WholesaleMaxPct: ptrTo(int64(defaultWholesaleMaxPct)), SingleMaxPct: ptrTo(int64(defaultSingleMaxPct))}
	if s.Cash != nil {
		buy.MaxSpend = ptrTo(max(*s.Cash-floor, 0))
	}
	buy.PreferTypes, buy.AvoidTypes = s.calendarTypes(day)
	if t, ok := s.trafficking(); ok && t >= hotCapTrafficking {
		if room, ok := s.hotRoom(); ok {
			buy.HotStockCap = &room
		}
	}
	d.Buy = &buy

	var sell protocol.SellDirectives
	if s.Cash != nil && *s.Cash < reserve {
		sell.SellDownToCash = &reserve
	}
	sell.HoldUIDs = s.holdUIDs()
	if sell.SellDownToCash == nil {
		sell.HoldUIDs = appendMissingUIDs(sell.HoldUIDs, s.wineHolds())
	}
	if sell.SellDownToCash != nil || len(sell.HoldUIDs) > 0 {
		d.Sell = &sell
	}

	d.Window = &protocol.WindowDirectives{ShowUIDs: s.planWindowUIDs(plan), AvoidStolen: ptrTo(true)}
	d.Services = s.wantedServices()
	d.UpgradesBuy = s.wantedUpgrades(plan, persona, day)
	if s.Permit != nil && !*s.Permit {
		d.Permit = "bribe"
		if s.permitAffordable() {
			d.Permit = "buy"
		}
	}
	if s.engineIdle() {
		d.Engine = "none"
	}
	if _, ok := s.augCondition(day); ok {
		d.Augs = "shoot"
	}
	if open, ok := s.scavengeTonight(); ok {
		d.Scavenge = &open
	}
	return d
}

// lintDirectives holds the directives to the rules the plan lints enforce, fixing them in
// place; each fix is described.
func lintDirectives(d *protocol.Directives, s *snapshot, day int) []string {
	var fixes []string
	note := func(format string, a ...any) { fixes = append(fixes, fmt.Sprintf(format, a...)) }
	hold := s.rentHold()
	if s.Cash != nil {
		cash := max(*s.Cash, 0)
		reserve := min(hold, cash)
		switch {
		case d.CashFloor == nil:
			d.CashFloor = ptrTo(reserve)
		case *d.CashFloor > cash:
			note("cash_floor %d is above cash %d; lowered to it", *d.CashFloor, cash)
			d.CashFloor = ptrTo(cash)
		case *d.CashFloor < reserve:
			note("cash_floor %d does not hold the rent due in %d day(s); raised to %d", *d.CashFloor, *s.Rent.DaysLeft, reserve)
			d.CashFloor = ptrTo(reserve)
		}
		free := cash - *d.CashFloor
		if d.Buy != nil && d.Buy.MaxSpend != nil && *d.Buy.MaxSpend > free {
			note("buy.max_spend %d is more than cash %d above the floor %d; lowered to %d", *d.Buy.MaxSpend, cash, *d.CashFloor, free)
			d.Buy.MaxSpend = ptrTo(free)
		}
		if cash < hold {
			if d.Sell == nil {
				d.Sell = &protocol.SellDirectives{}
			}
			if d.Sell.SellDownToCash == nil || *d.Sell.SellDownToCash < hold {
				note("rent and services %d are due in %d day(s) with cash %d: sell.sell_down_to_cash raised to %d", hold, *s.Rent.DaysLeft, cash, hold)
				d.Sell.SellDownToCash = ptrTo(hold)
			}
			// Short of rent, only what the shop must keep is held back from sale.
			must := s.holdUIDs()
			d.Sell.HoldUIDs = slices.DeleteFunc(d.Sell.HoldUIDs, func(u int64) bool {
				if slices.Contains(must, u) {
					return false
				}
				note("sell.hold_uids: released #%d, rent is short", u)
				return true
			})
		}
		if f := s.favorUpgrades(); len(f) > 0 {
			// Near-loss mitigations keep their place ahead of the favor order; the model's
			// other picks follow it, so favor is not spent on them first.
			var lead []string
			for _, n := range s.neededMitigations(day) {
				if slices.Contains(d.UpgradesBuy, n.u.ID) {
					lead = append(lead, n.u.ID)
				}
			}
			ordered := appendMissing(appendMissing(lead, f...), d.UpgradesBuy...)
			if !slices.Equal(ordered, d.UpgradesBuy) {
				note("upgrades_buy: the favor order (%s) goes first; unspent favor earns nothing", strings.Join(f, ", "))
			}
			d.UpgradesBuy = ordered
		}
		d.UpgradesBuy = s.affordableUpgrades(d.UpgradesBuy, note)
		if s.Permit == nil || !*s.Permit {
			affordable := s.permitAffordable()
			switch {
			case d.Permit == "buy" && !affordable:
				note("permit: buy needs %d plus the rent and %d and cash is %d; bribe instead", s.permitFee(), permitMargin, cash)
				d.Permit = "bribe"
			case d.Permit == "bribe" && affordable:
				note("permit: cash covers it; buy, a bribed permit keeps logging unlicensed crime")
				d.Permit = "buy"
			case d.Permit == "" && s.Permit != nil:
				d.Permit = "bribe"
				if affordable {
					d.Permit = "buy"
				}
				note("permit: unset lets the executor choose; %s", d.Permit)
			case d.Permit == "refuse":
				// docs/research/EXPERIMENTS.md §1: refusing from day 38 is arrest on day 79-80.
				d.Permit = "bribe"
				if affordable {
					d.Permit = "buy"
				}
				note("permit: refusing it ends in arrest by day 80; %s instead", d.Permit)
			}
		}
	}
	if s.Permit != nil && *s.Permit && d.Permit != "" {
		note("permit: dropped %q, the permit is held", d.Permit)
		d.Permit = ""
	}
	if b := d.Buy; b != nil {
		for _, t := range b.PreferTypes {
			if slices.Contains(b.AvoidTypes, t) {
				note("buy: %s is both preferred and avoided; dropped from both", t)
			}
		}
		both := func(t string) bool { return slices.Contains(b.PreferTypes, t) && slices.Contains(b.AvoidTypes, t) }
		prefer := slices.DeleteFunc(slices.Clone(b.PreferTypes), both)
		b.AvoidTypes = slices.DeleteFunc(b.AvoidTypes, both)
		b.PreferTypes = prefer
	}
	if t, ok := s.trafficking(); ok && t >= hotCapTrafficking {
		if room, ok := s.hotRoom(); ok {
			if d.Buy == nil {
				d.Buy = &protocol.BuyDirectives{}
			}
			if d.Buy.HotStockCap == nil || *d.Buy.HotStockCap > room {
				note("trafficking %d: buy.hot_stock_cap set to %d, what the smuggler bays hold", t, room)
				d.Buy.HotStockCap = ptrTo(room)
			}
		}
	}
	if d.Window != nil {
		keep := s.keepUIDs(s.production())
		d.Window.ShowUIDs = slices.DeleteFunc(d.Window.ShowUIDs, func(u int64) bool {
			it, ok := s.itemByUID(u)
			switch {
			case ok && !it.displayable():
				note("window: dropped stolen or contraband #%d (an inspection at opening)", u)
				return true
			case keep[u] != nil:
				note("window: dropped #%d, which the shop keeps for its use", u)
				return true
			}
			return false
		})
		if d.Window.AvoidStolen == nil || !*d.Window.AvoidStolen {
			d.Window.AvoidStolen = ptrTo(true)
		}
	}
	if hold := s.holdUIDs(); len(hold) > 0 {
		if d.Sell == nil {
			d.Sell = &protocol.SellDirectives{}
		}
		for _, u := range hold {
			if !slices.Contains(d.Sell.HoldUIDs, u) {
				note("sell.hold_uids: added #%d (kept for use, or stolen and still cooling)", u)
				d.Sell.HoldUIDs = append(d.Sell.HoldUIDs, u)
			}
		}
	}
	if _, ok := s.augCondition(day); ok && d.Augs != "shoot" {
		if d.Augs != "" {
			note("augs: %q on a difficulty where aug presence ends the run; shoot", d.Augs)
		}
		d.Augs = "shoot"
	}
	sellingDown := d.Sell != nil && d.Sell.SellDownToCash != nil && s.Cash != nil && *s.Cash < *d.Sell.SellDownToCash
	if sellingDown && d.Sell.MinPct != nil {
		note("sell.min_pct %d dropped: selling down to %d needs the executor's own liquidation floor", *d.Sell.MinPct, *d.Sell.SellDownToCash)
		d.Sell.MinPct = nil
	}
	if d.Sell != nil {
		short := sellingDown
		keep := s.wineHolds()
		for _, it := range s.Inventory {
			g, ok := wineGrade(it)
			if !ok || it.UID == nil {
				continue
			}
			held := slices.Contains(d.Sell.HoldUIDs, *it.UID)
			switch {
			case held && !slices.Contains(keep, *it.UID):
				note("sell.hold_uids: released %s wine #%d, cash does not cover rent, a week of services and %d for buying", wineGrades[g], *it.UID, buyingMargin)
				d.Sell.HoldUIDs = slices.DeleteFunc(d.Sell.HoldUIDs, func(u int64) bool { return u == *it.UID })
			case !held && !short && slices.Contains(keep, *it.UID):
				d.Sell.HoldUIDs = append(d.Sell.HoldUIDs, *it.UID)
			}
		}
	} else if w := s.wineHolds(); len(w) > 0 {
		d.Sell = &protocol.SellDirectives{HoldUIDs: w}
	}
	if d.Services != nil {
		if !s.drawsPower() {
			d.Services = slices.DeleteFunc(d.Services, func(id string) bool {
				if strings.EqualFold(id, "POWER_SERVICE") {
					note("services: power off, nothing the shop owns draws store power")
					return true
				}
				return false
			})
		}
		for _, id := range s.wantedServices() {
			if slices.Contains(d.Services, id) {
				continue
			}
			switch {
			case strings.Contains(id, "JANITORIAL"), strings.Contains(id, "INSURANCE"):
				note("services: %s added; insurance and janitorial on every night (gym, +21 days)", id)
				d.Services = append(d.Services, id)
			case strings.HasSuffix(id, "_REFERAL"):
				note("services: %s added; a bought referral earns only while it is on", id)
				d.Services = append(d.Services, id)
			}
		}
	}
	if ll, ok := s.rep("lower_levels"); ok && ll < bargainLLFloor && (d.Bargain == "ev_positive" || d.Bargain == "aggressive") {
		note("bargain: never, Lower Levels %.0f is below %d", ll, bargainLLFloor)
		d.Bargain = "never"
	}
	switch idle := s.engineIdle(); {
	case idle && d.Engine != "none":
		note("engine: no new wine for %d days; none frees the cash in its inputs", *s.wineIdle)
		d.Engine = "none"
	case !idle && d.Engine == "none":
		note("engine: none dropped; it is for a wine engine that has made no new wine for %d days", wineIdleDays)
		d.Engine = ""
	}
	if open, ok := s.scavengeTonight(); ok && open && (d.Scavenge == nil || !*d.Scavenge) {
		if d.Scavenge != nil {
			note("scavenge: the dumping ground is open and the shop is not wounded; go")
		}
		d.Scavenge = ptrTo(true)
	}
	return fixes
}

// rentHold is the cash to keep from two days before rent day: the rent due and the
// services' nights until then, as the gym's executor reserves it (sim/gym/Episode.Player.cs).
func (s *snapshot) rentHold() int64 {
	r := s.rentReserve()
	if r == 0 {
		return 0
	}
	if s.Day != nil && *s.Day%7 == supplierWeekday {
		// Supplier day: spend down to the bare rent (Claude play 3).
		return r
	}
	var svc int64
	for _, v := range s.Services {
		if v.inUse() && v.Cost != nil {
			svc += *v.Cost
		}
	}
	return r + svc*(*s.Rent.DaysLeft+1)
}

// affordableUpgrades keeps the upgrades on offer, in order, that Wild favor pays, or that
// leave cash for the rent and upgradeCashMargin: a listed upgrade skips the executor's own
// surplus check (Claude play 2, 3). Landlord grace may use the rent itself: it is bought
// instead of paying the rent.
func (s *snapshot) affordableUpgrades(ids []string, note func(string, ...any)) []string {
	var favor, cash int64
	if s.WildFavor != nil {
		favor = max(*s.WildFavor, 0)
	}
	if s.Cash != nil {
		cash = max(*s.Cash, 0)
	}
	keep := s.dueOrZero() + upgradeCashMargin
	var out []string
	for _, id := range ids {
		u, ok := s.offered(id)
		if !ok || u.Cost == nil {
			note("upgrades_buy: dropped %s, no price on offer", id)
			continue
		}
		f := min(favor, *u.Cost)
		c := *u.Cost - f
		grace := strings.EqualFold(id, "LANDLORD_GRACE")
		if c > 0 && (grace && c > cash || !grace && cash-c < keep) {
			note("upgrades_buy: dropped %s (%d: favor %d + cash %d), the cash left would not keep rent and %d", id, *u.Cost, f, c, upgradeCashMargin)
			continue
		}
		favor -= f
		cash -= c
		out = append(out, id)
	}
	return out
}

// wantedUpgrades are the upgrades the plan's rules buy tonight: mitigations of near loss
// conditions, then what the plan's BUY tonight lines name, then the style's next
// affordable purchase.
func (s *snapshot) wantedUpgrades(plan, persona string, day int) []string {
	var out []string
	add := func(id string) {
		if id != "" && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	for _, n := range s.neededMitigations(day) {
		add(n.u.ID)
	}
	for _, line := range strings.Split(strings.ToLower(plan), "\n") {
		if !strings.Contains(line, "buy tonight") || mentionsAny(line, []string{"buy tonight: none", "not buy", "no upgrade"}) {
			continue
		}
		for _, u := range s.UpgradesOffer {
			if mentionsAny(line, upgradeRefs(u)) {
				add(u.ID)
			}
		}
	}
	for _, n := range s.buyOrder(persona) {
		if n.affordable {
			add(n.u.ID)
			break
		}
	}
	for _, id := range s.favorUpgrades() {
		add(id)
	}
	return out
}

// wantedServices are the services in use plus insurance and janitorial, measured to win on
// every night (docs/research/EXPERIMENTS.md), and the referrals once bought; power only
// while something draws store power (Claude play). The game's ids when the state lists none.
func (s *snapshot) wantedServices() []string {
	if len(s.Services) == 0 {
		return []string{"INSURANCE_SERVICE", "JANITORIAL_SERVICE"}
	}
	power := s.drawsPower()
	var out []string
	for _, v := range s.Services {
		id := strings.ToUpper(v.ID)
		switch {
		case v.ID == "":
		case id == "POWER_SERVICE" && !power:
		case v.inUse(), strings.Contains(id, "INSURANCE"), strings.Contains(id, "JANITORIAL"),
			strings.HasSuffix(id, "_REFERAL") && (v.Unlocked == nil || *v.Unlocked):
			out = append(out, v.ID)
		}
	}
	return out
}

// permitAffordable: cash covers the fee, the rent due and permitMargin; while a mortgage is
// paid, the fee and two payments (Claude play 5).
func (s *snapshot) permitAffordable() bool {
	if s.Cash == nil {
		return false
	}
	if s.paysMortgage() {
		return *s.Cash >= s.permitFee()+2**s.Property.MortgageWeekly
	}
	due, _ := s.Rent.due()
	return *s.Cash >= s.permitFee()+due+permitMargin
}

// permitFee is the business permit's price on the shop's difficulty (docs/re/MITIGATIONS.md).
func (s *snapshot) permitFee() int64 {
	switch strings.ToLower(s.storeExtra["difficulty"]) {
	case "cozy":
		return 500
	case "hard":
		return 2000
	case "challenge":
		return 3000
	}
	return 1500
}

func (s *snapshot) paysMortgage() bool {
	p := s.Property
	return p != nil && p.MortgageWeekly != nil && *p.MortgageWeekly > 0
}

func appendMissingUIDs(list, uids []int64) []int64 {
	for _, u := range uids {
		if !slices.Contains(list, u) {
			list = append(list, u)
		}
	}
	return list
}

func (s *snapshot) matchService(ref string) (string, bool) {
	ref = normID(ref)
	if ref == "" {
		return "", false
	}
	if len(s.Services) == 0 {
		return strings.ToUpper(strings.ReplaceAll(ref, " ", "_")), true
	}
	for _, v := range s.Services {
		id := normID(v.ID)
		if ref == id || ref+" service" == id || ref == id+" service" || (v.Name != "" && ref == normID(v.Name)) {
			return v.ID, v.ID != ""
		}
	}
	return "", false
}

func (s *snapshot) matchUpgrade(ref string) (upgradeOffer, bool) {
	ref = normID(ref)
	for _, u := range s.UpgradesOffer {
		if ref != "" && u.ID != "" && (ref == normID(u.ID) || ref == normID(u.Name)) {
			return u, true
		}
	}
	return upgradeOffer{}, false
}

func normID(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(s, "_", " "))), " ")
}

// hotRoom is the most hot items the smuggler bays hold: those already inside and one per
// free cell. ok is false when a bay does not report its room.
func (s *snapshot) hotRoom() (int64, bool) {
	bays, _, free, known := s.bayRoom()
	if bays == 0 {
		return 0, true
	}
	if !known {
		return 0, false
	}
	var in int64
	for _, it := range s.Inventory {
		if it.stolen() && *it.StolenHeat > 0 && s.inBay(it) {
			in++
		}
	}
	return in + free, true
}

// holdUIDs are the items not to sell today: what the catalogue keeps for the shop's use,
// and stolen items still cooling unless rent forces a sale.
func (s *snapshot) holdUIDs() []int64 {
	set := map[int64]bool{}
	for u := range s.keepUIDs(s.production()) {
		set[u] = true
	}
	due, rentKnown := s.Rent.due()
	forced := rentKnown && s.Cash != nil && *s.Cash < due
	for _, it := range s.Inventory {
		if it.UID != nil && it.stolen() && it.coolDays() > 0 && !forced {
			set[*it.UID] = true
		}
	}
	out := make([]int64, 0, len(set))
	for u := range set {
		out = append(out, u)
	}
	slices.Sort(out)
	return out
}

// planWindowUIDs are the clean items the plan's WINDOW section puts in the window.
func (s *snapshot) planWindowUIDs(plan string) []int64 {
	var out []int64
	for _, line := range section(plan, "WINDOW") {
		l := strings.ToLower(line)
		if mentionsAny(l, []string{"remove", "take out", "pull", "never", "not "}) {
			continue
		}
		for _, m := range uidRE.FindAllStringSubmatch(l, -1) {
			u, err := strconv.ParseInt(m[1], 10, 64)
			if err != nil || slices.Contains(out, u) {
				continue
			}
			if it, ok := s.itemByUID(u); ok && it.displayable() {
				out = append(out, u)
			}
		}
	}
	return out
}

// calendarTypes are the item types a market event in the next calendarDays raises
// (buy ahead) or lowers (do not stock up).
func (s *snapshot) calendarTypes(day int) (prefer, avoid []string) {
	up, down := map[string]bool{}, map[string]bool{}
	for _, cd := range s.Calendar {
		if cd.Day <= int64(day) || cd.Day > int64(day+calendarDays) {
			continue
		}
		for _, e := range cd.Events {
			for i, t := range e.types {
				switch {
				case t == "":
				case e.pcts[i] > 0:
					up[t] = true
				case e.pcts[i] < 0:
					down[t] = true
				}
			}
		}
	}
	for t := range up {
		if !down[t] {
			prefer = append(prefer, t)
		}
	}
	for t := range down {
		if !up[t] {
			avoid = append(avoid, t)
		}
	}
	slices.Sort(prefer)
	slices.Sort(avoid)
	return prefer, avoid
}

// scavengeTonight: ok when the state says whether the dumping ground is open; open is
// false when the shop is freshly wounded.
func (s *snapshot) scavengeTonight() (open, ok bool) {
	ps := s.production()
	if !ps.has["scavenge"] {
		return false, false
	}
	wounded := ps.scav != nil && ps.scav.WoundedFresh != nil && *ps.scav.WoundedFresh
	return !wounded, true
}

func (s *snapshot) itemByUID(u int64) (item, bool) {
	for _, it := range s.Inventory {
		if it.UID != nil && *it.UID == u {
			return it, true
		}
	}
	return item{}, false
}

// knownUIDs are every uid the state names: inventory, machines, plants, bottles, water.
func (s *snapshot) knownUIDs() map[int64]bool {
	set := map[int64]bool{}
	add := func(u *int64) {
		if u != nil {
			set[*u] = true
		}
	}
	for _, it := range s.Inventory {
		add(it.UID)
	}
	ps := s.production()
	for _, m := range ps.machines {
		add(m.UID)
	}
	for _, h := range ps.hydro {
		add(h.UID)
	}
	for _, w := range ps.wine {
		add(w.UID)
	}
	for _, f := range ps.furnaces {
		add(f.UID)
	}
	for _, w := range ps.water {
		add(w.UID)
	}
	return set
}

// section returns the body lines of one plan heading.
func section(plan, heading string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(plan, "\n") {
		switch {
		case l == heading:
			in = true
		case slices.Contains(Headings, l):
			in = false
		case in:
			out = append(out, l)
		}
	}
	return out
}

func clampNote(n, lo, hi int64, key string, note func(string, ...any)) *int64 {
	c := min(max(n, lo), hi)
	if c != n {
		note("%s: %d clamped to %d", key, n, c)
	}
	return &c
}

// num reads an integer the way a small model writes one: 70, 70.0, "70", "1,350", "70%".
func num(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || math.Abs(x) > 1e15 {
			return 0, false
		}
		return int64(math.Round(x)), true
	case string:
		t := strings.TrimSpace(strings.NewReplacer(",", "", "%", "", "#", "").Replace(x))
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n, true
		}
	}
	return 0, false
}

func anyUID(v any) (int64, bool) {
	n, ok := num(v)
	return n, ok && n >= 0
}

func anyUIDs(v any) []int64 {
	xs, _ := v.([]any)
	var out []int64
	for _, x := range xs {
		if u, ok := anyUID(x); ok && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

func typeList(v any) []string {
	xs, _ := v.([]any)
	var out []string
	for _, x := range xs {
		t, _ := x.(string)
		t = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(t)), " ", "_")
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

func enumOf(v any, allowed []string) (string, bool) {
	t, _ := v.(string)
	t = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(t)), "-", "_")
	return t, slices.Contains(allowed, t)
}

func ptrTo[T any](v T) *T { return &v }
