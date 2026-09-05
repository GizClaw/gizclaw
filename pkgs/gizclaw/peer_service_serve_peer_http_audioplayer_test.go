package gizclaw

import (
	"context"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"net/http"
	"testing"
	"time"
)

func TestAudioPlayerHTTPRoundTrip(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	at := time.Now().UnixMilli()
	status := &rpcpb.AudioPlayerStatus{State: "playing", CurrentIndex: new(uint32(0)), PositionMs: 1200, Repeat: "all", PlaylistLength: 1, PlaylistRevision: 2, ObservedAtUnixMs: at}
	item := &rpcpb.AudioPlayerItem{Url: "https://media.example/music.mp3", Title: new("music")}
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch req.Method {
		case rpcapi.RPCMethodClientDeviceAudioPlayerGet:
			params, err := req.Params.AsClientDeviceAudioPlayerGetRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerGetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerGetResponse)
		case rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistGet:
			params, err := req.Params.AsClientDeviceAudioPlayerPlaylistGetRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse{Items: []*rpcpb.AudioPlayerItem{item}, PlaylistRevision: 2}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistGetResponse)
		case rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistSet:
			params, err := req.Params.AsClientDeviceAudioPlayerPlaylistSetRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			if len(params.Items) != 1 || params.Items[0].Url != item.Url {
				t.Error("playlist payload changed")
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerPlaylistSetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistSetResponse)
		case rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistAppend:
			params, err := req.Params.AsClientDeviceAudioPlayerPlaylistAppendRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			if len(params.Items) != 1 || params.Items[0].Url != item.Url {
				t.Error("playlist payload changed")
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerPlaylistAppendResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistAppendResponse)
		case rpcapi.RPCMethodClientDeviceAudioPlayerPlay:
			params, err := req.Params.AsClientDeviceAudioPlayerPlayRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			if params.Index == nil || *params.Index != 0 {
				t.Error("index zero was lost")
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerPlayResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlayResponse)
		case rpcapi.RPCMethodClientDeviceAudioPlayerStop:
			params, err := req.Params.AsClientDeviceAudioPlayerStopRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerStopResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerStopResponse)
		case rpcapi.RPCMethodClientDeviceAudioPlayerModeSet:
			params, err := req.Params.AsClientDeviceAudioPlayerModeSetRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			if params.Repeat != "all" {
				t.Error("repeat changed")
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerModeSetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerModeSetResponse)
		default:
			t.Errorf("unexpected method %s", req.Method)
			return nil, nil
		}
	})
	f.manager.SetPeerUp(f.owner, device)
	for _, test := range []struct {
		method, path, body string
		rpc                rpcapi.RPCMethod
	}{
		{"GET", "/gizclaw/v1/device/audioplayer", ``, rpcapi.RPCMethodClientDeviceAudioPlayerGet},
		{"GET", "/gizclaw/v1/device/audioplayer/playlist", ``, rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistGet},
		{"PUT", "/gizclaw/v1/device/audioplayer/playlist", `{"items":[{"url":"https://media.example/music.mp3","title":"music"}]}`, rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistSet},
		{"POST", "/gizclaw/v1/device/audioplayer/playlist/append", `{"items":[{"url":"https://media.example/music.mp3","title":"music"}]}`, rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistAppend},
		{"POST", "/gizclaw/v1/device/audioplayer/actions/play", `{"index":0}`, rpcapi.RPCMethodClientDeviceAudioPlayerPlay},
		{"POST", "/gizclaw/v1/device/audioplayer/actions/stop", ``, rpcapi.RPCMethodClientDeviceAudioPlayerStop},
		{"PUT", "/gizclaw/v1/device/audioplayer/mode", `{"repeat":"all"}`, rpcapi.RPCMethodClientDeviceAudioPlayerModeSet},
	} {
		t.Run(string(test.rpc), func(t *testing.T) {
			response := f.do(t, test.method, test.path, test.body)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if got := <-device.methods; got != test.rpc {
				t.Fatalf("method=%s", got)
			}
			if test.rpc == rpcapi.RPCMethodClientDeviceAudioPlayerPlaylistGet {
				list := decodeJSON[apitypes.AudioPlayerPlaylist](t, response)
				if len(list.Items) != 1 || list.Items[0].Url != item.Url || list.PlaylistRevision != 2 {
					t.Fatalf("playlist=%+v", list)
				}
			} else {
				player := decodeJSON[apitypes.AudioPlayerResponse](t, response).Status
				if player.State != "playing" || player.PositionMs != 1200 || player.CurrentIndex == nil || *player.CurrentIndex != 0 {
					t.Fatalf("status=%+v", player)
				}
			}
		})
	}
	stored := decodeJSON[apitypes.PeerStatus](t, f.do(t, "GET", "/gizclaw/v1/device/status", ""))
	if stored.Audioplayer == nil || stored.Audioplayer.PositionMs != 1200 || stored.Audioplayer.ObservedAtUnixMs != at {
		t.Fatalf("snapshot=%+v", stored)
	}
	if device.calls.Load() != 7 {
		t.Fatal("snapshot read contacted device")
	}
}

func TestAudioPlayerHTTPRejectsInvalidRequests(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	for _, test := range []struct{ method, path, body string }{
		{"POST", "/actions/play", `{}`},
		{"POST", "/actions/play", `{"index":-1}`},
		{"POST", "/actions/play", `{"index":32}`},
		{"PUT", "/playlist", `{}`},
		{"PUT", "/playlist", `{"items":[{"url":"http://example.com/music"}]}`},
		{"PUT", "/playlist", `{"items":[{"url":"https://user:secret@example.com/music"}]}`},
		{"POST", "/playlist/append", `{"items":[]}`},
		{"PUT", "/mode", `{"repeat":"random"}`},
	} {
		response := f.do(t, test.method, "/gizclaw/v1/device/audioplayer"+test.path, test.body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s: %d %s", test.path, test.body, response.Code, response.Body.String())
		}
	}
	response := f.do(t, "GET", "/gizclaw/v1/device/audioplayer", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("offline status=%d", response.Code)
	}
}

func TestAudioPlayerHTTPRejectsMalformedDeviceStatus(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	device := newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerGetResponse{Value: &rpcpb.AudioPlayerStatus{State: "playing", Repeat: "off", PlaylistLength: 1}}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerGetResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	response := f.do(t, "GET", "/gizclaw/v1/device/audioplayer", "")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	stored := decodeJSON[apitypes.PeerStatus](t, f.do(t, "GET", "/gizclaw/v1/device/status", ""))
	if stored.Audioplayer != nil {
		t.Fatal("malformed status persisted")
	}
}
