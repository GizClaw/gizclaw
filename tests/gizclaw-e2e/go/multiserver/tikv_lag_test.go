//go:build gizclaw_e2e

package multiserver_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/apitypes"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/api/rpcapi"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/sdk/go/gizcli"
	"github.com/GizClaw/gizclaw-go/tests/testsupport/tikvfixture"
	"github.com/pingcap/kvproto/pkg/metapb"
	tikvkv "github.com/tikv/client-go/v2/kv"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/tikvrpc/interceptor"
	"github.com/tikv/client-go/v2/util/async"
)

type learnerReadObservation struct {
	Address     string
	RegionID    uint64
	PeerID      uint64
	StoreID     uint64
	Role        metapb.PeerRole
	ReplicaRead tikvkv.ReplicaReadType
	Stale       bool
	Type        tikvrpc.CmdType
}

type learnerReadClient struct {
	tikv.Client
	mu       sync.Mutex
	attempts []learnerReadObservation
}

type learnerControlReadKey struct{}

func (c *learnerReadClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	// The borrowed SDK client runs this interceptor after the production routing
	// guard. Rejected selections are not data attempts; failed dispatches are.
	ctx = interceptor.WithRPCInterceptor(ctx, interceptor.NewRPCInterceptor("lag-test-dispatch", func(next interceptor.RPCInterceptorFunc) interceptor.RPCInterceptorFunc {
		return func(target string, request *tikvrpc.Request) (*tikvrpc.Response, error) {
			c.recordAttempt(ctx, target, request)
			return next(target, request)
		}
	}))
	return c.Client.SendRequest(ctx, address, req, timeout)
}

func (c *learnerReadClient) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	// The production async guard runs before this wrapper. The SDK async API
	// does not execute the synchronous interceptor chain.
	c.recordAttempt(ctx, address, req)
	c.Client.SendRequestAsync(ctx, address, req, callback)
}

func (c *learnerReadClient) recordAttempt(ctx context.Context, address string, req *tikvrpc.Request) {
	control, _ := ctx.Value(learnerControlReadKey{}).(bool)
	if !control || (req.Type != tikvrpc.CmdGet && req.Type != tikvrpc.CmdBatchGet && req.Type != tikvrpc.CmdScan) {
		return
	}
	observation := learnerReadObservation{
		Address: address, RegionID: req.RegionId, PeerID: req.Peer.GetId(), StoreID: req.Peer.GetStoreId(),
		Role: req.Peer.GetRole(), ReplicaRead: req.ReplicaReadType, Stale: req.StaleRead, Type: req.Type,
	}
	c.mu.Lock()
	c.attempts = append(c.attempts, observation)
	c.mu.Unlock()
}

func (c *learnerReadClient) snapshot() []learnerReadObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.attempts)
}

