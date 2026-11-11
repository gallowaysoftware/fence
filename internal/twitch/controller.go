package twitch

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/show"
)

// Config is the controller's settings; Defaults fills the zero ones.
type Config struct {
	// Predictions and Polls switch the two features; Verdict opens the run's format verdict
	// prediction at the run's start (it then holds Twitch's one slot until the run ends).
	Predictions bool
	Polls       bool
	Verdict     bool
	// LockAfter is a prediction's window (Twitch's prediction_window, 30 s to 30 min).
	LockAfter time.Duration
	// PollMax bounds a poll; PollMin keeps one open at least this long even when the day closes
	// first. PollEvery is the days between morning polls; MinVotes the fewest that are sent.
	PollMax   time.Duration
	PollMin   time.Duration
	PollEvery int
	MinVotes  int
	// TallyPath is where the bridge reads the vote (cmd/bridge -chat-tally); "" sends nothing.
	TallyPath string
}

func (c *Config) defaults() {
	if c.LockAfter <= 0 {
		c.LockAfter = 2 * time.Minute
	}
	if c.PollMax <= 0 {
		c.PollMax = 4 * time.Minute
	}
	if c.PollMin <= 0 {
		c.PollMin = 30 * time.Second
	}
	if c.PollEvery <= 0 {
		c.PollEvery = 1
	}
	if c.MinVotes <= 0 {
		c.MinVotes = 3
	}
}

// Triggers are told what the show did, so a scripted chat can answer it (ScriptChat).
type Triggers interface {
	OnPoll(words []string, now time.Time)
	OnPrediction(now time.Time)
	OnDay(day int, now time.Time)
}

// Controller drives the predictions and the chat votes from the show feed and chat, and
// publishes what the panel shows. It is not safe for concurrent use: one loop feeds it.
type Controller struct {
	cfg      Config
	pred     *Predictor
	chat     Chat
	triggers Triggers
	// back takes a dry run's simulated viewer predictions; nil live.
	back      func(user string, outcome int, points int64) bool
	publish   func(show.Event)
	log       func(call string, body any)
	synthetic *Synthetic

	run      string
	day      int
	verdict  VerdictSpec
	locked   []string
	clock    feedClock
	poll     *Poll
	last     *Poll
	pollDay  int
	rotation int
	asked    map[string]int
	ended    bool

	dirty     bool
	published time.Time
	refreshed time.Time
}

// Options wires a Controller.
type Options struct {
	Config      Config
	Predictions Predictions
	Chat        Chat
	Triggers    Triggers
	// Back is a dry run's simulated viewer predictions (DryRun.Back); nil live.
	Back      func(user string, outcome int, points int64) bool
	Publish   func(show.Event)
	Log       func(call string, body any)
	Synthetic *Synthetic
}

func NewController(o Options) *Controller {
	o.Config.defaults()
	c := &Controller{cfg: o.Config, chat: o.Chat, triggers: o.Triggers, back: o.Back, publish: o.Publish, log: o.Log,
		synthetic: o.Synthetic, asked: map[string]int{}}
	if c.publish == nil {
		c.publish = func(show.Event) {}
	}
	if c.log == nil {
		c.log = func(string, any) {}
	}
	c.pred = &Predictor{API: o.Predictions, LockAfter: o.Config.LockAfter, Log: c.log}
	return c
}

// Attach takes the panel state when the feed (re)connects. A prediction left in Twitch's slot by
// an earlier process is canceled: its bet's history is gone with that process.
func (c *Controller) Attach(ctx context.Context, s FeedState, now time.Time) {
	if c.pred.API != nil {
		if p, err := c.pred.API.Active(ctx); err == nil && p != nil && !c.pred.Busy() {
			if _, err := c.pred.API.End(ctx, p.ID, StatusCanceled, ""); err == nil {
				c.log("cancel_stale", map[string]any{"id": p.ID, "title": p.Title})
			}
		}
	}
	if s.Run == "" || s.Run == c.run {
		return
	}
	c.startRun(ctx, s.Run, s.Player, s.Day, s.End != nil, now)
}

