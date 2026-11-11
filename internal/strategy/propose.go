package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// The hybrid proposer (docs/research/HYBRID.md): at a decision point the model proposes 2-3
// alternative directive deltas, and the gym (sim/rollout, -hybrid llm) plays each forward over
// sampled futures beside the plan in force and the base policy, keeping the best. The gym is the
// judge, so the delta lints that guard a single unchecked plan (lintDelta, protectBuying) are not
// applied here: a bad proposal simply loses its rollouts.

// ProposeRequest is what fence-rollout writes at a decision point (sim/rollout/Hybrid.cs).
type ProposeRequest struct {
	Day        int              `json:"day"`
	Seed       int              `json:"seed"`
	Difficulty string           `json:"difficulty"`
	Triggers   []ProposeTrigger `json:"triggers"`
	// InForce is the delta the executor plays now, over Base.
	InForce     json.RawMessage `json:"in_force"`
	Base        json.RawMessage `json:"base_directives"`
	HoldDays    int             `json:"hold_days"`
	HorizonDays int             `json:"horizon_days"`
	// CommitHorizonDays is how far a decision is rolled when a candidate signs, refuses or puts off
	// the mortgage or the cash buyout (0: not lengthened).
	CommitHorizonDays int `json:"commit_horizon_days"`
	Samples           int `json:"samples"`
	// Step is the morning view step mode shows a player (sim/gym/StepMode.cs).
	Step json.RawMessage `json:"step"`
}

type ProposeTrigger struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Proposal struct {
	Name       string          `json:"name"`
	Why        string          `json:"why"`
	Directives json.RawMessage `json:"directives"`
	Fixes      []string        `json:"fixes,omitempty"`
}

// ProposeResult is the answer file: the usable proposals and what the call cost.
type ProposeResult struct {
	Candidates []Proposal `json:"candidates"`
	Error      string     `json:"error,omitempty"`
	Model      string     `json:"model,omitempty"`
	// Text is the model's raw answer, kept for distillation.
	Text string `json:"text,omitempty"`
	// Repair says how a malformed answer was mended: "local" (brackets closed), "model" (a
	// second call), or "" when it parsed as written.
	Repair      string   `json:"repair,omitempty"`
	Dropped     []string `json:"dropped,omitempty"`
	PromptBytes int      `json:"prompt_bytes"`
	Seconds     float64  `json:"seconds"`
}

// MaxProposals is how many alternatives a decision rolls besides the plan in force and the base.
const MaxProposals = 3

const proposeSystem = `You are the strategist of a pawnshop in the cyberpunk game "Probably Stolen". An executor plays every day by tuned rules under a base policy; you steer it with directives. You do not choose alone: a simulator of the real game plays each alternative you propose forward over several sampled futures, next to the plan in force and the base policy, and keeps whichever does best on survival and progress toward owning the shop (a buyout scores most; losing the run scores least; arrest and repossession risk at the end count against). So propose real alternatives that a different choice today could make better, not restatements of the plan in force.

HOW THE RUN IS WON: buy the shop out with cash (the landlord's price rises daily after day 50, up to a cap), or take the loan shark's mortgage and make every weekly payment (the last one wins). Lose: arrested, repossessed with too little stock, rent refused without Landlord Grace, a faction at -100. On hard the rent grows 10% a week and trading margins are thin. The mortgage offer comes on a rent morning once cash is twice the down payment (the state gives the numbers); "later" lets him come back next week, "decline" is final. After signing, the weekly payment replaces the rent, and a missed payment repossesses stock worth a payment (the run ends when there is not enough), so a signing is judged by whether the shop makes every payment, not by the weeks after it. Which win is in reach shows in the base policy (buyout_by_day aims at the cash buyout) and the cash against the price. Georgio asks for a loan on day 43 (25% or 50% of cash) and repays 4x on the first rent morning more than 5 days later that finds cash under the rent, otherwise on day 69. The permit official comes from day 38; unlicensed trading builds a criminal record (evidence 100 brings a sentencing officer: arrest at 300+ crime).`

