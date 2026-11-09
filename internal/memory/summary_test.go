package memory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func TestSummary(t *testing.T) {
	played := protocol.DayStats{Decisions: 3, Sales: 1, CashDelta: 374, Walkouts: 1, RepDelta: map[string]float64{"lower_levels": -2.5}}
	none := protocol.DayStats{RepDelta: map[string]float64{}}
	tests := []struct {
		name    string
		setup   func(t *testing.T, path string) *Store
		session string
		want    protocol.SummaryResponse
	}{
		{"unknown session", func(t *testing.T, path string) *Store {
			return openStore(t, path, Options{})
		}, "nope", protocol.SummaryResponse{Today: none}},
		{"hello only", func(t *testing.T, path string) *Store {
			s := openStore(t, path, Options{})
			s.Hello("s1", protocol.HelloRequest{Protocol: 1})
			return s
		}, "s1", protocol.SummaryResponse{Today: none}},
		{"day played, no lessons yet", func(t *testing.T, path string) *Store {
			s := openStore(t, path, Options{})
			playDay(s, "s1", 5)
			return s
		}, "s1", protocol.SummaryResponse{Day: 5, Today: played}},
		{"latest lessons version and its day", func(t *testing.T, path string) *Store {
			s := openStore(t, path, Options{})
			playDay(s, "s1", 5)
			s.flush()
			s.db.Exec(`INSERT INTO lessons VALUES (1, 1, 3, '- old', 'test', ''), (1, 2, 4, '  - newer  ', 'test', '')`)
			return s
		}, "s1", protocol.SummaryResponse{Day: 5, Today: played, Lessons: "- newer", LessonsDay: 4}},
		{"next morning starts at zero", func(t *testing.T, path string) *Store {
			s := openStore(t, path, Options{})
			playDay(s, "s1", 5)
			s.Event("s1", ev("s1", "waking_up", 6, `{}`))
			return s
		}, "s1", protocol.SummaryResponse{Day: 6, Today: none}},
		{"other session's run", func(t *testing.T, path string) *Store {
			s := openStore(t, path, Options{})
			playDay(s, "s1", 5)
			s.Event("s2", ev("s2", "new_game_started", 1, `{}`))
			return s
		}, "s2", protocol.SummaryResponse{Day: 1, Today: none}},
		{"session the tracker forgot", func(t *testing.T, path string) *Store {
			s := openStore(t, path, Options{})
			playDay(s, "s1", 5)
			s.Close()
			return openStore(t, path, Options{})
		}, "s1", protocol.SummaryResponse{Day: 5, Today: played}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.setup(t, filepath.Join(t.TempDir(), "memory.db"))
			// Writes are queued; without this flush a summary right after an outcome can
			// lag by a few milliseconds, which the overlay tolerates.
			s.flush()
			got, err := s.Summary(context.Background(), tt.session)
			if err != nil {
				t.Fatal(err)
			}
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(tt.want)
			if string(a) != string(b) {
				t.Fatalf("got  %s\nwant %s", a, b)
			}
		})
	}
}

func TestSummaryCancelled(t *testing.T) {
	s := openStore(t, "", Options{})
	playDay(s, "s1", 5)
	s.flush()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := s.Summary(ctx, "s1")
	if err == nil {
		t.Fatal("no error from a cancelled summary")
	}
	if got.Day != 5 || got.Today.RepDelta == nil {
		t.Fatalf("partial summary %+v", got)
	}
}
