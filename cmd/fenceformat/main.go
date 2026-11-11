// Command fenceformat lists the stream formats (docs/SHOW.md, "Formats"), writes a format's
// directives for the gym, the bridge or the mod, measures its odds in the gym and keeps the local
// record of attempts.
//
//	fenceformat list
//	fenceformat card FORMAT
//	fenceformat directives [-route R] FORMAT          (one line, for the mod's autopilot_directives)
//	fenceformat write [-route R] -dir DIR FORMAT
//	fenceformat lab [-seed0 N -seeds 60 -parallel 8 -gym DLL -out DIR -update] FORMAT
//	fenceformat tally [-update] FORMAT SUMMARY_DIR...
//	fenceformat record -run ID -ending E -day D [-source stream] FORMAT
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
	"text/tabwriter"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/route"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fenceformat:", err)
		os.Exit(1)
	}
}

const usage = "usage: fenceformat list | card | directives | write | lab | tally | record [flags] FORMAT"

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cmd, args := args[0], args[1:]
	fs := flag.NewFlagSet("fenceformat "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("formats", format.DefaultDir, "format directory")
	recordPath := fs.String("record", format.DefaultRecord, "the local record of attempts")
	routeRef := fs.String("route", "", "route for a format that names none (default "+format.DefaultRoute+")")
	outDir := fs.String("dir", "", "write: output directory")
	lab := labConfig{}
	fs.Int64Var(&lab.seed0, "seed0", 8300000, "lab: seeds are seed0+1 … seed0+seeds")
	fs.IntVar(&lab.seeds, "seeds", 60, "lab: number of seeds")
	fs.IntVar(&lab.parallel, "parallel", 8, "lab: gym processes at once (each at nice 10)")
	fs.IntVar(&lab.days, "days", 1000, "lab: day horizon")
	fs.StringVar(&lab.gym, "gym", "sim/gym/bin/Release/net10.0/fence-gym.dll", "lab: the gym's dll")
	fs.StringVar(&lab.out, "out", "", "lab: summary directory (default var/formats/lab/<format>-<seed0>)")
	update := fs.Bool("update", false, "lab, tally: write the measured record into the format file")
	var a format.Attempt
	fs.StringVar(&a.Run, "run", "", "record: the run's id")
	fs.StringVar(&a.Ending, "ending", "", "record: buyout, buyout_mortgage, rent, repo, arrest, …")
	fs.IntVar(&a.Day, "day", 0, "record: the last day")
	fs.StringVar(&a.Source, "source", "stream", "record: stream or league")
	fs.StringVar(&a.Model, "model", "", "record: the strategist")
	fs.StringVar(&a.Persona, "persona", "", "record: the persona")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if cmd == "list" {
		return list(*dir, *recordPath, stdout)
	}
	if fs.NArg() < 1 {
		return errors.New(usage)
	}
	f, err := format.Load(*dir, fs.Arg(0))
	if err != nil {
		return err
	}
	switch cmd {
	case "card":
		t, err := format.ReadRecord(*recordPath, f.Name, "")
		if err != nil {
			return err
		}
		c := format.CardOf(f, t)
		fmt.Fprintf(stdout, "%s\n%s\nHandicaps: %s\nLab: %s\nAttempts: %s\nNarrator: %s\n\n%s\n", c.Title, c.Card,
			strings.Join(c.Handicaps, "; "), c.Lab, c.Attempts, f.Pitch+" "+f.Lab.Words()+".", f.StrategistText())
	case "directives":
		path, err := f.RouteFile(*routeRef)
		if err != nil {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if b, err = f.Apply(b); err != nil {
			return err
		}
		line, err := route.StripMeta(b)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, line)
	case "write":
		if *outDir == "" {
			return errors.New("write needs -dir")
		}
		path, err := f.Materialize(*routeRef, *outDir)
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, path)
	case "lab":
		if lab.out == "" {
			lab.out = filepath.Join("var", "formats", "lab", fmt.Sprintf("%s-%d", f.Name, lab.seed0))
		}
		l, err := runLab(ctx, f, *routeRef, lab, stderr)
		if err != nil {
			return err
		}
		return report(f, l, *dir, *update, stdout)
	case "tally":
		var runs []format.Run
		for _, d := range fs.Args()[1:] {
			rs, err := readSummaries(d)
			if err != nil {
				return err
			}
			runs = append(runs, rs...)
		}
		if len(runs) == 0 {
			return errors.New("tally: no summaries")
		}
		l := f.Tally(runs)
		l.Route, l.Seeds = f.Lab.Route, seedRange(runs)
		if l.Route == "" {
			l.Route = routeName(f, *routeRef)
		}
		return report(f, l, *dir, *update, stdout)
	case "record":
		if a.Run == "" || a.Ending == "" {
			return errors.New("record needs -run and -ending")
		}
		a.Format, a.Won = f.Name, f.Won(a.Ending, a.Day)
		added, err := format.Append(*recordPath, a)
		if err != nil {
			return err
		}
		if !added {
			fmt.Fprintln(stdout, "already recorded")
		}
	default:
		return errors.New(usage)
	}
	return nil
}

func list(dir, recordPath string, w io.Writer) error {
	fs, err := format.List(dir)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "FORMAT\tDIFFICULTY\tROUTE\tHANDICAPS\tLAB\tMEDIAN END\tJEOPARDY\tATTEMPTS")
	for _, f := range fs {
		t, err := format.ReadRecord(recordPath, f.Name, "")
		if err != nil {
			return err
		}
		r := f.Route
		if r == "" {
			r = "any"
		}
		j := ""
		if f.Lab.Jeopardy() {
			j = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d/%d\t%d\t%s\t%s\n", f.Name, f.Difficulty, r, strings.Join(f.HandicapLabels(), ", "),
			f.Lab.Wins, f.Lab.Runs, f.Lab.MedianEndDay, j, t.Text())
	}
	return tw.Flush()
}

func report(f format.Format, l format.Lab, dir string, update bool, w io.Writer) error {
	lo, hi := format.Wilson(l.Wins, l.Runs)
	fmt.Fprintf(w, "%s: %d/%d won (95 %% CI %.0f–%.0f %%), median end day %d, median win day %d, endings %v\n",
		f.Name, l.Wins, l.Runs, lo, hi, l.MedianEndDay, l.MedianWinDay, l.Endings)
	if !update {
		return nil
	}
	l.Note = f.Lab.Note
	f.Lab = l
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, f.Name+".json"), append(b, '\n'), 0o644)
}
