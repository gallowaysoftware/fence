// Command fencetwitch is the stream's viewer participation (docs/SHOW.md, "Twitch"): it follows
// fenceshow's feed, puts a channel-points prediction on each of the strategist's bets and on the
// run's format verdict, runs a chat vote for the next morning's plan whose tally the bridge hands
// the strategist, and posts what the panel shows back to fenceshow. Only the dry run exists: it
// logs the Helix calls it would make and plays chat from a script. It never contacts Twitch.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
	"github.com/gallowaysoftware/fence/internal/twitch"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stderr); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(os.Stderr, "fencetwitch:", err)
		os.Exit(1)
	}
}

type config struct {
	show, log, script, bets, tally string
	live                           bool
	cfg                            twitch.Config
	runFor                         time.Duration
}

func parseFlags(args []string, getenv func(string) string, stderr io.Writer) (config, error) {
	env := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return def
	}
	var c config
	fs := flag.NewFlagSet("fencetwitch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.show, "show", env("FENCE_SHOW_URL", "http://127.0.0.1:7350"), "fenceshow's base URL: its /events feed is read and the panel's view posted to /v1/ingest")
	fs.BoolVar(&c.live, "live", false, "use the live Twitch client (not built yet: refuses to start)")
	fs.StringVar(&c.log, "dryrun-log", "var/twitch/dryrun.jsonl", "dry run: where the would-be Helix calls and the votes are logged (JSON lines)")
	fs.StringVar(&c.script, "chat-script", "", "dry run: the scripted chat (internal/twitch ScriptChat); empty: a silent chat")
	fs.StringVar(&c.bets, "synthetic-bets", "", "made-up cash/net-worth bets for a replay of a log without bets, settled by the feed's clock")
	fs.StringVar(&c.tally, "tally", env("FENCE_CHAT_TALLY", "var/twitch/chat-tally.json"), "the chat vote's tally for the bridge's -chat-tally; empty: not sent")
	fs.BoolVar(&c.cfg.Predictions, "predictions", true, "predictions on the bets and the verdict")
	fs.BoolVar(&c.cfg.Polls, "polls", true, "chat votes for the next morning's plan and at forks")
	fs.BoolVar(&c.cfg.Verdict, "verdict", true, "a prediction on the run's format verdict at its start; it holds Twitch's one slot until the run ends, so bets get none meanwhile")
	fs.DurationVar(&c.cfg.LockAfter, "lock-after", 2*time.Minute, "a prediction locks this long after it opens, or earlier when the feed could settle it (30s-30m)")
	fs.DurationVar(&c.cfg.PollMax, "poll-max", 4*time.Minute, "a chat vote closes after this long at most")
	fs.DurationVar(&c.cfg.PollMin, "poll-min", 30*time.Second, "a chat vote stays open at least this long, even when the day closes first")
	fs.IntVar(&c.cfg.PollEvery, "poll-every", 1, "a morning vote every N days")
	fs.IntVar(&c.cfg.MinVotes, "min-votes", 3, "a vote with fewer viewers is not sent to the strategist")
	fs.DurationVar(&c.runFor, "for", 0, "stop after this long (0: until interrupted)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected arguments %v", fs.Args())
	}
	if c.cfg.LockAfter < 30*time.Second || c.cfg.LockAfter > twitch.MaxWindow {
		return c, fmt.Errorf("-lock-after %s: want 30s to 30m", c.cfg.LockAfter)
	}
	c.cfg.TallyPath = c.tally
	return c, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) error {
	c, err := parseFlags(args, getenv, stderr)
	if err != nil {
		return err
	}
	if c.live {
		if _, err := twitch.LiveConfigFromEnv(getenv); err != nil {
			return err
		}
		return twitch.ErrNotBuilt
	}
	if c.runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.runFor)
		defer cancel()
	}
	if err := os.MkdirAll(filepath.Dir(c.log), 0o755); err != nil {
		return err
	}
	logf, err := os.OpenFile(c.log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer logf.Close()
	dry := twitch.NewDryRun(logf, nil)

	o := twitch.Options{Config: c.cfg, Predictions: dry, Back: dry.Back, Log: dry.Note, Publish: ingest(ctx, c.show, stderr)}
	if c.script != "" {
		f, err := os.Open(c.script)
		if err != nil {
			return err
		}
		sc, err := twitch.ParseScript(f)
		f.Close()
		if err != nil {
			return err
		}
		o.Chat, o.Triggers = sc, sc
	}
	if c.bets != "" {
		f, err := os.Open(c.bets)
		if err != nil {
			return err
		}
		o.Synthetic, err = twitch.ParseSynthetic(f)
		f.Close()
		if err != nil {
			return err
		}
	}
	ctl := twitch.NewController(o)
	fmt.Fprintf(stderr, "fencetwitch: dry run against %s; would-be Twitch calls in %s\n", c.show, c.log)

	type item struct {
		state *twitch.FeedState
		ev    *twitch.FeedEvent
	}
	feed := make(chan item, 256)
	go func() {
		_ = twitch.Subscribe(ctx, c.show,
			func(s twitch.FeedState) { feed <- item{state: &s} },
			func(e twitch.FeedEvent) { feed <- item{ev: &e} },
			func(err error) { fmt.Fprintln(stderr, "fencetwitch: feed:", err) })
	}()
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			// The refunds must go out after an interrupt too.
			ctl.Close(context.WithoutCancel(ctx), time.Now())
			return ctx.Err()
		case it := <-feed:
			if it.state != nil {
				ctl.Attach(ctx, *it.state, time.Now())
			} else {
				ctl.Feed(ctx, *it.ev, time.Now())
			}
		case now := <-tick.C:
			ctl.Tick(ctx, now)
		}
	}
}

// ingest posts the panel's view to fenceshow; a failure is reported, never fatal.
func ingest(ctx context.Context, base string, stderr io.Writer) func(show.Event) {
	url := strings.TrimRight(base, "/") + "/v1/ingest"
	return func(ev show.Event) {
		b, err := json.Marshal(ev)
		if err != nil {
			return
		}
		req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		cl := http.Client{Timeout: 3 * time.Second}
		resp, err := cl.Do(req)
		if err != nil {
			fmt.Fprintln(stderr, "fencetwitch: ingest:", err)
			return
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fmt.Fprintln(stderr, "fencetwitch: ingest:", resp.Status)
		}
	}
}
