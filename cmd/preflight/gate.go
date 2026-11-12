package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gallowaysoftware/fence/internal/gate"
)

// gates is what fencegate (docs/STREAM-OPS.md, "The drift gate") says about the strategist's
// and the narrator's endpoints; nil where an endpoint is not a gate.
type gates struct{ strat, narr *gate.Status }

func probeGates(ctx context.Context, hc *http.Client, c config) gates {
	probe := func(u string) *gate.Status {
		if u == "" {
			return nil
		}
		if st, ok := gate.Probe(ctx, hc, u); ok {
			return &st
		}
		return nil
	}
	g := gates{narr: probe(c.narrURL)}
	if c.stratProvider == "openai" {
		g.strat = probe(c.stratURL)
	}
	return g
}

// narrRemote: the narrator's loopback URL is a gate in front of another machine, so its model
// neither loads next to the strategist nor yields to the game.
func (g gates) narrRemote() bool { return g.narr != nil && !g.narr.UpstreamLocal }

// checkGate reports the gate when either role goes through one, warns when both roles call
// the same model at the same URL without one, or when a narrator with a model of its own queues
// behind the plans in it. ok is false when neither applies, so a rig without a gate keeps its table.
func checkGate(c config, g gates) (r Result, ok bool) {
	r = Result{Name: "drift gate"}
	st := g.strat
	if st == nil {
		st = g.narr
	}
	switch {
	case st == nil && c.stratProvider == "openai" && c.stratURL != "" && c.stratURL == c.narrURL && c.stratModel == c.narrModel:
		r.Status, r.Detail = Warn, "strategist and narrator call the same model without fencegate: the second caller waits behind the first or gets HTTP 503, and the narrator speaks captions (make gate)"
		return r, true
	case st == nil:
		return r, false
	case st.UpstreamLocal:
		r.Status, r.Detail = Warn, "the gate fronts a model on this machine: on stream the 5090 runs only the game and the encoder"
		return r, true
	}
	var who []string
	if g.strat != nil {
		who = append(who, "strategist")
	}
	if g.narr != nil {
		who = append(who, "narrator")
	}
	r.Status = Go
	r.Detail = fmt.Sprintf("%s through the gate to the remote endpoint", join(who))
	ownModel := c.stratProvider == "openai" && c.narrURL != "" && c.narrModel != "" && c.narrModel != c.stratModel
	switch {
	case g.strat == nil && c.stratProvider == "openai":
		r.Status, r.Detail = Warn, "only the narrator goes through the gate: point the bridge at its /plan/v1 too, or the strategist meets 503s"
	case g.narr != nil && ownModel:
		// The gate sends one request at a time, so a model of the narrator's own would still wait behind plans.
		r.Status, r.Detail = Warn, "the narrator has its own model ("+c.narrModel+") but goes through the gate, so its lines wait behind plans: point it at drift directly"
	case g.narr == nil && ownModel:
		r.Detail += "; the narrator on its own model, beside the gate"
	}
	if p := st.Roles["plan"]; p.Served > 0 {
		r.Detail += fmt.Sprintf("; plans p50 %d ms", p.UpstreamMS.P50)
	}
	if n := st.Roles["narrate"]; n.Requests > 0 {
		r.Detail += fmt.Sprintf("; narrator lines dropped %d of %d", n.Dropped, n.Requests)
	}
	return r, true
}

func join(s []string) string {
	if len(s) == 2 {
		return s[0] + " and " + s[1]
	}
	if len(s) == 1 {
		return s[0]
	}
	return "nothing"
}
