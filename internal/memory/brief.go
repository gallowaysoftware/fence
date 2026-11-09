package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

// briefTimeout caps Brief even when the decision deadline is far off: recall is an aid,
// not worth a slow answer.
const briefTimeout = 250 * time.Millisecond

// The price summaries read only the latest 200 deals per item and side, so they stay fast
// and follow the current economy.
const (
	qLessons = `SELECT text FROM lessons WHERE run_id = ? ORDER BY version DESC LIMIT 1`
	qPrices  = `SELECT count(*), avg(price), min(price), max(price),
		avg(CASE WHEN estimated_value > 0 THEN price * 100.0 / estimated_value END),
		min(CASE WHEN estimated_value > 0 THEN price * 100.0 / estimated_value END),
		max(CASE WHEN estimated_value > 0 THEN price * 100.0 / estimated_value END)
		FROM (SELECT price, estimated_value FROM prices WHERE item_id = ? AND side = ? ORDER BY id DESC LIMIT 200)`
	qPricesCondition = `SELECT count(*), avg(CASE WHEN estimated_value > 0 THEN price * 100.0 / estimated_value END), avg(price)
		FROM (SELECT price, estimated_value FROM prices WHERE item_id = ? AND side = ? AND condition = ? ORDER BY id DESC LIMIT 200)`
	qLastPrice   = `SELECT price, estimated_value, game_day, faction FROM prices WHERE item_id = ? AND side = ? ORDER BY id DESC LIMIT 1`
	qOddsBand    = `SELECT accepted, stayed, walked FROM bargain_stats WHERE move = ? AND pct = ? AND faction = ? AND rep_band = ?`
	qOddsFaction = `SELECT coalesce(sum(accepted), 0), coalesce(sum(stayed), 0), coalesce(sum(walked), 0) FROM bargain_stats WHERE move = ? AND pct = ? AND faction = ?`
	qClient      = `SELECT game_day, summary FROM interactions WHERE run_id = ? AND client_name = ? AND client_faction = ? ORDER BY id DESC LIMIT 6`
	qDay         = `SELECT decisions, sold, bought, cash_delta, walkouts FROM day_stats WHERE run_id = ? AND game_day = ?`
	qDayRep      = `SELECT faction, delta FROM day_rep WHERE run_id = ? AND game_day = ? ORDER BY faction`
)

// minOddsSamples is the least number of tries in the client's exact rep band before the
// faction-wide numbers stop being shown instead.
const minOddsSamples = 3

type section struct {
	header string
	lines  []string
}

// Brief implements Recall. It never fails: a slow or broken database yields less text.
func (s *Store) Brief(ctx context.Context, req *protocol.DecideRequest, maxBytes int) string {
	if s == nil || req == nil || maxBytes <= 0 {
		return ""
	}
	req = req.PolicyView()
	ctx, cancel := context.WithTimeout(ctx, briefTimeout)
	defer cancel()

	runID, carriedFrom, ok := s.runs.peek(req.SessionID, req.GameDay)
	q := querier{ctx: ctx, db: s.db}
	var facts []section
	var c barterContext
	if req.Kind == "barter" {
		c, _ = parseBarter(req.Context)
		facts = append(facts, q.prices(c), q.odds(c, req.Options))
	}
	if ok {
		facts = append(facts, q.client(runID, c), q.today(runID, req.GameDay))
	}
	var lessons string
	if ok {
		lessons = q.lessons(runID)
	}
	if lessons == "" && carriedFrom != 0 {
		lessons = q.lessons(carriedFrom)
	}
	return assemble(facts, lessons, maxBytes)
}

// assemble lays out facts in priority order then the lessons, within maxBytes. The lessons
// are guaranteed half the budget when there are facts to share it with.
func assemble(facts []section, lessons string, maxBytes int) string {
	const lessonsHeader = "Lessons from earlier days:\n"
	var lessonsLen int
	if lessons != "" {
		lessonsLen = len(lessonsHeader) + len(lessons) + 2
	}
	reserve := min(lessonsLen, maxBytes/2)
	var b strings.Builder
	budget := maxBytes - reserve
	for _, sec := range facts {
		if len(sec.lines) == 0 {
			continue
		}
		var part strings.Builder
		for _, l := range sec.lines {
			line := "- " + l + "\n"
			head := 0
			if part.Len() == 0 {
				head = len(sec.header) + 1 + sepLen(b.Len())
			}
			if b.Len()+part.Len()+head+len(line) > budget {
				break
			}
			if part.Len() == 0 {
				if b.Len() > 0 {
					part.WriteByte('\n')
				}
				part.WriteString(sec.header + "\n")
			}
			part.WriteString(line)
		}
		b.WriteString(part.String())
	}
	if lessons != "" {
		left := maxBytes - b.Len() - sepLen(b.Len()) - len(lessonsHeader) - 1
		if text := truncateLines(lessons, left); text != "" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(lessonsHeader + text + "\n")
		}
	}
	out := b.String()
	if len(out) > maxBytes {
		out = truncateLines(out, maxBytes)
	}
	return out
}

func sepLen(n int) int {
	if n > 0 {
		return 1
	}
	return 0
}

// truncateLines cuts s to at most n bytes, at a line end when there is one, never inside
// a UTF-8 sequence, without a trailing newline.
func truncateLines(s string, n int) string {
	s = strings.TrimRight(s, "\n")
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		return cut[:i]
	}
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut
}

type querier struct {
	ctx context.Context
	db  *sql.DB
}

