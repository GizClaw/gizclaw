package kv

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/tikv/client-go/v2/config/retry"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

// This gate requires the isolated three-voter fixture. It leaves a real
// secondary lock behind a rolled-back primary, without replacing any KV result.
func TestTiKVDuplicateReads(t *testing.T) {
	pd := os.Getenv("GIZCLAW_TEST_TIKV_PD")
	if pd == "" {
		t.Skip("real TiKV cluster is exercised by the Docker gate")
	}
	addresses := waitTiKVDuplicateReadTopology(t, pd)
	physical, err := storage.New(map[string]storage.Config{"cluster": storage.TiKVConfig{PDAddresses: []string{pd}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	client, _, err := physical.TiKV("cluster")
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewTiKVWithClient(client, TiKVReadOptions{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"claim", "compare-and-set", "compare-and-delete", "batch-get", "batch-get-missing"} {
		if !t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			prefix := "duplicate-reads-" + rand.Text()
			key, generation, peer := Key{prefix, "owner"}, Key{prefix, "generation"}, Key{prefix, "peer"}
			before := []byte("before")
			if scenario == "claim" || scenario == "batch-get-missing" {
				before = nil
			} else if err := store.Set(ctx, key, before); err != nil {
				t.Fatal(err)
			}
			if err := store.Set(ctx, generation, []byte("generation")); err != nil {
				t.Fatal(err)
			}
			startTS := leaveTiKVSecondaryLock(t, ctx, store, key, Key{prefix, "primary"})
			original := client.GetTiKVClient()
			observed := &tiKVDuplicateReadClient{Client: original, startTS: startTS, resolved: make(chan error, 1)}
			client.SetTiKVClient(observed)
			t.Cleanup(func() {
				client.SetTiKVClient(original)
				t.Logf("real RPCs: batch_get=%d resolve_lock=%d duplicate_batch_get=%d duplicate_resolve_lock=%d", observed.batchReads.Load(), observed.resolves.Load(), observed.duplicateReads.Load(), observed.duplicateResolves.Load())
			})

			want := []byte("after")
			switch scenario {
			case "batch-get", "batch-get-missing":
				results := store.BatchGet(ctx, []Key{key, {prefix, "missing"}, key, generation})
				for _, i := range []int{0, 2} {
					if !bytes.Equal(results[i].Value, before) || (before == nil && !errors.Is(results[i].Err, ErrNotFound)) || (before != nil && results[i].Err != nil) {
						t.Fatalf("duplicate result %d = %+v, want %q", i, results[i], before)
					}
				}
				if !errors.Is(results[1].Err, ErrNotFound) || results[3].Err != nil || string(results[3].Value) != "generation" {
					t.Fatalf("batch input order or missing result changed: %+v", results)
				}
				if before != nil {
					results[0].Value[0] = 'X'
					if !bytes.Equal(results[2].Value, before) {
						t.Fatal("duplicate results share mutable bytes")
					}
				}
				want = before
			default:
				mutation := Mutation{Conditions: []Condition{{Key: key, Expected: before}}, Entries: []Entry{{Key: key, Value: want}}}
				if scenario == "claim" {
					mutation.Conditions = append(mutation.Conditions, Condition{Key: generation, Expected: []byte("generation")})
					mutation.Entries = append(mutation.Entries, Entry{Key: peer, Value: []byte("online")})
				} else if scenario == "compare-and-delete" {
					mutation.Entries, mutation.DeleteKeys, want = nil, []Key{key}, nil
				}
				if ok, err := store.ApplyMutation(ctx, mutation); err != nil || !ok {
					t.Fatalf("conditional mutation = %v, %v", ok, err)
				}
				if scenario == "claim" {
					if value, err := store.Get(ctx, peer); err != nil || string(value) != "online" {
						t.Fatalf("claim snapshot = %q, %v", value, err)
					}
				}
			}
			// Read-side cleanup is asynchronous: a successful mutation/read does
			// not establish that the rollback or resolved-ts processing survived.
			select {
			case err := <-observed.resolved:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal("secondary-lock cleanup did not finish:", ctx.Err())
			}
			rawKey := store.recordKey(key)
			locks := tiKVDuplicateReadRequest(t, ctx, client, rawKey, tikvrpc.NewRequest(tikvrpc.CmdScanLock, &kvrpcpb.ScanLockRequest{StartKey: rawKey, EndKey: tiKVPrefixEnd(rawKey), MaxVersion: startTS, Limit: 1})).Resp.(*kvrpcpb.ScanLockResponse)
			if locks.Error != nil || len(locks.Locks) != 0 {
				t.Fatalf("secondary lock remains: %s", locks)
			}
			value, err := store.Get(ctx, key)
			if !bytes.Equal(value, want) || (want == nil && !errors.Is(err, ErrNotFound)) || (want != nil && err != nil) {
				t.Fatalf("readback = %q, %v; want %q", value, err, want)
			}
			ts, err := store.timestamp(ctx)
			if err != nil {
				t.Fatal(err)
			}
			waitTiKVDuplicateReadSafeTS(t, client, addresses, ts)
			if observed.duplicateReads.Load() != 0 || observed.duplicateResolves.Load() != 0 {
				t.Fatal("sent duplicate BatchGet or ResolveLock keys")
			}
		}) {
			return
		}
	}
	waitTiKVDuplicateReadTopology(t, pd)
}

func leaveTiKVSecondaryLock(t *testing.T, ctx context.Context, store *TiKV, secondary, primary Key) uint64 {
	t.Helper()
	ts, err := store.timestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	primaryKey := store.recordKey(primary)
	for _, key := range [][]byte{primaryKey, store.recordKey(secondary)} {
		resp := tiKVDuplicateReadRequest(t, ctx, store.client, key, tikvrpc.NewRequest(tikvrpc.CmdPrewrite, &kvrpcpb.PrewriteRequest{
			Mutations:   []*kvrpcpb.Mutation{{Op: kvrpcpb.Op_Put, Key: key, Value: (tiKVValue{payload: []byte("abandoned")}).encode()}},
			PrimaryLock: primaryKey, StartVersion: ts, LockTtl: 1, TxnSize: 2,
		})).Resp.(*kvrpcpb.PrewriteResponse)
		if len(resp.Errors) != 0 {
			t.Fatalf("prewrite abandoned transaction: %s", resp)
		}
	}
	rollback := tiKVDuplicateReadRequest(t, ctx, store.client, primaryKey, tikvrpc.NewRequest(tikvrpc.CmdBatchRollback, &kvrpcpb.BatchRollbackRequest{Keys: [][]byte{primaryKey}, StartVersion: ts})).Resp.(*kvrpcpb.BatchRollbackResponse)
	if rollback.Error != nil {
		t.Fatalf("rollback primary: %s", rollback)
	}
	readTS, err := store.timestamp(ctx)
	if err != nil {
		t.Fatal(err)
	}
	key := store.recordKey(secondary)
	locked := tiKVDuplicateReadRequest(t, ctx, store.client, key, tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Key: key, Version: readTS})).Resp.(*kvrpcpb.GetResponse)
	if lock := locked.GetError().GetLocked(); lock == nil || lock.LockVersion != ts || !bytes.Equal(lock.PrimaryLock, primaryKey) {
		t.Fatalf("expected the abandoned transaction's secondary lock: %s", locked)
	}
	return ts
}

