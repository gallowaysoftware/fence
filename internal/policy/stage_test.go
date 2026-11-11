package policy

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func stageFixture(t *testing.T) *protocol.DecideRequest {
	return fixtureNamed(t, "decide.stage.request.json")
}

// stageReq builds a stage request around a client and candidates written as JSON.
func stageReq(t *testing.T, client, candidates string, staged string, budgetLeft int64) *protocol.DecideRequest {
	t.Helper()
	ctx := `{"store":{"cash":1000,"day":50},"client":` + client + `,"counter":{"theirs":[],"yours_staged":[` + staged + `]},"budget_left":` +
		strings.TrimSpace(string(mustJSON(t, budgetLeft))) + `,"candidates":[` + candidates + `]}`
	var c stageCtx
	if err := json.Unmarshal([]byte(ctx), &c); err != nil {
		t.Fatalf("bad test context: %v\n%s", err, ctx)
	}
	req := &protocol.DecideRequest{SessionID: "s", DecisionID: "d", Kind: "stage", GameDay: 50, DeadlineMS: 20000, Context: json.RawMessage(ctx)}
	for _, it := range c.Candidates {
		req.Options = append(req.Options, protocol.Option{ID: offerID(*it.UID), Label: it.Name})
	}
	req.Options = append(req.Options, protocol.Option{ID: optNothingFits, Label: "Nothing fits"}, protocol.Option{ID: optDismiss, Label: "Send away"})
	return req
}

func mustJSON(t *testing.T, v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

const (
	buyer    = `{"name":"Buyer","faction":"lower_levels","intent":"buy","budget":500}`
	security = `{"name":"Officer","faction":"security","intent":"buy","budget":500,"is_security":true}`
	refuser  = `{"name":"Chemist","faction":"lower_levels","intent":"buy","budget":500,"no_contraband":true}`
	clean    = `{"uid":1,"name":"Lamp","price":100,"estimated_value":90,"bought_at":40,"stolen_heat":null,"contraband_level":0}`
	stolen   = `{"uid":2,"name":"Watch","price":300,"estimated_value":260,"bought_at":60,"stolen_heat":40,"contraband_level":0}`
	contra   = `{"uid":3,"name":"Pills","price":250,"estimated_value":220,"bought_at":50,"stolen_heat":null,"contraband_level":2}`
	serious  = `{"uid":4,"name":"Rifle","price":260,"estimated_value":220,"bought_at":50,"stolen_heat":null,"contraband_level":0,"serious_contraband":true}`
	under    = `{"uid":5,"name":"Toaster","price":100,"estimated_value":90,"bought_at":150,"stolen_heat":null,"contraband_level":0}`
	pricey   = `{"uid":6,"name":"Synth","price":900,"estimated_value":800,"bought_at":100,"stolen_heat":null,"contraband_level":0}`
)

func TestStageFixtureResponse(t *testing.T) {
	b, err := os.ReadFile("../../protocol/fixtures/decide.stage.response.json")
	if err != nil {
		t.Fatal(err)
	}
	var want protocol.DecideResponse
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	req := stageFixture(t)
	d := Heuristic{}.Decide(context.Background(), req)
	got := protocol.DecideResponse{DecisionID: req.DecisionID, Choice: d.Choice, Rationale: d.Rationale, Source: d.Source, OptionNotes: d.Notes}
	if a, b := mustJSON(t, got), mustJSON(t, want); string(a) != string(b) {
		t.Fatalf("heuristic answer differs from decide.stage.response.json:\ngot  %s\nwant %s", a, b)
	}
}

func TestCheckChoice(t *testing.T) {
	tests := []struct {
		name    string
		req     *protocol.DecideRequest
		option  string
		wantErr string
	}{
		{"clean offer", stageReq(t, buyer, clean, "", 500), "offer_1", ""},
		{"stolen to a civilian", stageReq(t, buyer, stolen, "", 500), "offer_2", ""},
		{"stolen to security", stageReq(t, security, clean+","+stolen, "", 500), "offer_2", "never offered to security"},
		{"contraband to security", stageReq(t, security, contra, "", 500), "offer_3", "never offered to security"},
		{"serious contraband to security", stageReq(t, security, serious, "", 500), "offer_4", "never offered to security"},
		{"clean to security", stageReq(t, security, clean, "", 500), "offer_1", ""},
		{"contraband to a refuser", stageReq(t, refuser, contra, "", 500), "offer_3", "refuses contraband"},
		{"stolen to a refuser", stageReq(t, refuser, stolen, "", 500), "offer_2", ""},
		{"not a candidate", stageReq(t, buyer, clean, "", 500), "offer_9", "names no listed candidate"},
		{"nothing fits", stageReq(t, security, stolen, "", 500), optNothingFits, ""},
		{"defer", stageReq(t, security, stolen, "", 500), protocol.OptionDefer, ""},
		{"barter is not checked", fixtureRequest(t), "sell_all", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckChoice(tt.req, protocol.Choice{Option: tt.option})
			if tt.wantErr == "" && err != nil || tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("err %v, want %q", err, tt.wantErr)
			}
		})
	}
}

