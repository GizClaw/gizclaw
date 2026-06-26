package gizclaw

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

type adminWorkspaceHistoryService interface {
	AdminListWorkspaceHistory(context.Context, string, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error)
	AdminGetWorkspaceHistory(context.Context, string, string) (workspace.HistoryEntry, error)
	AdminReadWorkspaceHistoryAudio(context.Context, string, string) (io.ReadCloser, int64, error)
}

func (s *adminService) ListContacts(ctx context.Context, request adminservice.ListContactsRequestObject) (adminservice.ListContactsResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminservice.ListContacts500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	resp, err := s.Contacts.AdminListContacts(ctx, socialutil.StringValue(request.Params.OwnerPublicKey), request.Params.Cursor, request.Params.Limit)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusInternalServerError:
			return adminservice.ListContacts500JSONResponse(body), nil
		default:
			return adminservice.ListContacts400JSONResponse(body), nil
		}
	}
	return adminservice.ListContacts200JSONResponse(resp), nil
}

func (s *adminService) CreateContact(ctx context.Context, request adminservice.CreateContactRequestObject) (adminservice.CreateContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminservice.CreateContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.CreateContact400JSONResponse(apitypes.NewErrorResponse("INVALID_CONTACT", "request body is required")), nil
	}
	item, err := s.Contacts.AdminCreateContact(ctx, *request.Body)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.CreateContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.CreateContact500JSONResponse(body), nil
		default:
			return adminservice.CreateContact400JSONResponse(body), nil
		}
	}
	return adminservice.CreateContact200JSONResponse(item), nil
}

func (s *adminService) GetContact(ctx context.Context, request adminservice.GetContactRequestObject) (adminservice.GetContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminservice.GetContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	item, err := s.Contacts.AdminGetContact(ctx, request.OwnerPublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.GetContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.GetContact500JSONResponse(body), nil
		default:
			return adminservice.GetContact400JSONResponse(body), nil
		}
	}
	return adminservice.GetContact200JSONResponse(item), nil
}

func (s *adminService) PutContact(ctx context.Context, request adminservice.PutContactRequestObject) (adminservice.PutContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminservice.PutContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.PutContact400JSONResponse(apitypes.NewErrorResponse("INVALID_CONTACT", "request body is required")), nil
	}
	item, err := s.Contacts.AdminPutContact(ctx, request.OwnerPublicKey, request.Id, *request.Body)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.PutContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.PutContact500JSONResponse(body), nil
		default:
			return adminservice.PutContact400JSONResponse(body), nil
		}
	}
	return adminservice.PutContact200JSONResponse(item), nil
}

func (s *adminService) DeleteContact(ctx context.Context, request adminservice.DeleteContactRequestObject) (adminservice.DeleteContactResponseObject, error) {
	if s == nil || s.Contacts == nil {
		return adminservice.DeleteContact500JSONResponse(apitypes.NewErrorResponse("SOCIAL_CONTACT_SERVICE_NOT_CONFIGURED", "contact service is not configured")), nil
	}
	item, err := s.Contacts.AdminDeleteContact(ctx, request.OwnerPublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DeleteContact404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DeleteContact500JSONResponse(body), nil
		default:
			return adminservice.DeleteContact400JSONResponse(body), nil
		}
	}
	return adminservice.DeleteContact200JSONResponse(item), nil
}

func (s *adminService) ListFriends(ctx context.Context, request adminservice.ListFriendsRequestObject) (adminservice.ListFriendsResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.ListFriends500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	resp, err := s.Friends.AdminListFriends(ctx, request.Params.Cursor, request.Params.Limit)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusInternalServerError:
			return adminservice.ListFriends500JSONResponse(body), nil
		default:
			return adminservice.ListFriends400JSONResponse(body), nil
		}
	}
	return adminservice.ListFriends200JSONResponse(resp), nil
}

