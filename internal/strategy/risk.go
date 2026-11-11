package strategy

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// inspectionThreshold is the trafficking record (FENCING included) below which no
// inspection is scheduled (docs/sim/SPEC.md §6.3, measured in docs/sim/VERIFIED.md).
const inspectionThreshold = 50

// rentReserve is the cash to hold back for rent: the rent due once rent day is two days
// away or less, else nothing (docs/research/EXPERIMENTS.md §5: fixed floors lose days).
func (s *snapshot) rentReserve() int64 {
	due, ok := s.Rent.due()
	if !ok || s.Rent.DaysLeft == nil || *s.Rent.DaysLeft > 2 {
		return 0
	}
	return due
}

// afford splits an upgrade's cost into the Wild favor and cash that would pay it (favor
// first); ok false when favor plus the cash above the rent reserve falls short.
func (s *snapshot) afford(u upgradeOffer) (favor, cash int64, ok bool) {
	if u.Cost == nil || s.Cash == nil {
		return 0, 0, false
	}
	var f int64
	if s.WildFavor != nil {
		f = max(*s.WildFavor, 0)
	}
	favor = min(f, *u.Cost)
	cash = *u.Cost - favor
	return favor, cash, cash <= *s.Cash-s.rentReserve()
}

func (s *snapshot) affordText(u upgradeOffer) string {
	favor, cash, ok := s.afford(u)
	if !ok {
		return ""
	}
	return fmt.Sprintf("favor %d + cash %d", favor, cash)
}

// buyStep is one clause of a persona's "Buy:" line, in the persona's order.
type buyStep struct {
	text    string
	minCash int64
}

var atCashRE = regexp.MustCompile(`at cash (\d[\d,]*)`)

func personaBuyOrder(persona string) []buyStep {
	var out []buyStep
	for _, line := range strings.Split(persona, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Buy:")
		if !ok {
			continue
		}
		for _, c := range strings.Split(rest, ";") {
			c = strings.TrimSpace(c)
			if c == "" {
				continue
			}
			st := buyStep{text: c}
			if m := atCashRE.FindStringSubmatch(strings.ToLower(c)); m != nil {
				st.minCash, _ = strconv.ParseInt(strings.ReplaceAll(m[1], ",", ""), 10, 64)
			}
			out = append(out, st)
		}
	}
	return out
}

// upgradeRefs are the lower-cased ways a plan or persona names u.
func upgradeRefs(u upgradeOffer) []string {
	var refs []string
	for _, r := range []string{u.Name, strings.ReplaceAll(u.ID, "_", " ")} {
		if r = strings.ToLower(strings.TrimSpace(r)); len(r) >= 4 && !slices.Contains(refs, r) {
			refs = append(refs, r)
		}
	}
	return refs
}

// buyNext is an upgrade on offer that the persona's buy order names, in that order.
type buyNext struct {
	u          upgradeOffer
	step       buyStep
	affordable bool
}

func (s *snapshot) buyOrder(persona string) []buyNext {
	var out []buyNext
	seen := map[string]bool{}
	for _, st := range personaBuyOrder(persona) {
		l := strings.ToLower(st.text)
		for _, u := range s.UpgradesOffer {
			if seen[u.name()] || !mentionsAny(l, upgradeRefs(u)) {
				continue
			}
			seen[u.name()] = true
			_, _, ok := s.afford(u)
			if st.minCash > 0 && (s.Cash == nil || *s.Cash < st.minCash) {
				ok = false
			}
			out = append(out, buyNext{u: u, step: st, affordable: ok})
		}
	}
	return out
}

// renderBuyOrder lists the upgrades on offer that the style's buy order names, so the plan
// can say BUY tonight for the next affordable one.
func (s *snapshot) renderBuyOrder(b *strings.Builder, persona string) {
	order := s.buyOrder(persona)
	if len(order) == 0 {
		return
	}
	b.WriteString("BUY ORDER (the style's purchases on offer now, in its order):\n")
	for _, n := range order {
		fmt.Fprintf(b, "- %s", n.u.text())
		if n.affordable {
			fmt.Fprintf(b, ": AFFORDABLE tonight (%s, rent reserve kept)", s.affordText(n.u))
		} else {
			b.WriteString(": not yet affordable")
		}
		fmt.Fprintf(b, " [style: %s]\n", n.step.text)
	}
}

