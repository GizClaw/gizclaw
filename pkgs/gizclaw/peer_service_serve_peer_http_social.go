package gizclaw

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// Stable error codes of the /gizclaw/v1/friends* and /gizclaw/v1/friend-groups*
// routes; api/http/peer.json lists which route returns which code.
const (
	publicHTTPInviteTokenNotFound             = "INVITE_TOKEN_NOT_FOUND"
	publicHTTPInviteTokenInvalid              = "INVITE_TOKEN_INVALID"
	publicHTTPFriendNotFound                  = "FRIEND_NOT_FOUND"
	publicHTTPFriendExists                    = "FRIEND_ALREADY_EXISTS"
	publicHTTPFriendSelfInvite                = "FRIEND_SELF_INVITE"
	publicHTTPFriendLimitReached              = "FRIEND_LIMIT_REACHED"
	publicHTTPFriendGroupNotFound             = "FRIEND_GROUP_NOT_FOUND"
	publicHTTPFriendGroupMemberNotFound       = "FRIEND_GROUP_MEMBER_NOT_FOUND"
	publicHTTPFriendGroupPermissionDenied     = "FRIEND_GROUP_PERMISSION_DENIED"
	publicHTTPFriendGroupNameConflict         = "FRIEND_GROUP_NAME_CONFLICT"
	publicHTTPFriendGroupAlreadyJoined        = "FRIEND_GROUP_ALREADY_JOINED"
	publicHTTPFriendGroupFull                 = "FRIEND_GROUP_FULL"
	publicHTTPFriendGroupLimitReached         = "FRIEND_GROUP_LIMIT_REACHED"
	publicHTTPFriendGroupOwnerCannotLeave     = "FRIEND_GROUP_OWNER_CANNOT_LEAVE"
	publicHTTPFriendGroupOwnerCannotBeRemoved = "FRIEND_GROUP_OWNER_CANNOT_BE_REMOVED"
	publicHTTPFriendGroupOwnerRoleImmutable   = "FRIEND_GROUP_OWNER_ROLE_IMMUTABLE"
	publicHTTPFriendGroupChanged              = "FRIEND_GROUP_CHANGED"
	publicHTTPFriendGroupPendingDeletion      = "FRIEND_GROUP_PENDING_DELETION"
	publicHTTPPeerDeleted                     = "PEER_DELETED"
)

