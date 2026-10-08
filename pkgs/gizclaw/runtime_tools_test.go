package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/toolcatalog"
	"google.golang.org/protobuf/proto"
)

// runtimeQueueContext observes Acquire's wait without sleeps or a production
// hook. Invoke's typed preparation does not inspect Done before that wait.
type runtimeQueueContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func TestRuntimeToolPlaybackCanChangeNonzeroIndex(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	profile, err := f.profiles.ResolveOwnerProfile(t.Context(), f.owner.String())
	if err != nil {
		t.Fatal(err)
	}
	binding := apitypes.RuntimeProfileToolBinding{ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.play"}, I18n: map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Play"}, "zh-CN": {DisplayName: "播放"}}}
	profile.Spec.Resources.Tools = &map[string]apitypes.RuntimeProfileToolBinding{"play": binding}
	response, err := f.profiles.PutRuntimeProfile(t.Context(), adminhttp.PutRuntimeProfileRequestObject{Id: profile.Id, Body: &adminhttp.RuntimeProfileUpsert{Id: profile.Id, Spec: profile.Spec}})
	if err != nil {
		t.Fatal(err)
	}
	updated, ok := response.(adminhttp.PutRuntimeProfile200JSONResponse)
	if !ok {
		t.Fatalf("Profile update: %T", response)
	}
	profile = apitypes.RuntimeProfile(updated)
	device := newFakeToolConn(func(_ context.Context, _ rpcpb.ClientTool, request *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
		params, err := request.Params.AsClientDeviceAudioPlayerPlayRequest()
		if err != nil {
			return nil, err
		}
		return newRPCResultResponse(request.Id, &rpcpb.ClientDeviceAudioPlayerPlayResponse{Value: &rpcpb.AudioPlayerStatus{State: "playing", Repeat: "off", PlaylistLength: 3, CurrentIndex: params.Index, PlaylistRevision: 1}}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerPlayResponse)
	})
	f.manager.SetPeerUp(f.owner, device)
	source, target, schema, err := toolcatalog.BindingSchema(profile, binding)
	if err != nil {
		t.Fatal(err)
	}
	tool := toolcatalog.Tool{Alias: "play", Binding: binding, Source: source, Target: target, Schema: schema}
	for _, index := range []int{0, 1, 0} {
		_, err := (&runtimeDevices{public: f.public}).Invoke(t.Context(), f.owner.String(), profile, tool, json.RawMessage(fmt.Sprintf(`{"index":%d}`, index)))
		if err != nil {
			t.Fatalf("index %d: %v", index, err)
		}
	}
	if device.calls.Load() != 3 {
		t.Fatalf("device calls %d", device.calls.Load())
	}
}

func (ctx *runtimeQueueContext) Done() <-chan struct{} {
	ctx.once.Do(func() { close(ctx.waiting) })
	return ctx.Context.Done()
}

