package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"plain", `{"a":1}`, `{"a":1}`, false},
		{"prose around", `Sure! {"a":1} hope that helps`, `{"a":1}`, false},
		{"fenced", "```json\n{\"a\": {\"b\": [1,2]}}\n```", `{"a": {"b": [1,2]}}`, false},
		{"think block", `<think>maybe {"a":0}</think>{"a":1}`, `{"a":1}`, false},
		{"only closing think", `reasoning {"x":0} </think> {"a":1}`, `{"a":1}`, false},
		{"unterminated think", `{"a":1}<think>{"b":2}`, `{"a":1}`, false},
		{"unterminated think only", `<think>{"b":2}`, ``, true},
		{"brace in string", `{"r":"a } b {","a":1}`, `{"r":"a } b {","a":1}`, false},
		{"escaped quote", `{"r":"say \"}\"","a":1}`, `{"r":"say \"}\"","a":1}`, false},
		{"invalid then valid", `{oops} {"a":1}`, `{"a":1}`, false},
		{"truncated", `{"a":`, ``, true},
		{"none", `no json here`, ``, true},
		{"empty", ``, ``, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractJSON(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("want error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

var schema = json.RawMessage(`{"type":"object"}`)

func TestOpenAI(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(t *testing.T, calls int, body map[string]any) (int, string)
		wantText   string
		wantErr    string
		wantCalls  int
		wantSchema []bool // whether each call carried response_format
	}{
		{
			name: "ok",
			handler: func(t *testing.T, _ int, body map[string]any) (int, string) {
				msgs := body["messages"].([]any)
				if msgs[0].(map[string]any)["role"] != "system" {
					t.Errorf("first message should be system: %v", msgs[0])
				}
				return 200, `{"model":"qwen","choices":[{"message":{"content":"{\"option\":\"accept\"}"},"finish_reason":"stop"}]}`
			},
			wantText: `{"option":"accept"}`, wantCalls: 1, wantSchema: []bool{true},
		},
		{
			name: "server rejects response_format then succeeds without",
			handler: func(_ *testing.T, calls int, _ map[string]any) (int, string) {
				if calls == 1 {
					return 400, `{"error":{"message":"response_format json_schema not supported"}}`
				}
				return 200, `{"choices":[{"message":{"content":"<think>hm</think>{}"}}]}`
			},
			wantText: `<think>hm</think>{}`, wantCalls: 2, wantSchema: []bool{true, false},
		},
		{
			name: "unrelated 400 is not retried",
			handler: func(*testing.T, int, map[string]any) (int, string) {
				return 400, `{"error":"context length exceeded"}`
			},
			wantErr: "HTTP 400", wantCalls: 1, wantSchema: []bool{true},
		},
		{
			name: "500",
			handler: func(*testing.T, int, map[string]any) (int, string) {
				return 503, `loading model`
			},
			wantErr: "HTTP 503", wantCalls: 1, wantSchema: []bool{true},
		},
		{
			name: "no choices",
			handler: func(*testing.T, int, map[string]any) (int, string) {
				return 200, `{"choices":[]}`
			},
			wantErr: "no choices", wantCalls: 1, wantSchema: []bool{true},
		},
		{
			name: "garbage body",
			handler: func(*testing.T, int, map[string]any) (int, string) {
				return 200, `<html>proxy error</html>`
			},
			wantErr: "decode response", wantCalls: 1, wantSchema: []bool{true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			var withSchema []bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					t.Errorf("path %s", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer k" {
					t.Errorf("auth header %q", got)
				}
				var body map[string]any
				b, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(b, &body); err != nil {
					t.Errorf("bad request body: %v", err)
				}
				_, has := body["response_format"]
				withSchema = append(withSchema, has)
				code, resp := tt.handler(t, int(calls.Add(1)), body)
				w.WriteHeader(code)
				io.WriteString(w, resp)
			}))
			defer srv.Close()
			c := &OpenAI{BaseURL: srv.URL + "/v1/", Model: "m", APIKey: "k"}
			resp, err := c.Complete(context.Background(), Request{System: "sys", Messages: []Message{{Role: RoleUser, Content: "hi"}}, Schema: schema})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if resp.Text != tt.wantText {
				t.Fatalf("text %q, want %q", resp.Text, tt.wantText)
			}
			if int(calls.Load()) != tt.wantCalls {
				t.Fatalf("calls %d, want %d", calls.Load(), tt.wantCalls)
			}
			if len(withSchema) != len(tt.wantSchema) {
				t.Fatalf("schema per call %v, want %v", withSchema, tt.wantSchema)
			}
			for i := range withSchema {
				if withSchema[i] != tt.wantSchema[i] {
					t.Fatalf("schema per call %v, want %v", withSchema, tt.wantSchema)
				}
			}
		})
	}
}

