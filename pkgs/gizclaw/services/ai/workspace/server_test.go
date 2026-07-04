package workspace

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestServerWorkspacesCRUD(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	createBody := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_name": "workflow-1",
		"parameters": {"mode": "demo"}
	}`)

	createResp, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := createResp.(adminservice.CreateWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResp)
	}
	if created.Name != "alpha001" || created.WorkflowName != "workflow-1" {
		t.Fatalf("CreateWorkspace() workspace = %#v", created)
	}
	if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() || created.LastActiveAt.IsZero() {
		t.Fatalf("CreateWorkspace() timestamps = %#v", created)
	}
	if !created.LastActiveAt.Equal(created.CreatedAt) {
		t.Fatalf("CreateWorkspace() last_active_at = %s, want created_at %s", created.LastActiveAt, created.CreatedAt)
	}
	if len(runtime.prepared) != 1 || runtime.prepared[0] != "alpha001" {
		t.Fatalf("runtime prepared after create = %#v", runtime.prepared)
	}

	listResp, err := srv.ListWorkspaces(ctx, adminservice.ListWorkspacesRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	listed, ok := listResp.(adminservice.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].Name != "alpha001" || listed.HasNext {
		t.Fatalf("ListWorkspaces() = %#v", listed)
	}

	getResp, err := srv.GetWorkspace(ctx, adminservice.GetWorkspaceRequestObject{Name: "alpha001"})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	got, ok := getResp.(adminservice.GetWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkspace() response = %#v", getResp)
	}
	if got.Name != "alpha001" {
		t.Fatalf("GetWorkspace() = %#v", got)
	}

	updateBody := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_name": "workflow-1",
		"parameters": {"mode": "updated"}
	}`)
	putResp, err := srv.PutWorkspace(ctx, adminservice.PutWorkspaceRequestObject{
		Name: "alpha001",
		Body: &updateBody,
	})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	updated, ok := putResp.(adminservice.PutWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("PutWorkspace() response = %#v", putResp)
	}
	if updated.CreatedAt.IsZero() || updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("PutWorkspace() timestamps = %#v", updated)
	}
	if !updated.LastActiveAt.Equal(created.LastActiveAt) {
		t.Fatalf("PutWorkspace() last_active_at = %s, want unchanged %s", updated.LastActiveAt, created.LastActiveAt)
	}
	if len(runtime.prepared) != 2 || runtime.prepared[1] != "alpha001" {
		t.Fatalf("runtime prepared after put = %#v", runtime.prepared)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminservice.DeleteWorkspaceRequestObject{Name: "alpha001"})
	if err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 1 || runtime.deleted[0] != "alpha001" {
		t.Fatalf("runtime deleted = %#v", runtime.deleted)
	}

	getAfterDelete, err := srv.GetWorkspace(ctx, adminservice.GetWorkspaceRequestObject{Name: "alpha001"})
	if err != nil {
		t.Fatalf("GetWorkspace() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminservice.GetWorkspace404JSONResponse); !ok {
		t.Fatalf("GetWorkspace() after delete response = %#v", getAfterDelete)
	}
}

