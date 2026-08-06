package peerresource

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
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
	petName := strings.TrimSpace(params.PetName)
	if petName == "" {
		return rpcapi.PetPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "pet name is required"}, nil
	}
	profileCtx, failure := s.gameplayProfileContext(ctx, "")
	if failure != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, failure.Error, nil
	}
	pet, err := runtime.GetPetByName(profileCtx, s.Caller.String(), petName)
	if err != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	item, err := runtime.Catalog.GetPetDefByID(ctx, pet.PetDefId)
	if err != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	path := valueOrZero(item.PixaPath)
	reader, size, err := runtime.Catalog.OpenAsset(path)
	if err != nil {
		return rpcapi.PetPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}, nil
	}
	petDefName, ok := s.profileResourceName(profilePetDefs, item.Id)
	if !ok {
		return rpcapi.PetPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "pet definition is not available"}, nil
	}
	return rpcapi.PetPixaDownloadResponse{PetName: pet.Name, PetDefName: petDefName, PixaPath: item.PixaPath, SizeBytes: size}, reader, nil, nil
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
	name := strings.TrimSpace(params.Name)
	if name == "" {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeInvalidParams, Message: "badge definition name is required"}, nil
	}
	id, ok := s.resolveProfileResourceName(profileBadgeDefs, name)
	if !ok {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: "badge definition is not available"}, nil
	}
	item, err := runtime.Catalog.GetBadgeDefByID(ctx, id)
	if err != nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, gameplayRPCError(err), nil
	}
	path := valueOrZero(item.PixaPath)
	reader, size, err := runtime.Catalog.OpenAsset(path)
	if err != nil {
		return rpcapi.BadgeDefPixaDownloadResponse{}, nil, &rpcapi.RPCError{Code: rpcapi.RPCErrorCodeNotFound, Message: err.Error()}, nil
	}
	return rpcapi.BadgeDefPixaDownloadResponse{Name: name, PixaPath: item.PixaPath, SizeBytes: size}, reader, nil, nil
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.ListPets(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	items := make([]rpcapi.Pet, 0, len(resp.Items))
	for _, pet := range resp.Items {
		projected, err := s.projectPet(ctx, pet)
		if err != nil {
			return businessError(req.Id, err)
		}
		items = append(items, projected)
	}
	return resultResponse(req.Id, rpcapi.PetListResponse{Items: items, HasNext: resp.HasNext, NextCursor: resp.NextCursor}, (*rpcapi.RPCPayload).FromServerPetListResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.GetPetByName(profileCtx, s.Caller.String(), params.Name)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectPet(ctx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerPetGetResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	pet, err := runtime.GetPetByName(profileCtx, s.Caller.String(), params.Name)
	if err != nil {
		return businessError(req.Id, err)
	}
	petDef, err := runtime.Catalog.GetPetDefByID(ctx, pet.PetDefId)
	if err != nil {
		return businessError(req.Id, err)
	}
	petDefName, ok := s.profileResourceName(profilePetDefs, petDef.Id)
	if !ok {
		return statusError(req.Id, http.StatusNotFound, "pet definition is not available")
	}
	return resultResponse(req.Id, petActions(pet, petDefName, petDef), (*rpcapi.RPCPayload).FromServerPetActionsGetResponse)
}

func petActions(pet apitypes.Pet, petDefName string, petDef apitypes.PetDef) rpcapi.PetActions {
	spec := petDef.Spec
	return rpcapi.PetActions{
		PetName:    pet.Name,
		PetDefName: petDefName,
		Bindings: rpcapi.PetVisualBindings{
			Feed: spec.Visual.Bindings.Behaviors.Feed, Bathe: spec.Visual.Bindings.Behaviors.Bathe,
			Play: spec.Visual.Bindings.Behaviors.Play, Heal: spec.Visual.Bindings.Behaviors.Heal,
			Idle: spec.Visual.Bindings.States.Idle, Sick: spec.Visual.Bindings.States.Sick,
			Dead: spec.Visual.Bindings.States.Dead, Sleep: spec.Visual.Bindings.States.Sleep,
		},
		ClipNames:       petClipNames(spec.Visual.Pixa.Metadata),
		PetDefUpdatedAt: petDef.UpdatedAt.Format(time.RFC3339Nano),
	}
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

func (s *Server) projectPet(ctx context.Context, pet apitypes.Pet) (rpcapi.Pet, error) {
	runtimeProfileID, err := s.runtimeProfileID(pet.RuntimeProfileId)
	if err != nil {
		return rpcapi.Pet{}, err
	}
	petDefName, ok := s.profileResourceName(profilePetDefs, pet.PetDefId)
	if !ok {
		return rpcapi.Pet{}, errors.New("gameplay: pet definition is not available in the active RuntimeProfile")
	}
	stats, err := convertType[rpcapi.PetStats](pet.Stats)
	if err != nil {
		return rpcapi.Pet{}, err
	}
	progression, err := convertType[rpcapi.PetProgression](pet.Progression)
	if err != nil {
		return rpcapi.Pet{}, err
	}
	if s.Workspaces == nil {
		return rpcapi.Pet{}, errors.New("gameplay: workspace service is not configured")
	}
	response, err := s.Workspaces.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Id: pet.WorkspaceId})
	if err != nil {
		return rpcapi.Pet{}, err
	}
	workspace, ok := response.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		return rpcapi.Pet{}, errors.New("gameplay: pet workspace is not available")
	}
	return rpcapi.Pet{
		Name: pet.Name, RuntimeProfileName: runtimeProfileID, PetDefName: petDefName,
		DisplayName: pet.DisplayName, WorkspaceName: workspace.Name,
		Stats: stats, Progression: progression, Lifecycle: rpcapi.PetLifecycle(pet.Lifecycle),
		DiedAt: pet.DiedAt, StateSettledAt: pet.StateSettledAt, LastActiveAt: pet.LastActiveAt,
		CreatedAt: pet.CreatedAt, UpdatedAt: pet.UpdatedAt,
	}, nil
}

