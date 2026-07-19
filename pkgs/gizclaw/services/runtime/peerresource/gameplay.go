package peerresource

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
)

func (s *Server) gameplayRuntime(req *rpcapi.RPCRequest) (*gameplay.Runtime, *rpcapi.RPCResponse) {
	if s.Gameplay == nil {
		return nil, internalError(req.Id, "gameplay service not configured")
	}
	return s.Gameplay, nil
}

func (s *Server) handlePetPixaDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetPixaDownloadRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, reader, rpcErr, err := s.PreparePetPixaDownload(ctx, params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	if reader != nil {
		_ = reader.Close()
	}
	if rpcErr != nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcErr.Code, Message: strings.TrimSpace(rpcErr.Message)}.RPCResponse()
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromServerPetPixaDownloadResponse)
}

func (s *Server) PreparePetPixaDownload(ctx context.Context, params rpcapi.PetPixaDownloadRequest) (rpcapi.PetPixaDownloadResponse, io.ReadCloser, *rpcapi.RPCError, error) {
	runtime := s.Gameplay
	if runtime == nil || runtime.Catalog == nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInternalError, Message: "gameplay service not configured"}, nil
	}
	petID := strings.TrimSpace(params.PetId)
	if petID == "" {
		return rpcapi.PetPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "pet id is required"}, nil
	}
	pet, err := runtime.GetPet(ctx, s.Caller.String(), petID)
	if err != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	item, err := runtime.Catalog.GetPetDefByID(ctx, pet.PetdefId)
	if err != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	path := valueOrZero(item.PixaPath)
	reader, size, err := runtime.Catalog.OpenAsset(path)
	if err != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}, nil
	}
	return rpcapi.PetPixaDownloadResponse{PetId: pet.Id, PetdefId: item.Id, PixaPath: item.PixaPath, SizeBytes: size}, reader, nil, nil
}

func (s *Server) handleBadgeDefPixaDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsBadgeDefPixaDownloadRequest)
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
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromBadgeDefPixaDownloadResponse)
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
	if !s.profileAllows(profileBadgeDefs, id) {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeForbidden, Message: "badge def pixa is not available to this peer"}, nil
	}
	path := valueOrZero(item.PixaPath)
	reader, size, err := runtime.Catalog.OpenAsset(path)
	if err != nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}, nil
	}
	return rpcapi.BadgeDefPixaDownloadResponse{Id: item.Id, PixaPath: item.PixaPath, SizeBytes: size}, reader, nil, nil
}

func (s *Server) handlePetList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerPetListRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPetListResponse)
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
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetPet(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPetGetResponse)
}

