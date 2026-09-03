//go:build gizclaw_e2e

package multiserver_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/adminhttp"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	eventpb "github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/eventproto"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/peerhttp"
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

func TestAPIKeyHTTPRoutesAcrossEdgeToOwnerServer(t *testing.T) {
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	edgeB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_B"))
	edgeAEndpoint := requiredEnv(t, "GIZCLAW_E2E_EDGE_A")

	peerB, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	clientB := connectAndServe(t, peerB, edgeB, serverB.PublicKey, "api-key-owner-server-b-via-edge-b")
	defer clientB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	registerSocialPeer(t, ctx, clientB, serverB)
	created, err := clientB.CreateAPIKey(ctx, "multi-server-api-key", rpcapi.APIKeyCreateRequest{
		DisplayName: "multi-server-edge-route",
	})
	if err != nil {
		t.Fatalf("create API key on Server B: %v", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		"http://"+edgeAEndpoint+"/gizclaw/v1/api-keys/self",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+created.APIKey)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request Server B API key through Edge A: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("request Server B API key through Edge A status=%d body=%s", response.StatusCode, body)
	}
	var got peerhttp.APIKey
	if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
		t.Fatalf("decode API key response through Edge A: %v", err)
	}
	if got.ApiKey != created.APIKey || got.DisplayName != "multi-server-edge-route" {
		t.Fatalf("API key through Edge A = %+v, want Server B key %q", got, created.APIKey)
	}
}

