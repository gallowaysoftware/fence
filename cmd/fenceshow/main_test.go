package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/gate"
	"github.com/gallowaysoftware/fence/internal/show"
)

func TestParseWindow(t *testing.T) {
	cases := []struct {
		in   string
		want show.Window
		bad  bool
	}{
		{"", show.Window{}, false},
		{"38-46", show.Window{From: 38, To: 46}, false},
		{"40-", show.Window{From: 40}, false},
		{"40", show.Window{From: 40}, false},
		{"46-38", show.Window{}, true},
		{"x-3", show.Window{}, true},
	}
	for _, c := range cases {
		got, err := parseWindow(c.in)
		if (err != nil) != c.bad || got != c.want {
			t.Errorf("parseWindow(%q) = %+v, %v", c.in, got, err)
		}
	}
}

func TestParseFlagsSources(t *testing.T) {
	env := func(string) string { return "" }
	cases := []struct {
		args []string
		ok   bool
	}{
		{[]string{"-replay", "m"}, true},
		{[]string{"-live-log", "var/log"}, true},
		{[]string{}, false},
		{[]string{"-replay", "m", "-feed", "f"}, false},
		{[]string{"-feed", "f", "-convert"}, false},
		{[]string{"-feed", "f", "-recorded-lines"}, true},
		{[]string{"-replay", "m", "-recorded-lines"}, false},
	}
	for _, c := range cases {
		if _, err := parseFlags(c.args, env, io.Discard); (err == nil) != c.ok {
			t.Errorf("parseFlags(%q): %v", c.args, err)
		}
	}
}

func TestNarratorLLM(t *testing.T) {
	cases := []struct {
		env  map[string]string
		args []string
		want string
		bad  bool
	}{
		{nil, nil, "none", false},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://drift/v1", "FENCE_SHOW_LLM_MODEL": "q"}, nil, "local q", false},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://drift/v1", "ANTHROPIC_API_KEY": "k"}, nil, "anthropic claude-haiku-5-5", false},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://drift/v1", "FENCE_SHOW_LLM_MODEL": "q", "ANTHROPIC_API_KEY": "k"}, []string{"-llm-provider", "openai"}, "local q", false},
		{nil, []string{"-llm-provider", "anthropic"}, "", true},
		{map[string]string{"ANTHROPIC_API_KEY": "k"}, []string{"-anthropic-model", "claude-sonnet-5-5"}, "anthropic claude-sonnet-5-5", false},
	}
	for _, c := range cases {
		cfg, err := parseFlags(append([]string{"-replay", "m"}, c.args...), func(k string) string { return c.env[k] }, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		_, desc, err := narratorLLM(cfg)
		if (err != nil) != c.bad || !strings.HasPrefix(desc, c.want) {
			t.Errorf("env %v args %q: %q, %v", c.env, c.args, desc, err)
		}
	}
}

// Only a narrator model served on this machine yields to the game; the API and a remote node keep
// speaking in character while it runs.
func TestSharesGPU(t *testing.T) {
	cases := []struct {
		env  map[string]string
		args []string
		want bool
	}{
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://127.0.0.1:9000/v1"}, nil, true},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://localhost:9000/v1"}, nil, true},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://drift:9002/v1"}, nil, false},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://127.0.0.1:9000/v1", "ANTHROPIC_API_KEY": "k"}, nil, false},
		{map[string]string{"FENCE_SHOW_LLM_URL": "http://127.0.0.1:9000/v1", "ANTHROPIC_API_KEY": "k"}, []string{"-llm-provider", "openai"}, true},
		{nil, nil, false},
	}
	for _, c := range cases {
		cfg, err := parseFlags(append([]string{"-replay", "m"}, c.args...), func(k string) string { return c.env[k] }, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if got := sharesGPU(cfg); got != c.want {
			t.Errorf("env %v args %q: %v", c.env, c.args, got)
		}
	}
}

// fencegate on loopback in front of another machine does not share the GPU; in front of this
// machine's server it does, and so does a plain local server.
func TestBehindRemoteGate(t *testing.T) {
	plain := httptest.NewServer(http.NotFoundHandler())
	defer plain.Close()
	for _, tc := range []struct {
		name, upstream string
		want           bool
	}{
		{"gate to another machine", "http://model.invalid:9002/v1", true},
		{"gate to this machine", "http://127.0.0.1:9000/v1", false},
	} {
		g, err := gate.New(gate.Config{Upstream: tc.upstream})
		if err != nil {
			t.Fatal(err)
		}
		s := httptest.NewServer(g)
		if got := behindRemoteGate(s.URL + "/narrate/v1"); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
		s.Close()
	}
	if behindRemoteGate(plain.URL + "/v1") {
		t.Error("a plain local server counted as a remote gate")
	}
}

func TestSplitSetup(t *testing.T) {
	if got := splitSetup(" Strategist: qwen27 ;; Voice: Kokoro;"); len(got) != 2 || got[1] != "Voice: Kokoro" {
		t.Errorf("splitSetup = %q", got)
	}
}

// The co-host asks the narrator's model, at the gate's filler priority when the narrator goes
// through the gate.
func TestCohostURL(t *testing.T) {
	cases := []struct{ llm, flag, want string }{
		{"http://127.0.0.1:7380/narrate/v1", "", "http://127.0.0.1:7380/filler/v1"},
		{"http://127.0.0.1:7380/narrate/v1/", "", "http://127.0.0.1:7380/filler/v1"},
		{"http://drift.invalid:9002/v1", "", "http://drift.invalid:9002/v1"},
		{"http://127.0.0.1:7380/narrate/v1", "http://127.0.0.1:7380/narrate/v1", "http://127.0.0.1:7380/narrate/v1"},
	}
	for _, tc := range cases {
		if got := cohostURL(config{llmURL: tc.llm, cohostURL: tc.flag}); got != tc.want {
			t.Errorf("cohostURL(%q, %q) = %q, want %q", tc.llm, tc.flag, got, tc.want)
		}
	}
	c, err := parseFlags([]string{"-replay", "m", "-cohost", "-llm-provider", "none"}, func(string) string { return "" }, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := narrator(c, show.NewHub(nil, ""), slog.New(slog.DiscardHandler)); err == nil {
		t.Error("a co-host without the narrator's LLM started")
	}
}
