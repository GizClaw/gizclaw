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
	if got.Slots.Stable.Version == nil || *got.Slots.Stable.Version != "9.9.0" {
		t.Fatalf("firmware stable version = %#v", got.Slots.Stable.Version)
	}
	if got.Slots.Stable.Artifact == nil || got.Slots.Stable.Artifact.TarPath == "" {
		t.Fatalf("firmware stable artifact = %#v", got.Slots.Stable.Artifact)
	}

	var out bytes.Buffer
	download, err := env.peer.DownloadFirmware(env.ctx, "firmware.files.download.shared", rpcapi.FirmwareFilesDownloadRequest{
		FirmwareId: sharedFirmware,
		Channel:    rpcapi.FirmwareChannelNameStable,
		Path:       "MANIFEST.txt",
	}, &out)
	if err != nil {
		t.Fatalf("firmware.files.download shared: %v", err)
	}
	if !strings.Contains(out.String(), "gizclaw devkit firmware") {
		t.Fatalf("firmware manifest = %q", out.String())
	}
	if download.Bytes != int64(out.Len()) {
		t.Fatalf("firmware.download bytes = %d", download.Bytes)
	}
	if download.Metadata.FirmwareId != sharedFirmware || download.Metadata.Channel != rpcapi.FirmwareChannelNameStable {
		t.Fatalf("firmware.download metadata = %#v", download.Metadata)
	}
	if download.Metadata.File.Path != "MANIFEST.txt" {
		t.Fatalf("firmware.download file = %#v", download.Metadata.File)
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
	if _, err := denied.GetFirmware(env.ctx, "firmware.get.denied", rpcapi.FirmwareGetRequest{FirmwareId: sharedFirmware}); err == nil || !strings.Contains(err.Error(), "acl: denied") {
		t.Fatalf("firmware.get denied error = %v", err)
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
