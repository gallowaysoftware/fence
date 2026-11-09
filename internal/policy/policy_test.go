package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func i64(v int64) *int64 { return &v }

func fixtureRequest(t *testing.T) *protocol.DecideRequest {
	t.Helper()
	return fixtureNamed(t, "decide.request.json")
}

func fixtureNamed(t *testing.T, name string) *protocol.DecideRequest {
	t.Helper()
	b, err := os.ReadFile("../../protocol/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var r protocol.DecideRequest
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

func withContext(t *testing.T, ctx string) *protocol.DecideRequest {
	r := fixtureRequest(t)
	r.Context = json.RawMessage(ctx)
	return r
}

var barterOpts = []protocol.Option{
	{ID: "accept", Label: "Accept"},
	{ID: "counter", Label: "Counter", Params: map[string]protocol.Param{"price": {Type: "int", Min: i64(1), Max: i64(400)}}},
	{ID: "refuse", Label: "Refuse"},
	{ID: "gift", Label: "Future", Params: map[string]protocol.Param{"item": {Type: "item_ref"}}},
}

func params(kv ...string) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = json.RawMessage(kv[i+1])
	}
	return m
}

func TestValidateChoice(t *testing.T) {
	tests := []struct {
		name    string
		choice  protocol.Choice
		wantErr string
	}{
		{"defer", protocol.Choice{Option: "defer"}, ""},
		{"defer with params", protocol.Choice{Option: "defer", Params: params("x", "1")}, "takes no params"},
		{"accept", protocol.Choice{Option: "accept"}, ""},
		{"counter", protocol.Choice{Option: "counter", Params: params("price", "140")}, ""},
		{"counter at min", protocol.Choice{Option: "counter", Params: params("price", "1")}, ""},
		{"counter at max", protocol.Choice{Option: "counter", Params: params("price", "400")}, ""},
		{"unknown option", protocol.Choice{Option: "steal"}, `valid options are "accept", "counter", "refuse", "defer"`},
		{"missing param", protocol.Choice{Option: "counter"}, `param "price" is required`},
		{"below min", protocol.Choice{Option: "counter", Params: params("price", "0")}, "outside [1, 400]"},
		{"above max", protocol.Choice{Option: "counter", Params: params("price", "401")}, "outside [1, 400]"},
		{"float", protocol.Choice{Option: "counter", Params: params("price", "140.5")}, "must be an integer"},
		{"float-looking int", protocol.Choice{Option: "counter", Params: params("price", "140.0")}, "must be an integer"},
		{"string", protocol.Choice{Option: "counter", Params: params("price", `"140"`)}, "must be an integer"},
		{"undeclared", protocol.Choice{Option: "counter", Params: params("price", "140", "tip", "5")}, `"tip" is not declared`},
		{"param on paramless", protocol.Choice{Option: "accept", Params: params("price", "1")}, "not declared"},
		{"unsupported param type", protocol.Choice{Option: "gift", Params: params("item", `"x"`)}, "cannot fill"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateChoice(barterOpts, tt.choice)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Fatalf("unexpected: %v", err)
			case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
				t.Fatalf("err %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// editCtx is the fixture context with overrides; a nil value deletes the key path.
func editCtx(t *testing.T, edits map[string]any) json.RawMessage {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(fixtureRequest(t).Context, &m); err != nil {
		t.Fatal(err)
	}
	for path, v := range edits {
		keys := strings.Split(path, ".")
		cur := m
		for _, k := range keys[:len(keys)-1] {
			switch next := cur[k].(type) {
			case map[string]any:
				cur = next
			case []any:
				cur = next[0].(map[string]any)
			default:
				t.Fatalf("bad path %s", path)
			}
		}
		if v == nil {
			delete(cur, keys[len(keys)-1])
		} else {
			cur[keys[len(keys)-1]] = v
		}
	}
	b, _ := json.Marshal(m)
	return b
}

func opts(ids ...string) []protocol.Option {
	var o []protocol.Option
	for _, id := range ids {
		o = append(o, protocol.Option{ID: id, Label: id})
	}
	return o
}

func TestHeuristic(t *testing.T) {
	buyOpts := []protocol.Option{
		{ID: "buy_all", Label: "Buy 1 item for 340"},
		{ID: "offer_markup_10", Label: "Offer 10% markup (always accepted)"},
		{ID: "demand_discount_10", Label: "Demand 10% discount (accept 82%, else stay 90%, else walk out -3 rep)"},
		{ID: "demand_discount_25", Label: "Demand 25% discount (accept 52%, else stay 72%, else walk out -3 rep)"},
		{ID: "dismiss", Label: "Send the customer away"},
	}
	wholesaleOpts := append([]protocol.Option{{ID: "accept_wholesale", Label: "Accept wholesale for 250"}}, buyOpts...)
	item := map[string]any{"uid": 1, "id": "radio", "name": "Radio", "price": 200, "estimated_value": 300,
		"features": []any{}, "stolen_heat": nil, "contraband_level": 0, "serious_contraband": false}
	with := func(base map[string]any, kv ...any) map[string]any {
		m := map[string]any{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i].(string)] = kv[i+1]
		}
		return m
	}
	selling := func(it map[string]any) map[string]any {
		return map[string]any{"client.intent": "sell", "client.budget": nil, "counter.yours_staged": []any{}, "counter.theirs": []any{it}}
	}
	merge := func(ms ...map[string]any) map[string]any {
		out := map[string]any{}
		for _, m := range ms {
			for k, v := range m {
				out[k] = v
			}
		}
		return out
	}
	sureMarkup := opts("sell_all")
	sureMarkup = append(sureMarkup, protocol.Option{ID: "add_markup_10", Label: "Add 10% markup (accept 98%, else stay 99%, else walk out -3 rep)"})
	tests := []struct {
		name       string
		kind       string
		edits      map[string]any
		ctx        string
		opts       []protocol.Option
		wantOption string
	}{
		// 340 is already over the 300 estimate and within budget; the 10% markup's
		// 82% accept / 8% refuse / 10% walk-out is worth 0.82*374 + 0.08*340 = 334 < 340.
		{"fixture sells outright", "barter", nil, "", nil, "sell_all"},
		{"near-certain markup is taken", "barter", nil, "", sureMarkup, "add_markup_10"},
		{"near-certain markup skipped when faction in danger", "barter", map[string]any{"reputation.lower_levels.value": -70.0}, "", sureMarkup, "sell_all"},
		{"over budget offers a discount", "barter", map[string]any{"client.budget": 300}, "", nil, "offer_discount_10"},
		{"over budget, capped sale under cost defers", "barter", map[string]any{"client.budget": 150}, "", opts("sell_all", "dismiss"), "defer"},
		{"selling under cost defers", "barter", map[string]any{"counter.yours_staged.price": 150}, "", nil, "defer"},
		{"no cost basis still sells", "barter", map[string]any{"counter.yours_staged.bought_at": nil, "counter.yours_staged.price": 150}, "", nil, "sell_all"},
		{"stolen to security defers", "barter", map[string]any{"client.is_security": true}, "", nil, "defer"},
		{"contraband to a refuser defers", "barter", map[string]any{"client.no_contraband": true, "counter.yours_staged.contraband_level": 2}, "", nil, "defer"},
		{"nothing staged defers", "barter", map[string]any{"counter.yours_staged": []any{}}, "", nil, "defer"},
		{"no bargain flags still works", "barter", map[string]any{"bargain": nil}, "", nil, "sell_all"},
		{"no budget defers", "barter", map[string]any{"client.budget": nil}, "", nil, "defer"},
		{"no client defers", "barter", map[string]any{"client": nil}, "", nil, "defer"},
		{"no counter defers", "barter", map[string]any{"counter": nil}, "", nil, "defer"},
		{"no intent defers", "barter", map[string]any{"client.intent": nil}, "", nil, "defer"},
		{"old intent spelling defers", "barter", map[string]any{"client.intent": "buying"}, "", nil, "defer"},
		{"item without price defers", "barter", map[string]any{"counter.yours_staged.price": nil}, "", nil, "defer"},
		{"price as string defers", "barter", map[string]any{"counter.yours_staged.price": "340"}, "", nil, "defer"},
		{"empty context defers", "barter", nil, `{}`, nil, "defer"},
		{"context not object defers", "barter", nil, `[1]`, nil, "defer"},
		{"unknown kind defers", "supplier", nil, "", nil, "defer"},
		{"options it does not know defer", "barter", nil, "", opts("haggle"), "defer"},
		{"unparseable odds are skipped", "barter", nil, "", []protocol.Option{{ID: "add_markup_10", Label: "Add 10% markup"}, {ID: "sell_all", Label: "Sell"}}, "sell_all"},

		{"buy cheap", "barter", selling(with(item, "price", 150)), "", buyOpts, "buy_all"},
		// Margin 80 outright; 10% off is worth 0.82*102 + 0.08*80 = 90, over the 5% edge.
		{"buy fair haggles", "barter", selling(with(item, "price", 220)), "", buyOpts, "demand_discount_10"},
		// Margin 20 outright; 25% off at 52% accept / 20% refuse / 28% walk is worth 46.
		{"buy thin margin demands discount", "barter", selling(with(item, "price", 280)), "", buyOpts, "demand_discount_25"},
		{"buy thin margin, only the small step", "barter", selling(with(item, "price", 280)), "", buyOpts[:3], "demand_discount_10"},
		{"buy thin margin, discount not offered", "barter", selling(with(item, "price", 280)), "", opts("buy_all", "dismiss"), "buy_all"},
		{"buy thin margin, faction in danger", "barter", merge(selling(with(item, "price", 280)), map[string]any{"reputation.lower_levels.value": -65.0}), "", buyOpts, "buy_all"},
		{"buy over estimate tries a deep discount", "barter", selling(with(item, "price", 340)), "", buyOpts, "demand_discount_25"},
		{"long-shot discount is not taken", "barter", selling(with(item, "price", 400)), "", append(buyOpts[:1:1], protocol.Option{ID: "demand_discount_50", Label: "Demand 50% discount (accept 2%, else stay 42%, else walk out -3 rep)"}, buyOpts[4]), "dismiss"},
		{"buy far over estimate dismisses", "barter", selling(with(item, "price", 600)), "", buyOpts, "dismiss"},
		{"buy unaffordable dismisses", "barter", merge(selling(item), map[string]any{"store.cash": 100}), "", buyOpts, "dismiss"},
		{"buy too hot dismisses", "barter", selling(with(item, "price", 100, "stolen_heat", 60)), "", buyOpts, "dismiss"},
		{"buy warm is fine", "barter", selling(with(item, "price", 100, "stolen_heat", 30)), "", buyOpts, "buy_all"},
		{"buy serious contraband defers", "barter", selling(with(item, "serious_contraband", true, "contraband_level", 3)), "", buyOpts, "defer"},
		{"wholesale beats the lot", "barter", merge(selling(item), map[string]any{"client.wholesale": map[string]any{"price": 150, "discount_pct": 25}}), "", wholesaleOpts, "accept_wholesale"},
		{"wholesale over cash is skipped", "barter", merge(selling(item), map[string]any{"client.wholesale": map[string]any{"price": 260, "discount_pct": 5}, "store.cash": 250}), "", wholesaleOpts, "buy_all"},
		{"buy keeps rent due tomorrow", "barter", merge(selling(item), map[string]any{"store.cash": 700, "store.rent_days_left": 1}), "", buyOpts, "dismiss"},
		{"buy ignores rent due later", "barter", merge(selling(item), map[string]any{"store.cash": 700, "store.rent_days_left": 5}), "", buyOpts, "buy_all"},
		{"buy without cash defers", "barter", merge(selling(item), map[string]any{"store.cash": nil}), "", buyOpts, "defer"},
		{"buy with nothing offered defers", "barter", selling(with(item, "price", 600)), "", opts("offer_markup_10"), "defer"},
		{"sell_and_buy, only theirs", "barter", merge(selling(with(item, "price", 150)), map[string]any{"client.intent": "sell_and_buy"}), "", buyOpts, "buy_all"},
		{"sell_and_buy, both counters defers", "barter", map[string]any{"client.intent": "sell_and_buy", "counter.theirs": []any{item}}, "", nil, "defer"},
		{"sell_and_buy, only yours", "barter", map[string]any{"client.intent": "sell_and_buy"}, "", nil, "sell_all"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := fixtureRequest(t)
			req.Kind = tt.kind
			req.Context = editCtx(t, tt.edits)
			if tt.ctx != "" {
				req.Context = json.RawMessage(tt.ctx)
			}
			if tt.opts != nil {
				req.Options = tt.opts
			}
			d := Heuristic{}.Decide(context.Background(), req)
			if d.Source != protocol.SourceHeuristic {
				t.Errorf("source %s", d.Source)
			}
			if d.Choice.Option != tt.wantOption {
				t.Fatalf("option %q (%s), want %q", d.Choice.Option, d.Rationale, tt.wantOption)
			}
			if err := ValidateChoice(req.Options, d.Choice); err != nil {
				t.Fatalf("heuristic produced an invalid choice: %v", err)
			}
		})
	}
}

// scripted replays one reply per call and records what it was sent.
type scripted struct {
	mu      sync.Mutex
	replies []reply
	got     []llm.Request
}

type reply struct {
	text  string
	err   error
	delay time.Duration
	hang  bool
}

func (s *scripted) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	s.mu.Lock()
	i := len(s.got)
	s.got = append(s.got, req)
	s.mu.Unlock()
	if i >= len(s.replies) {
		return llm.Response{}, errors.New("unscripted call")
	}
	r := s.replies[i]
	if r.hang {
		<-ctx.Done()
		return llm.Response{}, ctx.Err()
	}
	if r.delay > 0 {
		select {
		case <-time.After(r.delay):
		case <-ctx.Done():
			return llm.Response{}, ctx.Err()
		}
	}
	return llm.Response{Text: r.text}, r.err
}

