package strategy

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// directivesShop: day 41, cash 2000, rent 700 due in 2 days, power 40 a night, insurance
// offered but off, landlord grace and an aug scanner on offer, no permit, a smuggler bay
// with 3 free cells holding one cooling stolen item, trafficking 45, hard difficulty, the
// dumping ground open.
const directivesShop = `{"phase":"morning","store":{"cash":2000,"day":41,"wild_favor":100,"business_permit":false,"difficulty":"hard"},
 "rent":{"rent":700,"days_left":2,"landlord_grace":false},
 "services":[{"id":"POWER_SERVICE","name":"Power","cost":40,"using":true},{"id":"INSURANCE_SERVICE","name":"Insurance","cost":30,"using":false},{"id":"JANITORIAL_SERVICE","name":"Janitorial","cost":20,"using":false}],
 "upgrades":{"owned":[],"available":[{"id":"LANDLORD_GRACE","name":"Landlord Grace","cost":400},{"id":"AUG_SCANNER","name":"Aug Scanner","cost":900}]},
 "security":{"trafficking_value":45},
 "inventory":[{"uid":1,"id":"smuggler_bay","name":"Smuggler Bay","where":"stock","capacity":{"cells":6,"free":3}},
  {"uid":2,"name":"Radio","where":"inside:1","estimated_value":120,"stolen_heat":40},
  {"uid":3,"name":"Synth Watch","where":"showcase","estimated_value":380,"types":["household_good"]},
  {"uid":4,"name":"Knife","where":"stock","estimated_value":90,"contraband_level":1}],
 "market_events":{"active":[],"calendar":[{"day":42,"events":[{"name":"Boom","item_type":"food","value":20}]},{"day":44,"events":[{"name":"Glut","item_type":"weapon","value":-15}]},{"day":60,"events":[{"name":"Late","item_type":"tool","value":30}]}]},
 "afterhours":[{"location":"dumping_ground","available":true}]}`

func shop(t *testing.T, patch func(m map[string]any)) *snapshot {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(directivesShop), &m); err != nil {
		t.Fatal(err)
	}
	if patch != nil {
		patch(m)
	}
	raw, _ := json.Marshal(m)
	s, err := parseSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func store(m map[string]any) map[string]any { return m["store"].(map[string]any) }

func TestDeriveDirectives(t *testing.T) {
	plan := "CASH\n- floor 500 (rent day 42)\nWINDOW\n- put Synth Watch #3 in\n- never Knife #4\n- remove Radio #2\nGOALS\n- BUY tonight: Landlord Grace (400: favor 100 + cash 300; rent)"
	d := deriveDirectives(plan, shop(t, nil), "", 40)
	got, _ := json.Marshal(d)
	// Floor: rent 700 + power 40 x 3 nights, over the plan's 500; hot cap: 1 in the bay + 3 free
	// cells; power off, nothing draws it.
	want := `{"day":40,"cash_floor":820,"buy":{"wholesale_max_pct":100,"single_max_pct":80,"max_spend":1180,"prefer_types":["food"],"avoid_types":["weapon"],"hot_stock_cap":4},` +
		`"sell":{"hold_uids":[2]},"bargain":"ev_positive","window":{"show_uids":[3],"avoid_stolen":true},` +
		`"services":["INSURANCE_SERVICE","JANITORIAL_SERVICE"],"upgrades_buy":["LANDLORD_GRACE"],"permit":"bribe","augs":"shoot","scavenge":true}`
	if string(got) != want {
		t.Fatalf("derived\n%s\nwant\n%s", got, want)
	}
	if fixes := lintDirectives(&d, shop(t, nil), 40); len(fixes) > 0 {
		t.Errorf("derived directives fail their own lints: %v", fixes)
	}
}

