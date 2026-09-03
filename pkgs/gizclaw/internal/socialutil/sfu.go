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
	// FriendGroupMemberLimit is the fixed member cap, owner included. Every
	// participant decodes the other N-1 remote tracks, so Server-side decode
	// cost grows with the square of the room size; ten keeps one full room
	// affordable. It is deliberately not configurable.
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
// credential. Generation is the monotonic lifecycle identity of the binding:
// it starts at 1 and increases whenever the binding is replaced, so a running
// session detects a replacement and fails closed.
type SFUBinding struct {
	URL       string `json:"url"`
	RoomToken string `json:"room_token"`
	// Generation starts at 1 and advances only when the binding itself is
	// replaced, which today means a new Friend incarnation or a retirement.
	// Adding or removing an ordinary Friend Group member must leave it
	// untouched: every attached session compares the generation it captured
	// at attach time, so advancing it would revoke the members who stayed.
	// Losing one member is expressed by the per-Peer membership check
	// instead.
	Generation uint64 `json:"generation"`
}

// Validate reports whether the binding carries the final required identity.
// A zero Generation is rejected: it is either a record written before the
// field existed or a truncated one, and neither can be compared against a
// later replacement.
func (b SFUBinding) Validate() error {
	if strings.TrimSpace(b.URL) == "" {
		return fmt.Errorf("social: SFU binding url is required")
	}
	if strings.TrimSpace(b.RoomToken) == "" {
		return fmt.Errorf("social: SFU binding room_token is required")
	}
	if b.Generation == 0 {
		return fmt.Errorf("social: SFU binding generation is required")
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
