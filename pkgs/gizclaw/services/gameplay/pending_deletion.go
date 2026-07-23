package gameplay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/jmoiron/sqlx"
)

// PendingDeletionSource exposes the gameplay SQL handoff through the common
// lookup contract without making it part of the Admin or Peer API.
type PendingDeletionSource struct {
	DB *sqlx.DB
}

// Get loads one gameplay deletion event by ID.
func (s PendingDeletionSource) Get(ctx context.Context, deletionID string) (pendingdeletion.Record, error) {
	if s.DB == nil {
		return pendingdeletion.Record{}, errors.New("gameplay: database not configured")
	}
	return getPendingDeletion(ctx, s.DB, deletionID)
}

func getPendingDeletion(ctx context.Context, db queryRebinder, deletionID string) (pendingdeletion.Record, error) {
	var (
		record         pendingdeletion.Record
		owner          string
		deletedAt      string
		descriptorJSON string
	)
	err := db.QueryRowContext(ctx, db.Rebind(`SELECT deletion_id, kind, owner_public_key, resource_id, reason, deleted_at, descriptor_version, descriptor_json FROM gameplay_pending_deletions WHERE deletion_id = ?`), deletionID).Scan(
		&record.DeletionID,
		&record.Kind,
		&owner,
		&record.ResourceID,
		&record.Reason,
		&deletedAt,
		&record.DescriptorVersion,
		&descriptorJSON,
	)
	if err != nil {
		return pendingdeletion.Record{}, err
	}
	parsedDeletedAt, err := time.Parse(time.RFC3339Nano, deletedAt)
	if err != nil {
		return pendingdeletion.Record{}, fmt.Errorf("gameplay: decode pending deletion %q timestamp: %w", deletionID, err)
	}
	record.OwnerPublicKey = &owner
	record.DeletedAt = parsedDeletedAt
	record.Descriptor = json.RawMessage(descriptorJSON)
	if err := record.Validate(); err != nil {
		return pendingdeletion.Record{}, fmt.Errorf("gameplay: validate pending deletion %q: %w", deletionID, err)
	}
	return record, nil
}

// HasLocator reports whether gameplay contains a matching deletion event.
func (s PendingDeletionSource) HasLocator(ctx context.Context, locator pendingdeletion.Locator) (bool, error) {
	if s.DB == nil {
		return false, errors.New("gameplay: database not configured")
	}
	if locator.OwnerPublicKey == nil {
		return false, errors.New("gameplay: pending deletion locator owner is required")
	}
	owner := strings.TrimSpace(*locator.OwnerPublicKey)
	if owner == "" {
		return false, errors.New("gameplay: pending deletion locator owner is empty")
	}
	query := `SELECT deletion_id
		FROM gameplay_pending_deletion_locators
		WHERE kind = ? AND resource_id = ? AND owner_public_key = ?
		LIMIT 1`
	var deletionID string
	err := s.DB.QueryRowContext(ctx, s.DB.Rebind(query), locator.Kind, locator.ResourceID, owner).Scan(&deletionID)
	if err == nil {
		return s.validateLocatorRecord(ctx, deletionID, locator, owner)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("gameplay: lookup pending deletion: %w", err)
	}

	// Legacy #469 records predate the fixed locator table.
	query = `SELECT deletion_id
		FROM gameplay_pending_deletions
		WHERE kind = ? AND resource_id = ? AND owner_public_key = ?
		ORDER BY deleted_at, deletion_id
		LIMIT 1`
	err = s.DB.QueryRowContext(ctx, s.DB.Rebind(query), locator.Kind, locator.ResourceID, owner).Scan(&deletionID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("gameplay: lookup pending deletion: %w", err)
	}
	return s.validateLocatorRecord(ctx, deletionID, locator, owner)
}

func (s PendingDeletionSource) validateLocatorRecord(
	ctx context.Context,
	deletionID string,
	locator pendingdeletion.Locator,
	owner string,
) (bool, error) {
	record, err := s.Get(ctx, deletionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, fmt.Errorf("gameplay: pending deletion locator %q references a missing or mismatched record", deletionID)
		}
		return false, fmt.Errorf("gameplay: validate pending deletion locator %q: %w", deletionID, err)
	}
	if record.Kind != locator.Kind ||
		record.ResourceID != locator.ResourceID ||
		record.OwnerPublicKey == nil ||
		*record.OwnerPublicKey != owner {
		return false, fmt.Errorf("gameplay: pending deletion locator %q references a missing or mismatched record", deletionID)
	}
	return true, nil
}

var _ pendingdeletion.Source = PendingDeletionSource{}