const proposeAsk = `Propose %d alternatives to the plan in force for the next %d days (each is played for %d days, then the base policy), over %d sampled futures of %d days%s. Each alternative is the COMPLETE set of changes over the base policy, not over the plan in force: to keep part of the plan in force, repeat it; {} is the base policy itself, which is already a candidate. Make them differ from each other and from the plan in force on the lever today's events put in play (the mortgage terms, Georgio's loan, the permit, buying and selling, cash floors, the engines, upgrades).
Answer with one JSON object and nothing else: {"candidates":[{"name":"<short_snake_case>","why":"<at most 20 words: the fact in today's state that makes this worth trying>","directives":{...}}]}.`

// proposeSchema constrains the answer where the server supports response_format; parseProposals
// still checks every directive.
var proposeSchema = json.RawMessage(`{"type":"object","properties":{"candidates":{"type":"array","maxItems":3,"items":{"type":"object","properties":{"name":{"type":"string"},"why":{"type":"string"},"directives":{"type":"object"}},"required":["name","why","directives"]}}},"required":["candidates"]}`)

// gymKeys are executor extensions beyond the directives contract (sim/gym) a proposal may set.
const gymKeys = `Executor extensions (also allowed): "georgio" ("lend_25"|"lend_50"|"no": his day-43 loan), "georgio_peak_days" [first, last] (stop buying and sell down over those days to lend more), "georgio_collect" ("early": spend below the rent on the eve of a rent day after lending, so he repays 4x), "buyout" ("hold_cash_for_buyout"), "repo_reserve_payments" (stock to keep, in payments, while paying the mortgage; default 1.5), "mortgage_selldown_days" (days before a payment the sell-down starts; default 5), "mortgage_selldown_margin" (cash above the payment it sells down to; default 150), "spend_down_to" (end the day under this cash, buying stock at up to its estimate), "upgrade_reserve" (cash kept from upgrades).`

// BuildProposePrompt renders req as the user message: today's state, the run's risks, the
// policies in force and why the decision is asked now.
func BuildProposePrompt(req ProposeRequest) (string, *snapshot, error) {
	var step struct {
		Snapshot  json.RawMessage `json:"snapshot"`
		Yesterday json.RawMessage `json:"yesterday"`
		Risks     json.RawMessage `json:"risks"`
	}
	if err := json.Unmarshal(req.Step, &step); err != nil {
		return "", nil, fmt.Errorf("step snapshot: %w", err)
	}
	s, err := parseSnapshot(step.Snapshot)
	if err != nil {
		return "", nil, fmt.Errorf("day snapshot: %w", err)
	}
	day := s.day(req.Day)
	var b strings.Builder
	fmt.Fprintf(&b, "TODAY: day %d, morning (%s difficulty).\n", day, nonEmpty(req.Difficulty, "unknown"))
	s.render(&b, day)
	if r := compact(step.Risks); r != "" {
		fmt.Fprintf(&b, "\nRUN RISKS (the game's numbers): %s\n", r)
	}
	if y := compact(step.Yesterday); y != "" && y != "null" {
		fmt.Fprintf(&b, "YESTERDAY: %s\n", y)
	}
	fmt.Fprintf(&b, "\n%s\n%s\n%s\n", executorRules, deltaKeys, gymKeys)
	fmt.Fprintf(&b, "\nBASE POLICY of this run (over the rules above): %s\n", nonEmpty(compact(req.Base), "{}"))
	fmt.Fprintf(&b, "PLAN IN FORCE (changes over the base): %s\n", nonEmpty(compact(req.InForce), "{}"))
	b.WriteString("\nWHY YOU ARE ASKED NOW:\n")
	for _, t := range req.Triggers {
		fmt.Fprintf(&b, "- %s: %s\n", t.ID, t.Text)
	}
	b.WriteByte('\n')
	hold, horizon, samples := max(req.HoldDays, 1), max(req.HorizonDays, 1), max(req.Samples, 1)
	commit := ""
	if req.CommitHorizonDays > horizon {
		commit = fmt.Sprintf(" (%d days when any candidate, the plan in force included, signs, refuses or puts off the mortgage or the cash buyout: a commitment is judged to the end of its term)", req.CommitHorizonDays)
	}
	fmt.Fprintf(&b, proposeAsk, MaxProposals, hold, hold, samples, horizon, commit)
	return b.String(), s, nil
}

