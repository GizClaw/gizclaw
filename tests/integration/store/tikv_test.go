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
	Address string
	Stale   bool
	Version uint64
	Keys    int
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
			observation := tiKVReadObservation{Address: address, Stale: req.StaleRead}
			switch req.Type {
			case tikvrpc.CmdGet:
				observation.Version, observation.Keys = req.Get().Version, 1
			case tikvrpc.CmdBatchGet:
				observation.Version, observation.Keys = req.BatchGet().Version, len(req.BatchGet().Keys)
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
	root, err := kv.NewTiKVWithClient(client, kv.TiKVReadOptions{StaleReadDelay: 30 * time.Second, Labels: map[string]string{"region": "local"}}, nil)
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
	initial, err := owner.Assign(ctx, ownerKey, nil)
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
	t.Log("waiting for historical snapshots to include seeded records before freezing Raft replication")
	timer := time.NewTimer(31 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-timer.C:
	}
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
	_ = oldSecret
	owner.ServerEndpoint = "https://server-a-new.example"
	updated, err := owner.Assign(ctx, ownerKey, &initial.Version)
	if err != nil || updated.Version != 2 {
		t.Fatalf("refresh: %+v %v", updated, err)
	}
	primaryAssignment, err := routeStore.Get(kv.WithPrimaryReads(ctx), kv.Key{"by-peer", ownerKey.String()})
	if err != nil || bytes.Equal(primaryAssignment, oldAssignment) {
		t.Fatalf("primary-read context used the frozen Learner: %v", err)
	}
	if got, err := foreign.Resolve(ctx, ownerKey); err != nil || got.Version != 2 || got.ServerPublicKey != initial.ServerPublicKey {
		t.Fatalf("Edge resolve saw old assignment: %+v %v", got, err)
	}
	if _, err := foreign.Assign(ctx, ownerKey, nil); !errors.Is(err, peerroute.ErrVersionConflict) {
		t.Fatalf("foreign claim: %v", err)
	}
	if _, err := owner.Assign(ctx, ownerKey, &initial.Version); !errors.Is(err, peerroute.ErrVersionConflict) {
		t.Fatalf("old expected version accepted: %v", err)
	}
	if _, err := owner.Assign(ctx, newKey, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := foreign.Resolve(ctx, newKey); err != nil || got.ServerPublicKey != owner.ServerPublicKey.String() {
		t.Fatalf("Edge missed first assignment: %+v %v", got, err)
	}
	if err := keys.RevokeOwner(ctx, ownerKey.String(), toRevoke.Key.Name); err != nil {
		t.Fatal(err)
	}
	if _, err := apikey.NewServer(keyStore).Authenticate(ctx, toRevoke.Secret); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("revoked credential authenticated: %v", err)
	}
	if err := centralPeers.DeleteSelf(ctx, ownerKey); err != nil {
		t.Fatal(err)
	}
	if err := regionalPeers.EnsureAvailable(ctx, ownerKey); !errors.Is(err, peer.ErrPeerPendingDeletion) {
		t.Fatalf("regional deletion fence: %v", err)
	}
	if _, err := regionalPeers.EnsureConnectedPeer(ctx, ownerKey); !errors.Is(err, peer.ErrPeerPendingDeletion) {
		t.Fatalf("deleted Peer reconnected: %v", err)
	}
	if _, err := foreign.Resolve(ctx, ownerKey); !errors.Is(err, peer.ErrPeerPendingDeletion) {
		t.Fatalf("Edge routed pending deletion: %v", err)
	}
	if err := pendingPeerRetirement(t, ctx, centralPeers, peerStore, keys, owner); err != nil {
		t.Fatal(err)
	}
	if err := regionalPeers.EnsureAvailable(ctx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("regional permanent tombstone: %v", err)
	}
	if _, err := regionalPeers.EnsureConnectedPeer(ctx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("tombstoned Peer reconnected: %v", err)
	}
	if _, err := foreign.Resolve(ctx, ownerKey); !errors.Is(err, peer.ErrPeerDeleted) {
		t.Fatalf("Edge routed deleted Peer: %v", err)
	}
	if _, err := keys.Authenticate(ctx, retained.Secret); !errors.Is(err, apikey.ErrInvalidAPIKey) {
		t.Fatalf("retired owner key authenticated: %v", err)
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
	if _, err := regionalPeers.EnsureConnectedPeer(ctx, blockedKey); !errors.Is(err, peer.ErrPeerBlocked) {
		t.Fatalf("blocked Peer reconnected: %v", err)
	}
	// Establish stale controls after every strong-state assertion. The proxy
	// stays frozen, so a passing test cannot be explained by replication catchup.
	if got, err := routeStore.Get(ctx, kv.Key{"by-peer", ownerKey.String()}); err != nil || !bytes.Equal(got, oldAssignment) {
		t.Fatalf("Learner assignment control changed: %v", err)
	}
	if _, err := routeStore.Get(ctx, kv.Key{"by-peer", newKey.String()}); !errors.Is(err, kv.ErrNotFound) {
		t.Fatalf("Learner saw new assignment: %v", err)
	}
	if got, err := peerStore.Get(ctx, kv.Key{"by-pubkey", ownerKey.String()}); err != nil || !bytes.Equal(got, oldPeer) {
		t.Fatalf("Learner peer control changed: %v", err)
	}
	if _, err := keyStore.Get(ctx, kv.Key{"secrets", toRevoke.Secret}); err != nil {
		t.Fatalf("Learner no longer contains revoked credential: %v", err)
	}
	if time.Since(frozenAt) > 25*time.Second {
		t.Fatal("lag assertions exceeded the retained snapshot window")
	}
	staleReads, strongReads := 0, 0
	for _, r := range observer.snapshot()[readStart:] {
		if r.Stale {
			staleReads++
			if r.Address != "gate:20160" {
				t.Fatalf("ordinary read escaped regional Learner: %+v", r)
			}
		} else {
			strongReads++
			if r.Address != "leader:20160" {
				t.Fatalf("strong read used regional Learner: %+v", r)
			}
		}
	}
	if staleReads < 6 || strongReads == 0 {
		t.Fatalf("missing observed read paths: stale=%d strong=%d", staleReads, strongReads)
	}
	t.Logf("frozen regional Learner retained old assignment, active Peer and revoked API key; %d stale Learner reads and %d strong center reads verified in %s", staleReads, strongReads, time.Since(frozenAt).Round(time.Millisecond))
}

// Empty child-resource adapters model a Peer with no Social/Workspace/memory
// resources. The durable deletion workflow and API key retirement remain real.
type tiKVRetirement struct{ owner string }

func (r tiKVRetirement) SnapshotPeerSocial(context.Context, string) (social.PeerSnapshot, error) {
	return social.PeerSnapshot{PublicKey: r.owner}, nil
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
