package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

func recordDecision(ctx context.Context, tx *sql.Tx, session string, runID int64, d decisionOp, at string) error {
	req := d.req
	var name, faction, intent string
	if req.Kind == "barter" {
		if c, ok := parseBarter(req.Context); ok {
			name, faction, intent = c.Client.Name, c.Client.Faction, c.Client.Intent
		}
	}
	opts, err := json.Marshal(req.Options)
	if err != nil {
		return err
	}
	var params string
	if len(d.resp.Choice.Params) > 0 {
		b, err := json.Marshal(d.resp.Choice.Params)
		if err != nil {
			return err
		}
		params = string(b)
	}
	var clientSeq *int64
	if n, ok := ClientSeqFromSignature(req.StateSignature); ok {
		clientSeq = &n
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO decisions
		(session_id, decision_id, run_id, game_day, kind, client_name, client_faction, client_intent, context, options, choice, params, source, rationale, created_at, client_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (session_id, decision_id) DO NOTHING`,
		session, req.DecisionID, runID, req.GameDay, req.Kind, name, faction, intent, string(req.Context), string(opts),
		d.resp.Choice.Option, params, string(d.resp.Source), d.resp.Rationale, at, clientSeq)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO day_stats (run_id, game_day, decisions) VALUES (?, ?, 1)
		ON CONFLICT (run_id, game_day) DO UPDATE SET decisions = decisions + 1`, runID, req.GameDay)
	return err
}

func recordOutcome(ctx context.Context, tx *sql.Tx, session string, runID int64, oc protocol.Outcome, at string) error {
	var r outcomeResult
	if err := json.Unmarshal(oc.Result, &r); err != nil {
		r = outcomeResult{}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO outcomes (session_id, decision_id, run_id, game_day, kind, result, created_at, client_seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, (SELECT client_seq FROM decisions WHERE session_id = ? AND decision_id = ?))`,
		session, oc.DecisionID, runID, oc.GameDay, r.Kind, string(oc.Result), at, session, oc.DecisionID); err != nil {
		return err
	}
	var kind, rawCtx string
	err := tx.QueryRowContext(ctx, `SELECT kind, context FROM decisions WHERE session_id = ? AND decision_id = ?`, session, oc.DecisionID).Scan(&kind, &rawCtx)
	if errors.Is(err, sql.ErrNoRows) || kind != "barter" {
		return nil
	}
	if err != nil {
		return err
	}
	c, ok := parseBarter(json.RawMessage(rawCtx))
	if !ok {
		return nil
	}
	l := ledger{ctx: ctx, tx: tx, run: runID, day: oc.GameDay, at: at}
	faction := c.Client.Faction
	if faction == "" {
		faction = "unknown"
	}
	var summary string
	switch r.Kind {
	case "bargain":
		summary, err = l.bargain(c, faction, r)
	case "trade":
		summary, err = l.trade(c, faction, r)
	case "left":
		summary = "left (" + nonEmpty(r.Cause, "unknown cause") + ")"
		if r.Cause == "walked_out" {
			err = l.exec(`INSERT INTO day_stats (run_id, game_day, walkouts) VALUES (?, ?, 1)
				ON CONFLICT (run_id, game_day) DO UPDATE SET walkouts = walkouts + 1`, runID, oc.GameDay)
		}
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if err := l.repDelta(r.RepDelta); err != nil {
		return err
	}
	if c.Client.Name == "" && c.Client.Faction == "" {
		return nil
	}
	return l.exec(`INSERT INTO interactions (run_id, client_name, client_faction, game_day, decision_id, summary) VALUES (?, ?, ?, ?, ?, ?)`,
		runID, c.Client.Name, c.Client.Faction, oc.GameDay, oc.DecisionID, summary)
}

type ledger struct {
	ctx context.Context
	tx  *sql.Tx
	run int64
	day int
	at  string
}

func (l ledger) exec(q string, args ...any) error {
	_, err := l.tx.ExecContext(l.ctx, q, args...)
	return err
}

func (l ledger) bargain(c barterContext, faction string, r outcomeResult) (string, error) {
	move, pct, ok := parseMove(r.Option)
	var verdict, col string
	switch {
	case r.Accepted:
		verdict, col = "accepted", "accepted"
	case r.WalkedOut:
		verdict, col = "walked out", "walked"
	default:
		verdict, col = "refused, stayed", "stayed"
	}
	summary := r.Option + ": " + verdict
	if !ok {
		return summary, nil
	}
	err := l.exec(`INSERT INTO bargain_stats (move, pct, faction, rep_band, `+col+`) VALUES (?, ?, ?, ?, 1)
		ON CONFLICT (move, pct, faction, rep_band) DO UPDATE SET `+col+` = `+col+` + 1`,
		move, pct, faction, c.repBand())
	return summary, err
}

func (l ledger) trade(c barterContext, faction string, r outcomeResult) (string, error) {
	var pool []item
	switch r.Side {
	case "sold":
		pool = c.Counter.YoursStaged
	case "bought":
		pool = c.Counter.Theirs
	default:
		return "", nil
	}
	var traded []item
	var listed float64
	for _, uid := range r.Items {
		if i := slices.IndexFunc(pool, func(it item) bool { return it.UID == uid }); i >= 0 {
			traded = append(traded, pool[i])
			listed += pool[i].Price
		}
	}
	cash := math.Abs(r.CashDelta)
	col := r.Side
	if err := l.exec(`INSERT INTO day_stats (run_id, game_day, `+col+`, cash_delta) VALUES (?, ?, 1, ?)
		ON CONFLICT (run_id, game_day) DO UPDATE SET `+col+` = `+col+` + 1, cash_delta = cash_delta + excluded.cash_delta`,
		l.run, l.day, round(r.CashDelta)); err != nil {
		return "", err
	}
	var names []string
	for _, it := range traded {
		names = append(names, it.Name)
		// The cash actually exchanged is split over the items by their listed prices, so a
		// capped sale or a late bargain is reflected in the ledger.
		price := it.Price
		if listed > 0 && cash > 0 {
			price = cash * it.Price / listed
		}
		if err := l.exec(`INSERT INTO prices (item_id, item_name, condition, side, price, estimated_value, faction, run_id, game_day, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			it.ID, it.Name, it.condition(), r.Side, round(price), round(it.EstimatedValue), faction, l.run, l.day, l.at); err != nil {
			return "", err
		}
	}
	verb := "we sold"
	if r.Side == "bought" {
		verb = "we bought"
	}
	if len(names) == 0 {
		names = []string{fmt.Sprintf("%d items", len(r.Items))}
	}
	return fmt.Sprintf("%s %s for %d", verb, strings.Join(names, ", "), round(cash)), nil
}

func (l ledger) repDelta(d map[string]float64) error {
	for _, f := range sortedKeys(d) {
		if err := l.exec(`INSERT INTO day_rep (run_id, game_day, faction, delta) VALUES (?, ?, ?, ?)
			ON CONFLICT (run_id, game_day, faction) DO UPDATE SET delta = delta + excluded.delta`,
			l.run, l.day, f, d[f]); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
