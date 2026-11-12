package show

import (
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/agency"
	"github.com/gallowaysoftware/fence/internal/format"
)

// SetFormat puts a stream format (internal/format) on the name card: its title, handicaps, lab
// odds and the attempts in the local record so far, except the run except. The format's route
// stands in for a run that names none; set the run's route first.
func (p *Player) SetFormat(dir, name, record, except string) error {
	if name == "" {
		return nil
	}
	f, err := format.Load(dir, name)
	if err != nil {
		return err
	}
	t, err := format.ReadRecord(record, f.Name, except)
	if err != nil {
		return err
	}
	if p.Route == "" && f.Route != "" {
		p.SetRoute(f.Route)
	}
	// A deadline's odds depend on the route (0 to 54 wins in 60 by day 91 on sprint_91), so
	// the card quotes the run's own route when the lab measured it under the format.
	l, own := agency.DeadlineLab(f, p.Route)
	if own {
		f.Lab = l
	}
	c := format.CardOf(f, t)
	if own {
		c.Lab = "on the " + p.Route + " route, " + c.Lab
	}
	p.Format = &c
	if p.Difficulty == "" {
		p.Difficulty = f.Difficulty
	}
	return nil
}

// formatStakes tells the narrator the run's format and the odds in words: a number in the system
// prompt would be one the fact check rejects in every line.
func formatStakes(p *Player) string {
	if p == nil || p.Format == nil {
		return ""
	}
	c := p.Format
	var b strings.Builder
	fmt.Fprintf(&b, "This run is a challenge format, %s: %s The stakes: %s.", c.Title, c.Pitch, c.Stakes)
	if c.DeadlineDay > 0 {
		b.WriteString(" There is a deadline: the shop must be bought by a set day, shown on screen; say the day only when your facts give it.")
	}
	b.WriteString(" Let the stakes colour your lines; never quote odds or counts the facts do not give.\n")
	return b.String()
}

// FormatEnd adds the format's verdict to a run's end and returns the attempt it records; ok is
// false for an event that is not a run's end or a player without a format.
func FormatEnd(ev *Event, p *Player, source string) (format.Attempt, bool) {
	if ev.Kind != KindRunEnd || ev.End == nil || p == nil || p.Format == nil {
		return format.Attempt{}, false
	}
	c, e := p.Format, ev.End
	if v := c.Verdict(e.Won, e.Day); v != "" {
		ev.Text = strings.TrimSpace(ev.Text + " " + v)
	}
	return format.Attempt{Format: c.Name, Run: ev.Run, Source: source, Model: p.Model, Persona: p.Persona, Route: p.Route,
		Seed: p.Seed, Ending: e.Ending, Day: e.Day, Won: e.Won && (c.DeadlineDay == 0 || e.Day <= c.DeadlineDay)}, true
}