func (s *scripted) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.got)
}

func TestLLM(t *testing.T) {
	good := `{"rationale":"good odds","option":"add_markup_10","params":{}}`
	goodInt := `{"rationale":"soft ask","option":"counter","params":{"price":140}}`
	intReq := func(t *testing.T) *protocol.DecideRequest {
		r := fixtureRequest(t)
		r.Options = barterOpts
		return r
	}
	tests := []struct {
		name       string
		req        func(t *testing.T) *protocol.DecideRequest
		replies    []reply
		timeout    time.Duration
		wantSource protocol.Source
		wantOption string
		wantCalls  int
		wantNote   string
	}{
		{"valid first try", fixtureRequest, []reply{{text: good}}, 0, protocol.SourceLLM, "add_markup_10", 1, ""},
		{"params omitted for paramless option", fixtureRequest, []reply{{text: `{"rationale":"r","option":"dismiss"}`}}, 0, protocol.SourceLLM, "dismiss", 1, ""},
		{"wrapped in think and fences", fixtureRequest, []reply{{text: "<think>hmm {}</think>\n```json\n" + good + "\n```"}}, 0, protocol.SourceLLM, "add_markup_10", 1, ""},
		{"model defers", fixtureRequest, []reply{{text: `{"rationale":"?","option":"defer","params":{}}`}}, 0, protocol.SourceLLM, "defer", 1, ""},
		{"garbage then repaired", fixtureRequest, []reply{{text: "I would add a markup."}, {text: good}}, 0, protocol.SourceLLM, "add_markup_10", 2, ""},
		{"illegal option then repaired", fixtureRequest, []reply{{text: `{"option":"add_markup_50","params":{}}`}, {text: good}}, 0, protocol.SourceLLM, "add_markup_10", 2, ""},
		{"invented param then repaired", fixtureRequest, []reply{{text: `{"option":"sell_all","params":{"price":999}}`}, {text: good}}, 0, protocol.SourceLLM, "add_markup_10", 2, ""},
		{"int param valid", intReq, []reply{{text: goodInt}}, 0, protocol.SourceLLM, "counter", 1, ""},
		{"int param out of bounds then repaired", intReq, []reply{{text: `{"option":"counter","params":{"price":9999}}`}, {text: goodInt}}, 0, protocol.SourceLLM, "counter", 2, ""},
		{"int param out of bounds twice", intReq, []reply{
			{text: `{"option":"counter","params":{"price":9999}}`},
			{text: `{"option":"counter","params":{"price":-1}}`},
		}, 0, protocol.SourceFallback, "defer", 2, "outside"},
		{"int param missing twice", intReq, []reply{{text: `{"option":"counter","params":{}}`}, {text: `{"option":"counter"}`}}, 0, protocol.SourceFallback, "defer", 2, "required"},
		{"garbage twice", fixtureRequest, []reply{{text: "no"}, {text: ""}}, 0, protocol.SourceFallback, "defer", 2, "no JSON"},
		{"api error is not repaired", fixtureRequest, []reply{{err: errors.New("HTTP 529")}}, 0, protocol.SourceFallback, "defer", 1, "529"},
		{"hung model hits deadline", fixtureRequest, []reply{{hang: true}}, 50 * time.Millisecond, protocol.SourceFallback, "defer", 1, "deadline"},
		{"no time left to repair", fixtureRequest, []reply{{text: "junk", delay: 30 * time.Millisecond}}, 100 * time.Millisecond, protocol.SourceFallback, "defer", 1, "no time left"},
		{"unknown kind never calls", func(t *testing.T) *protocol.DecideRequest {
			r := fixtureRequest(t)
			r.Kind = "black_market_auction"
			return r
		}, nil, 0, protocol.SourceFallback, "defer", 0, "unknown decision kind"},
		{"huge context never calls", func(t *testing.T) *protocol.DecideRequest {
			return withContext(t, `{"blob":"`+strings.Repeat("x", DefaultMaxContextBytes)+`"}`)
		}, nil, 0, protocol.SourceFallback, "defer", 0, "prompt limit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &scripted{replies: tt.replies}
			p := &LLM{Client: client, MinRepairTime: 80 * time.Millisecond}
			ctx := context.Background()
			if tt.timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tt.timeout)
				defer cancel()
			}
			req := tt.req(t)
			start := time.Now()
			d := p.Decide(ctx, req)
			if tt.timeout > 0 && time.Since(start) > tt.timeout+200*time.Millisecond {
				t.Fatalf("overran the deadline: %v", time.Since(start))
			}
			if d.Source != tt.wantSource || d.Choice.Option != tt.wantOption {
				t.Fatalf("got %s/%s (%s), want %s/%s", d.Source, d.Choice.Option, d.Rationale, tt.wantSource, tt.wantOption)
			}
			if err := ValidateChoice(req.Options, d.Choice); err != nil {
				t.Fatalf("invalid choice escaped: %v", err)
			}
			if got := client.calls(); got != tt.wantCalls {
				t.Fatalf("calls %d, want %d", got, tt.wantCalls)
			}
			if len(d.Trace.Attempts) != tt.wantCalls {
				t.Fatalf("trace has %d attempts, want %d", len(d.Trace.Attempts), tt.wantCalls)
			}
			if tt.wantNote != "" && !strings.Contains(d.Rationale, tt.wantNote) {
				t.Fatalf("rationale %q, want containing %q", d.Rationale, tt.wantNote)
			}
			if tt.wantCalls == 2 {
				msgs := client.got[1].Messages
				if len(msgs) != 3 || msgs[1].Role != llm.RoleAssistant || !strings.Contains(msgs[2].Content, "invalid") {
					t.Fatalf("repair round malformed: %+v", msgs)
				}
			}
		})
	}
}

