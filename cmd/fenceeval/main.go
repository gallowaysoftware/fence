// Command fenceeval replays decide requests against a policy and reports what it chose,
// whether that was acceptable, and how long it took. Sources: protocol fixtures,
// hand-written scenarios (a request plus the options a sensible shopkeeper would take),
// and "decision" records from bridge logs.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "fenceeval:", err)
		os.Exit(1)
	}
}

type config struct {
	policy, baseURL, model, thinking, fixtures, scenarios, logs, filter string
	noSchema, asJSON                                                    bool
	temperature                                                         float64
	maxTokens, repeat                                                   int
	deadline                                                            time.Duration
}

func parseFlags(args []string, stderr io.Writer) (config, error) {
	var c config
	fs := flag.NewFlagSet("fenceeval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&c.policy, "policy", "heuristic", "heuristic|llm")
	fs.StringVar(&c.baseURL, "base-url", "", "OpenAI-compatible base URL for -policy llm, e.g. http://localhost:8080/v1")
	fs.StringVar(&c.model, "model", "", "model id for -policy llm")
	fs.StringVar(&c.thinking, "thinking", "", "on|off: send chat_template_kwargs.enable_thinking; empty leaves the server default")
	fs.BoolVar(&c.noSchema, "no-schema", false, "never send response_format")
	fs.Float64Var(&c.temperature, "temperature", -1, "sampling temperature; negative leaves the server default")
	fs.IntVar(&c.maxTokens, "max-tokens", policy.DefaultMaxTokens, "max output tokens per model call")
	fs.StringVar(&c.fixtures, "fixtures", "protocol/fixtures", "directory of decide.request*.json and decide.*.request.json fixtures; empty to skip")
	fs.StringVar(&c.scenarios, "scenarios", "internal/policy/testdata/scenarios", "directory of scenario JSONs; empty to skip")
	fs.StringVar(&c.logs, "logs", "", "glob of bridge decision logs, e.g. var/log/*.jsonl")
	fs.StringVar(&c.filter, "run", "", "only cases whose name matches this regexp")
	fs.IntVar(&c.repeat, "repeat", 1, "run every case this many times")
	fs.DurationVar(&c.deadline, "deadline", 0, "per-decision deadline; 0 uses each request's deadline_ms")
	fs.BoolVar(&c.asJSON, "json", false, "print JSON instead of a table")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected arguments %v", fs.Args())
	}
	if c.repeat < 1 {
		return c, errors.New("-repeat must be at least 1")
	}
	return c, nil
}

func buildPolicy(c config, getenv func(string) string) (policy.Policy, error) {
	switch c.policy {
	case "heuristic":
		return policy.Heuristic{}, nil
	case "llm":
	default:
		return nil, fmt.Errorf("unknown policy %q (want heuristic or llm)", c.policy)
	}
	if c.baseURL == "" || c.model == "" {
		return nil, errors.New("-policy llm needs -base-url and -model")
	}
	client := &llm.OpenAI{BaseURL: c.baseURL, Model: c.model, APIKey: getenv("OPENAI_API_KEY")}
	client.NoSchema = c.noSchema
	switch c.thinking {
	case "":
	case "on", "off":
		on := c.thinking == "on"
		client.Thinking = &on
	default:
		return nil, fmt.Errorf("-thinking %q (want on, off or empty)", c.thinking)
	}
	if c.temperature >= 0 {
		t := c.temperature
		client.Temperature = &t
	}
	return policy.NewLLM(client, policy.WithBounds(true), policy.WithMaxTokens(c.maxTokens)), nil
}

// evalCase is one request to replay. Expect is empty when nobody has said what is
// acceptable (fixtures, logs).
type evalCase struct {
	Name    string
	Source  string
	Note    string
	Expect  []string
	Request *protocol.DecideRequest
}

type scenarioFile struct {
	Note    string                 `json:"note"`
	Expect  []string               `json:"expect"`
	Request protocol.DecideRequest `json:"request"`
}

