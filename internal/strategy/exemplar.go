package strategy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// MaxExemplarBytes bounds the SIMILAR SITUATIONS block in the plan prompt.
const MaxExemplarBytes = 800

// Exemplar is one day of strong play: where the shop stood, what the player chose and what
// followed. An index of them comes from step-mode games (sim/gym/README.md, "Playing as an
// agent"), built by BuildExemplars.
type Exemplar struct {
	Run        string    `json:"run"`
	Difficulty string    `json:"difficulty"`
	Day        int       `json:"day"`
	Features   []float64 `json:"features"`
	Situation  string    `json:"situation"`
	Chosen     string    `json:"chosen"`
	Next       string    `json:"next"`
	// Weight is the run's outcome, 0..1: how far it lived and how it ended.
	Weight float64 `json:"weight"`
}

// Exemplars is a loaded index.
type Exemplars []Exemplar

// LoadExemplars reads an index written by BuildExemplars (JSON lines).
func LoadExemplars(path string) (Exemplars, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out Exemplars
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e Exemplar
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// features places a morning in a space where distance means "a similar situation". Each
// coordinate is scaled to roughly 0..1 so none dominates.
func (s *snapshot) features(day int) []float64 {
	f := func(p *int64) float64 {
		if p == nil {
			return 0
		}
		return float64(*p)
	}
	cash := f(s.Cash)
	due, _ := s.Rent.due()
	cover := math.Min(cash/math.Max(float64(due), 1), 5) / 5
	daysLeft := 7.0
	if s.Rent != nil && s.Rent.DaysLeft != nil {
		daysLeft = float64(*s.Rent.DaysLeft)
	}
	var stock, wines float64
	for _, it := range s.Inventory {
		if it.EstimatedValue != nil && *it.EstimatedValue > 0 {
			stock += float64(*it.EstimatedValue)
		}
		if _, ok := wineGrade(it); ok {
			wines++
		}
	}
	permit := 0.0
	if s.Permit != nil && *s.Permit {
		permit = 1
	}
	var evidence float64
	if s.Security != nil {
		if n, ok := leadingInt(strings.TrimSpace(s.Security.Evidence)); ok {
			evidence = float64(n)
		}
	}
	gap := 1.0
	if p := s.Property; p != nil && p.BuyoutPrice != nil && *p.BuyoutPrice > 0 {
		gap = math.Max(0, float64(*p.BuyoutPrice)-f(s.NetWorth)) / float64(*p.BuyoutPrice)
	}
	out := []float64{
		float64(min(day, 200)) / 200,
		cover,
		daysLeft / 7,
		math.Min(cash/3000, 1),
		math.Min(stock/5000, 1),
		math.Min(wines/20, 1),
		permit,
		math.Min(evidence/100, 1),
		math.Min(f(s.WildFavor)/1500, 1),
		math.Min(float64(len(s.UpgradesOwned))/12, 1),
		gap,
	}
	for _, id := range []string{"lower_levels", "upper_levels", "security", "black_market", "revolution"} {
		v, _ := s.rep(id)
		out = append(out, math.Max(-1, math.Min(v/100, 1))/2)
	}
	return out
}

func (s *snapshot) difficulty() string {
	return nonEmpty(strings.ToLower(s.storeExtra["difficulty"]), "normal")
}

// situation is a one-line summary of a morning for the prompt.
func (s *snapshot) situation(day int) string {
	var b []string
	b = append(b, fmt.Sprintf("day %d", day))
	if s.Cash != nil {
		b = append(b, fmt.Sprintf("cash %d", *s.Cash))
	}
	if due, ok := s.Rent.due(); ok && s.Rent.DaysLeft != nil {
		b = append(b, fmt.Sprintf("rent %d in %dd", due, *s.Rent.DaysLeft))
	}
	if s.NetWorth != nil {
		b = append(b, fmt.Sprintf("nw %d", *s.NetWorth))
	}
	if s.WildFavor != nil && *s.WildFavor > 0 {
		b = append(b, fmt.Sprintf("favor %d", *s.WildFavor))
	}
	if s.Permit != nil && !*s.Permit {
		b = append(b, "no permit")
	}
	var wines int
	for _, it := range s.Inventory {
		if _, ok := wineGrade(it); ok {
			wines++
		}
	}
	if wines > 0 {
		b = append(b, fmt.Sprintf("%d wines", wines))
	}
	return strings.Join(b, ", ")
}

// chosenText summarises a day's directives (the levers, not the counter details).
func chosenText(raw json.RawMessage, s *snapshot) string {
	var d struct {
		CashFloor   *int64   `json:"cash_floor"`
		Bargain     string   `json:"bargain"`
		Permit      string   `json:"permit"`
		Engine      string   `json:"engine"`
		Services    []string `json:"services"`
		UpgradesBuy []string `json:"upgrades_buy"`
		Buy         *struct {
			MaxSpend *int64 `json:"max_spend"`
		} `json:"buy"`
		Sell *struct {
			SellDownToCash *int64  `json:"sell_down_to_cash"`
			HoldUIDs       []int64 `json:"hold_uids"`
		} `json:"sell"`
	}
	if json.Unmarshal(raw, &d) != nil {
		return ""
	}
	var b []string
	var ups []string
	for _, id := range d.UpgradesBuy {
		if !s.owns(id) {
			ups = append(ups, id)
		}
	}
	if len(ups) > 0 {
		b = append(b, "buy "+strings.Join(ups[:min(len(ups), 3)], "+"))
	}
	if d.Permit != "" && (s.Permit == nil || !*s.Permit) {
		b = append(b, "permit "+d.Permit)
	}
	if d.Sell != nil && d.Sell.SellDownToCash != nil && s.Cash != nil && *d.Sell.SellDownToCash > *s.Cash {
		b = append(b, fmt.Sprintf("sell down to %d", *d.Sell.SellDownToCash))
	}
	if d.Sell != nil && len(d.Sell.HoldUIDs) > 0 {
		b = append(b, fmt.Sprintf("hold %d", len(d.Sell.HoldUIDs)))
	}
	if d.Buy != nil && d.Buy.MaxSpend != nil {
		b = append(b, fmt.Sprintf("spend ≤%d", *d.Buy.MaxSpend))
	}
	if d.Engine != "" {
		b = append(b, "engine "+d.Engine)
	}
	if d.Bargain != "" {
		b = append(b, "bargain "+d.Bargain)
	}
	return strings.Join(b, ", ")
}

// Similar renders the SIMILAR SITUATIONS block for a morning: the nearest exemplars of the
// same difficulty, nearer and stronger first, at most two per run and five days apart,
// within MaxExemplarBytes. "" when the index has nothing to say.
func (x Exemplars) Similar(raw json.RawMessage, day, k int) string {
	if len(x) == 0 {
		return ""
	}
	s, err := parseSnapshot(raw)
	if err != nil {
		return ""
	}
	day = s.day(day)
	q := s.features(day)
	diff := s.difficulty()
	type cand struct {
		e     *Exemplar
		score float64
	}
	var cs []cand
	for i := range x {
		e := &x[i]
		if e.Difficulty != diff || e.Weight <= 0 || len(e.Features) != len(q) {
			continue
		}
		var d float64
		for j := range q {
			d += (q[j] - e.Features[j]) * (q[j] - e.Features[j])
		}
		// The offset keeps a weak run's exact match from beating a strong run's near one.
		cs = append(cs, cand{e, (math.Sqrt(d) + 0.1) / e.Weight})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].score < cs[j].score })
	var picked []*Exemplar
	for _, c := range cs {
		if len(picked) == k {
			break
		}
		same := 0
		for _, p := range picked {
			if p.Run == c.e.Run {
				same++
				if abs(p.Day-c.e.Day) < 5 {
					same = 2
				}
			}
		}
		if same >= 2 {
			continue
		}
		picked = append(picked, c.e)
	}
	if len(picked) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("SIMILAR SITUATIONS (strong play in other shops; the levers chosen and what followed, not orders for today):\n")
	per := (MaxExemplarBytes - b.Len()) / len(picked)
	for _, e := range picked {
		b.WriteString(clip(fmt.Sprintf("- %s -> %s -> %s", e.Situation, nonEmpty(e.Chosen, "standing orders"), e.Next), per-1) + "\n")
	}
	return b.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// BuildExemplars indexes step-mode game directories (day-NNN/{snapshot,directives}.json and
