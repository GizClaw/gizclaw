package gizclaw

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerresource"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peersync"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/gofiber/fiber/v2"
)

const syncPrefix = "/gizclaw/v1/"

func validatePeerSyncQuery(ctx *fiber.Ctx) error {
	if ctx.Method() == http.MethodGet && ctx.Path() == syncPrefix+"sync" {
		values := ctx.Context().QueryArgs().PeekMulti("timestamp")
		valid := len(values) == 1
		if valid {
			timestamp, err := strconv.ParseInt(string(values[0]), 10, 64)
			valid = err == nil && timestamp >= 0 && timestamp <= peersync.MaxTimestamp
		}
		if !valid {
			return ctx.Status(http.StatusBadRequest).JSON(apiError(publicHTTPInvalidRequestCode, "timestamp must be one nonnegative safe integer"))
		}
	}
	return ctx.Next()
}

func (s *peerHTTP) SyncPeer(ctx context.Context, request peerhttp.SyncPeerRequestObject) (peerhttp.SyncPeerResponseObject, error) {
	owner, err := publicHTTPOwner(ctx)
	if err != nil {
		return peerhttp.SyncPeer401JSONResponse{UnauthorizedJSONResponse: peerhttp.UnauthorizedJSONResponse(unauthorizedPublicHTTP())}, nil
	}
	if request.Params.Timestamp < 0 || request.Params.Timestamp > peersync.MaxTimestamp {
		return peerhttp.SyncPeer400JSONResponse{BadRequestJSONResponse: peerhttp.BadRequestJSONResponse(apiError(publicHTTPInvalidRequestCode, "timestamp must be a nonnegative safe integer"))}, nil
	}
	if s == nil || s.Sync == nil {
		return peerSyncInternalError(), nil
	}
	current, err := s.syncSnapshot(ctx, owner)
	if err != nil {
		if errors.Is(err, peerresource.ErrDeviceRuntimeProfileNotBound) || errors.Is(err, sql.ErrNoRows) {
			return peerhttp.SyncPeer403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(apiError("API_KEY_OWNER_UNAVAILABLE", http.StatusText(http.StatusForbidden)))}, nil
		}
		return peerSyncInternalError(), nil
	}
	// Recheck availability after the projection; a concurrent owner deletion
	// must not turn this request into a successful state checkpoint.
	if s.PeerAvailability != nil {
		if err := s.PeerAvailability(ctx, owner); err != nil {
			status, code := apiKeyOwnerHTTPError(err)
			body := apiError(code, http.StatusText(status))
			switch status {
			case http.StatusForbidden:
				return peerhttp.SyncPeer403JSONResponse{ForbiddenJSONResponse: peerhttp.ForbiddenJSONResponse(body)}, nil
			case http.StatusConflict:
				return peerhttp.SyncPeer409JSONResponse{ConflictJSONResponse: peerhttp.ConflictJSONResponse(body)}, nil
			default:
				return peerSyncInternalError(), nil
			}
		}
	}
	result, err := s.Sync.Sync(ctx, owner, request.Params.Timestamp, current)
	if err != nil {
		return peerSyncInternalError(), nil
	}
	events, err := peerSyncEvents(result)
	if err != nil {
		return peerSyncInternalError(), nil
	}
	return peerSyncResponse{ctx: ctx, events: events}, nil
}

func peerSyncInternalError() peerhttp.SyncPeerResponseObject {
	return peerhttp.SyncPeer500JSONResponse{InternalErrorJSONResponse: peerhttp.InternalErrorJSONResponse(internalPublicHTTP())}
}

