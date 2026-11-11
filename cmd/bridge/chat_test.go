package main

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/twitch"
)

func TestChatAudience(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	path := filepath.Join(t.TempDir(), "tally.json")
	fact := "Twitch chat voted 62 % to hold the stock for better prices (hold 31, dump 19; 50 viewers, day 34's plan)."
	if err := twitch.WriteTally(path, twitch.Tally{ForDay: 34, Poll: "stock", Fact: fact, Written: now.UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	c := &chatAudience{path: path, log: slog.New(slog.DiscardHandler), now: func() time.Time { return now }}
	cases := []struct {
		name string
		day  int
		chat bool
	}{
		{"another day's plan", 33, false},
		{"the plan it is for", 34, true},
		{"only once", 34, false},
		{"nor the next day", 35, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := c.For(tc.day)
			if got := strings.Contains(b, fact); got != tc.chat {
				t.Errorf("chat = %v: %q", got, b)
			}
			if tc.chat && !strings.Contains(b, `the first line of RISK must start "Chat:"`) {
				t.Errorf("no ask for an answer: %q", b)
			}
		})
	}
	c.now = func() time.Time { return now.Add(time.Hour) }
	c.sent = 0
	if c.For(34) != "" {
		t.Error("an hour-old tally reached the plan")
	}
}
