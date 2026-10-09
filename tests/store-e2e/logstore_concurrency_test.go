//go:build store_e2e

package store_e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/jmoiron/sqlx"
)

// This opt-in load is identical for the baseline and candidate binaries. All
// rows are synthetic; the optional trigger adds the same per-row database work
// to each version, making queue amplification observable without a provider.
func TestPostgreSQLLogAppendLoad(t *testing.T) {
	evidence := os.Getenv("GIZCLAW_TEST_PG_LOG_EVIDENCE")
	if evidence == "" {
		t.Skip("set GIZCLAW_TEST_PG_LOG_EVIDENCE to save a controlled load run")
	}
	delay := 0
	if value := os.Getenv("GIZCLAW_TEST_PG_LOG_DELAY_MS"); value != "" {
		var err error
		delay, err = strconv.Atoi(value)
		if err != nil || delay < 0 || delay > 100 {
			t.Fatal("GIZCLAW_TEST_PG_LOG_DELAY_MS must be in [0, 100]")
		}
	}
	const writers, batches, batchSize = 128, 4, 4
	db := openPostgreSQL(t)
	db.SetMaxOpenConns(writers + 4)
	db.SetMaxIdleConns(writers + 4)
	table := uniqueTable("history_load")
	cleanupPostgreSQLTables(t, db, table, table+"_keys")
	store, err := logstore.NewSQLStoreWithDBAndTTL(db, table, 30*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if delay > 0 {
		function := table + "_delay"
		if _, err := db.Exec(fmt.Sprintf(`CREATE FUNCTION "%s"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(%f); RETURN NEW; END $$`, function, float64(delay)/1000)); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DROP FUNCTION "` + function + `"() CASCADE`) })
		if _, err := db.Exec(`CREATE TRIGGER load_delay BEFORE INSERT ON "` + table + `" FOR EACH ROW EXECUTE FUNCTION "` + function + `"()`); err != nil {
			t.Fatal(err)
		}
	}
	observer := openPostgreSQL(t)
	observer.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	samples, stop := observePostgreSQLLocks(ctx, observer)
	start := make(chan struct{})
	latencies := make([]float64, writers*batches)
	errors := make([]error, writers)
	var workers sync.WaitGroup
	for writer := range writers {
		workers.Go(func() {
			<-start
			for batch := range batches {
				records := make([]logstore.Record, batchSize)
				for index := range records {
					records[index] = logstore.Record{Stream: fmt.Sprintf("workspace-%d", writer), ID: fmt.Sprintf("%d-%d", batch, index), Kind: "message", Time: time.Now().UTC()}
				}
				at := time.Now()
				keys, err := store.Append(ctx, records)
				latencies[writer*batches+batch] = float64(time.Since(at).Microseconds()) / 1000
				if err != nil {
					errors[writer] = err
					return
				}
				if len(keys) != len(records) {
					errors[writer] = fmt.Errorf("accepted %d of %d records", len(keys), len(records))
					return
				}
			}
		})
	}
	at := time.Now()
	close(start)
	workers.Wait()
	duration := time.Since(at)
	stop()
	failed := 0
	for _, err := range errors {
		if err != nil {
			failed++
			t.Error(err)
		}
	}
	var rows int
	if err := db.Get(&rows, `SELECT COUNT(*) FROM "`+table+`"`); err != nil {
		t.Fatal(err)
	}
	slices.Sort(latencies)
	peakAdvisory, peakRelation := 0, 0
	for _, sample := range *samples {
		advisory, relation := 0, 0
		for _, activity := range sample.Activity {
			if activity.Wait == "advisory" {
				advisory++
			}
			if activity.Wait == "relation" {
				relation++
			}
		}
		peakAdvisory = max(peakAdvisory, advisory)
		peakRelation = max(peakRelation, relation)
	}
	summary := map[string]any{
		"writers": writers, "batches_per_writer": batches, "batch_size": batchSize, "delay_ms_per_row": delay,
		"rows": rows, "failed_writers": failed, "seconds": duration.Seconds(), "rows_per_second": float64(rows) / duration.Seconds(),
		"append_p50_ms": latencies[len(latencies)/2], "append_p95_ms": latencies[len(latencies)*95/100], "append_max_ms": latencies[len(latencies)-1],
		"peak_advisory_waiters": peakAdvisory, "peak_relation_waiters": peakRelation, "append_latency_ms": latencies,
	}
	if err := os.MkdirAll(evidence, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]any{"summary": summary, "locks": samples} {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(evidence, name+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	delete(summary, "append_latency_ms")
	data, _ := json.Marshal(summary)
	t.Log(string(data))
	if rows != writers*batches*batchSize {
		t.Fatalf("persisted %d rows, want %d", rows, writers*batches*batchSize)
	}
}

type postgresLockActivity struct {
	PID      int    `json:"pid"`
	State    string `json:"state"`
	WaitType string `json:"wait_type"`
	Wait     string `json:"wait"`
	Query    string `json:"query"`
	Blockers string `json:"blockers"`
}

type postgresLockSample struct {
	At       time.Time              `json:"at"`
	Activity []postgresLockActivity `json:"activity"`
	Error    string                 `json:"error,omitempty"`
}

func observePostgreSQLLocks(ctx context.Context, db *sqlx.DB) (*[]postgresLockSample, func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	samples := &[]postgresLockSample{}
	go func() {
		defer close(done)
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case at := <-ticker.C:
				sample := postgresLockSample{At: at}
				rows, err := db.QueryContext(ctx, `SELECT pid, state, COALESCE(wait_event_type,''), COALESCE(wait_event,''), query, pg_blocking_pids(pid)::text FROM pg_stat_activity WHERE datname=current_database() AND pid <> pg_backend_pid() AND state <> 'idle'`)
				if err != nil {
					if ctx.Err() == nil {
						sample.Error = err.Error()
						*samples = append(*samples, sample)
					}
					continue
				}
				for rows.Next() {
					var activity postgresLockActivity
					if err := rows.Scan(&activity.PID, &activity.State, &activity.WaitType, &activity.Wait, &activity.Query, &activity.Blockers); err != nil {
						sample.Error = err.Error()
						break
					}
					sample.Activity = append(sample.Activity, activity)
				}
				if err := rows.Err(); err != nil && ctx.Err() == nil {
					sample.Error = err.Error()
				}
				_ = rows.Close()
				*samples = append(*samples, sample)
			}
		}
	}()
	return samples, func() { cancel(); <-done }
}
