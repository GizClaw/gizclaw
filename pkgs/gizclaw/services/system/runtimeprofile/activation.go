package runtimeprofile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// ErrRegistrationDenied means the token cannot authorize a new activation.
var ErrRegistrationDenied = errors.New("registration token is disabled, expired, exhausted or changed")

func ensureRegistrationLifecycleColumns(ctx context.Context, tx *sqlx.Tx) error {
	for _, table := range []struct {
		name    string
		columns []string
	}{
		{"registration_tokens", []string{"enabled BOOLEAN NOT NULL DEFAULT TRUE", "expires_at TEXT", "max_activations BIGINT CHECK(max_activations>=0)"}},
		{"runtime_profile_owners", []string{"firmware_id TEXT"}},
	} {
		for _, definition := range table.columns {
			if tx.DriverName() == "postgres" || tx.DriverName() == "pgx" {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table.name+" ADD COLUMN IF NOT EXISTS "+definition); err != nil {
					return err
				}
				continue
			}
			rows, err := tx.QueryContext(ctx, "SELECT * FROM "+table.name+" WHERE 1=0")
			if err != nil {
				return err
			}
			columns, err := rows.Columns()
			closeErr := rows.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
			if !slices.Contains(columns, strings.Fields(definition)[0]) {
				if _, err := tx.ExecContext(ctx, "ALTER TABLE "+table.name+" ADD COLUMN "+definition); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func registrationExpiryText(value *time.Time) *string {
	if value == nil {
		return nil
	}
	return new(value.UTC().Format(time.RFC3339Nano))
}

func registrationAvailable(item apitypes.RegistrationToken, now time.Time) bool {
	return item.Enabled && (item.ExpiresAt == nil || now.Before(*item.ExpiresAt)) &&
		(item.MaxActivations == nil || item.ActivationCount < *item.MaxActivations)
}

// PreflightRegistration performs the read-only admission check. It reserves no
// capacity: RegisterOwner must repeat the authoritative check in its transaction.
func (s *Server) PreflightRegistration(ctx context.Context, token string) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	item, _, err := scanRegistrationTokenSQL(db.QueryRowContext(ctx, db.Rebind("SELECT "+registrationTokenProjection+" FROM registration_tokens WHERE token=?"), strings.TrimSpace(token)))
	if err != nil {
		return err
	}
	if !registrationAvailable(item, s.now()) {
		return ErrRegistrationDenied
	}
	_, err = s.ResolveRegistration(ctx, token)
	return err
}

// RegisterOwner atomically records a distinct public-key activation and the
// selected profile/firmware. Repeating an activation remains idempotent even if
// the administrator subsequently restricts the token. publish is a non-failing
// connection-local update, invoked after SQL commit under the owner's lock.
func (s *Server) RegisterOwner(ctx context.Context, owner, token string, publish func(Registration)) (Registration, error) {
	db, err := s.database()
	if err != nil {
		return Registration{}, err
	}
	if strings.TrimSpace(owner) == "" {
		return Registration{}, errors.New("registration owner is required")
	}
	release, err := s.ownerLocks.Acquire(ctx, owner)
	if err != nil {
		return Registration{}, err
	}
	defer release()
	// Resolve external Firmware resources before opening the SQL transaction.
	resolved, err := s.ResolveRegistration(ctx, token)
	if err != nil {
		return Registration{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return Registration{}, err
	}
	defer tx.Rollback()
	// The first statement acquires a write lock before any snapshot reads. This
	// serializes contenders on PostgreSQL and avoids SQLite read-to-write upgrades.
	if _, err := tx.ExecContext(ctx, tx.Rebind("UPDATE registration_tokens SET row_version=row_version WHERE token=?"), strings.TrimSpace(token)); err != nil {
		return Registration{}, err
	}
	item, _, err := scanRegistrationTokenSQL(tx.QueryRowContext(ctx, tx.Rebind("SELECT "+registrationTokenProjection+" FROM registration_tokens WHERE token=?"), strings.TrimSpace(token)))
	if err != nil {
		return Registration{}, err
	}
	if item.Id != resolved.TokenID || item.RuntimeProfileId != resolved.RuntimeProfile.Id || !sameOptionalString(item.FirmwareId, resolved.FirmwareID) {
		return Registration{}, ErrRegistrationDenied
	}
	var activated bool
	if err := tx.QueryRowContext(ctx, tx.Rebind("SELECT EXISTS(SELECT 1 FROM registration_token_activations WHERE token_id=? AND peer_public_key=?)"), item.Id, owner).Scan(&activated); err != nil {
		return Registration{}, err
	}
	if !activated && !registrationAvailable(item, s.now()) {
		return Registration{}, ErrRegistrationDenied
	}
	// Read the profile within the same transaction as its binding.
	profile, _, err := scanRuntimeProfileSQL(tx.QueryRowContext(ctx, tx.Rebind("SELECT "+runtimeProfileColumns+" FROM runtime_profiles WHERE id=?"), item.RuntimeProfileId))
	if err != nil {
		return Registration{}, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO registration_token_activations(token_id,peer_public_key,activated_at) VALUES (?,?,?) ON CONFLICT(token_id,peer_public_key) DO NOTHING"), item.Id, owner, s.now().UTC().Format(time.RFC3339Nano)); err != nil {
		return Registration{}, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind("INSERT INTO runtime_profile_owners(owner_public_key,runtime_profile_id,binding_id,firmware_id) VALUES (?,?,?,?) ON CONFLICT(owner_public_key) DO UPDATE SET runtime_profile_id=excluded.runtime_profile_id,binding_id=excluded.binding_id,firmware_id=COALESCE(excluded.firmware_id,runtime_profile_owners.firmware_id)"), owner, profile.Id, uuid.NewString(), item.FirmwareId); err != nil {
		return Registration{}, err
	}
	if err := tx.Commit(); err != nil {
		return Registration{}, err
	}
	registration := Registration{TokenID: item.Id, RuntimeProfile: profile, FirmwareID: cloneString(item.FirmwareId)}
	if publish != nil {
		publish(registration)
	}
	return registration, nil
}

func sameOptionalString(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// ResolveOwnerFirmware returns the authoritative firmware selected by registration.
// A missing SQL binding preserves the Peer firmware from earlier registrations.
func (s *Server) ResolveOwnerFirmware(ctx context.Context, owner string) (*string, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	var firmware *string
	err = db.QueryRowContext(ctx, db.Rebind("SELECT firmware_id FROM runtime_profile_owners WHERE owner_public_key=?"), owner).Scan(&firmware)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve registration firmware binding: %w", err)
	}
	return firmware, nil
}
