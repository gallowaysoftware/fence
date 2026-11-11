package show

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveAttachDoesNotRestartTheRun(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "s-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r"}),
		snapshot("morning", 1, 100, 500, map[string]any{"visible": false}),
		ev("money", 1, map[string]any{"kind": "day_ended", "day": 1, "net_worth": 500, "credits": 100}),
		snapshot("morning", 9, 300, 900, map[string]any{"visible": false}),
	} {
		b, _ := json.Marshal(l)
		f.Write(append(b, '\n'))
	}
	f.Close()

	var got []Event
	l := &Live{LogDir: dir, Player: Player{ID: "p"}, Publish: func(e Event) { got = append(got, e) }}
	tl, err := l.open(filepath.Join(dir, "s-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.f.Close()
	starts := 0
	for _, e := range got {
		if e.Kind == KindRunStart {
			starts++
			if e.Level >= LevelNotable {
				t.Errorf("attach mid-run published the run start at level %d: the narrator would greet day 1 again", e.Level)
			}
		}
	}
	if starts != 1 {
		t.Errorf("run starts = %d, want 1 to redraw the panel", starts)
	}
}

func TestLiveAttachToAFreshRunGreetsIt(t *testing.T) {
	dir := t.TempDir()
	b, _ := json.Marshal(ev("run_start", 1, map[string]any{"run_id": "r"}))
	os.WriteFile(filepath.Join(dir, "s-1.jsonl"), append(b, '\n'), 0o644)
	var got []Event
	l := &Live{LogDir: dir, Player: Player{ID: "p"}, Publish: func(e Event) { got = append(got, e) }}
	tl, err := l.open(filepath.Join(dir, "s-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer tl.f.Close()
	if len(got) != 1 || got[0].Kind != KindRunStart || got[0].Level != LevelMoment {
		t.Errorf("a session seen at its first line: %+v, want the run start as a moment", got)
	}
}
