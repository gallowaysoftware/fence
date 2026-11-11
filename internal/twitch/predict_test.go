package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeClock is a settable now.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time                { return c.t }
func (c *fakeClock) add(d time.Duration) time.Time { c.t = c.t.Add(d); return c.t }

func cashBet(id string, target int64) BetOpened {
	return BetOpened{ID: id, Claim: "Cash past " + thousands(target) + " by day 21", By: 21, Day: 14, Kind: "cash", Target: target}
}

func TestPredictionLifecycle(t *testing.T) {
	ctx := context.Background()
	type step struct {
		after time.Duration
		do    func(pr *Predictor, d *DryRun, now time.Time)
	}
	cases := []struct {
		name   string
		steps  []step
		status string // the panel's status at the end
		result string
		won    int // the winning outcome's index, -1 for none
		calls  []string
	}{
		{"opens, locks after its window, resolves Yes", []step{
			{0, func(pr *Predictor, d *DryRun, now time.Time) {
				pr.OpenBet(ctx, cashBet("b1", 2500), now)
				d.Back("u1", 0, 100)
				d.Back("u2", 1, 300)
			}},
			{2 * time.Minute, func(pr *Predictor, _ *DryRun, now time.Time) { pr.Tick(ctx, now) }},
			{time.Minute, func(pr *Predictor, _ *DryRun, _ time.Time) { pr.Resolve(ctx, "b1", true, "cash 2,612 on day 19") }},
		}, "resolved", "Yes: cash 2,612 on day 19", 0, []string{"POST 200", "PATCH LOCKED 200", "PATCH RESOLVED 200"}},
		{"a lost bet resolves No", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) { pr.OpenBet(ctx, cashBet("b1", 2500), now) }},
			{3 * time.Minute, func(pr *Predictor, _ *DryRun, now time.Time) { pr.Tick(ctx, now) }},
			{0, func(pr *Predictor, _ *DryRun, _ time.Time) { pr.Resolve(ctx, "b1", false, "cash 2,100 on day 21") }},
		}, "resolved", "No: cash 2,100 on day 21", 1, []string{"POST 200", "PATCH RESOLVED 200"}},
		{"evidence locks early: the target on screen", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) { pr.OpenBet(ctx, cashBet("b1", 2500), now) }},
			{10 * time.Second, func(pr *Predictor, _ *DryRun, now time.Time) {
				pr.Evidence(ctx, FeedEvent{Day: 14, Kind: "trade", Clock: &feedClock{Cash: 2400}}, now)
			}},
			{10 * time.Second, func(pr *Predictor, _ *DryRun, now time.Time) {
				pr.Evidence(ctx, FeedEvent{Day: 14, Kind: "trade", Clock: &feedClock{Cash: 2600}}, now)
				pr.Resolve(ctx, "b1", true, "cash 2,600 on day 14")
			}},
		}, "resolved", "Yes: cash 2,600 on day 14", 0, []string{"POST 200", "PATCH LOCKED 200", "PATCH RESOLVED 200"}},
		{"settled while still open: canceled and refunded", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) {
				pr.OpenBet(ctx, BetOpened{ID: "b1", Claim: "Medkits held to day 30", By: 30, Day: 22, Kind: "hold", Item: "Medkit"}, now)
			}},
			{20 * time.Second, func(pr *Predictor, _ *DryRun, _ time.Time) { pr.Resolve(ctx, "b1", false, "Medkit gone by day 22") }},
		}, "canceled", "", -1, []string{"POST 200", "PATCH CANCELED 200"}},
		{"an abandoned bet is canceled", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) { pr.OpenBet(ctx, cashBet("b1", 2500), now) }},
			{2 * time.Minute, func(pr *Predictor, _ *DryRun, now time.Time) { pr.Tick(ctx, now) }},
			{time.Hour, func(pr *Predictor, _ *DryRun, _ time.Time) { pr.Cancel(ctx, "b1", "the bet was abandoned") }},
		}, "canceled", "", -1, []string{"POST 200", "PATCH LOCKED 200", "PATCH CANCELED 200"}},
		{"one at a time: a second bet is skipped", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) {
				if !pr.OpenBet(ctx, cashBet("b1", 2500), now) {
					t.Error("first bet not opened")
				}
				if pr.OpenBet(ctx, cashBet("b2", 3000), now) {
					t.Error("second bet opened while the first holds the slot")
				}
			}},
		}, "active", "", -1, []string{"POST 200"}},
		{"Twitch's own window lock is picked up", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) { pr.OpenBet(ctx, cashBet("b1", 2500), now) }},
			{150 * time.Second, func(pr *Predictor, _ *DryRun, _ time.Time) { pr.Refresh(ctx) }},
			{0, func(pr *Predictor, _ *DryRun, _ time.Time) { pr.Resolve(ctx, "b1", true, "cash 2,600") }},
		}, "resolved", "Yes: cash 2,600", 0, []string{"POST 200", "PATCH RESOLVED 200"}},
		{"near 24 hours unresolved: canceled", []step{
			{0, func(pr *Predictor, _ *DryRun, now time.Time) {
				pr.OpenVerdict(ctx, VerdictSpec{Run: "r", Title: "Deadline sprint", Deadline: 91, LabPct: 52}, 1, now)
			}},
			{2 * time.Minute, func(pr *Predictor, _ *DryRun, now time.Time) { pr.Tick(ctx, now) }},
			{23 * time.Hour, func(pr *Predictor, _ *DryRun, now time.Time) { pr.Tick(ctx, now) }},
		}, "canceled", "", -1, []string{"POST 200", "PATCH LOCKED 200", "PATCH CANCELED 200"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
			var log bytes.Buffer
			d := NewDryRun(&log, clk.now)
			pr := &Predictor{API: d, LockAfter: 2 * time.Minute, Persona: "Shark"}
			for _, s := range tc.steps {
				s.do(pr, d, clk.add(s.after))
			}
			v := pr.View()
			if v == nil || v.Status != tc.status || v.Result != tc.result {
				t.Fatalf("view %+v, want status %s result %q", v, tc.status, tc.result)
			}
			for i, o := range v.Outcomes {
				if o.Won != (i == tc.won) {
					t.Errorf("outcome %d won=%v", i, o.Won)
				}
			}
			if got := calls(t, log.String()); strings.Join(got, ", ") != strings.Join(tc.calls, ", ") {
				t.Errorf("calls %v, want %v", got, tc.calls)
			}
		})
	}
}

