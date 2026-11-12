package gate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"
)

// samples keeps the last few hundred durations, enough for a stream's percentiles.
type samples struct {
	d    []time.Duration
	next int
}

const keepSamples = 512

func (s *samples) add(d time.Duration) {
	if len(s.d) < keepSamples {
		s.d = append(s.d, d)
		return
	}
	s.d[s.next] = d
	s.next = (s.next + 1) % keepSamples
}

// Millis summarises durations in milliseconds.
type Millis struct {
	N   int   `json:"n"`
	P50 int64 `json:"p50"`
	P95 int64 `json:"p95"`
	Max int64 `json:"max"`
}

func (s *samples) summary() Millis {
	if len(s.d) == 0 {
		return Millis{}
	}
	d := slices.Clone(s.d)
	slices.Sort(d)
	at := func(q float64) int64 { return d[int(q*float64(len(d)-1)+0.5)].Milliseconds() }
	return Millis{N: len(d), P50: at(0.5), P95: at(0.95), Max: d[len(d)-1].Milliseconds()}
}

type roleStats struct {
	mu sync.Mutex
	RoleStatus
	waits, upstream samples
}

func (s *roleStats) add(f func(*roleStats)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// RoleStatus is one role's counts since the gate started. Requests = Served + Dropped +
// Cancelled + Errors, once none is in the queue or in flight.
type RoleStatus struct {
	Requests  int `json:"requests"`
	Served    int `json:"served"`
	Dropped   int `json:"dropped"`
	Cancelled int `json:"cancelled"`
	Errors    int `json:"errors"`
	// Retries are upstream 503s retried: someone else holds the slot.
	Retries int `json:"retries"`
	Queued  int `json:"queued"`
	// WaitMS is the time in the queue (served, dropped and cancelled alike); UpstreamMS the
	// upstream's answer time of served requests, retries included.
	WaitMS     Millis `json:"wait_ms"`
	UpstreamMS Millis `json:"upstream_ms"`
}

// Status is what /gate and /metrics answer. It names no address: the pre-flight may show it.
type Status struct {
	Gate          string                `json:"gate"`
	UpstreamLocal bool                  `json:"upstream_local"`
	Busy          bool                  `json:"busy"`
	InFlight      string                `json:"in_flight,omitempty"`
	Roles         map[string]RoleStatus `json:"roles"`
}

const gateName = "fencegate"

func (g *Gate) Status() Status {
	g.mu.Lock()
	st := Status{Gate: gateName, UpstreamLocal: g.local, Busy: g.busy, Roles: map[string]RoleStatus{}}
	if g.inflight >= 0 {
		st.InFlight = g.inflight.String()
	}
	var queued [nRoles]int
	for r := range nRoles {
		queued[r] = len(g.queues[r])
	}
	g.mu.Unlock()
	for r := range nRoles {
		s := g.stats[r]
		s.mu.Lock()
		rs := s.RoleStatus
		rs.Queued, rs.WaitMS, rs.UpstreamMS = queued[r], s.waits.summary(), s.upstream.summary()
		s.mu.Unlock()
		st.Roles[Role(r).String()] = rs
	}
	return st
}

// Probe asks the server behind base (a caller's base URL, e.g. http://127.0.0.1:7380/narrate/v1)
// whether it is a gate, and returns its status. A plain model server is not a gate: ok is false.
func Probe(ctx context.Context, hc *http.Client, base string) (st Status, ok bool) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return st, false
	}
	u.Path, u.RawQuery = StatusPath, ""
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return st, false
	}
	resp, err := hc.Do(req)
	if err != nil {
		return st, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&st) != nil || st.Gate != gateName {
		return Status{}, false
	}
	return st, true
}
