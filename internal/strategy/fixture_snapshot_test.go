package strategy

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The mod's real snapshot shape (protocol/fixtures) must parse and render, not just the
// strategist's own scenarios.
func TestProtocolFixtureSnapshotRenders(t *testing.T) {
	raw, err := os.ReadFile("../../protocol/fixtures/event.day_snapshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var ev struct {
		GameDay int             `json:"game_day"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatal(err)
	}
	s, err := parseSnapshot(ev.Data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var b strings.Builder
	s.render(&b, ev.GameDay)
	out := b.String()
	for _, want := range []string{"Upgrades owned", "Services in use", "Electronics"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered snapshot lacks %q:\n%s", want, out)
		}
	}
}
