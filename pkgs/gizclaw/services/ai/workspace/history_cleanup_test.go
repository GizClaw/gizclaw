package workspace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

type failingHistoryCleanupObjects struct {
	objectstore.ObjectStore
	fail bool
}

func (s *failingHistoryCleanupObjects) Delete(name string) error {
	if s.fail {
		return errors.New("object deletion unavailable")
	}
	return s.ObjectStore.Delete(name)
}

func TestHistoryCleanupExpiredAssetsAndRetry(t *testing.T) {
	objects := newTestObjectStore(t)
	writes := &failingHistoryCleanupObjects{ObjectStore: objects, fail: true}
	records := newTestLogStore(t).(*memory.SQLHistoryStore)
	store := NewHistoryStore(records, writes, "demo")
	now := time.Now().UTC()
	expiry := now.Add(-time.Hour)
	entries := []memory.HistoryEntry{
		{ConversationID: "demo", ID: "expired", Role: memory.RoleUser, CreatedAt: expiry, ExpiresAt: &expiry},
		{ConversationID: "demo", ID: "live", Role: memory.RoleUser, CreatedAt: now},
		{ConversationID: "other", ID: "expired", Role: memory.RoleUser, CreatedAt: expiry, ExpiresAt: &expiry},
	}
	for i := range entries {
		name := entries[i].ConversationID + "/assets/" + entries[i].ID + ".ogg"
		entries[i].Assets = []memory.HistoryAsset{{Name: name, MIMEType: "audio/ogg", Bytes: 5}}
		if err := objects.Put(name, strings.NewReader("audio")); err != nil {
			t.Fatal(err)
		}
	}
	if err := records.Append(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	// Content reclamation must not erase the only references to binary objects.
	if err := records.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := records.Get(t.Context(), entries[0].Key()); !errors.Is(err, memory.ErrNotFound) {
		t.Fatalf("expired message remained readable: %v", err)
	}
	if err := store.CleanupExpired(t.Context()); err == nil {
		t.Fatal("binary deletion failure was ignored")
	}
	page, err := records.QueryCleanup(t.Context(), memory.HistoryCleanupQuery{ConversationID: "demo", Before: now, Limit: 10})
	if err != nil || len(page.Entries) != 1 || len(page.Entries[0].Assets) != 1 {
		t.Fatalf("failed deletion lost retry references: %+v %v", page, err)
	}
	writes.fail = false
	if err := store.CleanupExpired(t.Context()); err != nil {
		t.Fatal(err)
	}
	root, _ := objects.LocalDir()
	if _, err := os.Stat(filepath.Join(root, "demo/assets/expired.ogg")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expired binary was not physically removed: %v", err)
	}
	for _, name := range []string{"demo/assets/live.ogg", "other/assets/expired.ogg"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("cleanup removed unrelated binary %s: %v", name, err)
		}
	}
	if err := store.DeleteAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "demo/assets/live.ogg")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("DeleteAll kept binary: %v", err)
	}
	if err := store.DeleteAll(t.Context()); err != nil {
		t.Fatalf("DeleteAll was not idempotent: %v", err)
	}
}
