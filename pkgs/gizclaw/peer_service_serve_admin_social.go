package gizclaw

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

type adminWorkspaceHistoryService interface {
	AdminListWorkspaceHistory(context.Context, string, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error)
	AdminGetWorkspaceHistory(context.Context, string, string) (workspace.HistoryEntry, error)
	AdminReadWorkspaceHistoryAudio(context.Context, string, string) (io.ReadCloser, int64, error)
}

func (s *adminService) ListContacts(ctx context.Context, request adminhttp.ListContactsRequestObject) (adminhttp.ListContactsResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminhttp.ListContacts500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	resp, err := s.Contacts.AdminListContacts(ctx, socialutil.StringValue(request.Params.OwnerPublicKey), request.Params.Cursor, request.Params.Limit)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusInternalServerError:
			return adminhttp.ListContacts500JSONResponse(body), nil
		default:
			return adminhttp.ListContacts400JSONResponse(body), nil
		}
	}
	return adminhttp.ListContacts200JSONResponse(resp), nil
}

func (s *adminService) CreateContact(ctx context.Context, request adminhttp.CreateContactRequestObject) (adminhttp.CreateContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminhttp.CreateContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateContact400JSONResponse(apitypes.NewErrorResponse("INVALID_CONTACT", "request body is required")), nil
	}
	item, err := s.Contacts.AdminCreateContact(ctx, *request.Body)
	if err != nil {
		if errors.Is(err, socialutil.ErrResourceAlreadyExists) {
			return adminhttp.CreateContact409JSONResponse(apitypes.NewErrorResponse("CONTACT_ALREADY_EXISTS", err.Error())), nil
		}
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.CreateContact409JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.CreateContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.CreateContact500JSONResponse(body), nil
		default:
			return adminhttp.CreateContact400JSONResponse(body), nil
		}
	}
	return adminhttp.CreateContact200JSONResponse(item), nil
}

func (s *adminService) GetContact(ctx context.Context, request adminhttp.GetContactRequestObject) (adminhttp.GetContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminhttp.GetContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	owner := request.OwnerPublicKey
	id := request.Id
	item, err := s.Contacts.AdminGetContact(ctx, owner, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.GetContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.GetContact500JSONResponse(body), nil
		default:
			return adminhttp.GetContact400JSONResponse(body), nil
		}
	}
	return adminhttp.GetContact200JSONResponse(item), nil
}

func (s *adminService) PutContact(ctx context.Context, request adminhttp.PutContactRequestObject) (adminhttp.PutContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminhttp.PutContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutContact400JSONResponse(apitypes.NewErrorResponse("INVALID_CONTACT", "request body is required")), nil
	}
	owner := request.OwnerPublicKey
	id := request.Id
	if request.Body.Id != id {
		return adminhttp.PutContact400JSONResponse(apitypes.NewErrorResponse("RESOURCE_ID_MISMATCH", "body id must match path id")), nil
	}
	item, err := s.Contacts.AdminPutContact(ctx, owner, id, *request.Body)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.PutContact409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.PutContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.PutContact500JSONResponse(body), nil
		default:
			return adminhttp.PutContact400JSONResponse(body), nil
		}
	}
	return adminhttp.PutContact200JSONResponse(item), nil
}

func (s *adminService) DeleteContact(ctx context.Context, request adminhttp.DeleteContactRequestObject) (adminhttp.DeleteContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminhttp.DeleteContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	owner := request.OwnerPublicKey
	id := request.Id
	item, err := s.Contacts.AdminDeleteContact(ctx, owner, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.DeleteContact409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.DeleteContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DeleteContact500JSONResponse(body), nil
		default:
			return adminhttp.DeleteContact400JSONResponse(body), nil
		}
	}
	return adminhttp.DeleteContact200JSONResponse(item), nil
}

func (s *adminService) ListFriends(ctx context.Context, request adminhttp.ListFriendsRequestObject) (adminhttp.ListFriendsResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.ListFriends500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	resp, err := s.Friends.AdminListFriends(ctx, request.Params.Cursor, request.Params.Limit)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusInternalServerError:
			return adminhttp.ListFriends500JSONResponse(body), nil
		default:
			return adminhttp.ListFriends400JSONResponse(body), nil
		}
	}
	return adminhttp.ListFriends200JSONResponse(resp), nil
}

