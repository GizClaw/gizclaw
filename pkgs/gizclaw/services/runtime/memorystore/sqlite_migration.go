package memorystore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	flowrecall "github.com/GizClaw/flowcraft/memory/recall"
	flowsqlite "github.com/GizClaw/flowcraft/memory/recall/store/sqlite"
)

const (
	flowcraftSQLiteName       = "memory.db"
	legacyFlowcraftStateName  = "state.json"
	legacyFlowcraftVersion    = 1
	flowcraftMigrationName    = "workspace-state-v1"
	flowcraftMigrationTmpName = "memory.db.migrate"
)

type legacyFlowcraftState struct {
	Version     int                          `json:"version"`
	Facts       []flowrecall.TemporalFact    `json:"facts,omitempty"`
	Evidence    []legacyEvidenceRecord       `json:"evidence,omitempty"`
	SideEffects []legacySideEffectRecord     `json:"side_effects,omitempty"`
	Async       []legacyAsyncSemanticRecord  `json:"async_semantic,omitempty"`
	Counters    map[string]legacyCounterPair `json:"counters,omitempty"`
}

type legacyEvidenceRecord struct {
	Scope      flowrecall.Scope       `json:"scope"`
	FactID     string                 `json:"fact_id"`
	EvidenceID string                 `json:"evidence_id"`
	Ordinal    int                    `json:"ordinal"`
	Ref        flowrecall.EvidenceRef `json:"ref"`
}

type legacySideEffectRecord struct {
	Job        flowrecall.SideEffectJob     `json:"job"`
	Status     string                       `json:"status"`
	EnqueuedAt time.Time                    `json:"enqueued_at"`
	RetryAt    time.Time                    `json:"retry_at"`
	Failure    flowrecall.SideEffectFailure `json:"failure"`
	Result     flowrecall.SideEffectResult  `json:"result"`
}

type legacyAsyncSemanticRecord struct {
	Job        flowrecall.AsyncSemanticJob     `json:"job"`
	Status     string                          `json:"status"`
	EnqueuedAt time.Time                       `json:"enqueued_at"`
	Failure    flowrecall.AsyncSemanticFailure `json:"failure"`
	Result     flowrecall.AsyncSemanticResult  `json:"result"`
}

type legacyCounterPair struct {
	SideEffect    int `json:"side_effect,omitempty"`
	AsyncSemantic int `json:"async_semantic,omitempty"`
}

type sqliteMigrationOps struct {
	beforeImport func() error
	verify       func(context.Context, *sql.DB, legacyFlowcraftState, string) error
	checkpoint   func(context.Context, *sql.DB) error
	rename       func(string, string) error
}

func openManagedSQLite(ctx context.Context, dir string) (*flowsqlite.Backend, error) {
	if err := migrateLegacyFlowcraftState(ctx, dir, sqliteMigrationOps{rename: os.Rename}); err != nil {
		return nil, err
	}
	databasePath := filepath.Join(dir, flowcraftSQLiteName)
	if err := ensureManagedSQLiteFile(databasePath); err != nil {
		return nil, err
	}
	backend, err := flowsqlite.Open(ctx, databasePath)
	if err != nil {
		return nil, fmt.Errorf("memory store: open Flowcraft SQLite canonical store: %w", err)
	}
	return backend, nil
}

func ensureManagedSQLiteFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("memory store: create managed Flowcraft SQLite database: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("memory store: close managed Flowcraft SQLite database: %w", err)
	}
	return nil
}

