package storage

import (
	"github.com/jmoiron/sqlx"

	_ "github.com/ClickHouse/clickhouse-go/v2"
)

func newClickHouse(name string, cfg ClickHouseConfig) (*sqlx.DB, error) {
	sqlx.BindDriver(KindClickHouse, sqlx.QUESTION)
	return newSQL(name, KindClickHouse, KindClickHouse, cfg.DSN, nil)
}
