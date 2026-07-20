//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
)

func TestRegistrationDoesNotProjectFirmwareRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	list, err := env.peer.ListFirmwares(env.ctx, "firmware.list.shared", rpcapi.FirmwareListRequest{})
	if err != nil {
		t.Fatalf("firmware.list shared: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("firmware.list registered items = %#v", list.Items)
	}

	if _, err := env.peer.GetFirmware(env.ctx, "firmware.get.shared", rpcapi.FirmwareGetRequest{FirmwareId: sharedFirmware}); err == nil {
		t.Fatal("firmware.get registered error = nil")
	}
	var registeredOut bytes.Buffer
	if _, err := env.peer.DownloadFirmware(env.ctx, "firmware.files.download.shared", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: sharedFirmware,
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware/main.bin",
	}, &registeredOut); err == nil {
		t.Fatal("firmware.files.download registered error = nil")
	}

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	deniedList, err := denied.ListFirmwares(env.ctx, "firmware.list.denied", rpcapi.FirmwareListRequest{})
	if err != nil {
		t.Fatalf("firmware.list denied peer: %v", err)
	}
	if len(deniedList.Items) != 0 {
		t.Fatalf("firmware.list denied items = %#v", deniedList.Items)
	}
	if _, err := denied.GetFirmware(env.ctx, "firmware.get.denied", rpcapi.FirmwareGetRequest{FirmwareId: sharedFirmware}); err == nil {
		t.Fatalf("firmware.get denied error = %v", err)
	}
	var deniedOut bytes.Buffer
	if _, err := denied.DownloadFirmware(env.ctx, "firmware.files.download.denied", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: sharedFirmware,
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware/main.bin",
	}, &deniedOut); err == nil {
		t.Fatalf("firmware.files.download denied error = %v", err)
	}
}
