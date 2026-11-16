package show

import (
	"testing"
)

func press(day int, machine, button string) map[string]any {
	return ev("autopilot", day, map[string]any{"day": day, "phase": "morning", "action": "chore", "target": map[string]any{"uid": 17, "name": machine}, "why": "pressed " + button, "rule": "chore"})
}

func thermostat(day, n int) map[string]any {
	why := "the heating plate switched 1 time on its readout to hold the batch's window"
	if n != 1 {
		why = "the heating plate switched 2 times on its readout to hold the batch's window"
	}
	return ev("autopilot", day, map[string]any{"day": day, "phase": "customer", "action": "thermostat", "why": why, "rule": "player:lab", "client": "Cartel Handler", "client_seq": 4})
}

// A run of presses is one lab line, told when the autopilot moves on; the plate's corrections alone are
// ticker news and never name the customer who happened to be at the counter.
func TestLabRunIsOneLine(t *testing.T) {
	b := NewBuilder(Player{ID: "p1"})
	evs := feedAll(t, b,
		press(8, "Chem Finisher", "Output"),
		ev("autopilot", 8, map[string]any{"day": 8, "action": "chore_move", "target": map[string]any{"uid": 266, "name": "Vial of \"Dream Dust\""}, "why": "into Chem Finisher"}),
		press(8, "Chem Finisher", "Output"),
		press(8, "Blender", "Start"),
		thermostat(8, 2),
		ev("autopilot", 8, map[string]any{"day": 8, "action": "open_shutter", "why": "the window is set; open for business"}),
		thermostat(8, 1),
		ev("waking_up", 9, map[string]any{}),
	)
	th := find(evs, KindThought)
	if len(th) != 2 {
		t.Fatalf("thoughts = %+v", th)
	}
	if l := th[0]; l.Level != LevelNotable || l.Beat != BeatLab ||
		l.Text != "In the lab: pressed Output on the Chem Finisher (×2) and Start on the Blender; the heating plate switched 2 times to hold the batch's window." {
		t.Errorf("lab = %+v", l)
	}
	if l := th[1]; l.Level != LevelTicker || l.Beat != "" || l.Text != "In the lab: the heating plate switched 1 time to hold the batch's window." {
		t.Errorf("thermostat only = %+v", l)
	}
}

func dialogue(day int, seq int64, text string, history ...string) map[string]any {
	return ev("human_action", day, map[string]any{"client_seq": seq, "action": "dialogue_choice", "text": text, "history": history, "actor": "autopilot"})
}

// The handler's count is the chemist run's weekly moment; the check-in names the next count. The
// autopilot's own record of the same click adds nothing.
func TestCartelBeats(t *testing.T) {
	b := NewBuilder(Player{ID: "p1"})
	evs := feedAll(t, b,
		dialogue(4, 18, "[All Good]", "Cartel Handler: How the batch coming along?", "Cartel Handler: You need to cook at least 10 Dream Dust.", "Cartel Handler: It must be at least 40% pure"),
		ev("autopilot", 4, map[string]any{"day": 4, "action": "dialogue_choice", "detail": "cartelHandlerCheckin [1]", "why": "[All Good]", "rule": "player:cartel_all_good", "client": "Cartel Handler", "client_seq": 18}),
		dialogue(8, 46, "[Continue]", "Cartel Handler: Do you have the 10 Dream Dust?", "Cartel Handler: It must be at least 40% pure", "Cartel Handler: Put them on the counter"),
		ev("autopilot", 8, map[string]any{"day": 8, "action": "dialogue_choice", "detail": "cartelHandlerWeekly [0]", "why": "[Continue]", "rule": "player:cartel_yes", "client": "Cartel Handler", "client_seq": 46}),
	)
	th := find(evs, KindThought)
	if len(th) != 2 {
		t.Fatalf("thoughts = %+v", th)
	}
	if c := th[0]; c.Level != LevelNotable || c.Who != cartelHandler || c.Text != "The cartel checks in: told the handler all is well; the next count wants 10 Dream Dust at 40%+ purity." {
		t.Errorf("check-in = %+v", c)
	}
	if c := th[1]; c.Level != LevelMoment || c.Beat != BeatCartelQuota || c.Text != "The cartel's count: the handler takes the Dream Dust off the counter (quota 10 at 40%+ purity)." {
		t.Errorf("count = %+v", c)
	}
}

// The live log reports the board click as the game's human_action and as the autopilot's record: one beat.
func TestBoardJoinOnce(t *testing.T) {
	b := NewBuilder(Player{ID: "p1"})
	evs := feedAll(t, b,
		dialogue(5, 30, "[Join the board (170 Credits)]", "Appraiser Board Member: Will you join?"),
		ev("autopilot", 5, map[string]any{"day": 5, "action": "dialogue_choice", "detail": boardIntroID + " [0]", "why": "[Join the board (170 Credits)]", "rule": "player:appraisal", "client": boardMember, "client_seq": 30}),
	)
	if len(evs) != 1 || evs[0].Beat != BeatAppraisalJoin {
		t.Fatalf("events = %+v", evs)
	}
}

func TestUnnamedItemsCount(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{1, "an item"}, {8, "8 items"}} {
		if got := countOf(c.n, "item"); got != c.want {
			t.Errorf("countOf(%d) = %q, want %q", c.n, got, c.want)
		}
	}
	if got := countOf(2, "Wine"); got != "2× Wine" {
		t.Errorf("countOf(2, Wine) = %q", got)
	}
}
