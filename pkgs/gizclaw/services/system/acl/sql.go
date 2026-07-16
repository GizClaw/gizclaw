package acl

import (
	"context"
	"database/sql"
	"time"
)

type sqlExecutor interface {
	Rebind(string) string
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

type sqlQuerier interface {
	Rebind(string) string
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func parseNullableTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	t, err := parseTime(value.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

func (s *Server) validateStore() error {
	if s == nil {
		return errNilServer
	}
	if s.DB == nil {
		return errNilDB
	}
	return nil
}
