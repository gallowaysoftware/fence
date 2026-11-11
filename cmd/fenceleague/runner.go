package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/format"
)

const (
	resultFile = "result.json"
	errorFile  = "error.json"
	// maxMatchTime kills a match that hangs; the next run retries it.
	maxMatchTime = 8 * time.Hour
	// niceness keeps gyms and bridges behind anything the owner runs.
	niceness = "10"
)

// League runs a spec's matches.
type League struct {
	Spec      *Spec
	Local     *Local
	Dir       string // var/league/<name>
	BridgeBin string
	// Bridges are per-player builds (Player.Bridge), by player id.
	Bridges map[string]string
	GymDLL  string
	Persona bool // the bridge takes -persona
	Perks   bool // the gym takes -perks
	Conc    map[string]int
	Skip    map[string]string // player id → why it sits this run out
	Log     *slog.Logger

	proxy *Proxy
	gate  *GameGate

	mu     sync.Mutex
	active map[string]*activeMatch
	// down are endpoints whose last probe failed: their matches wait for a passing one.
	down map[string]bool
}

// reprobe is how often endpoints are pinged again, so a busy one rejoins when it frees up.
const reprobe = 10 * time.Minute

// ping probes endpoint and records whether its matches may start.
func (l *League) ping(ctx context.Context, name string) error {
	ep := l.Local.Endpoints[name]
	timeout, _ := ep.probeTimeout()
	d, err := Ping(ctx, ep, timeout)
	l.mu.Lock()
	if l.down == nil {
		l.down = map[string]bool{}
	}
	was := l.down[name]
	l.down[name] = err != nil
	l.mu.Unlock()
	l.Log.Info("endpoint ping", "endpoint", name, "seconds", round1(d.Seconds()), "err", err)
	if was != (err != nil) {
		l.Log.Info("endpoint availability", "endpoint", name, "available", err == nil)
	}
	return err
}

func (l *League) isDown(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.down[name]
}

// watchEndpoints re-probes every endpoint in use, except a yielding one while the owner plays.
func (l *League) watchEndpoints(ctx context.Context) {
	t := time.NewTicker(reprobe)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		for _, name := range mapKeys(l.Conc) {
			if l.Local.Endpoints[name].YieldsToGame && l.gate.Playing() {
				continue
			}
			l.ping(ctx, name)
		}
	}
}

type activeMatch struct {
	yields  bool
	cancel  context.CancelFunc
	yielded bool
}

// errYielded is a match aborted for the owner's game; it restarts from scratch later, since
// a game frozen mid-run would see its timeouts fire on resume and play differently.
var errYielded = errors.New("yielded to the owner's game")

