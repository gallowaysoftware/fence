package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/llm"
)

const (
	// MaxPlanBytes bounds a plan: it rides along in every customer prompt that day.
	MaxPlanBytes     = 2000
	maxLessonsBytes  = 1500
	defaultMaxTokens = 1500
)

// Headings are the plan's sections, in order. The overlay and the scorecard rely on them.
var Headings = []string{"CALENDAR", "GOALS", "CASH", "FACTIONS", "WINDOW", "STOCK", "KEEP", "PRODUCTION", "RISK", "CUSTOMERS"}

// DayPlan is one day's plan and how the day went against it.
type DayPlan struct {
	Day int `json:"day"`
	// Plan is "" when no plan was written for the day.
	Plan  string `json:"plan"`
	Model string `json:"model,omitempty"`
	// Scorecard is the evening review, "" until the day has ended.
	Scorecard string `json:"scorecard,omitempty"`
	// Facts is the day's numbers (cash, reputation, trades), from snapshots and outcomes.
	Facts string `json:"facts,omitempty"`
}

// PlanInput is everything the morning plan is written from. Every field must come from
// player-visible data.
type PlanInput struct {
	Day int
	// Snapshot is the morning day_snapshot event's data.
	Snapshot  json.RawMessage
	Yesterday *DayPlan
	// Lessons is memory's brief of earlier play, if any.
	Lessons string
}

