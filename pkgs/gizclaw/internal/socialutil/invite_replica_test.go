package socialutil

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	redis "github.com/redis/go-redis/v9"
)

type inviteReadHook struct {
	store kv.Store
	reads *int
}

func (h inviteReadHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h inviteReadHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h inviteReadHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		*h.reads++
		value, err := h.store.Get(ctx, kv.Key(strings.Split(command.Args()[1].(string), ":")))
		if errors.Is(err, kv.ErrNotFound) {
			err = redis.Nil
		}
		result := command.(*redis.StringCmd)
		result.SetVal(string(value))
		result.SetErr(err)
		return err
	}
}

type inviteReplicaStore struct {
	kv.Store
	authority kv.Store
	writes    int
}

func (s *inviteReplicaStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	s.writes++
	return s.authority.ApplyMutation(ctx, mutation)
}

func TestInviteTokenLookupFallsBackOnlyOnMiss(t *testing.T) {
	ctx := t.Context()
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	key := FriendInviteTokenKey("owner")
	value := map[string]string{"invite_token": "new-token"}
	if err := WriteInviteToken(ctx, authority, key, value); err != nil {
		t.Fatal(err)
	}
	reader, writer := redis.NewClient(&redis.Options{}), redis.NewClient(&redis.Options{})
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	ordinary, primary := 0, 0
	reader.AddHook(inviteReadHook{store: replica, reads: &ordinary})
	writer.AddHook(inviteReadHook{store: authority, reads: &primary})
	store, err := kv.NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadInviteToken(ctx, store, FriendInviteTokensRoot, "new-token"); err != nil || ordinary != 1 || primary != 2 {
		t.Fatalf("new token lookup err=%v reads=%d/%d", err, ordinary, primary)
	}
	if err := WriteInviteToken(ctx, replica, key, value); err != nil {
		t.Fatal(err)
	}
	if err := DeleteInviteToken(ctx, authority, key); err != nil {
		t.Fatal(err)
	}
	ordinary, primary = 0, 0
	if _, err := ReadInviteToken(ctx, store, FriendInviteTokensRoot, "new-token"); err != nil || ordinary != 2 || primary != 0 {
		t.Fatalf("successful stale lookup err=%v reads=%d/%d", err, ordinary, primary)
	}
}

func TestInviteTokenMutationRefreshesStaleObservation(t *testing.T) {
	for _, operation := range []string{"rotate", "delete", "delete-missing"} {
		t.Run(operation, func(t *testing.T) {
			ctx := t.Context()
			authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
			key := FriendInviteTokenKey("owner")
			token := func(value string) map[string]string { return map[string]string{"invite_token": value} }
			if operation != "delete-missing" {
				if err := WriteInviteToken(ctx, replica, key, token("old")); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteInviteToken(ctx, authority, key, token("current")); err != nil {
				t.Fatal(err)
			}
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
			if operation == "rotate" {
				err = WriteInviteToken(ctx, store, key, token("next"))
			} else {
				err = DeleteInviteToken(ctx, store, key)
			}
			if err != nil || ordinary == 0 || primary == 0 || store.writes > 2 {
				t.Fatalf("operation=%s err=%v reads=%d/%d writes=%d", operation, err, ordinary, primary, store.writes)
			}
			if _, err := authority.Get(ctx, InviteTokenIndexKey(FriendInviteTokensRoot, "current")); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("current token index was not removed: %v", err)
			}
			if operation == "rotate" {
				if _, err := ReadInviteToken(ctx, authority, FriendInviteTokensRoot, "next"); err != nil {
					t.Fatalf("new token is unavailable: %v", err)
				}
			} else if _, err := authority.Get(ctx, key); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("token survived deletion: %v", err)
			}
		})
	}
}