// publicSocialError maps friend and friend group service failures onto the
// Peer HTTP contract. notFound names what a bare kv.ErrNotFound means for the
// route; store and configuration failures are redacted as 500.
func publicSocialError(err error, notFound string) (int, apitypes.ErrorResponse) {
	switch {
	case errors.Is(err, friend.ErrInviteTokenLookupFailed):
		return http.StatusInternalServerError, internalPublicHTTP()
	case errors.Is(err, friend.ErrInviteTokenRequired),
		errors.Is(err, friendgroup.ErrInvalidMemberRole),
		errors.Is(err, socialutil.ErrInvalidInviteTokenTTL):
		return http.StatusBadRequest, apiError(publicHTTPInvalidRequestCode, strings.TrimPrefix(err.Error(), "social: "))
	case errors.Is(err, friend.ErrInviteTokenSelfOwned):
		return http.StatusBadRequest, apiError(publicHTTPFriendSelfInvite, "the invite token belongs to the bound device")
	case errors.Is(err, friend.ErrInviteTokenUnavailable), errors.Is(err, friendgroup.ErrInviteTokenUnavailable):
		return http.StatusNotFound, apiError(publicHTTPInviteTokenInvalid, "invite token does not exist or has expired")
	case errors.Is(err, friendgroup.ErrFriendGroupPermissionDenied):
		return http.StatusForbidden, apiError(publicHTTPFriendGroupPermissionDenied, strings.TrimPrefix(err.Error(), "social: "))
	case errors.Is(err, friendgroup.ErrFriendGroupMemberNotFound):
		return http.StatusNotFound, apiError(publicHTTPFriendGroupMemberNotFound, "friend group member not found")
	case errors.Is(err, friend.ErrPeerFriendLimit):
		return http.StatusConflict, apiError(publicHTTPFriendLimitReached, "peer friend limit reached")
	case errors.Is(err, friendgroup.ErrFriendGroupNameExists):
		return http.StatusConflict, apiError(publicHTTPFriendGroupNameConflict, strings.TrimPrefix(err.Error(), "social: "))
	case errors.Is(err, friendgroup.ErrFriendGroupMembershipNameImmutable):
		return http.StatusConflict, apiError(publicHTTPFriendGroupAlreadyJoined, "already a member of this friend group under a different name")
	case errors.Is(err, friendgroup.ErrFriendGroupFull):
		return http.StatusConflict, apiError(publicHTTPFriendGroupFull, "friend group is full")
	case errors.Is(err, friendgroup.ErrPeerFriendGroupLimit):
		return http.StatusConflict, apiError(publicHTTPFriendGroupLimitReached, "peer friend group limit reached")
	case errors.Is(err, friendgroup.ErrFriendGroupOwnerCannotLeave):
		return http.StatusConflict, apiError(publicHTTPFriendGroupOwnerCannotLeave, "the owner cannot leave the friend group; delete it instead")
	case errors.Is(err, friendgroup.ErrFriendGroupOwnerCannotBeRemoved):
		return http.StatusConflict, apiError(publicHTTPFriendGroupOwnerCannotBeRemoved, "the friend group owner cannot be removed")
	case errors.Is(err, friendgroup.ErrFriendGroupOwnerRoleImmutable):
		return http.StatusConflict, apiError(publicHTTPFriendGroupOwnerRoleImmutable, "the friend group owner role cannot change")
	case errors.Is(err, friendgroup.ErrGroupChanged):
		return http.StatusConflict, apiError(publicHTTPFriendGroupChanged, friendgroup.ErrGroupChanged.Error())
	case errors.Is(err, friendgroup.ErrFriendGroupPendingDeletion):
		return http.StatusConflict, apiError(publicHTTPFriendGroupPendingDeletion, "friend group is pending deletion")
	case errors.Is(err, runtimepeer.ErrPeerPendingDeletion):
		return http.StatusConflict, apiError(runtimepeer.PeerPendingDeletionCode, "peer is pending deletion")
	case errors.Is(err, runtimepeer.ErrPeerDeleted), errors.Is(err, runtimepeer.ErrPeerNotFound):
		return http.StatusConflict, apiError(publicHTTPPeerDeleted, "peer is deleted")
	case errors.Is(err, kv.ErrNotFound):
		return http.StatusNotFound, apiError(notFound, strings.ToLower(strings.ReplaceAll(notFound, "_", " ")))
	default:
		return http.StatusInternalServerError, internalPublicHTTP()
	}
}

// publicSocialName rejects an empty name or one with surrounding whitespace,
// which the social services would otherwise trim into a different name.
func publicSocialName(field, value string) (apitypes.ErrorResponse, bool) {
	if value == "" || value != strings.TrimSpace(value) {
		return apiError(publicHTTPInvalidRequestCode, field+" is required without surrounding whitespace"), false
	}
	return apitypes.ErrorResponse{}, true
}

// publicSocialPage validates cursor and limit like /gizclaw/v1/contacts.
func publicSocialPage(cursor *string, limit *int32) (*string, *int, apitypes.ErrorResponse, bool) {
	var outCursor *string
	if cursor != nil {
		value := strings.TrimSpace(*cursor)
		if value == "" {
			return nil, nil, apiError(publicHTTPInvalidRequestCode, "cursor must not be empty"), false
		}
		outCursor = &value
	}
	var outLimit *int
	if limit != nil {
		value := int(*limit)
		if value < 1 || value > maxPublicContactPageSize {
			return nil, nil, apiError(publicHTTPInvalidRequestCode, "limit must be between 1 and 200"), false
		}
		outLimit = &value
	}
	return outCursor, outLimit, apitypes.ErrorResponse{}, true
}

