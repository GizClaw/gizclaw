package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/model"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
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

	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResp)
	}
	if created.Name != "alpha001" || created.WorkflowName != "workflow-1" {
		t.Fatalf("CreateWorkspace() workspace = %#v", created)
	}
	if created.System == nil || *created.System {
		t.Fatalf("CreateWorkspace() system = %#v, want false", created.System)
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

	listResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces() response = %#v", listResp)
	}
	if len(listed.Items) != 1 || listed.Items[0].Name != "alpha001" || listed.HasNext {
		t.Fatalf("ListWorkspaces() = %#v", listed)
	}

	getResp, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: "alpha001"})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	got, ok := getResp.(adminhttp.GetWorkspace200JSONResponse)
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
	putResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{
		Name: "alpha001",
		Body: &updateBody,
	})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutWorkspace200JSONResponse)
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

	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: "alpha001"})
	if err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 0 {
		t.Fatalf("runtime deleted during fast delete = %#v", runtime.deleted)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, "alpha001"); err != nil || !pending {
		t.Fatalf("workspace pending deletion = %v, error = %v", pending, err)
	}

	getAfterDelete, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: "alpha001"})
	if err != nil {
		t.Fatalf("GetWorkspace() after delete error = %v", err)
	}
	if _, ok := getAfterDelete.(adminhttp.GetWorkspace404JSONResponse); !ok {
		t.Fatalf("GetWorkspace() after delete response = %#v", getAfterDelete)
	}
	createAfterDelete, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &createBody})
	if err != nil {
		t.Fatalf("CreateWorkspace() while pending error = %v", err)
	}
	if response, ok := createAfterDelete.(adminhttp.CreateWorkspace409JSONResponse); !ok || response.Error.Code != WorkspacePendingDeletionCode {
		t.Fatalf("CreateWorkspace() while pending response = %#v", createAfterDelete)
	}
	putAfterDelete, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "alpha001", Body: &updateBody})
	if err != nil {
		t.Fatalf("PutWorkspace() while pending error = %v", err)
	}
	if response, ok := putAfterDelete.(adminhttp.PutWorkspace409JSONResponse); !ok || response.Error.Code != WorkspacePendingDeletionCode {
		t.Fatalf("PutWorkspace() while pending response = %#v", putAfterDelete)
	}
	if _, _, err := srv.CreateSystemWorkspace(ctx, createBody); !errors.Is(err, ErrWorkspacePendingDeletion) {
		t.Fatalf("CreateSystemWorkspace() while pending error = %v", err)
	}
}

