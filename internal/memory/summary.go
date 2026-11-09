package memory

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"strings"

	"github.com/gallowaysoftware/fence/internal/protocol"
)

const (
	qSessionRun    = `SELECT r.id, r.last_day, r.carried_from FROM sessions s JOIN runs r ON r.id = s.run_id WHERE s.id = ?`
	qLessonsAndDay = `SELECT text, game_day FROM lessons WHERE run_id = ? ORDER BY version DESC LIMIT 1`
)

// Summary implements server.Summarizer: the session's current day totals and the run's
// lessons. Records are written asynchronously, so one sent a moment earlier may not be
// counted yet. On error it returns what it read so far.
func (s *Store) Summary(ctx context.Context, session string) (protocol.SummaryResponse, error) {
	out := protocol.SummaryResponse{Today: protocol.DayStats{RepDelta: map[string]float64{}}}
	q := querier{ctx: ctx, db: s.db}
	r, day, ok := s.runs.current(session)
	if !ok {
		// The tracker forgets sessions beyond maxSessions; the database does not.
		err := s.db.QueryRowContext(ctx, qSessionRun, session).Scan(&r.ID, &r.LastDay, &r.CarriedFrom)
		if errors.Is(err, sql.ErrNoRows) {
			return out, nil
		}
		if err != nil {
			return out, err
		}
		day = r.LastDay
	}
	out.Day = max(day, 0)

	t, _, err := q.dayTotals(r.ID, day)
	for f, d := range t.RepDelta {
		// Deltas are summed floats; two decimals is what the game shows.
		if d = math.Round(d*100) / 100; d == 0 {
			delete(t.RepDelta, f)
		} else {
			t.RepDelta[f] = d
		}
	}
	out.Today = t
	if err != nil {
		return out, err
	}

	for _, id := range []int64{r.ID, r.CarriedFrom} {
		if id == 0 {
			continue
		}
		var text string
		var lday int
		err := s.db.QueryRowContext(ctx, qLessonsAndDay, id).Scan(&text, &lday)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return out, err
		}
		out.Lessons, out.LessonsDay = truncateLines(strings.TrimSpace(text), MaxLessonsBytes), max(lday, 0)
		break
	}
	return out, nil
}
