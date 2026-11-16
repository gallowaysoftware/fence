package strategy

import "testing"

func TestMiniLabParse(t *testing.T) {
	raw := []byte(`{"phase":"morning","lab":{"unlocked":true,"scanner":true},"store":{"day":2}}`)
	s, err := parseSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("productionRaw=%q", string(s.productionRaw))
	t.Logf("labRaw=%q", string(s.labRaw))
	if len(s.labRaw) == 0 {
		t.Fatal("labRaw empty")
	}
	if s.production().lab == nil {
		t.Fatal("lab nil after parseProduction")
	}
}
