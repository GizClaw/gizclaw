package friend

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

type Server struct {
	Requests kv.Store
	Friends  kv.Store

	FriendOTPTTL time.Duration

	Now   func() time.Time
	NewID func() string
}

type otpRecord struct {
	PeerID    string    `json:"peer_id"`
	CodeHash  string    `json:"code_hash"`
	ExpiresAt time.Time `json:"expires_at"`
	Consumed  bool      `json:"consumed"`
}

func (s *Server) ReportFriendOTP(ctx context.Context, peerID, code string) error {
	store, err := s.requestsStore()
	if err != nil {
		return err
	}
	peerID = strings.TrimSpace(peerID)
	if peerID == "" {
		return errors.New("social: peer id is required")
	}
	if !socialutil.IsSixDigitCode(code) {
		return errors.New("social: friend otp must be exactly 6 digits")
	}
	record := otpRecord{
		PeerID:    peerID,
		CodeHash:  socialutil.HashCode(code),
		ExpiresAt: s.now().Add(s.friendOTPTTL()),
	}
	return socialutil.WriteJSON(ctx, store, socialutil.FriendOTPKey(peerID), record)
}

func (s *Server) CreateFriendRequest(ctx context.Context, owner string, req rpcapi.FriendRequestCreateRequest) (rpcapi.FriendRequestObject, error) {
	store, err := s.requestsStore()
	if err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	owner = strings.TrimSpace(owner)
	to := strings.TrimSpace(req.ToPeerId)
	if owner == "" || to == "" {
		return rpcapi.FriendRequestObject{}, errors.New("social: friend request peers are required")
	}
	if owner == to {
		return rpcapi.FriendRequestObject{}, errors.New("social: cannot friend self")
	}
	if _, err := s.GetFriendRelation(ctx, owner, socialutil.RelationID(owner, to)); err == nil {
		return rpcapi.FriendRequestObject{}, errors.New("social: peers are already friends")
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendRequestObject{}, err
	}
	if existing, ok, err := s.pendingRequest(ctx, owner, to); err != nil {
		return rpcapi.FriendRequestObject{}, err
	} else if ok {
		return existing, nil
	}
	if err := s.consumeOTP(ctx, to, req.Code); err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	now := s.now()
	id := s.newID()
	state := rpcapi.FriendRequestStatePending
	item := rpcapi.FriendRequestObject{
		Id:         &id,
		FromPeerId: &owner,
		ToPeerId:   &to,
		Message:    socialutil.OptionalString(strings.TrimSpace(socialutil.StringValue(req.Message))),
		State:      &state,
		CreatedAt:  &now,
		UpdatedAt:  &now,
	}
	return item, socialutil.WriteJSON(ctx, store, socialutil.FriendRequestKey(id), item)
}

func (s *Server) ListFriendRequests(ctx context.Context, owner string, req rpcapi.FriendRequestListRequest) (rpcapi.FriendRequestListResponse, error) {
	store, err := s.requestsStore()
	if err != nil {
		return rpcapi.FriendRequestListResponse{}, err
	}
	box := "all"
	if req.Box != nil && string(*req.Box) != "" {
		if !req.Box.Valid() {
			return rpcapi.FriendRequestListResponse{}, errors.New("social: invalid friend request box")
		}
		box = string(*req.Box)
	}
	if req.State != nil && !req.State.Valid() {
		return rpcapi.FriendRequestListResponse{}, errors.New("social: invalid friend request state")
	}
	items := make([]rpcapi.FriendRequestObject, 0)
	for entry, err := range store.List(ctx, socialutil.FriendRequestsRoot) {
		if err != nil {
			return rpcapi.FriendRequestListResponse{}, err
		}
		var item rpcapi.FriendRequestObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendRequestListResponse{}, err
		}
		if !socialutil.FriendRequestVisible(item, owner, box) {
			continue
		}
		if req.State != nil && (item.State == nil || *item.State != *req.State) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return socialutil.CompareByCreatedAtAsc(socialutil.TimeValue(items[i].CreatedAt), socialutil.StringValue(items[i].Id), socialutil.TimeValue(items[j].CreatedAt), socialutil.StringValue(items[j].Id))
	})
	page := socialutil.PageItems(items, socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit), func(item rpcapi.FriendRequestObject) string {
		return socialutil.StringValue(item.Id)
	})
	return rpcapi.FriendRequestListResponse{Items: page.Items, HasNext: page.HasNext, NextCursor: page.NextCursor}, nil
}

func (s *Server) AcceptFriendRequest(ctx context.Context, owner string, req rpcapi.FriendRequestAcceptRequest) (rpcapi.FriendRequestObject, error) {
	return s.transitionRequest(ctx, owner, req.Id, rpcapi.FriendRequestStateAccepted)
}

func (s *Server) RejectFriendRequest(ctx context.Context, owner string, req rpcapi.FriendRequestRejectRequest) (rpcapi.FriendRequestObject, error) {
	return s.transitionRequest(ctx, owner, req.Id, rpcapi.FriendRequestStateRejected)
}

func (s *Server) ListFriends(ctx context.Context, owner string, req rpcapi.FriendListRequest) (rpcapi.FriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	entries, err := socialutil.ListPage(ctx, store, socialutil.OwnerPrefix(socialutil.FriendsRoot, owner), socialutil.StringValue(req.Cursor), socialutil.IntValue(req.Limit))
	if err != nil {
		return rpcapi.FriendListResponse{}, err
	}
	items := make([]rpcapi.FriendObject, 0, len(entries.Items))
	for _, entry := range entries.Items {
		var item rpcapi.FriendObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendListResponse{}, err
		}
		items = append(items, item)
	}
	return rpcapi.FriendListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) DeleteFriend(ctx context.Context, owner string, req rpcapi.FriendDeleteRequest) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	item, err := s.GetFriendRelation(ctx, owner, req.Id)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	other := socialutil.StringValue(item.PeerId)
	if err := store.BatchDelete(ctx, []kv.Key{socialutil.FriendKey(owner, req.Id), socialutil.FriendKey(other, req.Id)}); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return item, nil
}

