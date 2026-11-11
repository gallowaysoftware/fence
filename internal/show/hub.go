package show

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is what a panel needs to draw itself from scratch: an OBS browser source reloads,
// and a viewer joins mid-run.
type State struct {
	Player     *Player    `json:"player,omitempty"`
	Run        string     `json:"run,omitempty"`
	Day        int        `json:"day"`
	Clock      *Clock     `json:"clock,omitempty"`
	Heat       *Heat      `json:"heat,omitempty"`
	Plan       *Event     `json:"plan,omitempty"`
	Candidates *Event     `json:"candidates,omitempty"`
	Ghost      *Event     `json:"ghost,omitempty"`
	Summary    *Event     `json:"summary,omitempty"`
	End        *Event     `json:"end,omitempty"`
	Say        *Event     `json:"say,omitempty"`
	Ticker     []Event    `json:"ticker"`
	Moments    []Event    `json:"moments"`
	NetWorth   [][2]int64 `json:"net_worth"`
	Board      []BoardRow `json:"board"`
	// HiddenUsed counts this run's entries where the hands read a hidden value (the ◆ marks),
	// for the "about this AI" card.
	HiddenUsed int `json:"hidden_used"`
	// Setup is the stream's own description of the rig for that card (fenceshow -setup).
	Setup []string `json:"setup,omitempty"`
}

const (
	tickerKeep  = 10
	momentsKeep = 6
	subBuffer   = 256
)

// Hub numbers events, keeps the panel state, records the feed and fans events out to
// subscribers. A slow subscriber loses events rather than holding up the game.
type Hub struct {
	mu     sync.Mutex
	seq    int64
	state  State
	subs   map[chan Event]struct{}
	rec    io.Writer
	board  string
	now    func() time.Time
	closed bool
}

// NewHub records the feed to rec (nil: none) and keeps the scoreboard in boardPath ("":
// in memory only).
func NewHub(rec io.Writer, boardPath string) *Hub {
	h := &Hub{subs: map[chan Event]struct{}{}, rec: rec, board: boardPath, now: time.Now}
	if boardPath != "" {
		if b, err := os.ReadFile(boardPath); err == nil {
			_ = json.Unmarshal(b, &h.state.Board)
		}
	}
	return h
}

// SetSetup puts the rig's description on the "about this AI" card.
// ContinueFrom numbers the next event after seq, so a recording appended to across restarts
// keeps one sequence and a line's "for" names one moment.
func (h *Hub) ContinueFrom(seq int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq = max(h.seq, seq)
}

// LastSeq is the highest seq in a recorded feed; 0 when there is none.
func LastSeq(r io.Reader) int64 {
	var last int64
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var e struct {
			Seq int64 `json:"seq"`
		}
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			last = max(last, e.Seq)
		}
	}
	return last
}

func (h *Hub) SetSetup(lines []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.state.Setup = lines
}

// SeedBoard adds finished runs the board does not have yet.
func (h *Hub) SeedBoard(rows []BoardRow) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range rows {
		if h.boardIndex(r.Run) < 0 {
			h.state.Board = append(h.state.Board, r)
		}
	}
}

func (h *Hub) boardIndex(run string) int {
	for i, r := range h.state.Board {
		if r.Run == run {
			return i
		}
	}
	return -1
}

// Publish stamps ev and sends it to every subscriber, returning it as sent.
func (h *Hub) Publish(ev Event) Event {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ev
	}
	h.seq++
	ev.Seq = h.seq
	ev.V = SchemaVersion
	ev.At = h.now().UnixMilli()
	h.apply(ev)
	if h.rec != nil {
		if b, err := json.Marshal(ev); err == nil {
			_, _ = h.rec.Write(append(b, '\n'))
		}
	}
	for ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	return ev
}

