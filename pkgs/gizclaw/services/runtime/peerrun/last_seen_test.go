package peerrun

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestLastSeenPersistsAndOnlyMovesForward(t *testing.T) {
	s := newTestServer(t)
	key := testPublicKey(t)
	if seen, err := s.GetLastSeen(t.Context(), key); err != nil || !seen.IsZero() {
		t.Fatalf("unseen peer = %v %v", seen, err)
	}
	first := time.Date(2026, 9, 6, 10, 0, 0, 123456789, time.UTC)
	if err := s.RecordLastSeen(t.Context(), key, first); err != nil {
		t.Fatal(err)
	}
	reopened := &Server{DB: s.DB}
	if seen, err := reopened.GetLastSeen(t.Context(), key); err != nil || !seen.Equal(first) {
		t.Fatalf("stored = %v %v, want %v", seen, err, first)
	}
	later := first.Add(time.Minute)
	if err := s.RecordLastSeen(t.Context(), key, later); err != nil {
		t.Fatal(err)
	}
	if seen, err := s.GetLastSeen(t.Context(), key); err != nil || !seen.Equal(later) {
		t.Fatalf("advanced = %v %v, want %v", seen, err, later)
	}
	// A late write from a connection that closed earlier must not roll the
	// recorded time back.
	if err := s.RecordLastSeen(t.Context(), key, first); err != nil {
		t.Fatal(err)
	}
	if seen, err := s.GetLastSeen(t.Context(), key); err != nil || !seen.Equal(later) {
		t.Fatalf("after stale write = %v %v, want %v", seen, err, later)
	}
	if err := s.RecordLastSeen(t.Context(), key, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if seen, err := s.GetLastSeen(t.Context(), key); err != nil || !seen.Equal(later) {
		t.Fatalf("after zero write = %v %v, want %v", seen, err, later)
	}
	if _, err := s.GetLastSeen(t.Context(), giznet.PublicKey{}); err != ErrInvalidPublicKey {
		t.Fatalf("empty key get error = %v", err)
	}
	if err := s.RecordLastSeen(t.Context(), giznet.PublicKey{}, later); err != ErrInvalidPublicKey {
		t.Fatalf("empty key record error = %v", err)
	}
}

func TestLastSeenColumnAddedToExistingTable(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.DB.ExecContext(t.Context(), `DROP TABLE peer_runs`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(t.Context(), `CREATE TABLE peer_runs (
 public_key TEXT PRIMARY KEY,
 registered_at TEXT,
 status_json TEXT,
 ota_json TEXT,
 pending_workspace TEXT,
 active_workspace TEXT,
 debug_mode TEXT NOT NULL DEFAULT 'off'
 )`); err != nil {
		t.Fatal(err)
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatalf("Initialize over legacy schema error = %v", err)
	}
	key := testPublicKey(t)
	seen := time.Date(2026, 9, 7, 8, 0, 0, 0, time.UTC)
	if err := s.RecordLastSeen(t.Context(), key, seen); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetLastSeen(t.Context(), key); err != nil || !got.Equal(seen) {
		t.Fatalf("migrated last seen = %v %v, want %v", got, err, seen)
	}
	if err := s.Initialize(t.Context()); err != nil {
		t.Fatalf("repeated Initialize error = %v", err)
	}
}
