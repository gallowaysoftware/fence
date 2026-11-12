// Command fencechoose makes a stream run's day-0 route choice (docs/STREAM.md, "Model agency"):
// the strategist picks a route from the menu or composes one, with the stream's previous choices
// in the prompt and the last one off the menu. It records the choice and prints what the mod, the
// bridge and fenceshow are given (docs/STREAM-OPS.md, "Starting a run").
//
//	fencechoose [-format F] [-start S] [-lean] [-llm-url URL -model M] [-dir var/stream/choices] [-dry-run]
//
// A choice that is not played must not count as the last run: delete its directory.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/agency"
	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/route"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

const defaultDir = "var/stream/choices"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], time.Now, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fencechoose:", err)
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func run(ctx context.Context, args []string, now func() time.Time, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fencechoose", flag.ContinueOnError)
	fs.SetOutput(stderr)
	formatName := fs.String("format", "", "the run's stream format (a hard one that names no route): the menu shows each route's odds of its deadline")
	start := fs.String("start", "", "the start the run fixes; only routes that play it are offered")
	persona := fs.String("persona", "", "the scenario card's archetype (a docs/personas id): it writes the thesis and plays the run whatever route is chosen; empty: the chosen route's persona")
	lean := fs.Bool("lean", false, "the lean prompt (one line a route), for drift's MLX, which refuses the full one for memory")
	url := fs.String("llm-url", env("GATE_URL", "http://127.0.0.1:7380")+"/plan/v1", "the strategist's OpenAI-compatible endpoint (default: drift through the gate's plan lane)")
	model := fs.String("model", env("DRIFT_MODEL", "qwen38-27b-mlx"), "the strategist's model")
	temperature := fs.Float64("temperature", 0.2, "sampling temperature (negative: the server's default)")
	thinking := fs.String("thinking", "off", "chat_template_kwargs.enable_thinking: on, off, or empty to send none")
	noSchema := fs.Bool("no-schema", true, "never send response_format (drift's MLX rejects it)")
	timeout := fs.Duration("timeout", 5*time.Minute, "longest wait for the answer, its repair round included")
	dir := fs.String("dir", defaultDir, "the stream's choices, one directory a run; the newest is the route off the menu")
	dryRun := fs.Bool("dry-run", false, "ask and print, but record nothing")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %q", fs.Args())
	}

	menu, err := agency.LoadMenu("")
	if err != nil {
		return err
	}
	if *start != "" && len(menu.For(*start).Entries) == 0 {
		return fmt.Errorf("-start %s: no route plays it; starts: %v", *start, menu.Starts())
	}
	ask := agency.Ask{FixedStart: *start, Lean: *lean, Recent: agency.Recent(*dir, "")}
	if *persona != "" {
		text, err := strategy.LoadPersona(strategy.DefaultPersonaDir, *persona)
		if err != nil {
			return err
		}
		ask.Persona, ask.PersonaText = *persona, text
	}
	if n := len(ask.Recent); n > 0 {
		ask.Exclude = ask.Recent[n-1]
	}
	var f *format.Format
	if *formatName != "" {
		ff, err := format.Load("", *formatName)
		if err != nil {
			return err
		}
		if ff.Route != "" {
			return fmt.Errorf("format %s plays the %s route: there is nothing to choose", ff.Name, ff.Route)
		}
		if ff.Difficulty != "hard" {
			return fmt.Errorf("format %s is %s; the menu is the hard routes", ff.Name, ff.Difficulty)
		}
		f, ask.Format = &ff, &ff
	}

	opts := llm.OpenAIOptions{NoSchema: *noSchema}
	switch *thinking {
	case "":
	case "on", "off":
		on := *thinking == "on"
		opts.Thinking = &on
	default:
		return fmt.Errorf("-thinking %q: want on, off or empty", *thinking)
	}
	if *temperature >= 0 {
		opts.Temperature = temperature
	}
	client := &llm.OpenAI{BaseURL: *url, Model: *model, OpenAIOptions: opts}
	cctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	ch := agency.Choose(cctx, client, menu, ask)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if ch.Raw == "" {
		// Nothing answered: a fallback here would put the default route on stream as the model's.
		return fmt.Errorf("no answer from %s (%s): %s", *url, *model, strings.Join(ch.Errors, "; "))
	}

	routeBytes, err := ch.RouteFile(menu)
	if err != nil {
		return err
	}
	runDir := filepath.Join(*dir, now().UTC().Format("20060102T150405Z"))
	// A composed route is written under its own name, since route.Parse names a route by its file
	// and that name goes on the name card; in a subdirectory, so no name can overwrite choice.json.
	routeRef, routePath := ch.Route, ""
	if ch.Composed {
		routePath = filepath.Join(runDir, "route", ch.Route+".json")
		routeRef = routePath
	} else {
		e, _ := menu.Get(ch.Route)
		routePath = e.Path
	}
	directives := routeBytes
	if f != nil {
		if directives, err = f.Apply(routeBytes); err != nil {
			return err
		}
	}
	line, err := route.StripMeta(directives)
	if err != nil {
		return err
	}

	materialized := ""
	if !*dryRun {
		if err := os.MkdirAll(runDir, 0o755); err != nil {
			return err
		}
		if ch.Composed {
			if err := os.MkdirAll(filepath.Dir(routePath), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(routePath, append(routeBytes, '\n'), 0o644); err != nil {
				return err
			}
		}
		if f != nil {
			if materialized, err = f.Materialize(routePath, filepath.Join(runDir, "format")); err != nil {
				return err
			}
		}
		b, err := json.MarshalIndent(ch, "", "  ")
		if err != nil {
			return err
		}
		// Written last: Recent counts a run by its choice.json, so a failed write above leaves none.
		if err := os.WriteFile(filepath.Join(runDir, agency.ChoiceFile), append(b, '\n'), 0o644); err != nil {
			return err
		}
	}
	report(stdout, ch, f, line, runDir, routeRef, materialized, *dryRun)
	return nil
}

