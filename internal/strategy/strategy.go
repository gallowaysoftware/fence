// Package strategy writes the shop's plan for each in-game day and scores the day against
// it at night. The plan is what per-customer advice serves (docs/PLAN.md, phase S).
package strategy

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	DefaultTimeout       = 2 * time.Minute
	DefaultRecallTimeout = 500 * time.Millisecond
	// keepDays bounds the per-run day history; only yesterday feeds a plan.
	keepDays    = 8
	maxSessions = 256
	// Same run-continuity window as memory: a reload goes back at most reloadDays, and a
	// few days played with the bridge down still continue the run.
	reloadDays = 2
	maxGapDays = 3
)

// Recall has memory.Recall's method set, so a memory store can supply the lessons
// without this package importing it.
type Recall interface {
	Brief(ctx context.Context, req *protocol.DecideRequest, maxBytes int) string
}

type Options struct {
	// LLM writes plans and scorecards; nil records only (scorecards carry facts alone).
	LLM    llm.Client
	Recall Recall
	// Audience is the stream audience's word for day n's plan (PlanInput.Audience); nil for none.
	Audience func(day int) string
	Logger   *slog.Logger
	// Timeout bounds each plan or scorecard, its repair round included.
	Timeout       time.Duration
	RecallTimeout time.Duration
	MaxTokens     int
	// Persona is the play style every plan follows (docs/personas), "" for none. A session's
	// sim_run event may name another by id.
	Persona string
	// PersonaDir resolves those ids; DefaultPersonaDir when empty.
	PersonaDir string
	// Exemplars are strong-play days the morning plan is shown the nearest of; nil for none.
	Exemplars Exemplars
	// DeltaDirectives writes directives as changes to the executor's defaults.
	DeltaDirectives bool
	// BaseDirectives is the directives JSON the executor plays over its defaults (the gym's
	// -directives), shown to the delta ask; "" for none.
	BaseDirectives string
	// Route is the run's route (route.Route.Text): its plans are written as that shop; "" for none.
	Route string
	// Agency (agency.go): the day-0 choice, the forks and the plan cadence.
	Agency Agency
}

// Strategist implements the server's Recorder (it sees the stripped views only) and
// Planner. Recording never blocks: model calls run on one goroutine chain per run, so a
// day's scorecard is written before the next morning's plan reads it.
type Strategist struct {
	opt    Options
	log    *slog.Logger
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex
	closed   bool
	sessions map[string]*run
	order    []string
	latest   *run
	// personas are per-session style overrides from sim_run.
	personas map[string]string
	// choiceShown are the sessions whose feed has the day-0 choice.
	choiceShown map[string]bool
}

// run is one playthrough as far as the strategist can tell; sessions of the same save
// share it.
type run struct {
	lastDay int
	days    map[int]*dayState
	// persona is the style a session override gave the run, so a reload keeps it.
	persona string
	// augs is the run's aug history, for the presence the plan needs.
	augs augHistory
	// wine is when each finished wine appeared, to tell a stalled wine engine.
	wine WineHistory
	// plan is the latest plan written; a failed morning keeps the previous one. directives
	// are written with it.
	plan, planModel string
	planDay         int
	directives      *protocol.Directives
	reviewDay       int
	tail            chan struct{}
	agencyRun
}

type dayState struct {
	n              int
	morningRaw     json.RawMessage
	morning, close *snapshot
	planGen        int
	plan, model    string
	directives     *protocol.Directives
	scorecard      string
	facts          string
	stats          *dayStats
	observed       int
	arrived        int
	// windfalls counts the day's mid-day re-plans; windfallSeen keys the items they were
	// for; offers are the engine items each barter decision's seller had.
	windfalls    int
	windfallSeen map[string]bool
	offers       map[string][]counterItem
}

func New(opt Options) *Strategist {
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.DiscardHandler)
	}
	if opt.Timeout <= 0 {
		opt.Timeout = DefaultTimeout
	}
	if opt.RecallTimeout <= 0 {
		opt.RecallTimeout = DefaultRecallTimeout
	}
	if opt.PersonaDir == "" {
		opt.PersonaDir = DefaultPersonaDir
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Strategist{opt: opt, log: opt.Logger, ctx: ctx, cancel: cancel, sessions: map[string]*run{}, personas: map[string]string{}}
}

// Close cancels pending model calls and waits for them to return.
func (s *Strategist) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.cancel()
	s.wg.Wait()
}

