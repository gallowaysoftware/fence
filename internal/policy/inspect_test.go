package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// The tests value appraisals with a fictional table (the real one is not committed).
const fixtureGuides = "testdata/appraisal_guides.fixture.json"

func TestMain(m *testing.M) {
	raw, err := ReadAppraisalGuides(fixtureGuides)
	if err == nil && raw == nil {
		err = errors.New(fixtureGuides + " is missing")
	}
	if err == nil {
		err = UseAppraisalGuides(raw)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestInspectFixtureResponse(t *testing.T) {
	b, err := os.ReadFile("../../protocol/fixtures/decide.inspect.response.json")
	if err != nil {
		t.Fatal(err)
	}
	var want protocol.DecideResponse
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	req := fixtureNamed(t, "decide.inspect.request.json")
	d := Heuristic{}.Decide(context.Background(), req)
	got := protocol.DecideResponse{DecisionID: req.DecisionID, Choice: d.Choice, Rationale: d.Rationale, Source: d.Source, OptionNotes: d.Notes}
	if a, b := mustJSON(t, got), mustJSON(t, want); string(a) != string(b) {
		t.Fatalf("heuristic answer differs from decide.inspect.response.json:\ngot  %s\nwant %s", a, b)
	}
}

// genuineCues are the words a genuine item of each kind shows; tests change one at a time.
var genuineCues = map[string]struct{ text, visual map[string]string }{
	kindCigarettes: {nil, map[string]string{"seal": cueMatches, "lettering": cueMatches, "logo": cueMatches, "box_colour": "matches colour card"}},
	kindStamp:      {map[string]string{"amount": "120"}, map[string]string{"seal": cueMissing, "lettering": cueMatches, "logo": cueMatches, "chip": cueMatches}},
	kindInjector: {map[string]string{"serial": "3127764", "company": injectorCompany, "model": injectorModel},
		map[string]string{"indicator": "normal", "body": "intact", "body_colour": "standard"}},
	kindRibwich: {map[string]string{"production_date": "7/ 622", "facility": "#12"}, map[string]string{"meat": "single", "top_bun": "regular", "pickles": "regular",
		"wrap": "sealed", "davre_logo": cueMatches, "ribwich_logo": cuePresent, "barcode_stamp": cuePresent, "facility_stamp": cuePresent}},
	kindHand: {nil, map[string]string{"material": "aluminium alloy", "interface_marking": "EMG", "marking_style": "full label", "condition": "pristine",
		"armor_plating": "none", "blade": "none"}},
	kindPlasma: {nil, map[string]string{"colour": "standard", "seal_brand": "none", "casing_wear": "none", "crack": "none", "screw": "covered", "reinforcement": "no"}},
}

// cues builds a context of kind with the genuine cues overridden by "visual.key=value" or
// "text.key=value" edits ("text.key=" sets an empty text; "-text.key" removes it).
func cues(kind, claim string, edits ...string) *inspectCtx {
	g := genuineCues[kind]
	c := &inspectCtx{Mode: modeAuthenticity, Claim: claim, Item: &inspectItem{Kind: kind, Name: kind},
		Text: maps.Clone(g.text), Visual: maps.Clone(g.visual)}
	if c.Text == nil {
		c.Text = map[string]string{}
	}
	if kind == kindRibwich || kind == kindHand || kind == kindPlasma {
		c.Mode = modeAppraisal
	}
	for _, e := range edits {
		if del, ok := strings.CutPrefix(e, "-"); ok {
			where, key, _ := strings.Cut(del, ".")
			if where == "text" {
				delete(c.Text, key)
			} else {
				delete(c.Visual, key)
			}
			continue
		}
		kv, val, _ := strings.Cut(e, "=")
		where, key, _ := strings.Cut(kv, ".")
		if where == "text" {
			c.Text[key] = val
		} else {
			c.Visual[key] = val
		}
	}
	return c
}

func issues(r *ruleCheck) []string {
	var out []string
	for _, f := range r.Findings {
		out = append(out, f.Issue+":"+f.Verdict)
	}
	return out
}

func TestInspectRuleCheck(t *testing.T) {
	tests := []struct {
		name string
		c    *inspectCtx
		want []string // issue:verdict per finding, in order
	}{
		{"genuine cigarettes", cues(kindCigarettes, "genuine"), nil},
		{"cigarette seal missing", cues(kindCigarettes, "genuine", "visual.seal=missing"), []string{"seal:counterfeit"}},
		{"cigarette misprint and off-shade", cues(kindCigarettes, "genuine", "visual.lettering=misprinted", "visual.box_colour=off-shade"), []string{"lettering:counterfeit", "color:counterfeit"}},
		{"stamp 120 without seal", cues(kindStamp, "genuine"), nil},
		{"stamp 200 with seal", cues(kindStamp, "genuine", "text.amount=200", "visual.seal=present"), nil},
		{"stamp 160 without seal", cues(kindStamp, "genuine", "text.amount=160"), []string{"seal:counterfeit"}},
		{"stamp 80 with seal", cues(kindStamp, "genuine", "text.amount=80", "visual.seal=present"), []string{"seal:counterfeit"}},
		{"stamp amount 130", cues(kindStamp, "genuine", "text.amount=130 credits"), []string{"amount:counterfeit"}},
		{"stamp amount 170 with its seal", cues(kindStamp, "genuine", "text.amount=170", "visual.seal=present"), []string{"amount:counterfeit"}},
		{"stamp wrong chip", cues(kindStamp, "genuine", "visual.chip=wrong chip"), []string{"chip:counterfeit"}},
		{"stamp 240 with seal (in the guide, never generated)", cues(kindStamp, "genuine", "text.amount=240", "visual.seal=present"), nil},
		{"cigarette seal broken", cues(kindCigarettes, "genuine", "visual.seal=broken"), []string{"seal:counterfeit"}},
		{"genuine injector", cues(kindInjector, "genuine"), nil},
		{"genuine injector at the prefix edges", cues(kindInjector, "genuine", "text.serial=4559999"), nil},
		{"genuine injector low edge", cues(kindInjector, "genuine", "text.serial=1251000"), nil},
		{"recalled prefix", cues(kindInjector, "genuine", "text.serial=2514471"), []string{"serial:unusable"}},
		{"recalled 421", cues(kindInjector, "genuine", "text.serial=4211234"), []string{"serial:unusable"}},
		{"six-digit serial", cues(kindInjector, "genuine", "text.serial=312776"), []string{"serial:counterfeit"}},
		{"eight-digit serial", cues(kindInjector, "genuine", "text.serial=31277645"), []string{"serial:counterfeit"}},
		{"prefix 124", cues(kindInjector, "genuine", "text.serial=1249999"), []string{"serial:counterfeit"}},
		{"prefix 456", cues(kindInjector, "genuine", "text.serial=4561000"), []string{"serial:counterfeit"}},
		{"no serial", cues(kindInjector, "genuine", "text.serial="), []string{"serial:counterfeit"}},
		{"misspelt model", cues(kindInjector, "genuine", "text.model=Imunivax"), []string{"label:counterfeit"}},
		{"blank company", cues(kindInjector, "genuine", "text.company="), []string{"label:counterfeit"}},
		{"company with Ltd.", cues(kindInjector, "genuine", "text.company=Carmicheal Pharmaceutical Ltd."), []string{"label:counterfeit"}},
		{"expired indicator", cues(kindInjector, "genuine", "visual.indicator=expired warning"), []string{"indicator:expired"}},
		{"fault indicator", cues(kindInjector, "genuine", "visual.indicator=fault warning"), []string{"indicator:unusable"}},
		{"cracked body", cues(kindInjector, "genuine", "visual.body=cracked"), []string{"body:unusable"}},
		{"off-colour body", cues(kindInjector, "genuine", "visual.body_colour=off-colour"), []string{"color:counterfeit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkInspect(tt.c)
			if got := issues(r); !slices.Equal(got, tt.want) {
				t.Fatalf("findings %v, want %v (passed %v, unknown %v)", got, tt.want, r.Passed, r.Unknown)
			}
			if !r.certain() {
				t.Fatalf("unknown cues: %v", r.Unknown)
			}
		})
	}
}

func TestInspectUnknownCuesAreNotCertain(t *testing.T) {
	for _, c := range []*inspectCtx{
		cues(kindInjector, "genuine", "-text.serial"),
		cues(kindInjector, "genuine", "visual.indicator=blank"),
		cues(kindStamp, "genuine", "-text.amount"),
		cues(kindCigarettes, "genuine", "-visual.logo"),
	} {
		r := checkInspect(c)
		if r.certain() || len(r.Findings) > 0 {
			t.Errorf("%v %v: findings %v unknown %v", c.Text, c.Visual, issues(r), r.Unknown)
		}
		if d := inspectHeuristic(inspectReq(t, c)); d.Choice.Option != protocol.OptionDefer {
			t.Errorf("heuristic chose %s on unknown cues", d.Choice.Option)
		}
	}
}

func TestInspectValues(t *testing.T) {
	tests := []struct {
		name string
		c    *inspectCtx
		want int64
	}{
		{"plain ribwich, second run, facility 12", cues(kindRibwich, ""), 2000},
		{"double meat first run Jan 610 facility 3", cues(kindRibwich, "", "visual.meat=double", "visual.top_bun=hidden under double meat", "visual.pickles=hidden under double meat",
			"text.production_date=1/ 610", "visual.ribwich_logo=absent", "text.facility=#3"), 2000 + 401 + 17 + 29 + 11},
		{"deluxe Dec 626 facility 9", cues(kindRibwich, "", "visual.top_bun=deluxe", "visual.pickles=extra", "text.production_date=12/ 626", "text.facility=#9"), 2000 + 203 - 13},
		{"opened", cues(kindRibwich, "", "visual.wrap=opened", "visual.meat=double"), 300},
		{"year between the runs", cues(kindRibwich, "", "text.production_date=3/ 617"), 300},
		{"month 13", cues(kindRibwich, "", "text.production_date=13/ 622"), 300},
		{"facility 40", cues(kindRibwich, "", "text.facility=#40"), 300},
		{"wrong Davre logo", cues(kindRibwich, "", "visual.davre_logo=wrong logo"), 300},
		{"no barcode stamp", cues(kindRibwich, "", "visual.barcode_stamp=missing", "-text.production_date"), 300},
		{"no facility stamp", cues(kindRibwich, "", "visual.facility_stamp=missing", "-text.facility"), 300},
		{"deluxe bun, plain pickles", cues(kindRibwich, "", "visual.top_bun=deluxe"), 300},
		{"first run with a Ribwich logo", cues(kindRibwich, "", "text.production_date=5/ 612"), 300},
		{"second run without one", cues(kindRibwich, "", "visual.ribwich_logo=absent"), 300},
		{"plain hand", cues(kindHand, ""), 1000},
		{"junk hand floors", cues(kindHand, "", "visual.interface_marking=none", "-visual.marking_style", "visual.condition=damaged"), 980},
		{"combo A", cues(kindHand, "", "visual.material=carbon composite", "visual.interface_marking=Neural", "visual.armor_plating=fitted", "visual.blade=fitted"), 1000 + 7 + 13 + 19 + 23 + 29},
		{"combo B", cues(kindHand, "", "visual.material=gold plated", "visual.interface_marking=Neural", "visual.marking_style=symbol"), 1000 + 11 + 13 + 31},
		{"worn gold EMG+ is no combo", cues(kindHand, "", "visual.material=gold plated", "visual.interface_marking=EMG+", "visual.condition=worn"), 1000 + 11 + 5 - 3},
		{"plain cell", cues(kindPlasma, ""), 3000},
		{"seal D on an unreinforced high cell", cues(kindPlasma, "", "visual.colour=high quality", "visual.seal_brand=Ignition Lab"), 3000 + 32 + 7},
		{"seal D needs unreinforced", cues(kindPlasma, "", "visual.colour=high quality", "visual.seal_brand=Ignition Lab", "visual.reinforcement=reinforced"), 3000 + 32 + 6},
		{"seal A on poor", cues(kindPlasma, "", "visual.colour=poor", "visual.seal_brand=LL"), 3000 - 96 + 1},
		{"seal A needs poor", cues(kindPlasma, "", "visual.colour=high quality", "visual.seal_brand=LL"), 3032},
		{"only the first seal rule of the group applies", cues(kindPlasma, "", "visual.seal_brand=Zhonghua", "visual.screw=uncovered"), 3000 - 2 + 3},
		{"the group falls through to a later rule", cues(kindPlasma, "", "visual.seal_brand=Zhonghua", "visual.crack=cracked"), 3000 - 16 + 5},
		{"wrecked cell floors", cues(kindPlasma, "", "visual.colour=poor", "visual.crack=cracked", "visual.casing_wear=heavily worn"), 2900},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := checkInspect(tt.c)
			if r.Value == nil {
				t.Fatal("no value")
			}
			if r.Value.Value != tt.want || !r.certain() {
				t.Fatalf("value %d (%v), want %d; unknown %v", r.Value.Value, r.Value.Lines, tt.want, r.Unknown)
			}
		})
	}
}

