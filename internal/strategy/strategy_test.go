package strategy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/memory"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/server"
)

func TestRenderScenarios(t *testing.T) {
	want := map[string][]string{
		"rent-tomorrow-low-cash": {
			"TODAY: day 27, morning.",
			"Rent: 600 due in 1 day(s) (day 28); SHORT 180; no landlord grace: not paying ends the run.",
			"Rent runway: 180 short of rent due on day 28; clean stock is worth about 1560 at estimate.",
			"Window special visitors (rolled, drawn from the regular customers): Office worker 55% for Synth Watch.",
			"- day 29 (in 2): Chip Shortage: module -10% (4 days)",
			"- Cassette (household_good) [Worn (-20%)] #302, est. 90, paid 50 (stock)",
			"Window (display case) now: Synth Watch #301, Retro Lamp #304.",
		},
		"faction-minus-85": {
			"- Lower Levels -85 (Disreputable); next perk Referral at 20, 105 to go; next penalty Lynch at -100, 15 away (ENDS THE RUN); DANGER; unlocked: PoorReviews",
			"Market today: Food Festival: food +15% (3 days).",
		},
		"referral-at-18": {
			"YESTERDAY (day 54): cash 2480 -> 2710",
			"YESTERDAY SCORED AGAINST ITS PLAN:\n- PARTLY: Lower Levels to 20 (reached 18)",
			"YESTERDAY'S PLAN (day 54):\nGOALS\n",
			"next perk Referral at 20, 2 to go",
			"- Tools for the dock, from Foreman Rudd, Lower Levels, wants 1 x toolkit, pays 380, +3 rep, 2 days",
		},
		"hot-stolen-inspection-prone": {
			"- Gold Chain #601 (luxury_item) in showcase, est. 1400, paid 820, heat 70, sells clean from day 64 [EXPOSED: not in a smuggler bay; an inspection finds it with chance = its heat % and confiscates it] [ON SHOWCASE: visible to security]",
			"- Holo Camera #602 (module) in stock, est. 900, paid 560, heat 40, sells clean from day 63 [EXPOSED",
			"Security record: trafficking 820, crimes fencing 820, bribery 0, evidence 72/100, projected sentence Prison",
			"- Security -25 (Disfavored); next perk Requisition at 0, 25 to go; next penalty AlarmIgnored at -40, 15 away",
		},
		"hot-stock-near-inspection-threshold": {
			"- Smart Watch #803 (electronics) in stock, est. 420, paid 160, heat 82, sells clean from day 35 [EXPOSED",
			"Hot stock exposed to an inspection: 5 item(s) outside a smuggler bay (2 inside one); an inspection finds each with chance = its heat % and CONFISCATES it: expected loss about 979 credits and 16 Security rep per inspection.",
			"No scheduled inspection while the trafficking record (fencing included) stays under 50 (now 44)",
			"- Security at -100 = immediate arrest: now -52, 48 away",
			"Inspection Early Warning (300) affordable: favor 180 + cash 120",
			"THREAT Security Sweep",
		},
		"contraband-in-stock": {
			"- Stim Injector #701 (substance) in stock, est. 450, paid 200, contraband level 3 SERIOUS\n",
			"- Combat Knife #703 (weapon) in showcase, est. 240, paid 130, contraband level 1 [ON SHOWCASE: visible to security]",
			"Services in use: Power (40 a night), Security Informant (60 a night).",
			"Early warning: an inspection is announced",
			"Night log:\n- Your informant says an inspection is coming in 2 days.",
		},
		"flush-nothing-to-sell": {
			"Property: buyout price 13900 (4500 to go).",
			"- day 90 (in 2): Launch Week: module +25% (4 days)",
			"Upgrades available (paid from Wild favor first, then cash, 0 favor on hand): Projector (2200) affordable: favor 0 + cash 2200, Landlord Grace (900) affordable: favor 0 + cash 900.",
		},
		"production-food-shortage": {
			"- Berries (food) [Ripe] x9 #801 #802 #803 #804 …, est. 30 each, paid 0 avg (stock)",
			"- Berries (food) [Moldy (-60%)] x3 #810 #811 #812, est. 12 each",
			"Calendar, next 7 days:\n- day 61 (in 3): Food Shortage: food +40% (4 days) [you hold 18 food items worth 444: buy food before day 61, sell it on days 61-64, not before]\n- day 63 (in 5): rent day\n",
			"afterhours: [{available=true, location=Dumping ground}]",
			"production: {garden=[{crop=berries, planted=6, plots=8, ready_in_days=2}], machines=[{contents=Berries x10 (Ripe), days_left=1",
			"food 444 -> tier 3 (Berries #801, Berries #802, Berries #803, Berries #804, 14 more)",
		},
		"calendar-threat-and-boom": {
			"- day 74 (in 1): Weapons Demand: weapon +30%, ammunition +30% (3 days) [you hold 0 weapon items worth 0: buy weapon before day 74, sell it on days 74-76, not before; you hold 1 ammunition items worth 160: buy ammunition before day 74, sell it on days 74-76, not before]\n- day 75 (in 2): THREAT Security Crackdown (3 days)\n- day 76 (in 3): cartel visit\n- day 77 (in 4): rent day\n",
			"- Holo Camera #733 (module) in stock, est. 900, paid 520, heat 50, sells clean from day 76, before rent is due on day 77 [EXPOSED",
		},
		"stolen-meat-smuggler-bay": {
			"- Wagyu Cut #955 (food) in smuggler bay #950 (inspections do not search it), est. 700, paid 300, heat 70, sells clean from day 86,",
			"- Ribs #954 (food) in showcase, est. 220, paid 90, heat 35, sells clean from day 84, before rent is due on day 87 [EXPOSED: not in a smuggler bay; an inspection finds it with chance = its heat % and confiscates it] [ON SHOWCASE: visible to security]",
			"- Prime Steak #952 (food) in stock, est. 260, paid 120, heat 10, sells clean from day 83",
			"- Prime Steak #951 (food) in stock, est. 260, paid 120, heat 0 (sells clean now)\n",
			"- day 84 (in 2): THREAT Security Sweep (2 days); rent day",
		},
		"keep-water-filters": {
			"- Water Filter (household_good) x3 #601 #602 #603, est. 140 each, paid 60 avg (stock)",
			"production: {machines=[{contents=Berries x8 (Ripe), days_left=2, id=fermenter_1, name=Fermenter, needs=clean water, output=Young Wine}",
			"- day 65 (in 1): Household Fair: household_good +20% (3 days) [you hold 5 household_good items worth 700: buy household_good before day 65, sell it on days 65-67, not before]",
			"- Filters for the block, from Mrs Ota, Lower Levels, wants 2 x water_filter, pays 320, +2 rep, 3 days",
		},
		"window-type-concentration": {
			"Window (display case) now: Antique Clock #901 (STOLEN: 35% chance of an inspection at each opening), Board Game #902.",
			"Window buyers: at opening the top 1 item type(s) by displayed value each bring one guaranteed buyer",
			"Displayed now by type: household_good 720 (draws a tier 3 buyer).",
			"tool 480 -> tier 3 (Toolkit #903, Wrench Set #904)",
		},
		"window-three-buyers-contraband": {
			"Window (display case) now: Pistol #921 (CONTRABAND: guarantees an inspection at every opening).",
			"top 3 item type(s)",
			"Displayed now by type: weapon 700 (draws a tier 3 buyer).",
			"medical 520 -> tier 3 (Med Kit #922, Med Kit #923); household_good 380 -> tier 3 (Silk Scarf #925); tool 300 -> tier 2 (Toolkit #924); food 40 -> tier 1 (Noodle Pack #927)",
			"Discerning shopper (Upper Levels, budget about 799+, wants a luxury item): 10% if a luxury item is displayed; you have Silk Scarf #925, Gold Watch #926.",
			"- day 51 (in 2): Clinic Strike: medical +20% (4 days)",
		},
		"hot-stock-low-cash": {
			"Shop: cash 300, retail markup 15% (fixed), 8 customers queued.",
			"Rent: 500 due in 3 day(s) (day 35); SHORT 200.",
			"stolen items clean before then: Smart Glasses (est. 650, clean on day 34), E-Reader (est. 280, clean on day 33)",
			"heat 10, sells clean from day 33, before rent is due on day 35 [EXPOSED",
			"Security record: trafficking 60, evidence Low, suspected of Fencing, projected sentence Fine.",
		},
		"wine-berries-two-bottles-no-yeast": {
			"- Hydroponic bay #500: bloomberry (perennial: never replant, established: 12 work to refruit), progress 2/12, harvest in 1 night(s) (morning of day 42) of 6;",
			"- Wine: 2 empty bottle(s) #520 #521; 0 yeast; berries: 3 best #510 #511 #512, 2 normal #513 #514, 1 under-ripe #515, 1 over-ripe #516, 1 moldy #517.",
			"Best fill of the 2 bottle(s) tonight: 1 x (best+best, no yeast -> Reserve 150), 1 x (best+normal, no yeast -> Production 125)",
			"With one wine_yeast (10) per bottle instead: 1 x (best+best + yeast -> Master Vintage 175), 1 x (best+normal + yeast -> Reserve 150), worth 367 (+50)",
			"SUGGESTION: under- and over-ripe berries #515 #516",
			"bottle #530 Production, aged 9 night(s), value 158, +3.75 a night",
			"- KEEP: Wine Bottle x2 #520 #521: use ≈ 76 vs sale est. 14",
			"- Sale beats use: Bloomberry (moldy) #517",
		},
		"water-worn-filter-household-buyer": {
			"Water Pitcher #600, 2 L, label Rustwater (sells as ~20); filter Water Filter durability 12/400 (WORN",
			"Water Jug #621 in water_purifier container, 6 L, label Rustwater (sells as ~60); the purifier changes it tonight and leaves the label stale",
			"Taps: gutter, rust",
			"- KEEP: Water Filter x3 #602 #603 #604: use ≈ 324 vs sale est. 29",
			"- KEEP: Water Test Strip x2 #630 #631: use ≈ 60",
		},
		"furnace-scrap-no-flux": {
			"- Furnace #700: loaded 0 scrap, 0 ore, 0 flux; in stock 7 scrap, 2 ore, 0 flux, 6 junk. Battery 40 (8 a smelt).",
			"2 scrap -> Low 42.5 (Fair 50 with flux); scrap+ore -> Fair 50 (High 62.5 with flux); 2 ore -> High 62.5 (Very high 75 with flux). No flux on hand",
			"- Scavenging (one outing a night, after closing): 5 attempts",
			"- KEEP: Scrap Metal x7 #710 #711 #712 #713 …: use ≈ 21 vs sale est. 11",
			"Junk x6 #730 #731 #732 #733 … (needs the furnace junk module)",
			"- day 36 (in 3): Construction Project: material +30% (3 days)",
		},
		"water-mislabelled-stock": {
			"Water Jug #800, 6 L, label Basewater (sells as ~240); Last scan: 93.10 % Rustwater -> OVER-LABELLED",
			"Large Bottled Water #801, 2 L, label Gutterflow (sells as ~8); Last scan: 99.95 % Pure Water -> UNDER-LABELLED: sells as Gutterflow though it is Pure (~160)",
			"Bottled Water #803, 0.75 L, label Basewater (sells as ~30), untested.\n",
			"- Sale beats use: Labeler #820",
		},
		"water-basewater-to-berries": {
			"WATER PROBLEM: the bay drinks water_jug #902 (tested Basewater).",
			"Bloomberry wants minerals at 200 ± 75 bp",
			"Route water by grade: Basewater",
			"Water Pitcher #912 in water_purifier container, 3 L, label Basewater (sells as ~120); Last scan: 99.01 % High Quality Water -> UNDER-LABELLED",
			"- day 109 (in 1): Water Treatment Breakdown: water +50% (3 days)",
			"Plant Analyser #950",
		},
	}
	notWant := map[string][]string{
		"water-worn-filter-household-buyer": {"tested Pure", "Pure)"},
		"water-mislabelled-stock":           {"tested Ghostwater"},
		"water-basewater-to-berries":        {"Bloomberry Seed #901"},
		"flush-nothing-to-sell":             {"Buyer Referral", "Rent runway"},
		"rent-tomorrow-low-cash":            {"Calendar, next 7 days:\n- day 27"},
		"window-type-concentration":         {"Discerning shopper (Upper"},
		"stolen-meat-smuggler-bay":          {"Smuggler Bay #950", "Ribs #953 (food) in smuggler bay #950 (inspections do not search it), est. 220, paid 90, heat 35, sells clean from day 84, before rent is due on day 87 [EXPOSED"},
		"window-three-buyers-contraband":    {"Pistol #921 (weapon) in showcase, est. 700, paid 380, contraband level 2\n"},
	}
	scs, err := LoadScenarios("testdata/days")
	if err != nil {
		t.Fatal(err)
	}
	if len(scs) != len(want) {
		t.Fatalf("%d scenarios, %d expectations", len(scs), len(want))
	}
	for _, sc := range scs {
		t.Run(sc.Name, func(t *testing.T) {
			prompt, err := BuildPlanPrompt(sc.Input())
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range want[sc.Name] {
				if !strings.Contains(prompt, w) {
					t.Errorf("prompt missing %q:\n%s", w, prompt)
				}
			}
			for _, nw := range append(notWant[sc.Name], "zz_", "777777", "days_to_inspection", "true_value") {
				if strings.Contains(prompt, nw) {
					t.Errorf("prompt has %q:\n%s", nw, prompt)
				}
			}
		})
	}
}