func TestServerSystemWorkspaceLifecycle(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	runtime := &recordingRuntimeStore{}
	srv.RuntimeStore = runtime
	ctx := context.Background()
	seedWorkflow(t, srv, "chatroom")
	body := adminhttp.WorkspaceUpsert{Name: "friend-chat", WorkflowName: "chatroom"}

	created, wasCreated, err := srv.CreateSystemWorkspace(ctx, body)
	if err != nil {
		t.Fatalf("CreateSystemWorkspace() error = %v", err)
	}
	if !wasCreated || created.System == nil || !*created.System {
		t.Fatalf("CreateSystemWorkspace() = %#v, created=%v", created, wasCreated)
	}
	existing, wasCreated, err := srv.CreateSystemWorkspace(ctx, body)
	if err != nil {
		t.Fatalf("CreateSystemWorkspace(existing) error = %v", err)
	}
	if wasCreated || existing.System == nil || !*existing.System {
		t.Fatalf("CreateSystemWorkspace(existing) = %#v, created=%v", existing, wasCreated)
	}

	putBody := adminhttp.WorkspaceUpsert{Name: "friend-chat", WorkflowName: "chatroom"}
	putResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "friend-chat", Body: &putBody})
	if err != nil {
		t.Fatalf("PutWorkspace(system) error = %v", err)
	}
	updated, ok := putResp.(adminhttp.PutWorkspace200JSONResponse)
	if !ok || updated.System == nil || !*updated.System {
		t.Fatalf("PutWorkspace(system) response = %#v", putResp)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: "friend-chat"})
	if err != nil {
		t.Fatalf("DeleteWorkspace(system) error = %v", err)
	}
	blocked, ok := deleteResp.(adminhttp.DeleteWorkspace409JSONResponse)
	if !ok || blocked.Error.Code != SystemWorkspaceDeleteForbiddenCode {
		t.Fatalf("DeleteWorkspace(system) response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 0 {
		t.Fatalf("runtime deleted after rejected generic delete = %#v", runtime.deleted)
	}
	if _, err := getWorkspace(ctx, srv.Store, "friend-chat"); err != nil {
		t.Fatalf("system workspace after rejected generic delete: %v", err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, "friend-chat"); err != nil || pending {
		t.Fatalf("system workspace pending deletion = %v, error = %v", pending, err)
	}

	deleted, err := srv.DeleteSystemWorkspace(ctx, "friend-chat")
	if err != nil {
		t.Fatalf("DeleteSystemWorkspace() error = %v", err)
	}
	if deleted.System == nil || !*deleted.System {
		t.Fatalf("DeleteSystemWorkspace() = %#v", deleted)
	}
	if len(runtime.deleted) != 1 || runtime.deleted[0] != "friend-chat" {
		t.Fatalf("runtime deleted after system delete = %#v", runtime.deleted)
	}
	if _, err := srv.DeleteSystemWorkspace(ctx, "friend-chat"); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("DeleteSystemWorkspace(missing) error = %v, want kv.ErrNotFound", err)
	}
	if len(runtime.deleted) != 2 || runtime.deleted[1] != "friend-chat" {
		t.Fatalf("runtime deleted after missing system delete = %#v", runtime.deleted)
	}
}

func TestWorkspaceDeleteSerializesWithPut(t *testing.T) {
	srv := newTestServer(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")
	body := adminhttp.WorkspaceUpsert{Name: "concurrent", WorkflowName: "workflow-1"}
	if response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	} else if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
		t.Fatalf("CreateWorkspace response = %#v", response)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		response, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: body.Name})
		if err == nil {
			if _, ok := response.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
				err = fmt.Errorf("DeleteWorkspace response = %#v", response)
			}
		}
		errs <- err
	}()
	go func() {
		<-start
		response, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: body.Name, Body: &body})
		if err == nil {
			switch response.(type) {
			case adminhttp.PutWorkspace200JSONResponse, adminhttp.PutWorkspace409JSONResponse:
			default:
				err = fmt.Errorf("PutWorkspace response = %#v", response)
			}
		}
		errs <- err
	}()
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := getWorkspace(ctx, srv.Store, body.Name); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("workspace after concurrent delete/put error = %v, want kv.ErrNotFound", err)
	}
	if pending, err := pendingdeletion.HasLocator(ctx, srv.Store, pendingdeletion.KindWorkspace, body.Name); err != nil || !pending {
		t.Fatalf("workspace pending deletion = %v, error = %v", pending, err)
	}
}