func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if len(raw) == 0 || json.Compact(&buf, raw) != nil {
		return ""
	}
	return buf.String()
}

// Propose asks c for the alternatives of one decision. A failed call or an unusable answer
// returns no candidates and the error; the gym then rolls only the plan in force and the base.
func Propose(ctx context.Context, c llm.Client, req ProposeRequest, maxTokens int) (r ProposeResult) {
	start := time.Now()
	prompt, snap, err := BuildProposePrompt(req)
	r.PromptBytes = len(prompt)
	defer func() { r.Seconds = math.Round(time.Since(start).Seconds()*10) / 10 }()
	if err != nil {
		r.Error = err.Error()
		return r
	}
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	msgs := []llm.Message{{Role: llm.RoleUser, Content: prompt}}
	resp, err := c.Complete(ctx, llm.Request{System: proposeSystem, Messages: msgs, MaxTokens: maxTokens, Schema: proposeSchema, SchemaName: "proposals"})
	if err != nil {
		r.Error = err.Error()
		return r
	}
	r.Model, r.Text = resp.Model, resp.Text
	raw, err := proposalJSON(resp.Text)
	if err != nil {
		if raw, err = proposalJSON(closeJSON(resp.Text)); err == nil {
			r.Repair = "local"
		}
	}
	if err != nil {
		// One more call, shown its own answer and the parse error: a dropped brace or quote is
		// cheaper to mend than the decision is to lose.
		fix := fmt.Sprintf("Your answer is not valid JSON of the form {\"candidates\":[...]} (%v). Reply with the corrected JSON object only, same candidates.", err)
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: resp.Text}, llm.Message{Role: llm.RoleUser, Content: fix})
		if resp2, err2 := c.Complete(ctx, llm.Request{System: proposeSystem, Messages: msgs, MaxTokens: maxTokens, Schema: proposeSchema, SchemaName: "proposals"}); err2 == nil {
			r.Text += "\n--- repair ---\n" + resp2.Text
			if raw, err = proposalJSON(resp2.Text); err != nil {
				raw, err = proposalJSON(closeJSON(resp2.Text))
			}
			if err == nil {
				r.Repair = "model"
			}
		}
	}
	if err == nil {
		r.Candidates, r.Dropped, err = parseProposals(raw, snap, string(req.Base))
	}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}

// proposalJSON is the first JSON object in text that has a candidates list. ExtractJSON alone
// returns the first balanced object, which in an answer missing one brace is a lone candidate.
func proposalJSON(text string) (json.RawMessage, error) {
	raw, err := llm.ExtractJSON(text)
	if err != nil {
		return nil, err
	}
	var probe struct {
		Candidates json.RawMessage `json:"candidates"`
	}
	if json.Unmarshal(raw, &probe) != nil || len(probe.Candidates) == 0 || probe.Candidates[0] != '[' {
		return nil, errors.New(`no {"candidates": [...]} object in the answer`)
	}
	return raw, nil
}

// jsonOutside are the bytes JSON allows outside a string; the first other one (a closing code
// fence, prose) ends the answer.
const jsonOutside = "{}[],:\" \t\r\n-+.0123456789eEtrufalsn"

// closeJSON mends the commonest malformed answers from the first '{': a closer that skips an
// open object or array gets the missing closers inserted before it, and an answer cut off
// mid-string or mid-object gets its string and brackets closed.
func closeJSON(text string) string {
	text = llm.StripThinking(text)
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return text
	}
	var out strings.Builder
	var stack []byte
	inStr, esc := false, false
	for i := start; i < len(text); i++ {
		c := text[i]
		if inStr {
			out.WriteByte(c)
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
		if !strings.ContainsRune(jsonOutside, rune(c)) {
			break
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}', ']':
			for len(stack) > 0 && stack[len(stack)-1] != c && slices.Contains(stack, c) {
				out.WriteByte(stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 || stack[len(stack)-1] != c {
				continue
			}
			stack = stack[:len(stack)-1]
		}
		out.WriteByte(c)
		if len(stack) == 0 {
			return out.String()
		}
	}
	if inStr {
		out.WriteByte('"')
	}
	s := strings.TrimRight(out.String(), " \t\r\n,:")
	for i := len(stack) - 1; i >= 0; i-- {
		s += string(stack[i])
	}
	return s
}