func report(w io.Writer, ch agency.Choice, f *format.Format, line, runDir, routeRef, materialized string, dryRun bool) {
	kind := "menu route"
	if ch.Composed {
		kind = "composed: " + ch.Composition.Summary()
	}
	fmt.Fprintf(w, "route     %s (%s)\n", ch.Route, kind)
	fmt.Fprintf(w, "save      a new hard save, start %s, perks %s\n", ch.Start, ch.Perks)
	who := "the route's"
	if ch.PersonaFixed {
		who = "the card's"
	}
	fmt.Fprintf(w, "persona   %s (%s)\n", ch.Persona, who)
	if ch.Format != "" {
		fmt.Fprintf(w, "format    %s\n", ch.Format)
	}
	if ch.Excluded != "" {
		fmt.Fprintf(w, "excluded  %s (played last)\n", ch.Excluded)
	}
	if len(ch.Recent) > 0 {
		fmt.Fprintf(w, "recent    %s\n", strings.Join(ch.Recent, ", "))
	}
	if ch.Fallback != "" {
		fmt.Fprintf(w, "FALLBACK  %s\n", ch.Fallback)
	}
	fmt.Fprintf(w, "model     %s, %s s\n", ch.Model, strconv.FormatFloat(float64(ch.MS)/1000, 'f', 1, 64))
	fmt.Fprintf(w, "thesis    %s\n\n", ch.Thesis)

	if dryRun {
		fmt.Fprintln(w, "DRY RUN: nothing recorded; the flags below name files that do not exist.")
	} else {
		fmt.Fprintf(w, "recorded  %s (delete the directory if this run is not played)\n", filepath.Join(runDir, agency.ChoiceFile))
		if materialized != "" {
			fmt.Fprintf(w, "gym/mod   %s\n", materialized)
		}
	}
	fmt.Fprintln(w, "\nmod, Options → Mods → autopilot_directives (one line):")
	fmt.Fprintln(w, line)
	flags := "-route " + routeRef
	if f != nil {
		flags += " -format " + f.Name
	}
	flags += " -persona " + ch.Persona
	fmt.Fprintf(w, "\nbridge:    -choice %s %s\n", filepath.Join(runDir, agency.ChoiceFile), flags)
	fmt.Fprintf(w, "fenceshow: %s\n", flags)
}
