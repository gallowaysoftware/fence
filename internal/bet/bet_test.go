package bet

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func snap(phase string, day int, cash, nw int64, extra string) json.RawMessage {
	s := fmt.Sprintf(`{"phase":%q,"store":{"day":%d,"cash":%d,"net_worth":%d}`, phase, day, cash, nw)
	if extra != "" {
		s += "," + extra
	}
	return json.RawMessage(s + "}")
}

const stock = `"inventory":[{"uid":7,"name":"Medkit","type":"medical"},{"uid":8,"name":"Medkit","type":"medical"},{"uid":9,"name":"Wine"}],
"upgrades":{"owned":[{"ID":"VENDING_MACHINE","Name":"Vending Machine"}],"available":[{"ID":"SHOWCASE_I","Name":"Glass Showcase"}]}`

func stake(t *testing.T, tr *Tracker, b protocol.Bet, day int) {
	t.Helper()
	if err := Lint(&b, snap("morning", day, 1000, 3000, stock), day); err != nil {
		t.Fatalf("Lint: %v", err)
	}
	rs := tr.Apply(&protocol.BetMove{Stake: &b}, day)
	if len(rs) == 0 || rs[len(rs)-1].Outcome != Staked {
		t.Fatalf("stake: %+v", rs)
	}
}

func outcomes(rs []Resolution) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Outcome+":"+r.Fact)
	}
	return strings.Join(out, "|")
}