func migrateLegacyFlowcraftState(ctx context.Context, dir string, ops sqliteMigrationOps) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("memory store: migrate legacy Flowcraft state: %w", err)
	}
	if ops.rename == nil {
		ops.rename = os.Rename
	}
	if ops.verify == nil {
		ops.verify = verifyMigrationCounts
	}
	if ops.checkpoint == nil {
		ops.checkpoint = checkpointMigratedSQLite
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("memory store: create Flowcraft binding root: %w", err)
	}
	databasePath := filepath.Join(dir, flowcraftSQLiteName)
	legacyPath := filepath.Join(dir, legacyFlowcraftStateName)
	databaseExists, err := regularFileExists(databasePath)
	if err != nil {
		return err
	}
	legacyExists, err := regularFileExists(legacyPath)
	if err != nil {
		return err
	}
	if databaseExists {
		if !legacyExists {
			return nil
		}
		_, digest, err := readLegacyFlowcraftState(legacyPath)
		if err != nil {
			return err
		}
		return verifyMigrationMarker(ctx, databasePath, digest)
	}
	if !legacyExists {
		return nil
	}

	raw, digest, err := readLegacyFlowcraftState(legacyPath)
	if err != nil {
		return err
	}
	state, err := decodeLegacyFlowcraftState(raw)
	if err != nil {
		return err
	}
	temporaryPath := filepath.Join(dir, flowcraftMigrationTmpName)
	if err := removeMigrationTemporaryFiles(temporaryPath); err != nil {
		return err
	}
	if err := buildMigratedSQLite(ctx, temporaryPath, state, digest, ops); err != nil {
		_ = removeMigrationTemporaryFiles(temporaryPath)
		return err
	}
	if err := syncFile(temporaryPath); err != nil {
		_ = removeMigrationTemporaryFiles(temporaryPath)
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = removeMigrationTemporaryFiles(temporaryPath)
		return fmt.Errorf("memory store: migrate legacy Flowcraft state: %w", err)
	}
	if err := ops.rename(temporaryPath, databasePath); err != nil {
		_ = removeMigrationTemporaryFiles(temporaryPath)
		return fmt.Errorf("memory store: publish migrated Flowcraft SQLite database: %w", err)
	}
	if err := syncDirectory(dir); err != nil {
		return err
	}
	return nil
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("memory store: inspect %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("memory store: expected regular file at %q", path)
	}
	return true, nil
}

func readLegacyFlowcraftState(path string) ([]byte, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("memory store: read legacy Flowcraft state: %w", err)
	}
	digest := sha256.Sum256(raw)
	return raw, hex.EncodeToString(digest[:]), nil
}

func decodeLegacyFlowcraftState(raw []byte) (legacyFlowcraftState, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var state legacyFlowcraftState
	if err := decoder.Decode(&state); err != nil {
		return legacyFlowcraftState{}, fmt.Errorf("memory store: decode legacy Flowcraft state: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return legacyFlowcraftState{}, err
	}
	if state.Version != legacyFlowcraftVersion {
		return legacyFlowcraftState{}, fmt.Errorf("memory store: unsupported legacy Flowcraft state version %d", state.Version)
	}
	if state.Counters == nil {
		state.Counters = map[string]legacyCounterPair{}
	}
	if err := validateLegacyFlowcraftState(state); err != nil {
		return legacyFlowcraftState{}, err
	}
	return state, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("memory store: decode legacy Flowcraft state trailer: %w", err)
	}
	return errors.New("memory store: legacy Flowcraft state contains multiple JSON values")
}

func validateLegacyFlowcraftState(state legacyFlowcraftState) error {
	validStatus := func(status string) bool {
		switch status {
		case "pending", "leased", "failed", "complete":
			return true
		default:
			return false
		}
	}
	for _, fact := range state.Facts {
		if fact.Scope.RuntimeID == "" || fact.ID == "" || !fact.Kind.IsValid() {
			return errors.New("memory store: legacy Flowcraft fact has an invalid runtime scope, id, or kind")
		}
	}
	for _, record := range state.Evidence {
		if record.Scope.RuntimeID == "" || record.FactID == "" || record.EvidenceID == "" {
			return errors.New("memory store: legacy Flowcraft evidence is missing runtime scope, fact id, or evidence id")
		}
	}
	for _, record := range state.SideEffects {
		if record.Job.Scope.RuntimeID == "" || record.Job.ID == "" || record.Job.RequestID == "" || !validStatus(record.Status) {
			return errors.New("memory store: legacy Flowcraft side-effect record is invalid")
		}
	}
	for _, record := range state.Async {
		if record.Job.Scope.RuntimeID == "" || record.Job.RequestID == "" || !validStatus(record.Status) {
			return errors.New("memory store: legacy Flowcraft async-semantic record is invalid")
		}
	}
	for key, counters := range state.Counters {
		if _, _, err := parseLegacyPartitionKey(key); err != nil || counters.SideEffect < 0 || counters.AsyncSemantic < 0 {
			return fmt.Errorf("memory store: invalid legacy Flowcraft counter partition %q", key)
		}
	}
	return nil
}

