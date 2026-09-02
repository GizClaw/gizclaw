package giztestcmd

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/giztest"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestConfigureClientRPCScopesDeviceResponse(t *testing.T) {
	client := &gizcli.Client{}
	vars := mustVariables(t, nil)
	counts := map[string]*inboundCounter{}
	steps := []giztest.Step{{ID: "info", Client: "alice", ClientRPC: &giztest.ClientRPCOperation{Method: "client.info.get", Response: map[string]any{"name": "Alice"}}}}
	if err := configureClientRPC(client, "alice", steps, vars, counts); err != nil {
		t.Fatal(err)
	}
	if client.Device.Name == nil || *client.Device.Name != "Alice" || counts["alice:client.info.get"] == nil {
		t.Fatalf("device=%#v counts=%#v", client.Device, counts)
	}
}

func TestConfigureClientRPCRejectsUnknownMethod(t *testing.T) {
	err := configureClientRPC(&gizcli.Client{}, "alice", []giztest.Step{{ID: "bad", Client: "alice", ClientRPC: &giztest.ClientRPCOperation{Method: "client.bad"}}}, mustVariables(t, nil), map[string]*inboundCounter{})
	if err == nil {
		t.Fatal("unknown client RPC accepted")
	}
}
