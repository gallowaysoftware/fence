// Package bet keeps the strategist's bet (docs/PROTOCOL.md, "Bets"): one checkable claim at a
// time, checked against player-visible state only. The bridge and the show feed a Tracker the
// same decision-log events (snapshots, sales, rent), so they settle a bet on the same day.
package bet

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Outcomes. Staked is a new bet; Due a judged bet past its deadline, waiting for the model's
// verdict; Expired a due bet the model never judged.
const (
	Staked    = "staked"
	Won       = "won"
	Lost      = "lost"
	Abandoned = "abandoned"
	Due       = "due"
	Expired   = "expired"
)

// MaxDays is how far ahead a bet's deadline may be.
const MaxDays = 42

// Resolution is a bet being staked or settled.
type Resolution struct {
	Bet     protocol.Bet
	Outcome string
	Day     int
	// Fact is what settled it, in words ("cash 9,120 on day 51"); "" for a stake.
	Fact string
	// Judged is set when the model judged it rather than the state.
	Judged bool
}

// Tracker holds the run's open bet and what has happened since it was staked.
type Tracker struct {
	open *protocol.Bet
	due  bool
	// held is the last snapshot's inventory: uid → name.
	held     map[int64]string
	sold     map[int64]string
	soldCash int64
	paid     int
	missed   int
	// seen is the latest snapshot's day: a plan that lands after its day ended is late.
	seen int
	// last is the latest bet settled and its day, so the same bet is not staked again at once.
	last    *protocol.Bet
	lastDay int
}

// RepeatDays is how long a settled bet may not be staked again: four upgrade bets in a row,
// each lost the same night, are one story told four times.
const RepeatDays = 3

// Late reports whether b's deadline passed before the plan staking it landed.
func (t *Tracker) Late(b *protocol.Bet) bool { return b.By < t.seen }

// Repeats reports whether b is the bet settled within the last RepeatDays.
func (t *Tracker) Repeats(b *protocol.Bet, day int) bool {
	return t.last != nil && day-t.lastDay < RepeatDays && t.last.Kind == b.Kind && strings.EqualFold(t.last.Item, b.Item) && slices.Equal(t.last.UIDs, b.UIDs)
}

// Open is the bet in force, nil for none.
func (t *Tracker) Open() *protocol.Bet {
	if t.open == nil {
		return nil
	}
	b := *t.open
	return &b
}

// IsDue reports whether the open bet is a judged one waiting for its verdict.
func (t *Tracker) IsDue() bool { return t.open != nil && t.due }

// view is the part of a day_snapshot a bet is checked against.
type view struct {
	Phase string `json:"phase"`
	Store struct {
		Day      int    `json:"day"`
		Cash     *int64 `json:"cash"`
		NetWorth *int64 `json:"net_worth"`
	} `json:"store"`
	Inventory []struct {
		UID      int64    `json:"uid"`
		Name     string   `json:"name"`
		Type     string   `json:"type"`
		ItemType string   `json:"item_type"`
		Types    []string `json:"types"`
	} `json:"inventory"`
	Clientele *struct {
		DisplayBuyers []string `json:"display_buyers"`
	} `json:"clientele"`
	Upgrades *struct {
		Owned     []struct{ ID, Name string } `json:"owned"`
		Available []struct{ ID, Name string } `json:"available"`
	} `json:"upgrades"`
	MarketEvents *struct {
		Active []struct {
			Name      string `json:"name"`
			StartDay  int    `json:"start_day"`
			Modifiers []struct {
				ItemType string `json:"item_type"`
				ValuePct int    `json:"value_pct"`
			} `json:"modifiers"`
		} `json:"active"`
	} `json:"market_events"`
}

func parse(raw json.RawMessage, gameDay int) (*view, bool) {
	var v view
	if json.Unmarshal(raw, &v) != nil {
		return nil, false
	}
	if v.Store.Day == 0 {
		v.Store.Day = gameDay
	}
	return &v, true
}

// Snapshot checks the open bet against a day_snapshot event's data.
func (t *Tracker) Snapshot(raw json.RawMessage, gameDay int) []Resolution {
	v, ok := parse(raw, gameDay)
	if !ok {
		return nil
	}
	t.seen = max(t.seen, v.Store.Day)
	if v.Inventory != nil {
		t.held = map[int64]string{}
		for _, it := range v.Inventory {
			t.held[it.UID] = it.Name
		}
	}
	return t.check(v)
}

