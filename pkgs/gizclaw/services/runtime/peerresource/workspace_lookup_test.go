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
			store := &workspaceLookupStore{Store: kv.NewMemory(nil), readErr: errors.New("Social backend is unavailable")}
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
	store := &workspaceLookupStore{Store: kv.NewMemory(nil), readErr: errors.New("Social backend is unavailable")}
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
	store := &workspaceLookupStore{Store: kv.NewMemory(nil), readErr: errors.New("Social backend is unavailable")}
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
		name    string
		readErr error
		code    rpcapi.StatusCode
		reads   int
	}{
		{name: "missing", code: rpcapi.StatusCodeNotFound, reads: 2},
		{name: "backend failure", readErr: errors.New("Social read failed"), code: rpcapi.StatusCodeInternal, reads: 1},
		{name: "canceled", readErr: context.Canceled, code: rpcapi.StatusCodeInternal, reads: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			name := socialutil.GroupWorkspaceName("missing-group")
			catalog := newWorkspaceLookupCatalog((giznet.PublicKey{2}).String(), name)
			store := &workspaceLookupStore{Store: kv.NewMemory(nil), readErr: test.readErr}
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
			for _, method := range store.methods {
				if method != "Get" {
					t.Fatalf("SFU lookup used %s, want exact locator Get", method)
				}
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
	keys    []kv.Key
	methods []string
	readErr error
}

func (s *workspaceLookupStore) recordRead(method string, key kv.Key) {
	s.keys = append(s.keys, slices.Clone(key))
	s.methods = append(s.methods, method)
}

func (s *workspaceLookupStore) Get(ctx context.Context, key kv.Key) ([]byte, error) {
	s.recordRead("Get", key)
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.Store.Get(ctx, key)
}

func (s *workspaceLookupStore) BatchGet(ctx context.Context, keys []kv.Key) []kv.GetResult {
	for _, key := range keys {
		s.recordRead("BatchGet", key)
	}
	if s.readErr != nil {
		results := make([]kv.GetResult, len(keys))
		for i := range results {
			results[i].Err = s.readErr
		}
		return results
	}
	return s.Store.BatchGet(ctx, keys)
}

func (s *workspaceLookupStore) HasMember(ctx context.Context, key kv.Key, member string) (bool, error) {
	s.recordRead("HasMember", key)
	if s.readErr != nil {
		return false, s.readErr
	}
	return s.Store.HasMember(ctx, key, member)
}

func (s *workspaceLookupStore) ListMembers(ctx context.Context, key kv.Key) ([]string, error) {
	s.recordRead("ListMembers", key)
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.Store.ListMembers(ctx, key)
}

func (s *workspaceLookupStore) RangeOrderedMembers(ctx context.Context, key kv.Key, query kv.OrderedRange) ([]string, error) {
	s.recordRead("RangeOrderedMembers", key)
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.Store.RangeOrderedMembers(ctx, key, query)
}

func TestWorkspaceLookupStoreCapturesEveryRead(t *testing.T) {
	for _, test := range []struct {
		name  string
		count int
		read  func(context.Context, *workspaceLookupStore, kv.Key) []error
	}{
		{"Get", 1, func(ctx context.Context, store *workspaceLookupStore, key kv.Key) []error {
			_, err := store.Get(ctx, key)
			return []error{err}
		}},
		{"BatchGet", 2, func(ctx context.Context, store *workspaceLookupStore, key kv.Key) []error {
			results := store.BatchGet(ctx, []kv.Key{key, key})
			errs := make([]error, len(results))
			for i, result := range results {
				errs[i] = result.Err
			}
			return errs
		}},
		{"HasMember", 1, func(ctx context.Context, store *workspaceLookupStore, key kv.Key) []error {
			_, err := store.HasMember(ctx, key, "member")
			return []error{err}
		}},
		{"ListMembers", 1, func(ctx context.Context, store *workspaceLookupStore, key kv.Key) []error {
			_, err := store.ListMembers(ctx, key)
			return []error{err}
		}},
		{"RangeOrderedMembers", 1, func(ctx context.Context, store *workspaceLookupStore, key kv.Key) []error {
			_, err := store.RangeOrderedMembers(ctx, key, kv.OrderedRange{Limit: 1})
			return []error{err}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			unavailable := errors.New("Social storage unavailable")
			store := &workspaceLookupStore{Store: kv.NewMemory(nil), readErr: unavailable}
			t.Cleanup(func() { _ = store.Close() })
			key := kv.Key{"probe"}
			errs := test.read(t.Context(), store, key)
			key[0] = "mutated"
			if len(errs) != test.count || len(store.keys) != test.count || len(store.methods) != test.count {
				t.Fatalf("read returned %d errors, recorded %d keys and %d methods", len(errs), len(store.keys), len(store.methods))
			}
			for i, err := range errs {
				if !errors.Is(err, unavailable) || store.methods[i] != test.name || !slices.Equal(store.keys[i], kv.Key{"probe"}) {
					t.Fatalf("read %d: error=%v method=%q key=%v", i, err, store.methods[i], store.keys[i])
				}
			}
		})
	}
}