// TestInspectCueVocabulary keeps the checker in step with the mod's cue dictionary: every word
// the mod can send for a cue is one the checker reads (testdata/inspect_cues.json is checked
// on the C# side against the dictionary).
func TestInspectCueVocabulary(t *testing.T) {
	b, err := os.ReadFile("testdata/inspect_cues.json")
	if err != nil {
		t.Fatal(err)
	}
	var vocab map[string]struct {
		Text   []string            `json:"text"`
		Visual map[string][]string `json:"visual"`
	}
	if err := json.Unmarshal(b, &vocab); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(slices.Sorted(maps.Keys(vocab)), slices.Sorted(maps.Keys(genuineCues))) {
		t.Fatalf("kinds %v, checker knows %v", slices.Sorted(maps.Keys(vocab)), slices.Sorted(maps.Keys(genuineCues)))
	}
	// Words that say the cue cannot be read.
	unreadable := map[string]bool{"blank": true, "unrecognised": true}
	for kind, v := range vocab {
		g := genuineCues[kind]
		if !slices.Equal(slices.Sorted(maps.Keys(v.Visual)), slices.Sorted(maps.Keys(g.visual))) {
			t.Errorf("%s visual cues %v, checker's genuine set %v", kind, slices.Sorted(maps.Keys(v.Visual)), slices.Sorted(maps.Keys(g.visual)))
		}
		if !slices.Equal(slices.Sorted(slices.Values(v.Text)), slices.Sorted(maps.Keys(g.text))) {
			t.Errorf("%s text fields %v, checker's genuine set %v", kind, v.Text, slices.Sorted(maps.Keys(g.text)))
		}
		base := checkInspect(cues(kind, "genuine"))
		for cue, words := range v.Visual {
			if !slices.Contains(words, g.visual[cue]) {
				t.Errorf("%s.%s: genuine word %q not in the vocabulary %v", kind, cue, g.visual[cue], words)
			}
			for _, w := range words {
				r := checkInspect(cues(kind, "genuine", "visual."+cue+"="+w))
				if !r.certain() && !unreadable[w] {
					t.Errorf("%s.%s=%q: checker cannot read it: %v", kind, cue, w, r.Unknown)
				}
				changed := len(r.Findings) != len(base.Findings) || r.Value != nil && r.Value.Value != base.Value.Value
				// Words that never change a verdict or a value on their own.
				neutral := w == g.visual[cue] || unreadable[w] || cue == "marking_style" || w == "hidden under double meat" ||
					kind == kindPlasma && cue == "seal_brand" || kind == kindHand && (w == "aluminium alloy" || w == "EMG") ||
					kind == kindRibwich && cue == "pickles" && w == "regular"
				if !changed && !neutral {
					t.Errorf("%s.%s=%q changes nothing: the checker does not know the word", kind, cue, w)
				}
			}
		}
	}
}

