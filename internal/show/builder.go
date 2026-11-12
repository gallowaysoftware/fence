package show

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/bet"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Builder turns bridge decision-log lines (internal/decisionlog) into show events, keeping
// what it needs between them: the shop's clock, item and upgrade names, the customers at
// the counter. It decodes only player-visible fields. Not safe for concurrent use.
type Builder struct {
	Player Player
	// Plans, when set, gives a day's plan at its morning (replay); live producers call Plan.
	Plans func(day int) *PlanInput
	// Why, when set, gives the executor's reason for a customer (gym summaries carry it).
	Why func(day int, clientSeq int64) string
	// Base is the directives the executor plays over its defaults (the bridge's
	// -base-directives): the standing plan a delta planner changes.
	Base *protocol.Directives

	run     string
	day     int
	started bool
	ended   bool

	clock     Clock
	haveClock bool
	heat      *Heat

	names    map[int64]string
	cost     map[int64]int64
	upgrades map[string]string
	clients  map[int64]client
	counters map[string]counter

	morningCash, morningNW int64
	// mornings is the last day whose morning was published; the day's waking_up event has
	// already moved b.day by then.
	mornings   int
	morningRep map[string]float64
	closeRep   map[string]float64
	today      DaySummary

	planDirectives []string
	planDay        int
	// goal is the last prose plan's first headline line and its day.
	goal        string
	goalDay     int
	lastThought string

	appraisalRank int
	mortgageWeeks int

	// The small beats (smallbeats.go): the day's counter stories, the night out, the plan's
	// first telling each day, the buyout milestone reached and the appraisal streak.
	counterBeats, counterAt int
	lastHaggle              *haggle
	night                   *night
	closeItems              map[int64]bool
	morningPlanDay          int
	milestone               int64
	streak                  int
	streakFees              int64
	augDetection            string
	handedOver              bool

	// bets is the strategist's bet, checked against what the log shows; speakers are the
	// names that have held a dialogue with the shop (ledger.go).
	bets     bet.Tracker
	speakers map[string]bool
	// headline is the latest prose plan's headline and its day, for the night's diary.
	headline    []string
	headlineDay int
	ghost       ghostRun
}

type client struct{ name, faction, intent string }

type counter struct {
	client client
	seq    int64
	items  map[int64]itemView
	whole  bool
}

func NewBuilder(p Player) *Builder {
	return &Builder{
		Player:   p,
		names:    map[int64]string{},
		cost:     map[int64]int64{},
		upgrades: map[string]string{},
		clients:  map[int64]client{},
		counters: map[string]counter{},
		speakers: map[string]bool{},
	}
}

// Day is the game day the builder has reached.
func (b *Builder) Day() int { return b.day }

// Clock is the shop's latest clock, nil before the first snapshot.
func (b *Builder) Clock() *Clock {
	if !b.haveClock {
		return nil
	}
	c := b.clock
	return &c
}

// The decision-log shapes, cut to the visible fields the show uses. None of them has a
// Hidden field, so `hidden` is skipped by the decoder and never reaches an event.
type logLine struct {
	Type       string       `json:"type"`
	TS         string       `json:"ts"`
	DecisionID string       `json:"decision_id"`
	Event      *logEvent    `json:"event"`
	Request    *logRequest  `json:"request"`
	Response   *logResponse `json:"response"`
	Outcome    *logOutcome  `json:"outcome"`
	// Show is a feed event a producer wrote into the log itself: fence-rollout's hybrid
	// decisions (sim/rollout/ShowLog.cs).
	Show *Event `json:"show"`
}

type logEvent struct {
	Kind    string          `json:"kind"`
	GameDay int             `json:"game_day"`
	Data    json.RawMessage `json:"data"`
}

type logRequest struct {
	Kind    string `json:"kind"`
	GameDay int    `json:"game_day"`
	Context struct {
		Client  *clientView `json:"client"`
		Counter struct {
			Theirs []itemView `json:"theirs"`
			Yours  []itemView `json:"yours_staged"`
		} `json:"counter"`
	} `json:"context"`
}

type logResponse struct {
	Rationale string `json:"rationale"`
	Source    string `json:"source"`
	Choice    struct {
		Option string `json:"option"`
	} `json:"choice"`
}

type logOutcome struct {
	GameDay int `json:"game_day"`
	Result  struct {
		Kind      string             `json:"kind"`
		Side      string             `json:"side"`
		Items     []int64            `json:"items"`
		CashDelta int64              `json:"cash_delta"`
		Option    string             `json:"option"`
		Accepted  *bool              `json:"accepted"`
		Walked    bool               `json:"walked_out"`
		RepDelta  map[string]float64 `json:"rep_delta"`
	} `json:"result"`
}

type clientView struct {
	Name      string          `json:"name"`
	Faction   string          `json:"faction"`
	Intent    string          `json:"intent"`
	Wholesale json.RawMessage `json:"wholesale"`
}

type itemView struct {
	UID      int64  `json:"uid"`
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Estimate int64  `json:"estimated_value"`
	BoughtAt *int64 `json:"bought_at"`
	Heat     *int64 `json:"stolen_heat"`
	Where    string `json:"where"`
}

// Feed takes one decision-log line and returns the events it makes, in order.
func (b *Builder) Feed(line []byte) ([]Event, error) {
	var l logLine
	if err := json.Unmarshal(line, &l); err != nil {
		return nil, err
	}
	var out []Event
	switch l.Type {
	case "event":
		if l.Event != nil {
			out = b.event(l.Event)
		}
	case "decision":
		if l.Request != nil {
			out = b.decision(l.DecisionID, l.Request, l.Response)
		}
	case "outcome":
		if l.Outcome != nil {
			out = b.outcome(l.DecisionID, l.Outcome)
		}
	case "show":
		if l.Show != nil && l.Show.Kind != "" && l.Show.Text != "" {
			out = []Event{*l.Show}
			if p, ok := b.hybridPlan(*l.Show); ok {
				out = append(out, p)
			}
		}
	}
	for i := range out {
		out[i].V = SchemaVersion
		out[i].TS = l.TS
		if out[i].Run == "" {
			out[i].Run = b.run
		}
		if out[i].Day == 0 {
			out[i].Day = b.day
		}
	}
	return out, nil
}

