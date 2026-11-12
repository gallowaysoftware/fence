// Package vod turns a run's recorded show feed or decision log into a VOD kit: YouTube chapters,
// a title, description and tags, highlight clip windows and an ffmpeg cut script, one kit per OBS
// recording (docs/STREAM-OPS.md, "After the stream"). It reads only what the show layer already
// made player-visible, and writes nothing but local files.
package vod

import (
	"bufio"
	"encoding/json"
	"io"
	"slices"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
)

// Point is a show event placed on the wall clock.
type Point struct {
	At time.Time
	Ev show.Event
	// Told is, for a narrator line, the moment it told as its own feed has it (by say.for), nil
	// when the feed does not hold it.
	Told *show.Event
}

// Run is what the feed says about one run: its name card, its stream format and how it ended.
type Run struct {
	ID     string
	Player show.Player
	// Format is the run's stream format title ("" for none), read from the run start's
	// player.format card without depending on its Go type.
	Format string
	// Deadline is the format's last winning day, 0 for none.
	Deadline int
	End      *show.End
}

// Timeline is a run's events in wall-clock order, with the narrator's lines kept apart, and the
// co-host's exchanges (its lines and the narrator's answers) apart from those.
type Timeline struct {
	Points    []Point
	Says      []Point
	Exchanges []Point
	Runs      map[string]*Run
}

// InExchange reports whether a line belongs to a co-host exchange rather than telling a moment.
func InExchange(s *show.Say) bool {
	return s != nil && (s.Speaker == show.SpeakerCohost || s.ReplyTo != 0)
}

// When is when an event was on screen: the hub's publish time for a recorded feed, else the
// source's time (a decision log's ts). Zero when neither is known.
func When(ev show.Event) time.Time {
	if ev.At > 0 {
		return time.UnixMilli(ev.At)
	}
	t, _ := time.Parse(time.RFC3339Nano, ev.TS)
	return t
}

// Format is a run's stream format as its run start's card names it.
type Format struct {
	Title string
	// Deadline is the last day a buyout wins the format, 0 for none.
	Deadline int
}

// ReadFeed reads a recorded show feed (JSON lines, as fenceshow -record writes), and the format
// card of each run start by run: read from the JSON, so a card show.Player does not (yet) decode
// still reaches the kit.
func ReadFeed(r io.Reader) ([]show.Event, map[string]Format, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	var out []show.Event
	formats := map[string]Format{}
	for sc.Scan() {
		var ev show.Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Kind == "" {
			continue
		}
		if ev.Kind == show.KindRunStart {
			if f := formatIn(sc.Bytes()); f.Title != "" {
				formats[ev.Run] = f
			}
		}
		out = append(out, ev)
	}
	return out, formats, sc.Err()
}

// SetFormats gives each run the format a feed named for it, unless it has one.
func (t *Timeline) SetFormats(formats map[string]Format) {
	for id, f := range formats {
		if r := t.Runs[id]; r != nil && r.Format == "" {
			r.Format, r.Deadline = f.Title, f.Deadline
		}
	}
}

