package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

const maxScorecardBytes = 700

// dayStats is what the day's decisions, outcomes and events add up to.
type dayStats struct {
	customers, decisions int
	sales, purchases     int
	cashIn, cashOut      int64
	soldUIDs             []int64
	walkouts, dismissed  map[string]bool
	repDelta             map[string]float64
	followed, overridden int
}

func newDayStats() *dayStats {
	return &dayStats{walkouts: map[string]bool{}, dismissed: map[string]bool{}, repDelta: map[string]float64{}}
}

// result is a barter outcome's result (docs/PROTOCOL.md, "POST /v1/outcome").
type result struct {
	Kind      string             `json:"kind"`
	Side      string             `json:"side"`
	Items     []int64            `json:"items"`
	CashDelta *int64             `json:"cash_delta"`
	RepDelta  map[string]float64 `json:"rep_delta"`
	WalkedOut bool               `json:"walked_out"`
	Cause     string             `json:"cause"`
}

func (st *dayStats) addOutcome(decisionID string, raw json.RawMessage) {
	var r result
	if json.Unmarshal(raw, &r) != nil {
		return
	}
	for f, d := range r.RepDelta {
		st.repDelta[f] += d
	}
	switch r.Kind {
	case "trade":
		var cash int64
		if r.CashDelta != nil {
			cash = *r.CashDelta
		}
		switch r.Side {
		case "sold":
			st.sales++
			st.cashIn += cash
			st.soldUIDs = append(st.soldUIDs, r.Items...)
		case "bought":
			st.purchases++
			st.cashOut -= cash
		}
	case "bargain":
		if r.WalkedOut {
			st.walkouts[decisionID] = true
		}
	case "left":
		switch r.Cause {
		case "walked_out":
			st.walkouts[decisionID] = true
		case "dismissed":
			st.dismissed[decisionID] = true
		}
	}
}

// facts renders the day's numbers: morning and close snapshots where present, trades from
// outcomes otherwise.
func facts(morning, close *snapshot, st *dayStats) string {
	var parts []string
	switch {
	case morning != nil && close != nil && morning.Cash != nil && close.Cash != nil:
		m, c := *morning.Cash, *close.Cash
		parts = append(parts, fmt.Sprintf("cash %d -> %d (%s)", m, c, signed(float64(c-m))))
	case morning != nil && morning.Cash != nil:
		parts = append(parts, fmt.Sprintf("cash %d at open, %s from trades", *morning.Cash, signed(float64(st.cashIn-st.cashOut))))
	}
	if close != nil && close.Rent != nil && close.Rent.DaysLeft != nil {
		if due, ok := close.Rent.due(); ok {
			parts = append(parts, fmt.Sprintf("rent %d due in %d day(s)", due, *close.Rent.DaysLeft))
		}
	}
	if r := repChanges(morning, close, st); r != "" {
		parts = append(parts, r)
	}
	parts = append(parts, fmt.Sprintf("%d sale(s) %s, %d purchase(s) %s, %d walk-out(s), %d dismissed",
		st.sales, signed(float64(st.cashIn)), st.purchases, signed(float64(-st.cashOut)), len(st.walkouts), len(st.dismissed)))
	if sold := soldNames(morning, st); sold != "" {
		parts = append(parts, "sold "+sold)
	}
	if st.customers > 0 {
		parts = append(parts, fmt.Sprintf("%d customer(s)", st.customers))
	}
	if st.followed+st.overridden > 0 {
		parts = append(parts, fmt.Sprintf("advice followed %d of %d", st.followed, st.followed+st.overridden))
	}
	if morning != nil && close != nil && morning.Inventory != nil && close.Inventory != nil {
		parts = append(parts, fmt.Sprintf("stock %d -> %d items", len(morning.Inventory), len(close.Inventory)))
	}
	if r := riskyFate(morning, close, st); r != "" {
		parts = append(parts, r)
	}
	if morning != nil && close != nil && morning.Security != nil && close.Security != nil &&
		morning.Security.Trafficking != nil && close.Security.Trafficking != nil {
		parts = append(parts, fmt.Sprintf("trafficking %d -> %d", *morning.Security.Trafficking, *close.Security.Trafficking))
	}
	return strings.Join(parts, "; ") + "."
}

// soldNames names the items sold today; ones bought the same day are not in the morning
// inventory and show by uid.
func soldNames(morning *snapshot, st *dayStats) string {
	names := map[int64]string{}
	if morning != nil {
		for _, it := range morning.Inventory {
			if it.UID != nil {
				names[*it.UID] = itemName(it)
			}
		}
	}
	var out []string
	for _, uid := range st.soldUIDs {
		out = append(out, strings.TrimSpace(fmt.Sprintf("%s #%d", names[uid], uid)))
	}
	return strings.Join(out, ", ")
}

