package gizclaw

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/badge"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/petspecies"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkg/store/kv"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestAdminServiceApplyResourceRequiresBody(t *testing.T) {
	t.Parallel()

	resp, err := (&adminService{}).ApplyResource(context.Background(), adminservice.ApplyResourceRequestObject{})
	if err != nil {
		t.Fatalf("ApplyResource() error = %v", err)
	}
	got, ok := resp.(adminservice.ApplyResource400JSONResponse)
	if !ok {
		t.Fatalf("ApplyResource() response = %T", resp)
	}
	if got.Error.Code != "INVALID_RESOURCE" {
		t.Fatalf("ApplyResource() code = %q", got.Error.Code)
	}
}

func TestAdminServiceResourceMethodsHandleValidationAndManagerErrors(t *testing.T) {
	resource := mustPeerServiceResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"method": "api_key",
			"body": {"api_key": "secret"}
		}
	}`)
	service := &adminService{}

	applyResp, err := service.ApplyResource(context.Background(), adminservice.ApplyResourceRequestObject{JSONBody: &resource})
	if err != nil {
		t.Fatalf("ApplyResource() error = %v", err)
	}
	if got, ok := applyResp.(adminservice.ApplyResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("ApplyResource() response = %T %+v", applyResp, applyResp)
	}

	getResp, err := service.GetResource(context.Background(), adminservice.GetResourceRequestObject{
		Kind: apitypes.ResourceKindCredential,
		Name: "minimax-main",
	})
	if err != nil {
		t.Fatalf("GetResource() error = %v", err)
	}
	if got, ok := getResp.(adminservice.GetResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("GetResource() response = %T %+v", getResp, getResp)
	}

	putResp, err := service.PutResource(context.Background(), adminservice.PutResourceRequestObject{})
	if err != nil {
		t.Fatalf("PutResource(nil body) error = %v", err)
	}
	if got, ok := putResp.(adminservice.PutResource400JSONResponse); !ok || got.Error.Code != "INVALID_RESOURCE" {
		t.Fatalf("PutResource(nil body) response = %T %+v", putResp, putResp)
	}

	putResp, err = service.PutResource(context.Background(), adminservice.PutResourceRequestObject{
		Kind:     apitypes.ResourceKindWorkspace,
		Name:     "minimax-main",
		JSONBody: &resource,
	})
	if err != nil {
		t.Fatalf("PutResource(path mismatch) error = %v", err)
	}
	if got, ok := putResp.(adminservice.PutResource400JSONResponse); !ok || got.Error.Code != "INVALID_RESOURCE_PATH" {
		t.Fatalf("PutResource(path mismatch) response = %T %+v", putResp, putResp)
	}

	putResp, err = service.PutResource(context.Background(), adminservice.PutResourceRequestObject{
		Kind:     apitypes.ResourceKindCredential,
		Name:     "minimax-main",
		JSONBody: &resource,
	})
	if err != nil {
		t.Fatalf("PutResource(manager error) error = %v", err)
	}
	if got, ok := putResp.(adminservice.PutResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("PutResource(manager error) response = %T %+v", putResp, putResp)
	}

	deleteResp, err := service.DeleteResource(context.Background(), adminservice.DeleteResourceRequestObject{
		Kind: apitypes.ResourceKindCredential,
		Name: "minimax-main",
	})
	if err != nil {
		t.Fatalf("DeleteResource() error = %v", err)
	}
	if got, ok := deleteResp.(adminservice.DeleteResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("DeleteResource() response = %T %+v", deleteResp, deleteResp)
	}
}

func TestAdminResourceHelpers(t *testing.T) {
	resource := mustPeerServiceResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"method": "api_key",
			"body": {"api_key": "secret"}
		}
	}`)

	if err := validateResourcePathMatch(resource, apitypes.ResourceKindCredential, "minimax-main"); err != nil {
		t.Fatalf("validateResourcePathMatch() error = %v", err)
	}
	if err := validateResourcePathMatch(resource, apitypes.ResourceKindWorkspace, "minimax-main"); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("validateResourcePathMatch(kind mismatch) error = %v", err)
	}
	if err := validateResourcePathMatch(resource, apitypes.ResourceKindCredential, "other"); err == nil || !strings.Contains(err.Error(), "metadata.name") {
		t.Fatalf("validateResourcePathMatch(name mismatch) error = %v", err)
	}

	status, body := resourceManagerError(&resourcemanager.Error{StatusCode: http.StatusNotFound, Code: "RESOURCE_NOT_FOUND", Message: "missing"})
	if status != http.StatusNotFound || body.Error.Code != "RESOURCE_NOT_FOUND" {
		t.Fatalf("resourceManagerError(resource error) = %d %+v", status, body)
	}
	status, body = resourceManagerError(errors.New("boom"))
	if status != http.StatusInternalServerError || body.Error.Code != "RESOURCE_MANAGER_ERROR" {
		t.Fatalf("resourceManagerError(generic error) = %d %+v", status, body)
	}
}

