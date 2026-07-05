package peerresource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
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
	if auth := s.authorizeResponse(ctx, req.Id, acl.GameRulesetResource(resp.Name), apitypes.ACLPermissionRead); auth != nil {
		return auth
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCResponse_Result).FromServerGameRulesetGetResponse)
}

func (s *Server) handlePetDefPixaDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetDefPixaDownloadRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, reader, rpcErr, err := s.PreparePetDefPixaDownload(ctx, params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if reader != nil {
		_ = reader.Close()
	}
	if rpcErr != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcErr.Code, Message: strings.TrimSpace(rpcErr.Message)}.RPCResponse()
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetDefPixaDownloadResponse)
}

func (s *Server) PreparePetDefPixaDownload(ctx context.Context, params rpcapi.PetDefPixaDownloadRequest) (rpcapi.PetDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	runtime := s.Gameplay
	if runtime == nil || runtime.Catalog == nil {
		return rpcapi.PetDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "gameplay service not configured"}, nil
	}
	id := strings.TrimSpace(params.Id)
	if id == "" {
		return rpcapi.PetDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "pet def id is required"}, nil
	}
	item, err := runtime.Catalog.GetPetDefByID(ctx, id)
	if err != nil {
		return rpcapi.PetDefPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	allowed, err := s.authorizeGameRulesetForPetDef(ctx, runtime.Catalog, id)
	if err != nil {
		return rpcapi.PetDefPixaDownloadResponse{}, nil, nil, err
	}
	if !allowed {
		return rpcapi.PetDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: "pet def pixa is not available to this peer"}, nil
	}
	path := valueOrZero(item.PixaPath)
	reader, size, err := runtime.Catalog.OpenAsset(path)
	if err != nil {
		return rpcapi.PetDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}, nil
	}
	return rpcapi.PetDefPixaDownloadResponse{Id: item.Id, PixaPath: item.PixaPath, SizeBytes: size}, reader, nil, nil
}

func (s *Server) handleBadgeDefPixaDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsBadgeDefPixaDownloadRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, reader, rpcErr, err := s.PrepareBadgeDefPixaDownload(ctx, params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if reader != nil {
		_ = reader.Close()
	}
	if rpcErr != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcErr.Code, Message: strings.TrimSpace(rpcErr.Message)}.RPCResponse()
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromBadgeDefPixaDownloadResponse)
}

func (s *Server) PrepareBadgeDefPixaDownload(ctx context.Context, params rpcapi.BadgeDefPixaDownloadRequest) (rpcapi.BadgeDefPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	runtime := s.Gameplay
	if runtime == nil || runtime.Catalog == nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "gameplay service not configured"}, nil
	}
	id := strings.TrimSpace(params.Id)
	if id == "" {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "badge def id is required"}, nil
	}
	item, err := runtime.Catalog.GetBadgeDefByID(ctx, id)
	if err != nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	allowed, err := s.authorizeGameRulesetForBadgeDef(ctx, runtime.Catalog, id)
	if err != nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, nil, err
	}
	if !allowed {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: "badge def pixa is not available to this peer"}, nil
	}
	path := valueOrZero(item.PixaPath)
	reader, size, err := runtime.Catalog.OpenAsset(path)
	if err != nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}, nil
	}
	return rpcapi.BadgeDefPixaDownloadResponse{Id: item.Id, PixaPath: item.PixaPath, SizeBytes: size}, reader, nil, nil
}

func (s *Server) authorizeGameRulesetForPetDef(ctx context.Context, catalog *gameplay.Catalog, id string) (bool, error) {
	return s.authorizeMatchingGameRuleset(ctx, catalog, func(ruleset apitypes.GameRuleset) bool {
		for _, entry := range ruleset.Spec.PetPool {
			if strings.TrimSpace(entry.PetdefId) == id {
				return true
			}
		}
		return false
	})
}

func (s *Server) authorizeGameRulesetForBadgeDef(ctx context.Context, catalog *gameplay.Catalog, id string) (bool, error) {
	return s.authorizeMatchingGameRuleset(ctx, catalog, func(ruleset apitypes.GameRuleset) bool {
		for _, badgeID := range valueOrZero(ruleset.Spec.BadgeDefIds) {
			if strings.TrimSpace(badgeID) == id {
				return true
			}
		}
		return false
	})
}

func (s *Server) authorizeMatchingGameRuleset(ctx context.Context, catalog *gameplay.Catalog, match func(apitypes.GameRuleset) bool) (bool, error) {
	if catalog == nil {
		return false, errors.New("gameplay catalog is not configured")
	}
	limit := int32(200)
	cursor := ""
	for {
		params := adminservice.ListGameRulesetsParams{Limit: &limit}
		if cursor != "" {
			params.Cursor = &cursor
		}
		resp, err := catalog.ListGameRulesets(ctx, adminservice.ListGameRulesetsRequestObject{Params: params})
		if err != nil {
			return false, err
		}
		list, ok := resp.(adminservice.ListGameRulesets200JSONResponse)
		if !ok {
			return false, fmt.Errorf("list game rulesets returned %T", resp)
		}
		for _, ruleset := range list.Items {
			if !match(ruleset) {
				continue
			}
			allowed, err := s.authorizeGameRulesetReadOrUse(ctx, ruleset.Name)
			if err != nil || allowed {
				return allowed, err
			}
		}
		if !list.HasNext || list.NextCursor == nil {
			return false, nil
		}
		cursor = *list.NextCursor
	}
}

func (s *Server) authorizeGameRulesetReadOrUse(ctx context.Context, name string) (bool, error) {
	for _, permission := range []apitypes.ACLPermission{apitypes.ACLPermissionRead, apitypes.ACLPermissionUse} {
		err := s.authorizeErr(ctx, acl.GameRulesetResource(name), permission)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, acl.ErrDenied) {
			return false, err
		}
	}
	return false, nil
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

func gameplayRPCError(err error) *rpcapi.RPCError {
	resp := businessError("", err)
	if resp == nil || resp.Error == nil {
		return nil
	}
	return &rpcapi.RPCError{Code: resp.Error.Code, Message: resp.Error.Message}
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
	if auth := s.authorizeResponse(ctx, req.Id, acl.GameRulesetResource(ruleset.Name), apitypes.ACLPermissionUse); auth != nil {
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
