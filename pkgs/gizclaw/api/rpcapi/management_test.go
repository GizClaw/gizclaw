package rpcapi

import (
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
	"testing"
)

func TestManagementRejectsMissingOrContradictoryEffects(t *testing.T) {
	for _, message := range []proto.Message{
		(*rpcpb.ClientDeviceFeaturesGetResponse)(nil),
		&rpcpb.ClientContentAgeGetResponse{},
		&rpcpb.ClientContentAgeSetResponse{Applied: &rpcpb.ContentAgeState{OptionId: "unknown", Options: []*rpcpb.ContentAgeOption{{Id: "known", DisplayName: "Known"}}}},
		&rpcpb.ClientDeviceFeatureSetResponse{Applied: &rpcpb.DeviceFeature{Id: "x", DisplayName: "X", Enabled: new(false)}},
		&rpcpb.ClientUsageTimeGetResponse{State: &rpcpb.UsageTimeState{LimitSeconds: proto.Uint32(10), UsedSeconds: proto.Uint32(10), Exhausted: new(false)}},
		&rpcpb.ClientAISafetyGetResponse{State: &rpcpb.AISafetyState{Level: "missing"}},
		&rpcpb.ClientManagementPasswordGetResponse{}, &rpcpb.ClientManagementPasswordSetResponse{IsSet: new(false)}, &rpcpb.ClientManagementPasswordResetResponse{IsSet: new(true)},
	} {
		if err := ValidateManagementResponse(message); err == nil {
			t.Fatalf("accepted missing or contradictory result %T", message)
		}
	}
	for _, message := range []proto.Message{&rpcpb.ClientUsageTimeSetRequest{}, &rpcpb.ClientDeviceFeatureSetRequest{FeatureId: "x"}, &rpcpb.ClientManagementPasswordSetRequest{}, &rpcpb.ClientContentAgeSetRequest{}, &rpcpb.ClientAISafetySetRequest{}} {
		if err := ValidateClientToolRequest(message); err == nil {
			t.Fatalf("invalid request %T", message)
		}
	}
}
