package friend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// ResolveSFUWorkspaceBinding returns the authoritative SFU binding of the
// Friend Workspace identified by workspaceID for peerPublicKey. It reports
// kv.ErrNotFound when no Friend relationship ever bound the Workspace,
// sfu.ErrRevoked when the relationship retired, and sfu.ErrNotMember when the
// Peer is not one of the two current relationship members.
func (s *Server) ResolveSFUWorkspaceBinding(ctx context.Context, workspaceID, peerPublicKey string) (socialutil.SFUWorkspaceBinding, error) {
	if err := customid.ValidateResourceID(workspaceID); err != nil {
		return socialutil.SFUWorkspaceBinding{}, fmt.Errorf("social: invalid workspace id: %w", err)
	}
	return s.resolveSFUBinding(ctx, peerPublicKey, socialutil.WorkspaceLocatorIDKey(workspaceID))
}

// ResolveSFUWorkspaceBindingByName is ResolveSFUWorkspaceBinding keyed by the
// Peer-visible Workspace name instead of the canonical ID.
func (s *Server) ResolveSFUWorkspaceBindingByName(ctx context.Context, workspaceName, peerPublicKey string) (socialutil.SFUWorkspaceBinding, error) {
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return socialutil.SFUWorkspaceBinding{}, errors.New("social: workspace name is required")
	}
	return s.resolveSFUBinding(ctx, peerPublicKey, socialutil.WorkspaceLocatorNameKey(workspaceName))
}

func (s *Server) resolveSFUBinding(ctx context.Context, peerPublicKey string, key kv.Key) (socialutil.SFUWorkspaceBinding, error) {
	store, err := s.friendsStore()
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return socialutil.SFUWorkspaceBinding{}, errors.New("social: peer public key is required")
	}
	locator, err := socialutil.ReadJSONValue[socialutil.WorkspaceBindingLocator](ctx, store, key)
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	if err := locator.Validate(); err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	if !slices.Equal(key, socialutil.WorkspaceLocatorIDKey(locator.WorkspaceID)) && !slices.Equal(key, socialutil.WorkspaceLocatorNameKey(locator.WorkspaceName)) {
		return socialutil.SFUWorkspaceBinding{}, errors.New("social: workspace locator identity mismatch")
	}
	binding, err := readWorkspaceBinding(ctx, store, locator.ResourceID)
	if errors.Is(err, kv.ErrNotFound) {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
	}
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	if binding.RelationID != locator.ResourceID || binding.WorkspaceID != locator.WorkspaceID || binding.WorkspaceName != locator.WorkspaceName {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
	}
	first, second, ok := relationPeers(binding.RelationID)
	if !ok {
		return socialutil.SFUWorkspaceBinding{}, fmt.Errorf("social: invalid Friend relation id %q", binding.RelationID)
	}
	item, active, err := readActiveRelationship(ctx, store, first, second)
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	if !active || socialutil.StringValue(item.WorkspaceName) != binding.WorkspaceName {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
	}
	members := []string{first, second}
	if !slices.Contains(members, peerPublicKey) {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrNotMember
	}
	return binding.sfuWorkspaceBinding(members), nil
}

// ListSFUWorkspaceBindingsForPeer returns the SFU Workspace bindings of every
// active Friend relationship of peerPublicKey. Servers use it to materialize
// the Peer's Social Workspaces in their local catalog.
func (s *Server) ListSFUWorkspaceBindingsForPeer(ctx context.Context, peerPublicKey string) ([]socialutil.SFUWorkspaceBinding, error) {
	store, err := s.friendsStore()
	if err != nil {
		return nil, err
	}
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return nil, errors.New("social: peer public key is required")
	}
	ids, err := store.ListMembers(ctx, friendCollectionKey(peerPublicKey))
	if err != nil {
		return nil, err
	}
	slices.Sort(ids)
	out := make([]socialutil.SFUWorkspaceBinding, 0, len(ids))
	for _, id := range ids {
		record, err := socialutil.ReadJSONValue[friendRecord](ctx, store, socialutil.FriendKey(peerPublicKey, id))
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if record.RelationID != id {
			return nil, errors.New("social: friend collection identity mismatch")
		}
		if err := record.validate(); err != nil {
			return nil, err
		}
		binding, err := readWorkspaceBinding(ctx, store, record.RelationID)
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if binding.WorkspaceName != record.WorkspaceName {
			return nil, errors.New("social: Friend Workspace binding is inconsistent")
		}
		first, second, ok := relationPeers(record.RelationID)
		if !ok {
			return nil, fmt.Errorf("social: invalid Friend relation id %q", record.RelationID)
		}
		out = append(out, binding.sfuWorkspaceBinding([]string{first, second}))
	}
	return out, nil
}

func relationPeers(relationID string) (string, string, bool) {
	first, second, ok := strings.Cut(strings.TrimSpace(relationID), ":")
	if !ok || first == "" || second == "" || first > second {
		return "", "", false
	}
	return first, second, true
}

func unmarshalEntry(data []byte, out any) error {
	return json.Unmarshal(data, out)
}
