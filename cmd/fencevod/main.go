// Command fencevod turns a run's recorded show feed or decision log, and the OBS recordings'
// start times, into a VOD kit per recording: YouTube chapters, title, description, tags,
// highlight clip windows and an ffmpeg cut script (docs/STREAM-OPS.md, "After the stream").
// Everything stays local: it uploads nothing and calls no model.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/show"
	"github.com/gallowaysoftware/fence/internal/vod"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fencevod:", err)
		os.Exit(1)
	}
}

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(s string) error { *l = append(*l, s); return nil }

type config struct {
	feeds, starts, recordings list
	replay, personas          string
	model, persona            string
	out, tz                   string
	clips                     int
	probe                     bool
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	var c config
	fs := flag.NewFlagSet("fencevod", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Var(&c.feeds, "feed", "a recorded show feed (fenceshow -record); repeat for a run's several stream feeds")
	fs.StringVar(&c.replay, "replay", "", "a match directory whose decision log gives the moments (the current builder's beats); with -feed, the feed gives only the narrator's lines")
	fs.Var(&c.starts, "recording-start", "when an OBS recording started: RFC 3339, or local \"2006-01-02 15:04:05\"; \"/3h10m\" appended gives its length. Repeat per recording")
	fs.Var(&c.recordings, "recording", "an OBS recording file, in the same order as -recording-start (its start is read from OBS's default file name when -recording-start is not given)")
	fs.StringVar(&c.out, "out", "", "output directory (e.g. var/vod/2026-11-11); one part-N directory per recording when there are several")
	fs.StringVar(&c.tz, "tz", "Local", "time zone of local recording times and OBS file names")
	fs.IntVar(&c.clips, "clips", 12, "how many highlight clips to pick")
	fs.BoolVar(&c.probe, "probe", true, "read each -recording's length with ffprobe when installed")
	fs.StringVar(&c.personas, "personas", "docs/personas", "persona directory, for -replay")
	fs.StringVar(&c.model, "model", "", "model name for a -replay whose match.json gives none")
	fs.StringVar(&c.persona, "persona", "", "persona id for a -replay whose bridge args give none")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	switch {
	case len(c.feeds) == 0 && c.replay == "":
		return c, errors.New("give -feed or -replay")
	case c.out == "":
		return c, errors.New("give -out")
	case len(c.starts) == 0 && len(c.recordings) == 0:
		return c, errors.New("give -recording-start (or -recording with OBS's default file names)")
	case len(c.starts) > 0 && len(c.recordings) > 0 && len(c.starts) != len(c.recordings):
		return c, errors.New("give one -recording-start per -recording")
	case c.clips < 0:
		return c, errors.New("-clips must be 0 or more")
	}
	return c, nil
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}
	loc, err := time.LoadLocation(c.tz)
	if err != nil {
		return err
	}
	recs, err := recordings(ctx, c, loc)
	if err != nil {
		return err
	}
	tl, err := timeline(c)
	if err != nil {
		return err
	}
	if len(tl.Points) == 0 {
		return errors.New("no events with a time in the input")
	}
	parts, err := vod.Split(tl, recs)
	if err != nil {
		return err
	}
	for i := range parts {
		p := &parts[i]
		k := vod.Make(p, tl.Runs, c.clips)
		dir := c.out
		if len(parts) > 1 {
			dir = filepath.Join(c.out, "part-"+strconv.Itoa(p.N))
		}
		if err := vod.Write(dir, k); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s: %s, days %d–%d, %d chapters, %d clips\n", dir, vod.Stamp(k.Length), k.FromDay, k.ToDay, len(k.Chapters), len(k.Clips))
		if len(p.Points) == 0 {
			fmt.Fprintf(stderr, "fencevod: recording %d holds no events: check its start time and -tz\n", p.N)
		}
	}
	return nil
}

func recordings(ctx context.Context, c config, loc *time.Location) ([]vod.Recording, error) {
	n := max(len(c.starts), len(c.recordings))
	recs := make([]vod.Recording, n)
	for i := range recs {
		r := &recs[i]
		if i < len(c.starts) {
			var err error
			if *r, err = vod.ParseStart(c.starts[i], loc); err != nil {
				return nil, err
			}
		}
		if i < len(c.recordings) {
			r.File = c.recordings[i]
			if r.Start.IsZero() {
				t, ok := vod.StartFromName(r.File, loc)
				if !ok {
					return nil, fmt.Errorf("%s: not an OBS default file name; give -recording-start", r.File)
				}
				r.Start = t
			}
			if r.Length == 0 && c.probe {
				r.Length = probeLength(ctx, r.File)
			}
		}
	}
	return recs, nil
}

// probeLength is a recording's length by ffprobe, 0 when ffprobe or the file is missing.
func probeLength(ctx context.Context, file string) time.Duration {
	if _, err := os.Stat(file); err != nil {
		return 0
	}
	out, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", file).Output()
	if err != nil {
		return 0
	}
	s, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

func timeline(c config) (*vod.Timeline, error) {
	var feed []show.Event
	formats := map[string]vod.Format{}
	for _, f := range c.feeds {
		evs, fs, err := readFeed(f)
		if err != nil {
			return nil, err
		}
		feed = append(feed, evs...)
		maps.Copy(formats, fs)
	}
	if c.replay == "" {
		tl := vod.Build(feed, nil)
		tl.SetFormats(formats)
		return tl, nil
	}
	m, err := show.LoadMatch(c.replay, c.personas)
	if err != nil {
		return nil, err
	}
	if c.model != "" && m.Player.Model == "" {
		m.Player.Model = c.model
	}
	id := m.Player.Persona
	if id == "" {
		id = c.persona
	}
	m.Player.SetPersona(c.personas, id, "")
	f, err := os.Open(m.LogFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var evs []show.Event
	if err := show.EachEvent(f, m.Builder(), show.Window{}, func(ev show.Event) error {
		evs = append(evs, ev)
		return nil
	}); err != nil {
		return nil, err
	}
	// The feed's own moments name what each line told; a nil feed still replaces the log's lines.
	tl := vod.Build(evs, append([]show.Event{}, feed...))
	// The feed's run start is the name card the stream showed; it fills what the match lacks.
	fed := vod.Build(feed, nil)
	fed.SetFormats(formats)
	fromFeed := fed.Runs
	for id, r := range tl.Runs {
		o := fromFeed[id]
		if o == nil && len(fromFeed) == 1 && len(tl.Runs) == 1 {
			for _, only := range fromFeed {
				o = only
			}
		}
		r.Merge(o)
	}
	return tl, nil
}

func readFeed(path string) ([]show.Event, map[string]vod.Format, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	evs, formats, err := vod.ReadFeed(f)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	return evs, formats, nil
}
