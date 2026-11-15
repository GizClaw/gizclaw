package kv

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	"github.com/pingcap/kvproto/pkg/metapb"
	"github.com/tikv/client-go/v2/tikv"
	"github.com/tikv/client-go/v2/tikvrpc"
	"github.com/tikv/client-go/v2/util/async"
)

func TestTiKVNativeLearnerPolicy(t *testing.T) {
	labels := []*metapb.StoreLabel{{Key: "region", Value: "bj"}, {Key: "$mode", Value: "readonly"}}
	if !tiKVMatchesLearner(metapb.PeerRole_Learner, labels, labels) {
		t.Fatal("matching Learner rejected")
	}
	if tiKVMatchesLearner(metapb.PeerRole_Voter, labels, labels) {
		t.Fatal("a Leader/Follower was accepted as a regional data target")
	}
	if tiKVMatchesLearner(metapb.PeerRole_Learner, labels[:1], labels) {
		t.Fatal("partially matching Learner accepted")
	}
	other := []*metapb.StoreLabel{{Key: "region", Value: "gz"}, {Key: "$mode", Value: "readonly"}}
	if tiKVMatchesLearner(metapb.PeerRole_Learner, other, labels) {
		t.Fatal("another region was accepted")
	}
}

func TestTiKVLegacyDelayIgnoredByNativeReads(t *testing.T) {
	for _, delay := range []time.Duration{-time.Hour, 0, time.Millisecond, 25 * time.Hour} {
		client := new(tikv.KVStore)
		clock := &tiKVSnapshotOracle{cachedTS: 100, freshTS: 200}
		client.SetOracle(clock)
		store, err := NewTiKVWithClient(client, TiKVReadOptions{StaleReadDelay: delay, Labels: map[string]string{"region": "bj"}}, nil)
		if err != nil || store.local == nil {
			t.Fatalf("labels did not enable local read visibility: %+v, %v", store, err)
		}
		_, ts, err := store.snapshotWithTimestamp(t.Context())
		if err != nil || ts != clock.cachedTS || clock.freshCalls != 0 {
			t.Fatalf("legacy delay altered native snapshot: delay=%v ts=%d err=%v", delay, ts, err)
		}
		plain, err := NewTiKVWithClient(client, TiKVReadOptions{StaleReadDelay: delay}, nil)
		if err != nil || plain.local != nil {
			t.Fatalf("legacy delay alone enabled the read/write layer: %+v, %v", plain, err)
		}
	}
}

type tiKVPolicyExecutor struct{}

func (tiKVPolicyExecutor) Go(f func()) { f() }
func (tiKVPolicyExecutor) Append(fs ...func()) {
	for _, f := range fs {
		f()
	}
}

type tiKVAsyncPolicyProbe struct {
	tikv.Client
	calls int
}

func (p *tiKVAsyncPolicyProbe) SendRequestAsync(_ context.Context, _ string, _ *tikvrpc.Request, callback async.Callback[*tikvrpc.Response]) {
	p.calls++
	callback.Schedule(&tikvrpc.Response{}, nil)
}

func TestTiKVAsyncPolicyRejectsBeforeDispatch(t *testing.T) {
	caller := t.Context()
	store := newTiKVSnapshotTestStore(t, &tiKVSnapshotOracle{cachedTS: 100}, true)
	view, err := store.readView(caller, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer view.close()
	probe := new(tiKVAsyncPolicyProbe)
	client := &tiKVReadPolicyClient{Client: probe}
	request := tikvrpc.NewRequest(tikvrpc.CmdBatchGet, &kvrpcpb.BatchGetRequest{Keys: [][]byte{[]byte("a"), []byte("b")}},
		kvrpcpb.Context{Peer: &metapb.Peer{Role: metapb.PeerRole_Voter}})
	result := make(chan error, 1)
	callback := func(_ *tikvrpc.Response, err error) { result <- err }
	client.SendRequestAsync(view.ctx, "leader:20160", request, async.NewCallback(tiKVPolicyExecutor{}, callback))
	if got := <-result; !errors.Is(view.readError(got), errTiKVRegionalReadTarget) || probe.calls != 0 || caller.Err() != nil {
		t.Fatalf("async policy sent forbidden data: err=%v calls=%d caller=%v", got, probe.calls, caller.Err())
	}
	// The same decorator must leave ordinary primary/unconfigured traffic alone.
	client.SendRequestAsync(caller, "leader:20160", request, async.NewCallback(tiKVPolicyExecutor{}, callback))
	if got := <-result; got != nil || probe.calls != 1 {
		t.Fatalf("unconfigured async traffic changed: err=%v calls=%d", got, probe.calls)
	}
}

func TestTiKVReadPolicyCancellationDoesNotCancelCaller(t *testing.T) {
	caller := t.Context()
	store := newTiKVSnapshotTestStore(t, &tiKVSnapshotOracle{cachedTS: 100, freshTS: 200}, true)
	view, err := store.readView(caller, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer view.close()
	view.cancel(errTiKVRegionalReadTarget)
	if !errors.Is(view.readError(context.Canceled), errTiKVRegionalReadTarget) || caller.Err() != nil {
		t.Fatalf("policy cancellation leaked to caller: caller=%v cause=%v", caller.Err(), context.Cause(view.ctx))
	}
	cancelled, cancel := context.WithCancel(caller)
	other, err := store.readView(cancelled, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.close()
	cancel()
	if !errors.Is(other.readError(context.Canceled), context.Canceled) {
		t.Fatal("caller cancellation was replaced with a routing error")
	}
}