func TestParseDirectives(t *testing.T) {
	tests := []struct {
		name, raw string
		want      string // the parsed directives as JSON
		fixes     []string
		err       string
	}{
		{"loose numbers and names", `{"cash_floor":"1,200","buy":{"single_max_pct":"85%","max_spend":300.4,"prefer_types":["Household Good"]},"sell":{"hold_uids":["#2",3]},"services":["insurance","Janitorial"],"upgrades_buy":["Landlord Grace","landlord_grace"],"bargain":"EV-positive"}`,
			`{"cash_floor":1200,"buy":{"single_max_pct":85,"max_spend":300,"prefer_types":["household_good"]},"sell":{"hold_uids":[2,3]},"bargain":"ev_positive","services":["INSURANCE_SERVICE","JANITORIAL_SERVICE"],"upgrades_buy":["LANDLORD_GRACE"]}`, nil, ""},
		{"out of range and unknown", `{"cash_floor":-5,"buy":{"wholesale_max_pct":140},"sell":{"hold_uids":[99]},"window":{"show_uids":[3,98]},"augs":"pet","permit":"steal","upgrades_buy":["MOON_BASE"],"services":["SPA"],"mood":"good"}`,
			`{"cash_floor":0,"buy":{"wholesale_max_pct":100},"window":{"show_uids":[3]}}`,
			[]string{`dropped unknown key "mood"`, "cash_floor: -5 clamped to 0", "buy.wholesale_max_pct: 140 clamped to 100", "sell.hold_uids: dropped unknown uid 99", "window.show_uids: dropped unknown uid 98", `services: dropped unknown service "SPA"`, `upgrades_buy: dropped "MOON_BASE", not on offer`, "permit: dropped steal", "augs: dropped pet"}, ""},
		{"production", `{"production":[{"machine_uid":3,"action":"fill wine","items":[2,77],"note":"x"},{"machine_uid":55,"action":"run"},{"machine_uid":3}]}`,
			`{"production":[{"machine_uid":3,"action":"fill wine","items":[2],"note":"x"}]}`,
			[]string{"production.items: dropped unknown uid 77", "production: dropped 55", "production: dropped 3"}, ""},
		{"no contract keys", `{"plan":"sell everything"}`, `{}`, []string{`dropped unknown key "plan"`}, "none of the contract's keys"},
		{"not an object", `[1,2]`, `{}`, nil, "not a JSON object"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _, fixes, err := parseDirectives(json.RawMessage(tt.raw), shop(t, nil))
			if tt.err == "" && err != nil || tt.err != "" && (err == nil || !strings.Contains(err.Error(), tt.err)) {
				t.Fatalf("err %v, want %q", err, tt.err)
			}
			if got, _ := json.Marshal(d); string(got) != tt.want {
				t.Errorf("parsed\n%s\nwant\n%s", got, tt.want)
			}
			all := strings.Join(fixes, "\n")
			for _, f := range tt.fixes {
				if !strings.Contains(all, f) {
					t.Errorf("fixes lack %q:\n%s", f, all)
				}
			}
			if len(fixes) != len(tt.fixes) {
				t.Errorf("%d fixes, want %d:\n%s", len(fixes), len(tt.fixes), all)
			}
		})
	}
}

