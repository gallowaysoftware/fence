// Package show turns a game into stream-worthy moments for the spectator panel and the
// narrator (docs/SHOW.md). Producers read only player-visible fields: a decision log's
// `hidden` objects are never decoded here.
package show

import (
	"encoding/json"
	"strings"
)

// SchemaVersion is Event.V. Bump it on a change a consumer must know about.
const SchemaVersion = 1

type Kind string

const (
	KindRunStart   Kind = "run_start"
	KindPlan       Kind = "plan"
	KindCandidates Kind = "candidates"
	KindThought    Kind = "thought"
	KindTrade      Kind = "trade"
	KindPurchase   Kind = "purchase"
	KindMoney      Kind = "money"
	KindHeat       Kind = "heat"
	KindAug        Kind = "aug"
	KindDay        Kind = "day"
	KindRunEnd     Kind = "run_end"
	KindSay        Kind = "say"
	// KindAutopilot is the live autopilot stopping (faulted, or handing a customer to the human) or the
	// human pausing and resuming it (docs/STREAM.md).
	KindAutopilot Kind = "autopilot"
)

// Levels say how loud a moment is: the ticker shows everything, the panel highlights
// notable ones, and the narrator only ever speaks about a moment.
const (
	LevelTicker  = 0
	LevelNotable = 1
	LevelMoment  = 2
)

// Event is one line of the show feed (JSON lines on disk, one SSE message on the wire).
// Text is always a complete, player-visible caption; the typed payloads are for layout.
type Event struct {
	V   int   `json:"v"`
	Seq int64 `json:"seq,omitempty"`
	// At is when the hub published it (unix ms); TS is when it happened in the source.
	At    int64  `json:"at,omitempty"`
	TS    string `json:"ts,omitempty"`
	Run   string `json:"run,omitempty"`
	Day   int    `json:"day"`
	Kind  Kind   `json:"kind"`
	Level int    `json:"level"`
	Text  string `json:"text"`

	Player     *Player     `json:"player,omitempty"`
	Plan       *Plan       `json:"plan,omitempty"`
	Candidates *Candidates `json:"candidates,omitempty"`
	Trade      *Trade      `json:"trade,omitempty"`
	Clock      *Clock      `json:"clock,omitempty"`
	Heat       *Heat       `json:"heat,omitempty"`
	Summary    *DaySummary `json:"summary,omitempty"`
	End        *End        `json:"end,omitempty"`
	Say        *Say        `json:"say,omitempty"`
	// Amount is the signed cash change of a money or purchase event.
	Amount int64 `json:"amount,omitempty"`
	// HiddenUsed names the hidden values an autopilot rule read (never the values): the panel marks the
	// entry as one where the hands knew more than a player could.
	HiddenUsed []string        `json:"hidden_used,omitempty"`
	Autopilot  *AutopilotState `json:"autopilot,omitempty"`
	// Beat names a stream beat the narrator plays up (BeatPromotion, …); "" for the rest.
	Beat string `json:"beat,omitempty"`
}

// The stream's big beats: moments the narrator is told what kind of moment it is.
const (
	BeatAppraisalJoin   = "appraisal_join"
	BeatPromotion       = "appraisal_promotion"
	BeatMortgageSigned  = "mortgage_signed"
	BeatMortgagePayment = "mortgage_payment"
	BeatBuyout          = "buyout"
	BeatAug             = "aug_confronted"
	BeatHandover        = "handover"
	BeatTakeBack        = "autopilot_back"
	// The small beats (smallbeats.go): speakable between the big ones.
	BeatCounter         = "counter"
	BeatNight           = "night"
	BeatMorning         = "morning_plan"
	BeatMilestone       = "milestone"
	BeatAppraisalStreak = "appraisal_streak"
)

// AutopilotState is the autopilot's state after a stop, pause or resume. NeedsHuman is a stop the
// autopilot cannot recover from by itself.
type AutopilotState struct {
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	NeedsHuman bool   `json:"needs_human"`
}

