// Command bridge is the Go sidecar the fence mod calls at each decision point.
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
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/format"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/memory"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/route"
	"github.com/gallowaysoftware/fence/internal/server"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Getenv, os.Stderr, nil); err != nil {
		fmt.Fprintln(os.Stderr, "bridge:", err)
		os.Exit(1)
	}
}

type config struct {
	listen, mode, policy, provider, baseURL, model, logDir, effort string
	fallbacks                                                      bool
	maxTokens                                                      int

	thinking, temperature string
	noSchema              bool

	memory, carryLessons, reflect bool
	memoryDB                      string

	persona string
	// exemplars is an index from fenceexemplars; "" shows the plan none.
	exemplars string
	// directives is full (every field) or delta (changes to the executor's defaults).
	directives string
	// bets lets each delta plan stake one checkable bet (docs/PROTOCOL.md, "Bets").
	bets bool
	// baseDirectives is a directives file the gym plays over its defaults (gym -directives).
	baseDirectives string
	// route is a docs/research/ROUTES.md route by name or path.
	route string
	// format is a stream format (internal/format) by name: handicaps over the route.
	format string
	// chatTally is fencetwitch's chat vote file: its fact goes to the morning plan it is for.
	chatTally string

	planner     string
	plannerWait time.Duration

	// The strategist's agency (docs/STREAM.md, "Model agency"): the day-0 choice file, the
	// forks, and the plan cadence.
	choice    string
	forks     bool
	planEvery int
}

