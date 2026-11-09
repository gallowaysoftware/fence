package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
)

// RunIndex says which run each session's records belong to, as memory inferred it.
type RunIndex struct {
	// Days maps session → game day → run.
	Days map[string]map[int]int64
	// Sessions maps session → the run it was last bound to.
	Sessions map[string]int64
}

// Run is the run a record of session on day belongs to; ok is false when memory never
// saw the session bound to one.
func (x RunIndex) Run(session string, day int) (int64, bool) {
	if id, ok := x.Days[session][day]; ok && id > 0 {
		return id, true
	}
	id, ok := x.Sessions[session]
	return id, ok && id > 0
}

// ReadRunIndex reads the run index from the database at path, read-only, without
// migrating it. A missing database is an empty index.
func ReadRunIndex(ctx context.Context, path string) (RunIndex, error) {
	x := RunIndex{Days: map[string]map[int]int64{}, Sessions: map[string]int64{}}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return x, nil
	}
	db, err := sql.Open("sqlite", "file:"+(&url.URL{Path: path}).EscapedPath()+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return x, err
	}
	defer db.Close()
	var v int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return x, fmt.Errorf("memory db %s: %w", path, err)
	}
	if v == 0 {
		return x, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, run_id FROM sessions`)
	if err != nil {
		return x, err
	}
	for rows.Next() {
		var s string
		var id int64
		if err := rows.Scan(&s, &id); err != nil {
			rows.Close()
			return x, err
		}
		x.Sessions[s] = id
	}
	rows.Close()
	// A new game on the same day number as the old run's last day lands in the later run.
	rows, err = db.QueryContext(ctx, `SELECT session_id, game_day, max(run_id) FROM (
		SELECT session_id, game_day, run_id FROM events
		UNION ALL SELECT session_id, game_day, run_id FROM decisions
		UNION ALL SELECT session_id, game_day, run_id FROM outcomes)
		WHERE game_day > 0 GROUP BY session_id, game_day`)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var day int
		var id int64
		if err := rows.Scan(&s, &day, &id); err != nil {
			return x, err
		}
		if x.Days[s] == nil {
			x.Days[s] = map[int]int64{}
		}
		x.Days[s][day] = id
	}
	return x, rows.Err()
}
