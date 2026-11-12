package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/agency"
)

// fingerprintDays is the window TRADING-STYLES.md fingerprints a style over.
const fingerprintDays = 98

// fingerprint is a subset of docs/research/TRADING-STYLES.md's trading fingerprint, pooled over
// a block's runs: resale (wine, water and engine products left out) and buying a week, the
// share bought in wholesale lots and hot, the stock held, the share of sales into a market
// event and to Upper Levels buyers, and the resale mix by type.
type fingerprint struct {
	SalesWeek, BoughtWeek, WholesalePct, HotPct, Stock, EventPct, ULPct float64
	Mix                                                                 map[string]float64
}

func (f fingerprint) vector() []float64 {
	return []float64{f.SalesWeek, f.BoughtWeek, f.WholesalePct, f.HotPct, f.Stock, f.EventPct, f.ULPct}
}

func (f fingerprint) mixText() string {
	keys := make([]string, 0, len(f.Mix))
	for k := range f.Mix {
		keys = append(keys, k)
	}
	slices.SortFunc(keys, func(a, b string) int { return int(math.Round(f.Mix[b]*1000 - f.Mix[a]*1000)) })
	var parts []string
	for i, k := range keys {
		if i == 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %.0f", strings.ToLower(k), 100*f.Mix[k]))
	}
	return strings.Join(parts, ", ")
}

// product is resale the fingerprint leaves out: the engines' own goods and inputs.
func product(id string) bool {
	id = strings.ToLower(id)
	for _, p := range []string{"wine", "water", "ingot", "berr", "bottle", "yeast", "ore", "flux", "rat", "meat", "filter", "tablet"} {
		if strings.Contains(id, p) {
			return true
		}
	}
	return false
}

type traceItem struct {
	ID   string   `json:"id"`
	T    []string `json:"t"`
	Got  float64  `json:"got"`
	Paid float64  `json:"paid"`
	Hot  *float64 `json:"hot"`
}

type traceLine struct {
	K         string      `json:"k"`
	Day       int         `json:"day"`
	Faction   string      `json:"faction"`
	Wholesale bool        `json:"wholesale"`
	Items     []traceItem `json:"items"`
	Bought    []traceItem `json:"bought"`
	Units     float64     `json:"units"`
	Events    []string    `json:"events"`
}

func readFingerprint(dir string) (fingerprint, int) {
	f := fingerprint{Mix: map[string]float64{}}
	paths, _ := filepath.Glob(filepath.Join(dir, "trace", "s*.jsonl"))
	var sales, bought, whole, hot, eventSales, ul, stock float64
	days, stockDays := 0, 0
	for _, p := range paths {
		fh, err := os.Open(p)
		if err != nil {
			continue
		}
		up := map[int][]string{}
		var lines []traceLine
		last := 0
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 0, 1<<20), 16<<20)
		for sc.Scan() {
			var l traceLine
			if json.Unmarshal(sc.Bytes(), &l) != nil || l.Day > fingerprintDays {
				continue
			}
			last = max(last, l.Day)
			if l.K == "day" {
				for _, e := range l.Events {
					t, v, _ := strings.Cut(e, ":")
					if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
						up[l.Day] = append(up[l.Day], t)
					}
				}
				stock += l.Units
				stockDays++
				continue
			}
			lines = append(lines, l)
		}
		fh.Close()
		days += last
		for _, l := range lines {
			for _, it := range l.Items {
				if product(it.ID) || len(it.T) == 0 {
					continue
				}
				sales += it.Got
				f.Mix[it.T[0]] += it.Got
				if slices.ContainsFunc(it.T, func(t string) bool { return slices.Contains(up[l.Day], t) }) {
					eventSales += it.Got
				}
				if strings.Contains(strings.ToLower(l.Faction), "upper") {
					ul += it.Got
				}
			}
			for _, it := range l.Bought {
				if product(it.ID) {
					continue
				}
				bought += it.Paid
				if l.Wholesale {
					whole += it.Paid
				}
				if it.Hot != nil {
					hot += it.Paid
				}
			}
		}
	}
	if days == 0 {
		return f, 0
	}
	weeks := float64(days) / 7
	pct := func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return 100 * a / b
	}
	f.SalesWeek, f.BoughtWeek = sales/weeks, bought/weeks
	f.WholesalePct, f.HotPct, f.EventPct, f.ULPct = pct(whole, bought), pct(hot, bought), pct(eventSales, sales), pct(ul, sales)
	if stockDays > 0 {
		f.Stock = stock / float64(stockDays)
	}
	for k, v := range f.Mix {
		f.Mix[k] = v / max(sales, 1)
	}
	return f, len(paths)
}

// tradeDistance is how far f trades from ref: the mean relative difference of the numeric
// features plus half the L1 distance of the resale mixes.
func tradeDistance(f, ref fingerprint) float64 {
	a, b := f.vector(), ref.vector()
	var d float64
	for i := range a {
		d += math.Abs(a[i]-b[i]) / max(math.Abs(b[i]), 1)
	}
	d /= float64(len(a))
	var mix float64
	for _, k := range keysOf(f.Mix, ref.Mix) {
		mix += math.Abs(f.Mix[k] - ref.Mix[k])
	}
	return d + mix/2
}

