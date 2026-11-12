package show

import "testing"

// TestForkIsNotAPlan: a strategist's fork reaches the feed as it was written, without the
// plan event a hybrid slate makes of its chosen candidate.
func TestForkIsNotAPlan(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	evs, err := b.Feed([]byte(`{"type":"show","ts":"2026-01-01T00:00:01Z","show":{"day":70,"kind":"candidates","level":2,"text":"Day 70, the loan shark: later.",` +
		`"candidates":{"day":70,"metric":"choice","options":[{"label":"later","score":0,"chosen":true,"why":"cash buyout by day 98"},{"label":"sign","score":0}]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || evs[0].Kind != KindCandidates || evs[0].V != SchemaVersion {
		t.Fatalf("%+v", evs)
	}
}

// TestAgencyMomentsRank: the day-0 route is a pivot that ranks with a run start, a fork with
// the run's big money moments, so the narrator and the VOD chapters pick them up.
func TestAgencyMomentsRank(t *testing.T) {
	route := Event{Kind: KindPlan, Level: LevelMoment, Beat: BeatRouteChoice, Plan: &Plan{Change: true, Changes: []string{"run the smelter route"}}}
	fork := Event{Kind: KindCandidates, Level: LevelMoment, Beat: BeatFork}
	if Priority(route) != 5 || Priority(fork) != 4 || !PlanPivot(route) {
		t.Errorf("route %d (pivot %v), fork %d", Priority(route), PlanPivot(route), Priority(fork))
	}
}