func TestTidyPlan(t *testing.T) {
	full := "CALENDAR\n- k\nGOALS\n- a\n- b\nCASH\n- c\nFACTIONS\n- d\nWINDOW\n- w\nSTOCK\n- e\nKEEP\n- k2\nPRODUCTION\n- p\nRISK\n- f\nCUSTOMERS\n- g"
	tests := []struct {
		name, in, want, err string
	}{
		{"canonical", full, full, ""},
		{"markdown, preamble, inline bodies", "Here is the plan:\nCalendar: k\n## **GOALS**\n1. a\n2) b\n**Cash:** c\nFactions:\n* d\nWindow: w\nSTOCK (today): e\nKeep: k2\nProduction:\n- p\nRISK\n• f\nCUSTOMERS\ng\n```", full, ""},
		{"reordered sections", "CUSTOMERS\n- g\nKEEP\n- k2\nPRODUCTION\n- p\nCALENDAR\n- k\nWINDOW\n- w\nGOALS\n- a\n- b\nCASH\n- c\nFACTIONS\n- d\nSTOCK\n- e\nRISK\n- f", full, ""},
		{"prose starting with a heading word is body", "CALENDAR\n- k\nGOALS\n- a\nCash is tight\nCASH\n- c\nFACTIONS\n- d\nWINDOW\n- w\nSTOCK\n- e\nKEEP\n- k2\nPRODUCTION\n- p\nRISK\n- f\nCUSTOMERS\n- g",
			"CALENDAR\n- k\nGOALS\n- a\n- Cash is tight\nCASH\n- c\nFACTIONS\n- d\nWINDOW\n- w\nSTOCK\n- e\nKEEP\n- k2\nPRODUCTION\n- p\nRISK\n- f\nCUSTOMERS\n- g", ""},
		{"think block dropped", "<think>GOALS\n- x</think>" + full, full, ""},
		{"missing headings", "GOALS\n- a\nCASH\n- c", "", "CALENDAR, FACTIONS, WINDOW, STOCK, KEEP, PRODUCTION, RISK, CUSTOMERS"},
		{"empty section", strings.Replace(full, "- f\n", "", 1), "", "RISK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TidyPlan(tt.in)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err %v, want containing %q", err, tt.err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v\nwant %q", got, err, tt.want)
			}
		})
	}
}