func TestServerSystemWorkspaceClassificationComesFromCreationPath(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = &recordingRuntimeStore{}
	ctx := context.Background()
	seedWorkflow(t, srv, "chatroom")
	body := adminhttp.WorkspaceUpsert{Name: "friend-user-created", WorkflowName: "chatroom"}

	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := createResp.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok || created.System == nil || *created.System {
		t.Fatalf("CreateWorkspace() response = %#v, want user Workspace", createResp)
	}
	if _, _, err := srv.CreateSystemWorkspace(ctx, body); err == nil {
		t.Fatal("CreateSystemWorkspace(user Workspace) error = nil, want classification conflict")
	}
	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: body.Name})
	if err != nil {
		t.Fatalf("DeleteWorkspace() error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkspace200JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace() response = %#v", deleteResp)
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
	if got.System == nil || *got.System {
		t.Fatalf("getWorkspace() legacy system = %#v, want false", got.System)
	}

	listResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{})
	if err != nil {
		t.Fatalf("ListWorkspaces() error = %v", err)
	}
	listed, ok := listResp.(adminhttp.ListWorkspaces200JSONResponse)
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
		body := adminhttp.WorkspaceUpsert{
			Name:         string(name),
			WorkflowName: "workflow-1",
		}
		if _, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateWorkspace(%q) error = %v", name, err)
		}
	}

	limit := int32(1)
	firstResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
		Params: adminhttp.ListWorkspacesParams{Limit: &limit},
	})
	if err != nil {
		t.Fatalf("ListWorkspaces(first page) error = %v", err)
	}
	first, ok := firstResp.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces(first page) response = %#v", firstResp)
	}
	if len(first.Items) != 1 || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkspaces(first page) = %#v", first)
	}

	cursor := string(*first.NextCursor)
	secondResp, err := srv.ListWorkspaces(ctx, adminhttp.ListWorkspacesRequestObject{
		Params: adminhttp.ListWorkspacesParams{
			Cursor: &cursor,
			Limit:  &limit,
		},
	})
	if err != nil {
		t.Fatalf("ListWorkspaces(second page) error = %v", err)
	}
	second, ok := secondResp.(adminhttp.ListWorkspaces200JSONResponse)
	if !ok {
		t.Fatalf("ListWorkspaces(second page) response = %#v", secondResp)
	}
	if len(second.Items) != 1 || second.Items[0].Name == first.Items[0].Name {
		t.Fatalf("ListWorkspaces(second page) = %#v", second)
	}
}

func TestServerWorkspaceLabelsRoundTripPreserveAndClear(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-1")
	ctx := context.Background()
	inputLabels := map[string]string{"collection": "raids", "tier": "Gold"}
	body := adminhttp.WorkspaceUpsert{Name: "labels01", WorkflowName: "workflow-1", Labels: &inputLabels}
	response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	created, ok := response.(adminhttp.CreateWorkspace200JSONResponse)
	if !ok || created.Labels == nil || (*created.Labels)["collection"] != "raids" {
		t.Fatalf("CreateWorkspace() = %#v", response)
	}
	inputLabels["collection"] = "mutated"
	(*created.Labels)["collection"] = "also-mutated"

	getResponse, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: "labels01"})
	if err != nil {
		t.Fatalf("GetWorkspace() error = %v", err)
	}
	stored := getResponse.(adminhttp.GetWorkspace200JSONResponse)
	if stored.Labels == nil || (*stored.Labels)["collection"] != "raids" {
		t.Fatalf("stored labels = %#v", stored.Labels)
	}

	preserve := adminhttp.WorkspaceUpsert{Name: "labels01", WorkflowName: "workflow-1"}
	putResponse, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "labels01", Body: &preserve})
	if err != nil {
		t.Fatalf("PutWorkspace(preserve) error = %v", err)
	}
	preserved := putResponse.(adminhttp.PutWorkspace200JSONResponse)
	if preserved.Labels == nil || (*preserved.Labels)["collection"] != "raids" {
		t.Fatalf("preserved labels = %#v", preserved.Labels)
	}

	empty := map[string]string{}
	clear := adminhttp.WorkspaceUpsert{Name: "labels01", WorkflowName: "workflow-1", Labels: &empty}
	putResponse, err = srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "labels01", Body: &clear})
	if err != nil {
		t.Fatalf("PutWorkspace(clear) error = %v", err)
	}
	cleared := putResponse.(adminhttp.PutWorkspace200JSONResponse)
	if cleared.Labels == nil || len(*cleared.Labels) != 0 {
		t.Fatalf("cleared labels = %#v", cleared.Labels)
	}
}

func TestServerWorkspaceLabelValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		labels map[string]string
	}{
		{name: "empty key", labels: map[string]string{"": "value"}},
		{name: "uppercase key", labels: map[string]string{"Collection": "value"}},
		{name: "invalid key character", labels: map[string]string{"collection/x": "value"}},
		{name: "invalid key end", labels: map[string]string{"collection-": "value"}},
		{name: "empty value", labels: map[string]string{"collection": ""}},
		{name: "leading whitespace", labels: map[string]string{"collection": " raids"}},
		{name: "control character", labels: map[string]string{"collection": "raid\n"}},
		{name: "invalid utf8", labels: map[string]string{"collection": string([]byte{0xff})}},
		{name: "oversized value", labels: map[string]string{"collection": strings.Repeat("x", maxWorkspaceLabelValueBytes+1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv := newTestServer(t)
			seedWorkflow(t, srv, "workflow-1")
			body := adminhttp.WorkspaceUpsert{Name: "invalid1", WorkflowName: "workflow-1", Labels: &test.labels}
			response, err := srv.CreateWorkspace(context.Background(), adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatalf("CreateWorkspace() error = %v", err)
			}
			if _, ok := response.(adminhttp.CreateWorkspace400JSONResponse); !ok {
				t.Fatalf("CreateWorkspace() response = %#v, want 400", response)
			}
			if _, err := getWorkspace(context.Background(), srv.Store, "invalid1"); !errors.Is(err, kv.ErrNotFound) {
				t.Fatalf("invalid Workspace write error = %v, want kv.ErrNotFound", err)
			}
		})
	}
}

func TestServerWorkspaceLabelFilteringBeforePagination(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedWorkflow(t, srv, "workflow-1")
	ctx := ownership.WithOwner(context.Background(), "peer-a")
	fixtures := []struct {
		name       string
		collection string
		tier       string
	}{
		{name: "alpha001", collection: "raids", tier: "gold"},
		{name: "beta0001", collection: "assistants", tier: "gold"},
		{name: "gamma001", collection: "raids", tier: "silver"},
		{name: "omega001", collection: "raids", tier: "gold"},
	}
	for _, fixture := range fixtures {
		labels := map[string]string{"collection": fixture.collection, "tier": fixture.tier}
		body := adminhttp.WorkspaceUpsert{Name: fixture.name, WorkflowName: "workflow-1", Labels: &labels}
		if _, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
			t.Fatalf("CreateWorkspace(%q) error = %v", fixture.name, err)
		}
	}

	selectors := []string{"collection=raids"}
	limit := int32(2)
	firstResponse, err := srv.ListWorkspaces(context.Background(), adminhttp.ListWorkspacesRequestObject{Params: adminhttp.ListWorkspacesParams{Label: &selectors, Limit: &limit}})
	if err != nil {
		t.Fatalf("ListWorkspaces(first) error = %v", err)
	}
	first := firstResponse.(adminhttp.ListWorkspaces200JSONResponse)
	if len(first.Items) != 2 || first.Items[0].Name != "alpha001" || first.Items[1].Name != "gamma001" || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("ListWorkspaces(first) = %#v", first)
	}
	cursor := *first.NextCursor
	secondResponse, err := srv.ListWorkspaces(context.Background(), adminhttp.ListWorkspacesRequestObject{Params: adminhttp.ListWorkspacesParams{Cursor: &cursor, Label: &selectors, Limit: &limit}})
	if err != nil {
		t.Fatalf("ListWorkspaces(second) error = %v", err)
	}
	second := secondResponse.(adminhttp.ListWorkspaces200JSONResponse)
	if len(second.Items) != 1 || second.Items[0].Name != "omega001" || second.HasNext {
		t.Fatalf("ListWorkspaces(second) = %#v", second)
	}

	owned, err := srv.ListWorkspacesByOwnerAndLabels(context.Background(), "peer-a", map[string]string{"collection": "raids", "tier": "gold"})
	if err != nil {
		t.Fatalf("ListWorkspacesByOwnerAndLabels() error = %v", err)
	}
	if len(owned) != 2 || owned[0].Name != "alpha001" || owned[1].Name != "omega001" {
		t.Fatalf("ListWorkspacesByOwnerAndLabels() = %#v", owned)
	}

	invalid := []string{"collection"}
	invalidResponse, err := srv.ListWorkspaces(context.Background(), adminhttp.ListWorkspacesRequestObject{Params: adminhttp.ListWorkspacesParams{Label: &invalid}})
	if err != nil {
		t.Fatalf("ListWorkspaces(invalid) error = %v", err)
	}
	if _, ok := invalidResponse.(adminhttp.ListWorkspaces400JSONResponse); !ok {
		t.Fatalf("ListWorkspaces(invalid) = %#v, want 400", invalidResponse)
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
	resp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &missingWorkflow})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing workflow) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(missing workflow) response = %#v", resp)
	}

	nilCreateResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{})
	if err != nil {
		t.Fatalf("CreateWorkspace(nil body) error = %v", err)
	}
	if _, ok := nilCreateResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(nil body) response = %#v", nilCreateResp)
	}

	missingName := mustWorkspaceUpsert(t, `{
		"name": " ",
		"workflow_name": "workflow-1"
	}`)
	missingNameResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &missingName})
	if err != nil {
		t.Fatalf("CreateWorkspace(missing name) error = %v", err)
	}
	if _, ok := missingNameResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(missing name) response = %#v", missingNameResp)
	}

	invalidWorkflowName := mustWorkspaceUpsert(t, `{
		"name": "alpha001",
		"workflow_name": "Bad_Name"
	}`)
	invalidWorkflowResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &invalidWorkflowName})
	if err != nil {
		t.Fatalf("CreateWorkspace(invalid workflow name) error = %v", err)
	}
	if _, ok := invalidWorkflowResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(invalid workflow name) response = %#v", invalidWorkflowResp)
	}
}

