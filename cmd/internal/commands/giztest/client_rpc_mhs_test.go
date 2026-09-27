package giztestcmd

import (
	"errors"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"google.golang.org/protobuf/proto"
)

func TestInstallMhsProvider(t *testing.T) {
	var handlers gizcli.DeviceControlHandlers
	for method, response := range map[string]any{
		"client.mhs.v0.read":  map[string]any{"payload": "CAEQMg=="},
		"client.mhs.v0.write": map[string]any{"payload": "CgQIABAU"},
	} {
		if err := installDeviceControl(&handlers, method, response); err != nil {
			t.Fatal(err)
		}
	}
	read, err := handlers.ReadMhsHwd(t.Context(), nil)
	if err != nil || len(read.Payload) == 0 {
		t.Fatalf("read = %v, %v", read, err)
	}
	var observed rpcpb.LedHwdReadResponse
	if err := proto.Unmarshal(read.Payload, &observed); err != nil || observed.BrightnessPercent == nil || *observed.BrightnessPercent != 50 {
		t.Fatalf("read payload = %v, %v", &observed, err)
	}
	read.Payload[0] = 0xff
	read, err = handlers.ReadMhsHwd(t.Context(), nil)
	if err != nil || read.Payload[0] != 8 {
		t.Fatalf("response is not independent: %v, %v", read, err)
	}
	written, err := handlers.WriteMhsHwd(t.Context(), nil)
	if err != nil || len(written.Payload) == 0 {
		t.Fatalf("write = %v, %v", written, err)
	}
	if err := installDeviceControl(&handlers, "client.mhs.v0.read", map[string]any{"error_code": 5}); err != nil {
		t.Fatal(err)
	}
	_, err = handlers.ReadMhsHwd(t.Context(), nil)
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("error = %v", err)
	}
	if err := installDeviceControl(&handlers, "client.mhs.v0.write", map[string]any{"unknown": true}); err == nil {
		t.Fatal("malformed scripted response accepted")
	}
}

func TestGoDeviceSimulatorServesEveryHwdPayload(t *testing.T) {
	reads := []struct {
		hwd     rpcpb.ClientHwd
		payload string
	}{
		{rpcpb.ClientHwd_CLIENT_HWD_WIFI, "CAESBGhvbWU="},
		{rpcpb.ClientHwd_CLIENT_HWD_BLE, "CAEQACAC"},
		{rpcpb.ClientHwd_CLIENT_HWD_MODEM, "CAEQARoDTFRF"},
		{rpcpb.ClientHwd_CLIENT_HWD_BATTERY, "CQAAAAAAAF5AEAEZAAAAAAAUrkA="},
		{rpcpb.ClientHwd_CLIENT_HWD_MIC, "CAEQAA=="},
		{rpcpb.ClientHwd_CLIENT_HWD_DISPLAY, "CEYQAQ=="},
		{rpcpb.ClientHwd_CLIENT_HWD_LED, "CAEQMg=="},
		{rpcpb.ClientHwd_CLIENT_HWD_SPEAKER, "CB4QAQ=="},
	}
	for _, tc := range reads {
		meta, err := rpcapi.ClientHwdMetadata(tc.hwd)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(meta.Name, func(t *testing.T) {
			var handlers gizcli.DeviceControlHandlers
			if err := installDeviceControl(&handlers, "client.mhs.v0.read", map[string]any{"payload": tc.payload}); err != nil {
				t.Fatal(err)
			}
			response, err := handlers.ReadMhsHwd(t.Context(), &rpcpb.ClientMhsV0ReadRequest{Id: meta.Name + ".main", Hwd: tc.hwd})
			if err != nil {
				t.Fatal(err)
			}
			message, err := rpcapi.ClientHwdReadResponseMessage(tc.hwd, response.Payload)
			if err != nil || proto.Size(message) == 0 {
				t.Fatalf("read payload %T, %v", message, err)
			}
		})
	}
	for _, tc := range []struct {
		hwd     rpcpb.ClientHwd
		payload string
	}{
		{rpcpb.ClientHwd_CLIENT_HWD_DISPLAY, "CgQISxAB"},
		{rpcpb.ClientHwd_CLIENT_HWD_LED, "CgQIABAU"},
		{rpcpb.ClientHwd_CLIENT_HWD_SPEAKER, "CgQINxAA"},
	} {
		meta, err := rpcapi.ClientHwdMetadata(tc.hwd)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(meta.Name+".write", func(t *testing.T) {
			var handlers gizcli.DeviceControlHandlers
			if err := installDeviceControl(&handlers, "client.mhs.v0.write", map[string]any{"payload": tc.payload}); err != nil {
				t.Fatal(err)
			}
			response, err := handlers.WriteMhsHwd(t.Context(), &rpcpb.ClientMhsV0WriteRequest{Id: meta.Name + ".main", Hwd: tc.hwd})
			if err != nil {
				t.Fatal(err)
			}
			message, err := rpcapi.ClientHwdWriteResponseMessage(tc.hwd, response.Payload)
			if err != nil || proto.Size(message) == 0 {
				t.Fatalf("write payload %T, %v", message, err)
			}
		})
	}
}
