// Package store wraps the SQLite transfer_log.db.
// See PRD §5.2.3 and §5.3.
//
// Tables (P1 W7 schema):
//
//	shares(id TEXT PK, kind TEXT, target TEXT, file_path TEXT, size INT,
//	       status TEXT, created_at INT, expires_at INT, downloads INT,
//	       max_downloads INT, password_hash TEXT, password_salt TEXT)
//	transfers(id TEXT PK, direction TEXT, peer_device_id TEXT, peer_name TEXT,
//	          file_path TEXT, size INT, status TEXT, started_at INT,
//	          finished_at INT, error TEXT)
//
// MVP P0 ships an open/close + schema bootstrap stub; real queries land P1 W7.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the *sql.DB for transfer_log.db.
type DB struct {
	*sql.DB
}

// Open creates or opens transfer_log.db in dir, running schema migrations.
func Open(dir string) (*DB, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}
	dsn := filepath.Join(dir, "transfer_log.db")
	// _pragma keys tune modernc.org/sqlite for our workload.
	dsn = "file:" + dsn + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{db}, nil
}

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS shares (
  id            TEXT PRIMARY KEY,
  kind          TEXT NOT NULL,
  target        TEXT,
  file_path     TEXT NOT NULL,
  size          INTEGER NOT NULL DEFAULT 0,
  status        TEXT NOT NULL,
  created_at    INTEGER NOT NULL,
  expires_at    INTEGER,
  downloads     INTEGER NOT NULL DEFAULT 0,
  max_downloads INTEGER,
  password_hash TEXT,
  password_salt TEXT
);

CREATE TABLE IF NOT EXISTS transfers (
  id              TEXT PRIMARY KEY,
  direction       TEXT NOT NULL,
  peer_device_id  TEXT NOT NULL,
  peer_name       TEXT,
  file_path       TEXT NOT NULL,
  size            INTEGER NOT NULL DEFAULT 0,
  status          TEXT NOT NULL,
  started_at      INTEGER NOT NULL,
  finished_at     INTEGER,
  error           TEXT
);

CREATE INDEX IF NOT EXISTS idx_shares_created ON shares(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transfers_started ON transfers(started_at DESC);
`
	_, err := db.Exec(schema)
	return err
}