func TestLintDirectives(t *testing.T) {
	i := func(v int64) *int64 { return &v }
	f := false
	tests := []struct {
		name  string
		patch func(m map[string]any)
		in    protocol.Directives
		check func(t *testing.T, d protocol.Directives)
		fix   string
	}{
		{"floor raised to the rent hold", nil, protocol.Directives{CashFloor: i(0)},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.CashFloor, 820) }, "raised to 820"},
		{"floor above cash", func(m map[string]any) { store(m)["cash"] = 500 }, protocol.Directives{CashFloor: i(900)},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.CashFloor, 500) }, "above cash 500"},
		{"no floor far from rent", func(m map[string]any) { m["rent"].(map[string]any)["days_left"] = 5 }, protocol.Directives{},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.CashFloor, 0) }, ""},
		{"max spend within cash above the floor", nil, protocol.Directives{CashFloor: i(820), Buy: &protocol.BuyDirectives{MaxSpend: i(5000)}},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.Buy.MaxSpend, 1180) }, "lowered to 1180"},
		{"sell down to the rent hold when short", func(m map[string]any) { store(m)["cash"] = 300 },
			protocol.Directives{Sell: &protocol.SellDirectives{SellDownToCash: i(500), HoldUIDs: []int64{3}}},
			func(t *testing.T, d protocol.Directives) {
				eq(t, *d.Sell.SellDownToCash, 820)
				if len(d.Sell.HoldUIDs) != 0 {
					t.Errorf("holds %v; short of rent, the watch must be sellable and the radio may be sold hot", d.Sell.HoldUIDs)
				}
			}, "raised to 820"},
		{"hot cap to bay room", nil, protocol.Directives{Buy: &protocol.BuyDirectives{HotStockCap: i(10)}},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.Buy.HotStockCap, 4) }, "hot_stock_cap set to 4"},
		{"hot cap free under 40", func(m map[string]any) { m["security"] = map[string]any{"trafficking_value": 20} },
			protocol.Directives{Buy: &protocol.BuyDirectives{HotStockCap: i(10)}},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.Buy.HotStockCap, 10) }, ""},
		{"no bay at trafficking 45", func(m map[string]any) {
			m["inventory"] = []any{map[string]any{"uid": 3, "name": "Synth Watch", "estimated_value": 380}}
		}, protocol.Directives{},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.Buy.HotStockCap, 0) }, "set to 0"},
		{"upgrades within favor and cash above the floor", func(m map[string]any) { store(m)["cash"] = 1500 },
			protocol.Directives{UpgradesBuy: []string{"AUG_SCANNER", "LANDLORD_GRACE"}},
			func(t *testing.T, d protocol.Directives) {
				// Free cash 680 and favor 100: the scanner (900) is out, grace (400) in.
				if strings.Join(d.UpgradesBuy, ",") != "LANDLORD_GRACE" {
					t.Errorf("upgrades %v", d.UpgradesBuy)
				}
			}, "dropped AUG_SCANNER"},
		{"grace may use the rent reserve", func(m map[string]any) { store(m)["cash"] = 800 },
			protocol.Directives{UpgradesBuy: []string{"LANDLORD_GRACE"}},
			func(t *testing.T, d protocol.Directives) { eq(t, int64(len(d.UpgradesBuy)), 1) }, ""},
		{"permit buy unaffordable", nil, protocol.Directives{Permit: "buy"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Permit, "bribe") }, "bribe instead"},
		{"permit refused", func(m map[string]any) { store(m)["cash"] = 4000 }, protocol.Directives{Permit: "refuse"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Permit, "buy") }, "arrest by day 80"},
		{"permit refused, state unknown", func(m map[string]any) { delete(store(m), "business_permit") }, protocol.Directives{Permit: "refuse"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Permit, "bribe") }, "arrest by day 80"},
		{"type both preferred and avoided", nil,
			protocol.Directives{Buy: &protocol.BuyDirectives{PreferTypes: []string{"weapon", "food"}, AvoidTypes: []string{"module", "weapon"}}},
			func(t *testing.T, d protocol.Directives) {
				eq(t, strings.Join(d.Buy.PreferTypes, ","), "food")
				eq(t, strings.Join(d.Buy.AvoidTypes, ","), "module")
			}, "weapon is both preferred and avoided"},
		{"permit held", func(m map[string]any) { store(m)["business_permit"] = true }, protocol.Directives{Permit: "buy"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Permit, "") }, "the permit is held"},
		{"no stolen, contraband or kept goods in the window", nil,
			protocol.Directives{Window: &protocol.WindowDirectives{ShowUIDs: []int64{2, 3, 4}, AvoidStolen: &f}},
			func(t *testing.T, d protocol.Directives) {
				if len(d.Window.ShowUIDs) != 1 || d.Window.ShowUIDs[0] != 3 || !*d.Window.AvoidStolen {
					t.Errorf("window %+v", *d.Window)
				}
			}, "dropped stolen or contraband #2"},
		{"cooling stolen held", nil, protocol.Directives{},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Sell.HoldUIDs[0], 2) }, "added #2"},
		{"augs shot on hard", nil, protocol.Directives{Augs: "serve"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Augs, "shoot") }, "aug presence ends the run"},
		{"augs free on normal", func(m map[string]any) { store(m)["difficulty"] = "normal" }, protocol.Directives{Augs: "serve"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Augs, "serve") }, ""},
		{"scavenge when open", nil, protocol.Directives{Scavenge: &f},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.Scavenge, true) }, "the dumping ground is open"},
		{"permit named when unset", nil, protocol.Directives{},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Permit, "bribe") }, "unset lets the executor choose"},
		{"permit bought with a margin", func(m map[string]any) { store(m)["cash"] = 3200 }, protocol.Directives{Permit: "bribe"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Permit, "buy") }, "keeps logging unlicensed crime"},
		{"no bargaining below Lower Levels -10", func(m map[string]any) {
			m["reputation"] = map[string]any{"lower_levels": map[string]any{"value": -12}}
		}, protocol.Directives{Bargain: "ev_positive"},
			func(t *testing.T, d protocol.Directives) { eq(t, d.Bargain, "never") }, "below -10"},
		{"power off without a powered machine", nil, protocol.Directives{Services: []string{"POWER_SERVICE", "INSURANCE_SERVICE"}},
			func(t *testing.T, d protocol.Directives) {
				eq(t, strings.Join(d.Services, ","), "INSURANCE_SERVICE,JANITORIAL_SERVICE")
			}, "power off"},
		{"power kept for a recharger", func(m map[string]any) {
			m["inventory"] = append(m["inventory"].([]any), map[string]any{"uid": 9, "id": "battery_recharger", "estimated_value": 200})
		}, protocol.Directives{Services: []string{"POWER_SERVICE", "INSURANCE_SERVICE", "JANITORIAL_SERVICE"}},
			func(t *testing.T, d protocol.Directives) {
				eq(t, strings.Join(d.Services, ","), "POWER_SERVICE,INSURANCE_SERVICE,JANITORIAL_SERVICE")
			}, ""},
		{"min_pct dropped while selling down", func(m map[string]any) { store(m)["cash"] = 300 },
			protocol.Directives{Sell: &protocol.SellDirectives{MinPct: i(95)}},
			func(t *testing.T, d protocol.Directives) {
				if d.Sell.MinPct != nil {
					t.Errorf("min_pct %d kept", *d.Sell.MinPct)
				}
			}, "liquidation floor"},
		{"favor order first", func(m map[string]any) {
			m["upgrades"] = map[string]any{"owned": []any{
				map[string]any{"id": "LANDLORD_GRACE"}}, "available": []any{map[string]any{"id": "SHOWCASE_I", "cost": 200},
				map[string]any{"id": "BUY_REFERAL", "cost": 200}, map[string]any{"id": "WILD_FIXER", "cost": 350}}}
			store(m)["difficulty"] = "normal"
			store(m)["cash"], store(m)["wild_favor"] = 4000, 2000
		}, protocol.Directives{UpgradesBuy: []string{"WILD_FIXER"}},
			func(t *testing.T, d protocol.Directives) {
				eq(t, strings.Join(d.UpgradesBuy, ","), "SHOWCASE_I,BUY_REFERAL,WILD_FIXER")
			},
			"favor order (SHOWCASE_I, BUY_REFERAL) goes first"},
		{"cash upgrades keep rent and a margin", func(m map[string]any) {
			m["upgrades"] = map[string]any{"available": []any{map[string]any{"id": "SHOWCASE_I", "cost": 200},
				map[string]any{"id": "BUY_REFERAL", "cost": 200}, map[string]any{"id": "WILD_FIXER", "cost": 350}}}
			store(m)["difficulty"] = "normal"
		}, protocol.Directives{UpgradesBuy: []string{"WILD_FIXER"}},
			func(t *testing.T, d protocol.Directives) { eq(t, strings.Join(d.UpgradesBuy, ","), "SHOWCASE_I") },
			"the cash left would not keep rent and 1200"},
		{"supplier day spends down to the bare rent", func(m map[string]any) { store(m)["day"] = 40 }, protocol.Directives{CashFloor: i(0)},
			func(t *testing.T, d protocol.Directives) { eq(t, *d.CashFloor, 700) }, "raised to 700"},
		{"hard skips the showcase", func(m map[string]any) {
			m["upgrades"] = map[string]any{"available": []any{map[string]any{"id": "SHOWCASE_I", "cost": 200}, map[string]any{"id": "BUY_REFERAL", "cost": 200}}}
		}, protocol.Directives{},
			func(t *testing.T, d protocol.Directives) { eq(t, strings.Join(d.UpgradesBuy, ","), "BUY_REFERAL") }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.in
			fixes := strings.Join(lintDirectives(&d, shop(t, tt.patch), 40), "\n")
			tt.check(t, d)
			if tt.fix != "" && !strings.Contains(fixes, tt.fix) {
				t.Errorf("fixes lack %q:\n%s", tt.fix, fixes)
			}
		})
	}
}

