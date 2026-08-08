package social

import (
	"context"
	"errors"
	"strings"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
)

// PeerSnapshot is persisted by the Peer coordinator before Social mutation.
type PeerSnapshot struct {
	PublicKey string                            `json:"public_key"`
	Contacts  []contact.PeerRetirementContact   `json:"contacts"`
	Friends   []friend.PeerRetirementFriend     `json:"friends"`
	Groups    []friendgroup.PeerRetirementGroup `json:"groups"`
}

type PeerRetirementResult struct {
	WorkspaceIDs   []string `json:"workspace_ids"`
	FriendGroupIDs []string `json:"friend_group_ids"`
}

type PeerRetirement struct {
	Contacts     *contact.Server
	Friends      *friend.Server
	FriendGroups *friendgroup.Server
}

func (r PeerRetirement) SnapshotPeerSocial(ctx context.Context, publicKey string) (PeerSnapshot, error) {
	if publicKey == "" || publicKey != strings.TrimSpace(publicKey) {
		return PeerSnapshot{}, errors.New("social: Peer public key is required and must be canonical")
	}
	if r.Contacts == nil || r.Friends == nil || r.FriendGroups == nil {
		return PeerSnapshot{}, errors.New("social: Peer retirement services are not configured")
	}
	contacts, err := r.Contacts.SnapshotPeerContacts(ctx, publicKey)
	if err != nil {
		return PeerSnapshot{}, err
	}
	friends, err := r.Friends.SnapshotPeerFriends(ctx, publicKey)
	if err != nil {
		return PeerSnapshot{}, err
	}
	groups, err := r.FriendGroups.SnapshotPeerGroups(ctx, publicKey)
	if err != nil {
		return PeerSnapshot{}, err
	}
	return PeerSnapshot{PublicKey: publicKey, Contacts: contacts, Friends: friends, Groups: groups}, nil
}

func (r PeerRetirement) RetirePeerSocial(ctx context.Context, snapshot PeerSnapshot) (PeerRetirementResult, error) {
	if snapshot.PublicKey == "" || snapshot.PublicKey != strings.TrimSpace(snapshot.PublicKey) {
		return PeerRetirementResult{}, errors.New("social: invalid Peer retirement snapshot")
	}
	if r.Contacts == nil || r.Friends == nil || r.FriendGroups == nil {
		return PeerRetirementResult{}, errors.New("social: Peer retirement services are not configured")
	}
	result := PeerRetirementResult{}
	for _, item := range snapshot.Contacts {
		if item.Owner != snapshot.PublicKey {
			return PeerRetirementResult{}, errors.New("social: Contact snapshot owner mismatch")
		}
		if err := r.Contacts.RetirePeerContact(ctx, item); err != nil {
			return PeerRetirementResult{}, err
		}
	}
	for _, item := range snapshot.Friends {
		if snapshot.PublicKey != item.FirstPeer && snapshot.PublicKey != item.SecondPeer {
			return PeerRetirementResult{}, errors.New("social: Friend snapshot owner mismatch")
		}
		if err := r.Friends.RetirePeerFriend(ctx, snapshot.PublicKey, item); err != nil {
			return PeerRetirementResult{}, err
		}
		result.WorkspaceIDs = append(result.WorkspaceIDs, item.WorkspaceID)
	}
	for _, item := range snapshot.Groups {
		if item.PeerPublicKey != snapshot.PublicKey {
			return PeerRetirementResult{}, errors.New("social: Friend Group snapshot member mismatch")
		}
		if err := r.FriendGroups.RetirePeerGroup(ctx, item); err != nil {
			return PeerRetirementResult{}, err
		}
		if item.OwnerPublicKey == snapshot.PublicKey {
			result.WorkspaceIDs = append(result.WorkspaceIDs, item.WorkspaceID)
			result.FriendGroupIDs = append(result.FriendGroupIDs, item.FriendGroupID)
		}
	}
	return result, nil
}
