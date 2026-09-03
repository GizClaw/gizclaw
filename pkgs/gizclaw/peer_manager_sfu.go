package gizclaw

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// sfuBindings returns the composite resolver that answers whether a Peer may
// attach to a Workspace's SFU Room right now. Friend bindings are consulted
// first, then Friend Group bindings; a Workspace neither service owns resolves
// to sfu.ErrNotBound.
func (m *Manager) sfuBindings() managerSFUBindings {
	return managerSFUBindings{manager: m}
}

type managerSFUBindings struct {
	manager *Manager
}

// sfuBindingNameResolver resolves an SFU binding by the Peer-visible Workspace
// name. The Social services own the name-to-binding mapping in the shared
// Social KV, so a run selection can be checked without consulting the
// Server-local Workspace catalog, whose name index is scoped by owner.
type sfuBindingNameResolver interface {
	ResolveSFUWorkspaceBindingByName(context.Context, string, string) (socialutil.SFUWorkspaceBinding, error)
}

func (r managerSFUBindings) ResolveSFUWorkspaceBinding(
	ctx context.Context,
	workspaceID, peerPublicKey string,
) (socialutil.SFUWorkspaceBinding, error) {
	m := r.manager
	if m == nil {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrNotBound
	}
	if m.sfuBindingResolver != nil {
		return m.sfuBindingResolver.ResolveSFUWorkspaceBinding(ctx, workspaceID, peerPublicKey)
	}
	return r.resolve(func(resolver sfu.BindingResolver) (socialutil.SFUWorkspaceBinding, error) {
		return resolver.ResolveSFUWorkspaceBinding(ctx, workspaceID, peerPublicKey)
	})
}

// ResolveSFUWorkspaceBindingByName is ResolveSFUWorkspaceBinding keyed by the
// Peer-visible Workspace name of the run selection.
func (r managerSFUBindings) ResolveSFUWorkspaceBindingByName(
	ctx context.Context,
	workspaceName, peerPublicKey string,
) (socialutil.SFUWorkspaceBinding, error) {
	m := r.manager
	if m == nil {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrNotBound
	}
	if m.sfuBindingResolver != nil {
		byName, ok := m.sfuBindingResolver.(sfuBindingNameResolver)
		if !ok {
			return socialutil.SFUWorkspaceBinding{}, errors.New("gizclaw: SFU binding resolver cannot resolve Workspace names")
		}
		return byName.ResolveSFUWorkspaceBindingByName(ctx, workspaceName, peerPublicKey)
	}
	return r.resolve(func(resolver sfu.BindingResolver) (socialutil.SFUWorkspaceBinding, error) {
		byName, ok := resolver.(sfuBindingNameResolver)
		if !ok {
			return socialutil.SFUWorkspaceBinding{}, kv.ErrNotFound
		}
		return byName.ResolveSFUWorkspaceBindingByName(ctx, workspaceName, peerPublicKey)
	})
}

// resolve consults the Friend service first, then the Friend Group service.
// A Workspace neither service owns resolves to sfu.ErrNotBound.
func (r managerSFUBindings) resolve(lookup func(sfu.BindingResolver) (socialutil.SFUWorkspaceBinding, error)) (socialutil.SFUWorkspaceBinding, error) {
	m := r.manager
	var resolvers []sfu.BindingResolver
	if m.Friends != nil {
		resolvers = append(resolvers, m.Friends)
	}
	if m.FriendGroups != nil {
		resolvers = append(resolvers, m.FriendGroups)
	}
	for _, resolver := range resolvers {
		binding, err := lookup(resolver)
		if err == nil {
			return binding, nil
		}
		if !errors.Is(err, kv.ErrNotFound) {
			return socialutil.SFUWorkspaceBinding{}, err
		}
	}
	return socialutil.SFUWorkspaceBinding{}, sfu.ErrNotBound
}

// sfuInputAccess classifies one inbound turn for the named Workspace. The
// first result reports whether the Workspace is an SFU Workspace; the second is
// the denial the Peer must receive, or nil when input may flow. The check
// reads the authoritative Social KV by Workspace name: the Server-local
// catalog indexes Social Workspaces under their creator's scope, so a lookup
// on behalf of another member would not find them.
func (m *Manager) sfuInputAccess(
	ctx context.Context,
	caller giznet.PublicKey,
	workspaceName string,
) (bool, *inputAccessError) {
	if m == nil {
		return false, sfuAccessCheckFailedError()
	}
	_, err := m.sfuBindings().ResolveSFUWorkspaceBindingByName(ctx, strings.TrimSpace(workspaceName), caller.String())
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, sfu.ErrNotBound):
		return false, nil
	case errors.Is(err, sfu.ErrNotMember), errors.Is(err, sfu.ErrRevoked):
		return true, sfuAccessRevokedError()
	default:
		return true, sfuAccessCheckFailedError()
	}
}

// allowSFURestrictedReload admits a reload of a Workspace the Peer does not
// own when the Peer is a current member of the Social resource bound to it.
// SFU Workspaces are the only membership-scoped Workspaces.
func (m *Manager) allowSFURestrictedReload(ctx context.Context, caller giznet.PublicKey, workspaceName string) bool {
	if m == nil {
		return false
	}
	_, err := m.sfuBindings().ResolveSFUWorkspaceBindingByName(ctx, strings.TrimSpace(workspaceName), caller.String())
	return err == nil
}
