package peerrun

import (
	"context"
	"fmt"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// RememberPeer records a peer encountered by this Server. It does not alter
// runtime status or agent selection. Shared registration remains authoritative.
func (s *Server) RememberPeer(ctx context.Context, publicKey giznet.PublicKey, createdAt time.Time) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if publicKey.IsZero() {
		return ErrInvalidPublicKey
	}
	if createdAt.IsZero() {
		return fmt.Errorf("peerrun: registration timestamp is required")
	}
	_, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,registered_at) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET registered_at=excluded.registered_at WHERE peer_runs.registered_at IS NULL OR peer_runs.registered_at<>excluded.registered_at`), publicKey.String(), createdAt.UTC().Format("2006-01-02T15:04:05.000000000Z"))
	return err
}

// ListPeerPublicKeys enumerates only this Server's directory in creation order.
// The public-key cursor is resolved locally; an unknown cursor returns no rows.
func (s *Server) ListPeerPublicKeys(ctx context.Context, cursor string, limit int) ([]string, bool, error) {
	db, err := s.database()
	if err != nil {
		return nil, false, err
	}
	if limit < 1 || limit > 200 {
		return nil, false, fmt.Errorf("peerrun: directory limit must be between 1 and 200")
	}
	query := `SELECT public_key FROM peer_runs WHERE registered_at IS NOT NULL`
	args := []any{}
	if cursor != "" {
		query += ` AND (registered_at,public_key) > (SELECT registered_at,public_key FROM peer_runs WHERE public_key=?)`
		args = append(args, cursor)
	}
	query += ` ORDER BY registered_at,public_key LIMIT ?`
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, db.Rebind(query), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	keys := make([]string, 0, limit+1)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, false, err
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	more := len(keys) > limit
	if more {
		keys = keys[:limit]
	}
	return keys, more, nil
}
