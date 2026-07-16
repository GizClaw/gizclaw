package workflow

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/png"
	"sync"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestWorkflowIconLifecycleAndProjection(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	srv.Assets = objectstore.Dir(t.TempDir())
	ctx := context.Background()
	doc := mustDocument(t, `{"name":"icon-demo","spec":{"driver":"flowcraft","flowcraft":{"workspace_layout":{},"runtime":{},"agents":[],"entry_agent":""}}}`)
	if response, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc}); err != nil {
		t.Fatal(err)
	} else if _, ok := response.(adminhttp.CreateWorkflow200JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", response)
	}

	response, err := srv.UploadWorkflowIcon(ctx, adminhttp.UploadWorkflowIconRequestObject{
		Name: "icon-demo", Format: adminhttp.UploadWorkflowIconParamsFormatPng, Body: bytes.NewReader(iconPNG(t)),
	})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, ok := response.(adminhttp.UploadWorkflowIcon200JSONResponse)
	if !ok || uploaded.Icon == nil || uploaded.Icon.Png == nil || *uploaded.Icon.Png != "icon-demo/icon.png" {
		t.Fatalf("UploadWorkflowIcon() response = %#v", response)
	}

	ordinary := doc
	ordinary.Spec = uploaded.Spec
	putResponse, err := srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: "icon-demo", Body: &ordinary})
	if err != nil {
		t.Fatal(err)
	}
	put, ok := putResponse.(adminhttp.PutWorkflow200JSONResponse)
	if !ok || put.Icon == nil || put.Icon.Png == nil || *put.Icon.Png != "icon-demo/icon.png" {
		t.Fatalf("PutWorkflow() did not preserve icon: %#v", putResponse)
	}

	injected := ordinary
	bad := "other/icon.png"
	injected.Icon = &apitypes.Icon{Png: &bad}
	putResponse, err = srv.PutWorkflow(ctx, adminhttp.PutWorkflowRequestObject{Name: "icon-demo", Body: &injected})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := putResponse.(adminhttp.PutWorkflow400JSONResponse); !ok {
		t.Fatalf("PutWorkflow(injected icon) response = %#v", putResponse)
	}

	deleteResponse, err := srv.DeleteWorkflowIcon(ctx, adminhttp.DeleteWorkflowIconRequestObject{Name: "icon-demo", Format: adminhttp.DeleteWorkflowIconParamsFormatPng})
	if err != nil {
		t.Fatal(err)
	}
	deleted, ok := deleteResponse.(adminhttp.DeleteWorkflowIcon200JSONResponse)
	if !ok || deleted.Icon != nil {
		t.Fatalf("DeleteWorkflowIcon() response = %#v", deleteResponse)
	}
}

func TestWorkflowIconRejectsTooLarge(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	response, err := srv.UploadWorkflowIcon(context.Background(), adminhttp.UploadWorkflowIconRequestObject{
		Name: "missing", Format: adminhttp.UploadWorkflowIconParamsFormatPng, Body: bytes.NewReader(make([]byte, 2*1024*1024+1)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.UploadWorkflowIcon413JSONResponse); !ok {
		t.Fatalf("UploadWorkflowIcon(too large) response = %#v", response)
	}
}

func TestWorkflowIconConcurrentFormatsPreserveBothSlots(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	srv.Assets = objectstore.Dir(t.TempDir())
	ctx := context.Background()
	doc := mustDocument(t, `{"name":"both-icons","spec":{"driver":"flowcraft","flowcraft":{"workspace_layout":{},"runtime":{},"agents":[],"entry_agent":""}}}`)
	createResponse, err := srv.CreateWorkflow(ctx, adminhttp.CreateWorkflowRequestObject{Body: &doc})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := createResponse.(adminhttp.CreateWorkflow200JSONResponse); !ok {
		t.Fatalf("CreateWorkflow() response = %#v", createResponse)
	}
	var wg sync.WaitGroup
	responses := make(chan adminhttp.UploadWorkflowIconResponseObject, 2)
	for _, request := range []adminhttp.UploadWorkflowIconRequestObject{
		{Name: "both-icons", Format: adminhttp.UploadWorkflowIconParamsFormatPng, Body: bytes.NewReader(iconPNG(t))},
		{Name: "both-icons", Format: adminhttp.UploadWorkflowIconParamsFormatPixa, Body: bytes.NewReader(iconPIXA())},
	} {
		wg.Go(func() {
			response, _ := srv.UploadWorkflowIcon(ctx, request)
			responses <- response
		})
	}
	wg.Wait()
	close(responses)
	for response := range responses {
		if _, ok := response.(adminhttp.UploadWorkflowIcon200JSONResponse); !ok {
			t.Fatalf("UploadWorkflowIcon() response = %#v", response)
		}
	}
	got, err := srv.workflow(ctx, "both-icons")
	if err != nil {
		t.Fatal(err)
	}
	if got.Icon == nil || got.Icon.Png == nil || got.Icon.Pixa == nil {
		t.Fatalf("concurrent icon slots = %#v", got.Icon)
	}
}

func iconPNG(t *testing.T) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func iconPIXA() []byte {
	const headerSize, clipSize, frameSize = 40, 56, 16
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + clipSize
	payloadOffset := frameOffset + frameSize
	data := make([]byte, payloadOffset+2)
	copy(data, "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], 1)
	binary.LittleEndian.PutUint16(data[10:12], 1)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], 1)
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	binary.LittleEndian.PutUint32(data[36:40], 2)
	binary.LittleEndian.PutUint32(data[clipOffset+40:clipOffset+44], 1)
	binary.LittleEndian.PutUint32(data[frameOffset+8:frameOffset+12], 2)
	return data
}
