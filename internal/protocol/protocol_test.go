package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/protocol/schema"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const fixturesDir = "../../protocol/fixtures"

// fixtureTypes maps fixture name patterns (filepath.Match) to a schema and Go type. Every
// fixture must match exactly one pattern, so a new message cannot skip the Go side.
var fixtureTypes = []struct {
	pattern, schema string
	new             func() any
}{
	{"hello.request.json", "hello.request.schema.json", func() any { return new(HelloRequest) }},
	{"hello.response.json", "hello.response.schema.json", func() any { return new(HelloResponse) }},
	{"event.*.json", "event.request.schema.json", func() any { return new(Event) }},
	{"decide.request.json", "decide.request.schema.json", func() any { return new(DecideRequest) }},
	{"decide.*.request.json", "decide.request.schema.json", func() any { return new(DecideRequest) }},
	{"decide.response*.json", "decide.response.schema.json", func() any { return new(DecideResponse) }},
	{"decide.*.response.json", "decide.response.schema.json", func() any { return new(DecideResponse) }},
	{"outcome.request.json", "outcome.request.schema.json", func() any { return new(Outcome) }},
	{"summary.response.json", "summary.response.schema.json", func() any { return new(SummaryResponse) }},
}

func fixtureType(name string) (schema string, newV func() any, err error) {
	n := 0
	for _, m := range fixtureTypes {
		if ok, _ := filepath.Match(m.pattern, name); ok {
			schema, newV = m.schema, m.new
			n++
		}
	}
	switch n {
	case 0:
		return "", nil, fmt.Errorf("fixture %s matches no fixtureTypes pattern: map it to a schema and Go type", name)
	case 1:
		return schema, newV, nil
	}
	return "", nil, fmt.Errorf("fixture %s matches %d fixtureTypes patterns", name, n)
}

func compileSchemas(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	entries, err := schema.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	var names []string
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".schema.json") {
			continue
		}
		b, err := schema.FS.ReadFile(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if err := c.AddResource(e.Name(), doc); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		names = append(names, e.Name())
	}
	out := make(map[string]*jsonschema.Schema, len(names))
	for _, n := range names {
		s, err := c.Compile(n)
		if err != nil {
			t.Fatalf("compile %s: %v", n, err)
		}
		out[n] = s
	}
	return out
}

func TestFixtures(t *testing.T) {
	schemas := compileSchemas(t)
	files, err := filepath.Glob(filepath.Join(fixturesDir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures found in %s: %v", fixturesDir, err)
	}
	covered := map[string]bool{}
	for _, path := range files {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			schemaName, newV, err := fixtureType(name)
			if err != nil {
				t.Fatal(err)
			}
			sch, ok := schemas[schemaName]
			if !ok {
				t.Fatalf("schema %s does not exist", schemaName)
			}
			covered[schemaName] = true
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if err := sch.Validate(inst); err != nil {
				t.Fatalf("fixture does not validate against %s:\n%v", schemaName, err)
			}

			v := newV()
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(v); err != nil {
				t.Fatalf("decode into %T: %v", v, err)
			}
			enc, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			var want, got any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(enc, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(want, got) {
				t.Fatalf("round-trip mismatch for %T\nwant %s\ngot  %s", v, raw, enc)
			}
		})
	}
	for name := range schemas {
		if !covered[name] {
			t.Errorf("schema %s has no fixture", name)
		}
	}
}