func (s *adminService) CreateFriend(ctx context.Context, request adminservice.CreateFriendRequestObject) (adminservice.CreateFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.CreateFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.CreateFriend400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND", "request body is required")), nil
	}
	item, err := s.Friends.AdminCreateFriendResource(ctx, request.Body.OwnerPublicKey, request.Body.PeerPublicKey)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.CreateFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.CreateFriend500JSONResponse(body), nil
		default:
			return adminservice.CreateFriend400JSONResponse(body), nil
		}
	}
	return adminservice.CreateFriend200JSONResponse(item), nil
}

func (s *adminService) GetFriend(ctx context.Context, request adminservice.GetFriendRequestObject) (adminservice.GetFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.GetFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	item, err := s.Friends.AdminGetFriend(ctx, request.OwnerPublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.GetFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.GetFriend500JSONResponse(body), nil
		default:
			return adminservice.GetFriend400JSONResponse(body), nil
		}
	}
	return adminservice.GetFriend200JSONResponse(item), nil
}

func (s *adminService) DeleteFriend(ctx context.Context, request adminservice.DeleteFriendRequestObject) (adminservice.DeleteFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.DeleteFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	item, err := s.Friends.AdminDeleteFriend(ctx, request.OwnerPublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DeleteFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DeleteFriend500JSONResponse(body), nil
		default:
			return adminservice.DeleteFriend400JSONResponse(body), nil
		}
	}
	return adminservice.DeleteFriend200JSONResponse(item), nil
}

func (s *adminService) ListPeerFriends(ctx context.Context, request adminservice.ListPeerFriendsRequestObject) (adminservice.ListPeerFriendsResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.ListPeerFriends500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	resp, err := s.Friends.ListFriends(ctx, request.PublicKey, rpcapi.FriendListRequest{Cursor: request.Params.Cursor, Limit: request.Params.Limit})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.ListPeerFriends404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.ListPeerFriends500JSONResponse(body), nil
		default:
			return adminservice.ListPeerFriends400JSONResponse(body), nil
		}
	}
	return adminservice.ListPeerFriends200JSONResponse(resp), nil
}

func (s *adminService) CreatePeerFriend(ctx context.Context, request adminservice.CreatePeerFriendRequestObject) (adminservice.CreatePeerFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.CreatePeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.CreatePeerFriend400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND", "request body is required")), nil
	}
	item, err := s.Friends.AdminCreateFriend(ctx, request.PublicKey, request.Body.PeerPublicKey)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.CreatePeerFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.CreatePeerFriend500JSONResponse(body), nil
		default:
			return adminservice.CreatePeerFriend400JSONResponse(body), nil
		}
	}
	return adminservice.CreatePeerFriend200JSONResponse(item), nil
}

func (s *adminService) GetPeerFriend(ctx context.Context, request adminservice.GetPeerFriendRequestObject) (adminservice.GetPeerFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.GetPeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	item, err := s.Friends.GetFriendRelation(ctx, request.PublicKey, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.GetPeerFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.GetPeerFriend500JSONResponse(body), nil
		default:
			return adminservice.GetPeerFriend400JSONResponse(body), nil
		}
	}
	return adminservice.GetPeerFriend200JSONResponse(item), nil
}

func (s *adminService) DeletePeerFriend(ctx context.Context, request adminservice.DeletePeerFriendRequestObject) (adminservice.DeletePeerFriendResponseObject, error) {
	if s == nil || s.Friends == nil {
		return adminservice.DeletePeerFriend500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_SERVICE_NOT_CONFIGURED", "friend service is not configured")), nil
	}
	item, err := s.Friends.DeleteFriend(ctx, request.PublicKey, rpcapi.FriendDeleteRequest{Id: request.Id})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DeletePeerFriend404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DeletePeerFriend500JSONResponse(body), nil
		default:
			return adminservice.DeletePeerFriend400JSONResponse(body), nil
		}
	}
	return adminservice.DeletePeerFriend200JSONResponse(item), nil
}

