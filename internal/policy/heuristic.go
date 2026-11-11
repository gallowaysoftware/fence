package policy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Heuristic is the zero-inference baseline. It is deterministic, reads only the visible
// context, and defers whenever a field it relies on is missing or odd. It picks the move
// with the best expected cash outcome, using the odds the option labels state.
type Heuristic struct {
	// Plan, when set, keeps the stage decision off items the day's plan holds back.
	Plan Planner
}

func (Heuristic) Name() string { return "heuristic" }

func (h Heuristic) Decide(_ context.Context, req *protocol.DecideRequest) Decision {
	req = req.PolicyView()
	var d Decision
	switch req.Kind {
	case "barter":
		d = barter(req)
	case "stage":
		plan, _ := recentPlan(h.Plan, req)
		d = stageHeuristic(req, plan)
	case inspectKind:
		d = inspectHeuristic(req)
	case augCheckKind:
		d = augHeuristic(req)
	default:
		return heuristicDefer("unknown decision kind " + strconv.Quote(req.Kind))
	}
	if err := CheckChoice(req, d.Choice); err != nil {
		d = heuristicDefer(err.Error())
	}
	d.Notes = OptionNotes(req)
	return d
}

func heuristicDefer(why string) Decision {
	return Decision{Choice: protocol.Defer(), Rationale: why, Source: protocol.SourceHeuristic, Trace: Trace{Note: why}}
}

func choose(id, why string) Decision {
	return Decision{Choice: protocol.Choice{Option: id}, Rationale: why, Source: protocol.SourceHeuristic}
}

const (
	// Stolen goods at or above this heat are not worth buying.
	maxHeat = 50
	// At or below this reputation with the customer's faction, no move that can make
	// them walk out (−3 rep) is taken: −100 ends the run.
	dangerRep = -60
	// A rolled bargain must beat the plain deal by this many percent in expectation to
	// pay for the walk-out rep loss and the trade-rep penalty of a hard bargain.
	bargainEdgePct = 5
	// Rolled bargains less likely than this to be accepted are not worth a walk-out.
	minAcceptPct = 50
	// Purchases keep the rent in hand when it is due within this many days.
	rentDays = 2
)

var bargainSteps = []int64{10, 25, 50, 75}

// markupBudgetCut is the budget cut on a successful Add Markup, per step (docs/re/BARTER.md).
var markupBudgetCut = map[int64]int64{10: 10, 25: 15, 50: 20, 75: 25}

type candidate struct {
	id string
	ev int64 // expected credits: revenue when selling, margin when buying
}

func barter(req *protocol.DecideRequest) Decision {
	c, err := parseBarter(req.Context)
	if err != nil {
		return heuristicDefer("barter context does not have the expected shape")
	}
	if c.Client == nil {
		return heuristicDefer("barter context has no client")
	}
	if c.Counter == nil {
		return heuristicDefer("barter context has no counter")
	}
	opts := map[string]string{}
	for _, o := range req.Options {
		if len(o.Params) == 0 {
			opts[o.ID] = o.Label
		}
	}
	switch c.Client.Intent {
	case "buy":
		return shopSells(c, opts)
	case "sell":
		return shopBuys(c, opts)
	case "sell_and_buy":
		theirs, yours := len(c.Counter.Theirs) > 0, len(c.Counter.YoursStaged) > 0
		switch {
		case theirs && yours:
			return heuristicDefer("both counters hold items; weighing a two-way deal is left to you")
		case yours:
			return shopSells(c, opts)
		case theirs:
			return shopBuys(c, opts)
		}
		return heuristicDefer("nothing on either counter yet")
	}
	return heuristicDefer("unrecognised client intent " + strconv.Quote(c.Client.Intent))
}

func inDanger(c *barterCtx) bool {
	rep, ok := c.clientRep()
	return ok && rep <= dangerRep
}

