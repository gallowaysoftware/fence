// Package server implements the HTTP side of docs/PROTOCOL.md.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	DefaultMaxBodyBytes = 1 << 20
	// maxTrackedDecisions bounds per-session memory of decision ids used to flag
	// outcomes that match no decision.
	maxTrackedDecisions = 4096
	// summaryTimeout keeps /v1/summary fast: the overlay polls it and a slow database
	// must cost the numbers, not the answer.
	summaryTimeout = 200 * time.Millisecond
	// DefaultRecorderQueue bounds each recorder's backlog; past it records are dropped
	// for that recorder only.
	DefaultRecorderQueue = 1024
)

type Config struct {
	Mode   protocol.Mode
	Policy policy.Policy
	// Model is reported by /v1/summary; empty for the heuristic.
	Model string
	Log   *decisionlog.Logger // nil disables the decision log
	// Recorders see every hello, event, decision and outcome (memory, the strategist).
	// Each gets its own queue and goroutine, so a slow or panicking one costs only its
	// own records.
	Recorders []Recorder
	// RecorderQueue bounds each recorder's backlog; 0 means DefaultRecorderQueue.
	RecorderQueue int
	// Summarizer, if set, supplies the day view for /v1/summary (memory).
	Summarizer Summarizer
	// Planner, if set, supplies the day's plan for /v1/summary (the strategist).
	Planner Planner
	Logger  *slog.Logger
	// MaxBodyBytes caps request bodies; larger ones get 413.
	MaxBodyBytes int64
	Now          func() time.Time
	NewSessionID func() string
}

// Recorder receives records in the order the server saw them, off the request path.
// Decisions reach it as the policy view and events as the recorder view: hidden values
// never leave the decision log.
type Recorder interface {
	Hello(session string, req protocol.HelloRequest)
	Event(session string, ev protocol.Event)
	Decision(session string, req *protocol.DecideRequest, resp protocol.DecideResponse)
	Outcome(session string, oc protocol.Outcome)
}

// Summarizer fills Day, Today, Lessons and LessonsDay; the server sets the rest. It must
// honour ctx. On error the server still uses whatever it returned.
type Summarizer interface {
	Summary(ctx context.Context, session string) (protocol.SummaryResponse, error)
}

// Planner supplies the strategist's current plan for a session. It is called on the
// /v1/summary path under the same deadline as the Summarizer.
type Planner interface {
	CurrentPlan(session string) (text string, day int, ok bool)
}

type Server struct {
	cfg       Config
	log       *slog.Logger
	recorders []*recorderQueue

	mu       sync.Mutex
	sessions map[string]*session
}

type session struct {
	id   string
	mode protocol.Mode

	mu        sync.Mutex
	lastSeq   int64
	seenSeq   bool
	decisions map[string]struct{}
	order     []string
}

func New(cfg Config) *Server {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.DiscardHandler)
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = DefaultMaxBodyBytes
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewSessionID == nil {
		cfg.NewSessionID = randomSessionID
	}
	if cfg.Mode == "" {
		cfg.Mode = protocol.ModeObserve
	}
	if cfg.Policy == nil {
		cfg.Policy = policy.Heuristic{}
	}
	if cfg.RecorderQueue <= 0 {
		cfg.RecorderQueue = DefaultRecorderQueue
	}
	s := &Server{cfg: cfg, log: cfg.Logger, sessions: map[string]*session{}}
	for i, r := range cfg.Recorders {
		if r != nil {
			s.recorders = append(s.recorders, startRecorder(r, i, cfg.RecorderQueue, cfg.Logger))
		}
	}
	return s
}

// Close drains every recorder's queue. Records sent after Close are dropped.
func (s *Server) Close() {
	for _, q := range s.recorders {
		q.close()
	}
}

// RecordsDropped is how many records recorders lost to a full queue or Close.
func (s *Server) RecordsDropped() int64 {
	var n int64
	for _, q := range s.recorders {
		n += q.dropped.Load()
	}
	return n
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("POST /v1/hello", s.hello)
	mux.HandleFunc("POST /v1/event", s.event)
	mux.HandleFunc("POST /v1/decide", s.decide)
	mux.HandleFunc("POST /v1/outcome", s.outcome)
	mux.HandleFunc("GET /v1/summary", s.summary)
	return mux
}

func randomSessionID() string {
	var b [8]byte
	rand.Read(b[:])
	return "s-" + hex.EncodeToString(b[:])
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, protocol.Health{OK: true, Protocol: protocol.Version})
}