func eq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFinishDirectives(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		rawErr    error
		fromModel string
		err       bool
		check     func(t *testing.T, d protocol.Directives)
	}{
		{"model keys kept, the rest derived", `{"bargain":"never","buy":{"single_max_pct":60}}`, nil, "buy,bargain", false,
			func(t *testing.T, d protocol.Directives) {
				eq(t, d.Bargain, "never")
				if d.Buy.SingleMaxPct != nil || d.Buy.WholesaleMaxPct != nil || d.Buy.MaxSpend != nil || d.CashFloor != nil {
					t.Errorf("buying limited while rent is covered: %+v floor %v", *d.Buy, d.CashFloor)
				}
				eq(t, d.Augs, "shoot")
				eq(t, d.Day, 40)
			}},
		{"no upgrades still spends favor", `{"upgrades_buy":[]}`, nil, "upgrades_buy", false,
			func(t *testing.T, d protocol.Directives) { eq(t, strings.Join(d.UpgradesBuy, ","), "LANDLORD_GRACE") }},
		{"call failed", "", errors.New("timeout"), "", true,
			func(t *testing.T, d protocol.Directives) {
				eq(t, d.Bargain, "ev_positive")
				eq(t, d.CashFloor == nil, true)
			}},
		{"garbage", `{"note":"x"}`, nil, "", true,
			func(t *testing.T, d protocol.Directives) { eq(t, d.Bargain, "ev_positive") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := finishDirectives(json.RawMessage(tt.raw), tt.rawErr, "CASH\n- floor 0", shop(t, nil), "", 40)
			if (r.Err != nil) != tt.err {
				t.Fatalf("err %v", r.Err)
			}
			eq(t, strings.Join(r.FromModel, ","), tt.fromModel)
			tt.check(t, r.Directives)
		})
	}
}

