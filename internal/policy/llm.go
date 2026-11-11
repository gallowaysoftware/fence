package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	DefaultMaxContextBytes = 32 << 10
	DefaultMaxTokens       = 4096
	// DefaultMinRepairTime is the least time left before the deadline for which a
	// repair round is worth starting; below it the answer would be cut off anyway.
	DefaultMinRepairTime = 1500 * time.Millisecond
	DefaultRecallBytes   = 4000
	// DefaultRecallTimeout bounds memory lookup so it never eats the model's deadline.
	DefaultRecallTimeout = 300 * time.Millisecond
	// maxPlanBytes bounds the day plan in the prompt; the strategist keeps it under 2 KB.
	maxPlanBytes = 2400
	// maxPlanAgeDays is how stale a plan may be and still be shown: a failed or pending
	// morning plan leaves yesterday's in force, but not last week's.
	maxPlanAgeDays = 2
	// maxRationale fits the in-game tooltip the rationale is shown in.
	maxRationale = 240
)

// LLM asks a model, validates the answer, repairs once, and otherwise defers. It relies
// on ctx carrying the decision deadline (already shortened by the caller's margin).
type LLM struct {
	Client llm.Client
	// Bounds puts minimum/maximum into the output schema. llama.cpp grammars enforce
	// them; the Anthropic API rejects numeric constraints, so it must be false there.
	Bounds          bool
	MaxTokens       int
	MaxContextBytes int
	MinRepairTime   time.Duration
	// Recall adds remembered facts to the prompt; nil means none.
	Recall        Recall
	RecallBytes   int
	RecallTimeout time.Duration
	// Plan supplies the day's plan from the strategist; nil means none.
	Plan Planner
	// Use notes what an item at the counter is worth used in the shop; nil means none.
	Use UseFunc
	// StageLLM sends stage decisions to the model; off, the fast rules answer them, because
	// staging is time-critical and the model's single pick came too late and too narrow.
	StageLLM bool
	Now      func() time.Time
}

type LLMOption func(*LLM)

