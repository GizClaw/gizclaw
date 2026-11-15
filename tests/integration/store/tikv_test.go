//go:build store_e2e

package store_e2e_test

import (
	"bytes"
	"context"
	"crypto/rand"
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
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/ai/workspace"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peer"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/runtime/peerroute"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/social"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/apikey"
	"github.com/GizClaw/gizclaw-go/pkgs/gizclaw/services/system/pendingdeletion"
	"github.com/GizClaw/gizclaw-go/pkgs/giznet"
	stores "github.com/GizClaw/gizclaw-go/pkgs/store"
	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/pingcap/kvproto/pkg/metapb"
	tikvkv "github.com/tikv/client-go/v2/kv"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/tikvrpc/interceptor"
	"github.com/tikv/client-go/v2/util/async"
)

func tiKVHTTP(ctx context.Context, method, endpoint string, input, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("test cluster HTTP status %d: %s", resp.StatusCode, detail)
	}
	if output != nil {
		return json.NewDecoder(resp.Body).Decode(output)
	}
	_, err = io.Copy(io.Discard, resp.Body)
	return err
}

func configureTiKVLearner(t *testing.T, address string) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	pdURL := "http://" + address + "/pd/api/v1"
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		var regions struct {
			Count int `json:"count"`
		}
		var stores struct {
			Count int `json:"count"`
		}
		err := tiKVHTTP(ctx, http.MethodGet, pdURL+"/regions", nil, &regions)
		if err == nil {
			err = tiKVHTTP(ctx, http.MethodGet, pdURL+"/stores", nil, &stores)
		}
		if err == nil && regions.Count > 0 && stores.Count >= 2 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("PD readiness timeout: regions=%d stores=%d err=%v", regions.Count, stores.Count, err)
		case <-ticker.C:
		}
	}
	if err := tiKVHTTP(ctx, http.MethodPost, pdURL+"/config/replicate", map[string]any{"max-replicas": 1, "enable-placement-rules": "true"}, nil); err != nil {
		t.Fatal(err)
	}
	for _, rule := range []map[string]any{
		{"group_id": "pd", "id": "default", "role": "voter", "count": 1, "label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"center"}}}},
		{"group_id": "gizclaw-e2e", "id": "regional", "role": "learner", "count": 1, "label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"local"}}}},
	} {
		if err := tiKVHTTP(ctx, http.MethodPost, pdURL+"/config/rule", rule, nil); err != nil {
			t.Fatal(err)
		}
	}
	for {
		var topology struct {
			Regions []struct {
				Leader struct {
					StoreID uint64 `json:"store_id"`
				} `json:"leader"`
				Peers []struct {
					StoreID  uint64 `json:"store_id"`
					RoleName string `json:"role_name"`
				} `json:"peers"`
			} `json:"regions"`
		}
		var storeList struct {
			Stores []struct {
				Store struct {
					ID      uint64                        `json:"id"`
					Address string                        `json:"address"`
					Labels  []struct{ Key, Value string } `json:"labels"`
				} `json:"store"`
			} `json:"stores"`
		}
		err := tiKVHTTP(ctx, http.MethodGet, pdURL+"/regions", nil, &topology)
		if err == nil {
			err = tiKVHTTP(ctx, http.MethodGet, pdURL+"/stores", nil, &storeList)
		}
		center, local := uint64(0), uint64(0)
		addresses := make(map[string]string)
		for _, store := range storeList.Stores {
			for _, label := range store.Store.Labels {
				if label.Key == "region" {
					if label.Value == "center" {
						center = store.Store.ID
						addresses["center"] = store.Store.Address
					}
					if label.Value == "local" {
						local = store.Store.ID
						addresses["local"] = store.Store.Address
					}
				}
			}
		}
		ready := err == nil && center != 0 && local != 0 && len(topology.Regions) > 0
		for _, region := range topology.Regions {
			learner := false
			for _, p := range region.Peers {
				if p.StoreID == local && p.RoleName == "Learner" {
					learner = true
				}
			}
			ready = ready && learner && region.Leader.StoreID == center
		}
		if ready {
			t.Logf("verified %d regions: center leader store=%d, regional Learner store=%d", len(topology.Regions), center, local)
			return addresses
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Learner placement readiness timeout: stores=%+v regions=%+v err=%v", storeList, topology, err)
		case <-ticker.C:
		}
	}
}

type tiKVReadObservation struct {
	Address     string
	Stale       bool
	ReplicaRead tikvkv.ReplicaReadType
	Type        tikvrpc.CmdType
	Version     uint64
	Role        metapb.PeerRole
	Keys        int
}
type observedTiKVClient struct {
	tikv.Client
	mu       sync.Mutex
	reads    []tiKVReadObservation
	attempts []tiKVReadObservation
}

func (c *observedTiKVClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	// The borrowed SDK client contains its synchronous context interceptor.
	// Append observation after the existing routing guard, immediately before
	// dispatch. Recording on wrapper entry would count rejected selections as
	// network attempts and falsely claim a Leader data fallback.
	ctx = interceptor.WithRPCInterceptor(ctx, interceptor.NewRPCInterceptor("native-test-dispatch", func(next interceptor.RPCInterceptorFunc) interceptor.RPCInterceptorFunc {
		return func(target string, request *tikvrpc.Request) (*tikvrpc.Response, error) {
			c.recordAttempt(target, request)
			return next(target, request)
		}
	}))
	resp, err := c.Client.SendRequest(ctx, address, req, timeout)
	c.record(address, req, resp, err)
	return resp, err
}

func (c *observedTiKVClient) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	c.recordAttempt(address, req)
	callback.Inject(func(resp *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		c.record(address, req, resp, err)
		return resp, err
	})
	c.Client.SendRequestAsync(ctx, address, req, callback)
}