// Build places events on the clock. A restarted fenceshow re-publishes the run start and the
// latest plan, heat and day with a new publish time but the source's ts, so those repeats are
// dropped. lines, when given, is the feed whose narrator lines replace those in events (moments
// from a decision log, lines from the stream's feed).
func Build(events []show.Event, lines []show.Event) *Timeline {
	t := &Timeline{Runs: map[string]*Run{}}
	type key struct {
		kind    show.Kind
		run, ts string
		day     int
		text    string
	}
	seen := map[key]bool{}
	// bySeq is the source's latest event with each seq: a restart before ee3f77b numbered the
	// feed from 1 again, so the line's moment is the last one before it with that number.
	var bySeq map[int64]show.Event
	add := func(ev show.Event, linesOnly bool) {
		at := When(ev)
		if ev.Seq != 0 && ev.Kind != show.KindSay {
			bySeq[ev.Seq] = ev
		}
		if at.IsZero() {
			return
		}
		if ev.Kind == show.KindSay {
			if ev.Say != nil {
				pt := Point{At: at, Ev: ev}
				if m, ok := bySeq[ev.Say.For]; ok && ev.Say.For != 0 {
					pt.Told = &m
				}
				if InExchange(ev.Say) {
					t.Exchanges = append(t.Exchanges, pt)
				} else {
					t.Says = append(t.Says, pt)
				}
			}
			return
		}
		if linesOnly {
			return
		}
		if ev.TS != "" {
			k := key{ev.Kind, ev.Run, ev.TS, ev.Day, ev.Text}
			if seen[k] {
				return
			}
			seen[k] = true
		}
		t.note(ev)
		t.Points = append(t.Points, Point{At: at, Ev: ev})
	}
	bySeq = map[int64]show.Event{}
	for _, ev := range events {
		if lines != nil && ev.Kind == show.KindSay {
			continue
		}
		add(ev, false)
	}
	bySeq = map[int64]show.Event{}
	for _, ev := range lines {
		add(ev, true)
	}
	byTime := func(a, b Point) int { return a.At.Compare(b.At) }
	slices.SortStableFunc(t.Points, byTime)
	slices.SortStableFunc(t.Says, byTime)
	slices.SortStableFunc(t.Exchanges, byTime)
	return t
}

func (t *Timeline) note(ev show.Event) {
	if ev.Run == "" {
		return
	}
	r := t.Runs[ev.Run]
	if r == nil {
		r = &Run{ID: ev.Run}
		t.Runs[ev.Run] = r
	}
	switch {
	case ev.Kind == show.KindRunStart && ev.Player != nil:
		if r.Player.ID == "" {
			r.Player = *ev.Player
		}
		if r.Format == "" {
			r.Format, r.Deadline = formatOf(ev)
		}
	case ev.Kind == show.KindRunEnd && ev.End != nil:
		e := *ev.End
		r.End = &e
	}
}

// formatOf reads the run start's format card by its JSON, so a card added to show.Player later
// (the stream-formats work) needs no change here.
func formatOf(ev show.Event) (string, int) {
	b, err := json.Marshal(ev)
	if err != nil {
		return "", 0
	}
	f := formatIn(b)
	return f.Title, f.Deadline
}

// formatIn reads player.format out of an event's JSON: its title, else its name.
func formatIn(event []byte) Format {
	var p struct {
		Player struct {
			Format *struct {
				Name        string `json:"name"`
				Title       string `json:"title"`
				DeadlineDay int    `json:"deadline_day"`
			} `json:"format"`
		} `json:"player"`
	}
	if json.Unmarshal(event, &p) != nil || p.Player.Format == nil {
		return Format{}
	}
	f := p.Player.Format
	if f.Title == "" {
		f.Title = f.Name
	}
	return Format{Title: f.Title, Deadline: f.DeadlineDay}
}

// Merge fills r's empty name-card fields from o: a decision log knows the run, a feed's run start
// the model and persona the stream showed.
func (r *Run) Merge(o *Run) {
	if o == nil {
		return
	}
	p, q := &r.Player, o.Player
	for _, f := range []struct {
		dst *string
		src string
	}{
		{&p.ID, q.ID}, {&p.Model, q.Model}, {&p.Persona, q.Persona}, {&p.PersonaName, q.PersonaName},
		{&p.PersonaTagline, q.PersonaTagline}, {&p.Route, q.Route}, {&p.RouteBrief, q.RouteBrief},
		{&p.Difficulty, q.Difficulty}, {&p.Start, q.Start}, {&p.Planner, q.Planner}, {&r.Format, o.Format},
	} {
		if *f.dst == "" {
			*f.dst = f.src
		}
	}
	if len(p.Perks) == 0 {
		p.Perks = q.Perks
	}
	if p.Seed == 0 {
		p.Seed = q.Seed
	}
	if r.Deadline == 0 {
		r.Deadline = o.Deadline
	}
	if r.End == nil {
		r.End = o.End
	}
}