func (s *Server) hello(w http.ResponseWriter, r *http.Request) {
	var req protocol.HelloRequest
	if !s.decode(w, r, &req) {
		return
	}
	if req.Protocol != protocol.Version {
		s.log.Warn("hello with unsupported protocol", "protocol", req.Protocol, "mod_version", req.ModVersion)
		writeJSON(w, http.StatusConflict, protocol.ErrorResponse{
			Error:    fmt.Sprintf("protocol %d not supported", req.Protocol),
			Protocol: protocol.Version,
		})
		return
	}
	sess := &session{id: s.cfg.NewSessionID(), mode: s.cfg.Mode, decisions: map[string]struct{}{}}
	s.mu.Lock()
	s.sessions[sess.id] = sess
	s.mu.Unlock()
	s.log.Info("session started", "session", sess.id, "mode", sess.mode, "mod_version", req.ModVersion,
		"game", req.Game.Version, "compat_ok", req.Compat.OK, "missing", req.Compat.Missing)
	s.record(func(r Recorder) { r.Hello(sess.id, req) })
	s.cfg.Log.Write(sess.id, helloRecord{header: s.header("hello", sess.id), Mode: sess.mode, Policy: s.cfg.Policy.Name(), Hello: req})
	writeJSON(w, http.StatusOK, protocol.HelloResponse{SessionID: sess.id, Protocol: protocol.Version, Mode: sess.mode})
}

func (s *Server) event(w http.ResponseWriter, r *http.Request) {
	var ev protocol.Event
	if !s.decode(w, r, &ev) {
		return
	}
	if err := ev.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sess := s.session(w, ev.SessionID)
	if sess == nil {
		return
	}
	rec := eventRecord{header: s.header("event", sess.id), Event: ev}
	rec.Missed, rec.OutOfOrder = sess.observeSeq(ev.Seq)
	if rec.Missed > 0 {
		s.log.Warn("event seq gap", "session", sess.id, "seq", ev.Seq, "missed", rec.Missed)
	}
	if rec.OutOfOrder {
		s.log.Warn("event seq not increasing", "session", sess.id, "seq", ev.Seq)
	}
	view := ev.RecorderView()
	s.record(func(r Recorder) { r.Event(sess.id, view) })
	s.cfg.Log.Write(sess.id, rec)
	w.WriteHeader(http.StatusNoContent)
}

// observeSeq returns how many events were skipped before seq. Sequences may start at 0
// or 1; a first event above 1 counts the ones before it as missed.
func (sess *session) observeSeq(seq int64) (missed int64, outOfOrder bool) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	switch {
	case !sess.seenSeq:
		missed = max(seq-1, 0)
	case seq <= sess.lastSeq:
		return 0, true
	default:
		missed = seq - sess.lastSeq - 1
	}
	sess.seenSeq, sess.lastSeq = true, seq
	return missed, false
}

func (s *Server) outcome(w http.ResponseWriter, r *http.Request) {
	var oc protocol.Outcome
	if !s.decode(w, r, &oc) {
		return
	}
	if err := oc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sess := s.session(w, oc.SessionID)
	if sess == nil {
		return
	}
	sess.mu.Lock()
	_, known := sess.decisions[oc.DecisionID]
	sess.mu.Unlock()
	if !known {
		s.log.Warn("outcome for unknown decision", "session", sess.id, "decision_id", oc.DecisionID)
	}
	s.record(func(r Recorder) { r.Outcome(sess.id, oc) })
	s.cfg.Log.Write(sess.id, outcomeRecord{header: s.header("outcome", sess.id), DecisionID: oc.DecisionID, KnownDecision: known, Outcome: oc})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) decide(w http.ResponseWriter, r *http.Request) {
	start := s.cfg.Now()
	var req protocol.DecideRequest
	if !s.decode(w, r, &req) {
		return
	}
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	sess := s.session(w, req.SessionID)
	if sess == nil {
		return
	}
	sess.trackDecision(req.DecisionID)

	deadline := start.Add(time.Duration(req.DeadlineMS)*time.Millisecond - margin(req.DeadlineMS))
	ctx, cancel := context.WithDeadline(r.Context(), deadline)
	defer cancel()

	var d policy.Decision
	if sess.mode == protocol.ModeObserve {
		d = policy.Fallback("bridge is in observe mode")
	} else {
		d = s.runPolicy(ctx, req.PolicyView())
	}
	// CheckChoice applies the kind's safety rules (e.g. no stolen goods offered to security)
	// to every policy, not only those that call it themselves.
	if err := errors.Join(policy.ValidateChoice(req.Options, d.Choice), policy.CheckChoice(&req, d.Choice)); err != nil {
		s.log.Error("policy returned an invalid choice", "policy", s.cfg.Policy.Name(), "err", err)
		d = policy.Fallback("policy returned an invalid choice: " + err.Error())
	}
	resp := protocol.DecideResponse{DecisionID: req.DecisionID, Choice: d.Choice, Rationale: d.Rationale, Source: d.Source,
		OptionNotes: displayNotes(req.Options, d.Notes)}
	latency := s.cfg.Now().Sub(start)
	// Recorded before answering so an outcome the mod sends right after cannot overtake it.
	s.record(func(r Recorder) { r.Decision(sess.id, req.PolicyView(), resp) })
	writeJSON(w, http.StatusOK, resp)

	s.log.Info("decision", "session", sess.id, "decision_id", req.DecisionID, "kind", req.Kind,
		"option", d.Choice.Option, "source", d.Source, "latency_ms", latency.Milliseconds())
	s.cfg.Log.Write(sess.id, decisionRecord{
		header: s.header("decision", sess.id), DecisionID: req.DecisionID, Mode: sess.mode,
		Policy: s.cfg.Policy.Name(), Request: req, Trace: d.Trace, Response: resp, LatencyMS: latency.Milliseconds(),
	})
}

