package gizclaw

import (
	"context"
	"database/sql"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func gameplayNotConfiguredResponse() apitypes.ErrorResponse {
	return apitypes.NewErrorResponse("GAMEPLAY_NOT_CONFIGURED", "gameplay service is not configured")
}

func (s *adminService) ListPeerPets(ctx context.Context, request adminhttp.ListPeerPetsRequestObject) (adminhttp.ListPeerPetsResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.ListPeerPets500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListPets(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminhttp.ListPeerPets500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPeerPets200JSONResponse(resp), nil
}

func (s *adminService) GetPeerPet(ctx context.Context, request adminhttp.GetPeerPetRequestObject) (adminhttp.GetPeerPetResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.GetPeerPet500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetPet(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminhttp.GetPeerPet404JSONResponse(apitypes.NewErrorResponse("PET_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPeerPet500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPeerPet200JSONResponse(item), nil
}

func (s *adminService) ListPeerBadges(ctx context.Context, request adminhttp.ListPeerBadgesRequestObject) (adminhttp.ListPeerBadgesResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.ListPeerBadges500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListBadges(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminhttp.ListPeerBadges500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPeerBadges200JSONResponse(resp), nil
}

func (s *adminService) GetPeerBadge(ctx context.Context, request adminhttp.GetPeerBadgeRequestObject) (adminhttp.GetPeerBadgeResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.GetPeerBadge500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetBadge(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminhttp.GetPeerBadge404JSONResponse(apitypes.NewErrorResponse("BADGE_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPeerBadge500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPeerBadge200JSONResponse(item), nil
}

func (s *adminService) GetPeerPoints(ctx context.Context, request adminhttp.GetPeerPointsRequestObject) (adminhttp.GetPeerPointsResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.GetPeerPoints500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.GetPoints(ctx, request.PublicKey, "")
	if err != nil {
		if isGameplayNotFound(err) {
			return adminhttp.GetPeerPoints404JSONResponse(apitypes.NewErrorResponse("POINTS_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPeerPoints500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPeerPoints200JSONResponse(resp), nil
}

func (s *adminService) ListPeerPointsTransactions(ctx context.Context, request adminhttp.ListPeerPointsTransactionsRequestObject) (adminhttp.ListPeerPointsTransactionsResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.ListPeerPointsTransactions500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListPointsTransactions(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminhttp.ListPeerPointsTransactions500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPeerPointsTransactions200JSONResponse(resp), nil
}

func (s *adminService) GetPeerPointsTransaction(ctx context.Context, request adminhttp.GetPeerPointsTransactionRequestObject) (adminhttp.GetPeerPointsTransactionResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.GetPeerPointsTransaction500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetPointsTransaction(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminhttp.GetPeerPointsTransaction404JSONResponse(apitypes.NewErrorResponse("POINTS_TRANSACTION_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPeerPointsTransaction500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPeerPointsTransaction200JSONResponse(item), nil
}

func (s *adminService) ListPeerGameResults(ctx context.Context, request adminhttp.ListPeerGameResultsRequestObject) (adminhttp.ListPeerGameResultsResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.ListPeerGameResults500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListGameResults(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminhttp.ListPeerGameResults500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPeerGameResults200JSONResponse(resp), nil
}

func (s *adminService) GetPeerGameResult(ctx context.Context, request adminhttp.GetPeerGameResultRequestObject) (adminhttp.GetPeerGameResultResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.GetPeerGameResult500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetGameResult(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminhttp.GetPeerGameResult404JSONResponse(apitypes.NewErrorResponse("GAME_RESULT_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPeerGameResult500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPeerGameResult200JSONResponse(item), nil
}

func (s *adminService) ListPeerRewardGrants(ctx context.Context, request adminhttp.ListPeerRewardGrantsRequestObject) (adminhttp.ListPeerRewardGrantsResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.ListPeerRewardGrants500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	resp, err := s.Gameplay.ListRewardGrants(ctx, request.PublicKey, apitypes.GameplayListRequest{Cursor: request.Params.Cursor, Limit: intPtrFromInt32(request.Params.Limit)})
	if err != nil {
		return adminhttp.ListPeerRewardGrants500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.ListPeerRewardGrants200JSONResponse(resp), nil
}

func (s *adminService) GetPeerRewardGrant(ctx context.Context, request adminhttp.GetPeerRewardGrantRequestObject) (adminhttp.GetPeerRewardGrantResponseObject, error) {
	if s.Gameplay == nil {
		return adminhttp.GetPeerRewardGrant500JSONResponse(gameplayNotConfiguredResponse()), nil
	}
	item, err := s.Gameplay.GetRewardGrant(ctx, request.PublicKey, request.Id)
	if err != nil {
		if isGameplayNotFound(err) {
			return adminhttp.GetPeerRewardGrant404JSONResponse(apitypes.NewErrorResponse("REWARD_GRANT_NOT_FOUND", err.Error())), nil
		}
		return adminhttp.GetPeerRewardGrant500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return adminhttp.GetPeerRewardGrant200JSONResponse(item), nil
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
