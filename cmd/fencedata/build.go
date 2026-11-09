package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/memory"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Day is one line of a trajectory file: everything recorded about one in-game day of a
// run. Hidden values appear only under Labels.
type Day struct {
	Run          string        `json:"run"`
	RunID        int64         `json:"run_id"`
	GameDay      int           `json:"game_day"`
	Sessions     []SessionInfo `json:"sessions"`
	Snapshots    []Snapshot    `json:"snapshots"`
	Customers    []Customer    `json:"customers"`
	Decisions    []Decision    `json:"decisions"`
	HumanActions []HumanAction `json:"human_actions"`
	Outcomes     []Outcome     `json:"outcomes"`
	Events       []Event       `json:"events"`
	Labels       []Label       `json:"labels"`
}

type SessionInfo struct {
	SessionID   string        `json:"session_id"`
	Mode        protocol.Mode `json:"mode,omitempty"`
	Policy      string        `json:"policy,omitempty"`
	ModVersion  string        `json:"mod_version,omitempty"`
	GameVersion string        `json:"game_version,omitempty"`
}

type Snapshot struct {
	SessionID string          `json:"session_id"`
	Seq       int64           `json:"seq"`
	TS        time.Time       `json:"ts"`
	Phase     string          `json:"phase"`
	Data      json.RawMessage `json:"data"`
}

type Customer struct {
	SessionID string          `json:"session_id"`
	Seq       int64           `json:"seq"`
	TS        time.Time       `json:"ts"`
	ClientSeq *int64          `json:"client_seq"`
	Covered   bool            `json:"covered"`
	Reason    string          `json:"reason"`
	Context   json.RawMessage `json:"context"`
	Options   json.RawMessage `json:"options"`
}

type Decision struct {
	SessionID      string            `json:"session_id"`
	TS             time.Time         `json:"ts"`
	DecisionID     string            `json:"decision_id"`
	Kind           string            `json:"kind"`
	ClientSeq      *int64            `json:"client_seq"`
	Mode           protocol.Mode     `json:"mode"`
	Policy         string            `json:"policy"`
	StateSignature string            `json:"state_signature,omitempty"`
	Context        json.RawMessage   `json:"context"`
	Options        []protocol.Option `json:"options"`
	Response       json.RawMessage   `json:"response"`
	Trace          json.RawMessage   `json:"trace,omitempty"`
	LatencyMS      int64             `json:"latency_ms"`
}

type HumanAction struct {
	SessionID  string          `json:"session_id"`
	Seq        int64           `json:"seq"`
	TS         time.Time       `json:"ts"`
	ClientSeq  *int64          `json:"client_seq"`
	Action     string          `json:"action"`
	ItemUID    *int64          `json:"item_uid"`
	DecisionID string          `json:"decision_id,omitempty"`
	Advised    string          `json:"advised,omitempty"`
	Matched    *bool           `json:"matched"`
	Data       json.RawMessage `json:"data"`
}

type Outcome struct {
	SessionID     string          `json:"session_id"`
	TS            time.Time       `json:"ts"`
	DecisionID    string          `json:"decision_id"`
	ClientSeq     *int64          `json:"client_seq"`
	KnownDecision bool            `json:"known_decision"`
	Result        json.RawMessage `json:"result"`
}

