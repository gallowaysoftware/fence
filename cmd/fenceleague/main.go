// Command fenceleague runs the T0 scoreboard: a fixed league of gym matches per player,
// and the report every training step is judged by (docs/PLAN.md, "Training path").
//
//	fenceleague run    -spec league/t0-first.json   play every unfinished match (resumable)
//	fenceleague status -spec league/t0-first.json
//	fenceleague report -spec league/t0-first.json   var/league/<name>/REPORT.md + summary.json
//	fenceleague probe  [-endpoints a,b]             latency and concurrency per endpoint
//	fenceleague gym    [-ref HEAD] [-overlay DIR]   a clean, built gym worktree
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

const (
	leagueRoot   = "var/league"
	defaultLocal = "var/league/local.json"
	probeFile    = "var/league/probe.json"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fenceleague:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: fenceleague run|status|report|probe|gym|propose [flags]")
	}
	cmd, args := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(stderr)
	specPath := fs.String("spec", "league/t0-first.json", "league spec")
	localPath := fs.String("local", defaultLocal, "machine config: endpoints and gym (untracked)")
	switch cmd {
	case "run":
		only := fs.String("only", "", "comma-separated player ids to run (default all)")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return cmdRun(ctx, *specPath, *localPath, *only, stderr)
	case "status":
		if err := fs.Parse(args); err != nil {
			return err
		}
		return cmdStatus(*specPath, *localPath, stdout)
	case "report":
		if err := fs.Parse(args); err != nil {
			return err
		}
		return cmdReport(*specPath, stdout)
	case "probe":
		eps := fs.String("endpoints", "", "comma-separated endpoint names (default all)")
		levels := fs.String("levels", "1,2,4,8", "concurrency levels")
		maxTok := fs.Int("max-tokens", 400, "completion tokens per request")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return cmdProbe(ctx, *localPath, *eps, *levels, *maxTok, stdout)
	case "gym":
		ref := fs.String("ref", "HEAD", "git ref the gym is built from")
		overlay := fs.String("overlay", "", "a worktree whose uncommitted sim/gym changes are laid over ref")
		dir := fs.String("dir", "", "worktree directory (default ~/.cache/fence-league/gym-<sha>[-overlay])")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return cmdGym(*ref, *overlay, *dir, stdout, stderr)
	case "propose":
		dir := fs.String("dir", "", "fence-rollout's -propose-dir")
		baseURL := fs.String("base-url", "", "OpenAI-compatible endpoint, e.g. http://127.0.0.1:9091/v1")
		model := fs.String("model", "", "model name")
		thinking := fs.String("thinking", "off", "on|off|'' (server default)")
		maxTok := fs.Int("max-tokens", proposeMaxTokens, "completion tokens per proposal")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return cmdPropose(ctx, *dir, *baseURL, *model, *thinking, *maxTok, stderr)
	}
	return fmt.Errorf("unknown command %q", cmd)
}