func TestRuntimeToolQueueReauthorizesBeforeDeviceRequest(t *testing.T) {
	for _, source := range []string{"mhs", "client_tool"} {
		for _, change := range []string{"unchanged", "binding_removed", "profile_updated", "scope_revoked"} {
			t.Run(source+"/"+change, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				f := newDeviceHTTPFixture(t)
				profile, err := f.profiles.ResolveOwnerProfile(ctx, f.owner.String())
				if err != nil {
					t.Fatal(err)
				}
				binding := apitypes.RuntimeProfileToolBinding{ClientTool: &apitypes.RuntimeProfileClientTool{Name: "audioplayer.stop"}}
				arguments := json.RawMessage(`{}`)
				device := newFakeToolConn(func(_ context.Context, _ rpcpb.ClientTool, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
					return newRPCResultResponse(req.Id, &rpcpb.ClientDeviceAudioPlayerStopResponse{Value: &rpcpb.AudioPlayerStatus{State: "stopped", Repeat: "off", PlaylistRevision: 1}}, (*rpcapi.RPCPayload).FromClientDeviceAudioPlayerStopResponse)
				})
				if source == "mhs" {
					profile.Spec.Mhs = &apitypes.RuntimeProfileMhs{V0: &apitypes.MhsV0Manifest{Devices: []apitypes.MhsV0Device{{Id: "screen", Hwd: apitypes.MhsV0DeviceHwdDisplay}}}}
					binding = apitypes.RuntimeProfileToolBinding{Mhs: &apitypes.RuntimeProfileMhsTool{Id: "screen", Operation: apitypes.RuntimeProfileMhsToolOperationWrite, Fields: &[]string{"brightness_percent"}}}
					arguments = json.RawMessage(`{"brightness_percent":40}`)
					device = newFakeDeviceConn(func(_ context.Context, req *rpcapi.RPCRequest) (*rpcapi.RPCResponse, error) {
						payload, err := proto.Marshal(&rpcpb.DisplayHwdWriteResponse{Applied: &rpcpb.DisplayHwdReadResponse{BrightnessPercent: new(uint32(40))}})
						if err != nil {
							return nil, err
						}
						return newRPCResultResponse(req.Id, &rpcpb.ClientMhsV0WriteResponse{Payload: payload}, (*rpcapi.RPCPayload).FromClientMhsV0WriteResponse)
					})
				}
				binding.I18n = map[string]apitypes.RuntimeProfileI18nText{"en": {DisplayName: "Operation"}, "zh-CN": {DisplayName: "操作"}}
				profile.Spec.Resources.Tools = &map[string]apitypes.RuntimeProfileToolBinding{"operation": binding}
				put := func() {
					t.Helper()
					result, err := f.profiles.PutRuntimeProfile(ctx, adminhttp.PutRuntimeProfileRequestObject{Id: profile.Id, Body: &adminhttp.RuntimeProfileUpsert{Id: profile.Id, Spec: profile.Spec}})
					if err != nil {
						t.Fatal(err)
					}
					updated, ok := result.(adminhttp.PutRuntimeProfile200JSONResponse)
					if !ok {
						data, _ := json.Marshal(result)
						t.Fatalf("Profile update: %s", data)
					}
					profile = apitypes.RuntimeProfile(updated)
				}
				put()
				f.manager.SetPeerUp(f.owner, device)
				_, target, schema, err := toolcatalog.BindingSchema(profile, binding)
				if err != nil {
					t.Fatal(err)
				}
				var allowed atomic.Bool
				allowed.Store(true)
				candidate := toolcatalog.Tool{Alias: "operation", Source: source, Target: target, Schema: schema, Binding: binding, Authorize: func(context.Context) error {
					if !allowed.Load() {
						return errors.New("Workflow scope revoked")
					}
					return nil
				}}
				release, err := f.control.locks.Acquire(ctx, f.owner)
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				queued := &runtimeQueueContext{Context: ctx, waiting: make(chan struct{})}
				snapshot := profile
				done := make(chan error, 1)
				go func() {
					_, err := (&runtimeDevices{public: f.public}).Invoke(queued, f.owner.String(), snapshot, candidate, arguments)
					done <- err
				}()
				select {
				case <-queued.waiting:
				case <-ctx.Done():
					t.Fatal("Tool did not enter the owner queue")
				}
				if device.calls.Load() != 0 {
					t.Fatal("queued Tool reached the device")
				}
				switch change {
				case "binding_removed":
					delete(*profile.Spec.Resources.Tools, candidate.Alias)
					put()
				case "profile_updated":
					profile.Spec.AppConfig = &apitypes.RuntimeProfileAppConfig{"queue-test": "true"}
					put()
				case "scope_revoked":
					allowed.Store(false)
				}
				release()
				select {
				case err = <-done:
				case <-ctx.Done():
					t.Fatal("queued Tool did not finish")
				}
				if change == "unchanged" {
					if err != nil || device.calls.Load() != 1 {
						t.Fatalf("authorized control: error=%v calls=%d", err, device.calls.Load())
					}
				} else {
					var rejected *toolcatalog.InvocationError
					if !errors.As(err, &rejected) || rejected.Code != "TOOL_UNAVAILABLE" || device.calls.Load() != 0 {
						t.Fatalf("revoked Tool: error=%v calls=%d", err, device.calls.Load())
					}
				}
			})
		}
	}
}
