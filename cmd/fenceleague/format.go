package main

import (
	"github.com/gallowaysoftware/fence/internal/format"
)

// recordFormat appends a finished format match to the local record the stream's format card
// reads; a crash is no attempt.
func recordFormat(m Match, r *Result) error {
	if m.Format == "" || r.Ending == "crash" {
		return nil
	}
	f, err := format.Load("", m.Format)
	if err != nil {
		return err
	}
	_, err = format.Append(format.DefaultRecord, format.Attempt{Format: f.Name, Run: r.Match, Source: "league", Model: r.Model,
		Persona: r.Persona, Route: r.Route, Seed: int64(r.Seed), Ending: r.Ending, Day: r.LastDay, Won: f.Won(r.Ending, r.LastDay)})
	return err
}
