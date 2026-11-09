package peerresource

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workspacetest"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/ownership"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

func TestWorkspaceGetMissingPrivateNameDoesNotReadSocialOrEnumerate(t *testing.T) {
	for _, name := range []string{"h106-chat", "ordinary", "social-group-invalid", "social-direct-invalid"} {
		t.Run(name, func(t *testing.T) {
			caller, foreign := giznet.PublicKey{1}, giznet.PublicKey{2}
			catalog := newWorkspaceLookupCatalog(foreign.String(), name)
			store := &workspaceLookupStore{Store: kv.NewMemory(nil), getErr: errors.New("Social backend is unavailable")}
			t.Cleanup(func() { _ = store.Close() })
			server := &Server{
				Caller: caller, Workspaces: catalog,
				Friends:      &friend.Server{Friends: store},
				FriendGroups: &friendgroup.Server{RelationshipStore: store},
			}
			response := workspaceLookupResponseForTest(t, server, name)
			if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
				t.Fatalf("missing private Workspace response = %#v, want NOT_FOUND", response)
			}
			if catalog.listCalls != 0 || len(store.keys) != 0 || catalog.created != 0 {
				t.Fatalf("missing private lookup enumerated %d pages, read %d Social keys, created %d Workspaces", catalog.listCalls, len(store.keys), catalog.created)
			}
		})
	}
}

func TestWorkspaceGetMissingPrivateNameAllowsNativeCreateWithUnavailableSocial(t *testing.T) {
	ctx := t.Context()
	caller, foreign := giznet.PublicKey{1}, giznet.PublicKey{2}
	workflows := workflowtest.New(t)
	createWorkflowForCollectionTest(t, ctx, workflows, "canonical-workflow")
	workspaces := &workspace.Server{DB: workspacetest.New(t).DB, Workflows: workflows}
	store := &workspaceLookupStore{Store: kv.NewMemory(nil), getErr: errors.New("Social backend is unavailable")}
	t.Cleanup(func() { _ = store.Close() })
	profile := runtimeProfileWithWorkspaceAlias("r1")
	server := &Server{
		Caller: foreign, Workspaces: workspaces, Workflows: workflows,
		Friends:        &friend.Server{Friends: store},
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	create := rpcapi.WorkspaceCreateBody{Name: "h106-chat", WorkflowName: "journey"}
	callWorkspaceCreate(t, ctx, server, create)
	server.Caller = caller
	response := workspaceLookupResponseForTest(t, server, create.Name)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("new device GET = %#v, want NOT_FOUND before native create", response)
	}
	callWorkspaceCreate(t, ctx, server, create)
	got := callWorkspaceGet(t, ctx, server, create.Name)
	if got.Value.Name != create.Name || !got.Value.Available {
		t.Fatalf("GET after native create = %#v, want available personal Workspace", got)
	}
	own, err := workspaces.GetWorkspaceByName(ownership.WithOwner(ctx, caller.String()), create.Name)
	if err != nil {
		t.Fatal(err)
	}
	other, err := workspaces.GetWorkspaceByName(ownership.WithOwner(ctx, foreign.String()), create.Name)
	if err != nil {
		t.Fatal(err)
	}
	if own.Id == other.Id || socialutil.StringValue(own.OwnerPublicKey) != caller.String() || socialutil.StringValue(other.OwnerPublicKey) != foreign.String() {
		t.Fatalf("same-named personal Workspaces crossed owner boundaries: own=%#v, other=%#v", own, other)
	}
	if len(store.keys) != 0 {
		t.Fatalf("personal GET/create/GET read %d Social keys", len(store.keys))
	}
}

func TestWorkspaceGetOwnedNameTakesPrecedenceOverSFUShape(t *testing.T) {
	caller := giznet.PublicKey{1}
	name := socialutil.GroupWorkspaceName("private-name-collision")
	catalog := newWorkspaceLookupCatalog(caller.String(), name)
	store := &workspaceLookupStore{Store: kv.NewMemory(nil), getErr: errors.New("Social backend is unavailable")}
	t.Cleanup(func() { _ = store.Close() })
	server := &Server{Caller: caller, Workspaces: catalog, Friends: &friend.Server{Friends: store}}
	got, err := server.getWorkspaceByName(server.ownerContext(t.Context()), name)
	if err != nil || got.Id != catalog.item.Id {
		t.Fatalf("owned lookup = %#v, %v; want owned ordinary Workspace", got, err)
	}
	if catalog.listCalls != 0 || len(store.keys) != 0 {
		t.Fatalf("owned lookup enumerated %d pages and read %d Social keys", catalog.listCalls, len(store.keys))
	}
}

