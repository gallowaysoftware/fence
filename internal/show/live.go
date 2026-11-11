package show

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Live follows a running bridge without changing it: it tails the newest decision log in
// LogDir and polls the bridge's /v1/summary for the strategist's plan (docs/SHOW.md, "Live
// hook"). A new log file (a new session) starts a new builder.
type Live struct {
	LogDir string
	// Bridge is the bridge's base URL (http://127.0.0.1:7341); "" skips the plan.
	Bridge string
	Player Player
	// Whys, when set, is consulted per customer like a match's gym summary.
	Whys func(day int, seq int64) string
	// Base is the executor's base directives (Builder.Base).
	Base    *protocol.Directives
	Poll    time.Duration
	Publish func(Event)
	Log     *slog.Logger
	Client  *http.Client
}

type tail struct {
	path    string
	session string
	f       *os.File
	partial []byte
	b       *Builder
	planKey string
}

// Run follows until ctx ends.
func (l *Live) Run(ctx context.Context) error {
	if l.Poll <= 0 {
		l.Poll = 500 * time.Millisecond
	}
	if l.Log == nil {
		l.Log = slog.New(slog.DiscardHandler)
	}
	if l.Client == nil {
		l.Client = &http.Client{Timeout: 2 * time.Second}
	}
	var t *tail
	defer func() {
		if t != nil {
			t.f.Close()
		}
	}()
	tick := time.NewTicker(l.Poll)
	defer tick.Stop()
	lastSummary := time.Time{}
	for {
		if p := NewestLog(l.LogDir); p != "" && (t == nil || p != t.path) {
			nt, err := l.open(p)
			if err != nil {
				l.Log.Warn("show: cannot follow log", "path", p, "err", err)
			} else {
				if t != nil {
					t.f.Close()
				}
				t = nt
				l.Log.Info("show: following", "log", p, "session", t.session)
			}
		}
		if t != nil {
			l.read(t, false)
			if l.Bridge != "" && time.Since(lastSummary) >= 2*time.Second {
				lastSummary = time.Now()
				l.plan(ctx, t)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
	}
}

// open starts following a log. What it already holds is read without publishing, then
// the run start and the latest plan, heat and day summary are published so the panel is
// current.
func (l *Live) open(path string) (*tail, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	b := NewBuilder(l.Player)
	b.Why, b.Base = l.Whys, l.Base
	t := &tail{path: path, session: strings.TrimSuffix(filepath.Base(path), ".jsonl"), f: f, b: b}
	l.read(t, true)
	return t, nil
}

func (l *Live) read(t *tail, catchUp bool) {
	var held []Event
	var start *Event
	buf := make([]byte, 256<<10)
	for {
		n, err := t.f.Read(buf)
		if n > 0 {
			t.partial = append(t.partial, buf[:n]...)
			for {
				i := bytes.IndexByte(t.partial, '\n')
				if i < 0 {
					break
				}
				line := t.partial[:i]
				evs, _ := t.b.Feed(line)
				for _, ev := range evs {
					if catchUp {
						if ev.Kind == KindRunStart {
							e := ev
							start, held = &e, nil
						} else {
							held = holdLatest(held, ev)
						}
						continue
					}
					// A run start clears the panel: the standing plan must be published again.
					if ev.Kind == KindRunStart {
						t.planKey = ""
					}
					l.Publish(ev)
				}
				t.partial = t.partial[i+1:]
			}
			// Keep the unread tail small and detached from the big read buffer.
			t.partial = append([]byte(nil), t.partial...)
		}
		if err == io.EOF || n == 0 {
			break
		}
		if err != nil {
			l.Log.Warn("show: read", "err", err)
			break
		}
	}
	if start != nil {
		t.planKey = ""
		if t.b.day > start.Day {
			// The run is under way: redraw the panel, but the narrator must not greet a
			// run on day 1 that is already on day 40.
			start.Level = LevelTicker
		}
		l.Publish(*start)
	}
	for _, ev := range held {
		l.Publish(ev)
	}
}

// plan polls the bridge's summary and publishes the plan when it changes.
func (l *Live) plan(ctx context.Context, t *tail) {
	u := strings.TrimRight(l.Bridge, "/") + "/v1/summary?session_id=" + url.QueryEscape(t.session)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return
	}
	resp, err := l.Client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var s protocol.SummaryResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&s); err != nil || (s.Plan == "" && s.Directives == nil) {
		return
	}
	if l.Player.Model == "" && s.Model != "" {
		t.b.Player.Model = s.Model
	}
	key := s.Plan
	if s.Directives != nil {
		if b, err := json.Marshal(s.Directives); err == nil {
			key += string(b)
		}
	}
	if key == t.planKey {
		return
	}
	t.planKey = key
	day := s.PlanDay
	if day == 0 {
		day = s.Day
	}
	ev := t.b.Plan(PlanInput{Day: day, Text: s.Plan, Directives: s.Directives})
	ev.V, ev.Run = SchemaVersion, t.b.run
	l.Publish(ev)
}
