package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

func openStore(t *testing.T, path string, opt Options) *Store {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "memory.db")
	}
	s, err := Open(path, opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func fixtureRequest(t *testing.T) *protocol.DecideRequest {
	t.Helper()
	b, err := os.ReadFile("../../protocol/fixtures/decide.request.json")
	if err != nil {
		t.Fatal(err)
	}
	var r protocol.DecideRequest
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	return &r
}

// barter builds a barter request. ours/theirs are item JSON objects.
func barter(session, id string, day int, client string, faction string, rep float64, ours, theirs []string, options ...string) *protocol.DecideRequest {
	ctx := fmt.Sprintf(`{"store":{"cash":1000,"day":%d},"reputation":{%q:{"value":%g,"status":"Known"}},
		"client":{"name":%q,"faction":%q,"intent":"buy","budget":900},
		"counter":{"theirs":[%s],"yours_staged":[%s]},"bargain":{},"market_events":[]}`,
		day, faction, rep, client, faction, strings.Join(theirs, ","), strings.Join(ours, ","))
	var opts []protocol.Option
	for _, o := range options {
		opts = append(opts, protocol.Option{ID: o, Label: o})
	}
	return &protocol.DecideRequest{SessionID: session, DecisionID: id, Kind: "barter", GameDay: day, DeadlineMS: 1000,
		Context: json.RawMessage(ctx), Options: opts}
}

func itemJSON(uid int, id, name string, price, est int, cond string) string {
	return fmt.Sprintf(`{"uid":%d,"id":%q,"name":%q,"price":%d,"estimated_value":%d,"features":[{"text":%q,"group":"condition"}],"stolen_heat":null,"contraband_level":0,"serious_contraband":false}`,
		uid, id, name, price, est, cond)
}

func outcome(session, id string, day int, result string) protocol.Outcome {
	return protocol.Outcome{SessionID: session, DecisionID: id, GameDay: day, Result: json.RawMessage(result)}
}

func ev(session, kind string, day int, data string) protocol.Event {
	return protocol.Event{SessionID: session, Kind: kind, GameDay: day, Data: json.RawMessage(data)}
}

func resp(id, option string) protocol.DecideResponse {
	return protocol.DecideResponse{DecisionID: id, Choice: protocol.Choice{Option: option}, Source: protocol.SourceLLM, Rationale: "test"}
}

func TestMigrationsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	for i := 0; i < 3; i++ {
		s, err := Open(path, Options{})
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		if err := migrate(context.Background(), s.db); err != nil {
			t.Fatalf("re-migrate: %v", err)
		}
		var v int
		s.db.QueryRow(`PRAGMA user_version`).Scan(&v)
		if v != SchemaVersion {
			t.Fatalf("user_version %d, want %d", v, SchemaVersion)
		}
		var mode string
		s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
		if mode != "wal" {
			t.Fatalf("journal_mode %s", mode)
		}
		s.Close()
	}
	s, _ := Open(path, Options{})
	s.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, SchemaVersion+1))
	s.Close()
	if _, err := Open(path, Options{}); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("opening a newer schema: %v", err)
	}
}

