package gizclaw

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestSocialRecoveryRetriesIndependentlyAndJoinsOnClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var blockedCalls, retryCalls atomic.Int64
		r := &socialRecovery{
			interval: 30 * time.Second,
			tasks: []socialRecoveryTask{
				{kind: "blocked", reconcile: func(ctx context.Context) error {
					blockedCalls.Add(1)
					<-ctx.Done()
					return ctx.Err()
				}},
				{kind: "retry", reconcile: func(context.Context) error {
					if retryCalls.Add(1) == 1 {
						return errors.New("temporary storage failure")
					}
					return nil
				}},
			},
		}
		defer r.close()
		r.start(t.Context())
		r.start(t.Context())
		synctest.Wait()
		if blockedCalls.Load() != 1 || retryCalls.Load() != 1 {
			t.Fatalf("initial attempts: blocked=%d retry=%d", blockedCalls.Load(), retryCalls.Load())
		}
		time.Sleep(r.interval)
		synctest.Wait()
		if retryCalls.Load() != 2 {
			t.Fatalf("attempts while another kind is blocked = %d", retryCalls.Load())
		}
		r.close()
		r.close()
		time.Sleep(r.interval)
		synctest.Wait()
		if retryCalls.Load() != 2 {
			t.Fatal("worker continued after Close")
		}
		r.start(t.Context())
		synctest.Wait()
		if blockedCalls.Load() != 2 || retryCalls.Load() != 3 {
			t.Fatal("workers did not restart after Close")
		}
		r.close()
	})
}

type blockingSocialRecoveryStore struct {
	*kv.Memory
	entered chan struct{}
	active  atomic.Int64
}

func (s *blockingSocialRecoveryStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	if !slices.Contains(key, "social-recovery-index") {
		return s.Memory.ListMembers(ctx, key)
	}
	s.active.Add(1)
	defer s.active.Add(-1)
	s.entered <- struct{}{}
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestServerListenDoesNotWaitForSocialRecovery(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	store := &blockingSocialRecoveryStore{Memory: kv.NewMemory(nil), entered: make(chan struct{}, 3)}
	t.Cleanup(func() { _ = store.Close() })
	server := completeTestServer(t, &Server{
		LocalStatic:      *keyPair,
		FriendStore:      store,
		FriendGroupStore: store,
		PeerListeners:    []giznet.Listener{newTestGiznetListener()},
	})
	listened := make(chan error, 1)
	go func() { listened <- server.Listen() }()
	select {
	case err := <-listened:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Listen waited for recovery discovery")
	}
	for range 3 {
		select {
		case <-store.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("Social recovery did not start after Listen")
		}
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	if store.active.Load() != 0 {
		t.Fatal("Close did not join recovery scans")
	}
}
