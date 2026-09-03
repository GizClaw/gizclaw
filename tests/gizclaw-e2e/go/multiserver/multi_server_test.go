//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet/gizwebrtc"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	redis "github.com/redis/go-redis/v9"
	_ "modernc.org/sqlite"
)

func TestSharedAssignmentRoutesAcrossBothEdges(t *testing.T) {
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	edgeA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_A"))
	edgeB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_B"))
	stateA := requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE")
	stateB := requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE")
	shared := openRedis(t)
	if serverA.PublicKey.Equal(serverB.PublicKey) {
		t.Fatal("the two Servers use the same identity")
	}
	if !edgeA.PublicKey.Equal(serverA.PublicKey) {
		t.Fatalf("Edge A bootstrapped from %s, want Server A %s", edgeA.PublicKey, serverA.PublicKey)
	}
	if !edgeB.PublicKey.Equal(serverB.PublicKey) {
		t.Fatalf("Edge B bootstrapped from %s, want Server B %s", edgeB.PublicKey, serverB.PublicKey)
	}

	peerA, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peerC, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	clientA := connectAndServe(t, peerA, serverA, serverA.PublicKey, "peer-a-direct-home")
	if err := clientA.SendBatteryTelemetry(41, false); err != nil {
		t.Fatalf("send Peer A direct telemetry: %v", err)
	}
	waitPeerRunStatus(t, stateA, peerA.Public, 41)
	assertPeerRunAbsent(t, stateB, peerA.Public)
	if err := clientA.Close(); err != nil {
		t.Fatalf("close Peer A direct client: %v", err)
	}

	clientB := connectAndServe(t, peerB, serverB, serverB.PublicKey, "peer-b-direct-home")
	if err := clientB.SendBatteryTelemetry(51, true); err != nil {
		t.Fatalf("send Peer B direct telemetry: %v", err)
	}
	waitPeerRunStatus(t, stateB, peerB.Public, 51)
	assertPeerRunAbsent(t, stateA, peerB.Public)
	if err := clientB.Close(); err != nil {
		t.Fatalf("close Peer B direct client: %v", err)
	}

	// Peer C has no assignment. Edge B's ordered bootstrap starts with Server B,
	// which must claim it exactly once; Edge A must then route back to Server B.
	clientC := connectAndServe(t, peerC, edgeB, serverB.PublicKey, "peer-c-first-claim-edge-b")
	if err := clientC.SendBatteryTelemetry(61, false); err != nil {
		t.Fatalf("send Peer C first-claim telemetry: %v", err)
	}
	waitPeerRunStatus(t, stateB, peerC.Public, 61)
	assertPeerRunAbsent(t, stateA, peerC.Public)
	if err := clientC.Close(); err != nil {
		t.Fatalf("close Peer C first-claim client: %v", err)
	}
	clientC = connectAndServe(t, peerC, edgeA, serverB.PublicKey, "peer-c-fixed-owner-edge-a")
	if err := clientC.SendBatteryTelemetry(62, true); err != nil {
		t.Fatalf("send Peer C fixed-owner telemetry: %v", err)
	}
	waitPeerRunStatus(t, stateB, peerC.Public, 62)
	assertPeerRunAbsent(t, stateA, peerC.Public)
	if err := clientC.Close(); err != nil {
		t.Fatalf("close Peer C fixed-owner client: %v", err)
	}

	sharedBeforeForeign := redisSnapshot(t, shared)
	serverARunsBeforeForeign := sqlTableSnapshot(t, stateA, "peer_runs")
	serverBRunsBeforeForeign := sqlTableSnapshot(t, stateB, "peer_runs")
	connectMustFail(t, peerA, serverB, serverB.PublicKey, "peer A admitted by foreign Server B")
	connectMustFail(t, peerB, serverA, serverA.PublicKey, "peer B admitted by foreign Server A")
	connectMustFail(t, peerC, serverA, serverA.PublicKey, "Peer C admitted by foreign Server A")
	assertSnapshotEqual(t, "shared Redis after foreign admission", sharedBeforeForeign, redisSnapshot(t, shared))
	assertSnapshotEqual(t, "Server A PeerRun after foreign admission", serverARunsBeforeForeign, sqlTableSnapshot(t, stateA, "peer_runs"))
	assertSnapshotEqual(t, "Server B PeerRun after foreign admission", serverBRunsBeforeForeign, sqlTableSnapshot(t, stateB, "peer_runs"))

	for _, tc := range []struct {
		name         string
		peer         *giznet.KeyPair
		edge         gizcli.ServerInfoMetadata
		server       giznet.PublicKey
		homeState    string
		foreignState string
		battery      int
	}{
		{name: "peer-a-edge-a", peer: peerA, edge: edgeA, server: serverA.PublicKey, homeState: stateA, foreignState: stateB, battery: 42},
		{name: "peer-a-edge-b", peer: peerA, edge: edgeB, server: serverA.PublicKey, homeState: stateA, foreignState: stateB, battery: 43},
		{name: "peer-b-edge-a", peer: peerB, edge: edgeA, server: serverB.PublicKey, homeState: stateB, foreignState: stateA, battery: 52},
		{name: "peer-b-edge-b", peer: peerB, edge: edgeB, server: serverB.PublicKey, homeState: stateB, foreignState: stateA, battery: 53},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := connectAndServe(t, tc.peer, tc.edge, tc.server, tc.name)
			if err := client.SendBatteryTelemetry(tc.battery, false); err != nil {
				_ = client.Close()
				t.Fatalf("send telemetry: %v", err)
			}
			waitPeerRunStatus(t, tc.homeState, tc.peer.Public, tc.battery)
			assertPeerRunAbsent(t, tc.foreignState, tc.peer.Public)
			if err := client.Close(); err != nil {
				t.Fatalf("close routed client: %v", err)
			}
		})
	}
	assertSnapshotEqual(t, "shared Redis after local-only telemetry", sharedBeforeForeign, redisSnapshot(t, shared))

	// A failed foreign admission must not transfer ownership. Reconnect through
	// the Edge whose bootstrap order starts with the foreign Server.
	connectAndPing(t, peerA, edgeB, serverA.PublicKey, "peer-a-owner-still-a")
	connectAndPing(t, peerB, edgeA, serverB.PublicKey, "peer-b-owner-still-b")
}

