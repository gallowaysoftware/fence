package main

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const bootstrapN = 2000

// CI is a point estimate with a 95 % percentile-bootstrap interval.
type CI struct {
	Est float64 `json:"est"`
	Lo  float64 `json:"lo"`
	Hi  float64 `json:"hi"`
	N   int     `json:"n"`
}

// Group is one row of a comparison: a player, or a player × persona, etc.
type Group struct {
	Key          string         `json:"key"`
	N            int            `json:"n"`
	Endings      map[string]int `json:"endings"`
	DaysMedian   CI             `json:"days_median"`
	SurvivalRate CI             `json:"survival_rate"` // alive or bought out at the horizon
	BuyoutRate   CI             `json:"buyout_rate"`
	AliveRate    CI             `json:"alive_rate"` // still playing at the horizon
	BuyoutDayMed *CI            `json:"buyout_day_median,omitempty"`
	NW30         CI             `json:"nw30_mean"`
	NW60         CI             `json:"nw60_mean"`
	NW90         CI             `json:"nw90_mean"`
	PeakNW       CI             `json:"peak_nw_mean"`
	MaxAugMedian float64        `json:"max_aug_median"`
	// AliveNWToBuyout is final net worth over the buyout price, for runs alive at the horizon.
	AliveNWToBuyout *CI     `json:"alive_nw_to_buyout_median,omitempty"`
	LLMCalls        float64 `json:"llm_calls_per_game"`
	Tokens          float64 `json:"tokens_per_game"`
	LatencyP50MS    float64 `json:"latency_p50_ms"`
	Fallbacks       float64 `json:"fallbacks_per_game"`
	PlanCoverage    float64 `json:"plan_coverage"` // plans written / days played
	WallMedianS     float64 `json:"wall_median_s"`
}

// Paired is a player against the baseline on the same seeds, starts and personas.
type Paired struct {
	Player string `json:"player"`
	Pairs  int    `json:"pairs"`
	Days   CI     `json:"days_diff_mean"`
	NW60   CI     `json:"nw60_diff_mean"`
}

type Summary struct {
	League   string   `json:"league"`
	Spec     *Spec    `json:"spec"`
	Matches  int      `json:"matches_total"`
	Done     int      `json:"matches_done"`
	Players  []Group  `json:"players"`
	Personas []Group  `json:"player_persona,omitempty"`
	Starts   []Group  `json:"player_start,omitempty"`
	Routes   []Group  `json:"player_route,omitempty"`
	PerkSets []Group  `json:"player_perks,omitempty"`
	Diffs    []Group  `json:"player_difficulty,omitempty"`
	HeldOut  []Group  `json:"player_held_out,omitempty"`
	Baseline string   `json:"baseline"`
	Paired   []Paired `json:"paired"`
	Results  []Result `json:"results"`
	Endings  []string `json:"endings"`
}

// LoadResults reads every finished match of s under dir, in spec order.
func LoadResults(s *Spec, dir string) ([]Result, error) {
	var out []Result
	for _, m := range s.Matches() {
		var r Result
		err := readJSON(filepath.Join(dir, m.ID(), resultFile), &r)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.ID(), err)
		}
		out = append(out, r)
	}
	return out, nil
}

