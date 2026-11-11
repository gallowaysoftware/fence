package format

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultRecord is the local record of attempts at the formats (var/ is not committed).
const DefaultRecord = "var/formats/record.jsonl"

// Attempt is one played run of a format: a stream run or a league match, not a lab run.
type Attempt struct {
	Format string `json:"format"`
	// Run is the run's id (a show run "<player>:<run_id>", a league match id); it is recorded once.
	Run     string `json:"run"`
	Source  string `json:"source"`
	Model   string `json:"model,omitempty"`
	Persona string `json:"persona,omitempty"`
	Route   string `json:"route,omitempty"`
	Seed    int64  `json:"seed,omitempty"`
	Ending  string `json:"ending"`
	Day     int    `json:"day"`
	Won     bool   `json:"won"`
	At      string `json:"at"`
}

// Tally is the attempts so far at one format.
type Tally struct {
	Attempts, Wins int
	Last           *Attempt
}

// Text is the tally for the card: "1 win in 3 attempts (last: rent, day 141)".
func (t Tally) Text() string {
	if t.Attempts == 0 {
		return "first attempt"
	}
	s := fmt.Sprintf("%d %s in %d %s", t.Wins, plural(t.Wins, "win", "wins"), t.Attempts, plural(t.Attempts, "attempt", "attempts"))
	if l := t.Last; l != nil {
		what := "won"
		switch {
		case !l.Won && strings.HasPrefix(l.Ending, "buyout"):
			what = "bought too late"
		case !l.Won:
			what = strings.ReplaceAll(l.Ending, "_", " ")
		}
		s += fmt.Sprintf(" (last: %s, day %d)", what, l.Day)
	}
	return s
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ReadRecord reads the attempts at format from the record, leaving out the run except (a replay
// must not show its own ending); a missing file is no attempts. Lines that do not parse are
// skipped: the record is append-only and a torn last line must not hide the rest.
func ReadRecord(path, format, except string) (Tally, error) {
	var t Tally
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	defer f.Close()
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var a Attempt
		if json.Unmarshal(sc.Bytes(), &a) != nil || a.Format != format || seen[a.Run] || (except != "" && a.Run == except) {
			continue
		}
		seen[a.Run] = true
		t.Attempts++
		if a.Won {
			t.Wins++
		}
		t.Last = &a
	}
	return t, sc.Err()
}

// Append adds an attempt to the record unless its run is already there; it reports whether it did.
func Append(path string, a Attempt) (bool, error) {
	if a.Format == "" || a.Run == "" {
		return false, errors.New("an attempt needs a format and a run")
	}
	if f, err := os.Open(path); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			var old Attempt
			if json.Unmarshal(sc.Bytes(), &old) == nil && old.Format == a.Format && old.Run == a.Run {
				f.Close()
				return false, nil
			}
		}
		f.Close()
	}
	if a.At == "" {
		a.At = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err == nil, err
}
