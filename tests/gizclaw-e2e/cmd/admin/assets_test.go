//go:build gizclaw_e2e

package admin_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestAdminAssetsUserStory(t *testing.T) {
	h := clitest.NewHarness(t, "512-admin-assets")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-assets-sn").MustSucceed(t)

	payload := []byte("immutable admin asset payload")
	uploadPath := filepath.Join(h.SandboxDir, "asset.bin")
	if err := os.WriteFile(uploadPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	upload := h.RunCLI("admin", "assets", "upload", "-f", uploadPath, "--media-type", "application/octet-stream", "--context", "admin-a")
	upload.MustSucceed(t)
	var created apitypes.Asset
	if err := json.Unmarshal([]byte(upload.Stdout), &created); err != nil {
		t.Fatalf("decode asset upload: %v\n%s", err, upload.Stdout)
	}
	if created.Metadata.Ref == "" || created.Metadata.SizeBytes != int64(len(payload)) || len(created.Bindings) != 0 {
		t.Fatalf("uploaded asset = %#v", created)
	}

	get := h.RunCLI("admin", "assets", "get", created.Metadata.Ref, "--context", "admin-a")
	get.MustSucceed(t)
	var got apitypes.Asset
	if err := json.Unmarshal([]byte(get.Stdout), &got); err != nil {
		t.Fatalf("decode asset get: %v\n%s", err, get.Stdout)
	}
	if got.Metadata.Ref != created.Metadata.Ref ||
		got.Metadata.MediaType != created.Metadata.MediaType ||
		got.Metadata.SizeBytes != created.Metadata.SizeBytes ||
		got.Metadata.Sha256 != created.Metadata.Sha256 ||
		!got.Metadata.CreatedAt.Equal(created.Metadata.CreatedAt) ||
		len(got.Bindings) != 0 {
		t.Fatalf("got asset = %#v, want %#v", got, created)
	}

	downloadPath := filepath.Join(h.SandboxDir, "downloaded.bin")
	download := h.RunCLI("admin", "assets", "download", created.Metadata.Ref, "-o", downloadPath, "--context", "admin-a")
	download.MustSucceed(t)
	downloaded, err := os.ReadFile(downloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(downloaded) != string(payload) {
		t.Fatalf("downloaded asset = %q, want %q", downloaded, payload)
	}

	deleted := h.RunCLI("admin", "assets", "delete", created.Metadata.Ref, "--context", "admin-a")
	deleted.MustSucceed(t)
	missing := h.RunCLI("admin", "assets", "get", created.Metadata.Ref, "--context", "admin-a")
	if missing.Err == nil {
		t.Fatal("get deleted asset unexpectedly succeeded")
	}
}
