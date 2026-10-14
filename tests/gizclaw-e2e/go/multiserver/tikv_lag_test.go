//go:build gizclaw_e2e

package multiserver_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
)

type learnerReadClient struct {
	tikv.Client
	reads   chan string
	peers   map[uint64]tikvfixture.RegionalPeer
	address string
}

type learnerControlReadKey struct{}

func (c *learnerReadClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	control, _ := ctx.Value(learnerControlReadKey{}).(bool)
	if control && (req.Type == tikvrpc.CmdGet || req.Type == tikvrpc.CmdScan) {
		// SDK labels are preferences and may yield to a healthy center store.
		// Pin only these diagnostic reads to PD's actual Learner. Application
		// clients retain their production routing and never use this wrapper.
		peer, found := c.peers[req.RegionId]
		if !found || peer.ID == 0 || peer.StoreID == 0 {
			return nil, fmt.Errorf("diagnostic Learner missing for Region %d", req.RegionId)
		}
		routed := *req
		// client-go can clear this flag when falling back to a leader while
		// retaining the historical timestamp. Keep controls on the Learner
		// even across that retry, without changing the original SDK request.
		routed.StaleRead = true
		if err := tikvrpc.SetContext(&routed, nil, &metapb.Peer{Id: peer.ID, StoreId: peer.StoreID, Role: metapb.PeerRole_Learner}); err != nil {
			return nil, err
		}
		req = &routed
		address = c.address
	}
	resp, err := c.Client.SendRequest(ctx, address, req, timeout)
	if err == nil && control && (req.Type == tikvrpc.CmdGet || req.Type == tikvrpc.CmdScan) {
		region, decodeErr := resp.GetRegionError()
		if region == nil && decodeErr == nil {
			select {
			case c.reads <- address:
			default:
				return nil, fmt.Errorf("Learner read observation capacity exceeded")
			}
		}
	}
	return resp, err
}

