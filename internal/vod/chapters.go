package vod

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
)

// Chapter is one YouTube chapter.
type Chapter struct {
	At      time.Duration `json:"-"`
	Seconds int           `json:"seconds"`
	Stamp   string        `json:"stamp"`
	Day     int           `json:"day"`
	Title   string        `json:"title"`
	Kind    string        `json:"kind,omitempty"`
	Beat    string        `json:"beat,omitempty"`
}

// YouTube's rules: the first chapter at 0:00, at least three, each at least 10 s long.
const (
	minChapter  = 10 * time.Second
	minChapters = 3
	maxChapters = 30
	// chapterEvery is the target spacing: a 3-hour session gets about 22 chapters.
	chapterEvery = 8 * time.Minute
)

// routine beats are the everyday moments (smallbeats.go): clip material, never a chapter.
var routine = map[string]bool{show.BeatCounter: true, show.BeatNight: true, show.BeatMorning: true, show.BeatAppraisalStreak: true}

// chapterScore ranks a moment as a chapter with the narrator's own ranking (show.Priority), so a
// kind or beat another package adds to Priority slots in here too; 0 is not a chapter. A named beat
// beats a plain event of the same priority.
func chapterScore(ev show.Event) int {
	pivot := show.PlanPivot(ev)
	if ev.Level < show.LevelMoment || (routine[ev.Beat] && !pivot) {
		return 0
	}
	switch ev.Kind {
	case show.KindSay, show.KindDay, show.KindHeat:
		// Days are the week anchors; heat moments fire on every tens mark of evidence.
		return 0
	case show.KindPlan:
		if !pivot {
			return 0
		}
		// A pivot is the morning plan too; the beat's bonus is for the big beats.
		return show.Priority(ev) * 10
	}
	s := show.Priority(ev) * 10
	if ev.Beat != "" {
		s += 5
	}
	return s
}

// diversity is what each earlier pick of the same kind and beat costs a candidate, so eight aug
// confrontations do not crowd out the rest of the run.
const diversity = 8

type candidate struct {
	pt    Point
	score int
	mag   int64
}

func magnitude(ev show.Event) int64 {
	m := ev.Amount
	if ev.Trade != nil {
		m = ev.Trade.Cash
	}
	if m < 0 {
		m = -m
	}
	return m
}

