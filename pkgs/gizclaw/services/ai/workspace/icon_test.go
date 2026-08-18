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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/iconasset"
)

func TestWorkspaceIconLifecycleAndProjection(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	srv.Assets = newTestObjectStore(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-icon")
	body := mustWorkspaceUpsert(t, `{
		"name": "workspace-icon",
		"workflow_id": "workflow-icon",
		"parameters": {"mode": "demo"}
	}`)
	createResponse, err := createWorkspaceForTest(srv, ctx, createWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	created, ok := createResponse.(createWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResponse)
	}
	workspaceID := created.Id

	want := workspaceIconPNG(t)
	uploadResponse, err := srv.UploadWorkspaceIcon(ctx, adminhttp.UploadWorkspaceIconRequestObject{
		Id: workspaceID, Format: adminhttp.UploadWorkspaceIconParamsFormat("png"), Body: bytes.NewReader(want),
	})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, ok := uploadResponse.(adminhttp.UploadWorkspaceIcon200JSONResponse)
	if !ok || uploaded.Icon == nil || uploaded.Icon.Png == nil || *uploaded.Icon.Png != iconasset.ObjectName(workspaceID, iconasset.FormatPNG) {
		t.Fatalf("UploadWorkspaceIcon() response = %#v", uploadResponse)
	}

	downloadResponse, err := srv.DownloadWorkspaceIcon(ctx, adminhttp.DownloadWorkspaceIconRequestObject{
		Id: workspaceID, Format: adminhttp.Png,
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
	putResponse, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &putBody})
	if err != nil {
		t.Fatal(err)
	}
	put, ok := putResponse.(adminhttp.PutWorkspace200JSONResponse)
	if !ok || put.Icon == nil || put.Icon.Png == nil {
		t.Fatalf("PutWorkspace() did not preserve icon: %#v", putResponse)
	}
	bad := "other/icon.png"
	putBody.Icon = &apitypes.Icon{Png: &bad}
	putResponse, err = srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Id: workspaceID, Body: &putBody})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := putResponse.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace(injected icon) response = %#v", putResponse)
	}

	deleteResponse, err := srv.DeleteWorkspaceIcon(ctx, adminhttp.DeleteWorkspaceIconRequestObject{
		Id: workspaceID, Format: adminhttp.DeleteWorkspaceIconParamsFormatPng,
	})
	if err != nil {
		t.Fatal(err)
	}
	deleted, ok := deleteResponse.(adminhttp.DeleteWorkspaceIcon200JSONResponse)
	if !ok || deleted.Icon != nil {
		t.Fatalf("DeleteWorkspaceIcon() response = %#v", deleteResponse)
	}
}

func TestWorkspaceIconAdminReadRemainsAvailableWhileMutationsAreFenced(t *testing.T) {
	srv := newTestServer(t)
	srv.Assets = newTestObjectStore(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-icon-fence")
	body := mustWorkspaceUpsert(t, `{
		"name": "workspace-icon-fence",
		"workflow_id": "workflow-icon-fence",
		"parameters": {"mode": "demo"}
	}`)
	createdResponse, err := createWorkspaceForTest(srv, ctx, createWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatal(err)
	}
	created := createdResponse.(createWorkspace200JSONResponse)
	icon := workspaceIconPNG(t)
	if response, err := srv.UploadWorkspaceIcon(ctx, adminhttp.UploadWorkspaceIconRequestObject{
		Id: created.Id, Format: adminhttp.UploadWorkspaceIconParamsFormatPng, Body: bytes.NewReader(icon),
	}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.UploadWorkspaceIcon200JSONResponse); !ok {
		t.Fatalf("UploadWorkspaceIcon() response = %#v", response)
	}
	if response, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Id: created.Id}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", response)
	}
	if response, err := srv.DownloadWorkspaceIcon(ctx, adminhttp.DownloadWorkspaceIconRequestObject{Id: created.Id, Format: adminhttp.Png}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.DownloadWorkspaceIcon200ImagepngResponse); !ok {
		t.Fatalf("DownloadWorkspaceIcon() while pending = %#v", response)
	}
	if response, err := srv.UploadWorkspaceIcon(ctx, adminhttp.UploadWorkspaceIconRequestObject{
		Id: created.Id, Format: adminhttp.UploadWorkspaceIconParamsFormatPng, Body: bytes.NewReader(icon),
	}); err != nil {
		t.Fatal(err)
	} else if conflict, ok := response.(adminhttp.UploadWorkspaceIcon409JSONResponse); !ok || conflict.Error.Code != WorkspacePendingDeletionCode {
		t.Fatalf("UploadWorkspaceIcon() while pending = %#v", response)
	}
	if response, err := srv.DeleteWorkspaceIcon(ctx, adminhttp.DeleteWorkspaceIconRequestObject{Id: created.Id, Format: adminhttp.DeleteWorkspaceIconParamsFormatPng}); err != nil {
		t.Fatal(err)
	} else if conflict, ok := response.(adminhttp.DeleteWorkspaceIcon409JSONResponse); !ok || conflict.Error.Code != WorkspacePendingDeletionCode {
		t.Fatalf("DeleteWorkspaceIcon() while pending = %#v", response)
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
