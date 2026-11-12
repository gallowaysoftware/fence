package strategy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/show"
)

// Agency is the strategist's share of the run beyond the morning plan (docs/STREAM.md,
// "Model agency"): the day-0 route it chose, the forks it is asked when they arise, and how
// often it writes the plan.
type Agency struct {
	// Choice is the day-0 route choice the run plays; nil when the route was given.
	Choice *Choice
	// Forks puts the mid-run decisions (forks.go) to the model.
	Forks bool
	// PlanEvery writes the morning plan every N days (0 or 1: daily) and holds it, with
	// the forks' patches, in between. Above 1 it also turns off the mid-day re-plans.
	PlanEvery int
	// Start is the run's start, which some forks depend on; BaseDirectives is read for the rest.
	Start string
	// Show writes a feed event into the session's decision log; nil drops them.
	Show func(session string, ev show.Event)
}

// Choice is the day-0 decision as the strategist tells it (internal/agency.Choice).
type Choice struct {
	Route    string `json:"route"`
	Composed bool   `json:"composed,omitempty"`
	// Summary is a composition's base and levers.
	Summary  string   `json:"summary,omitempty"`
	Thesis   string   `json:"thesis"`
	Fallback string   `json:"fallback,omitempty"`
	Menu     []string `json:"menu,omitempty"`
}

// carryPlan stands in for the plan on the days between plans, before the first one is written.
const carryPlan = "The executor plays the route; the strategist's next plan is due on a later morning."

type agencyRun struct {
	forks map[string]*forkState
	// delta is the last plan's directives before the forks' patches.
	delta       *protocol.Directives
	lastPlanned int
}

// forkOrder is the order patches apply in: a later fork wins a key two of them set.
var forkOrder = []string{"style", "engine", "fountain", "sprint", "mortgage", "hold"}

func (s *Strategist) agencyOn() bool { return s.opt.Agency.Forks || s.opt.Agency.PlanEvery > 1 }

