package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/sqlmeta"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

const (
	historyQueryPartitions = 31
	historyQueryRowsPerDay = 30_000
	historyQueryScopes     = 1_000
	historyQuerySeed       = 7_919
)

// Seed only synthetic history, through the production parent's exact column
// layout. COPY-like SQL keeps preparation practical; live writes use Append.
func seedPostgreSQLHistoryQuery(t *testing.T, ctx context.Context, db *sqlx.DB, table, evidence string) {
	t.Helper()
	parent, err := sqlmeta.PrepareSQLTable(db, "sql", table)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := sqlmeta.SQLIndexName(parent, "history_keys")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := sqlmeta.SQLIndexName(parent, "history_assets")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.GetContext(ctx, &count, `SELECT count(*) FROM "`+table+`" WHERE id LIKE 'seed-%'`); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		base := time.Now().UTC().Truncate(24 * time.Hour)
		tx, err := db.BeginTxx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		var schema string
		if err := tx.GetContext(ctx, &schema, "SELECT current_schema()"); err != nil {
			t.Fatal(err)
		}
		for day := 1; day <= historyQueryPartitions; day++ {
			lower := base.Add(time.Duration(day) * 24 * time.Hour)
			if _, err := tx.ExecContext(ctx, "SELECT partman.create_partition_time($1,ARRAY[$2]::timestamptz[])", schema+"."+table, lower); err != nil {
				t.Fatal(err)
			}
			statement := fmt.Sprintf(`INSERT INTO "%s" (conversation_id,id,turn_id,role,actor_id,origin,actor_name,text,created_at_unix_nano,expires_at_unix_nano,interrupted)
 SELECT 'scope-' || lpad(((n*$4) %% $5)::text,4,'0'), 'seed-' || lpad($1::text,2,'0') || '-' || lpad(n::text,6,'0'), '', 'assistant','','fixture','assistant','synthetic history',$2::bigint+n::bigint*1000000,$3::bigint+n::bigint*1000000,0
 FROM generate_series(0,$6::integer-1) n`, table)
			if _, err := tx.ExecContext(ctx, statement, day, lower.Add(-30*24*time.Hour).UnixNano(), lower.UnixNano(), historyQuerySeed, historyQueryScopes, historyQueryRowsPerDay); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO "`+keys+`" SELECT conversation_id,id,expires_at_unix_nano FROM "`+table+`" WHERE id LIKE 'seed-%'`); err != nil {
			t.Fatal(err)
		}

		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
		count = historyQueryPartitions * historyQueryRowsPerDay
	}
	if count != historyQueryPartitions*historyQueryRowsPerDay {
		t.Fatalf("seed rows=%d", count)
	}
	// Reuse exactly the seeded history across revisions; discard only rows
	// produced by previous runs inside this dedicated fixture schema.
	for _, relation := range []string{assets, table, keys} {
		if _, err := db.ExecContext(ctx, `DELETE FROM "`+relation+`" WHERE id NOT LIKE 'seed-%'`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, `ANALYZE "`+table+`"`); err != nil {
		t.Fatal(err)
	}
	var indexes string
	if err := db.GetContext(ctx, &indexes, `SELECT coalesce(json_agg(row_to_json(i)), '[]')::text FROM (SELECT tablename,indexname,indexdef FROM pg_indexes WHERE schemaname=current_schema() AND tablename LIKE $1 ORDER BY tablename,indexname) i`, table+"%"); err != nil {
		t.Fatal(err)
	}
	writeHistoryQueryEvidence(t, evidence, "indexes.json", []byte(indexes))
	var rangeJSON string
	if err := db.GetContext(ctx, &rangeJSON, `SELECT json_build_object('oldest_timestamp_ns',min(created_at_unix_nano),'newest_timestamp_ns',max(created_at_unix_nano),'earliest_expiry_ns',min(expires_at_unix_nano),'latest_expiry_ns',max(expires_at_unix_nano),'fingerprint',sum(hashtextextended(ROW(conversation_id,id,created_at_unix_nano,expires_at_unix_nano,role,text)::text,7919)::numeric)::text)::text FROM "`+table+`" WHERE id LIKE 'seed-%'`); err != nil {
		t.Fatal(err)
	}
	writeHistoryQueryJSON(t, evidence, "fixture.json", map[string]any{"rows": count, "partitions_with_data": historyQueryPartitions, "scopes": historyQueryScopes, "permutation_seed": historyQuerySeed, "rows_per_day": historyQueryRowsPerDay, "range": json.RawMessage(rangeJSON)})
	var settings string
	if err := db.GetContext(ctx, &settings, `SELECT json_build_object('version',version(),'max_connections',current_setting('max_connections'),'shared_buffers',current_setting('shared_buffers'),'work_mem',current_setting('work_mem'),'random_page_cost',current_setting('random_page_cost'),'track_io_timing',current_setting('track_io_timing'))::text`); err != nil {
		t.Fatal(err)
	}
	writeHistoryQueryEvidence(t, evidence, "postgres.json", []byte(settings))
}

// Capture the statement and bound values at database/sql, without maintaining
// a second SQL builder or changing the Store's production API.
type historyQueryConnector struct {
	driver.Connector
	statement string
	arguments []any
}
type historyQueryConn struct {
	driver.Conn
	capture *historyQueryConnector
}

func (c *historyQueryConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &historyQueryConn{Conn: conn, capture: c}, nil
}
func (c *historyQueryConn) QueryContext(ctx context.Context, query string, arguments []driver.NamedValue) (driver.Rows, error) {
	if strings.HasPrefix(query, "SELECT conversation_id,id,turn_id") {
		c.capture.statement = query
		c.capture.arguments = make([]any, len(arguments))
		for i, argument := range arguments {
			c.capture.arguments[i] = argument.Value
		}
	}
	queryer, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	return queryer.QueryContext(ctx, query, arguments)
}

func historyLoadQuery(scope int) memory.HistoryQuery {
	return memory.HistoryQuery{ConversationID: fmt.Sprintf("scope-%04d", scope), Descending: true, Limit: 50}
}

func capturePostgreSQLHistoryPlans(t *testing.T, ctx context.Context, db *sqlx.DB, dsn, table, evidence string) {
	t.Helper()
	connector, err := pq.NewConnector(dsn)
	if err != nil {
		t.Fatal(err)
	}
	capture := &historyQueryConnector{Connector: connector}
	traced := sqlx.NewDb(sql.OpenDB(capture), "postgres")
	defer traced.Close()
	store, err := memory.NewSQLHistoryStore(ctx, traced, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		scope int
	}{{"existing", 7}, {"missing", 1007}} {
		page, err := store.Query(ctx, historyLoadQuery(test.scope))
		if err != nil {
			t.Fatal(err)
		}
		if (test.scope < historyQueryScopes && len(page.Entries) != 50) || (test.scope >= historyQueryScopes && len(page.Entries) != 0) {
			t.Fatalf("%s page records=%d", test.name, len(page.Entries))
		}
		writeHistoryQueryJSON(t, evidence, test.name+".sql.json", map[string]any{"statement": capture.statement, "arguments": capture.arguments})
		var plan string
		if err := db.GetContext(ctx, &plan, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+capture.statement, capture.arguments...); err != nil {
			t.Fatal(err)
		}
		writeHistoryQueryEvidence(t, evidence, test.name+".plan.json", []byte(plan))
		// Expiration is the partition key. Measure its nullable disjunction
		// independently from attribute lookup on this NOT NULL retained table.
		statement := strings.ReplaceAll(capture.statement, "(expires_at_unix_nano IS NULL OR expires_at_unix_nano > $2)", "expires_at_unix_nano > $2")
		if err := db.GetContext(ctx, &plan, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+statement, capture.arguments...); err != nil {
			t.Fatal(err)
		}
		writeHistoryQueryEvidence(t, evidence, test.name+".nonnull-expiry.plan.json", []byte(plan))
	}
}

type historyQueryTiming struct {
	Milliseconds float64 `json:"ms"`
	Error        string  `json:"error,omitempty"`
}

func historyQuerySummary(timings []historyQueryTiming) map[string]any {
	values := make([]float64, len(timings))
	errors := 0
	for i, sample := range timings {
		values[i] = sample.Milliseconds
		if sample.Error != "" {
			errors++
		}
	}
	slices.Sort(values)
	quantile := func(q float64) float64 {
		if len(values) == 0 {
			return 0
		}
		return values[int(float64(len(values)-1)*q)]
	}
	return map[string]any{"count": len(timings), "errors": errors, "p50_ms": quantile(.5), "p95_ms": quantile(.95), "p99_ms": quantile(.99), "max_ms": quantile(1)}
}

func startPostgreSQLHistoryQueryLoad(t *testing.T, ctx context.Context, pool, observer *sqlx.DB, dsn, table, evidence string) func() {
	t.Helper()
	seedPostgreSQLHistoryQuery(t, ctx, observer, table, evidence)
	capturePostgreSQLHistoryPlans(t, ctx, observer, dsn, table, evidence)
	store, err := memory.NewSQLHistoryStore(ctx, pool, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	// Fixed work and concurrency, including enough readers to observe waits in
	// the production 128-session pool. Queries alternate existing/absent scopes.
	const readers = 160
	timings := make([]historyQueryTiming, readers)
	writes := make([][]historyQueryTiming, 4)
	probes := make([]historyQueryTiming, 32)
	connections := make([]*sql.Conn, 0, 128)
	for range 128 {
		conn, err := pool.Conn(ctx)
		if err != nil {
			for _, open := range connections {
				_ = open.Close()
			}
			t.Fatal(err)
		}
		connections = append(connections, conn)
	}
	for _, conn := range connections {
		_ = conn.Close()
	}
	before := pool.Stats()
	start := time.Now()
	ready := make(chan struct{})
	var workers sync.WaitGroup
	for reader := range readers {
		workers.Go(func() {
			<-ready
			queryCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			scope := reader % historyQueryScopes
			if reader%2 == 1 {
				scope += historyQueryScopes
			}
			at := time.Now()
			_, err := store.Query(queryCtx, historyLoadQuery(scope))
			timings[reader].Milliseconds = float64(time.Since(at)) / float64(time.Millisecond)
			if err != nil {
				timings[reader].Error = err.Error()
			}
		})
	}
	for writer := range writes {
		workers.Go(func() {
			<-ready
			for batch := range 32 {
				at := time.Now()
				err := store.Append(ctx, []memory.HistoryEntry{{ID: fmt.Sprintf("fixed-%d-%d-%d", start.UnixNano(), writer, batch), ConversationID: "benchmark.history", Role: memory.RoleUser, Text: "load", CreatedAt: at.UTC()}})
				sample := historyQueryTiming{Milliseconds: float64(time.Since(at)) / float64(time.Millisecond)}
				if err != nil {
					sample.Error = err.Error()
				}
				writes[writer] = append(writes[writer], sample)
			}
		})
	}
	workers.Go(func() {
		<-ready
		for i := range probes {
			probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			at := time.Now()
			var one int
			err := pool.GetContext(probeCtx, &one, "SELECT 1")
			cancel()
			probes[i].Milliseconds = float64(time.Since(at)) / float64(time.Millisecond)
			if err != nil {
				probes[i].Error = err.Error()
			}
		}
	})
	close(ready)
	workers.Wait()
	elapsed := time.Since(start)
	after := pool.Stats()
	writeHistoryQueryJSON(t, evidence, "query-load.json", map[string]any{"readers": readers, "elapsed_ms": float64(elapsed) / float64(time.Millisecond), "queries_per_second": float64(readers) / elapsed.Seconds(), "latency": historyQuerySummary(timings), "samples": timings, "pool_before": before, "pool_after": after, "pool_wait_count": after.WaitCount - before.WaitCount, "pool_wait_ms": float64(after.WaitDuration-before.WaitDuration) / float64(time.Millisecond)})
	var writeSamples []historyQueryTiming
	for _, batch := range writes {
		writeSamples = append(writeSamples, batch...)
	}
	writeHistoryQueryJSON(t, evidence, "write-load.json", map[string]any{"writers": len(writes), "latency": historyQuerySummary(writeSamples), "samples": writeSamples})
	writeHistoryQueryJSON(t, evidence, "pool-probes.json", map[string]any{"query": "SELECT 1", "timeout_ms": 2000, "latency": historyQuerySummary(probes), "samples": probes})
	t.Logf("fixed history query load: %v", historyQuerySummary(timings))
	loadCtx, cancel := context.WithCancel(ctx)
	var background sync.WaitGroup
	liveWrites := make([][]historyQueryTiming, 4)
	liveReads := make([]struct {
		Completed int
		Errors    int
	}, 16)
	for reader := range 16 {
		background.Go(func() {
			for loadCtx.Err() == nil {
				_, err := store.Query(loadCtx, historyLoadQuery(1000+reader))
				if loadCtx.Err() != nil {
					return
				}
				if err != nil {
					liveReads[reader].Errors++
				} else {
					liveReads[reader].Completed++
				}
			}
		})
	}
	for writer := range liveWrites {
		background.Go(func() {
			ticker := time.NewTicker(20 * time.Millisecond)
			defer ticker.Stop()
			for batch := 0; ; batch++ {
				select {
				case <-loadCtx.Done():
					return
				case <-ticker.C:
				}
				at := time.Now()
				err := store.Append(loadCtx, []memory.HistoryEntry{{ID: fmt.Sprintf("live-%d-%d-%d", start.UnixNano(), writer, batch), ConversationID: "benchmark.history", Role: memory.RoleUser, Text: "load", CreatedAt: at.UTC()}})
				if loadCtx.Err() != nil {
					return
				}
				sample := historyQueryTiming{Milliseconds: float64(time.Since(at)) / float64(time.Millisecond)}
				if err != nil {
					sample.Error = err.Error()
				}
				liveWrites[writer] = append(liveWrites[writer], sample)
			}
		})
	}
	return func() {
		cancel()
		background.Wait()
		var samples []historyQueryTiming
		for _, writer := range liveWrites {
			samples = append(samples, writer...)
		}
		writeHistoryQueryJSON(t, evidence, "giztest-writes.json", map[string]any{"latency": historyQuerySummary(samples), "samples": samples})
		writeHistoryQueryJSON(t, evidence, "giztest-readers.json", liveReads)
		writeHistoryQueryJSON(t, evidence, "pool-after-giztest.json", pool.Stats())
		var queries string
		if err := observer.GetContext(ctx, &queries, `SELECT coalesce(json_agg(row_to_json(q)),'[]')::text FROM (SELECT queryid::text,query,calls,rows,total_exec_time,mean_exec_time,shared_blks_hit,shared_blks_read,shared_blk_read_time,shared_blk_write_time FROM public.pg_stat_statements WHERE query LIKE $1 ORDER BY total_exec_time DESC) q`, "SELECT conversation_id,id,turn_id%"+table+"%"); err == nil {
			writeHistoryQueryEvidence(t, evidence, "pg-stat-statements.json", []byte(queries))
		} else {
			t.Logf("pg_stat_statements unavailable: %v", err)
		}
	}
}

func writeHistoryQueryJSON(t *testing.T, dir, name string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeHistoryQueryEvidence(t, dir, name, data)
}
func writeHistoryQueryEvidence(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
