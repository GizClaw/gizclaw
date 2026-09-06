package peerrun

import (
	"context"
	"errors"
	"fmt"

	"database/sql"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

// ErrInvalidDebugMode indicates an unsupported device-owned access mode.
var ErrInvalidDebugMode = errors.New("peerrun: invalid debug mode")

func validDebugMode(mode string) bool {
	return mode == "off" || mode == "readonly" || mode == "fullcontrol"
}

// GetDebugMode reads the durable device runtime setting; missing values mean off.
func (s *Server) GetDebugMode(ctx context.Context, publicKey giznet.PublicKey) (string, error) {
	db, err := s.database()
	if err != nil {
		return "", err
	}
	if publicKey.IsZero() {
		return "", ErrInvalidPublicKey
	}
	var mode string
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT debug_mode FROM peer_runs WHERE public_key=?`), publicKey.String()).Scan(&mode)
	if errors.Is(err, sql.ErrNoRows) {
		return "off", nil
	}
	if err != nil {
		return "", fmt.Errorf("peerrun: read debug mode: %w", err)
	}

	if !validDebugMode(mode) {
		return "", ErrInvalidDebugMode
	}
	return mode, nil
}

// SetDebugMode persists only this device's runtime debug setting.
func (s *Server) SetDebugMode(ctx context.Context, publicKey giznet.PublicKey, mode string) error {
	if !validDebugMode(mode) {
		return ErrInvalidDebugMode
	}
	db, err := s.database()
	if err != nil {
		return err
	}
	if publicKey.IsZero() {
		return ErrInvalidPublicKey
	}
	_, err = db.ExecContext(ctx, db.Rebind(`INSERT INTO peer_runs(public_key,debug_mode) VALUES (?,?) ON CONFLICT(public_key) DO UPDATE SET debug_mode=excluded.debug_mode`), publicKey.String(), mode)
	return err
}
