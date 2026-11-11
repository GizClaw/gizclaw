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
	tikvkv "github.com/tikv/client-go/v2/kv"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
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
	Keys        int
}
type observedTiKVClient struct {
	tikv.Client
	mu    sync.Mutex
	reads []tiKVReadObservation
}

func (c *observedTiKVClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	resp, err := c.Client.SendRequest(ctx, address, req, timeout)
	c.record(address, req, resp, err)
	return resp, err
}

func (c *observedTiKVClient) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	callback.Inject(func(resp *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		c.record(address, req, resp, err)
		return resp, err
	})
	c.Client.SendRequestAsync(ctx, address, req, callback)
}

func (c *observedTiKVClient) record(address string, req *tikvrpc.Request, resp *tikvrpc.Response, err error) {
	if err == nil && (req.Type == tikvrpc.CmdGet || req.Type == tikvrpc.CmdBatchGet || req.Type == tikvrpc.CmdScan) {
		regionError, decodeErr := resp.GetRegionError()
		if decodeErr == nil && regionError == nil {
			observation := tiKVReadObservation{Address: address, Stale: req.StaleRead, ReplicaRead: req.ReplicaReadType, Type: req.Type}
			switch req.Type {
			case tikvrpc.CmdGet:
				observation.Version, observation.Keys = req.Get().Version, 1
			case tikvrpc.CmdBatchGet:
				observation.Version, observation.Keys = req.BatchGet().Version, len(req.BatchGet().Keys)
			case tikvrpc.CmdScan:
				observation.Version = req.Scan().Version
			}
			c.mu.Lock()
			c.reads = append(c.reads, observation)
			c.mu.Unlock()
		}
	}
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
	t.Run("regional-replication-lag", func(t *testing.T) { testTiKVLag(t, client, observer) })
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

func testTiKVLag(t *testing.T, client *tikv.KVStore, observer *observedTiKVClient) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	const delay = 30 * time.Second
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{StaleReadDelay: delay, Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := kv.Prefixed(root, kv.Key{"lag-" + rand.Text()})
	peerStore := kv.Prefixed(base, kv.Key{"records"})
	routeStore := kv.Prefixed(base, kv.Key{"routes"})
	keyStore := kv.Prefixed(base, kv.Key{"keys"})
	centralPeers := &peer.Server{Store: peerStore, ServerPublicKey: giznet.PublicKey{71}}
	regionalPeers := &peer.Server{Store: peerStore}
	ownerKey, newKey, blockedKey := giznet.PublicKey{61}, giznet.PublicKey{62}, giznet.PublicKey{63}
	for _, key := range []giznet.PublicKey{ownerKey, newKey, blockedKey} {
		if _, err := centralPeers.SavePeer(ctx, apitypes.Peer{PublicKey: key.String(), Role: apitypes.PeerRoleClient, Status: apitypes.PeerRegistrationStatusActive}); err != nil {
			t.Fatal(err)
		}
	}
	owner := &peerroute.Server{Store: routeStore, Peers: regionalPeers, ServerPublicKey: giznet.PublicKey{71}, ServerEndpoint: "https://server-a.example"}
	foreign := &peerroute.Server{Store: routeStore, Peers: regionalPeers, ServerPublicKey: giznet.PublicKey{72}, ServerEndpoint: "https://server-b.example"}
	// Setup follows acknowledged Peer writes before the reader can see them.
	initial, err := owner.Assign(kv.WithPrimaryReads(ctx), ownerKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	keys := apikey.NewServer(keyStore)
	toRevoke, err := keys.Create(ctx, ownerKey.String(), "revoke", false)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := keys.Create(ctx, ownerKey.String(), "retire with peer", false)
	if err != nil {
		t.Fatal(err)
	}
	setKey, orderedKey := kv.Key{"members"}, kv.Key{"ordered-members"}
	if err := base.AddMembers(ctx, setKey, "a", "b"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ApplyMutation(ctx, kv.Mutation{AddOrderedMembers: []kv.SetMembers{{Key: orderedKey, Members: []string{"a", "b", "c"}}}}); err != nil {
		t.Fatal(err)
	}
	marker := kv.Key{"seed-ready"}
	if err := base.Set(ctx, marker, []byte("ready")); err != nil {
		t.Fatal(err)
	}
	t.Log("waiting for the configured historical reader to observe all seeded writes before freezing Raft")
	seedCtx, seedCancel := context.WithTimeout(ctx, delay+10*time.Second)
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
	proxyURL := requiredEnvironment(t, "GIZCLAW_TEST_TIKV_PROXY")
	var gate struct {
		Frozen  bool `json:"frozen"`
		Blocked int  `json:"blocked"`
	}
	if err := tiKVHTTP(ctx, http.MethodPost, proxyURL+"/freeze", nil, &gate); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = tiKVHTTP(cleanupCtx, http.MethodPost, proxyURL+"/resume", nil, nil)
	})
	if !gate.Frozen || gate.Blocked == 0 {
		t.Fatalf("replication was not interrupted: %+v", gate)
	}
	frozenAt := time.Now()
	readStart := len(observer.snapshot())
	assertLearnerReads := func(start int, operation string) {
		t.Helper()
		reads := observer.snapshot()[start:]
		if len(reads) == 0 {
			t.Fatalf("%s issued no observed reads", operation)
		}
		for _, r := range reads {
			// ReplicaRead and Version identify the historical Learner path;
			// the per-snapshot interceptor also marks Learner scans stale.
			if r.Address != "gate:20160" || r.ReplicaRead != tikvkv.ReplicaReadLearner || r.Version == 0 || r.Version>>18 >= uint64(frozenAt.UnixMilli()) {
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
	if _, err := owner.Assign(ctx, ownerKey, &initial.Version); !errors.Is(err, peerroute.ErrVersionConflict) {
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
	if err := base.RemoveMembers(ctx, setKey, "a"); err != nil {
		t.Fatal(err)
	}
	if err := base.AddMembers(ctx, setKey, "c"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ApplyMutation(ctx, kv.Mutation{
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
			if !r.Stale {
				t.Fatalf("historical Learner scan missing stale-read compatibility flag: %+v", r)
			}
		}
	}
	if scans < 2 {
		t.Fatalf("missing collection/member-range Scan proof: %d", scans)
	}
	if err := centralPeers.DeleteSelf(ctx, ownerKey); err != nil {
		t.Fatal(err)
	}
	if err := regionalPeers.EnsureAvailable(primaryCtx, ownerKey); !errors.Is(err, peer.ErrPeerPendingDeletion) {
		t.Fatalf("primary deletion fence: %v", err)
	}
	if err := regionalPeers.EnsureAvailable(ctx, ownerKey); err != nil {
		t.Fatalf("ordinary availability did not retain seeded Peer: %v", err)
	}
	if err := pendingPeerRetirement(t, ctx, centralPeers, peerStore, keys, owner); err != nil {
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
	if got, err := keys.Authenticate(ctx, retained.Secret); err != nil || got.Key.Name != retained.Key.Name {
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
		t.Fatal("lag assertions exceeded the retained snapshot window")
	}
	staleReads, strongReads := 0, 0
	for _, r := range observer.snapshot()[readStart:] {
		if r.Stale || r.ReplicaRead == tikvkv.ReplicaReadLearner {
			staleReads++
			if r.Address != "gate:20160" || r.Version == 0 || r.Version>>18 >= uint64(frozenAt.UnixMilli()) {
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
	if err := tiKVHTTP(ctx, http.MethodGet, proxyURL+"/status", nil, &gate); err != nil || !gate.Frozen {
		t.Fatalf("replication resumed during lag assertions: %+v %v", gate, err)
	}
	t.Logf("frozen Learner verified: %d historical reads, %d primary reads, collection scans and conflict fallback in %s", staleReads, strongReads, time.Since(frozenAt).Round(time.Millisecond))
	if err := tiKVHTTP(ctx, http.MethodPost, proxyURL+"/resume", nil, nil); err != nil {
		t.Fatal(err)
	}
	// A snapshot selected after resumption must age beyond all frozen writes.
	timer := time.NewTimer(delay + time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
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
	source := peer.PendingDeletionSource(store)
	now := time.Now().Add(time.Second)
	refs, _, err := source.ScanDue(ctx, now, 10, "")
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		// DeleteSelf just acknowledged this marker. An empty historical scan
		// conflicts with that known write, so retry discovery once on primary.
		refs, _, err = source.ScanDue(kv.WithPrimaryReads(ctx), now, 10, "")
		if err != nil {
			return err
		}
	}
	if len(refs) != 1 {
		return fmt.Errorf("pending tasks=%d, want 1", len(refs))
	}
	claim, claimed, err := source.Claim(ctx, refs[0], now, time.Minute)
	if err != nil {
		return err
	}
	if !claimed {
		return errors.New("pending deletion not claimed")
	}
	adapters := tiKVRetirement{owner: claim.Record.ResourceID}
	handler := peer.DeletionHandler{Server: server, Assignments: assignments, Source: source, Social: adapters, Workspaces: adapters, APIKeys: keys, RuntimeProfiles: adapters, Memory: adapters, Quiescer: adapters, WorkspaceLookup: adapters, FriendGroupLookup: adapters, Now: func() time.Time { return now.Add(time.Second) }}
	return handler.Handle(ctx, claim)
}
