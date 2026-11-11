package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

// proposeMaxTokens bounds one proposal answer: three candidates with a line of reason each.
const proposeMaxTokens = 1500

// runHybrid plays a hybrid match: fence-rollout hosts the game and rolls the candidates; with
// an endpoint, this process answers its proposal requests through the league's proxy.
func (l *League) runHybrid(ctx context.Context, m Match, ep *Endpoint, meta MatchMeta, dir string) error {
	id := m.ID()
	rollout := filepath.Join(mustAbs(l.Local.GymDir), "sim", "rollout", "bin", "Release", "net10.0", "fence-rollout.dll")
	if !fileExists(rollout) {
		return fmt.Errorf("no fence-rollout at %s: rebuild the gym with `fenceleague gym`", rollout)
	}
	ex := l.Spec.executor(m.Player)
	args := []string{rollout, "play",
		"-seed", strconv.Itoa(m.Seed), "-days", strconv.Itoa(l.Spec.Days),
		"-difficulty", m.Difficulty, "-start", m.Start,
		"-summary", filepath.Join(dir, "gym.summary.json"),
		"-rows", filepath.Join(dir, "decisions.jsonl"),
		"-decision-log", filepath.Join(dir, "log", "hybrid.jsonl"),
		"-work", filepath.Join("/dev/shm", "fence-rollout", id)}
	if m.Perks != NoPerks && l.Perks {
		args = append(args, "-perks", m.Perks)
	}
	if ex.Knobs != "" {
		args = append(args, "-knobs", ex.Knobs)
	}
	if d := l.directives(m); d != "" {
		args = append(args, "-directives", mustAbs(d))
	}
	proposeDir := filepath.Join(dir, "propose")
	if ep != nil {
		args = append(args, "-hybrid", "llm", "-propose-dir", proposeDir)
	} else {
		args = append(args, "-hybrid", "blind")
	}
	meta.GymArgs = append(args, m.Player.RolloutFlags...)
	if ep != nil {
		meta.Model = ep.Model
		if m.Player.Model != "" {
			meta.Model = m.Player.Model
		}
		if err := l.proxy.Register(id, m.Player.Endpoint, ep, filepath.Join(dir, "llm.jsonl")); err != nil {
			return err
		}
		defer l.proxy.Unregister(id)
		opts, err := ep.openAIOptions()
		if err != nil {
			return err
		}
		// A proposal is capped at proposeMaxTokens, which a reasoning model spends thinking: off unless the
		// endpoint asks for it.
		if opts.Thinking == nil {
			off := false
			opts.Thinking = &off
		}
		client := &llm.OpenAI{BaseURL: l.proxy.BaseURL(id), Model: meta.Model, OpenAIOptions: opts}
		pctx, stop := context.WithCancel(ctx)
		defer stop()
		go servePropose(pctx, proposeDir, client, proposeMaxTokens, l.Log.With("match", id))
	}
	if err := writeJSON(filepath.Join(dir, "match.json"), meta); err != nil {
		return err
	}
	return l.runGym(ctx, meta.GymArgs, dir)
}

func (e *Endpoint) openAIOptions() (llm.OpenAIOptions, error) {
	o := llm.OpenAIOptions{NoSchema: e.NoSchema}
	switch e.Thinking {
	case "":
	case "on", "off":
		on := e.Thinking == "on"
		o.Thinking = &on
	default:
		return o, fmt.Errorf("thinking %q: want on, off or empty", e.Thinking)
	}
	if e.Temperature != "" {
		t, err := strconv.ParseFloat(e.Temperature, 64)
		if err != nil || t < 0 {
			return o, fmt.Errorf("temperature %q: want a non-negative number", e.Temperature)
		}
		o.Temperature = &t
	}
	return o, nil
}

// servePropose answers fence-rollout's proposal requests in dir (sim/rollout/Hybrid.cs) until
// ctx ends: dNNN.request.json in, dNNN.response.json out, each written whole by a rename.
func servePropose(ctx context.Context, dir string, c llm.Client, maxTokens int, log *slog.Logger) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Error("propose dir", "err", err)
		return
	}
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		reqs, _ := filepath.Glob(filepath.Join(dir, "d*.request.json"))
		for _, path := range reqs {
			resp := strings.TrimSuffix(path, ".request.json") + ".response.json"
			if fileExists(resp) {
				continue
			}
			var req strategy.ProposeRequest
			b, err := os.ReadFile(path)
			var r strategy.ProposeResult
			if err == nil {
				err = json.Unmarshal(b, &req)
			}
			if err != nil {
				r.Error = "request: " + err.Error()
			} else {
				r = strategy.Propose(ctx, c, req, maxTokens)
			}
			if ctx.Err() != nil {
				return
			}
			log.Info("proposal", "day", req.Day, "candidates", len(r.Candidates), "seconds", r.Seconds, "err", r.Error)
			if err := writeJSON(resp, r); err != nil {
				log.Error("proposal answer", "err", err)
			}
		}
	}
}

// cmdPropose answers one fence-rollout's proposals from the command line, for a hybrid game run
// outside a league.
func cmdPropose(ctx context.Context, dir, baseURL, model, thinking string, maxTokens int, stderr io.Writer) error {
	if dir == "" || baseURL == "" || model == "" {
		return fmt.Errorf("propose needs -dir, -base-url and -model")
	}
	opts, err := (&Endpoint{Thinking: thinking}).openAIOptions()
	if err != nil {
		return err
	}
	servePropose(ctx, dir, &llm.OpenAI{BaseURL: strings.TrimRight(baseURL, "/"), Model: model, OpenAIOptions: opts}, maxTokens, slog.New(slog.NewTextHandler(stderr, nil)))
	return nil
}
