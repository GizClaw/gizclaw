package gizclaw

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"net"
)

func (c *rpcClient) AudioPlayerGet(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerGetRequest) (*rpcpb.ClientDeviceAudioPlayerGetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerGetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerGet, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerGetResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *rpcClient) AudioPlayerPlaylistGet(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest) (*rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistGetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistGet, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlaylistGetResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *rpcClient) AudioPlayerPlaylistSet(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest) (*rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistSetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistSet, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlaylistSetResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *rpcClient) AudioPlayerPlaylistAppend(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest) (*rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistAppendRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistAppend, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlaylistAppendResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *rpcClient) AudioPlayerPlay(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerPlayRequest) (*rpcpb.ClientDeviceAudioPlayerPlayResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlayRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerPlay, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerPlayResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *rpcClient) AudioPlayerStop(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerStopRequest) (*rpcpb.ClientDeviceAudioPlayerStopResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerStopRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerStop, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerStopResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *rpcClient) AudioPlayerModeSet(ctx context.Context, conn net.Conn, request *rpcpb.ClientDeviceAudioPlayerModeSetRequest) (*rpcpb.ClientDeviceAudioPlayerModeSetResponse, error) {
	params, err := newRPCRequestParams(request, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerModeSetRequest)
	if err != nil {
		return nil, err
	}
	result, err := callRPCResult(ctx, conn, newRPCRequest("audioplayer", rpcapi.RPCMethodClientDeviceAudioPlayerModeSet, params), rpcapi.RPCPayload.AsClientDeviceAudioPlayerModeSetResponse)
	if err != nil {
		return nil, err
	}
	return *result, nil
}