// planSystem states only mechanics from docs/re/STATE.md, docs/re/BARTER.md and
// docs/research/SOLVER.md §6.
var planSystem = strings.TrimSpace(`
You are the morning strategist of a pawnshop in the cyberpunk game "Probably Stolen". Each in-game morning you write the plan for the day. An advisor reads your plan before every customer and serves it; tonight the day is scored goal by goal against it.

HOW THE RUN IS WON: buy the shop out (the state gives the buyout price; it is 12,000 at the start on normal difficulty, more on harder ones, and rises daily after day 50), or take the landlord's mortgage and pay every week. Lose: arrested, any faction except the Black Market at -100, or rent refused without landlord grace. There are no loans; the only debts are rent arrears and the mortgage. Profit comes from buying from sellers below the estimated value and reselling near the estimate plus the fixed retail markup, so cash above the floor is for stock, and an empty shop earns nothing. The run is won across days: which factions to push past a perk or keep off a penalty, what to sell, hold or buy, how much heat and criminal record to carry toward the next inspection.

MECHANICS
- Rent is due every 7 days and grows after every landlord visit. Unpaid rent becomes arrears only with landlord grace; without it, refusing ends the run.
- Reputation runs -200..200 per faction; the state gives each faction's next perk and next penalty with the distance. Lower Levels 20 Referral sends one more customer every day for the rest of the run. Penalties grow as rep falls. A faction within 20 of a run-ending -100 comes first: raise it with generous deals to its customers and risk no walk-out with them.
- A trade moves the customer's faction by about 1 point per 400 credits of value (Lower Levels x0.66, Upper Levels x1.2, buying stolen x1.5-3), so a day moves a faction a few points at most; a gap of more than about 5 is a multi-day goal. Generous deals (a discount when selling, a markup when buying) add rep at a cash cost; hard bargains cost rep; a walk-out costs 3. Black Market rep losses count double, so be fair with Black Market customers rather than avoid them. Tourist, Church and Middle customers have no bar. Customers arrive at random; you choose only how to treat them.
- The window (display case) must be set before the shop opens: the forecast is fixed at opening. At opening the top N item types by displayed value each bring one guaranteed buyer (N is in the state); a type that is not in the top N draws no one, so concentrate value in the types you want buyers for and keep matching stock in the back. A type's total displayed value sets the buyer's tier: tier 2 from 145, tier 3 from 350 (household 300) from day 14; richer tiers have bigger budgets. Displayed items have no price of their own; buyers pay the normal counter price. Special visitors from signs or certificates are rolls that use up regular customer slots. The discerning shopper needs a luxury item displayed. Never display stolen or contraband goods: contraband guarantees an inspection at every opening, and a stolen item triggers one at its heat percentage; that inspection searches the whole store and gives no Security rep. The calendar is the backbone of the plan: it shows the next two weeks of market events (a type's value up or down by a percentage), THREAT events, rent and mortgage days and cartel visits. Buy a type before its rise (in bulk when cash allows), sell into it, sell before a fall; before a THREAT, clear what it would hurt (hot stock, contraband, spare cash at risk) and keep cash in hand. STOCK, WINDOW, PRODUCTION and RISK name the calendar day they serve.
- Production (the state's PRODUCTION block has the shop's own numbers; use them): machines, plants and bottles advance only while you sleep, one step a night, and a night without sleep is simply lost. Wine ferments in the bottle: 749+ ml of water, berries and yeast; 2 nights with yeast (superyeast 1, none 4). Score = berries (best +2, normal +1, under- or over-ripe 0, moldy -1) +1 for two or more berries +1 for yeast + water (tested High Quality or Pure +1, Basewater or a ration 0, worse -1 to -3): 1 Bucket brew 85, 2 Homebrew 100, 3-4 Production 125, 5 Reserve 150, 6+ Master Vintage 175, 0 or less undrinkable. Bottles and yeast are the limit, not berries: with bottles to spare one berry per bottle, with more berries than bottles two good berries per bottle (the state gives the best fill). Every berry but a moldy one is worth 85+ in a bottle with yeast against 14 as food, so only moldy berries go to food unless no bottle or yeast can be had. Finished wine ages anywhere with no cap (+3 to +5.25 a night by grade) and no market event ever moves alcohol or luxury prices: hold it unless cash is needed. Furnace: one smelt a night, 2 scrap or ore -> 1 ingot (blast module 3 -> 2), 8 power; ore and flux each lift purity a level (2 scrap Low 42.5, with flux Fair 50, 2 ore + flux Very high 75): about 18 a night on scrap, about 93 with the blast module on ore. Ingots are MATERIAL. Scavenge the dumping ground every night it is open (one free outing after closing; a welder in the bag adds scrap, a flashlight halves wound odds); skip it only when wounded.
- Water: grades by purity: Pure, High Quality, Basewater, Ghostwater, Rustwater, Gutterflow. Buyers pay for the LABEL; bottles and plants get the TRUE grade. Filtering, purifying, tablets and heat change the water but not the label, and pouring into a non-empty container keeps that container's label, so test (strip, or AquaScan Match) and relabel before selling treated water: under-labelled water sells too cheap; over-labelled water is exposed by checking customers (-1 rep) and builds a hidden fraud or endangerment record. Never raise a label with the labeler. A filter in the pitcher plus a chlorine tablet a litre turns free rust tap water into Basewater (one filter ≈ 10.8 L ≈ +324); the purifier machine takes one container a night up to High Quality. Route water by grade: Basewater to selling, High Quality or Pure to the hydroponic bay and wine. A bloomberry plant on Basewater falls to health ~0 and gives mostly moldy berries; on High Quality plus nutrient tablets (added after filtering, minerals 200 ± 75 bp) about 78, on Pure 100; the plant analyser's growth score shows it.
- USE VALUE: an item is worth the better of its sale price and its use here. The state's USE VALUE lines come from a catalogue of the game's code: KEEP lines beat their sale price now; "no use in this shop now" items are safe to sell.
- Events: random market events start without warning; only an information dealer's tip (1-4 days ahead) or the Lower Levels rumour perk (2-3 days) shows one early, so the calendar is all you can plan on. Speculate with FOOD and MATERIAL stock, never with wine.
- Stolen goods: heat 0-100 falls 18 a night wherever the item is (2-8 less for contraband). At heat 0 the item is no longer stolen and sells clean, with no crime; selling it hot is a fencing crime that grows with its heat, and selling it does not clear any risk. Buying stolen is not a crime and earns extra rep. Inspections do not search a smuggler bay; a stolen item they find anywhere else goes back to heat 100. So keep hot items in the smuggler bay (or at least out of the window and counters) and sell each on the day the state says it sells clean; sell hot only when rent cannot be met any other way before then. A security officer who sees stolen or contraband goods on a counter or in the window sets heat to 100 and starts an inspection. Clean items are never at risk.
- Contraband: selling it is a trafficking crime. Some Lower Levels customers refuse it.
- Inspections search stock, counters, showcase and the trash (a Lazy inspector skips the trash); the hidden stash only when the poster is down. Stolen items may be found; high-level contraband found is fined (arrest if the fine cannot be paid), critical contraband means arrest. A Shady inspector takes a bribe if you have the cash. A clean inspection gives Security +3. Inspections come more often as the trafficking record grows; the early-warning service announces them in the night log.
- The security record: crime amounts fall 20 a day while evidence is below 80/100; at 100 evidence a sentencing officer comes. Projected sentence: fine below 300 total crime, prison below 1500, execution above.
- Security officers: clean trades with them are safe and raise Security rep. Never sell them stolen or contraband goods.

WRITE THE PLAN in plain text, at most 1700 characters, short lines, in exactly this layout (ten headings in this order, each on its own line, each followed by lines starting with "- "):
CALENDAR
- <the next 7 days from the state's calendar, one line per relevant day: "day N: <event> -> <what it means for us>" (a price rise to buy ahead of or sell into, a threat to prepare for, rent or mortgage day, a cartel visit); "nothing in 7 days" if empty>
GOALS
- <2 to 4 goals, one per line, each reachable today and checkable tonight from closing cash, reputation values, or which named items were sold, kept or bought; each with its target number>
CASH
- <the floor: the least cash to hold at any moment today, never above current cash; usually the rent due if it falls within 3 days, else lower. Free to spend = cash minus the floor.>
FACTIONS
- <which faction to push toward which threshold and how; which to protect, how far it is from its next penalty, and how>
WINDOW
- <what the window holds when the shop opens: items to put in and take out, by name and #uid; which types that makes the top N and the tier each reaches; nothing stolen or contraband>
STOCK
- <which items to sell today; for each stolen item: "hold <item #uid> in the smuggler bay until day N, then sell clean"; what to buy or not buy>
- SPECULATION: <for each price rise in the calendar: what to buy now (how much cash) and what you already hold to sell into it, on which days; "none" only if the calendar has no rise>
KEEP
- <from the USE VALUE lines: items and quantities to hold back for the shop's own use, by name and #uid, with "use ≈ N vs sale M" and the chain each feeds; mark a use the state does not price "(uncertain)"; then "- SAFE TO SELL: <items with no use here>">
PRODUCTION
- <one line per machine, plant, bottle batch and water container that needs a move, from the PRODUCTION block: "<thing #uid>: tonight <action>; tomorrow <action>" with the numbers (nights, grade, value, day it is ready); water: what to test or relabel before selling and which water feeds the bay and the bottles; what to buy for production (bottles, yeast, flux, filters); tonight's scavenging; "none" if the state shows no production>
RISK
- <hot items not in a smuggler bay (exposed to an inspection), contraband, the security record, and what to move or clear before each THREAT day; what not to sell or buy, and to whom>
CUSTOMERS
- <whom to prioritise, whom to treat generously for rep, whom to turn away>
Use the state's own numbers: name items, factions, amounts and days. No generic advice ("be careful", "maximize profit"). Do not invent facts or effects the state and these rules do not give. If yesterday missed a goal, say what changes today. No preamble, no markdown.`)

