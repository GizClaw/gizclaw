package gizclaw

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
)

// seedDeviceWorkspaces binds the story-teller profile to the fixture owner and
// seeds its saves: two resolvable aliases, one whose Workflow binding is gone,
// a system Workspace, and a same-named Workspace owned by another Peer.
func seedDeviceWorkspaces(t *testing.T, f *deviceHTTPFixture) time.Time {
	t.Helper()
	seedRuntimeProfile(t, f, f.owner, "h106-tiga", storyTellerSpec())
	created := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	owner := f.owner.String()
	seed := func(id, name, workflowID, collection string, ownerKey string, system bool) {
		item := apitypes.Workspace{
			Id: id, Name: name, WorkflowId: workflowID, OwnerPublicKey: new(ownerKey), System: new(system),
			CreatedAt: created, UpdatedAt: created.Add(time.Hour), LastActiveAt: created.Add(2 * time.Hour),
		}
		if collection != "" {
			item.Labels = &map[string]string{"collection": collection}
		}
		workspacetest.Seed(t, f.workspaces, item)
	}
	seed("ws-aesop", "aesop-save", "secret-workflow-aesop", "story-teller", owner, false)
	seed("ws-riddle", "riddle-save", "secret-workflow-riddle", "games", owner, false)
	seed("ws-orphan", "orphan-save", "secret-workflow-gone", "story-teller", owner, false)
	seed("ws-pet", "pet", "secret-workflow-pet", "", owner, true)
	seed("ws-foreign", "aesop-save", "secret-workflow-aesop", "story-teller", "foreign-peer", false)
	return created
}

func listDeviceWorkspaceNames(t *testing.T, f *deviceHTTPFixture, query string) []string {
	t.Helper()
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/workspaces"+query, "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET workspaces%s status = %d body=%s", query, response.Code, response.Body.String())
	}
	items := decodeJSON[[]peerhttp.DeviceWorkspace](t, response)
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	slices.Sort(names)
	return names
}

func TestListDeviceWorkspacesIdentifiesWorkflowsByAlias(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	created := seedDeviceWorkspaces(t, f)

	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/workspaces", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET workspaces status = %d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, secret := range []string{"secret-", "workflow_id", "foreign"} {
		if strings.Contains(body, secret) {
			t.Fatalf("response leaks %q: %s", secret, body)
		}
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(body), &raw); err != nil {
		t.Fatal(err)
	}
	for _, item := range raw {
		for key := range item {
			if !slices.Contains([]string{"id", "name", "collection", "workflow_name", "available", "system", "created_at", "updated_at", "last_active_at"}, key) {
				t.Fatalf("unexpected DeviceWorkspace key %q in %s", key, body)
			}
		}
	}

	var items []peerhttp.DeviceWorkspace
	if err := json.Unmarshal([]byte(body), &items); err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]peerhttp.DeviceWorkspace, len(items))
	for _, item := range items {
		byID[item.Id] = item
	}
	if len(byID) != 4 {
		t.Fatalf("workspaces = %#v, want the four owned Workspaces", items)
	}
	aesop := byID["ws-aesop"]
	if aesop.Name != "aesop-save" || !aesop.Available || aesop.System ||
		aesop.Collection == nil || *aesop.Collection != "story-teller" ||
		aesop.WorkflowName == nil || *aesop.WorkflowName != "story.aesop" ||
		!aesop.CreatedAt.Equal(created) || !aesop.UpdatedAt.Equal(created.Add(time.Hour)) || !aesop.LastActiveAt.Equal(created.Add(2*time.Hour)) {
		t.Fatalf("aesop = %#v", aesop)
	}
	if riddle := byID["ws-riddle"]; riddle.WorkflowName == nil || *riddle.WorkflowName != "game.riddle" || riddle.Collection == nil || *riddle.Collection != "games" {
		t.Fatalf("riddle = %#v", riddle)
	}
	// A dangling binding keeps its collection but, like Peer RPC, loses the alias.
	if orphan := byID["ws-orphan"]; orphan.Available || orphan.WorkflowName != nil || orphan.Collection == nil || *orphan.Collection != "story-teller" {
		t.Fatalf("orphan = %#v", orphan)
	}
	if pet := byID["ws-pet"]; !pet.System || pet.Available || pet.WorkflowName != nil || pet.Collection != nil {
		t.Fatalf("pet = %#v", pet)
	}
}

