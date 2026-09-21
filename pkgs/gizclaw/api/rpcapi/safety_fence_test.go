package rpcapi

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestSafetyFenceRPCPresenceRoundTrip(t *testing.T) {
	for _, level := range []*apitypes.SafetyFenceLevel{nil, new(apitypes.SafetyFenceLevelOff), new(apitypes.SafetyFenceLevelGeneral), new(apitypes.SafetyFenceLevelChild)} {
		var payload RPCPayload
		if err := payload.FromServerReloadRunWorkspaceWithOptionsRequest(ServerReloadRunWorkspaceWithOptionsRequest{WorkspaceName: new("workspace"), Parameters: &WorkspaceParametersPatch{SafetyFenceLevel: level}}); err != nil {
			t.Fatal(err)
		}
		var wire bytes.Buffer
		if err := WriteRequest(&wire, &RPCRequest{V: RPCVersionV1, Id: "reload", Method: RPCMethodServerRunWorkspaceReloadWithOptions, Params: &payload}); err != nil {
			t.Fatal(err)
		}
		decoded, err := ReadRequest(&wire)
		if err != nil {
			t.Fatal(err)
		}
		value, err := decoded.Params.AsServerReloadRunWorkspaceWithOptionsRequest()
		if err != nil {
			t.Fatal(err)
		}
		got := value.Parameters.SafetyFenceLevel
		if (got == nil) != (level == nil) || (got != nil && *got != *level) {
			t.Fatalf("level = %v; want %v", got, level)
		}
	}
}