// Fixtures only show the happy path; these pin what the schemas must reject.
func TestSchemasReject(t *testing.T) {
	schemas := compileSchemas(t)
	tests := []struct {
		name, schema, doc string
	}{
		{"defer as option id", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"barter","game_day":1,"deadline_ms":10,"context":{},"options":[{"id":"defer","label":"x"}]}`},
		{"int param without bounds", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"barter","game_day":1,"deadline_ms":10,"context":{},"options":[{"id":"c","label":"x","params":{"price":{"type":"int"}}}]}`},
		{"unknown param type", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"barter","game_day":1,"deadline_ms":10,"context":{},"options":[{"id":"c","label":"x","params":{"p":{"type":"float","min":1,"max":2}}}]}`},
		{"no options", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"barter","game_day":1,"deadline_ms":10,"context":{},"options":[]}`},
		{"zero deadline", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"barter","game_day":1,"deadline_ms":0,"context":{},"options":[{"id":"a","label":"x"}]}`},
		{"unknown source", "decide.response.schema.json",
			`{"decision_id":"d","choice":{"option":"defer"},"source":"oracle"}`},
		{"unknown mode", "hello.response.schema.json",
			`{"session_id":"s","protocol":1,"mode":"autopilot"}`},
		{"option note not a string", "decide.response.schema.json",
			`{"decision_id":"d","choice":{"option":"defer"},"source":"llm","option_notes":{"a":1}}`},
		{"option note too long", "decide.response.schema.json",
			`{"decision_id":"d","choice":{"option":"defer"},"source":"llm","option_notes":{"a":"` + strings.Repeat("x", 121) + `"}}`},
		{"summary without today", "summary.response.schema.json",
			`{"session_id":"s","mode":"act","policy":"heuristic","model":"","day":0,"lessons":"","lessons_day":0}`},
		{"summary rep delta not a number", "summary.response.schema.json",
			`{"session_id":"s","mode":"act","policy":"heuristic","model":"","day":3,"today":{"decisions":0,"sales":0,"purchases":0,"cash_delta":0,"walkouts":0,"rep_delta":{"cartel":"up"}},"lessons":"","lessons_day":0}`},
		{"event data not object", "event.request.schema.json",
			`{"session_id":"s","seq":1,"kind":"k","game_day":1,"data":[]}`},
		{"event hidden not object", "event.request.schema.json",
			`{"session_id":"s","seq":1,"kind":"k","game_day":1,"data":{},"hidden":[1]}`},
		{"summary plan too long", "summary.response.schema.json",
			`{"session_id":"s","mode":"act","policy":"heuristic","model":"","day":3,"today":{"decisions":0,"sales":0,"purchases":0,"cash_delta":0,"walkouts":0,"rep_delta":{}},"lessons":"","lessons_day":0,"plan":"` + strings.Repeat("x", 2049) + `","plan_day":3}`},
		{"unknown top-level field", "outcome.request.schema.json",
			`{"session_id":"s","decision_id":"d","game_day":1,"result":{},"extra":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := jsonschema.UnmarshalJSON(strings.NewReader(tt.doc))
			if err != nil {
				t.Fatal(err)
			}
			if err := schemas[tt.schema].Validate(inst); err == nil {
				t.Fatalf("%s accepted %s", tt.schema, tt.doc)
			}
		})
	}
}

func i64(v int64) *int64 { return &v }

