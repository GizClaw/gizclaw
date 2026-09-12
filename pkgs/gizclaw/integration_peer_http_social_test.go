package gizclaw_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
)

// socialDevice is one registered device with its own Giznet connection and
// API key.
type socialDevice struct {
	client *gizcli.Client
	name   string
	key    string
}

// api returns the generated Peer HTTP client carried over this device's own
// connection. Which connection carries a request does not matter: the API key
// alone selects the device the request acts for.
func (d socialDevice) api(t *testing.T) *peerhttp.ClientWithResponses {
	t.Helper()
	api, err := d.client.PeerHTTPClient()
	if err != nil {
		t.Fatalf("PeerHTTPClient: %v", err)
	}
	return api
}

func (d socialDevice) publicKey() string {
	return d.client.KeyPair.Public.String()
}

func bearer(key string) peerhttp.RequestEditorFn {
	return func(_ context.Context, request *http.Request) error {
		request.Header.Set("Authorization", "Bearer "+key)
		return nil
	}
}

// startSocialTestServer starts a Server with an SFU endpoint, which Friend and
// Friend Group Workspaces need, and returns a registration token.
func startSocialTestServer(t *testing.T) (*testServer, string) {
	t.Helper()
	ts := startConfiguredTestServer(t, "", func(server *gizclaw.Server) {
		server.SFUURL = "wss://sfu.test"
	})
	ctx := context.Background()
	profiles := ts.server.Manager().RuntimeProfiles
	created, err := profiles.CreateRuntimeProfile(ctx, adminhttp.CreateRuntimeProfileRequestObject{
		Body: &adminhttp.RuntimeProfileUpsert{Id: "profile-social", Spec: apitypes.RuntimeProfileSpec{}},
	})
	if err != nil {
		t.Fatalf("CreateRuntimeProfile: %v", err)
	}
	if _, ok := created.(adminhttp.CreateRuntimeProfile200JSONResponse); !ok {
		t.Fatalf("CreateRuntimeProfile = %#v", created)
	}
	token, err := profiles.CreateRegistrationToken(ctx, adminhttp.CreateRegistrationTokenRequestObject{
		Body: &adminhttp.RegistrationTokenUpsert{Id: "token-social", Token: "registration-social", RuntimeProfileId: "profile-social"},
	})
	if err != nil {
		t.Fatalf("CreateRegistrationToken: %v", err)
	}
	registration, ok := token.(adminhttp.CreateRegistrationToken200JSONResponse)
	if !ok {
		t.Fatalf("CreateRegistrationToken = %#v", token)
	}
	return ts, registration.Token
}

// newSocialDevice connects a device, registers it the way a real device does
// and creates its API key over RPC.
func newSocialDevice(t *testing.T, ts *testServer, token, name, emoji string) socialDevice {
	t.Helper()
	ctx := context.Background()
	client := newTestClient(t, ts)
	if _, err := client.Register(ctx, "register-"+name, token); err != nil {
		t.Fatalf("Register(%s): %v", name, err)
	}
	ensurePeerInfo(t, client, apitypes.DeviceInfo{Name: &name, Emoji: &emoji})
	created, err := client.CreateAPIKey(ctx, "api-key-"+name, rpcapi.APIKeyCreateRequest{DisplayName: name})
	if err != nil {
		t.Fatalf("CreateAPIKey(%s): %v", name, err)
	}
	return socialDevice{client: client, name: name, key: created.APIKey}
}

func errorCodeOf(body []byte) string {
	text := string(body)
	start := strings.Index(text, `"code":"`)
	if start < 0 {
		return ""
	}
	text = text[start+len(`"code":"`):]
	return text[:strings.IndexByte(text, '"')]
}

func expectStatus(t *testing.T, what string, status int, body []byte, wantStatus int, wantCode string) {
	t.Helper()
	if status != wantStatus {
		t.Fatalf("%s status = %d, want %d; body=%s", what, status, wantStatus, body)
	}
	if wantCode != "" && errorCodeOf(body) != wantCode {
		t.Fatalf("%s code = %q, want %q; body=%s", what, errorCodeOf(body), wantCode, body)
	}
}