func loadScenarios(dir string) ([]evalCase, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []evalCase
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var s scenarioFile
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if len(s.Expect) == 0 {
			return nil, fmt.Errorf("%s: no expect", p)
		}
		req := s.Request
		out = append(out, evalCase{Name: strings.TrimSuffix(filepath.Base(p), ".json"), Source: "scenario", Note: s.Note, Expect: s.Expect, Request: &req})
	}
	return out, nil
}

func loadFixtures(dir string) ([]evalCase, error) {
	var paths []string
	for _, pattern := range []string{"decide.request*.json", "decide.*.request.json"} {
		ps, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		paths = append(paths, ps...)
	}
	var out []evalCase
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		var req protocol.DecideRequest
		if err := json.Unmarshal(b, &req); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, evalCase{Name: strings.TrimSuffix(filepath.Base(p), ".json"), Source: "fixture", Request: &req})
	}
	return out, nil
}

func loadLogs(glob string) ([]evalCase, error) {
	paths, err := filepath.Glob(glob)
	if err != nil {
		return nil, err
	}
	slices.Sort(paths)
	var out []evalCase
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
		line := 0
		for sc.Scan() {
			line++
			var rec struct {
				Type       string                 `json:"type"`
				DecisionID string                 `json:"decision_id"`
				Request    protocol.DecideRequest `json:"request"`
			}
			if json.Unmarshal(sc.Bytes(), &rec) != nil || rec.Type != "decision" {
				continue
			}
			name := rec.DecisionID
			if name == "" {
				name = fmt.Sprintf("%s:%d", filepath.Base(p), line)
			}
			req := rec.Request
			out = append(out, evalCase{Name: name, Source: "log", Request: &req})
		}
		err = sc.Err()
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
	}
	return out, nil
}

type result struct {
	Name       string   `json:"name"`
	Source     string   `json:"source"`
	Run        int      `json:"run"`
	Option     string   `json:"option"`
	By         string   `json:"by"`
	Acceptable *bool    `json:"acceptable,omitempty"`
	Expect     []string `json:"expect,omitempty"`
	LatencyMS  int64    `json:"latency_ms"`
	Attempts   int      `json:"attempts"`
	Rationale  string   `json:"rationale"`
	Error      string   `json:"error,omitempty"`
}

type totals struct {
	Cases      int   `json:"cases"`
	Defers     int   `json:"defers"`
	Fallbacks  int   `json:"fallbacks"`
	Repairs    int   `json:"repairs"`
	Scored     int   `json:"scored"`
	Acceptable int   `json:"acceptable"`
	ColdMS     int64 `json:"cold_ms"`
	P50MS      int64 `json:"p50_ms"`
	P95MS      int64 `json:"p95_ms"`
	MaxMS      int64 `json:"max_ms"`
}

func evaluate(ctx context.Context, pol policy.Policy, cases []evalCase, repeat int, deadline time.Duration) []result {
	var out []result
	for r := 1; r <= repeat; r++ {
		for _, c := range cases {
			d := deadline
			if d <= 0 {
				d = time.Duration(c.Request.DeadlineMS) * time.Millisecond
			}
			if d <= 0 {
				d = 20 * time.Second
			}
			dctx, cancel := context.WithTimeout(ctx, d)
			start := time.Now()
			dec := pol.Decide(dctx, c.Request.PolicyView())
			lat := time.Since(start)
			cancel()
			res := result{
				Name: c.Name, Source: c.Source, Run: r, Option: dec.Choice.Option, By: string(dec.Source),
				Expect: c.Expect, LatencyMS: lat.Milliseconds(), Attempts: len(dec.Trace.Attempts), Rationale: dec.Rationale,
			}
			if err := policy.ValidateChoice(c.Request.Options, dec.Choice); err != nil {
				res.Error = "invalid choice: " + err.Error()
			}
			if dec.Source == protocol.SourceFallback {
				res.Error = dec.Rationale
			}
			if len(c.Expect) > 0 {
				ok := res.Error == "" && slices.Contains(c.Expect, dec.Choice.Option)
				res.Acceptable = &ok
			}
			out = append(out, res)
		}
	}
	return out
}

