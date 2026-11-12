package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Delta directives: the executor's own rules are the base (an empty object plays them) and
// the strategist names only the changes it has a reason for. In the league the full
// directives played worse than the executor alone (seed 90001 normal: rent on day 104
// against 174), because every field they fill overrides a tuned default.

// executorRules, deltaWrite and deltaKeys make up deltaAsk; the hybrid proposer (propose.go)
// shows the rules and the keys with its own instructions.
const (
	executorRules = `The executor already plays every day by its own tuned rules; an empty object {} keeps all of them:
- buying: wholesale and single lots only below their estimate, prefers types a calendar rise names, keeps the rent from 3 days out;
- selling: at least 90% of the estimate, down to 65% when cash is short of rent; holds stolen goods until heat 0;
- services: insurance and janitorial while tonight's bill is payable; power only while something owned draws it;
- upgrades: Landlord Grace first, then the engine's essentials, then RENOVATION1, SHOWCASE_I, RENOVATION2, RENOVATION3, SHOWCASE_II from favor or real surplus only;
- permit: buys it once the fee leaves rent + 500, else donates goods (bribe) from day 45;
- wine engine on (normal), off on hard; scavenges when open; shoots augs; bargains only when the odds pay;
- mortgage: on a rent-morning offer it signs when the cash after the down payment keeps 1.5 payments + mortgage_float (1,000) and the cash after the down payment + half the repossessable stock + 80% of the term's (weekly gain + the rent it replaces) covers mortgage_margin (1.0) x all the payments; otherwise it answers "later" and the offer comes back next week. With buyout_by_day set, it answers "later" while the cash buyout (cash + 70% of the stock + the weekly gain) projects to cover the landlord's price by that day, and buys the shop for cash.
`
	deltaWrite = `Write ONLY the changes your plan needs today, as one JSON object with any of the keys below, plus "why": {"<key>": "<one line: the fact in today's state that makes the default wrong>"}, for example {"permit": "buy", "why": {"permit": "cash 2,600 covers the fee, rent 240 and 500"}}. A key without a why is ignored. Deviate only with a specific reason from today's state; when the defaults fit, answer {}.
`
	deltaKeys = `Keys: "cash_floor", "buy" {"wholesale_max_pct","single_max_pct","max_spend","prefer_types","avoid_types","hot_stock_cap"}, "sell" {"min_pct","sell_down_to_cash","hold_uids"}, "bargain" ("never"|"ev_positive"|"aggressive"), "window" {"show_uids"}, "services" [ids], "upgrades_buy" [ids, in order], "permit" ("buy"|"bribe"), "augs", "scavenge", "engine" ("wine", "water", "furnace", "rats" joined by "+", or "none"), "mortgage" ("auto": the rule above | "take": sign at the next offer | "later": not today, the offer comes back next week | "decline": FINAL, the loan shark never offers again, so the only win left is the cash buyout | "accept_if_rent": sign only to bridge a rent the cash cannot pay), "buyout_by_day" (answer "later" while the cash buyout projects by this day), "mortgage_margin" (a decimal), "mortgage_float", "mortgage_window" [first day, last day] it may sign, "mortgage_save_from_day" (from that day stop buying and save toward the offer), "mortgage_max_spend" (daily buying cap while paying), "appraisal" ("join": the appraisal board's clients and fees | "off").
Numbers are plain integers (mortgage_margin may be a decimal); a uid is the number after "#".`
	deltaAsk = executorRules + deltaWrite + deltaKeys
	// betAsk is shown when the run keeps bets (Options.Bets): the show calls each one back
	// when it is settled, so a filler bet is noise on stream.
	betAsk = `
BET (optional; at most one open at a time; it needs no "why"): a claim about this run that today's plan stakes and that a viewer can check later. Add "bet" only when the plan commits to something you could be wrong about (holding stock for a rise, a cash or net worth target, an upgrade by a day, a payment you mean to cover). Never a filler bet, never one already true, never a restatement of the rules. Shape: "bet": {"claim": "<one sentence in your voice, at most 120 characters, with its number and day>", "kind": "<kind>", "by": <deadline day: tomorrow at the earliest, at most 42 days from today; a bet needs time to play out>, plus the kind's fields}. Kinds: "cash" or "net_worth" with "target": reaches it by the day; "sell" with "uids" (or "item": a name in stock) and an optional "target", the least the sale brings: sold by the day; "hold" with "uids" or "item": still held at the day's close; "upgrade" with "item" (an upgrade id on offer): bought by the day; "event" with "item" (a market item type): a price rise for it is active by the day; "payment": the next rent or mortgage payment made by the day, none missed; "judged": anything else, which you judge yourself at the deadline from the state. An upgrade bet needs "upgrades_buy" to name it (the executor buys nothing else); a bet stands until its deadline, so drop one only when the state has changed against it; a bet just settled is not staked again for 3 days.`
)

