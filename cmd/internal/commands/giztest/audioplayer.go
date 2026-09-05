package giztestcmd

import (
	"context"
	"encoding/json"
	"fmt"

	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// Script responses at the device SDK boundary; the real server still owns HTTP
// authorization, request validation, reverse RPC and runtime snapshot writes.
func installAudioPlayer(handlers *gizcli.DeviceControlHandlers, method string, response any) error {
	var err error
	switch method {
	case "client.device.audioplayer.get":
		handlers.AudioPlayer.Get, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerGetRequest](response, new(rpcpb.ClientDeviceAudioPlayerGetResponse))
	case "client.device.audioplayer.playlist.get":
		handlers.AudioPlayer.PlaylistGet, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest](response, new(rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse))
	case "client.device.audioplayer.playlist.set":
		handlers.AudioPlayer.PlaylistSet, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerPlaylistSetRequest](response, new(rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse))
	case "client.device.audioplayer.playlist.append":
		handlers.AudioPlayer.PlaylistAppend, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerPlaylistAppendRequest](response, new(rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse))
	case "client.device.audioplayer.play":
		handlers.AudioPlayer.Play, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerPlayRequest](response, new(rpcpb.ClientDeviceAudioPlayerPlayResponse))
	case "client.device.audioplayer.stop":
		handlers.AudioPlayer.Stop, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerStopRequest](response, new(rpcpb.ClientDeviceAudioPlayerStopResponse))
	case "client.device.audioplayer.mode.set":
		handlers.AudioPlayer.ModeSet, err = audioPlayerResponse[*rpcpb.ClientDeviceAudioPlayerModeSetRequest](response, new(rpcpb.ClientDeviceAudioPlayerModeSetResponse))
	default:
		return fmt.Errorf("unsupported audioplayer method %q", method)
	}
	return err
}

func audioPlayerResponse[Q proto.Message, R proto.Message](response any, result R) (func(context.Context, Q) (R, error), error) {
	failure, err := deviceControlErrorResponse(response)
	if err != nil {
		return nil, err
	}
	if failure == nil {
		if result.ProtoReflect().Descriptor().Fields().ByName("value") != nil {
			response = map[string]any{"value": response}
		}
		data, err := json.Marshal(response)
		if err != nil {
			return nil, err
		}
		if err := protojson.Unmarshal(data, result); err != nil {
			return nil, err
		}
	}
	return func(context.Context, Q) (R, error) { return proto.Clone(result).(R), failure }, nil
}
