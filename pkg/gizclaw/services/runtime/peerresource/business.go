package peerresource

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
)

func (s *Server) handlePetList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsPetListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	owner := s.Caller.String()
	result, err := s.Pets.ListPets(ctx, owner, params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetListResponse)
}

func (s *Server) handlePetGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Pets.GetPet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetGetResponse)
}

func (s *Server) handlePetAdopt(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetAdoptRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Pets.AdoptPet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetAdoptResponse)
}

func (s *Server) handlePetPut(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetPutRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Pets.PutPet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetPutResponse)
}

func (s *Server) handlePetDelete(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetDeleteRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Pets.DeletePet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetDeleteResponse)
}

func (s *Server) handlePetFeed(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetFeedRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	result, err := s.Pets.FeedPet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetFeedResponse)
}

func (s *Server) handlePetWash(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetWashRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	result, err := s.Pets.WashPet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetWashResponse)
}

func (s *Server) handlePetPlay(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsPetPlayRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	if s.Pets == nil {
		return internalError(req.Id, "pet service not configured")
	}
	result, err := s.Pets.PlayPet(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromPetPlayResponse)
}

func (s *Server) handleWalletGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Wallets == nil {
		return internalError(req.Id, "wallet service not configured")
	}
	if _, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsWalletGetRequest); !ok {
		return invalidParams(req.Id)
	}
	owner := s.Caller.String()
	result, err := s.Wallets.GetWallet(ctx, owner)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromWalletGetResponse)
}

func (s *Server) handleWalletTransactionsList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Wallets == nil {
		return internalError(req.Id, "wallet service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsWalletTransactionsListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Wallets.ListTransactions(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromWalletTransactionsListResponse)
}

func (s *Server) handleWalletTransactionsGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Wallets == nil {
		return internalError(req.Id, "wallet service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsWalletTransactionsGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Wallets.GetTransaction(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromWalletTransactionsGetResponse)
}

func (s *Server) handleRewardList(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Rewards == nil {
		return internalError(req.Id, "reward service not configured")
	}
	params, ok := decodeOptionalParams(req, rpcapi.RPCRequest_Params.AsRewardListRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	owner := s.Caller.String()
	result, err := s.Rewards.ListRewards(ctx, owner, params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromRewardListResponse)
}

func (s *Server) handleRewardGet(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Rewards == nil {
		return internalError(req.Id, "reward service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsRewardGetRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Rewards.GetReward(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromRewardGetResponse)
}

func (s *Server) handleRewardClaim(ctx context.Context, req *rpcapi.RPCRequest) *rpcapi.RPCResponse {
	if s.Rewards == nil {
		return internalError(req.Id, "reward service not configured")
	}
	params, ok := decodeRequiredParams(req, rpcapi.RPCRequest_Params.AsRewardClaimRequest)
	if !ok {
		return invalidParams(req.Id)
	}
	result, err := s.Rewards.ClaimReward(ctx, s.Caller.String(), params)
	if err != nil {
		return businessError(req.Id, err)
	}
	return resultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromRewardClaimResponse)
}

func businessError(id string, err error) *rpcapi.RPCResponse {
	if errors.Is(err, kv.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return statusError(id, http.StatusNotFound, "not found")
	}
	return internalError(id, err.Error())
}