func keysOf(ms ...map[string]float64) []string {
	var out []string
	for _, m := range ms {
		for k := range m {
			if !slices.Contains(out, k) {
				out = append(out, k)
			}
		}
	}
	return out
}

// incomeNovelty is the L1 distance (in percentage points) of shares to the nearest menu
// route's published split, and that route.
func incomeNovelty(shares [4]float64) (float64, string) {
	lab, _ := agency.LabStats()
	best, name := math.Inf(1), ""
	for _, k := range slices.Sorted(func(yield func(string) bool) {
		for k := range lab {
			if !yield(k) {
				return
			}
		}
	}) {
		parts := strings.Split(lab[k].Income, "/")
		if len(parts) != 4 {
			continue
		}
		var d float64
		for i, p := range parts {
			v, _ := strconv.ParseFloat(p, 64)
			d += math.Abs(v - shares[i])
		}
		if d < best {
			best, name = d, k
		}
	}
	return best, name
}

type armReport struct {
	Name         string             `json:"name"`
	Runs         int                `json:"runs"`
	Wins         int                `json:"wins"`
	MedianWinDay float64            `json:"median_win_day"`
	Endings      map[string]int     `json:"endings"`
	DeltaDay     float64            `json:"paired_delta_win_day"`
	DeltaSE      float64            `json:"paired_delta_ci95"`
	Paired       int                `json:"paired"`
	Income       [4]float64         `json:"income_board_counter_production_georgio"`
	IncomeNovel  float64            `json:"income_distance"`
	Nearest      string             `json:"income_nearest_route"`
	Fingerprint  fingerprint        `json:"fingerprint"`
	TradeNovel   float64            `json:"trade_distance_to_ref"`
	Mix          string             `json:"mix"`
	Extra        map[string]float64 `json:"-"`
}

func report(block string, arms []arm, out io.Writer) error {
	ref := loadRuns(filepath.Join(block, arms[0].name))
	refFP, _ := readFingerprint(filepath.Join(block, arms[0].name))
	var reps []armReport
	for _, a := range arms {
		runs := loadRuns(filepath.Join(block, a.name))
		r := armReport{Name: a.name, Runs: len(runs), Endings: map[string]int{}}
		var winDays, deltas []float64
		for seed, s := range runs {
			end := s.GameOver
			if end == "" {
				end = "alive"
			}
			r.Endings[end]++
			if !s.won() {
				continue
			}
			r.Wins++
			winDays = append(winDays, float64(s.DaysPlayed))
			if b, ok := ref[seed]; ok && b.won() {
				deltas = append(deltas, float64(s.DaysPlayed-b.DaysPlayed))
			}
		}
		r.MedianWinDay = median(winDays)
		r.DeltaDay, r.DeltaSE = meanCI(deltas)
		r.Paired = len(deltas)
		r.Income, _ = income(runs)
		r.IncomeNovel, r.Nearest = incomeNovelty(r.Income)
		r.Fingerprint, _ = readFingerprint(filepath.Join(block, a.name))
		r.TradeNovel = tradeDistance(r.Fingerprint, refFP)
		r.Mix = r.Fingerprint.mixText()
		reps = append(reps, r)
	}
	fmt.Fprintf(out, "%-22s %7s %6s %16s %-22s %-18s %6s %7s %7s %5s %5s %5s %s\n", "route", "wins", "median", "Δ win day", "income B/C/P/G %", "nearest (L1)", "trade", "sales/w", "bought", "whl%", "hot%", "evt%", "mix")
	for _, r := range reps {
		inc := fmt.Sprintf("%.0f/%.0f/%.0f/%.0f", r.Income[0], r.Income[1], r.Income[2], r.Income[3])
		delta := "—"
		if r.Name != arms[0].name && r.Paired > 1 {
			delta = fmt.Sprintf("%+.1f ± %.1f", r.DeltaDay, r.DeltaSE)
		}
		fp := r.Fingerprint
		fmt.Fprintf(out, "%-22s %3d/%-3d %6.0f %16s %-22s %-18s %6.2f %7.0f %7.0f %5.0f %5.1f %5.0f %s\n", r.Name, r.Wins, r.Runs, r.MedianWinDay, delta, inc,
			fmt.Sprintf("%s (%.0f)", r.Nearest, r.IncomeNovel), r.TradeNovel, fp.SalesWeek, fp.BoughtWeek, fp.WholesalePct, fp.HotPct, fp.EventPct, r.Mix)
	}
	b, _ := json.MarshalIndent(reps, "", "  ")
	return os.WriteFile(filepath.Join(block, "report.json"), append(b, '\n'), 0o644)
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Sorted(slices.Values(xs))
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// meanCI is the mean and its 95 % half-width (1.96 SE).
func meanCI(xs []float64) (float64, float64) {
	n := float64(len(xs))
	if n < 2 {
		return 0, 0
	}
	var m float64
	for _, x := range xs {
		m += x
	}
	m /= n
	var v float64
	for _, x := range xs {
		v += (x - m) * (x - m)
	}
	return m, 1.96 * math.Sqrt(v/(n-1)/n)
}