func TestTidyPlanFits(t *testing.T) {
	long := strings.Repeat("x", 150)
	var b strings.Builder
	for _, h := range Headings {
		b.WriteString(h + "\n")
		for i := range 4 {
			fmt.Fprintf(&b, "- %d %s\n", i, long)
		}
	}
	got, err := TidyPlan(b.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > MaxPlanBytes {
		t.Fatalf("%d bytes, over %d", len(got), MaxPlanBytes)
	}
	if _, err := TidyPlan(got); err != nil {
		t.Fatalf("a fitted plan lost a section: %v\n%s", err, got)
	}
	huge, err := TidyPlan(strings.Join([]string{"CALENDAR", "- k", "GOALS", "- " + strings.Repeat("y", 3000), "CASH", "- c", "FACTIONS", "- d", "WINDOW", "- w", "STOCK", "- e", "KEEP", "- k2", "PRODUCTION", "- p", "RISK", "- f", "CUSTOMERS", "- g"}, "\n"))
	if err != nil || len(huge) > MaxPlanBytes {
		t.Fatalf("one long line: %d bytes, %v", len(huge), err)
	}
}

func TestReviewPromptHasOnlyGoals(t *testing.T) {
	plan := "GOALS\n- sell the watch\n- LL to 20\nCASH\n- floor 600\nFACTIONS\n- d\nWINDOW\n- w\nSTOCK\n- e\nRISK\n- f\nCUSTOMERS\n- g"
	got := BuildReviewPrompt(9, plan, "cash 1 -> 2 (+1).")
	if !strings.Contains(got, "GOALS FOR DAY 9:\n- sell the watch\n- LL to 20\n\nWHAT HAPPENED ON DAY 9: cash 1 -> 2 (+1).") || strings.Contains(got, "floor 600") {
		t.Fatalf("review prompt:\n%s", got)
	}
}

func TestFacts(t *testing.T) {
	morning, _ := parseSnapshot(json.RawMessage(`{"store":{"cash":1000},"rent":{"rent":500,"days_left":2},
		"reputation":{"lower_levels":{"value":10},"security":{"value":3}},
		"inventory":[{"uid":1,"name":"Watch","stolen_heat":40},{"uid":2,"name":"Knife","contraband_level":1},{"uid":3,"name":"Lamp"},{"uid":4,"name":"Ring","stolen_heat":20}],
		"security":{"trafficking_value":100}}`))
	closing, _ := parseSnapshot(json.RawMessage(`{"store":{"cash":1450},"rent":{"rent":500,"arrears":0,"days_left":2},
		"reputation":{"lower_levels":{"value":12.5},"security":{"value":3}},
		"inventory":[{"uid":1,"name":"Watch","stolen_heat":40,"where":"stock"}],
		"security":{"trafficking_value":160}}`))
	st := newDayStats()
	for i, r := range []string{
		`{"kind":"trade","side":"sold","items":[2,3],"cash_delta":600,"rep_delta":{"lower_levels":2.5}}`,
		`{"kind":"trade","side":"bought","items":[9],"cash_delta":-150}`,
		`{"kind":"bargain","walked_out":true}`,
		`{"kind":"left","cause":"walked_out"}`,
		`{"kind":"left","cause":"dismissed"}`,
		`not json`,
	} {
		id := fmt.Sprintf("d%d", i)
		if i == 3 {
			id = "d2" // the same walk-out reported twice
		}
		st.addOutcome(id, json.RawMessage(r))
	}
	tests := []struct {
		name           string
		morning, close *snapshot
		want           []string
	}{
		{"both snapshots", morning, closing, []string{
			"cash 1000 -> 1450 (+450)", "rent 500 due in 2 day(s)", "reputation Lower Levels 10 -> 12.5",
			"1 sale(s) +600, 1 purchase(s) -150, 1 walk-out(s), 1 dismissed", "sold Knife #2, Lamp #3",
			"stock 4 -> 1 items", "risky stock: Watch #1 held (heat 40, in stock), Knife #2 sold, Ring #4 gone", "trafficking 100 -> 160",
		}},
		{"morning only", morning, nil, []string{
			"cash 1000 at open, +450 from trades", "reputation Lower Levels 10 -> 12.5", "Ring #4 not sold",
		}},
		{"nothing", nil, nil, []string{"reputation Lower Levels +2.5", "1 sale(s) +600"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := facts(tt.morning, tt.close, st)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("facts missing %q:\n%s", w, got)
				}
			}
		})
	}
}

