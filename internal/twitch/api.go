// Package twitch is the stream's viewer participation (docs/SHOW.md, "Twitch"): a channel-points
// prediction on each of the strategist's bets and on the run's format verdict, and a chat vote
// over a fixed list whose tally the next morning's plan reads as one fact. Everything runs behind
// two interfaces, Predictions and Chat; the dry-run implementations log the would-be API calls
// and play chat from a script. The real Helix client is a stub (helix.go).
package twitch

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
)

// Twitch's limits on predictions, from the Helix reference
// (https://dev.twitch.tv/docs/api/reference/predictions, "Create Prediction" and "End Prediction",
// checked 2026-11-11) and the predictions guide (https://dev.twitch.tv/docs/api/predictions).
const (
	// MaxTitle: "The title is limited to a maximum of 45 characters."
	MaxTitle = 45
	// MaxOutcomeTitle (outcomes.title): "The title is limited to a maximum of 25 characters."
	MaxOutcomeTitle = 25
	// MinOutcomes, MaxOutcomes: "The list must contain a minimum of 2 choices and up to a maximum of 10 choices."
	MinOutcomes = 2
	MaxOutcomes = 10
	// MinWindow, MaxWindow bound prediction_window: "The minimum is 1 second and the maximum is
	// 1800 seconds (30 minutes)."
	MinWindow = time.Second
	MaxWindow = 1800 * time.Second
	// ResolveWithin: "If you don't select a winning outcome within 24 hours, all points are returned
	// to the viewers." We cancel ourselves an hour earlier, so the refund is ours and on the panel.
	ResolveWithin = 24 * time.Hour
)

// Prediction statuses, as Helix names them.
const (
	StatusActive   = "ACTIVE"
	StatusLocked   = "LOCKED"
	StatusResolved = "RESOLVED"
	StatusCanceled = "CANCELED"
)

// ErrBusy is Helix's 400 on a second prediction: "The broadcaster already has a prediction that's
// running; you may not create another prediction until the current prediction is resolved or
// canceled." A LOCKED prediction still holds the slot.
var ErrBusy = errors.New("twitch: a prediction is already active or locked")

// Prediction is a Helix prediction, as far as the show uses one.
type Prediction struct {
	ID       string
	Title    string
	Status   string
	Outcomes []Outcome
	Window   time.Duration
	Created  time.Time
}

// Outcome is one answer and who backed it (Helix: users, channel_points).
type Outcome struct {
	ID     string
	Title  string
	Users  int64
	Points int64
}

// Predictions is the slice of the Helix predictions API the show needs. Every call is the
// broadcaster's (channel:manage:predictions).
type Predictions interface {
	// Create is POST /helix/predictions.
	Create(ctx context.Context, title string, outcomes []string, window time.Duration) (Prediction, error)
	// End is PATCH /helix/predictions: status LOCKED, RESOLVED (winner is an outcome id) or CANCELED.
	End(ctx context.Context, id, status, winner string) (Prediction, error)
	// Get is GET /helix/predictions?id=…: the split so far.
	Get(ctx context.Context, id string) (Prediction, error)
	// Active is the prediction holding the slot (ACTIVE or LOCKED), if any: a restarted show
	// finds the one it left behind.
	Active(ctx context.Context) (*Prediction, error)
}

// CheckCreate applies Helix's documented checks to a Create call, so a dry run fails where the
// real API would.
func CheckCreate(title string, outcomes []string, window time.Duration) error {
	switch n := utf8.RuneCountInString(title); {
	case n == 0:
		return errors.New("twitch: empty title")
	case n > MaxTitle:
		return fmt.Errorf("twitch: title %q is %d characters, over %d", title, n, MaxTitle)
	}
	if len(outcomes) < MinOutcomes || len(outcomes) > MaxOutcomes {
		return fmt.Errorf("twitch: %d outcomes, want %d-%d", len(outcomes), MinOutcomes, MaxOutcomes)
	}
	for _, o := range outcomes {
		if n := utf8.RuneCountInString(o); n == 0 || n > MaxOutcomeTitle {
			return fmt.Errorf("twitch: outcome %q is %d characters, want 1-%d", o, n, MaxOutcomeTitle)
		}
	}
	if window < MinWindow || window > MaxWindow || window%time.Second != 0 {
		return fmt.Errorf("twitch: prediction_window %s, want whole seconds in %s-%s", window, MinWindow, MaxWindow)
	}
	return nil
}

// CheckEnd applies Helix's status rules to an End call on p: "To update the prediction's status
// to RESOLVED or CANCELED, its current status must be ACTIVE or LOCKED"; LOCKED needs ACTIVE.
func CheckEnd(p Prediction, status, winner string) error {
	open := p.Status == StatusActive || p.Status == StatusLocked
	switch status {
	case StatusLocked:
		if p.Status != StatusActive {
			return fmt.Errorf("twitch: cannot lock a %s prediction", p.Status)
		}
	case StatusCanceled:
		if !open {
			return fmt.Errorf("twitch: cannot cancel a %s prediction", p.Status)
		}
	case StatusResolved:
		if !open {
			return fmt.Errorf("twitch: cannot resolve a %s prediction", p.Status)
		}
		for _, o := range p.Outcomes {
			if o.ID == winner {
				return nil
			}
		}
		return fmt.Errorf("twitch: winning_outcome_id %q is not an outcome", winner)
	default:
		return fmt.Errorf("twitch: unknown status %q", status)
	}
	return nil
}
