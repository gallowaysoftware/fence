package twitch

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/show"
)

// What a prediction is about.
const (
	AboutBet     = "bet"
	AboutVerdict = "verdict"
)

// Predictor runs Twitch's one prediction slot. A bet or the run's verdict opens a prediction
// (Yes / No); it locks after LockAfter, or earlier on the first feed event that could settle it, so
// the window always closes before the outcome can be known; it resolves on the outcome. A bet
// settled while its prediction was still open is canceled with refunds instead: someone could
// have predicted with the answer on screen. A dropped or replaced bet, the run's end and the
// stream's end cancel too, as does the slot nearing Twitch's 24-hour limit.
type Predictor struct {
	API       Predictions
	LockAfter time.Duration
	// Persona is the run's persona name ("Shark"), for titles.
	Persona string
	cur     *slot
	// skipped is the latest bet that found the slot taken, shown under the one that holds it.
	skipped *show.AudienceSkip
	// shown is the last finished prediction, kept on the panel until the next opens.
	shown *show.AudiencePrediction
	// Log receives one line per decision that is not an API call (nil: none).
	Log func(call string, body any)
}

type slot struct {
	p       Prediction
	about   string
	key     string
	claim   string
	day, by int
	kind    string
	target  int64
	lockAt  time.Time
	note    string
}

// Yes and No are every prediction's outcomes, in this order.
var yesNo = []string{"Yes", "No"}

func (pr *Predictor) note(call string, body any) {
	if pr.Log != nil {
		pr.Log(call, body)
	}
}

// window is Twitch's prediction_window: a little past our own lock, so Twitch's lock is only
// the backstop when this process is gone.
func (pr *Predictor) window() time.Duration {
	return min(max(pr.lockAfter()+15*time.Second, 30*time.Second), MaxWindow)
}

func (pr *Predictor) lockAfter() time.Duration {
	return min(max(pr.LockAfter.Round(time.Second), 30*time.Second), MaxWindow-5*time.Second)
}

// Busy reports whether a prediction holds the slot.
func (pr *Predictor) Busy() bool { return pr.cur != nil }

// OpenBet opens a prediction on b; with the slot taken it records the skip and returns false.
func (pr *Predictor) OpenBet(ctx context.Context, b BetOpened, now time.Time) bool {
	if pr.cur != nil {
		pr.skipped = &show.AudienceSkip{Claim: b.Claim, Why: "Twitch runs one prediction at a time"}
		pr.note("skip", map[string]any{"bet": b.ID, "held_by": pr.cur.key})
		return false
	}
	return pr.open(ctx, &slot{about: AboutBet, key: b.ID, claim: b.Claim, day: b.Day, by: b.By, kind: b.Kind, target: b.Target}, BetTitle(b, pr.Persona), now)
}

// VerdictSpec is the run's format, as far as its verdict prediction needs it.
type VerdictSpec struct {
	Run      string
	Title    string
	Deadline int
	LabPct   int
}

// Worth is whether the format's outcome is in doubt: a deadline, or lab odds under 80 %.
func (v VerdictSpec) Worth() bool { return v.Deadline > 0 || (v.LabPct >= 0 && v.LabPct < 80) }

// OpenVerdict opens the run's format verdict prediction on day; false when the slot is taken.
func (pr *Predictor) OpenVerdict(ctx context.Context, v VerdictSpec, day int, now time.Time) bool {
	if pr.cur != nil {
		return false
	}
	claim := v.Title
	if v.Deadline > 0 {
		claim = fmt.Sprintf("%s: the shop bought by day %d", v.Title, v.Deadline)
	}
	return pr.open(ctx, &slot{about: AboutVerdict, key: "verdict:" + v.Run, claim: claim, day: day, by: v.Deadline}, VerdictTitle(v, pr.Persona), now)
}

func (pr *Predictor) open(ctx context.Context, s *slot, title string, now time.Time) bool {
	p, err := pr.API.Create(ctx, title, yesNo, pr.window())
	if err != nil {
		pr.shown = &show.AudiencePrediction{About: s.about, Title: title, Claim: s.claim, Day: s.day, By: s.by, Status: "failed", Note: err.Error()}
		return false
	}
	s.p, s.lockAt = p, now.Add(pr.lockAfter())
	pr.cur, pr.skipped, pr.shown = s, nil, nil
	return true
}

