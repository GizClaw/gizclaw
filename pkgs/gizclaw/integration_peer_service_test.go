package gizclaw_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestIntegrationPeerServiceLifecycle(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	adminPublicKey := ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: strPtr("admin")})

	device := newTestClient(t, ts)
	devicePublicKey := ensurePeerInfo(t, device, apitypes.DeviceInfo{
		Name: strPtr("peer"),
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn:    strPtr("sn/1"),
			Imeis: &[]apitypes.PeerIMEI{{Name: strPtr("main"), Tac: "12345678", Serial: "0000001"}},
			Labels: &[]apitypes.PeerLabel{{
				Key:   "batch",
				Value: "cn/east",
			}},
		},
	})

	items, err := listPeers(context.Background(), admin)
	if err != nil {
		t.Fatalf("ListPeers error: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("ListPeers returned %d items", len(items))
	}

	if _, err := approvePeer(context.Background(), admin, devicePublicKey, apitypes.PeerRoleClient); err != nil {
		t.Fatalf("ApprovePeer error: %v", err)
	}
	if _, err := refreshPeer(context.Background(), admin, devicePublicKey); err != nil {
		t.Fatalf("RefreshPeer error: %v", err)
	}
	if _, err := getPeer(context.Background(), admin, devicePublicKey); err != nil {
		t.Fatalf("GetPeer error: %v", err)
	}
	if publicKey, err := findPubKeyBySN(context.Background(), admin, "sn/1"); err != nil || publicKey != devicePublicKey {
		t.Fatalf("ResolvePeerBySN = %q, %v", publicKey, err)
	}
	if publicKey, err := findPubKeyByIMEI(context.Background(), admin, "12345678", "0000001"); err != nil || publicKey != devicePublicKey {
		t.Fatalf("ResolvePeerByIMEI = %q, %v", publicKey, err)
	}
	if _, err := getPeerInfo(context.Background(), admin, devicePublicKey); err != nil {
		t.Fatalf("GetPeerInfo error: %v", err)
	}
	if _, err := getPeerRuntime(context.Background(), admin, devicePublicKey); err != nil {
		t.Fatalf("GetPeerRuntime error: %v", err)
	}
	if _, err := blockPeer(context.Background(), admin, devicePublicKey); err != nil {
		t.Fatalf("BlockPeer error: %v", err)
	}
	if _, err := deletePeer(context.Background(), admin, adminPublicKey); err != nil {
		t.Fatalf("DeletePeer error: %v", err)
	}
}

func TestIntegrationAdminResourceAPIs(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: strPtr("admin")})

	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("ServerAdminClient error: %v", err)
	}

	missingResp, err := api.GetResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, "missing")
	if err != nil {
		t.Fatalf("GetResourceWithResponse(missing) error: %v", err)
	}
	if missingResp.JSON404 == nil || missingResp.JSON404.Error.Code != "RESOURCE_NOT_FOUND" {
		t.Fatalf("GetResource missing response status=%d body=%s", missingResp.StatusCode(), string(missingResp.Body))
	}

	resource := mustAdminResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)

	applyResp, err := api.ApplyResourceWithResponse(context.Background(), resource)
	if err != nil {
		t.Fatalf("ApplyResourceWithResponse(create) error: %v", err)
	}
	if applyResp.JSON200 == nil || applyResp.JSON200.Action != apitypes.ApplyActionCreated {
		t.Fatalf("ApplyResource create response status=%d body=%s", applyResp.StatusCode(), string(applyResp.Body))
	}

	getResp, err := api.GetResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err != nil {
		t.Fatalf("GetResourceWithResponse error: %v", err)
	}
	if getResp.JSON200 == nil {
		t.Fatalf("GetResource response status=%d body=%s", getResp.StatusCode(), string(getResp.Body))
	}

	updatedResource := mustAdminResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"description": "updated credential",
			"body": {"api_key": "secret"}
		}
	}`)
	updatedResp, err := api.ApplyResourceWithResponse(context.Background(), updatedResource)
	if err != nil {
		t.Fatalf("ApplyResourceWithResponse(update) error: %v", err)
	}
	if updatedResp.JSON200 == nil || updatedResp.JSON200.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("ApplyResource update response status=%d body=%s", updatedResp.StatusCode(), string(updatedResp.Body))
	}

	putResp, err := api.PutResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, "minimax-main", updatedResource)
	if err != nil {
		t.Fatalf("PutResourceWithResponse error: %v", err)
	}
	if putResp.JSON200 == nil {
		t.Fatalf("PutResource response status=%d body=%s", putResp.StatusCode(), string(putResp.Body))
	}

	deleteResp, err := api.DeleteResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err != nil {
		t.Fatalf("DeleteResourceWithResponse error: %v", err)
	}
	if deleteResp.JSON200 == nil {
		t.Fatalf("DeleteResource response status=%d body=%s", deleteResp.StatusCode(), string(deleteResp.Body))
	}
	getAfterDeleteResp, err := api.GetResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, "minimax-main")
	if err != nil {
		t.Fatalf("GetResourceWithResponse(after delete) error: %v", err)
	}
	if getAfterDeleteResp.JSON404 == nil || getAfterDeleteResp.JSON404.Error.Code != "RESOURCE_NOT_FOUND" {
		t.Fatalf("GetResource after delete response status=%d body=%s", getAfterDeleteResp.StatusCode(), string(getAfterDeleteResp.Body))
	}
}

func mustAdminResource(t *testing.T, raw string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := json.Unmarshal([]byte(raw), &resource); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return resource
}