func (s *adminService) CreateFriend(ctx context.Context, request adminhttp.CreateFriendRequestObject) (adminhttp.CreateFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.CreateFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateFriend400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND", "request body is required")), nil
	}
	item, err := s.Friends.AdminCreateFriendResource(ctx, request.Body.Id, request.Body.OwnerPublicKey, request.Body.PeerPublicKey)
	if err != nil {
		if errors.Is(err, socialutil.ErrResourceAlreadyExists) {
			return adminhttp.CreateFriend409JSONResponse(apitypes.NewErrorResponse("FRIEND_ALREADY_EXISTS", err.Error())), nil
		}
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.CreateFriend409JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.CreateFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.CreateFriend500JSONResponse(body), nil
		default:
			return adminhttp.CreateFriend400JSONResponse(body), nil
		}
	}
	return adminhttp.CreateFriend200JSONResponse(item), nil
}

func (s *adminService) GetFriend(ctx context.Context, request adminhttp.GetFriendRequestObject) (adminhttp.GetFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.GetFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	owner := request.OwnerPublicKey
	id := request.Id
	item, err := s.Friends.AdminGetFriend(ctx, owner, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.GetFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.GetFriend500JSONResponse(body), nil
		default:
			return adminhttp.GetFriend400JSONResponse(body), nil
		}
	}
	return adminhttp.GetFriend200JSONResponse(item), nil
}

func (s *adminService) DeleteFriend(ctx context.Context, request adminhttp.DeleteFriendRequestObject) (adminhttp.DeleteFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.DeleteFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	owner := request.OwnerPublicKey
	id := request.Id
	item, err := s.Friends.AdminDeleteFriend(ctx, owner, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.DeleteFriend409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.DeleteFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DeleteFriend500JSONResponse(body), nil
		default:
			return adminhttp.DeleteFriend400JSONResponse(body), nil
		}
	}
	return adminhttp.DeleteFriend200JSONResponse(item), nil
}

func (s *adminService) ListPeerFriends(ctx context.Context, request adminhttp.ListPeerFriendsRequestObject) (adminhttp.ListPeerFriendsResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.ListPeerFriends500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	resp, err := s.Friends.ListFriends(ctx, request.PublicKey, rpcapi.FriendListRequest{Cursor: request.Params.Cursor, Limit: request.Params.Limit})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.ListPeerFriends404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.ListPeerFriends500JSONResponse(body), nil
		default:
			return adminhttp.ListPeerFriends400JSONResponse(body), nil
		}
	}
	items := make([]adminhttp.AdminFriendObject, 0, len(resp.Items))
	for _, peerItem := range resp.Items {
		peerPublicKey := socialutil.StringValue(peerItem.PeerPublicKey)
		item, err := s.Friends.AdminGetFriend(ctx, request.PublicKey, socialutil.RelationID(request.PublicKey, peerPublicKey))
		if err != nil {
			return adminhttp.ListPeerFriends500JSONResponse(apitypes.NewErrorResponse("SOCIAL_SERVICE_ERROR", err.Error())), nil
		}
		items = append(items, item)
	}
	return adminhttp.ListPeerFriends200JSONResponse(adminhttp.AdminFriendListResponse{
		HasNext: resp.HasNext, Items: items, NextCursor: resp.NextCursor,
	}), nil
}

func (s *adminService) CreatePeerFriend(ctx context.Context, request adminhttp.CreatePeerFriendRequestObject) (adminhttp.CreatePeerFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.CreatePeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreatePeerFriend400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND", "request body is required")), nil
	}
	item, err := s.Friends.AdminCreateFriend(ctx, request.PublicKey, request.Body.PeerPublicKey)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.CreatePeerFriend409JSONResponse{PeerUnavailableJSONResponse: adminhttp.PeerUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.CreatePeerFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.CreatePeerFriend500JSONResponse(body), nil
		default:
			return adminhttp.CreatePeerFriend400JSONResponse(body), nil
		}
	}
	adminItem, err := s.Friends.AdminGetFriend(ctx, request.PublicKey, socialutil.RelationID(request.PublicKey, socialutil.StringValue(item.PeerPublicKey)))
	if err != nil {
		return adminhttp.CreatePeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_SERVICE_ERROR", err.Error())), nil
	}
	return adminhttp.CreatePeerFriend200JSONResponse(adminItem), nil
}