func parseFlags(args []string, getenv func(string) string, stderr io.Writer) (config, error) {
	env := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}
	var c config
	fs := flag.NewFlagSet("bridge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.listen, "listen", env("FENCE_LISTEN", "127.0.0.1:7341"), "listen address (env FENCE_LISTEN); keep it on loopback")
	fs.StringVar(&c.mode, "mode", env("FENCE_MODE", string(protocol.ModeObserve)), "observe|advise|act (env FENCE_MODE)")
	fs.StringVar(&c.policy, "policy", env("FENCE_POLICY", "heuristic"), "heuristic|llm (env FENCE_POLICY)")
	fs.StringVar(&c.provider, "provider", env("FENCE_PROVIDER", "openai"), "openai|anthropic, for -policy llm (env FENCE_PROVIDER)")
	fs.StringVar(&c.baseURL, "base-url", env("FENCE_BASE_URL", ""), "provider base URL, e.g. http://localhost:8080/v1 for openai (env FENCE_BASE_URL)")
	fs.StringVar(&c.model, "model", env("FENCE_MODEL", ""), "model name; anthropic defaults to "+llm.AnthropicDefaultModel+" (env FENCE_MODEL)")
	fs.StringVar(&c.logDir, "log-dir", env("FENCE_LOG_DIR", "var/log"), "decision log directory (env FENCE_LOG_DIR)")
	fs.StringVar(&c.effort, "effort", env("FENCE_EFFORT", "low"), "anthropic output_config.effort (env FENCE_EFFORT)")
	fs.BoolVar(&c.fallbacks, "anthropic-fallbacks", env("FENCE_ANTHROPIC_FALLBACKS", "true") == "true", "anthropic server-side refusal fallback (env FENCE_ANTHROPIC_FALLBACKS)")
	fs.IntVar(&c.maxTokens, "max-tokens", policy.DefaultMaxTokens, "max output tokens per model call")
	fs.StringVar(&c.thinking, "thinking", env("FENCE_THINKING", ""), "on|off: openai chat_template_kwargs.enable_thinking; empty leaves the server default (env FENCE_THINKING)")
	fs.StringVar(&c.temperature, "temperature", env("FENCE_TEMPERATURE", ""), "openai sampling temperature; empty leaves the server default (env FENCE_TEMPERATURE)")
	fs.BoolVar(&c.noSchema, "no-schema", env("FENCE_NO_SCHEMA", "false") == "true", "openai: never send response_format, for servers that reject it (env FENCE_NO_SCHEMA)")
	fs.BoolVar(&c.memory, "memory", env("FENCE_MEMORY", "true") == "true", "record play and give the model a price ledger, history and lessons (env FENCE_MEMORY)")
	fs.StringVar(&c.memoryDB, "memory-db", env("FENCE_MEMORY_DB", "var/memory.db"), "memory SQLite database (env FENCE_MEMORY_DB)")
	fs.BoolVar(&c.carryLessons, "memory-carry-lessons", env("FENCE_MEMORY_CARRY_LESSONS", "true") == "true", "start a new run with the previous run's lessons (env FENCE_MEMORY_CARRY_LESSONS)")
	fs.BoolVar(&c.reflect, "memory-reflect", env("FENCE_MEMORY_REFLECT", "true") == "true", "rewrite the lessons with the model at the end of each day (env FENCE_MEMORY_REFLECT)")
	fs.StringVar(&c.exemplars, "exemplars", env("FENCE_EXEMPLARS", ""), "strong-play index (fenceexemplars) whose nearest days the morning plan is shown; empty: off (env FENCE_EXEMPLARS)")
	fs.StringVar(&c.directives, "directives", env("FENCE_DIRECTIVES", "delta"), "full: the plan as every directive; delta: only changes to the executor's defaults (env FENCE_DIRECTIVES)")
	fs.BoolVar(&c.bets, "bets", env("FENCE_BETS", "false") == "true", "let each delta plan stake one checkable bet on the run, for the show (env FENCE_BETS)")
	fs.StringVar(&c.baseDirectives, "base-directives", env("FENCE_BASE_DIRECTIVES", ""), "the directives file the gym plays over its defaults (its -directives), shown to delta plans; empty: none (env FENCE_BASE_DIRECTIVES)")
	fs.StringVar(&c.format, "format", env("FENCE_FORMAT", ""), "the run's stream format ("+format.DefaultDir+" name): its handicaps are laid over the route's directives and locked, and the plans are told the format (env FENCE_FORMAT)")
	fs.StringVar(&c.route, "route", env("FENCE_ROUTE", ""), "the run's route ("+route.DefaultDir+" name or a file path): the morning plans are written as that shop, and its directives are the base unless -base-directives is set (env FENCE_ROUTE)")
	fs.StringVar(&c.persona, "persona", env("FENCE_PERSONA", ""), "play style for the morning plans: an id in "+strategy.DefaultPersonaDir+" or a file path (env FENCE_PERSONA)")
	fs.StringVar(&c.planner, "planner", env("FENCE_PLANNER", ""), "llm: the model writes the morning plans under any -policy, so -policy heuristic follows LLM plans; empty: plans only with -policy llm (env FENCE_PLANNER)")
	fs.StringVar(&c.choice, "choice", "", "the day-0 route choice (fenceleague's choice.json): its thesis goes to the feed and the plans")
	fs.BoolVar(&c.forks, "forks", false, "put the run's high-stakes decisions (mortgage, cash sprint, fountain, engines, trading style, holding through an event) to the model as forks")
	fs.IntVar(&c.planEvery, "plan-every", 1, "write the morning plan every N days and hold it, with the forks, in between")
	fs.DurationVar(&c.plannerWait, "planner-wait", 0, "hold a day's first decision up to this long for that day's plan; for the gym, which outruns the model")
	fs.StringVar(&c.chatTally, "chat-tally", env("FENCE_CHAT_TALLY", ""), "fencetwitch's chat vote (-tally): the morning plan it is for reads its tally as one fact; empty: none (env FENCE_CHAT_TALLY)")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected arguments %v", fs.Args())
	}
	return c, nil
}

// buildClient returns the model client for -policy llm, nil for the heuristic, and
// whether the provider accepts numeric bounds in the output schema.
func buildClient(c config, getenv func(string) string) (llm.Client, bool, error) {
	switch c.planner {
	case "", "llm":
	default:
		return nil, false, fmt.Errorf("unknown planner %q (want llm or empty)", c.planner)
	}
	switch c.policy {
	case "heuristic":
		if c.planner != "llm" {
			return nil, false, nil
		}
	case "llm":
	default:
		return nil, false, fmt.Errorf("unknown policy %q (want heuristic or llm)", c.policy)
	}
	switch c.provider {
	case "openai":
		if c.baseURL == "" || c.model == "" {
			return nil, false, errors.New("-provider openai needs -base-url and -model")
		}
		o, err := openAIOptions(c)
		if err != nil {
			return nil, false, err
		}
		return &llm.OpenAI{BaseURL: c.baseURL, Model: c.model, APIKey: getenv("OPENAI_API_KEY"), OpenAIOptions: o}, true, nil
	case "anthropic":
		key := getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, false, errors.New("-provider anthropic needs ANTHROPIC_API_KEY")
		}
		return &llm.Anthropic{BaseURL: c.baseURL, Model: c.model, APIKey: key, Effort: c.effort, Fallbacks: c.fallbacks}, false, nil
	default:
		return nil, false, fmt.Errorf("unknown provider %q (want openai or anthropic)", c.provider)
	}
}

