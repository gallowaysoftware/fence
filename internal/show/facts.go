package show

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Facts are what a narrator line may say about a moment: the numbers it was given and the
// claims they settle (is the rent covered, did the day go up or down, what sold for how
// much). The narrator invents colour, motive and character; the numbers are the game's.
type Facts struct {
	Day  int
	Cash *int64
	Rent *RentFact
	// Owned is set once the shop is bought: there is no rent to be short of.
	Owned bool
	// NetChange and CashChange are the day's, for a day summary.
	NetChange, CashChange *int64
	// Sold and Bought are the cash amounts a "sold for"/"bought for" may name.
	Sold, Bought []int64
	// Side is a trade's side ("sell", "buy"); Profit its profit when known.
	Side   string
	Profit *int64
	// Ghost is the run's standing against its lab book, on a ghost event.
	Ghost *Ghost
	// numbers is every other number the narrator was shown.
	numbers []int64
}

type RentFact struct {
	Owed    int64
	Days    int
	Covered bool
	// Margin is cash minus what is owed: what is spare when covered, the shortfall's
	// negative when not.
	Margin int64
	// Mortgage is set when what is owed is the mortgage's weekly payment, not rent.
	Mortgage bool
}

// FactsOf computes the facts of a moment from its typed payloads and its caption.
func FactsOf(ev Event) Facts {
	f := Facts{Day: ev.Day}
	add := func(ns ...int64) {
		for _, n := range ns {
			if n != 0 {
				f.numbers = append(f.numbers, abs(n))
			}
		}
	}
	add(int64(ev.Day))
	// Between snapshots the builder estimates cash from what it saw; income it did not see can
	// leave the estimate below zero (a 7,004 payback to Georgio in the full replay), which is no fact.
	if c := ev.Clock; c != nil && c.Cash >= 0 {
		cash := c.Cash
		f.Cash, f.Owned = &cash, c.Owned
		add(c.Cash, c.NetWorth, c.BuyoutPrice, c.MortgageWeekly, int64(c.MortgageWeeksLeft), c.OfferDown, c.OfferWeekly)
		if c.BuyoutPrice > 0 && c.NetWorth > 0 {
			add(c.BuyoutPrice - c.NetWorth)
		}
		// While a mortgage runs its weekly payment falls on the rent day instead of the rent.
		owed := c.RentDue + c.Arrears
		if c.MortgageWeekly > 0 {
			owed = c.MortgageWeekly + c.Arrears
		}
		if !c.Owned && owed > 0 {
			f.Rent = &RentFact{Owed: owed, Days: c.RentDays, Covered: c.Cash >= owed, Margin: c.Cash - owed, Mortgage: c.MortgageWeekly > 0}
			add(owed, c.Arrears, int64(c.RentDays), f.Rent.Margin)
			if !f.Rent.Mortgage {
				add(c.RentDue)
			}
		}
	}
	if t := ev.Trade; t != nil {
		f.Side = t.Side
		if t.HasProfit {
			p := t.Profit
			f.Profit = &p
		}
		switch t.Side {
		case "sell":
			f.Sold = append(f.Sold, t.Cash)
		case "buy":
			f.Bought = append(f.Bought, t.Cash)
		}
		add(t.Cash, t.Estimate, t.Cost, t.Profit, int64(t.Count), int64(t.Fakes))
	}
	if s := ev.Summary; s != nil {
		nc, cc := s.NetWorthDelta, s.CashDelta
		f.NetChange, f.CashChange = &nc, &cc
		f.Sold = append(f.Sold, s.Sold)
		f.Bought = append(f.Bought, s.Bought)
		add(s.NetWorthDelta, s.CashDelta, s.Sold, s.Bought, int64(s.SoldItems), int64(s.BoughtItems), int64(s.Customers))
		for _, r := range s.Rep {
			add(int64(r))
		}
	}
	if ev.Amount != 0 {
		add(ev.Amount)
		if ev.Kind == KindPurchase || ev.Amount < 0 {
			f.Bought = append(f.Bought, abs(ev.Amount))
		}
	}
	if h := ev.Heat; h != nil {
		add(h.Evidence, int64(h.HotItems))
	}
	if e := ev.End; e != nil {
		add(e.Cash, e.NetWorth, int64(e.Day))
	}
	if g := ev.Ghost; g != nil {
		f.Ghost = g
		add(g.NetWorth, g.P10, g.P50, g.P90)
	}
	texts := []string{ev.Text}
	if p := ev.Plan; p != nil {
		texts = slices.Concat(texts, p.Headline, p.Changes, p.Directives, p.Why, p.Goals)
	}
	if c := ev.Candidates; c != nil {
		add(int64(len(c.Options)))
		for _, o := range c.Options {
			add(int64(math.Round(o.Score)), int64(o.Rollouts))
			texts = slices.Concat(texts, []string{o.Label, o.Note}, o.Directives)
		}
	}
	for _, t := range texts {
		for _, m := range digitsRe.FindAllString(t, -1) {
			if n, ok := parseDigits(m); ok {
				add(n)
				if ev.Kind == KindPurchase {
					// "Bought Store Renovation I: 750, 117 of it in Wild's favor": the price
					// is 750 though 633 left the till.
					f.Bought = append(f.Bought, n)
				}
			}
		}
	}
	return f
}

