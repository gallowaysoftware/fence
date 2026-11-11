package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
)

// PerkSample draws one starting-perk loadout per (seed, start), the same for every player,
// following docs/re/PERKS.md §5.3: a share with no perks, a share of curated combos, the
// rest drawn under the game's selection rules with rule-changing perks up-weighted. Held-out
// perks and combos never appear unless IncludeHeldOut is set (an evaluation league).
type PerkSample struct {
	Catalogue    string   `json:"catalogue"`
	NoneShare    float64  `json:"none_share"`
	CuratedShare float64  `json:"curated_share"`
	Curated      []string `json:"curated,omitempty"`
	// RuleChanging perks get RuleWeight (default 3); minor-impact perks 0.3; Weights
	// overrides any perk.
	RuleChanging   []string           `json:"rule_changing,omitempty"`
	RuleWeight     float64            `json:"rule_weight,omitempty"`
	Weights        map[string]float64 `json:"weights,omitempty"`
	HeldOut        HeldOut            `json:"held_out"`
	IncludeHeldOut bool               `json:"include_held_out,omitempty"`
	Salt           string             `json:"salt,omitempty"`

	cat *perkCatalogue
}

type HeldOut struct {
	Perks  []string `json:"perks,omitempty"`
	Combos []string `json:"combos,omitempty"`
}

type perkCatalogue struct {
	Selection struct {
		Points    int `json:"points"`
		BaseSlots int `json:"base_slots"`
	} `json:"selection"`
	Perks []catPerk `json:"perks"`
}

type catPerk struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Cost         int      `json:"cost"`
	Incompatible []string `json:"incompatible"`
	ExtraSlots   int      `json:"extra_slots"`
	Impact       string   `json:"impact"`
}

const maxDraws = 10000

func (ps *PerkSample) load() error {
	b, err := os.ReadFile(ps.Catalogue)
	if err != nil {
		return fmt.Errorf("perk_sample catalogue: %w", err)
	}
	var c perkCatalogue
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("perk_sample catalogue %s: %w", ps.Catalogue, err)
	}
	c.Perks = slices.DeleteFunc(c.Perks, func(p catPerk) bool { return p.Kind != "starting" })
	slices.SortFunc(c.Perks, func(a, b catPerk) int { return strings.Compare(a.ID, b.ID) })
	if len(c.Perks) == 0 || c.Selection.Points <= 0 || c.Selection.BaseSlots <= 0 {
		return fmt.Errorf("perk_sample catalogue %s: no starting perks or selection rules", ps.Catalogue)
	}
	ps.cat = &c
	if ps.RuleWeight == 0 {
		ps.RuleWeight = 3
	}
	if ps.NoneShare < 0 || ps.CuratedShare < 0 || ps.NoneShare+ps.CuratedShare > 1 {
		return fmt.Errorf("perk_sample: shares %.2f + %.2f out of [0, 1]", ps.NoneShare, ps.CuratedShare)
	}
	known := func(id string) bool { return slices.ContainsFunc(c.Perks, func(p catPerk) bool { return p.ID == id }) }
	for _, id := range append(append(slices.Clone(ps.RuleChanging), ps.HeldOut.Perks...), mapKeys(ps.Weights)...) {
		if !known(id) {
			return fmt.Errorf("perk_sample: %q is not a starting perk in %s", id, ps.Catalogue)
		}
	}
	for i, combo := range ps.Curated {
		if ps.Curated[i], err = ps.legalSet(strings.Split(combo, "+")); err != nil {
			return fmt.Errorf("perk_sample curated %q: %w", combo, err)
		}
	}
	for i, combo := range ps.HeldOut.Combos {
		if ps.HeldOut.Combos[i], err = ps.legalSet(strings.Split(combo, "+")); err != nil {
			return fmt.Errorf("perk_sample held-out %q: %w", combo, err)
		}
	}
	if ps.CuratedShare > 0 && len(ps.trainCurated()) == 0 {
		return fmt.Errorf("perk_sample: curated_share without curated combos outside the held-out set")
	}
	return nil
}

