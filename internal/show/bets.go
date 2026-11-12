package show

import (
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/bet"
)

// betEvents turns the tracker's stakes and settlements into feed events. A stake and a
// settlement are moments, so the narrator sets the claim up and calls back to it; a judged
// bet coming due and one never judged are panel lines only.
func betEvents(rs []bet.Resolution, clock *Clock) []Event {
	var out []Event
	for _, r := range rs {
		st := &BetState{Claim: r.Bet.Claim, Kind: r.Bet.Kind, Target: r.Bet.Target, Item: r.Bet.Item, By: r.Bet.By, Staked: r.Bet.Day,
			Outcome: r.Outcome, Fact: r.Fact, Judged: r.Judged}
		ev := Event{Kind: KindBet, Day: r.Day, Level: LevelMoment, Bet: st, Clock: clock}
		// A model's verdict can run long; the caption is what the narrator reads first.
		fact := strings.TrimRight(clip(capitalize(r.Fact), 120), ".")
		judged := ""
		if r.Judged {
			judged = " (judged by the model)"
		}
		switch r.Outcome {
		case bet.Staked:
			ev.Beat = BeatBetMade
			ev.Text = fmt.Sprintf("The bet, due day %d: %s", r.Bet.By, r.Bet.Claim)
		case bet.Won:
			ev.Beat = BeatBetWon
			ev.Text = fmt.Sprintf("Bet won%s: %q, staked on day %d. %s.", judged, r.Bet.Claim, r.Bet.Day, fact)
		case bet.Lost:
			ev.Beat = BeatBetLost
			ev.Text = fmt.Sprintf("Bet lost%s: %q, staked on day %d. %s.", judged, r.Bet.Claim, r.Bet.Day, fact)
		case bet.Abandoned:
			ev.Beat = BeatBetAbandoned
			ev.Text = fmt.Sprintf("Bet abandoned: %q, staked on day %d. %s.", r.Bet.Claim, r.Bet.Day, fact)
		case bet.Due:
			ev.Level = LevelNotable
			ev.Text = fmt.Sprintf("Bet due: %q. The state cannot settle it; the model judges it in the next plan.", r.Bet.Claim)
		case bet.Expired:
			ev.Level = LevelNotable
			ev.Text = fmt.Sprintf("Bet expired unjudged: %q.", r.Bet.Claim)
		default:
			continue
		}
		if ev.Text != "" && ev.Text[len(ev.Text)-1] != '.' && ev.Text[len(ev.Text)-1] != '!' && ev.Text[len(ev.Text)-1] != '?' {
			ev.Text += "."
		}
		out = append(out, ev)
	}
	return out
}

// Bets is the bet events of a plan: what its move staked, dropped or judged. Producers call
// it after Plan with the same input.
func (b *Builder) Bets(in PlanInput) []Event {
	return betEvents(b.bets.Apply(in.Bet, in.Day), b.Clock())
}

// betCues tell the narrator how to set a bet up and how to call back to it.
var betCues = map[string]string{
	BeatBetMade:      "you are staking a bet on the run, out loud. Say the claim in your own words as a dare or a promise, keeping its number or its day, so the viewers can hold you to it.",
	BeatBetWon:       "a bet you staked has come good. Call back to your own claim from the day you made it, and what settled it (the moment says; invent no other reason), then crow about it in character.",
	BeatBetLost:      "a bet you staked has failed. Call back to your own claim from the day you made it, and what settled it (the moment says; invent no other reason), rueful or with gallows humour; own it.",
	BeatBetAbandoned: "you are walking away from a bet you staked. Call back to the claim with a wry, self-aware line about why it no longer stands.",
}

// betBuildUp are the moments that may mention the open bet: where the shop stands against it.
var betBuildUp = map[string]bool{BeatMorning: true, BeatMilestone: true, BeatNight: true}

// history is what the line may call back to: the open bet on a moment that bears on it, and
// the past dealings of the character the moment is with, into f. Their numbers become facts.
func (n *Narrator) history(ev Event, f *Facts) []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	var b strings.Builder
	var callback []string
	switch {
	case betCues[ev.Beat] != "":
		callback = append(callback, "bet")
	case n.bet != nil && (betBuildUp[ev.Beat] || ev.Kind == KindDay):
		fmt.Fprintf(&b, "Your open bet, staked on day %d, due day %d: %s. Mention it only if this moment moves you toward or away from it.\n", n.bet.Staked, n.bet.By, n.bet.Claim)
		f.allow(n.bet.Claim, int64(n.bet.By), int64(n.bet.Staked))
		callback = append(callback, "bet")
	}
	if p := ev.Plan; ev.Beat == BeatNight && p != nil && p.Source == "diary" {
		fmt.Fprintf(&b, "Tonight is also your diary: this morning's plan was: %s. %s. After the night's news, say in a few words how the day went against that plan%s.\n",
			strings.Join(p.Headline, "; "), strings.Join(p.Goals, "; "), map[bool]string{true: " and your bet", false: ""}[n.bet != nil])
		callback = append(callback, "diary")
	}
	if bg := n.ledger.Background(ev.Who, ev.Seq); bg != "" {
		fmt.Fprintf(&b, "You have dealt with them before. %s. A few words calling back to that history are welcome.\n", bg)
		callback = append(callback, "ledger")
	}
	f.history = b.String()
	return callback
}