func (b *Builder) event(e *logEvent) []Event {
	if e.Kind == "run_start" {
		b.runRules(e.Data)
	}
	if e.GameDay > 0 && e.GameDay > b.day && e.Kind != "day_snapshot" {
		b.day = e.GameDay
	}
	switch e.Kind {
	case "sim_run":
		var d struct {
			Seed       int64    `json:"seed"`
			Difficulty string   `json:"difficulty"`
			Start      string   `json:"start"`
			Perks      []string `json:"perks"`
		}
		if json.Unmarshal(e.Data, &d) == nil {
			b.fill(d.Difficulty, d.Start, d.Perks)
			if b.Player.Seed == 0 {
				b.Player.Seed = d.Seed
			}
		}
	case "run_start":
		var d struct {
			RunID      string   `json:"run_id"`
			Difficulty string   `json:"difficulty"`
			Start      string   `json:"start_type"`
			Perks      perkList `json:"perks"`
		}
		if json.Unmarshal(e.Data, &d) != nil {
			return nil
		}
		run := d.RunID
		if b.Player.ID != "" {
			run = b.Player.ID + ":" + d.RunID
		}
		if b.started && run == b.run {
			return nil
		}
		// The real game starts the next run in the same session after a game over.
		if b.started {
			b.Player.Difficulty, b.Player.Start, b.Player.Perks = "", "", nil
			b.ended, b.heat, b.planDay, b.planDirectives, b.day, b.mornings = false, nil, 0, nil, 0, 0
			b.bets = bet.Tracker{}
			b.ghost = ghostRun{}
		}
		b.fill(d.Difficulty, d.Start, d.Perks)
		b.Player.Book = BookFor(b.Player.Route, b.Player.Difficulty, b.Player.Start)
		// Every book is a Board run; a Board-free run would read "behind" all game.
		if f := b.Player.Format; f != nil && slices.Contains(f.Handicaps, "no_board") {
			b.Player.Book = nil
		}
		b.run, b.started = run, true
		p := b.Player
		return []Event{{Kind: KindRunStart, Level: LevelMoment, Day: max(e.GameDay, 1), Player: &p, Text: runStartText(p)}}
	case "day_snapshot":
		return b.snapshot(e)
	case "customer_observed":
		return b.customer(e)
	case "human_action":
		return b.action(e)
	case "money":
		return append(b.money(e), b.moneyBets(e)...)
	case "aug":
		return b.aug(e)
	case "autopilot":
		return b.autopilot(e)
	case "autopilot_fault":
		return b.autopilotFault(e)
	case "appraisal":
		return b.appraisal(e)
	case "run_end":
		var d struct {
			Ending string `json:"ending"`
			Day    int    `json:"day"`
			Cash   int64  `json:"cash"`
		}
		if json.Unmarshal(e.Data, &d) != nil || b.ended {
			return nil
		}
		b.ended = true
		end := &End{Ending: d.Ending, Day: d.Day, Cash: d.Cash, NetWorth: b.clock.NetWorth, Won: Won(d.Ending)}
		ev := Event{Kind: KindRunEnd, Level: LevelMoment, Day: d.Day, End: end, Clock: b.Clock(), Text: endText(end)}
		if end.Won {
			ev.Beat = BeatBuyout
		}
		return []Event{ev}
	}
	return nil
}

// perkList reads run_start's perks: the mod sends {"id", "name"} objects (GameDay.Run),
// sim_run plain ids.
type perkList []string

func (p *perkList) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	for _, r := range raw {
		var id string
		if json.Unmarshal(r, &id) != nil {
			var o struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(r, &o); err != nil {
				return err
			}
			id = o.ID
		}
		if id != "" {
			*p = append(*p, id)
		}
	}
	return nil
}

func (b *Builder) fill(difficulty, start string, perks []string) {
	if b.Player.Difficulty == "" {
		b.Player.Difficulty = difficulty
	}
	if b.Player.Start == "" {
		b.Player.Start = start
	}
	if len(b.Player.Perks) == 0 && len(perks) > 0 {
		b.Player.Perks = perks
	}
}

type snapshotView struct {
	Phase string `json:"phase"`
	Store struct {
		Cash     *int64 `json:"cash"`
		Day      int    `json:"day"`
		Weekday  int    `json:"weekday"`
		NetWorth *int64 `json:"net_worth"`
	} `json:"store"`
	Rent *struct {
		Due      int64 `json:"due"`
		Arrears  int64 `json:"arrears"`
		DaysLeft int   `json:"days_left"`
		Grace    bool  `json:"landlord_grace"`
	} `json:"rent"`
	Property *struct {
		Owned    bool  `json:"owned"`
		Buyout   int64 `json:"buyout_price"`
		Mortgage *struct {
			Weekly    int64 `json:"weekly"`
			WeeksLeft int   `json:"weeks_left"`
		} `json:"mortgage"`
		Offer *struct {
			Down   int64 `json:"down_payment"`
			Weekly int64 `json:"weekly"`
		} `json:"mortgage_offer"`
	} `json:"property"`
	Reputation map[string]struct {
		Value float64 `json:"value"`
	} `json:"reputation"`
	Inventory []itemView `json:"inventory"`
	Security  *struct {
		Visible bool `json:"visible"`
		Crimes  []struct {
			ID     string `json:"id"`
			Amount int64  `json:"amount"`
		} `json:"crimes"`
		Evidence  int64  `json:"evidence"`
		Level     string `json:"evidence_level"`
		Sentence  string `json:"projected_sentence"`
		Officer   string `json:"case_officer"`
		Operation string `json:"operation"`
	} `json:"security"`
	Upgrades *struct {
		Owned     []struct{ ID, Name string } `json:"owned"`
		Available []struct{ ID, Name string } `json:"available"`
	} `json:"upgrades"`
}