func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func (ps *PerkSample) perk(id string) (catPerk, bool) {
	i := slices.IndexFunc(ps.cat.Perks, func(p catPerk) bool { return p.ID == id })
	if i < 0 {
		return catPerk{}, false
	}
	return ps.cat.Perks[i], true
}

// legalSet checks ids against the selection rules and returns the canonical set id.
func (ps *PerkSample) legalSet(ids []string) (string, error) {
	cost, slots := 0, ps.cat.Selection.BaseSlots
	for _, id := range ids {
		p, ok := ps.perk(id)
		if !ok {
			return "", fmt.Errorf("unknown perk %q", id)
		}
		cost += p.Cost
		slots += p.ExtraSlots
		for _, other := range ids {
			if slices.Contains(p.Incompatible, other) {
				return "", fmt.Errorf("%s and %s are incompatible", id, other)
			}
		}
	}
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	switch {
	case len(slices.Compact(slices.Clone(sorted))) != len(sorted):
		return "", fmt.Errorf("a perk appears twice")
	case cost > ps.cat.Selection.Points:
		return "", fmt.Errorf("costs %d points, over %d", cost, ps.cat.Selection.Points)
	case len(ids) > slots:
		return "", fmt.Errorf("%d perks in %d slots", len(ids), slots)
	}
	return strings.Join(sorted, "+"), nil
}

func (ps *PerkSample) heldOut(set string) bool {
	if set == NoPerks {
		return false
	}
	if slices.Contains(ps.HeldOut.Combos, set) {
		return true
	}
	for _, id := range strings.Split(set, "+") {
		if slices.Contains(ps.HeldOut.Perks, id) {
			return true
		}
	}
	return false
}

func (ps *PerkSample) trainCurated() []string {
	if ps.IncludeHeldOut {
		return ps.Curated
	}
	return slices.DeleteFunc(slices.Clone(ps.Curated), ps.heldOut)
}

func (ps *PerkSample) weight(p catPerk) float64 {
	if w, ok := ps.Weights[p.ID]; ok {
		return w
	}
	if slices.Contains(ps.RuleChanging, p.ID) {
		return ps.RuleWeight
	}
	if p.Impact == "minor" {
		return 0.3
	}
	return 1
}

// Draw is the loadout for one (seed, start): reproducible from the spec alone.
func (ps *PerkSample) Draw(seed int, start string) string {
	rng := rand.New(rand.NewPCG(seedFor("perks", ps.Salt, strconv.Itoa(seed), start), 0))
	u := rng.Float64()
	switch {
	case u < ps.NoneShare:
		return NoPerks
	case u < ps.NoneShare+ps.CuratedShare:
		c := ps.trainCurated()
		return c[rng.IntN(len(c))]
	}
	for range maxDraws {
		if set, ok := ps.drawLoadout(rng); ok {
			return set
		}
	}
	return NoPerks
}

// drawLoadout picks 1–4 perks by weight without replacement and keeps the set only if the
// rules allow it (rejection sampling keeps the rule distribution exact).
func (ps *PerkSample) drawLoadout(rng *rand.Rand) (string, bool) {
	var pool []catPerk
	for _, p := range ps.cat.Perks {
		if ps.IncludeHeldOut || !slices.Contains(ps.HeldOut.Perks, p.ID) {
			pool = append(pool, p)
		}
	}
	n := 1 + rng.IntN(ps.cat.Selection.BaseSlots)
	var ids []string
	for len(ids) < n && len(pool) > 0 {
		total := 0.0
		for _, p := range pool {
			total += ps.weight(p)
		}
		x := rng.Float64() * total
		i := 0
		for ; i < len(pool)-1; i++ {
			if x -= ps.weight(pool[i]); x < 0 {
				break
			}
		}
		ids = append(ids, pool[i].ID)
		pool = slices.Delete(pool, i, i+1)
	}
	set, err := ps.legalSet(ids)
	if err != nil || (!ps.IncludeHeldOut && ps.heldOut(set)) {
		return "", false
	}
	return set, true
}
