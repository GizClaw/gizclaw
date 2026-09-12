package gizclaw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/internal/socialutil"
	runtimepeer "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friend"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social/friendgroup"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
)

// socialHTTPPeer is one API-key-bound device of a socialHTTPFixture.
type socialHTTPPeer struct {
	key    giznet.PublicKey
	secret string
}

// socialHTTPFixture serves the friend and friend group routes for four
// devices sharing one Server. None of them has a Giznet connection, so every
// call proves the routes work while the device is offline.
type socialHTTPFixture struct {
	*deviceHTTPFixture
	a, b, c, d socialHTTPPeer

	mu  sync.Mutex
	now time.Time
}

func newSocialHTTPFixture(t *testing.T) *socialHTTPFixture {
	t.Helper()
	ctx := context.Background()
	f := &socialHTTPFixture{deviceHTTPFixture: newDeviceHTTPFixture(t)}
	// Invite token store deadlines use wall time, so the clock starts there.
	f.now = time.Now().UTC().Truncate(time.Second)
	f.a = socialHTTPPeer{key: f.owner, secret: f.secret}
	for _, target := range []struct {
		peer        *socialHTTPPeer
		name, emoji string
	}{{&f.b, "bedroom-speaker", "🛏️"}, {&f.c, "kids-watch", "⌚"}, {&f.d, "garage-speaker", "🚗"}} {
		keyPair, err := giznet.GenerateKeyPair()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.peers.SavePeer(ctx, apitypes.Peer{
			PublicKey: keyPair.Public.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive,
			Device: apitypes.DeviceInfo{Name: new(target.name), Emoji: new(target.emoji)},
		}); err != nil {
			t.Fatal(err)
		}
		if err := f.profiles.BindOwnerProfile(ctx, keyPair.Public.String(), "profile-device-http"); err != nil {
			t.Fatal(err)
		}
		created, err := f.apiKeys.Create(ctx, keyPair.Public.String(), "phone", false)
		if err != nil {
			t.Fatal(err)
		}
		*target.peer = socialHTTPPeer{key: keyPair.Public, secret: created.Secret}
	}
	availability := func(ctx context.Context, publicKey string) error {
		key, err := parsePeerPublicKey(publicKey)
		if err != nil {
			return err
		}
		return f.peers.EnsureAvailable(ctx, key)
	}
	f.public.Friends = &friend.Server{
		InviteTokens:     kv.NewMemory(nil),
		Friends:          kv.NewMemory(nil),
		Workspaces:       &adminTestWorkspaceService{},
		Profiles:         f.peers,
		PeerAvailability: availability,
		SFUURL:           "wss://sfu.test",
		Now:              f.clock,
	}
	groupStore := kv.NewMemory(nil)
	f.public.FriendGroups = &friendgroup.Server{
		Groups:            groupStore,
		InviteTokens:      groupStore,
		Members:           groupStore,
		Belongs:           groupStore,
		RelationshipStore: groupStore,
		Workspaces:        &adminTestWorkspaceService{},
		Profiles:          f.peers,
		PeerAvailability:  availability,
		SFUURL:            "wss://sfu.test",
		Now:               f.clock,
	}
	f.public.Profiles = f.peers
	return f
}

func (f *socialHTTPFixture) clock() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *socialHTTPFixture) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func (f *socialHTTPFixture) as(t *testing.T, peer socialHTTPPeer, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return f.doWithSecret(t, peer.secret, method, "/gizclaw/v1"+path, body)
}

// expect asserts the status and, for an error, the stable error code.
func expect(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, status, response.Body.String())
	}
	if code != "" {
		if got := errorCode(t, response); got != code {
			t.Fatalf("error code = %q, want %q", got, code)
		}
	}
}

