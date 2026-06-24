package peerresource

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func (s *Server) handleContactList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsContactListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.ListContacts(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactListResponse)
}

func (s *Server) handleContactGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.GetContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactGetResponse)
}

func (s *Server) handleContactCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.CreateContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactCreateResponse)
}

func (s *Server) handleContactPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.PutContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactPutResponse)
}

func (s *Server) handleContactDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Contacts == nil {
		return internalError(req.Id, "contact service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Contacts.DeleteContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactDeleteResponse)
}

func (s *Server) handleFriendInviteTokenGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendInviteTokenGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.GetFriendInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendInviteTokenGetResponse)
}

func (s *Server) handleFriendInviteTokenCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendInviteTokenCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.CreateFriendInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendInviteTokenCreateResponse)
}

func (s *Server) handleFriendInviteTokenClear(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendInviteTokenClearRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.ClearFriendInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendInviteTokenClearResponse)
}

func (s *Server) handleFriendAdd(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendAddRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.AddFriend(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendAddResponse)
}

func (s *Server) handleFriendList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.ListFriends(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendListResponse)
}

func (s *Server) handleFriendDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Friends == nil {
		return internalError(req.Id, "friend service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Friends.DeleteFriend(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendDeleteResponse)
}

func (s *Server) handleFriendGroupList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendGroupListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ListFriendGroups(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupListResponse)
}

func (s *Server) handleFriendGroupGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.GetFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupGetResponse)
}

func (s *Server) handleFriendGroupCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.CreateFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupCreateResponse)
}

func (s *Server) handleFriendGroupPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.PutFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupPutResponse)
}

func (s *Server) handleFriendGroupDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.DeleteFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupDeleteResponse)
}

func (s *Server) handleFriendGroupInviteTokenGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupInviteTokenGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.GetFriendGroupInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupInviteTokenGetResponse)
}

func (s *Server) handleFriendGroupInviteTokenCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupInviteTokenCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.CreateFriendGroupInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupInviteTokenCreateResponse)
}

func (s *Server) handleFriendGroupInviteTokenClear(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupInviteTokenClearRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ClearFriendGroupInviteToken(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupInviteTokenClearResponse)
}

func (s *Server) handleFriendGroupJoin(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupJoinRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.JoinFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupJoinResponse)
}

func (s *Server) handleFriendGroupMembersList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ListFriendGroupMembers(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberListResponse)
}

func (s *Server) handleFriendGroupMembersAdd(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberAddRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.AddFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberAddResponse)
}

func (s *Server) handleFriendGroupMembersPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.PutFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberPutResponse)
}

func (s *Server) handleFriendGroupMembersDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.DeleteFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberDeleteResponse)
}

func (s *Server) handleFriendGroupMessagesList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMessageListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.ListFriendGroupMessages(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageListResponse)
}

func (s *Server) handleFriendGroupMessagesGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMessageGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.GetFriendGroupMessage(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageGetResponse)
}

func (s *Server) handleFriendGroupMessagesSend(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.FriendGroups == nil {
		return internalError(req.Id, "friend group service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMessageSendRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.FriendGroups.SendFriendGroupMessage(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageSendResponse)
}
