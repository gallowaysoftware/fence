package twitch

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// BetOpened is the strategist's bet being staked (docs/SHOW.md, "Bets and callbacks"). ID is
// stable across the bet's life; By is its deadline day. Kind, Target and Item are optional and
// only shape the prediction's title and when it locks.
type BetOpened struct {
	ID     string
	Claim  string
	By     int
	Day    int
	Kind   string
	Target int64
	Item   string
}

// Bet outcomes a resolution carries. Abandoned covers a dropped or replaced bet and one that
// expired unjudged: either way nobody won.
const (
	BetWon       = "won"
	BetLost      = "lost"
	BetAbandoned = "abandoned"
)

// BetResolved is the bet settled.
type BetResolved struct {
	ID      string
	Outcome string
	Fact    string
}

// BetSignal is one of the two; exactly one field is set.
type BetSignal struct {
	Opened   *BetOpened
	Resolved *BetResolved
}

// BetsFromFeed reads a feed `bet` event (kind "bet", the bets-callbacks branch) into a signal.
// "due" (a judged bet waiting for the model's verdict) is no signal: it settles later.
func BetsFromFeed(ev FeedEvent) (BetSignal, bool) {
	if ev.Kind != "bet" || ev.Bet == nil {
		return BetSignal{}, false
	}
	b := ev.Bet
	id := BetID(ev.Run, b.Staked, b.Claim)
	switch b.Outcome {
	case "staked":
		return BetSignal{Opened: &BetOpened{ID: id, Claim: b.Claim, By: b.By, Day: b.Staked, Kind: b.Kind, Target: b.Target, Item: b.Item}}, true
	case "won", "lost":
		return BetSignal{Resolved: &BetResolved{ID: id, Outcome: b.Outcome, Fact: b.Fact}}, true
	case "abandoned", "expired":
		return BetSignal{Resolved: &BetResolved{ID: id, Outcome: BetAbandoned, Fact: b.Fact}}, true
	}
	return BetSignal{}, false
}

// BetID names a bet by its run, stake day and claim: the feed has no id of its own.
func BetID(run string, staked int, claim string) string {
	return fmt.Sprintf("%s/%d/%s", run, staked, claim)
}

// Synthetic is a script of made-up bets for a replay of a log that has none (the dress
// rehearsal predates bets). Each is opened on its day and settled by the feed's own clock, the way
// the bridge would settle a cash or net-worth bet: won when the clock reaches the target on or
// before the deadline, lost on the first event past it.
//
//	# day  kind       target  by   claim
//	31     net_worth  4500    35   Net worth past 4,500 by day 35
type Synthetic struct {
	bets []BetOpened
	open map[string]BetOpened
}

// ParseSynthetic reads a synthetic bet script.
func ParseSynthetic(r io.Reader) (*Synthetic, error) {
	s := &Synthetic{open: map[string]BetOpened{}}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 5 {
			return nil, fmt.Errorf("bets line %d: want DAY KIND TARGET BY CLAIM", n)
		}
		day, err1 := strconv.Atoi(f[0])
		target, err2 := strconv.ParseInt(f[2], 10, 64)
		by, err3 := strconv.Atoi(f[3])
		if err1 != nil || err2 != nil || err3 != nil || by < day {
			return nil, fmt.Errorf("bets line %d: bad numbers", n)
		}
		if f[1] != "cash" && f[1] != "net_worth" {
			return nil, fmt.Errorf("bets line %d: kind %q: synthetic bets are cash or net_worth", n, f[1])
		}
		claim := strings.Join(f[4:], " ")
		s.bets = append(s.bets, BetOpened{ID: BetID("synthetic", day, claim), Claim: claim, By: by, Day: day, Kind: f[1], Target: target})
	}
	return s, sc.Err()
}

// Feed returns the signals ev causes: a bet staked on a morning plan of its day, and open bets
// settled against ev's clock.
func (s *Synthetic) Feed(ev FeedEvent) []BetSignal {
	var out []BetSignal
	for _, b := range s.open {
		if r := settle(b, ev); r != nil {
			delete(s.open, b.ID)
			out = append(out, BetSignal{Resolved: r})
		}
	}
	if ev.Kind == "plan" && ev.Plan != nil {
		for i := 0; i < len(s.bets); i++ {
			b := s.bets[i]
			if b.Day > ev.Plan.Day {
				continue
			}
			s.bets = append(s.bets[:i], s.bets[i+1:]...)
			i--
			if b.Day < ev.Plan.Day {
				continue
			}
			for id, o := range s.open {
				delete(s.open, id)
				out = append(out, BetSignal{Resolved: &BetResolved{ID: o.ID, Outcome: BetAbandoned, Fact: fmt.Sprintf("replaced by a new bet on day %d", b.Day)}})
			}
			s.open[b.ID] = b
			bb := b
			out = append(out, BetSignal{Opened: &bb})
		}
	}
	return out
}

func settle(b BetOpened, ev FeedEvent) *BetResolved {
	if ev.Clock != nil && ev.Day >= b.Day {
		have, what := ev.Clock.Cash, "cash"
		if b.Kind == "net_worth" {
			have, what = ev.Clock.NetWorth, "net worth"
		}
		if have >= b.Target && ev.Day <= b.By {
			return &BetResolved{ID: b.ID, Outcome: BetWon, Fact: fmt.Sprintf("%s %d on day %d", what, have, ev.Day)}
		}
	}
	if ev.Day > b.By {
		return &BetResolved{ID: b.ID, Outcome: BetLost, Fact: fmt.Sprintf("not reached by day %d", b.By)}
	}
	return nil
}
