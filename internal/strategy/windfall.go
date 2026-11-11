package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	// maxWindfallsPerDay bounds mid-day re-plans: each is a model call and grows the plan
	// every customer prompt carries.
	maxWindfallsPerDay = 2
	maxWindfallBytes   = 320
	windfallMaxTokens  = 400
)

// counterItem is a seller's item as the barter context lists it.
type counterItem struct {
	UID            *int64 `json:"uid"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	Price          *int64 `json:"price"`
	EstimatedValue *int64 `json:"estimated_value"`
}

func (it counterItem) key() string {
	if it.UID != nil {
		return fmt.Sprintf("#%d", *it.UID)
	}
	return strings.ToLower(nonEmpty(it.ID, it.Name))
}

func (it counterItem) label() string {
	t := nonEmpty(it.Name, it.ID)
	if it.UID != nil {
		t += fmt.Sprintf(" #%d", *it.UID)
	}
	return t
}

// engineItems are the seller's items in a barter context that the catalogue marks as
// engine-enabling, and the shop's cash.
func engineItems(ctx json.RawMessage) (items []counterItem, cash *int64) {
	var c struct {
		Store *struct {
			Cash *int64 `json:"cash"`
		} `json:"store"`
		Counter *struct {
			Theirs []counterItem `json:"theirs"`
		} `json:"counter"`
	}
	if json.Unmarshal(ctx, &c) != nil || c.Counter == nil {
		return nil, nil
	}
	for _, it := range c.Counter.Theirs {
		if e, ok := catalogue.Lookup(it.ID, it.Name); ok && e.Engine {
			items = append(items, it)
		}
	}
	if c.Store != nil {
		cash = c.Store.Cash
	}
	return items, cash
}

// windfall is one engine item turning up mid-day.
type windfall struct {
	item counterItem
	// how: "offered" (a seller has it on the counter) or "acquired".
	how  string
	cash *int64
}

func (w windfall) text(snapshot json.RawMessage) string {
	t := w.item.label()
	switch w.how {
	case "offered":
		t += " is on a seller's counter"
		if w.item.Price != nil {
			t += fmt.Sprintf(" at %d", *w.item.Price)
		}
	default:
		t += " has just been acquired by the shop"
	}
	sale := int64(0)
	if w.item.EstimatedValue != nil {
		sale = *w.item.EstimatedValue
	}
	if n := UseNote(snapshot, w.item.ID, w.item.Name, sale); n != "" {
		t += "; " + n
	}
	if w.cash != nil {
		t += fmt.Sprintf("; shop cash %d", *w.cash)
	}
	return t
}

const windfallSystem = `You are the morning strategist of a pawnshop in "Probably Stolen". Mid-day, an item that can start or grow an income engine has turned up. Decide whether it changes today's plan. Value it by what it enables in this shop (its use value), not its price. Follow YOUR STYLE's windfall rule: stay on the path unless the windfall clearly beats the path's next purchase. Never break the plan's cash floor or a RISK limit for it.
Answer in at most 3 short lines, plain text, no markdown:
WINDFALL: <item> -> pivot? yes|no, <why: its use against the path's next purchase>
- <the change to today's plan: buy it up to N, put it to work tonight, what to sell or skip to afford it>
- <a second change, or "no other change">`

// BuildWindfallPrompt is the delta-plan prompt for w against today's plan.
func BuildWindfallPrompt(plan, persona string, snapshot json.RawMessage, w windfall) string {
	var b strings.Builder
	fmt.Fprintf(&b, "TODAY'S PLAN:\n%s\n\n", plan)
	fmt.Fprintf(&b, "WINDFALL: %s.\n", w.text(snapshot))
	renderStyle(&b, persona)
	b.WriteString("\nWrite the WINDFALL lines.")
	return b.String()
}

// TidyWindfall keeps the delta's WINDFALL line and up to two change lines, within
// maxWindfallBytes; "" when there is no WINDFALL line.
func TidyWindfall(text string) string {
	var out []string
	for _, l := range strings.Split(llm.StripThinking(text), "\n") {
		l = cleanLine(l)
		switch {
		case l == "" || strings.HasPrefix(l, "```"):
		case len(out) == 0:
			if i := strings.Index(strings.ToUpper(l), "WINDFALL:"); i >= 0 {
				out = append(out, "WINDFALL:"+l[i+len("WINDFALL:"):])
			}
		case len(out) < 3:
			out = append(out, bullet(l))
		}
	}
	return truncateLines(strings.Join(out, "\n"), maxWindfallBytes)
}

