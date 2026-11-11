package show

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// Narrator turns moments into short spoken lines in the player's persona. It is separate
// from the decider and never holds the game up: it keeps one pending moment (a newer or
// louder one replaces it), skips moments older than MaxAge, and drops a line that comes
// back later than MaxLag. Without an LLM the caption is the line; without TTS the line is
// a caption only.
type Narrator struct {
	LLM llm.Client
	// LLMOff, when set and true, narrates from captions instead (the game wants the GPU).
	LLMOff func() bool
	TTS    TTS
	Voice  string
	// Persona is the persona id for a run whose player has none; PersonaD the persona
	// directory.
	Persona  string
	PersonaD string
	AudioDir string
	AudioURL string
	MinGap   time.Duration
	// FillAfter, when set, lets a notable event be spoken once the voice has been quiet this
	// long and nothing louder waits: a live day has about one moment, so without it the
	// stream goes minutes without a word.
	FillAfter time.Duration
	MaxAge    time.Duration
	MaxLag    time.Duration
	Timeout   time.Duration
	Log       *slog.Logger
	Publish   func(Event) Event

	mu      sync.Mutex
	pending *Event
	wake    chan struct{}
	player  *Player
	clock   *Clock
	lastNet *int64
	recent  []string
	// books are each persona's phrasebook for bookRun: what it may not say again.
	books   map[string]*phrasebook
	bookRun string
	now     func() time.Time
	// seen is the last event seq taken in; busy is set while a line is made, quietAt is
	// when its audio stops playing. WaitIdle reads them.
	seen    int64
	busy    bool
	quietAt time.Time

	// pace is the run's latest standing against its book (ghost.go), "" without one.
	pace string
}

func (n *Narrator) defaults() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.MinGap <= 0 {
		n.MinGap = 8 * time.Second
	}
	if n.MaxAge <= 0 {
		n.MaxAge = 15 * time.Second
	}
	if n.MaxLag <= 0 {
		n.MaxLag = 25 * time.Second
	}
	if n.Timeout <= 0 {
		n.Timeout = 8 * time.Second
	}
	if n.Log == nil {
		n.Log = slog.New(slog.DiscardHandler)
	}
	if n.AudioURL == "" {
		n.AudioURL = "/audio/"
	}
	if n.now == nil {
		n.now = time.Now
	}
	if n.quietAt.IsZero() {
		// A (re)started narrator has not been silent: what an attach replays is not filler.
		n.quietAt = n.now()
	}
	n.wake = make(chan struct{}, 1)
}

// Priority ranks moments when only one can be spoken.
func Priority(ev Event) int {
	switch ev.Beat {
	case BeatMilestone, BeatPace:
		return 4
	case BeatNight, BeatAppraisalStreak:
		return 2
	}
	switch ev.Kind {
	case KindRunEnd:
		return 6
	case KindRunStart:
		return 5
	case KindAutopilot:
		if ev.Autopilot != nil && ev.Autopilot.NeedsHuman {
			return 5
		}
		return 4
	case KindAug, KindHeat, KindMoney, KindPurchase:
		return 4
	case KindCandidates, KindPlan:
		return 3
	case KindTrade:
		return 2
	}
	return 1
}

// Run narrates the events until ctx ends or events closes.
func (n *Narrator) Run(ctx context.Context, events <-chan Event) {
	n.defaults()
	go n.intake(ctx, events)
	next := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-n.wake:
		}
		if d := next.Sub(n.now()); d > 0 {
			if Sleep(ctx, d) != nil {
				return
			}
		}
		ev := n.take()
		if ev == nil {
			continue
		}
		if age := n.now().Sub(time.UnixMilli(ev.At)); ev.At > 0 && age > n.slack(*ev, n.MaxAge) {
			n.Log.Info("narrator: moment too old; skipped", "kind", ev.Kind, "age", age.Round(time.Second))
			n.setBusy(false, 0)
			continue
		}
		spoken := n.speak(ctx, *ev)
		n.setBusy(false, spoken)
		next = n.now().Add(spoken + n.MinGap)
	}
}

