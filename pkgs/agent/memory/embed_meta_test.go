package memory

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestEmbedMetaModelMismatch(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})
	fs := newTestObjectStore(t)

	baseEmb := newMockEmbedder()
	if _, err := NewHost(ctx, HostConfig{Store: store, Embedder: baseEmb, ObjectStore: fs, Separator: testSep}); err != nil {
		t.Fatalf("seed host with base embedder: %v", err)
	}

	differentModel := newMockEmbedder()
	differentModel.model = "another-model"
	if _, err := NewHost(ctx, HostConfig{Store: store, Embedder: differentModel, ObjectStore: fs, Separator: testSep}); err == nil {
		t.Fatal("expected model mismatch error, got nil")
	}
}

func TestEmbedMetaDimensionMismatch(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})
	fs := newTestObjectStore(t)

	baseEmb := newMockEmbedder()
	if _, err := NewHost(ctx, HostConfig{Store: store, Embedder: baseEmb, ObjectStore: fs, Separator: testSep}); err != nil {
		t.Fatalf("seed host with base embedder: %v", err)
	}

	differentDim := newMockEmbedder()
	differentDim.dim = 4
	if _, err := NewHost(ctx, HostConfig{Store: store, Embedder: differentDim, ObjectStore: fs, Separator: testSep}); err == nil {
		t.Fatal("expected dimension mismatch error, got nil")
	}
}

func TestOpenWithEmbedderModelMismatchAgainstHost(t *testing.T) {
	ctx := context.Background()
	store := mustBadgerInMemory(t, &kv.Options{Separator: testSep})

	hostEmb := newMockEmbedder()
	host, err := NewHost(ctx, HostConfig{Store: store, Embedder: hostEmb, ObjectStore: newTestObjectStore(t), Separator: testSep})
	if err != nil {
		t.Fatalf("new host: %v", err)
	}

	mismatch := newMockEmbedder()
	mismatch.model = "other-model"
	if _, err := host.Open("persona-a", WithEmbedder(mismatch)); err == nil {
		t.Fatal("expected model mismatch on WithEmbedder override, got nil")
	}
}
