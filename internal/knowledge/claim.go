// Package knowledge harvests what experienced players say about Probably Stolen, checks
// the mechanical claims against the decompiled game, and distils a primer for the
// strategist. Bulk reading is done by an OpenAI-compatible model; this package only
// moves text, validates what comes back and merges it.
package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// Topics is the closed set a claim must be filed under.
var Topics = []string{
	"window/display",
	"stock & buying",
	"selling & haggling",
	"factions & perks",
	"heat & stolen",
	"contraband & inspections",
	"rent & cash",
	"upgrades & services",
	"after-hours",
	"night/suppliers",
	"early game",
	"endgame/buyout",
	"difficulty",
	"production",
	"scavenging",
	"market events & speculation",
}

// Kinds: a mechanic is checkable against the code; a strategy is advice; an opinion is
// taste.
var Kinds = []string{"mechanic", "strategy", "opinion"}

type Conditions struct {
	Days       string `json:"days,omitempty"`
	Difficulty string `json:"difficulty,omitempty"`
	Start      string `json:"start,omitempty"`
}

type Claim struct {
	ID         string     `json:"id"`
	Claim      string     `json:"claim"`
	Topic      string     `json:"topic"`
	Conditions Conditions `json:"conditions"`
	Kind       string     `json:"kind"`
	// System names the game system or loop the claim is about ("wine", "furnace",
	// "dumping ground", "wanted list"…): free text, normalised later by the systems stage.
	System   string `json:"system,omitempty"`
	SourceID string `json:"source_id"`
	URL      string `json:"url"`
	Locator  string `json:"locator,omitempty"`
	Quote    string `json:"quote,omitempty"`
}

const (
	maxClaimLen = 400
	minClaimLen = 15
	maxQuoteLen = 200
)

// Validate normalises c in place and reports why it is unusable, if it is.
func (c *Claim) Validate() error {
	c.Claim = collapseSpace(c.Claim)
	c.Topic = normTopic(c.Topic)
	c.Kind = strings.ToLower(strings.TrimSpace(c.Kind))
	c.Locator = strings.TrimSpace(c.Locator)
	c.System = strings.ToLower(collapseSpace(c.System))
	c.Quote = collapseSpace(c.Quote)
	if len(c.Claim) < minClaimLen {
		return fmt.Errorf("claim too short")
	}
	if len(c.Claim) > maxClaimLen {
		return fmt.Errorf("claim too long (%d)", len(c.Claim))
	}
	if !contains(Topics, c.Topic) {
		return fmt.Errorf("unknown topic %q", c.Topic)
	}
	if !contains(Kinds, c.Kind) {
		return fmt.Errorf("unknown kind %q", c.Kind)
	}
	if r := []rune(c.Quote); len(r) > maxQuoteLen {
		c.Quote = string(r[:maxQuoteLen-1]) + "…"
	}
	return nil
}

// ClaimID is stable across runs so cached stages can refer to claims.
func ClaimID(sourceID, claim string) string {
	h := sha256.Sum256([]byte(sourceID + "\x00" + strings.ToLower(collapseSpace(claim))))
	return "c" + hex.EncodeToString(h[:6])
}

// ParseClaims pulls claims out of model output: a {"claims":[...]} object, a bare array,
// or JSON lines. Unusable entries are counted, not fatal; an answer with no parseable
// structure at all is an error so the caller can retry.
func ParseClaims(out string) (claims []Claim, dropped int, err error) {
	s := llm.StripThinking(out)
	var raws []json.RawMessage
	if obj, e := llm.ExtractJSON(s); e == nil {
		var wrap struct {
			Claims []json.RawMessage `json:"claims"`
		}
		if json.Unmarshal(obj, &wrap) == nil && wrap.Claims != nil {
			raws = wrap.Claims
		}
	}
	if raws == nil {
		if i := strings.IndexByte(s, '['); i >= 0 {
			if j := strings.LastIndexByte(s, ']'); j > i {
				_ = json.Unmarshal([]byte(s[i:j+1]), &raws)
			}
		}
	}
	if raws == nil {
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "{") && json.Valid([]byte(line)) {
				raws = append(raws, json.RawMessage(line))
			}
		}
	}
	if raws == nil {
		// An answer cut off by the token limit still holds every claim before the cut.
		raws = completeObjects(s)
	}
	if raws == nil {
		return nil, 0, fmt.Errorf("no claims structure in output")
	}
	for _, r := range raws {
		var c Claim
		if json.Unmarshal(r, &c) != nil {
			// conditions sometimes arrive as a string; keep the claim, drop the field.
			var loose map[string]any
			if json.Unmarshal(r, &loose) != nil {
				dropped++
				continue
			}
			delete(loose, "conditions")
			b, _ := json.Marshal(loose)
			if json.Unmarshal(b, &c) != nil {
				dropped++
				continue
			}
		}
		if c.Validate() != nil {
			dropped++
			continue
		}
		claims = append(claims, c)
	}
	return claims, dropped, nil
}

func normTopic(t string) string {
	t = strings.ToLower(collapseSpace(t))
	t = strings.ReplaceAll(t, " and ", " & ")
	t = strings.ReplaceAll(t, " / ", "/")
	if contains(Topics, t) {
		return t
	}
	for _, k := range Topics {
		if strings.HasPrefix(k, t) || strings.HasPrefix(t, k) {
			return k
		}
	}
	return t
}

func collapseSpace(s string) string { return strings.Join(strings.Fields(s), " ") }

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// completeObjects returns the balanced JSON objects with a "claim" key that start after
// the first '[' of s, ignoring an unterminated tail.
func completeObjects(s string) []json.RawMessage {
	i := strings.IndexByte(s, '[')
	if i < 0 {
		return nil
	}
	var out []json.RawMessage
	for {
		j := strings.IndexByte(s[i:], '{')
		if j < 0 {
			return out
		}
		i += j
		end := balancedEnd(s[i:])
		if end == 0 {
			return out
		}
		obj := s[i : i+end]
		if json.Valid([]byte(obj)) && strings.Contains(obj, `"claim"`) {
			out = append(out, json.RawMessage(obj))
		}
		i += end
	}
}

func balancedEnd(s string) int {
	depth := 0
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}