func inspectReq(t *testing.T, c *inspectCtx) *protocol.DecideRequest {
	t.Helper()
	raw := mustJSON(t, c)
	req := &protocol.DecideRequest{SessionID: "s", DecisionID: "d", Kind: inspectKind, GameDay: 10, DeadlineMS: 20000, Context: raw}
	switch {
	case c.Mode == modeAppraisal:
		lo, hi := int64(1), int64(5000)
		req.Options = []protocol.Option{{ID: optEstimate, Label: "Submit an estimate", Params: map[string]protocol.Param{estimateParam: {Type: protocol.ParamInt, Min: &lo, Max: &hi}}}}
	case c.Claim == "counterfeit":
		req.Options = []protocol.Option{{ID: optGenuine, Label: "Tell them it is genuine"}, {ID: optCounterfeit, Label: "Agree it is counterfeit"}}
	default:
		for _, issue := range confrontIssues[c.Item.Kind] {
			req.Options = append(req.Options, protocol.Option{ID: confrontPrefix + issue, Label: "Confront: " + issue})
		}
		req.Options = append(req.Options, protocol.Option{ID: optGenuine, Label: "No confront"})
	}
	return req
}

// The confront toggles per kind, in the panel's order.
var confrontIssues = map[string][]string{
	kindCigarettes: {"lettering", "color", "seal", "logo", "box"},
	kindStamp:      {"lettering", "logo", "seal", "amount", "chip"},
	kindInjector:   {"indicator", "label", "serial", "body", "color"},
}

