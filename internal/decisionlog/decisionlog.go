// Package decisionlog appends one JSONL file per session: hello, events, decisions and
// outcomes, joinable by decision_id. It is the raw source of truth for recorded play:
// append-only, flushed whenever the queue empties and fsynced every SyncInterval. It never
// blocks or fails the caller: records are queued and written by one goroutine, and
// dropped (and counted) when the queue is full; the session's file then gets a
// "dropped" record so the loss shows up in the gap report.
package decisionlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultQueue = 1024
	// maxOpenFiles bounds descriptors if a misbehaving mod re-hellos in a loop.
	maxOpenFiles = 32
	// SyncInterval bounds how much flushed data a power loss can take.
	SyncInterval = 2 * time.Second
)

// TypeDropped is the record written when records for a session were dropped.
const TypeDropped = "dropped"

type droppedRecord struct {
	Type      string    `json:"type"`
	TS        time.Time `json:"ts"`
	SessionID string    `json:"session_id"`
	Count     int64     `json:"count"`
}

type entry struct {
	session string
	rec     any
}

type Logger struct {
	dir   string
	log   *slog.Logger
	queue chan entry
	done  chan struct{}

	mu     sync.RWMutex
	closed bool

	dropped atomic.Int64
	failed  atomic.Int64

	// lost counts dropped records per session until the writer marks them in the file.
	lostMu sync.Mutex
	lost   map[string]int64

	syncEvery time.Duration
	files     map[string]*bufio.Writer
	fds       map[string]*os.File
	dirty     map[string]bool
	order     []string
}

// New creates dir if needed. A failure here is the only error the logger ever returns.
func New(dir string, queue int, log *slog.Logger) (*Logger, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := newLogger(dir, queue, log)
	go l.run()
	return l, nil
}

func newLogger(dir string, queue int, log *slog.Logger) *Logger {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Logger{
		dir:       dir,
		log:       log,
		queue:     make(chan entry, queue),
		done:      make(chan struct{}),
		lost:      map[string]int64{},
		syncEvery: SyncInterval,
		files:     map[string]*bufio.Writer{},
		fds:       map[string]*os.File{},
		dirty:     map[string]bool{},
	}
}

// Write queues rec for the session's file. rec must not be mutated afterwards.
func (l *Logger) Write(session string, rec any) {
	if l == nil {
		return
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		l.dropped.Add(1)
		return
	}
	select {
	case l.queue <- entry{session, rec}:
	default:
		l.lostMu.Lock()
		l.lost[session]++
		l.lostMu.Unlock()
		if n := l.dropped.Add(1); n == 1 || n%100 == 0 {
			l.log.Warn("decision log queue full; dropping records", "dropped", n)
		}
	}
}

// Dropped is how many records were never written: queue full, logger closed, or a
// write error.
func (l *Logger) Dropped() int64 { return l.dropped.Load() + l.failed.Load() }

// Close drains the queue, flushes and closes every file.
func (l *Logger) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	close(l.queue)
	l.mu.Unlock()
	<-l.done
	return nil
}

func (l *Logger) run() {
	defer close(l.done)
	tick := time.NewTicker(l.syncEvery)
	defer tick.Stop()
	for {
		select {
		case e, ok := <-l.queue:
			if !ok {
				l.syncAll()
				for _, f := range l.fds {
					f.Close()
				}
				return
			}
			l.markLost(e.session)
			l.write(e)
			if len(l.queue) == 0 {
				l.flushAll()
			}
		case <-tick.C:
			l.syncAll()
		}
	}
}

// markLost writes a dropped record before the session's next record, if any were lost.
func (l *Logger) markLost(session string) {
	l.lostMu.Lock()
	n := l.lost[session]
	delete(l.lost, session)
	l.lostMu.Unlock()
	if n > 0 {
		l.write(entry{session, droppedRecord{Type: TypeDropped, TS: time.Now().UTC(), SessionID: session, Count: n}})
	}
}

func (l *Logger) write(e entry) {
	b, err := json.Marshal(e.rec)
	if err != nil {
		l.fail("marshal", e.session, err)
		return
	}
	w, err := l.file(e.session)
	if err != nil {
		l.fail("open", e.session, err)
		return
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		l.fail("write", e.session, err)
	}
	l.dirty[e.session] = true
}

func (l *Logger) fail(op, session string, err error) {
	if n := l.failed.Add(1); n == 1 || n%100 == 0 {
		l.log.Error("decision log "+op+" failed", "session", session, "err", err, "failed", n)
	}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// Path is where a session's records go.
func (l *Logger) Path(session string) string {
	name := unsafeName.ReplaceAllString(session, "_")
	if name == "" || name[0] == '.' {
		name = "_" + name
	}
	return filepath.Join(l.dir, name+".jsonl")
}

func (l *Logger) file(session string) (*bufio.Writer, error) {
	if w, ok := l.files[session]; ok {
		return w, nil
	}
	if len(l.order) >= maxOpenFiles {
		oldest := l.order[0]
		l.order = l.order[1:]
		l.files[oldest].Flush()
		l.fds[oldest].Sync()
		l.fds[oldest].Close()
		delete(l.files, oldest)
		delete(l.fds, oldest)
		delete(l.dirty, oldest)
	}
	f, err := os.OpenFile(l.Path(session), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", l.Path(session), err)
	}
	w := bufio.NewWriter(f)
	l.files[session], l.fds[session] = w, f
	l.order = append(l.order, session)
	return w, nil
}

func (l *Logger) flushAll() {
	for s, w := range l.files {
		if err := w.Flush(); err != nil {
			l.fail("flush", s, err)
		}
	}
}

// syncAll flushes and fsyncs every file written since the last sync.
func (l *Logger) syncAll() {
	l.flushAll()
	for s := range l.dirty {
		if f, ok := l.fds[s]; ok {
			if err := f.Sync(); err != nil {
				l.fail("sync", s, err)
			}
		}
		delete(l.dirty, s)
	}
}