func TestNormalizeWorkspaceUpsertAcceptsWorkflowAliasesAndResourceIDs(t *testing.T) {
	t.Parallel()

	for _, workflowName := range []string{"2fa-chat", "my_workflow"} {
		got, err := normalizeWorkspaceUpsert(adminhttp.WorkspaceUpsert{
			Name:         "runtime-workspace",
			WorkflowName: workflowName,
		}, "")
		if err != nil {
			t.Fatalf("normalizeWorkspaceUpsert(%q) error = %v", workflowName, err)
		}
		if got.WorkflowName != workflowName {
			t.Fatalf("normalizeWorkspaceUpsert() workflow_name = %q, want %q", got.WorkflowName, workflowName)
		}
	}
}

func TestServerRejectsInvalidToolkitPolicy(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedWorkflow(t, srv, "workflow-1")
	toolIDs := []string{""}
	body := adminhttp.WorkspaceUpsert{
		Name:         "alpha001",
		WorkflowName: "workflow-1",
		Toolkit:      &apitypes.ToolkitPolicy{ToolIds: &toolIDs},
	}

	createResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, ok := createResp.(adminhttp.CreateWorkspace400JSONResponse); !ok {
		t.Fatalf("CreateWorkspace() response = %#v", createResp)
	}

	putResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "alpha001", Body: &body})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	if _, ok := putResp.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace() response = %#v", putResp)
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
	resp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{
		Name: "expected1",
		Body: &body,
	})
	if err != nil {
		t.Fatalf("PutWorkspace() error = %v", err)
	}
	if _, ok := resp.(adminhttp.PutWorkspace400JSONResponse); !ok {
		t.Fatalf("PutWorkspace() response = %#v", resp)
	}

	nilPutResp, err := srv.PutWorkspace(ctx, adminhttp.PutWorkspaceRequestObject{Name: "expected1"})
	if err != nil {
		t.Fatalf("PutWorkspace(nil body) error = %v", err)
	}
	if _, ok := nilPutResp.(adminhttp.PutWorkspace400JSONResponse); !ok {
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
	if _, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body}); err != nil {
		t.Fatalf("CreateWorkspace(seed) error = %v", err)
	}
	duplicateResp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace(duplicate) error = %v", err)
	}
	if _, ok := duplicateResp.(adminhttp.CreateWorkspace409JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(duplicate) response = %#v", duplicateResp)
	}

	deleteResp, err := srv.DeleteWorkspace(ctx, adminhttp.DeleteWorkspaceRequestObject{Name: "missing"})
	if err != nil {
		t.Fatalf("DeleteWorkspace(missing) error = %v", err)
	}
	if _, ok := deleteResp.(adminhttp.DeleteWorkspace404JSONResponse); !ok {
		t.Fatalf("DeleteWorkspace(missing) response = %#v", deleteResp)
	}
	if len(runtime.deleted) != 0 {
		t.Fatalf("runtime deleted for missing workspace = %#v", runtime.deleted)
	}
}

