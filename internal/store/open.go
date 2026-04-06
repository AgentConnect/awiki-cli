package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	appconfig "github.com/agentconnect/awiki-cli/internal/config"
	_ "modernc.org/sqlite"
)

const sqliteDriverName = "sqlite"

func Open(paths appconfig.Paths) (*sql.DB, error) {
	return openDatabase(paths.DatabaseFile, OpenOptions{})
}

func OpenReadOnly(path string) (*sql.DB, error) {
	return openDatabase(path, OpenOptions{ReadOnly: true})
}

func openDatabase(path string, options OpenOptions) (*sql.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	if !options.ReadOnly {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create sqlite dir: %w", err)
		}
	}
	dsn := path
	if options.ReadOnly {
		dsn = fmt.Sprintf("file:%s?mode=ro", path)
	}
	db, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	if !options.ReadOnly {
		if err := configureDatabase(ctx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return db, nil
}

func configureDatabase(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA foreign_keys=ON;",
		"PRAGMA busy_timeout=5000;",
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure sqlite (%s): %w", pragma, err)
		}
	}
	return nil
}