func (c *Controller) startRun(ctx context.Context, run string, p *feedPlayer, day int, ended bool, now time.Time) {
	c.pred.Cancel(ctx, "", "a new run started")
	c.dropPoll("a new run started")
	c.run, c.day, c.ended, c.verdict, c.locked, c.pollDay, c.last = run, day, ended, VerdictSpec{Run: run, LabPct: -1}, nil, 0, nil
	c.asked = map[string]int{}
	if p != nil {
		c.pred.Persona = p.PersonaName
		if f := p.Format; f != nil {
			c.verdict = VerdictSpec{Run: run, Title: f.Title, Deadline: f.DeadlineDay, LabPct: f.LabPct}
			c.locked = lockedKeys(p.FormatHandicaps())
		}
	}
	// A verdict opened late is not worth much and too near the deadline is nearly settled.
	if c.cfg.Predictions && c.cfg.Verdict && !ended && c.verdict.Worth() && (c.verdict.Deadline == 0 || day <= c.verdict.Deadline-7) {
		if c.pred.OpenVerdict(ctx, c.verdict, day, now) {
			c.prediction(now)
		}
	}
	c.dirty = true
}

func (p *feedPlayer) FormatHandicaps() []string {
	if p == nil || p.Format == nil {
		return nil
	}
	return p.Format.Handicaps
}

// lockedKeys are the directive keys a format's handicaps (by label, as the card has them) lock.
func lockedKeys(labels []string) []string {
	var out []string
	for _, h := range format.Handicaps {
		if slices.Contains(labels, h.Label) || slices.Contains(labels, h.ID) {
			out = append(out, h.Locked...)
		}
	}
	return out
}

// Feed takes one feed event.
func (c *Controller) Feed(ctx context.Context, ev FeedEvent, now time.Time) {
	switch ev.Kind {
	case string(show.KindAudience), string(show.KindSay):
		return
	case string(show.KindRunStart):
		// A restarted fenceshow announces the run it was already showing.
		if ev.Run != c.run || c.run == "" {
			c.startRun(ctx, ev.Run, ev.Player, ev.Day, false, now)
		}
		return
	}
	if ev.Day > c.day {
		c.day = ev.Day
		if c.triggers != nil {
			c.triggers.OnDay(ev.Day, now)
		}
	}
	if ev.Clock != nil {
		c.clock = *ev.Clock
	}
	if c.cfg.Predictions {
		c.predictions(ctx, ev, now)
	}
	if c.cfg.Polls {
		c.polls(ev, now)
	}
	if ev.Kind == string(show.KindRunEnd) {
		c.ended = true
		c.pred.Cancel(ctx, "", "the run ended before the bet settled")
		c.dropPoll("the run ended")
	}
}

func (c *Controller) predictions(ctx context.Context, ev FeedEvent, now time.Time) {
	// Lock on what the event shows before reading what it settles: the lock reaches Twitch
	// before the answer reaches viewers, who watch behind the stream's delay.
	c.pred.Evidence(ctx, ev, now)
	var signals []BetSignal
	if s, ok := BetsFromFeed(ev); ok {
		signals = append(signals, s)
	}
	if c.synthetic != nil {
		signals = append(signals, c.synthetic.Feed(ev)...)
	}
	for _, s := range signals {
		switch {
		case s.Resolved != nil:
			r := s.Resolved
			c.log("bet_settled", map[string]any{"bet": r.ID, "outcome": r.Outcome, "fact": r.Fact})
			if r.Outcome == BetAbandoned {
				c.pred.Cancel(ctx, r.ID, "the bet was abandoned ("+r.Fact+")")
			} else {
				c.pred.Resolve(ctx, r.ID, r.Outcome == BetWon, r.Fact)
			}
			c.dirty = true
		case s.Opened != nil:
			c.log("bet_staked", map[string]any{"bet": s.Opened.ID, "claim": s.Opened.Claim, "by": s.Opened.By})
			if c.pred.OpenBet(ctx, *s.Opened, now) {
				c.prediction(now)
			}
			c.dirty = true
		}
	}
	if deadline, ok := c.pred.HoldsVerdict(); ok {
		key := VerdictKey(c.run)
		switch {
		case ev.Kind == string(show.KindRunEnd) && ev.End != nil:
			yes := ev.End.Won && (deadline == 0 || ev.End.Day <= deadline)
			c.pred.Resolve(ctx, key, yes, fmt.Sprintf("the run ended on day %d (%s)", ev.End.Day, strings.ReplaceAll(ev.End.Ending, "_", " ")))
			c.dirty = true
		case ev.Clock != nil && ev.Clock.Owned && (deadline == 0 || ev.Day <= deadline):
			c.pred.Resolve(ctx, key, true, fmt.Sprintf("the shop is owned on day %d", ev.Day))
			c.dirty = true
		case deadline > 0 && ev.Day > deadline && !c.clock.Owned:
			c.pred.Resolve(ctx, key, false, fmt.Sprintf("not bought by day %d", deadline))
			c.dirty = true
		}
	}
}

func (c *Controller) prediction(now time.Time) {
	if c.triggers != nil {
		c.triggers.OnPrediction(now)
	}
}

