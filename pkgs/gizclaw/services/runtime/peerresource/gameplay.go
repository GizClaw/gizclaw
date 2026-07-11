package peerresource

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerGameRulesetGetRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerGameRulesetGetResponse)
}

func (s *Server) handlePetDefPixaDownload(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsPetDefPixaDownloadRequest)
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
	return resultResponse(req.Id, result, (*rpcapi.RPCPayload).FromPetDefPixaDownloadResponse)
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
		params := adminhttp.ListGameRulesetsParams{Limit: &limit}
		if cursor != "" {
			params.Cursor = &cursor
		}
		resp, err := catalog.ListGameRulesets(ctx, adminhttp.ListGameRulesetsRequestObject{Params: params})
		if err != nil {
			return false, err
		}
		list, ok := resp.(adminhttp.ListGameRulesets200JSONResponse)
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

func (s *Server) handlePetPresentationGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	if runtime.Catalog == nil {
		return internalError(req.Id, "gameplay catalog is not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCPayload.AsServerPetPresentationGetRequest)
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
	return resultResponse(req.Id, petPresentation(pet, petDef), (*rpcapi.RPCPayload).FromServerPetPresentationGetResponse)
}

func (s *Server) handlePetAdopt(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	runtime, failure := s.gameplayRuntime(req)
	if failure != nil {
		return failure
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerPetAdoptRequest)
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
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPetAdoptResponse)
}

func petPresentation(pet apitypes.Pet, petDef apitypes.PetDef) rpcapi.PetPresentation {
	spec := petDef.Spec
	return rpcapi.PetPresentation{
		PetId:           pet.Id,
		PetdefId:        petDef.Id,
		DefaultLocale:   spec.DefaultLocale,
		Attr:            petPresentationAttr(spec.Attr),
		Drive:           petPresentationDrive(spec.Drive),
		PixaMetadata:    petPresentationPixaMetadata(spec.Visual.Pixa.Metadata),
		I18n:            petPresentationI18n(spec.I18n),
		PixaPath:        petDef.PixaPath,
		PetdefUpdatedAt: petDef.UpdatedAt.Format(time.RFC3339Nano),
	}
}

func petPresentationAttr(in apitypes.PetDefAttrSpec) rpcapi.PetPresentationAttrSpec {
	return rpcapi.PetPresentationAttrSpec{
		Life:        petPresentationAttrGroup(in.Life),
		Progression: petPresentationAttrGroup(in.Progression),
	}
}

func petPresentationAttrGroup(in apitypes.PetAttrGroupSpec) rpcapi.PetPresentationAttrGroupSpec {
	out := make(rpcapi.PetPresentationAttrGroupSpec, len(in))
	for key, value := range in {
		out[key] = rpcapi.PetPresentationAttrValueSpec{Initial: value.Initial}
	}
	return out
}

func petPresentationDrive(in apitypes.PetDefDriveSpec) rpcapi.PetPresentationDriveSpec {
	actions := make([]rpcapi.PetPresentationActionSpec, 0, len(in.Actions))
	for _, action := range in.Actions {
		item := rpcapi.PetPresentationActionSpec{
			Id:           action.Id,
			Cost:         action.Cost,
			VisualClipId: action.VisualClipId,
		}
		if action.Effect != nil {
			item.Effect = &rpcapi.PetPresentationActionEffectSpec{PetExpDelta: action.Effect.PetExpDelta}
			if action.Effect.AttrDelta != nil {
				item.Effect.AttrDelta = &rpcapi.PetPresentationAttrDelta{Life: petLifePtr(action.Effect.AttrDelta.Life)}
			}
		}
		actions = append(actions, item)
	}
	return rpcapi.PetPresentationDriveSpec{Actions: actions}
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

func petPresentationPixaMetadata(in apitypes.PetDefPixaMetadata) rpcapi.PetPresentationPixaMetadata {
	clips := make([]rpcapi.PetPresentationPixaClipMetadata, 0, len(in.Clips))
	for _, clip := range in.Clips {
		clips = append(clips, rpcapi.PetPresentationPixaClipMetadata{
			Id:           clip.Id,
			ActionId:     clip.ActionId,
			PixaClipName: clip.PixaClipName,
		})
	}
	return rpcapi.PetPresentationPixaMetadata{
		Version: in.Version,
		Canvas: rpcapi.PetPresentationPixaCanvasMetadata{
			Width:  in.Canvas.Width,
			Height: in.Canvas.Height,
		},
		Clips: clips,
	}
}

func petPresentationI18n(in apitypes.PetDefI18nSpec) rpcapi.PetPresentationI18nSpec {
	out := make(rpcapi.PetPresentationI18nSpec, len(in))
	for locale, catalog := range in {
		item := rpcapi.PetPresentationI18nCatalog{
			DisplayName: catalog.DisplayName,
			Description: catalog.Description,
		}
		if catalog.Attr != nil {
			item.Attr = &rpcapi.PetPresentationI18nAttrSpec{
				Life:        petPresentationI18nAttrGroup(catalog.Attr.Life),
				Progression: petPresentationI18nAttrGroup(catalog.Attr.Progression),
			}
		}
		if catalog.Drive != nil && catalog.Drive.Actions != nil {
			item.Drive = &rpcapi.PetPresentationI18nDriveSpec{Actions: petPresentationI18nDisplayMap(*catalog.Drive.Actions)}
		}
		out[locale] = item
	}
	return out
}

func petPresentationI18nAttrGroup(in *apitypes.PetDefI18nAttrGroup) *rpcapi.PetPresentationI18nAttrGroup {
	if in == nil {
		return nil
	}
	out := rpcapi.PetPresentationI18nAttrGroup(petPresentationI18nDisplayMap(*in))
	return &out
}

func petPresentationI18nDisplayMap(in map[string]apitypes.PetDefI18nDisplayText) map[string]rpcapi.PetPresentationI18nDisplayText {
	out := make(map[string]rpcapi.PetPresentationI18nDisplayText, len(in))
	for key, value := range in {
		out[key] = rpcapi.PetPresentationI18nDisplayText{DisplayName: value.DisplayName}
	}
	return out
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
	resp, err := runtime.DrivePet(ctx, s.Caller.String(), apiParams)
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
	params, ok := decodeOptionalParams(req, rpcapi.RPCPayload.AsServerPointsGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	resp, err := runtime.GetPoints(ctx, s.Caller.String(), valueOrZero(params.RulesetName))
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, resp, (*rpcapi.RPCPayload).FromServerPointsGetResponse)
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
