package gizclaw

import (
	"context"
	"errors"
	"net"
	"net/http"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/device/mhs"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

const mhsStateNotFoundCode = "MHS_STATE_NOT_FOUND"

func (s *peerHTTP) mhsManifest(ctx context.Context, owner giznet.PublicKey) (apitypes.MhsV0Manifest, *deviceControlError) {
	reads, ok := s.deviceReads(owner)
	if !ok {
		return apitypes.MhsV0Manifest{}, internalDeviceControlError()
	}
	manifest, err := reads.MhsManifest(ctx)
	if errors.Is(err, peerresource.ErrDeviceRuntimeProfileNotBound) {
		return apitypes.MhsV0Manifest{}, &deviceControlError{Status: http.StatusForbidden, Code: "API_KEY_OWNER_UNAVAILABLE", Message: http.StatusText(http.StatusForbidden)}
	}
	if err != nil {
		return apitypes.MhsV0Manifest{}, internalDeviceControlError()
	}
	return manifest, nil
}

func (s *peerHTTP) GetMhsManifest(ctx context.Context, _ peerhttp.GetMhsManifestRequestObject) (peerhttp.GetMhsManifestResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.GetMhsManifest401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	manifest, controlErr := s.mhsManifest(ctx, owner)
	if controlErr != nil {
		return getMhsManifestError(controlErr), nil
	}
	return peerhttp.GetMhsManifest200JSONResponse(manifest), nil
}

func (s *peerHTTP) ReadMhsStates(ctx context.Context, request peerhttp.ReadMhsStatesRequestObject) (peerhttp.ReadMhsStatesResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ReadMhsStates401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return readMhsStatesError(invalidDeviceRequest("request body is required")), nil
	}
	manifest, controlErr := s.mhsManifest(ctx, owner)
	if controlErr != nil {
		return readMhsStatesError(controlErr), nil
	}
	params, err := mhs.ReadRequest(manifest, *request.Body)
	if err != nil {
		return readMhsStatesError(invalidDeviceRequest(err.Error())), nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{notFoundCode: mhsStateNotFoundCode}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientMhsV0ReadResponse, error) {
		return client.ReadMhsStates(ctx, conn, "client.mhs.v0.read", params)
	}, nil)
	if controlErr != nil {
		return readMhsStatesError(controlErr), nil
	}
	refs := params.States
	response, err := mhs.Response(manifest, refs, result.GetStates())
	if err != nil {
		return readMhsStatesError(&deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned invalid MHS states"}), nil
	}
	return peerhttp.ReadMhsStates200JSONResponse(response), nil
}

func (s *peerHTTP) WriteMhsStates(ctx context.Context, request peerhttp.WriteMhsStatesRequestObject) (peerhttp.WriteMhsStatesResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.WriteMhsStates401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return writeMhsStatesError(invalidDeviceRequest("request body is required")), nil
	}
	manifest, controlErr := s.mhsManifest(ctx, owner)
	if controlErr != nil {
		return writeMhsStatesError(controlErr), nil
	}
	params, err := mhs.WriteRequest(manifest, *request.Body)
	if err != nil {
		return writeMhsStatesError(invalidDeviceRequest(err.Error())), nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{notFoundCode: mhsStateNotFoundCode}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientMhsV0WriteResponse, error) {
		return client.WriteMhsStates(ctx, conn, "client.mhs.v0.write", params)
	}, nil)
	if controlErr != nil {
		return writeMhsStatesError(controlErr), nil
	}
	refs := make([]*rpcpb.MhsStateRef, len(params.States))
	for i, state := range params.States {
		refs[i] = &rpcpb.MhsStateRef{DeviceId: state.DeviceId, State: state.State}
	}
	response, err := mhs.Response(manifest, refs, result.GetStates())
	if err != nil {
		return writeMhsStatesError(&deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned invalid MHS states"}), nil
	}
	return peerhttp.WriteMhsStates200JSONResponse(response), nil
}

func getMhsManifestError(e *deviceControlError) peerhttp.GetMhsManifestResponseObject {
	body := e.response()
	switch e.Status {
	case 400:
		return peerhttp.GetMhsManifest400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case 403:
		return peerhttp.GetMhsManifest403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(body)}
	case 409:
		return peerhttp.GetMhsManifest409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}
	default:
		return peerhttp.GetMhsManifest500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	}
}

func readMhsStatesError(e *deviceControlError) peerhttp.ReadMhsStatesResponseObject {
	body := e.response()
	switch e.Status {
	case 400:
		return peerhttp.ReadMhsStates400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case 403:
		return peerhttp.ReadMhsStates403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(body)}
	case 409:
		return peerhttp.ReadMhsStates409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case 501:
		return peerhttp.ReadMhsStates501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case 504:
		return peerhttp.ReadMhsStates504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case 500:
		return peerhttp.ReadMhsStates500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	case 404:
		return peerhttp.ReadMhsStates404JSONResponse(body)
	default:
		return peerhttp.ReadMhsStates502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func writeMhsStatesError(e *deviceControlError) peerhttp.WriteMhsStatesResponseObject {
	body := e.response()
	switch e.Status {
	case 400:
		return peerhttp.WriteMhsStates400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case 403:
		return peerhttp.WriteMhsStates403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(body)}
	case 409:
		return peerhttp.WriteMhsStates409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case 501:
		return peerhttp.WriteMhsStates501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case 504:
		return peerhttp.WriteMhsStates504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case 500:
		return peerhttp.WriteMhsStates500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	case 404:
		return peerhttp.WriteMhsStates404JSONResponse(body)
	default:
		return peerhttp.WriteMhsStates502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}