func (s *Server) runtimeProfileID(id string) (string, error) {
	profile := s.currentRuntimeProfile()
	if profile == nil || profile.Id == "" {
		return "", errors.New("gameplay: active RuntimeProfile is not available")
	}
	if id != profile.Id {
		return "", errors.New("gameplay: resource belongs to a different RuntimeProfile")
	}
	return profile.Id, nil
}

func (s *Server) projectPointsAccount(item apitypes.PointsAccount) (rpcapi.PointsAccount, error) {
	runtimeProfileID, err := s.runtimeProfileID(item.RuntimeProfileId)
	if err != nil {
		return rpcapi.PointsAccount{}, err
	}
	return rpcapi.PointsAccount{
		OwnerPublicKey:     item.OwnerPublicKey,
		RuntimeProfileName: runtimeProfileID,
		Balance:            item.Balance,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}, nil
}

func (s *Server) projectBadge(item apitypes.Badge) (rpcapi.Badge, error) {
	name, ok := s.profileResourceName(profileBadgeDefs, item.BadgeDefId)
	if !ok {
		return rpcapi.Badge{}, errors.New("gameplay: badge definition is not available in the active RuntimeProfile")
	}
	return rpcapi.Badge{Name: name, BadgeDefName: name, Exp: item.Exp, Level: item.Level, Active: item.Active, Progress: item.Progress, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}, nil
}

func (s *Server) projectPointsTransaction(ctx context.Context, item apitypes.PointsTransaction) (rpcapi.PointsTransaction, error) {
	projected, err := convertType[rpcapi.PointsTransaction](item)
	if err != nil {
		return rpcapi.PointsTransaction{}, err
	}
	projected.RuntimeProfileName, err = s.runtimeProfileID(item.RuntimeProfileId)
	if err != nil {
		return rpcapi.PointsTransaction{}, err
	}
	if item.PetId != nil {
		pet, err := s.Gameplay.GetPet(ctx, s.Caller.String(), *item.PetId)
		if err != nil {
			return rpcapi.PointsTransaction{}, err
		}
		projected.PetName = &pet.Name
	}
	return projected, nil
}

func (s *Server) projectGameResult(ctx context.Context, item apitypes.GameResult) (rpcapi.GameResult, error) {
	projected, err := convertType[rpcapi.GameResult](item)
	if err != nil {
		return rpcapi.GameResult{}, err
	}
	projected.RuntimeProfileName, err = s.runtimeProfileID(item.RuntimeProfileId)
	if err != nil {
		return rpcapi.GameResult{}, err
	}
	pet, err := s.Gameplay.GetPet(ctx, s.Caller.String(), item.PetId)
	if err != nil {
		return rpcapi.GameResult{}, err
	}
	gameName, ok := s.profileResourceName(profileGameDefs, item.GameDefId)
	if !ok {
		return rpcapi.GameResult{}, errors.New("gameplay: game definition is not available in the active RuntimeProfile")
	}
	projected.PetName = pet.Name
	projected.GameDefName = gameName
	return projected, nil
}

