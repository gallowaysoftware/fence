// Command fakemod plays the mod's role against a running bridge using the protocol
// fixtures: hello → event → decide → outcome.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/gallowaysoftware/fence/internal/fakemod"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fakemod:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fakemod", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bridge := fs.String("bridge", envOr("FENCE_BRIDGE", "http://127.0.0.1:7341"), "bridge base URL")
	fixtures := fs.String("fixtures", "protocol/fixtures", "directory holding the protocol fixtures")
	timeout := fs.Duration("timeout", time.Minute, "overall timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	_, err := fakemod.Run(ctx, fakemod.Config{BaseURL: *bridge, FixturesDir: *fixtures, Out: stdout})
	return err
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
