package knowledge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// Confidence tags, strongest first.
const (
	ConfVerified  = "verified-in-code"
	ConfCodeWrong = "contradicted-by-code"
	ConfContested = "contested"
	ConfOwner     = "owner-play"
	ConfMany      = "many-sources"
	ConfSingle    = "single-source"
)

type ContraRef struct {
	ID   string `json:"id"`
	Note string `json:"note,omitempty"`
}

type Cluster struct {
	ID          string      `json:"id"`
	Topic       string      `json:"topic"`
	Kind        string      `json:"kind"`
	System      string      `json:"system,omitempty"`
	Statement   string      `json:"statement"`
	Members     []string    `json:"members"`
	Sources     []string    `json:"sources"`
	Origins     int         `json:"origins"` // independent voices
	Quality     int         `json:"quality"` // best source tier
	Conditions  []string    `json:"conditions,omitempty"`
	Contradicts []ContraRef `json:"contradicts,omitempty"`
	Verdict     *Verdict    `json:"verdict,omitempty"`
	Confidence  string      `json:"confidence"`
}

// Group is the model's answer for one batch: indexes into the batch, 1-based as shown
// in the prompt.
type Group struct {
	Members     []int  `json:"members"`
	Statement   string `json:"statement"`
	Contradicts []int  `json:"contradicts,omitempty"` // other groups in the same answer, 1-based
	Note        string `json:"note,omitempty"`
}

// Partition is a clustering of n units: Sets[i] lists unit indexes (0-based).
type Partition struct {
	Sets       [][]int
	Statements []string
	Contra     []contraPair
}

type contraPair struct {
	A, B int // indexes into Sets
	Note string
}

// MergeGroups turns the model's groups into a partition of n units. A unit claimed by
// two groups stays in the first; a unit no group mentions becomes its own set with its
// own text; out-of-range indexes and empty groups are ignored. Contradiction links that
// point at a dropped or the same group are discarded.
func MergeGroups(texts []string, groups []Group) Partition {
	n := len(texts)
	owner := make([]int, n)
	for i := range owner {
		owner[i] = -1
	}
	var p Partition
	groupSet := make([]int, len(groups)) // group index -> set index, -1 if dropped
	for gi, g := range groups {
		groupSet[gi] = -1
		var set []int
		for _, m := range g.Members {
			u := m - 1
			if u < 0 || u >= n || owner[u] >= 0 {
				continue
			}
			owner[u] = len(p.Sets)
			set = append(set, u)
		}
		if len(set) == 0 {
			continue
		}
		sort.Ints(set)
		st := collapseSpace(g.Statement)
		if st == "" {
			st = texts[set[0]]
		}
		groupSet[gi] = len(p.Sets)
		p.Sets = append(p.Sets, set)
		p.Statements = append(p.Statements, st)
	}
	for u := 0; u < n; u++ {
		if owner[u] < 0 {
			owner[u] = len(p.Sets)
			p.Sets = append(p.Sets, []int{u})
			p.Statements = append(p.Statements, texts[u])
		}
	}
	seen := map[[2]int]bool{}
	for gi, g := range groups {
		a := groupSet[gi]
		if a < 0 {
			continue
		}
		for _, o := range g.Contradicts {
			if o < 1 || o > len(groups) {
				continue
			}
			b := groupSet[o-1]
			if b < 0 || b == a {
				continue
			}
			k := [2]int{min(a, b), max(a, b)}
			if seen[k] {
				continue
			}
			seen[k] = true
			p.Contra = append(p.Contra, contraPair{A: k[0], B: k[1], Note: g.Note})
		}
	}
	return p
}

