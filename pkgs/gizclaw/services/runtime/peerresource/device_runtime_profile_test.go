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
	runtimeindex "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

type ownerProfileStub struct {
	profiles map[string]apitypes.RuntimeProfile
	err      error
	calls    []string
}

type runtimeProfileSourceStub struct{ profile apitypes.RuntimeProfile }

func (source runtimeProfileSourceStub) ForEachProfile(_ context.Context, consume func(apitypes.RuntimeProfile) error) error {
	return consume(source.profile)
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
	alice := collectionTestBinding("runtime-alice", "Alice")
	alice.Tags = &[]string{"6-8", "stories"}
	aesop := collectionTestBinding("runtime-aesop", "Aesop")
	aesop.Tags = &[]string{"9-12", "stories"}
	riddle := collectionTestBinding("runtime-riddle", "Riddle")
	riddle.Tags = &[]string{"6-8", "games"}
	return apitypes.RuntimeProfile{
		Id: "h106-tiga", Revision: "rev-1",
		Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{

			"story.alice": alice,
			"story.aesop": aesop,
			"game.riddle": riddle,
		}},
	}
}

func TestDeviceRuntimeProfileSortsWorkflowNames(t *testing.T) {
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
		Workflows: []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "game.riddle", Tags: []string{"6-8", "games"}}, {Name: "story.aesop", Tags: []string{"9-12", "stories"}}, {Name: "story.alice", Tags: []string{"6-8", "stories"}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DeviceRuntimeProfile() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(profiles.calls, []string{owner.String()}) {
		t.Fatalf("resolved owners = %#v, want only the caller", profiles.calls)
	}
}

func TestDeviceRuntimeProfileWithoutWorkflowsReturnsEmptyWorkflows(t *testing.T) {
	owner := giznet.PublicKey{23}
	profiles := &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{owner.String(): {Id: "bare", Revision: "rev-0"}}}
	got, err := DeviceReads{Caller: owner, Profiles: profiles}.DeviceRuntimeProfile(context.Background())
	if err != nil {
		t.Fatalf("DeviceRuntimeProfile() error = %v", err)
	}
	if got.Workflows == nil || len(got.Workflows) != 0 {
		t.Fatalf("workflows = %#v, want an empty non-nil slice", got.Workflows)
	}
}

func TestDeviceRuntimeProfileFiltersAllTags(t *testing.T) {
	owner := giznet.PublicKey{26}
	profile := runtimeProfileCatalogFixture()
	reads := DeviceReads{Caller: owner, Profiles: &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{owner.String(): profile}}}
	got, err := reads.DeviceRuntimeProfileWithTags(context.Background(), []string{"6-8", "stories"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Workflows, []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "story.alice", Tags: []string{"6-8", "stories"}}}) {
		t.Fatalf("filtered workflows = %#v", got.Workflows)
	}
}

func TestDeviceRuntimeProfileUsesSQLiteSnapshotForTags(t *testing.T) {
	owner := giznet.PublicKey{27}
	profile := runtimeProfileCatalogFixture()
	index := runtimeindex.New(runtimeProfileSourceStub{profile: profile}, 0)
	if err := index.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = index.Close() })
	changed := profile.Spec.Workflows["story.alice"]
	changed.Tags = &[]string{"changed"}
	profile.Spec.Workflows["story.alice"] = changed
	reads := DeviceReads{Caller: owner, Profiles: &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{owner.String(): profile}}, Index: index}
	got, err := reads.DeviceRuntimeProfileWithTags(t.Context(), []string{"6-8", "stories"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Workflows, []peerhttp.DeviceRuntimeProfileWorkflow{{Name: "story.alice", Tags: []string{"6-8", "stories"}}}) {
		t.Fatalf("SQLite filtered workflows = %#v", got.Workflows)
	}
	workflows := workflowtest.New(t)
	createWorkflowForCollectionTest(t, t.Context(), workflows, "runtime-alice")
	var payload rpcapi.RPCPayload
	if err := payload.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Tags: []string{"6-8", "stories"}}); err != nil {
		t.Fatal(err)
	}
	server := &Server{Index: index, Workflows: workflows, RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
	response := server.handleWorkflowList(t.Context(), &rpcapi.RPCRequest{Id: "sqlite-list", Params: &payload})
	if response.Error != nil || response.Result == nil {
		t.Fatalf("SQLite RPC list = %#v", response)
	}
	listed, err := response.Result.AsWorkflowListResponse()
	if err != nil || len(listed.Items) != 1 || listed.Items[0].Name != "story.alice" {
		t.Fatalf("SQLite RPC workflows = %#v, %v", listed, err)
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
	filter := rpcapi.RPCPayload{}
	if err := filter.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Tags: []string{"6-8", "stories"}}); err != nil {
		t.Fatal(err)
	}
	filteredResponse := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: "filtered", Params: &filter})
	if filteredResponse.Error != nil || filteredResponse.Result == nil {
		t.Fatalf("filtered workflow list = %#v", filteredResponse)
	}
	filtered, err := filteredResponse.Result.AsWorkflowListResponse()
	if err != nil || len(filtered.Items) != 1 || filtered.Items[0].Name != "story.alice" {
		t.Fatalf("filtered workflow list = %#v, %v", filtered, err)
	}
	limit := 1
	pageRequest := rpcapi.RPCPayload{}
	if err := pageRequest.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Tags: []string{"6-8"}, Limit: &limit}); err != nil {
		t.Fatal(err)
	}
	firstResponse := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: "first", Params: &pageRequest})
	if firstResponse.Error != nil || firstResponse.Result == nil {
		t.Fatalf("first filtered page response = %#v", firstResponse)
	}
	first, err := firstResponse.Result.AsWorkflowListResponse()
	if err != nil || !first.HasNext || first.NextCursor == nil {
		t.Fatalf("first filtered page = %#v, %v", first, err)
	}
	pageRequest = rpcapi.RPCPayload{}
	if err := pageRequest.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Tags: []string{"6-8"}, Limit: &limit, Cursor: first.NextCursor}); err != nil {
		t.Fatal(err)
	}
	secondResponse := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: "second", Params: &pageRequest})
	if secondResponse.Error != nil || secondResponse.Result == nil {
		t.Fatalf("second filtered page response = %#v", secondResponse)
	}
	second, err := secondResponse.Result.AsWorkflowListResponse()
	if err != nil || len(second.Items) != 1 || second.HasNext || second.Items[0].Name == first.Items[0].Name {
		t.Fatalf("second filtered page = %#v, %v", second, err)
	}
	{
		params := rpcapi.RPCPayload{}
		if err := params.FromWorkflowListRequest(rpcapi.WorkflowListRequest{}); err != nil {
			t.Fatal(err)
		}
		response := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: "list", Params: &params})
		if response.Error != nil || response.Result == nil {
			t.Fatalf("workflow.list = %#v", response)
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
		httpNames := make([]string, len(catalog.Workflows))
		for i, item := range catalog.Workflows {
			httpNames[i] = item.Name
		}
		if !reflect.DeepEqual(rpcNames, httpNames) {
			t.Fatalf("RPC names = %#v, HTTP names = %#v", rpcNames, httpNames)
		}
	}
}
