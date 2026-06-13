package peerresource

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func (s *Server) handleContactList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsContactListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.ListContacts(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactListResponse)
}

func (s *Server) handleContactGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.GetContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactGetResponse)
}

func (s *Server) handleContactCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.CreateContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactCreateResponse)
}

func (s *Server) handleContactPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.PutContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactPutResponse)
}

func (s *Server) handleContactDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsContactDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.DeleteContact(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromContactDeleteResponse)
}

func (s *Server) handleFriendRequestsList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendRequestListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.ListFriendRequests(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendRequestListResponse)
}

func (s *Server) handleFriendRequestsCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendRequestCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.CreateFriendRequest(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendRequestCreateResponse)
}

func (s *Server) handleFriendRequestsAccept(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendRequestAcceptRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.AcceptFriendRequest(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendRequestAcceptResponse)
}

func (s *Server) handleFriendRequestsReject(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendRequestRejectRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.RejectFriendRequest(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendRequestRejectResponse)
}

func (s *Server) handleFriendList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.ListFriends(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendListResponse)
}

func (s *Server) handleFriendDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.DeleteFriend(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendDeleteResponse)
}

func (s *Server) handleFriendGroupList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendGroupListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.ListFriendGroups(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupListResponse)
}

func (s *Server) handleFriendGroupGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.GetFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupGetResponse)
}

func (s *Server) handleFriendGroupCreate(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupCreateRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.CreateFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupCreateResponse)
}

func (s *Server) handleFriendGroupPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.PutFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupPutResponse)
}

func (s *Server) handleFriendGroupDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.DeleteFriendGroup(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupDeleteResponse)
}

func (s *Server) handleFriendGroupMembersList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.ListFriendGroupMembers(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberListResponse)
}

func (s *Server) handleFriendGroupMembersAdd(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberAddRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.AddFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberAddResponse)
}

func (s *Server) handleFriendGroupMembersPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.PutFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberPutResponse)
}

func (s *Server) handleFriendGroupMembersDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMemberDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.DeleteFriendGroupMember(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMemberDeleteResponse)
}

func (s *Server) handleFriendGroupMessagesList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMessageListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.ListFriendGroupMessages(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageListResponse)
}

func (s *Server) handleFriendGroupMessagesGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMessageGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.GetFriendGroupMessage(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageGetResponse)
}

func (s *Server) handleFriendGroupMessagesSend(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Social == nil {
		return internalError(req.Id, "social service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsFriendGroupMessageSendRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Social.SendFriendGroupMessage(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromFriendGroupMessageSendResponse)
}