func (s *adminService) ListFriendGroups(ctx context.Context, request adminservice.ListFriendGroupsRequestObject) (adminservice.ListFriendGroupsResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.ListFriendGroups500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	resp, err := s.FriendGroups.AdminListFriendGroups(ctx, rpcapi.FriendGroupListRequest{Cursor: request.Params.Cursor, Limit: request.Params.Limit})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.ListFriendGroups404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.ListFriendGroups500JSONResponse(body), nil
		default:
			return adminservice.ListFriendGroups400JSONResponse(body), nil
		}
	}
	return adminservice.ListFriendGroups200JSONResponse(resp), nil
}

func (s *adminService) CreateFriendGroup(ctx context.Context, request adminservice.CreateFriendGroupRequestObject) (adminservice.CreateFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.CreateFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.CreateFriendGroup400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP", "request body is required")), nil
	}
	item, err := s.FriendGroups.AdminCreateFriendGroup(ctx, request.Body.Name, request.Body.Description)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.CreateFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.CreateFriendGroup500JSONResponse(body), nil
		default:
			return adminservice.CreateFriendGroup400JSONResponse(body), nil
		}
	}
	return adminservice.CreateFriendGroup200JSONResponse(item), nil
}

func (s *adminService) GetFriendGroup(ctx context.Context, request adminservice.GetFriendGroupRequestObject) (adminservice.GetFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.GetFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	item, err := s.FriendGroups.AdminGetFriendGroup(ctx, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.GetFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.GetFriendGroup500JSONResponse(body), nil
		default:
			return adminservice.GetFriendGroup400JSONResponse(body), nil
		}
	}
	return adminservice.GetFriendGroup200JSONResponse(item), nil
}

func (s *adminService) PutFriendGroup(ctx context.Context, request adminservice.PutFriendGroupRequestObject) (adminservice.PutFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.PutFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.PutFriendGroup400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP", "request body is required")), nil
	}
	item, err := s.FriendGroups.AdminPutFriendGroup(ctx, request.Id, request.Body.Name, request.Body.Description)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.PutFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.PutFriendGroup500JSONResponse(body), nil
		default:
			return adminservice.PutFriendGroup400JSONResponse(body), nil
		}
	}
	return adminservice.PutFriendGroup200JSONResponse(item), nil
}

func (s *adminService) DeleteFriendGroup(ctx context.Context, request adminservice.DeleteFriendGroupRequestObject) (adminservice.DeleteFriendGroupResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.DeleteFriendGroup500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	item, err := s.FriendGroups.AdminDeleteFriendGroup(ctx, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DeleteFriendGroup404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DeleteFriendGroup500JSONResponse(body), nil
		default:
			return adminservice.DeleteFriendGroup400JSONResponse(body), nil
		}
	}
	return adminservice.DeleteFriendGroup200JSONResponse(item), nil
}

func (s *adminService) ListFriendGroupMembers(ctx context.Context, request adminservice.ListFriendGroupMembersRequestObject) (adminservice.ListFriendGroupMembersResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.ListFriendGroupMembers500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	resp, err := s.FriendGroups.AdminListFriendGroupMembers(ctx, request.Id, rpcapi.FriendGroupMemberListRequest{Cursor: request.Params.Cursor, Limit: request.Params.Limit})
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.ListFriendGroupMembers404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.ListFriendGroupMembers500JSONResponse(body), nil
		default:
			return adminservice.ListFriendGroupMembers400JSONResponse(body), nil
		}
	}
	return adminservice.ListFriendGroupMembers200JSONResponse(resp), nil
}

func (s *adminService) CreateFriendGroupMember(ctx context.Context, request adminservice.CreateFriendGroupMemberRequestObject) (adminservice.CreateFriendGroupMemberResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.CreateFriendGroupMember500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.CreateFriendGroupMember400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_MEMBER", "request body is required")), nil
	}
	item, err := s.FriendGroups.AdminPutFriendGroupMember(ctx, request.Id, request.Body.PeerPublicKey, request.Body.Role)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.CreateFriendGroupMember404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.CreateFriendGroupMember500JSONResponse(body), nil
		default:
			return adminservice.CreateFriendGroupMember400JSONResponse(body), nil
		}
	}
	return adminservice.CreateFriendGroupMember200JSONResponse(item), nil
}

