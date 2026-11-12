// Command fenceshow is the stream's show layer: it turns a game (a finished league match,
// a recorded feed, or a running bridge) into the show feed, serves the OBS panel over it,
// and narrates the big moments with a local TTS voice. See docs/SHOW.md.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/gate"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/show"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fenceshow:", err)
		os.Exit(1)
	}
}

type config struct {
	replay, feed, liveMatch, liveLog, bridge string
	convert                                  bool
	days                                     string
	speed                                    float64
	listen, record, board, boardSeed         string
	personas, model, persona, playerID       string
	route, baseDirectives, prevRun           string
	format, formats, formatRecord            string
	defaultPersona                           string
	exitAfter                                time.Duration
	setup                                    string
	paced                                    bool
	recordedPace                             bool

	narrate                                            bool
	llmURL, llmModel, llmProvider, thinking            string
	anthropicURL, anthropicModel, anthropicKey         string
	tts, ttsURL, piperBin, piperModel, voice, audioDir string
	minGap, maxAge, maxLag, fillAfter, llmTimeout      time.Duration
	yieldToGame                                        bool
}

func parseFlags(args []string, getenv func(string) string, stderr io.Writer) (config, error) {
	env := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return def
	}
	home := getenv("HOME")
	var c config
	fs := flag.NewFlagSet("fenceshow", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.replay, "replay", "", "replay a league match directory (var/league/<league>/<match>)")
	fs.StringVar(&c.feed, "feed", "", "replay a recorded show feed (JSON lines)")
	fs.StringVar(&c.liveMatch, "live-match", "", "follow a running league match directory: its decision log and its bridge's plans")
	fs.StringVar(&c.liveLog, "live-log", "", "follow a bridge's -log-dir (the live game); plans come from -bridge")
	fs.StringVar(&c.bridge, "bridge", env("FENCE_SHOW_BRIDGE", "http://127.0.0.1:7341"), "bridge base URL for -live-log plans; empty: no plans")
	fs.BoolVar(&c.convert, "convert", false, "with -replay: write the whole feed as JSON lines to stdout and exit")
	fs.StringVar(&c.days, "days", "", "replay window FROM-TO (e.g. 38-46); the days before FROM still set the clock")
	fs.Float64Var(&c.speed, "speed", 1, "replay speed (2 = twice as fast)")
	fs.StringVar(&c.listen, "listen", env("FENCE_SHOW_LISTEN", "127.0.0.1:7350"), "panel server address; keep it on loopback")
	fs.StringVar(&c.record, "record", "", "append the feed (with narration) to this JSON-lines file")
	fs.StringVar(&c.board, "board", "var/show/board.json", "scoreboard file, kept across runs; empty: in memory")
	fs.StringVar(&c.boardSeed, "board-seed", "", "glob of finished match directories to put on the scoreboard")
	fs.StringVar(&c.personas, "personas", "docs/personas", "persona directory")
	fs.StringVar(&c.playerID, "player", "", "player id for -live-log (a league match names its own)")
	fs.StringVar(&c.model, "model", "", "model name for the name card when the source does not give one")
	fs.StringVar(&c.persona, "persona", "", "persona id for -live-log (a league match names its own)")
	fs.StringVar(&c.route, "route", "", "route name or file for -live-log, or for a -replay/-live-match whose match names none (a rehearsal's log dir)")
	fs.StringVar(&c.prevRun, "prev-run", "", "an earlier run on the same seed to race beside the route's book: a decision log, a match or rehearsal directory, or a gym summary")
	fs.StringVar(&c.baseDirectives, "base-directives", "", "for -live-log: the bridge's -base-directives file, for the standing plan's goals; empty: the -route's directives")
	fs.StringVar(&c.format, "format", "", "the run's stream format (docs/SHOW.md, \"Formats\"): its card and lab odds on the name card, its stakes for the narrator (a league match names its own)")
	fs.StringVar(&c.formats, "formats", format.DefaultDir, "format directory")
	fs.StringVar(&c.formatRecord, "format-record", format.DefaultRecord, "the local record of format attempts: the card reads it, and a -live-log run's end is appended")
	fs.StringVar(&c.defaultPersona, "default-persona", show.DefaultPersona, "persona for a run that has none, so the name card and the narrator have a character; empty: none")
	fs.DurationVar(&c.exitAfter, "exit-after", -1, "replay: exit this long after the last event (e.g. 20s); negative: keep serving")
	fs.StringVar(&c.setup, "setup", env("FENCE_SHOW_SETUP", ""), "the rig for the \"about this AI\" card, lines separated by ';' (e.g. \"Strategist: Qwen 27B, local;Voice: Kokoro\")")
	fs.BoolVar(&c.recordedPace, "recorded-pace", false, "replay: hold each event as long as the source did (decision-log or feed times, gaps capped at 3 min), divided by -speed: a rehearsal replays at stream pace")
	fs.BoolVar(&c.paced, "narration-paced", true, "replay: after each moment, wait (up to 60 s) for the narrator to finish it")

	fs.BoolVar(&c.narrate, "narrate", true, "narrate moments (captions at least)")
	fs.StringVar(&c.llmURL, "llm-url", env("FENCE_SHOW_LLM_URL", ""), "local narrator LLM base URL (OpenAI-compatible /v1)")
	fs.StringVar(&c.llmModel, "llm-model", env("FENCE_SHOW_LLM_MODEL", ""), "local narrator model")
	fs.StringVar(&c.llmProvider, "llm-provider", env("FENCE_SHOW_LLM_PROVIDER", "auto"), "auto (anthropic when ANTHROPIC_API_KEY is set, else -llm-url, else captions)|openai|anthropic|none")
	fs.StringVar(&c.anthropicURL, "anthropic-url", env("FENCE_SHOW_ANTHROPIC_URL", ""), "Anthropic API base URL; empty: "+llm.AnthropicDefaultBaseURL)
	fs.StringVar(&c.anthropicModel, "anthropic-model", env("FENCE_SHOW_ANTHROPIC_MODEL", defaultNarratorModel), "Anthropic narrator model")
	fs.StringVar(&c.thinking, "thinking", "off", "openai chat_template_kwargs.enable_thinking: on|off|empty")
	fs.StringVar(&c.tts, "tts", env("FENCE_SHOW_TTS", "auto"), "kokoro|piper|auto (kokoro, then piper)|none")
	fs.StringVar(&c.ttsURL, "tts-url", env("FENCE_SHOW_TTS_URL", "http://127.0.0.1:8880"), "Kokoro-FastAPI base URL")
	fs.StringVar(&c.piperBin, "piper", env("FENCE_SHOW_PIPER", filepath.Join(home, ".local/bin/piper")), "piper binary")
	fs.StringVar(&c.piperModel, "piper-model", env("FENCE_SHOW_PIPER_MODEL", filepath.Join(home, ".local/share/piper-voices/en_US-lessac-medium.onnx")), "piper voice model")
	fs.StringVar(&c.voice, "voice", "", "Kokoro voice; empty: the persona's (internal/show/tts.go)")
	fs.StringVar(&c.audioDir, "audio-dir", "var/show/audio", "where narration WAVs are written and served from")
	fs.DurationVar(&c.minGap, "min-gap", 8*time.Second, "silence between narrator lines")
	fs.DurationVar(&c.fillAfter, "fill-after", 60*time.Second, "after this much silence a notable event may be spoken too (0: moments only)")
	fs.DurationVar(&c.maxAge, "max-age", 15*time.Second, "never narrate a moment older than this")
	fs.DurationVar(&c.maxLag, "max-lag", 25*time.Second, "drop a line that is ready later than this after its moment")
	fs.DurationVar(&c.llmTimeout, "llm-timeout", 8*time.Second, "live: give the narrator's model this long for a line (queue included) before the caption is spoken; a replay paced by the narration gives it 30 s")
	fs.BoolVar(&c.yieldToGame, "yield-to-game", true, "no narrator LLM while Probably Stolen.exe runs (captions instead), when that LLM is served on this machine")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	c.anthropicKey = getenv("ANTHROPIC_API_KEY")
	n := 0
	for _, s := range []string{c.replay, c.feed, c.liveMatch, c.liveLog} {
		if s != "" {
			n++
		}
	}
	if n != 1 {
		return c, errors.New("give exactly one source: -replay, -feed, -live-match or -live-log")
	}
	if c.convert && c.replay == "" {
		return c, errors.New("-convert needs -replay")
	}
	return c, nil
}

