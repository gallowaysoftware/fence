package format

import "fmt"

// Card is a format as the panel's name card shows it and the narrator is told it.
type Card struct {
	Name       string   `json:"name"`
	Title      string   `json:"title"`
	Card       string   `json:"card"`
	Difficulty string   `json:"difficulty"`
	Handicaps  []string `json:"handicaps,omitempty"`
	// DeadlineDay, when set, is the last day a buyout wins the format.
	DeadlineDay int `json:"deadline_day,omitempty"`
	// Lab is the executor's record in digits ("won 31 times in 60 (about 5 in 10), median day 91");
	// LabPct its win share in percent, -1 when untested.
	Lab    string `json:"lab"`
	LabPct int    `json:"lab_pct"`
	// Attempts is the local record of played runs so far.
	Attempts string `json:"attempts"`
	// Pitch and Stakes are for the narrator: the premise and the odds in words, without a number.
	Pitch  string `json:"pitch"`
	Stakes string `json:"stakes"`
}

// CardOf is f's card with the attempts so far.
func CardOf(f Format, t Tally) Card {
	c := Card{Name: f.Name, Title: f.Title, Card: f.Card, Difficulty: f.Difficulty, Handicaps: f.HandicapLabels(),
		DeadlineDay: f.DeadlineDay, Attempts: t.Text(), Pitch: f.Pitch, Stakes: f.Lab.Words(), LabPct: -1}
	c.Lab = "untested in the lab"
	if l := f.Lab; l.Runs > 0 {
		c.LabPct = int(100*l.Rate() + 0.5)
		c.Lab = "the lab wins it " + l.Odds()
		if l.MedianWinDay > 0 {
			c.Lab += fmt.Sprintf(", median win day %d", l.MedianWinDay)
		}
	}
	return c
}

// Caption is the format as the run's start says it; its numbers become facts the narrator may
// speak at that moment.
func (c Card) Caption() string {
	s := "Format: " + c.Title
	if c.DeadlineDay > 0 {
		s += fmt.Sprintf(", the shop bought by day %d", c.DeadlineDay)
	}
	return s + "; " + c.Lab + "."
}

// Verdict is the format's result as the run's end says it: "" when the game's own ending says it all.
func (c Card) Verdict(won bool, day int) string {
	switch {
	case c.DeadlineDay == 0:
		return ""
	case won && day <= c.DeadlineDay:
		return fmt.Sprintf("%s won: bought by the day %d deadline.", c.Title, c.DeadlineDay)
	case won:
		return fmt.Sprintf("%s lost: the deadline was day %d.", c.Title, c.DeadlineDay)
	}
	return ""
}
