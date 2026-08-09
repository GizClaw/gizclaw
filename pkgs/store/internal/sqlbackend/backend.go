// Package sqlbackend owns shared helpers for SQLite- and PostgreSQL-backed
// logical Stores. The supplied connection pool is always borrowed.
package sqlbackend

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

const indexNamespacePrefix = "gizclaw/store/sqlbackend/v1\x00"

const concurrentDDLAttempts = 8

var identifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Dialect identifies a supported SQL Store dialect.
type Dialect string

const (
	// SQLite is the modernc.org/sqlite dialect.
	SQLite Dialect = "sqlite"
	// PostgreSQL is the lib/pq dialect.
	PostgreSQL Dialect = "postgres"
)

// Backend describes one validated logical table on a borrowed SQL pool.
type Backend struct {
	Dialect Dialect
	Kind    string
	Table   string
	Quoted  string
}

type operationError struct {
	operation string
	err       error
}

func (err *operationError) Error() string { return err.operation + " failed" }
func (err *operationError) Unwrap() error { return err.err }

// ExternalError preserves error identity while keeping driver text, which can
// contain bound values or connection details, out of the public error string.
func ExternalError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &operationError{operation: operation, err: err}
}

// UnixNano returns a lossless signed nanosecond representation.
func UnixNano(value time.Time) (int64, error) {
	value = value.UTC()
	nanoseconds := value.UnixNano()
	if !time.Unix(0, nanoseconds).UTC().Equal(value) {
		return 0, errors.New("time is outside signed nanosecond range")
	}
	return nanoseconds, nil
}

// Prepare validates a borrowed pool, Store kind, and logical table.
func Prepare(db *sqlx.DB, kind, table string) (Backend, error) {
	if db == nil {
		return Backend{}, errors.New("sqlbackend: db is nil")
	}
	if kind != "kv" && kind != "metrics" && kind != "log" {
		return Backend{}, fmt.Errorf("sqlbackend: unsupported store kind %q", kind)
	}
	if err := ValidateIdentifier(table); err != nil {
		return Backend{}, fmt.Errorf("sqlbackend: table %q: %w", table, err)
	}
	dialect := Dialect(db.DriverName())
	if dialect != SQLite && dialect != PostgreSQL {
		return Backend{}, fmt.Errorf("sqlbackend: unsupported driver %q", db.DriverName())
	}
	quoted, err := Quote(dialect, table)
	if err != nil {
		return Backend{}, err
	}
	return Backend{Dialect: dialect, Kind: kind, Table: table, Quoted: quoted}, nil
}

// Ensure directly executes idempotent table/index initialization statements.
// It records no version or history and leaves the borrowed pool open.
func Ensure(ctx context.Context, db *sqlx.DB, backend Backend, statements ...string) error {
	if err := db.PingContext(ctx); err != nil {
		return ExternalError(fmt.Sprintf("sqlbackend: ping %s table %q", backend.Dialect, backend.Table), err)
	}
	for _, statement := range statements {
		if err := executeInitializationStatement(ctx, db, backend, statement); err != nil {
			return ExternalError(fmt.Sprintf("sqlbackend: initialize %s table %q", backend.Kind, backend.Table), err)
		}
	}
	return nil
}

func executeInitializationStatement(ctx context.Context, db *sqlx.DB, backend Backend, statement string) error {
	for attempt := range concurrentDDLAttempts {
		if _, err := db.ExecContext(ctx, statement); err == nil {
			return nil
		} else if backend.Dialect != PostgreSQL || !isConcurrentDDLConflict(err) || attempt == concurrentDDLAttempts-1 {
			return err
		}
		// PostgreSQL can transiently report a catalog uniqueness conflict when
		// concurrent CREATE ... IF NOT EXISTS statements race. Retry the same
		// idempotent statement after the winning transaction becomes visible.
		delay := time.Duration(1<<attempt) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

type sqlStateError interface {
	SQLState() string
}

func isConcurrentDDLConflict(err error) bool {
	var state sqlStateError
	if !errors.As(err, &state) {
		return false
	}
	return state.SQLState() == "23505" || state.SQLState() == "42P07"
}

// IndexName derives a stable, bounded identifier for one table index.
func IndexName(backend Backend, purpose string) (string, error) {
	if err := ValidateIdentifier(purpose); err != nil {
		return "", fmt.Errorf("sqlbackend: index purpose %q: %w", purpose, err)
	}
	canonical := backend.Table
	if backend.Dialect == SQLite {
		canonical = asciiLower(canonical)
	}
	digest := sha256.Sum256([]byte(indexNamespacePrefix + backend.Kind + "\x00" + canonical + "\x00" + purpose))
	name := fmt.Sprintf("gizclaw_%s_%x_%s", backend.Kind, digest[:8], purpose)
	if err := ValidateIdentifier(name); err != nil {
		return "", fmt.Errorf("sqlbackend: derived index %q: %w", name, err)
	}
	return name, nil
}

// ValidateIdentifier validates an unqualified, portable SQL identifier.
func ValidateIdentifier(value string) error {
	if len(value) == 0 || len(value) > 63 || !identifierRE.MatchString(value) {
		return errors.New("identifier must match [A-Za-z_][A-Za-z0-9_]* and contain at most 63 bytes")
	}
	return nil
}

// Quote returns a validated identifier quoted for SQLite or PostgreSQL.
func Quote(dialect Dialect, value string) (string, error) {
	if dialect != SQLite && dialect != PostgreSQL {
		return "", fmt.Errorf("sqlbackend: unsupported dialect %q", dialect)
	}
	if err := ValidateIdentifier(value); err != nil {
		return "", err
	}
	return `"` + value + `"`, nil
}

// Rebind converts question-mark placeholders to the pool's dialect.
func Rebind(db *sqlx.DB, query string) string { return db.Rebind(query) }

func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}
