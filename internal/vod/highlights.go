package vod

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
)

// Clip is a highlight window for a Short or a Twitch clip.
type Clip struct {
	Start time.Duration `json:"-"`
	End   time.Duration `json:"-"`
	// StartS and EndS are Start and End in whole seconds.
	StartS  int    `json:"start_s"`
	EndS    int    `json:"end_s"`
	Day     int    `json:"day"`
	Caption string `json:"caption"`
	// Line is the narrator's line the clip pays off on, "" when it has none.
	Line string `json:"line,omitempty"`
	// Exchange is the co-host's exchange over the moment, each line led by its speaker's name.
	Exchange []string `json:"exchange,omitempty"`
	Kind     string   `json:"kind"`
	Beat     string   `json:"beat,omitempty"`
	Score    int      `json:"score"`
}

const (
	minClip = 30 * time.Second
	maxClip = 60 * time.Second
	// lead is the setup shown before the moment; after is how long the moment plays out
	// without a line.
	lead  = 12 * time.Second
	after = 18 * time.Second
	// sayWithin is how late a narrator line can follow its moment: the narrator's -max-age plus
	// -max-lag, doubled for a small beat.
	sayWithin = 80 * time.Second
	// wordsPerSecond is Kokoro's pace, for when a line's audio ends.
	wordsPerSecond = 2.6
)

// lineBonus is what a fact-checked line of the model's own (not the caption read out) adds:
// a clip with a payoff beats a louder moment told in silence.
const lineBonus = 15

// exchangeBonus is what a co-host exchange over the moment adds: a second voice disagreeing is a
// clip with a back-and-forth.
const exchangeBonus = 10

// crowding is what each clip already near a candidate costs it.
const crowding = 10

// Highlights picks up to n clip windows from the part's moments: ranked by show.Priority, a
// narrated line worth lineBonus, each repeat of a kind and beat costing diversity, none
// overlapping.
func Highlights(p *Part, n int) []Clip {
	type cand struct {
		pt    Point
		say   *Point
		ex    []Point
		score int
	}
	var cands []cand
	for i, pt := range p.Points {
		ev := pt.Ev
		if ev.Level < show.LevelMoment || ev.Kind == show.KindDay || ev.Kind == show.KindSay {
			continue
		}
		s := show.Priority(ev) * 10
		if ev.Beat == show.BeatMorning {
			// A plan read aloud is the slowest minute of the day.
			s -= 10
		}
		c := cand{pt: pt, score: s}
		if sp := lineFor(p, i); sp != nil {
			c.say = sp
			if sp.Ev.Say.Source == "llm" {
				c.score += lineBonus
			}
		}
		if c.ex = exchangeFor(p, pt); len(c.ex) > 0 {
			c.score += exchangeBonus
		}
		cands = append(cands, c)
	}
	var out []Clip
	used := map[string]int{}
	overlaps := func(a, b time.Duration) bool {
		for _, c := range out {
			if a < c.End && c.Start < b {
				return true
			}
		}
		return false
	}
	// near is how close two clips may be before the later pick pays crowding: the clips should
	// cover the whole session, not its busy first hour.
	near := p.Rec.Length / time.Duration(2*max(n, 1))
	crowd := func(at time.Time) int {
		k := 0
		for _, c := range out {
			if d := p.Off(at) - c.Start; d < near && d > -near {
				k++
			}
		}
		return k
	}
	for len(out) < n && len(cands) > 0 {
		best, bestScore := -1, 0
		for i, c := range cands {
			s := c.score - diversity*used[string(c.pt.Ev.Kind)+"/"+c.pt.Ev.Beat] - crowding*crowd(c.pt.At)
			if best < 0 || s > bestScore {
				best, bestScore = i, s
			}
		}
		c := cands[best]
		cands = slices.Delete(cands, best, best+1)
		last := c.say
		if len(c.ex) > 0 {
			last = &c.ex[len(c.ex)-1]
		}
		start, end, ok := window(p, c.pt, last)
		if !ok || overlaps(start, end) {
			continue
		}
		used[string(c.pt.Ev.Kind)+"/"+c.pt.Ev.Beat]++
		cl := Clip{Start: start, End: end, StartS: int(start / time.Second), EndS: int(end / time.Second), Day: c.pt.Ev.Day, Kind: string(c.pt.Ev.Kind), Beat: c.pt.Ev.Beat, Score: bestScore,
			Caption: clipCaption(c.pt.Ev, c.say)}
		if c.say != nil {
			cl.Line = c.say.Ev.Say.Line
		}
		for _, e := range c.ex {
			cl.Exchange = append(cl.Exchange, e.Ev.Say.Name+": "+e.Ev.Say.Line)
		}
		out = append(out, cl)
	}
	slices.SortStableFunc(out, func(a, b Clip) int { return int(a.Start - b.Start) })
	return out
}

// lineFor is the narrator line that told the part's i-th point: a line whose own feed names the
// moment (same source time and kind, or same seq), else the first line within sayWithin that no
// louder or later moment claims first.
func lineFor(p *Part, i int) *Point {
	pt := p.Points[i]
	j, _ := slices.BinarySearchFunc(p.Says, pt.At, func(s Point, t time.Time) int { return s.At.Compare(t) })
	for ; j < len(p.Says) && p.Says[j].At.Sub(pt.At) <= sayWithin; j++ {
		say := p.Says[j]
		if m := say.Told; m != nil {
			if (m.TS != "" && m.TS == pt.Ev.TS && m.Kind == pt.Ev.Kind) || (m.TS == "" && m.Seq == pt.Ev.Seq) {
				return &say
			}
			continue
		}
		for _, q := range p.Points[i+1:] {
			if !q.At.Before(say.At) {
				break
			}
			if q.Ev.Level >= show.LevelMoment && q.Ev.Kind != show.KindDay && show.Priority(q.Ev) >= show.Priority(pt.Ev) {
				return nil
			}
		}
		return &say
	}
	return nil
}

// exchangeFor is the co-host exchange over a moment: the lines whose own feed names it.
func exchangeFor(p *Part, pt Point) []Point {
	var out []Point
	for _, e := range p.Exchanges {
		if m := e.Told; m != nil && ((m.TS != "" && m.TS == pt.Ev.TS && m.Kind == pt.Ev.Kind) || (m.TS == "" && m.Seq == pt.Ev.Seq)) {
			out = append(out, e)
		}
	}
	return out
}

// window frames a clip: the setup before the moment, then the moment, then the line and a breath
// after it, held between 30 and 60 s and inside the recording.
func window(p *Part, pt Point, say *Point) (time.Duration, time.Duration, bool) {
	at := p.Off(pt.At)
	start, end := at-lead, at+after
	if say != nil {
		words := len(strings.Fields(say.Ev.Say.Line))
		spoken := time.Duration(float64(words)/wordsPerSecond*float64(time.Second)) + time.Second
		end = max(end, p.Off(say.At)+spoken+3*time.Second)
	}
	if end-start < minClip {
		start = end - minClip
	}
	if end-start > maxClip {
		start = end - maxClip
	}
	length := p.Rec.Length
	if start < 0 {
		end, start = min(end-start, length), 0
	}
	if end > length {
		start, end = max(0, start-(end-length)), length
	}
	start, end = start.Truncate(time.Second), end.Truncate(time.Second)
	return start, end, end-start >= minClip
}

func clipCaption(ev show.Event, say *Point) string {
	if say != nil && say.Ev.Say.Source == "llm" {
		return fmt.Sprintf("Day %d: “%s”", ev.Day, say.Ev.Say.Line)
	}
	return chapterTitle(ev)
}