type staticRecall string

func (r staticRecall) Brief(context.Context, *protocol.DecideRequest, int) string { return string(r) }

func TestLLMPrompt(t *testing.T) {
	tests := []struct {
		name    string
		ctx     string
		recall  Recall
		want    []string
		notWant []string
	}{
		{"fixture", "", nil, []string{
			"Day 69.",
			"Shop: cash 2450, rent 600 due in 3 days, retail markup 15%",
			"Lower Levels 12 (Known)", "Black Market 20.25 (Known)",
			"Customer: Retired farmer, faction Lower Levels",
			"wants to BUY from you",
			"- budget left 900",
			"- interests: electronics, music",
			"Their items (you may buy): none",
			"- Cassette: price 340, est. value 300, you paid 180; claims: Worn (condition); STOLEN heat 22",
			"Market events: electronics +10%",
			"- sell_all: Sell 1 item for 340 => you receive 340; you paid 180\n",
			"- offer_discount_10: Offer 10% discount (always accepted; raises their budget 20%) => you would receive 306; you paid 180\n",
			"- add_markup_10: Add 10% markup (accept 82%, refuse 8%, walk out 10% (-3 rep)) => expected 333 (vs 340 plain) = 82% accepted (receive 374), 8% refused (receive 340), 10% walk out (no deal, -3 rep)\n",
			"- dismiss: Send the customer away\n",
			`- counter: Counter (params: "price" integer from 1 to 400)` + "\n",
			"- defer: let the human player decide",
		}, []string{`"gift"`, "- gift", "From memory", "Already used", "{"}},
		{"recall section", "", staticRecall("  Retired farmers paid 120% for cassettes.  "), []string{
			"\nFrom memory of past play:\nRetired farmers paid 120% for cassettes.\n\nOptions:",
		}, nil},
		{"used bargains", `{"client":{"intent":"buy"},"bargain":{"sell_side_used":true,"blackmail_used":true}}`, nil, []string{
			"Already used with this customer: sell-side bargain, blackmail",
		}, nil},
		{"tourist has no bar", `{"client":{"faction":"tourist","intent":"sell"}}`, nil, []string{"faction Tourist (no reputation bar)", "wants to SELL to you"}, nil},
		{"buying notes", `{"store":{"cash":500,"rent_due":200,"rent_days_left":1},"client":{"intent":"sell"},"counter":{"theirs":[{"name":"Lamp","price":250,"estimated_value":400}]}}`, nil,
			nil, []string{"=>"}},
		{"unrenderable context falls back to JSON", `{"client": "a string"}`, nil, []string{`State (JSON): {"client":"a string"}`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &scripted{replies: []reply{{text: `{"option":"dismiss","params":{}}`}}}
			req := fixtureRequest(t)
			req.Options = append(req.Options, barterOpts[1], barterOpts[3])
			if tt.ctx != "" {
				req.Context = json.RawMessage(tt.ctx)
			}
			(&LLM{Client: client, Recall: tt.recall}).Decide(context.Background(), req)
			if client.calls() != 1 {
				t.Fatal("no call")
			}
			got := client.got[0]
			prompt := got.Messages[0].Content
			for _, want := range tt.want {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt missing %q:\n%s", want, prompt)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(prompt, nw) {
					t.Errorf("prompt has %q:\n%s", nw, prompt)
				}
			}
			if got.System != barterSystem || len(got.Schema) == 0 {
				t.Errorf("system or schema missing")
			}
		})
	}
}

