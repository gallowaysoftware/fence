package memory

import (
	"context"
	"database/sql"
	"fmt"
)

// migrations[i] takes the schema from user_version i to i+1. Append only; never edit a
// migration that has shipped.
var migrations = []string{
	`
CREATE TABLE runs (
	id           INTEGER PRIMARY KEY,
	created_at   TEXT NOT NULL,
	reason       TEXT NOT NULL,
	scenario     TEXT NOT NULL DEFAULT '',
	first_day    INTEGER NOT NULL,
	last_day     INTEGER NOT NULL,
	carried_from INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE sessions (
	id           TEXT PRIMARY KEY,
	run_id       INTEGER NOT NULL DEFAULT 0,
	created_at   TEXT NOT NULL,
	mod_version  TEXT NOT NULL DEFAULT '',
	game_version TEXT NOT NULL DEFAULT ''
);
CREATE TABLE decisions (
	id             INTEGER PRIMARY KEY,
	session_id     TEXT NOT NULL,
	decision_id    TEXT NOT NULL,
	run_id         INTEGER NOT NULL,
	game_day       INTEGER NOT NULL,
	kind           TEXT NOT NULL,
	client_name    TEXT NOT NULL DEFAULT '',
	client_faction TEXT NOT NULL DEFAULT '',
	client_intent  TEXT NOT NULL DEFAULT '',
	context        TEXT NOT NULL,
	options        TEXT NOT NULL,
	choice         TEXT NOT NULL,
	params         TEXT NOT NULL DEFAULT '',
	source         TEXT NOT NULL,
	rationale      TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL,
	UNIQUE (session_id, decision_id)
);
CREATE INDEX decisions_run_day ON decisions (run_id, game_day, id);
CREATE TABLE events (
	id         INTEGER PRIMARY KEY,
	session_id TEXT NOT NULL,
	run_id     INTEGER NOT NULL,
	seq        INTEGER NOT NULL,
	kind       TEXT NOT NULL,
	game_day   INTEGER NOT NULL,
	data       TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL
);
CREATE INDEX events_run_day ON events (run_id, game_day);
CREATE TABLE outcomes (
	id          INTEGER PRIMARY KEY,
	session_id  TEXT NOT NULL,
	decision_id TEXT NOT NULL,
	run_id      INTEGER NOT NULL,
	game_day    INTEGER NOT NULL,
	kind        TEXT NOT NULL,
	result      TEXT NOT NULL,
	created_at  TEXT NOT NULL
);
CREATE INDEX outcomes_decision ON outcomes (session_id, decision_id, id);
CREATE TABLE prices (
	id              INTEGER PRIMARY KEY,
	item_id         TEXT NOT NULL,
	item_name       TEXT NOT NULL,
	condition       TEXT NOT NULL,
	side            TEXT NOT NULL,
	price           INTEGER NOT NULL,
	estimated_value INTEGER NOT NULL,
	faction         TEXT NOT NULL,
	run_id          INTEGER NOT NULL,
	game_day        INTEGER NOT NULL,
	created_at      TEXT NOT NULL
);
CREATE INDEX prices_item ON prices (item_id, side, id);
CREATE INDEX prices_item_condition ON prices (item_id, side, condition, id);
CREATE TABLE bargain_stats (
	move     TEXT NOT NULL,
	pct      INTEGER NOT NULL,
	faction  TEXT NOT NULL,
	rep_band TEXT NOT NULL,
	accepted INTEGER NOT NULL DEFAULT 0,
	stayed   INTEGER NOT NULL DEFAULT 0,
	walked   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (move, pct, faction, rep_band)
) WITHOUT ROWID;
CREATE TABLE interactions (
	id             INTEGER PRIMARY KEY,
	run_id         INTEGER NOT NULL,
	client_name    TEXT NOT NULL,
	client_faction TEXT NOT NULL,
	game_day       INTEGER NOT NULL,
	decision_id    TEXT NOT NULL,
	summary        TEXT NOT NULL
);
CREATE INDEX interactions_client ON interactions (run_id, client_name, client_faction, id);
CREATE TABLE day_stats (
	run_id     INTEGER NOT NULL,
	game_day   INTEGER NOT NULL,
	decisions  INTEGER NOT NULL DEFAULT 0,
	sold       INTEGER NOT NULL DEFAULT 0,
	bought     INTEGER NOT NULL DEFAULT 0,
	cash_delta INTEGER NOT NULL DEFAULT 0,
	walkouts   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (run_id, game_day)
) WITHOUT ROWID;
CREATE TABLE day_rep (
	run_id   INTEGER NOT NULL,
	game_day INTEGER NOT NULL,
	faction  TEXT NOT NULL,
	delta    REAL NOT NULL DEFAULT 0,
	PRIMARY KEY (run_id, game_day, faction)
) WITHOUT ROWID;
CREATE TABLE lessons (
	run_id     INTEGER NOT NULL,
	version    INTEGER NOT NULL,
	game_day   INTEGER NOT NULL,
	text       TEXT NOT NULL,
	source     TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (run_id, version)
) WITHOUT ROWID;
`,
	// Phase D recording: day snapshots, every observed customer and the human's moves, all
	// keyed by run/session/day/client_seq so decisions, outcomes and actions join to the
	// customer they were about. client_seq is NULL where it is unknown.
	`
ALTER TABLE decisions ADD COLUMN client_seq INTEGER;
ALTER TABLE outcomes ADD COLUMN client_seq INTEGER;
CREATE INDEX decisions_client ON decisions (session_id, game_day, client_seq);
CREATE INDEX outcomes_client ON outcomes (session_id, game_day, client_seq);
CREATE TABLE day_snapshots (
	id             INTEGER PRIMARY KEY,
	run_id         INTEGER NOT NULL,
	session_id     TEXT NOT NULL,
	seq            INTEGER NOT NULL,
	game_day       INTEGER NOT NULL,
	phase          TEXT NOT NULL DEFAULT '',
	cash           INTEGER,
	rent_due       INTEGER,
	rent_days_left INTEGER,
	data           TEXT NOT NULL,
	created_at     TEXT NOT NULL,
	UNIQUE (session_id, seq)
);
CREATE INDEX day_snapshots_run_day ON day_snapshots (run_id, game_day, id);
CREATE TABLE day_snapshot_rep (
	snapshot_id         INTEGER NOT NULL,
	faction             TEXT NOT NULL,
	value               REAL NOT NULL,
	status              TEXT NOT NULL DEFAULT '',
	next_perk           TEXT NOT NULL DEFAULT '',
	next_perk_threshold REAL,
	PRIMARY KEY (snapshot_id, faction)
) WITHOUT ROWID;
CREATE TABLE customers (
	id             INTEGER PRIMARY KEY,
	run_id         INTEGER NOT NULL,
	session_id     TEXT NOT NULL,
	seq            INTEGER NOT NULL,
	game_day       INTEGER NOT NULL,
	client_seq     INTEGER,
	covered        INTEGER NOT NULL,
	reason         TEXT NOT NULL DEFAULT '',
	client_name    TEXT NOT NULL DEFAULT '',
	client_faction TEXT NOT NULL DEFAULT '',
	client_intent  TEXT NOT NULL DEFAULT '',
	context        TEXT NOT NULL DEFAULT '',
	options        TEXT NOT NULL DEFAULT '',
	created_at     TEXT NOT NULL,
	UNIQUE (session_id, seq)
);
CREATE INDEX customers_client ON customers (session_id, game_day, client_seq);
CREATE INDEX customers_run_day ON customers (run_id, game_day, id);
CREATE TABLE human_actions (
	id          INTEGER PRIMARY KEY,
	run_id      INTEGER NOT NULL,
	session_id  TEXT NOT NULL,
	seq         INTEGER NOT NULL,
	game_day    INTEGER NOT NULL,
	client_seq  INTEGER,
	action      TEXT NOT NULL,
	item_uid    INTEGER,
	decision_id TEXT NOT NULL DEFAULT '',
	advised     TEXT NOT NULL DEFAULT '',
	matched     INTEGER,
	data        TEXT NOT NULL,
	created_at  TEXT NOT NULL,
	UNIQUE (session_id, seq)
);
CREATE INDEX human_actions_client ON human_actions (session_id, game_day, client_seq);
CREATE INDEX human_actions_run_day ON human_actions (run_id, game_day, id);
`,
}

// SchemaVersion is the user_version a fully migrated database has.
var SchemaVersion = len(migrations)

// migrate applies every migration above the database's user_version, each in its own
// transaction, so running it again is a no-op.
func migrate(ctx context.Context, db *sql.DB) error {
	var v int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&v); err != nil {
		return err
	}
	if v > len(migrations) {
		return fmt.Errorf("memory db schema version %d is newer than this bridge (%d)", v, len(migrations))
	}
	for i := v; i < len(migrations); i++ {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
