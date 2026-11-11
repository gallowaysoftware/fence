package show

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestBaseDirectives(t *testing.T) {
	dir := t.TempDir()
	routeFile := filepath.Join(dir, "sprinter.json")
	os.WriteFile(routeFile, []byte(`{"_route":{"start":"apprentice","perks":"vending","brief":"Race."},"engine":"wine+furnace","permit":"buy"}`), 0o644)
	baseFile := filepath.Join(dir, "base.json")
	os.WriteFile(baseFile, []byte(`{"engine":"wine"}`), 0o644)

	d, err := BaseDirectives(routeFile, "")
	if err != nil || d == nil || d.Engine != "wine+furnace" || d.Permit != "buy" {
		t.Fatalf("route only: %+v, %v; want the route's directives", d, err)
	}
	if d, err = BaseDirectives(routeFile, baseFile); err != nil || d.Engine != "wine" || d.Permit != "" {
		t.Errorf("route and file: %+v, %v; want the file, as the bridge reads it", d, err)
	}
	if d, err = BaseDirectives("", ""); d != nil || err != nil {
		t.Errorf("neither: %+v, %v", d, err)
	}

	// A live panel's standing plan names the base's engine and permit, as a league replay's does.
	b := NewBuilder(Player{ID: "live"})
	b.Base, _ = BaseDirectives(routeFile, "")
	goals := strings.Join(b.goals(), "\n")
	if !strings.Contains(strings.ToLower(goals), "furnace") || !strings.Contains(strings.ToLower(goals), "permit") {
		t.Errorf("goals without the base's engine and permit:\n%s", goals)
	}
}

// The rehearsal's sprinter panel read "Saving for the mortgage: 26,888 / 7,143" on a route that
// buys for cash, and "mortgage offered" under "the shop is yours".
func TestCashRouteGoalsAndOwnedShop(t *testing.T) {
	var log bytes.Buffer
	for _, l := range []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r"}),
		snapshot("morning", 97, 26888, 30000, map[string]any{"visible": false}),
	} {
		b, _ := json.Marshal(l)
		log.Write(append(b, '\n'))
	}
	cash := int64(140)
	b := NewBuilder(Player{ID: "p"})
	b.Base = &protocol.Directives{BuyoutByDay: &cash}
	if err := EachEvent(&log, b, Window{}, func(Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	goals := strings.Join(b.goals(), "\n")
	if !strings.Contains(goals, "Saving for the buyout: 26,888 / 24,000") || strings.Contains(goals, "mortgage") {
		t.Errorf("cash route goals:\n%s", goals)
	}

	owned := snapshot("morning", 98, 1049, 30181, map[string]any{"visible": false})
	owned["event"].(map[string]any)["data"].(map[string]any)["property"] = map[string]any{"owned": true, "buyout_price": 26400,
		"mortgage_offer": map[string]any{"down_payment": 3500, "weekly": 1762}}
	line, _ := json.Marshal(owned)
	if _, err := b.Feed(line); err != nil {
		t.Fatal(err)
	}
	if c := b.Clock(); !c.Owned || c.OfferDown != 0 {
		t.Errorf("owned shop clock = %+v, want no mortgage offer", c)
	}
}
