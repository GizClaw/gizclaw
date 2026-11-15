package social

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/contact"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
)

func TestPeerSnapshotCoversLifecycleInventory(t *testing.T) {
	created := time.Now()
	saved := PeerSnapshot{
		PublicKey: "owner",
		Contacts:  []contact.PeerRetirementContact{{Owner: "owner", ID: "contact", Name: "contact-name", Incarnation: "contact-first"}},
		Friends:   []friend.PeerRetirementFriend{{RelationID: "owner:peer", FirstPeer: "owner", SecondPeer: "peer", WorkspaceID: "friend-workspace", WorkspaceName: "friend-name"}},
		Groups: []friendgroup.PeerRetirementGroup{{
			FriendGroupID: "group", WorkspaceID: "group-workspace", WorkspaceName: "group-name", OwnerPublicKey: "peer",
			PeerPublicKey: "owner", FriendGroupName: "local-name", Role: rpcapi.FriendGroupMemberRoleMember, CreatedAt: &created,
		}},
	}
	clone := func() PeerSnapshot {
		data, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot PeerSnapshot
		if err := json.Unmarshal(data, &snapshot); err != nil {
			t.Fatal(err)
		}
		return snapshot
	}
	if !saved.Covers(clone()) {
		t.Fatal("serialization changed coverage through time pointers or monotonic clock state")
	}
	if !saved.Covers(PeerSnapshot{PublicKey: "owner"}) {
		t.Fatal("already removed saved resources prevent coverage")
	}
	for _, scenario := range []struct {
		name   string
		change func(*PeerSnapshot)
	}{
		{"different-peer", func(s *PeerSnapshot) { s.PublicKey = "other" }},
		{"new-contact", func(s *PeerSnapshot) { s.Contacts[0].ID = "fresh" }},
		{"replaced-contact", func(s *PeerSnapshot) { s.Contacts[0].Incarnation = "contact-second" }},
		{"new-friend", func(s *PeerSnapshot) { s.Friends[0].RelationID = "new-pair" }},
		{"replaced-friend-workspace", func(s *PeerSnapshot) { s.Friends[0].WorkspaceID = "new-workspace" }},
		{"new-group", func(s *PeerSnapshot) { s.Groups[0].FriendGroupID = "fresh" }},
		{"replaced-membership", func(s *PeerSnapshot) { s.Groups[0].CreatedAt = new(created.Add(time.Second)) }},
		{"membership-identity-unknown", func(s *PeerSnapshot) { s.Groups[0].CreatedAt = nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			current := clone()
			scenario.change(&current)
			if saved.Covers(current) {
				t.Fatal("saved plan covers an omitted or different lifecycle")
			}
		})
	}
	old := clone()
	old.Groups[0].CreatedAt = nil
	if old.Covers(saved) {
		t.Fatal("old snapshot without membership lifecycle covers a current membership")
	}
	if (PeerSnapshot{}).Covers(PeerSnapshot{}) {
		t.Fatal("empty Peer identity covers an inventory")
	}
}

func TestPeerSnapshotDoesNotCoverFreshOmittedResources(t *testing.T) {
	saved := PeerSnapshot{PublicKey: "owner"}
	for _, current := range []PeerSnapshot{
		{PublicKey: "owner", Contacts: []contact.PeerRetirementContact{{Owner: "owner", ID: "contact", Name: "name", Incarnation: "first"}}},
		{PublicKey: "owner", Friends: []friend.PeerRetirementFriend{{FirstPeer: "owner", SecondPeer: "peer", RelationID: "owner:peer", WorkspaceID: "workspace", WorkspaceName: "name"}}},
		{PublicKey: "owner", Groups: []friendgroup.PeerRetirementGroup{{PeerPublicKey: "owner", FriendGroupID: "group", WorkspaceID: "workspace", WorkspaceName: "name"}}},
	} {
		if saved.Covers(current) {
			t.Fatalf("empty saved inventory covered %+v", current)
		}
	}
}