func Summarize(s *Spec, results []Result) *Summary {
	sum := &Summary{League: s.Name, Spec: s, Matches: len(s.Matches()), Done: len(results), Baseline: s.Baseline, Results: results}
	endings := map[string]bool{}
	for _, r := range results {
		endings[r.Ending] = true
	}
	for e := range endings {
		sum.Endings = append(sum.Endings, e)
	}
	slices.SortFunc(sum.Endings, func(a, b string) int {
		if d := endingRank(a) - endingRank(b); d != 0 {
			return d
		}
		return strings.Compare(a, b)
	})

	by := func(key func(Result) string) []Group {
		groups := map[string][]Result{}
		var order []string
		for _, r := range results {
			k := key(r)
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], r)
		}
		var out []Group
		for _, k := range order {
			out = append(out, group(s.Name, k, groups[k]))
		}
		return out
	}
	// Players in spec order, whatever finished first.
	for _, p := range s.Players {
		var rs []Result
		for _, r := range results {
			if r.Player == p.ID {
				rs = append(rs, r)
			}
		}
		if len(rs) > 0 {
			sum.Players = append(sum.Players, group(s.Name, p.ID, rs))
		}
	}
	if len(s.Personas) > 1 {
		sum.Personas = by(func(r Result) string { return r.Player + " · " + orNone(r.Persona) })
	}
	if distinct(results, func(r Result) string { return r.Perks }) > 1 {
		sum.PerkSets = by(func(r Result) string { return r.Player + " · " + r.Perks })
	}
	if s.PerkSample != nil && distinct(results, func(r Result) string { return strconv.FormatBool(r.PerksHeldOut) }) > 1 {
		sum.HeldOut = by(func(r Result) string {
			if r.PerksHeldOut {
				return r.Player + " · held-out perks"
			}
			return r.Player + " · training perks"
		})
	}
	if len(s.Difficulties) > 1 {
		sum.Diffs = by(func(r Result) string { return r.Player + " · " + r.Difficulty })
	}
	if len(s.Starts) > 1 {
		sum.Starts = by(func(r Result) string { return r.Player + " · " + r.Start })
	}
	if len(s.Routes) > 0 {
		sum.Routes = by(func(r Result) string { return r.Player + " · " + r.Route + " · " + orNone(r.Persona) })
	}
	base := map[string]Result{}
	for _, r := range results {
		if r.Player == s.Baseline {
			base[pairKey(r)] = r
		}
	}
	for _, p := range s.Players {
		if p.ID == s.Baseline {
			continue
		}
		var dd, dn []float64
		for _, r := range results {
			if b, ok := base[pairKey(r)]; ok && r.Player == p.ID {
				dd = append(dd, float64(r.DaysPlayed-b.DaysPlayed))
				dn = append(dn, float64(r.NWAt(60)-b.NWAt(60)))
			}
		}
		if len(dd) > 0 {
			sum.Paired = append(sum.Paired, Paired{Player: p.ID, Pairs: len(dd),
				Days: boot(dd, mean, seedFor(s.Name, p.ID, "pdays")), NW60: boot(dn, mean, seedFor(s.Name, p.ID, "pnw60"))})
		}
	}
	return sum
}

// endingRank orders the ending mix: good to bad, unknown reasons last.
func endingRank(e string) int {
	switch e {
	case "buyout", "buyout_mortgage":
		return 0
	case "alive":
		return 1
	case "rent":
		return 2
	case "arrest":
		return 3
	case "aug":
		return 4
	case "crash":
		return 9
	}
	return 5
}

func pairKey(r Result) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s|%s", r.Seed, r.Start, r.Route, r.Persona, r.PerksRequested, r.Difficulty)
}

func distinct(rs []Result, key func(Result) string) int {
	seen := map[string]bool{}
	for _, r := range rs {
		seen[key(r)] = true
	}
	return len(seen)
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}

