package gizclaw

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/gofiber/fiber/v2"
)

// All player routes share the same control error projection and JSON envelope.
type audioPlayerHTTPResponse struct {
	status int
	body   any
}

func (r audioPlayerHTTPResponse) write(w *fiber.Ctx) error {
	return w.Status(r.status).JSON(r.body)
}

func audioPlayerFailure(e *deviceControlError) audioPlayerHTTPResponse {
	return audioPlayerHTTPResponse{status: e.Status, body: e.response()}
}

func invalidAudioPlayerRequest() audioPlayerHTTPResponse {
	return audioPlayerHTTPResponse{status: http.StatusBadRequest, body: apiError(publicHTTPInvalidRequestCode, "invalid audioplayer request")}
}

func audioPlayerOwner(ctx context.Context) (giznet.PublicKey, *audioPlayerHTTPResponse) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		response := audioPlayerHTTPResponse{status: http.StatusUnauthorized, body: unauthorizedPublicHTTP()}
		return owner, &response
	}
	return owner, nil
}

func audioPlayerItems(items []apitypes.AudioPlayerItem) []*rpcpb.AudioPlayerItem {
	result := make([]*rpcpb.AudioPlayerItem, len(items))
	for i, item := range items {
		result[i] = &rpcpb.AudioPlayerItem{Url: item.Url, Title: item.Title, SourceRef: item.SourceRef}
	}
	return result
}

// Store only the player portion under the same owner lock used by telemetry.
func (c *deviceController) applyAudioPlayer(ctx context.Context, owner giznet.PublicKey, wire *rpcpb.AudioPlayerStatus) error {
	if c.status == nil {
		return nil
	}
	at := time.UnixMilli(wire.ObservedAtUnixMs).UTC()
	status := peertelemetry.AudioPlayerStatus(wire)
	mu := c.manager.telemetryStatusLock(owner)
	if mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}
	return (peertelemetry.StatusSync{Store: c.status}).SyncTelemetryStatus(ctx, owner, peertelemetry.StatusPatch{ReportedAt: at, AudioPlayer: &status})
}

func callAudioPlayer[R any](ctx context.Context, c *deviceController, owner giznet.PublicKey, call func(context.Context, *rpcClient, net.Conn) (*R, error), value func(*R) *rpcpb.AudioPlayerStatus) audioPlayerHTTPResponse {
	result, failure := callDeviceControl(ctx, c, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*R, error) {
		response, err := call(ctx, client, conn)
		if err != nil {
			return nil, err
		}
		if response == nil {
			return nil, fmt.Errorf("audioplayer: missing response")
		}
		status := value(response)
		if err := rpcapi.ValidateAudioPlayerStatus(status); err != nil {
			return nil, err
		}
		// A device without a wall clock may omit the observation time.
		if status.ObservedAtUnixMs == 0 {
			status.ObservedAtUnixMs = c.clock().UnixMilli()
		}
		return response, nil
	}, func(ctx context.Context, response *R) error { return c.applyAudioPlayer(ctx, owner, value(response)) })
	if failure != nil {
		return audioPlayerFailure(failure)
	}
	return audioPlayerHTTPResponse{status: http.StatusOK, body: apitypes.AudioPlayerResponse{Status: peertelemetry.AudioPlayerStatus(value(result))}}
}

func (r audioPlayerHTTPResponse) VisitGetDeviceAudioPlayerResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) GetDeviceAudioPlayer(ctx context.Context, request peerhttp.GetDeviceAudioPlayerRequestObject) (peerhttp.GetDeviceAudioPlayerResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	params := new(rpcpb.ClientDeviceAudioPlayerGetRequest)
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	return callAudioPlayer(ctx, s.DeviceControl, owner, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerGetResponse, error) {
		return client.AudioPlayerGet(ctx, conn, params)
	}, (*rpcpb.ClientDeviceAudioPlayerGetResponse).GetValue), nil
}

func (r audioPlayerHTTPResponse) VisitGetDeviceAudioPlayerPlaylistResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) GetDeviceAudioPlayerPlaylist(ctx context.Context, request peerhttp.GetDeviceAudioPlayerPlaylistRequestObject) (peerhttp.GetDeviceAudioPlayerPlaylistResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	params := new(rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest)
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	result, controlErr := callDeviceControl(ctx, s.DeviceControl, owner, deviceControlOptions{}, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse, error) {
		result, err := client.AudioPlayerPlaylistGet(ctx, conn, params)
		if err != nil {
			return nil, err
		}
		if err := rpcapi.ValidateAudioPlayerResponse(result); err != nil {
			return nil, err
		}
		return result, nil
	}, nil)
	if controlErr != nil {
		return audioPlayerFailure(controlErr), nil
	}
	items := make([]apitypes.AudioPlayerItem, len(result.Items))
	for i, item := range result.Items {
		items[i] = apitypes.AudioPlayerItem{Url: item.Url, Title: item.Title, SourceRef: item.SourceRef}
	}
	return audioPlayerHTTPResponse{status: http.StatusOK, body: apitypes.AudioPlayerPlaylist{Items: items, PlaylistRevision: int64(result.PlaylistRevision)}}, nil
}