// lintUpgrades flags a plan that does not mention an affordable upgrade that mitigates a
// near loss condition, or else the first affordable upgrade on the style's buy order.
func lintUpgrades(plan string, s *snapshot, persona string, day int) error {
	lower := strings.ToLower(plan)
	for _, n := range s.neededMitigations(day) {
		if !mentionsAny(lower, upgradeRefs(n.u)) {
			return fmt.Errorf("%s is within reach and %s pushes it back (%s): add \"- BUY tonight: %s\" under GOALS ahead of the style's purchases, or say there why not",
				n.cond.text, n.u.name(), n.text, n.u.name())
		}
	}
	for _, n := range s.buyOrder(persona) {
		if !n.affordable {
			continue
		}
		if mentionsAny(lower, upgradeRefs(n.u)) {
			return nil
		}
		return fmt.Errorf("the style's buy order names %s and the shop can afford it tonight (%s); add \"- BUY tonight: %s (%d: %s, why)\" under GOALS, or say there why not",
			n.u.name(), s.affordText(n.u), n.u.name(), *n.u.Cost, s.affordText(n.u))
	}
	return nil
}

// stolenRepCost is the Security rep an inspection takes for finding one stolen item:
// 1 plus 1-4 by value (ContrabandHelper.StartInspection severity).
func stolenRepCost(it item) int64 {
	v := int64(0)
	if it.EstimatedValue != nil {
		v = *it.EstimatedValue
	}
	switch {
	case v < 50:
		return 2
	case v < 85:
		return 3
	case v < 140:
		return 4
	}
	return 5
}

// exposed reports whether an inspection can find it: stolen and hot, outside a smuggler
// bay and the hidden stash.
func (s *snapshot) exposed(it item) bool {
	return it.stolen() && *it.StolenHeat > 0 && !s.inBay(it) && it.Where != "hidden"
}

// hotExposure is what one inspection is expected to take: each exposed hot item is found
// with chance = its heat % and confiscated, costing its value and Security rep.
func (s *snapshot) hotExposure() (n int, credits, rep int64) {
	var c, r int64
	for _, it := range s.Inventory {
		if !s.exposed(it) {
			continue
		}
		n++
		h := min(*it.StolenHeat, 100)
		if it.EstimatedValue != nil {
			c += h * *it.EstimatedValue
		}
		r += h * stolenRepCost(it)
	}
	return n, (c + 50) / 100, (r + 50) / 100
}

func (s *snapshot) trafficking() (int64, bool) {
	if s.Security == nil || s.Security.Trafficking == nil {
		return 0, false
	}
	return *s.Security.Trafficking, true
}

// bayRoom is the shop's smuggler bays and their grid cells; known is false when a bay
// does not report its capacity.
func (s *snapshot) bayRoom() (bays int, cells, free int64, known bool) {
	known = true
	for _, it := range s.Inventory {
		if !isBay(it) || strings.HasPrefix(it.Where, "inside:") {
			continue
		}
		bays++
		if it.Capacity == nil || it.Capacity.Cells == nil || it.Capacity.Free == nil {
			known = false
			continue
		}
		cells += *it.Capacity.Cells
		free += *it.Capacity.Free
	}
	return bays, cells, free, known && bays > 0
}

// renderBayRoom caps hot stock to the room the bays have, with the numbers, and prices a
// second bay by the loss it would hide.
func (s *snapshot) renderBayRoom(b *strings.Builder, exposed int, credits, rep int64) {
	bays, cells, free, known := s.bayRoom()
	switch {
	case bays == 0:
		if exposed > 0 {
			fmt.Fprintf(b, " No smuggler bay: every hot item is exposed; a bay is worth up to the expected loss it hides (about %d credits and %d Security rep per inspection).", credits, rep)
		}
	case !known:
		fmt.Fprintf(b, " Smuggler bays: %d (room not reported): cap hot stock to bay room.", bays)
	case free == 0:
		fmt.Fprintf(b, " Smuggler bays: %d, all %d cells full: buy no more hot stock than sells clean before the next inspection.", bays, cells)
		if exposed > 0 {
			fmt.Fprintf(b, " A second bay is worth up to the loss it would hide: about %d credits and %d Security rep per inspection.", credits, rep)
		}
	default:
		fmt.Fprintf(b, " Smuggler bays: %d with %d of %d cells free: cap hot stock to bay room (%d more cells).", bays, free, cells, free)
	}
}

