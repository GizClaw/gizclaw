package customid

import (
	"fmt"
	"net/url"
	"strings"
)

func OwnerScopedName(owner, id string) string {
	return owner + ":" + id
}

func SplitOwnerScopedName(name string) (string, string, error) {
	owner, id, ok := strings.Cut(name, ":")
	if !ok || owner == "" || id == "" {
		return "", "", fmt.Errorf("resource id must be owner_public_key:id")
	}
	if err := ValidateField("id", id); err != nil {
		return "", "", err
	}
	return owner, id, nil
}

func MembershipName(groupID, memberID string) string {
	return escapeMembershipComponent(groupID) + ":" + escapeMembershipComponent(memberID)
}

// ValidateMembershipName ensures the deterministic composite can itself be
// used as a ResourceMetadata ID.
func ValidateMembershipName(groupID, memberID string) error {
	if err := ValidateFriendGroupID(groupID); err != nil {
		return fmt.Errorf("friend group id: %w", err)
	}
	if err := ValidateResourceID(MembershipName(groupID, memberID)); err != nil {
		return fmt.Errorf("friend group member id: %w", err)
	}
	return nil
}

func escapeMembershipComponent(value string) string {
	return strings.ReplaceAll(url.PathEscape(value), ":", "%3A")
}

func SplitMembershipName(name string) (string, string, error) {
	groupSegment, memberSegment, ok := strings.Cut(name, ":")
	if !ok || groupSegment == "" || memberSegment == "" || strings.Contains(memberSegment, ":") {
		return "", "", fmt.Errorf("resource id must be friend_group_id:peer_public_key")
	}
	groupID, err := url.PathUnescape(groupSegment)
	if err != nil {
		return "", "", fmt.Errorf("resource id has invalid friend_group_id escaping: %w", err)
	}
	memberID, err := url.PathUnescape(memberSegment)
	if err != nil {
		return "", "", fmt.Errorf("resource id has invalid peer_public_key escaping: %w", err)
	}
	if groupID == "" || memberID == "" || MembershipName(groupID, memberID) != name {
		return "", "", fmt.Errorf("resource id must be friend_group_id:peer_public_key")
	}
	return groupID, memberID, nil
}