// TestCrossServerSocialSharesWorkspaceIdentity creates a Friend and a Friend
// Group across the two Servers. Both sides must resolve the same Workspace
// identity from the shared Social KV, invites are consumed, and PeerRun
// ownership stays local to the Server that admitted each Peer.
func TestCrossServerSocialSharesWorkspaceIdentity(t *testing.T) {
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	stateA := requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE")
	stateB := requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE")
	shared := openRedis(t)
	peerA, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	peerB, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientA := connectAndServe(t, peerA, serverA, serverA.PublicKey, "social-peer-a")
	defer clientA.Close()
	clientB := connectAndServe(t, peerB, serverB, serverB.PublicKey, "social-peer-b")
	defer clientB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	registerSocialPeer(t, ctx, clientA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	registerSocialPeer(t, ctx, clientB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
	if err := clientA.SendBatteryTelemetry(71, false); err != nil {
		t.Fatalf("send Peer A telemetry: %v", err)
	}
	if err := clientB.SendBatteryTelemetry(72, false); err != nil {
		t.Fatalf("send Peer B telemetry: %v", err)
	}
	waitPeerRunStatus(t, stateA, peerA.Public, 71)
	waitPeerRunStatus(t, stateB, peerB.Public, 72)
	assertPeerRunAbsent(t, stateB, peerA.Public)
	assertPeerRunAbsent(t, stateA, peerB.Public)

	// Friend: invite minted on Server B, accepted from Server A.
	friendInvite, err := clientB.CreateFriendInviteToken(ctx, "friend-invite-create", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("create Friend invite: %v", err)
	}
	sharedBeforeFriend := redisSnapshot(t, shared)
	friend, err := clientA.AddFriend(ctx, "friend-add", rpcapi.FriendAddRequest{InviteToken: friendInvite.InviteToken})
	if err != nil {
		t.Fatalf("cross-server Friend creation: %v", err)
	}
	defer func() {
		_, _ = clientA.DeleteFriend(context.Background(), "friend-delete", rpcapi.FriendDeleteRequest{Name: friend.Name})
	}()
	if friend.WorkspaceName == nil || *friend.WorkspaceName == "" {
		t.Fatalf("cross-server Friend has no Workspace: %+v", friend)
	}
	if snapshotsEqual(sharedBeforeFriend, redisSnapshot(t, shared)) {
		t.Fatal("shared Redis did not record the cross-server Friend")
	}
	ownerFriends, err := clientB.ListFriends(ctx, "friend-owner-list", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("list invite owner Friends: %v", err)
	}
	if len(ownerFriends.Items) != 1 || ownerFriends.Items[0].WorkspaceName == nil {
		t.Fatalf("invite owner Friends = %+v, want one relationship with a Workspace", ownerFriends.Items)
	}
	if *ownerFriends.Items[0].WorkspaceName != *friend.WorkspaceName {
		t.Fatalf("Friend Workspace differs across Servers: A=%q B=%q", *friend.WorkspaceName, *ownerFriends.Items[0].WorkspaceName)
	}
	if ownerFriends.Items[0].PeerPublicKey == nil || *ownerFriends.Items[0].PeerPublicKey != peerA.Public.String() {
		t.Fatalf("invite owner Friend peer = %v, want %s", ownerFriends.Items[0].PeerPublicKey, peerA.Public)
	}
	// Friend invites are long-lived until cleared or expired; acceptance must
	// neither rotate the token nor let a replay create a second relationship.
	friendInviteAfter, err := clientB.GetFriendInviteToken(ctx, "friend-invite-get", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("get Friend invite after acceptance: %v", err)
	}
	if friendInviteAfter.InviteToken == nil || *friendInviteAfter.InviteToken != friendInvite.InviteToken {
		t.Fatalf("Friend invite changed after cross-server acceptance: got %v, want %q", friendInviteAfter.InviteToken, friendInvite.InviteToken)
	}
	_, _ = clientA.AddFriend(ctx, "friend-add-replay", rpcapi.FriendAddRequest{InviteToken: friendInvite.InviteToken})
	friends, err := clientA.ListFriends(ctx, "friend-list", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("list Friends after replay: %v", err)
	}
	if len(friends.Items) != 1 || friends.Items[0].WorkspaceName == nil || *friends.Items[0].WorkspaceName != *friend.WorkspaceName {
		t.Fatalf("Friends after replay = %+v, want exactly the original relationship", friends.Items)
	}

	// Friend Group: created on Server B, joined from Server A.
	group, err := clientB.CreateFriendGroup(ctx, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "server-b-room"})
	if err != nil {
		t.Fatalf("create FriendGroup: %v", err)
	}
	defer func() {
		_, _ = clientB.DeleteFriendGroup(context.Background(), "friend-group-delete", rpcapi.FriendGroupDeleteRequest{Name: group.Name})
	}()
	if group.WorkspaceName == nil || *group.WorkspaceName == "" {
		t.Fatalf("FriendGroup has no Workspace: %+v", group)
	}
	groupInvite, err := clientB.CreateFriendGroupInviteToken(ctx, "friend-group-invite-create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("create FriendGroup invite: %v", err)
	}
	sharedBeforeJoin := redisSnapshot(t, shared)
	joined, err := clientA.JoinFriendGroup(ctx, "friend-group-join", rpcapi.FriendGroupJoinRequest{
		InviteToken: groupInvite.InviteToken,
		Name:        "server-b-room",
	})
	if err != nil {
		t.Fatalf("cross-server FriendGroup join: %v", err)
	}
	if joined.Group.WorkspaceName == nil || *joined.Group.WorkspaceName != *group.WorkspaceName {
		t.Fatalf("FriendGroup Workspace differs across Servers: owner=%q member=%v", *group.WorkspaceName, joined.Group.WorkspaceName)
	}
	if snapshotsEqual(sharedBeforeJoin, redisSnapshot(t, shared)) {
		t.Fatal("shared Redis did not record the cross-server FriendGroup membership")
	}
	groupName := group.Name
	members, err := clientB.ListFriendGroupMembers(ctx, "friend-group-owner-members", rpcapi.FriendGroupMemberListRequest{FriendGroupName: &groupName})
	if err != nil {
		t.Fatalf("list FriendGroup members: %v", err)
	}
	memberKeys := make([]string, 0, len(members.Items))
	for _, member := range members.Items {
		if member.PeerPublicKey != nil {
			memberKeys = append(memberKeys, *member.PeerPublicKey)
		}
	}
	sort.Strings(memberKeys)
	wantKeys := []string{peerA.Public.String(), peerB.Public.String()}
	sort.Strings(wantKeys)
	if strings.Join(memberKeys, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("FriendGroup members = %v, want %v", memberKeys, wantKeys)
	}
	groups, err := clientA.ListFriendGroups(ctx, "friend-group-list", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("list member FriendGroups: %v", err)
	}
	if len(groups.Items) != 1 || groups.Items[0].WorkspaceName == nil || *groups.Items[0].WorkspaceName != *group.WorkspaceName {
		t.Fatalf("member FriendGroups = %+v, want one Group with Workspace %q", groups.Items, *group.WorkspaceName)
	}

	// Social mutations never move PeerRun ownership or leak it into Redis.
	assertPeerRunAbsent(t, stateB, peerA.Public)
	assertPeerRunAbsent(t, stateA, peerB.Public)
	for key := range redisSnapshot(t, shared) {
		if strings.Contains(key, "runs") {
			t.Fatalf("shared Redis holds PeerRun key %q", key)
		}
	}
}

type byteSnapshot map[string][]byte

func openRedis(t *testing.T) *redis.Client {
	t.Helper()
	options, err := redis.ParseURL(requiredEnv(t, "GIZCLAW_E2E_REDIS_DSN"))
	if err != nil {
		t.Fatalf("parse Redis E2E DSN: %v", err)
	}
	client := redis.NewClient(options)
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping shared Redis: %v", err)
	}
	return client
}

