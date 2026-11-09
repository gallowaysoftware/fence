package decisionlog

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDroppedRecordsAreMarked(t *testing.T) {
	dir := t.TempDir()
	l := newLogger(dir, 1, nil)
	for i := range 3 {
		l.Write("s", map[string]any{"type": "event", "session_id": "s", "n": i})
	}
	go l.run()
	l.Close()
	s, err := ReadFile(l.Path("s"))
	if err != nil {
		t.Fatal(err)
	}
	if g := Gaps(s, 0); g.Dropped != 2 {
		t.Fatalf("dropped %d, want 2; records %+v", g.Dropped, s.Records)
	}
}

func line(typ string, ts int, extra string) string {
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC).Add(time.Duration(ts) * time.Minute).Format(time.RFC3339Nano)
	s := fmt.Sprintf(`{"type":%q,"ts":%q,"session_id":"s-1"`, typ, at)
	if extra != "" {
		s += "," + extra
	}
	return s + "}"
}

func event(ts int, seq int64) string {
	return line("event", ts, fmt.Sprintf(`"event":{"session_id":"s-1","seq":%d,"kind":"k","game_day":1,"data":{}}`, seq))
}

func TestGaps(t *testing.T) {
	min := func(m int) time.Time {
		return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC).Add(time.Duration(m) * time.Minute)
	}
	tests := []struct {
		name  string
		lines []string
		want  GapReport
	}{
		{"clean from 0", []string{line("hello", 0, ""), event(1, 0), event(1, 1), event(2, 2)},
			GapReport{Session: "s-1"}},
		{"clean from 1", []string{line("hello", 0, ""), event(1, 1), event(2, 2)},
			GapReport{Session: "s-1"}},
		{"late start and a hole", []string{line("hello", 0, ""), event(1, 3), event(2, 4), event(3, 8)},
			GapReport{Session: "s-1", SeqGaps: []SeqGap{{0, 3}, {4, 8}}, Missed: 5}},
		{"reordered", []string{event(1, 1), event(1, 3), event(1, 2), event(2, 4)},
			GapReport{Session: "s-1", SeqGaps: []SeqGap{{1, 3}}, Missed: 1, OutOfOrder: 1}},
		{"silence", []string{line("hello", 0, ""), event(1, 1), event(30, 2), line("decision", 31, "")},
			GapReport{Session: "s-1", Silences: []Silence{{min(1), min(30)}}}},
		{"dropped and torn line", []string{line("hello", 0, ""), line(TypeDropped, 1, `"count":4`), event(1, 1), `{"type":"ev`},
			GapReport{Session: "s-1", Dropped: 4, BadLines: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Read(strings.NewReader(strings.Join(tt.lines, "\n")))
			if err != nil {
				t.Fatal(err)
			}
			got := Gaps(s, 5*time.Minute)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got  %+v\nwant %+v", got, tt.want)
			}
			if got.Clean() != reflect.DeepEqual(tt.want, GapReport{Session: "s-1"}) {
				t.Fatalf("Clean() = %v", got.Clean())
			}
		})
	}
}

func TestReadDirOrder(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, lines ...string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("b.jsonl", strings.Replace(line("hello", 0, ""), "s-1", "s-b", 1))
	write("a.jsonl", strings.Replace(line("hello", 5, ""), "s-1", "s-a", 1))
	write("empty.jsonl")
	write("notes.txt", "x")
	ss, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range ss {
		ids = append(ids, s.ID)
	}
	if got := strings.Join(ids, ","); got != "s-b,s-a" {
		t.Fatalf("order %s", got)
	}
}
