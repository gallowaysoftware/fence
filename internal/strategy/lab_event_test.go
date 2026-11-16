package strategy

import (
	"os"
	"strings"
	"testing"
)

func TestLabEventRender(t *testing.T) {
	raw, err := os.ReadFile("/tmp/lab_event.json")
	if err != nil {
		t.Skip("no event fixture")
	}
	s, err := parseSnapshot(raw)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	s.production().renderLab(&b)
	t.Logf("renderLab output:\n%s", b.String())
	if s.production().lab == nil {
		t.Fatal("ps.lab nil after parseProduction")
	}
	if s.production().lab == nil {
		t.Fatal("ps.lab nil after parseProduction")
	}
}
