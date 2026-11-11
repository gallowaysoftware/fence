package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/fakemod"
	"github.com/gallowaysoftware/fence/internal/llm"
	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
	"github.com/gallowaysoftware/fence/internal/server"
	"github.com/gallowaysoftware/fence/internal/show"
)

// strategist is the bridge's planner: a prose plan for /v1/summary.
type strategist struct{}

func (strategist) CurrentPlan(string) (string, int, bool) {
	return "GOALS\n- Take the board's next promotion.\nCASH\n- Keep 1,700 for the mortgage payment.\nRISK\n- Hot stock stays off the window.", 70, true
}

// The live stream end to end without the game: fakemod plays the mod against a real bridge with a
// strategist; fenceshow's live hook attaches mid-session, as it does on stream night; the autopilot then
// takes a promotion, signs and pays the mortgage, hands over to the human and takes the shop back;
// the hybrid planner posts its slate to the panel; the narrator speaks the beats in a mood that
// follows the money. Everything a viewer would get is read back from the panel's own endpoints.
func TestLiveShowEndToEnd(t *testing.T) {
	dir := t.TempDir()
	l, err := decisionlog.New(dir, decisionlog.DefaultQueue, nil)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bridgeSrv := &http.Server{Handler: server.New(server.Config{Mode: protocol.ModeObserve, Policy: policy.Heuristic{}, Log: l, Model: "qwen27", Planner: strategist{}}).Handler()}
	go bridgeSrv.Serve(ln)
	t.Cleanup(func() { bridgeSrv.Shutdown(context.Background()); l.Close() })
	bridge := "http://" + ln.Addr().String()

	res, err := fakemod.Run(context.Background(), fakemod.Config{BaseURL: bridge, FixturesDir: fixtures})
	if err != nil {
		t.Fatal(err)
	}

	hub := show.NewHub(nil, "")
	hub.SetSetup([]string{"Strategist: a local model", "Voice: Kokoro"})
	panel := httptest.NewServer(show.Handler(hub, t.TempDir()))
	t.Cleanup(panel.Close)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	narr := &show.Narrator{LLM: &llm.OpenAI{BaseURL: fakeNarrator(t), Model: "narrator"},
		PersonaD: "../../docs/personas", Persona: "shark", MinGap: time.Millisecond, Publish: hub.Publish}
	evs, stop := hub.Subscribe()
	t.Cleanup(stop)
	go narr.Run(ctx, evs)

	live := &show.Live{LogDir: dir, Bridge: bridge, Player: show.Player{ID: "live", Model: "qwen27"}, Poll: 20 * time.Millisecond,
		Publish: func(e show.Event) { hub.Publish(e) }}
	live.Player.SetPersona("../../docs/personas", "shark", "")
	go live.Run(ctx)
	waitState(t, panel.URL, func(s show.State) bool { return s.Plan != nil })

	seq := int64(len(res.Events))
	send := func(kind string, day int, data map[string]any) {
		t.Helper()
		seq++
		b, _ := json.Marshal(map[string]any{"session_id": res.SessionID, "seq": seq, "kind": kind, "game_day": day, "data": data})
		resp, err := http.Post(bridge+"/v1/event", "application/json", bytes.NewReader(b))
		if err != nil || resp.StatusCode != http.StatusNoContent {
			t.Fatalf("event %s: %v %v", kind, resp, err)
		}
		resp.Body.Close()
	}
	ap := func(action, why, detail string) map[string]any {
		return map[string]any{"day": 70, "phase": "customer", "action": action, "why": why, "detail": detail, "client": "Appraiser Board Member"}
	}
	send("run_start", 70, map[string]any{"run_id": "save-3", "difficulty": "hard", "start_type": "apprentice",
		"perks": []map[string]any{{"id": "vending", "name": "Vending Machine"}, {"id": "short_sleeper", "name": "Short Sleeper"}}})
	send("day_snapshot", 70, map[string]any{"phase": "morning", "store": map[string]any{"cash": 2400, "day": 70, "net_worth": 15000},
		"rent": map[string]any{"due": 468, "days_left": 0}, "property": map[string]any{"owned": false, "buyout_price": 24650, "mortgage": map[string]any{"weekly": 1666, "weeks_left": 9}}})
	send("autopilot", 70, ap("dialogue_choice", "Accept (-298 Credits)", "appraisalBoardPromotion [0]"))
	send("autopilot", 70, map[string]any{"day": 70, "phase": "customer", "action": "stage", "target": map[string]any{"uid": 9, "name": "Lamp"},
		"why": "what Mara wants, within the budget", "client": "Mara", "hidden_used": []string{"item_exposable"}})
	send("money", 70, map[string]any{"kind": "mortgage_payment", "amount": 1666, "credits_left": 436})
	send("autopilot_fault", 70, map[string]any{"day": 70, "phase": "customer", "state": "handover", "reason": "Ivo: appraisal customer, not dismissable"})
	send("autopilot", 70, ap("resume", "resumed by the player (overlay)", ""))

	// The slate belongs to this run: post it once the run start has reached the panel.
	waitState(t, panel.URL, func(s show.State) bool { return hasBeat(s.Moments, show.BeatTakeBack) })
	slate := `{"kind":"candidates","day":70,"level":2,"text":"Two plans simulated over 24 rollouts; keeping the plan wins.",` +
		`"candidates":{"day":70,"metric":"buyout progress %","options":[{"label":"keep the plan","source":"base","score":61.2,"stderr":1.1,"rollouts":24,"chosen":true},{"label":"sell down the wine","source":"llm","score":57.9,"stderr":1.4,"rollouts":24}]}}`
	resp, err := http.Post(panel.URL+"/v1/ingest", "application/json", strings.NewReader(slate))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("ingest: %v %v", resp, err)
	}
	resp.Body.Close()

	st := waitState(t, panel.URL, func(s show.State) bool {
		return s.Candidates != nil && s.Plan != nil && s.Say != nil && s.Say.Say.Source == "llm"
	})
	if p := st.Plan.Plan; p.Source != "llm" || len(p.Headline) == 0 || p.Headline[0] != "Take the board's next promotion." {
		t.Errorf("plan on the panel = %+v", p)
	}
	if st.Player == nil || st.Player.PersonaName != "Shark" || st.Player.Model != "qwen27" || len(st.Player.Perks) != 2 {
		t.Errorf("name card = %+v", st.Player)
	}
	for _, b := range []string{show.BeatPromotion, show.BeatMortgagePayment, show.BeatHandover, show.BeatTakeBack} {
		if !hasBeat(st.Moments, b) {
			t.Errorf("no %s among the moments: %+v", b, st.Moments)
		}
	}
	if st.Clock == nil || st.Clock.MortgageWeeksLeft != 8 || st.Clock.Cash != 436 {
		t.Errorf("clock = %+v", st.Clock)
	}
	if st.HiddenUsed != 1 || len(st.Setup) != 2 {
		t.Errorf("about card: hidden %d, setup %v", st.HiddenUsed, st.Setup)
	}
	if o := st.Candidates.Candidates.Options; len(o) != 2 || !o[0].Chosen {
		t.Errorf("candidates = %+v", st.Candidates)
	}
	// The narrator spoke in persona, fact-checked, and broke: 436 does not cover the next 1,666.
	if s := st.Say.Say; (s.Check != "pass" && s.Check != "retry") || s.Mood != "broke" {
		t.Errorf("say = %+v", s)
	}

	// A browser source that joins now draws the same state first.
	resp, err = http.Get(panel.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 4<<20)
	for sc.Scan() && sc.Text() != "event: state" {
	}
	if !sc.Scan() || !strings.Contains(sc.Text(), show.BeatHandover) || !strings.Contains(sc.Text(), `"hidden_used":1`) {
		t.Errorf("SSE state = %.300s", sc.Text())
	}
	for _, page := range []string{"/", "/column", "/lower", "/about"} {
		r, err := http.Get(panel.URL + page)
		if err != nil || r.StatusCode != http.StatusOK {
			t.Errorf("%s: %v %v", page, r, err)
		}
		if r != nil {
			r.Body.Close()
		}
	}
}

// fakeNarrator answers with a different line each time: the narrator speaks a repeat as the caption.
func fakeNarrator(t *testing.T) string {
	lines := []string{"Over to my human. I'll be right here, counting.", "Promotion taken, chin up, eyes sharp.",
		"Another clip off the deed, nerves of steel.", "Back behind the counter where I belong.", "Simulations settled it: the plan holds firm.",
		"Morning comes and the hustle starts fresh.", "Quiet hands, loud ledger.", "Nobody walks out of here without paying."}
	var mu sync.Mutex
	i := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		text := lines[i%len(lines)]
		i++
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": text}}}})
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func hasBeat(evs []show.Event, beat string) bool {
	for _, e := range evs {
		if e.Beat == beat {
			return true
		}
	}
	return false
}

func waitState(t *testing.T, base string, ok func(show.State) bool) show.State {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var s show.State
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/state")
		if err == nil {
			s = show.State{}
			_ = json.NewDecoder(resp.Body).Decode(&s)
			resp.Body.Close()
			if ok(s) {
				return s
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	b, _ := json.Marshal(s)
	t.Fatalf("panel state never got there: %.3000s", b)
	return s
}