func TestOpenAIOptions(t *testing.T) {
	yes, no, temp := true, false, 0.3
	tests := []struct {
		name          string
		opts          OpenAIOptions
		reply         string
		wantKeys      map[string]string // top-level body key → JSON value; "" means absent
		wantReasoning string
	}{
		{"zero value", OpenAIOptions{}, `{"choices":[{"message":{"content":"{}"}}]}`,
			map[string]string{"response_format": "*", "chat_template_kwargs": "", "temperature": ""}, ""},
		{"no schema", OpenAIOptions{NoSchema: true}, `{"choices":[{"message":{"content":"{}"}}]}`,
			map[string]string{"response_format": ""}, ""},
		{"thinking off", OpenAIOptions{Thinking: &no}, `{"choices":[{"message":{"content":"{}"}}]}`,
			map[string]string{"chat_template_kwargs": `{"enable_thinking":false}`}, ""},
		{"thinking on, reasoning_content", OpenAIOptions{Thinking: &yes}, `{"choices":[{"message":{"content":"{}","reasoning_content":"hmm"}}]}`,
			map[string]string{"chat_template_kwargs": `{"enable_thinking":true}`}, "hmm"},
		{"reasoning field", OpenAIOptions{}, `{"choices":[{"message":{"content":"{}","reasoning":"hm2"}}]}`, nil, "hm2"},
		{"temperature", OpenAIOptions{Temperature: &temp}, `{"choices":[{"message":{"content":"{}"}}]}`,
			map[string]string{"temperature": "0.3"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]json.RawMessage
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				json.NewDecoder(r.Body).Decode(&body)
				io.WriteString(w, tt.reply)
			}))
			defer srv.Close()
			c := &OpenAI{BaseURL: srv.URL, Model: "m", OpenAIOptions: tt.opts}
			resp, err := c.Complete(context.Background(), Request{Schema: schema})
			if err != nil {
				t.Fatal(err)
			}
			for k, want := range tt.wantKeys {
				got, has := body[k]
				switch {
				case want == "" && has:
					t.Errorf("%s sent: %s", k, got)
				case want == "*" && !has, want != "" && want != "*" && string(got) != want:
					t.Errorf("%s = %s, want %s", k, got, want)
				}
			}
			if resp.Reasoning != tt.wantReasoning {
				t.Errorf("reasoning %q, want %q", resp.Reasoning, tt.wantReasoning)
			}
		})
	}
}

func TestOpenAIRemembersNoSchema(t *testing.T) {
	var sawSchema []bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		_, has := body["response_format"]
		sawSchema = append(sawSchema, has)
		if has {
			w.WriteHeader(400)
			io.WriteString(w, `unknown field response_format`)
			return
		}
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()
	c := &OpenAI{BaseURL: srv.URL, Model: "m"}
	for range 2 {
		if _, err := c.Complete(context.Background(), Request{Schema: schema}); err != nil {
			t.Fatal(err)
		}
	}
	if want := []bool{true, false, false}; len(sawSchema) != 3 || sawSchema[2] != want[2] || sawSchema[0] != want[0] {
		t.Fatalf("saw %v, want %v", sawSchema, want)
	}
}

func TestClientsHonorContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)
	clients := map[string]Client{
		"openai":    &OpenAI{BaseURL: srv.URL, Model: "m"},
		"anthropic": &Anthropic{BaseURL: srv.URL, APIKey: "k"},
	}
	for name, c := range clients {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			start := time.Now()
			_, err := c.Complete(ctx, Request{Messages: []Message{{Role: RoleUser, Content: "x"}}})
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err %v, want deadline exceeded", err)
			}
			if d := time.Since(start); d > time.Second {
				t.Fatalf("hung for %v", d)
			}
		})
	}
}

