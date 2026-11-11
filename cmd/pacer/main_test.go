package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPaceHolds(t *testing.T) {
	clock := time.Unix(0, 0)
	var slept []time.Duration
	p := &Pace{Day: 40 * time.Second, Client: 3 * time.Second, Action: 100 * time.Millisecond,
		Now:   func() time.Time { return clock },
		Sleep: func(d time.Duration) { slept = append(slept, d); clock = clock.Add(d) }}
	var got []string
	h := p.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = append(got, r.URL.Path+" "+string(b))
	}))
	post := func(path, body string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)))
	}
	post("/v1/event", `{"kind":"shutter_opened","game_day":5}`)
	post("/v1/event", `{"kind":"client_arrived","game_day":5}`)
	post("/v1/decide", `{"kind":"client_arrived"}`)
	post("/v1/event", `{"kind":"human_action","game_day":5}`)
	clock = clock.Add(10 * time.Second)
	post("/v1/event", `{"kind":"going_sleep","game_day":5}`)

	want := []time.Duration{3 * time.Second, 100 * time.Millisecond, 40*time.Second - 3*time.Second - 100*time.Millisecond - 10*time.Second}
	if len(slept) != len(want) {
		t.Fatalf("slept %v, want %v", slept, want)
	}
	for i := range want {
		if slept[i] != want[i] {
			t.Errorf("sleep %d = %v, want %v", i, slept[i], want[i])
		}
	}
	if len(got) != 5 || !strings.Contains(got[4], "going_sleep") {
		t.Errorf("forwarded %q: every request must reach the bridge with its body", got)
	}
}

func TestPaceLongDayIsNotHeld(t *testing.T) {
	clock := time.Unix(0, 0)
	var slept []time.Duration
	p := &Pace{Day: 30 * time.Second, Now: func() time.Time { return clock },
		Sleep: func(d time.Duration) { slept = append(slept, d) }}
	p.hold(eventHead{Kind: "shutter_opened", Day: 1})
	clock = clock.Add(90 * time.Second)
	p.hold(eventHead{Kind: "going_sleep", Day: 1})
	if len(slept) != 0 {
		t.Errorf("a day longer than -day slept %v more", slept)
	}
	p.hold(eventHead{Kind: "going_sleep", Day: 2})
	if len(slept) != 0 {
		t.Errorf("a day whose shutter never opened slept %v", slept)
	}
}
