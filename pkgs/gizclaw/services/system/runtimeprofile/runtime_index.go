package runtimeprofile

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimeindex "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/runtimeprofile"
	"github.com/jmoiron/sqlx"
)

type profileSource struct{ db *sqlx.DB }

const profileSourceBatchSize = 64

func (source profileSource) ForEachProfile(ctx context.Context, consume func(apitypes.RuntimeProfile) error) error {
	var upperID sql.NullString
	if err := source.db.QueryRowContext(ctx, `SELECT MAX(id) FROM runtime_profiles`).Scan(&upperID); err != nil {
		return err
	}
	if !upperID.Valid {
		return ctx.Err()
	}
	lastID := ""
	for {
		profiles, err := source.readBatch(ctx, lastID, upperID.String)
		if err != nil {
			return err
		}
		// The persistent SQL lease ends before the consumer builds its memory
		// index. A slow callback must not block unrelated authoritative reads.
		for _, profile := range profiles {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := consume(profile); err != nil {
				return err
			}
			lastID = profile.Id
		}
		if len(profiles) < profileSourceBatchSize {
			return ctx.Err()
		}
	}
}

func (source profileSource) readBatch(ctx context.Context, lastID, upperID string) ([]apitypes.RuntimeProfile, error) {
	rows, err := source.db.QueryContext(ctx, source.db.Rebind("SELECT "+runtimeProfileColumns+" FROM runtime_profiles WHERE id > ? AND id <= ? ORDER BY id LIMIT ?"), lastID, upperID, profileSourceBatchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]apitypes.RuntimeProfile, 0, profileSourceBatchSize)
	for rows.Next() {
		profile, _, err := scanRuntimeProfileSQL(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (s *Server) runtimeIndexOrError() (*runtimeindex.Index, error) {
	if s == nil || s.runtimeIndex == nil {
		return nil, errors.New("RuntimeProfile runtime index is not initialized")
	}
	return s.runtimeIndex, nil
}

func (s *Server) refreshIndexIfInitialized(ctx context.Context) error {
	if s == nil || s.runtimeIndex == nil {
		return nil
	}
	return s.runtimeIndex.Refresh(ctx)
}

func (s *Server) refreshIndexAfterCommit(ctx context.Context, profileID string) {
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.refreshIndexIfInitialized(refreshCtx); err != nil {
		// SQL has committed. Readers repair a stale profile revision on demand;
		// the periodic rotation retries global catalog refreshes.
		slog.WarnContext(ctx, "RuntimeProfile index refresh deferred after commit", "profile_id", profileID, "error", err)
	}
}

// RefreshMemoryIndex rotates the read-only runtime SQLite snapshot.
func (s *Server) RefreshMemoryIndex(ctx context.Context) error {
	index, err := s.runtimeIndexOrError()
	if err != nil {
		return err
	}
	return index.Refresh(ctx)
}

// RuntimeIndex returns the read-only runtime package that serves indexed queries.
func (s *Server) RuntimeIndex() *runtimeindex.Index {
	if s == nil {
		return nil
	}
	return s.runtimeIndex
}

// Close releases the runtime snapshot; the persistent DB remains caller-owned.
func (s *Server) Close() error {
	if s == nil || s.runtimeIndex == nil {
		return nil
	}
	return s.runtimeIndex.Close()
}
