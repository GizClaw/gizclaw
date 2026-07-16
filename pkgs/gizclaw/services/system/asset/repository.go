package asset

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

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
	if record.SchemaVersion != 1 || record.ID != id {
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
	if record.ExpiresAt != nil {
		entry.Deadline = *record.ExpiresAt
	}
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

func (r repository) bindings(ctx context.Context, id string) ([]Binding, error) {
	records, err := r.bindingRecords(ctx, id)
	if err != nil {
		return nil, err
	}
	bindings := make([]Binding, 0)
	for _, record := range records {
		bindings = append(bindings, Binding{Owner: record.Owner})
	}
	return bindings, nil
}

func (r repository) bindingRecords(ctx context.Context, id string) ([]bindingRecord, error) {
	records := make([]bindingRecord, 0)
	for entry, err := range r.store.List(ctx, bindingByAssetPrefix(id)) {
		if err != nil {
			return nil, fmt.Errorf("asset bindings list %s: %w", id, err)
		}
		var record bindingRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return nil, fmt.Errorf("asset binding decode %s: %w", id, err)
		}
		if record.State != bindingStatePending && record.State != bindingStateActive {
			return nil, fmt.Errorf("%w: invalid binding state %q", ErrInvalid, record.State)
		}
		records = append(records, record)
	}
	return records, nil
}

func (r repository) bind(ctx context.Context, id string, binding Binding, state bindingState, createdAt time.Time, deadline *time.Time) error {
	data, err := json.Marshal(bindingRecord{Owner: binding.Owner, State: state, CreatedAt: createdAt.UTC()})
	if err != nil {
		return fmt.Errorf("asset binding encode: %w", err)
	}
	hash := ownerHash(binding.Owner)
	entries := []kv.Entry{
		{Key: bindingByAssetKey(id, binding.Owner, hash), Value: data},
		{Key: bindingByOwnerKey(binding.Owner, hash, id), Value: data},
	}
	if deadline != nil {
		entries[0].Deadline = *deadline
		entries[1].Deadline = *deadline
	}
	if err := r.store.BatchSet(ctx, entries); err != nil {
		return fmt.Errorf("asset binding put: %w", err)
	}
	return nil
}

func (r repository) unbind(ctx context.Context, id string, owner Owner) error {
	hash := ownerHash(owner)
	if err := r.store.BatchDelete(ctx, []kv.Key{
		bindingByAssetKey(id, owner, hash),
		bindingByOwnerKey(owner, hash, id),
	}); err != nil {
		return fmt.Errorf("asset binding delete: %w", err)
	}
	return nil
}

func (r repository) ownerAssetIDs(ctx context.Context, owner Owner) ([]string, error) {
	hash := ownerHash(owner)
	ids := make([]string, 0)
	for entry, err := range r.store.List(ctx, bindingByOwnerPrefix(owner, hash)) {
		if err != nil {
			return nil, fmt.Errorf("owner bindings list: %w", err)
		}
		if len(entry.Key) != 5 {
			return nil, fmt.Errorf("%w: invalid owner binding key %v", ErrInvalid, entry.Key)
		}
		ids = append(ids, entry.Key[4])
	}
	return ids, nil
}

func assetKey(id string) kv.Key {
	return kv.Key{"assets", "by-id", id}
}

func bindingByAssetPrefix(id string) kv.Key {
	return kv.Key{"bindings", "by-asset", id}
}

func bindingByAssetKey(id string, owner Owner, hash string) kv.Key {
	return kv.Key{"bindings", "by-asset", id, string(owner.Kind), hash}
}

func bindingByOwnerPrefix(owner Owner, hash string) kv.Key {
	return kv.Key{"bindings", "by-owner", string(owner.Kind), hash}
}

func bindingByOwnerKey(owner Owner, hash, id string) kv.Key {
	return kv.Key{"bindings", "by-owner", string(owner.Kind), hash, id}
}

func ownerHash(owner Owner) string {
	digest := sha256.Sum256([]byte(string(owner.Kind) + "\x00" + owner.ID))
	return hex.EncodeToString(digest[:])
}
