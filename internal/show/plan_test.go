package show

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestSplitFixes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"[]", nil},
		{"[window.show_uids dropped: the executor's window stands]", []string{"window.show_uids dropped: the executor's window stands"}},
		{"[sell.min_pct dropped while selling down sell.sell_down_to_cash 527 dropped: rent is covered window.show_uids dropped: the executor's window stands]",
			[]string{"sell.min_pct dropped while selling down", "sell.sell_down_to_cash 527 dropped: rent is covered", "window.show_uids dropped: the executor's window stands"}},
		{"[sell: no reason given; the default stands engine: wine dropped on hard, where the executor runs none]",
			[]string{"sell: no reason given; the default stands", "engine: wine dropped on hard, where the executor runs none"}},
		{"[cash_floor reverted: supplier day spends down to the bare rent]", []string{"cash_floor reverted: supplier day spends down to the bare rent"}},
	}
	for _, c := range cases {
		if got := splitFixes(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitFixes(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestReadBridgeLog(t *testing.T) {
	log := `time=2026-11-10T09:48:07.413-04:00 level=INFO msg="day plan written" day=2 bytes=19 ms=13559 directives_ms=1679 plan="GOALS\n- Sell the wine."
time=2026-11-10T09:48:07.413-04:00 level=INFO msg=directives day=2 json="{\"day\":2,\"bargain\":\"never\"}" why="{\"bargain\":\"Lower Levels near -20\"}"
time=2026-11-10T09:48:07.413-04:00 level=INFO msg="directives fixed" day=2 from_model="[bargain window]" fixes="[window.show_uids dropped: the executor's window stands]"
time=2026-11-10T09:48:08.001-04:00 level=INFO msg=decision session=s-1 decision_id=d-4 kind=barter option=defer source=fallback latency_ms=0
time=2026-11-10T09:50:00.000-04:00 level=WARN msg="day plan failed; keeping the previous plan" day=3 kept_day=2 err="context deadline exceeded"
`
	plans, err := ReadBridgeLog(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	p := plans[2]
	if p == nil || p.Text != "GOALS\n- Sell the wine." || p.Directives == nil || p.Directives.Bargain != "never" ||
		!reflect.DeepEqual(p.FromModel, []string{"bargain", "window"}) || len(p.Fixes) != 1 || p.Why["bargain"] != "Lower Levels near -20" {
		t.Fatalf("day 2 = %+v", p)
	}
	if !plans[3].Failed {
		t.Errorf("day 3 = %+v", plans[3])
	}
}

func TestParsePlan(t *testing.T) {
	text := "CALENDAR\n- day 35: Rent 292 due -> Secure cash.\nGOALS\n- Buy the referral.\n- Fill 9 wine bottles #1161 #1162 #1163 ….\n- Sell the vending machine.\n- A fourth goal.\nCASH\n- Floor: 292.\nRISK\n- Power: cut off."
	secs := ParsePlan(text)
	if len(secs) != 4 || secs[1].Title != "GOALS" || len(secs[1].Lines) != 4 {
		t.Fatalf("sections = %+v", secs)
	}
	want := []string{"Buy the referral.", "Fill 9 wine bottles.", "Sell the vending machine.", "Floor: 292."}
	if got := Headline(secs, 4); !reflect.DeepEqual(got, want) {
		t.Errorf("headline = %q", got)
	}
	if got := Headline(ParsePlan("Keep 600 for rent.\nHold the cassettes."), 4); len(got) != 2 {
		t.Errorf("untitled headline = %q", got)
	}
}

func TestDescribe(t *testing.T) {
	floor, spend := int64(400), int64(250)
	yes := true
	d := &protocol.Directives{CashFloor: &floor, Buy: &protocol.BuyDirectives{MaxSpend: &spend, PreferTypes: []string{"household_good", "weapon"}, AvoidTypes: []string{"weapon"}},
		Sell: &protocol.SellDirectives{HoldUIDs: []int64{1, 2, 3}}, UpgradesBuy: []string{"SHELF_X"}, Permit: "buy", Scavenge: &yes, Engine: "wine+furnace", Mortgage: "decline"}
	names := Names{Item: func(u int64) string { return map[int64]string{1: "Lamp", 2: "Lamp", 3: "Kettle"}[u] },
		Upgrade: func(id string) string { return map[string]string{"SHELF_X": "Better Shelf"}[id] }}
	want := []string{"keep at least 400 cash", "spend at most 250 on stock", "stock up on household good", "hold 2× Lamp and Kettle",
		"buy the Better Shelf", "buy the vendor permit", "scavenge tonight", "run wine + furnace", "refuse the mortgage"}
	if got := Describe(d, names); !reflect.DeepEqual(got, want) {
		t.Errorf("Describe\n got %q\nwant %q", got, want)
	}
	if got := Changes([]string{"a", "b"}, []string{"b", "c"}); !reflect.DeepEqual(got, []string{"c", "no longer: a"}) {
		t.Errorf("Changes = %q", got)
	}
}

func TestPlanReasonsAndGoals(t *testing.T) {
	b := NewBuilder(Player{ID: "p"})
	b.Base = &protocol.Directives{Engine: "wine+furnace", Permit: "buy", Sell: &protocol.SellDirectives{}}
	b.clock, b.haveClock = Clock{Cash: 4120, NetWorth: 9000, RentDue: 353, RentDays: 6, BuyoutPrice: 24000, OfferDown: 3500, OfferWeekly: 1000}, true

	ev := b.Plan(PlanInput{Day: 44, Text: "GOALS\n- Sell the wine before rent.", Directives: &protocol.Directives{Bargain: "never"},
		FromModel: []string{"bargain"}, Why: map[string]string{"bargain": "Lower Levels near -20"}, Fixes: []string{"window.show_uids dropped"}})
	if want := []string{"bargain: Lower Levels near -20", "overruled: window.show_uids dropped"}; !reflect.DeepEqual(ev.Plan.Why, want) {
		t.Errorf("why = %q", ev.Plan.Why)
	}

	ev = b.Plan(PlanInput{Day: 45})
	want := []string{"Day 44 plan: Sell the wine before rent.", "Saving for the mortgage: 4,120 / 6,000", "Rent 353 in 6 days: covered, 3,767 to spare",
		"Buyout: net worth 9,000 / 24,000", "Buy the vendor permit", "Run wine + furnace"}
	if ev.Plan.Source != "executor" || !reflect.DeepEqual(ev.Plan.Goals, want) {
		t.Errorf("goals = %q (source %s)", ev.Plan.Goals, ev.Plan.Source)
	}
	if ev.Text != "Day 45: the plan stands. Day 44 plan: Sell the wine before rent." {
		t.Errorf("text = %q", ev.Text)
	}
}

// Lines from the t7 drift game's day-175 plan, as the panel showed them before the tidy-up.
func TestStripUIDs(t *testing.T) {
	cases := map[string]string{
		"Sell all Water items (#9845, #9847, #9848). Target: ~768 cash.":           "Sell all Water items. Target: ~768 cash.",
		"Sell: PulseGuard #9477, 10mm Ammo #9726, all Water items (#9697, #9473).": "Sell: PulseGuard, 10mm Ammo, all Water items.",
		"Hold: Nutrifruit #9720, #9719 (Heat 7, sell clean day 150).":              "Hold: Nutrifruit (Heat 7, sell clean day 150).",
		"Fill 9 wine bottles #1161 #1162 #1163 ….":                                 "Fill 9 wine bottles.",
	}
	for in, want := range cases {
		if got := stripUIDs(in); got != want {
			t.Errorf("stripUIDs(%q) = %q, want %q", in, got, want)
		}
	}
}