func TestPeerHTTPFriendsLifecycle(t *testing.T) {
	f := newSocialHTTPFixture(t)

	expect(t, f.as(t, f.a, http.MethodGet, "/friends/invite-token", ""), http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND")
	created := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodPost, "/friends/invite-token", ""))
	if created.InviteToken == "" || !created.ExpiresAt.Equal(f.clock().Add(socialutil.DefaultInviteTokenTTL)) {
		t.Fatalf("default invite token = %#v", created)
	}
	for _, body := range []string{`{"ttl_seconds":59}`, `{"ttl_seconds":604801}`} {
		expect(t, f.as(t, f.a, http.MethodPost, "/friends/invite-token", body), http.StatusBadRequest, "INVALID_REQUEST")
	}
	extended := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodPost, "/friends/invite-token", `{"ttl_seconds":86400}`))
	if extended.InviteToken != created.InviteToken || !extended.ExpiresAt.Equal(f.clock().Add(24*time.Hour)) {
		t.Fatalf("extended invite token = %#v, want %q for 24h", extended, created.InviteToken)
	}
	if got := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodGet, "/friends/invite-token", "")); got != extended {
		t.Fatalf("GET invite token = %#v, want %#v", got, extended)
	}

	// The extended token outlives the RPC default lifetime.
	f.advance(time.Hour)
	expect(t, f.as(t, f.a, http.MethodPost, "/friends", `{"invite_token":"`+created.InviteToken+`"}`), http.StatusBadRequest, "FRIEND_SELF_INVITE")
	expect(t, f.as(t, f.c, http.MethodPost, "/friends", `{"invite_token":"missing"}`), http.StatusNotFound, "INVITE_TOKEN_INVALID")
	expect(t, f.as(t, f.c, http.MethodPost, "/friends", `{"invite_token":" "}`), http.StatusBadRequest, "INVALID_REQUEST")
	response := f.as(t, f.b, http.MethodPost, "/friends", `{"invite_token":"`+created.InviteToken+`"}`)
	expect(t, response, http.StatusCreated, "")
	added := decodeJSON[peerhttp.Friend](t, response)
	if added.Name != f.a.key.String() || added.PeerPublicKey != f.a.key.String() || added.WorkspaceName == "" ||
		added.Info == nil || socialutil.StringValue(added.Info.DisplayName) != "kitchen-speaker" || socialutil.StringValue(added.Info.Emoji) != "🔊" {
		t.Fatalf("added friend = %#v", added)
	}
	expect(t, f.as(t, f.b, http.MethodPost, "/friends", `{"invite_token":"`+created.InviteToken+`"}`), http.StatusConflict, "FRIEND_ALREADY_EXISTS")

	list := decodeJSON[peerhttp.FriendList](t, f.as(t, f.a, http.MethodGet, "/friends?limit=10", ""))
	if len(list.Items) != 1 || list.HasNext || list.Items[0].Name != f.b.key.String() ||
		list.Items[0].Info == nil || socialutil.StringValue(list.Items[0].Info.DisplayName) != "bedroom-speaker" {
		t.Fatalf("friend list = %#v", list)
	}
	expect(t, f.as(t, f.a, http.MethodGet, "/friends?limit=0", ""), http.StatusBadRequest, "INVALID_REQUEST")
	expect(t, f.as(t, f.a, http.MethodGet, "/friends?limit=201", ""), http.StatusBadRequest, "INVALID_REQUEST")
	expect(t, f.as(t, f.a, http.MethodGet, "/friends?cursor=", ""), http.StatusBadRequest, "INVALID_REQUEST")

	got := decodeJSON[peerhttp.Friend](t, f.as(t, f.a, http.MethodGet, "/friends/"+url.PathEscape(f.b.key.String()), ""))
	if got.Name != f.b.key.String() || got.Info == nil || socialutil.StringValue(got.Info.Emoji) != "🛏️" {
		t.Fatalf("GET friend = %#v", got)
	}
	expect(t, f.as(t, f.a, http.MethodGet, "/friends/"+f.c.key.String(), ""), http.StatusNotFound, "FRIEND_NOT_FOUND")
	expect(t, f.as(t, f.a, http.MethodGet, "/friends/%20x", ""), http.StatusBadRequest, "INVALID_REQUEST")

	expect(t, f.as(t, f.a, http.MethodDelete, "/friends/invite-token", ""), http.StatusNoContent, "")
	expect(t, f.as(t, f.a, http.MethodDelete, "/friends/invite-token", ""), http.StatusNoContent, "")
	expect(t, f.as(t, f.a, http.MethodGet, "/friends/invite-token", ""), http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND")

	expect(t, f.as(t, f.a, http.MethodDelete, "/friends/"+f.b.key.String(), ""), http.StatusNoContent, "")
	// Like server.friend.delete, repeating a completed delete succeeds.
	expect(t, f.as(t, f.a, http.MethodDelete, "/friends/"+f.b.key.String(), ""), http.StatusNoContent, "")
	expect(t, f.as(t, f.a, http.MethodDelete, "/friends/"+f.c.key.String(), ""), http.StatusNotFound, "FRIEND_NOT_FOUND")
	expect(t, f.as(t, f.a, http.MethodGet, "/friends/"+f.b.key.String(), ""), http.StatusNotFound, "FRIEND_NOT_FOUND")
	if list := decodeJSON[peerhttp.FriendList](t, f.as(t, f.b, http.MethodGet, "/friends", "")); len(list.Items) != 0 {
		t.Fatalf("friend list after delete = %#v", list)
	}

	// None of the devices is connected; a device control route confirms it.
	expect(t, f.as(t, f.a, http.MethodPost, "/device/actions/find", ""), http.StatusConflict, "DEVICE_OFFLINE")
}