type fixedPlan string

func (p fixedPlan) CurrentPlan(string) (string, int, bool) { return string(p), 50, true }

func TestStageHeuristic(t *testing.T) {
	tests := []struct {
		name   string
		plan   string
		req    *protocol.DecideRequest
		want   string
		inWhy  string
		source protocol.Source
	}{
		{"best clean margin", "", stageReq(t, buyer, clean+","+stolen, "", 500), "offer_1", "Skipped: stolen Watch #2 sells clean from day 53", protocol.SourceHeuristic},
		{"plan sells the stolen one", "STOCK\n- sell Watch #2 today, rent is due", stageReq(t, buyer, clean+","+stolen, "", 500), "offer_2", "Watch #2", protocol.SourceHeuristic},
		{"plan holds it in the bay", "STOCK\n- hold Watch #2 in the smuggler bay until day 53, then sell clean", stageReq(t, buyer, stolen, "", 500), optDismiss, "Skipped: Watch #2 is on the plan's hold list", protocol.SourceHeuristic},
		{"plan keeps the clean one", "KEEP\n- Lamp #1 feeds the furnace\n- SAFE TO SELL: Pills #3", stageReq(t, buyer, clean+","+contra, "", 500), "offer_3", "Lamp #1 is on the plan's hold list", protocol.SourceHeuristic},
		{"safe to sell is not held", "KEEP\n- SAFE TO SELL: Lamp #1", stageReq(t, buyer, clean, "", 500), "offer_1", "", protocol.SourceHeuristic},
		{"security gets only clean goods", "STOCK\n- sell Watch #2", stageReq(t, security, stolen+","+contra+","+clean, "", 500), "offer_1", "", protocol.SourceHeuristic},
		{"refuser never gets contraband", "", stageReq(t, refuser, contra+","+clean, "", 500), "offer_1", "", protocol.SourceHeuristic},
		{"below cost is not offered", "", stageReq(t, buyer, under, "", 500), optDismiss, "", protocol.SourceHeuristic},
		{"over budget below the estimate", "", stageReq(t, buyer, pricey, "", 500), optDismiss, "", protocol.SourceHeuristic},
		{"over budget above the estimate", "", stageReq(t, buyer, pricey, "", 850), "offer_6", "capped at budget 850", protocol.SourceHeuristic},
		{"in budget beats over budget", "", stageReq(t, buyer, pricey+","+clean, "", 850), "offer_1", "", protocol.SourceHeuristic},
		{"nothing more once something is staged", "", stageReq(t, buyer, under, clean, 400), optNothingFits, "sell what is staged", protocol.SourceHeuristic},
		{"unparseable context", "", func() *protocol.DecideRequest {
			r := stageReq(t, buyer, clean, "", 500)
			r.Context = json.RawMessage(`{"candidates":"none"}`)
			return r
		}(), protocol.OptionDefer, "", protocol.SourceHeuristic},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Heuristic{Plan: fixedPlan(tt.plan)}.Decide(context.Background(), tt.req)
			if d.Choice.Option != tt.want || d.Source != tt.source {
				t.Fatalf("chose %s (%s, %s), want %s", d.Choice.Option, d.Source, d.Rationale, tt.want)
			}
			if !strings.Contains(d.Rationale, tt.inWhy) {
				t.Errorf("rationale %q lacks %q", d.Rationale, tt.inWhy)
			}
			if err := CheckChoice(tt.req, d.Choice); err != nil {
				t.Errorf("heuristic chose a forbidden offer: %v", err)
			}
		})
	}
}