func TestCrossServerSocialRejectsWithoutConsumingInvites(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	registerSocialPeer(t, ctx, clientB, serverB)
	if got := len(sqlTableSnapshot(t, stateA, "runtime-profiles")); got != 0 {
		t.Fatalf("Server A RuntimeProfile rows = %d, want 0", got)
	}
	if got := len(sqlTableSnapshot(t, stateB, "runtime-profiles")); got == 0 {
		t.Fatal("Server B RuntimeProfile store is empty after local provisioning")
	}
	if got := len(sqlTableSnapshot(t, stateA, "workflows")); got != 0 {
		t.Fatalf("Server A Workflow rows = %d, want 0", got)
	}
	if got := len(sqlTableSnapshot(t, stateB, "workflows")); got == 0 {
		t.Fatal("Server B Workflow store is empty after local provisioning")
	}

	friendInvite, err := clientB.CreateFriendInviteToken(ctx, "friend-invite-create", rpcapi.FriendInviteTokenCreateRequest{})
	if err != nil {
		t.Fatalf("create Friend invite: %v", err)
	}
	friendSharedBefore := redisSnapshot(t, shared)
	friendWorkspaceABefore := sqlTableSnapshot(t, stateA, "workspaces")
	friendWorkspaceBBefore := sqlTableSnapshot(t, stateB, "workspaces")
	assertNoPeerEvents(t, []*gizcli.Client{clientA, clientB}, func() {
		for attempt := 1; attempt <= 2; attempt++ {
			_, err = clientA.AddFriend(ctx, "friend-add", rpcapi.FriendAddRequest{InviteToken: friendInvite.InviteToken})
			assertRPCFailedPrecondition(t, err, "cross-server friend creation is not supported")
		}
	})
	assertSnapshotEqual(t, "shared Redis after Friend rejection", friendSharedBefore, redisSnapshot(t, shared))
	assertSnapshotEqual(t, "Server A Workspaces after Friend rejection", friendWorkspaceABefore, sqlTableSnapshot(t, stateA, "workspaces"))
	assertSnapshotEqual(t, "Server B Workspaces after Friend rejection", friendWorkspaceBBefore, sqlTableSnapshot(t, stateB, "workspaces"))
	friendInviteAfter, err := clientB.GetFriendInviteToken(ctx, "friend-invite-get", rpcapi.FriendInviteTokenGetRequest{})
	if err != nil {
		t.Fatalf("get Friend invite after rejection: %v", err)
	}
	if friendInviteAfter.InviteToken == nil || *friendInviteAfter.InviteToken != friendInvite.InviteToken {
		t.Fatalf("Friend invite changed after rejection: got %v, want %q", friendInviteAfter.InviteToken, friendInvite.InviteToken)
	}
	friends, err := clientA.ListFriends(ctx, "friend-list", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("list Friends after rejection: %v", err)
	}
	if len(friends.Items) != 0 {
		t.Fatalf("cross-server Friend rejection created %d relationship(s)", len(friends.Items))
	}
	ownerFriends, err := clientB.ListFriends(ctx, "friend-owner-list", rpcapi.FriendListRequest{})
	if err != nil {
		t.Fatalf("list invite owner Friends after rejection: %v", err)
	}
	if len(ownerFriends.Items) != 0 {
		t.Fatalf("cross-server Friend rejection created %d reciprocal relationship(s)", len(ownerFriends.Items))
	}

	group, err := clientB.CreateFriendGroup(ctx, "friend-group-create", rpcapi.FriendGroupCreateRequest{Name: "server-b-room"})
	if err != nil {
		t.Fatalf("create FriendGroup: %v", err)
	}
	if got := len(sqlTableSnapshot(t, stateA, "workspaces")); got != 0 {
		t.Fatalf("Server A Workspace rows after Server B group creation = %d, want 0", got)
	}
	if got := len(sqlTableSnapshot(t, stateB, "workspaces")); got == 0 {
		t.Fatal("Server B Workspace store is empty after local FriendGroup creation")
	}
	groupInvite, err := clientB.CreateFriendGroupInviteToken(ctx, "friend-group-invite-create", rpcapi.FriendGroupInviteTokenCreateRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("create FriendGroup invite: %v", err)
	}
	groupSharedBefore := redisSnapshot(t, shared)
	groupWorkspaceABefore := sqlTableSnapshot(t, stateA, "workspaces")
	groupWorkspaceBBefore := sqlTableSnapshot(t, stateB, "workspaces")
	assertNoPeerEvents(t, []*gizcli.Client{clientA, clientB}, func() {
		for attempt := 1; attempt <= 2; attempt++ {
			_, err = clientA.JoinFriendGroup(ctx, "friend-group-join", rpcapi.FriendGroupJoinRequest{
				InviteToken: groupInvite.InviteToken,
				Name:        "server-b-room",
			})
			assertRPCFailedPrecondition(t, err, "cross-server friend group membership is not supported")
		}
	})
	assertSnapshotEqual(t, "shared Redis after FriendGroup rejection", groupSharedBefore, redisSnapshot(t, shared))
	assertSnapshotEqual(t, "Server A Workspaces after FriendGroup rejection", groupWorkspaceABefore, sqlTableSnapshot(t, stateA, "workspaces"))
	assertSnapshotEqual(t, "Server B Workspaces after FriendGroup rejection", groupWorkspaceBBefore, sqlTableSnapshot(t, stateB, "workspaces"))
	groupInviteAfter, err := clientB.GetFriendGroupInviteToken(ctx, "friend-group-invite-get", rpcapi.FriendGroupInviteTokenGetRequest{FriendGroupName: group.Name})
	if err != nil {
		t.Fatalf("get FriendGroup invite after rejection: %v", err)
	}
	if groupInviteAfter.InviteToken == nil || *groupInviteAfter.InviteToken != groupInvite.InviteToken {
		t.Fatalf("FriendGroup invite changed after rejection: got %v, want %q", groupInviteAfter.InviteToken, groupInvite.InviteToken)
	}
	groups, err := clientA.ListFriendGroups(ctx, "friend-group-list", rpcapi.FriendGroupListRequest{})
	if err != nil {
		t.Fatalf("list FriendGroups after rejection: %v", err)
	}
	if len(groups.Items) != 0 {
		t.Fatalf("cross-server FriendGroup rejection created %d membership(s)", len(groups.Items))
	}
	groupName := group.Name
	members, err := clientB.ListFriendGroupMembers(ctx, "friend-group-owner-members", rpcapi.FriendGroupMemberListRequest{FriendGroupName: &groupName})
	if err != nil {
		t.Fatalf("list FriendGroup owner members after rejection: %v", err)
	}
	if len(members.Items) != 1 || members.Items[0].PeerPublicKey == nil || *members.Items[0].PeerPublicKey != peerB.Public.String() {
		t.Fatalf("FriendGroup members after rejection = %+v, want only owner %s", members.Items, peerB.Public)
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

func registerSocialPeer(
	t *testing.T,
	ctx context.Context,
	peer *gizcli.Client,
	server gizcli.ServerInfoMetadata,
) {
	t.Helper()
	var private giznet.Key
	if err := private.UnmarshalText([]byte(requiredEnv(t, "GIZCLAW_E2E_ADMIN_PRIVATE_KEY"))); err != nil {
		t.Fatalf("parse multi-server admin private key: %v", err)
	}
	adminKey, err := giznet.NewKeyPair(private)
	if err != nil {
		t.Fatalf("derive multi-server admin key: %v", err)
	}
	admin := connectAndServe(t, adminKey, server, server.PublicKey, "multi-server-admin")
	defer admin.Close()
	api, err := admin.ServerAdminClient()
	if err != nil {
		t.Fatalf("create multi-server Admin client: %v", err)
	}
	prefix := fmt.Sprintf("multi-server-social-%d", time.Now().UnixNano())
	workflowIDs := make([]string, 0, 3)
	for index := range 3 {
		id := fmt.Sprintf("%s-workflow-%d", prefix, index)
		response, err := api.CreateWorkflowWithResponse(ctx, socialWorkflow(id, index))
		if err != nil {
			t.Fatalf("create Social Workflow %d: %v", index, err)
		}
		if response.JSON200 == nil {
			t.Fatalf("create Social Workflow %d status=%d body=%s", index, response.StatusCode(), response.Body)
		}
		workflowIDs = append(workflowIDs, response.JSON200.Id)
	}
	profileID := prefix + "-profile"
	profile, err := api.CreateRuntimeProfileWithResponse(ctx, adminhttp.RuntimeProfileUpsert{
		Id: profileID,
		Spec: apitypes.RuntimeProfileSpec{
			Resources: apitypes.RuntimeProfileResources{},
			Workflows: apitypes.RuntimeProfileWorkflows{
				Collections: apitypes.RuntimeProfileWorkflowCollections{},
				System: apitypes.RuntimeProfileSystemWorkflows{
					FriendChatroom: workflowIDs[0],
					GroupChatroom:  workflowIDs[1],
					Pet:            workflowIDs[2],
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("create Social RuntimeProfile: %v", err)
	}
	if profile.JSON200 == nil {
		t.Fatalf("create Social RuntimeProfile status=%d body=%s", profile.StatusCode(), profile.Body)
	}
	tokenID := prefix + "-token"
	token, err := api.CreateRegistrationTokenWithResponse(ctx, adminhttp.RegistrationTokenUpsert{
		Id:               tokenID,
		Token:            tokenID,
		RuntimeProfileId: profile.JSON200.Id,
	})
	if err != nil {
		t.Fatalf("create Social RegistrationToken: %v", err)
	}
	if token.JSON200 == nil {
		t.Fatalf("create Social RegistrationToken status=%d body=%s", token.StatusCode(), token.Body)
	}
	if _, err := peer.Register(ctx, "multi-server-social-register", token.JSON200.Token); err != nil {
		t.Fatalf("register Social Peer on Server B: %v", err)
	}
}

func socialWorkflow(id string, index int) adminhttp.WorkflowUpsert {
	spec := apitypes.WorkflowSpec{
		Driver:   apitypes.WorkflowDriverChatroom,
		Chatroom: &apitypes.ChatRoomWorkflowSpec{},
	}
	if index == 2 {
		spec = apitypes.WorkflowSpec{
			Driver: apitypes.WorkflowDriverPet,
			Pet: &apitypes.PetWorkflowSpec{
				Driver:   apitypes.ReusableWorkflowDriverChatroom,
				Chatroom: &apitypes.ChatRoomWorkflowSpec{},
			},
		}
	}
	return adminhttp.WorkflowUpsert{Id: id, Spec: spec}
}

type eventReadResult struct {
	client int
	event  *eventpb.PeerEvent
	err    error
}

func assertNoPeerEvents(t *testing.T, clients []*gizcli.Client, operation func()) {
	t.Helper()
	streams := make([]io.Closer, 0, len(clients))
	results := make(chan eventReadResult, len(clients))
	for index, client := range clients {
		stream, err := client.DialPeerEventStream()
		if err != nil {
			t.Fatalf("open Peer Event probe %d: %v", index, err)
		}
		streams = append(streams, stream)
		go func() {
			for {
				event, err := gizcli.ReadPeerStreamEvent(stream)
				results <- eventReadResult{client: index, event: event, err: err}
				if err != nil {
					return
				}
			}
		}()
	}
	defer func() {
		for _, stream := range streams {
			_ = stream.Close()
		}
	}()
	armingTimer := time.NewTimer(250 * time.Millisecond)
	arming := true
	for arming {
		select {
		case result := <-results:
			if result.err != nil {
				t.Fatalf("Peer Event probe %d failed while arming: %v", result.client, result.err)
			}
		case <-armingTimer.C:
			arming = false
		}
	}
	operation()
	timer := time.NewTimer(750 * time.Millisecond)
	defer timer.Stop()
	select {
	case result := <-results:
		t.Fatalf("Peer Event probe %d completed after rejected Social mutation: event=%+v err=%v", result.client, result.event, result.err)
	case <-timer.C:
	}
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

func assertRPCFailedPrecondition(t *testing.T, err error, message string) {
	t.Helper()
	var rpcErr rpcapi.Error
	if !errors.As(err, &rpcErr) {
		t.Fatalf("error = %v, want RPC failed-precondition %q", err, message)
	}
	if rpcErr.Code != rpcapi.StatusCodeFailedPrecondition || rpcErr.Message != message {
		t.Fatalf("RPC error = (%d, %q), want (%d, %q)", rpcErr.Code, rpcErr.Message, rpcapi.StatusCodeFailedPrecondition, message)
	}
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
