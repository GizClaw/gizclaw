package gizclaw_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestIntegrationPeerConnectionCollectsDeviceSN(t *testing.T) {
	ts := startTestServer(t)
	admin := newTestClient(t, ts)
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})
	sn := "auto-collected-sn"
	device := newTestClientWithDevice(t, ts, apitypes.DeviceInfo{
		Identifiers: &apitypes.DeviceIdentifiers{Sn: &sn},
	})
	publicKey := device.KeyPair.Public.String()

	if err := waitUntil(testReadyTimeout, func() error {
		info, err := getPeerInfo(context.Background(), admin, publicKey)
		if err != nil {
			return err
		}
		if info.Identifiers == nil || info.Identifiers.Sn == nil || *info.Identifiers.Sn != sn {
			return fmt.Errorf("stored identifiers = %#v", info.Identifiers)
		}
		return nil
	}); err != nil {
		t.Fatalf("automatic device SN collection: %v", err)
	}
}

func TestIntegrationPeerServiceLifecycle(t *testing.T) {
	ts := startTestServer(t)

	admin := newTestClient(t, ts)
	adminPublicKey := ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})

	device := newTestClientWithDevice(t, ts, apitypes.DeviceInfo{
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn:    new("sn/1"),
			Imeis: &[]apitypes.PeerIMEI{{Name: new("main"), Tac: "12345678", Serial: "0000001"}},
			Labels: &[]apitypes.PeerLabel{{
				Key:   "batch",
				Value: "cn/east",
			}},
		},
	})
	devicePublicKey := ensurePeerInfo(t, device, apitypes.DeviceInfo{
		Name: new("peer"),
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
	matched, err := findPeersBySN(context.Background(), admin, "sn/1")
	if err != nil || len(matched) != 1 {
		t.Fatalf("FindPeersBySN = %+v, %v", matched, err)
	}
	matchedPeer, err := matched[0].AsExternalRef0Registration()
	if err != nil || matchedPeer.PublicKey != devicePublicKey {
		t.Fatalf("FindPeersBySN peer = %+v, %v", matchedPeer, err)
	}
	if publicKey, err := findPubKeysByIMEI(context.Background(), admin, "12345678", "0000001"); err != nil || len(publicKey) != 1 || publicKey[0] != devicePublicKey {
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
	ensureAdminPeer(t, ts, admin, apitypes.DeviceInfo{Name: new("admin")})

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
		"metadata": {"id": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)

	applyResp, err := api.ApplyResourceWithResponse(context.Background(), mustWritableAdminResource(t, resource))
	if err != nil {
		t.Fatalf("ApplyResourceWithResponse(create) error: %v", err)
	}
	if applyResp.JSON200 == nil || applyResp.JSON200.Action != apitypes.ApplyActionCreated {
		t.Fatalf("ApplyResource create response status=%d body=%s", applyResp.StatusCode(), string(applyResp.Body))
	}
	if applyResp.JSON200.Id == nil || *applyResp.JSON200.Id != "minimax-main" {
		t.Fatalf("ApplyResource create response id = %v, want minimax-main: %s", applyResp.JSON200.Id, string(applyResp.Body))
	}
	credentialID := *applyResp.JSON200.Id

	getResp, err := api.GetResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, credentialID)
	if err != nil {
		t.Fatalf("GetResourceWithResponse error: %v", err)
	}
	if getResp.JSON200 == nil {
		t.Fatalf("GetResource response status=%d body=%s", getResp.StatusCode(), string(getResp.Body))
	}

	updatedResource := mustAdminResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "`+credentialID+`"},
		"spec": {
			"provider": "minimax",
			"description": "updated credential",
			"body": {"api_key": "secret"}
		}
	}`)
	updatedResp, err := api.ApplyResourceWithResponse(context.Background(), mustWritableAdminResource(t, updatedResource))
	if err != nil {
		t.Fatalf("ApplyResourceWithResponse(update) error: %v", err)
	}
	if updatedResp.JSON200 == nil || updatedResp.JSON200.Action != apitypes.ApplyActionUpdated {
		t.Fatalf("ApplyResource update response status=%d body=%s", updatedResp.StatusCode(), string(updatedResp.Body))
	}

	putResp, err := api.PutResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, credentialID, updatedResource)
	if err != nil {
		t.Fatalf("PutResourceWithResponse error: %v", err)
	}
	if putResp.JSON200 == nil {
		t.Fatalf("PutResource response status=%d body=%s", putResp.StatusCode(), string(putResp.Body))
	}

	deleteResp, err := api.DeleteResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, credentialID)
	if err != nil {
		t.Fatalf("DeleteResourceWithResponse error: %v", err)
	}
	if deleteResp.JSON200 == nil {
		t.Fatalf("DeleteResource response status=%d body=%s", deleteResp.StatusCode(), string(deleteResp.Body))
	}
	getAfterDeleteResp, err := api.GetResourceWithResponse(context.Background(), apitypes.ResourceKindCredential, credentialID)
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

func mustWritableAdminResource(t *testing.T, resource apitypes.Resource) adminhttp.ApplyResourceJSONRequestBody {
	t.Helper()
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var writable adminhttp.ApplyResourceJSONRequestBody
	if err := json.Unmarshal(data, &writable); err != nil {
		t.Fatalf("json.Unmarshal(writable) error = %v", err)
	}
	return writable
}

func TestIntegrationDeviceSetsOwnDebugMode(t *testing.T) {
	ts := startTestServer(t)
	first := newTestClientWithDevice(t, ts, apitypes.DeviceInfo{})
	second := newTestClientWithDevice(t, ts, apitypes.DeviceInfo{})
	for _, mode := range []string{"readonly", "fullcontrol", "off"} {
		got, err := putInfo(context.Background(), first, apitypes.DeviceInfo{DebugMode: &mode})
		if err != nil {
			t.Fatal(err)
		}
		if got.DebugMode == nil || *got.DebugMode != mode {
			t.Fatalf("RPC mode = %v", got.DebugMode)
		}
		other, err := getInfo(context.Background(), second)
		if err != nil {
			t.Fatal(err)
		}
		if other.DebugMode != nil && *other.DebugMode != "off" {
			t.Fatalf("other device mode = %s", *other.DebugMode)
		}
	}
	if _, err := putInfo(context.Background(), first, apitypes.DeviceInfo{DebugMode: new("invalid")}); err == nil {
		t.Fatal("invalid RPC mode accepted")
	}
}
