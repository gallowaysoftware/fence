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