// Prompt states the facts for the narrator, with the claims spelled out: small models
// otherwise read "rent 353" next to "cash 581" as a shortfall.
func (f Facts) Prompt() string {
	var b strings.Builder
	line := func(format string, a ...any) { fmt.Fprintf(&b, "- "+format+"\n", a...) }
	if f.Cash != nil {
		line("Cash on hand: %s.", num(*f.Cash))
	}
	switch r := f.Rent; {
	case f.Owned:
		line("You own the shop: no rent is owed.")
	case r != nil && r.Mortgage && r.Covered:
		line("No rent while the mortgage runs. Next mortgage payment: %s due in %s. Your cash covers it with %s to spare. You are NOT short.", num(r.Owed), days(r.Days), num(r.Margin))
	case r != nil && r.Mortgage:
		line("No rent while the mortgage runs. Next mortgage payment: %s due in %s. Cash does not cover it: you are short by %s.", num(r.Owed), days(r.Days), num(-r.Margin))
	case r != nil && r.Covered:
		line("Rent: %s due in %s. Your cash covers it with %s to spare. You are NOT short on rent.", num(r.Owed), days(r.Days), num(r.Margin))
	case r != nil:
		line("Rent: %s due in %s. Cash does not cover it: you are short by %s.", num(r.Owed), days(r.Days), num(-r.Margin))
	}
	if f.NetChange != nil {
		line("Today: net worth %s, cash %s.", direction(*f.NetChange), direction(*f.CashChange))
	}
	switch {
	case f.Side == "sell" && len(f.Sold) > 0:
		line("You sold for %s.", num(f.Sold[0]))
	case f.Side == "buy" && len(f.Bought) > 0:
		line("You bought for %s.", num(f.Bought[0]))
	}
	if f.Profit != nil {
		line("Profit on it: %s.", signed(*f.Profit))
	}
	if g := f.Ghost; g != nil {
		line("Against the book (the lab's test runs of this route on the same day): %s; net worth %s, the book's median %s.", against(g.Status, "it"), num(g.NetWorth), num(g.P50))
	}
	return b.String()
}

func direction(n int64) string {
	switch {
	case n > 0:
		return "up " + num(n)
	case n < 0:
		return "down " + num(-n)
	}
	return "unchanged"
}

func signed(n int64) string {
	if n < 0 {
		return "a loss of " + num(-n)
	}
	return num(n)
}

// A Fault is one thing a line gets wrong.
type Fault string

