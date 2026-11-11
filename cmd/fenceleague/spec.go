package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	routes "github.com/gallowaysoftware/fence/internal/route"
)

// Spec is a league: committed, so the seeds stay held out and every result is comparable.
type Spec struct {
	Name   string   `json:"name"`
	Seeds  []int    `json:"seeds"`
	Starts []string `json:"starts"`
	// Difficulty is the default; Difficulties, when set, plays every match on each.
	Difficulty   string   `json:"difficulty"`
	Difficulties []string `json:"difficulties,omitempty"`
	// SeedsPerDifficulty plays only the first N seeds on a difficulty, to spend the nodes on
	// the others first.
	SeedsPerDifficulty map[string]int `json:"seeds_per_difficulty,omitempty"`
	Days               int            `json:"days"`
	// Personas are docs/personas ids; empty plays without one. Ignored by a bridge without
	// -persona support.
	Personas []string `json:"personas,omitempty"`
	// Perks are fixed starting-perk sets, each "none" or ids joined by "+"; every match
	// plays each set. PerkSample instead draws one set per seed and start. Neither: "none".
	Perks      []string    `json:"perks,omitempty"`
	PerkSample *PerkSample `json:"perk_sample,omitempty"`
	// Routes play each entry as one run spec (docs/research/ROUTES.md): the route gives the
	// start, the perks and the executor's directives, the persona the strategist's style.
	// Exclusive with starts, perks, perk_sample, personas and executor.directives.
	Routes   []RouteRun `json:"routes,omitempty"`
	Executor Executor   `json:"executor"`
	Players  []Player   `json:"players"`
	// PlanWait holds a day's first decision until its plan is written (plan mode).
	PlanWait string `json:"plan_wait,omitempty"`
	// DeadlineMS is the gym's per-decision deadline; above PlanWait in plan mode.
	DeadlineMS int `json:"deadline_ms,omitempty"`
	// Baseline is the player the others are compared with; the first player when empty.
	Baseline string `json:"baseline,omitempty"`
}

// RouteRun is a route and the persona that plays it.
type RouteRun struct {
	Route string `json:"route"`
	// Persona is a docs/personas id; empty takes the route's own.
	Persona string `json:"persona,omitempty"`
	r       routes.Route
}

// NoPerks is the perk set of a match that starts without any.
const NoPerks = "none"

// Executor is the gym's human role: everything the bridge is not asked about.
type Executor struct {
	Human string `json:"human"`
	Knobs string `json:"knobs,omitempty"`
	// Directives is a directives file (repo-relative or absolute) the gym plays over its
	// defaults; plan-mode bridges are shown it as the base their changes apply to.
	Directives string `json:"directives,omitempty"`
}

// Player is an (endpoint, model, policy mode). Endpoint names resolve in the local config.
type Player struct {
	ID       string `json:"id"`
	Endpoint string `json:"endpoint,omitempty"`
	// Model overrides the endpoint's default model.
	Model string `json:"model,omitempty"`
	Mode  string `json:"mode"`
	// Executor overrides the spec's for this player, e.g. a baseline on older knobs.
	Executor *Executor `json:"executor,omitempty"`
	// Bridge is a checkout whose ./cmd/bridge this player's matches run, so two builds of
	// the strategist can play the same seeds; empty runs the working tree's.
	Bridge string `json:"bridge,omitempty"`
	// BridgeFlags are added to this player's bridge command line (after the endpoint's).
	BridgeFlags []string `json:"bridge_flags,omitempty"`
	// RolloutFlags are added to a hybrid player's fence-rollout command line.
	RolloutFlags []string `json:"rollout_flags,omitempty"`
}

const (
	ModePlan      = "plan"      // LLM strategist + heuristic counter
	ModeLLM       = "llm"       // LLM counter and strategist
	ModeHeuristic = "heuristic" // bridge heuristic, no model
	ModeLocal     = "local"     // the gym's in-process heuristic with the executor's knobs
	// ModeHybrid plays fence-rollout's hybrid planner: the endpoint's model proposes candidate
	// directives at decision points and forked rollouts pick one; without an endpoint the
	// candidates are a blind lever sample (docs/research/HYBRID.md).
	ModeHybrid = "hybrid"
)

var idRE = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// executor is the human role p's matches play with.
func (s *Spec) executor(p Player) Executor {
	if p.Executor != nil {
		return *p.Executor
	}
	return s.Executor
}

func (p Player) usesLLM() bool {
	return p.Mode == ModePlan || p.Mode == ModeLLM || p.Mode == ModeHybrid && p.Endpoint != ""
}

