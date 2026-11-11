package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/route"
	"github.com/gallowaysoftware/fence/internal/show"
)

type labConfig struct {
	seed0           int64
	seeds, parallel int
	days            int
	gym, out        string
}

var errGame = errors.New("Probably Stolen is running: no new gym runs")

// runLab plays the format's directives in the gym, executor only, one process a seed, and tallies
// the summaries; a seed whose summary exists is not played again.
func runLab(ctx context.Context, f format.Format, routeRef string, c labConfig, stderr io.Writer) (format.Lab, error) {
	if c.seeds < 1 || c.parallel < 1 {
		return format.Lab{}, errors.New("lab: -seeds and -parallel must be positive")
	}
	path, err := f.Materialize(routeRef, c.out)
	if err != nil {
		return format.Lab{}, err
	}
	r, err := route.Load("", path)
	if err != nil {
		return format.Lab{}, err
	}
	gym, err := filepath.Abs(c.gym)
	if err != nil {
		return format.Lab{}, err
	}
	seeds := make(chan int64)
	errs := make(chan error, c.seeds)
	var wg sync.WaitGroup
	for range c.parallel {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seed := range seeds {
				errs <- playSeed(ctx, gym, f, r, path, c, seed)
			}
		}()
	}
	go func() {
		defer close(seeds)
		for i := int64(1); i <= int64(c.seeds); i++ {
			select {
			case seeds <- c.seed0 + i:
			case <-ctx.Done():
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	var failed []error
	for err := range errs {
		if err != nil {
			failed = append(failed, err)
		}
	}
	if len(failed) > 0 {
		fmt.Fprintf(stderr, "lab: %d runs failed; first: %v\n", len(failed), failed[0])
	}
	runs, err := readSummaries(c.out)
	if err != nil {
		return format.Lab{}, err
	}
	if len(runs) == 0 {
		return format.Lab{}, errors.Join(append([]error{errors.New("lab: no summaries")}, failed...)...)
	}
	l := f.Tally(runs)
	l.Route, l.Seeds = r.Name, seedRange(runs)
	return l, nil
}

func playSeed(ctx context.Context, gym string, f format.Format, r route.Route, directives string, c labConfig, seed int64) error {
	sum := filepath.Join(c.out, strconv.FormatInt(seed, 10)+".json")
	if st, err := os.Stat(sum); err == nil && st.Size() > 0 {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if show.GameRunning() {
		return errGame
	}
	args := []string{"-n", "10", "dotnet", gym, "run", "-seed", strconv.FormatInt(seed, 10), "-days", strconv.Itoa(c.days),
		"-no-trajectory", "-summary", sum + ".tmp", "-difficulty", f.Difficulty, "-start", r.Start, "-perks", r.Perks,
		"-human", "player", "-directives", directives}
	cmd := exec.CommandContext(ctx, "nice", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("seed %d: %w: %s", seed, err, lastLine(out))
	}
	return os.Rename(sum+".tmp", sum)
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return lines[len(lines)-1]
}

// readSummaries reads every gym summary (<seed>.json) in dir.
func readSummaries(dir string) ([]format.Run, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var runs []format.Run
	for _, p := range paths {
		if _, err := strconv.ParseInt(strings.TrimSuffix(filepath.Base(p), ".json"), 10, 64); err != nil {
			continue
		}
		r, err := format.ReadSummary(p)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

// seedRange names the seeds as the docs do: "8300001–060" for a block, else the count.
func seedRange(runs []format.Run) string {
	seeds := make([]int64, len(runs))
	for i, r := range runs {
		seeds[i] = r.Seed
	}
	slices.Sort(seeds)
	lo, hi := seeds[0], seeds[len(seeds)-1]
	if hi-lo+1 != int64(len(seeds)) {
		return fmt.Sprintf("%d seeds from %d to %d", len(seeds), lo, hi)
	}
	a, b := strconv.FormatInt(lo, 10), strconv.FormatInt(hi, 10)
	if len(a) == len(b) && len(a) > 3 && a[:len(a)-3] == b[:len(b)-3] {
		return a + "–" + b[len(b)-3:]
	}
	return a + "–" + b
}

func routeName(f format.Format, routeRef string) string {
	if p, err := f.RouteFile(routeRef); err == nil {
		return strings.TrimSuffix(filepath.Base(p), ".json")
	}
	return f.Route
}