func (s *Server) projectRewardGrant(ctx context.Context, item apitypes.RewardGrant) (rpcapi.RewardGrant, error) {
	projected, err := convertType[rpcapi.RewardGrant](item)
	if err != nil {
		return rpcapi.RewardGrant{}, err
	}
	projected.RuntimeProfileName, err = s.runtimeProfileID(item.RuntimeProfileId)
	if err != nil {
		return rpcapi.RewardGrant{}, err
	}
	if item.PetId != nil {
		pet, err := s.Gameplay.GetPet(ctx, s.Caller.String(), *item.PetId)
		if err != nil {
			return rpcapi.RewardGrant{}, err
		}
		projected.PetName = &pet.Name
	}
	return projected, nil
}

func (s *Server) projectPetDriveResponse(ctx context.Context, item apitypes.PetDriveResponse) (rpcapi.PetDriveResponse, error) {
	pet, err := s.projectPet(ctx, item.Pet)
	if err != nil {
		return rpcapi.PetDriveResponse{}, err
	}
	points, err := s.projectPointsAccount(item.Points)
	if err != nil {
		return rpcapi.PetDriveResponse{}, err
	}
	out := rpcapi.PetDriveResponse{Pet: pet, Points: points}
	for _, badge := range item.Badges {
		projected, err := s.projectBadge(badge)
		if err != nil {
			return rpcapi.PetDriveResponse{}, err
		}
		out.Badges = append(out.Badges, projected)
	}
	for _, transaction := range item.Transactions {
		projected, err := s.projectPointsTransaction(ctx, transaction)
		if err != nil {
			return rpcapi.PetDriveResponse{}, err
		}
		out.Transactions = append(out.Transactions, projected)
	}
	for _, grant := range item.RewardGrants {
		projected, err := s.projectRewardGrant(ctx, grant)
		if err != nil {
			return rpcapi.PetDriveResponse{}, err
		}
		out.RewardGrants = append(out.RewardGrants, projected)
	}
	if item.GameResult != nil {
		projected, err := s.projectGameResult(ctx, *item.GameResult)
		if err != nil {
			return rpcapi.PetDriveResponse{}, err
		}
		out.GameResult = &projected
	}
	return out, nil
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
	apiParams := apitypes.PetAdoptRequest{Name: params.Name, DisplayName: params.DisplayName}
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.AdoptPet(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return gameplayBusinessError(req.Id, err)
	}
	pet, err := s.projectPet(ctx, resp.Pet)
	if err != nil {
		return businessError(req.Id, err)
	}
	transaction, err := s.projectPointsTransaction(profileCtx, resp.Transaction)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	transaction.PetName = &pet.Name
	points, err := s.projectPointsAccount(resp.Points)
	if err != nil {
		return internalError(req.Id, err.Error())
	}
	return resultResponse(req.Id, rpcapi.PetAdoptResponse{Pet: pet, Points: points, Transaction: transaction}, (*rpcapi.RPCPayload).FromRuntimeAdoptResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.PutPetByName(profileCtx, s.Caller.String(), params.Name, params.DisplayName)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectPet(ctx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerPetPutResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.DeletePetByName(profileCtx, s.Caller.String(), params.Name)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectPet(ctx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerPetDeleteResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	if s.RewardEvaluator != nil {
		profileCtx = gameplay.WithRewardEvaluator(profileCtx, s.RewardEvaluator)
	}
	pet, err := runtime.GetPetByName(profileCtx, s.Caller.String(), params.PetName)
	if err != nil {
		return businessError(req.Id, err)
	}
	apiParams := apitypes.PetDriveRequest{PetId: pet.Id, IdempotencyKey: params.IdempotencyKey}
	if params.Behavior != nil {
		behavior := apitypes.PetBehavior(*params.Behavior)
		apiParams.Behavior = &behavior
	}
	if params.GameResult != nil {
		gameDefID, ok := s.resolveProfileResourceName(profileGameDefs, params.GameResult.GameName)
		if !ok {
			return statusError(req.Id, http.StatusNotFound, "game definition is not available")
		}
		metadata, err := convertType[*apitypes.GameplayMetadata](params.GameResult.Payload)
		if err != nil {
			return internalError(req.Id, err.Error())
		}
		apiParams.GameResult = &apitypes.PetDriveGameResultInput{
			GameDefId: gameDefID, Difficulty: params.GameResult.Difficulty, DurationMs: params.GameResult.DurationMs,
			IdempotencyKey: params.GameResult.IdempotencyKey, MaxScore: params.GameResult.MaxScore,
			OccurredAt: params.GameResult.OccurredAt, Outcome: params.GameResult.Outcome,
			Payload: metadata, Score: params.GameResult.Score,
		}
	}
	resp, err := runtime.DrivePet(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return gameplayBusinessError(req.Id, err)
	}
	projected, err := s.projectPetDriveResponse(profileCtx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerPetDriveResponse)
}

func gameplayBusinessError(id string, err error) *rpcapi.RPCResponse {
	switch {
	case errors.Is(err, gameplay.ErrPetDead):
		return statusError(id, http.StatusConflict, "pet is dead")
	case errors.Is(err, gameplay.ErrPetIDConflict):
		return statusError(id, http.StatusConflict, "pet id is already reserved")
	case errors.Is(err, gameplay.ErrInvalidPetID):
		return statusError(id, http.StatusBadRequest, "invalid pet id")
	}
	return businessError(id, err)
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
	projected, err := s.projectPointsAccount(resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerPointsGetResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.ListPointsTransactions(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	items := make([]rpcapi.PointsTransaction, 0, len(resp.Items))
	for _, item := range resp.Items {
		projected, err := s.projectPointsTransaction(profileCtx, item)
		if err != nil {
			return businessError(req.Id, err)
		}
		items = append(items, projected)
	}
	return resultResponse(req.Id, rpcapi.PointsTransactionListResponse{Items: items, HasNext: resp.HasNext, NextCursor: resp.NextCursor}, (*rpcapi.RPCPayload).FromServerPointsTransactionListResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.GetPointsTransaction(profileCtx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectPointsTransaction(profileCtx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerPointsTransactionGetResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.ListBadges(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	items := make([]rpcapi.Badge, 0, len(resp.Items))
	for _, item := range resp.Items {
		projected, err := s.projectBadge(item)
		if err != nil {
			return businessError(req.Id, err)
		}
		items = append(items, projected)
	}
	return resultResponse(req.Id, rpcapi.BadgeListResponse{Items: items, HasNext: resp.HasNext, NextCursor: resp.NextCursor}, (*rpcapi.RPCPayload).FromServerBadgeListResponse)
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
	badgeDefID, ok := s.resolveProfileResourceName(profileBadgeDefs, params.Name)
	if !ok {
		return statusError(req.Id, http.StatusNotFound, "badge is not available")
	}
	resp, err := runtime.GetBadge(ctx, s.Caller.String(), badgeDefID)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectBadge(resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerBadgeGetResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.ListGameResults(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	items := make([]rpcapi.GameResult, 0, len(resp.Items))
	for _, item := range resp.Items {
		projected, err := s.projectGameResult(profileCtx, item)
		if err != nil {
			return businessError(req.Id, err)
		}
		items = append(items, projected)
	}
	return resultResponse(req.Id, rpcapi.GameResultListResponse{Items: items, HasNext: resp.HasNext, NextCursor: resp.NextCursor}, (*rpcapi.RPCPayload).FromServerGameResultListResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.GetGameResult(profileCtx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectGameResult(profileCtx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerGameResultGetResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.ListRewardGrants(profileCtx, s.Caller.String(), apiParams)
	if err != nil {
		return businessError(req.Id, err)
	}
	items := make([]rpcapi.RewardGrant, 0, len(resp.Items))
	for _, item := range resp.Items {
		projected, err := s.projectRewardGrant(profileCtx, item)
		if err != nil {
			return businessError(req.Id, err)
		}
		items = append(items, projected)
	}
	return resultResponse(req.Id, rpcapi.RewardGrantListResponse{Items: items, HasNext: resp.HasNext, NextCursor: resp.NextCursor}, (*rpcapi.RPCPayload).FromServerRewardGrantListResponse)
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
	profileCtx, failure := s.gameplayProfileContext(ctx, req.Id)
	if failure != nil {
		return failure
	}
	resp, err := runtime.GetRewardGrant(profileCtx, s.Caller.String(), params.Id)
	if err != nil {
		return businessError(req.Id, err)
	}
	projected, err := s.projectRewardGrant(profileCtx, resp)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, projected, (*rpcapi.RPCPayload).FromServerRewardGrantGetResponse)
}
