package gate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// upstream is a fake one-slot model server: it records each call's tag (the request body's
// "tag" field) and answers 503 to a second concurrent caller, as drift does.
type upstream struct {
	mu       sync.Mutex
	order    []string
	inflight atomic.Int32
	overlap  atomic.Bool
	// hold, when set, blocks every call until closed.
	hold chan struct{}
	// busy503 answers 503 to this many calls before serving.
	busy503 atomic.Int32
	srv     *httptest.Server
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{}
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte(`{"data":[{"id":"m"}]}`))
			return
		}
		if u.inflight.Add(1) > 1 {
			u.overlap.Store(true)
		}
		defer u.inflight.Add(-1)
		if u.busy503.Load() > 0 {
			u.busy503.Add(-1)
			http.Error(w, "background task bypassed while foreground generation", http.StatusServiceUnavailable)
			return
		}
		b, _ := io.ReadAll(r.Body)
		u.mu.Lock()
		u.order = append(u.order, tagOf(string(b)))
		u.mu.Unlock()
		if u.hold != nil {
			<-u.hold
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model":"m","choices":[{"message":{"content":"ok ` + r.URL.Path + `"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func tagOf(body string) string {
	_, rest, _ := strings.Cut(body, `"tag":"`)
	tag, _, _ := strings.Cut(rest, `"`)
	return tag
}

func (u *upstream) calls() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.order...)
}

func newGate(t *testing.T, u *upstream, cfg Config) (*Gate, *httptest.Server) {
	t.Helper()
	cfg.Upstream = u.srv.URL + "/v1"
	if cfg.Backoff == 0 {
		cfg.Backoff = 5 * time.Millisecond
	}
	g, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(g)
	t.Cleanup(s.Close)
	return g, s
}

type result struct {
	tag    string
	status int
	gate   string
}

func post(ctx context.Context, base, path, tag string, hdr map[string]string) result {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+path, strings.NewReader(`{"tag":"`+tag+`"}`))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return result{tag: tag, status: -1}
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return result{tag: tag, status: resp.StatusCode, gate: resp.Header.Get(HeaderGate)}
}

// waitQueued blocks until n requests are queued behind the one in flight.
func waitQueued(t *testing.T, g *Gate, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		q := 0
		for _, rq := range g.queues {
			q += len(rq)
		}
		g.mu.Unlock()
		if q == n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("never saw %d queued", n)
}

func TestRoute(t *testing.T) {
	for _, tc := range []struct {
		path string
		role Role
		rest string
		ok   bool
	}{
		{"/plan/v1/chat/completions", Plan, "/chat/completions", true},
		{"/narrate/v1/chat/completions", Narrate, "/chat/completions", true},
		{"/filler/v1/models", Filler, "/models", true},
		{"/v1/chat/completions", Plan, "/chat/completions", true},
		{"/chat/completions", Plan, "/chat/completions", true},
		{"/narrate/v1", 0, "", false},
		{"/", 0, "", false},
	} {
		role, rest, ok := route(tc.path)
		if ok != tc.ok || (ok && (role != tc.role || rest != tc.rest)) {
			t.Errorf("route(%q) = %v %q %v, want %v %q %v", tc.path, role, rest, ok, tc.role, tc.rest, tc.ok)
		}
	}
}

// One request reaches the upstream at a time, whatever the mix of callers.
func TestSerialises(t *testing.T) {
	u := newUpstream(t)
	u.hold = make(chan struct{})
	g, s := newGate(t, u, Config{MaxWait: [nRoles]time.Duration{}})
	var wg sync.WaitGroup
	res := make(chan result, 9)
	for i, p := range []string{"/plan/v1", "/narrate/v1", "/filler/v1", "/plan/v1", "/narrate/v1", "/filler/v1", "/plan/v1", "/narrate/v1", "/filler/v1"} {
		wg.Go(func() { res <- post(t.Context(), s.URL, p+"/chat/completions", string(rune('a'+i)), nil) })
	}
	waitQueued(t, g, 8)
	close(u.hold)
	wg.Wait()
	close(res)
	for r := range res {
		if r.status != http.StatusOK {
			t.Errorf("%s: status %d", r.tag, r.status)
		}
	}
	if u.overlap.Load() {
		t.Error("two requests reached the upstream at once")
	}
	if got := g.Status().Roles["narrate"].Served; got != 3 {
		t.Errorf("narrate served %d, want 3", got)
	}
}