// Sold notes a sale: the items' uids and the cash it brought.
func (t *Tracker) Sold(uids []int64, cash int64) {
	if t.open == nil {
		return
	}
	if t.sold == nil {
		t.sold = map[int64]string{}
	}
	mine := false
	for _, u := range uids {
		t.sold[u] = t.held[u]
		mine = mine || t.names(u)
	}
	if mine {
		t.soldCash += cash
	}
}

// names reports whether item u is one the open bet names.
func (t *Tracker) names(u int64) bool {
	b := t.open
	if len(b.UIDs) > 0 {
		return slices.Contains(b.UIDs, u)
	}
	return b.Item != "" && strings.Contains(strings.ToLower(t.held[u]), strings.ToLower(b.Item))
}

// Money notes a money event (rent_paid, rent_missed, mortgage_payment) on day and checks a
// payment bet at once.
func (t *Tracker) Money(kind string, day int) []Resolution {
	if t.open == nil || t.open.Kind != "payment" {
		return nil
	}
	switch kind {
	case "rent_paid", "mortgage_payment", "mortgage_paid":
		t.paid = day
		return t.settle(Won, day, fmt.Sprintf("paid on day %d", day))
	case "rent_missed":
		t.missed = day
		return t.settle(Lost, day, fmt.Sprintf("missed on day %d", day))
	}
	return nil
}

func (t *Tracker) settle(outcome string, day int, fact string) []Resolution {
	r := Resolution{Bet: *t.open, Outcome: outcome, Day: day, Fact: fact}
	if outcome == Due {
		t.due = true
		return []Resolution{r}
	}
	t.last, t.lastDay = t.open, day
	t.open, t.due = nil, false
	return []Resolution{r}
}

func (t *Tracker) check(v *view) []Resolution {
	b := t.open
	if b == nil || t.due {
		return nil
	}
	day := v.Store.Day
	// A bet "by day N" stands through day N and its night (upgrades are bought after closing).
	over := day > b.By || (day == b.By && v.Phase == "sleep")
	reach := func(what string, have *int64) []Resolution {
		switch {
		case have == nil:
			return nil
		case *have >= b.Target:
			return t.settle(Won, day, fmt.Sprintf("%s %s on day %d", what, num(*have), day))
		case over:
			return t.settle(Lost, day, fmt.Sprintf("%s %s on day %d, short of %s", what, num(*have), day, num(b.Target)))
		}
		return nil
	}
	switch b.Kind {
	case "cash":
		return reach("cash", v.Store.Cash)
	case "net_worth":
		return reach("net worth", v.Store.NetWorth)
	case "sell":
		if name, ok := t.allSold(); ok {
			if b.Target > 0 && t.soldCash < b.Target {
				return t.settle(Lost, day, fmt.Sprintf("%s sold by day %d for %s, under %s", name, day, num(t.soldCash), num(b.Target)))
			}
			fact := fmt.Sprintf("%s sold by day %d", name, day)
			if t.soldCash > 0 {
				fact += " for " + num(t.soldCash)
			}
			return t.settle(Won, day, fact)
		}
		if over {
			return t.settle(Lost, day, fmt.Sprintf("not sold by day %d", b.By))
		}
	case "hold":
		if name, gone := t.gone(); gone {
			return t.settle(Lost, day, fmt.Sprintf("%s gone by day %d", name, day))
		}
		if over {
			return t.settle(Won, day, fmt.Sprintf("still held on day %d", day))
		}
	case "upgrade":
		if v.Upgrades != nil && slices.ContainsFunc(v.Upgrades.Owned, func(u struct{ ID, Name string }) bool { return sameUpgrade(b.Item, u.ID, u.Name) }) {
			return t.settle(Won, day, fmt.Sprintf("bought by day %d", day))
		}
		if over {
			return t.settle(Lost, day, fmt.Sprintf("not bought by day %d", b.By))
		}
	case "event":
		if name, pct, ok := rise(v, b.Item); ok {
			return t.settle(Won, day, fmt.Sprintf("%s on day %d: %s up %d%%", name, day, b.Item, pct))
		}
		if over {
			return t.settle(Lost, day, fmt.Sprintf("no %s rise by day %d", b.Item, b.By))
		}
	case "payment":
		if over && t.paid == 0 && t.missed == 0 {
			return t.settle(Lost, day, fmt.Sprintf("no payment by day %d", b.By))
		}
	case "judged":
		if day > b.By {
			return t.settle(Due, day, "")
		}
	}
	return nil
}

