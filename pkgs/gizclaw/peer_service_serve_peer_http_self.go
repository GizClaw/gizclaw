package gizclaw

import (
	"context"
	"errors"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerrun"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func (s *peerHTTP) GetMe(ctx context.Context, _ peerhttp.GetMeRequestObject) (peerhttp.GetMeResponseObject, error) {
	if s == nil || s.Self == nil {
		return peerhttp.GetMe500JSONResponse(apitypes.NewErrorResponse("PEER_HTTP_NOT_CONFIGURED", "peer http self service is not configured")), nil
	}
	result, err := s.Self.GetSelfRegistration(ctx, peerhttp.CallerPublicKey(ctx))
	if err != nil {
		if errors.Is(err, peer.ErrPeerNotFound) {
			return peerhttp.GetMe404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", err.Error())), nil
		}
		return peerhttp.GetMe500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return peerhttp.GetMe200JSONResponse(result), nil
}

func (s *peerHTTP) GetMeStatus(ctx context.Context, _ peerhttp.GetMeStatusRequestObject) (peerhttp.GetMeStatusResponseObject, error) {
	if s == nil || s.Self == nil || s.Status == nil {
		return peerhttp.GetMeStatus500JSONResponse(apitypes.NewErrorResponse("PEER_STATUS_NOT_CONFIGURED", "peer status service is not configured")), nil
	}
	publicKey := peerhttp.CallerPublicKey(ctx)
	if errResponse, ok := s.ensurePeerHTTPCaller(ctx, publicKey); !ok {
		if errors.Is(errResponse, peer.ErrPeerNotFound) {
			return peerhttp.GetMeStatus404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", errResponse.Error())), nil
		}
		return peerhttp.GetMeStatus500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", errResponse.Error())), nil
	}
	status, err := s.Status.GetStatus(ctx, publicKey)
	if err != nil {
		return peerhttp.GetMeStatus500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return peerhttp.GetMeStatus200JSONResponse(status), nil
}

func (s *peerHTTP) PutMeStatus(ctx context.Context, request peerhttp.PutMeStatusRequestObject) (peerhttp.PutMeStatusResponseObject, error) {
	if request.Body == nil {
		return peerhttp.PutMeStatus400JSONResponse(apitypes.NewErrorResponse("INVALID_STATUS", "request body required")), nil
	}
	if s == nil || s.Self == nil || s.Status == nil {
		return peerhttp.PutMeStatus500JSONResponse(apitypes.NewErrorResponse("PEER_STATUS_NOT_CONFIGURED", "peer status service is not configured")), nil
	}
	publicKey := peerhttp.CallerPublicKey(ctx)
	if errResponse, ok := s.ensurePeerHTTPCaller(ctx, publicKey); !ok {
		if errors.Is(errResponse, peer.ErrPeerNotFound) {
			return peerhttp.PutMeStatus404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", errResponse.Error())), nil
		}
		return peerhttp.PutMeStatus500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", errResponse.Error())), nil
	}
	status, err := s.Status.PutStatus(ctx, publicKey, *request.Body)
	if err != nil {
		if errors.Is(err, peerrun.ErrInvalidStatus) {
			return peerhttp.PutMeStatus400JSONResponse(apitypes.NewErrorResponse("INVALID_STATUS", err.Error())), nil
		}
		return peerhttp.PutMeStatus500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", err.Error())), nil
	}
	return peerhttp.PutMeStatus200JSONResponse(status), nil
}

func (s *peerHTTP) GetMeRuntime(ctx context.Context, _ peerhttp.GetMeRuntimeRequestObject) (peerhttp.GetMeRuntimeResponseObject, error) {
	if s == nil || s.Self == nil {
		return peerhttp.GetMeRuntime500JSONResponse(apitypes.NewErrorResponse("PEER_HTTP_NOT_CONFIGURED", "peer http self service is not configured")), nil
	}
	publicKey := peerhttp.CallerPublicKey(ctx)
	if errResponse, ok := s.ensurePeerHTTPCaller(ctx, publicKey); !ok {
		if errors.Is(errResponse, peer.ErrPeerNotFound) {
			return peerhttp.GetMeRuntime404JSONResponse(apitypes.NewErrorResponse("PEER_NOT_FOUND", errResponse.Error())), nil
		}
		return peerhttp.GetMeRuntime500JSONResponse(apitypes.NewErrorResponse("INTERNAL_ERROR", errResponse.Error())), nil
	}
	runtime := s.Self.GetSelfRuntime(ctx, publicKey)
	return peerhttp.GetMeRuntime200JSONResponse(runtime), nil
}

func (s *peerHTTP) ensurePeerHTTPCaller(ctx context.Context, publicKey giznet.PublicKey) (error, bool) {
	_, err := s.Self.GetSelfRegistration(ctx, publicKey)
	if err != nil {
		return err, false
	}
	return nil, true
}