func TestClipRationale(t *testing.T) {
	long := strings.Repeat("word ", 80)
	tests := []struct{ in, want string }{
		{"  Sell now.  ", "Sell now."},
		{"One. Two! Three? Four.", "One. Two!"},
		{"Price 3.5 is fine. Budget ok. Extra.", "Price 3.5 is fine. Budget ok."},
		{"multi\n line\tspace", "multi line space"},
		{"Cash vs. rent matters. Sell. More.", "Cash vs. rent matters. Sell."},
		{long, strings.TrimRight(long[:maxRationale-len("…")], " ") + "…"},
		{strings.Repeat("é", 200), strings.Repeat("é", (maxRationale-len("…"))/2) + "…"},
	}
	for _, tt := range tests {
		if got := clipRationale(tt.in); got != tt.want {
			t.Errorf("clipRationale(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if got := clipRationale(tt.in); len(got) > maxRationale {
			t.Errorf("clipRationale(%q) is %d bytes", tt.in, len(got))
		}
	}
}

func TestParseOdds(t *testing.T) {
	tests := []struct {
		label  string
		want   bargainOdds
		wantOK bool
	}{
		{"Add 10% markup (accept 82%, else stay 90%, else walk out -3 rep)", bargainOdds{82, 10}, true},
		{"Add 10% markup (accept 82%, refuse 8%, walk out 10% (-3 rep))", bargainOdds{82, 10}, true},
		{"Add 10% markup (accept 82%, else refused and they stay)", bargainOdds{82, 0}, true},
		{"Demand 25% discount (accepts 52%, refuses 20%, walks out 28%)", bargainOdds{52, 28}, true},
		{"Add 50% markup (accept 0%, else stay 40%, else walk out -3 rep)", bargainOdds{0, 60}, true},
		{"Add 10% markup (accept 82%, else stay 0%)", bargainOdds{82, 18}, true},
		{"Offer 10% discount (always accepted; raises their budget 20%)", bargainOdds{}, false},
		{"Add 10% markup (accept 82%)", bargainOdds{}, false},
		{"Add 10% markup (accept 90%, walk out 20%)", bargainOdds{}, false},
	}
	for _, tt := range tests {
		got, ok := parseOdds(tt.label)
		if ok != tt.wantOK || got != tt.want {
			t.Errorf("parseOdds(%q) = %+v, %v; want %+v, %v", tt.label, got, ok, tt.want, tt.wantOK)
		}
	}
}

// leaves collects every key and every scalar value under v, as they would print.
func leaves(v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			out[k] = true
			leaves(e, out)
		}
	case []any:
		for _, e := range x {
			leaves(e, out)
		}
	case nil:
	default:
		b, _ := json.Marshal(x)
		out[strings.Trim(string(b), `"`)] = true
	}
}

