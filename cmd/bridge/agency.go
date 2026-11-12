package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/gallowaysoftware/fence/internal/agency"
	"github.com/gallowaysoftware/fence/internal/decisionlog"
	"github.com/gallowaysoftware/fence/internal/show"
	"github.com/gallowaysoftware/fence/internal/strategy"
)

func readChoice(path string) (*strategy.Choice, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("-choice: %w", err)
	}
	var ch agency.Choice
	if err := json.Unmarshal(b, &ch); err != nil {
		return nil, fmt.Errorf("-choice %s: %w", path, err)
	}
	out := &strategy.Choice{Route: ch.Route, Composed: ch.Composed, Thesis: ch.Thesis, Fallback: ch.Fallback, Menu: ch.Menu}
	if ch.Composition != nil {
		out.Summary = ch.Composition.Summary()
	}
	return out, nil
}

// showRecord is a feed event in the decision log, as fence-rollout writes them
// (internal/show's builder passes it through).
type showRecord struct {
	Type      string     `json:"type"`
	TS        time.Time  `json:"ts"`
	SessionID string     `json:"session_id"`
	Show      show.Event `json:"show"`
}

// showTo writes the strategist's feed events into the decision log *ref points to once set.
func showTo(ref **decisionlog.Logger) func(string, show.Event) {
	return func(session string, ev show.Event) {
		if l := *ref; l != nil {
			l.Write(session, showRecord{Type: "show", TS: time.Now().UTC(), SessionID: session, Show: ev})
		}
	}
}