// TestIntegrationPeerHTTPFriendsThroughGoSDK drives the friend routes with
// the generated Go client over real Giznet connections.
func TestIntegrationPeerHTTPFriendsThroughGoSDK(t *testing.T) {
	ts, token := startSocialTestServer(t)
	ctx := context.Background()
	alice := newSocialDevice(t, ts, token, "alice-speaker", "🔊")
	bob := newSocialDevice(t, ts, token, "bob-watch", "⌚")
	aliceAPI, bobAPI := alice.api(t), bob.api(t)

	missing, err := aliceAPI.GetFriendInviteTokenWithResponse(ctx, bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "get missing invite token", missing.StatusCode(), missing.Body, http.StatusNotFound, "INVITE_TOKEN_NOT_FOUND")

	// The body is optional: the generated client sends none at all.
	defaultToken, err := aliceAPI.CreateFriendInviteTokenWithResponse(ctx, bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "create default invite token", defaultToken.StatusCode(), defaultToken.Body, http.StatusOK, "")
	if remaining := time.Until(defaultToken.JSON200.ExpiresAt); remaining > 6*time.Minute {
		t.Fatalf("default invite token lives %s, want the 5 minute RPC default", remaining)
	}
	ttl := int32(7 * 24 * 60 * 60)
	longToken, err := aliceAPI.CreateFriendInviteTokenWithJSONBodyWithResponse(ctx, peerhttp.InviteTokenCreateRequest{TtlSeconds: &ttl}, bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "extend invite token", longToken.StatusCode(), longToken.Body, http.StatusOK, "")
	if longToken.JSON200.InviteToken != defaultToken.JSON200.InviteToken || time.Until(longToken.JSON200.ExpiresAt) < 6*24*time.Hour {
		t.Fatalf("extended invite token = %#v, want %q for 7 days", longToken.JSON200, defaultToken.JSON200.InviteToken)
	}

	added, err := bobAPI.AddFriendWithResponse(ctx, peerhttp.FriendAddRequest{InviteToken: longToken.JSON200.InviteToken}, bearer(bob.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "add friend", added.StatusCode(), added.Body, http.StatusCreated, "")
	if added.JSON201.Name != alice.publicKey() || added.JSON201.Info == nil ||
		added.JSON201.Info.DisplayName == nil || *added.JSON201.Info.DisplayName != alice.name {
		t.Fatalf("added friend = %s", added.Body)
	}
	again, err := bobAPI.AddFriendWithResponse(ctx, peerhttp.FriendAddRequest{InviteToken: longToken.JSON200.InviteToken}, bearer(bob.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "add friend again", again.StatusCode(), again.Body, http.StatusConflict, "FRIEND_ALREADY_EXISTS")

	limit := int32(10)
	friends, err := aliceAPI.ListFriendsWithResponse(ctx, &peerhttp.ListFriendsParams{Limit: &limit}, bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "list friends", friends.StatusCode(), friends.Body, http.StatusOK, "")
	if len(friends.JSON200.Items) != 1 || friends.JSON200.Items[0].Name != bob.publicKey() ||
		friends.JSON200.Items[0].Info == nil || *friends.JSON200.Items[0].Info.Emoji != "⌚" {
		t.Fatalf("friends = %s", friends.Body)
	}

	// Bob's device goes offline; Alice still ends the friendship.
	if err := bob.client.Close(); err != nil {
		t.Fatalf("close bob: %v", err)
	}
	deleted, err := aliceAPI.DeleteFriendWithResponse(ctx, bob.publicKey(), bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "delete friend", deleted.StatusCode(), deleted.Body, http.StatusNoContent, "")
	gone, err := aliceAPI.GetFriendWithResponse(ctx, bob.publicKey(), bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "get deleted friend", gone.StatusCode(), gone.Body, http.StatusNotFound, "FRIEND_NOT_FOUND")
}

// TestIntegrationPeerHTTPFriendGroupsThroughGoSDK drives the friend group
// routes with the generated Go client over real Giznet connections, managing
// a device that is offline through another device's connection.
func TestIntegrationPeerHTTPFriendGroupsThroughGoSDK(t *testing.T) {
	ts, token := startSocialTestServer(t)
	ctx := context.Background()
	alice := newSocialDevice(t, ts, token, "alice-speaker", "🔊")
	bob := newSocialDevice(t, ts, token, "bob-watch", "⌚")
	carol := newSocialDevice(t, ts, token, "carol-tablet", "📱")
	aliceAPI, carolAPI := alice.api(t), carol.api(t)

	created, err := aliceAPI.CreateFriendGroupWithResponse(ctx, peerhttp.FriendGroupCreateRequest{Name: "family"}, bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "create group", created.StatusCode(), created.Body, http.StatusCreated, "")
	if created.JSON201.MyRole != peerhttp.FriendGroupRoleOwner {
		t.Fatalf("created group = %s", created.Body)
	}
	invite, err := aliceAPI.CreateFriendGroupInviteTokenWithResponse(ctx, "family", bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "create group invite token", invite.StatusCode(), invite.Body, http.StatusOK, "")
	for _, joiner := range []struct {
		device socialDevice
		name   string
	}{{bob, "home"}, {carol, "kids"}} {
		joined, err := joiner.device.api(t).JoinFriendGroupWithResponse(ctx, peerhttp.FriendGroupJoinRequest{
			InviteToken: invite.JSON200.InviteToken, Name: joiner.name,
		}, bearer(joiner.device.key))
		if err != nil {
			t.Fatal(err)
		}
		expectStatus(t, "join group as "+joiner.name, joined.StatusCode(), joined.Body, http.StatusOK, "")
		if joined.JSON200.Group.Name != joiner.name || joined.JSON200.Member.Role != peerhttp.FriendGroupRoleMember {
			t.Fatalf("join result = %s", joined.Body)
		}
	}

	// Bob's device goes offline. Its parent manages it with Bob's API key over
	// Carol's connection: no request needs the device.
	if err := bob.client.Close(); err != nil {
		t.Fatalf("close bob: %v", err)
	}
	if err := waitUntil(testReadyTimeout, func() error {
		response, err := carolAPI.FindDeviceWithResponse(ctx, bearer(bob.key))
		if err != nil {
			return err
		}
		if response.StatusCode() != http.StatusConflict || errorCodeOf(response.Body) != "DEVICE_OFFLINE" {
			return &unexpectedStatusError{status: response.StatusCode(), body: string(response.Body)}
		}
		return nil
	}); err != nil {
		t.Fatalf("bob never went offline: %v", err)
	}
	// Presence follows the Server's connection state, which drops Bob's
	// connection once the Server observes the close.
	var members *peerhttp.ListFriendGroupMembersResponse
	if err := waitUntil(testReadyTimeout, func() error {
		response, err := carolAPI.ListFriendGroupMembersWithResponse(ctx, "home", nil, bearer(bob.key))
		if err != nil {
			return err
		}
		members = response
		for _, member := range response.JSON200.Items {
			if member.PeerPublicKey == bob.publicKey() && (member.Online == nil || *member.Online) {
				return &unexpectedStatusError{status: response.StatusCode(), body: string(response.Body)}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("bob never listed offline: %v", err)
	}
	expectStatus(t, "list members while offline", members.StatusCode(), members.Body, http.StatusOK, "")
	names := map[string]string{}
	for _, member := range members.JSON200.Items {
		if member.Info == nil || member.Info.DisplayName == nil {
			t.Fatalf("member without info = %s", members.Body)
		}
		names[member.PeerPublicKey] = *member.Info.DisplayName
		// Every member has been seen; only Bob's device is disconnected.
		if wantOnline := member.PeerPublicKey != bob.publicKey(); member.Online == nil || *member.Online != wantOnline || member.LastSeenAt == nil {
			t.Fatalf("member presence = %s", members.Body)
		}
	}
	if len(names) != 3 || names[alice.publicKey()] != alice.name || names[bob.publicKey()] != bob.name || names[carol.publicKey()] != carol.name {
		t.Fatalf("member names = %v", names)
	}
	// The device RPC carries the same presence.
	rpcMembers, err := alice.client.ListFriendGroupMembers(ctx, "list-members", rpcapi.FriendGroupMemberListRequest{FriendGroupName: new("family")})
	if err != nil {
		t.Fatalf("ListFriendGroupMembers RPC: %v", err)
	}
	if len(rpcMembers.Items) != 3 {
		t.Fatalf("RPC members = %+v", rpcMembers.Items)
	}
	for _, member := range rpcMembers.Items {
		if wantOnline := member.Name != bob.publicKey(); member.Online == nil || *member.Online != wantOnline || member.LastSeenAt == nil {
			t.Fatalf("RPC member presence = %+v", member)
		}
	}
	left, err := carolAPI.LeaveFriendGroupWithResponse(ctx, "home", bearer(bob.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "leave while offline", left.StatusCode(), left.Body, http.StatusNoContent, "")
	afterLeave, err := carolAPI.GetFriendGroupWithResponse(ctx, "home", bearer(bob.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "get left group", afterLeave.StatusCode(), afterLeave.Body, http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND")

	ownerLeave, err := aliceAPI.LeaveFriendGroupWithResponse(ctx, "family", bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "owner leave", ownerLeave.StatusCode(), ownerLeave.Body, http.StatusConflict, "FRIEND_GROUP_OWNER_CANNOT_LEAVE")
	memberDissolve, err := carolAPI.DeleteFriendGroupWithResponse(ctx, "kids", bearer(carol.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "member dissolve", memberDissolve.StatusCode(), memberDissolve.Body, http.StatusForbidden, "FRIEND_GROUP_PERMISSION_DENIED")
	dissolved, err := aliceAPI.DeleteFriendGroupWithResponse(ctx, "family", bearer(alice.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "dissolve", dissolved.StatusCode(), dissolved.Body, http.StatusNoContent, "")
	afterDissolve, err := carolAPI.GetFriendGroupWithResponse(ctx, "kids", bearer(carol.key))
	if err != nil {
		t.Fatal(err)
	}
	expectStatus(t, "get dissolved group", afterDissolve.StatusCode(), afterDissolve.Body, http.StatusNotFound, "FRIEND_GROUP_NOT_FOUND")
}

type unexpectedStatusError struct {
	status int
	body   string
}

func (e *unexpectedStatusError) Error() string {
	return http.StatusText(e.status) + ": " + e.body
}
