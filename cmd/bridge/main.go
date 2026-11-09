// Command bridge is the Go sidecar the fence mod calls at each decision point.
package main

import (
	"context"
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
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/memory"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
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

	memory, carryLessons bool
	memoryDB             string
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
	switch c.policy {
	case "heuristic":
		return nil, false, nil
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
	if client == nil {
		return policy.Heuristic{}
	}
	opts := []policy.LLMOption{policy.WithMaxTokens(c.maxTokens), policy.WithBounds(bounds)}
	if recall != nil {
		opts = append(opts, policy.WithRecall(recall))
	}
	if plan != nil {
		opts = append(opts, policy.WithPlan(plan))
	}
	return policy.NewLLM(client, opts...)
}

// modelName is the model the overlay reports: none for the heuristic.
func modelName(c config, client llm.Client) string {
	switch {
	case client == nil:
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
	st, err := memory.Open(c.memoryDB, memory.Options{LLM: client, ReflectMaxTokens: c.maxTokens, CarryLessons: c.carryLessons, Logger: log})
	if err != nil {
		log.Error("memory unavailable; continuing without it", "db", c.memoryDB, "err", err)
		return nil
	}
	return st
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
	log := slog.New(slog.NewTextHandler(stderr, nil))
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
	var plan policy.Planner
	var planner server.Planner
	if client != nil {
		so := strategy.Options{LLM: client, Logger: log, MaxTokens: c.maxTokens}
		if mem != nil {
			so.Recall = mem
		}
		strat := strategy.New(so)
		defer strat.Close()
		recs, plan, planner = append(recs, strat), strat, strat
	}
	pol := buildPolicy(c, client, bounds, recall, plan)
	dlog, err := decisionlog.New(c.logDir, decisionlog.DefaultQueue, log)
	if err != nil {
		return fmt.Errorf("decision log: %w", err)
	}
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
		Recorders: recs, Summarizer: sum, Planner: planner, Logger: log})
	// Runs before the memory and decision log close: recorders drain into memory first.
	defer func() {
		bridge.Close()
		if n := bridge.RecordsDropped(); n > 0 {
			log.Warn("recorder records lost", "count", n)
		}
	}()
	srv := &http.Server{
		Handler:           bridge.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No WriteTimeout: a decide legitimately holds the response for up to deadline_ms.
	}
	log.Info("bridge listening", "addr", ln.Addr().String(), "mode", mode, "policy", pol.Name(),
		"provider", c.provider, "model", c.model, "log_dir", c.logDir, "memory", mem != nil, "memory_db", c.memoryDB)
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
