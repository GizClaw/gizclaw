//go:build gizclaw_e2e

package rpc_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	rpcpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcproto"
	clitest "github.com/GizClaw/gizclaw-go/tests/gizclaw-e2e/cmd"
)

func TestServerAssetRPCRejectsUnboundAdminUpload(t *testing.T) {
	h := clitest.NewHarness(t, "512-admin-assets")
	h.StartServerFromFixture("server_config.yaml")
	h.CreateAdminContext("admin-a").MustSucceed(t)
	h.RegisterContext("admin-a", "--sn", "admin-assets-sn").MustSucceed(t)
	h.CreateContext("peer-a").MustSucceed(t)
	h.RegisterContext("peer-a", "--sn", "peer-assets-sn").MustSucceed(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	peer := h.ConnectClientFromContext("peer-a")
	t.Cleanup(func() { _ = peer.Close() })
	admin := h.ConnectClientFromContext("admin-a")
	t.Cleanup(func() { _ = admin.Close() })
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatal(err)
	}
	upload, err := api.UploadAssetWithBodyWithResponse(ctx, &adminhttp.UploadAssetParams{
		MediaType: "image/png",
	}, "application/octet-stream", bytes.NewBufferString("unbound"))
	if err != nil {
		t.Fatal(err)
	}
	if upload.JSON201 == nil {
		t.Fatalf("upload status=%d body=%s", upload.StatusCode(), strings.TrimSpace(string(upload.Body)))
	}
	ref := upload.JSON201.Metadata.Ref
	t.Cleanup(func() {
		_, _ = api.DeleteAssetWithResponse(ctx, &adminhttp.DeleteAssetParams{Ref: apitypes.AssetRef(ref)})
	})

	var out bytes.Buffer
	if _, err := peer.DownloadAsset(ctx, "asset.download.unbound", rpcpb.AssetDownloadRequest{Ref: ref}, &out); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("DownloadAsset(unbound) error=%v bytes=%q", err, out.Bytes())
	}
	if out.Len() != 0 {
		t.Fatalf("unauthorized download sent %d bytes", out.Len())
	}
}