func (c *observedTiKVClient) record(address string, req *tikvrpc.Request, resp *tikvrpc.Response, err error) {
	if err != nil {
		return
	}
	observation, ok := readObservation(address, req)
	if !ok {
		return
	}
	regionError, decodeErr := resp.GetRegionError()
	if decodeErr != nil || regionError != nil {
		return
	}
	c.mu.Lock()
	c.reads = append(c.reads, observation)
	c.mu.Unlock()
}

// Record transport attempts before dispatch, including timeouts and rejected
// native responses. Success-only evidence cannot prove absence of fallback.
func (c *observedTiKVClient) recordAttempt(address string, req *tikvrpc.Request) {
	observation, ok := readObservation(address, req)
	if !ok {
		return
	}
	c.mu.Lock()
	c.attempts = append(c.attempts, observation)
	c.mu.Unlock()
}

func readObservation(address string, req *tikvrpc.Request) (tiKVReadObservation, bool) {
	observation := tiKVReadObservation{Address: address, Stale: req.StaleRead, ReplicaRead: req.ReplicaReadType, Type: req.Type, Role: req.Peer.GetRole()}
	switch req.Type {
	case tikvrpc.CmdGet:
		observation.Version, observation.Keys = req.Get().Version, 1
	case tikvrpc.CmdBatchGet:
		observation.Version, observation.Keys = req.BatchGet().Version, len(req.BatchGet().Keys)
	case tikvrpc.CmdScan:
		observation.Version = req.Scan().Version
	default:
		return tiKVReadObservation{}, false
	}
	return observation, true
}

func (c *observedTiKVClient) attemptedReads() []tiKVReadObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.attempts)
}

func (c *observedTiKVClient) snapshot() []tiKVReadObservation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]tiKVReadObservation(nil), c.reads...)
}

