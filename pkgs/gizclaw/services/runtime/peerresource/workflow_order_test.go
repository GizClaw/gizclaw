package peerresource

import (
	"encoding/base64"
	"fmt"
	"reflect"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/workflowtest"
	runtimeindex "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/runtimeprofile"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
)

func TestProfileWorkflowOrderMatchesRPCAndHTTP(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		t.Run(fmt.Sprintf("indexed=%t", indexed), func(t *testing.T) {
			ctx := t.Context()
			workflows := workflowtest.New(t)
			profile := apitypes.RuntimeProfile{Id: "ordered", Revision: "rev-1", Spec: apitypes.RuntimeProfileSpec{Workflows: apitypes.RuntimeProfileWorkflows{}}}
			for i, alias := range []string{"guess.animals", "guess.body-health", "guess.history-figures-cn", "translate-ja-zh", "translate-ko-zh", "translate-zh-en-auto"} {
				id := fmt.Sprintf("workflow-%d", i)
				createWorkflowForCollectionTest(t, ctx, workflows, id)
				binding := collectionTestBinding(id, alias)
				tag := "category:learn"
				if i >= 3 {
					tag = "category:translates"
				}
				binding.Tags = &[]string{tag}
				if alias == "guess.history-figures-cn" || alias == "translate-zh-en-auto" {
					binding.SortOrder = new(int32(-1))
				}
				profile.Spec.Workflows[alias] = binding
			}
			server := &Server{Workflows: workflows, RuntimeProfile: func() *apitypes.RuntimeProfile { return &profile }}
			owner := giznet.PublicKey{41}
			reads := DeviceReads{Caller: owner, Profiles: &ownerProfileStub{profiles: map[string]apitypes.RuntimeProfile{owner.String(): profile}}}
			if indexed {
				index := runtimeindex.New(runtimeProfileSourceStub{profile: profile}, 0)
				if err := index.Initialize(ctx); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = index.Close() })
				server.Index, reads.Index = index, index
			}
			for category, want := range map[string][]string{
				"learn":      {"guess.history-figures-cn", "guess.animals", "guess.body-health"},
				"translates": {"translate-zh-en-auto", "translate-ja-zh", "translate-ko-zh"},
			} {
				tags := []string{"category:" + category}
				httpProfile, err := reads.DeviceRuntimeProfileWithTags(ctx, tags)
				if err != nil {
					t.Fatal(err)
				}
				httpNames := make([]string, 0, len(httpProfile.Workflows))
				for _, item := range httpProfile.Workflows {
					httpNames = append(httpNames, item.Name)
				}
				if !reflect.DeepEqual(httpNames, want) {
					t.Errorf("%s HTTP order = %v, want %v", category, httpNames, want)
				}
				var names []string
				var cursor *string
				for page := range 4 {
					var params rpcapi.RPCPayload
					if err := params.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Tags: tags, Limit: new(1), Cursor: cursor}); err != nil {
						t.Fatal(err)
					}
					response := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: fmt.Sprintf("page-%d", page), Params: &params})
					if response.Error != nil {
						t.Fatalf("RPC list error: %v", response.Error)
					}
					result, err := response.Result.AsWorkflowListResponse()
					if err != nil {
						t.Fatal(err)
					}
					for _, item := range result.Items {
						names = append(names, item.Name)
					}
					if !result.HasNext {
						break
					}
					cursor = result.NextCursor
				}
				if !reflect.DeepEqual(names, want) {
					t.Errorf("%s RPC pages = %v, want %v", category, names, want)
				}
				// A change to the configured order changes the Profile revision.
				stale := base64.RawURLEncoding.EncodeToString([]byte(workflowTagSelectorRevision("rev-0", tags) + "\x00" + want[0]))
				var params rpcapi.RPCPayload
				if err := params.FromWorkflowListRequest(rpcapi.WorkflowListRequest{Tags: tags, Cursor: &stale}); err != nil {
					t.Fatal(err)
				}
				response := server.handleWorkflowList(ctx, &rpcapi.RPCRequest{Id: "stale", Params: &params})
				if response.Error == nil || response.Error.Code != rpcapi.StatusCodeAborted {
					t.Fatalf("stale ordered cursor = %v, want ABORTED", response.Error)
				}
			}
		})
	}
}

func TestWorkflowOrderDefaultsAndSignedBounds(t *testing.T) {
	bindings := apitypes.RuntimeProfileWorkflows{
		"alpha": {}, "bravo": {SortOrder: new(int32(0))},
		"first": {SortOrder: new(int32(-2147483648))},
		"last":  {SortOrder: new(int32(2147483647))},
	}
	aliases := []string{"last", "bravo", "first", "alpha"}
	sortWorkflowAliases(aliases, bindings)
	if want := []string{"first", "alpha", "bravo", "last"}; !reflect.DeepEqual(aliases, want) {
		t.Fatalf("signed order = %v, want %v", aliases, want)
	}
}