func group(league, key string, rs []Result) Group {
	g := Group{Key: key, N: len(rs), Endings: map[string]int{}}
	var toBuy, alive []float64
	var days, surv, buy, bday, nw30, nw60, nw90, peak, aug, calls, toks, lat, fb, wall []float64
	planned, played := 0, 0
	for _, r := range rs {
		g.Endings[r.Ending]++
		days = append(days, float64(r.DaysPlayed))
		surv = append(surv, b2f(r.Ending == "alive" || r.Won()))
		buy = append(buy, b2f(r.Won()))
		alive = append(alive, b2f(r.Ending == "alive"))
		if r.Ending == "alive" && r.NWToBuyout() > 0 {
			toBuy = append(toBuy, r.NWToBuyout())
		}
		if r.Won() {
			bday = append(bday, float64(r.BuyoutDay))
		}
		nw30 = append(nw30, float64(r.NWAt(30)))
		nw60 = append(nw60, float64(r.NWAt(60)))
		nw90 = append(nw90, float64(r.NWAt(90)))
		peak = append(peak, float64(r.PeakNW))
		aug = append(aug, float64(r.MaxAug))
		calls = append(calls, float64(r.LLMCalls))
		toks = append(toks, float64(r.PromptTokens+r.CompletionTokens))
		if r.LLMCalls > 0 {
			lat = append(lat, float64(r.LatencyP50MS))
		}
		fb = append(fb, float64(r.Fallbacks))
		wall = append(wall, r.WallSeconds)
		planned += r.Plans
		played += r.DaysPlayed
	}
	sd := func(m string) uint64 { return seedFor(league, key, m) }
	g.DaysMedian = boot(days, median, sd("days"))
	g.SurvivalRate = boot(surv, mean, sd("surv"))
	g.BuyoutRate = boot(buy, mean, sd("buy"))
	g.AliveRate = boot(alive, mean, sd("alive"))
	if len(bday) > 0 {
		c := boot(bday, median, sd("bday"))
		g.BuyoutDayMed = &c
	}
	g.NW30 = boot(nw30, mean, sd("nw30"))
	g.NW60 = boot(nw60, mean, sd("nw60"))
	g.NW90 = boot(nw90, mean, sd("nw90"))
	g.PeakNW = boot(peak, mean, sd("peak"))
	g.MaxAugMedian = median(aug)
	if len(toBuy) > 0 {
		c := boot(toBuy, median, sd("tobuy"))
		g.AliveNWToBuyout = &c
	}
	g.LLMCalls = mean(calls)
	g.Tokens = mean(toks)
	if len(lat) > 0 {
		g.LatencyP50MS = median(lat)
	}
	g.Fallbacks = mean(fb)
	if played > 0 {
		g.PlanCoverage = float64(planned) / float64(played)
	}
	g.WallMedianS = median(wall)
	return g
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func mean(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := 0.0
	for _, v := range x {
		s += v
	}
	return s / float64(len(x))
}

func median(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	s := slices.Clone(x)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// seedFor makes every interval reproducible: the same results give the same report.
func seedFor(parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func boot(x []float64, stat func([]float64) float64, seed uint64) CI {
	c := CI{Est: stat(x), Lo: stat(x), Hi: stat(x), N: len(x)}
	if len(x) < 2 {
		return c
	}
	rng := rand.New(rand.NewPCG(seed, 0x9e3779b97f4a7c15))
	stats := make([]float64, bootstrapN)
	buf := make([]float64, len(x))
	for i := range stats {
		for j := range buf {
			buf[j] = x[rng.IntN(len(x))]
		}
		stats[i] = stat(buf)
	}
	slices.Sort(stats)
	c.Lo = stats[bootstrapN*25/1000]
	c.Hi = stats[bootstrapN*975/1000-1]
	return c
}

// Markdown renders the summary deterministically: no clock, sorted everything.
func (sum *Summary) Markdown() string {
	var b strings.Builder
	s := sum.Spec
	fmt.Fprintf(&b, "# League %s\n\n", sum.League)
	starts := "starts " + strings.Join(s.Starts, ", ")
	if len(s.Routes) > 0 {
		var rs []string
		for _, r := range s.Routes {
			rs = append(rs, r.Route+" · "+orNone(r.Persona))
		}
		starts = "routes " + strings.Join(rs, ", ")
	}
	fmt.Fprintf(&b, "%d of %d matches finished. Difficulty %s, horizon %d days, %s, seeds %s",
		sum.Done, sum.Matches, strings.Join(s.Difficulties, ", "), s.Days, starts, intsJoin(s.Seeds))
	if len(s.Personas) > 0 {
		fmt.Fprintf(&b, ", personas %s", strings.Join(s.Personas, ", "))
	}
	switch {
	case s.PerkSample != nil:
		ps := s.PerkSample
		fmt.Fprintf(&b, ", perks drawn per seed from %s (%.0f %% none, %.0f %% curated, held out: %s)", ps.Catalogue,
			100*ps.NoneShare, 100*ps.CuratedShare, strings.Join(append(slices.Clone(ps.HeldOut.Perks), ps.HeldOut.Combos...), ", "))
		if ps.IncludeHeldOut {
			b.WriteString(", held-out sets included")
		}
	case len(s.Perks) > 0:
		fmt.Fprintf(&b, ", perk sets %s", strings.Join(s.Perks, ", "))
	}
	if n := countUnapplied(sum.Results); n > 0 {
		fmt.Fprintf(&b, " (%d matches could not apply their perks: the gym has no -perks, so they started with none)", n)
	}
	fmt.Fprintf(&b, ". Executor (unless a player overrides it): `-human %s`", s.Executor.Human)
	if s.Executor.Knobs != "" {
		fmt.Fprintf(&b, " `-knobs %s`", s.Executor.Knobs)
	}
	b.WriteString(".\n\nIntervals are 95 % bootstrap CIs over matches. Net worth on day N is 0 for runs that ended before it.\n\n")

	b.WriteString("## Players\n\n")
	b.WriteString("| Player | Mode · executor | n | Buyout | Buyout day (median) | Days (median) | Alive at horizon | NW / buyout price, alive (median) | NW d30 | NW d60 | NW d90 | Max aug (median) |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	modes := map[string]string{}
	for _, p := range s.Players {
		modes[p.ID] = p.Mode + " · " + s.executor(p).Human
	}
	for _, g := range sum.Players {
		bday, toBuy := "–", "–"
		if g.BuyoutDayMed != nil {
			bday = num(*g.BuyoutDayMed)
		}
		if g.AliveNWToBuyout != nil {
			toBuy = ratio(*g.AliveNWToBuyout)
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s | %s | %s | %s | %s | %.0f |\n", g.Key, modes[g.Key], g.N,
			pct(g.BuyoutRate), bday, num(g.DaysMedian), pct(g.AliveRate), toBuy, num(g.NW30), num(g.NW60), num(g.NW90), g.MaxAugMedian)
	}

	b.WriteString("\n## Endings\n\n| Player |")
	for _, e := range sum.Endings {
		fmt.Fprintf(&b, " %s |", e)
	}
	b.WriteString("\n|---|" + strings.Repeat("---|", len(sum.Endings)) + "\n")
	for _, g := range sum.Players {
		fmt.Fprintf(&b, "| %s |", g.Key)
		for _, e := range sum.Endings {
			fmt.Fprintf(&b, " %d |", g.Endings[e])
		}
		b.WriteString("\n")
	}

	if len(sum.Paired) > 0 {
		fmt.Fprintf(&b, "\n## Against %s (same seeds)\n\n| Player | Pairs | Δ days (mean) | Δ NW d60 (mean) |\n|---|---|---|---|\n", sum.Baseline)
		for _, p := range sum.Paired {
			fmt.Fprintf(&b, "| %s | %d | %s | %s |\n", p.Player, p.Pairs, signed(p.Days), signed(p.NW60))
		}
	}
	for _, sec := range []struct {
		title string
		gs    []Group
	}{{"Player × persona", sum.Personas}, {"Player × difficulty", sum.Diffs}, {"Player × start", sum.Starts}, {"Player × route · persona", sum.Routes}, {"Player × perks", sum.PerkSets}, {"Held-out perks (generalisation)", sum.HeldOut}} {
		if len(sec.gs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n| Group | n | Days (median) | Survive | Buyout | NW d60 |\n|---|---|---|---|---|---|\n", sec.title)
		for _, g := range sec.gs {
			fmt.Fprintf(&b, "| %s | %d | %s | %s | %s | %s |\n", g.Key, g.N, num(g.DaysMedian), pct(g.SurvivalRate), pct(g.BuyoutRate), num(g.NW60))
		}
	}

	b.WriteString("\n## Cost\n\n| Player | LLM calls / game | Tokens / game | Latency p50 | Plans / day | Fallbacks / game | Wall / game (median) |\n|---|---|---|---|---|---|---|\n")
	for _, g := range sum.Players {
		fmt.Fprintf(&b, "| %s | %.0f | %.0f | %.1f s | %.2f | %.1f | %s |\n", g.Key, g.LLMCalls, g.Tokens, g.LatencyP50MS/1000, g.PlanCoverage, g.Fallbacks, dur(g.WallMedianS))
	}

	b.WriteString("\n## Matches\n\n| Match | Ending | Days | NW d30 | NW d60 | NW d90 | Cash | Max aug | Plans | LLM calls | p50 | Fallbacks | Wall |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	rs := slices.Clone(sum.Results)
	slices.SortFunc(rs, func(a, b Result) int { return strings.Compare(a.Match, b.Match) })
	for _, r := range rs {
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d | %d | %d | %d | %.1f s | %d | %s |\n", r.Match, r.Ending, r.DaysPlayed,
			r.NWAt(30), r.NWAt(60), r.NWAt(90), r.Cash, r.MaxAug, r.Plans, r.LLMCalls, float64(r.LatencyP50MS)/1000, r.Fallbacks, dur(r.WallSeconds))
	}
	return b.String()
}

func countUnapplied(rs []Result) int {
	n := 0
	for _, r := range rs {
		if r.PerksRequested != "" && r.Perks != r.PerksRequested {
			n++
		}
	}
	return n
}

func ratio(c CI) string {
	if c.N < 2 || c.Lo == c.Hi {
		return fmt.Sprintf("%.2f", c.Est)
	}
	return fmt.Sprintf("%.2f [%.2f, %.2f]", c.Est, c.Lo, c.Hi)
}

func num(c CI) string {
	if c.N < 2 || c.Lo == c.Hi {
		return fmt.Sprintf("%.0f", c.Est)
	}
	return fmt.Sprintf("%.0f [%.0f, %.0f]", c.Est, c.Lo, c.Hi)
}

func signed(c CI) string {
	if c.N < 2 {
		return fmt.Sprintf("%+.0f", c.Est)
	}
	return fmt.Sprintf("%+.0f [%+.0f, %+.0f]", c.Est, c.Lo, c.Hi)
}

func pct(c CI) string {
	if c.N < 2 || c.Lo == c.Hi {
		return fmt.Sprintf("%.0f%%", 100*c.Est)
	}
	return fmt.Sprintf("%.0f%% [%.0f, %.0f]", 100*c.Est, 100*c.Lo, 100*c.Hi)
}

func dur(sec float64) string {
	if sec >= 3600 {
		return fmt.Sprintf("%.1f h", sec/3600)
	}
	if sec >= 60 {
		return fmt.Sprintf("%.1f min", sec/60)
	}
	return fmt.Sprintf("%.0f s", sec)
}

func intsJoin(xs []int) string {
	s := make([]string, len(xs))
	for i, x := range xs {
		s[i] = fmt.Sprint(x)
	}
	return strings.Join(s, ", ")
}