func TestDecideRequestValidate(t *testing.T) {
	base := func() DecideRequest {
		return DecideRequest{
			SessionID: "s", DecisionID: "d", Kind: "barter", DeadlineMS: 1000,
			Context: json.RawMessage(`{}`),
			Options: []Option{{ID: "a", Label: "A"}},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*DecideRequest)
		wantErr string
	}{
		{"valid", func(*DecideRequest) {}, ""},
		{"no session", func(r *DecideRequest) { r.SessionID = "" }, "session_id"},
		{"no decision", func(r *DecideRequest) { r.DecisionID = "" }, "decision_id"},
		{"no kind", func(r *DecideRequest) { r.Kind = "" }, "kind"},
		{"negative deadline", func(r *DecideRequest) { r.DeadlineMS = -1 }, "deadline_ms"},
		{"context array", func(r *DecideRequest) { r.Context = json.RawMessage(`[1]`) }, "context"},
		{"no options", func(r *DecideRequest) { r.Options = nil }, "options"},
		{"hidden object", func(r *DecideRequest) { r.Hidden = json.RawMessage(`{"a":1}`) }, ""},
		{"hidden null", func(r *DecideRequest) { r.Hidden = json.RawMessage(`null`) }, ""},
		{"hidden array", func(r *DecideRequest) { r.Hidden = json.RawMessage(`[1]`) }, "hidden"},
		{"defer id", func(r *DecideRequest) { r.Options[0].ID = "defer" }, "reserved"},
		{"dup id", func(r *DecideRequest) { r.Options = append(r.Options, Option{ID: "a"}) }, "duplicated"},
		{"min > max", func(r *DecideRequest) {
			r.Options[0].Params = map[string]Param{"p": {Type: "int", Min: i64(5), Max: i64(1)}}
		}, "min 5 > max 1"},
		{"unknown param type tolerated", func(r *DecideRequest) {
			r.Options[0].Params = map[string]Param{"p": {Type: "color"}}
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base()
			tt.mutate(&r)
			err := r.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestOptionChoosable(t *testing.T) {
	tests := []struct {
		name string
		opt  Option
		want bool
	}{
		{"no params", Option{ID: "a"}, true},
		{"int", Option{ID: "a", Params: map[string]Param{"p": {Type: "int", Min: i64(1), Max: i64(2)}}}, true},
		{"int missing max", Option{ID: "a", Params: map[string]Param{"p": {Type: "int", Min: i64(1)}}}, false},
		{"unknown type", Option{ID: "a", Params: map[string]Param{"p": {Type: "item_ref"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.opt.Choosable(); got != tt.want {
				t.Fatalf("Choosable() = %v, want %v", got, tt.want)
			}
		})
	}
}

// No fixture uses int params since barter became discrete; these keep the branch tested.
func TestSchemasAccept(t *testing.T) {
	schemas := compileSchemas(t)
	tests := []struct {
		name, schema, doc string
	}{
		{"int param request", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"loan","game_day":1,"deadline_ms":10,"context":{},"options":[{"id":"lend","label":"Lend","params":{"amount":{"type":"int","min":1,"max":500}}}]}`},
		{"without state_signature", "decide.request.schema.json",
			`{"session_id":"s","decision_id":"d","kind":"barter","game_day":1,"deadline_ms":10,"context":{},"options":[{"id":"dismiss","label":"x"}]}`},
		{"int param response", "decide.response.schema.json",
			`{"decision_id":"d","choice":{"option":"lend","params":{"amount":140}},"source":"llm"}`},
		{"summary with memory off", "summary.response.schema.json",
			`{"session_id":"s","mode":"act","policy":"heuristic","model":"","day":0,"today":{"decisions":0,"sales":0,"purchases":0,"cash_delta":0,"walkouts":0,"rep_delta":{}},"lessons":"","lessons_day":0}`},
		{"summary with plan", "summary.response.schema.json",
			`{"session_id":"s","mode":"observe","policy":"heuristic","model":"","day":69,"today":{"decisions":0,"sales":0,"purchases":0,"cash_delta":0,"walkouts":0,"rep_delta":{}},"lessons":"","lessons_day":0,"plan":"Hold the cassettes.","plan_day":69}`},
		{"empty hash", "hello.request.schema.json",
			`{"protocol":1,"mod_version":"0.1.0","game":{"version":"x","unity":"y","assembly_sha256":""},"compat":{"ok":true,"missing":[],"disabled_features":[]}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst, err := jsonschema.UnmarshalJSON(strings.NewReader(tt.doc))
			if err != nil {
				t.Fatal(err)
			}
			if err := schemas[tt.schema].Validate(inst); err != nil {
				t.Fatalf("%s rejected %s: %v", tt.schema, tt.doc, err)
			}
		})
	}
}

func TestIntParamsRoundTrip(t *testing.T) {
	in := `{"session_id":"s","decision_id":"d","kind":"loan","game_day":1,"deadline_ms":10,"state_signature":"x","context":{"a":1},"options":[{"id":"lend","label":"Lend","params":{"amount":{"type":"int","min":0,"max":500}}}]}`
	var r DecideRequest
	if err := json.Unmarshal([]byte(in), &r); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(r)
	var a, b any
	json.Unmarshal([]byte(in), &a)
	json.Unmarshal(out, &b)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("round trip lost data:\n%s\n%s", in, out)
	}
}

func TestPolicyViewDropsHidden(t *testing.T) {
	r := &DecideRequest{DecisionID: "d", Context: json.RawMessage(`{}`), Hidden: json.RawMessage(`{"appraisal_value":95}`)}
	v := r.PolicyView()
	if v.Hidden != nil {
		t.Fatalf("view kept hidden: %s", v.Hidden)
	}
	if string(r.Hidden) != `{"appraisal_value":95}` || v.DecisionID != "d" {
		t.Fatalf("original mutated or view incomplete: %+v %+v", r, v)
	}
}

func TestEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		ev      Event
		wantErr string
	}{
		{"valid", Event{SessionID: "s", Kind: "k", Seq: 1}, ""},
		{"hidden object", Event{SessionID: "s", Kind: "k", Hidden: json.RawMessage(`{"a":1}`)}, ""},
		{"hidden null", Event{SessionID: "s", Kind: "k", Hidden: json.RawMessage(`null`)}, ""},
		{"hidden array", Event{SessionID: "s", Kind: "k", Hidden: json.RawMessage(`[1]`)}, "hidden"},
		{"no session", Event{Kind: "k"}, "session_id"},
		{"negative seq", Event{SessionID: "s", Kind: "k", Seq: -1}, "seq"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ev.Validate()
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("error %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestEventRecorderViewDropsHidden(t *testing.T) {
	ev := Event{SessionID: "s", Kind: "day_snapshot", Data: json.RawMessage(`{}`), Hidden: json.RawMessage(`{"days_to_inspection":3}`)}
	v := ev.RecorderView()
	if v.Hidden != nil || string(ev.Hidden) == "" || v.Kind != "day_snapshot" {
		t.Fatalf("view %+v, original %+v", v, ev)
	}
}
