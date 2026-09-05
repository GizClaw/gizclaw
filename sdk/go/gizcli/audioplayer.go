package gizcli

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

// AudioPlayerHandlers controls the device's single player. Playlist mutations
// must be atomic; append preserves playback and must not be retried implicitly.
// Nil handlers answer UNIMPLEMENTED. A successful play acknowledges acceptance;
// actual playback state is reported through telemetry.
type AudioPlayerHandlers struct {
	Get            func(context.Context, *rpcpb.ClientDeviceAudioPlayerGetRequest) (*rpcpb.ClientDeviceAudioPlayerGetResponse, error)
	PlaylistGet    func(context.Context, *rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest) (*rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse, error)
	PlaylistSet    func(context.Context, *rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest) (*rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse, error)
	PlaylistAppend func(context.Context, *rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest) (*rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse, error)
	Play           func(context.Context, *rpcpb.ClientDeviceAudioPlayerPlayRequest) (*rpcpb.ClientDeviceAudioPlayerPlayResponse, error)
	Stop           func(context.Context, *rpcpb.ClientDeviceAudioPlayerStopRequest) (*rpcpb.ClientDeviceAudioPlayerStopResponse, error)
	ModeSet        func(context.Context, *rpcpb.ClientDeviceAudioPlayerModeSetRequest) (*rpcpb.ClientDeviceAudioPlayerModeSetResponse, error)
}

func (c *rpcClient) handleAudioPlayer(ctx context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	handlers := c.peer.deviceControlHandlers()
	if handlers == nil {
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
	switch req.Method {
	case rpcapi.RPCMethodClientDeviceAudioPlayerGet:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerGetRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerGetRequest, handlers.AudioPlayer.Get, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerGetResponse)
	case rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistGet:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlaylistGetRequest, handlers.AudioPlayer.PlaylistGet, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistGetResponse)
	case rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistSet:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlaylistSetRequest, handlers.AudioPlayer.PlaylistSet, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistSetResponse)
	case rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistAppend:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlaylistAppendRequest, handlers.AudioPlayer.PlaylistAppend, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistAppendResponse)
	case rpcapi.RPCMethodClientDeviceAudioPlayerPlay:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerPlayRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlayRequest, handlers.AudioPlayer.Play, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlayResponse)
	case rpcapi.RPCMethodClientDeviceAudioPlayerStop:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerStopRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerStopRequest, handlers.AudioPlayer.Stop, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerStopResponse)
	case rpcapi.RPCMethodClientDeviceAudioPlayerModeSet:
		return dispatchAudioPlayer(ctx, c, req, new(rpcpb.ClientDeviceAudioPlayerModeSetRequest), rpcapi.RPCPayload.AsClientDeviceAudioPlayerModeSetRequest, handlers.AudioPlayer.ModeSet, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerModeSetResponse)
	default:
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
}

func dispatchAudioPlayer[Q proto.Message, R proto.Message](ctx context.Context, c *rpcClient, req *rpcapi.RPCRequest, params Q, decode func(rpcapi.RPCPayload) (Q, error), handler func(context.Context, Q) (R, error), encode func(*rpcapi.RPCPayload, R) error) (*rpcapi.RPCResponse, error) {
	if req.Params == nil {
		switch any(params).(type) {
		case *rpcpb.ClientDeviceAudioPlayerGetRequest, *rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest, *rpcpb.ClientDeviceAudioPlayerStopRequest:
		default:
			return rpcInvalidParams(req.Id), nil
		}
	}
	if req.Params != nil {
		decoded, err := decode(*req.Params)
		if err != nil {
			return rpcInvalidParams(req.Id), nil
		}
		params = decoded
	}
	if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
		return rpcInvalidParams(req.Id), nil
	}
	if handler == nil {
		return deviceControlUnsupported(req.Id, req.Method), nil
	}
	c.peer.observeClientRPC(req.Method)
	result, err := handler(ctx, params)
	if err != nil {
		return deviceControlError(req.Id, err), nil
	}
	if err := rpcapi.ValidateAudioPlayerResponse(result); err != nil {
		return deviceControlError(req.Id, err), nil
	}
	return newRPCResultResponse(req.Id, result, encode)
}
