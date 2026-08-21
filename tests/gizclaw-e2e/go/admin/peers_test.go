//go:build gizclaw_e2e

package admin_test

import (
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestAdminAPIPeersListGetAndLookup(t *testing.T) {
	env := newAdminAPIHarness(t)

	limit := int32(10)
	list, err := env.api.ListPeersWithResponse(env.ctx, &adminhttp.ListPeersParams{Limit: &limit})
	if err != nil {
		t.Fatalf("list peers: %v", err)
	}
	requireStatusOK(t, list, list.Body)
	if list.JSON200 == nil || len(list.JSON200.Items) == 0 {
		t.Fatalf("list peers = %#v", list.JSON200)
	}

	get, err := env.api.GetPeerWithResponse(env.ctx, env.peerKey)
	if err != nil {
		t.Fatalf("get peer: %v", err)
	}
	requireStatusOK(t, get, get.Body)
	if get.JSON200 == nil {
		t.Fatalf("get peer = %#v", get.JSON200)
	}
	registration, err := get.JSON200.AsExternalRef0Registration()
	if err != nil || registration.PublicKey != env.peerKey {
		t.Fatalf("get peer registration = %#v, %v", registration, err)
	}

	found, err := env.api.FindPubKeyBySNWithResponse(env.ctx, env.peerSN)
	if err != nil {
		t.Fatalf("find peer by SN: %v", err)
	}
	requireStatusOK(t, found, found.Body)
	if found.JSON200 == nil || found.JSON200.PublicKey != env.peerKey {
		t.Fatalf("find peer by SN = %#v", found.JSON200)
	}
}

func TestAdminAPIRefreshPeerInvokesClientInfoAndIdentifiers(t *testing.T) {
	env := newAdminAPIHarness(t)
	peer := env.h.ConnectClientFromContextEventually("admin-api-peer", 30*time.Second)
	defer peer.Close()
	peer.Device = apitypes.DeviceInfo{
		Hardware: &apitypes.HardwareInfo{
			Manufacturer:     ptr("GizClaw"),
			Model:            ptr("E2E Admin Refresh"),
			HardwareRevision: ptr("rev-a"),
		},
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn:     ptr("admin-refresh-client-rpc"),
			Labels: &[]apitypes.PeerLabel{{Key: "mode", Value: "client-rpc"}},
		},
	}

	response, err := env.api.RefreshPeerWithResponse(env.ctx, env.peerKey)
	if err != nil {
		t.Fatalf("refresh peer: %v", err)
	}
	requireStatusOK(t, response, response.Body)
	if response.JSON200 == nil {
		t.Fatal("refresh peer missing JSON200")
	}
	device := response.JSON200.Peer.Device
	if device.Hardware == nil || device.Hardware.Model == nil || *device.Hardware.Model != "E2E Admin Refresh" {
		t.Fatalf("refreshed hardware = %#v", device)
	}
	if device.Identifiers == nil || device.Identifiers.Sn == nil || *device.Identifiers.Sn != "admin-refresh-client-rpc" {
		t.Fatalf("refreshed identifiers = %#v", device)
	}
}
