// Package protocol holds the Go side of the mod↔bridge contract in docs/PROTOCOL.md.
// The JSON Schemas in protocol/schema are the source of truth; these types must
// round-trip every fixture in protocol/fixtures.
package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Version is the protocol integer this bridge speaks.
const Version = 1

type Mode string

const (
	ModeObserve Mode = "observe"
	ModeAdvise  Mode = "advise"
	ModeAct     Mode = "act"
)

func ParseMode(s string) (Mode, error) {
	switch m := Mode(s); m {
	case ModeObserve, ModeAdvise, ModeAct:
		return m, nil
	}
	return "", fmt.Errorf("unknown mode %q (want observe, advise or act)", s)
}

type Source string

const (
	SourceLLM       Source = "llm"
	SourceHeuristic Source = "heuristic"
	SourceFallback  Source = "fallback"
)

// OptionDefer is always a legal choice and hands the decision to the human.
const OptionDefer = "defer"

type Health struct {
	OK       bool `json:"ok"`
	Protocol int  `json:"protocol"`
}

type HelloRequest struct {
	Protocol   int    `json:"protocol"`
	ModVersion string `json:"mod_version"`
	Game       Game   `json:"game"`
	Compat     Compat `json:"compat"`
}

type Game struct {
	Version        string `json:"version"`
	Unity          string `json:"unity"`
	AssemblySHA256 string `json:"assembly_sha256"`
	AssemblyMVID   string `json:"assembly_mvid,omitempty"`
}

type Compat struct {
	OK               bool     `json:"ok"`
	Missing          []string `json:"missing"`
	DisabledFeatures []string `json:"disabled_features"`
}

type HelloResponse struct {
	SessionID string `json:"session_id"`
	Protocol  int    `json:"protocol"`
	Mode      Mode   `json:"mode"`
	// AppraisalGuides are the appraisal guides' value tables when the bridge has them locally
	// (local/appraisal_guides.json, never committed); the mod has no file access.
	AppraisalGuides json.RawMessage `json:"appraisal_guides,omitempty"`
}

type Event struct {
	SessionID string          `json:"session_id"`
	Seq       int64           `json:"seq"`
	Kind      string          `json:"kind"`
	GameDay   int             `json:"game_day"`
	Data      json.RawMessage `json:"data"`
	// Hidden is what the player cannot see, as for DecideRequest.Hidden. It is logged and
	// must never reach a recorder or prompt: use RecorderView.
	Hidden json.RawMessage `json:"hidden,omitempty"`
}

// RecorderView is the event without Hidden.
func (e Event) RecorderView() Event {
	e.Hidden = nil
	return e
}

type DecideRequest struct {
	SessionID  string `json:"session_id"`
	DecisionID string `json:"decision_id"`
	Kind       string `json:"kind"`
	GameDay    int    `json:"game_day"`
	DeadlineMS int64  `json:"deadline_ms"`
	// StateSignature is opaque to the bridge; it is only logged.
	StateSignature string          `json:"state_signature,omitempty"`
	Context        json.RawMessage `json:"context"`
	Options        []Option        `json:"options"`
	// Hidden is what the player cannot see. It is logged and must never reach a
	// policy: use PolicyView.
	Hidden json.RawMessage `json:"hidden,omitempty"`
}

// PolicyView is the request a policy may see: everything except Hidden.
func (r *DecideRequest) PolicyView() *DecideRequest {
	v := *r
	v.Hidden = nil
	return &v
}

type Option struct {
	ID     string           `json:"id"`
	Label  string           `json:"label"`
	Params map[string]Param `json:"params,omitempty"`
}

const ParamInt = "int"

// Param is a declared parameter of an option, discriminated by Type. Fields that
// belong to other types are left zero; Min/Max are pointers so an int param
// missing a bound is detectable.
type Param struct {
	Type string `json:"type"`
	Min  *int64 `json:"min,omitempty"`
	Max  *int64 `json:"max,omitempty"`
}

// Supported reports whether this bridge knows how to produce a value for p.
func (p Param) Supported() bool {
	return p.Type == ParamInt && p.Min != nil && p.Max != nil && *p.Min <= *p.Max
}

// Choosable reports whether every param of o is one this bridge can fill. Options with
// param types from a newer mod are skipped rather than rejected.
func (o Option) Choosable() bool {
	for _, p := range o.Params {
		if !p.Supported() {
			return false
		}
	}
	return true
}

