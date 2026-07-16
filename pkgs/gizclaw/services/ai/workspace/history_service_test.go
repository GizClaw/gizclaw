package workspace

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/acl"
	"github.com/GizClaw/gizclaw-go/pkgs/store/objectstore"
)

func TestServerWorkspaceHistoryServiceAuthorizesReadPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	auth := &historyServiceAuthorizer{}
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
	subject := acl.PublicKeySubject("gear-a")
	list, err := srv.ListWorkspaceHistory(ctx, auth, subject, "demo0001", apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != entry.ID || list.Items[0].Text != "hello" {
		t.Fatalf("ListWorkspaceHistory() = %+v", list)
	}

	got, err := srv.GetWorkspaceHistory(ctx, auth, subject, "demo0001", entry.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceHistory() error = %v", err)
	}
	if got.ID != entry.ID || got.Text != "hello" {
		t.Fatalf("GetWorkspaceHistory() = %+v", got)
	}

	r, err := srv.ReadWorkspaceHistoryAsset(ctx, auth, subject, "demo0001", entry.Assets[0].Name)
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
	if len(auth.requests) != 3 {
		t.Fatalf("authorize requests = %+v", auth.requests)
	}
	for _, req := range auth.requests {
		if req.Subject != subject || req.Resource != acl.WorkspaceResource("demo0001") || req.Permission != apitypes.ACLPermissionRead {
			t.Fatalf("authorize request = %+v", req)
		}
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

func TestServerWorkspaceHistoryServiceDeniesReadPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	auth := &historyServiceAuthorizer{err: acl.ErrDenied}
	ctx := context.Background()
	seedWorkspace(t, srv, "demo0001")

	entry, err := srv.AppendWorkspaceHistory(ctx, "demo0001", AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "hello",
		Asset: &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	subject := acl.PublicKeySubject("gear-a")
	if _, err := srv.ListWorkspaceHistory(ctx, auth, subject, "demo0001", apitypes.PeerRunHistoryListRequest{}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if _, err := srv.GetWorkspaceHistory(ctx, auth, subject, "demo0001", entry.ID); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("GetWorkspaceHistory() error = %v", err)
	}
	if _, err := srv.ReadWorkspaceHistoryAsset(ctx, auth, subject, "demo0001", entry.Assets[0].Name); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("ReadWorkspaceHistoryAsset() error = %v", err)
	}
}

func TestServerWorkspaceHistoryServiceRequiresAuthorizer(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	ctx := context.Background()
	seedWorkspace(t, srv, "demo0001")
	entry, err := srv.AppendWorkspaceHistory(ctx, "demo0001", AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "hello",
		Asset: &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	subject := acl.PublicKeySubject("gear-a")
	tests := []struct {
		name string
		read func() error
	}{
		{
			name: "list",
			read: func() error {
				_, err := srv.ListWorkspaceHistory(ctx, nil, subject, "demo0001", apitypes.PeerRunHistoryListRequest{})
				return err
			},
		},
		{
			name: "get",
			read: func() error {
				_, err := srv.GetWorkspaceHistory(ctx, nil, subject, "demo0001", entry.ID)
				return err
			},
		},
		{
			name: "asset",
			read: func() error {
				_, err := srv.ReadWorkspaceHistoryAsset(ctx, nil, subject, "demo0001", entry.Assets[0].Name)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.read(); err == nil || !strings.Contains(err.Error(), "authorizer is required") {
				t.Fatalf("read error = %v", err)
			}
		})
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

type historyServiceAuthorizer struct {
	err      error
	requests []acl.AuthorizeRequest
}

func (a *historyServiceAuthorizer) Authorize(_ context.Context, req acl.AuthorizeRequest) error {
	a.requests = append(a.requests, req)
	return a.err
}