// noteWindfalls queues a re-plan for each engine item not yet re-planned today, within
// the day's limit. Callers hold s.mu.
func (s *Strategist) noteWindfalls(session string, r *run, day int, ws []windfall) {
	if s.opt.LLM == nil || r.plan == "" {
		return
	}
	d := r.day(day)
	for _, w := range ws {
		if d.windfalls >= maxWindfallsPerDay {
			return
		}
		if d.windfallSeen == nil {
			d.windfallSeen = map[string]bool{}
		}
		k := w.how + " " + w.item.key()
		if d.windfallSeen[k] {
			continue
		}
		d.windfallSeen[k] = true
		d.windfalls++
		w := w
		s.enqueue(r, func(ctx context.Context) { s.replan(ctx, session, r, day, w) })
	}
}

// replan asks for a delta plan and appends it to the run's current plan.
func (s *Strategist) replan(ctx context.Context, session string, r *run, day int, w windfall) {
	s.mu.Lock()
	plan, persona := r.plan, s.personaFor(session, r)
	var snap json.RawMessage
	if d := r.days[r.planDay]; d != nil {
		snap = d.morningRaw
	}
	s.mu.Unlock()
	if plan == "" {
		return
	}
	start := time.Now()
	resp, err := s.opt.LLM.Complete(ctx, llm.Request{System: windfallSystem, MaxTokens: windfallMaxTokens,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: BuildWindfallPrompt(plan, persona, snap, w)}}})
	if err != nil {
		s.log.Warn("windfall re-plan failed; the plan stands", "day", day, "item", w.item.label(), "err", err)
		return
	}
	delta := TidyWindfall(resp.Text)
	if delta == "" {
		s.log.Warn("windfall re-plan had no WINDFALL line; the plan stands", "day", day, "item", w.item.label())
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.plan != plan {
		// A new morning plan or another windfall landed meanwhile; append to that one.
		plan = r.plan
	}
	r.plan = plan + "\n" + delta
	if d := r.days[r.planDay]; d != nil && d.plan != "" {
		d.plan += "\n" + delta
	}
	s.log.Info("windfall re-plan", "day", day, "item", w.item.label(), "how", w.how, "ms", time.Since(start).Milliseconds())
}

// decisionWindfalls notes engine items a seller offers and remembers them by decision, so
// a later purchase outcome can name what was bought. Callers hold s.mu.
func (s *Strategist) decisionWindfalls(session string, r *run, req *protocol.DecideRequest) {
	if req.Kind != "barter" {
		return
	}
	items, cash := engineItems(req.Context)
	if len(items) == 0 {
		return
	}
	d := r.day(req.GameDay)
	if d.offers == nil {
		d.offers = map[string][]counterItem{}
	}
	d.offers[req.DecisionID] = items
	ws := make([]windfall, 0, len(items))
	for _, it := range items {
		ws = append(ws, windfall{item: it, how: "offered", cash: cash})
	}
	s.noteWindfalls(session, r, req.GameDay, ws)
}

// outcomeWindfalls re-plans for engine items a purchase just brought in. Callers hold s.mu.
func (s *Strategist) outcomeWindfalls(session string, r *run, oc protocol.Outcome) {
	var res result
	if json.Unmarshal(oc.Result, &res) != nil || res.Kind != "trade" || (res.Side != "bought" && res.Side != "received") {
		return
	}
	d := r.days[oc.GameDay]
	if d == nil {
		return
	}
	var ws []windfall
	for _, it := range d.offers[oc.DecisionID] {
		for _, u := range res.Items {
			if it.UID != nil && *it.UID == u {
				ws = append(ws, windfall{item: it, how: "acquired"})
			}
		}
	}
	s.noteWindfalls(session, r, oc.GameDay, ws)
}
