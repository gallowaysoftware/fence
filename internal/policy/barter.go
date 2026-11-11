package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// barterCtx is the v1 barter context (docs/PROTOCOL.md, "Barter context (v1)"). Every
// field is a pointer or a slice so a value the mod could not read stays distinguishable
// from zero.
type barterCtx struct {
	Store *struct {
		Cash         *int64 `json:"cash"`
		Day          *int64 `json:"day"`
		RetailMarkup *int64 `json:"retail_markup"`
		RentDue      *int64 `json:"rent_due"`
		RentDaysLeft *int64 `json:"rent_days_left"`
	} `json:"store"`
	Reputation map[string]repEntry `json:"reputation"`
	Client     *barterClient       `json:"client"`
	Counter    *struct {
		Theirs      []barterItem `json:"theirs"`
		YoursStaged []barterItem `json:"yours_staged"`
	} `json:"counter"`
	Bargain *struct {
		SellSideUsed  *bool `json:"sell_side_used"`
		BuySideUsed   *bool `json:"buy_side_used"`
		BlackmailUsed *bool `json:"blackmail_used"`
		ThreatenUsed  *bool `json:"threaten_used"`
	} `json:"bargain"`
	MarketEvents []struct {
		ItemType string `json:"item_type"`
		ValuePct int64  `json:"value_pct"`
	} `json:"market_events"`
}

type repEntry struct {
	Value  *float64 `json:"value"`
	Status string   `json:"status"`
}

type barterClient struct {
	Name            string   `json:"name"`
	Faction         string   `json:"faction"`
	Intent          string   `json:"intent"`
	Budget          *int64   `json:"budget"`
	Interests       []string `json:"interests"`
	Restrictions    []string `json:"restrictions"`
	NoContraband    *bool    `json:"no_contraband"`
	RequiresVariety *bool    `json:"requires_variety"`
	IsSecurity      *bool    `json:"is_security"`
	// Aug is the customer's aug status, stated by the mod when detection is assumed
	// perfect (docs/PROTOCOL.md); absent in tells mode, where AugTells lists what shows.
	Aug            *bool    `json:"aug"`
	AugTells       []string `json:"aug_tells"`
	OnWantedPoster *bool    `json:"on_wanted_poster"`
	AttractedBy    string   `json:"attracted_by"`
	Wholesale      *struct {
		Price       *int64 `json:"price"`
		DiscountPct *int64 `json:"discount_pct"`
	} `json:"wholesale"`
}