func TestRunHeuristic(t *testing.T) {
	type step struct {
		session string
		day     int
		newGame bool
		wantRun int64
		reason  string
	}
	tests := []struct {
		name   string
		latest *run
		steps  []step
	}{
		{"first run, days advance", nil, []step{
			{"s1", 1, false, 1, reasonFirst}, {"s1", 2, false, 1, ""}, {"s1", 3, false, 1, ""},
		}},
		{"new game always opens a run", nil, []step{
			{"s1", 5, false, 1, reasonFirst}, {"s1", 0, true, 2, reasonNewGame}, {"s1", 1, false, 2, ""},
		}},
		{"relaunch continues within the window", &run{ID: 7, LastDay: 10}, []step{
			{"s2", 10, false, 7, ""}, {"s3", 9, false, 7, ""}, {"s4", 13, false, 7, ""},
		}},
		{"relaunch far back is a new run", &run{ID: 7, LastDay: 10}, []step{
			{"s2", 3, false, 8, reasonDayJump},
		}},
		{"relaunch far ahead is a new run", &run{ID: 7, LastDay: 10}, []step{
			{"s2", 20, false, 8, reasonDayJump},
		}},
		{"jump inside a session", nil, []step{
			{"s1", 30, false, 1, reasonFirst}, {"s1", 2, false, 2, reasonDayJump},
		}},
		{"unknown day attaches without binding", &run{ID: 3, LastDay: 40}, []step{
			{"s1", 0, false, 3, ""}, {"s1", 1, false, 4, reasonDayJump},
		}},
		{"new game on a fresh session, then relaunch", &run{ID: 1, LastDay: 50}, []step{
			{"s1", 0, true, 2, reasonNewGame}, {"s1", 1, false, 2, ""}, {"s2", 2, false, 2, ""},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := newTracker(tt.latest, true)
			for i, s := range tt.steps {
				peekID, _, peekOK := tr.peek(s.session, s.day)
				r := tr.resolve(s.session, s.day, s.newGame, "")
				if r.ID != s.wantRun {
					t.Fatalf("step %d: run %d, want %d", i, r.ID, s.wantRun)
				}
				if s.reason != "" && r.Reason != s.reason {
					t.Fatalf("step %d: reason %s, want %s", i, r.Reason, s.reason)
				}
				if !s.newGame && peekOK != (peekID == r.ID) {
					t.Fatalf("step %d: peek (%d,%v) disagrees with resolve %d", i, peekID, peekOK, r.ID)
				}
			}
		})
	}
}

// playDay feeds one barter day through the recorder API: a markup accepted then a sale,
// a refused markup, and a walk-out.
func playDay(s *Store, session string, day int) {
	s.Hello(session, protocol.HelloRequest{Protocol: 1, ModVersion: "t"})
	s.Event(session, ev(session, "shutter_opened", day, `{}`))
	cassette := itemJSON(1, "cassette2", "Cassette", 340, 300, "Worn")
	d1 := fmt.Sprintf("d%d-1", day)
	s.Decision(session, barter(session, d1, day, "Retired farmer", "lower_levels", 12, []string{cassette}, nil, "sell_all", "add_markup_10", "dismiss"), resp(d1, "add_markup_10"))
	s.Outcome(session, outcome(session, d1, day, `{"kind":"bargain","option":"add_markup_10","accepted":true,"walked_out":false,"rep_delta":{}}`))
	d2 := fmt.Sprintf("d%d-2", day)
	s.Decision(session, barter(session, d2, day, "Retired farmer", "lower_levels", 12, []string{itemJSON(1, "cassette2", "Cassette", 374, 300, "Worn")}, nil, "sell_all", "dismiss"), resp(d2, "sell_all"))
	s.Outcome(session, outcome(session, d2, day, `{"kind":"trade","side":"sold","items":[1],"cash_delta":374,"rep_delta":{"lower_levels":0.5}}`))
	d3 := fmt.Sprintf("d%d-3", day)
	radio := itemJSON(2, "radio1", "Radio", 200, 400, "Scratched")
	s.Decision(session, barter(session, d3, day, "Dock worker", "lower_levels", 12, nil, []string{radio}, "buy_all", "demand_discount_25", "dismiss"), resp(d3, "demand_discount_25"))
	s.Outcome(session, outcome(session, d3, day, `{"kind":"bargain","option":"demand_discount_25","accepted":false,"walked_out":true,"rep_delta":{"lower_levels":-3}}`))
	s.Outcome(session, outcome(session, d3, day, `{"kind":"left","cause":"walked_out"}`))
}

