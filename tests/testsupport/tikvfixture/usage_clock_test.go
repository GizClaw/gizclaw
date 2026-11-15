package tikvfixture

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/errorpb"
	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/oracle"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

type usageClockOracle struct {
	oracle.Oracle
	cached      atomic.Uint64
	freshCalls  atomic.Int64
	validated   atomic.Uint64
	cacheErr    error
	validateErr error
}

func (o *usageClockOracle) GetLowResolutionTimestamp(context.Context, *oracle.Option) (uint64, error) {
	return o.cached.Load(), o.cacheErr
}

func (o *usageClockOracle) GetTimestamp(context.Context, *oracle.Option) (uint64, error) {
	o.freshCalls.Add(1)
	return o.cached.Load() + 1, nil
}

type usageClockFutureFunc func() (uint64, error)

func (f usageClockFutureFunc) Wait() (uint64, error) { return f() }

func (o *usageClockOracle) GetLowResolutionTimestampAsync(ctx context.Context, opt *oracle.Option) oracle.Future {
	ts, err := o.GetLowResolutionTimestamp(ctx, opt)
	return usageClockFutureFunc(func() (uint64, error) { return ts, err })
}

func (o *usageClockOracle) GetTimestampAsync(ctx context.Context, opt *oracle.Option) oracle.Future {
	ts, err := o.GetTimestamp(ctx, opt)
	return usageClockFutureFunc(func() (uint64, error) { return ts, err })
}

func (o *usageClockOracle) ValidateReadTS(_ context.Context, ts uint64, _ bool, _ *oracle.Option) error {
	o.validated.Store(ts)
	return o.validateErr
}

func TestUsageReadClockRetainsCacheAndPreservesFreshOracle(t *testing.T) {
	ctx := t.Context()
	gate, err := StartUsageGate(&gateTransport{}, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gate.Close() })
	validationErr := errors.New("original SDK validation")
	delegate := &usageClockOracle{validateErr: validationErr}
	cached := oracle.GoTimeToTS(time.Now()) + 7 // Preserve actual logical TSO bits.
	delegate.cached.Store(cached)
	clock := gate.WrapReadClock(delegate, true)
	opt := &oracle.Option{TxnScope: oracle.GlobalTxnScope}
	if ts, err := clock.GetLowResolutionTimestamp(ctx, opt); err != nil || ts != cached {
		t.Fatalf("unfrozen cache = %d, %v", ts, err)
	}
	want := cached // Freezing cannot rewind the cached timestamp already issued.
	observed := cached - 1501<<18
	gate.recordNativeRead(t.Context(), tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Version: observed}, kvrpcpb.Context{Peer: &metapb.Peer{Role: metapb.PeerRole_Learner}}), &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, nil)
	var state struct {
		Frozen      bool   `json:"frozen"`
		ReadVersion uint64 `json:"read_version"`
	}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/read-clock", map[string]bool{"frozen": true}, &state); err != nil {
		t.Fatal(err)
	}
	if !state.Frozen || state.ReadVersion != want {
		t.Fatalf("captured real timestamp = %+v, want %d", state, want)
	}
	next := cached + 1<<18
	delegate.cached.Store(next)
	if ts, err := clock.GetLowResolutionTimestamp(ctx, opt); err != nil || ts != want {
		t.Fatalf("retained cache = %d, %v", ts, err)
	}
	if ts, err := clock.GetTimestamp(ctx, opt); err != nil || ts != next+1 || delegate.freshCalls.Load() != 1 {
		t.Fatalf("fresh writer oracle = %d, %v", ts, err)
	}
	if err := clock.ValidateReadTS(ctx, want, true, opt); !errors.Is(err, validationErr) || delegate.validated.Load() != want {
		t.Fatalf("original SDK validation was bypassed: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := clock.GetLowResolutionTimestamp(cancelled, opt); !errors.Is(err, context.Canceled) {
		t.Fatalf("frozen read ignored cancellation: %v", err)
	}
	if err := HTTP(ctx, http.MethodPost, gate.URL()+"/reset", map[string]any{}, nil); err != nil {
		t.Fatal(err)
	}
	if ts, err := clock.GetLowResolutionTimestamp(ctx, opt); err != nil || ts != next {
		t.Fatalf("reset did not restore live SDK cache: %d, %v", ts, err)
	}
}

