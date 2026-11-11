package show

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

type fakeLLM struct {
	mu     sync.Mutex
	asked  []string
	system []string
	reply  string
	err    error
}

func (f *fakeLLM) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, r.Messages[0].Content)
	f.system = append(f.system, r.System)
	return llm.Response{Text: f.reply}, f.err
}

func (f *fakeLLM) lastSystem() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.system) == 0 {
		return ""
	}
	return f.system[len(f.system)-1]
}

func timeNowMS() int64 { return time.Now().UnixMilli() }

type fakeTTS struct{ err error }

func (fakeTTS) Name() string { return "fake" }
func (f fakeTTS) Speak(context.Context, string, string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return testWAV(24000, 12000), nil
}

// testWAV is a mono 16-bit PCM WAV of n samples.
func testWAV(rate, n int) []byte {
	b := make([]byte, 44+2*n)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(36+2*n))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*2))
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(2*n))
	return b
}

func TestWavDuration(t *testing.T) {
	if got := wavDuration(testWAV(24000, 12000)); got != 500*time.Millisecond {
		t.Errorf("duration = %v", got)
	}
	if got := wavDuration([]byte("not a wav")); got != 0 {
		t.Errorf("garbage duration = %v", got)
	}
}

func TestCleanLine(t *testing.T) {
	cases := map[string]string{
		"<think>hm</think>  \"Rent's paid.\"  ":                            "Rent's paid.",
		"Line: **One.** Two! Three? Four.":                                 "One. Two!",
		"Wholesale at a discount, and I'll take it every time":             "Wholesale at a discount, and I'll take it every time",
		"Day 41. I shot the aug. Bounty's mine. The Lower Levels approve.": "Day 41. I shot the aug.",
	}
	for in, want := range cases {
		if got := cleanLine(in); got != want {
			t.Errorf("cleanLine(%q) = %q, want %q", in, got, want)
		}
	}
	if got := speakable("Sold 2× Lamp for 40 (+12 on cost), 30%"); got != "Sold 2 Lamp for 40 (plus 12 on cost), 30 percent" {
		t.Errorf("speakable = %q", got)
	}
	// A plan caption spoken in the rehearsal (day 43).
	if got := speakable("Cash >= 2800 at close (target: >3000), floor ≤ 600"); got != "Cash at least 2800 at close (target: over 3000), floor at most 600" {
		t.Errorf("speakable = %q", got)
	}
}

// narrate runs a narrator over evs until it is idle and returns what it said.
func narrate(t *testing.T, n *Narrator, evs ...Event) []Event {
	t.Helper()
	var mu sync.Mutex
	var said []Event
	n.Publish = func(e Event) Event {
		mu.Lock()
		said = append(said, e)
		mu.Unlock()
		return e
	}
	if n.AudioDir == "" {
		n.AudioDir = t.TempDir()
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Event, len(evs))
	done := make(chan struct{})
	go func() { n.Run(ctx, ch); close(done) }()
	var last int64
	for _, e := range evs {
		ch <- e
		last = max(last, e.Seq)
	}
	n.WaitIdle(ctx, last, 5*time.Second)
	cancel()
	close(ch)
	<-done
	mu.Lock()
	defer mu.Unlock()
	return said
}

