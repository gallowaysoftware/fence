package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

const (
	VerdictConfirmed    = "confirmed"
	VerdictContradicted = "contradicted"
	VerdictChanged      = "version-changed"
	VerdictUnverifiable = "unverifiable"
)

type Verdict struct {
	Verdict  string   `json:"verdict"`
	Method   string   `json:"method,omitempty"`   // Type.Member or table.field the verdict rests on
	Evidence string   `json:"evidence,omitempty"` // what the code says, in our words
	Value    string   `json:"code_value,omitempty"`
	Terms    []string `json:"terms,omitempty"`
	Files    []string `json:"files,omitempty"` // snippet files shown to the model
}

// Corpus is the decompiled game plus the extracted data tables, held in memory as lines.
type Corpus struct {
	Files map[string][]string // display name -> lines
	names []string
}

// LoadCorpus reads every .cs under decompDir and every .json under dataDir.
func LoadCorpus(decompDir, dataDir string) (*Corpus, error) {
	c := &Corpus{Files: map[string][]string{}}
	for _, d := range []struct{ dir, ext, prefix string }{{decompDir, ".cs", ""}, {dataDir, ".json", "gamedata/"}} {
		if d.dir == "" {
			continue
		}
		root, err := filepath.EvalSymlinks(d.dir)
		if err != nil {
			return nil, err
		}
		d.dir = root
		err = filepath.WalkDir(d.dir, func(p string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(p, d.ext) {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(d.dir, p)
			c.Files[d.prefix+rel] = strings.Split(string(b), "\n")
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for n := range c.Files {
		c.names = append(c.names, n)
	}
	sort.Strings(c.names)
	return c, nil
}

// TypeNames lists the decompiled types (file names without extension).
func (c *Corpus) TypeNames() []string {
	var out []string
	for _, n := range c.names {
		if strings.HasSuffix(n, ".cs") {
			out = append(out, strings.TrimSuffix(filepath.Base(n), ".cs"))
		}
	}
	return out
}

type Snippet struct {
	File  string
	Start int // 1-based
	Lines []string
}

// Search ranks files by how many distinct terms they contain (rare terms weigh more)
// and returns windows of context around the hits, within budget characters.
func (c *Corpus) Search(terms []string, budget int) []Snippet {
	var ts []string
	for _, t := range terms {
		if t = strings.ToLower(strings.TrimSpace(t)); len(t) >= 3 && !contains(ts, t) {
			ts = append(ts, t)
		}
	}
	if len(ts) == 0 {
		return nil
	}
	df := map[string]int{}
	type hit struct {
		file  string
		score float64
		lines map[int][]string
	}
	var hits []hit
	for _, name := range c.names {
		lines := c.Files[name]
		h := hit{file: name, lines: map[int][]string{}}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(name), filepath.Ext(name)))
		for i, l := range lines {
			ll := strings.ToLower(l)
			for _, t := range ts {
				if strings.Contains(ll, t) {
					h.lines[i] = append(h.lines[i], t)
				}
			}
		}
		found := map[string]bool{}
		for _, tt := range h.lines {
			for _, t := range tt {
				found[t] = true
			}
		}
		for _, t := range ts {
			if base == t {
				found[t] = true
			}
		}
		if len(found) == 0 {
			continue
		}
		for t := range found {
			df[t]++
		}
		hits = append(hits, h)
	}
	for i := range hits {
		h := &hits[i]
		seen := map[string]bool{}
		for _, tt := range h.lines {
			for _, t := range tt {
				if !seen[t] {
					seen[t] = true
					h.score += 1 / float64(df[t])
				}
			}
		}
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(h.file), filepath.Ext(h.file)))
		for _, t := range ts {
			if base == t {
				h.score += 2
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })

	var out []Snippet
	used := 0
	const ctxLines = 16
	for _, h := range hits {
		if used >= budget || len(out) >= 12 {
			break
		}
		// Prefer lines matching the rarest terms.
		var idx []int
		for i := range h.lines {
			idx = append(idx, i)
		}
		sort.Slice(idx, func(a, b int) bool {
			ra, rb := rarity(h.lines[idx[a]], df), rarity(h.lines[idx[b]], df)
			if ra != rb {
				return ra > rb
			}
			return idx[a] < idx[b]
		})
		lines := c.Files[h.file]
		covered := map[int]bool{}
		perFile := 0
		for _, i := range idx {
			if used >= budget || covered[i] || perFile >= 4 {
				continue
			}
			perFile++
			s, e := max(0, i-ctxLines), min(len(lines), i+ctxLines+1)
			for k := s; k < e; k++ {
				covered[k] = true
			}
			sn := Snippet{File: h.file, Start: s + 1, Lines: lines[s:e]}
			out = append(out, sn)
			used += len(strings.Join(sn.Lines, "\n"))
			if len(out) >= 12 {
				break
			}
		}
	}
	return out
}

func rarity(terms []string, df map[string]int) float64 {
	r := 0.0
	for _, t := range terms {
		r += 1 / float64(df[t])
	}
	return r
}

const termsSystem = `You help check community claims about the game "Probably Stolen" against its decompiled C# source (Unity, Assembly-CSharp) and its extracted data tables (JSON: difficulty, reputation, services, upgrades, items, clients, days, market_events, afterhours, starting_perks, constants).
Propose search terms likely to appear in the code or tables that implement the claim: C# type, field or method names in their likely camelCase/PascalCase form, data-table field names, string ids. Prefer specific names over generic words. You get the list of type names that exist; pick real ones.
You get several numbered claims; answer for each.
Answer with only JSON: {"terms":{"1":["..."],"2":["..."]}} with 4 to 10 terms per claim.`

const verdictSystem = `You check one community claim about the game "Probably Stolen" against excerpts of its decompiled source and extracted data tables (build 26525552, version 1.0.x). Judge only from the excerpts.
Verdicts:
- "confirmed": the excerpts implement the claim (numbers match, allowing rounding).
- "contradicted": the excerpts implement the same rule differently (say what the code does instead).
- "version-changed": the excerpts show the claim was likely true in an older version (e.g. the code has a different value, a removed feature, or the claim is tagged as demo/playtest) — use only with a reason.
- "unverifiable": the excerpts do not cover the claim.
Be strict: a related but different rule is not confirmation. Name the member the verdict rests on as Type.Member (or table.field).
Answer with only JSON: {"verdict":"confirmed|contradicted|version-changed|unverifiable","method":"Type.Member","evidence":"one or two sentences on what the code does","code_value":"the value or rule in code, if relevant"}`

type Verifier struct {
	LLM       llm.Client
	Corpus    *Corpus
	MaxTokens int
	Budget    int // characters of snippets per claim
}

// Verify checks one mechanic cluster; the result is cached by cluster id.
func (v *Verifier) Verify(ctx context.Context, st Store, k Cluster, examples, terms []string) (Verdict, error) {
	var cached Verdict
	if readJSON(st.VerifyPath(k.ID), &cached) == nil {
		return cached, nil
	}
	claimText := k.Statement
	if len(examples) > 0 {
		claimText += "\nAs sources put it:\n" + bullet(examples)
	}
	tr := struct{ Terms []string }{terms}
	snips := v.Corpus.Search(tr.Terms, v.Budget)
	var b strings.Builder
	fileSet := map[string]bool{}
	var files []string
	for _, s := range snips {
		fmt.Fprintf(&b, "--- %s:%d\n%s\n", s.File, s.Start, strings.Join(s.Lines, "\n"))
		if !fileSet[s.File] {
			fileSet[s.File] = true
			files = append(files, s.File)
		}
	}
	vd := Verdict{Verdict: VerdictUnverifiable, Terms: tr.Terms, Files: files}
	if len(snips) > 0 {
		user := "Claim: " + claimText + "\n\nExcerpts:\n" + b.String()
		if _, err := askJSON(ctx, v.LLM, verdictSystem, user, v.MaxTokens, &vd); err != nil {
			return Verdict{}, fmt.Errorf("verdict: %w", err)
		}
		vd.Terms, vd.Files = tr.Terms, files
	}
	switch vd.Verdict {
	case VerdictConfirmed, VerdictContradicted, VerdictChanged, VerdictUnverifiable:
	default:
		vd.Verdict = VerdictUnverifiable
	}
	return vd, writeJSON(st.VerifyPath(k.ID), vd)
}

// Terms asks for search terms for several clusters in one call (the type-name list is
// the bulk of the prompt, so batching saves most of the cost). Missing answers are
// left empty.
func (v *Verifier) Terms(ctx context.Context, ks []Cluster) ([][]string, error) {
	var b strings.Builder
	b.WriteString("Type names: " + strings.Join(v.Corpus.TypeNames(), ", ") + "\n\nClaims:\n")
	for i, k := range ks {
		fmt.Fprintf(&b, "%d. %s\n", i+1, k.Statement)
	}
	var ans struct {
		Terms map[string][]string `json:"terms"`
	}
	if _, err := askJSON(ctx, v.LLM, termsSystem, b.String(), v.MaxTokens, &ans); err != nil {
		return nil, err
	}
	out := make([][]string, len(ks))
	for i := range ks {
		out[i] = ans.Terms[fmt.Sprint(i+1)]
	}
	return out, nil
}

// priorityTopics are verified first: the ways a run is lost, and the loops the owner
// plays (production, water, heat and inspections, the window).
var priorityTopics = map[string]int{
	"contraband & inspections": 3, "heat & stolen": 3, "rent & cash": 3, "endgame/buyout": 2,
	"production": 2, "window/display": 2, "scavenging": 2, "market events & speculation": 2,
	"factions & perks": 1, "selling & haggling": 1,
}

// VerifyOrder ranks mechanic clusters for verification: priority topics and water/loss
// keywords first, then by number of independent sources.
func VerifyOrder(ks []Cluster) []int {
	var idx []int
	for i, k := range ks {
		if k.Kind == "mechanic" {
			idx = append(idx, i)
		}
	}
	score := func(k Cluster) int {
		s := priorityTopics[k.Topic]*10 + k.Origins*4 + k.Quality
		l := strings.ToLower(k.Statement + " " + k.System)
		for _, w := range []string{"water", "arrest", "evict", "-100", "game over", "ends the run", "inspection", "heat", "display"} {
			if strings.Contains(l, w) {
				s += 8
				break
			}
		}
		return s
	}
	sort.SliceStable(idx, func(a, b int) bool { return score(ks[idx[a]]) > score(ks[idx[b]]) })
	return idx
}