type Event struct {
	SessionID string          `json:"session_id"`
	Seq       int64           `json:"seq"`
	TS        time.Time       `json:"ts"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
}

// Label is a hidden value and the record it belongs to: a decision (DecisionID) or an
// event (Seq).
type Label struct {
	Source     string          `json:"source"`
	SessionID  string          `json:"session_id"`
	Seq        *int64          `json:"seq,omitempty"`
	DecisionID string          `json:"decision_id,omitempty"`
	ClientSeq  *int64          `json:"client_seq"`
	Hidden     json.RawMessage `json:"hidden"`
}

// Trajectory is one run's days in order.
type Trajectory struct {
	Name  string
	RunID int64
	Days  []*Day
	// Sessions lists the sessions with records in this run, in log order.
	Sessions []string
}

// Logged record shapes (internal/server/records.go); only what the export reads.
type helloRec struct {
	Mode   protocol.Mode `json:"mode"`
	Policy string        `json:"policy"`
	Hello  struct {
		ModVersion string `json:"mod_version"`
		Game       struct {
			Version string `json:"version"`
		} `json:"game"`
	} `json:"hello"`
}

type eventRec struct {
	Event protocol.Event `json:"event"`
}

type decisionRec struct {
	DecisionID string                 `json:"decision_id"`
	Mode       protocol.Mode          `json:"mode"`
	Policy     string                 `json:"policy"`
	Request    protocol.DecideRequest `json:"request"`
	Trace      json.RawMessage        `json:"trace"`
	Response   json.RawMessage        `json:"response"`
	LatencyMS  int64                  `json:"latency_ms"`
}

type outcomeRec struct {
	DecisionID    string           `json:"decision_id"`
	KnownDecision bool             `json:"known_decision"`
	Outcome       protocol.Outcome `json:"outcome"`
}

type humanActionData struct {
	ClientSeq  *int64 `json:"client_seq"`
	Action     string `json:"action"`
	ItemUID    *int64 `json:"item_uid"`
	DecisionID string `json:"decision_id"`
	Advised    string `json:"advised"`
}

type customerData struct {
	ClientSeq *int64          `json:"client_seq"`
	Covered   bool            `json:"covered"`
	Reason    string          `json:"reason"`
	Context   json.RawMessage `json:"context"`
	Options   json.RawMessage `json:"options"`
}

// build groups every session's records into per-run trajectories. The logs are the
// source of truth; the index only says which run a session's day belongs to. Sessions
// memory never bound to a run become a trajectory of their own.
func build(sessions []decisionlog.Session, index memory.RunIndex) []*Trajectory {
	byName := map[string]*Trajectory{}
	var order []string
	dayOf := map[string]map[int]*Day{}
	get := func(session string, day int, info SessionInfo) *Day {
		name, id := runName(index, session, day)
		tr := byName[name]
		if tr == nil {
			tr = &Trajectory{Name: name, RunID: id}
			byName[name] = tr
			dayOf[name] = map[int]*Day{}
			order = append(order, name)
		}
		if !slices.Contains(tr.Sessions, session) {
			tr.Sessions = append(tr.Sessions, session)
		}
		d := dayOf[name][day]
		if d == nil {
			d = newDay(name, id, day)
			dayOf[name][day] = d
			tr.Days = append(tr.Days, d)
		}
		if !slices.ContainsFunc(d.Sessions, func(s SessionInfo) bool { return s.SessionID == session }) {
			d.Sessions = append(d.Sessions, info)
		}
		return d
	}
	for _, s := range sessions {
		buildSession(s, get)
	}
	out := make([]*Trajectory, 0, len(order))
	for _, name := range order {
		tr := byName[name]
		slices.SortStableFunc(tr.Days, func(a, b *Day) int { return a.GameDay - b.GameDay })
		out = append(out, tr)
	}
	slices.SortStableFunc(out, func(a, b *Trajectory) int {
		switch {
		case a.RunID != 0 && b.RunID != 0:
			return int(a.RunID - b.RunID)
		case a.RunID != 0:
			return -1
		case b.RunID != 0:
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

func newDay(name string, id int64, day int) *Day {
	return &Day{Run: name, RunID: id, GameDay: day, Sessions: []SessionInfo{}, Snapshots: []Snapshot{}, Customers: []Customer{},
		Decisions: []Decision{}, HumanActions: []HumanAction{}, Outcomes: []Outcome{}, Events: []Event{}, Labels: []Label{}}
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func runName(index memory.RunIndex, session string, day int) (string, int64) {
	if id, ok := index.Run(session, day); ok {
		return fmt.Sprintf("run-%04d", id), id
	}
	return "session-" + unsafeName.ReplaceAllString(session, "_"), 0
}

func buildSession(s decisionlog.Session, get func(session string, day int, info SessionInfo) *Day) {
	info := SessionInfo{SessionID: s.ID}
	// Decisions name their customer in the signature; a human action naming the decision
	// fills in the rest.
	clientOf := map[string]*int64{}
	options := map[string][]string{}
	for _, r := range s.Records {
		switch r.Type {
		case "hello":
			var h helloRec
			if json.Unmarshal(r.Raw, &h) == nil {
				info = SessionInfo{SessionID: s.ID, Mode: h.Mode, Policy: h.Policy, ModVersion: h.Hello.ModVersion, GameVersion: h.Hello.Game.Version}
			}
		case "decision":
			var d decisionRec
			if json.Unmarshal(r.Raw, &d) != nil {
				continue
			}
			if n, ok := memory.ClientSeqFromSignature(d.Request.StateSignature); ok {
				clientOf[d.DecisionID] = &n
			}
			ids := make([]string, 0, len(d.Request.Options))
			for _, o := range d.Request.Options {
				ids = append(ids, o.ID)
			}
			options[d.DecisionID] = ids
		}
	}
	for _, r := range s.Records {
		if r.Type != "event" {
			continue
		}
		var e eventRec
		if json.Unmarshal(r.Raw, &e) != nil || e.Event.Kind != memory.KindHumanAction {
			continue
		}
		var a humanActionData
		if json.Unmarshal(e.Event.Data, &a) == nil && a.DecisionID != "" && a.ClientSeq != nil && clientOf[a.DecisionID] == nil {
			clientOf[a.DecisionID] = a.ClientSeq
		}
	}

	seen := map[int64]bool{}
	for _, r := range s.Records {
		switch r.Type {
		case "event":
			var e eventRec
			if json.Unmarshal(r.Raw, &e) != nil {
				continue
			}
			ev := e.Event
			if seen[ev.Seq] {
				continue
			}
			seen[ev.Seq] = true
			d := get(s.ID, ev.GameDay, info)
			data := compact(ev.Data)
			var clientSeq *int64
			switch ev.Kind {
			case memory.KindDaySnapshot:
				var p struct {
					Phase string `json:"phase"`
				}
				json.Unmarshal(ev.Data, &p)
				d.Snapshots = append(d.Snapshots, Snapshot{SessionID: s.ID, Seq: ev.Seq, TS: r.TS, Phase: p.Phase, Data: data})
			case memory.KindCustomerObserved:
				var c customerData
				json.Unmarshal(ev.Data, &c)
				clientSeq = c.ClientSeq
				d.Customers = append(d.Customers, Customer{SessionID: s.ID, Seq: ev.Seq, TS: r.TS, ClientSeq: c.ClientSeq, Covered: c.Covered,
					Reason: c.Reason, Context: orNull(compact(c.Context)), Options: orNull(compact(c.Options))})
			case memory.KindHumanAction:
				var a humanActionData
				json.Unmarshal(ev.Data, &a)
				clientSeq = a.ClientSeq
				var opts []string
				if a.DecisionID != "" {
					opts = options[a.DecisionID]
				}
				d.HumanActions = append(d.HumanActions, HumanAction{SessionID: s.ID, Seq: ev.Seq, TS: r.TS, ClientSeq: a.ClientSeq,
					Action: a.Action, ItemUID: a.ItemUID, DecisionID: a.DecisionID, Advised: a.Advised,
					Matched: memory.AdviceMatched(a.Action, a.Advised, opts), Data: data})
			default:
				d.Events = append(d.Events, Event{SessionID: s.ID, Seq: ev.Seq, TS: r.TS, Kind: ev.Kind, Data: data})
			}
			if hasHidden(ev.Hidden) {
				seq := ev.Seq
				d.Labels = append(d.Labels, Label{Source: ev.Kind, SessionID: s.ID, Seq: &seq, ClientSeq: clientSeq, Hidden: compact(ev.Hidden)})
			}
		case "decision":
			var dr decisionRec
			if json.Unmarshal(r.Raw, &dr) != nil {
				continue
			}
			req := dr.Request
			d := get(s.ID, req.GameDay, info)
			id := nonEmpty(dr.DecisionID, req.DecisionID)
			opts := req.Options
			if opts == nil {
				opts = []protocol.Option{}
			}
			d.Decisions = append(d.Decisions, Decision{SessionID: s.ID, TS: r.TS, DecisionID: id, Kind: req.Kind, ClientSeq: clientOf[id],
				Mode: dr.Mode, Policy: dr.Policy, StateSignature: req.StateSignature, Context: orNull(compact(req.Context)), Options: opts,
				Response: orNull(compact(dr.Response)), Trace: compact(dr.Trace), LatencyMS: dr.LatencyMS})
			if hasHidden(req.Hidden) {
				d.Labels = append(d.Labels, Label{Source: "decision", SessionID: s.ID, DecisionID: id, ClientSeq: clientOf[id], Hidden: compact(req.Hidden)})
			}
		case "outcome":
			var or outcomeRec
			if json.Unmarshal(r.Raw, &or) != nil {
				continue
			}
			oc := or.Outcome
			id := nonEmpty(or.DecisionID, oc.DecisionID)
			d := get(s.ID, oc.GameDay, info)
			d.Outcomes = append(d.Outcomes, Outcome{SessionID: s.ID, TS: r.TS, DecisionID: id, ClientSeq: clientOf[id],
				KnownDecision: or.KnownDecision, Result: orNull(compact(oc.Result))})
		}
	}
}

func hasHidden(raw json.RawMessage) bool {
	t := bytes.TrimSpace(raw)
	return len(t) > 0 && !bytes.Equal(t, []byte("null")) && !bytes.Equal(t, []byte("{}"))
}

// compact makes the output independent of how the mod spaced its JSON.
func compact(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return nil
	}
	return b.Bytes()
}

func orNull(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return json.RawMessage("null")
	}
	return raw
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