// publicInviteTokenTTL converts the optional ttl_seconds body field; zero
// keeps the RPC default lifetime.
func publicInviteTokenTTL(body *peerhttp.InviteTokenCreateRequest) (time.Duration, apitypes.ErrorResponse, bool) {
	if body == nil || body.TtlSeconds == nil {
		return 0, apitypes.ErrorResponse{}, true
	}
	ttl := time.Duration(*body.TtlSeconds) * time.Second
	if err := socialutil.ValidateInviteTokenTTL(ttl); err != nil {
		return 0, apiError(publicHTTPInvalidRequestCode, "ttl_seconds must be between 60 and 604800"), false
	}
	return ttl, apitypes.ErrorResponse{}, true
}

// peerProfileInfo projects another Peer's public profile the same way as
// server.friend.info.get. A Peer that no longer exists has no info.
func (s *peerHTTP) peerProfileInfo(ctx context.Context, publicKeyText string) (*peerhttp.PeerProfileInfo, error) {
	if s.Profiles == nil {
		return nil, errors.New("gizclaw: peer profile service is not configured")
	}
	var publicKey giznet.PublicKey
	if err := publicKey.UnmarshalText([]byte(publicKeyText)); err != nil {
		return nil, err
	}
	info, err := s.Profiles.GetSelfInfo(ctx, publicKey)
	if errors.Is(err, runtimepeer.ErrPeerNotFound) || errors.Is(err, runtimepeer.ErrPeerDeleted) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &peerhttp.PeerProfileInfo{DisplayName: info.Name, Emoji: info.Emoji}, nil
}

func (s *peerHTTP) publicFriend(ctx context.Context, item rpcapi.FriendObject) (peerhttp.Friend, error) {
	out := peerhttp.Friend{
		Name:          item.Name,
		PeerPublicKey: socialutil.StringValue(item.PeerPublicKey),
		WorkspaceName: socialutil.StringValue(item.WorkspaceName),
	}
	if item.CreatedAt != nil {
		out.CreatedAt = *item.CreatedAt
	}
	if item.UpdatedAt != nil {
		out.UpdatedAt = *item.UpdatedAt
	}
	info, err := s.peerProfileInfo(ctx, out.PeerPublicKey)
	if err != nil {
		return peerhttp.Friend{}, err
	}
	out.Info = info
	return out, nil
}

