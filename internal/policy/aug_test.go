package policy

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestAugFixtureResponse(t *testing.T) {
	b, err := os.ReadFile("../../protocol/fixtures/decide.aug_check.response.json")
	if err != nil {
		t.Fatal(err)
	}
	var want protocol.DecideResponse
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	req := fixtureNamed(t, "decide.aug_check.request.json")
	d := Heuristic{}.Decide(context.Background(), req)
	got := protocol.DecideResponse{DecisionID: req.DecisionID, Choice: d.Choice, Rationale: d.Rationale, Source: d.Source, OptionNotes: d.Notes}
	if a, b := mustJSON(t, got), mustJSON(t, want); string(a) != string(b) {
		t.Fatalf("heuristic answer differs from decide.aug_check.response.json:\ngot  %s\nwant %s", a, b)
	}
}

// augRequest is the fixture with its client replaced and only the listed options offered.
func augRequest(t *testing.T, client string, options ...string) *protocol.DecideRequest {
	t.Helper()
	req := fixtureNamed(t, "decide.aug_check.request.json")
	var ctx map[string]json.RawMessage
	if err := json.Unmarshal(req.Context, &ctx); err != nil {
		t.Fatal(err)
	}
	ctx["client"] = json.RawMessage(client)
	if !strings.Contains(client, `"aug":`) {
		ctx["detection"] = json.RawMessage(`"tells"`)
	}
	req.Context, _ = json.Marshal(ctx)
	var opts []protocol.Option
	for _, id := range options {
		opts = append(opts, protocol.Option{ID: id, Label: id})
	}
	req.Options = opts
	return req
}

const (
	augHuman      = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["medical"],"aug":false}`
	augTold       = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["medical"],"aug":true}`
	augTwitch     = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["medical"],"aug_tells":["twitching: a small fast side-to-side shake instead of the slow bob"]}`
	augPattern    = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["material","energy_cell"],"idle":"bobs normally"}`
	augRefused    = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["medical"],"haggle":"refused a markup and stayed"}`
	augScannedYes = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["medical"],"scan":"AUG DETECTED"}`
	augScannedNo  = `{"name":"Scav","faction":"lower_levels","intent":"buy","wants":["medical"],"scan":"No Aug detected"}`
)

func TestAugHeuristic(t *testing.T) {
	all := []string{optShoot, optScan, optProbeMarkup, optThreaten, optServe, optDismissAug}
	noScan := []string{optShoot, optProbeMarkup, optThreaten, optServe, optDismissAug}
	tests := []struct {
		name, client string
		options      []string
		want         string
	}{
		{"stated aug", augTold, noScan, optShoot},
		{"stated human", augHuman, noScan, optServe},
		{"zero-false-positive tell", augTwitch, noScan, optShoot},
		{"pattern only, scanner owned", augPattern, all, optScan},
		{"pattern only, no scanner", augPattern, noScan, optServe},
		{"haggle refusal is not proof", augRefused, noScan, optServe},
		{"scanner says aug", augScannedYes, noScan, optShoot},
		{"scanner says human", augScannedNo, noScan, optServe},
		{"aug but gun gone", augTold, []string{optServe, optDismissAug}, optServe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Heuristic{}.Decide(context.Background(), augRequest(t, tt.client, tt.options...))
			if d.Choice.Option != tt.want {
				t.Fatalf("chose %q (%s), want %q", d.Choice.Option, d.Rationale, tt.want)
			}
		})
	}
}

func TestCheckAugChoice(t *testing.T) {
	tests := []struct {
		name, client, option string
		ok                   bool
	}{
		{"shoot a stated aug", augTold, optShoot, true},
		{"shoot a stated human", augHuman, optShoot, false},
		{"shoot on a tell", augTwitch, optShoot, true},
		{"shoot on the shopping pattern alone", augPattern, optShoot, false},
		{"shoot after a haggle refusal", augRefused, optShoot, true},
		{"shoot a scanned aug", augScannedYes, optShoot, true},
		{"shoot a scanned human", augScannedNo, optShoot, false},
		{"serving is never refused", augPattern, optServe, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := augRequest(t, tt.client, optShoot, optServe)
			err := CheckChoice(req, protocol.Choice{Option: tt.option})
			if (err == nil) != tt.ok {
				t.Fatalf("CheckChoice = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestAugPromptStatesTheMode(t *testing.T) {
	for _, tt := range []struct{ client, want, not string }{
		{augTold, "STATUS: AUG (confirmed)", "TELL:"},
		{augTwitch, "TELL: twitching", "STATUS:"},
		{augPattern, "no definitive tell", "STATUS:"},
	} {
		c, err := parseAug(augRequest(t, tt.client, optShoot).Context)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		renderAug(&b, c)
		if !strings.Contains(b.String(), tt.want) || strings.Contains(b.String(), tt.not) {
			t.Fatalf("prompt for %s:\n%s", tt.client, b.String())
		}
	}
}