// fakeLLM answers plan and review prompts and records what it was sent.
type fakeLLM struct {
	mu       sync.Mutex
	reqs     []llm.Request
	plans    int
	failPlan bool
	block    chan struct{}
}

func (f *fakeLLM) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		}
	}
	switch {
	case isDirectivesAsk(req):
		return llm.Response{Text: `{"cash_floor":0,"bargain":"ev_positive","scavenge":true}`, Model: "fake"}, nil
	case strings.HasPrefix(req.System, planSystem):
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failPlan {
			return llm.Response{}, errors.New("model down")
		}
		f.plans++
		return llm.Response{Text: fmt.Sprintf("CALENDAR\n- nothing\nGOALS\n- plan %d goal one\n- goal two\nCASH\n- floor 600\nFACTIONS\n- d\nWINDOW\n- w\nSTOCK\n- e\nKEEP\n- k2\nPRODUCTION\n- p\nRISK\n- f\nCUSTOMERS\n- g", f.plans), Model: "fake"}, nil
	case req.System == windfallSystem:
		return llm.Response{Text: "<think>hm</think>**WINDFALL:** blast module -> pivot? yes, 32 a night beats the path's next buy\n- buy it up to 150\n- no other change\n- a fourth line that is dropped", Model: "fake"}, nil
	case req.System == reviewSystem:
		return llm.Response{Text: "- MET: goal one (cash 1450)\n- NEXT: keep going", Model: "fake"}, nil
	}
	return llm.Response{}, errors.New("unexpected system prompt")
}

