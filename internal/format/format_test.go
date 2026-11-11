package format

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/route"
)

// TestShippedFormats: every format loads, names a route that exists on its difficulty, and its
// directives still read as that route with the handicaps in force and locked.
func TestShippedFormats(t *testing.T) {
	root := filepath.Join("..", "..")
	fs, err := List(filepath.Join(root, DefaultDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) < 6 {
		t.Fatalf("%d formats", len(fs))
	}
	for _, f := range fs {
		path, err := f.RouteFile("")
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		out, err := f.Apply(b)
		if err != nil {
			t.Fatal(err)
		}
		r, err := route.Parse(path, out)
		if err != nil {
			t.Fatalf("%s: %v", f.Name, err)
		}
		var d map[string]any
		if err := json.Unmarshal([]byte(r.Base), &d); err != nil {
			t.Fatal(err)
		}
		for _, id := range f.Handicaps {
			h, _ := HandicapByID(id)
			for _, k := range h.Locked {
				if !slices.Contains(d["locked"].([]any), any(k)) {
					t.Errorf("%s: %s not locked", f.Name, k)
				}
			}
		}
		if f.Lab.Runs == 0 {
			t.Errorf("%s: no lab record", f.Name)
		}
		if w := f.Lab.Words(); strings.ContainsAny(w, "0123456789") {
			t.Errorf("%s: odds in words hold a digit: %s", f.Name, w)
		}
	}
}

func TestApply(t *testing.T) {
	f := Format{Name: "x", Difficulty: "hard", Handicaps: []string{"clean_shop", "no_haggle", "no_board"}}
	out, err := f.Apply([]byte(`{"_route":{"start":"a","perks":"b"},"bargain":"discount","appraisal":"join","buy":{"single_max_pct":75},"locked":["permit"]}`))
	if err != nil {
		t.Fatal(err)
	}
	var d struct {
		Route     map[string]string `json:"_route"`
		Format    string            `json:"_format"`
		Bargain   string            `json:"bargain"`
		BargainB  string            `json:"bargain_buy"`
		Appraisal string            `json:"appraisal"`
		Buy       map[string]int    `json:"buy"`
		Locked    []string          `json:"locked"`
	}
	if err := json.Unmarshal(out, &d); err != nil {
		t.Fatal(err)
	}
	want := []string{"permit", "buy.hot_stock_cap", "bargain", "bargain_buy", "appraisal"}
	if d.Route["start"] != "a" || d.Format != "x" || d.Bargain != "never" || d.BargainB != "never" || d.Appraisal != "off" ||
		d.Buy["single_max_pct"] != 75 || d.Buy["hot_stock_cap"] != 0 || len(d.Buy) != 2 || !slices.Equal(d.Locked, want) {
		t.Fatalf("%s", out)
	}
	again, _ := f.Apply(out)
	if string(again) != string(out) {
		t.Errorf("applying twice changes the file:\n%s\n%s", out, again)
	}
}

func TestParse(t *testing.T) {
	ok := `{"title":"T","difficulty":"hard","card":"c","pitch":"p","lab":{"runs":2,"wins":1}}`
	if _, err := Parse("ok", []byte(ok)); err != nil {
		t.Fatal(err)
	}
	for name, s := range map[string]string{
		"difficulty": `{"title":"T","difficulty":"normal","card":"c","pitch":"p","lab":{"runs":0,"wins":0}}`,
		"handicap":   `{"title":"T","difficulty":"hard","handicaps":["no_fun"],"card":"c","pitch":"p","lab":{"runs":0,"wins":0}}`,
		"digit":      `{"title":"T","difficulty":"hard","card":"c","pitch":"by day 91","lab":{"runs":0,"wins":0}}`,
		"unknown":    `{"title":"T","difficulty":"hard","card":"c","pitch":"p","odds":3,"lab":{"runs":0,"wins":0}}`,
		"wins":       `{"title":"T","difficulty":"hard","card":"c","pitch":"p","lab":{"runs":1,"wins":2}}`,
	} {
		if _, err := Parse(name, []byte(s)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
	f := Format{Name: "c", Difficulty: "challenge", Route: "smelter"}
	if p, err := f.RouteFile(""); err != nil || p != filepath.Join(route.DefaultDir, "challenge", "smelter.json") {
		t.Errorf("%s %v", p, err)
	}
	if _, err := f.RouteFile("vintner"); err == nil {
		t.Error("a format's own route cannot be swapped")
	}
	if p, _ := (Format{Difficulty: "hard"}).RouteFile(""); p != filepath.Join(route.DefaultDir, DefaultRoute+".json") {
		t.Errorf("default route: %s", p)
	}
}

func TestTallyAndOdds(t *testing.T) {
	f := Format{DeadlineDay: 91}
	runs := []Run{{1, "buyout", 84}, {2, "buyout", 91}, {3, "buyout", 98}, {4, "rent", 40}, {5, "buyout_mortgage", 175}}
	l := f.Tally(runs)
	if l.Runs != 5 || l.Wins != 2 || l.Endings["late"] != 2 || l.Endings["rent"] != 1 || l.MedianWinDay != 88 || l.MedianEndDay != 91 {
		t.Fatalf("%+v", l)
	}
	for _, tc := range []struct {
		wins, runs int
		odds       string
	}{
		{31, 60, "31 times in 60 (about 5 in 10)"},
		{18, 60, "18 times in 60 (about 3 in 10)"},
		{1, 2940, "once in 2,940"},
		{3, 2940, "3 times in 2,940 (about 1 in 980)"},
		{59, 60, "59 times in 60 (about 59 in 60)"},
		{60, 60, "60 times in 60"},
		{0, 60, "0 times in 60"},
	} {
		if got := (Lab{Wins: tc.wins, Runs: tc.runs}).Odds(); got != tc.odds {
			t.Errorf("%d/%d: %q, want %q", tc.wins, tc.runs, got, tc.odds)
		}
	}
	if !(Lab{Wins: 31, Runs: 60}).Jeopardy() || (Lab{Wins: 60, Runs: 60}).Jeopardy() || (Lab{Wins: 3, Runs: 60}).Jeopardy() {
		t.Error("jeopardy is a 20–80 % chance")
	}
	if lo, hi := Wilson(60, 60); lo < 93.9 || lo > 94.1 || hi < 99.99 {
		t.Errorf("Wilson(60, 60) = %.1f–%.1f", lo, hi)
	}
}

func TestCard(t *testing.T) {
	f := Format{Name: "s", Title: "Sprint", Difficulty: "hard", DeadlineDay: 91, Handicaps: []string{"no_mortgage"}, Card: "c", Pitch: "p",
		Lab: Lab{Runs: 60, Wins: 31, MedianWinDay: 84}}
	c := CardOf(f, Tally{})
	if c.LabPct != 52 || c.Attempts != "first attempt" || !slices.Equal(c.Handicaps, []string{"No mortgage", "Bought by day 91"}) {
		t.Fatalf("%+v", c)
	}
	if got := c.Caption(); got != "Format: Sprint, the shop bought by day 91; the lab wins it 31 times in 60 (about 5 in 10), median win day 84." {
		t.Errorf("caption %q", got)
	}
	if c.Verdict(true, 98) == "" || !strings.Contains(c.Verdict(true, 91), "won") || c.Verdict(false, 50) != "" {
		t.Error("verdicts")
	}
}

func TestRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "var", "record.jsonl")
	if tl, err := ReadRecord(path, "x", ""); err != nil || tl.Attempts != 0 {
		t.Fatal(tl, err)
	}
	for _, a := range []Attempt{
		{Format: "x", Run: "a", Ending: "rent", Day: 40},
		{Format: "x", Run: "b", Ending: "buyout", Day: 90, Won: true},
		{Format: "x", Run: "a", Ending: "rent", Day: 40},
		{Format: "y", Run: "c", Ending: "buyout", Day: 90, Won: true},
	} {
		if _, err := Append(path, a); err != nil {
			t.Fatal(err)
		}
	}
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"format":"x","run":"torn`)
	f.Close()
	tl, err := ReadRecord(path, "x", "")
	if err != nil || tl.Attempts != 2 || tl.Wins != 1 || tl.Text() != "1 win in 2 attempts (last: won, day 90)" {
		t.Fatalf("%+v %v %q", tl, err, tl.Text())
	}
	if tl, _ := ReadRecord(path, "x", "b"); tl.Attempts != 1 || tl.Text() != "0 wins in 1 attempt (last: rent, day 40)" {
		t.Errorf("except: %q", tl.Text())
	}
}
