package main

import (
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
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

// play drives a gym -step-dir game with a model on the distilled prompt, writing each
// day's directives (and the WHY as notes.md, so the game reads like Claude's), until the
// gym writes final.json. The directives go to the gym as the model wrote them, extension
// keys included: the bridge would drop those.
func play(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("fencedistill play", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("step-dir", "", "the gym's -step-dir")
	endpoint := fs.String("endpoint", "http://127.0.0.1:9000/v1", "OpenAI-compatible base URL")
	model := fs.String("model", "", "model name at the endpoint")
	gym := fs.Bool("gym", true, "ask for the gym executor's extension keys (match the training data)")
	timeout := fs.Duration("timeout", 5*time.Minute, "per-call timeout")
	maxTokens := fs.Int("max-tokens", 700, "answer budget")
	delta := fs.Bool("delta", false, "round 2: play the delta planner (strategy.DistillDeltaPrompt) over -base, linted as the bridge lints")
	baseFile := fs.String("base", "", "with -delta: the base directives file the gym plays (its -directives)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || *model == "" {
		return errors.New("-step-dir and -model are required")
	}
	off, zero := false, 0.0
	c := &llm.OpenAI{BaseURL: *endpoint, Model: *model, OpenAIOptions: llm.OpenAIOptions{NoSchema: true, Thinking: &off, Temperature: &zero}}
	if *delta {
		return playDelta(c, *dir, *baseFile, *timeout, *maxTokens, stdout, stderr)
	}

	var prevWhy string
	var prevDay int
	for {
		day, err := nextDay(*dir)
		if err != nil {
			return err
		}
		if day == "" {
			fmt.Fprintf(stdout, "game over after day %d\n", prevDay)
			return nil
		}
		snapPath := filepath.Join(day, "snapshot.json")
		var snap stepSnapshot
		if err := readJSON(snapPath, &snap); err != nil {
			return err
		}
		n := snap.Day
		system, user, err := strategy.DistillPrompt(planInput(playDay{n: n, snap: snap}, prevWhy, prevDay), *gym)
		if err != nil {
			return fmt.Errorf("day %d: %w", n, err)
		}
		why, raw, text, err := ask(c, system, user, *timeout, *maxTokens)
		if err != nil {
			fmt.Fprintf(stderr, "day %d: %v; playing the base directives\n", n, err)
			raw, why = json.RawMessage("{}"), "(no usable answer: base directives)"
		}
		if err := os.WriteFile(filepath.Join(day, "answer.txt"), []byte(text), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(day, "notes.md"), []byte(why+"\n"), 0o644); err != nil {
			return err
		}
		tmp := filepath.Join(day, "directives.json.tmp")
		if err := os.WriteFile(tmp, raw, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(day, "directives.json")); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "day %d: %s\n", n, clip(why, 160))
		prevWhy, prevDay = why, n
	}
}

// playDelta is play for the delta planner: each day's answer goes through the bridge's delta
// lints, and the gym merges the result over its -directives base, as in a league's plan mode.
func playDelta(c llm.Client, dir, baseFile string, timeout time.Duration, maxTokens int, stdout, stderr io.Writer) error {
	var base string
	var baseMap map[string]any
	if baseFile != "" {
		var err error
		if base, baseMap, err = readBase(baseFile); err != nil {
			return err
		}
	}
	var yesterday *strategy.DayPlan
	prevDay := 0
	for {
		day, err := nextDay(dir)
		if err != nil {
			return err
		}
		if day == "" {
			fmt.Fprintf(stdout, "game over after day %d\n", prevDay)
			return nil
		}
		var snap stepSnapshot
		if err := readJSON(filepath.Join(day, "snapshot.json"), &snap); err != nil {
			return err
		}
		n := snap.Day
		system, user, err := strategy.DistillDeltaPrompt(strategy.PlanInput{Day: n, Snapshot: snap.Snapshot, Yesterday: yesterday, BaseDirectives: base})
		if err != nil {
			return fmt.Errorf("day %d: %w", n, err)
		}
		text, err := complete(c, system, user, timeout, maxTokens)
		var dr strategy.DirectivesResult
		if err == nil {
			dr = strategy.FinishDistillDelta(text, snap.Snapshot, n, base)
			err = dr.Err
		}
		if err != nil {
			// The bridge plays the base when the planner fails; so does this.
			fmt.Fprintf(stderr, "day %d: %v; playing the base directives\n", n, err)
			dr = strategy.DirectivesResult{}
			dr.Directives.Day = n
		}
		raw, _ := json.Marshal(dr.Directives)
		played, _ := normDelta(raw, baseMap)
		var notes strings.Builder
		why, _ := json.Marshal(dr.Why)
		fmt.Fprintf(&notes, "why: %s\nplayed: %s\n", why, changesLine(played))
		for _, f := range dr.Fixes {
			fmt.Fprintf(&notes, "fix: %s\n", f)
		}
		if err := os.WriteFile(filepath.Join(day, "answer.txt"), []byte(text), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(day, "notes.md"), []byte(notes.String()), 0o644); err != nil {
			return err
		}
		tmp := filepath.Join(day, "directives.json.tmp")
		if err := os.WriteFile(tmp, raw, 0o644); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(day, "directives.json")); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "day %d: %s\n", n, clip(changesLine(played), 200))
		yesterday, prevDay = &strategy.DayPlan{Day: n, Plan: changesLine(played)}, n
	}
}

func complete(c llm.Client, system, user string, timeout time.Duration, maxTokens int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resp, err := c.Complete(ctx, llm.Request{System: system, Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxTokens: maxTokens})
	return resp.Text, err
}

// nextDay waits for a day the gym has opened (snapshot.json) and nobody has answered
// (directives.json), and returns its directory; "" once the gym has written final.json.
// Day directories start at the warm-start day, not always day 1.
func nextDay(dir string) (string, error) {
	for {
		days, err := filepath.Glob(filepath.Join(dir, "day-*"))
		if err != nil {
			return "", err
		}
		slices.Sort(days)
		for _, d := range days {
			if fileExists(filepath.Join(d, "snapshot.json")) && !fileExists(filepath.Join(d, "directives.json")) {
				return d, nil
			}
		}
		if fileExists(filepath.Join(dir, "final.json")) {
			return "", nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// ask gets one answer, retrying once when it holds no JSON object.
func ask(c llm.Client, system, user string, timeout time.Duration, maxTokens int) (why string, raw json.RawMessage, text string, err error) {
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		var resp llm.Response
		resp, err = c.Complete(ctx, llm.Request{System: system, Messages: []llm.Message{{Role: llm.RoleUser, Content: user}}, MaxTokens: maxTokens})
		cancel()
		if err != nil {
			continue
		}
		text = resp.Text
		if raw, err = llm.ExtractJSON(text); err != nil {
			continue
		}
		var obj map[string]any
		if err = json.Unmarshal(raw, &obj); err != nil {
			continue
		}
		for _, l := range strings.Split(text, "\n") {
			if w, ok := strings.CutPrefix(strings.TrimSpace(l), "WHY:"); ok {
				why = strings.TrimSpace(w)
				break
			}
		}
		return why, raw, text, nil
	}
	return "", nil, text, err
}
