package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// GameSystem is one entry of the systems catalogue: a loop or mechanic players talk
// about, how it connects to the others, and how much of it this repo already covers.
type GameSystem struct {
	Name        string       `json:"name"`
	Aliases     []string     `json:"aliases,omitempty"`
	Claims      int          `json:"claims"`
	Origins     int          `json:"origins"`
	Description string       `json:"description,omitempty"`
	PlayerUse   string       `json:"player_use,omitempty"`
	Connections []Connection `json:"connections,omitempty"`
	Types       []string     `json:"types,omitempty"`
	Coverage    Coverage     `json:"coverage"`
}

type Connection struct {
	To  string `json:"to"`
	Via string `json:"via"`
}

type Coverage struct {
	REDocs     []string `json:"re_docs,omitempty"`
	Hooks      []string `json:"hooks,omitempty"` // mod features with a hook on this system
	Strategist bool     `json:"strategist"`
}

type sysCount struct {
	claims  int
	origins map[string]bool
	sample  []Claim
}

// CountSystems tallies the raw system labels of the claims.
func CountSystems(claims []Claim, srcs map[string]Source) map[string]*sysCount {
	out := map[string]*sysCount{}
	for _, c := range claims {
		if c.System == "" {
			continue
		}
		sc := out[c.System]
		if sc == nil {
			sc = &sysCount{origins: map[string]bool{}}
			out[c.System] = sc
		}
		sc.claims++
		o := c.SourceID
		if s, ok := srcs[c.SourceID]; ok {
			o = s.Origin()
		}
		sc.origins[o] = true
		sc.sample = append(sc.sample, c)
	}
	return out
}

const normSystemsSystem = `You normalise labels for game systems of "Probably Stolen" that were written by different annotators. Merge labels that name the same system or loop (e.g. "wine", "winemaking", "bloomberry wine" -> "wine"; "scavenging", "dumping ground" -> "dumping ground scavenging"). Keep distinct production chains, machines, locations, services and story arcs separate. Use short lowercase canonical names. Every input label must appear in exactly one system's aliases.
Answer with only JSON: {"systems":[{"name":"...","aliases":["label", "..."]}]}`

const describeSystemSystem = `You write one entry of a catalogue of the game systems and loops of "Probably Stolen", from claims community players made about it. Write in your own words, concise and concrete.
- description: one line, what the system is.
- player_use: one or two sentences, how players use it and why (numbers welcome).
- connections: how it feeds or depends on other systems, as {"to": other system, "via": the item/resource/effect that links them}, e.g. {"to":"wine","via":"ripe berries"}.
- types: up to 4 names from the provided list of decompiled C# types that most likely implement it (exact spelling; empty if none fits).
Answer with only JSON: {"description":"...","player_use":"...","connections":[{"to":"...","via":"..."}],"types":["..."]}`

type SystemCataloguer struct {
	LLM       llm.Client
	Corpus    *Corpus
	MaxTokens int
	Max       int // catalogue at most this many systems
}

// Catalogue normalises the raw labels, then describes each system.
func (sc *SystemCataloguer) Catalogue(ctx context.Context, claims []Claim, srcs map[string]Source, cachePath string, logf func(string, ...any)) ([]GameSystem, error) {
	var cached []GameSystem
	cache := map[string]GameSystem{}
	if readJSON(cachePath, &cached) == nil {
		for _, g := range cached {
			cache[g.Name] = g
		}
	}
	counts := CountSystems(claims, srcs)
	labels := make([]string, 0, len(counts))
	for l := range counts {
		labels = append(labels, l)
	}
	sort.Slice(labels, func(i, j int) bool { return counts[labels[i]].claims > counts[labels[j]].claims })

	// One-off labels are mostly synonyms of common ones; leaving them out keeps the
	// normalisation answer within one completion.
	var common []string
	for _, l := range labels {
		if counts[l].claims >= 2 && len(common) < 250 {
			common = append(common, l)
		}
	}
	var groups []GameSystem
	normPath := strings.TrimSuffix(cachePath, ".json") + ".norm.json"
	if readJSON(normPath, &groups) != nil {
		var b strings.Builder
		for _, l := range common {
			fmt.Fprintf(&b, "%s (%d)\n", l, counts[l].claims)
		}
		var ans struct {
			Systems []GameSystem `json:"systems"`
		}
		if _, err := askJSON(ctx, sc.LLM, normSystemsSystem, b.String(), sc.MaxTokens, &ans); err != nil {
			return nil, fmt.Errorf("normalise systems: %w", err)
		}
		groups = NormaliseSystems(common, ans.Systems)
		if err := writeJSON(normPath, groups); err != nil {
			return nil, err
		}
	}
	for i := range groups {
		g := &groups[i]
		origins := map[string]bool{}
		g.Claims = 0
		for _, a := range g.Aliases {
			if c := counts[a]; c != nil {
				g.Claims += c.claims
				for o := range c.origins {
					origins[o] = true
				}
			}
		}
		g.Origins = len(origins)
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Origins != groups[j].Origins {
			return groups[i].Origins > groups[j].Origins
		}
		return groups[i].Claims > groups[j].Claims
	})
	if sc.Max > 0 && len(groups) > sc.Max {
		groups = groups[:sc.Max]
	}
	types := sc.Corpus.TypeNames()
	typeSet := map[string]bool{}
	for _, t := range types {
		typeSet[t] = true
	}
	for i := range groups {
		g := &groups[i]
		if c, ok := cache[g.Name]; ok && c.Description != "" {
			g.Description, g.PlayerUse, g.Connections, g.Types = c.Description, c.PlayerUse, c.Connections, c.Types
			continue
		}
		var sample []string
		seenSrc := map[string]int{}
		for _, a := range g.Aliases {
			if c := counts[a]; c != nil {
				for _, cl := range c.sample {
					if seenSrc[cl.SourceID] < 3 && len(sample) < 40 {
						seenSrc[cl.SourceID]++
						sample = append(sample, cl.Claim)
					}
				}
			}
		}
		user := "Type names: " + strings.Join(types, ", ") + "\n\nSystem: " + g.Name + "\nClaims:\n" + bullet(sample)
		var ans GameSystem
		if _, err := askJSON(ctx, sc.LLM, describeSystemSystem, user, sc.MaxTokens, &ans); err != nil {
			logf("describe %s: %v", g.Name, err)
			continue
		}
		g.Description, g.PlayerUse, g.Connections = ans.Description, ans.PlayerUse, ans.Connections
		for _, t := range ans.Types {
			if typeSet[t] && !contains(g.Types, t) {
				g.Types = append(g.Types, t)
			}
		}
		logf("system %s: %d claims, %d origins, types %v", g.Name, g.Claims, g.Origins, g.Types)
		cached = append(cached, *g)
		if err := writeJSON(cachePath, cached); err != nil {
			return nil, err
		}
	}
	return groups, nil
}