func TestNarratorSpeaksTheLoudestFreshMoment(t *testing.T) {
	at := time.Now().UnixMilli()
	f := &fakeLLM{reply: "Evicted. The landlord wins this round."}
	var said []Event
	n := &Narrator{LLM: f, TTS: fakeTTS{}, AudioDir: t.TempDir(), Publish: func(e Event) Event { said = append(said, e); return e }}
	n.defaults()
	ch := make(chan Event, 4)
	for _, e := range []Event{
		{Seq: 1, Kind: KindThought, Level: LevelTicker, At: at, Text: "a customer"},
		{Seq: 2, Kind: KindTrade, Level: LevelMoment, At: at, Text: "Sold a lamp for 900."},
		{Seq: 3, Kind: KindRunEnd, Level: LevelMoment, At: at, Day: 9, Text: "Day 9: could not pay the rent."},
		{Seq: 4, Kind: KindTrade, Level: LevelMoment, At: at, Text: "Sold a kettle for 600."},
	} {
		ch <- e
	}
	close(ch)
	// While one moment waits, a louder one replaces it and a quieter one does not.
	n.intake(context.Background(), ch)
	ev := n.take()
	if ev == nil || ev.Seq != 3 {
		t.Fatalf("pending = %+v", ev)
	}
	n.speak(context.Background(), *ev)
	if len(said) != 1 {
		t.Fatalf("said %d lines: %+v", len(said), said)
	}
	s := said[0]
	if s.Kind != KindSay || s.Say.Line != "Evicted. The landlord wins this round." || s.Say.For != 3 || s.Say.Source != "llm" {
		t.Errorf("said %+v", s.Say)
	}
	if !strings.HasPrefix(s.Say.Audio, "/audio/line-000003") {
		t.Errorf("audio = %q", s.Say.Audio)
	}
	if _, err := os.Stat(filepath.Join(n.AudioDir, "line-000003.wav")); err != nil {
		t.Error(err)
	}
}

func TestNarratorDegrades(t *testing.T) {
	fresh := time.Now().UnixMilli()
	moment := Event{Seq: 5, Kind: KindMoney, Level: LevelMoment, At: fresh, Text: "Lent 700 to Georgio."}

	said := narrate(t, &Narrator{LLM: &fakeLLM{err: errors.New("down")}, TTS: fakeTTS{err: errors.New("no tts")}}, moment)
	if len(said) != 1 || said[0].Say.Line != moment.Text || said[0].Say.Audio != "" || said[0].Say.Source != "caption" {
		t.Errorf("degraded = %+v", said)
	}
	said = narrate(t, &Narrator{LLM: &fakeLLM{reply: "x"}, LLMOff: func() bool { return true }}, moment)
	if len(said) != 1 || said[0].Say.Source != "caption" {
		t.Errorf("game running = %+v", said)
	}
	old := moment
	old.At = time.Now().Add(-time.Minute).UnixMilli()
	if said := narrate(t, &Narrator{}, old); len(said) != 0 {
		t.Errorf("an old moment was narrated: %+v", said)
	}
}

