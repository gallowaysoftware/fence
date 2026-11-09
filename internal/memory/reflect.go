package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gallowaysoftware/fence/internal/llm"
)

const (
	// MaxLessonsBytes bounds the lessons file; it rides along in every prompt.
	MaxLessonsBytes = 2048
	maxDayLogBytes  = 24 << 10
)

type reflectJob struct {
	run, carriedFrom int64
	day              int
}

const reflectSystem = `You keep the lessons file of an AI that runs a pawnshop in the game "Probably Stolen". ` +
	`The file is included in every one of its decision prompts. You are given the current lessons and the log of ` +
	`one in-game day: each decision with what the shopkeeper could see, the option chosen and what happened. ` +
	`Rewrite the lessons as short, general, actionable rules about haggling, pricing, risk, cash and the factions ` +
	`that the results support. Keep rules that still hold, correct the ones the day contradicted, drop stale or ` +
	`vague ones. Do not list individual items, prices or customers: a separate price ledger already records those ` +
	`facts. Plain text, one rule per line starting with "- ", at most 2000 bytes in total. ` +
	`Answer with exactly one JSON object: {"lessons": string}.`

var lessonsSchema = json.RawMessage(`{"type":"object","properties":{"lessons":{"type":"string"}},"required":["lessons"],"additionalProperties":false}`)

func (s *Store) queueReflection(j reflectJob) {
	if s.opt.LLM == nil {
		return
	}
	select {
	case s.reflectQ <- j:
	default:
		s.log.Warn("reflection backlog full; skipping day", "run", j.run, "day", j.day)
	}
}

func (s *Store) reflector() {
	defer close(s.reflectDone)
	for j := range s.reflectQ {
		if s.ctx.Err() != nil {
			continue
		}
		if err := s.reflect(j); err != nil {
			s.log.Warn("reflection failed; lessons unchanged", "run", j.run, "day", j.day, "err", err)
		}
	}
}

func (s *Store) reflect(j reflectJob) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.opt.ReflectTimeout)
	defer cancel()
	log, n, err := s.dayLog(ctx, j.run, j.day)
	if err != nil || n == 0 {
		return err
	}
	q := querier{ctx: ctx, db: s.db}
	prev := q.lessons(j.run)
	if prev == "" && j.carriedFrom != 0 {
		prev = q.lessons(j.carriedFrom)
	}
	var p strings.Builder
	p.WriteString("Current lessons:\n")
	p.WriteString(nonEmpty(prev, "(none yet)"))
	fmt.Fprintf(&p, "\n\nDay %d log (%d decisions):\n%s", j.day, n, log)
	resp, err := s.opt.LLM.Complete(ctx, llm.Request{
		System:     reflectSystem,
		Messages:   []llm.Message{{Role: llm.RoleUser, Content: p.String()}},
		Schema:     lessonsSchema,
		SchemaName: "lessons",
		MaxTokens:  s.opt.ReflectMaxTokens,
	})
	if err != nil {
		return err
	}
	raw, err := llm.ExtractJSON(resp.Text)
	if err != nil {
		return err
	}
	var a struct {
		Lessons string `json:"lessons"`
	}
	if err := json.Unmarshal(raw, &a); err != nil {
		return err
	}
	text := truncateLines(strings.TrimSpace(a.Lessons), MaxLessonsBytes)
	if text == "" {
		return errors.New("model returned empty lessons")
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO lessons (run_id, version, game_day, text, source, created_at)
		SELECT ?, coalesce(max(version), 0) + 1, ?, ?, ?, ? FROM lessons WHERE run_id = ?`,
		j.run, j.day, text, "reflection "+resp.Model, s.opt.Now().UTC().Format(time.RFC3339Nano), j.run)
	if err == nil {
		s.log.Info("lessons rewritten", "run", j.run, "day", j.day, "bytes", len(text))
	}
	return err
}

// dayLog renders the day's decisions and outcomes, newest kept when over the cap. Every
// value comes from the stored visible context and outcomes.
func (s *Store) dayLog(ctx context.Context, runID int64, day int) (string, int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT session_id, decision_id, kind, context, choice, source, rationale
		FROM decisions WHERE run_id = ? AND game_day = ? ORDER BY id`, runID, day)
	if err != nil {
		return "", 0, err
	}
	type dec struct{ session, id, kind, context, choice, source, rationale string }
	var decs []dec
	for rows.Next() {
		var d dec
		if err := rows.Scan(&d.session, &d.id, &d.kind, &d.context, &d.choice, &d.source, &d.rationale); err != nil {
			rows.Close()
			return "", 0, err
		}
		decs = append(decs, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return "", 0, err
	}
	lines := make([]string, 0, len(decs))
	for i, d := range decs {
		results, err := s.outcomes(ctx, d.session, d.id)
		if err != nil {
			return "", 0, err
		}
		lines = append(lines, fmt.Sprintf("%d. %s Chose %s [%s]%s. Result: %s.",
			i+1, describeSituation(d.kind, d.context), d.choice, d.source, quoteRationale(d.rationale), nonEmpty(strings.Join(results, "; "), "none reported")))
	}
	total := 0
	start := len(lines)
	for start > 0 && total+len(lines[start-1])+1 <= maxDayLogBytes {
		start--
		total += len(lines[start]) + 1
	}
	var b strings.Builder
	if start > 0 {
		fmt.Fprintf(&b, "(%d earlier decisions omitted)\n", start)
	}
	for _, l := range lines[start:] {
		b.WriteString(l + "\n")
	}
	q := querier{ctx: ctx, db: s.db}
	if t := q.today(runID, day); len(t.lines) > 0 {
		b.WriteString("Day totals: " + t.lines[0] + "\n")
	}
	return b.String(), len(decs), nil
}