func (s *Strategist) Hello(session string, _ protocol.HelloRequest) { s.showChoice(session) }

func (s *Strategist) Event(session string, ev protocol.Event) {
	if ev.Kind == "sim_run" {
		s.simRun(session, ev)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if ev.Kind != "new_game_started" && ev.Kind != "day_snapshot" {
		// Any dated event binds the session, so a reload sees its run's plan at once.
		s.runFor(session, ev.GameDay)
	}
	switch ev.Kind {
	case "new_game_started":
		s.bind(session, newRun(ev.GameDay))
	case "day_snapshot":
		snap, err := parseSnapshot(ev.Data)
		if err != nil {
			s.log.Warn("day snapshot unreadable", "session", session, "err", err)
			return
		}
		n := snap.day(ev.GameDay)
		r := s.runFor(session, n)
		if r == nil {
			return
		}
		d := r.day(n)
		switch snap.Phase {
		case "morning":
			d.morning, d.morningRaw = snap, ev.Data
			r.augs.noteMorning(n, snap.NightLog)
			r.wine.note(n, snap)
			d.planGen++
			gen := d.planGen
			s.enqueue(r, func(ctx context.Context) { s.morning(ctx, session, r, n, gen) })
		case "close":
			d.close = snap
		}
	case "going_sleep":
		if r := s.runFor(session, ev.GameDay); r != nil {
			n := ev.GameDay
			s.enqueue(r, func(ctx context.Context) { s.reviewDay(ctx, r, n) })
		}
	case "aug":
		if r := s.runFor(session, ev.GameDay); r != nil && isGoodKill(ev.Data) {
			r.augs.noteKill(ev.GameDay)
		}
	case "customer_observed":
		if r := s.runFor(session, ev.GameDay); r != nil {
			r.day(ev.GameDay).observed++
		}
	case "client_arrived":
		if r := s.runFor(session, ev.GameDay); r != nil {
			r.day(ev.GameDay).arrived++
		}
	case "human_action":
		var a struct {
			Action  string `json:"action"`
			Advised string `json:"advised"`
		}
		if json.Unmarshal(ev.Data, &a) != nil || a.Advised == "" || a.Advised == protocol.OptionDefer || !isCounterMove(a.Action) {
			return
		}
		if r := s.runFor(session, ev.GameDay); r != nil {
			st := r.day(ev.GameDay).stats
			if a.Action == a.Advised {
				st.followed++
			} else {
				st.overridden++
			}
		}
	}
}

// simRun takes the session's persona override; an unknown persona keeps the default.
func (s *Strategist) simRun(session string, ev protocol.Event) {
	id := simRunPersona(ev.Data)
	if id == "" {
		return
	}
	if isPersonaPath(id) {
		s.log.Warn("sim_run persona must be an id, not a path; keeping the default", "session", session, "persona", id)
		return
	}
	text, err := LoadPersona(s.opt.PersonaDir, id)
	if err != nil {
		s.log.Warn("sim_run persona unavailable; keeping the default", "session", session, "err", err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		if len(s.personas) >= maxSessions {
			for k := range s.personas {
				if s.sessions[k] == nil {
					delete(s.personas, k)
				}
			}
		}
		s.personas[session] = text
		s.log.Info("persona set", "session", session, "persona", id)
	}
}

// personaFor is the style session's plans follow. Callers hold s.mu.
func (s *Strategist) personaFor(session string, r *run) string {
	if p, ok := s.personas[session]; ok {
		r.persona = p
		return p
	}
	if r.persona != "" {
		return r.persona
	}
	return s.opt.Persona
}

// isCounterMove tells barter options (docs/PROTOCOL.md, "Barter option ids") from the
// staging and inspection moves a human_action also reports.
func isCounterMove(action string) bool {
	for _, p := range []string{"sell_all", "buy_all", "accept_wholesale", "offer_", "add_markup_", "demand_discount_", "blackmail", "threaten", "dismiss"} {
		if strings.HasPrefix(action, p) {
			return true
		}
	}
	return false
}

func (s *Strategist) Decision(session string, req *protocol.DecideRequest, _ protocol.DecideResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.runFor(session, req.GameDay); r != nil && !s.closed {
		r.day(req.GameDay).stats.decisions++
		s.decisionWindfalls(session, r, req)
	}
}

func (s *Strategist) Outcome(session string, oc protocol.Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.runFor(session, oc.GameDay); r != nil && !s.closed {
		r.day(oc.GameDay).stats.addOutcome(oc.DecisionID, oc.Result)
		s.outcomeWindfalls(session, r, oc)
	}
}

// CurrentPlan is the latest plan for session's run and the day it was written for. When
// today's plan failed or is still being written, it is an earlier day's.
func (s *Strategist) CurrentPlan(session string) (text string, day int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.sessions[session]
	if r == nil || r.plan == "" {
		return "", 0, false
	}
	return r.plan, r.planDay, true
}

// CurrentDirectives is CurrentPlan's plan as directives (docs/PROTOCOL.md, "Directives"),
// as JSON, and the day they were written for.
func (s *Strategist) CurrentDirectives(session string) (directives json.RawMessage, day int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.sessions[session]
	if r == nil || r.plan == "" || r.directives == nil {
		return nil, 0, false
	}
	raw, err := json.Marshal(r.directives)
	if err != nil {
		return nil, 0, false
	}
	return raw, r.planDay, true
}

// Scorecard is the latest evening review for session's run: one line per goal, then the
// day's numbers.
func (s *Strategist) Scorecard(session string) (text string, day int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.sessions[session]
	if r == nil || r.reviewDay == 0 {
		return "", 0, false
	}
	d := r.days[r.reviewDay]
	if d == nil {
		return "", 0, false
	}
	text = d.scorecard
	if d.facts != "" {
		if text != "" {
			text += "\n"
		}
		text += "Numbers: " + d.facts
	}
	return text, d.n, text != ""
}

// Day returns the plan, scorecard and facts recorded for one day of session's run, for
// reflection. Facts are computed on the spot when the day has not been reviewed yet.
func (s *Strategist) Day(session string, n int) (DayPlan, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.sessions[session]
	if r == nil || r.days[n] == nil {
		return DayPlan{}, false
	}
	return r.days[n].dayPlan(), true
}

func (d *dayState) dayPlan() DayPlan {
	f := d.facts
	if f == "" {
		f = facts(d.morning, d.close, d.statsWithCustomers())
	}
	return DayPlan{Day: d.n, Plan: d.plan, Model: d.model, Scorecard: d.scorecard, Facts: f}
}

func (d *dayState) statsWithCustomers() *dayStats {
	d.stats.customers = max(d.observed, d.arrived)
	return d.stats
}

func newRun(day int) *run {
	return &run{lastDay: day, days: map[int]*dayState{}}
}

func (r *run) day(n int) *dayState {
	d := r.days[n]
	if d == nil {
		d = &dayState{n: n, stats: newDayStats()}
		r.days[n] = d
		for k := range r.days {
			if k < n-keepDays {
				delete(r.days, k)
			}
		}
	}
	return d
}

// yesterday is the latest day before n with a plan or a morning snapshot.
func (r *run) yesterday(n int) *dayState {
	var best *dayState
	for k, d := range r.days {
		if k < n && (d.plan != "" || d.morning != nil) && (best == nil || k > best.n) {
			best = d
		}
	}
	return best
}

func continues(lastDay, day int) bool {
	return lastDay <= 0 || (day >= lastDay-reloadDays && day <= lastDay+maxGapDays)
}

// runFor returns session's run, binding a new session to the latest run when its day
// continues it, and opening a new run on a day jump. Callers hold s.mu.
func (s *Strategist) runFor(session string, day int) *run {
	r := s.sessions[session]
	switch {
	case r != nil && day > 0 && !continues(r.lastDay, day):
		r = newRun(day)
		s.bind(session, r)
	case r == nil && day <= 0:
		return nil
	case r == nil && s.latest != nil && continues(s.latest.lastDay, day):
		r = s.latest
		s.bind(session, r)
	case r == nil:
		r = newRun(day)
		s.bind(session, r)
	}
	if day > r.lastDay {
		r.lastDay = day
	}
	return r
}

func (s *Strategist) bind(session string, r *run) {
	if _, ok := s.sessions[session]; !ok {
		s.order = append(s.order, session)
		if len(s.order) > maxSessions {
			delete(s.sessions, s.order[0])
			delete(s.personas, s.order[0])
			s.order = s.order[1:]
		}
	}
	s.sessions[session] = r
	s.latest = r
}

// enqueue runs job after every job queued before it for r. Callers hold s.mu.
func (s *Strategist) enqueue(r *run, job func(ctx context.Context)) {
	prev := r.tail
	done := make(chan struct{})
	r.tail = done
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer close(done)
		if prev != nil {
			select {
			case <-prev:
			case <-s.ctx.Done():
				return
			}
		}
		if s.ctx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.ctx, s.opt.Timeout)
		defer cancel()
		job(ctx)
	}()
}

