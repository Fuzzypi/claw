package store

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database connection.
type Store struct {
	db *sql.DB
}

// Open opens a SQLite database at the given path and runs migrations.
// If dbPath is empty or ":memory:", an in-memory database is used.
// The CLAW_DB_PATH environment variable overrides the default path.
func Open(dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = defaultDBPath()
	}

	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// SQLite only supports one writer at a time. For in-memory databases,
	// multiple connections each get a separate database. Limit to 1 connection.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func defaultDBPath() string {
	if p := os.Getenv("CLAW_DB_PATH"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "claw.db"
	}
	return filepath.Join(home, ".claw", "claw.db")
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pipelines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			project_dir TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME
		);

		CREATE TABLE IF NOT EXISTS jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline_id INTEGER NOT NULL REFERENCES pipelines(id),
			name TEXT NOT NULL,
			prompt TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			agent_id INTEGER,
			gate_command TEXT,
			gate_status TEXT,
			gate_exit_code INTEGER,
			gate_output TEXT,
			output TEXT,
			exit_code INTEGER,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			lease_expires_at DATETIME,
			started_at DATETIME,
			completed_at DATETIME,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS job_dependencies (
			job_id INTEGER NOT NULL REFERENCES jobs(id),
			depends_on INTEGER NOT NULL REFERENCES jobs(id),
			UNIQUE(job_id, depends_on)
		);

		CREATE TABLE IF NOT EXISTS agents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			command TEXT,
			args TEXT,
			cwd TEXT,
			timeout_secs INTEGER DEFAULT 600,
			status TEXT NOT NULL DEFAULT 'idle',
			current_job_id INTEGER,
			registered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS context_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline_id INTEGER NOT NULL REFERENCES pipelines(id),
			job_id INTEGER NOT NULL REFERENCES jobs(id),
			content TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS activity_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline_id INTEGER,
			job_id INTEGER,
			agent_id INTEGER,
			event TEXT NOT NULL,
			detail TEXT,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}
