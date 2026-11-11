package show

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Names resolves ids to display names; either func may be nil, and "" means unknown.
type Names struct {
	Item    func(uid int64) string
	Upgrade func(id string) string
}

// Describe renders directives as short instructions, one per field set.
func Describe(d *protocol.Directives, n Names) []string {
	name := n.Item
	if d == nil {
		return nil
	}
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	if d.CashFloor != nil {
		add("keep at least %d cash", *d.CashFloor)
	}
	if b := d.Buy; b != nil {
		if b.MaxSpend != nil {
			add("spend at most %d on stock", *b.MaxSpend)
		}
		if b.WholesaleMaxPct != nil {
			add("wholesale only under %d%% of estimate", *b.WholesaleMaxPct)
		}
		if b.SingleMaxPct != nil {
			add("single buys under %d%% of estimate", *b.SingleMaxPct)
		}
		// A type both preferred and avoided is neither (docs/PROTOCOL.md, "Directives").
		prefer, avoid := without(b.PreferTypes, b.AvoidTypes), without(b.AvoidTypes, b.PreferTypes)
		if len(prefer) > 0 {
			add("stock up on %s", words(prefer))
		}
		if len(avoid) > 0 {
			add("don't buy %s", words(avoid))
		}
		if b.HotStockCap != nil {
			add("hold at most %d stolen items", *b.HotStockCap)
		}
	}
	if s := d.Sell; s != nil {
		if s.SellDownToCash != nil {
			add("sell down to %d cash", *s.SellDownToCash)
		}
		if s.MinPct != nil {
			add("sell at %d%% of estimate or better", *s.MinPct)
		}
		if len(s.HoldUIDs) > 0 {
			add("hold %s", items(s.HoldUIDs, name))
		}
	}
	switch d.Bargain {
	case "never":
		add("no haggling")
	case "aggressive":
		add("haggle hard")
	}
	if w := d.Window; w != nil {
		if len(w.ShowUIDs) > 0 {
			add("window: %s", items(w.ShowUIDs, name))
		}
		if w.AvoidStolen != nil && *w.AvoidStolen {
			add("nothing stolen in the window")
		}
	}
	if len(d.UpgradesBuy) > 0 {
		ws := make([]string, len(d.UpgradesBuy))
		for i, id := range d.UpgradesBuy {
			if n.Upgrade != nil {
				ws[i] = n.Upgrade(id)
			}
			if ws[i] == "" {
				ws[i] = words([]string{id})
			}
		}
		add("buy the %s", joinAnd(ws))
	}
	switch d.Permit {
	case "buy":
		add("buy the vendor permit")
	case "bribe":
		add("bribe instead of a permit")
	case "refuse":
		add("refuse the permit")
	}
	switch d.Augs {
	case "shoot":
		add("shoot augs on sight")
	case "serve":
		add("serve augs")
	}
	if d.Scavenge != nil {
		if *d.Scavenge {
			add("scavenge tonight")
		} else {
			add("stay in tonight")
		}
	}
	if d.Engine != "" {
		add("run %s", strings.ReplaceAll(d.Engine, "+", " + "))
	}
	switch d.Mortgage {
	case "take":
		add("take the mortgage")
	case "decline":
		add("refuse the mortgage")
	case "auto":
		add("mortgage when it pays")
	case "accept_if_rent":
		add("mortgage only to cover rent")
	case "later":
		add("put the mortgage off a week")
	}
	if len(d.MortgageWindow) == 2 {
		add("sign the mortgage only on days %d to %d", d.MortgageWindow[0], d.MortgageWindow[1])
	}
	if d.BuyoutByDay != nil {
		add("buy the shop for cash by day %d if the savings project", *d.BuyoutByDay)
	}
	if d.MortgageSaveFromDay != nil {
		add("save for the mortgage from day %d", *d.MortgageSaveFromDay)
	}
	for _, p := range d.Production {
		if p.Note != "" {
			add("%s", p.Note)
		}
	}
	return out
}

// Changes lists the instructions in cur that prev did not have, and those dropped.
func Changes(prev, cur []string) []string {
	var out []string
	for _, c := range cur {
		if !slices.Contains(prev, c) {
			out = append(out, c)
		}
	}
	for _, p := range prev {
		if !slices.Contains(cur, p) {
			out = append(out, "no longer: "+p)
		}
	}
	return out
}