func (b *Builder) snapshot(e *logEvent) []Event {
	var s snapshotView
	if json.Unmarshal(e.Data, &s) != nil {
		return nil
	}
	if s.Store.Cash != nil {
		b.clock.Cash = *s.Store.Cash
		b.haveClock = true
	}
	if s.Store.NetWorth != nil {
		b.clock.NetWorth = *s.Store.NetWorth
	}
	if s.Store.Weekday > 0 {
		b.clock.Weekday = s.Store.Weekday
	}
	if r := s.Rent; r != nil {
		b.clock.RentDue, b.clock.Arrears, b.clock.RentDays, b.clock.Grace = r.Due, r.Arrears, r.DaysLeft, r.Grace
	}
	if p := s.Property; p != nil {
		b.clock.Owned, b.clock.BuyoutPrice = p.Owned, p.Buyout
		b.clock.MortgageWeekly, b.clock.MortgageWeeksLeft, b.clock.OfferDown, b.clock.OfferWeekly = 0, 0, 0, 0
		if m := p.Mortgage; m != nil {
			b.clock.MortgageWeekly, b.clock.MortgageWeeksLeft = m.Weekly, m.WeeksLeft
		}
		// The game keeps the loan shark's offer on a bought shop.
		if o := p.Offer; o != nil && !p.Owned {
			b.clock.OfferDown, b.clock.OfferWeekly = o.Down, o.Weekly
		}
	}
	hot := 0
	for _, it := range s.Inventory {
		b.learn(it)
		if it.Heat != nil && *it.Heat > 0 {
			hot++
		}
	}
	if u := s.Upgrades; u != nil {
		for _, x := range append(u.Owned, u.Available...) {
			b.upgrades[x.ID] = x.Name
		}
	}
	rep := map[string]float64{}
	for k, v := range s.Reputation {
		rep[k] = v.Value
	}

	out := betEvents(b.bets.Snapshot(e.Data, max(s.Store.Day, e.GameDay)), b.Clock())
	if s.Phase == "morning" || (s.Phase == "run_start" && b.day == 0) {
		day := max(s.Store.Day, e.GameDay, 1)
		if day > b.mornings && s.Phase == "morning" && b.haveClock {
			b.mornings = day
			// The morning's plan can take a minute; the panel shows the new day meanwhile.
			out = append(out, Event{Kind: KindThought, Level: LevelTicker, Day: day, Clock: b.Clock(),
				Text: fmt.Sprintf("Day %d begins with %s in the till.", day, num(b.clock.Cash))})
			out = append(out, b.ghostMorning(day)...)
		}
		b.day = day
		b.morningCash, b.morningNW, b.morningRep = b.clock.Cash, b.clock.NetWorth, rep
		b.closeRep = nil
		b.today = DaySummary{}
		b.counterBeats, b.counterAt, b.lastHaggle = 0, 0, nil
		if b.Plans != nil {
			if p := b.Plans(day); p != nil {
				out = append(out, b.Plan(*p))
				out = append(out, b.Bets(*p)...)
			}
		}
	}
	if s.Phase == "close" {
		b.closeRep = rep
		b.closeItems = map[int64]bool{}
		for _, it := range s.Inventory {
			b.closeItems[it.UID] = true
		}
	}
	if s.Phase == "sleep" {
		out = append(out, b.nightBeat(s.Inventory)...)
	}
	out = append(out, b.milestoneBeat()...)
	if s.Security != nil && s.Inventory != nil {
		h := &Heat{Visible: s.Security.Visible, HotItems: hot}
		if h.Visible {
			h.Evidence, h.Level, h.Sentence, h.Officer, h.Operation = s.Security.Evidence, s.Security.Level, s.Security.Sentence, s.Security.Officer, s.Security.Operation
			for _, c := range s.Security.Crimes {
				if h.Crimes == nil {
					h.Crimes = map[string]int64{}
				}
				h.Crimes[strings.ToLower(c.ID)] = c.Amount
			}
		}
		if ev, ok := b.heatChange(h); ok {
			out = append(out, ev)
		}
	}
	return out
}

func (b *Builder) learn(it itemView) {
	if it.UID == 0 {
		return
	}
	if it.Name != "" {
		b.names[it.UID] = it.Name
	}
	if it.BoughtAt != nil {
		b.cost[it.UID] = *it.BoughtAt
	}
}

func (b *Builder) itemName(uid int64) string { return b.names[uid] }

// heatChange compares the record with the last one shown. Evidence moving is notable; a
// new operation, officer or sentence, or evidence crossing a tens mark upward, is a moment.
func (b *Builder) heatChange(h *Heat) (Event, bool) {
	prev := b.heat
	b.heat = h
	ev := Event{Kind: KindHeat, Heat: h, Text: heatText(h)}
	switch {
	case prev == nil:
		ev.Level = LevelTicker
		return ev, true
	case h.Visible != prev.Visible:
		ev.Level = LevelNotable
	case h.Operation != prev.Operation || h.Sentence != prev.Sentence || h.Officer != prev.Officer:
		ev.Level = LevelMoment
	case h.Evidence/10 > prev.Evidence/10:
		ev.Level = LevelMoment
	case h.Evidence != prev.Evidence:
		ev.Level = LevelNotable
	case (h.HotItems > 0) != (prev.HotItems > 0):
		ev.Level = LevelTicker
	default:
		return Event{}, false
	}
	return ev, true
}

func heatText(h *Heat) string {
	if !h.Visible {
		if h.HotItems > 0 {
			return fmt.Sprintf("No security record yet; %d stolen items on hand.", h.HotItems)
		}
		return "No security record yet."
	}
	var parts []string
	if h.Level != "" {
		parts = append(parts, "evidence "+h.Level)
	} else {
		parts = append(parts, fmt.Sprintf("evidence %d", h.Evidence))
	}
	if h.Operation != "" {
		op := strings.ToLower(h.Operation)
		if h.Officer != "" {
			op += " by " + h.Officer
		}
		parts = append(parts, op)
	}
	if h.Sentence != "" {
		parts = append(parts, "projected sentence: "+strings.ToLower(h.Sentence))
	}
	if h.HotItems > 0 {
		parts = append(parts, fmt.Sprintf("%d stolen items on hand", h.HotItems))
	}
	return "Security record: " + strings.Join(parts, ", ") + "."
}

