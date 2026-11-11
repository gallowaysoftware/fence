// Package expert mines strong players' videos of Probably Stolen for decisions: what a
// player did at a given day and situation, why, and what came of it. Nothing extracted
// here is ground truth; every recurring habit becomes a hypothesis for the gym
// (docs/research/EXPERT-HYPOTHESES.md). Transcripts are fetched with the knowledge
// pipeline's fetcher and cached under var/expert, which is never committed.
package expert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/knowledge"
)

// Video is one candidate source. Rank orders creators by evidence of skill (1 = strongest);
// Pick marks the videos whose transcripts are mined.
type Video struct {
	ID       string `json:"id"` // YouTube id
	URL      string `json:"url"`
	Creator  string `json:"creator"`
	Title    string `json:"title"`
	Date     string `json:"date,omitempty"`
	Duration int    `json:"duration_s,omitempty"`
	Views    int    `json:"views,omitempty"`
	Series   string `json:"series,omitempty"`
	Evidence string `json:"evidence,omitempty"` // why we think this play is strong
	Rank     int    `json:"rank,omitempty"`
	Pick     bool   `json:"pick,omitempty"`
}

type SourceFile struct {
	Generated string  `json:"generated,omitempty"`
	Notes     string  `json:"notes,omitempty"`
	Videos    []Video `json:"videos"`
}