func buildMigratedSQLite(ctx context.Context, path string, state legacyFlowcraftState, digest string, ops sqliteMigrationOps) error {
	backend, err := flowsqlite.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("memory store: create migration SQLite database: %w", err)
	}
	if err := backend.Close(); err != nil {
		return fmt.Errorf("memory store: close initialized migration database: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("memory store: protect migration SQLite database: %w", err)
	}
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("memory store: reopen migration database: %w", err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		return fmt.Errorf("memory store: configure migration database: %w", err)
	}
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memory store: begin legacy Flowcraft migration: %w", err)
	}
	defer transaction.Rollback()
	if ops.beforeImport != nil {
		if err := ops.beforeImport(); err != nil {
			return fmt.Errorf("memory store: prepare legacy Flowcraft import: %w", err)
		}
	}
	if err := importLegacyFlowcraftState(ctx, transaction, state, digest); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("memory store: commit legacy Flowcraft migration: %w", err)
	}
	if err := ops.verify(ctx, database, state, digest); err != nil {
		return err
	}
	if err := ops.checkpoint(ctx, database); err != nil {
		return err
	}
	if err := database.Close(); err != nil {
		return fmt.Errorf("memory store: close migrated Flowcraft database: %w", err)
	}
	return verifyMigratedPublicReads(ctx, path, state)
}

func checkpointMigratedSQLite(ctx context.Context, database *sql.DB) error {
	if _, err := database.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("memory store: checkpoint migrated Flowcraft database: %w", err)
	}
	return nil
}

