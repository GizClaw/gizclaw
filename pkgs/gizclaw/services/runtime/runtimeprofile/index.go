package runtimeprofile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// Source streams complete persisted profiles into a new runtime snapshot.
type Source interface {
	ForEachProfile(context.Context, func(apitypes.RuntimeProfile) error) error
}

// Index owns one published, read-only in-memory SQLite snapshot at a time.
type Index struct {
	source          Source
	refreshInterval time.Duration
	memoryIndexMu   sync.RWMutex
	memoryIndex     *sqlx.DB
	indexRefreshMu  sync.Mutex
	indexClosed     bool
	rotationMu      sync.Mutex
	indexCancel     context.CancelFunc
	indexDone       chan struct{}
	generation      uint64
}

// New creates a runtime index backed by in-memory SQLite.
func New(source Source, refreshInterval time.Duration) *Index {
	return &Index{source: source, refreshInterval: refreshInterval}
}

// Initialize publishes the first snapshot and starts periodic rotation.
func (s *Index) Initialize(ctx context.Context) error {
	if err := s.Refresh(ctx); err != nil {
		return err
	}
	s.startRotation()
	return nil
}

// Generation increases when a new immutable SQLite instance is published.
func (s *Index) Generation() uint64 {
	if s == nil {
		return 0
	}
	s.memoryIndexMu.RLock()
	defer s.memoryIndexMu.RUnlock()
	return s.generation
}

// Entry is one RuntimeProfile binding or configuration item in the
// process-local SQLite index. Value is the item's JSON representation.
type Entry struct {
	RuntimeProfileID string
	Kind             string
	Name             string
	Value            json.RawMessage
}

// ErrStale means the persisted Profile revision changed while a
// caller was selecting an immutable memory snapshot.
var ErrStale = errors.New("RuntimeProfile memory index revision changed")

// GetEntry resolves one decomposed entry by Profile ID, kind, and name.
// A missing entry returns sql.ErrNoRows.
func (s *Index) GetEntry(ctx context.Context, profileID, kind, name string) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("RuntimeProfile runtime index is nil")
	}
	s.memoryIndexMu.RLock()
	defer s.memoryIndexMu.RUnlock()
	if s.memoryIndex == nil {
		return Entry{}, errors.New("RuntimeProfile memory index is not initialized")
	}
	var item Entry
	var value string
	err := s.memoryIndex.QueryRowContext(ctx,
		`SELECT runtime_profile_id,kind,name,value_json FROM entries WHERE runtime_profile_id=? AND kind=? AND name=?`,
		profileID, kind, name,
	).Scan(&item.RuntimeProfileID, &item.Kind, &item.Name, &value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Entry{}, sql.ErrNoRows
		}
		return Entry{}, err
	}
	item.Value = json.RawMessage(value)
	return item, nil
}