const recallSentinel = "zz_recall_sentinel"

// echoRecall returns a sentinel, plus anything hidden it was handed so a leak shows up.
type echoRecall struct{ calls int }

func (r *echoRecall) Brief(_ context.Context, req *protocol.DecideRequest, _ int) string {
	r.calls++
	return recallSentinel + string(req.Hidden)
}

const planSentinel = "zz_plan_sentinel"

// recordingPlanner returns a fixed plan and records which sessions asked; it only ever
// sees a session id, so it has no way to carry hidden values.
type recordingPlanner struct {
	text     string
	day      int
	ok       bool
	sessions []string
}

func (p *recordingPlanner) CurrentPlan(session string) (string, int, bool) {
	p.sessions = append(p.sessions, session)
	return p.text, p.day, p.text != "" || p.ok
}

// TestHiddenNeverInPrompt enforces the information boundary in docs/PROTOCOL.md: nothing
// from "hidden" may reach the model, through the first prompt or the repair round.
func TestHiddenNeverInPrompt(t *testing.T) {
	for _, fx := range []string{"decide.request.json", "decide.stage.request.json"} {
		t.Run(fx, func(t *testing.T) {
			hiddenNeverInPrompt(t, func(t *testing.T) *protocol.DecideRequest { return fixtureNamed(t, fx) })
		})
	}
}

