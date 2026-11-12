package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/bet"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// betBrief is the run's bet as the delta ask shows it: what settled since the last plan, the
// bet in force, and a due bet's call for a verdict.
func betBrief(t *bet.Tracker, news []bet.Resolution, morning *snapshot) string {
	var b strings.Builder
	for _, r := range news {
		switch r.Outcome {
		case bet.Won, bet.Lost:
			fmt.Fprintf(&b, "Your bet %s was %s: %s.\n", bet.Describe(r.Bet), strings.ToUpper(r.Outcome), r.Fact)
		}
	}
	o := t.Open()
	switch {
	case o == nil:
		b.WriteString("You have no open bet.")
	case t.IsDue():
		fmt.Fprintf(&b, "Your bet %s is due and cannot be checked by the state: judge it now from today's state, honestly, with \"bet_verdict\": {\"won\": true or false, \"why\": \"<the fact in today's state>\"}. You may also stake a new bet.", bet.Describe(*o))
	default:
		fmt.Fprintf(&b, "Your open bet: %s%s. Leave \"bet\" out to keep it; \"bet\": \"drop\" abandons it; a new bet replaces it and the old one counts as abandoned.", bet.Describe(*o), betProgress(*o, morning))
	}
	return b.String()
}

// noteBets keeps what the state settled for the next plan to read. Callers hold s.mu.
func (s *Strategist) noteBets(r *run, rs []bet.Resolution) {
	for _, res := range rs {
		s.log.Info("bet settled", "day", res.Day, "outcome", res.Outcome, "claim", res.Bet.Claim, "fact", res.Fact)
	}
	r.betNews = append(r.betNews, rs...)
}

// betProgress is where an open cash or net-worth bet stands this morning.
func betProgress(b protocol.Bet, s *snapshot) string {
	if s == nil {
		return ""
	}
	switch {
	case b.Kind == "cash" && s.Cash != nil:
		return fmt.Sprintf("; cash now %d of %d", *s.Cash, b.Target)
	case b.Kind == "net_worth" && s.NetWorth != nil:
		return fmt.Sprintf("; net worth now %d of %d", *s.NetWorth, b.Target)
	}
	return ""
}

// applyBet lints the plan's bet move against the morning it was written on, applies it to the
// run's tracker and logs it, so a show replaying bridge.log sees the same move. Callers hold
// s.mu.
func (s *Strategist) applyBet(r *run, d *dayState, n int, mv *protocol.BetMove) {
	r.betNews = nil
	if mv != nil && mv.Stake != nil {
		err := bet.Lint(mv.Stake, d.morningRaw, n)
		switch {
		case err != nil:
		case r.bets.Repeats(mv.Stake, n):
			err = fmt.Errorf("the same bet was settled in the last %d days", bet.RepeatDays)
		case r.bets.Late(mv.Stake):
			err = fmt.Errorf("the plan landed after day %d ended", mv.Stake.By)
		}
		if err != nil {
			s.log.Info("bet rejected", "day", n, "claim", mv.Stake.Claim, "err", err)
			mv.Stake = nil
		}
	}
	if mv != nil && mv.Verdict != nil && !r.bets.IsDue() {
		mv.Verdict = nil
	}
	if r.bets.IsDue() && (mv == nil || mv.Verdict == nil) {
		// The plan was asked for a verdict and gave none: the bet expires, and the logged drop
		// tells the show so.
		if mv == nil {
			mv = &protocol.BetMove{}
		}
		mv.Drop = true
	}
	if mv != nil && *mv == (protocol.BetMove{}) {
		mv = nil
	}
	for _, res := range r.bets.Apply(mv, n) {
		if res.Outcome == bet.Staked {
			continue
		}
		s.log.Info("bet settled", "day", n, "outcome", res.Outcome, "claim", res.Bet.Claim, "fact", res.Fact)
	}
	if n >= r.planDay {
		r.betMove = mv
	}
	if mv == nil {
		return
	}
	if b, err := json.Marshal(mv); err == nil {
		s.log.Info("bet", "day", n, "json", string(b))
	}
}

// CurrentBet is the latest plan's move on session's bet, nil when it made none.
func (s *Strategist) CurrentBet(session string) *protocol.BetMove {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.sessions[session]
	if r == nil || r.betMove == nil {
		return nil
	}
	mv := *r.betMove
	return &mv
}
