package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	DefaultQueue = 1024
	// reflectBacklog bounds pending end-of-day reflections; one runs at a time.
	reflectBacklog          = 4
	DefaultReflectTimeout   = 3 * time.Minute
	DefaultReflectMaxTokens = 4096
)

type Options struct {
	// Queue bounds pending writes; a record that does not fit is dropped and counted.
	Queue  int
	Logger *slog.Logger
	// LLM rewrites the lessons at the end of each in-game day; nil disables reflection.
	LLM              llm.Client
	ReflectMaxTokens int
	ReflectTimeout   time.Duration
	// CarryLessons starts a new run with the previous run's lessons instead of none.
	CarryLessons bool
	Now          func() time.Time
}

// Store is the bridge's memory. It records what the server sees (it satisfies
// server.Recorder), derives the price ledger and per-run facts from visible data and
// outcomes, and answers Recall.Brief. Recording never blocks: writes go through a
// bounded queue drained by one goroutine.
type Store struct {
	db   *sql.DB
	opt  Options
	log  *slog.Logger
	runs *tracker

	mu     sync.RWMutex
	closed bool
	queue  chan op
	done   chan struct{}

	reflectQ    chan reflectJob
	reflectDone chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc

	dropped atomic.Int64
	failed  atomic.Int64

	// Owned by the writer goroutine.
	knownRuns  map[int64]bool
	sessionRun map[string]int64
}

type opKind int

const (
	opHello opKind = iota
	opEvent
	opDecision
	opOutcome
	opFlush
)

type op struct {
	kind     opKind
	session  string
	run      run
	at       time.Time
	hello    protocol.HelloRequest
	event    protocol.Event
	decision decisionOp
	outcome  protocol.Outcome
	flushed  chan struct{}
}

type decisionOp struct {
	req  *protocol.DecideRequest
	resp protocol.DecideResponse
}

