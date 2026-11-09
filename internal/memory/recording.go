package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// Recording event kinds (docs/PROTOCOL.md, "Recording events (phase D)").
const (
	KindDaySnapshot      = "day_snapshot"
	KindCustomerObserved = "customer_observed"
	KindHumanAction      = "human_action"
)

type daySnapshot struct {
	Phase string `json:"phase"`
	Store struct {
		Cash         *float64 `json:"cash"`
		RentDue      *float64 `json:"rent_due"`
		RentDaysLeft *float64 `json:"rent_days_left"`
	} `json:"store"`
	// docs/re/DAY.md moved rent out of store; the store fields are the provisional shape.
	Rent struct {
		Due      *float64 `json:"due"`
		DaysLeft *float64 `json:"days_left"`
	} `json:"rent"`
	Reputation map[string]struct {
		Value    float64 `json:"value"`
		Status   string  `json:"status"`
		NextPerk *struct {
			ID        string   `json:"id"`
			Threshold *float64 `json:"threshold"`
		} `json:"next_perk"`
	} `json:"reputation"`
}

type customerObserved struct {
	ClientSeq *int64          `json:"client_seq"`
	Covered   bool            `json:"covered"`
	Reason    string          `json:"reason"`
	Context   json.RawMessage `json:"context"`
	Options   json.RawMessage `json:"options"`
}

type humanAction struct {
	ClientSeq  *int64 `json:"client_seq"`
	Action     string `json:"action"`
	ItemUID    *int64 `json:"item_uid"`
	DecisionID string `json:"decision_id"`
	Advised    string `json:"advised"`
}

// recordEventFacts extracts the recording events into their tables. Only the event's
// visible data is read: the store never receives hidden values.
func recordEventFacts(ctx context.Context, tx *sql.Tx, session string, runID int64, ev protocol.Event, at string) error {
	switch ev.Kind {
	case KindDaySnapshot:
		return recordSnapshot(ctx, tx, session, runID, ev, at)
	case KindCustomerObserved:
		return recordCustomer(ctx, tx, session, runID, ev, at)
	case KindHumanAction:
		return recordHumanAction(ctx, tx, session, runID, ev, at)
	}
	return nil
}

func recordSnapshot(ctx context.Context, tx *sql.Tx, session string, runID int64, ev protocol.Event, at string) error {
	var d daySnapshot
	json.Unmarshal(ev.Data, &d)
	res, err := tx.ExecContext(ctx, `INSERT INTO day_snapshots (run_id, session_id, seq, game_day, phase, cash, rent_due, rent_days_left, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (session_id, seq) DO NOTHING`,
		runID, session, ev.Seq, ev.GameDay, d.Phase, roundPtr(d.Store.Cash), roundPtr(firstSet(d.Rent.Due, d.Store.RentDue)), roundPtr(firstSet(d.Rent.DaysLeft, d.Store.RentDaysLeft)), string(ev.Data), at)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, f := range sortedKeys(d.Reputation) {
		r := d.Reputation[f]
		var perk string
		var threshold *float64
		if r.NextPerk != nil {
			perk, threshold = r.NextPerk.ID, r.NextPerk.Threshold
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO day_snapshot_rep (snapshot_id, faction, value, status, next_perk, next_perk_threshold) VALUES (?, ?, ?, ?, ?, ?)`,
			id, f, r.Value, r.Status, perk, threshold); err != nil {
			return err
		}
	}
	return nil
}

func recordCustomer(ctx context.Context, tx *sql.Tx, session string, runID int64, ev protocol.Event, at string) error {
	var d customerObserved
	json.Unmarshal(ev.Data, &d)
	var name, faction, intent string
	if c, ok := parseBarter(d.Context); ok {
		name, faction, intent = c.Client.Name, c.Client.Faction, c.Client.Intent
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO customers
		(run_id, session_id, seq, game_day, client_seq, covered, reason, client_name, client_faction, client_intent, context, options, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (session_id, seq) DO NOTHING`,
		runID, session, ev.Seq, ev.GameDay, d.ClientSeq, d.Covered, d.Reason, name, faction, intent, string(d.Context), string(d.Options), at)
	return err
}

func recordHumanAction(ctx context.Context, tx *sql.Tx, session string, runID int64, ev protocol.Event, at string) error {
	var d humanAction
	json.Unmarshal(ev.Data, &d)
	matched, err := adviceMatched(ctx, tx, session, d)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO human_actions
		(run_id, session_id, seq, game_day, client_seq, action, item_uid, decision_id, advised, matched, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT (session_id, seq) DO NOTHING`,
		runID, session, ev.Seq, ev.GameDay, d.ClientSeq, d.Action, d.ItemUID, d.DecisionID, d.Advised, matched, string(ev.Data), at); err != nil {
		return err
	}
	if d.DecisionID == "" || d.ClientSeq == nil {
		return nil
	}
	// The action names the customer its decision was about; fill it in where the decision
	// did not carry one.
	for _, table := range []string{"decisions", "outcomes"} {
		if _, err := tx.ExecContext(ctx, `UPDATE `+table+` SET client_seq = ? WHERE session_id = ? AND decision_id = ? AND client_seq IS NULL`,
			*d.ClientSeq, session, d.DecisionID); err != nil {
			return err
		}
	}
	return nil
}

func adviceMatched(ctx context.Context, tx *sql.Tx, session string, d humanAction) (*bool, error) {
	if d.DecisionID == "" {
		return AdviceMatched(d.Action, d.Advised, nil), nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, `SELECT options FROM decisions WHERE session_id = ? AND decision_id = ?`, session, d.DecisionID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return AdviceMatched(d.Action, d.Advised, nil), nil
	}
	if err != nil {
		return nil, err
	}
	var opts []protocol.Option
	json.Unmarshal([]byte(raw), &opts)
	ids := make([]string, 0, len(opts))
	for _, o := range opts {
		ids = append(ids, o.ID)
	}
	return AdviceMatched(d.Action, d.Advised, ids), nil
}

// AdviceMatched is nil when no advice was in force or the action is not a choice (staging
// an item, say); otherwise whether the human did what the bridge advised. options are the
// ids the decision offered, nil when the decision is unknown.
func AdviceMatched(action, advised string, options []string) *bool {
	if advised == "" || advised == protocol.OptionDefer || action == "" {
		return nil
	}
	yes, no := true, false
	switch {
	case action == advised:
		return &yes
	case options != nil && slices.Contains(options, action):
		return &no
	case options == nil && IsBarterChoice(action):
		return &no
	}
	return nil
}

// IsBarterChoice reports whether id is a barter option id (docs/PROTOCOL.md, "Barter
// option ids") rather than a staging or display move.
func IsBarterChoice(id string) bool {
	switch id {
	case "sell_all", "buy_all", "accept_wholesale", "dismiss":
		return true
	}
	_, _, ok := parseMove(id)
	return ok
}

// ClientSeqFromSignature reads the client seq from a state_signature
// ("c<client seq>:…"). The signature is opaque to the protocol, so this is best effort:
// ok is false for any other shape.
func ClientSeqFromSignature(sig string) (int64, bool) {
	rest, found := strings.CutPrefix(sig, "c")
	if !found {
		return 0, false
	}
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		rest = rest[:i]
	}
	n, err := strconv.ParseInt(rest, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func roundPtr(f *float64) *int64 {
	if f == nil {
		return nil
	}
	v := round(*f)
	return &v
}

func firstSet(vs ...*float64) *float64 {
	for _, v := range vs {
		if v != nil {
			return v
		}
	}
	return nil
}