func tiKVDuplicateReadRequest(t *testing.T, ctx context.Context, client *tikv.KVStore, key []byte, req *tikvrpc.Request) *tikvrpc.Response {
	t.Helper()
	bo := retry.NewBackofferWithVars(ctx, 5000, nil)
	location, err := client.GetRegionCache().LocateKey(bo, key)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.SendReq(bo, req, location.Region, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if regionErr, err := resp.GetRegionError(); err != nil || regionErr != nil {
		t.Fatalf("%s region error: %v, %v", req.Type, regionErr, err)
	}
	return resp
}

type tiKVDuplicateReadClient struct {
	tikv.Client
	startTS           uint64
	resolved          chan error
	batchReads        atomic.Int64
	resolves          atomic.Int64
	duplicateReads    atomic.Int64
	duplicateResolves atomic.Int64
}

func (c *tiKVDuplicateReadClient) observe(req *tikvrpc.Request) {
	var keys [][]byte
	var duplicate *atomic.Int64
	switch req.Type {
	case tikvrpc.CmdBatchGet:
		c.batchReads.Add(1)
		keys, duplicate = req.BatchGet().Keys, &c.duplicateReads
	case tikvrpc.CmdResolveLock:
		c.resolves.Add(1)
		keys, duplicate = req.ResolveLock().Keys, &c.duplicateResolves
	default:
		return
	}
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[string(key)] {
			duplicate.Add(1)
			return
		}
		seen[string(key)] = true
	}
}

