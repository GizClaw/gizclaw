package customid

import (
	"fmt"
	"strings"
)

func OwnerScopedName(owner, id string) string {
	return owner + ":" + id
}

func SplitOwnerScopedName(name string) (string, string, error) {
	owner, id, ok := strings.Cut(name, ":")
	if !ok || owner == "" || id == "" {
		return "", "", fmt.Errorf("metadata.name must be owner_public_key:id")
	}
	if err := ValidateField("id", id); err != nil {
		return "", "", err
	}
	return owner, id, nil
}

func MembershipName(groupID, memberID string) string {
	return groupID + ":" + memberID
}

func SplitMembershipName(name string) (string, string, error) {
	groupID, memberID, ok := strings.Cut(name, ":")
	if !ok || groupID == "" || memberID == "" {
		return "", "", fmt.Errorf("metadata.name must be friend_group_id:peer_public_key")
	}
	if strings.Contains(memberID, ":") {
		return "", "", fmt.Errorf("metadata.name must be friend_group_id:peer_public_key")
	}
	if err := ValidateField("friend_group_id", groupID); err != nil {
		return "", "", err
	}
	return groupID, memberID, nil
}