func (s *Strategist) planDay(ctx context.Context, session string, r *run, n, gen int) {
	s.mu.Lock()
	d := r.days[n]
	if d == nil || d.planGen != gen || s.opt.LLM == nil {
		s.mu.Unlock()
		return
	}
	in := PlanInput{Day: n, Snapshot: d.morningRaw, Persona: s.personaFor(session, r), Augs: r.augs.trackFor(d.morning, n), WineIdle: r.wine.Idle(n), Delta: s.opt.DeltaDirectives, BaseDirectives: s.opt.BaseDirectives, Route: s.opt.Route, Exemplars: s.opt.Exemplars.Similar(d.morningRaw, n, 3)}
	if y := r.yesterday(n); y != nil {
		yp := y.dayPlan()
		in.Yesterday = &yp
	}
	if w := r.days[n-7]; w != nil && w.morning != nil && w.morning.NetWorth != nil && d.morning != nil && d.morning.NetWorth != nil {
		g := *d.morning.NetWorth - *w.morning.NetWorth
		in.WeekGain = &g
	}
	s.mu.Unlock()

	in.Lessons = s.lessons(ctx, session, n)
	if s.opt.Audience != nil {
		in.Audience = s.opt.Audience(n)
	}
	start := time.Now()
	draft, err := DraftPlanConversation(ctx, s.opt.LLM, in, s.opt.MaxTokens)
	if err != nil {
		s.log.Warn("day plan failed; keeping the previous plan", "day", n, "kept_day", r.planDay, "err", err)
		return
	}
	planMS := time.Since(start).Milliseconds()
	// Directives are published with the plan, so whoever waits for the plan finds them.
	dr := DraftDirectives(ctx, s.opt.LLM, draft)

	s.mu.Lock()
	defer s.mu.Unlock()
	if d.planGen != gen {
		return
	}
	s.overlayForks(r, n, &dr.Directives)
	d.plan, d.model, d.directives = draft.Plan, draft.Model, &dr.Directives
	if n >= r.planDay || r.plan == "" {
		r.plan, r.planModel, r.planDay, r.directives = draft.Plan, draft.Model, n, &dr.Directives
	}
	s.log.Info("day plan written", "day", n, "bytes", len(draft.Plan), "ms", planMS, "directives_ms", time.Since(start).Milliseconds()-planMS, "plan", draft.Plan)
	if dr.Err != nil {
		s.log.Warn("model directives unusable; derived from the plan", "day", n, "err", dr.Err)
	}
	if b, err := json.Marshal(dr.Directives); err == nil {
		why, _ := json.Marshal(dr.Why)
		s.log.Info("directives", "day", n, "json", string(b), "why", string(why))
	}
	if len(dr.Fixes) > 0 {
		s.log.Info("directives fixed", "day", n, "from_model", dr.FromModel, "fixes", dr.Fixes)
	}
}

func (s *Strategist) reviewDay(ctx context.Context, r *run, n int) {
	s.mu.Lock()
	d := r.days[n]
	if d == nil {
		s.mu.Unlock()
		return
	}
	d.facts = facts(d.morning, d.close, d.statsWithCustomers())
	d.scorecard = ""
	if n >= r.reviewDay {
		r.reviewDay = n
	}
	plan, f := d.plan, d.facts
	s.mu.Unlock()
	if plan == "" || s.opt.LLM == nil {
		return
	}
	sc, err := DraftScorecard(ctx, s.opt.LLM, n, plan, f, s.opt.MaxTokens)
	if err != nil {
		s.log.Warn("scorecard failed; the day keeps its facts only", "day", n, "err", err)
		return
	}
	s.mu.Lock()
	d.scorecard = sc
	s.mu.Unlock()
}

func (s *Strategist) lessons(ctx context.Context, session string, n int) string {
	if s.opt.Recall == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(ctx, s.opt.RecallTimeout)
	defer cancel()
	return s.opt.Recall.Brief(ctx, &protocol.DecideRequest{SessionID: session, Kind: "day_plan", GameDay: n}, maxLessonsBytes)
}