var (
	digitsRe = regexp.MustCompile(`\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?k?`)
	wordRe   = regexp.MustCompile(`[a-z]+|\d{1,3}(?:,\d{3})+|\d+(?:\.\d+)?k?`)
	// A shortfall claimed about money; "short of the buyout" is not about the rent.
	shortRe   = regexp.MustCompile(`\b(?:short (?:on (?:cash|money|rent)|for (?:the )?rent|of (?:the )?rent|by)|(?:i am|i'm|we're|we are|still|come up|coming up|fall|falling|running) short\b|can(?:'t|not) (?:cover|make|pay|afford)(?: the)? rent|not enough (?:cash |money )?(?:for|to cover|to pay) (?:the )?rent|behind on (?:the )?rent)`)
	notRentRe = regexp.MustCompile(`^\W*(?:\w+\W+){0,3}?(?:of |for )?(?:the )?(?:buyout|mortgage|down payment|permit|upgrade)`)
	coverRe   = regexp.MustCompile(`\b(?:covers?|covered|covering)\b[^.!?]*\brent\b|\brent\b[^.!?]*\b(?:covered|is safe|'s safe|is handled|'s handled)\b`)
	sentEndRe = regexp.MustCompile(`[.!?;]`)
	lossRe    = regexp.MustCompile(`\b(?:at a loss|lost money|took a loss)\b`)
	weekdayRe = regexp.MustCompile(`\b(?:monday|tuesday|wednesday|thursday|friday|saturday|sunday)s?\b`)
	profitRe  = regexp.MustCompile(`\b(?:a profit|turned a profit|for a profit|made a killing)\b`)
	bookRe    = `(?:the )?(?:book|ghost|lab|pace)\b`
	aheadRe   = regexp.MustCompile(`\b(?:ahead of|beating|outpacing|in front of|ahead on) ` + bookRe)
	behindRe  = regexp.MustCompile(`\b(?:behind|trailing|chasing|lagging|off) ` + bookRe)
)

// CheckLine finds what a narrator line says that the facts contradict or do not give:
// a number that matches none of them (rounding allowed), a rent shortfall when the cash
// covers it or cover when it does not, a day going the wrong way, a sale or a buy at the
// wrong amount, "I sold" on a buy. Colour and opinion pass; it checks only numbers and
// these claims.
func CheckLine(line string, f Facts) []Fault {
	var out []Fault
	fault := func(format string, a ...any) { out = append(out, Fault(fmt.Sprintf(format, a...))) }
	l := strings.ToLower(strings.NewReplacer("’", "'", "‘", "'", "—", ", ", "–", "-").Replace(line))

	if m := shortRe.FindStringIndex(l); m != nil && !notRentRe.MatchString(l[m[1]:]) {
		switch {
		case f.Owned:
			fault("says %q but the shop is owned: no rent is owed", l[m[0]:m[1]])
		case f.Rent != nil && f.Rent.Covered:
			fault("says %q but cash %s covers the rent %s with %s to spare", l[m[0]:m[1]], num(*f.Cash), num(f.Rent.Owed), num(f.Rent.Margin))
		}
	}
	if coverRe.MatchString(l) && f.Rent != nil && !f.Rent.Covered {
		fault("says the rent is covered but cash %s is %s short of %s", num(*f.Cash), num(-f.Rent.Margin), num(f.Rent.Owed))
	}
	if f.Rent != nil {
		for _, w := range []struct {
			word string
			days int
		}{{"tomorrow", 1}, {"today", 0}, {"tonight", 0}, {"a week", 7}} {
			if rentWhen(l, w.word) && f.Rent.Days != w.days && !(w.days == 7 && (f.Rent.Days == 6 || f.Rent.Days == 8)) {
				fault("says the rent is due %s but it is due in %s", w.word, days(f.Rent.Days))
			}
		}
	}
	// The facts never name the weekday (the game's and a viewer's calendars need not agree).
	if m := weekdayRe.FindString(l); m != "" {
		fault("names a weekday (%s); the facts give none", m)
	}
	if (strings.Contains(l, "i sold") || strings.Contains(l, "sold it")) && f.Side == "buy" {
		fault("says sold, but this was a buy")
	}
	if strings.Contains(l, "i bought") && f.Side == "sell" {
		fault("says bought, but this was a sale")
	}
	if f.Profit != nil {
		if *f.Profit > 0 && lossRe.MatchString(l) {
			fault("says a loss but the profit was %s", num(*f.Profit))
		}
		if *f.Profit < 0 && profitRe.MatchString(l) {
			fault("says a profit but it was a loss of %s", num(-*f.Profit))
		}
	}

	// Ahead and behind are against the book's median; the status holds them a little past it.
	if g := f.Ghost; g != nil {
		if m := aheadRe.FindString(l); m != "" && g.NetWorth < g.P50 {
			fault("says %q but net worth %s is under the book's median %s", m, num(g.NetWorth), num(g.P50))
		}
		if m := behindRe.FindString(l); m != "" && g.NetWorth > g.P50 {
			fault("says %q but net worth %s is over the book's median %s", m, num(g.NetWorth), num(g.P50))
		}
	}

	nums := Numbers(l)
	for i := 1; i < len(nums); i++ {
		// "twenty-five five" for 255: whatever it parses to, a viewer hears two numbers.
		if strings.Trim(l[nums[i-1].End:nums[i].Start], " -") == "" {
			fault("runs two numbers together (%q): say it as one number", l[nums[i-1].Start:nums[i].End])
		}
	}
	for _, n := range nums {
		before := l[max(0, n.Start-60):n.Start]
		if i := sentEndRe.FindAllStringIndex(before, -1); len(i) > 0 {
			before = before[i[len(i)-1][1]:]
		}
		said := l[n.Start:n.End]
		if dir := dirBefore(before); dir != 0 && f.NetChange != nil {
			if !sameWay(dir, *f.NetChange) && !sameWay(dir, *f.CashChange) {
				fault("says %s %s but the day went net %s, cash %s", map[int]string{1: "up", -1: "down"}[dir], said, direction(*f.NetChange), direction(*f.CashChange))
				continue
			}
		}
		switch verb := lastVerb(before); {
		case verb == "sold" && len(f.Sold) > 0:
			if !near(n.Value, f.Sold) {
				fault("says sold for %s but the sale was %s", said, num(f.Sold[0]))
			}
			continue
		case verb == "bought" && len(f.Bought) > 0:
			if !near(n.Value, f.Bought) {
				fault("says bought or paid %s but it was %s", said, num(f.Bought[0]))
			}
			continue
		}
		if n.Value < 13 && !n.Unit {
			continue
		}
		if !near(n.Value, f.numbers) {
			fault("names %s, which is not in the facts", said)
		}
	}
	return out
}

