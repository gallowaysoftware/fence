package show

import (
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestBuilderBets(t *testing.T) {
	stake := &protocol.BetMove{Stake: &protocol.Bet{Claim: "Lamp gone by day 4 for 50 or more", Kind: "sell", Item: "Lamp", Target: 50, By: 4, Day: 3}}
	b := NewBuilder(Player{})
	b.Plans = func(day int) *PlanInput {
		if day == 3 {
			return &PlanInput{Day: 3, Text: "GOALS\n- sell the lamp", Bet: stake}
		}
		return &PlanInput{Day: day, Text: "GOALS\n- keep going"}
	}
	lines := []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r1"}),
		snap("morning", 3, 2600, map[string]any{"uid": 1, "name": "Lamp"}),
		ev("customer_observed", 3, map[string]any{"client_seq": 1, "context": map[string]any{"client": map[string]any{"name": "Eleanor", "intent": "dialogue"}}}),
	}
	lines = append(lines, counterLines("c1", 1, "Eleanor", 60)...)
	lines = append(lines, snap("close", 3, 2600))
	var bets []Event
	for _, e := range feedAll(t, b, lines...) {
		if e.Kind == KindBet {
			bets = append(bets, e)
		}
		if e.Kind == KindTrade && e.Who != "Eleanor" {
			t.Errorf("a trade with a speaker of a dialogue is not hers: %+v", e)
		}
	}
	if len(bets) != 2 {
		t.Fatalf("bet events %+v", bets)
	}
	if bets[0].Beat != BeatBetMade || bets[0].Level != LevelMoment || bets[0].Text != "The bet, due day 4: Lamp gone by day 4 for 50 or more." {
		t.Errorf("stake %+v", bets[0])
	}
	if bets[1].Beat != BeatBetWon || bets[1].Text != `Bet won: "Lamp gone by day 4 for 50 or more", staked on day 3. Lamp sold by day 3 for 60.` {
		t.Errorf("settlement %+v", bets[1])
	}
	if f := FactsOf(bets[1]); len(CheckLine("Lamp gone for sixty, like I said. Nobody bets against me.", f)) != 0 ||
		len(CheckLine("I lost the bet on that lamp.", f)) == 0 {
		t.Error("fact check on a won bet")
	}
}

func TestIsCharacter(t *testing.T) {
	speakers := map[string]bool{"Eleanor": true, "Foodie Tourist": true}
	for name, want := range map[string]bool{
		"Dr. Jackson": true, "Officer Lun": true, "The Mentor": true, "Eleanor": true, "Eleanor's Uncle": true,
		"Spacer": false, "Lower Level Citizen": false, "Security Officer": false, "Foodie Tourist": false, "Merchant": false, "": false,
	} {
		if got := isCharacter(name, speakers); got != want {
			t.Errorf("isCharacter(%q) = %v", name, got)
		}
	}
}

func TestLedger(t *testing.T) {
	var l Ledger
	l.Note(Event{Seq: 1, Who: jackson, Kind: KindTrade, Trade: &Trade{Side: "bought", Items: []string{"2× Furnace", "Scrap 9"}}})
	l.Note(Event{Seq: 2, Who: jackson, Text: "Haggled with Dr. Jackson (demand discount 10%): walked out."})
	l.Note(Event{Seq: 3, Who: georgio, Kind: KindMoney, Amount: -700})
	l.Note(Event{Seq: 4, Who: jackson, Text: "Haggled with Dr. Jackson (demand discount 10%): walked out."})
	if got := l.Background(jackson, 4); got != "Dr. Jackson, before today's moment: sold us Furnace; then walked out on our haggle (most recent last)" {
		t.Errorf("background %q", got)
	}
	if got := l.Background(georgio, 3); got != "" {
		t.Errorf("history includes the moment itself: %q", got)
	}
	for i := range 20 {
		l.Note(Event{Seq: int64(10 + i), Who: string(rune('A'+i)) + "x", Kind: KindMoney, Amount: 1})
	}
	if len(l.people) > ledgerPeople {
		t.Errorf("ledger holds %d people", len(l.people))
	}
}

func TestNarratorHistory(t *testing.T) {
	n := &Narrator{}
	n.bet = &BetState{Claim: "Cash past 9,000 by day 60", By: 60, Staked: 32, Outcome: "staked"}
	n.ledger.Note(Event{Seq: 1, Who: georgio, Kind: KindMoney, Amount: -700})
	ev := Event{Seq: 5, Day: 40, Kind: KindMoney, Who: georgio, Amount: 2800, Text: "Georgio paid back 2800.", Beat: BeatMorning}
	f := FactsOf(ev)
	cb := n.history(ev, &f)
	if strings.Join(cb, ",") != "bet,ledger" || !strings.Contains(f.history, "Cash past 9,000 by day 60") || !strings.Contains(f.history, "borrowed money from us") {
		t.Fatalf("history %v %q", cb, f.history)
	}
	if faults := CheckLine("Nine thousand by day 60, I said, and Georgio is paying.", f); len(faults) != 0 {
		t.Errorf("the open bet's numbers are facts: %v", faults)
	}
}

func TestNightDiary(t *testing.T) {
	b := NewBuilder(Player{})
	b.Plans = func(day int) *PlanInput {
		return &PlanInput{Day: day, Text: "GOALS\n- Close day 3 with cash 2,900\n- Sell the lamp"}
	}
	b.goOut("dumping_ground")
	evs := feedAll(t, b, ev("run_start", 1, map[string]any{"run_id": "r1"}),
		snap("morning", 3, 2600, map[string]any{"uid": 1, "name": "Lamp"}),
		snap("close", 3, 2600, map[string]any{"uid": 1, "name": "Lamp"}))
	b.goOut("dumping_ground")
	evs = append(evs, feedAll(t, b, snap("sleep", 3, 2600, map[string]any{"uid": 1, "name": "Lamp"}, map[string]any{"uid": 2, "name": "Scrap"}))...)
	night := evs[len(evs)-1]
	if night.Beat != BeatNight || night.Plan == nil || night.Plan.Source != "diary" {
		t.Fatalf("night %+v", night)
	}
	n := &Narrator{}
	f := FactsOf(night)
	if cb := n.history(night, &f); !strings.Contains(strings.Join(cb, ","), "diary") || !strings.Contains(f.history, "Close day 3 with cash 2,900") {
		t.Fatalf("diary %v %q", cb, f.history)
	}
	if faults := CheckLine("Back with scrap, and 2,900 never came.", f); len(faults) != 0 {
		t.Errorf("the plan's numbers are facts: %v", faults)
	}
}