func (s *snapshot) renderHotExposure(b *strings.Builder) {
	n, credits, rep := s.hotExposure()
	inBays := 0
	for _, it := range s.Inventory {
		if it.stolen() && s.inBay(it) {
			inBays++
		}
	}
	if n == 0 {
		if inBays > 0 {
			b.WriteString("Hot stock: all of it in smuggler bays.")
			s.renderBayRoom(b, 0, 0, 0)
			b.WriteString("\n")
		}
		return
	}
	fmt.Fprintf(b, "Hot stock exposed to an inspection: %d item(s) outside a smuggler bay (%d inside one); an inspection finds each with chance = its heat %% and CONFISCATES it: expected loss about %d credits and %d Security rep per inspection.", n, inBays, credits, rep)
	s.renderBayRoom(b, n, credits, rep)
	if t, ok := s.trafficking(); ok {
		if t < inspectionThreshold {
			fmt.Fprintf(b, " No scheduled inspection while the trafficking record (fencing included) stays under %d (now %d); patrols and a hot window still trigger one.", inspectionThreshold, t)
		} else {
			fmt.Fprintf(b, " Trafficking %d is at or over %d: inspections are scheduled every few days; keep hot stock to what the smuggler bays hold.", t, inspectionThreshold)
		}
	}
	hot := lossCond{key: "hot", rank: 10}
	if t, ok := s.trafficking(); ok && t >= inspectionThreshold-10 {
		hot.rank = 5
	}
	if m, _ := s.mitigate(hot); m != "" {
		b.WriteString(" -> " + m)
	}
	b.WriteString("\n")
}

// lossCond is one way this run can end, with how close it is.
type lossCond struct {
	key  string
	text string
	// rank orders the list: lower is closer.
	rank int64
	// imminent: one day or one event (a trade, an inspection, a visit) from ending the run.
	imminent bool
	// refs are what a plan that addresses it mentions (lower case).
	refs []string
}

// near: close enough that its mitigation comes before the style's path purchases.
func (c lossCond) near() bool { return c.imminent || c.rank <= 5 }

// mitigation is a way to push a loss condition back: an upgrade (by id, matched against
// the offer and owned lists) or a move with no purchase.
type mitigation struct {
	upgrade string
	text    string
}

// mitigations by loss key, best first (docs/re/MITIGATIONS.md). The state's own offer
// prices win over any cost named here.
var mitigations = map[string][]mitigation{
	"rent":         {{"LANDLORD_GRACE", "an armed grace turns a refused rent into a deferral owed next week (15-day cooldown from purchase; the landlord's plain greeting means it is armed)"}, {"", "sell to the rent-day merchant buyer and clean stock; Georgio repays 4x on a short rent day if you lent to him"}},
	"repo":         {{"", "refusing a payment seizes about one payment of eligible items and the run continues; keep stock outside smuggler bays, machines and containers worth more than a payment, or the cash"}},
	"arrest":       {{"", "move it into a smuggler bay (inspections skip it entirely)"}, {"SEC_GRACE", "armed Clemency turns a critical find or an unpayable high fine into a warning"}, {"INSPECTION_INFORMANT", "inspections announced 3, 2 and 1 days ahead (not the display-case inspector)"}},
	"sentencing":   {{"WILD_FIXER", "Wild's Fixers cut every crime type each night (15% + 50 at tier 1) at any evidence level, to keep the total under 300"}, {"EVIDENCE_REMOVAL", "zeroes the evidence but not the crime, so it regrows in 4-17 days (14-day cooldown, +100 each buy)"}, {"", "a sentence under 300 total crime is only a fine that wipes crime and evidence; below evidence 80 each crime type falls 20 a day; sell hot or contraband only to Black Market (crime /2) or Revolution (/3) buyers"}},
	"execution":    {{"WILD_FIXER", "Wild's Fixers cut every crime type each night"}, {"", "commit no new crime; below evidence 80 each crime type falls 20 a day; a sentence under 300 total is only a fine"}},
	"security":     {{"", "clean inspections (+3), reporting wanted customers (+5), shooting a real aug (+8), Security requests, clean trades with officers"}},
	"lower_levels": {{"", "trades with Lower Levels customers (about 1 point per 600 credits), Lower Levels requests (6-10), no walk-outs"}},
	"revolution":   {{"", "Revolution requests (4-6 each), shooting a real aug (+2), trades with Revolution customers"}},
	"cartel":       {{"", "meet the cartel's weekly dust count and purity"}},
	"aug":          {{"", "shoot augs; buy the Inventor's aug scanner when offered"}},
	"hot":          {{"", "move hot stock into a smuggler bay, or sell it the day it cools"}, {"INSPECTION_INFORMANT", "inspections announced 3 days ahead, time to bay or sell hot stock"}},
}