func TestUsageReadClockRequiresObservedRegionalSnapshot(t *testing.T) {
	for _, test := range []struct {
		name     string
		enabled  bool
		observed uint64
		cached   uint64
		issued   uint64
		want     uint64
	}{
		{name: "labels-required", observed: 97, cached: 100},
		{name: "observation-required", enabled: true, cached: 100},
		{name: "cached-PD-required", enabled: true, observed: 97},
		{name: "observation-exceeds-PD", enabled: true, observed: 101, cached: 100},
		{name: "actual-snapshot", enabled: true, observed: 97, cached: 100, want: 97},
		{name: "observation-equals-PD", enabled: true, observed: 100, cached: 100, want: 100},
		{name: "issued-ahead-of-completion", enabled: true, observed: 97, cached: 100, issued: 99, want: 99},
		{name: "cache-regressed-behind-issuance", enabled: true, observed: 97, cached: 100, issued: 101},
	} {
		t.Run(test.name, func(t *testing.T) {
			delegate := &usageClockOracle{}
			delegate.cached.Store(test.cached)
			gate := &UsageGate{}
			gate.regionalReadTS.Store(test.observed)
			gate.WrapReadClock(delegate, test.enabled)
			clock := gate.readClock.Load()
			clock.issued = test.issued
			version, err := clock.setFrozen(t.Context(), true)
			if version != test.want || clock.frozen.Load() != test.want || (err != nil) != (test.want == 0) {
				t.Fatalf("freeze = %d, %v; retained %d; want %d", version, err, clock.frozen.Load(), test.want)
			}
		})
	}
}