func (s *Strategist) morning(ctx context.Context, session string, r *run, n, gen int) {
	if !s.agencyOn() {
		s.planDay(ctx, session, r, n, gen)
		return
	}
	s.mu.Lock()
	d := r.days[n]
	if d == nil || d.planGen != gen || s.opt.LLM == nil {
		s.mu.Unlock()
		return
	}
	var forks []fork
	if s.opt.Agency.Forks && d.morning != nil {
		if w := r.days[n-7]; w != nil && w.morning != nil && w.morning.NetWorth != nil && d.morning.NetWorth != nil {
			g := *d.morning.NetWorth - *w.morning.NetWorth
			d.morning.weekGain = &g
		}
		if r.forks == nil {
			r.forks = map[string]*forkState{}
		}
		forks = detectForks(d.morning, n, s.forkContext(), r.forks)
	}
	system := forkSystem(s.opt.Route, s.personaFor(session, r))
	due := r.lastPlanned == 0 || n-r.lastPlanned >= max(1, s.opt.Agency.PlanEvery)
	if due {
		r.lastPlanned = n
	}
	s.mu.Unlock()

	for _, f := range forks {
		s.decideFork(ctx, session, r, n, f, system)
	}
	if due {
		s.planDay(ctx, session, r, n, gen)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d.planGen == gen && r.planDay < n {
		s.carry(r, n, len(forks) > 0)
	}
}

func (s *Strategist) forkContext() forkContext {
	fc := forkContext{start: s.opt.Agency.Start}
	if s.opt.BaseDirectives != "" {
		json.Unmarshal([]byte(s.opt.BaseDirectives), &fc.base)
	}
	if fc.base == nil {
		fc.base = map[string]any{}
	}
	return fc
}

// carry publishes day n's directives without a new plan: the last plan's, with the forks'
// patches. Callers hold s.mu.
func (s *Strategist) carry(r *run, n int, logIt bool) {
	var d protocol.Directives
	if r.delta != nil {
		d = cloneDirectives(r.delta)
	}
	s.applyForks(r, n, &d)
	d.Day = n
	if r.plan == "" {
		r.plan = carryPlan
	}
	r.planDay, r.directives = n, &d
	if logIt {
		if b, err := json.Marshal(d); err == nil {
			s.log.Info("directives", "day", n, "json", string(b), "why", "{}")
		}
	}
}

// overlayForks keeps the plan's own directives as the run's delta and applies the forks'
// patches over them. Callers hold s.mu.
func (s *Strategist) overlayForks(r *run, n int, d *protocol.Directives) {
	if !s.agencyOn() {
		return
	}
	c := cloneDirectives(d)
	r.delta = &c
	s.applyForks(r, n, d)
}

func (s *Strategist) applyForks(r *run, n int, d *protocol.Directives) {
	for _, k := range forkOrder {
		if st := r.forks[k]; st != nil && st.apply != nil && (st.until == 0 || n <= st.until) {
			st.apply(d)
		}
	}
}

func cloneDirectives(d *protocol.Directives) protocol.Directives {
	var c protocol.Directives
	if b, err := json.Marshal(d); err == nil {
		json.Unmarshal(b, &c)
	}
	return c
}

// decideFork asks the model and records its choice; an unusable answer leaves the route's
// own rule in force.
func (s *Strategist) decideFork(ctx context.Context, session string, r *run, n int, f fork, system string) {
	start := time.Now()
	a, raw, ok := askFork(ctx, s.opt.LLM, system, f)
	ms := time.Since(start).Milliseconds()
	s.mu.Lock()
	st := r.forks[f.Kind]
	st.asked++
	st.lastDay = n
	chosen := forkOption{ID: "route_rule", Label: "the route's own rule (no usable answer)"}
	if ok {
		for _, o := range f.Options {
			if o.ID == a.Choice {
				chosen = o
			}
		}
	}
	st.choice, st.apply, st.until = chosen.ID, chosen.apply, f.Until
	s.mu.Unlock()
	if !ok {
		s.log.Warn("fork answer unusable; the route's rule stands", "day", n, "kind", f.Kind, "raw", clip(raw, 300))
	}
	s.log.Info("fork", "day", n, "kind", f.Kind, "choice", chosen.ID, "fallback", !ok, "why", a.Why, "options", optionIDs(f), "ms", ms)
	if s.opt.Agency.Show == nil {
		return
	}
	level := show.LevelNotable
	if f.Moment || ok && chosen.ID != f.Options[0].ID {
		level = show.LevelMoment
	}
	text := fmt.Sprintf("Day %d, %s: the strategist chooses %s", n, lowerFirst(f.Title), chosen.Label)
	if a.Why != "" {
		text += ". " + a.Why
	}
	c := &show.Candidates{Day: n, Metric: "choice"}
	for _, o := range f.Options {
		opt := show.Candidate{Label: o.Label, Source: "llm", Chosen: o.ID == chosen.ID}
		if opt.Chosen {
			opt.Why = a.Why
		}
		c.Options = append(c.Options, opt)
	}
	s.opt.Agency.Show(session, show.Event{Day: n, Kind: show.KindCandidates, Level: level, Beat: show.BeatFork, Text: text, Candidates: c})
}

func optionIDs(f fork) string {
	ids := make([]string, len(f.Options))
	for i, o := range f.Options {
		ids[i] = o.ID
	}
	return strings.Join(ids, ",")
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// showChoice writes the day-0 choice and its thesis into session's feed once.
func (s *Strategist) showChoice(session string) {
	ch := s.opt.Agency.Choice
	if ch == nil || s.opt.Agency.Show == nil {
		return
	}
	s.mu.Lock()
	if s.choiceShown == nil {
		s.choiceShown = map[string]bool{}
	}
	if s.choiceShown[session] || len(s.choiceShown) >= maxSessions {
		s.mu.Unlock()
		return
	}
	s.choiceShown[session] = true
	s.mu.Unlock()
	head := "picks the " + ch.Route + " route"
	if ch.Composed {
		head = "composes its own route, " + ch.Route + " (" + ch.Summary + ")"
	}
	p := &show.Plan{Day: 0, Source: "llm", Change: true, Changes: []string{"run the " + ch.Route + " route"}, Headline: []string{"Day 0: the strategist " + head},
		Sections: []show.Section{{Title: "Thesis", Lines: []string{ch.Thesis}}}}
	if ch.Fallback != "" {
		p.Why = []string{"overruled: " + ch.Fallback}
	}
	s.opt.Agency.Show(session, show.Event{Kind: show.KindPlan, Level: show.LevelMoment, Beat: show.BeatRouteChoice, Text: "Day 0: the strategist " + head + ". " + ch.Thesis, Plan: p})
}
