package storage

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

// Keep this persisted hash input stable so existing table indexes retain their
// names after the helper ownership moves into storage.
const sqlIndexNamespacePrefix = "gizclaw/store/sql" + "backend/v1\x00"

const concurrentDDLAttempts = 8

var (
	identifierRE = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	tableNameRE  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

// SQLDialect identifies a supported SQL Store dialect.
type SQLDialect string

const (
	// SQLDialectSQLite is the modernc.org/sqlite dialect.
	SQLDialectSQLite SQLDialect = "sqlite"
	// SQLDialectPostgreSQL is the lib/pq dialect.
	SQLDialectPostgreSQL SQLDialect = "postgres"
)

// SQLTable describes one validated logical table on a borrowed SQL pool.
type SQLTable struct {
	dialect SQLDialect
	kind    string
	name    string
	quoted  string
}

// Dialect returns the table's validated SQL dialect.
func (table SQLTable) Dialect() SQLDialect { return table.dialect }

// Name returns the validated unqualified table name.
func (table SQLTable) Name() string { return table.name }

// Quoted returns the validated dialect-safe table identifier.
func (table SQLTable) Quoted() string { return table.quoted }

func (table SQLTable) validate() error {
	if table.dialect != SQLDialectSQLite && table.dialect != SQLDialectPostgreSQL {
		return errors.New("storage: sql table has invalid dialect")
	}
	validateName := ValidateSQLIdentifier
	if table.kind == "kv" {
		validateName = ValidateSQLTableName
	} else if table.kind != "metrics" && table.kind != "log" {
		return errors.New("storage: sql table has invalid Store kind")
	}
	if err := validateName(table.name); err != nil {
		return fmt.Errorf("storage: sql table has invalid name: %w", err)
	}
	if table.quoted != `"`+table.name+`"` {
		return errors.New("storage: sql table has invalid quoted identifier")
	}
	return nil
}

// ExternalSQLError preserves error identity while keeping driver text, which can
// contain bound values or connection details, out of the public error string.
func ExternalSQLError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &externalOperationError{operation: operation, err: err}
}

// SQLUnixNano returns a lossless signed nanosecond representation.
func SQLUnixNano(value time.Time) (int64, error) {
	value = value.UTC()
	nanoseconds := value.UnixNano()
	if !time.Unix(0, nanoseconds).UTC().Equal(value) {
		return 0, errors.New("time is outside signed nanosecond range")
	}
	return nanoseconds, nil
}

// PrepareSQLTable validates a borrowed pool, Store kind, and logical table.
func PrepareSQLTable(db *sqlx.DB, kind, table string) (SQLTable, error) {
	if db == nil {
		return SQLTable{}, errors.New("storage: sql db is nil")
	}
	if kind != "kv" && kind != "metrics" && kind != "log" {
		return SQLTable{}, fmt.Errorf("storage: sql unsupported store kind %q", kind)
	}
	validateTable := ValidateSQLIdentifier
	if kind == "kv" {
		validateTable = ValidateSQLTableName
	}
	if err := validateTable(table); err != nil {
		return SQLTable{}, fmt.Errorf("storage: sql table %q: %w", table, err)
	}
	dialect := SQLDialect(db.DriverName())
	if dialect != SQLDialectSQLite && dialect != SQLDialectPostgreSQL {
		return SQLTable{}, fmt.Errorf("storage: sql unsupported driver %q", db.DriverName())
	}
	quoted, err := QuoteSQLTableName(dialect, table)
	if err != nil {
		return SQLTable{}, err
	}
	return SQLTable{dialect: dialect, kind: kind, name: table, quoted: quoted}, nil
}

// EnsureSQLTable directly executes idempotent table/index initialization statements.
// It records no version or history and leaves the borrowed pool open.
func EnsureSQLTable(ctx context.Context, db *sqlx.DB, table SQLTable, statements ...string) error {
	if db == nil {
		return errors.New("storage: sql db is nil")
	}
	if err := table.validate(); err != nil {
		return err
	}
	if err := db.PingContext(ctx); err != nil {
		return ExternalSQLError(fmt.Sprintf("storage: sql ping %s table %q", table.dialect, table.name), err)
	}
	for _, statement := range statements {
		if err := executeInitializationStatement(ctx, db, table, statement); err != nil {
			return ExternalSQLError(fmt.Sprintf("storage: sql initialize %s table %q", table.kind, table.name), err)
		}
	}
	return nil
}

func executeInitializationStatement(ctx context.Context, db *sqlx.DB, table SQLTable, statement string) error {
	for attempt := range concurrentDDLAttempts {
		if _, err := db.ExecContext(ctx, statement); err == nil {
			return nil
		} else if table.dialect != SQLDialectPostgreSQL || !isConcurrentDDLConflict(err) || attempt == concurrentDDLAttempts-1 {
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

// SQLIndexName derives a stable, bounded identifier for one table index.
func SQLIndexName(table SQLTable, purpose string) (string, error) {
	if err := table.validate(); err != nil {
		return "", err
	}
	if err := ValidateSQLIdentifier(purpose); err != nil {
		return "", fmt.Errorf("storage: sql index purpose %q: %w", purpose, err)
	}
	canonical := table.name
	if table.dialect == SQLDialectSQLite {
		canonical = asciiLower(canonical)
	}
	digest := sha256.Sum256([]byte(sqlIndexNamespacePrefix + table.kind + "\x00" + canonical + "\x00" + purpose))
	name := fmt.Sprintf("gizclaw_%s_%x_%s", table.kind, digest[:8], purpose)
	if err := ValidateSQLIdentifier(name); err != nil {
		return "", fmt.Errorf("storage: sql derived index %q: %w", name, err)
	}
	return name, nil
}

// ValidateSQLIdentifier validates an unqualified, portable SQL identifier.
func ValidateSQLIdentifier(value string) error {
	if len(value) == 0 || len(value) > 63 || !identifierRE.MatchString(value) {
		return errors.New("identifier must match [A-Za-z_][A-Za-z0-9_]* and contain at most 63 bytes")
	}
	return nil
}

// ValidateSQLTableName validates an unqualified SQL table name. KV prefixes may
// contain hyphens, which remain safe because every accepted name is quoted.
func ValidateSQLTableName(value string) error {
	if len(value) == 0 || len(value) > 63 || !tableNameRE.MatchString(value) {
		return errors.New("table name must match [A-Za-z_][A-Za-z0-9_-]* and contain at most 63 bytes")
	}
	return nil
}

// QuoteSQLIdentifier returns a validated identifier quoted for SQLite or
// PostgreSQL.
func QuoteSQLIdentifier(dialect SQLDialect, value string) (string, error) {
	if dialect != SQLDialectSQLite && dialect != SQLDialectPostgreSQL {
		return "", fmt.Errorf("storage: sql unsupported dialect %q", dialect)
	}
	if err := ValidateSQLIdentifier(value); err != nil {
		return "", err
	}
	return `"` + value + `"`, nil
}

// QuoteSQLTableName returns a validated table name quoted for SQLite or
// PostgreSQL.
func QuoteSQLTableName(dialect SQLDialect, value string) (string, error) {
	if dialect != SQLDialectSQLite && dialect != SQLDialectPostgreSQL {
		return "", fmt.Errorf("storage: sql unsupported dialect %q", dialect)
	}
	if err := ValidateSQLTableName(value); err != nil {
		return "", err
	}
	return `"` + value + `"`, nil
}

func asciiLower(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}
