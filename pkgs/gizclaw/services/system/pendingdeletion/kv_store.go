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

// KVEntries returns the durable record entry. CreateOrGet adds the unique
// locator entry atomically with this record.
func KVEntries(record Record) ([]kv.Entry, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, fmt.Errorf("pending deletion: encode record: %w", err)
	}
	return []kv.Entry{{Key: byIDKey(record.DeletionID), Value: data}}, nil
}

// CreateOrGet persists one record for a locator, or returns the record that
// won an earlier concurrent create. It is the only KV producer write path for
// PendingDeletion records.
func CreateOrGet(ctx context.Context, store kv.Store, record Record) (Record, bool, error) {
	if store == nil {
		return Record{}, false, errors.New("pending deletion: KV store not configured")
	}
	entries, err := KVEntries(record)
	if err != nil {
		return Record{}, false, err
	}
	if existing, found, err := resolveExistingLocator(ctx, store, record.Kind, record.ResourceID); err != nil {
		return Record{}, false, err
	} else if found {
		return existing, false, nil
	}
	existingID, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{
		Key:   byLocatorKey(record.Kind, record.ResourceID),
		Value: []byte(record.DeletionID),
	}, entries)
	if err != nil {
		return Record{}, false, err
	}
	if created {
		return record, true, nil
	}
	if len(existingID) == 0 {
		return Record{}, false, errors.New("pending deletion: empty KV locator record")
	}
	existing, err := Get(ctx, store, string(existingID))
	if err != nil {
		return Record{}, false, fmt.Errorf("pending deletion: get existing locator record: %w", err)
	}
	return existing, false, nil
}

func resolveExistingLocator(ctx context.Context, store kv.Store, kind Kind, resourceID string) (Record, bool, error) {
	prefix := legacyByLocatorPrefix(kind, resourceID)
	if fixedID, err := store.Get(ctx, byLocatorKey(kind, resourceID)); err == nil {
		if len(fixedID) == 0 {
			return Record{}, false, errors.New("pending deletion: empty KV locator record")
		}
		existing, err := Get(ctx, store, string(fixedID))
		if err != nil {
			return Record{}, false, fmt.Errorf("pending deletion: get existing locator record: %w", err)
		}
		return existing, true, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return Record{}, false, err
	}
	var legacy *Record
	for entry, err := range store.List(ctx, prefix) {
		if err != nil {
			return Record{}, false, err
		}
		if len(entry.Key) != len(prefix)+1 {
			continue
		}
		deletionID := entry.Key[len(prefix)]
		candidate, err := Get(ctx, store, deletionID)
		if err != nil {
			return Record{}, false, fmt.Errorf("pending deletion: get legacy locator record: %w", err)
		}
		if candidate.Kind != kind || candidate.ResourceID != resourceID {
			return Record{}, false, fmt.Errorf(
				"pending deletion: legacy locator %q references %s %q",
				deletionID,
				candidate.Kind,
				candidate.ResourceID,
			)
		}
		if legacy == nil ||
			candidate.DeletedAt.Before(legacy.DeletedAt) ||
			(candidate.DeletedAt.Equal(legacy.DeletedAt) && candidate.DeletionID < legacy.DeletionID) {
			legacy = &candidate
		}
	}
	if legacy == nil {
		return Record{}, false, nil
	}
	deletionID := legacy.DeletionID
	existingID, created, err := kv.CreateIfAbsent(ctx, store, kv.Entry{
		Key:   byLocatorKey(kind, resourceID),
		Value: []byte(deletionID),
	}, nil)
	if err != nil {
		return Record{}, false, err
	}
	if !created {
		if len(existingID) == 0 {
			return Record{}, false, errors.New("pending deletion: empty KV locator record")
		}
		deletionID = string(existingID)
	}
	existing, err := Get(ctx, store, deletionID)
	if err != nil {
		return Record{}, false, fmt.Errorf("pending deletion: get migrated locator record: %w", err)
	}
	return existing, true, nil
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
	if deletionID, err := store.Get(ctx, byLocatorKey(kind, resourceID)); err == nil {
		if len(deletionID) == 0 {
			return false, errors.New("pending deletion: empty KV locator record")
		}
		return true, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return false, err
	}
	// Legacy #469 entries used a deletion-ID suffix. Keep source lookup
	// compatible until the cleanup processor consumes those records.
	for _, err := range store.List(ctx, legacyByLocatorPrefix(kind, resourceID)) {
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

func byLocatorKey(kind Kind, resourceID string) kv.Key {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(resourceID))
	return append(append(kv.Key{}, root...), "by-locator", string(kind), encoded)
}

func legacyByLocatorPrefix(kind Kind, resourceID string) kv.Key {
	return byLocatorKey(kind, resourceID)
}
