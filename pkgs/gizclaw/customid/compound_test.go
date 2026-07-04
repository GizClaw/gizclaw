package customid

import "testing"

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

func TestSplitMembershipName(t *testing.T) {
	groupID, memberID, err := SplitMembershipName("family01:PeerKeyWithMixedCase")
	if err != nil {
		t.Fatalf("SplitMembershipName: %v", err)
	}
	if groupID != "family01" || memberID != "PeerKeyWithMixedCase" {
		t.Fatalf("SplitMembershipName = %q, %q", groupID, memberID)
	}
}

func TestSplitMembershipNameRejectsInvalidGroupIDSegment(t *testing.T) {
	if _, _, err := SplitMembershipName("family:PeerKey"); err == nil {
		t.Fatal("SplitMembershipName accepted short group id")
	}
}

func TestSplitMembershipNameRejectsExtraSeparator(t *testing.T) {
	if _, _, err := SplitMembershipName("family01:Peer:Key"); err == nil {
		t.Fatal("SplitMembershipName accepted extra separator")
	}
}
