package decisionlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Record is one decision-log line: the common header plus the raw line.
type Record struct {
	Type      string          `json:"type"`
	TS        time.Time       `json:"ts"`
	SessionID string          `json:"session_id"`
	Raw       json.RawMessage `json:"-"`
}

// Session is one session's file as read back.
type Session struct {
	ID      string
	Path    string
	Records []Record
	// BadLines counts lines that are not a JSON record, such as one cut short by a crash.
	BadLines int
}

// ReadDir reads every *.jsonl file in dir, sorted by the session's first record time and
// then by session id.
func ReadDir(dir string) ([]Session, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(paths))
	for _, p := range paths {
		s, err := ReadFile(p)
		if err != nil {
			return nil, err
		}
		if len(s.Records) > 0 || s.BadLines > 0 {
			out = append(out, s)
		}
	}
	slices.SortStableFunc(out, func(a, b Session) int {
		if c := a.start().Compare(b.start()); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (s Session) start() time.Time {
	if len(s.Records) == 0 {
		return time.Time{}
	}
	return s.Records[0].TS
}

// ReadFile reads one session file. The session id comes from the records, else the file
// name.
func ReadFile(path string) (Session, error) {
	f, err := os.Open(path)
	if err != nil {
		return Session{}, err
	}
	defer f.Close()
	s, err := Read(f)
	s.Path = path
	if s.ID == "" {
		s.ID = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return s, err
}

func Read(r io.Reader) (Session, error) {
	var s Session
	br := bufio.NewReaderSize(r, 1<<16)
	for {
		line, err := br.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			var rec Record
			if json.Unmarshal(line, &rec) != nil || rec.Type == "" {
				s.BadLines++
			} else {
				rec.Raw = append(json.RawMessage(nil), line...)
				s.Records = append(s.Records, rec)
				if s.ID == "" {
					s.ID = rec.SessionID
				}
			}
		}
		if err == io.EOF {
			return s, nil
		}
		if err != nil {
			return s, err
		}
	}
}

// SeqGap is a run of event seqs the bridge never received: After+1 … Before-1.
type SeqGap struct {
	After  int64 `json:"after"`
	Before int64 `json:"before"`
}

func (g SeqGap) Missed() int64 { return g.Before - g.After - 1 }

// Silence is a stretch of a running session in which the bridge received nothing.
type Silence struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// GapReport is what a session's log says about data the bridge did not get.
type GapReport struct {
	Session    string    `json:"session_id"`
	SeqGaps    []SeqGap  `json:"seq_gaps,omitempty"`
	Missed     int64     `json:"missed"`
	OutOfOrder int       `json:"out_of_order"`
	Silences   []Silence `json:"silences,omitempty"`
	// Dropped is records the bridge received but could not log.
	Dropped  int64 `json:"dropped"`
	BadLines int   `json:"bad_lines"`
}

// Clean reports whether nothing is known to be missing.
func (g GapReport) Clean() bool {
	return g.Missed == 0 && g.OutOfOrder == 0 && len(g.Silences) == 0 && g.Dropped == 0 && g.BadLines == 0
}

// Gaps reports seq gaps among the session's events, records dropped by the logger, and
// silences: gaps longer than silence between consecutive records after the session's
// hello, while the game was running. Seqs may start at 0 or 1.
func Gaps(s Session, silence time.Duration) GapReport {
	g := GapReport{Session: s.ID, BadLines: s.BadLines}
	var last int64
	seen := false
	var prev time.Time
	for _, r := range s.Records {
		if !prev.IsZero() && silence > 0 && r.TS.Sub(prev) > silence {
			g.Silences = append(g.Silences, Silence{From: prev, To: r.TS})
		}
		if !r.TS.IsZero() {
			prev = r.TS
		}
		switch r.Type {
		case "event":
			var e struct {
				Event struct {
					Seq int64 `json:"seq"`
				} `json:"event"`
			}
			if json.Unmarshal(r.Raw, &e) != nil {
				continue
			}
			seq := e.Event.Seq
			switch {
			case !seen && seq > 1:
				g.SeqGaps = append(g.SeqGaps, SeqGap{After: 0, Before: seq})
			case seen && seq <= last:
				g.OutOfOrder++
				continue
			case seen && seq > last+1:
				g.SeqGaps = append(g.SeqGaps, SeqGap{After: last, Before: seq})
			}
			seen, last = true, seq
		case TypeDropped:
			var d droppedRecord
			if json.Unmarshal(r.Raw, &d) == nil {
				g.Dropped += d.Count
			}
		}
	}
	for _, sg := range g.SeqGaps {
		g.Missed += sg.Missed()
	}
	return g
}