// Behind the request in flight, the strategist goes first, then the narrator, then filler, each
// in arrival order.
func TestPriority(t *testing.T) {
	for _, tc := range []struct {
		name  string
		queue [][2]string // path, tag, in arrival order
		want  []string
	}{
		{"plan jumps the narrator", [][2]string{{"/narrate/v1", "n1"}, {"/plan/v1", "p1"}}, []string{"hold", "p1", "n1"}},
		{"filler last", [][2]string{{"/filler/v1", "f1"}, {"/narrate/v1", "n1"}, {"/plan/v1", "p1"}}, []string{"hold", "p1", "n1", "f1"}},
		{"fifo within a role", [][2]string{{"/narrate/v1", "n1"}, {"/narrate/v1", "n2"}, {"/plan/v1", "p1"}, {"/plan/v1", "p2"}}, []string{"hold", "p1", "p2", "n1", "n2"}},
		{"header overrides the path", [][2]string{{"/narrate/v1", "n1"}, {"/filler/v1?role=plan", "x"}}, []string{"hold", "x", "n1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t)
			u.hold = make(chan struct{})
			g, s := newGate(t, u, Config{})
			var wg sync.WaitGroup
			wg.Go(func() { post(t.Context(), s.URL, "/narrate/v1/chat/completions", "hold", nil) })
			waitQueued(t, g, 0)
			for len(u.calls()) == 0 {
				time.Sleep(time.Millisecond)
			}
			for i, q := range tc.queue {
				path, hdr := q[0], map[string]string(nil)
				if p, ok := strings.CutSuffix(path, "?role=plan"); ok {
					path, hdr = p, map[string]string{HeaderRole: "plan"}
				}
				wg.Go(func() { post(t.Context(), s.URL, path+"/chat/completions", q[1], hdr) })
				waitQueued(t, g, i+1)
			}
			close(u.hold)
			wg.Wait()
			if got := strings.Join(u.calls(), " "); got != strings.Join(tc.want, " ") {
				t.Errorf("order %q, want %q", got, strings.Join(tc.want, " "))
			}
		})
	}
}

// A request queued past its role's limit is answered 503 by the gate itself; the strategist's
// is never dropped by default.
func TestDrop(t *testing.T) {
	for _, tc := range []struct {
		name    string
		path    string
		hdr     map[string]string
		dropped bool
	}{
		{"narrator over its limit", "/narrate/v1", nil, true},
		{"filler over its limit", "/filler/v1", nil, true},
		{"strategist waits", "/plan/v1", nil, false},
		{"header raises the limit", "/narrate/v1", map[string]string{HeaderMaxWait: "10s"}, false},
		{"header sets a limit", "/plan/v1", map[string]string{HeaderMaxWait: "20ms"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t)
			u.hold = make(chan struct{})
			g, s := newGate(t, u, Config{MaxWait: [nRoles]time.Duration{Narrate: 20 * time.Millisecond, Filler: 20 * time.Millisecond}})
			var wg sync.WaitGroup
			wg.Go(func() { post(t.Context(), s.URL, "/plan/v1/chat/completions", "hold", nil) })
			for len(u.calls()) == 0 {
				time.Sleep(time.Millisecond)
			}
			res := make(chan result, 1)
			wg.Go(func() { res <- post(t.Context(), s.URL, tc.path+"/chat/completions", "q", tc.hdr) })
			waitQueued(t, g, 1)
			time.Sleep(80 * time.Millisecond)
			close(u.hold)
			wg.Wait()
			r := <-res
			if tc.dropped {
				if r.status != http.StatusServiceUnavailable || r.gate != "dropped" {
					t.Errorf("got %d %q, want 503 dropped", r.status, r.gate)
				}
			} else if r.status != http.StatusOK {
				t.Errorf("got %d, want 200", r.status)
			}
			role, _, _ := route(tc.path + "/x")
			if h := tc.hdr[HeaderRole]; h != "" {
				role, _ = ParseRole(h)
			}
			if got, want := g.Status().Roles[role.String()].Dropped, map[bool]int{true: 1}[tc.dropped]; got != want {
				t.Errorf("dropped count %d, want %d", got, want)
			}
		})
	}
}

// The narrator's client sees a drop as an HTTP 503, the error it already answers with the caption.
func TestDropIsAnErrorToTheNarratorClient(t *testing.T) {
	u := newUpstream(t)
	u.hold = make(chan struct{})
	g, s := newGate(t, u, Config{MaxWait: [nRoles]time.Duration{Narrate: 10 * time.Millisecond}})
	var wg sync.WaitGroup
	wg.Go(func() { post(t.Context(), s.URL, "/plan/v1/chat/completions", "hold", nil) })
	for len(u.calls()) == 0 {
		time.Sleep(time.Millisecond)
	}
	c := &llm.OpenAI{BaseURL: s.URL + "/narrate/v1", Model: "m", OpenAIOptions: llm.OpenAIOptions{NoSchema: true}}
	_, err := c.Complete(t.Context(), llm.Request{Messages: []llm.Message{{Role: llm.RoleUser, Content: "hi"}}})
	close(u.hold)
	wg.Wait()
	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusServiceUnavailable || !strings.Contains(apiErr.Body, "fencegate_dropped") {
		t.Fatalf("err = %v, want an HTTP 503 naming the drop", err)
	}
	if g.Status().Roles["narrate"].Dropped != 1 {
		t.Error("drop not counted")
	}
}