// rentWhen reports whether a sentence of l says the rent is due at word ("tomorrow").
func rentWhen(l, word string) bool {
	re := regexp.MustCompile(`\b(?:(?:due|owed|payable)(?: in)?|rent(?:'s| is)?|` + word + `'s) ` + word + `\b|\b` + word + `'s rent\b`)
	for _, s := range sentEndRe.Split(l, -1) {
		if strings.Contains(s, "rent") && re.MatchString(s) {
			return true
		}
	}
	return false
}

// dirBefore is +1 or -1 when the words just before a number say it is a rise or a fall
// ("down nearly", "up by", "lost").
func dirBefore(before string) int {
	ws := strings.Fields(before)
	for i := len(ws) - 1; i >= 0 && i >= len(ws)-3; i-- {
		switch strings.Trim(ws[i], ",") {
		case "down", "lost", "losing", "dropped", "fell", "bled":
			return -1
		case "up", "gained", "grew", "rose":
			return 1
		case "nearly", "almost", "about", "around", "roughly", "by", "over", "under", "just", "some", "a", "another":
			continue
		}
		return 0
	}
	return 0
}

func sameWay(dir int, n int64) bool { return (dir > 0 && n > 0) || (dir < 0 && n < 0) }

// lastVerb is the money verb nearest before a number in its sentence: "sold" or "bought".
func lastVerb(before string) string {
	best, at := "", -1
	for _, v := range []struct{ word, verb string }{
		{"sold", "sold"}, {"sell", "sold"}, {"selling", "sold"}, {"flipped", "sold"},
		{"bought", "bought"}, {"paid", "bought"}, {"spent", "bought"}, {"buying", "bought"}, {"buy", "bought"},
	} {
		if i := lastWord(before, v.word); i > at {
			best, at = v.verb, i
		}
	}
	return best
}

