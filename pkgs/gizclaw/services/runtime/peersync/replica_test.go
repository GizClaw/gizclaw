package peersync

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	redis "github.com/redis/go-redis/v9"
)

type checkpointReadHook struct{ data kv.Store }

func (h checkpointReadHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (h checkpointReadHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h checkpointReadHook) ProcessHook(redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		value, err := h.data.Get(ctx, kv.Key(strings.Split(command.Args()[1].(string), ":")))
		if errors.Is(err, kv.ErrNotFound) {
			err = redis.Nil
		}
		result := command.(*redis.StringCmd)
		result.SetVal(string(value))
		result.SetErr(err)
		return err
	}
}

type checkpointReadWriteStore struct {
	kv.Store
	authority kv.Store
}

func (s checkpointReadWriteStore) ApplyMutation(ctx context.Context, mutation kv.Mutation) (bool, error) {
	return s.authority.ApplyMutation(ctx, mutation)
}

func TestSyncResumesCommittedCheckpointWhileReplicaHasNoHead(t *testing.T) {
	authority, replica := kv.NewMemory(nil), kv.NewMemory(nil)
	reader, writer := redis.NewClient(&redis.Options{}), redis.NewClient(&redis.Options{})
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	reader.AddHook(checkpointReadHook{data: replica})
	writer.AddHook(checkpointReadHook{data: authority})
	reads, err := kv.NewRedisReadWriteWithClients(reader, writer, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{Store: checkpointReadWriteStore{Store: reads, authority: authority}}
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

var _ redis.Hook = checkpointReadHook{}