// DraftDeltaDirectives asks c, in the plan's conversation, for today's changes to the
// executor's defaults, and returns them linted. A failed call or unusable answer plays the
// defaults.
func DraftDeltaDirectives(ctx context.Context, c llm.Client, d *PlanDraft) DirectivesResult {
	prompt := deltaAsk + baseNote(d.base) + deltaFacts(d.snap)
	if d.bets {
		prompt += betAsk + "\n" + d.bet
	}
	msgs := append(slices.Clip(d.messages), llm.Message{Role: llm.RoleUser, Content: prompt})
	var raw json.RawMessage
	resp, err := c.Complete(ctx, llm.Request{System: d.system, Messages: msgs, MaxTokens: directivesMaxTokens})
	if err == nil {
		raw, err = llm.ExtractJSON(resp.Text)
	}
	return finishDelta(raw, err, d.snap, d.day, d.base)
}

func finishDelta(raw json.RawMessage, rawErr error, s *snapshot, day int, base string) DirectivesResult {
	r := DirectivesResult{Err: rawErr}
	if rawErr != nil {
		r.Directives.Day = day
		return r
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		r.Err = fmt.Errorf("directives are not a JSON object: %w", err)
		r.Directives.Day = day
		return r
	}
	r.Bet, r.Fixes = betMove(m)
	why, _ := m["why"].(map[string]any)
	// Models often give one reason for every change.
	all, _ := m["why"].(string)
	delete(m, "why")
	r.Why = reasons(why, all)
	for k := range m {
		if w, _ := why[k].(string); strings.TrimSpace(w) == "" && strings.TrimSpace(all) == "" {
			r.Fixes = append(r.Fixes, fmt.Sprintf("%s: no reason given; the default stands", k))
			delete(m, k)
		}
	}
	if len(m) > 0 {
		b, _ := json.Marshal(m)
		var present map[string]bool
		var fixes []string
		var err error
		r.Directives, present, fixes, err = parseDirectives(b, s)
		r.Fixes = append(r.Fixes, fixes...)
		if err != nil {
			r.Err = err
		}
		for _, k := range directiveKeys {
			if present[k] {
				r.FromModel = append(r.FromModel, k)
			}
		}
	}
	r.Directives.Day = day
	r.Fixes = append(r.Fixes, lintDelta(&r.Directives, s, day)...)
	r.Fixes = append(r.Fixes, lintBaseEngine(&r.Directives, base)...)
	r.Fixes = append(r.Fixes, lintDecline(&r.Directives, s)...)
	r.Fixes = append(r.Fixes, protectBuying(&r.Directives, s)...)
	r.Fixes = append(r.Fixes, lintLocked(&r.Directives, base)...)
	return r
}

// betMove takes the bet keys out of a delta answer: "bet" (an object, "drop" or "keep") and
// "bet_verdict" ({"won": bool, "why"}). An unreadable bet is dropped with a fix.
func betMove(m map[string]any) (*protocol.BetMove, []string) {
	var mv protocol.BetMove
	var fixes []string
	switch v := m["bet"].(type) {
	case string:
		mv.Drop = strings.EqualFold(strings.TrimSpace(v), "drop")
	case map[string]any:
		b, _ := json.Marshal(v)
		var bet protocol.Bet
		if err := json.Unmarshal(b, &bet); err != nil {
			fixes = append(fixes, "bet dropped: "+err.Error())
		} else {
			mv.Stake = &bet
		}
	}
	if v, ok := m["bet_verdict"].(map[string]any); ok {
		won, isBool := v["won"].(bool)
		if o, _ := v["outcome"].(string); !isBool && o != "" {
			won, isBool = strings.EqualFold(o, "won"), true
		}
		if isBool {
			why, _ := v["why"].(string)
			mv.Verdict = &protocol.BetVerdict{Won: won, Why: strings.TrimSpace(why)}
		}
	}
	delete(m, "bet")
	delete(m, "bet_verdict")
	if why, ok := m["why"].(map[string]any); ok {
		delete(why, "bet")
		delete(why, "bet_verdict")
	}
	if mv == (protocol.BetMove{}) {
		return nil, fixes
	}
	return &mv, fixes
}