// slack is how old a moment may get: a small beat (the night, a counter story) is still worth
// telling twice as late. With CPU TTS under load a line took 13 s, and every moment that waited
// for it passed the 15 s limit.
func (n *Narrator) slack(ev Event, limit time.Duration) time.Duration {
	if smallBeatCues[ev.Beat] != "" {
		return 2 * limit
	}
	return limit
}

func (n *Narrator) setBusy(busy bool, playing time.Duration) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.busy = busy
	if !busy {
		n.quietAt = n.now().Add(playing)
	}
}

// WaitIdle blocks until the narrator has taken in event seq and has nothing left to say
// about it (spoken and played, or skipped), or until max has passed. A replay calls it
// after each moment: the replay owns its clock, so it can wait for the voice.
func (n *Narrator) WaitIdle(ctx context.Context, seq int64, max time.Duration) {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		n.mu.Lock()
		idle := n.wake != nil && n.seen >= seq && n.pending == nil && !n.busy && !n.now().Before(n.quietAt)
		n.mu.Unlock()
		if idle || Sleep(ctx, 100*time.Millisecond) != nil {
			return
		}
	}
}

func (n *Narrator) intake(ctx context.Context, events <-chan Event) {
	for ev := range events {
		if ctx.Err() != nil {
			return
		}
		n.mu.Lock()
		if ev.Kind == KindRunStart {
			n.clock, n.lastNet, n.pace = nil, nil, ""
			// A restart's replayed run start is the same run: its phrasebook stands.
			if ev.Run != n.bookRun {
				n.books, n.bookRun = nil, ev.Run
			}
			if ev.Player != nil {
				p := *ev.Player
				n.player = &p
			}
		}
		if ev.Clock != nil {
			c := *ev.Clock
			n.clock = &c
		}
		if ev.Ghost != nil {
			n.pace = ev.Ghost.Status
		}
		if ev.Kind == KindDay && ev.Summary != nil {
			d := ev.Summary.NetWorthDelta
			n.lastNet = &d
		}
		fill := n.fills(ev)
		n.mu.Unlock()
		if (ev.Level < LevelMoment && !fill) || ev.Kind == KindSay {
			n.mu.Lock()
			n.seen = max(n.seen, ev.Seq)
			n.mu.Unlock()
			continue
		}
		n.mu.Lock()
		n.seen = max(n.seen, ev.Seq)
		if n.pending == nil || (!fill && (Priority(ev) >= Priority(*n.pending) || n.now().Sub(time.UnixMilli(n.pending.At)) > n.slack(*n.pending, n.MaxAge))) {
			e := ev
			n.pending = &e
		}
		n.mu.Unlock()
		select {
		case n.wake <- struct{}{}:
		default:
		}
	}
}

// fills reports whether a notable event may fill a silence; n.mu is held.
func (n *Narrator) fills(ev Event) bool {
	if n.FillAfter <= 0 || ev.Level != LevelNotable || n.busy || n.pending != nil {
		return false
	}
	switch ev.Kind {
	case KindSay, KindAutopilot:
		return false
	}
	return n.now().Sub(n.quietAt) >= n.FillAfter
}

func (n *Narrator) take() *Event {
	n.mu.Lock()
	defer n.mu.Unlock()
	ev := n.pending
	n.pending = nil
	n.busy = ev != nil
	return ev
}

