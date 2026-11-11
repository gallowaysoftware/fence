package show

import (
	"github.com/gallowaysoftware/fence/internal/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormatOnTheCard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sprint.json"), []byte(`{"title":"Thirteen-week sprint","difficulty":"hard","deadline_day":91,
		"card":"Buy the shop by day 91.","pitch":"A race against the calendar.","lab":{"runs":60,"wins":31,"median_win_day":84}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(dir, "record.jsonl")
	if err := os.WriteFile(record, []byte(`{"format":"sprint","run":"a","ending":"rent","day":40}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := Player{ID: "qwen27", Persona: "shark"}
	if err := p.SetFormat(dir, "sprint", record, ""); err != nil {
		t.Fatal(err)
	}
	if p.Format == nil || p.Difficulty != "hard" || p.Format.LabPct != 52 || p.Format.Attempts != "0 wins in 1 attempt (last: rent, day 40)" {
		t.Fatalf("%+v %+v", p, p.Format)
	}
	if err := (&Player{}).SetFormat(dir, "missing", record, ""); err == nil {
		t.Error("a missing format is an error")
	}

	// The run's start says the odds in digits, so the narrator may speak them there.
	start := Event{Kind: KindRunStart, Day: 1, Player: &p, Text: runStartText(p)}
	if !strings.Contains(start.Text, "Format: Thirteen-week sprint, the shop bought by day 91; the lab wins it 31 times in 60 (about 5 in 10)") {
		t.Errorf("run start %q", start.Text)
	}
	if f := CheckLine("The lab wins this 31 times in 60, about 5 in 10. Day 91 or bust.", FactsOf(start)); len(f) > 0 {
		t.Errorf("the start's odds fail the check: %v", f)
	}

	// Elsewhere the narrator has the stakes in words only.
	n := &Narrator{PersonaD: "../../docs/personas"}
	n.defaults()
	n.player = &p
	with := n.system(Mood{})
	if !strings.Contains(with, "Thirteen-week sprint") || !strings.Contains(with, "coin flip") || !strings.Contains(with, "deadline") {
		t.Errorf("system %q", with)
	}
	if stakes := formatStakes(&p); strings.ContainsAny(stakes, "0123456789") {
		t.Errorf("the stakes hold a digit: %q", stakes)
	}

	end := Event{Kind: KindRunEnd, Run: "qwen27:r1", Day: 98, End: &End{Ending: "buyout", Day: 98, Won: true}, Text: "Day 98: bought the shop. The run is won."}
	a, ok := FormatEnd(&end, &p, "stream")
	if !ok || a.Won || a.Format != "sprint" || a.Run != "qwen27:r1" || !strings.Contains(end.Text, "Thirteen-week sprint lost: the deadline was day 91.") {
		t.Errorf("late buyout: %+v %q", a, end.Text)
	}
	if _, ok := FormatEnd(&end, &Player{}, "stream"); ok {
		t.Error("no format, no attempt")
	}
}

// A format's title, pitch and stakes ride in every narrator prompt: none may hold a number, in digits
// or words, that the fact check would reject when the narrator repeats it.
func TestShippedFormatsHaveNoNumbersForTheNarrator(t *testing.T) {
	dir := "../../" + format.DefaultDir
	fs, err := format.List(dir)
	if err != nil || len(fs) == 0 {
		t.Fatal(fs, err)
	}
	for _, f := range fs {
		p := Player{}
		if err := p.SetFormat(dir, f.Name, filepath.Join(t.TempDir(), "none.jsonl"), ""); err != nil {
			t.Fatal(err)
		}
		if faults := CheckLine(f.Title+". "+formatStakes(&p), Facts{}); len(faults) > 0 {
			t.Errorf("%s: %v", f.Name, faults)
		}
	}
}