func cmdRun(ctx context.Context, specPath, localPath, only string, stderr io.Writer) error {
	spec, err := LoadSpec(specPath)
	if err != nil {
		return err
	}
	local, err := LoadLocal(localPath)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	dir := filepath.Join(leagueRoot, spec.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	unlock, err := lock(dir)
	if err != nil {
		return err
	}
	defer unlock()
	if err := writeJSON(filepath.Join(dir, "spec.json"), spec); err != nil {
		return err
	}

	gymDLL := filepath.Join(local.GymDir, "sim/gym/bin/Release/net10.0/fence-gym.dll")
	if !fileExists(gymDLL) {
		return fmt.Errorf("no gym at %s: run `fenceleague gym` and set gym_dir in %s", gymDLL, localPath)
	}
	bridgeBin, _ := filepath.Abs(filepath.Join(leagueRoot, "bin", "bridge"))
	build := exec.Command("go", "build", "-o", bridgeBin, "./cmd/bridge")
	build.Stdout, build.Stderr = stderr, stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("building the bridge: %w", err)
	}
	help, _ := exec.Command(bridgeBin, "-h").CombinedOutput()
	persona := strings.Contains(string(help), "-persona")
	if len(spec.Personas) > 0 && !persona {
		log.Warn("the bridge has no -persona flag; personas only label the matches")
	}
	if len(spec.Routes) > 0 && !strings.Contains(string(help), "-route") {
		return errors.New("the bridge has no -route flag")
	}
	if !strings.Contains(string(help), "-planner") {
		return errors.New("the bridge has no -planner flag")
	}

	perks := gymTakesPerks(local.GymDir)
	if !perks && (spec.PerkSample != nil || slices.ContainsFunc(spec.Perks, func(p string) bool { return p != NoPerks })) {
		log.Warn("the gym has no -perks flag; every match starts with no perks and records that")
	}

	bridges := map[string]string{}
	for _, p := range spec.Players {
		if p.Bridge == "" {
			continue
		}
		bin := mustAbs(filepath.Join(dir, "bin", "bridge-"+p.ID))
		build := exec.Command("go", "build", "-o", bin, "./cmd/bridge")
		build.Dir, build.Stdout, build.Stderr = p.Bridge, stderr, stderr
		if err := build.Run(); err != nil {
			return fmt.Errorf("player %s: building the bridge in %s: %w", p.ID, p.Bridge, err)
		}
		bridges[p.ID] = bin
	}

	probes := loadProbes()
	l := &League{Spec: spec, Local: local, Dir: mustAbs(dir), BridgeBin: bridgeBin, Bridges: bridges, GymDLL: mustAbs(gymDLL), Persona: persona, Perks: perks,
		Conc: map[string]int{}, Skip: map[string]string{}, Log: log}
	var keep []string
	if only != "" {
		keep = strings.Split(only, ",")
	}
	playing := gameRunning("/proc")
	pinged := map[string]bool{}
	for _, p := range spec.Players {
		if keep != nil && !slices.Contains(keep, p.ID) {
			l.Skip[p.ID] = "not in -only"
			continue
		}
		if !p.usesLLM() {
			continue
		}
		ep := local.Endpoints[p.Endpoint]
		if ep == nil {
			return fmt.Errorf("player %s: endpoint %q is not in %s", p.ID, p.Endpoint, localPath)
		}
		if ep.Disabled {
			l.Skip[p.ID] = "endpoint " + p.Endpoint + " is disabled in " + localPath
			continue
		}
		l.Conc[p.Endpoint] = ep.Concurrency
		if ep.Concurrency <= 0 {
			l.Conc[p.Endpoint] = max(1, probes[p.Endpoint])
		}
		if _, err := ep.probeTimeout(); err != nil {
			return fmt.Errorf("endpoint %s probe_timeout: %w", p.Endpoint, err)
		}
		if ep.YieldsToGame && playing {
			continue // its matches wait for the game to close
		}
		if !pinged[p.Endpoint] {
			pinged[p.Endpoint] = true
			l.ping(ctx, p.Endpoint) // a failure only holds its matches until a re-probe passes
		}
	}
	runErr := l.Run(ctx)
	if err := writeReport(spec, dir); err != nil {
		log.Error("report", "err", err)
	}
	return runErr
}

// gymTakesPerks reads the gym's source: its flag parser ignores flags it does not know, so
// asking it would not tell.
func gymTakesPerks(gymDir string) bool {
	files, _ := filepath.Glob(filepath.Join(gymDir, "sim", "gym", "*.cs"))
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), `"perks"`) {
			return true
		}
	}
	return false
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

// lock keeps two orchestrators off one league.
func lock(dir string) (func(), error) {
	path := filepath.Join(dir, ".lock")
	if b, err := os.ReadFile(path); err == nil {
		if pid, _ := strconv.Atoi(strings.TrimSpace(string(b))); pid > 0 && syscall.Kill(pid, 0) == nil {
			return nil, fmt.Errorf("league already running (pid %d)", pid)
		}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return nil, err
	}
	return func() { os.Remove(path) }, nil
}

