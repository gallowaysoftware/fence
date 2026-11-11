package strategy

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
)

func TestParseProposals(t *testing.T) {
	tests := []struct {
		name, raw, base string
		want            []string // name=directives
		dropped         string
	}{
		{"contract and gym keys", `{"candidates":[{"name":"take now","why":"cash covers 2 payments","directives":{"mortgage":"take","repo_reserve_payments":2}},
			{"name":"lend","why":"Georgio","directives":{"georgio":"lend_25","georgio_peak_days":[40,43]}}]}`, "",
			[]string{`take now={"mortgage":"take","repo_reserve_payments":2}`, `lend={"georgio":"lend_25","georgio_peak_days":[40,43]}`}, ""},
		{"bad values dropped", `{"candidates":[{"name":"x","directives":{"georgio":"all","bargain":"wild"}},{"name":"y","directives":{"cash_floor":300}}]}`, "",
			[]string{`y={"cash_floor":300}`}, "x: no usable change"},
		{"engine part of the base reverted", `{"candidates":[{"name":"wine","directives":{"engine":"wine","permit":"bribe"}}]}`, `{"engine":"wine+furnace"}`,
			[]string{`wine={"permit":"bribe"}`}, ""},
		{"at most three", `{"candidates":[{"name":"a","directives":{"cash_floor":1}},{"name":"b","directives":{"cash_floor":2}},{"name":"c","directives":{"cash_floor":3}},{"name":"d","directives":{"cash_floor":4}}]}`, "",
			[]string{`a={"cash_floor":1}`, `b={"cash_floor":2}`, `c={"cash_floor":3}`}, "d: more than 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dropped, _ := parseProposals(json.RawMessage(tt.raw), shop(t, nil), tt.base)
			var names []string
			for _, p := range got {
				names = append(names, p.Name+"="+string(p.Directives))
			}
			eq(t, strings.Join(names, "\n"), strings.Join(tt.want, "\n"))
			if tt.dropped != "" && !strings.Contains(strings.Join(dropped, "\n"), tt.dropped) {
				t.Errorf("dropped %q, want %q", dropped, tt.dropped)
			}
		})
	}
}

type cannedLLM struct {
	text string
	req  llm.Request
}

func (c *cannedLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	c.req = req
	return llm.Response{Text: c.text, Model: "canned"}, nil
}

func TestPropose(t *testing.T) {
	step, _ := json.Marshal(map[string]any{
		"snapshot": json.RawMessage(directivesShop),
		"risks":    map[string]any{"mortgage": map[string]any{"paying": false, "down_payment": 3000}},
	})
	c := &cannedLLM{text: "<think>x</think>```json\n{\"candidates\":[{\"name\":\"decline\",\"why\":\"payments exceed income\",\"directives\":{\"mortgage\":\"decline\"}}]}\n```"}
	r := Propose(context.Background(), c, ProposeRequest{Day: 41, Difficulty: "hard", Step: step, Base: json.RawMessage(`{"permit":"buy"}`),
		Triggers: []ProposeTrigger{{ID: "mortgage_offer", Text: "the loan shark offers"}}, HoldDays: 7, HorizonDays: 42, Samples: 4}, 800)
	if r.Error != "" || len(r.Candidates) != 1 || string(r.Candidates[0].Directives) != `{"mortgage":"decline"}` {
		t.Fatalf("result %+v", r)
	}
	prompt := c.req.Messages[0].Content
	for _, want := range []string{"mortgage_offer: the loan shark offers", `BASE POLICY of this run (over the rules above): {"permit":"buy"}`, "PLAN IN FORCE (changes over the base): {}", `"down_payment":3000`, "Propose 3 alternatives"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestProposalJSONRepair(t *testing.T) {
	tests := []struct {
		name, text, want string
	}{
		{"as written", `{"candidates":[{"name":"a","why":"x","directives":{"cash_floor":1}}]}`, `{"candidates":[{"name":"a","why":"x","directives":{"cash_floor":1}}]}`},
		{"candidate brace missing", `{"candidates":[{"name":"a","why":"x","directives":{"cash_floor":1}]}`, `{"candidates":[{"name":"a","why":"x","directives":{"cash_floor":1}}]}`},
		{"cut mid-string", `{"candidates":[{"name":"a","why":"x","directives":{"cash_floor":1}},{"name":"b","why":"the rent is du`, `{"candidates":[{"name":"a","why":"x","directives":{"cash_floor":1}},{"name":"b","why":"the rent is du"}]}`},
		{"brace in a string kept", "```json\n" + `{"candidates":[{"name":"a]","why":"}x{","directives":{}}` + "\n```", `{"candidates":[{"name":"a]","why":"}x{","directives":{}}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := proposalJSON(tt.text)
			if err != nil {
				raw, err = proposalJSON(closeJSON(tt.text))
			}
			if err != nil {
				t.Fatal(err)
			}
			eq(t, string(raw), tt.want)
		})
	}
}

type scriptLLM struct {
	answers []string
	reqs    []llm.Request
}

func (c *scriptLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) {
	c.reqs = append(c.reqs, req)
	a := c.answers[min(len(c.reqs), len(c.answers))-1]
	return llm.Response{Text: a, Model: "script"}, nil
}

func TestProposeRepairCall(t *testing.T) {
	step, _ := json.Marshal(map[string]any{"snapshot": json.RawMessage(directivesShop)})
	c := &scriptLLM{answers: []string{`I would decline. {"name":"x"`, `{"candidates":[{"name":"decline","why":"w","directives":{"mortgage":"decline"}}]}`}}
	r := Propose(context.Background(), c, ProposeRequest{Day: 41, Step: step, HoldDays: 7, HorizonDays: 42, CommitHorizonDays: 126, Samples: 4}, 800)
	if r.Error != "" || r.Repair != "model" || len(r.Candidates) != 1 || len(c.reqs) != 2 {
		t.Fatalf("result %+v after %d calls", r, len(c.reqs))
	}
	if len(c.reqs[0].Schema) == 0 || !strings.Contains(c.reqs[0].Messages[0].Content, "126 days when any candidate") {
		t.Errorf("first call: schema %s, prompt without the commitment horizon", c.reqs[0].Schema)
	}
	if m := c.reqs[1].Messages; len(m) != 3 || m[1].Role != llm.RoleAssistant || !strings.Contains(m[2].Content, "not valid JSON") {
		t.Errorf("repair call messages %+v", m)
	}
}
