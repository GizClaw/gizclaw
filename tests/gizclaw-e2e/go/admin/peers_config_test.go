//go:build gizclaw_e2e

package admin_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIPeerConfigInfoRuntime(t *testing.T) {
	env := newAdminAPIHarness(t)

	cfg, err := env.api.GetPeerConfigWithResponse(env.ctx, env.peerKey)
	if err != nil {
		t.Fatalf("get peer config: %v", err)
	}
	requireStatusOK(t, cfg, cfg.Body)
	if cfg.JSON200 == nil {
		t.Fatalf("get peer config missing JSON200")
	}

	nextCfg := *cfg.JSON200
	nextCfg.View = ptr("default-client")
	putCfg, err := env.api.PutPeerConfigWithResponse(env.ctx, env.peerKey, nextCfg)
	if err != nil {
		t.Fatalf("put peer config: %v", err)
	}
	requireStatusOK(t, putCfg, putCfg.Body)
	if putCfg.JSON200 == nil || putCfg.JSON200.View == nil || *putCfg.JSON200.View != "default-client" {
		t.Fatalf("put peer config = %#v", putCfg.JSON200)
	}

	info, err := env.api.GetPeerInfoWithResponse(env.ctx, env.peerKey)
	if err != nil {
		t.Fatalf("get peer info: %v", err)
	}
	requireStatusOK(t, info, info.Body)
	if info.JSON200 == nil || info.JSON200.Identifiers == nil || info.JSON200.Identifiers.Sn == nil || *info.JSON200.Identifiers.Sn != env.peerSN {
		t.Fatalf("get peer info = %#v", info.JSON200)
	}

	nextInfo := apitypes.DeviceInfo{Name: ptr("admin-api-peer-info")}
	putInfo, err := env.api.PutPeerInfoWithResponse(env.ctx, env.peerKey, nextInfo)
	if err != nil {
		t.Fatalf("put peer info: %v", err)
	}
	requireStatusOK(t, putInfo, putInfo.Body)
	if putInfo.JSON200 == nil || putInfo.JSON200.Name == nil || *putInfo.JSON200.Name != "admin-api-peer-info" {
		t.Fatalf("put peer info = %#v", putInfo.JSON200)
	}

	runtime, err := env.api.GetPeerRuntimeWithResponse(env.ctx, env.peerKey)
	if err != nil {
		t.Fatalf("get peer runtime: %v", err)
	}
	requireStatusOK(t, runtime, runtime.Body)
	if runtime.JSON200 == nil {
		t.Fatalf("get peer runtime missing JSON200")
	}
}
