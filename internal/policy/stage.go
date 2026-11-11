package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// The stage decision (docs/PROTOCOL.md): a buying customer waits on the shop, and the
// question is which one item to put on the back counter next. Advice only.
const (
	optNothingFits = "nothing_fits"
	optDismiss     = "dismiss"
	offerPrefix    = "offer_"
	// Heat falls this much per night (docs/re/STATE.md); at 0 an item sells clean.
	heatPerNight = 18
)

// stageCtx is the barter context without the bargain block, plus the candidates.
type stageCtx struct {
	barterCtx
	BudgetLeft *int64      `json:"budget_left"`
	Candidates []stageItem `json:"candidates"`
	Omitted    *int64      `json:"candidates_omitted"`
}

type stageItem struct {
	barterItem
	Where                string   `json:"where"`
	HiddenFromInspection *bool    `json:"hidden_from_inspection"`
	Matches              []string `json:"matches"`
	Attracted            *bool    `json:"attracted"`
	OverBudget           *bool    `json:"over_budget"`
}

func parseStage(raw json.RawMessage) (*stageCtx, error) {
	var c stageCtx
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func offerUID(id string) (int64, bool) {
	if !strings.HasPrefix(id, offerPrefix) {
		return 0, false
	}
	n, err := strconv.ParseInt(strings.TrimPrefix(id, offerPrefix), 10, 64)
	return n, err == nil
}

func offerID(uid int64) string { return offerPrefix + strconv.FormatInt(uid, 10) }

func (c *stageCtx) candidate(uid int64) (stageItem, bool) {
	for _, it := range c.Candidates {
		if it.UID != nil && *it.UID == uid {
			return it, true
		}
	}
	return stageItem{}, false
}

func (c *stageCtx) day() (int64, bool) {
	if c.Store == nil || c.Store.Day == nil {
		return 0, false
	}
	return *c.Store.Day, true
}

// stageUnsafe says why offering it to this customer must never be advised; "" when it may be.
func stageUnsafe(cl *barterClient, it stageItem) string {
	if cl == nil {
		return ""
	}
	contraband := it.contraband() || isTrue(it.SeriousContraband)
	if isTrue(cl.IsSecurity) && (it.stolen() || contraband) {
		return "stolen or contraband goods are never offered to security"
	}
	if isTrue(cl.NoContraband) && contraband {
		return "this customer refuses contraband"
	}
	return ""
}

// CheckChoice rejects a choice that the options allow but the visible state rules out:
// offering a candidate that is not listed, or stolen or contraband goods to a customer who
// must never get them; a confront or estimate the visible cues prove wrong. Every policy's
// answer passes through it.
func CheckChoice(req *protocol.DecideRequest, choice protocol.Choice) error {
	switch req.Kind {
	case "stage":
		return checkStageChoice(req, choice)
	case inspectKind:
		return checkInspectChoice(req, choice)
	case augCheckKind:
		return checkAugChoice(req, choice)
	}
	return nil
}

func checkStageChoice(req *protocol.DecideRequest, choice protocol.Choice) error {
	uid, ok := offerUID(choice.Option)
	if !ok {
		return nil
	}
	c, err := parseStage(req.Context)
	if err != nil {
		return errors.New("the stage context does not parse, so no offer can be checked")
	}
	it, ok := c.candidate(uid)
	if !ok {
		return fmt.Errorf("option %q names no listed candidate", choice.Option)
	}
	if why := stageUnsafe(c.Client, it); why != "" {
		return fmt.Errorf("option %q is not allowed: %s", choice.Option, why)
	}
	return nil
}

// received is what offering it alone brings in: its price, capped by the budget left.
func (c *stageCtx) received(it stageItem) (int64, bool) {
	if it.Price == nil {
		return 0, false
	}
	p := *it.Price
	if c.BudgetLeft != nil && p > *c.BudgetLeft {
		return *c.BudgetLeft, true
	}
	return p, true
}

func (c *stageCtx) cleanDay(it stageItem) (int64, bool) {
	d, ok := c.day()
	if !ok || it.StolenHeat == nil {
		return 0, false
	}
	return d + (*it.StolenHeat+heatPerNight-1)/heatPerNight, true
}

// stageNotes gives each offer its outcome: what it brings in against what was paid, and
// when a stolen one sells clean. The prompt also gets the claimed features after " = ".
func stageNotes(c *stageCtx) func(id, label string) string {
	return func(id, _ string) string {
		uid, ok := offerUID(id)
		if !ok {
			return ""
		}
		it, ok := c.candidate(uid)
		if !ok {
			return ""
		}
		recv, ok := c.received(it)
		if !ok {
			return ""
		}
		var parts []string
		head := fmt.Sprintf("receive %d", recv)
		if it.Price != nil && recv < *it.Price {
			head += " (capped by budget"
			if it.EstimatedValue != nil && recv < *it.EstimatedValue {
				head += fmt.Sprintf(", under est. %d", *it.EstimatedValue)
			}
			head += ")"
		}
		switch {
		case it.BoughtAt != nil:
			head += fmt.Sprintf(", margin %+d vs paid %d", recv-*it.BoughtAt, *it.BoughtAt)
			if recv < *it.BoughtAt {
				head += " (below cost)"
			}
		case it.EstimatedValue != nil:
			head += fmt.Sprintf(", %+d vs est. %d", recv-*it.EstimatedValue, *it.EstimatedValue)
		}
		parts = append(parts, head)
		if d, ok := c.cleanDay(it); ok {
			parts = append(parts, fmt.Sprintf("STOLEN: selling is a fencing crime; sells clean from day %d", d))
		}
		if it.contraband() {
			parts = append(parts, "contraband: selling is a trafficking crime")
		}
		note := strings.Join(parts, "; ")
		if len(it.Features) > 0 {
			var fs []string
			for _, f := range it.Features {
				fs = append(fs, f.Text)
			}
			note += " = claims " + strings.Join(fs, ", ")
		}
		return note
	}
}

// stageUseNotes is stageNotes with each candidate's use against what offering it brings in.
func stageUseNotes(c *stageCtx, use itemUse) func(id, label string) string {
	note := stageNotes(c)
	if use == nil {
		return note
	}
	return func(id, label string) string {
		n := note(id, label)
		uid, ok := offerUID(id)
		if !ok {
			return n
		}
		it, ok := c.candidate(uid)
		if !ok {
			return n
		}
		recv, _ := c.received(it)
		u := use(it.ID, it.Name, recv)
		if u == "" {
			return n
		}
		if n == "" {
			return "[" + u + "]"
		}
		return n + "; [" + u + "]"
	}
}

func renderStage(b *strings.Builder, c *stageCtx, use itemUse) {
	renderParties(b, &c.barterCtx)
	if c.Counter != nil {
		renderItems(b, "Already staged for them", c.Counter.YoursStaged, true, use)
	}
	if c.BudgetLeft != nil {
		fmt.Fprintf(b, "Budget left after the staged items: %d\n", *c.BudgetLeft)
	}
	n := len(c.Candidates)
	fmt.Fprintf(b, "Candidates from the shop: %d, as the offer_ options below (~price is the expected counter price)", n)
	if c.Omitted != nil && *c.Omitted > 0 {
		fmt.Fprintf(b, "; %d more that fit less well are not listed", *c.Omitted)
	}
	b.WriteString("\n")
	if n > 0 && c.allBelowCost() {
		b.WriteString("Every candidate would sell below what you paid.\n")
	}
	renderMarket(b, &c.barterCtx)
}

func (c *stageCtx) allBelowCost() bool {
	for _, it := range c.Candidates {
		recv, ok := c.received(it)
		if !ok || it.BoughtAt == nil || recv >= *it.BoughtAt {
			return false
		}
	}
	return true
}

// planIntent reads what the plan says about item uid: hold it (KEEP list, or a STOCK line
// that holds it) or sell it (a STOCK line, or KEEP's SAFE TO SELL line). A plan that does
// not name the item says neither.
func planIntent(plan string, uid int64) (hold, sell bool) {
	if plan == "" {
		return false, false
	}
	tag := regexp.MustCompile(`#` + strconv.FormatInt(uid, 10) + `\b`)
	section := ""
	for _, line := range strings.Split(plan, "\n") {
		if h, rest, ok := planHeading(line); ok {
			section, line = h, rest
		}
		if !tag.MatchString(line) {
			continue
		}
		l := strings.ToLower(line)
		switch section {
		case "KEEP":
			if strings.Contains(l, "safe to sell") {
				sell = true
			} else {
				hold = true
			}
		case "STOCK":
			if strings.Contains(l, "hold") || strings.Contains(l, "keep") || strings.Contains(l, "not sell") || strings.Contains(l, "don't sell") {
				hold = true
			} else if strings.Contains(l, "sell") {
				sell = true
			}
		}
	}
	return hold, sell
}

var planHeadings = map[string]bool{"CALENDAR": true, "GOALS": true, "CASH": true, "FACTIONS": true, "WINDOW": true, "STOCK": true, "KEEP": true, "PRODUCTION": true, "RISK": true, "CUSTOMERS": true}

// planHeading recognises the strategist's headings, with markdown marks or a colon, and
// returns the text after the colon.
func planHeading(line string) (heading, rest string, ok bool) {
	t := strings.Trim(strings.TrimSpace(line), "#* ")
	head, rest, colon := strings.Cut(t, ":")
	head = strings.Trim(strings.TrimSpace(head), "* ")
	if i := strings.IndexByte(head, '('); i > 0 {
		head = strings.TrimSpace(head[:i])
	}
	h := strings.ToUpper(head)
	return h, strings.Trim(rest, "* "), planHeadings[h] && (head == h || colon)
}

// stageHeuristic offers the in-budget candidate with the best margin over what was paid,
// never a held item, a stolen one the plan does not say to sell, or one below cost.
func stageHeuristic(req *protocol.DecideRequest, plan string) Decision {
	c, err := parseStage(req.Context)
	if err != nil {
		return heuristicDefer("stage context does not have the expected shape")
	}
	if c.Client == nil {
		return heuristicDefer("stage context has no client")
	}
	offered := map[string]bool{}
	for _, o := range req.Options {
		offered[o.ID] = true
	}
	type pick struct {
		it           stageItem
		recv, margin int64
		over         bool
	}
	var best *pick
	var skipped []string
	for _, it := range c.Candidates {
		if it.UID == nil || !offered[offerID(*it.UID)] || stageUnsafe(c.Client, it) != "" {
			continue
		}
		recv, ok := c.received(it)
		if !ok {
			continue
		}
		hold, sell := planIntent(plan, *it.UID)
		if hold {
			skipped = append(skipped, fmt.Sprintf("%s is on the plan's hold list", itemName(it)))
			continue
		}
		if it.stolen() && !sell {
			if d, ok := c.cleanDay(it); ok {
				skipped = append(skipped, fmt.Sprintf("stolen %s sells clean from day %d", itemName(it), d))
			} else {
				skipped = append(skipped, fmt.Sprintf("%s is stolen", itemName(it)))
			}
			continue
		}
		cost := it.EstimatedValue
		if it.BoughtAt != nil {
			cost = it.BoughtAt
		}
		over := it.Price != nil && recv < *it.Price
		if cost != nil && recv < *cost || over && it.EstimatedValue != nil && recv < *it.EstimatedValue {
			continue
		}
		p := pick{it: it, recv: recv, over: over}
		if cost != nil {
			p.margin = recv - *cost
		}
		if best == nil || best.over && !p.over || best.over == p.over && p.margin > best.margin {
			best = &p
		}
	}
	why := ""
	if len(skipped) > 0 {
		why = " Skipped: " + strings.Join(skipped, "; ") + "."
	}
	if best == nil {
		staged := c.Counter != nil && len(c.Counter.YoursStaged) > 0
		switch {
		case staged && offered[optNothingFits]:
			return choose(optNothingFits, "Nothing more is worth offering; sell what is staged."+why)
		case offered[optDismiss]:
			return choose(optDismiss, "Nothing in the shop fits this customer at a profit."+why)
		case offered[optNothingFits]:
			return choose(optNothingFits, "Nothing in the shop fits this customer at a profit."+why)
		}
		return heuristicDefer("nothing fits and no way to say so is offered")
	}
	it := best.it
	fits := "fits them"
	switch {
	case isTrue(it.Attracted):
		fits = "is what they came for"
	case len(it.Matches) > 0:
		fits = "matches " + strings.Join(it.Matches, ", ")
	}
	paid := ""
	if it.BoughtAt != nil {
		paid = fmt.Sprintf(", paid %d (%+d)", *it.BoughtAt, best.margin)
	}
	budget := ""
	if c.BudgetLeft != nil {
		if best.over {
			budget = fmt.Sprintf(" capped at budget %d", *c.BudgetLeft)
		} else {
			budget = fmt.Sprintf(" within budget %d", *c.BudgetLeft)
		}
	}
	return choose(offerID(*it.UID), fmt.Sprintf("%s %s: brings %d%s%s.%s", itemName(it), fits, best.recv, budget, paid, why))
}

func itemName(it stageItem) string {
	name := it.Name
	if name == "" {
		name = it.ID
	}
	if it.UID != nil {
		name += " #" + strconv.FormatInt(*it.UID, 10)
	}
	return name
}

// stageRecallView is req as memory understands it: a barter request whose staged items are
// the candidates, so the brief carries their past deals.
func stageRecallView(req *protocol.DecideRequest) *protocol.DecideRequest {
	var ctx map[string]json.RawMessage
	if json.Unmarshal(req.Context, &ctx) != nil {
		return req
	}
	cands := ctx["candidates"]
	if len(cands) == 0 {
		cands = json.RawMessage("[]")
	}
	counter, err := json.Marshal(map[string]json.RawMessage{"theirs": json.RawMessage("[]"), "yours_staged": cands})
	if err != nil {
		return req
	}
	ctx["counter"] = counter
	delete(ctx, "candidates")
	raw, err := json.Marshal(ctx)
	if err != nil {
		return req
	}
	v := *req
	v.Kind = "barter"
	v.Context = raw
	v.Options = nil
	return &v
}