func (r audioPlayerHTTPResponse) VisitSetDeviceAudioPlayerPlaylistResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) SetDeviceAudioPlayerPlaylist(ctx context.Context, request peerhttp.SetDeviceAudioPlayerPlaylistRequestObject) (peerhttp.SetDeviceAudioPlayerPlaylistResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	if request.Body == nil || request.Body.Items == nil {
		return invalidAudioPlayerRequest(), nil
	}
	params := &rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest{Items: audioPlayerItems(request.Body.Items)}
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	return callAudioPlayer(ctx, s.DeviceControl, owner, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse, error) {
		return client.AudioPlayerPlaylistSet(ctx, conn, params)
	}, (*rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse).GetValue), nil
}

func (r audioPlayerHTTPResponse) VisitAppendDeviceAudioPlayerPlaylistResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) AppendDeviceAudioPlayerPlaylist(ctx context.Context, request peerhttp.AppendDeviceAudioPlayerPlaylistRequestObject) (peerhttp.AppendDeviceAudioPlayerPlaylistResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	if request.Body == nil {
		return invalidAudioPlayerRequest(), nil
	}
	params := &rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest{Items: audioPlayerItems(request.Body.Items)}
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	return callAudioPlayer(ctx, s.DeviceControl, owner, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse, error) {
		return client.AudioPlayerPlaylistAppend(ctx, conn, params)
	}, (*rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse).GetValue), nil
}

func (r audioPlayerHTTPResponse) VisitPlayDeviceAudioPlayerResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) PlayDeviceAudioPlayer(ctx context.Context, request peerhttp.PlayDeviceAudioPlayerRequestObject) (peerhttp.PlayDeviceAudioPlayerResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	if request.Body == nil {
		return invalidAudioPlayerRequest(), nil
	}
	if request.Body.Index == nil || *request.Body.Index < 0 || *request.Body.Index >= rpcapi.MaxAudioPlayerItems {
		return invalidAudioPlayerRequest(), nil
	}
	params := &rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: new(uint32(*request.Body.Index))}
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	return callAudioPlayer(ctx, s.DeviceControl, owner, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerPlayResponse, error) {
		return client.AudioPlayerPlay(ctx, conn, params)
	}, (*rpcpb.ClientDeviceAudioPlayerPlayResponse).GetValue), nil
}

func (r audioPlayerHTTPResponse) VisitStopDeviceAudioPlayerResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) StopDeviceAudioPlayer(ctx context.Context, request peerhttp.StopDeviceAudioPlayerRequestObject) (peerhttp.StopDeviceAudioPlayerResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	params := new(rpcpb.ClientDeviceAudioPlayerStopRequest)
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	return callAudioPlayer(ctx, s.DeviceControl, owner, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerStopResponse, error) {
		return client.AudioPlayerStop(ctx, conn, params)
	}, (*rpcpb.ClientDeviceAudioPlayerStopResponse).GetValue), nil
}

func (r audioPlayerHTTPResponse) VisitSetDeviceAudioPlayerModeResponse(w *fiber.Ctx) error {
	return r.write(w)
}

func (s *peerHTTP) SetDeviceAudioPlayerMode(ctx context.Context, request peerhttp.SetDeviceAudioPlayerModeRequestObject) (peerhttp.SetDeviceAudioPlayerModeResponseObject, error) {
	owner, failure := audioPlayerOwner(ctx)
	if failure != nil {
		return *failure, nil
	}
	if request.Body == nil {
		return invalidAudioPlayerRequest(), nil
	}
	params := &rpcpb.ClientDeviceAudioPlayerModeSetRequest{Repeat: string(request.Body.Repeat)}
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return invalidAudioPlayerRequest(), nil
	}
	return callAudioPlayer(ctx, s.DeviceControl, owner, func(ctx context.Context, client *rpcClient, conn net.Conn) (*rpcpb.ClientDeviceAudioPlayerModeSetResponse, error) {
		return client.AudioPlayerModeSet(ctx, conn, params)
	}, (*rpcpb.ClientDeviceAudioPlayerModeSetResponse).GetValue), nil
}
