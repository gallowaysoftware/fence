package vod

import (
	"fmt"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
)

// Kit is everything one VOD needs.
type Kit struct {
	Part      int           `json:"part"`
	Of        int           `json:"of"`
	Recording string        `json:"recording,omitempty"`
	Start     time.Time     `json:"start"`
	Length    time.Duration `json:"-"`
	LengthS   int           `json:"length_s"`
	FromDay   int           `json:"from_day"`
	ToDay     int           `json:"to_day"`
	Run       string        `json:"run,omitempty"`

	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	Chapters    []Chapter `json:"chapters"`
	Clips       []Clip    `json:"clips"`
}

// Make builds a part's kit; runs is the timeline's (Timeline.Runs).
func Make(p *Part, runs map[string]*Run, clips int) Kit {
	from, to := p.Days()
	k := Kit{Part: p.N, Of: p.Of, Recording: p.Rec.File, Start: p.Rec.Start, Length: p.Rec.Length, LengthS: int(p.Rec.Length / time.Second), FromDay: from, ToDay: to}
	r := mainRun(p, runs)
	if r != nil {
		k.Run = r.ID
	}
	k.Chapters = Chapters(p)
	k.Clips = Highlights(p, clips)
	k.Title = title(r, p)
	k.Description = description(r, p, k)
	k.Tags = tags(r)
	return k
}

// mainRun is the run the part shows most of.
func mainRun(p *Part, runs map[string]*Run) *Run {
	count := map[string]int{}
	best := ""
	for _, pt := range p.Points {
		if pt.Ev.Run == "" {
			continue
		}
		count[pt.Ev.Run]++
		if best == "" || count[pt.Ev.Run] > count[best] {
			best = pt.Ev.Run
		}
	}
	if r := runs[best]; r != nil {
		return r
	}
	return &Run{ID: best}
}

func persona(r *Run) string {
	if r.Player.PersonaName != "" {
		return "the " + r.Player.PersonaName
	}
	return "an AI"
}

func model(r *Run) string {
	if r.Player.Model != "" {
		return r.Player.Model
	}
	if r.Player.ID != "" {
		return r.Player.ID
	}
	return "An LLM"
}

// title says who plays, as whom and how hard, and never how it ends. YouTube cuts titles at 100
// characters.
func title(r *Run, p *Part) string {
	var b strings.Builder
	if r.Format != "" {
		b.WriteString(r.Format + ": ")
	}
	fmt.Fprintf(&b, "%s plays Probably Stolen as %s", model(r), persona(r))
	var how []string
	if d := r.Player.Difficulty; d != "" {
		how = append(how, d)
	}
	if rt := r.Player.Route; rt != "" {
		how = append(how, "the "+rt+" route")
	}
	if len(how) > 0 {
		b.WriteString(" (" + strings.Join(how, ", ") + ")")
	}
	if p.Of > 1 {
		fmt.Fprintf(&b, ", part %d of %d", p.N, p.Of)
	}
	return clip(b.String(), 100)
}

// SpoilerLine is the line below which the description gives the result away.
const SpoilerLine = "⚠ Spoilers below: how the run ends."

