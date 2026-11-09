package peerresource

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestSharedWorkspaceLookupReusesRecordWithoutWorkflowStore(t *testing.T) {
	f := newSharedSQLLookupFixture(t)
	if err := f.workflows.DB.Close(); err != nil {
		t.Fatal(err)
	}
	response := workspaceLookupResponseForTest(t, f.server, f.item.Name)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("warm shared GET = %#v, want success without Workflow storage", response)
	}
	if _, rpcErr := f.server.ResolveRunWorkspaceSelection(t.Context(), f.item.Name); rpcErr != nil {
		t.Fatalf("warm shared selection = %#v", rpcErr)
	}
	items, err := f.server.effectiveWorkspaces(t.Context())
	if err != nil || len(items) != 1 || items[0].Id != f.item.Id {
		t.Fatalf("warm shared listing = %#v, %v", items, err)
	}
	if f.catalog.createCalls != 0 || f.catalog.readCalls < 3 {
		t.Fatalf("warm lookups used %d creation calls and %d exact reads", f.catalog.createCalls, f.catalog.readCalls)
	}
}

func TestSharedWorkspaceLookupDoesNotAcquireCreationLock(t *testing.T) {
	f := newSharedSQLLookupFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	unlock := f.catalog.IconLocks.LockRecord(f.item.Name)
	done := make(chan error, 1)
	go func() {
		_, err := f.server.getWorkspaceByName(f.server.ownerContext(ctx), f.item.Name)
		done <- err
	}()
	select {
	case err := <-done:
		unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		unlock()
		<-done
		t.Fatal("existing shared Workspace lookup waited for the creation lock")
	}
	if f.catalog.createCalls != 0 {
		t.Fatalf("warm lookup invoked creation %d times", f.catalog.createCalls)
	}
}

func TestSharedWorkspaceLookupNeedsCreatorOnlyForLocalMiss(t *testing.T) {
	f := newSharedSQLLookupFixture(t)
	local := workspacetest.New(t)
	local.Workflows = f.workflows
	catalog := &sharedSQLLookupCatalog{Server: local}
	f.server.Workspaces = catalog
	for range 2 {
		item, err := f.server.getWorkspaceByName(f.server.ownerContext(t.Context()), f.item.Name)
		if err != nil || item.Id != f.item.Id {
			t.Fatalf("local shared lookup = %#v, %v", item, err)
		}
	}
	if catalog.createCalls != 1 || catalog.readCalls != 2 {
		t.Fatalf("cold/warm lookup used %d creates and %d exact reads", catalog.createCalls, catalog.readCalls)
	}
}

func TestSharedWorkspaceLookupReusesNameReaderWithoutCreator(t *testing.T) {
	f := newSharedSQLLookupFixture(t)
	f.server.Workspaces = &sharedWorkspaceNameReader{
		WorkspaceAdminService: f.catalog.Server,
		reader:                f.catalog.Server,
	}
	if _, ok := f.server.Workspaces.(systemWorkspaceCreator); ok {
		t.Fatal("read-only fixture unexpectedly exposes creation")
	}
	item, err := f.server.getWorkspaceByName(f.server.ownerContext(t.Context()), f.item.Name)
	if err != nil || item.Id != f.item.Id {
		t.Fatalf("read-only shared lookup = %#v, %v", item, err)
	}
}

func TestSharedWorkspaceLookupPreservesAvailabilityErrors(t *testing.T) {
	for _, readErr := range []error{workspace.ErrWorkspacePendingDeletion, kv.ErrNotFound, sql.ErrNoRows, context.Canceled, errors.New("Workspace storage unavailable")} {
		t.Run(readErr.Error(), func(t *testing.T) {
			f := newSharedSQLLookupFixture(t)
			f.catalog.readErr = readErr
			_, err := f.server.getWorkspaceByName(f.server.ownerContext(t.Context()), f.item.Name)
			if !errors.Is(err, readErr) || f.catalog.createCalls != 0 {
				t.Fatalf("availability error = %v, creation calls = %d", err, f.catalog.createCalls)
			}
		})
	}
	t.Run("owner unavailable", func(t *testing.T) {
		f := newSharedSQLLookupFixture(t)
		unavailable := errors.New("owner is unavailable")
		f.catalog.PeerAvailability = func(_ context.Context, owner string) error {
			if owner == socialutil.StringValue(f.item.OwnerPublicKey) {
				return unavailable
			}
			return nil
		}
		_, err := f.server.getWorkspaceByName(f.server.ownerContext(t.Context()), f.item.Name)
		if !errors.Is(err, unavailable) || f.catalog.createCalls != 0 {
			t.Fatalf("owner fence = %v, creation calls = %d", err, f.catalog.createCalls)
		}
	})
}

