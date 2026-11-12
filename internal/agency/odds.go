package agency

import (
	_ "embed"
	"encoding/json"

	"github.com/gallowaysoftware/fence/internal/format"
)

//go:embed deadline_odds.json
var deadlineJSON []byte

// Odds are a route's buyouts by a format's deadline in the lab, executor alone, the format's
// handicaps applied; Lo and Hi bound the Wilson 95 % interval, in percent.
type Odds struct {
	Wins         int `json:"wins"`
	Runs         int `json:"runs"`
	Lo           int `json:"lo"`
	Hi           int `json:"hi"`
	MedianWinDay int `json:"median_win_day"`
}

// Pct is the win share in percent, rounded.
func (o Odds) Pct() int {
	if o.Runs == 0 {
		return 0
	}
	return (200*o.Wins + o.Runs) / (2 * o.Runs)
}

// FormatOdds are every menu route's odds on one deadline format.
type FormatOdds struct {
	DeadlineDay int             `json:"deadline_day"`
	Handicaps   []string        `json:"handicaps"`
	Seeds       string          `json:"seeds"`
	Routes      map[string]Odds `json:"routes"`
}

// deadlineOdds are deadline_odds.json's formats; a file that does not parse fails every test.
var deadlineOdds = func() map[string]FormatOdds {
	var f struct {
		Formats map[string]FormatOdds `json:"formats"`
	}
	if err := json.Unmarshal(deadlineJSON, &f); err != nil {
		panic("agency/deadline_odds.json: " + err.Error())
	}
	return f.Formats
}()

// DeadlineOdds are the odds measured for the format named name (sim/lab/scripts/deadline_odds.py);
// ok is false for a format without a deadline or not measured.
func DeadlineOdds(name string) (FormatOdds, bool) {
	o, ok := deadlineOdds[name]
	return o, ok
}

// DeadlineLab is route's record on the format as the format card counts it; ok is false when the
// route was not measured on it.
func DeadlineLab(f format.Format, route string) (format.Lab, bool) {
	fo, ok := DeadlineOdds(f.Name)
	if !ok || fo.DeadlineDay != f.DeadlineDay {
		return format.Lab{}, false
	}
	o, ok := fo.Routes[route]
	if !ok || o.Runs == 0 {
		return format.Lab{}, false
	}
	return format.Lab{Route: route, Seeds: fo.Seeds, Runs: o.Runs, Wins: o.Wins, MedianWinDay: o.MedianWinDay}, true
}