// speak makes and publishes one line, returning how long its audio plays.
func (n *Narrator) speak(ctx context.Context, ev Event) time.Duration {
	start := n.now()
	sp := n.line(ctx, ev)
	line := sp.line
	if line == "" {
		return 0
	}
	say := &Say{Line: line, For: ev.Seq, Source: sp.source, Check: sp.check, Faults: sp.faults, MS: sp.ms, Mood: n.mood(ev).Name}
	var dur time.Duration
	if n.TTS != nil && n.AudioDir != "" {
		voice := n.voice()
		tctx, cancel := context.WithTimeout(ctx, n.MaxLag)
		wav, err := n.TTS.Speak(tctx, speakable(line), voice)
		cancel()
		if err != nil {
			n.Log.Warn("narrator: tts failed; caption only", "err", err)
		} else {
			name := fmt.Sprintf("line-%06d.wav", ev.Seq)
			if werr := os.WriteFile(filepath.Join(n.AudioDir, name), wav, 0o644); werr == nil {
				say.Audio, say.Voice = n.AudioURL+name, voice
				dur = wavDuration(wav)
			}
		}
	}
	if lag := n.now().Sub(time.UnixMilli(ev.At)); ev.At > 0 && lag > n.slack(ev, n.MaxLag) {
		n.Log.Info("narrator: line too late; dropped", "kind", ev.Kind, "lag", lag.Round(time.Second), "took", n.now().Sub(start).Round(time.Millisecond))
		return 0
	}
	if sp.source == "llm" {
		n.book().add(line)
	}
	n.recent = append(n.recent, line)
	if len(n.recent) > 4 {
		n.recent = n.recent[1:]
	}
	n.Publish(Event{Kind: KindSay, Day: ev.Day, Run: ev.Run, Level: LevelNotable, Text: line, Say: say})
	n.Log.Info("narrator: said", "kind", ev.Kind, "source", sp.source, "check", sp.check, "llm_ms", sp.ms, "audio", say.Audio != "", "took", n.now().Sub(start).Round(time.Millisecond), "line", line)
	return dur
}

// Recall reads a recorded feed (-record) so a restarted narrator remembers what the last
// run's persona already said there.
func (n *Narrator) Recall(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	var run, persona string
	var lines []string
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		switch {
		case ev.Kind == KindRunStart && ev.Run != run:
			run, lines, persona = ev.Run, nil, n.Persona
			if ev.Player != nil && ev.Player.Persona != "" {
				persona = ev.Player.Persona
			}
		case ev.Kind == KindSay && ev.Say != nil && ev.Say.Source == "llm" && ev.Run == run:
			lines = append(lines, ev.Say.Line)
		}
	}
	if run == "" {
		return
	}
	pb := newPhrasebook(persona)
	for _, l := range lines {
		pb.add(l)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.books, n.bookRun = map[string]*phrasebook{persona: pb}, run
}

// personaID is the persona the narrator speaks as.
func (n *Narrator) personaID() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.player != nil && n.player.Persona != "" {
		return n.player.Persona
	}
	return n.Persona
}

// book is the speaking persona's phrasebook.
func (n *Narrator) book() *phrasebook {
	id := n.personaID()
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.books == nil {
		n.books = map[string]*phrasebook{}
	}
	if n.books[id] == nil {
		n.books[id] = newPhrasebook(id)
	}
	return n.books[id]
}