// Open opens (creating and migrating if needed) the database at path and starts the
// writer.
func Open(path string, opt Options) (*Store, error) {
	if opt.Queue <= 0 {
		opt.Queue = DefaultQueue
	}
	if opt.Logger == nil {
		opt.Logger = slog.New(slog.DiscardHandler)
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.ReflectTimeout <= 0 {
		opt.ReflectTimeout = DefaultReflectTimeout
	}
	if opt.ReflectMaxTokens <= 0 {
		opt.ReflectMaxTokens = DefaultReflectMaxTokens
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("memory db %s: %w", path, err)
	}
	latest, err := loadLatestRun(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{
		db:          db,
		opt:         opt,
		log:         opt.Logger,
		runs:        newTracker(latest, opt.CarryLessons),
		queue:       make(chan op, opt.Queue),
		done:        make(chan struct{}),
		reflectQ:    make(chan reflectJob, reflectBacklog),
		reflectDone: make(chan struct{}),
		knownRuns:   map[int64]bool{},
		sessionRun:  map[string]int64{},
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.write()
	go s.reflector()
	return s, nil
}

func loadLatestRun(ctx context.Context, db *sql.DB) (*run, error) {
	var r run
	err := db.QueryRowContext(ctx, `SELECT id, reason, scenario, first_day, last_day, carried_from FROM runs ORDER BY id DESC LIMIT 1`).
		Scan(&r.ID, &r.Reason, &r.Scenario, &r.FirstDay, &r.LastDay, &r.CarriedFrom)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Close stops reflection, drains queued writes and closes the database.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	close(s.queue)
	s.mu.Unlock()
	<-s.done
	s.cancel()
	close(s.reflectQ)
	<-s.reflectDone
	return s.db.Close()
}

// Dropped is how many records were lost: queue full, store closed, or a write error.
func (s *Store) Dropped() int64 { return s.dropped.Load() + s.failed.Load() }

func (s *Store) Hello(session string, req protocol.HelloRequest) {
	s.enqueue(op{kind: opHello, session: session, hello: req})
}

// Event records ev's visible data; hidden values are dropped here even if the caller
// passed them.
func (s *Store) Event(session string, ev protocol.Event) {
	ev = ev.RecorderView()
	var scenario string
	newGame := ev.Kind == "new_game_started"
	if newGame {
		var d struct {
			Scenario string `json:"scenario_id"`
		}
		json.Unmarshal(ev.Data, &d)
		scenario = d.Scenario
	}
	ev.Data = clone(ev.Data)
	s.enqueue(op{kind: opEvent, session: session, run: s.runs.resolve(session, ev.GameDay, newGame, scenario), event: ev})
}

// Decision records a decision. Only the policy view of req is kept: hidden values are
// evaluation data and stay in the decision log.
func (s *Store) Decision(session string, req *protocol.DecideRequest, resp protocol.DecideResponse) {
	v := req.PolicyView()
	v.Context = clone(v.Context)
	s.enqueue(op{kind: opDecision, session: session, run: s.runs.resolve(session, v.GameDay, false, ""), decision: decisionOp{req: v, resp: resp}})
}

func (s *Store) Outcome(session string, oc protocol.Outcome) {
	oc.Result = clone(oc.Result)
	s.enqueue(op{kind: opOutcome, session: session, run: s.runs.resolve(session, oc.GameDay, false, ""), outcome: oc})
}

func clone(b json.RawMessage) json.RawMessage {
	if b == nil {
		return nil
	}
	return append(json.RawMessage(nil), b...)
}

func (s *Store) enqueue(o op) {
	o.at = s.opt.Now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		s.dropped.Add(1)
		return
	}
	select {
	case s.queue <- o:
	default:
		if n := s.dropped.Add(1); n == 1 || n%100 == 0 {
			s.log.Warn("memory queue full; dropping records", "dropped", n)
		}
	}
}

// flush waits until everything queued before it is written. Tests only: it blocks.
func (s *Store) flush() {
	ch := make(chan struct{})
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return
	}
	s.queue <- op{kind: opFlush, flushed: ch}
	s.mu.RUnlock()
	<-ch
}

func (s *Store) write() {
	defer close(s.done)
	for o := range s.queue {
		if o.kind == opFlush {
			close(o.flushed)
			continue
		}
		if err := s.apply(o); err != nil {
			if n := s.failed.Add(1); n == 1 || n%100 == 0 {
				s.log.Error("memory write failed", "session", o.session, "err", err, "failed", n)
			}
		}
	}
}

func (s *Store) apply(o op) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := o.at.Format(time.RFC3339Nano)
	var newRun bool
	if o.kind != opHello {
		if newRun, err = s.ensureRun(ctx, tx, o.run, at); err != nil {
			return err
		}
	}
	var reflect *reflectJob
	switch o.kind {
	case opHello:
		_, err = tx.ExecContext(ctx, `INSERT INTO sessions (id, created_at, mod_version, game_version) VALUES (?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
			o.session, at, o.hello.ModVersion, o.hello.Game.Version)
	case opEvent:
		err = s.bindSession(ctx, tx, o.session, o.run.ID, at)
		if err == nil {
			ev := o.event
			_, err = tx.ExecContext(ctx, `INSERT INTO events (session_id, run_id, seq, kind, game_day, data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				o.session, o.run.ID, ev.Seq, ev.Kind, ev.GameDay, string(ev.Data), at)
		}
		if err == nil {
			err = recordEventFacts(ctx, tx, o.session, o.run.ID, o.event, at)
		}
		if err == nil && o.event.Kind == "going_sleep" && o.event.GameDay > 0 {
			reflect = &reflectJob{run: o.run.ID, carriedFrom: o.run.CarriedFrom, day: o.event.GameDay}
		}
	case opDecision:
		err = s.bindSession(ctx, tx, o.session, o.run.ID, at)
		if err == nil {
			err = recordDecision(ctx, tx, o.session, o.run.ID, o.decision, at)
		}
	case opOutcome:
		err = s.bindSession(ctx, tx, o.session, o.run.ID, at)
		if err == nil {
			err = recordOutcome(ctx, tx, o.session, o.run.ID, o.outcome, at)
		}
	}
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if newRun {
		s.knownRuns[o.run.ID] = true
	}
	if reflect != nil {
		s.queueReflection(*reflect)
	}
	return nil
}

// ensureRun creates the run row the first time the writer sees the run, copying the
// carried-over lessons into it.
func (s *Store) ensureRun(ctx context.Context, tx *sql.Tx, r run, at string) (bool, error) {
	if s.knownRuns[r.ID] {
		_, err := tx.ExecContext(ctx, `UPDATE runs SET last_day = max(last_day, ?) WHERE id = ?`, r.LastDay, r.ID)
		return false, err
	}
	res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO runs (id, created_at, reason, scenario, first_day, last_day, carried_from)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, at, r.Reason, r.Scenario, r.FirstDay, r.LastDay, r.CarriedFrom)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE runs SET last_day = max(last_day, ?) WHERE id = ?`, r.LastDay, r.ID)
	} else if r.CarriedFrom != 0 {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO lessons (run_id, version, game_day, text, source, created_at)
			SELECT ?, 1, ?, text, 'carried from run ' || run_id, ? FROM lessons WHERE run_id = ? ORDER BY version DESC LIMIT 1`,
			r.ID, r.FirstDay, at, r.CarriedFrom)
	}
	return true, err
}

func (s *Store) bindSession(ctx context.Context, tx *sql.Tx, session string, runID int64, at string) error {
	if s.sessionRun[session] == runID {
		return nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO sessions (id, run_id, created_at) VALUES (?, ?, ?) ON CONFLICT (id) DO UPDATE SET run_id = excluded.run_id`,
		session, runID, at)
	if err == nil {
		if len(s.sessionRun) >= maxSessions {
			clear(s.sessionRun)
		}
		s.sessionRun[session] = runID
	}
	return err
}
