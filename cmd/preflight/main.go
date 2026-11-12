// Command preflight runs every automatable go-live check for a stream (docs/STREAM-OPS.md,
// "Pre-flight") and prints a go/no-go table. It exits 1 on any NO-GO.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/show"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	code, err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "preflight:", err)
		os.Exit(2)
	}
	os.Exit(code)
}

// Status is a check's verdict. Only NoGo stops the stream.
type Status string

const (
	Go   Status = "GO"
	Warn Status = "WARN"
	NoGo Status = "NO-GO"
	Skip Status = "SKIP"
)

type Result struct {
	Name   string
	Status Status
	Detail string
}

type config struct {
	repo, gameDir, gameData, backupDir, steamApps string
	backup, compat, soundcheck                    bool
	bridge, show                                  string
	stratProvider, stratURL, stratModel           string
	narrURL, narrModel, anthropicKey              string
	narrTest                                      bool
	// narrRemote: the narrator's URL is fencegate in front of another machine.
	narrRemote                               bool
	ttsURL, voice, persona, personas, wavOut string
	obsDirs                                  []string
	minFreeMB, minDiskMB                     int64
	timeout                                  time.Duration
	// hooks for tests
	runCompat func(ctx context.Context, c config) (int, string, error)
	gpuFree   func(ctx context.Context) (freeMB, totalMB int64, err error)
	gameUp    func() bool
	diskFree  func(path string) (int64, error)
}

// envFile reads KEY=VALUE lines (the bridge's ~/.config/fence/bridge.env); comments and blanks are skipped.
func envFile(path string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok {
			out[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return out
}

func parseFlags(args []string, getenv func(string) string, stderr io.Writer) (config, error) {
	home := getenv("HOME")
	bridgeEnv := envFile(filepath.Join(home, ".config/fence/bridge.env"))
	env := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		if v := bridgeEnv[k]; v != "" {
			return v
		}
		return def
	}
	steam := filepath.Join(home, ".local/share/Steam/steamapps")
	c := config{steamApps: steam}
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.repo, "repo", ".", "the fence checkout (compat/, mod/, docs/personas)")
	fs.StringVar(&c.gameDir, "game-dir", env("GAME_DIR", filepath.Join(steam, "common/Probably Stolen")), "the installed game")
	fs.StringVar(&c.gameData, "game-data", env("GAME_DATA", filepath.Join(steam, "compatdata/4348910/pfx/drive_c/users/steamuser/AppData/LocalLow/Questing Goose Studio/Probably Stolen")), "the game's saves and logs")
	fs.StringVar(&c.backupDir, "backup-dir", env("BACKUP_DIR", filepath.Join(home, ".local/share/fence/save-backups")), "where make backup-saves puts its snapshots")
	fs.BoolVar(&c.backup, "backup", false, "back the saves up now when the newest backup is older than the newest save")
	fs.BoolVar(&c.compat, "compat", true, "check every hook against the installed build (Fence.Compat --check-only; needs dotnet, ~30 s)")
	fs.BoolVar(&c.soundcheck, "soundcheck", false, "post a sound-check moment to fenceshow and wait for the narrator to speak it through the panel (shows on the panel: before going live only)")
	fs.StringVar(&c.bridge, "bridge", env("FENCE_BRIDGE", "http://127.0.0.1:7341"), "the bridge")
	fs.StringVar(&c.show, "show", env("FENCE_SHOW_URL", "http://127.0.0.1:7350"), "fenceshow's panel server")
	fs.StringVar(&c.stratProvider, "strategist-provider", env("FENCE_PROVIDER", "openai"), "the strategist's provider: openai|anthropic|none")
	fs.StringVar(&c.stratURL, "strategist-url", env("FENCE_BASE_URL", ""), "the strategist's OpenAI-compatible base URL (…/v1)")
	fs.StringVar(&c.stratModel, "strategist-model", env("FENCE_MODEL", ""), "the strategist's model")
	fs.StringVar(&c.narrURL, "narrator-url", env("FENCE_SHOW_LLM_URL", ""), "the narrator's OpenAI-compatible base URL; empty with ANTHROPIC_API_KEY set: the API")
	fs.StringVar(&c.narrModel, "narrator-model", env("FENCE_SHOW_LLM_MODEL", ""), "the narrator's model")
	fs.BoolVar(&c.narrTest, "narrator-test", true, "ask the narrator for one short test line")
	fs.StringVar(&c.ttsURL, "tts-url", env("FENCE_SHOW_TTS_URL", "http://127.0.0.1:8880"), "Kokoro-FastAPI")
	fs.StringVar(&c.persona, "persona", env("FENCE_PERSONA", show.DefaultPersona), "the run's persona: its file must exist and its voice speaks the test line")
	fs.StringVar(&c.voice, "voice", "", "Kokoro voice for the test line; empty: the persona's")
	fs.StringVar(&c.wavOut, "wav", "var/show/preflight.wav", "where the test line is written, to play through OBS")
	obs := fs.String("obs-config", "", "OBS config directory; empty: ~/.config/obs-studio and the Flatpak's")
	fs.Int64Var(&c.minFreeMB, "min-vram-mb", 3000, "free GPU memory below which GPU Kokoro does not fit")
	fs.Int64Var(&c.minDiskMB, "min-disk-mb", 4000, "free disk under var/ below which the recording may run out")
	fs.DurationVar(&c.timeout, "timeout", 20*time.Second, "per network check")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	c.anthropicKey = getenv("ANTHROPIC_API_KEY")
	if *obs != "" {
		c.obsDirs = []string{*obs}
	} else {
		c.obsDirs = []string{filepath.Join(home, ".config/obs-studio"), filepath.Join(home, ".var/app/com.obsproject.Studio/config/obs-studio")}
	}
	c.personas = filepath.Join(c.repo, "docs/personas")
	if !filepath.IsAbs(c.wavOut) {
		c.wavOut = filepath.Join(c.repo, c.wavOut)
	}
	return c, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) (int, error) {
	c, err := parseFlags(args, getenv, stderr)
	if err != nil {
		return 2, err
	}
	results := Checks(ctx, c)
	Print(stdout, results)
	if Verdict(results) == NoGo {
		return 1, nil
	}
	return 0, nil
}

