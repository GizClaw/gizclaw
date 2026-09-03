package gizclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/gameplay"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peertelemetry"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/resourcemanager"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/metrics"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
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

func TestAdminSocialErrorMapsFriendGroupFull(t *testing.T) {
	for _, test := range []struct {
		err     error
		code    string
		message string
	}{
		{err: friendgroup.ErrFriendGroupFull, code: "FRIEND_GROUP_FULL", message: friendgroup.ErrFriendGroupFull.Error()},
	} {
		status, body := adminSocialError(fmt.Errorf("wrapped: %w", test.err))
		if status != http.StatusConflict || body.Error.Code != test.code || body.Error.Message != test.message {
			t.Fatalf("adminSocialError(%v) = %d/%+v", test.err, status, body)
		}
	}
}

func TestAdminPeerFenceAllowsReadsAndRepeatedDeleteOnly(t *testing.T) {
	keyPair, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peers := &runtimepeer.Server{Store: mustBadgerInMemory(t, nil)}
	if _, err := peers.SavePeer(t.Context(), apitypes.Peer{
		PublicKey: keyPair.Public.String(), Role: apitypes.PeerRoleClient,
		Status: apitypes.PeerRegistrationStatusActive, Device: apitypes.DeviceInfo{},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := peers.DeletePeer(t.Context(), adminhttp.DeletePeerRequestObject{PublicKey: keyPair.Public.String()}); err != nil {
		t.Fatal(err)
	}
	service := &adminService{Peers: peers}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(service.fenceDeletedPeerBusiness)
	app.All("/*", func(ctx *fiber.Ctx) error { return ctx.SendStatus(http.StatusNoContent) })

	for _, tc := range []struct {
		method string
		path   string
		want   int
	}{
		{method: http.MethodGet, path: "/peers/" + keyPair.Public.String(), want: http.StatusNoContent},
		{method: http.MethodGet, path: "/peers/" + keyPair.Public.String() + "/pets", want: http.StatusConflict},
		{method: http.MethodPost, path: "/peers/" + keyPair.Public.String() + "/friends", want: http.StatusConflict},
		{method: http.MethodDelete, path: "/peers/" + keyPair.Public.String(), want: http.StatusNoContent},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Fatalf("%s %s status = %d, want %d", tc.method, tc.path, resp.StatusCode, tc.want)
		}
	}
}

func TestAdminSocialPutRejectsBodyIDMismatch(t *testing.T) {
	service := &adminService{Contacts: &contact.Server{}, FriendGroups: &friendgroup.Server{}}
	ctx := t.Context()

	contactResponse, err := service.PutContact(ctx, adminhttp.PutContactRequestObject{
		OwnerPublicKey: "owner", Id: "contact", Body: &adminhttp.AdminContactPutRequest{Id: "other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := contactResponse.(adminhttp.PutContact400JSONResponse); !ok || response.Error.Code != "RESOURCE_ID_MISMATCH" {
		t.Fatalf("PutContact() = %#v", contactResponse)
	}

	groupResponse, err := service.PutFriendGroup(ctx, adminhttp.PutFriendGroupRequestObject{
		Id: "group", Body: &adminhttp.AdminFriendGroupPutRequest{Id: "other"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := groupResponse.(adminhttp.PutFriendGroup400JSONResponse); !ok || response.Error.Code != "RESOURCE_ID_MISMATCH" {
		t.Fatalf("PutFriendGroup() = %#v", groupResponse)
	}

	memberResponse, err := service.PutFriendGroupMember(ctx, adminhttp.PutFriendGroupMemberRequestObject{
		Id: "group", PublicKey: "peer", Body: &adminhttp.AdminFriendGroupMemberPutRequest{Id: "group:other", Role: rpcapi.FriendGroupMemberRoleMember},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := memberResponse.(adminhttp.PutFriendGroupMember400JSONResponse); !ok || response.Error.Code != "RESOURCE_ID_MISMATCH" {
		t.Fatalf("PutFriendGroupMember() = %#v", memberResponse)
	}

	expiresAt := time.Now().Add(time.Hour)
	tokenResponse, err := service.PutFriendGroupInviteToken(ctx, adminhttp.PutFriendGroupInviteTokenRequestObject{
		Id: "group", Body: &adminhttp.AdminFriendGroupInviteTokenPutRequest{Id: "other", InviteToken: "token", ExpiresAt: expiresAt},
	})
	if err != nil {
		t.Fatal(err)
	}
	if response, ok := tokenResponse.(adminhttp.PutFriendGroupInviteToken400JSONResponse); !ok || response.Error.Code != "RESOURCE_ID_MISMATCH" {
		t.Fatalf("PutFriendGroupInviteToken() = %#v", tokenResponse)
	}
}

func TestAdminFriendGroupMemberObjectUsesCanonicalMembershipID(t *testing.T) {
	peer := "peer:/blue"
	item := adminFriendGroupMemberObject("group:/red", rpcapi.FriendGroupMemberObject{PeerPublicKey: &peer})
	want := customid.MembershipName("group:/red", peer)
	if item.Id != want {
		t.Fatalf("adminFriendGroupMemberObject().Id = %q, want %q", item.Id, want)
	}
}

func TestAdminServiceDeletePeerPetUsesGameplayLifecycle(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := sqlx.Open("sqlite", "file:admin-delete-peer-pet?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	workspaces := &adminGameplayWorkspaceService{}
	runtime := &gameplay.Runtime{DB: db, Workspaces: workspaces}
	if err := runtime.Migration(ctx); err != nil {
		t.Fatalf("Migration() error = %v", err)
	}
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := db.ExecContext(ctx, `INSERT INTO gameplay_pets (
		owner_public_key, id, name, runtime_profile_id, pet_def_id, display_name, workspace_id,
		stats_json, progression_json, lifecycle, died_at, state_settled_at, last_active_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"peer-a", "pet-a", "pet-name", "runtime-profile-default", "petdef-a", "Pet A", "id-pet-pet-a",
		`{"life":100,"health":100,"satiety":100,"hygiene":100,"mood":100,"energy":100}`, `{"experience":0,"level":1}`, "alive", nil, now, now, now, now,
	); err != nil {
		t.Fatalf("insert pet: %v", err)
	}

	service := &adminService{Gameplay: runtime}
	resp, err := service.DeletePeerPet(ctx, adminhttp.DeletePeerPetRequestObject{PublicKey: "peer-a", Id: "pet-a"})
	if err != nil {
		t.Fatalf("DeletePeerPet() error = %v", err)
	}
	deleted, ok := resp.(adminhttp.DeletePeerPet200JSONResponse)
	if !ok || deleted.Id != "pet-a" {
		t.Fatalf("DeletePeerPet() response = %#v", resp)
	}
	if len(workspaces.deleted) != 0 {
		t.Fatalf("DeletePeerPet deleted bound Workspace = %#v", workspaces.deleted)
	}
	var pendingCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_pending_deletions WHERE kind = 'pet' AND owner_public_key = ? AND resource_id = ?`, "peer-a", "pet-a").Scan(&pendingCount); err != nil {
		t.Fatalf("query Pet pending deletion: %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("Pet pending deletion count = %d, want 1", pendingCount)
	}
	resp, err = service.DeletePeerPet(ctx, adminhttp.DeletePeerPetRequestObject{PublicKey: "peer-a", Id: "pet-a"})
	if err != nil {
		t.Fatalf("DeletePeerPet(retry) error = %v", err)
	}
	if _, ok := resp.(adminhttp.DeletePeerPet200JSONResponse); !ok {
		t.Fatalf("DeletePeerPet(retry) response = %#v", resp)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM gameplay_pending_deletions WHERE kind = 'pet' AND owner_public_key = ? AND resource_id = ?`, "peer-a", "pet-a").Scan(&pendingCount); err != nil {
		t.Fatalf("query repeated Pet pending deletion: %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("repeated Pet pending deletion count = %d, want 1", pendingCount)
	}
}

type adminGameplayWorkspaceService struct {
	deleted []string
}

func newTestFriendServer(store kv.Store) *friend.Server {
	return &friend.Server{
		Friends:    store,
		Workspaces: &adminGameplayWorkspaceService{},
		SFUURL:     "wss://sfu.test",
	}
}

func (s *adminGameplayWorkspaceService) CreateSystemWorkspace(_ context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	system := true
	return apitypes.Workspace{Id: "id-" + body.Name, Name: body.Name, WorkflowId: body.WorkflowId, System: &system}, true, nil
}

func (s *adminGameplayWorkspaceService) DeleteSystemWorkspace(_ context.Context, name string) (apitypes.Workspace, error) {
	s.deleted = append(s.deleted, name)
	system := true
	return apitypes.Workspace{Name: name, System: &system}, nil
}

func (s *adminGameplayWorkspaceService) GetWorkspaceByName(_ context.Context, name string) (apitypes.Workspace, error) {
	return apitypes.Workspace{Id: "id-" + name, Name: name}, nil
}

func (s *adminGameplayWorkspaceService) GetWorkspace(_ context.Context, request adminhttp.GetWorkspaceRequestObject) (adminhttp.GetWorkspaceResponseObject, error) {
	return adminhttp.GetWorkspace200JSONResponse(apitypes.Workspace{Id: request.Id}), nil
}

func (s *adminGameplayWorkspaceService) RetireSystemWorkspace(_ context.Context, name string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	return apitypes.Workspace{Name: name}, nil
}

func (s *adminGameplayWorkspaceService) RetireSystemWorkspaceByID(_ context.Context, id string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	return apitypes.Workspace{Id: id}, nil
}

func (s *adminGameplayWorkspaceService) GetRetiredSystemWorkspace(_ context.Context, _ string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func (s *adminGameplayWorkspaceService) GetRetiredSystemWorkspaceByID(_ context.Context, _ string, _ socialutil.SFUWorkspaceKind, _ string) (apitypes.Workspace, error) {
	return apitypes.Workspace{}, kv.ErrNotFound
}

func TestAdminServiceResourceMethodsHandleValidationAndManagerErrors(t *testing.T) {
	resource := mustPeerServiceResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-id"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)
	service := &adminService{}

	writable := mustPeerServiceWritableResource(t, resource)
	applyResp, err := service.ApplyResource(context.Background(), adminhttp.ApplyResourceRequestObject{JSONBody: &writable})
	if err != nil {
		t.Fatalf("ApplyResource() error = %v", err)
	}
	if got, ok := applyResp.(adminhttp.ApplyResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("ApplyResource() response = %T %+v", applyResp, applyResp)
	}

	getResp, err := service.GetResource(context.Background(), adminhttp.GetResourceRequestObject{
		Kind: apitypes.ResourceKindCredential,
		Id:   "minimax-main",
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
		Id:       "minimax-id",
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
		Id:       "minimax-id",
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
		Id:   "minimax-main",
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
		"metadata": {"id": "minimax-id"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)

	if err := validateResourcePathMatch(resource, apitypes.ResourceKindCredential, "minimax-id"); err != nil {
		t.Fatalf("validateResourcePathMatch() error = %v", err)
	}
	if err := validateResourcePathMatch(resource, apitypes.ResourceKindWorkspace, "minimax-id"); err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("validateResourcePathMatch(kind mismatch) error = %v", err)
	}
	if err := validateResourcePathMatch(resource, apitypes.ResourceKindCredential, "other"); err == nil || !strings.Contains(err.Error(), "metadata.id") {
		t.Fatalf("validateResourcePathMatch(id mismatch) error = %v", err)
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

func TestAdminServicePutResourceComparesTransportDecodedPathID(t *testing.T) {
	resource := mustPeerServiceResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax/team%main"},
		"spec": {
			"provider": "minimax",
			"body": {"api_key": "secret"}
		}
	}`)
	response, err := (&adminService{}).PutResource(t.Context(), adminhttp.PutResourceRequestObject{
		Kind:     apitypes.ResourceKindCredential,
		Id:       "minimax/team%main",
		JSONBody: &resource,
	})
	if err != nil {
		t.Fatalf("PutResource() error = %v", err)
	}
	if got, ok := response.(adminhttp.PutResource500JSONResponse); !ok || got.Error.Code != "RESOURCE_MANAGER_NOT_CONFIGURED" {
		t.Fatalf("PutResource() response = %T %+v", response, response)
	}
}

func TestResource200JSONResponseSerializesResourceUnion(t *testing.T) {
	resource := mustPeerServiceResource(t, `{
		"apiVersion": "gizclaw.admin/v1alpha1",
		"kind": "Credential",
		"metadata": {"id": "minimax-main"},
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

	friendService := newTestFriendServer(kv.NewMemory(nil))
	groupStore := kv.NewMemory(nil)
	groupService := &friendgroup.Server{
		Groups:            groupStore,
		InviteTokens:      groupStore,
		Members:           groupStore,
		Belongs:           groupStore,
		RelationshipStore: groupStore,
		Workspaces:        &adminGameplayWorkspaceService{},
		SFUURL:            "wss://sfu.test",
		Now:               func() time.Time { return time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC) },
		NewID:             func() string { return "group-a" },
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{Friends: friendService, FriendGroups: groupService}, nil))

	rec := serveAdminJSON(app, http.MethodPost, "/social/friends", `{"id":"peer-a:peer-b","owner_public_key":"peer-a","peer_public_key":"peer-b"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST friend status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"owner_public_key":"peer-a"`) || !strings.Contains(rec.Body.String(), `"peer_public_key":"peer-b"`) || !strings.Contains(rec.Body.String(), `"workspace_id":"id-social-direct-`) {
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
	friendService.Workspaces = &adminGameplayWorkspaceService{}
	rec = serveAdminAsset(app, http.MethodDelete, "/social/friends/peer-a/peer-a:peer-b", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE friend status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friends/peer-a/peer-a:peer-b", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET deleted friend status = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups", `{"id":"group-a","name":"Room","owner_public_key":"peer-a"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST group status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"created_by_peer_public_key":"peer-a"`) || strings.Contains(rec.Body.String(), "my_role") {
		t.Fatalf("admin-created group owner projection is invalid: %s", rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups", `{"id":"literal%2Fgroup","name":"Escaped Room","owner_public_key":"peer-c"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST escaped group status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friend-groups/literal%252Fgroup/members", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Escaped Room"`) {
		t.Fatalf("GET escaped group members status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups", `{"id":"slash/group","name":"Slash Room","owner_public_key":"peer-d"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST slash group status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friend-groups/slash%2Fgroup/members", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Slash Room"`) {
		t.Fatalf("GET slash group members status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups", `{"id":"plus+group","name":"Plus Room","owner_public_key":"peer-e"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST plus group status = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friend-groups/plus+group/members", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Plus Room"`) {
		t.Fatalf("GET plus group members status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminAsset(app, http.MethodGet, "/social/friend-groups/group-a/members", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Room"`) || !strings.Contains(rec.Body.String(), `"role":"owner"`) {
		t.Fatalf("GET owner member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups/group-a/members", `{"id":"group-a:peer-b","name":"Room B","peer_public_key":"peer-b","role":"member"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPost, "/social/friend-groups/group-a/members", `{"id":"group-a:peer-b","name":"Room B","peer_public_key":"peer-b","role":"member"}`)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), `"code":"FRIEND_GROUP_MEMBER_ALREADY_EXISTS"`) {
		t.Fatalf("POST duplicate member status=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = serveAdminJSON(app, http.MethodPut, "/social/friend-groups/group-a/members/peer-a", `{"id":"group-a:peer-a","role":"admin"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"role":"admin"`) {
		t.Fatalf("PUT member status=%d body=%s", rec.Code, rec.Body.String())
	}
	expiresAt := time.Date(2026, 6, 13, 0, 5, 0, 0, time.UTC).Format(time.RFC3339)
	rec = serveAdminJSON(app, http.MethodPut, "/social/friend-groups/group-a/invite-token", `{"id":"group-a","invite_token":"token-a","expires_at":"`+expiresAt+`"}`)
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
	groupService.Workspaces = &adminGameplayWorkspaceService{}
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
					Name:            "history-a",
					Type:            apitypes.PeerRunHistoryEntryTypeGear,
					GearId:          new("gear-a"),
					ActorName:       "transcript",
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

func TestAdminServerLogsStreamsLogAndEndEvents(t *testing.T) {
	t.Parallel()

	timeNs := "1783403541016789000"
	logs := &fakeServerLogQuery{
		entries: []apitypes.ServerLogEntry{{
			TimeMs:  1783403541016,
			TimeNs:  &timeNs,
			Level:   "ERROR",
			Message: "agenthost failed",
			Source:  "gizclaw",
			Path:    "slog",
			Fields:  map[string]string{"error": "boom"},
		}},
		end: apitypes.ServerLogStreamEnd{Count: 1, HasNext: true, NextCursor: new("cursor-1")},
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{ServerLogs: logs}, nil))

	rec := serveAdminAsset(app, http.MethodGet, "/logs/stream?filter=level:ERROR&start_time_ms=1783400000000&end_time_ms=1783403600000&limit=10&order=desc", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET logs status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: log\n",
		`"time_ms":1783403541016`,
		`"level":"ERROR"`,
		"event: end\n",
		`"next_cursor":"cursor-1"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if logs.req.Filter != "level:ERROR" || logs.req.StartTimeMs != 1783400000000 || logs.req.EndTimeMs != 1783403600000 || logs.req.Limit != 10 || logs.req.Order != ServerLogOrderDesc {
		t.Fatalf("request = %#v", logs.req)
	}
}

func TestAdminServerLogsErrors(t *testing.T) {
	t.Parallel()

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{}, nil))
	rec := serveAdminAsset(app, http.MethodGet, "/logs/stream?start_time_ms=1000&end_time_ms=2000", "")
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "LOG_QUERY_NOT_CONFIGURED") {
		t.Fatalf("unconfigured status=%d body=%s", rec.Code, rec.Body.String())
	}

	app = fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{ServerLogs: &fakeServerLogQuery{}}, nil))
	rec = serveAdminAsset(app, http.MethodGet, "/logs/stream?end_time_ms=2000", "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_LOG_QUERY") {
		t.Fatalf("bad request status=%d body=%s", rec.Code, rec.Body.String())
	}

	app = fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{ServerLogs: &fakeServerLogQuery{err: ServerLogBackendError(errors.New("denied"))}}, nil))
	rec = serveAdminAsset(app, http.MethodGet, "/logs/stream?start_time_ms=1000&end_time_ms=2000", "")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "LOG_QUERY_BACKEND_ERROR") {
		t.Fatalf("pre-stream backend status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminServerLogsPostStartErrorUsesSSE(t *testing.T) {
	t.Parallel()

	logs := &fakeServerLogQuery{
		entries: []apitypes.ServerLogEntry{{
			TimeMs:  1000,
			Level:   "INFO",
			Message: "first",
			Source:  "gizclaw",
			Path:    "slog",
			Fields:  map[string]string{},
		}},
		err: ServerLogBackendError(errors.New("search failed")),
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{ServerLogs: logs}, nil))

	rec := serveAdminAsset(app, http.MethodGet, "/logs/stream?start_time_ms=1000&end_time_ms=2000", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("post-start status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: log\n") || !strings.Contains(body, "event: error\n") || !strings.Contains(body, "LOG_QUERY_BACKEND_ERROR") {
		t.Fatalf("post-start body = %s", body)
	}
}

func TestWaitFirstServerLogEventPrefersBufferedEventOverDone(t *testing.T) {
	t.Parallel()

	events := make(chan serverLogEvent, 1)
	done := make(chan error, 1)
	events <- serverLogEvent{name: "log", data: apitypes.ServerLogEntry{Message: "first"}}
	done <- ServerLogBackendError(errors.New("search failed"))

	event, err, hasFirst, donePending := waitFirstServerLogEvent(context.Background(), events, done)
	if !hasFirst || event.name != "log" || err != nil || !donePending {
		t.Fatalf("waitFirstServerLogEvent() event=%#v err=%v hasFirst=%v donePending=%v", event, err, hasFirst, donePending)
	}
}

func TestAdminServerLogsAllowsCursorOnlyContinuation(t *testing.T) {
	t.Parallel()

	logs := &fakeServerLogQuery{end: apitypes.ServerLogStreamEnd{}}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{ServerLogs: logs}, nil))

	rec := serveAdminAsset(app, http.MethodGet, "/logs/stream?cursor=opaque&limit=2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("cursor status=%d body=%s", rec.Code, rec.Body.String())
	}
	if logs.req.Cursor != "opaque" || logs.req.Limit != 2 {
		t.Fatalf("request = %#v", logs.req)
	}
	if logs.req.StartTimeSet || logs.req.EndTimeSet || logs.req.FilterSet || logs.req.OrderSet {
		t.Fatalf("unexpected explicit query fields = %#v", logs.req)
	}

	rec = serveAdminAsset(app, http.MethodGet, "/logs/stream?cursor=opaque&filter=", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("explicit empty filter status=%d body=%s", rec.Code, rec.Body.String())
	}
	if logs.req.Filter != "*" || !logs.req.FilterSet {
		t.Fatalf("explicit empty filter request = %#v", logs.req)
	}
}

func TestAdminPeerTelemetryLatest(t *testing.T) {
	t.Parallel()

	peer := adminTelemetryTestPeer()
	now := time.Now().UTC()
	store := metrics.NewMemoryStore()
	if err := store.Append(context.Background(), []metrics.Sample{{
		Name:      peertelemetry.MetricBatteryPercent,
		Labels:    map[string]string{"peer_id": peer.String()},
		Timestamp: now.Add(-time.Minute),
		Value:     91,
	}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{
		PeerTelemetry: &peertelemetry.AdminService{
			Metrics: store,
			Now: func() time.Time {
				return now
			},
		},
	}, nil))

	rec := serveAdminAsset(app, http.MethodGet, "/peers/"+peer.String()+"/telemetry/latest?fields=battery.percent", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("latest status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response apitypes.PeerTelemetryLatestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("latest JSON error = %v", err)
	}
	if response.PeerPublicKey != peer.String() || len(response.Values) != 1 {
		t.Fatalf("latest response = %#v", response)
	}
	if got := response.Values[0]; got.Field != apitypes.PeerTelemetryFieldBatteryPercent || got.Value != 91 {
		t.Fatalf("latest value = %#v", got)
	}
}

func TestAdminPeerTelemetryRange(t *testing.T) {
	t.Parallel()

	peer := adminTelemetryTestPeer()
	start := time.Now().UTC().Add(-10 * time.Minute).Truncate(time.Second)
	store := metrics.NewMemoryStore()
	if err := store.Append(context.Background(), []metrics.Sample{
		{Name: peertelemetry.MetricGNSSLatitude, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start, Value: 37.1},
		{Name: peertelemetry.MetricGNSSLatitude, Labels: map[string]string{"peer_id": peer.String()}, Timestamp: start.Add(time.Minute), Value: 37.2},
	}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{
		PeerTelemetry: &peertelemetry.AdminService{Metrics: store},
	}, nil))

	path := "/peers/" + peer.String() + "/telemetry?field=gnss.latitude&start_time_ms=" +
		strconv.FormatInt(start.UnixMilli(), 10) + "&end_time_ms=" +
		strconv.FormatInt(start.Add(time.Minute).UnixMilli(), 10) + "&step_ms=60000&limit=2&order=desc"
	rec := serveAdminAsset(app, http.MethodGet, path, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("range status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response apitypes.PeerTelemetryRangeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("range JSON error = %v", err)
	}
	if len(response.Points) != 2 || response.Points[0].Value != 37.2 || response.Points[1].Value != 37.1 {
		t.Fatalf("range points = %#v", response.Points)
	}
}

func TestAdminPeerTelemetryErrors(t *testing.T) {
	t.Parallel()

	peer := adminTelemetryTestPeer()
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{}, nil))
	rec := serveAdminAsset(app, http.MethodGet, "/peers/"+peer.String()+"/telemetry/latest", "")
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "TELEMETRY_QUERY_NOT_CONFIGURED") {
		t.Fatalf("unconfigured status=%d body=%s", rec.Code, rec.Body.String())
	}

	app = fiber.New(fiber.Config{DisableStartupMessage: true})
	adminhttp.RegisterHandlers(app, adminhttp.NewStrictHandler(&adminService{
		PeerTelemetry: &peertelemetry.AdminService{Metrics: metrics.NewMemoryStore()},
	}, nil))
	rec = serveAdminAsset(app, http.MethodGet, "/peers/"+peer.String()+"/telemetry/latest?fields=bad", "")
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "INVALID_TELEMETRY_QUERY") {
		t.Fatalf("bad field status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func adminTelemetryTestPeer() giznet.PublicKey {
	var peer giznet.PublicKey
	for i := range peer {
		peer[i] = byte(255 - i)
	}
	return peer
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

//go:fix inline
func adminTestStringPtr(value string) *string {
	return new(value)
}

type fakeAdminWorkspaceHistory struct {
	list  apitypes.PeerRunHistoryListResponse
	entry workspace.HistoryEntry
	audio []byte
}

type fakeServerLogQuery struct {
	req     ServerLogStreamRequest
	entries []apitypes.ServerLogEntry
	end     apitypes.ServerLogStreamEnd
	err     error
}

func (f *fakeServerLogQuery) StreamServerLogs(_ context.Context, req ServerLogStreamRequest, emit func(apitypes.ServerLogEntry) error) (apitypes.ServerLogStreamEnd, error) {
	f.req = req
	for _, entry := range f.entries {
		if err := emit(entry); err != nil {
			return apitypes.ServerLogStreamEnd{}, err
		}
	}
	if f.err != nil {
		return apitypes.ServerLogStreamEnd{}, f.err
	}
	return f.end, nil
}

func (f *fakeAdminWorkspaceHistory) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
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

func mustPeerServiceWritableResource(t *testing.T, resource apitypes.Resource) adminhttp.ApplyResourceJSONRequestBody {
	t.Helper()
	data, err := json.Marshal(resource)
	if err != nil {
		t.Fatalf("marshal writable resource: %v", err)
	}
	var writable adminhttp.ApplyResourceJSONRequestBody
	if err := json.Unmarshal(data, &writable); err != nil {
		t.Fatalf("unmarshal writable resource: %v", err)
	}
	return writable
}