func TestInspectHeuristic(t *testing.T) {
	tests := []struct {
		name   string
		c      *inspectCtx
		option string
		value  string
	}{
		{"recalled serial", cues(kindInjector, "genuine", "text.serial=3551234"), "confront_serial", ""},
		{"genuine injector", cues(kindInjector, "genuine"), "genuine", ""},
		{"claimed fake, really genuine", cues(kindCigarettes, "counterfeit"), "genuine", ""},
		{"claimed fake, really fake", cues(kindCigarettes, "counterfeit", "visual.logo=wrong logo"), "counterfeit", ""},
		{"fake stamp seal", cues(kindStamp, "genuine", "visual.seal=present"), "confront_seal", ""},
		{"ribwich appraisal", cues(kindRibwich, "", "visual.meat=double", "visual.top_bun=hidden under double meat", "visual.pickles=hidden under double meat"), optEstimate, "2521"},
		{"invalid ribwich", cues(kindRibwich, "", "text.facility=#44"), optEstimate, "315"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Heuristic{}.Decide(context.Background(), inspectReq(t, tt.c))
			if d.Choice.Option != tt.option {
				t.Fatalf("chose %s (%s), want %s", d.Choice.Option, d.Rationale, tt.option)
			}
			if got := string(d.Choice.Params[estimateParam]); got != tt.value {
				t.Fatalf("value %q, want %q", got, tt.value)
			}
		})
	}
}

