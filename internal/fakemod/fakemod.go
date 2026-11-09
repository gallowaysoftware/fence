// Package fakemod plays the mod's side of the protocol from the shared fixtures, so the
// bridge can be smoke-tested end to end without the game.
package fakemod

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

type Config struct {
	BaseURL     string
	FixturesDir string
	Out         io.Writer
	HTTP        *http.Client
}

// Result is what one run observed, for tests.
type Result struct {
	SessionID string
	Mode      protocol.Mode
	Decision  protocol.DecideResponse
	Latency   time.Duration
	Rehello   bool
	Summary   protocol.SummaryResponse
	// Events is the kinds sent, in order.
	Events []string
}

// Before the decision: the morning snapshot, the shutter, the customer at the counter.
// After it: the human's move. Seqs are renumbered per session, as the mod does.
var (
	eventsBefore = []string{"event.day_snapshot.json", "event.request.json", "event.customer_observed.json"}
	eventsAfter  = []string{"event.human_action.json"}
)

// Run does health → hello → events → decide → human action → outcome → summary. Like the
// real mod, it answers a 404 on decide (bridge restarted, session forgotten) with one
// re-hello and retry.
func Run(ctx context.Context, cfg Config) (Result, error) {
	m := &mod{cfg: cfg}
	if m.cfg.HTTP == nil {
		m.cfg.HTTP = &http.Client{}
	}
	if m.cfg.Out == nil {
		m.cfg.Out = io.Discard
	}
	var res Result

	var health protocol.Health
	if code, err := m.do(ctx, http.MethodGet, "/v1/health", nil, &health); err != nil || code != 200 {
		return res, statusErr("health", code, err)
	}
	m.printf("health: ok=%v protocol=%d\n", health.OK, health.Protocol)

	hello, err := m.fixture("hello.request.json")
	if err != nil {
		return res, err
	}
	if res.SessionID, res.Mode, err = m.hello(ctx, hello); err != nil {
		return res, err
	}

	for _, name := range eventsBefore {
		if err := m.event(ctx, &res, name); err != nil {
			return res, err
		}
	}

	dec, err := m.fixture("decide.request.json")
	if err != nil {
		return res, err
	}
	var req protocol.DecideRequest
	if err := remarshal(dec, &req); err != nil {
		return res, fmt.Errorf("decide fixture: %w", err)
	}
	for attempt := 0; ; attempt++ {
		dec["session_id"] = res.SessionID
		start := time.Now()
		code, err := m.do(ctx, http.MethodPost, "/v1/decide", dec, &res.Decision)
		res.Latency = time.Since(start)
		if err == nil && code == http.StatusNotFound && attempt == 0 {
			m.printf("decide: 404 unknown session; re-hello\n")
			res.Rehello = true
			if res.SessionID, res.Mode, err = m.hello(ctx, hello); err != nil {
				return res, err
			}
			continue
		}
		if err != nil || code != 200 {
			return res, statusErr("decide", code, err)
		}
		break
	}
	d := res.Decision
	m.printf("decide: %s %s → option=%s params=%s source=%s in %v (deadline %dms)\n  rationale: %s\n",
		req.Kind, req.DecisionID, d.Choice.Option, paramsString(d.Choice.Params), d.Source,
		res.Latency.Round(time.Millisecond), req.DeadlineMS, d.Rationale)
	for _, o := range req.Options {
		if n, ok := d.OptionNotes[o.ID]; ok {
			m.printf("  note %s: %s\n", o.ID, n)
		}
	}
	if d.DecisionID != req.DecisionID {
		return res, fmt.Errorf("decide: answered decision %q, asked %q", d.DecisionID, req.DecisionID)
	}
	if err := policy.ValidateChoice(req.Options, d.Choice); err != nil {
		return res, fmt.Errorf("decide: bridge returned an illegal choice: %w", err)
	}
	if res.Latency > time.Duration(req.DeadlineMS)*time.Millisecond {
		return res, fmt.Errorf("decide: answered after %v, past the %dms deadline", res.Latency, req.DeadlineMS)
	}

	for _, name := range eventsAfter {
		if err := m.event(ctx, &res, name); err != nil {
			return res, err
		}
	}

	oc, err := m.fixture("outcome.request.json")
	if err != nil {
		return res, err
	}
	oc["session_id"] = res.SessionID
	if code, err := m.do(ctx, http.MethodPost, "/v1/outcome", oc, nil); err != nil || code != http.StatusNoContent {
		return res, statusErr("outcome", code, err)
	}
	m.printf("outcome: %v → 204\n", oc["decision_id"])

	code, err := m.do(ctx, http.MethodGet, "/v1/summary?session_id="+url.QueryEscape(res.SessionID), nil, &res.Summary)
	if err != nil || code != 200 {
		return res, statusErr("summary", code, err)
	}
	sm := res.Summary
	m.printf("summary: day %d mode=%s policy=%s model=%q\n  today: %d decisions, %d sales, %d purchases, cash %+d, %d walk-outs, rep %s\n  lessons (day %d): %q\n",
		sm.Day, sm.Mode, sm.Policy, sm.Model, sm.Today.Decisions, sm.Today.Sales, sm.Today.Purchases, sm.Today.CashDelta,
		sm.Today.Walkouts, repString(sm.Today.RepDelta), sm.LessonsDay, sm.Lessons)
	return res, nil
}

func repString(d map[string]float64) string {
	if len(d) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(d)
	return string(b)
}

type mod struct {
	cfg Config
	seq int64
}

func (m *mod) event(ctx context.Context, res *Result, name string) error {
	ev, err := m.fixture(name)
	if err != nil {
		return err
	}
	m.seq++
	ev["session_id"], ev["seq"] = res.SessionID, m.seq
	if code, err := m.do(ctx, http.MethodPost, "/v1/event", ev, nil); err != nil || code != http.StatusNoContent {
		return statusErr("event "+name, code, err)
	}
	kind, _ := ev["kind"].(string)
	res.Events = append(res.Events, kind)
	_, hidden := ev["hidden"]
	m.printf("event: %s seq=%d hidden=%v → 204\n", kind, m.seq, hidden)
	return nil
}

func (m *mod) printf(format string, a ...any) { fmt.Fprintf(m.cfg.Out, format, a...) }

func (m *mod) hello(ctx context.Context, body map[string]any) (string, protocol.Mode, error) {
	var hr protocol.HelloResponse
	code, err := m.do(ctx, http.MethodPost, "/v1/hello", body, &hr)
	if err != nil || code != 200 {
		return "", "", statusErr("hello", code, err)
	}
	m.seq = 0
	m.printf("hello: session=%s mode=%s protocol=%d\n", hr.SessionID, hr.Mode, hr.Protocol)
	return hr.SessionID, hr.Mode, nil
}

func (m *mod) fixture(name string) (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(m.cfg.FixturesDir, name))
	if err != nil {
		return nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return v, nil
}

func (m *mod) do(ctx context.Context, method, path string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(m.cfg.BaseURL, "/")+path, rd)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.cfg.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if out != nil && resp.StatusCode == 200 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s response: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

func statusErr(step string, code int, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", step, err)
	}
	return fmt.Errorf("%s: unexpected HTTP %d", step, code)
}

func remarshal(in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func paramsString(p map[string]json.RawMessage) string {
	if len(p) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(p)
	return string(b)
}