func description(r *Run, p *Part, k Kit) string {
	var b strings.Builder
	pl := r.Player
	fmt.Fprintf(&b, "A language model runs a stolen-goods shop, live. %s plays Probably Stolen as %s", model(r), persona(r))
	if pl.PersonaTagline != "" {
		fmt.Fprintf(&b, " (%s)", strings.TrimRight(pl.PersonaTagline, "."))
	}
	b.WriteString(": every morning it writes the plan, a rule-based autopilot works the counter, and a narrator speaks the big moments in its voice. Every number the narrator says is checked against the game first.\n\n")
	if k.FromDay > 0 {
		if p.Of > 1 {
			fmt.Fprintf(&b, "Part %d of %d: days %d to %d.\n", p.N, p.Of, k.FromDay, k.ToDay)
		} else {
			fmt.Fprintf(&b, "Days %d to %d.\n", k.FromDay, k.ToDay)
		}
	}
	if r.Format != "" {
		fmt.Fprintf(&b, "Format: %s", r.Format)
		if r.Deadline > 0 {
			fmt.Fprintf(&b, ", the shop bought by day %d", r.Deadline)
		}
		b.WriteString(".\n")
	}
	if pl.Route != "" {
		fmt.Fprintf(&b, "Route: %s", pl.Route)
		if pl.RouteBrief != "" {
			fmt.Fprintf(&b, ". %s", firstSentence(pl.RouteBrief))
		}
		b.WriteString("\n")
	}
	var setup []string
	if pl.Difficulty != "" {
		setup = append(setup, "difficulty "+pl.Difficulty)
	}
	if pl.Start != "" {
		setup = append(setup, pl.Start+" start")
	}
	if len(pl.Perks) > 0 {
		setup = append(setup, "perks "+strings.Join(pl.Perks, " + "))
	}
	if pl.Seed != 0 {
		setup = append(setup, fmt.Sprintf("seed %d", pl.Seed))
	}
	if len(setup) > 0 {
		b.WriteString("Setup: " + strings.Join(setup, ", ") + ".\n")
	}
	b.WriteString("Fair play: the AI sees only what a player sees on screen.\n")
	if len(k.Chapters) > 0 {
		b.WriteString("\nChapters\n")
		for _, c := range k.Chapters {
			fmt.Fprintf(&b, "%s %s\n", c.Stamp, c.Title)
		}
	}
	b.WriteString("\nProbably Stolen is developed by Questing Goose Studio and published by tinyBuild. Copyright © tinyBuild, LLC. All Rights Reserved. www.tinybuild.com\n")
	b.WriteString("This channel is not affiliated with them; the AI plays through our own mod.\n")
	b.WriteString("\n\n\n" + SpoilerLine + "\n\n\n")
	b.WriteString(result(r, p, k))
	return b.String()
}

// result says how the run ended, or where this part leaves it.
func result(r *Run, p *Part, k Kit) string {
	if e := r.End; e != nil && endsIn(p, r.ID) {
		var s string
		switch {
		case show.Won(e.Ending):
			s = fmt.Sprintf("Result: the shop is bought on day %d", e.Day)
		default:
			s = fmt.Sprintf("Result: the run ends on day %d (%s)", e.Day, strings.ReplaceAll(e.Ending, "_", " "))
		}
		if e.NetWorth > 0 {
			s += fmt.Sprintf(", net worth %s credits", Num(e.NetWorth))
		}
		s += "."
		if r.Deadline > 0 && show.Won(e.Ending) {
			if e.Day <= r.Deadline {
				s += fmt.Sprintf(" Inside the format's day %d deadline.", r.Deadline)
			} else {
				s += fmt.Sprintf(" Past the format's day %d deadline.", r.Deadline)
			}
		}
		return s + "\n"
	}
	if c := lastClock(p); c != nil {
		return fmt.Sprintf("Where this part leaves it: day %d, cash %s, net worth %s credits. To be continued.\n", k.ToDay, Num(c.Cash), Num(c.NetWorth))
	}
	return "To be continued.\n"
}

func endsIn(p *Part, run string) bool {
	for _, pt := range p.Points {
		if pt.Ev.Kind == show.KindRunEnd && pt.Ev.Run == run {
			return true
		}
	}
	return false
}

func lastClock(p *Part) *show.Clock {
	for i := len(p.Points) - 1; i >= 0; i-- {
		if c := p.Points[i].Ev.Clock; c != nil && c.NetWorth > 0 {
			return c
		}
	}
	return nil
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

// tags fit YouTube's 500 characters with room to spare.
func tags(r *Run) []string {
	t := []string{"Probably Stolen", "AI plays", "LLM", "AI agent", "shop simulator", "tinyBuild", "Questing Goose Studio", "AI livestream"}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		for _, x := range t {
			if strings.EqualFold(x, s) {
				return
			}
		}
		t = append(t, s)
	}
	pl := r.Player
	add(pl.Model)
	if fam, _, ok := strings.Cut(pl.Model, "-"); ok {
		add(fam)
	}
	add(pl.PersonaName)
	if pl.Route != "" {
		add(pl.Route + " route")
	}
	if pl.Difficulty != "" {
		add(pl.Difficulty + " difficulty")
	}
	add(r.Format)
	n := 0
	for i, s := range t {
		n += len(s) + 1
		if n > 450 {
			return t[:i]
		}
	}
	return t
}