// NormaliseSystems makes the model's grouping a partition of labels: each label lands in
// the first system that names it, unknown labels are dropped, and unclaimed labels
// become their own system.
func NormaliseSystems(labels []string, proposed []GameSystem) []GameSystem {
	known := map[string]bool{}
	for _, l := range labels {
		known[l] = true
	}
	taken := map[string]bool{}
	var out []GameSystem
	idx := map[string]int{}
	for _, p := range proposed {
		name := strings.ToLower(collapseSpace(p.Name))
		if name == "" {
			continue
		}
		var al []string
		for _, a := range append([]string{name}, p.Aliases...) {
			a = strings.ToLower(collapseSpace(a))
			if known[a] && !taken[a] {
				taken[a] = true
				al = append(al, a)
			}
		}
		if len(al) == 0 {
			continue
		}
		if i, ok := idx[name]; ok {
			out[i].Aliases = append(out[i].Aliases, al...)
			continue
		}
		idx[name] = len(out)
		out = append(out, GameSystem{Name: name, Aliases: al})
	}
	for _, l := range labels {
		if !taken[l] {
			out = append(out, GameSystem{Name: l, Aliases: []string{l}})
		}
	}
	return out
}

// genericTypes are touched by nearly every system, so a hook on them says nothing about
// coverage of one system.
var genericTypes = map[string]bool{"PlayerStore": true, "StoreStation": true, "GameManager": true, "ModHook": true, "RunStats": true, "StoreUIManager": true, "DialogUIManager": true}

// AssessCoverage fills Coverage from the repo: docs/re mentions, hook manifest entries,
// and the strategist's source. It is a text match, so it errs towards "mentioned".
func AssessCoverage(repo string, systems []GameSystem) error {
	reDocs, _ := filepath.Glob(filepath.Join(repo, "docs/re/*.md"))
	docText := map[string]string{}
	for _, d := range reDocs {
		b, err := os.ReadFile(d)
		if err != nil {
			return err
		}
		docText[filepath.Base(d)] = strings.ToLower(string(b))
	}
	type hook struct {
		ID, Type, Member, Why string
		Features              []string
	}
	var hooks []hook
	for _, f := range []string{"compat/hooks.json", "compat/hooks.proposed.json"} {
		var m struct {
			Hooks []hook `json:"hooks"`
		}
		if b, err := os.ReadFile(filepath.Join(repo, f)); err == nil {
			if err := json.Unmarshal(b, &m); err != nil {
				return fmt.Errorf("%s: %w", f, err)
			}
			hooks = append(hooks, m.Hooks...)
		}
	}
	var strat strings.Builder
	goFiles, _ := filepath.Glob(filepath.Join(repo, "internal/strategy/*.go"))
	for _, f := range goFiles {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		strat.Write(b)
	}
	stratText := strings.ToLower(strat.String())

	for i := range systems {
		s := &systems[i]
		var pats []*regexp.Regexp
		for _, a := range append([]string{s.Name}, s.Aliases...) {
			if len(a) >= 4 {
				pats = append(pats, regexp.MustCompile(`\b`+regexp.QuoteMeta(strings.ToLower(a))+`\b`))
			}
		}
		for _, t := range s.Types {
			if !genericTypes[t] {
				pats = append(pats, regexp.MustCompile(`\b`+regexp.QuoteMeta(strings.ToLower(t))+`\b`))
			}
		}
		match := func(text string) bool {
			for _, p := range pats {
				if p.MatchString(text) {
					return true
				}
			}
			return false
		}
		s.Coverage = Coverage{}
		names := make([]string, 0, len(docText))
		for n := range docText {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			if match(docText[n]) {
				s.Coverage.REDocs = append(s.Coverage.REDocs, n)
			}
		}
		feat := map[string]bool{}
		for _, h := range hooks {
			typeHit := contains(s.Types, h.Type) && !genericTypes[h.Type]
			if typeHit || match(strings.ToLower(h.ID+" "+h.Member+" "+h.Why)) {
				for _, f := range h.Features {
					feat[f] = true
				}
			}
		}
		for f := range feat {
			s.Coverage.Hooks = append(s.Coverage.Hooks, f)
		}
		sort.Strings(s.Coverage.Hooks)
		s.Coverage.Strategist = match(stratText)
	}
	return nil
}