func (s *adminService) PutFriendGroupMember(ctx context.Context, request adminservice.PutFriendGroupMemberRequestObject) (adminservice.PutFriendGroupMemberResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.PutFriendGroupMember500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.PutFriendGroupMember400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_MEMBER", "request body is required")), nil
	}
	item, err := s.FriendGroups.AdminPutFriendGroupMember(ctx, request.Id, request.PublicKey, request.Body.Role)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.PutFriendGroupMember404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.PutFriendGroupMember500JSONResponse(body), nil
		default:
			return adminservice.PutFriendGroupMember400JSONResponse(body), nil
		}
	}
	return adminservice.PutFriendGroupMember200JSONResponse(item), nil
}

func (s *adminService) DeleteFriendGroupMember(ctx context.Context, request adminservice.DeleteFriendGroupMemberRequestObject) (adminservice.DeleteFriendGroupMemberResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.DeleteFriendGroupMember500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	item, err := s.FriendGroups.AdminDeleteFriendGroupMember(ctx, request.Id, request.PublicKey)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DeleteFriendGroupMember404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DeleteFriendGroupMember500JSONResponse(body), nil
		default:
			return adminservice.DeleteFriendGroupMember400JSONResponse(body), nil
		}
	}
	return adminservice.DeleteFriendGroupMember200JSONResponse(item), nil
}

func (s *adminService) GetFriendGroupInviteToken(ctx context.Context, request adminservice.GetFriendGroupInviteTokenRequestObject) (adminservice.GetFriendGroupInviteTokenResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.GetFriendGroupInviteToken500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	resp, err := s.FriendGroups.AdminGetFriendGroupInviteToken(ctx, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.GetFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.GetFriendGroupInviteToken500JSONResponse(body), nil
		default:
			return adminservice.GetFriendGroupInviteToken400JSONResponse(body), nil
		}
	}
	return adminservice.GetFriendGroupInviteToken200JSONResponse(resp), nil
}

func (s *adminService) PutFriendGroupInviteToken(ctx context.Context, request adminservice.PutFriendGroupInviteTokenRequestObject) (adminservice.PutFriendGroupInviteTokenResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.PutFriendGroupInviteToken500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	if request.Body == nil {
		return adminservice.PutFriendGroupInviteToken400JSONResponse(apitypes.NewErrorResponse("INVALID_FRIEND_GROUP_INVITE_TOKEN", "request body is required")), nil
	}
	resp, err := s.FriendGroups.AdminPutFriendGroupInviteToken(ctx, request.Id, request.Body.InviteToken, request.Body.ExpiresAt)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.PutFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.PutFriendGroupInviteToken500JSONResponse(body), nil
		default:
			return adminservice.PutFriendGroupInviteToken400JSONResponse(body), nil
		}
	}
	return adminservice.PutFriendGroupInviteToken200JSONResponse(rpcapi.FriendGroupInviteTokenGetResponse{InviteToken: &resp.InviteToken, ExpiresAt: &resp.ExpiresAt}), nil
}

func (s *adminService) DeleteFriendGroupInviteToken(ctx context.Context, request adminservice.DeleteFriendGroupInviteTokenRequestObject) (adminservice.DeleteFriendGroupInviteTokenResponseObject, error) {
	if s == nil || s.FriendGroups == nil {
		return adminservice.DeleteFriendGroupInviteToken500JSONResponse(apitypes.NewErrorResponse("SOCIAL_FRIEND_GROUP_SERVICE_NOT_CONFIGURED", "friend group service is not configured")), nil
	}
	resp, err := s.FriendGroups.AdminDeleteFriendGroupInviteToken(ctx, request.Id)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DeleteFriendGroupInviteToken404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DeleteFriendGroupInviteToken500JSONResponse(body), nil
		default:
			return adminservice.DeleteFriendGroupInviteToken400JSONResponse(body), nil
		}
	}
	return adminservice.DeleteFriendGroupInviteToken200JSONResponse(resp), nil
}

