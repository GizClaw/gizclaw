package storage

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	_ "github.com/lib/pq"
)

func newPostgreSQL(name string, cfg PostgreSQLConfig) (*sqlx.DB, error) {
	if cfg.MaxOpenConns < 0 || cfg.ConnMaxIdleTime < 0 {
		return nil, fmt.Errorf("storage: postgresql %q requires nonnegative max open connections and connection idle time", name)
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen == 0 {
		maxOpen = 128
	}
	idleTime := cfg.ConnMaxIdleTime
	if idleTime == 0 {
		idleTime = time.Minute
	}
	return newSQL(name, KindPostgreSQL, "postgres", cfg.DSN, func(db *sqlx.DB) error {
		// Retain the burst's sessions rather than repeating TLS/authentication
		// at every permission refresh. Bound both the burst and idle retention.
		db.SetMaxOpenConns(maxOpen)
		db.SetMaxIdleConns(maxOpen)
		db.SetConnMaxIdleTime(idleTime)
		return nil
	})
}
