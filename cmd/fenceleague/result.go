package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Result is one finished match. Every field comes from the match directory, so a result
// can be rebuilt from the files alone (fenceleague report -rebuild).
type Result struct {
	Match    string `json:"match"`
	Player   string `json:"player"`
	Mode     string `json:"mode"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
	Seed     int    `json:"seed"`
	Start    string `json:"start"`
	Persona  string `json:"persona,omitempty"`
	Route    string `json:"route,omitempty"`
	Format   string `json:"format,omitempty"`
	// Perks is the set the match started with; PerksRequested differs when the gym could
	// not apply the spec's set.
	Perks          string `json:"perks"`
	PerksRequested string `json:"perks_requested"`
	PerksHeldOut   bool   `json:"perks_held_out,omitempty"`
	Difficulty     string `json:"difficulty"`
	Horizon        int    `json:"horizon"`

	// Ending is the game-over reason (buyout, rent, arrest, aug, …), "alive" at the horizon,
	// or "crash" when the gym failed mid-game.
	Ending     string `json:"ending"`
	DaysPlayed int    `json:"days_played"`
	LastDay    int    `json:"last_day"`
	BuyoutDay  int    `json:"buyout_day,omitempty"`
	Crash      string `json:"crash,omitempty"`

	Cash      int64   `json:"cash"`
	NetWorth  []int64 `json:"net_worth_by_day"`
	CashByDay []int64 `json:"cash_by_day"`
	PeakNW    int64   `json:"peak_net_worth"`
	// BuyoutPrice is the last price the snapshots showed for buying the shop.
	BuyoutPrice int64              `json:"buyout_price,omitempty"`
	Rep         map[string]float64 `json:"rep,omitempty"`
	Permit      *bool              `json:"permit,omitempty"`
	AugByDay    []int              `json:"aug_presence_by_day,omitempty"`
	MaxAug      int                `json:"max_aug_presence"`
	Evidence    []int              `json:"evidence_by_day,omitempty"`

	Customers        int            `json:"customers"`
	Decisions        int            `json:"decisions"`
	Deferred         int            `json:"deferred"`
	DecisionFailures int            `json:"decision_failures"`
	Sources          map[string]int `json:"sources,omitempty"`
	Fallbacks        int            `json:"fallbacks"`

	LLMCalls         int     `json:"llm_calls"`
	LLMErrors        int     `json:"llm_errors"`
	LLMRetries       int     `json:"llm_retries"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	LatencyMeanMS    int64   `json:"latency_mean_ms"`
	LatencyP50MS     int64   `json:"latency_p50_ms"`
	LatencyP90MS     int64   `json:"latency_p90_ms"`
	QueueMS          int64   `json:"queue_ms"`
	LLMSeconds       float64 `json:"llm_seconds"`
	Plans            int     `json:"plans"`
	PlanFailures     int     `json:"plan_failures"`
	ScorecardFails   int     `json:"scorecard_failures"`
	Lessons          int     `json:"lessons"`
	ReflectFails     int     `json:"reflect_failures"`

	GymSeconds  float64 `json:"gym_seconds"`
	WallSeconds float64 `json:"wall_seconds"`
}

// Won is a buyout, outright or by paying off the mortgage.
func (r *Result) Won() bool { return strings.HasPrefix(r.Ending, "buyout") }

// NWToBuyout is the final net worth as a share of the buyout price; 0 when unknown.
func (r *Result) NWToBuyout() float64 {
	if r.BuyoutPrice <= 0 || len(r.NetWorth) == 0 {
		return 0
	}
	return float64(r.NetWorth[len(r.NetWorth)-1]) / float64(r.BuyoutPrice)
}

// NWAt is net worth at the end of day d (1-based); 0 when the run ended before it.
func (r *Result) NWAt(d int) int64 {
	if d < 1 || d > len(r.NetWorth) {
		return 0
	}
	return r.NetWorth[d-1]
}

type gymSummary struct {
	LastDay    int       `json:"last_day"`
	DaysPlayed int       `json:"days_played"`
	Cash       float64   `json:"cash"`
	GameOver   *string   `json:"game_over"`
	Crash      *string   `json:"crash"`
	CashByDay  []float64 `json:"cash_by_day"`
	NWByDay    []float64 `json:"net_worth_by_day"`
	Seconds    float64   `json:"seconds"`
	Stats      struct {
		Customers        int `json:"customers"`
		Decisions        int `json:"decisions"`
		Deferred         int `json:"deferred"`
		DecisionFailures int `json:"decision_failures"`
	} `json:"stats"`
	End *struct {
		Rep    map[string]float64 `json:"rep"`
		Permit *bool              `json:"permit"`
	} `json:"end"`
}