type DecideResponse struct {
	DecisionID string `json:"decision_id"`
	Choice     Choice `json:"choice"`
	Rationale  string `json:"rationale,omitempty"`
	Source     Source `json:"source"`
	// OptionNotes maps offered option ids to a display note for the overlay; never executed.
	OptionNotes map[string]string `json:"option_notes,omitempty"`
}

// MaxOptionNote is the longest option note, in bytes.
const MaxOptionNote = 120

// SummaryResponse is the day (meta-turn) view: GET /v1/summary.
type SummaryResponse struct {
	SessionID  string   `json:"session_id"`
	Mode       Mode     `json:"mode"`
	Policy     string   `json:"policy"`
	Model      string   `json:"model"`
	Day        int      `json:"day"`
	Today      DayStats `json:"today"`
	Lessons    string   `json:"lessons"`
	LessonsDay int      `json:"lessons_day"`
	// Plan is the strategist's plan for the day, at most MaxPlanBytes; PlanDay the day it
	// was written for. Both are absent when there is no plan.
	Plan    string `json:"plan,omitempty"`
	PlanDay int    `json:"plan_day,omitempty"`
	// Directives is the plan as machine-readable instructions for an executor; absent
	// when there is no plan.
	Directives *Directives `json:"directives,omitempty"`
}

// MaxPlanBytes bounds SummaryResponse.Plan.
const MaxPlanBytes = 2048

// Directives is the strategist's plan for an executor (docs/PROTOCOL.md, "Directives").
// Every field is optional and an executor uses its own default for one that is absent,
// so numbers are pointers: 0 (no floor, no hot stock) is an instruction, absent is not.
type Directives struct {
	Day         int                   `json:"day,omitempty"`
	CashFloor   *int64                `json:"cash_floor,omitempty"`
	Buy         *BuyDirectives        `json:"buy,omitempty"`
	Sell        *SellDirectives       `json:"sell,omitempty"`
	Bargain     string                `json:"bargain,omitempty"`
	Window      *WindowDirectives     `json:"window,omitempty"`
	Services    []string              `json:"services,omitempty"`
	UpgradesBuy []string              `json:"upgrades_buy,omitempty"`
	Permit      string                `json:"permit,omitempty"`
	Production  []ProductionDirective `json:"production,omitempty"`
	Augs        string                `json:"augs,omitempty"`
	Scavenge    *bool                 `json:"scavenge,omitempty"`
	// Engine is the production engine the gym's executor runs: wine, water and furnace joined
	// by "+", or none.
	Engine string `json:"engine,omitempty"`
	// Mortgage levers of the gym's executor (sim/gym/PlayerRole.cs): when to sign the loan
	// shark's offer and how to play while paying it.
	Mortgage            string   `json:"mortgage,omitempty"`
	MortgageMargin      *float64 `json:"mortgage_margin,omitempty"`
	MortgageFloat       *int64   `json:"mortgage_float,omitempty"`
	MortgageWindow      []int64  `json:"mortgage_window,omitempty"`
	MortgageSaveFromDay *int64   `json:"mortgage_save_from_day,omitempty"`
	MortgageMaxSpend    *int64   `json:"mortgage_max_spend,omitempty"`
	// BuyoutByDay: answer the loan shark "later" while the cash buyout projects by this day.
	BuyoutByDay *int64 `json:"buyout_by_day,omitempty"`
	// Appraisal is the gym's appraisal board lever: join, or off.
	Appraisal string `json:"appraisal,omitempty"`
}

type BuyDirectives struct {
	WholesaleMaxPct *int64   `json:"wholesale_max_pct,omitempty"`
	SingleMaxPct    *int64   `json:"single_max_pct,omitempty"`
	MaxSpend        *int64   `json:"max_spend,omitempty"`
	PreferTypes     []string `json:"prefer_types,omitempty"`
	AvoidTypes      []string `json:"avoid_types,omitempty"`
	HotStockCap     *int64   `json:"hot_stock_cap,omitempty"`
}

type SellDirectives struct {
	MinPct         *int64  `json:"min_pct,omitempty"`
	SellDownToCash *int64  `json:"sell_down_to_cash,omitempty"`
	HoldUIDs       []int64 `json:"hold_uids,omitempty"`
}

type WindowDirectives struct {
	ShowUIDs    []int64 `json:"show_uids,omitempty"`
	AvoidStolen *bool   `json:"avoid_stolen,omitempty"`
}