func TestUsageGateObservesOnlySuccessfulRegionalDataSnapshots(t *testing.T) {
	for _, test := range []struct {
		name        string
		command     tikvrpc.CmdType
		response    any
		err         error
		center      bool
		nilResponse bool
		want        bool
	}{
		{name: "Get-present", command: tikvrpc.CmdGet, response: &kvrpcpb.GetResponse{Value: []byte("actual")}, want: true},
		{name: "Get-missing", command: tikvrpc.CmdGet, response: &kvrpcpb.GetResponse{NotFound: true}, want: true},
		{name: "BatchGet", command: tikvrpc.CmdBatchGet, response: &kvrpcpb.BatchGetResponse{Pairs: []*kvrpcpb.KvPair{{Value: []byte("actual")}}}, want: true},
		{name: "BatchGet-empty", command: tikvrpc.CmdBatchGet, response: &kvrpcpb.BatchGetResponse{}, want: true},
		{name: "Scan", command: tikvrpc.CmdScan, response: &kvrpcpb.ScanResponse{Pairs: []*kvrpcpb.KvPair{{Value: []byte("actual")}}}, want: true},
		{name: "Scan-empty", command: tikvrpc.CmdScan, response: &kvrpcpb.ScanResponse{}, want: true},
		{name: "Get-region-error", command: tikvrpc.CmdGet, response: &kvrpcpb.GetResponse{RegionError: &errorpb.Error{}}},
		{name: "Get-key-error", command: tikvrpc.CmdGet, response: &kvrpcpb.GetResponse{Error: &kvrpcpb.KeyError{}}},
		{name: "BatchGet-region-error", command: tikvrpc.CmdBatchGet, response: &kvrpcpb.BatchGetResponse{RegionError: &errorpb.Error{}}},
		{name: "BatchGet-key-error", command: tikvrpc.CmdBatchGet, response: &kvrpcpb.BatchGetResponse{Error: &kvrpcpb.KeyError{}}},
		{name: "BatchGet-pair-error", command: tikvrpc.CmdBatchGet, response: &kvrpcpb.BatchGetResponse{Pairs: []*kvrpcpb.KvPair{{Error: &kvrpcpb.KeyError{}}}}},
		{name: "Scan-region-error", command: tikvrpc.CmdScan, response: &kvrpcpb.ScanResponse{RegionError: &errorpb.Error{}}},
		{name: "Scan-key-error", command: tikvrpc.CmdScan, response: &kvrpcpb.ScanResponse{Error: &kvrpcpb.KeyError{}}},
		{name: "Scan-pair-error", command: tikvrpc.CmdScan, response: &kvrpcpb.ScanResponse{Pairs: []*kvrpcpb.KvPair{{Error: &kvrpcpb.KeyError{}}}}},
		{name: "transport-error", command: tikvrpc.CmdGet, response: &kvrpcpb.GetResponse{}, err: errors.New("transport unavailable")},
		{name: "center-is-not-regional", command: tikvrpc.CmdGet, response: &kvrpcpb.GetResponse{}, center: true},
		{name: "missing-response", command: tikvrpc.CmdGet},
		{name: "nil-native-response", command: tikvrpc.CmdGet, nilResponse: true},
		{name: "typed-nil-Get", command: tikvrpc.CmdGet, response: (*kvrpcpb.GetResponse)(nil)},
		{name: "BatchGet-nil-pair", command: tikvrpc.CmdBatchGet, response: &kvrpcpb.BatchGetResponse{Pairs: []*kvrpcpb.KvPair{nil}}},
		{name: "Scan-nil-pair", command: tikvrpc.CmdScan, response: &kvrpcpb.ScanResponse{Pairs: []*kvrpcpb.KvPair{nil}}},
		{name: "mismatched-response", command: tikvrpc.CmdGet, response: &kvrpcpb.ScanResponse{}},
		{name: "write-is-not-read", command: tikvrpc.CmdCommit, response: &kvrpcpb.CommitResponse{}},
	} {
		for _, asynchronous := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/async=%t", test.name, asynchronous), func(t *testing.T) {
				native := &gateTransport{response: &tikvrpc.Response{Resp: test.response}, err: test.err, requests: make(chan *tikvrpc.Request, 1)}
				if test.nilResponse {
					native.response = nil
				}
				gate := &UsageGate{Client: native}
				receipt := new(usageReadReceipt)
				ctx := context.WithValue(t.Context(), usageReadReceiptKey{}, receipt)
				var payload any
				switch test.command {
				case tikvrpc.CmdGet:
					payload = &kvrpcpb.GetRequest{Version: 97}
				case tikvrpc.CmdBatchGet:
					payload = &kvrpcpb.BatchGetRequest{Version: 97}
				case tikvrpc.CmdScan:
					payload = &kvrpcpb.ScanRequest{Version: 97}
				case tikvrpc.CmdCommit:
					payload = &kvrpcpb.CommitRequest{CommitVersion: 97}
				}
				role := metapb.PeerRole_Learner
				if test.center {
					role = metapb.PeerRole_Voter
				}
				request := tikvrpc.NewRequest(test.command, payload, kvrpcpb.Context{Peer: &metapb.Peer{Role: role}, StaleRead: true})
				check := func(response *tikvrpc.Response, err error) {
					if response != native.response || !errors.Is(err, test.err) {
						t.Fatalf("native response changed: %p, %v", response, err)
					}
				}
				if asynchronous {
					done := make(chan struct{})
					gate.SendRequestAsync(ctx, "tikv:20160", request, async.NewCallback(nil, func(response *tikvrpc.Response, err error) {
						check(response, err)
						close(done)
					}))
					<-done
				} else {
					response, err := gate.SendRequest(ctx, "tikv:20160", request, time.Second)
					check(response, err)
				}
				want := uint64(0)
				if test.want {
					want = 97
				}
				if observed := gate.regionalReadTS.Load(); observed != want {
					t.Fatalf("observed snapshot = %d; want %d", observed, want)
				}
				wantReceipt := test.want && test.command == tikvrpc.CmdGet
				if receipt.learner.Load() != wantReceipt {
					t.Fatalf("Get-only caller receipt = %t; want %t", receipt.learner.Load(), wantReceipt)
				}
				receipt.learner.Store(false)
				gate.regionalReadTS.Store(100)
				gate.recordNativeRead(ctx, request, native.response, test.err)
				if gate.regionalReadTS.Load() != 100 || receipt.learner.Load() != wantReceipt {
					t.Fatal("older response regressed the observed snapshot or lost its independent receipt")
				}
			})
		}
	}
}

func TestUsageReadClockDoesNotFreezeFailedCache(t *testing.T) {
	failure := errors.New("cache unavailable")
	delegate := &usageClockOracle{cacheErr: failure}
	clock := &usageReadClock{Oracle: delegate, regionalEnabled: true}
	if _, err := clock.setFrozen(t.Context(), true); !errors.Is(err, failure) || clock.frozen.Load() != 0 {
		t.Fatalf("failed cache was replaced: %v", err)
	}
}

type usageClockBlockedTransport struct {
	*gateTransport
	version uint64
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *usageClockBlockedTransport) SendRequest(ctx context.Context, address string, req *tikvrpc.Request, timeout time.Duration) (*tikvrpc.Response, error) {
	var err error
	if req.Type == tikvrpc.CmdGet && req.Get().Version == c.version {
		c.once.Do(func() {
			close(c.started)
			select {
			case <-c.release:
			case <-ctx.Done():
				err = ctx.Err()
			}
		})
	}
	if err != nil {
		return nil, err
	}
	return c.gateTransport.SendRequest(ctx, address, req, timeout)
}

func (c *usageClockBlockedTransport) SendRequestAsync(ctx context.Context, address string, req *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	go func() {
		response, err := c.SendRequest(ctx, address, req, time.Second)
		callback.Invoke(response, err)
	}()
}