func TestServerRejectsIncompleteFlowcraftModelParameters(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	ctx := context.Background()
	seedFlowcraftWorkflow(t, srv, "flowcraft-chat", true)
	seedModel(t, srv, "chat-model", apitypes.ModelKindLlm)
	seedModel(t, srv, "embedding-model", apitypes.ModelKindEmbedding)
	seedModel(t, srv, "speech-model", apitypes.ModelKindTts)

	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{name: "missing parameters", body: `{"name":"missing-params","workflow_name":"flowcraft-chat"}`, want: `"generate_model" requires a concrete Model resource name`},
		{name: "missing generate", body: `{"name":"missing-generate","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft"}}`, want: `"generate_model" requires a concrete Model resource name`},
		{name: "symbolic generate", body: `{"name":"symbolic-generate","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"generate_model","extract_model":"chat-model"}}`, want: `"generate_model" requires a concrete Model resource name`},
		{name: "missing model", body: `{"name":"missing-model","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"not-found","extract_model":"chat-model","embedding_model":"embedding-model"}}`, want: `references missing Model "not-found"`},
		{name: "wrong kind", body: `{"name":"wrong-kind","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"speech-model","extract_model":"chat-model","embedding_model":"embedding-model"}}`, want: `has kind "tts", want "llm"`},
		{name: "missing extract", body: `{"name":"missing-extract","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"chat-model"}}`, want: `"extract_model" requires a concrete Model resource name`},
		{name: "missing embedding", body: `{"name":"missing-embedding","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"chat-model","extract_model":"chat-model"}}`, want: `"embedding_model" requires a concrete Model resource name`},
		{name: "missing embedding model", body: `{"name":"missing-embedding-model","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"chat-model","extract_model":"chat-model","embedding_model":"not-found"}}`, want: `references missing Model "not-found"`},
		{name: "wrong embedding kind", body: `{"name":"wrong-embedding-kind","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"chat-model","extract_model":"chat-model","embedding_model":"chat-model"}}`, want: `has kind "llm", want "embedding"`},
		{name: "valid", body: `{"name":"valid-models","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"chat-model","extract_model":"chat-model","embedding_model":"embedding-model"}}`, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := mustWorkspaceUpsert(t, tt.body)
			resp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatalf("CreateWorkspace() error = %v", err)
			}
			if tt.ok {
				if _, ok := resp.(adminhttp.CreateWorkspace200JSONResponse); !ok {
					t.Fatalf("CreateWorkspace() response = %#v", resp)
				}
				return
			}
			invalid, ok := resp.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok {
				t.Fatalf("CreateWorkspace() response = %#v, want 400", resp)
			}
			if !strings.Contains(invalid.Error.Message, tt.want) {
				t.Fatalf("CreateWorkspace() message = %q, want substring %q", invalid.Error.Message, tt.want)
			}
		})
	}

	srv.Models = nil
	body := mustWorkspaceUpsert(t, `{"name":"model-service-missing","workflow_name":"flowcraft-chat","parameters":{"agent_type":"flowcraft","generate_model":"chat-model","extract_model":"chat-model","embedding_model":"embedding-model"}}`)
	resp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace(model service missing) error = %v", err)
	}
	if _, ok := resp.(adminhttp.CreateWorkspace500JSONResponse); !ok {
		t.Fatalf("CreateWorkspace(model service missing) response = %#v, want 500", resp)
	}
}