func (c *Controller) polls(ev FeedEvent, now time.Time) {
	switch {
	case ev.Kind == string(show.KindCandidates) && ev.Candidates != nil && ev.Candidates.Metric == "choice" && len(ev.Candidates.Options) >= 2:
		// A fork the strategist was just asked: chat's verdict reaches the next plan.
		var labels []string
		chosen := -1
		for i, o := range ev.Candidates.Options {
			labels = append(labels, o.Label)
			if o.Chosen {
				chosen = i
			}
		}
		labels = labels[:min(len(labels), 5)]
		spec, mine := ForkSpec(forkQuestion(ev.Text), labels, chosen)
		c.closePoll(now)
		day := max(ev.Candidates.Day, ev.Day)
		c.openPoll(spec, "the fork on day "+fmt.Sprint(day), day+1, now)
		c.poll.Fork = mine
	case ev.Kind == string(show.KindPlan) && ev.Plan != nil:
		c.answer(ev)
		day := ev.Plan.Day
		if (c.poll != nil && c.poll.Status == PollOpen) || c.ended || day <= c.pollDay || day%c.cfg.PollEvery != 0 {
			return
		}
		if spec, ok := c.nextSpec(day); ok {
			c.openPoll(spec, fmt.Sprintf("day %d's plan", day+1), day+1, now)
		}
	case ev.Kind == string(show.KindDay) && c.poll != nil && c.poll.Status == PollOpen && ev.Day >= c.poll.ForDay-1:
		// The day closed: the tally must be in before tomorrow's plan is written.
		c.poll.ClosesAt = maxTime(now, c.poll.Opened.Add(c.cfg.PollMin))
		c.dirty = true
	}
}

func forkQuestion(caption string) string {
	q := caption
	if i := strings.Index(q, ":"); i > 0 && i < 40 {
		q = q[:i]
	}
	return "Chat's call: " + strings.TrimSpace(q) + "?"
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// nextSpec is the morning poll for day: the mortgage while it is on offer (once a week), else
// the next in turn that the run's format leaves open.
func (c *Controller) nextSpec(day int) (PollSpec, bool) {
	offer := c.clock.OfferDown > 0 && !c.clock.Owned && c.clock.MortgageWk == 0
	for _, s := range Polls {
		if s.Offer && offer && c.open(s) && (c.asked[s.ID] == 0 || day-c.asked[s.ID] >= 7) {
			return s, true
		}
	}
	for range Polls {
		s := Polls[c.rotation%len(Polls)]
		c.rotation++
		if !s.Offer && c.open(s) {
			return s, true
		}
	}
	return PollSpec{}, false
}

func (c *Controller) open(s PollSpec) bool {
	for _, k := range s.Locks {
		if slices.Contains(c.locked, k) {
			return false
		}
	}
	return true
}

func (c *Controller) openPoll(spec PollSpec, forWhat string, forDay int, now time.Time) {
	if c.poll != nil && c.poll.Fact != "" {
		c.last = c.poll
	}
	c.poll = NewPoll(spec, forWhat, forDay, now, now.Add(c.cfg.PollMax))
	c.pollDay = forDay - 1
	c.asked[spec.ID] = forDay - 1
	c.dirty = true
	var words []string
	for _, o := range spec.Options {
		words = append(words, o.Word)
	}
	c.log("poll_open", map[string]any{"poll": spec.ID, "question": spec.Question, "words": words, "for": forWhat})
	if c.triggers != nil {
		c.triggers.OnPoll(words, now)
	}
}

// closePoll ends the open poll and sends its tally.
func (c *Controller) closePoll(now time.Time) {
	p := c.poll
	if p == nil || p.Status != PollOpen {
		return
	}
	p.Close(c.cfg.MinVotes)
	c.log("poll_close", map[string]any{"poll": p.Spec.ID, "counts": p.Counts(), "voters": p.Total(), "stats": p.Stats, "fact": p.Fact, "note": p.Note})
	if p.Fact != "" && c.cfg.TallyPath != "" {
		t := Tally{Run: c.run, ForDay: p.ForDay, Poll: p.Spec.ID, Fact: p.Fact, Written: now.UnixMilli()}
		if err := WriteTally(c.cfg.TallyPath, t); err != nil {
			p.Note = "tally not written: " + err.Error()
		} else {
			c.log("tally_sent", t)
		}
	}
	c.dirty = true
}

// dropPoll ends a poll without sending it.
func (c *Controller) dropPoll(why string) {
	if c.poll != nil && c.poll.Status == PollOpen {
		c.poll.Status, c.poll.Note = PollClosed, "not sent: "+why
		c.log("poll_drop", map[string]any{"poll": c.poll.Spec.ID, "why": why})
		c.dirty = true
	}
}

// answer finds the strategist's "Chat:" line in the plan that read the tally.
func (c *Controller) answer(ev FeedEvent) {
	p := c.poll
	if p == nil || p.Status == PollOpen {
		p = c.last
	}
	if p == nil || p.Fact == "" || p.Answer != "" || ev.Plan.Day < p.ForDay {
		return
	}
	for _, s := range ev.Plan.Sections {
		for _, l := range s.Lines {
			if rest, ok := cutFold(strings.TrimLeft(l, "-* "), "chat:"); ok {
				p.Answer = strings.TrimSpace(rest)
				c.log("strategist_answer", map[string]any{"poll": p.Spec.ID, "day": ev.Plan.Day, "answer": p.Answer})
				c.dirty = true
				return
			}
		}
	}
}

func cutFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return "", false
}

