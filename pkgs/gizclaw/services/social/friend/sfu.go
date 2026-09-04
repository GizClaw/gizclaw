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
	return s.resolveSFUBinding(ctx, peerPublicKey, func(binding workspaceBinding) bool {
		return binding.WorkspaceID == workspaceID
	}, func(intent retirementIntent) bool {
		return intent.WorkspaceID == workspaceID
	}, func(receipt retirementReceipt) bool {
		return receipt.WorkspaceID == workspaceID
	})
}

// ResolveSFUWorkspaceBindingByName is ResolveSFUWorkspaceBinding keyed by the
// Peer-visible Workspace name instead of the canonical ID.
func (s *Server) ResolveSFUWorkspaceBindingByName(ctx context.Context, workspaceName, peerPublicKey string) (socialutil.SFUWorkspaceBinding, error) {
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return socialutil.SFUWorkspaceBinding{}, errors.New("social: workspace name is required")
	}
	return s.resolveSFUBinding(ctx, peerPublicKey, func(binding workspaceBinding) bool {
		return binding.WorkspaceName == workspaceName
	}, func(intent retirementIntent) bool {
		return intent.WorkspaceName == workspaceName
	}, func(receipt retirementReceipt) bool {
		return receipt.WorkspaceName == workspaceName
	})
}

func (s *Server) resolveSFUBinding(
	ctx context.Context,
	peerPublicKey string,
	matchBinding func(workspaceBinding) bool,
	matchIntent func(retirementIntent) bool,
	matchReceipt func(retirementReceipt) bool,
) (socialutil.SFUWorkspaceBinding, error) {
	store, err := s.friendsStore()
	if err != nil {
		return socialutil.SFUWorkspaceBinding{}, err
	}
	peerPublicKey = strings.TrimSpace(peerPublicKey)
	if peerPublicKey == "" {
		return socialutil.SFUWorkspaceBinding{}, errors.New("social: peer public key is required")
	}
	for entry, err := range store.List(ctx, workspaceBindingsRoot) {
		if err != nil {
			return socialutil.SFUWorkspaceBinding{}, err
		}
		if len(entry.Key) != len(workspaceBindingsRoot)+1 {
			continue
		}
		relationID := socialutil.UnescapeStoreSegment(entry.Key[len(workspaceBindingsRoot)])
		binding, err := readWorkspaceBinding(ctx, store, relationID)
		if errors.Is(err, kv.ErrNotFound) {
			continue
		}
		if err != nil {
			return socialutil.SFUWorkspaceBinding{}, err
		}
		if !matchBinding(binding) {
			continue
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
	for entry, err := range store.List(ctx, retirementIntentsRoot) {
		if err != nil {
			return socialutil.SFUWorkspaceBinding{}, err
		}
		var intent retirementIntent
		if err := unmarshalEntry(entry.Value, &intent); err != nil {
			return socialutil.SFUWorkspaceBinding{}, err
		}
		if matchIntent(intent) {
			return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
		}
	}
	for entry, err := range store.List(ctx, retirementReceiptsRoot) {
		if err != nil {
			return socialutil.SFUWorkspaceBinding{}, err
		}
		var receipt retirementReceipt
		if err := unmarshalEntry(entry.Value, &receipt); err != nil {
			return socialutil.SFUWorkspaceBinding{}, err
		}
		if matchReceipt(receipt) {
			return socialutil.SFUWorkspaceBinding{}, sfu.ErrRevoked
		}
	}
	return socialutil.SFUWorkspaceBinding{}, kv.ErrNotFound
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
	out := make([]socialutil.SFUWorkspaceBinding, 0)
	for entry, err := range store.List(ctx, socialutil.OwnerPrefix(socialutil.FriendsRoot, peerPublicKey)) {
		if err != nil {
			return nil, err
		}
		var record friendRecord
		if err := unmarshalEntry(entry.Value, &record); err != nil {
			return nil, err
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
