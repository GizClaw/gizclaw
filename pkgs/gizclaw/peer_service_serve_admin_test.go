package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestAdminServiceApplyResourceRequiresBody(t *testing.T) {
	t.Parallel()

	resp, err := (&adminService{}).ApplyResource(context.Background(), adminhttp.ApplyResourceRequestObject{})
	if err != nil {
		t.Fatalf("ApplyResource() error = %v", err)
	}
	got, ok := resp.(adminhttp.ApplyResource400JSONResponse)
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
			"body": {"api_key": "secret"}
		}
	}`)
	service := &adminService{}

	applyResp, err := service.ApplyResource(context.Background(), adminhttp.ApplyResourceRequestObject{JSONBody: &resource})
	if err != nil {
		t.Fatalf("ApplyResource() error = %v", err)
	}
	if got, ok := applyResp.(adminhttp.ApplyResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("ApplyResource() response = %T %+v", applyResp, applyResp)
	}

	getResp, err := service.GetResource(context.Background(), adminhttp.GetResourceRequestObject{
		Kind: apitypes.ResourceKindCredential,
		Name: "minimax-main",
	})
	if err != nil {
		t.Fatalf("GetResource() error = %v", err)
	}
	if got, ok := getResp.(adminhttp.GetResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("GetResource() response = %T %+v", getResp, getResp)
	}

	putResp, err := service.PutResource(context.Background(), adminhttp.PutResourceRequestObject{})
	if err != nil {
		t.Fatalf("PutResource(nil body) error = %v", err)
	}
	if got, ok := putResp.(adminhttp.PutResource400JSONResponse); !ok || got.Error.Code != "INVALID_RESOURCE" {
		t.Fatalf("PutResource(nil body) response = %T %+v", putResp, putResp)
	}

	putResp, err = service.PutResource(context.Background(), adminhttp.PutResourceRequestObject{
		Kind:     apitypes.ResourceKindWorkspace,
		Name:     "minimax-main",
		JSONBody: &resource,
	})
	if err != nil {
		t.Fatalf("PutResource(path mismatch) error = %v", err)
	}
	if got, ok := putResp.(adminhttp.PutResource400JSONResponse); !ok || got.Error.Code != "INVALID_RESOURCE_PATH" {
		t.Fatalf("PutResource(path mismatch) response = %T %+v", putResp, putResp)
	}

	putResp, err = service.PutResource(context.Background(), adminhttp.PutResourceRequestObject{
		Kind:     apitypes.ResourceKindCredential,
		Name:     "minimax-main",
		JSONBody: &resource,
	})
	if err != nil {
		t.Fatalf("PutResource(manager error) error = %v", err)
	}
	if got, ok := putResp.(adminhttp.PutResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("PutResource(manager error) response = %T %+v", putResp, putResp)
	}

	deleteResp, err := service.DeleteResource(context.Background(), adminhttp.DeleteResourceRequestObject{
		Kind: apitypes.ResourceKindCredential,
		Name: "minimax-main",
	})
	if err != nil {
		t.Fatalf("DeleteResource() error = %v", err)
	}
	if got, ok := deleteResp.(adminhttp.DeleteResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
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

func TestAdminSocialHandlersUseDomainServices(t *testing.T) {
	t.Parallel()

	friendService := &friend.Server{Friends: kv.NewMemory(nil)}
	groupStore := kv.NewMemory(nil)
	groupService := &friendgroup.Server{
		Groups:        groupStore,
		InviteTokens:  groupStore,
		Members:       groupStore,
		Belongs:       groupStore,
		Messages:      groupStore,
		MessageAssets: objectstore.Dir(t.TempDir()),
		Now:           func() time.Time { return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) },
		NewID:         func() string { return "group-a" },
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{Friends: friendService, FriendGroups: groupService}, nil))

	rec := serveAdminJSON(app, http.MethodPost, "/social/friends", `{"owner_public_key":"peer-a","peer_public_key":"peer-b"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST friend status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"owner_public_key":"peer-a"`) || !strings.Contains(rec.Body.String(), `"peer_public_key":"peer-b"`) || !strings.Contains(rec.Body.String(), `"workspace_name":"social-direct-`) {
		t.Fatalf("POST friend body = %s", rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friends?limit=1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"owner_public_key":"peer-a"`) || !strings.Contains(rec.Body.String(), `"has_next":true`) {
		t.Fatalf("GET social friends status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friends/peer-a/peer-a:peer-b", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"owner_public_key":"peer-a"`) {
		t.Fatalf("GET social friend status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/peers/peer-b/friends", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"peer_public_key":"peer-a"`) {
		t.Fatalf("GET peer-b friends status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodDelete, "/social/friends/peer-a/peer-a:peer-b", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE friend status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friends/peer-a/peer-a:peer-b", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET deleted friend status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups", `{"name":"Room"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST group status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "created_by_peer_public_key") || strings.Contains(rec.Body.String(), "my_role") {
		t.Fatalf("admin-created group should not include peer role fields: %s", rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups/group-a/members", `{"peer_public_key":"peer-a","role":"owner"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Fatalf("POST owner member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPut, "/social/friend-groups/group-a/members/peer-a", `{"role":"admin"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("PUT member status=%d body=%s", rec.Code, rec.Body.String())
	}
	expiresAt := time.Date(2026, 6, 13, 0, 5, 0, 0, time.UTC).Format(time.RFC3339)
	rec = serveAdminJSON(app, http.MethodPut, "/social/friend-groups/group-a/invite-token", `{"invite_token":"token-a","expires_at":"`+expiresAt+`"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"invite_token":"token-a"`) {
		t.Fatalf("PUT token status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friend-groups/group-a/invite-token", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"invite_token":"token-a"`) {
		t.Fatalf("GET token status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodDelete, "/social/friend-groups/group-a/invite-token", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE token status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodDelete, "/social/friend-groups/group-a/members/peer-a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodDelete, "/social/friend-groups/group-a", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE group status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminWorkspaceHistoryHandlersServePersistedHistoryAndOggAudio(t *testing.T) {
	t.Parallel()

	history := &fakeAdminWorkspaceHistory{
		list: apitypes.PeerRunHistoryListResponse{
			Available: true,
			Items: []apitypes.PeerRunHistoryEntry{
				{
					Id:              "history-a",
					Type:            apitypes.PeerRunHistoryEntryTypeGear,
					GearId:          adminTestStringPtr("gear-a"),
					Name:            "transcript",
					Text:            "hello",
					CreatedAt:       time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC),
					ReplayAvailable: true,
				},
			},
		},
		entry: workspace.HistoryEntry{
			ID:              "history-a",
			Type:            "gear",
			GearID:          "gear-a",
			Name:            "transcript",
			Text:            "hello",
			CreatedAt:       time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC),
			ReplayAvailable: true,
		},
		audio: []byte("ogg-opus"),
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{WorkspaceAdminService: history}, nil))

	rec := serveAdminAsset(app, http.MethodGet, "/workspaces/workspace-a/history?order=asc&limit=1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"id":"history-a"`) {
		t.Fatalf("GET history status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/workspaces/workspace-a/history/history-a", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"text":"hello"`) {
		t.Fatalf("GET history entry status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/workspaces/workspace-a/history/history-a/audio.ogg", "")
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "audio/ogg" || rec.Body.String() != "ogg-opus" {
		t.Fatalf("GET history audio status=%d content-type=%q body=%q", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
	}
}

func TestAdminSocialErrorClassifiesServiceConfigurationFailures(t *testing.T) {
	t.Parallel()

	for _, err := range []error{
		errors.New("workspace: runtime store is required"),
		errors.New("workspace: history store is required"),
		errors.New("workspace history: object store is required"),
	} {
		status, body := adminSocialError(err)
		if status != http.StatusInternalServerError || body.Error.Code != "SOCIAL_SERVICE_ERROR" {
			t.Fatalf("adminSocialError(%v) = %d %#v, want 500 SOCIAL_SERVICE_ERROR", err, status, body)
		}
	}
}

func serveAdminAsset(app *fiber.App, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	rec := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(rec, req)
	return rec
}

func serveAdminJSON(app *fiber.App, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	fiberHTTPHandler(app).ServeHTTP(rec, req)
	return rec
}

func adminTestStringPtr(value string) *string {
	return &value
}

type fakeAdminWorkspaceHistory struct {
	list  apitypes.PeerRunHistoryListResponse
	entry workspace.HistoryEntry
	audio []byte
}

func (f *fakeAdminWorkspaceHistory) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	return nil, nil
}

func (f *fakeAdminWorkspaceHistory) CreateWorkspace(context.Context, adminhttp.CreateWorkspaceRequestObject) (adminhttp.CreateWorkspaceResponseObject, error) {
	return nil, nil
}

func (f *fakeAdminWorkspaceHistory) DeleteWorkspace(context.Context, adminhttp.DeleteWorkspaceRequestObject) (adminhttp.DeleteWorkspaceResponseObject, error) {
	return nil, nil
}

func (f *fakeAdminWorkspaceHistory) GetWorkspace(context.Context, adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return nil, nil
}

func (f *fakeAdminWorkspaceHistory) PutWorkspace(context.Context, adminhttp.PutWorkspaceRequestObject) (adminhttp.PutWorkspaceResponseObject, error) {
	return nil, nil
}

func (f *fakeAdminWorkspaceHistory) AdminListWorkspaceHistory(context.Context, string, apitypes.PeerRunHistoryListRequest) (apitypes.PeerRunHistoryListResponse, error) {
	return f.list, nil
}

func (f *fakeAdminWorkspaceHistory) AdminGetWorkspaceHistory(context.Context, string, string) (workspace.HistoryEntry, error) {
	return f.entry, nil
}

func (f *fakeAdminWorkspaceHistory) AdminReadWorkspaceHistoryAudio(context.Context, string, string) (io.ReadCloser, int64, error) {
	return io.NopCloser(bytes.NewReader(f.audio)), int64(len(f.audio)), nil
}

func mustPeerServiceResource(t *testing.T, raw string) apitypes.Resource {
	t.Helper()

	var resource apitypes.Resource
	if err := json.Unmarshal([]byte(raw), &resource); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return resource
}