// BuildPlanPrompt renders in as the morning prompt: yesterday's scorecard first, then
// yesterday's plan, memory's lessons and today's state.
func BuildPlanPrompt(in PlanInput) (string, error) {
	s, err := parseSnapshot(in.Snapshot)
	if err != nil {
		return "", fmt.Errorf("day snapshot: %w", err)
	}
	day := s.day(in.Day)
	var b strings.Builder
	if y := in.Yesterday; y != nil {
		if y.Facts != "" {
			fmt.Fprintf(&b, "YESTERDAY (day %d): %s\n", y.Day, y.Facts)
		}
		if y.Scorecard != "" {
			fmt.Fprintf(&b, "YESTERDAY SCORED AGAINST ITS PLAN:\n%s\n", y.Scorecard)
		}
		if y.Facts != "" || y.Scorecard != "" {
			b.WriteByte('\n')
		}
		if y.Plan != "" {
			fmt.Fprintf(&b, "YESTERDAY'S PLAN (day %d):\n%s\n\n", y.Day, y.Plan)
		}
	}
	if l := strings.TrimSpace(in.Lessons); l != "" {
		fmt.Fprintf(&b, "FROM MEMORY OF PAST PLAY:\n%s\n\n", truncateLines(l, maxLessonsBytes))
	}
	fmt.Fprintf(&b, "TODAY: day %d, morning.\n", day)
	s.render(&b, day)
	fmt.Fprintf(&b, "\nWrite the plan for day %d.", day)
	return b.String(), nil
}

