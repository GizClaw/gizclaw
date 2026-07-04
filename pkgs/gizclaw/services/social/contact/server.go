package contact

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type Server struct {
	Store kv.Store

	Now   func() time.Time
	NewID func() string
}

func (s *Server) ListContacts(ctx context.Context, owner string, req rpcapi.ContactListRequest) (rpcapi.ContactListResponse, error) {
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactListResponse{}, err
	}
	prefix := socialutil.OwnerPrefix(socialutil.ContactsRoot, owner)
	entries, err := socialutil.ListPage(ctx, store, prefix, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.ContactListResponse{}, err
	}
	items := make([]rpcapi.ContactObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var item rpcapi.ContactObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.ContactListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.ContactListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) GetContact(ctx context.Context, owner string, req rpcapi.ContactGetRequest) (rpcapi.ContactObject, error) {
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	return socialutil.ReadJSONValue[rpcapi.ContactObject](ctx, store, socialutil.ContactKey(owner, req.Id))
}

func (s *Server) CreateContact(ctx context.Context, owner string, req rpcapi.ContactCreateRequest) (rpcapi.ContactObject, error) {
	return s.createContact(ctx, owner, s.newID(), req.DisplayName, req.PhoneNumber)
}

func (s *Server) AdminListContacts(ctx context.Context, owner string, cursor *string, limit *int) (adminservice.AdminContactListResponse, error) {
	store, err := s.store()
	if err != nil {
		return adminservice.AdminContactListResponse{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner != "" {
		page, err := s.ListContacts(ctx, owner, rpcapi.ContactListRequest{Cursor: cursor, Limit: limit})
		if err != nil {
			return adminservice.AdminContactListResponse{}, err
		}
		items := make([]adminservice.AdminContactObject, 0, len(page.Items))
		for _, item := range page.Items {
			items = append(items, adminContactObject(owner, item))
		}
		return adminservice.AdminContactListResponse{Items: items, HasNext: page.HasNext, NextCursor: page.NextCursor}, nil
	}

	_, pageLimit := socialutil.NormalizeListParams("", socialutil.IntValue(limit))
	entries, err := kv.ListAfter(ctx, store, socialutil.ContactsRoot, adminContactCursorAfter(socialutil.StringValue(cursor)), pageLimit+1)
	if err != nil {
		return adminservice.AdminContactListResponse{}, err
	}
	hasNext := len(entries) > pageLimit
	if hasNext {
		entries = entries[:pageLimit]
	}
	items := make([]adminservice.AdminContactObject, 0, len(entries))
	for _, entry := range entries {
		owner, ok := adminContactOwner(entry.Key)
		if !ok {
			continue
		}
		var item rpcapi.ContactObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return adminservice.AdminContactListResponse{}, err
		}
		items = append(items, adminContactObject(owner, item))
	}
	var next *string
	if hasNext && len(entries) > 0 {
		cursor := adminContactCursor(entries[len(entries)-1].Key)
		if cursor != "" {
			next = &cursor
		}
	}
	return adminservice.AdminContactListResponse{Items: items, HasNext: hasNext, NextCursor: next}, nil
}

func (s *Server) AdminCreateContact(ctx context.Context, req adminservice.AdminContactCreateRequest) (adminservice.AdminContactObject, error) {
	id := socialutil.StringValue(req.Id)
	if id == "" {
		id = s.newID()
	} else if err := customid.ValidateField("contact id", id); err != nil {
		return adminservice.AdminContactObject{}, err
	}
	item, err := s.createContact(ctx, req.OwnerPublicKey, id, req.DisplayName, req.PhoneNumber)
	if err != nil {
		return adminservice.AdminContactObject{}, err
	}
	return adminContactObject(req.OwnerPublicKey, item), nil
}

func (s *Server) AdminApplyContact(ctx context.Context, owner, id string, displayName, phoneNumber *string) (adminservice.AdminContactObject, error) {
	if err := customid.ValidateField("contact id", id); err != nil {
		return adminservice.AdminContactObject{}, err
	}
	item, err := s.upsertContact(ctx, owner, id, displayName, phoneNumber)
	if err != nil {
		return adminservice.AdminContactObject{}, err
	}
	return adminContactObject(owner, item), nil
}

func (s *Server) AdminGetContact(ctx context.Context, owner, id string) (adminservice.AdminContactObject, error) {
	item, err := s.GetContact(ctx, owner, rpcapi.ContactGetRequest{Id: strings.TrimSpace(id)})
	if err != nil {
		return adminservice.AdminContactObject{}, err
	}
	return adminContactObject(owner, item), nil
}

func (s *Server) AdminPutContact(ctx context.Context, owner, id string, req adminservice.AdminContactPutRequest) (adminservice.AdminContactObject, error) {
	item, err := s.PutContact(ctx, owner, rpcapi.ContactPutRequest{Id: strings.TrimSpace(id), DisplayName: req.DisplayName, PhoneNumber: req.PhoneNumber})
	if err != nil {
		return adminservice.AdminContactObject{}, err
	}
	return adminContactObject(owner, item), nil
}

func (s *Server) AdminDeleteContact(ctx context.Context, owner, id string) (adminservice.AdminContactObject, error) {
	item, err := s.DeleteContact(ctx, owner, rpcapi.ContactDeleteRequest{Id: strings.TrimSpace(id)})
	if err != nil {
		return adminservice.AdminContactObject{}, err
	}
	return adminContactObject(owner, item), nil
}

func (s *Server) createContact(ctx context.Context, owner, id string, displayNameValue, phoneNumberValue *string) (rpcapi.ContactObject, error) {
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	if err := socialutil.RequireOwner(owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact id is required")
	}
	displayName := strings.TrimSpace(socialutil.StringValue(displayNameValue))
	phoneNumber := strings.TrimSpace(socialutil.StringValue(phoneNumberValue))
	if displayName == "" && phoneNumber == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	if _, err := store.Get(ctx, socialutil.ContactKey(owner, id)); err == nil {
		return rpcapi.ContactObject{}, errors.New("social: contact id already exists")
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.ContactObject{}, err
	}
	if phoneNumber != "" {
		if err := s.ensureUniquePhone(ctx, owner, "", phoneNumber); err != nil {
			return rpcapi.ContactObject{}, err
		}
	}
	now := s.now()
	item := rpcapi.ContactObject{Id: &id, CreatedAt: &now, UpdatedAt: &now}
	if displayName != "" {
		item.DisplayName = &displayName
	}
	if phoneNumber != "" {
		item.PhoneNumber = &phoneNumber
	}
	return item, socialutil.WriteJSON(ctx, store, socialutil.ContactKey(owner, id), item)
}

func (s *Server) upsertContact(ctx context.Context, owner, id string, displayNameValue, phoneNumberValue *string) (rpcapi.ContactObject, error) {
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	if err := socialutil.RequireOwner(owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact id is required")
	}
	displayName := strings.TrimSpace(socialutil.StringValue(displayNameValue))
	phoneNumber := strings.TrimSpace(socialutil.StringValue(phoneNumberValue))
	if displayName == "" && phoneNumber == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	if phoneNumber != "" {
		if err := s.ensureUniquePhone(ctx, owner, id, phoneNumber); err != nil {
			return rpcapi.ContactObject{}, err
		}
	}
	now := s.now()
	item := rpcapi.ContactObject{Id: &id, CreatedAt: &now, UpdatedAt: &now}
	if existing, err := socialutil.ReadJSONValue[rpcapi.ContactObject](ctx, store, socialutil.ContactKey(owner, id)); err == nil {
		item.CreatedAt = existing.CreatedAt
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.ContactObject{}, err
	}
	if displayName != "" {
		item.DisplayName = &displayName
	}
	if phoneNumber != "" {
		item.PhoneNumber = &phoneNumber
	}
	return item, socialutil.WriteJSON(ctx, store, socialutil.ContactKey(owner, id), item)
}

func (s *Server) PutContact(ctx context.Context, owner string, req rpcapi.ContactPutRequest) (rpcapi.ContactObject, error) {
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	item, err := socialutil.ReadJSONValue[rpcapi.ContactObject](ctx, store, socialutil.ContactKey(owner, req.Id))
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	displayName := strings.TrimSpace(socialutil.StringValue(item.DisplayName))
	phoneNumber := strings.TrimSpace(socialutil.StringValue(item.PhoneNumber))
	if req.DisplayName != nil {
		displayName = strings.TrimSpace(*req.DisplayName)
	}
	if req.PhoneNumber != nil {
		phoneNumber = strings.TrimSpace(*req.PhoneNumber)
		if phoneNumber != "" {
			if err := s.ensureUniquePhone(ctx, owner, req.Id, phoneNumber); err != nil {
				return rpcapi.ContactObject{}, err
			}
		}
	}
	if displayName == "" && phoneNumber == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	item.DisplayName = socialutil.OptionalString(displayName)
	item.PhoneNumber = socialutil.OptionalString(phoneNumber)
	now := s.now()
	item.UpdatedAt = &now
	return item, socialutil.WriteJSON(ctx, store, socialutil.ContactKey(owner, req.Id), item)
}

func (s *Server) DeleteContact(ctx context.Context, owner string, req rpcapi.ContactDeleteRequest) (rpcapi.ContactObject, error) {
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	item, err := socialutil.ReadJSONValue[rpcapi.ContactObject](ctx, store, socialutil.ContactKey(owner, req.Id))
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	return item, store.Delete(ctx, socialutil.ContactKey(owner, req.Id))
}

func (s *Server) ensureUniquePhone(ctx context.Context, owner, currentID, phone string) error {
	if phone == "" {
		return nil
	}
	store, err := s.store()
	if err != nil {
		return err
	}
	normalized := socialutil.NormalizePhone(phone)
	for entry, err := range store.List(ctx, socialutil.OwnerPrefix(socialutil.ContactsRoot, owner)) {
		if err != nil {
			return err
		}
		var item rpcapi.ContactObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return err
		}
		if socialutil.StringValue(item.Id) != currentID && socialutil.NormalizePhone(socialutil.StringValue(item.PhoneNumber)) == normalized {
			return errors.New("social: contact phone_number already exists")
		}
	}
	return nil
}

func (s *Server) store() (kv.Store, error) {
	if s == nil || s.Store == nil {
		return nil, errors.New("social: contact service not configured")
	}
	return s.Store, nil
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

func adminContactObject(owner string, item rpcapi.ContactObject) adminservice.AdminContactObject {
	return adminservice.AdminContactObject{
		OwnerPublicKey: strings.TrimSpace(owner),
		Id:             socialutil.StringValue(item.Id),
		DisplayName:    item.DisplayName,
		PhoneNumber:    item.PhoneNumber,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func adminContactOwner(key kv.Key) (string, bool) {
	if len(key) < 3 {
		return "", false
	}
	return socialutil.UnescapeStoreSegment(key[1]), true
}

func adminContactCursor(key kv.Key) string {
	if len(key) < 3 {
		return ""
	}
	return key[1] + "/" + key[2]
}

func adminContactCursorAfter(cursor string) kv.Key {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil
	}
	parts := strings.Split(cursor, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return append(append(kv.Key{}, socialutil.ContactsRoot...), parts[0], parts[1])
}