func TestLedgerAndBrief(t *testing.T) {
	s := openStore(t, "", Options{})
	playDay(s, "s1", 5)
	s.flush()
	if n := s.Dropped(); n != 0 {
		t.Fatalf("dropped %d", n)
	}

	req := barter("s1", "q", 5, "Retired farmer", "lower_levels", 12,
		[]string{itemJSON(9, "cassette2", "Cassette", 350, 310, "Worn")}, []string{itemJSON(10, "radio1", "Radio", 210, 400, "Mint")},
		"sell_all", "add_markup_10", "demand_discount_25", "dismiss")
	got := s.Brief(context.Background(), req, 4000)
	for _, want := range []string{
		"Cassette (cassette2): we sold 1x for 374, 125% of est; last 374 (est 300) on day 5, lower_levels client",
		"add_markup_10: 1 tries, 1 accepted, 0 refused and stayed, 0 walked out (all rep bands)",
		"demand_discount_25: 1 tries, 0 accepted, 0 refused and stayed, 1 walked out (all rep bands)",
		`Earlier this run with "Retired farmer" (lower_levels):`,
		"day 5: we sold Cassette for 374",
		"day 5: add_markup_10: accepted",
		"Today so far (day 5):\n- 3 decisions, 1 sales, 0 purchases, cash +374 from trades, 1 walk-outs; rep lower_levels -2.50",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("brief missing %q\n--- brief ---\n%s", want, got)
		}
	}
	if strings.Contains(got, "Dock worker") {
		t.Errorf("brief leaks another client's history:\n%s", got)
	}
	if again := s.Brief(context.Background(), req, 4000); again != got {
		t.Errorf("brief is not deterministic")
	}

	// The ledger is global: a new run still sees prices and odds but not the old run's
	// client history or totals.
	s.Event("s2", ev("s2", "new_game_started", 0, `{"scenario_id":"DRUG_DEALER"}`))
	s.flush()
	req.SessionID, req.GameDay = "s2", 1
	got = s.Brief(context.Background(), req, 4000)
	if !strings.Contains(got, "Cassette (cassette2): we sold 1x") || !strings.Contains(got, "add_markup_10: 1 tries") {
		t.Errorf("new run lost the global ledger:\n%s", got)
	}
	if strings.Contains(got, "Earlier this run") || strings.Contains(got, "Today so far") {
		t.Errorf("new run inherited per-run facts:\n%s", got)
	}
}

func TestBriefEmptyAndCapped(t *testing.T) {
	s := openStore(t, "", Options{})
	req := barter("s1", "q", 5, "X", "lower_levels", 0, []string{itemJSON(1, "a", "A", 1, 1, "")}, nil, "sell_all")
	if got := s.Brief(context.Background(), req, 4000); got != "" {
		t.Fatalf("empty db brief %q", got)
	}
	for day := 1; day <= 3; day++ {
		playDay(s, "s1", day)
	}
	s.db.Exec(`INSERT INTO lessons VALUES (1, 1, 3, ?, 'test', '')`, strings.Repeat("- ünïcode rule that goes on\n", 70))
	s.flush()
	req = barter("s1", "q", 3, "Retired farmer", "lower_levels", 12,
		[]string{itemJSON(9, "cassette2", "Cassette", 350, 310, "Worn")}, []string{itemJSON(10, "radio1", "Radio", 210, 400, "Mint")},
		"sell_all", "add_markup_10", "demand_discount_25")
	full := s.Brief(context.Background(), req, 1<<20)
	for _, max := range []int{0, 1, 10, 80, 200, 500, 1000, 3000, len(full)} {
		got := s.Brief(context.Background(), req, max)
		if len(got) > max || !utf8.ValidString(got) {
			t.Fatalf("max %d: got %d bytes, valid utf8 %v", max, len(got), utf8.ValidString(got))
		}
		if max >= 500 && (!strings.Contains(got, "Lessons from earlier days:") || !strings.Contains(got, "Cassette")) {
			t.Fatalf("max %d: both facts and lessons should fit:\n%s", max, got)
		}
	}
}

