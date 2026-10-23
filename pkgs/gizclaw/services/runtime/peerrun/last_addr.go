package peerrun

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
)

func addLastAddrColumn(ctx context.Context, db *sqlx.DB) error {
	_, err := db.ExecContext(ctx, `ALTER TABLE peer_runs ADD COLUMN last_addr TEXT`)
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "duplicate column") || strings.Contains(message, "already exists") {
		return nil
	}
	return fmt.Errorf("peerrun: add last_addr column: %w", err)
}

// GetLastAddr reads the most recent accepted connection's signaling source
// address, including its port. It remains available while the Peer is offline.
// A Peer without an observed IP endpoint returns an empty address.
func (s *Server) GetLastAddr(ctx context.Context, publicKey giznet.PublicKey) (string, error) {
	db, err := s.database()
	if err != nil {
		return "", err
	}
	if publicKey.IsZero() {
		return "", ErrInvalidPublicKey
	}
	var stored sql.NullString
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT last_addr FROM peer_runs WHERE public_key=?`), publicKey.String()).Scan(&stored)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("peerrun: read last address: %w", err)
	}
	return stored.String, nil
}

// RecordLastAddr saves a signaling source IP and port. The caller must serialize
// this write with connection publication for the same Peer. Closing connections
// must not write addresses, because their activity can outlive a replacement.
func (s *Server) RecordLastAddr(ctx context.Context, publicKey giznet.PublicKey, address string) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if publicKey.IsZero() {
		return ErrInvalidPublicKey
	}
	endpoint, err := netip.ParseAddrPort(address)
	if err != nil || endpoint.Port() == 0 {
		return errors.New("peerrun: invalid signaling source address")
	}
	endpoint = netip.AddrPortFrom(endpoint.Addr().Unmap().WithZone(""), endpoint.Port())
	_, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,last_addr) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET last_addr=excluded.last_addr`), publicKey.String(), endpoint.String())
	if err != nil {
		return fmt.Errorf("peerrun: record last address: %w", err)
	}
	return nil
}