func TestTiKV(t *testing.T) {
	address := requiredEnvironment(t, "GIZCLAW_TEST_TIKV_PD")
	addresses := configureTiKVLearner(t, address)
	physical, err := storage.New(map[string]storage.Config{"central": storage.TiKVConfig{PDAddresses: []string{address}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	client, _, err := physical.TiKV("central")
	if err != nil {
		t.Fatal(err)
	}
	observer := &observedTiKVClient{Client: client.GetTiKVClient()}
	client.SetTiKVClient(observer)
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	secondPhysical, err := storage.New(map[string]storage.Config{"other": storage.TiKVConfig{PDAddresses: []string{address}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = secondPhysical.Close() })
	secondClient, _, _ := secondPhysical.TiKV("other")
	secondRoot, err := kv.NewTiKVWithClient(secondClient, kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := kv.Key{"test-" + rand.Text()}
	t.Run("shared-kv-contract", func(t *testing.T) { assertSharedKV(t, kv.Prefixed(root, prefix), kv.Prefixed(secondRoot, prefix)) })
	t.Run("registry", func(t *testing.T) { testTiKVRegistry(t, physical) })
	t.Run("regional-cached-snapshot", func(t *testing.T) { testTiKVLag(t, client, observer) })
	t.Run("regional-replication-unavailable", func(t *testing.T) { testTiKVUnavailableLearner(t, client, observer, addresses) })
	t.Run("local-read-your-writes", func(t *testing.T) { testTiKVLocalWrites(t, client) })
	t.Run("presence-pages", func(t *testing.T) { testTiKVPresence(t, address, addresses) })
	t.Run("presence-transitions", func(t *testing.T) { testTiKVPresenceTransitions(t, address) })
}

func testTiKVRegistry(t *testing.T, physical *storage.Storage) {
	prefix := "registry-" + rand.Text()
	registry, err := stores.New(map[string]stores.Config{"a": {Kind: stores.KindKeyValue, Storage: "central", Prefix: prefix + "/a"}, "b": {Kind: stores.KindKeyValue, Storage: "central", Prefix: prefix + "/b"}}, physical)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := registry.KV("a")
	b, _ := registry.KV("b")
	if _, _, ok := kv.SharedAtomicStore(kv.WithStrongReads(a), b); !ok {
		t.Fatal("strong view lost shared atomic root")
	}
	if err := a.Set(t.Context(), kv.Key{"record"}, []byte("a")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Get(t.Context(), kv.Key{"record"}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("prefix isolation: %v", err)
	}
	if err := registry.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Get(t.Context(), kv.Key{"record"}); err != nil {
		t.Fatalf("logical close closed borrowed client: %v", err)
	}
	if err := a.Delete(t.Context(), kv.Key{"record"}); err != nil {
		t.Fatal(err)
	}
	for _, configs := range []map[string]stores.Config{
		{"a": {Kind: stores.KindKeyValue, Storage: "central"}},
		{"a": {Kind: stores.KindKeyValue, Storage: "central", Prefix: prefix}, "b": {Kind: stores.KindKeyValue, Storage: "central", Prefix: prefix + "/nested"}},
	} {
		if _, err := stores.New(configs, physical); err == nil {
			t.Fatal("accepted unsafe TiKV prefixes")
		}
	}
}

// Unlike the cached-snapshot service test below, this test really interrupts
// Raft replication. Ordinary reads must fail rather than change data authority.
func testTiKVUnavailableLearner(t *testing.T, client *tikv.KVStore, observer *observedTiKVClient, addresses map[string]string) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	options := kv.TiKVReadOptions{Labels: map[string]string{"region": "local"}}
	readerRoot, err := kv.NewTiKVWithClient(client, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	writerRoot, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := kv.Key{"native-unavailable-" + rand.Text()}
	reader, writer := kv.Prefixed(readerRoot, prefix), kv.Prefixed(writerRoot, prefix)
	key, secondKey, set := kv.Key{"record"}, kv.Key{"second-record"}, kv.Key{"members"}
	if err := writer.BatchSet(ctx, []kv.Entry{{Key: key, Value: []byte("seed")}, {Key: secondKey, Value: []byte("second-seed")}}); err != nil {
		t.Fatal(err)
	}
	if err := writer.AddMembers(ctx, set, "member"); err != nil {
		t.Fatal(err)
	}
	// Require an actual healthy native read first. This fails, rather than
	// enabling stale reads or relaxing routing, if TiKV rejects the protocol.
	deadline, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, err := reader.Get(deadline, key)
		if err == nil && string(value) == "seed" {
			break
		}
		if err != nil && !errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("healthy native Learner Get failed: %v", err)
		}
		select {
		case <-deadline.Done():
			t.Fatalf("healthy native Learner did not observe seed: %q, %v", value, err)
		case <-ticker.C:
		}
	}
	operations := []struct {
		name string
		read func(context.Context, kv.Store) error
	}{
		{"Get", func(ctx context.Context, store kv.Store) error { _, err := store.Get(ctx, key); return err }},
		{"BatchGet", func(ctx context.Context, store kv.Store) error {
			results := store.BatchGet(ctx, []kv.Key{key, secondKey})
			if results[0].Err != nil {
				return results[0].Err
			}
			return results[1].Err
		}},
		{"Scan", func(ctx context.Context, store kv.Store) error { _, err := store.ListMembers(ctx, set); return err }},
	}
	assertNativeAttempts := func(start int, name string, required bool) {
		t.Helper()
		attempts := observer.attemptedReads()[start:]
		if required && len(attempts) == 0 {
			t.Fatalf("%s issued no native data request", name)
		}
		for _, attempt := range attempts {
			if attempt.Address != addresses["local"] || attempt.Role != metapb.PeerRole_Learner || attempt.ReplicaRead != tikvkv.ReplicaReadLearner || attempt.Stale {
				t.Fatalf("%s attempted a nonmatching/Leader/stale data request: %+v", name, attempt)
			}
		}
		t.Logf("%s: %d native Learner data attempts, including failed responses; no Leader data attempts", name, len(attempts))
	}
	for _, operation := range operations {
		start := len(observer.attemptedReads())
		if err := operation.read(ctx, reader); err != nil {
			t.Fatalf("healthy native Learner %s failed: %v", operation.name, err)
		}
		assertNativeAttempts(start, "healthy "+operation.name, true)
		expectedCommand := map[string]tikvrpc.CmdType{"Get": tikvrpc.CmdGet, "BatchGet": tikvrpc.CmdBatchGet, "Scan": tikvrpc.CmdScan}[operation.name]
		if !slices.ContainsFunc(observer.attemptedReads()[start:], func(attempt tiKVReadObservation) bool { return attempt.Type == expectedCommand }) {
			t.Fatalf("healthy %s never dispatched native %s (an optimized Get does not exercise BatchGet)", operation.name, expectedCommand)
		}
	}
	t.Run("wrong-labels-do-not-cancel-caller-or-poison-primary", func(t *testing.T) {
		wrongRoot, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{Labels: map[string]string{"region": "no-such-region"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		wrong := kv.Prefixed(wrongRoot, prefix)
		for _, operation := range operations {
			caller, callerCancel := context.WithTimeout(ctx, 5*time.Second)
			start := len(observer.attemptedReads())
			err := operation.read(caller, wrong)
			callerErr := caller.Err()
			if err == nil || errors.Is(err, kv.ErrNotFound) || callerErr != nil {
				callerCancel()
				t.Fatalf("wrong-label %s = %v, caller = %v", operation.name, err, callerErr)
			}
			if attempts := observer.attemptedReads()[start:]; len(attempts) != 0 {
				callerCancel()
				t.Fatalf("wrong-label %s reached a nonmatching data peer: %+v", operation.name, attempts)
			}
			if err := operation.read(kv.WithPrimaryReads(caller), wrong); err != nil {
				callerCancel()
				t.Fatalf("wrong-label rejection poisoned explicit primary %s: %v", operation.name, err)
			}
			callerCancel()
		}
	})

	proxyURL := requiredEnvironment(t, "GIZCLAW_TEST_TIKV_PROXY")
	var gate struct {
		Frozen  bool `json:"frozen"`
		Blocked int  `json:"blocked"`
	}
	if err := tiKVHTTP(ctx, http.MethodPost, proxyURL+"/freeze", nil, &gate); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := tiKVHTTP(cleanup, http.MethodPost, proxyURL+"/resume", nil, nil); err != nil {
			t.Error(err)
		}
	}()
	if !gate.Frozen || gate.Blocked == 0 {
		t.Fatalf("Raft replication was not interrupted: %+v", gate)
	}
	if err := writer.Set(ctx, key, []byte("committed-while-paused")); err != nil {
		t.Fatalf("leader write during paused replication: %v", err)
	}
	// Wait for the real cached PD timestamp to cover the acknowledged write.
	// This prevents an older cached snapshot from legitimately returning seed.
	barrier, err := client.GetOracle().GetTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
	if err != nil {
		t.Fatal(err)
	}
	for {
		cached, err := client.GetOracle().GetLowResolutionTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
		if err != nil {
			t.Fatal(err)
		}
		if cached >= barrier {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	for _, operation := range operations {
		start := len(observer.attemptedReads())
		callCtx, callCancel := context.WithTimeout(ctx, 2*time.Second)
		err := operation.read(callCtx, reader)
		callCancel()
		if err == nil || errors.Is(err, kv.ErrNotFound) {
			t.Fatalf("paused replication %s returned a successful/absent result: %v", operation.name, err)
		}
		assertNativeAttempts(start, "paused replication "+operation.name, operation.name == "Get")
		// The SDK may already classify the Learner as unhealthy after Get;
		// later calls may reject routing before transport, but never use Leader.
		if err := operation.read(kv.WithPrimaryReads(ctx), reader); err != nil {
			t.Fatalf("explicit primary %s failed during paused replication: %v", operation.name, err)
		}
	}
	if got, err := reader.Get(kv.WithPrimaryReads(ctx), key); err != nil || string(got) != "committed-while-paused" {
		t.Fatalf("explicit primary did not retain acknowledged write: %q, %v", got, err)
	}
	if err := tiKVHTTP(ctx, http.MethodGet, proxyURL+"/status", nil, &gate); err != nil || !gate.Frozen {
		t.Fatalf("Raft resumed during unavailable-Learner checks: %+v, %v", gate, err)
	}
	t.Logf("real Raft replication paused: blocked=%d; native Learner errors propagated while primary reads and writes remained available", gate.Blocked)
}

func testTiKVLag(t *testing.T, client *tikv.KVStore, observer *observedTiKVClient) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Separate backend instances model remote writes. Local writes now have a
	// postimage overlay; another instance must still retain historical absence.
	writerRoot, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := kv.Key{"lag-" + rand.Text()}
	base, writerBase := kv.Prefixed(root, prefix), kv.Prefixed(writerRoot, prefix)
	peerStore := kv.Prefixed(base, kv.Key{"records"})
	routeStore := kv.Prefixed(base, kv.Key{"routes"})
	keyStore := kv.Prefixed(base, kv.Key{"keys"})
	writerPeers := kv.Prefixed(writerBase, kv.Key{"records"})
	writerRoutes := kv.Prefixed(writerBase, kv.Key{"routes"})
	writerKeys := kv.Prefixed(writerBase, kv.Key{"keys"})
	centralPeers := &peer.Server{Store: writerPeers, ServerPublicKey: giznet.PublicKey{71}}
	regionalPeers := &peer.Server{Store: peerStore}
	ownerKey, newKey, blockedKey := giznet.PublicKey{61}, giznet.PublicKey{62}, giznet.PublicKey{63}
	for _, key := range []giznet.PublicKey{ownerKey, newKey, blockedKey} {
		if _, err := centralPeers.SavePeer(ctx, apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive}); err != nil {
			t.Fatal(err)
		}
	}
	owner := &peerroute.Server{Store: writerRoutes, Peers: regionalPeers, ServerPublicKey: giznet.PublicKey{71}, ServerEndpoint: "https://server-a.example"}
	foreign := &peerroute.Server{Store: routeStore, Peers: regionalPeers, ServerPublicKey: giznet.PublicKey{72}, ServerEndpoint: "https://server-b.example"}
	// Setup follows acknowledged Peer writes before the reader can see them.
	initial, err := owner.Assign(kv.WithPrimaryReads(ctx), ownerKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := apikey.NewServer(writerKeys)
	regionalKeys := apikey.NewServer(keyStore)
	toRevoke, err := keys.Create(ctx, ownerKey.String(), "revoke", false)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := keys.Create(ctx, ownerKey.String(), "retire with peer", false)
	if err != nil {
		t.Fatal(err)
	}
	setKey, orderedKey := kv.Key{"members"}, kv.Key{"ordered-members"}
	if err := writerBase.AddMembers(ctx, setKey, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := writerBase.ApplyMutation(ctx, kv.Mutation{AddOrderedMembers: []kv.SetMembers{{Key: orderedKey, Members: []string{"a", "b", "c"}}}}); err != nil {
		t.Fatal(err)
	}
	marker := kv.Key{"seed-ready"}
	if err := writerBase.Set(ctx, marker, []byte("ready")); err != nil {
		t.Fatal(err)
	}
	t.Log("waiting for native Learner reads to observe seeded writes before freezing only the cached PD timestamp")
	seedCtx, seedCancel := context.WithTimeout(ctx, 10*time.Second)
	seedTicker := time.NewTicker(100 * time.Millisecond)
	for {
		value, err := base.Get(seedCtx, marker)
		if err == nil && bytes.Equal(value, []byte("ready")) {
			break
		}
		select {
		case <-seedCtx.Done():
			t.Fatalf("seed marker not visible to historical reader: %s %v", value, err)
		case <-seedTicker.C:
		}
	}
	seedTicker.Stop()
	seedCancel()
	clock := freezeTiKVReadClock(t, client)
	frozenAt := time.Now()
	readStart := len(observer.snapshot())
	assertLearnerReads := func(start int, operation string) {
		t.Helper()
		reads := observer.snapshot()[start:]
		if len(reads) == 0 {
			t.Fatalf("%s issued no observed reads", operation)
		}
		for _, r := range reads {
			// The exact cached snapshot remains fixed while native replication
			// and ReadIndex synchronization stay healthy.
			if r.Address != "gate:20160" || r.Role != metapb.PeerRole_Learner || r.Stale || r.ReplicaRead != tikvkv.ReplicaReadLearner || r.Version != clock.cached {
				t.Fatalf("%s escaped the historical Learner: %+v", operation, r)
			}
		}
	}
	assertFallback := func(start int, operation string) {
		t.Helper()
		reads := observer.snapshot()[start:]
		if len(reads) < 2 || reads[0].Address != "gate:20160" || reads[0].ReplicaRead != tikvkv.ReplicaReadLearner {
			t.Fatalf("%s did not start on Learner: %+v", operation, reads)
		}
		last := reads[len(reads)-1]
		if last.Address != "leader:20160" || last.ReplicaRead != tikvkv.ReplicaReadLeader || last.Type != tikvrpc.CmdGet || last.Stale {
			t.Fatalf("%s did not reload from primary: %+v", operation, reads)
		}
	}
	oldAssignment, err := routeStore.Get(ctx, kv.Key{"by-peer", ownerKey.String()})
	if err != nil {
		t.Fatal(err)
	}
	oldPeer, err := peerStore.Get(ctx, kv.Key{"by-pubkey", ownerKey.String()})
	if err != nil {
		t.Fatal(err)
	}
	oldSecret, err := keyStore.Get(ctx, kv.Key{"secrets", toRevoke.Secret})
	if err != nil {
		t.Fatal(err)
	}
	owner.ServerEndpoint = "https://server-a-new.example"
	updated, err := owner.Assign(ctx, ownerKey, &initial.Version)
	if err != nil || updated.Version != 2 {
		t.Fatalf("refresh: %+v %v", updated, err)
	}
	primaryCtx := kv.WithPrimaryReads(ctx)
	primaryAssignment, err := routeStore.Get(primaryCtx, kv.Key{"by-peer", ownerKey.String()})
	if err != nil || bytes.Equal(primaryAssignment, oldAssignment) {
		t.Fatalf("primary-read context used the frozen Learner: %v", err)
	}
	start := len(observer.snapshot())
	if got, err := regionalPeers.LoadPeer(ctx, ownerKey); err != nil || got.PublicKey != ownerKey.String() || got.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("ordinary Peer read: %+v %v", got, err)
	}
	assertLearnerReads(start, "peer.LoadPeer")
	for _, lookup := range []struct {
		name string
		read func(context.Context, giznet.PublicKey) (apitypes.PeerAssignment, error)
	}{{"route.Lookup", foreign.Lookup}, {"route.Resolve", foreign.Resolve}} {
		start = len(observer.snapshot())
		if got, err := lookup.read(ctx, ownerKey); err != nil || got.Version != initial.Version || got.ServerEndpoint != initial.ServerEndpoint || got.ServerPublicKey != initial.ServerPublicKey {
			t.Fatalf("%s did not retain seeded assignment: %+v %v", lookup.name, got, err)
		}
		assertLearnerReads(start, lookup.name)
	}
	if _, err := foreign.Assign(ctx, ownerKey, nil); !errors.Is(err, peerroute.ErrVersionConflict) {
		t.Fatalf("foreign claim: %v", err)
	}
	start = len(observer.snapshot())
	staleOwner := &peerroute.Server{Store: routeStore, Peers: regionalPeers, ServerPublicKey: owner.ServerPublicKey, ServerEndpoint: owner.ServerEndpoint}
	if _, err := staleOwner.Assign(ctx, ownerKey, &initial.Version); !errors.Is(err, peerroute.ErrVersionConflict) {
		t.Fatalf("old expected version accepted: %v", err)
	}
	assertFallback(start, "stale expected_version conflict")
	if _, err := owner.Assign(ctx, newKey, nil); err != nil {
		t.Fatal(err)
	}
	start = len(observer.snapshot())
	if _, err := foreign.Resolve(ctx, newKey); !errors.Is(err, peerroute.ErrAssignmentNotFound) {
		t.Fatalf("ordinary resolve must retain the missing assignment: %v", err)
	}
	assertLearnerReads(start, "missing first assignment")
	if got, err := foreign.Resolve(primaryCtx, newKey); err != nil || got.Version != 1 || got.ServerPublicKey != owner.ServerPublicKey.String() {
		t.Fatalf("primary assignment diagnostic: %+v %v", got, err)
	}
	if err := keys.RevokeOwner(ctx, ownerKey.String(), toRevoke.Key.Name); err != nil {
		t.Fatal(err)
	}
	start = len(observer.snapshot())
	if principal, err := apikey.NewServer(keyStore).Authenticate(ctx, toRevoke.Secret); err != nil || principal.Key.Name != toRevoke.Key.Name || principal.Key.Owner != ownerKey.String() {
		t.Fatalf("ordinary authentication did not retain seeded key: %+v %v", principal, err)
	}
	assertLearnerReads(start, "API Authenticate")
	if _, err := keys.Authenticate(primaryCtx, toRevoke.Secret); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("primary retained revoked credential: %v", err)
	}
	if err := writerBase.RemoveMembers(ctx, setKey, "a"); err != nil {
		t.Fatal(err)
	}
	if err := writerBase.AddMembers(ctx, setKey, "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := writerBase.ApplyMutation(ctx, kv.Mutation{
		RemoveOrderedMembers: []kv.SetMembers{{Key: orderedKey, Members: []string{"b"}}},
		AddOrderedMembers:    []kv.SetMembers{{Key: orderedKey, Members: []string{"d"}}},
	}); err != nil {
		t.Fatal(err)
	}
	after := "a"
	query := kv.OrderedRange{After: &after, Limit: 2}
	start = len(observer.snapshot())
	if got, err := base.ListMembers(ctx, setKey); err != nil || !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("ordinary collection did not retain seeded members: %v %v", got, err)
	}
	if got, err := base.RangeOrderedMembers(ctx, orderedKey, query); err != nil || !slices.Equal(got, []string{"b", "c"}) {
		t.Fatalf("ordinary range did not retain seeded members: %v %v", got, err)
	}
	assertLearnerReads(start, "collection and member range")
	scans := 0
	for _, r := range observer.snapshot()[start:] {
		if r.Type == tikvrpc.CmdScan {
			scans++
			if r.Stale {
				t.Fatalf("native Learner scan unexpectedly used StaleRead: %+v", r)
			}
		}
	}
	if scans < 2 {
		t.Fatalf("missing collection/member-range Scan proof: %d", scans)
	}
	if err := centralPeers.DeleteSelf(ctx, ownerKey); err != nil {
		t.Fatal(err)
	}
	if err := regionalPeers.EnsureAvailable(primaryCtx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("primary deletion fence: %v", err)
	}
	if err := regionalPeers.EnsureAvailable(ctx, ownerKey); err != nil {
		t.Fatalf("ordinary availability did not retain seeded Peer: %v", err)
	}
	if err := pendingPeerRetirement(t, ctx, centralPeers, writerPeers, keys, owner); err != nil {
		t.Fatal(err)
	}
	if got, err := peerStore.Get(primaryCtx, kv.Key{"by-pubkey", ownerKey.String()}); err != nil || !bytes.Equal(got, []byte(`{"version":1,"state":"deleted"}`)) {
		t.Fatalf("primary permanent tombstone: %s %v", got, err)
	}
	if _, err := regionalPeers.EnsureConnectedPeer(primaryCtx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("primary tombstoned Peer reconnected: %v", err)
	}
	if _, err := keys.Authenticate(primaryCtx, retained.Secret); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("primary retained retired owner key: %v", err)
	}
	if _, err := keys.Create(ctx, ownerKey.String(), "resurrection", false); !errors.Is(err, apikey.ErrOwnerRetired) {
		t.Fatalf("retired owner created key: %v", err)
	}
	blockedRecord, err := centralPeers.LoadPeer(ctx, blockedKey)
	if err != nil {
		t.Fatal(err)
	}
	blockedRecord.Status = apitypes.PeerRegistrationStatusBlocked
	if _, err := centralPeers.SavePeer(ctx, blockedRecord); err != nil {
		t.Fatal(err)
	}
	if _, err := regionalPeers.EnsureConnectedPeer(primaryCtx, blockedKey); !errors.Is(err, peer.ErrPeerBlocked) {
		t.Fatalf("primary blocked Peer reconnected: %v", err)
	}
	// Ordinary services must still return the exact old records. A positive
	// historical lookup alone does not authorize switching to the primary.
	start = len(observer.snapshot())
	if got, err := regionalPeers.LoadPeer(ctx, ownerKey); err != nil || got.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("ordinary deleted Peer no longer active in snapshot: %+v %v", got, err)
	}
	if got, err := foreign.Resolve(ctx, ownerKey); err != nil || got.Version != initial.Version || got.ServerEndpoint != initial.ServerEndpoint {
		t.Fatalf("ordinary deleted Peer route did not retain snapshot: %+v %v", got, err)
	}
	if got, err := regionalKeys.Authenticate(ctx, retained.Secret); err != nil || got.Key.Name != retained.Key.Name {
		t.Fatalf("ordinary retired owner key did not retain snapshot: %+v %v", got, err)
	}
	if got, err := regionalPeers.LoadPeer(ctx, blockedKey); err != nil || got.Status != apitypes.PeerRegistrationStatusActive {
		t.Fatalf("ordinary blocked Peer did not retain snapshot: %+v %v", got, err)
	}
	assertLearnerReads(start, "deleted, retired and blocked ordinary reads")
	if got, err := routeStore.Get(ctx, kv.Key{"by-peer", ownerKey.String()}); err != nil || !bytes.Equal(got, oldAssignment) {
		t.Fatalf("Learner assignment control changed: %v", err)
	}
	if _, err := routeStore.Get(ctx, kv.Key{"by-peer", newKey.String()}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Learner saw new assignment: %v", err)
	}
	if got, err := peerStore.Get(ctx, kv.Key{"by-pubkey", ownerKey.String()}); err != nil || !bytes.Equal(got, oldPeer) {
		t.Fatalf("Learner peer control changed: %v", err)
	}
	if got, err := keyStore.Get(ctx, kv.Key{"secrets", toRevoke.Secret}); err != nil || !bytes.Equal(got, oldSecret) {
		t.Fatalf("Learner revoked credential control changed: %v", err)
	}
	if time.Since(frozenAt) > 25*time.Second {
		t.Fatal("cached snapshot assertions exceeded their test budget")
	}
	staleReads, strongReads := 0, 0
	for _, r := range observer.snapshot()[readStart:] {
		if r.Stale || r.ReplicaRead == tikvkv.ReplicaReadLearner {
			staleReads++
			if r.Address != "gate:20160" || r.Role != metapb.PeerRole_Learner || r.Stale || r.Version != clock.cached {
				t.Fatalf("historical read escaped regional Learner: %+v", r)
			}
		} else {
			strongReads++
			if r.Address != "leader:20160" {
				t.Fatalf("primary read used regional Learner: %+v", r)
			}
		}
	}
	if staleReads < 6 || strongReads == 0 {
		t.Fatalf("missing observed read paths: stale=%d primary=%d", staleReads, strongReads)
	}
	t.Logf("test-only frozen PD snapshot %d with healthy Raft: %d native Learner reads, %d primary reads, collection scans and explicit conflict recovery in %s", clock.cached, staleReads, strongReads, time.Since(frozenAt).Round(time.Millisecond))
	clock.frozen.Store(false)
	// Let the real cached PD timestamp advance beyond the remote writes.
	convergenceCtx, convergenceCancel := context.WithTimeout(ctx, 10*time.Second)
	defer convergenceCancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		value, peerErr := peerStore.Get(convergenceCtx, kv.Key{"by-pubkey", ownerKey.String()})
		_, revokedErr := keyStore.Get(convergenceCtx, kv.Key{"secrets", toRevoke.Secret})
		_, retiredErr := keyStore.Get(convergenceCtx, kv.Key{"secrets", retained.Secret})
		members, membersErr := base.ListMembers(convergenceCtx, setKey)
		ordered, orderedErr := base.RangeOrderedMembers(convergenceCtx, orderedKey, query)
		blocked, blockedErr := regionalPeers.LoadPeer(convergenceCtx, blockedKey)
		if peerErr == nil && bytes.Equal(value, []byte(`{"version":1,"state":"deleted"}`)) && errors.Is(revokedErr, kv.ErrNotFound) && errors.Is(retiredErr, kv.ErrNotFound) && membersErr == nil && slices.Equal(members, []string{"b", "c"}) && orderedErr == nil && slices.Equal(ordered, []string{"c", "d"}) && blockedErr == nil && blocked.Status == apitypes.PeerRegistrationStatusBlocked {
			break
		}
		select {
		case <-convergenceCtx.Done():
			t.Fatalf("Learner did not converge: peer=%s/%v revoked=%v retired=%v members=%v/%v ordered=%v/%v blocked=%+v/%v", value, peerErr, revokedErr, retiredErr, members, membersErr, ordered, orderedErr, blocked, blockedErr)
		case <-ticker.C:
		}
	}
	if _, err := regionalPeers.EnsureConnectedPeer(ctx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("converged deleted Peer reconnected: %v", err)
	}
	if _, err := foreign.Resolve(ctx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("converged Edge routed deleted Peer: %v", err)
	}
	if _, err := keys.Authenticate(ctx, toRevoke.Secret); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("converged revoked credential authenticated: %v", err)
	}
	if _, err := keys.Authenticate(ctx, retained.Secret); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("converged retired owner key authenticated: %v", err)
	}
	if _, err := regionalPeers.EnsureConnectedPeer(ctx, blockedKey); !errors.Is(err, peer.ErrPeerBlocked) {
		t.Fatalf("converged blocked Peer reconnected: %v", err)
	}
	if got, err := foreign.Resolve(ctx, newKey); err != nil || got.Version != 1 || got.ServerPublicKey != owner.ServerPublicKey.String() {
		t.Fatalf("converged first assignment: %+v %v", got, err)
	}
}

