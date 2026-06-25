package friend

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

type WorkspaceService interface {
	CreateWorkspace(context.Context, adminservice.CreateWorkspaceRequestObject) (adminservice.CreateWorkspaceResponseObject, error)
	DeleteWorkspace(context.Context, adminservice.DeleteWorkspaceRequestObject) (adminservice.DeleteWorkspaceResponseObject, error)
}

type ACL interface {
	PutRole(context.Context, string, apitypes.ACLPermissionList) (apitypes.ACLRole, error)
	PutPolicyBinding(context.Context, string, float64, apitypes.ACLPolicy) (apitypes.ACLPolicyBinding, error)
	DeletePolicyBinding(context.Context, string) (apitypes.ACLPolicyBinding, error)
}

type Server struct {
	InviteTokens kv.Store
	Friends      kv.Store
	Workspaces   WorkspaceService
	ACL          ACL

	Now   func() time.Time
	NewID func() string
}

type inviteTokenRecord struct {
	PeerPublicKey string    `json:"peer_public_key"`
	InviteToken   string    `json:"invite_token"`
	CreatedAt     time.Time `json:"created_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (s *Server) GetFriendInviteToken(ctx context.Context, owner string, _ rpcapi.FriendInviteTokenGetRequest) (rpcapi.FriendInviteTokenGetResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenGetResponse{}, err
	}
	record, ok, err := s.activeInviteToken(ctx, store, strings.TrimSpace(owner))
	if err != nil || !ok {
		return rpcapi.FriendInviteTokenGetResponse{}, err
	}
	return rpcapi.FriendInviteTokenGetResponse{InviteToken: &record.InviteToken, ExpiresAt: &record.ExpiresAt}, nil
}

func (s *Server) CreateFriendInviteToken(ctx context.Context, owner string, _ rpcapi.FriendInviteTokenCreateRequest) (rpcapi.FriendInviteTokenCreateResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendInviteTokenCreateResponse{}, errors.New("social: peer public key is required")
	}
	if record, ok, err := s.activeInviteToken(ctx, store, owner); err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	} else if ok {
		return rpcapi.FriendInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
	}
	now := s.now()
	record := inviteTokenRecord{
		PeerPublicKey: owner,
		InviteToken:   s.newID(),
		CreatedAt:     now,
		ExpiresAt:     now.Add(s.inviteTokenTTL()),
	}
	if strings.TrimSpace(record.InviteToken) == "" {
		return rpcapi.FriendInviteTokenCreateResponse{}, errors.New("social: invite token is empty")
	}
	if err := socialutil.WriteJSON(ctx, store, socialutil.FriendInviteTokenKey(owner), record); err != nil {
		return rpcapi.FriendInviteTokenCreateResponse{}, err
	}
	return rpcapi.FriendInviteTokenCreateResponse{InviteToken: record.InviteToken, ExpiresAt: record.ExpiresAt}, nil
}

func (s *Server) ClearFriendInviteToken(ctx context.Context, owner string, _ rpcapi.FriendInviteTokenClearRequest) (rpcapi.FriendInviteTokenClearResponse, error) {
	store, err := s.inviteTokensStore()
	if err != nil {
		return rpcapi.FriendInviteTokenClearResponse{}, err
	}
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendInviteTokenClearResponse{}, errors.New("social: peer public key is required")
	}
	if err := store.Delete(ctx, socialutil.FriendInviteTokenKey(owner)); err != nil && !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendInviteTokenClearResponse{}, err
	}
	return rpcapi.FriendInviteTokenClearResponse{}, nil
}

func (s *Server) AddFriend(ctx context.Context, owner string, req rpcapi.FriendAddRequest) (rpcapi.FriendAddResponse, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return rpcapi.FriendAddResponse{}, errors.New("social: peer public key is required")
	}
	record, err := s.findInviteToken(ctx, strings.TrimSpace(req.InviteToken))
	if err != nil {
		return rpcapi.FriendAddResponse{}, err
	}
	to := strings.TrimSpace(record.PeerPublicKey)
	if to == "" {
		return rpcapi.FriendAddResponse{}, errors.New("social: invite token owner is empty")
	}
	if owner == to {
		return rpcapi.FriendAddResponse{}, errors.New("social: cannot friend self")
	}
	relationID := socialutil.RelationID(owner, to)
	if existing, err := s.GetFriendRelation(ctx, owner, relationID); err == nil {
		return existing, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendAddResponse{}, err
	}
	workspaceName, rollback, err := s.ensureDirectChatWorkspace(ctx, owner, to)
	if err != nil {
		return rpcapi.FriendAddResponse{}, err
	}
	friend, err := s.createFriendRows(ctx, owner, to, workspaceName)
	if err != nil {
		if rollback != nil {
			rollback()
		}
		return rpcapi.FriendAddResponse{}, err
	}
	return friend, nil
}

func (s *Server) AdminCreateFriend(ctx context.Context, owner string, peerPublicKey string) (rpcapi.FriendObject, error) {
	owner = strings.TrimSpace(owner)
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if owner == "" || peerPublicKey == "" {
		return rpcapi.FriendObject{}, errors.New("social: friend peers are required")
	}
	if owner == peerPublicKey {
		return rpcapi.FriendObject{}, errors.New("social: cannot friend self")
	}
	relationID := socialutil.RelationID(owner, peerPublicKey)
	if existing, err := s.GetFriendRelation(ctx, owner, relationID); err == nil {
		return existing, nil
	} else if !errors.Is(err, kv.ErrNotFound) {
		return rpcapi.FriendObject{}, err
	}
	workspaceName, rollback, err := s.ensureDirectChatWorkspace(ctx, owner, peerPublicKey)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	friend, err := s.createFriendRows(ctx, owner, peerPublicKey, workspaceName)
	if err != nil {
		if rollback != nil {
			rollback()
		}
		return rpcapi.FriendObject{}, err
	}
	return friend, nil
}

func (s *Server) AdminListFriends(ctx context.Context, cursor *string, limit *int) (adminservice.AdminFriendListResponse, error) {
	store, err := s.friendsStore()
	if err != nil {
		return adminservice.AdminFriendListResponse{}, err
	}
	_, pageLimit := socialutil.NormalizeListParams("", socialutil.IntValue(limit))
	entries, err := kv.ListAfter(ctx, store, socialutil.FriendsRoot, adminFriendCursorAfter(socialutil.StringValue(cursor)), pageLimit+1)
	if err != nil {
		return adminservice.AdminFriendListResponse{}, err
	}
	hasNext := len(entries) > pageLimit
	if hasNext {
		entries = entries[:pageLimit]
	}
	items := make([]adminservice.AdminFriendObject, 0, len(entries))
	for _, entry := range entries {
		owner, ok := adminFriendOwner(entry.Key)
		if !ok {
			continue
		}
		var item rpcapi.FriendObject
		if err := json.Unmarshal(entry.Value, &item); err != nil {
			return adminservice.AdminFriendListResponse{}, err
		}
		item = friendObjectForOwner(owner, item)
		items = append(items, adminFriendObject(owner, item))
	}
	var next *string
	if hasNext && len(entries) > 0 {
		cursor := adminFriendCursor(entries[len(entries)-1].Key)
		if cursor != "" {
			next = &cursor
		}
	}
	return adminservice.AdminFriendListResponse{Items: items, HasNext: hasNext, NextCursor: next}, nil
}

func (s *Server) AdminCreateFriendResource(ctx context.Context, owner string, peerPublicKey string) (adminservice.AdminFriendObject, error) {
	item, err := s.AdminCreateFriend(ctx, owner, peerPublicKey)
	if err != nil {
		return adminservice.AdminFriendObject{}, err
	}
	return adminFriendObject(strings.TrimSpace(owner), item), nil
}

func (s *Server) AdminGetFriend(ctx context.Context, owner, id string) (adminservice.AdminFriendObject, error) {
	item, err := s.GetFriendRelation(ctx, owner, id)
	if err != nil {
		return adminservice.AdminFriendObject{}, err
	}
	return adminFriendObject(strings.TrimSpace(owner), item), nil
}

func (s *Server) AdminDeleteFriend(ctx context.Context, owner, id string) (adminservice.AdminFriendObject, error) {
	item, err := s.DeleteFriend(ctx, owner, rpcapi.FriendDeleteRequest{Id: strings.TrimSpace(id)})
	if err != nil {
		return adminservice.AdminFriendObject{}, err
	}
	return adminFriendObject(strings.TrimSpace(owner), item), nil
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
		item = friendObjectForOwner(owner, item)
		items = append(items, item)
	}
	return rpcapi.FriendListResponse{Items: items, HasNext: entries.HasNext, NextCursor: entries.NextCursor}, nil
}

func (s *Server) DeleteFriend(ctx context.Context, owner string, req rpcapi.FriendDeleteRequest) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	relationID := friendRelationID(owner, req.Id)
	item, err := s.GetFriendRelation(ctx, owner, req.Id)
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	if err := s.deleteDirectChatWorkspace(ctx, owner, item); err != nil {
		return rpcapi.FriendObject{}, err
	}
	other := socialutil.StringValue(item.PeerPublicKey)
	if err := store.BatchDelete(ctx, []kv.Key{socialutil.FriendKey(owner, relationID), socialutil.FriendKey(other, relationID)}); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return friendObjectForOwner(owner, item), nil
}

func (s *Server) GetFriendRelation(ctx context.Context, owner, id string) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	item, err := socialutil.ReadJSONValue[rpcapi.FriendObject](ctx, store, socialutil.FriendKey(owner, friendRelationID(owner, id)))
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	return friendObjectForOwner(owner, item), nil
}

func friendRelationID(owner, id string) string {
	id = strings.TrimSpace(id)
	if strings.Contains(id, ":") {
		return id
	}
	return socialutil.RelationID(owner, id)
}

func friendObjectForOwner(owner string, item rpcapi.FriendObject) rpcapi.FriendObject {
	peer := strings.TrimSpace(socialutil.StringValue(item.PeerPublicKey))
	if peer == "" {
		peer = relationPeer(owner, socialutil.StringValue(item.Id))
	}
	if peer != "" {
		item.Id = &peer
		item.PeerPublicKey = &peer
	}
	return item
}

func relationPeer(owner, relationID string) string {
	owner = strings.TrimSpace(owner)
	parts := strings.Split(strings.TrimSpace(relationID), ":")
	if len(parts) != 2 {
		return ""
	}
	switch {
	case parts[0] == owner:
		return parts[1]
	case parts[1] == owner:
		return parts[0]
	default:
		return ""
	}
}

func adminFriendObject(owner string, item rpcapi.FriendObject) adminservice.AdminFriendObject {
	return adminservice.AdminFriendObject{
		OwnerPublicKey: strings.TrimSpace(owner),
		Id:             socialutil.StringValue(item.Id),
		PeerPublicKey:  socialutil.StringValue(item.PeerPublicKey),
		WorkspaceName:  socialutil.StringValue(item.WorkspaceName),
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func adminFriendOwner(key kv.Key) (string, bool) {
	if len(key) < 3 {
		return "", false
	}
	return socialutil.UnescapeStoreSegment(key[1]), true
}

func adminFriendCursor(key kv.Key) string {
	if len(key) < 3 {
		return ""
	}
	return key[1] + "/" + key[2]
}

func adminFriendCursorAfter(cursor string) kv.Key {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return nil
	}
	parts := strings.Split(cursor, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return append(append(kv.Key{}, socialutil.FriendsRoot...), parts[0], parts[1])
}

func (s *Server) createFriendRows(ctx context.Context, from, to, workspaceName string) (rpcapi.FriendObject, error) {
	store, err := s.friendsStore()
	if err != nil {
		return rpcapi.FriendObject{}, err
	}
	rel := socialutil.RelationID(from, to)
	now := s.now()
	entries := make([]kv.Entry, 0, 2)
	var ownerRow rpcapi.FriendObject
	for _, row := range []struct{ owner, peer string }{{from, to}, {to, from}} {
		peer := row.peer
		item := rpcapi.FriendObject{Id: &peer, PeerPublicKey: &peer, WorkspaceName: &workspaceName, CreatedAt: &now, UpdatedAt: &now}
		if row.owner == from {
			ownerRow = item
		}
		data, err := json.Marshal(item)
		if err != nil {
			return rpcapi.FriendObject{}, err
		}
		entries = append(entries, kv.Entry{Key: socialutil.FriendKey(row.owner, rel), Value: data})
	}
	if err := store.BatchSet(ctx, entries); err != nil {
		return rpcapi.FriendObject{}, err
	}
	return ownerRow, nil
}

func (s *Server) ensureDirectChatWorkspace(ctx context.Context, from, to string) (string, func(), error) {
	if from == "" || to == "" {
		return "", nil, errors.New("social: friend peers are required")
	}
	workspaceName := socialutil.DirectWorkspaceName(socialutil.RelationID(from, to))
	created := false
	if s.Workspaces != nil {
		body := adminservice.WorkspaceUpsert{
			Name:         workspaceName,
			WorkflowName: socialutil.ChatRoomWorkflowName,
			Parameters:   socialutil.ChatRoomWorkspaceParameters(apitypes.ChatRoomModeDirect),
		}
		resp, err := s.Workspaces.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &body})
		if err != nil {
			return "", nil, err
		}
		switch resp.(type) {
		case adminservice.CreateWorkspace200JSONResponse:
			created = true
		case adminservice.CreateWorkspace409JSONResponse:
		default:
			return "", nil, errors.New("social: create direct chat workspace failed")
		}
	}
	if err := s.grantWorkspace(ctx, workspaceName, from, to); err != nil {
		if created {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
		return "", nil, err
	}
	rollback := func() {
		_ = s.revokeWorkspace(ctx, workspaceName, from, to)
		if created {
			_ = s.deleteWorkspace(ctx, workspaceName)
		}
	}
	return workspaceName, rollback, nil
}

func (s *Server) deleteDirectChatWorkspace(ctx context.Context, owner string, item rpcapi.FriendObject) error {
	other := socialutil.StringValue(item.PeerPublicKey)
	workspaceName := socialutil.StringValue(item.WorkspaceName)
	if workspaceName == "" {
		workspaceName = socialutil.DirectWorkspaceName(socialutil.RelationID(owner, other))
	}
	if err := s.revokeWorkspace(ctx, workspaceName, owner, other); err != nil {
		return err
	}
	return s.deleteWorkspace(ctx, workspaceName)
}

func (s *Server) grantWorkspace(ctx context.Context, workspaceName string, peers ...string) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	roleName, permissions := socialutil.WorkspaceACLRole()
	if _, err := s.ACL.PutRole(ctx, roleName, permissions); err != nil {
		return err
	}
	for _, peerID := range peers {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" {
			continue
		}
		if _, err := s.ACL.PutPolicyBinding(ctx, socialutil.WorkspaceACLBindingID(workspaceName, peerID), 0, apitypes.ACLPolicy{
			Subject:  acl.PublicKeySubject(peerID),
			Resource: acl.WorkspaceResource(workspaceName),
			Role:     roleName,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) revokeWorkspace(ctx context.Context, workspaceName string, peers ...string) error {
	if s == nil || s.ACL == nil {
		return nil
	}
	for _, peerID := range peers {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" {
			continue
		}
		if _, err := s.ACL.DeletePolicyBinding(ctx, socialutil.WorkspaceACLBindingID(workspaceName, peerID)); err != nil && !errors.Is(err, acl.ErrPolicyBindingNotFound) {
			return err
		}
	}
	return nil
}

func (s *Server) deleteWorkspace(ctx context.Context, workspaceName string) error {
	if s == nil || s.Workspaces == nil {
		return nil
	}
	resp, err := s.Workspaces.DeleteWorkspace(ctx, adminservice.DeleteWorkspaceRequestObject{Name: workspaceName})
	if err != nil {
		return err
	}
	switch resp.(type) {
	case adminservice.DeleteWorkspace200JSONResponse, adminservice.DeleteWorkspace404JSONResponse:
		return nil
	default:
		return errors.New("social: delete direct chat workspace failed")
	}
}

func (s *Server) activeInviteToken(ctx context.Context, store kv.Store, owner string) (inviteTokenRecord, bool, error) {
	if owner == "" {
		return inviteTokenRecord{}, false, errors.New("social: peer public key is required")
	}
	record, err := socialutil.ReadJSONValue[inviteTokenRecord](ctx, store, socialutil.FriendInviteTokenKey(owner))
	if err != nil {
		if errors.Is(err, kv.ErrNotFound) {
			return inviteTokenRecord{}, false, nil
		}
		return inviteTokenRecord{}, false, err
	}
	if strings.TrimSpace(record.InviteToken) == "" || !record.ExpiresAt.After(s.now()) {
		_ = store.Delete(ctx, socialutil.FriendInviteTokenKey(owner))
		return inviteTokenRecord{}, false, nil
	}
	return record, true, nil
}

func (s *Server) findInviteToken(ctx context.Context, inviteToken string) (inviteTokenRecord, error) {
	inviteToken = strings.TrimSpace(inviteToken)
	if inviteToken == "" {
		return inviteTokenRecord{}, errors.New("social: invite token is required")
	}
	store, err := s.inviteTokensStore()
	if err != nil {
		return inviteTokenRecord{}, err
	}
	now := s.now()
	for entry, err := range store.List(ctx, socialutil.FriendInviteTokensRoot) {
		if err != nil {
			return inviteTokenRecord{}, err
		}
		var record inviteTokenRecord
		if err := json.Unmarshal(entry.Value, &record); err != nil {
			return inviteTokenRecord{}, err
		}
		if strings.TrimSpace(record.InviteToken) == "" || !record.ExpiresAt.After(now) {
			_ = store.Delete(ctx, entry.Key)
			continue
		}
		if record.InviteToken == inviteToken {
			return record, nil
		}
	}
	return inviteTokenRecord{}, errors.New("social: invite token not found")
}

func (s *Server) inviteTokensStore() (kv.Store, error) {
	if s == nil || s.InviteTokens == nil {
		return nil, errors.New("social: friend invite token service not configured")
	}
	return s.InviteTokens, nil
}

func (s *Server) friendsStore() (kv.Store, error) {
	if s == nil || s.Friends == nil {
		return nil, errors.New("social: friend service not configured")
	}
	return s.Friends, nil
}

func (s *Server) inviteTokenTTL() time.Duration {
	return socialutil.DefaultInviteTokenTTL
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
