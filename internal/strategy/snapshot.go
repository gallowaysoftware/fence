package strategy

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

// HeatDecayPerDay is how much a stolen item's heat falls each night, contraband included:
// UpdateStolenItem's contraband slow-down never fires (measured, docs/sim/VERIFIED.md).
const HeatDecayPerDay = 18

var factionNames = map[string]string{
	"lower_levels": "Lower Levels",
	"upper_levels": "Upper Levels",
	"security":     "Security",
	"black_market": "Black Market",
	"revolution":   "Revolution",
	"cartel":       "Cartel",
}

// factionOrder is the order the game's reputation screen uses.
var factionOrder = []string{"lower_levels", "upper_levels", "security", "black_market", "revolution", "cartel"}

func factionName(id string) string {
	if n, ok := factionNames[id]; ok {
		return n
	}
	return id
}

type threshold struct {
	at   int
	perk string
}

// thresholds are the perk/penalty steps from StoreReputationPerk (docs/re/STATE.md),
// ascending.
var thresholds = map[string][]threshold{
	"lower_levels": {{-100, "Lynch"}, {-80, "Avoided"}, {-60, "Target"}, {-40, "Distrusted"}, {-20, "PoorReviews"}, {20, "Referral"}, {40, "RareGood"}, {60, "Tips"}, {80, "TightLips"}, {100, "Pillar"}},
	"upper_levels": {{-100, "Hitman"}, {-60, "Petition"}, {-40, "TouristTrap"}, {0, "DiscerningTaste"}, {20, "Tips"}, {40, "Charity"}, {60, "Gossip"}, {80, "HouseholdName"}, {100, "TopReview"}},
	"security":     {{-100, "ImmediateArrest"}, {-80, "ThoroughInspection"}, {-60, "Priority"}, {-40, "AlarmIgnored"}, {-20, "Watchlist"}, {0, "Requisition"}, {20, "Surplus"}, {40, "Protection"}, {60, "Trusted"}, {80, "ReliableVendor"}, {100, "VIP"}},
	"black_market": {{-60, "Uninsurable"}, {-40, "Distrusted"}, {0, "Honor"}, {20, "Broker"}, {40, "Reliable"}, {60, "Network"}},
	"revolution":   {{-100, "Hit"}, {-40, "Blacklisted"}, {20, "Request"}, {40, "Quartermaster"}, {60, "Friend"}, {80, "RaidLoot"}},
	"cartel":       {{-100, "Retired"}, {25, "IncreasedStipend"}, {50, "ReliableChemist"}, {100, "AuthorizedExit"}},
}

// endsRun reports whether reaching -100 with faction id ends the run: every faction with
// a bar except the Black Market.
func endsRun(id string) bool { return id != "black_market" }

// nextPerk is the lowest positive-side threshold above v; the snapshot's own next_perk
// wins when present.
func nextPerk(id string, v float64, r repEntry) (threshold, bool) {
	if r.NextPerk != nil && r.NextPerk.ID != "" && r.NextPerk.Threshold != nil {
		return threshold{int(*r.NextPerk.Threshold), r.NextPerk.ID}, true
	}
	for _, t := range thresholds[id] {
		if t.at >= 0 && float64(t.at) > v {
			return t, true
		}
	}
	return threshold{}, false
}

// nextPenalty is the highest negative threshold below v; the snapshot's own
// next_negative_perk wins when present.
func nextPenalty(id string, v float64, r repEntry) (threshold, bool) {
	if p := r.NextNegativePerk; p != nil && p.ID != "" && p.Threshold != nil {
		return threshold{int(*p.Threshold), p.ID}, true
	}
	ts := thresholds[id]
	for i := len(ts) - 1; i >= 0; i-- {
		if ts[i].at < 0 && float64(ts[i].at) < v {
			return ts[i], true
		}
	}
	return threshold{}, false
}

func fmtNum(v float64) string {
	return strconv.FormatFloat(math.Round(v*100)/100, 'f', -1, 64)
}

func signed(v float64) string {
	if v > 0 {
		return "+" + fmtNum(v)
	}
	return fmtNum(v)
}