// Compose lifts a partition of the outer sets' statements back onto the inner units.
func Compose(inner, outer Partition) Partition {
	var p Partition
	innerToFinal := make([]int, len(inner.Sets))
	for oi, oset := range outer.Sets {
		var units []int
		for _, is := range oset {
			units = append(units, inner.Sets[is]...)
			innerToFinal[is] = oi
		}
		sort.Ints(units)
		p.Sets = append(p.Sets, units)
		p.Statements = append(p.Statements, outer.Statements[oi])
	}
	seen := map[[2]int]bool{}
	addC := func(a, b int, note string) {
		if a == b {
			return
		}
		k := [2]int{min(a, b), max(a, b)}
		if !seen[k] {
			seen[k] = true
			p.Contra = append(p.Contra, contraPair{A: k[0], B: k[1], Note: note})
		}
	}
	for _, c := range inner.Contra {
		addC(innerToFinal[c.A], innerToFinal[c.B], c.Note)
	}
	for _, c := range outer.Contra {
		addC(c.A, c.B, c.Note)
	}
	return p
}

const clusterSystem = `You merge duplicate claims about the game "Probably Stolen". You get a numbered list of claims on one topic. Group claims that state the same fact or the same advice (even with different wording or slightly different numbers that clearly refer to the same thing). Do not merge claims that are merely related.

For each group write one canonical statement in plain English, keeping concrete numbers. When claims in a group disagree on a number, state the range and add a note.
Flag contradictions: if group A and group B make incompatible statements (different values for the same rule, or opposite advice for the same situation), list B's group number in A's "contradicts" and explain in "note".

Every claim number must appear in exactly one group; a claim with no duplicate is a group of one.
Answer with only JSON: {"groups":[{"members":[1,4],"statement":"...","contradicts":[],"note":""}]}`

// Clusterer merges claims topic by topic.
type Clusterer struct {
	LLM       llm.Client
	Batch     int
	MaxTokens int
	Workers   int
}

func (c *Clusterer) partition(ctx context.Context, topic string, texts []string, depth int) (Partition, error) {
	if len(texts) <= 1 {
		return MergeGroups(texts, nil), nil
	}
	if len(texts) <= c.Batch {
		var b strings.Builder
		fmt.Fprintf(&b, "Topic: %s\n\n", topic)
		for i, t := range texts {
			fmt.Fprintf(&b, "%d. %s\n", i+1, t)
		}
		var ans struct {
			Groups []Group `json:"groups"`
		}
		if _, err := askJSON(ctx, c.LLM, clusterSystem, b.String(), c.MaxTokens, &ans); err != nil {
			return Partition{}, err
		}
		return MergeGroups(texts, ans.Groups), nil
	}
	var inner Partition
	for start := 0; start < len(texts); start += c.Batch {
		end := min(start+c.Batch, len(texts))
		p, err := c.partition(ctx, topic, texts[start:end], depth+1)
		if err != nil {
			return Partition{}, err
		}
		off := len(inner.Sets)
		for i, s := range p.Sets {
			shifted := make([]int, len(s))
			for k, u := range s {
				shifted[k] = u + start
			}
			inner.Sets = append(inner.Sets, shifted)
			inner.Statements = append(inner.Statements, p.Statements[i])
		}
		for _, cp := range p.Contra {
			inner.Contra = append(inner.Contra, contraPair{A: cp.A + off, B: cp.B + off, Note: cp.Note})
		}
	}
	if depth > 0 {
		return inner, nil
	}
	// One more pass over the batch statements catches duplicates split across batches;
	// sorting puts near-identical statements next to each other when they still need
	// more than one batch.
	perm := make([]int, len(inner.Statements))
	for i := range perm {
		perm[i] = i
	}
	sort.SliceStable(perm, func(a, b int) bool {
		return strings.ToLower(inner.Statements[perm[a]]) < strings.ToLower(inner.Statements[perm[b]])
	})
	sorted := make([]string, len(perm))
	for i, p := range perm {
		sorted[i] = inner.Statements[p]
	}
	outer, err := c.partition(ctx, topic, sorted, depth+1)
	if err != nil {
		return Partition{}, err
	}
	for _, set := range outer.Sets {
		for k, u := range set {
			set[k] = perm[u]
		}
		sort.Ints(set)
	}
	return Compose(inner, outer), nil
}

