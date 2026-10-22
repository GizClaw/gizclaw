package gizcli

import (
	"context"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestManagementProvidersDiscoveryEffectsAndFailures(t *testing.T) {
	device := &Client{}
	limit := uint32(60)
	feature := true
	passwordSet := false
	age := "child"
	level := "standard"
	factoryResets := 0
	if err := device.HandleDeviceControl(DeviceControlHandlers{FactoryReset: func(context.Context, bool) error { factoryResets++; return nil }}); err != nil {
		t.Fatal(err)
	}
	ageState := func() *rpcpb.ContentAgeState {
		return &rpcpb.ContentAgeState{OptionId: age, Options: []*rpcpb.ContentAgeOption{{Id: "child", DisplayName: "Child", MinAgeYears: proto.Uint32(6), MaxAgeYears: proto.Uint32(12)}, {Id: "teen", DisplayName: "Teen", MinAgeYears: proto.Uint32(13)}}}
	}
	safetyState := func() *rpcpb.AISafetyState {
		return &rpcpb.AISafetyState{Level: level, Options: []*rpcpb.AISafetyOption{{Id: "standard", DisplayName: "Standard"}, {Id: "strict", DisplayName: "Strict"}}}
	}
	featureState := func() *rpcpb.DeviceFeature {
		return &rpcpb.DeviceFeature{Id: "camera", DisplayName: "Camera", Enabled: new(feature), Writable: new(true)}
	}
	usageState := func() *rpcpb.UsageTimeState {
		return &rpcpb.UsageTimeState{LimitSeconds: new(limit), UsedSeconds: proto.Uint32(30), Exhausted: new(limit > 0 && 30 >= limit)}
	}
	handlers := map[rpcpb.ClientTool]ClientToolHandler{
		rpcpb.ClientTool_CLIENT_TOOL_CONTENT_AGE_GET: func(context.Context, proto.Message) (proto.Message, error) {
			return &rpcpb.ClientContentAgeGetResponse{State: ageState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_CONTENT_AGE_SET: func(_ context.Context, m proto.Message) (proto.Message, error) {
			next := m.(*rpcpb.ClientContentAgeSetRequest).OptionId
			if next != "teen" && next != "child" {
				return nil, ErrDeviceResourceNotFound
			}
			age = next
			return &rpcpb.ClientContentAgeSetResponse{Applied: ageState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FEATURES_GET: func(context.Context, proto.Message) (proto.Message, error) {
			return &rpcpb.ClientDeviceFeaturesGetResponse{Features: []*rpcpb.DeviceFeature{featureState()}}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FEATURE_SET: func(_ context.Context, m proto.Message) (proto.Message, error) {
			v := m.(*rpcpb.ClientDeviceFeatureSetRequest)
			if v.FeatureId != "camera" {
				return nil, ErrDeviceResourceNotFound
			}
			feature = *v.Enabled
			return &rpcpb.ClientDeviceFeatureSetResponse{Applied: featureState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_GET: func(context.Context, proto.Message) (proto.Message, error) {
			return &rpcpb.ClientUsageTimeGetResponse{State: usageState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_SET: func(_ context.Context, m proto.Message) (proto.Message, error) {
			limit = min(*m.(*rpcpb.ClientUsageTimeSetRequest).LimitSeconds, 3600)
			return &rpcpb.ClientUsageTimeSetResponse{Applied: usageState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_AI_SAFETY_GET: func(context.Context, proto.Message) (proto.Message, error) {
			return &rpcpb.ClientAISafetyGetResponse{State: safetyState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_AI_SAFETY_SET: func(_ context.Context, m proto.Message) (proto.Message, error) {
			next := m.(*rpcpb.ClientAISafetySetRequest).Level
			if next != "standard" && next != "strict" {
				return nil, ErrDeviceResourceNotFound
			}
			level = next
			return &rpcpb.ClientAISafetySetResponse{Applied: safetyState()}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_GET: func(context.Context, proto.Message) (proto.Message, error) {
			return &rpcpb.ClientManagementPasswordGetResponse{IsSet: new(passwordSet)}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_SET: func(context.Context, proto.Message) (proto.Message, error) {
			passwordSet = true
			return &rpcpb.ClientManagementPasswordSetResponse{IsSet: new(passwordSet)}, nil
		},
		rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_RESET: func(context.Context, proto.Message) (proto.Message, error) {
			passwordSet = false
			return &rpcpb.ClientManagementPasswordResetResponse{IsSet: new(passwordSet)}, nil
		},
	}
	for tool := range handlers {
		if r := deviceControlDispatch(t, device, tool, nil); r.Error == nil {
			t.Fatal("uninstalled tool reported success")
		}
		if err := device.HandleClientTool(tool, handlers[tool]); err != nil {
			t.Fatal(err)
		}
	}
	listed := deviceControlDispatch(t, device, rpcapi.RPCMethodClientToolV0List, nil)
	tools, err := listed.Result.AsClientToolV0ListResponse()
	if err != nil {
		t.Fatal(err)
	}
	for tool := range handlers {
		if !slices.Contains(tools.Tools, tool) {
			t.Fatal("installed tool absent from discovery")
		}
	}
	cases := []struct {
		tool   rpcpb.ClientTool
		encode func(*rpcapi.RPCPayload) error
	}{
		{rpcpb.ClientTool_CLIENT_TOOL_CONTENT_AGE_SET, func(p *rpcapi.RPCPayload) error {
			return p.FromClientContentAgeSetRequest(&rpcpb.ClientContentAgeSetRequest{OptionId: "teen"})
		}},
		{rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FEATURE_SET, func(p *rpcapi.RPCPayload) error {
			return p.FromClientDeviceFeatureSetRequest(&rpcpb.ClientDeviceFeatureSetRequest{FeatureId: "camera", Enabled: new(false)})
		}},
		{rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_SET, func(p *rpcapi.RPCPayload) error {
			return p.FromClientUsageTimeSetRequest(&rpcpb.ClientUsageTimeSetRequest{LimitSeconds: proto.Uint32(9000)})
		}},
		{rpcpb.ClientTool_CLIENT_TOOL_AI_SAFETY_SET, func(p *rpcapi.RPCPayload) error {
			return p.FromClientAISafetySetRequest(&rpcpb.ClientAISafetySetRequest{Level: "strict"})
		}},
		{rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_SET, func(p *rpcapi.RPCPayload) error {
			return p.FromClientManagementPasswordSetRequest(&rpcpb.ClientManagementPasswordSetRequest{Password: "test-sensitive-password"})
		}},
	}
	observed := false
	device.ObserveDeviceRequest(func(_ rpcapi.RPCMethod, tool rpcpb.ClientTool, message proto.Message) {
		if tool == rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_SET {
			observed = true
			if message.(*rpcpb.ClientManagementPasswordSetRequest).Password != "" {
				t.Fatal("password exposed to observer")
			}
		}
	})
	for _, tc := range cases {
		if r := deviceControlDispatch(t, device, tc.tool, tc.encode); r.Error != nil {
			t.Fatal(r.Error)
		}
	}
	if age != "teen" || feature || limit != 3600 || level != "strict" || !passwordSet || !observed {
		t.Fatal("provider effects not committed")
	}
	usage := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_GET, nil)
	state, err := usage.Result.AsClientUsageTimeGetResponse()
	if err != nil || state.State.GetLimitSeconds() != 3600 {
		t.Fatal("actual clamped value lost")
	}
	if r := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_MANAGEMENT_PASSWORD_RESET, nil); r.Error != nil || passwordSet || factoryResets != 0 {
		t.Fatal("password reset performed factory reset")
	}
	if r := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FEATURE_SET, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFeatureSetRequest(&rpcpb.ClientDeviceFeatureSetRequest{FeatureId: "unknown", Enabled: new(false)})
	}); r.Error == nil || r.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatal("unknown feature accepted")
	}
	if r := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_DEVICE_FEATURE_SET, func(p *rpcapi.RPCPayload) error {
		return p.FromClientDeviceFeatureSetRequest(&rpcpb.ClientDeviceFeatureSetRequest{FeatureId: "camera"})
	}); r.Error == nil || r.Error.Code != rpcapi.StatusCodeInvalidArgument {
		t.Fatal("missing false presence accepted")
	}
	if err := device.HandleClientTool(rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_GET, func(context.Context, proto.Message) (proto.Message, error) {
		return &rpcpb.ClientUsageTimeGetResponse{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if r := deviceControlDispatch(t, device, rpcpb.ClientTool_CLIENT_TOOL_USAGE_TIME_GET, nil); r.Error == nil {
		t.Fatal("missing actual result reported success")
	}
}