func importLegacyFlowcraftState(ctx context.Context, transaction *sql.Tx, state legacyFlowcraftState, digest string) error {
	// The import targets the schema owned by the pinned Flowcraft memory
	// dependency. Keeping the complete import in one transaction is required to
	// preserve queue leases and terminal results that public enqueue APIs would
	// intentionally regenerate or scrub.
	if _, err := transaction.ExecContext(ctx, `CREATE TABLE gizclaw_memory_migrations (
		name text PRIMARY KEY,
		source_version integer NOT NULL,
		source_sha256 text NOT NULL,
		fact_count integer NOT NULL,
		evidence_count integer NOT NULL,
		side_effect_count integer NOT NULL,
		async_count integer NOT NULL,
		counter_partition_count integer NOT NULL,
		completed_at_ns bigint NOT NULL
	)`); err != nil {
		return fmt.Errorf("memory store: create migration marker: %w", err)
	}
	for _, fact := range state.Facts {
		payload, err := json.Marshal(fact)
		if err != nil {
			return fmt.Errorf("memory store: encode migrated fact %q: %w", fact.ID, err)
		}
		var validTo, expiresAt any
		if fact.ValidTo != nil {
			validTo = fact.ValidTo.UnixNano()
		}
		if fact.ExpiresAt != nil {
			expiresAt = fact.ExpiresAt.UnixNano()
		}
		if _, err := transaction.ExecContext(ctx, `INSERT INTO recall_facts(
			runtime_id,user_id,id,kind,observed_at_ns,valid_to_ns,closed,expires_at_ns,merge_key,corrected_by,origin_request_id,payload_json
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, fact.Scope.RuntimeID, fact.Scope.UserID, fact.ID, string(fact.Kind),
			fact.ObservedAt.UnixNano(), validTo, boolInteger(fact.Closed), expiresAt, fact.MergeKey, fact.CorrectedBy,
			fact.Origin.RequestID, string(payload)); err != nil {
			return fmt.Errorf("memory store: import legacy fact %q: %w", fact.ID, err)
		}
		seen := map[string]struct{}{}
		for _, entity := range fact.Entities {
			if entity == "" {
				continue
			}
			if _, exists := seen[entity]; exists {
				continue
			}
			seen[entity] = struct{}{}
			if _, err := transaction.ExecContext(ctx, `INSERT INTO recall_fact_entities(runtime_id,user_id,fact_id,entity) VALUES(?,?,?,?)`,
				fact.Scope.RuntimeID, fact.Scope.UserID, fact.ID, entity); err != nil {
				return fmt.Errorf("memory store: import entity for legacy fact %q: %w", fact.ID, err)
			}
		}
	}
	for _, record := range state.Evidence {
		payload, err := json.Marshal(record.Ref)
		if err != nil {
			return fmt.Errorf("memory store: encode migrated evidence %q: %w", record.EvidenceID, err)
		}
		if _, err := transaction.ExecContext(ctx, `INSERT INTO recall_evidence_refs(runtime_id,user_id,fact_id,evidence_id,ordinal,payload_json) VALUES(?,?,?,?,?,?)`,
			record.Scope.RuntimeID, record.Scope.UserID, record.FactID, record.EvidenceID, record.Ordinal, string(payload)); err != nil {
			return fmt.Errorf("memory store: import legacy evidence %q: %w", record.EvidenceID, err)
		}
	}
	for _, record := range state.SideEffects {
		if err := importLegacySideEffect(ctx, transaction, record); err != nil {
			return err
		}
	}
	for _, record := range state.Async {
		if err := importLegacyAsyncSemantic(ctx, transaction, record); err != nil {
			return err
		}
	}
	for key, counters := range state.Counters {
		runtimeID, userID, _ := parseLegacyPartitionKey(key)
		for _, counter := range []struct {
			kind  string
			value int
		}{{"side_effect", counters.SideEffect}, {"async_semantic", counters.AsyncSemantic}} {
			if _, err := transaction.ExecContext(ctx, `INSERT INTO recall_queue_counters(kind,runtime_id,user_id,cancelled_total) VALUES(?,?,?,?)`,
				counter.kind, runtimeID, userID, counter.value); err != nil {
				return fmt.Errorf("memory store: import legacy %s counter for %q: %w", counter.kind, key, err)
			}
		}
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO gizclaw_memory_migrations(
		name,source_version,source_sha256,fact_count,evidence_count,side_effect_count,async_count,counter_partition_count,completed_at_ns
	) VALUES(?,?,?,?,?,?,?,?,?)`, flowcraftMigrationName, legacyFlowcraftVersion, digest, len(state.Facts), len(state.Evidence),
		len(state.SideEffects), len(state.Async), len(state.Counters), time.Now().UnixNano()); err != nil {
		return fmt.Errorf("memory store: record completed legacy Flowcraft migration: %w", err)
	}
	return nil
}

func importLegacySideEffect(ctx context.Context, transaction *sql.Tx, record legacySideEffectRecord) error {
	payload, err := json.Marshal(record.Job)
	if err != nil {
		return fmt.Errorf("memory store: encode migrated side-effect job %q: %w", record.Job.ID, err)
	}
	failureClass, failureError := string(record.Failure.ErrClass), record.Failure.Err
	result, err := optionalJSON(record.Result, record.Status == "complete")
	if err != nil {
		return err
	}
	var retryAt, leaseUntil any
	if !record.RetryAt.IsZero() {
		retryAt = record.RetryAt.UnixNano()
	}
	if !record.Job.LeaseUntil.IsZero() {
		leaseUntil = record.Job.LeaseUntil.UnixNano()
	}
	_, err = transaction.ExecContext(ctx, `INSERT INTO recall_side_effect_jobs(
		id,request_id,runtime_id,user_id,kind,status,enqueued_at_ns,retry_at_ns,lease_until_ns,lease_token,attempt,failure_class,failure_err,result_json,payload_json
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, record.Job.ID, record.Job.RequestID, record.Job.Scope.RuntimeID,
		record.Job.Scope.UserID, string(record.Job.Kind), record.Status, record.EnqueuedAt.UnixNano(), retryAt, leaseUntil,
		record.Job.LeaseToken, record.Job.Attempt, failureClass, failureError, result, string(payload))
	if err != nil {
		return fmt.Errorf("memory store: import legacy side-effect job %q: %w", record.Job.ID, err)
	}
	return nil
}

func importLegacyAsyncSemantic(ctx context.Context, transaction *sql.Tx, record legacyAsyncSemanticRecord) error {
	payload, err := json.Marshal(record.Job)
	if err != nil {
		return fmt.Errorf("memory store: encode migrated async-semantic job %q: %w", record.Job.RequestID, err)
	}
	result, err := optionalJSON(record.Result, record.Status == "complete")
	if err != nil {
		return err
	}
	var leaseUntil any
	if !record.Job.LeaseUntil.IsZero() {
		leaseUntil = record.Job.LeaseUntil.UnixNano()
	}
	if _, err := transaction.ExecContext(ctx, `INSERT INTO recall_async_semantic_jobs(
		request_id,runtime_id,user_id,status,enqueued_at_ns,lease_until_ns,lease_token,attempt,failure_class,failure_err,result_json,payload_json
	) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, record.Job.RequestID, record.Job.Scope.RuntimeID, record.Job.Scope.UserID,
		record.Status, record.EnqueuedAt.UnixNano(), leaseUntil, record.Job.LeaseToken, record.Job.Attempt,
		string(record.Failure.ErrClass), record.Failure.Err, result, string(payload)); err != nil {
		return fmt.Errorf("memory store: import legacy async-semantic job %q: %w", record.Job.RequestID, err)
	}
	seen := map[string]struct{}{}
	for _, factID := range record.Job.EpisodeFactIDs {
		if factID == "" {
			continue
		}
		if _, exists := seen[factID]; exists {
			continue
		}
		seen[factID] = struct{}{}
		if _, err := transaction.ExecContext(ctx, `INSERT INTO recall_async_semantic_job_episodes(request_id,runtime_id,user_id,episode_fact_id) VALUES(?,?,?,?)`,
			record.Job.RequestID, record.Job.Scope.RuntimeID, record.Job.Scope.UserID, factID); err != nil {
			return fmt.Errorf("memory store: import episode for legacy async-semantic job %q: %w", record.Job.RequestID, err)
		}
	}
	return nil
}

func optionalJSON(value any, include bool) (string, error) {
	if !include {
		return "", nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("memory store: encode migrated terminal result: %w", err)
	}
	return string(payload), nil
}

func verifyMigrationCounts(ctx context.Context, database *sql.DB, state legacyFlowcraftState, digest string) error {
	var version, facts, evidence, sideEffects, async, counters int
	var recordedDigest string
	err := database.QueryRowContext(ctx, `SELECT source_version,source_sha256,fact_count,evidence_count,side_effect_count,async_count,counter_partition_count
		FROM gizclaw_memory_migrations WHERE name = ?`, flowcraftMigrationName).Scan(
		&version, &recordedDigest, &facts, &evidence, &sideEffects, &async, &counters,
	)
	if err != nil {
		return fmt.Errorf("memory store: read completed migration marker: %w", err)
	}
	if version != legacyFlowcraftVersion || recordedDigest != digest || facts != len(state.Facts) || evidence != len(state.Evidence) ||
		sideEffects != len(state.SideEffects) || async != len(state.Async) || counters != len(state.Counters) {
		return errors.New("memory store: completed legacy Flowcraft migration marker does not match imported state")
	}
	for table, want := range map[string]int{
		"recall_facts": len(state.Facts), "recall_evidence_refs": len(state.Evidence),
		"recall_side_effect_jobs": len(state.SideEffects), "recall_async_semantic_jobs": len(state.Async),
	} {
		var got int
		if err := database.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
			return fmt.Errorf("memory store: count migrated %s: %w", table, err)
		}
		if got != want {
			return fmt.Errorf("memory store: migrated %s count = %d, want %d", table, got, want)
		}
	}
	if err := verifyScopeCounts(ctx, database, "recall_facts", countFactsByScope(state.Facts)); err != nil {
		return err
	}
	if err := verifyScopeCounts(ctx, database, "recall_evidence_refs", countEvidenceByScope(state.Evidence)); err != nil {
		return err
	}
	if err := verifyScopeCounts(ctx, database, "recall_side_effect_jobs", countSideEffectsByScope(state.SideEffects)); err != nil {
		return err
	}
	if err := verifyScopeCounts(ctx, database, "recall_async_semantic_jobs", countAsyncByScope(state.Async)); err != nil {
		return err
	}
	if err := verifyScopeStatusCounts(ctx, database, "recall_side_effect_jobs", countSideEffectStatuses(state.SideEffects)); err != nil {
		return err
	}
	if err := verifyScopeStatusCounts(ctx, database, "recall_async_semantic_jobs", countAsyncStatuses(state.Async)); err != nil {
		return err
	}
	if err := verifyCounterValues(ctx, database, state.Counters); err != nil {
		return err
	}
	return nil
}

