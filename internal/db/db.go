package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS repositories (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    url         TEXT NOT NULL UNIQUE,
    local_path  TEXT NOT NULL,
    created_at  TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS prompt_requests (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    repository_id   INTEGER NOT NULL REFERENCES repositories(id),
    title           TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'deleted')),
    session_id      TEXT NOT NULL,
    issue_number    INTEGER,
    issue_url       TEXT,
    created_at      TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS messages (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    prompt_request_id INTEGER NOT NULL REFERENCES prompt_requests(id),
    role              TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    content           TEXT NOT NULL,
    raw_response      TEXT,
    created_at        TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS revisions (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    prompt_request_id INTEGER NOT NULL REFERENCES prompt_requests(id),
    content           TEXT NOT NULL,
    published_at      TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_prompt_requests_repository ON prompt_requests(repository_id);
CREATE INDEX IF NOT EXISTS idx_prompt_requests_status ON prompt_requests(status);
CREATE INDEX IF NOT EXISTS idx_messages_prompt_request ON messages(prompt_request_id);
CREATE INDEX IF NOT EXISTS idx_revisions_prompt_request ON revisions(prompt_request_id);
`

func DBPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	dir := filepath.Join(home, ".prompter")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating .prompter directory: %w", err)
	}
	return filepath.Join(dir, "prompter.db"), nil
}

func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("running schema migration: %w", err)
	}
	return db, nil
}