// Checks runs every check in the order the runbook's pre-flight lists them.
func Checks(ctx context.Context, c config) []Result {
	if c.runCompat == nil {
		c.runCompat = dotnetCompat
	}
	if c.gpuFree == nil {
		c.gpuFree = nvidiaFree
	}
	if c.gameUp == nil {
		c.gameUp = show.GameRunning
	}
	if c.diskFree == nil {
		c.diskFree = diskFree
	}
	hc := &http.Client{Timeout: c.timeout}
	g := probeGates(ctx, hc, c)
	c.narrRemote = g.narrRemote()
	build, out := checkBuild(c)
	results := []Result{out}
	results = append(results,
		checkCompat(ctx, c, build),
		checkMod(c),
		checkBackup(c),
		checkBridge(ctx, hc, c),
		checkStrategist(ctx, hc, c),
		checkNarrator(ctx, hc, c),
	)
	if r, ok := checkGate(c, g); ok {
		results = append(results, r)
	}
	results = append(results,
		checkTTS(ctx, c),
		checkShow(ctx, hc, c),
		checkSoundcheck(ctx, hc, c),
		checkOBS(c),
		checkGPU(ctx, c),
		checkGame(c),
		checkDisk(c),
	)
	return results
}

// Verdict is NO-GO when any check is, else GO.
func Verdict(rs []Result) Status {
	for _, r := range rs {
		if r.Status == NoGo {
			return NoGo
		}
	}
	return Go
}