func (n *Narrator) voice() string {
	if n.Voice != "" {
		return n.Voice
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.player != nil && n.player.Persona != "" {
		return PersonaVoice(n.player.Persona)
	}
	return PersonaVoice(n.Persona)
}

// spoken is a line and how it was made.
type spoken struct {
	line, source, check string
	faults              []string
	ms                  int64
}

// line asks the LLM for a line and checks it against the moment's facts and the persona's
// phrasebook; a line that gets a fact wrong or says an old thing again is asked for once more
// with the faults named, then the caption is the line.
func (n *Narrator) line(ctx context.Context, ev Event) spoken {
	caption := spoken{line: templateLine(ev), source: "caption"}
	if n.LLM == nil || (n.LLMOff != nil && n.LLMOff()) {
		return caption
	}
	start := n.now()
	facts := FactsOf(ev)
	mood := n.mood(ev)
	pb := n.book()
	news := ev.Text + "\n" + facts.Prompt()
	if p := ev.Plan; p != nil {
		news += "\n" + strings.Join(slices.Concat(p.Headline, p.Changes, p.Why, p.Goals), "\n")
	}
	msgs := []llm.Message{{Role: llm.RoleUser, Content: n.prompt(ev, facts, pb.avoid(), pb.image(n.personaID(), mood.Name))}}
	var first []string
	for try := 0; try < 2; try++ {
		cctx, cancel := context.WithTimeout(ctx, n.Timeout)
		resp, err := n.LLM.Complete(cctx, llm.Request{System: n.system(mood), Messages: msgs, MaxTokens: 120})
		cancel()
		if err != nil {
			n.Log.Warn("narrator: llm failed; using the caption", "err", err)
			break
		}
		l := cleanLine(resp.Text)
		if l == "" {
			break
		}
		faults := CheckLine(l, facts)
		if len(faults) == 0 {
			// A retry that still repeats goes to the caption: in the rehearsal accepting it let
			// "in the till" through 17 times.
			faults = pb.repeats(l, news)
		}
		if len(faults) == 0 {
			check := "pass"
			if try > 0 {
				check = "retry"
			}
			return spoken{line: l, source: "llm", check: check, faults: first, ms: n.now().Sub(start).Milliseconds()}
		}
		fs := make([]string, len(faults))
		for i, f := range faults {
			fs[i] = string(f)
		}
		if try == 0 {
			first = fs
		}
		n.Log.Info("narrator: line rejected", "try", try+1, "line", l, "faults", fs)
		msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: l},
			llm.Message{Role: llm.RoleUser, Content: "That line will not do: " + strings.Join(fs, "; ") +
				". Say it again in your voice, using only the facts listed. Your line:"})
	}
	caption.check, caption.faults, caption.ms = "fallback", first, n.now().Sub(start).Milliseconds()
	if first == nil {
		caption.check = ""
	}
	return caption
}

// mood is the position at the moment spoken about: its own clock, else the latest one seen.
func (n *Narrator) mood(ev Event) Mood {
	n.mu.Lock()
	defer n.mu.Unlock()
	c := ev.Clock
	if c == nil {
		c = n.clock
	}
	return withPace(MoodOf(c, n.lastNet), n.pace)
}

func (n *Narrator) system(mood Mood) string {
	n.mu.Lock()
	id := n.Persona
	if n.player != nil && n.player.Persona != "" {
		id = n.player.Persona
	}
	var routeBrief string
	if n.player != nil {
		routeBrief = n.player.RouteBrief
	}
	stakes := formatStakes(n.player)
	n.mu.Unlock()
	p := LoadPersona(n.PersonaD, id)
	var b strings.Builder
	who := "the shopkeeper"
	if id != "" {
		who = "the " + p.Name
	}
	fmt.Fprintf(&b, "You are %s, an AI running a pawn shop in the game Probably Stolen, live on stream. You narrate your own play out loud for the viewers.\n", who)
	if p.Tagline != "" {
		fmt.Fprintf(&b, "Your game: %s.\n", strings.ToLower(p.Tagline[:1])+p.Tagline[1:])
	}
	fmt.Fprintf(&b, "Your voice: %s. Stay in that character in every line.\n", p.Style)
	if mood.Cue != "" {
		fmt.Fprintf(&b, "Your mood right now, in that character: %s Let it colour the line; do not talk about how rich or poor you are unless the moment is about money.\n", mood.Cue)
	}
	if routeBrief != "" {
		fmt.Fprintf(&b, "This run's route, what your shop does (background for what you care about; never quote its numbers):\n%s\n", routeBrief)
	}
	b.WriteString(stakes)
	if p.Text != "" {
		fmt.Fprintf(&b, "How you play (background for motive and attitude; its prices and days are not today's facts, never quote them):\n%s\n", clip(strings.TrimSpace(p.Text), 900))
	}
	b.WriteString("Speak ONE or TWO short sentences, at most 28 words, first person, reacting to the moment you are given. " +
		"It is read aloud by text-to-speech: plain words only, no emojis, no stage directions, no quotation marks, no lists. " +
		"Invent feeling, motive, character and wry asides freely, but the numbers are sacred: use at most one number, only one from the facts you are given, rounded if you like, written in digits (242, 1,500: the voice reads them), " +
		"and never claim anything about money, rent or trades that the facts do not say. Never name a weekday. Money is credits, never dollars. Do not repeat your previous lines or their images.")
	return b.String()
}

