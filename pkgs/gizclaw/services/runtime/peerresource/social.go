package peerresource

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func (s *Server) handleContactList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsContactListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.ListContacts(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromContactListResponse)
}

func (s *Server) handleContactGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsContactGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.GetContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromContactGetResponse)
}

func (s *Server) handleContactCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsContactCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.CreateContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromContactCreateResponse)
}

func (s *Server) handleContactPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsContactPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.PutContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromContactPutResponse)
}

func (s *Server) handleContactDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsContactDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.DeleteContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromContactDeleteResponse)
}

func (s *Server) handleFriendInviteTokenGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendInviteTokenGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.GetFriendInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendInviteTokenGetResponse)
}

func (s *Server) handleFriendInviteTokenCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendInviteTokenCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.CreateFriendInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendInviteTokenCreateResponse)
}

func (s *Server) handleFriendInviteTokenClear(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendInviteTokenClearRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.ClearFriendInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendInviteTokenClearResponse)
}

func (s *Server) handleFriendAdd(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendAddRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.AddFriend(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendAddResponse)
}

func (s *Server) handleFriendList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.ListFriends(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendListResponse)
}

func (s *Server) handleFriendDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.DeleteFriend(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendDeleteResponse)
}

func (s *Server) handleFriendGroupList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendGroupListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ListFriendGroups(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupListResponse)
}

func (s *Server) handleFriendGroupGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.GetFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupGetResponse)
}

func (s *Server) handleFriendGroupCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.CreateFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupCreateResponse)
}

func (s *Server) handleFriendGroupPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.PutFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupPutResponse)
}

func (s *Server) handleFriendGroupDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.DeleteFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupDeleteResponse)
}

func (s *Server) handleFriendGroupInviteTokenGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupInviteTokenGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.GetFriendGroupInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenGetResponse)
}

func (s *Server) handleFriendGroupInviteTokenCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupInviteTokenCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.CreateFriendGroupInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenCreateResponse)
}

func (s *Server) handleFriendGroupInviteTokenClear(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupInviteTokenClearRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ClearFriendGroupInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupInviteTokenClearResponse)
}

func (s *Server) handleFriendGroupJoin(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupJoinRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.JoinFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupJoinResponse)
}

func (s *Server) handleFriendGroupMembersList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendGroupMemberListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ListFriendGroupMembers(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMemberListResponse)
}

func (s *Server) handleFriendGroupMembersAdd(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupMemberAddRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.AddFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMemberAddResponse)
}

func (s *Server) handleFriendGroupMembersPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupMemberPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.PutFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMemberPutResponse)
}

func (s *Server) handleFriendGroupMembersDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupMemberDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.DeleteFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMemberDeleteResponse)
}

func (s *Server) handleFriendGroupMessagesList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsFriendGroupMessageListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ListFriendGroupMessages(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMessageListResponse)
}

func (s *Server) handleFriendGroupMessagesGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupMessageGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.GetFriendGroupMessage(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMessageGetResponse)
}

func (s *Server) handleFriendGroupMessagesSend(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsFriendGroupMessageSendRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.SendFriendGroupMessage(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromFriendGroupMessageSendResponse)
}
