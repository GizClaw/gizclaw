package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lib/pq"
)

// Only the opt-in build overlay selects this driver. It intentionally implements
// Driver, not DriverContext, exactly like pq.Driver in the pinned pq version.
func init() { sql.Register("run-state-trace", runStateTraceDriver{}) }

type runStateTraceEvent struct {
	Time        time.Time `json:"time"`
	Application string    `json:"application"`
	Connection  uint64    `json:"connection"`
	Operation   string    `json:"operation"`
	DurationMS  float64   `json:"duration_ms"`
	Error       string    `json:"error,omitempty"`
}

var runStateTrace struct {
	sync.Mutex
	events []runStateTraceEvent
	next   atomic.Uint64
}

func traceRunState(app string, id uint64, operation string, start time.Time, err error) {
	code := ""
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code = "deadline"
	case errors.Is(err, context.Canceled):
		code = "canceled"
	case err != nil && err != io.EOF:
		if pgError, ok := errors.AsType[*pq.Error](err); ok && pgError.Code == "57014" {
			code = "pg_canceled"
		} else {
			code = "driver_error"
		}
	}
	runStateTrace.Lock()
	runStateTrace.events = append(runStateTrace.events, runStateTraceEvent{Time: time.Now().UTC(), Application: app, Connection: id, Operation: operation, DurationMS: float64(time.Since(start)) / float64(time.Millisecond), Error: code})
	runStateTrace.Unlock()
}

type runStateTraceDriver struct{}

func (runStateTraceDriver) Open(dsn string) (driver.Conn, error) {
	id := runStateTrace.next.Add(1)
	app := ""
	if parsed, err := url.Parse(dsn); err == nil {
		app = parsed.Query().Get("application_name")
	}
	start := time.Now()
	conn, err := (pq.Driver{}).Open(dsn)
	traceRunState(app, id, "connect", start, err)
	if err != nil {
		return nil, err
	}
	return &runStateTraceConn{Conn: conn, app: app, id: id}, nil
}

type runStateTraceConn struct {
	driver.Conn
	app string
	id  uint64
}

func (c *runStateTraceConn) Close() error {
	start := time.Now()
	err := c.Conn.Close()
	traceRunState(c.app, c.id, "close", start, err)
	return err
}

func (c *runStateTraceConn) Ping(ctx context.Context) error { return c.Conn.(driver.Pinger).Ping(ctx) }
func (c *runStateTraceConn) ResetSession(ctx context.Context) error {
	return c.Conn.(driver.SessionResetter).ResetSession(ctx)
}
func (c *runStateTraceConn) IsValid() bool { return c.Conn.(driver.Validator).IsValid() }
func (c *runStateTraceConn) CheckNamedValue(v *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(v)
}
func (c *runStateTraceConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, query)
}

func (c *runStateTraceConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	start := time.Now()
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	traceRunState(c.app, c.id, "begin", start, err)
	if err != nil {
		return nil, err
	}
	return &runStateTraceTx{Tx: tx, conn: c, start: start}, nil
}

func (c *runStateTraceConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	start := time.Now()
	result, err := c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
	traceRunState(c.app, c.id, "exec", start, err)
	return result, err
}

func (c *runStateTraceConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	op := "query"
	if strings.Contains(query, "SELECT pending_workspace,active_workspace FROM peer_runs") {
		op = "run_state"
	}
	start := time.Now()
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	traceRunState(c.app, c.id, op, start, err)
	if err != nil {
		return nil, err
	}
	return &runStateTraceRows{Rows: rows, conn: c, op: op, start: start}, nil
}

type runStateTraceRows struct {
	driver.Rows
	conn  *runStateTraceConn
	op    string
	start time.Time
}

func (r *runStateTraceRows) Close() error {
	err := r.Rows.Close()
	traceRunState(r.conn.app, r.conn.id, r.op+"_rows_close", r.start, err)
	return err
}

type runStateTraceTx struct {
	driver.Tx
	conn  *runStateTraceConn
	start time.Time
}

func (tx *runStateTraceTx) Commit() error {
	err := tx.Tx.Commit()
	traceRunState(tx.conn.app, tx.conn.id, "commit", tx.start, err)
	return err
}
func (tx *runStateTraceTx) Rollback() error {
	err := tx.Tx.Rollback()
	traceRunState(tx.conn.app, tx.conn.id, "rollback", tx.start, err)
	return err
}