// lintDelta reverts the deviations that have cost runs, leaving the executor's default in
// their place.
func lintDelta(d *protocol.Directives, s *snapshot, day int) []string {
	var fixes []string
	note := func(format string, a ...any) { fixes = append(fixes, fmt.Sprintf(format, a...)) }
	if day%7 == supplierWeekday {
		// Claude play 3: a cash cap on supplier day starves the week's buying.
		if d.CashFloor != nil {
			note("cash_floor reverted: supplier day spends down to the bare rent")
			d.CashFloor = nil
		}
		if d.Buy != nil && d.Buy.MaxSpend != nil {
			note("buy.max_spend reverted: supplier day spends down to the bare rent")
			d.Buy.MaxSpend = nil
		}
	}
	if s.Cash != nil && d.CashFloor != nil && *d.CashFloor > *s.Cash {
		note("cash_floor %d is above cash %d; reverted", *d.CashFloor, *s.Cash)
		d.CashFloor = nil
	}
	if len(d.UpgradesBuy) > 0 {
		// A listed upgrade skips the executor's surplus gate (Claude play 2, 3): only what
		// favor pays or what leaves the rent and the margin stays.
		kept := s.affordableUpgrades(d.UpgradesBuy, note)
		if len(kept) == 0 {
			d.UpgradesBuy = nil
		} else {
			// The list replaces the executor's engine purchases; keep the essential one.
			if _, ok := s.offered("WINEMAKER"); ok && !s.hard() {
				kept = appendMissing(kept, "WINEMAKER")
			}
			d.UpgradesBuy = kept
		}
	}
	if s.Permit == nil || !*s.Permit {
		switch {
		case d.Permit == "refuse":
			note("permit: refuse reverted (arrest by day 80)")
			d.Permit = ""
		case d.Permit == "" && s.paysMortgage() && !s.permitAffordable():
			note("permit: bribe, the mortgage leaves too little for the fee and two payments")
			d.Permit = "bribe"
		case d.Permit == "bribe" && s.permitAffordable():
			note("permit: bribe reverted, cash covers the fee, rent and 500 (a bribed permit keeps logging crime)")
			d.Permit = ""
		}
	} else if d.Permit != "" {
		d.Permit = ""
	}
	if d.Bargain == "aggressive" {
		note("bargain: aggressive reverted (-13 to -19 days, gym)")
		d.Bargain = ""
	}
	if ll, ok := s.rep("lower_levels"); ok && ll < bargainLLFloor && d.Bargain == "ev_positive" {
		note("bargain: never, Lower Levels %.0f is below %d", ll, bargainLLFloor)
		d.Bargain = "never"
	}
	if ll, ok := s.rep("lower_levels"); ok && ll < augServeLL && !s.hard() {
		if _, ends := s.augCondition(day); !ends && d.Augs == "" {
			// Claude play 4: each shot aug disguised as Lower Levels costs about 5 rep.
			note("augs: serve, Lower Levels %.0f is below %d and aug presence does not end this run", ll, augServeLL)
			d.Augs = "serve"
		}
	}
	if d.Services != nil {
		for _, id := range []string{"INSURANCE", "JANITORIAL"} {
			if !slices.ContainsFunc(d.Services, func(x string) bool { return strings.Contains(strings.ToUpper(x), id) }) {
				note("services: %s kept on (gym, +21 days)", id)
				for _, w := range s.wantedServices() {
					if strings.Contains(w, id) {
						d.Services = append(d.Services, w)
					}
				}
			}
		}
		if !s.drawsPower() {
			d.Services = slices.DeleteFunc(d.Services, func(id string) bool { return strings.EqualFold(id, "POWER_SERVICE") })
		}
	}
	if d.Sell != nil && d.Sell.MinPct != nil && d.Sell.SellDownToCash != nil {
		note("sell.min_pct dropped while selling down")
		d.Sell.MinPct = nil
	}
	if d.Sell != nil {
		short := s.Cash != nil && *s.Cash < s.dueOrZero()
		d.Sell.HoldUIDs = slices.DeleteFunc(d.Sell.HoldUIDs, func(u int64) bool {
			it, ok := s.itemByUID(u)
			if g, wine := wineGrade(it); ok && wine && !slices.Contains(s.wineHolds(), u) {
				note("sell.hold_uids: released %s wine #%d (liquidity before aging)", wineGrades[g], u)
				return true
			}
			return short && !slices.Contains(s.holdUIDs(), u)
		})
	}
	if d.Window != nil {
		d.Window.ShowUIDs = slices.DeleteFunc(d.Window.ShowUIDs, func(u int64) bool {
			it, ok := s.itemByUID(u)
			return ok && !it.displayable()
		})
	}
	if _, ok := s.augCondition(day); ok && d.Augs == "serve" {
		note("augs: serve reverted where aug presence ends the run")
		d.Augs = ""
	}
	if d.Scavenge != nil && !*d.Scavenge {
		if open, ok := s.scavengeTonight(); ok && open {
			note("scavenge: false reverted, the dumping ground is open")
			d.Scavenge = nil
		}
	}
	if d.Engine == "none" && s.hard() {
		d.Engine = "" // the executor's default on hard
	} else if d.Engine == "none" && !s.engineIdle() {
		note("engine: none reverted; the wine engine has not stalled")
		d.Engine = ""
	}
	return fixes
}