func parseWindow(s string) (show.Window, error) {
	if s == "" {
		return show.Window{}, nil
	}
	a, b, ok := strings.Cut(s, "-")
	from, err1 := strconv.Atoi(a)
	to := 0
	var err2 error
	if ok && b != "" {
		to, err2 = strconv.Atoi(b)
	}
	if err1 != nil || err2 != nil || from < 0 || (to > 0 && to < from) {
		return show.Window{}, fmt.Errorf("-days %q: want FROM-TO", s)
	}
	return show.Window{From: from, To: to}, nil
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	c, err := parseFlags(args, getenv, stderr)
	if err != nil {
		return err
	}
	win, err := parseWindow(c.days)
	if err != nil {
		return err
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	var prev *show.PrevRun
	if c.prevRun != "" {
		if prev, err = show.LoadPrevRun(c.prevRun); err != nil {
			return fmt.Errorf("-prev-run: %w", err)
		}
	}

	var match *show.Match
	if dir := c.replay + c.liveMatch; dir != "" {
		if match, err = show.LoadMatch(dir, c.personas); err != nil {
			return err
		}
		if c.model != "" {
			match.Player.Model = c.model
		}
		id := match.Player.Persona
		if c.persona != "" {
			id = c.persona
		}
		match.Player.SetPersona(c.personas, id, c.defaultPersona)
		if match.Player.Route == "" {
			match.Player.SetRoute(c.route)
		}
		match.Player.PrevRun = prev
		if c.format != "" {
			if err := match.Player.SetFormat(c.formats, c.format, c.formatRecord, filepath.Base(filepath.Clean(dir))); err != nil {
				return err
			}
		}
	}
	if c.convert {
		f, err := os.Open(match.LogFile)
		if err != nil {
			return err
		}
		defer f.Close()
		enc := json.NewEncoder(stdout)
		return show.EachEvent(f, match.Builder(), win, func(ev show.Event) error { return enc.Encode(ev) })
	}

	var rec io.Writer
	if c.record != "" {
		if err := os.MkdirAll(filepath.Dir(c.record), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(c.record, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		rec = f
	}
	hub := show.NewHub(rec, c.board)
	defer hub.Close()
	if c.record != "" {
		if f, err := os.Open(c.record); err == nil {
			hub.ContinueFrom(show.LastSeq(f))
			f.Close()
		}
	}
	hub.SetSetup(splitSetup(c.setup))
	if c.boardSeed != "" {
		dirs, _ := filepath.Glob(c.boardSeed)
		var rows []show.BoardRow
		for _, r := range show.Board(dirs, c.personas) {
			// A replay must not show its own ending on the board before it gets there.
			if c.replay == "" || r.Run != filepath.Base(filepath.Clean(c.replay)) {
				rows = append(rows, r)
			}
		}
		hub.SeedBoard(rows)
	}

	audioDir := ""
	var gate show.Gate
	if c.narrate {
		audioDir = c.audioDir
		if err := os.MkdirAll(audioDir, 0o755); err != nil {
			return err
		}
		n, err := narrator(c, hub, log)
		if err != nil {
			return err
		}
		if c.record != "" && c.replay == "" && c.feed == "" {
			if f, err := os.Open(c.record); err == nil {
				n.Recall(f)
				f.Close()
			}
		}
		if c.paced && (c.replay != "" || c.feed != "") {
			// The replay waits for the voice, so a slow LLM or TTS costs pace, not lines.
			n.MaxAge, n.MaxLag, n.Timeout = 90*time.Second, 90*time.Second, 30*time.Second
			gate = func(ctx context.Context, ev show.Event) {
				if show.Priority(ev) >= show.Priority(show.Event{Kind: show.KindPlan}) {
					n.WaitIdle(ctx, ev.Seq, 60*time.Second)
				}
			}
		}
		evs, cancel := hub.Subscribe()
		defer cancel()
		go n.Run(ctx, evs)
	}

	ln, err := net.Listen("tcp", c.listen)
	if err != nil {
		return err
	}
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); !net.ParseIP(host).IsLoopback() {
		log.Warn("listening beyond loopback; /v1/ingest has no authentication", "addr", ln.Addr().String())
	}
	srv := &http.Server{Handler: show.Handler(hub, audioDir), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()
	log.Info("show panel", "url", "http://"+ln.Addr().String()+"/", "column", "/column", "lower", "/lower")

	toHub := func(ev show.Event) show.Event { return hub.Publish(ev) }
	publish := toHub
	if match != nil {
		publish = formatPublish(toHub, &match.Player, "", log)
	}
	pace := show.DefaultPace
	pace.Speed = c.speed
	pace.Recorded, pace.MaxHold = c.recordedPace, 3*time.Minute
	switch {
	case c.replay != "":
		// A freshly opened panel needs a moment to connect before the run starts.
		if show.Sleep(ctx, 2*time.Second) != nil {
			return nil
		}
		err = show.Replay(ctx, match, win, pace, gate, publish)
	case c.feed != "":
		f, ferr := os.Open(c.feed)
		if ferr != nil {
			return ferr
		}
		defer f.Close()
		if show.Sleep(ctx, 2*time.Second) != nil {
			return nil
		}
		err = show.PlayFeed(ctx, f, win, pace, gate, publish)
	default:
		l := &show.Live{Log: log, Bridge: c.bridge}
		if match != nil {
			l.LogDir, l.Player, l.Bridge, l.Base = filepath.Dir(match.LogFile), match.Player, "", match.Base
			if match.Listen != "" {
				l.Bridge = "http://" + match.Listen
			}
		} else {
			l.LogDir = c.liveLog
			l.Player = show.Player{ID: c.playerID, Model: c.model}
			l.Player.SetPersona(c.personas, c.persona, c.defaultPersona)
			l.Player.SetRoute(c.route)
			l.Player.PrevRun = prev
			if l.Base, err = show.BaseDirectives(c.route, c.baseDirectives); err != nil {
				return fmt.Errorf("base directives: %w", err)
			}
			if l.Player.ID == "" {
				l.Player.ID = "live"
			}
			if err := l.Player.SetFormat(c.formats, c.format, c.formatRecord, ""); err != nil {
				return err
			}
			if c.format != "" && c.baseDirectives == "" {
				l.Base, err = formatBase(c.formats, c.format, c.route)
				if err != nil {
					return err
				}
			}
		}
		record := ""
		if match == nil {
			record = c.formatRecord
		}
		pub := formatPublish(toHub, &l.Player, record, log)
		l.Publish = func(ev show.Event) { pub(ev) }
		return l.Run(ctx)
	}
	if err != nil && ctx.Err() == nil {
		return err
	}
	if c.exitAfter >= 0 {
		_ = show.Sleep(ctx, c.exitAfter)
		return nil
	}
	<-ctx.Done()
	return nil
}

func narrator(c config, hub *show.Hub, log *slog.Logger) (*show.Narrator, error) {
	n := &show.Narrator{Voice: c.voice, PersonaD: c.personas, Persona: c.defaultPersona, AudioDir: c.audioDir, MinGap: c.minGap, FillAfter: c.fillAfter, MaxAge: c.maxAge,
		MaxLag: c.maxLag, Timeout: c.llmTimeout, Log: log, Publish: hub.Publish}
	client, desc, err := narratorLLM(c)
	if err != nil {
		return nil, err
	}
	n.LLM = client
	if c.yieldToGame && sharesGPU(c) {
		var gaming atomic.Bool
		poll := func() { gaming.Store(show.GameRunning() && !behindRemoteGate(c.llmURL)) }
		poll()
		go func() {
			for range time.Tick(30 * time.Second) {
				poll()
			}
		}()
		n.LLMOff = gaming.Load
	}
	kokoro := &show.Kokoro{URL: c.ttsURL, HTTP: &http.Client{}}
	piper := &show.Piper{Bin: c.piperBin, Model: c.piperModel}
	switch c.tts {
	case "kokoro":
		n.TTS = kokoro
	case "piper":
		n.TTS = piper
	case "auto":
		n.TTS = show.Chain{kokoro}
		if _, err := os.Stat(c.piperModel); err == nil {
			n.TTS = show.Chain{kokoro, piper}
		}
	case "none":
	default:
		return nil, fmt.Errorf("-tts %q: want kokoro, piper, auto or none", c.tts)
	}
	log.Info("narrator", "llm", desc, "tts", c.tts, "voice", c.voice, "persona", c.defaultPersona)
	return n, nil
}

// defaultNarratorModel is small and fast: a line is ~600 prompt tokens and ~40 out.
const defaultNarratorModel = "claude-haiku-5-5"

// sharesGPU reports whether the narrator's model is served on this machine, next to the game: an
// OpenAI-compatible server on loopback. The API and a remote server never compete with the game, so
// the narrator keeps speaking in character while it runs.
func sharesGPU(c config) bool {
	p := c.llmProvider
	if p == "auto" && c.anthropicKey == "" && c.llmURL != "" {
		p = "openai"
	}
	if p != "openai" {
		return false
	}
	u, err := url.Parse(c.llmURL)
	if err != nil {
		return false
	}
	h := u.Hostname()
	ip := net.ParseIP(h)
	return h == "localhost" || (ip != nil && ip.IsLoopback())
}

// behindRemoteGate reports whether a loopback narrator URL is fencegate in front of another
// machine (drift): that model does not share the GPU with the game. Asked at each poll, since the
// gate may start after fenceshow.
func behindRemoteGate(llmURL string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st, ok := gate.Probe(ctx, http.DefaultClient, llmURL)
	return ok && !st.UpstreamLocal
}

// narratorLLM picks the narrator's model: the Anthropic API when asked for, or under auto
// when ANTHROPIC_API_KEY is set; else the local -llm-url; else none (captions).
func narratorLLM(c config) (llm.Client, string, error) {
	provider := c.llmProvider
	if provider == "auto" {
		switch {
		case c.anthropicKey != "":
			provider = "anthropic"
		case c.llmURL != "":
			provider = "openai"
		default:
			provider = "none"
		}
	}
	switch provider {
	case "none":
		return nil, "none (captions)", nil
	case "openai":
		if c.llmURL == "" {
			return nil, "", errors.New("-llm-provider openai needs -llm-url")
		}
		o := llm.OpenAIOptions{NoSchema: true}
		if c.thinking == "on" || c.thinking == "off" {
			on := c.thinking == "on"
			o.Thinking = &on
		}
		t := 0.7
		o.Temperature = &t
		return &llm.OpenAI{BaseURL: c.llmURL, Model: c.llmModel, APIKey: os.Getenv("OPENAI_API_KEY"), OpenAIOptions: o}, "local " + c.llmModel, nil
	case "anthropic":
		if c.anthropicKey == "" {
			return nil, "", errors.New("-llm-provider anthropic needs ANTHROPIC_API_KEY")
		}
		// No effort: the narrator writes one short line, and a small model may not take it.
		return &llm.Anthropic{BaseURL: c.anthropicURL, Model: c.anthropicModel, APIKey: c.anthropicKey}, "anthropic " + c.anthropicModel, nil
	}
	return nil, "", fmt.Errorf("-llm-provider %q: want auto, openai, anthropic or none", c.llmProvider)
}

func splitSetup(s string) []string {
	var out []string
	for _, l := range strings.Split(s, ";") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
