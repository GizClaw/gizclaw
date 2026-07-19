package workspace

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestWorkspaceIconLifecycleAndProjection(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	srv.Assets = objectstore.Dir(t.TempDir())
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-icon")
	body := mustWorkspaceUpsert(t, `{
		"name": "workspace-icon",
		"workflow_name": "workflow-icon",
		"parameters": {"mode": "demo"}
	}`)
	createResponse, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := createResponse.(adminhttp.CreateWorkspace200JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResponse)
	}

	want := workspaceIconPNG(t)
	uploadResponse, err := srv.UploadWorkspaceIcon(ctx, adminhttp.UploadWorkspaceIconRequestObject{
		Name: "workspace-icon", Format: adminhttp.UploadWorkspaceIconParamsFormat("png"), Body: bytes.NewReader(want),
	})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, ok := uploadResponse.(adminhttp.UploadWorkspaceIcon200JSONResponse)
	if !ok || uploaded.Icon == nil || uploaded.Icon.Png == nil || *uploaded.Icon.Png != "workspace-icon/icon.png" {
		t.Fatalf("UploadWorkspaceIcon() response = %#v", uploadResponse)
	}

	downloadResponse, err := srv.DownloadWorkspaceIcon(ctx, adminhttp.DownloadWorkspaceIconRequestObject{
		Name: "workspace-icon", Format: adminhttp.Png,
	})
	if err != nil {
		t.Fatal(err)
	}
	downloaded, ok := downloadResponse.(adminhttp.DownloadWorkspaceIcon200ImagepngResponse)
	if !ok {
		t.Fatalf("DownloadWorkspaceIcon() response = %#v", downloadResponse)
	}
	got, err := io.ReadAll(downloaded.Body)
	if err != nil {
		t.Fatal(err)
	}
	if closer, ok := downloaded.Body.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("DownloadWorkspaceIcon() bytes differ")
	}

	putBody := body
	putResponse, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "workspace-icon", Body: &putBody})
	if err != nil {
		t.Fatal(err)
	}
	put, ok := putResponse.(adminhttp.PutWorkspace200JSONResponse)
	if !ok || put.Icon == nil || put.Icon.Png == nil {
		t.Fatalf("PutWorkspace() did not preserve icon: %#v", putResponse)
	}
	bad := "other/icon.png"
	putBody.Icon = &apitypes.Icon{Png: &bad}
	putResponse, err = srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "workspace-icon", Body: &putBody})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := putResponse.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace(injected icon) response = %#v", putResponse)
	}

	deleteResponse, err := srv.DeleteWorkspaceIcon(ctx, adminhttp.DeleteWorkspaceIconRequestObject{
		Name: "workspace-icon", Format: adminhttp.DeleteWorkspaceIconParamsFormatPng,
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, ok := deleteResponse.(adminhttp.DeleteWorkspaceIcon200JSONResponse)
	if !ok || deleted.Icon != nil {
		t.Fatalf("DeleteWorkspaceIcon() response = %#v", deleteResponse)
	}
}

func workspaceIconPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