// coolDays is the nights until it's heat reaches 0, when it stops being stolen and sells
// clean (no fencing crime).
func (it item) coolDays() int64 {
	if it.StolenHeat == nil || *it.StolenHeat <= 0 {
		return 0
	}
	return (*it.StolenHeat + HeatDecayPerDay - 1) / HeatDecayPerDay
}

// inBay reports whether it sits inside a smuggler bay, which inspections do not search.
func (s *snapshot) inBay(it item) bool {
	uid, ok := strings.CutPrefix(it.Where, "inside:")
	if !ok {
		return false
	}
	for _, c := range s.Inventory {
		if c.UID != nil && fmt.Sprint(*c.UID) == uid {
			return isBay(c)
		}
	}
	return false
}

func isBay(it item) bool { return strings.HasPrefix(strings.ToLower(it.ID), "smuggler_bay") }

func (s *snapshot) whereLabel(it item) string {
	if s.inBay(it) {
		return "smuggler bay #" + strings.TrimPrefix(it.Where, "inside:") + " (inspections do not search it)"
	}
	if uid, ok := strings.CutPrefix(it.Where, "inside:"); ok {
		for _, c := range s.Inventory {
			if c.UID != nil && fmt.Sprint(*c.UID) == uid {
				return fmt.Sprintf("%s #%s", itemName(c), uid)
			}
		}
	}
	return whereLabel(it.Where)
}

// render writes the snapshot compactly for a prompt. Absent values are left out.
func (s *snapshot) render(b *strings.Builder, day int) {
	var shop []string
	if s.Weekday != "" {
		shop = append(shop, s.Weekday)
	}
	if s.Cash != nil {
		shop = append(shop, fmt.Sprintf("cash %d", *s.Cash))
	}
	if s.NetWorth != nil {
		shop = append(shop, fmt.Sprintf("net worth %d", *s.NetWorth))
	}
	if s.WildFavor != nil {
		shop = append(shop, fmt.Sprintf("wild favor %d", *s.WildFavor))
	}
	if s.Attractiveness != nil {
		shop = append(shop, fmt.Sprintf("attractiveness %d", *s.Attractiveness))
	}
	if s.RetailMarkup != nil {
		shop = append(shop, fmt.Sprintf("retail markup %d%% (fixed)", *s.RetailMarkup))
	}
	if s.Queue != nil && *s.Queue > 0 {
		shop = append(shop, fmt.Sprintf("%d customers queued", *s.Queue))
	}
	if c := s.Clientele; c != nil && c.NormalCustomers != nil {
		shop = append(shop, fmt.Sprintf("%d regular customers a day", *c.NormalCustomers))
	}
	for _, k := range sortedKeys(s.storeExtra) {
		shop = append(shop, k+" "+s.storeExtra[k])
	}
	if len(shop) > 0 {
		fmt.Fprintf(b, "Shop: %s.\n", strings.Join(shop, ", "))
	}
	s.renderRent(b, day)
	s.renderProperty(b)
	s.renderReputation(b)
	s.renderPerks(b)
	s.renderInventory(b, day)
	s.renderClientele(b)
	s.renderWindowMath(b, day)
	s.renderProduction(b, day)
	s.renderUseValues(b, s.production())
	s.renderSecurity(b)
	s.renderHotExposure(b)
	s.renderServices(b)
	s.renderAugs(b, day)
	s.renderLossConditions(b, day)
	s.renderMarket(b, day)
	for _, k := range sortedKeys(s.other) {
		fmt.Fprintf(b, "%s: %s\n", strings.ReplaceAll(k, "_", " "), s.other[k])
	}
	if len(s.Requests) > 0 {
		b.WriteString("Requests board:\n")
		for i, r := range s.Requests {
			if i == 6 {
				break
			}
			b.WriteString("- " + r + s.requestUseNote(i) + "\n")
		}
	}
	if len(s.NightLog) > 0 {
		b.WriteString("Night log:\n")
		for i, l := range s.NightLog {
			if i == 8 {
				break
			}
			if l = plainLog(l); l != "" {
				b.WriteString("- " + clip(l, 160) + "\n")
			}
		}
	}
}

