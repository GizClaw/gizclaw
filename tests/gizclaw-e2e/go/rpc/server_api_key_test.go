//go:build gizclaw_e2e

package rpc_test

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerAPIKeyCreateRPC(t *testing.T) {
	env := newServerResourceHarness(t)
	ordinary, err := env.peer.CreateAPIKey(env.ctx, "api-key.ordinary", rpcapi.APIKeyCreateRequest{
		DisplayName: "ordinary",
	})
	if err != nil {
		t.Fatalf("create ordinary API key: %v", err)
	}
	manager, err := env.peer.CreateAPIKey(env.ctx, "api-key.manager", rpcapi.APIKeyCreateRequest{
		DisplayName: "manager", ManageAPIKeys: true,
	})
	if err != nil {
		t.Fatalf("create manager API key: %v", err)
	}
	if ordinary.Value == nil || manager.Value == nil || ordinary.Value.ManageAPIKeys || !manager.Value.ManageAPIKeys {
		t.Fatalf("created metadata: ordinary=%#v manager=%#v", ordinary.Value, manager.Value)
	}
	if ordinary.Value.Name == manager.Value.Name || ordinary.APIKey == manager.APIKey ||
		!strings.HasPrefix(ordinary.APIKey, "gizclaw_sk_v1_") || !strings.HasPrefix(manager.APIKey, "gizclaw_sk_v1_") {
		t.Fatalf("created API keys are not distinct opaque credentials")
	}
}