func TestWaitIdle(t *testing.T) {
	n := &Narrator{TTS: fakeTTS{}, AudioDir: t.TempDir(), MinGap: time.Hour, Publish: func(e Event) Event { return e }}
	ch := make(chan Event, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go n.Run(ctx, ch)
	ch <- Event{Seq: 9, Kind: KindMoney, Level: LevelMoment, At: time.Now().UnixMilli(), Text: "Paid the rent."}
	start := time.Now()
	n.WaitIdle(ctx, 9, 5*time.Second)
	// The fake line plays for 500 ms; the wait covers it but not the hour of MinGap.
	if d := time.Since(start); d < 400*time.Millisecond || d > 5*time.Second {
		t.Errorf("WaitIdle took %v", d)
	}
}

func TestPromptStatesRentCover(t *testing.T) {
	n := &Narrator{}
	ev := Event{Day: 41, Text: "Shot an aug.", Clock: &Clock{Cash: 1304, RentDue: 321, RentDays: 1}}
	if got := n.prompt(ev, FactsOf(ev), nil, ""); !strings.Contains(got, "Rent: 321 due in 1 day. Your cash covers it with 983 to spare. You are NOT short on rent.") {
		t.Errorf("prompt = %q", got)
	}
	ev = Event{Day: 42, Text: "Paid the rent.", Clock: &Clock{Cash: 100, RentDue: 300, Arrears: 50, RentDays: 0}}
	if got := n.prompt(ev, FactsOf(ev), nil, ""); !strings.Contains(got, "you are short by 250") {
		t.Errorf("prompt = %q", got)
	}
}

func TestPersona(t *testing.T) {
	dir := "../../docs/personas"
	p := LoadPersona(dir, "shark")
	if p.Name != "Shark" || p.Tagline != "Nothing but nerve" || p.Voice != "am_fenrir" || p.Style == "" || p.Text == "" {
		t.Errorf("shark = %+v", p)
	}
	var pl Player
	pl.SetPersona(dir, "", DefaultPersona)
	if pl.Persona != "magnate" || pl.PersonaName != "Magnate" || pl.PersonaTagline != "The generalist" {
		t.Errorf("default = %+v", pl)
	}
	n := &Narrator{PersonaD: dir, Persona: DefaultPersona}
	n.defaults()
	if s := n.system(Mood{}); !strings.Contains(s, "You are the Magnate") || !strings.Contains(s, "old-money") || !strings.Contains(s, "numbers are sacred") {
		t.Errorf("system = %q", s)
	}
	n.player = &Player{Persona: "shark"}
	if n.voice() != "am_fenrir" || !strings.Contains(n.system(Mood{}), "street hustler") {
		t.Errorf("shark narrator: %s, %q", n.voice(), n.system(Mood{}))
	}
	if strings.Contains(n.system(Mood{}), "route") {
		t.Errorf("route background without a route: %q", n.system(Mood{}))
	}
	n.player.SetRoute("../../sim/lab/scripts/directives/routes/rock_bottom.json")
	if n.player.Route != "rock_bottom" || !strings.Contains(n.system(Mood{}), "This run's route") || !strings.Contains(n.system(Mood{}), "underdog") {
		t.Errorf("rock bottom route: %+v, %q", n.player, n.system(Mood{}))
	}
	if s := runStartText(*n.player); !strings.Contains(s, "the rock bottom route") {
		t.Errorf("run start %q", s)
	}
}

// A notable event fills a silence longer than FillAfter; it never displaces a waiting moment, and
// a pause of the autopilot is not filler.
func TestNarratorFillsSilence(t *testing.T) {
	now := time.Now()
	n := &Narrator{FillAfter: 30 * time.Second}
	n.defaults()
	n.now = func() time.Time { return now }
	n.quietAt = now.Add(-time.Minute)
	ch := make(chan Event, 4)
	ch <- Event{Seq: 1, Kind: KindAutopilot, Level: LevelNotable, At: now.UnixMilli(), Text: "Autopilot paused."}
	ch <- Event{Seq: 2, Kind: KindTrade, Level: LevelNotable, At: now.UnixMilli(), Text: "Sold a lamp for 180."}
	ch <- Event{Seq: 3, Kind: KindDay, Level: LevelNotable, At: now.UnixMilli(), Text: "Day 12 closes."}
	close(ch)
	n.intake(context.Background(), ch)
	if ev := n.take(); ev == nil || ev.Seq != 2 {
		t.Fatalf("filler = %+v", ev)
	}
	n.setBusy(false, 0)
	ch = make(chan Event, 2)
	ch <- Event{Seq: 4, Kind: KindTrade, Level: LevelNotable, At: now.UnixMilli(), Text: "Sold a kettle for 160."}
	close(ch)
	n.intake(context.Background(), ch)
	if ev := n.take(); ev != nil {
		t.Errorf("the voice just stopped; no filler yet: %+v", ev)
	}
}

func TestNarratorDoesNotFillAtStart(t *testing.T) {
	n := &Narrator{FillAfter: 30 * time.Second}
	n.defaults()
	ch := make(chan Event, 1)
	ch <- Event{Seq: 1, Kind: KindDay, Level: LevelNotable, At: time.Now().UnixMilli(), Text: "Day 9 closes."}
	close(ch)
	n.intake(context.Background(), ch)
	if ev := n.take(); ev != nil {
		t.Errorf("a narrator that just started filled with %+v (a restart's catch-up is not news)", ev)
	}
}