func (s *snapshot) owns(id string) bool {
	if slices.ContainsFunc(s.ownedIDs, func(o string) bool { return strings.EqualFold(o, id) }) {
		return true
	}
	for _, o := range s.UpgradesOwned {
		if strings.EqualFold(strings.ReplaceAll(o, " ", "_"), id) || strings.EqualFold(o, id) {
			return true
		}
	}
	return false
}

func (s *snapshot) offered(id string) (upgradeOffer, bool) {
	for _, u := range s.UpgradesOffer {
		if strings.EqualFold(u.ID, id) {
			return u, true
		}
	}
	return upgradeOffer{}, false
}

// mitigate is the best mitigation for c in this shop: owned upgrades first ("ready"),
// then one on offer (to buy tonight when c is near and it is affordable), then a move.
// buy is set when the plan should buy it tonight.
func (s *snapshot) mitigate(c lossCond) (text string, buy *upgradeOffer) {
	var moves []string
	for _, m := range mitigations[c.key] {
		if m.upgrade == "" {
			moves = append(moves, m.text)
		}
	}
	move := strings.Join(moves, "; ")
	for _, m := range mitigations[c.key] {
		if m.upgrade == "" {
			continue
		}
		if s.owns(m.upgrade) {
			return m.text + ": OWNED, keep it ready", nil
		}
		u, ok := s.offered(m.upgrade)
		if !ok || !c.near() {
			continue
		}
		favor, cash, aff := s.afford(u)
		if !aff && c.key == "rent" && s.Cash != nil {
			// Grace is bought instead of paying the rent, so the rent reserve is free for it.
			aff = cash <= *s.Cash
		}
		if aff {
			return fmt.Sprintf("BUY %s tonight (%d: favor %d + cash %d) — %s", u.name(), *u.Cost, favor, cash, m.text), &u
		}
		return fmt.Sprintf("%s on offer at %d, not affordable yet: save for it; meanwhile %s", u.name(), *u.Cost, nonEmpty(move, "hold the line")), nil
	}
	return move, nil
}

// neededMitigation is an upgrade to buy tonight to push back a near loss condition.
type neededMitigation struct {
	u    upgradeOffer
	cond lossCond
	text string
}

// neededMitigations are the upgrades the plan should buy tonight to push back a near loss
// condition, closest first.
func (s *snapshot) neededMitigations(day int) []neededMitigation {
	var out []neededMitigation
	for _, c := range s.lossConditions(day) {
		t, u := s.mitigate(c)
		if u != nil && !slices.ContainsFunc(out, func(o neededMitigation) bool { return o.u.ID == u.ID }) {
			out = append(out, neededMitigation{*u, c, t})
		}
	}
	return out
}

var factionEnding = map[string]string{
	"lower_levels": "the mob (Lynch)",
	"security":     "immediate arrest",
	"revolution":   "the shop bombed",
	"cartel":       "the cartel retires you",
}

func leadingInt(s string) (int64, bool) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, false
	}
	n, err := strconv.ParseInt(s[:i], 10, 64)
	return n, err == nil
}