func TestTracker(t *testing.T) {
	tests := []struct {
		name  string
		bet   protocol.Bet
		steps func(tr *Tracker) []Resolution
		want  string
	}{
		{"cash reached before the deadline", protocol.Bet{Claim: "Cash past 2,000 by day 15", Kind: "cash", Target: 2000, By: 15},
			func(tr *Tracker) []Resolution {
				return append(tr.Snapshot(snap("close", 11, 1500, 3000, ""), 11), tr.Snapshot(snap("close", 12, 2100, 3000, ""), 12)...)
			}, "won:cash 2,100 on day 12"},
		{"cash still short at the deadline's close", protocol.Bet{Claim: "Cash past 2,000 by day 15", Kind: "cash", Target: 2000, By: 15},
			func(tr *Tracker) []Resolution {
				r := tr.Snapshot(snap("close", 15, 1500, 3000, ""), 15)
				return append(r, tr.Snapshot(snap("sleep", 15, 1900, 3000, ""), 15)...)
			}, "lost:cash 1,900 on day 15, short of 2,000"},
		{"sell by uid", protocol.Bet{Claim: "Both medkits gone by day 12", Kind: "sell", UIDs: []int64{7, 8}, By: 12},
			func(tr *Tracker) []Resolution {
				tr.Sold([]int64{7}, 120)
				r := tr.Snapshot(snap("close", 11, 1000, 3000, stock), 11)
				tr.Sold([]int64{8, 9}, 300)
				return append(r, tr.Snapshot(snap("close", 12, 1000, 3000, stock), 12)...)
			}, "won:Medkit sold by day 12 for 420"},
		{"hold broken by a sale", protocol.Bet{Claim: "Holding the wine to day 20", Kind: "hold", Item: "wine", By: 20},
			func(tr *Tracker) []Resolution {
				tr.Sold([]int64{9}, 80)
				return tr.Snapshot(snap("close", 11, 1000, 3000, `"inventory":[{"uid":7,"name":"Medkit"}]`), 11)
			}, "lost:wine gone by day 11"},
		{"upgrade bought", protocol.Bet{Claim: "Showcase by day 14", Kind: "upgrade", Item: "glass showcase", By: 14},
			func(tr *Tracker) []Resolution {
				return tr.Snapshot(snap("sleep", 13, 300, 3000, `"upgrades":{"owned":[{"ID":"SHOWCASE_I","Name":"Glass Showcase"}]}`), 13)
			}, "won:bought by day 13"},
		{"event rise", protocol.Bet{Claim: "Medical rises by day 30", Kind: "event", Item: "medical", By: 30},
			func(tr *Tracker) []Resolution {
				return tr.Snapshot(snap("morning", 14, 300, 3000, `"market_events":{"active":[{"name":"Flu Season","start_day":14,"modifiers":[{"item_type":"medical","value_pct":50}]}]}`), 14)
			}, "won:Flu Season on day 14: medical up 50%"},
		{"payment on the deadline, after the shop opens", protocol.Bet{Claim: "Rent paid on day 14", Kind: "payment", By: 14},
			func(tr *Tracker) []Resolution {
				return append(tr.Snapshot(snap("open", 14, 1000, 3000, ""), 14), tr.Money("rent_paid", 14)...)
			}, "won:paid on day 14"},
		{"payment missed", protocol.Bet{Claim: "Rent covered on day 14", Kind: "payment", By: 14},
			func(tr *Tracker) []Resolution { return tr.Money("rent_missed", 14) }, "lost:missed on day 14"},
		{"judged bet comes due, then the model judges it", protocol.Bet{Claim: "Security leaves us alone till day 12", Kind: "judged", By: 12},
			func(tr *Tracker) []Resolution {
				r := tr.Snapshot(snap("close", 12, 1000, 3000, ""), 12)
				r = append(r, tr.Snapshot(snap("morning", 13, 1000, 3000, ""), 13)...)
				return append(r, tr.Apply(&protocol.BetMove{Verdict: &protocol.BetVerdict{Won: true, Why: "no inspection logged"}}, 13)...)
			}, "due:|won:no inspection logged"},
		{"a new bet abandons the open one; the same one again keeps it", protocol.Bet{Claim: "Cash past 2,000 by day 15", Kind: "cash", Target: 2000, By: 15},
			func(tr *Tracker) []Resolution {
				same := protocol.Bet{Claim: "Two grand by day 15, I said", Kind: "cash", Target: 2000, By: 15}
				r := tr.Apply(&protocol.BetMove{Stake: &same}, 11)
				next := protocol.Bet{Claim: "Net worth 5,000 by day 20", Kind: "net_worth", Target: 5000, By: 20}
				return append(r, tr.Apply(&protocol.BetMove{Stake: &next}, 12)...)
			}, "abandoned:replaced by a new bet on day 12|staked:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tr Tracker
			stake(t, &tr, tt.bet, 10)
			if got := outcomes(tt.steps(&tr)); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLateStake(t *testing.T) {
	var tr Tracker
	tr.Snapshot(snap("morning", 22, 1000, 3000, ""), 22)
	if rs := tr.Apply(&protocol.BetMove{Stake: &protocol.Bet{Claim: "x", Kind: "upgrade", Item: "A", By: 21, Day: 21}}, 21); len(rs) != 0 || tr.Open() != nil {
		t.Errorf("a plan landing after its deadline staked %+v", rs)
	}
}

func TestDueWaitsForAPlan(t *testing.T) {
	var tr Tracker
	b := protocol.Bet{Claim: "Security stays away", Kind: "judged", By: 8}
	tr.Apply(&protocol.BetMove{Stake: &b}, 7)
	tr.Snapshot(snap("morning", 9, 100, 100, ""), 9)
	if rs := tr.Apply(nil, 10); len(rs) != 0 || !tr.IsDue() {
		t.Fatalf("a day without a plan settled a due bet: %+v", rs)
	}
	if got := outcomes(tr.Apply(&protocol.BetMove{Drop: true}, 14)); got != "expired:no verdict" {
		t.Errorf("weekly plan without a verdict: %q", got)
	}
}

func TestRepeats(t *testing.T) {
	var tr Tracker
	b := protocol.Bet{Claim: "Referral tonight", Kind: "upgrade", Item: "BUY_REFERAL", By: 10}
	tr.Apply(&protocol.BetMove{Stake: &b}, 10)
	tr.Snapshot(snap("sleep", 10, 100, 100, `"upgrades":{"owned":[]}`), 10)
	again := b
	again.By = 11
	if !tr.Repeats(&again, 11) || tr.Repeats(&again, 13) {
		t.Error("a bet lost yesterday may not come straight back; three days on it may")
	}
}

func TestLint(t *testing.T) {
	tests := []struct {
		name string
		bet  protocol.Bet
		want string
	}{
		{"already true is filler", protocol.Bet{Claim: "Cash past 900", Kind: "cash", Target: 900, By: 12}, "already true"},
		{"deadline today", protocol.Bet{Claim: "x", Kind: "cash", Target: 5000, By: 10}, "deadline"},
		{"deadline too far", protocol.Bet{Claim: "x", Kind: "cash", Target: 5000, By: 60}, "deadline"},
		{"unknown kind", protocol.Bet{Claim: "x", Kind: "vibes", By: 12}, "unknown kind"},
		{"no claim", protocol.Bet{Kind: "cash", Target: 5000, By: 12}, "no claim"},
		{"item not in stock", protocol.Bet{Claim: "x", Kind: "sell", Item: "rifle", By: 12}, "names no item"},
		{"owned upgrade", protocol.Bet{Claim: "x", Kind: "upgrade", Item: "VENDING_MACHINE", By: 12}, "already true"},
		{"upgrade not on offer", protocol.Bet{Claim: "x", Kind: "upgrade", Item: "MOON_BASE", By: 12}, "not on offer"},
		{"fine", protocol.Bet{Claim: "Five grand by day 30", Kind: "cash", Target: 5000, By: 30}, ""},
		{"no event type", protocol.Bet{Claim: "x", Kind: "event", By: 12}, "no item type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Lint(&tt.bet, snap("morning", 10, 1000, 3000, stock), 10)
			if (err == nil) != (tt.want == "") || (err != nil && !strings.Contains(err.Error(), tt.want)) {
				t.Errorf("Lint = %v, want %q", err, tt.want)
			}
		})
	}
	ev := protocol.Bet{Claim: "Rent paid and grace bought", Kind: "event", Item: "rent_paid_and_grace", By: 12}
	known := protocol.Bet{Claim: "Household rises", Kind: "event", Item: "household_good", By: 12}
	raw := snap("morning", 10, 1000, 3000, `"clientele":{"display_buyers":["household_good"]}`)
	if Lint(&ev, raw, 10) != nil || ev.Kind != "judged" || Lint(&known, raw, 10) != nil || known.Kind != "event" {
		t.Errorf("event kinds: %+v %+v", ev, known)
	}
	b := protocol.Bet{Claim: "Showcase by day 14", Kind: "upgrade", Item: "Glass_Showcase", By: 14}
	if err := Lint(&b, snap("morning", 10, 1000, 3000, stock), 10); err != nil || b.Item != "SHOWCASE_I" || b.Day != 10 {
		t.Errorf("Lint normalised to %+v, %v", b, err)
	}
}