func TestInspectHeuristicDefersAfterARefusedRuleValue(t *testing.T) {
	c := cues(kindRibwich, "")
	last := int64(2100)
	c.Appraisal = &appraisalTerms{LastEstimate: &last, LastFeedback: "too_high"}
	d := Heuristic{}.Decide(context.Background(), inspectReq(t, c))
	if d.Choice.Option != protocol.OptionDefer {
		t.Fatalf("bid %s again after it was refused", d.Choice.Params[estimateParam])
	}
}

func TestCheckInspectChoice(t *testing.T) {
	recalled := inspectReq(t, cues(kindInjector, "genuine", "text.serial=2514471"))
	unknown := inspectReq(t, cues(kindInjector, "genuine", "visual.indicator=blank"))
	ribwich := inspectReq(t, cues(kindRibwich, ""))
	fees := cues(kindRibwich, "")
	fees.Appraisal = &appraisalTerms{FeePct: map[string]int64{"perfect": 21, "excellent": 18, "good": 15, "mediocre": 0, "awful": 0}}
	senior := inspectReq(t, fees)
	est := func(n int) protocol.Choice {
		return protocol.Choice{Option: optEstimate, Params: map[string]json.RawMessage{estimateParam: json.RawMessage(strconv.Itoa(n))}}
	}
	tests := []struct {
		name    string
		req     *protocol.DecideRequest
		choice  protocol.Choice
		wantErr string
	}{
		{"right confront", recalled, protocol.Choice{Option: "confront_serial"}, ""},
		{"confront of a clean cue", recalled, protocol.Choice{Option: "confront_body"}, "certainly wrong"},
		{"genuine is allowed", recalled, protocol.Choice{Option: optGenuine}, ""},
		{"confront of an unreadable cue", unknown, protocol.Choice{Option: "confront_indicator"}, ""},
		{"defer", recalled, protocol.Defer(), ""},
		{"estimate in band", ribwich, est(2000), ""},
		{"estimate too high", ribwich, est(3100), "too high"},
		{"estimate too low", ribwich, est(900), "too low"},
		{"grade the rank refuses", senior, est(1600), "pays nothing"},
		{"grade the rank pays", senior, est(1800), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckChoice(tt.req, tt.choice)
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err %v, want %q", err, tt.wantErr)
			}
		})
	}
}

// The game's generators (docs/re/INSPECT.md), restated: what each fake looks like in the mod's
// cue words. The coverage test checks that the rules catch every one and name a true issue.
type genItem struct {
	c      *inspectCtx
	issues []string // the true issue toggles
	value  int64    // appraisal kinds: the true value
}

func roll(r *rand.Rand, pct int) bool { return r.IntN(100) < pct }

func spriteWord(i int, wrong string) string {
	switch {
	case i == 0:
		return cueMatches
	case i < 0:
		return cueMissing
	}
	return wrong
}

