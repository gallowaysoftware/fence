package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
)

// writeFeed writes a synthetic 21-day feed (our own events, no game text) published from start,
// a game day every two minutes.
func writeFeed(t *testing.T, start time.Time) string {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	pub := func(d int, off time.Duration, ev show.Event) {
		ev.V, ev.Run, ev.Day = 1, "p:feed", d
		ev.At = start.Add(time.Duration(d-1)*2*time.Minute + off).UnixMilli()
		if err := enc.Encode(ev); err != nil {
			t.Fatal(err)
		}
	}
	pub(1, 0, show.Event{Kind: show.KindRunStart, Level: show.LevelMoment, Text: "New run.",
		Player: &show.Player{ID: "p", Model: "test-model", PersonaName: "Tester", Difficulty: "hard"}})
	for d := 1; d <= 21; d++ {
		pub(d, 100*time.Second, show.Event{Kind: show.KindDay, Level: show.LevelNotable, Text: "Day closed.",
			Clock: &show.Clock{Cash: int64(d * 100), NetWorth: int64(d * 300)}})
	}
	pub(10, 30*time.Second, show.Event{Kind: show.KindPurchase, Level: show.LevelMoment, Beat: show.BeatAppraisalJoin, Text: "Joined the appraisal board for 170."})
	pub(21, 110*time.Second, show.Event{Kind: show.KindRunEnd, Level: show.LevelMoment, Text: "The run ends.",
		End: &show.End{Ending: "evicted", Day: 21, NetWorth: 6300}})
	path := filepath.Join(t.TempDir(), "feed.jsonl")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRun(t *testing.T) {
	start := time.Date(2026, 11, 11, 6, 0, 0, 0, time.UTC)
	feed := writeFeed(t, start)
	for _, c := range []struct {
		name  string
		args  []string
		parts []string
	}{
		{"one recording", []string{"-recording-start", "2026-11-11T05:59:00Z"}, []string{""}},
		{"two sessions", []string{"-recording-start", "2026-11-11T05:59:00Z", "-recording-start", "2026-11-11T06:20:00Z"}, []string{"part-1", "part-2"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := t.TempDir()
			var stdout bytes.Buffer
			args := append([]string{"-feed", feed, "-out", out, "-probe=false"}, c.args...)
			if err := run(context.Background(), args, &stdout, io.Discard); err != nil {
				t.Fatal(err)
			}
			for i, p := range c.parts {
				desc, err := os.ReadFile(filepath.Join(out, p, "description.txt"))
				if err != nil {
					t.Fatal(err)
				}
				ch, _ := os.ReadFile(filepath.Join(out, p, "chapters.txt"))
				if !strings.HasPrefix(string(ch), "0:00 ") {
					t.Errorf("%s chapters:\n%s", p, ch)
				}
				last := i == len(c.parts)-1
				if got := strings.Contains(string(desc), "Result: the run ends on day 21 (evicted)"); got != last {
					t.Errorf("%s: result in description %v, want %v\n%s", p, got, last, desc)
				}
			}
			if n := strings.Count(stdout.String(), "chapters"); n != len(c.parts) {
				t.Errorf("stdout:\n%s", stdout.String())
			}
		})
	}
}

func TestRunRejects(t *testing.T) {
	feed := writeFeed(t, time.Date(2026, 11, 11, 6, 0, 0, 0, time.UTC))
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"-out", "x", "-recording-start", "2026-11-11T06:00:00Z"}, "-feed or -replay"},
		{[]string{"-feed", feed, "-recording-start", "2026-11-11T06:00:00Z"}, "-out"},
		{[]string{"-feed", feed, "-out", "x"}, "-recording-start"},
		{[]string{"-feed", feed, "-out", "x", "-recording", "a.mkv", "-recording", "b.mkv", "-recording-start", "2026-11-11T06:00:00Z"}, "one -recording-start per -recording"},
		{[]string{"-feed", feed, "-out", "x", "-recording", "final.mkv"}, "not an OBS default file name"},
		{[]string{"-feed", feed, "-out", "x", "-recording-start", "2026-11-12T06:00:00Z"}, "after the feed's last event"},
	} {
		err := run(context.Background(), c.args, io.Discard, io.Discard)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("run(%v) = %v, want an error with %q", c.args, err, c.want)
		}
	}
}