func (s *Store) outcomes(ctx context.Context, session, decisionID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT result FROM outcomes WHERE session_id = ? AND decision_id = ? ORDER BY id`, session, decisionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, describeOutcome(raw))
	}
	return out, rows.Err()
}

func quoteRationale(r string) string {
	r = strings.TrimSpace(r)
	if r == "" {
		return ""
	}
	if len(r) > 300 {
		r = truncateLines(r, 300) + "…"
	}
	return fmt.Sprintf(" because %q", r)
}

func describeSituation(kind, raw string) string {
	if kind != "barter" {
		return kind + " decision."
	}
	c, ok := parseBarter(json.RawMessage(raw))
	if !ok {
		return "Barter."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s, wants to %s", nonEmpty(c.Client.Name, "client"), nonEmpty(c.Client.Faction, "unknown faction"), nonEmpty(c.Client.Intent, "?"))
	if c.Client.Budget != nil {
		fmt.Fprintf(&b, ", budget %d", round(*c.Client.Budget))
	}
	fmt.Fprintf(&b, ", our rep with them %s).", c.repBand())
	for _, side := range []struct {
		label string
		items []item
	}{{"Ours on the counter", c.Counter.YoursStaged}, {"Theirs", c.Counter.Theirs}} {
		if len(side.items) == 0 {
			continue
		}
		var parts []string
		for _, it := range side.items {
			p := fmt.Sprintf("%s price %d est %d", it.Name, round(it.Price), round(it.EstimatedValue))
			if cond := it.condition(); cond != "" {
				p += " [" + cond + "]"
			}
			if it.BoughtAt != nil {
				p += fmt.Sprintf(" bought at %d", round(*it.BoughtAt))
			}
			if it.StolenHeat != nil {
				p += fmt.Sprintf(" stolen heat %d", round(*it.StolenHeat))
			}
			parts = append(parts, p)
		}
		fmt.Fprintf(&b, " %s: %s.", side.label, strings.Join(parts, "; "))
	}
	return b.String()
}

func describeOutcome(raw string) string {
	var r outcomeResult
	if json.Unmarshal([]byte(raw), &r) != nil {
		return "unreadable outcome"
	}
	var s string
	switch r.Kind {
	case "bargain":
		switch {
		case r.Accepted:
			s = r.Option + " accepted"
		case r.WalkedOut:
			s = r.Option + " refused, client walked out"
		default:
			s = r.Option + " refused, client stayed"
		}
	case "trade":
		s = fmt.Sprintf("%s %d items, cash %+d", nonEmpty(r.Side, "traded"), len(r.Items), round(r.CashDelta))
	case "left":
		s = "client left (" + nonEmpty(r.Cause, "unknown cause") + ")"
	default:
		s = nonEmpty(r.Kind, "unknown") + " outcome"
	}
	var reps []string
	for _, f := range sortedKeys(r.RepDelta) {
		if d := r.RepDelta[f]; d != 0 {
			reps = append(reps, fmt.Sprintf("%s %+.2f", f, d))
		}
	}
	if len(reps) > 0 {
		s += ", rep " + strings.Join(reps, ", ")
	}
	return s
}
