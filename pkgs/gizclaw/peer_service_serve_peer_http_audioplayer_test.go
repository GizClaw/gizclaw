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
	device := newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		switch tool {
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_GET:
			params, err := req.Params.AsClientDeviceAudioPlayerGetRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerGetResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerGetResponse)
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET:
			params, err := req.Params.AsClientDeviceAudioPlayerPlaylistGetRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerPlaylistGetResponse{Items: []*rpcpb.AudioPlayerItem{item}, PlaylistRevision: 2}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlaylistGetResponse)
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET:
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
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND:
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
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY:
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
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_STOP:
			params, err := req.Params.AsClientDeviceAudioPlayerStopRequest()
			if err != nil {
				return nil, err
			}
			if err := rpcapi.ValidateAudioPlayerRequest(params); err != nil {
				return nil, err
			}
			return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerStopResponse{Value: status}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerStopResponse)
		case rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_MODE_SET:
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
		rpc                rpcpb.ClientTool
	}{
		{"GET", "audioplayer.get", ``, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_GET},
		{"GET", "audioplayer.playlist.get", ``, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET},
		{"PUT", "audioplayer.playlist.set", `{"items":[{"url":"https://media.example/music.mp3","title":"music"}]}`, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_SET},
		{"POST", "audioplayer.playlist.append", `{"items":[{"url":"https://media.example/music.mp3","title":"music"}]}`, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_APPEND},
		{"POST", "audioplayer.play", `{"index":0}`, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAY},
		{"POST", "audioplayer.stop", ``, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_STOP},
		{"PUT", "audioplayer.mode.set", `{"repeat":"all"}`, rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_MODE_SET},
	} {
		t.Run(test.rpc.String(), func(t *testing.T) {
			response := f.invoke(t, test.path, test.body)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if got := <-device.tools; got != test.rpc {
				t.Fatalf("method=%s", got)
			}
			if test.rpc == rpcpb.ClientTool_CLIENT_TOOL_AUDIOPLAYER_PLAYLIST_GET {
				list := decodeToolResult[apitypes.AudioPlayerPlaylist](t, response)
				if len(list.Items) != 1 || list.Items[0].Url != item.Url || list.PlaylistRevision != 2 {
					t.Fatalf("playlist=%+v", list)
				}
			} else {
				player := decodeToolResult[struct {
					Value apitypes.AudioPlayerStatus `json:"value"`
				}](t, response).Value
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
		{"POST", "audioplayer.play", `{}`},
		{"POST", "audioplayer.play", `{"index":-1}`},
		{"POST", "audioplayer.play", `{"index":32}`},
		{"PUT", "audioplayer.playlist.set", `{}`},
		{"PUT", "audioplayer.playlist.set", `{"items":[{"url":"http://example.com/music"}]}`},
		{"PUT", "audioplayer.playlist.set", `{"items":[{"url":"https://user:secret@example.com/music"}]}`},
		{"POST", "audioplayer.playlist.append", `{"items":[]}`},
		{"PUT", "audioplayer.mode.set", `{"repeat":"random"}`},
	} {
		response := f.invoke(t, test.path, test.body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s %s: %d %s", test.path, test.body, response.Code, response.Body.String())
		}
	}
	response := f.invoke(t, "audioplayer.get", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("offline status=%d", response.Code)
	}
}

func TestAudioPlayerHTTPRejectsMalformedDeviceStatus(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	device := newFakeToolConn(func(_ context.Context, tool rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerGetResponse{Value: &rpcpb.AudioPlayerStatus{State: "playing", Repeat: "off", PlaylistLength: 1}}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerGetResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	response := f.invoke(t, "audioplayer.get", "")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	stored := decodeJSON[apitypes.PeerStatus](t, f.do(t, "GET", "/gizclaw/v1/device/status", ""))
	if stored.Audioplayer != nil {
		t.Fatal("malformed status persisted")
	}
}