// Tick reads chat, closes a poll whose time is up, keeps the prediction's window and split, and
// publishes the panel's view when it changed (at most once a second).
func (c *Controller) Tick(ctx context.Context, now time.Time) {
	if c.chat != nil {
		for _, m := range c.chat.Due(now) {
			c.message(m, now)
		}
	}
	if c.poll != nil && c.poll.Status == PollOpen && !now.Before(c.poll.ClosesAt) {
		c.closePoll(now)
	}
	if c.pred.Busy() {
		c.pred.Tick(ctx, now)
		if now.Sub(c.refreshed) >= 5*time.Second {
			c.pred.Refresh(ctx)
			c.refreshed = now
			c.dirty = true
		}
	}
	if c.dirty && now.Sub(c.published) >= time.Second {
		c.Publish(now)
	}
}

func (c *Controller) message(m Message, now time.Time) {
	if c.back != nil {
		if i, pts, ok := SimulatedPrediction(m.Text); ok {
			if c.back(m.UserID, i, pts) {
				c.dirty = true
			}
			return
		}
	}
	if c.poll == nil {
		return
	}
	if c.poll.Vote(m, now) == VoteCounted {
		c.dirty = true
	}
}

// Close ends the stream: an open prediction is canceled with refunds (Twitch refunds after 24
// hours anyway) and an open poll is sent.
func (c *Controller) Close(ctx context.Context, now time.Time) {
	c.pred.Cancel(ctx, "", "the stream ended")
	c.closePoll(now)
	c.Publish(now)
}

// View is what the panel shows.
func (c *Controller) View() show.Audience {
	a := show.Audience{Prediction: c.pred.View()}
	if c.poll != nil {
		a.Poll = c.poll.View()
	}
	if c.last != nil && c.last != c.poll {
		a.LastPoll = c.last.View()
	}
	return a
}

// Publish sends the panel's view as an `audience` event.
func (c *Controller) Publish(now time.Time) {
	a := c.View()
	if a.Prediction == nil && a.Poll == nil && a.LastPoll == nil {
		return
	}
	c.publish(show.Event{Kind: show.KindAudience, Run: c.run, Day: c.day, Level: show.LevelTicker, Text: Caption(a), Audience: &a})
	c.dirty, c.published = false, now
}

// Caption is the view in one line, for the feed's text and a VOD's transcript.
func Caption(a show.Audience) string {
	var parts []string
	if p := a.Prediction; p != nil {
		s := "Prediction (" + p.Status + "): " + p.Title
		if split := splitText(p.Outcomes); split != "" {
			s += " " + split
		}
		if p.Result != "" {
			s += "; " + p.Result
		}
		if p.Status == "canceled" && p.Note != "" {
			s += "; " + p.Note
		}
		parts = append(parts, s)
	}
	if q := a.Poll; q != nil {
		var opts []string
		for _, o := range q.Options {
			opts = append(opts, fmt.Sprintf("!vote %s %d", o.Word, o.Votes))
		}
		s := fmt.Sprintf("Chat vote (%s) for %s: %s %s", q.Status, q.For, q.Question, strings.Join(opts, ", "))
		if q.Answer != "" {
			s += "; the strategist: " + q.Answer
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " · ")
}

func splitText(os []show.AudienceOutcome) string {
	var users int64
	for _, o := range os {
		users += o.Users
	}
	if users == 0 {
		return ""
	}
	var bits []string
	for _, o := range os {
		bits = append(bits, fmt.Sprintf("%s %d%%", o.Title, (200*o.Users+users)/(2*users)))
	}
	return "(" + strings.Join(bits, " / ") + ")"
}