func TestAnthropic(t *testing.T) {
	tests := []struct {
		name     string
		client   Anthropic
		status   int
		resp     string
		check    func(t *testing.T, r *http.Request, body map[string]any)
		wantText string
		wantErr  string
	}{
		{
			name:   "ok with structured output",
			client: Anthropic{APIKey: "k", Effort: "low", Fallbacks: true},
			status: 200,
			resp:   `{"model":"claude-sonnet-5-5","content":[{"type":"thinking","thinking":""},{"type":"text","text":"{\"option\":\"refuse\"}"}],"stop_reason":"end_turn","stop_details":null}`,
			check: func(t *testing.T, r *http.Request, body map[string]any) {
				if r.URL.Path != "/v1/messages" {
					t.Errorf("path %s", r.URL.Path)
				}
				for k, want := range map[string]string{
					"x-api-key":         "k",
					"anthropic-version": anthropicVersion,
					"anthropic-beta":    anthropicFallbackBeta,
					"content-type":      "application/json",
				} {
					if got := r.Header.Get(k); got != want {
						t.Errorf("header %s = %q, want %q", k, got, want)
					}
				}
				if body["model"] != AnthropicDefaultModel {
					t.Errorf("model %v", body["model"])
				}
				if body["system"] != "sys" {
					t.Errorf("system %v", body["system"])
				}
				if body["fallbacks"] != "default" {
					t.Errorf("fallbacks %v", body["fallbacks"])
				}
				oc, _ := body["output_config"].(map[string]any)
				if oc["effort"] != "low" {
					t.Errorf("effort %v", oc["effort"])
				}
				f, _ := oc["format"].(map[string]any)
				if f["type"] != "json_schema" || f["schema"] == nil {
					t.Errorf("format %v", f)
				}
				for _, banned := range []string{"temperature", "top_p", "top_k", "thinking"} {
					if _, ok := body[banned]; ok {
						t.Errorf("request must not send %s", banned)
					}
				}
			},
			wantText: `{"option":"refuse"}`,
		},
		{
			name:   "fallbacks off sends no beta",
			client: Anthropic{APIKey: "k", Model: "claude-haiku-5-5"},
			status: 200,
			resp:   `{"content":[{"type":"text","text":"{}"}],"stop_reason":"end_turn"}`,
			check: func(t *testing.T, r *http.Request, body map[string]any) {
				if r.Header.Get("anthropic-beta") != "" {
					t.Errorf("unexpected beta header")
				}
				if _, ok := body["fallbacks"]; ok {
					t.Errorf("unexpected fallbacks")
				}
				if body["model"] != "claude-haiku-5-5" {
					t.Errorf("model %v", body["model"])
				}
			},
			wantText: `{}`,
		},
		{
			name:    "refusal",
			client:  Anthropic{APIKey: "k"},
			status:  200,
			resp:    `{"content":[],"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber","explanation":"no"}}`,
			wantErr: "refused (cyber: no)",
		},
		{
			name:    "truncated",
			client:  Anthropic{APIKey: "k"},
			status:  200,
			resp:    `{"content":[{"type":"text","text":"{\"opt"}],"stop_reason":"max_tokens"}`,
			wantErr: "max_tokens",
		},
		{
			name:    "overloaded",
			client:  Anthropic{APIKey: "k"},
			status:  529,
			resp:    `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			wantErr: "HTTP 529",
		},
		{
			name:    "no key",
			client:  Anthropic{},
			wantErr: "no API key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("bad body: %v", err)
				}
				if tt.check != nil {
					tt.check(t, r, body)
				}
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.resp)
			}))
			defer srv.Close()
			c := tt.client
			c.BaseURL = srv.URL
			resp, err := c.Complete(context.Background(), Request{System: "sys", Messages: []Message{{Role: RoleUser, Content: "hi"}}, Schema: schema})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if resp.Text != tt.wantText {
				t.Fatalf("text %q, want %q", resp.Text, tt.wantText)
			}
		})
	}
}
