package strategy

import (
	"encoding/json"
	"slices"
	"strings"
)

// The levers below are what Claude agents playing gym games through step mode learned: one
// kept a normal game alive to day 200 where the executor alone died on day 175
// (docs/research/CLAUDE-PLAY.md, "Claude play"); a second played two normal games to game
// over ("Claude play 2", reported by the coordinator, not yet written up). Checked against
// the gym's arms (docs/research/EXPERIMENTS.md).

// favorOrder is the order Wild favor buys upgrades in (Claude play, lesson 1; Water Trading
// second from the first won game, Claude play 5), without the
// supplier contacts and shareholdings: not worth a trading shop's working capital (Claude
// play 2). Hard leaves out the showcases and renovations: the showcase cost days there (gym
// v2 hard, n=40: grace only +3 days over the baseline list) and with a retail markup of 0
// traffic is not profit.
var favorOrder = map[bool][]string{
	false: {"LANDLORD_GRACE", "WINEMAKER", "WATER_TRADER", "SHOWCASE_I", "BUY_REFERAL", "SELL_REFERAL", "SHOWCASE_II", "RENOVATION1", "RENOVATION2", "RENOVATION3"},
	true:  {"LANDLORD_GRACE", "WINEMAKER", "BUY_REFERAL"},
}

const (
	// buyingMargin is the cash a shop keeps for buying stock on top of rent and a week of
	// services before it holds wine to age (Claude play, lesson 2).
	buyingMargin = 1000
	// permitMargin is the cash left after the permit fee and the rent due when the permit is
	// bought: bribing an affordable permit keeps logging unlicensed crime (Claude play 2).
	permitMargin = 500
	// wineIdleDays without a new finished wine turns the wine engine off: ~10 (Claude play,
	// lesson 6), ~5 (Claude play 2).
	wineIdleDays = 5
	// bargainLLFloor: below this Lower Levels rep, bargaining stops (Claude play 2).
	bargainLLFloor = -10
	// upgradeCashMargin is the cash, above the rent due, an upgrade paid partly in cash must
	// leave (Claude play 3).
	upgradeCashMargin = 1200
	// augServeLL: below this Lower Levels rep on normal, augs are served, not shot (Claude play 4).
	augServeLL = -20
	// supplierWeekday is day % 7 on supplier day, two days before rent.
	supplierWeekday = 5
)

func (s *snapshot) dueOrZero() int64 {
	d, _ := s.Rent.due()
	return d
}

func (s *snapshot) hard() bool {
	d := strings.ToLower(s.storeExtra["difficulty"])
	return d == "hard" || d == "challenge"
}

// favorUpgrades are the favor order's upgrades on offer, in order.
func (s *snapshot) favorUpgrades() []string {
	var out []string
	for _, id := range favorOrder[s.hard()] {
		if _, ok := s.offered(id); ok {
			out = append(out, id)
		}
	}
	return out
}

// liquid: cash covers the rent due, a week of the services in use and the buying margin,
// so finished wine may be held to age (Claude play, lesson 2).
func (s *snapshot) liquid() bool {
	if s.Cash == nil {
		return true
	}
	due, _ := s.Rent.due()
	var svc int64
	for _, v := range s.Services {
		if v.inUse() && v.Cost != nil {
			svc += *v.Cost
		}
	}
	return *s.Cash >= due+7*svc+buyingMargin
}

// wineGrades are the finished wine grades, worst first.
var wineGrades = []string{"bucket brew", "homebrew", "production", "reserve", "master vintage"}

// wineGrade is a finished wine's grade index in wineGrades; ok false for an empty bottle
// or anything else.
func wineGrade(it item) (int, bool) {
	if it.ID != "wine_bottle" || it.stolen() {
		return 0, false
	}
	for _, f := range it.Features {
		t := strings.ToLower(f.Text)
		for i := len(wineGrades) - 1; i >= 0; i-- {
			if strings.HasPrefix(t, wineGrades[i]) {
				return i, true
			}
		}
	}
	return 0, false
}

// wineHolds are the finished wines to age rather than sell: Homebrew and better while the
// shop is liquid, only Reserve and Master Vintage when it is not (lowest grades sell first).
func (s *snapshot) wineHolds() []int64 {
	least := 1
	if !s.liquid() {
		least = 3
	}
	var out []int64
	for _, it := range s.Inventory {
		if g, ok := wineGrade(it); ok && g >= least && it.UID != nil {
			out = append(out, *it.UID)
		}
	}
	return out
}

// drawsPower: something the shop owns runs on store power, so the power service pays.
func (s *snapshot) drawsPower() bool {
	for _, it := range s.Inventory {
		if mentionsAny(strings.ToLower(it.ID), []string{"recharger", "powerblock", "current_converter", "wine_rack"}) {
			return true
		}
	}
	for _, m := range s.production().machines {
		if m.PowerPerNight != nil && *m.PowerPerNight > 0 {
			return true
		}
	}
	return false
}

// engineIdle: wine has been made and none new has finished for wineIdleDays.
func (s *snapshot) engineIdle() bool {
	return s.wineIdle != nil && *s.wineIdle >= wineIdleDays
}

// WineHistory is when each finished wine was first seen, so the strategist can tell a
// stalled wine engine from a running one.
type WineHistory struct {
	seen    map[int64]bool
	lastNew int
}

// Note records a morning snapshot's finished wines.
func (h *WineHistory) Note(day int, raw json.RawMessage) {
	if s, err := parseSnapshot(raw); err == nil {
		h.note(day, s)
	}
}

func (h *WineHistory) note(day int, s *snapshot) {
	if h.seen == nil {
		h.seen = map[int64]bool{}
	}
	for _, it := range s.Inventory {
		if _, ok := wineGrade(it); ok && it.UID != nil && !h.seen[*it.UID] {
			h.seen[*it.UID] = true
			h.lastNew = max(h.lastNew, day)
		}
	}
}

// Idle is the days since the last new finished wine as of day; nil before the first.
func (h *WineHistory) Idle(day int) *int {
	if len(h.seen) == 0 {
		return nil
	}
	n := day - h.lastNew
	return &n
}

func (s *snapshot) favorText() string {
	ids := s.favorUpgrades()
	if len(ids) == 0 {
		return ""
	}
	var parts []string
	for _, id := range ids {
		if u, ok := s.offered(id); ok && u.Cost != nil {
			parts = append(parts, u.text())
			continue
		}
		parts = append(parts, id)
	}
	return strings.Join(parts, ", ")
}

func appendMissing(list []string, ids ...string) []string {
	for _, id := range ids {
		if !slices.ContainsFunc(list, func(x string) bool { return strings.EqualFold(x, id) }) {
			list = append(list, id)
		}
	}
	return list
}

// cashBuyoutProjection is cash + 70% of the rest of net worth + weeks of the last week's net
// worth gain, against the buyout price with 5% for its daily rise; ok is false when the
// state lacks them.
func (s *snapshot) cashBuyoutProjection(weeks int64) (projected, price int64, ok bool) {
	if s.Cash == nil || s.NetWorth == nil || s.Property == nil || s.Property.BuyoutPrice == nil {
		return 0, 0, false
	}
	projected = *s.Cash + max(0, *s.NetWorth-*s.Cash)*7/10
	if s.weekGain != nil {
		projected += max(0, *s.weekGain) * weeks
	}
	return projected, *s.Property.BuyoutPrice * 105 / 100, true
}