// Run plays every unfinished match, as many at once as the endpoints and CPU allow.
func (l *League) Run(ctx context.Context) error {
	l.active = map[string]*activeMatch{}
	l.gate = NewGameGate("/proc")
	l.gate.OnChange(l.onGame)
	gctx, stopGate := context.WithCancel(ctx)
	defer stopGate()
	go l.gate.Watch(gctx, 5*time.Second)

	go l.watchEndpoints(gctx)
	var err error
	if l.proxy, err = NewProxy(l.gate, l.Local.Endpoints, l.Conc); err != nil {
		return err
	}
	if err := l.proxy.Start(); err != nil {
		return err
	}
	defer l.proxy.Close()

	for _, id := range mapKeys(l.Skip) {
		l.Log.Info("player sits this run out", "player", id, "why", l.Skip[id])
	}
	var pending []Match
	for _, m := range l.Spec.Matches() {
		if _, ok := l.Skip[m.Player.ID]; ok {
			continue
		}
		if fileExists(filepath.Join(l.Dir, m.ID(), resultFile)) {
			continue
		}
		pending = append(pending, m)
	}
	l.Log.Info("league", "name", l.Spec.Name, "pending", len(pending), "concurrency", l.Conc)

	type done struct {
		m   Match
		err error
	}
	doneC := make(chan done)
	running := map[string]int{} // endpoint ("" for none) → matches
	gyms := 0
	failures := 0
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	cancelled := ctx.Done()
	for len(pending) > 0 || gyms > 0 {
		if ctx.Err() == nil {
			for i := 0; i < len(pending); {
				m := pending[i]
				if !l.fits(m, running, gyms) {
					i++
					continue
				}
				pending = append(pending[:i], pending[i+1:]...)
				running[m.Player.Endpoint]++
				gyms++
				go func() { doneC <- done{m, l.runMatch(ctx, m)} }()
			}
		} else {
			pending = nil
		}
		select {
		case d := <-doneC:
			running[d.m.Player.Endpoint]--
			gyms--
			if errors.Is(d.err, errYielded) {
				l.Log.Info("match aborted for the owner's game; it restarts when the game closes", "match", d.m.ID())
				os.RemoveAll(filepath.Join(l.Dir, d.m.ID()))
				if ctx.Err() == nil {
					pending = append([]Match{d.m}, pending...)
				}
			} else if d.err != nil {
				failures++
				l.Log.Error("match failed; a later run retries it", "match", d.m.ID(), "err", d.err)
				writeJSON(filepath.Join(l.Dir, d.m.ID(), errorFile), map[string]string{"error": d.err.Error()})
			} else {
				l.Log.Info("match done", "match", d.m.ID())
				if err := writeReport(l.Spec, l.Dir); err != nil {
					l.Log.Error("report", "err", err)
				}
			}
		case <-tick.C:
		case <-cancelled:
			cancelled = nil // the running matches see ctx themselves; just wait for them
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d matches failed", failures)
	}
	return ctx.Err()
}

func (l *League) maxGyms() int {
	n := l.Local.MaxGyms
	if n <= 0 {
		n = max(1, runtime.NumCPU()/4)
	}
	if l.gate.Playing() {
		w := l.Local.MaxGymsWhileGaming
		if w <= 0 {
			w = 1
		}
		n = min(n, w)
	}
	return n
}

func (l *League) fits(m Match, running map[string]int, gyms int) bool {
	if gyms >= l.maxGyms() {
		return false
	}
	if !m.Player.usesLLM() {
		return true
	}
	ep := l.Local.Endpoints[m.Player.Endpoint]
	if ep.YieldsToGame && l.gate.Playing() || l.isDown(m.Player.Endpoint) {
		return false
	}
	// A hybrid match calls its model only at decision points and rolls out in between; the
	// proxy's slots cap its requests, so it does not hold an endpoint slot for the match.
	if m.Player.Mode == ModeHybrid {
		return true
	}
	return running[m.Player.Endpoint] < max(1, l.Conc[m.Player.Endpoint])
}

// onGame aborts every yielding match when the owner starts playing; fits keeps them from
// starting again until the game closes.
func (l *League) onGame(playing bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Log.Info("owner's game", "running", playing)
	if !playing {
		return
	}
	for name, ep := range l.Local.Endpoints {
		if ep.YieldsToGame && ep.UnloadURL != "" {
			go l.unload(name, ep.UnloadURL)
		}
	}
	for id, a := range l.active {
		if a.yields && !a.yielded {
			a.yielded = true
			a.cancel()
			l.Log.Info("aborting match for the owner's game", "match", id)
		}
	}
}

func (l *League) unload(name, url string) {
	c := http.Client{Timeout: time.Minute}
	resp, err := c.Get(url)
	if err != nil {
		l.Log.Error("unloading the endpoint's model failed", "endpoint", name, "err", err)
		return
	}
	resp.Body.Close()
	l.Log.Info("unloaded the endpoint's model for the owner's game", "endpoint", name, "status", resp.StatusCode)
}

// track registers a starting match; false when the owner is already playing and it yields.
func (l *League) track(id string, yields bool, cancel context.CancelFunc) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if yields && l.gate.Playing() {
		return false
	}
	l.active[id] = &activeMatch{yields: yields, cancel: cancel}
	return true
}

func (l *League) untrack(id string) (yielded bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.active[id]
	delete(l.active, id)
	return a != nil && a.yielded
}

// MatchMeta is written before a match starts: what was run, for the record.
type MatchMeta struct {
	Match      string   `json:"match"`
	League     string   `json:"league"`
	Player     Player   `json:"player"`
	Model      string   `json:"model,omitempty"`
	Seed       int      `json:"seed"`
	Start      string   `json:"start"`
	Persona    string   `json:"persona,omitempty"`
	Route      string   `json:"route,omitempty"`
	Format     string   `json:"format,omitempty"`
	Perks      string   `json:"perks"`
	PerksAsked string   `json:"perks_requested"`
	HeldOut    bool     `json:"perks_held_out,omitempty"`
	Difficulty string   `json:"difficulty"`
	Days       int      `json:"days"`
	Executor   Executor `json:"executor"`
	BridgeArgs []string `json:"bridge_args,omitempty"`
	GymArgs    []string `json:"gym_args"`
	GymDir     string   `json:"gym_dir"`
	StartedAt  string   `json:"started_at"`
}