func (n *Narrator) prompt(ev Event, f Facts, avoid []string, image string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Day %d. Moment: %s\n", ev.Day, ev.Text)
	if cue := smallBeatCues[ev.Beat]; cue != "" {
		fmt.Fprintf(&b, "This moment: %s\n", cue)
	} else if cue := beatCues[ev.Beat]; cue != "" {
		fmt.Fprintf(&b, "This is a big moment of the run: %s\n", cue)
	}
	if p := ev.Plan; p != nil {
		if len(p.Headline) > 0 {
			fmt.Fprintf(&b, "Plan: %s\n", strings.Join(p.Headline, "; "))
		}
		if len(p.Changes) > 0 {
			fmt.Fprintf(&b, "Changed since yesterday: %s\n", strings.Join(p.Changes[:min(4, len(p.Changes))], "; "))
		}
		var reasons, fixes []string
		for _, w := range p.Why {
			if r, ok := strings.CutPrefix(w, "overruled: "); ok {
				fixes = append(fixes, r)
			} else {
				reasons = append(reasons, w)
			}
		}
		if len(reasons) > 0 {
			fmt.Fprintf(&b, "Your reasons: %s\n", strings.Join(reasons[:min(3, len(reasons))], "; "))
		}
		if len(fixes) > 0 {
			fmt.Fprintf(&b, "The bridge corrected you: %s\n", fixes[0])
		}
		if len(p.Headline) == 0 && len(p.Goals) > 0 {
			fmt.Fprintf(&b, "Standing goals: %s\n", strings.Join(p.Goals[:min(3, len(p.Goals))], "; "))
		}
	}
	if facts := f.Prompt(); facts != "" {
		b.WriteString("Facts:\n" + facts)
	}
	if len(n.recent) > 0 {
		fmt.Fprintf(&b, "Your previous lines: %s\n", strings.Join(n.recent, " / "))
	}
	if len(avoid) > 0 {
		fmt.Fprintf(&b, "Words to rest, you have leaned on them lately (do not use them): %s\n", strings.Join(avoid, "; "))
	}
	if image != "" {
		fmt.Fprintf(&b, "A fresh image you could use, only if it fits: %s.\n", image)
	}
	b.WriteString("Your line:")
	return b.String()
}

// beatCues tell the narrator what kind of big moment it is speaking about.
var beatCues = map[string]string{
	BeatAppraisalJoin:   "you just joined the appraisal board; appraisal customers will pay fees for your eye.",
	BeatPromotion:       "the appraisal board just promoted you: a step up in rank, standing and fees. Savour it.",
	BeatMortgageSigned:  "you just signed the mortgage: the shop will be yours if the weekly payments keep coming. A gamble with the deed.",
	BeatMortgagePayment: "a mortgage payment just went out. One step closer to owning the place; say how it feels.",
	BeatBuyout:          "the shop is bought: the run is won. This is the finale; make it land.",
	BeatAug:             "you just pulled the gun on an aug at your counter. Tense, quick, then the relief.",
	BeatHandover:        "the autopilot just handed the controls to its human operator. Say plainly that it is over to the human, and why, in a few words.",
	BeatTakeBack:        "the human handed the shop back; the AI is in control again.",
}

// smallBeatCues say what an everyday moment is, so the line tells it rather than reads it.
var smallBeatCues = map[string]string{
	BeatCounter:         "a customer at the counter. Tell it like a tiny story: who they were, how the haggle went, what changed hands.",
	BeatNight:           "the night out after closing. Say what the night brought home, or that it brought nothing.",
	BeatMorning:         "the morning. Say what you mean to do today, in one sentence, in your own words.",
	BeatMilestone:       "a milestone on the road to buying the shop. Mark it.",
	BeatAppraisalStreak: "a run of appraisals: your eye is earning its fees.",
	BeatPace:            "the race against the book, the lab's test runs of your route: you just pulled ahead of it, fell behind it, or crossed the edge of its band. Say where you stand against that ghost and how it feels.",
}