func (s *adminService) GetPeerFriend(ctx context.Context, request adminhttp.GetPeerFriendRequestObject) (adminhttp.GetPeerFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.GetPeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	item, err := s.Friends.AdminGetFriend(ctx, request.PublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.GetPeerFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.GetPeerFriend500JSONResponse(body), nil
		default:
			return adminhttp.GetPeerFriend400JSONResponse(body), nil
		}
	}
	return adminhttp.GetPeerFriend200JSONResponse(item), nil
}

func (s *adminService) DeletePeerFriend(ctx context.Context, request adminhttp.DeletePeerFriendRequestObject) (adminhttp.DeletePeerFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminhttp.DeletePeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	item, err := s.Friends.AdminDeleteFriend(ctx, request.PublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.DeletePeerFriend409JSONResponse{PeerUnavailableJSONResponse: adminhttp.PeerUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.DeletePeerFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DeletePeerFriend500JSONResponse(body), nil
		default:
			return adminhttp.DeletePeerFriend400JSONResponse(body), nil
		}
	}
	return adminhttp.DeletePeerFriend200JSONResponse(item), nil
}

func (s *adminService) ListFriendGroups(ctx context.Context, request adminhttp.ListFriendGroupsRequestObject) (adminhttp.ListFriendGroupsResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.ListFriendGroups500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	resp, err := s.FriendGroups.AdminListFriendGroupObjects(ctx, rpcapi.FriendGroupListRequest{Cursor: request.Params.Cursor, Limit: request.Params.Limit})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.ListFriendGroups404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.ListFriendGroups500JSONResponse(body), nil
		default:
			return adminhttp.ListFriendGroups400JSONResponse(body), nil
		}
	}
	return adminhttp.ListFriendGroups200JSONResponse(resp), nil
}

func (s *adminService) CreateFriendGroup(ctx context.Context, request adminhttp.CreateFriendGroupRequestObject) (adminhttp.CreateFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.CreateFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateFriendGroup400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP", "request body is required")), nil
	}
	item, err := s.FriendGroups.AdminCreateFriendGroup(ctx, request.Body.Id, request.Body.OwnerPublicKey, request.Body.Name, request.Body.DisplayName, request.Body.Description)
	if err != nil {
		if errors.Is(err, socialutil.ErrResourceAlreadyExists) {
			return adminhttp.CreateFriendGroup409JSONResponse(apitypes.NewErrorResponse("FRIEND_GROUP_ALREADY_EXISTS", err.Error())), nil
		}
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.CreateFriendGroup409JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.CreateFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.CreateFriendGroup500JSONResponse(body), nil
		default:
			return adminhttp.CreateFriendGroup400JSONResponse(body), nil
		}
	}
	return adminhttp.CreateFriendGroup200JSONResponse(item), nil
}

func (s *adminService) GetFriendGroup(ctx context.Context, request adminhttp.GetFriendGroupRequestObject) (adminhttp.GetFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.GetFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	id := request.Id
	item, err := s.FriendGroups.AdminGetFriendGroupObject(ctx, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.GetFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.GetFriendGroup500JSONResponse(body), nil
		default:
			return adminhttp.GetFriendGroup400JSONResponse(body), nil
		}
	}
	return adminhttp.GetFriendGroup200JSONResponse(item), nil
}

func (s *adminService) PutFriendGroup(ctx context.Context, request adminhttp.PutFriendGroupRequestObject) (adminhttp.PutFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.PutFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutFriendGroup400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP", "request body is required")), nil
	}
	id := request.Id
	if request.Body.Id != id {
		return adminhttp.PutFriendGroup400JSONResponse(apitypes.NewErrorResponse("RESOURCE_ID_MISMATCH", "body id must match path id")), nil
	}
	item, err := s.FriendGroups.AdminPutFriendGroup(ctx, id, request.Body.DisplayName, request.Body.Description)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.PutFriendGroup409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.PutFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.PutFriendGroup500JSONResponse(body), nil
		default:
			return adminhttp.PutFriendGroup400JSONResponse(body), nil
		}
	}
	projected, err := s.FriendGroups.AdminFriendGroupObject(ctx, id, item)
	if err != nil {
		return adminhttp.PutFriendGroup500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.PutFriendGroup200JSONResponse(projected), nil
}

