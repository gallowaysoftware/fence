package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// The inventor (cmd/fenceinvent): the model proposes routes the menu does not have, and the
// gym screens them; only the gym's confirmation puts one on the menu.

const inventMaxTokens = 4000

const inventSystem = `You are designing new strategies ("routes") for a pawn shop in the game Probably Stolen on hard difficulty, for a live stream where language models play. A gym runs the game's own rules and will test every route you propose on 30 seeds; the ones that win at least 57 of 60 on fresh seeds join the menu the stream's players choose from. Propose routes that are different from the menu in what the shop does and how it makes its money, not copies with a number nudged. Answer with one JSON object and nothing else.`

// Proposal is one invented route.
type Proposal struct {
	Composition
	Thesis string `json:"thesis"`
	// Novelty is the model's claim of what is new: which money or which counter behaviour.
	Novelty string `json:"novelty"`
	// Errors say why the proposal cannot be screened ("" when it can).
	Errors []string `json:"errors,omitempty"`
}

// InventPrompt asks for n routes; the menu is shown with each route's levers over the
// appraiser's, so the model sees what has been tried.
func InventPrompt(m *Menu, n int) string {
	var b strings.Builder
	b.WriteString(shopRules + "\n\nTHE MENU (tested; each line: what it changes over the appraiser, and the lab):\n")
	ref, _ := m.Get(DefaultRoute)
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "- %s: start %s, perks %s; changes %s. %s Lab: %s.\n", e.Name, e.Start, e.Perks, diffText(ref, e), e.Brief, e.Lab.text())
	}
	b.WriteString(`
What the lab already found (docs/research): without the Appraisal Board every route loses (0/60); production nets almost nothing before the buyout; aggressive haggling loses every run to the Lower Levels mob; holding stock and narrow specialties win but slowly; the smelter's furnace and the rancher's rats are the only engines a little ahead.

Levers a route may change (over its base route):
`)
	b.WriteString(LeverText())
	fmt.Fprintf(&b, `
Propose %d new routes. Each must differ from every menu route and from each other in at least two levers or in its start. Answer:
{"routes": [{"name": "<new_name>", "base": "<menu route>", "levers": {"<lever>": <value>, ...}, "start": "<only to change the base's>", "perks": "<only to change the base's>", "thesis": "<why it could win hard, 2-3 sentences>", "novelty": "<what a viewer would see that no menu route shows>"}, ...]}`, n)
	return b.String()
}

// diffText names the levers e changes over ref.
func diffText(ref, e Entry) string {
	var a, b map[string]any
	json.Unmarshal([]byte(ref.Base), &a)
	json.Unmarshal([]byte(e.Base), &b)
	var parts []string
	for _, k := range sortedKeys(b) {
		if !reflect.DeepEqual(a[k], b[k]) {
			v, _ := json.Marshal(b[k])
			parts = append(parts, k+" "+string(v))
		}
	}
	for _, k := range sortedKeys(a) {
		if _, ok := b[k]; !ok {
			parts = append(parts, k+" removed")
		}
	}
	if len(parts) == 0 {
		return "nothing"
	}
	return strings.Join(parts, ", ")
}

func sortedKeys(m map[string]any) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && ks[j] < ks[j-1]; j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
	return ks
}

// Invent asks c for n routes, validates each, and gives the invalid ones one repair round.
func Invent(ctx context.Context, c llm.Client, m *Menu, n int) ([]Proposal, string, error) {
	msgs := []llm.Message{{Role: llm.RoleUser, Content: InventPrompt(m, n)}}
	resp, err := c.Complete(ctx, llm.Request{System: inventSystem, Messages: msgs, MaxTokens: inventMaxTokens})
	if err != nil {
		return nil, "", err
	}
	ps, err := parseProposals(resp.Text, m)
	if err != nil {
		return nil, resp.Text, err
	}
	var bad []string
	for _, p := range ps {
		if len(p.Errors) > 0 {
			bad = append(bad, fmt.Sprintf("%s: %s", p.Name, strings.Join(p.Errors, "; ")))
		}
	}
	if len(bad) == 0 {
		return ps, resp.Text, nil
	}
	msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Text},
		llm.Message{Role: llm.RoleUser, Content: "These routes cannot be played as written:\n" + strings.Join(bad, "\n") + "\nAnswer again with the whole corrected list as one JSON object."})
	resp2, err := c.Complete(ctx, llm.Request{System: inventSystem, Messages: msgs, MaxTokens: inventMaxTokens})
	if err != nil {
		return ps, resp.Text, nil
	}
	if ps2, err := parseProposals(resp2.Text, m); err == nil {
		return ps2, resp.Text + "\n---repair---\n" + resp2.Text, nil
	}
	return ps, resp.Text, nil
}

func parseProposals(text string, m *Menu) ([]Proposal, error) {
	raw, err := llm.ExtractJSON(text)
	if err != nil {
		return nil, err
	}
	var a struct {
		Routes []Proposal `json:"routes"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	seen := map[string]string{}
	for i := range a.Routes {
		p := &a.Routes[i]
		liftMeta(&p.Composition)
		p.Errors = p.Validate(m)
		if len(p.Errors) > 0 {
			continue
		}
		key := p.fingerprint(m)
		if other, dup := seen[key]; dup {
			p.Errors = append(p.Errors, "plays exactly as "+other)
		}
		for _, e := range m.Entries {
			if key == e.Start+"|"+sortedSet(e.Perks)+"|"+canonical(e.Base) {
				p.Errors = append(p.Errors, "plays exactly as the menu's "+e.Name)
			}
		}
		seen[key] = p.Name
	}
	return a.Routes, nil
}

// fingerprint is what the gym plays: start, perks and the merged directives.
func (p *Proposal) fingerprint(m *Menu) string {
	base, _ := m.Get(p.Base)
	d, _ := p.merged(base)
	b, _ := json.Marshal(d)
	return p.start(base) + "|" + sortedSet(p.perks(base)) + "|" + string(b)
}

func canonical(js string) string {
	var v any
	json.Unmarshal([]byte(js), &v)
	b, _ := json.Marshal(v)
	return string(b)
}
