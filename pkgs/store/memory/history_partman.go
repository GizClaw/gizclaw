package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/jmoiron/sqlx"
)

func (store *SQLHistoryStore) partmanParent(ctx context.Context, tx *sqlx.Tx) (string, error) {
	var schema string
	if err := tx.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
		return "", storage.ExternalSQLError("history: resolve partition schema", err)
	}
	if err := storage.ValidateSQLIdentifier(schema); err != nil {
		return "", err
	}
	return schema + "." + store.table.Name(), nil
}

func (store *SQLHistoryStore) initializePartman(ctx context.Context) error {
	tx, err := store.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := storage.LockPostgreSQLTable(ctx, tx, store.table); err != nil {
		return err
	}
	for _, operation := range []string{"run_maintenance", "drop_partition_time"} {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "pg_partman "+operation); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "SET LOCAL TIME ZONE 'UTC'"); err != nil {
		return err
	}
	var installed bool
	if err := tx.GetContext(ctx, &installed, "SELECT EXISTS(SELECT 1 FROM pg_extension e JOIN pg_namespace n ON n.oid=e.extnamespace WHERE e.extname='pg_partman' AND n.nspname='partman')"); err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("history: PostgreSQL requires pg_partman installed in schema partman")
	}
	parent, err := store.partmanParent(ctx, tx)
	if err != nil {
		return err
	}
	var registered bool
	if err := tx.GetContext(ctx, &registered, "SELECT EXISTS(SELECT 1 FROM partman.part_config WHERE parent_table=$1)", parent); err != nil {
		return storage.ExternalSQLError("history: inspect pg_partman registration", err)
	}
	if !registered {
		if _, err := tx.ExecContext(ctx, "SELECT partman.create_parent($1, 'expires_at_unix_nano', '1 day', p_epoch := 'nanoseconds', p_premake := 2, p_start_partition := $2, p_default_table := false, p_jobmon := false)", parent, time.Now().UTC().Truncate(24*time.Hour).Format(time.RFC3339)); err != nil {
			return storage.ExternalSQLError("history: register pg_partman parent", err)
		}
	}
	// The partition key is already the exact expiration timestamp. Adding the
	// message retention window again here would retain expired content twice.
	result, err := tx.ExecContext(ctx, "UPDATE partman.part_config SET retention='0 seconds',retention_keep_table=false,infinite_time_partitions=true,inherit_privileges=true WHERE parent_table=$1 AND control='expires_at_unix_nano' AND epoch='nanoseconds' AND partition_interval='1 day'", parent)
	if err != nil {
		return storage.ExternalSQLError("history: configure pg_partman retention", err)
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fmt.Errorf("history: incompatible pg_partman registration")
	}
	var valid bool
	if err := tx.GetContext(ctx, &valid, "SELECT EXISTS(SELECT 1 FROM pg_partitioned_table p JOIN pg_attribute a ON a.attrelid=p.partrelid AND a.attnum=p.partattrs[0] WHERE p.partrelid=to_regclass(quote_ident(current_schema())||'.'||quote_ident($1)) AND p.partstrat='r' AND p.partnatts=1 AND a.attname='expires_at_unix_nano')", store.table.Name()); err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("history: incompatible expiration partition key")
	}
	if err := store.preparePartmanDays(ctx, tx, []time.Time{time.Now().UTC().Add(store.ttl)}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return storage.ExternalSQLError("history: commit pg_partman initialization", err)
	}
	day := time.Now().UTC().Add(store.ttl).Truncate(24 * time.Hour)
	store.prepared.Store(day, struct{}{})
	store.prepared.Store(day.AddDate(0, 0, 1), struct{}{})
	return nil
}

func (store *SQLHistoryStore) preparePartitions(ctx context.Context, entries []HistoryEntry) error {
	if store.table.Dialect() != storage.SQLDialectPostgreSQL {
		return nil
	}
	days := make(map[time.Time]struct{})
	for _, entry := range entries {
		if entry.ExpiresAt == nil {
			return fmt.Errorf("%w: PostgreSQL history expiration is required", ErrInvalidInput)
		}
		day := entry.ExpiresAt.UTC().Truncate(24 * time.Hour)
		if _, ok := store.prepared.Load(day); !ok {
			days[day] = struct{}{}
		}
	}
	if len(days) == 0 {
		return nil
	}
	required := make([]time.Time, 0, len(days))
	for day := range days {
		required = append(required, day)
	}
	tx := store.tx
	if tx == nil {
		var err error
		tx, err = store.db.BeginTxx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}
	if err := storage.LockPostgreSQLTable(ctx, tx, store.table); err != nil {
		return err
	}
	if store.tx == nil {
		pending := required[:0]
		for _, day := range required {
			if _, ok := store.prepared.Load(day); !ok {
				pending = append(pending, day)
			}
		}
		required = pending
	}
	if err := store.preparePartmanDays(ctx, tx, required); err != nil {
		return err
	}
	if store.tx == nil {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	for day := range days {
		store.prepared.Store(day, struct{}{})
	}
	return nil
}

func (store *SQLHistoryStore) preparePartmanDays(ctx context.Context, tx *sqlx.Tx, days []time.Time) error {
	if _, err := tx.ExecContext(ctx, "SET LOCAL TIME ZONE 'UTC'"); err != nil {
		return err
	}
	parent, err := store.partmanParent(ctx, tx)
	if err != nil {
		return err
	}
	for _, at := range days {
		for _, day := range []time.Time{at.UTC().Truncate(24 * time.Hour), at.UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)} {
			if _, err := tx.ExecContext(ctx, "SELECT partman.create_partition_time($1, ARRAY[$2]::timestamptz[])", parent, day); err != nil {
				return storage.ExternalSQLError("history: prepare expiration partition", err)
			}
		}
	}
	return nil
}

func (store *SQLHistoryStore) maintainPartman(ctx context.Context, tx *sqlx.Tx) error {
	if _, err := tx.ExecContext(ctx, "SET LOCAL TIME ZONE 'UTC'"); err != nil {
		return err
	}
	if err := storage.LockPostgreSQLTable(ctx, tx, store.table); err != nil {
		return err
	}
	for _, operation := range []string{"run_maintenance", "drop_partition_time"} {
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", "pg_partman "+operation); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "LOCK TABLE ONLY "+store.table.Quoted()+" IN ACCESS EXCLUSIVE MODE"); err != nil {
		return storage.ExternalSQLError("history: protect retention parent", err)
	}
	parent, err := store.partmanParent(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "SELECT partman.run_maintenance(p_parent_table := $1,p_analyze := false,p_jobmon := false)", parent); err != nil {
		return storage.ExternalSQLError("history: run pg_partman maintenance", err)
	}
	return store.preparePartmanDays(ctx, tx, []time.Time{time.Now().UTC().Add(store.ttl)})
}
