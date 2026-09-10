package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
	_ "modernc.org/sqlite/vec"
)

func OpenDB(path string) (*sql.DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

func Migrate(db *sql.DB) error {
	data, err := os.ReadFile("internal/db/migrations.sql")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	if _, err := db.Exec(string(data)); err != nil {
		return fmt.Errorf("execute migrations: %w", err)
	}

	return nil
}