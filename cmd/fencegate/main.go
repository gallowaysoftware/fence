// Command fencegate lets the strategist and the narrator share one model slot (drift serves one
// request at a time). It listens on loopback and serialises their calls to -upstream, the
// strategist's first: point the bridge at http://127.0.0.1:7380/plan/v1 and fenceshow at
// http://127.0.0.1:7380/narrate/v1. See docs/STREAM-OPS.md, "The drift gate".
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/gate"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fencegate:", err)
		os.Exit(1)
	}
}

type config struct {
	listen, upstream string
	maxWait          [3]time.Duration
	retryFor         time.Duration
	logEvery         time.Duration
}

func parseFlags(args []string, getenv func(string) string, stderr io.Writer) (config, error) {
	env := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return def
	}
	var c config
	fs := flag.NewFlagSet("fencegate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.listen, "listen", env("FENCE_GATE_LISTEN", "127.0.0.1:7380"), "address to serve on; keep it on loopback")
	fs.StringVar(&c.upstream, "upstream", env("FENCE_GATE_UPSTREAM", ""), "the model server's OpenAI-compatible base URL (make gate passes DRIFT_URL from local.mk)")
	fs.DurationVar(&c.maxWait[gate.Plan], "max-wait-plan", gate.DefaultMaxWait[gate.Plan], "longest queue wait for the strategist; 0: as long as the bridge waits")
	fs.DurationVar(&c.maxWait[gate.Narrate], "max-wait-narrate", gate.DefaultMaxWait[gate.Narrate], "longest queue wait for a narrator line before it is dropped (the narrator speaks the caption)")
	fs.DurationVar(&c.maxWait[gate.Filler], "max-wait-filler", gate.DefaultMaxWait[gate.Filler], "longest queue wait for filler")
	fs.DurationVar(&c.retryFor, "retry-for", 20*time.Second, "keep retrying an upstream 503 (the slot busy with another caller) this long")
	fs.DurationVar(&c.logEvery, "log-every", 5*time.Minute, "log the per-role summary this often; 0: never")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if c.upstream == "" {
		return c, errors.New("-upstream (or FENCE_GATE_UPSTREAM) is required")
	}
	return c, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer) error {
	c, err := parseFlags(args, getenv, stderr)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	g, err := gate.New(gate.Config{Upstream: c.upstream, MaxWait: c.maxWait, RetryFor: c.retryFor, Log: log})
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", c.listen)
	if err != nil {
		return err
	}
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); !net.ParseIP(host).IsLoopback() {
		log.Warn("listening beyond loopback; the gate has no authentication", "addr", ln.Addr().String())
	}
	st := g.Status()
	log.Info("fencegate listening", "addr", ln.Addr().String(), "upstream_local", st.UpstreamLocal,
		"max_wait_plan", c.maxWait[gate.Plan], "max_wait_narrate", c.maxWait[gate.Narrate], "max_wait_filler", c.maxWait[gate.Filler])
	srv := &http.Server{Handler: g, ReadHeaderTimeout: 5 * time.Second}
	if c.logEvery > 0 {
		go func() {
			t := time.NewTicker(c.logEvery)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					for _, r := range []string{"plan", "narrate", "filler"} {
						s := g.Status().Roles[r]
						if s.Requests > 0 {
							log.Info("gate summary", "role", r, "requests", s.Requests, "served", s.Served, "dropped", s.Dropped,
								"cancelled", s.Cancelled, "errors", s.Errors, "retries", s.Retries,
								"wait_p50_ms", s.WaitMS.P50, "wait_p95_ms", s.WaitMS.P95, "upstream_p50_ms", s.UpstreamMS.P50, "upstream_p95_ms", s.UpstreamMS.P95)
						}
					}
				}
			}
		}()
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(sctx)
	case err := <-errc:
		return err
	}
}
