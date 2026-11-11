package vod

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
)

// t0 is when the synthetic run starts; a game day lasts two minutes, as on the stream.
var t0 = time.Date(2026, 11, 11, 6, 0, 0, 0, time.UTC)

const day = 2 * time.Minute

const runID = "p:synthetic"

// at is a time on day d, off into it.
func at(d int, off time.Duration) time.Time { return t0.Add(time.Duration(d-1)*day + off) }

func ev(d int, off time.Duration, kind show.Kind, level int, beat, text string) show.Event {
	return show.Event{V: 1, TS: at(d, off).Format(time.RFC3339Nano), Run: runID, Day: d, Kind: kind, Level: level, Beat: beat, Text: text}
}

// synthetic is a 30-day run of our own made-up events: a morning plan and a counter story each
// day, the big beats on fixed days, five aug confrontations, a kind no package defines yet, and a
// buyout on day 30.
func synthetic() []show.Event {
	start := ev(1, 0, show.KindRunStart, show.LevelMoment, "", "New run.")
	start.Player = &show.Player{ID: "p", Model: "test-model-7b", PersonaName: "Tester", PersonaTagline: "All checks", Route: "sprinter", Difficulty: "hard", Seed: 42}
	out := []show.Event{start}
	for d := 1; d <= 30; d++ {
		plan := ev(d, 5*time.Second, show.KindPlan, show.LevelMoment, show.BeatMorning, fmt.Sprintf("Day %d plan: keep the floor.", d))
		plan.Plan = &show.Plan{Day: d}
		counter := ev(d, 60*time.Second, show.KindTrade, show.LevelMoment, show.BeatCounter, "Sold a crate for 80.")
		counter.Trade = &show.Trade{Side: "sold", Cash: 80}
		close := ev(d, 110*time.Second, show.KindDay, show.LevelNotable, "", "Day closed.")
		close.Clock = &show.Clock{Cash: int64(100 * d), NetWorth: int64(500 * d)}
		out = append(out, plan, counter, close)
	}
	pivot := ev(9, 30*time.Second, show.KindPlan, show.LevelMoment, show.BeatMorning, "Day 9 plan: save for the deed.")
	pivot.Plan = &show.Plan{Day: 9, Change: true, Changes: []string{"save for the deed"}}
	out = append(out,
		ev(3, 40*time.Second, show.KindPurchase, show.LevelMoment, show.BeatAppraisalJoin, "Joined the appraisal board for 170: appraisal customers from now on."),
		ev(12, 40*time.Second, show.KindPurchase, show.LevelMoment, show.BeatPromotion, "Promoted by the appraisal board to Appraiser, for 212: bigger appraisals, bigger fees."),
		ev(17, 40*time.Second, show.KindThought, show.LevelMoment, show.BeatMilestone, "Net worth 6,000: halfway to the 12,000 buyout (50%)."),
		ev(20, 50*time.Second, show.Kind("bet"), show.LevelMoment, "bet_won", "Bet won: cash 9,000 by day 20."),
		pivot,
	)
	for _, d := range []int{5, 11, 15, 22, 26} {
		out = append(out, ev(d, 80*time.Second, show.KindAug, show.LevelMoment, show.BeatAug, "Shot an aug: bounty 119."))
	}
	buy := ev(30, 90*time.Second, show.KindMoney, show.LevelMoment, show.BeatBuyout, "Bought the shop outright: 12,000.")
	buy.Amount = -12000
	end := ev(30, 95*time.Second, show.KindRunEnd, show.LevelMoment, "", "The run ends: buyout.")
	end.End = &show.End{Ending: "buyout_cash", Day: 30, NetWorth: 15000, Won: true}
	return append(out, buy, end)
}

// say is a narrator line spoken after the moment, told in the feed by seq.
func say(moment show.Event, after time.Duration, line string) show.Event {
	t, _ := time.Parse(time.RFC3339Nano, moment.TS)
	return show.Event{V: 1, At: t.Add(after).UnixMilli(), Run: runID, Day: moment.Day, Kind: show.KindSay, Level: show.LevelNotable,
		Text: line, Say: &show.Say{Line: line, For: moment.Seq, Source: "llm", Check: "pass"}}
}

