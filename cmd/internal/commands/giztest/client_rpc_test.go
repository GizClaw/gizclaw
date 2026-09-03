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

// A scripted delay must never silently become an immediate answer: a scenario
// that scripts one is written to exercise a timeout, and answering at once
// would let it pass without reaching that path.
func TestScriptedDeviceDelayBounds(t *testing.T) {
	tests := []struct {
		name    string
		value   any
		wantMs  int64
		wantErr bool
	}{
		{name: "absent", value: map[string]any{}},
		{name: "zero", value: map[string]any{"delay_ms": 0}},
		{name: "typical", value: map[string]any{"delay_ms": 1200}, wantMs: 1200},
		{name: "at the bound", value: map[string]any{"delay_ms": maxScriptedDelayMs}, wantMs: maxScriptedDelayMs},
		{name: "above the bound", value: map[string]any{"delay_ms": int64(maxScriptedDelayMs) + 1}, wantErr: true},
		{name: "far above the bound", value: map[string]any{"delay_ms": uint64(1) << 62}, wantErr: true},
		{name: "negative", value: map[string]any{"delay_ms": -1}, wantErr: true},
		{name: "not an integer", value: map[string]any{"delay_ms": 1.5}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delay, err := scriptedDeviceDelay(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("delay = %v, want an error", delay)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if delay.Milliseconds() != test.wantMs {
				t.Fatalf("delay = %v, want %dms", delay, test.wantMs)
			}
		})
	}
}