func LoadSpec(path string) (*Spec, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Spec
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := s.normalize(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &s, nil
}

func (s *Spec) normalize() error {
	if !idRE.MatchString(s.Name) {
		return fmt.Errorf("name %q: want lower-case letters, digits, . _ -", s.Name)
	}
	if len(s.Seeds) == 0 || len(s.Players) == 0 {
		return errors.New("needs seeds and players")
	}
	if err := s.normalizeRoutes(); err != nil {
		return err
	}
	if len(s.Starts) == 0 && len(s.Routes) == 0 {
		s.Starts = []string{"none"}
	}
	if s.Difficulty == "" {
		s.Difficulty = "hard"
	}
	if len(s.Difficulties) == 0 {
		s.Difficulties = []string{s.Difficulty}
	}
	if s.Days <= 0 {
		s.Days = 200
	}
	if s.Executor.Human == "" {
		s.Executor.Human = "default"
	}
	if s.PlanWait == "" {
		s.PlanWait = "100s"
	}
	if _, err := time.ParseDuration(s.PlanWait); err != nil {
		return fmt.Errorf("plan_wait: %w", err)
	}
	if s.DeadlineMS <= 0 {
		// The gym's HTTP client gives up at 120 s.
		s.DeadlineMS = 110000
	}
	seen := map[string]bool{}
	for i := range s.Players {
		p := &s.Players[i]
		if p.Executor != nil && p.Executor.Human == "" {
			return fmt.Errorf("player %s: executor without human", p.ID)
		}
		if p.Executor != nil && p.Executor.Directives != "" && len(s.Routes) > 0 {
			return fmt.Errorf("player %s: executor directives with routes (the route gives them)", p.ID)
		}
		if !idRE.MatchString(p.ID) || seen[p.ID] {
			return fmt.Errorf("player id %q: empty, duplicate or not lower-case", p.ID)
		}
		seen[p.ID] = true
		switch p.Mode {
		case ModePlan, ModeLLM:
			if p.Endpoint == "" {
				return fmt.Errorf("player %s: mode %s needs an endpoint", p.ID, p.Mode)
			}
		case ModeHeuristic, ModeLocal, ModeHybrid:
		default:
			return fmt.Errorf("player %s: mode %q (want plan, llm, heuristic, local or hybrid)", p.ID, p.Mode)
		}
	}
	if s.Baseline == "" {
		s.Baseline = s.Players[0].ID
	} else if !seen[s.Baseline] {
		return fmt.Errorf("baseline %q is not a player", s.Baseline)
	}
	if err := s.normalizePerks(); err != nil {
		return err
	}
	for _, id := range append(append(slices.Clone(s.Starts), s.Personas...), s.Difficulties...) {
		if !idRE.MatchString(id) {
			return fmt.Errorf("start or persona %q is not an id", id)
		}
	}
	return nil
}

func (s *Spec) normalizeRoutes() error {
	if len(s.Routes) == 0 {
		return nil
	}
	if len(s.Starts) > 0 || len(s.Perks) > 0 || s.PerkSample != nil || len(s.Personas) > 0 || s.Executor.Directives != "" {
		return errors.New("routes are exclusive with starts, perks, perk_sample, personas and executor.directives")
	}
	seen := map[string]bool{}
	for i := range s.Routes {
		rr := &s.Routes[i]
		r, err := routes.Load("", rr.Route)
		if err != nil {
			return err
		}
		rr.r = r
		if rr.Persona == "" {
			rr.Persona = r.Persona
		}
		if rr.Persona != "" && !idRE.MatchString(rr.Persona) {
			return fmt.Errorf("route %s: persona %q is not an id", rr.Route, rr.Persona)
		}
		key := r.Name + "/" + rr.Persona
		if seen[key] {
			return fmt.Errorf("route %s with persona %q twice", rr.Route, rr.Persona)
		}
		seen[key] = true
	}
	return nil
}

func (s *Spec) normalizePerks() error {
	if s.PerkSample != nil && len(s.Perks) > 0 {
		return errors.New("perks and perk_sample are exclusive")
	}
	if s.PerkSample != nil {
		return s.PerkSample.load()
	}
	if len(s.Routes) > 0 {
		return nil
	}
	if len(s.Perks) == 0 {
		s.Perks = []string{NoPerks}
	}
	for i, set := range s.Perks {
		if set == NoPerks {
			continue
		}
		ids := strings.Split(set, "+")
		for _, p := range ids {
			if !idRE.MatchString(p) || p == NoPerks {
				return fmt.Errorf("perk set %q: %q is not an id", set, p)
			}
		}
		slices.Sort(ids)
		s.Perks[i] = strings.Join(ids, "+")
	}
	return nil
}

// perkSets are the sets a seed plays.
func (s *Spec) perkSets(seed int, start string) []string {
	ps := s.PerkSample
	if ps == nil {
		if len(s.Perks) == 0 {
			return []string{NoPerks}
		}
		return s.Perks
	}
	return []string{ps.Draw(seed, start)}
}

// Match is one game: a player on a seed, start, persona and perk set.
type Match struct {
	Player  Player
	Seed    int
	Start   string
	Persona string
	Perks   string
	// Difficulty is part of the id only in a league of several.
	Difficulty string
	multiDiff  bool
	// HeldOut marks a perk set kept out of training: a generalisation test.
	HeldOut bool
	// Route is the route's name, "" for a match without one; RoutePath its file.
	Route     string
	RoutePath string
}

func (m Match) ID() string {
	if m.Route != "" {
		id := m.Player.ID + "__r-" + m.Route + "__s" + strconv.Itoa(m.Seed)
		if m.Persona != "" {
			id += "__" + m.Persona
		}
		if m.multiDiff {
			id += "__" + m.Difficulty
		}
		return id
	}
	id := m.Player.ID + "__" + m.Start + "__s" + strconv.Itoa(m.Seed)
	if m.Persona != "" {
		id += "__" + m.Persona
	}
	if m.Perks != "" && m.Perks != NoPerks {
		id += "__p-" + m.Perks
	}
	if m.multiDiff {
		id += "__" + m.Difficulty
	}
	return id
}

// Matches is the league in a fixed order: difficulty-major in the listed order (list the one
// to finish first first), then seed-major, so an interrupted league has comparable coverage
// across players.
func (s *Spec) Matches() []Match {
	personas := s.Personas
	if len(personas) == 0 {
		personas = []string{""}
	}
	diffs := s.Difficulties
	if len(diffs) == 0 {
		diffs = []string{s.Difficulty}
	}
	var out []Match
	for _, diff := range diffs {
		seeds := s.Seeds
		if n, ok := s.SeedsPerDifficulty[diff]; ok && n < len(seeds) {
			seeds = seeds[:n]
		}
		for _, seed := range seeds {
			for _, rr := range s.Routes {
				for _, p := range s.Players {
					out = append(out, Match{Player: p, Seed: seed, Start: rr.r.Start, Persona: rr.Persona, Perks: sortedPerks(rr.r.Perks),
						Route: rr.r.Name, RoutePath: mustAbs(rr.r.Path), Difficulty: diff, multiDiff: len(diffs) > 1})
				}
			}
			for _, start := range s.Starts {
				for _, persona := range personas {
					for _, perks := range s.perkSets(seed, start) {
						for _, p := range s.Players {
							out = append(out, Match{Player: p, Seed: seed, Start: start, Persona: persona, Perks: perks,
								HeldOut:    s.PerkSample != nil && s.PerkSample.heldOut(perks),
								Difficulty: diff, multiDiff: len(diffs) > 1})
						}
					}
				}
			}
		}
	}
	return out
}

func sortedPerks(set string) string {
	ids := strings.Split(set, "+")
	slices.Sort(ids)
	return strings.Join(ids, "+")
}

// Local is the untracked machine config (var/league/local.json): endpoints and the gym.
type Local struct {
	// GymDir is a built checkout of the gym (fenceleague gym makes one).
	GymDir    string               `json:"gym_dir"`
	Endpoints map[string]*Endpoint `json:"endpoints"`
	// MaxGyms bounds local gym processes; 0 means NumCPU/4.
	MaxGyms int `json:"max_gyms,omitempty"`
	// MaxGymsWhileGaming bounds them while the owner plays.
	MaxGymsWhileGaming int `json:"max_gyms_while_gaming,omitempty"`
}

type Endpoint struct {
	BaseURL     string   `json:"base_url"`
	Model       string   `json:"model"`
	Thinking    string   `json:"thinking,omitempty"`
	Temperature string   `json:"temperature,omitempty"`
	NoSchema    bool     `json:"no_schema,omitempty"`
	ExtraFlags  []string `json:"extra_flags,omitempty"`
	// Concurrency is how many matches (and requests) it serves at once; 0 takes the
	// probe's recommendation, else 1.
	Concurrency int `json:"concurrency,omitempty"`
	// YieldsToGame keeps its matches off while the owner plays (the owner's game or live
	// session needs it): running ones abort and restart from scratch once the game closes.
	YieldsToGame bool `json:"yields_to_game,omitempty"`
	// UnloadURL is fetched when the owner starts playing, to free the GPU the game needs.
	UnloadURL string `json:"unload_url,omitempty"`
	// Disabled endpoints get no request at all, not even a ping; their players sit out.
	Disabled bool `json:"disabled,omitempty"`
	// ProbeTimeout skips the endpoint for a run when a one-token request takes longer.
	ProbeTimeout string `json:"probe_timeout,omitempty"`
}

func LoadLocal(path string) (*Local, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w (copy league/local.example.json to %s)", err, path)
	}
	var l Local
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for name, e := range l.Endpoints {
		if e.BaseURL == "" || e.Model == "" {
			return nil, fmt.Errorf("%s: endpoint %s needs base_url and model", path, name)
		}
		e.BaseURL = strings.TrimRight(e.BaseURL, "/")
	}
	return &l, nil
}

func (e *Endpoint) probeTimeout() (time.Duration, error) {
	if e.ProbeTimeout == "" {
		return time.Minute, nil
	}
	return time.ParseDuration(e.ProbeTimeout)
}

// bridgeFlags are the OpenAI options for the bridge, matching the endpoint's needs.
func (e *Endpoint) bridgeFlags() []string {
	var f []string
	if e.NoSchema {
		f = append(f, "-no-schema")
	}
	if e.Thinking != "" {
		f = append(f, "-thinking", e.Thinking)
	}
	if e.Temperature != "" {
		f = append(f, "-temperature", e.Temperature)
	}
	return append(f, e.ExtraFlags...)
}