// DraftPlan asks c for the day's plan and returns it tidied to the plan headings within
// MaxPlanBytes. A plan missing a heading gets one repair round.
func DraftPlan(ctx context.Context, c llm.Client, in PlanInput, maxTokens int) (plan, model string, err error) {
	prompt, err := BuildPlanPrompt(in)
	if err != nil {
		return "", "", err
	}
	snap, _ := parseSnapshot(in.Snapshot)
	day := snap.day(in.Day)
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	msgs := []llm.Message{{Role: llm.RoleUser, Content: prompt}}
	for attempt := 0; attempt < 2; attempt++ {
		resp, cerr := c.Complete(ctx, llm.Request{System: planSystem, Messages: msgs, MaxTokens: maxTokens})
		if cerr != nil {
			return "", "", cerr
		}
		plan, err = TidyPlan(resp.Text)
		if err == nil && attempt == 0 {
			err = errors.Join(goalsCheck(plan), lintPlan(plan, snap, day))
		}
		if err == nil {
			return plan, resp.Model, nil
		}
		msgs = append(msgs,
			llm.Message{Role: llm.RoleAssistant, Content: nonEmpty(strings.TrimSpace(resp.Text), "(empty)")},
			llm.Message{Role: llm.RoleUser, Content: "That plan is unusable: " + err.Error() +
				". Rewrite it with the seven headings " + strings.Join(Headings, ", ") + ", each on its own line followed by lines starting with \"- \"."},
		)
	}
	return "", "", err
}

// lintPlan catches the plan errors a small model keeps making against rules it was given:
// selling a hot stolen item early when rent does not force it, and selling stolen or
// contraband goods to security. Each finding goes back to the model in the repair round.
func lintPlan(plan string, s *snapshot, day int) error {
	due, rentKnown := s.Rent.due()
	forced := rentKnown && s.Cash != nil && *s.Cash < due
	var errs []error
	for _, it := range s.Inventory {
		if !it.stolen() && !it.contraband() {
			continue
		}
		refs := []string{strings.ToLower(itemName(it))}
		if it.UID != nil {
			refs = append(refs, fmt.Sprintf("#%d", *it.UID))
		}
		for _, line := range strings.Split(strings.ToLower(plan), "\n") {
			if !mentionsAny(line, refs) || !strings.Contains(line, "sell") || mentionsAny(line, []string{"do not sell", "don't sell", "never sell", "not sell", "hold", "until", "keep"}) {
				continue
			}
			if strings.Contains(line, "security") {
				errs = append(errs, fmt.Errorf("%q plans to sell %s to security, which is never allowed", strings.TrimPrefix(line, "- "), itemName(it)))
				continue
			}
			if it.stolen() && !forced {
				if d := it.coolDays(); d > 0 {
					errs = append(errs, fmt.Errorf("%q sells stolen %s (heat %d) before it cools on day %d; rent does not force it, so hold it in back stock or the hidden stash and sell it clean then",
						strings.TrimPrefix(line, "- "), itemName(it), *it.StolenHeat, day+int(d)))
				}
			}
		}
	}
	errs = append(errs, lintProduction(plan, s)...)
	return errors.Join(errs...)
}

var negations = []string{"do not sell", "don't sell", "never sell", "not sell", "hold", "until", "keep"}