func (f *fakeLLM) requests() []llm.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]llm.Request(nil), f.reqs...)
}

// planRequests are the requests without the directives follow-ups.
func (f *fakeLLM) planRequests() []llm.Request {
	var out []llm.Request
	for _, r := range f.requests() {
		if !isDirectivesAsk(r) {
			out = append(out, r)
		}
	}
	return out
}

func isDirectivesAsk(req llm.Request) bool {
	n := len(req.Messages)
	return n > 0 && strings.HasPrefix(req.Messages[n-1].Content, directivesAsk)
}

type stubRecall struct{ got []*protocol.DecideRequest }

func (r *stubRecall) Brief(_ context.Context, req *protocol.DecideRequest, _ int) string {
	r.got = append(r.got, req)
	return "- lesson: zz_recall_lesson"
}

func event(kind string, day int, data string) protocol.Event {
	return protocol.Event{SessionID: "s1", Kind: kind, GameDay: day, Data: json.RawMessage(data)}
}

func snapshotEvent(phase string, day int, cash int) protocol.Event {
	return event("day_snapshot", day, fmt.Sprintf(`{"phase":%q,"store":{"cash":%d,"day":%d},"rent":{"rent":600,"days_left":2},"reputation":{"lower_levels":{"value":10}},"inventory":[{"uid":1,"name":"Watch","estimated_value":300}]}`, phase, cash, day))
}

