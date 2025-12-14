package db

import (
	"database/sql"
	"embed"
	"fmt"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var embedMigrations embed.FS

type DB struct {
	*sql.DB
}

func Open(dbPath string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	if _, err := sqlDB.Exec(`PRAGMA journal_mode = WAL;`); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}

	return &DB{DB: sqlDB}, nil
}

func (db *DB) Migrate() error {
	goose.SetBaseFS(embedMigrations)

	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("setting dialect: %w", err)
	}

	if err := goose.Up(db.DB, "migrations"); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	return nil
}

// GetHighWaterMark returns the Unix timestamp of the most recently processed event.
// Returns 0 if no events have been processed yet.
func (db *DB) GetHighWaterMark() (int64, error) {
	var ts int64
	err := db.QueryRow(`SELECT last_event_at FROM high_water_mark WHERE id = 1`).Scan(&ts)
	if err != nil {
		return 0, fmt.Errorf("getting high water mark: %w", err)
	}
	return ts, nil
}

// SetHighWaterMark updates the high water mark if the given timestamp is greater
// than the current value. This ensures we only move forward in time.
func (db *DB) SetHighWaterMark(ts int64) error {
	_, err := db.Exec(`
		UPDATE high_water_mark
		SET last_event_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1 AND last_event_at < ?
	`, ts, ts)
	if err != nil {
		return fmt.Errorf("setting high water mark: %w", err)
	}
	return nil
}