func lastWord(s, w string) int {
	for i := len(s); i > 0; {
		j := strings.LastIndex(s[:i], w)
		if j < 0 {
			return -1
		}
		if (j == 0 || !isLetter(s[j-1])) && (j+len(w) == len(s) || !isLetter(s[j+len(w)])) {
			return j
		}
		i = j
	}
	return -1
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' }

// near reports whether x is one of ns as a speaker would round it: within a tenth, or
// exact below 20.
func near(x int64, ns []int64) bool {
	for _, n := range ns {
		n = abs(n)
		if x == n || (n >= 20 && abs(x-n)*10 <= n) {
			return true
		}
	}
	return false
}

// A Number is one number in a line, in digits or words.
type Number struct {
	Value      int64
	Start, End int
	// Unit is set when days, weeks or the night's searches follow it, where small numbers are
	// facts too.
	Unit bool
}

var (
	units = map[string]int64{"zero": 0, "one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "niner": 9,
		"ten": 10, "eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19}
	tens = map[string]int64{"twenty": 20, "thirty": 30, "forty": 40, "fifty": 50, "sixty": 60, "seventy": 70, "eighty": 80, "ninety": 90}
)

// unitRe starts the words after a number that make a small number a fact: "three scavenges"
// for a night of five went on air in the variety replay.
var unitRe = regexp.MustCompile(`^(?:day|week|scaveng|search|trip|dig)`)

// Numbers finds the numbers in a lower-case line: digits ("1,304", "1.2k") and words
// ("a hundred and thirty", "five hundred eighty one", "three fifty").
func Numbers(l string) []Number {
	toks := wordRe.FindAllStringIndex(l, -1)
	var out []Number
	for i := 0; i < len(toks); i++ {
		w := l[toks[i][0]:toks[i][1]]
		if w[0] >= '0' && w[0] <= '9' {
			if v, ok := parseDigits(w); ok {
				out = append(out, Number{Value: v, Start: toks[i][0], End: toks[i][1]})
			}
			continue
		}
		v, end, ok := wordNumber(l, toks, i)
		if !ok {
			continue
		}
		out = append(out, Number{Value: v, Start: toks[i][0], End: toks[end][1]})
		i = end
	}
	for k := range out {
		rest := strings.TrimLeft(l[out[k].End:], " ")
		out[k].Unit = unitRe.MatchString(rest)
	}
	return out
}

// wordNumber reads a number in words starting at token i; end is its last token.
func wordNumber(l string, toks [][]int, i int) (v int64, end int, ok bool) {
	tok := func(k int) string {
		if k >= len(toks) {
			return ""
		}
		return l[toks[k][0]:toks[k][1]]
	}
	var total, cur int64
	end = -1
	lastSmall := false // the previous word was 1-19 with no scale yet ("three fifty")
	for k := i; k < len(toks); k++ {
		w := tok(k)
		if k > i {
			// "seven fifty. one hundred seventeen" is two numbers, not 750,117; "three
			// thousand, two hundred" is one.
			gap := strings.Trim(l[toks[k-1][1]:toks[k][0]], " -")
			if gap != "" && !(gap == "," && (tok(k-1) == "thousand" || tok(k-1) == "grand")) {
				break
			}
		}
		switch {
		case (w == "a" || w == "an") && k == i && (tok(k+1) == "hundred" || tok(k+1) == "thousand"):
			cur, lastSmall, end = 1, false, k
			continue
		case w == "and" && end >= 0 && k+1 < len(toks) && (units[tok(k+1)] > 0 || tens[tok(k+1)] > 0):
			continue
		case units[w] > 0 || w == "zero":
			if end >= 0 && cur%10 != 0 && !lastSmall {
				return total + cur, end, true
			}
			cur += units[w]
			// "three fifty", and after a thousand "nine thousand five seventy-eight"
			afterK := k > i && (tok(k-1) == "thousand" || tok(k-1) == "grand")
			lastSmall = cur < 20 && (total == 0 && end < 0 || afterK)
		case tens[w] > 0:
			if lastSmall {
				cur = cur*100 + tens[w]
			} else {
				cur += tens[w]
			}
			lastSmall = false
		case w == "hundred" && end >= 0:
			cur *= 100
			lastSmall = false
		case (w == "thousand" || w == "grand") && end >= 0:
			total += cur * 1000
			cur, lastSmall = 0, false
		default:
			if end < 0 {
				return 0, 0, false
			}
			return total + cur, end, true
		}
		end = k
	}
	if end < 0 {
		return 0, 0, false
	}
	return total + cur, end, true
}

func parseDigits(s string) (int64, bool) {
	s = strings.ReplaceAll(s, ",", "")
	mult := 1.0
	if strings.HasSuffix(s, "k") {
		s, mult = strings.TrimSuffix(s, "k"), 1000
	}
	if mult == 1 && !strings.Contains(s, ".") {
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int64(f*mult + 0.5), true
}