// parseProposals keeps each candidate whose directives the contract and the state allow
// (parseDirectives) plus the executor extensions (gymExtras); dropped says why the others went.
func parseProposals(raw json.RawMessage, s *snapshot, base string) (out []Proposal, dropped []string, err error) {
	var a struct {
		Candidates []struct {
			Name       string          `json:"name"`
			Why        string          `json:"why"`
			Directives json.RawMessage `json:"directives"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return nil, nil, fmt.Errorf("answer is not {\"candidates\": [...]}: %w", err)
	}
	for i, c := range a.Candidates {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			name = fmt.Sprintf("alt%d", i+1)
		}
		if len(out) == MaxProposals {
			dropped = append(dropped, name+": more than "+fmt.Sprint(MaxProposals))
			continue
		}
		var m map[string]any
		if json.Unmarshal(c.Directives, &m) != nil {
			dropped = append(dropped, name+": directives are not an object")
			continue
		}
		delete(m, "why")
		delete(m, "day")
		extras, fixes := gymExtras(m)
		var d json.RawMessage
		if len(m) > 0 {
			b, _ := json.Marshal(m)
			pd, _, pfix, perr := parseDirectives(b, s)
			fixes = append(fixes, pfix...)
			if perr == nil {
				fixes = append(fixes, lintBaseEngine(&pd, base)...)
				pd.Day = 0
				d, _ = json.Marshal(pd)
			}
		}
		merged := map[string]any{}
		if len(d) > 0 {
			json.Unmarshal(d, &merged)
		}
		for k, v := range extras {
			merged[k] = v
		}
		if len(merged) == 0 {
			dropped = append(dropped, name+": no usable change ("+strings.Join(fixes, "; ")+")")
			continue
		}
		d, _ = json.Marshal(merged)
		out = append(out, Proposal{Name: clip(name, 40), Why: clip(strings.TrimSpace(c.Why), 200), Directives: d, Fixes: fixes})
	}
	if len(out) == 0 {
		return nil, dropped, errors.New("no usable candidate")
	}
	return out, dropped, nil
}

// gymExtras takes the executor extensions out of m, keeping the well-formed ones.
func gymExtras(m map[string]any) (map[string]any, []string) {
	out := map[string]any{}
	var fixes []string
	enum := func(k string, allowed ...string) {
		v, ok := m[k]
		if !ok {
			return
		}
		delete(m, k)
		if s, _ := v.(string); slices.Contains(allowed, s) {
			out[k] = s
		} else {
			fixes = append(fixes, fmt.Sprintf("%s: dropped %v (want %s)", k, v, strings.Join(allowed, "|")))
		}
	}
	number := func(k string, lo, hi float64, integer bool) {
		v, ok := m[k]
		if !ok {
			return
		}
		delete(m, k)
		x, ok := v.(float64)
		if !ok || math.IsNaN(x) {
			fixes = append(fixes, fmt.Sprintf("%s: dropped %v (want a number)", k, v))
			return
		}
		x = min(max(x, lo), hi)
		if integer {
			x = math.Round(x)
		}
		out[k] = x
	}
	enum("georgio", "lend_25", "lend_50", "no")
	enum("georgio_collect", "early")
	enum("buyout", "hold_cash_for_buyout")
	number("repo_reserve_payments", 0, 5, false)
	number("mortgage_selldown_days", 0, 30, true)
	number("mortgage_selldown_margin", 0, 20000, true)
	number("spend_down_to", 0, 1e7, true)
	number("upgrade_reserve", 0, 1e7, true)
	if v, ok := m["georgio_peak_days"]; ok {
		delete(m, "georgio_peak_days")
		w, _ := v.([]any)
		from, ok1 := num(firstOr(w, 0))
		to, ok2 := num(firstOr(w, 1))
		if len(w) == 2 && ok1 && ok2 && 0 <= from && from <= to {
			out["georgio_peak_days"] = []int64{from, to}
		} else {
			fixes = append(fixes, fmt.Sprintf("georgio_peak_days: dropped %v (want [from, to] days)", v))
		}
	}
	return out, fixes
}
