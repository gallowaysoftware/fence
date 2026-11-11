package show

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

// Lines from the dress rehearsal: the Shark's habits within minutes of each other.
func TestPhrasebookRepeats(t *testing.T) {
	pb := newPhrasebook("shark")
	for _, l := range []string{
		"Four thousand in the till. Rent is just a whisper.",
		"Another day, another bag full. Rent is just a scratch on my knee.",
		"Thirteen cans and some stamps. That is blood in the water.",
	} {
		pb.add(l)
	}
	cases := []struct {
		line, news, want string
	}{
		{"Five thousand deep. The whisper of rent fades.", "", `leans on "a whisper"`},
		{"Sold the perfume for 500, and it went in the till.", "", `leans on "the till"`},
		{"Easy, another bag full today.", "", `repeats "another bag full"`},
		{"Then hitting the counter early.", "", ""},
		{"Another day of stamps and cans.", "", `opens with "another day"`},
		{"Rent is due tomorrow. Plenty in the drawer.", "", ""},
		{"That aug smelled of blood and cheap cologne.", "", `leans on "blood in the water"`},
		{"The board promotes me. Master Appraiser at last.", "", ""},
		// A phrase of the moment's own words is news, however often it comes up.
		{"Off to the dumping ground with a crowbar.", "Out tonight: dumping ground.", ""},
		{"A blood bag for the nurse, gone.", "Sold Blood Bag to Mid-Level Nurse for 120.", ""},
	}
	for _, c := range cases {
		fs := pb.repeats(c.line, c.news)
		got := ""
		if len(fs) > 0 {
			got = string(fs[0])
		}
		if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
			t.Errorf("repeats(%q) = %q, want %q", c.line, got, c.want)
		}
	}
	// The dumping ground twice is still news when the second moment names it.
	pb.add("Off to the dumping ground with a crowbar.")
	if fs := pb.repeats("Back to the dumping ground, crowbar in hand.", "Night at the dumping ground."); len(fs) != 0 {
		t.Errorf("news flagged: %v", fs)
	}
}

func TestPhrasebookForgets(t *testing.T) {
	pb := newPhrasebook("shark")
	pb.add("Cash is blood in the water.")
	for i := range motifRest {
		pb.add(strings.Repeat("x", i+1) + " filler words go here")
	}
	if fs := pb.repeats("Blood, and plenty of it.", ""); len(fs) != 0 {
		t.Errorf("an image rested %d lines is still flagged: %v", motifRest, fs)
	}
	if fs := pb.repeats("Smell that blood in the water.", ""); len(fs) == 0 {
		t.Error("a four-word phrase of 11 lines ago passed")
	}
	for i := range gram4Keep {
		pb.add(strings.Repeat("y", i+1) + " filler words go here")
	}
	if fs := pb.repeats("Smell that blood in the water.", ""); len(fs) != 0 {
		t.Errorf("a phrase %d lines old is still flagged: %v", gram4Keep, fs)
	}
}

func TestPhrasebookAvoidAndImages(t *testing.T) {
	pb := newPhrasebook("shark")
	pb.add("Four hundred in the till. The marks keep coming.")
	pb.add("Three hundred in the till, rent is a joke.")
	av := pb.avoid()
	if len(av) == 0 || av[0] != "the till" || !slices.Contains(av, "opening with three hundred") {
		t.Errorf("avoid = %q", av)
	}
	seen := map[string]bool{}
	for range 4 {
		im := pb.image("shark", "steady")
		if im == "" || seen[im] {
			t.Fatalf("image rotation repeated or ran dry: %q after %v", im, seen)
		}
		seen[im] = true
	}
	pb.add("Bait on the hook, waiting.")
	for range 10 {
		if im := pb.image("shark", "steady"); strings.Contains(im, "bait") {
			t.Errorf("offered an image just used: %q", im)
		}
	}
	if pb.image("nobody", "steady") != "" {
		t.Error("a persona without a bank got an image")
	}
}