func redisSnapshot(t *testing.T, client *redis.Client) byteSnapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	keys := make([]string, 0)
	iterator := client.Scan(ctx, 0, "*", 0).Iterator()
	for iterator.Next(ctx) {
		keys = append(keys, iterator.Val())
	}
	if err := iterator.Err(); err != nil {
		t.Fatalf("scan shared Redis: %v", err)
	}
	sort.Strings(keys)
	snapshot := make(byteSnapshot, len(keys))
	for _, key := range keys {
		value, err := client.Get(ctx, key).Bytes()
		if err != nil {
			t.Fatalf("read shared Redis snapshot key %q: %v", key, err)
		}
		snapshot[key] = value
	}
	return snapshot
}

func sqlTableSnapshot(t *testing.T, path, table string) byteSnapshot {
	t.Helper()
	snapshot, err := querySQLTableSnapshot(path, table)
	if err != nil {
		t.Fatalf("snapshot %s table %q: %v", path, table, err)
	}
	return snapshot
}

func querySQLTableSnapshot(path, table string) (byteSnapshot, error) {
	switch table {
	case "peer_runs", "runtime-profiles", "workflows", "workspaces":
	default:
		return nil, fmt.Errorf("unsupported E2E table %q", table)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout%3d5000")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `SELECT encoded_key, value FROM "`+table+`" ORDER BY encoded_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	snapshot := make(byteSnapshot)
	for rows.Next() {
		var key, value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		snapshot[string(key)] = append([]byte(nil), value...)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func assertSnapshotEqual(t *testing.T, name string, before, after byteSnapshot) {
	t.Helper()
	if maps.EqualFunc(before, after, func(a, b []byte) bool { return string(a) == string(b) }) {
		return
	}
	changed := make([]string, 0)
	for key, beforeValue := range before {
		afterValue, ok := after[key]
		if !ok || string(beforeValue) != string(afterValue) {
			changed = append(changed, key)
		}
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			changed = append(changed, key)
		}
	}
	sort.Strings(changed)
	t.Fatalf("%s changed keys: %s", name, strings.Join(changed, ", "))
}

func waitPeerRunStatus(t *testing.T, path string, publicKey giznet.PublicKey, battery int) {
	t.Helper()
	key := "runs:by-peer:" + publicKey.String() + ":status"
	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		snapshot, err := querySQLTableSnapshot(path, "peer_runs")
		if err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		value, ok := snapshot[key]
		if !ok {
			lastErr = errors.New("status key is absent")
			time.Sleep(100 * time.Millisecond)
			continue
		}
		var status apitypes.PeerStatus
		if err := json.Unmarshal(value, &status); err != nil {
			lastErr = err
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if status.BatteryPercent != nil && *status.BatteryPercent == battery {
			return
		}
		lastErr = fmt.Errorf("battery = %v, want %d", status.BatteryPercent, battery)
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("wait for local PeerRun status %s in %s: %v", publicKey, path, lastErr)
}

func assertPeerRunAbsent(t *testing.T, path string, publicKey giznet.PublicKey) {
	t.Helper()
	key := "runs:by-peer:" + publicKey.String() + ":status"
	if _, ok := sqlTableSnapshot(t, path, "peer_runs")[key]; ok {
		t.Fatalf("foreign PeerRun status %s exists in %s", publicKey, path)
	}
}

// registerSocialPeer registers the Peer on its home Server with the token
// seeded by run_multi_server_tests.sh. Both Servers seed the same
// RuntimeProfile ID, which cross-server Workspace activation relies on to
// resolve the Workspace owner's profile locally.
func registerSocialPeer(
	t *testing.T,
	ctx context.Context,
	peer *gizcli.Client,
	server gizcli.ServerInfoMetadata,
	tokenEnv string,
) {
	t.Helper()
	if _, err := peer.Register(ctx, "multi-server-social-register", requiredEnv(t, tokenEnv)); err != nil {
		t.Fatalf("register Social Peer on %s: %v", server.PublicKey, err)
	}
}

func snapshotsEqual(before, after byteSnapshot) bool {
	return maps.EqualFunc(before, after, func(a, b []byte) bool { return string(a) == string(b) })
}

func fetchServer(t *testing.T, endpoint string) gizcli.ServerInfoMetadata {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := gizcli.FetchServerInfo(ctx, endpoint)
	if err != nil {
		t.Fatalf("fetch %s: %v", endpoint, err)
	}
	return info
}

func connectAndPing(
	t *testing.T,
	key *giznet.KeyPair,
	transport gizcli.ServerInfoMetadata,
	authoritative giznet.PublicKey,
	id string,
) {
	t.Helper()
	client := connectAndServe(t, key, transport, authoritative, id)
	defer client.Close()
}

func connectAndServe(
	t *testing.T,
	key *giznet.KeyPair,
	transport gizcli.ServerInfoMetadata,
	authoritative giznet.PublicKey,
	id string,
) *gizcli.Client {
	t.Helper()
	client, err := connect(key, transport, authoritative)
	if err != nil {
		t.Fatalf("%s connect: %v", id, err)
	}
	go func() { _ = client.Serve() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := client.Ping(ctx, id); err != nil {
		_ = client.Close()
		t.Fatalf("%s ping: %v", id, err)
	}
	return client
}

func connectMustFail(
	t *testing.T,
	key *giznet.KeyPair,
	transport gizcli.ServerInfoMetadata,
	authoritative giznet.PublicKey,
	description string,
) {
	t.Helper()
	client, err := connect(key, transport, authoritative)
	if err != nil {
		return
	}
	defer client.Close()
	go func() { _ = client.Serve() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, pingErr := client.Ping(ctx, "foreign-admission"); pingErr == nil {
		t.Fatal(description)
	}
}

func connect(
	key *giznet.KeyPair,
	transport gizcli.ServerInfoMetadata,
	authoritative giznet.PublicKey,
) (*gizcli.Client, error) {
	client := &gizcli.Client{
		KeyPair: key,
		DialTransport: func(
			key *giznet.KeyPair,
			_ giznet.PublicKey,
			_ string,
			policy giznet.SecurityPolicy,
		) (giznet.Listener, giznet.Conn, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			return gizwebrtc.Dial(ctx, key, transport.TransportPublicKey, gizwebrtc.DialConfig{
				SignalingURL:   transport.SignalingURL,
				ICEServers:     transport.ICEServers,
				SecurityPolicy: policy,
			})
		},
	}
	if err := client.Dial(authoritative, authoritative.String()); err != nil {
		return nil, err
	}
	return client, nil
}

func requiredEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}