type barterItem struct {
	UID            *int64 `json:"uid"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	Price          *int64 `json:"price"`
	EstimatedValue *int64 `json:"estimated_value"`
	BoughtAt       *int64 `json:"bought_at"`
	Features       []struct {
		Text  string `json:"text"`
		Group string `json:"group"`
	} `json:"features"`
	StolenHeat        *int64 `json:"stolen_heat"`
	ContrabandLevel   *int64 `json:"contraband_level"`
	SeriousContraband *bool  `json:"serious_contraband"`
}

func (it barterItem) stolen() bool     { return it.StolenHeat != nil }
func (it barterItem) contraband() bool { return it.ContrabandLevel != nil && *it.ContrabandLevel > 0 }

func parseBarter(raw json.RawMessage) (*barterCtx, error) {
	var c barterCtx
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *barterCtx) cash() *int64 {
	if c.Store == nil {
		return nil
	}
	return c.Store.Cash
}

func (c *barterCtx) clientRep() (float64, bool) {
	if c.Client == nil {
		return 0, false
	}
	r, ok := c.Reputation[c.Client.Faction]
	if !ok || r.Value == nil {
		return 0, false
	}
	return *r.Value, true
}

// factionNames are the display names the prompt uses; the ids stay in the context.
var factionNames = map[string]string{
	"lower_levels": "Lower Levels",
	"upper_levels": "Upper Levels",
	"security":     "Security",
	"black_market": "Black Market",
	"revolution":   "Revolution",
	"cartel":       "Cartel",
	"tourist":      "Tourist",
	"church":       "Church",
	"middle":       "Middle",
}

func factionName(id string) string {
	if n, ok := factionNames[id]; ok {
		return n
	}
	return id
}

var intentText = map[string]string{
	"buy":          "wants to BUY from you (you sell the items on your back counter)",
	"sell":         "wants to SELL to you (you buy the items on their counter)",
	"sell_and_buy": "wants to both SELL to you and BUY from you",
}

// renderBarter writes the context as compact text. Absent fields are left out, never
// guessed; the model is told so in the system prompt.
func renderBarter(b *strings.Builder, c *barterCtx, use itemUse) {
	renderParties(b, c)
	if c.Counter != nil {
		renderItems(b, "Their items (you may buy)", c.Counter.Theirs, false, use)
		renderItems(b, "Your staged items (you may sell)", c.Counter.YoursStaged, true, use)
		if c.Client != nil && c.Client.Intent == "buy" && len(c.Counter.YoursStaged) == 0 {
			b.WriteString("Nothing is staged yet, so nothing can be sold until the player stages items: defer.\n")
		}
	}
	if bg := c.Bargain; bg != nil {
		var used []string
		for _, f := range []struct {
			v    *bool
			name string
		}{{bg.SellSideUsed, "sell-side bargain"}, {bg.BuySideUsed, "buy-side bargain"}, {bg.BlackmailUsed, "blackmail"}, {bg.ThreatenUsed, "threaten"}} {
			if isTrue(f.v) {
				used = append(used, f.name)
			}
		}
		if len(used) > 0 {
			fmt.Fprintf(b, "Already used with this customer: %s\n", strings.Join(used, ", "))
		}
	}
	renderMarket(b, c)
}

// renderParties writes the shop, the reputations and the customer.
func renderParties(b *strings.Builder, c *barterCtx) {
	if s := c.Store; s != nil {
		var parts []string
		if s.Cash != nil {
			parts = append(parts, fmt.Sprintf("cash %d", *s.Cash))
		}
		if s.RentDue != nil {
			r := fmt.Sprintf("rent %d due", *s.RentDue)
			if s.RentDaysLeft != nil {
				r += fmt.Sprintf(" in %d days", *s.RentDaysLeft)
			}
			parts = append(parts, r)
		}
		if s.RetailMarkup != nil {
			parts = append(parts, fmt.Sprintf("retail markup %d%%", *s.RetailMarkup))
		}
		if len(parts) > 0 {
			fmt.Fprintf(b, "Shop: %s\n", strings.Join(parts, ", "))
		}
	}
	if len(c.Reputation) > 0 {
		var parts []string
		for _, id := range sortedKeys(c.Reputation) {
			r := c.Reputation[id]
			if r.Value == nil {
				continue
			}
			p := fmt.Sprintf("%s %s", factionName(id), fmtRep(*r.Value))
			if r.Status != "" {
				p += " (" + r.Status + ")"
			}
			parts = append(parts, p)
		}
		if len(parts) > 0 {
			fmt.Fprintf(b, "Reputation: %s\n", strings.Join(parts, ", "))
		}
	}
	if cl := c.Client; cl != nil {
		b.WriteString("Customer:")
		if cl.Name != "" {
			b.WriteString(" " + cl.Name)
		}
		if cl.Faction != "" {
			fmt.Fprintf(b, ", faction %s", factionName(cl.Faction))
			if !isRepFaction(cl.Faction) {
				b.WriteString(" (no reputation bar)")
			}
		}
		b.WriteString("\n")
		if t, ok := intentText[cl.Intent]; ok {
			fmt.Fprintf(b, "- %s\n", t)
		} else if cl.Intent != "" {
			fmt.Fprintf(b, "- intent %q\n", cl.Intent)
		}
		if cl.Budget != nil {
			fmt.Fprintf(b, "- budget left %d\n", *cl.Budget)
		}
		if len(cl.Interests) > 0 {
			fmt.Fprintf(b, "- interests: %s\n", strings.Join(cl.Interests, ", "))
		}
		if len(cl.Restrictions) > 0 {
			fmt.Fprintf(b, "- restrictions: %s\n", strings.Join(cl.Restrictions, ", "))
		}
		var tags []string
		if isTrue(cl.IsSecurity) {
			tags = append(tags, "SECURITY officer")
		}
		if isTrue(cl.NoContraband) {
			tags = append(tags, "refuses contraband")
		}
		if isTrue(cl.RequiresVariety) {
			tags = append(tags, "requires variety")
		}
		if isTrue(cl.OnWantedPoster) {
			tags = append(tags, "on a WANTED poster")
		}
		if isTrue(cl.Aug) {
			tags = append(tags, "an AUG (augmented, confirmed)")
		}
		for _, t := range cl.AugTells {
			tags = append(tags, "aug tell: "+t)
		}
		if len(tags) > 0 {
			fmt.Fprintf(b, "- %s\n", strings.Join(tags, "; "))
		}
		if cl.AttractedBy != "" {
			fmt.Fprintf(b, "- attracted by %s\n", cl.AttractedBy)
		}
		if w := cl.Wholesale; w != nil && w.Price != nil {
			fmt.Fprintf(b, "- offers WHOLESALE: everything on their counters for %d", *w.Price)
			if w.DiscountPct != nil {
				fmt.Fprintf(b, " (%d%% off)", *w.DiscountPct)
			}
			b.WriteString("\n")
		}
	}
}

func renderMarket(b *strings.Builder, c *barterCtx) {
	if len(c.MarketEvents) > 0 {
		var parts []string
		for _, e := range c.MarketEvents {
			parts = append(parts, fmt.Sprintf("%s %+d%%", e.ItemType, e.ValuePct))
		}
		fmt.Fprintf(b, "Market events: %s\n", strings.Join(parts, ", "))
	}
}

// itemUse is the strategist's use-versus-sale note for an item ("" when it knows no use);
// sale is what the item would sell for.
type itemUse func(id, name string, sale int64) string

// useNote is use's note for it in brackets: against the counter price for the shop's own
// items, against the estimate for a seller's.
func useNote(use itemUse, it barterItem, own bool) string {
	if use == nil {
		return ""
	}
	sale := it.EstimatedValue
	if own {
		sale = it.Price
	}
	var v int64
	if sale != nil {
		v = *sale
	}
	if n := use(it.ID, it.Name, v); n != "" {
		return " [" + n + "]"
	}
	return ""
}

func renderItems(b *strings.Builder, title string, items []barterItem, own bool, use itemUse) {
	if len(items) == 0 {
		fmt.Fprintf(b, "%s: none\n", title)
		return
	}
	var price, est int64
	priced := true
	for _, it := range items {
		if it.Price == nil || it.EstimatedValue == nil {
			priced = false
			continue
		}
		price += *it.Price
		est += *it.EstimatedValue
	}
	if priced && len(items) > 1 {
		fmt.Fprintf(b, "%s: %d items, total price %d, total estimate %d\n", title, len(items), price, est)
	} else {
		fmt.Fprintf(b, "%s:\n", title)
	}
	for _, it := range items {
		name := it.Name
		if name == "" {
			name = it.ID
		}
		b.WriteString("- " + name)
		if it.Price != nil {
			fmt.Fprintf(b, ": price %d", *it.Price)
		}
		if it.EstimatedValue != nil {
			fmt.Fprintf(b, ", est. value %d", *it.EstimatedValue)
		}
		if own && it.BoughtAt != nil {
			fmt.Fprintf(b, ", you paid %d", *it.BoughtAt)
		}
		if len(it.Features) > 0 {
			var fs []string
			for _, f := range it.Features {
				if f.Group != "" {
					fs = append(fs, f.Text+" ("+f.Group+")")
				} else {
					fs = append(fs, f.Text)
				}
			}
			fmt.Fprintf(b, "; claims: %s", strings.Join(fs, ", "))
		}
		if it.StolenHeat != nil {
			fmt.Fprintf(b, "; STOLEN heat %d", *it.StolenHeat)
		}
		if it.contraband() {
			fmt.Fprintf(b, "; CONTRABAND level %d", *it.ContrabandLevel)
			if isTrue(it.SeriousContraband) {
				b.WriteString(" (serious)")
			}
		}
		b.WriteString(useNote(use, it, own) + "\n")
	}
}

func isRepFaction(id string) bool {
	return slices.Contains([]string{"lower_levels", "upper_levels", "security", "black_market", "revolution", "cartel"}, id)
}

func isTrue(b *bool) bool { return b != nil && *b }

func fmtRep(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// bargainOdds is what an option label states about a rolled bargain. The mod writes the
// game's own tooltip odds into the label; both "else stay S%" (cumulative, one roll) and
// an explicit "walk out W%" are understood.
type bargainOdds struct {
	accept, walk int64
}

var (
	reAccept = regexp.MustCompile(`accept(?:s|ed)?\s+(\d+)%`)
	reStay   = regexp.MustCompile(`stay\s+(\d+)%`)
	reWalk   = regexp.MustCompile(`walk(?:s)?\s+out\s+(\d+)%`)
)

func parseOdds(label string) (bargainOdds, bool) {
	l := strings.ToLower(label)
	m := reAccept.FindStringSubmatch(l)
	if m == nil {
		return bargainOdds{}, false
	}
	o := bargainOdds{}
	o.accept, _ = strconv.ParseInt(m[1], 10, 64)
	switch {
	case reWalk.MatchString(l):
		o.walk, _ = strconv.ParseInt(reWalk.FindStringSubmatch(l)[1], 10, 64)
	case reStay.MatchString(l):
		s, _ := strconv.ParseInt(reStay.FindStringSubmatch(l)[1], 10, 64)
		o.walk = max(0, 100-max(s, o.accept))
	case strings.Contains(l, "they stay"):
		o.walk = 0
	default:
		return bargainOdds{}, false
	}
	if o.accept > 100 || o.walk > 100-o.accept {
		return bargainOdds{}, false
	}
	return o, true
}

// Outcome arithmetic shared by the heuristic and the prompt's option notes. Selling
// values are credits received; buying values are margin against the estimate.

type sellSide struct{ price, budget, cost int64 }
type buySide struct{ price, value, spendable int64 }

func (c *barterCtx) sellSide() (sellSide, bool) {
	if c.Client == nil || c.Client.Budget == nil || c.Counter == nil || len(c.Counter.YoursStaged) == 0 {
		return sellSide{}, false
	}
	s := sellSide{budget: *c.Client.Budget, cost: -1}
	cost := int64(0)
	for _, it := range c.Counter.YoursStaged {
		if it.Price == nil {
			return sellSide{}, false
		}
		s.price += *it.Price
		if it.BoughtAt == nil {
			cost = -1
		} else if cost >= 0 {
			cost += *it.BoughtAt
		}
	}
	s.cost = cost
	return s, true
}

func (c *barterCtx) buySide() (buySide, bool) {
	cash := c.cash()
	if c.Counter == nil || len(c.Counter.Theirs) == 0 || cash == nil {
		return buySide{}, false
	}
	b := buySide{spendable: *cash - c.rentReserve()}
	for _, it := range c.Counter.Theirs {
		if it.Price == nil || it.EstimatedValue == nil {
			return buySide{}, false
		}
		b.price += *it.Price
		b.value += *it.EstimatedValue
	}
	return b, true
}

func (s sellSide) paid() string {
	if s.cost < 0 {
		return ""
	}
	return fmt.Sprintf("; you paid %d", s.cost)
}

func (s sellSide) plain() int64 { return min(s.price, s.budget) }

func (s sellSide) discount(n int64) int64 {
	return min(s.price*(100-n)/100, s.budget*(100+2*n)/100)
}

func (s sellSide) markupWon(n int64) int64 {
	return min(s.price*(100+n)/100, s.budget*(100-markupBudgetCut[n])/100)
}

func (s sellSide) markupEV(n int64, o bargainOdds) int64 {
	return (o.accept*s.markupWon(n) + (100-o.accept-o.walk)*s.plain()) / 100
}

// margin is what buying at p leaves against the estimate; 0 when p is not affordable.
func (b buySide) margin(p int64) int64 {
	if p > b.spendable {
		return 0
	}
	return max(0, b.value-p)
}

func (b buySide) discountEV(n int64, o bargainOdds) int64 {
	return (o.accept*b.margin(b.price*(100-n)/100) + (100-o.accept-o.walk)*b.margin(b.price)) / 100
}

// stepOf returns N from an option id like "add_markup_25".
func stepOf(id, prefix string) (int64, bool) {
	if !strings.HasPrefix(id, prefix) {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(id, prefix), 10, 64)
	if err != nil || markupBudgetCut[n] == 0 {
		return 0, false
	}
	return n, true
}

// optionNotes spells out what each option yields, so the model weighs outcomes instead
// of doing arithmetic. Only visible values and the labels' own odds go in.
func optionNotes(c *barterCtx) func(id, label string) string {
	sell, canSell := c.sellSide()
	buy, canBuy := c.buySide()
	return func(id, label string) string {
		if canSell {
			switch {
			case id == "sell_all":
				note := fmt.Sprintf("you receive %d", sell.plain())
				if sell.price > sell.budget {
					note += " (capped by budget)"
				}
				return note + sell.paid()
			}
			if n, ok := stepOf(id, "offer_discount_"); ok {
				return fmt.Sprintf("you would receive %d", sell.discount(n)) + sell.paid()
			}
			if n, ok := stepOf(id, "add_markup_"); ok {
				if o, ok := parseOdds(label); ok {
					return fmt.Sprintf("expected %d (vs %d plain) = %s", sell.markupEV(n, o), sell.plain(),
						rollOutcomes(o, fmt.Sprintf("receive %d", sell.markupWon(n)), fmt.Sprintf("receive %d", sell.plain())))
				}
			}
		}
		if canBuy {
			afford := func(p int64) string {
				if p > buy.spendable {
					return fmt.Sprintf("; more than the %d you can spend", buy.spendable)
				}
				return ""
			}
			switch id {
			case "buy_all":
				return fmt.Sprintf("costs %d for an estimate of %d (margin %+d)%s", buy.price, buy.value, buy.value-buy.price, afford(buy.price))
			case "accept_wholesale":
				if w := c.Client.Wholesale; w != nil && w.Price != nil {
					return fmt.Sprintf("costs %d for an estimate of %d (margin %+d)%s", *w.Price, buy.value, buy.value-*w.Price, afford(*w.Price))
				}
			}
			if n, ok := stepOf(id, "demand_discount_"); ok {
				if o, ok := parseOdds(label); ok {
					p := buy.price * (100 - n) / 100
					return fmt.Sprintf("expected margin %d (vs %d buying now) = %s", buy.discountEV(n, o), buy.margin(buy.price),
						rollOutcomes(o, fmt.Sprintf("pay %d, margin %+d", p, buy.value-p), fmt.Sprintf("price stays %d, margin %+d", buy.price, buy.value-buy.price)))
				}
			}
			if n, ok := stepOf(id, "offer_markup_"); ok {
				p := buy.price * (100 + n) / 100
				return fmt.Sprintf("costs %d (margin %+d)%s", p, buy.value-p, afford(p))
			}
		}
		return ""
	}
}

func rollOutcomes(o bargainOdds, accepted, refused string) string {
	parts := []string{fmt.Sprintf("%d%% accepted (%s)", o.accept, accepted)}
	if r := 100 - o.accept - o.walk; r > 0 {
		parts = append(parts, fmt.Sprintf("%d%% refused (%s)", r, refused))
	}
	if o.walk > 0 {
		parts = append(parts, fmt.Sprintf("%d%% walk out (no deal, -3 rep)", o.walk))
	} else {
		parts = append(parts, "no walk-out risk")
	}
	return strings.Join(parts, ", ")
}
