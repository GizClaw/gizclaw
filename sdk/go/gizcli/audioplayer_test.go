package gizcli

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestGenericAudioPlayerHandlerAcceptsChangingIndex(t *testing.T) {
	device := &Client{}
	var indices []uint32
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY, func(_ context.Context, message proto.Message) (proto.Message, error) {
		request := message.(*rpcpb.ClientDeviceAudioPlayerPlayRequest)
		indices = append(indices, request.GetIndex())
		return &rpcpb.ClientDeviceAudioPlayerPlayResponse{Value: &rpcpb.AudioPlayerStatus{State: "playing", Repeat: "off", PlaylistLength: 3, CurrentIndex: new(request.GetIndex())}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, index := range []uint32{0, 1, 0} {
		response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY, func(payload *rpcapi.RPCPayload) error {
			return payload.FromClientDeviceAudioPlayerPlayRequest(&rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: &index})
		})
		if response.Error != nil {
			t.Fatalf("index %d: %v", index, response.Error)
		}
	}
	if len(indices) != 3 || indices[1] != 1 {
		t.Fatalf("received indices %v", indices)
	}
}

func TestAudioPlayerProviderValidationAndErrors(t *testing.T) {
	device := &Client{}
	calls := 0
	if err := device.HandleDeviceControl(DeviceControlHandlers{AudioPlayer: AudioPlayerHandlers{
		Play: func(_ context.Context, request *rpcpb.ClientDeviceAudioPlayerPlayRequest) (*rpcpb.ClientDeviceAudioPlayerPlayResponse, error) {
			calls++
			if request.GetIndex() != 0 {
				return nil, ErrDeviceRejected
			}
			return &rpcpb.ClientDeviceAudioPlayerPlayResponse{Value: &rpcpb.AudioPlayerStatus{State: "buffering", Repeat: "off", PlaylistLength: 1, CurrentIndex: new(request.GetIndex())}}, nil
		},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, index := range []*uint32{nil, new(uint32(32)), new(uint32(1)), new(uint32(0))} {
		response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY, func(payload *rpcapi.RPCPayload) error {
			return payload.FromClientDeviceAudioPlayerPlayRequest(&rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: index})
		})
		if index == nil || *index == 0 {
			if response.Error != nil {
				t.Fatal(response.Error)
			}
			result, err := response.Result.AsClientDeviceAudioPlayerPlayResponse()
			if err != nil || result.Value.State != "buffering" || result.Value.CurrentIndex == nil {
				t.Fatalf("result=%v err=%v", result, err)
			}
		} else if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument {
			t.Fatalf("response=%+v", response)
		}
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
	response := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_STOP, nil)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("unsupported=%+v", response)
	}
}
