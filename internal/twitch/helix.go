package twitch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrNotBuilt is every live call: the Helix and EventSub clients are stubs until they can be
// tested without credentials (docs/STREAM-OPS.md, "Twitch").
var ErrNotBuilt = errors.New("twitch: the live Helix/EventSub client is not built yet; use -dryrun")

// The environment the live client will read. The token lives in a file outside the repo.
const (
	EnvClientID    = "TWITCH_CLIENT_ID"
	EnvBroadcaster = "TWITCH_BROADCASTER_ID"
	EnvTokenFile   = "TWITCH_TOKEN_FILE"
)

// LiveConfig is what the live client needs. The scopes it will ask for: channel:manage:predictions
// (create, lock, resolve, cancel and read predictions) and user:read:chat (EventSub
// channel.chat.message over a WebSocket, which takes a user access token).
type LiveConfig struct {
	ClientID      string
	BroadcasterID string
	TokenFile     string
}

// Scopes are the OAuth scopes the owner grants the app.
var Scopes = []string{"channel:manage:predictions", "user:read:chat"}

// LiveConfigFromEnv reads the client's settings and refuses a token file inside the working tree,
// so a token can never be committed by accident.
func LiveConfigFromEnv(getenv func(string) string) (LiveConfig, error) {
	c := LiveConfig{ClientID: getenv(EnvClientID), BroadcasterID: getenv(EnvBroadcaster), TokenFile: getenv(EnvTokenFile)}
	var missing []string
	for k, v := range map[string]string{EnvClientID: c.ClientID, EnvBroadcaster: c.BroadcasterID, EnvTokenFile: c.TokenFile} {
		if v == "" {
			missing = append(missing, k)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		return c, fmt.Errorf("twitch: set %s", strings.Join(missing, ", "))
	}
	abs, err := filepath.Abs(c.TokenFile)
	if err != nil {
		return c, err
	}
	if wd, err := os.Getwd(); err == nil && (abs == wd || strings.HasPrefix(abs, wd+string(filepath.Separator))) {
		return c, fmt.Errorf("twitch: %s is inside the working tree; keep the token outside the repo (e.g. ~/.config/fence/twitch-token)", EnvTokenFile)
	}
	return c, nil
}

// Helix is the live Predictions client: POST/PATCH/GET https://api.twitch.tv/helix/predictions
// with the headers Authorization: Bearer <token> and Client-Id. A stub: every call fails.
type Helix struct{ Config LiveConfig }

func (Helix) Create(context.Context, string, []string, time.Duration) (Prediction, error) {
	return Prediction{}, ErrNotBuilt
}
func (Helix) End(context.Context, string, string, string) (Prediction, error) {
	return Prediction{}, ErrNotBuilt
}
func (Helix) Get(context.Context, string) (Prediction, error) { return Prediction{}, ErrNotBuilt }
func (Helix) Active(context.Context) (*Prediction, error)     { return nil, ErrNotBuilt }

// EventSubChat is the live Chat: a WebSocket to wss://eventsub.wss.twitch.tv/ws and a
// channel.chat.message v1 subscription (condition broadcaster_user_id and user_id, the
// broadcaster's own id when it reads its own chat). A stub: it delivers nothing.
type EventSubChat struct{ Config LiveConfig }

func (EventSubChat) Due(time.Time) []Message { return nil }
