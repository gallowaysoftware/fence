package strategy

import (
	"fmt"
	"slices"
	"strings"
)

// Window rules from docs/re/WINDOW.md.
const (
	windowMinAttractiveness = 100
	tier2Value              = 145
	tier3Value              = 350
	tier3Household          = 300
	tier3MinDay             = 14
	attractivenessTier2     = 400
)

// majorTypes are the only item types that draw window buyers (TypeHelper.MAJOR_TYPES_LIST).
var majorTypes = []string{"food", "medical", "material", "household_good", "substance", "weapon", "water", "armor", "tool", "module", "ammunition"}

func majorType(t string) (string, bool) {
	switch t {
	case "water_ration":
		t = "water"
	case "energy_cell":
		t = "material"
	}
	return t, slices.Contains(majorTypes, t)
}

// windowBuyers is how many types draw a guaranteed buyer at opening: 1, +1 with the Buy
// Referral service, +1 per showcase upgrade. Vandalism (-1) is not in the snapshot.
func (s *snapshot) windowBuyers() int {
	n := 1
	if slices.ContainsFunc(s.Services, func(v service) bool { return v.inUse() && strings.EqualFold(v.ID, "BUY_REFERAL") }) {
		n++
	}
	for _, u := range s.UpgradesOwned {
		if strings.Contains(strings.ToLower(u), "showcase") {
			n++
		}
	}
	return n
}

type typeValue struct {
	typ   string
	value int64
	items []string
}

// valuesByType sums estimated values of the given items per major type; an item with
// several types counts toward each.
func valuesByType(items []item) []typeValue {
	byType := map[string]*typeValue{}
	for _, it := range items {
		v := int64(0)
		if it.EstimatedValue != nil {
			v = *it.EstimatedValue
		}
		for _, t := range it.types() {
			mt, ok := majorType(t)
			if !ok {
				continue
			}
			tv := byType[mt]
			if tv == nil {
				tv = &typeValue{typ: mt}
				byType[mt] = tv
			}
			tv.value += v
			name := itemName(it)
			if it.UID != nil {
				name += fmt.Sprintf(" #%d", *it.UID)
			}
			tv.items = append(tv.items, name)
		}
	}
	out := make([]typeValue, 0, len(byType))
	for _, tv := range byType {
		out = append(out, *tv)
	}
	slices.SortFunc(out, func(a, b typeValue) int {
		if a.value != b.value {
			return int(b.value - a.value)
		}
		return strings.Compare(a.typ, b.typ)
	})
	return out
}

func (s *snapshot) tier(t string, value int64, day int) int {
	t3 := int64(tier3Value)
	if t == "household_good" {
		t3 = tier3Household
	}
	switch {
	case value >= t3 && day >= tier3MinDay:
		return 3
	case value >= tier2Value || (s.Attractiveness != nil && *s.Attractiveness >= attractivenessTier2) || t == "tool" || t == "module":
		return 2
	}
	return 1
}

// renderWindowMath states what the window can do today: how many guaranteed buyers, what
// each type of clean stock is worth if displayed and which buyer tier that reaches, and
// the discerning shopper's chance.
func (s *snapshot) renderWindowMath(b *strings.Builder, day int) {
	if s.Inventory == nil {
		return
	}
	if a := s.Attractiveness; a != nil && *a < windowMinAttractiveness {
		fmt.Fprintf(b, "Window buyers: none today (attractiveness %d is below %d).\n", *a, windowMinAttractiveness)
		return
	}
	n := s.windowBuyers()
	fmt.Fprintf(b, "Window buyers: at opening the top %d item type(s) by displayed value each bring one guaranteed buyer; only %s count.\n",
		n, strings.Join(majorTypes, ", "))
	var shown, clean []item
	for _, it := range s.Inventory {
		if it.Where == "showcase" {
			shown = append(shown, it)
		}
		if it.displayable() {
			clean = append(clean, it)
		}
	}
	if tv := valuesByType(shown); len(tv) > 0 {
		var parts []string
		for i, v := range tv {
			p := fmt.Sprintf("%s %d", v.typ, v.value)
			if i < n {
				p += fmt.Sprintf(" (draws a tier %d buyer)", s.tier(v.typ, v.value, day))
			} else {
				p += " (no buyer: below the top " + fmt.Sprint(n) + ")"
			}
			parts = append(parts, p)
		}
		fmt.Fprintf(b, "Displayed now by type: %s.\n", strings.Join(parts, "; "))
	}
	if tv := valuesByType(clean); len(tv) > 0 {
		var parts []string
		for i, v := range tv {
			if i == 6 {
				break
			}
			names := v.items
			if len(names) > 4 {
				names = append(names[:4:4], fmt.Sprintf("%d more", len(v.items)-4))
			}
			parts = append(parts, fmt.Sprintf("%s %d -> tier %d (%s)", v.typ, v.value, s.tier(v.typ, v.value, day), strings.Join(names, ", ")))
		}
		fmt.Fprintf(b, "Clean stock you could display, by type (tier 2 from %d, tier 3 from %d, household %d, day %d+): %s.\n",
			tier2Value, tier3Value, tier3Household, tier3MinDay, strings.Join(parts, "; "))
	}
	s.renderDiscerning(b, day, clean)
}

func (s *snapshot) renderDiscerning(b *strings.Builder, day int, clean []item) {
	var lux []string
	for _, it := range clean {
		if slices.Contains(it.types(), "luxury_item") {
			name := itemName(it)
			if it.UID != nil {
				name += fmt.Sprintf(" #%d", *it.UID)
			}
			lux = append(lux, name)
		}
	}
	ul, ok := s.Reputation["upper_levels"]
	perk := ok && slices.ContainsFunc(ul.unlocked(), func(p string) bool {
		return strings.EqualFold(p, "DiscerningTaste") || strings.EqualFold(p, "UL_DISCERNING_TASTE")
	})
	if !perk && ok && ul.Value != nil && ul.unlocked() == nil {
		perk = *ul.Value >= 0
	}
	if !perk || len(lux) == 0 || day < tier3MinDay {
		return
	}
	chance := int64(1)
	if ul.Value != nil && *ul.Value > 0 {
		chance += int64(*ul.Value) / 5
	}
	if c := s.Clientele; c != nil && c.Discerning != nil && c.Discerning.Chance != nil {
		chance = *c.Discerning.Chance
	}
	fmt.Fprintf(b, "Discerning shopper (Upper Levels, budget about %d+, wants a luxury item): %d%% if a luxury item is displayed; you have %s.\n",
		750+day, chance, strings.Join(lux, ", "))
}