func (s *snapshot) renderRent(b *strings.Builder, day int) {
	r := s.Rent
	due, ok := r.due()
	if !ok {
		return
	}
	var parts []string
	t := fmt.Sprintf("Rent: %d due", due)
	if r.Arrears != nil && *r.Arrears > 0 {
		t += fmt.Sprintf(" (%d rent + %d arrears)", *r.Rent, *r.Arrears)
	}
	if r.DaysLeft != nil {
		t += fmt.Sprintf(" in %d day(s) (day %d)", *r.DaysLeft, day+int(*r.DaysLeft))
	}
	parts = append(parts, t)
	if s.Cash != nil {
		if left := *s.Cash - due; left >= 0 {
			parts = append(parts, fmt.Sprintf("cash after rent %d", left))
		} else {
			parts = append(parts, fmt.Sprintf("SHORT %d", -left))
		}
	}
	if g := r.LandlordGrace; g != nil {
		if *g {
			parts = append(parts, "landlord grace owned (a missed rent carries over as arrears)")
		} else {
			parts = append(parts, "no landlord grace: not paying ends the run")
		}
	}
	fmt.Fprintf(b, "%s.\n", strings.Join(parts, "; "))
	s.renderRunway(b, day)
}

// renderRunway says how rent can be met when cash alone does not cover it: what clean
// stock is worth, and which stolen items cool in time to be sold clean.
func (s *snapshot) renderRunway(b *strings.Builder, day int) {
	due, ok := s.Rent.due()
	if !ok || s.Cash == nil || s.Rent.DaysLeft == nil || *s.Cash >= due {
		return
	}
	var clean int64
	var cooling []string
	for _, it := range s.Inventory {
		if it.EstimatedValue == nil || it.contraband() {
			continue
		}
		if !it.stolen() {
			clean += *it.EstimatedValue
		} else if d := it.coolDays(); d < *s.Rent.DaysLeft {
			cooling = append(cooling, fmt.Sprintf("%s (est. %d, clean on day %d)", itemName(it), *it.EstimatedValue, day+int(d)))
		}
	}
	fmt.Fprintf(b, "Rent runway: %d short of rent due on day %d; clean stock is worth about %d at estimate", due-*s.Cash, day+int(*s.Rent.DaysLeft), clean)
	if len(cooling) > 0 {
		fmt.Fprintf(b, "; stolen items clean before then: %s", strings.Join(cooling, ", "))
	}
	b.WriteString(".\n")
}

func (s *snapshot) renderProperty(b *strings.Builder) {
	p := s.Property
	if p == nil {
		return
	}
	var parts []string
	if p.Owned != nil && *p.Owned {
		parts = append(parts, "shop owned")
	}
	if p.BuyoutPrice != nil {
		t := fmt.Sprintf("buyout price %d", *p.BuyoutPrice)
		if s.Cash != nil && *s.Cash < *p.BuyoutPrice {
			t += fmt.Sprintf(" (%d to go)", *p.BuyoutPrice-*s.Cash)
		}
		parts = append(parts, t)
	}
	if p.BuyoutAffordable != nil && *p.BuyoutAffordable {
		parts = append(parts, "BUYOUT AFFORDABLE NOW (the landlord offers it on rent day)")
	}
	if p.MortgageWeekly != nil {
		t := fmt.Sprintf("mortgage %d a week", *p.MortgageWeekly)
		if p.WeeksLeft != nil {
			t += fmt.Sprintf(", %d weeks left", *p.WeeksLeft)
		}
		parts = append(parts, t)
	}
	if p.OfferDown != nil {
		t := fmt.Sprintf("mortgage offered: %d down", *p.OfferDown)
		if p.OfferWeekly != nil && p.OfferWeeks != nil {
			t += fmt.Sprintf(", then %d a week for %d weeks", *p.OfferWeekly, *p.OfferWeeks)
		}
		parts = append(parts, t)
	}
	if len(parts) > 0 {
		fmt.Fprintf(b, "Property: %s.\n", strings.Join(parts, "; "))
	}
}