func lockHolder(dir string) int {
	b, err := os.ReadFile(filepath.Join(dir, ".lock"))
	if err != nil {
		return 0
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	if pid > 0 && syscall.Kill(pid, 0) == nil {
		return pid
	}
	return 0
}

func cmdStatus(specPath, localPath string, w io.Writer) error {
	spec, err := LoadSpec(specPath)
	if err != nil {
		return err
	}
	dir := filepath.Join(leagueRoot, spec.Name)
	pid := lockHolder(dir)
	if pid > 0 {
		fmt.Fprintf(w, "league %s: running (pid %d)\n", spec.Name, pid)
	} else {
		fmt.Fprintf(w, "league %s: not running\n", spec.Name)
	}
	disabled := map[string]bool{}
	if local, err := LoadLocal(localPath); err == nil {
		for name, ep := range local.Endpoints {
			disabled[name] = ep.Disabled
		}
	}
	type counts struct{ done, running, failed, pending, skipped int }
	per := map[string]*counts{}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "MATCH\tSTATE\tDETAIL")
	for _, m := range spec.Matches() {
		c := per[m.Player.ID]
		if c == nil {
			c = &counts{}
			per[m.Player.ID] = c
		}
		md := filepath.Join(dir, m.ID())
		var r Result
		switch {
		case readJSON(filepath.Join(md, resultFile), &r) == nil:
			c.done++
			fmt.Fprintf(tw, "%s\tdone\t%s day %d, nw %d, %d LLM calls, %s\n", m.ID(), r.Ending, r.DaysPlayed, lastNW(r), r.LLMCalls, dur(r.WallSeconds))
		case fileExists(filepath.Join(md, errorFile)):
			c.failed++
			var e map[string]string
			readJSON(filepath.Join(md, errorFile), &e)
			fmt.Fprintf(tw, "%s\tfailed\t%s\n", m.ID(), clip(e["error"], 100))
		case disabled[m.Player.Endpoint]:
			c.skipped++
		case pid > 0 && fileExists(filepath.Join(md, "match.json")):
			c.running++
			fmt.Fprintf(tw, "%s\trunning\t%s\n", m.ID(), progress(md))
		default:
			c.pending++
		}
	}
	tw.Flush()
	fmt.Fprintln(w)
	tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PLAYER\tDONE\tRUNNING\tFAILED\tPENDING\tSKIPPED")
	for _, p := range spec.Players {
		c := per[p.ID]
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\n", p.ID, c.done, c.running, c.failed, c.pending, c.skipped)
	}
	return tw.Flush()
}

func lastNW(r Result) int64 {
	if len(r.NetWorth) == 0 {
		return 0
	}
	return r.NetWorth[len(r.NetWorth)-1]
}

// progress is a running match's latest planned day and call count.
func progress(md string) string {
	var meta MatchMeta
	readJSON(filepath.Join(md, "match.json"), &meta)
	started, _ := time.Parse(time.RFC3339, meta.StartedAt)
	day := "-"
	if b, err := os.ReadFile(filepath.Join(md, "bridge.log")); err == nil {
		s := string(b)
		if i := strings.LastIndex(s, `msg="day plan written" day=`); i >= 0 {
			rest := s[i+len(`msg="day plan written" day=`):]
			day, _, _ = strings.Cut(rest, " ")
		}
	}
	calls := 0
	if b, err := os.ReadFile(filepath.Join(md, "llm.jsonl")); err == nil {
		calls = strings.Count(string(b), "\n")
	}
	return fmt.Sprintf("plan day %s, %d LLM calls, %s elapsed", day, calls, dur(time.Since(started).Seconds()))
}

func cmdReport(specPath string, w io.Writer) error {
	spec, err := LoadSpec(specPath)
	if err != nil {
		return err
	}
	dir := filepath.Join(leagueRoot, spec.Name)
	if err := writeReport(spec, dir); err != nil {
		return err
	}
	fmt.Fprintln(w, filepath.Join(dir, "REPORT.md"))
	return nil
}

func writeReport(spec *Spec, dir string) error {
	results, err := LoadResults(spec, dir)
	if err != nil {
		return err
	}
	sum := Summarize(spec, results)
	if err := writeJSON(filepath.Join(dir, "summary.json"), sum); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "REPORT.md"), []byte(sum.Markdown()), 0o644)
}

func loadProbes() map[string]int {
	var all map[string]ProbeResult
	out := map[string]int{}
	if readJSON(probeFile, &all) == nil {
		for k, v := range all {
			out[k] = v.Recommended
		}
	}
	return out
}

