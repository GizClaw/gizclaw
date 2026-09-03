package socialutil

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	// SFUWorkflowID is the built-in system Workflow every Server materializes
	// for Friend and Friend Group SFU Workspaces.
	SFUWorkflowID = "system-sfu"
	// FriendGroupMemberLimit is the fixed member cap (including the owner).
	FriendGroupMemberLimit = 10
)

// SFUWorkspaceKind names which Social resource owns an SFU Workspace.
type SFUWorkspaceKind string

const (
	SFUWorkspaceKindFriend      SFUWorkspaceKind = "friend"
	SFUWorkspaceKindFriendGroup SFUWorkspaceKind = "friend_group"
)

// SFUBinding is the persisted SFU Room binding shared by every Server through
// the Social KV. RoomToken is a public stable Room identity, never a
// credential.
type SFUBinding struct {
	URL        string `json:"url"`
	RoomToken  string `json:"room_token"`
	Generation uint64 `json:"generation"`
}

// Validate reports whether the binding carries the final required identity.
func (b SFUBinding) Validate() error {
	if strings.TrimSpace(b.URL) == "" {
		return fmt.Errorf("social: SFU binding url is required")
	}
	if strings.TrimSpace(b.RoomToken) == "" {
		return fmt.Errorf("social: SFU binding room_token is required")
	}
	return nil
}

// SFUWorkspaceBinding is the authoritative answer to "may this Peer attach to
// this Workspace's SFU Room right now".
type SFUWorkspaceBinding struct {
	WorkspaceID   string
	WorkspaceName string
	Kind          SFUWorkspaceKind
	// SocialID is the relation ID for Friends or the canonical Group ID.
	SocialID string
	// Owner is the public key recorded as the system Workspace owner. Every
	// Server materializes its local Workspace copy with this owner so the
	// copies stay identical across the deployment.
	Owner string
	// Members lists the current member public keys of the Social resource.
	Members []string
	SFU     SFUBinding
}

// NewRoomToken mints a random opaque Room identity.
func NewRoomToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("social: mint SFU room token: %w", err)
	}
	return "room-" + hex.EncodeToString(buf[:]), nil
}