func repChanges(morning, close *snapshot, st *dayStats) string {
	var parts []string
	if morning != nil && close != nil && len(morning.Reputation) > 0 && len(close.Reputation) > 0 {
		for _, id := range factionOrder {
			m, ok1 := morning.rep(id)
			c, ok2 := close.rep(id)
			if ok1 && ok2 && fmtNum(m) != fmtNum(c) {
				parts = append(parts, fmt.Sprintf("%s %s -> %s", factionName(id), fmtNum(m), fmtNum(c)))
			}
		}
	} else {
		for _, id := range sortedKeys(st.repDelta) {
			if d := st.repDelta[id]; fmtNum(d) != "0" {
				p := fmt.Sprintf("%s %s", factionName(id), signed(d))
				if morning != nil {
					if m, ok := morning.rep(id); ok {
						p = fmt.Sprintf("%s %s -> %s", factionName(id), fmtNum(m), fmtNum(m+d))
					}
				}
				parts = append(parts, p)
			}
		}
	}
	if len(parts) == 0 {
		return "reputation unchanged"
	}
	return "reputation " + strings.Join(parts, ", ")
}

// riskyFate says what became of the morning's stolen and contraband items: sold, or still
// held (with the closing heat when known).
func riskyFate(morning, close *snapshot, st *dayStats) string {
	if morning == nil {
		return ""
	}
	held := map[int64]item{}
	if close != nil {
		for _, it := range close.Inventory {
			if it.UID != nil {
				held[*it.UID] = it
			}
		}
	}
	var parts []string
	for _, it := range morning.Inventory {
		if (!it.stolen() && !it.contraband()) || it.UID == nil {
			continue
		}
		name := fmt.Sprintf("%s #%d", itemName(it), *it.UID)
		switch c, ok := held[*it.UID]; {
		case slices.Contains(st.soldUIDs, *it.UID):
			parts = append(parts, name+" sold")
		case ok && c.StolenHeat != nil:
			parts = append(parts, fmt.Sprintf("%s held (heat %d, in %s)", name, *c.StolenHeat, nonEmpty(c.Where, "stock")))
		case ok:
			parts = append(parts, fmt.Sprintf("%s held (in %s)", name, nonEmpty(c.Where, "stock")))
		case close != nil:
			parts = append(parts, name+" gone")
		default:
			parts = append(parts, name+" not sold")
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "risky stock: " + strings.Join(parts, ", ")
}

var reviewSystem = strings.TrimSpace(`
You score one day of a pawnshop in the game "Probably Stolen" against the goals set that morning. You get the goals and the day's numbers.
Write one line per goal, in order: "- MET: <goal in a few words> (<the number that decides it>)", or MISSED or PARTLY with the number. When the numbers do not settle a goal (for example an item that is not named as sold), write UNKNOWN; never guess MISSED. Then one line "- NEXT: <the one change for tomorrow>".
Use only the numbers given. Plain text, at most 600 characters, nothing else.`)

// BuildReviewPrompt renders the evening review prompt for day's plan and facts.
// Only the GOALS go in, so the model scores those and not every line of the plan.
func BuildReviewPrompt(day int, plan, dayFacts string) string {
	goals := strings.Join(Goals(plan), "\n")
	if goals == "" {
		goals = plan
	}
	return fmt.Sprintf("GOALS FOR DAY %d:\n%s\n\nWHAT HAPPENED ON DAY %d: %s\n\nScore each goal.", day, goals, day, dayFacts)
}

// DraftScorecard asks c to score day's plan against its facts.
func DraftScorecard(ctx context.Context, c llm.Client, day int, plan, dayFacts string, maxTokens int) (string, error) {
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	resp, err := c.Complete(ctx, llm.Request{
		System:    reviewSystem,
		Messages:  []llm.Message{{Role: llm.RoleUser, Content: BuildReviewPrompt(day, plan, dayFacts)}},
		MaxTokens: maxTokens,
	})
	if err != nil {
		return "", err
	}
	return tidyScorecard(resp.Text)
}

func tidyScorecard(text string) (string, error) {
	var lines []string
	for _, l := range strings.Split(llm.StripThinking(text), "\n") {
		l = cleanLine(l)
		if l == "" || strings.HasPrefix(l, "```") {
			continue
		}
		if b := bullet(l); strings.Contains(b, ":") {
			lines = append(lines, b)
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("scorecard has no lines")
	}
	return truncateLines(strings.Join(lines, "\n"), maxScorecardBytes), nil
}
