//go:build store_e2e

package store_e2e_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GizClaw/gizclaw-go/pkgs/store/kv"
	"github.com/GizClaw/gizclaw-go/pkgs/store/storage"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/config"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

type policyObservation struct {
	Address                 string
	Region, Version         uint64
	Command                 tikvrpc.CmdType
	Stale, NotReady, Denied bool
	Key                     []byte
}
type policyObserver struct {
	tikv.Client
	before func(*tikvrpc.Request)
	mu     sync.Mutex
	reads  []policyObservation
}

func (o *policyObserver) record(addr string, req *tikvrpc.Request, resp *tikvrpc.Response, err error) {
	entry := policyObservation{Address: addr, Region: req.RegionId, Command: req.Type, Stale: req.StaleRead, Denied: errors.Is(err, storage.ErrTiKVReadDenied)}
	switch req.Type {
	case tikvrpc.CmdGet:
		entry.Version, entry.Key = req.Get().Version, bytes.Clone(req.Get().Key)
	case tikvrpc.CmdBatchGet:
		entry.Version = req.BatchGet().Version
	case tikvrpc.CmdScan:
		entry.Version, entry.Key = req.Scan().Version, bytes.Clone(req.Scan().StartKey)
	default:
		if !req.IsTxnWriteRequest() {
			return
		}
	}
	if resp != nil {
		re, _ := resp.GetRegionError()
		entry.NotReady = re != nil && re.DataIsNotReady != nil
	}
	o.mu.Lock()
	o.reads = append(o.reads, entry)
	o.mu.Unlock()
}
func (o *policyObserver) SendRequest(ctx context.Context, addr string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	if o.before != nil {
		o.before(req)
	}
	resp, err := o.Client.SendRequest(ctx, addr, req, timeout)
	o.record(addr, req, resp, err)
	return resp, err
}
func (o *policyObserver) SendRequestAsync(ctx context.Context, addr string, req *tikvrpc.Request, cb async.Callback[*tikvrpc.Response]) {
	if o.before != nil {
		o.before(req)
	}
	cb.Inject(func(resp *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		o.record(addr, req, resp, err)
		return resp, err
	})
	o.Client.SendRequestAsync(ctx, addr, req, cb)
}
func (o *policyObserver) take() []policyObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	r := o.reads
	o.reads = nil
	return r
}