func TestPlanIntent(t *testing.T) {
	plan := "GOALS\n- sell Gold Chain #601 for rent\n**STOCK:** sell Clock #900 today\n- hold Ribs #953 in the smuggler bay until day 84\n" +
		"KEEP\n- Water Jug #12 (water chain)\n- SAFE TO SELL: Kettle #5, Lamp #9001\nRISK\n- sell nothing hot to security, Clock #900"
	tests := []struct {
		uid        int64
		hold, sell bool
	}{
		{601, false, false},
		{900, false, true},
		{953, true, false},
		{12, true, false},
		{5, false, true},
		{9001, false, true},
		{90, false, false},
		{1, false, false},
	}
	for _, tt := range tests {
		if hold, sell := planIntent(plan, tt.uid); hold != tt.hold || sell != tt.sell {
			t.Errorf("#%d: hold %v sell %v, want %v %v", tt.uid, hold, sell, tt.hold, tt.sell)
		}
	}
	if hold, sell := planIntent("", 1); hold || sell {
		t.Error("an empty plan says something")
	}
}

func TestStageNotes(t *testing.T) {
	req := stageReq(t, buyer, clean+","+stolen+","+contra+","+under+","+pricey, "", 500)
	got := OptionNotes(req)
	want := map[string]string{
		"offer_1": "receive 100, margin +60 vs paid 40",
		"offer_2": "receive 300, margin +240 vs paid 60; STOLEN: selling is a fencing crime; sells clean from day 53",
		"offer_3": "receive 250, margin +200 vs paid 50; contraband: selling is a trafficking crime",
		"offer_5": "receive 100, margin -50 vs paid 150 (below cost)",
		"offer_6": "receive 500 (capped by budget, under est. 800), margin +400 vs paid 100",
	}
	if a, b := mustJSON(t, got), mustJSON(t, want); string(a) != string(b) {
		t.Fatalf("notes\ngot  %s\nwant %s", a, b)
	}
}

// The model is asked to fix an offer the state forbids, and never gets it through.
func TestLLMStageRejectsForbiddenOffers(t *testing.T) {
	bad := `{"rationale":"Hot watch, big margin.","option":"offer_2","params":{}}`
	good := `{"rationale":"Clean lamp.","option":"offer_1","params":{}}`
	tests := []struct {
		name    string
		replies []reply
		want    string
		source  protocol.Source
	}{
		{"repaired", []reply{{text: bad}, {text: good}}, "offer_1", protocol.SourceLLM},
		{"insists", []reply{{text: bad}, {text: bad}}, protocol.OptionDefer, protocol.SourceFallback},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &scripted{replies: tt.replies}
			d := NewLLM(client, WithStageLLM(true)).Decide(context.Background(), stageReq(t, security, clean+","+stolen, "", 500))
			if d.Choice.Option != tt.want || d.Source != tt.source {
				t.Fatalf("chose %s (%s: %s), want %s", d.Choice.Option, d.Source, d.Rationale, tt.want)
			}
			if len(client.got) != 2 || !strings.Contains(client.got[1].Messages[2].Content, "never offered to security") {
				t.Fatalf("the repair round did not say why: %+v", client.got)
			}
		})
	}
}

type recordingRecall struct{ got []*protocol.DecideRequest }

func (r *recordingRecall) Brief(_ context.Context, req *protocol.DecideRequest, _ int) string {
	r.got = append(r.got, req)
	return "Past deals: Radio sold at 110% of est"
}