// calls is the dry-run log as "METHOD [STATUS] CODE" lines.
func calls(t *testing.T, log string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(log), "\n") {
		if l == "" {
			continue
		}
		var e struct {
			Call   string         `json:"call"`
			Body   map[string]any `json:"body"`
			Status string         `json:"status"`
		}
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("log line %q: %v", l, err)
		}
		if !strings.Contains(e.Call, "/helix/") {
			continue
		}
		s := strings.Fields(e.Call)[0]
		if st, ok := e.Body["status"].(string); ok {
			s += " " + st
		}
		out = append(out, s+" "+strings.Fields(e.Status)[0])
	}
	return out
}

func TestDryRunEnforcesTwitchRules(t *testing.T) {
	ctx := context.Background()
	clk := &fakeClock{t: time.Unix(1_800_000_000, 0)}
	var log bytes.Buffer
	d := NewDryRun(&log, clk.now)
	cases := []struct {
		name     string
		title    string
		outcomes []string
		window   time.Duration
		ok       bool
	}{
		{"title over 45", strings.Repeat("x", 46), yesNo, time.Minute, false},
		{"title of 45 runes", strings.Repeat("é", 45), yesNo, time.Minute, true},
		{"one outcome", "Will it?", []string{"Yes"}, time.Minute, false},
		{"outcome over 25", "Will it?", []string{"Yes", strings.Repeat("n", 26)}, time.Minute, false},
		{"window over 30 minutes", "Will it?", yesNo, 31 * time.Minute, false},
		{"window under a second", "Will it?", yesNo, 0, false},
		{"a second one while the first runs", "Again?", yesNo, time.Minute, false},
	}
	var first Prediction
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := d.Create(ctx, tc.title, tc.outcomes, tc.window)
			if (err == nil) != tc.ok {
				t.Fatalf("err %v, want ok=%v", err, tc.ok)
			}
			if tc.ok {
				first = p
			}
		})
	}
	if _, err := d.End(ctx, first.ID, StatusResolved, "nope"); err == nil {
		t.Error("resolved with an unknown winner")
	}
	if _, err := d.End(ctx, first.ID, StatusLocked, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.End(ctx, first.ID, StatusLocked, ""); err == nil {
		t.Error("locked a locked prediction")
	}
	if _, err := d.Create(ctx, "Locked still holds the slot?", yesNo, time.Minute); err == nil {
		t.Error("a locked prediction did not hold the slot")
	}
	if _, err := d.End(ctx, first.ID, StatusResolved, first.Outcomes[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.End(ctx, first.ID, StatusCanceled, ""); err == nil {
		t.Error("canceled a resolved prediction")
	}
	if _, err := d.Create(ctx, "Free again?", yesNo, time.Minute); err != nil {
		t.Errorf("slot not freed by resolving: %v", err)
	}
	// Every line of the log is a JSON object naming its call.
	for _, l := range strings.Split(strings.TrimSpace(log.String()), "\n") {
		var e LogEntry
		if err := json.Unmarshal([]byte(l), &e); err != nil || e.Call == "" || e.T == "" {
			t.Errorf("log line %q", l)
		}
		if strings.Contains(l, "Bearer") || strings.Contains(l, "oauth") {
			t.Errorf("log line holds a credential: %q", l)
		}
	}
}

func TestTitles(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"a short claim is the question", BetTitle(BetOpened{Claim: "Cash past 2,500 by day 21."}, "Shark"), "Cash past 2,500 by day 21?"},
		{"a long cash claim", BetTitle(BetOpened{Claim: "Cash past 2,500 by day 21, rent and all, and the landlord can whistle", Kind: "cash", Target: 2500, By: 21}, "Shark"), "Cash 2,500 by day 21?"},
		{"a long claim with a long item", BetTitle(BetOpened{Claim: strings.Repeat("words ", 20), Kind: "sell", Item: strings.Repeat("Medkit ", 8), By: 30}, "Shark"), "Will the Shark make it?"},
		{"verdict", VerdictTitle(VerdictSpec{Deadline: 91}, "Shark"), "Will the Shark own the shop by day 91?"},
		{"verdict, long persona", VerdictTitle(VerdictSpec{Deadline: 91}, "Honest Broker of the Lower Levels"), "Bought by day 91?"},
		{"no deadline", VerdictTitle(VerdictSpec{LabPct: 2}, "The Magnate"), "Will the Magnate buy the shop?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("%q, want %q", tc.got, tc.want)
			}
			if utf8.RuneCountInString(tc.got) > MaxTitle {
				t.Errorf("%q is over %d characters", tc.got, MaxTitle)
			}
		})
	}
}
