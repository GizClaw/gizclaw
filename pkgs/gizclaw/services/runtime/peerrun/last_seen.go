package peerrun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
)

// lastSeenLayout keeps the stored timestamp lexicographically ordered so the
// monotonic comparison below can run inside the upsert statement.
const lastSeenLayout = "2006-01-02T15:04:05.000000000Z"

// addLastSeenColumn adds last_seen_at to a peer_runs table created before the
// column existed. Both supported drivers report an existing column as an
// error, which is the expected outcome for an already-current schema.
func addLastSeenColumn(ctx context.Context, db *sqlx.DB) error {
	_, err := db.ExecContext(ctx, `ALTER TABLE peer_runs ADD COLUMN last_seen_at TEXT`)
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "duplicate column") || strings.Contains(message, "already exists") {
		return nil
	}
	return fmt.Errorf("peerrun: add last_seen_at column: %w", err)
}

// GetLastSeen reads the durable last activity timestamp observed for a Peer.
// A Peer that has never connected reports the zero time.
func (s *Server) GetLastSeen(ctx context.Context, publicKey giznet.PublicKey) (time.Time, error) {
	db, err := s.database()
	if err != nil {
		return time.Time{}, err
	}
	if publicKey.IsZero() {
		return time.Time{}, ErrInvalidPublicKey
	}
	var stored sql.NullString
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT last_seen_at FROM peer_runs WHERE public_key=?`), publicKey.String()).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("peerrun: read last seen: %w", err)
	}
	if !stored.Valid || stored.String == "" {
		return time.Time{}, nil
	}
	seen, err := time.Parse(lastSeenLayout, stored.String)
	if err != nil {
		return time.Time{}, fmt.Errorf("peerrun: decode last seen: %w", err)
	}
	return seen.UTC(), nil
}

// RecordLastSeen persists the last activity timestamp observed for a Peer.
// The stored value only moves forward, so a late write from a closing
// connection cannot roll back a newer one.
func (s *Server) RecordLastSeen(ctx context.Context, publicKey giznet.PublicKey, seen time.Time) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if publicKey.IsZero() {
		return ErrInvalidPublicKey
	}
	if seen.IsZero() {
		return nil
	}
	encoded := seen.UTC().Format(lastSeenLayout)
	_, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,last_seen_at) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET last_seen_at=excluded.last_seen_at WHERE peer_runs.last_seen_at IS NULL OR peer_runs.last_seen_at<excluded.last_seen_at`), publicKey.String(), encoded)
	if err != nil {
		return fmt.Errorf("peerrun: record last seen: %w", err)
	}
	return nil
}