// lintProduction catches a plan that sells what the catalogue says to keep, or that
// ignores a production fault the state names.
func lintProduction(plan string, s *snapshot) []error {
	ps := s.production()
	keep := s.keepUIDs(ps)
	// A name identifies a KEEP group only when no other stock item shares it.
	names := map[string]*useGroup{}
	shared := map[string]bool{}
	for _, it := range s.Inventory {
		n := strings.ToLower(itemName(it))
		if it.UID != nil && keep[*it.UID] != nil {
			if g := names[n]; g != nil && g != keep[*it.UID] {
				shared[n] = true
			}
			names[n] = keep[*it.UID]
		}
	}
	for _, it := range s.Inventory {
		n := strings.ToLower(itemName(it))
		if (it.UID == nil || keep[*it.UID] == nil) && !s.inMachine(it) {
			shared[n] = true
		}
	}
	var errs []error
	type finding struct {
		g       *useGroup
		display bool
	}
	reported := map[finding]bool{}
	lower := strings.ToLower(plan)
	section := ""
	for _, line := range strings.Split(lower, "\n") {
		if h, _, ok := headingOf(line); ok {
			section = h
			continue
		}
		display := section == "WINDOW" && mentionsAny(line, []string{"display", "add", "show", "put"}) && !mentionsAny(line, []string{"remove", "take out", "pull"})
		if !display && (!strings.Contains(line, "sell") || mentionsAny(line, negations)) {
			continue
		}
		var hit *useGroup
		for _, m := range uidRE.FindAllStringSubmatch(line, -1) {
			var u int64
			fmt.Sscan(m[1], &u)
			if g := keep[u]; g != nil {
				hit = g
			}
		}
		for n, g := range names {
			if hit == nil && !shared[n] && strings.Contains(line, n) {
				hit = g
			}
		}
		if hit != nil && !reported[finding{hit, display}] {
			reported[finding{hit, display}] = true
			sale, _ := hit.saleEach()
			switch {
			case display:
				errs = append(errs, fmt.Errorf("%q puts %s in the window, which invites buyers for an item the shop should keep (use %s); display other stock", strings.TrimPrefix(line, "- "), hit.label(), hit.e.valueText(hit.grade)))
				continue
			case hit.e == cleanWaterEntry:
				errs = append(errs, fmt.Errorf("%q sells %s, tested clean water that the hydroponic bay and the wine bottles need (Basewater can be sold instead); keep it for them", strings.TrimPrefix(line, "- "), hit.label()))
				continue
			}
			errs = append(errs, fmt.Errorf("%q sells %s, whose use here (%s, %s) beats its sale (about %d); keep it, or say why this price beats its use", strings.TrimPrefix(line, "- "), hit.label(), hit.e.valueText(hit.grade), hit.e.Chain, sale))
		}
	}
	if bad := ps.badBayWater(); len(bad) > 0 {
		fixed := false
		for _, line := range strings.Split(lower, "\n") {
			if mentionsAny(line, []string{"bay", "hydroponic"}) && mentionsAny(line, []string{"high quality", "hq", "pure", "purifier"}) {
				fixed = true
			}
		}
		if !fixed {
			errs = append(errs, fmt.Errorf("%s, so its berries rot; PRODUCTION must move High Quality or Pure water (the purifier's output) into the bay and sell the Basewater", strings.Join(bad, "; ")))
		}
	}
	if ps.has["scavenge"] && !(ps.scav != nil && ps.scav.WoundedFresh != nil && *ps.scav.WoundedFresh) {
		for _, line := range strings.Split(lower, "\n") {
			if strings.Contains(line, "scaveng") && mentionsAny(line, []string{"skip", "no scaveng", "don't scaveng", "do not scaveng", "avoid"}) {
				errs = append(errs, errors.New("the dumping ground is open and the shop is not wounded: scavenge tonight (a free outing; the state gives the wound odds) rather than skip it"))
				break
			}
		}
	}
	if ps.wornFilters() && !mentionsAny(lower, []string{"swap", "replace", "fit ", "change the filter", "install", "new filter", "fresh filter"}) {
		errs = append(errs, errors.New("the pitcher's filter is worn out; PRODUCTION must swap a stock filter into it before filtering more water"))
	}
	return errs
}

var uidRE = regexp.MustCompile(`#(\d+)`)

func mentionsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// goalsCheck asks for a repair when the plan has one goal; a second try that still has one
// is accepted rather than lose the plan.
func goalsCheck(plan string) error {
	if n := len(Goals(plan)); n < 2 {
		return fmt.Errorf("GOALS has %d line(s); it needs 2 to 4, one per line", n)
	}
	return nil
}

// Goals returns the plan's GOALS lines.
func Goals(plan string) []string {
	var out []string
	in := false
	for _, l := range strings.Split(plan, "\n") {
		switch {
		case l == "GOALS":
			in = true
		case slices.Contains(Headings, l):
			in = false
		case in && strings.HasPrefix(l, "- "):
			out = append(out, l)
		}
	}
	return out
}

