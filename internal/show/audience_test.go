package show

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHubKeepsTheAudience(t *testing.T) {
	h := NewHub(nil, "")
	srv := httptest.NewServer(Handler(h, ""))
	defer srv.Close()
	body := `{"kind":"audience","day":33,"text":"Chat vote (open)","audience":{"poll":{"question":"Hold or dump?","for":"day 34's plan","status":"open","options":[{"word":"hold","label":"hold","votes":3}],"total":3}}}`
	resp, err := http.Post(srv.URL+"/v1/ingest", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	s := h.State()
	if s.Audience == nil || s.Audience.Audience.Poll.Total != 3 {
		t.Fatalf("audience %+v", s.Audience)
	}
	for _, e := range append(s.Ticker, s.Moments...) {
		if e.Kind == KindAudience {
			t.Errorf("the audience reached the ticker or moments: %+v", e)
		}
	}
	h.Publish(Event{Kind: KindRunStart, Run: "p:r2", Day: 1, Text: "New run.", Player: &Player{ID: "p"}})
	if h.State().Audience != nil {
		t.Error("a new run kept the last run's audience")
	}
	resp, err = http.Get(srv.URL + "/audience.js")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("audience.js: %v %v", err, resp.Status)
	}
	resp.Body.Close()
}
