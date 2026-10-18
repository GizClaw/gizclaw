package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/tikvrpc/interceptor"
	"github.com/tikv/client-go/v2/util/async"
	pd "github.com/tikv/pd/client"
	"github.com/tikv/pd/client/clients/router"
	"github.com/tikv/pd/client/opt"
	"github.com/tikv/pd/client/pkg/caller"
)

type readTestPD struct {
	pd.Client
	region func(context.Context) (*router.Region, error)
}

func (p *readTestPD) WithCallerComponent(caller.Component) pd.Client { return p }
func (p *readTestPD) GetClusterID(context.Context) uint64            { return 1 }
func (p *readTestPD) GetAllStores(context.Context, ...opt.GetStoreOption) ([]*metapb.Store, error) {
	return nil, nil
}
func (p *readTestPD) GetRegion(ctx context.Context, _ []byte, _ ...opt.GetRegionOption) (*router.Region, error) {
	return p.region(ctx)
}

type readTestRPC struct {
	tikv.Client
	send func(context.Context, string, *tikvrpc.Request) (*tikvrpc.Response, error)
}

func (c readTestRPC) SendRequest(ctx context.Context, addr string, req *tikvrpc.Request, _ time.Duration) (*tikvrpc.Response, error) {
	return c.send(ctx, addr, req)
}
func (c readTestRPC) SendRequestAsync(ctx context.Context, addr string, req *tikvrpc.Request, cb async.Callback[*tikvrpc.Response]) {
	r, e := c.send(ctx, addr, req)
	cb.Invoke(r, e)
}

type readTestContext struct{ ctx context.Context }

func (c readTestContext) Name() string                   { return "caller" }
func (c readTestContext) CallerContext() context.Context { return c.ctx }
func (c readTestContext) Wrap(next interceptor.RPCInterceptorFunc) interceptor.RPCInterceptorFunc {
	return next
}

func TestTiKVLocalReaderGuardsSyncAndAsync(t *testing.T) {
	cache := tikv.NewRegionCache(&readTestPD{})
	defer cache.Close()
	labels := []*metapb.StoreLabel{{Key: "zone", Value: "near"}}
	cache.SetRegionCacheStore(1, "near:20160", "", tikvrpc.TiKV, 1, labels)
	cache.SetRegionCacheStore(2, "far:20160", "", tikvrpc.TiKV, 1, []*metapb.StoreLabel{{Key: "zone", Value: "far"}})
	for _, asynchronous := range []bool{false, true} {
		for _, command := range []tikvrpc.CmdType{tikvrpc.CmdGet, tikvrpc.CmdBatchGet, tikvrpc.CmdScan} {
			for _, bad := range []string{"", "voter", "label", "address", "forwarded", "missing"} {
				var payload any = &kvrpcpb.GetRequest{Version: 91}
				if command == tikvrpc.CmdBatchGet {
					payload = &kvrpcpb.BatchGetRequest{Version: 91}
				}
				if command == tikvrpc.CmdScan {
					payload = &kvrpcpb.ScanRequest{Version: 91}
				}
				req := tikvrpc.NewRequest(command, payload, kvrpcpb.Context{Peer: &metapb.Peer{Id: 3, StoreId: 1, Role: metapb.PeerRole_Learner}})
				address := "near:20160"
				switch bad {
				case "voter":
					req.Peer.Role = metapb.PeerRole_Voter
				case "label":
					req.Peer.StoreId = 2
					address = "far:20160"
				case "address":
					address = "old:20160"
				case "forwarded":
					req.ForwardedHost = "center:20160"
				case "missing":
					req.Peer = nil
				}
				sent := 0
				client := &tiKVLocalReader{cache: cache, labels: labels, Client: readTestRPC{send: func(_ context.Context, addr string, r *tikvrpc.Request) (*tikvrpc.Response, error) {
					sent++
					if addr != "near:20160" || !r.StaleRead || r.ReplicaRead {
						t.Fatal("invalid historical RPC")
					}
					version := r.GetStartTS()
					if version != 91 {
						t.Fatalf("version=%d", version)
					}
					return &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, nil
				}}}
				var err error
				if asynchronous {
					calls := 0
					client.SendRequestAsync(t.Context(), address, req, async.NewCallback(nil, func(_ *tikvrpc.Response, e error) { calls++; err = e }))
					if calls != 1 {
						t.Fatal("callback not completed exactly once")
					}
				} else {
					_, err = client.SendRequest(t.Context(), address, req, time.Second)
				}
				if bad == "" {
					if err != nil || sent != 1 {
						t.Fatalf("valid target: %d %v", sent, err)
					}
				} else if !errors.Is(err, ErrTiKVReadDenied) || sent != 0 {
					t.Fatalf("blocked target %s: sent=%d error=%v", bad, sent, err)
				}
			}
		}
	}
	cache.SetRegionCacheStore(1, "near:20160", "", tikvrpc.TiKV, 1, nil)
	client := &tiKVLocalReader{cache: cache, labels: labels}
	if err := client.prepare("near:20160", tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{}, kvrpcpb.Context{Peer: &metapb.Peer{StoreId: 1, Role: metapb.PeerRole_Learner}})); !errors.Is(err, ErrTiKVReadDenied) {
		t.Fatal("changed cached labels were ignored")
	}
}