func Print(w io.Writer, rs []Result) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CHECK\tRESULT\tDETAIL")
	warns := 0
	for _, r := range rs {
		if r.Status == Warn {
			warns++
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Name, r.Status, r.Detail)
	}
	tw.Flush()
	v := Verdict(rs)
	switch {
	case v == NoGo:
		fmt.Fprintln(w, "\nVERDICT: NO-GO. Fix every NO-GO row (docs/STREAM-OPS.md, \"Pre-flight\") and run it again.")
	case warns > 0:
		fmt.Fprintf(w, "\nVERDICT: GO with %d warning(s). Read each WARN row before going live.\n", warns)
	default:
		fmt.Fprintln(w, "\nVERDICT: GO.")
	}
}

var buildRe = regexp.MustCompile(`"buildid"\s+"(\d+)"`)

// checkBuild: the installed build must be one compat/versions has a snapshot of; the mod embeds
// that directory to recognise verified builds.
func checkBuild(c config) (string, Result) {
	r := Result{Name: "game build"}
	if _, err := os.Stat(filepath.Join(c.gameDir, "Probably Stolen_Data/Managed/Assembly-CSharp.dll")); err != nil {
		r.Status, r.Detail = NoGo, "no game at "+c.gameDir+" (set GAME_DIR)"
		return "", r
	}
	b, err := os.ReadFile(filepath.Join(c.steamApps, "appmanifest_4348910.acf"))
	m := buildRe.FindSubmatch(b)
	if err != nil || m == nil {
		r.Status, r.Detail = Warn, "cannot read the Steam build id; the compat check below still runs"
		return "", r
	}
	build := string(m[1])
	snap := filepath.Join(c.repo, "compat/versions", build+".json")
	if _, err := os.Stat(snap); err != nil {
		r.Status, r.Detail = NoGo, "build "+build+" has no verified snapshot: the game patched. Run `make compat`, then `make mod-install` (runbook: Recovery, game patch)"
		return build, r
	}
	r.Status, r.Detail = Go, "build "+build+" verified ("+filepath.Join("compat/versions", build+".json")+")"
	return build, r
}

func dotnetCompat(ctx context.Context, c config) (int, string, error) {
	if _, err := exec.LookPath("dotnet"); err != nil {
		return 0, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "dotnet", "run", "--project", "mod/tools/Fence.Compat", "-c", "Release", "--", "--check-only", "--game-dir", c.gameDir)
	cmd.Dir = c.repo
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), string(out), nil
	}
	if err != nil {
		return 0, string(out), err
	}
	return 0, string(out), nil
}

// checkCompat runs `make compat`'s check without writing a snapshot: every required hook must
// resolve in the installed build.
func checkCompat(ctx context.Context, c config, build string) Result {
	r := Result{Name: "compat hooks"}
	if !c.compat {
		r.Status, r.Detail = Skip, "-compat=false"
		return r
	}
	code, out, err := c.runCompat(ctx, c)
	last := lastLine(out)
	switch {
	case err != nil:
		r.Status, r.Detail = Warn, "could not run Fence.Compat ("+err.Error()+"); run `make compat` by hand"
	case code == 0:
		r.Status, r.Detail = Go, "every required hook resolves"
		if build != "" {
			r.Detail += " in build " + build
		}
	case code == 1:
		r.Status, r.Detail = NoGo, "a required hook is missing: "+last+" (runbook: Recovery, game patch)"
	default:
		r.Status, r.Detail = NoGo, fmt.Sprintf("Fence.Compat exit %d: %s", code, last)
	}
	return r
}

func lastLine(s string) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	return clip(strings.TrimSpace(ls[len(ls)-1]), 140)
}

