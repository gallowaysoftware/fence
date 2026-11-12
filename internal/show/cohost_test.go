package show

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// booth answers as the narrator, the co-host or the narrator answering the co-host, by prompt.
type booth struct {
	mu                     sync.Mutex
	narrator, cohost, back []string
	asked                  []string
}

func (b *booth) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	pick := func(ls *[]string) string {
		l := (*ls)[0]
		if len(*ls) > 1 {
			*ls = (*ls)[1:]
		}
		return l
	}
	switch {
	case strings.Contains(r.System, "co-host in the booth"):
		b.asked = append(b.asked, "cohost")
		return llm.Response{Text: pick(&b.cohost)}, nil
	case strings.Contains(r.Messages[0].Content, "came back at you"):
		b.asked = append(b.asked, "back")
		return llm.Response{Text: pick(&b.back)}, nil
	}
	b.asked = append(b.asked, "narrator")
	return llm.Response{Text: pick(&b.narrator)}, nil
}

// numbered narrates evs, numbering what it publishes as the hub would.
func numbered(t *testing.T, n *Narrator, evs ...Event) []Event {
	t.Helper()
	var mu sync.Mutex
	var said []Event
	seq := int64(1000)
	n.Publish = func(e Event) Event {
		mu.Lock()
		defer mu.Unlock()
		seq++
		e.Seq = seq
		said = append(said, e)
		return e
	}
	n.AudioDir = t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan Event, len(evs))
	done := make(chan struct{})
	go func() { n.Run(ctx, ch); close(done) }()
	for _, e := range evs {
		ch <- e
		n.WaitIdle(ctx, e.Seq, 5*time.Second)
	}
	cancel()
	close(ch)
	<-done
	mu.Lock()
	defer mu.Unlock()
	return said
}

