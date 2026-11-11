package format

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
)

// Rate is the lab's win share, 0 without runs.
func (l Lab) Rate() float64 {
	if l.Runs == 0 {
		return 0
	}
	return float64(l.Wins) / float64(l.Runs)
}

// Odds is the record in words and digits: "31 times in 60 (about 5 in 10)".
func (l Lab) Odds() string {
	if l.Runs == 0 {
		return "untested"
	}
	if l.Wins == 1 {
		return fmt.Sprintf("once in %s", group(l.Runs))
	}
	s := fmt.Sprintf("%s times in %s", group(l.Wins), group(l.Runs))
	if r := Roughly(l.Wins, l.Runs); r != "" {
		s += " (" + r + ")"
	}
	return s
}

// Roughly is a share as a viewer says it: "about 4 in 10", "about 1 in 30"; "" when it is all or
// nothing, which the counts already say.
func Roughly(wins, runs int) string {
	if runs == 0 || wins == 0 || wins == runs {
		return ""
	}
	p := float64(wins) / float64(runs)
	if k := math.Round(p * 10); p >= 0.1 && k < 10 {
		return fmt.Sprintf("about %d in 10", int(k))
	}
	if p > 0.9 {
		return fmt.Sprintf("about %d in %d", int(math.Round(1/(1-p)))-1, int(math.Round(1/(1-p))))
	}
	return fmt.Sprintf("about 1 in %s", group(int(math.Round(1/p))))
}

// Words is the lab's odds without a number, for the narrator's system prompt: a number there
// would be one the fact check rejects in every line but the run's start.
func (l Lab) Words() string {
	p := l.Rate()
	switch {
	case l.Runs == 0:
		return "nobody knows the odds: the lab has not played it"
	case p < 0.02:
		return "a long shot: the lab has almost never won it"
	case p < 0.2:
		return "long odds: the lab loses it far more often than it wins"
	case p < 0.4:
		return "the lab loses it more often than it wins"
	case p < 0.6:
		return "a coin flip: the lab wins it about half the time"
	case p < 0.8:
		return "the lab wins it more often than not, but it can go either way"
	case p < 0.98:
		return "the lab usually wins it; a loss would be an upset"
	}
	return "the lab wins it every time; only a blunder loses it"
}

// Jeopardy is whether the odds make good television: a real chance of losing and of winning.
func (l Lab) Jeopardy() bool {
	p := l.Rate()
	return l.Runs > 0 && p >= 0.2 && p <= 0.8
}

func group(n int) string {
	s := fmt.Sprint(n)
	if n < 1000 {
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// Run is one gym run's outcome as the lab counts it.
type Run struct {
	Seed   int64  `json:"seed"`
	Ending string `json:"ending"`
	Day    int    `json:"day"`
}

// ReadSummary reads a gym run summary (fence-gym run -summary).
func ReadSummary(path string) (Run, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Run{}, err
	}
	var s struct {
		Seed     int64  `json:"seed"`
		GameOver string `json:"game_over"`
		LastDay  int    `json:"last_day"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Run{}, fmt.Errorf("%s: %w", path, err)
	}
	if s.GameOver == "" {
		s.GameOver = "alive"
	}
	return Run{Seed: s.Seed, Ending: s.GameOver, Day: s.LastDay}, nil
}

// Tally is the lab record of runs on the format: a buyout after the deadline is a loss, counted
// as "late".
func (f Format) Tally(runs []Run) Lab {
	l := Lab{Runs: len(runs), Endings: map[string]int{}}
	var ends, wins []int
	for _, r := range runs {
		ends = append(ends, r.Day)
		ending := r.Ending
		if f.Won(r.Ending, r.Day) {
			l.Wins++
			wins = append(wins, r.Day)
		} else if strings.HasPrefix(r.Ending, "buyout") {
			ending = "late"
		}
		l.Endings[ending]++
	}
	l.MedianEndDay, l.MedianWinDay = median(ends), median(wins)
	return l
}

func median(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	xs = slices.Clone(xs)
	slices.Sort(xs)
	n := len(xs)
	if n%2 == 1 {
		return xs[n/2]
	}
	return int(math.Round(float64(xs[n/2-1]+xs[n/2]) / 2))
}

// Wilson is the 95 % Wilson score interval of wins in runs, in percent.
func Wilson(wins, runs int) (lo, hi float64) {
	if runs == 0 {
		return 0, 100
	}
	const z = 1.96
	n, p := float64(runs), float64(wins)/float64(runs)
	den := 1 + z*z/n
	mid := (p + z*z/(2*n)) / den
	half := z * math.Sqrt(p*(1-p)/n+z*z/(4*n*n)) / den
	return 100 * math.Max(0, mid-half), 100 * math.Min(1, mid+half)
}
