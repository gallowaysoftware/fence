package strategy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// directivesLead is directivesAsk's opening, which assumes a prose plan came first.
const directivesLead = "Now write that plan as directives for the executor that plays the counter and the night: "

// gymExtensionsAsk lists the gym executor's directive keys beyond the protocol contract
// (sim/gym/README.md, "Playing as an agent"). The strong play we distil from leans on them.
const gymExtensionsAsk = `The gym executor also reads these keys:
"engine": "wine" or "water" or "wine+water" or "none" (the production engine the executor runs),
"growth": [upgrade ids bought with Wild favor or surplus cash, in order],
"upgrade_reserve": cash kept after an engine upgrade,
"mortgage": "auto" or "take" or "decline" or "accept_if_rent",
"buy": {"stock_cap": most trading stock to hold, by estimate, "restock_below": buy while trading stock is below this},
"sell": {"liquidate_min_pct": least % of the estimate when liquidating for rent, "sell_hot": true to stage stolen goods}.
Stock caps count trading stock only: engine inputs and hold_uids are excluded.`

// DistillPrompt is the one-call directives prompt a distilled model is trained and served
// on: the morning plan prompt, then directives with a one-line WHY instead of the prose
// plan. With gym, the ask also lists the gym executor's extension keys.
func DistillPrompt(in PlanInput, gym bool) (system, user string, err error) {
	prompt, err := BuildPlanPrompt(in)
	if err != nil {
		return "", "", err
	}
	s, _ := parseSnapshot(in.Snapshot)
	day := s.day(in.Day)
	prompt = strings.TrimSuffix(prompt, fmt.Sprintf("\nWrite the plan for day %d.", day))

	system = planSystem
	if strings.TrimSpace(in.Persona) != "" {
		system += "\n" + styleRule
	}
	var b strings.Builder
	b.WriteString(prompt)
	fmt.Fprintf(&b, "\nWrite day %d as directives, not a prose plan: first one line \"WHY: <the day's reasoning: rent, cash, the move that matters>\", then the directives for the executor that plays the counter and the night: ", day)
	b.WriteString(strings.TrimPrefix(directivesPrompt(s), directivesLead))
	if gym {
		b.WriteString("\n" + gymExtensionsAsk)
	}
	return system, b.String(), nil
}

// DistillDeltaPrompt is the one-call prompt a distilled delta planner is trained and served
// on: the morning plan prompt, then the delta ask over in.BaseDirectives. The answer is the
// delta object alone, its "why" carrying the reasoning, so no prose plan is paid for.
func DistillDeltaPrompt(in PlanInput) (system, user string, err error) {
	prompt, err := BuildPlanPrompt(in)
	if err != nil {
		return "", "", err
	}
	s, _ := parseSnapshot(in.Snapshot)
	day := s.day(in.Day)
	prompt = strings.TrimSuffix(prompt, fmt.Sprintf("\nWrite the plan for day %d.", day))

	system = planSystem
	if strings.TrimSpace(in.Route) != "" {
		system += "\n" + routeRule
	}
	if strings.TrimSpace(in.style()) != "" {
		system += "\n" + styleRule
	}
	user = prompt + fmt.Sprintf("\nWrite day %d as changes to the executor's plan, not a prose plan: put \"why\" first, then the keys it explains.\n", day) +
		deltaAsk + baseNote(in.BaseDirectives) + deltaFacts(s)
	return system, user, nil
}

// FinishDistillDelta turns a distilled delta planner's answer into the day's directives with
// the bridge's delta lints, as the league's delta planner would have played it.
func FinishDistillDelta(answer string, snapshot json.RawMessage, day int, base string) DirectivesResult {
	s, err := parseSnapshot(snapshot)
	if err != nil {
		r := DirectivesResult{Err: fmt.Errorf("day snapshot: %w", err)}
		r.Directives.Day = day
		return r
	}
	raw, err := llm.ExtractJSON(answer)
	return finishDelta(raw, err, s, s.day(day), base)
}