// ListProfileIDs lists every RuntimeProfile represented in the memory index.
func (s *Index) ListProfileIDs(ctx context.Context) ([]string, error) {
	if s == nil {
		return nil, errors.New("RuntimeProfile runtime index is nil")
	}
	s.memoryIndexMu.RLock()
	defer s.memoryIndexMu.RUnlock()
	if s.memoryIndex == nil {
		return nil, errors.New("RuntimeProfile memory index is not initialized")
	}
	rows, err := s.memoryIndex.QueryContext(ctx, `SELECT runtime_profile_id FROM profiles ORDER BY runtime_profile_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Refresh rebuilds the process-local, decomposed index from every
// persisted RuntimeProfile. Call it after changes made by another Server.
func (s *Index) Refresh(ctx context.Context) error {
	if s == nil {
		return errors.New("RuntimeProfile runtime index is nil")
	}
	s.indexRefreshMu.Lock()
	defer s.indexRefreshMu.Unlock()
	if s.indexClosed {
		return errors.New("RuntimeProfile memory index is closed")
	}
	if s.source == nil {
		return errors.New("RuntimeProfile source is not configured")
	}
	index, err := sqlx.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	index.SetMaxOpenConns(1)
	index.SetMaxIdleConns(1)
	defer func() {
		if index != nil {
			_ = index.Close()
		}
	}()
	for _, statement := range []string{
		`PRAGMA temp_store=MEMORY`,
		`PRAGMA journal_mode=MEMORY`,
		`CREATE TABLE profiles (runtime_profile_id TEXT PRIMARY KEY, revision TEXT NOT NULL)`,
		`CREATE TABLE entries (runtime_profile_id TEXT NOT NULL, kind TEXT NOT NULL, name TEXT NOT NULL, value_json TEXT NOT NULL, PRIMARY KEY(runtime_profile_id, kind, name))`,
		`CREATE TABLE workflow_tags (runtime_profile_id TEXT NOT NULL, name TEXT NOT NULL, tag TEXT NOT NULL, PRIMARY KEY(runtime_profile_id, name, tag))`,
		`CREATE INDEX workflow_tags_by_tag ON workflow_tags(tag, runtime_profile_id, name)`,
	} {
		if _, err := index.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := s.source.ForEachProfile(ctx, func(profile apitypes.RuntimeProfile) error {
		return insertMemoryProfile(ctx, index, profile)
	}); err != nil {
		return err
	}
	if _, err := index.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		return err
	}
	s.memoryIndexMu.Lock()
	previous := s.memoryIndex
	s.memoryIndex = index
	s.generation++
	s.memoryIndexMu.Unlock()
	index = nil
	if previous != nil {
		return previous.Close()
	}
	return nil
}

func (s *Index) startRotation() {
	s.rotationMu.Lock()
	defer s.rotationMu.Unlock()
	if s.indexCancel != nil {
		return
	}
	interval := s.refreshInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.indexCancel, s.indexDone = cancel, done
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Refresh(ctx); err != nil && ctx.Err() == nil {
					slog.Warn("refresh RuntimeProfile memory index", "error", err)
				}
			}
		}
	}()
}

func insertMemoryProfile(ctx context.Context, index *sqlx.DB, profile apitypes.RuntimeProfile) error {
	tx, err := index.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO profiles(runtime_profile_id,revision) VALUES (?,?)`, profile.Id, profile.Revision); err != nil {
		return err
	}
	add := func(kind, name string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode %s/%s: %w", kind, name, err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO entries(runtime_profile_id,kind,name,value_json) VALUES (?,?,?,?)`, profile.Id, kind, name, string(encoded))
		return err
	}
	for alias, binding := range profile.Spec.Workflows {
		if err := add("workflow", alias, binding); err != nil {
			return err
		}
		if binding.Tags != nil {
			for _, tag := range *binding.Tags {
				if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_tags(runtime_profile_id,name,tag) VALUES (?,?,?)`, profile.Id, alias, tag); err != nil {
					return err
				}
			}
		}
	}
	for _, group := range []struct {
		kind   string
		values *map[string]apitypes.RuntimeProfileBinding
	}{
		{"model", profile.Spec.Resources.Models},
		{"voice", profile.Spec.Resources.Voices},
		{"tool", profile.Spec.Resources.Tools},
	} {
		if group.values != nil {
			for alias, binding := range *group.values {
				if err := add(group.kind, alias, binding); err != nil {
					return err
				}
			}
		}
	}
	if profile.Spec.Resources.Memories != nil {
		for alias, binding := range *profile.Spec.Resources.Memories {
			if err := add("memory", alias, binding); err != nil {
				return err
			}
		}
	}
	if profile.Spec.AppConfig != nil {
		for name, value := range *profile.Spec.AppConfig {
			if err := add("app_config", name, value); err != nil {
				return err
			}
		}
	}
	if profile.Spec.SafetyFences != nil {
		if profile.Spec.SafetyFences.General != nil {
			if err := add("safety_fence", "general", profile.Spec.SafetyFences.General); err != nil {
				return err
			}
		}
		if profile.Spec.SafetyFences.Child != nil {
			if err := add("safety_fence", "child", profile.Spec.SafetyFences.Child); err != nil {
				return err
			}
		}
	}
	if profile.Spec.Mhs != nil && profile.Spec.Mhs.V0 != nil {
		for _, device := range profile.Spec.Mhs.V0.Devices {
			if err := add("mhs.v0.device", device.Id, device); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// ListEntries reads every indexed entry of one kind across all profiles.
// Kinds are workflow, model, voice, tool, memory, app_config, safety_fence,
// and mhs.v0.device. Entries are ordered by profile ID and name.
func (s *Index) ListEntries(ctx context.Context, kind string) ([]Entry, error) {
	if s == nil {
		return nil, errors.New("RuntimeProfile runtime index is nil")
	}
	if !slices.Contains([]string{"workflow", "model", "voice", "tool", "memory", "app_config", "safety_fence", "mhs.v0.device"}, kind) {
		return nil, fmt.Errorf("unsupported RuntimeProfile entry kind %q", kind)
	}
	s.memoryIndexMu.RLock()
	defer s.memoryIndexMu.RUnlock()
	if s.memoryIndex == nil {
		return nil, errors.New("RuntimeProfile memory index is not initialized")
	}
	return queryMemoryEntries(ctx, s.memoryIndex, `SELECT runtime_profile_id,kind,name,value_json FROM entries WHERE kind=? ORDER BY runtime_profile_id,name`, kind)
}

// ListWorkflowsByTags reads Workflow bindings across all profiles. A
// Workflow must contain every requested opaque tag; an empty selector matches all.
func (s *Index) ListWorkflowsByTags(ctx context.Context, tags []string) ([]Entry, error) {
	if s == nil {
		return nil, errors.New("RuntimeProfile runtime index is nil")
	}
	if err := validateMemoryTagSelector(tags); err != nil {
		return nil, err
	}
	s.memoryIndexMu.RLock()
	defer s.memoryIndexMu.RUnlock()
	if s.memoryIndex == nil {
		return nil, errors.New("RuntimeProfile memory index is not initialized")
	}
	return queryMemoryWorkflows(ctx, s.memoryIndex, "", tags)
}

// ListProfileWorkflowsByTags queries the immutable SQLite snapshot for
// one Profile revision. It refreshes once if another Server changed that Profile.
func (s *Index) ListProfileWorkflowsByTags(ctx context.Context, profileID, revision string, tags []string) ([]Entry, error) {
	if s == nil {
		return nil, errors.New("RuntimeProfile runtime index is nil")
	}
	if err := validateMemoryTagSelector(tags); err != nil {
		return nil, err
	}
	for attempt := range 2 {
		s.memoryIndexMu.RLock()
		index := s.memoryIndex
		if index == nil {
			s.memoryIndexMu.RUnlock()
			return nil, errors.New("RuntimeProfile memory index is not initialized")
		}
		var cachedRevision string
		err := index.QueryRowContext(ctx, `SELECT revision FROM profiles WHERE runtime_profile_id=?`, profileID).Scan(&cachedRevision)
		if err == nil && cachedRevision == revision {
			entries, queryErr := queryMemoryWorkflows(ctx, index, profileID, tags)
			s.memoryIndexMu.RUnlock()
			return entries, queryErr
		}
		s.memoryIndexMu.RUnlock()
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if attempt == 0 {
			if err := s.Refresh(ctx); err != nil {
				return nil, err
			}
		}
	}
	return nil, ErrStale
}

func validateMemoryTagSelector(tags []string) error {
	if len(tags) > 32 {
		return errors.New("too many tags (maximum 32)")
	}
	for _, tag := range tags {
		if !utf8.ValidString(tag) || len(tag) == 0 || len(tag) > 128 {
			return fmt.Errorf("invalid workflow tag %q", tag)
		}
	}
	return nil
}

func queryMemoryWorkflows(ctx context.Context, index *sqlx.DB, profileID string, tags []string) ([]Entry, error) {
	profileClause := ""
	var args []any
	if profileID != "" {
		profileClause = " AND e.runtime_profile_id=?"
		args = append(args, profileID)
	}
	if len(tags) == 0 {
		return queryMemoryEntries(ctx, index, `SELECT e.runtime_profile_id,e.kind,e.name,e.value_json FROM entries e WHERE e.kind='workflow'`+profileClause+` ORDER BY e.runtime_profile_id,e.name`, args...)
	}
	unique := append([]string(nil), tags...)
	sort.Strings(unique)
	unique = slices.Compact(unique)
	queryArgs := make([]any, 0, len(args)+len(unique)+1)
	queryArgs = append(queryArgs, args...)
	for _, tag := range unique {
		queryArgs = append(queryArgs, tag)
	}
	query := `SELECT e.runtime_profile_id,e.kind,e.name,e.value_json FROM entries e JOIN workflow_tags t ON t.runtime_profile_id=e.runtime_profile_id AND t.name=e.name WHERE e.kind='workflow'` + profileClause + ` AND t.tag IN (` + strings.TrimSuffix(strings.Repeat("?,", len(unique)), ",") + `) GROUP BY e.runtime_profile_id,e.kind,e.name,e.value_json HAVING COUNT(DISTINCT t.tag)=? ORDER BY e.runtime_profile_id,e.name`
	queryArgs = append(queryArgs, len(unique))
	return queryMemoryEntries(ctx, index, query, queryArgs...)
}

func queryMemoryEntries(ctx context.Context, index *sqlx.DB, query string, args ...any) ([]Entry, error) {
	rows, err := index.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Entry, 0)
	for rows.Next() {
		var item Entry
		var value string
		if err := rows.Scan(&item.RuntimeProfileID, &item.Kind, &item.Name, &value); err != nil {
			return nil, err
		}
		item.Value = json.RawMessage(value)
		result = append(result, item)
	}
	return result, rows.Err()
}

// Close releases the process-local SQLite index. The Source owns persistent storage.
func (s *Index) Close() error {
	if s == nil {
		return nil
	}
	s.rotationMu.Lock()
	cancel, done := s.indexCancel, s.indexDone
	s.indexCancel, s.indexDone = nil, nil
	if cancel != nil {
		cancel()
	}
	s.rotationMu.Unlock()
	if done != nil {
		<-done
	}
	s.indexRefreshMu.Lock()
	defer s.indexRefreshMu.Unlock()
	s.indexClosed = true
	s.memoryIndexMu.Lock()
	index := s.memoryIndex
	s.memoryIndex = nil
	s.memoryIndexMu.Unlock()
	if index != nil {
		return index.Close()
	}
	return nil
}