// buildPolicy wraps client (nil: the heuristic) into a policy; recall may be nil.
func buildPolicy(c config, client llm.Client, bounds bool, recall policy.Recall, plan policy.Planner) policy.Policy {
	if c.policy != "llm" {
		return gatePlans(policy.Heuristic{Plan: plan}, plan, c.plannerWait)
	}
	opts := []policy.LLMOption{policy.WithMaxTokens(c.maxTokens), policy.WithBounds(bounds)}
	if recall != nil {
		opts = append(opts, policy.WithRecall(recall))
	}
	if plan != nil {
		opts = append(opts, policy.WithPlan(plan))
		if u, ok := plan.(interface {
			UseNote(session, id, name string, sale int64) string
		}); ok {
			opts = append(opts, policy.WithUse(u.UseNote))
		}
	}
	return gatePlans(policy.NewLLM(client, opts...), plan, c.plannerWait)
}

// modelName is the model the overlay reports: none for the heuristic.
func modelName(c config, client llm.Client) string {
	switch {
	case client == nil, c.policy != "llm":
		return ""
	case c.model == "" && c.provider == "anthropic":
		return llm.AnthropicDefaultModel
	}
	return c.model
}

func openAIOptions(c config) (llm.OpenAIOptions, error) {
	o := llm.OpenAIOptions{NoSchema: c.noSchema}
	switch c.thinking {
	case "":
	case "on", "off":
		on := c.thinking == "on"
		o.Thinking = &on
	default:
		return o, fmt.Errorf("-thinking %q: want on, off or empty", c.thinking)
	}
	if c.temperature != "" {
		t, err := strconv.ParseFloat(c.temperature, 64)
		if err != nil || t < 0 {
			return o, fmt.Errorf("-temperature %q: want a non-negative number", c.temperature)
		}
		o.Temperature = &t
	}
	return o, nil
}

// openMemory never stops the bridge: without a usable database it plays without memory.
func openMemory(c config, client llm.Client, log *slog.Logger) *memory.Store {
	if !c.memory {
		return nil
	}
	if !c.reflect {
		client = nil
	}
	st, err := memory.Open(c.memoryDB, memory.Options{LLM: client, ReflectMaxTokens: c.maxTokens, CarryLessons: c.carryLessons, Logger: log})
	if err != nil {
		log.Error("memory unavailable; continuing without it", "db", c.memoryDB, "err", err)
		return nil
	}
	return st
}

// loadAppraisalGuides reads the value tables the inspect policy and the mod (through hello) use.
// Without them the bridge still runs: appraisals are deferred and the autopilot skips the board.
func loadAppraisalGuides(getenv func(string) string, log *slog.Logger) json.RawMessage {
	path := policy.AppraisalGuidesPath(getenv)
	raw, err := policy.ReadAppraisalGuides(path)
	if err != nil {
		raw = nil
	}
	// ReadAppraisalGuides has parsed it, so this only installs it.
	_ = policy.UseAppraisalGuides(raw)
	switch {
	case err != nil:
		log.Warn("appraisal guides unreadable: appraisals are deferred and the autopilot will not join the appraisal board", "path", path, "err", err)
	case raw == nil:
		log.Info("no appraisal guides: appraisals are deferred and the autopilot will not join the appraisal board", "path", path)
	default:
		log.Info("appraisal guides loaded", "path", path)
	}
	return raw
}

