package gizclaw

import (
	"context"
	"errors"
	"fmt"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/agenthost"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
)

type retainedMemoryResolver interface {
	ResolveRetainedMemoryByID(context.Context, string) (agenthost.Spec, error)
}

// workspaceMemoryCleanup purges a deleted Workspace's long-term memory from
// the physical backend of its current Memory binding. The binding is resolved
// from the retained Workspace row and the owner's current RuntimeProfile, so
// memory written under an earlier binding is not reached. A Workspace with no
// resolvable binding has nothing to purge.
type workspaceMemoryCleanup struct {
	Resolver   retainedMemoryResolver
	Stores     *memorystore.Registry
	ServerRoot string
}

func (c workspaceMemoryCleanup) PurgeWorkspaceMemory(ctx context.Context, workspaceID string) error {
	request, ok, err := c.request(ctx, workspaceID)
	if err != nil || !ok {
		return err
	}
	return c.Stores.PurgeWorkspace(ctx, request)
}

func (c workspaceMemoryCleanup) WorkspaceMemoryAbsent(ctx context.Context, workspaceID string) (bool, error) {
	request, ok, err := c.request(ctx, workspaceID)
	if err != nil || !ok {
		return err == nil, err
	}
	return c.Stores.WorkspaceMemoryEmpty(ctx, request)
}

func (c workspaceMemoryCleanup) request(ctx context.Context, workspaceID string) (memorystore.Request, bool, error) {
	if c.Resolver == nil || c.Stores == nil {
		return memorystore.Request{}, false, errors.New("gizclaw: Workspace memory cleanup is not configured")
	}
	spec, err := c.Resolver.ResolveRetainedMemoryByID(ctx, workspaceID)
	if errors.Is(err, agenthost.ErrMemoryBindingNotFound) {
		return memorystore.Request{}, false, nil
	}
	if err != nil {
		return memorystore.Request{}, false, fmt.Errorf("gizclaw: resolve Workspace %q memory binding: %w", workspaceID, err)
	}
	if spec.MemoryBinding == nil || spec.MemoryLayout == nil {
		return memorystore.Request{}, false, nil
	}
	return memorystore.Request{
		WorkspaceID:     workspaceID,
		ProfileID:       spec.MemoryProfileID,
		ProfileRevision: spec.MemoryProfileRevision,
		BindingName:     spec.MemoryName,
		Layout:          *spec.MemoryLayout,
		Binding:         *spec.MemoryBinding,
		ServerRoot:      c.ServerRoot,
	}, true, nil
}
