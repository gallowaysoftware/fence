package strategy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/show"
)

func forkSnap(t *testing.T, js string) *snapshot {
	t.Helper()
	s, err := parseSnapshot(json.RawMessage(js))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const appraiserBase = `{"engine":"wine+furnace","bays":2,"appraisal":"join","buyout_by_day":140,"buyout_save":["insurance"],"buyout_save_from":56}`

func baseCtx(start, base string) forkContext {
	fc := forkContext{start: start}
	json.Unmarshal([]byte(base), &fc.base)
	return fc
}

func kinds(fs []fork) string {
	var k []string
	for _, f := range fs {
		k = append(k, f.Kind)
	}
	return strings.Join(k, ",")
}

func TestDetectForks(t *testing.T) {
	offer := `{"phase":"morning","store":{"day":70,"cash":9000,"net_worth":20000},"rent":{"rent":900,"days_left":0},
		"property":{"owned":false,"buyout_price":31000,"mortgage_offer":{"down_payment":4000,"weekly":1700,"weeks":16}}}`
	st := map[string]*forkState{}
	fs := detectForks(forkSnap(t, offer), 70, baseCtx("apprentice", appraiserBase), st)
	if got := kinds(fs); got != "mortgage,sprint,style" {
		t.Fatalf("day 70 with an offer: %s", got)
	}
	m := fs[0]
	if !strings.Contains(strings.Join(m.Facts, " "), "4000 down, then 1700 a week for 16 weeks (27200 in all)") {
		t.Errorf("mortgage facts: %v", m.Facts)
	}
	// Asked and answered "later": not again until a week has passed.
	st["mortgage"].lastDay, st["mortgage"].choice = 70, "later"
	if f := detectForks(forkSnap(t, offer), 72, baseCtx("apprentice", appraiserBase), st); strings.Contains(kinds(f), "mortgage") {
		t.Errorf("asked again two days later: %s", kinds(f))
	}
	if f := detectForks(forkSnap(t, offer), 77, baseCtx("apprentice", appraiserBase), st); !strings.Contains(kinds(f), "mortgage") {
		t.Errorf("not asked a week later: %s", kinds(f))
	}
	st["mortgage"].choice = "decline"
	if f := detectForks(forkSnap(t, offer), 84, baseCtx("apprentice", appraiserBase), st); strings.Contains(kinds(f), "mortgage") {
		t.Errorf("asked after a final decline: %s", kinds(f))
	}

	// A medical shortage with medical stock on hand asks whether to hold it.
	shortage := `{"phase":"morning","store":{"day":20,"cash":800},"market_events":{"active":[{"name":"Flu season","start_day":19,"duration":5,"negociation_datas":[{"item_type":"MEDICAL","value":50}]}]},
		"inventory":[{"uid":1,"type":"MEDICAL","where":"stock"},{"uid":2,"type":"MEDICAL","where":"stock"},{"uid":3,"type":"MEDICAL","where":"showcase"},{"uid":4,"type":"FOOD","where":"stock"}]}`
	st = map[string]*forkState{}
	fs = detectForks(forkSnap(t, shortage), 20, baseCtx("apprentice", appraiserBase), st)
	if kinds(fs) != "hold" {
		t.Fatalf("shortage: %s (active %+v)", kinds(fs), forkSnap(t, shortage).Active)
	}
	var d protocol.Directives
	fs[0].Options[1].apply(&d)
	if d.Sell == nil || len(d.Sell.HoldUIDs) != 3 || fs[0].Until != 23 {
		t.Errorf("hold: %+v until %d", d.Sell, fs[0].Until)
	}
	if f := detectForks(forkSnap(t, shortage), 27, baseCtx("apprentice", appraiserBase), st); f != nil {
		t.Errorf("the same event asked twice: %s", kinds(f))
	}

	// A route that already runs a style or the furnace is not asked about them.
	fs = detectForks(forkSnap(t, `{"phase":"morning","store":{"day":30,"cash":5000}}`), 30, baseCtx("apprentice", `{"engine":"wine+furnace","specialty":{"types":["MEDICAL"]}}`), map[string]*forkState{})
	if kinds(fs) != "" {
		t.Errorf("styled furnace route: %s", kinds(fs))
	}
	fs = detectForks(forkSnap(t, `{"phase":"morning","store":{"day":30,"cash":5000}}`), 30, baseCtx("rancher", `{"engine":"wine"}`), map[string]*forkState{})
	if kinds(fs) != "engine,style" || len(fs[0].Options) != 3 {
		t.Errorf("rancher without furnace or rats: %s %+v", kinds(fs), fs)
	}
}

type forkLLM struct {
	answers []string
	asked   []string
}

func (s *forkLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	s.asked = append(s.asked, req.Messages[len(req.Messages)-1].Content)
	if len(s.answers) == 0 {
		return llm.Response{Text: "{}"}, nil
	}
	a := s.answers[0]
	s.answers = s.answers[1:]
	return llm.Response{Text: a}, nil
}

// TestForkChoiceHoldsAndCarries: a fork's choice patches every later day's directives, the
// carried days included, and goes to the feed with the model's reason.
func TestForkChoiceHoldsAndCarries(t *testing.T) {
	c := &forkLLM{answers: []string{`{"choice":"decline","why":"Cash 9000 already covers a third of the 31000 price."}`}}
	var feed []show.Event
	s := New(Options{LLM: c, BaseDirectives: appraiserBase, Agency: Agency{Forks: true, PlanEvery: 7, Show: func(_ string, ev show.Event) { feed = append(feed, ev) }}})
	defer s.Close()
	r := newRun(70)
	r.forks = map[string]*forkState{}
	snap := forkSnap(t, `{"phase":"morning","store":{"day":70,"cash":9000,"net_worth":20000},"property":{"owned":false,"buyout_price":31000,"mortgage_offer":{"down_payment":4000,"weekly":1700,"weeks":16}}}`)
	f, ok := mortgageFork(snap, 70, 9000, false, false, nil, &forkState{})
	if !ok {
		t.Fatal("no mortgage fork")
	}
	r.forks["mortgage"] = &forkState{}
	s.decideFork(context.Background(), "sess", r, 70, f, "sys")
	s.mu.Lock()
	s.carry(r, 71, false)
	d := r.directives
	s.mu.Unlock()
	var signed protocol.Directives
	f.Options[1].apply(&signed)
	if signed.Mortgage != "take" || signed.BuyoutByDay == nil || *signed.BuyoutByDay != 1 {
		t.Errorf("sign must clear the cash rule: %+v", signed)
	}
	if d == nil || d.Mortgage != "decline" || d.Day != 71 || r.planDay != 71 || r.plan != carryPlan {
		t.Fatalf("carried: %+v plan %q day %d", d, r.plan, r.planDay)
	}
	if len(feed) != 1 || feed[0].Kind != show.KindCandidates || feed[0].Level != show.LevelMoment || !strings.Contains(feed[0].Text, "a third of the 31000") {
		t.Fatalf("feed: %+v", feed)
	}
	var chosen int
	for _, o := range feed[0].Candidates.Options {
		if o.Chosen {
			chosen++
		}
	}
	if chosen != 1 || !strings.Contains(c.asked[0], "decline: decline for good") {
		t.Errorf("options %+v asked %q", feed[0].Candidates.Options, c.asked[0])
	}
	// An unusable answer leaves the route's rule: no patch.
	c.answers = []string{`{"choice":"maybe","why":"?"}`}
	s.decideFork(context.Background(), "sess", r, 77, f, "sys")
	s.mu.Lock()
	s.carry(r, 78, false)
	d = r.directives
	s.mu.Unlock()
	if d.Mortgage != "" || r.forks["mortgage"].choice != "route_rule" {
		t.Errorf("after an unusable answer: %+v", d)
	}
}

func TestShowChoiceOncePerSession(t *testing.T) {
	var feed []show.Event
	s := New(Options{Agency: Agency{Choice: &Choice{Route: "hoard_smelter", Composed: true, Summary: "smelter with speculate", Thesis: "Hoard the ingots."},
		Show: func(_ string, ev show.Event) { feed = append(feed, ev) }}})
	defer s.Close()
	s.Hello("a", protocol.HelloRequest{})
	s.Hello("a", protocol.HelloRequest{})
	if len(feed) != 1 || feed[0].Kind != show.KindPlan || !strings.Contains(feed[0].Text, "composes its own route, hoard_smelter (smelter with speculate). Hoard the ingots.") {
		t.Errorf("%+v", feed)
	}
}