func TestTiKVReadContextCancellationAndMetadata(t *testing.T) {
	callerCtx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	ctx := interceptor.WithRPCInterceptor(context.Background(), readTestContext{callerCtx})
	client := &tiKVLocalReader{Client: readTestRPC{send: func(ctx context.Context, _ string, _ *tikvrpc.Request) (*tikvrpc.Response, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}}
	start := time.Now()
	_, err := client.SendRequest(ctx, "local", tikvrpc.NewRequest(tikvrpc.CmdStoreSafeTS, &kvrpcpb.StoreSafeTSRequest{}), time.Minute)
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("lost caller deadline: %v elapsed=%v", err, time.Since(start))
	}
	var cancelled tiKVReadCanceled
	if !errors.As(err, &cancelled) || cancelled.Cause() != context.Canceled {
		t.Fatal("SDK cancellation did not remain terminal")
	}
	p := &tiKVReadPD{Client: &readTestPD{region: func(ctx context.Context) (*router.Region, error) {
		if ctx.Err() == nil {
			t.Fatal("metadata lost caller cancellation")
		}
		return nil, ctx.Err()
	}}}
	_, err = p.WithCallerComponent(caller.Component("region-cache")).GetRegion(ctx, nil)
	if err == nil {
		t.Fatal("metadata ignored elapsed deadline")
	}
}

func TestTiKVReaderConfigAndBorrowing(t *testing.T) {
	cfg := TiKVConfig{PDAddresses: []string{"pd:2379"}, ReadLocalOnly: true}
	if err := ValidateTiKVConfig(cfg); err == nil {
		t.Fatal("accepted local-only without stale read labels")
	}
	cfg.StaleReadDelay = time.Second
	cfg.ReadLabels = map[string]string{"zone": "near"}
	if err := ValidateTiKVConfig(cfg); err != nil {
		t.Fatal(err)
	}
	w, r := &tikv.KVStore{}, &tikv.KVStore{}
	s := &Storage{tikvs: map[string]*tikv.KVStore{"test": w}, tikvReaders: map[string]*tikv.KVStore{}}
	if got, err := s.TiKVReader("test"); err != nil || got != w {
		t.Fatal("legacy client changed")
	}
	s.tikvReaders["test"] = r
	if got, err := s.TiKVReader("test"); err != nil || got != r {
		t.Fatal("separate reader lost")
	}
	s.closed = true
	if _, err := s.TiKVReader("test"); err == nil {
		t.Fatal("borrowed a closed registry")
	}
}