func TestCohostCue(t *testing.T) {
	cases := []struct {
		name string
		ev   Event
		want bool
	}{
		{"route choice", Event{Kind: KindPlan, Beat: BeatRouteChoice}, true},
		{"fork", Event{Kind: KindCandidates, Beat: BeatFork}, true},
		{"bet made", Event{Kind: KindBet, Beat: BeatBetMade}, true},
		{"bet lost", Event{Kind: KindBet, Beat: BeatBetLost}, true},
		{"change of course", Event{Kind: KindPlan, Plan: &Plan{Change: true, Changes: []string{"mortgage: sign the offer"}}}, true},
		{"reshuffled shopping", Event{Kind: KindPlan, Plan: &Plan{Change: true, Changes: []string{"no longer: buy lamps"}}}, false},
		{"behind the book", Event{Kind: KindGhost, Beat: BeatPace, Ghost: &Ghost{Status: PaceBehind}}, true},
		{"ahead of the book", Event{Kind: KindGhost, Beat: BeatPace, Ghost: &Ghost{Status: PaceAhead}}, false},
		{"sold at a loss", Event{Kind: KindTrade, Trade: &Trade{Side: "sell", HasProfit: true, Profit: -40}}, true},
		{"sold at a profit", Event{Kind: KindTrade, Trade: &Trade{Side: "sell", HasProfit: true, Profit: 40}}, false},
		{"a down day", Event{Kind: KindDay, Summary: &DaySummary{NetWorthDelta: -700}}, true},
		{"a counter story", Event{Kind: KindTrade, Beat: BeatCounter, Trade: &Trade{Side: "sell"}}, false},
		{"the night", Event{Kind: KindThought, Beat: BeatNight}, false},
	}
	for _, tc := range cases {
		if got := cohostCue(tc.ev) != ""; got != tc.want {
			t.Errorf("%s: reacts %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCohostVoice(t *testing.T) {
	for narrator, want := range map[string]string{"am_fenrir": "bf_alice", "bm_george": "bf_alice", "af_bella": "bm_daniel", "bf_emma": "bm_daniel"} {
		if got := CohostVoice(narrator); got != want || got == narrator {
			t.Errorf("CohostVoice(%s) = %s, want %s", narrator, got, want)
		}
	}
	for id, v := range personaVoices {
		if c := CohostVoice(v.voice); c == v.voice {
			t.Errorf("%s: the co-host speaks in the narrator's voice %s", id, c)
		}
	}
}

// The day-0 thesis gets an exchange at once: the narrator's line, the co-host's in its own voice,
// and the narrator's answer, numbered so the answer points at the jab. The next decision waits
// for Every narrator lines; the second exchange goes unanswered (ReplyEvery 2).
func TestCohostExchange(t *testing.T) {
	now := time.Now().UnixMilli()
	b := &booth{
		narrator: []string{"Sprinter it is. Fast money, no detours.", "Rent paid, shelves full.", "Signing the mortgage today.", "Another quiet sale.", "The crate went cheap.", "Quiet hands, full shelves.", "New plan: the deed."},
		cohost:   []string{"Fast money has a way of running out fast.", "A mortgage now? Bold, for someone renting a shoebox."},
		back:     []string{"Watch me."},
	}
	n := &Narrator{LLM: b, TTS: fakeTTS{}, PersonaD: "../../docs/personas", Persona: "shark", MinGap: time.Millisecond,
		Cohost: &Cohost{LLM: b, Every: 3, ReplyEvery: 2, Gap: time.Millisecond}}
	choice := Event{Seq: 1, Day: 0, Kind: KindPlan, Level: LevelMoment, Beat: BeatRouteChoice, At: now, Text: "Route: sprinter. Thesis: fast money."}
	pivot := Event{Seq: 3, Day: 3, Kind: KindPlan, Level: LevelMoment, At: now, Text: "Day 3 plan: sign the mortgage.", Plan: &Plan{Change: true, Changes: []string{"mortgage: sign the offer"}}}
	routine := func(seq int64) Event {
		return Event{Seq: seq, Day: 4, Kind: KindTrade, Level: LevelMoment, Beat: BeatCounter, At: now, Text: "Sold a lamp for 90.", Trade: &Trade{Side: "sell", Cash: 90}}
	}
	pivot2 := pivot
	pivot2.Seq = 7
	said := numbered(t, n, choice, routine(2), pivot, routine(4), routine(5), routine(6), pivot2)

	var got []string
	for _, e := range said {
		s := e.Say
		tag := "narrator"
		switch {
		case s.Speaker == SpeakerCohost:
			tag = "cohost"
			if s.Name != DefaultCohostName || s.Voice != "bf_alice" || !strings.Contains(s.Audio, "cohost-") || s.Check != "pass" {
				t.Errorf("co-host line %+v", s)
			}
		case s.ReplyTo != 0:
			tag = "reply"
			if s.ReplyTo != said[1].Seq || s.Name != "Shark" || s.Voice != "am_fenrir" || s.For != choice.Seq {
				t.Errorf("answer %+v, want it to answer seq %d as the Shark", s, said[1].Seq)
			}
		}
		got = append(got, tag)
	}
	want := "narrator cohost reply narrator narrator narrator narrator narrator narrator cohost"
	if strings.Join(got, " ") != want {
		t.Errorf("said %v, want %s", got, want)
	}
	if said[1].Say.For != choice.Seq {
		t.Errorf("the co-host's line is for %d, want the thesis %d", said[1].Say.For, choice.Seq)
	}
}

// The co-host's numbers pass the fact check like the narrator's: a jab that gets one wrong twice is
// not spoken, and no caption stands in for it.
func TestCohostFactCheck(t *testing.T) {
	now := time.Now().UnixMilli()
	b := &booth{narrator: []string{"Sold it at a loss, live with it."}, cohost: []string{"Losing 900 credits on a lamp, impressive."}}
	n := &Narrator{LLM: b, TTS: fakeTTS{}, MinGap: time.Millisecond, Cohost: &Cohost{LLM: b, Gap: time.Millisecond}}
	loss := Event{Seq: 1, Day: 5, Kind: KindTrade, Level: LevelMoment, At: now, Text: "Sold a lamp for 60 (-40 on cost).", Trade: &Trade{Side: "sell", Cash: 60, HasProfit: true, Profit: -40}}
	said := numbered(t, n, loss)
	if len(said) != 1 || said[0].Say.Speaker != "" {
		t.Errorf("said %+v, want the narrator alone", said)
	}
	if strings.Count(strings.Join(b.asked, " "), "cohost") != 2 {
		t.Errorf("asked %v, want the co-host twice (the retry named the fault)", b.asked)
	}
	b = &booth{narrator: []string{"Sold it at a loss, live with it."}, cohost: []string{"Forty down on a lamp, impressive."}}
	n = &Narrator{LLM: b, TTS: fakeTTS{}, MinGap: time.Millisecond, Cohost: &Cohost{LLM: b, Gap: time.Millisecond}}
	if said := numbered(t, n, loss); len(said) != 2 || said[1].Say.Line != "Forty down on a lamp, impressive." {
		t.Errorf("said %+v, want the co-host's true jab", said)
	}
}

// A caption (the narrator's model failed or the gate dropped the line) gets no exchange: the drift
// slot is busy, and the co-host has nothing of the player's to answer.
func TestCohostNeedsTheNarratorsLine(t *testing.T) {
	b := &booth{narrator: []string{""}, cohost: []string{"Bold."}}
	n := &Narrator{LLM: b, TTS: fakeTTS{}, MinGap: time.Millisecond, Cohost: &Cohost{LLM: b, Gap: time.Millisecond}}
	said := numbered(t, n, Event{Seq: 1, Kind: KindPlan, Level: LevelMoment, Beat: BeatRouteChoice, At: time.Now().UnixMilli(), Text: "Route: sprinter."})
	if len(said) != 1 || said[0].Say.Source != "caption" {
		t.Errorf("said %+v", said)
	}
}

// A moment as loud as the decision takes the floor and the co-host stands down; a quieter one
// waits out the exchange.
func TestCohostStandsDown(t *testing.T) {
	cases := []struct {
		name    string
		pending *Event
		want    bool
	}{
		{"nothing waits", nil, true},
		{"a counter story waits", &Event{Kind: KindTrade, Beat: BeatCounter, Level: LevelMoment}, true},
		{"a bet settles", &Event{Kind: KindBet, Beat: BeatBetLost, Level: LevelMoment}, false},
		{"the run ends", &Event{Kind: KindRunEnd, Level: LevelMoment}, false},
	}
	for _, tc := range cases {
		n := &Narrator{}
		n.defaults()
		n.pending = tc.pending
		if got := n.waitTurn(context.Background(), 0, Priority(Event{Kind: KindPlan, Beat: BeatFork})); got != tc.want {
			t.Errorf("%s: floor free %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A restart reads each voice's lines back into its own phrasebook.
func TestRecallSeparatesTheCohost(t *testing.T) {
	feed := strings.Join([]string{
		`{"kind":"run_start","run":"r1","player":{"id":"p","persona":"shark"}}`,
		`{"kind":"say","run":"r1","say":{"line":"The till is singing tonight.","source":"llm"}}`,
		`{"kind":"say","run":"r1","say":{"line":"Fast money runs out fast.","source":"llm","speaker":"cohost","name":"Rook"}}`,
	}, "\n")
	n := &Narrator{Cohost: &Cohost{}}
	n.Recall(strings.NewReader(feed))
	if got := n.books["shark"].lines; len(got) != 1 || got[0] != "The till is singing tonight." {
		t.Errorf("narrator's book %v", got)
	}
	if got := n.Cohost.book.lines; len(got) != 1 || got[0] != "Fast money runs out fast." {
		t.Errorf("co-host's book %v", got)
	}
}
