//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRegistrationBindsFirmwareRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	got, err := env.peer.GetFirmware(env.ctx, "firmware.get.shared")
	if err != nil {
		t.Fatalf("firmware.get shared: %v", err)
	}
	if got.Name != sharedFirmware {
		t.Fatalf("firmware.get registered = %#v", got)
	}
	var registeredOut bytes.Buffer
	result, err := env.peer.DownloadFirmware(env.ctx, "firmware.files.download.shared", rpcapi.FirmwareFilesDownloadRequest{
		Channel: rpcapi.FirmwareChannelNameStable,
		Path:    "firmware/main.bin",
	}, &registeredOut)
	if err != nil {
		t.Fatalf("firmware.files.download registered: %v", err)
	}
	if result.Metadata.FirmwareId != sharedFirmware || result.Bytes == 0 || registeredOut.Len() == 0 {
		t.Fatalf("firmware.files.download registered = %#v bytes=%d", result.Metadata, registeredOut.Len())
	}

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	if _, err := denied.GetFirmware(env.ctx, "firmware.get.denied"); err == nil {
		t.Fatalf("firmware.get denied error = %v", err)
	}
	var deniedOut bytes.Buffer
	if _, err := denied.DownloadFirmware(env.ctx, "firmware.files.download.denied", rpcapi.FirmwareFilesDownloadRequest{
		Channel: rpcapi.FirmwareChannelNameStable,
		Path:    "firmware/main.bin",
	}, &deniedOut); err == nil {
		t.Fatalf("firmware.files.download denied error = %v", err)
	}
}
