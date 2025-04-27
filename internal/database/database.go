package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB handles database operations
type DB struct {
	db *sql.DB
}

// LastRunInfo represents information about the last completed run
type LastRunInfo struct {
	Timestamp time.Time
	CsvPath   string
}

// New creates a new database connection
func New(dbPath string) (*DB, error) {
	// Ensure directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize database
	if err := initDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return &DB{db: db}, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// initDB initializes the database schema
func initDB(db *sql.DB) error {
	// Create last_run table to store only the last run information
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS last_run (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			timestamp DATETIME NOT NULL,
			csv_path TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create last_run table: %w", err)
	}

	return nil
}

// UpdateLastRun updates the information about the last completed run
func (d *DB) UpdateLastRun(timestamp time.Time, csvPath string) error {
	// Use upsert (insert or replace) to ensure we only have one row
	_, err := d.db.Exec(`
		INSERT OR REPLACE INTO last_run (id, timestamp, csv_path)
		VALUES (1, ?, ?)
	`, timestamp, csvPath)

	if err != nil {
		return fmt.Errorf("failed to update last run info: %w", err)
	}

	return nil
}

// GetLastRun retrieves information about the last run
func (d *DB) GetLastRun() (*LastRunInfo, error) {
	row := d.db.QueryRow(`
		SELECT timestamp, csv_path FROM last_run
		WHERE id = 1
	`)

	var info LastRunInfo
	err := row.Scan(&info.Timestamp, &info.CsvPath)
	if err == sql.ErrNoRows {
		return nil, nil // No last run recorded yet
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get last run info: %w", err)
	}

	return &info, nil
}
