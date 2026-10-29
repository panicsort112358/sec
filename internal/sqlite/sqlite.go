// Package sqlite opens the application's SQLite database.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("provide a SQLite database path")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable SQLite foreign keys: %w", err)
	}
	return db, nil
}

func DefaultDirectory() string {
	if dir := strings.TrimSpace(os.Getenv("SEC_DATABASE_DIR")); dir != "" {
		return dir
	}
	return "data"
}