func TestStrategistDay(t *testing.T) {
	f := &fakeLLM{}
	rec := &stubRecall{}
	s := New(Options{LLM: f, Recall: rec})
	defer s.Close()

	if _, _, ok := s.CurrentPlan("s1"); ok {
		t.Fatal("plan before any snapshot")
	}
	s.Hello("s1", protocol.HelloRequest{})
	s.Event("s1", snapshotEvent("morning", 10, 1000))
	s.wg.Wait()
	plan, day, ok := s.CurrentPlan("s1")
	if !ok || day != 10 || !strings.Contains(plan, "plan 1 goal one") {
		t.Fatalf("plan %q day %d ok %v", plan, day, ok)
	}
	if len(rec.got) != 1 || rec.got[0].SessionID != "s1" || rec.got[0].GameDay != 10 || rec.got[0].Hidden != nil {
		t.Fatalf("recall asked %+v", rec.got)
	}

	s.Decision("s1", &protocol.DecideRequest{GameDay: 10}, protocol.DecideResponse{})
	s.Event("s1", event("customer_observed", 10, `{}`))
	s.Event("s1", event("human_action", 10, `{"action":"sell_all","advised":"sell_all"}`))
	s.Event("s1", event("human_action", 10, `{"action":"dismiss","advised":"add_markup_10"}`))
	s.Event("s1", event("human_action", 10, `{"action":"stage","advised":"sell_all"}`))
	s.Outcome("s1", protocol.Outcome{DecisionID: "d1", GameDay: 10, Result: json.RawMessage(`{"kind":"trade","side":"sold","items":[1],"cash_delta":450}`)})
	s.Event("s1", snapshotEvent("close", 10, 1450))
	s.Event("s1", event("going_sleep", 10, `{}`))
	s.wg.Wait()

	sc, sday, ok := s.Scorecard("s1")
	if !ok || sday != 10 || !strings.HasPrefix(sc, "- MET: goal one (cash 1450)\n- NEXT: keep going\nNumbers: cash 1000 -> 1450 (+450)") {
		t.Fatalf("scorecard %q day %d", sc, sday)
	}
	for _, w := range []string{"sold Watch #1", "1 customer(s)", "advice followed 1 of 2"} {
		if !strings.Contains(sc, w) {
			t.Errorf("scorecard missing %q: %s", w, sc)
		}
	}
	reqs := f.requests()
	if review := reqs[len(reqs)-1]; review.System != reviewSystem || !strings.Contains(review.Messages[0].Content, "- plan 1 goal one") {
		t.Fatalf("review request %+v", review)
	}

	// The next morning opens with yesterday's scorecard, and a failed plan keeps day 10's.
	f.mu.Lock()
	f.failPlan = true
	f.mu.Unlock()
	s.Event("s1", snapshotEvent("morning", 11, 1450))
	s.wg.Wait()
	reqs = f.requests()
	morning := reqs[len(reqs)-1].Messages[0].Content
	if !strings.HasPrefix(morning, "YESTERDAY (day 10): cash 1000 -> 1450") ||
		!strings.Contains(morning, "YESTERDAY SCORED AGAINST ITS PLAN:\n- MET: goal one") ||
		!strings.Contains(morning, "YESTERDAY'S PLAN (day 10):\nCALENDAR\n- nothing\nGOALS\n- plan 1 goal one") ||
		!strings.Contains(morning, "FROM MEMORY OF PAST PLAY:\n- lesson: zz_recall_lesson") {
		t.Fatalf("morning prompt:\n%s", morning)
	}
	if plan, day, _ := s.CurrentPlan("s1"); day != 10 || !strings.Contains(plan, "plan 1") {
		t.Fatalf("failed plan replaced the previous one: day %d %q", day, plan)
	}
	if d, ok := s.Day("s1", 10); !ok || d.Plan == "" || d.Scorecard == "" || d.Facts == "" {
		t.Fatalf("day 10 record %+v", d)
	}

	// A reload in a new session continues the run; a far day jump starts a new one.
	f.mu.Lock()
	f.failPlan = false
	f.mu.Unlock()
	s.Event("s2", protocol.Event{SessionID: "s2", Kind: "waking_up", GameDay: 11})
	if _, day, ok := s.CurrentPlan("s2"); !ok || day != 10 {
		t.Fatalf("reloaded session lost the run's plan (day %d)", day)
	}
	s.Event("s3", protocol.Event{SessionID: "s3", Kind: "waking_up", GameDay: 40})
	if _, _, ok := s.CurrentPlan("s3"); ok {
		t.Fatal("a new run inherited a plan")
	}
}

