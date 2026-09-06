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
	Store      kv.Store
	SourceName string
	OwnedKinds []Kind
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

var _ LookupSource = KVSource{}
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
	fingerprint, err := Fingerprint(record)
	if err != nil {
		return Record{}, false, err
	}
	task := Task{Record: record, MarkerFingerprint: fingerprint, Status: StatusQueued, Phase: PhaseValidate, NextAttemptAt: record.DeletedAt, UpdatedAt: record.DeletedAt}
	state, err := encodeKVTaskState(task)
	if err != nil {
		return Record{}, false, err
	}
	locatorKey := byLocatorKey(record.Kind, record.ResourceID)
	entries = append(entries, kv.Entry{Key: locatorKey, Value: []byte(record.DeletionID)}, kv.Entry{Key: kvTaskKey(record.DeletionID), Value: state})
	add, _ := kvTaskIndexChanges(nil, &task)
	created, err := store.ApplyMutation(ctx, kv.Mutation{
		Conditions: []kv.Condition{{Key: locatorKey}, {Key: byIDKey(record.DeletionID)}, {Key: kvTaskKey(record.DeletionID)}},
		Entries:    entries, AddOrderedMembers: add,
	})
	if err != nil {
		return Record{}, false, err
	}
	if created {
		return record, true, nil
	}
	existingID, err := store.Get(ctx, locatorKey)
	if errors.Is(err, kv.ErrNotFound) {
		return Record{}, false, ErrConflict
	}
	if err != nil {
		return Record{}, false, err
	}

	if len(existingID) == 0 {
		return Record{}, false, errors.New("pending deletion: empty KV locator record")
	}
	existing, err := Get(ctx, store, string(existingID))
	if err != nil {
		return Record{}, false, fmt.Errorf("pending deletion: get existing locator record: %w", err)
	}
	if err := validateLocatorRecord(existing, record.Kind, record.ResourceID); err != nil {
		return Record{}, false, err
	}
	return existing, false, nil
}

// GetByLocator loads the PendingDeletion record for one logical resource.
func GetByLocator(ctx context.Context, store kv.Store, kind Kind, resourceID string) (Record, error) {
	if store == nil {
		return Record{}, errors.New("pending deletion: KV store not configured")
	}
	record, found, err := resolveExistingLocator(ctx, store, kind, resourceID)
	if err != nil {
		return Record{}, err
	}
	if !found {
		return Record{}, kv.ErrNotFound
	}
	return record, nil
}

func resolveExistingLocator(ctx context.Context, store kv.Store, kind Kind, resourceID string) (Record, bool, error) {
	if fixedID, err := store.Get(ctx, byLocatorKey(kind, resourceID)); err == nil {
		if len(fixedID) == 0 {
			return Record{}, false, errors.New("pending deletion: empty KV locator record")
		}
		existing, err := Get(ctx, store, string(fixedID))
		if err != nil {
			return Record{}, false, fmt.Errorf("pending deletion: get existing locator record: %w", err)
		}
		if err := validateLocatorRecord(existing, kind, resourceID); err != nil {
			return Record{}, false, err
		}
		return existing, true, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return Record{}, false, err
	}
	return Record{}, false, nil
}

func validateLocatorRecord(record Record, kind Kind, resourceID string) error {
	if record.Kind == kind && record.ResourceID == resourceID {
		return nil
	}
	return fmt.Errorf(
		"pending deletion: %s %q locator references %s %q record %q",
		kind,
		resourceID,
		record.Kind,
		record.ResourceID,
		record.DeletionID,
	)
}

// Get loads and validates one KV-backed deletion event by ID.
func Get(ctx context.Context, store kv.Store, deletionID string) (Record, error) {
	record, err := getStoredRecord(ctx, store, deletionID)
	if err != nil {
		return Record{}, err
	}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("pending deletion: validate %s: %w", deletionID, err)
	}
	return record, nil
}

func getStoredRecord(ctx context.Context, store kv.Store, deletionID string) (Record, error) {
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
	if record.DeletionID != deletionID {
		return Record{}, fmt.Errorf(
			"pending deletion: record key %q contains deletion id %q",
			deletionID,
			record.DeletionID,
		)
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
		record, err := Get(ctx, store, string(deletionID))
		if err != nil {
			return false, fmt.Errorf("pending deletion: get locator record: %w", err)
		}
		if err := validateLocatorRecord(record, kind, resourceID); err != nil {
			return false, err
		}
		return true, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return false, err
	}
	return false, nil
}

// AbsentLocatorCondition fences a domain mutation against a concurrent deletion
// request. Apply it on the same KV store used by CreateOrGet, together with a
// condition on the domain record so a completed tombstone cannot be overwritten.
func AbsentLocatorCondition(kind Kind, resourceID string) kv.Condition {
	return kv.Condition{Key: byLocatorKey(kind, resourceID)}
}

func byIDKey(deletionID string) kv.Key {
	return append(append(kv.Key{}, root...), "by-id", deletionID)
}

func byLocatorKey(kind Kind, resourceID string) kv.Key {
	encoded := base64.RawURLEncoding.EncodeToString([]byte(resourceID))
	return append(append(kv.Key{}, root...), "by-locator", string(kind), encoded)
}
