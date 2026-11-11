package show

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHubStateAndBoard(t *testing.T) {
	board := filepath.Join(t.TempDir(), "board.json")
	var rec bytes.Buffer
	h := NewHub(&rec, board)
	h.SeedBoard([]BoardRow{{Run: "old", Player: "p0", Ending: "buyout", Won: true, Days: 190}})
	h.Publish(Event{Kind: KindRunStart, Run: "p1:r1", Day: 1, Level: LevelMoment, Text: "New run.", Player: &Player{ID: "p1", Model: "m"}})
	h.Publish(Event{Kind: KindPlan, Day: 1, Level: LevelMoment, Text: "plan", Plan: &Plan{Day: 1}})
	for i := range 12 {
		h.Publish(Event{Kind: KindThought, Day: 1, Text: "t" + string(rune('a'+i))})
	}
	h.Publish(Event{Kind: KindDay, Day: 1, Level: LevelNotable, Text: "day", Clock: &Clock{Cash: 10, NetWorth: 900}})
	h.Publish(Event{Kind: KindRunEnd, Day: 3, Level: LevelMoment, Text: "end", End: &End{Ending: "rent", Day: 3}})

	s := h.State()
	if s.Plan == nil || s.Clock.NetWorth != 900 || len(s.Ticker) != tickerKeep || s.Ticker[tickerKeep-1].Text != "tl" || len(s.NetWorth) != 1 {
		t.Fatalf("state = %+v", s)
	}
	if len(s.Board) != 2 || s.Board[1].Ending != "rent" || s.Board[1].Live || s.Board[1].NetWorth != 900 {
		t.Fatalf("board = %+v", s.Board)
	}
	if got := NewHub(nil, board).State().Board; len(got) != 2 {
		t.Errorf("board not kept across hubs: %+v", got)
	}
	if n := strings.Count(rec.String(), "\n"); n != 16 {
		t.Errorf("recorded %d lines", n)
	}
}

func TestServeSSEAndIngest(t *testing.T) {
	h := NewHub(nil, "")
	h.Publish(Event{Kind: KindRunStart, Run: "r", Day: 1, Text: "New run.", Player: &Player{ID: "p"}})
	srv := httptest.NewServer(Handler(h, t.TempDir()))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	r := bufio.NewReader(resp.Body)
	next := func() string {
		for {
			l, err := r.ReadString('\n')
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(l, "data: ") {
				return strings.TrimPrefix(strings.TrimSpace(l), "data: ")
			}
		}
	}
	var st State
	if err := json.Unmarshal([]byte(next()), &st); err != nil || st.Player == nil || st.Player.ID != "p" {
		t.Fatalf("state %+v %v", st, err)
	}

	body := `{"kind":"candidates","day":4,"level":2,"text":"Three plans simulated; the cautious one wins.","candidates":{"day":4,"metric":"days survived","options":[{"label":"cautious","score":212.5,"rollouts":24,"chosen":true},{"label":"bold","score":201}]}}`
	pr, err := http.Post(srv.URL+"/v1/ingest", "application/json", strings.NewReader(body))
	if err != nil || pr.StatusCode != http.StatusOK {
		t.Fatalf("ingest: %v %v", pr.Status, err)
	}
	var got Event
	if err := json.Unmarshal([]byte(next()), &got); err != nil || got.Kind != KindCandidates || got.Seq != 2 || len(got.Candidates.Options) != 2 {
		t.Fatalf("event %+v %v", got, err)
	}
	if bad, _ := http.Post(srv.URL+"/v1/ingest", "application/json", strings.NewReader(`{"kind":"trade"}`)); bad.StatusCode != http.StatusBadRequest {
		t.Errorf("an event without text was accepted: %s", bad.Status)
	}
	if a, _ := http.Get(srv.URL + "/audio/../board.json"); a.StatusCode != http.StatusNotFound {
		t.Errorf("audio path escape: %s", a.Status)
	}
}

func TestEachEventWindow(t *testing.T) {
	var log bytes.Buffer
	for _, l := range []map[string]any{
		ev("run_start", 1, map[string]any{"run_id": "r"}),
		snapshot("morning", 1, 100, 500, map[string]any{"visible": false}),
		ev("money", 1, map[string]any{"kind": "day_ended", "day": 1, "net_worth": 500, "credits": 100}),
		snapshot("morning", 2, 100, 500, map[string]any{"visible": true, "evidence": 30}),
		ev("money", 2, map[string]any{"kind": "day_ended", "day": 2, "net_worth": 600, "credits": 200}),
		snapshot("morning", 3, 200, 600, map[string]any{"visible": true, "evidence": 30}),
		ev("money", 3, map[string]any{"kind": "rent_paid", "amount": 50}),
		ev("money", 3, map[string]any{"kind": "day_ended", "day": 3, "net_worth": 650, "credits": 150}),
		snapshot("morning", 4, 150, 650, map[string]any{"visible": true, "evidence": 30}),
	} {
		b, _ := json.Marshal(l)
		log.Write(append(b, '\n'))
	}
	var got []string
	err := EachEvent(&log, NewBuilder(Player{ID: "p"}), Window{From: 3, To: 3}, func(e Event) error {
		got = append(got, string(e.Kind)+":"+strings.Fields(e.Text)[0])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "run_start:New heat:Security day:Day thought:Day money:Paid day:Day"
	if strings.Join(got, " ") != want {
		t.Errorf("window events %q\nwant %q", strings.Join(got, " "), want)
	}
}

func TestHubContinuesARecording(t *testing.T) {
	rec := "{\"seq\":41,\"kind\":\"day\"}\n{\"seq\":42,\"kind\":\"say\"}\n"
	h := NewHub(nil, "")
	defer h.Close()
	h.ContinueFrom(LastSeq(strings.NewReader(rec)))
	if ev := h.Publish(Event{Kind: KindThought, Text: "x"}); ev.Seq != 43 {
		t.Errorf("seq after a restart = %d, want 43", ev.Seq)
	}
}
