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

const mhsHwdNotFoundCode = "MHS_HWD_NOT_FOUND"

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

func (s *peerHTTP) ReadMhsHwd(ctx context.Context, request peerhttp.ReadMhsHwdRequestObject) (peerhttp.ReadMhsHwdResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.ReadMhsHwd401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return readMhsHwdError(invalidDeviceRequest("request body is required")), nil
	}
	manifest, controlErr := s.mhsManifest(ctx, owner)
	if controlErr != nil {
		return readMhsHwdError(controlErr), nil
	}
	params, err := mhs.ReadRequest(manifest, *request.Body)
	if err != nil {
		return readMhsHwdError(invalidDeviceRequest(err.Error())), nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{notFoundCode: mhsHwdNotFoundCode}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientMhsV0ReadResponse, error) {
		return client.ReadMhsHwd(ctx, conn, "client.mhs.v0.read", params)
	}, nil)
	if controlErr != nil {
		return readMhsHwdError(controlErr), nil
	}
	response, err := mhs.ReadResponse(params, result)
	if err != nil {
		return readMhsHwdError(&deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned invalid HWD payload"}), nil
	}
	return peerhttp.ReadMhsHwd200JSONResponse(response), nil
}

func (s *peerHTTP) WriteMhsHwd(ctx context.Context, request peerhttp.WriteMhsHwdRequestObject) (peerhttp.WriteMhsHwdResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.WriteMhsHwd401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Body == nil {
		return writeMhsHwdError(invalidDeviceRequest("request body is required")), nil
	}
	manifest, controlErr := s.mhsManifest(ctx, owner)
	if controlErr != nil {
		return writeMhsHwdError(controlErr), nil
	}
	params, err := mhs.WriteRequest(manifest, *request.Body)
	if err != nil {
		return writeMhsHwdError(invalidDeviceRequest(err.Error())), nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{notFoundCode: mhsHwdNotFoundCode}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientMhsV0WriteResponse, error) {
		return client.WriteMhsHwd(ctx, conn, "client.mhs.v0.write", params)
	}, nil)
	if controlErr != nil {
		return writeMhsHwdError(controlErr), nil
	}
	response, err := mhs.WriteResponse(params, result)
	if err != nil {
		return writeMhsHwdError(&deviceControlError{Status: http.StatusBadGateway, Code: deviceErrorCode, Message: "device returned invalid HWD payload"}), nil
	}
	return peerhttp.WriteMhsHwd200JSONResponse(response), nil
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

func readMhsHwdError(e *deviceControlError) peerhttp.ReadMhsHwdResponseObject {
	body := e.response()
	switch e.Status {
	case 400:
		return peerhttp.ReadMhsHwd400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case 403:
		return peerhttp.ReadMhsHwd403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(body)}
	case 409:
		return peerhttp.ReadMhsHwd409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case 501:
		return peerhttp.ReadMhsHwd501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case 504:
		return peerhttp.ReadMhsHwd504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case 500:
		return peerhttp.ReadMhsHwd500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	case 404:
		return peerhttp.ReadMhsHwd404JSONResponse(body)
	default:
		return peerhttp.ReadMhsHwd502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}

func writeMhsHwdError(e *deviceControlError) peerhttp.WriteMhsHwdResponseObject {
	body := e.response()
	switch e.Status {
	case 400:
		return peerhttp.WriteMhsHwd400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(body)}
	case 403:
		return peerhttp.WriteMhsHwd403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(body)}
	case 409:
		return peerhttp.WriteMhsHwd409JSONResponse{DeviceOfflineJSONResponse: peerhttp.DeviceOfflineJSONResponse(body)}
	case 501:
		return peerhttp.WriteMhsHwd501JSONResponse{DeviceUnsupportedJSONResponse: peerhttp.DeviceUnsupportedJSONResponse(body)}
	case 504:
		return peerhttp.WriteMhsHwd504JSONResponse{DeviceTimeoutJSONResponse: peerhttp.DeviceTimeoutJSONResponse(body)}
	case 500:
		return peerhttp.WriteMhsHwd500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(body)}
	case 404:
		return peerhttp.WriteMhsHwd404JSONResponse(body)
	default:
		return peerhttp.WriteMhsHwd502JSONResponse{DeviceErrorJSONResponse: peerhttp.DeviceErrorJSONResponse(body)}
	}
}
