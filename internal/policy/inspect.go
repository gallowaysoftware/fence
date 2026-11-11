package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// The inspect decision (docs/re/INSPECT.md): the player has the magnifier on a customer's
// item. Cigarettes, food stamps and injectors get a verdict (take the claim, or confront one
// issue); appraisal-service items get an estimate. Advice only: the mod never confronts or
// estimates.
const (
	inspectKind    = "inspect"
	optGenuine     = "genuine"
	optCounterfeit = "counterfeit"
	confrontPrefix = "confront_"
	optEstimate    = "estimate"
	estimateParam  = "value"

	modeAuthenticity = "authenticity"
	modeAppraisal    = "appraisal"
)

// Item kinds as the mod names them.
const (
	kindCigarettes = "cigarettes"
	kindStamp      = "food_stamp"
	kindInjector   = "injector"
	kindRibwich    = "ribwich"
	kindHand       = "prosthetic_hand"
	kindPlasma     = "plasma_cell"
)

type inspectCtx struct {
	Mode       string            `json:"mode"`
	Item       *inspectItem      `json:"item"`
	Claim      string            `json:"claim"`
	Client     *inspectClient    `json:"client"`
	Reputation *inspectRep       `json:"reputation"`
	Text       map[string]string `json:"text"`
	Visual     map[string]string `json:"visual"`
	Appraisal  *appraisalTerms   `json:"appraisal"`
}

