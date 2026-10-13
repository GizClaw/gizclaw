package gizclaw

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestHistoryRetentionDeletesExpiredAssetsWithoutWrites(t *testing.T) {
	store := newTestWorkspaceHistoryLog(t)
	objects := newTestObjectStore(t)
	expiry := time.Now().UTC().Add(-time.Hour)
	for _, entry := range []memory.HistoryEntry{
		{ConversationID: "idle", ID: "expired", Role: memory.RoleUser, CreatedAt: expiry, ExpiresAt: &expiry},
		{ConversationID: "active", ID: "live", Role: memory.RoleUser, CreatedAt: time.Now().UTC()},
	} {
		name := entry.ConversationID + "/assets/" + entry.ID + ".ogg"
		entry.Assets = []memory.HistoryAsset{{Name: name, MIMEType: "audio/ogg", Bytes: 5}}
		if err := objects.Put(name, strings.NewReader("audio")); err != nil {
			t.Fatal(err)
		}
		if err := store.Append(t.Context(), []memory.HistoryEntry{entry}); err != nil {
			t.Fatal(err)
		}
	}
	r := newHistoryRetention(objects, store)
	r.interval = time.Millisecond
	r.start(t.Context())
	defer r.close()
	root, _ := objects.LocalDir()
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		page, err := store.QueryCleanup(t.Context(), memory.HistoryCleanupQuery{Before: time.Now().UTC(), Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Entries) == 0 {
			break
		}
		select {
		case <-timeout.C:
			t.Fatal("idle binary assets were not reclaimed")
		case <-ticker.C:
		}
	}
	if _, err := os.Stat(filepath.Join(root, "idle/assets/expired.ogg")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expired binary was not physically removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "active/assets/live.ogg")); err != nil {
		t.Fatalf("retention removed live binary: %v", err)
	}
}
