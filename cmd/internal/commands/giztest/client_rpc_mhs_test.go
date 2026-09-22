package giztestcmd

import (
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

func TestInstallMhsProvider(t *testing.T) {
	response := map[string]any{"states": []any{
		map[string]any{"device_id": "led.main", "state": "enabled", "value": map[string]any{"bool_value": false}},
		map[string]any{"device_id": "display.main", "state": "brightness", "value": map[string]any{"int_value": 40}},
	}}
	var handlers gizcli.DeviceControlHandlers
	for _, method := range []string{"client.mhs.v0.read", "client.mhs.v0.write"} {
		if err := installDeviceControl(&handlers, method, response); err != nil {
			t.Fatal(err)
		}
	}
	read, err := handlers.ReadMhsStates(t.Context(), nil)
	if err != nil || len(read.GetStates()) != 2 || read.States[0].Value.Value == nil || read.States[0].Value.GetBoolValue() || read.States[1].Value.GetIntValue() != 40 {
		t.Fatalf("read = %v, %v", read, err)
	}
	read.States[0].DeviceId = "mutated"
	read, err = handlers.ReadMhsStates(t.Context(), nil)
	if err != nil || read.States[0].DeviceId != "led.main" {
		t.Fatalf("response is not independent: %v, %v", read, err)
	}
	written, err := handlers.WriteMhsStates(t.Context(), nil)
	if err != nil || written.States[1].Value.GetIntValue() != 40 {
		t.Fatalf("write = %v, %v", written, err)
	}
	if err := installDeviceControl(&handlers, "client.mhs.v0.read", map[string]any{"error_code": 5}); err != nil {
		t.Fatal(err)
	}
	_, err = handlers.ReadMhsStates(t.Context(), nil)
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("error = %v", err)
	}
	if err := installDeviceControl(&handlers, "client.mhs.v0.write", map[string]any{"unknown": true}); err == nil {
		t.Fatal("malformed scripted response accepted")
	}
}
