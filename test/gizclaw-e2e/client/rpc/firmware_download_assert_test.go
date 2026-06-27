//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/gizcli"
)

var firmwareBundleDownloads = []struct {
	path                string
	contains            []byte
	contentTypeContains string
}{
	{
		path:                "MANIFEST.txt",
		contains:            []byte("gizclaw devkit firmware bundle"),
		contentTypeContains: "text/plain",
	},
	{
		path:                "firmware/main.bin",
		contains:            []byte("GIZCLAW_MAIN_FIRMWARE_V1"),
		contentTypeContains: "application/octet-stream",
	},
	{
		path:                "firmware/voice_dsp.bin",
		contains:            []byte("GIZCLAW_VOICE_DSP_FIRMWARE_V1"),
		contentTypeContains: "application/octet-stream",
	},
	{
		path:                "firmware/wifi_coprocessor.bin",
		contains:            []byte("GIZCLAW_WIFI_COPROCESSOR_V1"),
		contentTypeContains: "application/octet-stream",
	},
	{
		path:                "assets/icons/status.png",
		contains:            []byte("\x89PNG\r\n\x1a\n"),
		contentTypeContains: "image/png",
	},
	{
		path:                "assets/images/splash.png",
		contains:            []byte("\x89PNG\r\n\x1a\n"),
		contentTypeContains: "image/png",
	},
	{
		path:                "config/device.json",
		contains:            []byte(`"modules":["main","voice_dsp","wifi_coprocessor"]`),
		contentTypeContains: "application/json",
	},
	{
		path:                "docs/release-notes.txt",
		contains:            []byte("artifact package used by e2e rpc firmware download tests"),
		contentTypeContains: "text/plain",
	},
}

func assertFirmwareBundleRPCDownloads(t *testing.T, ctx context.Context, peer *gizcli.Client, requestIDPrefix, firmwareID string) {
	t.Helper()

	for _, tt := range firmwareBundleDownloads {
		t.Run(strings.ReplaceAll(tt.path, "/", "_"), func(t *testing.T) {
			var out bytes.Buffer
			download, err := peer.DownloadFirmware(ctx, requestIDPrefix+"."+strings.ReplaceAll(tt.path, "/", "."), rpcapi.FirmwareFilesDownloadRequest{
				FirmwareId: firmwareID,
				Channel:    rpcapi.FirmwareChannelNameStable,
				Path:       tt.path,
			}, &out)
			if err != nil {
				t.Fatalf("firmware.files.download %s: %v", tt.path, err)
			}
			if download.Bytes != int64(out.Len()) {
				t.Fatalf("firmware.download %s bytes = %d, payload len = %d", tt.path, download.Bytes, out.Len())
			}
			if download.Metadata.FirmwareId != firmwareID || download.Metadata.Channel != rpcapi.FirmwareChannelNameStable {
				t.Fatalf("firmware.download %s metadata = %#v", tt.path, download.Metadata)
			}
			if download.Metadata.Path != tt.path || download.Metadata.File.Path != tt.path {
				t.Fatalf("firmware.download %s file metadata = %#v", tt.path, download.Metadata)
			}
			if !bytes.Contains(out.Bytes(), tt.contains) {
				t.Fatalf("firmware.download %s payload %q does not contain %q", tt.path, out.Bytes(), tt.contains)
			}
			if download.Metadata.File.Size != int64(out.Len()) {
				t.Fatalf("firmware.download %s file size = %d, payload len = %d", tt.path, download.Metadata.File.Size, out.Len())
			}
			if download.Metadata.File.ContentType == nil || !strings.Contains(*download.Metadata.File.ContentType, tt.contentTypeContains) {
				t.Fatalf("firmware.download %s content_type = %#v, want contains %q", tt.path, download.Metadata.File.ContentType, tt.contentTypeContains)
			}
		})
	}
}