// ClusterAll groups claims per topic and builds clusters with source statistics.
func (c *Clusterer) ClusterAll(ctx context.Context, claims []Claim, srcs map[string]Source, logf func(string, ...any)) ([]Cluster, error) {
	byTopic := map[string][]Claim{}
	for _, cl := range claims {
		byTopic[cl.Topic] = append(byTopic[cl.Topic], cl)
	}
	type result struct {
		topic string
		cs    []Cluster
		err   error
	}
	results := make(chan result)
	sem := make(chan struct{}, max(1, c.Workers))
	var wg sync.WaitGroup
	for topic, cls := range byTopic {
		sort.Slice(cls, func(i, j int) bool {
			if cls[i].System != cls[j].System {
				return cls[i].System < cls[j].System
			}
			return strings.ToLower(cls[i].Claim) < strings.ToLower(cls[j].Claim)
		})
		wg.Add(1)
		go func(topic string, cls []Claim) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			texts := make([]string, len(cls))
			for i, cl := range cls {
				texts[i] = cl.Claim
			}
			p, err := c.partition(ctx, topic, texts, 0)
			if err != nil {
				results <- result{topic: topic, err: err}
				return
			}
			results <- result{topic: topic, cs: BuildClusters(topic, cls, p, srcs)}
		}(topic, cls)
	}
	go func() { wg.Wait(); close(results) }()
	var all []Cluster
	var firstErr error
	for r := range results {
		if r.err != nil {
			logf("cluster %s: %v", r.topic, r.err)
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		logf("cluster %s: %d clusters", r.topic, len(r.cs))
		all = append(all, r.cs...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Topic != all[j].Topic {
			return all[i].Topic < all[j].Topic
		}
		if all[i].Origins != all[j].Origins {
			return all[i].Origins > all[j].Origins
		}
		return all[i].ID < all[j].ID
	})
	return all, firstErr
}

// BuildClusters turns a partition of one topic's claims into clusters.
func BuildClusters(topic string, cls []Claim, p Partition, srcs map[string]Source) []Cluster {
	out := make([]Cluster, len(p.Sets))
	for i, set := range p.Sets {
		k := Cluster{Topic: topic, Statement: p.Statements[i]}
		kinds := map[string]int{}
		systems := map[string]int{}
		srcSeen, originSeen, condSeen := map[string]bool{}, map[string]bool{}, map[string]bool{}
		for _, u := range set {
			cl := cls[u]
			k.Members = append(k.Members, cl.ID)
			kinds[cl.Kind]++
			if cl.System != "" {
				systems[cl.System]++
			}
			if !srcSeen[cl.SourceID] {
				srcSeen[cl.SourceID] = true
				k.Sources = append(k.Sources, cl.SourceID)
			}
			s, ok := srcs[cl.SourceID]
			origin := cl.SourceID
			if ok {
				origin = s.Origin()
				k.Quality = max(k.Quality, s.Quality)
			}
			originSeen[origin] = true
			if cond := condString(cl.Conditions); cond != "" && !condSeen[cond] {
				condSeen[cond] = true
				k.Conditions = append(k.Conditions, cond)
			}
		}
		sort.Strings(k.Members)
		k.Origins = len(originSeen)
		k.Kind = majority(kinds, Kinds)
		k.System = majority(systems, nil)
		h := sha256.Sum256([]byte(strings.Join(k.Members, ",")))
		k.ID = "k" + hex.EncodeToString(h[:6])
		out[i] = k
	}
	for _, cp := range p.Contra {
		out[cp.A].Contradicts = append(out[cp.A].Contradicts, ContraRef{ID: out[cp.B].ID, Note: cp.Note})
		out[cp.B].Contradicts = append(out[cp.B].Contradicts, ContraRef{ID: out[cp.A].ID, Note: cp.Note})
	}
	for i := range out {
		out[i].Confidence = Confidence(out[i])
	}
	return out
}