func genCigarette(r *rand.Rand) genItem {
	seal, logo, letter, box := 0, 0, 0, 0
	var is []string
	for len(is) == 0 {
		if roll(r, 10) {
			is = append(is, "seal")
			seal = 1
			if roll(r, 50) {
				seal = -1
			}
		}
		if roll(r, 20) {
			is = append(is, "logo")
			if logo = r.IntN(5); logo == 0 {
				logo = -1
			}
		}
		if roll(r, 30) {
			is = append(is, "lettering")
			if letter = r.IntN(8); letter == 0 {
				letter = -1
			}
		}
		if roll(r, 20) {
			is = append(is, "color")
			box = 1 + r.IntN(4)
		}
	}
	boxWord := "matches colour card"
	if box != 0 {
		boxWord = "off-shade"
	}
	return genItem{c: cues(kindCigarettes, "genuine", "visual.seal="+spriteWord(seal, "broken"), "visual.logo="+spriteWord(logo, "wrong logo"),
		"visual.lettering="+spriteWord(letter, "misprinted"), "visual.box_colour="+boxWord), issues: is}
}

func genStamp(r *rand.Rand) genItem {
	amount := 40 * (1 + r.IntN(5))
	printed := amount
	logo, chip, letter := 0, 0, 0
	seal := amount >= 160
	var is []string
	for len(is) == 0 {
		if roll(r, 20) {
			is = append(is, "logo")
			if logo = r.IntN(6); logo == 0 {
				logo = -1
			}
		}
		if roll(r, 20) {
			is = append(is, "chip")
			if chip = r.IntN(4); chip == 0 {
				chip = -1
			}
		}
		if roll(r, 20) {
			is = append(is, "lettering")
			if letter = r.IntN(9); letter == 0 {
				letter = -1
			}
		}
		if roll(r, 12) {
			printed = amount + 10
			if !slices.Contains(is, "amount") {
				is = append(is, "amount")
			}
		}
		seal = printed >= 160
		if roll(r, 10) {
			is = append(is, "seal")
			seal = !seal
		}
	}
	sealWord := cueMissing
	if seal {
		sealWord = cuePresent
	}
	return genItem{c: cues(kindStamp, "genuine", "text.amount="+strconv.Itoa(printed)+" credits", "visual.seal="+sealWord, "visual.logo="+spriteWord(logo, "wrong logo"),
		"visual.chip="+spriteWord(chip, "wrong chip"), "visual.lettering="+spriteWord(letter, "misprinted")), issues: is}
}

func genSerial(r *rand.Rand) string {
	for {
		p := 125 + r.IntN(331)
		if !slices.Contains(recalledPrefixes, p) {
			return fmt.Sprintf("%d%d", p, 1000+r.IntN(9000))
		}
	}
}

func genInjector(r *rand.Rand) genItem {
	edits := []string{"text.serial=" + genSerial(r)}
	var issue string
	switch {
	case roll(r, 20): // expired
		edits, issue = append(edits, "visual.indicator=expired warning"), "indicator"
	case roll(r, 25): // unusable
		switch {
		case roll(r, 33):
			edits, issue = append(edits, "visual.indicator=fault warning"), "indicator"
		case roll(r, 50):
			edits, issue = append(edits, "visual.body=cracked"), "body"
		default:
			p := recalledPrefixes[r.IntN(len(recalledPrefixes)-1)]
			edits, issue = append(edits, fmt.Sprintf("text.serial=%d%d", p, 1000+r.IntN(9000))), "serial"
		}
	case roll(r, 30): // counterfeit
		switch {
		case roll(r, 25):
			bad := ""
			switch {
			case roll(r, 20):
				bad = strconv.Itoa(100000 + r.IntN(900000))
			case roll(r, 20):
				bad = strconv.Itoa(10000000 + r.IntN(90000000))
			case roll(r, 20):
				bad = fmt.Sprintf("%d%d", 100+r.IntN(25), 1000+r.IntN(9000))
			case roll(r, 20):
				bad = fmt.Sprintf("%d%d", 456+r.IntN(544), 1000+r.IntN(9000))
			}
			edits, issue = append(edits, "text.serial="+bad), "serial"
		case roll(r, 33):
			edits, issue = append(edits, "visual.body_colour=off-colour"), "color"
		case roll(r, 50):
			models := []string{"Immunivaxine", "Immunovax", "Imunivax", "ImmuniPro", "ImmuniVax 3000", "Im univax", "Immunivx", "Immunevax", "Imm nivax", ""}
			edits, issue = append(edits, "text.model="+models[r.IntN(len(models))]), "label"
		default:
			companies := []string{"Micheal Pharmaceutical", "Carmicheal Inc.", "Carmiicheal Phamaceutical", "Carmicheal Pharmaceutical Ltd.", "Crmichal Pharmaceutical", "Carmicheal's Meds", "Carmicheal", ""}
			edits, issue = append(edits, "text.company="+companies[r.IntN(len(companies))]), "label"
		}
	}
	g := genItem{c: cues(kindInjector, "genuine", edits...)}
	if issue != "" {
		g.issues = []string{issue}
	}
	return g
}

