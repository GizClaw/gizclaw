package giztestcmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// audioFixture owns one test Peer's actual playlist and playback state. It
// returns protocol-selected protobuf messages through the ordinary SDK handler.
type audioFixture struct {
	mu     sync.Mutex
	items  []*rpcpb.AudioPlayerItem
	status *rpcpb.AudioPlayerStatus
}

func newAudioFixture(value any) (*audioFixture, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var config struct {
		Items        []json.RawMessage `json:"items"`
		CurrentIndex *uint32           `json:"current_index"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	fixture := &audioFixture{status: &rpcpb.AudioPlayerStatus{State: "stopped", Repeat: "off", CurrentIndex: config.CurrentIndex, PlaylistRevision: 1}}
	for _, data := range config.Items {
		item := new(rpcpb.AudioPlayerItem)
		if err := protojson.Unmarshal(data, item); err != nil {
			return nil, err
		}
		fixture.items = append(fixture.items, item)
	}
	fixture.status.PlaylistLength = uint32(len(fixture.items))
	return fixture, nil
}

func (f *audioFixture) invoke(ctx context.Context, request proto.Message) (proto.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch request := request.(type) {
	case *rpcpb.ClientDeviceAudioPlayerPlaylistGetRequest:
		result := &rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse{PlaylistRevision: f.status.PlaylistRevision}
		for _, item := range f.items {
			result.Items = append(result.Items, proto.CloneOf(item))
		}
		return result, nil
	case *rpcpb.ClientDeviceAudioPlayerGetRequest:
		return &rpcpb.ClientDeviceAudioPlayerGetResponse{Value: proto.CloneOf(f.status)}, nil
	case *rpcpb.ClientDeviceAudioPlayerPlayRequest:
		index := uint32(0)
		if request.Index != nil {
			index = *request.Index
		}
		if int(index) >= len(f.items) {
			return nil, rpcapi.Error{Code: rpcapi.StatusCodeOutOfRange, Message: "track index out of range"}
		}
		f.status.CurrentIndex = new(index)
		f.status.PositionMs = 0
		f.status.State = "playing"
		return &rpcpb.ClientDeviceAudioPlayerPlayResponse{Value: proto.CloneOf(f.status)}, nil
	case *rpcpb.ClientDeviceAudioPlayerStopRequest:
		f.status.State = "stopped"
		return &rpcpb.ClientDeviceAudioPlayerStopResponse{Value: proto.CloneOf(f.status)}, nil
	case *rpcpb.ClientDeviceAudioPlayerModeSetRequest:
		f.status.Repeat = request.Repeat
		return &rpcpb.ClientDeviceAudioPlayerModeSetResponse{Value: proto.CloneOf(f.status)}, nil
	default:
		return nil, fmt.Errorf("unsupported stateful audio procedure %s", request.ProtoReflect().Descriptor().Name())
	}
}