func (l *League) runMatch(ctx context.Context, m Match) (err error) {
	id := m.ID()
	dir := filepath.Join(l.Dir, id)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, maxMatchTime)
	defer cancel()
	began := time.Now()
	var ep *Endpoint
	yields := false
	if m.Player.usesLLM() {
		ep = l.Local.Endpoints[m.Player.Endpoint]
		yields = ep.YieldsToGame
	}
	if !l.track(id, yields, cancel) {
		return errYielded
	}
	defer func() {
		if l.untrack(id) {
			err = errYielded
		} else if err == nil {
			err = l.finish(m, dir, time.Since(began))
		}
	}()

	if m.Format != "" {
		// The match plays, and records, the format's directives as they were when it started.
		f, err := format.Load("", m.Format)
		if err != nil {
			return err
		}
		if m.RoutePath, err = f.Materialize(m.RoutePath, filepath.Join(dir, "format")); err != nil {
			return err
		}
	}
	meta := MatchMeta{Match: id, League: l.Spec.Name, Player: m.Player, Seed: m.Seed, Start: m.Start, Persona: m.Persona, Route: m.Route, Format: m.Format,
		Perks: l.appliedPerks(m), PerksAsked: m.Perks, HeldOut: m.HeldOut, Difficulty: m.Difficulty, Days: l.Spec.Days, Executor: l.Spec.executor(m.Player), GymDir: l.Local.GymDir, StartedAt: began.UTC().Format(time.RFC3339)}

	meta.Executor.Directives = l.directives(m)
	if m.Player.Mode == ModeHybrid {
		return l.runHybrid(ctx, m, ep, meta, dir)
	}
	var bridgeURL string
	if m.Player.Mode != ModeLocal {
		model := ""
		if ep != nil {
			model = ep.Model
			if m.Player.Model != "" {
				model = m.Player.Model
			}
			if err := l.proxy.Register(id, m.Player.Endpoint, ep, filepath.Join(dir, "llm.jsonl")); err != nil {
				return err
			}
			defer l.proxy.Unregister(id)
		}
		meta.Model = model
		port, err := freePort()
		if err != nil {
			return err
		}
		addr := "127.0.0.1:" + strconv.Itoa(port)
		meta.BridgeArgs = l.bridgeArgs(m, ep, model, addr, dir)
		stop, err := l.startBridge(ctx, l.bridgeBin(m.Player), meta.BridgeArgs, dir)
		if err != nil {
			return err
		}
		defer stop()
		bridgeURL = "http://" + addr
	}

	meta.GymArgs = l.gymArgs(m, bridgeURL, dir)
	if err := writeJSON(filepath.Join(dir, "match.json"), meta); err != nil {
		return err
	}
	return l.runGym(ctx, meta.GymArgs, dir)
}

func (l *League) bridgeArgs(m Match, ep *Endpoint, model, addr, dir string) []string {
	// A plan-mode bridge only supplies directives: in observe mode every decide defers to the
	// executor's own counter (sim/gym/README.md).
	mode := "act"
	if m.Player.Mode == ModePlan {
		mode = "observe"
	}
	a := []string{"-listen", addr, "-mode", mode,
		"-log-dir", filepath.Join(dir, "log"),
		"-memory-db", filepath.Join(dir, "memory.db"),
		"-memory-carry-lessons=false", "-memory-reflect=false"}
	switch m.Player.Mode {
	case ModeHeuristic:
		a = append(a, "-policy", "heuristic")
	case ModePlan:
		a = append(a, "-policy", "heuristic", "-planner", "llm", "-planner-wait", l.Spec.PlanWait)
	case ModeLLM:
		a = append(a, "-policy", "llm", "-planner-wait", l.Spec.PlanWait)
	}
	if ep != nil {
		a = append(a, "-provider", "openai", "-base-url", l.proxy.BaseURL(m.ID()), "-model", model)
		a = append(a, ep.bridgeFlags()...)
	}
	if d := l.directives(m); d != "" && m.Player.Mode == ModePlan {
		a = append(a, "-base-directives", mustAbs(d))
	}
	if m.Route != "" && m.Player.Mode == ModePlan {
		a = append(a, "-route", m.RoutePath)
	}
	if m.Format != "" && m.Player.Mode == ModePlan {
		a = append(a, "-format", m.Format)
	}
	a = append(a, m.Player.BridgeFlags...)
	if m.Persona != "" && l.Persona {
		a = append(a, "-persona", m.Persona)
	}
	return a
}