// genRibwich restates RibwichHelper's generator over the fixture table's runs and facilities:
// a random genuine, double-meat, deluxe, opened or invalid pack, with its value by that table.
func genRibwich(r *rand.Rand) genItem {
	month, year := 1+r.IntN(12), 610+r.IntN(5)
	if roll(r, 50) {
		year = 620 + r.IntN(7)
	}
	facility := 1 + r.IntN(20)
	meat, bun, pickles, wrap, davre := 0, 0, 0, 0, 0
	barcode, facStamp := true, true
	period := 1
	if year >= 620 {
		period = 2
	}
	logo := -1
	if period == 2 {
		logo = 0
	}
	fake := false
	switch r.IntN(5) {
	case 1:
		meat = 1
	case 2:
		bun, pickles = 1, 1
	case 3:
		bun, pickles, wrap = 1, 1, 1+r.IntN(2)
	case 4:
		fake = true
		deluxe := roll(r, 20)
		if deluxe {
			bun, pickles = 1, 1
		}
		switch {
		case deluxe && roll(r, 25):
			if roll(r, 50) {
				bun = 0
			} else {
				pickles = 0
			}
		case roll(r, 20):
			davre = 1 + r.IntN(7)
		case roll(r, 5):
			barcode = false
		case roll(r, 5):
			facStamp = false
		case roll(r, 20):
			month, year = 1+r.IntN(12), 0
			if roll(r, 20) {
				month, year = 13+r.IntN(3), 610+r.IntN(5)
			} else if n := r.IntN(100); n < 33 {
				year = 605 + r.IntN(5)
			} else if n < 66 {
				year = 615 + r.IntN(5)
			} else {
				year = 627 + r.IntN(5)
			}
		default:
			facility = 21 + r.IntN(10)
		}
	}
	value := int64(2000)
	switch {
	case wrap != 0, fake:
		value = 300
	default:
		if meat == 1 {
			value += 401
		}
		if bun == 1 && pickles == 1 {
			value += 203
		}
		if month == 1 && year == 610 {
			value += 17
		}
		if period == 1 {
			value += 29
		}
		if facility == 3 || facility == 4 {
			value += 11
		} else if facility == 9 || facility == 10 {
			value -= 13
		}
	}
	word := func(on bool, yes, no string) string {
		if on {
			return yes
		}
		return no
	}
	edits := []string{
		fmt.Sprintf("text.production_date=%d/ %d", month, year), fmt.Sprintf("text.facility=#%d", facility),
		"visual.meat=" + word(meat == 1, "double", "single"),
		"visual.top_bun=" + word(bun == 1, "deluxe", "regular"), "visual.pickles=" + word(pickles == 1, "extra", "regular"),
		"visual.wrap=" + word(wrap != 0, "opened", "sealed"), "visual.davre_logo=" + spriteWord(davre, "wrong logo"),
		"visual.ribwich_logo=" + map[int]string{-1: "absent", 0: cuePresent}[logo],
		"visual.barcode_stamp=" + word(barcode, cuePresent, cueMissing), "visual.facility_stamp=" + word(facStamp, cuePresent, cueMissing),
	}
	if meat == 1 {
		edits = append(edits, "visual.top_bun=hidden under double meat", "visual.pickles=hidden under double meat")
	}
	if !barcode {
		edits = append(edits, "-text.production_date")
	}
	if !facStamp {
		edits = append(edits, "-text.facility")
	}
	return genItem{c: cues(kindRibwich, "", edits...), value: value}
}