// Confidence ranks a cluster: code beats community, the owner's play beats a crowd,
// a crowd beats one voice.
func Confidence(k Cluster) string {
	if k.Verdict != nil {
		switch k.Verdict.Verdict {
		case VerdictConfirmed:
			return ConfVerified
		case VerdictContradicted:
			return ConfCodeWrong
		}
	}
	// Model-flagged contradictions are not used here: in a spot check of 24 flagged pairs
	// only one was a real disagreement. They stay on the cluster as review leads, and
	// "contested" is assigned when a person curates the primer.
	switch {
	case k.Quality >= QualityOwner:
		return ConfOwner
	case k.Origins >= 2:
		return ConfMany
	}
	return ConfSingle
}

// majority picks the most common key; ties go to the earlier entry of prefer, then
// alphabetical order.
func majority(counts map[string]int, prefer []string) string {
	best, bestN := "", 0
	rank := func(s string) int {
		for i, p := range prefer {
			if p == s {
				return i
			}
		}
		return len(prefer)
	}
	for k, n := range counts {
		if n > bestN || n == bestN && (rank(k) < rank(best) || rank(k) == rank(best) && k < best) {
			best, bestN = k, n
		}
	}
	return best
}

func condString(c Conditions) string {
	var parts []string
	for _, s := range []string{c.Days, c.Difficulty, c.Start} {
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, ", ")
}

const contradictSystem = `You find contradictions between statements about the game "Probably Stolen". You get a numbered list of statements on one topic, each already deduplicated. Report pairs that cannot both be true: different numbers for the same rule, the same mechanic described incompatibly, or opposite advice for the same situation. Statements that are merely about different things, or that add detail, are NOT contradictions. Most lists have few or none.
Answer with only JSON: {"pairs":[{"a":3,"b":17,"note":"one line on what disagrees"}]}`

// Contradictions re-derives contradiction links between clusters, topic by topic, from
// their statements alone. The clustering pass also flags them, but there the model
// mixes up claim and group numbers; a dedicated pass over numbered statements does not.
func (c *Clusterer) Contradictions(ctx context.Context, ks []Cluster, logf func(string, ...any)) error {
	byTopic := map[string][]int{}
	for i := range ks {
		ks[i].Contradicts = nil
		if ks[i].Kind != "opinion" {
			byTopic[ks[i].Topic] = append(byTopic[ks[i].Topic], i)
		}
	}
	batch := max(c.Batch, 150)
	for _, topic := range Topics {
		idx := byTopic[topic]
		sort.SliceStable(idx, func(a, b int) bool {
			if ks[idx[a]].System != ks[idx[b]].System {
				return ks[idx[a]].System < ks[idx[b]].System
			}
			return idx[a] < idx[b]
		})
		found := 0
		for start := 0; start < len(idx); start += batch {
			part := idx[start:min(start+batch, len(idx))]
			if len(part) < 2 {
				continue
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Topic: %s\n\n", topic)
			for i, k := range part {
				fmt.Fprintf(&b, "%d. %s\n", i+1, ks[k].Statement)
			}
			var ans struct {
				Pairs []struct {
					A, B int
					Note string
				} `json:"pairs"`
			}
			if _, err := askJSON(ctx, c.LLM, contradictSystem, b.String(), c.MaxTokens, &ans); err != nil {
				return fmt.Errorf("contradictions %s: %w", topic, err)
			}
			for _, p := range ans.Pairs {
				if p.A < 1 || p.B < 1 || p.A > len(part) || p.B > len(part) || p.A == p.B {
					continue
				}
				a, bb := part[p.A-1], part[p.B-1]
				ks[a].Contradicts = append(ks[a].Contradicts, ContraRef{ID: ks[bb].ID, Note: p.Note})
				ks[bb].Contradicts = append(ks[bb].Contradicts, ContraRef{ID: ks[a].ID, Note: p.Note})
				found++
			}
		}
		logf("contradictions %s: %d pairs", topic, found)
	}
	for i := range ks {
		ks[i].Confidence = Confidence(ks[i])
	}
	return nil
}
