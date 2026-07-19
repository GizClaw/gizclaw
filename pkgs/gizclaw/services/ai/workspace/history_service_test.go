package workspace

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestServerWorkspaceHistoryServiceReadPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	ctx := context.Background()
	seedWorkspace(t, srv, "demo0001")

	entry, err := srv.AppendWorkspaceHistory(ctx, " demo0001 ", AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "hello",
		Asset: &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	list, err := srv.ListWorkspaceHistory(ctx, "demo0001", apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != entry.ID || list.Items[0].Text != "hello" {
		t.Fatalf("ListWorkspaceHistory() = %+v", list)
	}

	got, err := srv.GetWorkspaceHistory(ctx, "demo0001", entry.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceHistory() error = %v", err)
	}
	if got.ID != entry.ID || got.Text != "hello" {
		t.Fatalf("GetWorkspaceHistory() = %+v", got)
	}

	r, err := srv.ReadWorkspaceHistoryAsset(ctx, "demo0001", entry.Assets[0].Name)
	if err != nil {
		t.Fatalf("ReadWorkspaceHistoryAsset() error = %v", err)
	}
	data, err := io.ReadAll(r)
	if closeErr := r.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(data) != "opus" {
		t.Fatalf("asset data = %q", data)
	}
}

func TestServerAppendWorkspaceHistoryBumpsLastActiveAt(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	ctx := context.Background()
	seedWorkspace(t, srv, "demo0001")

	before, err := getWorkspace(ctx, srv.Store, "demo0001")
	if err != nil {
		t.Fatalf("getWorkspace(before) error = %v", err)
	}
	entryCreatedAt := before.LastActiveAt.Add(2 * time.Hour)
	entry, err := srv.AppendWorkspaceHistory(ctx, "demo0001", AppendHistoryRequest{
		CreatedAt: entryCreatedAt,
		Name:      "assistant",
		Text:      "hello",
		Type:      "agent",
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	if !entry.CreatedAt.Equal(entryCreatedAt) {
		t.Fatalf("entry created_at = %s, want %s", entry.CreatedAt, entryCreatedAt)
	}
	after, err := getWorkspace(ctx, srv.Store, "demo0001")
	if err != nil {
		t.Fatalf("getWorkspace(after) error = %v", err)
	}
	if !after.LastActiveAt.Equal(entryCreatedAt) {
		t.Fatalf("last_active_at = %s, want %s", after.LastActiveAt, entryCreatedAt)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updated_at = %s, want unchanged %s", after.UpdatedAt, before.UpdatedAt)
	}

	if _, err := srv.AppendWorkspaceHistory(ctx, "demo0001", AppendHistoryRequest{
		CreatedAt: before.LastActiveAt.Add(time.Minute),
		Name:      "older",
		Text:      "old",
		Type:      "agent",
	}); err != nil {
		t.Fatalf("AppendWorkspaceHistory(older) error = %v", err)
	}
	got, err := getWorkspace(ctx, srv.Store, "demo0001")
	if err != nil {
		t.Fatalf("getWorkspace(final) error = %v", err)
	}
	if !got.LastActiveAt.Equal(entryCreatedAt) {
		t.Fatalf("last_active_at after older append = %s, want unchanged %s", got.LastActiveAt, entryCreatedAt)
	}
}

func TestServerWorkspaceHistoryServiceErrors(t *testing.T) {
	t.Parallel()

	var nilServer *Server
	if _, err := nilServer.AppendWorkspaceHistory(context.Background(), "demo0001", AppendHistoryRequest{}); err == nil || !strings.Contains(err.Error(), "nil server") {
		t.Fatalf("nil AppendWorkspaceHistory() error = %v", err)
	}

	srv := newTestServer(t)
	if _, err := srv.AppendWorkspaceHistory(context.Background(), "", AppendHistoryRequest{}); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("AppendWorkspaceHistory(empty) error = %v", err)
	}
	seedWorkspace(t, srv, "demo0001")
	if _, err := srv.AppendWorkspaceHistory(context.Background(), "demo0001", AppendHistoryRequest{}); err == nil || !strings.Contains(err.Error(), "runtime store") {
		t.Fatalf("AppendWorkspaceHistory(no runtime store) error = %v", err)
	}
}

func seedWorkspace(t *testing.T, srv *Server, name string) {
	t.Helper()

	seedWorkflow(t, srv, "workflow-1")
	body := adminhttp.WorkspaceUpsert{Name: name, WorkflowName: "workflow-1"}
	resp, err := srv.CreateWorkspace(context.Background(), adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkspace200JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() response = %#v", resp)
	}
}