func (s *adminService) ListWorkspaceHistory(ctx context.Context, request adminservice.ListWorkspaceHistoryRequestObject) (adminservice.ListWorkspaceHistoryResponseObject, error) {
	history, ok := s.workspaceHistory()
	if !ok {
		return adminservice.ListWorkspaceHistory500JSONResponse(apitypes.NewErrorResponse("WORKSPACE_HISTORY_SERVICE_NOT_CONFIGURED", "workspace history service is not configured")), nil
	}
	req := apitypes.PeerRunHistoryListRequest{
		Cursor: request.Params.Cursor,
		Limit:  request.Params.Limit,
	}
	if request.Params.Order != nil {
		order := apitypes.PeerRunHistoryListRequestOrder(*request.Params.Order)
		req.Order = &order
	}
	resp, err := history.AdminListWorkspaceHistory(ctx, request.Name, req)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.ListWorkspaceHistory404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.ListWorkspaceHistory500JSONResponse(body), nil
		default:
			return adminservice.ListWorkspaceHistory400JSONResponse(body), nil
		}
	}
	return adminservice.ListWorkspaceHistory200JSONResponse(toRPCHistoryListResponse(resp)), nil
}

func (s *adminService) GetWorkspaceHistory(ctx context.Context, request adminservice.GetWorkspaceHistoryRequestObject) (adminservice.GetWorkspaceHistoryResponseObject, error) {
	history, ok := s.workspaceHistory()
	if !ok {
		return adminservice.GetWorkspaceHistory500JSONResponse(apitypes.NewErrorResponse("WORKSPACE_HISTORY_SERVICE_NOT_CONFIGURED", "workspace history service is not configured")), nil
	}
	entry, err := history.AdminGetWorkspaceHistory(ctx, request.Name, request.HistoryId)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.GetWorkspaceHistory404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.GetWorkspaceHistory500JSONResponse(body), nil
		default:
			return adminservice.GetWorkspaceHistory400JSONResponse(body), nil
		}
	}
	return adminservice.GetWorkspaceHistory200JSONResponse(toRPCHistoryEntry(entry.Public())), nil
}

func (s *adminService) DownloadWorkspaceHistoryAudio(ctx context.Context, request adminservice.DownloadWorkspaceHistoryAudioRequestObject) (adminservice.DownloadWorkspaceHistoryAudioResponseObject, error) {
	history, ok := s.workspaceHistory()
	if !ok {
		return adminservice.DownloadWorkspaceHistoryAudio500JSONResponse(apitypes.NewErrorResponse("WORKSPACE_HISTORY_SERVICE_NOT_CONFIGURED", "workspace history service is not configured")), nil
	}
	r, size, err := history.AdminReadWorkspaceHistoryAudio(ctx, request.Name, request.HistoryId)
	if err != nil {
		status, body := adminSocialError(err)
		switch status {
		case http.StatusNotFound:
			return adminservice.DownloadWorkspaceHistoryAudio404JSONResponse(body), nil
		case http.StatusInternalServerError:
			return adminservice.DownloadWorkspaceHistoryAudio500JSONResponse(body), nil
		default:
			return adminservice.DownloadWorkspaceHistoryAudio400JSONResponse(body), nil
		}
	}
	return adminservice.DownloadWorkspaceHistoryAudio200AudiooggResponse{Body: r, ContentLength: size}, nil
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
		CreatedAt:       item.CreatedAt,
		GearId:          item.GearId,
		Id:              item.Id,
		Name:            item.Name,
		ReplayAvailable: item.ReplayAvailable,
		Text:            item.Text,
		Type:            rpcapi.PeerRunHistoryEntryType(item.Type),
	}
}

func adminSocialError(err error) (int, apitypes.ErrorResponse) {
	switch {
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
