package contact

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
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
	store, err := s.store()
	if err != nil {
		return rpcapi.ContactObject{}, err
	}
	if err := socialutil.RequireOwner(owner); err != nil {
		return rpcapi.ContactObject{}, err
	}
	displayName := strings.TrimSpace(socialutil.StringValue(req.DisplayName))
	phoneNumber := strings.TrimSpace(socialutil.StringValue(req.PhoneNumber))
	if displayName == "" && phoneNumber == "" {
		return rpcapi.ContactObject{}, errors.New("social: contact display_name or phone_number is required")
	}
	if phoneNumber != "" {
		if err := s.ensureUniquePhone(ctx, owner, "", phoneNumber); err != nil {
			return rpcapi.ContactObject{}, err
		}
	}
	now := s.now()
	id := s.newID()
	item := rpcapi.ContactObject{Id: &id, CreatedAt: &now, UpdatedAt: &now}
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
