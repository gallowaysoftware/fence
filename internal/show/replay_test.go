package show

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPlayFeedRecordedPace(t *testing.T) {
	feed := strings.Join([]string{
		`{"kind":"thought","day":1,"text":"a","at":1000000}`,
		`{"kind":"thought","day":1,"text":"b","at":1004000}`,
		// An hour's gap (a restart) is capped.
		`{"kind":"thought","day":1,"text":"c","at":4604000}`,
	}, "\n")
	var at []time.Time
	pace := Pace{Speed: 100, Recorded: true, MaxHold: 8 * time.Second}
	start := time.Now()
	err := PlayFeed(context.Background(), strings.NewReader(feed), Window{}, pace, nil, func(ev Event) Event {
		at = append(at, time.Now())
		return ev
	})
	if err != nil || len(at) != 3 {
		t.Fatalf("err %v, %d events", err, len(at))
	}
	// 4 s and the 8 s cap at speed 100: 40 ms, then 80 ms.
	if d := at[1].Sub(at[0]); d < 35*time.Millisecond || d > 200*time.Millisecond {
		t.Errorf("first hold %v, want ~40ms", d)
	}
	if d := at[2].Sub(at[1]); d < 75*time.Millisecond || d > 300*time.Millisecond {
		t.Errorf("capped hold %v, want ~80ms", d)
	}
	if d := at[0].Sub(start); d > 20*time.Millisecond {
		t.Errorf("first event waited %v", d)
	}
}

// A recorded show replays as it went out: its lines, the co-host's exchange included, in their
// places between the moments, pointing at the moments' new numbers; plain PlayFeed leaves them out.
func TestPlayFeedLines(t *testing.T) {
	feed := strings.Join([]string{
		`{"seq":40,"kind":"plan","level":2,"day":0,"beat":"route_choice","text":"Route: sprinter.","at":1000000}`,
		`{"seq":41,"kind":"say","day":0,"text":"Sprinter.","at":1001000,"say":{"line":"Sprinter.","for":40,"source":"llm","audio":"/audio/line-000040.wav"}}`,
		`{"seq":42,"kind":"say","day":0,"text":"Fast runs out.","at":1004000,"say":{"line":"Fast runs out.","for":40,"source":"llm","speaker":"cohost","name":"Rook"}}`,
		`{"seq":43,"kind":"say","day":0,"text":"Watch me.","at":1007000,"say":{"line":"Watch me.","for":40,"source":"llm","name":"Shark","reply_to":42}}`,
		`{"seq":44,"kind":"thought","day":1,"text":"a customer","at":1009000}`,
	}, "\n")
	cases := []struct {
		name  string
		play  func(publish func(Event) Event) error
		kinds string
	}{
		{"lines", func(p func(Event) Event) error {
			return PlayFeedLines(context.Background(), strings.NewReader(feed), Window{}, Pace{Speed: 1000, Recorded: true}, p)
		}, "plan say say say thought"},
		{"afresh", func(p func(Event) Event) error {
			return PlayFeed(context.Background(), strings.NewReader(feed), Window{}, Pace{Speed: 1000, Recorded: true}, nil, p)
		}, "plan thought"},
	}
	for _, tc := range cases {
		var got []Event
		err := tc.play(func(ev Event) Event {
			ev.Seq = int64(len(got) + 1)
			got = append(got, ev)
			return ev
		})
		var kinds []string
		for _, e := range got {
			kinds = append(kinds, string(e.Kind))
		}
		if err != nil || strings.Join(kinds, " ") != tc.kinds {
			t.Fatalf("%s: err %v, kinds %v", tc.name, err, kinds)
		}
		if tc.name != "lines" {
			continue
		}
		if s := got[1].Say; s.For != 1 || s.Audio != "/audio/line-000040.wav" {
			t.Errorf("narrator line %+v", s)
		}
		if s := got[2].Say; s.For != 1 || s.Speaker != SpeakerCohost || s.Name != "Rook" {
			t.Errorf("co-host line %+v", s)
		}
		if s := got[3].Say; s.ReplyTo != 3 || s.For != 1 {
			t.Errorf("answer %+v, want reply_to 3", s)
		}
	}
}
