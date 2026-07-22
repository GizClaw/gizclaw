package pendingdeletion

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

var root = kv.Key{"pending-deletion"}

// KVSource exposes pending deletion records stored in a KV backend.
type KVSource struct {
	Store kv.Store
}

// Get loads one deletion event by ID.
func (s KVSource) Get(ctx context.Context, deletionID string) (Record, error) {
	if s.Store == nil {
		return Record{}, errors.New("pending deletion: KV store not configured")
	}
	return Get(ctx, s.Store, deletionID)
}

// HasLocator reports whether the KV backend contains a matching event.
func (s KVSource) HasLocator(ctx context.Context, locator Locator) (bool, error) {
	if s.Store == nil {
		return false, errors.New("pending deletion: KV store not configured")
	}
	if locator.OwnerPublicKey != nil {
		return false, errors.New("pending deletion: KV locator owner filter is not supported")
	}
	return HasLocator(ctx, s.Store, locator.Kind, locator.ResourceID)
}

var _ Source = KVSource{}

// KVEntries returns the durable record and its multi-value locator index.
func KVEntries(record Record) ([]kv.Entry, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("pending deletion: encode record: %w", err)
	}
	return []kv.Entry{
		{Key: byIDKey(record.DeletionID), Value: data},
		{Key: byLocatorKey(record.Kind, record.ResourceID, record.DeletionID), Value: []byte{}},
	}, nil
}

// Get loads and validates one KV-backed deletion event by ID.
func Get(ctx context.Context, store kv.Store, deletionID string) (Record, error) {
	if store == nil {
		return Record{}, errors.New("pending deletion: KV store not configured")
	}
	data, err := store.Get(ctx, byIDKey(deletionID))
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("pending deletion: decode %s: %w", deletionID, err)
	}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("pending deletion: validate %s: %w", deletionID, err)
	}
	return record, nil
}

// HasLocator reports whether any deletion event exists for a resource locator.
func HasLocator(ctx context.Context, store kv.Store, kind Kind, resourceID string) (bool, error) {
	if store == nil {
		return false, errors.New("pending deletion: KV store not configured")
	}
	for _, err := range store.List(ctx, byLocatorPrefix(kind, resourceID)) {
		if err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func byIDKey(deletionID string) kv.Key {
	return append(append(kv.Key{}, root...), "by-id", deletionID)
}

func byLocatorKey(kind Kind, resourceID, deletionID string) kv.Key {
	return append(byLocatorPrefix(kind, resourceID), deletionID)
}

func byLocatorPrefix(kind Kind, resourceID string) kv.Key {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(resourceID))
	return append(append(kv.Key{}, root...), "by-locator", string(kind), encoded)
}