// final.json): one exemplar per answered day, with the next five days' outcome and the
// run's weight.
func BuildExemplars(dirs []string) (Exemplars, error) {
	var out Exemplars
	for _, dir := range dirs {
		var final struct {
			GameOver   *string `json:"game_over"`
			DaysPlayed int     `json:"days_played"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, "final.json")); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		} else if err := json.Unmarshal(b, &final); err != nil {
			return nil, fmt.Errorf("%s/final.json: %w", dir, err)
		}
		weight := math.Min(float64(final.DaysPlayed)/200, 1)
		switch {
		case final.GameOver != nil && strings.HasPrefix(*final.GameOver, "buyout"):
			weight = 2 // a won run is the top source
		case final.GameOver != nil && (*final.GameOver == "arrest" || *final.GameOver == "execution"):
			weight /= 2
		}
		type day struct {
			n    int
			s    *snapshot
			raw  json.RawMessage
			dirs json.RawMessage
		}
		var days []day
		paths, _ := filepath.Glob(filepath.Join(dir, "day-*"))
		sort.Strings(paths)
		for _, p := range paths {
			var w struct {
				Day      int             `json:"day"`
				Snapshot json.RawMessage `json:"snapshot"`
			}
			b, err := os.ReadFile(filepath.Join(p, "snapshot.json"))
			if err != nil || json.Unmarshal(b, &w) != nil {
				continue
			}
			s, err := parseSnapshot(w.Snapshot)
			if err != nil {
				continue
			}
			d, _ := os.ReadFile(filepath.Join(p, "directives.json"))
			days = append(days, day{w.Day, s, w.Snapshot, d})
		}
		run := filepath.Base(filepath.Clean(dir))
		for i, d := range days {
			if len(d.dirs) == 0 {
				continue
			}
			next := "run ended"
			if j := min(i+5, len(days)-1); j > i {
				a, z := days[i].s, days[j].s
				next = fmt.Sprintf("by day %d nw %d->%d, cash %d->%d", days[j].n, deref(a.NetWorth), deref(z.NetWorth), deref(a.Cash), deref(z.Cash))
				if a.Permit != nil && z.Permit != nil && !*a.Permit && *z.Permit {
					next += ", permit bought"
				}
				var bought []string
				for _, id := range z.UpgradesOwned {
					if !slices.Contains(a.UpgradesOwned, id) {
						bought = append(bought, id)
					}
				}
				if len(bought) > 0 {
					next += ", bought " + strings.Join(bought, "+")
				}
			}
			if i+5 >= len(days) && final.GameOver != nil {
				next += fmt.Sprintf("; game over (%s) day %d", *final.GameOver, final.DaysPlayed+1)
			}
			out = append(out, Exemplar{Run: run, Difficulty: d.s.difficulty(), Day: d.n, Features: d.s.features(d.n),
				Situation: d.s.situation(d.n), Chosen: chosenText(d.dirs, d.s), Next: next, Weight: weight})
		}
	}
	return out, nil
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