// Empty child-resource adapters model a Peer with no Social/Workspace/memory
// resources. The durable deletion workflow and API key retirement remain real.
type tiKVRetirement struct{ owner string }

func (r tiKVRetirement) SnapshotPeerSocial(context.Context, string) (social.PeerSnapshot, error) {
	return social.PeerSnapshot{PublicKey: r.owner}, nil
}
func (r tiKVRetirement) PreparePeerSocialRetirement(ctx context.Context, owner string) (social.PeerSnapshot, error) {
	return r.SnapshotPeerSocial(ctx, owner)
}
func (tiKVRetirement) RetirePeerSocial(context.Context, social.PeerSnapshot) (social.PeerRetirementResult, error) {
	return social.PeerRetirementResult{}, nil
}
func (r tiKVRetirement) SnapshotPeerWorkspaces(context.Context, string) (workspace.PeerRetirementSnapshot, error) {
	return workspace.PeerRetirementSnapshot{PublicKey: r.owner}, nil
}
func (r tiKVRetirement) PreparePeerWorkspacesRetirement(ctx context.Context, owner string) (workspace.PeerRetirementSnapshot, error) {
	return r.SnapshotPeerWorkspaces(ctx, owner)
}
func (tiKVRetirement) RetirePeerWorkspaces(context.Context, workspace.PeerRetirementSnapshot) ([]string, error) {
	return nil, nil
}
func (tiKVRetirement) DeleteOwnerProfileBinding(context.Context, string) error { return nil }
func (tiKVRetirement) PurgePeerMemory(context.Context, string) error           { return nil }
func (tiKVRetirement) PeerMemoryAbsent(context.Context, string) (bool, error)  { return true, nil }
func (tiKVRetirement) QuiescePeer(context.Context, giznet.PublicKey) error     { return nil }
func (tiKVRetirement) Get(context.Context, string) (pendingdeletion.Record, error) {
	return pendingdeletion.Record{}, pendingdeletion.ErrNotFound
}
func (tiKVRetirement) HasLocator(context.Context, pendingdeletion.Locator) (bool, error) {
	return false, nil
}
func pendingPeerRetirement(t *testing.T, ctx context.Context, server *peer.Server, store kv.Store, keys *apikey.Server, assignments peer.DeletionAssignments) error {
	t.Helper()
	now := time.Now().Add(time.Second)
	var deletionID string
	for execution := 1; execution <= 32; execution++ {
		// Advance only the worker's scheduling clock. TiKV versions, ordinary
		// reader snapshots and the permanent Peer tombstone remain real.
		source := peer.PendingDeletionSource(store)
		refs, _, err := source.ScanDue(ctx, now, 10, "")
		if err != nil {
			return err
		}
		if len(refs) == 0 {
			// The preceding delete or defer acknowledged this task. Preserve the
			// existing bounded discovery recovery after that known write.
			refs, _, err = source.ScanDue(kv.WithPrimaryReads(ctx), now, 10, "")
			if err != nil {
				return err
			}
		}
		if len(refs) != 1 {
			return fmt.Errorf("execution %d pending tasks=%d, want 1", execution, len(refs))
		}
		claim, claimed, err := source.Claim(ctx, refs[0], now, time.Minute)
		if err != nil {
			return err
		}
		if !claimed {
			return errors.New("pending deletion not claimed")
		}
		if execution == 1 {
			deletionID = claim.Record.DeletionID
		}
		if claim.Record.DeletionID != deletionID || claim.ExecutionCount != execution-1 {
			return fmt.Errorf("execution %d claimed deletion=%s count=%d", execution, claim.Record.DeletionID, claim.ExecutionCount)
		}
		adapters := tiKVRetirement{owner: claim.Record.ResourceID}
		finishedAt := now.Add(time.Second)
		handler := peer.DeletionHandler{Server: server, Assignments: assignments, Source: source, Social: adapters, Workspaces: adapters, APIKeys: keys, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters, WorkspaceLookup: adapters, FriendGroupLookup: adapters, Now: func() time.Time { return finishedAt }}
		err = handler.Handle(ctx, claim)
		if execution == 32 {
			if err != nil {
				return err
			}
			if _, err := source.GetTask(ctx, deletionID); !errors.Is(err, pendingdeletion.ErrNotFound) {
				return fmt.Errorf("completed deletion task remains: %v", err)
			}
			t.Log("Peer retirement completed after 32 persisted executions with hourly scheduling")
			return nil
		}
		outcome, ok := errors.AsType[*pendingdeletion.OutcomeError](err)
		if !ok || outcome.Class != pendingdeletion.OutcomeDeferred || outcome.Code != "peer_cleanup_rescan_pending" || outcome.After != time.Hour {
			return fmt.Errorf("execution %d did not defer for hourly rescan: %v", execution, err)
		}
		nextAttempt := finishedAt.Add(outcome.After)
		if err := source.Defer(ctx, claim, outcome.Code, outcome.Message, nextAttempt, finishedAt); err != nil {
			return err
		}
		task, err := source.GetTask(ctx, deletionID)
		if err != nil {
			return err
		}
		if task.ExecutionCount != execution || task.FailureCount != 0 || !task.NextAttemptAt.Equal(nextAttempt) {
			return fmt.Errorf("execution %d persisted count=%d failures=%d next=%s", execution, task.ExecutionCount, task.FailureCount, task.NextAttemptAt)
		}
		early, _, err := source.ScanDue(ctx, nextAttempt.Add(-time.Nanosecond), 10, "")
		if err != nil || len(early) != 0 {
			return fmt.Errorf("execution %d became due before its hourly delay: tasks=%d error=%v", execution, len(early), err)
		}
		now = task.NextAttemptAt
	}
	return errors.New("Peer retirement did not complete")
}
