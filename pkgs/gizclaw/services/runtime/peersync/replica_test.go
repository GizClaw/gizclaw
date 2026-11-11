package peersync

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/kvfixture"
)

type checkpointReadWriteStore struct {
	kv.Store
	authority kv.Store
	before    func()
	attempts  *atomic.Int32
}

func (s checkpointReadWriteStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	if s.attempts != nil {
		s.attempts.Add(1)
	}
	if s.before != nil {
		s.before()
	}
	return s.authority.ApplyMutation(ctx, mutation)
}

func TestSyncResumesCommittedCheckpointWhileReplicaHasNoHead(t *testing.T) {
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	server := &Server{Store: kvfixture.ReadView(authority, replica, nil, nil)}
	owner := giznet.PublicKey{91}
	snapshot := Snapshot{"runtime": json.RawMessage(`{"online":true}`)}
	first, err := server.Sync(t.Context(), owner, 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.Sync(t.Context(), owner, first.Timestamp, snapshot)
	if err != nil || second.Reset || len(second.Upserts) != 0 || second.Timestamp <= first.Timestamp {
		t.Fatalf("committed checkpoint could not resume with a lagging replica: %+v %v", second, err)
	}
	if _, err := replica.Get(t.Context(), stateKey(owner, "head")); !errors.Is(err, kv.ErrNotFound) {
		t.Fatal("replica unexpectedly received the checkpoint head")
	}
}

func TestCheckpointStaleHeadRetriesPrimaryAfterConditionalRejection(t *testing.T) {
	ctx, owner := t.Context(), giznet.PublicKey{98}
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	first, err := (&Server{Store: authority}).Sync(ctx, owner, 0, Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	var ordinary, primary, attempts atomic.Int32
	reads := kvfixture.ReadView(authority, replica, &ordinary, &primary)
	server := &Server{Store: checkpointReadWriteStore{Store: reads, authority: authority, attempts: &attempts, before: func() {
		if attempts.Load() == 1 && (ordinary.Load() != 1 || primary.Load() != 0) {
			t.Errorf("first head read bypassed replica: %d/%d", ordinary.Load(), primary.Load())
		}
	}}}
	second, err := server.Sync(ctx, owner, 0, Snapshot{})
	if err != nil || second.Timestamp <= first.Timestamp || attempts.Load() != 2 || primary.Load() != 1 {
		t.Fatalf("stale-head retry=%+v err=%v attempts=%d reads=%d/%d", second, err, attempts.Load(), ordinary.Load(), primary.Load())
	}
}