// allSold reports whether every item the bet names has been sold, and what to call them.
func (t *Tracker) allSold() (string, bool) {
	b := t.open
	if len(b.UIDs) > 0 {
		for _, u := range b.UIDs {
			if _, ok := t.sold[u]; !ok {
				return "", false
			}
		}
		return itemWords(b, t.sold[b.UIDs[0]]), true
	}
	for _, n := range t.sold {
		if b.Item != "" && strings.Contains(strings.ToLower(n), strings.ToLower(b.Item)) {
			return itemWords(b, n), true
		}
	}
	return "", false
}

// gone reports whether an item a hold bet names has left the stock.
func (t *Tracker) gone() (string, bool) {
	b := t.open
	if t.held == nil {
		return "", false
	}
	if len(b.UIDs) > 0 {
		for _, u := range b.UIDs {
			_, kept := t.held[u]
			if name, sold := t.sold[u]; sold || !kept {
				return itemWords(b, name), true
			}
		}
		return "", false
	}
	for _, n := range t.held {
		if strings.Contains(strings.ToLower(n), strings.ToLower(b.Item)) {
			return "", false
		}
	}
	return itemWords(b, ""), true
}

func itemWords(b *protocol.Bet, name string) string {
	switch {
	case b.Item != "":
		return b.Item
	case name != "":
		return name
	}
	return "the items"
}

// sameUpgrade matches an upgrade by id or name as models write them ("Red_Imp_Brewery").
func sameUpgrade(want, id, name string) bool {
	w := upgradeKey(want)
	return w != "" && (w == upgradeKey(id) || w == upgradeKey(name))
}

func upgradeKey(s string) string {
	return strings.ToLower(strings.NewReplacer("_", "", " ", "", "-", "").Replace(s))
}

// rise finds an active market event raising itemType.
func rise(v *view, itemType string) (name string, pct int, ok bool) {
	if v.MarketEvents == nil || itemType == "" {
		return "", 0, false
	}
	for _, e := range v.MarketEvents.Active {
		if e.StartDay > v.Store.Day {
			continue
		}
		for _, m := range e.Modifiers {
			if m.ValuePct > 0 && strings.EqualFold(m.ItemType, itemType) {
				return e.Name, m.ValuePct, true
			}
		}
	}
	return "", 0, false
}

// Apply takes a plan's move on day: a verdict for a due bet, a drop, or a new stake (which
// abandons an open one unless it is the same bet again). A drop of a due bet expires it; a
// nil move (a day without a plan) leaves it waiting, so weekly plans judge it when they come.
func (t *Tracker) Apply(m *protocol.BetMove, day int) []Resolution {
	var out []Resolution
	if t.IsDue() {
		switch {
		case m != nil && m.Verdict != nil:
			outcome := Lost
			if m.Verdict.Won {
				outcome = Won
			}
			r := t.settle(outcome, day, strings.TrimSpace(m.Verdict.Why))
			r[0].Judged = true
			out = append(out, r...)
		case m != nil && m.Drop:
			out = append(out, t.settle(Expired, day, "no verdict")...)
		}
	}
	if m == nil {
		return out
	}
	if m.Drop && t.open != nil {
		out = append(out, t.settle(Abandoned, day, fmt.Sprintf("dropped on day %d", day))...)
	}
	if s := m.Stake; s != nil {
		if (t.open != nil && same(t.open, s)) || t.Late(s) {
			return out
		}
		if t.open != nil {
			out = append(out, t.settle(Abandoned, day, fmt.Sprintf("replaced by a new bet on day %d", day))...)
		}
		b := *s
		b.UIDs = slices.Clone(s.UIDs)
		if b.Day == 0 {
			b.Day = day
		}
		t.open, t.due = &b, false
		t.sold, t.soldCash, t.paid, t.missed = nil, 0, 0, 0
		out = append(out, Resolution{Bet: b, Outcome: Staked, Day: day})
	}
	return out
}

func same(a, b *protocol.Bet) bool {
	return a.Kind == b.Kind && a.Target == b.Target && a.By == b.By && strings.EqualFold(a.Item, b.Item) && slices.Equal(a.UIDs, b.UIDs)
}

