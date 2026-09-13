package peerresource

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/logstore"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestWorkspaceHistoryListHonorsOrderAndTimeRange(t *testing.T) {
	ctx := context.Background()
	workflows := workflowtest.New(t)
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	objects, err := objectstore.NewRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	logDB, err := sqlx.Open("sqlite", filepath.Join(t.TempDir(), "history.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	logDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = logDB.Close() })
	records, err := logstore.NewSQLStoreWithDB(logDB, "workspace_history")
	if err != nil {
		t.Fatal(err)
	}
	runtimes := workspace.NewObjectRuntimeStore(objects, records, objects)
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller:     giznet.PublicKey{1},
		Workspaces: &workspace.Server{DB: workspacetest.New(t).DB, Workflows: workflows, RuntimeStore: runtimes},
		Workflows:  workflows,
		RuntimeProfile: func() *apitypes.RuntimeProfile {
			return &profile
		},
	}
	callWorkspaceCreate(t, ctx, server, rpcapi.WorkspaceCreateBody{
		Name: "history-list", Collection: "story-teller", WorkflowName: "journey",
	})
	created, rpcErr := server.ResolveAccessibleWorkspace(ctx, "history-list")
	if rpcErr != nil {
		t.Fatalf("resolve Workspace: %#v", rpcErr)
	}
	runtime, err := runtimes.GetWorkspaceRuntime(ctx, created.Id)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for day := range 3 {
		entry, err := runtime.History.Append(ctx, workspace.AppendHistoryRequest{Type: "agent", Name: "assistant", Text: "hello", CreatedAt: base.AddDate(0, 0, day)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, entry.ID)
	}
	list := func(request rpcapi.WorkspaceHistoryListRequest) *rpcapi.RPCResponse {
		t.Helper()
		request.WorkspaceName = "history-list"
		var payload rpcapi.RPCPayload
		if err := payload.FromWorkspaceHistoryListRequest(request); err != nil {
			t.Fatal(err)
		}
		response, handled, err := server.Dispatch(ctx, &rpcapi.RPCRequest{Id: "history", Method: rpcapi.RPCMethodServerWorkspaceHistoryList, Params: &payload})
		if err != nil || !handled {
			t.Fatalf("history list handled=%v error=%v", handled, err)
		}
		return response
	}
	names := func(response *rpcapi.RPCResponse) []string {
		t.Helper()
		if response.Error != nil || response.Result == nil {
			t.Fatalf("history list error = %#v", response.Error)
		}
		page, err := response.Result.AsWorkspaceHistoryListResponse()
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			out = append(out, item.Name)
		}
		return out
	}
	desc := rpcapi.WorkspaceHistoryListRequestOrder("desc")
	asc := rpcapi.WorkspaceHistoryListRequestOrder("asc")
	start := base.AddDate(0, 0, 1).UnixMilli()
	end := base.AddDate(0, 0, 2).UnixMilli()

	if got := names(list(rpcapi.WorkspaceHistoryListRequest{Order: &desc, EndTimeMs: &end})); !slices.Equal(got, []string{ids[1], ids[0]}) {
		t.Fatalf("desc before end = %v", got)
	}
	if got := names(list(rpcapi.WorkspaceHistoryListRequest{Order: &asc, StartTimeMs: &start})); !slices.Equal(got, []string{ids[1], ids[2]}) {
		t.Fatalf("asc from start = %v", got)
	}
	if got := names(list(rpcapi.WorkspaceHistoryListRequest{Order: &asc, StartTimeMs: &start, EndTimeMs: &end})); !slices.Equal(got, []string{ids[1]}) {
		t.Fatalf("asc within [start, end) = %v", got)
	}
	if got := names(list(rpcapi.WorkspaceHistoryListRequest{Order: &asc, Cursor: &ids[0]})); !slices.Equal(got, []string{ids[1], ids[2]}) {
		t.Fatalf("asc after item cursor = %v", got)
	}
	for name, request := range map[string]rpcapi.WorkspaceHistoryListRequest{
		"inverted range": {StartTimeMs: &end, EndTimeMs: &start},
		"negative end":   {EndTimeMs: new(int64(-1))},
	} {
		response := list(request)
		if response.Error == nil || response.Error.Code != rpcapi.StatusCodeInvalidArgument {
			t.Fatalf("%s response = %#v, want INVALID_ARGUMENT", name, response.Error)
		}
	}
}