func shopSells(c *barterCtx, opts map[string]string) Decision {
	items := c.Counter.YoursStaged
	if len(items) == 0 {
		return heuristicDefer("stage the items you want to sell first")
	}
	for _, it := range items {
		if it.Price == nil {
			return heuristicDefer("a staged item has no price")
		}
		if (it.stolen() || it.contraband()) && isTrue(c.Client.IsSecurity) {
			return heuristicDefer("do not sell stolen or contraband goods to security")
		}
		if it.contraband() && isTrue(c.Client.NoContraband) {
			return heuristicDefer("this customer refuses contraband; unstage it")
		}
	}
	s, ok := c.sellSide()
	if !ok {
		return heuristicDefer("client budget is not shown")
	}
	danger := inDanger(c)

	var cands []candidate
	if _, ok := opts["sell_all"]; ok {
		cands = append(cands, candidate{"sell_all", s.plain()})
	}
	for _, n := range bargainSteps {
		id := "offer_discount_" + strconv.FormatInt(n, 10)
		if _, ok := opts[id]; ok {
			cands = append(cands, candidate{id, s.discount(n)})
		}
		id = "add_markup_" + strconv.FormatInt(n, 10)
		if label, ok := opts[id]; ok {
			o, ok := parseOdds(label)
			if !ok || o.accept < minAcceptPct || (danger && o.walk > 0) {
				continue
			}
			cands = append(cands, candidate{id, s.markupEV(n, o) * 100 / (100 + bargainEdgePct)})
		}
	}
	best, ok := pick(cands)
	if !ok {
		return heuristicDefer("no sell option is offered")
	}
	if s.cost >= 0 && best.ev < s.cost {
		return heuristicDefer(fmt.Sprintf("the best deal (%d) is under what you paid (%d)", best.ev, s.cost))
	}
	switch {
	case best.id == "sell_all" && s.price > s.budget:
		return choose(best.id, fmt.Sprintf("Budget %d caps the %d sale, but no discount does better.", s.budget, s.price))
	case best.id == "sell_all":
		return choose(best.id, fmt.Sprintf("%d is within budget and no bargain beats it on expected value.", s.price))
	case strings.HasPrefix(best.id, "offer_discount_"):
		return choose(best.id, fmt.Sprintf("Price %d is over budget %d; the discount raises their budget and nets more.", s.price, s.budget))
	}
	return choose(best.id, fmt.Sprintf("The markup's odds beat selling at %d outright.", s.price))
}

func shopBuys(c *barterCtx, opts map[string]string) Decision {
	items := c.Counter.Theirs
	if len(items) == 0 {
		return heuristicDefer("the customer has nothing on the counter yet")
	}
	if c.cash() == nil {
		return heuristicDefer("cash is not shown")
	}
	_, canDismiss := opts["dismiss"]
	for _, it := range items {
		if it.Price == nil || it.EstimatedValue == nil {
			return heuristicDefer("a counter item has no price or estimated value")
		}
		if it.StolenHeat != nil && *it.StolenHeat >= maxHeat {
			if canDismiss {
				return choose("dismiss", fmt.Sprintf("Stolen goods at heat %d are too hot to buy.", *it.StolenHeat))
			}
			return heuristicDefer("a stolen item is too hot and dismiss is not offered")
		}
		if isTrue(it.SeriousContraband) {
			return heuristicDefer("serious contraband is a judgment call")
		}
	}
	b, _ := c.buySide()
	danger := inDanger(c)

	var cands []candidate
	if _, ok := opts["buy_all"]; ok {
		cands = append(cands, candidate{"buy_all", b.margin(b.price)})
	}
	if w := c.Client.Wholesale; w != nil && w.Price != nil {
		if _, ok := opts["accept_wholesale"]; ok {
			// Wholesale also takes what is still on the front counter; only the listed
			// items are valued, so this understates it when the counter overflows.
			cands = append(cands, candidate{"accept_wholesale", b.margin(*w.Price)})
		}
	}
	for _, n := range bargainSteps {
		id := "demand_discount_" + strconv.FormatInt(n, 10)
		label, ok := opts[id]
		if !ok {
			continue
		}
		o, ok := parseOdds(label)
		if !ok || o.accept < minAcceptPct || (danger && o.walk > 0) {
			continue
		}
		cands = append(cands, candidate{id, b.discountEV(n, o) * 100 / (100 + bargainEdgePct)})
	}
	best, ok := pick(cands)
	if !ok || best.ev <= 0 {
		why := fmt.Sprintf("Asking %d against an estimate of %d", b.price, b.value)
		if b.price > b.spendable {
			why += fmt.Sprintf(" with %d cash to spend", b.spendable)
		}
		if canDismiss {
			return choose("dismiss", why+" leaves no margin.")
		}
		return heuristicDefer(why + "; dismiss is not offered")
	}
	switch best.id {
	case "buy_all":
		return choose(best.id, fmt.Sprintf("Asking %d against an estimate of %d is a good margin.", b.price, b.value))
	case "accept_wholesale":
		return choose(best.id, fmt.Sprintf("Wholesale at %d against an estimate of %d is the best margin.", *c.Client.Wholesale.Price, b.value))
	}
	return choose(best.id, fmt.Sprintf("Asking %d leaves a thin margin on %d; the discount's odds are worth it.", b.price, b.value))
}

// rentReserve is cash a purchase must not touch: rent falling due within rentDays.
func (c *barterCtx) rentReserve() int64 {
	s := c.Store
	if s == nil || s.RentDue == nil || s.RentDaysLeft == nil || *s.RentDaysLeft > rentDays {
		return 0
	}
	return *s.RentDue
}

// pick returns the highest expected value; ties keep the earlier, plainer option.
func pick(cands []candidate) (candidate, bool) {
	if len(cands) == 0 {
		return candidate{}, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.ev > best.ev {
			best = c
		}
	}
	return best, true
}