func publicFriendGroup(item rpcapi.FriendGroupObject) peerhttp.FriendGroup {
	out := peerhttp.FriendGroup{
		Name:                   item.Name,
		DisplayName:            item.DisplayName,
		Description:            item.Description,
		CreatedByPeerPublicKey: item.CreatedByPeerPublicKey,
		WorkspaceName:          item.WorkspaceName,
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
	if item.MyRole != nil {
		out.MyRole = peerhttp.FriendGroupRole(*item.MyRole)
	}
	return out
}

func (s *peerHTTP) publicFriendGroupMember(ctx context.Context, item rpcapi.FriendGroupMemberObject) (peerhttp.FriendGroupMember, error) {
	out := peerhttp.FriendGroupMember{
		Name:          item.Name,
		PeerPublicKey: socialutil.StringValue(item.PeerPublicKey),
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
	if item.Role != nil {
		out.Role = peerhttp.FriendGroupRole(*item.Role)
	}
	info, err := s.peerProfileInfo(ctx, out.PeerPublicKey)
	if err != nil {
		return peerhttp.FriendGroupMember{}, err
	}
	out.Info = info
	return out, nil
}

func publicInviteToken(token string, expiresAt time.Time) peerhttp.InviteToken {
	return peerhttp.InviteToken{InviteToken: token, ExpiresAt: expiresAt}
}

func (s *peerHTTP) GetFriendInviteToken(ctx context.Context, _ peerhttp.GetFriendInviteTokenRequestObject) (peerhttp.GetFriendInviteTokenResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetFriendInviteToken401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.GetFriendInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	result, err := s.Friends.GetFriendInviteToken(ctx, owner.String(), rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		return peerhttp.GetFriendInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if result.InviteToken == nil || result.ExpiresAt == nil {
		return peerhttp.GetFriendInviteToken404JSONResponse(apiError(publicHTTPInviteTokenNotFound, "no active friend invite token")), nil
	}
	return peerhttp.GetFriendInviteToken200JSONResponse(publicInviteToken(*result.InviteToken, *result.ExpiresAt)), nil
}

func (s *peerHTTP) CreateFriendInviteToken(ctx context.Context, request peerhttp.CreateFriendInviteTokenRequestObject) (peerhttp.CreateFriendInviteTokenResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.CreateFriendInviteToken401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.CreateFriendInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	ttl, body, ok := publicInviteTokenTTL(request.Body)
	if !ok {
		return peerhttp.CreateFriendInviteToken400JSONResponse(body), nil
	}
	result, err := s.Friends.CreateFriendInviteTokenWithTTL(ctx, owner.String(), ttl)
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPInviteTokenNotFound); status {
		case http.StatusBadRequest:
			return peerhttp.CreateFriendInviteToken400JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.CreateFriendInviteToken409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
		default:
			return peerhttp.CreateFriendInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.CreateFriendInviteToken200JSONResponse(publicInviteToken(result.InviteToken, result.ExpiresAt)), nil
}

func (s *peerHTTP) ClearFriendInviteToken(ctx context.Context, _ peerhttp.ClearFriendInviteTokenRequestObject) (peerhttp.ClearFriendInviteTokenResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ClearFriendInviteToken401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.ClearFriendInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if _, err := s.Friends.ClearFriendInviteToken(ctx, owner.String(), rpcapi.FriendInviteTokenClearRequest{}); err != nil {
		return peerhttp.ClearFriendInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.ClearFriendInviteToken204Response{}, nil
}

func (s *peerHTTP) AddFriend(ctx context.Context, request peerhttp.AddFriendRequestObject) (peerhttp.AddFriendResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.AddFriend401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.AddFriend500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil || strings.TrimSpace(request.Body.InviteToken) == "" {
		return peerhttp.AddFriend400JSONResponse(apiError(publicHTTPInvalidRequestCode, "invite_token is required")), nil
	}
	item, existed, err := s.Friends.AddFriendReportingExisting(ctx, owner.String(), rpcapi.FriendAddRequest{InviteToken: strings.TrimSpace(request.Body.InviteToken)})
	if err == nil && existed {
		return peerhttp.AddFriend409JSONResponse(apiError(publicHTTPFriendExists, "the devices are already friends")), nil
	}
	var out peerhttp.Friend
	if err == nil {
		out, err = s.publicFriend(ctx, item)
	}
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPInviteTokenInvalid); status {
		case http.StatusBadRequest:
			return peerhttp.AddFriend400JSONResponse(body), nil
		case http.StatusNotFound:
			return peerhttp.AddFriend404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.AddFriend409JSONResponse(body), nil
		default:
			return peerhttp.AddFriend500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.AddFriend201JSONResponse(out), nil
}

func (s *peerHTTP) ListFriends(ctx context.Context, request peerhttp.ListFriendsRequestObject) (peerhttp.ListFriendsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListFriends401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.ListFriends500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	cursor, limit, body, ok := publicSocialPage(request.Params.Cursor, request.Params.Limit)
	if !ok {
		return peerhttp.ListFriends400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	result, err := s.Friends.ListFriends(ctx, owner.String(), rpcapi.FriendListRequest{Cursor: cursor, Limit: limit})
	if err != nil {
		return peerhttp.ListFriends500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	items := make([]peerhttp.Friend, len(result.Items))
	for i := range result.Items {
		if items[i], err = s.publicFriend(ctx, result.Items[i]); err != nil {
			return peerhttp.ListFriends500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.ListFriends200JSONResponse{Items: items, HasNext: result.HasNext, NextCursor: result.NextCursor}, nil
}

func (s *peerHTTP) GetFriend(ctx context.Context, request peerhttp.GetFriendRequestObject) (peerhttp.GetFriendResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetFriend401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.GetFriend500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendName", request.FriendName); !ok {
		return peerhttp.GetFriend400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	item, err := s.Friends.GetFriendRelation(ctx, owner.String(), request.FriendName)
	var out peerhttp.Friend
	if err == nil {
		out, err = s.publicFriend(ctx, item)
	}
	if err != nil {
		if status, body := publicSocialError(err, publicHTTPFriendNotFound); status == http.StatusNotFound {
			return peerhttp.GetFriend404JSONResponse(body), nil
		}
		return peerhttp.GetFriend500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	return peerhttp.GetFriend200JSONResponse(out), nil
}

func (s *peerHTTP) DeleteFriend(ctx context.Context, request peerhttp.DeleteFriendRequestObject) (peerhttp.DeleteFriendResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.DeleteFriend401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.Friends == nil {
		return peerhttp.DeleteFriend500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendName", request.FriendName); !ok {
		return peerhttp.DeleteFriend400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	if _, err := s.Friends.DeleteFriend(ctx, owner.String(), rpcapi.FriendDeleteRequest{Name: request.FriendName}); err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendNotFound); status {
		case http.StatusNotFound:
			return peerhttp.DeleteFriend404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.DeleteFriend409JSONResponse(body), nil
		default:
			return peerhttp.DeleteFriend500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.DeleteFriend204Response{}, nil
}

func (s *peerHTTP) ListFriendGroups(ctx context.Context, request peerhttp.ListFriendGroupsRequestObject) (peerhttp.ListFriendGroupsResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListFriendGroups401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.ListFriendGroups500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	cursor, limit, body, ok := publicSocialPage(request.Params.Cursor, request.Params.Limit)
	if !ok {
		return peerhttp.ListFriendGroups400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	result, err := s.FriendGroups.ListFriendGroups(ctx, owner.String(), rpcapi.FriendGroupListRequest{Cursor: cursor, Limit: limit})
	if err != nil {
		return peerhttp.ListFriendGroups500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	items := make([]peerhttp.FriendGroup, len(result.Items))
	for i := range result.Items {
		items[i] = publicFriendGroup(result.Items[i])
	}
	return peerhttp.ListFriendGroups200JSONResponse{Items: items, HasNext: result.HasNext, NextCursor: result.NextCursor}, nil
}

func (s *peerHTTP) CreateFriendGroup(ctx context.Context, request peerhttp.CreateFriendGroupRequestObject) (peerhttp.CreateFriendGroupResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.CreateFriendGroup401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.CreateFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.CreateFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	if body, ok := publicSocialName("name", request.Body.Name); !ok {
		return peerhttp.CreateFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	item, err := s.FriendGroups.CreateFriendGroup(ctx, owner.String(), rpcapi.FriendGroupCreateRequest{
		Name: request.Body.Name, DisplayName: request.Body.DisplayName, Description: request.Body.Description,
	})
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusBadRequest:
			return peerhttp.CreateFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.CreateFriendGroup409JSONResponse(body), nil
		default:
			return peerhttp.CreateFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.CreateFriendGroup201JSONResponse(publicFriendGroup(item)), nil
}

func (s *peerHTTP) JoinFriendGroup(ctx context.Context, request peerhttp.JoinFriendGroupRequestObject) (peerhttp.JoinFriendGroupResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.JoinFriendGroup401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.JoinFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil || strings.TrimSpace(request.Body.InviteToken) == "" {
		return peerhttp.JoinFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "invite_token is required"))}, nil
	}
	if body, ok := publicSocialName("name", request.Body.Name); !ok {
		return peerhttp.JoinFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	result, err := s.FriendGroups.JoinFriendGroup(ctx, owner.String(), rpcapi.FriendGroupJoinRequest{
		InviteToken: strings.TrimSpace(request.Body.InviteToken), Name: request.Body.Name,
	})
	var member peerhttp.FriendGroupMember
	if err == nil {
		member, err = s.publicFriendGroupMember(ctx, result.Member)
	}
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPInviteTokenInvalid); status {
		case http.StatusBadRequest:
			return peerhttp.JoinFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.JoinFriendGroup404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.JoinFriendGroup409JSONResponse(body), nil
		default:
			return peerhttp.JoinFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.JoinFriendGroup200JSONResponse{Group: publicFriendGroup(result.Group), Member: member}, nil
}

func (s *peerHTTP) GetFriendGroup(ctx context.Context, request peerhttp.GetFriendGroupRequestObject) (peerhttp.GetFriendGroupResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetFriendGroup401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.GetFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.GetFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	item, err := s.FriendGroups.GetFriendGroup(ctx, owner.String(), rpcapi.FriendGroupGetRequest{Name: request.FriendGroupName})
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusNotFound:
			return peerhttp.GetFriendGroup404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.GetFriendGroup409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
		default:
			return peerhttp.GetFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.GetFriendGroup200JSONResponse(publicFriendGroup(item)), nil
}

func (s *peerHTTP) PutFriendGroup(ctx context.Context, request peerhttp.PutFriendGroupRequestObject) (peerhttp.PutFriendGroupResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.PutFriendGroup401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.PutFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.PutFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.PutFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	item, err := s.FriendGroups.PutFriendGroup(ctx, owner.String(), rpcapi.FriendGroupPutRequest{
		Name: request.FriendGroupName, DisplayName: request.Body.DisplayName, Description: request.Body.Description,
	})
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusForbidden:
			return peerhttp.PutFriendGroup403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.PutFriendGroup404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.PutFriendGroup409JSONResponse{FriendGroupConflictJSONResponse: peerhttp.FriendGroupConflictJSONResponse(body)}, nil
		default:
			return peerhttp.PutFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.PutFriendGroup200JSONResponse(publicFriendGroup(item)), nil
}

func (s *peerHTTP) DeleteFriendGroup(ctx context.Context, request peerhttp.DeleteFriendGroupRequestObject) (peerhttp.DeleteFriendGroupResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.DeleteFriendGroup401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.DeleteFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.DeleteFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	if _, err := s.FriendGroups.DeleteFriendGroup(ctx, owner.String(), rpcapi.FriendGroupDeleteRequest{Name: request.FriendGroupName}); err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusForbidden:
			return peerhttp.DeleteFriendGroup403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.DeleteFriendGroup404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.DeleteFriendGroup409JSONResponse{FriendGroupConflictJSONResponse: peerhttp.FriendGroupConflictJSONResponse(body)}, nil
		default:
			return peerhttp.DeleteFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.DeleteFriendGroup204Response{}, nil
}

func (s *peerHTTP) GetFriendGroupInviteToken(ctx context.Context, request peerhttp.GetFriendGroupInviteTokenRequestObject) (peerhttp.GetFriendGroupInviteTokenResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetFriendGroupInviteToken401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.GetFriendGroupInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.GetFriendGroupInviteToken400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	result, err := s.FriendGroups.GetFriendGroupInviteToken(ctx, owner.String(), rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: request.FriendGroupName})
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusForbidden:
			return peerhttp.GetFriendGroupInviteToken403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.GetFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusConflict:
			return peerhttp.GetFriendGroupInviteToken409JSONResponse{FriendGroupConflictJSONResponse: peerhttp.FriendGroupConflictJSONResponse(body)}, nil
		default:
			return peerhttp.GetFriendGroupInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	if result.InviteToken == nil || result.ExpiresAt == nil {
		return peerhttp.GetFriendGroupInviteToken404JSONResponse(apiError(publicHTTPInviteTokenNotFound, "no active friend group invite token")), nil
	}
	return peerhttp.GetFriendGroupInviteToken200JSONResponse(publicInviteToken(*result.InviteToken, *result.ExpiresAt)), nil
}

func (s *peerHTTP) CreateFriendGroupInviteToken(ctx context.Context, request peerhttp.CreateFriendGroupInviteTokenRequestObject) (peerhttp.CreateFriendGroupInviteTokenResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.CreateFriendGroupInviteToken401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.CreateFriendGroupInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.CreateFriendGroupInviteToken400JSONResponse(body), nil
	}
	ttl, body, ok := publicInviteTokenTTL(request.Body)
	if !ok {
		return peerhttp.CreateFriendGroupInviteToken400JSONResponse(body), nil
	}
	result, err := s.FriendGroups.CreateFriendGroupInviteTokenWithTTL(ctx, owner.String(), rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: request.FriendGroupName}, ttl)
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusBadRequest:
			return peerhttp.CreateFriendGroupInviteToken400JSONResponse(body), nil
		case http.StatusForbidden:
			return peerhttp.CreateFriendGroupInviteToken403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.CreateFriendGroupInviteToken404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.CreateFriendGroupInviteToken409JSONResponse{FriendGroupConflictJSONResponse: peerhttp.FriendGroupConflictJSONResponse(body)}, nil
		default:
			return peerhttp.CreateFriendGroupInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.CreateFriendGroupInviteToken200JSONResponse(publicInviteToken(result.InviteToken, result.ExpiresAt)), nil
}

func (s *peerHTTP) ClearFriendGroupInviteToken(ctx context.Context, request peerhttp.ClearFriendGroupInviteTokenRequestObject) (peerhttp.ClearFriendGroupInviteTokenResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ClearFriendGroupInviteToken401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.ClearFriendGroupInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.ClearFriendGroupInviteToken400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	if _, err := s.FriendGroups.ClearFriendGroupInviteToken(ctx, owner.String(), rpcapi.FriendGroupInviteTokenClearRequest{FriendGroupName: request.FriendGroupName}); err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusForbidden:
			return peerhttp.ClearFriendGroupInviteToken403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.ClearFriendGroupInviteToken404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.ClearFriendGroupInviteToken409JSONResponse{FriendGroupConflictJSONResponse: peerhttp.FriendGroupConflictJSONResponse(body)}, nil
		default:
			return peerhttp.ClearFriendGroupInviteToken500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.ClearFriendGroupInviteToken204Response{}, nil
}

func (s *peerHTTP) LeaveFriendGroup(ctx context.Context, request peerhttp.LeaveFriendGroupRequestObject) (peerhttp.LeaveFriendGroupResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.LeaveFriendGroup401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.LeaveFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.LeaveFriendGroup400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	if _, err := s.FriendGroups.LeaveFriendGroup(ctx, owner.String(), request.FriendGroupName); err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusNotFound:
			// The caller's own membership is the target, so a missing one
			// means the caller has no such Group.
			body = apiError(publicHTTPFriendGroupNotFound, "friend group not found")
			return peerhttp.LeaveFriendGroup404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.LeaveFriendGroup409JSONResponse(body), nil
		default:
			return peerhttp.LeaveFriendGroup500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.LeaveFriendGroup204Response{}, nil
}

func (s *peerHTTP) ListFriendGroupMembers(ctx context.Context, request peerhttp.ListFriendGroupMembersRequestObject) (peerhttp.ListFriendGroupMembersResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ListFriendGroupMembers401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.ListFriendGroupMembers500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if body, ok := publicSocialName("friendGroupName", request.FriendGroupName); !ok {
		return peerhttp.ListFriendGroupMembers400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	cursor, limit, body, ok := publicSocialPage(request.Params.Cursor, request.Params.Limit)
	if !ok {
		return peerhttp.ListFriendGroupMembers400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
	}
	result, err := s.FriendGroups.ListFriendGroupMembers(ctx, owner.String(), rpcapi.FriendGroupMemberListRequest{
		FriendGroupName: &request.FriendGroupName, Cursor: cursor, Limit: limit,
	})
	items := make([]peerhttp.FriendGroupMember, len(result.Items))
	for i := 0; err == nil && i < len(result.Items); i++ {
		items[i], err = s.publicFriendGroupMember(ctx, result.Items[i])
	}
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusNotFound:
			return peerhttp.ListFriendGroupMembers404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.ListFriendGroupMembers409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
		default:
			return peerhttp.ListFriendGroupMembers500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.ListFriendGroupMembers200JSONResponse{Items: items, HasNext: result.HasNext, NextCursor: result.NextCursor}, nil
}

func (s *peerHTTP) AddFriendGroupMember(ctx context.Context, request peerhttp.AddFriendGroupMemberRequestObject) (peerhttp.AddFriendGroupMemberResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.AddFriendGroupMember401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.AddFriendGroupMember500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.AddFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	for _, field := range []struct{ name, value string }{
		{"friendGroupName", request.FriendGroupName},
		{"peer_public_key", request.Body.PeerPublicKey},
		{"member_name", request.Body.MemberName},
	} {
		if body, ok := publicSocialName(field.name, field.value); !ok {
			return peerhttp.AddFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
	}
	if !request.Body.Role.Valid() {
		return peerhttp.AddFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "role must be admin or member"))}, nil
	}
	item, err := s.FriendGroups.AddFriendGroupMember(ctx, owner.String(), rpcapi.FriendGroupMemberAddRequest{
		FriendGroupName: request.FriendGroupName,
		PeerPublicKey:   request.Body.PeerPublicKey,
		MemberName:      request.Body.MemberName,
		Role:            rpcapi.FriendGroupMemberMutableRole(request.Body.Role),
	})
	var out peerhttp.FriendGroupMember
	if err == nil {
		out, err = s.publicFriendGroupMember(ctx, item)
	}
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusBadRequest:
			return peerhttp.AddFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		case http.StatusForbidden:
			return peerhttp.AddFriendGroupMember403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.AddFriendGroupMember404JSONResponse{FriendGroupNotFoundJSONResponse: peerhttp.FriendGroupNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.AddFriendGroupMember409JSONResponse(body), nil
		default:
			return peerhttp.AddFriendGroupMember500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.AddFriendGroupMember201JSONResponse(out), nil
}

func (s *peerHTTP) PutFriendGroupMember(ctx context.Context, request peerhttp.PutFriendGroupMemberRequestObject) (peerhttp.PutFriendGroupMemberResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.PutFriendGroupMember401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.PutFriendGroupMember500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	if request.Body == nil {
		return peerhttp.PutFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "request body is required"))}, nil
	}
	for _, field := range []struct{ name, value string }{
		{"friendGroupName", request.FriendGroupName},
		{"memberName", request.MemberName},
	} {
		if body, ok := publicSocialName(field.name, field.value); !ok {
			return peerhttp.PutFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
	}
	if !request.Body.Role.Valid() {
		return peerhttp.PutFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "role must be admin or member"))}, nil
	}
	item, err := s.FriendGroups.PutFriendGroupMember(ctx, owner.String(), rpcapi.FriendGroupMemberPutRequest{
		FriendGroupName: request.FriendGroupName,
		Name:            request.MemberName,
		Role:            rpcapi.FriendGroupMemberMutableRole(request.Body.Role),
	})
	var out peerhttp.FriendGroupMember
	if err == nil {
		out, err = s.publicFriendGroupMember(ctx, item)
	}
	if err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusBadRequest:
			return peerhttp.PutFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		case http.StatusForbidden:
			return peerhttp.PutFriendGroupMember403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.PutFriendGroupMember404JSONResponse{FriendGroupMemberNotFoundJSONResponse: peerhttp.FriendGroupMemberNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.PutFriendGroupMember409JSONResponse(body), nil
		default:
			return peerhttp.PutFriendGroupMember500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.PutFriendGroupMember200JSONResponse(out), nil
}

func (s *peerHTTP) DeleteFriendGroupMember(ctx context.Context, request peerhttp.DeleteFriendGroupMemberRequestObject) (peerhttp.DeleteFriendGroupMemberResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.DeleteFriendGroupMember401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if s == nil || s.FriendGroups == nil {
		return peerhttp.DeleteFriendGroupMember500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
	}
	for _, field := range []struct{ name, value string }{
		{"friendGroupName", request.FriendGroupName},
		{"memberName", request.MemberName},
	} {
		if body, ok := publicSocialName(field.name, field.value); !ok {
			return peerhttp.DeleteFriendGroupMember400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}, nil
		}
	}
	if _, err := s.FriendGroups.DeleteFriendGroupMember(ctx, owner.String(), rpcapi.FriendGroupMemberDeleteRequest{
		FriendGroupName: request.FriendGroupName, Name: request.MemberName,
	}); err != nil {
		switch status, body := publicSocialError(err, publicHTTPFriendGroupNotFound); status {
		case http.StatusForbidden:
			return peerhttp.DeleteFriendGroupMember403JSONResponse{FriendGroupForbiddenJSONResponse: peerhttp.FriendGroupForbiddenJSONResponse(body)}, nil
		case http.StatusNotFound:
			return peerhttp.DeleteFriendGroupMember404JSONResponse{FriendGroupMemberNotFoundJSONResponse: peerhttp.FriendGroupMemberNotFoundJSONResponse(body)}, nil
		case http.StatusConflict:
			return peerhttp.DeleteFriendGroupMember409JSONResponse(body), nil
		default:
			return peerhttp.DeleteFriendGroupMember500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}, nil
		}
	}
	return peerhttp.DeleteFriendGroupMember204Response{}, nil
}
