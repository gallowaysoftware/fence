// Package memory is what the bridge remembers across decisions, days and runs.
package memory

import (
	"context"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Recall supplies remembered facts for a decision. Everything it returns must be derivable
// from player-visible context and outcomes: it reaches the prompt, so nothing from
// DecideRequest.Hidden may flow into it.
type Recall interface {
	// Brief returns prompt-ready notes relevant to req, at most maxBytes, or "" when there
	// is nothing useful. It must not block past ctx.
	Brief(ctx context.Context, req *protocol.DecideRequest, maxBytes int) string
}

// None is the Recall used when memory is off.
type None struct{}

func (None) Brief(context.Context, *protocol.DecideRequest, int) string { return "" }
