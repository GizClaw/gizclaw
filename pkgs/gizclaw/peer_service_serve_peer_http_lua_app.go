package gizclaw

import (
	"context"
	"io"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func (s *peerHTTP) InstallLuaApp(ctx context.Context, request peerhttp.InstallLuaAppRequestObject) (peerhttp.InstallLuaAppResponseObject, error) {
	// Include time waiting for the owner command lock in the upload deadline.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return clientToolHTTPResponse{http.StatusUnauthorized, unauthorizedPublicHTTP()}, nil
	}
	contentType, _, typeErr := mime.ParseMediaType(peerHTTPContentType(ctx))
	if typeErr != nil || contentType != "application/octet-stream" {
		return clientToolFailure(invalidDeviceRequest("expected application/octet-stream")), nil
	}
	if request.Params.ContentLength < 1 || request.Params.ContentLength > rpcapi.LuaAppArchiveMaxBytes || request.Body == nil {
		return clientToolFailure(invalidDeviceRequest("invalid archive length")), nil
	}
	metadata := &rpcpb.ClientLuaAppInstallStreamRequest{ContentLength: uint32(request.Params.ContentLength), Sha256: request.Params.Sha256}
	if rpcapi.ValidateLuaAppRequest(metadata) != nil {
		return clientToolFailure(invalidDeviceRequest("invalid archive metadata")), nil
	}
	result, failure := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{timeout: 2 * time.Minute}, func(ctx context.Context, _ *rpcClient, conn net.Conn) (*rpcpb.ClientLuaAppInstallResponse, error) {
		body, ok := request.Body.(io.ReadCloser)
		if !ok {
			body = io.NopCloser(request.Body)
		}
		return rpcapi.UploadLuaApp(ctx, conn, metadata, body)
	}, nil)
	if failure != nil {
		if ctx.Err() != nil {
			failure = mapDeviceControlError(ctx.Err(), ctx, "")
		}
		return clientToolFailure(failure), nil
	}
	return clientToolHTTPResponse{http.StatusOK, result}, nil
}
