// Command fencedata reads what the bridge recorded: `status` reports runs, sessions, days,
// counts and gaps; `export` writes one trajectory file per run (JSONL, one line per
// in-game day) for imitation and outcome learning. The decision logs are the source of
// truth; the memory database only says which run a session's days belong to.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/memory"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fencedata:", err)
		os.Exit(1)
	}
}

const usage = "usage: fencedata status|export [flags]"

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}
	cmd, args := args[0], args[1:]
	fs := flag.NewFlagSet("fencedata "+cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	logDir := fs.String("log-dir", envOr("FENCE_LOG_DIR", "var/log"), "decision log directory (env FENCE_LOG_DIR)")
	memDB := fs.String("memory-db", envOr("FENCE_MEMORY_DB", "var/memory.db"), "memory database, read-only, for run ids (env FENCE_MEMORY_DB)")
	silence := fs.Duration("silence", 10*time.Minute, "report a running session that logged nothing for longer than this")
	var out *string
	switch cmd {
	case "export":
		out = fs.String("out", "var/export", "directory for the trajectory files")
	case "status":
	default:
		return fmt.Errorf("unknown command %q; %s", cmd, usage)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments %v", fs.Args())
	}
	sessions, err := decisionlog.ReadDir(*logDir)
	if err != nil {
		return err
	}
	index, err := memory.ReadRunIndex(ctx, *memDB)
	if err != nil {
		fmt.Fprintf(stderr, "fencedata: memory index unavailable, grouping by session: %v\n", err)
		index = memory.RunIndex{}
	}
	trs := build(sessions, index)
	if cmd == "status" {
		return status(stdout, sessions, trs, *silence)
	}
	return export(stdout, *out, trs)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// export writes <out>/<run>.jsonl for every trajectory, replacing each file atomically.
func export(stdout io.Writer, out string, trs []*Trajectory) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, tr := range trs {
		var b bytes.Buffer
		for _, d := range tr.Days {
			line, err := json.Marshal(d)
			if err != nil {
				return fmt.Errorf("%s day %d: %w", tr.Name, d.GameDay, err)
			}
			b.Write(line)
			b.WriteByte('\n')
		}
		path := filepath.Join(out, tr.Name+".jsonl")
		if err := writeAtomic(path, b.Bytes()); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s: %d days, %d sessions\n", path, len(tr.Days), len(tr.Sessions))
	}
	return nil
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

type counts struct {
	days                                                               []int
	events, snapshots, customers, decisions, actions, outcomes, labels int
}

func (c *counts) add(d *Day) {
	if !slices.Contains(c.days, d.GameDay) {
		c.days = append(c.days, d.GameDay)
	}
	c.events += len(d.Events) + len(d.Snapshots) + len(d.Customers) + len(d.HumanActions)
	c.snapshots += len(d.Snapshots)
	c.customers += len(d.Customers)
	c.decisions += len(d.Decisions)
	c.actions += len(d.HumanActions)
	c.outcomes += len(d.Outcomes)
	c.labels += len(d.Labels)
}

func dayRange(days []int) string {
	if len(days) == 0 {
		return "-"
	}
	lo, hi := slices.Min(days), slices.Max(days)
	if lo == hi {
		return fmt.Sprint(lo)
	}
	return fmt.Sprintf("%d-%d (%d)", lo, hi, len(days))
}

func status(w io.Writer, sessions []decisionlog.Session, trs []*Trajectory, silence time.Duration) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RUN\tSESSIONS\tDAYS\tEVENTS\tSNAPSHOTS\tCUSTOMERS\tDECISIONS\tHUMAN\tOUTCOMES\tLABELS")
	var total counts
	totalDays := 0
	runsOf := map[string][]string{}
	for _, tr := range trs {
		var c counts
		for _, d := range tr.Days {
			c.add(d)
			total.add(d)
		}
		totalDays += len(tr.Days)
		for _, s := range tr.Sessions {
			runsOf[s] = append(runsOf[s], tr.Name)
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n", tr.Name, len(tr.Sessions), dayRange(c.days),
			c.events, c.snapshots, c.customers, c.decisions, c.actions, c.outcomes, c.labels)
	}
	fmt.Fprintln(tw)
	fmt.Fprintln(tw, "SESSION\tRUNS\tSTART\tRECORDS\tMISSED\tOUT-OF-ORDER\tDROPPED\tSILENCES\tBAD LINES")
	var details []string
	var missed, dropped int64
	silences, gapped := 0, 0
	for _, s := range sessions {
		g := decisionlog.Gaps(s, silence)
		start := "-"
		if len(s.Records) > 0 {
			start = s.Records[0].TS.UTC().Format(time.RFC3339)
		}
		runs := strings.Join(runsOf[s.ID], ",")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d\t%d\t%d\n", s.ID, nonEmpty(runs, "-"), start, len(s.Records),
			g.Missed, g.OutOfOrder, g.Dropped, len(g.Silences), g.BadLines)
		missed += g.Missed
		dropped += g.Dropped
		silences += len(g.Silences)
		if !g.Clean() {
			gapped++
		}
		for _, sg := range g.SeqGaps {
			details = append(details, fmt.Sprintf("%s: seq %d-%d missing (%d)", s.ID, sg.After+1, sg.Before-1, sg.Missed()))
		}
		for _, si := range g.Silences {
			details = append(details, fmt.Sprintf("%s: nothing logged %s to %s (%s)", s.ID,
				si.From.UTC().Format(time.RFC3339), si.To.UTC().Format(time.RFC3339), si.To.Sub(si.From).Round(time.Second)))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(details) > 0 {
		fmt.Fprintln(w, "\nGaps:")
		for _, d := range details {
			fmt.Fprintln(w, "  "+d)
		}
	}
	fmt.Fprintf(w, "\nTotal: %d runs, %d sessions (%d with gaps), %d run-days; %d customers, %d decisions, %d human actions, %d outcomes, %d snapshots, %d labels; %d events missed, %d records dropped, %d silences\n",
		len(trs), len(sessions), gapped, totalDays, total.customers, total.decisions, total.actions, total.outcomes, total.snapshots, total.labels,
		missed, dropped, silences)
	return nil
}