func TestServerValidatesRuntimeFlowcraftModelAliases(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	seedFlowcraftWorkflow(t, srv, "flowcraft-chat", true)
	seedModel(t, srv, "chat-model", apitypes.ModelKindLlm)
	seedModel(t, srv, "embedding-resource", apitypes.ModelKindEmbedding)
	ctx := WithRuntimeModelBindings(
		WithRuntimeWorkflowBindings(context.Background(), map[string]string{"2fa-chat": "flowcraft-chat"}),
		map[string]string{
			"generate-model":  "chat-model",
			"extract-model":   "chat-model",
			"embedding-model": "embedding-resource",
			"wrong-embedding": "chat-model",
		},
	)

	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{
			name: "valid aliases",
			body: `{"name":"runtime-valid","workflow_name":"2fa-chat","parameters":{"agent_type":"flowcraft","generate_model":"generate-model","extract_model":"extract-model","embedding_model":"embedding-model"}}`,
			ok:   true,
		},
		{
			name: "missing override alias",
			body: `{"name":"runtime-missing","workflow_name":"2fa-chat","parameters":{"agent_type":"flowcraft","generate_model":"missing-alias","extract_model":"extract-model","embedding_model":"embedding-model"}}`,
			want: `references missing runtime Model alias "missing-alias"`,
		},
		{
			name: "wrong override kind",
			body: `{"name":"runtime-wrong-kind","workflow_name":"2fa-chat","parameters":{"agent_type":"flowcraft","generate_model":"generate-model","extract_model":"extract-model","embedding_model":"wrong-embedding"}}`,
			want: `has kind "llm", want "embedding"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := mustWorkspaceUpsert(t, tt.body)
			resp, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatalf("CreateWorkspace() error = %v", err)
			}
			if tt.ok {
				if _, ok := resp.(adminhttp.CreateWorkspace200JSONResponse); !ok {
					t.Fatalf("CreateWorkspace() response = %#v", resp)
				}
				return
			}
			invalid, ok := resp.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok {
				t.Fatalf("CreateWorkspace() response = %#v, want 400", resp)
			}
			if !strings.Contains(invalid.Error.Message, tt.want) {
				t.Fatalf("CreateWorkspace() message = %q, want substring %q", invalid.Error.Message, tt.want)
			}
			if strings.Contains(invalid.Error.Message, "chat-model") {
				t.Fatalf("CreateWorkspace() message exposes canonical runtime Model: %q", invalid.Error.Message)
			}
		})
	}
	getResp, err := srv.GetWorkspace(ctx, adminhttp.GetWorkspaceRequestObject{Name: "runtime-valid"})
	if err != nil {
		t.Fatalf("GetWorkspace(runtime-valid) error = %v", err)
	}
	stored, ok := getResp.(adminhttp.GetWorkspace200JSONResponse)
	if !ok {
		t.Fatalf("GetWorkspace(runtime-valid) response = %#v", getResp)
	}
	parameters, err := stored.Parameters.AsFlowcraftWorkspaceParameters()
	if err != nil {
		t.Fatalf("stored Workspace parameters: %v", err)
	}
	if parameters.GenerateModel == nil || *parameters.GenerateModel != "generate-model" ||
		parameters.ExtractModel == nil || *parameters.ExtractModel != "extract-model" ||
		parameters.EmbeddingModel == nil || *parameters.EmbeddingModel != "embedding-model" {
		t.Fatalf("stored Workspace parameters = %#v, want runtime aliases", parameters)
	}
}

func TestServerValidatesRuntimeASTTranslateAliases(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	store, err := srv.workflowStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), workflowReferenceKey("ast-workflow"), []byte(`{"name":"ast-workflow","spec":{"driver":"ast-translate","ast_translate":{"translation_model":"translate-model","lang_pair":"auto"}}}`)); err != nil {
		t.Fatal(err)
	}
	seedModel(t, srv, "translation-resource", apitypes.ModelKindTranslation)
	seedModel(t, srv, "llm-resource", apitypes.ModelKindLlm)
	ctx := WithRuntimeVoiceBindings(
		WithRuntimeModelBindings(
			WithRuntimeWorkflowBindings(context.Background(), map[string]string{"translate": "ast-workflow"}),
			map[string]string{"translate-model": "translation-resource", "wrong-model": "llm-resource"},
		),
		map[string]string{"translator": "voice-resource"},
	)
	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{name: "valid", body: `{"name":"ast-valid","workflow_name":"translate","parameters":{"agent_type":"ast-translate","translation_model":"translate-model","voice":{"tts_voice":"translator"}}}`, ok: true},
		{name: "missing model", body: `{"name":"ast-missing-model","workflow_name":"translate","parameters":{"agent_type":"ast-translate","translation_model":"missing"}}`, want: `missing runtime Model alias "missing"`},
		{name: "wrong model kind", body: `{"name":"ast-wrong-model","workflow_name":"translate","parameters":{"agent_type":"ast-translate","translation_model":"wrong-model"}}`, want: `has kind "llm", want "translation"`},
		{name: "missing voice", body: `{"name":"ast-missing-voice","workflow_name":"translate","parameters":{"agent_type":"ast-translate","voice":{"tts_voice":"missing"}}}`, want: `missing runtime Voice alias "missing"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := mustWorkspaceUpsert(t, test.body)
			response, err := srv.CreateWorkspace(ctx, adminhttp.CreateWorkspaceRequestObject{Body: &body})
			if err != nil {
				t.Fatal(err)
			}
			if test.ok {
				if _, ok := response.(adminhttp.CreateWorkspace200JSONResponse); !ok {
					t.Fatalf("CreateWorkspace() = %#v, want 200", response)
				}
				return
			}
			invalid, ok := response.(adminhttp.CreateWorkspace400JSONResponse)
			if !ok || !strings.Contains(invalid.Error.Message, test.want) {
				t.Fatalf("CreateWorkspace() = %#v, want %q", response, test.want)
			}
			if strings.Contains(invalid.Error.Message, "translation-resource") || strings.Contains(invalid.Error.Message, "llm-resource") {
				t.Fatalf("CreateWorkspace() exposes canonical Model ID: %q", invalid.Error.Message)
			}
		})
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
		Models:        &model.Server{Store: kv.Prefixed(store, kv.Key{"models"})},
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

