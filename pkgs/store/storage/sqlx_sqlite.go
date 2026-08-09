package storage

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmoiron/sqlx"

	_ "modernc.org/sqlite"
)

func newSQLite(name string, cfg SQLiteConfig) (*sqlx.DB, error) {
	if (cfg.DSN == "") == (cfg.Dir == "") {
		return nil, fmt.Errorf("storage: sqlite %q requires exactly one of dsn or dir", name)
	}
	dsn := cfg.DSN
	if dsn == "" {
		dsn = cfg.Dir
	}
	if err := prepareSQLiteDir(name, cfg.Dir); err != nil {
		return nil, err
	}
	if err := validateSQLiteDSN(dsn); err != nil {
		return nil, fmt.Errorf("storage: sql %q sqlite dsn: %w", name, err)
	}
	sqlx.BindDriver(KindSQLite, sqlx.QUESTION)
	return newSQL(name, KindSQLite, KindSQLite, dsn, func(db *sqlx.DB) error {
		configureSQLitePool(db)
		if err := configureSQLiteConnection(db); err != nil {
			return fmt.Errorf("storage: sql %q configure sqlite: %w", name, err)
		}
		return nil
	})
}

func configureSQLitePool(db *sqlx.DB) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
}

func configureSQLiteConnection(db *sqlx.DB) error {
	for _, stmt := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func validateSQLiteDSN(dsn string) error {
	queryStart := strings.IndexRune(dsn, '?')
	if queryStart < 1 {
		return nil
	}
	query, err := url.ParseQuery(dsn[queryStart+1:])
	if err != nil {
		return fmt.Errorf("parse query: %w", err)
	}
	for _, key := range []string{
		"_busy_timeout",
		"_timeout",
		"_foreign_keys",
		"_fk",
		"_journal_mode",
		"_journal",
		"_synchronous",
		"_sync",
		"_auto_vacuum",
		"_vacuum",
		"_query_only",
	} {
		if _, ok := query[key]; ok {
			return fmt.Errorf("query parameter %q is unsupported; GizClaw owns SQLite PRAGMA configuration", key)
		}
	}
	return nil
}

func prepareSQLiteDir(name, dir string) error {
	if dir == "" {
		return nil
	}
	parent := filepath.Dir(dir)
	if parent == "." || parent == "" {
		return nil
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("storage: sql %q mkdir: %w", name, err)
	}
	return nil
}