// TestInspectCoverage: with every visible cue described, the rules catch every generated fake
// and name one of its real issues, and pass every genuine item; appraisals are exact.
func TestInspectCoverage(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	const n = 4000
	for _, gen := range []struct {
		kind string
		f    func(*rand.Rand) genItem
	}{{kindCigarettes, genCigarette}, {kindStamp, genStamp}, {kindInjector, genInjector}} {
		caught := 0
		for range n {
			g := gen.f(r)
			req := inspectReq(t, g.c)
			d := Heuristic{}.Decide(context.Background(), req)
			if len(g.issues) == 0 {
				if d.Choice.Option != optGenuine {
					t.Fatalf("%s genuine %v %v: chose %s (%s)", gen.kind, g.c.Text, g.c.Visual, d.Choice.Option, d.Rationale)
				}
				continue
			}
			issue, ok := strings.CutPrefix(d.Choice.Option, confrontPrefix)
			if !ok || !slices.Contains(g.issues, issue) {
				t.Fatalf("%s fake with %v (%v %v): chose %s (%s)", gen.kind, g.issues, g.c.Text, g.c.Visual, d.Choice.Option, d.Rationale)
			}
			caught++
		}
		if caught == 0 {
			t.Fatalf("%s: no fakes generated", gen.kind)
		}
	}
	for range n {
		g := genRibwich(r)
		rc := checkInspect(g.c)
		if rc.Value == nil || rc.Value.Value != g.value || !rc.certain() {
			t.Fatalf("ribwich %v %v: value %v, want %d; unknown %v", g.c.Text, g.c.Visual, rc.Value, g.value, rc.Unknown)
		}
	}
}

func TestAppraisalWithoutGuides(t *testing.T) {
	raw, err := ReadAppraisalGuides(fixtureGuides)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { UseAppraisalGuides(raw) })
	if err := UseAppraisalGuides(nil); err != nil {
		t.Fatal(err)
	}
	c := cues(kindRibwich, "")
	r := checkInspect(c)
	if r.Value != nil || r.certain() || !strings.Contains(strings.Join(r.Unknown, "; "), "not loaded") {
		t.Fatalf("value %v, unknown %v", r.Value, r.Unknown)
	}
	if d := inspectHeuristic(inspectReq(t, c)); d.Choice.Option != protocol.OptionDefer {
		t.Fatalf("heuristic chose %s without the tables", d.Choice.Option)
	}
	if _, sys := currentGuides(); !strings.Contains(sys, "value tables are not loaded") {
		t.Fatal("the prompt does not say the tables are missing")
	}
}

func TestAppraisalPromptStatesTheTable(t *testing.T) {
	_, sys := currentGuides()
	for _, want := range []string{
		"Prosthetic hand: base 1000.", "+29 fixture combo A", "at least 980",
		"Worth 300 if any of: wrap open; month not 1-12; year not 610-614 or 620-626",
		"seal, only the first that applies: +1 fixture seal A, else +3 fixture seal B",
		"+5 interface emg_plus", "Grading against the true value",
	} {
		if !strings.Contains(sys, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
}

func TestAppraisalGuidesPath(t *testing.T) {
	if got := AppraisalGuidesPath(func(k string) string { return map[string]string{AppraisalGuidesEnv: "/x/g.json"}[k] }); got != "/x/g.json" {
		t.Fatalf("env path: %s", got)
	}
	if got := AppraisalGuidesPath(func(string) string { return "" }); !strings.HasSuffix(got, "/local/appraisal_guides.json") {
		t.Fatalf("default path: %s", got)
	}
	if raw, err := ReadAppraisalGuides(t.TempDir() + "/none.json"); raw != nil || err != nil {
		t.Fatalf("missing file: %s %v", raw, err)
	}
	bad := t.TempDir() + "/bad.json"
	os.WriteFile(bad, []byte(`{"_note": "no tables"}`), 0o644)
	if _, err := ReadAppraisalGuides(bad); err == nil {
		t.Fatal("a file without tables was accepted")
	}
	main, wt := t.TempDir(), t.TempDir()
	os.WriteFile(wt+"/.git", []byte("gitdir: "+main+"/.git/worktrees/wt\n"), 0o644)
	if got := mainCheckout(wt); got != main {
		t.Fatalf("worktree resolves to %s, want %s", got, main)
	}
	if got := mainCheckout(main); got != main {
		t.Fatalf("checkout resolves to %s", got)
	}
}
