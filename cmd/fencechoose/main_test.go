package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/agency"
)

const thesis = "A ranch that sells its rat meat to the Board's buyers before the rent climbs, because the cage is the one engine that pays in cash before the buyout; I will watch the cash line against the deadline every week and cut the upgrades the day it slips."

// fakeLLM answers each chat completion with the next answer and keeps the prompts it was sent.
func fakeLLM(t *testing.T, answers ...string) (*httptest.Server, *[]string) {
	var prompts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		prompts = append(prompts, req.Messages[len(req.Messages)-1].Content)
		if len(answers) == 0 {
			http.Error(w, "no more answers", http.StatusInternalServerError)
			return
		}
		a := answers[0]
		answers = answers[1:]
		json.NewEncoder(w).Encode(map[string]any{"model": "fake", "choices": []any{map[string]any{"message": map[string]any{"content": a}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &prompts
}

func answer(v map[string]any) string {
	v["thesis"] = thesis
	b, _ := json.Marshal(v)
	return string(b)
}

func TestChoose(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "choices")
	t.Chdir(filepath.Join("..", ".."))
	clock := time.Date(2026, 11, 12, 20, 0, 0, 0, time.UTC)
	choose := func(srv *httptest.Server, extra ...string) (string, error) {
		clock = clock.Add(time.Hour)
		var out, errb bytes.Buffer
		args := append([]string{"-llm-url", srv.URL, "-model", "fake", "-dir", dir, "-lean"}, extra...)
		err := run(context.Background(), args, func() time.Time { return clock }, &out, &errb)
		return out.String(), err
	}
	stamp := func() string { return filepath.Join(dir, clock.UTC().Format("20060102T150405Z")) }

	for _, tc := range []struct {
		name    string
		answers []string
		args    []string
		want    []string // in the output
		files   []string // in the run's directory
		prompt  string   // in the first prompt
	}{
		{
			name: "first run, menu route on a format", answers: []string{answer(map[string]any{"route": "rancher_sprinter"})},
			args:  []string{"-format", "sprint_91"},
			want:  []string{"route     rancher_sprinter (menu route)", "start rancher", "-route rancher_sprinter -format sprint_91 -persona ", `"_format":"sprint_91"`, "-choice " + filepath.Join(dir, "20261112T210000Z", "choice.json")},
			files: []string{"choice.json", "format/rancher_sprinter.json"},
		},
		{
			name: "the last route is off the menu", answers: []string{answer(map[string]any{"route": "rancher_sprinter"}), answer(map[string]any{"route": "smelter_sprinter"})},
			args:   []string{"-format", "sprint_91"},
			want:   []string{"route     smelter_sprinter", "excluded  rancher_sprinter (played last)"},
			files:  []string{"choice.json", "format/smelter_sprinter.json"},
			prompt: "The last run played rancher_sprinter, so it is off tonight's menu",
		},
		{
			name: "a composition plays from its own file", answers: []string{answer(map[string]any{"compose": map[string]any{"name": "big_vintner", "base": "vintner", "levers": map[string]any{"bays": 3}}})},
			want:   []string{"route     big_vintner (composed: vintner with bays 3)", "-route " + filepath.Join(dir, "20261112T230000Z", "route", "big_vintner.json") + " -persona "},
			files:  []string{"choice.json", "route/big_vintner.json"},
			prompt: "The last run played smelter_sprinter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, prompts := fakeLLM(t, tc.answers...)
			out, err := choose(srv, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output lacks %q:\n%s", w, out)
				}
			}
			for _, f := range tc.files {
				if _, err := os.Stat(filepath.Join(stamp(), f)); err != nil {
					t.Error(err)
				}
			}
			if !strings.Contains((*prompts)[0], tc.prompt) {
				t.Errorf("prompt lacks %q", tc.prompt)
			}
		})
	}
	if got := agency.Recent(dir, ""); strings.Join(got, ",") != "rancher_sprinter,smelter_sprinter,big_vintner" {
		t.Errorf("recent %v", got)
	}

	// A dry run records nothing; a composition played last is no menu route, so nothing is excluded.
	// An endpoint that answers nothing is an error, not a fallback.
	srv, _ := fakeLLM(t, answer(map[string]any{"route": "smelter"}))
	if out, err := choose(srv, "-dry-run"); err != nil || !strings.Contains(out, "DRY RUN") || strings.Contains(out, "excluded") {
		t.Errorf("dry run: %v\n%s", err, out)
	}
	srv, _ = fakeLLM(t)
	if _, err := choose(srv); err == nil || !strings.Contains(err.Error(), "no answer") {
		t.Errorf("dead endpoint: %v", err)
	}
	if n := len(agency.Recent(dir, "")); n != 3 {
		t.Errorf("%d choices recorded, want 3", n)
	}
	if _, err := choose(srv, "-format", "nonesuch"); err == nil {
		t.Error("unknown format")
	}
}

// The card's persona writes the thesis and plays the run, whatever route the model picks.
func TestChoosePersona(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "choices")
	t.Chdir(filepath.Join("..", ".."))
	srv, prompts := fakeLLM(t, answer(map[string]any{"route": "vintner"}))
	var out, errb bytes.Buffer
	args := []string{"-llm-url", srv.URL, "-model", "fake", "-dir", dir, "-lean", "-persona", "botanist"}
	if err := run(context.Background(), args, time.Now, &out, &errb); err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{"route     vintner (menu route)", "persona   botanist (the card's)", "-route vintner -persona botanist"} {
		if !strings.Contains(out.String(), w) {
			t.Errorf("output lacks %q:\n%s", w, out.String())
		}
	}
	if p := (*prompts)[0]; !strings.Contains(p, "the run's card makes you this archetype") || !strings.Contains(p, "\nBotanist\nPath: plants clean the water.") || strings.Contains(p, "## Notes") {
		t.Errorf("prompt lacks the persona's prompt part, or carries its notes:\n%s", p)
	}
	if err := run(context.Background(), []string{"-llm-url", srv.URL, "-dir", dir, "-persona", "../x"}, time.Now, &out, &errb); err == nil {
		t.Error("a missing persona")
	}
}
