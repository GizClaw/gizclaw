package memorystore

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestScopeForRequestUsesSelectedImplementation(t *testing.T) {
	workspace := apitypes.Mem0MemoryLayoutPolicyScopeWorkspace
	volcPeer := apitypes.VolcMem0MemoryLayoutPolicyScopePeer
	request := Request{
		WorkspaceID: "workspace-a", OwnerPublicKey: "owner-a",
		Layout: apitypes.MemoryLayout{Spec: apitypes.MemoryLayoutSpec{
			Mem0:     apitypes.Mem0MemoryLayoutPolicy{Scope: &workspace},
			VolcMem0: apitypes.VolcMem0MemoryLayoutPolicy{Scope: &volcPeer},
		}},
	}
	if err := request.Binding.Connection.FromRuntimeProfileMem0Connection(apitypes.RuntimeProfileMem0Connection{
		Type: apitypes.RuntimeProfileMem0ConnectionTypeMem0, Endpoint: "https://api.mem0.ai", ApiKey: "test", ProjectId: "test",
	}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		driver apitypes.RuntimeProfileMemoryDriver
		shared bool
	}{
		{apitypes.RuntimeProfileMemoryDriverMem0, false},
		{apitypes.RuntimeProfileMemoryDriverVolcMem0, true},
	} {
		request.Binding.Driver = test.driver
		scope, err := ScopeForRequest(request)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(scope.AppID, "peer:") != test.shared {
			t.Fatalf("%s scope = %#v, shared=%v", test.driver, scope, test.shared)
		}
		if !test.shared && scope != (memory.Scope{AppID: request.WorkspaceID}) {
			t.Fatalf("%s scope = %#v", test.driver, scope)
		}
	}
	request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverMem0
	request.Layout.Spec.Mem0.Scope = nil
	scope, err := ScopeForRequest(request)
	if err != nil || scope.AppID != request.WorkspaceID {
		t.Fatalf("omitted scope = %#v, %v", scope, err)
	}
	selfHostedPeer := apitypes.Mem0SelfHostedMemoryLayoutPolicyScopePeer
	request.Layout.Spec.Mem0SelfHosted = &apitypes.Mem0SelfHostedMemoryLayoutPolicy{Scope: &selfHostedPeer}
	if err := request.Binding.Connection.FromRuntimeProfileMem0SelfHostedConnection(apitypes.RuntimeProfileMem0SelfHostedConnection{
		Type: apitypes.RuntimeProfileMem0SelfHostedConnectionTypeMem0SelfHosted, Endpoint: "http://localhost:8000",
	}); err != nil {
		t.Fatal(err)
	}
	if scope, err := ScopeForRequest(request); err != nil || !strings.HasPrefix(scope.AppID, "peer:") {
		t.Fatalf("independent self-hosted scope = %#v, %v", scope, err)
	}
	request.Layout.Spec.Mem0SelfHosted.Scope = nil
	if scope, err := ScopeForRequest(request); err != nil || scope.AppID != request.WorkspaceID {
		t.Fatalf("omitted self-hosted scope = %#v, %v", scope, err)
	}
	request.Binding.Driver = apitypes.RuntimeProfileMemoryDriverVolcMem0
	request.OwnerPublicKey = ""
	if _, err := ScopeForRequest(request); err == nil {
		t.Fatal("peer scope accepted without owner")
	}
}