func TestStrategistNeverBlocks(t *testing.T) {
	f := &fakeLLM{block: make(chan struct{})}
	s := New(Options{LLM: f})
	start := time.Now()
	for d := 1; d <= 5; d++ {
		s.Event("s1", snapshotEvent("morning", d, 100))
		s.Event("s1", event("going_sleep", d, `{}`))
		s.Outcome("s1", protocol.Outcome{GameDay: d, Result: json.RawMessage(`{}`)})
		s.CurrentPlan("s1")
	}
	if el := time.Since(start); el > 200*time.Millisecond {
		t.Fatalf("recording blocked for %v", el)
	}
	close(f.block)
	s.wg.Wait()
	if _, day, ok := s.CurrentPlan("s1"); !ok || day != 5 {
		t.Fatalf("plan for day %d after the queue drained, want 5", day)
	}
	s.Close()
}

func TestStrategistWithoutLLM(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	s.Event("s1", snapshotEvent("morning", 3, 500))
	s.Event("s1", snapshotEvent("close", 3, 700))
	s.Event("s1", event("going_sleep", 3, `{}`))
	s.wg.Wait()
	if _, _, ok := s.CurrentPlan("s1"); ok {
		t.Fatal("plan without a model")
	}
	if sc, _, ok := s.Scorecard("s1"); !ok || !strings.HasPrefix(sc, "Numbers: cash 500 -> 700") {
		t.Fatalf("scorecard %q", sc)
	}
}

