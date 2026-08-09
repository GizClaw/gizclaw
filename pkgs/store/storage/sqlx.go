package storage

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

func newSQL(
	name, kind, driver, dsn string,
	configure func(*sqlx.DB) error,
) (*sqlx.DB, error) {
	if dsn == "" {
		return nil, fmt.Errorf("storage: %s %q requires dsn", kind, name)
	}
	if sqlx.BindType(driver) == sqlx.UNKNOWN {
		return nil, fmt.Errorf("storage: sql %q unsupported dialect %q", name, driver)
	}
	db, err := sqlx.Open(driver, dsn)
	if err != nil {
		return nil, &externalOperationError{operation: fmt.Sprintf("storage: sql %q open", name), err: err}
	}
	if configure != nil {
		if err := configure(db); err != nil {
			db.Close()
			return nil, err
		}
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, &externalOperationError{operation: fmt.Sprintf("storage: sql %q ping", name), err: err}
	}
	return db, nil
}
