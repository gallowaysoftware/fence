package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

func fixtureEvent(t *testing.T, name string) protocol.Event {
	t.Helper()
	b, err := os.ReadFile("../../protocol/fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var e protocol.Event
	if err := json.Unmarshal(b, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func query(t *testing.T, s *Store, q string, args ...any) string {
	t.Helper()
	rows, err := s.db.Query(q, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		var parts []string
		for _, v := range vals {
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			if v == nil {
				v = "NULL"
			}
			parts = append(parts, fmt.Sprint(v))
		}
		out = append(out, strings.Join(parts, "|"))
	}
	return strings.Join(out, "\n")
}

func TestRecordingTables(t *testing.T) {
	s := openStore(t, "", Options{})
	snap := fixtureEvent(t, "event.day_snapshot.json")
	cust := fixtureEvent(t, "event.customer_observed.json")
	act := fixtureEvent(t, "event.human_action.json")
	req := fixtureRequest(t)
	sid := req.SessionID

	s.Hello(sid, protocol.HelloRequest{Protocol: 1})
	s.Event(sid, snap)
	s.Event(sid, snap) // a resent event (mod buffer flush) is kept once
	s.Event(sid, cust)
	s.Decision(sid, req, resp(req.DecisionID, "sell_all"))
	s.Outcome(sid, outcome(sid, req.DecisionID, req.GameDay, `{"kind":"trade","side":"sold","items":[2231],"cash_delta":340,"rep_delta":{}}`))
	s.Event(sid, act)

	actionAt := func(seq int64, data string) protocol.Event {
		return protocol.Event{SessionID: sid, Seq: seq, Kind: KindHumanAction, GameDay: 69, Data: json.RawMessage(data)}
	}
	// A decision without a signature gets its customer from the action that names it.
	unsigned := fixtureRequest(t)
	unsigned.DecisionID, unsigned.StateSignature = "d-0002", ""
	s.Decision(sid, unsigned, resp("d-0002", "add_markup_10"))
	s.Outcome(sid, outcome(sid, "d-0002", 69, `{"kind":"left","cause":"dismissed"}`))
	for i, data := range []string{
		`{"client_seq":42,"action":"dismiss","decision_id":"d-0002","advised":"add_markup_10"}`, // a choice against advice
		`{"client_seq":42,"action":"add_markup_10","decision_id":"d-0002","advised":"add_markup_10"}`,
		`{"client_seq":42,"action":"showcase","item_uid":7}`,                              // no advice
		`{"client_seq":42,"action":"offer_discount_25","advised":"defer"}`,                // advice was "you decide"
		`{"client_seq":43,"action":"buy_all","decision_id":"d-gone","advised":"dismiss"}`, // unknown decision
	} {
		s.Event(sid, actionAt(int64(10+i), data))
	}
	s.flush()
	if n := s.Dropped(); n != 0 {
		t.Fatalf("dropped %d", n)
	}

	tests := []struct {
		name, q, want string
	}{
		{"snapshot columns",
			`SELECT run_id, session_id, seq, game_day, phase, cash, rent_due, rent_days_left FROM day_snapshots`,
			"1|s-0001|3|69|morning|2450|600|1"},
		{"snapshot reputation rows",
			`SELECT faction, value, status, next_perk, next_perk_threshold FROM day_snapshot_rep ORDER BY faction`,
			"black_market|20.25|Established|BMRELIABLE|40\nlower_levels|12|Known|LLREFERAL|20"},
		{"snapshot keeps the visible JSON",
			`SELECT json_extract(data, '$.inventory[0].where') FROM day_snapshots`, "showcase"},
		{"customer",
			`SELECT seq, game_day, client_seq, covered, reason, client_name, client_faction, client_intent, json_extract(context, '$.client.name') FROM customers`,
			"4|69|41|0|story client|Dr. Jackson|upper_levels|sell|Dr. Jackson"},
		{"decision linked by signature",
			`SELECT decision_id, client_seq FROM decisions ORDER BY id`, "d-0001|41\nd-0002|42"},
		{"outcomes linked through their decision",
			`SELECT decision_id, client_seq FROM outcomes ORDER BY id`, "d-0001|41\nd-0002|42"},
		{"human actions",
			`SELECT seq, client_seq, action, item_uid, decision_id, advised, matched FROM human_actions ORDER BY seq`,
			"5|41|stage|2231|d-0001|sell_all|NULL\n" +
				"10|42|dismiss|NULL|d-0002|add_markup_10|0\n" +
				"11|42|add_markup_10|NULL|d-0002|add_markup_10|1\n" +
				"12|42|showcase|7|||NULL\n" +
				"13|42|offer_discount_25|NULL||defer|NULL\n" +
				"14|43|buy_all|NULL|d-gone|dismiss|0"},
		{"customer joins its decisions, actions and outcomes",
			`SELECT c.client_seq,
				(SELECT count(*) FROM decisions d WHERE d.session_id = c.session_id AND d.game_day = c.game_day AND d.client_seq = c.client_seq),
				(SELECT count(*) FROM human_actions h WHERE h.session_id = c.session_id AND h.game_day = c.game_day AND h.client_seq = c.client_seq),
				(SELECT count(*) FROM outcomes o WHERE o.session_id = c.session_id AND o.game_day = c.game_day AND o.client_seq = c.client_seq)
			FROM customers c`, "41|1|1|1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := query(t, s, tt.q); got != tt.want {
				t.Fatalf("got\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestClientSeqFromSignature(t *testing.T) {
	tests := []struct {
		sig  string
		want int64
		ok   bool
	}{
		{"c41:i2231:b0000", 41, true},
		{"c7", 7, true},
		{"", 0, false},
		{"x41:i1", 0, false},
		{"c:i1", 0, false},
		{"c-1:i1", 0, false},
	}
	for _, tt := range tests {
		got, ok := ClientSeqFromSignature(tt.sig)
		if got != tt.want || ok != tt.ok {
			t.Errorf("ClientSeqFromSignature(%q) = %d, %v; want %d, %v", tt.sig, got, ok, tt.want, tt.ok)
		}
	}
}

// A database written by the first schema keeps its rows and gains the new tables.
func TestMigrateFromV1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	saved := migrations
	migrations = migrations[:1]
	err = migrate(ctx, db)
	migrations = saved
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO decisions (session_id, decision_id, run_id, game_day, kind, context, options, choice, source, created_at)
		VALUES ('s', 'd', 1, 3, 'barter', '{}', '[]', 'dismiss', 'llm', '')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s := openStore(t, path, Options{})
	if got := query(t, s, `SELECT decision_id, client_seq FROM decisions`); got != "d|NULL" {
		t.Fatalf("decisions after migration: %s", got)
	}
	for _, table := range []string{"day_snapshots", "day_snapshot_rep", "customers", "human_actions"} {
		if got := query(t, s, `SELECT count(*) FROM `+table); got != "0" {
			t.Fatalf("%s: %s", table, got)
		}
	}
}

// Hidden values sent with recording events reach neither the database nor anything built
// from it: the brief, the summary and the reflection prompt.
func TestHiddenEventsNeverInMemory(t *testing.T) {
	const sentinel = "SENTINEL_EVENT_HIDDEN_51c9"
	var prompts []string
	var mu sync.Mutex
	fake := fakeLLM(func(req llm.Request) (llm.Response, error) {
		mu.Lock()
		prompts = append(prompts, req.System+req.Messages[0].Content)
		mu.Unlock()
		return llm.Response{Text: `{"lessons":"- hold stock for rent"}`, Model: "fake"}, nil
	})
	path := filepath.Join(t.TempDir(), "memory.db")
	s, err := Open(path, Options{LLM: fake})
	if err != nil {
		t.Fatal(err)
	}
	hidden := json.RawMessage(`{"days_to_inspection":313131,"client":{"identifier":"` + sentinel + `_id"},"items":[{"uid":2231,"true_value":876543,"real_features":["` + sentinel + `_f"]}]}`)
	req := fixtureRequest(t)
	req.Hidden = nil
	sid := req.SessionID
	s.Hello(sid, protocol.HelloRequest{Protocol: 1})
	for _, name := range []string{"event.day_snapshot.json", "event.customer_observed.json"} {
		e := fixtureEvent(t, name)
		e.Hidden = hidden
		s.Event(sid, e)
	}
	s.Decision(sid, req, resp(req.DecisionID, "sell_all"))
	act := fixtureEvent(t, "event.human_action.json")
	act.Hidden = hidden
	s.Event(sid, act)
	s.Outcome(sid, outcome(sid, req.DecisionID, req.GameDay, `{"kind":"trade","side":"sold","items":[2231],"cash_delta":340,"rep_delta":{}}`))
	s.Event(sid, protocol.Event{SessionID: sid, Seq: 9, Kind: "going_sleep", GameDay: req.GameDay, Data: json.RawMessage(`{}`), Hidden: hidden})
	waitLessons(t, s, 1, 1)

	brief := s.Brief(context.Background(), fixtureRequest(t), 1<<20)
	sum, err := s.Summary(context.Background(), sid)
	if err != nil {
		t.Fatal(err)
	}
	summary, _ := json.Marshal(sum)
	if !strings.Contains(brief, "hold stock for rent") || sum.Today.Decisions != 1 {
		t.Fatalf("memory lacks the recorded day: brief %q summary %s", brief, summary)
	}
	if got := query(t, s, `SELECT count(*) FROM day_snapshots`) + query(t, s, `SELECT count(*) FROM customers`) + query(t, s, `SELECT count(*) FROM human_actions`); got != "111" {
		t.Fatalf("recording tables not filled: %s", got)
	}
	mu.Lock()
	reflected := strings.Join(prompts, "\n")
	mu.Unlock()
	s.Close()

	var files []byte
	for _, suffix := range []string{"", "-wal", "-shm"} {
		b, _ := os.ReadFile(path + suffix)
		files = append(files, b...)
	}
	for _, needle := range []string{sentinel, "313131", "876543", "real_features", "true_value", "days_to_inspection", "dr_jackson"} {
		for where, text := range map[string]string{"brief": brief, "summary": string(summary), "reflection prompt": reflected, "database": string(files)} {
			if strings.Contains(text, needle) {
				t.Errorf("hidden value %q reached the %s", needle, where)
			}
		}
	}
}

func TestReadRunIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	if x, err := ReadRunIndex(context.Background(), path); err != nil || len(x.Sessions) != 0 {
		t.Fatalf("missing db: %+v %v", x, err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("reading the index created the database")
	}
	s := openStore(t, path, Options{})
	playDay(s, "s1", 5)
	playDay(s, "s1", 30) // a day jump opens run 2 for the same session
	s.Hello("s2", protocol.HelloRequest{Protocol: 1})
	s.Close()

	x, err := ReadRunIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		session string
		day     int
		want    int64
		ok      bool
	}{
		{"s1", 5, 1, true},
		{"s1", 30, 2, true},
		{"s1", 31, 2, true}, // an unseen day falls back to the session's run
		{"s2", 5, 0, false}, // hello only: never bound
		{"s9", 5, 0, false},
	}
	for _, tt := range tests {
		if got, ok := x.Run(tt.session, tt.day); got != tt.want || ok != tt.ok {
			t.Errorf("Run(%s, %d) = %d, %v; want %d, %v", tt.session, tt.day, got, ok, tt.want, tt.ok)
		}
	}
}

func TestAdviceMatched(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name, action, advised string
		options               []string
		want                  *bool
	}{
		{"followed", "sell_all", "sell_all", nil, &yes},
		{"other offered option", "dismiss", "sell_all", []string{"sell_all", "dismiss"}, &no},
		{"not a choice of that decision", "stage", "sell_all", []string{"sell_all", "dismiss"}, nil},
		{"unknown decision, barter id", "add_markup_25", "sell_all", nil, &no},
		{"unknown decision, staging move", "unstage", "sell_all", nil, nil},
		{"no advice", "sell_all", "", nil, nil},
		{"advice was defer", "sell_all", "defer", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AdviceMatched(tt.action, tt.advised, tt.options)
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