var (
	thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)
	spaceRe = regexp.MustCompile(`\s+`)
	sentRe  = regexp.MustCompile(`[.!?](\s|$)`)
)

// cleanLine keeps a model's answer to two speakable sentences.
func cleanLine(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	s = strings.NewReplacer("*", "", "\"", "", "“", "", "”", "", "#", "").Replace(s)
	s = strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
	if i := strings.Index(strings.ToLower(s), "line:"); i >= 0 && i < 20 {
		s = strings.TrimSpace(s[i+5:])
	}
	if locs := sentRe.FindAllStringIndex(s, -1); len(locs) > 2 {
		s = s[:locs[1][0]+1]
	}
	return clip(s, 240)
}

// templateLine is the caption as a line, cut to what fits a breath.
func templateLine(ev Event) string {
	if p := ev.Plan; p != nil && len(p.Headline) == 0 && len(p.Directives) > 2 {
		return fmt.Sprintf("Day %d plan: %s.", ev.Day, capitalize(joinAnd(p.Directives[:2])))
	}
	return clip(ev.Text, 200)
}

var (
	countRe = regexp.MustCompile(`(\d+)× `)
	plusRe  = regexp.MustCompile(`\+(\d)`)
)

// speakable rewrites the few symbols TTS reads badly.
func speakable(s string) string {
	s = countRe.ReplaceAllString(s, "$1 ")
	s = plusRe.ReplaceAllString(s, "plus $1")
	// Kokoro reads ">=" as "equals" and drops ">": plan captions carry both.
	return strings.NewReplacer("%", " percent", "…", ".", "&", " and ", "≥", "at least", ">=", "at least", "≤", "at most", "<=", "at most",
		">", "over ", "<", "under ").Replace(s)
}

// wavDuration reads a PCM WAV's length from its header; 0 when it cannot tell.
func wavDuration(b []byte) time.Duration {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0
	}
	var byteRate uint32
	for i := 12; i+8 <= len(b); {
		id, size := string(b[i:i+4]), binary.LittleEndian.Uint32(b[i+4:i+8])
		switch id {
		case "fmt ":
			if i+16 <= len(b) {
				byteRate = binary.LittleEndian.Uint32(b[i+16 : i+20])
			}
		case "data":
			n := uint64(size)
			if rest := uint64(len(b) - i - 8); n == 0 || n > rest {
				n = rest
			}
			if byteRate == 0 {
				return 0
			}
			return time.Duration(n * uint64(time.Second) / uint64(byteRate))
		}
		i += 8 + int(size)
		if size%2 == 1 {
			i++
		}
	}
	return 0
}

var (
	lineWordRe = regexp.MustCompile(`[a-z']+`)
	// stopWords and plainWords never make a phrase a repeat on their own: "rent is due
	// tomorrow" is news each week, "rent is just a whisper" is a cliché.
	stopWords = setOf("a", "an", "the", "and", "or", "but", "so", "of", "to", "in", "on", "at", "for", "with", "from", "by", "is", "are", "was", "be", "am", "i", "i'm", "me", "my", "it", "it's", "its", "that", "this", "just", "not", "no", "all", "up", "out", "like", "still", "got", "get", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "hundred", "thousand",
		"then", "now", "here", "there", "what", "when", "we", "you", "he", "his", "they", "them", "your", "our", "as", "if", "too", "more", "some", "every", "another", "back", "over", "off", "into", "than", "have", "has", "had", "will", "can", "do", "did", "him", "her", "she", "us")
	plainWords = setOf("rent", "rent's", "due", "today", "tomorrow", "day", "days", "week", "credits", "cash", "paid", "pay", "sold", "bought", "net", "worth")
)

func setOf(ws ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range ws {
		m[w] = true
	}
	return m
}