func (s *Server) GetFriendRelation(ctx context.Context, owner, id string) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	return socialutil.ReadJSONValue[rpcapi.FriendObject](ctx, store, socialutil.FriendKey(owner, id))
}

func (s *Server) transitionRequest(ctx context.Context, owner, id string, next rpcapi.FriendRequestState) (rpcapi.FriendRequestObject, error) {
	store, err := s.requestsStore()
	if err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	item, err := socialutil.ReadJSONValue[rpcapi.FriendRequestObject](ctx, store, socialutil.FriendRequestKey(id))
	if err != nil {
		return rpcapi.FriendRequestObject{}, err
	}
	if socialutil.StringValue(item.ToPeerId) != owner {
		return rpcapi.FriendRequestObject{}, errors.New("social: only receiver can transition friend request")
	}
	if next == rpcapi.FriendRequestStateAccepted && item.State != nil && *item.State == rpcapi.FriendRequestStateAccepted {
		return item, nil
	}
	if item.State == nil || *item.State != rpcapi.FriendRequestStatePending {
		return rpcapi.FriendRequestObject{}, errors.New("social: friend request is not pending")
	}
	now := s.now()
	item.State = &next
	item.UpdatedAt = &now
	item.RespondedAt = &now
	if next == rpcapi.FriendRequestStateAccepted {
		if err := s.createFriendRows(ctx, item); err != nil {
			return rpcapi.FriendRequestObject{}, err
		}
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.FriendRequestKey(id), item); err != nil {
		if next == rpcapi.FriendRequestStateAccepted {
			rollbackErr := s.deleteFriendRows(ctx, item)
			return rpcapi.FriendRequestObject{}, errors.Join(err, rollbackErr)
		}
		return rpcapi.FriendRequestObject{}, err
	}
	return item, nil
}

func (s *Server) createFriendRows(ctx context.Context, req rpcapi.FriendRequestObject) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	from, to, requestID := socialutil.StringValue(req.FromPeerId), socialutil.StringValue(req.ToPeerId), socialutil.StringValue(req.Id)
	rel := socialutil.RelationID(from, to)
	now := s.now()
	entries := make([]kv.Entry, 0, 2)
	for _, row := range []struct{ owner, peer string }{{from, to}, {to, from}} {
		item := rpcapi.FriendObject{Id: &rel, PeerId: &row.peer, RequestId: &requestID, CreatedAt: &now, UpdatedAt: &now}
		data, err := json.Marshal(item)
		if err != nil {
			return err
		}
		entries = append(entries, kv.Entry{Key: socialutil.FriendKey(row.owner, rel), Value: data})
	}
	return store.BatchSet(ctx, entries)
}

func (s *Server) deleteFriendRows(ctx context.Context, req rpcapi.FriendRequestObject) error {
	store, err := s.friendsStore()
	if err != nil {
		return err
	}
	from, to := socialutil.StringValue(req.FromPeerId), socialutil.StringValue(req.ToPeerId)
	rel := socialutil.RelationID(from, to)
	return store.BatchDelete(ctx, []kv.Key{socialutil.FriendKey(from, rel), socialutil.FriendKey(to, rel)})
}

func (s *Server) consumeOTP(ctx context.Context, peerID, code string) error {
	if !socialutil.IsSixDigitCode(code) {
		return errors.New("social: friend otp code must be exactly 6 digits")
	}
	store, err := s.requestsStore()
	if err != nil {
		return err
	}
	record, err := socialutil.ReadJSONValue[otpRecord](ctx, store, socialutil.FriendOTPKey(peerID))
	if err != nil {
		if !errors.Is(err, kv.ErrNotFound) {
			return err
		}
		return errors.New("social: friend otp not found")
	}
	if record.Consumed || !record.ExpiresAt.After(s.now()) || record.CodeHash != socialutil.HashCode(code) {
		return errors.New("social: invalid friend otp")
	}
	record.Consumed = true
	return socialutil.WriteJSON(ctx, store, socialutil.FriendOTPKey(peerID), record)
}

func (s *Server) pendingRequest(ctx context.Context, from, to string) (rpcapi.FriendRequestObject, bool, error) {
	store, err := s.requestsStore()
	if err != nil {
		return rpcapi.FriendRequestObject{}, false, err
	}
	for entry, err := range store.List(ctx, socialutil.FriendRequestsRoot) {
		if err != nil {
			return rpcapi.FriendRequestObject{}, false, err
		}
		var item rpcapi.FriendRequestObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return rpcapi.FriendRequestObject{}, false, err
		}
		if socialutil.StringValue(item.FromPeerId) == from && socialutil.StringValue(item.ToPeerId) == to && item.State != nil && *item.State == rpcapi.FriendRequestStatePending {
			return item, true, nil
		}
	}
	return rpcapi.FriendRequestObject{}, false, nil
}

func (s *Server) requestsStore() (kv.Store, error) {
	if s == nil || s.Requests == nil {
		return nil, errors.New("social: friend request service not configured")
	}
	return s.Requests, nil
}

func (s *Server) friendsStore() (kv.Store, error) {
	if s == nil || s.Friends == nil {
		return nil, errors.New("social: friend service not configured")
	}
	return s.Friends, nil
}

func (s *Server) friendOTPTTL() time.Duration {
	if s != nil && s.FriendOTPTTL > 0 {
		return s.FriendOTPTTL
	}
	return socialutil.DefaultFriendOTPTTL
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