// seqLLM answers each request with the next reply, then the last again.
type seqLLM struct {
	mu      sync.Mutex
	replies []string
	asked   []string
}

func (f *seqLLM) Complete(_ context.Context, r llm.Request) (llm.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, r.Messages[0].Content)
	i := min(len(f.asked), len(f.replies)) - 1
	return llm.Response{Text: f.replies[i]}, nil
}

func TestNarratorRejectsRepeatsTwice(t *testing.T) {
	at := time.Now().UnixMilli()
	f := &seqLLM{replies: []string{"Five hundred in the till.", "Cash in the till, again.", "A fresh line about the haul."}}
	n := &Narrator{LLM: f, Persona: "shark"}
	n.defaults()
	n.book().add("Four hundred in the till.")
	said := narrate(t, n, Event{Seq: 1, At: at, Level: LevelMoment, Kind: KindTrade, Text: "Sold a lamp for 500.", Trade: &Trade{Side: "sell", Cash: 500}})
	if len(said) != 1 || said[0].Say.Source != "caption" || said[0].Say.Check != "fallback" {
		t.Fatalf("a retry that repeats was spoken: %+v", said[0].Say)
	}
	if !strings.Contains(f.asked[0], "Words to rest") || !strings.Contains(f.asked[0], "the till") {
		t.Errorf("prompt has no list of phrases to rest: %q", f.asked[0])
	}
}

func TestNarratorRecallsAcrossRestart(t *testing.T) {
	rec := strings.Join([]string{
		`{"kind":"run_start","run":"p:r0","player":{"id":"p","persona":"magnate"}}`,
		`{"kind":"say","run":"p:r0","say":{"line":"Old run, old till.","source":"llm"}}`,
		`{"kind":"run_start","run":"p:r1","player":{"id":"p","persona":"shark"}}`,
		`{"kind":"say","run":"p:r1","say":{"line":"Four hundred in the till.","source":"llm"}}`,
		`{"kind":"say","run":"p:r1","say":{"line":"Day 3 plan: sell.","source":"caption"}}`,
	}, "\n")
	n := &Narrator{Persona: "magnate"}
	n.Recall(strings.NewReader(rec))
	n.player = &Player{Persona: "shark"}
	if fs := n.book().repeats("Cash in the till.", ""); len(fs) == 0 {
		t.Error("a restarted narrator forgot the run's lines")
	}
	// The restart replays the same run's start: the book stands. A new run starts afresh.
	n.defaults()
	ch := make(chan Event, 2)
	ch <- Event{Kind: KindRunStart, Run: "p:r1", Level: LevelNotable, Player: &Player{Persona: "shark"}}
	close(ch)
	n.intake(context.Background(), ch)
	if fs := n.book().repeats("Cash in the till.", ""); len(fs) == 0 {
		t.Error("the replayed run start wiped the phrasebook")
	}
	ch = make(chan Event, 1)
	ch <- Event{Kind: KindRunStart, Run: "p:r2", Level: LevelNotable, Player: &Player{Persona: "shark"}}
	close(ch)
	n.intake(context.Background(), ch)
	if fs := n.book().repeats("Cash in the till.", ""); len(fs) != 0 {
		t.Errorf("a new run kept the last one's phrasebook: %v", fs)
	}
}

// From the second variety replay: phrases across a full stop or a number, and "I let him".
func TestColourGramsStayInClause(t *testing.T) {
	got := colourGrams("Forty-five clean marks in a row. The Board signs, I let him pay 175 credits gladly.", 3, nil)
	for _, g := range got {
		if strings.Contains(g, "row the") || strings.Contains(g, "pay credits") || g == "i let him" {
			t.Errorf("gram %q", g)
		}
	}
	if !slices.Contains(got, "forty-five clean marks") && !slices.Contains(got, "five clean marks") {
		t.Errorf("grams = %q", got)
	}
}
