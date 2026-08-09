// Package sqlmigration owns the common migration lifecycle for SQL-backed
// logical Stores. The supplied connection pool is always borrowed.
package sqlmigration

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

const namespacePrefix = "gizclaw/store/sqlmigration/v1\x00"

var identifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Dialect identifies a supported SQL Store dialect.
type Dialect string

const (
	// SQLite is the modernc.org/sqlite dialect.
	SQLite Dialect = "sqlite"
	// PostgreSQL is the lib/pq dialect.
	PostgreSQL Dialect = "postgres"
)

// Namespace contains stable identifiers derived from a logical table claim.
type Namespace struct {
	Dialect      Dialect
	Table        string
	VersionTable string
	LockID       int64
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

// Prepare validates a borrowed pool and derives its stable migration namespace.
func Prepare(db *sqlx.DB, schemaKind, table string) (Namespace, error) {
	if db == nil {
		return Namespace{}, errors.New("sqlmigration: db is nil")
	}
	if schemaKind != "kv" && schemaKind != "metrics" && schemaKind != "log" {
		return Namespace{}, fmt.Errorf("sqlmigration: unsupported schema kind %q", schemaKind)
	}
	if err := ValidateIdentifier(table); err != nil {
		return Namespace{}, fmt.Errorf("sqlmigration: table %q: %w", table, err)
	}
	dialect := Dialect(db.DriverName())
	if dialect != SQLite && dialect != PostgreSQL {
		return Namespace{}, fmt.Errorf("sqlmigration: unsupported driver %q", db.DriverName())
	}
	canonical := table
	if dialect == SQLite {
		canonical = asciiLower(table)
	}
	digest := sha256.Sum256([]byte(namespacePrefix + schemaKind + "\x00" + canonical))
	lockID := int64(binary.BigEndian.Uint64(digest[:8]))
	if lockID == 0 {
		lockID = 1
	}
	return Namespace{
		Dialect:      dialect,
		Table:        table,
		VersionTable: fmt.Sprintf("gizclaw_%s_%x_schema_versions", schemaKind, digest[:8]),
		LockID:       lockID,
	}, nil
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
		return "", fmt.Errorf("sqlmigration: unsupported dialect %q", dialect)
	}
	if err := ValidateIdentifier(value); err != nil {
		return "", err
	}
	return `"` + value + `"`, nil
}

// Run applies all forward migrations and verifies the latest version. It does
// not call Provider.Close because goose closes the supplied borrowed pool.
func Run(ctx context.Context, db *sqlx.DB, schemaKind, table string, migrations ...*goose.Migration) (Namespace, error) {
	namespace, err := Prepare(db, schemaKind, table)
	if err != nil {
		return Namespace{}, err
	}
	if err := db.PingContext(ctx); err != nil {
		return Namespace{}, fmt.Errorf("sqlmigration: ping %s table %q: %w", namespace.Dialect, table, err)
	}
	dataExists, err := TableExists(ctx, db, namespace.Dialect, table)
	if err != nil {
		return Namespace{}, err
	}
	versionExists, err := TableExists(ctx, db, namespace.Dialect, namespace.VersionTable)
	if err != nil {
		return Namespace{}, err
	}
	if dataExists && !versionExists {
		return Namespace{}, fmt.Errorf("sqlmigration: table %q exists without migration history", table)
	}

	dialect := goose.DialectSQLite3
	options := []goose.ProviderOption{
		goose.WithTableName(namespace.VersionTable),
		goose.WithGoMigrations(migrations...),
		goose.WithDisableGlobalRegistry(true),
	}
	if namespace.Dialect == PostgreSQL {
		dialect = goose.DialectPostgres
		locker, err := lock.NewPostgresSessionLocker(
			lock.WithLockID(namespace.LockID),
			lock.WithLockTimeout(1, 30),
			lock.WithUnlockTimeout(1, 5),
		)
		if err != nil {
			return Namespace{}, fmt.Errorf("sqlmigration: create postgres locker: %w", err)
		}
		options = append(options, goose.WithSessionLocker(locker))
	}
	provider, err := goose.NewProvider(dialect, db.DB, nil, options...)
	if err != nil {
		return Namespace{}, fmt.Errorf("sqlmigration: create %s provider for %q: %w", schemaKind, table, err)
	}
	wantVersion := int64(0)
	for _, migration := range migrations {
		if migration != nil && migration.Version > wantVersion {
			wantVersion = migration.Version
		}
	}
	if _, err := provider.Up(ctx); err != nil {
		// Two SQLite providers can both read version zero before either starts
		// its transactional migration. The loser may then receive an
		// already-exists error from its stale transaction snapshot even though
		// the winner committed the data table and version row atomically. Verify
		// the fresh committed version before treating that race as a failure;
		// every constructor still performs its exact schema checks afterward.
		if namespace.Dialect != SQLite {
			return Namespace{}, fmt.Errorf("sqlmigration: migrate %s table %q: %w", schemaKind, table, err)
		}
		if contextErr := ctx.Err(); contextErr != nil {
			return Namespace{}, contextErr
		}
		version, versionErr := provider.GetDBVersion(ctx)
		if versionErr != nil || version != wantVersion {
			return Namespace{}, fmt.Errorf("sqlmigration: migrate %s table %q: %w", schemaKind, table, err)
		}
	}
	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return Namespace{}, fmt.Errorf("sqlmigration: inspect %s table %q version: %w", schemaKind, table, err)
	}
	if version != wantVersion {
		return Namespace{}, fmt.Errorf("sqlmigration: %s table %q version is %d, want %d", schemaKind, table, version, wantVersion)
	}
	return namespace, nil
}

// TableExists reports whether an exact table exists in the current schema.
func TableExists(ctx context.Context, db *sqlx.DB, dialect Dialect, table string) (bool, error) {
	if err := ValidateIdentifier(table); err != nil {
		return false, err
	}
	var exists bool
	switch dialect {
	case SQLite:
		err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND lower(name) = lower(?))`,
			table,
		).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("sqlmigration: inspect sqlite table %q: %w", table, err)
		}
	case PostgreSQL:
		err := db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = $1)`,
			table,
		).Scan(&exists)
		if err != nil {
			return false, fmt.Errorf("sqlmigration: inspect postgres table %q: %w", table, err)
		}
	default:
		return false, fmt.Errorf("sqlmigration: unsupported dialect %q", dialect)
	}
	return exists, nil
}

// Rebind converts question-mark placeholders to the pool's dialect.
func Rebind(db *sqlx.DB, query string) string { return db.Rebind(query) }

// TxExec executes one statement inside a goose transaction.
func TxExec(ctx context.Context, tx *sql.Tx, statement string) error {
	_, err := tx.ExecContext(ctx, statement)
	return err
}

func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}
