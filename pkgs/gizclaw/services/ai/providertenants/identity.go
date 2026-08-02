package providertenants

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return socialutil.NewID()
}

func createNamedTenant(ctx context.Context, store kv.Store, recordKey, nameKey kv.Key, id string, value any) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	_, created, err := kv.CreateIfAbsent(ctx, store,
		kv.Entry{Key: nameKey, Value: []byte(id)},
		[]kv.Entry{{Key: recordKey, Value: data}},
	)
	return created, err
}

func deleteNamedTenant(ctx context.Context, store kv.Store, recordKey, nameKey kv.Key) error {
	if err := store.BatchDelete(ctx, []kv.Key{recordKey, nameKey}); err != nil {
		return fmt.Errorf("delete canonical tenant: %w", err)
	}
	return nil
}

func tenantNameKey(root kv.Key, name string) kv.Key {
	return append(append(kv.Key{}, root...), escapeStoreSegment(name))
}