// TestHiddenNeverReachesModel: events carrying a hidden block, and data with fields the
// strategist does not know, must produce byte-identical model requests and directives to
// clean ones.
func TestHiddenNeverReachesModel(t *testing.T) {
	const hidden = `"hidden":{"days_to_inspection":3,"zz_secret_key":"zz_secret_value","items":[{"uid":1,"true_value":987654321,"real_features":["zz_counterfeit"]},{"uid":77,"stolen_heat":90}],"store":{"cash":555444333,"business_permit":false},"rent":{"rent":444333222,"days_left":1},"security":{"trafficking_value":999}}`
	var directives []string
	run := func(withHidden bool) []llm.Request {
		f := &fakeLLM{}
		s := New(Options{LLM: f})
		defer s.Close()
		send := func(kind string, day int, data string) {
			raw := fmt.Sprintf(`{"session_id":"s1","seq":1,"kind":%q,"game_day":%d,"data":%s`, kind, day, data)
			if withHidden {
				raw += "," + hidden
			}
			var ev protocol.Event
			if err := json.Unmarshal([]byte(raw+"}"), &ev); err != nil {
				t.Fatal(err)
			}
			s.Event("s1", ev)
		}
		snap := `{"phase":%q,"store":{"cash":%d,"day":%d},"reputation":{"lower_levels":{"value":10}},"inventory":[{"uid":1,"name":"Watch","estimated_value":300,"stolen_heat":40}]}`
		send("day_snapshot", 5, fmt.Sprintf(snap, "morning", 1000, 5))
		send("customer_observed", 5, `{"client_seq":1,"covered":true}`)
		send("day_snapshot", 5, fmt.Sprintf(snap, "close", 1200, 5))
		send("going_sleep", 5, `{}`)
		send("day_snapshot", 6, fmt.Sprintf(snap, "morning", 1200, 6))
		s.wg.Wait()
		d, _, ok := s.CurrentDirectives("s1")
		if !ok {
			t.Fatal("no directives")
		}
		directives = append(directives, string(d))
		return f.requests()
	}
	with, without := run(true), run(false)
	if len(with) != 5 {
		t.Fatalf("%d model calls, want plan, directives, review, plan, directives", len(with))
	}
	a, _ := json.Marshal(with)
	b, _ := json.Marshal(without)
	if string(a) != string(b) {
		t.Fatalf("model requests differ when hidden is present:\nwith    %s\nwithout %s", a, b)
	}
	if directives[0] != directives[1] {
		t.Fatalf("directives differ when hidden is present:\nwith    %s\nwithout %s", directives[0], directives[1])
	}
	for _, s := range []string{"zz_", "987654321", "555444333", "444333222", "days_to_inspection", "#77"} {
		if strings.Contains(string(a), s) {
			t.Errorf("hidden %q reached the model", s)
		}
	}
	for _, s := range []string{"444333222", "[77", ",77", `"permit"`, `"hot_stock_cap"`} {
		if strings.Contains(directives[0], s) {
			t.Errorf("hidden %q reached the directives: %s", s, directives[0])
		}
	}
}

var (
	_ server.Recorder          = (*Strategist)(nil)
	_ server.DirectivesPlanner = (*Strategist)(nil)
	_ policy.Planner           = (*Strategist)(nil)
	_ Recall                   = (*memory.Store)(nil)
)

func TestLintPlan(t *testing.T) {
	snap := func(cash int) *snapshot {
		s, err := parseSnapshot(json.RawMessage(fmt.Sprintf(`{"store":{"cash":%d,"day":44},"rent":{"rent":700,"days_left":2},
			"inventory":[{"uid":901,"name":"Antique Clock","stolen_heat":35},{"uid":902,"name":"Combat Knife","contraband_level":1},{"uid":903,"name":"Toolkit"}]}`, cash)))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	tests := []struct {
		name, line string
		cash       int
		want       string
	}{
		{"selling hot stolen early", "- Sell Antique Clock #901 to clear the heat", 1800, "before it cools on day 46"},
		{"holding is fine", "- Hold Antique Clock #901 in the smuggler bay until day 46, then sell clean", 1800, ""},
		{"rent forces the sale", "- Sell Antique Clock #901 for rent", 400, ""},
		{"stolen to security", "- Sell the Clock #901 to Security for rep", 400, "to security"},
		{"contraband to security", "- sell combat knife to security officers", 1800, "to security"},
		{"never selling to security is fine", "- Never sell Combat Knife #902 to Security", 1800, ""},
		{"clean item", "- Sell Toolkit #903", 1800, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := lintPlan("GOALS\n"+tt.line, snap(tt.cash), 44)
			if tt.want == "" && err != nil || tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("err %v, want %q", err, tt.want)
			}
		})
	}
}