func TestResource200JSONResponseSerializesResourceUnion(t *testing.T) {
	resource := mustPeerServiceResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"name": "minimax-main"},
		"spec": {
			"provider": "minimax",
			"method": "api_key",
			"body": {"api_key": "secret"}
		}
	}`)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/resource", func(ctx *fiber.Ctx) error {
		return resource200JSONResponse{Resource: resource}.VisitGetResourceResponse(ctx)
	})

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	rec := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"kind":"Credential"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestBusinessAssetHandlersServePetSpeciesPixa(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := &petspecies.Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
	}
	if _, err := service.Put(ctx, "rabbit", apitypes.PetSpeciesSpec{Name: "Rabbit"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminservice.RegisterHandlers(app, adminservice.NewStrictHandler(&adminService{PetSpecies: service}, nil))

	pixa := string(testAdminPixa(240, 240, []string{"idle"}))
	rec := serveAdminAsset(app, http.MethodPut, "/pet-species/rabbit/pixa", pixa)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"pixa_path":"rabbit.pixa"`) || !strings.Contains(rec.Body.String(), `"clip_names":["idle"]`) {
		t.Fatalf("PUT body = %s", rec.Body.String())
	}

	rec = serveAdminAsset(app, http.MethodGet, "/pet-species/rabbit/pixa", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != pixa {
		t.Fatalf("GET body = %q", rec.Body.String())
	}

	rec = serveAdminAsset(app, http.MethodPut, "/pet-species/missing/pixa", pixa)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PUT missing status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBusinessAssetHandlersServeBadgeIcon(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	service := &badge.Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
	}
	if _, err := service.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder"}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminservice.RegisterHandlers(app, adminservice.NewStrictHandler(&adminService{Badges: service}, nil))

	icon := "png-bytes"
	rec := serveAdminAsset(app, http.MethodPut, "/badges/founder/icon", icon)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"icon_path":"founder/icon"`) {
		t.Fatalf("PUT body = %s", rec.Body.String())
	}

	rec = serveAdminAsset(app, http.MethodGet, "/badges/founder/icon", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != icon {
		t.Fatalf("GET body = %q", rec.Body.String())
	}

	rec = serveAdminAsset(app, http.MethodPut, "/badges/missing/icon", icon)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("PUT missing status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBusinessResourceHandlersListResources(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	petSpecies := &petspecies.Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
	}
	if _, err := petSpecies.Put(ctx, "rabbit", apitypes.PetSpeciesSpec{Name: "Rabbit"}); err != nil {
		t.Fatalf("Put(pet species) error = %v", err)
	}
	badges := &badge.Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
	}
	if _, err := badges.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder", Description: "First badge"}); err != nil {
		t.Fatalf("Put(badge) error = %v", err)
	}

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminservice.RegisterHandlers(app, adminservice.NewStrictHandler(&adminService{PetSpecies: petSpecies, Badges: badges}, nil))

	rec := serveAdminAsset(app, http.MethodGet, "/pet-species", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /pet-species status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"rabbit"`) || !strings.Contains(rec.Body.String(), `"has_next":false`) {
		t.Fatalf("GET /pet-species body = %s", rec.Body.String())
	}

	rec = serveAdminAsset(app, http.MethodGet, "/badges", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /badges status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"id":"founder"`) || !strings.Contains(rec.Body.String(), `"description":"First badge"`) {
		t.Fatalf("GET /badges body = %s", rec.Body.String())
	}
}

func TestBusinessResourceHandlersReportAssetErrors(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminservice.RegisterHandlers(app, adminservice.NewStrictHandler(&adminService{}, nil))

	assertAdminAssetError(t, serveAdminAsset(app, http.MethodGet, "/pet-species", ""), http.StatusInternalServerError, "PET_SPECIES_SERVICE_NOT_CONFIGURED")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodPut, "/pet-species/rabbit/pixa", "not-pixa"), http.StatusInternalServerError, "PET_SPECIES_SERVICE_NOT_CONFIGURED")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodGet, "/pet-species/rabbit/pixa", ""), http.StatusInternalServerError, "PET_SPECIES_SERVICE_NOT_CONFIGURED")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodGet, "/badges", ""), http.StatusInternalServerError, "BADGE_SERVICE_NOT_CONFIGURED")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodPut, "/badges/founder/icon", "icon"), http.StatusInternalServerError, "BADGE_SERVICE_NOT_CONFIGURED")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodGet, "/badges/founder/icon", ""), http.StatusInternalServerError, "BADGE_SERVICE_NOT_CONFIGURED")

	ctx := context.Background()
	petSpecies := &petspecies.Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
	}
	if _, err := petSpecies.Put(ctx, "rabbit", apitypes.PetSpeciesSpec{Name: "Rabbit"}); err != nil {
		t.Fatalf("Put(pet species) error = %v", err)
	}
	badges := &badge.Server{
		Store:  kv.NewMemory(nil),
		Assets: objectstore.Dir(t.TempDir()),
	}
	if _, err := badges.Put(ctx, "founder", apitypes.BadgeSpec{Name: "Founder"}); err != nil {
		t.Fatalf("Put(badge) error = %v", err)
	}
	app = fiber.New(fiber.Config{DisableStartupMessage: true})
	adminservice.RegisterHandlers(app, adminservice.NewStrictHandler(&adminService{PetSpecies: petSpecies, Badges: badges}, nil))

	assertAdminAssetError(t, serveAdminAsset(app, http.MethodPut, "/pet-species/rabbit/pixa", "not-pixa"), http.StatusBadRequest, "INVALID_ASSET")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodGet, "/pet-species/rabbit/pixa", ""), http.StatusBadRequest, "INVALID_ASSET")
	assertAdminAssetError(t, serveAdminAsset(app, http.MethodGet, "/badges/founder/icon", ""), http.StatusBadRequest, "INVALID_ASSET")
}

func serveAdminAsset(app *fiber.App, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(rec, req)
	return rec
}

func assertAdminAssetError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var got apitypes.ErrorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(error response) error = %v body=%s", err, rec.Body.String())
	}
	if got.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q body=%s", got.Error.Code, wantCode, rec.Body.String())
	}
}

func mustPeerServiceResource(t *testing.T, raw string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := json.Unmarshal([]byte(raw), &resource); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return resource
}

func testAdminPixa(width, height uint16, clips []string) []byte {
	const (
		headerSize     = 40
		clipEntrySize  = 56
		frameEntrySize = 16
	)
	paletteOffset := headerSize
	clipOffset := paletteOffset + 2
	frameOffset := clipOffset + len(clips)*clipEntrySize
	payloadOffset := frameOffset + frameEntrySize
	data := make([]byte, payloadOffset)
	copy(data[0:4], "PIXA")
	binary.LittleEndian.PutUint16(data[4:6], 1)
	binary.LittleEndian.PutUint16(data[6:8], headerSize)
	binary.LittleEndian.PutUint16(data[8:10], width)
	binary.LittleEndian.PutUint16(data[10:12], height)
	binary.LittleEndian.PutUint16(data[12:14], 1)
	binary.LittleEndian.PutUint16(data[14:16], uint16(len(clips)))
	binary.LittleEndian.PutUint32(data[16:20], 1)
	binary.LittleEndian.PutUint32(data[20:24], uint32(paletteOffset))
	binary.LittleEndian.PutUint32(data[24:28], uint32(clipOffset))
	binary.LittleEndian.PutUint32(data[28:32], uint32(frameOffset))
	binary.LittleEndian.PutUint32(data[32:36], uint32(payloadOffset))
	for i, name := range clips {
		base := clipOffset + i*clipEntrySize
		copy(data[base:base+32], name)
		binary.LittleEndian.PutUint32(data[base+40:base+44], 1)
	}
	return data
}
