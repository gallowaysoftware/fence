package agency

import (
	"strings"
	"testing"
)

// TestCompositionPaths: the paths lever expands under the composition, is overridden by
// explicit levers, and rejects names the menu does not know.
func TestCompositionPaths(t *testing.T) {
	m := testMenu(t)
	if len(m.Paths) == 0 {
		t.Fatal("no paths loaded from the routes directory")
	}
	if _, ok := m.Paths["wine"]; !ok {
		t.Fatalf("wine path missing: %v", m.Paths)
	}

	// A composition naming paths validates clean and the merged directives carry the keys.
	c := Composition{Name: "pathed", Base: "water_baron", Levers: lev(`{"paths":["wine","fountain"]}`)}
	if errs := c.Validate(m); len(errs) > 0 {
		t.Fatalf("valid composition rejected: %v", errs)
	}
	base, _ := m.Get("water_baron")
	d, err := c.merged(m, base)
	if err != nil {
		t.Fatal(err)
	}
	if d["engine"] != "furnace+water" { // the base's engine wins: paths only fill gaps
		t.Fatalf("engine = %v, want the base's furnace+water", d["engine"])
	}
	if d["fountain"] != "buy" {
		t.Fatalf("fountain = %v, want buy from the path", d["fountain"])
	}

	// An explicit lever overrides the path's value.
	c2 := Composition{Name: "pathed2", Base: "water_baron", Levers: lev(`{"paths":["wine"],"bays":4}`)}
	d2, err := c2.merged(m, base)
	if err != nil {
		t.Fatal(err)
	}
	if d2["bays"] != float64(4) {
		t.Fatalf("bays = %v, want 4 (the lever over the path's 2)", d2["bays"])
	}

	// An unknown name is named in the errors.
	c3 := Composition{Name: "pathed3", Base: "water_baron", Levers: lev(`{"paths":["alchemy"]}`)}
	errs := c3.Validate(m)
	if !strings.Contains(strings.Join(errs, "; "), "alchemy") {
		t.Fatalf("errors %v do not name the unknown path", errs)
	}
}