func TestServerWorkspaceLastActiveBackfillsLegacyRecords(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	createdAt := time.Date(2026, 6, 22, 8, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	legacy := map[string]any{
		"name":          "legacy",
		"workflow_name": "workflow-1",
		"created_at":    createdAt.Format(time.RFC3339Nano),
		"updated_at":    updatedAt.Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := srv.Store.Set(ctx, workspaceKey("legacy"), data); err != nil {
		t.Fatalf("seed legacy workspace: %v", err)
	}

	got, err := getWorkspace(ctx, srv.Store, "legacy")
	if err != nil {
		t.Fatalf("getWorkspace() error = %v", err)
	}
	if !got.LastActiveAt.Equal(createdAt) {
		t.Fatalf("getWorkspace() last_active_at = %s, want created_at %s", got.LastActiveAt, createdAt)
	}

	listResp, err := srv.ListWorkspaces(ctx, adminservice.ListWorkspacesRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	listed, ok := listResp.(adminservice.ListWorkspaces200JSONResponse)
	if !ok || len(listed.Items) != 1 {
		t.Fatalf("ListWorkspaces() response = %#v", listResp)
	}
	if !listed.Items[0].LastActiveAt.Equal(createdAt) {
		t.Fatalf("ListWorkspaces() last_active_at = %s, want created_at %s", listed.Items[0].LastActiveAt, createdAt)
	}
}

func TestServerListWorkspacesPagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	for _, name := range []string{"alpha001", "beta0001", "gamma001"} {
		body := adminservice.WorkspaceUpsert{
			Name:         string(name),
			WorkflowName: "workflow-1",
		}
		if _, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateWorkspace(%q) error = %v", name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListWorkspaces(ctx, adminservice.ListWorkspacesRequestObject{
		Params: adminservice.ListWorkspacesParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListWorkspaces(first page) error = %v", err)
	}
	first, ok := firstResp.(adminservice.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkspaces(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListWorkspaces(ctx, adminservice.ListWorkspacesRequestObject{
		Params: adminservice.ListWorkspacesParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListWorkspaces(second page) error = %v", err)
	}
	second, ok := secondResp.(adminservice.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 || second.Items[0].Name == first.Items[0].Name {
		t.Fatalf("ListWorkspaces(second page) = %#v", second)
	}
}

func TestServerRejectsInvalidWorkspaceReferences(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	missingWorkflow := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_name": "missing-workflow"
	}`)
	resp, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &missingWorkflow})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing workflow) error = %v", err)
	}
	if _, ok := resp.(adminservice.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(missing workflow) response = %#v", resp)
	}

	nilCreateResp, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{})
	if err != nil {
		t.Fatalf("CreateWorkspace(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminservice.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(nil body) response = %#v", nilCreateResp)
	}

	missingName := mustWorkspaceUpsert(t, `{
		"name": " ",
		"workflow_name": "workflow-1"
	}`)
	missingNameResp, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &missingName})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing name) error = %v", err)
	}
	if _, ok := missingNameResp.(adminservice.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(missing name) response = %#v", missingNameResp)
	}

	invalidWorkflowName := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_name": "Bad_Name"
	}`)
	invalidWorkflowResp, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &invalidWorkflowName})
	if err != nil {
		t.Fatalf("CreateWorkspace(invalid workflow name) error = %v", err)
	}
	if _, ok := invalidWorkflowResp.(adminservice.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(invalid workflow name) response = %#v", invalidWorkflowResp)
	}
}

func TestServerPutRejectsPathNameMismatch(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	body := mustWorkspaceUpsert(t, `{
		"name": "other001",
		"workflow_name": "workflow-1"
	}`)
	resp, err := srv.PutWorkspace(ctx, adminservice.PutWorkspaceRequestObject{
		Name: "expected1",
		Body: &body,
	})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	if _, ok := resp.(adminservice.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace() response = %#v", resp)
	}

	nilPutResp, err := srv.PutWorkspace(ctx, adminservice.PutWorkspaceRequestObject{Name: "expected1"})
	if err != nil {
		t.Fatalf("PutWorkspace(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminservice.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace(nil body) response = %#v", nilPutResp)
	}
}

func TestServerWorkspaceConflictAndMissingDelete(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")

	body := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_name": "workflow-1"
	}`)
	if _, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateWorkspace(seed) error = %v", err)
	}
	duplicateResp, err := srv.CreateWorkspace(ctx, adminservice.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace(duplicate) error = %v", err)
	}
	if _, ok := duplicateResp.(adminservice.CreateWorkspace409JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(duplicate) response = %#v", duplicateResp)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminservice.DeleteWorkspaceRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteWorkspace(missing) error = %v", err)
	}
	if _, ok := deleteResp.(adminservice.DeleteWorkspace404JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace(missing) response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 1 || runtime.deleted[0] != "missing" {
		t.Fatalf("runtime deleted for missing workspace = %#v", runtime.deleted)
	}
}

func TestServerStoreHelpers(t *testing.T) {
	t.Parallel()

	var nilServer *Server
	if _, err := nilServer.store(); err == nil {
		t.Fatal("nil server store() error = nil")
	}
	if _, err := nilServer.workflowStore(); err == nil {
		t.Fatal("nil server workflowStore() error = nil")
	}
	if _, err := (&Server{}).workflowStore(); err == nil {
		t.Fatal("empty server workflowStore() error = nil")
	}

	base := kv.NewMemory(nil)
	srv := &Server{Store: base}
	if got, err := srv.workflowStore(); err != nil || got != base {
		t.Fatalf("workflowStore fallback = %v, %v", got, err)
	}

	workflows := kv.NewMemory(nil)
	srv.WorkflowStore = workflows
	if got, err := srv.workflowStore(); err != nil || got != workflows {
		t.Fatalf("workflowStore explicit = %v, %v", got, err)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	store, err := kv.NewBadgerInMemory(nil)
	if err != nil {
		t.Fatalf("NewBadgerInMemory() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Server{
		Store:         kv.Prefixed(store, kv.Key{"workspaces"}),
		WorkflowStore: kv.Prefixed(store, kv.Key{"workflows"}),
	}
}

func seedWorkflow(t *testing.T, srv *Server, name string) {
	t.Helper()

	store, err := srv.workflowStore()
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	if err := store.Set(context.Background(), workflowReferenceKey(name), []byte(`{}`)); err != nil {
		t.Fatalf("seed workflow %q: %v", name, err)
	}
}

func mustWorkspaceUpsert(t *testing.T, raw string) adminservice.WorkspaceUpsert {
	t.Helper()

	var upsert adminservice.WorkspaceUpsert
	if err := json.Unmarshal([]byte(raw), &upsert); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	return upsert
}

type recordingRuntimeStore struct {
	prepared []string
	deleted  []string
}

func (s *recordingRuntimeStore) PrepareWorkspace(_ context.Context, workspace string) (Runtime, error) {
	s.prepared = append(s.prepared, workspace)
	return Runtime{ObjectPrefix: ObjectPrefix(workspace), LocalDir: "/tmp/" + workspace}, nil
}

func (s *recordingRuntimeStore) GetWorkspaceRuntime(_ context.Context, workspace string) (Runtime, error) {
	return Runtime{ObjectPrefix: ObjectPrefix(workspace), LocalDir: "/tmp/" + workspace}, nil
}

func (s *recordingRuntimeStore) DeleteWorkspaceRuntime(_ context.Context, workspace string) error {
	s.deleted = append(s.deleted, workspace)
	return nil
}