// Chapters picks the part's chapters: an opening at 0:00, the start of each week (or of each
// pair of weeks when weeks would fill more than half the list), and the best moments by
// show.Priority in what room is left, no two closer than the part's spacing.
func Chapters(p *Part) []Chapter {
	length := p.Rec.Length
	n := int(length / chapterEvery)
	n = min(max(n, 10), maxChapters, int(length/minChapter))
	if n < minChapters {
		return nil
	}
	gap := max(minChapter, length/time.Duration(3*n))

	type pick struct {
		at    time.Duration
		day   int
		title string
		ev    *show.Event
	}
	var picks []pick
	clash := func(at time.Duration) bool {
		for _, q := range picks {
			if d := q.at - at; d < gap && d > -gap {
				return true
			}
		}
		return false
	}

	weeks := weekStarts(p)
	group := 1
	for (len(weeks)+group-1)/group > n/2 {
		group++
	}
	from, to := p.Days()
	for i := 0; i < len(weeks); i += group {
		j := min(i+group, len(weeks)) - 1
		last := min(to, weeks[j].week*7)
		if j+1 < len(weeks) {
			last = weeks[j+1].day - 1
		}
		picks = append(picks, pick{at: p.Off(weeks[i].at), day: weeks[i].day, title: weekTitle(weeks[i].week, weeks[j].week, weeks[i].day, last)})
	}
	if len(picks) == 0 || picks[0].at > gap {
		title := fmt.Sprintf("Day %d: picking up the run", from)
		if from <= 1 {
			title = "The stream starts"
		}
		picks = slices.Insert(picks, 0, pick{at: 0, day: from, title: title})
	}
	picks[0].at = 0

	var cands []candidate
	for _, pt := range p.Points {
		if s := chapterScore(pt.Ev); s > 0 {
			cands = append(cands, candidate{pt, s, magnitude(pt.Ev)})
		}
	}
	used := map[string]int{}
	titled := map[string]bool{}
	for len(picks) < n && len(cands) > 0 {
		best := -1
		bestScore := 0
		for i, c := range cands {
			s := c.score - diversity*used[string(c.pt.Ev.Kind)+"/"+c.pt.Ev.Beat]
			if best < 0 || s > bestScore || (s == bestScore && c.mag > cands[best].mag) {
				best, bestScore = i, s
			}
		}
		c := cands[best]
		cands = slices.Delete(cands, best, best+1)
		ev := c.pt.Ev
		body := titleBody(ev)
		if titled[body] {
			// The same pivot taken up again ("run wine + furnace" every few days) is one chapter.
			continue
		}
		at := p.Off(c.pt.At)
		if clash(at) {
			// A loud moment right on a bare week anchor names it: "Week 5: days 29–35 · Halfway…".
			if c.score < 40 {
				continue
			}
			for i := range picks {
				if d := picks[i].at - at; d < gap && d > -gap && picks[i].ev == nil {
					if c.score >= 50 {
						picks[i].title = chapterTitle(ev)
					} else {
						picks[i].title += " · " + body
					}
					picks[i].ev = &ev
					titled[body] = true
					used[string(ev.Kind)+"/"+ev.Beat]++
					break
				}
			}
			continue
		}
		titled[body] = true
		used[string(ev.Kind)+"/"+ev.Beat]++
		picks = append(picks, pick{at: at, day: ev.Day, title: chapterTitle(ev), ev: &ev})
	}
	slices.SortStableFunc(picks, func(a, b pick) int { return int(a.at - b.at) })

	out := make([]Chapter, 0, len(picks))
	for _, q := range picks {
		if len(out) > 0 && q.at-out[len(out)-1].At < minChapter {
			continue
		}
		c := Chapter{At: q.at, Seconds: int(q.at / time.Second), Stamp: Stamp(q.at), Day: q.day, Title: q.title}
		if q.ev != nil {
			c.Kind, c.Beat = string(q.ev.Kind), q.ev.Beat
		}
		out = append(out, c)
	}
	if len(out) < minChapters {
		return nil
	}
	return out
}

type weekStart struct {
	week, day int
	at        time.Time
}

// weekStarts is the first event of each game week (days 1–7 are week 1) the part shows.
func weekStarts(p *Part) []weekStart {
	var out []weekStart
	for _, pt := range p.Points {
		d := pt.Ev.Day
		if d <= 0 {
			continue
		}
		w := (d-1)/7 + 1
		if len(out) == 0 || w > out[len(out)-1].week {
			out = append(out, weekStart{w, d, pt.At})
		}
	}
	return out
}

func weekTitle(from, to, day, last int) string {
	days := fmt.Sprintf("days %d–%d", day, last)
	if last <= day {
		days = fmt.Sprintf("day %d", day)
	}
	if to > from {
		return fmt.Sprintf("Weeks %d–%d: %s", from, to, days)
	}
	return fmt.Sprintf("Week %d: %s", from, days)
}

var (
	promotedRe = regexp.MustCompile(`to (.+?), for `)
	pctRe      = regexp.MustCompile(`\((\d+)%\)`)
	weeksRe    = regexp.MustCompile(`(\d+) weeks? to go`)
)

// chapterTitle is a short, spoiler-safe name for a moment: the run's ending is "how it ends",
// whatever it was.
func chapterTitle(ev show.Event) string {
	t := titleBody(ev)
	if ev.Day > 0 {
		t = fmt.Sprintf("Day %d: %s", ev.Day, t)
	}
	return t
}