func BuildResult(m Match, s *Spec, dir string) (*Result, error) {
	var g gymSummary
	if err := readJSON(filepath.Join(dir, "gym.summary.json"), &g); err != nil {
		return nil, fmt.Errorf("gym summary: %w", err)
	}
	var meta MatchMeta
	readJSON(filepath.Join(dir, "match.json"), &meta)
	r := &Result{Match: m.ID(), Player: m.Player.ID, Mode: m.Player.Mode, Endpoint: m.Player.Endpoint, Model: meta.Model,
		Perks: meta.Perks, PerksRequested: meta.PerksAsked, PerksHeldOut: meta.HeldOut,
		Seed: m.Seed, Start: m.Start, Persona: m.Persona, Route: m.Route, Format: m.Format, Difficulty: m.Difficulty, Horizon: s.Days,
		LastDay: g.LastDay, DaysPlayed: g.DaysPlayed, Cash: int64(g.Cash), GymSeconds: g.Seconds,
		Customers: g.Stats.Customers, Decisions: g.Stats.Decisions, Deferred: g.Stats.Deferred, DecisionFailures: g.Stats.DecisionFailures}
	for _, v := range g.CashByDay {
		r.CashByDay = append(r.CashByDay, int64(v))
	}
	for _, v := range g.NWByDay {
		r.NetWorth = append(r.NetWorth, int64(v))
		r.PeakNW = max(r.PeakNW, int64(v))
	}
	switch {
	case g.Crash != nil && *g.Crash != "":
		r.Ending, r.Crash = "crash", clip(*g.Crash, 400)
	case g.GameOver != nil && *g.GameOver != "":
		r.Ending = *g.GameOver
	default:
		r.Ending = "alive"
	}
	if r.Won() {
		r.BuyoutDay = g.LastDay
	}
	if g.End != nil {
		r.Rep, r.Permit = g.End.Rep, g.End.Permit
	}
	if err := r.readTrajectory(filepath.Join(dir, "trajectory.jsonl")); err != nil {
		return nil, err
	}
	if err := r.readDecisions(filepath.Join(dir, "log")); err != nil {
		return nil, err
	}
	if err := r.readCalls(filepath.Join(dir, "llm.jsonl")); err != nil {
		return nil, err
	}
	r.readBridgeLog(filepath.Join(dir, "bridge.log"))
	return r, nil
}

// readTrajectory takes the buyout price from the day snapshots and the hidden per-day labels
// the gym records: aug presence (≥ 100 ends a hard run) and security evidence.
func (r *Result) readTrajectory(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	for sc.Scan() {
		var day struct {
			GameDay   int `json:"game_day"`
			Snapshots []struct {
				Data struct {
					Property *struct {
						BuyoutPrice *int64 `json:"buyout_price"`
					} `json:"property"`
				} `json:"data"`
			} `json:"snapshots"`
			Labels []struct {
				Source string `json:"source"`
				Hidden struct {
					Aug      *int `json:"aug_presence"`
					Security *struct {
						Evidence *int `json:"evidence"`
					} `json:"security"`
				} `json:"hidden"`
			} `json:"labels"`
		}
		if json.Unmarshal(sc.Bytes(), &day) != nil || day.GameDay < 1 {
			continue
		}
		for _, s := range day.Snapshots {
			if p := s.Data.Property; p != nil && p.BuyoutPrice != nil && *p.BuyoutPrice > 0 {
				r.BuyoutPrice = *p.BuyoutPrice
			}
		}
		aug, ev := -1, -1
		for _, l := range day.Labels {
			if l.Source != "day_snapshot" {
				continue
			}
			if l.Hidden.Aug != nil {
				aug = max(aug, *l.Hidden.Aug)
			}
			if l.Hidden.Security != nil && l.Hidden.Security.Evidence != nil {
				ev = max(ev, *l.Hidden.Security.Evidence)
			}
		}
		if aug >= 0 {
			r.AugByDay = setDay(r.AugByDay, day.GameDay, aug)
			r.MaxAug = max(r.MaxAug, aug)
		}
		if ev >= 0 {
			r.Evidence = setDay(r.Evidence, day.GameDay, ev)
		}
	}
	return sc.Err()
}

// setDay puts v at day (1-based), carrying the last value over any gap.
func setDay(s []int, day, v int) []int {
	for len(s) < day {
		last := 0
		if len(s) > 0 {
			last = s[len(s)-1]
		}
		s = append(s, last)
	}
	s[day-1] = v
	return s
}

func (r *Result) readDecisions(dir string) error {
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	slices.Sort(files)
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 64<<20)
		for sc.Scan() {
			line := sc.Bytes()
			if !strings.Contains(string(line[:min(len(line), 40)]), `"decision"`) {
				continue
			}
			var d struct {
				Type     string `json:"type"`
				Response struct {
					Source string `json:"source"`
				} `json:"response"`
			}
			if json.Unmarshal(line, &d) != nil || d.Type != "decision" {
				continue
			}
			if r.Sources == nil {
				r.Sources = map[string]int{}
			}
			r.Sources[d.Response.Source]++
			if d.Response.Source == "fallback" {
				r.Fallbacks++
			}
		}
		f.Close()
		if err := sc.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Result) readCalls(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	defer f.Close()
	var lat []int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c CallRecord
		if json.Unmarshal(sc.Bytes(), &c) != nil {
			continue
		}
		r.LLMCalls++
		if c.Status != 200 || c.Error != "" {
			r.LLMErrors++
		}
		if c.Retried {
			r.LLMRetries++
		}
		r.PromptTokens += c.PromptTokens
		r.CompletionTokens += c.CompletionTokens
		r.QueueMS += c.QueueMS
		lat = append(lat, c.LatencyMS)
	}
	if len(lat) > 0 {
		var sum int64
		for _, v := range lat {
			sum += v
		}
		r.LatencyMeanMS = sum / int64(len(lat))
		r.LLMSeconds = round1(float64(sum) / 1000)
		slices.Sort(lat)
		r.LatencyP50MS = lat[len(lat)/2]
		r.LatencyP90MS = lat[min(len(lat)-1, len(lat)*9/10)]
	}
	return sc.Err()
}

func (r *Result) readBridgeLog(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	s := string(b)
	r.Plans = strings.Count(s, `msg="day plan written"`)
	r.PlanFailures = strings.Count(s, `msg="day plan failed`)
	r.ScorecardFails = strings.Count(s, `msg="scorecard failed`)
	r.Lessons = strings.Count(s, `msg="lessons rewritten"`)
	r.ReflectFails = strings.Count(s, `msg="reflection failed`)
}

func round1(f float64) float64 { return math.Round(f*10) / 10 }
