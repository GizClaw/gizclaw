// Package sqltest supplies observable SQLite connections for storage regression tests.
package sqltest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"modernc.org/sqlite"
)

// Observer records executed SQL statements and rows consumed by the application.
// Delay applies to each statement, including statements inside transactions.
type Observer struct {
	Statements atomic.Int64
	Rows       atomic.Int64
	Delay      atomic.Int64
}

// Reset clears counts without changing statement latency.
func (o *Observer) Reset() { o.Statements.Store(0); o.Rows.Store(0) }

// New creates an observed SQLite database and closes it at test cleanup.
func New(t testing.TB) (*sqlx.DB, *Observer) {
	t.Helper()
	observer := &Observer{}
	raw := sql.OpenDB(&connector{observer: observer})
	raw.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := raw.Close(); err != nil {
			t.Error(err)
		}
	})
	return sqlx.NewDb(raw, "sqlite"), observer
}

type connector struct{ observer *Observer }

func (c *connector) Driver() driver.Driver { return &sqlite.Driver{} }
func (c *connector) Connect(ctx context.Context) (driver.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := c.Driver().Open(":memory:")
	if err != nil {
		return nil, err
	}
	return &observedConn{Conn: conn, observer: c.observer}, nil
}

type observedConn struct {
	driver.Conn
	observer *Observer
}

func (c *observedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	executor, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if err := c.observer.before(ctx); err != nil {
		return nil, err
	}
	return executor.ExecContext(ctx, query, args)
}
func (c *observedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	if err := c.observer.before(ctx); err != nil {
		return nil, err
	}
	rows, err := queryer.QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	return &observedRows{Rows: rows, observer: c.observer}, nil
}
func (c *observedConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &observedStmt{Stmt: stmt, observer: c.observer}, nil
}
func (c *observedConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if conn, ok := c.Conn.(driver.ConnBeginTx); ok {
		return conn.BeginTx(ctx, options)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return c.Conn.Begin()
}

type observedStmt struct {
	driver.Stmt
	observer *Observer
}

func (s *observedStmt) Exec(args []driver.Value) (driver.Result, error) {
	if err := s.observer.before(context.Background()); err != nil {
		return nil, err
	}
	return s.Stmt.Exec(args)
}
func (s *observedStmt) Query(args []driver.Value) (driver.Rows, error) {
	if err := s.observer.before(context.Background()); err != nil {
		return nil, err
	}
	rows, err := s.Stmt.Query(args)
	if err != nil {
		return nil, err
	}
	return &observedRows{Rows: rows, observer: s.observer}, nil
}

type observedRows struct {
	driver.Rows
	observer *Observer
}

func (r *observedRows) Next(values []driver.Value) error {
	err := r.Rows.Next(values)
	if err == nil {
		r.observer.Rows.Add(1)
	}
	return err
}
func (o *Observer) before(ctx context.Context) error {
	o.Statements.Add(1)
	if delay := time.Duration(o.Delay.Load()); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return ctx.Err()
}
