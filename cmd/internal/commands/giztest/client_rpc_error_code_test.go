package giztestcmd

import (
	"errors"
	"math"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// A YAML document decodes a negative error code as int and a non-negative one
// as uint64, so a scripted 404 has to survive both forms.
func TestDeviceControlErrorResponseAcceptsEveryIntegralForm(t *testing.T) {
	for _, raw := range []any{404, int64(404), uint64(404), uint(404), float64(404)} {
		scripted, err := deviceControlErrorResponse(map[string]any{"error_code": raw})
		if err != nil {
			t.Fatalf("%T: unexpected error %v", raw, err)
		}
		var rpcErr rpcapi.Error
		if !errors.As(scripted, &rpcErr) {
			t.Fatalf("%T: scripted error = %v", raw, scripted)
		}
		if rpcErr.Code != rpcapi.RPCErrorCodeNotFound {
			t.Fatalf("%T: code = %d, want %d", raw, rpcErr.Code, rpcapi.RPCErrorCodeNotFound)
		}
	}
	scripted, err := deviceControlErrorResponse(map[string]any{"error_code": -32602, "error_message": "no"})
	if err != nil {
		t.Fatalf("negative code: unexpected error %v", err)
	}
	var rpcErr rpcapi.Error
	if !errors.As(scripted, &rpcErr) || rpcErr.Code != rpcapi.RPCErrorCodeInvalidParams || rpcErr.Message != "no" {
		t.Fatalf("negative code = %+v", rpcErr)
	}
}

func TestDeviceControlErrorResponseRejectsMalformedCode(t *testing.T) {
	// A value that does not fit the int32 wire field would otherwise wrap into
	// a different RPC error code.
	outOfRange := []any{
		int64(math.MaxInt32) + 1,
		uint64(math.MaxInt32) + 1,
		int64(math.MinInt32) - 1,
		float64(math.MaxInt32) + 1,
		uint64(math.MaxInt64) + 1,
	}
	for _, raw := range append([]any{"404", true, 1.5, nil}, outOfRange...) {
		scripted, err := deviceControlErrorResponse(map[string]any{"error_code": raw})
		if err == nil {
			t.Fatalf("%T: expected a validation error, got scripted %v", raw, scripted)
		}
		if scripted != nil {
			t.Fatalf("%T: validation error must not install a provider", raw)
		}
	}
}

// A malformed code fails the step instead of installing a provider that answers
// with the validation message.
func TestInstallDeviceControlRejectsMalformedCode(t *testing.T) {
	var handlers gizcli.DeviceControlHandlers
	if err := installDeviceControl(&handlers, "client.wifi.saved.forget", map[string]any{"error_code": "404"}); err == nil {
		t.Fatal("installDeviceControl accepted a malformed error_code")
	}
	if handlers.ForgetWifi != nil {
		t.Fatal("installDeviceControl installed a provider for a malformed error_code")
	}
}

func TestInstallDeviceControlWifiScanAndConnectErrors(t *testing.T) {
	var handlers gizcli.DeviceControlHandlers
	for _, method := range []string{"client.wifi.scan", "client.wifi.connect"} {
		if err := installDeviceControl(&handlers, method, map[string]any{"error_code": -32602, "error_message": "rejected"}); err != nil {
			t.Fatalf("install %s: %v", method, err)
		}
	}
	_, scanErr := handlers.ScanWifi(t.Context(), new(int64(8000)))
	connectErr := handlers.ConnectWifi(t.Context(), "home", new("correct-horse"))
	for method, err := range map[string]error{"client.wifi.scan": scanErr, "client.wifi.connect": connectErr} {
		var rpcErr rpcapi.Error
		if !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.RPCErrorCodeInvalidParams || rpcErr.Message != "rejected" {
			t.Fatalf("%s error = %#v", method, err)
		}
	}
}