// displayNotes keeps the notes for offered options only, each within MaxOptionNote.
func displayNotes(opts []protocol.Option, notes map[string]string) map[string]string {
	var out map[string]string
	for _, o := range opts {
		n := policy.ClipNote(notes[o.ID])
		if n == "" {
			continue
		}
		if out == nil {
			out = make(map[string]string, len(notes))
		}
		out[o.ID] = n
	}
	return out
}

// margin keeps the answer inside deadline_ms with room for encoding and the loopback
// hop, so the mod never has to fire its own timeout.
func margin(deadlineMS int64) time.Duration {
	return min(max(time.Duration(deadlineMS)*time.Millisecond/10, 100*time.Millisecond), time.Second)
}

// runPolicy returns at the deadline even if the policy ignores ctx, and turns a panic
// into a defer: a bug in a policy must not cost the player control of the game.
func (s *Server) runPolicy(ctx context.Context, req *protocol.DecideRequest) policy.Decision {
	ch := make(chan policy.Decision, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				s.log.Error("policy panicked", "policy", s.cfg.Policy.Name(), "panic", p)
				ch <- s.fallback(req, fmt.Sprintf("policy panicked: %v", p))
			}
		}()
		ch <- s.cfg.Policy.Decide(ctx, req)
	}()
	select {
	case d := <-ch:
		return d
	case <-ctx.Done():
		return s.fallback(req, "deadline reached before the policy answered")
	}
}

// fallback defers but keeps the option notes: they need no model, so they still help
// the player when the policy failed.
func (s *Server) fallback(req *protocol.DecideRequest, reason string) policy.Decision {
	d := policy.Fallback(reason)
	func() {
		defer func() { recover() }()
		d.Notes = policy.OptionNotes(req)
	}()
	return d
}

func (s *Server) summary(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("session_id")
	if id == "" {
		writeError(w, http.StatusBadRequest, errors.New("session_id is required"))
		return
	}
	sess := s.session(w, id)
	if sess == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), summaryTimeout)
	defer cancel()
	// In parallel, so a slow database cannot cost the plan.
	planc := make(chan plan, 1)
	go func() { planc <- s.currentPlan(ctx, sess.id) }()
	var day protocol.SummaryResponse
	if s.cfg.Summarizer != nil {
		day = s.daySummary(ctx, sess.id)
	}
	p := <-planc
	resp := protocol.SummaryResponse{
		SessionID: sess.id, Mode: sess.mode, Policy: s.cfg.Policy.Name(), Model: s.cfg.Model,
		Day: day.Day, Today: day.Today, Lessons: day.Lessons, LessonsDay: day.LessonsDay,
		Plan: p.text, PlanDay: p.day,
	}
	if resp.Today.RepDelta == nil {
		resp.Today.RepDelta = map[string]float64{}
	}
	writeJSON(w, http.StatusOK, resp)
}

// daySummary returns at ctx's deadline even if the summarizer ignores it; a late or
// failed summary is zeros, never an error to the mod.
func (s *Server) daySummary(ctx context.Context, session string) protocol.SummaryResponse {
	v, err := bounded(ctx, func() (protocol.SummaryResponse, error) { return s.cfg.Summarizer.Summary(ctx, session) })
	if err != nil {
		s.log.Warn("summary incomplete", "session", session, "err", err)
	}
	return v
}

