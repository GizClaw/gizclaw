package memory

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestHistoryCleanupPagesSurviveContentRetention(t *testing.T) {
	store := newHistoryTestStore(t)
	now := time.Now().UTC()
	expiry := now.Add(-time.Hour)
	var entries []HistoryEntry
	for i := range 205 {
		conversation, id := fmt.Sprintf("workspace-%d", i%2), fmt.Sprintf("message-%03d", i)
		entries = append(entries, HistoryEntry{ConversationID: conversation, ID: id, Role: RoleUser, CreatedAt: expiry.Add(-time.Hour), ExpiresAt: &expiry,
			Assets: []HistoryAsset{{Name: conversation + "/" + id, MIMEType: "audio/ogg", Bytes: 42}}})
	}
	live := HistoryEntry{ConversationID: "workspace-0", ID: "live", Role: RoleUser, CreatedAt: now}
	entries = append(entries, live)
	if err := store.Append(t.Context(), entries); err != nil {
		t.Fatal(err)
	}
	if err := store.Maintain(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.QueryCleanup(t.Context(), HistoryCleanupQuery{Limit: 10}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unscoped cleanup without cutoff succeeded: %v", err)
	}
	deleted := map[HistoryKey]bool{}
	if err := CleanupHistory(t.Context(), store, HistoryCleanupQuery{Before: now}, func(key HistoryKey, asset HistoryAsset) error {
		if deleted[key] || asset.Name != key.ConversationID+"/"+key.ID {
			t.Fatalf("duplicated or mismatched cleanup: %+v %+v", key, asset)
		}
		deleted[key] = true
		return nil
	}); err != nil || len(deleted) != 205 {
		t.Fatalf("cleanup skipped a page after deletion: count=%d err=%v", len(deleted), err)
	}
	if _, err := store.Get(t.Context(), live.Key()); err != nil {
		t.Fatalf("cleanup removed live history: %v", err)
	}
	page, err := store.QueryCleanup(t.Context(), HistoryCleanupQuery{Before: now, Limit: 1})
	if err != nil || len(page.Entries) != 0 {
		t.Fatalf("cleanup left pending references: %+v %v", page, err)
	}
}