func oneRecording(t *testing.T, events, lines []show.Event, start time.Time) (*Part, *Timeline) {
	t.Helper()
	tl := Build(events, lines)
	parts, err := Split(tl, []Recording{{Start: start}})
	if err != nil {
		t.Fatal(err)
	}
	return &parts[0], tl
}

func TestStamp(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{0, "0:00"}, {9 * time.Second, "0:09"}, {61 * time.Second, "1:01"}, {59*time.Minute + 59*time.Second, "59:59"},
		{time.Hour, "1:00:00"}, {3*time.Hour + 12*time.Minute + 5*time.Second, "3:12:05"},
	} {
		if got := Stamp(c.d); got != c.want {
			t.Errorf("Stamp(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestParseStart(t *testing.T) {
	loc := time.FixedZone("AST", -4*3600)
	for _, c := range []struct {
		in      string
		want    time.Time
		length  time.Duration
		wantErr bool
	}{
		{in: "2026-11-11T06:36:40Z", want: time.Date(2026, 11, 11, 6, 36, 40, 0, time.UTC)},
		{in: "2026-11-11 02:36:40", want: time.Date(2026, 11, 11, 6, 36, 40, 0, time.UTC)},
		{in: "2026-11-11T02:36", want: time.Date(2026, 11, 11, 6, 36, 0, 0, time.UTC)},
		{in: "2026-11-11 02-36-40", want: time.Date(2026, 11, 11, 6, 36, 40, 0, time.UTC)},
		{in: "2026-11-11T06:36:40Z/3h10m", want: time.Date(2026, 11, 11, 6, 36, 40, 0, time.UTC), length: 190 * time.Minute},
		{in: "2026-11-11 02:36:40/soon", wantErr: true},
		{in: "yesterday", wantErr: true},
	} {
		r, err := ParseStart(c.in, loc)
		if (err != nil) != c.wantErr {
			t.Errorf("ParseStart(%q) error %v, want error %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && (!r.Start.Equal(c.want) || r.Length != c.length) {
			t.Errorf("ParseStart(%q) = %v %v, want %v %v", c.in, r.Start, r.Length, c.want, c.length)
		}
	}
}

func TestStartFromName(t *testing.T) {
	for _, c := range []struct {
		path string
		ok   bool
	}{
		{"/rec/2026-11-11 02-36-40.mkv", true},
		{"stream 2026-11-11 02-36-40.mp4", true},
		{"/rec/final-cut.mkv", false},
	} {
		got, ok := StartFromName(c.path, time.UTC)
		if ok != c.ok || (ok && !got.Equal(time.Date(2026, 11, 11, 2, 36, 40, 0, time.UTC))) {
			t.Errorf("StartFromName(%q) = %v %v", c.path, got, ok)
		}
	}
}

func TestBuildDropsRestartRepeats(t *testing.T) {
	first := ev(1, 0, show.KindPlan, show.LevelNotable, "", "Day 1 plan.")
	first.At, first.Seq = at(1, 0).UnixMilli(), 1
	// A restart publishes the latest plan again: same source ts, a later publish time.
	again := first
	again.At, again.Seq = at(1, 30*time.Second).UnixMilli(), 1
	tl := Build([]show.Event{first, again}, nil)
	if len(tl.Points) != 1 {
		t.Fatalf("points = %d, want 1 (the restart's repeat dropped)", len(tl.Points))
	}
}

func TestSplit(t *testing.T) {
	tl := Build(synthetic(), nil)
	recs := []Recording{
		{Start: at(1, -time.Minute)},
		{Start: at(16, 0), Length: 10 * day},
		{Start: at(26, 0)},
	}
	parts, err := Split(tl, recs)
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []struct {
		from, to int
		length   time.Duration
	}{
		{1, 15, 15*day + time.Minute},
		{16, 25, 10 * day},
		{26, 30, 4*day + 110*time.Second + tail},
	} {
		p := parts[i]
		from, to := p.Days()
		if from != want.from || to != want.to || p.Rec.Length != want.length || p.N != i+1 || p.Of != 3 {
			t.Errorf("part %d: days %d–%d, length %v, %d of %d; want days %d–%d, length %v", i+1, from, to, p.Rec.Length, p.N, p.Of, want.from, want.to, want.length)
		}
	}
	if _, err := Split(tl, []Recording{{Start: at(40, 0)}}); err == nil {
		t.Error("a recording after the feed's end: want an error")
	}
}

func TestChapters(t *testing.T) {
	for _, c := range []struct {
		name    string
		start   time.Time
		opening string
	}{
		{"recording starts a minute early", at(1, -time.Minute), "A new run"},
		{"recording joins mid-week", at(9, 0), "Week 2: days 9–14"},
		{"recording starts before a quiet stretch", at(1, -10*time.Minute), "The stream starts"},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, _ := oneRecording(t, synthetic(), nil, c.start)
			ch := Chapters(p)
			if len(ch) < minChapters || len(ch) > maxChapters {
				t.Fatalf("%d chapters", len(ch))
			}
			if ch[0].At != 0 || !strings.Contains(ch[0].Title, c.opening) {
				t.Errorf("first chapter %s %q, want 0:00 with %q", ch[0].Stamp, ch[0].Title, c.opening)
			}
			augs, weeks := 0, 0
			for i, x := range ch {
				if i > 0 && x.At-ch[i-1].At < minChapter {
					t.Errorf("chapters %d and %d are %v apart", i-1, i, x.At-ch[i-1].At)
				}
				if x.Beat == show.BeatAug {
					augs++
				}
				if strings.HasPrefix(x.Title, "Week") {
					weeks++
				}
				for _, banned := range []string{"Bought the shop", "buyout"} {
					if strings.Contains(x.Title, banned) {
						t.Errorf("chapter %q gives the ending away", x.Title)
					}
				}
				if x.Beat == show.BeatCounter || x.Beat == show.BeatMorning && !strings.Contains(x.Title, "Change of plan") {
					t.Errorf("routine beat as a chapter: %q", x.Title)
				}
			}
			if augs > 2 {
				t.Errorf("%d aug chapters: diversity should hold them to two", augs)
			}
			if weeks == 0 {
				t.Error("no week chapters")
			}
			last := ch[len(ch)-1]
			if !strings.HasSuffix(last.Title, "How it ends") {
				t.Errorf("last chapter %q, want the spoiler-safe ending", last.Title)
			}
		})
	}
}

func TestChaptersTakeUnknownKinds(t *testing.T) {
	// A quiet run: the bet, a kind this package does not know, is its only loud moment.
	var events []show.Event
	for d := 1; d <= 30; d++ {
		events = append(events, ev(d, 0, show.KindDay, show.LevelNotable, "", "Day closed."))
	}
	events = append(events, ev(20, 50*time.Second, show.Kind("bet"), show.LevelMoment, "bet_won", "Bet won: cash 9,000 by day 20."))
	p, _ := oneRecording(t, events, nil, at(1, 0))
	for _, x := range Chapters(p) {
		if x.Kind == "bet" {
			if x.Title != "Day 20: Bet won" {
				t.Errorf("bet chapter %q", x.Title)
			}
			return
		}
	}
	t.Error("a moment of a kind this package does not know never became a chapter")
}

func TestChapterTitle(t *testing.T) {
	trade := func(side string, cash int64, fakes int) show.Event {
		e := ev(4, 0, show.KindTrade, show.LevelMoment, "", "Sold things.")
		e.Trade = &show.Trade{Side: side, Cash: cash, Fakes: fakes}
		return e
	}
	pivot := ev(9, 0, show.KindPlan, show.LevelMoment, show.BeatMorning, "Day 9 plan.")
	pivot.Plan = &show.Plan{Change: true, Changes: []string{"no longer: hold the crate", "take the mortgage"}}
	for _, c := range []struct {
		ev   show.Event
		want string
	}{
		{ev(30, 0, show.KindMoney, show.LevelMoment, show.BeatBuyout, "Bought the shop outright: 26,400."), "Day 30: How it ends"},
		{ev(12, 0, show.KindPurchase, show.LevelMoment, show.BeatPromotion, "Promoted by the appraisal board to Master Appraiser, for 298: bigger appraisals."), "Day 12: Promoted to Master Appraiser"},
		{ev(17, 0, show.KindThought, show.LevelMoment, show.BeatMilestone, "Net worth 18,000: three quarters of the way to the 24,000 buyout (75%)."), "Day 17: Three quarters of the way to the deed"},
		{ev(40, 0, show.KindMoney, show.LevelMoment, show.BeatMortgagePayment, "Mortgage payment made: 500. 12 weeks to go."), "Day 40: A mortgage payment, 12 weeks to go"},
		{pivot, "Day 9: Change of plan: take the mortgage"},
		{trade("sold", 1250, 0), "Day 4: A big sale: 1,250 credits"},
		{trade("bought", 600, 0), "Day 4: A big buy: 600 credits"},
		{trade("sold", 90, 1), "Day 4: A counterfeit goes out the door"},
		{ev(5, 0, show.Kind("fork"), show.LevelMoment, "", "The run forks: two strategists from one morning. Both play on."), "Day 5: The run forks"},
	} {
		if got := chapterTitle(c.ev); got != c.want {
			t.Errorf("chapterTitle(%s %s) = %q, want %q", c.ev.Kind, c.ev.Beat, got, c.want)
		}
	}
}

func TestHighlights(t *testing.T) {
	events := synthetic()
	for i := range events {
		events[i].Seq = int64(i + 1)
	}
	var lines []show.Event
	var promo show.Event
	for _, e := range events {
		if e.Beat == show.BeatPromotion {
			promo = e
		}
	}
	lines = append(lines, say(promo, 6*time.Second, "Appraiser now, and the fees climb with me."))
	feed := append(append([]show.Event{}, events...), lines...)
	p, _ := oneRecording(t, feed, nil, at(1, -time.Minute))
	clips := Highlights(p, 8)
	if len(clips) == 0 {
		t.Fatal("no clips")
	}
	var told bool
	for i, c := range clips {
		if l := c.End - c.Start; l < minClip || l > maxClip {
			t.Errorf("clip %d is %v long", i, l)
		}
		if c.Start < 0 || c.End > p.Rec.Length {
			t.Errorf("clip %d [%v, %v] outside the recording (%v)", i, c.Start, c.End, p.Rec.Length)
		}
		if i > 0 && c.Start < clips[i-1].End {
			t.Errorf("clips %d and %d overlap", i-1, i)
		}
		if c.Beat == show.BeatPromotion {
			told = c.Line != "" && strings.Contains(c.Caption, c.Line)
			lineAt := p.Off(time.UnixMilli(lines[0].At))
			if c.End < lineAt+3*time.Second {
				t.Errorf("the promotion clip ends at %v, before its line (at %v) is spoken", c.End, lineAt)
			}
		}
	}
	if !told {
		t.Error("the promotion's narrated line is not its clip's caption")
	}
}

// A decision log's moments with a feed's lines: the line goes to the log moment with the source
// time of the moment its own feed says it told, even after a restart renumbered that feed.
func TestHighlightsLinesFromAnotherFeed(t *testing.T) {
	log := synthetic()
	var aug show.Event
	for _, e := range log {
		if e.Beat == show.BeatAug && e.Day == 11 {
			aug = e
		}
	}
	old := aug
	old.Seq, old.Beat = 7, ""
	// Restarted earlier in the feed: seq 7 was another event before.
	stale := ev(2, 0, show.KindTrade, show.LevelMoment, "", "Sold a lamp.")
	stale.Seq = 7
	line := say(old, 20*time.Second, "The chrome came out and so did the bounty.")
	p, _ := oneRecording(t, log, []show.Event{stale, old, line}, at(1, 0))
	for _, c := range Highlights(p, 20) {
		if c.Line != "" && c.Day != 11 {
			t.Errorf("the line went to day %d's %s", c.Day, c.Kind)
		}
		if c.Day == 11 && c.Beat == show.BeatAug && c.Line == "" {
			t.Error("the aug on day 11 lost its line")
		}
	}
}

func TestKit(t *testing.T) {
	events := synthetic()
	// A format card from a branch this package does not import: read by its JSON alone.
	var start map[string]any
	b, _ := json.Marshal(events[0])
	_ = json.Unmarshal(b, &start)
	start["player"].(map[string]any)["format"] = map[string]any{"name": "sprint", "title": "The Sprint", "deadline_day": 35}
	b, _ = json.Marshal(start)
	var feed bytes.Buffer
	feed.Write(b)
	feed.WriteByte('\n')
	enc := json.NewEncoder(&feed)
	for _, e := range events[1:] {
		_ = enc.Encode(e)
	}
	evs, formats, err := ReadFeed(&feed)
	if err != nil {
		t.Fatal(err)
	}
	p, tl := oneRecording(t, evs, nil, at(1, -time.Minute))
	tl.SetFormats(formats)
	k := Make(p, tl.Runs, 5)
	for _, want := range []string{"The Sprint", "test-model-7b", "Tester", "hard", "sprinter"} {
		if !strings.Contains(k.Title, want) {
			t.Errorf("title %q lacks %q", k.Title, want)
		}
	}
	for _, spoiler := range []string{"buy", "Bought", "won", "30"} {
		if strings.Contains(k.Title, spoiler) {
			t.Errorf("title %q gives away %q", k.Title, spoiler)
		}
	}
	cut := strings.Index(k.Description, SpoilerLine)
	res := strings.Index(k.Description, "Result: the shop is bought on day 30")
	chapters := strings.Index(k.Description, "0:00 ")
	if cut < 0 || res < cut || chapters < 0 || chapters > cut {
		t.Errorf("description: chapters at %d, spoiler line at %d, result at %d\n%s", chapters, cut, res, k.Description)
	}
	if !strings.Contains(k.Description, "Inside the format's day 35 deadline") {
		t.Error("description lacks the format's verdict")
	}
	n := 0
	for _, tag := range k.Tags {
		n += len(tag) + 1
	}
	if n > 500 || len(k.Tags) < 5 {
		t.Errorf("tags %v (%d characters)", k.Tags, n)
	}

	dir := t.TempDir()
	k.Recording = filepath.Join(dir, "it's a recording.mkv")
	if err := Write(dir, k); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"title.txt", "description.txt", "chapters.txt", "tags.txt", "highlights.tsv", "cut.sh", "kit.json"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Error(err)
		}
	}
	sh, _ := os.ReadFile(filepath.Join(dir, "cut.sh"))
	if !strings.Contains(string(sh), `'\''s a recording.mkv'`) || strings.Count(string(sh), "ffmpeg -hide_banner") != len(k.Clips) {
		t.Errorf("cut.sh:\n%s", sh)
	}
}

func TestPartWithoutEnd(t *testing.T) {
	tl := Build(synthetic(), nil)
	parts, err := Split(tl, []Recording{{Start: at(1, 0)}, {Start: at(16, 0)}})
	if err != nil {
		t.Fatal(err)
	}
	k := Make(&parts[0], tl.Runs, 3)
	if !strings.Contains(k.Description, "Where this part leaves it: day 15") || strings.Contains(k.Description, "Result:") {
		t.Errorf("part 1's description:\n%s", k.Description)
	}
	if !strings.HasSuffix(k.Title, "part 1 of 2") {
		t.Errorf("title %q", k.Title)
	}
}
