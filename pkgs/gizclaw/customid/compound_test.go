package customid

import (
	"strings"
	"testing"
)

func TestOwnerScopedName(t *testing.T) {
	if got := OwnerScopedName("owner-key", "contact01"); got != "owner-key:contact01" {
		t.Fatalf("OwnerScopedName = %q", got)
	}
}

func TestSplitOwnerScopedName(t *testing.T) {
	owner, id, err := SplitOwnerScopedName("PeerKeyWithMixedCase:contact01")
	if err != nil {
		t.Fatalf("SplitOwnerScopedName: %v", err)
	}
	if owner != "PeerKeyWithMixedCase" || id != "contact01" {
		t.Fatalf("SplitOwnerScopedName = %q, %q", owner, id)
	}
}

func TestSplitOwnerScopedNameRejectsInvalidCustomIDSegment(t *testing.T) {
	if _, _, err := SplitOwnerScopedName("PeerKey:short"); err == nil {
		t.Fatal("SplitOwnerScopedName accepted short id")
	}
}

func TestMembershipName(t *testing.T) {
	if got := MembershipName("family01", "PeerKeyWithMixedCase"); got != "family01:PeerKeyWithMixedCase" {
		t.Fatalf("MembershipName = %q", got)
	}
}

func TestMembershipNameEscapesOpaqueComponents(t *testing.T) {
	got := MembershipName("family:blue/team", "Peer:Key/One")
	if got != "family%3Ablue%2Fteam:Peer%3AKey%2FOne" {
		t.Fatalf("MembershipName = %q", got)
	}
	groupID, memberID, err := SplitMembershipName(got)
	if err != nil {
		t.Fatalf("SplitMembershipName: %v", err)
	}
	if groupID != "family:blue/team" || memberID != "Peer:Key/One" {
		t.Fatalf("SplitMembershipName = %q, %q", groupID, memberID)
	}
}

func TestValidateMembershipNameBoundsDerivedID(t *testing.T) {
	groupID := strings.Repeat("😀", MaxFriendGroupIDCharacters)
	peerID := strings.Repeat("z", 44)
	if err := ValidateMembershipName(groupID, peerID); err != nil {
		t.Fatalf("ValidateMembershipName(maximum group ID): %v", err)
	}
	if got := MembershipName(groupID, peerID); len(got) > MaxResourceIDCharacters {
		t.Fatalf("MembershipName length = %d, want <= %d", len(got), MaxResourceIDCharacters)
	}
	if err := ValidateMembershipName(strings.Repeat("😀", MaxFriendGroupIDCharacters+1), peerID); err == nil {
		t.Fatal("ValidateMembershipName accepted oversized FriendGroup ID")
	}
	if err := ValidateMembershipName("family", strings.Repeat("/", MaxResourceIDCharacters)); err == nil {
		t.Fatal("ValidateMembershipName accepted oversized derived ID")
	}
}

func TestSplitMembershipName(t *testing.T) {
	groupID, memberID, err := SplitMembershipName("family01:PeerKeyWithMixedCase")
	if err != nil {
		t.Fatalf("SplitMembershipName: %v", err)
	}
	if groupID != "family01" || memberID != "PeerKeyWithMixedCase" {
		t.Fatalf("SplitMembershipName = %q, %q", groupID, memberID)
	}
}

func TestSplitMembershipNameAcceptsOpaqueGroupID(t *testing.T) {
	groupID, memberID, err := SplitMembershipName("01K1HZZZ9PV2KYRHZJ4V94Z0DQ:PeerKey")
	if err != nil {
		t.Fatalf("SplitMembershipName: %v", err)
	}
	if groupID != "01K1HZZZ9PV2KYRHZJ4V94Z0DQ" || memberID != "PeerKey" {
		t.Fatalf("SplitMembershipName = %q, %q", groupID, memberID)
	}
}

func TestSplitMembershipNameRejectsExtraSeparator(t *testing.T) {
	if _, _, err := SplitMembershipName("family01:Peer:Key"); err == nil {
		t.Fatal("SplitMembershipName accepted extra separator")
	}
}
