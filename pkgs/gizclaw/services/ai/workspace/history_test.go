package workspace

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestHistoryStoreAppendListAndReadAsset(t *testing.T) {
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	base := time.Now().UTC().Truncate(time.Second)
	store.Now = func() time.Time { return base }

	ctx := context.Background()
	gearEntry, err := store.Append(ctx, AppendHistoryRequest{
		Type:      "gear",
		GearID:    "gear-a",
		Name:      "gear",
		Text:      "你好",
		CreatedAt: base,
		Asset:     &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("Append gear: %v", err)
	}
	if len(gearEntry.Assets) != 1 || gearEntry.ExpiresAt == nil {
		t.Fatalf("gear entry asset metadata = %+v", gearEntry)
	}
	if _, err := store.Append(ctx, AppendHistoryRequest{
		Type:      "agent",
		Name:      "agent",
		Text:      "好的",
		CreatedAt: base.Add(time.Second),
	}); err != nil {
		t.Fatalf("Append agent: %v", err)
	}

	limit := 1
	resp, err := store.List(ctx, apitypes.PeerRunHistoryListRequest{Limit: &limit})
	if err != nil {
		t.Fatalf("List first page: %v", err)
	}
	if !resp.Available || !resp.HasNext || resp.NextCursor == nil || len(resp.Items) != 1 {
		t.Fatalf("first page = %+v", resp)
	}
	if item := resp.Items[0]; item.Type != apitypes.PeerRunHistoryEntryTypeGear || item.GearId == nil || *item.GearId != "gear-a" || item.Text != "你好" || !item.ReplayAvailable {
		t.Fatalf("first item = %+v", item)
	}

	resp, err = store.List(ctx, apitypes.PeerRunHistoryListRequest{Cursor: resp.NextCursor, Limit: &limit})
	if err != nil {
		t.Fatalf("List second page: %v", err)
	}
	if resp.HasNext || len(resp.Items) != 1 || resp.Items[0].Type != apitypes.PeerRunHistoryEntryTypeAgent || resp.Items[0].Text != "好的" || !resp.Items[0].ReplayAvailable {
		t.Fatalf("second page = %+v", resp)
	}

	r, err := store.ReadAsset(ctx, gearEntry.Assets[0].Name)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	data, err := io.ReadAll(r)
	if closeErr := r.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "opus" {
		t.Fatalf("asset data = %q", data)
	}
}

func TestHistoryStoreListSupportsDescAndMissingCursorBoundary(t *testing.T) {
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	ctx := context.Background()

	oldest, err := store.Append(ctx, AppendHistoryRequest{Type: "agent", Name: "agent", Text: "oldest", CreatedAt: base})
	if err != nil {
		t.Fatalf("Append oldest: %v", err)
	}
	middle, err := store.Append(ctx, AppendHistoryRequest{Type: "agent", Name: "agent", Text: "middle", CreatedAt: base.Add(time.Second)})
	if err != nil {
		t.Fatalf("Append middle: %v", err)
	}
	newest, err := store.Append(ctx, AppendHistoryRequest{Type: "agent", Name: "agent", Text: "newest", CreatedAt: base.Add(2 * time.Second)})
	if err != nil {
		t.Fatalf("Append newest: %v", err)
	}
	if err := store.Objects.Delete(store.entryObjectName(middle.ID)); err != nil {
		t.Fatalf("Delete middle entry: %v", err)
	}

	limit := 1
	resp, err := store.List(ctx, apitypes.PeerRunHistoryListRequest{Cursor: &middle.ID, Limit: &limit})
	if err != nil {
		t.Fatalf("List asc after missing cursor: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Name != newest.ID {
		t.Fatalf("asc after missing cursor = %+v, want newest %q", resp, newest.ID)
	}

	desc := apitypes.PeerRunHistoryListRequestOrderDesc
	resp, err = store.List(ctx, apitypes.PeerRunHistoryListRequest{Cursor: &middle.ID, Limit: &limit, Order: &desc})
	if err != nil {
		t.Fatalf("List desc before missing cursor: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Name != oldest.ID {
		t.Fatalf("desc before missing cursor = %+v, want oldest %q", resp, oldest.ID)
	}

	resp, err = store.List(ctx, apitypes.PeerRunHistoryListRequest{Limit: &limit, Order: &desc})
	if err != nil {
		t.Fatalf("List latest desc page: %v", err)
	}
	if !resp.HasNext || resp.NextCursor == nil || *resp.NextCursor != newest.ID || len(resp.Items) != 1 || resp.Items[0].Name != newest.ID {
		t.Fatalf("latest desc page = %+v, want first item and next cursor %q", resp, newest.ID)
	}
}

func TestHistoryStoreInternalRangePreservesOriginAndHighWater(t *testing.T) {
	t.Parallel()
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	base := time.Date(2026, 7, 29, 1, 0, 0, 0, time.UTC)
	ctx := context.Background()
	first, err := store.Append(ctx, AppendHistoryRequest{
		Type: "gear", GearID: "peer-a", Origin: HistoryOriginAgentHost,
		Text: "first", CreatedAt: base,
	})
	if err != nil {
		t.Fatalf("Append first: %v", err)
	}
	second, err := store.Append(ctx, AppendHistoryRequest{
		Type: "agent", Origin: HistoryOriginAgentHost,
		Text: "second", CreatedAt: base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("Append second: %v", err)
	}
	third, err := store.Append(ctx, AppendHistoryRequest{
		Type: "agent", Text: "imported", CreatedAt: base.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatalf("Append third: %v", err)
	}
	page, err := store.ListEntries(ctx, first.ID, second.ID, 1)
	if err != nil {
		t.Fatalf("ListEntries() error = %v", err)
	}
	if page.HasNext || page.NextCursor != "" || len(page.Entries) != 1 ||
		page.Entries[0].ID != second.ID ||
		page.Entries[0].Origin != HistoryOriginAgentHost {
		t.Fatalf("ListEntries() = %#v", page)
	}
	latest, ok, err := store.LatestEntry(ctx)
	if err != nil || !ok || latest.ID != third.ID || latest.Origin != "" {
		t.Fatalf("LatestEntry() = %#v, %v, %v", latest, ok, err)
	}
	beforeSecond, ok, err := store.LatestEntryBefore(ctx, second.CreatedAt)
	if err != nil || !ok || beforeSecond.ID != first.ID {
		t.Fatalf("LatestEntryBefore(second) = %#v, %v, %v", beforeSecond, ok, err)
	}
	if entry, ok, err := store.LatestEntryBefore(ctx, first.CreatedAt); err != nil || ok {
		t.Fatalf("LatestEntryBefore(first) = %#v, %v, %v", entry, ok, err)
	}
}

func TestHistoryStoreListRejectsUnsupportedOrder(t *testing.T) {
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	order := apitypes.PeerRunHistoryListRequestOrder("sideways")
	if _, err := store.List(context.Background(), apitypes.PeerRunHistoryListRequest{Order: &order}); err == nil || !strings.Contains(err.Error(), "unsupported order") {
		t.Fatalf("List unsupported order error = %v", err)
	}
}

func TestHistoryStoreValidatesGearAndAgentSource(t *testing.T) {
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	ctx := context.Background()

	if _, err := store.Append(ctx, AppendHistoryRequest{Type: "gear", Name: "gear", Text: "x"}); err == nil || !strings.Contains(err.Error(), "gear_id") {
		t.Fatalf("Append gear without gear_id error = %v", err)
	}
	if _, err := store.Append(ctx, AppendHistoryRequest{Type: "agent", GearID: "gear-a", Name: "agent", Text: "x"}); err == nil || !strings.Contains(err.Error(), "gear_id") {
		t.Fatalf("Append agent with gear_id error = %v", err)
	}
	if _, err := store.Append(ctx, AppendHistoryRequest{Type: "bad", Name: "agent", Text: "x"}); err == nil || !strings.Contains(err.Error(), "unsupported history type") {
		t.Fatalf("Append bad type error = %v", err)
	}
	if _, err := (&HistoryStore{}).Append(ctx, AppendHistoryRequest{Type: "agent", Name: "agent", Text: "x"}); err == nil || !strings.Contains(err.Error(), "object store") {
		t.Fatalf("Append without object store error = %v", err)
	}
}

func TestHistoryStoreMalformedEntryBlocksList(t *testing.T) {
	objects := objectstore.Dir(t.TempDir())
	store := NewHistoryStore(objects, "demo")
	if err := objects.Put(store.entryObjectName("bad"), strings.NewReader(`{"id":`)); err != nil {
		t.Fatalf("Put malformed entry: %v", err)
	}
	if _, err := store.List(context.Background(), apitypes.PeerRunHistoryListRequest{}); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("List malformed error = %v", err)
	}
}

func TestHistoryStoreReadAssetRejectsInvalidNames(t *testing.T) {
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	for _, name := range []string{"", "other/history/assets/a/audio.opus", store.entryObjectName("entry")} {
		if _, err := store.ReadAsset(context.Background(), name); err == nil {
			t.Fatalf("ReadAsset(%q) error = nil", name)
		}
	}
}

func TestHistoryStoreHelpersCoverAssetExtensionsAndValidation(t *testing.T) {
	store := NewHistoryStore(objectstore.Dir(t.TempDir()), "demo")
	for _, tc := range []struct {
		mime string
		want string
	}{
		{mime: "audio/opus", want: ".opus"},
		{mime: "audio/ogg; codecs=opus", want: ".ogg"},
		{mime: "audio/mpeg", want: ".mp3"},
		{mime: "application/octet-stream", want: ".bin"},
	} {
		if got := store.assetObjectName("id", tc.mime); !strings.HasSuffix(got, tc.want) {
			t.Fatalf("assetObjectName(%q) = %q, want suffix %q", tc.mime, got, tc.want)
		}
	}
	for _, entry := range []HistoryEntry{
		{ID: "id", Type: "agent", Name: "agent"},
		{ID: "id", Type: "gear", GearID: "gear", Name: "gear"},
	} {
		if err := validateHistoryEntry(entry); err == nil || !strings.Contains(err.Error(), "created_at") {
			t.Fatalf("validateHistoryEntry(%+v) error = %v", entry, err)
		}
	}
}

func TestHistoryStoreCleanupExpiredRemovesMetadataAndAssets(t *testing.T) {
	objects := objectstore.Dir(t.TempDir())
	store := NewHistoryStore(objects, "demo")
	now := time.Now().UTC()
	store.Now = func() time.Time { return now }
	entry, err := store.Append(context.Background(), AppendHistoryRequest{
		Type:      "agent",
		Name:      "agent",
		Text:      "hello",
		CreatedAt: now,
		Asset:     &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus"), TTL: time.Second},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	store.Now = func() time.Time { return now.Add(2 * time.Second) }
	if err := store.CleanupExpired(context.Background()); err != nil {
		t.Fatalf("CleanupExpired: %v", err)
	}
	if _, err := store.Get(context.Background(), entry.ID); err == nil {
		t.Fatal("Get expired entry error = nil")
	}
	if _, err := store.ReadAsset(context.Background(), entry.Assets[0].Name); err == nil {
		t.Fatal("ReadAsset expired asset error = nil")
	}
	items, err := objects.List(store.historyPrefix())
	if err != nil {
		t.Fatalf("List history prefix: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("history objects after cleanup = %+v", items)
	}
}

func TestHistoryStoreGetExpiredRemovesMetadataAndAssets(t *testing.T) {
	objects := objectstore.Dir(t.TempDir())
	store := NewHistoryStore(objects, "demo")
	now := time.Now().UTC()
	store.Now = func() time.Time { return now }
	entry, err := store.Append(context.Background(), AppendHistoryRequest{
		Type:      "agent",
		Name:      "agent",
		Text:      "hello",
		CreatedAt: now,
		Asset:     &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus"), TTL: time.Second},
	})
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	store.Now = func() time.Time { return now.Add(2 * time.Second) }
	if _, err := store.Get(context.Background(), entry.ID); err == nil {
		t.Fatal("Get expired entry error = nil")
	}
	if _, err := store.ReadAsset(context.Background(), entry.Assets[0].Name); err == nil {
		t.Fatal("ReadAsset expired asset error = nil")
	}
	items, err := objects.List(store.historyPrefix())
	if err != nil {
		t.Fatalf("List history prefix: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("history objects after expired get = %+v", items)
	}
}