func TestWorkspaceGetUnknownSFUNameUsesOnlyExactLocators(t *testing.T) {
	for _, test := range []struct {
		name   string
		getErr error
		code   rpcapi.StatusCode
		reads  int
	}{
		{name: "missing", code: rpcapi.StatusCodeNotFound, reads: 2},
		{name: "backend failure", getErr: errors.New("Social read failed"), code: rpcapi.StatusCodeInternal, reads: 1},
		{name: "canceled", getErr: context.Canceled, code: rpcapi.StatusCodeInternal, reads: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			name := socialutil.GroupWorkspaceName("missing-group")
			catalog := newWorkspaceLookupCatalog((giznet.PublicKey{2}).String(), name)
			store := &workspaceLookupStore{Store: kv.NewMemory(nil), getErr: test.getErr}
			t.Cleanup(func() { _ = store.Close() })
			server := &Server{
				Caller: giznet.PublicKey{1}, Workspaces: catalog,
				Friends:      &friend.Server{Friends: store},
				FriendGroups: &friendgroup.Server{RelationshipStore: store},
			}
			response := workspaceLookupResponseForTest(t, server, name)
			if response.Error == nil || response.Error.Code != test.code {
				t.Fatalf("SFU lookup response = %#v, want code %v", response, test.code)
			}
			if catalog.listCalls != 0 || len(store.keys) != test.reads || catalog.created != 0 {
				t.Fatalf("SFU lookup enumerated %d pages, read %d keys, created %d Workspaces", catalog.listCalls, len(store.keys), catalog.created)
			}
			for _, key := range store.keys {
				if !slices.Equal(key, socialutil.WorkspaceLocatorNameKey(name)) {
					t.Fatalf("lookup key = %v, want exact Workspace name locator", key)
				}
			}
		})
	}
}

func TestWorkspaceGetSharedSFUUsesBindingWithoutEnumeration(t *testing.T) {
	ctx := t.Context()
	owner, member, stranger := giznet.PublicKey{1}, giznet.PublicKey{2}, giznet.PublicKey{3}
	catalog := newWorkspaceLookupCatalog(owner.String(), "")
	store := kv.NewMemory(nil)
	t.Cleanup(func() { _ = store.Close() })
	friends := &friend.Server{Friends: store, Workspaces: catalog, SFUURL: "wss://sfu.test"}
	relation, err := friends.AdminCreateFriend(ctx, owner.String(), member.String())
	if err != nil {
		t.Fatal(err)
	}
	name := socialutil.StringValue(relation.WorkspaceName)
	profile := apitypes.RuntimeProfile{}
	server := &Server{
		Caller: member, Workspaces: catalog, Friends: friends,
		RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile },
	}
	response := workspaceLookupResponseForTest(t, server, name)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("member SFU GET = %#v, want success", response)
	}
	got, err := response.Result.AsWorkspaceGetResponse()
	if err != nil || got.Value.Name != name {
		t.Fatalf("member SFU GET result = %#v, %v", got, err)
	}
	server.Caller = stranger
	response = workspaceLookupResponseForTest(t, server, name)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("stranger SFU GET = %#v, want NOT_FOUND", response)
	}
	if _, err := friends.DeleteFriend(ctx, owner.String(), rpcapi.FriendDeleteRequest{Name: member.String()}); err != nil {
		t.Fatal(err)
	}
	server.Caller = member
	response = workspaceLookupResponseForTest(t, server, name)
	if response.Error == nil || response.Error.Code != rpcapi.StatusCodeNotFound {
		t.Fatalf("revoked member SFU GET = %#v, want NOT_FOUND", response)
	}
	if catalog.listCalls != 0 {
		t.Fatalf("SFU GET enumerated %d global Workspace pages", catalog.listCalls)
	}
}

func workspaceLookupResponseForTest(t *testing.T, server *Server, name string) *rpcapi.RPCResponse {
	t.Helper()
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkspaceGetRequest(rpcapi.WorkspaceGetRequest{Name: name}); err != nil {
		t.Fatal(err)
	}
	return server.handleWorkspaceGet(t.Context(), &rpcapi.RPCRequest{Id: "get", Params: &payload})
}

type workspaceLookupCatalog struct {
	*sharedWorkspaceNameService
	listCalls int
}

func newWorkspaceLookupCatalog(owner, name string) *workspaceLookupCatalog {
	return &workspaceLookupCatalog{sharedWorkspaceNameService: &sharedWorkspaceNameService{
		owner: owner,
		item: apitypes.Workspace{
			Id: "1111111111111111", Name: name, OwnerPublicKey: &owner,
			WorkflowId: "ordinary-workflow", System: new(false),
		},
	}}
}

func (s *workspaceLookupCatalog) ListWorkspaces(context.Context, adminhttp.ListWorkspacesRequestObject) (adminhttp.ListWorkspacesResponseObject, error) {
	s.listCalls++
	return nil, errors.New("global Workspace enumeration is forbidden")
}

type workspaceLookupStore struct {
	kv.Store
	keys   []kv.Key
	getErr error
}

func (s *workspaceLookupStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.keys = append(s.keys, slices.Clone(key))
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.Store.Get(ctx, key)
}