func (s *adminService) DeleteFriendGroup(ctx context.Context, request adminhttp.DeleteFriendGroupRequestObject) (adminhttp.DeleteFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.DeleteFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	id := request.Id
	item, err := s.FriendGroups.AdminDeleteFriendGroup(ctx, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.DeleteFriendGroup409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.DeleteFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DeleteFriendGroup500JSONResponse(body), nil
		default:
			return adminhttp.DeleteFriendGroup400JSONResponse(body), nil
		}
	}
	projected, err := s.FriendGroups.AdminFriendGroupObject(ctx, id, item)
	if err != nil {
		return adminhttp.DeleteFriendGroup500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.DeleteFriendGroup200JSONResponse(projected), nil
}

func (s *adminService) ListFriendGroupMembers(ctx context.Context, request adminhttp.ListFriendGroupMembersRequestObject) (adminhttp.ListFriendGroupMembersResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.ListFriendGroupMembers500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	groupID := request.Id
	resp, err := s.FriendGroups.AdminListFriendGroupMembers(ctx, groupID, rpcapi.FriendGroupMemberListRequest{Cursor: request.Params.Cursor, Limit: request.Params.Limit})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.ListFriendGroupMembers404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.ListFriendGroupMembers500JSONResponse(body), nil
		default:
			return adminhttp.ListFriendGroupMembers400JSONResponse(body), nil
		}
	}
	items := make([]adminhttp.AdminFriendGroupMemberObject, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, adminFriendGroupMemberObject(groupID, item))
	}
	return adminhttp.ListFriendGroupMembers200JSONResponse{
		Items: items, HasNext: resp.HasNext, NextCursor: resp.NextCursor,
	}, nil
}

func (s *adminService) CreateFriendGroupMember(ctx context.Context, request adminhttp.CreateFriendGroupMemberRequestObject) (adminhttp.CreateFriendGroupMemberResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.CreateFriendGroupMember500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.CreateFriendGroupMember400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_MEMBER", "request body is required")), nil
	}
	groupID := request.Id
	wantID := customid.MembershipName(groupID, request.Body.PeerPublicKey)
	if request.Body.Id != wantID {
		return adminhttp.CreateFriendGroupMember400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_MEMBER", "id must match friend group and peer public key")), nil
	}
	item, err := s.FriendGroups.AdminCreateFriendGroupMember(ctx, groupID, request.Body.PeerPublicKey, request.Body.Name, request.Body.Role)
	if err != nil {
		if errors.Is(err, friendgroup.ErrFriendGroupMemberAlreadyExists) {
			return adminhttp.CreateFriendGroupMember409JSONResponse(apitypes.NewErrorResponse("FRIEND_GROUP_MEMBER_ALREADY_EXISTS", err.Error())), nil
		}
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.CreateFriendGroupMember409JSONResponse(body), nil
		case http.StatusNotFound:
			return adminhttp.CreateFriendGroupMember404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.CreateFriendGroupMember500JSONResponse(body), nil
		default:
			return adminhttp.CreateFriendGroupMember400JSONResponse(body), nil
		}
	}
	return adminhttp.CreateFriendGroupMember200JSONResponse(adminFriendGroupMemberObject(groupID, item)), nil
}

func (s *adminService) PutFriendGroupMember(ctx context.Context, request adminhttp.PutFriendGroupMemberRequestObject) (adminhttp.PutFriendGroupMemberResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.PutFriendGroupMember500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutFriendGroupMember400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_MEMBER", "request body is required")), nil
	}
	groupID := request.Id
	publicKey := request.PublicKey
	wantID := customid.MembershipName(groupID, publicKey)
	if request.Body.Id != wantID {
		return adminhttp.PutFriendGroupMember400JSONResponse(apitypes.NewErrorResponse("RESOURCE_ID_MISMATCH", "body id must match the path friend group and peer public key")), nil
	}
	item, err := s.FriendGroups.AdminPutFriendGroupMember(ctx, groupID, publicKey, "", request.Body.Role)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.PutFriendGroupMember409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.PutFriendGroupMember404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.PutFriendGroupMember500JSONResponse(body), nil
		default:
			return adminhttp.PutFriendGroupMember400JSONResponse(body), nil
		}
	}
	return adminhttp.PutFriendGroupMember200JSONResponse(adminFriendGroupMemberObject(groupID, item)), nil
}