type plan struct {
	text string
	day  int
}

// currentPlan is the planner's plan cut to MaxPlanBytes; none when it is late, panics or
// has nothing.
func (s *Server) currentPlan(ctx context.Context, session string) plan {
	if s.cfg.Planner == nil {
		return plan{}
	}
	p, err := bounded(ctx, func() (plan, error) {
		text, day, ok := s.cfg.Planner.CurrentPlan(session)
		if !ok {
			return plan{}, nil
		}
		return plan{text: clipText(strings.TrimSpace(text), protocol.MaxPlanBytes), day: max(day, 0)}, nil
	})
	if err != nil {
		s.log.Warn("plan unavailable", "session", session, "err", err)
		return plan{}
	}
	if p.text == "" {
		return plan{}
	}
	return p
}

// bounded runs f, returning at ctx's deadline even if f ignores it; a panic in f becomes
// an error. On an error from f its value is still returned.
func bounded[T any](ctx context.Context, f func() (T, error)) (T, error) {
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				ch <- result{err: fmt.Errorf("panicked: %v", p)}
			}
		}()
		v, err := f()
		ch <- result{v, err}
	}()
	select {
	case res := <-ch:
		return res.v, res.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}

// clipText cuts s to at most n bytes, at a line end when there is one, never inside a
// UTF-8 sequence.
func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		return strings.TrimRight(cut[:i], "\n")
	}
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// record queues f for every recorder without waiting on any of them.
func (s *Server) record(f func(Recorder)) {
	for _, q := range s.recorders {
		q.send(f)
	}
}

type recorderQueue struct {
	r     Recorder
	index int
	log   *slog.Logger
	ch    chan func(Recorder)
	done  chan struct{}

	mu      sync.RWMutex
	closed  bool
	dropped atomic.Int64
}

func startRecorder(r Recorder, index, size int, log *slog.Logger) *recorderQueue {
	q := &recorderQueue{r: r, index: index, log: log, ch: make(chan func(Recorder), size), done: make(chan struct{})}
	go q.run()
	return q
}

func (q *recorderQueue) send(f func(Recorder)) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	if !q.closed {
		select {
		case q.ch <- f:
			return
		default:
		}
	}
	if n := q.dropped.Add(1); n == 1 || n%100 == 0 {
		q.log.Warn("recorder queue full; dropping records", "recorder", q.index, "type", fmt.Sprintf("%T", q.r), "dropped", n)
	}
}

func (q *recorderQueue) run() {
	defer close(q.done)
	for f := range q.ch {
		q.call(f)
	}
}

// call costs a panicking recorder only the record.
func (q *recorderQueue) call(f func(Recorder)) {
	defer func() {
		if p := recover(); p != nil {
			q.log.Error("recorder panicked", "recorder", q.index, "type", fmt.Sprintf("%T", q.r), "panic", p)
		}
	}()
	f(q.r)
}

func (q *recorderQueue) close() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	close(q.ch)
	q.mu.Unlock()
	<-q.done
}

func (sess *session) trackDecision(id string) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if _, ok := sess.decisions[id]; ok {
		return
	}
	if len(sess.order) >= maxTrackedDecisions {
		delete(sess.decisions, sess.order[0])
		sess.order = sess.order[1:]
	}
	sess.decisions[id] = struct{}{}
	sess.order = append(sess.order, id)
}

// session answers 404 for an unknown id: after a bridge restart the mod must re-hello.
func (s *Server) session(w http.ResponseWriter, id string) *session {
	s.mu.Lock()
	sess := s.sessions[id]
	s.mu.Unlock()
	if sess == nil {
		writeJSON(w, http.StatusNotFound, protocol.ErrorResponse{Error: "unknown session; send hello again"})
	}
	return sess
}

func (s *Server) decode(w http.ResponseWriter, r *http.Request, v any) bool {
	body := http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
	dec := json.NewDecoder(body)
	err := dec.Decode(v)
	if err == nil && dec.More() {
		err = errors.New("trailing data after JSON body")
	}
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("body over %d bytes", tooBig.Limit))
		return false
	}
	if errors.Is(err, io.EOF) {
		err = errors.New("empty body")
	}
	writeError(w, http.StatusBadRequest, err)
	return false
}

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, protocol.ErrorResponse{Error: err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