func (b *Builder) customer(e *logEvent) []Event {
	var d struct {
		ClientSeq int64 `json:"client_seq"`
		Context   struct {
			Client *clientView `json:"client"`
		} `json:"context"`
	}
	if json.Unmarshal(e.Data, &d) != nil || d.Context.Client == nil {
		return nil
	}
	c := client{name: d.Context.Client.Name, faction: d.Context.Client.Faction, intent: d.Context.Client.Intent}
	b.clients[d.ClientSeq] = c
	if c.intent == "dialogue" {
		b.speakers[c.name] = true
		return nil
	}
	b.today.Customers++
	text := fmt.Sprintf("%s%s wants to %s.", c.name, factionTag(c.faction), intentWords(c.intent))
	if b.Why != nil {
		if w := b.Why(b.day, d.ClientSeq); w != "" {
			text = fmt.Sprintf("%s%s, %s: %s.", c.name, factionTag(c.faction), intentWords(c.intent), strings.TrimSuffix(w, "."))
		}
	}
	return []Event{{Kind: KindThought, Level: LevelTicker, Text: text}}
}

func factionTag(f string) string {
	switch f {
	case "", "none":
		return ""
	}
	return " (" + strings.ReplaceAll(f, "_", " ") + ")"
}

func intentWords(intent string) string {
	switch intent {
	case "buy":
		return "buy"
	case "sell":
		return "sell"
	case "sell_and_buy":
		return "trade"
	}
	return strings.ReplaceAll(intent, "_", " ")
}

func (b *Builder) decision(id string, r *logRequest, resp *logResponse) []Event {
	c := counter{items: map[int64]itemView{}}
	if cl := r.Context.Client; cl != nil {
		c.client = client{name: cl.Name, faction: cl.Faction, intent: cl.Intent}
		c.whole = len(cl.Wholesale) > 0 && string(cl.Wholesale) != "null"
	}
	for _, it := range append(r.Context.Counter.Theirs, r.Context.Counter.Yours...) {
		c.items[it.UID] = it
		b.learn(it)
	}
	b.counters[id] = c
	if len(b.counters) > 512 {
		b.counters = map[string]counter{id: c}
	}
	// Only a model's own reasoning is worth the ticker; the fallback's is boilerplate.
	if resp == nil || resp.Rationale == "" || resp.Source == "" || resp.Source == "fallback" {
		return nil
	}
	t := strings.TrimSpace(resp.Rationale)
	if t == b.lastThought {
		return nil
	}
	b.lastThought = t
	who := c.client.name
	if who == "" {
		who = r.Kind
	}
	return []Event{{Kind: KindThought, Level: LevelTicker, Text: fmt.Sprintf("%s: %s", who, clip(t, 200))}}
}

func (b *Builder) outcome(id string, o *logOutcome) []Event {
	res := o.Result
	c := b.counters[id]
	switch res.Kind {
	case "trade":
		return b.trade(c, res.Side, res.Items, res.CashDelta)
	case "bargain":
		if res.Accepted == nil {
			return nil
		}
		verdict := "refused"
		if *res.Accepted {
			verdict = "accepted"
		}
		if res.Walked {
			verdict = "walked out"
		}
		who := c.client.name
		if who == "" {
			who = "The customer"
		}
		b.haggled(c.client.name, optionWords(res.Option), verdict)
		return []Event{{Kind: KindThought, Level: LevelTicker, Who: b.character(c.client.name), Text: fmt.Sprintf("Haggled with %s (%s): %s.", who, optionWords(res.Option), verdict)}}
	}
	return nil
}

// isFake is an item whose shown name marks it as counterfeit: the shop has seen through it.
func isFake(name string) bool {
	l := strings.ToLower(name)
	return strings.Contains(l, "counterfeit") || strings.Contains(l, "(fake")
}

func optionWords(opt string) string {
	return strings.ReplaceAll(opt, "_", " ") + "%"
}

func (b *Builder) trade(c counter, side string, uids []int64, cash int64) []Event {
	t := &Trade{Side: side, Count: len(uids), Cash: abs(cash), Customer: c.client.name, Faction: c.client.faction}
	var names []string
	costKnown := len(uids) > 0
	for _, u := range uids {
		it, ok := c.items[u]
		n := it.Name
		if n == "" {
			n = b.names[u]
		}
		if n == "" {
			n = "item"
		}
		names = append(names, n)
		if ok {
			t.Estimate += it.Estimate
		}
		if cost, ok := b.cost[u]; ok && side == "sold" {
			t.Cost += cost
		} else {
			costKnown = false
		}
	}
	for _, n := range names {
		if isFake(n) {
			t.Fakes++
		}
	}
	t.Items = groupNames(names)
	b.clock.Cash += cash
	if side == "sold" {
		b.bets.Sold(uids, abs(cash))
	}
	switch side {
	case "sold":
		b.today.Sold += t.Cash
		b.today.SoldItems += t.Count
		if costKnown {
			t.Profit, t.HasProfit = t.Cash-t.Cost, true
		}
	case "bought":
		b.today.Bought += t.Cash
		b.today.BoughtItems += t.Count
		t.Wholesale = c.whole && t.Count > 1
		if t.Estimate > 0 {
			t.Profit, t.HasProfit = t.Estimate-t.Cash, true
		}
		for _, u := range uids {
			if it, ok := c.items[u]; ok {
				b.cost[u] = it.Price
			}
		}
	}
	ev := Event{Kind: KindTrade, Level: tradeLevel(t, b.clock.NetWorth), Trade: t, Clock: b.Clock(), Text: tradeText(t), Who: b.character(t.Customer)}
	b.counterBeat(&ev)
	return []Event{ev}
}

