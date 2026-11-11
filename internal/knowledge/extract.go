package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

const (
	chunkTokens   = 6000
	overlapTokens = 400
)

const extractSystem = `You read community material about the PC game "Probably Stolen" (a cyberpunk pawnshop sim: buy and sell at the counter, haggle, six factions with reputation, stolen goods with heat, contraband and Security inspections, weekly rent, buying out the store, production machines, night outings) and extract concrete claims an expert player would want.

Rules:
- One claim = one sentence, concrete and self-contained (name the item, number, day, faction). Paraphrase in plain English; never copy long passages.
- Prefer numbers, thresholds, timings, cause-and-effect, and advice with its reason. Skip story spoilers that carry no gameplay consequence, UI tutorials, generic filler ("have fun", "experiment"), and the narrator's personal chatter.
- kind: "mechanic" = how the game works (checkable in code: numbers, rules, triggers); "strategy" = advice on what to do; "opinion" = a preference with no concrete rule.
- topic: exactly one of: TOPICS
- system: a short lowercase name for the game system or loop the claim is about (e.g. "wine", "berry farming", "furnace", "water purification", "chemistry", "dumping ground", "wanted list", "display case", "insurance", "rent", "market events", "appraisal", "guns", "rats", "fish", "underground exchange", "joe wild network", "dr jackson", "commissary"). Reuse the same name for the same system.
- conditions: day range ("days 1-14"), difficulty (Price Gouge/Cozy, Mark Up/Normal, Thin Margins/Hard, Red Ink/Challenge, Rock Bottom), start type (Apprentice, Water Merchant, Chemist, ...) when the source states or clearly implies them; otherwise leave empty strings.
- locator: the [m:ss] timestamp nearest the claim for transcripts, or the nearest section heading / commenter name for pages.
- quote: a short supporting excerpt from the text, at most 200 characters.
- If the text is a playtest/demo-era source, still extract, but say "demo" in conditions.days only if the source says the claim is demo-specific.
- Transcripts are automatic captions and mishear names. Use the game's spelling: Augs (AI-controlled infiltrators; captions say "orgs", "orcs", "ogs"), Joe Wild (network/favor upgrades), Dr. Jackson (machines and modules), Georgio, the Mentor, the Landlord, Odin Haraldsson, Lou, Officer Lun, Sector 13 Cartel, Black Market, Revolution, Security, Upper Levels, Lower Levels, BloomBerry, Nutrifruit, Hemingway cigarettes, Dream Dust, Smuggler's Bay, Underground Exchange, Commissary, Dumping Ground, GP Mining Coop, Red Imp Brewery, AgeWell, Liquid Luck, Rent Grace; difficulties Cozy/Price Gouge, Mark Up (normal), Thin Margins (hard), Red Ink (challenge); Rock Bottom is a start.
- If nothing in the text is useful, return {"claims": []}.

Answer with only a JSON object: {"claims":[{"claim":"...","topic":"...","system":"...","kind":"mechanic|strategy|opinion","conditions":{"days":"","difficulty":"","start":""},"locator":"...","quote":"..."}]}`

// Extractor turns source text into claims, one cached JSONL file per source.
type Extractor struct {
	LLM       llm.Client
	MaxTokens int
}

func (e *Extractor) system() string {
	return strings.Replace(extractSystem, "TOPICS", strings.Join(Topics, " | "), 1)
}

// ExtractSource returns the number of claims kept and dropped. A finished source has
// a claims file (possibly empty) and is skipped.
func (e *Extractor) ExtractSource(ctx context.Context, st Store, src Source) (kept, dropped int, err error) {
	if exists(st.ClaimsPath(src.ID)) {
		cs, err := ReadClaims(st.ClaimsPath(src.ID))
		return len(cs), 0, err
	}
	b, err := os.ReadFile(st.TextPath(src.ID))
	if err != nil {
		return 0, 0, err
	}
	seen := map[string]bool{}
	var all []Claim
	chunks := ChunkText(string(b), chunkTokens, overlapTokens)
	for _, ch := range chunks {
		user := fmt.Sprintf("Source: %s (%s, by %s, %s)\nPart %d of %d.\n\n<<<TEXT\n%s\nTEXT>>>",
			src.Title, src.Type, orDash(src.Channel), orDash(src.Date), ch.Index+1, len(chunks), ch.Text)
		cs, d, err := e.extractChunk(ctx, user)
		dropped += d
		if err != nil {
			return 0, dropped, fmt.Errorf("%s part %d: %w", src.ID, ch.Index+1, err)
		}
		for _, c := range cs {
			c.SourceID, c.URL = src.ID, src.URL
			c.ID = ClaimID(src.ID, c.Claim)
			if seen[c.ID] {
				continue // the overlap between chunks re-extracts the same sentence
			}
			seen[c.ID] = true
			all = append(all, c)
		}
	}
	return len(all), dropped, WriteClaims(st.ClaimsPath(src.ID), all)
}

func (e *Extractor) extractChunk(ctx context.Context, user string) ([]Claim, int, error) {
	req := llm.Request{System: e.system(), Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxTokens: e.MaxTokens}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		resp, err := e.LLM.Complete(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}
		cs, d, err := ParseClaims(resp.Text)
		if err == nil {
			return cs, d, nil
		}
		lastErr = err
		req.Messages = append(req.Messages,
			llm.Message{Role: llm.RoleAssistant, Content: truncate(resp.Text, 2000)},
			llm.Message{Role: llm.RoleUser, Content: "That was not the requested JSON object. Answer again with only {\"claims\":[...]}."})
	}
	return nil, 0, lastErr
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func ReadClaims(path string) ([]Claim, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []Claim
	for _, line := range bytes.Split(b, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var c Claim
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, c)
	}
	return out, nil
}

func WriteClaims(path string, cs []Claim) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, c := range cs {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return writeFile(path, buf.Bytes())
}
