package socialutil

import (
	"context"
	"time"
)

// PresenceService reads shared connection presence. lastSeenAt is the zero
// time when no transition has been observed or the presence read failed.
type PresenceService interface {
	PeerPresence(ctx context.Context, peerPublicKey string) (online bool, lastSeenAt time.Time)
}

// PresenceFields returns list-only presence fields. A nil service omits both
// fields; an unknown last-seen time is omitted and known times use UTC.
func PresenceFields(ctx context.Context, service PresenceService, peerPublicKey string) (*bool, *time.Time) {
	if service == nil {
		return nil, nil
	}
	online, lastSeenAt := service.PeerPresence(ctx, peerPublicKey)
	if lastSeenAt.IsZero() {
		return &online, nil
	}
	return &online, new(lastSeenAt.UTC())
}

// RoomPresenceService reports whether a Peer's device is running a Workspace.
// Running a Friend Group's Workspace is what attaches the
// device to the Group's SFU Room, so the answer implies the Peer is online.
type RoomPresenceService interface {
	PeerRunningWorkspace(ctx context.Context, peerPublicKey, workspaceName string) bool
}

// InRoomField returns the list-only in_room field for a member of the Social
// resource bound to workspaceName. A nil service or an empty Workspace name
// omits the field.
func InRoomField(ctx context.Context, service RoomPresenceService, peerPublicKey, workspaceName string) *bool {
	if service == nil || workspaceName == "" {
		return nil
	}
	return new(service.PeerRunningWorkspace(ctx, peerPublicKey, workspaceName))
}

type roomPresenceSnapshotService interface {
	PeerRoomPresence(context.Context, string, string) (bool, time.Time, bool)
}

// MemberPresenceFields reads one shared snapshot when supported by presence.
// Legacy providers retain their separate presence and room interfaces.
func MemberPresenceFields(ctx context.Context, presence PresenceService, rooms RoomPresenceService, publicKey, workspace string) (*bool, *time.Time, *bool) {
	combined, ok := presence.(roomPresenceSnapshotService)
	if !ok || rooms == nil {
		online, seen := PresenceFields(ctx, presence, publicKey)
		return online, seen, InRoomField(ctx, rooms, publicKey, workspace)
	}
	online, seen, inRoom := combined.PeerRoomPresence(ctx, publicKey, workspace)
	var lastSeen *time.Time
	if !seen.IsZero() {
		lastSeen = new(seen.UTC())
	}
	if workspace == "" {
		return &online, lastSeen, nil
	}
	return &online, lastSeen, &inRoom
}
