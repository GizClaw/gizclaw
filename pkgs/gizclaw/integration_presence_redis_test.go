//go:build store_e2e

package gizclaw_test

import (
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workflow/agents/sfu"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
	redis "github.com/redis/go-redis/v9"
)

// TestIntegrationSharedRedisPresence uses two real Servers with independent
// SQL databases, real WebRTC devices and a Docker Redis primary/replica. It
// verifies Social RPC queries, not just direct Store reads.
func TestIntegrationSharedRedisPresence(t *testing.T) {
	clients := make([]*redis.Client, 0, 2)
	for _, name := range []string{"GIZCLAW_TEST_REDIS_READ_DSN", "GIZCLAW_TEST_REDIS_DSN"} {
		url := os.Getenv(name)
		if url == "" {
			t.Fatalf("%s is required; run tests/store-e2e/run_redis_readwrite_tests.sh", name)
		}
		options, err := redis.ParseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		options.ContextTimeoutEnabled = true
		client := redis.NewClient(options)
		t.Cleanup(func() { _ = client.Close() })
		clients = append(clients, client)
	}
	store, err := kv.NewRedisReadWriteWithClients(clients[0], clients[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	root := kv.Prefixed(store, kv.Key{"presence-e2e-" + rand.Text()})
	sfuConfig := sfu.Config{URL: os.Getenv("GIZCLAW_TEST_LIVEKIT_URL"),
		APIKey: os.Getenv("GIZCLAW_TEST_LIVEKIT_API_KEY"), APISecret: os.Getenv("GIZCLAW_TEST_LIVEKIT_API_SECRET")}
	if sfuConfig.URL == "" || sfuConfig.APIKey == "" || sfuConfig.APISecret == "" {
		t.Fatal("LiveKit environment is required; run tests/store-e2e/run_redis_readwrite_tests.sh")
	}
	start := func(endpoint string) (*testServer, string) {
		ts := startConfiguredTestServer(t, "", func(s *gizclaw.Server) {
			s.PeerStore = kv.Prefixed(root, kv.Key{"peers"})
			s.FriendStore = kv.Prefixed(root, kv.Key{"friends"})
			s.FriendGroupStore = kv.Prefixed(root, kv.Key{"groups"})
			s.PublicEndpoint = endpoint
			s.SFUURL, s.SFU = sfuConfig.URL, sfuConfig
		})
		profiles := ts.server.Manager().RuntimeProfiles
		if _, err := profiles.CreateRuntimeProfile(t.Context(), adminhttp.CreateRuntimeProfileRequestObject{
			Body: &adminhttp.RuntimeProfileUpsert{Id: "profile-presence", Spec: apitypes.RuntimeProfileSpec{}},
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := profiles.CreateRegistrationToken(t.Context(), adminhttp.CreateRegistrationTokenRequestObject{
			Body: &adminhttp.RegistrationTokenUpsert{Id: "token-presence", Token: "registration-presence", RuntimeProfileId: "profile-presence"},
		}); err != nil {
			t.Fatal(err)
		}
		return ts, "registration-presence"
	}
	a, tokenA := start("server-a.test:9820")
	b, tokenB := start("server-b.test:9820")
	owner := newSocialDevice(t, a, tokenA, "owner", "")
	member := newSocialDevice(t, b, tokenB, "member", "")
	invite, err := member.client.CreateFriendInviteToken(t.Context(), "presence-friend-invite", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.client.AddFriend(t.Context(), "presence-friend-add", rpcapi.FriendAddRequest{InviteToken: invite.InviteToken}); err != nil {
		t.Fatal(err)
	}
	group, err := owner.client.CreateFriendGroup(t.Context(), "presence-create-group", rpcapi.FriendGroupCreateRequest{Name: "team"})
	if err != nil {
		t.Fatal(err)
	}
	groupInvite, err := owner.client.CreateFriendGroupInviteToken(t.Context(), "presence-group-invite", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := member.client.JoinFriendGroup(t.Context(), "presence-join-group", rpcapi.FriendGroupJoinRequest{Name: "team", InviteToken: groupInvite.InviteToken}); err != nil {
		t.Fatal(err)
	}
	if group.WorkspaceName == nil {
		t.Fatal("group has no SFU Workspace")
	}
	wantRoom := false
	// Keep the querying Server's other SQL resources available, but remove the
	// table that previously supplied cross-user last_seen_at.
	if _, err := a.server.PeerRunDB.ExecContext(t.Context(), `DROP TABLE peer_runs`); err != nil {
		t.Fatal(err)
	}
	assertRemote := func(wantOnline bool) {
		t.Helper()
		if err := waitUntil(15*time.Second, func() error {
			friends, err := owner.client.ListFriends(t.Context(), "presence-friends", rpcapi.FriendListRequest{})
			if err != nil {
				return err
			}
			if len(friends.Items) != 1 || friends.Items[0].Online == nil || *friends.Items[0].Online != wantOnline || friends.Items[0].LastSeenAt == nil {
				return fmt.Errorf("remote Friend presence = %+v", friends.Items)
			}
			members, err := owner.client.ListFriendGroupMembers(t.Context(), "presence-members", rpcapi.FriendGroupMemberListRequest{FriendGroupName: new(group.Name)})
			if err != nil {
				return err
			}
			for _, item := range members.Items {
				if item.PeerPublicKey != nil && *item.PeerPublicKey == member.publicKey() {
					if item.Online == nil || *item.Online != wantOnline || item.LastSeenAt == nil || item.InRoom == nil || *item.InRoom != wantRoom {
						return fmt.Errorf("remote member presence = %+v", item)
					}
					return nil
				}
			}
			return fmt.Errorf("remote member missing: %+v", members.Items)
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertRemote(true)
	if _, err := member.client.SetServerRunWorkspace(t.Context(), "presence-join-room", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: *group.WorkspaceName}); err != nil {
		t.Fatal(err)
	}
	wantRoom = true
	assertRemote(true)
	rooms := lksdk.NewRoomServiceClient(sfuConfig.URL, sfuConfig.APIKey, sfuConfig.APISecret)
	if err := waitUntil(15*time.Second, func() error {
		page, err := rooms.ListRooms(t.Context(), &livekit.ListRoomsRequest{})
		if err != nil {
			return err
		}
		for _, room := range page.Rooms {
			participants, err := rooms.ListParticipants(t.Context(), &livekit.ListParticipantsRequest{Room: room.Name})
			if err != nil {
				return err
			}
			for _, participant := range participants.Participants {
				if participant.Identity == member.publicKey() {
					return nil
				}
			}
		}
		return fmt.Errorf("remote device has not joined LiveKit")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := member.client.StopServerRun(t.Context(), "presence-leave-room"); err != nil {
		t.Fatal(err)
	}
	wantRoom = false
	assertRemote(true)
	if _, err := member.client.SetServerRunWorkspace(t.Context(), "presence-rejoin-room", rpcapi.ServerSetRunWorkspaceRequest{WorkspaceName: *group.WorkspaceName}); err != nil {
		t.Fatal(err)
	}
	wantRoom = true
	assertRemote(true)
	if err := member.client.Close(); err != nil {
		t.Fatal(err)
	}
	wantRoom = false
	assertRemote(false)
	if err := clients[0].Close(); err != nil {
		t.Fatal(err)
	}
	friends, err := owner.client.ListFriends(t.Context(), "presence-reader-failure", rpcapi.FriendListRequest{})
	if err != nil || len(friends.Items) != 1 || friends.Items[0].Online == nil || *friends.Items[0].Online || friends.Items[0].LastSeenAt != nil {
		t.Fatalf("reader failure must not fall back to SQL or primary: %+v, %v", friends, err)
	}
}