// directives is the file the match's executor plays: its route's, else the executor's.
func (l *League) directives(m Match) string {
	if m.RoutePath != "" {
		return m.RoutePath
	}
	return l.Spec.executor(m.Player).Directives
}

// appliedPerks is what the match really starts with: none on a gym without -perks.
func (l *League) appliedPerks(m Match) string {
	if l.Perks {
		return m.Perks
	}
	return NoPerks
}

func (l *League) gymArgs(m Match, bridgeURL, dir string) []string {
	a := []string{l.GymDLL, "run",
		"-seed", strconv.Itoa(m.Seed), "-days", strconv.Itoa(l.Spec.Days),
		"-difficulty", m.Difficulty, "-start", m.Start,
		"-human", l.Spec.executor(m.Player).Human,
		"-deadline-ms", strconv.Itoa(l.Spec.DeadlineMS),
		"-out", filepath.Join(dir, "trajectory.jsonl"),
		"-summary", filepath.Join(dir, "gym.summary.json")}
	if m.Perks != NoPerks && l.Perks {
		a = append(a, "-perks", m.Perks)
	}
	if k := l.Spec.executor(m.Player).Knobs; k != "" {
		a = append(a, "-knobs", k)
	}
	if d := l.directives(m); d != "" {
		a = append(a, "-directives", mustAbs(d))
	}
	if bridgeURL != "" {
		a = append(a, "-bridge", bridgeURL)
	}
	return a
}

// childEnv is ours without FENCE_*: a match is configured by its flags alone, never by the
// owner's bridge settings.
func childEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "FENCE_") {
			env = append(env, kv)
		}
	}
	return env
}

func (l *League) bridgeBin(p Player) string {
	if b := l.Bridges[p.ID]; b != "" {
		return b
	}
	return l.BridgeBin
}

func (l *League) startBridge(ctx context.Context, bin string, args []string, dir string) (stop func(), err error) {
	out, err := os.Create(filepath.Join(dir, "bridge.log"))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("nice", append([]string{"-n", niceness, bin}, args...)...)
	cmd.Env = childEnv()
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		out.Close()
		return nil, err
	}
	exited := make(chan struct{})
	go func() { cmd.Wait(); out.Close(); close(exited) }()
	stop = func() {
		cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-exited:
		case <-time.After(40 * time.Second):
			cmd.Process.Kill()
			<-exited
		}
	}
	addr := args[1]
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/v1/health")
		if err == nil {
			resp.Body.Close()
			return stop, nil
		}
		select {
		case <-exited:
			return nil, fmt.Errorf("bridge exited at start; see %s", filepath.Join(dir, "bridge.log"))
		case <-ctx.Done():
			stop()
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			stop()
			return nil, errors.New("bridge did not come up in 30 s")
		}
	}
}

func (l *League) runGym(ctx context.Context, args []string, dir string) error {
	out, err := os.Create(filepath.Join(dir, "gym.log"))
	if err != nil {
		return err
	}
	defer out.Close()
	cmd := exec.Command("nice", append([]string{"-n", niceness, "dotnet"}, args...)...)
	cmd.Dir = filepath.Dir(args[0])
	cmd.Env = childEnv()
	if strings.HasSuffix(args[0], "fence-rollout.dll") {
		// It re-execs itself without W^X unless that is already off, and a re-exec'd child
		// would outlive a kill of its parent.
		cmd.Env = append(cmd.Env, "DOTNET_EnableWriteXorExecute=0")
	}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	select {
	case err := <-exited:
		if err != nil && !fileExists(filepath.Join(dir, "gym.summary.json")) {
			return fmt.Errorf("gym: %w; see %s", err, filepath.Join(dir, "gym.log"))
		}
		return nil
	case <-ctx.Done():
		// The whole group: fence-rollout's forked branches too.
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-exited
		return ctx.Err()
	}
}

func (l *League) finish(m Match, dir string, wall time.Duration) error {
	r, err := BuildResult(m, l.Spec, dir)
	if err != nil {
		return err
	}
	r.WallSeconds = round1(wall.Seconds())
	if err := writeJSON(filepath.Join(dir, resultFile), r); err != nil {
		return err
	}
	return recordFormat(m, r)
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// writeJSON writes atomically, so a result file is either whole or absent.
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