func TestHiddenNeverInMemory(t *testing.T) {
	const sentinel = "SENTINEL_HIDDEN_7f3a"
	var prompts []string
	var mu sync.Mutex
	fake := fakeLLM(func(req llm.Request) (llm.Response, error) {
		mu.Lock()
		prompts = append(prompts, req.System+req.Messages[0].Content)
		mu.Unlock()
		return llm.Response{Text: `{"lessons":"- keep markups small"}`, Model: "fake"}, nil
	})
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := Open(path, Options{LLM: fake})
	if err != nil {
		t.Fatal(err)
	}
	req := fixtureRequest(t)
	req.Hidden = json.RawMessage(`{"client":{"identifier":"` + sentinel + `_id","is_aug":true,"stealth_wanted":true},
		"items":[{"uid":2231,"true_value":987654321,"real_features":["` + sentinel + `_feature"]}],"days_to_inspection":424242}`)
	s.Hello(req.SessionID, protocol.HelloRequest{Protocol: 1})
	s.Decision(req.SessionID, req, resp(req.DecisionID, "sell_all"))
	s.Outcome(req.SessionID, outcome(req.SessionID, req.DecisionID, req.GameDay, `{"kind":"trade","side":"sold","items":[2231],"cash_delta":340,"rep_delta":{}}`))
	s.Event(req.SessionID, ev(req.SessionID, "going_sleep", req.GameDay, `{}`))
	waitLessons(t, s, 1, 1)

	next := fixtureRequest(t)
	next.DecisionID = "d-0002"
	brief := s.Brief(context.Background(), next, 1<<20)
	if !strings.Contains(brief, "Cassette") || !strings.Contains(brief, "keep markups small") {
		t.Fatalf("brief lacks the recorded facts:\n%s", brief)
	}
	mu.Lock()
	reflected := strings.Join(prompts, "\n")
	mu.Unlock()
	if !strings.Contains(reflected, "Cassette") {
		t.Fatalf("reflection prompt lacks the day:\n%s", reflected)
	}
	s.Close()

	var files []byte
	for _, suffix := range []string{"", "-wal", "-shm"} {
		b, _ := os.ReadFile(path + suffix)
		files = append(files, b...)
	}
	for _, needle := range []string{sentinel, "987654321", "424242", "real_features", "true_value", "stealth_wanted", "retired_farmer"} {
		for where, text := range map[string]string{"brief": brief, "reflection prompt": reflected, "database": string(files)} {
			if strings.Contains(text, needle) {
				t.Errorf("hidden value %q reached the %s", needle, where)
			}
		}
	}
}

type fakeLLM func(llm.Request) (llm.Response, error)

func (f fakeLLM) Complete(_ context.Context, req llm.Request) (llm.Response, error) { return f(req) }

func waitLessons(t *testing.T, s *Store, run int64, version int) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var text string
		if s.db.QueryRow(`SELECT text FROM lessons WHERE run_id = ? AND version = ?`, run, version).Scan(&text) == nil {
			return text
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("lessons v%d for run %d never written", version, run)
	return ""
}

func TestReflection(t *testing.T) {
	var calls int
	var mu sync.Mutex
	fake := fakeLLM(func(req llm.Request) (llm.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if !strings.Contains(req.Messages[0].Content, "Day ") || len(req.Schema) == 0 {
			return llm.Response{}, errors.New("unexpected request")
		}
		switch calls {
		case 1:
			return llm.Response{Text: "<think>hm</think>```json\n{\"lessons\": \"- rule one\"}\n```", Model: "fake"}, nil
		case 2:
			return llm.Response{}, errors.New("server down")
		default:
			return llm.Response{Text: fmt.Sprintf(`{"lessons": %q}`, strings.Repeat("- a long rule\n", 400)), Model: "fake"}, nil
		}
	})
	s := openStore(t, "", Options{LLM: fake})
	playDay(s, "s1", 1)
	s.Event("s1", ev("s1", "going_sleep", 1, `{}`))
	if got := waitLessons(t, s, 1, 1); got != "- rule one" {
		t.Fatalf("v1 %q", got)
	}

	playDay(s, "s1", 2)
	s.Event("s1", ev("s1", "going_sleep", 2, `{}`)) // LLM fails: lessons unchanged
	playDay(s, "s1", 3)
	s.Event("s1", ev("s1", "going_sleep", 3, `{}`))
	v2 := waitLessons(t, s, 1, 2)
	if len(v2) > MaxLessonsBytes || !strings.HasSuffix(v2, "rule") {
		t.Fatalf("v2 is %d bytes, ends %q", len(v2), v2[max(0, len(v2)-20):])
	}
	if v1 := waitLessons(t, s, 1, 1); v1 != "- rule one" {
		t.Fatalf("previous version not kept: %q", v1)
	}

	// A day without decisions costs no call.
	mu.Lock()
	before := calls
	mu.Unlock()
	s.Event("s1", ev("s1", "going_sleep", 9, `{}`))
	s.flush()
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if calls != before {
		t.Fatalf("reflected over an empty day")
	}
}

