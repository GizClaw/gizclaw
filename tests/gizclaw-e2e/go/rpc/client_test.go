//go:build gizclaw_e2e

package rpc_test

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
)

func TestClientRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	imeis := []apitypes.PeerIMEI{{
		Name:   testStringPtr("primary"),
		Tac:    "12345678",
		Serial: "901234",
	}}
	labels := []apitypes.PeerLabel{{
		Key:   "mode",
		Value: "client-rpc",
	}}
	env.peer.Device = apitypes.DeviceInfo{
		Hardware: &apitypes.HardwareInfo{
			Manufacturer:     testStringPtr("GizClaw"),
			Model:            testStringPtr("E2E RPC"),
			HardwareRevision: testStringPtr("rev-a"),
		},
		Identifiers: &apitypes.DeviceIdentifiers{
			Sn:     testStringPtr("client-rpc-sn"),
			Imeis:  &imeis,
			Labels: &labels,
		},
	}

	admin := env.h.ConnectClientFromContext("admin-a")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create admin client: %v", err)
	}
	resp, err := api.RefreshPeerWithResponse(env.ctx, env.h.ContextPublicKey("peer-a"))
	if err != nil {
		t.Fatalf("refresh peer: %v", err)
	}
	if resp.JSON200 == nil {
		t.Fatalf("refresh peer status %d: %s", resp.StatusCode(), resp.Body)
	}
	if resp.JSON200.Errors != nil && len(*resp.JSON200.Errors) > 0 {
		t.Fatalf("refresh peer errors = %v", *resp.JSON200.Errors)
	}
	got := resp.JSON200.Peer.Device
	if got.Identifiers == nil || got.Identifiers.Sn == nil || *got.Identifiers.Sn != "client-rpc-sn" {
		t.Fatalf("refreshed device identifiers = %#v", got.Identifiers)
	}
	if got.Hardware == nil {
		t.Fatalf("refreshed hardware is nil: %#v", got)
	}
	if got.Hardware.Manufacturer == nil || *got.Hardware.Manufacturer != "GizClaw" {
		t.Fatalf("refreshed manufacturer = %#v", got.Hardware.Manufacturer)
	}
	if got.Hardware.Model == nil || *got.Hardware.Model != "E2E RPC" {
		t.Fatalf("refreshed model = %#v", got.Hardware.Model)
	}
	if got.Identifiers.Imeis == nil || len(*got.Identifiers.Imeis) != 1 || (*got.Identifiers.Imeis)[0].Serial != "901234" {
		t.Fatalf("refreshed imeis = %#v", got.Identifiers.Imeis)
	}
	if got.Identifiers.Labels == nil || len(*got.Identifiers.Labels) != 1 || (*got.Identifiers.Labels)[0].Value != "client-rpc" {
		t.Fatalf("refreshed labels = %#v", got.Identifiers.Labels)
	}
	if resp.JSON200.UpdatedFields == nil || !hasString(*resp.JSON200.UpdatedFields, "device.identifiers.imeis") {
		t.Fatalf("updated fields = %#v", resp.JSON200.UpdatedFields)
	}
}
