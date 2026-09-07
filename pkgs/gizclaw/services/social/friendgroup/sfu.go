package friendgroup

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/customid"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// requireMemberCapacity rejects adding one more member when the Group already
// holds socialutil.FriendGroupMemberLimit members. Committing an admission
// additionally compares the shared group version captured before this read.
func (s *Server) requireMemberCapacity(ctx context.Context, friendGroupID string) error {
	members, err := s.memberPublicKeys(ctx, friendGroupID)
	if err != nil {
		return err
	}
	if len(members) >= socialutil.FriendGroupMemberLimit {
		return fmt.Errorf("%w: %d members", ErrFriendGroupFull, len(members))
	}
	return nil
}

// removeMember atomically deletes one membership together with its belongs
// and name projections. The Room and its binding generation are unchanged:
// the Group keeps the same Room for its remaining members, and the removed
// Peer loses access through the per-Peer membership check: new turns are
// refused at once, and the removed Peer's SFU runtime terminates on its next
// binding recheck.
func (s *Server) removeMember(ctx context.Context, friendGroupID, peerID string, current rpcapi.FriendGroupMemberObject) error {
	members, err := s.membersStore()
	if err != nil {
		return err
	}
	belongs, err := s.belongsStore()
	if err != nil {
		return err
	}
	// Resolve the transaction boundary and the key prefixes from the member
	// and belong stores themselves, exactly as createMember does. Reading the
	// prefixes off the configured Server fields instead would let a wiring
	// that binds those stores elsewhere delete keys nobody ever wrote, leaving
	// the membership in place and the removed Peer still able to talk.
	guard, err := s.readGroupMutation(ctx, friendGroupID, members, belongs)
	if err != nil {
		return err
	}
	prefixes := guard.prefixes
	memberKey := s.relationshipKey(prefixes[0], socialutil.GroupMemberKey(friendGroupID, peerID))
	memberData, err := guard.store.Get(ctx, memberKey)
	if errors.Is(err, kv.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	changed, err := guard.apply(ctx, kv.Mutation{
		Conditions:           []kv.Condition{{Key: memberKey, Expected: memberData}},
		DeleteKeys:           []kv.Key{s.relationshipKey(prefixes[0], socialutil.GroupMemberKey(friendGroupID, peerID)), s.relationshipKey(prefixes[1], socialutil.GroupBelongKey(peerID, friendGroupID)), s.relationshipKey(prefixes[1], socialutil.GroupNameKey(peerID, socialutil.StringValue(current.FriendGroupName)))},
		RemoveMembers:        []kv.SetMembers{{Key: s.relationshipKey(prefixes[0], memberCollectionKey(friendGroupID)), Members: []string{peerID}}, {Key: s.relationshipKey(prefixes[1], belongCollectionKey(peerID)), Members: []string{friendGroupID}}},
		RemoveOrderedMembers: []kv.SetMembers{{Key: s.relationshipKey(prefixes[1], belongPageKey(peerID)), Members: []string{socialutil.EscapeStoreSegment(friendGroupID)}}},
	}, false)
	if err != nil {
		return err
	}
	if !changed {
		return ErrGroupChanged
	}

	return nil
}

// ResolveSFUWorkspaceBinding returns the authoritative SFU binding of the
// Friend Group Workspace identified by workspaceID for peerPublicKey. It
// reports kv.ErrNotFound when no Group ever bound the Workspace,
// sfu.ErrRevoked when the Group retired or is pending deletion, and
// sfu.ErrNotMember when the Peer is not a current member.
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
	store, err := s.relationshipStore()
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
	binding, err := s.readWorkspaceBinding(ctx, locator.ResourceID)
	if errors.Is(err, kv.ErrNotFound) {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
	}
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	if binding.FriendGroupID != locator.ResourceID || binding.WorkspaceID != locator.WorkspaceID || binding.WorkspaceName != locator.WorkspaceName {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
	}
	return s.currentSFUBinding(ctx, binding, peerPublicKey)
}

// currentSFUBinding verifies that the Group behind binding is still live and
// that peerPublicKey is a current member before exposing the Room identity.
func (s *Server) currentSFUBinding(ctx context.Context, binding workspaceBinding, peerPublicKey string) (socialutil.SFUWorkspaceBinding, error) {
	groups, err := s.groupsStore()
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	group, err := socialutil.ReadJSONValue[rpcapi.FriendGroupObject](ctx, groups, socialutil.GroupKey(binding.FriendGroupID))
	if errors.Is(err, kv.ErrNotFound) {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
	}
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	if socialutil.StringValue(group.WorkspaceName) != binding.WorkspaceName {
		return socialutil.SFUWorkspaceBinding{}, errors.New("social: FriendGroup Workspace binding is inconsistent")
	}
	if err := s.rejectDataPendingDeletion(ctx, binding.FriendGroupID); err != nil {
		if errors.Is(err, errFriendGroupPendingDeletion) {
			return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
		}
		return socialutil.SFUWorkspaceBinding{}, err
	}
	store, err := s.membersStore()
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	members, err := store.ListMembers(ctx, memberCollectionKey(binding.FriendGroupID))
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	slices.Sort(members)
	if !slices.Contains(members, peerPublicKey) {
		return socialutil.SFUWorkspaceBinding{}, sfu.ErrNotMember
	}
	return binding.sfuWorkspaceBinding(members), nil
}

// ListSFUWorkspaceBindingsForPeer returns the SFU Workspace bindings of every
// Friend Group peerPublicKey currently belongs to. Servers use it to
// materialize the Peer's Social Workspaces in their local catalog.
func (s *Server) ListSFUWorkspaceBindingsForPeer(ctx context.Context, peerPublicKey string) ([]socialutil.SFUWorkspaceBinding, error) {
	belongs, err := s.belongsStore()
	if err != nil {
		return nil, err
	}
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return nil, errors.New("social: peer public key is required")
	}
	groupIDs, err := belongs.ListMembers(ctx, belongCollectionKey(peerPublicKey))
	if err != nil {
		return nil, err
	}
	slices.Sort(groupIDs)
	out := make([]socialutil.SFUWorkspaceBinding, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		binding, err := s.readWorkspaceBinding(ctx, groupID)
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		resolved, err := s.currentSFUBinding(ctx, binding, peerPublicKey)
		if errors.Is(err, sfu.ErrRevoked) || errors.Is(err, sfu.ErrNotMember) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, resolved)
	}
	return out, nil
}
