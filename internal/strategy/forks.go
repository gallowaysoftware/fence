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

// Forks are the run's high-stakes decisions put to the model when they arise (docs/STREAM.md,
// "Model agency"): the mortgage, the cash sprint, the fountain, lighting an engine, the
// trading style, and holding stock through a market event. The executor keeps the routine
// work; a fork's answer is a directive patch held over every later day until it expires or
// the fork is asked again.

const forkMaxTokens = 500

type forkOption struct {
	ID, Label string
	// apply patches a day's directives; nil keeps the route's own rule.
	apply func(d *protocol.Directives)
}

type fork struct {
	Kind  string
	Title string
	Facts []string
	// Options are in display order; the route's own rule is the first unless said otherwise.
	Options []forkOption
	// Moment marks the forks the narrator should treat as a big beat.
	Moment bool
	// Until is the last day the choice holds (0: the rest of the run).
	Until int
}

// forkState is what the run remembers of one fork kind.
type forkState struct {
	asked, lastDay int
	choice         string
	apply          func(d *protocol.Directives)
	until          int
	key            string // the event a hold fork was asked for
}

// forkContext is what fork detection reads besides the morning: the route and its base policy.
type forkContext struct {
	start string
	base  map[string]any
}

func (fc forkContext) baseEngine() []string {
	e, _ := fc.base["engine"].(string)
	if e == "" || e == "none" {
		return nil
	}
	return strings.Split(e, "+")
}

func (fc forkContext) has(key string) bool { _, ok := fc.base[key]; return ok }

func (fc forkContext) baseInt(key string) (int64, bool) {
	f, ok := fc.base[key].(float64)
	return int64(f), ok
}

func i64(v int64) *int64 { return &v }

// detectForks lists the forks today's morning raises, given what the run already decided.
func detectForks(s *snapshot, n int, fc forkContext, st map[string]*forkState) []fork {
	var out []fork
	state := func(k string) *forkState {
		if st[k] == nil {
			st[k] = &forkState{}
		}
		return st[k]
	}
	cash := int64(0)
	if s.Cash != nil {
		cash = *s.Cash
	}
	p := s.Property
	owned := p != nil && p.Owned != nil && *p.Owned
	paying := p != nil && p.MortgageWeekly != nil && *p.MortgageWeekly > 0
	due, _ := s.Rent.due()
	common := s.forkFacts(n, fc)

	if f, ok := mortgageFork(s, n, cash, owned, paying, common, state("mortgage")); ok {
		out = append(out, f)
	}
	if ms := state("sprint"); !owned && !paying && ms.asked == 0 && n >= 54 && n <= 70 && fc.has("buyout_by_day") {
		by, _ := fc.baseInt("buyout_by_day")
		from, _ := fc.baseInt("buyout_save_from")
		ins := "the insurance"
		for _, v := range s.Services {
			if strings.Contains(strings.ToUpper(v.ID), "INSURANCE") && v.Cost != nil {
				ins = fmt.Sprintf("the insurance (%d a night)", *v.Cost)
			}
		}
		out = append(out, fork{Kind: "sprint", Title: "The cash sprint: how hard to save for the buyout", Moment: true,
			Facts: append(common, fmt.Sprintf("The route's rule: from day %d cut %s and buy the shop for cash by day %d, telling the loan shark \"later\" while that projects; burglars take ~1 000 of goods a run without insurance, the fee would be ~5 000.", from, ins, by)),
			Options: []forkOption{
				{ID: "sprint", Label: fmt.Sprintf("sprint as the route says: no insurance from day %d, cash buyout by day %d", from, by)},
				{ID: "all_in", Label: "all in: from today cut the insurance and the upgrades, cash buyout by day 112", apply: func(d *protocol.Directives) {
					d.BuyoutByDay, d.BuyoutSave, d.BuyoutSaveFrom = i64(112), []string{"insurance", "upgrades"}, i64(int64(n))
				}},
				{ID: "insured", Label: fmt.Sprintf("keep the insurance and still aim for cash by day %d", by), apply: func(d *protocol.Directives) {
					d.BuyoutSaveFrom = i64(999)
				}},
				{ID: "mortgage", Label: "give up the cash race: sign the loan shark's mortgage at the next offer", apply: signNow},
			}})
	}
	if fs := state("fountain"); slices.Contains(fc.baseEngine(), "water") && !fc.has("fountain") && fs.choice != "buy" && fs.choice != "landlord" &&
		fs.asked < 3 && (fs.lastDay == 0 || n-fs.lastDay >= 7) && !s.owns("VENDING_FOUNTAIN") {
		if u, ok := s.offered("VENDING_FOUNTAIN"); ok && u.Cost != nil && cash >= *u.Cost+due+500 {
			out = append(out, fork{Kind: "fountain", Title: "Wild's water fountain is affordable", Moment: true,
				Facts: append(common, fmt.Sprintf("Wild sells the water fountain for %d; it sells the shop's bottled water by itself every powered night (~300–600 a week once stocked, lab: the fountain route sold ~5 800 a run and won 60/60 at the same median day as without it). The landlord also offers one for +150 weekly rent, compounding.", *u.Cost)),
				Options: []forkOption{
					{ID: "not_now", Label: "not now: keep the cash for stock"},
					{ID: "buy", Label: fmt.Sprintf("buy Wild's fountain (%d)", *u.Cost), apply: func(d *protocol.Directives) { d.Fountain = "buy" }},
					{ID: "landlord", Label: "take the landlord's fountain (+150 weekly rent)", apply: func(d *protocol.Directives) { d.Fountain = "landlord" }},
				}})
		}
	}
	if es := state("engine"); es.asked == 0 && n >= 7 && cash >= 1500+due {
		engine := fc.baseEngine()
		var opts []forkOption
		with := func(e string) func(d *protocol.Directives) {
			return func(d *protocol.Directives) {
				d.Engine = strings.Join(append(slices.Clone(engine), e), "+")
			}
		}
		if !slices.Contains(engine, "furnace") && !s.production().has["furnace"] {
			opts = append(opts, forkOption{ID: "furnace", Label: "light a furnace: smelt scrap into ingots (lab: the smelter is the fastest route, the only engine clearly ahead in cash)", apply: with("furnace")})
		}
		if fc.start == "rancher" && !slices.Contains(engine, "rats") {
			opts = append(opts, forkOption{ID: "rats", Label: "start the rats: breed and feed the cage, cut them up at 400 g for the counter (lab: a week faster on the rancher start)", apply: with("rats")})
		}
		if len(opts) > 0 {
			out = append(out, fork{Kind: "engine", Title: "Cash for a new engine", Facts: common,
				Options: append([]forkOption{{ID: "stay", Label: "stay as the route is: the cash goes to stock and the buyout"}}, opts...)})
		}
	}
	if ss := state("style"); (ss.asked == 0 && n >= 28 || ss.asked == 1 && n >= 63) && !fc.has("specialty") && !fc.has("speculate") && fc.base["bargain"] != "never" {
		out = append(out, styleFork(s, common, ss.choice))
	}
	if f, ok := holdFork(s, n, common, state("hold")); ok {
		out = append(out, f)
	}
	return out
}