func usageClockSelect(ctx context.Context, clock oracle.Oracle, asynchronous bool) (uint64, error) {
	opt := &oracle.Option{TxnScope: oracle.GlobalTxnScope}
	if asynchronous {
		return clock.GetLowResolutionTimestampAsync(ctx, opt).Wait()
	}
	return clock.GetLowResolutionTimestamp(ctx, opt)
}

func TestUsageReadClockFreezeIncludesIssuedSnapshotBeforeNativeCompletion(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		t.Run(fmt.Sprintf("async=%t", asynchronous), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			native := &usageClockBlockedTransport{
				gateTransport: &gateTransport{response: &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, requests: make(chan *tikvrpc.Request, 1)},
				version:       99,
				started:       make(chan struct{}),
				release:       make(chan struct{}),
			}
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(native.release) }) }
			defer release()
			gate, err := StartUsageGate(native, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer gate.Close()
			delegate := &usageClockOracle{}
			delegate.cached.Store(97)
			clock := gate.WrapReadClock(delegate, true)
			read := func() error {
				version, err := usageClockSelect(ctx, clock, asynchronous)
				if err != nil {
					return err
				}
				request := tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Version: version}, kvrpcpb.Context{Peer: &metapb.Peer{Role: metapb.PeerRole_Learner}})
				if !asynchronous {
					_, err := gate.SendRequest(ctx, "learner:20160", request, time.Second)
					return err
				}
				done := make(chan error, 1)
				gate.SendRequestAsync(ctx, "learner:20160", request, async.NewCallback(nil, func(_ *tikvrpc.Response, err error) { done <- err }))
				select {
				case err := <-done:
					return err
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if err := read(); err != nil {
				t.Fatal(err)
			}
			<-native.requests
			delegate.cached.Store(99)
			late := make(chan error, 1)
			go func() { late <- read() }()
			select {
			case <-native.started:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if gate.regionalReadTS.Load() != 97 {
				t.Fatal("the newer request completed before the native transport hold")
			}
			var state struct {
				Frozen      bool   `json:"frozen"`
				ReadVersion uint64 `json:"read_version"`
			}
			if err := HTTP(ctx, http.MethodPost, gate.URL()+"/read-clock", map[string]bool{"frozen": true}, &state); err != nil {
				t.Fatal(err)
			}
			if !state.Frozen || state.ReadVersion != 99 || gate.regionalReadTS.Load() != 97 {
				t.Fatalf("freeze rewound an issued snapshot or waited for its native completion: %+v", state)
			}
			release()
			select {
			case err := <-late:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			<-native.requests
			if gate.regionalReadTS.Load() != state.ReadVersion {
				t.Fatal("late native completion exceeded the acknowledged frozen version")
			}
			delegate.cached.Store(101)
			for range 3 {
				if err := read(); err != nil {
					t.Fatal(err)
				}
				if request := <-native.requests; request.Get().Version != state.ReadVersion {
					t.Fatalf("native ordinary version = %d; frozen acknowledgment = %d", request.Get().Version, state.ReadVersion)
				}
			}
		})
	}
}

type usageClockBlockedKey struct{}

type usageClockBlockedOracle struct {
	*usageClockOracle
	started    chan struct{}
	release    chan struct{}
	version    uint64
	failure    error
	futureWait bool
}

