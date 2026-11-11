package route

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRouteFiles: every shipped route, hard and challenge, loads with a start, perks, a persona
// that exists and a brief, and its base policy leaves the catalogue entry out.
func TestRouteFiles(t *testing.T) {
	for _, tc := range []struct {
		dir  string
		want int
	}{
		{filepath.Join("..", "..", DefaultDir), 12},
		{filepath.Join("..", "..", DefaultDir, "challenge"), 1},
	} {
		testRouteDir(t, tc.dir, tc.want)
	}
}

func testRouteDir(t *testing.T, dir string, want int) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(paths) < want {
		t.Fatalf("%d routes in %s (%v), want at least %d", len(paths), dir, err, want)
	}
	for _, p := range paths {
		name := strings.TrimSuffix(filepath.Base(p), ".json")
		r, err := Load(dir, name)
		if err != nil {
			t.Error(err)
			continue
		}
		if r.Name != name || r.Brief == "" || r.Persona == "" {
			t.Errorf("%s: name %q, persona %q, brief %q", p, r.Name, r.Persona, r.Brief)
		}
		if _, err := os.Stat(filepath.Join("..", "..", "docs", "personas", r.Persona+".md")); err != nil {
			t.Errorf("%s: persona %s: %v", name, r.Persona, err)
		}
		if strings.Contains(r.Base, "_route") || !strings.Contains(r.Base, `"appraisal":"join"`) {
			t.Errorf("%s: base %s", name, r.Base)
		}
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ok", `{"_route":{"start":"fence","perks":"vending","persona":"fence","brief":" A shop. "},"engine":"wine","bays":2}`)
	write("bare", `{"engine":"wine"}`)
	write("nostart", `{"_route":{"perks":"vending"}}`)
	write("long", `{"_route":{"start":"a","perks":"b","brief":"`+strings.Repeat("x", MaxBriefBytes+1)+`"}}`)
	r, err := Load(dir, "ok")
	if err != nil {
		t.Fatal(err)
	}
	if r.Start != "fence" || r.Perks != "vending" || r.Persona != "fence" || r.Text() != "ok: A shop." || r.Base != `{"bays":2,"engine":"wine"}` {
		t.Fatalf("%+v", r)
	}
	for ref, want := range map[string]string{"bare": "no _route", "nostart": "start and perks", "long": "the limit", "Bad Name": "not a name", "missing": "no such file"} {
		if _, err := Load(dir, ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v, want %q", ref, err, want)
		}
	}
	if b, err := StripMeta([]byte("{\n \"z\": 1, \"a\": 2\n}")); err != nil || b != `{"z":1,"a":2}` {
		t.Errorf("a file without _route is compacted as it stands: %s %v", b, err)
	}
}