func mortgageFork(s *snapshot, n int, cash int64, owned, paying bool, common []string, ms *forkState) (fork, bool) {
	p := s.Property
	if owned || paying || p == nil || p.OfferDown == nil || *p.OfferDown <= 0 || cash < *p.OfferDown || ms.choice == "decline" || ms.choice == "sign" {
		return fork{}, false
	}
	if ms.lastDay != 0 && n-ms.lastDay < 7 {
		return fork{}, false
	}
	terms := fmt.Sprintf("The loan shark offers the mortgage: %d down", *p.OfferDown)
	if p.OfferWeekly != nil && p.OfferWeeks != nil {
		terms += fmt.Sprintf(", then %d a week for %d weeks (%d in all); a missed payment is repossession, the end of the run", *p.OfferWeekly, *p.OfferWeeks, *p.OfferWeekly**p.OfferWeeks)
	}
	return fork{Kind: "mortgage", Title: "The loan shark is at the door", Moment: true,
		Facts: append(common, terms+". Signing ends the rent; declining is final (he never offers again, so the cash buyout is the only win left).",
			"Lab (60-seed gym runs): a signed mortgage wins only at its last payment, 16 weeks after signing (around day 170 when signed near day 55); the routes' cash buyouts win around day 84–112."),
		Options: []forkOption{
			{ID: "later", Label: "later: not this week, he comes back next rent day", apply: func(d *protocol.Directives) { d.Mortgage = "later" }},
			{ID: "sign", Label: "sign today", apply: signNow},
			{ID: "decline", Label: "decline for good: buy the shop for cash or lose", apply: func(d *protocol.Directives) { d.Mortgage = "decline" }},
		}}, true
}

// signNow signs at the loan shark's next visit. The route's buyout_by_day would otherwise answer
// "later" for it while the cash buyout projects (always before day 84, sim/gym PlayerRole.Mortgage),
// so it is set to a day already past.
func signNow(d *protocol.Directives) {
	d.Mortgage, d.BuyoutByDay = "take", i64(1)
}

