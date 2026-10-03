package agenthost

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
)

func retainedMemoryResolver(t *testing.T, profile func(context.Context, string) (apitypes.RuntimeProfile, error)) (ServiceResolver, apitypes.Workspace) {
	t.Helper()
	alias := apitypes.WorkflowMemoryAlias("pet-memory")
	memoryWorkflow := mustWorkflow(t, "workflow-1")
	memoryWorkflow.Spec.Memory = &alias
	owner := "owner-public-key"
	retained := systemWorkspace("retiring", "workflow-1", nil)
	retained.OwnerPublicKey = &owner
	return ServiceResolver{
		// The availability gate rejects the retiring Workspace; the retained
		// resolver must read the retained Admin row instead.
		Workspaces: fakeWorkspaceService{
			items:           map[string]apitypes.Workspace{retained.Id: retained},
			availabilityErr: workspace.ErrWorkspacePendingDeletion,
		},
		Workflows:              fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow-1": memoryWorkflow}},
		MemoryLayouts:          fakeMemoryLayoutService{item: apitypes.MemoryLayout{Id: "pet-layout"}},
		RuntimeProfileForOwner: profile,
	}, retained
}

func memoryProfile(aliases ...string) apitypes.RuntimeProfile {
	bindings := make(map[string]apitypes.RuntimeProfileMemoryBinding, len(aliases))
	for _, alias := range aliases {
		bindings[alias] = apitypes.RuntimeProfileMemoryBinding{LayoutId: "pet-layout", Driver: apitypes.RuntimeProfileMemoryDriverFlowcraft}
	}
	return apitypes.RuntimeProfile{
		Id: "owner-profile", Revision: "revision-1",
		Spec: apitypes.RuntimeProfileSpec{Resources: apitypes.RuntimeProfileResources{Memories: &bindings}},
	}
}

func TestResolveRetainedMemoryByIDResolvesPendingDeletionWorkspace(t *testing.T) {
	resolver, retained := retainedMemoryResolver(t, func(context.Context, string) (apitypes.RuntimeProfile, error) {
		return memoryProfile("pet-memory"), nil
	})
	if _, err := resolver.ResolveMemoryByID(t.Context(), retained.Id); !errors.Is(err, workspace.ErrWorkspacePendingDeletion) {
		t.Fatalf("ResolveMemoryByID() error = %v, want the availability gate", err)
	}
	spec, err := resolver.ResolveRetainedMemoryByID(t.Context(), retained.Id)
	if err != nil {
		t.Fatalf("ResolveRetainedMemoryByID() error = %v", err)
	}
	if spec.Workspace.Id != retained.Id || spec.MemoryName != "pet-memory" || spec.MemoryBinding == nil ||
		spec.MemoryLayout == nil || spec.MemoryProfileID != "owner-profile" || spec.MemoryProfileRevision != "revision-1" {
		t.Fatalf("ResolveRetainedMemoryByID() = %#v", spec)
	}
}

func TestResolveRetainedMemoryByIDClassifiesMissingBindings(t *testing.T) {
	for _, tc := range []struct {
		name    string
		edit    func(*ServiceResolver, *apitypes.Workspace)
		profile func(context.Context, string) (apitypes.RuntimeProfile, error)
	}{
		{
			name: "workspace finalized",
			edit: func(r *ServiceResolver, _ *apitypes.Workspace) {
				r.Workspaces = fakeWorkspaceService{}
			},
		},
		{
			name: "workflow deleted",
			edit: func(r *ServiceResolver, _ *apitypes.Workspace) {
				r.Workflows = fakeWorkflowService{}
			},
		},
		{
			name: "owner binding deleted",
			profile: func(_ context.Context, owner string) (apitypes.RuntimeProfile, error) {
				return apitypes.RuntimeProfile{}, fmt.Errorf("resolve owner %q: %w", owner, sql.ErrNoRows)
			},
		},
		{
			name: "memory alias removed",
			profile: func(context.Context, string) (apitypes.RuntimeProfile, error) {
				return memoryProfile("other-memory"), nil
			},
		},
		{
			name: "ownerless memory workflow",
			edit: func(r *ServiceResolver, retained *apitypes.Workspace) {
				retained.OwnerPublicKey = nil
				r.Workspaces = fakeWorkspaceService{items: map[string]apitypes.Workspace{retained.Id: *retained}}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile := tc.profile
			if profile == nil {
				profile = func(context.Context, string) (apitypes.RuntimeProfile, error) {
					return memoryProfile("pet-memory"), nil
				}
			}
			resolver, retained := retainedMemoryResolver(t, profile)
			if tc.edit != nil {
				tc.edit(&resolver, &retained)
			}
			_, err := resolver.ResolveRetainedMemoryByID(t.Context(), retained.Id)
			if !errors.Is(err, ErrMemoryBindingNotFound) {
				t.Fatalf("ResolveRetainedMemoryByID() error = %v, want ErrMemoryBindingNotFound", err)
			}
		})
	}
}

func TestResolveRetainedMemoryByIDKeepsTransientFailuresRetryable(t *testing.T) {
	resolver, retained := retainedMemoryResolver(t, func(context.Context, string) (apitypes.RuntimeProfile, error) {
		return apitypes.RuntimeProfile{}, errors.New("runtime profile database is locked")
	})
	_, err := resolver.ResolveRetainedMemoryByID(t.Context(), retained.Id)
	if err == nil || errors.Is(err, ErrMemoryBindingNotFound) {
		t.Fatalf("ResolveRetainedMemoryByID() error = %v, want a transient failure", err)
	}
}

func TestResolveRetainedMemoryByIDSkipsOwnerProfileWithoutWorkflowMemory(t *testing.T) {
	resolver, retained := retainedMemoryResolver(t, func(context.Context, string) (apitypes.RuntimeProfile, error) {
		t.Fatal("owner RuntimeProfile resolved for a Workflow without Memory")
		return apitypes.RuntimeProfile{}, nil
	})
	resolver.Workflows = fakeWorkflowService{items: map[string]apitypes.Workflow{"workflow-1": mustWorkflow(t, "workflow-1")}}
	spec, err := resolver.ResolveRetainedMemoryByID(t.Context(), retained.Id)
	if err != nil || spec.MemoryBinding != nil || spec.Workspace.Id != retained.Id {
		t.Fatalf("ResolveRetainedMemoryByID() = %#v, %v; want no Memory binding", spec, err)
	}
}