func hiddenNeverInPrompt(t *testing.T, fixtureRequest func(*testing.T) *protocol.DecideRequest) {
	fixtureHidden := string(fixtureRequest(t).Hidden)
	if fixtureHidden == "" {
		t.Fatal("fixture has no hidden block; this test needs one")
	}
	tests := []struct {
		name   string
		hidden string
	}{
		{"fixture", fixtureHidden},
		{"sentinels everywhere", `{"zz_secret_key":"zz_secret_value","nested":{"zz_deep_key":[{"zz_in_array":"zz_array_value"}]},"zz_number":987654321}`},
		{"shadows visible keys", `{"client":{"intent":"zz_true_intent","budget":123456789},"store":{"cash":555444333},"counter":{"theirs":[{"name":"zz_hidden_item","price":1}]}}`},
		{"empty", `{}`},
		{"null", `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Two garbage answers force the repair round, so every message is checked.
			// The recall sees the request the policy sees; it must never get hidden.
			// The option notes reach the player's overlay, so they get the same scrutiny.
			var notes []map[string]string
			run := func(hidden string) []llm.Request {
				client := &scripted{replies: []reply{{text: "nope"}, {text: "still nope"}}}
				req := fixtureRequest(t)
				req.Hidden = json.RawMessage(hidden)
				rec := &echoRecall{}
				pl := &recordingPlanner{text: planSentinel, day: req.GameDay}
				d := (&LLM{Client: client, Recall: rec, Plan: pl}).Decide(context.Background(), req)
				if rec.calls != 1 {
					t.Fatalf("recall called %d times", rec.calls)
				}
				if len(pl.sessions) != 1 || pl.sessions[0] != req.SessionID {
					t.Fatalf("planner asked for %v", pl.sessions)
				}
				notes = append(notes, d.Notes, Heuristic{}.Decide(context.Background(), req).Notes)
				return client.got
			}
			with, without := run(tt.hidden), run("")
			if len(notes[0]) == 0 {
				t.Fatal("no option notes; the leak check below would be vacuous")
			}
			for i := range 2 {
				a, _ := json.Marshal(notes[i])
				b, _ := json.Marshal(notes[i+2])
				if string(a) != string(b) {
					t.Fatalf("option notes differ when hidden is present:\nwith    %s\nwithout %s", a, b)
				}
			}
			if len(with) != 2 {
				t.Fatalf("expected a repair round, got %d calls", len(with))
			}
			if !strings.Contains(with[0].Messages[0].Content, recallSentinel) || !strings.Contains(with[1].Messages[0].Content, recallSentinel) {
				t.Fatalf("recall brief missing from the prompt")
			}
			if !strings.HasPrefix(with[0].Messages[0].Content, "TODAY'S PLAN") || !strings.Contains(with[1].Messages[0].Content, planSentinel) {
				t.Fatalf("plan missing from the prompt")
			}
			for i := range with {
				a, _ := json.Marshal(with[i])
				b, _ := json.Marshal(without[i])
				if string(a) != string(b) {
					t.Fatalf("call %d differs when hidden is present:\nwith    %s\nwithout %s", i, a, b)
				}
			}

			var h any
			json.Unmarshal([]byte(tt.hidden), &h)
			secret := map[string]bool{}
			leaves(h, secret)
			visible := map[string]bool{}
			var ctx any
			json.Unmarshal(fixtureRequest(t).Context, &ctx)
			leaves(ctx, visible)
			texts := make([]string, 0, len(with)+1)
			for _, req := range with {
				text := req.System + string(req.Schema)
				for _, m := range req.Messages {
					text += m.Content
				}
				texts = append(texts, text)
			}
			n, _ := json.Marshal(notes[:2])
			texts = append(texts, string(n))
			for _, text := range texts {
				// Short leaves ("3", "false", English words used as keys) occur in any
				// prompt; the byte-for-byte comparison above covers them.
				for s := range secret {
					if len(s) >= 6 && !visible[s] && strings.Contains(text, s) {
						t.Errorf("hidden %q leaked into the prompt or the notes", s)
					}
				}
			}
		})
	}
}

func TestChoiceSchema(t *testing.T) {
	compile := func(t *testing.T, raw json.RawMessage) *jsonschema.Schema {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("choice.json", doc); err != nil {
			t.Fatal(err)
		}
		s, err := c.Compile("choice.json")
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	tests := []struct {
		name   string
		bounds bool
		answer string
		valid  bool
	}{
		{"counter", false, `{"rationale":"r","option":"counter","params":{"price":140}}`, true},
		{"defer", false, `{"rationale":"r","option":"defer","params":{}}`, true},
		{"unsupported option excluded", false, `{"rationale":"r","option":"gift","params":{}}`, false},
		{"unknown option", false, `{"rationale":"r","option":"steal","params":{}}`, false},
		{"extra key", false, `{"rationale":"r","option":"accept","params":{},"mood":"x"}`, false},
		{"no bounds lets range through to validation", false, `{"rationale":"r","option":"counter","params":{"price":9999}}`, true},
		{"bounds enforced", true, `{"rationale":"r","option":"counter","params":{"price":9999}}`, false},
		{"string price", true, `{"rationale":"r","option":"counter","params":{"price":"140"}}`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := choiceSchema(barterOpts, tt.bounds)
			if err != nil {
				t.Fatal(err)
			}
			if !tt.bounds && bytes.Contains(raw, []byte("minimum")) {
				t.Fatalf("unbounded schema carries numeric constraints: %s", raw)
			}
			if !bytes.HasPrefix(raw, []byte(`{"type":"object","properties":{"rationale"`)) {
				t.Fatalf("key order lost: %s", raw)
			}
			inst, err := jsonschema.UnmarshalJSON(strings.NewReader(tt.answer))
			if err != nil {
				t.Fatal(err)
			}
			err = compile(t, raw).Validate(inst)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v, want %v (%v)\nschema %s", err == nil, tt.valid, err, raw)
			}
		})
	}
}

func TestOptionNotes(t *testing.T) {
	buying := `{"store":{"cash":500,"rent_due":200,"rent_days_left":1},"client":{"intent":"sell","wholesale":{"price":320}},"counter":{"theirs":[{"name":"Lamp","price":250,"estimated_value":400}]}}`
	tests := []struct {
		ctx, id, label, want string
	}{
		{buying, "buy_all", "", "costs 250 for an estimate of 400 (margin +150)"},
		{buying, "accept_wholesale", "", "costs 320 for an estimate of 400 (margin +80); more than the 300 you can spend"},
		{buying, "demand_discount_10", "accept 80%, else stay 88%", "expected margin 152 (vs 150 buying now) = 80% accepted (pay 225, margin +175), 8% refused (price stays 250, margin +150), 12% walk out (no deal, -3 rep)"},
		{buying, "demand_discount_25", "accept 98%, else stay 100%", "expected margin 211 (vs 150 buying now) = 98% accepted (pay 187, margin +213), 2% refused (price stays 250, margin +150), no walk-out risk"},
		{buying, "offer_markup_25", "", "costs 312 (margin +88); more than the 300 you can spend"},
		{buying, "demand_discount_10", "no odds here", ""},
		{buying, "sell_all", "", ""},
		{buying, "dismiss", "", ""},
		{`{"client":{"intent":"buy","budget":500},"counter":{"yours_staged":[{"price":700},{"price":100,"bought_at":50}]}}`, "sell_all", "", "you receive 500 (capped by budget)"},
		{`{"client":{"intent":"buy","budget":500},"counter":{"yours_staged":[{"price":700}]}}`, "offer_discount_25", "", "you would receive 525"},
		{`{"client":{"intent":"buy"},"counter":{"yours_staged":[{"price":700}]}}`, "sell_all", "", ""},
	}
	for _, tt := range tests {
		c, err := parseBarter(json.RawMessage(tt.ctx))
		if err != nil {
			t.Fatal(err)
		}
		if got := optionNotes(c)(tt.id, tt.label); got != tt.want {
			t.Errorf("%s %s: got %q, want %q", tt.ctx, tt.id, got, tt.want)
		}
	}
}

// fixtureNotes are the notes for decide.request.json. decide.response.json shows a subset
// of them, checked by TestFixtureNotesMatch.
func fixtureNotes(*testing.T) map[string]string {
	return map[string]string{
		"sell_all":          "you receive 340; you paid 180",
		"offer_discount_10": "you would receive 306; you paid 180",
		"add_markup_10":     "expected 333 (vs 340 plain)",
		"add_markup_25":     "expected 289 (vs 340 plain)",
	}
}

func TestFixtureNotesMatch(t *testing.T) {
	b, err := os.ReadFile("../../protocol/fixtures/decide.response.json")
	if err != nil {
		t.Fatal(err)
	}
	var r protocol.DecideResponse
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.OptionNotes) == 0 {
		t.Fatal("decide.response.json has no option_notes")
	}
	got := OptionNotes(fixtureRequest(t))
	for id, want := range r.OptionNotes {
		if got[id] != want {
			t.Errorf("%s: bridge notes %q, fixture %q", id, got[id], want)
		}
	}
}

func TestDisplayNotes(t *testing.T) {
	long := protocol.Option{ID: "buy_all", Label: "Buy"}
	tests := []struct {
		name string
		req  func(t *testing.T) *protocol.DecideRequest
		want map[string]string
	}{
		{"fixture", fixtureRequest, fixtureNotes(t)},
		{"unknown kind", func(t *testing.T) *protocol.DecideRequest {
			r := fixtureRequest(t)
			r.Kind = "loan"
			return r
		}, nil},
		{"context does not parse", func(t *testing.T) *protocol.DecideRequest { return withContext(t, `{"store":"closed"}`) }, nil},
		{"no context", func(t *testing.T) *protocol.DecideRequest { return withContext(t, ``) }, nil},
		{"unfillable option skipped", func(t *testing.T) *protocol.DecideRequest {
			r := fixtureRequest(t)
			r.Options = []protocol.Option{{ID: "sell_all", Label: "Sell", Params: map[string]protocol.Param{"x": {Type: "item_ref"}}}}
			return r
		}, nil},
		{"long note clipped", func(t *testing.T) *protocol.DecideRequest {
			r := withContext(t, `{"store":{"cash":-`+strings.Repeat("9", 18)+`},"client":{"intent":"sell"},"counter":{"theirs":[{"price":`+strings.Repeat("9", 18)+`,"estimated_value":-`+strings.Repeat("9", 18)+`}]}}`)
			r.Options = []protocol.Option{long}
			return r
		}, map[string]string{"buy_all": "costs 999999999999999999 for an estimate of -999999999999999999 (margin -1999999999999999998); more than the -9999999…"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OptionNotes(tt.req(t))
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(tt.want)
			if string(a) != string(b) {
				t.Fatalf("got  %s\nwant %s", a, b)
			}
			for id, n := range got {
				if len(n) > protocol.MaxOptionNote {
					t.Errorf("%s note is %d bytes", id, len(n))
				}
			}
		})
	}
}

func TestClipNote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"  short  ", "short"},
		{strings.Repeat("a", 120), strings.Repeat("a", 120)},
		{strings.Repeat("a", 121), strings.Repeat("a", 117) + "…"},
		{strings.Repeat("a", 116) + "éééé", strings.Repeat("a", 116) + "…"},
	}
	for _, tt := range tests {
		if got := ClipNote(tt.in); got != tt.want || len(got) > protocol.MaxOptionNote {
			t.Errorf("ClipNote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// Notes need no model, so a failed or deferred decision still carries them.
func TestDecisionsCarryNotes(t *testing.T) {
	want := fixtureNotes(t)
	llmWith := func(r reply) Policy { return &LLM{Client: &scripted{replies: []reply{r, r}}} }
	tests := []struct {
		name       string
		policy     Policy
		req        func(t *testing.T) *protocol.DecideRequest
		wantSource protocol.Source
		want       map[string]string
	}{
		{"heuristic", Heuristic{}, fixtureRequest, protocol.SourceHeuristic, want},
		{"heuristic defer", Heuristic{}, func(t *testing.T) *protocol.DecideRequest {
			r := fixtureRequest(t)
			r.Context = json.RawMessage(strings.Replace(string(r.Context), `"bought_at": 180`, `"bought_at": 900`, 1))
			return r
		}, protocol.SourceHeuristic, map[string]string{
			"sell_all": "you receive 340; you paid 900", "offer_discount_10": "you would receive 306; you paid 900",
			"add_markup_10": "expected 333 (vs 340 plain)", "add_markup_25": "expected 289 (vs 340 plain)",
		}},
		{"llm", llmWith(reply{text: `{"rationale":"r","option":"sell_all","params":{}}`}), fixtureRequest, protocol.SourceLLM, want},
		{"llm garbage", llmWith(reply{text: "no"}), fixtureRequest, protocol.SourceFallback, want},
		{"llm api error", llmWith(reply{err: errors.New("HTTP 529")}), fixtureRequest, protocol.SourceFallback, want},
		{"unknown kind", Heuristic{}, func(t *testing.T) *protocol.DecideRequest {
			r := fixtureRequest(t)
			r.Kind = "loan"
			return r
		}, protocol.SourceHeuristic, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.policy.Decide(context.Background(), tt.req(t))
			if d.Source != tt.wantSource {
				t.Fatalf("source %s (%s), want %s", d.Source, d.Rationale, tt.wantSource)
			}
			a, _ := json.Marshal(d.Notes)
			b, _ := json.Marshal(tt.want)
			if string(a) != string(b) {
				t.Fatalf("notes %s\nwant  %s", a, b)
			}
		})
	}
}

func TestPlanInPrompt(t *testing.T) {
	plan := "GOALS\n- Lower Levels 18 -> 20\nCASH\n- floor 900"
	tests := []struct {
		name    string
		planner Planner
		want    []string
		notWant []string
	}{
		{"no planner", nil, nil, []string{"PLAN"}},
		{"today's plan opens the prompt", &recordingPlanner{text: plan, day: 69},
			[]string{"TODAY'S PLAN (from this morning's strategist; serve it):\n" + plan + "\n\nDay 69.\n"}, []string{"LATEST PLAN"}},
		{"yesterday's plan while today's is pending", &recordingPlanner{text: plan, day: 68},
			[]string{"LATEST PLAN (written for day 68; today's is not ready, so adjust its dates):\n" + plan + "\n\nDay 69."}, []string{"TODAY'S PLAN"}},
		{"stale plan dropped", &recordingPlanner{text: plan, day: 66}, nil, []string{"PLAN", "Lower Levels 18 -> 20"}},
		{"plan from a later day dropped", &recordingPlanner{text: plan, day: 70}, nil, []string{"PLAN"}},
		{"empty plan dropped", &recordingPlanner{text: "  ", day: 69, ok: true}, nil, []string{"PLAN"}},
		{"oversized plan clipped", &recordingPlanner{text: strings.Repeat("é", maxPlanBytes), day: 69},
			[]string{"TODAY'S PLAN"}, []string{strings.Repeat("é", maxPlanBytes/2+1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &scripted{replies: []reply{{text: `{"option":"dismiss","params":{}}`}}}
			p := NewLLM(client)
			if tt.planner != nil {
				p = NewLLM(client, WithPlan(tt.planner))
			}
			p.Decide(context.Background(), fixtureRequest(t))
			prompt := client.got[0].Messages[0].Content
			if !utf8.ValidString(prompt) {
				t.Fatal("prompt is not valid UTF-8")
			}
			for _, w := range tt.want {
				if !strings.Contains(prompt, w) {
					t.Errorf("prompt missing %q:\n%s", w, prompt)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(prompt, nw) {
					t.Errorf("prompt has %q:\n%s", nw, prompt)
				}
			}
			if client.got[0].System != barterSystem {
				t.Error("the plan must not change the system prompt (prefix cache)")
			}
		})
	}
}

func TestBarterSystemServesThePlan(t *testing.T) {
	for _, w := range []string{"serve it", "KEEP list", "say so in the rationale", "information, not the goal", "Coercion only when clearly justified"} {
		if !strings.Contains(barterSystem, w) {
			t.Errorf("barter system prompt lacks %q", w)
		}
	}
}
