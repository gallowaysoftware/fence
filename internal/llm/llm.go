// Package llm is the bridge's only view of a model: one structured-output completion.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

type Request struct {
	System   string
	Messages []Message
	// Schema constrains the output where the server supports it. Callers must still
	// validate: some servers ignore it.
	Schema     json.RawMessage
	SchemaName string
	MaxTokens  int
}

type Response struct {
	// Text is the model's answer as sent, kept raw for the decision log; ExtractJSON
	// pulls the object out of it.
	Text string
	// Reasoning is thinking the server returned separately from Text, if any.
	Reasoning  string
	Model      string
	StopReason string
}

type Client interface {
	Complete(ctx context.Context, req Request) (Response, error)
}

// APIError is a non-2xx answer from a provider.
type APIError struct {
	Provider string
	Status   int
	Body     string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: HTTP %d: %s", e.Provider, e.Status, e.Body)
}

const maxErrBody = 2 << 10

// maxRespBody bounds what a misbehaving server can make us buffer.
const maxRespBody = 8 << 20

func postJSON(ctx context.Context, hc *http.Client, provider, url string, hdr http.Header, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", provider, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBody))
	if err != nil {
		return fmt.Errorf("%s: read body: %w", provider, err)
	}
	if resp.StatusCode/100 != 2 {
		s := string(raw)
		if len(s) > maxErrBody {
			s = s[:maxErrBody] + "…"
		}
		return &APIError{Provider: provider, Status: resp.StatusCode, Body: strings.TrimSpace(s)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s: decode response: %w", provider, err)
	}
	return nil
}

var errNoJSON = errors.New("no JSON object in model output")

// ExtractJSON returns the first complete JSON object in s, skipping <think> blocks,
// markdown fences and surrounding prose that servers ignoring response_format emit.
func ExtractJSON(s string) (json.RawMessage, error) {
	s = StripThinking(s)
	for start := strings.IndexByte(s, '{'); start >= 0; {
		if end := objectEnd(s[start:]); end > 0 {
			cand := s[start : start+end]
			if json.Valid([]byte(cand)) {
				return json.RawMessage(cand), nil
			}
		}
		next := strings.IndexByte(s[start+1:], '{')
		if next < 0 {
			break
		}
		start += 1 + next
	}
	return nil, errNoJSON
}

// StripThinking removes <think>…</think> blocks. An unterminated block means the model
// ran out of tokens mid-thought, so everything after it is discarded.
func StripThinking(s string) string {
	for {
		i := strings.Index(s, "<think>")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "</think>")
		if j < 0 {
			s = s[:i]
			break
		}
		s = s[:i] + s[i+j+len("</think>"):]
	}
	// Some templates emit only the closing tag because the opening one was in the prompt.
	if j := strings.LastIndex(s, "</think>"); j >= 0 {
		s = s[j+len("</think>"):]
	}
	return s
}

// objectEnd returns the length of the balanced {...} prefix of s, or 0.
func objectEnd(s string) int {
	depth := 0
	inStr, esc := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}