// lossConditions lists every ending that applies to this run (docs/re/DAY.md, Endings),
// closest first.
func (s *snapshot) lossConditions(day int) []lossCond {
	var out []lossCond
	if due, ok := s.Rent.due(); ok {
		c := lossCond{key: "rent", refs: []string{"rent"}}
		grace := s.Rent.LandlordGrace != nil && *s.Rent.LandlordGrace
		days := int64(7)
		if s.Rent.DaysLeft != nil {
			days = *s.Rent.DaysLeft
		}
		cash := int64(-1)
		if s.Cash != nil {
			cash = *s.Cash
		}
		switch {
		case grace:
			c.text = fmt.Sprintf("rent: %d due in %d day(s); landlord grace owned (one missed rent carries over)", due, days)
			c.rank = 60 + days
		case cash >= due:
			c.text = fmt.Sprintf("rent: %d due in %d day(s), cash %d covers it; no landlord grace: refusing rent ends the run", due, days, cash)
			c.rank = 20 + days
		default:
			c.text = fmt.Sprintf("rent: %d due in %d day(s), cash %d is SHORT %d; no landlord grace: refusing rent ends the run", due, days, max(cash, 0), due-max(cash, 0))
			c.rank = days
			c.imminent = days <= 1
		}
		out = append(out, c)
	}
	if p := s.Property; p != nil && p.MortgageWeekly != nil {
		c := lossCond{key: "repo", refs: []string{"mortgage", "repo"}, rank: 25}
		var stock int64
		for _, it := range s.Inventory {
			// Smuggler-bay contents, machines and containers are never seized.
			if it.EstimatedValue != nil && !s.inBay(it) && !isBay(it) && machineKind(it.ID) == "" {
				stock += *it.EstimatedValue
			}
		}
		when := ""
		for _, d := range s.Calendar {
			if d.Day < int64(day) || when != "" {
				continue
			}
			for _, e := range d.Events {
				if strings.EqualFold(e.ID, "mortgage_payment") {
					when = fmt.Sprintf(" on day %d", d.Day)
					c.rank = d.Day - int64(day)
					if s.Cash != nil && *s.Cash < *p.MortgageWeekly {
						c.imminent = d.Day-int64(day) <= 1 && stock < *p.MortgageWeekly
					} else {
						c.rank += 20
					}
				}
			}
		}
		c.text = fmt.Sprintf("repo: mortgage payment %d%s; a refused payment seizes about one payment of stock (seizable stock about %d) and ends the run only when there is too little", *p.MortgageWeekly, when, stock)
		out = append(out, c)
	}
	inspections := false
	if t, ok := s.trafficking(); ok && t >= inspectionThreshold {
		inspections = true
	}
	var fine int64
	for _, it := range s.Inventory {
		if !it.contraband() || s.inBay(it) || it.Where == "hidden" {
			continue
		}
		lvl := *it.ContrabandLevel
		name := itemName(it) + uidText(it.UID)
		switch {
		case lvl >= 4:
			c := lossCond{key: "arrest", refs: refsFor(it, "contraband"), rank: 3}
			c.text = fmt.Sprintf("arrest: CRITICAL contraband %s outside a smuggler bay; found at an inspection = arrest (unless security grace)", name)
			if inspections || it.seenBySecurity() {
				c.rank, c.imminent = 1, true
			}
			out = append(out, c)
		case lvl == 3 && it.EstimatedValue != nil:
			fine += *it.EstimatedValue
		}
	}
	if fine > 0 {
		c := lossCond{key: "arrest", refs: []string{"contraband", "fine"}, rank: 8}
		c.text = fmt.Sprintf("arrest: HIGH contraband outside a smuggler bay; found, it is fined about %d, and an unpaid fine is arrest", fine)
		if s.Cash != nil && *s.Cash < fine {
			c.text += fmt.Sprintf(" (cash %d cannot pay it)", *s.Cash)
			c.rank = 4
			c.imminent = inspections
		}
		out = append(out, c)
	}
	if sec := s.Security; sec != nil {
		ev, evOK := leadingInt(sec.Evidence)
		var crime int64
		for _, c := range sec.Crimes {
			if c.Amount != nil {
				crime += *c.Amount
			}
		}
		if evOK {
			c := lossCond{key: "sentencing", refs: []string{"evidence", "sentenc"}, rank: (100 - ev) / 5}
			c.text = fmt.Sprintf("arrest by sentencing: evidence %d/100; at 100 a sentencing officer comes (crime falls 20 a day only while evidence is under 80)", ev)
			c.imminent = ev >= 90
			out = append(out, c)
		}
		if crime > 0 || sec.ProjectedSentence != "" {
			c := lossCond{key: "execution", refs: []string{"execution", "crime", "sentenc"}, rank: 40}
			c.text = fmt.Sprintf("execution: total crime %d of 1500 at sentencing", crime)
			if sec.ProjectedSentence != "" {
				c.text += " (projected sentence: " + sec.ProjectedSentence + ")"
			}
			if crime >= 1500 || strings.Contains(strings.ToLower(sec.ProjectedSentence), "execut") {
				c.rank = 10
				if evOK {
					c.rank = (100 - ev) / 5
					c.imminent = ev >= 90
				}
			}
			out = append(out, c)
		}
	}
	for _, id := range factionOrder {
		end, ok := factionEnding[id]
		r, has := s.Reputation[id]
		if !ok || !has || r.Value == nil {
			continue
		}
		dist := *r.Value + 100
		c := lossCond{key: id, refs: []string{strings.ToLower(factionName(id))}, rank: int64(dist / 3)}
		c.text = fmt.Sprintf("%s at -100 = %s: now %s, %s away", factionName(id), end, fmtNum(*r.Value), fmtNum(dist))
		c.imminent = dist <= 5
		out = append(out, c)
	}
	if c, ok := s.augCondition(day); ok {
		out = append(out, c)
	}
	for k, v := range s.storeExtra {
		if strings.Contains(strings.ToLower(k), "sentinel") {
			c := lossCond{key: "sentinel", refs: []string{"sentinel"}, rank: 35}
			c.text = fmt.Sprintf("sentinel addiction: %s %s; a pool of 20 ends the run", strings.ReplaceAll(k, "_", " "), v)
			if n, ok := leadingInt(v); ok {
				c.rank = 20 - n
				c.imminent = n >= 15
			}
			out = append(out, c)
		}
	}
	for _, it := range s.Inventory {
		if strings.Contains(strings.ToLower(it.ID), "reactor") {
			out = append(out, lossCond{key: "reactor", refs: []string{"reactor"}, rank: 45,
				text: fmt.Sprintf("reactor: %s%s can explode overnight, which ends the run", itemName(it), uidText(it.UID))})
			break
		}
	}
	slices.SortStableFunc(out, func(a, b lossCond) int { return int(a.rank - b.rank) })
	return out
}

