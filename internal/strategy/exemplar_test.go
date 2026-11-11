package strategy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestExemplarsSimilar(t *testing.T) {
	raw := json.RawMessage(fmt.Sprintf(wineShop, 900))
	s := snap(t, string(raw))
	q := s.features(90)
	far := make([]float64, len(q))
	for i := range far {
		far[i] = q[i] + 0.2
	}
	long := strings.Repeat("x", 400)
	x := Exemplars{
		{Run: "a", Difficulty: "normal", Day: 90, Features: q, Situation: "near " + long, Chosen: "buy SHOWCASE_I", Next: "nw up", Weight: 1},
		{Run: "a", Difficulty: "normal", Day: 91, Features: q, Situation: "same run next day", Weight: 1},
		{Run: "b", Difficulty: "normal", Day: 60, Features: far, Situation: "far but strong", Weight: 1},
		{Run: "c", Difficulty: "normal", Day: 88, Features: q, Situation: "near but weak", Weight: 0.05},
		{Run: "d", Difficulty: "hard", Day: 90, Features: q, Situation: "other difficulty", Weight: 1},
	}
	got := x.Similar(raw, 90, 2)
	if len(got) > MaxExemplarBytes {
		t.Errorf("%d bytes, over %d", len(got), MaxExemplarBytes)
	}
	for _, w := range []string{"near xx", "far but strong"} {
		if !strings.Contains(got, w) {
			t.Errorf("lacks %q:\n%s", w, got)
		}
	}
	for _, w := range []string{"same run next day", "near but weak", "other difficulty"} {
		if strings.Contains(got, w) {
			t.Errorf("has %q:\n%s", w, got)
		}
	}
	if Exemplars(nil).Similar(raw, 90, 3) != "" {
		t.Error("an empty index shows something")
	}
}