func NewLLM(c llm.Client, opts ...LLMOption) *LLM {
	p := &LLM{Client: c}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Recall has memory.Recall's method set. It is declared here so policy does not depend
// on the memory package; any memory.Recall satisfies it.
type Recall interface {
	Brief(ctx context.Context, req *protocol.DecideRequest, maxBytes int) string
}

// Planner has the server's Planner method set: the latest plan for a session and the
// game day it was written for.
type Planner interface {
	CurrentPlan(session string) (text string, day int, ok bool)
}

func WithRecall(r Recall) LLMOption { return func(p *LLM) { p.Recall = r } }

// UseFunc is the strategist's use-versus-sale note for an item in session's shop ("" when
// it knows no use); sale is what the item would sell for, 0 if unknown.
type UseFunc func(session, id, name string, sale int64) string

// WithUse notes each item's use value next to it in barter and stage prompts, so an item
// is weighed by what it enables in the shop, not only by its price.
func WithUse(u UseFunc) LLMOption { return func(p *LLM) { p.Use = u } }

// WithPlan puts the strategist's plan for the day at the top of every prompt.
func WithPlan(pl Planner) LLMOption  { return func(p *LLM) { p.Plan = pl } }
func WithStageLLM(on bool) LLMOption { return func(p *LLM) { p.StageLLM = on } }
func WithBounds(b bool) LLMOption    { return func(p *LLM) { p.Bounds = b } }
func WithMaxTokens(n int) LLMOption  { return func(p *LLM) { p.MaxTokens = n } }

func (p *LLM) Name() string { return "llm" }

// Decide carries option notes on every decision, a fallback included: they need no model.
func (p *LLM) Decide(ctx context.Context, req *protocol.DecideRequest) Decision {
	// The server already strips hidden; this keeps the guarantee local to the prompt.
	req = req.PolicyView()
	if req.Kind == "stage" && !p.StageLLM {
		return Heuristic{Plan: p.Plan}.Decide(ctx, req)
	}
	d := p.decide(ctx, req)
	if err := CheckChoice(req, d.Choice); err != nil {
		trace := d.Trace
		d = Fallback(err.Error())
		d.Trace, d.Trace.Note = trace, err.Error()
	}
	d.Notes = OptionNotes(req)
	return d
}

func (p *LLM) decide(ctx context.Context, req *protocol.DecideRequest) Decision {
	spec, ok := kinds[req.Kind]
	if !ok {
		return Fallback("unknown decision kind " + fmt.Sprintf("%q", req.Kind))
	}
	if req.Kind == inspectKind {
		_, spec.System = currentGuides()
	}
	maxCtx := p.MaxContextBytes
	if maxCtx <= 0 {
		maxCtx = DefaultMaxContextBytes
	}
	if len(req.Context) > maxCtx {
		return Fallback(fmt.Sprintf("context is %d bytes, over the %d-byte prompt limit", len(req.Context), maxCtx))
	}
	prompt, err := buildPrompt(req, spec, p.plan(req), p.brief(ctx, req), p.itemUse(req))
	if err != nil {
		return Fallback("cannot build prompt: " + err.Error())
	}
	schema, err := choiceSchema(req.Options, p.Bounds)
	if err != nil {
		return Fallback("cannot build schema: " + err.Error())
	}
	maxTokens := p.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	trace := Trace{System: spec.System, Prompt: prompt}
	msgs := []llm.Message{{Role: llm.RoleUser, Content: prompt}}

	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 && !p.timeLeft(ctx) {
			trace.Note = "no time left for a repair round"
			break
		}
		start := p.now()
		resp, err := p.Client.Complete(ctx, llm.Request{
			System: spec.System, Messages: msgs, Schema: schema, SchemaName: "choice", MaxTokens: maxTokens,
		})
		a := Attempt{Raw: resp.Text, Reasoning: resp.Reasoning, Model: resp.Model, StopReason: resp.StopReason, LatencyMS: p.now().Sub(start).Milliseconds()}
		if err != nil {
			a.Error = err.Error()
			trace.Attempts = append(trace.Attempts, a)
			// A transport or API failure is not something the model can repair.
			break
		}
		choice, rationale, verr := parseAnswer(resp.Text, req.Options)
		if verr == nil {
			verr = CheckChoice(req, choice)
		}
		if verr == nil {
			trace.Attempts = append(trace.Attempts, a)
			return Decision{Choice: choice, Rationale: rationale, Source: protocol.SourceLLM, Trace: trace}
		}
		a.Error = verr.Error()
		trace.Attempts = append(trace.Attempts, a)
		msgs = append(msgs,
			llm.Message{Role: llm.RoleAssistant, Content: nonEmpty(resp.Text)},
			llm.Message{Role: llm.RoleUser, Content: "That answer is invalid: " + verr.Error() +
				"\nReply with only the corrected JSON object."},
		)
	}
	d := Fallback(fallbackReason(trace))
	d.Trace = trace
	if d.Trace.Note == "" {
		d.Trace.Note = d.Rationale
	}
	return d
}

func fallbackReason(t Trace) string {
	if t.Note != "" {
		return t.Note
	}
	if n := len(t.Attempts); n > 0 && t.Attempts[n-1].Error != "" {
		return "model failed: " + t.Attempts[n-1].Error
	}
	return "model gave no valid answer"
}

// Some APIs reject an empty assistant turn; the repair round must still be well-formed.
func nonEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(empty)"
	}
	return s
}

// recentPlan returns the strategist's plan for req's session, or "" when there is none
// recent enough to follow.
func recentPlan(pl Planner, req *protocol.DecideRequest) (text string, day int) {
	if pl == nil {
		return "", 0
	}
	text, day, ok := pl.CurrentPlan(req.SessionID)
	text = strings.TrimSpace(text)
	if !ok || text == "" || day > req.GameDay || req.GameDay-day > maxPlanAgeDays {
		return "", 0
	}
	if len(text) > maxPlanBytes {
		cut := maxPlanBytes
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
	}
	return text, day
}

// plan returns the plan section for req, or "" when there is no recent plan.
func (p *LLM) plan(req *protocol.DecideRequest) string {
	text, day := recentPlan(p.Plan, req)
	if text == "" {
		return ""
	}
	if day == req.GameDay {
		return "TODAY'S PLAN (from this morning's strategist; serve it):\n" + text
	}
	return fmt.Sprintf("LATEST PLAN (written for day %d; today's is not ready, so adjust its dates):\n%s", day, text)
}

func (p *LLM) itemUse(req *protocol.DecideRequest) itemUse {
	if p.Use == nil {
		return nil
	}
	return func(id, name string, sale int64) string { return p.Use(req.SessionID, id, name, sale) }
}