func (s *adminService) DeleteFriendGroupMember(ctx context.Context, request adminhttp.DeleteFriendGroupMemberRequestObject) (adminhttp.DeleteFriendGroupMemberResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.DeleteFriendGroupMember500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	groupID := request.Id
	publicKey := request.PublicKey
	item, err := s.FriendGroups.AdminDeleteFriendGroupMember(ctx, groupID, publicKey)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.DeleteFriendGroupMember409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.DeleteFriendGroupMember404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DeleteFriendGroupMember500JSONResponse(body), nil
		default:
			return adminhttp.DeleteFriendGroupMember400JSONResponse(body), nil
		}
	}
	return adminhttp.DeleteFriendGroupMember200JSONResponse(adminFriendGroupMemberObject(groupID, item)), nil
}

func adminFriendGroupMemberObject(friendGroupID string, item rpcapi.FriendGroupMemberObject) adminhttp.AdminFriendGroupMemberObject {
	peerPublicKey := socialutil.StringValue(item.PeerPublicKey)
	return adminhttp.AdminFriendGroupMemberObject{
		Id:            customid.MembershipName(friendGroupID, peerPublicKey),
		FriendGroupId: friendGroupID,
		Name:          socialutil.StringValue(item.FriendGroupName),
		PeerPublicKey: peerPublicKey,
		Role:          socialutil.GroupRole(item),
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
	}
}

func (s *adminService) GetFriendGroupInviteToken(ctx context.Context, request adminhttp.GetFriendGroupInviteTokenRequestObject) (adminhttp.GetFriendGroupInviteTokenResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.GetFriendGroupInviteToken500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	id := request.Id
	resp, err := s.FriendGroups.AdminGetFriendGroupInviteToken(ctx, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.GetFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.GetFriendGroupInviteToken500JSONResponse(body), nil
		default:
			return adminhttp.GetFriendGroupInviteToken400JSONResponse(body), nil
		}
	}
	return adminhttp.GetFriendGroupInviteToken200JSONResponse(resp), nil
}

func (s *adminService) PutFriendGroupInviteToken(ctx context.Context, request adminhttp.PutFriendGroupInviteTokenRequestObject) (adminhttp.PutFriendGroupInviteTokenResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.PutFriendGroupInviteToken500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminhttp.PutFriendGroupInviteToken400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_INVITE_TOKEN", "request body is required")), nil
	}
	id := request.Id
	if request.Body.Id != id {
		return adminhttp.PutFriendGroupInviteToken400JSONResponse(apitypes.NewErrorResponse("RESOURCE_ID_MISMATCH", "body id must match path id")), nil
	}
	resp, err := s.FriendGroups.AdminPutFriendGroupInviteToken(ctx, id, request.Body.InviteToken, request.Body.ExpiresAt)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.PutFriendGroupInviteToken409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.PutFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.PutFriendGroupInviteToken500JSONResponse(body), nil
		default:
			return adminhttp.PutFriendGroupInviteToken400JSONResponse(body), nil
		}
	}
	return adminhttp.PutFriendGroupInviteToken200JSONResponse(rpcapi.FriendGroupInviteTokenGetResponse{InviteToken: &resp.InviteToken, ExpiresAt: &resp.ExpiresAt}), nil
}

func (s *adminService) DeleteFriendGroupInviteToken(ctx context.Context, request adminhttp.DeleteFriendGroupInviteTokenRequestObject) (adminhttp.DeleteFriendGroupInviteTokenResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminhttp.DeleteFriendGroupInviteToken500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	id := request.Id
	resp, err := s.FriendGroups.AdminDeleteFriendGroupInviteToken(ctx, id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusConflict:
			return adminhttp.DeleteFriendGroupInviteToken409JSONResponse{SocialUnavailableJSONResponse: adminhttp.SocialUnavailableJSONResponse(body)}, nil
		case http.StatusNotFound:
			return adminhttp.DeleteFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DeleteFriendGroupInviteToken500JSONResponse(body), nil
		default:
			return adminhttp.DeleteFriendGroupInviteToken400JSONResponse(body), nil
		}
	}
	return adminhttp.DeleteFriendGroupInviteToken200JSONResponse(resp), nil
}