func seedFlowcraftWorkflow(t *testing.T, srv *Server, name string, extract bool) {
	t.Helper()

	settings := `"generate_model":"generate_model"`
	if extract {
		settings += `,"extract_model":"extract_model"`
	}
	settings += `,"embedding_model":"embedding_model"`
	store, err := srv.workflowStore()
	if err != nil {
		t.Fatalf("workflow store: %v", err)
	}
	body := fmt.Appendf(nil, `{"name":%q,"spec":{"driver":"flowcraft","flowcraft":{"settings":{%s}}}}`, name, settings)
	if err := store.Set(context.Background(), workflowReferenceKey(name), body); err != nil {
		t.Fatalf("seed flowcraft workflow %q: %v", name, err)
	}
}

func seedModel(t *testing.T, srv *Server, id string, kind apitypes.ModelKind) {
	t.Helper()

	modelServer, ok := srv.Models.(*model.Server)
	if !ok {
		t.Fatalf("Models = %T", srv.Models)
	}
	data, err := json.Marshal(apitypes.Model{Id: id, Kind: kind})
	if err != nil {
		t.Fatalf("json.Marshal(model) error = %v", err)
	}
	if err := modelServer.Store.Set(context.Background(), kv.Key{"by-id", id}, data); err != nil {
		t.Fatalf("seed model %q: %v", id, err)
	}
}

func mustWorkspaceUpsert(t *testing.T, raw string) adminhttp.WorkspaceUpsert {
	t.Helper()

	var upsert adminhttp.WorkspaceUpsert
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
