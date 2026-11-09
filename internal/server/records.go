package server

import (
	"time"

	"github.com/gallowaysoftware/fence/internal/policy"
	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Decision-log record shapes. Every record carries type, ts and session_id; decisions
// and outcomes join on decision_id.
type header struct {
	Type      string    `json:"type"`
	TS        time.Time `json:"ts"`
	SessionID string    `json:"session_id"`
}

func (s *Server) header(typ, session string) header {
	return header{Type: typ, TS: s.cfg.Now().UTC(), SessionID: session}
}

type helloRecord struct {
	header
	Mode   protocol.Mode         `json:"mode"`
	Policy string                `json:"policy"`
	Hello  protocol.HelloRequest `json:"hello"`
}

type eventRecord struct {
	header
	Missed     int64          `json:"missed,omitempty"`
	OutOfOrder bool           `json:"out_of_order,omitempty"`
	Event      protocol.Event `json:"event"`
}

type decisionRecord struct {
	header
	DecisionID string                  `json:"decision_id"`
	Mode       protocol.Mode           `json:"mode"`
	Policy     string                  `json:"policy"`
	Request    protocol.DecideRequest  `json:"request"`
	Trace      policy.Trace            `json:"trace"`
	Response   protocol.DecideResponse `json:"response"`
	LatencyMS  int64                   `json:"latency_ms"`
}

type outcomeRecord struct {
	header
	DecisionID    string           `json:"decision_id"`
	KnownDecision bool             `json:"known_decision"`
	Outcome       protocol.Outcome `json:"outcome"`
}
