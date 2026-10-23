package peerrun

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/jmoiron/sqlx"
)

func TestLastAddrSurvivesDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	open := func() *Server {
		t.Helper()
		db, err := sqlx.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		server := &Server{DB: db}
		if err := server.Initialize(t.Context()); err != nil {
			t.Fatal(err)
		}
		return server
	}
	s := open()
	key := testPublicKey(t)
	if address, err := s.GetLastAddr(t.Context(), key); err != nil || address != "" {
		t.Fatalf("unseen address = %q, %v", address, err)
	}
	if err := s.RecordLastAddr(t.Context(), key, "[::ffff:198.51.100.8]:4242"); err != nil {
		t.Fatal(err)
	}
	if err := s.DB.Close(); err != nil {
		t.Fatal(err)
	}
	s = open()
	if address, err := s.GetLastAddr(t.Context(), key); err != nil || address != "198.51.100.8:4242" {
		t.Fatalf("reopened address = %q, %v", address, err)
	}
	seen := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	if err := s.RecordLastSeen(t.Context(), key, seen); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PutStatus(t.Context(), key, apitypes.PeerStatus{Volume: new(42)}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordLastAddr(t.Context(), key, "[2001:db8::1]:5678"); err != nil {
		t.Fatal(err)
	}
	if address, err := s.GetLastAddr(t.Context(), key); err != nil || address != "[2001:db8::1]:5678" {
		t.Fatalf("replacement address = %q, %v", address, err)
	}
	if got, err := s.GetLastSeen(t.Context(), key); err != nil || !got.Equal(seen) {
		t.Fatalf("address write changed last seen = %v, %v", got, err)
	}
	if status, err := s.GetStatus(t.Context(), key); err != nil || status.Volume == nil || *status.Volume != 42 {
		t.Fatalf("address write changed status = %+v, %v", status, err)
	}
	if keys, _, err := s.ListPeerPublicKeys(t.Context(), "", 10); err != nil || len(keys) != 0 {
		t.Fatalf("address write added directory entries = %v, %v", keys, err)
	}
}

func TestLastAddrColumnAddedWithoutChangingExistingRuntime(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.DB.ExecContext(t.Context(), `ALTER TABLE peer_runs DROP COLUMN last_addr`); err != nil {
		t.Fatal(err)
	}
	key := testPublicKey(t)
	if err := s.SetDebugMode(t.Context(), key, "readonly"); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLastAddr(t.Context(), key); err != nil || got != "" {
		t.Fatalf("legacy address = %q, %v", got, err)
	}
	if got, err := s.GetDebugMode(t.Context(), key); err != nil || got != "readonly" {
		t.Fatalf("migration changed debug mode = %q, %v", got, err)
	}
	if err := s.RecordLastAddr(t.Context(), key, "198.51.100.1:1000"); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLastAddr(t.Context(), key); err != nil || got != "198.51.100.1:1000" {
		t.Fatalf("repeated initialization changed address = %q, %v", got, err)
	}
}

func TestLastAddrRejectsInvalidEndpoints(t *testing.T) {
	s := newTestServer(t)
	key := testPublicKey(t)
	for _, address := range []string{"", "gizwebrtc:server:remote", "example.com:443", "192.0.2.1", "192.0.2.1:0", "192.0.2.1:65536", "192.0.2.1:443, 198.51.100.1:80"} {
		if err := s.RecordLastAddr(t.Context(), key, address); err == nil {
			t.Fatalf("accepted invalid endpoint %q", address)
		}
	}
	if got, err := s.GetLastAddr(t.Context(), key); err != nil || got != "" {
		t.Fatalf("invalid write created an address = %q, %v", got, err)
	}
	if _, err := s.GetLastAddr(t.Context(), giznet.PublicKey{}); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("empty key read = %v", err)
	}
	if err := s.RecordLastAddr(t.Context(), giznet.PublicKey{}, "192.0.2.1:443"); !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("empty key write = %v", err)
	}
}
