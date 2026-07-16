package asset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type repository struct {
	store kv.Store
}

func (r repository) asset(ctx context.Context, id string) (assetRecord, error) {
	data, err := r.store.Get(ctx, assetKey(id))
	if errors.Is(err, kv.ErrNotFound) {
		return assetRecord{}, ErrNotFound
	}
	if err != nil {
		return assetRecord{}, fmt.Errorf("asset metadata get %s: %w", id, err)
	}
	var record assetRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return assetRecord{}, fmt.Errorf("asset metadata decode %s: %w", id, err)
	}
	if record.SchemaVersion != assetSchemaVersion || record.ID != id {
		return assetRecord{}, fmt.Errorf("%w: invalid metadata record %s", ErrInvalid, id)
	}
	return record, nil
}

func (r repository) putAsset(ctx context.Context, record assetRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("asset metadata encode %s: %w", record.ID, err)
	}
	entry := kv.Entry{Key: assetKey(record.ID), Value: data}
	if err := r.store.BatchSet(ctx, []kv.Entry{entry}); err != nil {
		return fmt.Errorf("asset metadata put %s: %w", record.ID, err)
	}
	return nil
}

func (r repository) deleteAsset(ctx context.Context, id string) error {
	if err := r.store.Delete(ctx, assetKey(id)); err != nil {
		return fmt.Errorf("asset metadata delete %s: %w", id, err)
	}
	return nil
}

func assetKey(id string) kv.Key {
	return kv.Key{"assets", "by-id", id}
}