func (o *usageClockBlockedOracle) GetLowResolutionTimestamp(ctx context.Context, opt *oracle.Option) (uint64, error) {
	if blocked, _ := ctx.Value(usageClockBlockedKey{}).(bool); !blocked {
		return o.usageClockOracle.GetLowResolutionTimestamp(ctx, opt)
	}
	close(o.started)
	select {
	case <-o.release:
		return o.version, o.failure
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (o *usageClockBlockedOracle) GetLowResolutionTimestampAsync(ctx context.Context, opt *oracle.Option) oracle.Future {
	if o.futureWait {
		return usageClockFutureFunc(func() (uint64, error) { return o.GetLowResolutionTimestamp(ctx, opt) })
	}
	ts, err := o.GetLowResolutionTimestamp(ctx, opt)
	return usageClockFutureFunc(func() (uint64, error) { return ts, err })
}

func TestUsageReadClockFreezeRechecksOutstandingCachedSelectors(t *testing.T) {
	for _, mode := range []string{"sync", "async-create", "async-wait"} {
		for _, test := range []struct {
			name    string
			version uint64
			failure error
			cancel  bool
		}{
			{name: "newer-cache", version: 139},
			{name: "cache-error", failure: errors.New("cache unavailable")},
			{name: "empty-cache"},
			{name: "cancelled", version: 139, cancel: true},
		} {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				delegate := &usageClockBlockedOracle{
					usageClockOracle: &usageClockOracle{}, started: make(chan struct{}), release: make(chan struct{}),
					version: test.version, failure: test.failure, futureWait: mode == "async-wait",
				}
				delegate.cached.Store(100)
				gate := new(UsageGate)
				gate.regionalReadTS.Store(97)
				clock := gate.WrapReadClock(delegate, true)
				selectorContext, cancelSelector := context.WithCancel(context.WithValue(ctx, usageClockBlockedKey{}, true))
				defer cancelSelector()
				type result struct {
					version uint64
					err     error
				}
				done := make(chan result, 1)
				go func() {
					version, err := usageClockSelect(selectorContext, clock, mode != "sync")
					done <- result{version, err}
				}()
				select {
				case <-delegate.started:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				frozen, err := gate.readClock.Load().setFrozen(ctx, true)
				if err != nil || frozen != 97 {
					t.Fatalf("freeze while selector delegates = %d, %v", frozen, err)
				}
				if test.cancel {
					cancelSelector()
				}
				close(delegate.release)
				select {
				case got := <-done:
					if test.cancel {
						if got.version != 0 || !errors.Is(got.err, context.Canceled) {
							t.Fatalf("canceled selector = %+v", got)
						}
					} else if got.err != nil || got.version != frozen {
						t.Fatalf("outstanding selector escaped frozen publication: %+v; frozen %d", got, frozen)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				if delegate.freshCalls.Load() != 0 {
					t.Fatal("frozen selector reached the fresh timestamp fallback")
				}
			})
		}
	}
}

func TestUsageReadClockRejectsFreezeDuringFreshCacheFallback(t *testing.T) {
	for _, asynchronous := range []bool{false, true} {
		for _, cacheError := range []bool{false, true} {
			t.Run(fmt.Sprintf("async=%t/error=%t", asynchronous, cacheError), func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				delegate := &usageClockOracle{}
				if cacheError {
					delegate.cacheErr = errors.New("cache unavailable")
				}
				gate := new(UsageGate)
				gate.regionalReadTS.Store(97)
				clock := gate.WrapReadClock(delegate, true)
				if version, err := usageClockSelect(ctx, clock, asynchronous); version != 0 || !errors.Is(err, delegate.cacheErr) {
					t.Fatalf("original cache failure = %d, %v", version, err)
				}
				delegate.cacheErr = nil
				delegate.cached.Store(100)
				// This is the production reader's GetTimestampWithRetry fallback. It
				// must remain authoritative, including when its native read is delayed.
				fresh, err := clock.GetTimestamp(ctx, &oracle.Option{TxnScope: oracle.GlobalTxnScope})
				if err != nil || fresh != 101 {
					t.Fatalf("fresh fallback = %d, %v", fresh, err)
				}
				delegate.cached.Store(102)
				if frozen, err := gate.readClock.Load().setFrozen(t.Context(), true); err == nil || frozen != 0 {
					t.Fatal("freeze acknowledged while an untracked fresh fallback could advance the floor")
				}
				gate.recordNativeRead(ctx, tikvrpc.NewRequest(tikvrpc.CmdGet, &kvrpcpb.GetRequest{Version: fresh}, kvrpcpb.Context{Peer: &metapb.Peer{Role: metapb.PeerRole_Learner}}), &tikvrpc.Response{Resp: &kvrpcpb.GetResponse{}}, nil)
				// The regional read view closes its context after snapshot completion.
				cancel()
				frozen, err := gate.readClock.Load().setFrozen(t.Context(), true)
				if err != nil || frozen != fresh {
					t.Fatalf("freeze after fresh fallback completion = %d, %v", frozen, err)
				}
				if version, err := usageClockSelect(t.Context(), clock, asynchronous); err != nil || version != frozen {
					t.Fatalf("post-fallback frozen selector = %d, %v", version, err)
				}
				opt := &oracle.Option{TxnScope: oracle.GlobalTxnScope}
				if version, err := clock.GetTimestamp(t.Context(), opt); err != nil || version != 103 {
					t.Fatalf("frozen writer timestamp = %d, %v", version, err)
				}
				if version, err := clock.GetTimestampAsync(t.Context(), opt).Wait(); err != nil || version != 103 || delegate.freshCalls.Load() != 3 {
					t.Fatalf("frozen async writer timestamp = %d, %v; calls %d", version, err, delegate.freshCalls.Load())
				}
			})
		}
	}
}