func without(list, drop []string) []string {
	var out []string
	for _, x := range list {
		if !slices.Contains(drop, x) {
			out = append(out, x)
		}
	}
	return out
}

func words(ids []string) string {
	ws := make([]string, len(ids))
	for i, id := range ids {
		ws[i] = strings.ToLower(strings.ReplaceAll(id, "_", " "))
	}
	return joinAnd(ws)
}

func items(uids []int64, name func(int64) string) string {
	counts := map[string]int{}
	var order []string
	for _, u := range uids {
		n := ""
		if name != nil {
			n = name(u)
		}
		if n == "" {
			n = "item"
		}
		if counts[n] == 0 {
			order = append(order, n)
		}
		counts[n]++
	}
	ws := make([]string, len(order))
	for i, n := range order {
		ws[i] = countOf(counts[n], n)
	}
	return joinAnd(ws)
}

func countOf(n int, what string) string {
	if n == 1 {
		return what
	}
	return fmt.Sprintf("%d× %s", n, what)
}

func joinAnd(ws []string) string {
	switch len(ws) {
	case 0:
		return ""
	case 1:
		return ws[0]
	}
	return strings.Join(ws[:len(ws)-1], ", ") + " and " + ws[len(ws)-1]
}

// uidRe matches the item uids plans cite ("#4253 #4251 …"): handles for the executor,
// noise for a viewer.
var uidRe = regexp.MustCompile(`(\s*#\d+)+( …)?`)

// leftRe is what a list of uids leaves behind once they are cut: "(, , )", "Large,,, Small".
var leftRe = []struct {
	re *regexp.Regexp
	to string
}{
	{regexp.MustCompile(`(\s*,)+(\s*[,)])`), "$2"},
	{regexp.MustCompile(`,(\s*\()`), "$1"},
	{regexp.MustCompile(`\(\s*\)`), ""},
	{regexp.MustCompile(`\(\s*,\s*`), "("},
	{regexp.MustCompile(`\s+([.,)])`), "$1"},
}

// stripUIDs cuts the uids from a plan line and tidies the punctuation they leave.
func stripUIDs(line string) string {
	line = uidRe.ReplaceAllString(line, "")
	for _, r := range leftRe {
		line = r.re.ReplaceAllString(line, r.to)
	}
	return strings.TrimSpace(line)
}

// ParsePlan splits the strategist's prose plan into its sections: a line in capitals
// ("GOALS", "CASH") opens one; "- " bullets are its lines. Text before any heading is an
// untitled section.
func ParsePlan(text string) []Section {
	var out []Section
	cur := -1
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if isHeading(line) {
			out = append(out, Section{Title: strings.TrimSuffix(strings.Trim(line, "#* "), ":")})
			cur = len(out) - 1
			continue
		}
		line = stripUIDs(strings.TrimLeft(line, "-*• "))
		if cur < 0 {
			out = append(out, Section{})
			cur = 0
		}
		out[cur].Lines = append(out[cur].Lines, line)
	}
	return out
}

func isHeading(line string) bool {
	t := strings.TrimSuffix(strings.Trim(line, "#* "), ":")
	if t == "" || len(t) > 24 || strings.HasPrefix(line, "-") {
		return false
	}
	return strings.ToUpper(t) == t && strings.ContainsFunc(t, func(r rune) bool { return r >= 'A' && r <= 'Z' })
}

// headlineQuota is where the plan says what today is for, and how many lines of each the
// panel has room for.
var headlineQuota = []struct {
	title string
	n     int
}{{"GOALS", 3}, {"CASH", 1}, {"RISK", 1}}

// Headline picks up to n lines that say what today is about.
func Headline(secs []Section, n int) []string {
	var out []string
	for _, q := range headlineQuota {
		for _, s := range secs {
			if !strings.EqualFold(s.Title, q.title) {
				continue
			}
			for _, l := range s.Lines[:min(q.n, len(s.Lines))] {
				if len(out) == n {
					return out
				}
				out = append(out, l)
			}
		}
	}
	if len(out) == 0 {
		for _, s := range secs {
			for _, l := range s.Lines {
				if len(out) == n {
					return out
				}
				out = append(out, l)
			}
		}
	}
	return out
}