func (s *snapshot) renderReputation(b *strings.Builder) {
	ids := make([]string, 0, len(s.Reputation))
	for _, id := range factionOrder {
		if _, ok := s.Reputation[id]; ok {
			ids = append(ids, id)
		}
	}
	for _, id := range sortedKeys(s.Reputation) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	b.WriteString("Reputation (-200..200):\n")
	for _, id := range ids {
		r := s.Reputation[id]
		if r.Value == nil {
			continue
		}
		v := *r.Value
		fmt.Fprintf(b, "- %s %s", factionName(id), fmtNum(v))
		if r.Status != "" {
			fmt.Fprintf(b, " (%s)", r.Status)
		}
		if t, ok := nextPerk(id, v, r); ok {
			fmt.Fprintf(b, "; next perk %s at %d, %s to go", t.perk, t.at, fmtNum(float64(t.at)-v))
		}
		t, ok := nextPenalty(id, v, r)
		if ok {
			fmt.Fprintf(b, "; next penalty %s at %d, %s away", t.perk, t.at, fmtNum(v-float64(t.at)))
			if t.at == -100 && endsRun(id) {
				b.WriteString(" (ENDS THE RUN)")
			}
		}
		if endsRun(id) && v > -100 && v+100 <= 25 {
			b.WriteString("; DANGER")
			if !ok || t.at != -100 {
				fmt.Fprintf(b, ": %s from -100, which ends the run", fmtNum(v+100))
			}
		}
		if u := r.unlocked(); len(u) > 0 {
			fmt.Fprintf(b, "; unlocked: %s", strings.Join(u, ", "))
		}
		b.WriteString("\n")
	}
}

// renderClientele is the window: what it holds now and whom it is forecast to draw.
func (s *snapshot) renderClientele(b *strings.Builder) {
	var shown []string
	for _, it := range s.Inventory {
		if it.Where == "showcase" {
			t := itemName(it)
			if it.UID != nil {
				t += fmt.Sprintf(" #%d", *it.UID)
			}
			switch {
			case it.contraband():
				t += " (CONTRABAND: guarantees an inspection at every opening)"
			case it.stolen():
				t += fmt.Sprintf(" (STOLEN: %d%% chance of an inspection at each opening)", *it.StolenHeat)
			}
			shown = append(shown, t)
		}
	}
	if s.Inventory != nil {
		fmt.Fprintf(b, "Window (display case) now: %s.\n", nonEmpty(strings.Join(shown, ", "), "empty"))
	}
	c := s.Clientele
	if c == nil {
		return
	}
	if len(c.DisplayBuyers) > 0 {
		fmt.Fprintf(b, "The window as it stands draws buyers for: %s.\n", strings.Join(c.DisplayBuyers, ", "))
	}
	if d := c.Discerning; d != nil && d.Cooldown != nil && *d.Cooldown > 0 {
		fmt.Fprintf(b, "Discerning shopper on cooldown for %d more openings.\n", *d.Cooldown)
	}
	if len(c.Predictions) > 0 {
		byID := map[string]string{}
		for _, it := range s.Inventory {
			if it.ID != "" && it.Name != "" {
				byID[it.ID] = it.Name
			}
		}
		var ps []string
		for _, p := range c.Predictions {
			t := nonEmpty(p.Client, "a customer")
			if p.Chance != nil {
				t += fmt.Sprintf(" %d%%", *p.Chance)
			}
			ps = append(ps, t+" for "+nonEmpty(byID[p.Item], p.Item))
		}
		fmt.Fprintf(b, "Window special visitors (rolled, drawn from the regular customers): %s.\n", strings.Join(ps, "; "))
	}
}

