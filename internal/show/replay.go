package show

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// Pace is how long the replay holds each kind of moment at speed 1, so a viewer can read
// it and the narrator has room to speak.
type Pace struct {
	Speed float64
	// Recorded, when set, holds each event for the time that passed before the next one in
	// the source (decision-log ts, a feed's publish time), capped at MaxHold: a rehearsal
	// replays at the pace it was played, so the narrator's density can be measured offline.
	Recorded bool
	MaxHold  time.Duration
	Ticker   time.Duration
	Notable  time.Duration
	Moment   time.Duration
	Plan     time.Duration
}

// DefaultPace makes a hard game day last about 20–30 s.
var DefaultPace = Pace{Speed: 1, Ticker: 700 * time.Millisecond, Notable: 2500 * time.Millisecond, Moment: 5 * time.Second, Plan: 7 * time.Second}

// Gate, when set on a replay, is called after each moment is published and may hold the
// replay (the narrator's WaitIdle).
type Gate func(ctx context.Context, ev Event)

// pacer holds events as a Pace says; a recorded pace remembers the previous event's time.
type pacer struct {
	Pace
	last time.Time
}

// wait sleeps before an event that happened at when (zero: unknown), as the recorded pace
// says; the fixed pace holds after each event instead (hold).
func (p *pacer) wait(ctx context.Context, when time.Time) error {
	if !p.Recorded || when.IsZero() {
		return nil
	}
	d := when.Sub(p.last)
	if p.last.IsZero() || d < 0 {
		d = 0
	}
	p.last = when
	if p.MaxHold > 0 && d > p.MaxHold {
		d = p.MaxHold
	}
	if p.Speed > 0 {
		d = time.Duration(float64(d) / p.Speed)
	}
	return Sleep(ctx, d)
}

func (p Pace) hold(ev Event) time.Duration {
	if p.Recorded || ev.Kind == KindSay {
		return 0
	}
	d := p.Ticker
	switch {
	case ev.Kind == KindPlan && ev.Level > LevelTicker:
		d = p.Plan
	case ev.Level >= LevelMoment:
		d = p.Moment
	case ev.Level == LevelNotable:
		d = p.Notable
	}
	if p.Speed > 0 {
		d = time.Duration(float64(d) / p.Speed)
	}
	return d
}

// Window limits a replay to days [From, To]; zero means unbounded. The days before From
// are still read, so the clock, names and plan are right when the window opens.
type Window struct{ From, To int }

func (w Window) before(day int) bool { return w.From > 0 && day < w.From }
func (w Window) after(day int) bool  { return w.To > 0 && day > w.To }

// EachEvent reads a decision log through b and calls emit for every event in the window.
// When the window opens late, the latest plan, heat and day summary before it are emitted
// first so the panel is not blank. emit returning an error stops the read.
func EachEvent(r io.Reader, b *Builder, w Window, emit func(Event) error) error {
	br := bufio.NewReaderSize(r, 1<<20)
	var held []Event
	opened := w.From <= 1
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 1 {
			evs, ferr := b.Feed(line)
			if ferr == nil {
				for _, ev := range evs {
					if w.after(ev.Day) {
						return nil
					}
					if !opened && ev.Kind == KindRunStart {
						if err := emit(ev); err != nil {
							return err
						}
						continue
					}
					if !opened && w.before(ev.Day) {
						held = holdLatest(held, ev)
						continue
					}
					if !opened {
						opened = true
						for _, h := range held {
							if err := emit(h); err != nil {
								return err
							}
						}
						held = nil
					}
					if err := emit(ev); err != nil {
						return err
					}
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// holdLatest keeps the run start and the latest plan, heat, day summary and standing against the book.
func holdLatest(held []Event, ev Event) []Event {
	switch ev.Kind {
	case KindPlan, KindHeat, KindDay, KindGhost:
	default:
		return held
	}
	for i := range held {
		if held[i].Kind == ev.Kind {
			held[i] = ev
			return held
		}
	}
	return append(held, ev)
}

// Replay plays a match's decision log into publish at the given pace.
func Replay(ctx context.Context, m *Match, w Window, pace Pace, gate Gate, publish func(Event) Event) error {
	f, err := os.Open(m.LogFile)
	if err != nil {
		return err
	}
	defer f.Close()
	p := &pacer{Pace: pace}
	return EachEvent(f, m.Builder(), w, func(ev Event) error {
		var when time.Time
		// What a late window replays first (the run start, the latest plan, heat and day) is
		// not paced: its times are the earlier days'.
		if !w.before(ev.Day) && ev.Kind != KindRunStart {
			when, _ = time.Parse(time.RFC3339Nano, ev.TS)
		}
		return show(ctx, ev, p, when, gate, publish)
	})
}

func show(ctx context.Context, ev Event, pace *pacer, when time.Time, gate Gate, publish func(Event) Event) error {
	if err := pace.wait(ctx, when); err != nil {
		return err
	}
	ev = publish(ev)
	if err := Sleep(ctx, pace.hold(ev)); err != nil {
		return err
	}
	if gate != nil && ev.Level >= LevelMoment {
		gate(ctx, ev)
	}
	return ctx.Err()
}

// PlayFeed replays a recorded show feed (JSON lines, as the hub records) at the given pace. Its
// narrator lines are left out: the narrator speaks the moments afresh.
func PlayFeed(ctx context.Context, r io.Reader, w Window, pace Pace, gate Gate, publish func(Event) Event) error {
	return playFeed(ctx, r, w, pace, gate, publish, false)
}

// PlayFeedLines replays a recorded feed with the lines it recorded, the narrator's and the
// co-host's, each with its audio path and where it fell between the moments, so a show replays as
// it went out. A line's for and reply_to follow the moments' new numbers.
func PlayFeedLines(ctx context.Context, r io.Reader, w Window, pace Pace, publish func(Event) Event) error {
	return playFeed(ctx, r, w, pace, nil, publish, true)
}

func playFeed(ctx context.Context, r io.Reader, w Window, pace Pace, gate Gate, publish func(Event) Event, lines bool) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	p := &pacer{Pace: pace}
	seqs := map[int64]int64{}
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || (ev.Kind == KindSay && (!lines || ev.Say == nil)) {
			continue
		}
		if w.after(ev.Day) {
			return nil
		}
		if w.before(ev.Day) && ev.Kind != KindRunStart {
			continue
		}
		var when time.Time
		if ev.At > 0 {
			when = time.UnixMilli(ev.At)
		}
		old := ev.Seq
		if s := ev.Say; s != nil {
			c := *s
			c.For, c.ReplyTo = seqs[c.For], seqs[c.ReplyTo]
			ev.Say = &c
		}
		ev.Seq, ev.At = 0, 0
		var got Event
		if err := show(ctx, ev, p, when, gate, func(e Event) Event { got = publish(e); return got }); err != nil {
			return err
		}
		if old != 0 {
			seqs[old] = got.Seq
		}
	}
	return sc.Err()
}

// Sleep waits d or until ctx ends, returning ctx.Err() then.
func Sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