func LoadSources(path string) (SourceFile, error) {
	var f SourceFile
	b, err := os.ReadFile(path)
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Categories a decision is filed under; they mirror the strategist's day-level levers.
var Categories = []string{
	"upgrade", "stock_buy", "wholesale", "sell_price", "haggle", "faction", "heat_stolen",
	"contraband_inspection", "service", "production", "rent_cash", "night_outing", "story_npc", "other",
}

// Decision is one (situation, decision, reason, outcome) tuple from narration.
type Decision struct {
	VideoID   string `json:"video_id"`
	Creator   string `json:"creator"`
	At        string `json:"at,omitempty"`  // transcript timestamp of the passage, m:ss or h:mm:ss
	Day       int    `json:"day,omitempty"` // in-game day when stated, else 0
	Category  string `json:"category"`
	Situation string `json:"situation"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	Outcome   string `json:"outcome,omitempty"`
	Numbers   string `json:"numbers,omitempty"` // prices, cash, rep values as stated
}

// Bought is one upgrade, machine or service purchase with its day if stated.
type Bought struct {
	Item string `json:"item"`
	Day  int    `json:"day,omitempty"`
	Cost int    `json:"cost,omitempty"`
}

// Facts are the run-level facts one transcript chunk states.
type Facts struct {
	Start         string   `json:"start,omitempty"`
	Difficulty    string   `json:"difficulty,omitempty"`
	Perks         []string `json:"perks,omitempty"`
	Engines       []string `json:"engines,omitempty"`
	Bought        []Bought `json:"bought,omitempty"`
	TurningPoints []string `json:"turning_points,omitempty"`
	FirstDay      int      `json:"first_day,omitempty"`
	LastDay       int      `json:"last_day,omitempty"`
	Ending        string   `json:"ending,omitempty"` // buyout/mortgage day or loss, if shown
	CashNotes     []string `json:"cash_notes,omitempty"`
}

type Extraction struct {
	Decisions []Decision `json:"decisions"`
	Facts     Facts      `json:"facts"`
}

func (d *Decision) Validate() error {
	d.Category = strings.ToLower(strings.TrimSpace(d.Category))
	d.Situation = clean(d.Situation, 300)
	d.Decision = clean(d.Decision, 300)
	d.Reason = clean(d.Reason, 300)
	d.Outcome = clean(d.Outcome, 300)
	d.Numbers = clean(d.Numbers, 160)
	if !contains(Categories, d.Category) {
		d.Category = "other"
	}
	if len(d.Decision) < 8 {
		return fmt.Errorf("no decision")
	}
	if d.Day < 0 || d.Day > 400 {
		d.Day = 0
	}
	return nil
}

func clean(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = string(r[:n-1]) + "…"
	}
	return s
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Store lays out var/expert. Transcripts live in the knowledge Store shape (text/, raw/)
// so knowledge.Fetcher can fill it.
type Store struct{ Dir string }

func (s Store) Knowledge() knowledge.Store { return knowledge.Store{Dir: s.Dir} }
func (s Store) SourcesPath() string        { return filepath.Join(s.Dir, "sources.json") }
func (s Store) ChunkPath(vid string, i int) string {
	return filepath.Join(s.Dir, "extract", fmt.Sprintf("%s.%02d.json", vid, i))
}
func (s Store) DecisionsPath() string          { return filepath.Join(s.Dir, "decisions.jsonl") }
func (s Store) RunsPath() string               { return filepath.Join(s.Dir, "runs.json") }
func (s Store) CreatorPath(slug string) string { return filepath.Join(s.Dir, "creators", slug+".json") }
func (s Store) HabitsPath() string             { return filepath.Join(s.Dir, "habits.json") }
func (s Store) VideoDir() string               { return filepath.Join(s.Dir, "video") }
func (s Store) FramePath(vid string, sec int) string {
	return filepath.Join(s.Dir, "frames", fmt.Sprintf("%s-%05d.jpg", vid, sec))
}
func (s Store) ReadPath(vid string, sec int) string {
	return filepath.Join(s.Dir, "frames", fmt.Sprintf("%s-%05d.json", vid, sec))
}

// KnowledgeSource is the knowledge-pipeline view of v, so its fetcher and cache apply.
func KnowledgeSource(v Video) knowledge.Source {
	return knowledge.Source{
		ID: "expert-" + v.ID, Type: knowledge.TypeVideo, Title: v.Title, URL: "https://www.youtube.com/watch?v=" + v.ID,
		Channel: v.Creator, Date: v.Date, Duration: v.Duration, Quality: knowledge.QualityHigh,
	}
}

func Slug(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// RunFacts merges the per-chunk facts of one video: first non-empty scalar wins, lists are
// unioned, purchases are deduplicated by item and ordered by day (unknown days keep
// transcript order after the dated ones).
func RunFacts(parts []Facts) Facts {
	var f Facts
	seenB := map[string]bool{}
	for _, p := range parts {
		f.Start = firstNonEmpty(f.Start, p.Start)
		f.Difficulty = firstNonEmpty(f.Difficulty, p.Difficulty)
		f.Ending = firstNonEmpty(p.Ending, f.Ending)
		f.Perks = union(f.Perks, p.Perks)
		f.Engines = union(f.Engines, p.Engines)
		f.TurningPoints = append(f.TurningPoints, p.TurningPoints...)
		f.CashNotes = append(f.CashNotes, p.CashNotes...)
		if p.FirstDay > 0 && (f.FirstDay == 0 || p.FirstDay < f.FirstDay) {
			f.FirstDay = p.FirstDay
		}
		if p.LastDay > f.LastDay {
			f.LastDay = p.LastDay
		}
		for _, b := range p.Bought {
			k := strings.ToLower(strings.TrimSpace(b.Item))
			if k == "" || seenB[k] {
				continue
			}
			seenB[k] = true
			f.Bought = append(f.Bought, b)
		}
	}
	sort.SliceStable(f.Bought, func(i, j int) bool {
		di, dj := f.Bought[i].Day, f.Bought[j].Day
		if di == 0 || dj == 0 {
			return di != 0 && dj == 0
		}
		return di < dj
	})
	return f
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return strings.TrimSpace(b)
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	for _, x := range a {
		seen[strings.ToLower(x)] = true
	}
	for _, x := range b {
		x = strings.TrimSpace(x)
		if x != "" && !seen[strings.ToLower(x)] {
			seen[strings.ToLower(x)] = true
			a = append(a, x)
		}
	}
	return a
}

var reStamp = regexp.MustCompile(`^\[(\d+(?::\d{2}){1,2})\]`)

// Seconds parses a transcript stamp (m:ss or h:mm:ss).
func Seconds(stamp string) (int, bool) {
	parts := strings.Split(strings.Trim(stamp, "[]"), ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	t := 0
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, false
		}
		t = t*60 + n
	}
	return t, true
}

// FirstStamp returns the first paragraph stamp of a chunk of cleaned transcript.
func FirstStamp(text string) string {
	for _, l := range strings.Split(text, "\n") {
		if m := reStamp.FindStringSubmatch(strings.TrimSpace(l)); m != nil {
			return m[1]
		}
	}
	return ""
}
