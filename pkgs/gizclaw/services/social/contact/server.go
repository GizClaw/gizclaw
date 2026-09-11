package contact

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/internal/keyedlock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const (
	PeerPendingDeletionCode = "PEER_PENDING_DELETION"
	PeerDeletedCode         = "PEER_DELETED"
	// ContactLimitReachedCode is the stable error code for ErrPeerContactLimit.
	ContactLimitReachedCode = "CONTACT_LIMIT_REACHED"
	// PeerContactLimit is the fixed number of Contacts one owner Peer may
	// have. Devices size their Contact arrays and offline allowlists to it and
	// reject longer lists, so it is deliberately not configurable.
	PeerContactLimit = 8
	contactColumns   = "id,owner_public_key,name,display_name,phone_number,created_at,updated_at,incarnation"
)

var (
	ErrPeerPendingDeletion = errors.New("social: Peer pending deletion")
	ErrPeerDeleted         = errors.New("social: Peer deleted")
	// ErrNotFound means no Contact matches the requested identity and owner.
	ErrNotFound = errors.New("social: contact not found")
	// ErrPeerContactLimit means the owner already has PeerContactLimit Contacts.
	ErrPeerContactLimit = errors.New("social: peer contact limit reached")
)

// Server owns the SQL Contact catalog and owner admission coordination.
type Server struct {
	DB               *sqlx.DB
	PeerAvailability func(context.Context, string) error
	Now              func() time.Time
	NewID            func() string
	ownerLocks       keyedlock.Locker[string]
}