func (s *adminService) ListWorkspaceHistory(ctx context.Context, request adminhttp.ListWorkspaceHistoryRequestObject) (adminhttp.ListWorkspaceHistoryResponseObject, error) {
	history, ok := s.workspaceHistory()
	if !ok {
		return adminhttp.ListWorkspaceHistory500JSONResponse(apitypes.NewErrorResponse("WORKSPACE_HISTORY_SERVICE_NOT_CONFIGURED", "workspace history service is not configured")), nil
	}
	req := apitypes.PeerRunHistoryListRequest{
		Cursor: request.Params.Cursor,
		Limit:  request.Params.Limit,
	}
	if request.Params.Order != nil {
		order := apitypes.PeerRunHistoryListRequestOrder(*request.Params.Order)
		req.Order = &order
	}
	resp, err := history.AdminListWorkspaceHistory(ctx, request.Id, req)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.ListWorkspaceHistory404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.ListWorkspaceHistory500JSONResponse(body), nil
		default:
			return adminhttp.ListWorkspaceHistory400JSONResponse(body), nil
		}
	}
	items := make([]adminhttp.AdminWorkspaceHistoryEntry, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, toAdminHistoryEntry(item))
	}
	return adminhttp.ListWorkspaceHistory200JSONResponse(adminhttp.AdminWorkspaceHistoryListResponse{
		Available:  resp.Available,
		HasNext:    resp.HasNext,
		Items:      items,
		Message:    resp.Message,
		NextCursor: resp.NextCursor,
	}), nil
}

func (s *adminService) GetWorkspaceHistory(ctx context.Context, request adminhttp.GetWorkspaceHistoryRequestObject) (adminhttp.GetWorkspaceHistoryResponseObject, error) {
	history, ok := s.workspaceHistory()
	if !ok {
		return adminhttp.GetWorkspaceHistory500JSONResponse(apitypes.NewErrorResponse("WORKSPACE_HISTORY_SERVICE_NOT_CONFIGURED", "workspace history service is not configured")), nil
	}
	entry, err := history.AdminGetWorkspaceHistory(ctx, request.Id, request.HistoryId)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.GetWorkspaceHistory404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.GetWorkspaceHistory500JSONResponse(body), nil
		default:
			return adminhttp.GetWorkspaceHistory400JSONResponse(body), nil
		}
	}
	return adminhttp.GetWorkspaceHistory200JSONResponse(toAdminHistoryEntry(entry.Public())), nil
}

func (s *adminService) DownloadWorkspaceHistoryAudio(ctx context.Context, request adminhttp.DownloadWorkspaceHistoryAudioRequestObject) (adminhttp.DownloadWorkspaceHistoryAudioResponseObject, error) {
	history, ok := s.workspaceHistory()
	if !ok {
		return adminhttp.DownloadWorkspaceHistoryAudio500JSONResponse(apitypes.NewErrorResponse("WORKSPACE_HISTORY_SERVICE_NOT_CONFIGURED", "workspace history service is not configured")), nil
	}
	r, size, err := history.AdminReadWorkspaceHistoryAudio(ctx, request.Id, request.HistoryId)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminhttp.DownloadWorkspaceHistoryAudio404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminhttp.DownloadWorkspaceHistoryAudio500JSONResponse(body), nil
		default:
			return adminhttp.DownloadWorkspaceHistoryAudio400JSONResponse(body), nil
		}
	}
	return adminhttp.DownloadWorkspaceHistoryAudio200AudiooggResponse{Body: r, ContentLength: size}, nil
}

func (s *adminService) workspaceHistory() (adminWorkspaceHistoryService, bool) {
	if s == nil || s.WorkspaceAdminService == nil {
		return nil, false
	}
	history, ok := s.WorkspaceAdminService.(adminWorkspaceHistoryService)
	return history, ok
}

