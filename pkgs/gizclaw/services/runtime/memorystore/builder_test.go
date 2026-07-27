package memorystore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/store/memory"
)

func TestManagedBindingRootUsesServerWorkspaceProfileAndAlias(t *testing.T) {
	root := t.TempDir()
	got, err := managedBindingRoot(root, "default", "pet-memory")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "data", "memory", "default", "pet-memory")
	if got != want {
		t.Fatalf("managedBindingRoot() = %q, want %q", got, want)
	}
}

func TestManagedBindingRootRejectsUnsafeAndSymlinkPaths(t *testing.T) {
	root := t.TempDir()
	for _, value := range []string{"", ".", "..", "../escape", "a/b", `a\b`} {
		if _, err := managedBindingRoot(root, value, "memory"); err == nil {
			t.Errorf("managedBindingRoot(profile=%q) succeeded", value)
		}
		if _, err := managedBindingRoot(root, "profile", value); err == nil {
			t.Errorf("managedBindingRoot(alias=%q) succeeded", value)
		}
	}
	realRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := managedBindingRoot(linkRoot, "profile", "memory"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("managedBindingRoot(symlink) error = %v", err)
	}
}

func TestProjectionSignatureExcludesExtractionAndWritePolicy(t *testing.T) {
	policy := testFlowcraftPolicy()
	before, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	policy.Extraction.SystemPrompt = new("changed")
	policy.Write.Mode = apitypes.FlowcraftMemoryWritePolicyModeAsyncSemantic
	after, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("extraction/write policy changed derived-index identity")
	}
	policy.Bbh.SearchOverfetch = new(8)
	changed, err := projectionSignature(policy)
	if err != nil {
		t.Fatal(err)
	}
	if before == changed {
		t.Fatal("BBH policy did not change derived-index identity")
	}
}

func TestBuildManagedFlowcraftStoreKeepsWorkspaceScopesIsolated(t *testing.T) {
	request := managedTestRequest(t)
	request.WorkspaceName = "workspace-a"
	request.BindingName = "memory"
	result, err := Build(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Closer != nil {
		t.Cleanup(func() { _ = result.Closer.Close() })
	}
	if _, err := result.Store.Observe(context.Background(), memory.Observation{
		Scope: memory.Scope{AppID: "workspace-a"},
		Text:  "Mochi likes salmon.",
	}); err != nil {
		t.Fatal(err)
	}
	recallA, err := result.Store.Recall(context.Background(), memory.Query{
		Scope: memory.Scope{AppID: "workspace-a"},
		Text:  "salmon",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recallA.Matches) == 0 {
		t.Fatal("workspace-a did not recall its fact")
	}
	recallB, err := result.Store.Recall(context.Background(), memory.Query{
		Scope: memory.Scope{AppID: "workspace-b"},
		Text:  "salmon",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recallB.Matches) != 0 {
		t.Fatalf("workspace-b recalled %d workspace-a facts", len(recallB.Matches))
	}
}

func TestManagedFlowcraftProjectionRebuildPreservesCanonicalFacts(t *testing.T) {
	request := managedTestRequest(t)
	first, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Store.Observe(t.Context(), memory.Observation{
		Scope: memory.Scope{AppID: request.WorkspaceName},
		Text:  "Mochi likes salmon.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := first.Closer.Close(); err != nil {
		t.Fatal(err)
	}

	request.Layout.Spec.Flowcraft.Bbh.SearchOverfetch = new(7)
	second, err := Build(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Closer.Close() })
	recalled, err := second.Store.Recall(t.Context(), memory.Query{
		Scope: memory.Scope{AppID: request.WorkspaceName},
		Text:  "salmon",
		Limit: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(recalled.Matches) == 0 {
		t.Fatal("derived-index rebuild lost canonical Workspace facts")
	}
}

func testFlowcraftPolicy() apitypes.FlowcraftMemoryLayoutPolicy {
	return apitypes.FlowcraftMemoryLayoutPolicy{
		Extraction: apitypes.FlowcraftMemoryExtractionPolicy{
			Mode: apitypes.FlowcraftMemoryExtractionPolicyModeTwoPass,
		},
		Bbh: apitypes.FlowcraftMemoryBBHPolicy{
			SearchOverfetch: new(2),
		},
		Lanes: []apitypes.FlowcraftMemoryLanePolicy{{
			Name: "facts",
			Kind: apitypes.FlowcraftMemoryLanePolicyKindNote,
		}},
		Write: apitypes.FlowcraftMemoryWritePolicy{
			Mode: apitypes.FlowcraftMemoryWritePolicyModeSync,
			Tier: apitypes.FlowcraftMemoryWritePolicyTierGeneral,
		},
	}
}
