package strategy

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// perks.json is docs/re/PERKS.md as data: every starting and reputation perk with its effect
// and a strategic note.
//
//go:embed perks.json
var perksJSON []byte

type perkInfo struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Faction   string `json:"faction"`
	Threshold *int64 `json:"threshold"`
	Polarity  string `json:"polarity"`
	Strategy  string `json:"strategy"`
}

// perkIndex is perks.json keyed by lower-cased id: the game spells starting perks
// "aug_menace" and reputation perks "LLREFERAL".
var perkIndex = func() map[string]perkInfo {
	var doc struct {
		Perks []perkInfo `json:"perks"`
	}
	if err := json.Unmarshal(perksJSON, &doc); err != nil {
		panic("strategy: perks.json: " + err.Error())
	}
	m := make(map[string]perkInfo, len(doc.Perks))
	for _, p := range doc.Perks {
		m[strings.ToLower(p.ID)] = p
	}
	return m
}()

func lookupPerk(id string) (perkInfo, bool) {
	p, ok := perkIndex[strings.ToLower(id)]
	return p, ok
}

var perkFactions = map[string]string{
	"FACTION_LOWER_LEVEL": "Lower Levels", "FACTION_UPPER_LEVEL": "Upper Levels", "FACTION_SECURITY": "Security",
	"FACTION_BLACK_MARKET": "Black Market", "FACTION_REVOLUTION": "Revolution", "FACTION_CARTEL": "Cartel",
}

// activePerks are the run's starting perks and the reputation perks unlocked now: the
// snapshot's perks block, else each faction's unlocked list.
func (s *snapshot) activePerks() (starting, rep []string) {
	starting = s.StartingPerks
	rep = s.RepPerks
	if rep == nil {
		for _, id := range sortedKeys(s.Reputation) {
			rep = append(rep, s.Reputation[id].unlocked()...)
		}
	}
	return starting, rep
}

func (s *snapshot) hasPerk(id string) bool {
	starting, rep := s.activePerks()
	match := func(p string) bool { return strings.EqualFold(p, id) }
	return slices.ContainsFunc(starting, match) || slices.ContainsFunc(rep, match)
}

// renderPerks lists what is in force today, each with its strategic note, so the plan works
// with the run's perks instead of rediscovering them.
func (s *snapshot) renderPerks(b *strings.Builder) {
	starting, rep := s.activePerks()
	if len(starting) == 0 && len(rep) == 0 {
		return
	}
	b.WriteString("ACTIVE PERKS:\n")
	line := func(id, origin string) {
		p, ok := lookupPerk(id)
		if !ok {
			fmt.Fprintf(b, "- %s (%s)\n", id, origin)
			return
		}
		if p.Kind == "reputation" {
			origin = perkFactions[p.Faction]
			if p.Threshold != nil {
				origin += fmt.Sprintf(" %d", *p.Threshold)
			}
		}
		fmt.Fprintf(b, "- %s (%s): %s\n", p.Name, origin, firstSentences(p.Strategy, 220))
	}
	for _, id := range starting {
		line(id, "starting")
	}
	for _, id := range rep {
		line(id, "reputation")
	}
}

// firstSentences cuts s after the last full sentence within max bytes.
func firstSentences(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if i := strings.LastIndex(cut, ". "); i > 0 {
		return cut[:i+1]
	}
	return strings.TrimRight(cut, " ") + "…"
}