func verifyScopeStatusCounts(ctx context.Context, database *sql.DB, table string, want map[string]int) error {
	rows, err := database.QueryContext(ctx, "SELECT runtime_id,user_id,status,COUNT(*) FROM "+table+" GROUP BY runtime_id,user_id,status")
	if err != nil {
		return fmt.Errorf("memory store: count migrated %s statuses: %w", table, err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var runtimeID, userID, status string
		var count int
		if err := rows.Scan(&runtimeID, &userID, &status, &count); err != nil {
			return fmt.Errorf("memory store: scan migrated %s status count: %w", table, err)
		}
		got[scopeCountKey(runtimeID, userID)+"\x00"+status] = count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("memory store: read migrated %s status counts: %w", table, err)
	}
	if !equalCountMaps(got, want) {
		return fmt.Errorf("memory store: migrated %s per-scope status counts do not match legacy state", table)
	}
	return nil
}

func verifyScopeCounts(ctx context.Context, database *sql.DB, table string, want map[string]int) error {
	rows, err := database.QueryContext(ctx, "SELECT runtime_id,user_id,COUNT(*) FROM "+table+" GROUP BY runtime_id,user_id")
	if err != nil {
		return fmt.Errorf("memory store: count migrated %s scopes: %w", table, err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var runtimeID, userID string
		var count int
		if err := rows.Scan(&runtimeID, &userID, &count); err != nil {
			return fmt.Errorf("memory store: scan migrated %s scope count: %w", table, err)
		}
		got[scopeCountKey(runtimeID, userID)] = count
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("memory store: read migrated %s scope counts: %w", table, err)
	}
	if !equalCountMaps(got, want) {
		return fmt.Errorf("memory store: migrated %s per-scope counts do not match legacy state", table)
	}
	return nil
}

func verifyCounterValues(ctx context.Context, database *sql.DB, counters map[string]legacyCounterPair) error {
	want := map[string]int{}
	for partition, pair := range counters {
		runtimeID, userID, _ := parseLegacyPartitionKey(partition)
		want["side_effect\x00"+scopeCountKey(runtimeID, userID)] = pair.SideEffect
		want["async_semantic\x00"+scopeCountKey(runtimeID, userID)] = pair.AsyncSemantic
	}
	rows, err := database.QueryContext(ctx, `SELECT kind,runtime_id,user_id,cancelled_total FROM recall_queue_counters`)
	if err != nil {
		return fmt.Errorf("memory store: read migrated queue counters: %w", err)
	}
	defer rows.Close()
	got := map[string]int{}
	for rows.Next() {
		var kind, runtimeID, userID string
		var value int
		if err := rows.Scan(&kind, &runtimeID, &userID, &value); err != nil {
			return fmt.Errorf("memory store: scan migrated queue counter: %w", err)
		}
		got[kind+"\x00"+scopeCountKey(runtimeID, userID)] = value
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("memory store: read migrated queue counter rows: %w", err)
	}
	if !equalCountMaps(got, want) {
		return errors.New("memory store: migrated queue counters do not match legacy state")
	}
	return nil
}

func countFactsByScope(records []flowrecall.TemporalFact) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[scopeCountKey(record.Scope.RuntimeID, record.Scope.UserID)]++
	}
	return counts
}