func cmdProbe(ctx context.Context, localPath, names, levelsS string, maxTok int, w io.Writer) error {
	local, err := LoadLocal(localPath)
	if err != nil {
		return err
	}
	var levels []int
	for _, s := range strings.Split(levelsS, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n < 1 {
			return fmt.Errorf("-levels %q", levelsS)
		}
		levels = append(levels, n)
	}
	var eps []string
	if names != "" {
		eps = strings.Split(names, ",")
	} else {
		for k := range local.Endpoints {
			eps = append(eps, k)
		}
		slices.Sort(eps)
	}
	all := map[string]ProbeResult{}
	readJSON(probeFile, &all)
	for _, name := range eps {
		ep := local.Endpoints[name]
		if ep == nil {
			return fmt.Errorf("no endpoint %q", name)
		}
		if ep.Disabled {
			fmt.Fprintf(w, "%s: skipped, disabled\n", name)
			continue
		}
		if ep.YieldsToGame && gameRunning("/proc") {
			fmt.Fprintf(w, "%s: skipped, the owner is playing\n", name)
			continue
		}
		res := Probe(ctx, name, ep, levels, maxTok)
		all[name] = res
		fmt.Fprintf(w, "%s (%s): ping %.1f s, recommended concurrency %d\n", name, ep.Model, res.PingS, res.Recommended)
		for _, lv := range res.Levels {
			fmt.Fprintf(w, "  n=%d ok=%d wall %.1f s, p50 %.1f s, max %.1f s, %.1f tok/s %s\n", lv.N, lv.OK, lv.WallS, lv.LatencyP50S, lv.LatencyMaxS, lv.TokPerS, lv.Error)
		}
		if err := writeJSON(probeFile, all); err != nil {
			return err
		}
	}
	return nil
}

// cmdGym makes a clean worktree of ref, so in-progress edits in the main checkout never
// break a league, optionally with another worktree's uncommitted sim/gym changes on top.
func cmdGym(ref, overlay, dir string, stdout, stderr io.Writer) error {
	sha, err := exec.Command("git", "rev-parse", "--short", ref).Output()
	if err != nil {
		return fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache", "fence-league", "gym-"+strings.TrimSpace(string(sha)))
		if overlay != "" {
			dir += "-" + filepath.Base(overlay)
		}
	}
	runIn := func(cwd string, name string, args ...string) error {
		c := exec.Command(name, args...)
		c.Dir, c.Stdout, c.Stderr = cwd, stderr, stderr
		return c.Run()
	}
	if !fileExists(dir) {
		if err := runIn(".", "git", "worktree", "add", "--detach", dir, ref); err != nil {
			return err
		}
		if overlay != "" {
			patch, err := exec.Command("git", "-C", overlay, "diff", "--", "sim/gym").Output()
			if err != nil {
				return err
			}
			c := exec.Command("git", "apply")
			c.Dir, c.Stdin, c.Stderr = dir, strings.NewReader(string(patch)), stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("applying %s's sim/gym diff: %w", overlay, err)
			}
			untracked, err := exec.Command("git", "-C", overlay, "ls-files", "--others", "--exclude-standard", "--", "sim/gym").Output()
			if err != nil {
				return err
			}
			for _, f := range strings.Fields(string(untracked)) {
				b, err := os.ReadFile(filepath.Join(overlay, f))
				if err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(dir, f), b, 0o644); err != nil {
					return err
				}
			}
		}
		// The loot extractor's class database, fetched once per checkout otherwise.
		if tools, err := filepath.Abs("var/tools"); err == nil && fileExists(tools) {
			os.MkdirAll(filepath.Join(dir, "var"), 0o755)
			os.Symlink(tools, filepath.Join(dir, "var", "tools"))
		}
	}
	if err := runIn(dir, "dotnet", "build", "sim/gym", "-c", "Release"); err != nil {
		return err
	}
	if fileExists(filepath.Join(dir, "sim", "rollout")) {
		if err := runIn(dir, "dotnet", "build", "sim/rollout", "-c", "Release"); err != nil {
			return err
		}
	}
	fmt.Fprintln(stdout, dir)
	return nil
}