func TestListDeviceWorkspacesFilters(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedDeviceWorkspaces(t, f)

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", []string{"aesop-save", "orphan-save", "pet", "riddle-save"}},
		{"?collection=story-teller", []string{"aesop-save", "orphan-save"}},
		{"?workflow_name=story.aesop", []string{"aesop-save"}},
		{"?collection=story-teller&workflow_name=story.aesop", []string{"aesop-save"}},
		{"?collection=games&workflow_name=story.aesop", []string{}},
		{"?collection=missing", []string{}},
		// The Admin Workflow ID is not a filter value.
		{"?workflow_name=secret-workflow-aesop", []string{}},
	} {
		if got := listDeviceWorkspaceNames(t, f, tc.query); !slices.Equal(got, tc.want) {
			t.Fatalf("GET workspaces%s = %v, want %v", tc.query, got, tc.want)
		}
	}
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/workspaces?collection=", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("empty collection filter status = %d body=%s", response.Code, response.Body.String())
	}
}

func TestListDeviceWorkspacesFollowsCurrentProfile(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedDeviceWorkspaces(t, f)
	seedRuntimeProfile(t, f, f.owner, "h106-next", apitypes.RuntimeProfileSpec{
		Workflows: apitypes.RuntimeProfileWorkflows{Collections: apitypes.RuntimeProfileWorkflowCollections{
			"story-teller": {"story.fables": runtimeProfileHTTPBinding("secret-workflow-aesop", "Fables")},
		}},
	})
	response := f.do(t, http.MethodGet, "/gizclaw/v1/device/workspaces?collection=story-teller", "")
	if response.Code != http.StatusOK {
		t.Fatalf("GET workspaces status = %d body=%s", response.Code, response.Body.String())
	}
	for _, item := range decodeJSON[[]peerhttp.DeviceWorkspace](t, response) {
		if item.Id == "ws-aesop" && (item.WorkflowName == nil || *item.WorkflowName != "story.fables" || !item.Available) {
			t.Fatalf("rebound aesop = %#v, want the alias of the current profile", item)
		}
	}
}

func TestDeleteDeviceWorkspace(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedDeviceWorkspaces(t, f)

	response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/workspaces/ws-aesop", "")
	if response.Code != http.StatusAccepted || response.Body.Len() != 0 {
		t.Fatalf("DELETE status = %d body=%s", response.Code, response.Body.String())
	}
	if got := listDeviceWorkspaceNames(t, f, ""); !slices.Equal(got, []string{"orphan-save", "pet", "riddle-save"}) {
		t.Fatalf("workspaces after delete = %v", got)
	}
	response = f.do(t, http.MethodGet, "/gizclaw/v1/device/workspaces/ws-aesop/history", "")
	if response.Code != http.StatusNotFound || errorCode(t, response) != "WORKSPACE_NOT_FOUND" {
		t.Fatalf("history of pending Workspace status = %d body=%s", response.Code, response.Body.String())
	}
	if _, err := f.workspaces.GetWorkspaceByName(ownership.WithOwner(context.Background(), f.owner.String()), "aesop-save"); !errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
		t.Fatalf("GetWorkspaceByName(pending) error = %v, want pending deletion", err)
	}

	for _, tc := range []struct {
		id     string
		status int
		code   string
	}{
		{"ws-aesop", http.StatusNotFound, "WORKSPACE_NOT_FOUND"},   // already pending deletion
		{"ws-foreign", http.StatusNotFound, "WORKSPACE_NOT_FOUND"}, // another owner's Workspace
		{"ws-absent", http.StatusNotFound, "WORKSPACE_NOT_FOUND"},
		{"ws-pet", http.StatusConflict, "SYSTEM_WORKSPACE_DELETE_FORBIDDEN"},
	} {
		response := f.do(t, http.MethodDelete, "/gizclaw/v1/device/workspaces/"+tc.id, "")
		if response.Code != tc.status || errorCode(t, response) != tc.code {
			t.Fatalf("DELETE %s status = %d body=%s, want %d %s", tc.id, response.Code, response.Body.String(), tc.status, tc.code)
		}
	}
	foreign, err := f.workspaces.GetAvailableWorkspaceByID(context.Background(), "ws-foreign")
	if err != nil || foreign.Name != "aesop-save" {
		t.Fatalf("foreign Workspace = %#v err=%v", foreign, err)
	}
}

