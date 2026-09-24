package gizclaw

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/memorystore"
)

type peerMemoryCleanup struct {
	ProfileForOwner func(context.Context, string) (apitypes.RuntimeProfile, error)
	Layouts         interface {
		GetMemoryLayout(context.Context, adminhttp.GetMemoryLayoutRequestObject) (adminhttp.GetMemoryLayoutResponseObject, error)
	}
	Stores     *memorystore.Registry
	ServerRoot string
}

func (c peerMemoryCleanup) PurgePeerMemory(ctx context.Context, owner string) error {
	requests, err := c.requests(ctx, owner)
	if err != nil {
		return err
	}
	for _, request := range requests {
		if err := c.Stores.PurgeWorkspace(ctx, request); err != nil {
			return fmt.Errorf("purge Peer memory binding %q: %w", request.BindingName, err)
		}
	}
	return nil
}

func (c peerMemoryCleanup) PeerMemoryAbsent(ctx context.Context, owner string) (bool, error) {
	requests, err := c.requests(ctx, owner)
	if err != nil {
		return false, err
	}
	for _, request := range requests {
		empty, err := c.Stores.WorkspaceMemoryEmpty(ctx, request)
		if err != nil {
			return false, fmt.Errorf("verify Peer memory binding %q: %w", request.BindingName, err)
		}
		if !empty {
			return false, nil
		}
	}
	return true, nil
}

func (c peerMemoryCleanup) requests(ctx context.Context, owner string) ([]memorystore.Request, error) {
	if c.ProfileForOwner == nil || c.Layouts == nil || c.Stores == nil {
		return nil, fmt.Errorf("Peer memory cleanup is not configured")
	}
	profile, err := c.ProfileForOwner(ctx, owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("resolve Peer memory profile: %w", err)
	}
	if profile.Spec.Resources.Memories == nil {
		return nil, nil
	}
	bindings := *profile.Spec.Resources.Memories
	names := make([]string, 0, len(bindings))
	for name := range bindings {
		names = append(names, name)
	}
	sort.Strings(names)
	requests := make([]memorystore.Request, 0, len(names))
	for _, name := range names {
		binding := bindings[name]
		response, err := c.Layouts.GetMemoryLayout(ctx, adminhttp.GetMemoryLayoutRequestObject{Id: binding.LayoutId})
		if err != nil {
			return nil, fmt.Errorf("get Peer memory layout %q: %w", binding.LayoutId, err)
		}
		layoutResponse, ok := response.(adminhttp.GetMemoryLayout200JSONResponse)
		if !ok {
			return nil, fmt.Errorf("Peer memory layout %q unavailable: %T", binding.LayoutId, response)
		}
		request := memorystore.Request{
			WorkspaceID: "peer-memory", OwnerPublicKey: owner,
			ProfileID: profile.Id, ProfileRevision: profile.Revision,
			BindingName: name, Binding: binding, Layout: apitypes.MemoryLayout(layoutResponse),
			ServerRoot: c.ServerRoot,
		}
		shared, err := memorystore.PeerScoped(request)
		if err != nil {
			return nil, err
		}
		if shared {
			requests = append(requests, request)
		}
	}
	return requests, nil
}
