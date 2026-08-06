package providertenants

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func validateResourceID(id string) error {
	return customid.ValidateResourceID(id)
}

func validateResourceReference(field, id string) error {
	if err := customid.ValidateResourceID(id); err != nil {
		return fmt.Errorf("%s: %w", field, err)
	}
	return nil
}

func createTenant(ctx context.Context, store kv.Store, recordKey kv.Key, value any) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	_, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{Key: recordKey, Value: data}, nil)
	return created, err
}

func deleteTenant(ctx context.Context, store kv.Store, recordKey kv.Key) error {
	return store.Delete(ctx, recordKey)
}