// Evidence locks an open bet prediction on the first feed event that could settle its bet, or
// that puts the answer on screen.
func (pr *Predictor) Evidence(ctx context.Context, ev FeedEvent, now time.Time) {
	s := pr.cur
	if s == nil || s.about != AboutBet || s.p.Status != StatusActive {
		return
	}
	if why := reveals(s, ev); why != "" {
		pr.lock(ctx, why)
	}
}

// reveals says why ev could settle the bet in s, or put its answer in view; "" when it cannot.
// A bet the state checks is checked on the day's snapshots (the close above all), its payments on
// money events, its sales on trades.
func reveals(s *slot, ev FeedEvent) string {
	switch {
	case ev.Day > s.day:
		return "a new day"
	case ev.Kind == "day":
		return "the day closed"
	case ev.Kind == "run_end":
		return "the run ended"
	}
	switch s.kind {
	case "cash", "net_worth":
		if c := ev.Clock; c != nil && s.target > 0 {
			have := c.Cash
			if s.kind == "net_worth" {
				have = c.NetWorth
			}
			if have >= s.target {
				return "the target is on screen"
			}
		}
	case "sell", "hold":
		if ev.Kind == "trade" && ev.Trade != nil && ev.Trade.Side == "sold" {
			return "a sale"
		}
	case "payment":
		if ev.Kind == "money" {
			return "a payment"
		}
	case "upgrade":
		if ev.Kind == "purchase" {
			return "a purchase"
		}
	}
	return ""
}

func (pr *Predictor) lock(ctx context.Context, why string) {
	s := pr.cur
	p, err := pr.API.End(ctx, s.p.ID, StatusLocked, "")
	if err != nil {
		// Twitch locks a prediction itself when its window runs out.
		if g, gerr := pr.API.Get(ctx, s.p.ID); gerr == nil {
			s.p = g
		}
		return
	}
	s.p, s.note = p, "locked: "+why
}

// Tick locks the open prediction when its window is up and cancels one near Twitch's 24-hour
// limit; call it every second or so.
func (pr *Predictor) Tick(ctx context.Context, now time.Time) {
	s := pr.cur
	if s == nil {
		return
	}
	switch {
	case s.p.Status != StatusActive || now.Before(s.lockAt):
	case !now.Before(s.p.Created.Add(s.p.Window)):
		// Twitch has locked it itself.
		pr.Refresh(ctx)
	default:
		pr.lock(ctx, "the window closed")
	}
	if now.Sub(s.p.Created) >= ResolveWithin-time.Hour {
		pr.Cancel(ctx, s.key, "nearly 24 hours unresolved; Twitch would refund it anyway")
	}
}

// Refresh reads the split so far.
func (pr *Predictor) Refresh(ctx context.Context) {
	if s := pr.cur; s != nil {
		if p, err := pr.API.Get(ctx, s.p.ID); err == nil {
			s.p.Outcomes = p.Outcomes
			if s.p.Status == StatusActive && p.Status == StatusLocked {
				s.p.Status, s.note = StatusLocked, "locked: the window closed"
			}
		}
	}
}

// Resolve settles the prediction for key: Yes when yes. A prediction still open is canceled with
// refunds instead (the answer arrived before the window closed).
func (pr *Predictor) Resolve(ctx context.Context, key string, yes bool, fact string) {
	s := pr.cur
	if s == nil || s.key != key {
		return
	}
	pr.Refresh(ctx)
	if s.p.Status == StatusActive {
		pr.Cancel(ctx, key, "settled before the window closed: "+fact)
		return
	}
	winner := s.p.Outcomes[1].ID
	if yes {
		winner = s.p.Outcomes[0].ID
	}
	p, err := pr.API.End(ctx, s.p.ID, StatusResolved, winner)
	if err != nil {
		pr.Cancel(ctx, key, "could not resolve: "+err.Error())
		return
	}
	s.p = p
	pr.finish(s, map[bool]string{true: "Yes", false: "No"}[yes]+": "+fact)
	markWinner(pr.shown, yes)
}

// Cancel refunds the prediction for key ("" for whichever holds the slot).
func (pr *Predictor) Cancel(ctx context.Context, key, why string) {
	s := pr.cur
	if s == nil || (key != "" && s.key != key) {
		return
	}
	pr.Refresh(ctx)
	if p, err := pr.API.End(ctx, s.p.ID, StatusCanceled, ""); err == nil {
		s.p = p
	} else {
		s.p.Status = StatusCanceled
	}
	s.note = "canceled, points refunded: " + why
	pr.finish(s, "")
}