type ProductionDirective struct {
	MachineUID int64   `json:"machine_uid"`
	Action     string  `json:"action"`
	Items      []int64 `json:"items,omitempty"`
	Note       string  `json:"note,omitempty"`
}

// The enumerated directive values, as in protocol/schema/directives.schema.json.
var (
	BargainModes = []string{"never", "ev_positive", "aggressive"}
	PermitModes  = []string{"buy", "refuse", "bribe"}
	AugModes     = []string{"shoot", "serve"}
	EngineModes  = []string{"wine", "none"}
	// EngineParts join with "+" into an engine; EngineModes are the contract's original values.
	EngineParts    = []string{"wine", "water", "furnace"}
	MortgageModes  = []string{"auto", "take", "later", "decline", "accept_if_rent"}
	AppraisalModes = []string{"join", "off"}
)

type DayStats struct {
	Decisions int64 `json:"decisions"`
	Sales     int64 `json:"sales"`
	Purchases int64 `json:"purchases"`
	CashDelta int64 `json:"cash_delta"`
	Walkouts  int64 `json:"walkouts"`
	// RepDelta is faction id → reputation change today; never null on the wire.
	RepDelta map[string]float64 `json:"rep_delta"`
}

// Choice params are raw so each value is checked against its declared param type.
type Choice struct {
	Option string                     `json:"option"`
	Params map[string]json.RawMessage `json:"params,omitempty"`
}

func Defer() Choice { return Choice{Option: OptionDefer} }

type Outcome struct {
	SessionID  string          `json:"session_id"`
	DecisionID string          `json:"decision_id"`
	GameDay    int             `json:"game_day"`
	Result     json.RawMessage `json:"result"`
}

type ErrorResponse struct {
	Error    string `json:"error"`
	Protocol int    `json:"protocol,omitempty"`
}

// Validate checks what the bridge relies on. Unknown fields are tolerated (an additive
// change from a newer mod must not break an older bridge); the schemas are stricter.
func (r *DecideRequest) Validate() error {
	var errs []error
	if r.SessionID == "" {
		errs = append(errs, errors.New("session_id is required"))
	}
	if r.DecisionID == "" {
		errs = append(errs, errors.New("decision_id is required"))
	}
	if r.Kind == "" {
		errs = append(errs, errors.New("kind is required"))
	}
	if r.DeadlineMS <= 0 {
		errs = append(errs, errors.New("deadline_ms must be positive"))
	}
	if len(r.Context) > 0 && !isObject(r.Context) {
		errs = append(errs, errors.New("context must be an object"))
	}
	if len(r.Hidden) > 0 && !isObject(r.Hidden) && string(r.Hidden) != "null" {
		errs = append(errs, errors.New("hidden must be an object"))
	}
	if len(r.Options) == 0 {
		errs = append(errs, errors.New("options must not be empty"))
	}
	seen := make(map[string]bool, len(r.Options))
	for i, o := range r.Options {
		switch {
		case o.ID == "":
			errs = append(errs, fmt.Errorf("options[%d].id is required", i))
		case o.ID == OptionDefer:
			errs = append(errs, fmt.Errorf("options[%d].id %q is reserved", i, OptionDefer))
		case seen[o.ID]:
			errs = append(errs, fmt.Errorf("options[%d].id %q is duplicated", i, o.ID))
		}
		seen[o.ID] = true
		for name, p := range o.Params {
			if p.Type == ParamInt && p.Min != nil && p.Max != nil && *p.Min > *p.Max {
				errs = append(errs, fmt.Errorf("options[%d].params.%s: min %d > max %d", i, name, *p.Min, *p.Max))
			}
		}
	}
	return errors.Join(errs...)
}

func (o *Outcome) Validate() error {
	var errs []error
	if o.SessionID == "" {
		errs = append(errs, errors.New("session_id is required"))
	}
	if o.DecisionID == "" {
		errs = append(errs, errors.New("decision_id is required"))
	}
	return errors.Join(errs...)
}

func (e *Event) Validate() error {
	var errs []error
	if e.SessionID == "" {
		errs = append(errs, errors.New("session_id is required"))
	}
	if e.Kind == "" {
		errs = append(errs, errors.New("kind is required"))
	}
	if e.Seq < 0 {
		errs = append(errs, errors.New("seq must not be negative"))
	}
	if len(e.Hidden) > 0 && !isObject(e.Hidden) && string(e.Hidden) != "null" {
		errs = append(errs, errors.New("hidden must be an object"))
	}
	return errors.Join(errs...)
}

func isObject(raw json.RawMessage) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}