func TestDirectivesPrompt(t *testing.T) {
	p := directivesPrompt(shop(t, nil))
	for _, w := range []string{"Service ids: POWER_SERVICE (Power, on), INSURANCE_SERVICE (Insurance, off)", "Upgrade ids on offer: LANDLORD_GRACE (Landlord Grace (400))", "Business permit: not held (2000); say bribe or buy every day.", "in the order Wild favor buys them"} {
		if !strings.Contains(p, w) {
			t.Errorf("prompt lacks %q:\n%s", w, p)
		}
	}
}

// Snapshots recorded before 5bcd757 listed for-sale upgrades (state 1) under owned.
func TestParseLegacyUpgrades(t *testing.T) {
	s := snap(t, `{"store":{"day":30},"upgrades":{"owned":[{"id":"LANDLORD_GRACE","name":"Rent Grace","state":2},{"id":"SHOWCASE_I","name":"Showcase Expansion I","state":1},"Old Name"],"available":[{"id":"X","name":"X","cost":5}]}}`)
	eq(t, strings.Join(s.UpgradesOwned, ","), "Rent Grace,Old Name")
	eq(t, len(s.UpgradesOffer), 1)
}

// wineShop: normal, rent 300 due in 5 days, insurance 10 a night on, one wine of each grade
// from Bucket Brew to Reserve and an empty bottle.
const wineShop = `{"store":{"cash":%d,"day":90,"difficulty":"normal","business_permit":true},"rent":{"rent":300,"days_left":5},
 "services":[{"id":"INSURANCE_SERVICE","cost":10,"using":true}],
 "inventory":[{"uid":1,"id":"wine_bottle","estimated_value":90,"features":[{"text":"Bucket Brew (-15%%)"}]},
  {"uid":2,"id":"wine_bottle","estimated_value":110,"features":[{"text":"Homebrew (+0%%)"}]},
  {"uid":3,"id":"wine_bottle","estimated_value":140,"features":[{"text":"Production Wine (+25%%)"}]},
  {"uid":4,"id":"wine_bottle","estimated_value":180,"features":[{"text":"Reserve (+50%%)"}]},
  {"uid":5,"id":"wine_bottle","estimated_value":12}]}`

