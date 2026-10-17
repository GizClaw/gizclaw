package socialutil

import (
	"context"
	"sync"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerpresence"
)

// PresencePageTimeout bounds presence decoration for a whole list page,
// including the Friend Group's optional room binding lookup.
const PresencePageTimeout = 2 * time.Second

const presencePageConcurrency = 8

// ReadPresencePage decorates indices concurrently with at most eight workers
// under one shared deadline. Each index is visited once, including after
// cancellation so presence fields can retain their conservative defaults.
// read must honor ctx and only modify its own item; all workers finish before
// this function returns. A shorter caller deadline takes precedence.
func ReadPresencePage(ctx context.Context, count int, read func(context.Context, int)) {
	if count == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, PresencePageTimeout)
	defer cancel()
	workers := min(count, presencePageConcurrency)
	var done sync.WaitGroup
	for worker := range workers {
		done.Go(func() {
			for i := worker; i < count; i += workers {
				read(ctx, i)
			}
		})
	}
	done.Wait()
}

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
	if ctx.Err() != nil {
		return new(false), nil
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
	if ctx.Err() != nil {
		return new(false)
	}
	return new(service.PeerRunningWorkspace(ctx, peerPublicKey, workspaceName))
}

type roomPresenceSnapshotService interface {
	PeerRoomPresence(context.Context, string, string) (bool, time.Time, bool)
}

type roomPresenceBatchService interface {
	PeerRoomPresenceBatch(context.Context, []string, string) []peerpresence.Snapshot
}

// AddFriendPresence reads a whole page's presence through the batch provider
// when available, with bounded per-item reads for legacy providers.
func AddFriendPresence(ctx context.Context, presence PresenceService, items []rpcapi.FriendObject) {
	ctx, cancel := context.WithTimeout(ctx, PresencePageTimeout)
	defer cancel()
	batch, ok := presence.(roomPresenceBatchService)
	if !ok {
		ReadPresencePage(ctx, len(items), func(ctx context.Context, i int) {
			items[i].Online, items[i].LastSeenAt = PresenceFields(ctx, presence, StringValue(items[i].PeerPublicKey))
		})
		return
	}
	keys := make([]string, len(items))
	for i := range items {
		keys[i] = StringValue(items[i].PeerPublicKey)
	}
	for i, snapshot := range batch.PeerRoomPresenceBatch(ctx, keys, "") {
		items[i].Online, items[i].LastSeenAt = snapshotFields(snapshot)
	}
}

// AddMemberPresence reads a whole page's connection and room fields together.
// It retains the optional-field behavior of legacy presence/room providers.
func AddMemberPresence(ctx context.Context, presence PresenceService, rooms RoomPresenceService, workspace string, items []rpcapi.FriendGroupMemberObject) {
	ctx, cancel := context.WithTimeout(ctx, PresencePageTimeout)
	defer cancel()
	batch, ok := presence.(roomPresenceBatchService)
	if !ok {
		ReadPresencePage(ctx, len(items), func(ctx context.Context, i int) {
			item := &items[i]
			item.Online, item.LastSeenAt, item.InRoom = MemberPresenceFields(ctx, presence, rooms, StringValue(item.PeerPublicKey), workspace)
		})
		return
	}
	keys := make([]string, len(items))
	for i := range items {
		keys[i] = StringValue(items[i].PeerPublicKey)
	}
	for i, snapshot := range batch.PeerRoomPresenceBatch(ctx, keys, workspace) {
		items[i].Online, items[i].LastSeenAt = snapshotFields(snapshot)
		if rooms != nil && workspace != "" {
			items[i].InRoom = new(snapshot.Online && snapshot.WorkspaceName == workspace)
		}
	}
}

func snapshotFields(snapshot peerpresence.Snapshot) (*bool, *time.Time) {
	if snapshot.LastSeenAt.IsZero() {
		return &snapshot.Online, nil
	}
	return &snapshot.Online, new(snapshot.LastSeenAt.UTC())
}

// MemberPresenceFields reads one shared snapshot when supported by presence.
// Legacy providers retain their separate presence and room interfaces.
func MemberPresenceFields(ctx context.Context, presence PresenceService, rooms RoomPresenceService, publicKey, workspace string) (*bool, *time.Time, *bool) {
	combined, ok := presence.(roomPresenceSnapshotService)
	if !ok || rooms == nil || ctx.Err() != nil {
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