func (pr *Predictor) finish(s *slot, result string) {
	a := pr.view(s)
	a.Result = result
	pr.cur, pr.shown = nil, a
}

// Holds reports whether the slot holds the prediction for key.
func (pr *Predictor) Holds(key string) bool { return pr.cur != nil && pr.cur.key == key }

// HoldsVerdict reports whether the run's verdict holds the slot, and its deadline.
func (pr *Predictor) HoldsVerdict() (deadline int, ok bool) {
	if pr.cur == nil || pr.cur.about != AboutVerdict {
		return 0, false
	}
	return pr.cur.by, true
}

// VerdictKey is the slot key of run's verdict.
func VerdictKey(run string) string { return "verdict:" + run }

// View is the prediction for the panel: the open one, else the last finished one; nil for none.
func (pr *Predictor) View() *show.AudiencePrediction {
	if pr.cur == nil {
		return pr.shown
	}
	a := pr.view(pr.cur)
	a.Skipped = pr.skipped
	return a
}

func (pr *Predictor) view(s *slot) *show.AudiencePrediction {
	a := &show.AudiencePrediction{About: s.about, Title: s.p.Title, Claim: s.claim, Day: s.day, By: s.by,
		Status: strings.ToLower(s.p.Status), Note: s.note}
	if s.p.Status == StatusActive {
		a.LocksAt = s.lockAt.UnixMilli()
	}
	for _, o := range s.p.Outcomes {
		a.Outcomes = append(a.Outcomes, show.AudienceOutcome{Title: o.Title, Users: o.Users, Points: o.Points})
	}
	return a
}

// markWinner marks the winning outcome on a resolved view.
func markWinner(a *show.AudiencePrediction, yes bool) {
	if a == nil || len(a.Outcomes) < 2 {
		return
	}
	a.Outcomes[map[bool]int{true: 0, false: 1}[yes]].Won = true
}

// BetTitle is a bet's prediction title within Twitch's 45 characters: the claim as a question
// when it fits, else one built from its kind, else "Will the Shark make it?". The panel shows the
// whole claim beside it.
func BetTitle(b BetOpened, persona string) string {
	if q := question(b.Claim); fits(q) {
		return q
	}
	var t string
	switch b.Kind {
	case "cash":
		t = fmt.Sprintf("Cash %s by day %d?", thousands(b.Target), b.By)
	case "net_worth":
		t = fmt.Sprintf("Net worth %s by day %d?", thousands(b.Target), b.By)
	case "sell":
		t = fmt.Sprintf("%s sold by day %d?", b.Item, b.By)
	case "hold":
		t = fmt.Sprintf("Still holding %s on day %d?", b.Item, b.By)
	case "upgrade":
		t = fmt.Sprintf("%s bought by day %d?", b.Item, b.By)
	case "payment":
		t = fmt.Sprintf("A payment made by day %d?", b.By)
	}
	if t != "" && fits(t) {
		return t
	}
	return "Will " + the(persona) + " make it?"
}

// VerdictTitle is the run's verdict as a question: "Will the Shark own the shop by day 91?".
func VerdictTitle(v VerdictSpec, persona string) string {
	var tries []string
	if v.Deadline > 0 {
		tries = []string{
			fmt.Sprintf("Will %s own the shop by day %d?", the(persona), v.Deadline),
			fmt.Sprintf("Bought by day %d?", v.Deadline),
		}
	} else {
		tries = []string{fmt.Sprintf("Will %s buy the shop?", the(persona)), "Will the shop be bought?"}
	}
	for _, t := range tries {
		if fits(t) {
			return t
		}
	}
	return tries[len(tries)-1]
}

func the(persona string) string {
	switch {
	case persona == "":
		return "the AI"
	case strings.HasPrefix(strings.ToLower(persona), "the "):
		return "t" + persona[1:]
	}
	return "the " + persona
}

func fits(s string) bool { return utf8.RuneCountInString(s) <= MaxTitle }

func question(claim string) string {
	c := strings.TrimRight(strings.TrimSpace(claim), ".!")
	if c == "" {
		return ""
	}
	if !strings.HasSuffix(c, "?") {
		c += "?"
	}
	return c
}

// thousands writes n with comma separators, as the claims do ("2,500").
func thousands(n int64) string {
	s := fmt.Sprint(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
