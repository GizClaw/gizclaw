package gizclaw

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/gearservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

type rpcServer struct {
	gear            gearservice.StrictServerInterface
	callerPublicKey giznet.PublicKey
}

func (s *rpcServer) Ping(ctx context.Context, conn net.Conn, id string) (*rpcapi.PingResponse, error) {
	return callRPCPing(ctx, conn, id)
}

func (s *rpcServer) Handle(conn net.Conn) error {
	return handleRPC(conn, s.dispatch)
}

func (s *rpcServer) dispatch(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req == nil {
		return rpcapi.Error{Code: rpcapi.RPCErrorCodeInvalidRequest, Message: "nil request"}.RPCResponse(), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodPeerPing:
		return handleRPCPing(ctx, req)
	case rpcapi.RPCMethodGearConfigGet:
		return s.handleGetConfig(ctx, req)
	case rpcapi.RPCMethodGearInfoGet:
		return s.handleGetInfo(ctx, req)
	case rpcapi.RPCMethodGearInfoPut:
		return s.handlePutInfo(ctx, req)
	case rpcapi.RPCMethodGearOtaGet:
		return s.handleGetOTA(ctx, req)
	case rpcapi.RPCMethodGearRegistrationGet:
		return s.handleGetRegistration(ctx, req)
	case rpcapi.RPCMethodGearRegistrationRegister:
		return s.handleRegisterGear(ctx, req)
	case rpcapi.RPCMethodGearRuntimeGet:
		return s.handleGetRuntime(ctx, req)
	default:
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeMethodNotFound, Message: fmt.Sprintf("unknown method: %s", req.Method)}.RPCResponse(), nil
	}
}

func (s *rpcServer) handleGetConfig(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsGearGetConfigRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	resp, err := s.gear.GetConfig(s.gearContext(ctx), gearservice.GetConfigRequestObject{})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.GetConfig200JSONResponse:
		result, err := convertRPCType[rpcapi.GearGetConfigResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearGetConfigResponse)
	case gearservice.GetConfig404JSONResponse:
		return rpcAPIError(req.Id, http.StatusNotFound, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) handleGetInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsGearGetInfoRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	resp, err := s.gear.GetInfo(s.gearContext(ctx), gearservice.GetInfoRequestObject{})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.GetInfo200JSONResponse:
		result, err := convertRPCType[rpcapi.GearGetInfoResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearGetInfoResponse)
	case gearservice.GetInfo404JSONResponse:
		return rpcAPIError(req.Id, http.StatusNotFound, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) handlePutInfo(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsGearPutInfoRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	body, err := convertRPCType[gearservice.PutInfoJSONRequestBody](params)
	if err != nil {
		return nil, err
	}
	resp, err := s.gear.PutInfo(s.gearContext(ctx), gearservice.PutInfoRequestObject{Body: &body})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.PutInfo200JSONResponse:
		result, err := convertRPCType[rpcapi.GearPutInfoResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearPutInfoResponse)
	case gearservice.PutInfo400JSONResponse:
		return rpcAPIError(req.Id, http.StatusBadRequest, apitypes.ErrorResponse(body)), nil
	case gearservice.PutInfo404JSONResponse:
		return rpcAPIError(req.Id, http.StatusNotFound, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) handleGetOTA(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsGearGetOTARequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	resp, err := s.gear.GetOTA(s.gearContext(ctx), gearservice.GetOTARequestObject{})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.GetOTA200JSONResponse:
		result, err := convertRPCType[rpcapi.GearGetOTAResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearGetOTAResponse)
	case gearservice.GetOTA404JSONResponse:
		return rpcAPIError(req.Id, http.StatusNotFound, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) handleGetRegistration(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsGearGetRegistrationRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	resp, err := s.gear.GetRegistration(s.gearContext(ctx), gearservice.GetRegistrationRequestObject{})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.GetRegistration200JSONResponse:
		result, err := convertRPCType[rpcapi.GearGetRegistrationResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearGetRegistrationResponse)
	case gearservice.GetRegistration404JSONResponse:
		return rpcAPIError(req.Id, http.StatusNotFound, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) handleRegisterGear(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		return rpcapi.Error{RequestID: req.Id, Code: rpcapi.RPCErrorCodeInvalidParams, Message: "missing params"}.RPCResponse(), nil
	}
	params, err := req.Params.AsGearRegisterRequest()
	if err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	body, err := convertRPCType[gearservice.RegisterGearJSONRequestBody](params)
	if err != nil {
		return nil, err
	}
	resp, err := s.gear.RegisterGear(s.gearContext(ctx), gearservice.RegisterGearRequestObject{Body: &body})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.RegisterGear200JSONResponse:
		result, err := convertRPCType[rpcapi.GearRegisterResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearRegisterResponse)
	case gearservice.RegisterGear400JSONResponse:
		return rpcAPIError(req.Id, http.StatusBadRequest, apitypes.ErrorResponse(body)), nil
	case gearservice.RegisterGear409JSONResponse:
		return rpcAPIError(req.Id, http.StatusConflict, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) handleGetRuntime(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := validateRPCParams(req.Params, rpcapi.RPCRequest_Params.AsGearGetRuntimeRequest); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	resp, err := s.gear.GetRuntime(s.gearContext(ctx), gearservice.GetRuntimeRequestObject{})
	if err != nil {
		return nil, err
	}
	switch body := resp.(type) {
	case gearservice.GetRuntime200JSONResponse:
		result, err := convertRPCType[rpcapi.GearGetRuntimeResponse](body)
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(req.Id, result, (*rpcapi.RPCResponse_Result).FromGearGetRuntimeResponse)
	case gearservice.GetRuntime400JSONResponse:
		return rpcAPIError(req.Id, http.StatusBadRequest, apitypes.ErrorResponse(body)), nil
	default:
		return rpcUnexpectedResponse(req.Id, resp), nil
	}
}

func (s *rpcServer) gearContext(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return gearservice.WithCallerPublicKey(ctx, s.callerPublicKey)
}
