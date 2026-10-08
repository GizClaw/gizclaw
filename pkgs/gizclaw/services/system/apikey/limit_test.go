package apikey

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestPeerAPIKeyLimitAndRevokeReleasesCapacity(t *testing.T) {
	server := NewServer(kv.NewMemory(nil))
	owner := testOwner(t)
	var first Created
	for i := range PeerAPIKeyLimit {
		created, err := server.Create(t.Context(), owner, "key", i == 0)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = created
		}
	}
	if _, err := server.Create(t.Context(), owner, "eleventh", false); !errors.Is(err, ErrPeerAPIKeyLimit) {
		t.Fatalf("Create(11th) = %v", err)
	}
	page, err := server.ListOwner(t.Context(), owner, "", maxListLimit)
	if err != nil || len(page.Items) != PeerAPIKeyLimit {
		t.Fatalf("ListOwner = %d keys, %v", len(page.Items), err)
	}
	if _, err := server.Create(t.Context(), testOwner(t), "other owner", false); err != nil {
		t.Fatalf("independent owner: %v", err)
	}
	if err := server.RevokeOwner(t.Context(), owner, first.Key.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Create(t.Context(), owner, "replacement", true); err != nil {
		t.Fatalf("released capacity: %v", err)
	}
	if err := server.CleanupPeer(t.Context(), owner); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Create(t.Context(), owner, "retired", false); !errors.Is(err, ErrOwnerRetired) {
		t.Fatalf("retired owner: %v", err)
	}
}

type concurrentAPIKeyLimitStore struct {
	kv.Store
	entered chan struct{}
	release chan struct{}
}

func (s concurrentAPIKeyLimitStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.entered <- struct{}{}
	select {
	case <-s.release:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	return s.Store.ApplyMutation(ctx, mutation)
}

func TestPeerAPIKeyLimitAcrossConcurrentServers(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	store := kv.Prefixed(kv.NewMemory(nil), kv.Key{"api-keys"})
	owner := testOwner(t)
	seed := NewServer(store)
	for range PeerAPIKeyLimit - 1 {
		if _, err := seed.Create(t.Context(), owner, "existing", false); err != nil {
			t.Fatal(err)
		}
	}
	hook := concurrentAPIKeyLimitStore{Store: store, entered: make(chan struct{}, 2), release: make(chan struct{})}
	results := make(chan error, 2)
	for range 2 {
		server := NewServer(hook)
		go func() { _, err := server.Create(ctx, owner, "concurrent", false); results <- err }()
	}
	for range 2 {
		select {
		case <-hook.entered:
		case <-ctx.Done():
			t.Fatal("both Servers did not reach the shared capacity check")
		}
	}
	close(hook.release)
	winners, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrPeerAPIKeyLimit):
			rejected++
		default:
			t.Fatalf("Create = %v", err)
		}
	}
	if winners != 1 || rejected != 1 {
		t.Fatalf("winners=%d rejected=%d", winners, rejected)
	}
	page, err := seed.ListOwner(t.Context(), owner, "", maxListLimit)
	if err != nil || len(page.Items) != PeerAPIKeyLimit {
		t.Fatalf("concurrent total = %d, %v", len(page.Items), err)
	}
}
