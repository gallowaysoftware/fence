package strategy

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestCatalogueLoads(t *testing.T) {
	raw, err := os.ReadFile("catalogue.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadCatalogue(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Items) < 40 {
		t.Fatalf("%d entries", len(c.Items))
	}
	for _, bad := range []string{
		`{"items":[{"key":"x","match":["("],"when":["always"],"source":"s"}]}`,
		`{"items":[{"key":"x","match":["x"],"when":["sometimes"],"source":"s"}]}`,
		`{"items":[{"key":"x","match":["x"],"when":["always"]}]}`,
	} {
		if _, err := LoadCatalogue([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestCatalogueLookup(t *testing.T) {
	tests := []struct{ id, name, want string }{
		{"water_filter", "", "water_filter"},
		{"water_filter_adv", "", "water_filter_adv"},
		{"", "Water Filter", "water_filter"},
		{"wine_superyeast", "", "wine_superyeast"},
		{"wine_yeast", "", "wine_yeast"},
		{"advanced_flux_agent", "", "advanced_flux_agent"},
		{"flux_agent", "", "flux_agent"},
		{"bloomberry_seed", "", "bloomberry_seed"},
		{"seed_packet", "Hydroponic Seeds", "seed"},
		{"wine_berry", "Bloomberry", "wine_berry"},
		{"testing_strip", "", "water_test_strip"},
		{"furnace_module_blast", "", "furnace_module_blast"},
		{"desk_lamp", "Desk Lamp", ""},
	}
	for _, tt := range tests {
		e, ok := catalogue.Lookup(tt.id, tt.name)
		got := ""
		if ok {
			got = e.Key
		}
		if got != tt.want {
			t.Errorf("Lookup(%q, %q) = %q, want %q", tt.id, tt.name, got, tt.want)
		}
	}
}

func TestBestFill(t *testing.T) {
	tests := []struct {
		name             string
		b, n, m, y, k, w int
		value            int
		wantFill         string
	}{
		// Two bottles, no yeast: pair the best berries (5 Reserve, 4 Production) and sell the rest.
		{"bottles scarce", 3, 2, 2, 0, 2, 0, 150 + 125 + 14*3, "1 x (best+best, no yeast -> Reserve 150), 1 x (best+normal, no yeast -> Production 125)"},
		// Plenty of bottles and yeast: one berry each beats pairing.
		{"bottles plenty", 0, 2, 0, 2, 2, 0, 200, "2 x (normal + yeast -> Homebrew 100)"},
		{"lone under-ripe needs yeast", 0, 0, 1, 1, 1, 0, 85, "1 x (under/over-ripe + yeast -> Bucket brew 85)"},
		{"no bottles", 1, 1, 1, 0, 0, 0, 42, ""},
		{"pure water lifts a grade", 2, 0, 0, 1, 1, 1, 175, "1 x (best+best + yeast -> Master Vintage 175)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := bestFill(tt.b, tt.n, tt.m, tt.y, tt.k, tt.w)
			if p.value != tt.value {
				t.Errorf("value %d, want %d", p.value, tt.value)
			}
			if got := p.describe(tt.w, ""); got != tt.wantFill {
				t.Errorf("fill %q, want %q", got, tt.wantFill)
			}
		})
	}
}

func TestIngotPurity(t *testing.T) {
	tests := []struct {
		ore, inputs int
		flux, blast bool
		want        int
	}{
		{0, 2, false, false, 6},
		{0, 2, true, false, 7},
		{1, 2, true, false, 8},
		{2, 2, true, false, 9},
		{0, 3, true, true, 7},
		{3, 3, true, true, 9},
		{0, 3, false, true, 5},
	}
	for _, tt := range tests {
		if got := ingotPurity(tt.ore, tt.inputs, tt.flux, tt.blast, 0); got != tt.want {
			t.Errorf("ingotPurity(%d/%d flux %v blast %v) = %d, want %d", tt.ore, tt.inputs, tt.flux, tt.blast, got, tt.want)
		}
	}
}

func TestUseNote(t *testing.T) {
	water := json.RawMessage(`{"inventory":[{"uid":1,"id":"water_filter","name":"Water Filter","estimated_value":25}],
		"water":{"taps":{"rust":true}}}`)
	dry := json.RawMessage(`{"inventory":[{"uid":1,"id":"water_filter","name":"Water Filter","estimated_value":25}]}`)
	tests := []struct {
		name, id, itemName string
		snap               json.RawMessage
		want               string
	}{
		{"open use", "water_filter", "Water Filter", water, "use ≈ 324 vs sale 140 (water: "},
		{"closed use", "water_filter", "", dry, "no use in this shop now (needs dirty water or a tap to clean"},
		{"no snapshot", "water_filter", "", nil, "use ≈ 324 vs sale 140"},
		{"berry grade from name", "wine_berry", "Moldy Bloomberry", nil, "use ≈ 0 vs sale 140"},
		{"module valued by what it enables", "furnace_module_blast", "", nil, "use ≈ 32 a night (448 over 14 nights) vs sale 140 (furnace: "},
		{"unknown item", "desk_lamp", "", water, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UseNote(tt.snap, tt.id, tt.itemName, 140)
			if tt.want == "" {
				if got != "" {
					t.Fatalf("got %q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("got %q, want containing %q", got, tt.want)
			}
		})
	}
}

func TestStrategistUseNote(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	s.Event("s1", event("day_snapshot", 9, `{"phase":"morning","store":{"day":9},"inventory":[{"uid":1,"id":"scrap_metal","name":"Scrap Metal","estimated_value":10}]}`))
	if got := s.UseNote("s1", "scrap_metal", "Scrap Metal", 10); !strings.Contains(got, "no use in this shop now (needs a furnace)") {
		t.Fatalf("got %q", got)
	}
	if got := s.UseNote("nobody", "scrap_metal", "", 10); !strings.HasPrefix(got, "use ≈ 21 vs sale 10") {
		t.Fatalf("unknown session: %q", got)
	}
}

func TestLintProduction(t *testing.T) {
	scs, err := LoadScenarios("testdata/days")
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]*snapshot{}
	for _, sc := range scs {
		s, err := parseSnapshot(sc.Event.Data)
		if err != nil {
			t.Fatal(err)
		}
		byName[sc.Name] = s
	}
	tests := []struct {
		scenario, plan, want string
	}{
		{"water-worn-filter-household-buyer", "STOCK\n- Sell 2 Water Filters to Mrs Ota for 90\nPRODUCTION\n- swap Water Filter #602 into the pitcher", "beats its sale"},
		{"water-worn-filter-household-buyer", "STOCK\n- Keep Water Filter #602, #604\nPRODUCTION\n- swap Water Filter #602 into the pitcher", ""},
		{"water-worn-filter-household-buyer", "WINDOW\n- Add Water Filter #604\nPRODUCTION\n- swap Water Filter #602 into the pitcher", "in the window"},
		{"water-worn-filter-household-buyer", "WINDOW\n- Remove Water Filter #603\nPRODUCTION\n- refill the pitcher", "filter is worn out"},
		{"water-basewater-to-berries", "STOCK\n- Sell Water Pitcher #912\nPRODUCTION\n- Bay #900: High Quality water from the purifier", "tested clean water"},
		{"water-basewater-to-berries", "STOCK\n- Sell Water Tank #910\nPRODUCTION\n- Bay #900: add tablets", "drinks Basewater"},
		{"water-basewater-to-berries", "STOCK\n- Sell Water Tank #910 into the event\nPRODUCTION\n- Bay #900: swap in High Quality pitcher #912", ""},
		{"furnace-scrap-no-flux", "STOCK\n- Sell 5 Scrap Metal to Foreman Rudd\nPRODUCTION\n- scavenge tonight", "beats its sale"},
		{"furnace-scrap-no-flux", "STOCK\n- Sell Junk #730\nPRODUCTION\n- Scavenging: skip tonight", "scavenge tonight"},
		{"wine-berries-two-bottles-no-yeast", "STOCK\n- Sell Moldy Bloomberry #517 as food", ""},
		{"wine-berries-two-bottles-no-yeast", "STOCK\n- Sell Bloomberry #515 #516 as food", "beats its sale"},
	}
	for _, tt := range tests {
		t.Run(tt.scenario+"/"+tt.want, func(t *testing.T) {
			s := byName[tt.scenario]
			if s == nil {
				t.Fatalf("no scenario %s", tt.scenario)
			}
			var msgs []string
			for _, e := range lintProduction(tt.plan, s) {
				msgs = append(msgs, e.Error())
			}
			got := strings.Join(msgs, "\n")
			if tt.want == "" && got != "" || tt.want != "" && !strings.Contains(got, tt.want) {
				t.Fatalf("lint %q, want %q", got, tt.want)
			}
		})
	}
}

// Hidden values in the production and water blocks (health, wine score, purity, an
// untested container's true grade) never reach the prompt, even rendered generically.
func TestProductionHiddenStaysOut(t *testing.T) {
	raw := json.RawMessage(`{"store":{"day":5},"inventory":[{"uid":1,"id":"water_jug","name":"Water Jug"}],
		"production":{"hydroponics":[{"uid":2,"seed":{"plant_id":"bloomberry"},"health":777777}],
			"wine":[{"uid":3,"state":"fermenting","score":777777,"nights_left":2}],
			"mystery":{"health":777777,"purity_bp":777777,"note":"zz_visible_ok"}},
		"water":{"containers":[{"uid":1,"label_grade":"Basewater","true_grade":"Ghostwater","purity_bp":777777,"parts":{"water":777777},"exposed":false}],
			"dirty_water_tracker":777777}}`)
	s, err := parseSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	s.render(&b, 5)
	out := b.String()
	for _, bad := range []string{"777777", "tested Ghostwater"} {
		if strings.Contains(out, bad) {
			t.Errorf("prompt has %q:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "zz_visible_ok") || !strings.Contains(out, "label Basewater") {
		t.Errorf("visible values lost:\n%s", out)
	}
}