// styles are the trading styles that won hard 60/60 (docs/research/TRADING-STYLES.md).
var styles = []struct {
	id, label string
	apply     func(d *protocol.Directives)
}{
	{"apothecary", "a medicine shop: buy only medical lots (lab: wins 11.6 days later than the generalist)", func(d *protocol.Directives) {
		d.Specialty = &protocol.SpecialtyDirectives{Types: []string{"MEDICAL"}, LotShare: ptrF(0.3)}
	}},
	{"hardware_store", "a household-goods shop (lab: wins 13.8 days later)", func(d *protocol.Directives) {
		d.Specialty = &protocol.SpecialtyDirectives{Types: []string{"HOUSEHOLD_GOOD"}}
	}},
	{"speculator", "hoard medical, food, water and materials for shortages, up to 7 days (lab: wins 23 days later, twice the sales into events)", func(d *protocol.Directives) {
		d.Speculate = &protocol.SpeculateDirectives{Types: []string{"MEDICAL", "FOOD", "WATER", "MATERIAL"}, MaxDays: i64(7)}
	}},
	{"fair_dealer", "never haggle with anyone (lab: as fast as the generalist, sells a fifth more)", func(d *protocol.Directives) {
		d.Bargain, d.BargainBuy = "never", "never"
	}},
	{"clean_shop", "buy nothing stolen (lab: as fast as the generalist)", func(d *protocol.Directives) {
		if d.Buy == nil {
			d.Buy = &protocol.BuyDirectives{}
		}
		d.Buy.HotStockCap = i64(0)
	}},
}

func ptrF(v float64) *float64 { return &v }

func styleFork(s *snapshot, common []string, current string) fork {
	keep := "keep trading as now (the generalist: buy any lot under its estimate, sell to whoever asks)"
	if current != "" && current != "generalist" {
		keep = "go back to the generalist"
	}
	f := fork{Kind: "style", Title: "How should the shop trade from here?", Facts: common,
		Options: []forkOption{{ID: "generalist", Label: keep}}}
	if mix := s.stockMix(); mix != "" {
		f.Facts = append(f.Facts, mix)
	}
	for _, st := range styles {
		label := st.label
		if st.id == current {
			label = "stay " + st.id + ": " + label
		}
		f.Options = append(f.Options, forkOption{ID: st.id, Label: label, apply: st.apply})
	}
	return f
}