func TestDeviceWorkspaceHandlersMapFailures(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedDeviceWorkspaces(t, f)
	service := &PeerService{manager: f.manager, public: &peerHTTP{Workspaces: f.workspaces}}
	handler := &peerHTTP{DeviceReads: service.deviceReadsForAPIKey}
	ctx := peerhttp.WithCallerPublicKey(context.Background(), f.owner)

	// The binding can disappear after the middleware validated the owner.
	if err := f.manager.RuntimeProfiles.DeleteOwnerProfileBinding(context.Background(), f.owner.String()); err != nil {
		t.Fatal(err)
	}
	response, err := handler.ListDeviceWorkspaces(ctx, peerhttp.ListDeviceWorkspacesRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if forbidden, ok := response.(peerhttp.ListDeviceWorkspaces403JSONResponse); !ok || forbidden.Error.Code != "API_KEY_OWNER_UNAVAILABLE" {
		t.Fatalf("lost binding response = %#v", response)
	}

	if response, _ := (&peerHTTP{}).ListDeviceWorkspaces(ctx, peerhttp.ListDeviceWorkspacesRequestObject{}); !isResponseType[peerhttp.ListDeviceWorkspaces500JSONResponse](response) {
		t.Fatalf("unconfigured list response = %#v", response)
	}
	if response, _ := (&peerHTTP{}).DeleteDeviceWorkspace(ctx, peerhttp.DeleteDeviceWorkspaceRequestObject{WorkspaceId: "ws-aesop"}); !isResponseType[peerhttp.DeleteDeviceWorkspace500JSONResponse](response) {
		t.Fatalf("unconfigured delete response = %#v", response)
	}
	unwired := &peerHTTP{DeviceReads: (&PeerService{manager: f.manager}).deviceReadsForAPIKey}
	if response, _ := unwired.DeleteDeviceWorkspace(ctx, peerhttp.DeleteDeviceWorkspaceRequestObject{WorkspaceId: "ws-aesop"}); !isResponseType[peerhttp.DeleteDeviceWorkspace500JSONResponse](response) {
		t.Fatalf("missing Workspace service response = %#v", response)
	}
	if response, _ := handler.ListDeviceWorkspaces(context.Background(), peerhttp.ListDeviceWorkspacesRequestObject{}); !isResponseType[peerhttp.ListDeviceWorkspaces401JSONResponse](response) {
		t.Fatalf("ownerless list response = %#v", response)
	}
	if response, _ := handler.DeleteDeviceWorkspace(context.Background(), peerhttp.DeleteDeviceWorkspaceRequestObject{WorkspaceId: "ws-aesop"}); !isResponseType[peerhttp.DeleteDeviceWorkspace401JSONResponse](response) {
		t.Fatalf("ownerless delete response = %#v", response)
	}
}

func TestDeleteDeviceWorkspaceRejectsUnavailableOwner(t *testing.T) {
	f := newDeviceHTTPFixture(t)
	seedDeviceWorkspaces(t, f)
	f.workspaces.PeerAvailability = func(context.Context, string) error { return workspace.ErrPeerPendingDeletion }
	service := &PeerService{manager: f.manager, public: &peerHTTP{Workspaces: f.workspaces}}
	handler := &peerHTTP{DeviceReads: service.deviceReadsForAPIKey}
	response, err := handler.DeleteDeviceWorkspace(peerhttp.WithCallerPublicKey(context.Background(), f.owner), peerhttp.DeleteDeviceWorkspaceRequestObject{WorkspaceId: "ws-aesop"})
	if err != nil {
		t.Fatal(err)
	}
	if conflict, ok := response.(peerhttp.DeleteDeviceWorkspace409JSONResponse); !ok || conflict.Error.Code != "PEER_PENDING_DELETION" {
		t.Fatalf("response = %#v", response)
	}
}
