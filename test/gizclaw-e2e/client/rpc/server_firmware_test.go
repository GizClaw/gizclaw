//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
)

func TestServerFirmwareRPC(t *testing.T) {
	env := newServerResourceHarness(t)

	list, err := env.peer.ListFirmwares(env.ctx, "firmware.list.shared", rpcapi.FirmwareListRequest{})
	if err != nil {
		t.Fatalf("firmware.list shared: %v", err)
	}
	if len(list.Items) == 0 {
		t.Fatalf("firmware.list returned no items")
	}

	got, err := env.peer.GetFirmware(env.ctx, "firmware.get.shared", rpcapi.FirmwareGetRequest{FirmwareId: sharedFirmware})
	if err != nil {
		t.Fatalf("firmware.get shared: %v", err)
	}
	if got.Name != sharedFirmware {
		t.Fatalf("firmware.get name = %q", got.Name)
	}
	if got.Slots.Stable.Artifact == nil || got.Slots.Stable.Artifact.TarPath == "" {
		t.Fatalf("firmware stable artifact = %#v", got.Slots.Stable.Artifact)
	}

	assertFirmwareBundleRPCDownloads(t, env.ctx, env.peer, "firmware.files.download.shared", sharedFirmware)

	denied := env.h.ConnectClientFromContext("peer-denied")
	defer denied.Close()
	deniedList, err := denied.ListFirmwares(env.ctx, "firmware.list.denied", rpcapi.FirmwareListRequest{})
	if err != nil {
		t.Fatalf("firmware.list denied peer: %v", err)
	}
	if len(deniedList.Items) != 0 {
		t.Fatalf("firmware.list denied items = %#v", deniedList.Items)
	}
	if _, err := denied.GetFirmware(env.ctx, "firmware.get.denied", rpcapi.FirmwareGetRequest{FirmwareId: sharedFirmware}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("firmware.get denied error = %v", err)
	}
	var deniedOut bytes.Buffer
	if _, err := denied.DownloadFirmware(env.ctx, "firmware.files.download.denied", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: sharedFirmware,
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "firmware/main.bin",
	}, &deniedOut); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("firmware.files.download denied error = %v", err)
	}
}

func hasFirmware(items []rpcapi.Firmware, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