// tradeLevel: a sale or purchase the size of a week's rent early on is a moment; small
// change stays in the ticker. The bar rises with net worth, so a rich shop's routine 600 sale
// is not narrated every day.
func tradeLevel(t *Trade, netWorth int64) int {
	big, gain := max(500, netWorth/20), max(300, netWorth/40)
	switch {
	case t.Fakes > 0 && t.Side == "sold":
		return LevelMoment
	case t.Fakes > 0:
		return LevelNotable
	case t.Cash >= big, t.HasProfit && t.Side == "sold" && t.Profit >= gain, t.HasProfit && t.Profit <= -200:
		return LevelMoment
	case t.Cash >= 150, t.HasProfit && t.Side == "sold" && t.Profit >= 100, t.HasProfit && t.Profit <= -50:
		return LevelNotable
	}
	return LevelTicker
}

func tradeText(t *Trade) string {
	who := ""
	if t.Customer != "" {
		who = " " + map[string]string{"sold": "to", "bought": "from"}[t.Side] + " " + t.Customer
	}
	what := summarizeItems(t.Items, 3)
	switch t.Side {
	case "sold":
		s := fmt.Sprintf("Sold %s%s for %d", what, who, t.Cash)
		if t.HasProfit && t.Cost > 0 {
			s += fmt.Sprintf(" (%+d on cost)", t.Profit)
		}
		s += "."
		if t.Fakes > 0 {
			s += " A counterfeit went out the door."
		}
		return s
	case "bought":
		verb := "Bought"
		if t.Wholesale {
			verb = "Bought wholesale"
		}
		s := fmt.Sprintf("%s %s%s for %d", verb, what, who, t.Cash)
		if t.Estimate > 0 {
			s += fmt.Sprintf(" (estimate %d)", t.Estimate)
		}
		return s + "."
	}
	return fmt.Sprintf("Traded %s for %d.", what, t.Cash)
}

// groupNames turns ["Wine","Wine","Crowbar"] into ["2× Wine","Crowbar"], most first.
func groupNames(names []string) []string {
	counts := map[string]int{}
	var order []string
	for _, n := range names {
		if counts[n] == 0 {
			order = append(order, n)
		}
		counts[n]++
	}
	slices.SortStableFunc(order, func(a, b string) int { return counts[b] - counts[a] })
	out := make([]string, len(order))
	for i, n := range order {
		out[i] = countOf(counts[n], n)
	}
	return out
}