// run serves until ctx is done. ready, if set, receives the bound address.
func run(ctx context.Context, args []string, getenv func(string) string, stderr io.Writer, ready chan<- string) error {
	c, err := parseFlags(args, getenv, stderr)
	if err != nil {
		return err
	}
	mode, err := protocol.ParseMode(c.mode)
	if err != nil {
		return err
	}
	client, bounds, err := buildClient(c, getenv)
	if err != nil {
		return err
	}
	var persona string
	if c.persona != "" {
		if persona, err = strategy.LoadPersona(strategy.DefaultPersonaDir, c.persona); err != nil {
			return err
		}
	}
	var exemplars strategy.Exemplars
	if c.exemplars != "" {
		if exemplars, err = strategy.LoadExemplars(c.exemplars); err != nil {
			return err
		}
	}
	var base, routeText, start string
	switch {
	case c.format != "":
		if base, routeText, err = formatRoute(c.format, c.route); err != nil {
			return err
		}
	case c.route != "":
		r, err := route.Load("", c.route)
		if err != nil {
			return err
		}
		base, routeText, start = r.Base, r.Text(), r.Start
	}
	var choice *strategy.Choice
	if c.choice != "" {
		if choice, err = readChoice(c.choice); err != nil {
			return err
		}
		routeText += "\nYour day-0 thesis for this run: " + choice.Thesis
	}
	if c.baseDirectives != "" {
		b, err := os.ReadFile(c.baseDirectives)
		if err != nil {
			return err
		}
		if base, err = route.StripMeta(b); err != nil {
			return fmt.Errorf("-base-directives %s: %w", c.baseDirectives, err)
		}
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	guides := loadAppraisalGuides(getenv, log)
	mem := openMemory(c, client, log)
	var recs []server.Recorder
	var recall policy.Recall
	var sum server.Summarizer
	if mem != nil {
		recs, recall, sum = append(recs, mem), mem, mem
		defer func() {
			mem.Close()
			if n := mem.Dropped(); n > 0 {
				log.Warn("memory records lost", "count", n)
			}
		}()
	}
	// The strategist needs a model; it is closed after the server, which drains into it.
	var dlogRef *decisionlog.Logger
	var plan policy.Planner
	var planner server.Planner
	var watch *modelWatch
	if client != nil {
		if c.directives != "full" && c.directives != "delta" {
			return fmt.Errorf("-directives %q: want full or delta", c.directives)
		}
		watch = &modelWatch{Client: client}
		so := strategy.Options{LLM: watch, Logger: log, MaxTokens: c.maxTokens, Persona: persona, DeltaDirectives: c.directives == "delta", Bets: c.bets && c.directives == "delta", BaseDirectives: base, Route: routeText, Exemplars: exemplars,
			Agency: strategy.Agency{Choice: choice, Forks: c.forks, PlanEvery: c.planEvery, Start: start, Show: showTo(&dlogRef)}}
		if mem != nil {
			so.Recall = mem
		}
		if c.chatTally != "" {
			so.Audience = (&chatAudience{path: c.chatTally, log: log}).For
		}
		strat := strategy.New(so)
		defer strat.Close()
		recs, plan, planner = append(recs, strat), strat, strat
	}
	pol := buildPolicy(c, client, bounds, recall, plan)
	if g, ok := pol.(*planGate); ok {
		recs = append(recs, g)
		g.modelIdle = watch.idleFor
	}
	dlog, err := decisionlog.New(c.logDir, decisionlog.DefaultQueue, log)
	if err != nil {
		return fmt.Errorf("decision log: %w", err)
	}
	dlogRef = dlog
	defer func() {
		dlog.Close()
		if n := dlog.Dropped(); n > 0 {
			log.Warn("decision log records lost", "count", n)
		}
	}()

	ln, err := net.Listen("tcp", c.listen)
	if err != nil {
		return err
	}
	if host, _, _ := net.SplitHostPort(ln.Addr().String()); !net.ParseIP(host).IsLoopback() {
		log.Warn("listening beyond loopback; the bridge has no authentication", "addr", ln.Addr().String())
	}
	bridge := server.New(server.Config{Mode: mode, Policy: pol, Model: modelName(c, client), Log: dlog,
		Recorders: recs, Summarizer: sum, Planner: planner, AppraisalGuides: guides, Logger: log})
	// Runs before the memory and decision log close: recorders drain into memory first.
	defer func() {
		bridge.Close()
		if n := bridge.RecordsDropped(); n > 0 {
			log.Warn("recorder records lost", "count", n)
		}
	}()
	handler := bridge.Handler()
	if g, ok := pol.(*planGate); ok {
		handler = g.holdSummary(handler)
	}
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: a decide legitimately holds the response for up to deadline_ms.
	}
	log.Info("bridge listening", "addr", ln.Addr().String(), "mode", mode, "policy", pol.Name(),
		"provider", c.provider, "model", c.model, "log_dir", c.logDir, "memory", mem != nil, "memory_db", c.memoryDB, "persona", c.persona, "route", c.route, "planner", c.planner)
	if ready != nil {
		ready <- ln.Addr().String()
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
