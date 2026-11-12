// Command fenceinvent is the route inventor (docs/research/ROUTES.md, "Invented routes"): the
// model proposes hard routes the menu does not have, and the gym screens and confirms them.
//
//	fenceinvent propose -dir DIR -n 4 -base-url URL -model M    the model's proposals → DIR/routes/*.json
//	fenceinvent screen  -dir DIR -tag screen -from SEED -seeds 30   gym runs of every proposal and the reference
//	fenceinvent report  -dir DIR -tag screen                        wins, win day, paired Δ, income split, fingerprint
//
// A proposal is promoted only by hand, after `screen -tag confirm` on 60 fresh seeds wins at
// least 57 (the menu's bar).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gallowaysoftware/fence/internal/agency"
	"github.com/gallowaysoftware/fence/internal/llm"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fenceinvent:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fenceinvent propose|screen|report [flags]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	dir := fs.String("dir", "", "the round's directory")
	switch args[0] {
	case "propose":
		n := fs.Int("n", 4, "routes to propose")
		baseURL := fs.String("base-url", "", "OpenAI-compatible endpoint")
		model := fs.String("model", "", "model name")
		thinking := fs.String("thinking", "off", "on|off|'' (server default)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return propose(ctx, *dir, *n, *baseURL, *model, *thinking, out)
	case "screen", "report":
		tag := fs.String("tag", "screen", "the block's name: screen, confirm, …")
		from := fs.Int("from", 0, "first seed")
		seeds := fs.Int("seeds", 30, "seeds from -from")
		par := fs.Int("par", 8, "gym processes at once")
		gym := fs.String("gym", filepath.Join("sim", "gym", "bin", "Release", "net10.0", "fence-gym.dll"), "fence-gym.dll")
		only := fs.String("only", "", "comma-separated proposals (default: every one in DIR/routes)")
		ref := fs.String("ref", agency.DefaultRoute, "the menu route every proposal is paired with")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		arms, err := loadArms(*dir, *ref, *only)
		if err != nil {
			return err
		}
		block := filepath.Join(*dir, *tag)
		if args[0] == "screen" {
			if *from <= 0 {
				return errors.New("screen needs -from")
			}
			if err := screen(ctx, block, *gym, arms, *from, *seeds, *par); err != nil {
				return err
			}
		}
		return report(block, arms, out)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func propose(ctx context.Context, dir string, n int, baseURL, model, thinking string, out io.Writer) error {
	if dir == "" || baseURL == "" || model == "" {
		return errors.New("propose needs -dir, -base-url and -model")
	}
	menu, err := agency.LoadMenu("")
	if err != nil {
		return err
	}
	opts := llm.OpenAIOptions{}
	if thinking != "" {
		on := thinking == "on"
		opts.Thinking = &on
	}
	c := &llm.OpenAI{BaseURL: strings.TrimRight(baseURL, "/"), Model: model, OpenAIOptions: opts}
	ps, raw, err := agency.Invent(ctx, c, menu, n)
	if err := os.MkdirAll(filepath.Join(dir, "routes"), 0o755); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(dir, "raw.txt"), []byte(raw), 0o644)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(ps, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "proposals.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	for _, p := range ps {
		if len(p.Errors) > 0 {
			fmt.Fprintf(out, "rejected %s: %s\n", p.Name, strings.Join(p.Errors, "; "))
			continue
		}
		f, err := p.File(menu, "", "Invented: "+p.Summary()+". "+p.Thesis)
		if err != nil {
			fmt.Fprintf(out, "rejected %s: %v\n", p.Name, err)
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, "routes", p.Name+".json"), f, 0o644); err != nil {
			return err
		}
		fmt.Fprintf(out, "proposed %s: %s\n  thesis: %s\n  novelty: %s\n", p.Name, p.Summary(), p.Thesis, p.Novelty)
	}
	return nil
}