func TestTiKVLocalReadChannels(t *testing.T) {
	if os.Getenv("GIZCLAW_TEST_TIKV_POLICY_FIXTURE") != "isolated" {
		t.Skip("requires dedicated multi-Learner fixture")
	}
	previousAsync := config.GetGlobalConfig().EnableAsyncBatchGet
	config.UpdateGlobal(func(c *config.Config) { c.EnableAsyncBatchGet = true })
	t.Cleanup(func() { config.UpdateGlobal(func(c *config.Config) { c.EnableAsyncBatchGet = previousAsync }) })
	pdAddress := requiredEnvironment(t, "GIZCLAW_TEST_TIKV_PD")
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	waitPolicy(t, ctx, func() bool {
		var stores, regions struct {
			Count int `json:"count"`
		}
		return tiKVHTTP(ctx, http.MethodGet, "http://"+pdAddress+"/pd/api/v1/stores", nil, &stores) == nil && tiKVHTTP(ctx, http.MethodGet, "http://"+pdAddress+"/pd/api/v1/regions", nil, &regions) == nil && stores.Count >= 3 && regions.Count > 0
	})
	if err := tiKVHTTP(ctx, http.MethodPost, "http://"+pdAddress+"/pd/api/v1/config/replicate", map[string]any{"max-replicas": 1, "enable-placement-rules": "true"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := tiKVHTTP(ctx, http.MethodPost, "http://"+pdAddress+"/pd/api/v1/config/rule", map[string]any{"group_id": "pd", "id": "default", "role": "voter", "count": 1, "label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"center"}}}}, nil); err != nil {
		t.Fatal(err)
	}
	rule := map[string]any{"group_id": "gizclaw-e2e", "id": "regional", "role": "learner", "count": 2, "label_constraints": []map[string]any{{"key": "region", "op": "in", "values": []string{"local"}}}}
	if err := tiKVHTTP(ctx, http.MethodPost, "http://"+pdAddress+"/pd/api/v1/config/rule", rule, nil); err != nil {
		t.Fatal(err)
	}
	physical, err := storage.New(map[string]storage.Config{"test": storage.TiKVConfig{PDAddresses: []string{pdAddress}, StaleReadDelay: 5 * time.Second, ReadLabels: map[string]string{"region": "local"}, ReadLocalOnly: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = physical.Close() })
	client, _, err := physical.TiKV("test")
	if err != nil {
		t.Fatal(err)
	}
	observer := &policyObserver{Client: client.GetTiKVClient()}
	client.SetTiKVClient(observer)
	var local []string
	center := ""
	waitPolicy(t, ctx, func() bool {
		stores, err := client.GetPDClient().GetAllStores(ctx)
		if err != nil {
			return false
		}
		byID := map[uint64]string{}
		for _, store := range stores {
			byID[store.Id] = store.Address
			for _, label := range store.Labels {
				if label.Key == "region" && label.Value == "center" {
					center = store.Address
				}
			}
		}
		r, err := client.GetPDClient().GetRegion(ctx, []byte("policy"))
		if err != nil || r == nil {
			return false
		}
		var peers []*metapb.Peer
		for _, peer := range r.Meta.Peers {
			if peer.Role == metapb.PeerRole_Learner {
				peers = append(peers, peer)
			}
		}
		slices.SortFunc(peers, func(a, b *metapb.Peer) int {
			if a.StoreId < b.StoreId {
				return -1
			}
			return 1
		})
		local = nil
		for _, peer := range peers {
			local = append(local, byID[peer.StoreId])
		}
		return len(local) == 2 && center != ""
	})
	t.Logf("PD verified center=%s Learners=%v", center, local)

	reader, err := physical.TiKVReader("test")
	if err != nil {
		t.Fatal(err)
	}
	if reader == client || reader.GetRegionCache() == client.GetRegionCache() || reader.GetPDClient() == client.GetPDClient() {
		t.Fatal("read/write routing state is shared")
	}
	readObserver := &policyObserver{Client: reader.GetTiKVClient()}
	reader.SetTiKVClient(readObserver)
	store, err := kv.NewTiKVWithClients(reader, client, kv.TiKVReadOptions{StaleReadDelay: 5 * time.Second, Labels: map[string]string{"region": "local"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	strong := kv.WithStrongReads(store)
	keys := []kv.Key{{"channels", "a"}, {"channels", "m"}, {"channels", "z"}}
	for _, key := range keys {
		if err := store.Set(ctx, key, []byte("old")); err != nil {
			t.Fatal(err)
		}
	}
	set, ordered := kv.Key{"channels", "members"}, kv.Key{"channels", "ordered"}
	members := make([]string, 600)
	for i := range members {
		members[i] = fmt.Sprintf("%04d", i)
	}
	if err := store.AddMembers(ctx, set, members...); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyMutation(ctx, kv.Mutation{AddOrderedMembers: []kv.SetMembers{{Key: ordered, Members: members}}}); err != nil {
		t.Fatal(err)
	}
	waitPolicy(t, ctx, func() bool {
		op, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		v, err := store.ListMembers(op, set)
		return err == nil && len(v) == 600
	})
	control := func(t *testing.T, addr, path string) {
		t.Helper()
		if err := tiKVHTTP(ctx, http.MethodPost, "http://"+strings.Split(addr, ":")[0]+":8080"+path, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	assertLocal := func(t *testing.T, reads []policyObservation, oneSnapshot bool) {
		t.Helper()
		sent, denied := 0, 0
		var ts uint64
		for _, r := range reads {
			if r.Version == 0 {
				continue
			}
			if r.Denied {
				denied++
				continue
			}
			sent++
			if !slices.Contains(local, r.Address) || !r.Stale {
				t.Fatalf("ordinary RPC escaped: %+v", r)
			}
			if ts == 0 {
				ts = r.Version
			}
			if oneSnapshot && r.Version != ts {
				t.Fatal("SDK retry/page changed snapshot")
			}
		}
		t.Logf("completed local data RPCs=%d; non-local SDK candidates rejected before send=%d; first version=%d", sent, denied, ts)
	}
	t.Run("native-batch-and-cross-region-scan", func(t *testing.T) {
		readObserver.take()
		if _, err := store.Get(ctx, keys[2]); err != nil {
			t.Fatal(err)
		}
		var rawKey []byte
		for _, r := range readObserver.take() {
			if !r.Denied {
				rawKey = r.Key
				break
			}
		}
		if _, err := store.ListMembers(ctx, set); err != nil {
			t.Fatal(err)
		}
		var prefix []byte
		for _, r := range readObserver.take() {
			if r.Command == tikvrpc.CmdScan && !r.Denied {
				prefix = r.Key
				break
			}
		}
		if len(rawKey) == 0 || len(prefix) == 0 {
			t.Fatal("missing real Get/Scan observations")
		}
		if _, err := client.SplitRegions(ctx, [][]byte{rawKey, append(bytes.Clone(prefix), []byte("0300")...)}, false, nil); err != nil {
			t.Fatal(err)
		}
		waitPolicy(t, ctx, func() bool {
			op, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			for _, r := range store.BatchGet(op, keys) {
				if r.Err != nil {
					return false
				}
			}
			return true
		})
		readObserver.take()
		for _, r := range store.BatchGet(ctx, keys) {
			if r.Err != nil || string(r.Value) != "old" {
				t.Fatalf("batch=%+v", r)
			}
		}
		reads := readObserver.take()
		assertLocal(t, reads, true)
		regions := map[uint64]bool{}
		for _, r := range reads {
			if !r.Denied {
				regions[r.Region] = true
			}
		}
		if len(regions) < 2 {
			t.Fatal("batch did not cross Regions")
		}
		v, err := store.ListMembers(ctx, set)
		if err != nil || !slices.Equal(v, members) {
			t.Fatalf("list=%d %v", len(v), err)
		}
		reads = readObserver.take()
		assertLocal(t, reads, true)
		regions = map[uint64]bool{}
		for _, r := range reads {
			if !r.Denied && r.Command == tikvrpc.CmdScan {
				regions[r.Region] = true
			}
		}
		if len(regions) < 2 {
			t.Fatal("scan did not cross Regions")
		}
		has, err := store.HasMember(ctx, set, "0300")
		if err != nil || !has {
			t.Fatalf("membership=%t %v", has, err)
		}
		assertLocal(t, readObserver.take(), true)
		after := "0299"
		v, err = store.RangeOrderedMembers(ctx, ordered, kv.OrderedRange{After: &after, Limit: 10})
		if err != nil || !slices.Equal(v, members[300:310]) {
			t.Fatalf("range=%v %v", v, err)
		}
		assertLocal(t, readObserver.take(), true)
	})
	t.Run("notfound", func(t *testing.T) {
		readObserver.take()
		_, err := store.Get(ctx, kv.Key{"channels", "missing"})
		if !errors.Is(err, kv.ErrNotFound) {
			t.Fatal(err)
		}
		reads := readObserver.take()
		assertLocal(t, reads, true)
		sent := 0
		for _, r := range reads {
			if !r.Denied {
				sent++
			}
		}
		if sent != 1 {
			t.Fatal("NotFound triggered another data read")
		}
	})
	t.Run("single-learner-fault", func(t *testing.T) {
		for _, addr := range local {
			control(t, addr, "/disconnect")
			for range 3 {
				op, cancel := context.WithTimeout(ctx, 2*time.Second)
				readObserver.take()
				v, err := store.Get(op, keys[0])
				cancel()
				if err != nil || string(v) != "old" {
					t.Fatalf("one Learner unavailable=%q %v", v, err)
				}
				assertLocal(t, readObserver.take(), true)
			}
			control(t, addr, "/connect")
		}
	})
	t.Run("all-local-fault-and-writer-isolation", func(t *testing.T) {
		for _, addr := range local {
			control(t, addr, "/disconnect")
			defer control(t, addr, "/connect")
		}
		for name, operation := range map[string]func(context.Context) error{
			"get":    func(ctx context.Context) error { _, e := store.Get(ctx, keys[0]); return e },
			"batch":  func(ctx context.Context) error { return store.BatchGet(ctx, keys)[0].Err },
			"member": func(ctx context.Context) error { _, e := store.HasMember(ctx, set, "0300"); return e },
			"list":   func(ctx context.Context) error { _, e := store.ListMembers(ctx, set); return e },
			"range": func(ctx context.Context) error {
				_, e := store.RangeOrderedMembers(ctx, ordered, kv.OrderedRange{Limit: 10})
				return e
			},
		} {
			readObserver.take()
			op, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			start := time.Now()
			err := operation(op)
			cancel()
			if err == nil || errors.Is(err, kv.ErrNotFound) || time.Since(start) > 1500*time.Millisecond {
				t.Fatalf("%s elapsed=%v error=%v", name, time.Since(start), err)
			}
			assertLocal(t, readObserver.take(), true)
		}
		observer.take()
		if err := store.Set(ctx, keys[0], []byte("new")); err != nil {
			t.Fatal(err)
		}
		v, err := strong.Get(ctx, keys[0])
		if err != nil || string(v) != "new" {
			t.Fatalf("writer affected=%q %v", v, err)
		}
		matched, err := store.CompareAndMutate(ctx, keys[0], []byte("old"), []kv.Entry{{Key: keys[0], Value: []byte("wrong")}}, nil)
		if err != nil || matched {
			t.Fatalf("stale guard accepted: %t %v", matched, err)
		}
		matched, err = store.CompareAndMutate(ctx, keys[0], []byte("new"), []kv.Entry{{Key: keys[0], Value: []byte("current")}}, nil)
		if err != nil || !matched {
			t.Fatalf("current guard rejected: %t %v", matched, err)
		}
		for _, r := range observer.take() {
			if r.Address != center || r.Stale {
				t.Fatalf("writer/strong read misrouted: %+v", r)
			}
		}
	})
	waitPolicy(t, ctx, func() bool {
		op, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		v, err := store.Get(op, keys[0])
		return err == nil && string(v) == "current"
	})
	t.Run("safe-ts-lag-and-recovery", func(t *testing.T) {
		for _, addr := range local {
			control(t, addr, "/freeze")
			defer control(t, addr, "/resume")
		}
		if err := store.Set(ctx, keys[0], []byte("during-lag")); err != nil {
			t.Fatal(err)
		}
		time.Sleep(7 * time.Second)
		readObserver.take()
		op, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		_, err := store.Get(op, keys[0])
		cancel()
		if err == nil {
			t.Fatal("returned a different snapshot during lag")
		}
		reads := readObserver.take()
		assertLocal(t, reads, true)
		if !slices.ContainsFunc(reads, func(r policyObservation) bool { return r.NotReady }) {
			t.Fatal("no actual DataIsNotReady response observed")
		}
		v, err := strong.Get(ctx, keys[0])
		if err != nil || string(v) != "during-lag" {
			t.Fatalf("strong during lag=%q %v", v, err)
		}
		for _, addr := range local {
			control(t, addr, "/resume")
		}
		waitPolicy(t, ctx, func() bool {
			op, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			v, err := store.Get(op, keys[0])
			return err == nil && string(v) == "during-lag"
		})
	})
	t.Run("socket-deadlines-get-batch-scan", func(t *testing.T) {
		for _, kind := range []tikvrpc.CmdType{tikvrpc.CmdGet, tikvrpc.CmdBatchGet, tikvrpc.CmdScan} {
			operation := func(op context.Context) error {
				switch kind {
				case tikvrpc.CmdGet:
					_, err := store.Get(op, keys[0])
					return err
				case tikvrpc.CmdBatchGet:
					return store.BatchGet(op, keys)[0].Err
				default:
					_, err := store.ListMembers(op, set)
					return err
				}
			}
			// Recover routing after the preceding outages before timing a real
			// response stall; scans must first read their collection metadata.
			waitPolicy(t, ctx, func() bool {
				op, cancel := context.WithTimeout(ctx, 2*time.Second)
				defer cancel()
				return operation(op) == nil
			})
			heldResponses := func() int {
				held := 0
				for _, addr := range local {
					var status struct{ Held int }
					if err := tiKVHTTP(ctx, http.MethodGet, "http://"+strings.Split(addr, ":")[0]+":8080/status", nil, &status); err != nil {
						t.Fatal(err)
					}
					held += status.Held
				}
				return held
			}
			heldBefore := heldResponses()
			var paused atomic.Bool
			fault := &policyObserver{Client: readObserver, before: func(req *tikvrpc.Request) {
				if req.Type == kind && req.Peer.GetRole() == metapb.PeerRole_Learner && paused.CompareAndSwap(false, true) {
					for _, addr := range local {
						control(t, addr, "/pause-reads")
					}
				}
			}}
			reader.SetTiKVClient(fault)
			readObserver.take()
			op, cancel := context.WithTimeout(ctx, time.Second)
			start := time.Now()
			err := operation(op)
			elapsed := time.Since(start)
			cancel()
			reader.SetTiKVClient(readObserver)
			for _, addr := range local {
				control(t, addr, "/resume-reads")
			}
			if !paused.Load() || heldResponses() <= heldBefore || !errors.Is(err, context.DeadlineExceeded) || elapsed > 1500*time.Millisecond {
				t.Fatalf("%s deadline elapsed=%v err=%v cause=%v (%T) paused=%t", kind, elapsed, err, errors.Unwrap(err), errors.Unwrap(err), paused.Load())
			}
			// Async BatchGet can stop its SDK run loop on cancellation before
			// completion callbacks execute. The gate counter verifies real
			// blocked responses independently of those completion observations.
			assertLocal(t, readObserver.take(), true)
			t.Logf("%s real response stall respected 1s caller deadline: %v", kind, elapsed)
		}
	})
}

func waitPolicy(t *testing.T, ctx context.Context, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("local Learner readiness timed out")
		case <-ticker.C:
		}
	}
}
