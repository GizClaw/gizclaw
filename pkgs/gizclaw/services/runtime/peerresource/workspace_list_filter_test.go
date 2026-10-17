package peerresource

import (
	"encoding/base64"
	"slices"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestWorkspaceListSystemFilter(t *testing.T) {
	server, _, ordinary, system := workspaceListFilterFixture(t)
	all := append(append([]string{}, ordinary...), system...)
	slices.Sort(all)
	for _, tc := range []struct {
		name   string
		params rpcapi.WorkspaceListRequest
		want   []string
	}{
		{"omitted", rpcapi.WorkspaceListRequest{}, all},
		{"ordinary", rpcapi.WorkspaceListRequest{System: new(false)}, ordinary},
		{"system including shared SFU", rpcapi.WorkspaceListRequest{System: new(true)}, system},
		{"prefix", rpcapi.WorkspaceListRequest{Prefix: new("social-")}, sortedWorkspaceListNames(ordinary[4:], system)},
		{"prefix and ordinary", rpcapi.WorkspaceListRequest{Prefix: new(" list- "), System: new(false)}, []string{"list-c", "list-e", "list-g"}},
		{"prefix and system", rpcapi.WorkspaceListRequest{Prefix: new("social-"), System: new(true)}, system},
		{"empty prefix", rpcapi.WorkspaceListRequest{Prefix: new("  ")}, all},
		{"no prefix matches", rpcapi.WorkspaceListRequest{Prefix: new("missing")}, nil},
		{"no ordinary matches", rpcapi.WorkspaceListRequest{Prefix: new(system[1]), System: new(false)}, nil},
		{"no system matches", rpcapi.WorkspaceListRequest{Prefix: new("list-c"), System: new(true)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := workspaceListFilterPage(t, server, tc.params)
			assertWorkspaceListNames(t, page, tc.want)
			if page.HasNext || page.NextCursor != nil {
				t.Fatalf("complete page has continuation: %+v", page)
			}
			if page.RuntimeProfileName != "default" || page.RuntimeProfileRevision != "r1" {
				t.Fatalf("profile projection = %+v", page)
			}
			for _, item := range page.Items {
				if tc.params.System != nil && item.System != *tc.params.System {
					t.Fatalf("Workspace %q system = %v, want %v", item.Name, item.System, *tc.params.System)
				}
				if item.Name == "list-g" && item.Available {
					t.Fatal("Workspace with a removed Workflow should remain visible but unavailable")
				}
			}
		})
	}

	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceDeleteRequest(rpcapi.WorkspaceDeleteRequest{Name: "a-system"}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkspaceDelete(t.Context(), &rpcapi.RPCRequest{Id: "delete-system", Params: &payload})
	if response.Error == nil || response.Result != nil {
		t.Fatalf("system Workspace generic deletion = %+v, want rejection", response)
	}
	assertWorkspaceListNames(t, workspaceListFilterPage(t, server, rpcapi.WorkspaceListRequest{System: new(true)}), system)
}

func TestWorkspaceListSystemFilterEmpty(t *testing.T) {
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{Caller: giznet.PublicKey{1}, Workspaces: workspacetest.New(t), RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	for _, system := range []*bool{nil, new(false), new(true)} {
		page := workspaceListFilterPage(t, server, rpcapi.WorkspaceListRequest{System: system, Limit: new(1)})
		if len(page.Items) != 0 || page.HasNext || page.NextCursor != nil {
			t.Fatalf("empty list with system %v = %+v", system, page)
		}
	}
}

func TestWorkspaceListFiltersBeforePagination(t *testing.T) {
	server, _, ordinary, system := workspaceListFilterFixture(t)
	all := append(append([]string{}, ordinary...), system...)
	slices.Sort(all)
	for _, tc := range []struct {
		name   string
		params rpcapi.WorkspaceListRequest
		want   []string
	}{
		{"all", rpcapi.WorkspaceListRequest{}, all},
		{"ordinary", rpcapi.WorkspaceListRequest{System: new(false)}, ordinary},
		{"system", rpcapi.WorkspaceListRequest{System: new(true)}, system},
		{"prefix ordinary", rpcapi.WorkspaceListRequest{Prefix: new("list-"), System: new(false)}, []string{"list-c", "list-e", "list-g"}},
		{"mixed prefix ordinary", rpcapi.WorkspaceListRequest{Prefix: new("social-"), System: new(false)}, ordinary[4:]},
		{"prefix system", rpcapi.WorkspaceListRequest{Prefix: new("social-"), System: new(true)}, system},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, limit := range []int{1, 2, len(tc.want), len(tc.want) + 1} {
				params := tc.params
				params.Limit = &limit
				var got []string
				for range len(tc.want) + 1 {
					page := workspaceListFilterPage(t, server, params)
					remaining := len(tc.want) - len(got)
					assertWorkspaceListNames(t, page, tc.want[len(got):len(got)+min(limit, remaining)])
					for _, item := range page.Items {
						got = append(got, item.Name)
					}
					if page.HasNext != (len(got) < len(tc.want)) || (page.NextCursor != nil) != page.HasNext {
						t.Fatalf("limit %d continuation after %v: %+v", limit, got, page)
					}
					if !page.HasNext {
						break
					}
					if *page.NextCursor == "" || (params.Cursor != nil && *page.NextCursor == *params.Cursor) {
						t.Fatal("cursor did not advance")
					}
					params.Cursor = page.NextCursor
				}
				if !slices.Equal(got, tc.want) {
					t.Fatalf("limit %d pages = %v, want %v", limit, got, tc.want)
				}
			}
		})
	}
}

func TestWorkspaceListCursorBindsFilters(t *testing.T) {
	server, profile, _, _ := workspaceListFilterFixture(t)
	for _, tc := range []struct {
		name string
		from rpcapi.WorkspaceListRequest
		to   rpcapi.WorkspaceListRequest
	}{
		{"add false", rpcapi.WorkspaceListRequest{Prefix: new("list-")}, rpcapi.WorkspaceListRequest{Prefix: new("list-"), System: new(false)}},
		{"remove false", rpcapi.WorkspaceListRequest{System: new(false)}, rpcapi.WorkspaceListRequest{}},
		{"add true", rpcapi.WorkspaceListRequest{}, rpcapi.WorkspaceListRequest{System: new(true)}},
		{"remove true", rpcapi.WorkspaceListRequest{System: new(true)}, rpcapi.WorkspaceListRequest{}},
		{"false to true", rpcapi.WorkspaceListRequest{System: new(false)}, rpcapi.WorkspaceListRequest{System: new(true)}},
		{"true to false", rpcapi.WorkspaceListRequest{System: new(true)}, rpcapi.WorkspaceListRequest{System: new(false)}},
		{"narrow prefix retaining anchor", rpcapi.WorkspaceListRequest{Prefix: new("list-"), System: new(false)}, rpcapi.WorkspaceListRequest{Prefix: new("list-c"), System: new(false)}},
		{"add prefix retaining anchor", rpcapi.WorkspaceListRequest{System: new(false)}, rpcapi.WorkspaceListRequest{Prefix: new("list-"), System: new(false)}},
		{"remove prefix retaining anchor", rpcapi.WorkspaceListRequest{Prefix: new("list-"), System: new(false)}, rpcapi.WorkspaceListRequest{System: new(false)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.from.Limit = new(1)
			first := workspaceListFilterPage(t, server, tc.from)
			if first.NextCursor == nil {
				t.Fatal("fixture must produce a continuation")
			}
			tc.to.Cursor = first.NextCursor
			response := workspaceListFilterResponse(t, server, tc.to)
			if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted || response.Result != nil {
				t.Fatalf("changed selector response = %+v, want ABORTED", response)
			}
		})
	}

	params := rpcapi.WorkspaceListRequest{Prefix: new(" list- "), System: new(false), Limit: new(1)}
	first := workspaceListFilterPage(t, server, params)
	params.Cursor, params.Prefix, params.Limit = first.NextCursor, new("list-"), new(2)
	assertWorkspaceListNames(t, workspaceListFilterPage(t, server, params), []string{"list-e", "list-g"})
	profile.Revision = "r2"
	response := workspaceListFilterResponse(t, server, params)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted {
		t.Fatalf("changed profile response = %+v, want ABORTED", response)
	}
}

func TestWorkspaceListCursorBoundaries(t *testing.T) {
	server, _, _, _ := workspaceListFilterFixture(t)
	for _, cursor := range []string{
		"not-base64!",
		base64.RawURLEncoding.EncodeToString([]byte("r1")),
		base64.RawURLEncoding.EncodeToString([]byte("r1\x00missing")),
	} {
		response := workspaceListFilterResponse(t, server, rpcapi.WorkspaceListRequest{Cursor: &cursor})
		if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted {
			t.Fatalf("invalid cursor %q response = %+v, want ABORTED", cursor, response)
		}
	}
	// Existing unfiltered revision/name cursors continue to work.
	legacy := base64.RawURLEncoding.EncodeToString([]byte("r1\x00list-c"))
	assertWorkspaceListNames(t, workspaceListFilterPage(t, server, rpcapi.WorkspaceListRequest{Cursor: &legacy, Limit: new(1)}), []string{"list-e"})

	params := rpcapi.WorkspaceListRequest{System: new(false), Limit: new(1)}
	first := workspaceListFilterPage(t, server, params)
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceDeleteRequest(rpcapi.WorkspaceDeleteRequest{Name: first.Items[0].Name}); err != nil {
		t.Fatal(err)
	}
	response := server.handleWorkspaceDelete(t.Context(), &rpcapi.RPCRequest{Id: "delete", Params: &payload})
	if response.Error != nil {
		t.Fatalf("delete cursor anchor: %+v", response)
	}
	params.Cursor = first.NextCursor
	response = workspaceListFilterResponse(t, server, params)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted {
		t.Fatalf("deleted anchor response = %+v, want ABORTED", response)
	}
}

func workspaceListFilterFixture(t *testing.T) (*Server, *apitypes.RuntimeProfile, []string, []string) {
	t.Helper()
	ctx := t.Context()
	workspaces := workspacetest.New(t)
	workflows := workflowtest.New(t)
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	if err := workflows.EnsureBuiltinWorkflows(ctx); err != nil {
		t.Fatal(err)
	}
	workspaces.Workflows = workflows
	caller := giznet.PublicKey{1}
	friendStore := kv.NewMemory(nil)
	t.Cleanup(func() { _ = friendStore.Close() })
	friends := &friend.Server{Friends: friendStore, Workspaces: workspaces, SFUURL: "wss://sfu.test"}
	var system []string
	for i := range 5 {
		// Include both caller-owned and other-owned SFU Workspaces, all
		// discovered through real Friend relationships.
		owner, peer := giznet.PublicKey{byte(i + 2)}.String(), caller.String()
		if i == 0 {
			owner, peer = peer, owner
		}
		relation, err := friends.AdminCreateFriend(ctx, owner, peer)
		if err != nil {
			t.Fatal(err)
		}
		system = append(system, socialutil.StringValue(relation.WorkspaceName))
	}
	slices.Sort(system)
	// These ordinary names interleave with SFU names without changing their
	// lifecycle classification. The first SFU precedes both matching users.
	ordinary := []string{"list-c", "list-e", "list-g", "sfu-user", system[0] + "-user", system[2] + "-user"}
	now := time.Date(2026, time.October, 17, 0, 0, 0, 0, time.UTC)
	for _, name := range append(append([]string{}, ordinary...), "a-system", "list-foreign") {
		item := apitypes.Workspace{
			Id: name, Name: name, WorkflowId: "canonical-workflow", OwnerPublicKey: new(caller.String()),
			CreatedAt: now, LastActiveAt: now, UpdatedAt: now,
		}
		if name == "a-system" {
			item.System = new(true)
		} else if name != "list-c" {
			item.System = new(false)
		}
		if name == "list-g" {
			item.WorkflowId = "removed-workflow"
		}
		if name == "list-foreign" {
			item.OwnerPublicKey = new(giznet.PublicKey{9}.String())
		}
		workspacetest.Seed(t, workspaces, item)
	}
	profile := runtimeProfileWithWorkspaceAlias("r1")
	return &Server{Caller: caller, Workspaces: workspaces, Workflows: workflows, Friends: friends, RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}, &profile, ordinary, system
}

func sortedWorkspaceListNames(groups ...[]string) []string {
	names := slices.Concat(groups...)
	slices.Sort(names)
	return names
}

func workspaceListFilterResponse(t *testing.T, server *Server, params rpcapi.WorkspaceListRequest) *rpcapi.RPCResponse {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceListRequest(params); err != nil {
		t.Fatal(err)
	}
	response, handled, err := server.Dispatch(t.Context(), &rpcapi.RPCRequest{Id: "list", Method: rpcapi.RPCMethodServerWorkspaceList, Params: &payload})
	if err != nil || !handled || response == nil {
		t.Fatalf("list dispatch = %+v, handled=%v, err=%v", response, handled, err)
	}
	return response
}

func workspaceListFilterPage(t *testing.T, server *Server, params rpcapi.WorkspaceListRequest) rpcapi.WorkspaceListResponse {
	t.Helper()
	response := workspaceListFilterResponse(t, server, params)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("list response error = %+v, result = %+v", response.Error, response.Result)
	}
	page, err := response.Result.AsWorkspaceListResponse()
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func assertWorkspaceListNames(t *testing.T, page rpcapi.WorkspaceListResponse, want []string) {
	t.Helper()
	names := make([]string, 0, len(page.Items))
	for _, item := range page.Items {
		names = append(names, item.Name)
	}
	if !slices.Equal(names, want) {
		t.Fatalf("list names = %v, want %v", names, want)
	}
}
