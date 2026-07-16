package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/asset"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestAdminAssetLifecycle(t *testing.T) {
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true, StreamRequestBody: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{Assets: assets, AssetMaxBytes: 1024}, nil))

	upload := httptest.NewRequest(http.MethodPost, "/assets?media_type=image%2Fpng", bytes.NewBufferString("png-data"))
	upload.Header.Set("Content-Type", "application/octet-stream")
	uploadResponse, err := app.Test(upload)
	if err != nil {
		t.Fatal(err)
	}
	defer uploadResponse.Body.Close()
	if uploadResponse.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(uploadResponse.Body)
		t.Fatalf("upload status=%d body=%s", uploadResponse.StatusCode, body)
	}
	var created apitypes.Asset
	if err := json.NewDecoder(uploadResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Metadata.Ref == "" || created.Metadata.MediaType != "image/png" || len(created.Bindings) != 0 {
		t.Fatalf("created asset = %#v", created)
	}

	query := url.QueryEscape(created.Metadata.Ref)
	getResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/assets?ref="+query, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer getResponse.Body.Close()
	if getResponse.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", getResponse.StatusCode)
	}

	downloadResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/assets/content?ref="+query, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer downloadResponse.Body.Close()
	payload, err := io.ReadAll(downloadResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	if downloadResponse.StatusCode != http.StatusOK || downloadResponse.Header.Get("Content-Type") != "application/octet-stream" || string(payload) != "png-data" || downloadResponse.Header.Get("ETag") == "" {
		t.Fatalf("download status=%d content-type=%q etag=%q payload=%q", downloadResponse.StatusCode, downloadResponse.Header.Get("Content-Type"), downloadResponse.Header.Get("ETag"), payload)
	}

	deleteResponse, err := app.Test(httptest.NewRequest(http.MethodDelete, "/assets?ref="+query, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer deleteResponse.Body.Close()
	if deleteResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", deleteResponse.StatusCode)
	}
	missingResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/assets?ref="+query, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer missingResponse.Body.Close()
	if missingResponse.StatusCode != http.StatusNotFound {
		t.Fatalf("get deleted status = %d", missingResponse.StatusCode)
	}
}

func TestAdminAssetLifecycleThroughNetHTTPAdapter(t *testing.T) {
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true, StreamRequestBody: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{Assets: assets, AssetMaxBytes: 1024}, nil))
	server := httptest.NewServer(fiberHTTPHandler(app))
	defer server.Close()
	client, err := adminhttp.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.UploadAssetWithBodyWithResponse(context.Background(), &adminhttp.UploadAssetParams{MediaType: "image/png"}, "application/octet-stream", bytes.NewBufferString("png-data"))
	if err != nil {
		t.Fatal(err)
	}
	if response.JSON201 == nil {
		t.Fatalf("upload status=%d body=%s", response.StatusCode(), response.Body)
	}
}

func TestAdminAssetLifecycleThroughGizHTTP(t *testing.T) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientConn, serverConn := newTestWebRTCConnPair(t, serverKey, clientKey,
		testGiznetSecurityPolicy{allowService: func(_ giznet.PublicKey, service uint64) bool {
			return service == ServiceAdminHTTP
		}},
		testGiznetSecurityPolicy{})
	defer clientConn.Close()
	defer serverConn.Close()
	server := &Server{
		LocalStatic:        *serverKey,
		PeerStore:          mustBadgerInMemory(t, nil),
		AssetMetadataStore: kv.NewMemory(nil),
		AssetObjects:       objectstore.Dir(t.TempDir()),
	}
	if err := server.init(); err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.peerService.ServeConn(serverConn) }()
	if err := waitUntil(testReadyTimeout, func() error {
		if _, ok := server.manager.Peer(clientKey.Public); !ok {
			return errors.New("peer is not ready")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	httpClient := &http.Client{Transport: gizhttp.NewRoundTripper(clientConn, ServiceAdminHTTP), Timeout: 5 * time.Second}
	client, err := adminhttp.NewClientWithResponses("http://gizclaw", adminhttp.WithHTTPClient(httpClient))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.UploadAssetWithBodyWithResponse(context.Background(), &adminhttp.UploadAssetParams{MediaType: "image/png"}, "application/octet-stream", bytes.NewBufferString("png-data"))
	if err != nil {
		select {
		case serverErr := <-serveErr:
			t.Fatalf("upload error=%v ServeConn=%v", err, serverErr)
		default:
			t.Fatal(err)
		}
	}
	if response.JSON201 == nil {
		t.Fatalf("upload status=%d body=%s", response.StatusCode(), response.Body)
	}
}

func TestAdminAssetRejectsOversizedUpload(t *testing.T) {
	assets, err := asset.New(kv.NewMemory(nil), objectstore.Dir(t.TempDir()), asset.Options{})
	if err != nil {
		t.Fatal(err)
	}
	service := &adminService{Assets: assets, AssetMaxBytes: 3}
	response, err := service.UploadAsset(context.Background(), adminhttp.UploadAssetRequestObject{
		Params: adminhttp.UploadAssetParams{MediaType: "image/png"},
		Body:   bytes.NewBufferString("four"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := response.(adminhttp.UploadAsset413JSONResponse); !ok {
		t.Fatalf("UploadAsset() response = %T", response)
	}
}