// checkMod: the installed mod is the one built from this checkout.
func checkMod(c config) Result {
	r := Result{Name: "mod installed"}
	dest := filepath.Join(c.gameDir, "Mods/fence")
	inst, err := fileSum(filepath.Join(dest, "Fence.Mod.dll"))
	if err != nil {
		r.Status, r.Detail = NoGo, "no fence mod in "+dest+": `make mod-install`"
		return r
	}
	built, err := fileSum(filepath.Join(c.repo, "mod/src/Fence.Mod/bin/Release/netstandard2.1/Fence.Mod.dll"))
	switch {
	case err != nil:
		r.Status, r.Detail = Warn, "installed, but this checkout has no Release build to compare: `make mod-install` builds and installs"
	case built != inst:
		r.Status, r.Detail = Warn, "the installed Fence.Mod.dll differs from this checkout's build: `make mod-install` if that is not on purpose"
	default:
		r.Status, r.Detail = Go, "Fence.Mod.dll matches this checkout's Release build"
	}
	return r
}

func fileSum(p string) ([32]byte, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(b), nil
}

// checkBackup: a save backup newer than the newest save, or one made now with -backup.
func checkBackup(c config) Result {
	r := Result{Name: "save backup"}
	saves, _ := filepath.Glob(filepath.Join(c.gameData, "*.es3"))
	if len(saves) == 0 {
		r.Status, r.Detail = Warn, "no saves found in the game's data folder (a new game?)"
		return r
	}
	var newest time.Time
	for _, s := range saves {
		if fi, err := os.Stat(s); err == nil && fi.ModTime().After(newest) {
			newest = fi.ModTime()
		}
	}
	last, name := newestBackup(c.backupDir)
	if !last.Before(newest) {
		r.Status, r.Detail = Go, fmt.Sprintf("%s holds the %d save(s) as they are now", filepath.Join(c.backupDir, name), len(saves))
		return r
	}
	if c.backup {
		dest := filepath.Join(c.backupDir, time.Now().Format("20060102-150405"))
		if err := copySaves(saves, dest); err != nil {
			r.Status, r.Detail = NoGo, "backup failed: "+err.Error()
			return r
		}
		r.Status, r.Detail = Go, fmt.Sprintf("backed up %d save(s) to %s", len(saves), dest)
		return r
	}
	when := "never"
	if !last.IsZero() {
		when = last.Format("2006-01-02 15:04")
	}
	r.Status, r.Detail = NoGo, "the saves changed since the last backup ("+when+"): `make backup-saves`, or run with -backup"
	return r
}

func newestBackup(dir string) (time.Time, string) {
	var t time.Time
	var name string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(dir, e.Name(), "*.es3"))
		for _, f := range files {
			if fi, err := os.Stat(f); err == nil && fi.ModTime().After(t) {
				t, name = fi.ModTime(), e.Name()
			}
		}
	}
	return t, name
}

// copySaves keeps each save's modification time, as `cp -p` does in scripts/backup-saves.sh.
func copySaves(saves []string, dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, s := range saves {
		b, err := os.ReadFile(s)
		if err != nil {
			return err
		}
		to := filepath.Join(dest, filepath.Base(s))
		if err := os.WriteFile(to, b, 0o644); err != nil {
			return err
		}
		if fi, err := os.Stat(s); err == nil {
			_ = os.Chtimes(to, fi.ModTime(), fi.ModTime())
		}
	}
	return nil
}

