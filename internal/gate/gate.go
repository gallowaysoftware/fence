// Package gate serialises calls to a model server that serves one request at a time (drift's
// MTPLX queues a second caller behind the first, or answers it 503), so the strategist and the
// narrator can share it. Callers pick their priority by path: /plan/v1/…, /narrate/v1/…,
// /filler/v1/…; the rest of the path goes to the upstream base URL. One request reaches the upstream at a time, the
// highest role first; a queued request that waits longer than its role's limit is answered 503
// so the caller takes its own fallback (the narrator speaks the caption).
package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

type Role int

const (
	Plan Role = iota
	Narrate
	Filler
	nRoles
)

var roleNames = [nRoles]string{"plan", "narrate", "filler"}

func (r Role) String() string { return roleNames[r] }

// ParseRole reads a role name; ok is false for anything else.
func ParseRole(s string) (Role, bool) {
	i := slices.Index(roleNames[:], strings.ToLower(strings.TrimSpace(s)))
	return Role(i), i >= 0
}

const (
	// HeaderRole overrides the path's role; HeaderMaxWait (a Go duration) the role's queue limit.
	HeaderRole    = "X-Fence-Role"
	HeaderMaxWait = "X-Fence-Max-Wait"
	// HeaderGate is on every answer the gate itself writes: "dropped", "unavailable".
	HeaderGate = "X-Fence-Gate"
	// HeaderWait is the time the request queued, in milliseconds, on proxied answers.
	HeaderWait = "X-Fence-Gate-Wait-Ms"
	// StatusPath answers the gate's metrics as JSON; MetricsPath is the same.
	StatusPath  = "/gate"
	MetricsPath = "/metrics"
)

type Config struct {
	// Upstream is the model server's OpenAI-compatible base URL, e.g. http://host:port/v1.
	Upstream string
	// MaxWait is each role's longest queue wait; 0 waits as long as the caller does.
	MaxWait [nRoles]time.Duration
	// RetryFor bounds how long an upstream 503 (another caller holds the slot) is retried.
	RetryFor time.Duration
	// Backoff is the first wait after a 503; it doubles up to 4 s.
	Backoff time.Duration
	HTTP    *http.Client
	Log     *slog.Logger
}

// DefaultMaxWait: the narrator gives a line 8 s in all (fenceshow's live timeout) and drift
// makes one in about 2.5 s, so a line still queued after 5 s will not be in time.
var DefaultMaxWait = [nRoles]time.Duration{Plan: 0, Narrate: 5 * time.Second, Filler: 3 * time.Second}

type Gate struct {
	cfg      Config
	upstream *url.URL
	local    bool

	mu     sync.Mutex
	busy   bool
	queues [nRoles][]*waiter
	stats  [nRoles]*roleStats
	// inflight is the role holding the upstream, -1 when idle.
	inflight Role
}

type waiter struct{ ready chan struct{} }

var errDropped = errors.New("gate: queue wait over the role's limit")

func New(cfg Config) (*Gate, error) {
	u, err := url.Parse(strings.TrimRight(cfg.Upstream, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("gate: upstream %q is not a URL", cfg.Upstream)
	}
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Transport: &http.Transport{
			Proxy:       http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		}}
	}
	if cfg.Log == nil {
		cfg.Log = slog.New(slog.DiscardHandler)
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = 250 * time.Millisecond
	}
	g := &Gate{cfg: cfg, upstream: u, local: isLoopback(u), inflight: -1}
	for i := range g.stats {
		g.stats[i] = &roleStats{}
	}
	return g, nil
}

func isLoopback(u *url.URL) bool {
	h := u.Hostname()
	ip := net.ParseIP(h)
	return h == "localhost" || (ip != nil && ip.IsLoopback())
}

// acquire waits for the upstream slot. It returns errDropped when maxWait passes first, or the
// context's error when the caller gives up.
func (g *Gate) acquire(ctx context.Context, role Role, maxWait time.Duration) error {
	g.mu.Lock()
	if !g.busy {
		g.busy, g.inflight = true, role
		g.mu.Unlock()
		return nil
	}
	w := &waiter{ready: make(chan struct{})}
	g.queues[role] = append(g.queues[role], w)
	g.mu.Unlock()

	var expired <-chan time.Time
	if maxWait > 0 {
		t := time.NewTimer(maxWait)
		defer t.Stop()
		expired = t.C
	}
	var err error
	select {
	case <-w.ready:
		return nil
	case <-expired:
		err = errDropped
	case <-ctx.Done():
		err = ctx.Err()
	}
	if g.unqueue(role, w) {
		return err
	}
	// The slot was handed over as we gave up: pass it on.
	<-w.ready
	g.release()
	return err
}

func (g *Gate) unqueue(role Role, w *waiter) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	i := slices.Index(g.queues[role], w)
	if i < 0 {
		return false
	}
	g.queues[role] = slices.Delete(g.queues[role], i, i+1)
	return true
}

// release hands the slot to the first waiter of the highest role.
func (g *Gate) release() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for r := range nRoles {
		if q := g.queues[r]; len(q) > 0 {
			g.queues[r] = q[1:]
			g.inflight = r
			close(q[0].ready)
			return
		}
	}
	g.busy, g.inflight = false, -1
}

// maxRequestBody bounds what a caller can make the gate buffer.
const maxRequestBody = 16 << 20