func (s *snapshot) renderSecurity(b *strings.Builder) {
	sec := s.Security
	if sec == nil {
		return
	}
	if sec.Visible != nil && !*sec.Visible {
		b.WriteString("Security record: not visible (no record document).\n")
		return
	}
	var parts []string
	if sec.Trafficking != nil {
		parts = append(parts, fmt.Sprintf("trafficking %d", *sec.Trafficking))
	}
	if len(sec.Crimes) > 0 {
		var cs []string
		for _, c := range sec.Crimes {
			if c.Amount != nil {
				cs = append(cs, fmt.Sprintf("%s %d", strings.ToLower(c.ID), *c.Amount))
			}
		}
		if len(cs) > 0 {
			parts = append(parts, "crimes "+strings.Join(cs, ", "))
		}
	}
	for _, f := range []struct{ k, v string }{
		{"evidence", sec.Evidence}, {"suspected of", sec.Suspected}, {"projected sentence", sec.ProjectedSentence},
		{"case officer", sec.CaseOfficer}, {"recommended action", sec.Action}, {"operation", sec.Operation}, {"note", sec.Note},
	} {
		if f.v != "" {
			parts = append(parts, f.k+" "+f.v)
		}
	}
	for _, k := range sortedKeys(sec.extra) {
		parts = append(parts, k+" "+sec.extra[k])
	}
	if len(parts) > 0 {
		fmt.Fprintf(b, "Security record: %s.\n", strings.Join(parts, ", "))
	}
}

func (s *snapshot) renderServices(b *strings.Builder) {
	if len(s.UpgradesOwned) > 0 {
		fmt.Fprintf(b, "Upgrades owned: %s.\n", strings.Join(s.UpgradesOwned, ", "))
	}
	if len(s.UpgradesOffer) > 0 {
		var offer []string
		for i, u := range s.UpgradesOffer {
			if i == 10 {
				offer = append(offer, "…")
				break
			}
			t := u.text()
			if a := s.affordText(u); a != "" {
				t += " affordable: " + a
			}
			offer = append(offer, t)
		}
		fav := ""
		if s.WildFavor != nil {
			fav = fmt.Sprintf(", %d favor on hand", *s.WildFavor)
		}
		fmt.Fprintf(b, "Upgrades available (paid from Wild favor first, then cash%s): %s.\n", fav, strings.Join(offer, ", "))
	}
	if f := s.favorText(); f != "" {
		fav := ""
		if s.WildFavor != nil {
			fav = fmt.Sprintf(" (%d favor on hand)", *s.WildFavor)
		}
		fmt.Fprintf(b, "FAVOR ORDER, not yet bought%s: %s.\n", fav, f)
	}
	if s.wineIdle != nil {
		fmt.Fprintf(b, "Wine engine: last new finished wine %d day(s) ago.\n", *s.wineIdle)
	}
	var using []string
	for _, v := range s.Services {
		if !v.inUse() {
			continue
		}
		t := nonEmpty(v.Name, v.ID)
		if v.Cost != nil {
			t += fmt.Sprintf(" (%d a night)", *v.Cost)
		}
		using = append(using, t)
	}
	if len(using) > 0 {
		fmt.Fprintf(b, "Services in use: %s.\n", strings.Join(using, ", "))
	}
	if s.earlyWarning() {
		b.WriteString("Early warning: an inspection is announced in the night log 3, 2 and 1 days ahead; no announcement means none within 3 days.\n")
	}
}

func (s *snapshot) renderMarket(b *strings.Builder, day int) {
	var ev []string
	for _, e := range s.Active {
		if t := e.text(); t != "" {
			ev = append(ev, t)
		}
	}
	if len(ev) > 0 {
		fmt.Fprintf(b, "Market today: %s.\n", strings.Join(ev, "; "))
	}
	var lines []string
	seen := map[string]bool{}
	for _, d := range s.Calendar {
		if d.Day <= int64(day) || d.Day > int64(day+calendarDays) {
			continue
		}
		var es []string
		for _, e := range d.Events {
			start := d.Day
			if e.StartDay != nil {
				start = *e.StartDay
			}
			key := fmt.Sprintf("%s@%d", e.Name, start)
			if seen[key] {
				continue
			}
			seen[key] = true
			if t := calendarText(e); t != "" {
				es = append(es, t+s.holding(e, d.Day))
			}
		}
		if len(es) > 0 {
			lines = append(lines, fmt.Sprintf("- day %d (in %d): %s", d.Day, d.Day-int64(day), strings.Join(es, "; ")))
		}
	}
	if len(lines) > 0 {
		fmt.Fprintf(b, "Calendar, next %d days:\n%s\n", calendarDays, strings.Join(lines, "\n"))
	} else if s.Calendar != nil {
		fmt.Fprintf(b, "Calendar, next %d days: nothing.\n", calendarDays)
	}
}

