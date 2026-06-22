package workspace

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/acl"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/adminservice"
	"github.com/GizClaw/gizclaw-go/pkg/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkg/store/objectstore"
)

func TestServerWorkspaceHistoryServiceAuthorizesReadPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	auth := &historyServiceAuthorizer{}
	srv.Authorizer = auth
	ctx := context.Background()
	seedWorkspace(t, srv, "demo")

	entry, err := srv.AppendWorkspaceHistory(ctx, "demo", AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "hello",
		Asset: &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	subject := acl.PublicKeySubject("gear-a")
	list, err := srv.ListWorkspaceHistory(ctx, subject, "demo", apitypes.PeerRunHistoryListRequest{})
	if err != nil {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != entry.ID || list.Items[0].Text != "hello" {
		t.Fatalf("ListWorkspaceHistory() = %+v", list)
	}

	got, err := srv.GetWorkspaceHistory(ctx, subject, "demo", entry.ID)
	if err != nil {
		t.Fatalf("GetWorkspaceHistory() error = %v", err)
	}
	if got.ID != entry.ID || got.Text != "hello" {
		t.Fatalf("GetWorkspaceHistory() = %+v", got)
	}

	r, err := srv.ReadWorkspaceHistoryAsset(ctx, subject, "demo", entry.Assets[0].Name)
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
		if req.Subject != subject || req.Resource != acl.WorkspaceResource("demo") || req.Permission != apitypes.ACLPermissionWorkspaceRead {
			t.Fatalf("authorize request = %+v", req)
		}
	}
}

func TestServerWorkspaceHistoryServiceDeniesReadPaths(t *testing.T) {
	t.Parallel()

	srv := newTestServer(t)
	srv.RuntimeStore = NewObjectRuntimeStore(objectstore.Dir(t.TempDir()))
	srv.Authorizer = &historyServiceAuthorizer{err: acl.ErrDenied}
	ctx := context.Background()
	seedWorkspace(t, srv, "demo")

	entry, err := srv.AppendWorkspaceHistory(ctx, "demo", AppendHistoryRequest{
		Type:  "agent",
		Name:  "assistant",
		Text:  "hello",
		Asset: &AppendHistoryAsset{MIMEType: "audio/opus", Data: []byte("opus")},
	})
	if err != nil {
		t.Fatalf("AppendWorkspaceHistory() error = %v", err)
	}
	subject := acl.PublicKeySubject("gear-a")
	if _, err := srv.ListWorkspaceHistory(ctx, subject, "demo", apitypes.PeerRunHistoryListRequest{}); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("ListWorkspaceHistory() error = %v", err)
	}
	if _, err := srv.GetWorkspaceHistory(ctx, subject, "demo", entry.ID); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("GetWorkspaceHistory() error = %v", err)
	}
	if _, err := srv.ReadWorkspaceHistoryAsset(ctx, subject, "demo", entry.Assets[0].Name); !errors.Is(err, acl.ErrDenied) {
		t.Fatalf("ReadWorkspaceHistoryAsset() error = %v", err)
	}
}

func TestServerWorkspaceHistoryServiceErrors(t *testing.T) {
	t.Parallel()

	var nilServer *Server
	if _, err := nilServer.AppendWorkspaceHistory(context.Background(), "demo", AppendHistoryRequest{}); err == nil || !strings.Contains(err.Error(), "nil server") {
		t.Fatalf("nil AppendWorkspaceHistory() error = %v", err)
	}

	srv := newTestServer(t)
	if _, err := srv.AppendWorkspaceHistory(context.Background(), "", AppendHistoryRequest{}); err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Fatalf("AppendWorkspaceHistory(empty) error = %v", err)
	}
	seedWorkspace(t, srv, "demo")
	if _, err := srv.AppendWorkspaceHistory(context.Background(), "demo", AppendHistoryRequest{}); err == nil || !strings.Contains(err.Error(), "runtime store") {
		t.Fatalf("AppendWorkspaceHistory(no runtime store) error = %v", err)
	}
}

func seedWorkspace(t *testing.T, srv *Server, name string) {
	t.Helper()

	seedWorkflow(t, srv, "workflow-1")
	body := adminservice.WorkspaceUpsert{Name: name, WorkflowName: "workflow-1"}
	resp, err := srv.CreateWorkspace(context.Background(), adminservice.CreateWorkspaceRequestObject{Body: &body})
	if err != nil {
		t.Fatalf("CreateWorkspace() error = %v", err)
	}
	if _, ok := resp.(adminservice.CreateWorkspace200JSONResponse); !ok {
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
