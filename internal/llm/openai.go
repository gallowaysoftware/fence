package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// OpenAI talks to any OpenAI-compatible /chat/completions endpoint (llama-swap,
// llama.cpp server, vLLM, OpenAI itself).
type OpenAI struct {
	BaseURL string // e.g. http://localhost:8080/v1
	Model   string
	APIKey  string // optional; local servers usually need none
	HTTP    *http.Client
	OpenAIOptions

	// noSchema latches once the server rejects response_format, so later calls do not
	// pay for the rejected round-trip again.
	noSchema atomic.Bool
}

// OpenAIOptions are the server-specific knobs. The zero value sends a plain request
// with response_format, as before.
type OpenAIOptions struct {
	// NoSchema never sends response_format. For servers known to reject it (MTPLX
	// without llguidance answers 400), this saves the rejected round-trip on the first
	// call; otherwise the client learns it from that 400.
	NoSchema bool
	// Thinking, if set, sends chat_template_kwargs.enable_thinking (Qwen3 templates).
	Thinking *bool
	// Temperature, if set, overrides the server default.
	Temperature *float64
}

type oaRequest struct {
	Model              string          `json:"model"`
	Messages           []Message       `json:"messages"`
	MaxTokens          int             `json:"max_tokens,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	ResponseFormat     *oaRespFmt      `json:"response_format,omitempty"`
	ChatTemplateKwargs *oaTemplateArgs `json:"chat_template_kwargs,omitempty"`
	Stream             bool            `json:"stream"`
}

type oaTemplateArgs struct {
	EnableThinking bool `json:"enable_thinking"`
}

type oaRespFmt struct {
	Type       string        `json:"type"`
	JSONSchema *oaJSONSchema `json:"json_schema,omitempty"`
}

type oaJSONSchema struct {
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type oaResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			// llama.cpp, vLLM and MTPLX name the separated thinking differently.
			ReasoningContent string `json:"reasoning_content"`
			Reasoning        string `json:"reasoning"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (c *OpenAI) Complete(ctx context.Context, req Request) (Response, error) {
	msgs := make([]Message, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, Message{Role: "system", Content: req.System})
	}
	msgs = append(msgs, req.Messages...)
	body := oaRequest{Model: c.Model, Messages: msgs, MaxTokens: req.MaxTokens, Temperature: c.Temperature}
	if c.Thinking != nil {
		body.ChatTemplateKwargs = &oaTemplateArgs{EnableThinking: *c.Thinking}
	}
	if len(req.Schema) > 0 && !c.NoSchema && !c.noSchema.Load() {
		name := req.SchemaName
		if name == "" {
			name = "answer"
		}
		body.ResponseFormat = &oaRespFmt{Type: "json_schema", JSONSchema: &oaJSONSchema{Name: name, Strict: true, Schema: req.Schema}}
	}
	hdr := http.Header{}
	if c.APIKey != "" {
		hdr.Set("Authorization", "Bearer "+c.APIKey)
	}
	url := strings.TrimRight(c.BaseURL, "/") + "/chat/completions"
	var out oaResponse
	err := postJSON(ctx, c.client(), "openai", url, hdr, body, &out)
	var apiErr *APIError
	if body.ResponseFormat != nil && errors.As(err, &apiErr) && apiErr.Status == http.StatusBadRequest && mentionsResponseFormat(apiErr.Body) {
		c.noSchema.Store(true)
		body.ResponseFormat = nil
		out = oaResponse{}
		err = postJSON(ctx, c.client(), "openai", url, hdr, body, &out)
	}
	if err != nil {
		return Response{}, err
	}
	if len(out.Choices) == 0 {
		return Response{}, errors.New("openai: response has no choices")
	}
	ch := out.Choices[0]
	reasoning := ch.Message.ReasoningContent
	if reasoning == "" {
		reasoning = ch.Message.Reasoning
	}
	return Response{Text: ch.Message.Content, Reasoning: reasoning, Model: out.Model, StopReason: ch.FinishReason}, nil
}

func mentionsResponseFormat(body string) bool {
	b := strings.ToLower(body)
	return strings.Contains(b, "response_format") || strings.Contains(b, "json_schema")
}

// defaultHTTP fails fast when a roaming host is gone: without a dial timeout a vanished
// LAN peer holds the call until the decision deadline.
var defaultHTTP = &http.Client{Transport: &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	DialContext:         (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	MaxIdleConnsPerHost: 4,
	IdleConnTimeout:     90 * time.Second,
}}

func (c *OpenAI) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return defaultHTTP
}
