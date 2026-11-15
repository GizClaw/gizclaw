package socialutil

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	redis "github.com/redis/go-redis/v9"
)

func TestPeerRetirementFenceProvesReaderAndPreservesNamespace(t *testing.T) {
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	reader, writer := redis.NewClient(&redis.Options{}), redis.NewClient(&redis.Options{})
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	ordinary, primary := 0, 0
	reader.AddHook(inviteReadHook{store: replica, reads: &ordinary})
	writer.AddHook(inviteReadHook{store: authority, reads: &primary})
	reads, err := kv.NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := &inviteReplicaStore{Store: reads, authority: authority}
	owner := "peer/with:segments%+"
	prefix := kv.Key{"invitation-domain"}
	prepared, err := FencePeerRetirement(ctx, kv.Prefixed(store, prefix), owner)
	if err != nil || kv.WithPrimaryReads(prepared) != prepared || ordinary != 1 || primary != 1 {
		t.Fatalf("fence proof = %v reads=%d/%d", err, ordinary, primary)
	}
	key := PeerRetirementFenceKey(owner)
	if len(key) != 2 || key[1] != EscapeStoreSegment(owner) {
		t.Fatalf("unsafe fence key = %v", key)
	}
	if _, err := authority.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("fence escaped namespace: %v", err)
	}
	fullKey := append(slices.Clone(prefix), key...)
	if data, err := authority.Get(ctx, fullKey); err != nil || string(data) != owner {
		t.Fatalf("permanent fence = %q, %v", data, err)
	}
	if condition := PeerRetirementCondition(owner); !slices.Equal(condition.Key, key) || condition.Expected != nil {
		t.Fatalf("fence admission condition = %+v", condition)
	}
	// The ordinary caller remains on its configured reader.
	if _, err := store.Get(ctx, fullKey); !errors.Is(err, kv.ErrNotFound) || primary != 1 {
		t.Fatalf("fence pinned ordinary reads: primary=%d err=%v", primary, err)
	}
}

func TestPeerRetirementFenceKeepsCaughtUpReader(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	prepared, err := FencePeerRetirement(ctx, store, "owner")
	if err != nil || prepared != ctx {
		t.Fatalf("caught-up reader = %v, context changed=%v", err, prepared != ctx)
	}
	matched, err := store.ApplyMutation(ctx, kv.Mutation{Conditions: []kv.Condition{PeerRetirementCondition("owner")}})
	if err != nil || matched {
		t.Fatalf("fenced admission applied=%v err=%v", matched, err)
	}
}

func TestUpdateInviteTokenCallbackCannotChangeExpectedBytes(t *testing.T) {
	ctx := t.Context()
	store := kv.NewMemory(nil)
	key := FriendInviteTokenKey("owner")
	if err := WriteInviteToken(ctx, store, key, map[string]string{"invite_token": "old"}); err != nil {
		t.Fatal(err)
	}
	data, err := UpdateInviteToken(ctx, store, key, nil, func(current []byte) ([]byte, error) {
		index := bytes.Index(current, []byte("old"))
		copy(current[index:], "new")
		return current, nil
	})
	if err != nil || !bytes.Contains(data, []byte("new")) {
		t.Fatalf("callback-owned bytes = %s, %v", data, err)
	}
	if _, err := ReadInviteToken(ctx, store, FriendInviteTokensRoot, "old"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("old token index survived = %v", err)
	}
	if _, err := ReadInviteToken(ctx, store, FriendInviteTokensRoot, "new"); err != nil {
		t.Fatal(err)
	}
	data, err = UpdateInviteToken(ctx, store, key, nil, func(current []byte) ([]byte, error) {
		if current == nil {
			t.Fatal("existing invitation appeared absent")
		}
		return nil, nil
	})
	if err != nil || data != nil {
		t.Fatalf("delete callback = %s, %v", data, err)
	}
	if _, err := ReadInviteToken(ctx, store, FriendInviteTokensRoot, "new"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("deleted index survived = %v", err)
	}
	_, err = UpdateInviteToken(ctx, store, key, nil, func(current []byte) ([]byte, error) {
		if current != nil {
			t.Fatal("absent invitation callback did not receive nil")
		}
		return []byte(`{"invite_token":"fresh"}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type uncertainInviteCommitStore struct {
	kv.Store
	calls int
	err   error
}

func (s *uncertainInviteCommitStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.calls++
	if _, err := s.Store.ApplyMutation(ctx, mutation); err != nil {
		return false, err
	}
	return false, s.err
}

func TestUpdateInviteTokenNeverReplaysUncertainCommit(t *testing.T) {
	ctx := t.Context()
	uncertain := errors.New("uncertain commit")
	store := &uncertainInviteCommitStore{Store: kv.NewMemory(nil), err: uncertain}
	callbackCalls := 0
	_, err := UpdateInviteToken(ctx, store, FriendInviteTokenKey("owner"), nil, func([]byte) ([]byte, error) {
		callbackCalls++
		return []byte(`{"invite_token":"committed"}`), nil
	})
	if !errors.Is(err, uncertain) || store.calls != 1 || callbackCalls != 1 {
		t.Fatalf("uncertain update err=%v mutations=%d callbacks=%d", err, store.calls, callbackCalls)
	}
	if _, err := ReadInviteToken(ctx, store.Store, FriendInviteTokensRoot, "committed"); err != nil {
		t.Fatalf("uncertain commit fixture did not commit = %v", err)
	}
}