// lintBaseEngine reverts an engine that only names part of the base's: models restate
// "wine" as a keep, which would switch off the base's furnace.
func lintBaseEngine(d *protocol.Directives, base string) []string {
	if d.Engine == "" || d.Engine == "none" || base == "" {
		return nil
	}
	var b struct {
		Engine string `json:"engine"`
	}
	if json.Unmarshal([]byte(base), &b) != nil || b.Engine == "" {
		return nil
	}
	have := strings.Split(b.Engine, "+")
	want := strings.Split(d.Engine, "+")
	if len(want) >= len(have) || slices.ContainsFunc(want, func(p string) bool { return !slices.Contains(have, p) }) {
		return nil
	}
	e := d.Engine
	d.Engine = ""
	return []string{fmt.Sprintf("engine: %s reverted, a part of the base's %s", e, b.Engine)}
}

// declineWeeks is how far ahead a decline's cash buyout must project: the offer comes back
// every week, and a decline ends it for good.
const declineWeeks = 4

// lintDecline turns a decline into "later" unless the cash buyout already projects: a
// declined loan shark never returns (seed 2100005: the model declined, then asked to take
// four weeks later and lost to rent).
func lintDecline(d *protocol.Directives, s *snapshot) []string {
	if d.Mortgage != "decline" {
		return nil
	}
	if p, price, ok := s.cashBuyoutProjection(declineWeeks); ok && p >= price {
		return nil
	} else if ok {
		d.Mortgage = "later"
		return []string{fmt.Sprintf("mortgage: decline reverted to later, decline is final and the cash buyout projects to %d of %d in %d weeks", p, price, declineWeeks)}
	}
	d.Mortgage = "later"
	return []string{"mortgage: decline reverted to later, decline is final and the cash buyout cannot be projected"}
}

// baseNote tells the model the run's base policy: without it, "{} keeps the defaults" is
// wrong, and a change it believes new may already be in force.
func baseNote(base string) string {
	if strings.TrimSpace(base) == "" {
		return ""
	}
	return "\nThis run's executor plays a base policy over those rules; {} keeps it, and each field you send overrides the base's (nested objects merge field by field): " + base
}

// deltaFacts are the ids and states the changes may name.
func deltaFacts(s *snapshot) string {
	var b strings.Builder
	var svc []string
	for _, v := range s.Services {
		if v.ID != "" && (v.Unlocked == nil || *v.Unlocked) {
			on := "off"
			if v.inUse() {
				on = "on"
			}
			svc = append(svc, v.ID+" ("+on+")")
		}
	}
	if len(svc) > 0 {
		fmt.Fprintf(&b, "\nService ids: %s.", strings.Join(svc, ", "))
	}
	if f := s.favorUpgrades(); len(f) > 0 {
		fmt.Fprintf(&b, "\nUpgrades not yet bought (favor order): %s; a listed upgrade skips the executor's surplus check, so list one only when favor pays it or cash keeps rent + %d after it.", strings.Join(f, ", "), upgradeCashMargin)
	}
	if s.Permit != nil && !*s.Permit {
		fmt.Fprintf(&b, "\nBusiness permit: not held (%d).", s.permitFee())
	}
	if s.Day != nil && *s.Day%7 == supplierWeekday {
		b.WriteString("\nToday is supplier day: no cash_floor or max_spend.")
	}
	return b.String()
}

func reasons(why map[string]any, all string) map[string]string {
	out := map[string]string{}
	for k, v := range why {
		if w, _ := v.(string); strings.TrimSpace(w) != "" {
			out[k] = strings.TrimSpace(w)
		}
	}
	if strings.TrimSpace(all) != "" {
		out["*"] = strings.TrimSpace(all)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