// stockMix is the shop's stock by type, the visible base for a style choice; "" when the
// snapshot's items carry no type (the gym's and the mod's do not yet, so the hold fork cannot arise).
func (s *snapshot) stockMix() string {
	count := map[string]int{}
	for _, it := range s.Inventory {
		if it.EstimatedValue == nil || *it.EstimatedValue <= 0 {
			continue
		}
		for _, t := range it.types() {
			count[t]++
		}
	}
	keys := sortedKeys(count)
	slices.SortStableFunc(keys, func(a, b string) int { return count[b] - count[a] })
	var parts []string
	for i, k := range keys {
		if i == 6 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", k, count[k]))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Stock by type (items): " + strings.Join(parts, ", ") + "."
}

// holdTypes are the types a market event lifts by +50 % (TRADING-STYLES.md); holding them
// through the event is the speculator's bet.
var holdTypes = []string{"medical", "food", "water", "material"}

func holdFork(s *snapshot, n int, common []string, hs *forkState) (fork, bool) {
	if hs.lastDay != 0 && n-hs.lastDay < 7 {
		return fork{}, false
	}
	for _, e := range s.Active {
		for i, t := range e.types {
			if i >= len(e.pcts) || e.pcts[i] < 30 || !slices.Contains(holdTypes, t) {
				continue
			}
			start := int64(n)
			if e.StartDay != nil {
				start = *e.StartDay
			}
			key := fmt.Sprintf("%s@%d", e.Name, start)
			if key == hs.key {
				continue
			}
			var uids []int64
			for _, it := range s.Inventory {
				if it.UID != nil && slices.Contains(it.types(), t) && it.Where != "" && !strings.HasPrefix(it.Where, "inside:") {
					uids = append(uids, *it.UID)
				}
			}
			if len(uids) < 3 {
				continue
			}
			end := n + 3
			if e.Duration != nil {
				end = int(start + *e.Duration - 1)
			}
			typ := strings.ToUpper(t)
			hs.key = key
			return fork{Kind: "hold", Title: fmt.Sprintf("%s: %s up %d %%", e.text(), t, e.pcts[i]), Until: end,
				Facts: append(common, fmt.Sprintf("The shop holds %d %s items; the event runs to day %d.", len(uids), t, end)),
				Options: []forkOption{
					{ID: "sell_into", Label: "sell into the rise as buyers come (the executor's rule)"},
					{ID: "hold", Label: fmt.Sprintf("hold the %d %s items for the event's last days", len(uids), t), apply: func(d *protocol.Directives) {
						if d.Sell == nil {
							d.Sell = &protocol.SellDirectives{}
						}
						d.Sell.HoldUIDs = append(slices.Clone(d.Sell.HoldUIDs), uids...)
					}},
					{ID: "stock_up", Label: fmt.Sprintf("buy more %s while it lasts", t), apply: func(d *protocol.Directives) {
						if d.Buy == nil {
							d.Buy = &protocol.BuyDirectives{}
						}
						if !slices.Contains(d.Buy.PreferTypes, typ) {
							d.Buy.PreferTypes = append(slices.Clone(d.Buy.PreferTypes), typ)
						}
					}},
				}}, true
		}
	}
	return fork{}, false
}

// forkFacts are the morning's visible numbers every fork shows.
func (s *snapshot) forkFacts(n int, fc forkContext) []string {
	var b []string
	line := fmt.Sprintf("Day %d", n)
	if s.Cash != nil {
		line += fmt.Sprintf(", cash %d", *s.Cash)
	}
	if s.NetWorth != nil {
		line += fmt.Sprintf(", net worth %d", *s.NetWorth)
	}
	if s.weekGain != nil {
		line += fmt.Sprintf(" (%+d over the last 7 days)", *s.weekGain)
	}
	if due, ok := s.Rent.due(); ok {
		line += fmt.Sprintf(", rent %d due", due)
		if s.Rent.DaysLeft != nil {
			line += fmt.Sprintf(" in %d days", *s.Rent.DaysLeft)
		}
	}
	b = append(b, line+".")
	if p := s.Property; p != nil && p.BuyoutPrice != nil {
		l := fmt.Sprintf("The landlord sells the shop for %d.", *p.BuyoutPrice)
		if s.NetWorth != nil && s.weekGain != nil && *s.weekGain > 0 && *p.BuyoutPrice > *s.NetWorth {
			l += fmt.Sprintf(" At last week's gain the net worth reaches it around day %d.", n+int((*p.BuyoutPrice-*s.NetWorth)*7 / *s.weekGain))
		}
		b = append(b, l)
	}
	return b
}

type forkAnswer struct {
	Choice string `json:"choice"`
	Why    string `json:"why"`
}

// askFork puts f to the model; ok is false when no usable answer came back.
func askFork(ctx context.Context, c llm.Client, system string, f fork) (forkAnswer, string, bool) {
	var b strings.Builder
	fmt.Fprintf(&b, "A DECISION FOR YOU (the executor will not decide it): %s\n\n", f.Title)
	for _, l := range f.Facts {
		b.WriteString(l + "\n")
	}
	b.WriteString("\nOptions:\n")
	ids := make([]string, len(f.Options))
	for i, o := range f.Options {
		ids[i] = o.ID
		fmt.Fprintf(&b, "- %s: %s\n", o.ID, o.Label)
	}
	b.WriteString("\nChoose one. Answer with one JSON object: {\"choice\": \"<option id>\", \"why\": \"<one or two sentences for the viewers, citing a number above>\"}.")
	enum, _ := json.Marshal(ids)
	schema := json.RawMessage(`{"type":"object","properties":{"choice":{"enum":` + string(enum) + `},"why":{"type":"string"}},"required":["choice","why"]}`)
	resp, err := c.Complete(ctx, llm.Request{System: system, Messages: []llm.Message{{Role: llm.RoleUser, Content: b.String()}}, Schema: schema, SchemaName: "fork_choice", MaxTokens: forkMaxTokens})
	if err != nil {
		return forkAnswer{}, err.Error(), false
	}
	raw, err := llm.ExtractJSON(resp.Text)
	if err != nil {
		return forkAnswer{}, resp.Text, false
	}
	var a forkAnswer
	if json.Unmarshal(raw, &a) != nil || !slices.Contains(ids, a.Choice) || strings.TrimSpace(a.Why) == "" {
		return forkAnswer{}, resp.Text, false
	}
	a.Why = clip(strings.TrimSpace(a.Why), 400)
	return a, resp.Text, true
}

func forkSystem(route, persona string) string {
	s := "You are the strategist of a pawn shop in Probably Stolen on hard, live on a stream. The executor (a tuned script) plays the routine; the big decisions are yours, and the viewers see each one with your reason."
	if route != "" {
		s += "\n\nYOUR ROUTE: " + route
	}
	if persona != "" {
		s += "\n\nYOUR STYLE:\n" + persona
	}
	return s
}
