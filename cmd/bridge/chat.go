package main

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gallowaysoftware/fence/internal/twitch"
)

// chatAudience reads fencetwitch's chat vote for the morning plan it is for, once per tally.
// Only the tally file's fact is read, never chat itself.
type chatAudience struct {
	path string
	log  *slog.Logger
	now  func() time.Time

	mu   sync.Mutex
	sent int64
}

func (c *chatAudience) For(day int) string {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	t, ok := twitch.ReadTally(c.path, day, now())
	if !ok {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.Written == c.sent {
		return ""
	}
	c.sent = t.Written
	c.log.Info("chat tally", "day", day, "poll", t.Poll, "fact", t.Fact)
	return twitch.TallyBrief(t)
}
