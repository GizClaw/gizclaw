package peerrun

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// ErrInvalidDebugMode indicates an unsupported device-owned access mode.
var ErrInvalidDebugMode = errors.New("peerrun: invalid debug mode")

func validDebugMode(mode string) bool {
	return mode == "off" || mode == "readonly" || mode == "fullcontrol"
}

// GetDebugMode reads the durable device runtime setting; missing values mean off.
func (s *Server) GetDebugMode(ctx context.Context, publicKey giznet.PublicKey) (string, error) {
	store, err := s.store()
	if err != nil {
		return "", err
	}
	key, err := key(publicKey, "debug-mode")
	if err != nil {
		return "", err
	}
	data, err := store.Get(ctx, key)
	if errors.Is(err, kv.ErrNotFound) {
		return "off", nil
	}
	if err != nil {
		return "", fmt.Errorf("peerrun: read debug mode: %w", err)
	}
	mode := string(data)
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
	store, err := s.store()
	if err != nil {
		return err
	}
	key, err := key(publicKey, "debug-mode")
	if err != nil {
		return err
	}
	return store.Set(ctx, key, []byte(mode))
}