type inspectItem struct {
	UID   *int64 `json:"uid"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Price *int64 `json:"price"`
}

type inspectClient struct {
	Name    string `json:"name"`
	Faction string `json:"faction"`
	Intent  string `json:"intent"`
}

// inspectRep is the faction a wrong confront costs reputation with.
type inspectRep struct {
	Faction string   `json:"faction"`
	Value   *float64 `json:"value"`
	Status  string   `json:"status"`
}

type appraisalTerms struct {
	TheirFigure  *int64           `json:"their_figure"`
	Rank         string           `json:"rank"`
	BaseFee      *int64           `json:"base_fee"`
	FeePct       map[string]int64 `json:"fee_pct"`
	Refusals     *int64           `json:"refusals"`
	LastEstimate *int64           `json:"last_estimate"`
	LastFeedback string           `json:"last_feedback"`
}

func parseInspect(raw json.RawMessage) (*inspectCtx, error) {
	var c inspectCtx
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c.Item == nil || c.Item.Kind == "" {
		return nil, errors.New("inspect context has no item kind")
	}
	return &c, nil
}

// Cue vocabulary: the words the mod's cue dictionary (mod/src/Fence.Core/Inspect/InspectCues.cs)
// uses for what each visual tell depicts. testdata/inspect_cues.json is the shared list; both
// sides test against it.
const (
	cueMatches  = "matches reference"
	cueMisprint = "misprinted"
	cueWrongLog = "wrong logo"
	cueMissing  = "missing"
	cuePresent  = "present"
)

// The rules, restated from docs/re/INSPECT.md.
var (
	injectorCompany  = "Carmicheal Pharmaceutical"
	injectorModel    = "Immunivax"
	recalledPrefixes = []int{250, 251, 301, 355, 385, 386, 421}
	stampDenom       = 40
	stampSealFrom    = 160
)

// finding is one rule a visible cue breaks.
type finding struct {
	// Issue is the confront toggle it proves ("" when the item has no confront, e.g. appraisal).
	Issue   string
	Verdict string
	Why     string
}

type ruleCheck struct {
	Findings []finding
	Passed   []string
	// Unknown lists cues that are missing or unreadable; a verdict resting on them is not certain.
	Unknown []string
	// Clear holds the confront issues whose cues were all read and break no rule: confronting
	// one of them is certainly wrong.
	Clear map[string]bool
	Value *valuation
}

type valuation struct {
	Value int64
	Lines []string
}

func (r *ruleCheck) fail(issue, verdict, why string) {
	r.Findings = append(r.Findings, finding{Issue: issue, Verdict: verdict, Why: why})
}

func (r *ruleCheck) pass(issue, why string) {
	r.Passed = append(r.Passed, why)
	if issue != "" {
		r.Clear[issue] = true
	}
}

func (r *ruleCheck) unknown(what string) { r.Unknown = append(r.Unknown, what) }

func (r *ruleCheck) certain() bool { return len(r.Unknown) == 0 }

// finding returns the first finding for issue.
func (r *ruleCheck) finding(issue string) (finding, bool) {
	for _, f := range r.Findings {
		if f.Issue == issue {
			return f, true
		}
	}
	return finding{}, false
}

// verdict is the worst condition the findings prove: unusable > counterfeit > expired.
func (r *ruleCheck) verdict() string {
	rank := map[string]int{"expired": 1, "counterfeit": 2, "opened": 2, "unusable": 3}
	v := ""
	for _, f := range r.Findings {
		if rank[f.Verdict] > rank[v] {
			v = f.Verdict
		}
	}
	return v
}

// checkInspect evaluates the visible cues against the rules. It never sees hidden values.
func checkInspect(c *inspectCtx) *ruleCheck {
	r := &ruleCheck{Clear: map[string]bool{}}
	switch c.Item.Kind {
	case kindCigarettes:
		checkCigarettes(c, r)
	case kindStamp:
		checkStamp(c, r)
	case kindInjector:
		checkInjector(c, r)
	case kindRibwich, kindHand, kindPlasma:
		r.Value = valueAppraisal(c, r)
	default:
		r.unknown("item kind " + strconv.Quote(c.Item.Kind) + " has no rules")
	}
	return r
}

func (c *inspectCtx) visual(r *ruleCheck, cue string) (string, bool) {
	v, ok := c.Visual[cue]
	if !ok || v == "" {
		r.unknown(cue + " not shown")
		return "", false
	}
	return v, true
}

func (c *inspectCtx) text(r *ruleCheck, field string) (string, bool) {
	v, ok := c.Text[field]
	if !ok {
		r.unknown(field + " text not shown")
	}
	return v, ok
}

// referenceCue checks a sprite cue that must match the reference item exactly.
func referenceCue(c *inspectCtx, r *ruleCheck, cue, issue, label string) {
	v, ok := c.visual(r, cue)
	if !ok {
		return
	}
	if v == cueMatches {
		r.pass(issue, label+" matches the reference")
		return
	}
	r.fail(issue, "counterfeit", label+": "+v+" (a genuine one matches the reference exactly)")
}

func checkCigarettes(c *inspectCtx, r *ruleCheck) {
	referenceCue(c, r, "seal", "seal", "seal")
	referenceCue(c, r, "lettering", "lettering", "lettering")
	referenceCue(c, r, "logo", "logo", "logo")
	if v, ok := c.visual(r, "box_colour"); ok {
		if v == "matches colour card" {
			r.pass("color", "box colour matches the colour card")
		} else {
			r.fail("color", "counterfeit", "box colour is "+v+" (genuine packs match the colour card)")
		}
	}
	// Only a scripted story pack carries a box issue, and it has every other issue too.
	if len(r.Findings) == 0 && r.certain() {
		r.Clear["box"] = true
	}
}

func checkStamp(c *inspectCtx, r *ruleCheck) {
	amount, amountOK := 0, false
	if s, ok := c.text(r, "amount"); ok {
		if n, err := leadingInt(s); err == nil {
			amount, amountOK = n, true
			if amount%stampDenom != 0 {
				r.fail("amount", "counterfeit", fmt.Sprintf("printed amount %d is not a multiple of %d", amount, stampDenom))
			} else {
				r.pass("amount", fmt.Sprintf("amount %d is a multiple of %d", amount, stampDenom))
			}
		} else {
			r.unknown("amount " + strconv.Quote(s) + " is not a number")
		}
	}
	if v, ok := c.visual(r, "seal"); ok {
		switch {
		case !amountOK:
			r.unknown("seal rule needs the amount")
		case (v == cuePresent) != (amount >= stampSealFrom):
			want := "no seal"
			if amount >= stampSealFrom {
				want = "a seal"
			}
			r.fail("seal", "counterfeit", fmt.Sprintf("seal %s on a %d stamp (genuine stamps carry a seal exactly when the amount is %d or more, so it needs %s)", v, amount, stampSealFrom, want))
		default:
			r.pass("seal", fmt.Sprintf("seal %s fits amount %d", v, amount))
		}
	}
	referenceCue(c, r, "lettering", "lettering", "lettering")
	referenceCue(c, r, "logo", "logo", "logo")
	referenceCue(c, r, "chip", "chip", "chip")
	if f, ok := r.finding("chip"); ok {
		f.Why += "; the stamp guide calls a missing or damaged chip unusable, and the game exposes it as a chip issue"
		for i := range r.Findings {
			if r.Findings[i].Issue == "chip" {
				r.Findings[i] = f
			}
		}
	}
}

func checkInjector(c *inspectCtx, r *ruleCheck) {
	company, okC := c.text(r, "company")
	model, okM := c.text(r, "model")
	switch {
	case okC && company != injectorCompany:
		r.fail("label", "counterfeit", fmt.Sprintf("company %q is not %q", company, injectorCompany))
	case okM && model != injectorModel:
		r.fail("label", "counterfeit", fmt.Sprintf("model %q is not %q", model, injectorModel))
	case okC && okM:
		r.pass("label", "company and model are spelled exactly right")
	}
	if serial, ok := c.text(r, "serial"); ok {
		if why, verdict := serialProblem(serial); why != "" {
			r.fail("serial", verdict, why)
		} else {
			r.pass("serial", fmt.Sprintf("serial %s: 7 digits, prefix %s in 125-455, not recalled", serial, serial[:3]))
		}
	}
	if v, ok := c.visual(r, "indicator"); ok {
		switch v {
		case "normal":
			r.pass("indicator", "indicator normal")
		case "expired warning":
			r.fail("indicator", "expired", "indicator shows the expired warning")
		case "fault warning":
			r.fail("indicator", "unusable", "indicator shows the fault warning")
		default:
			r.unknown("indicator " + strconv.Quote(v))
		}
	}
	if v, ok := c.visual(r, "body"); ok {
		if v == "intact" {
			r.pass("body", "body intact")
		} else {
			r.fail("body", "unusable", "body is "+v)
		}
	}
	if v, ok := c.visual(r, "body_colour"); ok {
		if v == "standard" {
			r.pass("color", "body colour standard")
		} else {
			r.fail("color", "counterfeit", "body colour is "+v+" (genuine injectors are the standard colour)")
		}
	}
}

// serialProblem says what is wrong with an injector serial and what it makes the injector.
func serialProblem(s string) (why, verdict string) {
	if s == "" {
		return "no serial printed", "counterfeit"
	}
	if strings.Trim(s, "0123456789") != "" {
		return fmt.Sprintf("serial %q is not all digits", s), "counterfeit"
	}
	if len(s) != 7 {
		return fmt.Sprintf("serial %s has %d digits, not 7", s, len(s)), "counterfeit"
	}
	prefix, _ := strconv.Atoi(s[:3])
	suffix, _ := strconv.Atoi(s[3:])
	if prefix < 125 || prefix > 455 {
		return fmt.Sprintf("serial %s: prefix %d is outside 125-455", s, prefix), "counterfeit"
	}
	if suffix < 1000 {
		return fmt.Sprintf("serial %s: last four digits %s are under 1000", s, s[3:]), "counterfeit"
	}
	if slices.Contains(recalledPrefixes, prefix) {
		return fmt.Sprintf("serial %s: prefix %d is recalled", s, prefix), "unusable"
	}
	return "", ""
}

// leadingInt reads the first run of digits in s ("120 credits" -> 120).
func leadingInt(s string) (int, error) {
	start := strings.IndexAny(s, "0123456789")
	if start < 0 {
		return 0, fmt.Errorf("no digits in %q", s)
	}
	end := start
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	return strconv.Atoi(s[start:end])
}

func (v *valuation) add(n int64, why string) {
	if n == 0 {
		return
	}
	v.Value += n
	v.Lines = append(v.Lines, fmt.Sprintf("%+d %s", n, why))
}

func (v *valuation) floor(min int64) *valuation {
	if v.Value < min {
		v.Lines = append(v.Lines, fmt.Sprintf("floor %d", min))
		v.Value = min
	}
	return v
}

// parseRibwichDate reads "M/ YYY" (the barcode prints month, slash, year).
func parseRibwichDate(s string) (month, year int, err error) {
	m, y, ok := strings.Cut(s, "/")
	if !ok {
		return 0, 0, fmt.Errorf("no slash in %q", s)
	}
	if month, err = strconv.Atoi(strings.TrimSpace(m)); err != nil {
		return 0, 0, err
	}
	year, err = strconv.Atoi(strings.TrimSpace(y))
	return month, year, err
}

// Grades as the game computes them (integer percentages of the true value).
var gradeNames = []string{"perfect", "excellent", "good", "mediocre", "awful"}

func pct(v, p int64) int64 { return v * p / 100 }

func estimateGrade(truth, est int64) string {
	if est >= pct(truth, 95) && est <= pct(truth, 105) {
		return "perfect"
	}
	if est < pct(truth, 95) {
		switch {
		case est <= pct(truth, 75):
			return "awful"
		case est <= pct(truth, 85):
			return "mediocre"
		case est <= pct(truth, 90):
			return "good"
		}
		return "excellent"
	}
	switch {
	case est >= pct(truth, 125):
		return "awful"
	case est >= pct(truth, 115):
		return "mediocre"
	case est >= pct(truth, 110):
		return "good"
	}
	return "excellent"
}

// estimateRefused says whether the game refuses est for an item worth truth, and why.
func estimateRefused(c *inspectCtx, truth, est int64) string {
	if est < pct(truth, 50) {
		return "too low (under 50% of the true value)"
	}
	if est > pct(truth, 150) {
		return "too high (over 150% of the true value)"
	}
	g := estimateGrade(truth, est)
	if c.Appraisal != nil && c.Appraisal.FeePct != nil {
		if p, ok := c.Appraisal.FeePct[g]; ok && p <= 0 {
			return "a " + g + " grade pays nothing at this rank, so it is refused"
		}
	}
	return ""
}

func inspectOptions(req *protocol.DecideRequest) map[string]protocol.Option {
	m := map[string]protocol.Option{}
	for _, o := range req.Options {
		m[o.ID] = o
	}
	return m
}

// inspectNotes gives each option what the rule check says of it.
func inspectNotes(c *inspectCtx) func(id, label string) string {
	r := checkInspect(c)
	return func(id, _ string) string {
		switch {
		case strings.HasPrefix(id, confrontPrefix):
			issue := strings.TrimPrefix(id, confrontPrefix)
			if f, ok := r.finding(issue); ok {
				return "RIGHT by the rules: " + f.Why
			}
			if r.Clear[issue] {
				return "wrong by the rules: no " + issue + " problem visible (-2 rep)"
			}
			return "unproven: nothing visible shows a " + issue + " problem"
		case id == optGenuine && c.Claim == "counterfeit":
			if len(r.Findings) > 0 {
				return "they will deny it (no penalty): " + r.Findings[0].Why
			}
			if r.certain() {
				return "RIGHT by the rules: every cue matches a genuine item (+2 rep)"
			}
			return "no fake tell visible, but " + strings.Join(r.Unknown, "; ")
		case id == optGenuine:
			if len(r.Findings) > 0 {
				return "wrong by the rules: " + r.Findings[0].Why
			}
			if r.certain() {
				return "RIGHT by the rules: every cue matches a genuine item"
			}
			return "no fake tell visible, but " + strings.Join(r.Unknown, "; ")
		case id == optCounterfeit:
			if len(r.Findings) > 0 {
				return "RIGHT by the rules: " + r.Findings[0].Why
			}
			if r.certain() {
				return "gives up +2 rep: every cue matches a genuine item"
			}
		case id == optEstimate:
			if r.Value == nil || !r.certain() {
				return ""
			}
			v := r.Value.Value
			return fmt.Sprintf("true value %d by the rules; perfect %d-%d; refused under %d or over %d", v, pct(v, 95), pct(v, 105), pct(v, 50), pct(v, 150))
		}
		return ""
	}
}

func renderInspect(b *strings.Builder, c *inspectCtx) {
	it := c.Item
	name := it.Name
	if name == "" {
		name = it.ID
	}
	fmt.Fprintf(b, "Inspecting %s (%s", name, strings.ReplaceAll(it.Kind, "_", " "))
	if it.UID != nil {
		fmt.Fprintf(b, " #%d", *it.UID)
	}
	b.WriteString(")")
	if it.Price != nil {
		fmt.Fprintf(b, ", price now %d", *it.Price)
	}
	if cl := c.Client; cl != nil {
		fmt.Fprintf(b, ", from %s", cl.Name)
		var tags []string
		if cl.Faction != "" {
			tags = append(tags, cl.Faction)
		}
		if cl.Intent != "" {
			tags = append(tags, cl.Intent)
		}
		if len(tags) > 0 {
			fmt.Fprintf(b, " (%s)", strings.Join(tags, ", "))
		}
	}
	b.WriteString(".\n")
	switch c.Mode {
	case modeAppraisal:
		b.WriteString("The customer pays you to APPRAISE it: submit one estimate.\n")
	default:
		fmt.Fprintf(b, "They claim it is %s.\n", strings.ToUpper(nonEmptyStr(c.Claim, "unknown")))
	}
	if len(c.Text) > 0 {
		b.WriteString("Printed text (verbatim):\n")
		for _, k := range sortedKeys(c.Text) {
			fmt.Fprintf(b, "- %s: %q\n", strings.ReplaceAll(k, "_", " "), c.Text[k])
		}
	}
	if len(c.Visual) > 0 {
		b.WriteString("What you see:\n")
		for _, k := range sortedKeys(c.Visual) {
			fmt.Fprintf(b, "- %s: %s\n", strings.ReplaceAll(k, "_", " "), c.Visual[k])
		}
	}
	if rep := c.Reputation; rep != nil && rep.Faction != "" && c.Mode != modeAppraisal {
		fmt.Fprintf(b, "Reputation with %s", rep.Faction)
		if rep.Value != nil {
			fmt.Fprintf(b, ": %g", *rep.Value)
		}
		if rep.Status != "" {
			fmt.Fprintf(b, " (%s)", rep.Status)
		}
		b.WriteString("; each wrong confront costs 2 here.\n")
	}
	if a := c.Appraisal; a != nil {
		renderAppraisal(b, a)
	}
	r := checkInspect(c)
	b.WriteString("RULE CHECK (deterministic, from the cues above):\n")
	for _, f := range r.Findings {
		b.WriteString("- FAIL: " + f.Why)
		if f.Issue != "" {
			b.WriteString(" -> " + confrontPrefix + f.Issue)
		}
		b.WriteString("\n")
	}
	for _, p := range r.Passed {
		b.WriteString("- ok: " + p + "\n")
	}
	for _, u := range r.Unknown {
		b.WriteString("- unknown: " + u + "\n")
	}
	if r.Value != nil {
		b.WriteString("Value: " + strings.Join(r.Value.Lines, ", ") + fmt.Sprintf(" = %d", r.Value.Value) + "\n")
	}
	switch v := r.verdict(); {
	case c.Mode == modeAppraisal:
	case v != "":
		fmt.Fprintf(b, "Rule verdict: %s.\n", v)
	case r.certain():
		b.WriteString("Rule verdict: genuine (no rule broken, every cue read).\n")
	default:
		b.WriteString("Rule verdict: no rule broken, but some cues are unknown.\n")
	}
}

func renderAppraisal(b *strings.Builder, a *appraisalTerms) {
	b.WriteString("Appraisal terms:")
	if a.TheirFigure != nil {
		fmt.Fprintf(b, " their own figure %d.", *a.TheirFigure)
	}
	if a.Rank != "" {
		fmt.Fprintf(b, " Your rank: %s.", a.Rank)
	}
	if a.BaseFee != nil {
		fmt.Fprintf(b, " Fee = %d", *a.BaseFee)
		if len(a.FeePct) > 0 {
			var ps []string
			for _, g := range gradeNames {
				if p, ok := a.FeePct[g]; ok {
					ps = append(ps, fmt.Sprintf("%s %d%%", g, p))
				}
			}
			b.WriteString(" + a grade share of the estimate (" + strings.Join(ps, ", ") + "; 0% means the estimate is refused)")
		}
		b.WriteString(".")
	}
	if a.Refusals != nil {
		fmt.Fprintf(b, " Refusals so far: %d", *a.Refusals)
		if a.LastEstimate != nil && a.LastFeedback != "" {
			fmt.Fprintf(b, " (last estimate %d was %s)", *a.LastEstimate, strings.ReplaceAll(a.LastFeedback, "_", " "))
		}
		b.WriteString("; about three refusals and they walk out.")
	}
	b.WriteString("\n")
}

func nonEmptyStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// inspectHeuristic follows the rule check: confront the issue a broken rule proves, take a
// claim no rule contradicts, bid the top of the perfect band on an appraisal.
func inspectHeuristic(req *protocol.DecideRequest) Decision {
	c, err := parseInspect(req.Context)
	if err != nil {
		return heuristicDefer("inspect context does not have the expected shape")
	}
	r := checkInspect(c)
	opts := inspectOptions(req)
	switch c.Mode {
	case modeAppraisal:
		return appraisalHeuristic(c, r, opts)
	case modeAuthenticity:
	default:
		return heuristicDefer("unknown inspect mode " + strconv.Quote(c.Mode))
	}
	if len(r.Findings) > 0 {
		f := r.Findings[0]
		if c.Claim == "counterfeit" {
			if _, ok := opts[optCounterfeit]; ok {
				return choose(optCounterfeit, "Agree it is fake: "+f.Why+".")
			}
			return heuristicDefer("a fake tell is visible but agreeing is not offered")
		}
		for _, f := range r.Findings {
			if _, ok := opts[confrontPrefix+f.Issue]; ok && f.Issue != "" {
				return choose(confrontPrefix+f.Issue, fmt.Sprintf("Looks %s: %s.", f.Verdict, f.Why))
			}
		}
		return heuristicDefer("the broken rule has no matching confront: " + f.Why)
	}
	if !r.certain() {
		return heuristicDefer("some cues are unreadable: " + strings.Join(r.Unknown, "; "))
	}
	if _, ok := opts[optGenuine]; !ok {
		return heuristicDefer("the item looks genuine but taking the claim is not offered")
	}
	if c.Claim == "counterfeit" {
		return choose(optGenuine, "Every cue matches a genuine item: tell them, for +2 rep.")
	}
	return choose(optGenuine, "Every cue matches a genuine item; a confront would cost 2 rep.")
}

func appraisalHeuristic(c *inspectCtx, r *ruleCheck, opts map[string]protocol.Option) Decision {
	o, ok := opts[optEstimate]
	if !ok {
		return heuristicDefer("no estimate option is offered")
	}
	p, ok := o.Params[estimateParam]
	if !ok || p.Min == nil || p.Max == nil {
		return heuristicDefer("the estimate option has no bounded value")
	}
	if r.Value == nil || !r.certain() {
		return heuristicDefer("the value cannot be computed: " + strings.Join(r.Unknown, "; "))
	}
	v := r.Value.Value
	// The fee is a share of the estimate, and the top of the perfect band still grades perfect.
	est := min(max(pct(v, 105), *p.Min), *p.Max)
	if a := c.Appraisal; a != nil && a.LastEstimate != nil && *a.LastEstimate == est && a.LastFeedback != "" {
		return heuristicDefer(fmt.Sprintf("the rules give %d, but %d was already refused as %s", v, est, strings.ReplaceAll(a.LastFeedback, "_", " ")))
	}
	if why := estimateRefused(c, v, est); why != "" {
		return heuristicDefer(fmt.Sprintf("the rule value %d would be refused: %s", v, why))
	}
	d := choose(optEstimate, fmt.Sprintf("Worth %d by the rules (%s); %d is the top of the perfect band.", v, strings.Join(r.Value.Lines, ", "), est))
	d.Choice.Params = map[string]json.RawMessage{estimateParam: json.RawMessage(strconv.FormatInt(est, 10))}
	return d
}

// checkInspectChoice refuses what the visible cues prove wrong: a confront of an issue whose
// cues were all read and are fine (a certain -2 rep), and an estimate the game would refuse
// for the value the visible cues fix.
func checkInspectChoice(req *protocol.DecideRequest, choice protocol.Choice) error {
	if choice.Option == protocol.OptionDefer {
		return nil
	}
	c, err := parseInspect(req.Context)
	if err != nil {
		return errors.New("the inspect context does not parse, so the choice cannot be checked")
	}
	r := checkInspect(c)
	switch {
	case strings.HasPrefix(choice.Option, confrontPrefix):
		issue := strings.TrimPrefix(choice.Option, confrontPrefix)
		if _, ok := r.finding(issue); !ok && r.Clear[issue] {
			return fmt.Errorf("option %q is certainly wrong: the %s cues are visible and break no rule, so it costs 2 rep; pick the issue a FAIL line names, or %q", choice.Option, issue, optGenuine)
		}
	case choice.Option == optEstimate:
		if r.Value == nil || !r.certain() {
			return nil
		}
		raw, ok := choice.Params[estimateParam]
		if !ok {
			return nil
		}
		est, err := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			return nil
		}
		if why := estimateRefused(c, r.Value.Value, est); why != "" {
			return fmt.Errorf("estimate %d would be refused: %s; the rules give %d, perfect is %d-%d", est, why, r.Value.Value, pct(r.Value.Value, 95), pct(r.Value.Value, 105))
		}
	}
	return nil
}

// buildInspectSystem states the rules from docs/re/INSPECT.md in our own words, and the
// appraisal value tables as loaded (t nil: not loaded).
func buildInspectSystem(t appraisalTables) string {
	return strings.TrimSpace(`
You advise the owner of a pawnshop in the cyberpunk game "Probably Stolen". The player is looking at a customer's item under the magnifier. You perceive like an expert player: the prompt lists every printed text verbatim and every visual tell in words, and you know all of the game's rules below. Pick one listed option.

AUTHENTICITY (cigarettes, food stamps, injectors). The customer claims the item is genuine or counterfeit.
- Claimed genuine: confront ONE issue, or accept the claim ("genuine"). A confront is right only if the item really has that issue: then it is exposed, its price drops to its true grade, and you get a further 15% off. A wrong confront costs 2 reputation with the customer's faction (Lower Levels for factions without a bar) and you may try again, paying 2 each time. Accepting a fake means paying the genuine price; buyers may later catch it (-1 rep, sale blocked).
- Claimed counterfeit: "genuine" tells them it is really genuine: if right, +2 rep with this customer and the price rises to the genuine price less 15%; if wrong they just deny it, no penalty. "counterfeit" leaves it as claimed.
- One visible problem is enough: genuine items never show any of the tells below.

CIGARETTES: a genuine pack matches the reference pack and colour card exactly: seal, lettering, logo and box colour. A missing or broken seal, or any wrong, misprinted or missing element, is a fake; confront that element (seal, lettering, logo; an off-shade box is the color issue). The box issue never applies to ordinary packs. Genuine 100, counterfeit 50.

FOOD STAMPS: real stamps come in multiples of 40 (40, 80, 120, 160, 200; the guide also lists 240, which is legal but never seen). A printed amount that is not a multiple of 40 is fake (amount issue). A genuine stamp has a seal exactly when its amount is 160 or more; a seal on a smaller stamp, or none on a larger one, is fake (seal issue). Lettering, logo and chip must match the reference; otherwise confront that one (the guide calls a missing or damaged chip "unusable"; in play it is the chip issue). A fake is worth 8 once exposed; a genuine one 0.75 x its amount.

INJECTORS (Immunivax): company must read exactly "Carmicheal Pharmaceutical" and model exactly "Immunivax" (any misspelling, extra word or blank is a counterfeit label). The serial is exactly 7 digits: a prefix 125-455, then 4 digits 1000-9999; 6 or 8 digits, a prefix outside 125-455, or no serial is a counterfeit serial. Prefixes 250, 251, 301, 355, 385, 386 and 421 are recalled: unusable (serial issue). The indicator shows normal, an expired warning (expired: indicator issue) or a fault warning (unusable: indicator issue). A cracked body is unusable (body issue); an off-colour body is counterfeit (color issue). There is no printed expiry date. Exposed values: genuine 160, expired 80, counterfeit 40, unusable 8.

APPRAISAL SERVICE (prosthetic hands, Ribwich packs, plasma cells): the customer pays a fee for an estimate of the true value, which the appraisal guides' value tables fix from the visible cues:
` + renderGuides(t) + `- Grading against the true value: 95-105% perfect; 90-95% or 105-110% excellent; 85-90% or 110-115% good; 75-85% or 115-125% mediocre; worse is awful. Under 50% or over 150% is refused ("too low"/"too high"), as is a grade your rank pays 0% for. Refusals cost appraiser progress and patience: about the third makes them walk out. Accepted: fee = rank base fee + grade % of YOUR ESTIMATE, so the top of the perfect band pays most.

THE RULE CHECK in the prompt applies these rules to the cues mechanically. Trust its FAIL lines and its value unless a cue it relies on is marked unknown; explain the decisive rule in the rationale. Confront the issue a FAIL line names. Never confront an issue whose cues are fine. Defer only when a needed cue is unknown.
` + answerRules + `
- For "estimate" put the integer in params: {"value": N}.`)
}
