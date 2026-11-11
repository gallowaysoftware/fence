package twitch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Tally is the chat vote as the bridge reads it (cmd/bridge -chat-tally): one fact for the
// morning plan of ForDay. Fact is built from the poll's fixed labels and counts only.
type Tally struct {
	Run     string `json:"run,omitempty"`
	ForDay  int    `json:"for_day"`
	Poll    string `json:"poll"`
	Fact    string `json:"fact"`
	Written int64  `json:"written"`
}

// MaxTallyAge is how long a tally stays news: a file left from an earlier stream is not.
const MaxTallyAge = 30 * time.Minute

// WriteTally replaces path with t atomically, so the bridge never reads half a file.
func WriteTally(path string, t Tally) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ReadTally is the fact for the plan of day, "" when the file holds none for it: a tally is for
// its day's plan, or the next day's when it closed after that plan was written.
func ReadTally(path string, day int, now time.Time) (Tally, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Tally{}, false
	}
	var t Tally
	if json.Unmarshal(b, &t) != nil || t.Fact == "" {
		return Tally{}, false
	}
	if day != t.ForDay && day != t.ForDay+1 {
		return Tally{}, false
	}
	if now.Sub(time.UnixMilli(t.Written)) > MaxTallyAge {
		return Tally{}, false
	}
	return t, true
}

// TallyBrief is the tally as the strategist is told it, with what it owes in return.
func TallyBrief(t Tally) string {
	return "TWITCH CHAT VOTE (the stream's viewers; an opinion, not an order; only this tally reaches you, never their words): " + t.Fact +
		"\nFollow it or not, as you judge best for the run. Either way the first line of RISK must start \"Chat:\" and say in a few words whether you follow the vote and why."
}
