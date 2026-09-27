package rpcapi

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	rpcproto "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"google.golang.org/protobuf/proto"
)

func TestSafetyFenceRPCPresenceRoundTrip(t *testing.T) {
	for _, level := range []*apitypes.SafetyFenceLevel{nil, new(apitypes.SafetyFenceLevel("alpha")), new(apitypes.SafetyFenceLevel("bravo")), new(apitypes.SafetyFenceLevel("charlie")), new(apitypes.SafetyFenceLevel("delta"))} {
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

func TestLegacySafetyFenceEnumWireFieldDoesNotSelectNewString(t *testing.T) {
	// Field 4 carried the old WorkspaceParametersPatch enum as varint.
	var patch rpcproto.WorkspaceParametersPatch
	if err := proto.Unmarshal([]byte{0x20, 0x02}, &patch); err != nil {
		t.Fatal(err)
	}
	if patch.SafetyFenceLevel != nil {
		t.Fatalf("legacy enum became new identifier %q", *patch.SafetyFenceLevel)
	}
}

func TestWorkflowListCarriesProfileSafetyFenceCatalog(t *testing.T) {
	response := WorkflowListResponse{
		RuntimeProfileName: "profile", RuntimeProfileRevision: "revision",
		SafetyFences: []SafetyFenceOption{{Name: "alpha"}, {Name: "bravo", DisplayName: new("Bravo")}},
	}
	var payload RPCPayload
	if err := payload.FromWorkflowListResponse(response); err != nil {
		t.Fatal(err)
	}
	got, err := payload.AsWorkflowListResponse()
	if err != nil || len(got.SafetyFences) != 2 || got.SafetyFences[1].DisplayName == nil || *got.SafetyFences[1].DisplayName != "Bravo" {
		t.Fatalf("catalog = %#v, error = %v", got.SafetyFences, err)
	}
}