// Player is the name card. Persona is a docs/personas id ("" for none).
type Player struct {
	ID          string `json:"id"`
	Model       string `json:"model,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	Persona     string `json:"persona,omitempty"`
	PersonaName string `json:"persona_name,omitempty"`
	// PersonaTagline is the persona's path in a few words, for the name card.
	PersonaTagline string `json:"persona_tagline,omitempty"`
	// Route is the run's docs/research/ROUTES.md route; RouteBrief what the shop does on it.
	Route      string   `json:"route,omitempty"`
	RouteBrief string   `json:"route_brief,omitempty"`
	Difficulty string   `json:"difficulty,omitempty"`
	Start      string   `json:"start,omitempty"`
	Perks      []string `json:"perks,omitempty"`
	Seed       int64    `json:"seed,omitempty"`
	Planner    string   `json:"planner,omitempty"`
}

// Plan is the morning plan or a change to it. Source is "llm" when the strategist's prose
// is known, "directives" when only its logged directives are (league logs before docs/SHOW.md's
// recording hook), "executor" when the model changed nothing.
type Plan struct {
	Day      int       `json:"day"`
	Source   string    `json:"source"`
	Change   bool      `json:"change,omitempty"`
	Headline []string  `json:"headline,omitempty"`
	Sections []Section `json:"sections,omitempty"`
	// Directives are the plan's instructions to the executor in words; Changes what moved
	// since the previous plan.
	Directives []string `json:"directives,omitempty"`
	Changes    []string `json:"changes,omitempty"`
	// Why holds the model's reasons ("bargain: Lower Levels near -20") and the bridge's
	// corrections ("overruled: …").
	Why []string `json:"why,omitempty"`
	// Goals are what the shop is working toward whatever today's orders: the last prose
	// plan's goal, the mortgage, rent and buyout against cash, the standing directives.
	Goals []string `json:"goals,omitempty"`
}

type Section struct {
	Title string   `json:"title"`
	Lines []string `json:"lines"`
}

// Candidates is the hybrid planner's slate ("LLM proposes, gym decides"): each candidate
// plan with its simulated outcome. Producers POST it to /v1/ingest.
type Candidates struct {
	Day     int         `json:"day"`
	Metric  string      `json:"metric,omitempty"`
	Options []Candidate `json:"options"`
}

type Candidate struct {
	Label      string   `json:"label"`
	Source     string   `json:"source,omitempty"`
	Directives []string `json:"directives,omitempty"`
	Score      float64  `json:"score"`
	StdErr     float64  `json:"stderr,omitempty"`
	Rollouts   int      `json:"rollouts,omitempty"`
	Chosen     bool     `json:"chosen,omitempty"`
	Note       string   `json:"note,omitempty"`
	// Why is the proposer's reason, whole; Delta the candidate's directives over the base policy,
	// from which a hybrid replay shows the chosen one as today's plan.
	Why   string          `json:"why,omitempty"`
	Delta json.RawMessage `json:"delta,omitempty"`
}

type Trade struct {
	Side      string   `json:"side"`
	Items     []string `json:"items"`
	Count     int      `json:"count"`
	Cash      int64    `json:"cash"`
	Estimate  int64    `json:"estimate,omitempty"`
	Cost      int64    `json:"cost,omitempty"`
	Profit    int64    `json:"profit,omitempty"`
	HasProfit bool     `json:"has_profit,omitempty"`
	Customer  string   `json:"customer,omitempty"`
	Faction   string   `json:"faction,omitempty"`
	Wholesale bool     `json:"wholesale,omitempty"`
	// Fakes counts items whose name marks them as counterfeit.
	Fakes int `json:"fakes,omitempty"`
}

// Clock is the cash against what is owed.
type Clock struct {
	Cash        int64 `json:"cash"`
	NetWorth    int64 `json:"net_worth,omitempty"`
	RentDue     int64 `json:"rent_due,omitempty"`
	RentDays    int   `json:"rent_days"`
	Arrears     int64 `json:"arrears,omitempty"`
	Grace       bool  `json:"grace,omitempty"`
	Owned       bool  `json:"owned,omitempty"`
	BuyoutPrice int64 `json:"buyout_price,omitempty"`
	// MortgageWeekly is set while a mortgage runs; OfferDown while one is on the table.
	MortgageWeekly    int64 `json:"mortgage_weekly,omitempty"`
	MortgageWeeksLeft int   `json:"mortgage_weeks_left,omitempty"`
	OfferDown         int64 `json:"offer_down,omitempty"`
	OfferWeekly       int64 `json:"offer_weekly,omitempty"`
	Weekday           int   `json:"weekday,omitempty"`
}

// Heat is the security record as the player's dossier shows it, plus the stolen stock
// on hand. Visible is false before the shop has a record document.
type Heat struct {
	Visible   bool             `json:"visible"`
	Evidence  int64            `json:"evidence,omitempty"`
	Level     string           `json:"level,omitempty"`
	Operation string           `json:"operation,omitempty"`
	Officer   string           `json:"officer,omitempty"`
	Sentence  string           `json:"sentence,omitempty"`
	Crimes    map[string]int64 `json:"crimes,omitempty"`
	HotItems  int              `json:"hot_items,omitempty"`
}

type DaySummary struct {
	NetWorthDelta int64          `json:"net_worth_delta"`
	CashDelta     int64          `json:"cash_delta"`
	Sold          int64          `json:"sold"`
	SoldItems     int            `json:"sold_items"`
	Bought        int64          `json:"bought"`
	BoughtItems   int            `json:"bought_items"`
	Customers     int            `json:"customers"`
	Rep           map[string]int `json:"rep,omitempty"`
}

type End struct {
	Ending   string `json:"ending"`
	Day      int    `json:"day"`
	Cash     int64  `json:"cash"`
	NetWorth int64  `json:"net_worth,omitempty"`
	Won      bool   `json:"won"`
}

// Say is a narrator line. Audio is a URL path on the show server, "" for captions only.
type Say struct {
	Line   string `json:"line"`
	Audio  string `json:"audio,omitempty"`
	Voice  string `json:"voice,omitempty"`
	For    int64  `json:"for,omitempty"`
	Source string `json:"source"`
	// Check is the fact check: "pass", "retry" (the second line passed) or "fallback"
	// (neither did; the caption was spoken). Faults are the first line's.
	Check  string   `json:"check,omitempty"`
	Faults []string `json:"faults,omitempty"`
	// MS is how long the LLM took over the line, both tries.
	MS int64 `json:"ms,omitempty"`
	// Mood is the narrator's read of the shop's position when it spoke (MoodOf).
	Mood string `json:"mood,omitempty"`
}

// BoardRow is one run on the scoreboard.
type BoardRow struct {
	Run        string `json:"run"`
	Player     string `json:"player"`
	Model      string `json:"model,omitempty"`
	Persona    string `json:"persona,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	Ending     string `json:"ending,omitempty"`
	Days       int    `json:"days"`
	NetWorth   int64  `json:"net_worth,omitempty"`
	Won        bool   `json:"won,omitempty"`
	Live       bool   `json:"live,omitempty"`
}

// Won reports whether an ending is a victory, as cmd/fenceleague counts it: a buyout.
func Won(ending string) bool { return strings.HasPrefix(ending, "buyout") }
