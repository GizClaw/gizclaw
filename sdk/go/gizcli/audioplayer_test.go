package gizcli

import (
	"context"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
)

func TestAudioPlayerProviderValidationAndErrors(t *testing.T) {
	device := &Client{}
	calls := 0
	if err := device.HandleDeviceControl(DeviceControlHandlers{AudioPlayer: AudioPlayerHandlers{
		Play: func(_ context.Context, request *rpcpb.ClientDeviceAudioPlayerPlayRequest) (*rpcpb.ClientDeviceAudioPlayerPlayResponse, error) {
			calls++
			if request.GetIndex() != 0 {
				return nil, ErrDeviceRejected
			}
			return &rpcpb.ClientDeviceAudioPlayerPlayResponse{Value: &rpcpb.AudioPlayerStatus{State: "buffering", Repeat: "off", PlaylistLength: 1, CurrentIndex: request.Index}}, nil
		},
	}}); err != nil {
		t.Fatal(err)
	}
	for _, index := range []*uint32{nil, new(uint32(32)), new(uint32(1)), new(uint32(0))} {
		response := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceAudioPlayerPlay, func(payload *rpcapi.RPCPayload) error {
			return payload.FromClientDeviceAudioPlayerPlayRequest(&rpcpb.ClientDeviceAudioPlayerPlayRequest{Index: index})
		})
		if index != nil && *index == 0 {
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
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	response := deviceControlDispatch(t, device, rpcapi.RPCMethodClientDeviceAudioPlayerStop, nil)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeUnimplemented {
		t.Fatalf("unsupported=%+v", response)
	}
}