// TidyPlan normalises a model's plan: markdown and preamble dropped, the headings in
// order, bullets as "- ", then cut to MaxPlanBytes by dropping the last lines of the
// longest sections. It fails when a heading is missing or empty.
func TidyPlan(text string) (string, error) {
	text = llm.StripThinking(text)
	sections := map[string][]string{}
	cur := ""
	for _, line := range strings.Split(text, "\n") {
		line = cleanLine(line)
		if line == "" || strings.HasPrefix(line, "```") {
			continue
		}
		if h, rest, ok := headingOf(line); ok {
			cur = h
			if rest != "" {
				sections[cur] = append(sections[cur], bullet(rest))
			}
			continue
		}
		if cur != "" {
			sections[cur] = append(sections[cur], bullet(line))
		}
	}
	var missing []string
	for _, h := range Headings {
		if len(sections[h]) == 0 {
			missing = append(missing, h)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing or empty: %s", strings.Join(missing, ", "))
	}
	fit(sections, MaxPlanBytes)
	var b strings.Builder
	for i, h := range Headings {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(h)
		for _, l := range sections[h] {
			b.WriteString("\n" + l)
		}
	}
	return b.String(), nil
}

func cleanLine(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.TrimSpace(s)
	return strings.TrimSpace(strings.TrimLeft(s, "#"))
}

// headingOf recognises "GOALS", "Goals:", "GOALS (today): text" and the like. A word
// followed by prose without a colon ("Cash is low") is a body line, not a heading.
func headingOf(line string) (h, rest string, ok bool) {
	for _, h := range Headings {
		if len(line) < len(h) || !strings.EqualFold(line[:len(h)], h) {
			continue
		}
		tail := line[len(h):]
		if tail != "" && !strings.ContainsRune(" :(", rune(tail[0])) {
			continue
		}
		tail = strings.TrimSpace(tail)
		if strings.HasPrefix(tail, "(") {
			if i := strings.IndexByte(tail, ')'); i > 0 {
				tail = strings.TrimSpace(tail[i+1:])
			}
		}
		switch {
		case tail == "":
			return h, "", true
		case strings.HasPrefix(tail, ":"):
			return h, strings.TrimSpace(tail[1:]), true
		}
		return "", "", false
	}
	return "", "", false
}

func bullet(s string) string {
	s = strings.TrimSpace(s)
	for _, p := range []string{"- ", "* ", "• ", "– ", "— "} {
		if strings.HasPrefix(s, p) {
			return "- " + strings.TrimSpace(s[len(p):])
		}
	}
	if i := strings.IndexAny(s, ".)"); i > 0 && i <= 2 && strings.Trim(s[:i], "0123456789") == "" && len(s) > i+1 {
		return "- " + strings.TrimSpace(s[i+1:])
	}
	return "- " + s
}

func planLen(sections map[string][]string) int {
	n := 0
	for _, h := range Headings {
		n += len(h) + 1
		for _, l := range sections[h] {
			n += len(l) + 1
		}
	}
	return n - 1
}

func fit(sections map[string][]string, maxBytes int) {
	for planLen(sections) > maxBytes {
		longest := ""
		for _, h := range Headings {
			if len(sections[h]) > 1 && (longest == "" || len(sections[h]) > len(sections[longest])) {
				longest = h
			}
		}
		if longest != "" {
			sections[longest] = sections[longest][:len(sections[longest])-1]
			continue
		}
		// One line per section and still too long: shorten the longest line.
		var lh string
		for _, h := range Headings {
			if lh == "" || len(sections[h][0]) > len(sections[lh][0]) {
				lh = h
			}
		}
		over := planLen(sections) - maxBytes
		sections[lh][0] = clip(sections[lh][0], max(len(sections[lh][0])-over, 8))
	}
}

// clip cuts s to at most n bytes, never inside a UTF-8 sequence, ending in "…".
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimRight(s[:cut], " ") + "…"
}

// truncateLines cuts s to at most n bytes at a line end when there is one.
func truncateLines(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if i := strings.LastIndexByte(s[:n], '\n'); i > 0 {
		return s[:i]
	}
	return clip(s, n)
}
