package giztest

import (
	"testing"

	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestConfigureClientRPCScopesDeviceResponse(t *testing.T) {
	client := &gizcli.Client{}
	vars := &variables{values: map[string]value{}}
	counts := map[string]*inboundCounter{}
	steps := []Step{{ID: "info", Client: "alice", ClientRPC: &ClientRPCOperation{Method: "client.info.get", Response: map[string]any{"name": "Alice"}}}}
	if err := configureClientRPC(client, "alice", steps, vars, counts); err != nil {
		t.Fatal(err)
	}
	if client.Device.Name == nil || *client.Device.Name != "Alice" || counts["alice:client.info.get"] == nil {
		t.Fatalf("device=%#v counts=%#v", client.Device, counts)
	}
}

func TestConfigureClientRPCRejectsUnknownMethod(t *testing.T) {
	err := configureClientRPC(&gizcli.Client{}, "alice", []Step{{ID: "bad", Client: "alice", ClientRPC: &ClientRPCOperation{Method: "client.bad"}}}, &variables{values: map[string]value{}}, map[string]*inboundCounter{})
	if err == nil {
		t.Fatal("unknown client RPC accepted")
	}
}