// TestTiKVLearnerLagThroughBothEdges runs production Server/Edge processes and
// the public Go SDK around a real Raft outage of the native Learner reader.
func TestTiKVLearnerLagThroughBothEdges(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	serverA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_A"))
	serverB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_SERVER_B"))
	edgeA := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_A"))
	edgeB := fetchServer(t, requiredEnv(t, "GIZCLAW_E2E_EDGE_B"))
	topology, ready, err := tikvfixture.ReadTopology(ctx, requiredEnv(t, "GIZCLAW_E2E_TIKV_PD"))
	if err != nil || !ready {
		t.Fatalf("TiKV topology=%+v ready=%v err=%v", topology, ready, err)
	}
	shared := openSharedTiKV(t)
	observer := &learnerReadClient{Client: shared.GetTiKVClient()}
	shared.SetTiKVClient(observer)
	root, err := kv.NewTiKVWithClient(shared, kv.TiKVReadOptions{Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := kv.NewTiKVWithClient(shared, kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerRecords := kv.Prefixed(root, kv.Key{"peers", "records"})
	assignments := kv.Prefixed(root, kv.Key{"peers", "routes"})
	credentials := kv.Prefixed(root, kv.Key{"api-keys"})
	regionalCtx := context.WithValue(ctx, learnerControlReadKey{}, true)
	assertNativeAttempts := func(start int, required bool) {
		t.Helper()
		attempts := observer.snapshot()[start:]
		if required && len(attempts) == 0 {
			t.Fatal("native Learner read issued no data request")
		}
		for _, attempt := range attempts {
			peer, found := topology.LearnerPeers[attempt.RegionID]
			if !found || attempt.Address != "tikv-replication-proxy:20160" || attempt.PeerID != peer.ID || attempt.StoreID != peer.StoreID || attempt.Role != metapb.PeerRole_Learner || attempt.ReplicaRead != tikvkv.ReplicaReadLearner || attempt.Stale {
				t.Fatalf("ordinary read attempted nonmatching/Leader/stale data request: %+v", attempt)
			}
		}
	}

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
	deletePeerA := connectAndServe(t, deleteKeyA, edgeA, serverA.PublicKey, "lag-delete-owner-a")
	t.Cleanup(func() { _ = deletePeerA.Close() })
	registerSocialPeer(t, ctx, deletePeerA, serverA, "GIZCLAW_TEST_REGISTRATION_TOKEN_A")
	deletePeerB := connectAndServe(t, deleteKeyB, edgeB, serverB.PublicKey, "lag-delete-owner-b")
	t.Cleanup(func() { _ = deletePeerB.Close() })
	registerSocialPeer(t, ctx, deletePeerB, serverB, "GIZCLAW_TEST_REGISTRATION_TOKEN_B")
	start := len(observer.snapshot())
	waitLagRecords(t, regionalCtx, peerRecords,
		kv.Key{"by-pubkey", revokeKey.Public.String()}, kv.Key{"by-pubkey", deleteKeyA.Public.String()}, kv.Key{"by-pubkey", deleteKeyB.Public.String()})
	assertNativeAttempts(start, true)
	for _, client := range []*gizcli.Client{revokePeer, deletePeerA, deletePeerB} {
		waitLagDevice(t, ctx, client)
	}
	revoked, err := revokePeer.CreateAPIKey(ctx, "lag-key-create", rpcapi.APIKeyCreateRequest{DisplayName: "lag-revoke"})
	if err != nil {
		t.Fatal(err)
	}
	deletedCredentialA, err := deletePeerA.CreateAPIKey(ctx, "lag-delete-key-a", rpcapi.APIKeyCreateRequest{DisplayName: "retire-a"})
	if err != nil {
		t.Fatal(err)
	}
	deletedCredentialB, err := deletePeerB.CreateAPIKey(ctx, "lag-delete-key-b", rpcapi.APIKeyCreateRequest{DisplayName: "retire-b"})
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Value == nil || deletedCredentialA.Value == nil || deletedCredentialB.Value == nil || revoked.APIKey == "" || deletedCredentialA.APIKey == "" || deletedCredentialB.APIKey == "" {
		t.Fatal("SDK key creation returned no credential")
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
	waitLagRecords(t, regionalCtx, credentials,
		kv.Key{"secrets", revoked.APIKey}, kv.Key{"secrets", deletedCredentialA.APIKey}, kv.Key{"secrets", deletedCredentialB.APIKey})
	for _, endpoint := range edges {
		for _, secret := range []string{revoked.APIKey, deletedCredentialA.APIKey, deletedCredentialB.APIKey} {
			waitLagHTTPReady(t, ctx, endpoint, secret)
		}
	}

	// Native reads need replication synchronization. Establish ownership and
	// roaming while healthy, rather than demanding admission during the outage.
	t.Run("first-assignments-and-cross-edge-roaming", func(t *testing.T) {
		for _, owner := range []struct {
			first, second  int
			server         giznet.PublicKey
			state, foreign string
		}{
			{0, 1, serverA.PublicKey, states[0], states[1]},
			{1, 0, serverB.PublicKey, states[1], states[0]},
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
			waitLagRecords(t, regionalCtx, assignments, kv.Key{"by-peer", key.Public.String()})
			client = connectAndServe(t, key, transports[owner.second], owner.server, "lag-first-claim-other-edge")
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			assertPeerRunAbsent(t, owner.foreign, key.Public)
			assertLagAssignment(t, ctx, kv.WithStrongReads(assignments), key.Public, owner.server)
			seededKey, previous := deleteKeyA, deletePeerA
			if owner.first == 1 {
				seededKey, previous = deleteKeyB, deletePeerB
			}
			waitLagRecords(t, regionalCtx, assignments, kv.Key{"by-peer", seededKey.Public.String()})
			if err := previous.Close(); err != nil {
				t.Fatal(err)
			}
			client = connectAndServe(t, seededKey, transports[owner.second], owner.server, "lag-cross-edge-seeded-owner")
			if err := client.SendBatteryTelemetry(47, false); err != nil {
				t.Fatal(err)
			}
			waitPeerRunStatus(t, owner.state, seededKey.Public, 47)
			assertPeerRunAbsent(t, owner.foreign, seededKey.Public)
			if owner.first == 0 {
				deletePeerA = client
			} else {
				deletePeerB = client
			}
		}
	})
	if t.Failed() {
		return
	}

	// This fault tests the KV reader, not session survival during unavailable
	// storage. Re-establish the same identities after native reads recover.
	for _, client := range []*gizcli.Client{revokePeer, deletePeerA, deletePeerB} {
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Separate backend instances keep this diagnostic reader free of the
	// writer's local postimages, so its requests must reach the actual Learner.
	probeKey := kv.Key{"e2e-native-lag", rand.Text()}
	if err := writer.Set(ctx, probeKey, []byte("seed")); err != nil {
		t.Fatal(err)
	}
	waitLagValue(t, regionalCtx, root, probeKey, []byte("seed"))
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
	if err := writer.Set(ctx, probeKey, []byte("committed-while-paused")); err != nil {
		t.Fatalf("primary write during paused Raft: %v", err)
	}
	// Cover the acknowledged write with the real cached PD timestamp. This is
	// a timestamp barrier, not a safe-ts probe or a fixed historical delay.
	barrier, err := shared.GetOracle().GetTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
	if err != nil {
		t.Fatal(err)
	}
	waitLagCachedTimestamp(t, ctx, shared, barrier)
	for i, control := range []struct {
		store kv.Store
		key   kv.Key
	}{
		{root, probeKey},
		{peerRecords, kv.Key{"by-pubkey", deleteKeyA.Public.String()}},
		{credentials, kv.Key{"secrets", revoked.APIKey}},
	} {
		start := len(observer.snapshot())
		callCtx, stop := context.WithTimeout(regionalCtx, 2*time.Second)
		_, err := control.store.Get(callCtx, control.key)
		stop()
		if err == nil || errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("paused native Learner returned success/absence for %v: %v", control.key, err)
		}
		// After the first error, the SDK may reject an unhealthy Learner before
		// dispatch. No later call may send data to the Leader instead.
		assertNativeAttempts(start, i == 0)
		start = len(observer.snapshot())
		value, err := control.store.Get(kv.WithPrimaryReads(regionalCtx), control.key)
		if err != nil {
			t.Fatalf("explicit primary control during paused Raft: %v", err)
		}
		if i == 0 && string(value) != "committed-while-paused" {
			t.Fatalf("primary lost acknowledged write: %q", value)
		}
		attempts := observer.snapshot()[start:]
		if len(attempts) == 0 {
			t.Fatal("explicit primary control issued no data request")
		}
		for _, attempt := range attempts {
			if attempt.StoreID != topology.CenterStoreID || attempt.Role == metapb.PeerRole_Learner || attempt.Stale || attempt.ReplicaRead != tikvkv.ReplicaReadLeader {
				t.Fatalf("explicit primary control escaped center voter: %+v", attempt)
			}
		}
	}
	if err := tikvfixture.HTTP(ctx, http.MethodGet, proxy+"/status", nil, &gate); err != nil || !gate.Frozen || gate.Blocked == 0 {
		t.Fatalf("Raft fault did not persist: %+v %v", gate, err)
	}
	t.Logf("real Raft outage verified: blocked=%d; native Learner errors, no Leader data fallback, primary reads/writes healthy", gate.Blocked)
	if err := tikvfixture.HTTP(ctx, http.MethodPost, proxy+"/resume", nil, nil); err != nil {
		t.Fatal(err)
	}
	start = len(observer.snapshot())
	waitLagValue(t, regionalCtx, root, probeKey, []byte("committed-while-paused"))
	assertNativeAttempts(start, true)
	for _, endpoint := range edges {
		for _, secret := range []string{revoked.APIKey, deletedCredentialA.APIKey, deletedCredentialB.APIKey} {
			waitLagHTTPReady(t, ctx, endpoint, secret)
		}
	}
	revokePeer = connectAndServe(t, revokeKey, edgeB, serverB.PublicKey, "lag-revoke-owner-after-recovery")
	deletePeerA = connectAndServe(t, deleteKeyA, edgeA, serverA.PublicKey, "lag-delete-owner-a-after-recovery")
	deletePeerB = connectAndServe(t, deleteKeyB, edgeB, serverB.PublicKey, "lag-delete-owner-b-after-recovery")
	for _, client := range []*gizcli.Client{revokePeer, deletePeerA, deletePeerB} {
		waitLagDevice(t, ctx, client)
	}

	t.Run("api-key-revocation", func(t *testing.T) {
		if _, err := revokePeer.RevokeAPIKey(ctx, "lag-revoke", rpcapi.APIKeyRevokeRequest{Name: revoked.Value.Name}); err != nil {
			t.Fatal(err)
		}
		if _, err := credentials.Get(kv.WithPrimaryReads(ctx), kv.Key{"secrets", revoked.APIKey}); !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("primary retained revoked credential: %v", err)
		}
		waitLagAbsent(t, regionalCtx, credentials, kv.Key{"secrets", revoked.APIKey})
		for _, endpoint := range edges {
			waitLagHTTPStatus(t, ctx, endpoint, revoked.APIKey, http.StatusUnauthorized)
		}
		if _, err := revokePeer.Ping(ctx, "lag-revoke-control"); err != nil {
			t.Fatalf("key revocation lost owner transport: %v", err)
		}
	})
	t.Run("peer-deletion-on-both-home-servers", func(t *testing.T) {
		for i, client := range []*gizcli.Client{deletePeerA, deletePeerB} {
			if _, err := client.DeletePeer(ctx, fmt.Sprintf("lag-peer-delete-%d", i), rpcapi.ServerPeerDeleteRequest{}); err != nil {
				t.Fatalf("SDK Peer delete: %v", err)
			}
			key := []*giznet.KeyPair{deleteKeyA, deleteKeyB}[i]
			// The permanent tombstone is part of the request commit, independent
			// of background discovery or completion of hourly cleanup passes.
			assertLagTombstone(t, kv.WithPrimaryReads(ctx), peerRecords, key.Public)
			pending, err := pendingdeletion.HasLocator(kv.WithPrimaryReads(ctx), peerRecords, pendingdeletion.KindPeer, key.Public.String())
			if err != nil || !pending {
				t.Fatalf("acknowledged deletion has no durable cleanup request: pending=%v err=%v", pending, err)
			}
		}
		for i, key := range []*giznet.KeyPair{deleteKeyA, deleteKeyB} {
			waitLagValue(t, regionalCtx, peerRecords, kv.Key{"by-pubkey", key.Public.String()}, []byte(`{"version":1,"state":"deleted"}`))
			home := serverA.PublicKey
			if i == 1 {
				home = serverB.PublicKey
			}
			assertLagAssignment(t, regionalCtx, assignments, key.Public, home)
			assertLagAssignment(t, ctx, kv.WithStrongReads(assignments), key.Public, home)
		}
		for _, secret := range []string{deletedCredentialA.APIKey, deletedCredentialB.APIKey} {
			for _, endpoint := range edges {
				waitLagHTTPStatus(t, ctx, endpoint, secret, http.StatusUnauthorized)
			}
		}
		for i, key := range []*giznet.KeyPair{deleteKeyA, deleteKeyB} {
			home := serverA.PublicKey
			if i == 1 {
				home = serverB.PublicKey
			}
			for _, transport := range []gizcli.ServerInfoMetadata{edgeA, edgeB} {
				connectMustFail(t, key, transport, home, "deleted Peer reconnected through an Edge")
			}
			assertLagTombstone(t, kv.WithPrimaryReads(ctx), peerRecords, key.Public)
			assertLagAssignment(t, ctx, kv.WithStrongReads(assignments), key.Public, home)
			pending, err := pendingdeletion.HasLocator(kv.WithPrimaryReads(ctx), peerRecords, pendingdeletion.KindPeer, key.Public.String())
			if err != nil || !pending {
				t.Fatalf("Peer cleanup request disappeared before hourly rescans: pending=%v err=%v", pending, err)
			}
			// At least 32 hourly executions retain this binding. Workspace and
			// credential cleanup can start earlier; task completion is not expected.
			assertLagSQLCount(t, states[i], "runtime_profile_owners", key.Public, 1)
			assertPeerRunAbsent(t, states[1-i], key.Public)
		}
		// A healthy owner must still connect through each Edge after the
		// deleted identities fail, so a transport outage cannot pass this gate.
		if err := revokePeer.Close(); err != nil {
			t.Fatal(err)
		}
		for _, transport := range []gizcli.ServerInfoMetadata{edgeA, edgeB} {
			client := connectAndServe(t, revokeKey, transport, serverB.PublicKey, "lag-live-owner-after-retirement")
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func lagHTTPStatus(ctx context.Context, endpoint, secret string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+endpoint+"/gizclaw/v1/api-keys/self", nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func waitLagHTTPReady(t *testing.T, ctx context.Context, endpoint, secret string) {
	t.Helper()
	waitLagHTTPStatus(t, ctx, endpoint, secret, http.StatusOK)
}

func waitLagHTTPStatus(t *testing.T, ctx context.Context, endpoint, secret string, want int) {
	t.Helper()
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := lagHTTPStatus(readyCtx, endpoint, secret)
		if err == nil && status == want {
			return
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("Edge %s credential status=%d, want %d: last error=%v, wait=%v", endpoint, status, want, err, readyCtx.Err())
		case <-ticker.C:
		}
	}
}

func waitLagDevice(t *testing.T, ctx context.Context, client *gizcli.Client) {
	t.Helper()
	readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := client.GetServerInfo(readyCtx, "lag-peer-visible"); err == nil {
			return
		}
		select {
		case <-readyCtx.Done():
			t.Fatal("device Peer input did not become readable on its Server")
		case <-ticker.C:
		}
	}
}

func waitLagRecords(t *testing.T, ctx context.Context, store kv.Store, keys ...kv.Key) {
	t.Helper()
	readyCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		ready := true
		for _, key := range keys {
			if _, err := store.Get(readyCtx, key); errors.Is(err, kv.ErrNotFound) {
				ready = false
				break
			} else if err != nil {
				t.Fatal(err)
			}
		}
		if ready {
			return
		}
		select {
		case <-readyCtx.Done():
			t.Fatal("seed records did not become readable on the Learner")
		case <-ticker.C:
		}
	}
}

func waitLagValue(t *testing.T, ctx context.Context, store kv.Store, key kv.Key, want []byte) {
	t.Helper()
	waitLagRead(t, ctx, store, key, func(value []byte, err error) bool {
		return err == nil && bytes.Equal(value, want)
	})
}

func waitLagAbsent(t *testing.T, ctx context.Context, store kv.Store, key kv.Key) {
	t.Helper()
	waitLagRead(t, ctx, store, key, func(_ []byte, err error) bool {
		return errors.Is(err, kv.ErrNotFound)
	})
}

func waitLagRead(t *testing.T, ctx context.Context, store kv.Store, key kv.Key, ready func([]byte, error) bool) {
	t.Helper()
	readyCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, err := store.Get(readyCtx, key)
		if ready(value, err) {
			return
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("Learner record %v did not converge: value=%q err=%v wait=%v", key, value, err, readyCtx.Err())
		case <-ticker.C:
		}
	}
}

func waitLagCachedTimestamp(t *testing.T, ctx context.Context, client *tikv.KVStore, barrier uint64) {
	t.Helper()
	readyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		cached, err := client.GetOracle().GetLowResolutionTimestamp(readyCtx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
		if err != nil {
			t.Fatal(err)
		}
		if cached >= barrier {
			return
		}
		select {
		case <-readyCtx.Done():
			t.Fatalf("cached PD timestamp did not cover acknowledged write: cached=%d barrier=%d: %v", cached, barrier, readyCtx.Err())
		case <-ticker.C:
		}
	}
}

func assertLagTombstone(t *testing.T, ctx context.Context, store kv.Store, key giznet.PublicKey) {
	t.Helper()
	value, err := store.Get(ctx, kv.Key{"by-pubkey", key.String()})
	if err != nil || !bytes.Equal(value, []byte(`{"version":1,"state":"deleted"}`)) {
		t.Fatalf("acknowledged Peer deletion has no permanent tombstone: value=%q err=%v", value, err)
	}
}

func assertLagAssignment(t *testing.T, ctx context.Context, store kv.Store, key, home giznet.PublicKey) {
	t.Helper()
	data, err := store.Get(ctx, kv.Key{"by-peer", key.String()})
	if err != nil {
		t.Fatal(err)
	}
	var assignment apitypes.PeerAssignment
	if err := json.Unmarshal(data, &assignment); err != nil || assignment.PeerPublicKey != key.String() || assignment.ServerPublicKey != home.String() || assignment.Version != 1 {
		t.Fatalf("fixed home assignment changed: %+v err=%v", assignment, err)
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