// syncSnapshot only calls the existing owner-scoped read projections. It never
// contacts devices, reads Admin DTOs, or adds permissions to the API key.
func (s *peerHTTP) syncSnapshot(ctx context.Context, owner giznet.PublicKey) (peersync.Snapshot, error) {
	reads, ok := s.deviceReads(owner)
	if !ok || s.Contacts == nil || s.Friends == nil || s.FriendGroups == nil {
		return nil, peerresource.ErrDeviceServiceNotConfigured
	}
	snapshot := make(peersync.Snapshot)
	if err := addSyncRead(ctx, snapshot, "device", reads.DeviceInfo); err != nil {
		return nil, err
	}
	if err := addSyncRead(ctx, snapshot, "device/runtime", reads.DeviceRuntime); err != nil {
		return nil, err
	}
	if err := addSyncRead(ctx, snapshot, "device/status", reads.DeviceStatus); err != nil {
		return nil, err
	}
	if err := addSyncRead(ctx, snapshot, "device/runtime-profile", reads.DeviceRuntimeProfile); err != nil {
		return nil, err
	}
	if err := addSyncRead(ctx, snapshot, "device/mhs/v0/manifest", reads.MhsManifest); err != nil {
		return nil, err
	}
	firmware, err := reads.DeviceFirmware(ctx)
	if err == nil {
		if err := addSyncItem(snapshot, "device/firmware", peerhttp.DeviceFirmware{Description: firmware.Description, Slots: firmware.Slots}); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, peerresource.ErrDeviceFirmwareNotBound) {
		return nil, err
	}
	workspaces, err := reads.DeviceWorkspaces(ctx, peerresource.DeviceWorkspaceFilter{})
	if err != nil {
		return nil, err
	}
	for _, item := range workspaces {
		if err := addSyncItem(snapshot, "device/workspaces/"+url.PathEscape(item.Id), item); err != nil {
			return nil, err
		}
	}
	if err := s.syncContacts(ctx, snapshot, owner); err != nil {
		return nil, err
	}
	if err := s.syncFriends(ctx, snapshot, owner); err != nil {
		return nil, err
	}
	if err := s.syncFriendGroups(ctx, snapshot, owner); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func addSyncRead[T any](ctx context.Context, snapshot peersync.Snapshot, path string, read func(context.Context) (T, error)) error {
	value, err := read(ctx)
	if err != nil {
		return err
	}
	return addSyncItem(snapshot, path, value)
}

func addSyncItem(snapshot peersync.Snapshot, path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	snapshot[syncPrefix+path] = data
	return nil
}

func (s *peerHTTP) syncContacts(ctx context.Context, snapshot peersync.Snapshot, owner giznet.PublicKey) error {
	page, err := s.Contacts.ListContacts(ctx, owner.String(), rpcapi.ContactListRequest{Limit: new(maxPublicContactPageSize)})
	if err != nil {
		return err
	}
	// Contacts, Friends, Groups, and Group members have domain-enforced limits
	// below the page size. Refuse any incomplete projection instead of deleting
	// items that were merely omitted by pagination.
	if page.HasNext {
		return errors.New("sync: incomplete contact projection")
	}
	for _, item := range page.Items {
		if err := addSyncItem(snapshot, "contacts/"+url.PathEscape(item.Name), publicContact(item)); err != nil {
			return err
		}
	}
	return nil
}

func (s *peerHTTP) syncFriends(ctx context.Context, snapshot peersync.Snapshot, owner giznet.PublicKey) error {
	page, err := s.Friends.ListFriends(ctx, owner.String(), rpcapi.FriendListRequest{Limit: new(maxPublicContactPageSize)})
	if err != nil {
		return err
	}
	if page.HasNext {
		return errors.New("sync: incomplete friend projection")
	}
	for _, item := range page.Items {
		projected, err := s.publicFriend(ctx, item)
		if err != nil {
			return err
		}
		if err := addSyncItem(snapshot, "friends/"+url.PathEscape(item.Name), projected); err != nil {
			return err
		}
	}
	token, err := s.Friends.GetFriendInviteToken(ctx, owner.String(), rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		return err
	}
	if token.InviteToken != nil && token.ExpiresAt != nil {
		return addSyncItem(snapshot, "friends/invite-token", publicInviteToken(*token.InviteToken, *token.ExpiresAt))
	}
	return nil
}

func (s *peerHTTP) syncFriendGroups(ctx context.Context, snapshot peersync.Snapshot, owner giznet.PublicKey) error {
	page, err := s.FriendGroups.ListFriendGroups(ctx, owner.String(), rpcapi.FriendGroupListRequest{Limit: new(maxPublicContactPageSize)})
	if err != nil {
		return err
	}
	if page.HasNext {
		return errors.New("sync: incomplete group projection")
	}
	for _, item := range page.Items {
		path := "friend-groups/" + url.PathEscape(item.Name)
		if err := addSyncItem(snapshot, path, publicFriendGroup(item)); err != nil {
			return err
		}
		members, err := s.FriendGroups.ListFriendGroupMembers(ctx, owner.String(), rpcapi.FriendGroupMemberListRequest{FriendGroupName: &item.Name, Limit: new(maxPublicContactPageSize)})
		if err != nil {
			return err
		}
		if members.HasNext {
			return errors.New("sync: incomplete member projection")
		}
		for _, member := range members.Items {
			projected, err := s.publicFriendGroupMember(ctx, member)
			if err != nil {
				return err
			}
			if err := addSyncItem(snapshot, path+"/members/"+url.PathEscape(member.Name), projected); err != nil {
				return err
			}
		}
		if item.MyRole != nil && *item.MyRole == rpcapi.FriendGroupMemberRoleOwner {
			token, err := s.FriendGroups.GetFriendGroupInviteToken(ctx, owner.String(), rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: item.Name})
			if err != nil {
				return err
			}
			if token.InviteToken != nil && token.ExpiresAt != nil {
				if err := addSyncItem(snapshot, path+"/invite-token", publicInviteToken(*token.InviteToken, *token.ExpiresAt)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func peerSyncEvents(result peersync.Result) ([]peerhttp.SyncEvent, error) {
	events := make([]peerhttp.SyncEvent, 0, len(result.Upserts)+len(result.Deletes)+2)
	if result.Reset {
		var event peerhttp.SyncEvent
		if err := event.FromSyncReset(peerhttp.SyncReset{}); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	for _, key := range result.Deletes {
		var event peerhttp.SyncEvent
		if err := event.FromSyncDelete(peerhttp.SyncDelete{Key: key}); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	for _, key := range slices.Sorted(maps.Keys(result.Upserts)) {
		var data peerhttp.SyncUpsert_Data
		if err := data.UnmarshalJSON(result.Upserts[key]); err != nil {
			return nil, err
		}
		var event peerhttp.SyncEvent
		if err := event.FromSyncUpsert(peerhttp.SyncUpsert{Key: key, Data: data}); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	var event peerhttp.SyncEvent
	if err := event.FromSyncDone(peerhttp.SyncDone{Timestamp: result.Timestamp}); err != nil {
		return nil, err
	}
	return append(events, event), nil
}

type peerSyncResponse struct {
	ctx    context.Context
	events []peerhttp.SyncEvent
}

func (response peerSyncResponse) VisitSyncPeerResponse(ctx *fiber.Ctx) error {
	ctx.Set("Content-Type", "text/event-stream")
	ctx.Set("Cache-Control", "no-store")
	ctx.Set("X-Accel-Buffering", "no")
	ctx.Status(http.StatusOK)
	ctx.Response().SetBodyStreamWriter(func(w *bufio.Writer) {
		for _, event := range response.events {
			if response.ctx.Err() != nil {
				return
			}
			name, err := event.Discriminator()
			if err != nil {
				return
			}
			if err := writeSSEEvent(w, name, event); err != nil {
				return
			}
		}
	})
	return nil
}