func TestReflectionOffWithoutLLM(t *testing.T) {
	s := openStore(t, "", Options{})
	playDay(s, "s1", 1)
	s.Event("s1", ev("s1", "going_sleep", 1, `{}`))
	s.flush()
	var n int
	s.db.QueryRow(`SELECT count(*) FROM lessons`).Scan(&n)
	if n != 0 {
		t.Fatalf("%d lessons without an LLM", n)
	}
}

func TestLessonsCarryOver(t *testing.T) {
	for _, carry := range []bool{true, false} {
		t.Run(fmt.Sprint("carry=", carry), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "memory.db")
			s := openStore(t, path, Options{CarryLessons: carry})
			playDay(s, "s1", 8)
			s.flush()
			s.db.Exec(`INSERT INTO lessons VALUES (1, 1, 8, '- old run rule', 'test', '')`)
			s.Close()

			// A restart keeps the run; a new game starts another.
			s = openStore(t, path, Options{CarryLessons: carry})
			req := barter("s2", "q", 9, "Retired farmer", "lower_levels", 12, nil, nil, "dismiss")
			if got := s.Brief(context.Background(), req, 4000); !strings.Contains(got, "old run rule") || !strings.Contains(got, "Earlier this run") {
				t.Fatalf("restart lost the run:\n%s", got)
			}
			s.Event("s3", ev("s3", "new_game_started", 0, `{}`))
			req.SessionID, req.GameDay = "s3", 1
			// Before the writer catches up, Brief predicts the carry too.
			got := s.Brief(context.Background(), req, 4000)
			s.flush()
			after := s.Brief(context.Background(), req, 4000)
			for _, g := range []string{got, after} {
				if strings.Contains(g, "old run rule") != carry {
					t.Fatalf("carry=%v but brief:\n%s", carry, g)
				}
			}
			var reason string
			s.db.QueryRow(`SELECT reason FROM runs WHERE id = 2`).Scan(&reason)
			if reason != reasonNewGame {
				t.Fatalf("run 2 reason %q", reason)
			}
		})
	}
}

func TestRecordingNeverBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	s := openStore(t, path, Options{Queue: 4})
	// Another connection holds the write lock, so the writer stalls on busy_timeout.
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(context.Background(), `BEGIN IMMEDIATE`); err != nil {
		t.Fatal(err)
	}
	req := fixtureRequest(t)
	start := time.Now()
	for i := 0; i < 2000; i++ {
		s.Decision(req.SessionID, req, resp(req.DecisionID, "dismiss"))
		s.Event(req.SessionID, ev(req.SessionID, "x", 69, `{}`))
		s.Outcome(req.SessionID, outcome(req.SessionID, req.DecisionID, 69, `{"kind":"left","cause":"dismissed"}`))
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("6000 records took %v with a stalled writer", d)
	}
	if s.Dropped() == 0 {
		t.Fatal("a full queue should drop")
	}
	conn.ExecContext(context.Background(), `ROLLBACK`)
	conn.Close()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s.Decision(req.SessionID, req, resp(req.DecisionID, "dismiss")) // after Close: dropped, no panic
}
