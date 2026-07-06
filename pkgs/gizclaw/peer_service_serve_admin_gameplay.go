package gizclaw

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func gameplayNotConfiguredResponse() apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("GAMEPLAY_NOT_CONFIGURED", "gameplay service is not configured")
}

func (s *adminService) ListPeerPets(ctx context.Context, request adminservice.ListPeerPetsRequestObject) (adminservice.ListPeerPetsResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.ListPeerPets500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListPets(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminservice.ListPeerPets500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPeerPets200JSONResponse(resp), nil
}

func (s *adminService) GetPeerPet(ctx context.Context, request adminservice.GetPeerPetRequestObject) (adminservice.GetPeerPetResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.GetPeerPet500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetPet(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminservice.GetPeerPet404JSONResponse(apitypes.NewErrorResponse("PET_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPeerPet500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPeerPet200JSONResponse(item), nil
}

func (s *adminService) ListPeerBadges(ctx context.Context, request adminservice.ListPeerBadgesRequestObject) (adminservice.ListPeerBadgesResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.ListPeerBadges500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListBadges(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminservice.ListPeerBadges500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPeerBadges200JSONResponse(resp), nil
}

func (s *adminService) GetPeerBadge(ctx context.Context, request adminservice.GetPeerBadgeRequestObject) (adminservice.GetPeerBadgeResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.GetPeerBadge500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetBadge(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminservice.GetPeerBadge404JSONResponse(apitypes.NewErrorResponse("BADGE_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPeerBadge500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPeerBadge200JSONResponse(item), nil
}

func (s *adminService) GetPeerPoints(ctx context.Context, request adminservice.GetPeerPointsRequestObject) (adminservice.GetPeerPointsResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.GetPeerPoints500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.GetPoints(ctx, request.PublicKey, "")
	if err != nil {
		if isGameplayNotFound(err) {
			return adminservice.GetPeerPoints404JSONResponse(apitypes.NewErrorResponse("POINTS_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPeerPoints500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPeerPoints200JSONResponse(resp), nil
}

func (s *adminService) ListPeerPointsTransactions(ctx context.Context, request adminservice.ListPeerPointsTransactionsRequestObject) (adminservice.ListPeerPointsTransactionsResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.ListPeerPointsTransactions500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListPointsTransactions(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminservice.ListPeerPointsTransactions500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPeerPointsTransactions200JSONResponse(resp), nil
}

func (s *adminService) GetPeerPointsTransaction(ctx context.Context, request adminservice.GetPeerPointsTransactionRequestObject) (adminservice.GetPeerPointsTransactionResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.GetPeerPointsTransaction500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetPointsTransaction(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminservice.GetPeerPointsTransaction404JSONResponse(apitypes.NewErrorResponse("POINTS_TRANSACTION_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPeerPointsTransaction500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPeerPointsTransaction200JSONResponse(item), nil
}

func (s *adminService) ListPeerGameResults(ctx context.Context, request adminservice.ListPeerGameResultsRequestObject) (adminservice.ListPeerGameResultsResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.ListPeerGameResults500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListGameResults(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminservice.ListPeerGameResults500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPeerGameResults200JSONResponse(resp), nil
}

func (s *adminService) GetPeerGameResult(ctx context.Context, request adminservice.GetPeerGameResultRequestObject) (adminservice.GetPeerGameResultResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.GetPeerGameResult500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetGameResult(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminservice.GetPeerGameResult404JSONResponse(apitypes.NewErrorResponse("GAME_RESULT_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPeerGameResult500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPeerGameResult200JSONResponse(item), nil
}

func (s *adminService) ListPeerRewardGrants(ctx context.Context, request adminservice.ListPeerRewardGrantsRequestObject) (adminservice.ListPeerRewardGrantsResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.ListPeerRewardGrants500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListRewardGrants(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminservice.ListPeerRewardGrants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.ListPeerRewardGrants200JSONResponse(resp), nil
}

func (s *adminService) GetPeerRewardGrant(ctx context.Context, request adminservice.GetPeerRewardGrantRequestObject) (adminservice.GetPeerRewardGrantResponseObject, error) {
	if s.Gameplay == nil {
		return adminservice.GetPeerRewardGrant500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetRewardGrant(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminservice.GetPeerRewardGrant404JSONResponse(apitypes.NewErrorResponse("REWARD_GRANT_NOT_FOUND", err.Error())), nil
		}
		return adminservice.GetPeerRewardGrant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminservice.GetPeerRewardGrant200JSONResponse(item), nil
}

func intPtrFromInt32(v *int32) *int {
	if v == nil {
		return nil
	}
	out := int(*v)
	return &out
}

func isGameplayNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, kv.ErrNotFound)
}
