package memory

import "sync"

// A run is one save/playthrough. The game gives no save id to a network-only mod, so the
// run is inferred from what the bridge sees:
//
//   - new_game_started always opens a new run and binds the session to it.
//   - A session (one hello, i.e. one game load) is bound to a run by the first record that
//     carries a known game_day (> 0). It continues the most recent run when that day is
//     within [last_day-ReloadDays, last_day+MaxGapDays] of the run's last seen day
//     (reloading the morning save, or a few days played with the bridge down); otherwise
//     it opens a new run.
//   - Within a bound session the same window applies: a day jump outside it opens a new run.
//
// Known limit: loading a different save whose day happens to be within the window of the
// last run is taken as the same run.
const (
	ReloadDays = 2
	MaxGapDays = 3
	// maxSessions bounds the session→run map if a mod re-hellos in a loop.
	maxSessions = 256
)

const (
	reasonFirst   = "first"
	reasonNewGame = "new_game"
	reasonDayJump = "day_jump"
)

type run struct {
	ID          int64
	Reason      string
	Scenario    string
	FirstDay    int
	LastDay     int
	CarriedFrom int64
}

type tracker struct {
	carry bool

	mu       sync.Mutex
	nextID   int64
	latest   *run
	sessions map[string]*run
	// days is the latest known day each bound session reported.
	days  map[string]int
	order []string
}

func newTracker(latest *run, carry bool) *tracker {
	t := &tracker{carry: carry, nextID: 1, sessions: map[string]*run{}, days: map[string]int{}}
	if latest != nil {
		t.latest = latest
		t.nextID = latest.ID + 1
	}
	return t
}

func continues(lastDay, day int) bool {
	return lastDay <= 0 || (day >= lastDay-ReloadDays && day <= lastDay+MaxGapDays)
}

// resolve returns a snapshot of the run a record from session on day belongs to, opening
// and binding runs as the heuristic says.
func (t *tracker) resolve(session string, day int, newGame bool, scenario string) run {
	t.mu.Lock()
	defer t.mu.Unlock()
	if newGame {
		r := t.open(reasonNewGame, scenario, day)
		t.bind(session, r)
		t.see(session, day)
		return *r
	}
	r, bound := t.sessions[session]
	switch {
	case bound && day > 0 && !continues(r.LastDay, day):
		r = t.open(reasonDayJump, "", day)
		t.bind(session, r)
	case bound:
	case day <= 0:
		// Unknown day: attribute to the latest run without binding, so a later dated
		// record still decides.
		if t.latest == nil {
			t.latest = t.open(reasonFirst, "", 0)
		}
		return *t.latest
	case t.latest != nil && continues(t.latest.LastDay, day):
		r = t.latest
		t.bind(session, r)
	default:
		reason := reasonDayJump
		if t.latest == nil {
			reason = reasonFirst
		}
		r = t.open(reason, "", day)
		t.bind(session, r)
	}
	if day > r.LastDay {
		r.LastDay = day
	}
	t.see(session, day)
	return *r
}

func (t *tracker) see(session string, day int) {
	if day > 0 {
		t.days[session] = day
	}
}

// current returns the run session is bound to and the latest day it reported, else the
// run's last day. ok is false for a session not bound to a run.
func (t *tracker) current(session string) (r run, day int, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.sessions[session]
	if !ok {
		return run{}, 0, false
	}
	day, seen := t.days[session]
	if !seen {
		day = b.LastDay
	}
	return *b, day, true
}

// peek is resolve without side effects. ok is false when the record would open a new run,
// in which case carriedFrom is the run it would inherit lessons from.
func (t *tracker) peek(session string, day int) (id int64, carriedFrom int64, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if r, bound := t.sessions[session]; bound {
		if day <= 0 || continues(r.LastDay, day) {
			return r.ID, r.CarriedFrom, true
		}
	} else if t.latest != nil && (day <= 0 || continues(t.latest.LastDay, day)) {
		return t.latest.ID, t.latest.CarriedFrom, true
	}
	if t.carry && t.latest != nil {
		carriedFrom = t.latest.ID
	}
	return 0, carriedFrom, false
}

func (t *tracker) open(reason, scenario string, day int) *run {
	r := &run{ID: t.nextID, Reason: reason, Scenario: scenario, FirstDay: day, LastDay: day}
	if t.carry && t.latest != nil {
		r.CarriedFrom = t.latest.ID
	}
	t.nextID++
	t.latest = r
	return r
}

func (t *tracker) bind(session string, r *run) {
	if _, ok := t.sessions[session]; !ok {
		if len(t.order) >= maxSessions {
			delete(t.sessions, t.order[0])
			delete(t.days, t.order[0])
			t.order = t.order[1:]
		}
		t.order = append(t.order, session)
	}
	t.sessions[session] = r
}