// Lint checks a new bet against the morning snapshot it is staked on and normalises it; an
// error says why it cannot stand. A bet already won when staked is filler.
func Lint(b *protocol.Bet, raw json.RawMessage, day int) error {
	b.Claim = strings.TrimSpace(b.Claim)
	if b.Claim == "" {
		return fmt.Errorf("no claim")
	}
	if len(b.Claim) > protocol.MaxBetClaim {
		b.Claim = strings.TrimSpace(b.Claim[:protocol.MaxBetClaim])
	}
	b.Kind = strings.ToLower(strings.TrimSpace(b.Kind))
	if !slices.Contains(protocol.BetKinds, b.Kind) {
		return fmt.Errorf("unknown kind %q", b.Kind)
	}
	// A bet settled the night it is staked has no time to build: in the b1 league most were.
	if b.By <= day || b.By > day+MaxDays {
		return fmt.Errorf("deadline day %d is not within days %d-%d", b.By, day+1, day+MaxDays)
	}
	b.Day = day
	v, ok := parse(raw, day)
	if !ok {
		return nil
	}
	held := map[int64]string{}
	for _, it := range v.Inventory {
		held[it.UID] = it.Name
	}
	switch b.Kind {
	case "cash", "net_worth":
		have := v.Store.Cash
		if b.Kind == "net_worth" {
			have = v.Store.NetWorth
		}
		if b.Target <= 0 {
			return fmt.Errorf("no target")
		}
		if have != nil && *have >= b.Target {
			return fmt.Errorf("already true: %s %s", strings.ReplaceAll(b.Kind, "_", " "), num(*have))
		}
	case "sell", "hold":
		b.UIDs = slices.DeleteFunc(b.UIDs, func(u int64) bool { _, ok := held[u]; return !ok })
		if len(b.UIDs) == 0 && !slices.ContainsFunc(sortedNames(held), func(n string) bool {
			return b.Item != "" && strings.Contains(strings.ToLower(n), strings.ToLower(b.Item))
		}) {
			return fmt.Errorf("names no item in stock")
		}
		if b.Item == "" {
			b.Item = held[b.UIDs[0]]
		}
	case "upgrade":
		if v.Upgrades == nil {
			return nil
		}
		for _, u := range v.Upgrades.Owned {
			if sameUpgrade(b.Item, u.ID, u.Name) {
				return fmt.Errorf("already true: %s is owned", b.Item)
			}
		}
		i := slices.IndexFunc(v.Upgrades.Available, func(u struct{ ID, Name string }) bool { return sameUpgrade(b.Item, u.ID, u.Name) })
		if i < 0 {
			return fmt.Errorf("upgrade %q is not on offer", b.Item)
		}
		b.Item = v.Upgrades.Available[i].ID
	case "event":
		if b.Item == "" {
			return fmt.Errorf("no item type")
		}
		if _, _, ok := rise(v, b.Item); ok {
			return fmt.Errorf("already true: %s is up", b.Item)
		}
		// Models name events and items as types ("rent_paid_and_grace"); the state cannot
		// check those, so the model judges them.
		if !slices.ContainsFunc(itemTypes(v), func(t string) bool { return strings.EqualFold(t, b.Item) }) {
			b.Kind = "judged"
		}
	}
	return nil
}

// itemTypes are the market item types the snapshot names.
func itemTypes(v *view) []string {
	var out []string
	for _, it := range v.Inventory {
		out = append(append(out, it.Type, it.ItemType), it.Types...)
	}
	if v.Clientele != nil {
		out = append(out, v.Clientele.DisplayBuyers...)
	}
	if v.MarketEvents != nil {
		for _, e := range v.MarketEvents.Active {
			for _, m := range e.Modifiers {
				out = append(out, m.ItemType)
			}
		}
	}
	return slices.DeleteFunc(out, func(t string) bool { return t == "" })
}

func sortedNames(m map[int64]string) []string {
	var out []string
	for _, n := range m {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// num writes n with thousands separators, as the show's captions do.
func num(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// Describe is a bet in words for a prompt: the claim, its stake day and its deadline.
func Describe(b protocol.Bet) string {
	return fmt.Sprintf("%q (staked day %d, due day %d)", b.Claim, b.Day, b.By)
}
