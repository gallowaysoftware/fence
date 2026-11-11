package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProbeLevel is one concurrency level of a probe: n identical plan-sized requests at once.
type ProbeLevel struct {
	N             int     `json:"n"`
	OK            int     `json:"ok"`
	WallS         float64 `json:"wall_s"`
	LatencyP50S   float64 `json:"latency_p50_s"`
	LatencyMaxS   float64 `json:"latency_max_s"`
	CompletionTok int     `json:"completion_tokens"`
	TokPerS       float64 `json:"aggregate_tok_per_s"`
	Error         string  `json:"error,omitempty"`
}

type ProbeResult struct {
	Endpoint    string       `json:"endpoint"`
	Model       string       `json:"model"`
	At          string       `json:"at"`
	PingS       float64      `json:"ping_s"`
	Levels      []ProbeLevel `json:"levels"`
	Recommended int          `json:"recommended_concurrency"`
}

// A plan-sized request: about the strategist's prompt length, asking for a few hundred tokens.
var probePrompt = "You run a pawn shop in a dystopian city. Here is today's ledger.\n" +
	strings.Repeat("Item: brass lamp; bought 40; estimate 55; heat 0; faction lower levels; days held 3.\n", 60) +
	"Write today's plan under the headings GOALS, CASH, STOCK, RISK, in about 250 words."

type chatReq struct {
	Model       string         `json:"model"`
	Messages    []chatMsg      `json:"messages"`
	MaxTokens   int            `json:"max_tokens"`
	Temperature *float64       `json:"temperature,omitempty"`
	Kwargs      map[string]any `json:"chat_template_kwargs,omitempty"`
}

type chatMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func complete(ctx context.Context, ep *Endpoint, prompt string, maxTokens int) (completion int, err error) {
	r := chatReq{Model: ep.Model, Messages: []chatMsg{{Role: "user", Content: prompt}}, MaxTokens: maxTokens}
	if ep.Thinking != "" {
		r.Kwargs = map[string]any{"enable_thinking": ep.Thinking == "on"}
	}
	if ep.Temperature != "" {
		if t, err := strconv.ParseFloat(ep.Temperature, 64); err == nil {
			r.Temperature = &t
		}
	}
	body, _ := json.Marshal(r)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("status %d: %s", resp.StatusCode, clip(string(b), 200))
	}
	var out struct {
		Usage struct {
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	json.Unmarshal(b, &out)
	return out.Usage.CompletionTokens, nil
}

// Ping is a one-token request: is the endpoint free enough to play on right now?
func Ping(ctx context.Context, ep *Endpoint, timeout time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	_, err := complete(ctx, ep, "Reply with the word ok.", 8)
	if err != nil && strings.HasPrefix(err.Error(), "status 507") {
		select {
		case <-time.After(retryAfter):
			_, err = complete(ctx, ep, "Reply with the word ok.", 8)
		case <-ctx.Done():
		}
	}
	return time.Since(start), err
}

// Probe measures latency and aggregate throughput at each concurrency level and recommends
// the largest level that still adds throughput without stretching latency past 2.5×.
func Probe(ctx context.Context, name string, ep *Endpoint, levels []int, maxTokens int) ProbeResult {
	res := ProbeResult{Endpoint: name, Model: ep.Model, At: time.Now().UTC().Format(time.RFC3339), Recommended: 1}
	if d, err := Ping(ctx, ep, 5*time.Minute); err == nil {
		res.PingS = round1(d.Seconds())
	} else {
		res.Levels = append(res.Levels, ProbeLevel{Error: err.Error()})
		return res
	}
	var base *ProbeLevel
	for _, n := range levels {
		lv := probeLevel(ctx, ep, n, maxTokens)
		res.Levels = append(res.Levels, lv)
		if lv.OK < n {
			break
		}
		if base == nil {
			base = &res.Levels[len(res.Levels)-1]
			continue
		}
		prev := res.Levels[len(res.Levels)-2]
		if lv.TokPerS >= 1.25*prev.TokPerS && lv.LatencyP50S <= 2.5*base.LatencyP50S {
			res.Recommended = n
		} else {
			break
		}
	}
	return res
}

func probeLevel(ctx context.Context, ep *Endpoint, n, maxTokens int) ProbeLevel {
	lv := ProbeLevel{N: n}
	lat := make([]float64, n)
	toks := make([]int, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := time.Now()
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			t := time.Now()
			// A distinct prefix per request, so no server shares a cached prompt across them.
			toks[i], errs[i] = complete(cctx, ep, fmt.Sprintf("Request %d at %d.\n", i, t.UnixNano())+probePrompt, maxTokens)
			lat[i] = time.Since(t).Seconds()
		}()
	}
	wg.Wait()
	lv.WallS = round1(time.Since(start).Seconds())
	var ok []float64
	for i := range n {
		if errs[i] != nil {
			lv.Error = errs[i].Error()
			continue
		}
		lv.OK++
		lv.CompletionTok += toks[i]
		ok = append(ok, lat[i])
	}
	if len(ok) > 0 {
		lv.LatencyP50S = round1(median(ok))
		lv.LatencyMaxS = round1(slices.Max(ok))
	}
	if lv.WallS > 0 {
		lv.TokPerS = round1(float64(lv.CompletionTok) / lv.WallS)
	}
	return lv
}
