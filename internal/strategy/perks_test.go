package strategy

import (
	"strings"
	"testing"
)

func snap(t *testing.T, raw string) *snapshot {
	t.Helper()
	s, err := parseSnapshot([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPerkIndexCoversEveryPerk(t *testing.T) {
	for _, id := range []string{"aug_menace", "AUG_MENACE", "LLREFERAL", "llreferal", "BM_HONOR"} {
		if p, ok := lookupPerk(id); !ok || p.Strategy == "" {
			t.Fatalf("lookupPerk(%q) = %+v, %v", id, p, ok)
		}
	}
	if len(perkIndex) < 90 {
		t.Fatalf("only %d perks indexed", len(perkIndex))
	}
}

func TestActivePerksBlock(t *testing.T) {
	s := snap(t, `{"phase":"morning","store":{"day":20,"difficulty":"normal"},"perks":{"starting":["aug_menace","nonexistent_perk"],"reputation":["LLREFERAL"]}}`)
	var b strings.Builder
	s.renderPerks(&b)
	got := b.String()
	for _, want := range []string{"ACTIVE PERKS:\n", "- Aug Menace (starting): ", "- Referral (Lower Levels 20): ", "- nonexistent_perk (starting)\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
	if !s.augMenace() {
		t.Fatal("aug_menace in the perks block should switch on the menace rules")
	}
}

func TestActivePerksFallBackToUnlockedReputation(t *testing.T) {
	s := snap(t, `{"phase":"morning","reputation":{"lower_levels":{"value":25,"unlocked_perks":["LLREFERAL"]}}}`)
	var b strings.Builder
	s.renderPerks(&b)
	if !strings.Contains(b.String(), "Referral (Lower Levels 20)") {
		t.Fatalf("got %q", b.String())
	}
	var none strings.Builder
	snap(t, `{"phase":"morning"}`).renderPerks(&none)
	if none.Len() != 0 {
		t.Fatalf("no perks should render nothing, got %q", none.String())
	}
}

func TestBayRoomInTheHotStockRule(t *testing.T) {
	hot := `{"uid":2,"id":"radio","name":"Radio","where":"stock","estimated_value":400,"stolen_heat":60}`
	tests := []struct{ name, inv, want, not string }{
		{"room left", `[{"uid":1,"id":"smuggler_bay","where":"stock","capacity":{"cells":12,"free":5}},` + hot + `]`, "Smuggler bays: 1 with 5 of 12 cells free: cap hot stock to bay room", "second bay"},
		{"full", `[{"uid":1,"id":"smuggler_bay","where":"stock","capacity":{"cells":12,"free":0}},` + hot + `]`, "A second bay is worth up to the loss it would hide: about 240 credits", "cells free"},
		{"none", `[` + hot + `]`, "No smuggler bay: every hot item is exposed", "Smuggler bays:"},
		{"unknown room", `[{"uid":1,"id":"smuggler_bay","where":"stock"},` + hot + `]`, "room not reported", "cells"},
		{"all hidden", `[{"uid":1,"id":"smuggler_bay","where":"stock","capacity":{"cells":12,"free":3}},{"uid":2,"id":"radio","where":"inside:1","stolen_heat":60}]`, "Hot stock: all of it in smuggler bays. Smuggler bays: 1 with 3 of 12 cells free", "exposed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			snap(t, `{"phase":"morning","inventory":`+tt.inv+`}`).renderHotExposure(&b)
			if !strings.Contains(b.String(), tt.want) || strings.Contains(b.String(), tt.not) {
				t.Fatalf("got %q", b.String())
			}
		})
	}
}

func TestLintRepDiscounts(t *testing.T) {
	s := snap(t, `{"phase":"morning","reputation":{"lower_levels":{"value":12},"security":{"value":-85}}}`)
	tests := []struct {
		line string
		bad  bool
	}{
		{"- Offer 10% discounts to Lower Levels customers to reach the Referral perk", true},
		{"- Be generous with Upper Levels buyers to build rep", true},
		{"- Give Security customers a discount to climb back from -85 rep", false},
		{"- Never discount for rep; volume builds it", false},
		{"- Demand a discount on every single lot", false},
		{"- Repair the radio and report the thief", false},
	}
	for _, tt := range tests {
		if errs := lintRepDiscounts(tt.line, s); (len(errs) > 0) != tt.bad {
			t.Errorf("%q: errors %v, want flagged=%v", tt.line, errs, tt.bad)
		}
	}
}
