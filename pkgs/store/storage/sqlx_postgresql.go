package storage

import (
	"github.com/jmoiron/sqlx"

	_ "github.com/lib/pq"
)

func newPostgreSQL(name string, cfg PostgreSQLConfig) (*sqlx.DB, error) {
	return newSQL(name, KindPostgreSQL, "postgres", cfg.DSN, nil)
}