// Upstream 503s (another caller holds drift) are retried with backoff for up to RetryFor.
func TestRetries503(t *testing.T) {
	for _, tc := range []struct {
		name       string
		busy       int32
		retryFor   time.Duration
		wantStatus int
		wantRetry  int
	}{
		{"served after two 503s", 2, time.Second, http.StatusOK, 2},
		{"no 503", 0, time.Second, http.StatusOK, 0},
		{"gives up after RetryFor", 1000, 30 * time.Millisecond, http.StatusServiceUnavailable, -1},
		{"retries off", 1, 0, http.StatusServiceUnavailable, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := newUpstream(t)
			u.busy503.Store(tc.busy)
			g, s := newGate(t, u, Config{RetryFor: tc.retryFor})
			r := post(t.Context(), s.URL, "/plan/v1/chat/completions", "p", nil)
			if r.status != tc.wantStatus {
				t.Errorf("status %d, want %d", r.status, tc.wantStatus)
			}
			st := g.Status().Roles["plan"]
			if tc.wantRetry >= 0 && st.Retries != tc.wantRetry {
				t.Errorf("retries %d, want %d", st.Retries, tc.wantRetry)
			}
			if tc.wantRetry < 0 && st.Retries < 2 {
				t.Errorf("retries %d, want several", st.Retries)
			}
		})
	}
}

// A caller that gives up while queued leaves the queue and does not hold the slot.
func TestCancelledWaiterLeaves(t *testing.T) {
	u := newUpstream(t)
	u.hold = make(chan struct{})
	g, s := newGate(t, u, Config{})
	var wg sync.WaitGroup
	wg.Go(func() { post(t.Context(), s.URL, "/plan/v1/chat/completions", "hold", nil) })
	for len(u.calls()) == 0 {
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithCancel(t.Context())
	wg.Go(func() { post(ctx, s.URL, "/plan/v1/chat/completions", "gone", nil) })
	waitQueued(t, g, 1)
	cancel()
	waitQueued(t, g, 0)
	wg.Go(func() { post(t.Context(), s.URL, "/narrate/v1/chat/completions", "n", nil) })
	waitQueued(t, g, 1)
	close(u.hold)
	wg.Wait()
	if got := strings.Join(u.calls(), " "); got != "hold n" {
		t.Errorf("upstream saw %q, want \"hold n\"", got)
	}
	st := g.Status()
	if st.Roles["plan"].Cancelled != 1 || st.Busy {
		t.Errorf("cancelled %d busy %v, want 1 false", st.Roles["plan"].Cancelled, st.Busy)
	}
}

// A listing never waits for the slot: the pre-flight reads /models while a plan is written.
func TestGetPassesThrough(t *testing.T) {
	u := newUpstream(t)
	u.hold = make(chan struct{})
	_, s := newGate(t, u, Config{})
	var wg sync.WaitGroup
	wg.Go(func() { post(t.Context(), s.URL, "/plan/v1/chat/completions", "hold", nil) })
	for len(u.calls()) == 0 {
		time.Sleep(time.Millisecond)
	}
	resp, err := http.Get(s.URL + "/narrate/v1/models")
	close(u.hold)
	wg.Wait()
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /models: %v %v", err, resp)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `"id":"m"`) {
		t.Errorf("listing %q", b)
	}
}

func TestProbe(t *testing.T) {
	u := newUpstream(t)
	_, s := newGate(t, u, Config{})
	post(t.Context(), s.URL, "/narrate/v1/chat/completions", "n", nil)
	st, ok := Probe(t.Context(), http.DefaultClient, s.URL+"/narrate/v1")
	if !ok || !st.UpstreamLocal || st.Roles["narrate"].Served != 1 || st.Roles["narrate"].UpstreamMS.N != 1 {
		t.Errorf("Probe = %+v %v", st, ok)
	}
	if _, ok := Probe(t.Context(), http.DefaultClient, u.srv.URL+"/v1"); ok {
		t.Error("a plain model server probed as a gate")
	}
}

func TestSamplesSummary(t *testing.T) {
	var s samples
	for i := 1; i <= 100; i++ {
		s.add(time.Duration(i) * time.Millisecond)
	}
	if got := s.summary(); got != (Millis{N: 100, P50: 51, P95: 95, Max: 100}) {
		t.Errorf("summary %+v", got)
	}
	for i := 0; i < 2*keepSamples; i++ {
		s.add(time.Second)
	}
	if got := s.summary(); got.N != keepSamples || got.P50 != 1000 {
		t.Errorf("after wrap %+v", got)
	}
}
