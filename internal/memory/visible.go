package memory

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
)

// barterContext is the part of the visible barter context (docs/PROTOCOL.md, "Barter
// context v1") memory derives facts from. It is decoded from DecideRequest.Context only.
type barterContext struct {
	Reputation map[string]struct {
		Value  float64 `json:"value"`
		Status string  `json:"status"`
	} `json:"reputation"`
	Client struct {
		Name    string   `json:"name"`
		Faction string   `json:"faction"`
		Intent  string   `json:"intent"`
		Budget  *float64 `json:"budget"`
	} `json:"client"`
	Counter struct {
		Theirs      []item `json:"theirs"`
		YoursStaged []item `json:"yours_staged"`
	} `json:"counter"`
}

type item struct {
	UID            int64     `json:"uid"`
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Price          float64   `json:"price"`
	EstimatedValue float64   `json:"estimated_value"`
	BoughtAt       *float64  `json:"bought_at"`
	Features       []feature `json:"features"`
	StolenHeat     *float64  `json:"stolen_heat"`
}

type feature struct {
	Text  string `json:"text"`
	Group string `json:"group"`
}

func parseBarter(raw json.RawMessage) (barterContext, bool) {
	var c barterContext
	if len(raw) == 0 || json.Unmarshal(raw, &c) != nil {
		return c, false
	}
	return c, true
}

// items returns every counter item, ours first, deduplicated by uid.
func (c *barterContext) items() []item {
	out := make([]item, 0, len(c.Counter.YoursStaged)+len(c.Counter.Theirs))
	seen := map[int64]bool{}
	for _, it := range slices.Concat(c.Counter.YoursStaged, c.Counter.Theirs) {
		if seen[it.UID] {
			continue
		}
		seen[it.UID] = true
		out = append(out, it)
	}
	return out
}

// repBand buckets the client's faction rep so bargain odds pool over similar standings.
// Factions without a rep bar (tourist, church, middle) are "none".
func (c *barterContext) repBand() string {
	r, ok := c.Reputation[c.Client.Faction]
	if !ok {
		return "none"
	}
	return repBand(r.Value)
}

func repBand(v float64) string {
	switch {
	case v <= -50:
		return "<=-50"
	case v <= -10:
		return "-50..-10"
	case v < 10:
		return "-10..10"
	case v < 50:
		return "10..50"
	case v < 100:
		return "50..100"
	}
	return ">=100"
}

// condition is the item's visible condition features, sorted, as one key.
func (it item) condition() string {
	var cs []string
	for _, f := range it.Features {
		if f.Group == "condition" && f.Text != "" {
			cs = append(cs, f.Text)
		}
	}
	if len(cs) == 0 {
		return ""
	}
	slices.Sort(cs)
	return strings.Join(slices.Compact(cs), ",")
}

func round(f float64) int64 { return int64(math.Round(f)) }

// parseMove splits a bargain option id ("add_markup_25") into move and percent.
func parseMove(option string) (move string, pct int, ok bool) {
	switch option {
	case "blackmail", "threaten":
		return option, 0, true
	}
	i := strings.LastIndexByte(option, '_')
	if i < 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(option[i+1:])
	if err != nil {
		return "", 0, false
	}
	switch move = option[:i]; move {
	case "offer_discount", "add_markup", "offer_markup", "demand_discount":
		return move, n, true
	}
	return "", 0, false
}

type outcomeResult struct {
	Kind      string             `json:"kind"`
	Option    string             `json:"option"`
	Accepted  bool               `json:"accepted"`
	WalkedOut bool               `json:"walked_out"`
	RepDelta  map[string]float64 `json:"rep_delta"`
	Side      string             `json:"side"`
	Items     []int64            `json:"items"`
	CashDelta float64            `json:"cash_delta"`
	Cause     string             `json:"cause"`
}
