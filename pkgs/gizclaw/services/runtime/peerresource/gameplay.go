package peerresource

import (
	"context"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
)

func (s *Server) gameplayRuntime(req *rpcapi.RPCRequest) (*gameplay.Runtime, *rpcapi.RPCResponse) {
	if s.Gameplay == nil {
		return nil, internalError(req.Id, "gameplay service not configured")
	}
	return s.Gameplay, nil
}

func (s *Server) handleGameRulesetGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerGameRulesetGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetGameRuleset(ctx, valueOrZero(params.Name))
	if err != nil {
		return businessError(req.Id, err)
	}
	if auth := s.authorizeResponse(ctx, req.Id, acl.GameRulesetResource(resp.Name), apitypes.ACLPermissionGamerulesetRead); auth != nil {
		return auth
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerGameRulesetGetResponse)
}

func (s *Server) handlePetList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerPetListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.GameplayListRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.ListPets(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPetListResponse)
}

func (s *Server) handlePetGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerPetGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetPet(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPetGetResponse)
}

func (s *Server) handlePetAdopt(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerPetAdoptRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.PetAdoptRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	ruleset, err := runtime.GetGameRuleset(ctx, valueOrZero(apiParams.RulesetName))
	if err != nil {
		return businessError(req.Id, err)
	}
	if auth := s.authorizeResponse(ctx, req.Id, acl.GameRulesetResource(ruleset.Name), apitypes.ACLPermissionGamerulesetUse); auth != nil {
		return auth
	}
	apiParams.RulesetName = &ruleset.Name
	resp, err := runtime.AdoptPet(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPetAdoptResponse)
}

func (s *Server) handlePetPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerPetPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.PetPutRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.PutPet(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPetPutResponse)
}

func (s *Server) handlePetDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerPetDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.DeletePet(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPetDeleteResponse)
}

func (s *Server) handlePetDrive(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerPetDriveRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.PetDriveRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.DrivePet(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPetDriveResponse)
}

func (s *Server) handlePointsGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerPointsGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetPoints(ctx, s.Caller.String(), valueOrZero(params.RulesetName))
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPointsGetResponse)
}

func (s *Server) handlePointsTransactionsList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerPointsTransactionListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.GameplayListRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.ListPointsTransactions(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPointsTransactionListResponse)
}

func (s *Server) handlePointsTransactionsGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerPointsTransactionGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetPointsTransaction(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerPointsTransactionGetResponse)
}

func (s *Server) handleBadgeList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerBadgeListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.GameplayListRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.ListBadges(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerBadgeListResponse)
}

func (s *Server) handleBadgeGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerBadgeGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetBadge(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerBadgeGetResponse)
}

func (s *Server) handleGameResultList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerGameResultListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.GameplayListRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.ListGameResults(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerGameResultListResponse)
}

func (s *Server) handleGameResultGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerGameResultGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetGameResult(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerGameResultGetResponse)
}

func (s *Server) handleRewardGrantList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsServerRewardGrantListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.GameplayListRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	resp, err := runtime.ListRewardGrants(ctx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerRewardGrantListResponse)
}

func (s *Server) handleRewardGrantGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsServerRewardGrantGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetRewardGrant(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerRewardGrantGetResponse)
}