func (g *Gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == StatusPath || r.URL.Path == MetricsPath {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(g.Status())
		return
	}
	role, rest, ok := route(r.URL.Path)
	if !ok {
		http.Error(w, "fencegate: want /plan/v1/…, /narrate/v1/… or /filler/v1/…", http.StatusNotFound)
		return
	}
	if h := r.Header.Get(HeaderRole); h != "" {
		if hr, ok := ParseRole(h); ok {
			role = hr
		}
	}
	maxWait := g.cfg.MaxWait[role]
	if h := r.Header.Get(HeaderMaxWait); h != "" {
		if d, err := time.ParseDuration(h); err == nil && d >= 0 {
			maxWait = d
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBody))
	if err != nil {
		http.Error(w, "fencegate: read request: "+err.Error(), http.StatusBadRequest)
		return
	}
	// A listing never generates, so it never waits for the slot.
	if r.Method == http.MethodGet {
		g.forward(w, r, rest, body, role, 0, false)
		return
	}
	st := g.stats[role]
	st.add(func(s *roleStats) { s.Requests++ })
	queued := time.Now()
	err = g.acquire(r.Context(), role, maxWait)
	wait := time.Since(queued)
	switch {
	case errors.Is(err, errDropped):
		st.add(func(s *roleStats) { s.Dropped++; s.waits.add(wait) })
		g.cfg.Log.Info("gate: dropped", "role", role, "wait_ms", wait.Milliseconds(), "max_wait", maxWait)
		writeGateError(w, http.StatusServiceUnavailable, "dropped",
			fmt.Sprintf("fencegate: %s request dropped after %v in the queue (limit %v)", role, wait.Round(time.Millisecond), maxWait))
		return
	case err != nil:
		st.add(func(s *roleStats) { s.Cancelled++; s.waits.add(wait) })
		g.cfg.Log.Info("gate: caller left the queue", "role", role, "wait_ms", wait.Milliseconds())
		return
	}
	defer g.release()
	st.add(func(s *roleStats) { s.waits.add(wait) })
	w.Header().Set(HeaderWait, fmt.Sprint(wait.Milliseconds()))
	g.forward(w, r, rest, body, role, wait, true)
}

// route splits /<role>/<rest> into the role and the path below the upstream's base URL. A path
// with no role prefix (/v1/…, /chat/completions) is the strategist's.
func route(p string) (Role, string, bool) {
	seg, rest, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
	role, ok := ParseRole(seg)
	if !ok {
		role, rest = Plan, strings.TrimPrefix(p, "/")
	}
	rest = "/" + strings.TrimPrefix(rest, "v1/")
	if rest == "/v1" || rest == "/" {
		return 0, "", false
	}
	return role, rest, true
}

func writeGateError(w http.ResponseWriter, status int, kind, msg string) {
	w.Header().Set(HeaderGate, kind)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg, "type": "fencegate_" + kind}})
}

// forward sends one request upstream, retrying 503s for up to RetryFor, and copies the answer.
func (g *Gate) forward(w http.ResponseWriter, r *http.Request, rest string, body []byte, role Role, wait time.Duration, count bool) {
	ctx := r.Context()
	target := *g.upstream
	target.Path = g.upstream.Path + rest
	target.RawQuery = r.URL.RawQuery
	start := time.Now()
	backoff := g.cfg.Backoff
	retries := 0
	var resp *http.Response
	var err error
	for {
		req, rerr := http.NewRequestWithContext(ctx, r.Method, target.String(), bytes.NewReader(body))
		if rerr != nil {
			writeGateError(w, http.StatusBadGateway, "unavailable", "fencegate: "+rerr.Error())
			return
		}
		for _, h := range []string{"Content-Type", "Authorization", "Accept"} {
			if v := r.Header.Get(h); v != "" {
				req.Header.Set(h, v)
			}
		}
		resp, err = g.cfg.HTTP.Do(req)
		if err != nil || resp.StatusCode != http.StatusServiceUnavailable || time.Since(start)+backoff > g.cfg.RetryFor {
			break
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
		retries++
		g.cfg.Log.Info("gate: upstream busy; retrying", "role", role, "retry", retries, "backoff", backoff)
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-time.After(backoff):
		}
		if err != nil {
			break
		}
		backoff = min(2*backoff, 4*time.Second)
	}
	took := time.Since(start)
	st := g.stats[role]
	if err != nil {
		if count {
			st.add(func(s *roleStats) { s.Errors++; s.Retries += retries })
		}
		g.cfg.Log.Warn("gate: upstream failed", "role", role, "wait_ms", wait.Milliseconds(), "upstream_ms", took.Milliseconds(), "retries", retries, "err", redact(err))
		if ctx.Err() == nil {
			writeGateError(w, http.StatusBadGateway, "unavailable", "fencegate: upstream unreachable: "+redact(err))
		}
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Cache-Control"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, cerr := copyFlushing(w, resp.Body)
	took = time.Since(start)
	if !count {
		return
	}
	ok := resp.StatusCode/100 == 2 && cerr == nil
	st.add(func(s *roleStats) {
		s.Retries += retries
		if ok {
			s.Served++
			s.upstream.add(took)
		} else {
			s.Errors++
		}
	})
	g.cfg.Log.Info("gate: served", "role", role, "status", resp.StatusCode, "wait_ms", wait.Milliseconds(), "upstream_ms", took.Milliseconds(), "retries", retries)
}

// redact keeps the upstream's address out of logs and answers: they may be shown on stream.
func redact(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Op + ": " + ue.Err.Error()
	}
	return err.Error()
}

// copyFlushing copies a streamed answer (stream: true) through as it arrives.
func copyFlushing(w http.ResponseWriter, r io.Reader) (int64, error) {
	rc := http.NewResponseController(w)
	buf := make([]byte, 16<<10)
	var n int64
	for {
		k, err := r.Read(buf)
		if k > 0 {
			if _, werr := w.Write(buf[:k]); werr != nil {
				return n, werr
			}
			n += int64(k)
			_ = rc.Flush()
		}
		if err == io.EOF {
			return n, nil
		}
		if err != nil {
			return n, err
		}
	}
}