func (h *Hub) apply(ev Event) {
	s := &h.state
	if ev.Day > s.Day || ev.Kind == KindRunStart {
		s.Day = ev.Day
	}
	if ev.Clock != nil {
		c := *ev.Clock
		s.Clock = &c
	}
	switch ev.Kind {
	case KindRunStart:
		*s = State{Board: s.Board, Setup: s.Setup, Player: ev.Player, Run: ev.Run, Day: ev.Day}
		if p := ev.Player; p != nil {
			if i := h.boardIndex(ev.Run); i >= 0 {
				s.Board = append(s.Board[:i], s.Board[i+1:]...)
			}
			s.Board = append(s.Board, BoardRow{Run: ev.Run, Player: p.ID, Model: p.Model, Persona: p.PersonaName, Difficulty: p.Difficulty, Days: ev.Day, Live: true})
		}
	case KindPlan:
		e := ev
		s.Plan = &e
	case KindCandidates:
		e := ev
		s.Candidates = &e
	case KindHeat:
		s.Heat = ev.Heat
	case KindGhost:
		e := ev
		s.Ghost = &e
	case KindDay:
		e := ev
		s.Summary = &e
		if ev.Clock != nil {
			s.NetWorth = append(s.NetWorth, [2]int64{int64(ev.Day), ev.Clock.NetWorth})
		}
		if i := h.boardIndex(s.Run); i >= 0 && ev.Clock != nil {
			s.Board[i].Days, s.Board[i].NetWorth = ev.Day, ev.Clock.NetWorth
		}
	case KindRunEnd:
		e := ev
		s.End = &e
		if i := h.boardIndex(s.Run); i >= 0 && ev.End != nil {
			r := &s.Board[i]
			r.Live, r.Ending, r.Days, r.Won = false, ev.End.Ending, ev.End.Day, ev.End.Won
			if ev.End.NetWorth > 0 {
				r.NetWorth = ev.End.NetWorth
			}
			h.saveBoard()
		}
	case KindSay:
		e := ev
		s.Say = &e
		return
	}
	if len(ev.HiddenUsed) > 0 {
		s.HiddenUsed++
	}
	if (ev.Kind == KindThought || ev.Level == LevelTicker) && ev.Kind != KindGhost {
		s.Ticker = appendKeep(s.Ticker, ev, tickerKeep)
	} else if ev.Kind != KindPlan && ev.Kind != KindCandidates && ev.Level > LevelTicker {
		s.Moments = appendKeep(s.Moments, ev, momentsKeep)
	}
}

func appendKeep(list []Event, ev Event, n int) []Event {
	list = append(list, ev)
	if len(list) > n {
		list = append([]Event(nil), list[len(list)-n:]...)
	}
	return list
}

func (h *Hub) saveBoard() {
	if h.board == "" {
		return
	}
	b, err := json.MarshalIndent(h.state.Board, "", " ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(h.board), 0o755)
	_ = os.WriteFile(h.board, b, 0o644)
}

// State is a copy of the panel state.
func (h *Hub) State() State {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, _ := json.Marshal(h.state)
	var s State
	_ = json.Unmarshal(b, &s)
	return s
}

// Subscribe returns a channel of every event published from now on, and its cancel.
func (h *Hub) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, subBuffer)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

// Close ends every subscription.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for ch := range h.subs {
		delete(h.subs, ch)
		close(ch)
	}
}

// ServeSSE streams the state, then every event, as server-sent events: `event: state`
// once, then unnamed messages, one Event each.
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	ch, cancel := h.Subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	st, _ := json.Marshal(h.State())
	fmt.Fprintf(w, "retry: 2000\nevent: state\ndata: %s\n\n", st)
	fl.Flush()
	beat := time.NewTicker(15 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-beat.C:
			fmt.Fprint(w, ": beat\n\n")
			fl.Flush()
		case ev, ok := <-ch:
			if !ok {
				return
			}
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Seq, b)
			fl.Flush()
		}
	}
}

// maxIngest bounds one POST to /v1/ingest.
const maxIngest = 1 << 20

// ServeIngest publishes events another process POSTs: one JSON Event, or JSON lines. It
// is how the hybrid planner's candidates (and any later bridge hook) reach the show.
func (h *Hub) ServeIngest(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxIngest))
	n := 0
	for {
		var ev Event
		err := dec.Decode(&ev)
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, "bad event: "+err.Error(), http.StatusBadRequest)
			return
		}
		if ev.Kind == "" || ev.Text == "" {
			http.Error(w, "an event needs kind and text", http.StatusBadRequest)
			return
		}
		h.Publish(ev)
		n++
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\"published\":%d}\n", n)
}