func TestWineHolds(t *testing.T) {
	tests := []struct {
		name string
		cash int
		want string
	}{
		{"liquid: Homebrew and better age", 1370, "2,3,4"},
		{"short of the buying margin: only Reserve", 1369, "4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := snap(t, fmt.Sprintf(wineShop, tt.cash))
			d := protocol.Directives{Sell: &protocol.SellDirectives{HoldUIDs: []int64{1, 2, 3, 4}}}
			lintDirectives(&d, s, 90)
			var got []string
			for _, u := range d.Sell.HoldUIDs {
				got = append(got, fmt.Sprint(u))
			}
			slices.Sort(got)
			eq(t, strings.Join(got, ","), tt.want)
		})
	}
}

func TestEngineIdle(t *testing.T) {
	var h WineHistory
	h.Note(80, json.RawMessage(fmt.Sprintf(wineShop, 500)))
	s := snap(t, fmt.Sprintf(wineShop, 500))
	for _, tt := range []struct {
		day  int
		in   string
		want string
	}{{84, "none", ""}, {85, "", "none"}, {90, "wine", "none"}} {
		s.wineIdle = h.Idle(tt.day)
		d := protocol.Directives{Engine: tt.in}
		lintDirectives(&d, s, tt.day)
		eq(t, d.Engine, tt.want)
	}
	if (&WineHistory{}).Idle(10) != nil {
		t.Error("idle before any wine")
	}
}

func TestProtectBuying(t *testing.T) {
	i := func(v int64) *int64 { return &v }
	d := protocol.Directives{CashFloor: i(900), Buy: &protocol.BuyDirectives{MaxSpend: i(40), SingleMaxPct: i(60)}}
	short := shop(t, func(m map[string]any) { store(m)["cash"] = 300 })
	if fixes := protectBuying(&d, short); len(fixes) > 0 || d.CashFloor == nil {
		t.Errorf("rent short: limits dropped %v", fixes)
	}
	protectBuying(&d, shop(t, nil))
	if d.CashFloor != nil || d.Buy.MaxSpend != nil || d.Buy.SingleMaxPct != nil {
		t.Errorf("rent covered: limits kept %+v", d)
	}
}