func countEvidenceByScope(records []legacyEvidenceRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[scopeCountKey(record.Scope.RuntimeID, record.Scope.UserID)]++
	}
	return counts
}

func countSideEffectsByScope(records []legacySideEffectRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[scopeCountKey(record.Job.Scope.RuntimeID, record.Job.Scope.UserID)]++
	}
	return counts
}

func countAsyncByScope(records []legacyAsyncSemanticRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[scopeCountKey(record.Job.Scope.RuntimeID, record.Job.Scope.UserID)]++
	}
	return counts
}

func countSideEffectStatuses(records []legacySideEffectRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[scopeCountKey(record.Job.Scope.RuntimeID, record.Job.Scope.UserID)+"\x00"+record.Status]++
	}
	return counts
}

func countAsyncStatuses(records []legacyAsyncSemanticRecord) map[string]int {
	counts := map[string]int{}
	for _, record := range records {
		counts[scopeCountKey(record.Job.Scope.RuntimeID, record.Job.Scope.UserID)+"\x00"+record.Status]++
	}
	return counts
}

func scopeCountKey(runtimeID, userID string) string {
	return runtimeID + "\x00" + userID
}

func equalCountMaps(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func verifyMigratedPublicReads(ctx context.Context, path string, state legacyFlowcraftState) error {
	backend, err := flowsqlite.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("memory store: reopen migrated Flowcraft database: %w", err)
	}
	fail := func(err error) error {
		return errors.Join(err, backend.Close())
	}
	if len(state.Facts) > 0 {
		want := state.Facts[0]
		got, err := backend.TemporalStore().Get(ctx, want.Scope, want.ID)
		if err != nil {
			return fail(fmt.Errorf("memory store: verify migrated fact %q: %w", want.ID, err))
		}
		equal, err := equalJSON(want, got)
		if err != nil {
			return fail(fmt.Errorf("memory store: compare migrated fact %q: %w", want.ID, err))
		}
		if !equal {
			return fail(fmt.Errorf("memory store: migrated fact %q changed during import", want.ID))
		}
	}
	if len(state.Evidence) > 0 {
		want := state.Evidence[0]
		got, err := backend.EvidenceStore().Get(ctx, want.Scope, want.EvidenceID)
		if err != nil {
			return fail(fmt.Errorf("memory store: verify migrated evidence %q: %w", want.EvidenceID, err))
		}
		equal, err := equalJSON(want.Ref, got)
		if err != nil {
			return fail(fmt.Errorf("memory store: compare migrated evidence %q: %w", want.EvidenceID, err))
		}
		if !equal {
			return fail(fmt.Errorf("memory store: migrated evidence %q changed during import", want.EvidenceID))
		}
	}
	if err := backend.Close(); err != nil {
		return fmt.Errorf("memory store: close verified migration database: %w", err)
	}
	return nil
}

