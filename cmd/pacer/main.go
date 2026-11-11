// Command pacer sits between the gym and a bridge and slows the gym to a live game's pace, so
// the show layer (fenceshow's live hook, the narrator, TTS, the panel) can be rehearsed at a
// stream's event rate without the game (docs/STREAM-OPS.md, "Dress rehearsal").
//
//	gym -bridge http://127.0.0.1:7342 ── pacer ── bridge :7341 ── fenceshow -live-log
//
// It holds each customer's arrival and each counter action for a moment and closes no day sooner
// than -day after the shutter opened. The morning before it is the strategist's (the bridge's
// -planner-wait), as live. Everything else passes through untouched.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "pacer:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("pacer", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "127.0.0.1:7342", "address the gym's -bridge points at")
	target := fs.String("bridge", "http://127.0.0.1:7341", "the bridge")
	var p Pace
	fs.DurationVar(&p.Day, "day", 45*time.Second, "shortest wall time of a game day's open hours, shutter up to sleep")
	fs.DurationVar(&p.Client, "client", 3*time.Second, "hold before each customer arrives")
	fs.DurationVar(&p.Action, "action", 100*time.Millisecond, "hold before each of the player's actions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	u, err := url.Parse(*target)
	if err != nil || u.Host == "" {
		return fmt.Errorf("-bridge %q: want http://host:port", *target)
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	p.Log = log
	srv := &http.Server{Addr: *listen, Handler: p.Handler(httputil.NewSingleHostReverseProxy(u))}
	go func() { <-ctx.Done(); srv.Close() }()
	log.Info("pacing", "listen", *listen, "day", p.Day, "client", p.Client, "action", p.Action)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Pace holds the gym's events to a live game's rhythm.
type Pace struct {
	Day, Client, Action time.Duration
	Log                 *slog.Logger
	// Sleep is time.Sleep unless a test replaces it.
	Sleep func(time.Duration)
	Now   func() time.Time

	mu       sync.Mutex
	dayStart time.Time
	day      int
}

type eventHead struct {
	Kind string `json:"kind"`
	Day  int    `json:"game_day"`
}

// Handler paces /v1/event requests by their kind and forwards every request to next.
func (p *Pace) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/event" {
			body, err := io.ReadAll(r.Body)
			r.Body.Close()
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			var ev eventHead
			if json.Unmarshal(body, &ev) == nil {
				p.hold(ev)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (p *Pace) hold(ev eventHead) {
	sleep, now := p.Sleep, p.Now
	if sleep == nil {
		sleep = time.Sleep
	}
	if now == nil {
		now = time.Now
	}
	switch ev.Kind {
	case "shutter_opened":
		p.mu.Lock()
		p.dayStart, p.day = now(), ev.Day
		p.mu.Unlock()
	case "client_arrived":
		sleep(p.Client)
	case "human_action":
		sleep(p.Action)
	case "going_sleep":
		p.mu.Lock()
		start, day := p.dayStart, p.day
		p.mu.Unlock()
		if start.IsZero() || day != ev.Day {
			return
		}
		took := now().Sub(start)
		if rest := p.Day - took; rest > 0 {
			sleep(rest)
		}
		if p.Log != nil {
			p.Log.Info("day", "day", day, "played", took.Round(time.Second), "held", max(p.Day, took).Round(time.Second))
		}
	}
}
