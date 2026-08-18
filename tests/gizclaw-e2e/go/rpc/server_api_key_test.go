//go:build gizclaw_e2e

package rpc_test

import (
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestServerAPIKeyManagementRPC(t *testing.T) {
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
	listed, err := env.peer.ListAPIKeys(env.ctx, "api-key.list", rpcapi.APIKeyListRequest{Limit: 100})
	if err != nil {
		t.Fatalf("list API keys: %v", err)
	}
	if !containsAPIKey(listed.Items, ordinary.Value.Name) || !containsAPIKey(listed.Items, manager.Value.Name) {
		t.Fatalf("list API keys did not return both created keys: %#v", listed.Items)
	}
	if _, err := env.peer.RevokeAPIKey(env.ctx, "api-key.revoke", rpcapi.APIKeyRevokeRequest{Name: ordinary.Value.Name}); err != nil {
		t.Fatalf("revoke ordinary API key: %v", err)
	}
	listed, err = env.peer.ListAPIKeys(env.ctx, "api-key.list.after-revoke", rpcapi.APIKeyListRequest{Limit: 100})
	if err != nil {
		t.Fatalf("list API keys after revoke: %v", err)
	}
	if containsAPIKey(listed.Items, ordinary.Value.Name) || !containsAPIKey(listed.Items, manager.Value.Name) {
		t.Fatalf("revoke did not remove exactly the requested key: %#v", listed.Items)
	}
}

func containsAPIKey(items []rpcapi.APIKey, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