func equalJSON(left, right any) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}

func verifyMigrationMarker(ctx context.Context, path, digest string) error {
	database, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("memory store: open Flowcraft migration marker: %w", err)
	}
	defer database.Close()
	var version int
	var recordedDigest string
	err = database.QueryRowContext(ctx, `SELECT source_version,source_sha256 FROM gizclaw_memory_migrations WHERE name = ?`, flowcraftMigrationName).Scan(&version, &recordedDigest)
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(fmt.Sprint(err), "no such table") {
		return errors.New("memory store: legacy state.json coexists with an unmarked memory.db")
	}
	if err != nil {
		return fmt.Errorf("memory store: verify legacy Flowcraft migration marker: %w", err)
	}
	if version != legacyFlowcraftVersion || recordedDigest != digest {
		return errors.New("memory store: legacy state.json digest does not match the completed SQLite migration")
	}
	return nil
}

func parseLegacyPartitionKey(key string) (string, string, error) {
	if runtimeID, ok := strings.CutSuffix(key, "/global"); ok {
		if runtimeID != "" {
			return runtimeID, "", nil
		}
	}
	if index := strings.LastIndex(key, "/u:"); index > 0 && index+3 < len(key) {
		return key[:index], key[index+3:], nil
	}
	return "", "", errors.New("invalid Flowcraft partition key")
}

func boolInteger(value bool) int {
	if value {
		return 1
	}
	return 0
}

func removeMigrationTemporaryFiles(path string) error {
	for _, candidate := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(candidate); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("memory store: remove interrupted migration file %q: %w", candidate, err)
		}
	}
	return nil
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("memory store: open migrated database for sync: %w", err)
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return fmt.Errorf("memory store: sync migrated database: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("memory store: open Flowcraft binding directory for sync: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("memory store: sync Flowcraft binding directory: %w", err)
	}
	return nil
}