func toRPCHistoryListResponse(resp apitypes.PeerRunHistoryListResponse) rpcapi.WorkspaceHistoryListResponse {
	items := make([]rpcapi.PeerRunHistoryEntry, 0, len(resp.Items))
	for _, item := range resp.Items {
		items = append(items, toRPCHistoryEntry(item))
	}
	return rpcapi.WorkspaceHistoryListResponse{
		Available:  resp.Available,
		Items:      items,
		HasNext:    resp.HasNext,
		Message:    resp.Message,
		NextCursor: resp.NextCursor,
	}
}

func toRPCHistoryEntry(item apitypes.PeerRunHistoryEntry) rpcapi.PeerRunHistoryEntry {
	return rpcapi.PeerRunHistoryEntry{
		ActorName:       item.ActorName,
		CreatedAt:       item.CreatedAt,
		GearId:          item.GearId,
		Name:            item.Name,
		ReplayAvailable: item.ReplayAvailable,
		Text:            item.Text,
		Type:            rpcapi.PeerRunHistoryEntryType(item.Type),
	}
}

func toAdminHistoryEntry(item apitypes.PeerRunHistoryEntry) adminhttp.AdminWorkspaceHistoryEntry {
	return adminhttp.AdminWorkspaceHistoryEntry{
		ActorName:       item.ActorName,
		CreatedAt:       item.CreatedAt,
		GearId:          item.GearId,
		Id:              item.Name,
		ReplayAvailable: item.ReplayAvailable,
		Text:            item.Text,
		Type:            rpcapi.PeerRunHistoryEntryType(item.Type),
	}
}

func adminSocialError(err error) (int, apitypes.ErrorResponse) {
	switch {
	case errors.Is(err, friend.ErrCrossServerFriendCreation):
		return http.StatusConflict, apitypes.NewErrorResponse("CROSS_SERVER_FRIEND_CREATION_UNSUPPORTED", friend.ErrCrossServerFriendCreation.Error())
	case errors.Is(err, friendgroup.ErrCrossServerFriendGroupMembership):
		return http.StatusConflict, apitypes.NewErrorResponse("CROSS_SERVER_FRIEND_GROUP_MEMBERSHIP_UNSUPPORTED", friendgroup.ErrCrossServerFriendGroupMembership.Error())
	case errors.Is(err, workspace.ErrWorkspacePendingDeletion):
		return http.StatusConflict, apitypes.NewErrorResponse(workspace.WorkspacePendingDeletionCode, err.Error())
	case errors.Is(err, workspace.ErrPeerPendingDeletion):
		return http.StatusConflict, apitypes.NewErrorResponse(workspace.PeerPendingDeletionCode, err.Error())
	case errors.Is(err, workspace.ErrPeerDeleted):
		return http.StatusConflict, apitypes.NewErrorResponse(workspace.PeerDeletedCode, err.Error())
	case errors.Is(err, runtimepeer.ErrPeerPendingDeletion):
		return http.StatusConflict, apitypes.NewErrorResponse(runtimepeer.PeerPendingDeletionCode, err.Error())
	case errors.Is(err, runtimepeer.ErrPeerDeleted):
		return http.StatusConflict, apitypes.NewErrorResponse(runtimepeer.PeerDeletedCode, err.Error())
	case errors.Is(err, contact.ErrPeerPendingDeletion):
		return http.StatusConflict, apitypes.NewErrorResponse(contact.PeerPendingDeletionCode, err.Error())
	case errors.Is(err, contact.ErrPeerDeleted):
		return http.StatusConflict, apitypes.NewErrorResponse(contact.PeerDeletedCode, err.Error())
	case errors.Is(err, kv.ErrNotFound), errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound, apitypes.NewErrorResponse("SOCIAL_RESOURCE_NOT_FOUND", err.Error())
	case strings.Contains(err.Error(), "not configured"),
		strings.Contains(err.Error(), "runtime store is required"),
		strings.Contains(err.Error(), "history store is required"),
		strings.Contains(err.Error(), "object store is required"),
		strings.Contains(err.Error(), "decode"),
		strings.Contains(err.Error(), "encode"):
		return http.StatusInternalServerError, apitypes.NewErrorResponse("SOCIAL_SERVICE_ERROR", err.Error())
	default:
		return http.StatusBadRequest, apitypes.NewErrorResponse("INVALID_SOCIAL_REQUEST", err.Error())
	}
}