func (s *Server) handlePetActionsGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	if runtime.Catalog == nil {
		return internalError(req.Id, "gameplay catalog is not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetActionsGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	pet, err := runtime.GetPet(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	petDef, err := runtime.Catalog.GetPetDefByID(ctx, pet.PetdefId)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, petActions(pet, petDef), (*rpcapi.RPCPayload).FromServerPetActionsGetResponse)
}

func petActions(pet apitypes.Pet, petDef apitypes.PetDef) rpcapi.PetActions {
	spec := petDef.Spec
	return rpcapi.PetActions{
		PetId:           pet.Id,
		PetdefId:        petDef.Id,
		DefaultLocale:   petDef.I18n.DefaultLocale,
		Actions:         petActionsList(spec.Drive, spec.Visual.Pixa.Metadata),
		ClipNames:       petClipNames(spec.Visual.Pixa.Metadata),
		I18n:            petActionsI18n(petDef.I18n),
		PetdefUpdatedAt: petDef.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func petActionsList(drive apitypes.PetDefDriveSpec, pixa apitypes.PetDefPixaMetadata) []rpcapi.PetAction {
	clipsByID := map[string]string{}
	clipsByAction := map[string]string{}
	for _, clip := range pixa.Clips {
		if strings.TrimSpace(clip.Id) != "" {
			clipsByID[clip.Id] = clip.PixaClipName
		}
		if clip.ActionId != nil && strings.TrimSpace(*clip.ActionId) != "" {
			clipsByAction[*clip.ActionId] = clip.PixaClipName
		}
	}

	actions := make([]rpcapi.PetAction, 0, len(drive.Actions))
	for _, action := range drive.Actions {
		item := rpcapi.PetAction{
			Id:           action.Id,
			Cost:         action.Cost,
			VisualClipId: action.VisualClipId,
		}
		if item.VisualClipId != nil {
			if clipName, ok := clipsByID[*item.VisualClipId]; ok {
				item.PixaClipName = &clipName
			}
		}
		if item.PixaClipName == nil {
			if clipName, ok := clipsByID[action.Id]; ok {
				item.PixaClipName = &clipName
			}
		}
		if item.PixaClipName == nil {
			if clipName, ok := clipsByAction[action.Id]; ok {
				item.PixaClipName = &clipName
			}
		}
		if action.Effect != nil {
			item.Effect = &rpcapi.PetActionEffectSpec{PetExpDelta: action.Effect.PetExpDelta}
			if action.Effect.AttrDelta != nil {
				item.Effect.AttrDeltaLife = petLifePtr(action.Effect.AttrDelta.Life)
			}
		}
		actions = append(actions, item)
	}
	return actions
}

func petClipNames(pixa apitypes.PetDefPixaMetadata) map[string]string {
	out := make(map[string]string, len(pixa.Clips))
	for _, clip := range pixa.Clips {
		if id := strings.TrimSpace(clip.Id); id != "" {
			out[id] = clip.PixaClipName
		}
	}
	return out
}

func petLifePtr(in *apitypes.PetLife) *rpcapi.PetLife {
	if in == nil {
		return nil
	}
	out := make(rpcapi.PetLife, len(*in))
	for key, value := range *in {
		out[key] = value
	}
	return &out
}

func petActionsI18n(in apitypes.PetDefI18nSpec) rpcapi.PetActionsI18n {
	out := make(rpcapi.PetActionsI18n, len(in.AdditionalProperties))
	for locale, catalog := range in.AdditionalProperties {
		item := rpcapi.PetActionsI18nCatalog{}
		if catalog.Drive != nil && catalog.Drive.Actions != nil {
			item.Actions = petActionsI18nDisplayMap(*catalog.Drive.Actions)
		}
		out[locale] = item
	}
	return out
}

func petActionsI18nDisplayMap(in map[string]apitypes.PetDefI18nDisplayText) map[string]rpcapi.PetActionI18nText {
	out := make(map[string]rpcapi.PetActionI18nText, len(in))
	for key, value := range in {
		out[key] = rpcapi.PetActionI18nText{Name: value.DisplayName}
	}
	return out
}

func (s *Server) handlePetAdopt(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsRuntimeAdoptRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.PetAdoptRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.AdoptPet(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromRuntimeAdoptResponse)
}

func (s *Server) handlePetPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetPutRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPetPutResponse)
}

func (s *Server) handlePetDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.DeletePet(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPetDeleteResponse)
}

func (s *Server) handlePetDrive(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetDriveRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	apiParams, err := convertType[apitypes.PetDriveRequest](params)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.DrivePet(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPetDriveResponse)
}

func (s *Server) handlePointsGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	_, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerPointsGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.GetPoints(profileCtx, s.Caller.String(), "")
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPointsGetResponse)
}

func (s *Server) gameplayProfileContext(ctx context.Context, requestID string) (context.Context, *rpcapi.RPCResponse) {
	if s == nil || s.RuntimeProfile == nil {
		return ctx, statusError(requestID, 403, "device has no active RuntimeProfile")
	}
	profile := s.RuntimeProfile()
	if profile == nil {
		return ctx, statusError(requestID, 403, "device has no active RuntimeProfile")
	}
	return gameplay.WithRuntimeProfile(ctx, *profile), nil
}

func (s *Server) handlePointsTransactionsList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerPointsTransactionListRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPointsTransactionListResponse)
}

func (s *Server) handlePointsTransactionsGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPointsTransactionGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetPointsTransaction(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPointsTransactionGetResponse)
}

func (s *Server) handleBadgeList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerBadgeListRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerBadgeListResponse)
}

func (s *Server) handleBadgeGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerBadgeGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetBadge(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerBadgeGetResponse)
}

func (s *Server) handleGameResultList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerGameResultListRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerGameResultListResponse)
}

func (s *Server) handleGameResultGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerGameResultGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetGameResult(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerGameResultGetResponse)
}

func (s *Server) handleRewardGrantList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerRewardGrantListRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerRewardGrantListResponse)
}

func (s *Server) handleRewardGrantGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerRewardGrantGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetRewardGrant(ctx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerRewardGrantGetResponse)
}