// holding says what the shop holds of an event's types and which way to trade them, so
// the plan cannot read a rise as a reason to sell early.
func (s *snapshot) holding(e marketEvent, first int64) string {
	start := first
	if e.StartDay != nil {
		start = *e.StartDay
	}
	end := start
	if e.Duration != nil && *e.Duration > 0 {
		end = start + *e.Duration - 1
	}
	var parts []string
	for i, t := range e.types {
		var n int
		var v int64
		for _, it := range s.Inventory {
			if it.displayable() && slices.Contains(it.types(), t) {
				n++
				if it.EstimatedValue != nil {
					v += *it.EstimatedValue
				}
			}
		}
		p := fmt.Sprintf("you hold %d %s items worth %d", n, t, v)
		switch pct := e.pcts[i]; {
		case pct > 0:
			p += fmt.Sprintf(": buy %s before day %d, sell it on days %d-%d, not before", t, start, start, end)
		case pct < 0:
			p += fmt.Sprintf(": sell %s before day %d, buy it cheap on days %d-%d", t, start, start, end)
		}
		parts = append(parts, p)
	}
	if len(parts) == 0 {
		return ""
	}
	return " [" + strings.Join(parts, "; ") + "]"
}

// calendarDays is how far ahead the plan looks; the game shows 14.
const calendarDays = 7

var innateText = map[string]string{
	"rent_day":         "rent day",
	"mortgage_payment": "mortgage payment",
	"cartel_visit":     "cartel visit",
}

func calendarText(e marketEvent) string {
	if t, ok := innateText[strings.ToLower(e.ID)]; ok {
		return t
	}
	return e.text()
}

type group struct {
	name        string
	where       map[string]int
	n           int
	est, paid   int64
	estN, paidN int
	uids        []int64
}