func refsFor(it item, extra string) []string {
	refs := []string{strings.ToLower(itemName(it)), extra}
	if it.UID != nil {
		refs = append(refs, fmt.Sprintf("#%d", *it.UID))
	}
	return refs
}

func (s *snapshot) renderLossConditions(b *strings.Builder, day int) {
	cs := s.lossConditions(day)
	if len(cs) == 0 {
		return
	}
	b.WriteString("LOSS CONDITIONS for this run, closest first (RISK checks each):\n")
	for _, c := range cs {
		b.WriteString("- ")
		if c.imminent {
			b.WriteString("IMMINENT: ")
		}
		b.WriteString(c.text)
		if m, _ := s.mitigate(c); m != "" {
			b.WriteString(" -> " + m)
		}
		b.WriteString("\n")
	}
}

// lintLoss flags a plan that ignores a loss condition one day or one event from ending
// the run.
func lintLoss(plan string, s *snapshot, day int) []error {
	lower := strings.ToLower(plan)
	var errs []error
	for _, c := range s.lossConditions(day) {
		if c.imminent && !mentionsAny(lower, c.refs) {
			errs = append(errs, fmt.Errorf("RISK ignores a condition that can end the run within a day: %s; say how today's plan keeps it from triggering", c.text))
		}
	}
	return errs
}