func TestPeerHTTPFriendInviteTokenExpiry(t *testing.T) {
	f := newSocialHTTPFixture(t)
	created := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodPost, "/friends/invite-token", "{}"))
	f.advance(socialutil.DefaultInviteTokenTTL + time.Second)
	expect(t, f.as(t, f.b, http.MethodPost, "/friends", `{"invite_token":"`+created.InviteToken+`"}`), http.StatusNotFound, "INVITE_TOKEN_INVALID")
	expect(t, f.as(t, f.a, http.MethodGet, "/friends/invite-token", ""), http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND")
}

func TestPeerHTTPFriendRejectsRetiringCounterpart(t *testing.T) {
	f := newSocialHTTPFixture(t)
	created := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodPost, "/friends/invite-token", ""))
	if err := f.peers.DeleteSelf(context.Background(), f.a.key); err != nil {
		t.Fatalf("DeleteSelf: %v", err)
	}
	expect(t, f.as(t, f.b, http.MethodPost, "/friends", `{"invite_token":"`+created.InviteToken+`"}`), http.StatusConflict, "PEER_PENDING_DELETION")
	// The retiring owner's own key is rejected before any handler runs.
	expect(t, f.as(t, f.a, http.MethodGet, "/friends", ""), http.StatusConflict, "PEER_PENDING_DELETION")
}

func TestPublicSocialErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{friend.ErrInviteTokenRequired, http.StatusBadRequest, "INVALID_REQUEST"},
		{friendgroup.ErrInvalidMemberRole, http.StatusBadRequest, "INVALID_REQUEST"},
		{socialutil.ErrInvalidInviteTokenTTL, http.StatusBadRequest, "INVALID_REQUEST"},
		{friend.ErrInviteTokenSelfOwned, http.StatusBadRequest, "FRIEND_SELF_INVITE"},
		{friend.ErrInviteTokenUnavailable, http.StatusNotFound, "INVITE_TOKEN_INVALID"},
		{friendgroup.ErrInviteTokenUnavailable, http.StatusNotFound, "INVITE_TOKEN_INVALID"},
		{fmt.Errorf("wrapped: %w", friend.ErrInviteTokenLookupFailed), http.StatusInternalServerError, "INTERNAL_ERROR"},
		{friendgroup.ErrFriendGroupPermissionDenied, http.StatusForbidden, "FRIEND_GROUP_PERMISSION_DENIED"},
		{friendgroup.ErrFriendGroupMemberNotFound, http.StatusNotFound, "FRIEND_GROUP_MEMBER_NOT_FOUND"},
		{kv.ErrNotFound, http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND"},
		{friend.ErrPeerFriendLimit, http.StatusConflict, "FRIEND_LIMIT_REACHED"},
		{friendgroup.ErrFriendGroupNameExists, http.StatusConflict, "FRIEND_GROUP_NAME_CONFLICT"},
		{friendgroup.ErrFriendGroupMembershipNameImmutable, http.StatusConflict, "FRIEND_GROUP_ALREADY_JOINED"},
		{fmt.Errorf("%w: 10 members", friendgroup.ErrFriendGroupFull), http.StatusConflict, "FRIEND_GROUP_FULL"},
		{friendgroup.ErrPeerFriendGroupLimit, http.StatusConflict, "FRIEND_GROUP_LIMIT_REACHED"},
		{friendgroup.ErrFriendGroupOwnerCannotLeave, http.StatusConflict, "FRIEND_GROUP_OWNER_CANNOT_LEAVE"},
		{friendgroup.ErrFriendGroupOwnerCannotBeRemoved, http.StatusConflict, "FRIEND_GROUP_OWNER_CANNOT_BE_REMOVED"},
		{friendgroup.ErrFriendGroupOwnerRoleImmutable, http.StatusConflict, "FRIEND_GROUP_OWNER_ROLE_IMMUTABLE"},
		{friendgroup.ErrGroupChanged, http.StatusConflict, "FRIEND_GROUP_CHANGED"},
		{friendgroup.ErrFriendGroupPendingDeletion, http.StatusConflict, "FRIEND_GROUP_PENDING_DELETION"},
		{runtimepeer.ErrPeerPendingDeletion, http.StatusConflict, "PEER_PENDING_DELETION"},
		{runtimepeer.ErrPeerDeleted, http.StatusConflict, "PEER_DELETED"},
		{runtimepeer.ErrPeerNotFound, http.StatusConflict, "PEER_DELETED"},
		{errors.New("store exploded"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		status, body := publicSocialError(tc.err, "FRIEND_GROUP_NOT_FOUND")
		if status != tc.status || body.Error.Code != tc.code {
			t.Fatalf("publicSocialError(%v) = %d %q, want %d %q", tc.err, status, body.Error.Code, tc.status, tc.code)
		}
		if tc.status == http.StatusInternalServerError && body.Error.Message != http.StatusText(http.StatusInternalServerError) {
			t.Fatalf("publicSocialError(%v) leaked %q", tc.err, body.Error.Message)
		}
	}
}

type socialHTTPPresenceStub struct{ seen time.Time }

func (s socialHTTPPresenceStub) PeerPresence(context.Context, string) (bool, time.Time) {
	return true, s.seen
}

func TestPeerHTTPFriendGroupRoles(t *testing.T) {
	f := newSocialHTTPFixture(t)
	f.public.FriendGroups.Presence = socialHTTPPresenceStub{f.clock()}
	a, b, c, d := f.a, f.b, f.c, f.d

	expect(t, f.as(t, a, http.MethodPost, "/friend-groups", `{"name":" room"}`), http.StatusBadRequest, "INVALID_REQUEST")
	response := f.as(t, a, http.MethodPost, "/friend-groups", `{"name":"room","display_name":"Family"}`)
	expect(t, response, http.StatusCreated, "")
	group := decodeJSON[peerhttp.FriendGroup](t, response)
	if group.Name != "room" || group.MyRole != peerhttp.FriendGroupRoleOwner || socialutil.StringValue(group.DisplayName) != "Family" {
		t.Fatalf("created group = %#v", group)
	}
	expect(t, f.as(t, a, http.MethodPost, "/friend-groups", `{"name":"room"}`), http.StatusConflict, "FRIEND_GROUP_NAME_CONFLICT")

	expect(t, f.as(t, a, http.MethodGet, "/friend-groups/room/invite-token", ""), http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND")
	expect(t, f.as(t, a, http.MethodPost, "/friend-groups/room/invite-token", `{"ttl_seconds":1}`), http.StatusBadRequest, "INVALID_REQUEST")
	token := decodeJSON[peerhttp.InviteToken](t, f.as(t, a, http.MethodPost, "/friend-groups/room/invite-token", `{"ttl_seconds":3600}`))
	if !token.ExpiresAt.Equal(f.clock().Add(time.Hour)) {
		t.Fatalf("group invite token = %#v, want 1h", token)
	}
	join := func(peer socialHTTPPeer, name string) *httptest.ResponseRecorder {
		return f.as(t, peer, http.MethodPost, "/friend-groups/@join", `{"invite_token":"`+token.InviteToken+`","name":"`+name+`"}`)
	}
	response = join(b, "family")
	expect(t, response, http.StatusOK, "")
	joined := decodeJSON[peerhttp.FriendGroupJoinResult](t, response)
	if joined.Member.Online != nil || joined.Member.LastSeenAt != nil || joined.Group.Name != "family" || joined.Group.MyRole != peerhttp.FriendGroupRoleMember ||
		joined.Member.PeerPublicKey != b.key.String() || joined.Member.Role != peerhttp.FriendGroupRoleMember ||
		joined.Member.Info == nil || socialutil.StringValue(joined.Member.Info.DisplayName) != "bedroom-speaker" {
		t.Fatalf("join result = %#v", joined)
	}
	expect(t, join(b, "family"), http.StatusOK, "")
	expect(t, join(b, "renamed"), http.StatusConflict, "FRIEND_GROUP_ALREADY_JOINED")
	expect(t, join(c, "kids"), http.StatusOK, "")
	expect(t, f.as(t, c, http.MethodPost, "/friend-groups", `{"name":"mine"}`), http.StatusCreated, "")
	expect(t, f.as(t, d, http.MethodPost, "/friend-groups", `{"name":"taken"}`), http.StatusCreated, "")
	expect(t, join(d, "taken"), http.StatusConflict, "FRIEND_GROUP_NAME_CONFLICT")
	expect(t, f.as(t, d, http.MethodPost, "/friend-groups/@join", `{"invite_token":"missing","name":"x"}`), http.StatusNotFound, "INVITE_TOKEN_INVALID")

	// Promote C to admin; only the owner may change roles.
	memberPath := func(group string, peer socialHTTPPeer) string {
		return "/friend-groups/" + group + "/members/" + url.PathEscape(peer.key.String())
	}
	expect(t, f.as(t, b, http.MethodPut, memberPath("family", c), `{"role":"admin"}`), http.StatusForbidden, "FRIEND_GROUP_PERMISSION_DENIED")
	expect(t, f.as(t, a, http.MethodPut, memberPath("room", c), `{"role":"owner"}`), http.StatusBadRequest, "")
	promoted := decodeJSON[peerhttp.FriendGroupMember](t, f.as(t, a, http.MethodPut, memberPath("room", c), `{"role":"admin"}`))
	if promoted.Online != nil || promoted.LastSeenAt != nil || promoted.Role != peerhttp.FriendGroupRoleAdmin || promoted.Info == nil || socialutil.StringValue(promoted.Info.DisplayName) != "kids-watch" {
		t.Fatalf("promoted member = %#v", promoted)
	}
	expect(t, f.as(t, a, http.MethodPut, memberPath("room", a), `{"role":"member"}`), http.StatusConflict, "FRIEND_GROUP_OWNER_ROLE_IMMUTABLE")
	expect(t, f.as(t, a, http.MethodPut, memberPath("room", d), `{"role":"member"}`), http.StatusNotFound, "FRIEND_GROUP_MEMBER_NOT_FOUND")

	// Members list for every member, with profile info.
	members := decodeJSON[peerhttp.FriendGroupMemberList](t, f.as(t, b, http.MethodGet, "/friend-groups/family/members", ""))
	if len(members.Items) != 3 || members.HasNext {
		t.Fatalf("members = %#v", members)
	}
	roles := map[string]peerhttp.FriendGroupRole{}
	for _, item := range members.Items {
		if item.Online == nil || !*item.Online || item.LastSeenAt == nil || !item.LastSeenAt.Equal(f.clock()) || item.Info == nil || socialutil.StringValue(item.Info.DisplayName) == "" || item.Name != item.PeerPublicKey {
			t.Fatalf("member without info = %#v", item)
		}
		roles[item.PeerPublicKey] = item.Role
	}
	if roles[a.key.String()] != peerhttp.FriendGroupRoleOwner || roles[b.key.String()] != peerhttp.FriendGroupRoleMember || roles[c.key.String()] != peerhttp.FriendGroupRoleAdmin {
		t.Fatalf("member roles = %#v", roles)
	}
	page := decodeJSON[peerhttp.FriendGroupMemberList](t, f.as(t, b, http.MethodGet, "/friend-groups/family/members?limit=2", ""))
	if len(page.Items) != 2 || !page.HasNext || page.NextCursor == nil {
		t.Fatalf("first member page = %#v", page)
	}
	rest := decodeJSON[peerhttp.FriendGroupMemberList](t, f.as(t, b, http.MethodGet, "/friend-groups/family/members?limit=2&cursor="+url.QueryEscape(*page.NextCursor), ""))
	if len(rest.Items) != 1 || rest.HasNext {
		t.Fatalf("second member page = %#v", rest)
	}

	// A non-member cannot resolve the Group name at all.
	for _, route := range []struct{ method, path, body string }{
		{http.MethodGet, "/friend-groups/room", ""},
		{http.MethodGet, "/friend-groups/room/members", ""},
		{http.MethodPut, "/friend-groups/room", `{"display_name":"x"}`},
		{http.MethodDelete, "/friend-groups/room", ""},
		{http.MethodPost, "/friend-groups/room/@leave", ""},
		{http.MethodGet, "/friend-groups/room/invite-token", ""},
		{http.MethodPost, "/friend-groups/room/invite-token", ""},
		{http.MethodDelete, "/friend-groups/room/invite-token", ""},
	} {
		response := f.as(t, d, route.method, route.path, route.body)
		if response.Code != http.StatusNotFound || errorCode(t, response) != "FRIEND_GROUP_NOT_FOUND" {
			t.Fatalf("non-member %s %s = %d %s", route.method, route.path, response.Code, response.Body.String())
		}
	}

	// A member reads but cannot change the Group or its invite token.
	if got := decodeJSON[peerhttp.FriendGroup](t, f.as(t, b, http.MethodGet, "/friend-groups/family", "")); got.Name != "family" || got.MyRole != peerhttp.FriendGroupRoleMember {
		t.Fatalf("member GET group = %#v", got)
	}
	for _, peer := range []socialHTTPPeer{b, c} {
		name := map[socialHTTPPeer]string{b: "family", c: "kids"}[peer]
		for _, route := range []struct{ method, path, body string }{
			{http.MethodPut, "/friend-groups/" + name, `{"display_name":"x"}`},
			{http.MethodDelete, "/friend-groups/" + name, ""},
			{http.MethodGet, "/friend-groups/" + name + "/invite-token", ""},
			{http.MethodPost, "/friend-groups/" + name + "/invite-token", ""},
			{http.MethodDelete, "/friend-groups/" + name + "/invite-token", ""},
		} {
			response := f.as(t, peer, route.method, route.path, route.body)
			if response.Code != http.StatusForbidden || errorCode(t, response) != "FRIEND_GROUP_PERMISSION_DENIED" {
				t.Fatalf("non-owner %s %s = %d %s", route.method, route.path, response.Code, response.Body.String())
			}
		}
	}
	updated := decodeJSON[peerhttp.FriendGroup](t, f.as(t, a, http.MethodPut, "/friend-groups/room", `{"display_name":"Home","description":"weekend"}`))
	if socialutil.StringValue(updated.DisplayName) != "Home" || socialutil.StringValue(updated.Description) != "weekend" {
		t.Fatalf("updated group = %#v", updated)
	}

	// Member removal follows server.friend_group.members.delete.
	expect(t, f.as(t, b, http.MethodDelete, memberPath("family", c), ""), http.StatusForbidden, "FRIEND_GROUP_PERMISSION_DENIED")
	expect(t, f.as(t, a, http.MethodDelete, memberPath("room", a), ""), http.StatusConflict, "FRIEND_GROUP_OWNER_CANNOT_BE_REMOVED")
	expect(t, f.as(t, a, http.MethodDelete, memberPath("room", d), ""), http.StatusNotFound, "FRIEND_GROUP_MEMBER_NOT_FOUND")
	expect(t, f.as(t, c, http.MethodDelete, memberPath("kids", b), ""), http.StatusNoContent, "")
	expect(t, f.as(t, b, http.MethodGet, "/friend-groups/family", ""), http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND")

	// Admins may add members, only the owner may add admins.
	add := func(peer socialHTTPPeer, group, role string) *httptest.ResponseRecorder {
		return f.as(t, peer, http.MethodPost, "/friend-groups/"+group+"/members", `{"peer_public_key":"`+b.key.String()+`","member_name":"family","role":"`+role+`"}`)
	}
	expect(t, add(c, "kids", "admin"), http.StatusForbidden, "FRIEND_GROUP_PERMISSION_DENIED")
	response = add(c, "kids", "member")
	expect(t, response, http.StatusCreated, "")
	if added := decodeJSON[peerhttp.FriendGroupMember](t, response); added.Online != nil || added.LastSeenAt != nil || added.PeerPublicKey != b.key.String() || added.Role != peerhttp.FriendGroupRoleMember {
		t.Fatalf("added member = %#v", added)
	}

	// Leaving: the owner must dissolve instead; admins and members may leave.
	expect(t, f.as(t, a, http.MethodPost, "/friend-groups/room/@leave", ""), http.StatusConflict, "FRIEND_GROUP_OWNER_CANNOT_LEAVE")
	expect(t, f.as(t, c, http.MethodPost, "/friend-groups/kids/@leave", ""), http.StatusNoContent, "")
	expect(t, f.as(t, b, http.MethodPost, "/friend-groups/family/@leave", ""), http.StatusNoContent, "")
	expect(t, f.as(t, b, http.MethodPost, "/friend-groups/family/@leave", ""), http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND")
	if list := decodeJSON[peerhttp.FriendGroupList](t, f.as(t, c, http.MethodGet, "/friend-groups", "")); len(list.Items) != 1 || list.Items[0].Name != "mine" {
		t.Fatalf("C groups after leave = %#v", list)
	}

	groups := decodeJSON[peerhttp.FriendGroupList](t, f.as(t, a, http.MethodGet, "/friend-groups?limit=5", ""))
	if len(groups.Items) != 1 || groups.Items[0].Name != "room" || groups.Items[0].MyRole != peerhttp.FriendGroupRoleOwner {
		t.Fatalf("A groups = %#v", groups)
	}
	expect(t, f.as(t, a, http.MethodDelete, "/friend-groups/room/invite-token", ""), http.StatusNoContent, "")
	expect(t, f.as(t, a, http.MethodGet, "/friend-groups/room/invite-token", ""), http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND")
	expect(t, f.as(t, a, http.MethodDelete, "/friend-groups/room", ""), http.StatusNoContent, "")
	expect(t, f.as(t, a, http.MethodDelete, "/friend-groups/room", ""), http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND")
	expect(t, f.as(t, a, http.MethodGet, "/friend-groups/room", ""), http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND")
}

func TestPeerHTTPFriendGroupInviteTokenExpiry(t *testing.T) {
	f := newSocialHTTPFixture(t)
	expect(t, f.as(t, f.a, http.MethodPost, "/friend-groups", `{"name":"room"}`), http.StatusCreated, "")
	token := decodeJSON[peerhttp.InviteToken](t, f.as(t, f.a, http.MethodPost, "/friend-groups/room/invite-token", ""))
	if !token.ExpiresAt.Equal(f.clock().Add(socialutil.DefaultInviteTokenTTL)) {
		t.Fatalf("default group invite token = %#v", token)
	}
	f.advance(socialutil.DefaultInviteTokenTTL + time.Second)
	expect(t, f.as(t, f.b, http.MethodPost, "/friend-groups/@join", `{"invite_token":"`+token.InviteToken+`","name":"room"}`), http.StatusNotFound, "INVITE_TOKEN_INVALID")
}

func TestPeerHTTPSocialDebugAccess(t *testing.T) {
	f := newSocialHTTPFixture(t)
	ctx := context.Background()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, "/gizclaw/v1"+path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer gizclaw_pk_"+f.a.key.String())
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		return response
	}
	if err := f.manager.PeerRun.SetDebugMode(ctx, f.a.key, "readonly"); err != nil {
		t.Fatal(err)
	}
	expect(t, do(http.MethodGet, "/friends", ""), http.StatusOK, "")
	expect(t, do(http.MethodGet, "/friend-groups", ""), http.StatusOK, "")
	expect(t, do(http.MethodPost, "/friends/invite-token", ""), http.StatusForbidden, "DEBUG_ACCESS_FORBIDDEN")
	expect(t, do(http.MethodPost, "/friend-groups", `{"name":"room"}`), http.StatusForbidden, "DEBUG_ACCESS_FORBIDDEN")
	if err := f.manager.PeerRun.SetDebugMode(ctx, f.a.key, "fullcontrol"); err != nil {
		t.Fatal(err)
	}
	expect(t, do(http.MethodPost, "/friends/invite-token", ""), http.StatusOK, "")
	expect(t, do(http.MethodPost, "/friend-groups", `{"name":"room"}`), http.StatusCreated, "")
}