// TestTiKVLearnerLagThroughBothEdges runs production Server/Edge processes and
// the public Go SDK while Raft replication into the readable Learner is frozen.
func TestTiKVLearnerLagThroughBothEdges(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	edgeA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_A"))
	edgeB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_B"))
	pd := requiredEnv(t, "GIZCLAW_E2E_TIKV_PD")
	topology, ready, err := tikvfixture.ReadTopology(ctx, pd)
	if err != nil || !ready {
		t.Fatalf("TiKV topology=%+v ready=%v err=%v", topology, ready, err)
	}
	shared := openSharedTiKV(t)
	observer := &learnerReadClient{Client: shared.GetTiKVClient(), reads: make(chan string, 128), peers: topology.LearnerPeers, address: "tikv-replication-proxy:20160"}
	shared.SetTiKVClient(observer)
	root, err := kv.NewTiKVWithClient(shared, kv.TiKVReadOptions{StaleReadDelay: time.Minute, Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerRecords := kv.Prefixed(root, kv.Key{"peers", "records"})
	assignments := kv.Prefixed(root, kv.Key{"peers", "routes"})
	credentials := kv.Prefixed(root, kv.Key{"api-keys"})
	regionalCtx := context.WithValue(ctx, learnerControlReadKey{}, true)

	revokeKey, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	deleteKeyA, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	deleteKeyB, err := giznet.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	revokePeer := connectAndServe(t, revokeKey, edgeB, serverB.PublicKey, "lag-revoke-owner")
	t.Cleanup(func() { _ = revokePeer.Close() })
	registerSocialPeer(t, ctx, revokePeer, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
	revoked, err := revokePeer.CreateAPIKey(ctx, "lag-key-create", rpcapi.APIKeyCreateRequest{DisplayName: "lag-revoke"})
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Value == nil {
		t.Fatal("SDK key creation returned no record")
	}
	deletePeerA := connectAndServe(t, deleteKeyA, edgeA, serverA.PublicKey, "lag-delete-owner-a")
	t.Cleanup(func() { _ = deletePeerA.Close() })
	registerSocialPeer(t, ctx, deletePeerA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	deletedCredentialA, err := deletePeerA.CreateAPIKey(ctx, "lag-delete-key-a", rpcapi.APIKeyCreateRequest{DisplayName: "retire-a"})
	if err != nil {
		t.Fatal(err)
	}
	deletePeerB := connectAndServe(t, deleteKeyB, edgeB, serverB.PublicKey, "lag-delete-owner-b")
	t.Cleanup(func() { _ = deletePeerB.Close() })
	registerSocialPeer(t, ctx, deletePeerB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
	deletedCredentialB, err := deletePeerB.CreateAPIKey(ctx, "lag-delete-key-b", rpcapi.APIKeyCreateRequest{DisplayName: "retire-b"})
	if err != nil {
		t.Fatal(err)
	}
	for i, client := range []*gizcli.Client{deletePeerA, deletePeerB} {
		if _, err := client.CreateWorkspace(ctx, fmt.Sprintf("lag-workspace-%d", i), rpcapi.WorkspaceCreateRequest{Name: "lag-owned-workspace", WorkflowName: "workspace-reload-echo"}); err != nil {
			t.Fatal(err)
		}
	}
	states := []string{requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE"), requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE")}
	for i, key := range []*giznet.KeyPair{deleteKeyA, deleteKeyB} {
		assertLagSQLCount(t, states[i], "runtime_profile_owners", key.Public, 1)
		assertLagSQLCount(t, states[i], "workspaces", key.Public, 1)
	}
	edges := []string{requiredEnv(t, "GIZCLAW_E2E_EDGE_A"), requiredEnv(t, "GIZCLAW_E2E_EDGE_B")}
	for _, endpoint := range edges {
		assertLagHTTPStatus(t, ctx, endpoint, revoked.APIKey, http.StatusOK)
		assertLagHTTPStatus(t, ctx, endpoint, deletedCredentialA.APIKey, http.StatusOK)
		assertLagHTTPStatus(t, ctx, endpoint, deletedCredentialB.APIKey, http.StatusOK)
	}
	// These sessions only seed the historical snapshot. Open fresh SDK sessions
	// for the lag assertions after the wait, as real reconnecting devices do.
	for _, client := range []*gizcli.Client{revokePeer, deletePeerA, deletePeerB} {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("waiting 61s for seeded credentials and active Peers to enter the regional historical snapshot")
	timer := time.NewTimer(61 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
	revokePeer = connectAndServe(t, revokeKey, edgeB, serverB.PublicKey, "lag-revoke-reconnect")
	deletePeerA = connectAndServe(t, deleteKeyA, edgeA, serverA.PublicKey, "lag-delete-reconnect-a")
	deletePeerB = connectAndServe(t, deleteKeyB, edgeB, serverB.PublicKey, "lag-delete-reconnect-b")
	proxy := requiredEnv(t, "GIZCLAW_E2E_TIKV_PROXY")
	var gate struct {
		Frozen  bool `json:"frozen"`
		Blocked int  `json:"blocked"`
	}
	if err := tikvfixture.HTTP(ctx, http.MethodPost, proxy+"/freeze", nil, &gate); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tikvfixture.HTTP(cleanup, http.MethodPost, proxy+"/resume", nil, nil); err != nil {
			t.Error(err)
		}
	})
	if !gate.Frozen || gate.Blocked == 0 {
		t.Fatalf("Raft replication was not interrupted: %+v", gate)
	}
	frozenAt := time.Now()
	beforeA, err := peerRecords.Get(regionalCtx, kv.Key{"by-pubkey", deleteKeyA.Public.String()})
	if err != nil {
		t.Fatal(err)
	}
	beforeB, err := peerRecords.Get(regionalCtx, kv.Key{"by-pubkey", deleteKeyB.Public.String()})
	if err != nil {
		t.Fatal(err)
	}
	beforeCredential, err := credentials.Get(regionalCtx, kv.Key{"secrets", revoked.APIKey})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("first-assignments-and-cross-edge-roaming", func(t *testing.T) {
		for _, owner := range []struct {
			first, second  int
			server         giznet.PublicKey
			state, foreign string
		}{
			{0, 1, serverA.PublicKey, requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE"), requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE")},
			{1, 0, serverB.PublicKey, requiredEnv(t, "GIZCLAW_E2E_SERVER_B_STATE"), requiredEnv(t, "GIZCLAW_E2E_SERVER_A_STATE")},
		} {
			key, err := giznet.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			transports := []gizcli.ServerInfoMetadata{edgeA, edgeB}
			client := connectAndServe(t, key, transports[owner.first], owner.server, "lag-first-claim")
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			client = connectAndServe(t, key, transports[owner.second], owner.server, "lag-cross-edge-owner")
			if err := client.SendBatteryTelemetry(47, false); err != nil {
				t.Fatal(err)
			}
			waitPeerRunStatus(t, owner.state, key.Public, 47)
			assertPeerRunAbsent(t, owner.foreign, key.Public)
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := assignments.Get(regionalCtx, kv.Key{"by-peer", key.Public.String()}); err != kv.ErrNotFound {
				t.Fatalf("frozen Learner unexpectedly has first assignment: %v", err)
			}
			stored, err := kv.WithStrongReads(assignments).Get(ctx, kv.Key{"by-peer", key.Public.String()})
			if err != nil {
				t.Fatal(err)
			}
			var assignment apitypes.PeerAssignment
			if err := json.Unmarshal(stored, &assignment); err != nil || assignment.ServerPublicKey != owner.server.String() || assignment.Version != 1 {
				t.Fatalf("home assignment=%+v %v", assignment, err)
			}
		}
	})
	t.Run("api-key-revocation", func(t *testing.T) {
		if _, err := revokePeer.RevokeAPIKey(ctx, "lag-revoke", rpcapi.APIKeyRevokeRequest{Name: revoked.Value.Name}); err != nil {
			t.Fatal(err)
		}
		for _, endpoint := range edges {
			assertLagHTTPStatus(t, ctx, endpoint, revoked.APIKey, http.StatusUnauthorized)
		}
		if _, err := revokePeer.Ping(ctx, "lag-revoke-control"); err != nil {
			t.Fatalf("owner transport was lost: %v", err)
		}
		value, err := credentials.Get(regionalCtx, kv.Key{"secrets", revoked.APIKey})
		if err != nil || !bytes.Equal(value, beforeCredential) {
			t.Fatalf("revocation stale control changed: %v", err)
		}
	})
	t.Run("peer-deletion-on-both-home-servers", func(t *testing.T) {
		for i, client := range []*gizcli.Client{deletePeerA, deletePeerB} {
			if _, err := client.DeletePeer(ctx, fmt.Sprintf("lag-peer-delete-%d", i), rpcapi.ServerPeerDeleteRequest{}); err != nil {
				t.Fatalf("SDK Peer delete: %v", err)
			}
		}
		for i, key := range []*giznet.KeyPair{deleteKeyA, deleteKeyB} {
			waitLagTombstone(t, ctx, kv.WithStrongReads(peerRecords), key.Public)
			assertLagSQLCount(t, states[i], "runtime_profile_owners", key.Public, 0)
			assertLagSQLCount(t, states[i], "workspaces", key.Public, 0)
			authority := serverA.PublicKey
			if i == 1 {
				authority = serverB.PublicKey
			}
			for _, transport := range []gizcli.ServerInfoMetadata{edgeA, edgeB} {
				connectMustFail(t, key, transport, authority, "deleted Peer reconnected through an Edge")
			}
		}
		for _, endpoint := range edges {
			assertLagHTTPStatus(t, ctx, endpoint, deletedCredentialA.APIKey, http.StatusUnauthorized)
			assertLagHTTPStatus(t, ctx, endpoint, deletedCredentialB.APIKey, http.StatusUnauthorized)
		}
		for i, key := range []*giznet.KeyPair{deleteKeyA, deleteKeyB} {
			old := beforeA
			if i == 1 {
				old = beforeB
			}
			value, err := peerRecords.Get(regionalCtx, kv.Key{"by-pubkey", key.Public.String()})
			if err != nil || !bytes.Equal(value, old) {
				t.Fatalf("deleted Peer stale control changed: %v", err)
			}
		}
	})
	if time.Since(frozenAt) > 50*time.Second {
		t.Fatal("SDK lag assertions exceeded the historical snapshot window")
	}
	observations := 0
	for len(observer.reads) > 0 {
		address := <-observer.reads
		observations++
		if address != "tikv-replication-proxy:20160" {
			t.Fatalf("stale control escaped the regional Learner: %s", address)
		}
	}
	if observations < 8 {
		t.Fatalf("missing regional read proof: %d", observations)
	}
	if err := tikvfixture.HTTP(ctx, http.MethodGet, proxy+"/status", nil, &gate); err != nil || !gate.Frozen {
		t.Fatalf("replication resumed during lag assertions: %+v %v", gate, err)
	}
	t.Logf("complete SDK/Edge/Server TiKV lag chain passed: both home assignments, both Edge HTTP revocations, both Peer tombstones; %d successful regional stale controls, Raft still frozen", observations)
}

func assertLagHTTPStatus(t *testing.T, ctx context.Context, endpoint, secret string, want int) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Edge credential request: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("Edge %s credential status=%d, want %d", endpoint, resp.StatusCode, want)
	}
}
func waitLagTombstone(t *testing.T, ctx context.Context, store kv.Store, key giznet.PublicKey) {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, err := store.Get(ctx, kv.Key{"by-pubkey", key.String()})
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(value, []byte(`{"version":1,"state":"deleted"}`)) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("authoritative Peer deletion did not finalize")
		case <-ticker.C:
		}
	}
}

func assertLagSQLCount(t *testing.T, path, table string, owner giznet.PublicKey, want int) {
	t.Helper()
	switch table {
	case "runtime_profile_owners", "workspaces":
	default:
		t.Fatal("unsupported lifecycle table")
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout%3d5000")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE owner_public_key=?", owner.String()).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("home SQL %s owner count=%d, want %d", table, count, want)
	}
}