func (c *tiKVDuplicateReadClient) complete(req *tikvrpc.Request, resp *tikvrpc.Response, err error) {
	if req.Type != tikvrpc.CmdResolveLock || req.ResolveLock().StartVersion != c.startTS {
		return
	}
	if err == nil {
		regionErr, decodeErr := resp.GetRegionError()
		if decodeErr != nil || regionErr != nil {
			err = fmt.Errorf("secondary cleanup region error: %v, %v", regionErr, decodeErr)
		} else if keyErr := resp.Resp.(*kvrpcpb.ResolveLockResponse).Error; keyErr != nil {
			err = fmt.Errorf("secondary cleanup key error: %s", keyErr)
		}
	}
	select {
	case c.resolved <- err:
	default:
	}
}

func (c *tiKVDuplicateReadClient) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	c.observe(req)
	resp, err := c.Client.SendRequest(ctx, address, req, timeout)
	c.complete(req, resp, err)
	return resp, err
}

func (c *tiKVDuplicateReadClient) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	c.observe(req)
	callback.Inject(func(resp *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		c.complete(req, resp, err)
		return resp, err
	})
	c.Client.SendRequestAsync(ctx, address, req, callback)
}

func waitTiKVDuplicateReadTopology(t *testing.T, pd string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var stores struct {
			Stores []struct {
				Store struct {
					ID        uint64 `json:"id"`
					Address   string `json:"address"`
					StateName string `json:"state_name"`
				} `json:"store"`
			} `json:"stores"`
		}
		var topology struct {
			Regions []struct {
				Leader struct{ ID uint64 } `json:"leader"`
				Peers  []struct {
					StoreID  uint64 `json:"store_id"`
					RoleName string `json:"role_name"`
				} `json:"peers"`
				PendingPeers []json.RawMessage `json:"pending_peers"`
				DownPeers    []json.RawMessage `json:"down_peers"`
			} `json:"regions"`
		}
		storeErr := tiKVDuplicateReadJSON(ctx, "http://"+pd+"/pd/api/v1/stores", &stores)
		regionErr := tiKVDuplicateReadJSON(ctx, "http://"+pd+"/pd/api/v1/regions", &topology)
		ready := storeErr == nil && regionErr == nil && len(stores.Stores) == 3 && len(topology.Regions) > 0
		addresses := make([]string, 0, 3)
		up := make(map[uint64]bool)
		for _, entry := range stores.Stores {
			ready = ready && entry.Store.StateName == "Up"
			up[entry.Store.ID] = entry.Store.StateName == "Up"
			addresses = append(addresses, entry.Store.Address)
		}
		for _, region := range topology.Regions {
			ready = ready && region.Leader.ID != 0 && len(region.Peers) == 3 && len(region.PendingPeers) == 0 && len(region.DownPeers) == 0
			seen := make(map[uint64]bool)
			for _, peer := range region.Peers {
				ready = ready && up[peer.StoreID] && peer.RoleName == "Voter" && !seen[peer.StoreID]
				seen[peer.StoreID] = true
			}
		}
		if ready {
			t.Logf("PD verified three Up stores and three distinct voters in each of %d Regions", len(topology.Regions))
			return addresses
		}
		select {
		case <-ctx.Done():
			t.Fatalf("three-voter readiness: stores=%+v regions=%+v errors=%v, %v: %v", stores, topology, storeErr, regionErr, ctx.Err())
		case <-ticker.C:
		}
	}
}

func tiKVDuplicateReadJSON(ctx context.Context, endpoint string, output any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PD status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(output)
}

func waitTiKVDuplicateReadSafeTS(t *testing.T, client *tikv.KVStore, addresses []string, target uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		ready := true
		for _, address := range addresses {
			resp, err := client.GetTiKVClient().SendRequest(ctx, address, tikvrpc.NewRequest(tikvrpc.CmdStoreSafeTS, &kvrpcpb.StoreSafeTSRequest{KeyRange: &kvrpcpb.KeyRange{}}), time.Second)
			if err != nil {
				t.Fatalf("voter %s failed after secondary cleanup: %v", address, err)
			}
			ready = ready && resp.Resp.(*kvrpcpb.StoreSafeTSResponse).SafeTs >= target
		}
		if ready {
			t.Logf("all three voter safe timestamps advanced past %d after cleanup", target)
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("voter safe timestamps did not advance after cleanup:", ctx.Err())
		case <-ticker.C:
		}
	}
}
