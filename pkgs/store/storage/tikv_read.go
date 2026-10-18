package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// ErrTiKVReadDenied identifies a reader transport rejecting a target before
// network I/O. The SDK may consume it while retrying another local replica.
var ErrTiKVReadDenied = errors.New("storage: tikv reader requires a matching local Learner")

// TiKVReader borrows the connector's historical-read client. With
// read_local_only it has its own RPC connections, Region cache and health
// state; otherwise it is the same client returned by TiKV. Storage owns both.
func (s *Storage) TiKVReader(name string) (*tikv.KVStore, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New("storage: registry is closed")
	}
	if reader := s.tikvReaders[name]; reader != nil {
		return reader, nil
	}
	if writer := s.tikvs[name]; writer != nil {
		return writer, nil
	}
	return nil, fmt.Errorf("storage: tikv %q not found", name)
}

type tiKVLocalReader struct {
	tikv.Client
	cache  *tikv.RegionCache
	labels []*metapb.StoreLabel
}

func newTiKVLocalReader(store *tikv.KVStore, labels map[string]string) *tiKVLocalReader {
	c := &tiKVLocalReader{Client: store.GetTiKVClient(), cache: store.GetRegionCache()}
	for key, value := range labels {
		c.labels = append(c.labels, &metapb.StoreLabel{Key: key, Value: value})
	}
	return c
}

// The SDK owns selection, retries, batch splitting and scan pagination. Check
// the selected peer against the same PD-backed cache immediately before both
// synchronous and asynchronous sends. Rejection affects only the reader's
// private routing state, never the writer's connection or health cache.
func (c *tiKVLocalReader) prepare(addr string, req *tikvrpc.Request) error {
	if req.Type != tikvrpc.CmdGet && req.Type != tikvrpc.CmdBatchGet && req.Type != tikvrpc.CmdScan {
		return nil
	}
	if req.Peer == nil || req.Peer.Role != metapb.PeerRole_Learner || req.ForwardedHost != "" {
		return ErrTiKVReadDenied
	}
	for _, store := range c.cache.GetAllStores() {
		if store.StoreID() == req.Peer.StoreId && store.GetAddr() == addr && store.IsLabelsMatch(c.labels) {
			// Native scanners do not set StaleRead, and SDK retries can clear it.
			// Keep historical reads local without changing the request's version.
			req.StaleRead, req.ReplicaRead = true, false
			return nil
		}
	}
	return ErrTiKVReadDenied
}

func (c *tiKVLocalReader) SendRequest(ctx context.Context, addr string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	ctx, cancel, caller := tiKVCallContext(ctx)
	defer cancel()
	if err := tiKVCallerError(caller); err != nil {
		return nil, tiKVReadCanceled{err}
	}
	if err := c.prepare(addr, req); err != nil {
		return nil, err
	}
	resp, err := c.Client.SendRequest(ctx, addr, req, min(timeout, 5*time.Second))
	if cause := tiKVCallerError(caller); cause != nil {
		err = tiKVReadCanceled{cause}
	}
	return resp, err
}

func (c *tiKVLocalReader) SendRequestAsync(ctx context.Context, addr string, req *tikvrpc.Request, cb async.Callback[*tikvrpc.Response]) {
	ctx, cancel, caller := tiKVCallContext(ctx)
	if err := tiKVCallerError(caller); err != nil {
		cancel()
		cb.Invoke(nil, tiKVReadCanceled{err})
		return
	}
	if err := c.prepare(addr, req); err != nil {
		cancel()
		cb.Invoke(nil, err)
		return
	}
	cb.Inject(func(resp *tikvrpc.Response, err error) (*tikvrpc.Response, error) {
		cancel()
		if cause := tiKVCallerError(caller); cause != nil {
			err = tiKVReadCanceled{cause}
		}
		return resp, err
	})
	c.Client.SendRequestAsync(ctx, addr, req, cb)
}

// CallerContext is supplied by the KV adapter's snapshot interceptor. Native
// scanners otherwise use a background context. Preserve SDK cancellation too
// (for example, cancellation of sibling batches) and release every callback.
func tiKVCallContext(ctx context.Context) (context.Context, context.CancelFunc, context.Context) {
	caller := ctx
	if bound, ok := interceptor.GetRPCInterceptorFromCtx(ctx).(interface{ CallerContext() context.Context }); ok {
		caller = bound.CallerContext()
	}
	deadline := time.Now().Add(5 * time.Second)
	if d, ok := caller.Deadline(); ok {
		if d.Before(deadline) {
			deadline = d
		}
	}
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	stop := context.AfterFunc(caller, cancel)
	return callCtx, func() { stop(); cancel() }, caller
}

// A child RPC timer can fire just before the caller's timer at the same
// deadline. Treat that elapsed caller deadline as cancellation as well.
func tiKVCallerError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d, ok := ctx.Deadline(); ok && !time.Now().Before(d) {
		return context.DeadlineExceeded
	}
	return nil
}

// This is actual caller cancellation. Cause gives the SDK's legacy error API
// its terminal cancellation signal; Unwrap preserves DeadlineExceeded for Go
// callers instead of turning an elapsed deadline into another Store failure.
type tiKVReadCanceled struct{ err error }

func (e tiKVReadCanceled) Error() string { return e.err.Error() }
func (e tiKVReadCanceled) Cause() error  { return context.Canceled }
func (e tiKVReadCanceled) Unwrap() error { return e.err }

// Bind scanner metadata I/O to the same caller as its data RPCs. The embedded
// client retains normal background discovery, timestamps, TLS and lifecycle.
type tiKVReadPD struct{ pd.Client }

func (p *tiKVReadPD) WithCallerComponent(component caller.Component) pd.Client {
	return &tiKVReadPD{Client: p.Client.WithCallerComponent(component)}
}

func (p *tiKVReadPD) GetRegion(ctx context.Context, key []byte, options ...opt.GetRegionOption) (*router.Region, error) {
	ctx, cancel, _ := tiKVCallContext(ctx)
	defer cancel()
	return p.Client.GetRegion(ctx, key, options...)
}
func (p *tiKVReadPD) GetPrevRegion(ctx context.Context, key []byte, options ...opt.GetRegionOption) (*router.Region, error) {
	ctx, cancel, _ := tiKVCallContext(ctx)
	defer cancel()
	return p.Client.GetPrevRegion(ctx, key, options...)
}
func (p *tiKVReadPD) GetRegionByID(ctx context.Context, id uint64, options ...opt.GetRegionOption) (*router.Region, error) {
	ctx, cancel, _ := tiKVCallContext(ctx)
	defer cancel()
	return p.Client.GetRegionByID(ctx, id, options...)
}
func (p *tiKVReadPD) ScanRegions(ctx context.Context, start, end []byte, limit int, options ...opt.GetRegionOption) ([]*router.Region, error) {
	ctx, cancel, _ := tiKVCallContext(ctx)
	defer cancel()
	return p.Client.ScanRegions(ctx, start, end, limit, options...)
}
func (p *tiKVReadPD) BatchScanRegions(ctx context.Context, ranges []router.KeyRange, limit int, options ...opt.GetRegionOption) ([]*router.Region, error) {
	ctx, cancel, _ := tiKVCallContext(ctx)
	defer cancel()
	return p.Client.BatchScanRegions(ctx, ranges, limit, options...)
}
func (p *tiKVReadPD) GetStore(ctx context.Context, id uint64, options ...opt.GetStoreOption) (*metapb.Store, error) {
	ctx, cancel, _ := tiKVCallContext(ctx)
	defer cancel()
	return p.Client.GetStore(ctx, id, options...)
}
