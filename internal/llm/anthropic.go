package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	AnthropicDefaultBaseURL = "https://api.anthropic.com"
	AnthropicDefaultModel   = "claude-sonnet-5-5"
	anthropicVersion        = "2026-10-21"
	anthropicFallbackBeta   = "server-side-fallback-2026-07-01"
)

// Anthropic calls the Messages API over plain HTTP.
type Anthropic struct {
	BaseURL string
	Model   string
	APIKey  string
	// Effort is output_config.effort. Decisions run under a game-side deadline, so the
	// bridge defaults to "low" rather than the model's own default.
	Effort string
	// Fallbacks opts into server-side refusal fallback ("default" routing). Turn it off
	// when BaseURL is a proxy that rejects the beta.
	Fallbacks bool
	HTTP      *http.Client
}

type anRequest struct {
	Model        string       `json:"model"`
	MaxTokens    int          `json:"max_tokens"`
	System       string       `json:"system,omitempty"`
	Messages     []Message    `json:"messages"`
	OutputConfig *anOutputCfg `json:"output_config,omitempty"`
	Fallbacks    string       `json:"fallbacks,omitempty"`
}

type anOutputCfg struct {
	Effort string    `json:"effort,omitempty"`
	Format *anFormat `json:"format,omitempty"`
}

type anFormat struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

type anResponse struct {
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason  string `json:"stop_reason"`
	StopDetails *struct {
		Category    string `json:"category"`
		Explanation string `json:"explanation"`
	} `json:"stop_details"`
}

func (c *Anthropic) Complete(ctx context.Context, req Request) (Response, error) {
	if c.APIKey == "" {
		return Response{}, errors.New("anthropic: no API key (set ANTHROPIC_API_KEY)")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 4096
	}
	body := anRequest{
		Model:     c.model(),
		MaxTokens: maxTokens,
		System:    req.System,
		Messages:  req.Messages,
	}
	if c.Effort != "" || len(req.Schema) > 0 {
		body.OutputConfig = &anOutputCfg{Effort: c.Effort}
		if len(req.Schema) > 0 {
			body.OutputConfig.Format = &anFormat{Type: "json_schema", Schema: req.Schema}
		}
	}
	hdr := http.Header{}
	hdr.Set("x-api-key", c.APIKey)
	hdr.Set("anthropic-version", anthropicVersion)
	if c.Fallbacks {
		body.Fallbacks = "default"
		hdr.Set("anthropic-beta", anthropicFallbackBeta)
	}
	base := c.BaseURL
	if base == "" {
		base = AnthropicDefaultBaseURL
	}
	var out anResponse
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	if err := postJSON(ctx, hc, "anthropic", strings.TrimRight(base, "/")+"/v1/messages", hdr, body, &out); err != nil {
		return Response{}, err
	}
	var text strings.Builder
	for _, b := range out.Content {
		if b.Type == "text" {
			text.WriteString(b.Text)
		}
	}
	resp := Response{Text: text.String(), Model: out.Model, StopReason: out.StopReason}
	switch out.StopReason {
	case "refusal":
		detail := ""
		if out.StopDetails != nil {
			detail = fmt.Sprintf(" (%s: %s)", out.StopDetails.Category, out.StopDetails.Explanation)
		}
		return resp, fmt.Errorf("anthropic: refused%s", detail)
	case "max_tokens":
		return resp, errors.New("anthropic: output truncated at max_tokens")
	}
	return resp, nil
}

func (c *Anthropic) model() string {
	if c.Model != "" {
		return c.Model
	}
	return AnthropicDefaultModel
}