func getJSON(ctx context.Context, hc *http.Client, u string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func checkBridge(ctx context.Context, hc *http.Client, c config) Result {
	r := Result{Name: "bridge"}
	var h struct {
		OK       bool `json:"ok"`
		Protocol int  `json:"protocol"`
	}
	code, err := getJSON(ctx, hc, strings.TrimRight(c.bridge, "/")+"/v1/health", &h)
	if err != nil || code != http.StatusOK || !h.OK {
		r.Status, r.Detail = NoGo, fmt.Sprintf("%s: %s; start it (`make bridge …` or `systemctl --user start fence-bridge`)", hostOf(c.bridge), errOr(err, code))
		return r
	}
	r.Status, r.Detail = Go, fmt.Sprintf("%s healthy, protocol %d", hostOf(c.bridge), h.Protocol)
	return r
}

func errOr(err error, code int) string {
	if err != nil {
		return clip(err.Error(), 100)
	}
	return "HTTP " + strconv.Itoa(code)
}

// modelState finds model among an OpenAI-compatible /models listing; llama-swap adds a status.
func modelState(ctx context.Context, hc *http.Client, base, model string) (found bool, status string, err error) {
	var l struct {
		Data []struct {
			ID     string `json:"id"`
			Status *struct {
				Value string `json:"value"`
			} `json:"status"`
		} `json:"data"`
	}
	code, err := getJSON(ctx, hc, strings.TrimRight(base, "/")+"/models", &l)
	if err != nil || code != http.StatusOK {
		return false, "", errors.New(errOr(err, code))
	}
	for _, d := range l.Data {
		if d.ID == model || strings.HasPrefix(d.ID, model+"/") || strings.HasSuffix(d.ID, "/"+model) {
			if d.Status != nil {
				status = d.Status.Value
			}
			return true, status, nil
		}
	}
	return false, "", nil
}

// checkStrategist: the endpoint answers and lists the model. It never asks for a completion: on a
// shared llama-swap that could load a second model.
func checkStrategist(ctx context.Context, hc *http.Client, c config) Result {
	r := Result{Name: "strategist"}
	switch c.stratProvider {
	case "none", "":
		r.Status, r.Detail = Warn, "no strategist: the day plays on the base directives (`make bridge ARGS=\"-planner llm …\"`)"
		return r
	case "anthropic":
		if c.anthropicKey == "" {
			r.Status, r.Detail = NoGo, "anthropic strategist without ANTHROPIC_API_KEY"
			return r
		}
		r.Status, r.Detail = Go, "Anthropic API, key set (model "+orDefault(c.stratModel, llm.AnthropicDefaultModel)+")"
		return r
	}
	if c.stratURL == "" || c.stratModel == "" {
		r.Status, r.Detail = Warn, "no -strategist-url/-strategist-model (nor FENCE_BASE_URL/FENCE_MODEL): not checked"
		return r
	}
	found, status, err := modelState(ctx, hc, c.stratURL, c.stratModel)
	host := hostOf(c.stratURL)
	switch {
	case err != nil:
		r.Status, r.Detail = NoGo, host+" does not answer ("+err.Error()+"): runbook, Recovery, model endpoint down"
	case !found:
		r.Status, r.Detail = NoGo, host+" does not serve "+c.stratModel
	default:
		r.Status, r.Detail = Go, c.stratModel+" listed at "+host+withStatus(status)
	}
	return r
}

func withStatus(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// hostOf names an endpoint without its address: the table may be shown on stream.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "the endpoint"
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() || u.Hostname() == "localhost" {
		return "localhost:" + u.Port()
	}
	return "the remote endpoint"
}

// checkNarrator: the narrator's model answers one short line, unless asking would load a model on
// this machine's shared server (SHOW.md, "Sharing the GPU").
func checkNarrator(ctx context.Context, hc *http.Client, c config) Result {
	r := Result{Name: "narrator LLM"}
	var client llm.Client
	desc := ""
	switch {
	case c.narrURL == "" && c.anthropicKey != "":
		client, desc = &llm.Anthropic{APIKey: c.anthropicKey, Model: "claude-haiku-5-5", HTTP: hc}, "Anthropic API"
	case c.narrURL == "":
		r.Status, r.Detail = Warn, "none configured: the narrator speaks the captions (set FENCE_SHOW_LLM_URL or ANTHROPIC_API_KEY)"
		return r
	default:
		found, status, err := modelState(ctx, hc, c.narrURL, c.narrModel)
		host := hostOf(c.narrURL)
		switch {
		case err != nil:
			r.Status, r.Detail = Warn, host+" does not answer ("+err.Error()+"): the narrator falls back to captions"
			return r
		case !found:
			r.Status, r.Detail = Warn, host+" does not serve "+c.narrModel+": captions only"
			return r
		case status == "unloaded" && strings.HasPrefix(host, "localhost") && !c.narrRemote:
			r.Status, r.Detail = Warn, c.narrModel+" is listed but not loaded on this machine: a line would load it next to the strategist"
			return r
		}
		// As fenceshow asks (its -thinking off): a thinking model spends the 40 tokens thinking.
		t, thinking := 0.7, false
		client, desc = &llm.OpenAI{BaseURL: c.narrURL, Model: c.narrModel, HTTP: hc, OpenAIOptions: llm.OpenAIOptions{NoSchema: true, Temperature: &t, Thinking: &thinking}}, c.narrModel+" at "+host
	}
	if !c.narrTest {
		r.Status, r.Detail = Go, desc+" (not asked: -narrator-test=false)"
		return r
	}
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	resp, err := client.Complete(cctx, llm.Request{System: "You are a pawn shop owner on a live stream.", MaxTokens: 40,
		Messages: []llm.Message{{Role: llm.RoleUser, Content: "Say hello to the viewers in one short sentence."}}})
	took := time.Since(start).Round(100 * time.Millisecond)
	switch {
	case err != nil:
		r.Status, r.Detail = Warn, desc+": "+clip(err.Error(), 100)+": captions only"
	case strings.TrimSpace(resp.Text) == "":
		r.Status, r.Detail = Warn, desc+" answered nothing"
	case took > 8*time.Second:
		r.Status, r.Detail = Warn, fmt.Sprintf("%s answered in %v: over the live timeout (8 s), lines will fall back to captions", desc, took)
	default:
		r.Status, r.Detail = Go, fmt.Sprintf("%s answered in %v", desc, took)
	}
	return r
}

// checkTTS speaks the test line in the persona's voice and saves it to play through OBS.
func checkTTS(ctx context.Context, c config) Result {
	r := Result{Name: "TTS test line"}
	voice := c.voice
	if voice == "" {
		voice = show.PersonaVoice(c.persona)
	}
	k := &show.Kokoro{URL: c.ttsURL, HTTP: &http.Client{Timeout: 90 * time.Second}}
	start := time.Now()
	wav, err := k.Speak(ctx, "Pre-flight check. The narrator is on the air.", voice)
	took := time.Since(start).Round(100 * time.Millisecond)
	if err != nil {
		r.Status, r.Detail = Warn, hostOf(c.ttsURL)+": "+clip(err.Error(), 100)+": piper or captions only (runbook: TTS)"
		return r
	}
	_ = os.MkdirAll(filepath.Dir(c.wavOut), 0o755)
	_ = os.WriteFile(c.wavOut, wav, 0o644)
	r.Status, r.Detail = Go, fmt.Sprintf("voice %s in %v; play %s through OBS to check the audio route", voice, took, relTo(c.repo, c.wavOut))
	if took > 3*time.Second {
		r.Status = Warn
		r.Detail = fmt.Sprintf("voice %s took %v a line (CPU Kokoro?): the narrator will say less; GPU Kokoro if VRAM allows. Saved to %s", voice, took, relTo(c.repo, c.wavOut))
	}
	if _, err := os.Stat(filepath.Join(c.personas, c.persona+".md")); err != nil {
		r.Status, r.Detail = NoGo, "persona "+c.persona+" has no file in docs/personas"
	}
	return r
}

func relTo(base, p string) string {
	if rel, err := filepath.Rel(base, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

func checkShow(ctx context.Context, hc *http.Client, c config) Result {
	r := Result{Name: "fenceshow panel"}
	var s show.State
	code, err := getJSON(ctx, hc, strings.TrimRight(c.show, "/")+"/state", &s)
	if err != nil || code != http.StatusOK {
		r.Status, r.Detail = NoGo, fmt.Sprintf("%s: %s; start fenceshow -live-log …", hostOf(c.show), errOr(err, code))
		return r
	}
	for _, p := range []string{"/", "/column", "/lower", "/about"} {
		if code, err := getJSON(ctx, hc, strings.TrimRight(c.show, "/")+p, nil); err != nil || code != http.StatusOK {
			r.Status, r.Detail = NoGo, "panel page "+p+": "+errOr(err, code)
			return r
		}
	}
	r.Status = Go
	switch {
	case s.Player != nil:
		r.Detail = fmt.Sprintf("serving; on screen: %s, day %d", orDefault(s.Player.Model, s.Player.ID), s.Day)
	default:
		r.Detail = "serving; waiting for the game's first event"
	}
	if len(s.Setup) == 0 {
		r.Status, r.Detail = Warn, r.Detail+"; no -setup lines for the \"about this AI\" card"
	}
	return r
}

// checkSoundcheck has fenceshow's own narrator speak a moment, with the panel's real LLM and TTS
// settings: the line then plays from the browser source, so the OBS mixer should move.
func checkSoundcheck(ctx context.Context, hc *http.Client, c config) Result {
	r := Result{Name: "panel soundcheck"}
	if !c.soundcheck {
		r.Status, r.Detail = Skip, "-soundcheck to have the panel's narrator speak a test moment"
		return r
	}
	base := strings.TrimRight(c.show, "/")
	start := time.Now().UnixMilli()
	body := `{"kind":"thought","level":2,"text":"Sound check: the narrator is on the air."}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/ingest", strings.NewReader(body))
	if err != nil {
		r.Status, r.Detail = NoGo, err.Error()
		return r
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		code := 0
		if resp != nil {
			code = resp.StatusCode
			resp.Body.Close()
		}
		r.Status, r.Detail = NoGo, "ingest: "+errOr(err, code)
		return r
	}
	resp.Body.Close()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		var s show.State
		if code, err := getJSON(ctx, hc, base+"/state", &s); err == nil && code == http.StatusOK && s.Say != nil && s.Say.Say != nil && s.Say.At >= start {
			say := s.Say.Say
			took := time.Duration(s.Say.At-start) * time.Millisecond
			switch {
			case say.Audio == "":
				r.Status, r.Detail = Warn, fmt.Sprintf("spoken as a caption only (%s), no audio: fenceshow's TTS is down or -tts none", say.Source)
			case say.Source != "llm":
				r.Status, r.Detail = Warn, fmt.Sprintf("audio in %v, but from the caption: the panel's narrator LLM is off or failing", took.Round(100*time.Millisecond))
			default:
				r.Status, r.Detail = Go, fmt.Sprintf("narrator line with audio in %v (voice %s): check the browser source moves in the OBS mixer", took.Round(100*time.Millisecond), say.Voice)
			}
			return r
		}
		if sleep(ctx, 500*time.Millisecond) != nil {
			break
		}
	}
	r.Status, r.Detail = NoGo, "the panel's narrator said nothing within 90 s (fenceshow -narrate=false? a moment narrated just before?)"
	return r
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// obsScene is the part of an OBS scene collection the check reads.
type obsScene struct {
	Name    string `json:"name"`
	Sources []struct {
		Name     string         `json:"name"`
		ID       string         `json:"id"`
		Settings map[string]any `json:"settings"`
	} `json:"sources"`
}

var gameCaptureIDs = []string{"pipewire-screen-capture-source", "pipewire-window-capture-source", "xcomposite_input", "game_capture", "window_capture"}

// checkOBS reads OBS's scene collections: a browser source on the panel with its audio routed
// through OBS, and a game capture. It cannot see whether OBS is live; that stays manual.
func checkOBS(c config) Result {
	r := Result{Name: "OBS scene"}
	var files []string
	for _, d := range c.obsDirs {
		fs, _ := filepath.Glob(filepath.Join(d, "basic/scenes/*.json"))
		files = append(files, fs...)
	}
	if len(files) == 0 {
		r.Status, r.Detail = Warn, "no OBS scene collection on this machine: check the scene by hand on the streaming machine (runbook: OBS)"
		return r
	}
	panelHost := c.show
	if u, err := url.Parse(c.show); err == nil && u.Host != "" {
		panelHost = u.Host
	}
	var problems []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var sc obsScene
		if json.Unmarshal(b, &sc) != nil {
			continue
		}
		var browser, audio, game bool
		for _, s := range sc.Sources {
			switch {
			case s.ID == "browser_source":
				u, _ := s.Settings["url"].(string)
				if strings.Contains(u, panelHost) {
					browser = true
					if v, _ := s.Settings["reroute_audio"].(bool); v {
						audio = true
					}
				}
			case slices.Contains(gameCaptureIDs, s.ID):
				game = true
			}
		}
		if browser && game && audio {
			r.Status, r.Detail = Go, fmt.Sprintf("collection %q: game capture + panel browser source, audio through OBS", sc.Name)
			return r
		}
		var miss []string
		if !game {
			miss = append(miss, "no game capture")
		}
		if !browser {
			miss = append(miss, "no browser source on the panel ("+hostOf(c.show)+")")
		} else if !audio {
			miss = append(miss, "the panel's browser source does not route audio through OBS (no narration on stream)")
		}
		problems = append(problems, fmt.Sprintf("%q: %s", sc.Name, strings.Join(miss, ", ")))
	}
	r.Status, r.Detail = Warn, strings.Join(problems, "; ")
	return r
}

func nvidiaFree(ctx context.Context) (int64, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=memory.used,memory.total", "--format=csv,noheader,nounits").Output()
	if err != nil {
		return 0, 0, err
	}
	f := strings.Split(strings.TrimSpace(strings.Split(string(out), "\n")[0]), ",")
	if len(f) != 2 {
		return 0, 0, fmt.Errorf("nvidia-smi: %q", out)
	}
	used, err1 := strconv.ParseInt(strings.TrimSpace(f[0]), 10, 64)
	total, err2 := strconv.ParseInt(strings.TrimSpace(f[1]), 10, 64)
	if err := errors.Join(err1, err2); err != nil {
		return 0, 0, err
	}
	return total - used, total, nil
}

// checkGPU: room for GPU Kokoro next to the strategist's model (docs/STREAM-OPS.md, the GPU budget).
func checkGPU(ctx context.Context, c config) Result {
	r := Result{Name: "GPU memory"}
	free, total, err := c.gpuFree(ctx)
	if err != nil {
		r.Status, r.Detail = Skip, "no nvidia-smi"
		return r
	}
	r.Detail = fmt.Sprintf("%d of %d MiB free", free, total)
	if free < c.minFreeMB {
		r.Status = Warn
		r.Detail += fmt.Sprintf(": under %d, no room for GPU Kokoro; run CPU Kokoro (port 8881)", c.minFreeMB)
		return r
	}
	r.Status = Go
	return r
}

func checkGame(c config) Result {
	r := Result{Name: "game process", Status: Go}
	if c.gameUp() {
		r.Detail = "Probably Stolen.exe is running: fenceshow's narrator yields a local LLM to it (-yield-to-game)"
		if hostOf(c.narrURL) != "the remote endpoint" && !c.narrRemote && c.anthropicKey == "" && c.narrURL != "" {
			r.Status = Warn
			r.Detail += ", so a local narrator speaks captions only; use the API or a remote model, or -yield-to-game=false"
		}
		return r
	}
	r.Detail = "not running yet (start it after this table is GO)"
	return r
}

func diskFree(path string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize) >> 20, nil
}

func checkDisk(c config) Result {
	r := Result{Name: "disk"}
	dir := filepath.Join(c.repo, "var")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		dir = c.repo
	}
	mb, err := c.diskFree(dir)
	if err != nil {
		r.Status, r.Detail = Skip, err.Error()
		return r
	}
	r.Detail = fmt.Sprintf("%d MiB free under %s", mb, relTo(c.repo, dir))
	r.Status = Go
	if mb < c.minDiskMB {
		r.Status = NoGo
		r.Detail += fmt.Sprintf(": under %d; the feed, decision log and narration WAVs need room", c.minDiskMB)
	}
	return r
}

func clip(s string, n int) string {
	s = string(bytes.TrimSpace([]byte(s)))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
