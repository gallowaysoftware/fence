package vod

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Recording is one OBS recording: when it started on the wall clock, how long it runs (0:
// unknown) and its file ("" when not given).
type Recording struct {
	Start  time.Time
	Length time.Duration
	File   string
}

// Part is the slice of a timeline one recording holds; offsets are from the recording's start.
type Part struct {
	N, Of  int
	Rec    Recording
	Points []Point
	Says   []Point
}

// tail is how long a recording whose length is unknown is taken to run past its last event.
const tail = 30 * time.Second

// Split maps each recording to the events it holds. A recording without a length ends where the
// next one starts, the last one shortly after the feed's last event.
func Split(t *Timeline, recs []Recording) ([]Part, error) {
	recs = slices.Clone(recs)
	slices.SortStableFunc(recs, func(a, b Recording) int { return a.Start.Compare(b.Start) })
	var last time.Time
	if n := len(t.Points); n > 0 {
		last = t.Points[n-1].At
	}
	if n := len(t.Says); n > 0 && t.Says[n-1].At.After(last) {
		last = t.Says[n-1].At
	}
	var parts []Part
	for i, r := range recs {
		if r.Start.IsZero() {
			return nil, fmt.Errorf("recording %d has no start time", i+1)
		}
		if r.Length <= 0 {
			switch {
			case i+1 < len(recs):
				r.Length = recs[i+1].Start.Sub(r.Start)
			case last.After(r.Start):
				r.Length = last.Sub(r.Start) + tail
			}
		}
		if r.Length <= 0 {
			return nil, fmt.Errorf("recording %d starts at %s, after the feed's last event", i+1, r.Start.Format(time.RFC3339))
		}
		end := r.Start.Add(r.Length)
		in := func(p Point) bool { return !p.At.Before(r.Start) && p.At.Before(end) }
		p := Part{N: i + 1, Of: len(recs), Rec: r}
		for _, pt := range t.Points {
			if in(pt) {
				p.Points = append(p.Points, pt)
			}
		}
		for _, pt := range t.Says {
			if in(pt) {
				p.Says = append(p.Says, pt)
			}
		}
		parts = append(parts, p)
	}
	return parts, nil
}

// Off is a point's offset into the part's recording.
func (p *Part) Off(t time.Time) time.Duration { return t.Sub(p.Rec.Start) }

// Days is the first and last game day the part shows, 0 0 when it shows none.
func (p *Part) Days() (from, to int) {
	for _, pt := range p.Points {
		if pt.Ev.Day <= 0 {
			continue
		}
		if from == 0 || pt.Ev.Day < from {
			from = pt.Ev.Day
		}
		to = max(to, pt.Ev.Day)
	}
	return from, to
}

// obsName is OBS's default recording name, "%CCYY-%MM-%DD %hh-%mm-%ss", in local time.
const obsName = "2006-01-02 15-04-05"

var startLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", obsName}

// ParseStart reads a recording's start: RFC 3339, or a local "2006-01-02 15:04:05" (a T or a
// space; seconds optional), or OBS's file name format. An optional "/DURATION" gives its length
// ("2026-11-11T06:38:00Z/3h10m").
func ParseStart(s string, loc *time.Location) (Recording, error) {
	s = strings.TrimSpace(s)
	var r Recording
	if at, d, ok := strings.Cut(s, "/"); ok {
		l, err := time.ParseDuration(d)
		if err != nil || l <= 0 {
			return r, fmt.Errorf("recording start %q: bad length %q", s, d)
		}
		s, r.Length = at, l
	}
	for _, l := range startLayouts {
		if t, err := time.ParseInLocation(l, s, loc); err == nil {
			r.Start = t
			return r, nil
		}
	}
	return r, fmt.Errorf("recording start %q: want RFC 3339 or 2006-01-02 15:04:05 (local)", s)
}

// StartFromName reads the start time out of an OBS file name ("2026-11-11 02-38-00.mkv").
func StartFromName(path string, loc *time.Location) (time.Time, bool) {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if len(base) < len(obsName) {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(obsName, base[len(base)-len(obsName):], loc)
	return t, err == nil
}
