package runtimeprofile

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	runtimeindex "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/runtimeprofile"
	"github.com/jmoiron/sqlx"
)

type profileSource struct{ db *sqlx.DB }

func (source profileSource) ForEachProfile(ctx context.Context, consume func(apitypes.RuntimeProfile) error) error {
	rows, err := source.db.QueryContext(ctx, "SELECT "+runtimeProfileColumns+" FROM runtime_profiles ORDER BY id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		profile, _, err := scanRuntimeProfileSQL(rows)
		if err != nil {
			return err
		}
		if err := consume(profile); err != nil {
			return err
		}
	}
	return rows.Err()
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