func TestStagePrompt(t *testing.T) {
	rec := &recordingRecall{}
	client := &scripted{replies: []reply{{text: `{"rationale":"r","option":"offer_2240","params":{}}`}}}
	plan := "KEEP\n- Radio #2240 (uncertain)"
	d := (&LLM{StageLLM: true, Client: client, Recall: rec, Plan: &recordingPlanner{text: plan, day: 69}}).Decide(context.Background(), stageFixture(t))
	if d.Choice.Option != "offer_2240" {
		t.Fatalf("chose %s: %s", d.Choice.Option, d.Rationale)
	}
	req := client.got[0]
	if req.System != stageSystem {
		t.Fatal("stage decisions must use the stage system prompt")
	}
	prompt := req.Messages[0].Content
	for _, w := range []string{
		"TODAY'S PLAN", plan, "Past deals: Radio",
		"Budget left after the staged items: 900", "Candidates from the shop: 4",
		"- offer_2240: Radio #2240 (showcase): ~598, est 520, paid 300; wants electronics, music => receive 598, margin +298 vs paid 300\n",
		"=> receive 345, margin +165 vs paid 180; STOLEN: selling is a fencing crime; sells clean from day 71 = claims Worn\n",
		"- nothing_fits: Nothing in the shop fits this customer\n",
	} {
		if !strings.Contains(prompt, w) {
			t.Errorf("prompt lacks %q:\n%s", w, prompt)
		}
	}
	if strings.Contains(prompt, "Nothing is staged yet") {
		t.Error("the barter defer hint must not reach a stage prompt")
	}

	if len(rec.got) != 1 {
		t.Fatalf("recall called %d times", len(rec.got))
	}
	view := rec.got[0]
	var ctx struct {
		Counter struct {
			YoursStaged []struct {
				ID string `json:"id"`
			} `json:"yours_staged"`
		} `json:"counter"`
		Candidates json.RawMessage `json:"candidates"`
	}
	if err := json.Unmarshal(view.Context, &ctx); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range ctx.Counter.YoursStaged {
		ids = append(ids, it.ID)
	}
	if view.Kind != "barter" || strings.Join(ids, ",") != "cassette2,radio,walkman,synth" || ctx.Candidates != nil || view.Options != nil {
		t.Fatalf("memory sees kind %q, staged %v, candidates %s, options %v", view.Kind, ids, ctx.Candidates, view.Options)
	}

	for _, w := range []string{"KEEP list", "SAFE TO SELL", "smuggler bay", "Never offer stolen or contraband goods to security", "nothing_fits", "Past deals from memory"} {
		if !strings.Contains(stageSystem, w) {
			t.Errorf("stage system prompt lacks %q", w)
		}
	}
}

func TestLLMStageUsesTheFastRulesUnlessEnabled(t *testing.T) {
	req := stageFixture(t)
	want := Heuristic{}.Decide(context.Background(), req)

	client := &scripted{}
	d := NewLLM(client).Decide(context.Background(), req)
	if client.calls() != 0 {
		t.Fatalf("model called %d times for a stage decision", client.calls())
	}
	if d.Choice.Option != want.Choice.Option || d.Source != protocol.SourceHeuristic {
		t.Fatalf("chose %s (%s), want the heuristic's %s", d.Choice.Option, d.Source, want.Choice.Option)
	}
	if len(d.Notes) == 0 {
		t.Error("option notes are missing")
	}

	client = &scripted{replies: []reply{{text: `{"rationale":"r","option":"offer_2240","params":{}}`}}}
	d = NewLLM(client, WithStageLLM(true)).Decide(context.Background(), req)
	if client.calls() != 1 || d.Source != protocol.SourceLLM {
		t.Fatalf("enabled: %d calls, source %s", client.calls(), d.Source)
	}
}

func TestLLMStageFastRulesKeepTheSafetyCheck(t *testing.T) {
	client := &scripted{}
	d := NewLLM(client).Decide(context.Background(), stageReq(t, security, stolen, "", 500))
	if client.calls() != 0 || d.Choice.Option == "offer_2" {
		t.Fatalf("offered stolen goods to security: %+v", d)
	}
}