func summarize(rs []result) totals {
	t := totals{Cases: len(rs)}
	var lats []int64
	for i, r := range rs {
		if r.Option == protocol.OptionDefer {
			t.Defers++
		}
		if r.By == string(protocol.SourceFallback) {
			t.Fallbacks++
		}
		if r.Attempts > 1 {
			t.Repairs++
		}
		if r.Acceptable != nil {
			t.Scored++
			if *r.Acceptable {
				t.Acceptable++
			}
		}
		// The first call pays model load and prompt-cache warmup; report it apart.
		if i == 0 {
			t.ColdMS = r.LatencyMS
			if len(rs) > 1 {
				continue
			}
		}
		lats = append(lats, r.LatencyMS)
	}
	slices.Sort(lats)
	if n := len(lats); n > 0 {
		t.P50MS = percentile(lats, 50)
		t.P95MS = percentile(lats, 95)
		t.MaxMS = lats[n-1]
	}
	return t
}

// percentile is nearest-rank on sorted values.
func percentile(sorted []int64, p int) int64 {
	rank := (p*len(sorted) + 99) / 100
	return sorted[max(rank, 1)-1]
}

func printTable(w io.Writer, rs []result, t totals) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CASE\tSRC\tRUN\tOPTION\tBY\tOK\tMS\tTRIES\tRATIONALE")
	for _, r := range rs {
		ok := "-"
		if r.Acceptable != nil {
			ok = map[bool]string{true: "yes", false: "NO"}[*r.Acceptable]
		}
		why := r.Rationale
		if r.Error != "" {
			why = "ERR " + r.Error
		}
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\t%d\t%d\t%s\n", r.Name, r.Source, r.Run, r.Option, r.By, ok, r.LatencyMS, r.Attempts, oneLine(why, 90))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	acc := "-"
	if t.Scored > 0 {
		acc = fmt.Sprintf("%d/%d", t.Acceptable, t.Scored)
	}
	_, err := fmt.Fprintf(w, "\ncases %d  acceptable %s  defers %d  fallbacks %d  repairs %d  cold %dms  p50 %dms  p95 %dms  max %dms\n",
		t.Cases, acc, t.Defers, t.Fallbacks, t.Repairs, t.ColdMS, t.P50MS, t.P95MS, t.MaxMS)
	return err
}

func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) error {
	c, err := parseFlags(args, stderr)
	if err != nil {
		return err
	}
	pol, err := buildPolicy(c, getenv)
	if err != nil {
		return err
	}
	var cases []evalCase
	for _, src := range []struct {
		arg  string
		load func(string) ([]evalCase, error)
	}{{c.fixtures, loadFixtures}, {c.scenarios, loadScenarios}, {c.logs, loadLogs}} {
		if src.arg == "" {
			continue
		}
		cs, err := src.load(src.arg)
		if err != nil {
			return err
		}
		cases = append(cases, cs...)
	}
	if c.filter != "" {
		re, err := regexp.Compile(c.filter)
		if err != nil {
			return fmt.Errorf("-run: %w", err)
		}
		cases = slices.DeleteFunc(cases, func(e evalCase) bool { return !re.MatchString(e.Name) })
	}
	if len(cases) == 0 {
		return errors.New("no cases to evaluate")
	}
	rs := evaluate(ctx, pol, cases, c.repeat, c.deadline)
	t := summarize(rs)
	if c.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Policy  string   `json:"policy"`
			Model   string   `json:"model,omitempty"`
			Results []result `json:"results"`
			Totals  totals   `json:"totals"`
		}{pol.Name(), c.model, rs, t})
	}
	return printTable(stdout, rs, t)
}
