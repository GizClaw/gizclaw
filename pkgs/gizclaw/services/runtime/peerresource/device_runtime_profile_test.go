package peerresource

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ownerProfileStub struct {
	profiles map[string]apitypes.RuntimeProfile
	err      error
	calls    []string
}

func (s *ownerProfileStub) ResolveOwnerProfile(_ context.Context, owner string) (apitypes.RuntimeProfile, error) {
	s.calls = append(s.calls, owner)
	if s.err != nil {
		return apitypes.RuntimeProfile{}, s.err
	}
	profile, ok := s.profiles[owner]
	if !ok {
		return apitypes.RuntimeProfile{}, sql.ErrNoRows
	}
	return profile, nil
}

func runtimeProfileCatalogFixture() apitypes.RuntimeProfile {
	return apitypes.RuntimeProfile{
		Id: "h106-tiga", Revision: "rev-1",
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{
			Collections: apitypes.RuntimeProfileWorkflowCollections{
				"story-teller": {
					"story.alice": collectionTestBinding("runtime-alice", "Alice"),
					"story.aesop": collectionTestBinding("runtime-aesop", "Aesop"),
				},
				"games": {"game.riddle": collectionTestBinding("runtime-riddle", "Riddle")},
				"empty": {},
			},
		}},
	}
}

func TestDeviceRuntimeProfileSortsCollectionsAndWorkflowNames(t *testing.T) {
	owner := giznet.PublicKey{21}
	other := giznet.PublicKey{22}
	profiles := &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{
		owner.String(): runtimeProfileCatalogFixture(),
		other.String(): {Id: "other-profile", Revision: "rev-9"},
	}}
	got, err := DeviceReads{Caller: owner, Profiles: profiles}.DeviceRuntimeProfile(context.Background())
	if err != nil {
		t.Fatalf("DeviceRuntimeProfile() error = %v", err)
	}
	want := peerhttp.DeviceRuntimeProfile{
		Name: "h106-tiga", Revision: "rev-1",
		Collections: []peerhttp.DeviceRuntimeProfileCollection{
			{Name: "empty", Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{}},
			{Name: "games", Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "game.riddle"}}},
			{Name: "story-teller", Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "story.aesop"}, {Name: "story.alice"}}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeviceRuntimeProfile() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(profiles.calls, []string{owner.String()}) {
		t.Fatalf("resolved owners = %#v, want only the caller", profiles.calls)
	}
}

func TestDeviceRuntimeProfileWithoutWorkflowsReturnsEmptyCollections(t *testing.T) {
	owner := giznet.PublicKey{23}
	profiles := &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{owner.String(): {Id: "bare", Revision: "rev-0"}}}
	got, err := DeviceReads{Caller: owner, Profiles: profiles}.DeviceRuntimeProfile(context.Background())
	if err != nil {
		t.Fatalf("DeviceRuntimeProfile() error = %v", err)
	}
	if got.Collections == nil || len(got.Collections) != 0 {
		t.Fatalf("collections = %#v, want an empty non-nil slice", got.Collections)
	}
}

func TestDeviceRuntimeProfileErrors(t *testing.T) {
	owner := giznet.PublicKey{24}
	reads := DeviceReads{Caller: owner, Profiles: &ownerProfileStub{}}
	if _, err := reads.DeviceRuntimeProfile(context.Background()); !errors.Is(err, ErrDeviceRuntimeProfileNotBound) {
		t.Fatalf("unbound error = %v, want ErrDeviceRuntimeProfileNotBound", err)
	}
	storeErr := errors.New("store down")
	reads.Profiles = &ownerProfileStub{err: storeErr}
	if _, err := reads.DeviceRuntimeProfile(context.Background()); !errors.Is(err, storeErr) || errors.Is(err, ErrDeviceRuntimeProfileNotBound) {
		t.Fatalf("store error = %v, want the store failure", err)
	}
}

// When every bound Workflow exists, the HTTP catalog names the same profile,
// revision, and workflows that server.workflow.list returns to the device.
func TestDeviceRuntimeProfileMatchesWorkflowListRPC(t *testing.T) {
	ctx := context.Background()
	owner := giznet.PublicKey{25}
	workflows := workflowtest.New(t)
	for _, name := range []string{"runtime-alice", "runtime-aesop", "runtime-riddle"} {
		createWorkflowForCollectionTest(t, ctx, workflows, name)
	}
	profile := runtimeProfileCatalogFixture()
	catalog, err := DeviceReads{Caller: owner, Profiles: &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{owner.String(): profile}}}.DeviceRuntimeProfile(ctx)
	if err != nil {
		t.Fatalf("DeviceRuntimeProfile() error = %v", err)
	}
	server := &Server{Caller: owner, Workflows: workflows, RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	for _, collection := range catalog.Collections {
		params := rpcapi.RPCPayload{}
		if err := params.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Collection: collection.Name}); err != nil {
			t.Fatal(err)
		}
		response := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: "list", Params: &params})
		if response.Error != nil || response.Result == nil {
			t.Fatalf("workflow.list %q = %#v", collection.Name, response)
		}
		listed, err := response.Result.AsWorkflowListResponse()
		if err != nil {
			t.Fatal(err)
		}
		if listed.RuntimeProfileName != catalog.Name || listed.RuntimeProfileRevision != catalog.Revision {
			t.Fatalf("RPC profile = %q@%q, HTTP catalog = %q@%q", listed.RuntimeProfileName, listed.RuntimeProfileRevision, catalog.Name, catalog.Revision)
		}
		rpcNames := make([]string, len(listed.Items))
		for i, item := range listed.Items {
			rpcNames[i] = item.Name
		}
		httpNames := make([]string, len(collection.Workflows))
		for i, item := range collection.Workflows {
			httpNames[i] = item.Name
		}
		if !reflect.DeepEqual(rpcNames, httpNames) {
			t.Fatalf("collection %q RPC names = %#v, HTTP names = %#v", collection.Name, rpcNames, httpNames)
		}
	}
}