func summarizeItems(items []string, n int) string {
	if len(items) <= n {
		return joinAnd(items)
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(" and %d more", len(items)-n)
}

func (b *Builder) action(e *logEvent) []Event {
	var d struct {
		Action    string   `json:"action"`
		Upgrade   string   `json:"upgrade"`
		Cost      int64    `json:"cost"`
		CashSpent *int64   `json:"cash_spent"`
		Via       string   `json:"via"`
		TurnedOn  []string `json:"turned_on"`
		TurnedOff []string `json:"turned_off"`
		Location  string   `json:"location"`
		ItemUID   int64    `json:"item_uid"`
		Into      string   `json:"into"`
		ClientSeq int64    `json:"client_seq"`
		Text      string   `json:"text"`
		History   []string `json:"history"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return nil
	}
	switch d.Action {
	case "upgrade_buy":
		name := b.upgrades[d.Upgrade]
		if name == "" {
			name = strings.ToLower(strings.ReplaceAll(d.Upgrade, "_", " "))
		}
		spent := d.Cost
		if d.CashSpent != nil {
			spent = *d.CashSpent
		}
		text := fmt.Sprintf("Bought %s for %d.", name, d.Cost)
		if spent < d.Cost {
			text = fmt.Sprintf("Bought %s: %d, %d of it in Wild's favor.", name, d.Cost, d.Cost-spent)
		}
		b.clock.Cash -= spent
		return []Event{{Kind: KindPurchase, Level: LevelMoment, Amount: -spent, Clock: b.Clock(), Text: text}}
	case "services_set":
		var parts []string
		for _, s := range d.TurnedOn {
			parts = append(parts, "on: "+serviceName(s))
		}
		for _, s := range d.TurnedOff {
			parts = append(parts, "off: "+serviceName(s))
		}
		if len(parts) == 0 {
			return nil
		}
		return []Event{{Kind: KindThought, Level: LevelTicker, Text: "Services " + strings.Join(parts, ", ") + "."}}
	case "scavenge":
		b.scavenge()
	case "go_out":
		b.goOut(d.Location)
		return []Event{{Kind: KindThought, Level: LevelTicker, Text: "Out tonight: " + strings.ReplaceAll(d.Location, "_", " ") + "."}}
	case "conceal":
		n := b.names[d.ItemUID]
		if n == "" {
			n = "a stolen item"
		}
		return []Event{{Kind: KindThought, Level: LevelNotable, Text: fmt.Sprintf("Hid %s in the %s.", n, d.Into)}}
	case "unholster":
		return b.unholster(d.ClientSeq)
	case "dialogue_choice":
		// The gym logs the customer after the dialogue: the history names the speaker.
		if b.clients[d.ClientSeq].name == boardMember || speaker(d.History) == boardMember {
			return b.boardChoice(d.Text, true, d.History)
		}
	}
	return nil
}

// autopilot turns the live autopilot's action log into ticker thoughts, each with the executor's reason
// (what docs/SHOW.md's gym replays read from gym.summary.json); its pauses and resumes into autopilot events.
// The day's plan arrives from /v1/summary instead.
func (b *Builder) autopilot(e *logEvent) []Event {
	var d struct {
		Action string `json:"action"`
		Target *struct {
			Name string `json:"name"`
		} `json:"target"`
		Why        string   `json:"why"`
		Detail     string   `json:"detail"`
		Client     string   `json:"client"`
		HiddenUsed []string `json:"hidden_used"`
	}
	if json.Unmarshal(e.Data, &d) != nil || d.Action == "" {
		return nil
	}
	switch d.Action {
	case "pause", "resume":
		state := map[string]string{"pause": "paused", "resume": "running"}[d.Action]
		ev := Event{Kind: KindAutopilot, Level: LevelNotable, Autopilot: &AutopilotState{State: state, Reason: d.Why}, Text: "Autopilot " + state + ": " + clip(d.Why, 160) + "."}
		if d.Action == "resume" && b.handedOver {
			ev.Level, ev.Beat, ev.Text = LevelMoment, BeatTakeBack, "Back on autopilot: the AI has the shop again."
		}
		b.handedOver = b.handedOver && d.Action != "resume"
		return []Event{ev}
	case "plan_day":
		return nil
	case "dialogue_choice":
		if id, _, _ := strings.Cut(d.Detail, " "); id == boardIntroID || id == boardPromotionID {
			if evs := b.boardChoice(d.Why, id == boardPromotionID, nil); evs != nil {
				return evs
			}
		}
	}
	what := strings.ReplaceAll(d.Action, "_", " ")
	if d.Action == "counter" && d.Detail != "" {
		what = strings.ReplaceAll(d.Detail, "_", " ")
		if c := d.Detail[len(d.Detail)-1]; c >= '0' && c <= '9' {
			what += "%"
		}
	}
	if d.Target != nil && d.Target.Name != "" {
		what += " " + d.Target.Name
	}
	if d.Client != "" {
		what += " (" + d.Client + ")"
	}
	text := strings.ToUpper(what[:1]) + what[1:]
	if d.Why != "" {
		text += ": " + clip(d.Why, 160)
	}
	level := LevelTicker
	if d.Action == "raise_gun" || d.Action == "shoot" {
		level = LevelNotable
	}
	return []Event{{Kind: KindThought, Level: level, HiddenUsed: d.HiddenUsed, Text: text + "."}}
}

func (b *Builder) autopilotFault(e *logEvent) []Event {
	var d struct {
		State  string `json:"state"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return nil
	}
	b.handedOver = true
	return []Event{{Kind: KindAutopilot, Level: LevelMoment, Beat: BeatHandover, Autopilot: &AutopilotState{State: d.State, Reason: d.Reason, NeedsHuman: true}, Text: handoverText(d.State, d.Reason)}}
}

func serviceName(id string) string {
	return strings.ToLower(strings.TrimSuffix(strings.ReplaceAll(id, "_SERVICE", ""), "_"))
}

func (b *Builder) money(e *logEvent) []Event {
	var d struct {
		Kind      string `json:"kind"`
		NetWorth  *int64 `json:"net_worth"`
		Credits   *int64 `json:"credits"`
		Amount    int64  `json:"amount"`
		Left      *int64 `json:"credits_left"`
		Price     int64  `json:"price"`
		Value     int64  `json:"value"`
		Cost      int64  `json:"cost"`
		CashSpent int64  `json:"cash_spent"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return nil
	}
	if d.Left != nil {
		b.clock.Cash = *d.Left
	}
	if evs, ok := b.mortgage(d.Kind, e.Data); ok {
		return evs
	}
	ev := func(level int, amount int64, format string, a ...any) []Event {
		return []Event{{Kind: KindMoney, Level: level, Amount: amount, Clock: b.Clock(), Text: fmt.Sprintf(format, a...)}}
	}
	switch d.Kind {
	case "day_ended":
		if d.Credits != nil {
			b.clock.Cash = *d.Credits
		}
		if d.NetWorth != nil {
			b.clock.NetWorth = *d.NetWorth
		}
		return []Event{b.daySummary()}
	case "rent_paid":
		return with(ev(LevelNotable, -d.Amount, "Paid the rent: %d.", d.Amount), landlord)
	case "rent_missed":
		return with(ev(LevelMoment, 0, "Missed the rent."), landlord)
	case "georgio_loan":
		// The shop lends to Georgio; he repays four times as much later.
		b.clock.Cash -= d.Amount
		return with(ev(LevelMoment, -d.Amount, "Lent %d to Georgio.", d.Amount), georgio)
	case "georgio_payback":
		b.clock.Cash += d.Amount
		return with(ev(LevelMoment, d.Amount, "Georgio paid back %d.", d.Amount), georgio)
	case "permit_bought":
		return ev(LevelMoment, -d.Price, "Bought the vendor permit for %d.", d.Price)
	case "permit_bribe":
		return ev(LevelTicker, -d.Value, "Bribed the permit inspector with goods worth %d.", d.Value)
	case "inspector_bribe":
		return ev(LevelMoment, -d.Amount, "Bribed the inspector: %d.", d.Amount)
	case "burglary":
		return ev(LevelMoment, 0, "The shop was burgled.")
	case "service_paid", "wild_upgrade":
		return nil
	}
	if d.Amount != 0 {
		return ev(LevelTicker, d.Amount, "%s: %d.", strings.ReplaceAll(d.Kind, "_", " "), d.Amount)
	}
	return nil
}

func (b *Builder) daySummary() Event {
	s := b.today
	s.CashDelta = b.clock.Cash - b.morningCash
	s.NetWorthDelta = b.clock.NetWorth - b.morningNW
	rep := b.closeRep
	if rep != nil && b.morningRep != nil {
		for k, v := range rep {
			if d := int(math.Round(v - b.morningRep[k])); d != 0 {
				if s.Rep == nil {
					s.Rep = map[string]int{}
				}
				s.Rep[k] = d
			}
		}
	}
	level := LevelNotable
	if b.day%7 == 0 || abs(s.NetWorthDelta) >= max(600, b.clock.NetWorth/25) {
		level = LevelMoment
	}
	c := b.Clock()
	text := fmt.Sprintf("Day %d closes: cash %d (%+d), net worth %d (%+d).", b.day, b.clock.Cash, s.CashDelta, b.clock.NetWorth, s.NetWorthDelta)
	switch {
	case c == nil || c.Owned:
	case c.MortgageWeekly > 0:
		text += fmt.Sprintf(" Mortgage payment %d due in %s.", c.MortgageWeekly, days(c.RentDays))
	case c.RentDue > 0:
		text += fmt.Sprintf(" Rent %d due in %s.", c.RentDue, days(c.RentDays))
	}
	return Event{Kind: KindDay, Level: level, Summary: &s, Clock: c, Text: text}
}

func days(n int) string {
	switch n {
	case 0:
		return "0 days (today)"
	case 1:
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

func (b *Builder) aug(e *logEvent) []Event {
	var d struct {
		Event string `json:"event"`
		Name  string `json:"name"`
		Cash  int64  `json:"cash_delta"`
		Cause string `json:"cause"`
	}
	if json.Unmarshal(e.Data, &d) != nil {
		return nil
	}
	b.clock.Cash += d.Cash
	switch d.Event {
	case "good_kill":
		return []Event{{Kind: KindAug, Level: LevelMoment, Beat: BeatAug, Amount: d.Cash, Clock: b.Clock(), Text: fmt.Sprintf("Shot an aug posing as %s. Bounty %d.", aOrAn(d.Name), d.Cash)}}
	case "wrongful_kill":
		return []Event{{Kind: KindAug, Level: LevelMoment, Beat: BeatAug, Text: fmt.Sprintf("Shot %s. Not an aug: manslaughter.", aOrAn(d.Name))}}
	case "wanted_capture":
		return []Event{{Kind: KindAug, Level: LevelMoment, Amount: d.Cash, Text: fmt.Sprintf("Caught a wanted %s.", d.Name)}}
	}
	return nil
}

func aOrAn(name string) string {
	if name == "" {
		return "a customer"
	}
	if strings.HasPrefix(name, "The ") || strings.ToUpper(name[:1]) != name[:1] {
		return name
	}
	if strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

// Plan makes the plan event for a day, comparing it with the previous plan.
func (b *Builder) Plan(in PlanInput) Event {
	p := &Plan{Day: in.Day, Source: "directives"}
	p.Directives = append(Describe(in.Directives, Names{Item: b.itemName, Upgrade: func(id string) string { return b.upgrades[id] }}), in.Extra...)
	if in.Text != "" {
		p.Source = "llm"
		p.Sections = ParsePlan(in.Text)
		p.Headline = Headline(p.Sections, 4)
	} else if len(in.FromModel) == 0 && len(p.Directives) == 0 {
		p.Source = "executor"
	}
	keys := make([]string, 0, len(in.Why))
	for k := range in.Why {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if k == "*" {
			p.Why = append(p.Why, in.Why[k])
		} else {
			p.Why = append(p.Why, reasonKey(k)+": "+in.Why[k])
		}
	}
	for _, f := range in.Fixes {
		p.Why = append(p.Why, "overruled: "+f)
	}
	if len(p.Headline) == 0 && b.goal != "" {
		p.Goals = append(p.Goals, fmt.Sprintf("Day %d plan: %s", b.goalDay, b.goal))
	}
	p.Goals = append(p.Goals, b.goals()...)
	if len(p.Headline) > 0 {
		b.goal, b.goalDay = p.Headline[0], in.Day
		b.headline, b.headlineDay = p.Headline, in.Day
	}
	first := b.planDay == 0
	p.Changes = Changes(b.planDirectives, p.Directives)
	p.Change = !first && len(p.Changes) > 0
	b.planDirectives, b.planDay = p.Directives, in.Day

	ev := Event{Kind: KindPlan, Day: in.Day, Plan: p, Clock: b.Clock(), Level: LevelNotable}
	switch {
	case in.Failed:
		ev.Text = fmt.Sprintf("Day %d: the planner failed; yesterday's plan stands.", in.Day)
	case len(p.Headline) > 0:
		ev.Text = fmt.Sprintf("Day %d plan: %s", in.Day, p.Headline[0])
	case p.Source == "executor" && len(p.Goals) > 0:
		ev.Text = fmt.Sprintf("Day %d: the plan stands. %s.", in.Day, strings.TrimRight(p.Goals[0], "."))
		ev.Level = LevelTicker
	case p.Source == "executor":
		ev.Text = fmt.Sprintf("Day %d: the plan stands.", in.Day)
		ev.Level = LevelTicker
	case len(p.Directives) == 0 && len(p.Goals) > 0:
		ev.Text = fmt.Sprintf("Day %d: the bridge overruled every change the model asked for; the plan stands. %s.", in.Day, strings.TrimRight(p.Goals[0], "."))
	case len(p.Directives) == 0:
		ev.Text = fmt.Sprintf("Day %d: the bridge overruled every change the model asked for; the plan stands.", in.Day)
	default:
		ev.Text = fmt.Sprintf("Day %d plan: %s.", in.Day, capitalize(joinAnd(p.Directives)))
	}
	if first || (p.Change && substantive(p.Changes)) {
		ev.Level = LevelMoment
	}
	// The day's first word on the plan is told in one sentence; later changes stay as they were.
	if !in.Failed && ev.Level >= LevelNotable && in.Day > b.morningPlanDay {
		b.morningPlanDay = in.Day
		ev.Level, ev.Beat = LevelMoment, BeatMorning
	}
	return ev
}

// hybridPlan is today's plan in a hybrid game: the chosen candidate of a slate
// (docs/research/HYBRID.md), whose reason is the plan's why. A hybrid replay has no other plan.
func (b *Builder) hybridPlan(ev Event) (Event, bool) {
	// A strategist's fork (metric "choice") patches the plan in force; it is not a new plan.
	if ev.Kind != KindCandidates || ev.Candidates == nil || ev.Candidates.Metric == "choice" {
		return Event{}, false
	}
	i := slices.IndexFunc(ev.Candidates.Options, func(o Candidate) bool { return o.Chosen })
	if i < 0 {
		return Event{}, false
	}
	o := ev.Candidates.Options[i]
	var d protocol.Directives
	var raw map[string]json.RawMessage
	if len(o.Delta) > 0 && json.Unmarshal(o.Delta, &raw) == nil {
		_ = json.Unmarshal(o.Delta, &d)
	}
	day := cmp.Or(ev.Day, ev.Candidates.Day, b.day)
	in := PlanInput{Day: day, Directives: &d, Extra: gymWords(raw)}
	for k := range raw {
		in.FromModel = append(in.FromModel, k)
	}
	if o.Why != "" {
		in.Why = map[string]string{"*": o.Why}
	}
	p := b.Plan(in)
	if p.Plan.Source == "directives" {
		p.Plan.Source = "hybrid"
		p.Text = fmt.Sprintf("Day %d plan: %s.", day, capitalize(o.Label))
	}
	// The slate itself is the moment; its plan only updates the panel.
	p.Level = min(p.Level, LevelNotable)
	return p, true
}

// gymWords describes the gym executor's levers that protocol.Directives does not carry.
func gymWords(raw map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var out []string
	for _, k := range keys {
		var s string
		var n float64
		_ = json.Unmarshal(raw[k], &s)
		_ = json.Unmarshal(raw[k], &n)
		switch k {
		case "georgio":
			switch s {
			case "lend_25":
				out = append(out, "lend Georgio a quarter of the cash")
			case "lend_50":
				out = append(out, "lend Georgio half the cash")
			case "no":
				out = append(out, "turn Georgio down")
			}
		case "georgio_collect":
			out = append(out, "spend under the rent so Georgio repays early")
		case "georgio_peak_days":
			out = append(out, "sell down for Georgio's loan on days "+strings.Trim(string(raw[k]), "[]"))
		case "buyout":
			out = append(out, "hold cash for the buyout")
		case "repo_reserve_payments":
			out = append(out, fmt.Sprintf("keep stock worth %g payments", n))
		case "spend_down_to":
			out = append(out, fmt.Sprintf("spend down to %s cash", num(int64(n))))
		case "upgrade_reserve":
			out = append(out, fmt.Sprintf("keep %s back from upgrades", num(int64(n))))
		case "mortgage_selldown_days":
			out = append(out, fmt.Sprintf("sell down %g days before a payment", n))
		}
	}
	return out
}

// goals are the standing goals from the clock and the base directives.
func (b *Builder) goals() []string {
	var out []string
	cashRoute := b.Base != nil && (b.Base.BuyoutByDay != nil || b.Base.Mortgage == "decline")
	if c := b.Clock(); c != nil && !c.Owned {
		switch {
		case c.MortgageWeekly > 0:
			out = append(out, fmt.Sprintf("Paying the mortgage: %s a week, %s left", num(c.MortgageWeekly), plural(c.MortgageWeeksLeft, "week")))
		case cashRoute && c.BuyoutPrice > 0:
			// The route answers the loan shark "later" and buys the shop with cash.
			out = append(out, fmt.Sprintf("Saving for the buyout: %s / %s", num(c.Cash), num(c.BuyoutPrice)))
		case c.OfferDown > 0:
			// The executor signs once the cash leaves 1.5 payments and its float (1,000 by
			// default) after the down payment (sim/gym/PlayerRole.cs).
			keep := int64(1000)
			if b.Base != nil && b.Base.MortgageFloat != nil {
				keep = *b.Base.MortgageFloat
			}
			out = append(out, fmt.Sprintf("Saving for the mortgage: %s / %s", num(c.Cash), num(c.OfferDown+c.OfferWeekly*3/2+keep)))
		}
		what, owed := "Rent", c.RentDue+c.Arrears
		if c.MortgageWeekly > 0 {
			what, owed = "Next payment", c.MortgageWeekly+c.Arrears
		}
		if owed > 0 {
			if c.Cash >= owed {
				out = append(out, fmt.Sprintf("%s %s in %s: covered, %s to spare", what, num(owed), days(c.RentDays), num(c.Cash-owed)))
			} else {
				out = append(out, fmt.Sprintf("%s %s in %s: short by %s", what, num(owed), days(c.RentDays), num(owed-c.Cash)))
			}
		}
		if c.BuyoutPrice > 0 && c.MortgageWeekly == 0 && !cashRoute {
			out = append(out, fmt.Sprintf("Buyout: net worth %s / %s", num(c.NetWorth), num(c.BuyoutPrice)))
		}
	}
	for _, d := range Describe(b.Base, Names{Item: b.itemName, Upgrade: func(id string) string { return b.upgrades[id] }}) {
		if substantive([]string{d}) {
			out = append(out, capitalize(d))
		}
	}
	return out
}

// reasonKey names a directive key for a viewer: "upgrades_buy" → "upgrades".
func reasonKey(k string) string {
	return strings.ReplaceAll(strings.TrimSuffix(k, "_buy"), "_", " ")
}

// num writes n with thousands separators.
func num(n int64) string {
	s := strconv.FormatInt(abs(n), 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if n < 0 {
		return "-" + s
	}
	return s
}

// bigChanges are the instructions that change the run rather than the day's shopping.
var bigChanges = []string{"buy the", "refuse", "bribe", "mortgage", "take the", "run ", "sell down", "keep at least", "shoot", "serve", "stay in", "haggle hard", "save for"}

// substantive is false when a plan change only reshuffles what to buy, hold or show.
func substantive(changes []string) bool {
	for _, c := range changes {
		if strings.HasPrefix(c, "no longer: ") {
			continue
		}
		for _, p := range bigChanges {
			if strings.HasPrefix(c, p) {
				return true
			}
		}
	}
	return false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func runStartText(p Player) string {
	who := p.Model
	if who == "" {
		who = p.ID
	}
	if p.PersonaName != "" {
		who += " as " + p.PersonaName
	}
	s := fmt.Sprintf("New run: %s takes the shop", who)
	if p.Difficulty != "" {
		s += " on " + p.Difficulty
	}
	if p.Route != "" {
		s += ", the " + strings.ReplaceAll(p.Route, "_", " ") + " route"
	}
	if p.Start != "" {
		s += ", " + strings.ReplaceAll(p.Start, "_", " ") + " start"
	}
	if p.Format != nil {
		return s + ". " + p.Format.Caption()
	}
	return s + "."
}

func endText(e *End) string {
	switch {
	case e.Won:
		return fmt.Sprintf("Day %d: bought the shop. The run is won.", e.Day)
	case e.Ending == "rent":
		return fmt.Sprintf("Day %d: could not pay the rent. Evicted with %d cash.", e.Day, e.Cash)
	case e.Ending == "arrest":
		return fmt.Sprintf("Day %d: arrested. The run is over.", e.Day)
	case e.Ending == "execution":
		return fmt.Sprintf("Day %d: executed. The run is over.", e.Day)
	case e.Ending == "alive":
		return fmt.Sprintf("Day %d: still standing at the horizon.", e.Day)
	}
	return fmt.Sprintf("Day %d: the run ends (%s).", e.Day, e.Ending)
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}