func (q querier) lessons(runID int64) string {
	var text string
	if err := q.db.QueryRowContext(q.ctx, qLessons, runID).Scan(&text); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

func (q querier) prices(c barterContext) section {
	sec := section{header: "Past deals for the items on the counter (price as % of the estimated value shown then):"}
	seen := map[string]bool{}
	for _, it := range c.items() {
		if it.ID == "" || seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		for _, side := range []string{"sold", "bought"} {
			if l := q.priceLine(it, side); l != "" {
				sec.lines = append(sec.lines, l)
			}
		}
	}
	return sec
}

func (q querier) priceLine(it item, side string) string {
	var n int
	var avg, lo, hi, avgPct, loPct, hiPct sql.NullFloat64
	if err := q.db.QueryRowContext(q.ctx, qPrices, it.ID, side).Scan(&n, &avg, &lo, &hi, &avgPct, &loPct, &hiPct); err != nil || n == 0 {
		return ""
	}
	verb := "we sold"
	if side == "bought" {
		verb = "we bought"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s (%s): %s %dx for %s", it.Name, it.ID, verb, n, span(lo.Float64, hi.Float64, avg.Float64, ""))
	if avgPct.Valid {
		fmt.Fprintf(&b, ", %s of est", span(loPct.Float64, hiPct.Float64, avgPct.Float64, "%"))
	}
	var last, est int64
	var day int
	var faction string
	if q.db.QueryRowContext(q.ctx, qLastPrice, it.ID, side).Scan(&last, &est, &day, &faction) == nil {
		fmt.Fprintf(&b, "; last %d (est %d) on day %d, %s client", last, est, day, faction)
	}
	if cond := it.condition(); cond != "" {
		var cn int
		var cPct, cAvg sql.NullFloat64
		if q.db.QueryRowContext(q.ctx, qPricesCondition, it.ID, side, cond).Scan(&cn, &cPct, &cAvg) == nil && cn > 0 && cn < n {
			fmt.Fprintf(&b, "; %s only: %dx, avg %d", cond, cn, round(cAvg.Float64))
			if cPct.Valid {
				fmt.Fprintf(&b, " (%d%% of est)", round(cPct.Float64))
			}
		}
	}
	return b.String()
}

func span(lo, hi, avg float64, unit string) string {
	if round(lo) == round(hi) {
		return fmt.Sprintf("%d%s", round(lo), unit)
	}
	return fmt.Sprintf("%d-%d%s (avg %d%s)", round(lo), round(hi), unit, round(avg), unit)
}

func (q querier) odds(c barterContext, opts []protocol.Option) section {
	faction := nonEmpty(c.Client.Faction, "unknown")
	band := c.repBand()
	sec := section{header: fmt.Sprintf("Bargain results seen with %s clients (rep band %s):", faction, band)}
	for _, o := range opts {
		move, pct, ok := parseMove(o.ID)
		if !ok {
			continue
		}
		var a, st, w int
		scope := ""
		err := q.db.QueryRowContext(q.ctx, qOddsBand, move, pct, faction, band).Scan(&a, &st, &w)
		if err != nil || a+st+w < minOddsSamples {
			if q.db.QueryRowContext(q.ctx, qOddsFaction, move, pct, faction).Scan(&a, &st, &w) != nil {
				continue
			}
			scope = " (all rep bands)"
		}
		if n := a + st + w; n > 0 {
			sec.lines = append(sec.lines, fmt.Sprintf("%s: %d tries, %d accepted, %d refused and stayed, %d walked out%s", o.ID, n, a, st, w, scope))
		}
	}
	return sec
}

func (q querier) client(runID int64, c barterContext) section {
	if c.Client.Name == "" && c.Client.Faction == "" {
		return section{}
	}
	sec := section{header: fmt.Sprintf("Earlier this run with %q (%s):", c.Client.Name, nonEmpty(c.Client.Faction, "unknown"))}
	rows, err := q.db.QueryContext(q.ctx, qClient, runID, c.Client.Name, c.Client.Faction)
	if err != nil {
		return sec
	}
	defer rows.Close()
	for rows.Next() {
		var day int
		var summary string
		if rows.Scan(&day, &summary) == nil {
			sec.lines = append(sec.lines, fmt.Sprintf("day %d: %s", day, summary))
		}
	}
	return sec
}

func (q querier) today(runID int64, day int) section {
	t, ok, _ := q.dayTotals(runID, day)
	if !ok {
		return section{}
	}
	line := fmt.Sprintf("%d decisions, %d sales, %d purchases, cash %+d from trades, %d walk-outs", t.Decisions, t.Sales, t.Purchases, t.CashDelta, t.Walkouts)
	var reps []string
	for _, f := range sortedKeys(t.RepDelta) {
		reps = append(reps, fmt.Sprintf("%s %+.2f", f, t.RepDelta[f]))
	}
	if len(reps) > 0 {
		line += "; rep " + strings.Join(reps, ", ")
	}
	return section{header: fmt.Sprintf("Today so far (day %d):", day), lines: []string{line}}
}

// dayTotals reads day_stats and day_rep for (runID, day); ok is false when the day has no
// stats row. Zero reputation changes are left out. On a day_rep error the totals are kept.
func (q querier) dayTotals(runID int64, day int) (t protocol.DayStats, ok bool, err error) {
	t.RepDelta = map[string]float64{}
	err = q.db.QueryRowContext(q.ctx, qDay, runID, day).Scan(&t.Decisions, &t.Sales, &t.Purchases, &t.CashDelta, &t.Walkouts)
	if errors.Is(err, sql.ErrNoRows) {
		return t, false, nil
	}
	if err != nil {
		return t, false, err
	}
	rows, err := q.db.QueryContext(q.ctx, qDayRep, runID, day)
	if err != nil {
		return t, true, err
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		var d float64
		if err := rows.Scan(&f, &d); err != nil {
			return t, true, err
		}
		if d != 0 {
			t.RepDelta[f] = d
		}
	}
	return t, true, rows.Err()
}
