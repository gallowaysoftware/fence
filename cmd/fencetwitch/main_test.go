package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
	"github.com/gallowaysoftware/fence/internal/twitch"
)

func TestLiveRefuses(t *testing.T) {
	wd, _ := os.Getwd()
	outside := filepath.Join(t.TempDir(), "token")
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"no settings", nil, "set TWITCH_BROADCASTER_ID, TWITCH_CLIENT_ID, TWITCH_TOKEN_FILE"},
		{"a token in the tree", map[string]string{twitch.EnvClientID: "c", twitch.EnvBroadcaster: "b", twitch.EnvTokenFile: filepath.Join(wd, "token")}, "inside the working tree"},
		{"not built", map[string]string{twitch.EnvClientID: "c", twitch.EnvBroadcaster: "b", twitch.EnvTokenFile: outside}, "not built yet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := run(context.Background(), []string{"-live"}, func(k string) string { return tc.env[k] }, io.Discard)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := parseFlags([]string{"-lock-after", "10s"}, func(string) string { return "" }, io.Discard); err == nil {
		t.Error("a 10 s window was accepted")
	}
}

// TestDryRunAgainstAShow runs the command against a feed carrying a bet as the bets branch
// publishes it (kind "bet" with a `bet` payload): the bet becomes a prediction on the panel and a
// would-be POST in the dry-run log, and stopping the command refunds it.
func TestDryRunAgainstAShow(t *testing.T) {
	hub := show.NewHub(nil, "")
	feed := []string{
		`{"seq":1,"kind":"run_start","run":"p:r1","day":1,"level":2,"text":"New run.","player":{"id":"p","persona_name":"Shark"}}`,
		`{"seq":2,"kind":"bet","run":"p:r1","day":3,"level":2,"text":"The bet, due day 6: Cash past 900 by day 6.","bet":{"claim":"Cash past 900 by day 6","kind":"cash","target":900,"by":6,"staked":3,"outcome":"staked"}}`,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: state\ndata: {\"day\":0}\n\n")
		for _, l := range feed {
			fmt.Fprintf(w, "data: %s\n\n", l)
		}
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	mux.HandleFunc("POST /v1/ingest", hub.ServeIngest)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "dry.jsonl")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"-show", srv.URL, "-dryrun-log", logPath, "-tally", filepath.Join(dir, "tally.json")}, func(string) string { return "" }, io.Discard)
	}()
	var state show.State
	for range 100 {
		time.Sleep(100 * time.Millisecond)
		if state = hub.State(); state.Audience != nil {
			break
		}
	}
	cancel()
	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if state.Audience == nil || state.Audience.Audience.Prediction == nil || state.Audience.Audience.Prediction.Title != "Cash past 900 by day 6?" {
		t.Fatalf("panel state %+v", state.Audience)
	}
	log, _ := os.ReadFile(logPath)
	if !strings.Contains(string(log), `"call":"POST /helix/predictions"`) || !strings.Contains(string(log), `"status":"CANCELED"`) {
		t.Errorf("dry-run log:\n%s", log)
	}
}