func (s *snapshot) renderInventory(b *strings.Builder, day int) {
	if s.Inventory == nil {
		return
	}
	var est, paid int64
	var stolen, contra []item
	groups := map[string]*group{}
	for _, it := range s.Inventory {
		if it.EstimatedValue != nil {
			est += *it.EstimatedValue
		}
		if it.BoughtAt != nil {
			paid += *it.BoughtAt
		}
		switch {
		case it.stolen():
			stolen = append(stolen, it)
		case it.contraband():
			contra = append(contra, it)
		case isBay(it):
		default:
			name := itemName(it)
			if t := it.itemType(); t != "" {
				name += " (" + t + ")"
			}
			if f := featureText(it); f != "" {
				name += " [" + f + "]"
			}
			g := groups[name]
			if g == nil {
				g = &group{name: name, where: map[string]int{}}
				groups[name] = g
			}
			g.n++
			if it.UID != nil {
				g.uids = append(g.uids, *it.UID)
			}
			g.where[nonEmpty(it.Where, "stock")]++
			if it.EstimatedValue != nil {
				g.est += *it.EstimatedValue
				g.estN++
			}
			if it.BoughtAt != nil {
				g.paid += *it.BoughtAt
				g.paidN++
			}
		}
	}
	fmt.Fprintf(b, "Inventory: %d items, est. value %d, paid %d.\n", len(s.Inventory), est, paid)

	slices.SortStableFunc(stolen, func(a, c item) int { return int(*c.StolenHeat - *a.StolenHeat) })
	if len(stolen) > 0 {
		fmt.Fprintf(b, "Stolen (heat falls %d a night, contraband too; at heat 0 it is no longer stolen and sells clean; selling it hot is a fencing crime that grows with its heat):\n", HeatDecayPerDay)
		for _, it := range stolen {
			b.WriteString("- " + s.itemLine(it))
			h := *it.StolenHeat
			if d := it.coolDays(); d > 0 {
				fmt.Fprintf(b, ", heat %d, sells clean from day %d", h, day+int(d))
				if r := s.Rent; r != nil && r.DaysLeft != nil && d < *r.DaysLeft {
					fmt.Fprintf(b, ", before rent is due on day %d", day+int(*r.DaysLeft))
				}
			} else {
				b.WriteString(", heat 0 (sells clean now)")
			}
			if it.contraband() {
				b.WriteString(", also " + contraText(it))
			}
			if h > 0 && !s.inBay(it) {
				b.WriteString(" [EXPOSED: not in a smuggler bay; an inspection finds it with chance = its heat % and confiscates it]")
			}
			if it.seenBySecurity() {
				fmt.Fprintf(b, " [ON %s: visible to security]", strings.ToUpper(strings.ReplaceAll(it.Where, "_", " ")))
			}
			b.WriteString("\n")
		}
	}
	if len(contra) > 0 {
		b.WriteString("Contraband (fined if found at an inspection; serious contraband found means arrest):\n")
		for _, it := range contra {
			fmt.Fprintf(b, "- %s, %s", s.itemLine(it), contraText(it))
			if it.seenBySecurity() {
				fmt.Fprintf(b, " [ON %s: visible to security]", strings.ToUpper(strings.ReplaceAll(it.Where, "_", " ")))
			}
			b.WriteString("\n")
		}
	}
	if len(groups) > 0 {
		gs := make([]*group, 0, len(groups))
		for _, g := range groups {
			gs = append(gs, g)
		}
		slices.SortFunc(gs, func(a, c *group) int {
			if a.est != c.est {
				return int(c.est - a.est)
			}
			return strings.Compare(a.name, c.name)
		})
		const maxGroups = 12
		b.WriteString("Clean stock:\n")
		for i, g := range gs {
			if i == maxGroups {
				var n int
				var e int64
				for _, r := range gs[i:] {
					n += r.n
					e += r.est
				}
				fmt.Fprintf(b, "- %d more items, est. %d\n", n, e)
				break
			}
			fmt.Fprintf(b, "- %s", g.name)
			if g.n > 1 {
				fmt.Fprintf(b, " x%d", g.n)
			}
			for i, u := range g.uids {
				if i == 4 {
					b.WriteString(" …")
					break
				}
				fmt.Fprintf(b, " #%d", u)
			}
			if g.estN > 0 {
				fmt.Fprintf(b, ", est. %d", g.est/int64(g.estN))
				if g.n > 1 {
					b.WriteString(" each")
				}
			}
			if g.paidN > 0 {
				fmt.Fprintf(b, ", paid %d", g.paid/int64(g.paidN))
				if g.n > 1 {
					b.WriteString(" avg")
				}
			}
			fmt.Fprintf(b, " (%s)\n", whereText(g.where))
		}
	}
}

func itemName(it item) string {
	return nonEmpty(it.Name, nonEmpty(it.ID, "item"))
}

func (s *snapshot) itemLine(it item) string {
	var parts []string
	name := itemName(it)
	if it.UID != nil {
		name += fmt.Sprintf(" #%d", *it.UID)
	}
	if t := it.itemType(); t != "" {
		name += " (" + t + ")"
	}
	parts = append(parts, name+" in "+s.whereLabel(it))
	if it.EstimatedValue != nil {
		parts = append(parts, fmt.Sprintf("est. %d", *it.EstimatedValue))
	}
	if it.BoughtAt != nil {
		parts = append(parts, fmt.Sprintf("paid %d", *it.BoughtAt))
	}
	return strings.Join(parts, ", ")
}

func featureText(it item) string {
	var fs []string
	for _, f := range it.Features {
		if f.Text != "" {
			fs = append(fs, f.Text)
		}
	}
	return strings.Join(fs, ", ")
}

func whereLabel(w string) string {
	switch w {
	case "":
		return "stock"
	case "hidden":
		return "the hidden stash (searched only when the poster is down)"
	case "trash":
		return "the trash (searched unless the inspector is lazy)"
	}
	return strings.ReplaceAll(w, "_", " ")
}

func contraText(it item) string {
	s := fmt.Sprintf("contraband level %d", *it.ContrabandLevel)
	if it.SeriousContraband != nil && *it.SeriousContraband {
		s += " SERIOUS"
	}
	return s
}

func whereText(m map[string]int) string {
	var parts []string
	for _, k := range sortedKeys(m) {
		if len(m) == 1 {
			return k
		}
		parts = append(parts, fmt.Sprintf("%d %s", m[k], k))
	}
	return strings.Join(parts, ", ")
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
