package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/protocol/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	scenarios = "../../internal/policy/testdata/scenarios"
	fixtures  = "../../protocol/fixtures"
)

func TestScenariosAreValid(t *testing.T) {
	raw, err := schema.FS.ReadFile("decide.request.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("decide.json", doc); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("decide.json")
	if err != nil {
		t.Fatal(err)
	}
	cases, err := loadScenarios(scenarios)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 12 {
		t.Fatalf("only %d scenarios", len(cases))
	}
	for _, ec := range cases {
		t.Run(ec.Name, func(t *testing.T) {
			b, _ := os.ReadFile(filepath.Join(scenarios, ec.Name+".json"))
			var f struct {
				Request json.RawMessage `json:"request"`
			}
			json.Unmarshal(b, &f)
			inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(f.Request))
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(inst); err != nil {
				t.Fatalf("request does not match the decide schema: %v", err)
			}
			if ec.Note == "" {
				t.Error("no note")
			}
			for _, id := range ec.Expect {
				if id != protocol.OptionDefer && !slices.ContainsFunc(ec.Request.Options, func(o protocol.Option) bool { return o.ID == id }) {
					t.Errorf("expect %q is not an offered option", id)
				}
			}
		})
	}
}

func TestHeuristicPassesScenarios(t *testing.T) {
	var out bytes.Buffer
	if err := run(context.Background(), []string{"-fixtures", fixtures, "-scenarios", scenarios, "-json"}, noEnv, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	var rep struct {
		Results []result `json:"results"`
		Totals  totals   `json:"totals"`
	}
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	for _, r := range rep.Results {
		if r.Acceptable != nil && !*r.Acceptable {
			t.Errorf("%s: heuristic chose %s (%s), want one of %v", r.Name, r.Option, r.Rationale, r.Expect)
		}
		if r.Error != "" {
			t.Errorf("%s: %s", r.Name, r.Error)
		}
	}
	fx, err := loadFixtures(fixtures)
	if err != nil {
		t.Fatal(err)
	}
	if len(fx) < 2 {
		t.Fatalf("only %d fixtures; the stage fixture is missing", len(fx))
	}
	if rep.Totals.Scored == 0 || rep.Totals.Cases != rep.Totals.Scored+len(fx) {
		t.Errorf("totals %+v", rep.Totals)
	}
}

func noEnv(string) string { return "" }

// fakeModel answers every call with text and records the request bodies.
func fakeModel(t *testing.T, text string, bodies *[]map[string]any) string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		*bodies = append(*bodies, b)
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRun(t *testing.T) {
	logDir := t.TempDir()
	logLine := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	fx, _ := os.ReadFile(filepath.Join(fixtures, "decide.request.json"))
	var req map[string]any
	json.Unmarshal(fx, &req)
	os.WriteFile(filepath.Join(logDir, "s-1.jsonl"), []byte(strings.Join([]string{
		logLine(map[string]any{"type": "hello"}),
		logLine(map[string]any{"type": "decision", "decision_id": "d-77", "request": req}),
		"not json",
	}, "\n")+"\n"), 0o644)

	tests := []struct {
		name     string
		args     []string
		answer   string
		want     []string
		wantErr  string
		wantBody map[string]string
	}{
		{"heuristic table", []string{"-run", "obvious|nothing"}, "", []string{
			"CASE", "obvious-sale  scenario  1    sell_all", "nothing-staged", "acceptable 2/2  defers 1",
		}, "", nil},
		{"llm valid", []string{"-policy", "llm", "-run", "^obvious-sale$", "-thinking", "off", "-no-schema", "-temperature", "0.2"},
			`{"rationale":"Fine.","option":"sell_all","params":{}}`,
			[]string{"sell_all  llm  yes", "acceptable 1/1  defers 0  fallbacks 0  repairs 0"}, "",
			map[string]string{"chat_template_kwargs": `{"enable_thinking":false}`, "temperature": "0.2", "response_format": ""}},
		{"llm garbage falls back", []string{"-policy", "llm", "-run", "^obvious-sale$", "-repeat", "2"}, `no idea`,
			[]string{"defer  fallback  NO", "ERR model failed", "cases 2  acceptable 0/2  defers 2  fallbacks 2  repairs 2"}, "", nil},
		{"logs", []string{"-scenarios", "", "-fixtures", "", "-logs", filepath.Join(logDir, "*.jsonl")}, "",
			[]string{"d-77  log  1    sell_all  heuristic  -"}, "", nil},
		{"llm needs a model", []string{"-policy", "llm"}, "", nil, "needs -base-url and -model", nil},
		{"bad thinking", []string{"-policy", "llm", "-base-url", "http://x", "-model", "m", "-thinking", "maybe"}, "", nil, "-thinking", nil},
		{"filter matches nothing", []string{"-run", "^zzz$"}, "", nil, "no cases", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodies []map[string]any
			args := append([]string{"-fixtures", fixtures, "-scenarios", scenarios}, tt.args...)
			if tt.answer != "" {
				args = append(args, "-base-url", fakeModel(t, tt.answer, &bodies), "-model", "m")
			}
			var out bytes.Buffer
			err := run(context.Background(), args, noEnv, &out, io.Discard)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			flat := strings.Join(strings.Fields(out.String()), " ")
			for _, w := range tt.want {
				if !strings.Contains(flat, strings.Join(strings.Fields(w), " ")) {
					t.Errorf("output missing %q:\n%s", w, out.String())
				}
			}
			for k, want := range tt.wantBody {
				got, has := bodies[0][k]
				gb, _ := json.Marshal(got)
				if want == "" && has || want != "" && string(gb) != want {
					t.Errorf("body %s = %s, want %q", k, gb, want)
				}
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	yes, no := true, false
	rs := []result{
		{Option: "sell_all", By: "llm", LatencyMS: 9000, Attempts: 1, Acceptable: &yes},
		{Option: "defer", By: "fallback", LatencyMS: 100, Attempts: 2, Acceptable: &no},
	}
	for i := 1; i <= 18; i++ {
		rs = append(rs, result{Option: "dismiss", By: "llm", LatencyMS: int64(i * 100), Attempts: 1})
	}
	got := summarize(rs)
	want := totals{Cases: 20, Defers: 1, Fallbacks: 1, Repairs: 1, Scored: 2, Acceptable: 1, ColdMS: 9000, P50MS: 900, P95MS: 1800, MaxMS: 1800}
	if got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}