// PeerRetirementContact identifies one creation instance owned by a retiring Peer.
type PeerRetirementContact struct {
	Owner       string `json:"owner"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Incarnation string `json:"incarnation"`
}

type contactRow struct {
	ID, Owner, Incarnation string
	Item                   rpcapi.ContactObject
}

// Initialize creates business constraints and indexes once at Server startup.
func (s *Server) Initialize(ctx context.Context) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, query := range []string{
		`CREATE TABLE IF NOT EXISTS contacts (
 id TEXT PRIMARY KEY CHECK(length(id)>0), owner_public_key TEXT NOT NULL CHECK(length(owner_public_key)>0),
 name TEXT NOT NULL CHECK(length(name)>0),display_name TEXT,phone_number TEXT,normalized_phone TEXT,
 created_at TEXT NOT NULL,updated_at TEXT NOT NULL,incarnation TEXT NOT NULL,
 UNIQUE(owner_public_key,name),UNIQUE(owner_public_key,normalized_phone),
 CHECK(display_name IS NOT NULL OR phone_number IS NOT NULL))`,
		`CREATE INDEX IF NOT EXISTS contacts_owner_id ON contacts(owner_public_key,id)`,
	} {
		if _, err := tx.ExecContext(ctx, query); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Server) SnapshotPeerContacts(ctx context.Context, owner string) ([]PeerRetirementContact, error) {
	db, err := s.database()
	if err != nil {
		return nil, err
	}
	if err := socialutil.RequireOwner(owner); err != nil {
		return nil, err
	}
	if owner != strings.TrimSpace(owner) {
		return nil, errors.New("social: Peer public key must be canonical")
	}
	release, err := s.ownerLocks.Acquire(ctx, owner)
	if err != nil {
		return nil, err
	}
	defer release()
	var result []PeerRetirementContact
	cursor := ""
	for {
		page, err := snapshotContactPage(ctx, db, owner, cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if len(page) < 256 {
			return result, nil
		}
		cursor = page[len(page)-1].ID
	}
}

func snapshotContactPage(ctx context.Context, db *sqlx.DB, owner, cursor string) ([]PeerRetirementContact, error) {
	rows, err := db.QueryContext(ctx, db.Rebind(`SELECT id,name,incarnation FROM contacts WHERE owner_public_key=? AND id>? ORDER BY id LIMIT 256`), owner, cursor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PeerRetirementContact, 0, 256)
	for rows.Next() {
		item := PeerRetirementContact{Owner: owner}
		if err := rows.Scan(&item.ID, &item.Name, &item.Incarnation); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Server) RetirePeerContact(ctx context.Context, snapshot PeerRetirementContact) error {
	db, err := s.database()
	if err != nil {
		return err
	}
	if err := socialutil.RequireOwner(snapshot.Owner); err != nil {
		return err
	}
	if err := customid.ValidateResourceID(snapshot.ID); err != nil || snapshot.Name == "" || snapshot.Name != strings.TrimSpace(snapshot.Name) || snapshot.Incarnation == "" {
		return errors.New("social: invalid Contact Peer retirement snapshot")
	}
	release, err := s.ownerLocks.Acquire(ctx, snapshot.Owner)
	if err != nil {
		return err
	}
	defer release()
	result, err := db.ExecContext(ctx, db.Rebind(`DELETE FROM contacts WHERE id=? AND owner_public_key=? AND name=? AND incarnation=?`), snapshot.ID, snapshot.Owner, snapshot.Name, snapshot.Incarnation)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 1 {
		return nil
	}
	var exists int
	err = db.QueryRowContext(ctx, db.Rebind(`SELECT 1 FROM contacts WHERE id=?`), snapshot.ID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("social: Contact no longer matches Peer retirement snapshot")
}

func (s *Server) ListContacts(ctx context.Context, owner string, req rpcapi.ContactListRequest) (rpcapi.ContactListResponse, error) {
	if err := socialutil.RequireOwner(owner); err != nil {
		return rpcapi.ContactListResponse{}, err
	}
	rows, more, next, err := s.listContacts(ctx, strings.TrimSpace(owner), socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.ContactListResponse{}, err
	}
	items := make([]rpcapi.ContactObject, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.Item)
	}
	return rpcapi.ContactListResponse{Items: items, HasNext: more, NextCursor: next}, nil
}

func (s *Server) AdminListContacts(ctx context.Context, owner string, cursor *string, limit *int) (adminhttp.AdminContactListResponse, error) {
	rows, more, next, err := s.listContacts(ctx, strings.TrimSpace(owner), socialutil.StringValue(cursor), socialutil.IntValue(limit))
	if err != nil {
		return adminhttp.AdminContactListResponse{}, err
	}
	items := make([]adminhttp.AdminContactObject, 0, len(rows))
	for _, row := range rows {
		items = append(items, adminContactObject(row.Owner, row.ID, row.Item))
	}
	return adminhttp.AdminContactListResponse{Items: items, HasNext: more, NextCursor: next}, nil
}

func (s *Server) listContacts(ctx context.Context, owner, cursor string, limit int) ([]contactRow, bool, *string, error) {
	db, err := s.database()
	if err != nil {
		return nil, false, nil, err
	}
	_, limit = socialutil.NormalizeListParams("", limit)
	query := `SELECT ` + contactColumns + ` FROM contacts WHERE `
	args := []any{}
	if owner != "" {
		query += `owner_public_key=? AND id>?`
		args = append(args, owner, cursor)
	} else {
		after := [2]string{}
		if cursor != "" {
			data, err := base64.RawURLEncoding.DecodeString(cursor)
			if err != nil {
				return nil, false, nil, errors.New("social: invalid Contact cursor")
			}
			if err := json.Unmarshal(data, &after); err != nil {
				return nil, false, nil, errors.New("social: invalid Contact cursor")
			}
		}
		query += `(owner_public_key,id)>(?,?)`
		args = append(args, after[0], after[1])
	}
	query += ` ORDER BY owner_public_key,id LIMIT ?`
	args = append(args, limit+1)
	rows, err := db.QueryContext(ctx, db.Rebind(query), args...)
	if err != nil {
		return nil, false, nil, err
	}
	defer rows.Close()
	result := make([]contactRow, 0, limit+1)
	for rows.Next() {
		row, err := scanContact(rows)
		if err != nil {
			return nil, false, nil, err
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(result) <= limit {
		return result, false, nil, nil
	}
	result = result[:limit]
	last := result[len(result)-1]
	next := last.ID
	if owner == "" {
		data, err := json.Marshal([2]string{last.Owner, last.ID})
		if err != nil {
			return nil, false, nil, err
		}
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	return result, true, &next, nil
}

func (s *Server) GetContact(ctx context.Context, owner string, req rpcapi.ContactGetRequest) (rpcapi.ContactObject, error) {
	db, err := s.database()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	row, err := scanContact(db.QueryRowContext(ctx, db.Rebind(`SELECT `+contactColumns+` FROM contacts WHERE owner_public_key=? AND name=?`), strings.TrimSpace(owner), strings.TrimSpace(req.Name)))
	return row.Item, err
}

func (s *Server) CreateContact(ctx context.Context, owner string, req rpcapi.ContactCreateRequest) (rpcapi.ContactObject, error) {
	return s.createContact(ctx, owner, s.newID(), req.Name, req.DisplayName, req.PhoneNumber)
}

func (s *Server) createContact(ctx context.Context, owner, id, name string, displayValue, phoneValue *string) (rpcapi.ContactObject, error) {
	db, err := s.database()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	if err := socialutil.RequireOwner(owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	owner = strings.TrimSpace(owner)
	if err := customid.ValidateResourceID(id); err != nil {
		return rpcapi.ContactObject{}, fmt.Errorf("social: contact id: %w", err)
	}
	if name == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact name is required")
	}
	if name != strings.TrimSpace(name) {
		return rpcapi.ContactObject{}, errors.New("social: contact name must not contain surrounding whitespace")
	}
	display := socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(displayValue)))
	phone := socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(phoneValue)))
	if display == nil && phone == nil {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	release, err := s.ownerLocks.Acquire(ctx, owner)
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	defer release()
	if err := s.ensurePeerAvailable(ctx, owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	// The owner lock serializes this count with the insert below.
	var existing int
	if err := db.QueryRowContext(ctx, db.Rebind(`SELECT COUNT(*) FROM contacts WHERE owner_public_key=?`), owner).Scan(&existing); err != nil {
		return rpcapi.ContactObject{}, err
	}
	if existing >= PeerContactLimit {
		return rpcapi.ContactObject{}, fmt.Errorf("%w: %d contacts", ErrPeerContactLimit, existing)
	}
	now := s.now()
	item := rpcapi.ContactObject{Name: name, DisplayName: display, PhoneNumber: phone, CreatedAt: &now, UpdatedAt: &now}
	result, err := db.ExecContext(ctx, db.Rebind(`INSERT INTO contacts(id,owner_public_key,name,display_name,phone_number,normalized_phone,created_at,updated_at,incarnation) VALUES (?,?,?,?,?,?,?,?,?) ON CONFLICT DO NOTHING`), id, owner, name, display, phone, normalizedPhone(phone), now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), uuid.NewString())
	if err != nil {
		return rpcapi.ContactObject{}, contactSQLError(err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	if count == 0 {
		return rpcapi.ContactObject{}, fmt.Errorf("%w: contact id, name or phone_number", socialutil.ErrResourceAlreadyExists)
	}
	return item, nil
}

func (s *Server) PutContact(ctx context.Context, owner string, req rpcapi.ContactPutRequest) (rpcapi.ContactObject, error) {
	return s.putContact(ctx, owner, strings.TrimSpace(req.Name), req.DisplayName, req.PhoneNumber, true)
}
func (s *Server) putContactByID(ctx context.Context, owner, id string, display, phone *string) (rpcapi.ContactObject, error) {
	return s.putContact(ctx, owner, id, display, phone, false)
}
func (s *Server) putContact(ctx context.Context, owner, id string, displayValue, phoneValue *string, byName bool) (rpcapi.ContactObject, error) {
	db, err := s.database()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	owner = strings.TrimSpace(owner)
	release, err := s.ownerLocks.Acquire(ctx, owner)
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	defer release()
	if err := s.ensurePeerAvailable(ctx, owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	display := socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(displayValue)))
	phone := socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(phoneValue)))
	field := "id"
	if byName {
		field = "name"
	}
	row, err := scanContact(db.QueryRowContext(ctx, db.Rebind(`UPDATE contacts SET display_name=CASE WHEN ? THEN ? ELSE display_name END,phone_number=CASE WHEN ? THEN ? ELSE phone_number END,normalized_phone=CASE WHEN ? THEN ? ELSE normalized_phone END,updated_at=? WHERE owner_public_key=? AND `+field+`=? RETURNING `+contactColumns), displayValue != nil, display, phoneValue != nil, phone, phoneValue != nil, normalizedPhone(phone), s.now().Format(time.RFC3339Nano), owner, id))
	return row.Item, contactSQLError(err)
}

func (s *Server) DeleteContact(ctx context.Context, owner string, req rpcapi.ContactDeleteRequest) (rpcapi.ContactObject, error) {
	return s.deleteContact(ctx, owner, strings.TrimSpace(req.Name), true)
}
func (s *Server) deleteContactByID(ctx context.Context, owner, id string) (rpcapi.ContactObject, error) {
	return s.deleteContact(ctx, owner, id, false)
}
func (s *Server) deleteContact(ctx context.Context, owner, id string, byName bool) (rpcapi.ContactObject, error) {
	db, err := s.database()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	owner = strings.TrimSpace(owner)
	release, err := s.ownerLocks.Acquire(ctx, owner)
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	defer release()
	if err := s.ensurePeerAvailable(ctx, owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	field := "id"
	if byName {
		field = "name"
	}
	row, err := scanContact(db.QueryRowContext(ctx, db.Rebind(`DELETE FROM contacts WHERE owner_public_key=? AND `+field+`=? RETURNING `+contactColumns), owner, id))
	return row.Item, err
}

func normalizedPhone(phone *string) *string {
	if phone == nil {
		return nil
	}
	normalized := socialutil.NormalizePhone(*phone)
	return &normalized
}

func contactSQLError(err error) error {
	if err == nil {
		return nil
	}
	code := ""
	if typed, ok := errors.AsType[interface {
		error
		SQLState() string
	}](err); ok {
		code = typed.SQLState()
	}
	if typed, ok := errors.AsType[interface {
		error
		Code() int
	}](err); ok {
		switch typed.Code() {
		case 1555, 2067:
			code = "23505"
		case 275:
			code = "23514"
		}
	}
	switch code {
	case "23505":
		return fmt.Errorf("%w: contact name or phone_number", socialutil.ErrResourceAlreadyExists)
	case "23514":
		return errors.New("social: contact display_name or phone_number is required")
	}
	return err
}

func scanContact(row interface{ Scan(...any) error }) (contactRow, error) {
	var value contactRow
	var created, updated string
	err := row.Scan(&value.ID, &value.Owner, &value.Item.Name, &value.Item.DisplayName, &value.Item.PhoneNumber, &created, &updated, &value.Incarnation)
	if errors.Is(err, sql.ErrNoRows) {
		return value, ErrNotFound
	}
	if err != nil {
		return value, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return value, err
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return value, err
	}
	value.Item.CreatedAt = &createdAt
	value.Item.UpdatedAt = &updatedAt
	return value, nil
}

func (s *Server) database() (*sqlx.DB, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("social: contact service not configured")
	}
	switch s.DB.DriverName() {
	case "sqlite", "postgres":
	default:
		return nil, fmt.Errorf("contact: unsupported SQL driver %q", s.DB.DriverName())
	}
	return s.DB, nil
}
func (s *Server) ensurePeerAvailable(ctx context.Context, owner string) error {
	if s == nil || s.PeerAvailability == nil {
		return nil
	}
	return s.PeerAvailability(ctx, owner)
}
func (s *Server) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}
func (s *Server) newID() string {
	if s != nil && s.NewID != nil {
		return s.NewID()
	}
	return socialutil.NewID()
}
func (s *Server) readContactByID(ctx context.Context, owner, id string) (rpcapi.ContactObject, error) {
	db, err := s.database()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	row, err := scanContact(db.QueryRowContext(ctx, db.Rebind(`SELECT `+contactColumns+` FROM contacts WHERE owner_public_key=? AND id=?`), strings.TrimSpace(owner), id))
	return row.Item, err
}
func adminContactObject(owner, id string, item rpcapi.ContactObject) adminhttp.AdminContactObject {
	return adminhttp.AdminContactObject{OwnerPublicKey: strings.TrimSpace(owner), Id: id, Name: item.Name, DisplayName: item.DisplayName, PhoneNumber: item.PhoneNumber, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}

func (s *Server) AdminCreateContact(ctx context.Context, req adminhttp.AdminContactCreateRequest) (adminhttp.AdminContactObject, error) {
	id := req.Id
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminContactObject{}, fmt.Errorf("social: contact %w", err)
	}
	item, err := s.createContact(ctx, req.OwnerPublicKey, id, req.Name, req.DisplayName, req.PhoneNumber)
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return adminContactObject(req.OwnerPublicKey, id, item), nil
}

func (s *Server) AdminGetContact(ctx context.Context, owner, id string) (adminhttp.AdminContactObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminContactObject{}, fmt.Errorf("social: contact %w", err)
	}
	item, err := s.readContactByID(ctx, owner, id)
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return adminContactObject(owner, id, item), nil
}

func (s *Server) AdminGetContactByID(ctx context.Context, id string) (adminhttp.AdminContactObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminContactObject{}, fmt.Errorf("social: contact %w", err)
	}
	db, err := s.database()
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	row, err := scanContact(db.QueryRowContext(ctx, db.Rebind(`SELECT `+contactColumns+` FROM contacts WHERE id=?`), id))
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return adminContactObject(row.Owner, row.ID, row.Item), nil
}

func (s *Server) AdminPutContactByID(ctx context.Context, id string, req adminhttp.AdminContactPutRequest) (adminhttp.AdminContactObject, error) {
	item, err := s.AdminGetContactByID(ctx, id)
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return s.AdminPutContact(ctx, item.OwnerPublicKey, id, req)
}

func (s *Server) AdminDeleteContactByID(ctx context.Context, id string) (adminhttp.AdminContactObject, error) {
	item, err := s.AdminGetContactByID(ctx, id)
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return s.AdminDeleteContact(ctx, item.OwnerPublicKey, id)
}

func (s *Server) AdminPutContact(ctx context.Context, owner, id string, req adminhttp.AdminContactPutRequest) (adminhttp.AdminContactObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminContactObject{}, fmt.Errorf("social: contact %w", err)
	}
	item, err := s.putContactByID(ctx, owner, id, req.DisplayName, req.PhoneNumber)
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return adminContactObject(owner, id, item), nil
}

func (s *Server) AdminDeleteContact(ctx context.Context, owner, id string) (adminhttp.AdminContactObject, error) {
	if err := customid.ValidateResourceID(id); err != nil {
		return adminhttp.AdminContactObject{}, fmt.Errorf("social: contact %w", err)
	}
	item, err := s.deleteContactByID(ctx, owner, id)
	if err != nil {
		return adminhttp.AdminContactObject{}, err
	}
	return adminContactObject(owner, id, item), nil
}