func (p *LLM) brief(ctx context.Context, req *protocol.DecideRequest) string {
	if p.Recall == nil {
		return ""
	}
	n := p.RecallBytes
	if n <= 0 {
		n = DefaultRecallBytes
	}
	d := p.RecallTimeout
	if d <= 0 {
		d = DefaultRecallTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	if req.Kind == "stage" {
		req = stageRecallView(req)
	}
	s := strings.TrimSpace(p.Recall.Brief(ctx, req, n))
	if len(s) > n {
		s = s[:n]
	}
	return s
}

func (p *LLM) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

func (p *LLM) timeLeft(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	dl, ok := ctx.Deadline()
	if !ok {
		return true
	}
	min := p.MinRepairTime
	if min <= 0 {
		min = DefaultMinRepairTime
	}
	return dl.Sub(p.now()) >= min
}

type answer struct {
	Rationale string                     `json:"rationale"`
	Option    string                     `json:"option"`
	Params    map[string]json.RawMessage `json:"params"`
}

func parseAnswer(text string, opts []protocol.Option) (protocol.Choice, string, error) {
	raw, err := llm.ExtractJSON(text)
	if err != nil {
		return protocol.Choice{}, "", err
	}
	var a answer
	if err := json.Unmarshal(raw, &a); err != nil {
		return protocol.Choice{}, "", fmt.Errorf("answer is not the expected object: %w", err)
	}
	if a.Option == "" {
		return protocol.Choice{}, "", fmt.Errorf(`answer has no "option"`)
	}
	c := protocol.Choice{Option: a.Option}
	if len(a.Params) > 0 {
		c.Params = a.Params
	}
	if err := ValidateChoice(opts, c); err != nil {
		return protocol.Choice{}, "", err
	}
	return c, clipRationale(a.Rationale), nil
}

// clipRationale keeps the first two sentences, within maxRationale bytes.
func clipRationale(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	sentences := 0
	for i := 0; i < len(s); i++ {
		// A sentence ends at punctuation followed by a capitalised word, so "vs. x"
		// and "3.5" do not split.
		if (s[i] == '.' || s[i] == '!' || s[i] == '?') && (i+1 == len(s) || i+2 < len(s) && s[i+1] == ' ' && unicode.IsUpper(rune(s[i+2]))) {
			if sentences++; sentences == 2 {
				s = s[:i+1]
				break
			}
		}
	}
	if len(s) <= maxRationale {
		return s
	}
	cut := maxRationale - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return strings.TrimRight(s[:cut], " ") + "…"
}

func buildPrompt(req *protocol.DecideRequest, spec kindSpec, plan, brief string, use itemUse) (string, error) {
	var b strings.Builder
	if plan != "" {
		b.WriteString(plan + "\n\n")
	}
	fmt.Fprintf(&b, "Day %d.\n", req.GameDay)
	var note func(id, label string) string
	ok := false
	if len(req.Context) > 0 {
		note, ok = spec.Render(&b, req.Context, use)
	}
	if !ok {
		var c bytes.Buffer
		if len(req.Context) > 0 {
			if err := json.Compact(&c, req.Context); err != nil {
				return "", err
			}
		}
		fmt.Fprintf(&b, "State (JSON): %s\n", nonEmptyJSON(c.String()))
	}
	if brief != "" {
		fmt.Fprintf(&b, "\nFrom memory of past play:\n%s\n", brief)
	}
	b.WriteString("\nOptions:\n")
	for _, o := range req.Options {
		if !o.Choosable() {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s", o.ID, o.Label)
		if note != nil {
			if n := note(o.ID, o.Label); n != "" {
				b.WriteString(" => " + n)
			}
		}
		if len(o.Params) > 0 {
			var ps []string
			for _, name := range sortedKeys(o.Params) {
				p := o.Params[name]
				ps = append(ps, fmt.Sprintf("%q integer from %d to %d", name, *p.Min, *p.Max))
			}
			fmt.Fprintf(&b, " (params: %s)", strings.Join(ps, ", "))
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "- %s: let the human player decide\n", protocol.OptionDefer)
	return b.String(), nil
}

func nonEmptyJSON(s string) string {
	if s == "" {
		return "{}"
	}
	return s
}