func TestSharedWorkspaceLookupRejectsBindingIdentityMismatch(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*apitypes.Workspace)
	}{
		{"id", func(item *apitypes.Workspace) { item.Id = "2222222222222222" }},
		{"name", func(item *apitypes.Workspace) { item.Name = "different" }},
		{"owner", func(item *apitypes.Workspace) { item.OwnerPublicKey = new("different") }},
		{"ordinary", func(item *apitypes.Workspace) { item.System = new(false) }},
		{"workflow", func(item *apitypes.Workspace) { item.WorkflowId = "ordinary" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newSharedSQLLookupFixture(t)
			f.catalog.mutate = test.mutate
			_, err := f.server.getWorkspaceByName(f.server.ownerContext(t.Context()), f.item.Name)
			if err == nil || f.catalog.createCalls != 0 {
				t.Fatalf("mismatched binding lookup = %v, creation calls = %d", err, f.catalog.createCalls)
			}
		})
	}
}

type sharedSQLLookupFixture struct {
	server    *Server
	catalog   *sharedSQLLookupCatalog
	workflows *workflow.Server
	item      apitypes.Workspace
}

func newSharedSQLLookupFixture(t *testing.T) sharedSQLLookupFixture {
	t.Helper()
	ctx := t.Context()
	owner, member := giznet.PublicKey{1}, giznet.PublicKey{2}
	workflows := workflowtest.New(t)
	if err := workflows.EnsureBuiltinWorkflows(ctx); err != nil {
		t.Fatal(err)
	}
	workspaces := workspacetest.New(t)
	workspaces.Workflows = workflows
	catalog := &sharedSQLLookupCatalog{Server: workspaces}
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	friends := &friend.Server{Friends: store, Workspaces: catalog, SFUURL: "wss://sfu.test"}
	relation, err := friends.AdminCreateFriend(ctx, owner.String(), member.String())
	if err != nil {
		t.Fatal(err)
	}
	name := socialutil.StringValue(relation.WorkspaceName)
	item, err := catalog.GetWorkspaceByName(ownership.WithOwner(ctx, owner.String()), name)
	if err != nil {
		t.Fatal(err)
	}
	catalog.createCalls = 0
	profile := apitypes.RuntimeProfile{}
	server := &Server{
		Caller: member, Workspaces: catalog, Friends: friends,
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	return sharedSQLLookupFixture{server: server, catalog: catalog, workflows: workflows, item: item}
}

type sharedSQLLookupCatalog struct {
	*workspace.Server
	createCalls int
	readCalls   int
	readErr     error
	mutate      func(*apitypes.Workspace)
}

func (s *sharedSQLLookupCatalog) CreateSystemWorkspace(ctx context.Context, body adminhttp.WorkspaceUpsert) (apitypes.Workspace, bool, error) {
	s.createCalls++
	return s.Server.CreateSystemWorkspace(ctx, body)
}

func (s *sharedSQLLookupCatalog) GetAvailableWorkspaceByID(ctx context.Context, id string) (apitypes.Workspace, error) {
	s.readCalls++
	if s.readErr != nil {
		return apitypes.Workspace{}, s.readErr
	}
	item, err := s.Server.GetAvailableWorkspaceByID(ctx, id)
	if err == nil && s.mutate != nil {
		s.mutate(&item)
	}
	return item, err
}

type sharedWorkspaceNameReader struct {
	workspace.WorkspaceAdminService
	reader *workspace.Server
}

func (s *sharedWorkspaceNameReader) GetWorkspaceByName(ctx context.Context, name string) (apitypes.Workspace, error) {
	return s.reader.GetWorkspaceByName(ctx, name)
}
