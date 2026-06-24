package badge

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestServerPutUploadDownloadAndList(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	srv := &Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
		Now: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}
	item, err := srv.Put(ctx, "badge-a", apitypes.BadgeSpec{Name: "Badge A", Description: "first"})
	if err != nil {
		t.Fatalf("Put error = %v", err)
	}
	if item.Id != "badge-a" || item.Name != "Badge A" || item.IconPath != "" {
		t.Fatalf("Put item = %#v", item)
	}
	icon := []byte("png-ish")
	item, err = srv.UploadIcon(ctx, "badge-a", bytes.NewReader(icon))
	if err != nil {
		t.Fatalf("UploadIcon error = %v", err)
	}
	if item.IconPath != "badge-a/icon" {
		t.Fatalf("UploadIcon path = %q", item.IconPath)
	}
	r, err := srv.DownloadIcon(ctx, "badge-a")
	if err != nil {
		t.Fatalf("DownloadIcon error = %v", err)
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll error = %v", err)
	}
	if !bytes.Equal(got, icon) {
		t.Fatalf("download bytes = %q, want %q", got, icon)
	}

	if _, err := srv.Put(ctx, "badge-b", apitypes.BadgeSpec{Name: "Badge B", Description: "second"}); err != nil {
		t.Fatalf("Put badge-b error = %v", err)
	}
	items, hasNext, next, err := srv.List(ctx, "", 1)
	if err != nil {
		t.Fatalf("List page 1 error = %v", err)
	}
	if len(items) != 1 || !hasNext || next == nil {
		t.Fatalf("List page 1 = items %#v hasNext %v next %v", items, hasNext, next)
	}
	items, hasNext, _, err = srv.List(ctx, *next, 1)
	if err != nil {
		t.Fatalf("List page 2 error = %v", err)
	}
	if len(items) != 1 || hasNext {
		t.Fatalf("List page 2 = items %#v hasNext %v", items, hasNext)
	}
}

func TestServerUploadRequiresExistingBadge(t *testing.T) {
	_, err := (&Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}).UploadIcon(context.Background(), "missing", bytes.NewReader([]byte("icon")))
	if err == nil {
		t.Fatalf("UploadIcon missing badge error = nil, want error")
	}
}

func TestServerGetUpdateDeleteAndConfigurationErrors(t *testing.T) {
	ctx := context.Background()
	srv := &Server{Store: kv.NewMemory(nil), Assets: objectstore.Dir(t.TempDir())}
	if _, err := srv.Put(ctx, " ", apitypes.BadgeSpec{Name: "bad"}); err == nil {
		t.Fatalf("Put blank id error = nil, want error")
	}
	if _, err := srv.Put(ctx, "founder", apitypes.BadgeSpec{}); err == nil {
		t.Fatalf("Put blank name error = nil, want error")
	}
	item, err := srv.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder", Description: "first", IconPath: stringPtr("icons/founder")})
	if err != nil {
		t.Fatalf("Put founder error = %v", err)
	}
	if item.IconPath != "icons/founder" {
		t.Fatalf("Put founder icon path = %q", item.IconPath)
	}
	if _, err := srv.UploadIcon(ctx, "founder", bytes.NewReader([]byte("icon"))); err != nil {
		t.Fatalf("UploadIcon founder error = %v", err)
	}
	got, err := srv.Get(ctx, "founder")
	if err != nil {
		t.Fatalf("Get founder error = %v", err)
	}
	if got.Name != "Founder" || got.IconPath != "icons/founder" {
		t.Fatalf("Get founder = %#v", got)
	}
	updated, err := srv.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder II", Description: "second"})
	if err != nil {
		t.Fatalf("Put update founder error = %v", err)
	}
	if updated.Name != "Founder II" || updated.Description != "second" || updated.IconPath != "icons/founder" {
		t.Fatalf("Put update founder = %#v", updated)
	}
	deleted, err := srv.Delete(ctx, "founder")
	if err != nil {
		t.Fatalf("Delete founder error = %v", err)
	}
	if deleted.Id != "founder" {
		t.Fatalf("Delete founder = %#v", deleted)
	}
	if _, err := srv.Get(ctx, "founder"); err == nil {
		t.Fatalf("Get deleted founder error = nil, want error")
	}
	if _, err := srv.DownloadIcon(ctx, "founder"); err == nil {
		t.Fatalf("Download deleted founder error = nil, want error")
	}

	if _, _, _, err := (&Server{}).List(ctx, "", 0); err == nil {
		t.Fatalf("List without store error = nil, want error")
	}
	if _, err := (&Server{Store: kv.NewMemory(nil)}).UploadIcon(ctx, "founder", bytes.NewReader([]byte("icon"))); err == nil {
		t.Fatalf("UploadIcon without assets error = nil, want error")
	}
}

func stringPtr(value string) *string {
	return &value
}