// titleBody is a chapter title without its day.
func titleBody(ev show.Event) string {
	t := ""
	switch {
	case ev.Kind == show.KindRunEnd || ev.Beat == show.BeatBuyout:
		t = "How it ends"
	case ev.Kind == show.KindRunStart:
		t = "A new run"
		if ev.Player != nil && ev.Player.PersonaName != "" {
			t = "A new run: the " + ev.Player.PersonaName + " takes the shop"
		}
	case ev.Beat == show.BeatAppraisalJoin:
		t = "Joining the appraisal board"
	case ev.Beat == show.BeatPromotion:
		t = "A promotion from the appraisal board"
		if m := promotedRe.FindStringSubmatch(ev.Text); m != nil {
			t = "Promoted to " + m[1]
		}
	case ev.Beat == show.BeatMortgageSigned:
		t = "Signing the mortgage"
	case ev.Beat == show.BeatMortgagePayment:
		t = "A mortgage payment"
		if m := weeksRe.FindStringSubmatch(ev.Text); m != nil {
			t += ", " + m[0]
		} else if strings.Contains(ev.Text, "last") {
			t = "The last mortgage payment"
		}
	case ev.Beat == show.BeatAug:
		t = "An aug at the counter"
	case ev.Beat == show.BeatHandover:
		t = "Over to the human"
	case ev.Beat == show.BeatTakeBack:
		t = "Back on autopilot"
	case ev.Beat == show.BeatRouteChoice:
		t = "The plan for the run"
	case ev.Beat == show.BeatFork && ev.Candidates != nil:
		t = "A decision"
		for _, o := range ev.Candidates.Options {
			if o.Chosen {
				t = "The call: " + o.Label
			}
		}
	case ev.Beat == show.BeatMilestone:
		t = "A milestone on the road to the deed"
		if m := pctRe.FindStringSubmatch(ev.Text); m != nil {
			pct, _ := strconv.Atoi(m[1])
			t = map[int]string{25: "A quarter of the way to the deed", 50: "Halfway to the deed", 75: "Three quarters of the way to the deed"}[pct]
		}
	case ev.Kind == show.KindBet && ev.Bet != nil:
		t = map[string]string{show.BeatBetMade: "A bet: ", show.BeatBetWon: "Bet won: ", show.BeatBetLost: "Bet lost: ", show.BeatBetAbandoned: "Bet abandoned: "}[ev.Beat] + strings.TrimSuffix(ev.Bet.Claim, ".")
	case show.PlanPivot(ev):
		t = "Change of plan: " + firstChange(ev.Plan.Changes)
	case ev.Kind == show.KindTrade && ev.Trade != nil:
		switch {
		case ev.Trade.Fakes > 0 && ev.Trade.Side == "sold":
			t = "A counterfeit goes out the door"
		case ev.Trade.Side == "sold":
			t = fmt.Sprintf("A big sale: %s credits", Num(ev.Trade.Cash))
		default:
			t = fmt.Sprintf("A big buy: %s credits", Num(ev.Trade.Cash))
		}
	default:
		t = firstClause(ev.Text)
	}
	return t
}

func firstChange(changes []string) string {
	for _, c := range changes {
		if !strings.HasPrefix(c, "no longer: ") {
			return clip(c, 48)
		}
	}
	return "a new course"
}

// firstClause is a caption's first clause, cut to a chapter title's length.
func firstClause(s string) string {
	s = strings.TrimSpace(s)
	for _, sep := range []string{": ", ". ", "; ", " ("} {
		if i := strings.Index(s, sep); i > 0 {
			s = s[:i]
		}
	}
	return clip(strings.TrimRight(s, ".:;,"), 56)
}

// clip cuts s to n characters on a word boundary.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	cut := string(r[:n])
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}

// Stamp is a YouTube timestamp: M:SS under an hour, H:MM:SS from there.
func Stamp(d time.Duration) string {
	s := int(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// Num writes n with thousands separators, as the panel does.
func Num(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}
