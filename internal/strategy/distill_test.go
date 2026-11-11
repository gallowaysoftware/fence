package strategy

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestDistillPrompt(t *testing.T) {
	if !strings.HasPrefix(directivesAsk, directivesLead) {
		t.Fatal("directivesAsk no longer starts with directivesLead")
	}
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
	for _, tc := range []struct {
		name string
		gym  bool
	}{{"contract", false}, {"gym", true}} {
		t.Run(tc.name, func(t *testing.T) {
			system, user, err := DistillPrompt(PlanInput{Day: ev.GameDay, Snapshot: ev.Data}, tc.gym)
			if err != nil {
				t.Fatal(err)
			}
			if system != planSystem {
				t.Error("system prompt is not the plan system prompt")
			}
			if strings.Contains(user, "Write the plan for day") {
				t.Error("prompt still asks for a prose plan")
			}
			for _, want := range []string{"TODAY: day", "WHY: ", `"cash_floor"`, "Upgrades owned"} {
				if !strings.Contains(user, want) {
					t.Errorf("prompt lacks %q", want)
				}
			}
			if got := strings.Contains(user, `"engine"`); got != tc.gym {
				t.Errorf("gym extensions present = %v, want %v", got, tc.gym)
			}
		})
	}
}

func TestDistillDelta(t *testing.T) {
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
	base := `{"engine":"wine+furnace","buyout_by_day":140}`
	system, user, err := DistillDeltaPrompt(PlanInput{Day: ev.GameDay, Snapshot: ev.Data, BaseDirectives: base})
	if err != nil {
		t.Fatal(err)
	}
	if system != planSystem {
		t.Error("system prompt is not the plan system prompt")
	}
	if strings.Contains(user, "Write the plan for day") {
		t.Error("prompt still asks for a prose plan")
	}
	for _, want := range []string{"TODAY: day", `"buyout_by_day"`, "an empty object {} keeps all of them", base} {
		if !strings.Contains(user, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}

	for _, tc := range []struct {
		name, answer string
		want         []string
		err          bool
	}{
		{"empty", `{}`, nil, false},
		{"reasoned", `{"why":{"permit":"cash covers the fee"},"permit":"buy","scavenge":false}`, []string{"permit"}, false},
		{"prose", `no changes today`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := FinishDistillDelta(tc.answer, ev.Data, ev.GameDay, base)
			if (r.Err != nil) != tc.err {
				t.Fatalf("err = %v, want error %v", r.Err, tc.err)
			}
			if !slices.Equal(r.FromModel, tc.want) {
				t.Errorf("from model = %v, want %v", r.FromModel, tc.want)
			}
		})
	}
}
