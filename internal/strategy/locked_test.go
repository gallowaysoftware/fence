package strategy

import (
	"encoding/json"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestLintLocked(t *testing.T) {
	cap0, spend := int64(9), int64(300)
	for _, tc := range []struct {
		name, base string
		in         protocol.Directives
		want       string
		fixes      int
	}{
		{"no locks", `{"bargain":"never"}`, protocol.Directives{Day: 4, Bargain: "aggressive"}, `{"day":4,"bargain":"aggressive"}`, 0},
		{"top-level and nested", `{"locked":["bargain","appraisal","buy.hot_stock_cap","mortgage"]}`,
			protocol.Directives{Day: 4, Bargain: "aggressive", Appraisal: "join", Permit: "buy", Buy: &protocol.BuyDirectives{HotStockCap: &cap0}},
			`{"day":4,"permit":"buy"}`, 3},
		{"a nested sibling stays", `{"locked":["buy.hot_stock_cap"]}`,
			protocol.Directives{Day: 4, Buy: &protocol.BuyDirectives{HotStockCap: &cap0, MaxSpend: &spend}},
			`{"day":4,"buy":{"max_spend":300}}`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.in
			fixes := lintLocked(&d, tc.base)
			got, _ := json.Marshal(d)
			if string(got) != tc.want || len(fixes) != tc.fixes {
				t.Errorf("got %s %q, want %s and %d fixes", got, fixes, tc.want, tc.fixes)
			}
		})
	}
}
